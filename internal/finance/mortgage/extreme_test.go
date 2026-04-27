package mortgage

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// --- Summation extreme values ---

func TestSummationVeryHighRate(t *testing.T) {
	// 50% rate, 30 years
	got, err := Summation(0.50, 30)
	if err != nil {
		t.Fatal(err)
	}
	// At 50% the factor should be small (heavy discounting)
	if got <= 0 || got > 30 {
		t.Errorf("Summation(0.50, 30) = %f", got)
	}
}

func TestSummationVeryLongTerm(t *testing.T) {
	// 6% rate, 100 years
	got, err := Summation(0.06, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Should converge to approximately 1/(1-e^(-r/12)) * e^(-r/12)
	if got <= 0 {
		t.Errorf("Summation(0.06, 100) = %f, want positive", got)
	}
	// Compare with 30-year: 100-year should be larger but not by much
	got30, _ := Summation(0.06, 30)
	if got < got30 {
		t.Error("100-year summation should be >= 30-year")
	}
}

func TestSummationTinyRate(t *testing.T) {
	// Nearly zero rate
	got, err := Summation(1e-12, 30)
	if err != nil {
		t.Fatal(err)
	}
	// Should be close to 12*30 = 360
	if math.Abs(got-360) > 1 {
		t.Errorf("Summation(1e-12, 30) = %f, want ~360", got)
	}
}

func TestSummationNegativeRate(t *testing.T) {
	// Negative rate (deflation scenario)
	got, err := Summation(-0.02, 10)
	if err != nil {
		t.Fatal(err)
	}
	// With negative rate, payments grow in value → summation > 12*10
	if got <= 120 {
		t.Errorf("Summation(-0.02, 10) = %f, want > 120", got)
	}
}

// --- Calc extreme values ---

func TestCalcVeryExpensiveHouse(t *testing.T) {
	m := MtgLine{
		PriceStatus: inp(),
		Price:       10000000, // $10M
		PctStatus:   inp(),
		Pct:         0.20,
		YearsStatus: inp(),
		Years:       30,
		RateStatus:  inp(),
		Rate:        0.07,
		TaxStatus:   inp(),
		Tax:         0,
		BalloonStat: types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err != nil {
		t.Fatalf("Calc error on $10M: %v", result.Err)
	}
	// Financed = $8M, monthly should be ~$53K
	if result.Line.Monthly < 40000 || result.Line.Monthly > 70000 {
		t.Errorf("monthly on $10M = %f", result.Line.Monthly)
	}
}

func TestCalcVerySmallLoan(t *testing.T) {
	m := MtgLine{
		PriceStatus: inp(),
		Price:       100, // $100
		PctStatus:   inp(),
		Pct:         0.10,
		YearsStatus: inp(),
		Years:       1,
		RateStatus:  inp(),
		Rate:        0.06,
		TaxStatus:   inp(),
		Tax:         0,
		BalloonStat: types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err != nil {
		t.Fatalf("Calc error on $100: %v", result.Err)
	}
	if result.Line.Monthly <= 0 {
		t.Error("monthly should be positive")
	}
}

func TestCalcZeroDownPayment(t *testing.T) {
	m := MtgLine{
		PriceStatus: inp(),
		Price:       200000,
		PctStatus:   inp(),
		Pct:         0, // 0% down
		YearsStatus: inp(),
		Years:       30,
		RateStatus:  inp(),
		Rate:        0.06,
		TaxStatus:   inp(),
		Tax:         0,
		BalloonStat: types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err != nil {
		t.Fatalf("Calc error: %v", result.Err)
	}
	if result.Line.Financed != 200000 {
		t.Errorf("financed with 0%% down = %f, want 200000", result.Line.Financed)
	}
	if result.Line.Cash != 0 {
		t.Errorf("cash with 0%% down = %f, want 0", result.Line.Cash)
	}
}

func TestCalcHighPoints(t *testing.T) {
	m := MtgLine{
		PriceStatus:  inp(),
		Price:        200000,
		PctStatus:    inp(),
		Pct:          0.20,
		PointsStatus: inp(),
		Points:       0.05, // 5 points
		YearsStatus:  inp(),
		Years:        30,
		RateStatus:   inp(),
		Rate:         0.05,
		TaxStatus:    inp(),
		Tax:          0,
		BalloonStat:  types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	// Cash should include points: price*(pct + (1-pct)*points) = 200000*(0.20 + 0.80*0.05) = 200000*0.24 = 48000
	if math.Abs(result.Line.Cash-48000) > 1 {
		t.Errorf("cash with 5 points = %f, want 48000", result.Line.Cash)
	}
}

func TestCalcZeroRate(t *testing.T) {
	m := MtgLine{
		PriceStatus: inp(),
		Price:       120000,
		PctStatus:   inp(),
		Pct:         0,
		YearsStatus: inp(),
		Years:       10,
		RateStatus:  inp(),
		Rate:        0, // 0% interest
		TaxStatus:   inp(),
		Tax:         0,
		BalloonStat: types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	// At 0%, monthly = financed / (12 * years) = 120000 / 120 = 1000
	if math.Abs(result.Line.Monthly-1000) > 1 {
		t.Errorf("monthly at 0%% = %f, want 1000", result.Line.Monthly)
	}
}

// --- APR extreme values ---

func TestAPRHighPoints(t *testing.T) {
	m := MtgLine{
		PriceStatus:    inp(),
		Price:          200000,
		PctStatus:      inp(),
		Pct:            0.20,
		PointsStatus:   inp(),
		Points:         0.08, // 8 points — extreme
		YearsStatus:    inp(),
		Years:          30,
		RateStatus:     inp(),
		Rate:           0.05,
		TaxStatus:      inp(),
		Tax:            0,
		BalloonStat:    types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	apr, conv, err := FullTermAPR(result.Line, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	if !conv {
		t.Error("APR should converge even with high points")
	}
	// APR should be significantly higher than nominal rate (5%)
	nominalYield := 0.05 // the input rate
	if apr <= nominalYield {
		t.Errorf("APR with 8 points = %f, should exceed nominal rate %f", apr, nominalYield)
	}
}

func TestAPRShortTerm(t *testing.T) {
	// 1-year mortgage with points
	m := MtgLine{
		PriceStatus:    inp(),
		Price:          100000,
		PctStatus:      inp(),
		Pct:            0,
		PointsStatus:   inp(),
		Points:         0.02,
		YearsStatus:    inp(),
		Years:          1,
		RateStatus:     inp(),
		Rate:           0.06,
		TaxStatus:      inp(),
		Tax:            0,
		BalloonStat:    types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	apr, conv, err := FullTermAPR(result.Line, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	if !conv {
		t.Error("APR should converge for 1-year loan")
	}
	// Short term + points = very high APR
	if apr <= 0.08 {
		t.Errorf("1-year APR with 2 points = %f, expected > 8%%", apr)
	}
}

// --- TerminalBalloon sanity ---

func TestTerminalBalloonDecreasing(t *testing.T) {
	m := MtgLine{
		PriceStatus:    inp(),
		Price:          200000,
		PctStatus:      inp(),
		Pct:            0.20,
		YearsStatus:    inp(),
		Years:          30,
		RateStatus:     inp(),
		Rate:           0.06,
		TaxStatus:      inp(),
		Tax:            0,
		BalloonStat:    types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	// Balance should decrease monotonically over time
	prevBal := result.Line.Financed * 2 // start higher than any balance
	for year := 1; year <= 30; year++ {
		bal, err := TerminalBalloon(&result.Line, float64(year))
		if err != nil {
			t.Fatalf("year %d: %v", year, err)
		}
		if bal > prevBal+1 { // +1 for rounding tolerance
			t.Errorf("balance increased at year %d: %f > %f", year, bal, prevBal)
		}
		prevBal = bal
	}
}

// --- Calc validation ---

func TestCalcFinancedExceedsPrice(t *testing.T) {
	m := MtgLine{
		PriceStatus:    inp(),
		Price:          100000,
		FinancedStatus: inp(),
		Financed:       200000, // more than price
		YearsStatus:    inp(),
		Years:          30,
		RateStatus:     inp(),
		Rate:           0.06,
		BalloonStat:    types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err == nil {
		t.Error("financed > price should produce error")
	}
}

func TestCalcZeroPrice(t *testing.T) {
	m := MtgLine{
		PriceStatus: inp(),
		Price:       0,
		PctStatus:   inp(),
		Pct:         0.20,
		YearsStatus: inp(),
		Years:       30,
		RateStatus:  inp(),
		Rate:        0.06,
		BalloonStat: types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err == nil {
		t.Error("zero price should produce error")
	}
}
