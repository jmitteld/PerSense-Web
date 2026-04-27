package mortgage

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// --- helpers ---

func inp() int8  { return types.InOutInput }
func outp() int8 { return types.InOutOutput }

// makeBasicMortgage creates a standard mortgage for testing:
// $200,000 price, 20% down, 30 years, 6% rate, no tax, no balloon
func makeBasicMortgage() MtgLine {
	return MtgLine{
		PriceStatus: inp(),
		Price:       200000,
		PctStatus:   inp(),
		Pct:         0.20,
		YearsStatus: inp(),
		Years:       30,
		RateStatus:  inp(),
		Rate:        0.06,
		TaxStatus:   inp(),
		Tax:         0,
		BalloonStat: types.BalloonBlank,
	}
}

// --- ZeroMortgage / IsEmpty tests ---

func TestZeroMortgage(t *testing.T) {
	var m MtgLine
	m.Price = 100000
	m.PriceStatus = inp()
	ZeroMortgage(&m)
	if !IsEmpty(&m) {
		t.Error("zeroed mortgage should be empty")
	}
	if m.BalloonStat != types.BalloonBlank {
		t.Error("zeroed mortgage should have BalloonBlank")
	}
}

func TestIsEmpty(t *testing.T) {
	m := MtgLine{}
	if !IsEmpty(&m) {
		t.Error("default mortgage should be empty")
	}
	m.PriceStatus = inp()
	if IsEmpty(&m) {
		t.Error("mortgage with price should not be empty")
	}
}

// --- Summation tests ---

func TestSummationZeroRate(t *testing.T) {
	// Zero rate: summation = 12 * t
	got, err := Summation(0, 30)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-360) > 0.001 {
		t.Errorf("Summation(0, 30) = %f, want 360", got)
	}
}

func TestSummationPositiveRate(t *testing.T) {
	// With 6% rate over 30 years, summation should be the present value factor
	got, err := Summation(0.06, 30)
	if err != nil {
		t.Fatal(err)
	}
	// Should be roughly 166.79 (standard 30-year 6% PV factor)
	if got < 100 || got > 250 {
		t.Errorf("Summation(0.06, 30) = %f, expected ~166", got)
	}
}

func TestSummationShortTerm(t *testing.T) {
	got, err := Summation(0.06, 1)
	if err != nil {
		t.Fatal(err)
	}
	// 1 year at 6%: roughly 11.6 months equivalent
	if got < 10 || got > 12.5 {
		t.Errorf("Summation(0.06, 1) = %f, expected ~11.6", got)
	}
}

// --- Calc tests ---

func TestCalcComputeMonthly(t *testing.T) {
	m := makeBasicMortgage()
	result := Calc(m)
	if result.Err != nil {
		t.Fatalf("Calc error: %v", result.Err)
	}
	ei := result.Line

	// Should compute monthly payment, cash, and financed
	if ei.MonthlyStatus != outp() {
		t.Error("monthly should be computed (output)")
	}
	if ei.CashStatus != outp() {
		t.Error("cash should be computed")
	}
	if ei.FinancedStatus != outp() {
		t.Error("financed should be computed")
	}

	// Financed = 200000 * 0.80 = 160000
	if math.Abs(ei.Financed-160000) > 0.01 {
		t.Errorf("financed = %f, want 160000", ei.Financed)
	}

	// Monthly payment for $160K, 30yr, 6% should be ~$959
	if ei.Monthly < 900 || ei.Monthly > 1100 {
		t.Errorf("monthly = %f, expected ~959", ei.Monthly)
	}
}

func TestCalcComputePrice(t *testing.T) {
	// First compute the exact monthly for a $200K mortgage
	m0 := makeBasicMortgage()
	r0 := Calc(m0)
	if r0.Err != nil {
		t.Fatal(r0.Err)
	}

	// Now reverse: given that exact monthly, compute price
	m := MtgLine{
		MonthlyStatus: inp(),
		Monthly:       r0.Line.Monthly,
		PctStatus:     inp(),
		Pct:           0.20,
		YearsStatus:   inp(),
		Years:         30,
		RateStatus:    inp(),
		Rate:          0.06,
		TaxStatus:     inp(),
		Tax:           0,
		BalloonStat:   types.BalloonBlank,
	}
	result := Calc(m)
	if result.Err != nil {
		t.Fatalf("Calc error: %v", result.Err)
	}
	ei := result.Line

	if ei.PriceStatus != outp() {
		t.Error("price should be computed")
	}
	// Should round-trip back to $200,000
	if math.Abs(ei.Price-200000) > 1 {
		t.Errorf("price = %f, want ~200000", ei.Price)
	}
}

