package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// zzhorizonkeys_fixture_test.go — ROUND 38. THE KEY FUNCTION ITSELF, PINNED.
//
// The round-37 audit (F3): "zzhorizon_key_test.go never calls fz5MaxYear —
// fz5MaxYear and the goamort token computation are two hand-typed copies of the
// same three-way max, coupled by a comment. If either copy gains or loses a
// term, the fuzzer's era split and every Python arm's scope split silently
// diverge and this test stays green."
//
// Round 38's fix is structural (HorizonKeys is now the only implementation and
// both consumers delegate to it), and THIS file pins the function's semantics
// against hand-built fixtures so that a change to any of the three terms is a
// seen failure, not a silent re-stratification. zzhorizon_key_test.go remains
// the CLI-level guard (the token line's FORMAT, reached <= horizon, and the
// keys-can-split witnesses — it does NOT compare the token to an engine-side
// value; with one shared implementation that equality is structural, not
// tested); this file is the in-process guard the audit asked for.
//
// Every fixture asserts ALL THREE keys, because R34's whole finding is that
// the keys differ and each difference is a scope claim.

func hzFixture(rowYear int, balloonYears []int, lastDateYear int, lastDateOK bool) AmortResult {
	var r AmortResult
	if rowYear > 0 {
		r.Schedule = []PaymentRecord{
			{PayNum: 1, Date: types.NewDateRec(rowYear-1, time.March, 15)},
			{PayNum: 2, Date: types.NewDateRec(rowYear, time.March, 15)},
		}
	}
	for _, y := range balloonYears {
		r.Balloons = append(r.Balloons, ResolvedBalloon{Date: types.NewDateRec(y, time.June, 1)})
	}
	if lastDateOK {
		r.LastDate = types.NewDateRec(lastDateYear, time.December, 1)
	}
	// !lastDateOK leaves the zero DateRec, which dateutil.DateOK rejects.
	return r
}

func TestHorizonKeysFixtures(t *testing.T) {
	cases := []struct {
		name                       string
		gr                         AmortResult
		horizon, reached, lastdate int
	}{
		{"plain: all three coincide",
			hzFixture(2029, nil, 2029, true), 2029, 2029, 2029},
		{"early retirement (R34's bias): nominal LastDate past the walk — horizon takes it, reached must NOT",
			hzFixture(2030, nil, 2101, true), 2101, 2030, 2101},
		{"balloon past the last row: both horizon and reached take it",
			hzFixture(2044, []int{2137}, 2044, true), 2137, 2137, 2044},
		{"prepayment carries rows past LastDate (§72's shape): reached == horizon > lastdate",
			hzFixture(2100, nil, 2044, true), 2100, 2100, 2044},
		{"invalid LastDate contributes NOTHING, to either horizon or lastdate",
			hzFixture(2050, nil, 0, false), 2050, 2050, 0},
		{"empty schedule, balloon only",
			hzFixture(0, []int{2088}, 0, false), 2088, 2088, 0},
		{"max over several balloons, not the last one",
			hzFixture(2040, []int{2090, 2060}, 2041, true), 2090, 2090, 2041},
	}
	for _, c := range cases {
		h, r, l := HorizonKeys(c.gr)
		if h != c.horizon || r != c.reached || l != c.lastdate {
			t.Errorf("%s: HorizonKeys = (%d, %d, %d), want (%d, %d, %d)",
				c.name, h, r, l, c.horizon, c.reached, c.lastdate)
		}
	}
}

