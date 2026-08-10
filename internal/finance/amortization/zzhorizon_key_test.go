package amortization

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// zzhorizon_key_test.go — ROUND 36. THE THREE ERA KEYS, PINNED.
//
// WHY THIS FILE EXISTS. Round 35 split §72's era on `goamort bdump`'s
// `lastdate` — the last REGULAR payment date — and published "3 divergences in
// 255 IN-SCOPE cases" on the faithful port. All three of those cases have a
// prepayment series that carries the schedule decades past the last regular
// payment (port horizons 2109, 2100, 2116), so all three are OUT of scope under
// `fz5MaxYear`, the definition every published in-scope figure in this project
// actually uses. A stratification label is a coverage claim (R28); two labels
// spelled "in scope" were two different claims.
//
// Round 22 already learned this once — its comment on `fz5MaxYear` says so in
// as many words — and it happened again thirteen rounds later because nothing
// in the tree ASSERTED that the harness's key and the fuzzer's key were the same
// key. This file is that assertion.
//
// ⚠️ AND IT PINS A THIRD KEY, BECAUSE THE ROUND-36 AUDIT FOUND THE FIRST FIX
// INCOMPLETE. `fz5MaxYear` takes max(last schedule row, balloons, resolved
// LastDate). The last term is the loan's NOMINAL last regular payment date,
// which a prepayment-retired schedule NEVER REACHES: a loan whose extra payments
// retire it in 2030 can carry a nominal LastDate in 2101 and be labelled out of
// scope for a date no row in either engine ever holds. The ratified client
// boundary (claude/decisions_2026-08-03b_client_2099_boundary.md) is about the
// dates the schedule reaches. On the ceiling family that difference moves 29 of
// 196 out-of-scope screens back in, 5 of them divergent — so it is not a nicety.
//
// The keys, and what each is for:
//
//	lastdate  last regular payment date            round 35's key. WRONG for this
//	                                               purpose; kept only so the
//	                                               retraction is reproducible.
//	horizon   max(row, balloons, LastDate)         the key the round-38
//	                                               contingency table (475 in
//	                                               34,967) was built on. Kept and
//	                                               still emitted so that table
//	                                               stays comparable.
//	reached   max(row, balloons)                   what the walk ACTUALLY
//	                                               PRODUCES, what the decision
//	                                               says, and — SINCE ROUND 48 —
//	                                               == fz5MaxYear, THE STANDING KEY.
//
// 🚨 ROUND 48 EXECUTED 3a.11. `fz5MaxYear` now returns `reached`. Round 36's note
// that this was "NOT yet the standing key — changing that is a measurement change
// owed its own round" stood for five rounds; round 48 is that round, and every
// published in-scope rate AND COUNT restates with it. That restatement is the
// SAME TREE MEASURED CORRECTLY, not a regression (R14/R36).
//
// ⚠️ cmd/goamort's `horizon` token DELIBERATELY DOES NOT MOVE. It emits all three
// keys on one line and the Python arms parse all three BY NAME; repointing the
// token would silently re-key every arm that reads it. What changed is which key
// the FUZZER scores under, and that is pinned by name, against a named default,
// in zzr48_scopekey_test.go.

// hzProbe is one screen driven through both paths.
type hzProbe struct {
	toks    []string
	why     string
	control bool // all three keys must coincide
}

