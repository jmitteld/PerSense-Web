package presentvalue

// Coverage tests for the dispatch / guard / advisory branches that the
// existing suite leaves uncovered. Each test names the exact branch it
// exercises so a future reader can map it back to the source.

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// CheckSecondLifeProvided is the exported wrapper (calc.go:367). It was
// at 0% — exercise both the nil-actuarial pass and the two-life reject.
func TestCheckSecondLifeProvidedExported(t *testing.T) {
	// nil actuarial -> no error.
	if err := CheckSecondLifeProvided(PVInput{}); err != nil {
		t.Fatalf("nil actuarial should pass, got %v", err)
	}
	// two-life contingency with only one table -> error.
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	err := CheckSecondLifeProvided(twoLifeLumpInput(cfg, actuarial.BothLiving))
	if err == nil || !strings.Contains(err.Error(), "second life table") {
		t.Fatalf("expected second-life error, got %v", err)
	}
}

// checkSecondLifeProvided names a periodic row when the two-life
// contingency sits on a periodic payment (calc.go:396-399).
func TestCheckSecondLifeProvidedPeriodicRow(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	in := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: 0.06},
		},
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(2040, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 100,
			Act: actuarial.EitherLiving,
		}},
		Actuarial: cfg,
	}
	err := CheckSecondLifeProvided(in)
	if err == nil || !strings.Contains(err.Error(), "periodic payment line 1") {
		t.Fatalf("expected periodic-row two-life error, got %v", err)
	}
}

// Not-enough-inputs: neither frontward nor backward (calc.go:352-353).
// A lone lump-sum row with only a Date, no SumValue, no rate.
func TestCalculateNotEnoughInputs(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: newDate(2030, time.January, 1),
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: newDate(2024, time.January, 1),
			// no rate, no sumvalue
		},
	}
	res := Calculate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "not enough inputs") {
		t.Fatalf("expected not-enough-inputs error, got %v", res.Err)
	}
}

// forwardOnly with rate/as-of missing returns the "needs both rate and
// as-of" error (calc.go:464-466). Call forwardOnly directly so we reach
// the guard regardless of dispatch.
func TestForwardOnlyMissingRateAsOf(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: newDate(2030, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 1000,
		}},
		PresVal: PresValLine{}, // no rate, no as-of
	}
	res := forwardOnly(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "Rate and an As-of Date") {
		t.Fatalf("expected missing rate/as-of error, got %v", res.Err)
	}
}
