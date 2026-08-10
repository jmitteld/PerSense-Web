package amortization

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// zzr48_scopekey_test.go — ROUND 48. THE SCOPE KEY IS NOW `reached`, AND IT IS
// A NAMED, PINNED, PRINTED SELECTION RATHER THAN A HARD-WIRED TUPLE INDEX.
//
// WHY THIS FILE EXISTS. Nate ratified decision 3a.11 on 2026-08-09: the standing
// in-scope key becomes `reached` — max(last schedule row, balloons) — because the
// client boundary of 2026-08-03 is about the dates a schedule ACTUALLY REACHES,
// and `horizon`'s third term (the loan's NOMINAL resolved LastDate) is a date a
// prepayment-retired schedule never gets to. §72's own witness: a four-year loan
// that retires in 2030 carrying a nominal LastDate of 2101, excluded from the
// in-scope population by a date 71 years after the last row either engine prints.
//
// The decision then sat UNEXECUTED FOR FIVE ROUNDS (43-47), every round naming it
// as the top item, because executing it restates every published in-scope figure
// at once and that was judged to be owed its own round. This is that round.
//
// 🚨 WHAT THIS CHANGE IS NOT. It is NOT a regression and NOT an improvement. It
// is THE SAME TREE MEASURED CORRECTLY (R14/R36). The rate moves UP because the
// key admits cases that were previously excluded on a date no row holds. Anyone
// reading a before/after pair here without that sentence is reading a population
// change as an engine change, which is precisely what R36 exists to prevent.
//
// 🚨 R57 — A SEED IS NOT A POPULATION, AND NEITHER IS A SEED PLUS AN N. The key
// selects which compared cases land in the in-scope denominator, so it is a knob
// that determines the population. It is therefore PRINTED IN THE RUN'S OWN
// SUMMARY (dos_fuzzer5_test.go's era-split line), not merely defaulted silently.
//
// WHY THE SELECTOR LIVES IN A _test.go FILE. `HorizonKeys` (horizonkeys.go) is
// production code reached by cmd/goamort. The scope key is a HARNESS concept —
// a stratification label, not an engine behaviour — and standing rule 10 is that
// internal-consistency machinery never drives engine behaviour. HorizonKeys stays
// pure and keeps returning all three keys; only the harness chooses among them.

// scopeKeyDefault is the STANDING KEY, and changing this line restates every
// published in-scope rate and COUNT in the project. It is deliberately a named
// constant rather than a literal inside a switch: item 0e's whole lesson is that
// a bare literal at a hard-deciding site is a number nothing pins.
const scopeKeyDefault = "reached"

// fz5ScopeKeyName returns the key this run is scored under.
//
// 🚨 AN UNKNOWN VALUE IS A HARD FAILURE, NOT A SILENT FALLBACK. A typo'd
// PERSENSE_SCOPE_KEY that quietly scored under the default would produce a run
// labelled with one population and measured under another — the exact failure
// R57 was written for.
func fz5ScopeKeyName() string {
	k := os.Getenv("PERSENSE_SCOPE_KEY")
	if k == "" {
		return scopeKeyDefault
	}
	switch k {
	case "reached", "horizon", "lastdate":
		return k
	}
	panic(fmt.Sprintf("PERSENSE_SCOPE_KEY=%q is not one of reached|horizon|lastdate — "+
		"refusing to score a run whose population label does not match its "+
		"population (R57)", k))
}

// fz5ScopeYear resolves the selected key for one case. The three keys come from
// the single shared implementation in horizonkeys.go; this function only chooses.
func fz5ScopeYear(gr AmortResult) int {
	horizon, reached, lastdate := HorizonKeys(gr)
	switch fz5ScopeKeyName() {
	case "reached":
		return reached
	case "horizon":
		return horizon
	case "lastdate":
		return lastdate
	}
	panic("unreachable: fz5ScopeKeyName validated the key")
}

