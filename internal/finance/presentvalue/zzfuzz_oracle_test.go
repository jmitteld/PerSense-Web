package presentvalue

import (
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

// TestFuzzPVVsDOS aggressively fuzzes the forward PV engine (lump + periodic,
// both COLA modes) against the real DOS PRESVALU engine across a wide parameter
// space, looking for divergences the fixed-seed sweep might miss.
func TestFuzzPVVsDOS(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skip("PV oracle not present")
	}
	rng := rand.New(rand.NewSource(0x70765f66))

	// Per-section case count; override with PERSENSE_FUZZ_N (default preserves
	// the original fixed counts).
	nLump, nPeriodic := 4000, 5000
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			nLump, nPeriodic = v, v
		}
	}

	// --- Lump sums: wide amount/rate/horizon ---
	lChecked, lFails, lMax := 0, 0, 0.0
	for i := 0; i < nLump; i++ {
		amount := math.Round((1+rng.Float64()*2_000_000)*100) / 100
		rate := 0.0005 + rng.Float64()*0.40
		months := 1 + rng.Intn(600) // up to 50 years
		op, ok := runPVLumpOracle(amount, rate, months)
		if !ok {
			continue
		}
		gp := goPVLump(amount, rate, months)
		lChecked++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > lMax {
			lMax = rel
		}
		if rel > 1e-6 {
			lFails++
			if lFails <= 12 {
				t.Errorf("LUMP amt=%.2f r=%.5f mo=%d: DOS=%.6f Go=%.6f (rel %.2e)", amount, rate, months, op, gp, rel)
			}
		}
	}
	t.Logf("lump: checked %d, divergences %d, maxRel=%.2e", lChecked, lFails, lMax)

	// --- Periodic streams: wide rate, COLA up to just below the rate (kept
	// strictly below so the discounted stream stays convergent), both modes. ---
	pChecked, pFails, pMax := 0, 0, 0.0
	var worst string
	perYrChoices := []int{1, 2, 3, 4, 6, 12}
	for i := 0; i < nPeriodic; i++ {
		amt := math.Round((10+rng.Float64()*50000)*100) / 100
		rate := 0.01 + rng.Float64()*0.39
		perYr := perYrChoices[rng.Intn(len(perYrChoices))]
		years := 1 + rng.Intn(50)
		n := 1 + rng.Intn(perYr*years)
		cola := 0.0
		if rng.Intn(2) == 0 {
			cola = rng.Float64() * rate * 0.9 // strictly below the rate
		}
		cnt := rng.Intn(2) == 0
		op, ok := runPVPeriodicOracle(amt, rate, perYr, n, cola, cnt)
		if !ok {
			continue
		}
		gp := goPVPeriodic(amt, rate, perYr, n, cola, cnt)
		pChecked++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > pMax {
			pMax = rel
			worst = "amt=" + strconv.FormatFloat(amt, 'f', 0, 64) + " r=" + strconv.FormatFloat(rate, 'f', 4, 64) +
				" py=" + strconv.Itoa(perYr) + " n=" + strconv.Itoa(n) + " cola=" + strconv.FormatFloat(cola, 'f', 4, 64) +
				" cnt=" + strconv.FormatBool(cnt)
		}
		if rel > 1e-6 {
			pFails++
			if pFails <= 12 {
				t.Errorf("PERIODIC amt=%.0f r=%.4f py=%d n=%d cola=%.4f cnt=%v: DOS=%.6f Go=%.6f (rel %.2e)",
					amt, rate, perYr, n, cola, cnt, op, gp, rel)
			}
		}
	}
	t.Logf("periodic: checked %d, divergences %d, maxRel=%.2e at [%s]", pChecked, pFails, pMax, worst)
}
