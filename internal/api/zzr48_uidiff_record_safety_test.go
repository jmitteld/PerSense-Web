package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// zzr48_uidiff_record_safety_test.go — ROUND 48. ITEMS 0x AND 0y, PINNED.
//
// WHAT WENT WRONG (round 47 measured both):
//
//	0x / R57 — `uidiff_results.json` recorded `seed` and nothing else. But
//	gen.generate draws EVERY tier from ONE shared RNG stream, so `singlePer`
//	advances that stream by OPTION_KEYS(16) x singlePer draws before the stacked
//	tier begins: at singlePer=2 (the committed default) the run is 242 cases and
//	`stacked-72` denotes a DIFFERENT LOAN than it does at singlePer=3 (the
//	committed BASELINE's value, 258 cases). Round 46's baseline could not be
//	reproduced from its own record, and rounds 46 and 47 each burned time
//	discovering that again.
//
//	0y / R58 — run.js wrote uidiff_results.json from a run in which 189 of 200
//	stacked cases ERRORED, and reported success. The committed baseline survived
//	only because /tmp copies happened to exist.
//
// 🚨 R59 — THIS GUARD BRACE-MATCHES ITS SUBJECT, IT DOES NOT PEEK THROUGH A
// WINDOW. Round 47's own guard scanned a fixed character window on one side of
// an anchor, examined 35 of the 900 characters inside the block it named, and
// PASSED on the mutant at the faithful position. Every block this file reasons
// about is extracted by balanced-delimiter matching and asserted to be of a
// plausible size before anything is concluded from it.
//
// 🚨 R50 — IT ASSERTS ACROSS FILES. A Go test reading its own source is
// unconditionally true. The subject here is testplan/harness/uidiff/run.js.

func uidiffRunJS(t *testing.T) string {
	t.Helper()
	// Walk up to the repo root rather than hard-coding a depth, so the guard
	// does not silently start skipping if the package moves.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		p := filepath.Join(dir, "testplan", "harness", "uidiff", "run.js")
		if b, err := os.ReadFile(p); err == nil {
			if len(b) < 4000 {
				t.Fatalf("run.js is only %d bytes — that is not the harness this "+
					"guard was written against, and a guard that scans the wrong "+
					"file passes for the wrong reason", len(b))
			}
			return string(b)
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate testplan/harness/uidiff/run.js from the package dir")
	return ""
}

// matchBalanced returns the source from the first occurrence of `anchor` through
// the delimiter that closes the FIRST `open` after it. It is the whole block or
// nothing — never a fixed window (R59).
func matchBalanced(t *testing.T, src, anchor string, open, close rune) string {
	t.Helper()
	i := strings.Index(src, anchor)
	if i < 0 {
		t.Fatalf("anchor %q not found in run.js — the guard's subject has been "+
			"renamed or removed, which reads as PASS in a naive scan and must not", anchor)
	}
	j := strings.IndexRune(src[i:], open)
	if j < 0 {
		t.Fatalf("no %q after anchor %q", string(open), anchor)
	}
	start := i + j
	depth := 0
	for k, r := range src[start:] {
		switch r {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return src[start : start+k+1]
			}
		}
	}
	t.Fatalf("unbalanced %q after anchor %q", string(open), anchor)
	return ""
}

// TestUidiffRecordsEveryPopulationKnob pins item 0x.
//
// The claim is NOT "the word singlePer appears somewhere in run.js" — it appears
// in the flag parser regardless. The claim is that every knob handed to
// gen.generate is ALSO carried in the object written to the results file. So the
// guard extracts BOTH object literals by balanced braces and compares their key
// sets. If a future knob is added to the generator call and not to the record,
// this fails — which is the actual failure mode R57 describes.
func TestUidiffRecordsEveryPopulationKnob(t *testing.T) {
	src := uidiffRunJS(t)

	genArgs := matchBalanced(t, src, "gen.generate(", '{', '}')
	popKnobs := matchBalanced(t, src, "const popKnobs =", '{', '}')
	if len(genArgs) < 10 || len(popKnobs) < 20 {
		t.Fatalf("implausibly small match: genArgs=%q popKnobs=%q — R59, a guard "+
			"must assert its match is big enough to be the thing it named",
			genArgs, popKnobs)
	}

	ident := regexp.MustCompile(`(?m)\b([A-Za-z_][A-Za-z0-9_]*)\s*(?::|,|\})`)
	keys := func(block string) map[string]bool {
		out := map[string]bool{}
		for _, m := range ident.FindAllStringSubmatch(block, -1) {
			out[m[1]] = true
		}
		return out
	}
	gk, pk := keys(genArgs), keys(popKnobs)
	if len(gk) == 0 {
		t.Fatal("parsed zero keys out of the gen.generate argument object")
	}
	for k := range gk {
		if !pk[k] {
			t.Errorf("generator knob %q is passed to gen.generate but is NOT recorded "+
				"in popKnobs — a results file that omits it cannot identify its own "+
				"population (R57). Recording `seed` alone is exactly this bug.", k)
		}
	}

	// And the record must actually reach the file: `population: popKnobs` inside
	// the JSON.stringify payload, not merely computed and logged.
	payload := matchBalanced(t, src, "JSON.stringify(", '{', '}')
	if len(payload) < 60 {
		t.Fatalf("implausibly small results payload match (%d bytes)", len(payload))
	}
	if !strings.Contains(payload, "population: popKnobs") {
		t.Error("the results-file payload does not carry `population: popKnobs` — " +
			"printing the knobs to a log line is what round 47 already had, and it " +
			"is not a record (R57 says: in the RESULTS FILE, not just a log line)")
	}
}

