package amortization

// dosport_walk.go — the schedule walk, the Newton solver, and the adjustment
// re-amortization for the faithful DOS port (continuation of dosport.go):
// RepayFancyLoan (AMORTOP.pas:1101-1237), Iterate (:1415-1497), Re_Amortize
// (:1499-1613), plus the EstimateAndRefinePayment seed (Amortize.pas:377-430) and
// the AmortizeDOS entry. Scope: the ordinary engine (360/365 basis, balloons,
// ARMs, moratorium, target, skip, prepayments, US-rule) — the space the fuzzer
// exercises. In-advance and Rule-of-78 fall back to the production engine for now
// (documented TODOs); they are bounded corners the fuzzer does not generate.

import (
	"math"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// dpSavedState mirrors saved_balloon_state (AMORTOP.pas:30-37): the running
// counters Iterate must protect across each trial RepayFancyLoan walk.
type dpSavedState struct {
	nextBalloon, npre, nextAdj int
	d                          float64
	pres                       []dpPrepay
	// The inner RepayFancyLoan of an Iterate trial overwrites the engine's
	// payment/nextPayment working records AND the old* save-for-reAmortize fields
	// (its per-period saveDataForReAmortize clobbers them); save them too so a
	// reAmortize that runs an inner Iterate does not corrupt the OUTER walk it is
	// embedded in. Without restoring oldNextBalloon, the outer reAmortize's
	// end-block `nextBalloon = oldNextBalloon` would adopt the inner walk's final
	// counter (balloon already consumed), making a SECOND ARM miss the balloon.
	payment, nextPayment    dpPayment
	oldNextBalloon, oldNpre int
	oldPre                  []dpPrepay
}

func (e *dosEng) saveState() dpSavedState {
	s := dpSavedState{nextBalloon: e.nextBalloon, npre: e.npre, nextAdj: e.nextAdj, d: e.d,
		payment: e.payment, nextPayment: e.nextPayment,
		oldNextBalloon: e.oldNextBalloon, oldNpre: e.oldNpre}
	s.pres = make([]dpPrepay, len(e.pres))
	copy(s.pres, e.pres)
	s.oldPre = make([]dpPrepay, len(e.oldPre))
	copy(s.oldPre, e.oldPre)
	return s
}

func (e *dosEng) restoreState(s dpSavedState) {
	e.nextBalloon, e.npre, e.nextAdj, e.d = s.nextBalloon, s.npre, s.nextAdj, s.d
	e.payment, e.nextPayment = s.payment, s.nextPayment
	e.oldNextBalloon, e.oldNpre = s.oldNextBalloon, s.oldNpre
	copy(e.pres, s.pres)
	e.oldPre = make([]dpPrepay, len(s.oldPre))
	copy(e.oldPre, s.oldPre)
}

// saveDataForReAmortize mirrors SaveDataForReAmortize (AMORTOP.pas:1079-1089).
func (e *dosEng) saveDataForReAmortize() {
	e.oldNpre = e.npre
	e.oldNextBalloon = e.nextBalloon
	e.oldPre = make([]dpPrepay, len(e.pres))
	copy(e.oldPre, e.pres)
}

// repayFancyLoan mirrors RepayFancyLoan (AMORTOP.pas:1101-1237). It walks the
// schedule from firstdate, advancing the balance through p. With collect=true it
// returns the per-payment rows and (when entire) folds the residual into the
// final payment; with collect=false (the Iterate trial) it leaves the terminal
// principal UNFORCED so Newton can drive it to zero. Re_Amortize runs at each
// adjustment only when (next_adj<=adjnum) or entire.
func (e *dosEng) repayFancyLoan(p, usap *float64, loandate, firstdate types.DateRec,
	collect, entire bool, adjnum int) []dpPayment {

	// WhenToStop selection (AMORTOP.pas:1130-1133).
	usesPayment := collect || (adjnum > 0)
	// stopdate (AMORTOP.pas:1139-1147).
	if adjnum > 0 {
		e.stopdate = e.adjs[adjnum].date
	} else {
		e.stopdate = e.veryLast
	}

	// t := firstdate - 1 period (the first base_date).
	t, _ := dateutil.AddPeriod(firstdate, e.loan.PerYr, firstdate.Time.Day(), true)
	// prevdate (paidthru): the date from which the FIRST period's interest accrues.
	//   - non-prepaid: loanDate — the first row spans the actual [loanDate, firstDate]
	//     stub (short OR long), matching DOS.
	//   - prepaid (non-in-advance): max(loanDate, firstDate-1period). On a SHORT/clean
	//     stub firstDate-1period ≤ loanDate, so paidthru=loanDate and the first row is
	//     the actual sub-period stub (e.g. annual loan, 7-month first, prepaid: rate·7/12).
	//     On a LONG stub firstDate-1period > loanDate, so paidthru=firstDate-1period and
	//     the first row is capped at ONE period (rate·1), with the excess
	//     [loanDate, firstDate-1period] collected as CLOSING prepaid interest, not in the
	//     schedule (verified vs oracle: prepaid==non-prepaid for short/clean, but a long
	//     first period differs — prepaid caps the first row at one period).
	//   - in-advance: its own model; not routed through this port.
	paidthru := loandate
	if e.set.Prepaid && !e.set.InAdvance {
		if fp1, err := dateutil.AddPeriod(firstdate, e.loan.PerYr, firstdate.Time.Day(), true); err == nil &&
			dateutil.DateComp(fp1, loandate) > 0 {
			paidthru = fp1
		}
	} else if e.set.Prepaid && e.set.InAdvance {
		paidthru = firstdate
	}

	if entire {
		e.nextBalloon = 1
	}
	e.abort = false
	e.nextPayment.init(t, paidthru)
	e.computeNext(&e.nextPayment, p, usap)

	var rows []dpPayment
	for {
		e.payment = e.nextPayment
		e.saveDataForReAmortize()
		e.computeNext(&e.nextPayment, p, usap)

		// final-fold (AMORTOP.pas:1208-1212): only when (not lastok) or entire.
		whenToStop := &e.nextPayment
		if usesPayment {
			whenToStop = &e.payment
		}
		if (!e.loan.LastOK || entire) && whenToStop.principal < minPmt {
			whenToStop.payamt += whenToStop.principal
			whenToStop.principal = 0
		}

		if collect {
			// PrintAndReset (AMORTOP.pas:1004-1009): the payment landing ON very_last
			// absorbs the ENTIRE remaining principal — regardless of residual size —
			// so an ARM/skip schedule that did not amortize cleanly retires with a
			// ballooned final row. This is in the BUILD path only (not the Iterate
			// solve), which is why interest matches DOS but the balance would not
			// retire without it.
			if dateutil.DateComp(e.payment.date, e.veryLast) == 0 {
				e.payment.payamt += e.payment.principal
				e.payment.principal = 0
			}
			rows = append(rows, e.payment)
			// DecideWhetherToPrintALine itself calls Re_Amortize (AMORTOP.pas:1075):
			// the printed/built schedule re-amortizes at each adjustment whenever the
			// next payment date has passed it.
			if e.nextAdj <= e.nadj &&
				dateutil.DateComp(e.nextPayment.date, e.adjs[e.nextAdj].date) > 0 {
				e.reAmortize(p)
			}
		} else if (e.nextAdj <= adjnum || entire) && e.nextAdj <= e.nadj &&
			dateutil.DateComp(e.nextPayment.date, e.adjs[e.nextAdj].date) > 0 {
			e.reAmortize(p)
		}

		// termination (AMORTOP.pas:1219-1221).
		stop := false
		if (!e.loan.LastOK || collect) && whenToStop.principal == 0 {
			stop = true
		}
		if dateutil.DateComp(whenToStop.date, e.stopdate) >= 0 {
			stop = true
		}
		if e.abort {
			stop = true
		}
		if stop {
			break
		}
		if len(rows) > 5000 { // safety bound
			break
		}
	}
	return rows
}

// iterate mirrors Iterate (AMORTOP.pas:1415-1497): a finite-difference Newton
// refinement of the scalar *x (payment, rate, or amount) so the schedule's
// terminal principal lands on zero, using RepayFancyLoan as the residual.
func (e *dosEng) iterate(p0, usap0 float64, loandate, firstdate types.DateRec,
	x *float64, entire bool, targetIsAmount bool) bool {

	const halfpenny = 0.005
	const accLimit = 2e-8
	saved := e.saveState()

	p, usap := p0, usap0
	e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
	e.repayFancyLoan(&p, &usap, loandate, firstdate, false, entire, 0)
	e.restoreState(saved)
	if math.Abs(p) < halfpenny {
		return true
	}
	final := p
	delta := small * *x
	count := 0
	*x += delta
	bestp := math.MaxFloat64
	bestx := *x

	for {
		e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
		count++
		if targetIsAmount {
			p = *x
		} else {
			p = p0
		}
		usap = usap0
		savex := *x
		e.repayFancyLoan(&p, &usap, loandate, firstdate, false, entire, 0)
		e.restoreState(saved)
		*x = savex
		var newdelta float64
		if math.Abs(final-p) > teeny {
			newdelta = delta * p / (final - p)
		}
		if math.Abs(delta) < teeny || math.Abs(newdelta/delta) > 1 {
			count += 5
		}
		delta = newdelta
		*x += delta
		final = p
		if math.Abs(p) < bestp {
			bestp = math.Abs(p)
			bestx = *x
		}
		if count >= 20 || bestp < halfpenny || math.Abs(e.loan.LoanRate) > 2 {
			break
		}
	}
	*x = bestx
	if bestp > halfpenny && bestp > accLimit*p0 {
		return false // did not converge
	}
	return true
}

// reAmortize mirrors Re_Amortize (AMORTOP.pas:1499-1613) for the rate-only / AO7
// path: at adj[next_adj], adopt the new rate, compute the analytic segment
// payment over [adj → last] netting discounted future balloons, then refine it
// with Iterate(til_adj) when balloons/prepayments remain. Advances next_adj.
func (e *dosEng) reAmortize(p *float64) {
	*p = e.payment.principal
	usap := e.payment.usap
	adj := &e.adjs[e.nextAdj]
	if adj.rateOK {
		e.loan.LoanRate = adj.loanrate
		e.truerate, _ = ComputeTrueRate(&e.loan, &e.set)
	}
	if adj.amtok {
		e.d = adj.amount
		e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
	} else {
		// compute a new payment amount.
		n, _ := dateutil.NumberOfInstallments(adj.date, e.loan.LastDate, e.loan.PerYr, types.OnOrAfter)
		e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
		adjp := *p
		e.nextBalloon = e.oldNextBalloon
		if e.oldNpre > 0 {
			e.npre = e.oldNpre
			copy(e.pres, e.oldPre)
		} else {
			e.npre = 0
		}
		rate, _ := interest.RateFromYield(e.loan.LoanRate, byte(e.loan.PerYr), e.set.YrDays)
		for i := e.nextBalloon; i <= e.userNballoons; i++ {
			yd := dateutil.YearsDif(e.balloons[i].date, adj.date, e.set.Basis, e.set.YrInv, false)
			disc, _ := interest.Exxp(-rate * yd)
			adjp -= e.balloons[i].amount * disc
		}
		denom := 1 - powF(e.f, -(n-1))
		if math.Abs(denom) < teeny {
			e.d = adjp / float64(n-1)
		} else {
			e.d = adjp * (e.f - 1) / denom
		}
		// Iterate(til_adj) only when balloons/prepayments remain (AMORTOP.pas:1571).
		if e.userNballoons > 0 || e.npre > 0 {
			saveN := e.nballoons
			e.nballoons = e.userNballoons
			t := e.nextPayment.date
			// Iterate on e.d DIRECTLY (DOS passes the global `d` by reference,
			// AMORTOP.pas:1577) — the inner walk's payment IS e.d, so the Newton
			// must move e.d itself, not a copy. Passing a copy here left the walk
			// using the unrefined seed, so the terminal never moved and Newton
			// diverged → abort at the first ARM on balloon×skip stacks.
			if e.iterate(*p, usap, e.payment.date, t, &e.d, false, false) {
				adj.amount = e.d
				adj.amtok = true
			} else {
				e.abort = true
				e.errorflag = true
			}
			e.nballoons = saveN
		}
		adj.amount = e.d
	}

	// Restore counters, step back one payment, recompute NextPayment, inc next_adj.
	e.nextBalloon = e.oldNextBalloon
	if e.oldNpre > 0 {
		copy(e.pres, e.oldPre)
		e.npre = e.oldNpre
	}
	e.nextPayment = e.payment
	e.computeNext(&e.nextPayment, p, &usap)
	e.nextAdj++
	*p = e.nextPayment.principal
}

// powF returns f^n for integer n (avoids exxp/lnn overflow guards for the seed).
func powF(f float64, n int) float64 { return math.Pow(f, float64(n)) }