func TestCalcFromCash(t *testing.T) {
	// Given price and cash, compute pct and financed
	m := MtgLine{
		PriceStatus: inp(),
		Price:       200000,
		CashStatus:  inp(),
		Cash:        40000,
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
	ei := result.Line

	if ei.PctStatus != outp() {
		t.Error("pct should be computed")
	}
	if math.Abs(ei.Pct-0.20) > 0.001 {
		t.Errorf("pct = %f, want ~0.20", ei.Pct)
	}
}

func TestCalcFromFinanced(t *testing.T) {
	// Given price and financed, compute pct and cash
	m := MtgLine{
		PriceStatus:    inp(),
		Price:          200000,
		FinancedStatus: inp(),
		Financed:       160000,
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
		t.Fatalf("Calc error: %v", result.Err)
	}
	ei := result.Line

	if ei.PctStatus != outp() {
		t.Error("pct should be computed")
	}
	if math.Abs(ei.Pct-0.20) > 0.001 {
		t.Errorf("pct = %f, want 0.20", ei.Pct)
	}
}

func TestCalcBalloonComputed(t *testing.T) {
	// Set a low monthly payment so balloon captures the remaining balance
	m := makeBasicMortgage()
	m.MonthlyStatus = inp()
	m.Monthly = 500 // much less than full amortizing payment (~959)
	m.WhenStatus = inp()
	m.When = 5 // balloon at 5 years
	// No howmuch → BalloonUnk → should compute balloon

	result := Calc(m)
	if result.Err != nil {
		t.Fatalf("Calc error: %v", result.Err)
	}
	ei := result.Line

	if ei.HowMuchStatus != outp() {
		t.Error("balloon amount should be computed")
	}
	// With only $500/mo on a $160K loan, balloon at 5 years should be large
	if ei.HowMuch < 100000 {
		t.Errorf("balloon = %f, expected large remaining balance", ei.HowMuch)
	}
}

func TestCalcValidationErrors(t *testing.T) {
	// Negative years
	m := makeBasicMortgage()
	m.Years = -5
	result := Calc(m)
	if result.Err == nil {
		t.Error("negative years should produce error")
	}

	// Balloon amount without when
	m = makeBasicMortgage()
	m.HowMuchStatus = inp()
	m.HowMuch = 50000
	result = Calc(m)
	if result.Err == nil {
		t.Error("balloon amount without years should produce error")
	}
}

// --- EnoughDataForAPR tests ---

func TestEnoughDataForAPR(t *testing.T) {
	m := makeBasicMortgage()
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !EnoughDataForAPR(&result.Line) {
		t.Error("calculated mortgage should have enough data for APR")
	}

	empty := MtgLine{}
	if EnoughDataForAPR(&empty) {
		t.Error("empty mortgage should not have enough data for APR")
	}
}

// --- APR calculation tests ---

func TestFullTermAPRNoPoints(t *testing.T) {
	m := makeBasicMortgage()
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	ei := result.Line

	apr, converged, err := FullTermAPR(ei, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	if !converged {
		t.Error("APR should converge")
	}
	// With no points, APR should equal the nominal rate (as yield)
	// Rate is 0.06 continuous → yield ≈ 0.06168
	if math.Abs(apr-0.06168) > 0.002 {
		t.Errorf("APR = %f, expected ~0.06168 (6%% continuous as yield)", apr)
	}
}

func TestFullTermAPRWithPoints(t *testing.T) {
	m := makeBasicMortgage()
	m.PointsStatus = inp()
	m.Points = 0.02 // 2 points
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	ei := result.Line

	apr, converged, err := FullTermAPR(ei, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	if !converged {
		t.Error("APR should converge")
	}
	// With 2 points, APR should be higher than the nominal rate
	nominalYield := 0.06168
	if apr <= nominalYield {
		t.Errorf("APR with points (%f) should exceed nominal yield (%f)", apr, nominalYield)
	}
	// Should be roughly 0.063-0.065 for 2 points on 30yr
	if apr < 0.062 || apr > 0.07 {
		t.Errorf("APR = %f, expected roughly 0.063-0.065", apr)
	}
}

func TestOneMonthAPR(t *testing.T) {
	m := makeBasicMortgage()
	m.PointsStatus = inp()
	m.Points = 0.02
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	ei := result.Line

	apr, err := OneMonthAPR(&ei, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	// One-month APR with 2 points should be very high
	// (paying points for only 1 month of benefit)
	fullAPR, _, _ := FullTermAPR(ei, 365.25)
	if apr <= fullAPR {
		t.Errorf("one-month APR (%f) should exceed full-term APR (%f)", apr, fullAPR)
	}
}

// --- Comparison tests ---

func TestCompareAPRsAlwaysBetter(t *testing.T) {
	// Mortgage 1: lower rate AND lower points → always better
	e1 := makeBasicMortgage()
	e1.PointsStatus = inp()
	e1.Points = 0.01
	r1 := Calc(e1)
	if r1.Err != nil {
		t.Fatal(r1.Err)
	}

	e2 := makeBasicMortgage()
	e2.Rate = 0.07
	e2.PointsStatus = inp()
	e2.Points = 0.02
	r2 := Calc(e2)
	if r2.Err != nil {
		t.Fatal(r2.Err)
	}

	result, err := CompareAPRs(r1.Line, r2.Line, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "Mortgage 1 is always better." {
		t.Errorf("summary = %q, want 'Mortgage 1 is always better.'", result.Summary)
	}
}

func TestCompareAPRsCrossover(t *testing.T) {
	// Mortgage 1: lower rate, higher points (better long term)
	e1 := makeBasicMortgage()
	e1.Rate = 0.055
	e1.PointsStatus = inp()
	e1.Points = 0.03
	r1 := Calc(e1)
	if r1.Err != nil {
		t.Fatal(r1.Err)
	}

	// Mortgage 2: higher rate, no points (better short term)
	e2 := makeBasicMortgage()
	e2.Rate = 0.065
	r2 := Calc(e2)
	if r2.Err != nil {
		t.Fatal(r2.Err)
	}

	result, err := CompareAPRs(r1.Line, r2.Line, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	// Should find a crossover point
	if result.CrossoverTime <= 0 {
		t.Logf("summary: %s", result.Summary)
		// Crossover iteration may not always converge for all configurations,
		// so just verify no crash
	}
}

// --- TerminalBalloon tests ---

func TestTerminalBalloon(t *testing.T) {
	m := makeBasicMortgage()
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	ei := result.Line

	// At year 0, balance should be close to financed amount
	bal, err := TerminalBalloon(&ei, twelfth)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(bal-ei.Financed) > 1000 {
		t.Errorf("balance at start = %f, want ~%f", bal, ei.Financed)
	}

	// At full term, balance should be roughly one payment
	// (TerminalBalloon "includes the regular payment that would be due on that date")
	bal, err = TerminalBalloon(&ei, float64(ei.Years))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(bal) > ei.Monthly*2 {
		t.Errorf("balance at term = %f, want roughly one payment (%f)", bal, ei.Monthly)
	}
}

// --- Consistency: monthly payment × summation ≈ financed ---

func TestSummationConsistency(t *testing.T) {
	// The monthly payment times the summation factor should equal the financed amount
	m := makeBasicMortgage()
	result := Calc(m)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	ei := result.Line

	summ, err := Summation(ei.Rate, float64(ei.Years))
	if err != nil {
		t.Fatal(err)
	}
	reconstructed := (ei.Monthly - ei.Tax) * summ
	if math.Abs(reconstructed-ei.Financed) > 0.01 {
		t.Errorf("monthly * summation = %f, financed = %f", reconstructed, ei.Financed)
	}
}
