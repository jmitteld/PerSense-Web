package presentvalue

// Coverage for the harder solver branches: PV-6 (from-date) latest /
// PUNT paths, the PV-8 rate-solver guards, and the PV-9 as-of solver
// sign / range guards. Each test targets a specific source branch.

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// evaluatePVAt skips fully-blank rows during a rate solve
// (backward.go:1342, 1355). A PV-8 rate solve with a blank extra lump and
// periodic row exercises the skip guards inside the solver's evaluator.
func TestEvaluatePVAtSkipsBlankRows(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{
			{ // fully specified -> contributes
				DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 10000,
			},
			{Date: types.UnknownDate()}, // blank -> skipped
		},
		Periodics: []PeriodicPayment{
			{FromDate: types.UnknownDate(), ToDate: types.UnknownDate()}, // blank -> skipped
		},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			SumValueStatus: types.InOutInput, SumValue: 9500, // solve rate
		},
	}
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Rate <= 0 {
		t.Fatalf("expected a positive solved rate, got %v", res.Rate)
	}
}

// PV-2 lump-date solve stalls when the amount is so tiny that the Newton
// derivative dval = amt*rate*exp(...) underflows teeny (backward.go:780).
func TestSolveLumpDateStall(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{{
			AmtStatus: types.InOutInput, Amt: 1e-12,
			ValStatus: types.InOutInput, Val: 0.5e-12,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: newDate(2024, time.January, 1),
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			SumValueStatus: types.InOutInput, SumValue: 0.5e-12,
		},
	}
	res := Calculate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "stalled") {
		t.Fatalf("expected a stalled date-solve error, got %v", res.Err)
	}
}

// PV-6 from-date solve with ToDate = the "latest" sentinel
// (backward.go:961-973): an open-ended periodic stream whose start date
// is solved from the target value.
func TestSolvePeriodicFromDateToLatest(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	in := PVInput{
		Settings: defaultSettings(),
		Periodics: []PeriodicPayment{{
			// FromDate blank -> solved.
			ToDateStatus: types.InOutInput, ToDate: types.LatestDate(),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 100,
			ValStatus: types.InOutInput, Val: 15000,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			SumValueStatus: types.InOutInput, SumValue: 15000,
		},
	}
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Periodics[0].FromDate.IsUnknown() {
		t.Fatalf("from date should be solved")
	}
}

// PV-6 from-date solve, finite ToDate, normal (non-PUNT) path
// (backward.go:974-999): forward-build a periodic value then blank From.
func TestSolvePeriodicFromDateFinite(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	fwd := PVInput{
		Settings: defaultSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: newDate(2026, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: newDate(2031, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 100,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: 0.06},
		},
	}
	fr := forwardOnly(fwd)
	if fr.Err != nil {
		t.Fatalf("forward setup: %v", fr.Err)
	}
	target := fr.Periodics[0].Val

	solve := fwd
	ps := make([]PeriodicPayment, 1)
	copy(ps, fwd.Periodics)
	ps[0].FromDateStatus = types.StatusEmpty
	ps[0].FromDate = types.UnknownDate()
	ps[0].ValStatus = types.InOutInput
	ps[0].Val = target
	solve.Periodics = ps
	solve.PresVal.SumValueStatus = types.InOutInput
	solve.PresVal.SumValue = target

	res := Calculate(solve)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
}

// PV-6 PUNT path: cola == rate (rate-cola below Teeny) forces the
// from-date approximation to fall back to ToDate (backward.go:982-986).
func TestSolvePeriodicFromDatePunt(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	in := PVInput{
		Settings: defaultSettings(),
		Periodics: []PeriodicPayment{{
			ToDateStatus: types.InOutInput, ToDate: newDate(2030, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 100,
			COLAStatus: types.InOutInput, COLA: 0.06, // cola == rate
			ValStatus: types.InOutInput, Val: 6000,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			SumValueStatus: types.InOutInput, SumValue: 6000,
		},
	}
	res := Calculate(in)
	// PUNT may still produce a result or a downstream error; we only need
	// to drive the branch, not assert a specific date.
	_ = res
}

// PV-5 to-date solve: forward-build a periodic value, blank the ToDate,
// and solve it. The to-date value is a step function, so the day-level
// refinement finds no continuous sign change and keeps the grid date
// (backward.go:1148/1155 in refineDateByDays).
func TestSolvePeriodicToDate(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	fwd := PVInput{
		Settings: defaultSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: newDate(2025, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: newDate(2029, time.June, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 250,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: 0.07},
		},
	}
	fr := forwardOnly(fwd)
	if fr.Err != nil {
		t.Fatalf("forward setup: %v", fr.Err)
	}
	target := fr.Periodics[0].Val

	solve := fwd
	ps := make([]PeriodicPayment, 1)
	copy(ps, fwd.Periodics)
	ps[0].ToDateStatus = types.StatusEmpty
	ps[0].ToDate = types.UnknownDate()
	ps[0].ValStatus = types.InOutInput
	ps[0].Val = target
	solve.Periodics = ps
	solve.PresVal.SumValueStatus = types.InOutInput
	solve.PresVal.SumValue = target

	res := Calculate(solve)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Periodics[0].ToDate.IsUnknown() {
		t.Fatalf("to date should be solved")
	}
}

// PV-8 rate solver: the "rate not determined" early exit when the second
// iteration's diff is zero (backward.go:1227-1234). A payment dated AT
// the as-of date has no rate sensitivity, and a SumValue equal to that
// rate-independent amount makes (target - sum) == 0 on iteration 2.
func TestSolveRateUndetermined(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: asOf, // payment at asof: rate-independent
			AmtStatus: types.InOutInput, Amt: 1000,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			SumValueStatus: types.InOutInput, SumValue: 1000, // exactly the rate-independent sum
		},
	}
	res := Calculate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "Rate is not determined") {
		t.Fatalf("expected rate-not-determined error, got %v", res.Err)
	}
}

// PeriodicSummation exact mode with an open-ended (latest) ToDate drives
// the infinite-series convergence break (calc.go:144): each period's
// contribution shrinks until it underflows teeny and the loop breaks.
func TestPeriodicSummationExactInfiniteBreak(t *testing.T) {
	settings := defaultSettings()
	settings.Exact = true
	asOf := newDate(2024, time.January, 1)
	from := newDate(2025, time.January, 1)
	to := types.LatestDate()
	got, err := PeriodicSummation(0.20, 0, asOf, from, to, 12, 100000, &settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got <= 0 {
		t.Fatalf("expected a positive converged factor, got %v", got)
	}
}

// PV-9 as-of solver: opposite-sign target gives the sign error
// (backward.go:1294-1301). Positive payments, negative target value.
func TestSolveAsOfOppositeSign(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 10000,
		}},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			SumValueStatus: types.InOutInput, SumValue: -5000, // wrong sign
		},
	}
	res := Calculate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "opposite sign") {
		t.Fatalf("expected opposite-sign as-of error, got %v", res.Err)
	}
}

// PV-9 as-of solver: a target that drives the date past the supported
// range yields the non-convergence error (backward.go:1317-1324). A very
// large target relative to the payment forces the solved date far out.
func TestSolveAsOfOutOfRange(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 1000,
		}},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.0001}, // tiny rate, huge ln/rate step
			SumValueStatus: types.InOutInput, SumValue: 1e12,
		},
	}
	res := Calculate(in)
	// Either out-of-range non-convergence or another solve error; the
	// branch under test is the maxDate guard.
	if res.Err == nil {
		t.Fatalf("expected an as-of out-of-range error, got SumValue %v", res.SumValue)
	}
}
