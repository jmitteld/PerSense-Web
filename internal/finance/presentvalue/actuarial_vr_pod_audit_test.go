package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// This file audits two under-covered intersections surfaced by the
// DOS re-review: (1) the variable-rate backward amount solve when the
// row is life-contingent, and (2) the unknown Payment-on-Death solve
// when a life-contingent periodic row is also on the screen. Both go
// through forward valuations that were recently corrected to fold in
// survival weighting, so these round trips pin that the backward paths
// invert the same weighted forward value DOS uses (PRESVALU.pas
// PVLX `amtn := valn/FancySummation` with fold_in_life, and
// ComputeUnknownPOD).

// vrAuditSchedule is a simple two-step variable-rate schedule.
func vrAuditSchedule() []RateLine {
	return []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
		{Date: dateOf(2032, time.January, 1), Rate: 0.07},
	}
}

// TestVRBackwardSolvePeriodicAmount_Contingent round-trips the
// variable-rate backward amount solve for a *life-contingent* periodic
// row. The VR solver brackets the amount via two forward runs, so it
// inherits the forward path's survival weighting — this confirms it.
func TestVRBackwardSolvePeriodicAmount_Contingent(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	const wantAmt = 1500.0

	base := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2045, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: wantAmt,
				Act: actuarial.Living,
			}},
			RateSchedule: vrAuditSchedule(),
			Actuarial:    cfg,
		}
	}

	fwd := Calculate(base())
	if fwd.Err != nil {
		t.Fatalf("VR forward: %v", fwd.Err)
	}
	// Confirm contingency actually reduces the value vs. non-contingent,
	// so the round trip is meaningfully exercising the weighting.
	nc := base()
	nc.Periodics[0].Act = actuarial.NotContingent
	ncFwd := Calculate(nc)
	if ncFwd.Err != nil {
		t.Fatalf("VR non-contingent forward: %v", ncFwd.Err)
	}
	if !(fwd.SumValue < ncFwd.SumValue-1) {
		t.Fatalf("VR contingent SumValue %.2f not < non-contingent %.2f", fwd.SumValue, ncFwd.SumValue)
	}

	bwd := base()
	bwd.Periodics[0].AmtStatus = types.StatusEmpty
	bwd.Periodics[0].Amt = 0
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("VR backward solve: %v", res.Err)
	}
	if math.Abs(res.Periodics[0].Amt-wantAmt) > 0.5 {
		t.Errorf("VR contingent solved amount = %.4f, want %.4f", res.Periodics[0].Amt, wantAmt)
	}
}

// TestVRBackwardSolveLumpAmount_Contingent round-trips the variable-rate
// backward amount solve for a life-contingent lump sum.
func TestVRBackwardSolveLumpAmount_Contingent(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	const wantAmt = 80000.0

	base := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
				AmtStatus: types.InOutInput, Amt: wantAmt,
				Act: actuarial.Living,
			}},
			RateSchedule: vrAuditSchedule(),
			Actuarial:    cfg,
		}
	}

	fwd := Calculate(base())
	if fwd.Err != nil {
		t.Fatalf("VR forward: %v", fwd.Err)
	}
	bwd := base()
	bwd.LumpSums[0].AmtStatus = types.StatusEmpty
	bwd.LumpSums[0].Amt = 0
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("VR backward lump solve: %v", res.Err)
	}
	if math.Abs(res.LumpSums[0].Amt-wantAmt) > 0.5 {
		t.Errorf("VR contingent solved lump amount = %.4f, want %.4f", res.LumpSums[0].Amt, wantAmt)
	}
}

// TestSolveUnknownPOD_WithContingentPeriodic solves for an unknown
// Payment-on-Death amount while a life-contingent periodic row is also
// present. The POD solve subtracts the present value of every other row
// from the target; if the contingent periodic row were mis-valued
// (unweighted), the solved POD would absorb the error. Round-trip pins
// that the known-row valuation feeding ComputeUnknownPOD is survival-
// weighted.
func TestSolveUnknownPOD_WithContingentPeriodic(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1)
	rate := 0.06
	const wantPOD = 60000.0

	base := func() PVInput {
		cfg := actuarialTestCfg(asOf, dob)
		cfg.POD = wantPOD
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: rate},
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2042, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 1200,
				Act: actuarial.Living,
			}},
			Actuarial: cfg,
		}
	}

	fwd := Calculate(base())
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}
	if fwd.PODValue <= 0 {
		t.Fatalf("expected positive POD value in forward, got %.4f", fwd.PODValue)
	}

	// Backward: mark POD unknown, supply the forward Sum Value.
	bwd := base()
	bwd.Actuarial.POD = 0
	bwd.Actuarial.PODUnknown = true
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("unknown-POD solve: %v", res.Err)
	}
	if math.Abs(res.POD-wantPOD) > 1.0 {
		t.Errorf("solved POD = %.4f, want %.4f (contingent periodic mis-valued in the residual?)",
			res.POD, wantPOD)
	}
}
