package amortization

// §71 (round 34) — THE FAITHFUL PORT'S WALK DID NOT TERMINATE, AND THE SCREEN
// THAT PROVES IT NEEDS NO ADVANCED OPTION AND NO ROUTER CLAUSE TO REACH IT.
//
// Found while round 34 was gating an unrelated router change: fuzzer5 seed 44016
// case 12 hung for 29 minutes of CPU on one case. Reduced, the shape is ordinary:
//
//	goamort 100000 0.08 900 12 plusreg loandmy=17.10.2025 firstdmy=17.11.2025 \
//	  pre=12:5000:52:10
//
// — a long-term loan with a WEEKLY prepayment series still live when the walk
// crosses DOS's Julian ceiling (day 70000 = 26 August 2091, §69). Its router
// exclusion set is EMPTY; `DPTRACEENGINE=1` prints `GENGINE dosport`. The peryr=12
// twin returns in milliseconds.
//
// MECHANISM (docs/discrepancies.md §71, audited against the DOS sources):
//
//   - DOS's MDY does not FAIL on Julian overflow, it POISONS: `x.m := errorbyte`
//     and return (VIDEODAT.pas:373). The record stays readable.
//   - DateComp sorts a poisoned record AFTER every real date (INTSUTIL.pas:
//     829-830), so a poisoned prepayment nextdate is simply never consumed again
//     and the walk carries on with regular payments.
//   - RepayFancyLoan then clamps its own horizon: `if (not dateok(stopdate)) then
//     begin stopdate := firstdate; stopdate.y := 100+pred(centurydiv); end`
//     (AMORTOP.pas:1143-1147), with DOS's own comment "Keep going as long as
//     possible".
//
// The port's callers DISCARD dateutil's overflow error and keep the OLD date
// (dosport.go's checkOffBalloon, dosport_entry.go's buildDosEng), and the walk has
// no counterpart to the horizon clamp. So the prepayment's nextdate freezes,
// computeNext's balloonpos=-1 arm does not advance the base date, every walk state
// becomes invariant, and the loop runs forever. Instrumented: 400,000 iterations
// with the date pinned at 2096-12-17 and the balance unchanged.
//
// A ROUTINE FAITHFUL TO THE ORIGINAL, REACHED BY A CALLER THAT IS NOT — the fifth
// time (§59, §66, §67, §68).
//
// WHAT THIS TEST GUARDS — ROUND 35 UPGRADED IT FROM TERMINATION TO THE ANSWER
//
// Round 34 could guard termination only. The oracle DRIVER escalated DOS's
// non-fatal line-25 EMessage to `noteError -> Halt` (legacy/oracle/Globals.pas),
// so this screen came back as `ERR Bad date passed to Julian function: m=-88`
// in 17 ms and what DOS would actually PRINT was unknowable.
//
// Round 35 fixed the driver — behind PERSENSE_ORACLE_SOFT_EMESSAGE, so the
// DEFAULT is byte-identical and no published figure moves (rule 7) — and
// adjudicated the screen against the compiled engine:
//
//	PERSENSE_ORACLE_SOFT_EMESSAGE=1 amort_oracle 100000 0.08 900 12 plusreg \
//	  loandmy=17.10.2025 firstdmy=17.11.2025 pre=12:5000:52:10
//	  -> payment 743.6690 interest 126970.33 paid 226970.33     (15 ms)
//
// DOS terminates, fast, with a real schedule — exactly as AMORTOP.pas:1143-1147's
// "Keep going as long as possible" clamp predicts. The port then had DOS's
// poison-and-clamp restored at the three call sites (dosport.go's
// checkOffBalloon, dosport_entry.go's buildDosEng, and the walk's missing horizon
// clamp) and now reproduces those numbers to the cent in 8 ms.
//
// ⚠️ Do NOT weaken this into "runs fast", and do NOT drop the totals assertion.
// Termination alone is satisfied by a tree that answers by ABORTING: with round
// 34's iteration bound in the tree the PRE binary does not hang — it returns
// `ERR amortization: payment solve did not converge` in about 5-7 s. That is
// termination without fidelity, and TestSec71ScreenMatchesDOS is what
// distinguishes them.
//
// (An earlier draft of this comment said the pre-fix binary "does not return at
// all" and cited two goamort md5s. Both were wrong and both were removed at the
// round-35 audit: the bound had already been landed in round 34, so the pre-fix
// tree returns; and Go embeds build paths in a binary, so a goamort md5 is not
// reproducible provenance for anyone else. Rule 11 — the claim that DOES carry
// its provenance is the oracle command line on TestSec71ScreenMatchesDOS.)

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// sec71Screen builds the reduced repro: 900 monthly periods from 17.11.2025 with
// a 5000-payment weekly prepayment series starting at period 12. The series' own
// stop date (5000 weekly payments from 17.10.2026) lands far past DOS's
// 26-Aug-2091 representation ceiling — Julian day 70000.
func sec71Screen() LoanInput {
	ld := types.NewDateRec(2025, time.October, 17)
	fd := types.NewDateRec(2025, time.November, 17)
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.08,
			NStatus: types.InOutInput, NPeriods: 900,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: ld,
			FirstStatus: types.InOutInput, FirstDate: fd,
		},
		Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2026, time.October, 17),
			PerYr:           52,
			NNStatus:        types.InOutInput, NN: 5000,
			PaymentStatus: types.InOutInput, Payment: 10,
		}},
		Settings: Settings{
			Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360,
			PlusRegular: true,
		},
	}
	return in
}

