package actuarial

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"testing"
)

// Spot values transcribed directly from the original distribution files
// MALE.ACT / FEMALE.ACT (qx expressed there as "x.xxxE-3" = ×0.001). These pin
// the recovered 1988 HHS tables to their source so a future edit that corrupts
// the data fails loudly.
func TestPersense1988SpotValues(t *testing.T) {
	maleWant := map[int]float64{
		0: 0.000377, 1: 0.000377, 6: 0.000350, 18: 0.000472,
		40: 0.001341, 65: 0.012851, 80: 0.057026, 100: 0.270906,
		114: 0.914167, 115: 1.0,
	}
	for age, want := range maleWant {
		if got := Persense1988MaleQx[age]; math.Abs(got-want) > 1e-12 {
			t.Errorf("male qx[%d] = %g, want %g", age, got, want)
		}
	}
	femaleWant := map[int]float64{
		1: 0.000194, 6: 0.000160, 18: 0.000229, 40: 0.000742,
		65: 0.007336, 80: 0.036395, 100: 0.239215, 114: 0.898885, 115: 1.0,
	}
	for age, want := range femaleWant {
		if got := Persense1988FemaleQx[age]; math.Abs(got-want) > 1e-12 {
			t.Errorf("female qx[%d] = %g, want %g", age, got, want)
		}
	}
}

// TestPersense1988TableShape checks structural invariants of the recovered
// tables: extent (ages 0..115), qx bounded in [0,1], a closing qx of 1.0, the
// known female age-0 gap, and a strictly decreasing survivor curve.
func TestPersense1988TableShape(t *testing.T) {
	if len(Persense1988MaleQx) != 116 {
		t.Errorf("male qx length = %d, want 116 (ages 0..115)", len(Persense1988MaleQx))
	}
	if len(Persense1988FemaleQx) != 116 {
		t.Errorf("female qx length = %d, want 116 (ages 0..115)", len(Persense1988FemaleQx))
	}
	// The source female table omits age 0; we preserve qx[0]=0.
	if Persense1988FemaleQx[0] != 0 {
		t.Errorf("female qx[0] = %g, want 0 (source file has no age-0 row)", Persense1988FemaleQx[0])
	}
	for name, qx := range map[string][]float64{"male": Persense1988MaleQx, "female": Persense1988FemaleQx} {
		for age, q := range qx {
			if q < 0 || q > 1 {
				t.Errorf("%s qx[%d] = %g out of [0,1]", name, age, q)
			}
		}
		if last := qx[len(qx)-1]; math.Abs(last-1.0) > 1e-12 {
			t.Errorf("%s terminal qx = %g, want 1.0 (table must close)", name, last)
		}
	}
	// lx must be non-increasing and reach (near) zero at the closing age.
	for _, lt := range []*LifeTable{Persense1988Male(), Persense1988Female()} {
		for i := 1; i < len(lt.Lx); i++ {
			if lt.Lx[i] > lt.Lx[i-1]+1e-9 {
				t.Fatalf("%s: lx increased at age %d (%g > %g)", lt.Name, i, lt.Lx[i], lt.Lx[i-1])
			}
		}
		if lt.Lx[len(lt.Lx)-1] > 1e-6 {
			t.Errorf("%s: survivors at closing age = %g, want ~0", lt.Name, lt.Lx[len(lt.Lx)-1])
		}
	}
}

// TestPersense1988Survival sanity-checks derived survival against the source qx:
// one-year conditional survival from age x equals 1-qx[x], and the female cohort
// (lighter mortality) outlives the male cohort at age 65.
func TestPersense1988Survival(t *testing.T) {
	male := Persense1988Male()
	// P(survive 65→66 | alive at 65) = 1 - qx[65].
	got := male.ConditionalSurvival(65, 66)
	want := 1 - Persense1988MaleQx[65]
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("male 1-yr survival at 65 = %g, want %g (1-qx[65])", got, want)
	}
	female := Persense1988Female()
	// Female mortality is lighter, so survival from birth to 65 is higher.
	if female.SurvivalProb(65) <= male.SurvivalProb(65) {
		t.Errorf("expected female survival to 65 (%g) > male (%g)",
			female.SurvivalProb(65), male.SurvivalProb(65))
	}
}

// TestPersense1988MatchesServedJS guards against drift between the Go canonical
// tables (used for engine validation) and the PERSENSE_1988_* arrays actually
// served to the browser in cmd/persense/static/lifetables.js — the two must be
// the same data or the frontend computes on a different basis than the tests
// certify. (Post-mortem lesson: two copies of the "same" data silently diverge.)
func TestPersense1988MatchesServedJS(t *testing.T) {
	const jsPath = "../../../cmd/persense/static/lifetables.js"
	raw, err := os.ReadFile(jsPath)
	if err != nil {
		t.Skipf("served life-table JS not found (%s): %v", jsPath, err)
	}
	js := string(raw)

	check := func(varName string, goQx []float64) {
		arr := extractJSQx(t, js, varName)
		for age, q := range arr {
			if age >= len(goQx) {
				t.Errorf("%s: JS has age %d beyond Go table length %d", varName, age, len(goQx))
				continue
			}
			if math.Abs(goQx[age]-q) > 1e-12 {
				t.Errorf("%s drift at age %d: JS=%g Go=%g", varName, age, q, goQx[age])
			}
		}
	}
	check("PERSENSE_1988_MALE_QX", Persense1988MaleQx)
	check("PERSENSE_1988_FEMALE_QX", Persense1988FemaleQx)
}

// extractJSQx pulls the [age,qx] pairs out of a `var NAME = [ ... ];` block.
func extractJSQx(t *testing.T, js, name string) map[int]float64 {
	t.Helper()
	block := regexp.MustCompile(`(?s)var\s+` + regexp.QuoteMeta(name) + `\s*=\s*\[(.*?)\]\s*;`)
	m := block.FindStringSubmatch(js)
	if m == nil {
		t.Fatalf("could not find JS array %s", name)
	}
	pair := regexp.MustCompile(`\[\s*(\d+)\s*,\s*([0-9.eE+-]+)\s*\]`)
	out := map[int]float64{}
	for _, p := range pair.FindAllStringSubmatch(m[1], -1) {
		age, _ := strconv.Atoi(p[1])
		q, err := strconv.ParseFloat(p[2], 64)
		if err != nil {
			t.Fatalf("%s: bad qx %q at age %d", name, p[2], age)
		}
		out[age] = q
	}
	if len(out) == 0 {
		t.Fatalf("%s: no [age,qx] pairs parsed", name)
	}
	return out
}
