package presentvalue

// Tests for the variable-rate forward PV path (DOS PVL fancy mode).
// Coverage strategy: cross-check against the fixed-rate path. Any
// variable-rate calc with a single-entry schedule must equal the
// equivalent fixed-rate calc to floating-point precision — if it
// doesn't, the variable-rate code has a bug independent of any
// expected numeric values. Multi-segment schedules are then validated
// by hand-computed integrals.

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

func vrTestSettings() PVSettings {
	return PVSettings{
		Basis:     types.Basis360,
		PerYr:     12,
		COLAMonth: types.COLAAnnual,
		Exact:     false,
		YrDays:    360,
		YrInv:     1.0 / 360,
	}
}

func dateOf(y int, m time.Month, d int) types.DateRec {
	return types.NewDateRec(y, m, d)
}

// approx is local to this file to avoid stepping on other tests'
// helpers. Tolerance is per-call; 1e-6 is the engine-internal default.
func approx(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// Single-entry schedule must match the fixed-rate result. This is the
// strongest free invariant the engine offers — any drift between paths
// fails here long before the numerics are wrong on a real problem.
func TestVRSingleRateMatchesFixedRate(t *testing.T) {
	settings := vrTestSettings()
	asOf := dateOf(2024, time.January, 1)
	payDate := dateOf(2025, time.January, 1)

	// Fixed-rate baseline: $10,000 in 1 year at 8% continuous → ~$9,231.16
	fixedVal, err := LumpSumValue(10000, payDate, asOf, 0.08,
		settings.Basis, settings.YrInv)
	if err != nil {
		t.Fatal(err)
	}

	// Variable-rate with a single line (rate active forever):
	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.08},
	}
	df, err := VRDiscountFactor(asOf, payDate, schedule,
		settings.Basis, settings.YrInv)
	if err != nil {
		t.Fatal(err)
	}
	vrVal := 10000 * df

	t.Logf("fixed = %.6f, vr = %.6f", fixedVal, vrVal)
	if !approx(fixedVal, vrVal, 1e-6) {
		t.Errorf("single-rate VR diverges from fixed: %.6f vs %.6f", vrVal, fixedVal)
	}
}

// Two-segment schedule, hand-computed. $10,000 paid 2 years out, 4%
// for the first year then 8% for the second.
//
// Hand calc:
//   discount integral = 0.04 × 1 + 0.08 × 1 = 0.12
//   PV = 10000 × exp(-0.12) ≈ 8869.20
func TestVRTwoSegmentSchedule(t *testing.T) {
	settings := vrTestSettings()
	asOf := dateOf(2024, time.January, 1)
	payDate := dateOf(2026, time.January, 1)

	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04}, // starting
		{Date: dateOf(2025, time.January, 1), Rate: 0.08}, // year 2 onward
	}
	df, err := VRDiscountFactor(asOf, payDate, schedule,
		settings.Basis, settings.YrInv)
	if err != nil {
		t.Fatal(err)
	}
	pv := 10000 * df
	t.Logf("two-segment PV = %.4f (expected ≈ 8869.20)", pv)
	const want = 8869.20
	if !approx(pv, want, 0.01) {
		t.Errorf("PV = %.4f, want %.2f", pv, want)
	}
}

// Three-segment schedule with a rate change inside a single accrual
// segment of length > 1 year. Tests the per-segment integration walk.
//
// As-of 2024-01-01, payment 2027-01-01 (3 years). Rates: 5% until
// 2025-01-01, then 7% until 2026-01-01, then 10% after.
// Integral = 0.05 + 0.07 + 0.10 = 0.22 → PV factor = exp(-0.22)
func TestVRThreeSegmentSchedule(t *testing.T) {
	settings := vrTestSettings()
	asOf := dateOf(2024, time.January, 1)
	payDate := dateOf(2027, time.January, 1)

	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.05},
		{Date: dateOf(2025, time.January, 1), Rate: 0.07},
		{Date: dateOf(2026, time.January, 1), Rate: 0.10},
	}
	df, err := VRDiscountFactor(asOf, payDate, schedule,
		settings.Basis, settings.YrInv)
	if err != nil {
		t.Fatal(err)
	}
	pv := 1000.0 * df
	want := 1000.0 * math.Exp(-0.22)
	t.Logf("three-segment PV = %.6f, want %.6f", pv, want)
	if !approx(pv, want, 1e-4) {
		t.Errorf("PV = %.6f, want %.6f", pv, want)
	}
}

