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
// WHAT THIS TEST GUARDS, AND WHAT IT DOES NOT
//
// It guards TERMINATION only. The full fix — restoring poison-and-clamp at the
// three call sites — changes the ANSWER on these screens and is a gated engine
// change for its own round; and the oracle harness Halts on this input
// (legacy/oracle/Globals.pas escalates DOS's non-fatal line-25 message), so what
// DOS would PRINT here is not yet adjudicable. Round 34 landed only the walk's
// iteration bound, which converts the hang into the walk's existing abort path.
//
// ⚠️ Do NOT weaken this into "runs fast". It asserts that AmortizeDOS RETURNS.
// The failing direction was verified by building the pre-fix tree separately: it
// does not return at all, and the deadline below is what turns that into a test
// failure instead of a hung suite.

import (
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
		// The ANSWER is deliberately not asserted — see the header. What matters
		// is that the walk came back at all, with either a schedule or an error.
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
