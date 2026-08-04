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
	"fmt"
	"math"
	"os"
	"time"

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
	//
	// AMORTOP.pas:1149-1150 spells the origin day as `firstdate.d` — the day field
	// of the LOCAL parameter, not h^.firstdate.d. On the top-level walk the two
	// coincide; on Re_Amortize's sub-walk `firstdate` is a snapped date that DOS
	// may have left as a PHANTOM (day past the end of the month). e.subFirstDay
	// carries that raw day when the caller had to clamp to build a DateRec; see
	// the field's comment in dosport.go.
	firstDayRaw := firstdate.Time.Day()
	if e.subFirstDay > 0 {
		firstDayRaw = e.subFirstDay
	}
	t, _ := dateutil.AddPeriod(firstdate, e.loan.PerYr, firstDayRaw, true)
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
		if fp1, err := dateutil.AddPeriod(firstdate, e.loan.PerYr, firstDayRaw, true); err == nil &&
			dateutil.DateComp(fp1, loandate) > 0 {
			paidthru = fp1
		}
	} else if e.set.Prepaid && e.set.InAdvance {
		paidthru = firstdate
	}

	// AMORTOP.pas:1129 — `saverate := h^.loanrate`, taken BEFORE the walk so the
	// epilogue at :1233 can undo whatever Re_Amortize did to the live rate.
	saveRate := e.loan.LoanRate

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
				e.reAmortize(p, usap)
			}
		} else if (e.nextAdj <= adjnum || entire) && e.nextAdj <= e.nadj &&
			dateutil.DateComp(e.nextPayment.date, e.adjs[e.nextAdj].date) > 0 {
			// Non-collect (Iterate trial) walk: re-amortize at a pending adjustment
			// only when this trial is permitted to cross it — either the adjustment is
			// at/before the segment boundary being solved (next_adj <= adjnum) or this
			// is the full-schedule (entire) solve — AND there is a pending adjustment
			// (next_adj <= nadj) whose date the next payment has passed. A bounded
			// segment solve (adjnum>0) deliberately stops re-amortizing past its own
			// boundary so it isolates just that segment's terminal.
			e.reAmortize(p, usap)
		}

		// termination (AMORTOP.pas:1219-1221).
		stop := false
		// Stop on a zeroed balance only when the schedule is ALLOWED to end on
		// payoff: either the last date is unknown (payoff is the only terminator) or
		// we are building the real schedule (collect). During a bounded Iterate trial
		// with a known last date we keep walking to stopdate so the UNFORCED terminal
		// residual stays observable for Newton — see repayFancyLoan's collect vs. trial
		// contract.
		if (!e.loan.LastOK || collect) && whenToStop.principal == 0 {
			stop = true
		}
		// Always stop once the walk reaches the segment/schedule end date.
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

	// ---- RepayFancyLoan's epilogue (AMORTOP.pas:1233-1234) ----
	//
	//	h^.loanrate := saverate;
	//	ComputeTrueRate;
	//
	// The port had neither line. Restoring the rate is the smaller half; the
	// consequential half is that DOS re-derives the true rate here, on EVERY
	// RepayFancyLoan — including the throwaway trial walks Iterate runs — and
	// ComputeTrueRate ends in
	//
	//	truerate := RateFromYield(ReportedRate(h^.loanrate), df.c.peryr)
	//	RateFromYield(yy,n) = nn*lnn(1 + yy/nn)          (INTSUTIL.pas:1270-1275)
	//
	// so a trial rate at or below -nn drives lnn's argument non-positive. lnn then
	// raises "Error: The data you have specified contain an inconsistency." and
	// sets BOTH errorflag and overflowflag (INTSUTIL.pas:1164-1171). Neither is
	// cleared before the screen ends, so DOS refuses the screen outright: MakeTable
	// runs `Enter(no_tab); if (errorflag) then exit;` (Amortize.pas:1457-1458) and
	// draws no table at all.
	//
	// Iterate's own exit test is `abs(h^.loanrate) > 2` (AMORTOP.pas:1485), so on an
	// ANNUAL loan (nn=1) the secant is explicitly allowed to roam to -2 — well past
	// the -1 where lnn dies. This is not an exotic excursion; it is the ordinary
	// width of the rate search.
	//
	// 2026-07-26 fuzzer5 cycle 21 seed 20303 — verified against the real DOS engine
	// with an FPC stack dump (Dump_Stack in the oracle's MessageBox):
	//
	//	amort_oracle 281995.17 0.0383350000 15 1 b365_360 r78 usa \
	//	  loandmy=8.2.2023 firstdmy=8.2.2024 mor=36 pre=12:74:12:500.60 \
	//	  adj=144::21176.82 targ=2951.87 pts=0.019302
	//	  DOS:  ERR Error: The data you have specified contain an inconsistency.
	//	        HELPCODE $02040006 = DO_LnnNegative (HelpSystemUnit.pas:179)
	//	        LNN <- RATEFROMYIELD <- COMPUTETRUERATE(amortop:1265)
	//	           <- REPAYFANCYLOAN(amortop:1234) <- ITERATE(amortop:1464)
	//	           <- RE_AMORTIZE(amortop:1522) <- REPAYFANCYLOAN(amortop:1215)
	//	           <- ESTIMATEANDREFINEADJRATE(amortize:360)
	//	  port: int=-897262.99 paid=-615267.82 rows=83
	//
	// `adj=144::21176.82` is a new-payment-with-no-new-rate adjustment, so
	// Re_Amortize's AO6 arm solves the IMPLIED rate; the secant wandered below
	// -100%, DOS died there, and the port — swallowing every ComputeTrueRate error
	// with `_` — carried on from a zeroed true rate and returned 83 rows of
	// nonsense (negative interest, negative total paid) for a screen the real
	// engine refuses. Dropping any one of the prepayment series, the adjustment,
	// the target or either date makes DOS accept, and the two then agree exactly.
	//
	// This is the same defect solveSegmentRate was fixed for on 2026-07-25 (seed
	// 8900, fancybisect.go:1131-1152) — the production bisect route already
	// propagates the lnn error. This is the structural-port route.
	e.loan.LoanRate = saveRate
	if tr, err := ComputeTrueRate(&e.loan, &e.set); err != nil {
		e.errorflag = true
		e.overflowflag = true
	} else {
		e.truerate = tr
	}

	return rows
}

