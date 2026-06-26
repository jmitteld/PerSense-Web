package mortgage

import (
	"testing"

	"github.com/persense/persense-port/internal/types"
)

const yrdays360 = 360.0

// computedBasic returns a fully-solved standard mortgage (monthly computed).
func computedBasic(rate float64, years int) MtgLine {
	m := MtgLine{
		PriceStatus: inp(), Price: 200000,
		PctStatus: inp(), Pct: 0.20,
		YearsStatus: inp(), Years: years,
		RateStatus: inp(), Rate: rate,
		TaxStatus:   inp(),
		BalloonStat: types.BalloonBlank,
	}
	res := Calc(m)
	if res.Err != nil {
		panic(res.Err)
	}
	return res.Line
}

// computedBalloon returns a solved mortgage carrying a known balloon.
func computedBalloon(rate float64, years, when int, howmuch float64) MtgLine {
	m := MtgLine{
		PriceStatus: inp(), Price: 200000,
		PctStatus: inp(), Pct: 0.20,
		YearsStatus: inp(), Years: years,
		RateStatus: inp(), Rate: rate,
		TaxStatus:  inp(),
		WhenStatus: inp(), When: when,
		HowMuchStatus: inp(), HowMuch: howmuch,
	}
	res := Calc(m)
	if res.Err != nil {
		panic(res.Err)
	}
	return res.Line
}

func TestCalc_DispatchBranches(t *testing.T) {
	// Years <= 0 error.
	if res := Calc(MtgLine{YearsStatus: inp(), Years: 0}); res.Err == nil {
		t.Error("Years=0 should error")
	}
	// HowMuch filled but When blank -> error.
	if res := Calc(MtgLine{HowMuchStatus: inp(), HowMuch: 1000,
		PriceStatus: inp(), Price: 100000}); res.Err == nil {
		t.Error("balloon amt without yrs should error")
	}
	// Price + Monthly both input, no balloon -> "nothing to solve" error.
	full := computedBasic(0.06, 30)
	over := MtgLine{
		PriceStatus: inp(), Price: 200000,
		PctStatus: inp(), Pct: 0.20,
		YearsStatus: inp(), Years: 30,
		RateStatus: inp(), Rate: 0.06,
		MonthlyStatus: inp(), Monthly: full.Monthly,
		TaxStatus:   inp(),
		BalloonStat: types.BalloonBlank,
	}
	if res := Calc(over); res.Err == nil {
		t.Error("price+monthly+no balloon should error")
	}
	// Balloon unknown (price+monthly+when, howmuch blank) -> balloonCalc.
	bUnk := over
	bUnk.WhenStatus = inp()
	bUnk.When = 15
	if res := Calc(bUnk); res.Err != nil || res.Line.HowMuchStatus != types.InOutOutput {
		t.Errorf("balloon-unknown solve failed: err=%v", res.Err)
	}
	// Balloon known: compute monthly.
	if res := Calc(MtgLine{
		PriceStatus: inp(), Price: 200000, PctStatus: inp(), Pct: 0.2,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: 0.06,
		TaxStatus: inp(), WhenStatus: inp(), When: 10,
		HowMuchStatus: inp(), HowMuch: 50000,
	}); res.Err != nil || res.Line.MonthlyStatus != types.InOutOutput {
		t.Errorf("balloon-known monthly solve failed: err=%v", res.Err)
	}
	// Monthly -> Price via Cash.
	if res := Calc(MtgLine{
		MonthlyStatus: inp(), Monthly: full.Monthly,
		CashStatus: inp(), Cash: 40000,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: 0.06,
		TaxStatus: inp(), BalloonStat: types.BalloonBlank,
	}); res.Err != nil || res.Line.PriceStatus != types.InOutOutput {
		t.Errorf("monthly->price via cash failed: err=%v", res.Err)
	}
	// Monthly -> Price with neither pct nor cash -> error.
	if res := Calc(MtgLine{
		MonthlyStatus: inp(), Monthly: 1000,
		FinancedStatus: inp(), Financed: 160000,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: 0.06,
		TaxStatus: inp(), BalloonStat: types.BalloonBlank,
	}); res.Err == nil {
		t.Error("monthly->price without pct/cash should error")
	}
}

