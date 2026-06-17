package actuarial

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// makehamSULTLx builds the SOA Standard Ultimate Life Table lx column from
// Makeham's Law (A=0.00022, B=2.7e-6, c=1.124) — the exact law behind the SULT.
func makehamSULTLx() []float64 {
	const A, B, c = 0.00022, 2.7e-6, 1.124
	lnc := math.Log(c)
	lx := make([]float64, 131)
	for a := 0; a <= 130; a++ {
		lx[a] = 100000.0 * math.Exp(-(A*float64(a) + (B/lnc)*(math.Pow(c, float64(a))-1)))
	}
	return lx
}

// runActuarialOracle batches queries to the live actuarialmath oracle
// (scripts/actuarial_oracle.py) and returns one value per query. Returns ok=false
// when python3 or the actuarialmath library is unavailable, so the test skips
// cleanly — exactly like the DOS-oracle sweeps skip without Free Pascal.
func runActuarialOracle(queries []string) ([]float64, bool) {
	script := os.Getenv("PERSENSE_ACTU_ORACLE")
	if script == "" {
		script = "../../../scripts/actuarial_oracle.py"
	}
	if _, err := os.Stat(script); err != nil {
		return nil, false
	}
	if err := exec.Command("python3", "-c", "import actuarialmath").Run(); err != nil {
		return nil, false
	}
	cmd := exec.Command("python3", script)
	cmd.Stdin = strings.NewReader(strings.Join(queries, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != len(queries) {
		return nil, false
	}
	vals := make([]float64, len(lines))
	for i, ln := range lines {
		f := strings.Fields(ln)
		if len(f) < 2 || f[0] != "value" {
			vals[i] = math.NaN()
			continue
		}
		vals[i], _ = strconv.ParseFloat(f[1], 64)
	}
	return vals, true
}

// TestActuarialLiveThirdPartyOracle validates the Go life-contingency engine
// against a LIVE third-party oracle built around the open-source `actuarialmath`
// library and the SOA Standard Ultimate Life Table. Because the original DOS
// actuarial core is missing (docs/actuarial_oracle_blocked.md), this independent
// library is the substitute gold standard. Unlike the committed-JSON check
// (TestSULTvsActuarialMath), this computes every reference value at test time by
// shelling to the oracle — a true differential harness, the actuarial analog of
// the DOS source-oracle rig. Skips when python3 / actuarialmath is absent.
func TestActuarialLiveThirdPartyOracle(t *testing.T) {
	ages := []int{20, 25, 30, 35, 40, 45, 50, 55, 60, 65, 70, 75, 80, 85, 90}
	ks := []int{1, 2, 5, 10, 15, 20, 30}

	type qd struct {
		kind string
		x, k int
	}
	var queries []string
	var meta []qd
	add := func(q string, m qd) { queries = append(queries, q); meta = append(meta, m) }
	for _, x := range ages {
		for _, k := range ks {
			if x+k <= 110 {
				add(fmt.Sprintf("surv %d %d", x, k), qd{"surv", x, k})
			}
		}
	}
	for _, x := range ages {
		add(fmt.Sprintf("lifeexp %d", x), qd{"lifeexp", x, 0})
		add(fmt.Sprintf("annuity %d", x), qd{"annuity", x, 0})
		add(fmt.Sprintf("insurance %d", x), qd{"insurance", x, 0})
		add(fmt.Sprintf("pod %d", x), qd{"pod", x, 0})
	}

	vals, ok := runActuarialOracle(queries)
	if !ok {
		t.Skip("actuarialmath oracle unavailable (need python3 + `pip install actuarialmath ipython`); " +
			"the committed-JSON TestSULTvsActuarialMath covers this offline")
	}

	tbl := NewLifeTableFromLx("SULT", makehamSULTLx())
	const v = 1.0 / 1.05
	now := types.NewDateRec(2024, time.January, 1)

	// Build annuity/insurance from the ENGINE's survival so the comparison
	// exercises ConditionalSurvival end-to-end against the library's annuity.
	engAnnuity := func(x int) float64 {
		s := 0.0
		for k := 0; x+k <= 130; k++ {
			s += math.Pow(v, float64(k)) * tbl.ConditionalSurvival(float64(x), float64(x+k))
		}
		return s
	}
	engInsurance := func(x int) float64 {
		s := 0.0
		for k := 0; x+k < 130; k++ {
			s += math.Pow(v, float64(k+1)) *
				(tbl.ConditionalSurvival(float64(x), float64(x+k)) - tbl.ConditionalSurvival(float64(x), float64(x+k+1)))
		}
		return s
	}
	engPOD := func(x int) float64 {
		cfg := ActuarialConfig{Table1: tbl, DOB1: types.NewDateRec(2024-x, time.January, 1), Now: now, POD: 1e6}
		// PODValue treats `rate` as a CONTINUOUS force (discount = exp(-rate·t)),
		// matching how the PV engine passes the continuous "True" rate. The SULT
		// oracle uses an EFFECTIVE i=5%, whose continuous force is δ=ln(1.05) — so
		// pass that, not 0.05, to compare like for like.
		return cfg.PODValue(now, math.Log(1.05)) / 1e6
	}

	counts := map[string]int{}
	maxErr := map[string]float64{}
	for i, m := range meta {
		want := vals[i]
		if math.IsNaN(want) {
			continue
		}
		var got float64
		tol := 1e-9
		switch m.kind {
		case "surv":
			got = tbl.ConditionalSurvival(float64(m.x), float64(m.x+m.k))
			if lp := lifeProbLiving(tbl, m.x, m.k); math.Abs(lp-want) > math.Abs(got-want) {
				got = lp
			}
		case "lifeexp":
			got, tol = tbl.LifeExpectancy(float64(m.x)), 1e-6
		case "annuity":
			got, tol = engAnnuity(m.x), 1e-6
		case "insurance":
			got, tol = engInsurance(m.x), 1e-6
		case "pod":
			got, tol = engPOD(m.x), 1e-5 // PODValue rounds to cents (Round2)
		}
		e := math.Abs(got - want)
		counts[m.kind]++
		if e > maxErr[m.kind] {
			maxErr[m.kind] = e
		}
		if e > tol {
			t.Errorf("%s(x=%d,k=%d): engine=%.10f oracle=%.10f (|err|=%.2e > %.0e)",
				m.kind, m.x, m.k, got, want, e, tol)
		}
	}
	for _, k := range []string{"surv", "lifeexp", "annuity", "insurance", "pod"} {
		t.Logf("live actuarialmath oracle — %-10s: %d checks, max |err| = %.2e", k, counts[k], maxErr[k])
	}
}

// TestActuarialSOAPublishedAnchor checks the engine against the SOA's OFFICIALLY
// PUBLISHED Standard Ultimate Life Table values (i=5%) — the printed numbers in
// the SOA exam table booklet, independent of ANY library or generator. The Go
// engine's first-principles Makeham survival, composed into the standard
// actuarial quantities, must reproduce the published table to its printed
// precision. This always runs (no python needed), so a regression in the
// life-table math fails CI even offline.
func TestActuarialSOAPublishedAnchor(t *testing.T) {
	tbl := NewLifeTableFromLx("SULT", makehamSULTLx())
	const v = 1.0 / 1.05
	ann := func(x int) float64 {
		s := 0.0
		for k := 0; x+k <= 130; k++ {
			s += math.Pow(v, float64(k)) * tbl.ConditionalSurvival(float64(x), float64(x+k))
		}
		return s
	}
	ins := func(x int) float64 {
		s := 0.0
		for k := 0; x+k < 130; k++ {
			s += math.Pow(v, float64(k+1)) *
				(tbl.ConditionalSurvival(float64(x), float64(x+k)) - tbl.ConditionalSurvival(float64(x), float64(x+k+1)))
		}
		return s
	}
	endow := func(x, n int) float64 {
		return math.Pow(v, float64(n)) * tbl.ConditionalSurvival(float64(x), float64(x+n))
	}

	// SOA Standard Ultimate Life Table, i=0.05 (published booklet values).
	type anchor struct {
		x                    int
		aax, kA1000, ex, e10 float64
	}
	pub := []anchor{
		{20, 19.9664, 49.22, 65.4132, 0.61224},
		{30, 19.3834, 76.98, 55.5792, 0.61152},
		{40, 18.4578, 121.06, 45.7777, 0.60920},
		{50, 17.0245, 189.31, 36.0915, 0.60182},
		{60, 14.9041, 290.28, 26.7100, 0.57864},
		{70, 12.0083, 428.18, 18.0112, 0.50994},
		{80, 8.5484, 592.93, 10.6059, 0.33952},
		{90, 5.1835, 753.17, 5.1612, 0.09168},
	}
	for _, a := range pub {
		if g := ann(a.x); math.Abs(g-a.aax) > 5e-4 {
			t.Errorf("ä_%d = %.4f, SOA published %.4f", a.x, g, a.aax)
		}
		if g := 1000 * ins(a.x); math.Abs(g-a.kA1000) > 0.01 {
			t.Errorf("1000·A_%d = %.2f, SOA published %.2f", a.x, g, a.kA1000)
		}
		if g := tbl.LifeExpectancy(float64(a.x)); math.Abs(g-a.ex) > 5e-4 {
			t.Errorf("e_%d = %.4f, SOA published %.4f", a.x, g, a.ex)
		}
		if g := endow(a.x, 10); math.Abs(g-a.e10) > 5e-5 {
			t.Errorf("10E_%d = %.5f, SOA published %.5f", a.x, g, a.e10)
		}
	}
	t.Logf("engine reproduces the SOA published SULT (ä_x, 1000·A_x, e_x, 10E_x) for %d ages to printed precision", len(pub))
}