// iterate mirrors Iterate (AMORTOP.pas:1415-1497): a finite-difference Newton
// refinement of the scalar *x (payment, rate, or amount) so the schedule's
// terminal principal lands on zero, using RepayFancyLoan as the residual.
// dosFancy mirrors the Pascal module-global `fancy` flag: true iff any Advanced
// Option (balloon / prepayment / adjustment / moratorium / target / skip) is
// present. In DOS this is the Advanced-Options UI toggle
// (AmortizationScreenUnit.pas: ToggleAdvanced); it is NOT set by USA / exact /
// basis. Both DOS's Iterate terminal (AMORTOP.pas:1438/1464) and its schedule
// dispatch (Amortize.pas:1493) key on it.
func (e *dosEng) dosFancy() bool {
	if e.nballoons > 0 || e.npre > 0 || e.nadj > 0 || e.morPresent || e.targValue != 0 {
		return true
	}
	for i := range e.skipSet {
		if e.skipSet[i] {
			return true
		}
	}
	return false
}

// iterateTerminal chooses the Newton terminal exactly as DOS's Iterate does
// (AMORTOP.pas:1438/1464): the usap-aware period-by-period RepayFancyLoan when
// `fancy OR (exact AND basis<>x360)`, else the closed-form RepayLoan recursion.
// USA-rule is deliberately NOT a trigger — DOS solves a plain USA loan's payment
// with RepayLoan (usap has no effect on the solved number); the usap-aware walk is
// used only to RENDER the schedule. The old port always used RepayFancyLoan here,
// so a USA-arrears non-360 loan solved its payment against the whole-month display
// walk instead of RepayLoan's actual-day annuity — the residual round-trip miss.
// The schedule DISPLAY (built separately by AmortizeDOS) is unaffected.
func (e *dosEng) iterateTerminal(p, usap *float64, loandate, firstdate types.DateRec, entire bool) {
	if e.dosFancy() || (e.set.Exact && e.set.Basis != types.Basis360) {
		e.repayFancyLoan(p, usap, loandate, firstdate, false, entire, 0)
		return
	}
	// Closed-form simple recursion. RepayLoan reads the term/dates/rate off e.loan
	// and the payment from e.d; it is basis-actual-day-prorate for !prepaid
	// (matching DOS Amortize.pas:1286) so forward and backward solves round-trip.
	*p = RepayLoan(*p, e.d, &e.loan, &e.set, e.set.YrInv)
}

