package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// A1: DOS DaysCloseEnough month-end rule — end-of-month fancy loans accrue the
// whole-month fraction on clamped date pairs. Oracle:
// `amort_oracle 100000 0.12 24 12 loandmy=31.12.2023 firstdmy=31.1.2024 targ=0.01 rows`
// → payment 4707.3472, 2/29/24 int 962.93, 3/31/24 int 925.48.
func TestA1MonthEndDaysCloseEnough(t *testing.T) {
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.12,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 24,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2023, time.December, 31),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 31),
	}
	res := Amortize(LoanInput{Loan: loan, Settings: basicSettings(), Fancy: true,
		Target: Target{TargetStatus: types.InOutInput, TargetValue: 0.01}})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	pay := res.Schedule[1].PayAmt
	if math.Abs(pay-4707.3472) > 0.05 {
		t.Errorf("payment = %.4f, want 4707.3472 (oracle)", pay)
	}
	for _, r := range res.Schedule {
		switch r.Date.Time.Format("2006-01-02") {
		case "2024-02-29":
			if math.Abs(r.Interest-962.93) > 0.05 {
				t.Errorf("2/29/24 int = %.4f, want 962.93 (whole 30/360 month)", r.Interest)
			}
		case "2024-03-31":
			if math.Abs(r.Interest-925.48) > 0.05 {
				t.Errorf("3/31/24 int = %.4f, want 925.48", r.Interest)
			}
		}
	}
}

// A1 semimonthly: the 15th/month-end grid is whole periods under DaysCloseEnough
// with the peryr=24 half-month adjustment. Oracle: `amort_oracle 100000 0.10 48
// 24 firstdmy=15.1.2024 b12=20000 plusreg pay=2229.8065` → 2/29/24 int 393.79
// (= 100000-ish balance × 10% × 1/24), non-Feb rows already matched.
func TestA1SemimonthlyFebRow(t *testing.T) {
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.10,
		PayAmtStatus: types.InOutInput, PayAmt: 2229.8065,
		NStatus: types.InOutInput, NPeriods: 48,
		PerYrStatus: types.InOutInput, PerYr: 24,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 15),
	}
	s := basicSettings()
	s.PlusRegular = true
	s.PerYr = 24
	res := Amortize(LoanInput{Loan: loan, Settings: s, Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2025, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 20000}}})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	for _, r := range res.Schedule {
		if r.Date.Time.Format("2006-01-02") == "2024-02-29" {
			if math.Abs(r.Interest-393.79) > 0.05 {
				t.Errorf("2/29/24 int = %.4f, want 393.79 (oracle, 1/24 year)", r.Interest)
			}
			return
		}
	}
	t.Error("no 2024-02-29 row found")
}
