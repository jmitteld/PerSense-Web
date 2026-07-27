package amortization

import (
	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// unreachedAdjPrepass ports DOS's one-sided-adjustment PRE-PASS, the loop that
// runs between TackOnFinalBalloon and TABLE_START (Amortize.pas:1408-1419):
//
//	for i := 1 to nadj do
//	  if (adj[i]^.loanratestatus >= defp) and (adj[i]^.amountstatus < defp) then
//	    begin
//	      if (not EstimateAndRefineAdjPayment(i)) then
//	        exit;
//	      d := h^.payamt;
//	    end
//	  else if (adj[i]^.loanratestatus < defp) and (adj[i]^.amountstatus >= defp) then
//	    begin
//	      if (not EstimateAndRefineAdjRate(i)) then
//	        exit;
//	    end;
//
// Both helpers (Amortize.pas:319-368) are the same shape: save the balloon
// state, then
//
//	RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false,
//	               til_adj, no_value_calc, adjnum)
//
// and report `(not errorflag)`. The caller's bare `exit` abandons the whole
// screen — the message the user sees was already raised inside the walk.
//
// THE LOAD-BEARING DETAIL is the `nil` Output argument. RepayFancyLoan's
// zero-balance stop is gated on `((not h^.lastok) or (Output<>nil))`
// (AMORTOP.pas:1223), so with a KNOWN term and no output stream neither
// disjunct holds and the walk does NOT stop when the balance reaches zero. It
// runs on to `DateComp(WhenToStop^.date, stopdate) >= 0` — h^.lastdate — driving
// the balance negative past the retirement point. The DISPLAY walk (Output<>nil)
// does stop at zero. So DOS solves a one-sided adjustment that lies BEYOND where
// the displayed schedule retires the loan, and a failure there condemns a screen
// whose visible rows never reach that adjustment at all.
//
// That is exactly the gap this closes. The port solves adjustments lazily,
// inline in the walk (engine.go's adjustment block), so an adjustment past the
// last emitted row was simply never solved and never got the chance to refuse.
// 2026-07-25 fuzzer5 seed 8901 (N=1000) — verified against the real DOS engine:
//
//	amort_oracle 466527.57 0.0583950000 9 1 b365 exact plusreg mor=24 \
//	  b36=120317.96 b48=129854.93 b60=29743.65 pre=12:50:12:272.69 \
//	  adj=72:0.1213360000:89834.69 adj=84::70412.26 targ=10379.78 \
//	  pts=0.035979 payhard=64479.95
//	  DOS:  ERR Error: The data you have specified contain an inconsistency.
//	  port: int=117951.36 paid=584478.93 rows=50 (retires 1/1/2029 = month 60)
//
// The balloons retire the loan at month 60, so neither engine's DISPLAY reaches
// the month-84 payment-only adjustment — and indeed dropping that row makes DOS
// print 117951.36 / 584478.93, the port's exact numbers. It is only the pre-pass
// that touches it. The dispatch discriminates precisely as the Pascal does:
//
//	adj=84:0.09:70412.26  (both sides given)  → neither arm fires → schedule
//	adj=84:0.09:          (rate only)         → AdjPayment arm → "did not converge"
//	adj=84::70412.26      (payment only)      → AdjRate arm    → "inconsistency"
//
// SCOPE. This deliberately runs ONLY when a one-sided adjustment lies after the
// last displayed row. DOS pre-solves every one-sided adjustment and freezes the
// answer with `status := outp` so the display reuses it; the port instead
// re-solves inline, and that inline solve is what every earlier fuzzer sweep
// validated. Re-solving reachable adjustments here would substitute the unforced
// walk's answer for the display walk's and put that whole validated surface back
// in play for no fidelity gain, since for a reachable adjustment the two engines
// already agree. Only the unreachable case is unmodelled, so only it is added.
func unreachedAdjPrepass(input LoanInput, settings *Settings,
	payment, truerate, f float64, result *AmortResult) error {
	if result.Err != nil || len(result.Schedule) == 0 || !input.Fancy {
		return nil
	}
	last := result.Schedule[len(result.Schedule)-1].Date
	beyond := false
	for i := range input.Adjustments {
		adj := &input.Adjustments[i]
		if adj.DateStatus < types.InOutDefault {
			continue
		}
		// The Pascal's two arms are `rate given and amount not` / `amount given
		// and rate not`. A row with BOTH sides supplied is applied verbatim and a
		// row with NEITHER is inert, so both fall through the if/else-if untouched
		// — nothing is solved and nothing can refuse. Match that exactly: only a
		// row with exactly one side given is a pre-pass row. Mirrors the
		// hasRate/hasAmt test the inline adjustment block already uses.
		hasRate := adj.LoanRateStatus >= types.InOutDefault
		hasAmt := adj.AmtOK
		if hasRate == hasAmt {
			continue
		}
		// AT-OR-AFTER, not strictly after. DOS's pre-pass walk sets
		// `stopdate := adj[adjnum]^.date` (AMORTOP.pas:1139-1141) and runs
		// `until ... (DateComp(WhenToStop^.date, stopdate) >= 0)` with the
		// zero-balance disjunct switched off by Output=nil — so the walk arrives at
		// the adjustment carrying whatever balance the UNFOLDED schedule has there,
		// which for a loan that retires ON that very row is not zero but the
		// negative overshoot the display folded away. Solving a rate (or a payment)
		// against that state is what raises DOS's refusal, and an adjustment sited
		// exactly on the last displayed row is therefore just as unmodelled by the
		// inline solve as one sited past it.
		//
		// 2026-07-25 fuzzer5 seed 8917 — verified against the real DOS engine:
		//
		//	amort_oracle 418041.91 0.0607680000 10 1 b365_360 exact plusreg mor=24 \
		//	  b36=48057.28 b72=116142.56 b84=13433.38 adj=12:0.0379600000: \
		//	  adj=48:0.0734160000:68259.61 adj=96::60891.41 targ=1710.45 \
		//	  pts=0.004316 payhard=60559.43
		//	  DOS:  ERR Error: The data you have specified contain an inconsistency.
		//	  port: int=123103.22 paid=541145.13 rows=9 (retires 1/1/2032)
		//
		// The month-96 payment-only adjustment falls on 1/1/2032 — the port's LAST
		// row — while the nominal term runs to 1/1/2034. Ablating that one token
		// makes DOS print 123103.22 / 541145.13, the port's exact numbers, so the
		// adjustment contributes nothing to the schedule and everything to the
		// refusal. With `> 0` the pre-pass skipped it by a single day.
		if dateutil.DateComp(adj.Date, last) >= 0 {
			beyond = true
			break
		}
	}
	if !beyond {
		return nil
	}
	// generateFancyScheduleMode's `unforced` mode IS RepayFancyLoan with
	// Output=nil: it never folds a sub-minpmt residual and never breaks out at a
	// zero balance, and it bounds the walk by veryLast (engine.go:3126-3136). So
	// it reaches the late adjustment on a negative balance exactly as DOS's
	// pre-pass walk does, runs the same inline solve, and surfaces the same
	// refusal. Only the error is taken from it — the rows it produces are the
	// solver's scratch, not a schedule anyone displays.
	if pre := generateFancyScheduleMode(input, payment, settings, truerate, f,
		true); pre.Err != nil {
		return pre.Err
	}
	return nil
}
