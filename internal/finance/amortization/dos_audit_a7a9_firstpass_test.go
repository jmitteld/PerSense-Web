package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// A7: over-determined N + LastDate — DOS lets N win (Amortize.pas:220-226
// overwrites lastdate unconditionally). Oracle: `10000 0.12 24 12 pay=488
// b6=3000 lastdmy=1.1.2025` ≡ the same run without lastdmy → payment 488,
// interest 854.89, 24 rows.
func TestA7NWinsOverLastDate(t *testing.T) {
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 10000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.12,
		PayAmtStatus: types.InOutInput, PayAmt: 488,
		NStatus: types.InOutInput, NPeriods: 24,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
		LastStatus: types.InOutInput, LastDate: types.NewDateRec(2025, time.January, 1), LastOK: true,
	}
	res := Amortize(LoanInput{Loan: loan, Settings: basicSettings(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.July, 1),
			AmountStatus: types.InOutInput, Amount: 3000}}})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	// Oracle `... rows` emits exactly 18 rows (the balloon retires the loan
	// early; the pre-fix Go truncated at the stale LastDate with 12 rows).
	if len(res.Schedule) != 18 {
		t.Errorf("rows = %d, want 18 (oracle row count; N wins over the stale LastDate)", len(res.Schedule))
	}
	if math.Abs(res.TotalInt-854.89) > 0.5 {
		t.Errorf("total interest = %.2f, want ~854.89 (oracle)", res.TotalInt)
	}
}

// A9: N derived from first+last snaps via DOS NumberOfInstallments.
// Oracle: `intutil noi 2024 1 15 2025 3 12 12 on_or_before` → n 14 last 2025 2 15.
func TestA9NDerivationSnaps(t *testing.T) {
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 10000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.12,
		PayAmtStatus: types.InOutInput, PayAmt: 400,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2023, time.December, 15),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 15),
		LastStatus: types.InOutInput, LastDate: types.NewDateRec(2025, time.March, 12), LastOK: true,
	}
	if err := FirstPass(&loan); err != nil {
		t.Fatalf("FirstPass: %v", err)
	}
	if loan.NPeriods != 14 {
		t.Errorf("n = %d, want 14 (DOS NumberOfInstallments snap)", loan.NPeriods)
	}
	if got := loan.LastDate.Time.Format("2006-01-02"); got != "2025-02-15" {
		t.Errorf("snapped last = %s, want 2025-02-15", got)
	}
}
