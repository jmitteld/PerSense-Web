package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// Canonical (first-principles) validation of the life-contingency engine.
//
// The DOS ACTUARY unit source is missing and life-contingency was never
// compiled into the shipped builds (see docs/fidelity_validation_roadmap.md
// §0), so there is no DOS output to diff against. These tests therefore
// validate the engine against *standard actuarial mathematics*: pure
// endowment, the life annuity, term-insurance / Payment-on-Death, life
// expectancy, and the two-life survival composition. Each "expected"
// value is computed here by an independent brute-force method derived
// from the textbook definition (lx built directly from qx; explicit
// summation), NOT by calling the engine's own LifeProb / discount /
// summation helpers. This makes the comparison a genuine independent
// oracle for the part that integrates survival with present value — the
// exact area where bugs have lived.
//
// Conventions mirrored from the engine: continuous discounting
// exp(-rate·Δyears); 30/360 year fractions; survival measured relative
// to the as-of ("now") age. Round-number dates keep ages integral so lx
// is read without interpolation.

// canonQx reproduces the mock mortality curve actuarialTestCfg uses, so
// the independently-built lx below matches the engine's table exactly.
func canonQx() []float64 {
	qx := make([]float64, 121)
	for i := range qx {
		qx[i] = 0.001 + 0.0001*float64(i)*float64(i)/120.0
		if qx[i] > 1 {
			qx[i] = 1
		}
	}
	qx[120] = 1
	return qx
}

// canonLx builds lx from qx by the textbook recursion lx0=100000,
// lx[i+1]=lx[i]*(1-qx[i]) — independent of the engine's table code.
func canonLx(qx []float64) []float64 {
	lx := make([]float64, len(qx)+1)
	lx[0] = 100000
	for i := range qx {
		lx[i+1] = lx[i] * (1 - qx[i])
	}
	return lx
}

// TestCanonicalPureEndowment: a life-contingent (Living) lump of F due in
// n years is the pure endowment  F · v^n · npx, where v^n = exp(-rate·n)
// and npx = lx[x+n]/lx[x]. Validates the lump survival-weighting +
// discounting integration.
func TestCanonicalPureEndowment(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1) // age 70 at as-of
	cfg := actuarialTestCfg(asOf, dob)
	lx := canonLx(canonQx())
	rate := 0.06
	const F = 100000.0
	x := 70 // age now

	for _, n := range []int{1, 5, 10, 20} {
		res := Calculate(PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: rate},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: dateOf(2026+n, time.January, 1),
				AmtStatus: types.InOutInput, Amt: F,
				Act: actuarial.Living,
			}},
			Actuarial: cfg,
		})
		if res.Err != nil {
			t.Fatalf("n=%d: %v", n, res.Err)
		}
		want := F * math.Exp(-rate*float64(n)) * lx[x+n] / lx[x]
		if rel := math.Abs(res.SumValue-want) / want; rel > 1e-9 {
			t.Errorf("n=%d: pure endowment engine=%.6f want=%.6f (rel %.2e)", n, res.SumValue, want, rel)
		}
	}
}

// TestCanonicalLifeAnnuity: an n-year temporary life annuity-due paying P
// at ages x, x+1, … has present value  P · Σ_k v^k · kpx. Validates the
// periodic survival-weighting + per-payment discounting + summation.
func TestCanonicalLifeAnnuity(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1) // age 70
	cfg := actuarialTestCfg(asOf, dob)
	lx := canonLx(canonQx())
	rate := 0.06
	const P = 12000.0
	x := 70
	fromYear, toYear := 2030, 2045 // ages 74..89, annual payments

	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: rate},
		},
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(fromYear, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(toYear, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 1, // annual
			AmtStatus: types.InOutInput, Amt: P,
			Act: actuarial.Living,
		}},
		Actuarial: cfg,
	})
	if res.Err != nil {
		t.Fatalf("annuity: %v", res.Err)
	}

	// Independent sum over payment dates from fromYear..toYear inclusive.
	want := 0.0
	for y := fromYear; y <= toYear; y++ {
		k := y - 2026                // years from as-of to this payment
		ageAt := y - dob.Time.Year() // integer age at payment
		want += P * math.Exp(-rate*float64(k)) * lx[ageAt] / lx[x]
	}
	if rel := math.Abs(res.SumValue-want) / want; rel > 1e-6 {
		t.Errorf("life annuity engine=%.6f want=%.6f (rel %.2e)", res.SumValue, want, rel)
	}
}

