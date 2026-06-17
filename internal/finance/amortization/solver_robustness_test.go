package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Backward-solver robustness tests.
//
// The fancy backward solvers (SolveLoanAmount, SolveRate) are
// documented best-effort: when prepayments/adjustments are present they
// refine a closed-form estimate via a finite-difference Newton loop
// against the schedule engine. A previously-found bug let that loop run
// away to ~$11.3M when the residual went insensitive to the variable.
// These tests sweep a matrix of option stacks and assert the results
// stay finite, positive and within a sane band — so any future
// divergence (NaN, Inf, or an absurd magnitude) is caught regardless of
// which option combination triggers it. For plain (non-fancy) loans we
// additionally assert exact round-trip recovery.

// plainLoanSolveAmount builds a plain loan with Amount left blank.
func plainLoanSolveAmount(rate float64, n int, payment float64) LoanInput {
	loan := mkFancyLoan(0, rate, n, payment)
	loan.AmountStatus = types.StatusEmpty
	return LoanInput{Loan: loan, Settings: fancyTestSettings()}
}

// TestSolverRobustness_PlainRoundTripGrid solves the payment for a grid
// of plain loans, then solves the amount and rate back and checks
// recovery. Plain loans have a closed form, so recovery should be tight.
func TestSolverRobustness_PlainRoundTripGrid(t *testing.T) {
	rates := []float64{0.03, 0.06, 0.085, 0.12}
	terms := []int{12, 60, 120, 360}
	amounts := []float64{10_000, 200_000, 1_000_000}

	for _, r := range rates {
		for _, n := range terms {
			for _, amt := range amounts {
				// Forward: solve the payment for this amount.
				fwd := LoanInput{Loan: mkFancyLoan(amt, r, n, 0), Settings: fancyTestSettings()}
				fwd.Loan.PayAmtStatus = types.StatusEmpty
				d, err := SolvePayment(fwd)
				if err != nil {
					t.Errorf("SolvePayment(amt=%.0f r=%.3f n=%d): %v", amt, r, n, err)
					continue
				}
				if d <= 0 || math.IsNaN(d) || math.IsInf(d, 0) {
					t.Errorf("SolvePayment(amt=%.0f r=%.3f n=%d) = %.4f, implausible", amt, r, n, d)
					continue
				}

				// Backward 1: recover the amount from the payment.
				ai := plainLoanSolveAmount(r, n, d)
				gotAmt, conv, err := SolveLoanAmount(ai)
				if err != nil {
					t.Errorf("SolveLoanAmount(r=%.3f n=%d d=%.4f): %v", r, n, d, err)
					continue
				}
				if !conv {
					t.Errorf("SolveLoanAmount(r=%.3f n=%d) plain loan did not converge", r, n)
				}
				if !helpClose(gotAmt, amt, 0.5, 1e-4) {
					t.Errorf("amount round-trip: got %.4f want %.4f (r=%.3f n=%d)", gotAmt, amt, r, n)
				}

				// Backward 2: recover the rate from amount+payment+term.
				ri := LoanInput{Loan: mkFancyLoan(amt, r, n, d), Settings: fancyTestSettings()}
				ri.Loan.LoanRateStatus = types.StatusEmpty
				gotRate, _, err := SolveRate(ri)
				if err != nil {
					t.Errorf("SolveRate(amt=%.0f n=%d d=%.4f): %v", amt, n, d, err)
					continue
				}
				if !helpClose(gotRate, r, 1e-4, 1e-3) {
					t.Errorf("rate round-trip: got %.6f want %.6f (amt=%.0f n=%d)", gotRate, r, amt, n)
				}
			}
		}
	}
}

// optionStack describes one advanced-option configuration to fuzz the
// solvers against.
type optionStack struct {
	name       string
	prepay     bool
	balloon    bool
	adjustUp   bool
	adjustDown bool
	skip       bool
}

func buildStack(s optionStack, solveFor string) LoanInput {
	// 30-year, $1199.10/mo @ 6% reference loan; Amount or LoanRate
	// blanked depending on what we solve for.
	loan := mkFancyLoan(200_000, 0.06, 360, 1199.10)
	in := LoanInput{Loan: loan, Settings: fancyTestSettings(), Fancy: true}

	if s.prepay {
		in.Prepayments = []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       newDate(2024, time.February, 1),
			StopDateStatus:  types.InOutInput,
			StopDate:        newDate(2029, time.February, 1),
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         100,
		}}
	}
	if s.balloon {
		in.Balloons = []BalloonPayment{{
			DateStatus:   types.InOutInput,
			Date:         newDate(2030, time.January, 1),
			AmountStatus: types.InOutInput,
			Amount:       25_000,
		}}
	}
	if s.adjustUp {
		in.Adjustments = append(in.Adjustments, RateAdjustment{
			DateStatus:     types.InOutInput,
			Date:           newDate(2027, time.January, 1),
			LoanRateStatus: types.InOutInput,
			LoanRate:       0.08,
		})
	}
	if s.adjustDown {
		in.Adjustments = append(in.Adjustments, RateAdjustment{
			DateStatus:     types.InOutInput,
			Date:           newDate(2028, time.January, 1),
			LoanRateStatus: types.InOutInput,
			LoanRate:       0.04,
		})
	}
	if s.skip {
		ms, _ := MonthSetFromString("7-8")
		in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: "7-8", MonthSet: ms}
	}

	switch solveFor {
	case "amount":
		in.Loan.AmountStatus = types.StatusEmpty
	case "rate":
		in.Loan.LoanRateStatus = types.StatusEmpty
	}
	return in
}

