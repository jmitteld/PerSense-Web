package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func newDate(y int, m time.Month, d int) types.DateRec {
	return types.NewDateRec(y, m, d)
}

func defaultSettings() PVSettings {
	return PVSettings{
		Basis:     types.Basis360,
		PerYr:     12,
		COLAMonth: types.COLAAnnual,
		Exact:     false,
		YrDays:    360,
		YrInv:     1.0 / 360,
	}
}

// --- Zero/Empty tests ---

func TestZeroLumpSum(t *testing.T) {
	var ls LumpSumPayment
	ls.Amt = 10000
	ls.AmtStatus = types.InOutInput
	ZeroLumpSum(&ls)
	if !LumpSumIsEmpty(&ls) {
		t.Error("zeroed lump sum should be empty")
	}
}

func TestZeroPeriodic(t *testing.T) {
	var p PeriodicPayment
	p.Amt = 500
	ZeroPeriodic(&p)
	if !PeriodicIsEmpty(&p) {
		t.Error("zeroed periodic should be empty")
	}
}

func TestZeroPresValLine(t *testing.T) {
	var pv PresValLine
	pv.SumValue = 50000
	ZeroPresValLine(&pv)
	if !PresValLineIsEmpty(&pv) {
		t.Error("zeroed presval line should be empty")
	}
}

// --- SumFormula tests ---

func TestSumFormulaZeroRate(t *testing.T) {
	// When lnf ≈ 0, sum = n
	got, err := SumFormula(0, 360)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-360) > 0.001 {
		t.Errorf("SumFormula(0, 360) = %f, want 360", got)
	}
}

func TestSumFormulaGeometric(t *testing.T) {
	// For lnf = ln(0.5) ≈ -0.693, n=10:
	// (1 - 0.5^10) / (1 - 0.5) = (1 - 1/1024) / 0.5 ≈ 1.998
	lnf := math.Log(0.5)
	got, err := SumFormula(lnf, 10)
	if err != nil {
		t.Fatal(err)
	}
	expected := (1 - math.Pow(0.5, 10)) / (1 - 0.5)
	if math.Abs(got-expected) > 0.001 {
		t.Errorf("SumFormula(ln(0.5), 10) = %f, want %f", got, expected)
	}
}

func TestSumFormulaTinyRate(t *testing.T) {
	// Second-order approximation region
	lnf := 1e-6
	n := 100.0
	got, err := SumFormula(lnf, n)
	if err != nil {
		t.Fatal(err)
	}
	// Should be close to n
	if math.Abs(got-n) > 1 {
		t.Errorf("SumFormula(1e-6, 100) = %f, want ~100", got)
	}
}

// --- LumpSumValue tests ---

func TestLumpSumValueSameDate(t *testing.T) {
	// When asOf == paymentDate, value == amount
	date := newDate(2024, time.January, 1)
	got, err := LumpSumValue(10000, date, date, 0.06, types.Basis360, 1.0/360)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-10000) > 0.01 {
		t.Errorf("LumpSumValue(same date) = %f, want 10000", got)
	}
}

func TestLumpSumValueFuture(t *testing.T) {
	// Payment in the future is worth less today
	asOf := newDate(2024, time.January, 1)
	payDate := newDate(2025, time.January, 1)
	got, err := LumpSumValue(10000, payDate, asOf, 0.06, types.Basis360, 1.0/360)
	if err != nil {
		t.Fatal(err)
	}
	// exp(0.06 * (-1)) * 10000 ≈ 9417.6
	if got >= 10000 || got < 9000 {
		t.Errorf("LumpSumValue(1yr future) = %f, expected ~9418", got)
	}
}

func TestLumpSumValuePast(t *testing.T) {
	// Payment in the past is worth more today (accumulated)
	asOf := newDate(2025, time.January, 1)
	payDate := newDate(2024, time.January, 1)
	got, err := LumpSumValue(10000, payDate, asOf, 0.06, types.Basis360, 1.0/360)
	if err != nil {
		t.Fatal(err)
	}
	// exp(0.06 * 1) * 10000 ≈ 10618.4
	if got <= 10000 || got > 11000 {
		t.Errorf("LumpSumValue(1yr past) = %f, expected ~10618", got)
	}
}

func TestLumpSumValueZeroRate(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	payDate := newDate(2025, time.January, 1)
	got, err := LumpSumValue(10000, payDate, asOf, 0, types.Basis360, 1.0/360)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-10000) > 0.01 {
		t.Errorf("LumpSumValue(0%% rate) = %f, want 10000", got)
	}
}

// --- PeriodicSummation tests ---

func TestPeriodicSummationZeroRate(t *testing.T) {
	settings := defaultSettings()
	asOf := newDate(2023, time.December, 1)
	from := newDate(2024, time.January, 1)
	to := newDate(2024, time.December, 1)

	got, err := PeriodicSummation(0, 0, asOf, from, to, 12, 12, &settings)
	if err != nil {
		t.Fatal(err)
	}
	// At 0% rate, factor = number of payments (approximately)
	if math.Abs(got-12) > 1 {
		t.Errorf("PeriodicSummation(0%%) = %f, want ~12", got)
	}
}

func TestPeriodicSummationPositiveRate(t *testing.T) {
	settings := defaultSettings()
	asOf := newDate(2023, time.December, 1)
	from := newDate(2024, time.January, 1)
	to := newDate(2053, time.December, 1)

	got, err := PeriodicSummation(0.06, 0, asOf, from, to, 12, 360, &settings)
	if err != nil {
		t.Fatal(err)
	}
	// 30 years monthly at 6%: PV factor should be ~167 (similar to mortgage Summation)
	if got < 100 || got > 250 {
		t.Errorf("PeriodicSummation(6%%, 30yr) = %f, expected ~167", got)
	}
}