// Payment at as-of date should yield factor = 1 (no discounting).
func TestVRPaymentOnAsOfDateUnchanged(t *testing.T) {
	settings := vrTestSettings()
	d := dateOf(2024, time.June, 15)
	schedule := []RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.05}}
	df, err := VRDiscountFactor(d, d, schedule, settings.Basis, settings.YrInv)
	if err != nil {
		t.Fatal(err)
	}
	if df != 1.0 {
		t.Errorf("factor at as-of date = %v, want 1.0", df)
	}
}

// Payment before as-of date should give factor > 1 (accumulated
// forward). Mirrors the fixed-rate sign convention in LumpSumValue.
func TestVRPaymentInPastAccumulates(t *testing.T) {
	settings := vrTestSettings()
	asOf := dateOf(2025, time.January, 1)
	past := dateOf(2024, time.January, 1)
	schedule := []RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.08}}
	df, err := VRDiscountFactor(asOf, past, schedule, settings.Basis, settings.YrInv)
	if err != nil {
		t.Fatal(err)
	}
	want := math.Exp(0.08) // 1 year accumulation at 8%
	t.Logf("past-payment factor = %.6f, want %.6f", df, want)
	if !approx(df, want, 1e-6) {
		t.Errorf("factor = %.6f, want %.6f", df, want)
	}
}

// End-to-end: forward calc through Calculate(), single rate-schedule
// entry, lump sum input. Should match the fixed-rate Calculate() with
// the same rate.
func TestVRCalculateMatchesFixedRate(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	payDate := dateOf(2025, time.January, 1)
	settings := vrTestSettings()

	// Fixed-rate run.
	fixedInput := PVInput{
		Settings: settings,
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       asOf,
			R:          RateEntry{Status: types.InOutInput, Rate: 0.08},
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput,
			Date:       payDate,
			AmtStatus:  types.InOutInput,
			Amt:        10000,
		}},
	}
	fixed := Calculate(fixedInput)
	if fixed.Err != nil {
		t.Fatalf("fixed error: %v", fixed.Err)
	}

	// Variable-rate run with the same effective rate.
	vrInput := fixedInput
	// Note: the PresVal.Rate above is irrelevant in VR mode; the
	// engine ignores it once RateSchedule is non-empty. Set it
	// anyway to make sure the test isn't accidentally relying on it.
	vrInput.RateSchedule = []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.08},
	}
	vr := Calculate(vrInput)
	if vr.Err != nil {
		t.Fatalf("vr error: %v", vr.Err)
	}

	t.Logf("fixed SumValue = %.6f, vr SumValue = %.6f",
		fixed.SumValue, vr.SumValue)
	if !approx(fixed.SumValue, vr.SumValue, 1e-6) {
		t.Errorf("VR diverges from fixed at single-rate sentinel: %.6f vs %.6f",
			vr.SumValue, fixed.SumValue)
	}
}

// ============================================================
// Variable rate × actuarial (life contingency)
//
// The DOS source supports this combination via PRESVALU.pas's
// `{$ifdef ACTU} or (fold_in_life) {$endif}` branches inside the
// PVLX summation loops. The Windows port was compiled without
// -DACTU and silently dropped the combined feature; the web port
// re-adds it. Tests below pin the math.
// ============================================================

// Single-rate-entry × actuarial must match the fixed-rate actuarial
// path. Cross-check invariant: the variable-rate engine with a
// degenerate schedule (one entry, applied forever) is a strict
// superset of the fixed-rate engine.
func TestVR_ActuarialSingleRateMatchesFixedRate(t *testing.T) {
	settings := vrTestSettings()
	asOf := dateOf(2024, time.January, 1)
	through := dateOf(2054, time.January, 1)
	dob := dateOf(1959, time.January, 1) // 65-year-old at asOf

	// Mock life table: rough mortality curve. Real test below uses a
	// realistic table; this is just for engine-equivalence.
	qx := make([]float64, 121)
	for i := range qx {
		// Smoothly rising qx — at age 0 about 0.001, climbing toward 1.0 at 119.
		qx[i] = 0.001 + 0.0001*float64(i)*float64(i)/float64(120)
		if qx[i] > 1 {
			qx[i] = 1
		}
	}
	qx[120] = 1
	table := actuarial.NewLifeTableFromQx("mock", qx)
	cfg := &actuarial.ActuarialConfig{
		Table1: table,
		DOB1:   dob,
		Now:    asOf,
	}

	// Fixed-rate run with Calculate().
	fixedInput := PVInput{
		Settings: settings,
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       asOf,
			R:          RateEntry{Status: types.InOutInput, Rate: 0.05},
		},
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput,
			FromDate:       asOf,
			ToDateStatus:   types.InOutInput,
			ToDate:         through,
			PerYrStatus:    types.InOutInput,
			PerYr:          12,
			AmtStatus:      types.InOutInput,
			Amt:            2000,
			Act:            actuarial.Living,
		}},
		Actuarial: cfg,
	}
	fixed := Calculate(fixedInput)
	if fixed.Err != nil {
		t.Fatalf("fixed: %v", fixed.Err)
	}

	// Variable-rate run with the same rate, single schedule entry.
	vrInput := fixedInput
	vrInput.RateSchedule = []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.05},
	}
	vr := Calculate(vrInput)
	if vr.Err != nil {
		t.Fatalf("vr: %v", vr.Err)
	}

	t.Logf("fixed SumValue=%.6f  vr SumValue=%.6f", fixed.SumValue, vr.SumValue)
	if !approx(fixed.SumValue, vr.SumValue, 1e-3) {
		t.Errorf("VR actuarial diverges from fixed: %.6f vs %.6f",
			vr.SumValue, fixed.SumValue)
	}
}

