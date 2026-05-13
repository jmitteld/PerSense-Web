// help_examples_test.go: exercises Mortgage with the worked examples
// in legacy/src/win_source/Help/MS_EX*.html. Each subtest names its
// source file and quotes the expected value. Tolerances are loose
// (≈$0.50 absolute or ~0.05% relative) to absorb the help docs'
// penny-rounded display vs. the Go port's float64 precision.
//
// This file was added by the test-matrix exercise to assess
// correctness of mortgage Calc, APR comparison, and balloon paths
// against the canonical help worked examples.
package mortgage

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// closeEnough returns true if got matches want within max(absTol,
// relTol * |want|).
func closeEnough(got, want, absTol, relTol float64) bool {
	tol := absTol
	if r := relTol * math.Abs(want); r > tol {
		tol = r
	}
	return math.Abs(got-want) <= tol
}

// mortgageHelpRow builds an MtgLine using the user-facing rate
// convention (loan rate / nominal monthly-compounded), matching how
// the rates appear in the help docs. Calc operates on the internal
// continuously-compounded "true rate", so the row's Rate field is
// converted here.
func mortgageHelpRow(price, pts, pctDown, loanRate, tax float64, yrs int) MtgLine {
	return MtgLine{
		PriceStatus:  types.InOutInput,
		Price:        price,
		PointsStatus: types.InOutInput,
		Points:       pts,
		PctStatus:    types.InOutInput,
		Pct:          pctDown,
		YearsStatus:  types.InOutInput,
		Years:        yrs,
		RateStatus:   types.InOutInput,
		Rate:         LoanRateToTrueRate(loanRate),
		TaxStatus:    types.InOutInput,
		Tax:          tax,
		BalloonStat:  types.BalloonBlank,
	}
}

// M01 - MS_EX1: forward monthly on a 20yr 8% $200K @ 20% down + 2pts.
// Help table prints Monthly Total = 1,538.30.
func TestHelpMS_EX1_ForwardMonthly(t *testing.T) {
	m := mortgageHelpRow(200_000, 0.02, 0.20, 0.08, 200, 20)
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("Calc error: %v", r.Err)
	}
	t.Logf("MS_EX1 → Cash=%.2f Financed=%.2f Monthly=%.2f",
		r.Line.Cash, r.Line.Financed, r.Line.Monthly)
	const wantMonthly = 1538.30
	if !closeEnough(r.Line.Monthly, wantMonthly, 0.50, 0.001) {
		t.Errorf("monthly = %.4f, help says %.2f", r.Line.Monthly, wantMonthly)
	}
	if !closeEnough(r.Line.Financed, 160_000, 1, 0) {
		t.Errorf("financed = %.2f, want 160,000", r.Line.Financed)
	}
	// Cash = down (40,000) + points charge (0.02 * 160,000 = 3,200) = 43,200
	if !closeEnough(r.Line.Cash, 43_200, 1, 0) {
		t.Errorf("cash = %.2f, want 43,200", r.Line.Cash)
	}
}

// M02 - MS_EX2: solve for Price given Cash Required (instead of %Down)
// and a $1,650 monthly budget on a 30yr 8.5% with 1.5 points + $200 tax.
// Help table prints Price = 241,749.12.
func TestHelpMS_EX2_SolvePrice(t *testing.T) {
	m := MtgLine{
		PointsStatus:  types.InOutInput,
		Points:        0.015,
		CashStatus:    types.InOutInput,
		Cash:          56_000,
		YearsStatus:   types.InOutInput,
		Years:         30,
		RateStatus:    types.InOutInput,
		Rate:          LoanRateToTrueRate(0.085),
		TaxStatus:     types.InOutInput,
		Tax:           200,
		MonthlyStatus: types.InOutInput,
		Monthly:       1_650,
		BalloonStat:   types.BalloonBlank,
	}
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("Calc error: %v", r.Err)
	}
	t.Logf("MS_EX2 → Price=%.2f Financed=%.2f Pct=%.4f",
		r.Line.Price, r.Line.Financed, r.Line.Pct)
	const wantPrice = 241_749.12
	if !closeEnough(r.Line.Price, wantPrice, 1.0, 0.001) {
		t.Errorf("price = %.4f, help says %.2f", r.Line.Price, wantPrice)
	}
	if !closeEnough(r.Line.Financed, 188_577.83, 1.0, 0.001) {
		t.Errorf("financed = %.4f, help says 188,577.83", r.Line.Financed)
	}
}

