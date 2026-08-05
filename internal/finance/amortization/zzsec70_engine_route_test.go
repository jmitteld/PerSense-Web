package amortization

// §70 (round 33) — the ENGINE ROUTER is a measured surface, not a detail.
//
// Round 32 rebased the stacked in-scope rate to 475 HARD in 35,000 and
// attributed every one of them to a single SIGNAL class (`HARD:divergent_class`,
// i.e. the totals disagree). Round 33 asked WHERE the two schedules part and
// found the answer was not in the faithful DOS port at all: over a 20-seed,
// 400-case-per-seed corpus (testplan/harness/engine_attribution_arm.py), of
// 4,720 routed cases the faithful port answered 166 with ZERO divergences, and
// all 58 divergences sat in the piecewise fallback:
//
//	engine / first rejecting clause                  cases  diverged      rate
//	piecewise:in_advance_or_r78_or_daily              3401        34  1 in 100
//	piecewise:replace_mode_with_extras                 206        13   1 in 16
//	piecewise:balloon_plus_ao6_or_ao7_adjustment        82         6   1 in 14
//	piecewise:exact_non360                             398         4  1 in 100
//	piecewise:adjustment_carries_amount_ao6             70         1   1 in 70
//	dosport                                            166         0         0
//
// So `dosPortCanHandle`'s eleven clauses are not plumbing — each one is a
// decision to hand a slice of the population to the engine that carries the
// whole measured defect rate. That makes the ROUTE a thing worth pinning.
//
// WHAT THIS TEST GUARDS (it is a characterization test, not a behavior claim):
//
//  1. `dosPortCanHandle` and `dosPortRoute` cannot drift. The refactor that
//     gave the router a reason string made the predicate a wrapper over the
//     reason; this asserts that relationship directly, so a later edit that
//     adds a clause to one and not the other fails here rather than silently
//     mis-attributing a future round's arm.
//  2. Each reason string this round MEASURED is REACHABLE from a plausible
//     input. A reason that no input can produce is a row of zeros in every
//     future attribution table and would read as "this clause costs nothing".
//
// It deliberately does NOT assert that any particular loan SHOULD route one way
// — that is the open question §3b item 1 now carries, and freezing an answer
// here would prejudge it.

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func routeBase() (LoanInput, Loan, Settings) {
	ld := types.NewDateRec(2024, time.January, 1)
	fd := types.NewDateRec(2024, time.February, 1)
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.10,
		NStatus: types.InOutInput, NPeriods: 24,
		PerYrStatus: types.InOutInput, PerYr: 12,
		PayAmtStatus:   types.InOutInput,
		PayAmt:         4614.49,
		LoanDateStatus: types.InOutInput, LoanDate: ld,
		FirstStatus: types.InOutInput, FirstDate: fd,
		LastOK: true,
	}
	in := LoanInput{Loan: loan, Fancy: true}
	s := Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360,
		PlusRegular: true}
	return in, loan, s
}

func TestEngineRoutePredicateMatchesReason(t *testing.T) {
	in, loan, s := routeBase()
	mutate := []struct {
		name string
		f    func(*LoanInput, *Loan, *Settings)
	}{
		{"plain-fancy", func(*LoanInput, *Loan, *Settings) {}},
		{"not-fancy", func(i *LoanInput, _ *Loan, _ *Settings) { i.Fancy = false }},
		{"r78", func(_ *LoanInput, _ *Loan, st *Settings) { st.R78 = true }},
		// Added round 34 while the R78 split was in flight; kept after it was
		// reverted (§71) because it is a distinct shape from the balloon cases.
		{"prepay-unbounded", func(i *LoanInput, _ *Loan, _ *Settings) {
			i.Prepayments = []Prepayment{{
				StartDateStatus: types.InOutInput,
				StartDate:       types.NewDateRec(2024, time.June, 1),
				PerYr:           12,
				PaymentStatus:   types.InOutInput, Payment: 100,
			}}
		}},
		{"in-advance", func(_ *LoanInput, _ *Loan, st *Settings) { st.InAdvance = true }},
		{"daily", func(_ *LoanInput, _ *Loan, st *Settings) { st.Daily = true }},
		{"exact-365", func(_ *LoanInput, _ *Loan, st *Settings) {
			st.Exact = true
			st.Basis = types.Basis365
		}},
		{"huge-term", func(_ *LoanInput, l *Loan, _ *Settings) {
			l.NPeriods = MaxSchedulePeriods + 1
		}},
		{"replace-mode-with-balloon", func(i *LoanInput, l *Loan, st *Settings) {
			st.PlusRegular = false
			i.Balloons = []BalloonPayment{{
				DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.August, 1),
				AmountStatus: types.InOutInput, Amount: 5000,
			}}
		}},
		{"adjustment-with-amount", func(i *LoanInput, _ *Loan, _ *Settings) {
			i.Adjustments = []RateAdjustment{{
				DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.August, 1),
				LoanRateStatus: types.StatusEmpty,
				AmtOK:          true, AmountStatus: types.InOutInput, Amount: 5000,
			}}
		}},
	}

	for _, m := range mutate {
		i2, l2, s2 := in, loan, s
		m.f(&i2, &l2, &s2)
		reason := dosPortRoute(i2, l2, &s2)
		got := dosPortCanHandle(i2, l2, &s2)
		if want := reason == ""; got != want {
			t.Errorf("%s: dosPortCanHandle=%v but dosPortRoute=%q (predicate and "+
				"reason have drifted apart)", m.name, got, reason)
		}
		// Round 34: all THREE views must agree. dosPortRoutes is now the single
		// implementation; the other two are defined from it and this pins that.
		set := dosPortRoutes(i2, l2, &s2)
		if (len(set) == 0) != got {
			t.Errorf("%s: dosPortCanHandle=%v but dosPortRoutes=%v", m.name, got, set)
		}
		if len(set) > 0 && set[0] != reason {
			t.Errorf("%s: dosPortRoute=%q but dosPortRoutes[0]=%q — the FIRST reason "+
				"is what every round-33 attribution parsed; it must not move",
				m.name, reason, set[0])
		}
		seenR := map[string]bool{}
		for _, r := range set {
			if seenR[r] {
				t.Errorf("%s: dosPortRoutes returned %q twice — it is a SET and a "+
					"duplicate would double-count a clause in the co-exclusion profile",
					m.name, r)
			}
			seenR[r] = true
		}
	}
}