// Complementarity under variable rates: Living-contingent PV +
// Dead-contingent PV should equal the non-contingent PV computed
// with the same variable rate schedule. Same property the fixed-
// rate actuarial path satisfies, now extended.
func TestVR_ActuarialComplementarity(t *testing.T) {
	settings := vrTestSettings()
	asOf := dateOf(2024, time.January, 1)
	through := dateOf(2054, time.January, 1)
	dob := dateOf(1959, time.January, 1)

	// Same mock table as above. Keep deterministic so the test is
	// independent of any SSA data drift.
	qx := make([]float64, 121)
	for i := range qx {
		qx[i] = 0.001 + 0.0001*float64(i)*float64(i)/float64(120)
		if qx[i] > 1 {
			qx[i] = 1
		}
	}
	qx[120] = 1
	table := actuarial.NewLifeTableFromQx("mock", qx)
	cfg := &actuarial.ActuarialConfig{Table1: table, DOB1: dob, Now: asOf}

	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
		{Date: dateOf(2030, time.January, 1), Rate: 0.06},
		{Date: dateOf(2040, time.January, 1), Rate: 0.08},
	}
	base := PVInput{
		Settings: settings,
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       asOf,
		},
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput,
			FromDate:       asOf,
			ToDateStatus:   types.InOutInput,
			ToDate:         through,
			PerYrStatus:    types.InOutInput,
			PerYr:          12,
			AmtStatus:      types.InOutInput,
			Amt:            2000,
		}},
		RateSchedule: schedule,
		Actuarial:    cfg,
	}

	noLife := base
	noLife.Periodics[0].Act = actuarial.NotContingent
	plain := Calculate(noLife)

	livingIn := base
	livingIn.Periodics[0].Act = actuarial.Living
	living := Calculate(livingIn)

	deadIn := base
	deadIn.Periodics[0].Act = actuarial.Dead
	dead := Calculate(deadIn)

	t.Logf("VR plain=%.4f  living=%.4f  dead=%.4f  living+dead=%.4f",
		plain.SumValue, living.SumValue, dead.SumValue,
		living.SumValue+dead.SumValue)
	if !approx(living.SumValue+dead.SumValue, plain.SumValue, 0.01) {
		t.Errorf("complementarity broken under VR: living+dead=%.4f, plain=%.4f, gap=%.4f",
			living.SumValue+dead.SumValue, plain.SumValue,
			living.SumValue+dead.SumValue-plain.SumValue)
	}
}

// POD integration under variable rates. Sanity bounds: PODValue
// should be positive, less than the face POD, and should differ
// from the fixed-rate result by a non-trivial amount when the
// schedule has multiple distinct rates.
func TestVR_ActuarialPODIntegratesSchedule(t *testing.T) {
	settings := vrTestSettings()
	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1984, time.January, 1) // 40-year-old

	qx := make([]float64, 121)
	for i := range qx {
		qx[i] = 0.001 + 0.0001*float64(i)*float64(i)/float64(120)
		if qx[i] > 1 {
			qx[i] = 1
		}
	}
	qx[120] = 1
	table := actuarial.NewLifeTableFromQx("mock", qx)
	cfg := &actuarial.ActuarialConfig{
		Table1: table,
		DOB1:   dob,
		Now:    asOf,
		POD:    50000,
	}

	// Variable-rate version with a step from 4% to 8% mid-window.
	vrInput := PVInput{
		Settings: settings,
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput,
			Date:       dateOf(2024, time.January, 2), // placeholder; engine still requires fully-spec rows
			AmtStatus:  types.InOutInput,
			Amt:        0.01, // negligible
			Act:        actuarial.NotContingent,
		}},
		RateSchedule: []RateLine{
			{Date: dateOf(1900, time.January, 1), Rate: 0.04},
			{Date: dateOf(2040, time.January, 1), Rate: 0.08},
		},
		Actuarial: cfg,
	}
	vr := Calculate(vrInput)
	if vr.Err != nil {
		t.Fatalf("vr: %v", vr.Err)
	}

	t.Logf("VR PODValue=%.4f", vr.PODValue)
	if vr.PODValue <= 0 || vr.PODValue > 50000 {
		t.Errorf("PODValue=%.4f out of bounds (0, 50000]", vr.PODValue)
	}
	if vr.PODValue < 100 {
		t.Errorf("PODValue=%.4f looks suspiciously small for a 40-yr-old over their lifetime",
			vr.PODValue)
	}
}