// TestFz5MaxYearIsHorizonKeys pins the delegation itself: the fuzzer's era key
// IS the shared, selected HorizonKeys key and never its own arithmetic, on the
// fixtures where the three keys differ most. If fz5MaxYear ever grows a private
// three-way max again — the audit-F3 defect — this fails.
//
// 🚨 ROUND 48: THIS USED TO PIN `horizon` SPECIFICALLY, AND IT WAS THE GUARD
// THAT FAILED WHEN THE RATIFIED KEY WAS FINALLY EXECUTED. That is the guard
// working: decision 3a.11 moves the standing key to `reached`.
//
// 🚨 AND THE ROUND-48 AUDIT KILLED THE FIRST VERSION OF THIS RE-POINTING. It
// asserted only that fz5MaxYear returned ONE OF the three keys — a disjunction
// over three ints, which is far weaker than the equality it replaced. The
// auditor demonstrated it: a mutant that DROPS THE BALLOON TERM (the canonical
// audit-F3 defect, and the exact shape round 22 wrote the original comment
// about) returns 2044 on the balloon-dominant fixture, 2044 is `lastdate`, and
// the disjunction is satisfied. The pre-round-48 form killed that mutant; the
// weakened form survived 4 of 4. Worse, the coincident fixture makes the
// disjunction UNCONDITIONALLY TRUE — all three keys are equal there, so that row
// asserted nothing at all (R49: the sample cannot express the difference).
//
// ✅ THE ASSERTION IS NOW AN EQUALITY AGAINST THE SELECTED KEY, which is both
// stronger than the disjunction and correct under the new standing key.
func TestFz5MaxYearIsHorizonKeys(t *testing.T) {
	// 🚨 CLEAR THE OPERATOR'S OVERRIDE FIRST. This test pins the STANDING key,
	// which is a claim about the DEFAULT — not about whatever the current shell
	// asked for. The first version omitted this and the round's own post-edit
	// gate caught it: `PERSENSE_SCOPE_KEY=horizon go test ./...` failed here,
	// which would have broken the very both-keys-in-one-session comparison R36
	// requires and that this round is built on. A guard that forbids a supported
	// invocation is a guard that will be deleted by whoever needs that
	// invocation.
	scopeKeyEnvCleared(t)
	sawSplit := false
	for _, gr := range []AmortResult{
		hzFixture(2030, nil, 2101, true),         // horizon != reached
		hzFixture(2044, []int{2137}, 2044, true), // balloon-dominant
		hzFixture(2029, nil, 2029, true),         // coincident
	} {
		h, r, l := HorizonKeys(gr)
		if h != r || r != l {
			sawSplit = true
		}
		if got, want := fz5MaxYear(gr), fz5ScopeYear(gr); got != want {
			t.Errorf("fz5MaxYear = %d, selected scope key = %d — the fuzzer's era "+
				"key has grown its own arithmetic again (audit F3)", got, want)
		}
		// And it must equal the SHARED key by name, not merely agree with the
		// selector: both could drift together if fz5ScopeYear stopped delegating.
		if got := fz5MaxYear(gr); got != r {
			t.Errorf("fz5MaxYear = %d, HorizonKeys `reached` = %d (horizon %d, "+
				"lastdate %d) — the standing key is `reached` (3a.11) and the "+
				"fuzzer is not using it", got, r, h, l)
		}
	}
	// R24/R20 — the fixtures must actually SPLIT the keys, or every assertion
	// above is satisfied by any of the three and this guard is decorative. This
	// is the assertion whose absence let the weakened disjunction pass.
	if !sawSplit {
		t.Error("no fixture splits the three keys — an equality against `reached` " +
			"cannot distinguish it from `horizon` or `lastdate` here, so this guard " +
			"is asserting nothing (R49)")
	}
}

// TestHorizonKeysReachedNeverExceedsHorizon is the structural inequality the
// CLI test asserts per-screen, pinned here for all fixtures: horizon is a max
// over reached plus one more term, so reached > horizon is impossible unless
// someone edits one branch and not the other.
func TestHorizonKeysReachedNeverExceedsHorizon(t *testing.T) {
	fixtures := []AmortResult{
		hzFixture(2030, nil, 2101, true),
		hzFixture(2100, nil, 2044, true),
		hzFixture(0, []int{2088}, 0, false),
		hzFixture(2040, []int{2090, 2060}, 2041, true),
	}
	for i, gr := range fixtures {
		h, r, _ := HorizonKeys(gr)
		if r > h {
			t.Errorf("fixture %d: reached %d > horizon %d", i, r, h)
		}
	}
}
