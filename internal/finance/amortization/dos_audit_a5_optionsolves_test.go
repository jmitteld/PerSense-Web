package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func a5loan(amount, rate, payment float64, n int) Loan {
	return Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		PayAmtStatus: types.InOutInput, PayAmt: payment,
		NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}
}

func TestA5(t *testing.T) {
	set := basicSettings()
	// amount+skip: oracle `0 0.12 24 12 noamt pay=888.4879 skip=6-8` -> 14134.974937
	{
		loan := a5loan(0, 0.12, 888.4879, 24)
		loan.AmountStatus = types.StatusEmpty
		ms, _ := MonthSetFromString("6-8")
		amt, _, err := SolveLoanAmount(LoanInput{Loan: loan, Settings: set, Fancy: true,
			SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "6-8", MonthSet: ms}})
		if err != nil || math.Abs(amt-14134.974937) > 1.0 {
			t.Errorf("amount+skip = %.4f err=%v, want ~14134.97", amt, err)
		}
	}
	// rate+skip: oracle -> 0.0521349495
	{
		loan := a5loan(100000, 0, 733.7646, 360)
		loan.LoanRateStatus = types.StatusEmpty
		ms, _ := MonthSetFromString("6-8")
		r, _, err := SolveRate(LoanInput{Loan: loan, Settings: set, Fancy: true,
			SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "6-8", MonthSet: ms}})
		if err != nil || math.Abs(r-0.0521349495) > 5e-4 {
			t.Errorf("rate+skip = %.6f err=%v, want ~0.052135", r, err)
		}
	}
	// amount+mor: oracle -> 6066.870036
	{
		loan := a5loan(0, 0.12, 500, 24)
		loan.AmountStatus = types.StatusEmpty
		amt, _, err := SolveLoanAmount(LoanInput{Loan: loan, Settings: set, Fancy: true,
			Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2025, time.January, 1)}})
		if err != nil || math.Abs(amt-6066.870036) > 1.0 {
			t.Errorf("amount+mor = %.4f err=%v, want ~6066.87", amt, err)
		}
	}
	// rate+mor negative: oracle -> -0.6781533547
	{
		loan := a5loan(10000, 0, 500, 24)
		loan.LoanRateStatus = types.StatusEmpty
		r, _, err := SolveRate(LoanInput{Loan: loan, Settings: set, Fancy: true,
			Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2025, time.January, 1)}})
		if err != nil || math.Abs(r-(-0.6781533547)) > 5e-3 {
			t.Errorf("rate+mor = %.6f err=%v, want ~-0.678153", r, err)
		}
	}
}
