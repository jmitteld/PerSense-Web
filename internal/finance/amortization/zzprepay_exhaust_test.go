package amortization

// zzprepay_exhaust_test.go — TERMINATION gate for the prepayment cursor
// (round 19, found while fixing discrepancies §59).
//
// THE BUG. Both sites that advance a prepayment series' "next extra due"
// cursor were written as
//
//	if next, err := dateutil.AddPeriod(nextDates[i], pp.PerYr, day, false); err == nil {
//		nextDates[i] = next
//	}
//
// with no else. AddPeriod for peryr 26 and 52 goes through Julian/MDY, so it
// FAILS past DOS's day-70000 ceiling (26 Aug 2091). On failure the cursor stays
// pointing at the extra just paid, so the series is due again at the same date,
// and the off-cycle drain's `for {}` loop pays it, appends a row, fails to
// advance, and repeats. NN and the stop date bound it in most shapes; where they
// do not, it grows the schedule slice until the kernel kills the process.
//
// WHY IT WAS LATENT UNTIL ROUND 19. The walk never got that far. The A2 block's
// own `AddDays(lastPending, 1)` hit the same ceiling first and stopped the walk
// — that is §59. Fixing §59 let the walk into the region where this one bites.
// Same root (a DOS range guard reached at a port-only site); the first bug was
// masking the second.
//
// HOW IT ESCAPED THE GATE, which is the part worth remembering. The paired
// regression reported NEW 0 while this was live, and reported it *because of*
// the bug: an OOM-killed test binary writes NOTHING, so every failure that seed
// would have reported vanishes from the post set — and a vanished failure is
// indistinguishable from a fixed one. That is harness defect #9's shape (an
// unbounded oracle exec silently discarding whole seeds) occurring on the
// PRODUCT side of the same harness. R8 — bound the unbounded thing and make the
// bound a counted terminal bucket — was written for the oracle and needs to
// cover the binary under test too. `run_arm.sh` surfaced it only because a
// killed seed leaves a log with no `ledger:` line, which is worth keeping.

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// sec59PrepayRepro is fuzzer5 seed 44006, case 312 (0-based) at FUZZ_N=400 —
// the case that actually OOM-killed the round-19 build. Found by bisecting the
// seed on N and dumping the generated command (FZ5CASEDUMP=1).
//
// The mechanism is visible in the tokens: `pre=900:71:26:261.24` starts a
// BIWEEKLY series 900 months (75 years) after a 2023 loan date — i.e. in 2098,
// already past the 26 Aug 2091 ceiling — so its very first AddPeriod fails.
// `b984=35472.07` puts a balloon in 2105, holding very_last past the last
// regular payment so the A2 block hands the tail to the drain.
const sec59PrepayRepro = "460887.78 0.1160710000 290 2 b365_360 exact prepaid inadv plusreg usa " +
	"loandmy=12.1.2023 firstdmy=12.7.2023 b312=76586.69 b984=35472.07 " +
	"pre=252:174:2:3180.44 pre=900:71:26:261.24 targ=4342.66 pts=0.012677 " +
	"payhard=24665.37 noterm"

// Measured on the round-18 build (pre-§59) and again on the round-19 build with
// the exhaustion fix. Both produce these to the cent.
//
// THIS IS NOT A DOS GOLDEN. The real DOS engine does not terminate on this
// screen — it is one of the R8b non-terminating shapes — so there is no oracle
// verdict here and per standing policy an absent oracle response is scored
// neither way. What these numbers assert is narrower and still worth asserting:
// **round 19 must not move this case.** §59 changes a displayed balloon cell,
// not the schedule, so the totals are a cross-version invariant. If they move,
// something in the drain changed that was not supposed to.
const (
	sec59PrepayWantInterest = 1033339.19
	sec59PrepayWantPaid     = 1494226.97
)

