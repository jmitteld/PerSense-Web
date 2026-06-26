// help_examples_test.go: exercises the actuarial module with
// synthetic test cases. There are no worked examples for the
// Actuarial window in legacy/src/win_source/Help/ (the ACTUARY unit
// was DOS-only and the Windows port stripped it out). Tests
// therefore use first-principles synthetic life tables and verify
// mathematical identities documented in PRESVALU.pas and
// PETYPES.PAS.
//
// Added by the test-matrix exercise to assess correctness across
// life-table operations and multi-life contingency logic.
package actuarial

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// constantQxTable creates a life table where qx is constant q for
// ages 0..maxAge (and the final qx forces lx[max+1]=0). With a flat
// qx, lx[k] = 100000 * (1-q)^k, so SurvivalProb(k) = (1-q)^k. This
// is the easiest property to verify analytically.
func constantQxTable(q float64, maxAge int) *LifeTable {
	qx := make([]float64, maxAge+1)
	for i := 0; i < maxAge; i++ {
		qx[i] = q
	}
	qx[maxAge] = 1.0
	return NewLifeTableFromQx("constant-q", qx)
}

// AC1 — SurvivalProb on a constant-qx table matches the closed form
// (1-q)^k at every integer age.
func TestActuarial_SurvivalProbConstantQx(t *testing.T) {
	const q = 0.1
	lt := constantQxTable(q, 20)
	for k := 0; k <= 15; k++ {
		got := lt.SurvivalProb(float64(k))
		want := math.Pow(1-q, float64(k))
		t.Logf("SurvivalProb(%d) = %.6f, want (1-q)^k = %.6f", k, got, want)
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("SurvivalProb(%d) = %.10f, want %.10f", k, got, want)
		}
	}
}

// AC2 — ConditionalSurvival is multiplicative:
//
//	P(0→b) = P(0→a) × P(a→b)
func TestActuarial_ConditionalSurvivalMultiplicative(t *testing.T) {
	lt := constantQxTable(0.05, 30)
	for _, ab := range []struct{ a, b float64 }{
		{0, 10}, {2, 8}, {5, 25}, {1.5, 12.5},
	} {
		p0a := lt.SurvivalProb(ab.a)
		pab := lt.ConditionalSurvival(ab.a, ab.b)
		p0b := lt.SurvivalProb(ab.b)
		t.Logf("a=%.1f b=%.1f: P(0→a)=%.6f × P(a→b)=%.6f = %.6f vs P(0→b)=%.6f",
			ab.a, ab.b, p0a, pab, p0a*pab, p0b)
		if math.Abs(p0a*pab-p0b) > 1e-9 {
			t.Errorf("multiplicativity failed: P(0→%.1f)·P(%.1f→%.1f)=%.10f vs P(0→%.1f)=%.10f",
				ab.a, ab.a, ab.b, p0a*pab, ab.b, p0b)
		}
	}
}

// AC3 — Two-life "BothLiving" contingency P = p1*p2 (assuming
// independence, which is the model's stated assumption).
func TestActuarial_BothLivingProbability(t *testing.T) {
	lt := constantQxTable(0.1, 50)
	cfg := &ActuarialConfig{
		Table1: lt,
		DOB1:   types.NewDateRec(2000, time.January, 1),
		Table2: lt,
		DOB2:   types.NewDateRec(2000, time.January, 1),
		Now:    types.NewDateRec(2020, time.January, 1),
	}
	future := types.NewDateRec(2030, time.January, 1)
	pBoth := cfg.LifeProb(future, BothLiving)
	p1 := cfg.LifeProb(future, Living)
	t.Logf("BothLiving@2030 = %.6f vs Living^2 = %.6f (p1=%.6f)",
		pBoth, p1*p1, p1)
	if math.Abs(pBoth-p1*p1) > 1e-9 {
		t.Errorf("BothLiving = %.10f, want p1·p2 = %.10f", pBoth, p1*p1)
	}
}

// AC4 — Two-life "EitherLiving" contingency P = p1 + p2 − p1·p2.
func TestActuarial_EitherLivingProbability(t *testing.T) {
	lt := constantQxTable(0.1, 50)
	cfg := &ActuarialConfig{
		Table1: lt,
		DOB1:   types.NewDateRec(2000, time.January, 1),
		Table2: lt,
		DOB2:   types.NewDateRec(1990, time.January, 1),
		Now:    types.NewDateRec(2020, time.January, 1),
	}
	future := types.NewDateRec(2030, time.January, 1)
	pEither := cfg.LifeProb(future, EitherLiving)
	p1 := cfg.LifeProb(future, Living)
	// To get p2 alone we need a single-life-2 contingency that
	// ignores person 1. There's no direct enum for that, so compute
	// from the underlying table: p2 = ConditionalSurvival(ageNow2,
	// agePay2).
	ageNow2 := yearsDif(cfg.DOB2, cfg.Now)
	agePay2 := yearsDif(cfg.DOB2, future)
	p2 := lt.ConditionalSurvival(ageNow2, agePay2)
	want := p1 + p2 - p1*p2
	t.Logf("EitherLiving = %.6f, want p1+p2-p1p2 = %.6f (p1=%.6f p2=%.6f)",
		pEither, want, p1, p2)
	if math.Abs(pEither-want) > 1e-9 {
		t.Errorf("EitherLiving = %.10f, want %.10f", pEither, want)
	}
}

// AC5 — LifeProb under NotContingent always = 1.0.
func TestActuarial_NotContingentAlwaysOne(t *testing.T) {
	lt := constantQxTable(0.5, 20)
	cfg := &ActuarialConfig{
		Table1: lt,
		DOB1:   types.NewDateRec(2000, time.January, 1),
		Now:    types.NewDateRec(2020, time.January, 1),
	}
	dates := []types.DateRec{
		types.NewDateRec(2020, time.January, 1),
		types.NewDateRec(2030, time.January, 1),
		types.NewDateRec(2050, time.January, 1),
	}
	for _, d := range dates {
		p := cfg.LifeProb(d, NotContingent)
		t.Logf("LifeProb(%s, NotContingent) = %.6f",
			d.Time.Format("1/2/06"), p)
		if math.Abs(p-1.0) > 1e-12 {
			t.Errorf("NotContingent on %s = %.10f, want 1.0",
				d.Time.Format("1/2/06"), p)
		}
	}
}

// AC6 — PODValue: payment-on-death PV under a known table with a
// fixed rate. Smoke test that POD is positive, finite, and less than
// the gross POD amount (because future deaths are discounted).
func TestActuarial_PODValueSanity(t *testing.T) {
	lt := constantQxTable(0.05, 50)
	cfg := &ActuarialConfig{
		Table1: lt,
		DOB1:   types.NewDateRec(2000, time.January, 1),
		Now:    types.NewDateRec(2020, time.January, 1),
		POD:    100_000.0,
	}
	pod := cfg.PODValue(types.NewDateRec(2020, time.January, 1), 0.05)
	t.Logf("PODValue (POD=100k, rate=5%%) = %.4f", pod)
	if !(pod > 0 && pod < cfg.POD) {
		t.Errorf("PODValue = %.4f, want 0 < pod < %.4f", pod, cfg.POD)
	}
	if math.IsNaN(pod) || math.IsInf(pod, 0) {
		t.Errorf("PODValue is non-finite: %v", pod)
	}
}
