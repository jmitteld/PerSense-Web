package amortization

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestDOSSolverOptionsAudit — 2026-07-24 audit pass: every backward solver
// crossed with the advanced options the earlier fuzzers did NOT cross.
//
// Coverage map before this test (see dos_fuzzer3_fancy_test.go):
//
//	{payment, rate, amount} × ONE of {mor, balloon, pre, skip}   ← covered
//
// Gaps this test closes:
//   - TERM solve (solveterm) × options
//   - BALLOON-AMOUNT solve (solveballoon=) × options
//   - AO9 prepay-amount solve (presolve=) × options
//   - AO10 prepay-duration solve (predur=) × options
//   - ADJUSTMENTS (rate-only and set-both) × {amount, rate, term} solves
//   - TARGET (principal minimum) × {amount, rate, term} solves
//   - STACKED option pairs × {amount, rate, term} solves
//
// Known-frontier exclusions (deliberate, documented):
//   - AO6/AO7 adjustments (payment-only / date-only) combined with balloons:
//     confirmed DOS print-path corruption, the port intentionally diverges
//     (docs/dos_known_frontier.md). Only rate-only / set-both adjustments here.
//   - Rule-of-78 × fancy options: DOS treats R78 as a basic-loan method.
//
// Classification mirrors fuzzer3-fancy: hard-fail only when Go produces a
// DEGENERATE solve where DOS refuses; log valid-root-DOS-missed, conservative
// Go refusals, and value divergences (worst-case reported).
func TestDOSSolverOptionsAudit(t *testing.T) {
	gateOracle(t)
	rng := rand.New(rand.NewSource(fuzzSeed(0x50f7ad17)))

	N := 1200
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			N = v
		}
	}
	q6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	cents := func(x float64) float64 { return math.Round(x*100) / 100 }
	addMonths := func(months int) types.DateRec {
		return types.NewDateRec(2024+months/12, time.Month(months%12+1), 1)
	}

	const (
		dosSolved = iota
		dosRefused
		dosFlake
		dosDateHorizon
	)
	namedField := func(f []string, name string) (float64, bool) {
		for i := 0; i+1 < len(f); i++ {
			if f[i] == name {
				v, e := strconv.ParseFloat(f[i+1], 64)
				return v, e == nil
			}
		}
		return 0, false
	}
	// dosRun execs the oracle and pulls the labelled value for the solver.
	dosRun := func(label string, args []string) (float64, int) {
		for try := 0; try < 8; try++ {
			out, err := exec.Command(oracleBin, args...).Output()
			if err != nil {
				continue
			}
			f := strings.Fields(strings.TrimSpace(string(out)))
			if len(f) == 0 {
				continue
			}
			if f[0] == "ERR" || f[0] == "ENGINE" {
				msg := strings.ToLower(strings.Join(f, " "))
				if strings.Contains(msg, "julian") || strings.Contains(msg, "bad date") ||
					strings.Contains(msg, "last payment not found") {
					return 0, dosDateHorizon
				}
				return 0, dosRefused
			}
			// heap-flake sentinels: zero/negative primary value → retry
			v, ok := namedField(f, label)
			if !ok {
				return 0, dosRefused
			}
			if v == 0 && (label == "payment" || label == "prepay") {
				continue
			}
			if paid, has := namedField(f, "paid"); has && paid <= 0 {
				continue
			}
			// The balloon/rate/term queries also print the solved field's status
			// byte; a status other than 1 (outp) means MakeTable did NOT actually
			// solve it — treat as a refusal, not a solved 0. (First run misread
			// DOS `rate 0.000000 status 3` as a solved zero rate.)
			if label == "balloon" || label == "rate" || label == "term" {
				if st, has := namedField(f, "status"); has && st != 1 {
					return 0, dosRefused
				}
			}
			return v, dosSolved
		}
		return 0, dosFlake
	}

	retires := func(r AmortResult, principal float64) bool {
		if r.Err != nil || len(r.Schedule) == 0 {
			return false
		}
		bal := r.Schedule[len(r.Schedule)-1].Principal
		return math.Abs(bal) <= math.Max(1.0, 1e-4*math.Abs(principal))
	}

	// ---- option pool -------------------------------------------------------
	type opt struct {
		name  string
		flags []string
		apply func(in *LoanInput)
	}
	mkOpt := func(kind string, n, mPer, perYr int, amount, rate float64) (opt, bool) {
		switch kind {
		case "mor":
			k := 1 + rng.Intn(n/2+1)
			m := k * mPer
			return opt{"mor", []string{"mor=" + strconv.Itoa(m)}, func(in *LoanInput) {
				in.Fancy = true
				in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: addMonths(m)}
			}}, true
		case "skip":
			if perYr != 12 {
				return opt{}, false
			}
			start := 1 + rng.Intn(10)
			end := start + rng.Intn(3)
			if end > 12 {
				end = 12
			}
			ss := strconv.Itoa(start)
			if end > start {
				ss = fmt.Sprintf("%d-%d", start, end)
			}
			ms, _ := MonthSetFromString(ss)
			return opt{"skip", []string{"skip=" + ss}, func(in *LoanInput) {
				in.Fancy = true
				in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: ss, MonthSet: ms}
			}}, true
		case "pre":
			startM := (1 + rng.Intn(n/2+1)) * mPer
			nn := 1 + rng.Intn(n/2+1)
			amt := cents(100 + rng.Float64()*5000)
			return opt{"pre", []string{fmt.Sprintf("pre=%d:%d:%d:%.2f", startM, nn, perYr, amt)}, func(in *LoanInput) {
				in.Fancy = true
				in.Prepayments = append(in.Prepayments, Prepayment{
					StartDateStatus: types.InOutInput, StartDate: addMonths(startM),
					NNStatus: types.InOutInput, NN: nn,
					PerYrStatus: types.InOutInput, PerYr: perYr,
					PaymentStatus: types.InOutInput, Payment: amt})
			}}, true
		case "bal":
			bm := (1 + rng.Intn(n)) * mPer
			amt := cents(amount * (0.05 + rng.Float64()*0.8))
			return opt{"bal", []string{fmt.Sprintf("b%d=%.2f", bm, amt)}, func(in *LoanInput) {
				in.Fancy = true
				in.Balloons = append(in.Balloons, BalloonPayment{
					DateStatus: types.InOutInput, Date: addMonths(bm),
					AmountStatus: types.InOutInput, Amount: amt})
			}}, true
		case "adjR": // rate-only adjustment (AO5)
			am := (1 + rng.Intn(n-1)) * mPer
			r2 := q6(rate * (0.5 + rng.Float64()))
			return opt{"adjR", []string{fmt.Sprintf("adj=%d:%.6f:", am, r2)}, func(in *LoanInput) {
				in.Fancy = true
				in.Adjustments = append(in.Adjustments, RateAdjustment{
					DateStatus: types.InOutInput, Date: addMonths(am),
					LoanRateStatus: types.InOutInput, LoanRate: r2})
			}}, true
		case "adjB": // set-both adjustment (new rate AND new payment)
			am := (1 + rng.Intn(n-1)) * mPer
			r2 := q6(rate * (0.5 + rng.Float64()))
			p2 := cents((amount / float64(n)) * (0.8 + rng.Float64()*1.2))
			return opt{"adjB", []string{fmt.Sprintf("adj=%d:%.6f:%.2f", am, r2, p2)}, func(in *LoanInput) {
				in.Fancy = true
				in.Adjustments = append(in.Adjustments, RateAdjustment{
					DateStatus: types.InOutInput, Date: addMonths(am),
					LoanRateStatus: types.InOutInput, LoanRate: r2,
					AmountStatus: types.InOutInput, Amount: p2, AmtOK: true})
			}}, true
		case "targ":
			tv := cents((amount / float64(n)) * (0.2 + rng.Float64()*0.5))
			return opt{"targ", []string{fmt.Sprintf("targ=%.2f", tv)}, func(in *LoanInput) {
				in.Fancy = true
				in.Target = Target{TargetStatus: types.InOutInput, TargetValue: tv}
			}}, true
		}
		return opt{}, false
	}

	solvers := []string{
		"term", "term", "term",
		"balloonamt", "balloonamt", "balloonamt",
		"preamt", "preamt",
		"predur", "predur",
		"amount", "amount", "amount",
		"rate", "rate", "rate",
	}
	// option kinds per solver (drawn 1, sometimes 2)
	optPool := map[string][]string{
		"term":       {"mor", "skip", "pre", "bal", "adjR", "targ"},
		"balloonamt": {"mor", "skip", "pre", "targ", "adjR"},
		"preamt":     {"mor", "skip", "bal"},
		"predur":     {"mor", "skip", "bal"},
		"amount":     {"adjR", "adjB", "targ", "mor", "skip", "pre", "bal"},
		"rate":       {"adjR", "adjB", "targ", "mor", "skip", "pre", "bal"},
	}
	// stacked pairs allowed for amount/rate/term (new territory)
	pairPool := [][2]string{
		{"mor", "skip"}, {"mor", "targ"}, {"skip", "targ"}, {"bal", "skip"},
		{"pre", "mor"}, {"bal", "pre"}, {"adjR", "mor"}, {"adjR", "skip"},
	}

	bases := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}
	checked, bothRefused, flaked, dateHorizon := 0, 0, 0, 0
	goSolvesDosRefuses, goValidDosMisses, dosSolvesGoRefuses, valDiverge := 0, 0, 0, 0
	worstVal, worstMsg := 0.0, ""
	cover := map[string]int{}

	for c := 0; c < N; c++ {
		amount := cents(20000 + rng.Float64()*480000)
		rate := q6(0.02 + rng.Float64()*0.12)
		perYr := 12
		if rng.Intn(5) == 0 {
			perYr = []int{1, 2, 4}[rng.Intn(3)]
		}
		mPer := 12 / perYr
		years := 2 + rng.Intn(13)
		n := years * perYr
		basis := bases[rng.Intn(len(bases))]
		exact := rng.Intn(4) == 0
		prepaid := rng.Intn(3) != 0
		inadv := rng.Intn(4) == 0

		solver := solvers[rng.Intn(len(solvers))]

		// options: 1 draw from the solver's pool; amount/rate/term get a
		// stacked PAIR 40% of the time.
		var opts []opt
		usePair := (solver == "amount" || solver == "rate" || solver == "term") && rng.Intn(5) < 2
		if usePair {
			pr := pairPool[rng.Intn(len(pairPool))]
			o1, ok1 := mkOpt(pr[0], n, mPer, perYr, amount, rate)
			o2, ok2 := mkOpt(pr[1], n, mPer, perYr, amount, rate)
			if !ok1 || !ok2 {
				continue
			}
			opts = []opt{o1, o2}
		} else {
			pool := optPool[solver]
			o, ok := mkOpt(pool[rng.Intn(len(pool))], n, mPer, perYr, amount, rate)
			if !ok {
				continue
			}
			opts = []opt{o}
		}

		// plusreg: forced ON when prepayments are in play (the engine's
		// validated port surface and the oracle's predur both force additive).
		// EXCEPTION — the oracle's `solveballoon=` setup FORCES
		// `df.c.plus_regular := false` AFTER flag parsing (amort_oracle.pas:766),
		// so every balloon-amount solve runs REPLACE mode in DOS regardless of
		// the plusreg flag. Mirror that on the Go side or every balloonamt case
		// diverges by exactly one regular payment (the ADD-vs-REPLACE delta at
		// the balloon row — the first run of this audit demonstrated it).
		plusreg := true
		if solver == "balloonamt" {
			plusreg = false
		}

		s := gzSettings(perYr, basis, exact, prepaid, inadv, false, false)
		s.PlusRegular = plusreg
		var sflags []string
		if bf, ok := basisFlag(basis); ok {
			sflags = append(sflags, bf)
		}
		if exact {
			sflags = append(sflags, "exact")
		}
		if prepaid {
			sflags = append(sflags, "prepaid")
		}
		if inadv {
			sflags = append(sflags, "inadv")
		}
		if plusreg {
			sflags = append(sflags, "plusreg")
		}

		payment := cents((amount / float64(n)) * (0.5 + rng.Float64()*1.5))
		payFlag := "payhard=" + strconv.FormatFloat(payment, 'f', 2, 64)

		base := []string{
			strconv.FormatFloat(amount, 'f', 2, 64),
			strconv.FormatFloat(rate, 'f', 10, 64),
			strconv.Itoa(n), strconv.Itoa(perYr),
		}
		base = append(base, sflags...)
		for _, o := range opts {
			base = append(base, o.flags...)
		}

		mkIn := func() LoanInput {
			in := gzLoanInput(amount, rate, n, perYr, s)
			for _, o := range opts {
				o.apply(&in)
			}
			return in
		}

		var dosVal, goVal float64
		var outcome int
		var goOK, goValid bool
		intTol := false  // value compared as integer (term / duration)
		extraFlags := "" // solver-specific oracle flags, for the repro label

		switch solver {
		case "term":
			dosVal, outcome = dosRun("term", append(append([]string{}, base...), payFlag, "solveterm"))
			in := mkIn()
			in.Loan.NStatus = types.StatusEmpty
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, payment
			r := Amortize(in)
			if r.Err == nil && r.NPeriods > 0 {
				goVal, goOK = float64(r.NPeriods), true
				goValid = retires(r, amount)
			}
			intTol = true
		case "balloonamt":
			w := (n/2 + rng.Intn(n/2)) * mPer // terminating balloon in the back half
			extraFlags = fmt.Sprintf("solveballoon=%d", w)
			dosVal, outcome = dosRun("balloon", append(append([]string{}, base...), payFlag, extraFlags))
			in := mkIn()
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, payment
			in.Fancy = true
			in.Balloons = append(in.Balloons, BalloonPayment{
				DateStatus: types.InOutInput, Date: addMonths(w)}) // amount blank → solve
			r := Amortize(in)
			if r.Err == nil {
				for _, b := range r.Balloons {
					if b.Solved {
						goVal, goOK = b.Amount, true
					}
				}
				goValid = retires(r, amount)
			}
		case "preamt": // AO9
			startM := (1 + rng.Intn(n/3+1)) * mPer
			maxNN := (n*mPer - startM) / mPer
			if maxNN < 1 {
				continue
			}
			nn := 1 + rng.Intn(maxNN)
			extraFlags = fmt.Sprintf("presolve=%d:%d:%d", startM, nn, perYr)
			dosVal, outcome = dosRun("prepay", append(append([]string{}, base...), payFlag, extraFlags))
			in := mkIn()
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, payment
			in.Fancy = true
			in.Prepayments = append(in.Prepayments, Prepayment{
				StartDateStatus: types.InOutInput, StartDate: addMonths(startM),
				NNStatus: types.InOutInput, NN: nn,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PaymentStatus: types.StatusEmpty})
			r := Amortize(in)
			if r.Err == nil && r.SolvedPrepay > 0 {
				goVal, goOK = r.SolvedPrepay, true
				goValid = retires(r, amount)
			}
		case "predur": // AO10
			startM := (1 + rng.Intn(n/3+1)) * mPer
			amt := cents(200 + rng.Float64()*8000)
			extraFlags = fmt.Sprintf("predur=%d:%d:%.2f", startM, perYr, amt)
			dosVal, outcome = dosRun("duration", append(append([]string{}, base...), payFlag, extraFlags))
			in := mkIn()
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, payment
			in.Fancy = true
			in.Prepayments = append(in.Prepayments, Prepayment{
				StartDateStatus: types.InOutInput, StartDate: addMonths(startM),
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PaymentStatus: types.InOutInput, Payment: amt})
			pi := len(in.Prepayments) - 1
			r := Amortize(in)
			if r.Err == nil && in.Prepayments[pi].NNStatus >= types.InOutDefault && in.Prepayments[pi].NN > 0 {
				goVal, goOK = float64(in.Prepayments[pi].NN), true
				goValid = retires(r, amount)
			}
			intTol = true
		case "amount":
			dosVal, outcome = dosRun("solvedamount", append(append([]string{}, base...), payFlag, "noamt"))
			in := mkIn()
			in.Loan.AmountStatus = types.StatusEmpty
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, payment
			av, conv, err := SolveLoanAmount(in)
			goVal, goOK = av, conv && err == nil
			if goOK {
				chk := gzLoanInput(av, rate, n, perYr, s)
				for _, o := range opts {
					o.apply(&chk)
				}
				chk.Loan.PayAmtStatus, chk.Loan.PayAmt = types.InOutInput, payment
				goValid = retires(Amortize(chk), av)
			}
		default: // rate
			dosVal, outcome = dosRun("rate", append(append([]string{}, base...), payFlag, "solverate"))
			in := mkIn()
			in.Loan.LoanRateStatus = types.StatusEmpty
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, payment
			rv, conv, err := SolveRate(in)
			goVal, goOK = rv, conv && err == nil
			if goOK {
				chk := gzLoanInput(amount, rv, n, perYr, s)
				for _, o := range opts {
					o.apply(&chk)
				}
				chk.Loan.PayAmtStatus, chk.Loan.PayAmt = types.InOutInput, payment
				goValid = retires(Amortize(chk), amount)
			}
		}

		if outcome == dosFlake {
			flaked++
			continue
		}
		if outcome == dosDateHorizon {
			dateHorizon++
			continue
		}
		if goOK && (math.IsNaN(goVal) || math.IsInf(goVal, 0)) {
			goOK = false
		}
		// API-parity guard: the web's /calc endpoint refuses Rate Adjustments
		// together with in-advance interest ("Sorry - you can't change rates
		// when interest is computed in advance." in DOS; equivalent message in
		// the web). The engine-internal solvers this audit drives don't carry
		// that surface guard, so mirror it: the PRODUCT refuses these.
		if inadv {
			for _, o := range opts {
				if o.name == "adjR" || o.name == "adjB" {
					goOK = false
				}
			}
		}
		dosOK := outcome == dosSolved
		optNames := ""
		for _, o := range opts {
			optNames += o.name + "+"
		}
		optNames = strings.TrimSuffix(optNames, "+")
		label := fmt.Sprintf("%s [%s/%s] %s %s", strings.Join(base, " "), solver, optNames, payFlag, extraFlags)

		switch {
		case goOK && !dosOK:
			if goValid {
				goValidDosMisses++
			} else {
				goSolvesDosRefuses++
				if goSolvesDosRefuses <= 25 {
					t.Errorf("Go SOLVES (degenerate) where DOS REFUSES: Go=%.6f | %s", goVal, label)
				}
			}
		case !goOK && dosOK:
			dosSolvesGoRefuses++
		case !goOK && !dosOK:
			bothRefused++
		default:
			checked++
			cover[solver+"/"+optNames]++
			var diverged bool
			var rel float64
			switch {
			case solver == "rate":
				if d := math.Abs(goVal - dosVal); d > 5e-4 {
					diverged, rel = true, d
				}
			case intTol:
				if goVal != dosVal {
					diverged, rel = true, math.Abs(goVal-dosVal)
				}
			default:
				tol := math.Max(0.02, 5e-4*math.Abs(dosVal))
				if d := math.Abs(goVal - dosVal); d > tol {
					diverged, rel = true, d/math.Max(1, math.Abs(dosVal))
				}
			}
			if diverged {
				valDiverge++
				if rel > worstVal {
					worstVal = rel
					worstMsg = fmt.Sprintf("Go=%.6f DOS=%.6f rel=%.2e | %s", goVal, dosVal, rel, label)
				}
				if valDiverge <= 12 {
					t.Logf("DIVERGE: Go=%.6f DOS=%.6f | %s", goVal, dosVal, label)
				}
			}
		}
	}

	t.Logf("solver×options audit: %d both-solved, %d both-refused, %d flake, %d date-horizon; "+
		"Go-solves-DOS-refuses(degenerate)=%d (HARD); Go-valid-DOS-misses=%d; DOS-solves-Go-refuses=%d; "+
		"value-diverge=%d worst=%.2e [%s]",
		checked, bothRefused, flaked, dateHorizon, goSolvesDosRefuses, goValidDosMisses,
		dosSolvesGoRefuses, valDiverge, worstVal, worstMsg)
	covKeys := make([]string, 0, len(cover))
	for k := range cover {
		covKeys = append(covKeys, fmt.Sprintf("%s=%d", k, cover[k]))
	}
	t.Logf("coverage: %s", strings.Join(covKeys, " "))
	if checked+bothRefused < N/6 {
		t.Fatalf("only %d/%d adjudicated — harness/oracle problem", checked+bothRefused, N)
	}
	if valDiverge > checked/20 {
		t.Errorf("value divergence rate too high: %d of %d", valDiverge, checked)
	}
}
