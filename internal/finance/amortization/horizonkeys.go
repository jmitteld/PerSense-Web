package amortization

import (
	"github.com/persense/persense-port/internal/dateutil"
)

// HorizonKeys returns the THREE scope keys of an engine answer, from the ONE
// implementation both consumers share.
//
// WHY THIS FILE EXISTS (round 38, from the round-37 audit's F3). Round 36
// introduced the `horizon` token in cmd/goamort and the comment "zzhorizon_key_
// test.go pins `horizon` equal to fz5MaxYear". The round-37 audit found that
// the test never called fz5MaxYear at all: the fuzzer's key and the token were
// TWO HAND-TYPED COPIES of the same three-way max, coupled only by a comment —
// the third iteration of a false-claim shape on the same file. If either copy
// gained or lost a term, the fuzzer's era split and every Python arm's scope
// split would silently diverge while the "pin" stayed green.
//
// The fix is structural, not another test of a copy: there is now exactly one
// implementation. fz5MaxYear (dos_fuzzer5_test.go) and cmd/goamort's `horizon`
// token both call this function. zzhorizonkeys_fixture_test.go pins the
// function itself against hand-built AmortResult fixtures.
//
// The keys (see zzhorizon_key_test.go for the full history):
//
//	horizon  = max(last schedule row, balloons, resolved LastDate)  — fz5MaxYear,
//	           the key the standing contingency table is built on. Includes the
//	           loan's NOMINAL last regular payment date, which a prepayment-
//	           retired schedule never reaches (R34's bias).
//	reached  = max(last schedule row, balloons) — what the walk PRODUCES; what
//	           the ratified client decision says.
//	lastdate = the resolved LastDate's year (0 if not a valid date) — round
//	           35's mis-key, kept so the retraction stays reproducible.
//
// R2 applies: every term is the ENGINE'S output (gr.Schedule's last row,
// gr.Balloons, gr.LastDate), never a harness-side re-derivation.
func HorizonKeys(gr AmortResult) (horizon, reached, lastdate int) {
	if n := len(gr.Schedule); n > 0 {
		if y := gr.Schedule[n-1].Date.Time.Year(); y > reached {
			reached = y
		}
	}
	for _, b := range gr.Balloons {
		if y := b.Date.Time.Year(); y > reached {
			reached = y
		}
	}
	horizon = reached
	if dateutil.DateOK(gr.LastDate) {
		lastdate = gr.LastDate.Time.Year()
		if lastdate > horizon {
			horizon = lastdate
		}
	}
	return horizon, reached, lastdate
}