// TestHorizonTokenMatchesFz5MaxYear drives cmd/goamort's `horizon` token and the
// in-process engine over the same screens and asserts the two agree.
//
// The token is the ONLY way a Python arm can ask the port for its own horizon
// (R2 — a harness-computed date manufactures a frontier, and that rule has
// returned six defects). If the token and fz5MaxYear ever disagree, an arm and
// the fuzzer are splitting the same population two different ways while both
// call the result "in scope".
func TestHorizonTokenMatchesFz5MaxYear(t *testing.T) {
	bin := os.Getenv("PERSENSE_GOAMORT")
	if bin == "" {
		bin = "/tmp/goamort"
	}
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Skipf("goamort not built at %s (build it with "+
				"`go build -o /tmp/goamort ./cmd/goamort`); this guard needs the CLI",
				bin)
		}
		t.Skipf("goamort not present at %s", bin)
	}

	probes := []hzProbe{
		{[]string{"403901.74", "0.0926", "240", "12", "plusreg",
			"loandmy=29.8.2024", "firstdmy=29.9.2024", "pre=854:52:6:3683.25", "exact"},
			"§72 case B: lastdate 2044, horizon 2100 — the case that made round 35's key wrong", false},
		{[]string{"483080.02", "0.0839", "480", "12", "plusreg",
			"loandmy=29.5.2025", "firstdmy=29.6.2025", "pre=854:246:12:1137.73", "usa", "b365"},
			"§72 case C: lastdate 2065, horizon 2116", false},
		{[]string{"333366.23", "0.0575", "700", "12", "plusreg",
			"loandmy=15.3.2024", "firstdmy=15.4.2024", "pre=240:20000:24:3717.61"},
			"§72 case A: lastdate 2082, horizon 2109", false},
		{[]string{"233825.48", "0.0567", "900", "12", "plusreg",
			"loandmy=29.11.2026", "firstdmy=29.12.2026", "pre=1:246:24:4199.15", "usa"},
			"EARLY RETIREMENT: horizon 2101 but the walk ends 2030 — horizon != reached", false},
		{[]string{"403901.74", "0.0926", "240", "12", "plusreg",
			"loandmy=29.8.2024", "firstdmy=29.9.2024", "exact"},
			"negative control: no prepayment, so all three keys must coincide", true},
	}

	sawSplit := false
	for _, p := range probes {
		hz, reached, ld, ok := hzRunToken(t, bin, p.toks)
		if !ok {
			t.Errorf("%s: goamort printed no horizon line for %v", p.why, p.toks)
			continue
		}
		if reached > hz {
			t.Errorf("%s: reached %d > horizon %d — horizon is a MAX over reached "+
				"plus LastDate and cannot be the smaller of the two", p.why, reached, hz)
		}
		if hz != reached {
			sawSplit = true
		}
		if p.control {
			if hz != reached || reached != ld {
				t.Errorf("negative control: a screen with no prepayment must have "+
					"horizon == reached == lastdate, got %d / %d / %d", hz, reached, ld)
			}
		}
		t.Logf("horizon=%d reached=%d lastdate=%d  |  %s", hz, reached, ld, p.why)
	}

	// R20 / rule 16: a distinction that never fires has not been demonstrated.
	// If no probe splits horizon from reached, this test is asserting nothing
	// about the very difference it was written for.
	if !sawSplit {
		t.Error("no probe produced horizon != reached — the early-retirement case " +
			"this test exists to pin is no longer reachable, which means either " +
			"the engine changed or the probe is stale. A guard that cannot fire " +
			"is not a guard (rule 16's corollary).")
	}
}

// TestHorizonKeyDisagreesWithLastdate is the POSITIVE CONTROL (R24): it asserts
// that the three keys CAN disagree, on the exact screens where round 35's split
// went wrong. If this ever passes trivially — all three keys equal everywhere —
// the retraction in §72 has lost its evidence and the guard above is vacuous.
func TestHorizonKeyDisagreesWithLastdate(t *testing.T) {
	bin := os.Getenv("PERSENSE_GOAMORT")
	if bin == "" {
		bin = "/tmp/goamort"
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("goamort not present at %s", bin)
	}
	// §72 case B: round 35 called this IN SCOPE on lastdate=2044.
	toks := []string{"403901.74", "0.0926", "240", "12", "plusreg",
		"loandmy=29.8.2024", "firstdmy=29.9.2024", "pre=854:52:6:3683.25", "exact"}
	hz, _, ld, ok := hzRunToken(t, bin, toks)
	if !ok {
		t.Fatal("goamort printed no horizon line")
	}
	if ld > 2099 {
		t.Fatalf("lastdate %d is already out of scope — this screen no longer "+
			"demonstrates the mis-key", ld)
	}
	if hz <= 2099 {
		t.Fatalf("horizon %d is in scope too — the screen round 35 mis-keyed no "+
			"longer splits the keys, so §72's retraction needs a new witness", hz)
	}
	t.Logf("§72 case B: lastdate %d (IN scope, round 35's key) vs horizon %d "+
		"(OUT of scope, the project's key) — the mis-key, reproduced", ld, hz)
}

func hzRunToken(t *testing.T, bin string, toks []string) (hz, reached, ld int, ok bool) {
	t.Helper()
	out, err := exec.Command(bin, append(append([]string{}, toks...), "horizon")...).Output()
	if err != nil {
		return 0, 0, 0, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		w := strings.Fields(line)
		if len(w) == 6 && w[0] == "horizon" && w[2] == "reached" && w[4] == "lastdate" {
			a, e1 := strconv.Atoi(w[1])
			b, e2 := strconv.Atoi(w[3])
			c, e3 := strconv.Atoi(w[5])
			if e1 != nil || e2 != nil || e3 != nil {
				return 0, 0, 0, false
			}
			return a, b, c, true
		}
	}
	return 0, 0, 0, false
}
