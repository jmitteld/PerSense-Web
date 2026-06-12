package actuarial

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestActuarialExtendedThirdParty broadens the independent third-party
// validation of the actuarial engine far beyond the single-life whole-life
// checks in TestSULTvsActuarialMath. Against a first-principles reference
// (scripts/gen_actuarial_reference.py, anchored to `actuarialmath` at i=5%) it
// validates, on TWO independent mortality tables (the SOA SULT / Makeham law and
// a separate Gompertz law — demonstrating the engine is table-independent):
//
//   - the LifeProb dispatch for ALL SIX contingency types (Living, Dead,
//     Only1Living, Only2Living, EitherLiving, BothLiving);
//   - single-life term / temporary / deferred annuities, term / endowment
//     insurance, pure endowment, and the whole-life annuity VARIANCE, at three
//     interest rates (3 %, 5 %, 7 %);
//   - TWO-LIFE joint-life and last-survivor annuities and insurances, plus the
//     only-1 / only-2 contingent annuities, built through the engine's LifeProb;
//   - the exact identities last-survivor = x + y − joint for annuities and
//     insurances, computed entirely from engine values.

type singleSec struct {
	WLAnnuity       map[string]float64 `json:"wl_annuity"`
	WLInsurance     map[string]float64 `json:"wl_insurance"`
	WLAnnuityVar    map[string]float64 `json:"wl_annuity_var"`
	TempAnnuity     map[string]float64 `json:"temp_annuity"`
	DeferredAnnuity map[string]float64 `json:"deferred_annuity"`
	TermInsurance   map[string]float64 `json:"term_insurance"`
	PureEndow       map[string]float64 `json:"pure_endow"`
	EndowInsurance  map[string]float64 `json:"endow_insurance"`
	PodDOS          map[string]float64 `json:"pod_dos"`
}

type twoLifeVal struct {
	JointAnnuity   float64 `json:"joint_annuity"`
	LastAnnuity    float64 `json:"last_annuity"`
	JointInsurance float64 `json:"joint_insurance"`
	LastInsurance  float64 `json:"last_insurance"`
	Only1Annuity   float64 `json:"only1_annuity"`
	Only2Annuity   float64 `json:"only2_annuity"`
}

type gridVal struct {
	Living1 float64 `json:"living1"`
	Dead1   float64 `json:"dead1"`
	Only1   float64 `json:"only1"`
	Only2   float64 `json:"only2"`
	Either  float64 `json:"either"`
	Both    float64 `json:"both"`
}

type actBlock struct {
	Lx           []float64                        `json:"lx"`
	Single       map[string]singleSec             `json:"single"`
	TwoLife      map[string]map[string]twoLifeVal `json:"twolife"`
	LifeprobGrid map[string]gridVal               `json:"lifeprob_grid"`
}

type extRef struct {
	actBlock          // SULT at the top level
	Gompertz actBlock `json:"gompertz"`
}

func TestActuarialExtendedThirdParty(t *testing.T) {
	raw, err := os.ReadFile("testdata/sult_reference.json")
	if err != nil {
		t.Skipf("actuarial reference not present: %v", err)
	}
	var ref extRef
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("bad reference json: %v", err)
	}
	if len(ref.Single) == 0 || len(ref.TwoLife) == 0 || len(ref.Gompertz.Single) == 0 {
		t.Skip("reference json predates the extended suite; regenerate with scripts/gen_actuarial_reference.py")
	}
	validateActuarialBlock(t, "SULT", ref.actBlock)
	validateActuarialBlock(t, "Gompertz", ref.Gompertz)
}

