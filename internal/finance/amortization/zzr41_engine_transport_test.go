package amortization

// ROUND 41 — the ENGINE IDENTITY is TRANSPORTED, not reconstructed.
//
// WHY THIS FILE EXISTS. Every per-engine number this project has ever published
// was produced by parsing engine.go's GENGINE lines off stderr and pairing them
// with dos_fuzzer5's FZ5ENGBEGIN/FZ5ENGEND bracket. That reconstruction is not
// hypothetically fragile — it has ALREADY BEEN WRONG ONCE. Round 34's first
// draft took the LAST GENGINE line inside the bracket instead of the first and
// moved 390 compared cases from piecewise to dosport: a 23% inflation of the
// faithful port's denominator, in the direction that flatters the port (rule 12).
// The bracket was the fix, and it is still a stderr-parsing heuristic that only
// exists because the fact never left the engine.
//
// R39 says a cell the original computes must be transported, never
// reconstructed. The engine label is not a DOS cell, but the argument is
// identical, and round 41 applies it to the INSTRUMENT rather than the product:
// AmortResult.EngineUsed / .RouteReason are stamped from the routing predicate's
// own value at the single branch in Amortize.
//
// WHAT THIS TEST PINS, and why each part is here rather than assumed:
//
//   1. VALUE — the label is right on both arms, against HARD-CODED expectations.
//      It deliberately does NOT compute its `want` from dosPortCanHandle: a test
//      that derives its expectation from the thing under test is self-referential
//      and this codebase has already shipped one of those
//      (frontend_diff_sweep_test.go:447, item 24).
//   2. SURVIVAL (R42) — the piecewise arm reassigns `result` wholesale several
//      times (`result = generate…Schedule(...)`). That is exactly how round 39
//      lost SolvedPrepay. The stamp is a DEFERRED write to the named return, and
//      this asserts it is still there after a full piecewise walk.
//   3. NON-VACUITY — the two arms must produce DIFFERENT labels in one run. A
//      mutant that returns a constant ("dosport" always, or "" always) passes
//      assertion 1 on one arm alone; it cannot pass this.
//   4. AGREEMENT WITH THE OLD INSTRUMENT (R36) — the transported label is checked
//      against the GENGINE line the reconstruction reads, on the same call. R36:
//      run the old instrument and the new one side by side. If they ever
//      disagree, one of the two per-engine denominators in every published table
//      is wrong, and this says so at build time rather than in a round note.
//
// ⚠️ It does NOT assert that any particular loan SHOULD route one way. That is
// still §3b item 8's open question, exactly as zzsec70_engine_route_test.go says.

import (
	"bufio"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// r41TransportBase is a plain-fancy screen that the router accepts, built
// independently of routeBase() so a future edit to that helper cannot silently
// re-point this file's expectations.
func r41TransportBase() LoanInput {
	ld := types.NewDateRec(2024, time.January, 1)
	fd := types.NewDateRec(2024, time.February, 1)
	return LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.10,
			NStatus: types.InOutInput, NPeriods: 24,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus:   types.InOutInput,
			PayAmt:         4614.49,
			LoanDateStatus: types.InOutInput, LoanDate: ld,
			FirstStatus: types.InOutInput, FirstDate: fd,
			LastOK: true,
		},
		Fancy:    true,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
	}
}

func TestEngineUsedIsTransportedNotReconstructed(t *testing.T) {
	// ---- arm A: the faithful port answers ----
	inA := r41TransportBase()
	resA := Amortize(inA)
	if resA.Err != nil {
		t.Fatalf("arm A errored, so the test proves nothing about the stamp: %v", resA.Err)
	}
	if len(resA.Schedule) == 0 {
		t.Fatalf("arm A produced no schedule — a stamp on an empty result is not evidence")
	}
	if resA.EngineUsed != "dosport" {
		t.Errorf("arm A: EngineUsed = %q, want \"dosport\" (a plain-fancy in-domain "+
			"screen). If the router legitimately changed, fix the expectation here "+
			"DELIBERATELY — do not derive it from dosPortCanHandle.", resA.EngineUsed)
	}
	if resA.RouteReason != "" {
		t.Errorf("arm A: RouteReason = %q, want \"\" on the dosport arm", resA.RouteReason)
	}

	// ---- arm B: an excluded screen falls to piecewise ----
	// in_advance is clause 1 (`in_advance_or_r78_or_daily`) and is the largest
	// row of the standing contingency table (26,269 cases, r38).
	inB := r41TransportBase()
	inB.Settings.InAdvance = true
	resB := Amortize(inB)
	if resB.Err != nil {
		t.Fatalf("arm B errored, so the test proves nothing about the stamp: %v", resB.Err)
	}
	if len(resB.Schedule) == 0 {
		t.Fatalf("arm B produced no schedule")
	}
	if resB.EngineUsed != "piecewise" {
		t.Errorf("arm B: EngineUsed = %q, want \"piecewise\" (in-advance is excluded "+
			"by clause 1)", resB.EngineUsed)
	}
	if resB.RouteReason != "in_advance_or_r78_or_daily" {
		t.Errorf("arm B: RouteReason = %q, want \"in_advance_or_r78_or_daily\"",
			resB.RouteReason)
	}

	// ---- R42: the stamp survived the result reassignments ----
	//
	// MEASURED, round 41, not assumed: a probe tree with the deferred stamp
	// replaced by a direct assignment at the routing branch reports EngineUsed ==
	// "" on BOTH ARMS — the dosport arm as well as the piecewise one. `result` is
	// wholesale-reassigned after the branch on both paths. So the OBVIOUS
	// implementation of this field produces a permanently empty column, and a
	// consumer that read "" as "the port took it" would label EVERY screen
	// dosport — round 34's 23% denominator inflation again, with nothing left
	// over. That is why the stamp is deferred, and this is the assertion that
	// distinguishes a transport from an assignment.
	if resA.EngineUsed == "" || resB.EngineUsed == "" {
		t.Errorf("R42 VIOLATION: EngineUsed was cleared by a later `result = ...` "+
			"in Amortize (arm A %q, arm B %q). The stamp must be DEFERRED onto the "+
			"named return value, not assigned at the routing branch.",
			resA.EngineUsed, resB.EngineUsed)
	}

	// ---- non-vacuity: the two arms must not agree ----
	if resA.EngineUsed == resB.EngineUsed {
		t.Errorf("NON-VACUITY FAILURE: both arms reported %q. A constant label "+
			"passes every per-arm assertion above and makes every per-engine "+
			"denominator in the project meaningless.", resA.EngineUsed)
	}
}

