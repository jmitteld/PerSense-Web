package amortization

import (
	"math"
	"os"
	"strings"
	"testing"
)

// zzr48_tolerances_test.go — ROUND 48. ITEM 0e: THE NINE HARD-DECIDING
// TOLERANCES, NAMED AND PINNED.
//
// WHAT ROUND 47 MEASURED. `dos_fuzzer5_test.go` decides every HARD verdict this
// project publishes at NINE sites, in SIX distinct algebraic forms, and every
// one of them was a BARE INLINE LITERAL. Not one was a named constant, and not
// one was pinned by any test. Seven mutations, each loosening one floor by two
// to four orders of magnitude, ALL left the suite green — including
// `:2910`'s `2e-6`, the tolerance that decides Signal 6's 20-in-1,856, which is
// 20 of the 25 remaining in-scope divergences. A published rate whose deciding
// constant can be moved four orders of magnitude with no test failing is a rate
// with no lower bound on its own meaning.
//
// 🚨 AND `zztacktolerance_test.go` DID NOT HELP, IN FOUR SEPARATE WAYS. Round 47
// measured it pinning four things VACUOUSLY: a hand-retyped mirror with no link
// to the line it mirrors (its header comment CLAIMED a link; the claim was
// false, measured in both directions), two scale-invariant spread bounds, and
// `:376`, where the constant ALGEBRAICALLY CANCELS. That comment is corrected in
// this round and `fz5TackTol` now DELEGATES here, so the mirror is a real link
// rather than a claimed one.
//
// WHAT THIS FILE DOES, AND WHAT IT CANNOT DO:
//
//	✅ Every floor and every relative coefficient is a NAMED CONSTANT, used by
//	   the fuzzer's own comparison sites. Changing a value fails
//	   TestToleranceValuesArePinned by name and says which published number moves.
//	✅ A source guard asserts the nine sites still REFERENCE the named helpers,
//	   so "loosen it back to an inline literal" is caught rather than silently
//	   reintroducing exactly the state round 47 found.
//	⚠️ IT CANNOT MAKE A LOOSENED TOLERANCE FAIL A DIFFERENTIAL RUN. Nothing can:
//	   a wider tolerance means fewer findings, and a suite with fewer findings is
//	   greener. That asymmetry is why these had to be pinned BY VALUE rather than
//	   by behaviour, and it is why item 0e was worth a round.

// ---------------------------------------------------------------------------
// THE CONSTANTS. Changing any line here restates a published number; each says
// which one.
// ---------------------------------------------------------------------------

const (
	// TOTALS (dos_fuzzer5_test.go :2456-2457 -> :2462). Decides the headline
	// in-scope HARD rate — 25 in 2,091 on `reached`.
	//
	// 🚨 THIS FLOOR CROSSES OVER AT $2,000 AND THEN GROWS WITHOUT BOUND. At
	// $55,940 of interest it is $27.97; at $1,000,000 it is $500. It is the
	// loosest tolerance in the project by a wide margin and the one most worth
	// arguing about with the client.
	tolTotalsFloor = 1.00
	tolTotalsRel   = 5e-4

	// BALLOON TACK (:2534-2535 -> :2537). Decides the balloon tack-on class.
	tolTackFloor   = 0.05
	tolTackLoanRel = 1e-5
	tolTackAmtRel  = 5e-4

	// BACKWARD SOLVE, AMOUNT (:2601 -> :2603).
	tolSolveAmtFloor = 0.01
	tolSolveAmtRel   = 2e-6

	// BACKWARD SOLVE, RATE (:2617). A flat rate-space tolerance, not a money one.
	tolSolveRate = 5e-6

	// REGULAR PAYMENT (:2682 -> :2685), and the APR modal-payment discriminator
	// (:2881), which compares the same quantity.
	tolPayFloor = 0.011
	tolPayRel   = 2e-6

	// ADJUSTMENT ROW AMOUNT (:2782 -> :2792). Decides Signal 7, 30 findings over
	// 858 rows.
	//
	// ⚠️ Numerically identical to the payment tolerance TODAY. Deliberately a
	// SEPARATE NAME: it decides a different signal over a different population,
	// and one of the two moving should not silently move the other.
	tolAdjAmtFloor = 0.011
	tolAdjAmtRel   = 2e-6

	// ADJUSTMENT ROW RATE (:2815).
	tolAdjRate = 5e-6

	// 🚨 APR (:2910). THE ONE THAT MATTERS MOST. Decides Signal 6 — 20 in 1,856,
	// 1 in 93, which is 20 of the 25 remaining in-scope divergences and the
	// entire open APR-R class. Round 47 loosened this to 2e-1 — five orders of
	// magnitude — and the suite stayed green.
	tolAPR = 2e-6
)