func TestSec71FaithfulWalkTerminates(t *testing.T) {
	// The PAYMENT-SOLVE arm is the one that ran away. The payment-GIVEN arm on the
	// same input always returned, because the runaway lives in Iterate's TRIAL
	// walk (collect=false), where the walk's `len(rows) > 5000` bound is dead code.
	done := make(chan AmortResult, 1)
	go func() { done <- AmortizeDOS(sec71Screen()) }()

	select {
	case res := <-done:
		// Termination is this test's only job; TestSec71ScreenMatchesDOS owns the
		// answer. Kept separate on purpose: if the fix regresses to an abort, this
		// one still passes and the other one names what actually broke.
		if res.Err == nil && len(res.Schedule) == 0 {
			t.Errorf("AmortizeDOS returned neither an error nor a schedule")
		}
		t.Logf("§71 screen returned: err=%v rows=%d", res.Err, len(res.Schedule))
	case <-time.After(120 * time.Second):
		t.Fatal("AmortizeDOS did not return within 120s on the §71 screen — the " +
			"walk's iteration bound (dosport_walk.go) has been removed or " +
			"weakened, or a new caller has reintroduced the frozen-date loop. " +
			"See docs/discrepancies.md §71.")
	}
}

// TestSec71MonthlyTwinUnaffected is the NEGATIVE CONTROL (R19/R24). The defect is
// specific to prepayment series at peryr 26 or 52, whose AddPeriod arm routes
// through MDY/Julian; peryr 1,2,3,4,6,12,24 are pure field arithmetic and never
// touch it. Without this control the test above would pass just as well if the
// walk had been broken so that nothing ever runs.
func TestSec71MonthlyTwinUnaffected(t *testing.T) {
	in := sec71Screen()
	in.Prepayments[0].PerYr = 12
	res := AmortizeDOS(in)
	if res.Err != nil {
		t.Fatalf("the monthly twin must still amortize normally, got %v", res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Fatal("the monthly twin produced no schedule")
	}
}

// TestSec71ScreenMatchesDOS is the ROUND-35 assertion: the §71 screen does not
// merely return, it returns DOS'S ANSWER.
//
// The goldens carry their provenance (rule 6). They are the compiled DOS engine's
// own output, adjudicated 2026-08-05 with the driver's EMessage escalation lifted:
//
//	PERSENSE_ORACLE_SOFT_EMESSAGE=1 /tmp/oraclebuild/amort_oracle \
//	  100000 0.08 900 12 plusreg loandmy=17.10.2025 firstdmy=17.11.2025 \
//	  pre=12:5000:52:10
//	payment 743.6690 interest 126970.33 paid 226970.33
//
// The gate is a NON-fatal notice on stderr, not a change to the answer: DOS's
// EMessage has no `exit` and no `errorflag := true` at either shipped
// implementation (VIDEODAT.pas:86-100, dos_source/Globals.pas:98-104), so the
// engine computes the same schedule with or without it. Only the harness could
// ever have refused this screen.
//
// ⚠️ WHY THIS IS NOT A CROSS-CHECK TEST. It asserts frozen goldens rather than
// spawning the oracle, because reproducing them needs the SOFT_EMESSAGE gate and
// the default oracle build still Halts here. When the driver escalation is lifted
// by default — its own round, because it converts an INDETERMINATE population
// into a COMPARED one on several surfaces (see the comment on EMessage) — this
// should become an ordinary oracle differential.
func TestSec71ScreenMatchesDOS(t *testing.T) {
	const (
		wantInt  = 126970.33
		wantPaid = 226970.33
	)
	done := make(chan AmortResult, 1)
	go func() { done <- AmortizeDOS(sec71Screen()) }()

	select {
	case res := <-done:
		if res.Err != nil {
			t.Fatalf("§71 screen must ANSWER, not abort: %v\n"+
				"An abort here is round 34's iteration-bound net firing, which "+
				"means the poison-and-clamp fix has been lost from one of "+
				"dosport.go checkOffBalloon / dosport_entry.go buildDosEng / "+
				"dosport_walk.go's AMORTOP.pas:1143-1147 horizon clamp. "+
				"See docs/discrepancies.md §71.", res.Err)
		}
		if math.Abs(res.TotalInt-wantInt) > 0.02 || math.Abs(res.TotalPaid-wantPaid) > 0.02 {
			t.Errorf("§71 screen: int=%.2f paid=%.2f, want %.2f / %.2f (DOS, adjudicated)",
				res.TotalInt, res.TotalPaid, wantInt, wantPaid)
		}
	case <-time.After(120 * time.Second):
		t.Fatal("AmortizeDOS did not return within 120s on the §71 screen")
	}
}

// TestSec71PoisonAndClampControls are the POSITIVE CONTROLS (rule 20). Two
// neighbouring screens on the same shape whose prepayment series does NOT cross
// the 26-Aug-2091 ceiling: the fix must leave them exactly where they were.
//
// Provenance: same oracle, same session, default build (no gate needed — neither
// screen fires EMessage), and each was checked to be byte-identical between the
// pre-fix binary 364f9769c1e57d9055279daf60f16fe0 and the post-fix binary
// e1177fefa22a1900c5fee0e8fdbecfa6.
//
//	... pre=100:52:52:1.36   -> payment 668.2672 interest 501511.23 paid 601511.23
//	... pre=854:246:12:1.36  -> payment 668.3529 interest 501852.15 paid 601852.15
//
// Without these, a fix that simply truncated every long walk would pass the two
// tests above.
func TestSec71PoisonAndClampControls(t *testing.T) {
	cases := []struct {
		name            string
		nn, peryr       int
		payment         float64
		start           types.DateRec
		wantInt, wantPd float64
	}{
		// pre=100:52:52:1.36 — monthsAfter(loanDate 17.10.2025, 100) = 17.2.2034,
		// 52 WEEKLY payments, so the series retires in 2035, far inside the
		// ceiling. This is the "26/52 arm but no overflow" control.
		{"weekly-inside-ceiling", 52, 52, 1.36, types.NewDateRec(2034, time.February, 17), 501511.23, 601511.23},
		// pre=854:246:12:1.36 — monthsAfter(loanDate, 854) = 17.12.2096, i.e. the
		// series STARTS 5 years PAST the 26-Aug-2091 ceiling, on the MONTHLY arm
		// that never calls MDY. The pairing matters: it separates "crosses the
		// ceiling" from "uses the Julian arm", and only the latter is §71.
		{"monthly-past-ceiling", 246, 12, 1.36, types.NewDateRec(2096, time.December, 17), 501852.15, 601852.15},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := sec71Screen()
			in.Prepayments[0].NN = c.nn
			in.Prepayments[0].PerYr = c.peryr
			in.Prepayments[0].Payment = c.payment
			in.Prepayments[0].StartDate = c.start
			res := AmortizeDOS(in)
			if res.Err != nil {
				t.Fatalf("control must amortize: %v", res.Err)
			}
			if math.Abs(res.TotalInt-c.wantInt) > 0.02 || math.Abs(res.TotalPaid-c.wantPd) > 0.02 {
				t.Errorf("int=%.2f paid=%.2f, want %.2f / %.2f (oracle)",
					res.TotalInt, res.TotalPaid, c.wantInt, c.wantPd)
			}
		})
	}
}