func TestCalc_WarningsAndComputeBranches(t *testing.T) {
	// Financed > Price warning.
	res := Calc(MtgLine{
		PriceStatus: inp(), Price: 100000,
		FinancedStatus: inp(), Financed: 120000,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: 0.06,
		TaxStatus: inp(), BalloonStat: types.BalloonBlank,
	})
	if len(res.Warnings) == 0 {
		t.Error("expected financed>price warning")
	}
	// Unusually-high rate warning.
	res2 := Calc(MtgLine{
		PriceStatus: inp(), Price: 200000, PctStatus: inp(), Pct: 0.2,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: 0.30,
		TaxStatus: inp(), BalloonStat: types.BalloonBlank,
	})
	if len(res2.Warnings) == 0 {
		t.Error("expected high-rate warning")
	}
	// computeCashPctAndFinanced via Financed (Pct near 1 -> error).
	if res := Calc(MtgLine{
		PriceStatus: inp(), Price: 200000,
		FinancedStatus: inp(), Financed: 100,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: 0.06,
		TaxStatus: inp(), BalloonStat: types.BalloonBlank,
	}); res.Err == nil {
		t.Error("tiny financed should error (pct rounds to 100%)")
	}
	// Cash near price -> error.
	if res := Calc(MtgLine{
		PriceStatus: inp(), Price: 200000,
		CashStatus: inp(), Cash: 199900,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: 0.06,
		TaxStatus: inp(), BalloonStat: types.BalloonBlank,
	}); res.Err == nil {
		t.Error("cash near price should error")
	}
	// Price = 0 -> error.
	if res := Calc(MtgLine{
		PriceStatus: inp(), Price: 0, PctStatus: inp(), Pct: 0.2,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: 0.06,
		TaxStatus: inp(),
	}); res.Err == nil {
		t.Error("price=0 should error")
	}
}

func TestSummation_Branches(t *testing.T) {
	// Zero rate -> 12*t.
	if v, err := Summation(0, 10); err != nil || v != 120 {
		t.Errorf("Summation(0,10) = %v, %v; want 120", v, err)
	}
	// Normal.
	if v, err := Summation(0.06, 30); err != nil || v <= 0 {
		t.Errorf("Summation normal = %v, %v", v, err)
	}
}

func TestAPRHelpers(t *testing.T) {
	b := computedBalloon(0.06, 30, 10, 50000)
	// TerminalBalloon with balloon active (When <= t).
	if _, err := TerminalBalloon(&b, 12); err != nil {
		t.Errorf("TerminalBalloon: %v", err)
	}
	// ValueOfPaymentsForTerminatedLoan: When < t and t <= Years terminal branch.
	if _, err := ValueOfPaymentsForTerminatedLoan(&b, 0.06, 15); err != nil {
		t.Errorf("ValueOfPaymentsForTerminatedLoan: %v", err)
	}
	// OneMonthAPR.
	if _, err := OneMonthAPR(&b, yrdays360); err != nil {
		t.Errorf("OneMonthAPR: %v", err)
	}
	// FullTermAPR.
	if _, conv, err := FullTermAPR(b, yrdays360); err != nil || !conv {
		t.Errorf("FullTermAPR: conv=%v err=%v", conv, err)
	}
}