// The five composite forms, as functions (the sixth and seventh forms are the
// two FLAT tolerances, tolSolveRate and tolAdjRate, which need no function), so
// that the fuzzer's sites and every pin share ONE
// expression rather than a retyped copy (that retyping is what made
// zztacktolerance_test.go vacuous).

func tolTotals(dosValue float64) float64 {
	return math.Max(tolTotalsFloor, tolTotalsRel*math.Abs(dosValue))
}

func tolTack(loanAmount, dosTack float64) float64 {
	return math.Max(math.Max(tolTackFloor, tolTackLoanRel*math.Abs(loanAmount)),
		tolTackAmtRel*math.Abs(dosTack))
}

func tolSolveAmount(dosValue float64) float64 {
	return tolSolveAmtFloor + tolSolveAmtRel*math.Abs(dosValue)
}

func tolPayment(dosValue float64) float64 {
	return tolPayFloor + tolPayRel*math.Abs(dosValue)
}

func tolAdjAmount(dosValue float64) float64 {
	return tolAdjAmtFloor + tolAdjAmtRel*math.Abs(dosValue)
}

// ---------------------------------------------------------------------------
// THE PIN.
// ---------------------------------------------------------------------------

// TestToleranceValuesArePinned is the ONE test item 0e asks for: it fails when
// any of the nine deciding tolerances changes, and names the published figure
// that moves with it.
//
// 🚨 IT PINS BY VALUE, ON PURPOSE. R47's lesson from `zztacktolerance_test.go`
// is that a "scaling" or "spread" assertion is SCALE-INVARIANT — multiply the
// whole tolerance by 1,000 and a ratio test still passes. Only a literal
// expected value can fail on a loosening, so this test is deliberately the dull
// kind. If you are editing it because a value changed, the value changing IS the
// event: say which number moved, in the round record, before you edit the line.
func TestToleranceValuesArePinned(t *testing.T) {
	for _, c := range []struct {
		name    string
		got     float64
		want    float64
		decides string
	}{
		{"tolTotalsFloor", tolTotalsFloor, 1.00, "the headline in-scope HARD rate (25 in 2,091, `reached`)"},
		{"tolTotalsRel", tolTotalsRel, 5e-4, "the same, above the $2,000 crossover"},
		{"tolTackFloor", tolTackFloor, 0.05, "the balloon tack-on class"},
		{"tolTackLoanRel", tolTackLoanRel, 1e-5, "the balloon tack-on class"},
		{"tolTackAmtRel", tolTackAmtRel, 5e-4, "the balloon tack-on class"},
		{"tolSolveAmtFloor", tolSolveAmtFloor, 0.01, "Signal 5, solved amount"},
		{"tolSolveAmtRel", tolSolveAmtRel, 2e-6, "Signal 5, solved amount"},
		{"tolSolveRate", tolSolveRate, 5e-6, "Signal 5, solved rate"},
		{"tolPayFloor", tolPayFloor, 0.011, "the regular-payment class and the APR modal discriminator"},
		{"tolPayRel", tolPayRel, 2e-6, "the regular-payment class and the APR modal discriminator"},
		{"tolAdjAmtFloor", tolAdjAmtFloor, 0.011, "Signal 7, 30 findings over 858 rows"},
		{"tolAdjAmtRel", tolAdjAmtRel, 2e-6, "Signal 7, 30 findings over 858 rows"},
		{"tolAdjRate", tolAdjRate, 5e-6, "Signal 7, rate column"},
		{"tolAPR", tolAPR, 2e-6, "🚨 Signal 6 — 20 in 1,856 (1 in 93), the whole open APR-R class"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %g, pinned at %g — this tolerance DECIDES %s. Changing it "+
				"restates that number. Do not edit this line to make a run green: "+
				"a wider tolerance always produces fewer findings (item 0e).",
				c.name, c.got, c.want, c.decides)
		}
	}
}

