package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestBackwardSolveEchoesKnownRowValues verifies that when a backward
// solve targets one unknown row, the OTHER (known, fully-specified)
// rows have their present value reported in the result instead of a
// stale 0. The solved row is filled by its own solver; the known rows
// are filled by computeKnownRowSum. Regression guard for the
// display-only gap where known rows echoed Val=0 in backward solves.
func TestBackwardSolveEchoesKnownRowValues(t *testing.T) {
	asof := newDate(2026, time.January, 1)
	rate := 0.06

	// A known periodic row: $1000/yr-equivalent monthly stream,
	// 2027-2030. We first value it on the forward path to learn its PV,
	// then confirm the backward solve reports that same PV on the row.
	knownPeriodic := func() PeriodicPayment {
		return PeriodicPayment{
			FromDateStatus: types.InOutInput,
			FromDate:       newDate(2027, time.January, 1),
			ToDateStatus:   types.InOutInput,
			ToDate:         newDate(2030, time.January, 1),
			PerYrStatus:    types.InOutInput,
			PerYr:          12,
			AmtStatus:      types.InOutInput,
			Amt:            1000,
		}
	}

	// Forward: value the periodic row alone to get its expected PV.
	fwd := Calculate(PVInput{
		Periodics: []PeriodicPayment{knownPeriodic()},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       asof,
			R:          RateEntry{Status: types.StatusFromRate, Rate: rate},
		},
		Settings: defaultSettings(),
	})
	if fwd.Err != nil {
		t.Fatal(fwd.Err)
	}
	wantPeriodicPV := fwd.Periodics[0].Val
	if math.Abs(wantPeriodicPV) < 1 {
		t.Fatalf("forward periodic PV unexpectedly near zero: %v", wantPeriodicPV)
	}

	// Backward: add an unknown lump (date known, amount blank) and a
	// target SumValue. The solver fills the lump; the known periodic row
	// must now report wantPeriodicPV, not 0.
	target := wantPeriodicPV + 5000.0
	bwd := Calculate(PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput,
			Date:       newDate(2030, time.January, 1),
		}},
		Periodics: []PeriodicPayment{knownPeriodic()},
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput,
			AsOf:           asof,
			R:              RateEntry{Status: types.StatusFromRate, Rate: rate},
			SumValueStatus: types.InOutInput,
			SumValue:       target,
		},
		Settings: defaultSettings(),
	})
	if bwd.Err != nil {
		t.Fatal(bwd.Err)
	}

	gotPeriodicPV := bwd.Periodics[0].Val
	if math.Abs(gotPeriodicPV-wantPeriodicPV) > 0.01 {
		t.Errorf("known periodic row Val = %.4f, want %.4f (was the known-row "+
			"value left at 0?)", gotPeriodicPV, wantPeriodicPV)
	}
	if bwd.Periodics[0].ValStatus != types.InOutOutput {
		t.Errorf("known periodic row ValStatus = %d, want InOutOutput (%d)",
			bwd.Periodics[0].ValStatus, types.InOutOutput)
	}

	// The reported pieces must reconcile with the target: solved lump PV
	// + known periodic PV == target SumValue.
	lumpPV := bwd.LumpSums[0].Val
	if math.Abs((lumpPV+gotPeriodicPV)-target) > 0.01 {
		t.Errorf("rows do not reconcile: lump %.4f + periodic %.4f = %.4f, want %.4f",
			lumpPV, gotPeriodicPV, lumpPV+gotPeriodicPV, target)
	}
}
