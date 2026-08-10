package amortization

import (
	"os"
	"strings"
	"testing"
)

// ROUND 44, item 0m(i)-B — THE GUARD FOR SIGNAL 7's CONTROL POPULATION (R49/R51).
//
// Signal 7 (the adjustment-cell echo probe) was added in the SAME 39e commit as
// Signal 6 and, like Signal 6, shipped without a negative control. Round 43
// closed 0m(i) for Signal 6 and wrote R49. This file is the same work for
// Signal 7 — and it came out differently in a way that produced a NEW rule.
//
// THE MEASUREMENT (round 44, seeds 50100/50107/50109 at PERSENSE_FUZZ_N=400,
// 1,200 generated screens, oracle built -dV_3 -dSCROLLS -dPVLX):
//
//	seed   piecewise rows   DOS displays amt   Go carries   mutant A2 -> adj_amount_missing
//	50100        91                30              30        0 -> 30   (delta 30)
//	50107        79                24              23        1 -> 24   (delta 23)
//	50109        86                28              28        0 -> 28   (delta 28)
//
//	seed   DOS displays rate   Go carries   mutant B -> adj_rate_missing
//	50100        24                24         0 -> 24
//	50107        25                25         0 -> 25
//	50109        21                21         0 -> 21
//
// THE DELTA EQUALS THE FUNNEL'S THIRD NUMBER ON EVERY SEED, ON BOTH ARMS. That
// is a stronger positive control than round 43 got for Signal 6: the population
// count does not merely license the control, it PREDICTS the mutant's finding
// count exactly. Signal 7 is a GATE for the adjustment-cell echo, both arms.
//
// 🚨 R51 — AND HOW THIS ROUND NEARLY GOT THE OPPOSITE ANSWER.
//
// The FIRST mutant (A) re-introduced the historical defect named in
// engine.go:1931's own comment — key the "has a value" test on `a.AmtOK` alone
// instead of `a.AmtOK || a.AmountStatus == types.InOutOutput`, the comment's
// stated reason being that "AmtOK alone is too narrow ... keying on amtok loses
// exactly the row DOS paints." MUTANT A WAS COMPLETELY INERT — byte-identical
// findings on all three seeds — WHILE THIS FUNNEL READ 30, 23 AND 28.
//
// Round 43's R49 would have read that as "the population is fine, so the
// instrument is insensitive." IT IS NEITHER. Mutant A2 (drop the echo
// outright) turned the same population red 30/24/28. The difference is that
// mutant A mutated a DISJUNCT THAT IS DEAD on this generator's population:
// `a.AmtOK` is already true on every one of those rows, so removing the second
// disjunct changes nothing.
//
//	R49 asks: CAN THE POPULATION EXPRESS THE DEFECT?      (here: yes, 30)
//	R51 asks: DOES THE MUTANT REACH THAT POPULATION?      (mutant A: no)
//
// A non-vacuous funnel plus an inert mutant means ESCALATE THE MUTANT, not
// doubt the instrument. It is the exact inverse of the failure round 43 spent
// three rounds on, and a funnel keyed to the SIGNAL's predicate is silent about
// it — the funnel was right about Signal 7 and said nothing about mutant A.
//
// ⚠️ AND A FILED, UNFIXED CONSEQUENCE: engine.go:1931's `|| a.AmountStatus ==
// types.InOutOutput` disjunct is NOT LOAD-BEARING on any of 256 measured
// piecewise adjustment rows, though its comment says it is. Either the
// generator cannot produce the rate-only-adjustment shape the comment
// describes, or the claim is wrong. R31: that is a statement about the
// generator until someone widens it. FILED as §84, NOT FIXED — removing a
// disjunct that is dead on the sample you measured is how a latent defect
// ships.
//
// WHAT THIS FILE GUARDS: the funnel, not the numbers. The measurement is
// worthless if a later round deletes or narrows the counters, and would then be
// back to running a mutant against an unmeasured population.
//
// ⚠️ R38/R50: the numeric record is asserted ACROSS FILES
// (docs/discrepancies.md §84), never against this file's own text, and the
// source-layout needles match in CONTEXT wherever the bare identifier would be
// ambiguous. TWO of them (`adjAlreadyRedAmt++`, `adjAlreadyRedRate++`) are bare
// identifiers, which is safe ONLY because each is unique in the fuzzer — an
// earlier draft of this comment claimed "every assertion matches in context"
// and that was simply false; the round-44 audit caught it. Say what the guard
// does, not what it aspires to. A self-reading guard whose expected value
// lives in the file it reads is unconditionally true — that is R50, and round
// 43's first attempt at exactly this guard died of it.
func TestR44Signal7ControlPopulationIsInstrumented(t *testing.T) {
	src, err := os.ReadFile("dos_fuzzer5_test.go")
	if err != nil {
		t.Fatalf("cannot read the fuzzer source: %v", err)
	}
	s := string(src)

	for _, c := range []struct{ what, needle string }{
		{"the piecewise-engine clause", `adjEng == "piecewise" {` + "\n\t\t\t\t\t\tadjPiecewiseRows++"},
		{"the DOS-displays-amount clause", "dr.amtStatus == 1 {\n\t\t\t\t\t\t\tadjDosDisplaysAmt++"},
		{"the amount discriminating clause", "ga.AmountSolved {\n\t\t\t\t\t\t\t\tadjDiscrimAmt++"},
		{"the amount already-red clause (NF-1's own population)", "adjAlreadyRedAmt++"},
		{"the DOS-displays-rate clause", "dr.rateStatus == 1 {\n\t\t\t\t\t\t\tadjDosDisplaysRate++"},
		{"the rate discriminating clause", "ga.RateSolved {\n\t\t\t\t\t\t\t\tadjDiscrimRate++"},
		// ⚠️ ADDED AFTER THE ROUND-44 AUDIT FOUND IT MISSING. Without this needle,
		// deleting `adjAlreadyRedRate++` still COMPILES — the variable is still
		// passed to the RATE Logf — so the guard stayed green while the rate
		// funnel silently reported 0 already-red rows. A counter that is read but
		// never written is exactly the vacuity this file exists to prevent.
		{"the rate already-red clause", "adjAlreadyRedRate++"},
	} {
		if !strings.Contains(s, c.needle) {
			t.Errorf("Signal 7's control-population funnel has lost %s.\n"+
				"  Expected to find, in dos_fuzzer5_test.go:\n    %q\n"+
				"  Without this funnel Signal 7's negative control is unmeasured, and "+
				"round 44 showed that an INERT MUTANT ON A HEALTHY POPULATION is a "+
				"statement about the mutant, not the instrument (R51). "+
				"See docs/discrepancies.md §84.",
				c.what, c.needle)
		}
	}

	// The funnel must be PRINTED. A counter nobody reports is a cell nothing
	// reads back — R40, the defect family this project keeps re-finding.
	for _, line := range []string{
		"Signal 7 CONTROL POPULATION (R49) AMOUNT",
		"Signal 7 CONTROL POPULATION (R49) RATE",
		"Signal 7 / NF-1 RECONCILIATION (item 0-NF)",
	} {
		if !strings.Contains(s, line) {
			t.Errorf("the %q line is no longer printed. It is printed EVERY run on "+
				"purpose: a later round must be able to read the discriminating "+
				"population and the NF-1 denominator out of the log instead of "+
				"re-deriving them. R40/R49.", line)
		}
	}

	// The funnel must be computed BEFORE the comparison switches. Measured
	// after, it describes the outcome rather than the sample and stops being a
	// control at all — the same ordering requirement round 43 put on Signal 6.
	// ⚠️ Anchor the comparison on the Errorf FORMAT STRING, not on the bare token
	// `adj_amount_missing` — that token also appears in this funnel's own
	// explanatory comment and in the reconciliation log line, and the first of
	// those sits ABOVE the funnel, which made the first form of this assertion
	// fail against correct code. A source-layout guard is only as good as the
	// uniqueness of its needle.
	iFunnel := strings.Index(s, "adjPiecewiseRows++")
	iCompare := strings.Index(s, "SIG=HARD:adj_amount_missing %s")
	if iFunnel < 0 || iCompare < 0 || iFunnel > iCompare {
		t.Errorf("Signal 7's control-population funnel must be computed BEFORE the "+
			"amount/rate comparison switches (funnel at %d, comparison at %d).",
			iFunnel, iCompare)
	}
}