// TestToleranceFormsComposeAsMeasured pins the SHAPES, not just the scalars.
// A constant can be right while the expression using it is wrong — round 47's
// `:376` is the standing example, where the pinned constant algebraically
// cancelled and the test asserted nothing.
//
// Each case below is chosen so that the floor and the relative term give
// DIFFERENT answers, which is the only regime in which the composition is
// observable at all.
func TestToleranceFormsComposeAsMeasured(t *testing.T) {
	// Totals: floor-dominated below the crossover, relative-dominated above it.
	if got := tolTotals(100); got != 1.00 {
		t.Errorf("tolTotals(100) = %g, want the 1.00 floor", got)
	}
	if got := tolTotals(55940); math.Abs(got-27.97) > 1e-9 {
		t.Errorf("tolTotals(55940) = %g, want 27.97 — the documented crossover "+
			"behaviour that makes this the loosest tolerance in the project", got)
	}
	if got := tolTotals(2000); got != 1.00 {
		t.Errorf("tolTotals(2000) = %g — $2,000 is exactly the crossover and the "+
			"floor must still win there", got)
	}
	// POSITIVE CONTROL (R24): the crossover must actually be a crossover, i.e.
	// the two regimes must give different answers. If they did not, the two
	// assertions above would both be satisfied by a constant function.
	if tolTotals(100) == tolTotals(55940) {
		t.Error("tolTotals is constant across the crossover — the floor/relative " +
			"composition this test exists to pin is not observable")
	}

	// Tack: three-way max. Pick values where each term wins in turn.
	if got := tolTack(100, 10); got != 0.05 {
		t.Errorf("tolTack(100,10) = %g, want the 0.05 floor", got)
	}
	if got := tolTack(1e6, 10); math.Abs(got-10) > 1e-9 {
		t.Errorf("tolTack(1e6,10) = %g, want 10 (the loan term wins)", got)
	}
	if got := tolTack(100, 1e6); math.Abs(got-500) > 1e-9 {
		t.Errorf("tolTack(100,1e6) = %g, want 500 (the tack term wins)", got)
	}

	// Additive forms: floor at zero, floor plus relative above it.
	if got := tolSolveAmount(0); got != tolSolveAmtFloor {
		t.Errorf("tolSolveAmount(0) = %g, want the bare floor %g", got, tolSolveAmtFloor)
	}
	if got := tolPayment(1e6); math.Abs(got-(0.011+2.0)) > 1e-9 {
		t.Errorf("tolPayment(1e6) = %g, want 2.011", got)
	}
	if got := tolAdjAmount(1e6); math.Abs(got-(0.011+2.0)) > 1e-9 {
		t.Errorf("tolAdjAmount(1e6) = %g, want 2.011", got)
	}
	// The additive forms must be SIGN-INSENSITIVE: DOS reports negative interest
	// and negative adjustment amounts, and a tolerance that shrank on a negative
	// value would tighten exactly where the port is least tested.
	for _, f := range []struct {
		n string
		g func(float64) float64
	}{{"tolTotals", tolTotals}, {"tolSolveAmount", tolSolveAmount},
		{"tolPayment", tolPayment}, {"tolAdjAmount", tolAdjAmount}} {
		if f.g(-12345) != f.g(12345) {
			t.Errorf("%s is sign-sensitive: f(-12345)=%g f(12345)=%g", f.n, f.g(-12345), f.g(12345))
		}
	}
}

