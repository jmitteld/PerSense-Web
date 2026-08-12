package amortization

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ITEM 0j — `accTol := accLimit * accInit`, fancybisect.go. REDONE IN ROUND 54.
//
// 🚨 PROVENANCE. Filed by round 50; round 50's code was never pushed and is
// lost, so this is a redo, not a recovery.
//
// THE DEFECT. DOS's Iterate accepts a result unless BOTH residual tests fail:
//
//	AMORTOP.pas:1487
//	  if (bestp > halfpenny) and (bestp > acc_limit * init) then  { refuse }
//
// with `acc_limit = 2E-8` (:1423) and `init := p` (:1435) — the SIGNED starting
// balance. The port wrote `accLimit * math.Abs(accInit)`.
//
// The two disagree exactly when `init` is negative. There, DOS's threshold is
// NEGATIVE; `bestp` is an absolute value and so is always >= 0; the relative
// clause is therefore always true and CANNOT rescue a residual the half-penny
// test already rejected — DOS refuses. Taking the magnitude instead turns the
// threshold positive and lets the port ACCEPT a result DOS declines.
//
// ⭐ THE CROSS-CHECK THAT MAKES THIS MORE THAN A READING. The port's OTHER
// engine already ports the clause correctly — dosport_walk.go:586 is
// `bestp > halfpenny && bestp > accLimit*p0`, with no magnitude. The two
// engines disagreed with each other about one ported line, and only one of
// them agreed with the Pascal. R50's "assert ACROSS files" in the other
// direction: the tree itself carried the answer.
//
// 🚨🚨 THE GATE IS WEAKER THAN USUAL AND THIS MUST BE SAID WHENEVER IT IS
// CITED. No reachable screen is known that flips on it: the APR class arm was
// measured at 562 acceptance decisions with ZERO carrying a negative accInit
// (round 50's finding, carried through r51-r53 and NOT re-derived here). So
// the behavioural guard below drives dosIterateCore with a SYNTHETIC TERMINAL
// rather than a screen, and it is a FIDELITY guard, not a defect closure. The
// round-54 paired regression over seeds 50100-50109, N=400, measured
// FIXED = 0 / NEW = 0 — this change moves nothing on the standing population,
// which is the expected result, not a disappointing one.

// TestR50Item0jNegativeInitRefusesLikeDOS is the behavioural guard.
//
// The terminal is synthetic and chosen to make the arithmetic legible:
//   - it is smooth and monotone, so the secant converges to its best estimate;
//   - its residual settles ABOVE the half-penny (so the absolute clause fails);
//   - accInit is NEGATIVE, so DOS's relative clause also fails and DOS refuses.
//
// With `math.Abs` the threshold becomes +2e-8*|init|, which the residual may
// clear — the port then reports converged where DOS reports non-convergence.
func TestR50Item0jNegativeInitRefusesLikeDOS(t *testing.T) {
	// A terminal whose root the secant cannot reach to within half a penny:
	// a steep, offset line whose zero sits where the iteration stalls on the
	// count>=20 arm. residual(x) = 1000*(x-7) + 0.02 keeps |p| well above
	// halfpenny at the best x the loop finds within its budget.
	const resid = 0.02
	terminal := func(x float64) float64 {
		// Deliberately shallow near the seed so 20 passes cannot close the
		// gap to half a penny, but well-behaved enough to have a bestp.
		return resid + 1e-12*(x-7)
	}

	// |accInit| large enough that 2e-8*|accInit| EXCEEDS the residual: this is
	// precisely the regime where the magnitude version rescues the answer.
	// 2e-8 * 5e6 = 0.10 > 0.02 = residual.
	const mag = 5.0e6

	// POSITIVE init: DOS's own relative clause applies and ACCEPTS. This is the
	// positive control — it proves the relative limb is REACHED and doing work,
	// so the negative case below is not passing vacuously (R49/R51).
	if _, ok := dosIterateCore(7.0, +mag, terminal, terminal, nil, false); !ok {
		t.Fatalf("positive control: init=+%g should be ACCEPTED by the relative "+
			"clause (2e-8*%g = %g > residual %g) — the limb is not being reached, "+
			"so the negative assertion below would be vacuous",
			mag, mag, accLimitFor(mag), resid)
	}

	// NEGATIVE init: DOS's threshold is negative, the relative clause cannot
	// rescue, and DOS refuses.
	if _, ok := dosIterateCore(7.0, -mag, terminal, terminal, nil, false); ok {
		t.Errorf("item 0j: init=-%g was ACCEPTED. DOS computes acc_limit*init = %g "+
			"(NEGATIVE), so `bestp > acc_limit*init` is true for every bestp and "+
			"Iterate returns false (AMORTOP.pas:1487). Taking math.Abs(init) turns "+
			"the threshold into +%g and accepts a residual DOS declines.",
			mag, -accLimitFor(mag), accLimitFor(mag))
	}
}

// accLimitFor mirrors the constant so the failure messages carry real numbers
// rather than a restated formula. A guard's failure message is a claim; this
// keeps the claim checkable.
func accLimitFor(v float64) float64 { return 2e-8 * math.Abs(v) }

// TestR50Item0jSourceAgreesAcrossEngines is the across-files assertion (R50: a
// self-reading guard is unconditionally true — this one reads a DIFFERENT file
// from the one it protects).
//
// It pins the two engines to the SAME rendering of AMORTOP.pas:1487: neither
// may take the magnitude of the acceptance base. If a future edit reintroduces
// `math.Abs` at either site, this fails and names the file.
func TestR50Item0jSourceAgreesAcrossEngines(t *testing.T) {
	for _, f := range []struct{ path, want string }{
		{"fancybisect.go", "accTol := accLimit * accInit"},
		{"dosport_walk.go", "bestp > accLimit*p0"},
	} {
		b, err := os.ReadFile(filepath.Join(".", f.path))
		if err != nil {
			t.Fatalf("read %s: %v", f.path, err)
		}
		s := string(b)
		if !strings.Contains(s, f.want) {
			t.Errorf("%s: expected the DOS acceptance clause as %q — "+
				"AMORTOP.pas:1487 takes acc_limit*init on the SIGNED balance",
				f.path, f.want)
		}
		if strings.Contains(s, "accLimit * math.Abs(accInit)") ||
			strings.Contains(s, "accLimit*math.Abs(p0)") {
			t.Errorf("%s: the acceptance base is wrapped in math.Abs again — "+
				"item 0j. DOS refuses when init is negative; the magnitude "+
				"version accepts.", f.path)
		}
	}
}
