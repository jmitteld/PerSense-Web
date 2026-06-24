package mortgage

import (
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// withPointsMtg returns a solved standard mortgage with points added.
func withPointsMtg(rate float64, years int, points float64) MtgLine {
	m := computedBasic(rate, years)
	m.Points = points
	return m
}

func TestCompareAPRs_CrossoverIteration(t *testing.T) {
	// Genuine APR crossover requires a SMALL rate gap paired with points, so
	// that the lower-rate mortgage's points make it worse short-term but better
	// long-term. These (rate1,points1,rate2,points2) tuples were verified to
	// produce a "cross" verdict, exercising the 2-D Newton iteration body.
	cases := []struct{ r1, p1, r2, p2 float64 }{
		{0.030, 0.05, 0.035, 0.00}, // crosses ~15.6 yrs
		{0.030, 0.02, 0.040, 0.00}, // crosses ~2.1 yrs
		{0.030, 0.08, 0.035, 0.05}, // crosses ~7.7 yrs
		{0.030, 0.05, 0.040, 0.02}, // crosses ~3.3 yrs
	}
	crossed := false
	for _, c := range cases {
		a := computedBasic(c.r1, 30)
		a.Points = c.p1
		b := computedBasic(c.r2, 30)
		b.Points = c.p2
		res, err := CompareAPRs(a, b, yrdays360)
		if err != nil {
			t.Fatalf("CompareAPRs %+v: %v", c, err)
		}
		if res.CrossoverTime > 0 {
			crossed = true
		}
		// Reverse order to exercise the mirror "Mortgage 2 is better" branch.
		if _, err := CompareAPRs(b, a, yrdays360); err != nil {
			t.Fatalf("CompareAPRs reverse %+v: %v", c, err)
		}
	}
	if !crossed {
		t.Error("expected at least one crossover among the verified cases")
	}
}

func TestCompareAPRs_BalloonCrossoverFallback(t *testing.T) {
	// Crossing balloon mortgages: A higher rate / no points, B lower rate /
	// points, both with a balloon, so the crossover sits near the balloon
	// date and the tryBalloonDates fallback runs.
	a := computedBalloon(0.07, 30, 7, 20000)
	b := computedBalloon(0.05, 30, 7, 90000)
	b.Points = 0.06
	if _, err := CompareAPRs(a, b, yrdays360); err != nil {
		t.Fatalf("CompareAPRs balloon fallback: %v", err)
	}
	// Balloon only on the second mortgage.
	c := computedBasic(0.07, 30)
	dloan := computedBalloon(0.05, 30, 5, 80000)
	dloan.Points = 0.05
	if _, err := CompareAPRs(c, dloan, yrdays360); err != nil {
		t.Fatalf("CompareAPRs second-balloon fallback: %v", err)
	}
}

func TestTryBalloonDates_Flip(t *testing.T) {
	// Verified inputs where the APR ordering flips across the balloon date, so
	// tryBalloonDates resolves the crossover to the balloon date itself.
	e1 := computedBalloon(0.05, 30, 3, 10000)
	e2 := computedBalloon(0.03, 30, 3, 5000)
	e2.Points = 0.04
	apr, tt, ok := tryBalloonDates(e1, e2, yrdays360)
	if !ok {
		t.Fatal("expected tryBalloonDates to detect a flip")
	}
	if tt != 3 || apr < 0 {
		t.Errorf("unexpected balloon-date crossover: apr=%v t=%v", apr, tt)
	}
	// Symmetric case: balloon flip detected via the second mortgage.
	if _, _, ok := tryBalloonDates(e2, e1, yrdays360); !ok {
		t.Error("expected flip in the mirrored argument order too")
	}
	// First mortgage has NO balloon, second does and flips: exercises the
	// second (e2) balloon branch of tryBalloonDates.
	noBalloon := computedBasic(0.03, 30)
	balloon := computedBalloon(0.03, 30, 3, 10000)
	balloon.Points = 0.04
	if _, _, ok := tryBalloonDates(noBalloon, balloon, yrdays360); !ok {
		t.Error("expected flip via the second mortgage's balloon")
	}
}

func TestCrossover_InIterationBalloonFallback(t *testing.T) {
	// Verified pair where the 2-D Newton stalls and the in-iteration balloon
	// fallback resolves the crossover onto the balloon date.
	a := computedBalloon(0.03, 30, 2, 20000)
	a.Points = 0.05
	b := computedBalloon(0.03, 30, 2, 6000)
	apr, tm, found, err := iterateToFindCrossoverAPRandTime(a, b, yrdays360)
	if err != nil {
		t.Fatalf("iterateToFindCrossoverAPRandTime: %v", err)
	}
	if !found || tm != 2 {
		t.Errorf("expected balloon-date fallback crossover at t=2, got found=%v t=%v apr=%v", found, tm, apr)
	}
}

func TestCompareAPRs_NonConvergeAndBalloonFallback(t *testing.T) {
	// Identical mortgages never cross -> the crossover search runs to its
	// iteration limit and reports non-convergence.
	a := computedBasic(0.03, 30)
	res, err := CompareAPRs(a, a, yrdays360)
	if err != nil {
		t.Fatalf("CompareAPRs identical: %v", err)
	}
	if res.Summary != "Crossover computation did not converge." {
		t.Errorf("expected non-converge summary, got %q", res.Summary)
	}
	// Balloon pair that flips at the balloon date: CompareAPRs falls through
	// the stalled 2-D iteration into the tryBalloonDates fallback.
	b1 := computedBalloon(0.05, 30, 3, 10000)
	b2 := computedBalloon(0.03, 30, 3, 5000)
	b2.Points = 0.04
	if _, err := CompareAPRs(b1, b2, yrdays360); err != nil {
		t.Fatalf("CompareAPRs balloon flip: %v", err)
	}
}

func TestRowGen_ReachableErrorBranches(t *testing.T) {
	base := computedBasic(0.06, 30) // Monthly is the output

	// bumpField error inside GenerateRows (invalid vary passes the VaryNone
	// guard, then bumpField rejects it on row 2).
	if _, err := GenerateRows(base, VaryField(99), 1, 2); err == nil {
		t.Error("GenerateRows with invalid vary should error via bumpField")
	}

	// Base whose OUTPUT is the balloon (HowMuch) -> the HowMuch case in the
	// re-solve switch.
	balloonOut := Calc(MtgLine{
		PriceStatus: inp(), Price: 200000, PctStatus: inp(), Pct: 0.2,
		MonthlyStatus: inp(), Monthly: 1300,
		YearsStatus: inp(), Years: 30, RateStatus: inp(), Rate: 0.06,
		TaxStatus: inp(), WhenStatus: inp(), When: 15,
	}).Line
	if balloonOut.HowMuchStatus == types.InOutOutput {
		if _, err := GenerateRows(balloonOut, VaryRate, 0.0025, 2); err != nil {
			t.Errorf("GenerateRows balloon-output base: %v", err)
		}
	}

	// A generated row that fails Calc: varying Years downward past zero makes a
	// later row's Years non-positive, which Calc rejects — exercising the
	// per-row error return inside GenerateRows.
	if _, err := GenerateRows(base, VaryYears, -40, 2); err == nil {
		t.Error("GenerateRows driving Years negative should surface a Calc error")
	}

	// GenerateGrid: GenerateRows error in the 1-D collapse path.
	if _, err := GenerateGrid(base, VaryNone, 0, 0, VaryNone, 0, 0); err == nil {
		t.Error("GenerateGrid 1-D with VaryNone should error")
	}
	// GenerateGrid: bumpField error on the secondary axis.
	if _, err := GenerateGrid(base, VaryRate, 0.0025, 2, VaryField(99), 1, 2); err == nil {
		t.Error("GenerateGrid invalid vary2 should error")
	}
	// GenerateGrid: Calc error on a secondary-axis base (drive Years negative).
	if _, err := GenerateGrid(base, VaryRate, 0.0025, 2, VaryYears, -40, 2); err == nil {
		t.Error("GenerateGrid secondary base Calc error should propagate")
	}
	// GenerateGrid: inner GenerateRows error in the 2-D loop (invalid vary1).
	if _, err := GenerateGrid(base, VaryField(99), 1, 2, VaryYears, 1, 2); err == nil {
		t.Error("GenerateGrid invalid vary1 in 2-D loop should error")
	}
}
