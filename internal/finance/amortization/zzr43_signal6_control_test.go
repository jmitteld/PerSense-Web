package amortization

import (
	"os"
	"strings"
	"testing"
)

// ROUND 43, item 0m(i) — THE GUARD FOR SIGNAL 6's CONTROL POPULATION (R49).
//
// THE HISTORY, because it is the whole justification for this file.
//
// Round 39e built Signal 6 (the APR probe) and wrote its negative control: revert
// the modal-payment reconstruction, expect Signal 6 to go red. It did not. 39e
// recorded the result honestly — "the fuzzer-level negative control for the APR
// probe was INERT at seed 50100" — and the project read that as a statement about
// the PROBE's sensitivity. START_HERE then carried, for three consecutive rounds,
// the instruction "build a probe tree that re-introduces the modal-payment APR
// defect and assert Signal 6 goes RED. ~20 lines."
//
// It was never a 20-line job, and it was never about the mutant.
// ROUND40_AUDIT §3.1 had already written the correct requirement:
//
//	"Signal 6 is a FINDER, not a GATE, until the control is re-run on a seed
//	 whose population contains a pay-solve ∩ points screen on the AmortizeDOS
//	 arm where modal != payment."
//
// That sentence never reached START_HERE, so rounds 41, 42 and 43 each
// re-specified the experiment that had already failed. Round 43 measured the
// population and the reason is now arithmetic rather than conjecture:
//
//	seed  pts>0  pay-solve  dosport  modal!=RegularPayment
//	50100   205     48         1            0     <- 39e's seed. VACUOUS.
//	50107   184     45         5            1
//	50109   186     43         4            1
//	... 20 seeds, 8,000 generated screens: 37 dosport, 2 discriminating.
//
// The bottleneck is the ENGINE clause: only ~4.7% of pay-solve ∩ points screens
// route to dosport, and almost none of those has a modal that differs. So the
// control's population is roughly 1 in 4,000 GENERATED screens, and seed 50100
// holds none of it. No mutant could have moved that result.
//
// THE CONTROLLED EXPERIMENT (round 43, probe tree at engine.go:756, the modal
// reconstruction re-introduced into applyAPR's payment argument):
//
//	seed   discriminating   pristine   mutant
//	50100        0           4 differ   4 differ   <- INERT, reproduced exactly
//	50107        1           2 differ   3 differ   <- +1  RED
//	50109        1           1 differ   3 differ   <- +2  RED
//
// Signal 6 goes red exactly where the population can express the defect and
// nowhere else. THAT is the control 39e was reaching for, and Signal 6 may now
// be quoted as a gate FOR THE MODAL-PAYMENT REGRESSION specifically — the
// residual 20-in-1,856 remains unattributed (R27) and is a separate claim.
//
// → R49: A CONTROL IS ONLY A CONTROL IF ITS POPULATION CAN EXPRESS THE DEFECT.
//
// WHAT THIS FILE GUARDS. The measurement above is worthless if the funnel that
// produced it is deleted or quietly narrowed, and a future round would then be
// back to re-running the mutant on a vacuous seed. This is a source-layout guard
// over the four funnel counters and the clause set that defines the population.
//
// ⚠️ R38/r42: it matches in CONTEXT, never on a bare identifier, because a guard
// that matches its own declaration survives the mutant that deletes the use.
// Every assertion here was seen to fail against the deletion it names.
func TestR43Signal6ControlPopulationIsInstrumented(t *testing.T) {
	src, err := os.ReadFile("dos_fuzzer5_test.go")
	if err != nil {
		t.Fatalf("cannot read the fuzzer source: %v", err)
	}
	s := string(src)

	// The four clauses of ROUND40_AUDIT §3.1's requirement. Each is asserted at
	// its USE SITE, with enough context that deleting the increment fails even
	// though the identifier survives in its declaration and in the log line.
	for _, c := range []struct{ what, needle string }{
		{"the points clause", "points > 0 {\n\t\t\taprDiscrimPts++"},
		{"the pay-solve clause", "mode == fz5ModePaySolve {\n\t\t\t\taprDiscrimPaySolve++"},
		{"the dosport (AmortizeDOS arm) clause", `gr.EngineUsed == "dosport" {` + "\n\t\t\t\t\taprDiscrimDosport++"},
		{"the modal-differs clause increment", "aprDiscrim++\n"},
		{"the modal reconstruction itself", "if counts[k] > bestN {"},
		{"the worst-delta record", "aprDiscrimWorst = d"},
	} {
		if !strings.Contains(s, c.needle) {
			t.Errorf("Signal 6's control-population funnel has lost %s.\n"+
				"  Expected to find, in dos_fuzzer5_test.go:\n    %q\n"+
				"  This funnel is the ONLY thing standing between a future round and "+
				"a fourth re-run of round 39e's vacuous negative control. R49. "+
				"See ROUND40_AUDIT §3.1 and docs/discrepancies.md §83.",
				c.what, c.needle)
		}
	}

	// The funnel must be PRINTED. A counter nobody reports is a cell nothing
	// reads back (R40), which is the defect family this whole project keeps
	// re-finding.
	if !strings.Contains(s, "Signal 6 CONTROL POPULATION (R49)") {
		t.Error("the Signal 6 control-population line is no longer printed. " +
			"It is printed EVERY run on purpose: a future round must be able to " +
			"pick a discriminating seed from the log instead of rediscovering " +
			"that seed 50100 has none. R40/R49.")
	}

	// The population is measured on the SAMPLE, before and independently of the
	// APR comparison. If it ever moves inside the comparison it becomes a
	// statement about the outcome instead of the sample, and the control
	// silently reverts to circular.
	iFunnel := strings.Index(s, "aprDiscrimPts++")
	iCompare := strings.Index(s, "aprEligible++")
	if iFunnel < 0 || iCompare < 0 || iFunnel > iCompare {
		t.Errorf("the control-population funnel must be computed BEFORE the APR "+
			"comparison (funnel at %d, comparison at %d). Measured after, it "+
			"describes the outcome rather than the sample and stops being a "+
			"control at all.", iFunnel, iCompare)
	}
}

