package presentvalue

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// Tests the "beyond life span" error ported from
// legacy/src/dos_source/PRESVALU.pas:873-883. Solving for the amount of
// a life-contingent lump sum divides the value back out by the survival
// probability; when the payment is dated so far out that survival is
// effectively impossible, that probability is ~0 and the divide blows
// up. DOS errors instead of returning a garbage amount — so must we.

func TestLifeSpan_BackwardSolveBeyondHorizonErrors(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1959, time.January, 1) // age 65 at as-of
	cfg := actuarialTestCfg(asOf, dob)   // life table tops out at age 120

	// Pay date in 2095 -> age ~136, well past the table's horizon, so
	// the conditional survival probability is effectively zero.
	beyond := dateOf(2095, time.January, 1)

	in := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.05},
			SumValueStatus: types.InOutInput, SumValue: 1000,
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: beyond,
			AmtStatus: types.StatusEmpty, // solve for the amount
			Act:       actuarial.Living,
		}},
		Actuarial: cfg,
	}

	res := Calculate(in)
	if res.Err == nil {
		t.Fatalf("expected a 'beyond life span' error solving an amount past the "+
			"life table horizon; got none (solved amount = %.4f)", res.LumpSums[0].Amt)
	}
	if !strings.Contains(res.Err.Error(), "beyond") {
		t.Errorf("error should mention the payment is beyond the life table horizon; got: %v", res.Err)
	}
}

// Control: the same setup with an in-horizon date solves normally
// (no error), confirming the guard is specific to the zero-probability
// case and didn't break ordinary contingent backward solves.
func TestLifeSpan_BackwardSolveWithinHorizonSucceeds(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1959, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)
	within := dateOf(2034, time.January, 1) // age ~75, well within the table

	// Forward-value a known amount, then solve it back.
	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: 0.05},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: within,
				AmtStatus: types.InOutInput, Amt: 100000,
				Act: actuarial.Living,
			}},
			Actuarial: cfg,
		}
	}
	fwd := Calculate(mk())
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}
	bwd := mk()
	bwd.LumpSums[0].AmtStatus = types.StatusEmpty
	bwd.LumpSums[0].Amt = 0
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("in-horizon backward solve should succeed, got: %v", res.Err)
	}
}
