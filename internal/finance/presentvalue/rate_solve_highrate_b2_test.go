package presentvalue

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Regression guard for audit B2 (discrepancies.md §30): the PV-8 rate solver
// restarts once from 0 but the earlier port RESET the iteration count each pass,
// capping the second pass at 30 damped ±0.04 steps — so it failed to converge on
// true rates above ~120%, where DOS (which byte-wraps the count for a ~256-step
// second pass, PRESVALU.pas:707) succeeds. Confirmed vs the DOS oracle.
func TestRateSolveHighRate_B2(t *testing.T) {
	amount := 1_000_000.0
	asof := types.NewDateRec(2024, 1, 1)
	pay := types.NewDateRec(2025, 1, 1)
	s := PVSettings{Basis: types.Basis360, PerYr: 12, COLAMonth: types.COLAAnnual, YrDays: 360, YrInv: 1.0 / 360}

	for _, r := range []float64{0.10, 1.0, 1.5, 2.0, 3.0, 4.0} {
		sv := amount * math.Exp(-r)
		in := &PVInput{
			Settings: s,
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: pay,
				AmtStatus: types.InOutInput, Amt: amount,
			}},
			PresVal: PresValLine{
				AsOfStatus:     types.InOutInput,
				AsOf:           asof,
				SumValueStatus: types.InOutInput,
				SumValue:       sv,
			},
		}
		var res PVResult
		solveRate(in, &res)
		if res.Err != nil {
			t.Errorf("rate %.2f: solve failed (%v) — did not converge", r, res.Err)
			continue
		}
		if got := in.PresVal.R.Rate; math.Abs(got-r) > 0.02 {
			t.Errorf("rate %.2f: solved %.4f, expected ~%.2f", r, got, r)
		}
	}
}
