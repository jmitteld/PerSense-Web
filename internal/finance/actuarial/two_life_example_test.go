package actuarial

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestTwoLifeJointAndSurvivorExample is a single, concrete, hand-checkable
// worked example for the two-life (joint-life and last-survivor) path,
// complementing the parametrized sweep in TestActuarialExtendedThirdParty.
//
// Scenario: two independent lives on the SOA Standard Ultimate Life Table
// (SULT, Makeham A=0.00022 B=2.7e-6 c=1.124), aged x=65 and y=60, valued at
// i=5% (the SULT's own valuation rate). The quantities are annuities-due
// paying 1 per year while the contingency holds:
//
//   - joint-life          ä_{65:60} — pays while BOTH are alive
//   - last-survivor       ä_{65:60-bar} — pays while AT LEAST ONE is alive
//
// The two EXPECTED constants below are independently verified with the
// open-source `actuarialmath` library (SULT().p_x survival composed under the
// independence assumption), NOT by this engine:
//
//	from actuarialmath import SULT
//	s = SULT(); v = 1/1.05
//	kpx = lambda a,k: s.p_x(a, t=k)
//	joint = sum(v**k * kpx(65,k)*kpx(60,k)            for k in range(120))  # 12.373812
//	last  = sum(v**k * (1-(1-kpx(65,k))*(1-kpx(60,k))) for k in range(120)) # 16.080052
//	# single lives, for the identity below:
//	s.whole_life_annuity(65) -> 13.549790 ; s.whole_life_annuity(60) -> 14.904074
//
// The committed SULT lx in testdata/sult_reference.json reproduces these to
// ~1e-7, so the engine — which reads that lx and folds survival through its
// own LifeProb dispatch — must match the actuarialmath figures to <1e-5.
func TestTwoLifeJointAndSurvivorExample(t *testing.T) {
	const (
		wantJointAnnuity = 12.373812 // ä_{65:60}, actuarialmath SULT, i=5%
		wantLastAnnuity  = 16.080052 // ä_{65:60-bar} (last survivor)
		wantAnn65        = 13.549790 // ä_65 single life
		wantAnn60        = 14.904074 // ä_60 single life
		tol              = 1e-5
	)

	raw, err := os.ReadFile("testdata/sult_reference.json")
	if err != nil {
		t.Skipf("SULT reference not present: %v", err)
	}
	var ref struct {
		Lx []float64 `json:"lx"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("bad reference json: %v", err)
	}
	tbl := NewLifeTableFromLx("SULT", ref.Lx)
	omega := tbl.MaxAge()

	const now = 2024
	const x, y = 65, 60
	i := 0.05
	v := 1.0 / (1 + i)

	cfg := &ActuarialConfig{
		Table1: tbl, DOB1: types.NewDateRec(now-x, time.January, 1),
		Table2: tbl, DOB2: types.NewDateRec(now-y, time.January, 1),
		Now: types.NewDateRec(now, time.January, 1),
	}
	dateK := func(k int) types.DateRec { return types.NewDateRec(now+k, time.January, 1) }

	// Annuity-due Σ_k vᵏ · P(contingency holds at year k), summed over the
	// engine's own LifeProb so this exercises the production survival dispatch.
	annuity := func(ct byte) float64 {
		s := 0.0
		for k := 0; k <= omega; k++ {
			s += math.Pow(v, float64(k)) * cfg.LifeProb(dateK(k), ct)
		}
		return s
	}

	gotJoint := annuity(BothLiving)
	gotLast := annuity(EitherLiving)
	gotAnn65 := annuity(Living) // person-1-alive annuity = single life ä_65

	// Person-2-alive single annuity: swap which life is "person 1".
	cfg2 := &ActuarialConfig{
		Table1: tbl, DOB1: types.NewDateRec(now-y, time.January, 1),
		Now: types.NewDateRec(now, time.January, 1),
	}
	gotAnn60 := 0.0
	for k := 0; k <= omega; k++ {
		gotAnn60 += math.Pow(v, float64(k)) * cfg2.LifeProb(dateK(k), Living)
	}

	check := func(name string, got, want float64) {
		if d := math.Abs(got - want); d > tol {
			t.Errorf("%s: engine=%.8f want=%.8f (|err|=%.2e > %.0e)", name, got, want, d, tol)
		} else {
			t.Logf("%s: engine=%.8f want=%.8f (|err|=%.2e)", name, got, want, d)
		}
	}
	check("joint-life ä_{65:60}", gotJoint, wantJointAnnuity)
	check("last-survivor ä_{65:60-bar}", gotLast, wantLastAnnuity)
	check("single ä_65", gotAnn65, wantAnn65)
	check("single ä_60", gotAnn60, wantAnn60)

	// Exact actuarial identity: ä_{x:y-bar} = ä_x + ä_y − ä_{x:y}. This holds
	// from the engine's OWN values regardless of the reference, so it catches a
	// mis-wired last-survivor complement even if the constants drifted.
	if d := math.Abs(gotLast - (gotAnn65 + gotAnn60 - gotJoint)); d > 1e-9 {
		t.Errorf("identity ä_{x:y-bar}=ä_x+ä_y−ä_{x:y} violated: last=%.10f vs %.10f (|err|=%.2e)",
			gotLast, gotAnn65+gotAnn60-gotJoint, d)
	}
}