// TestUidiffDelegatesTheResultsGateToAnExecutedCheck pins item 0y — but note
// carefully what it does and does NOT claim.
//
// 🚨 THE ROUND-48 AUDIT KILLED THE FIRST VERSION OF THIS TEST, AND IT WAS RIGHT
// TO. That version scanned run.js for the strings `errored.length`, `partial`
// and the two quarantine filenames inside a brace-matched ternary. Nine mutants
// were run against it and FOUR SURVIVED: a multi-line `writeFileSync` naming the
// baseline directly (that check was a LINE window — R59's condemned shape,
// inside the file condemning it), `const partial = false`, `const errored = []`
// — WHICH IS ROUND 47'S EXACT BUG — and an inverted ternary. Every needle stayed
// present as text while the gate was dead. A Go test cannot execute run.js, so
// no amount of scanning closes this.
//
// ✅ THE RULE NOW LIVES IN record.js AS A PURE FUNCTION, and selftest.js's trap4
// exercises it on six inputs plus two refusal cases at the start of EVERY uidiff
// run. Five mutants against that gate — including the four above — are killed by
// a check that RUNS. This Go test's only remaining job is to assert run.js still
// DELEGATES, i.e. nobody has reintroduced an inline decision that bypasses the
// executed gate. That is a much narrower claim than the first version made, and
// stating it narrowly is the point.
func TestUidiffDelegatesTheResultsGateToAnExecutedCheck(t *testing.T) {
	src := uidiffRunJS(t)

	for _, need := range []struct{ frag, why string }{
		{"require('./record')", "run.js must import the module holding the rule"},
		{"record.chooseResultsFile({ erroredCount: errored.length, only })",
			"the filename must come from the executed gate, not an inline ternary"},
	} {
		if n := strings.Count(src, need.frag); n != 1 {
			t.Errorf("run.js contains %d occurrences of %q (want exactly 1): %s",
				n, need.frag, need.why)
		}
	}

	// The baseline filename must not appear in run.js at all any more: if it
	// does, some path is naming it directly and bypassing record.js. This is a
	// WHOLE-FILE claim, not a line window — the shape the audit condemned.
	if strings.Contains(src, "'uidiff_results.json'") {
		t.Error("run.js names 'uidiff_results.json' directly — the baseline filename " +
			"belongs to record.js alone, and a direct mention means a path can reach " +
			"it without passing the executed gate (audit F5, mutant U6)")
	}

	// And the executed gate must be wired into the run: trap4 has to be called by
	// runAll, or it is a function nobody exercises.
	self := uidiffSibling(t, "selftest.js")
	runAll := matchBalanced(t, self, "function runAll(", '{', '}')
	if len(runAll) < 60 {
		t.Fatalf("runAll body is only %d bytes — too small to be the selftest "+
			"entry point (R59)", len(runAll))
	}
	if !strings.Contains(runAll, "trap4(log)") {
		t.Errorf("selftest.runAll does not call trap4 — the results-file gate would "+
			"then be a function nothing exercises, which is exactly the state the "+
			"audit found\n--- runAll ---\n%s", runAll)
	}

	// POSITIVE CONTROL (R24): record.js must exist and hold three DISTINCT
	// filenames. If it does not, every assertion above is about a file that is
	// not doing the work.
	rec := uidiffSibling(t, "record.js")
	seen := map[string]bool{}
	for _, name := range []string{"'uidiff_results.json'", "'uidiff_results.ERRORED.json'", "'uidiff_results.PARTIAL.json'"} {
		if !strings.Contains(rec, name) {
			t.Errorf("record.js does not mention %s — the gate is not where this "+
				"test says it is", name)
		}
		seen[name] = true
	}
	if len(seen) != 3 {
		t.Error("the three results-file names are not distinct in record.js")
	}
}

func uidiffSibling(t *testing.T, name string) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		p := filepath.Join(dir, "testplan", "harness", "uidiff", name)
		if b, err := os.ReadFile(p); err == nil {
			if len(b) < 500 {
				t.Fatalf("%s is only %d bytes — not the file this guard names (R59)", name, len(b))
			}
			return string(b)
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate testplan/harness/uidiff/%s", name)
	return ""
}
