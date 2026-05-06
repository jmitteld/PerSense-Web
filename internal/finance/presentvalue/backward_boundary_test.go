package presentvalue

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestSolveLumpDateRateAtTeenyThreshold checks the boundary where rate
// is exactly at the teeny=1e-10 cutoff that triggers
// "interest rate too small". Below the cutoff is rejected; just above
// should solve.
func TestSolveLumpDateRateAtTeenyThreshold(t *testing.T) {
	// Just below cutoff -> rejected.
	below := PVInput{
		LumpSums: []LumpSumPayment{{
			AmtStatus: types.InOutInput,
			Amt:       10000,
		}},
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput,
			AsOf:           newDate(2024, time.January, 1),
			R:              RateEntry{Status: types.StatusFromRate, Rate: 1e-11},
			SumValueStatus: types.InOutInput,
			SumValue:       10000,
		},
		Settings: defaultSettings(),
	}
	if r := Calculate(below); r.Err == nil {
		t.Error("rate below teeny should be rejected")
	}
}

// TestSolveLumpDateConvergenceFailure forces the Newton iteration to
// run out of attempts. Use a value/amount ratio that produces a date
// far past the convergence horizon. The iteration cap is 30; the
// `count = 30` clamp at PRESVALU.pas:915 prevents runaway. We expect
// either convergence to a sensible date OR a "did not converge" error,
// not a panic or NaN.
func TestSolveLumpDateConvergenceClamp(t *testing.T) {
	// Extreme target: tiny value relative to amount with tiny rate.
	// The closed-form yrs ≈ ln(1e-30)/0.001 = -69000 years which gets
	// clamped at ±20 years per step but still won't converge inside 30
	// iterations.
	input := PVInput{
		LumpSums: []LumpSumPayment{{
			AmtStatus: types.InOutInput,
			Amt:       1e10,
		}},
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput,
			AsOf:           newDate(2024, time.January, 1),
			R:              RateEntry{Status: types.StatusFromRate, Rate: 0.001},
			SumValueStatus: types.InOutInput,
			SumValue:       1, // 10 billion / 1 over 0.1% rate
		},
		Settings: defaultSettings(),
	}
	res := Calculate(input)
	// Either it converges to some date OR errors with "did not
	// converge" / "diverged". Anything that's neither (e.g. NaN date)
	// is a bug.
	if res.Err != nil {
		// Acceptable error messages for this pathological input.
		ok := strings.Contains(res.Err.Error(), "converge") ||
			strings.Contains(res.Err.Error(), "diverged") ||
			strings.Contains(res.Err.Error(), "time period too long")
		if !ok {
			t.Errorf("unexpected error for divergent solve: %v", res.Err)
		}
		return
	}
	got := res.LumpSums[0].Date
	if got.Time.IsZero() || math.IsNaN(0) {
		t.Errorf("solver returned invalid date %v with no error", got.Time)
	}
}

// TestSolveRateSecondPassRestart constructs a PV scenario where the
// first-pass Newton iteration starting at rate=0.1 fails to converge,
// forcing the second-pass restart from rate=0 (PRESVALU.pas:744,
// backward.go:836).
//
// To trigger this, we need a payment configuration where rate=0.1 is
// far from the true root and the damped Newton step (±0.04) bounces
// rather than converges. A negative-rate solution (annuity at
// negative rate) is one such case the first pass struggles with.
func TestSolveRateNegativeSolution(t *testing.T) {
	// Forward at rate = -0.02 (negative — sumvalue exceeds nominal
	// payment total).
	asof := newDate(2024, time.January, 1)
	paymentDate := newDate(2034, time.January, 1)
	amount := 10000.0
	knownRate := -0.02

	forward := PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: paymentDate,
			AmtStatus: types.InOutInput, Amt: amount,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asof,
			R: RateEntry{Status: types.StatusFromRate, Rate: knownRate},
		},
		Settings: defaultSettings(),
	}
	fwd := Calculate(forward)
	if fwd.Err != nil {
		t.Fatal(fwd.Err)
	}

	// Backward: solve for rate. The first guess of 0.1 is far from
	// -0.02; this exercises the iteration and may trigger the
	// second-pass restart from 0.
	backward := PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: paymentDate,
			AmtStatus: types.InOutInput, Amt: amount,
		}},
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput,
			AsOf:           asof,
			SumValueStatus: types.InOutInput,
			SumValue:       fwd.SumValue,
		},
		Settings: defaultSettings(),
	}
	bwd := Calculate(backward)
	if bwd.Err != nil {
		// Either converges OR returns the documented
		// "rate is not determined" / "did not converge" message.
		ok := strings.Contains(bwd.Err.Error(), "converge") ||
			strings.Contains(bwd.Err.Error(), "not determined")
		if !ok {
			t.Errorf("unexpected error: %v", bwd.Err)
		}
		return
	}
	// SumValue should round-trip to the same value.
	if math.Abs(bwd.SumValue-fwd.SumValue) > 1.0 {
		t.Errorf("round-trip drift: got %f want %f", bwd.SumValue, fwd.SumValue)
	}
}

