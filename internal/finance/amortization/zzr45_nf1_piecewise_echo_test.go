package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// ROUND 45 — NF-1. THE ADJUSTMENT AMOUNT ECHO ON THE **PIECEWISE** ENGINE.
//
// ⚠️ READ THIS BEFORE CHANGING ANYTHING HERE.
//
// Round 39D reported NF-1 as "the adjustment echo is only implemented on ONE
// engine": any AO6 / in-advance / R78 / daily screen still showed Nate's blank
// cell. Rounds 40-44 carried it untouched. The root cause, found in round 45 by
// reading AMORTOP.pas rather than by fuzzing:
//
//	AMORTOP.pas:1571-1592, Re_Amortize, the `not amtok` arm ("compute new
//	payment amount"):
//
//	  if (user_nballoons > 0) or (npre > 0) or ((df.c.exact) and (df.c.basis<>x360)) then
//	    begin
//	      ...
//	      if Iterate(p, usap, Payment.date, t, d, til_adj) then
//	        begin
//	          adj[next_adj]^.amount       := d;      <-- GATED store
//	          adj[next_adj]^.amountstatus := outp;
//	          adj[next_adj]^.amtok        := true;   <-- the latch, GATED
//	        end
//	      ...
//	    end;
//
//	  adj[next_adj]^.amount       := d;              <-- UNCONDITIONAL store
//	  adj[next_adj]^.amountstatus := outp;           <-- OUTSIDE the gate
//
// DOS stores the re-amortized payment on EVERY crossing. Only the `amtok` LATCH
// ("a later walk may reuse this") lives behind the balloons-or-prepay-or-exact
// gate. `dosport_walk.go:867-870` ports that faithfully — which is exactly why
// 39D measured the dosport route clean over 558 screens.
//
// `engine.go` put the WHOLE store inside the gate (the gate is engine.go:4416;
// the store sat at :4565-4567 together with `AmtOK = true`). So on any piecewise
// screen with no balloon, no prepayment and not exact-non-360, the amount DOS
// paints was computed, used to build the schedule, and then thrown away — the
// echo's "has a value" test (`a.AmtOK || a.AmountStatus == types.InOutOutput`,
// engine.go:1936) saw neither flag and the cell stayed BLANK.
//
// 🚨 THE FIX MUST NOT SET `AmtOK`. `hasAmt := adj.AmtOK` (engine.go:4157) is the
// port of DOS's `if (adj[next_adj]^.amtok)` branch selector at AMORTOP.pas:1515.
// Setting it would send the NEXT walk over the same screen down the AO6 arm and
// change the schedule. DOS does not set it here and neither do we.
//
// 🚨 THE PRE-DECLARATION, WRITTEN BEFORE MEASURING (R36): THE FIX CANNOT MOVE
// ANY ARITHMETIC. Every other reader of an ADJUSTMENT's AmountStatus thresholds
// at `>= types.InOutDefault` (=2) — backward.go:2506/2512/2524, engine.go:3510,
// payoff.go:453. The write moves the field from InOutEmpty (0) to InOutOutput
// (1); BOTH are < 2, so no reader crosses. The ONLY consumer whose behaviour
// changes is the echo's third argument. This is a display-transport correction,
// and the schedule totals must be byte-identical before and after.
//
// ⚠️ IN-ADVANCE CANNOT BE TESTED HERE, AND THAT IS DOS'S CHOICE, NOT A GAP:
//
//	amort_oracle 100000 0.07 360 12 adj=84:0.08: inadv adjdump
//	  → ERR Sorry - you can't change rates when interest is computed in advance.
//
// So the reachable piecewise ∩ rate-adjustment population is R78, daily, and the
// AO6-carrying screens. 39D's "in-advance" clause of NF-1 is UNREACHABLE for a
// rate adjustment, and this file records that rather than leaving it implied.
//
// ⚠️ Every oracle line below was produced in round 45 by
// /tmp/oraclebuild/amort_oracle, built by legacy/oracle/build_linux.sh:114 with
//
//	-Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX     (NO -dACTU; R47)

// r45MustBePiecewise is the shared in-guard POSITIVE CONTROL (R24/R49). These
// fixtures exist ONLY to cover the piecewise engine — NF-1 was never open on
// dosport. If a router change moved them, they would silently start passing for
// the wrong reason, which is the failure R49 exists to prevent.
func r45MustBePiecewise(t *testing.T, in LoanInput, r AmortResult) {
	t.Helper()
	if r.EngineUsed != "piecewise" {
		t.Fatalf("POSITIVE CONTROL FAILED: this fixture must be answered by the "+
			"PIECEWISE engine or it tests nothing. EngineUsed=%q RouteReason=%q. "+
			"If the router changed, find another piecewise-routed adjustment "+
			"screen; do NOT relax this check.", r.EngineUsed, r.RouteReason)
	}
	// And prove DOS's gate is genuinely CLOSED on this screen, so the echoed
	// value can only have come from the UNCONDITIONAL store at AMORTOP.pas:1591.
	// Without this the test would still pass if the fix were (wrongly) made by
	// widening the gate instead of adding the unconditional store.
	if len(in.Balloons) != 0 || len(in.Prepayments) != 0 ||
		(in.Settings.Exact && in.Settings.Basis != types.Basis360) {
		t.Fatalf("POSITIVE CONTROL FAILED: this fixture must have NO balloons, NO "+
			"prepayments and must not be exact-on-a-non-360-basis, so that "+
			"AMORTOP.pas:1571's gate is FALSE. balloons=%d prepayments=%d "+
			"exact=%v basis=%v", len(in.Balloons), len(in.Prepayments),
			in.Settings.Exact, in.Settings.Basis)
	}
}

