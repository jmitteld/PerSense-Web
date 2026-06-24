package interest

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func oracleBin() string {
	if p := os.Getenv("PERSENSE_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/amort_oracle"
}

func oracleIntutil(args ...string) (string, bool) {
	full := append([]string{"intutil"}, args...)
	out, err := exec.Command(oracleBin(), full...).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// TestFuzzRound2VsDOS hammers interest.Round2 against the real DOS Round2,
// concentrating on exact half-cent boundaries (x.xx5) where round-half-DOWN
// behavior diverges from round-half-up, plus a broad random sweep.
func TestFuzzRound2VsDOS(t *testing.T) {
	if _, err := os.Stat(oracleBin()); err != nil {
		t.Skip("oracle not present")
	}
	checked, fails := 0, 0

	check := func(x float64) {
		oraStr, ok := oracleIntutil("round2", strconv.FormatFloat(x, 'f', 10, 64))
		if !ok {
			return
		}
		checked++
		goStr := fmt.Sprintf("%.6f", Round2(x))
		// Oracle prints with :0:6; normalize -0.000000.
		if oraStr == "-0.000000" {
			oraStr = "0.000000"
		}
		gs := goStr
		if gs == "-0.000000" {
			gs = "0.000000"
		}
		if gs != oraStr {
			fails++
			if fails <= 25 {
				t.Errorf("Round2(%s): DOS=%s Go=%s", strconv.FormatFloat(x, 'f', 10, 64), oraStr, gs)
			}
		}
	}

	// Exact half-cent boundaries: i + n/1000 + 0.005, both signs.
	for cents := 0; cents < 100; cents++ {
		base := float64(cents)/100.0 + 0.005
		for _, whole := range []float64{0, 1, 7, 100, 12345} {
			check(whole + base)
			check(-(whole + base))
		}
	}
	// Values just below/above the half-cent (round-down should still hold at
	// exactly .5; just-above rounds up).
	for cents := 0; cents < 100; cents++ {
		b := float64(cents) / 100.0
		check(b + 0.0049999)
		check(b + 0.0050001)
		check(b + 0.005)
	}
	// Broad random sweep across magnitudes.
	rng := rand.New(rand.NewSource(0x52306e64))
	for i := 0; i < 4000; i++ {
		mag := math.Pow(10, rng.Float64()*7-1) // 0.1 .. 1e6
		x := (rng.Float64()*2 - 1) * mag
		check(x)
	}
	t.Logf("Round2 vs DOS: checked %d, divergences %d", checked, fails)
}

// TestFuzzTranscendentalVsDOS compares Exxp/Lnn/Power to the DOS versions,
// including the near-singularity Taylor-series regions.
func TestFuzzTranscendentalVsDOS(t *testing.T) {
	if _, err := os.Stat(oracleBin()); err != nil {
		t.Skip("oracle not present")
	}
	rng := rand.New(rand.NewSource(987654321))
	cmp := func(label, oraStr string, goVal float64, goErr error) (bad bool) {
		if goErr != nil {
			return false
		}
		ora, err := strconv.ParseFloat(oraStr, 64)
		if err != nil {
			return false
		}
		rel := math.Abs(ora-goVal) / math.Max(1e-9, math.Abs(goVal))
		if rel > 1e-9 && math.Abs(ora-goVal) > 1e-9 {
			t.Errorf("%s: DOS=%s Go=%.12f (rel %.2e)", label, oraStr, goVal, rel)
			return true
		}
		return false
	}

	exxpChecked, lnnChecked, powChecked := 0, 0, 0
	for i := 0; i < 3000; i++ {
		// Exxp over [-80, 80], with extra density near 0 (Taylor region).
		x := rng.Float64()*160 - 80
		if rng.Intn(3) == 0 {
			x = (rng.Float64()*2 - 1) * 1e-4
		}
		if s, ok := oracleIntutil("exxp", strconv.FormatFloat(x, 'f', 12, 64)); ok {
			v, e := Exxp(x)
			cmp(fmt.Sprintf("Exxp(%.12f)", x), s, v, e)
			exxpChecked++
		}
		// Lnn over (0, 1e6], extra density near 1.
		y := math.Pow(10, rng.Float64()*6-3)
		if rng.Intn(3) == 0 {
			y = 1 + (rng.Float64()*2-1)*1e-4
		}
		if s, ok := oracleIntutil("lnn", strconv.FormatFloat(y, 'f', 12, 64)); ok {
			v, e := Lnn(y)
			cmp(fmt.Sprintf("Lnn(%.12f)", y), s, v, e)
			lnnChecked++
		}
		// Power: base (0,1000], exponent [-5,5].
		b := rng.Float64() * 1000
		n := rng.Float64()*10 - 5
		if s, ok := oracleIntutil("power", strconv.FormatFloat(b, 'f', 10, 64), strconv.FormatFloat(n, 'f', 10, 64)); ok {
			v, e := Power(b, n)
			cmp(fmt.Sprintf("Power(%.6f,%.6f)", b, n), s, v, e)
			powChecked++
		}
	}
	t.Logf("transcendental vs DOS: Exxp=%d Lnn=%d Power=%d", exxpChecked, lnnChecked, powChecked)
}