// TestScopeKeyDefaultIsReached pins the DECISION itself.
//
// This is the assertion that would have failed for five rounds. If someone
// reverts the standing key without also restating the published figures, this
// test names the decision, its date, and the fact that the revert is a
// measurement change rather than a fix.
func TestScopeKeyDefaultIsReached(t *testing.T) {
	// 🚨 ROUND-48 AUDIT F6. The first version of this test called a bare
	// os.Unsetenv, which persists for the REST OF THE TEST BINARY. The auditor
	// demonstrated the consequence under -shuffle: with the order
	// (TestScopeKeyDefaultIsReached, TestDOSFuzzer5AllAdvancedOptions) an
	// operator who ran PERSENSE_SCOPE_KEY=horizon got a run that scored and
	// printed `reached`. Today that is masked only by a FILENAME ACCIDENT —
	// dos_fuzzer5_test.go sorts before zzr48_*. A test that silently discards
	// the operator's declared population is the exact hazard this file's own
	// panic message exists to prevent (R57).
	scopeKeyEnvCleared(t)
	if got := fz5ScopeKeyName(); got != "reached" {
		t.Errorf("standing scope key is %q, want \"reached\" — Nate ratified 3a.11 "+
			"on 2026-08-09 and every published in-scope rate and COUNT is keyed on "+
			"it. Changing this is a MEASUREMENT change (R14/R36), not a fix, and it "+
			"restates the whole surface table.", got)
	}
	if scopeKeyDefault != "reached" {
		t.Errorf("scopeKeyDefault = %q, want \"reached\"", scopeKeyDefault)
	}
}

// TestFz5MaxYearFollowsTheSelectedScopeKey is the pin, in BOTH directions, on
// the fixture where the three keys differ MOST.
//
// 🚨 R24 / R20 — THE POSITIVE CONTROL IS INSIDE THE TEST. A fixture on which all
// three keys coincide would let this pass no matter which key fz5MaxYear
// returned. The loop therefore asserts, before scoring anything, that the
// fixture actually SPLITS all three keys — so a pin that cannot fire is itself a
// failure rather than a silent pass.
func TestFz5MaxYearFollowsTheSelectedScopeKey(t *testing.T) {
	// horizon 2101 (from LastDate) != reached 2030 (the walk) != lastdate 2101.
	// Use a balloon so `reached` is independently pinned above the schedule row.
	split := hzFixture(2030, []int{2044}, 2101, true)
	h, r, l := HorizonKeys(split)
	if !(h != r && r != l) {
		t.Fatalf("POSITIVE CONTROL FAILED: fixture does not split the keys "+
			"(horizon=%d reached=%d lastdate=%d). A pin on a fixture where the keys "+
			"coincide asserts nothing about which key is selected (R24).", h, r, l)
	}

	for _, c := range []struct {
		key  string
		want int
	}{
		{"reached", r},
		{"horizon", h},
		{"lastdate", l},
	} {
		t.Setenv("PERSENSE_SCOPE_KEY", c.key)
		if got := fz5ScopeYear(split); got != c.want {
			t.Errorf("PERSENSE_SCOPE_KEY=%s: fz5ScopeYear = %d, want %d", c.key, got, c.want)
		}
		if got := fz5MaxYear(split); got != c.want {
			t.Errorf("PERSENSE_SCOPE_KEY=%s: fz5MaxYear = %d, want %d — the fuzzer's "+
				"in-scope predicate and the selected scope key have come apart, so "+
				"the era split no longer means what its label says (R28)", c.key, got, c.want)
		}
	}

	// And the default, cleared, resolves to `reached` THROUGH fz5MaxYear — the
	// function the two scoring sites actually call (dos_fuzzer5_test.go:2221 and
	// :2385), not merely through the selector.
	os.Unsetenv("PERSENSE_SCOPE_KEY")
	if got := fz5MaxYear(split); got != r {
		t.Errorf("with no PERSENSE_SCOPE_KEY set, fz5MaxYear = %d, want reached = %d", got, r)
	}
}

// TestScopeKeyRejectsAnUnknownValue is the other direction: the harness must
// REFUSE a population label it does not recognise rather than silently scoring
// under the default. A run mislabelled this way is unfalsifiable after the fact.
func TestScopeKeyRejectsAnUnknownValue(t *testing.T) {
	t.Setenv("PERSENSE_SCOPE_KEY", "horzion") // a plausible typo, not a wild value
	defer func() {
		if recover() == nil {
			t.Error("PERSENSE_SCOPE_KEY=\"horzion\" was ACCEPTED — a typo'd key that " +
				"silently scores under the default produces a run whose label and " +
				"population disagree, which is R57's exact failure mode")
		}
	}()
	_ = fz5ScopeKeyName()
}

