package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// The Present Value total the engine returns must always equal the sum of the
// per-row Values it returns. This is the computational guard behind the
// "PV total doesn't match the rows" class of bug: if a forward calc ever
// reports a SumValue that diverges from the rows it computed, this fails.
// (The stale-total symptom that prompted this was a frontend display issue —
// the computed total wasn't cleared when the rows changed — but this pins the
// backend invariant so a real summation regression can't hide.)

func sumRowValues(r PVResult) float64 {
	s := 0.0
	for i := range r.LumpSums {
		s += r.LumpSums[i].Val
	}
	for i := range r.Periodics {
		s += r.Periodics[i].Val
	}
	return s
}

func TestSumValueEqualsRowSum_Mixed(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	in := PVInput{
		Settings: defaultSettings(),
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf, R: RateEntry{Status: types.InOutInput, Rate: 0.08}},
		LumpSums: []LumpSumPayment{
			{DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1), AmtStatus: types.InOutInput, Amt: 10000},
			{DateStatus: types.InOutInput, Date: newDate(2030, time.January, 1), AmtStatus: types.InOutInput, Amt: 50000},
		},
		Periodics: []PeriodicPayment{
			{FromDateStatus: types.InOutInput, FromDate: asOf, ToDateStatus: types.InOutInput, ToDate: newDate(2034, time.January, 1), PerYrStatus: types.InOutInput, PerYr: 12, AmtStatus: types.InOutInput, Amt: 1000},
		},
	}
	r := Calculate(in)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	want := sumRowValues(r)
	if math.Abs(r.SumValue-want) > 0.01 {
		t.Errorf("SumValue=%.4f but sum of row Values=%.4f (diff %.4f)", r.SumValue, want, r.SumValue-want)
	}
}

func TestSumValueEqualsRowSum_SingleLump(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: newDate(2024, time.January, 1), R: RateEntry{Status: types.InOutInput, Rate: 0.08}},
		LumpSums: []LumpSumPayment{
			{DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1), AmtStatus: types.InOutInput, Amt: 10000},
		},
	}
	r := Calculate(in)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	if math.Abs(r.SumValue-r.LumpSums[0].Val) > 0.01 {
		t.Errorf("single lump: SumValue=%.4f but row Value=%.4f", r.SumValue, r.LumpSums[0].Val)
	}
}

// Reducing a multi-row screen to a single lump must give a SumValue equal to
// that lump alone — i.e. the total reflects the current rows, not a prior set.
func TestSumValueReflectsCurrentRows(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	mk := func(lumps []LumpSumPayment) PVResult {
		return Calculate(PVInput{
			Settings: defaultSettings(),
			PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf, R: RateEntry{Status: types.InOutInput, Rate: 0.08}},
			LumpSums: lumps,
		})
	}
	multi := mk([]LumpSumPayment{
		{DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1), AmtStatus: types.InOutInput, Amt: 10000},
		{DateStatus: types.InOutInput, Date: newDate(2030, time.January, 1), AmtStatus: types.InOutInput, Amt: 50000},
		{DateStatus: types.InOutInput, Date: newDate(2040, time.January, 1), AmtStatus: types.InOutInput, Amt: 25000},
	})
	single := mk([]LumpSumPayment{
		{DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1), AmtStatus: types.InOutInput, Amt: 10000},
	})
	if single.Err != nil || multi.Err != nil {
		t.Fatalf("calc err: multi=%v single=%v", multi.Err, single.Err)
	}
	if math.Abs(single.SumValue-single.LumpSums[0].Val) > 0.01 {
		t.Errorf("single SumValue=%.4f, want row value %.4f", single.SumValue, single.LumpSums[0].Val)
	}
	if multi.SumValue <= single.SumValue {
		t.Errorf("multi total (%.2f) should exceed single (%.2f)", multi.SumValue, single.SumValue)
	}
}
