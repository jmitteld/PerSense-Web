package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func nd(y int, m time.Month, d int) types.DateRec {
	return types.NewDateRec(y, m, d)
}

func ds() PVSettings {
	return PVSettings{
		Basis: types.Basis360, PerYr: 12, COLAMonth: types.COLAAnnual,
		Exact: false, YrDays: 360, YrInv: 1.0 / 360,
	}
}

// --- SumFormula extreme ---

func TestSumFormulaVeryLargeN(t *testing.T) {
	// n=10000 at a small positive rate
	lnf := -0.005 / 12 // typical mortgage factor
	got, err := SumFormula(lnf, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if got <= 0 || math.IsInf(got, 0) || math.IsNaN(got) {
		t.Errorf("SumFormula(%g, 10000) = %g", lnf, got)
	}
}

func TestSumFormulaNearZero(t *testing.T) {
	// lnf very close to 0 — tests the second-order approximation
	for _, lnf := range []float64{1e-11, -1e-11, 1e-6, -1e-6} {
		got, err := SumFormula(lnf, 100)
		if err != nil {
			t.Fatalf("SumFormula(%g, 100) error: %v", lnf, err)
		}
		if math.Abs(got-100) > 1 {
			t.Errorf("SumFormula(%g, 100) = %g, want ~100", lnf, got)
		}
	}
}

func TestSumFormulaPositiveLnf(t *testing.T) {
	// Positive lnf means payments grow faster than discount rate (divergent if n→∞)
	got, err := SumFormula(0.01, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Should be > n since each term > 1
	if got <= 100 {
		t.Errorf("SumFormula(0.01, 100) = %g, want > 100", got)
	}
}

// --- LumpSumValue extreme ---

func TestLumpSumValueVeryFarFuture(t *testing.T) {
	asOf := nd(2024, time.January, 1)
	payDate := nd(2124, time.January, 1) // 100 years out
	got, err := LumpSumValue(10000, payDate, asOf, 0.06, types.Basis360, 1.0/360)
	if err != nil {
		t.Fatal(err)
	}
	// exp(-0.06 * 100) * 10000 ≈ 10000 * 0.00248 ≈ $24.8
	if got <= 0 || got > 100 {
		t.Errorf("PV of $10K in 100 years = %f, expected tiny positive", got)
	}
}

func TestLumpSumValueVeryHighRate(t *testing.T) {
	asOf := nd(2024, time.January, 1)
	payDate := nd(2025, time.January, 1)
	got, err := LumpSumValue(10000, payDate, asOf, 0.50, types.Basis360, 1.0/360)
	if err != nil {
		t.Fatal(err)
	}
	// exp(-0.50) * 10000 ≈ 6065
	if got < 5000 || got > 7000 {
		t.Errorf("PV at 50%% rate = %f, expected ~6065", got)
	}
}

func TestLumpSumValueNegativeRate(t *testing.T) {
	// Negative rates: future payments worth MORE
	asOf := nd(2024, time.January, 1)
	payDate := nd(2025, time.January, 1)
	got, err := LumpSumValue(10000, payDate, asOf, -0.02, types.Basis360, 1.0/360)
	if err != nil {
		t.Fatal(err)
	}
	if got <= 10000 {
		t.Errorf("PV with negative rate = %f, want > 10000", got)
	}
}

// --- PeriodicSummation extreme ---

func TestPeriodicSummationVeryLongTerm(t *testing.T) {
	s := ds()
	asOf := nd(2023, time.December, 1)
	from := nd(2024, time.January, 1)
	to := nd(2124, time.January, 1) // 100 years

	got, err := PeriodicSummation(0.06, 0, asOf, from, to, 12, 1200, &s)
	if err != nil {
		t.Fatal(err)
	}
	// Should be large but finite
	if got <= 0 || math.IsInf(got, 0) || math.IsNaN(got) {
		t.Errorf("100-year summation = %g", got)
	}
	// Should be close to the perpetuity value (~200)
	if got < 150 || got > 210 {
		t.Errorf("100-year summation = %g, expected ~200", got)
	}
}

func TestPeriodicSummationWeekly(t *testing.T) {
	s := ds()
	s.Exact = true // weekly needs exact mode
	asOf := nd(2023, time.December, 1)
	from := nd(2024, time.January, 1)
	to := nd(2024, time.December, 31) // ~1 year

	got, err := PeriodicSummation(0.06, 0, asOf, from, to, 52, 52, &s)
	if err != nil {
		t.Fatal(err)
	}
	// 52 weekly payments: factor should be ~50-52
	if got < 40 || got > 55 {
		t.Errorf("weekly summation = %g, expected ~50", got)
	}
}

func TestPeriodicSummationInfinite(t *testing.T) {
	s := ds()
	// COLA >= rate with infinite term should error
	_, err := PeriodicSummation(0.06, 0.06, nd(2024, time.January, 1),
		nd(2024, time.January, 1), types.LatestDate(), 12, 99999, &s)
	if err == nil {
		t.Error("infinite series with COLA >= rate should error")
	}
}

// --- Calculate extreme ---

func TestCalculateNoPayments(t *testing.T) {
	input := PVInput{
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       nd(2024, time.January, 1),
			R:          RateEntry{Status: types.StatusFromRate, Rate: 0.06},
		},
		Settings: ds(),
	}
	result := Calculate(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.SumValue != 0 {
		t.Errorf("PV with no payments = %f, want 0", result.SumValue)
	}
}

func TestCalculateZeroRate(t *testing.T) {
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{DateStatus: types.InOutInput, Date: nd(2025, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 10000},
		},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       nd(2024, time.January, 1),
			R:          RateEntry{Status: types.StatusFromRate, Rate: 0},
		},
		Settings: ds(),
	}
	result := Calculate(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	// At 0% rate, PV = face value
	if math.Abs(result.SumValue-10000) > 1 {
		t.Errorf("PV at 0%% = %f, want 10000", result.SumValue)
	}
}

func TestCalculateMultipleLumpSums(t *testing.T) {
	// 10 lump sums, each $1000, at annual intervals
	var ls []LumpSumPayment
	for i := 1; i <= 10; i++ {
		ls = append(ls, LumpSumPayment{
			DateStatus: types.InOutInput,
			Date:       nd(2024+i, time.January, 1),
			AmtStatus:  types.InOutInput,
			Amt:        1000,
		})
	}
	input := PVInput{
		LumpSums: ls,
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       nd(2024, time.January, 1),
			R:          RateEntry{Status: types.StatusFromRate, Rate: 0.08},
		},
		Settings: ds(),
	}
	result := Calculate(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	// Sum of 10 discounted payments should be less than $10K
	if result.SumValue >= 10000 || result.SumValue < 5000 {
		t.Errorf("PV of 10x$1000 at 8%% = %f", result.SumValue)
	}
	// Should have 10 results
	if len(result.LumpSums) != 10 {
		t.Errorf("got %d lump sum results, want 10", len(result.LumpSums))
	}
	// Each subsequent payment should have lower PV
	for i := 1; i < len(result.LumpSums); i++ {
		if result.LumpSums[i].Val >= result.LumpSums[i-1].Val {
			t.Errorf("PV should decrease: period %d (%f) >= period %d (%f)",
				i+1, result.LumpSums[i].Val, i, result.LumpSums[i-1].Val)
			break
		}
	}
}

func TestCalculateHighCOLA(t *testing.T) {
	s := ds()
	s.Exact = true
	input := PVInput{
		Periodics: []PeriodicPayment{
			{
				FromDateStatus: types.InOutInput, FromDate: nd(2024, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: nd(2033, time.December, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 1000,
				COLAStatus: types.InOutInput, COLA: 0.10, // 10% COLA — extreme
			},
		},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       nd(2024, time.January, 1),
			R:          RateEntry{Status: types.StatusFromRate, Rate: 0.06},
		},
		Settings: s,
	}
	result := Calculate(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	// High COLA should make PV significantly larger than without
	noCOLA := PVInput{
		Periodics: []PeriodicPayment{
			{
				FromDateStatus: types.InOutInput, FromDate: nd(2024, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: nd(2033, time.December, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 1000,
			},
		},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       nd(2024, time.January, 1),
			R:          RateEntry{Status: types.StatusFromRate, Rate: 0.06},
		},
		Settings: s,
	}
	resultNoCOLA := Calculate(noCOLA)
	if result.SumValue <= resultNoCOLA.SumValue {
		t.Errorf("10%% COLA PV (%f) should exceed 0%% COLA PV (%f)",
			result.SumValue, resultNoCOLA.SumValue)
	}
}
