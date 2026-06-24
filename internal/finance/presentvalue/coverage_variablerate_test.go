package presentvalue

// Coverage for variable-rate branches (variablerate.go) the existing
// suite leaves uncovered: the integrate guard, the forward error arms
// (missing dates / blank amount / peryr<=0 / empty schedule), the
// degenerate vrDiscountByYears offset, and the amount/POD/date solver
// error guards.

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func vrSched() []RateLine {
	return []RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.06}}
}

// integrateRateForward returns 0 when from >= to (variablerate.go:76).
func TestIntegrateRateForwardReversed(t *testing.T) {
	s := vrSched()
	got := integrateRateForward(dateOf(2025, time.January, 1),
		dateOf(2024, time.January, 1), s, types.Basis360, 1.0/360)
	if got != 0 {
		t.Fatalf("reversed integrate should be 0, got %v", got)
	}
}

// forwardVariableRate called with an empty schedule reports the internal
// "called without a rate schedule" error (variablerate.go:278-281).
func TestForwardVariableRateNoSchedule(t *testing.T) {
	res := forwardVariableRate(PVInput{Settings: vrTestSettings()})
	if res.Err == nil || !strings.Contains(res.Err.Error(), "without a rate schedule") {
		t.Fatalf("expected no-schedule error, got %v", res.Err)
	}
}

// A VR periodic row with peryr<=0 errors out of vrPeriodicValue
// (variablerate.go:163) via forwardVariableRate's peryr guard
// (variablerate.go:331-333).
func TestVRPeriodicPerYrZero(t *testing.T) {
	in := PVInput{
		Settings: vrTestSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(2025, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(2030, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 0,
			AmtStatus: types.InOutInput, Amt: 100,
		}},
		PresVal:      PresValLine{AsOfStatus: types.InOutInput, AsOf: dateOf(2024, time.January, 1)},
		RateSchedule: vrSched(),
	}
	res := Calculate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "Pmts-Yr") {
		t.Fatalf("expected peryr error, got %v", res.Err)
	}
}

// A VR lump row missing its Date errors (variablerate.go:300-303).
func TestVRLumpMissingDate(t *testing.T) {
	in := PVInput{
		Settings: vrTestSettings(),
		LumpSums: []LumpSumPayment{{
			AmtStatus: types.InOutInput, Amt: 1000, // Date blank
		}},
		PresVal:      PresValLine{AsOfStatus: types.InOutInput, AsOf: dateOf(2024, time.January, 1)},
		RateSchedule: vrSched(),
	}
	// With a blank amount/date and no SumValue, this routes to
	// forwardVariableRate (no backward target), which reports the
	// incomplete-row error. Supply a SumValue would instead try a solve;
	// keep SumValue blank to hit the forward arm.
	res := forwardVariableRate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "Date or Amount") {
		t.Fatalf("expected missing date/amount error, got %v", res.Err)
	}
}

// A VR periodic row missing a date errors (variablerate.go:323-324).
func TestVRPeriodicMissingDate(t *testing.T) {
	in := PVInput{
		Settings: vrTestSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(2025, time.January, 1),
			// ToDate blank
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 100,
		}},
		PresVal:      PresValLine{AsOfStatus: types.InOutInput, AsOf: dateOf(2024, time.January, 1)},
		RateSchedule: vrSched(),
	}
	res := forwardVariableRate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "From Date or To Date") {
		t.Fatalf("expected missing periodic date error, got %v", res.Err)
	}
}

// A VR periodic row with both dates but blank amount (and no SumValue
// target) errors (variablerate.go:327-329).
func TestVRPeriodicBlankAmount(t *testing.T) {
	in := PVInput{
		Settings: vrTestSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(2025, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(2030, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			// amount blank
		}},
		PresVal:      PresValLine{AsOfStatus: types.InOutInput, AsOf: dateOf(2024, time.January, 1)},
		RateSchedule: vrSched(),
	}
	res := forwardVariableRate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "blank Amount") {
		t.Fatalf("expected blank-amount error, got %v", res.Err)
	}
}

