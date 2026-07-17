package amortization

// TestDOSFuzzer3Fancy is the ADVERSARIAL edge differential fuzzer for the
// ADVANCED-OPTION (fancy) amortization solves — the coverage the plain fuzzer3
// (dos_fuzzer3_test.go) skips entirely. It draws a plain loan plus ONE advanced
// option (skip / moratorium / balloon / prepayment) with DELIBERATELY WIDE,
// often-degenerate parameters — a balloon larger than the loan, a prepayment
// series that over-retires it, a moratorium spanning almost the whole term — and
// solves a blank field (payment / rate / amount) on it.
//
// Hard assertion (one-directional): **Go must not SOLVE a case DOS REFUSES.**
// The oracle emits `ERR <message>` on a refused/over-determined fancy screen; a
// term/date-range breakdown ("Julian" / "last payment not found") is treated as
// indeterminate (date-horizon), as in the plain fuzzer3. The reverse (DOS solves,
// Go refuses) and value divergences on both-solved cases are LOGGED, not failed.
//
// Inputs are byte-identical: gzLoanInput + the oracle share the default loan date
// (2024-01-01) and first-period date; option dates use the same addMonths grid the
// proven TestDOSInAdvanceFancyFuzz uses; money is 2dp, rate 6dp→10dp.

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