// TestEngineUsedAgreesWithTheGENGINETrace is R36's side-by-side: the NEW
// instrument (the transported field) against the OLD one (the stderr line every
// published per-engine table was built from). Disagreement means one of the two
// is wrong and every engine-filtered count is suspect.
//
// It reads the FIRST GENGINE line of the call, which is the outermost
// invocation's own line — the same choice dos_fuzzer5's FZ5ENGBEGIN bracket
// makes, and the one round 34 had to correct to.
func TestEngineUsedAgreesWithTheGENGINETrace(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*LoanInput)
	}{
		{"dosport-arm", func(*LoanInput) {}},
		{"in-advance", func(i *LoanInput) { i.Settings.InAdvance = true }},
		{"r78", func(i *LoanInput) { i.Settings.R78 = true }},
		{"not-fancy", func(i *LoanInput) { i.Fancy = false }},
		{"exact-365", func(i *LoanInput) {
			i.Settings.Exact = true
			i.Settings.Basis = types.Basis365
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := r41TransportBase()
			c.mut(&in)

			// Capture stderr for the duration of the one call.
			//
			// ⚠️ dpTraceEngine is latched at package init from the environment
			// (dosport_walk.go:459), so t.Setenv cannot turn the trace on — the first
			// draft of this test did exactly that and every subtest SKIPPED, which is
			// R12's "a skip is not a pass" and would have left the R36 cross-check
			// permanently unexecuted while reading green. This test lives in the
			// package, so it sets the latch directly and restores it.
			savedTrace := dpTraceEngine
			dpTraceEngine = true
			old := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				dpTraceEngine = savedTrace
				t.Fatalf("pipe: %v", err)
			}
			os.Stderr = w
			res := Amortize(in)
			w.Close()
			os.Stderr = old
			dpTraceEngine = savedTrace

			var first string
			sc := bufio.NewScanner(r)
			for sc.Scan() {
				if strings.HasPrefix(sc.Text(), "GENGINE ") {
					first = sc.Text()
					break
				}
			}
			r.Close()

			if first == "" {
				// NOT a skip. The trace is forced on above, so no GENGINE line means
				// the OLD instrument stopped emitting — which would silently retire
				// the cross-check and, more importantly, break every analyzer that
				// still parses it.
				t.Fatalf("no GENGINE line captured with dpTraceEngine forced true — "+
					"the old instrument has stopped emitting. Every per-engine table "+
					"in this project is built by parsing that line. (transported label "+
					"was %q)", res.EngineUsed)
			}

			traceEngine := "piecewise"
			traceReason := ""
			if first == "GENGINE dosport" {
				traceEngine = "dosport"
			} else {
				for _, f := range strings.Fields(first) {
					if strings.HasPrefix(f, "reason=") {
						traceReason = strings.TrimPrefix(f, "reason=")
					}
				}
			}

			if res.EngineUsed != traceEngine {
				t.Errorf("R36 DISAGREEMENT: transported EngineUsed=%q but the GENGINE "+
					"trace says %q (line: %q). Every per-engine denominator this project "+
					"has published came from the trace; if these disagree, one of the two "+
					"is wrong and the tables are suspect.",
					res.EngineUsed, traceEngine, first)
			}
			if res.RouteReason != traceReason {
				t.Errorf("R36 DISAGREEMENT: transported RouteReason=%q but the GENGINE "+
					"trace says %q (line: %q)", res.RouteReason, traceReason, first)
			}
		})
	}
}
