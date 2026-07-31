package api

import (
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Bit-level guard for the 365/360 rate kicker at the API boundary
// (docs/discrepancies.md §49).
//
// pvKickerRate / pvUnkickerRate implement DOS's PercentValueFromCell vratecol
// x365_360 arm, INTSUTIL.pas:1611-1614:
//
//	PercentValueFromCell := RateFromYield(YieldFromRate(rp^,n)/kicker,n)
//
// Until 2026-07-31 they spelled that out by hand as
// `n*(math.Exp(r/n)-1)` and `n*math.Log(1+y*k/n)`. That is the same
// bypass-the-DOS-primitive defect class as the PV COLA conversion (§48), with
// three distinct consequences: math.Exp and math.Log are not correctly rounded
// where FPC's are (13.67% / 9.38% of arguments — interest/crmath.go), and the
// bare calls skip DOS's exxp Taylor branch for |x| < 1e-4 and its ±70 guards.
//
// The oracle side uses `amort_oracle intutil kickbits RATE N SCALE`, which calls
// the REAL INTSUTIL RateFromYield/YieldFromRate and prints the raw float64 bit
// pattern. Bits rather than decimals because Pascal's `:0:6` double-rounds —
// see legacy/oracle/OracleBits.pas.

func amortOracleBinForBits() string {
	if p := os.Getenv("PERSENSE_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/amort_oracle"
}

// dosKickBits asks the real DOS engine for
// RateFromYield(YieldFromRate(rate,n)*scale, n) as raw bits.
func dosKickBits(t *testing.T, bin string, rate float64, n int, scale float64) (uint64, bool) {
	t.Helper()
	out, err := exec.Command(bin, "intutil", "kickbits",
		strconv.FormatFloat(rate, 'f', 12, 64),
		strconv.Itoa(n),
		strconv.FormatFloat(scale, 'f', 17, 64)).Output()
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" || strings.HasPrefix(s, "ERR") {
		return 0, false
	}
	v, e := strconv.ParseUint(s, 16, 64)
	return v, e == nil
}

func TestPVKickerRateMatchesDOSBits(t *testing.T) {
	bin := amortOracleBinForBits()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("oracle required but not present (%s)", bin)
		}
		t.Skipf("amort oracle not present (%s); build via legacy/oracle/build_linux.sh", bin)
	}

	// n = RealPerYr(peryr, yrdays). The API always passes the settings
	// frequency; see the scoped-gap note on pvKickerRate about DOS's per-row
	// override, which the port cannot reach (RateLine has no PerYr field).
	perYrs := []byte{1, 2, 4, 12, 24}
	rates := []float64{0.001, 0.01, 0.025, 0.05, 0.075, 0.10, 0.125, 0.18, 0.25, 0.40}

	checked, bad := 0, 0
	for _, py := range perYrs {
		for _, r := range rates {
			// yrdays is 360 for the 365/360 basis (DOS SetYrDays,
			// INTSUTIL.pas:333) — x365_360 counts actual days over a 360
			// denominator, so yrdays stays 360.
			const yrDays = 360.0

			// Forward: displayed -> internal, scale = +kicker.
			gotFwd := pvKickerRate(r, py, yrDays, types.Basis365360)
			wantFwd, ok := dosKickBits(t, bin, r, int(py), pvBasisKicker)
			if !ok {
				t.Fatalf("oracle kickbits failed for rate=%v n=%d", r, py)
			}
			checked++
			if math.Float64bits(gotFwd) != wantFwd {
				bad++
				t.Errorf("pvKickerRate(%v, peryr=%d): got %016X (%.17g), DOS %016X (%.17g), %d ULP",
					r, py, math.Float64bits(gotFwd), gotFwd,
					wantFwd, math.Float64frombits(wantFwd),
					int64(math.Float64bits(gotFwd))-int64(wantFwd))
			}

			// Inverse: internal -> displayed, scale = 1/kicker. This is the
			// direction DOS itself writes at INTSUTIL.pas:1614.
			gotInv := pvUnkickerRate(r, py, yrDays, types.Basis365360)
			wantInv, ok := dosKickBits(t, bin, r, int(py), 1/pvBasisKicker)
			if !ok {
				t.Fatalf("oracle kickbits failed for rate=%v n=%d (inverse)", r, py)
			}
			checked++
			if math.Float64bits(gotInv) != wantInv {
				bad++
				t.Errorf("pvUnkickerRate(%v, peryr=%d): got %016X (%.17g), DOS %016X (%.17g), %d ULP",
					r, py, math.Float64bits(gotInv), gotInv,
					wantInv, math.Float64frombits(wantInv),
					int64(math.Float64bits(gotInv))-int64(wantInv))
			}
		}
	}
	t.Logf("kicker bit-fidelity: checked=%d divergences=%d", checked, bad)

	// Both bases that do not apply the kicker must be exact identities.
	for _, b := range []types.BasisType{types.Basis360, types.Basis365} {
		if v := pvKickerRate(0.0825, 12, 360, b); v != 0.0825 {
			t.Errorf("pvKickerRate must be identity on basis %v, got %v", b, v)
		}
		if v := pvUnkickerRate(0.0825, 12, 360, b); v != 0.0825 {
			t.Errorf("pvUnkickerRate must be identity on basis %v, got %v", b, v)
		}
	}
}

// TestPVKickerRoundTripsThroughDOSPrimitives checks the two directions invert
// each other to within a couple of ULP, which the hand-inlined version also did
// — this is here so a future edit cannot fix the bit test by breaking the
// inverse relationship the UI echo depends on.
func TestPVKickerRoundTripsThroughDOSPrimitives(t *testing.T) {
	for _, py := range []byte{1, 12, 24} {
		for _, r := range []float64{0.01, 0.05, 0.10, 0.25} {
			internal := pvKickerRate(r, py, 360, types.Basis365360)
			back := pvUnkickerRate(internal, py, 360, types.Basis365360)
			if math.Abs(back-r) > 1e-12 {
				t.Errorf("round trip peryr=%d rate=%v: internal=%v back=%v (drift %g)",
					py, r, internal, back, back-r)
			}
		}
	}
}
