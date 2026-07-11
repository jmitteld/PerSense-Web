package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// A2: a balloon dated after the last regular payment gets ONE off-cycle row
// (no phantom regular payments). Oracle: `amort_oracle 100000 0.08 24 12
// b30=50000` → payment 2668.85, interest 14052.47, last rows 1/1/26 then the
// 7/1/26 balloon.
func TestA2TrailingBalloonNoPhantomRows(t *testing.T) {
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.08,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 24,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}
	res := Amortize(LoanInput{Loan: loan, Settings: basicSettings(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2026, time.July, 1),
			AmountStatus: types.InOutInput, Amount: 50000}}})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	// Solved payment ~2668.85 (oracle)
	if len(res.Schedule) < 2 {
		t.Fatal("no schedule")
	}
	pay := res.Schedule[1].PayAmt
	if math.Abs(pay-2668.85) > 0.5 {
		t.Errorf("payment = %.4f, want ~2668.85 (oracle)", pay)
	}
	if math.Abs(res.TotalInt-14052.47) > 5 {
		t.Errorf("total interest = %.2f, want ~14052.47 (oracle)", res.TotalInt)
	}
	// 24 regular rows + 1 trailing balloon row = 25; no rows between 1/1/26 and 7/1/26.
	if len(res.Schedule) != 25 {
		t.Errorf("rows = %d, want 25 (24 regular + trailing balloon)", len(res.Schedule))
	}
	last := res.Schedule[len(res.Schedule)-1]
	if got := last.Date.Time.Format("2006-01-02"); got != "2026-07-01" {
		t.Errorf("last row date = %s, want 2026-07-01", got)
	}
	prev := res.Schedule[len(res.Schedule)-2]
	if got := prev.Date.Time.Format("2006-01-02"); got != "2026-01-01" {
		t.Errorf("penultimate row = %s, want 2026-01-01 (no phantom rows)", got)
	}
}

// A3: coincident balloon + target in REPLACE mode keeps DOS's `payamt − d`
// term. Oracle: `amort_oracle 100000 0.08 60 12 pay=1500 targ=800 b12=500 rows`
// → row 1/1/25: pay 403.48, int 603.48, bal 90721.58.
func TestA3CoincidentExtraTargetFloor(t *testing.T) {
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.08,
		PayAmtStatus: types.InOutInput, PayAmt: 1500,
		NStatus: types.InOutInput, NPeriods: 60,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}
	res := Amortize(LoanInput{Loan: loan, Settings: basicSettings(), Fancy: true,
		Target: Target{TargetStatus: types.InOutInput, TargetValue: 800},
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2025, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 500}}})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	found := false
	for _, r := range res.Schedule {
		if r.Date.Time.Format("2006-01-02") == "2025-01-01" {
			found = true
			if math.Abs(r.PayAmt-403.48) > 0.02 {
				t.Errorf("1/1/25 pay = %.4f, want 403.48 (oracle: 500−1500+800+603.48)", r.PayAmt)
			}
			if math.Abs(r.Interest-603.48) > 0.02 {
				t.Errorf("1/1/25 int = %.4f, want 603.48", r.Interest)
			}
			if math.Abs(r.Principal-90721.58) > 0.05 {
				t.Errorf("1/1/25 bal = %.4f, want 90721.58", r.Principal)
			}
		}
	}
	if !found {
		t.Error("no 2025-01-01 row found")
	}
}