// M03 - MS_EX3 row 1: forward with no balloon, $280K @ 20% down + 2.5pts,
// 30yr 8.25% + $300 tax. Help table prints Monthly Total = 1,982.84.
func TestHelpMS_EX3_Row1_NoBalloon(t *testing.T) {
	m := mortgageHelpRow(280_000, 0.025, 0.20, 0.0825, 300, 30)
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("Calc error: %v", r.Err)
	}
	t.Logf("MS_EX3r1 → Cash=%.2f Financed=%.2f Monthly=%.2f",
		r.Line.Cash, r.Line.Financed, r.Line.Monthly)
	const wantMonthly = 1_982.84
	if !closeEnough(r.Line.Monthly, wantMonthly, 0.50, 0.001) {
		t.Errorf("monthly = %.4f, help says %.2f", r.Line.Monthly, wantMonthly)
	}
}

// M04 - MS_EX3 row 2: same loan + 8-yr balloon, payment hardened to
// $1,600 → solve balloon amount. Help prints balloon = 98,372.
func TestHelpMS_EX3_Row2_SolveBalloon(t *testing.T) {
	m := MtgLine{
		PriceStatus:   types.InOutInput,
		Price:         280_000,
		PointsStatus:  types.InOutInput,
		Points:        0.025,
		PctStatus:     types.InOutInput,
		Pct:           0.20,
		YearsStatus:   types.InOutInput,
		Years:         30,
		RateStatus:    types.InOutInput,
		Rate:          LoanRateToTrueRate(0.0825),
		TaxStatus:     types.InOutInput,
		Tax:           300,
		MonthlyStatus: types.InOutInput,
		Monthly:       1_600,
		WhenStatus:    types.InOutInput,
		When:          8,
		// HowMuchStatus left empty → balloon amount is the unknown
	}
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("Calc error: %v", r.Err)
	}
	t.Logf("MS_EX3r2 → BalloonAmount=%.2f (year %d)", r.Line.HowMuch, r.Line.When)
	const wantBalloon = 98_372.0
	if !closeEnough(r.Line.HowMuch, wantBalloon, 5.0, 0.005) {
		t.Errorf("balloon = %.4f, help says %.0f", r.Line.HowMuch, wantBalloon)
	}
}

// M05 - MS_EX3 row 3: same loan + 8-yr balloon hardened to $100K →
// solve monthly. Help prints Monthly Total = 1,593.67.
func TestHelpMS_EX3_Row3_MonthlyWithBalloon(t *testing.T) {
	m := mortgageHelpRow(280_000, 0.025, 0.20, 0.0825, 300, 30)
	m.WhenStatus = types.InOutInput
	m.When = 8
	m.HowMuchStatus = types.InOutInput
	m.HowMuch = 100_000
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("Calc error: %v", r.Err)
	}
	t.Logf("MS_EX3r3 → Monthly=%.2f", r.Line.Monthly)
	const wantMonthly = 1_593.67
	if !closeEnough(r.Line.Monthly, wantMonthly, 0.50, 0.001) {
		t.Errorf("monthly = %.4f, help says %.2f", r.Line.Monthly, wantMonthly)
	}
}

// M06 - MS_EX5: APR comparison between low-points vs low-rate. Help
// prints: A APR=8.4257, B APR=8.6094, cross at 8.6984 / 6yr 10mo.
func TestHelpMS_EX5_APRComparison(t *testing.T) {
	// First Calc each mortgage so Monthly is populated. The rate
	// argument is the user-facing loan rate; convert at the edge.
	mkRow := func(pts, loanRate float64) MtgLine {
		m := MtgLine{
			PriceStatus:    types.InOutInput,
			Price:          10_000,
			PointsStatus:   types.InOutInput,
			Points:         pts,
			PctStatus:      types.InOutInput,
			Pct:            0,
			YearsStatus:    types.InOutInput,
			Years:          30,
			RateStatus:     types.InOutInput,
			Rate:           LoanRateToTrueRate(loanRate),
			TaxStatus:      types.InOutInput,
			Tax:            0,
			BalloonStat:    types.BalloonBlank,
		}
		r := Calc(m)
		if r.Err != nil {
			t.Fatalf("Calc error: %v", r.Err)
		}
		return r.Line
	}
	a := mkRow(0.03, 0.081)
	b := mkRow(0.01, 0.085)
	t.Logf("MS_EX5 inputs → A Monthly=%.2f (help 74.07), B Monthly=%.2f (help 76.89)",
		a.Monthly, b.Monthly)
	cmp, err := CompareAPRs(a, b, 360)
	if err != nil {
		t.Fatalf("CompareAPRs error: %v", err)
	}
	// FullTermAPR and the crossover iterator both convert their
	// internal true-rate iterates to loan rates via YieldFromRate
	// before returning, so cmp.APR1 / APR2 / CrossoverAPR are
	// already in the help docs' display convention.
	aprA := 100 * cmp.APR1
	aprB := 100 * cmp.APR2
	aprX := 100 * cmp.CrossoverAPR
	t.Logf("MS_EX5 → APR_A=%.4f%%  APR_B=%.4f%%  cross=%.4f%% at t=%.4f yr",
		aprA, aprB, aprX, cmp.CrossoverTime)
	t.Logf("  summary: %s", cmp.Summary)
	if !closeEnough(aprA, 8.4257, 0.01, 0.002) {
		t.Errorf("APR_A = %.4f%%, help says 8.4257%%", aprA)
	}
	if !closeEnough(aprB, 8.6094, 0.01, 0.002) {
		t.Errorf("APR_B = %.4f%%, help says 8.6094%%", aprB)
	}
	if !closeEnough(aprX, 8.6984, 0.05, 0.005) {
		t.Errorf("crossover = %.4f%%, help says 8.6984%%", aprX)
	}
	// 6 years, 10 months = 6.8333...
	if !closeEnough(cmp.CrossoverTime, 6+10.0/12, 0.05, 0.01) {
		t.Errorf("crossover time = %.4f yr, help says ~6.833 yr", cmp.CrossoverTime)
	}
}