// TestR45NF1PiecewiseAdjustmentAmountEcho — the round-45 regression for NF-1.
//
// R78 routes AWAY from dosport (`dosPortCanHandle`'s
// `in_advance_or_r78_or_daily` rejection), so this screen is answered by the
// PIECEWISE engine — the engine that answers Nate's screens.
//
//	amort_oracle 100000 0.07 360 12 adj=84:0.08: r78 adjdump
//	  → adjrow 1 date 1/1/2031 rate 0.0800000000 ratestatus 3 \
//	    amount 723.211515 amtstatus 1 amtok FALSE
//	  → payment 665.3025 interest 155491.79 paid 255491.79
//
// Note `amtok FALSE` in DOS TOO. That is the whole point: keying the echo on
// amtok loses exactly the row DOS paints.
func TestR45NF1PiecewiseAdjustmentAmountEcho(t *testing.T) {
	in := r45R78AdjScreen()
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize refused: %v", r.Err)
	}
	r45MustBePiecewise(t, in, r)

	if len(r.Adjustments) != 1 {
		t.Fatalf("expected 1 echoed adjustment, got %d", len(r.Adjustments))
	}
	a := r.Adjustments[0]

	if !a.AmountSolved {
		t.Errorf("NF-1: AmountSolved is false on a PIECEWISE screen. DOS reports " +
			"amtstatus 1 (outp) on this row and paints 723.211515 into its grid — " +
			"this is the blank cell Nate reported. Check that engine.go performs " +
			"DOS's UNCONDITIONAL store (AMORTOP.pas:1591-1592) OUTSIDE the " +
			"balloons-or-prepay-or-exact gate.")
	}
	if math.Abs(a.Amount-723.211515) > 5e-4 {
		t.Errorf("echoed adjustment amount %.6f, DOS 723.211515", a.Amount)
	}
	// The rate was the CALLER's: it must not echo as solved, or the UI paints it
	// green and the request builder reads it back as blank on the next submit.
	if a.RateSolved {
		t.Errorf("RateSolved is true on a rate the CALLER supplied")
	}
	if math.Abs(a.Rate-0.08) > 1e-9 {
		t.Errorf("echoed adjustment rate %.10f, want the caller's 0.08", a.Rate)
	}
}