// TestR43Signal6ControlSeedsAreRecorded pins the two seeds round 43 measured as
// carrying a discriminating case, so the next round does not repeat the
// twenty-seed search — and pins the record that seed 50100 carries NONE, which
// is the single fact that stops a fourth re-run of the vacuous experiment.
//
// 🚨 R50 — THIS GUARD'S FIRST FORM WAS VACUOUS, AND THE SHAPE IS NEW.
// It read THIS file and asserted the seeds appeared in it. Mutation testing
// killed it on sight: renaming "50107" throughout the file renames it in the
// needle AND the haystack, so the assertion holds for every possible value.
// A SELF-READING GUARD WHOSE EXPECTED VALUE LIVES IN THE FILE IT READS IS
// UNCONDITIONALLY TRUE. That is round 42's "a guard can match its own
// declaration" one level up. The fix is the same one: ASSERT ACROSS FILES.
// This version reads docs/discrepancies.md §83, which is where the measurement
// actually lives.
//
// ⚠️ IT IS A DOCUMENTATION PIN, NOT A MEASUREMENT. It asserts the seeds are
// written down, not that they still discriminate — the population is a property
// of the GENERATOR (R31), and 2 in 8,000 is thin. If the generator changes,
// re-run the funnel across seeds and update §83; do not delete it.
func TestR43Signal6ControlSeedsAreRecorded(t *testing.T) {
	doc, err := os.ReadFile("../../../docs/discrepancies.md")
	if err != nil {
		t.Fatalf("cannot read docs/discrepancies.md, where §83 records the "+
			"Signal 6 control population: %v", err)
	}
	d := string(doc)
	i := strings.Index(d, "## §83")
	if i < 0 {
		t.Fatal("docs/discrepancies.md no longer contains §83, which is the only " +
			"place the Signal 6 control population, its two discriminating seeds " +
			"and the controlled experiment are recorded. R49.")
	}
	sec := d[i:]
	if j := strings.Index(sec[6:], "\n## §"); j >= 0 {
		sec = sec[:j+6]
	}

	for _, want := range []struct{ what, needle string }{
		{"the first discriminating seed", "50107"},
		{"the second discriminating seed", "50109"},
		{"the vacuous seed 39e used", "50100"},
		{"the verdict on that seed", "VACUOUS"},
		{"the dosport bottleneck", "4.7%"},
		{"the mutant's measured effect", "3 differ"},
	} {
		if !strings.Contains(sec, want.needle) {
			t.Errorf("docs/discrepancies.md §83 has lost %s (%q).\n"+
				"  §83 is the record that Signal 6's negative control was VACUOUS "+
				"rather than inert, and that its population is ~1 in 4,000 "+
				"generated screens. Without it a future round re-runs round 39e's "+
				"experiment for the fourth time. R49/R50.",
				want.what, want.needle)
		}
	}
}