// TestR44Signal7ControlIsRecorded pins the round-44 measurement where it
// actually lives — docs/discrepancies.md §84 — so a later round does not repeat
// the seed search or, worse, repeat mutant A and conclude Signal 7 is blind.
//
// ⚠️ R50: this reads a DIFFERENT FILE on purpose. Round 43's first version of
// the equivalent Signal 6 guard read its own source and asserted the seeds
// appeared in it, which is unconditionally true for every possible value.
//
// ⚠️ IT IS A DOCUMENTATION PIN, NOT A MEASUREMENT. It asserts the control is
// written down, not that the population still discriminates — the population is
// a property of the GENERATOR (R31). If the generator changes, re-run the
// funnel and update §84; do not delete it.
func TestR44Signal7ControlIsRecorded(t *testing.T) {
	doc, err := os.ReadFile("../../../docs/discrepancies.md")
	if err != nil {
		t.Fatalf("cannot read docs/discrepancies.md, where §84 records Signal 7's "+
			"control population and the R51 near-miss: %v", err)
	}
	d := string(doc)
	i := strings.Index(d, "## §84")
	if i < 0 {
		t.Fatal("docs/discrepancies.md no longer contains §84, which is the only " +
			"place Signal 7's discriminating population, the two controlling " +
			"mutants and the INERT mutant A are recorded. R49/R51.")
	}
	sec := d[i:]
	if j := strings.Index(sec[6:], "\n## §"); j >= 0 {
		sec = sec[:j+6]
	}

	// ⚠️ EVERY NEEDLE HERE IS A FULL PHRASE, NOT A TOKEN. Mutation testing killed
	// an earlier form of this list: the needle "MUTANT A" is satisfied by the
	// string "MUTANT A2", so deleting the INERT mutant's record left the guard
	// green because the WORKING mutant's record spelled the needle. A guard
	// whose needle is a prefix of a neighbouring, surviving string is a guard
	// that cannot see the deletion it exists to catch. R38.
	for _, want := range []struct{ what, needle string }{
		{"the amount arm's controlled result", "| **0 -> 30** | 30 | 30 |"},
		{"the second seed's amount delta and its funnel prediction", "| **1 -> 24** | 23 | 23 |"},
		{"the rate arm's controlled result", "| **0 -> 24** | 24 | 4 → **4** |"},
		{"the INERT mutant's record (NOT mutant A2's)", "**MUTANT A produced byte-identical findings on all three seeds — INERT — while"},
		{"the dead disjunct mutant A exposed", "`a.AmtOK` is already true on every one of those rows"},
		{"the rule the near-miss produced", "R51 — A MUTANT THAT IS INERT ON A HEALTHY POPULATION IS A STATEMENT ABOUT"},
		{"the instruction that follows from it", "Escalate the mutant before doubting the instrument"},
		{"the filed-not-fixed disposition of the dead disjunct", "DO NOT\nDELETE THE DISJUNCT"},
	} {
		if !strings.Contains(sec, want.needle) {
			t.Errorf("docs/discrepancies.md §84 has lost %s (%q).\n"+
				"  §84 is the record that Signal 7 IS a gate on both arms, that its "+
				"funnel predicts the mutant's finding count exactly, and that a "+
				"mutant can be inert on a HEALTHY population because it never "+
				"reaches the clause (R51). Without it a later round re-runs mutant A "+
				"and concludes Signal 7 is blind.",
				want.what, want.needle)
		}
	}
}