func TestPeriodicSummationWithCOLA(t *testing.T) {
	settings := defaultSettings()
	settings.Exact = true // use exact mode for COLA

	asOf := newDate(2023, time.December, 1)
	from := newDate(2024, time.January, 1)
	to := newDate(2033, time.December, 1)

	// Without COLA
	noCola, err := PeriodicSummation(0.06, 0, asOf, from, to, 12, 120, &settings)
	if err != nil {
		t.Fatal(err)
	}

	// With 3% COLA
	withCola, err := PeriodicSummation(0.06, 0.03, asOf, from, to, 12, 120, &settings)
	if err != nil {
		t.Fatal(err)
	}

	// COLA should increase the present value
	if withCola <= noCola {
		t.Errorf("COLA should increase PV: noCola=%f, withCola=%f", noCola, withCola)
	}
}

func TestPeriodicSummationExact(t *testing.T) {
	settings := defaultSettings()
	settings.Exact = true

	asOf := newDate(2023, time.December, 1)
	from := newDate(2024, time.January, 1)
	to := newDate(2024, time.December, 1)

	got, err := PeriodicSummation(0.06, 0, asOf, from, to, 12, 12, &settings)
	if err != nil {
		t.Fatal(err)
	}
	if got < 10 || got > 13 {
		t.Errorf("PeriodicSummation(exact, 1yr) = %f, expected ~11.6", got)
	}
}

// --- Calculate tests ---

func TestCalculateLumpSums(t *testing.T) {
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{
				DateStatus: types.InOutInput,
				Date:       newDate(2025, time.January, 1),
				AmtStatus:  types.InOutInput,
				Amt:        10000,
			},
			{
				DateStatus: types.InOutInput,
				Date:       newDate(2026, time.January, 1),
				AmtStatus:  types.InOutInput,
				Amt:        10000,
			},
		},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       newDate(2024, time.January, 1),
			R: RateEntry{
				Status: types.StatusFromRate,
				Rate:   0.06,
			},
			SumValueStatus: types.StatusEmpty,
		},
		Settings: defaultSettings(),
	}

	result := Calculate(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	// Both lump sums should have values computed
	for i, ls := range result.LumpSums {
		if ls.ValStatus != types.InOutOutput {
			t.Errorf("lumpsum[%d] value not computed", i)
		}
		if ls.Val <= 0 {
			t.Errorf("lumpsum[%d] value should be positive: %f", i, ls.Val)
		}
	}

	// Sum should be less than $20K (discounted)
	if result.SumValue >= 20000 || result.SumValue < 15000 {
		t.Errorf("sum value = %f, expected roughly 18000-19500", result.SumValue)
	}

	// First payment (1yr out) should be worth more than second (2yr out)
	if result.LumpSums[0].Val <= result.LumpSums[1].Val {
		t.Error("nearer payment should have higher present value")
	}
}

func TestCalculatePeriodic(t *testing.T) {
	input := PVInput{
		Periodics: []PeriodicPayment{
			{
				FromDateStatus: types.InOutInput,
				FromDate:       newDate(2024, time.January, 1),
				ToDateStatus:   types.InOutInput,
				ToDate:         newDate(2053, time.December, 1),
				PerYrStatus:    types.InOutInput,
				PerYr:          12,
				AmtStatus:      types.InOutInput,
				Amt:            1000,
				NInstallments:  360,
			},
		},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       newDate(2023, time.December, 1),
			R: RateEntry{
				Status: types.StatusFromRate,
				Rate:   0.06,
			},
		},
		Settings: defaultSettings(),
	}

	result := Calculate(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	if len(result.Periodics) != 1 {
		t.Fatal("expected 1 periodic result")
	}

	pp := result.Periodics[0]
	if pp.ValStatus != types.InOutOutput {
		t.Error("periodic value not computed")
	}

	// $1000/mo for 30yr at 6%: PV ≈ $166,791
	if pp.Val < 100000 || pp.Val > 250000 {
		t.Errorf("periodic PV = %f, expected ~167000", pp.Val)
	}
}

func TestCalculateMixed(t *testing.T) {
	// Lump sum + periodic
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{
				DateStatus: types.InOutInput,
				Date:       newDate(2025, time.June, 1),
				AmtStatus:  types.InOutInput,
				Amt:        50000,
			},
		},
		Periodics: []PeriodicPayment{
			{
				FromDateStatus: types.InOutInput,
				FromDate:       newDate(2024, time.January, 1),
				ToDateStatus:   types.InOutInput,
				ToDate:         newDate(2024, time.December, 1),
				PerYrStatus:    types.InOutInput,
				PerYr:          12,
				AmtStatus:      types.InOutInput,
				Amt:            1000,
				NInstallments:  12,
			},
		},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       newDate(2024, time.January, 1),
			R: RateEntry{
				Status: types.StatusFromRate,
				Rate:   0.05,
			},
		},
		Settings: defaultSettings(),
	}

	result := Calculate(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	// Sum should be lump sum PV + periodic PV
	lsPV := result.LumpSums[0].Val
	ppPV := result.Periodics[0].Val
	if math.Abs(result.SumValue-lsPV-ppPV) > 0.01 {
		t.Errorf("sum (%f) != lumpsum (%f) + periodic (%f)", result.SumValue, lsPV, ppPV)
	}
}

func TestCalculateInsufficientData(t *testing.T) {
	result := Calculate(PVInput{Settings: defaultSettings()})
	if result.Err == nil {
		t.Error("empty input should produce error")
	}
}
