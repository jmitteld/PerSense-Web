package amortization

// runAdjustmentPrePassDOS ports Amortize.pas:1408-1419 — the loop that calls
// EstimateAndRefineAdjPayment once per one-sided (rate given, amount blank)
// adjustment BEFORE the display walk.
//
// See the long comment at the call site in dosport_entry.go for why this matters
// and what it is allowed to change. In one line: the pre-pass exists to leave
// h^.lastdate month-end-snapped, and nothing else about it survives.
//
// EstimateAndRefineAdjPayment (Amortize.pas:324-345) is:
//
//	adj_save_balloon.Save;
//	save_rate := h^.loanrate;
//	p := h^.amount;  usap := 0;  f := GrowthPerPeriod;
//	RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, til_adj, no_value_calc, adjnum);
//	p := h^.amount;  usap := 0;
//	adj_save_balloon.Restore;
//	h^.loanrate := save_rate;
//	ComputeTrueRate;
//
// Note what is NOT restored: h^.lastdate. That omission is the whole mechanism.
// `saved_balloon_state` (AMORTOP.pas:132-146) saves next_balloon, npre, next_adj,
// d and pre[] — not lastdate.
func (e *dosEng) runAdjustmentPrePassDOS(basePay float64) {
	if e.nadj == 0 {
		return
	}
	for i := 1; i <= e.nadj; i++ {
		a := &e.adjs[i]
		// DOS's gate: `(adj[i]^.loanratestatus >= defp) and (adj[i]^.amountstatus < defp)`
		// — a rate-only adjustment, i.e. exactly the ones whose payment
		// Re_Amortize has to solve.
		if !a.rateOK || a.amtok {
			continue
		}

		// --- EstimateAndRefineAdjPayment: save ---
		saveRate := e.loan.LoanRate
		saveNextBalloon, saveNpre, saveNextAdj, saveD := e.nextBalloon, e.npre, e.nextAdj, e.d
		savePre := make([]dpPrepay, len(e.pres))
		copy(savePre, e.pres)
		saveOldNextBalloon, saveOldNpre := e.oldNextBalloon, e.oldNpre
		saveOldPre := make([]dpPrepay, len(e.oldPre))
		copy(saveOldPre, e.oldPre)
		saveSnap, saveSubFirstDay := e.reAmortLastSnap, e.subFirstDay

		p := e.loan.Amount
		usap := 0.0
		e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)

		// collect=false, entire=false, adjnum=i  ->  RepayFancyLoan(..., nil, false, til_adj, ..., adjnum)
		e.repayFancyLoan(&p, &usap, e.loan.LoanDate, e.loan.FirstDate, false, false, i)

		// --- restore (everything except LastDate) ---
		e.nextBalloon, e.npre, e.nextAdj = saveNextBalloon, saveNpre, saveNextAdj
		copy(e.pres, savePre)
		e.oldNextBalloon, e.oldNpre = saveOldNextBalloon, saveOldNpre
		copy(e.oldPre, saveOldPre)
		e.reAmortLastSnap, e.subFirstDay = saveSnap, saveSubFirstDay
		e.loan.LoanRate = saveRate
		e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)

		// Amortize.pas:1414 `d := h^.payamt` — the walking payment goes back to the
		// BASE payment for the next iteration and for the display walk. DOS does
		// not carry the adjusted payment out of the pre-pass.
		e.d = basePay
		_ = saveD

		// DOS aborts the whole computation if the pre-pass fails
		// (`if (not EstimateAndRefineAdjPayment(i)) then exit`, Amortize.pas:1411-1412).
		// e.abort / e.errorflag are already latched by the walk in that case; leave
		// them set and stop, exactly as DOS does.
		if e.errorflag || e.abort {
			return
		}
	}
	// The pre-pass must not leave the walk state mid-schedule.
	e.abort = false
}
