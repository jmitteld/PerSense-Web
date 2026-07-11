package amortization

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// A10: DOS refuses a rate solve beyond ±200%. Oracle:
// `amort_oracle 10000 0 12 12 norate pay=2500` → ERR did not converge.
func TestA10RateBrake(t *testing.T) {
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 10000,
		LoanRateStatus: types.StatusEmpty,
		PayAmtStatus:   types.InOutInput, PayAmt: 2500,
		NStatus: types.InOutInput, NPeriods: 12,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}
	_, conv, err := SolveRate(LoanInput{Loan: loan, Settings: basicSettings()})
	if err == nil || conv {
		t.Errorf("rate solve beyond ±200%% should refuse (DOS), got conv=%v err=%v", conv, err)
	}
	if err != nil && !strings.Contains(err.Error(), "did not converge") {
		t.Errorf("error should carry the DOS wording, got %q", err.Error())
	}
}

// A12: discount points add a settlement line and flow into total interest.
func TestA12PointsSettlement(t *testing.T) {
	mk := func() Loan {
		return Loan{
			AmountStatus: types.InOutInput, Amount: 10000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.12,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 12,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
			PointsStatus: types.InOutInput, Points: 0.02,
		}
	}
	// Points-only settlement: oracle `10000 0.12 12 12 pts=0.02 dumpraw` →
	// L0 1/1/24 int 200.00 bal 10000.00; totals interest 861.85.
	{
		res := Amortize(LoanInput{Loan: mk(), Settings: basicSettings()})
		if res.Err != nil {
			t.Fatalf("err: %v", res.Err)
		}
		r0 := res.Schedule[0]
		if r0.PayNum != 0 || math.Abs(r0.Interest-200.00) > 0.01 ||
			r0.Date.Time.Format("2006-01-02") != "2024-01-01" {
			t.Errorf("row0 = #%d %s int %.2f, want #0 2024-01-01 int 200.00", r0.PayNum, r0.Date.Time.Format("2006-01-02"), r0.Interest)
		}
		if math.Abs(res.TotalInt-861.85) > 0.05 {
			t.Errorf("total interest = %.2f, want 861.85 (oracle: 661.85 + 200 points)", res.TotalInt)
		}
	}
	// Combined prepaid stub + points: oracle `10000 0.12 12 12 pts=0.02 prepaid
	// loandmy=15.1.2024 firstdmy=1.3.2024 dumpraw` → L0 1/15/24 int 253.33 (ONE row).
	{
		loan := mk()
		loan.LoanDate = types.NewDateRec(2024, time.January, 15)
		loan.FirstDate = types.NewDateRec(2024, time.March, 1)
		s := basicSettings()
		s.Prepaid = true
		res := Amortize(LoanInput{Loan: loan, Settings: s})
		if res.Err != nil {
			t.Fatalf("err: %v", res.Err)
		}
		r0 := res.Schedule[0]
		if math.Abs(r0.Interest-253.33) > 0.02 {
			t.Errorf("combined stub row0 int = %.4f, want 253.33 (53.33 stub + 200 points)", r0.Interest)
		}
		if len(res.Schedule) > 1 && res.Schedule[1].PayNum == 0 {
			t.Error("stub and points must combine into ONE settlement row (DOS)")
		}
	}
	// Fancy path (balloon): oracle `10000 0.12 12 12 pts=0.02 b6=2000 pay=730
	// dumpraw` → L0 1/1/24 200.00.
	{
		loan := mk()
		loan.PayAmtStatus = types.InOutInput
		loan.PayAmt = 730
		res := Amortize(LoanInput{Loan: loan, Settings: basicSettings(), Fancy: true,
			Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.July, 1),
				AmountStatus: types.InOutInput, Amount: 2000}}})
		if res.Err != nil {
			t.Fatalf("err: %v", res.Err)
		}
		r0 := res.Schedule[0]
		if math.Abs(r0.Interest-200.00) > 0.01 {
			t.Errorf("fancy row0 int = %.2f, want 200.00", r0.Interest)
		}
	}
}