// dpTrace mirrors the DOS-side Iterate tracer (scripts/build_trace_oracle.sh):
// set DPTRACE=1 to dump this port's secant trajectory to stderr in the same
// GITR0/GITR/GITRend shape the oracle emits as ITR0/ITR/ITRend, so the two can be
// diffed step-for-step. Read once — this sits inside the Newton loop.
var dpTrace = os.Getenv("DPTRACE") != ""

// dpTraceAV is the same idea one level down: DPTRACEAV=1 dumps every discounted
// term of the APR value stream (backward.go's ComputeAPRWithPoints) as GAV lines,
// matching the oracle's AV lines from `build_trace_oracle.sh -mode aprv`. Use it
// when the secant trajectories diverge from the very first evaluation, which
// means the two engines are discounting different cashflows.
var dpTraceAV = os.Getenv("DPTRACEAV") != ""

// dpTraceSeg dumps the segment-payment terminal on a grid (SEGTERM lines) so the
// shape of the root being solved at an adjustment boundary can be inspected.
var dpTraceSeg = os.Getenv("DPTRACESEG") != ""

// dpTraceSegRows dumps every ROW of the segment-RATE terminal (SEGROW lines),
// the counterpart to the DOS trace oracle's `-mode cn` CN lines. §66 (round 25):
// DOS's Iterate and the port's dosIterateCore agree step-for-step on c4 and
// still return different roots, because their TERMINALS differ at the shared
// seed — so the question is which row of the sub-walk first disagrees, not what
// the solver did. Extremely verbose; use with a single case.
var dpTraceSegRows = os.Getenv("DPTRACESEGROWS") != ""