func validateActuarialBlock(t *testing.T, label string, blk actBlock) {
	tbl := NewLifeTableFromLx(label, blk.Lx)
	omega := tbl.MaxAge()
	const now = 2024

	cfg := func(x, y int) *ActuarialConfig {
		return &ActuarialConfig{
			Table1: tbl, DOB1: types.NewDateRec(now-x, time.January, 1),
			Table2: tbl, DOB2: types.NewDateRec(now-y, time.January, 1),
			Now: types.NewDateRec(now, time.January, 1),
		}
	}
	dateK := func(k int) types.DateRec { return types.NewDateRec(now+k, time.January, 1) }
	kpx := func(x, k int) float64 { return tbl.ConditionalSurvival(float64(x), float64(x+k)) }

	// (1) LifeProb dispatch — all six contingency types vs the independent grid.
	lpFails, lpMax := 0, 0.0
	for key, w := range blk.LifeprobGrid {
		x, y, k := parse3(t, key)
		c, d := cfg(x, y), dateK(k)
		pairs := []struct {
			ct   byte
			want float64
		}{
			{Living, w.Living1}, {Dead, w.Dead1}, {Only1Living, w.Only1},
			{Only2Living, w.Only2}, {EitherLiving, w.Either}, {BothLiving, w.Both},
		}
		for _, p := range pairs {
			got := c.LifeProb(d, p.ct)
			if dd := math.Abs(got - p.want); dd > lpMax {
				lpMax = dd
			}
			if math.Abs(got-p.want) > 1e-7 {
				lpFails++
				if lpFails <= 8 {
					t.Errorf("[%s] LifeProb[%s] type %d: got %.10f want %.10f", label, key, p.ct, got, p.want)
				}
			}
		}
	}
	t.Logf("[%s] LifeProb 6-contingency grid: %d points, fails %d, max |err|=%.2e", label, len(blk.LifeprobGrid)*6, lpFails, lpMax)

	// (2) single-life quantities at three rates.
	sFails, sMax := 0, 0.0
	bump := func(d float64) {
		if d > sMax {
			sMax = d
		}
	}
	ann := func(x, nMax int, v float64) float64 {
		s := 0.0
		for k := 0; k < nMax; k++ {
			s += math.Pow(v, float64(k)) * kpx(x, k)
		}
		return s
	}
	ins := func(x, nMax int, vv float64) float64 {
		s := 0.0
		for k := 0; k < nMax; k++ {
			s += math.Pow(vv, float64(k+1)) * (kpx(x, k) - kpx(x, k+1))
		}
		return s
	}
	for rk, sec := range blk.Single {
		i, _ := strconv.ParseFloat(rk, 64)
		v := 1.0 / (1 + i)
		for xs, want := range sec.WLAnnuity {
			x := atoi(t, xs)
			cmp := func(name string, got, w, tol float64) {
				dd := math.Abs(got - w)
				bump(dd)
				if dd > tol {
					sFails++
					if sFails <= 12 {
						t.Errorf("[%s] i=%s %s age %d: got %.8f want %.8f", label, rk, name, x, got, w)
					}
				}
			}
			cmp("wl_annuity", ann(x, omega-x+1, v), want, 1e-6)
			cmp("wl_insurance", ins(x, omega-x, v), sec.WLInsurance[xs], 1e-6)
			a1 := ins(x, omega-x, v)
			a2 := ins(x, omega-x, v*v)
			d := 1 - v
			cmp("wl_annuity_var", (a2-a1*a1)/(d*d), sec.WLAnnuityVar[xs], 1e-4)
			c := cfg(x, x)
			c.POD = 1e8
			cmp("pod_dos", c.PODValue(c.Now, math.Log(1+i))/1e8, sec.PodDOS[xs], 1e-6)
			for _, n := range []int{5, 10, 20} {
				k := xs + "," + strconv.Itoa(n)
				cmp("temp_annuity", ann(x, n, v), sec.TempAnnuity[k], 1e-6)
				cmp("deferred_annuity", ann(x, omega-x+1, v)-ann(x, n, v), sec.DeferredAnnuity[k], 1e-6)
				cmp("term_insurance", ins(x, n, v), sec.TermInsurance[k], 1e-6)
				pe := math.Pow(v, float64(n)) * kpx(x, n)
				cmp("pure_endow", pe, sec.PureEndow[k], 1e-6)
				cmp("endow_insurance", ins(x, n, v)+pe, sec.EndowInsurance[k], 1e-6)
			}
		}
	}
	t.Logf("[%s] single-life quantities (3 rates): fails %d, max |err|=%.2e", label, sFails, sMax)

	// (3) two-life annuities/insurances via LifeProb + the x+y−joint identities.
	tFails, tMax, idMax := 0, 0.0, 0.0
	K2 := func(x, y int) int {
		if omega-y > omega-x {
			return omega - y
		}
		return omega - x
	}
	twoAnn := func(c *ActuarialConfig, x, y int, v float64, ct byte) float64 {
		s := 0.0
		for k := 0; k <= K2(x, y); k++ {
			s += math.Pow(v, float64(k)) * c.LifeProb(dateK(k), ct)
		}
		return s
	}
	twoIns := func(c *ActuarialConfig, x, y int, v float64, ct byte) float64 {
		s := 0.0
		for k := 0; k < K2(x, y); k++ {
			s += math.Pow(v, float64(k+1)) * (c.LifeProb(dateK(k), ct) - c.LifeProb(dateK(k+1), ct))
		}
		return s
	}
	for rk, pairs := range blk.TwoLife {
		i, _ := strconv.ParseFloat(rk, 64)
		v := 1.0 / (1 + i)
		for key, want := range pairs {
			ps := strings.Split(key, ",")
			x, y := atoi(t, ps[0]), atoi(t, ps[1])
			c := cfg(x, y)
			cmp := func(name string, got, w float64) {
				dd := math.Abs(got - w)
				if dd > tMax {
					tMax = dd
				}
				if dd > 1e-6 {
					tFails++
					if tFails <= 12 {
						t.Errorf("[%s] i=%s two-life %s %s: got %.8f want %.8f", label, rk, name, key, got, w)
					}
				}
			}
			jointAnn := twoAnn(c, x, y, v, BothLiving)
			lastAnn := twoAnn(c, x, y, v, EitherLiving)
			jointIns := twoIns(c, x, y, v, BothLiving)
			lastIns := twoIns(c, x, y, v, EitherLiving)
			cmp("joint_annuity", jointAnn, want.JointAnnuity)
			cmp("last_annuity", lastAnn, want.LastAnnuity)
			cmp("joint_insurance", jointIns, want.JointInsurance)
			cmp("last_insurance", lastIns, want.LastInsurance)
			cmp("only1_annuity", twoAnn(c, x, y, v, Only1Living), want.Only1Annuity)
			cmp("only2_annuity", twoAnn(c, x, y, v, Only2Living), want.Only2Annuity)

			// Exact identities ä_xȳ = ä_x + ä_y − ä_xy and A_xȳ = A_x + A_y − A_xy,
			// from engine values — independent of the combination formula.
			annX := twoAnn(c, x, y, v, Living)
			annY := twoAnn(cfg(y, x), y, x, v, Living)
			if dd := math.Abs(lastAnn - (annX + annY - jointAnn)); dd > idMax {
				idMax = dd
			}
			if math.Abs(lastAnn-(annX+annY-jointAnn)) > 1e-6 {
				tFails++
				t.Errorf("[%s] i=%s identity ä_xȳ %s: %.8f vs x+y-joint %.8f", label, rk, key, lastAnn, annX+annY-jointAnn)
			}
			insX := twoIns(c, x, y, v, Living)
			insY := twoIns(cfg(y, x), y, x, v, Living)
			if math.Abs(lastIns-(insX+insY-jointIns)) > 1e-6 {
				tFails++
				t.Errorf("[%s] i=%s identity A_xȳ %s: %.8f vs x+y-joint %.8f", label, rk, key, lastIns, insX+insY-jointIns)
			}
		}
	}
	t.Logf("[%s] two-life quantities (3 rates): fails %d, max |err|=%.2e | x+y-joint identity max |err|=%.2e", label, tFails, tMax, idMax)
}

func parse3(t *testing.T, key string) (int, int, int) {
	p := strings.Split(key, ",")
	if len(p) != 3 {
		t.Fatalf("bad grid key %q", key)
	}
	return atoi(t, p[0]), atoi(t, p[1]), atoi(t, p[2])
}