func TestCompareAPRs_Paths(t *testing.T) {
	// Not enough data.
	if _, err := CompareAPRs(MtgLine{}, computedBasic(0.06, 30), yrdays360); err == nil {
		t.Error("CompareAPRs with empty mortgage A should error")
	}
	if _, err := CompareAPRs(computedBasic(0.06, 30), MtgLine{}, yrdays360); err == nil {
		t.Error("CompareAPRs with empty mortgage B should error")
	}
	// Mortgage 1 always better (lower rate, no points).
	a := computedBasic(0.05, 30)
	bb := computedBasic(0.09, 30)
	r, err := CompareAPRs(a, bb, yrdays360)
	if err != nil || r.Summary == "" {
		t.Errorf("CompareAPRs always-better: %v %q", err, r.Summary)
	}
	// Reverse (mortgage 2 better).
	r2, err := CompareAPRs(bb, a, yrdays360)
	if err != nil || r2.Summary == "" {
		t.Errorf("CompareAPRs reverse: %v %q", err, r2.Summary)
	}
	// Points-vs-rate tradeoff to exercise the crossover search:
	// A: lower rate but points; B: higher rate, no points.
	withPoints := computedBasic(0.05, 30)
	withPoints.Points = 0.03
	noPoints := computedBasic(0.065, 30)
	if _, err := CompareAPRs(withPoints, noPoints, yrdays360); err != nil {
		t.Errorf("CompareAPRs crossover search: %v", err)
	}
	// Balloon-carrying pair to exercise tryBalloonDates fallback.
	bal1 := computedBalloon(0.05, 30, 7, 80000)
	bal1.Points = 0.04
	bal2 := computedBalloon(0.07, 30, 7, 20000)
	if _, err := CompareAPRs(bal1, bal2, yrdays360); err != nil {
		t.Errorf("CompareAPRs balloon pair: %v", err)
	}
}

func TestRowGeneration(t *testing.T) {
	base := computedBasic(0.06, 30) // monthly is the computed output
	// Error paths.
	if _, err := GenerateRows(base, VaryRate, 0.0025, 0); err == nil {
		t.Error("n=0 should error")
	}
	if _, err := GenerateRows(base, VaryRate, 0.0025, MaxWhatIfRows+1); err == nil {
		t.Error("n too large should error")
	}
	if _, err := GenerateRows(base, VaryNone, 0.0025, 3); err == nil {
		t.Error("VaryNone should error")
	}
	// Base with no computed output -> error.
	noOut := MtgLine{PriceStatus: inp(), Price: 200000, MonthlyStatus: inp(), Monthly: 1200}
	if _, err := GenerateRows(noOut, VaryRate, 0.0025, 2); err == nil {
		t.Error("base without an output field should error")
	}
	// Each vary field.
	for _, v := range []VaryField{VaryRate, VaryYears, VaryPoints, VaryPctDown, VaryPrice, VaryMonthly} {
		if _, err := GenerateRows(base, v, 0.0, 2); err != nil {
			t.Errorf("GenerateRows vary=%d: %v", v, err)
		}
	}
	// bumpField unknown vary.
	m := base
	if err := bumpField(&m, VaryField(99), 1); err == nil {
		t.Error("unknown vary field should error")
	}
	// EnoughDataForRowGeneration.
	if !EnoughDataForRowGeneration(&base) {
		t.Error("computed base should have enough data")
	}
	if EnoughDataForRowGeneration(&MtgLine{}) {
		t.Error("empty should not have enough data")
	}
	// GenerateGrid 1-D (vary2 None) and 2-D.
	if _, err := GenerateGrid(base, VaryRate, 0.0025, 2, VaryNone, 0, 0); err != nil {
		t.Errorf("GenerateGrid 1-D: %v", err)
	}
	if _, err := GenerateGrid(base, VaryRate, 0.0025, 2, VaryYears, 1, 2); err != nil {
		t.Errorf("GenerateGrid 2-D: %v", err)
	}
	if _, err := GenerateGrid(base, VaryRate, 0.0025, 2, VaryYears, 1, MaxWhatIfRows+1); err == nil {
		t.Error("GenerateGrid count2 too large should error")
	}
}