func allStacks() []optionStack {
	return []optionStack{
		{name: "prepay", prepay: true},
		{name: "balloon", balloon: true},
		{name: "adjust-up", adjustUp: true},
		{name: "adjust-down", adjustDown: true},
		{name: "skip", skip: true},
		{name: "prepay+balloon", prepay: true, balloon: true},
		{name: "prepay+adjust", prepay: true, adjustDown: true},
		{name: "balloon+adjust", balloon: true, adjustUp: true},
		{name: "prepay+balloon+adjust", prepay: true, balloon: true, adjustDown: true},
		{name: "all", prepay: true, balloon: true, adjustUp: true, adjustDown: true, skip: true},
	}
}

// TestSolverRobustness_AmountNeverDiverges is the direct regression net
// for the $11.3M runaway: across every option stack, SolveLoanAmount
// must return a finite, positive principal within a sane band. The
// reference loan is ~$200k, so anything above $5M (the original bug
// returned $11.3M) or non-positive is a failure.
func TestSolverRobustness_AmountNeverDiverges(t *testing.T) {
	for _, s := range allStacks() {
		in := buildStack(s, "amount")
		got, conv, err := SolveLoanAmount(in)
		if err != nil {
			t.Errorf("[%s] SolveLoanAmount errored: %v", s.name, err)
			continue
		}
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("[%s] SolveLoanAmount = %v (NaN/Inf)", s.name, got)
			continue
		}
		if got <= 0 {
			t.Errorf("[%s] SolveLoanAmount = %.2f (non-positive)", s.name, got)
		}
		if got > 5_000_000 {
			t.Errorf("[%s] SolveLoanAmount = %.2f exceeds sane band (regression: solver diverged)", s.name, got)
		}
		// The closed-form estimate for this loan is ~$200k; even with
		// options the answer should stay within an order of magnitude.
		if got < 50_000 || got > 1_000_000 {
			t.Logf("[%s] SolveLoanAmount = %.2f (converged=%v) — outside tight band but finite; review",
				s.name, got, conv)
		}
	}
}

// TestSolverRobustness_RateNeverDiverges is the symmetric net for
// SolveRate: across every stack, the solved rate must be finite and in
// a plausible (0, 1] band (0%–100% annual).
func TestSolverRobustness_RateNeverDiverges(t *testing.T) {
	for _, s := range allStacks() {
		in := buildStack(s, "rate")
		// Give a concrete amount so the rate solve is well-posed.
		in.Loan.Amount = 200_000
		in.Loan.AmountStatus = types.InOutInput
		got, conv, err := SolveRate(in)
		if err != nil {
			t.Errorf("[%s] SolveRate errored: %v", s.name, err)
			continue
		}
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("[%s] SolveRate = %v (NaN/Inf)", s.name, got)
			continue
		}
		if got <= 0 || got > 1.0 {
			t.Errorf("[%s] SolveRate = %.6f outside plausible (0,1] band", s.name, got)
		}
		t.Logf("[%s] SolveRate = %.6f (converged=%v)", s.name, got, conv)
	}
}

// TestSolverRobustness_SolvePaymentResidual checks the closed-form
// payment solve: feeding the solved payment back through RepayLoan
// should leave a terminal balance near zero for plain loans.
func TestSolverRobustness_SolvePaymentResidual(t *testing.T) {
	cases := []struct {
		amt  float64
		rate float64
		n    int
	}{
		{200_000, 0.06, 360},
		{50_000, 0.09, 60},
		{1_000_000, 0.045, 240},
	}
	for _, c := range cases {
		in := LoanInput{Loan: mkFancyLoan(c.amt, c.rate, c.n, 0), Settings: fancyTestSettings()}
		in.Loan.PayAmtStatus = types.StatusEmpty
		d, err := SolvePayment(in)
		if err != nil {
			t.Errorf("SolvePayment(amt=%.0f r=%.3f n=%d): %v", c.amt, c.rate, c.n, err)
			continue
		}
		loan := mkFancyLoan(c.amt, c.rate, c.n, d)
		settings := fancyTestSettings()
		residual := RepayLoan(c.amt, d, &loan, &settings, settings.YrInv)
		// Terminal balance should be within a few cents of zero relative
		// to the loan size.
		if math.Abs(residual) > math.Max(0.05, 1e-6*c.amt) {
			t.Errorf("SolvePayment residual too large: amt=%.0f r=%.3f n=%d d=%.4f residual=%.4f",
				c.amt, c.rate, c.n, d, residual)
		}
	}
}
