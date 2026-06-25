package amortization

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestAmortARMSkipDOSFaithful guards the Class-A fix: an ARM (rate adjustment)
// combined with skipped months. The fuzzer (TestDOSOptionCubeFuzz) found this
// whole family diverging. Two distinct bugs had to be fixed together:
//
//  1. Dispatch order — `skipActive` was checked before the adjustment-strip
//     branch, so an ARM+skip loan solved its BASE payment with the adjustment
//     present (ill-posed: the re-amortization absorbs the balance, so the
//     bisection can't bracket). The strip-adjustment branch now runs first for
//     any adjustment + downstream option, recovering DOS's base payment.
//  2. Final-payment balloon — DOS keeps the SKIP-BLIND annuity after the reset
//     (it does NOT Iterate for skip the way it does for balloons), so the loan
//     negative-amortizes and DOS dumps the residual into the final scheduled
//     payment. Go now folds that residual into the last row for ARM loans.
//
// Golden (real DOS engine): amort_oracle 88000 0.0483 360 12 adj=70:0.0938: skip=6
//
//	→ base payment 505.69, re-amortized to 692.09 at the reset, final row
//	  balloons to ~63,497.88, total interest 191,266.58, retires to 0.
//
// To confirm the guard: revert the dispatch reorder and the ARM final-balloon
// branch in engine.go — total interest drops ~$46k (the loan retires smoothly
// at a too-high payment) or the loan leaves ~$63k owing.
func TestAmortARMSkipDOSFaithful(t *testing.T) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 88000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.0483,
			NStatus: types.InOutInput, NPeriods: 360,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 2, 1),
		},
		Settings:    Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Adjustments: []RateAdjustment{adjRateAt(70, 0.0938)},
		SkipMonths:  skipSetRaw("6"),
		Fancy:       true,
	}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	const dosInterest = 191266.58
	if d := math.Abs(r.TotalInt - dosInterest); d > 5 {
		t.Errorf("total interest = %.2f, want DOS %.2f (±5). A value near $145,084 means the "+
			"AO5 re-amortization went skip-AWARE (over-amortizing) instead of DOS's skip-blind "+
			"annuity + ballooned final payment.", r.TotalInt, dosInterest)
	}
	finalBal := r.Schedule[len(r.Schedule)-1].Principal
	if finalBal < -5 || finalBal > 5 {
		t.Errorf("final balance = %.2f, want ~0. ~$62,805 means the final scheduled payment "+
			"did not balloon to absorb the skip-blind residual like DOS.", finalBal)
	}
	// Base (pre-reset) payment must match DOS's 505.69 — the dispatch must strip
	// the adjustment for the base solve, not solve it ill-posed with the ARM in.
	if d := math.Abs(r.Schedule[0].PayAmt - 505.69); d > 0.5 {
		t.Errorf("base payment = %.2f, want DOS 505.69 (±0.50). A wrong base means the "+
			"adjustment-strip dispatch branch did not run before the skip branch.",
			r.Schedule[0].PayAmt)
	}
}