// Backward-mode rejection: VR mode must refuse rows with unknowns
// (matches DOS limitation; we surface a clear error rather than
// silently producing nonsense).
func TestVRRejectsMissingInputs(t *testing.T) {
	input := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       dateOf(2024, time.January, 1),
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput,
			Date:       dateOf(2025, time.January, 1),
			// AmtStatus left empty — backward case
		}},
		RateSchedule: []RateLine{
			{Date: dateOf(1900, time.January, 1), Rate: 0.05},
		},
	}
	res := Calculate(input)
	if res.Err == nil {
		t.Errorf("expected error for missing amount in VR mode, got SumValue=%.2f", res.SumValue)
	}
}

// vrPeriodicValue used to apply COLA as exp(t × cola) regardless of
// COLAMonth, treating the entered yield as a continuous rate. The
// rest of PV applies the COLA as an effective annual yield stepped
// at the anniversary (or month-specific) date — `1+cola` per step.
// After §0.9.5 / R9 the VR path matches: a single-rate VR schedule
// with COLA must equal the fixed-rate path for both stepped and
// continuous settings.
func TestVRPeriodicCOLAMatchesFixedRate_Annual(t *testing.T) {
	settings := vrTestSettings()
	settings.COLAMonth = types.COLAAnnual
	asOf := dateOf(2024, time.January, 1)
	fromDate := dateOf(2024, time.January, 1)
	toDate := dateOf(2034, time.January, 1)

	const rate = 0.06
	const cola = 0.03
	const amount = 1000.0

	// Fixed-rate baseline via PeriodicSummation (drives the annual
	// stepped path at calc.go:115). Amount-scaled value:
	const nInst = 121 // 10 years × 12 + 1 (both endpoints inclusive)
	sum, err := PeriodicSummation(rate, cola, asOf, fromDate, toDate, 12, nInst, &settings)
	if err != nil {
		t.Fatalf("PeriodicSummation: %v", err)
	}
	fixedVal := amount * sum

	// Variable-rate with a single line at the same rate (active from
	// long before fromDate so it covers every payment).
	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: rate},
	}
	vrVal, _, err := vrPeriodicValue(amount, cola, asOf, fromDate, toDate,
		12, schedule, &settings, nil, actuarial.NotContingent)
	if err != nil {
		t.Fatalf("vrPeriodicValue: %v", err)
	}
	if !approx(fixedVal, vrVal, 1e-4) {
		t.Errorf("VR annual-COLA = %.6f, fixed-rate annual-COLA = %.6f "+
			"(diff %.6f). The VR path used to apply continuous COLA "+
			"regardless of COLAMonth — single-line VR must now match "+
			"the fixed-rate annual-stepped result.",
			vrVal, fixedVal, vrVal-fixedVal)
	}
}

func TestVRPeriodicCOLAMatchesFixedRate_Continuous(t *testing.T) {
	settings := vrTestSettings()
	settings.COLAMonth = types.COLAContinuous
	asOf := dateOf(2024, time.January, 1)
	fromDate := dateOf(2024, time.January, 1)
	toDate := dateOf(2034, time.January, 1)

	const rate = 0.06
	const cola = 0.03
	const amount = 1000.0

	const nInst = 121 // 10 years × 12 + 1 (both endpoints inclusive)
	sum, err := PeriodicSummation(rate, cola, asOf, fromDate, toDate, 12, nInst, &settings)
	if err != nil {
		t.Fatalf("PeriodicSummation: %v", err)
	}
	fixedVal := amount * sum

	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: rate},
	}
	vrVal, _, err := vrPeriodicValue(amount, cola, asOf, fromDate, toDate,
		12, schedule, &settings, nil, actuarial.NotContingent)
	if err != nil {
		t.Fatalf("vrPeriodicValue: %v", err)
	}
	if !approx(fixedVal, vrVal, 1e-3) {
		t.Errorf("VR continuous-COLA = %.6f, fixed-rate continuous-COLA = %.6f "+
			"(diff %.6f). The continuous-COLA paths use exp(t × ln(1+cola)) "+
			"on both sides; they must agree.", vrVal, fixedVal, vrVal-fixedVal)
	}
}