func TestPrepayCursorTerminatesOnTheOOMRepro(t *testing.T) {
	in, _ := m5Parse(t, sec59PrepayRepro)

	// A wall-clock bound is the honest assertion for a termination property.
	// Asserting a row count instead would pass a build that hangs after emitting
	// only a few rows.
	done := make(chan AmortResult, 1)
	go func() { done <- Amortize(in) }()

	select {
	case res := <-done:
		if res.Err != nil {
			t.Fatalf("Amortize errored: %v\n  repro: amort_oracle %s bdump",
				res.Err, sec59PrepayRepro)
		}
		t.Logf("returned %d rows, interest %.2f, paid %.2f",
			len(res.Schedule), res.TotalInt, res.TotalPaid)

		if math.Abs(res.TotalInt-sec59PrepayWantInterest) > 0.005 ||
			math.Abs(res.TotalPaid-sec59PrepayWantPaid) > 0.005 {
			t.Errorf("totals MOVED: interest %.2f (want %.2f), paid %.2f (want %.2f).\n"+
				"  These are a round-18-vs-round-19 invariant, not a DOS golden — see "+
				"the const block. §59 was supposed to change a displayed balloon cell "+
				"and nothing else.\n  repro: amort_oracle %s bdump",
				res.TotalInt, sec59PrepayWantInterest, res.TotalPaid,
				sec59PrepayWantPaid, sec59PrepayRepro)
		}
		// The engine's own runaway guard stops at 10000 payments. Landing near it
		// means the walk is running away and only that guard is catching it.
		if len(res.Schedule) > 9000 {
			t.Errorf("schedule reached %d rows — the walk is running away and only "+
				"the payNum guard is stopping it, which is the symptom "+
				"prepayExhausted exists to prevent", len(res.Schedule))
		}

	case <-time.After(60 * time.Second):
		t.Fatalf("Amortize DID NOT RETURN in 60s.\n"+
			"  This is the round-19 prepayment-cursor defect: AddPeriod fails past "+
			"DOS's day-70000 ceiling (26 Aug 2091), the cursor is left unadvanced, "+
			"and the off-cycle drain re-pays the same extra without bound.\n"+
			"  See prepayExhausted in engine.go.\n"+
			"  repro: amort_oracle %s bdump", sec59PrepayRepro)
	}
}

// TestPrepayExhaustionIsReachable guards the premise of the test above: that
// AddPeriod really does fail for the day-based frequencies past the ceiling. If
// a future change makes AddPeriod total, the repro stops exercising the
// exhaustion path and would pass vacuously — R7's silent-coverage-loss failure
// mode applied to a regression test rather than to a generator.
//
// This matters here more than usual, because the FIRST version of this file
// used three hand-constructed screens instead of the real repro, and all three
// passed against a build that still had the defect: NN bounded the re-payment
// at a few hundred rows, so they terminated. A regression test that passes in
// both directions is not a regression test, which is why the case above is the
// measured OOM case and not a plausible-looking substitute.
func TestPrepayExhaustionIsReachable(t *testing.T) {
	past := types.NewDateRec(2098, time.September, 1) // the repro's series start
	for _, perYr := range []int{26, 52} {
		if _, err := addPeriodForTest(past, perYr); err == nil {
			t.Errorf("AddPeriod(%v, peryr=%d) now SUCCEEDS past the day-70000 "+
				"ceiling. The OOM repro no longer exercises the exhaustion path — "+
				"re-target it or delete it, but do not leave it passing vacuously.",
				past.Time.Format("2006-01-02"), perYr)
		}
	}
	// The month-based frequencies do NOT round-trip through MDY, so they must
	// keep working — otherwise the month path has acquired a ceiling it did not
	// have, and that is a separate defect.
	for _, perYr := range []int{1, 2, 4, 12} {
		if _, err := addPeriodForTest(past, perYr); err != nil {
			t.Errorf("AddPeriod(%v, peryr=%d) fails: %v — month-based frequencies "+
				"use month arithmetic and should not meet the Julian ceiling",
				past.Time.Format("2006-01-02"), perYr, err)
		}
	}
}

// addPeriodForTest names the same call the engine makes, with the origin day
// the engine would pass.
func addPeriodForTest(d types.DateRec, perYr int) (types.DateRec, error) {
	return dateutil.AddPeriod(d, perYr, d.Time.Day(), false)
}
