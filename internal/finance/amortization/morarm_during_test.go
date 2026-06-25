package amortization

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestMorARMDuringWindowDOSFaithful guards the moratorium+ARM fix (the hardest
// corner the fuzzer surfaced). When a rate adjustment falls INSIDE the
// interest-only moratorium window, DOS does NOT recompute the payment at the
// moratorium boundary. The ARM's Re_Amortize (AMORTOP.pas:1547) already set the
// payment as the annuity over [ARM date -> last date] — ignoring that
// amortization is deferred until FirstRepay — so that payment is sized for MORE
// periods than actually amortize. The loan under-amortizes and DOS balloons the
// FINAL scheduled payment to retire it.
//
// Before the fix Go re-solved at the moratorium boundary over the shorter
// (actual) amortizing window, retiring smoothly and reporting ~$129k LESS total
// interest. The fix (engine.go armDuringMoratorium): suppress the boundary
// recompute when an ARM governs from inside the moratorium; the AO5 payment +
// the final-fold then reproduce DOS.
//
// Golden (real DOS): amort_oracle 388000 0.1058 300 12 adj=19:0.1189: mor=77
//
//	-> interest-only 3420.87 (10.58%) then 3844.43 (11.89% from m20), payment
//	   4101.29 from m77, total interest 992,776.00, final payment balloons to
//	   ~182,059 (retires to 0).
//
// To confirm the guard: drop the armDuringMoratorium gate — Go re-solves to
// 4318.91 at m77, retires smoothly, and total interest falls to ~$863,565.
func TestMorARMDuringWindowDOSFaithful(t *testing.T) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 388000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.1058,
			NStatus: types.InOutInput, NPeriods: 300,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 2, 1),
		},
		Settings:    Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Adjustments: []RateAdjustment{adjRateAt(19, 0.1189)},
		Moratorium:  Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: dateMonthsAfterLoan(77)},
		Fancy:       true,
	}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	const dosInterest = 992776.00
	if d := math.Abs(r.TotalInt - dosInterest); d > 50 {
		t.Errorf("total interest = %.2f, want DOS %.2f (±50). ~$863,565 means the moratorium "+
			"boundary re-solved the payment instead of keeping the ARM's full-term annuity.",
			r.TotalInt, dosInterest)
	}
	finalBal := r.Schedule[len(r.Schedule)-1].Principal
	if finalBal < -5 || finalBal > 5 {
		t.Errorf("final balance = %.2f, want ~0 (DOS balloons the final payment to retire)", finalBal)
	}
	seg := func(payNum int) float64 {
		for _, row := range r.Schedule {
			if row.PayNum == payNum {
				return row.PayAmt
			}
		}
		return 0
	}
	if d := math.Abs(seg(100) - 4101.29); d > 1 {
		t.Errorf("payment at m100 = %.2f, want DOS ~4101.29 (±1). 4318.91 means the boundary "+
			"recompute was not suppressed for the ARM-during-moratorium case.", seg(100))
	}
}
