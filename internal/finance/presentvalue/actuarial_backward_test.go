package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// actuarialTestCfg builds a small life-contingency config for the
// FP8 backward-solver tests.
func actuarialTestCfg(asOf, dob types.DateRec) *actuarial.ActuarialConfig {
	qx := make([]float64, 121)
	for i := range qx {
		qx[i] = 0.001 + 0.0001*float64(i)*float64(i)/120.0
		if qx[i] > 1 {
			qx[i] = 1
		}
	}
	qx[120] = 1
	return &actuarial.ActuarialConfig{
		Table1: actuarial.NewLifeTableFromQx("mock", qx),
		DOB1:   dob,
		Now:    asOf,
	}
}

// TestPV1ActuarialRoundTrip verifies dispatch_gaps FP8: a
// life-contingent lump sum forward-valued, then solved backward for
// its amount, recovers the original amount — the solver must divide
// the residual by the survival probability (DOS PRESVALU.pas:873-883).
func TestPV1ActuarialRoundTrip(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1959, time.January, 1)
	payDate := dateOf(2034, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)

	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: 0.05},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: payDate,
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

	// Backward: blank the amount, feed the sum value back.
	bwd := mk()
	bwd.LumpSums[0].AmtStatus = types.StatusEmpty
	bwd.LumpSums[0].Amt = 0
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("backward: %v", res.Err)
	}
	if math.Abs(res.LumpSums[0].Amt-100000) > 1.0 {
		t.Errorf("solved amount = %.2f, want 100000 (LifeProb divide missing?)",
			res.LumpSums[0].Amt)
	}
}

// TestUnknownPODRoundTrip verifies dispatch_gaps R4-10: a known
// Payment-on-Death amount forward-valued, then solved backward from
// the resulting Sum Value, recovers the original POD amount (DOS
// ComputeUnknownPOD).
func TestUnknownPODRoundTrip(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1959, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)
	cfg.POD = 75000

	base := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: 0.05},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: dateOf(2030, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 20000,
			}},
		}
	}

	known := base()
	a := *cfg
	known.Actuarial = &a
	fwd := Calculate(known)
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}

	// Backward: POD unknown, target Sum Value supplied.
	bwd := base()
	au := *cfg
	au.POD = 0
	au.PODUnknown = true
	bwd.Actuarial = &au
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("backward: %v", res.Err)
	}
	if math.Abs(res.POD-75000) > 1.0 {
		t.Errorf("solved POD = %.2f, want 75000", res.POD)
	}
}

// TestPV2ActuarialRejected verifies that solving for the DATE of a
// life-contingent payment is rejected (DOS no_time_with_life).
func TestPV2ActuarialRejected(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1959, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)

	in := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.05},
			SumValueStatus: types.InOutInput, SumValue: 60000,
		},
		LumpSums: []LumpSumPayment{{
			// Amount only; date blank -> dispatches to solveLumpDate (PV-2).
			AmtStatus: types.InOutInput, Amt: 100000,
			Act: actuarial.Living,
		}},
		Actuarial: cfg,
	}
	res := Calculate(in)
	if res.Err == nil {
		t.Errorf("expected a rejection for solving a contingent payment's date, got none")
	}
}