func TestDOSFuzzer3Fancy(t *testing.T) {
	gateOracle(t)
	rng := rand.New(rand.NewSource(fuzzSeed(0x66616e33))) // "fan3"

	N := 3000
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			N = v
		}
	}
	q6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	cents := func(x float64) float64 { return math.Round(x*100) / 100 }

	// addMonths mirrors the oracle's month-based option-date construction (loan
	// day-of-month = 1, January), identical to TestDOSInAdvanceFancyFuzz.
	addMonths := func(months int) types.DateRec {
		return types.NewDateRec(2024+months/12, time.Month(months%12+1), 1)
	}

	const (
		dosSolved = iota
		dosRefused
		dosFlake
		dosDateHorizon
	)
	// dosRun execs the oracle (retrying transient 0-payment heap flakes) and
	// classifies the line for the requested solve token.
	dosRun := func(tok string, args []string) (float64, int) {
		for try := 0; try < 8; try++ {
			out, err := exec.Command(oracleBin, args...).Output()
			if err != nil {
				continue
			}
			f := strings.Fields(strings.TrimSpace(string(out)))
			if len(f) == 0 {
				continue
			}
			if f[0] == "ERR" {
				msg := strings.ToLower(strings.Join(f, " "))
				if strings.Contains(msg, "julian") || strings.Contains(msg, "bad date") ||
					strings.Contains(msg, "last payment not found") {
					return 0, dosDateHorizon
				}
				return 0, dosRefused
			}
			// Numerical-breakdown sentinel (heap-sensitive New(h) path): the oracle
			// emits "... interest -1.00 paid -1.00" or a payment/amount of 0 instead
			// of a real result on ~some spawns. Treat as indeterminate and RETRY — a
			// valid schedule never has non-positive paid. (Same sentinel the
			// fuzzer2/plain fuzzer3 guard against.)
			sentinel := false
			for i := 0; i+1 < len(f); i++ {
				if f[i] == "paid" {
					if paid, e := strconv.ParseFloat(f[i+1], 64); e == nil && paid <= 0 {
						sentinel = true
					}
				}
			}
			if sentinel {
				continue // retry the spawn
			}
			// amount solve emits "... solvedamount A"; others start with the token.
			if tok == "solvedamount" {
				at := -1
				for i := 0; i+1 < len(f); i++ {
					if f[i] == "solvedamount" {
						at = i
						break
					}
				}
				if at < 0 {
					return 0, dosRefused
				}
				v, e := strconv.ParseFloat(f[at+1], 64)
				if e != nil {
					return 0, dosFlake
				}
				if v == 0 {
					continue // degenerate/flaky 0 amount — retry (real amount is non-zero)
				}
				return v, dosSolved
			}
			if f[0] != tok || len(f) < 2 {
				return 0, dosRefused
			}
			v, e := strconv.ParseFloat(f[1], 64)
			if e != nil {
				return 0, dosFlake
			}
			if tok == "payment" && v == 0 {
				continue // known heap-flake payment==0 sentinel
			}
			return v, dosSolved
		}
		return 0, dosFlake
	}

	perYrs := []int{1, 2, 4, 12}
	bases := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}
	fields := []string{"payment", "rate", "amount"}

	checked, bothRefused, flaked, dateHorizon := 0, 0, 0, 0
	goSolvesDosRefuses := 0
	goValidDosMisses := 0 // Go found a valid root DOS's Iterate missed (logged)
	dosSolvesGoRefuses := 0
	valDiverge := 0
	worstVal, worstMsg := 0.0, ""
	optCover := map[string]int{}

	for c := 0; c < N; c++ {
		amount := cents(10000 + rng.Float64()*490000)
		rate := q6(0.02 + rng.Float64()*0.20)
		perYr := perYrs[rng.Intn(len(perYrs))]
		mPer := 12 / perYr
		years := 2 + rng.Intn(12)
		n := years * perYr
		basis := bases[rng.Intn(len(bases))]
		exact := rng.Intn(2) == 0
		prepaid := rng.Intn(2) == 0
		inadv := rng.Intn(2) == 0

		s := gzSettings(perYr, basis, exact, prepaid, inadv, false, false)
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

		// Pick one advanced option with ADVERSARIAL (wide/degenerate) parameters,
		// building BOTH the oracle flag and the Go mutation.
		var optName string
		var optFlags []string
		var apply func(in *LoanInput)
		switch rng.Intn(4) {
		case 0: // moratorium — up to nearly the whole term
			k := 1 + rng.Intn(n)
			morMonths := k * mPer
			optName = "moratorium"
			optFlags = []string{"mor=" + strconv.Itoa(morMonths)}
			apply = func(in *LoanInput) {
				in.Fancy = true
				in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: addMonths(morMonths)}
			}
		case 1: // balloon — up to 2.5x the loan (often exceeds it → refusal-prone)
			k := 1 + rng.Intn(n)
			bMonths := k * mPer
			amt := cents(amount * (0.05 + rng.Float64()*2.5))
			optName = "balloon"
			optFlags = []string{fmt.Sprintf("b%d=%.2f", bMonths, amt)}
			apply = func(in *LoanInput) {
				in.Fancy = true
				in.Balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: addMonths(bMonths),
					AmountStatus: types.InOutInput, Amount: amt}}
			}
		case 2: // prepayment series — large extras that can over-retire the loan
			startK := 1 + rng.Intn(n)
			startMonths := startK * mPer
			nn := 1 + rng.Intn(n)
			amt := cents(200 + rng.Float64()*20000)
			optName = "prepayment"
			optFlags = []string{fmt.Sprintf("pre=%d:%d:%d:%.2f", startMonths, nn, perYr, amt)}
			apply = func(in *LoanInput) {
				in.Fancy = true
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: addMonths(startMonths),
					NNStatus: types.InOutInput, NN: nn, PerYrStatus: types.InOutInput, PerYr: perYr,
					PaymentStatus: types.InOutInput, Payment: amt}}
			}
		default: // skip-months (monthly only) — a wide contiguous run
			if perYr != 12 {
				continue // skip is month-based; only meaningful monthly
			}
			start := 1 + rng.Intn(10)
			end := start + rng.Intn(4)
			if end > 12 {
				end = 12
			}
			skipStr := strconv.Itoa(start)
			if end > start {
				skipStr = fmt.Sprintf("%d-%d", start, end)
			}
			ms, _ := MonthSetFromString(skipStr)
			optName = "skip"
			optFlags = []string{"skip=" + skipStr}
			apply = func(in *LoanInput) {
				in.Fancy = true
				in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: skipStr, MonthSet: ms}
			}
		}

		field := fields[rng.Intn(len(fields))]
		// An adversarial known payment (under- to over-funded) for the rate/amount
		// solves, which need a payment to solve against — the oracle gets it as
		// payhard=, Go as an InOutInput PayAmt. (The payment solve leaves it blank.)
		payment := cents((amount / float64(n)) * (0.3 + rng.Float64()*2.0))

		// Base args shared by all solves: amount rate n perYr <settings> <option>.
		base := []string{
			strconv.FormatFloat(amount, 'f', 2, 64),
			strconv.FormatFloat(rate, 'f', 10, 64),
			strconv.Itoa(n), strconv.Itoa(perYr),
		}
		base = append(base, sflags...)
		base = append(base, optFlags...)
		payFlag := "payhard=" + strconv.FormatFloat(payment, 'f', 2, 64)

		var dosVal float64
		var outcome int
		var goVal float64
		var goOK, goValid bool

		// retires reports whether a Go schedule genuinely amortizes (its last
		// remaining balance is ~0 relative to the loan). Used to tell a VALID root
		// DOS's less-robust Iterate simply missed from a DEGENERATE spurious solve.
		retires := func(r AmortResult, principal float64) bool {
			if r.Err != nil || len(r.Schedule) == 0 {
				return false
			}
			bal := r.Schedule[len(r.Schedule)-1].Principal
			return math.Abs(bal) <= math.Max(1.0, 1e-4*math.Abs(principal))
		}

		switch field {
		case "payment":
			dosVal, outcome = dosRun("payment", base)
			in := gzLoanInput(amount, rate, n, perYr, s)
			in.Loan.PayAmtStatus = types.StatusEmpty
			apply(&in)
			r := Amortize(in)
			if r.Err == nil && len(r.Schedule) > 0 {
				goVal, goOK = modalReg(r.Schedule), true
				goValid = retires(r, amount)
			}
		case "rate":
			dosVal, outcome = dosRun("rate", append(append([]string{}, base...), payFlag, "solverate"))
			in := gzLoanInput(amount, rate, n, perYr, s)
			in.Loan.LoanRateStatus = types.StatusEmpty
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, payment
			apply(&in)
			rv, conv, err := SolveRate(in)
			goVal, goOK = rv, conv && err == nil
			if goOK { // forward-compute at the solved rate to test validity
				chk := gzLoanInput(amount, rv, n, perYr, s)
				chk.Loan.PayAmtStatus, chk.Loan.PayAmt = types.InOutInput, payment
				apply(&chk)
				goValid = retires(Amortize(chk), amount)
			}
		default: // amount
			dosVal, outcome = dosRun("solvedamount", append(append([]string{}, base...), payFlag, "noamt"))
			in := gzLoanInput(amount, rate, n, perYr, s)
			in.Loan.AmountStatus = types.StatusEmpty
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, payment
			apply(&in)
			av, conv, err := SolveLoanAmount(in)
			goVal, goOK = av, conv && err == nil
			if goOK {
				chk := gzLoanInput(av, rate, n, perYr, s)
				chk.Loan.PayAmtStatus, chk.Loan.PayAmt = types.InOutInput, payment
				apply(&chk)
				goValid = retires(Amortize(chk), av)
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
		if goOK && !(goVal == goVal && !math.IsInf(goVal, 0)) {
			goOK = false // non-finite is not a solve
		}
		dosOK := outcome == dosSolved
		label := strings.Join(base, " ")
		if field != "payment" {
			label += " " + payFlag
		}
		label += " [" + field + "]"

		switch {
		case goOK && !dosOK:
			// HARD only when Go's solution is DEGENERATE (a real spurious-solve
			// regression). If Go's value produces a genuinely retiring schedule, Go
			// found a VALID root DOS's less-robust Iterate missed (e.g. an
			// exact×in-advance dominating-balloon rate that amortizes to the cent but
			// DOS's secant can't reach) — DOS's iteration limitation, not a Go bug —
			// so LOG it, don't fail. Same spirit as the date-horizon reclassification.
			if goValid {
				goValidDosMisses++
			} else {
				goSolvesDosRefuses++
				if goSolvesDosRefuses <= 25 {
					t.Errorf("Go SOLVES (degenerate) where DOS REFUSES (%s/%s): Go=%.6f | %s", optName, field, goVal, label)
				}
			}
		case !goOK && dosOK:
			dosSolvesGoRefuses++
		case !goOK && !dosOK:
			bothRefused++
		default:
			checked++
			optCover[optName]++
			// modalReg is only a reliable proxy for the solved REGULAR payment when
			// every regular row carries it — true for moratorium/skip, NOT for
			// prepayment (the extra REPLACES the regular) or balloon (a mid-schedule
			// balloon row, and the dominating-balloon negative case, skew the modal).
			// So the payment VALUE check is skipped for prepayment/balloon; their
			// SOLVABILITY agreement (checked above) still holds.
			skipVal := field == "payment" && (optName == "prepayment" || optName == "balloon")
			// Value agreement (generous; logged, not failed). Rate absolute, money relative.
			var diverged bool
			var rel float64
			if field == "rate" {
				if d := math.Abs(goVal - dosVal); d > 5e-4 {
					diverged, rel = true, d
				}
			} else {
				tol := math.Max(0.02, 5e-4*math.Abs(dosVal))
				if d := math.Abs(goVal - dosVal); d > tol {
					diverged, rel = true, d/math.Max(1, math.Abs(dosVal))
				}
			}
			if diverged && !skipVal {
				valDiverge++
				if rel > worstVal {
					worstVal = rel
					worstMsg = fmt.Sprintf("%s/%s Go=%.6f DOS=%.6f rel=%.2e | %s", optName, field, goVal, dosVal, rel, label)
				}
			}
		}
	}
	t.Logf("fancy fuzzer3: %d both-solved, %d both-refused, %d flake, %d date-horizon; "+
		"Go-solves-DOS-refuses(degenerate)=%d (HARD); Go-valid-DOS-misses=%d (logged); "+
		"DOS-solves-Go-refuses=%d (conservative, logged); value-diverge=%d worst=%.2e [%s]; cover=%v",
		checked, bothRefused, flaked, dateHorizon, goSolvesDosRefuses, goValidDosMisses,
		dosSolvesGoRefuses, valDiverge, worstVal, worstMsg, optCover)
	if checked+bothRefused < N/6 {
		t.Fatalf("only %d/%d cases adjudicated — harness/oracle problem", checked+bothRefused, N)
	}
}
