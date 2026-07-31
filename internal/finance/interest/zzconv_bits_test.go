package interest

import (
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// Bit-level pinning of the two rate-conversion primitives against the REAL DOS
// INTSUTIL routines (docs/discrepancies.md §49).
//
// These two functions sit under every rate a user types or reads back, in all
// three screens:
//
//	INTSUTIL.pas:1270-1275  RateFromYield(yy,n) = nn*lnn(1 + yy/nn)
//	INTSUTIL.pas:1263-1268  YieldFromRate(rr,n) = nn*(exxp(rr/nn) - 1)
//	                        nn = RealPerYr(n)   (INTSUTIL.pas:1255-1261)
//
// Two separate 2026-07-31 findings were callers spelling these out by hand
// instead of calling them — the PV COLA conversion (§48, math.Log1p for
// lnn(1+y)) and the API's 365/360 kicker (§49, math.Exp/math.Log). In both
// cases the shared function was already correct and the caller bypassed it. So
// the primitives themselves need a guard that is independent of any caller: if
// these ever drift, every one of those call sites drifts silently with them.
//
// The oracle side uses `amort_oracle intutil rfybits|yfrbits VALUE N`, which
// calls the real INTSUTIL routine and prints the raw float64 bit pattern.
// Those are NEW intutil function names, so no existing intutil caller (three
// of which require the entire stdout to be one number) is affected.

func amortOracleBinForConv() string {
	if p := os.Getenv("PERSENSE_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/amort_oracle"
}

func dosConvBits(t *testing.T, bin, fn string, v float64, n int) (uint64, bool) {
	t.Helper()
	out, err := exec.Command(bin, "intutil", fn,
		strconv.FormatFloat(v, 'f', 12, 64), strconv.Itoa(n)).Output()
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" || strings.HasPrefix(s, "ERR") {
		return 0, false
	}
	b, e := strconv.ParseUint(s, 16, 64)
	return b, e == nil
}

func TestRateYieldConversionsMatchDOSBits(t *testing.T) {
	bin := amortOracleBinForConv()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("oracle required but not present (%s)", bin)
		}
		t.Skipf("amort oracle not present (%s); build via legacy/oracle/build_linux.sh", bin)
	}

	// n covers the plain frequencies plus the two RealPerYr special cases that
	// derive nn from yrdays (52 -> yrdays/7, 26 -> yrdays/14).
	ns := []int{1, 2, 3, 4, 6, 12, 24, 26, 52}
	// Values straddle DOS's |x| < Small (1e-4) Taylor branches in exxp and lnn,
	// which is where a bare math.Exp / math.Log diverges most (the API kicker
	// bug reached 706 ULP at rate 0.001, peryr 12 — x = 8.3e-5, inside Taylor).
	vals := []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.01, 0.05,
		0.075, 0.10, 0.125, 0.18, 0.25, 0.5, 1.0}

	// yrdays must match what the oracle's global holds for a default run: DOS
	// SetYrDays leaves 360 unless the basis is x365 (INTSUTIL.pas:333), and the
	// intutil probes do not change the basis.
	const yrDays = 360.0

	checked, bad := 0, 0
	for _, n := range ns {
		for _, v := range vals {
			gotR, errR := RateFromYield(v, byte(n), yrDays)
			if wantR, ok := dosConvBits(t, bin, "rfybits", v, n); ok && errR == nil {
				checked++
				if math.Float64bits(gotR) != wantR {
					bad++
					t.Errorf("RateFromYield(%v, %d): Go %016X (%.17g) DOS %016X (%.17g) %d ULP",
						v, n, math.Float64bits(gotR), gotR, wantR, math.Float64frombits(wantR),
						int64(math.Float64bits(gotR))-int64(wantR))
				}
			}
			gotY, errY := YieldFromRate(v, byte(n), yrDays)
			if wantY, ok := dosConvBits(t, bin, "yfrbits", v, n); ok && errY == nil {
				checked++
				if math.Float64bits(gotY) != wantY {
					bad++
					t.Errorf("YieldFromRate(%v, %d): Go %016X (%.17g) DOS %016X (%.17g) %d ULP",
						v, n, math.Float64bits(gotY), gotY, wantY, math.Float64frombits(wantY),
						int64(math.Float64bits(gotY))-int64(wantY))
				}
			}
		}
	}
	if checked < 200 {
		t.Fatalf("only %d conversions compared; the probe is not reaching the oracle", checked)
	}
	t.Logf("rate/yield conversion bit-fidelity: checked=%d divergences=%d", checked, bad)
}