// TestEngineRouteCoExclusion is R29's regression guard.
//
// Round 33 read its contingency table — built from dosPortRoute's FIRST reason
// under short-circuiting — as a work queue: widen the most enriched clause and
// the cases it holds reach the faithful port. A probe port with R78 removed from
// `in_advance_or_r78_or_daily` AND NOTHING ELSE then scored 33 HARD divergences
// over seeds 50100-50107, IDENTICAL SEED FOR SEED to the shipped build's 33,
// because every one of those screens is excluded by at least one other clause as
// well. Routing the 56 round-33 repros through that probe put ZERO of them on the
// faithful port.
//
// This test pins the property that made the plan unsound, so that it stays
// visible in the code rather than only in a retracted round note: a screen can
// carry SEVERAL exclusions at once, and neutralising any one of them leaves it on
// the piecewise engine. If a future refactor silently returns to first-match-only
// semantics, the second assertion here fails.
func TestEngineRouteCoExclusion(t *testing.T) {
	in, loan, s := routeBase()

	// A screen excluded by THREE independent clauses: R78, exact×non-360, and a
	// balloon in REPLACE mode. Each of the three is a separate row of the round-33
	// table; under short-circuiting the screen was attributed to R78 alone.
	in.Balloons = []BalloonPayment{{
		DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.August, 1),
		AmountStatus: types.InOutInput, Amount: 5000,
	}}
	s.R78 = true
	s.Exact = true
	s.Basis = types.Basis365
	s.PlusRegular = false

	set := dosPortRoutes(in, loan, &s)
	want := []string{"in_advance_or_r78_or_daily", "exact_non360", "replace_mode_with_extras"}
	if len(set) != len(want) {
		t.Fatalf("co-exclusion set = %v, want exactly %v", set, want)
	}
	for i := range want {
		if set[i] != want[i] {
			t.Fatalf("co-exclusion set = %v, want %v (ORDER is part of the contract: "+
				"dosPortRoute's first element must not move)", set, want)
		}
	}

	// R29 ITSELF: neutralising the clause the short-circuiting table blamed does
	// NOT put the screen on the faithful port. This is the property that made
	// round 33's one-clause-at-a-time remedy measure exactly zero.
	s.R78 = false
	after := dosPortRoutes(in, loan, &s)
	if len(after) == 0 {
		t.Fatal("removing the FIRST clause put the screen on the faithful port — " +
			"that is the assumption R29 exists to deny; if the router really has " +
			"become first-match-only, the co-exclusion profile is meaningless")
	}
	if dosPortCanHandle(in, loan, &s) {
		t.Fatal("dosPortCanHandle became true after neutralising one of three clauses")
	}
}

