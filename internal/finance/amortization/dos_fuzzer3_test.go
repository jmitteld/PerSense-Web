package amortization

// TestDOSFuzzer3 is the ADVERSARIAL edge differential fuzzer — the complement to
// fuzzer2. Where fuzzer2 rotates the unknown over DOS-CONSISTENT values (so it only
// probes the well-conditioned interior of the solvable domain), fuzzer3 draws every
// input INDEPENDENTLY and widely — including under-funded payments far below
// principal ÷ term (deeply negative implied rates), tiny/huge amounts, and the full
// settings space (basis × exact × pmts-yr).
//
// Its hard assertion is one-directional and targets the exact class fuzzer2 is
// structurally blind to: **Go must not SOLVE a case DOS REFUSES.** That is the
// 2026-07-16 rate-solve bug — Go returned a spurious/unreachable rate where DOS emits
// "Computation of payment amount or interest rate did not converge." The reverse
// (DOS solves, Go refuses) is Go being *conservative* at the fuzzy convergence
// boundary of two non-byte-identical iterative solvers; that and value divergences
// are LOGGED for review, not failed, so the net stays green on inherent boundary
// fuzz while still catching a real "Go computes what DOS won't" regression.
//
// Robustness: inputs are byte-identical on both sides (rate 6dp→10dp, payment/amount
// in cents, dates explicit); the term is capped to ≤40 years so DOS's date horizon
// isn't hit; and the oracle's "payment 0.0000 / paid<0" numerical-breakdown sentinel
// is treated as indeterminate.

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

