// Regressions for the 2026-07-12 audit PASS-3 findings (docs/discrepancies.md
// §22, AF1-AF6 forward + P3-F1..F7 backward). Every golden cites the real
// DOS-engine command that produced it.
package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// AF1: a payment dated ON an adjustment date is made BEFORE the adjustment
// applies, so the AO6 implied-rate recompute sees that payment's balance and
// one fewer remaining period.
//
//	amort_oracle 100000 0.09 120 12 payhard=1300 adj=24::1100 payoff=1.2.2024 → payoff 100449.9644
func TestPass3AF1PayoffAdjOnPaymentDate(t *testing.T) {
	in := LoanInput{Loan: pass1Loan(100000, 0.09, 1300, 120), Settings: basicSettings(), Fancy: true,
		Adjustments: []RateAdjustment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2026, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 1100, AmtOK: true}}}
	got, err := PayoffBalance(in, types.NewDateRec(2024, time.February, 1))
	if err != nil || math.Abs(got-100449.9644) > 0.05 {
		t.Errorf("payoff = %.4f err=%v, want 100449.9644 (oracle; strict-< gave 100451.4622)", got, err)
	}
	// Two AO6 rows, payoff between them (negative-implied-rate territory):
	//
	//	amort_oracle 100000 0.09 120 12 payhard=1300 adj=24::1100 adj=48::900 payoff=15.3.2028 → payoff 65537.8966
	in2 := in
	in2.Adjustments = append(append([]RateAdjustment{}, in.Adjustments...),
		RateAdjustment{DateStatus: types.InOutInput, Date: types.NewDateRec(2028, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 900, AmtOK: true})
	got2, err2 := PayoffBalance(in2, types.NewDateRec(2028, time.March, 15))
	if err2 != nil || math.Abs(got2-65537.8966) > 0.05 {
		t.Errorf("two-AO6 payoff = %.4f err=%v, want 65537.8966 (oracle)", got2, err2)
	}
}

// AF2: on the 360 basis the first-period prorate is the RAW 30/360 YearsDif
// (Amortize.pas:1286+1516) — a whole period on ordinary clean pairs but NOT on
// clamped or February month-end pairs. Also covers the prepaid-clearing rule:
// a loan taken strictly after the natural period start clears prepaid outright
// (Amortize.pas:1252-1259).
//
//	amort_oracle 50000 0.10 14 12 loandmy=31.1.2024 firstdmy=29.2.2024 rows
//	  → payment 3797.6090, row1 int 402.78 (29/360), interest 3166.53
//	amort_oracle 50000 0.10 14 12 loandmy=28.2.2025 firstdmy=28.3.2025 rows
//	  → payment 3796.5626, row1 int 388.89 (28/360), interest 3151.88
//	amort_oracle 50000 0.10 14 12 loandmy=31.1.2024 firstdmy=29.2.2024 prepaid
//	  → payment 3797.6090, interest 3166.53 (identical to non-prepaid: cleared)
//	amort_oracle 50000 0.10 14 12 loandmy=15.1.2024 firstdmy=15.2.2024
//	  → payment 3798.6555, row1 416.67 (ordinary pair control, whole month)
func TestPass3AF2Clamped360FirstPeriod(t *testing.T) {
	mk := func(ld, fd types.DateRec, prepaid bool) LoanInput {
		s := basicSettings()
		s.Prepaid = prepaid
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 50000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.10,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 14,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: ld,
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: s}
	}
	check := func(tag string, in LoanInput, wantPay, wantRow1, wantInt float64) {
		t.Helper()
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("%s: %v", tag, res.Err)
		}
		var pay, row1 float64
		for _, r := range res.Schedule {
			if r.PayNum == 1 {
				pay, row1 = r.PayAmt, r.Interest
				break
			}
		}
		if math.Abs(pay-wantPay) > 0.005 || (wantRow1 > 0 && math.Abs(row1-wantRow1) > 0.005) ||
			math.Abs(res.TotalInt-wantInt) > 0.05 {
			t.Errorf("%s: pay=%.4f row1=%.4f int=%.2f, want %.4f / %.2f / %.2f (oracle)",
				tag, pay, row1, res.TotalInt, wantPay, wantRow1, wantInt)
		}
	}
	check("clamped Jan31→Feb29", mk(types.NewDateRec(2024, time.January, 31), types.NewDateRec(2024, time.February, 29), false), 3797.6090, 402.78, 3166.53)
	check("Feb-end Feb28→Mar28", mk(types.NewDateRec(2025, time.February, 28), types.NewDateRec(2025, time.March, 28), false), 3796.5626, 388.89, 3151.88)
	check("clamped prepaid (cleared)", mk(types.NewDateRec(2024, time.January, 31), types.NewDateRec(2024, time.February, 29), true), 3797.6090, 402.78, 3166.53)
	check("ordinary pair control", mk(types.NewDateRec(2024, time.January, 15), types.NewDateRec(2024, time.February, 15), false), 3798.6555, 416.67, 3181.18)
}

// AF6: the display very-last fold retires a prepay-series loan's residual into
// the final scheduled payment exactly as for plain/balloon loans.
//
//	amort_oracle 50000 0.08 60 12 payhard=900 pre=6:12:12:200
//	  → final row int 138.15 prin 20722.07 bal 0.00, interest 15560.22, paid 65560.22
//	amort_oracle 200000 0.09 480 12 payhard=1600 pre=12:120:12:100 → paid 4258344.89
func TestPass3AF6PrepaySeriesResidualFold(t *testing.T) {
	mk := func(amount, rate, pay float64, n int, preStart types.DateRec, nn int, preAmt float64) LoanInput {
		return LoanInput{Loan: pass1Loan(amount, rate, pay, n), Settings: basicSettings(), Fancy: true,
			Prepayments: []Prepayment{{
				StartDateStatus: types.InOutInput, StartDate: preStart,
				NNStatus: types.InOutInput, NN: nn,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: preAmt,
			}}}
	}
	res := Amortize(mk(50000, 0.08, 900, 60, types.NewDateRec(2024, time.July, 1), 12, 200))
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	if math.Abs(res.TotalPaid-65560.22) > 0.05 || math.Abs(res.FinalPrinc) > 0.01 {
		t.Errorf("paid=%.2f finalBal=%.4f, want 65560.22 / 0 (oracle; unfixed left bal 19960.22)", res.TotalPaid, res.FinalPrinc)
	}
	last := res.Schedule[len(res.Schedule)-1]
	if math.Abs(last.Interest-138.15) > 0.005 {
		t.Errorf("final row int = %.4f, want 138.15 (oracle)", last.Interest)
	}
	res2 := Amortize(mk(200000, 0.09, 1600, 480, types.NewDateRec(2025, time.January, 1), 120, 100))
	if res2.Err != nil || math.Abs(res2.TotalPaid-4258344.89) > 0.05 || math.Abs(res2.FinalPrinc) > 0.01 {
		t.Errorf("neg-am: paid=%.2f finalBal=%.4f err=%v, want 4258344.89 / 0 (oracle)", res2.TotalPaid, res2.FinalPrinc, res2.Err)
	}
}