func (e *dosEng) iterate(p0, usap0 float64, loandate, firstdate types.DateRec,
	x *float64, entire bool, targetIsAmount bool) bool {

	const halfpenny = 0.005
	const accLimit = 2e-8
	// AMORTOP.pas:1421 declares Iterate's OWN `small = 0.001`, which SHADOWS the
	// unit-global `small = 1E-4` (PETYPES.PAS:147). The port had been using the
	// package-level `small` (= types.Small = 1e-4), making the secant's first
	// probe step ten times narrower than DOS's.
	//
	// This is not a precision nicety — it selects the ROOT. The probe step sets
	// the first difference quotient, which sets the first Newton jump, which sets
	// which basin the whole trajectory falls into; and the fancy payment-solve
	// objective is genuinely multi-rooted once skips and a principal-reduction
	// target are in play (ComputeNext's balloonpos=0 arm subtracts `d`
	// unconditionally — AMORTOP.pas:614-621 — so a skip+prepayment row's principal
	// moves OPPOSITE to `d`).
	//
	// 2026-07-25 fuzzer5 seed 8922 — verified against the real DOS engine:
	//
	//	amort_oracle 245695.48 0.1158040000 120 12 plusreg b24=14683.43 \
	//	  b56=13819.97 pre=47:144:24:59.23 pre=2:97:12:179.81 targ=827.44 skip=2,8,11
	//
	// Both engines seed at d = 2974.1496 (adjp 210849.1453 over nrepay 120) and
	// both walks are row-for-row identical at any GIVEN payment. With small=1e-4
	// the port probed 2974.4470, read a local slope of -804/unit and stepped to
	// 5246 — the RIGHT root, 4773.4528. DOS probes 2977.1237, reads the true
	// slope of -80.5/unit, overshoots to ~25700 and the secant then swings left
	// onto -4492.4752, which is the answer the DOS engine actually prints
	// (int 139811.59, paid 385507.07).
	const small = 0.001
	// DOS Iterate disables hard_payment for the duration of the Newton iteration
	// (AMORTOP.pas:1433) and restores it after (:1496): the trial schedules must
	// NOT round per-period interest to cents, or the terminal is noisy and the
	// secant diverges. Without this, a GIVEN (hard) payment whose ARM re-amortizes
	// solved a wrong segment payment and the loan failed to retire.
	saveHard := e.hardPay
	e.hardPay = false
	defer func() { e.hardPay = saveHard }()
	saved := e.saveState()

	p, usap := p0, usap0
	e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
	// AMORTOP.pas:1439 vs :1465 — the PRE-LOOP terminal evaluation is hard-coded to
	// `til_adj` (false, AMORTOP.pas:21) while the in-loop trials pass
	// `entire_or_no` through. DOS is asymmetric here on purpose or by accident, but
	// it is observable: this walk supplies `final`, the secant's first baseline, so
	// on an `entire` solve DOS measures its first difference quotient between a
	// til_adj terminal and an entire terminal, where passing `entire` to both (as
	// the port did) makes the first step a true two-point slope.
	//
	// FIDELITY-ONLY, NO OBSERVED REPRO. Every case tested so far reaches `iterate`
	// with entire=false, where the two spellings coincide, so this change is a
	// no-op on all of them — it is here because it is what the Pascal says, not
	// because it fixed a divergence. (An earlier revision of this comment blamed
	// it for a 0.18 gap on the `495178.90 … adj=70 skip=2,8,11 b126=114024.00`
	// segment payment; that gap was really the production engine's segment solve
	// dropping the live `usap` — see LoanInput.initUsap. Testing disproved the
	// attribution; the change is kept, unvalidated, on textual grounds.)
	e.iterateTerminal(&p, &usap, loandate, firstdate, false)
	e.restoreState(saved)
	if math.Abs(p) < halfpenny {
		return true
	}
	if dpTrace {
		fmt.Fprintf(os.Stderr, "GITR0 seedx=%.10f p=%.10f\n", *x, p)
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
		// AMORTOP.pas:1453-1454:
		//
		//	if (overflowflag) then
		//	  goto 1;
		//
		// `goto 1` lands past `x := bestx` AND past the convergence verdict, so DOS
		// adopts NO value for the target and emits no "did not converge" dialog — the
		// screen is already condemned by the errorflag the same guard set. Mirror
		// both halves: leave *x on its last trial value and report failure. See
		// repayFancyLoan's epilogue for how the flag gets set.
		if e.overflowflag {
			return false
		}
		if targetIsAmount {
			p = *x
		} else {
			p = p0
		}
		usap = usap0
		savex := *x
		e.iterateTerminal(&p, &usap, loandate, firstdate, entire)
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
		if dpTrace {
			fmt.Fprintf(os.Stderr, "GITR n=%d p=%.10f delta=%.10f newx=%.10f\n", count, p, delta, *x)
		}
		if math.Abs(p) < bestp {
			bestp = math.Abs(p)
			bestx = *x
		}
		if count >= 20 || bestp < halfpenny || math.Abs(e.loan.LoanRate) > 2 {
			break
		}
	}
	if dpTrace {
		fmt.Fprintf(os.Stderr, "GITRend bestp=%.10f bestx=%.10f count=%d\n", bestp, bestx, count)
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
// USA-RULE FIDELITY (AMORTOP.pas:1507-1508, :1610): DOS's Re_Amortize takes only
// `p` as a var param; it reads and writes the UNIT-LEVEL global `usap`
// (AMORTOP.pas:73). Every RepayFancyLoan call site passes that same global as the
// `usapart` var param (AMORTOP.pas:1344, :1439, :1465, Amortize.pas:337, :360,
// :554, :573, :637, :1115, :1161, :1495, APRReportScreenUnit.pas:163), so
// `usapart` is an ALIAS for the global. Re_Amortize therefore does two things to
// the caller's accumulator, not one:
//
//	rewind:  usap := Payment.usaprinc      (undo the ComputeNext that overshot the adj)
//	advance: NextPayment.ComputeNext(p, usap)   (redo that row at the NEW rate/payment)
//
// and the advanced value is what the walk's next row reads. Taking `usap` by
// value here (as this function did) got the rewind right and silently dropped the
// write-back, so the walk resumed with the OLD-payment row's accumulator.
//
// Traced on 495178.90 0.1032190000 216 12 usa adj=70:0.0369020000: skip=2,8,11
// (ComputeNext tracer, scripts/build_trace_oracle.sh -mode cn):
//
//	CN d=129-12-1 p=422408.983823 usap=3602.399734 int=3602.399734 pay=6737.082885 uout=467.716582   <- pre-adj row
//	CN d=129-12-1 p=422408.983823 usap=3602.399734 int=1287.900047 pay=3595.511828 uout=1294.787952  <- Re_Amortize redo
//	CN d=130-1-1  p=420101.372042 usap=1294.787952 int=1287.900047                                   <- walk resumes on 1294.79
//
// Go resumed on 467.716582, inflating the interest base by 827.07 and charging
// 1290.44 instead of 1287.90 — a 2.54 gap that persisted in every later balance
// (DOS totalInt 413283.23 vs Go 413287.19).
func (e *dosEng) reAmortize(p, usapp *float64) {
	*p = e.payment.principal
	*usapp = e.payment.usap
	usap := *usapp
	adj := &e.adjs[e.nextAdj]
	if adj.rateOK {
		e.loan.LoanRate = adj.loanrate
		e.truerate, _ = ComputeTrueRate(&e.loan, &e.set)
	}
	if adj.amtok {
		e.d = adj.amount
		if !adj.rateOK {
			// AO6 (EstimateAndRefineAdjRate, AMORTOP.pas:1521-1535): a new payment
			// with NO new rate — solve the implied RATE at which that payment
			// amortizes the balance over the remaining term (the mirror of AO5).
			// DOS calls the generic Iterate with x aliasing h^.loanrate; Iterate
			// recomputes f from the mutated rate each step (AMORTOP.pas:1455) and
			// the 360-basis walk reads the rate directly for per-period interest.
			if e.iterate(*p, usap, e.payment.date, e.nextPayment.date, &e.loan.LoanRate, false, false) {
				adj.loanrate = e.loan.LoanRate
				adj.rateOK = true
				e.truerate, _ = ComputeTrueRate(&e.loan, &e.set)
				e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
			} else {
				e.abort = true
				e.errorflag = true
			}
		} else {
			e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
		}
	} else {
		// compute a new payment amount.
		//
		// AMORTOP.pas:1547 —
		//
		//	n := NumberOfInstallments(adj[next_adj]^.date, h^.lastdate,
		//	                          h^.peryr, on_or_after);
		//
		// `NumberOfInstallments(var f, l : daterec; ...)` takes `l` BY REFERENCE
		// and documents that it "adjusts l to be exactly on a payment day"
		// (INTSUTIL.pas:936-1021). Its monthly branch ends
		//
		//	if (flast) then l.d := daysinm(l) else l.d := f.d;
		//
		// with `flast := LastDayFn(f, peryr)` — true iff the START date (here the
		// ADJUSTMENT date) is the last day of its own month. This call site has NO
		// save/restore guard, unlike Amortize.pas:1301-1304 which brackets its own
		// call with `save_last`. So on a month-end adjustment date the snap
		// PERSISTS into the h^.lastdate global, and because that cell is a
		// displayed output (FirstPass sets laststatus := outp) the DOS screen shows
		// the rewritten date. FirstPass derives the same cell via AddNPeriods,
		// which has no month-end stickiness — hence the two answers diverge
		// exactly when the adjustment date is the last day of its month.
		//
		// e.veryLast is captured in dosport_entry.go BEFORE the walk, mirroring
		// DOS's `DetermineVeryLast` at Amortize.pas:1320 running before the
		// adjustment prepass at :1405 — so the walk horizon is unaffected by this
		// snap and the schedule does not move. Only the reported cell changes.
		//
		// RAW + clamp rather than the normalizing wrapper, for the reason spelled
		// out at the NumberOfInstallmentsRaw call further down this file: the snap
		// can produce a phantom daterec (day 29 snapping into a non-leap February)
		// that types.DateRec cannot hold, and normalizing would roll 29/2 forward
		// to 1/3 rather than back to the month end DOS displays.
		//
		// 2026-07-29 task #94, fuzzer5 seed 21081.
		n, sy, sm, sd := dateutil.NumberOfInstallmentsRaw(adj.date, e.loan.LastDate,
			e.loan.PerYr, types.OnOrAfter)
		lastDay := sd
		if dim := dateutil.DaysInM(types.NewDateRec(sy, time.Month(sm), 1)); lastDay > dim {
			lastDay = dim
		}
		// The structural port applies the snap to its own live loan — DOS mutates
		// the global and this walk is the faithful one — and records it as well.
		// Safe here because e.veryLast was captured in dosport_entry.go BEFORE the
		// walk, mirroring DetermineVeryLast at Amortize.pas:1320 running before the
		// prepass at :1405, so the horizon and therefore the schedule cannot move.
		// (The PIECEWISE engine cannot do the same — see reAmortLastSnap in
		// dosport.go for the measured 6838.28 divergence.)
		e.loan.LastDate = types.NewDateRec(sy, time.Month(sm), lastDay)
		e.reAmortLastSnap = e.loan.LastDate
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
			// isLoanCalc=TRUE — iAMZ screen; see dosport_entry.go's seed loop.
			yd := dateutil.YearsDif(e.balloons[i].date, adj.date, e.set.Basis, e.set.YrInv, true)
			disc, _ := interest.Exxp(-rate * yd)
			adjp -= e.balloons[i].amount * disc
		}
		pw, perr := powF(e.f, -(n - 1))
		if perr != nil {
			// Same failure shape reAmortize uses for a refused Iterate above:
			// abort the walk and latch errorflag rather than seeding with junk.
			e.abort = true
			e.errorflag = true
			return
		}
		denom := 1 - pw
		if math.Abs(denom) < teeny {
			e.d = adjp / float64(n-1)
		} else {
			e.d = adjp * (e.f - 1) / denom
		}
		// AMORTOP.pas:1571 gates the Iterate refinement on THREE disjuncts, not two:
		//
		//	if (user_nballoons > 0) or (npre > 0) or ((df.c.exact) and (df.c.basis<>x360)) then
		//
		// The third one is the one that had no port. The closed-form `d` just
		// computed above is a LEVEL-PERIOD annuity payment: it assumes every
		// remaining period discounts by the same factor `f`. That assumption is
		// exactly true on the 30/360 basis, where every month is 30 days by
		// construction, and it is exactly true on any periodic (non-`exact`) walk,
		// where the growth factor is applied per period regardless of the calendar.
		// It is FALSE on an exact-day walk off a non-360 basis, where each row
		// accrues over the real elapsed days — 28, 30 and 31 apart — so the annuity
		// payment does not retire the balance and DOS refines it with the same
		// Iterate the balloon/prepayment arms use.
		//
		// 2026-07-28, fuzzer5 seed 40001 on the newly-opened day-29/30/31 loan-date
		// axis. Minimal reproducer:
		//
		//	amort_oracle 115239.23 0.0367770000 132 12 b365_360 exact \
		//	  loandmy=30.6.2025 firstdmy=30.7.2025 adj=67:0.0637620000:
		//	→ DOS int 30575.26 / paid 145814.49, Go int 30311.12 / paid 145550.35
		//	  (dInt −264.14)
		//
		// TestM5Rows puts the whole gap in one place: every row through index 66
		// agrees to the cent, and at 2/28/31 — the row after the adjustment — DOS
		// charges the refined 1144.76 while the port charges its unrefined 1165.93
		// and stays there for the rest of the schedule.
		//
		// The three ingredients the option sweep isolated all fall out of this gate.
		// `exact` is required because it IS the missing disjunct — drop it and both
		// sides print 30105.51. A day-29/30/31 loan date is required because it is
		// what makes the elapsed-day spread wide enough for the closed form to miss:
		// at day 28 (and at day 1) February steps 28-to-28 like every other month
		// and both sides print 30578.25. And the adjustment has to land late enough
		// for the miss to compound over a long remaining tail.
		//
		// The sweep's other signature — DOS returning ONE total for adj=7 and adj=8
		// (44020.01) and one for adj=67 and adj=68 (30575.26) while the port
		// returned two distinct totals for each pair — is the Amortize.pas:258-271
		// snap doing its job on the DOS side: month 8 is entered at 28/2/2026 (day
		// 30 clamped by CheckForDaysTooLarge) and `on_or_before` walks it back to
		// 30/1/2026, which is month 7's date exactly. The port snaps identically
		// (engine.go's adjustment loop, verified against `adjdump`) and lands on the
		// same date; the pairs differed only because the UNREFINED payment differs,
		// so that signature is downstream of this same gate, not a second bug.
		//
		// Written as the DOS disjunction verbatim, so the balloon/prepayment arms
		// keep their existing behaviour on the 360 basis and on periodic walks.
		if e.userNballoons > 0 || e.npre > 0 ||
			(e.set.Exact && e.set.Basis != types.Basis360) {
			saveN := e.nballoons
			e.nballoons = e.userNballoons
			t := e.nextPayment.date
			// DOS then does
			//
			//	n := NumberOfInstallments(h^.firstdate, t, h^.peryr, on_or_after);
			//
			// (AMORTOP.pas:1575) purely for its SIDE EFFECT: NumberOfInstallments
			// takes `l` as a VAR parameter and "adjusts l to be exactly on a
			// payment day, in the vicinity of the input l" (INTSUTIL.pas:936-941),
			// so `t` is SNAPPED FORWARD off whatever row happened to be pending
			// and onto the loan's own regular grid anchored at h^.firstdate. `n`
			// itself is discarded — the snapped `t` is what Iterate receives as the
			// sub-walk's first-payment date on the very next line.
			//
			// Without the snap the sub-walk starts at NextPayment.date, which after
			// a prepayment series is an OFF-CYCLE extra row (e.g. the 7/7/2032
			// weekly extra), so its terminal is a different function of d and the
			// Newton lands on a different root. The RATE branch (AMORTOP.pas:1523)
			// deliberately passes `nextpayment.date` RAW — only this AMOUNT branch
			// snaps. 2026-07-25 fuzzer5 pass 2 — verified vs the real DOS engine:
			//
			//	amort_oracle 365363.02 0.1307330000 22 2 b365 plusreg mor=18 \
			//	  b42=29501.07 pre=84:155:52:120.74 adj=102:0.0583360000: \
			//	  targ=7696.54 pts=0.039318 payhard=30038.84
			//	→ re-amortizes at 7/1/2032 to 29452.65; the unsnapped port
			//	  converged to 24179.83 and under-amortized the tail by 1591.91
			//	  of interest.
			//
			// Take the snap through the RAW variant: NumberOfInstallments's monthly
			// branch ends `if (flast) then l.d:=daysinm(l) else l.d:=f.d`
			// (INTSUTIL.pas:1013) with no clamp, so the snapped `l` is routinely a
			// PHANTOM daterec (29/2/2030 for a day-29 loan snapping into February).
			// types.DateRec cannot hold one — the normalizing wrapper would turn it
			// into 1/3/2030 and shift the whole sub-walk grid a period late — so the
			// clamped date and the phantom's raw day travel separately. See
			// dosEng.subFirstDay (dosport.go) for the seed-40024 evidence.
			_, sy, sm, sd := dateutil.NumberOfInstallmentsRaw(e.loan.FirstDate, t,
				e.loan.PerYr, types.OnOrAfter)
			clampDay := sd
			if dim := dateutil.DaysInM(types.NewDateRec(sy, time.Month(sm), 1)); clampDay > dim {
				clampDay = dim
			}
			t = types.NewDateRec(sy, time.Month(sm), clampDay)
			saveSubFirstDay := e.subFirstDay
			e.subFirstDay = sd
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
			e.subFirstDay = saveSubFirstDay
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
	// DOS advances the GLOBAL usap here (AMORTOP.pas:1610), which is the caller's
	// accumulator — write straight through it.
	*usapp = usap
	e.computeNext(&e.nextPayment, p, usapp)
	e.nextAdj++
	*p = e.nextPayment.principal
}

// solveUnknownBalloon solves the amount of a date-only "target" balloon (AO2,
// EstimateAndRefineBalloon, Amortize.pas:628-663) so the schedule retires. The
// regular payment must already be set (given). DOS first-guesses half the loan
// amount then drives the terminal balance to zero with the generic Iterate, the
// unknown x aliasing the balloon's amount. (DOS has a fast residual shortcut when
// the balloon sits ON very_last; the Iterate handles that case too, just less
// directly, so the port uses the one general path.)
func (e *dosEng) solveUnknownBalloon() bool {
	unk := e.unkBalloon
	e.balloons[unk].amount = 0.5 * e.loan.Amount // DOS first guess
	ok := e.iterate(e.loan.Amount, 0, e.loan.LoanDate, e.loan.FirstDate,
		&e.balloons[unk].amount, true, false)
	// Clamp at 0, like the piecewise SolveBalloonAmount (backward.go: `if a2 < 0`).
	// A balloon date where the regular payment already over-retires implies a
	// negative target balloon; DOS pins it to zero (the loan simply retires early)
	// and the result-advisory layer flags it as A-W4 "essentially zero".
	if e.balloons[unk].amount < 0 {
		e.balloons[unk].amount = 0
	}
	return ok
}

// solveUnknownPrepay solves the per-payment amount of an "unknown prepayment"
// series (AO9, EstimateAndRefinePeriodicPrepayment, Amortize.pas:665) so the
// schedule retires. The regular payment is given. The first guess spreads the
// terminal residual (with the prepayment off) over the NN payments; Iterate then
// drives the terminal balance to zero with the unknown aliasing the series amount.
func (e *dosEng) solveUnknownPrepay() bool {
	unk := e.unkPre
	// First guess: terminal residual of the schedule with the prepayment at 0,
	// spread over the NN payments.
	e.pres[unk].payment = 0
	p, usap := e.loan.Amount, 0.0
	saved := e.saveState()
	e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
	e.repayFancyLoan(&p, &usap, e.loan.LoanDate, e.loan.FirstDate, false, true, 0)
	residual := p
	e.restoreState(saved)
	nn := e.pres[unk].nn
	if nn < 1 {
		nn = 1
	}
	guess := residual / float64(nn)
	if guess <= 0 {
		guess = 0.1 * e.d
	}
	e.pres[unk].payment = guess
	return e.iterate(e.loan.Amount, 0, e.loan.LoanDate, e.loan.FirstDate,
		&e.pres[unk].payment, true, false)
}

// powF returns f^n for integer n, computed the way DOS computes it:
//
//	denom := (1 - exxp(-pred(n) * lnn(f)));      {AMORTOP.pas:1565}
//
// It is deliberately NOT math.Pow. math.Pow is a single correctly-scaled
// primitive; DOS's expression is a composition of two library calls, each
// rounded to double on the way through, and the two answers differ in the last
// bit on a large fraction of arguments. That bit matters here because the value
// feeds the adjustment re-seed `d`, and `d` is the seed for a bracket-free
// secant whose direction on a flat terminal plateau is decided by the last bit
// (see interest/crmath.go for the full argument and the seed-20622 trace).
//
// The earlier comment claimed math.Pow was used to dodge exxp/lnn's overflow
// guards. Those guards cannot fire on this path: DOS's own code runs the same
// exxp/lnn pair with the same arguments, so any input that would trip the guard
// would equally be a DOS error — and on the guard's own terms, f is a
// per-period growth factor slightly above 1, so lnn(f) is near zero and
// -n*lnn(f) stays far inside +/-70 for every schedule the walk can build.
// Errors are propagated rather than swallowed so a genuine out-of-range input
// surfaces instead of silently seeding the secant with a wrong value.
func powF(f float64, n int) (float64, error) {
	lnf, err := interest.Lnn(f)
	if err != nil {
		return 0, err
	}
	return interest.Exxp(float64(n) * lnf)
}