// TestEngineRouteR78StaysAnExclusion pins §71 — round 34's BLOCKED clause split.
//
// This test asserts the OPPOSITE of what the fidelity argument says it should,
// and that is the point. Read the long comment at the clause in dosport_entry.go
// before touching it.
//
// R78 is INERT in DOS for a fancy loan (every df.c.R78 read is (not fancy)-gated
// or dominated by a `fancy` disjunct), and splitting it out of this clause is a
// measured fidelity GAIN — two screens adjudicated against the oracle, one of
// them a live `go_solved_dos_refused`. Round 34 built the split, gated it, and
// then found that fuzzer5 seed 44016 case 12 — whose ONLY exclusion is this
// clause — routes to AmortizeDOS and never returns. The clause is an ACCIDENTAL
// GUARD over a non-terminating path in the faithful port.
//
// So the split is REVERTED and this test keeps it reverted, because the fidelity
// argument is correct and persuasive and will be rediscovered. Whoever removes
// R78 from that clause must first make AmortizeDOS terminate on §71's screen.
func TestEngineRouteR78StaysAnExclusion(t *testing.T) {
	in, loan, s := routeBase()
	s.R78 = true
	got := dosPortRoutes(in, loan, &s)
	if len(got) != 1 || got[0] != "in_advance_or_r78_or_daily" {
		t.Errorf("a fancy R78 screen routed as %v — if this is a deliberate split, "+
			"§71 (AmortizeDOS does not terminate on seed 44016 case 12) must be "+
			"closed first; see dosport_entry.go at the clause", got)
	}
	// NEGATIVE CONTROL (R19/R24): without this the test would pass just as well
	// if the whole clause had been deleted and something else were catching r78.
	for _, tc := range []struct {
		name string
		set  func(*Settings)
	}{
		{"in_advance", func(st *Settings) { st.InAdvance = true }},
		{"daily", func(st *Settings) { st.Daily = true }},
	} {
		_, _, s2 := routeBase()
		tc.set(&s2)
		if g := dosPortRoutes(in, loan, &s2); len(g) != 1 ||
			g[0] != "in_advance_or_r78_or_daily" {
			t.Errorf("%s: routes = %v, want exactly [in_advance_or_r78_or_daily]",
				tc.name, g)
		}
	}
}

// TestEngineRouteReasonsAreReachable pins the reason strings round 33's
// attribution table was built from. A reason nothing can produce silently reads
// as a clause that costs nothing.
func TestEngineRouteReasonsAreReachable(t *testing.T) {
	in, loan, s := routeBase()

	cases := []struct {
		want string
		f    func(*LoanInput, *Loan, *Settings)
	}{
		{"disabled_or_not_fancy_or_backward", func(i *LoanInput, _ *Loan, _ *Settings) {
			i.Fancy = false
		}},
		{"nperiods_gt_max", func(_ *LoanInput, l *Loan, _ *Settings) {
			l.NPeriods = MaxSchedulePeriods + 1
		}},
		{"in_advance_or_r78_or_daily", func(_ *LoanInput, _ *Loan, st *Settings) {
			st.R78 = true
		}},
		{"exact_non360", func(_ *LoanInput, _ *Loan, st *Settings) {
			st.Exact = true
			st.Basis = types.Basis365
		}},
		{"replace_mode_with_extras", func(i *LoanInput, _ *Loan, st *Settings) {
			st.PlusRegular = false
			i.Balloons = []BalloonPayment{{
				DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.August, 1),
				AmountStatus: types.InOutInput, Amount: 5000,
			}}
		}},
		{"adjustment_carries_amount_ao6", func(i *LoanInput, _ *Loan, _ *Settings) {
			i.Adjustments = []RateAdjustment{{
				DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.August, 1),
				LoanRateStatus: types.StatusEmpty,
				AmtOK:          true, AmountStatus: types.InOutInput, Amount: 5000,
			}}
		}},
		// The two rows below were MISSING from this test's first landing while the
		// docs claimed "every reason in the table is reachable" — caught by the
		// same-day review. balloon_plus_ao6_or_ao7_adjustment is the TABLE'S MOST
		// ENRICHED clause (1 in 14), which made the omission the worst possible one.
		{"balloon_plus_ao6_or_ao7_adjustment", func(i *LoanInput, _ *Loan, _ *Settings) {
			// On-grid balloon with a KNOWN amount + a date-only (AO7) adjustment:
			// no rate, no amount — the confirmed-DOS-bug exclusion.
			i.Balloons = []BalloonPayment{{
				DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.August, 1),
				AmountStatus: types.InOutInput, Amount: 5000,
			}}
			i.Adjustments = []RateAdjustment{{
				DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.June, 1),
				LoanRateStatus: types.StatusEmpty,
			}}
		}},
		{"degenerate_term_or_peryr", func(_ *LoanInput, l *Loan, _ *Settings) {
			l.LastOK = false
		}},
	}

	seen := map[string]bool{}
	for _, c := range cases {
		i2, l2, s2 := in, loan, s
		c.f(&i2, &l2, &s2)
		got := dosPortRoute(i2, l2, &s2)
		if got != c.want {
			t.Errorf("expected reason %q, got %q — the attribution table's row "+
				"for %q no longer corresponds to any reachable input",
				c.want, got, c.want)
			continue
		}
		seen[got] = true
	}
	if len(seen) != len(cases) {
		t.Errorf("only %d of %d reasons reachable", len(seen), len(cases))
	}
}