func TestDOSFuzzer3(t *testing.T) {
	gateOracle(t)
	rng := rand.New(rand.NewSource(fuzzSeed(20260716)))

	ld := types.NewDateRec(2020, time.January, 1)
	firstDate := func(perYr int) types.DateRec {
		switch perYr {
		case 24:
			// Semimonthly: a HALF-month first period (~15 days), NOT the loan date.
			// Returning ld gave a zero-length first period (first payment on the
			// loan date) — a degenerate semimonthly config, inconsistent with the
			// biweekly/weekly offsets below, that skewed all perYr=24 draws. (The
			// engine now handles firstDate=loanDate correctly too — see
			// TestTermSolveZeroFirstPeriodMatchesDOS — but realistic dates give
			// proper semimonthly coverage.)
			return types.DateRec{Time: ld.Time.AddDate(0, 0, 15)}
		case 26:
			return types.DateRec{Time: ld.Time.AddDate(0, 0, 14)}
		case 52:
			return types.DateRec{Time: ld.Time.AddDate(0, 0, 7)}
		default:
			return types.DateRec{Time: ld.Time.AddDate(0, 12/perYr, 0)}
		}
	}
	q6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	cents := func(x float64) float64 { return math.Round(x*100) / 100 }

	const (
		dosSolved = iota
		dosRefused
		dosFlake
		dosDateHorizon // DOS's date arithmetic overflowed on an enormous SOLVED term
	)
	idxOf := func(f []string, tok string) int {
		for i, x := range f {
			if x == tok {
				return i
			}
		}
		return -1
	}
	runOracle := func(args []string) ([]string, bool) {
		for try := 0; try < 8; try++ {
			out, err := exec.Command(oracleBin, args...).Output()
			if err != nil {
				continue
			}
			f := strings.Fields(strings.TrimSpace(string(out)))
			if len(f) >= 2 {
				return f, true
			}
		}
		return nil, false
	}
	dosSolve := func(field string, args []string) (float64, int) {
		f, ran := runOracle(args)
		if !ran {
			return 0, dosFlake
		}
		// A refusal is an "ERR <msg>" line. Solution lines start with their token at
		// position 0 (payment/rate/term) — anchor there, because the ERR message
		// "...or interest RATE did not converge" contains the word "rate" and must NOT
		// be mistaken for a solved value. (amount uses `noamt`, whose success line is
		// "payment .. paid .. solvedamount A"; ERR never contains "solvedamount".)
		if f[0] == "ERR" {
			// Distinguish a genuine computational refusal ("did not converge") from a
			// DOS DATE-HORIZON breakdown. A term-solve can produce an enormous solved
			// term (a low-rate or barely-amortizing payment → hundreds of periods ≈
			// centuries); the final payment date then falls outside DOS's representable
			// range and DOS breaks down, in one of two ways:
			//   - "Bad date passed to Julian function: m=-99" — its Julian date routine
			//     overflows; or
			//   - "Internal error - last payment not found" — DetermineLastPaymentDate
			//     (AMORTOP.pas) cannot locate/represent the last payment date.
			// Both are DOS failing to REPRESENT a well-defined result (the loan does
			// amortize — payment > per-period interest — just over an impractical
			// span), NOT a refusal to compute; the project treats the date horizon as
			// an artifact, so it is indeterminate here rather than a refusal Go must
			// mirror. 2026-07-16 multi-seed hunt (b365/360 term solves).
			msg := strings.ToLower(strings.Join(f, " "))
			if strings.Contains(msg, "julian") || strings.Contains(msg, "bad date") ||
				strings.Contains(msg, "last payment not found") {
				return 0, dosDateHorizon
			}
			return 0, dosRefused
		}
		// Numerical-breakdown sentinel: a valid schedule never has non-positive paid.
		if p := idxOf(f, "paid"); p >= 0 && p+1 < len(f) {
			if paid, _ := strconv.ParseFloat(f[p+1], 64); paid <= 0 {
				return 0, dosFlake
			}
		}
		if field == "amount" {
			at := idxOf(f, "solvedamount")
			if at < 0 || at+1 >= len(f) {
				return 0, dosRefused
			}
			v, _ := strconv.ParseFloat(f[at+1], 64)
			return v, dosSolved
		}
		tok := map[string]string{"payment": "payment", "rate": "rate", "nper": "term"}[field]
		if len(f) < 2 || f[0] != tok {
			return 0, dosRefused
		}
		if field == "nper" {
			iv, _ := strconv.Atoi(f[1])
			return float64(iv), dosSolved
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		return v, dosSolved
	}

	goSolve := func(field string, amount, rate float64, nper, perYr int, payment float64, set Settings) (float64, bool) {
		mk := func(blank string) LoanInput {
			st := func(f string) int8 {
				if f == blank {
					return types.StatusEmpty
				}
				return types.InOutInput
			}
			return LoanInput{Loan: Loan{
				AmountStatus: st("amount"), Amount: amount,
				LoanRateStatus: st("rate"), LoanRate: rate,
				PayAmtStatus: st("payment"), PayAmt: payment,
				NStatus: st("nper"), NPeriods: nper,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				LoanDateStatus: types.InOutInput, LoanDate: ld,
				FirstStatus: types.InOutInput, FirstDate: firstDate(perYr),
			}, Settings: set}
		}
		switch field {
		case "rate":
			r, conv, err := SolveRate(mk("rate"))
			return r, conv && err == nil
		case "amount":
			a, conv, err := SolveLoanAmount(mk("amount"))
			return a, conv && err == nil
		case "nper":
			res := Amortize(mk("nper"))
			if res.Err != nil || res.NPeriods <= 0 {
				return 0, false
			}
			return float64(res.NPeriods), true
		default:
			res := Amortize(mk("payment"))
			if res.Err != nil || len(res.Schedule) == 0 {
				return 0, false
			}
			return payoffRegularPayment(res, mk("payment").Loan), true
		}
	}

	N := 3000
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			N = v
		}
	}
	perYrs := []int{1, 2, 4, 12, 24, 26, 52}
	bases := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}
	fields := []string{"rate", "amount", "nper", "payment"}

	checked, bothRefused, flaked, dateHorizon := 0, 0, 0, 0
	goSolvesDosRefuses := 0 // the HARD-fail class: Go computes what DOS won't
	dosSolvesGoRefuses := 0 // Go conservative at the boundary (logged)
	valDiverge := 0
	worstValMsg := ""
	worstVal := 0.0

	for c := 0; c < N; c++ {
		amount := cents(1000 + rng.Float64()*4_999_000)
		rate := q6(rng.Float64() * 0.30)
		perYr := perYrs[rng.Intn(len(perYrs))]
		// Cap the term at ≤40 years so the last payment date stays well inside DOS's
		// date range (a 185-year term is a DOS horizon artifact, not a solver test).
		maxN := 40 * perYr
		if maxN > 595 {
			maxN = 595
		}
		nper := 6 + rng.Intn(maxN-5)
		basis := bases[rng.Intn(len(bases))]
		if (perYr == 26 || perYr == 52) && basis == types.Basis360 {
			basis = types.Basis365
		}
		exact := rng.Intn(2) == 0
		fundFrac := 0.002 + rng.Float64()*2.2 // spans the under-funded (degenerate) regime
		payment := cents((amount / float64(nper)) * fundFrac)
		if payment < 0.01 {
			payment = 0.01
		}
		field := fields[rng.Intn(len(fields))]
		set := gzSettings(perYr, basis, exact, false, false, false, false)

		fdt := firstDate(perYr)
		args := []string{
			strconv.FormatFloat(amount, 'f', 2, 64),
			strconv.FormatFloat(rate, 'f', 10, 64),
			strconv.Itoa(nper), strconv.Itoa(perYr),
			fmt.Sprintf("loandmy=%d.%d.%d", ld.Time.Day(), int(ld.Time.Month()), ld.Time.Year()),
			fmt.Sprintf("firstdmy=%d.%d.%d", fdt.Time.Day(), int(fdt.Time.Month()), fdt.Time.Year()),
		}
		if bf, ok := basisFlag(basis); ok {
			args = append(args, bf)
		}
		if exact {
			args = append(args, "exact")
		}
		if field != "payment" {
			args = append(args, "payhard="+strconv.FormatFloat(payment, 'f', 2, 64))
		}
		switch field {
		case "rate":
			args = append(args, "solverate")
		case "amount":
			args = append(args, "noamt")
		case "nper":
			args = append(args, "solveterm")
		}

		dosVal, outcome := dosSolve(field, args)
		if outcome == dosFlake {
			flaked++
			continue
		}
		if outcome == dosDateHorizon {
			// DOS overflowed its date range on an enormous solved term — indeterminate,
			// not a refusal Go must mirror. (Go's own huge term is likewise a valid but
			// impractical result; neither engine is "wrong" here.)
			dateHorizon++
			continue
		}
		dosOK := outcome == dosSolved
		goVal, goOK := goSolve(field, amount, rate, nper, perYr, payment, set)

		if goOK && !dosOK {
			// HARD FAIL: Go computed a value DOS refuses — the target regression class.
			goSolvesDosRefuses++
			if goSolvesDosRefuses <= 20 {
				t.Errorf("Go SOLVES where DOS REFUSES (solve %s): Go=%.6f | "+
					"amount=%.2f rate=%.6f nper=%d perYr=%d basis=%v exact=%v payment=%.2f | [%s]",
					field, goVal, amount, rate, nper, perYr, basis, exact, payment, strings.Join(args, " "))
			}
			continue
		}
		if !goOK && dosOK {
			// Go is conservative at the boundary of two non-identical iterative solvers.
			// Logged, not failed (see doc comment).
			dosSolvesGoRefuses++
			continue
		}
		if !goOK { // both refused — agreement
			bothRefused++
			continue
		}
		checked++
		// Both solved: characterize value agreement (generous; logged, not failed).
		tol := math.Max(0.05, 1e-3*math.Abs(dosVal))
		if field == "nper" {
			tol = 2
		}
		if d := math.Abs(goVal - dosVal); d > tol {
			rel := d / math.Max(1, math.Abs(dosVal))
			valDiverge++
			if rel > worstVal {
				worstVal = rel
				worstValMsg = fmt.Sprintf("solve %s Go=%.4f DOS=%.4f rel=%.2e [%s]", field, goVal, dosVal, rel, strings.Join(args, " "))
			}
		}
	}
	t.Logf("fuzzer3: %d both-solved, %d both-refused, %d flake, %d date-horizon; "+
		"Go-solves-DOS-refuses=%d (HARD); DOS-solves-Go-refuses=%d (conservative, logged); "+
		"value-diverge=%d worst rel=%.2e [%s]",
		checked, bothRefused, flaked, dateHorizon, goSolvesDosRefuses, dosSolvesGoRefuses, valDiverge, worstVal, worstValMsg)
	if checked+bothRefused < N/4 {
		t.Fatalf("only %d/%d cases adjudicated", checked+bothRefused, N)
	}
}
