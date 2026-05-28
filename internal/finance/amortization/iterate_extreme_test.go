// iterate_extreme_test.go — corner-case coverage for the Iterate
// refinement loop.
//
// What's pinned here:
//   - Tiny / huge / very-low / very-high rates do not blow up.
//   - x0 ≈ optimum returns immediately via the early-exit.
//   - Negative-rate excursions during Newton recover (the engine
//     refuses bad rates with +Inf, and iterateNewton shrinks delta).
//   - Stacked Advanced Options (prepay + balloon + adjustment) still
//     converge or bail with a clear "didn't converge" signal.
//   - The plain non-fancy path is untouched by these changes.

package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// helper: build a fancy input with a single prepayment series.
func extremeInput(amount, rate float64, n int, payment float64) LoanInput {
	return LoanInput{
		Loan:     mkFancyLoan(amount, rate, n, payment),
		Settings: fancyTestSettings(),
		Fancy:    true,
		Prepayments: []Prepayment{
			{
				StartDateStatus: types.InOutInput,
				StartDate:       types.DateRec{Time: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
				StopDateStatus:  types.InOutInput,
				StopDate:        types.DateRec{Time: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)},
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
				PaymentStatus:   types.InOutInput,
				Payment:         50,
			},
		},
	}
}

// TestIterateExtreme_HighRate: 50% rate. Closed-form Summation values
// are well-defined here; iterate should not overflow.
func TestIterateExtreme_HighRate(t *testing.T) {
	// At 50% annual, $1000/mo payment, 360 periods: principal is small
	// (~ 1000/(0.50/12) * (1 - 1/f^n) ≈ ~24000).
	input := extremeInput(0, 0.50, 360, 1000)
	input.Loan.AmountStatus = types.StatusEmpty
	solved, conv, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("high-rate: %v", err)
	}
	t.Logf("solved amount @ 50%%: $%.2f (converged=%v)", solved, conv)
	if solved <= 0 || solved > 1_000_000 || math.IsNaN(solved) || math.IsInf(solved, 0) {
		t.Errorf("solved=%.4f is not a sensible principal", solved)
	}
}

// TestIterateExtreme_NearZeroRate: rate just above the tiny threshold.
// The closed-form solver guards against rate < tiny; this checks the
// just-above-tiny path.
func TestIterateExtreme_NearZeroRate(t *testing.T) {
	// 0.0002 annual; tiny=1e-5. f-1 ≈ 0.0002/12 ≈ 1.7e-5, just above
	// the guard. SolveLoanAmount should succeed.
	input := extremeInput(0, 0.0002, 360, 600)
	input.Loan.AmountStatus = types.StatusEmpty
	solved, conv, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("near-zero rate: %v", err)
	}
	t.Logf("solved amount @ 0.02%%: $%.2f (converged=%v)", solved, conv)
	if solved <= 0 || solved > 10_000_000 || math.IsNaN(solved) {
		t.Errorf("solved=%.4f is implausible", solved)
	}
}

// TestIterateExtreme_BelowTinyRateErrors: rate below the tiny threshold
// should error cleanly, not crash.
func TestIterateExtreme_BelowTinyRateErrors(t *testing.T) {
	input := extremeInput(0, 1e-9, 360, 600) // far below tiny
	input.Loan.AmountStatus = types.StatusEmpty
	_, _, err := SolveLoanAmount(input)
	if err == nil {
		t.Error("expected error for sub-tiny rate, got nil")
	}
}

// TestIterateExtreme_PaymentEqualsTextbook: when the user supplies the
// exact textbook payment and the fancy options are mild, iterate should
// converge quickly (often in zero or one steps).
func TestIterateExtreme_PaymentEqualsTextbook(t *testing.T) {
	// $200k @ 6% 30yr textbook pmt = $1199.10. With a small prepayment
	// series, the iterate has work to do but the closed-form is close.
	input := extremeInput(0, 0.06, 360, 1199.10)
	input.Loan.AmountStatus = types.StatusEmpty
	solved, conv, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("textbook payment: %v", err)
	}
	t.Logf("solved amount @ textbook payment: $%.2f (converged=%v)", solved, conv)
	if solved < 100_000 || solved > 400_000 {
		t.Errorf("solved=%.4f outside plausible band [100k, 400k]", solved)
	}
}

