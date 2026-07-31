package mortgage

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Bit-level differential against the real DOS mortgage engine
// (docs/discrepancies.md §49). See the header of the amortization counterpart,
// internal/finance/amortization/zzbits_fidelity_test.go, for why a decimal
// differential cannot certify bit fidelity.
//
// RESULT WHEN WRITTEN: 300 of 300 randomized solve-monthly cases bit-identical
// on all four outputs (monthly, price, cash, financed), 0 divergences. The
// mortgage engine needed no change; this is a guard, not a fix. That outcome is
// itself worth recording — a 2026-07-31 audit of both engines for the
// bypass-the-DOS-primitive defect that hit PV (§48) found every transcendental
// already routed through interest.Exxp / Lnn / RateFromYield / YieldFromRate
// with DOS's own n argument. This test is the empirical confirmation.
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

func TestDOSMtgBitFidelity(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("mortgage oracle required but not present (%s)", bin)
		}
		t.Skipf("mortgage oracle not present (%s); build via TARGET=mtg_oracle legacy/oracle/build_linux.sh", bin)
	}
	env := append(os.Environ(), "PERSENSE_ORACLE_RAWBITS=1")
	rng := rand.New(rand.NewSource(20630))

	checked, diverged, noBits := 0, 0, 0
	worst := int64(0)
	for i := 0; i < 300; i++ {
		price := quantize(float64(50000+rng.Intn(950000)), 2)
		pct := quantize(0.05+rng.Float64()*0.45, 10)
		years := 5 + rng.Intn(35)
		rate := quantize(0.01+rng.Float64()*0.16, 10) // true (continuous) rate

		cmd := exec.Command(bin, "monthly",
			strconv.FormatFloat(price, 'f', 2, 64),
			strconv.FormatFloat(pct, 'f', 10, 64),
			strconv.Itoa(years),
			strconv.FormatFloat(rate, 'f', 10, 64))
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		bits := parseRawBits(string(out))
		if len(bits) == 0 {
			noBits++
			continue
		}
		r := Calc(MtgLine{
			PriceStatus: types.InOutInput, Price: price,
			PctStatus: types.InOutInput, Pct: pct,
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			PointsStatus: types.InOutInput, Points: 0,
			TaxStatus: types.InOutInput, Tax: 0,
			MonthlyStatus: types.StatusEmpty,
		})
		if r.Err != nil {
			continue
		}
		checked++
		for _, f := range []struct {
			key string
			got float64
		}{
			{"monthly", r.Line.Monthly},
			{"price", r.Line.Price},
			{"cash", r.Line.Cash},
			{"financed", r.Line.Financed},
		} {
			want, ok := bits[f.key]
			if !ok {
				continue
			}
			g := math.Float64bits(f.got)
			if g == want {
				continue
			}
			diverged++
			d := int64(g) - int64(want)
			if d < 0 {
				d = -d
			}
			if d > worst {
				worst = d
			}
			if diverged <= 10 {
				t.Errorf("%s bits differ: price=%.2f pct=%.10f years=%d rate=%.10f\n"+
					"  Go  %016X (%.17g)\n  DOS %016X (%.17g)\n  %d ULP",
					f.key, price, pct, years, rate, g, f.got, want, math.Float64frombits(want), d)
			}
		}
	}
	if noBits > 0 {
		t.Errorf("%d runs produced no RAWBITS line — is PERSENSE_ORACLE_RAWBITS reaching the oracle?", noBits)
	}
	if checked < 200 {
		t.Fatalf("only %d cases compared; the differential is not exercising the engine", checked)
	}
	t.Logf("mortgage bit-fidelity: checked=%d (x4 fields) divergences=%d worst=%d ULP", checked, diverged, worst)
}