// scopeKeyEnvCleared clears PERSENSE_SCOPE_KEY for the duration of ONE test and
// restores whatever the operator set, via t.Cleanup. Audit F6.
func scopeKeyEnvCleared(t *testing.T) {
	t.Helper()
	prev, had := os.LookupEnv("PERSENSE_SCOPE_KEY")
	if err := os.Unsetenv("PERSENSE_SCOPE_KEY"); err != nil {
		t.Fatalf("could not clear PERSENSE_SCOPE_KEY: %v", err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv("PERSENSE_SCOPE_KEY", prev)
		} else {
			os.Unsetenv("PERSENSE_SCOPE_KEY")
		}
	})
}

// TestScopeKeySelectionIsInvariantToScheduleShape closes ROUND-48 AUDIT F1.
//
// 🚨 WHAT THE AUDITOR BROKE. Every fixture pinning the scope key had exactly TWO
// schedule rows — a property no real case has. So this mutant passed EVERY guard
// the round added while printing one label and measuring the other:
//
//	func fz5MaxYear(gr AmortResult) int {
//	    if len(gr.Schedule) > 2 {           // i.e. every real schedule
//	        h, _, _ := HorizonKeys(gr); return h
//	    }
//	    return fz5ScopeYear(gr)             // i.e. only the hand fixtures
//	}
//
// It reproduced the `horizon` arm's numbers byte-for-byte (in-scope 219,
// out-of-scope 11 on seed 50100) under a banner reading SCOPE KEY=reached. The
// label and the measurement were never linked by an assertion — which is R57's
// failure mode inside the fix for R57.
//
// THE FIX IS TO ASSERT THE INVARIANCE A DISCRIMINATING MUTANT MUST BREAK. A
// mutant of that shape has to key on something about the case: schedule length,
// balloon count, or magnitude. So the selection is asserted to be UNCHANGED
// across all three, at sizes real cases actually reach.
func TestScopeKeySelectionIsInvariantToScheduleShape(t *testing.T) {
	scopeKeyEnvCleared(t)

	// Schedules from a hand fixture up to a 40-year monthly grid, each carrying
	// the horizon != reached split (an early-retiring walk under a far-future
	// nominal LastDate).
	for _, rows := range []int{2, 3, 12, 97, 265, 480} {
		gr := hzFixtureRows(rows, 2030, nil, 2101, true)
		h, r, _ := HorizonKeys(gr)
		if h == r {
			t.Fatalf("rows=%d: fixture does not split horizon(%d) from reached(%d) — "+
				"a mutant keyed on schedule length would be invisible here (R49)", rows, h, r)
		}
		if got := fz5MaxYear(gr); got != r {
			t.Errorf("rows=%d: fz5MaxYear = %d but the standing key `reached` = %d "+
				"(horizon %d). THE PRINTED LABEL AND THE MEASURED POPULATION HAVE "+
				"COME APART — this is audit F1, and it is invisible to every "+
				"fixture-based pin because real schedules are longer than fixtures.",
				rows, got, r, h)
		}
	}

	// Balloon count and magnitude: the other two things a discriminating mutant
	// can cheaply key on.
	for _, balloons := range [][]int{nil, {2044}, {2044, 2050}, {2044, 2050, 2060}} {
		gr := hzFixtureRows(97, 2030, balloons, 2101, true)
		_, r, _ := HorizonKeys(gr)
		if got := fz5MaxYear(gr); got != r {
			t.Errorf("balloons=%v: fz5MaxYear = %d, `reached` = %d (audit F1)", balloons, got, r)
		}
	}
}

// hzFixtureRows is hzFixture with a caller-chosen number of schedule rows, so a
// pin can be taken at lengths real cases actually reach. The last row carries
// rowYear, which is what `reached` reads.
func hzFixtureRows(rows, rowYear int, balloonYears []int, lastDateYear int, lastDateOK bool) AmortResult {
	gr := hzFixture(rowYear, balloonYears, lastDateYear, lastDateOK)
	if rows <= len(gr.Schedule) || rowYear <= 0 {
		return gr
	}
	full := make([]PaymentRecord, 0, rows)
	for i := 0; i < rows-1; i++ {
		full = append(full, PaymentRecord{
			PayNum: i + 1,
			Date:   types.NewDateRec(rowYear-1, time.March, 15),
		})
	}
	full = append(full, gr.Schedule[len(gr.Schedule)-1])
	gr.Schedule = full
	return gr
}