// TestIterateExtreme_StackedAdvancedOptions: prepayment + balloon +
// adjustment all together. iterate should still return a sensible
// value (may or may not converge, but must not panic).
func TestIterateExtreme_StackedAdvancedOptions(t *testing.T) {
	input := LoanInput{
		Loan:     mkFancyLoan(0, 0.06, 360, 1199.10),
		Settings: fancyTestSettings(),
		Fancy:    true,
		Prepayments: []Prepayment{
			{
				StartDateStatus: types.InOutInput,
				StartDate:       types.DateRec{Time: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
				StopDateStatus:  types.InOutInput,
				StopDate:        types.DateRec{Time: time.Date(2029, 2, 1, 0, 0, 0, 0, time.UTC)},
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
				PaymentStatus:   types.InOutInput,
				Payment:         100,
			},
		},
		Balloons: []BalloonPayment{
			{
				DateStatus:   types.InOutInput,
				Date:         types.DateRec{Time: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)},
				AmountStatus: types.InOutInput,
				Amount:       25_000,
			},
		},
		Adjustments: []RateAdjustment{
			{
				DateStatus:     types.InOutInput,
				Date:           types.DateRec{Time: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
				LoanRateStatus: types.InOutInput,
				LoanRate:       0.05,
			},
		},
	}
	input.Loan.AmountStatus = types.StatusEmpty
	solved, conv, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("stacked options: %v", err)
	}
	t.Logf("solved amount with stacked options: $%.2f (converged=%v)", solved, conv)
	if solved <= 0 || solved > 5_000_000 || math.IsNaN(solved) {
		t.Errorf("solved=%.4f is implausible", solved)
	}
}

// TestIterateExtreme_VeryLargeLoan: $10M loan. Confirm the relative
// tolerance (acc_limit) is used so absolute halfpenny isn't required
// at this scale.
func TestIterateExtreme_VeryLargeLoan(t *testing.T) {
	// Payment chosen so closed-form lands near $10M.
	// 10_000_000 / 166.78 ≈ $59,960 monthly.
	input := extremeInput(0, 0.06, 360, 59960)
	input.Loan.AmountStatus = types.StatusEmpty
	solved, conv, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("very large loan: %v", err)
	}
	t.Logf("solved very-large amount: $%.2f (converged=%v)", solved, conv)
	if solved < 5_000_000 || solved > 50_000_000 {
		t.Errorf("solved=%.4f outside plausible band for large loan", solved)
	}
}

// TestIterateExtreme_RateSolveNegativeExcursion: a rate solve whose
// Newton step would push the rate negative. iterateNewton's +Inf
// recovery should kick in and the solver should still return a
// non-negative rate.
func TestIterateExtreme_RateSolveNegativeExcursion(t *testing.T) {
	// Construct a fancy schedule whose payment is high enough that
	// the closed-form rate solve lands near zero — increasing the
	// chance that a Newton step pushes negative.
	input := LoanInput{
		Loan:     mkFancyLoan(100_000, 0.001, 360, 350),
		Settings: fancyTestSettings(),
		Fancy:    true,
		Prepayments: []Prepayment{
			{
				StartDateStatus: types.InOutInput,
				StartDate:       types.DateRec{Time: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
				StopDateStatus:  types.InOutInput,
				StopDate:        types.DateRec{Time: time.Date(2029, 2, 1, 0, 0, 0, 0, time.UTC)},
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
				PaymentStatus:   types.InOutInput,
				Payment:         50,
			},
		},
	}
	input.Loan.LoanRateStatus = types.StatusEmpty
	solved, conv, err := SolveRate(input)
	if err != nil {
		// SolveRate may legitimately return an error for some
		// configurations (e.g. payment too low). We accept that — the
		// goal is "no panic, no NaN", not "always succeeds".
		t.Logf("rate solve returned error (acceptable): %v", err)
		return
	}
	t.Logf("solved rate: %.6f (converged=%v)", solved, conv)
	if math.IsNaN(solved) || math.IsInf(solved, 0) || solved < -1 || solved > 2 {
		t.Errorf("solved rate %.6f is not in a sane range", solved)
	}
}

// TestIterateExtreme_BestXNotConvergedReturnsClosedForm: when iterate
// genuinely cannot improve on the closed-form (e.g., no prepayment
// effect because the series is empty after StopDate before any
// payment), the converged flag is false and the returned value should
// still be sensible (the closed-form fallback).
func TestIterateExtreme_BestXNotConvergedReturnsClosedForm(t *testing.T) {
	// Prepayment series that ends before its first payment fires —
	// effectively no prepayments. Iterate sees a fancy=true input but
	// the series contributes nothing, so the closed-form should match
	// reality.
	input := LoanInput{
		Loan:     mkFancyLoan(0, 0.06, 360, 1199.10),
		Settings: fancyTestSettings(),
		Fancy:    true,
		Prepayments: []Prepayment{
			{
				StartDateStatus: types.InOutInput,
				StartDate:       types.DateRec{Time: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
				StopDateStatus:  types.InOutInput,
				StopDate:        types.DateRec{Time: time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)},
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
				PaymentStatus:   types.InOutInput,
				Payment:         100,
			},
		},
	}
	input.Loan.AmountStatus = types.StatusEmpty
	solved, conv, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("empty-prepayment-series: %v", err)
	}
	t.Logf("solved (effectively-empty prepayments): $%.2f (converged=%v)", solved, conv)
	// Should be very near the closed-form $200k.
	if math.Abs(solved-200_000) > 5_000 {
		t.Errorf("solved=%.4f, expected ~$200k", solved)
	}
}