// TestCanonicalTwoLifeAnnuityPV drives a TWO-LIFE contingent periodic stream
// through the real Calculate (the production path: periodicWithActuarial folds
// the joint / last-survivor survival into each period's discount) and validates
// it against an independent two-life annuity sum. This closes the gap left by
// TestCanonicalTwoLifeComposition, which only checked LifeProb at the
// probability level, not folded through the PV discounting.
func TestCanonicalTwoLifeAnnuityPV(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob1 := dateOf(1956, time.January, 1) // age 70
	dob2 := dateOf(1961, time.January, 1) // age 65
	cfg := twoLifeCfg(asOf, dob1, dob2)

	lx1 := canonLx(canonQx())
	qx2 := make([]float64, 121)
	for i := range qx2 {
		qx2[i] = 0.002 + 0.00025*float64(i)*float64(i)/120.0
		if qx2[i] > 1 {
			qx2[i] = 1
		}
	}
	qx2[120] = 1
	lx2 := canonLx(qx2)

	x1, x2 := 70, 65
	rate := 0.06
	const P = 12000.0
	fromYear, toYear := 2030, 2050

	run := func(act byte, surv func(s1, s2 float64) float64) {
		res := Calculate(PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: rate},
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(fromYear, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(toYear, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 1,
				AmtStatus: types.InOutInput, Amt: P,
				Act: act,
			}},
			Actuarial: cfg,
		})
		if res.Err != nil {
			t.Fatalf("%s: %v", actuarial.ContingencyLabel(act), res.Err)
		}
		want := 0.0
		for y := fromYear; y <= toYear; y++ {
			k := y - 2026
			s1 := lx1[y-dob1.Time.Year()] / lx1[x1]
			s2 := lx2[y-dob2.Time.Year()] / lx2[x2]
			want += P * math.Exp(-rate*float64(k)) * surv(s1, s2)
		}
		if rel := math.Abs(res.SumValue-want) / want; rel > 1e-6 {
			t.Errorf("%s annuity engine=%.6f want=%.6f (rel %.2e)",
				actuarial.ContingencyLabel(act), res.SumValue, want, rel)
		}
	}

	run(actuarial.BothLiving, func(s1, s2 float64) float64 { return s1 * s2 })
	run(actuarial.EitherLiving, func(s1, s2 float64) float64 { return 1 - (1-s1)*(1-s2) })
	run(actuarial.Only1Living, func(s1, s2 float64) float64 { return s1 * (1 - s2) })
	run(actuarial.Only2Living, func(s1, s2 float64) float64 { return (1 - s1) * s2 })
}

// TestCanonicalPaymentOnDeath: the Payment-on-Death present value is the
// expected discounted death benefit  B · Σ_k (k|qx) · v^(k+0.5), where
// (k|qx) = (lx[x+k]-lx[x+k+1])/lx[x] is the deferred mortality
// probability and the benefit is discounted to the mid-point of the
// death year. Validates the POD term-insurance valuation.
func TestCanonicalPaymentOnDeath(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1) // age 70
	cfg := actuarialTestCfg(asOf, dob)
	cfg.POD = 250000
	lx := canonLx(canonQx())
	rate := 0.06
	x := 70
	maxAge := len(lx) - 1

	got := cfg.PODValue(asOf, rate)

	want := 0.0
	for k := 0; x+k < maxAge; k++ {
		dq := (lx[x+k] - lx[x+k+1]) / lx[x]
		if dq <= 0 {
			continue
		}
		want += cfg.POD * dq * math.Exp(-rate*(float64(k)+0.5))
	}
	// The engine rounds POD value to cents (interest.Round2); compare
	// within a cent.
	if math.Abs(got-want) > 0.01 {
		t.Errorf("POD value engine=%.4f want=%.4f (diff %.4f)", got, want, got-want)
	}
}

// TestCanonicalLifeExpectancy: curtate life expectancy e_x = Σ_{k≥1} kpx.
// Validates the table's LifeExpectancy against an independent lx sum.
func TestCanonicalLifeExpectancy(t *testing.T) {
	lx := canonLx(canonQx())
	table := actuarial.NewLifeTableFromQx("mock", canonQx())
	for _, x := range []int{0, 40, 70, 90} {
		want := 0.0
		maxAge := len(lx) - 1
		for k := 1; x+k <= maxAge; k++ {
			want += lx[x+k] / lx[x]
		}
		got := table.LifeExpectancy(float64(x))
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("e_%d engine=%.6f want=%.6f", x, got, want)
		}
	}
}

// TestCanonicalTwoLifeComposition: with two independent lives the joint
// and last-survivor probabilities follow from the single-life survivals.
// Validates LifeProb's two-life composition and that survivalProb2 reads
// Person 2's table/DOB correctly.
func TestCanonicalTwoLifeComposition(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob1 := dateOf(1956, time.January, 1) // age 70
	dob2 := dateOf(1961, time.January, 1) // age 65
	cfg := twoLifeCfg(asOf, dob1, dob2)

	// Independent lx for each life. twoLifeCfg uses canonQx for life 1
	// and a heavier curve for life 2 (replicated here).
	lx1 := canonLx(canonQx())
	qx2 := make([]float64, 121)
	for i := range qx2 {
		qx2[i] = 0.002 + 0.00025*float64(i)*float64(i)/120.0
		if qx2[i] > 1 {
			qx2[i] = 1
		}
	}
	qx2[120] = 1
	lx2 := canonLx(qx2)

	x1, x2 := 70, 65
	for _, yr := range []int{2030, 2040, 2055} {
		d := dateOf(yr, time.January, 1)
		ageAt1 := yr - dob1.Time.Year()
		ageAt2 := yr - dob2.Time.Year()
		s1 := lx1[ageAt1] / lx1[x1]
		s2 := lx2[ageAt2] / lx2[x2]

		check := func(act byte, want float64) {
			got := cfg.LifeProb(d, act)
			if math.Abs(got-want) > 1e-9 {
				t.Errorf("year %d %s: engine=%.9f want=%.9f", yr, actuarial.ContingencyLabel(act), got, want)
			}
		}
		check(actuarial.Living, s1)                    // person 1 alive
		check(actuarial.BothLiving, s1*s2)             // joint survival
		check(actuarial.EitherLiving, 1-(1-s1)*(1-s2)) // last survivor
		check(actuarial.Only1Living, s1*(1-s2))        // 1 alive, 2 dead
		check(actuarial.Only2Living, (1-s1)*s2)        // 2 alive, 1 dead
	}
}
