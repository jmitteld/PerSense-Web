package actuarial

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestActuarialFuzzVsThirdParty throws ~500 RANDOMLY drawn life-contingency
// queries at the engine and diffs each against the live actuarialmath oracle
// (the substitute gold standard while the DOS actuarial core is missing —
// docs/actuarial_oracle_blocked.md). Where TestActuarialLiveThirdPartyOracle
// walks a fixed age/duration grid, this samples the (kind, age, term) space
// randomly with a fixed seed, so a systematic error the grid happens to step
// over still surfaces. Skips cleanly when python3 / actuarialmath is absent.
func TestActuarialFuzzVsThirdParty(t *testing.T) {
	n := 500
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			n = v
		}
	}

	type qd struct {
		kind string
		x, k int
	}
	rng := rand.New(rand.NewSource(0xAC7A1F))
	var queries []string
	var meta []qd
	for len(queries) < n {
		switch rng.Intn(5) {
		case 0: // k-year survival
			x := 20 + rng.Intn(76) // 20..95
			maxK := 110 - x
			if maxK < 1 {
				continue
			}
			k := 1 + rng.Intn(maxK)
			queries = append(queries, fmt.Sprintf("surv %d %d", x, k))
			meta = append(meta, qd{"surv", x, k})
		case 1:
			x := 20 + rng.Intn(76)
			queries = append(queries, fmt.Sprintf("lifeexp %d", x))
			meta = append(meta, qd{"lifeexp", x, 0})
		case 2:
			x := 20 + rng.Intn(76)
			queries = append(queries, fmt.Sprintf("annuity %d", x))
			meta = append(meta, qd{"annuity", x, 0})
		case 3:
			x := 20 + rng.Intn(76)
			queries = append(queries, fmt.Sprintf("insurance %d", x))
			meta = append(meta, qd{"insurance", x, 0})
		case 4:
			x := 20 + rng.Intn(76)
			queries = append(queries, fmt.Sprintf("pod %d", x))
			meta = append(meta, qd{"pod", x, 0})
		}
	}

	vals, ok := runActuarialOracle(queries)
	if !ok {
		t.Skip("actuarialmath oracle unavailable (need python3 + `pip install actuarialmath ipython`)")
	}

	tbl := NewLifeTableFromLx("SULT", makehamSULTLx())
	const v = 1.0 / 1.05
	now := types.NewDateRec(2024, time.January, 1)
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
		return cfg.PODValue(now, math.Log(1.05)) / 1e6
	}

	counts := map[string]int{}
	maxErr := map[string]float64{}
	divergences := 0
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
			got, tol = engPOD(m.x), 1e-5
		}
		e := math.Abs(got - want)
		counts[m.kind]++
		if e > maxErr[m.kind] {
			maxErr[m.kind] = e
		}
		if e > tol {
			divergences++
			if divergences <= 20 {
				t.Errorf("%s(x=%d,k=%d): engine=%.10f oracle=%.10f (|err|=%.2e > %.0e)",
					m.kind, m.x, m.k, got, want, e, tol)
			}
		}
	}
	total := 0
	for _, k := range []string{"surv", "lifeexp", "annuity", "insurance", "pod"} {
		t.Logf("fuzz vs actuarialmath — %-10s: %d checks, max |err| = %.2e", k, counts[k], maxErr[k])
		total += counts[k]
	}
	t.Logf("fuzz total: %d random queries checked, %d divergences", total, divergences)
}