// vrDiscountByYears returns 1.0 for a zero offset (variablerate.go:388).
func TestVRDiscountByYearsZero(t *testing.T) {
	settings := vrTestSettings()
	got := vrDiscountByYears(0, dateOf(2024, time.January, 1), vrSched(), &settings)
	if got != 1.0 {
		t.Fatalf("zero-offset discount should be 1.0, got %v", got)
	}
}

// solveVariableRateAmount: a blank lump amount solved from the target
// Sum Value (variablerate.go forward solve, covering r0/r1/unit/solve).
func TestSolveVariableRateAmountLump(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	in := PVInput{
		Settings: vrTestSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2025, time.January, 1),
			// amount blank -> solved
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			SumValueStatus: types.InOutInput, SumValue: 9417.65,
		},
		RateSchedule: vrSched(),
	}
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	// amount ≈ 9417.65 * exp(0.06) ≈ 10000.
	if res.LumpSums[0].Amt < 9000 || res.LumpSums[0].Amt > 11000 {
		t.Fatalf("solved amount out of range: %v", res.LumpSums[0].Amt)
	}
}

// solveVariableRatePOD: a blank Payment-on-Death solved from the target
// Sum Value in VR mode (variablerate.go:495-528).
func TestSolveVariableRatePOD(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1954, time.January, 1))
	cfg.PODUnknown = true
	in := PVInput{
		Settings: vrTestSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2030, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 10000,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			SumValueStatus: types.InOutInput, SumValue: 30000,
		},
		RateSchedule: vrSched(),
		Actuarial:    cfg,
	}
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.POD == 0 {
		t.Fatalf("expected a non-zero solved POD")
	}
}

// vrUnknownDate skips a row whose amount is blank (variablerate.go:553)
// and counts a both-dates-blank periodic as two unknowns (565), so the
// date solve does not fire and the engine routes to the amount solver.
func TestVRUnknownDateAmountBlankAndBothBlank(t *testing.T) {
	in := &PVInput{
		Periodics: []PeriodicPayment{
			{ // amount blank -> skipped by vrUnknownDate
				FromDateStatus: types.InOutInput, FromDate: dateOf(2025, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2030, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
			},
			{ // both dates blank, amount present -> counts as 2 unknowns
				AmtStatus: types.InOutInput, Amt: 100,
				PerYrStatus: types.InOutInput, PerYr: 12,
			},
		},
	}
	_, _, ok := vrUnknownDate(in)
	if ok {
		t.Fatalf("vrUnknownDate should not report a single unknown here")
	}
}

// solveVariableRateDate: a blank lump Date solved from the target Sum
// Value via the bisection path (variablerate.go:582-700).
func TestSolveVariableRateDateLump(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	// Forward: $10,000 one year out at 6% VR -> ~9417.65. Solve the date.
	in := PVInput{
		Settings: vrTestSettings(),
		LumpSums: []LumpSumPayment{{
			AmtStatus: types.InOutInput, Amt: 10000,
			// Date blank -> solved
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			SumValueStatus: types.InOutInput, SumValue: 9417.65,
		},
		RateSchedule: vrSched(),
	}
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	// Solved date should be ~1 year past asOf.
	yr := res.LumpSums[0].Date.Time.Year()
	if yr < 2024 || yr > 2026 {
		t.Fatalf("solved date year out of range: %d", yr)
	}
}

// solveVariableRateDate periodic To-date solve, exercising the
// installment-grid snap-down (variablerate.go:682-694).
func TestSolveVariableRateDatePeriodicTo(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	// Build a forward periodic to get a real SumValue, then blank the To.
	fwd := PVInput{
		Settings: vrTestSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(2025, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(2030, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 100,
		}},
		PresVal:      PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
		RateSchedule: vrSched(),
	}
	fr := forwardVariableRate(fwd)
	if fr.Err != nil {
		t.Fatalf("forward setup error: %v", fr.Err)
	}
	target := fr.SumValue

	solve := fwd
	ps := make([]PeriodicPayment, 1)
	copy(ps, fwd.Periodics)
	ps[0].ToDateStatus = types.StatusEmpty
	ps[0].ToDate = types.UnknownDate()
	solve.Periodics = ps
	solve.PresVal.SumValueStatus = types.InOutInput
	solve.PresVal.SumValue = target

	res := Calculate(solve)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Periodics[0].ToDate.IsUnknown() {
		t.Fatalf("To date should be solved")
	}
}
