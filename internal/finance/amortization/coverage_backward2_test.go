package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_backward2_test.go drives the closed-form backward helpers
// directly to reach their internal branches: ComputeAPRWithPoints guesses
// and stub-skip, solveAdjRate clamps and degenerate denominator,
// SolveBalloonAmount immediate-zero and clamp arms, and the additive
// prepayment tiny-rate branch.

// TestComputeAPRWithPointsBranches covers the stub-row skip (backward.go:449),
// the non-positive first-guess fallback (:463), and the degenerate-denominator
// delta reset (:474) by feeding a flat single-payment schedule.
func TestComputeAPRWithPointsBranches(t *testing.T) {
	s := &Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360}
	loanDate := types.NewDateRec(2024, time.January, 1)
	// A normal multi-row schedule WITH a leading settlement-stub row (PayNum 0).
	sched := []PaymentRecord{
		{PayNum: 0, Date: loanDate, PayAmt: 50},
		{PayNum: 1, Date: types.NewDateRec(2024, time.February, 1), PayAmt: 1000},
		{PayNum: 2, Date: types.NewDateRec(2024, time.March, 1), PayAmt: 1000},
		{PayNum: 3, Date: types.NewDateRec(2024, time.April, 1), PayAmt: 1000},
	}
	// Non-positive first guess forces the vRate<=0 -> 0.1 fallback.
	apr, _, _ := ComputeAPRWithPoints(sched, loanDate, 2950, -1, 12, s)
	if apr <= 0 {
		t.Errorf("APR with negative first guess = %.4f, want a sensible positive rate", apr)
	}

	// Single regular payment: value() barely changes between trial rates, so the
	// secant denominator collapses and the delta-reset arm runs.
	flat := []PaymentRecord{
		{PayNum: 1, Date: types.NewDateRec(2024, time.February, 1), PayAmt: 1000},
	}
	ComputeAPRWithPoints(flat, loanDate, 990, 0.1, 12, s)
}

// TestSolveAdjRateClampsAndDegenerate drives solveAdjRate directly:
//   - a payment far above the amortizing one implies a strongly negative rate
//     that the secant clamps at -1.9 (backward.go:1023),
//   - a payment far below it implies a rate the secant clamps at +1.9 (:1025).
func TestSolveAdjRateClamps(t *testing.T) {
	loan := mkFancyLoan(100000, 0.06, 12, 0)

	// Overpaying payment -> implied rate negative, clamps low.
	if r, ok := solveAdjRate(100000, 50000, 12, loan, 1.0/360); ok {
		// converged values land inside the band
		if r < -2.0 || r > 2.0 {
			t.Errorf("overpay solveAdjRate r=%.4f out of [-2,2]", r)
		}
	}
	// Tiny payment -> can never retire, implied rate clamps high (no convergence).
	if r, _ := solveAdjRate(100000, 1, 12, loan, 1.0/360); r < -2.0 || r > 2.0 {
		t.Errorf("underpay solveAdjRate r=%.4f out of [-2,2]", r)
	}
}

// TestSolveBalloonAmountOverFundedNegative guards the balloon-solve fix: an
// OVER-funded loan (regular payment exceeds what the term needs) has a
// terminating balloon that solves NEGATIVE — the full-term balance
// over-amortizes past early retirement and the balloon refunds it to land the
// terminal balance on zero. The pre-fix engine discounted the TRUNCATED display
// schedule (which retires early, so a late balloon had no effect) and clamped the
// secant at ≥0, wrongly returning 0. Value verified against the DOS oracle's
// solveballoon query: −300757.72 to the cent.
func TestSolveBalloonAmountOverFundedNegative(t *testing.T) {
	loan := aw4BaseLoan()
	loan.PayAmt = 1500 // over-pays: the terminating balloon solves NEGATIVE
	in := LoanInput{
		Loan:     loan,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
		Fancy:    true,
		Balloons: []BalloonPayment{{
			DateStatus:   types.InOutInput,
			Date:         types.NewDateRec(2054, time.January, 1),
			AmountStatus: types.StatusEmpty,
		}},
	}
	got, err := SolveBalloonAmount(in, 0)
	if err != nil {
		t.Fatalf("SolveBalloonAmount: %v", err)
	}
	const want = -300757.72 // DOS oracle
	if d := got - want; d > 0.02 || d < -0.02 {
		t.Errorf("over-funded target balloon = %.4f, want %.4f (DOS oracle)", got, want)
	}
}

// TestAdditivePrepayTinyRate covers the tiny-rate (zero-interest) branch of
// prepayClosedFormGuess (backward.go:714-734): undiscounted balance split
// across the unknown prepayment count.
func TestAdditivePrepayTinyRate(t *testing.T) {
	loan := Loan{
		AmountStatus:   types.InOutInput,
		Amount:         24000,
		LoanRateStatus: types.InOutInput,
		LoanRate:       0, // zero rate -> tiny-rate branch
		PayAmtStatus:   types.InOutInput,
		PayAmt:         500,
		NStatus:        types.InOutInput,
		NPeriods:       24,
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		LoanDateStatus: types.InOutInput,
		LoanDate:       types.NewDateRec(2024, time.January, 1),
		FirstStatus:    types.InOutInput,
		FirstDate:      types.NewDateRec(2024, time.February, 1),
		LastOK:         true,
		LastDate:       types.NewDateRec(2026, time.January, 1),
	}
	in := LoanInput{
		Loan:     loan,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024, time.March, 1),
			NNStatus:        types.InOutInput,
			NN:              12,
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			// amount unknown
		}},
	}
	got, err := prepayClosedFormGuess(in, 0)
	if err != nil {
		t.Fatalf("prepayClosedFormGuess tiny-rate: %v", err)
	}
	// principal 24000, regular pays 24*500=12000, remaining 12000 over 12 extras = 1000/ea.
	if got < 900 || got > 1100 {
		t.Errorf("tiny-rate additive prepay = %.4f, want ~1000", got)
	}
}
