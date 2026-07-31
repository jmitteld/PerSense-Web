package amortization

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// Bit-level differential against the real DOS amortization engine
// (docs/discrepancies.md §49).
//
// WHY A SECOND DIFFERENTIAL. The existing sweeps compare the oracle's PRINTED
// decimals with tolerances (this package's TestDOSDifferentialSweep uses 1e-4
// relative on the payment). That can never certify bit fidelity, for two
// independent reasons: Pascal's ':0:N' write double-rounds (§48), and the
// tolerance is also silently absorbing input quantization (see quantize below).
// A systematic last-bits divergence can therefore sit under every existing
// sweep indefinitely — which is exactly what happened to the PV COLA
// conversion. This test closes that gap for the amortization payment.
//
// RESULT WHEN WRITTEN: 400 of 400 randomized cases bit-identical, 0 divergences.
// The amortization engine needed no change; this is a guard, not a fix.
//
// SCOPE NOTE — why only the payment. The oracle's 'interest' and 'paid' figures
// are NOT engine doubles: amort_oracle re-parses them out of the already
// formatted 2-decimal totals line (NumAfter(totalsLine, ...)), so their low bits
// carry no engine information and the driver deliberately does not emit raw bits
// for them. Those two stay covered as decimals by the existing sweeps.
// parseRawBits reads the oracle's `RAWBITS k=HEX|k=HEX` line into a map.
//
// The oracle emits it only when PERSENSE_ORACLE_RAWBITS is set in its
// environment; with the variable unset its stdout is byte-identical to what it
// was before the mechanism existed, so no other parser in the corpus is
// affected. See legacy/oracle/OracleBits.pas for the emission contract.
func parseRawBits(out string) map[string]uint64 {
	m := map[string]uint64{}
	for _, ln := range strings.Split(out, "\n") {
		if !strings.HasPrefix(ln, "RAWBITS ") {
			continue
		}
		for _, kv := range strings.Split(strings.TrimSpace(ln[len("RAWBITS "):]), "|") {
			p := strings.SplitN(kv, "=", 2)
			if len(p) != 2 {
				continue
			}
			if v, err := strconv.ParseUint(p[1], 16, 64); err == nil {
				m[p[0]] = v
			}
		}
	}
	return m
}

// quantize returns the exact float64 the oracle will parse back from the
// decimal string we hand it.
//
// THIS IS NOT OPTIONAL FOR A BIT-LEVEL COMPARISON. Every oracle argument
// crosses the process boundary as text. If Go keeps a full-precision float
// while the oracle parses a rounded rendering of it, the two engines start from
// DIFFERENT doubles and the differential measures input quantization rather
// than fidelity. Measured: an unquantized rate at 6dp made 400 of 400
// amortization payments compare unequal, worst case ~4.8e10 ULP (~1e-5
// relative); quantizing made all 400 bit-identical with no engine change. It is
// also why the older decimal sweeps need tolerances near 1e-4 — they are
// absorbing this, not engine error.
func quantize(x float64, decimals int) float64 {
	v, err := strconv.ParseFloat(strconv.FormatFloat(x, 'f', decimals, 64), 64)
	if err != nil {
		return x
	}
	return v
}

func TestDOSAmortPaymentBitFidelity(t *testing.T) {
	bin := oracleBin
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("oracle required but not present (%s)", bin)
		}
		t.Skipf("amort oracle not present (%s); build via legacy/oracle/build_linux.sh", bin)
	}
	env := append(os.Environ(), "PERSENSE_ORACLE_RAWBITS=1")
	rng := rand.New(rand.NewSource(20620))

	checked, diverged, noBits := 0, 0, 0
	worst := int64(0)
	for i := 0; i < 400; i++ {
		amount := quantize(float64(1000+rng.Intn(999000)), 2)
		rate := quantize(0.005+rng.Float64()*0.18, 6)
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := (2 + rng.Intn(38)) * perYr

		cmd := exec.Command(bin,
			strconv.FormatFloat(amount, 'f', 2, 64),
			strconv.FormatFloat(rate, 'f', 6, 64),
			strconv.Itoa(n), strconv.Itoa(perYr))
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		bits, ok := parseRawBits(string(out))["payment"]
		if !ok {
			noBits++
			continue
		}
		gp, _, ok2 := goSolve(amount, rate, n, perYr)
		if !ok2 {
			continue
		}
		checked++
		got := math.Float64bits(gp)
		if got == bits {
			continue
		}
		diverged++
		d := int64(got) - int64(bits)
		if d < 0 {
			d = -d
		}
		if d > worst {
			worst = d
		}
		if diverged <= 10 {
			t.Errorf("payment bits differ: amount=%.2f rate=%.6f n=%d peryr=%d\n"+
				"  Go  %016X (%.17g)\n  DOS %016X (%.17g)\n  %d ULP",
				amount, rate, n, perYr, got, gp, bits, math.Float64frombits(bits), d)
		}
	}
	if noBits > 0 {
		t.Errorf("%d runs produced no RAWBITS line — is PERSENSE_ORACLE_RAWBITS reaching the oracle?", noBits)
	}
	if checked < 300 {
		t.Fatalf("only %d cases compared; the differential is not exercising the engine", checked)
	}
	t.Logf("amortization payment bit-fidelity: checked=%d divergences=%d worst=%d ULP", checked, diverged, worst)
}
