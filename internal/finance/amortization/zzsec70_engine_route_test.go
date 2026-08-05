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