// ---------------------------------------------------------------------------
// THE DRIFT GUARD.
// ---------------------------------------------------------------------------

// TestToleranceSitesUseTheNamedConstants is the half that TestToleranceValuesArePinned
// cannot cover.
//
// Pinning the constants stops someone editing `tolAPR`. It does NOT stop someone
// editing `dos_fuzzer5_test.go:2910` back to a bare `2e-1` and leaving the
// constant untouched — which is EXACTLY the state round 47 found, and it would
// leave every value pin passing while the deciding site ignored them. So this
// guard reads the fuzzer's source and asserts each site still references its
// helper.
//
// 🚨 R59 — WHOLE STATEMENTS, NOT A WINDOW ON ONE SIDE OF AN ANCHOR. Each needle
// below is a COMPLETE comparison or assignment, counted before it is trusted, so
// a rename that makes a needle disappear FAILS rather than silently passing. The
// count assertion is the direction check R38 demands: here a missing needle reads
// as FAILURE, which is the safe direction.
//
// 🚨 R50 — IT ASSERTS ACROSS FILES. This test is in zzr48_tolerances_test.go and
// its subject is dos_fuzzer5_test.go. A guard that reads its own source is
// unconditionally true.
func TestToleranceSitesUseTheNamedConstants(t *testing.T) {
	src := readSiblingSource(t, "dos_fuzzer5_test.go")

	for _, need := range []struct{ frag, decides string }{
		{"intTol := tolTotals(dos.interest)", "totals interest — the headline HARD rate"},
		{"paidTol := tolTotals(dos.paid)", "totals paid — the headline HARD rate"},
		{"tackTol := tolTack(amount, dosTack.amount)", "the balloon tack-on class"},
		{"tol := tolSolveAmount(dos.solvedAmt)", "Signal 5, solved amount"},
		{"math.Abs(goSolved-dos.solvedRate) > tolSolveRate", "Signal 5, solved rate"},
		{"pTol := tolPayment(dos.payment)", "the regular-payment class"},
		{"aTol := tolAdjAmount(dr.amount)", "Signal 7, adjustment amount"},
		{"math.Abs(ga.Rate-dr.rate) > tolAdjRate", "Signal 7, adjustment rate"},
		{"d > tolPayment(gr.RegularPayment)", "the APR modal-payment discriminator"},
		{"if dAPR > tolAPR {", "🚨 Signal 6 — 20 in 1,856, the whole open APR-R class"},
	} {
		if n := strings.Count(src, need.frag); n != 1 {
			t.Errorf("dos_fuzzer5_test.go contains %d occurrences of %q (want exactly 1). "+
				"This site decides %s. If it has been changed back to a bare literal, "+
				"every value pin in this file keeps passing while the deciding "+
				"comparison ignores them — the exact state item 0e was opened for.",
				n, need.frag, need.decides)
		}
	}

	// POSITIVE CONTROL (R24), and the one that would have caught round 47's
	// state: the bare literals must be GONE from the deciding sites. If any of
	// these reappears in a comparison, a site has drifted back.
	for _, gone := range []string{
		"math.Max(1.0, 5e-4*math.Abs(dos.interest))",
		"0.011 + 2e-6*math.Abs(dos.payment)",
		"0.01 + 2e-6*math.Abs(dos.solvedAmt)",
		"if dAPR > 2e-6 {",
	} {
		if strings.Contains(src, gone) {
			t.Errorf("the inline literal %q is back in dos_fuzzer5_test.go — a "+
				"deciding tolerance has been un-named again (item 0e / R56)", gone)
		}
	}
}

func readSiblingSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("could not read %s: %v", name, err)
	}
	if len(b) < 50000 {
		t.Fatalf("%s is only %d bytes — that is not the fuzzer this guard was "+
			"written against, and a guard that scans the wrong file passes for "+
			"the wrong reason (R59)", name, len(b))
	}
	return string(b)
}
