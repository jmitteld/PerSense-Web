package amortization

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestAmortMultiARMBalloonDOSFaithful guards the Class-C fix: a balloon combined
// with TWO OR MORE rate adjustments (ARMs). The fuzzer (TestDOSOptionCubeFuzz)
// found this family diverging by $12k–$42k in total interest.
//
// Root cause: refineAdjustmentPayment (the schedule-oracle solve that retires an
// ARM+balloon tail to zero — correct for a SINGLE adjustment, Divergence 1) is
// ill-posed with multiple ARMs: refining the first adjustment's payment while a
// later adjustment is still a pending rate-only reset makes the terminal depend
// on that second re-amortization, so the bisection lands off DOS's value. DOS
// instead uses the plain balloon-discounting annuity at EACH reset
// (Re_Amortize: netBal = balance − discounted future balloons, amortized over
// the remaining term at the new rate). Original fix gated refineAdjustmentPayment
// to len(Adjustments)==1 so multi-ARM loans used the plain per-reset annuity.
// The global-Iterate refactor (M1) then REPLACED that with solveSegmentPayment —
// DOS's til_adj segment solve over [adj → last] at the current rate, ignoring
// later adjustments — which composes for any number of ARMs and refines each
// segment exactly (not just the analytic seed).
//
// Golden (real DOS): amort_oracle 329000 0.0693 240 12 b206=22500 adj=58:0.1194: adj=180:0.0653: plusreg
//
//	→ segment payments 2483.94 / 3380.59 / 2937.74, total interest 426,265.36,
//	  retires at term.
func TestAmortMultiARMBalloonDOSFaithful(t *testing.T) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 329000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.0693,
			NStatus: types.InOutInput, NPeriods: 240,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 2, 1),
		},
		Settings:    Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Adjustments: []RateAdjustment{adjRateAt(58, 0.1194), adjRateAt(180, 0.0653)},
		Balloons:    []BalloonPayment{balloonAt(206, 22500)},
		Fancy:       true,
	}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	const dosInterest = 426265.36
	if d := math.Abs(r.TotalInt - dosInterest); d > 150 {
		t.Errorf("total interest = %.2f, want DOS %.2f (±150). ~$438,489 means the multi-ARM "+
			"re-amortization drifted (refineAdjustmentPayment ill-posed with 2+ ARMs).",
			r.TotalInt, dosInterest)
	}
	finalBal := r.Schedule[len(r.Schedule)-1].Principal
	if finalBal < -5 || finalBal > 5 {
		t.Errorf("final balance = %.2f, want ~0 (DOS retires at term)", finalBal)
	}
	// Segment payments must match DOS's per-reset plain annuity.
	seg := func(payNum int) float64 {
		for _, row := range r.Schedule {
			if row.PayNum == payNum {
				return row.PayAmt
			}
		}
		return 0
	}
	for _, c := range []struct {
		payNum int
		want   float64
	}{{30, 2483.94}, {120, 3380.59}, {200, 2937.74}} {
		if d := math.Abs(seg(c.payNum) - c.want); d > 2 {
			t.Errorf("payment at m%d = %.2f, want DOS ~%.2f (±2)", c.payNum, seg(c.payNum), c.want)
		}
	}
}
