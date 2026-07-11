package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// A6/A8 regressions: prepaid solves on an odd (3-period) first gap.
func TestA6A8PrepaidOddFirstSolves(t *testing.T) {
	set := basicSettings()
	set.Prepaid = true
	mk := func() Loan {
		return Loan{
			AmountStatus: types.InOutInput, Amount: 10000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.12,
			PayAmtStatus: types.InOutInput, PayAmt: 888.4879,
			NStatus: types.InOutInput, NPeriods: 12,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.April, 1),
		}
	}
	// A6 amount: oracle `0 0.12 12 12 noamt pay=888.4879 prepaid first=3` -> 10000.000149
	{
		loan := mk()
		loan.AmountStatus = types.StatusEmpty
		loan.Amount = 0
		amt, _, err := SolveLoanAmount(LoanInput{Loan: loan, Settings: set})
		if err != nil || math.Abs(amt-10000.000149) > 0.5 {
			t.Errorf("A6 prepaid amount = %.4f err=%v, want ~10000.00", amt, err)
		}
	}
	// A6 rate: oracle `10000 0 12 12 norate pay=888.4879 prepaid first=3` -> 0.1200000283
	{
		loan := mk()
		loan.LoanRateStatus = types.StatusEmpty
		loan.LoanRate = 0
		r, _, err := SolveRate(LoanInput{Loan: loan, Settings: set})
		if err != nil || math.Abs(r-0.12) > 2e-4 {
			t.Errorf("A6 prepaid rate = %.6f err=%v, want ~0.120000", r, err)
		}
	}
	// A8 term: oracle `10000 0.12 0 12 noterm pay=888.4879 prepaid first=3` -> n=12 last 2025-03-01
	{
		loan := mk()
		loan.NStatus = types.StatusEmpty
		loan.NPeriods = 0
		loan.LastStatus = types.StatusEmpty
		res := Amortize(LoanInput{Loan: loan, Settings: set})
		if res.Err != nil || res.NPeriods != 12 {
			t.Errorf("A8 prepaid term = %d err=%v, want 12", res.NPeriods, res.Err)
		}
		if res.LastDate.Time.Format("2006-01-02") != "2025-03-01" {
			t.Errorf("A8 last date = %s, want 2025-03-01", res.LastDate.Time.Format("2006-01-02"))
		}
	}
	// Guard: non-prepaid controls unchanged (oracle: amount 9805.825389, rate 0.0915511282, n=13)
	{
		s2 := basicSettings()
		loan := mk()
		loan.AmountStatus = types.StatusEmpty
		loan.Amount = 0
		amt, _, _ := SolveLoanAmount(LoanInput{Loan: loan, Settings: s2})
		if math.Abs(amt-9805.825389) > 0.5 {
			t.Errorf("non-prepaid amount control = %.4f, want ~9805.83", amt)
		}
		loan2 := mk()
		loan2.NStatus = types.StatusEmpty
		loan2.NPeriods = 0
		loan2.LastStatus = types.StatusEmpty
		res := Amortize(LoanInput{Loan: loan2, Settings: s2})
		if res.NPeriods != 13 {
			t.Errorf("non-prepaid term control = %d, want 13", res.NPeriods)
		}
	}
	// Guard: prepaid SHORT first (within one period — no stub, no shift):
	// oracle `10000 0.12 0 12 noterm pay=888.4879 prepaid loandmy=20.1.2024 firstdmy=1.2.2024`
	{
		loan := mk()
		loan.LoanDate = types.NewDateRec(2024, time.January, 20)
		loan.FirstDate = types.NewDateRec(2024, time.February, 1)
		loan.AmountStatus = types.StatusEmpty
		loan.Amount = 0
		amt, _, err := SolveLoanAmount(LoanInput{Loan: loan, Settings: set})
		// oracle `0 0.12 12 12 noamt pay=888.4879 prepaid loandmy=20.1.2024 firstdmy=1.2.2024`
		// -> solvedamount 10063.102109 (short first period: no stub, no shift)
		if err != nil || math.Abs(amt-10063.102109) > 0.5 {
			t.Errorf("prepaid SHORT-first amount = %.4f err=%v, want ~10063.10", amt, err)
		}
	}
}
