package amortization

import (
	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// CheckPrepaymentStops is the port of the count-to-date arm of DOS's
// CheckPrepayments (AMORTOP.pas:400-475):
//
//	ok1 := (startdatestatus >= defp);
//	ok2 := (peryrstatus >= defp) and (pre[i]^.peryr > 0);
//	ok3 := (nnstatus >= defp);
//	if (ok1 and ok2) then
//	  begin
//	    if (ok3) then
//	      begin
//	        AddNPeriods(startdate, stopdate, pre[i]^.peryr, pred(nn));
//	        stopdatestatus := outp;
//	      end
//	    ...
//
// This matters far more than it looks. DOS converts a COUNT-specified prepayment
// series into a DATE-specified one exactly once, up front, and from that moment
// the count is never consulted again: the forward walk retires a series solely on
//
//	if (DateComp(nextdate, stopdate) > 0) then dec(npre)
//
// (CheckOffBalloon, AMORTOP.pas:545-571 — there is no count bound anywhere in the
// walk). So the number of extras DOS actually emits is whatever fits on or before
// that ONE derived date, which is not necessarily `nn`.
//
// The gap opens because AddNPeriods is NOT n applications of AddPeriod. For
// peryr in {1,2,3,4,6,12,24} AddNPeriods (INTSUTIL.pas:1392-1410) takes a year
// shortcut — `lastdate.y := firstdate.y + (n div peryr)` and then iterates only
// the remainder. For peryr=24 (semi-monthly) with an anchor day that sits OFF the
// natural 1st/16th grid, the two disagree: AddPeriod(1/31, 24, 31, add) = 2/16,
// i.e. the NEXT month's 16th, so iterated stepping runs a permanent half-slot
// ahead of the year shortcut. Concretely, for start 1/31/2028, peryr 24, nn 193:
//
//	AddNPeriods(1/31/2028, 24, 192)      = 1/31/2036   (DOS: 192 extras emitted)
//	192 iterated AddPeriod steps         = 2/1/2036    (193 extras emitted)
//
// Go bounded the series by the COUNT everywhere and, where it did derive a stop
// date, derived it by iterated AddPeriod — so it emitted one extra 287.73
// payment on 2/1/2036 that DOS never emits, and every row after it was shifted.
// fuzzer5 seed 21030: DOS solvedamount 290522.756673, Go 290661.154241.
//
// Days 1, 16, 28, 29 and 30 all keep AddNPeriods == n x AddPeriod for peryr=24,
// which is why the ablation fingered day 31 alone.
//
// Running this as a pre-pass at the single public entry (Amortize, before the
// engine dispatch and before the term solve) makes StopDate the authoritative
// bound for BOTH engines and every downstream consumer, exactly as it is in DOS.
// The mutation is in place on the caller's backing array — deliberate, and the
// same convention AO10's duration solve already uses, so the API/UI echo the
// derived stop date the way DOS's `stopdatestatus := outp` does.
func CheckPrepaymentStops(prepays []Prepayment) {
	for i := range prepays {
		pp := &prepays[i]
		ok1 := pp.StartDateStatus >= types.InOutDefault && dateutil.DateOK(pp.StartDate)
		ok2 := pp.PerYrStatus >= types.InOutDefault && pp.PerYr > 0
		ok3 := pp.NNStatus >= types.InOutDefault && pp.NN > 0
		if !(ok1 && ok2 && ok3) {
			continue
		}
		// DOS derives the stop date even when one is already on the screen — its
		// CheckPrepayments gives the COUNT priority. The port cannot, because it
		// runs this prepass at more than one entry (Amortize plus each backward
		// solver, mirroring how snapMoratoriumFirstRepay is repeated) and the
		// post-term-solve prepayment-window rewrite (backward.go:2092-2099)
		// deliberately writes a re-based window back into the SAME slice. Re-deriving
		// from the count on a second pass would clobber that rewrite and throw the
		// terminating balloon years off. Deriving only into an ABSENT stop date is
		// observationally identical on a first pass — DOS's own count-to-date
		// conversion is likewise a one-shot screen prepass.
		if pp.StopDateStatus >= types.InOutDefault && dateutil.DateOK(pp.StopDate) {
			continue
		}
		stop, err := dateutil.AddNPeriods(pp.StartDate, pp.PerYr, pp.NN-1)
		if err != nil {
			continue
		}
		// StopDateStatus goes to InOutInput (present) rather than DOS's `outp`.
		// DOS's status ladder is badp=-1 < outp=1 < defp=2 < inp=3, and DOS's walk
		// never gates on the status at all — CheckOffBalloon just compares
		// nextdate to stopdate. The port's ~10 consumers instead spell "a stop date
		// is present" as `StopDateStatus >= types.InOutDefault`, which `outp` would
		// fail, silently restoring the count-bounded behaviour this pre-pass exists
		// to remove. Same reasoning and same choice as backward.go:2092-2099 and
		// the AO10 duration-solve write-backs (dosport_entry.go:650, engine.go:1124).
		// The one gate that needs DOS's finer reading of `outp` — the post-term-solve
		// window rewrite — reads Prepayment.stopFromNN instead.
		pp.StopDate, pp.StopDateStatus = stop, types.InOutInput
		pp.stopFromNN = true
	}
}