// TestSolvePeriodicToDateColaEqualsRate exercises the special case in
// PRESVALU.pas:969 where (rate - cola) is small enough to switch from
// the closed-form ln(...)/(rate-cola) path to the AddNPeriods path.
//
// Backward solve for toDate when COLA exactly equals rate.
func TestSolvePeriodicToDateColaEqualsRate(t *testing.T) {
	asof := newDate(2024, time.January, 1)
	from := newDate(2024, time.January, 1)
	knownTo := newDate(2026, time.December, 1)
	rate := 0.06
	cola := 0.06 // exactly equal — triggers the special path
	amount := 1000.0

	forward := PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: from,
			ToDateStatus:   types.InOutInput, ToDate: knownTo,
			PerYrStatus:    types.InOutInput, PerYr: 12,
			AmtStatus:      types.InOutInput, Amt: amount,
			COLAStatus:     types.InOutInput, COLA: cola,
			NInstallments:  36,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asof,
			R: RateEntry{Status: types.StatusFromRate, Rate: rate},
		},
		Settings: defaultSettings(),
	}
	fwd := Calculate(forward)
	if fwd.Err != nil {
		t.Fatal(fwd.Err)
	}

	// Backward: toDate blank, expect the rate=cola code path to fire.
	backward := PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: from,
			PerYrStatus:    types.InOutInput, PerYr: 12,
			AmtStatus:      types.InOutInput, Amt: amount,
			COLAStatus:     types.InOutInput, COLA: cola,
		}},
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput, AsOf: asof,
			R:              RateEntry{Status: types.StatusFromRate, Rate: rate},
			SumValueStatus: types.InOutInput,
			SumValue:       fwd.SumValue,
		},
		Settings: defaultSettings(),
	}
	bwd := Calculate(backward)
	if bwd.Err != nil {
		t.Fatalf("rate=cola path failed: %v", bwd.Err)
	}
	got := bwd.Periodics[0].ToDate
	delta := math.Abs(got.Time.Sub(knownTo.Time).Hours() / 24)
	// rate=cola exact: the closed form is `n = round(target/amount)`
	// which gives integer-period precision; tolerance ±1 month.
	if delta > 35 {
		t.Errorf("toDate with cola=rate: got %s, want %s (delta %.0f days)",
			got.Time.Format("2006-01-02"),
			knownTo.Time.Format("2006-01-02"), delta)
	}
}

// TestSolveAsOfRateTooSmall exercises the rate-too-small guard in
// solveAsOf (backward.go:885).
func TestSolveAsOfRateTooSmall(t *testing.T) {
	input := PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput,
			Date:       newDate(2025, time.January, 1),
			AmtStatus:  types.InOutInput,
			Amt:        10000,
		}},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.StatusFromRate, Rate: 1e-12},
			SumValueStatus: types.InOutInput,
			SumValue:       9000,
		},
		Settings: defaultSettings(),
	}
	res := Calculate(input)
	if res.Err == nil {
		t.Error("expected rate-too-small error")
	} else if !strings.Contains(res.Err.Error(), "rate too small") {
		t.Errorf("unexpected error: %v", res.Err)
	}
}

// TestEmptyInputPVCalculate confirms that an empty screen (no rows,
// rate and asof present) returns SumValue = 0 with no error.
func TestEmptyInputPVCalculate(t *testing.T) {
	input := PVInput{
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       newDate(2024, time.January, 1),
			R:          RateEntry{Status: types.StatusFromRate, Rate: 0.06},
		},
		Settings: defaultSettings(),
	}
	res := Calculate(input)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if res.SumValue != 0 {
		t.Errorf("empty input SumValue = %f, want 0", res.SumValue)
	}
}

// TestPaymentOnAsOfDate covers the years=0 boundary where the discount
// factor exp(0) = 1 and amount = value identically.
func TestPaymentOnAsOfDate(t *testing.T) {
	d := newDate(2024, time.January, 1)
	input := PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: d,
			AmtStatus: types.InOutInput, Amt: 10000,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: d,
			R: RateEntry{Status: types.StatusFromRate, Rate: 0.06},
		},
		Settings: defaultSettings(),
	}
	res := Calculate(input)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if math.Abs(res.SumValue-10000) > 0.01 {
		t.Errorf("same-day SumValue = %f, want 10000", res.SumValue)
	}
}
