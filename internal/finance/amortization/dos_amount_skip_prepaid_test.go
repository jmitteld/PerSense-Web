package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestAmountSolvePrepaidSkipBlind guards the DOS prepaid amount-solve shortcut
// (Amortize.pas:458), which is SKIP-BLIND: with prepaid set (the UI default)
// and no balloons/prepayments/in-advance/exact-daily, DOS returns the closed
// form and ignores skip months entirely; without prepaid it Iterates and the
// solve is skip-aware. Oracle-verified values (2026-07-24 solver-options
// audit; discrepancies §40).
func TestAmountSolvePrepaidSkipBlind(t *testing.T) {
	mk := func(prepaid bool, skip string, morMonths int) LoanInput {
		s := Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, Prepaid: prepaid, PlusRegular: true}
		in := LoanInput{
			Loan: Loan{
				LoanRateStatus: types.InOutInput, LoanRate: 0.12,
				NStatus: types.InOutInput, NPeriods: 24,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus: types.InOutInput, PayAmt: 888.4879,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 2, 1),
				AmountStatus: types.StatusEmpty,
			},
			Settings: s, Fancy: true,
		}
		if skip != "" {
			ms, _ := MonthSetFromString(skip)
			in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: skip, MonthSet: ms}
		}
		if morMonths > 0 {
			in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2024+(morMonths)/12, time.Month(1+((morMonths)%12)), 1)}
		}
		return in
	}
	cases := []struct {
		name string
		in   LoanInput
		want float64
	}{
		{"prepaid+skip → BLIND 18874.49", mk(true, "6-8", 0), 18874.492533},
		{"bare+skip → AWARE 14134.97", mk(false, "6-8", 0), 14134.974937},
		{"prepaid+mor+skip → mor-only 15305.10", mk(true, "6-8", 6), 15305.100114},
	}
	for _, c := range cases {
		v, ok, err := SolveLoanAmount(c.in)
		if err != nil || !ok {
			t.Errorf("%s: err=%v ok=%v", c.name, err, ok)
			continue
		}
		if math.Abs(v-c.want) > 0.5 {
			t.Errorf("%s: got %.4f want %.4f", c.name, v, c.want)
		} else {
			t.Logf("%s: %.4f ✓", c.name, v)
		}
	}
}
