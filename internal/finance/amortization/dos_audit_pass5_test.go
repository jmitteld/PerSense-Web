// Regressions for the 2026-07-14 audit PASS-5 finding: P4-N7 (payment-only
// adjustment × day-count frequency initial-payment solve). Goldens cite the real
// DOS-engine (amort_oracle) command that produced them.
//
// P4-N7 background: a payment-only (implied-rate) adjustment at a day-count
// frequency (semimonthly/biweekly/weekly) left the blank REGULAR payment on the
// un-refined closed-form seed. The non-exact odd-first/in-advance refine arm
// (engine.go) was gated `len(input.Adjustments)==0`, so any adjustment skipped
// it — even though DOS's EstimateAndRefinePayment refines EVERY odd-first/
// in-advance loan and its Iterate walk strips adjustments (Re_Amortize gate,
// AMORTOP.pas:1215). The exact-daily refine arm already admitted adjustments for
// exactly this reason; the fix drops the gate on the non-exact arm to match. A
// day-count first period (first payment ON the loan date, a zero-length first
// period) is always "odd", so the refine now fires.
package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestPass5PaymentOnlyAdjustmentDayCount pins the initial (blank) regular payment
// for payment-only adjustments at day-count frequencies. `adj=M::AMOUNT` in the
// oracle is a payment change M MONTHS after the loan date (not period M).
//
//	amort_oracle 100000 0.06 72 24 adj=24::2083 → payment 1515.5786, interest 22172.35
//	amort_oracle 100000 0.06 72 24 adj=12::1800 → payment 1515.5786, interest 22489.47
//	amort_oracle 100000 0.06 78 26 adj=24::2000 → payment 1401.8410 (biweekly, 365 basis)
//	amort_oracle 100000 0.06 72 24 adj=24:0.09: → payment 1515.5786 (rate-change control)
func TestPass5PaymentOnlyAdjustmentDayCount(t *testing.T) {
	ld := types.NewDateRec(2024, time.January, 1)
	addMonths := func(m int) types.DateRec {
		return types.NewDateRec(ld.Time.Year()+m/12, time.Month(int(ld.Time.Month())+m%12), ld.Time.Day())
	}
	// Semimonthly (peryr=24, 360 basis, first payment ON the loan date).
	semi := func(adj RateAdjustment) LoanInput {
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.06,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 72,
			PerYrStatus: types.InOutInput, PerYr: 24,
			LoanDateStatus: types.InOutInput, LoanDate: ld,
			FirstStatus: types.InOutInput, FirstDate: ld,
		}, Settings: Settings{Basis: types.Basis360, PerYr: 24, YrDays: 360, YrInv: 1.0 / 360},
			Fancy: true, Adjustments: []RateAdjustment{adj}}
	}
	firstPay := func(r AmortResult) float64 {
		for _, row := range r.Schedule {
			if row.PayNum == 1 {
				return row.PayAmt
			}
		}
		return 0
	}
	payOnly := func(months int, amt float64) RateAdjustment {
		return RateAdjustment{DateStatus: types.InOutInput, Date: addMonths(months),
			AmountStatus: types.InOutInput, Amount: amt, AmtOK: true}
	}

	// Case 1: the P4-N7 golden — payment AND interest to the cent.
	r1 := Amortize(semi(payOnly(24, 2083)))
	if r1.Err != nil || math.Abs(firstPay(r1)-1515.5786) > 0.005 || math.Abs(r1.TotalInt-22172.35) > 0.05 {
		t.Errorf("N7 semimonthly adj=24::2083: pay=%.4f int=%.2f err=%v, want 1515.5786 / 22172.35 "+
			"(pre-fix pay was 1519.3676)", firstPay(r1), r1.TotalInt, r1.Err)
	}
	// Case 2: low-amount variant, adj at 12 months.
	r2 := Amortize(semi(payOnly(12, 1800)))
	if r2.Err != nil || math.Abs(firstPay(r2)-1515.5786) > 0.005 || math.Abs(r2.TotalInt-22489.47) > 0.05 {
		t.Errorf("N7 semimonthly adj=12::1800: pay=%.4f int=%.2f, want 1515.5786 / 22489.47", firstPay(r2), r2.TotalInt)
	}
	// Case 3: rate-change control — the payment must be the plain refined payment too.
	r3 := Amortize(semi(RateAdjustment{DateStatus: types.InOutInput, Date: addMonths(24),
		LoanRateStatus: types.InOutInput, LoanRate: 0.09}))
	if r3.Err != nil || math.Abs(firstPay(r3)-1515.5786) > 0.005 {
		t.Errorf("rate-change control adj=24:0.09: pay=%.4f, want 1515.5786", firstPay(r3))
	}

	// Case 4: biweekly (peryr=26, 365.25 basis, first = loan date + 14 days).
	// Payment AND interest are DOS-faithful to the cent after the pass-6 fix
	// (solveSegmentRate: the implied rate is solved over the actual-day segment
	// schedule, not the uniform balanceAfterN — the old ~$19/0.08% interest
	// residual is closed). See dos_audit_pass6_test.go.
	fdBi := types.DateRec{Time: ld.Time.AddDate(0, 0, 14)}
	bi := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.06,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 78,
		PerYrStatus: types.InOutInput, PerYr: 26,
		LoanDateStatus: types.InOutInput, LoanDate: ld,
		FirstStatus: types.InOutInput, FirstDate: fdBi,
	}, Settings: Settings{Basis: types.Basis365, PerYr: 26, YrDays: 365.25, YrInv: 1.0 / 365.25},
		Fancy: true, Adjustments: []RateAdjustment{payOnly(24, 2000)}}
	rb := Amortize(bi)
	if rb.Err != nil || math.Abs(firstPay(rb)-1401.8410) > 0.01 {
		t.Errorf("N7 biweekly adj=24::2000: pay=%.4f err=%v, want 1401.8410 (pre-fix was 1398.6254)", firstPay(rb), rb.Err)
	}
	if math.Abs(rb.TotalInt-24895.73) > 0.05 { // to the cent (DOS 24895.73)
		t.Errorf("N7 biweekly interest=%.4f, want 24895.73 (pass-6 segment-rate fix)", rb.TotalInt)
	}
}
