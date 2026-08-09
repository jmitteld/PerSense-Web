package amortization

// ROUND 41 — R41's Q4/Q7 snapshot has an ORDERING DEPENDENCY, and this pins it.
//
// dos_fuzzer5_test.go carries the four-question verdict alongside the
// seven-question one by taking `caseHardQ4 := caseHard` at a single point: after
// the last of the four STANDING questions' `caseHard = true` sites and before the
// first of Signals 5/6/7's. That is exact today and it is invisible tomorrow —
// a new four-question signal added below the snapshot would be silently absent
// from the Q4 column, and the published R41 delta ("the three new signals add
// N HARD cases") would OVERSTATE the instrument's effect, in the direction that
// excuses the port. Rule 12: the harness is a suspect before the engine is.
//
// START_HERE rule 35 (R35): a documented trap is not a guard. Turn it into an
// assertion. This is that assertion — it reads the source, because the property
// is a property of the source's LAYOUT, not of any runtime value.
//
// It deliberately fails LOUDLY on an unrecognised signal name rather than
// assuming a new signal belongs to Q7: a signal nobody classified is exactly the
// case this guard exists for.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The four STANDING questions, as the exit criterion and every pre-39e figure
// define them: totals, tack (the balloon family), horizon, and the solved
// amount/rate/term. These must all be set ABOVE the snapshot.
var r41Q4Signals = map[string]bool{
	"divergent_class":            true,
	"balloon_value_differs":      true,
	"balloon_dos_only":           true,
	"balloon_go_only":            true,
	"solved_amount_differs":      true,
	"solved_rate_differs":        true,
	"solved_term_differs":        true,
	"go_solved_dos_date_horizon": true,
	"dos_solved_go_refused":      true,
	"go_solved_dos_refused":      true,
}

// The three questions 39e ADDED. These must all be set BELOW the snapshot.
var r41Q7Signals = map[string]bool{
	"regular_payment_differs": true, // Signal 5
	"apr_differs":             true, // Signal 6
	"adj_echo_count":          true, // Signal 7 ...
	"adj_echo_date":           true,
	"adj_amount_missing":      true,
	"adj_amount_differs":      true,
	"adj_amount_invented":     true,
	"adj_rate_missing":        true,
	"adj_rate_differs":        true,
	"adj_rate_invented":       true,
}

func TestQ4SnapshotPrecedesEveryNewSignal(t *testing.T) {
	src, err := os.ReadFile("dos_fuzzer5_test.go")
	if err != nil {
		t.Fatalf("cannot read the fuzzer source, so this guard proves nothing: %v", err)
	}
	lines := strings.Split(string(src), "\n")

	snapshot := -1
	for i, l := range lines {
		if strings.Contains(l, "caseHardQ4 := caseHard") {
			if snapshot >= 0 {
				t.Fatalf("more than one `caseHardQ4 := caseHard` snapshot (lines %d and %d) — "+
					"the Q4 column is then whichever ran last, which is not a definition",
					snapshot+1, i+1)
			}
			snapshot = i
		}
	}
	if snapshot < 0 {
		t.Fatalf("the `caseHardQ4 := caseHard` snapshot is GONE. Either R41's " +
			"question-set split was removed (say so deliberately and update " +
			"START_HERE §2) or it was renamed and this guard silently stopped " +
			"guarding.")
	}

	// Walk every `caseHard = true` site and classify it by the SIG=HARD name in
	// the t.Errorf immediately following it.
	sigRe := regexp.MustCompile(`SIG=HARD:([a-z_0-9]+)`)
	var q4Below, q7Above, unknown []string
	seen := map[string]bool{}
	sites := 0

	for i, l := range lines {
		if !strings.Contains(l, "caseHard = true") {
			continue
		}
		sites++
		name := ""
		for j := i + 1; j < i+8 && j < len(lines); j++ {
			if m := sigRe.FindStringSubmatch(lines[j]); m != nil {
				name = m[1]
				break
			}
		}
		if name == "" {
			// The totals class site emits its signature elsewhere; it is above the
			// snapshot and that is all this guard needs from it.
			if i > snapshot {
				unknown = append(unknown, "line "+strconv.Itoa(i+1)+" (no SIG=HARD nearby)")
			}
			continue
		}
		seen[name] = true
		switch {
		case r41Q4Signals[name]:
			if i > snapshot {
				q4Below = append(q4Below, name+" @"+strconv.Itoa(i+1))
			}
		case r41Q7Signals[name]:
			if i < snapshot {
				q7Above = append(q7Above, name+" @"+strconv.Itoa(i+1))
			}
		default:
			unknown = append(unknown, name+" @"+strconv.Itoa(i+1))
		}
	}

	if sites < 10 {
		t.Errorf("only %d `caseHard = true` sites found — the scan is not seeing the "+
			"file it thinks it is; this guard would pass vacuously", sites)
	}
	for _, n := range q4Below {
		t.Errorf("FOUR-QUESTION signal %s is set BELOW the caseHardQ4 snapshot "+
			"(line %d). It will be missing from the Q4 column, so the published R41 "+
			"delta will OVERSTATE how much of the rate movement is the instrument — "+
			"in the direction that excuses the port. Move the site above the "+
			"snapshot, or set caseHardQ4 explicitly there.", n, snapshot+1)
	}
	for _, n := range q7Above {
		t.Errorf("SEVEN-QUESTION-ONLY signal %s is set ABOVE the caseHardQ4 snapshot "+
			"(line %d). It will be counted in the Q4 column, which makes the "+
			"four-question rate un-comparable to every figure published before 39e — "+
			"the exact thing R41 exists to prevent.", n, snapshot+1)
	}
	for _, n := range unknown {
		t.Errorf("UNCLASSIFIED HARD signal %s. A new signal must be declared in "+
			"r41Q4Signals or r41Q7Signals in this file, because the question set is "+
			"part of every rate this fuzzer reports (rule 9). Failing loudly rather "+
			"than assuming it is a new question.", n)
	}
}
