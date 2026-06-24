package presentvalue

// Coverage for appendResultAdvisories (advisories.go) — the P-W3/P-W4/
// P-W7 result-sanity advisories. Existing advisories_test.go covers some
// paths; these target the specific branches the coverage profile flags.

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func hasCode(warnings []string, code string) bool {
	for _, w := range warnings {
		if strings.Contains(w, code) {
			return true
		}
	}
	return false
}

// P-W3: a solved rate above 1.0 (100%) triggers the "very high rate"
// note (advisories.go:27-31). Construct a deep-discount lump sum: a
// $100,000 payment one year out worth only $1,000 today implies a rate
// near ln(100) ~ 4.6.
func TestAdvisoryPW3HighRate(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 100000,
		}},
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput,
			AsOf:           newDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput, SumValue: 36000, // solve rate (~1.02)
		},
	}
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Rate <= 1.0 {
		t.Fatalf("expected solved rate > 1.0, got %v", res.Rate)
	}
	if !hasCode(res.Warnings, "P-W3") {
		t.Fatalf("expected P-W3 high-rate advisory, got %v", res.Warnings)
	}
}

// P-W4 (lump): a solved lump-sum Amount that comes out ~0 triggers the
// "essentially zero" advisory (advisories.go:34-41). Supply a row whose
// own Value target is ~0 so the solved amount is ~0.
func TestAdvisoryPW4LumpZeroAmount(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1),
			// Amount blank -> solved; Value target 0.
			ValStatus: types.InOutInput, Val: 0.0,
		}},
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput,
			AsOf:           newDate(2024, time.January, 1),
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			SumValueStatus: types.InOutInput, SumValue: 0.0,
		},
	}
	res := Calculate(in)
	// A zero-value row may surface a FirstPass error (value cannot be
	// zero when it's the only field). Use a tiny non-zero value instead.
	if res.Err != nil {
		in.LumpSums[0].Val = 0.4
		in.PresVal.SumValue = 0.4
		res = Calculate(in)
		if res.Err != nil {
			t.Fatalf("unexpected error: %v", res.Err)
		}
	}
	if !hasCode(res.Warnings, "P-W4") {
		t.Fatalf("expected P-W4 lump advisory, got warnings=%v amt=%v", res.Warnings, res.LumpSums[0].Amt)
	}
}

// P-W4 (periodic): a solved periodic Amount that comes out ~0 triggers
// the periodic "essentially zero" advisory (advisories.go:43-50).
func TestAdvisoryPW4PeriodicZeroAmount(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: newDate(2025, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: newDate(2030, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			// Amount blank -> solved; tiny target Value.
			ValStatus: types.InOutInput, Val: 0.5,
		}},
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput,
			AsOf:           newDate(2024, time.January, 1),
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			SumValueStatus: types.InOutInput, SumValue: 0.5,
		},
	}
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !hasCode(res.Warnings, "P-W4") {
		t.Fatalf("expected P-W4 periodic advisory, got warnings=%v amt=%v",
			res.Warnings, res.Periodics[0].Amt)
	}
}

// P-W7: a forward calc with non-zero payments that net to ~0 at the
// given rate (advisories.go:53-71). Two equal-and-opposite lump sums on
// the same date cancel exactly.
func TestAdvisoryPW7NettingToZero(t *testing.T) {
	d := newDate(2025, time.January, 1)
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{
			{DateStatus: types.InOutInput, Date: d, AmtStatus: types.InOutInput, Amt: 10000},
			{DateStatus: types.InOutInput, Date: d, AmtStatus: types.InOutInput, Amt: -10000},
		},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: newDate(2024, time.January, 1),
			R: RateEntry{Status: types.InOutInput, Rate: 0.06},
		},
	}
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !hasCode(res.Warnings, "P-W7") {
		t.Fatalf("expected P-W7 netting advisory, got warnings=%v sum=%v",
			res.Warnings, res.SumValue)
	}
}
