// Pass-7 (2026-07-14) fancy APR-with-points coverage. Extends the fancy-APR
// differential (TestFancyAPRVsOracle covered only balloon / ARM / prepayment at a
// single payhard that runs through the adjustment) with:
//   - moratorium / skip-months / target APR
//   - balloon and ARM under heavy OVERPAYMENT (early payoff)
//   - a rate adjustment scheduled AFTER an overpayment-induced payoff (the fix)
//
// The DOS goldens are transcribed from amort_oracle and were confirmed STABLE
// across 12-15 runs (the APR oracle is otherwise non-deterministically flaky, so
// these are pinned rather than run live). Base loan for every case:
//
//	amort_oracle 100000 0.100000 120 12 payhard=<P>.00 pts=0.0300 <option> apr
//
// loan date 1/1/2024, first 2/1/2024, 360 basis.
//
// THE FIX (engine.go): DOS's Re_Amortize (AMORTOP.pas:1545-1569) re-solves an
// AO5/AO7 payment as `adjp := p` (the current balance) with NO clamp, so past an
// overpayment payoff — where the value walk runs on a NEGATIVE balance — it yields
// a negative (refund) payment that DOS carries through the APR value. The port had
// a DOS-absent `if netBal < 0 { netBal = 0 }` clamp that zeroed those payments, so
// an adjustment scheduled after payoff gave the wrong APR. Removing the clamp
// matches DOS to the cent.
package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func TestPass7FancyAPRWithPoints(t *testing.T) {
	d := func(y, m, dd int) types.DateRec { return types.NewDateRec(y, time.Month(m), dd) }
	base := func(payhard float64) LoanInput {
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanDateStatus: types.InOutInput, LoanDate: d(2024, 1, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.10,
			FirstStatus: types.InOutInput, FirstDate: d(2024, 2, 1),
			NStatus: types.InOutInput, NPeriods: 120, PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: payhard,
			PointsStatus: types.InOutInput, Points: 0.03,
		}, Settings: Settings{Basis: types.Basis360, PerYr: 12, PlusRegular: false, YrDays: 360, YrInv: 1.0 / 360},
			Fancy: true}
	}
	cases := []struct {
		name    string
		payhard float64
		want    float64 // DOS oracle APR (stable)
		mutate  func(*LoanInput)
	}{
		// schedule-shaping options (payhard 1500, runs through)
		{"moratorium mor=3", 1500, 0.108638, func(in *LoanInput) {
			in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: d(2024, 4, 1)}
		}},
		{"skip 4-6", 1500, 0.105973, func(in *LoanInput) {
			ms, _ := MonthSetFromString("4-6")
			in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: "4-6", MonthSet: ms}
		}},
		{"target 0.01", 1500, 0.108963, func(in *LoanInput) {
			in.Target = Target{TargetStatus: types.InOutInput, TargetValue: 0.01}
		}},
		// balloon under overpayment (b60=20000) — early payoff, but the balloon
		// (not an adjustment) is unaffected by the clamp; these pass pre- and
		// post-fix and guard the overpaying-balloon value walk.
		{"balloon b60 p1500", 1500, 0.110641, mkBalloon2029},
		{"balloon b60 p2000", 2000, 0.130902, mkBalloon2029},
		{"balloon b60 p2500", 2500, 0.083855, mkBalloon2029},
		// ARM (rate down at month 60) under overpayment — the FIX. p2000/p2100 run
		// THROUGH the adjustment (pass pre-fix); p2200+ retire BEFORE it (fail
		// pre-fix: clamped port gave 0.117611 / 0.124730 / 0.144859).
		{"armdown p2000 (thru)", 2000, 0.109808, mkArmDown2029},
		{"armdown p2200 (payoff<adj)", 2200, 0.115683, mkArmDown2029},
		{"armdown p2300 (payoff<adj)", 2300, 0.119482, mkArmDown2029},
		{"armdown p2500 (payoff<adj)", 2500, 0.129605, mkArmDown2029},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base(c.payhard)
			c.mutate(&in)
			r := Amortize(in)
			if r.Err != nil {
				t.Fatalf("Amortize: %v", r.Err)
			}
			if math.Abs(r.APR-c.want) > 5e-5 {
				t.Errorf("APR=%.6f want %.6f (Δ=%+.6f) — DOS oracle golden", r.APR, c.want, r.APR-c.want)
			}
		})
	}
}

func mkBalloon2029(in *LoanInput) {
	in.Balloons = []BalloonPayment{{
		DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1),
		AmountStatus: types.InOutInput, Amount: 20000,
	}}
}

func mkArmDown2029(in *LoanInput) {
	in.Adjustments = []RateAdjustment{{
		DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1),
		LoanRateStatus: types.InOutInput, LoanRate: 0.06,
	}}
}