// TestSec71CursorFreezeBoundary is SITE 1's OWN GUARD, and it exists because
// site 1 looked inert when it was measured on its own.
//
// Reverting each of §71's three call sites individually (round 35):
//
//	site 1  dosport.go      checkOffBalloon adopts the poisoned nextdate
//	site 2  dosport_entry.go buildDosEng adopts the poisoned stopdate
//	site 3  dosport_walk.go  the AMORTOP.pas:1143-1147 horizon clamp
//
// against the tests AS THEY STOOD BEFORE THIS ONE WAS WRITTEN, sites 2 and 3
// each failed and site 1 PASSED; and a 150-screen ceiling-arm differential of
// "sites 2+3 only" against the full fix returned FIXED=0, NEW=0. By rule 16 that
// is an unconfirmed change and site 1 should not have landed — EXCEPT that the
// source predicts precisely where it bites, and the prediction holds. This test
// is that prediction, so against the CURRENT test set all three sites fail when
// reverted individually (verified).
//
// Sites 2+3 terminate §71's screen by CLAMPING at 2049, which is EARLIER than
// the 26-Aug-2091 ceiling, so the prepayment cursor is normally retired long
// before it can freeze. The clamp only engages when stopdate is INVALID. So the
// exposed band is: a series whose own stop date is still VALID — hence at or
// just under the ceiling, because AddNPeriods' 26/52 arm went through MDY — while
// the schedule's very_last is valid and LATER. There the cursor advances to
// within one week of day 70000, AddPeriod overflows, and with the site-1 guard
// in place the cursor FREEZES at a date that is still <= its own stopdate: it is
// never retired, computeNext re-emits it forever, and the walk is invariant.
//
// Measured on the §71 screen, sweeping the series length (goamort
// `pre=12:<nn>:52:10`, everything else unchanged):
//
//	nn      sites 2+3 only              full fix                    DOS
//	3384    501782.70                   501782.70                   501782.70
//	3385    ERR did not converge  <---  501792.40                   501792.40
//	3386    126970.33                   126970.33                   126970.33 (soft)
//
// ONE value of nn wide. Below it nothing overflows; above it the stop date
// itself is poisoned and site 2 hands the walk a wall. At exactly the boundary
// site 1 is the only thing standing between the shipped product and a hang —
// and the answer it produces is DOS's, to the cent.
//
// Provenance: PERSENSE_ORACLE_SOFT_EMESSAGE=1 was NOT needed for nn=3385 (no
// poisoned date reaches Julian there); the default oracle build prints
// `payment 631.0471 interest 501792.40 paid 601792.40` directly.
func TestSec71CursorFreezeBoundary(t *testing.T) {
	const (
		wantInt  = 501792.40
		wantPaid = 601792.40
	)
	in := sec71Screen()
	in.Prepayments[0].NN = 3385

	done := make(chan AmortResult, 1)
	go func() { done <- AmortizeDOS(in) }()

	select {
	case res := <-done:
		if res.Err != nil {
			t.Fatalf("nn=3385 must ANSWER, not abort: %v\n"+
				"This is the one-period band where the prepayment cursor "+
				"freezes while its own stop date is still valid. An abort "+
				"means dosport.go's checkOffBalloon has stopped adopting the "+
				"poisoned date (AMORTOP.pas:559 is unconditional).", res.Err)
		}
		if math.Abs(res.TotalInt-wantInt) > 0.02 || math.Abs(res.TotalPaid-wantPaid) > 0.02 {
			t.Errorf("nn=3385: int=%.2f paid=%.2f, want %.2f / %.2f (DOS)",
				res.TotalInt, res.TotalPaid, wantInt, wantPaid)
		}
	case <-time.After(120 * time.Second):
		t.Fatal("AmortizeDOS did not return within 120s at the cursor-freeze boundary")
	}
}
