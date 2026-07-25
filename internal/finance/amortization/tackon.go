package amortization

import (
	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// ---------------------------------------------------------------------------
// DetermineVeryLast (AMORTOP.pas:1293-1304)
// ---------------------------------------------------------------------------

// determineVeryLast returns the LATEST of {lastDate, every balloon date,
// every prepayment stop date} — the date the schedule must actually run to.
//
//	procedure DetermineVeryLast;
//	begin
//	  if (nballoons > 0) and (DateComp(balloon[nballoons]^.date, h^.lastdate) > 0) then
//	    very_last := balloon[nballoons]^.date
//	  else very_last := h^.lastdate;
//	  for i := 1 to npre do
//	    if (DateComp(pre[i]^.stopdate, very_last) > 0) then very_last := pre[i]^.stopdate;
//	end;
//
// Otherwise a balloon dated after the last regular payment, or a prepayment
// series whose stop date extends past lastDate, would be silently cut off.
//
// Extracted from generateFancyScheduleMode (2026-07-24, §46) so
// tackOnFinalBalloon resolves the terminating-balloon date from the identical
// rule the walk itself uses — a divergence between the two would place the
// tacked-on row on a date the schedule never reaches.
func determineVeryLast(loan *Loan, balloons []BalloonPayment, prepays []Prepayment) types.DateRec {
	veryLast := loan.LastDate
	for _, b := range balloons {
		if b.DateStatus >= types.InOutDefault &&
			dateutil.DateComp(b.Date, veryLast) > 0 {
			veryLast = b.Date
		}
	}
	for _, pp := range prepays {
		if pp.StopDateStatus >= types.InOutDefault &&
			dateutil.DateComp(pp.StopDate, veryLast) > 0 {
			veryLast = pp.StopDate
		}
		// NN-derived stop date (DOS DetermineVeryLast + CheckPrepayments,
		// AMORTOP.pas:400/1293-1304): a series specified by COUNT (NN extras) with
		// no explicit stop date still ends on a definite date — StartDate plus
		// (NN-1) prepayment periods. When that derived date runs PAST the last
		// regular payment date, DOS extends the schedule to it (the loan's residual
		// principal is retired by those trailing extras). Go previously extended
		// veryLast only for an EXPLICIT StopDate, so an NN-only series whose last
		// extra fell after the last payment date was cut one (or more) rows short,
		// leaving the balance unretired. Deriving the stop here mirrors DOS for both
		// arrears and in-advance loans.
		if pp.StopDateStatus < types.InOutDefault &&
			pp.NNStatus >= types.InOutDefault && pp.NN > 0 &&
			pp.PerYrStatus >= types.InOutDefault && pp.PerYr > 0 &&
			pp.StartDateStatus >= types.InOutDefault {
			derived := pp.StartDate
			startDay := pp.StartDate.Time.Day()
			ok := true
			for k := 1; k < pp.NN; k++ {
				nd, err := dateutil.AddPeriod(derived, pp.PerYr, startDay, false)
				if err != nil {
					ok = false
					break
				}
				derived = nd
			}
			if ok && dateutil.DateComp(derived, veryLast) > 0 {
				veryLast = derived
			}
		}
	}
	return veryLast
}

// veryLastRegularAmount mirrors AMORTOP.pas:1306-1320.
//
//	function VeryLastRegularAmount: real;
//	begin
//	  theresult := 0;
//	  for i := 1 to npre do
//	    if (DateComp(pre[i]^.stopdate, very_last) = 0) then theresult := pre[i]^.payment;
//	  if (theresult = 0) and (DateComp(h^.lastdate, very_last) = 0) then
//	    if (nadj > 0) then theresult := adj[nadj]^.amount
//	    else theresult := h^.payamt;
//	  VeryLastRegularAmount := theresult;
//	end;
//
// It answers "what regular money already lands on very_last?" — the amount the
// PlusReg convention subtracts out of a terminating balloon so the balloon
// stands ALONGSIDE the regular payment instead of including it.
func veryLastRegularAmount(loan *Loan, adjs []RateAdjustment, prepays []Prepayment, veryLast types.DateRec) float64 {
	res := 0.0
	for _, pp := range prepays {
		if pp.StopDateStatus >= types.InOutDefault &&
			dateutil.DateComp(pp.StopDate, veryLast) == 0 {
			res = pp.Payment
		}
	}
	if res == 0 && loan.LastOK && dateutil.DateComp(loan.LastDate, veryLast) == 0 {
		last := -1
		for i := range adjs {
			if adjs[i].DateStatus >= types.InOutDefault {
				last = i
			}
		}
		if last >= 0 {
			res = adjs[last].Amount
		} else {
			res = loan.PayAmt
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// TackOnFinalBalloon (Amortize.pas:1040-1088)
// ---------------------------------------------------------------------------

// tackOnResult carries the outcome of TackOnFinalBalloon.
type tackOnResult struct {
	// Fired is true when the gate opened AND a row was produced.
	Fired bool
	Date  types.DateRec
	// Amount is the terminating balloon DOS computes and paints into the
	// Balloon Payments grid with datestatus = amountstatus = outp.
	Amount float64
	// Live reports whether the row stays inside nballoons — i.e. whether it
	// participates in the payment table and the APR. DOS de-activates the row
	// with dec(nballoons) ONLY on the non-merge path AND only when the residual
	// moved the balloon by at least minpmt; below that threshold the row stays
	// live, and on the merge path it was a real user balloon all along.
	Live bool
	// MergeIdx >= 0 means the row OVERWROTE input.Balloons[MergeIdx] (DOS
	// merge_w_existing): very_last coincided with the last user balloon's date.
	MergeIdx int
	// Adjusted mirrors DOS's DA_TerminatingBalloonChanged message box, raised
	// only on the merge path when the recomputed amount actually moved.
	Adjusted bool
}

// tackOnGateOpen mirrors the caller gate at Amortize.pas:1386-1394:
//
//	if (fancy) and (h^.amountstatus >= defp) and (h^.loanratestatus >= defp) and
//	  (((nadj = 0) and (h^.payamtstatus >= defp)) or (adj_fully_specified))
//	  and (unkballoon = 0)
//	  and (nballoons < LineCount[AMZBalloonblock])
//	  then TackOnFinalBalloon;
//
// Note `>= defp` is DEFAULT-or-INPUT and therefore EXCLUDES a SOLVED (outp)
// value: DOS refuses to tack a terminating balloon onto a loan whose payment or
// amount it computed itself, because such a loan amortizes exactly by
// construction and the row would be meaningless.
func tackOnGateOpen(input *LoanInput) bool {
	if !input.Fancy {
		return false
	}
	l := &input.Loan
	if l.AmountStatus < types.InOutDefault || l.LoanRateStatus < types.InOutDefault {
		return false
	}
	if anyAdjRowPresent(input.Adjustments) {
		// adj_fully_specified (AMORTOP.pas:381-396): every present row needs its
		// date, rate AND amount.
		if adjRowsNotFullySpecified(input.Adjustments) {
			return false
		}
	} else if l.PayAmtStatus < types.InOutDefault {
		return false
	}
	// unkballoon = 0: no balloon row is itself awaiting a solve. A present row
	// with a date but no amount is DOS's unknown balloon.
	live := 0
	for _, b := range input.Balloons {
		present := b.DateStatus >= types.InOutDefault || b.AmountStatus >= types.InOutDefault
		if !present {
			continue
		}
		if b.AmountStatus < types.InOutDefault || b.DateStatus < types.InOutDefault {
			return false
		}
		live++
	}
	// nballoons < LineCount[AMZBalloonblock]: DOS needs a free grid row to hold
	// the tacked-on balloon.
	return live < maxBalloonRows
}

// maxBalloonRows is DOS `maxballoon` — the Balloon Payments grid row capacity.
// PETYPES.PAS:380 `maxballoon = maxlines`, and maxlines is 127 under the
// product's own build defines (SCROLLS, not NO_FRILLS, not CHEAP —
// legacy/src/win_source/Persense.cfg, mirrored by legacy/oracle/build_linux.sh).
// The tack-on row needs one free slot.
const maxBalloonRows = 127

// tackOnFinalBalloon ports Amortize.pas:1040-1088 verbatim:
//
//	procedure TackOnFinalBalloon;
//	begin
//	    {Computation is overspecified - compute last balloon}
//	  save_last := very_last;  save_lastok := h^.lastok;
//	  merge_w_existing := (nballoons > 0) and (dateok(very_last)) and
//	                      (DateComp(very_last, balloon[nballoons]^.date) = 0);
//	  if (merge_w_existing) then
//	    begin h^.lastok := true; oldamt := balloon[nballoons]^.amount; end
//	  else
//	    begin
//	      if (df.c.plus_regular) then oldamt := 0 else oldamt := h^.payamt;
//	      if (not dateok(very_last)) then DetermineVeryLast;
//	      inc(nballoons);
//	      balloon[nballoons]^.date := very_last;
//	      balloon[nballoons]^.datestatus := outp;
//	    end;
//	  unkballoon := nballoons;
//	  if EstimateAndRefineBalloon then
//	    begin
//	      if (abs(balloon[unkballoon]^.amount - oldamt) >= minpmt) then
//	        begin
//	          if (merge_w_existing) then
//	            MessageBox('Please note that the amount of your terminating balloon has been ajusted.', ...)
//	          else begin dec(nballoons); end;
//	        end;
//	    end;
//	  unkballoon := 0; very_last := save_last; h^.lastok := save_lastok;
//	        {This says, don't really use this last balloon in generating a table.}
//	end;
//
// The amount comes from EstimateAndRefineBalloon's very_last branch
// (Amortize.pas:628-645), which is a CLOSED FORM, not the secant iteration:
// pin the tack-on balloon to zero, run the UNFORCED full-term walk, and read
//
//	balloon^.amount := nextpayment.payamt + nextpayment.principal
//	if plus_regular then balloon^.amount := balloon^.amount - VeryLastRegularAmount
//
// `nextpayment` is the row that would fall on very_last. In Go the unforced
// generateFancyScheduleMode reproduces both halves exactly: its final schedule
// row's PayAmt is DOS's nextpayment.payamt (zero in a skipped month, the
// prepayment-inclusive amount otherwise) and its FinalPrinc is
// nextpayment.principal — the same pairing SolveBalloonAmount already relies on.
// Verified against the oracle's `bdump` on three shapes (skip / mid-term
// balloon / prepayment series) to the cent, 2026-07-24 §46.
func tackOnFinalBalloon(input LoanInput, settings *Settings) tackOnResult {
	var out tackOnResult
	out.MergeIdx = -1

	if !tackOnGateOpen(&input) {
		return out
	}

	// Index of the LAST live balloon — DOS balloon[nballoons]; the array is
	// kept sorted by date (Amortize.pas:273 SortBalloons).
	lastIdx := -1
	for i := range input.Balloons {
		if input.Balloons[i].DateStatus >= types.InOutDefault &&
			dateutil.DateOK(input.Balloons[i].Date) {
			lastIdx = i
		}
	}

	clone := input
	clone.Balloons = append([]BalloonPayment(nil), input.Balloons...)

	// The walk needs a bounded term. DOS keeps h^.lastok as it found it and lets
	// very_last carry the stop; the Go walk bounds itself by LastDate, so derive
	// it from FirstDate + NPeriods when the caller left it unresolved — the same
	// derivation SolveBalloonAmount uses (backward.go:851-861).
	if !clone.Loan.LastOK && clone.Loan.NPeriods > 0 && dateutil.DateOK(clone.Loan.FirstDate) {
		day := clone.Loan.FirstDate.Time.Day()
		last := clone.Loan.FirstDate
		for k := 1; k < clone.Loan.NPeriods; k++ {
			nd, err := dateutil.AddPeriod(last, clone.Loan.PerYr, day, false)
			if err != nil {
				return out
			}
			last = nd
		}
		clone.Loan.LastDate = last
		clone.Loan.LastOK = true
	}
	if !clone.Loan.LastOK || !dateutil.DateOK(clone.Loan.LastDate) {
		return out
	}

	veryLast := determineVeryLast(&clone.Loan, clone.Balloons, clone.Prepayments)
	if !dateutil.DateOK(veryLast) {
		return out
	}

	// merge_w_existing: very_last coincides with the last user balloon's date,
	// so DOS RE-SOLVES that row in place rather than appending a new one.
	oldAmt := 0.0
	if lastIdx >= 0 && dateutil.DateComp(veryLast, clone.Balloons[lastIdx].Date) == 0 {
		out.MergeIdx = lastIdx
		oldAmt = clone.Balloons[lastIdx].Amount
		// Pin the balloon under solve to zero for the probe walk
		// (Amortize.pas:631 `balloon[unkballoon]^.amount := 0`).
		clone.Balloons[lastIdx].Amount = 0
	} else if settings.PlusRegular {
		oldAmt = 0
	} else {
		oldAmt = clone.Loan.PayAmt
	}
	// On the NON-merge path DOS appends the row with amount 0, which is
	// numerically identical to not appending it — so the probe walk runs on the
	// balloon set as-is.

	s := *settings
	tr, _ := ComputeTrueRate(&clone.Loan, &s)
	fg := GrowthPerPeriod(&clone.Loan, s.YrInv)
	// This is DOS's RepayFancyLoan(..., nil, false, entire, no_value_calc, 0)
	// at Amortize.pas:637 -- the one call site with Output=nil, entire=TRUE and
	// value_calc=FALSE, so the residual fold at AMORTOP.pas:1209-1213 is live.
	// See LoanInput.entireWalk for what that changes (Re_Amortize restarts from
	// the folded principal).
	clone.entireWalk = true
	res := generateFancyScheduleMode(clone, clone.Loan.PayAmt, &s, tr, fg, true)
	if res.Err != nil || len(res.Schedule) == 0 {
		return out
	}

	amt := res.Schedule[len(res.Schedule)-1].PayAmt + res.FinalPrinc
	if settings.PlusRegular {
		amt -= veryLastRegularAmount(&clone.Loan, clone.Adjustments, clone.Prepayments, veryLast)
	}

	out.Fired = true
	out.Date = veryLast
	out.Amount = amt

	moved := amt-oldAmt >= minPmt || oldAmt-amt >= minPmt
	switch {
	case out.MergeIdx >= 0:
		// DOS never decrements on the merge path: the row was and remains a real
		// balloon, it just carries a recomputed amount.
		out.Live = true
		out.Adjusted = moved
	case moved:
		// dec(nballoons) — "don't really use this last balloon in generating a
		// table." Display only: excluded from the payment table AND the APR.
		out.Live = false
	default:
		// Residual under minpmt ($1.00): DOS leaves nballoons incremented, so the
		// row stays live and participates in the table.
		out.Live = true
	}
	return out
}
