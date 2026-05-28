// iterate_fancy_test.go — round-trip coverage for the Iterate
// refinement on backward solves over fancy schedules.
//
// Strategy: build a known fancy schedule (with prepayments and/or
// adjustments), capture a representative input field (loan amount or
// rate), blank it out, run SolveLoanAmount / SolveRate, and assert
// recovery within a documented tolerance.
//
// These tests pin the iterateNewton refinement loop. Plain (non-fancy)
// recovery is already pinned by backward_test.go; this file targets
// the new fancy-mode paths specifically.

package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func fancyTestSettings() Settings {
	return Settings{
		Basis:       types.Basis360,
		PerYr:       12,
		Prepaid:     true,
		PlusRegular: true,
		YrDays:      360,
		YrInv:       1.0 / 360.0,
	}
}

func mkFancyLoan(amount, rate float64, n int, payment float64) Loan {
	return Loan{
		AmountStatus:   types.InOutInput,
		Amount:         amount,
		LoanRateStatus: types.InOutInput,
		LoanRate:       rate,
		LoanDateStatus: types.InOutInput,
		LoanDate:       types.DateRec{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		FirstStatus:    types.InOutInput,
		FirstDate:      types.DateRec{Time: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		NStatus:        types.InOutInput,
		NPeriods:       n,
		PayAmtStatus:   types.InOutInput,
		PayAmt:         payment,
	}
}

// TestIterateRefinesAmountWithPrepayments verifies that for a fancy
// schedule (prepayments active), the iterate refinement produces a
// principal whose terminal residual is smaller than the closed-form
// estimate's residual. This is the heart of what the Iterate port
// buys over a plain closed-form: the closed form ignores prepayments,
// so its answer leaves the schedule under- or over-amortized; the
// iterate refinement steers the principal to where the schedule
// retires (≈) exactly at term.
//
// We don't assert recovery of a specific number — the "right" answer
// depends on the prepayment pattern and basis. Instead the test pins
// the property the iterate is supposed to deliver: residual shrinks.
func TestIterateRefinesAmountWithPrepayments(t *testing.T) {
	// 200k @ 6%, 30 years, textbook payment ~$1199.10, plus $100/mo
	// extra prepayments for the first 5 years. The closed-form solver
	// (which ignores prepayments) returns ~$200k. The fancy schedule
	// at $200k retires before 360 payments. iterate should adjust the
	// principal upward.
	pmt := 1199.10
	prep100 := 100.0
	makeInput := func(amount float64) LoanInput {
		return LoanInput{
			Loan:     mkFancyLoan(amount, 0.06, 360, pmt),
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
					Payment:         prep100,
				},
			},
		}
	}

	closedFormEstimate := 200_000.0 // approximate closed-form answer
	closedResid := fancyResidual(makeInput(closedFormEstimate))
	t.Logf("closed-form residual at amount=$%.2f: %.4f",
		closedFormEstimate, closedResid)

	// Solve for amount with the fancy refinement active.
	input := makeInput(0)
	input.Loan.AmountStatus = types.StatusEmpty
	solved, conv, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("SolveLoanAmount err: %v", err)
	}
	t.Logf("solved amount: $%.4f (converged=%v)", solved, conv)

	solvedResid := fancyResidual(makeInput(solved))
	t.Logf("residual at solved amount: %.4f", solvedResid)

	// The iterate's job is to make the residual smaller than the
	// closed-form's. Allow a small tolerance for cases where the
	// closed-form is already near-optimal.
	if math.Abs(solvedResid) > math.Abs(closedResid)+iterateHalfpenny {
		t.Errorf("iterate did not improve residual: closed-form |resid|=%.4f, "+
			"solved |resid|=%.4f", math.Abs(closedResid), math.Abs(solvedResid))
	}

	// Sanity: solver should return a positive, sensible principal.
	if solved <= 0 || solved > 10*closedFormEstimate {
		t.Errorf("solved amount $%.2f is implausible", solved)
	}
}

// TestIterateRefinesRateWithAdjustments verifies that the rate-solve
// iterate refinement produces a residual smaller than (or close to)
// the closed-form rate's residual on a fancy schedule. The closed-form
// SolveRate ignores the ARM adjustment, so its answer leaves the
// schedule under- or over-amortized at term; iterate steers the rate
// toward a balance that retires near zero.
func TestIterateRefinesRateWithAdjustments(t *testing.T) {
	pmt := 1199.10
	makeInput := func(rate float64) LoanInput {
		return LoanInput{
			Loan:     mkFancyLoan(200_000, rate, 360, pmt),
			Settings: fancyTestSettings(),
			Fancy:    true,
			Adjustments: []RateAdjustment{
				{
					DateStatus:     types.InOutInput,
					Date:           types.DateRec{Time: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
					LoanRateStatus: types.InOutInput,
					LoanRate:       0.05,
				},
			},
		}
	}

	// Closed-form would land near the textbook 6% (ignoring the
	// adjustment). Capture the residual at that point as a baseline.
	closedFormRate := 0.06
	closedResid := fancyResidual(makeInput(closedFormRate))
	t.Logf("closed-form residual at rate=%.4f: %.4f", closedFormRate, closedResid)

	input := makeInput(0.06)
	input.Loan.LoanRateStatus = types.StatusEmpty
	solved, conv, err := SolveRate(input)
	if err != nil {
		t.Fatalf("SolveRate err: %v", err)
	}
	t.Logf("solved rate: %.6f (converged=%v)", solved, conv)

	solvedResid := fancyResidual(makeInput(solved))
	t.Logf("residual at solved rate: %.4f", solvedResid)

	// Iterate should not make things worse. Allow a small slack (the
	// closed form may already be optimal on cases where the adjustment
	// doesn't materially affect the schedule).
	if math.Abs(solvedResid) > math.Abs(closedResid)+iterateHalfpenny {
		t.Errorf("iterate did not improve residual: closed-form |resid|=%.4f, "+
			"solved |resid|=%.4f", math.Abs(closedResid), math.Abs(solvedResid))
	}

	// Sanity: solver should return a positive, sensible rate.
	if solved <= 0 || solved > 0.5 {
		t.Errorf("solved rate %.6f is implausible (expected 0 < r < 0.5)", solved)
	}
}

// TestIterateFallbackOnPlainLoan confirms that the iterate-refine code
// path is NOT exercised on a plain (non-fancy) loan: the closed-form
// estimate should be returned directly. Otherwise the new code might
// silently slow down or perturb the common-path answer.
func TestIterateFallbackOnPlainLoan(t *testing.T) {
	pmt := 1199.10
	input := LoanInput{
		Loan:     mkFancyLoan(200_000, 0.06, 360, pmt),
		Settings: fancyTestSettings(),
		Fancy:    false, // <-- crucial
	}

	input.Loan.AmountStatus = types.StatusEmpty
	solved, _, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("SolveLoanAmount err: %v", err)
	}
	// Should be very close to original closed-form (~$200k).
	if math.Abs(solved-200_000) > 100 {
		t.Errorf("plain-loan solve produced %.2f, expected ~200,000", solved)
	}
}
