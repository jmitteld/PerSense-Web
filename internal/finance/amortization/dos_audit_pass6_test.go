// Regressions for the 2026-07-14 audit PASS-6 finding: the P4-N7 interest
// RESIDUAL — payment-only (implied-rate) adjustments at a day-count frequency on
// a non-360 basis (biweekly/weekly) had their post-adjustment SEGMENT interest
// ~0.08% off, even after pass 5 fixed the initial payment. Goldens cite the real
// DOS-engine (amort_oracle) command that produced them.
//
// ROOT CAUSE: DOS's EstimateAndRefineAdjRate (Amortize.pas:347-368) solves the
// implied rate by calling RepayFancyLoan — the ACTUAL-DAY schedule walk — and
// running Iterate (AMORTOP.pas:1415) to drive its terminal balance to zero. The
// port's solveAdjRate instead used a UNIFORM-period recurrence (balanceAfterN,
// constant GrowthPerPeriod). On the 360 basis uniform == actual, so semimonthly
// matched; on the 365 basis a biweekly period is exactly 14 days vs the uniform
// 365.25/26 = 14.05, so the implied rate drifted and the segment over-amortized
// (a $2019 final payment instead of $2000, ~$19 excess interest).
//
// FIX (fancybisect.go solveSegmentRate + engine.go AO6 dispatch): solve the
// implied rate over the REAL segment schedule — the sub-loan [adj -> last] with
// the new payment, driven through dosIterate (the faithful port of DOS's
// Iterate) over the actual-day fancy terminal — exactly as EstimateAndRefineAdjRate
// does. Engaged only on exact / day-count-non-360 (where uniform != actual); the
// 360 basis keeps the cheaper uniform solveAdjRate (identical result there).
package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestPass6PaymentOnlyAdjustmentSegmentInterest pins the FULL result (initial
// payment AND total interest) for payment-only adjustments across day-count
// frequencies. All FAIL on the pre-pass-6 engine (biweekly interest was ~24914.75
// vs 24895.73; weekly similarly) and pass on the segment-rate-solve engine.
//
//	amort_oracle 100000 0.06 72 24 adj=24::2083  → payment 1515.5786, interest 22172.35 (semimonthly, 360)
//	amort_oracle 100000 0.06 78 26 adj=24::2000  → payment 1401.8410, interest 24895.73 (biweekly, 365.25)
//	amort_oracle 100000 0.06 78 26 adj=18::1600  → payment 1401.8410, interest 17071.80 (biweekly)
//	amort_oracle 100000 0.06 156 52 adj=24::900  → payment  700.5532, interest 19657.54 (weekly, 365.25)
func TestPass6PaymentOnlyAdjustmentSegmentInterest(t *testing.T) {
	ld := types.NewDateRec(2024, time.January, 1)
	addMonths := func(m int) types.DateRec {
		return types.NewDateRec(ld.Time.Year()+m/12, time.Month(int(ld.Time.Month())+m%12), ld.Time.Day())
	}
	firstPay := func(r AmortResult) float64 {
		for _, row := range r.Schedule {
			if row.PayNum == 1 {
				return row.PayAmt
			}
		}
		return 0
	}
	type gold struct {
		name             string
		n, peryr, adjM   int
		basis            types.BasisType
		yd               float64
		adjAmt           float64
		wantPay, wantInt float64
	}
	cases := []gold{
		{"semimonthly 360", 72, 24, 24, types.Basis360, 360, 2083, 1515.5786, 22172.35},
		{"biweekly 365 @24mo", 78, 26, 24, types.Basis365, 365.25, 2000, 1401.8410, 24895.73},
		{"biweekly 365 @18mo", 78, 26, 18, types.Basis365, 365.25, 1600, 1401.8410, 17071.80},
		{"weekly 365 @24mo", 156, 52, 24, types.Basis365, 365.25, 900, 700.5532, 19657.54},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fd := ld
			if c.peryr == 26 || c.peryr == 52 {
				fd = types.DateRec{Time: ld.Time.AddDate(0, 0, 364/c.peryr)}
			}
			li := LoanInput{Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 100000,
				LoanRateStatus: types.InOutInput, LoanRate: 0.06,
				PayAmtStatus: types.StatusEmpty,
				NStatus:      types.InOutInput, NPeriods: c.n,
				PerYrStatus: types.InOutInput, PerYr: c.peryr,
				LoanDateStatus: types.InOutInput, LoanDate: ld,
				FirstStatus: types.InOutInput, FirstDate: fd,
			}, Settings: Settings{Basis: c.basis, PerYr: byte(c.peryr), YrDays: c.yd, YrInv: 1.0 / c.yd},
				Fancy: true, Adjustments: []RateAdjustment{{
					DateStatus: types.InOutInput, Date: addMonths(c.adjM),
					AmountStatus: types.InOutInput, Amount: c.adjAmt, AmtOK: true,
				}}}
			r := Amortize(li)
			if r.Err != nil {
				t.Fatalf("err %v", r.Err)
			}
			if p := firstPay(r); math.Abs(p-c.wantPay) > 0.005 {
				t.Errorf("payment=%.4f want %.4f", p, c.wantPay)
			}
			if math.Abs(r.TotalInt-c.wantInt) > 0.05 {
				t.Errorf("interest=%.4f want %.2f (|e|=%.4f) — segment implied-rate solve",
					r.TotalInt, c.wantInt, math.Abs(r.TotalInt-c.wantInt))
			}
		})
	}
}
