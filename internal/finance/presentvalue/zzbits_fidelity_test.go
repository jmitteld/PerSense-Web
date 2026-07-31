package presentvalue

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

// Bit-level differential against the real DOS PV engine
// (docs/discrepancies.md §48, §49).
//
// THIS TEST IS THE ONE THAT FOUND §48. Run as a throwaway shell script on
// 2026-07-31, comparing the screen total as a raw float64 instead of as the
// oracle's 6dp text, it reported 2,063 divergences in 5,040 cells — while every
// decimal-tolerance sweep in this package was reporting zero. The cause was the
// COLA yield->continuous conversion using math.Log1p(y) where DOS uses
// RateFromYield(y,1) = lnn(1+y) (INTSUTIL.pas:1270 / :1601). After the fix the
// same sweep reports 0. It is checked in here so that capability is standing
// rather than reconstructed each time.
//
// The grid deliberately holds cola == 0 cells alongside non-zero ones: in the
// original run divergences were identically zero on every cola == 0 cell, and
// that contrast is what localized the defect to the conversion rather than to
// the PV engine. Keep both.
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

// pvTableBits runs the oracle's table mode and returns the screen total's bits.
func pvTableBits(bin string, rate float64, basis, colaMonth string,
	from, to, asof string, perYr int, amt, cola float64) (uint64, bool) {
	cmd := exec.Command(bin, "table",
		strconv.FormatFloat(rate, 'f', 10, 64), basis, "detail", "none", colaMonth,
		"asof="+asof,
		fmt.Sprintf("per=%s:%s:%d:%s:%s", from, to, perYr,
			strconv.FormatFloat(amt, 'f', 2, 64),
			strconv.FormatFloat(cola, 'f', 10, 64)))
	cmd.Env = append(os.Environ(), "PERSENSE_ORACLE_RAWBITS=1")
	out, err := cmd.Output()
	if err != nil || strings.Contains(string(out), "ERR") {
		return 0, false
	}
	v, ok := parseRawBits(string(out))["pv"]
	return v, ok
}

func TestDOSPVScreenTotalBitFidelity(t *testing.T) {
	bin := pvOracleBin()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("PV oracle required but not present (%s)", bin)
		}
		t.Skipf("PV oracle not present (%s)", bin)
	}

	bases := []struct {
		name string
		typ  types.BasisType
		yrD  float64
	}{
		{"360", types.Basis360, 360},
		{"365", types.Basis365, 365.25},
		{"365360", types.Basis365360, 360},
	}
	colaMonths := []struct {
		name string
		val  byte
	}{
		{"ann", types.COLAAnnual},
		{"cnt", types.COLAContinuous},
		{"6", 6},
		{"11", 11},
	}
	perYrs := []int{2, 3, 4, 6, 12, 24, 26, 52}
	rates := []float64{0.04, 0.075, 0.10, 0.125, 0.18}
	// cola == 0 is load-bearing — see the header.
	colas := []float64{0, 0.02, 0.05, 0.10}

	const from, to, asof = "1.6.2030", "1.6.2085", "1.1.2028"
	fromD := types.NewDateRec(2030, time.June, 1)
	toD := types.NewDateRec(2085, time.June, 1)
	asofD := types.NewDateRec(2028, time.January, 1)

	checked, diverged, zeroCola := 0, 0, 0
	worst := int64(0)
	for _, b := range bases {
		for _, cm := range colaMonths {
			for _, py := range perYrs {
				for _, rate := range rates {
					for _, cola := range colas {
						want, ok := pvTableBits(bin, rate, b.name, cm.name, from, to, asof, py, 1000.00, cola)
						if !ok {
							continue
						}
						s := PVSettings{Basis: b.typ, PerYr: 12, COLAMonth: cm.val,
							YrDays: b.yrD, YrInv: 1 / b.yrD}
						p := PeriodicPayment{
							FromDateStatus: types.InOutInput, FromDate: fromD,
							ToDateStatus: types.InOutInput, ToDate: toD,
							PerYrStatus: types.InOutInput, PerYr: py,
							AmtStatus: types.InOutInput, Amt: 1000.00,
						}
						if cola != 0 {
							p.COLAStatus, p.COLA = types.InOutInput, cola
						}
						res := Calculate(PVInput{
							Periodics: []PeriodicPayment{p},
							PresVal: PresValLine{
								AsOfStatus: types.InOutInput, AsOf: asofD,
								R: RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
							},
							Settings: s,
						})
						if res.Err != nil {
							continue
						}
						checked++
						if cola == 0 {
							zeroCola++
						}
						got := math.Float64bits(res.SumValue)
						if got == want {
							continue
						}
						diverged++
						d := int64(got) - int64(want)
						if d < 0 {
							d = -d
						}
						if d > worst {
							worst = d
						}
						if diverged <= 10 {
							t.Errorf("pv bits differ: basis=%s peryr=%d rate=%v cola=%v colamonth=%s\n"+
								"  Go  %016X (%.17g)\n  DOS %016X (%.17g)\n  %d ULP",
								b.name, py, rate, cola, cm.name,
								got, res.SumValue, want, math.Float64frombits(want), d)
						}
					}
				}
			}
		}
	}
	if checked < 1000 {
		t.Fatalf("only %d cells compared; the differential is not exercising the engine", checked)
	}
	t.Logf("PV screen-total bit-fidelity: checked=%d (%d at cola=0) divergences=%d worst=%d ULP",
		checked, zeroCola, diverged, worst)
}
