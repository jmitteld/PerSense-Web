package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestPODOnlyForward verifies that a worksheet with NO lump sums and NO
// periodic payments but a Payment-on-Death still produces a present
// value. DOS FrontwardCalc (PRESVALU.pas:669-691) sums the (empty) row
// loops and then adds PodValue(asof, r.rate) unconditionally when
// fold_in_life is set, so a POD-only screen is a valid forward
// calculation whose total is exactly the POD's actuarial present value.
//
// This is the engine-side guarantee behind the frontend fix that lets
// calcPV submit a row-less worksheet when the POD field is populated.
func TestPODOnlyForward(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1959, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)
	cfg.POD = 20000
	rate := 0.06

	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: rate},
		},
		Actuarial: cfg,
	})
	if res.Err != nil {
		t.Fatalf("POD-only forward calc failed: %v", res.Err)
	}

	want := cfg.PODValue(asOf, rate)
	if want == 0 {
		t.Fatal("test config produced a zero POD value — mortality setup broken")
	}
	if math.Abs(res.SumValue-want) > 1e-9 {
		t.Errorf("SumValue = %.6f, want PODValue = %.6f", res.SumValue, want)
	}
	if math.Abs(res.PODValue-want) > 1e-9 {
		t.Errorf("result.PODValue = %.6f, want %.6f", res.PODValue, want)
	}
}
