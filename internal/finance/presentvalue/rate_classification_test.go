package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// These tests pin the present-value rate-line classification that the
// Go FirstPass performs in place of the DOS YieldRateTranslation +
// rate-row cascade (PRESVALU.pas:535-588). On closer reading, DOS's
// YieldRateTranslation is a *status classifier*: a rate line counts as
// supplied when its status is at least "default" (the rate-
// representation display next to it is commented out in the
// authoritative build), and FirstPass then increments the missing
// count by the presence of the As-of Date and the Sum Value. The Go
// FirstPass replicates exactly that counting (backward.go:353-373):
// rate + as-of present -> forward; rate blank with Sum Value -> rate
// solve; as-of blank with Sum Value -> as-of solve; under-supplied ->
// error. The only DOS feature not modelled is multiple rate ROWS with
// empty-row deletion, which does not apply to the single-rate-line
// screen model and has no effect on the computed numbers.

func rateClassPeriodicRow() PeriodicPayment {
	return PeriodicPayment{
		FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
		ToDateStatus: types.InOutInput, ToDate: dateOf(2040, time.January, 1),
		PerYrStatus: types.InOutInput, PerYr: 12,
		AmtStatus: types.InOutInput, Amt: 1000,
	}
}

// TestRateLineClassificationDispatch walks the rate/as-of/sum-value
// presence matrix and asserts the engine routes to the right path.
func TestRateLineClassificationDispatch(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	rate := 0.06

	// Establish a self-consistent target Sum Value via a forward calc.
	fwd := Calculate(PVInput{
		Settings:  vrTestSettings(),
		PresVal:   PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf, R: RateEntry{Status: types.StatusFromRate, Rate: rate}},
		Periodics: []PeriodicPayment{rateClassPeriodicRow()},
	})
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}
	target := fwd.SumValue

	// Case 1: rate + as-of present, no Sum Value -> forward calc.
	t.Run("forward", func(t *testing.T) {
		res := Calculate(PVInput{
			Settings:  vrTestSettings(),
			PresVal:   PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf, R: RateEntry{Status: types.StatusFromRate, Rate: rate}},
			Periodics: []PeriodicPayment{rateClassPeriodicRow()},
		})
		if res.Err != nil {
			t.Fatalf("unexpected error: %v", res.Err)
		}
		if math.Abs(res.SumValue-target) > 0.01 {
			t.Errorf("forward SumValue = %.4f, want %.4f", res.SumValue, target)
		}
	})

	// Case 2: rate blank, as-of + Sum Value present -> solve rate (PV-8).
	t.Run("rate_solve", func(t *testing.T) {
		res := Calculate(PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R:              RateEntry{Status: types.StatusEmpty},
				SumValueStatus: types.InOutInput, SumValue: target,
			},
			Periodics: []PeriodicPayment{rateClassPeriodicRow()},
		})
		if res.Err != nil {
			t.Fatalf("rate solve error: %v", res.Err)
		}
		if math.Abs(res.Rate-rate) > 1e-4 {
			t.Errorf("solved rate = %.6f, want %.6f", res.Rate, rate)
		}
	})

	// Case 3: as-of blank, rate + Sum Value present -> solve as-of (PV-9).
	t.Run("asof_solve", func(t *testing.T) {
		res := Calculate(PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus:     types.StatusEmpty,
				R:              RateEntry{Status: types.StatusFromRate, Rate: rate},
				SumValueStatus: types.InOutInput, SumValue: target,
			},
			Periodics: []PeriodicPayment{rateClassPeriodicRow()},
		})
		if res.Err != nil {
			t.Fatalf("as-of solve error: %v", res.Err)
		}
		gotYrs := math.Abs(res.AsOf.Time.Sub(asOf.Time).Hours() / (24 * 365.25))
		if gotYrs > 0.05 {
			t.Errorf("solved as-of = %s, want %s", res.AsOf.Time.Format("2006-01-02"), asOf.Time.Format("2006-01-02"))
		}
	})

	// Case 4: rate AND as-of both blank, Sum Value present -> two
	// unknowns, the engine must refuse rather than guess.
	t.Run("under_determined", func(t *testing.T) {
		res := Calculate(PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus:     types.StatusEmpty,
				R:              RateEntry{Status: types.StatusEmpty},
				SumValueStatus: types.InOutInput, SumValue: target,
			},
			Periodics: []PeriodicPayment{rateClassPeriodicRow()},
		})
		if res.Err == nil {
			t.Errorf("expected an error for rate+as-of both blank, got SumValue %.4f", res.SumValue)
		}
	})
}

// TestRateProvidedAsYieldCountsAsPresent confirms a rate supplied in a
// non-"FromRate" representation (here a yield) is still classified as
// present — the YieldRateTranslation behavior — so the screen runs a
// forward calc rather than reporting the rate as missing.
func TestRateProvidedAsYieldCountsAsPresent(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.StatusFromYield, Rate: 0.06},
		},
		Periodics: []PeriodicPayment{rateClassPeriodicRow()},
	})
	if res.Err != nil {
		t.Fatalf("a yield-supplied rate should classify as present, got error: %v", res.Err)
	}
	if res.SumValue <= 0 {
		t.Errorf("expected a positive SumValue with a yield-supplied rate, got %.4f", res.SumValue)
	}
}