// TestR45NF1Round39DMinimalRepro is NF-1's CANONICAL case — the exact command
// ROUND39D published as the minimal repro, carried verbatim for five rounds and
// never turned into a test. It routes piecewise on
// `adjustment_carries_amount_ao6` and it carries BOTH arms on one screen, so it
// is simultaneously the positive case (row 1, AO5, DOS solves the amount) and
// the negative case (row 2, AO6, the amount is the USER'S and must not be
// relabelled as solved).
//
//	amort_oracle 105319.00 0.0648162 120 12 payhard=947.00 b365 prepaid \
//	  adj=17:0.0422379: adj=67::1303.00 adjdump
//	  → adjrow 1 date 6/1/2025 rate 0.0422379000 ratestatus 3 \
//	    amount 1142.997616 amtstatus 1 amtok FALSE
//	  → adjrow 2 date 8/1/2029 rate 0.1040701364 ratestatus 1 \
//	    amount 1303.000000 amtstatus 3 amtok TRUE
//	  → payment 947.0000 interest 36988.85 paid 142307.85
//
// Reproduced byte-for-byte in round 45 before the fix was written.
func TestR45NF1Round39DMinimalRepro(t *testing.T) {
	s := gzSettings(12, types.Basis365, false, true, false, false, false)
	in := r39Loan(105319.00, 0.0648162, 120, 12, s)
	in.Fancy = true
	in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, 947.00
	in.Adjustments = []RateAdjustment{
		{DateStatus: types.InOutInput, Date: types.NewDateRec(2025, time.June, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.0422379},
		{DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.August, 1),
			AmountStatus: types.InOutInput, Amount: 1303.00, AmtOK: true},
	}

	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize refused: %v", r.Err)
	}
	r45MustBePiecewise(t, in, r)
	if len(r.Adjustments) != 2 {
		t.Fatalf("expected 2 echoed adjustments, got %d", len(r.Adjustments))
	}

	// Row 1 — AO5. DOS solved it; the port threw it away. This is NF-1.
	if a := r.Adjustments[0]; !a.AmountSolved {
		t.Errorf("NF-1 (39D's repro, row 1): AmountSolved false; DOS paints " +
			"1142.997616 with amtstatus 1. This is the blank cell.")
	} else if math.Abs(a.Amount-1142.997616) > 5e-4 {
		t.Errorf("row 1 echoed amount %.6f, DOS 1142.997616", a.Amount)
	}

	// Row 2 — AO6. The NEGATIVE control, and it is the one that matters: a fix
	// that fills the echo indiscriminately would mark the USER'S OWN 1303.00 as
	// solved, the UI would paint it green, and the request builder would then
	// read it back as blank on the next submit — turning AO5 into AO7. That is
	// the round-39 defect running backwards.
	a2 := r.Adjustments[1]
	if a2.AmountSolved {
		t.Errorf("row 2: AmountSolved is TRUE on an amount the CALLER supplied " +
			"(DOS: amtstatus 3 = input). The fix has overshot — it must sit under " +
			"engine.go's `if !hasAmt` arm, mirroring DOS's `not amtok` branch.")
	}
	if math.Abs(a2.Amount-1303.00) > 5e-4 {
		t.Errorf("row 2 echoed amount %.6f, want the caller's 1303.00", a2.Amount)
	}
	// DOS SOLVED row 2's rate (ratestatus 1 = outp) — the AO6 arm.
	if !a2.RateSolved {
		t.Errorf("row 2: RateSolved is false; DOS reports ratestatus 1 (outp) and " +
			"paints 0.1040701364 — the AO6 rate solve.")
	} else if math.Abs(a2.Rate-0.1040701364) > 1e-6 {
		t.Errorf("row 2 echoed rate %.10f, DOS 0.1040701364", a2.Rate)
	}
}

// TestR45NF1BlankAdjustmentStillEchoesTheReamortizedPayment pins the strongest
// statement of DOS's unconditional store, and it is the mutant-killer for any
// fix that tries to gate the store on "did the user change something": an
// adjustment row that changes NEITHER the rate NOR the amount still gets the
// re-amortized payment stored, because Re_Amortize ran.
//
//	amort_oracle 100000 0.07 360 12 adj=84:: adjdump
//	  → adjrow 1 date 1/1/2031 rate 0.0000000000 ratestatus 0 \
//	    amount 665.302495 amtstatus 1 amtok FALSE
//
// It is simultaneously the RATE arm's negative control: ratestatus 0 means the
// rate cell must stay ABSENT, not echo as a solved 0.0000%.
func TestR45NF1BlankAdjustmentStillEchoesTheReamortizedPayment(t *testing.T) {
	s := gzSettings(12, types.Basis360, false, false, false, true, false)
	in := r39Loan(100000, 0.07, 360, 12, s)
	in.Fancy = true
	in.Adjustments = []RateAdjustment{{
		DateStatus: types.InOutInput, Date: types.NewDateRec(2031, time.January, 1)}}

	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize refused: %v", r.Err)
	}
	r45MustBePiecewise(t, in, r)
	if len(r.Adjustments) != 1 {
		t.Fatalf("expected 1 echoed adjustment, got %d", len(r.Adjustments))
	}
	a := r.Adjustments[0]

	if !a.AmountSolved {
		t.Errorf("NF-1: a rate-and-amount-blank adjustment echoed no amount. DOS " +
			"stores the re-amortized payment on EVERY crossing (AMORTOP.pas:1591), " +
			"whether or not the row changed anything, and reports amtstatus 1 with " +
			"amount 665.302495 here.")
	} else if math.Abs(a.Amount-665.302495) > 5e-4 {
		t.Errorf("echoed amount %.6f, DOS 665.302495", a.Amount)
	}
	// DOS: ratestatus 0 (empty). The rate cell must stay blank.
	if a.RateSolved || a.Rate != 0 {
		t.Errorf("the rate echoed as %.10f solved=%v on a row DOS leaves EMPTY "+
			"(ratestatus 0). A 0 here paints \"0.0000%%\" into a blank DOS cell.",
			a.Rate, a.RateSolved)
	}
}

// r45R78AdjScreen is the shared fixture, kept as a function so the mutation pass
// and the controls cannot drift apart.
func r45R78AdjScreen() LoanInput {
	s := gzSettings(12, types.Basis360, false, false, false, true, false)
	in := r39Loan(100000, 0.07, 360, 12, s)
	in.Fancy = true
	in.Adjustments = []RateAdjustment{{
		DateStatus: types.InOutInput, Date: types.NewDateRec(2031, time.January, 1),
		LoanRateStatus: types.InOutInput, LoanRate: 0.08}}
	return in
}