// M07 - Round-trip: forward-compute monthly (EX1), then solve for
// Price using the computed Monthly. The original Price should be
// recovered to within a dollar.
func TestHelpMS_EX1_RoundTripPriceFromMonthly(t *testing.T) {
	fwd := mortgageHelpRow(200_000, 0.02, 0.20, 0.08, 200, 20)
	r1 := Calc(fwd)
	if r1.Err != nil {
		t.Fatalf("forward Calc error: %v", r1.Err)
	}
	rev := MtgLine{
		PointsStatus:  types.InOutInput,
		Points:        0.02,
		PctStatus:     types.InOutInput,
		Pct:           0.20,
		YearsStatus:   types.InOutInput,
		Years:         20,
		RateStatus:    types.InOutInput,
		Rate:          LoanRateToTrueRate(0.08),
		TaxStatus:     types.InOutInput,
		Tax:           200,
		MonthlyStatus: types.InOutInput,
		Monthly:       r1.Line.Monthly,
		BalloonStat:   types.BalloonBlank,
	}
	r2 := Calc(rev)
	if r2.Err != nil {
		t.Fatalf("reverse Calc error: %v", r2.Err)
	}
	t.Logf("MS_EX1 round-trip → Price=%.4f (want 200,000)", r2.Line.Price)
	if !closeEnough(r2.Line.Price, 200_000, 0.5, 0) {
		t.Errorf("round-trip price = %.4f, want 200,000", r2.Line.Price)
	}
}

// M08 - MS_EX4 step 2: zero points, zero down, monthly hardened from
// the 30yr at 8.1% on $240K (=1,777.79), term shortened to 15 yr,
// balloon at year 15 → balloon ≈ 184,912 per help.
func TestHelpMS_EX4_BalloonAtTermEnd(t *testing.T) {
	// First compute the 30-yr monthly.
	step1 := MtgLine{
		PriceStatus:    types.InOutInput,
		Price:          240_000,
		PointsStatus:   types.InOutInput,
		Points:         0,
		PctStatus:      types.InOutInput,
		Pct:            0,
		YearsStatus:    types.InOutInput,
		Years:          30,
		RateStatus:     types.InOutInput,
		Rate:           LoanRateToTrueRate(0.081),
		TaxStatus:      types.InOutInput,
		Tax:            0,
		BalloonStat:    types.BalloonBlank,
	}
	r1 := Calc(step1)
	if r1.Err != nil {
		t.Fatalf("step1 Calc: %v", r1.Err)
	}
	t.Logf("MS_EX4 step1 → Monthly=%.2f (help 1,777.79)", r1.Line.Monthly)

	// Step 2: re-amortize at 15 yr with hardened payment, balloon at yr 15.
	step2 := MtgLine{
		PriceStatus:   types.InOutInput,
		Price:         240_000,
		PointsStatus:  types.InOutInput,
		Points:        0,
		PctStatus:     types.InOutInput,
		Pct:           0,
		YearsStatus:   types.InOutInput,
		Years:         15,
		RateStatus:    types.InOutInput,
		Rate:          LoanRateToTrueRate(0.081),
		TaxStatus:     types.InOutInput,
		Tax:           0,
		MonthlyStatus: types.InOutInput,
		Monthly:       r1.Line.Monthly,
		WhenStatus:    types.InOutInput,
		When:          15,
		// HowMuch left empty → solve.
	}
	r2 := Calc(step2)
	if r2.Err != nil {
		t.Fatalf("step2 Calc: %v", r2.Err)
	}
	t.Logf("MS_EX4 step2 → Balloon=%.2f (help ≈184,912)", r2.Line.HowMuch)
	if !closeEnough(r2.Line.HowMuch, 184_912, 50, 0.005) {
		t.Errorf("balloon = %.4f, help says ≈184,912", r2.Line.HowMuch)
	}
}
