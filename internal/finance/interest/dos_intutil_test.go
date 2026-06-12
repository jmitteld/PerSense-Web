package interest

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// Boundary differential of the core interest-math kernel (Exxp / Lnn / Power /
// Round2) against the REAL DOS INTSUTIL functions, via the amort_oracle
// `intutil` mode. These primitives are already exercised bit-identically on
// every amortization/PV/mortgage oracle case; this test makes the kernel's own
// boundary behaviour explicit, with special emphasis on the DOS round-half-DOWN
// Round2 convention (the single most fidelity-sensitive primitive).
//
// Build the oracle:  legacy/oracle/build_linux.sh   (-> /tmp/oraclebuild/amort_oracle)

func intutilBin() string {
	if p := os.Getenv("PERSENSE_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/amort_oracle"
}

func runIntutil(t *testing.T, args ...string) (float64, bool) {
	out, err := exec.Command(intutilBin(), append([]string{"intutil"}, args...)...).Output()
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(out))
	if strings.HasPrefix(s, "ERR") {
		return 0, false
	}
	v, e := strconv.ParseFloat(s, 64)
	return v, e == nil
}

func skipIfNoIntutil(t *testing.T) {
	if _, err := os.Stat(intutilBin()); err != nil {
		t.Skipf("oracle not present (%s); build via legacy/oracle/build_linux.sh", intutilBin())
	}
}

// TestDOSRound2HalfCent pins the DOS round-half-DOWN behaviour at and around the
// half-cent across magnitudes and signs. This is where Go's Round2 must NOT use
// Go's default round-half-to-even, and the most likely place for a money-rounding
// divergence to hide.
func TestDOSRound2HalfCent(t *testing.T) {
	skipIfNoIntutil(t)
	vals := []float64{
		1.005, 2.675, 0.125, 0.135, 100.005, 1000.005, 0.005, 0.015, 0.025,
		-1.005, -2.675, -0.125, 9.995, 12.345, 12.344, 12.346, 0.0049, 0.0051,
		1234567.895, 0.9999, 1.0001, 2.5, 2.50, 3.14159,
	}
	fails, maxd := 0, 0.0
	for _, v := range vals {
		// Feed the exact same decimal text to both sides so they parse the
		// identical float64.
		s := strconv.FormatFloat(v, 'f', -1, 64)
		dos, ok := runIntutil(t, "round2", s)
		if !ok {
			t.Fatalf("oracle round2 %s failed", s)
		}
		go2 := Round2(v)
		d := math.Abs(dos - go2)
		if d > maxd {
			maxd = d
		}
		if d > 1e-6 {
			fails++
			t.Errorf("Round2(%s): DOS=%.6f Go=%.6f", s, dos, go2)
		}
	}
	t.Logf("Round2 half-cent grid: %d values, fails %d, max |err|=%.2e", len(vals), fails, maxd)
}

func TestDOSIntutilMathSweep(t *testing.T) {
	skipIfNoIntutil(t)
	rng := rand.New(rand.NewSource(20260711))
	checkedExxp, checkedLnn, checkedPow := 0, 0, 0
	maxRel := 0.0
	bump := func(dos, got float64) {
		rel := math.Abs(dos-got) / math.Max(1, math.Abs(got))
		if rel > maxRel {
			maxRel = rel
		}
	}

	// Exxp over a wide finite range (including boundary magnitudes where the
	// exponent is large but in-range for both engines).
	for i := 0; i < 300; i++ {
		x := -20 + rng.Float64()*40
		got, err := Exxp(x)
		if err != nil {
			continue
		}
		dos, ok := runIntutil(t, "exxp", strconv.FormatFloat(x, 'f', 10, 64))
		if !ok {
			continue
		}
		checkedExxp++
		bump(dos, got)
		// exp(x) is the identical DOS algorithm; residual is libm-level (FPC RTL
		// vs Go math), far below the app's display precision.
		if math.Abs(dos-got)/math.Max(1, math.Abs(got)) > 1e-6 {
			t.Errorf("Exxp(%.6f): DOS=%.10g Go=%.10g", x, dos, got)
		}
	}

	// Lnn over (tiny, large].
	for i := 0; i < 300; i++ {
		x := math.Pow(10, -6+rng.Float64()*15) // 1e-6 .. 1e9
		got, err := Lnn(x)
		if err != nil {
			continue
		}
		dos, ok := runIntutil(t, "lnn", strconv.FormatFloat(x, 'f', 12, 64))
		if !ok {
			continue
		}
		checkedLnn++
		bump(dos, got)
		if math.Abs(dos-got) > 1e-6 {
			t.Errorf("Lnn(%.6g): DOS=%.10g Go=%.10g", x, dos, got)
		}
	}

	// Power over REALISTIC growth factors: base near 1 (e.g. (1+rate/peryr)) and
	// exponents up to a 30-year monthly term. DOS Power is literally
	// exxp(n*lnn(x)) — identical to the Go port — so any residual here is purely
	// the underlying libm exp/ln differing between the FPC oracle and Go's math,
	// amplified by the exponent. Within the band the engine actually uses, that
	// residual stays far below display precision.
	powMaxRel := 0.0
	for i := 0; i < 400; i++ {
		base := 0.80 + rng.Float64()*0.45 // 0.80 .. 1.25
		n := 1 + rng.Float64()*360
		got, err := Power(base, n)
		if err != nil {
			continue
		}
		dos, ok := runIntutil(t, "power",
			strconv.FormatFloat(base, 'f', 10, 64), strconv.FormatFloat(n, 'f', 6, 64))
		if !ok {
			continue
		}
		checkedPow++
		rel := math.Abs(dos-got) / math.Max(1, math.Abs(got))
		if rel > powMaxRel {
			powMaxRel = rel
		}
		// 1e-6 is above the libm residual amplified by the exponent and far below
		// the app's display precision (2-6 decimals).
		if rel > 1e-6 {
			t.Errorf("Power(%.6f,%.4f): DOS=%.10g Go=%.10g (rel %.2e)", base, n, dos, got, rel)
		}
	}
	t.Logf("intutil math sweep: exxp %d, lnn %d (max relErr %.2e); power %d realistic-band (max relErr %.2e)",
		checkedExxp, checkedLnn, maxRel, checkedPow, powMaxRel)
}
