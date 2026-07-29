package amortization

// dosport.go — a FAITHFUL structural port of the DOS amortization engine
// (legacy/src/dos_source/AMORTOP.pas + Amortize.pas), mirroring the original
// procedures (ComputeNext, FindNextExtra, CheckOffBalloon, RepayFancyLoan,
// Iterate, Re_Amortize) and control flow as closely as Go allows. It exists as a
// PARALLEL engine: the production Amortize() in engine.go is unchanged and stays
// the default; AmortizeDOS() is selected only behind a flag, and is validated
// against the real DOS oracle by the fuzzer until it is monotone-to-zero, at
// which point it becomes the default (docs/global_iterate_refactor.md).
//
// Why a port and not more heuristics: the piecewise engine solves each segment
// with a SUB-LOAN, which is not identical to the real schedule's tail, so stacked
// options (multi-ARM × balloon × skip × moratorium) drift. DOS instead runs ONE
// schedule walk (RepayFancyLoan) and solves every unknown with ONE Newton over it
// (Iterate), with Re_Amortize computed inline. Porting that structure makes the
// combinations compose correctly by construction.
//
// Fidelity notes (literal DOS behaviours the piecewise engine did NOT reproduce):
//   - ComputeNext floors every regular payment at interest-only: with no target,
//     targ.target = 0 (ZeroTarget, Amortize.pas:82) and the UNGUARDED branch
//     `if payamt-interest < targ.target then payamt := targ.target+interest`
//     (AMORTOP.pas:643/649) prevents per-period negative amortization.
//   - The final scheduled payment absorbs the residual principal (WhenToStop
//     fold, AMORTOP.pas:1208-1212).
//
// Indexing: DOS arrays are 1-based (balloon[1..nballoons]); to keep the port a
// line-for-line analogue we keep 1-based slices with an unused index 0.

import (
	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// frBalloon mirrors the FR_BALLOON xsource bit (AMORTOP.pas:15).
const frBalloon = 1

// dpBalloon is balloonrec: a dated lump (a balloon, or the merged "next extra").
type dpBalloon struct {
	date   types.DateRec
	amount float64
}

// dpPrepay mirrors the prepaymentrec fields the walk uses.
type dpPrepay struct {
	nextdate  types.DateRec
	startdate types.DateRec
	stopdate  types.DateRec
	stopOK    bool
	peryr     int
	payment   float64
	nn        int
	nnOK      bool
}

// dpAdj mirrors adjrec.
type dpAdj struct {
	date     types.DateRec
	loanrate float64
	rateOK   bool
	amount   float64
	amtok    bool
}

// dpPayment mirrors the Paymenttype object (AMORTOP.pas:39-46).
type dpPayment struct {
	baseDate, date, prevdate          types.DateRec
	payamt, interest, principal, usap float64
	paynum                            int
}

// init mirrors Paymenttype.Init (AMORTOP.pas:479-484): set base_date and prevdate.
func (pt *dpPayment) init(bdate, pdate types.DateRec) {
	pt.baseDate = bdate
	pt.prevdate = pdate
}

// dosEng holds the Pascal module-level "globals" (h^, df.c, balloon[], pre[],
// adj[], mor, targ, skp, and the running counters). One instance per Amortize
// call keeps the port pure/testable instead of using real package globals.
type dosEng struct {
	set  Settings // df.c
	loan Loan     // h^ (loanRate is mutated by Re_Amortize)

	// 1-based arrays (index 0 unused), to mirror balloon[1..nballoons] etc.
	balloons      []dpBalloon
	nballoons     int // current live balloon count (Re_Amortize may shrink/restore)
	userNballoons int // count the user supplied
	nextBalloon   int // next_balloon (1-based)
	unkBalloon    int // unkballoon (1-based): a date-only target balloon to solve (0 = none)
	pres          []dpPrepay
	npre          int
	unkPre        int // unkpre (1-based): a prepay series with blank amount to solve (0 = none)
	adjs          []dpAdj
	nadj          int
	nextAdj       int // next_adj (1-based)

	morPresent    bool
	morFirstRepay types.DateRec
	targValue     float64 // targ.target (0 when no target — see fidelity note)
	skipSet       [13]bool

	// repayFrom is Pascal's `repay_from` (Amortize.pas:1260-1288): "the date on
	// which you begin amortizing". It is the discount ORIGIN for every analytic
	// seed term — balloon PVs, prepayment-series PVs, the unknown-prepayment
	// solve and the unknown-duration solve all measure YearsDif from it, NOT
	// from the loan date. firstRepayEff is Pascal's `mor^.first_repay` AFTER
	// that same block has defaulted it (it is written even with no moratorium),
	// and nrepay is the installment count the annuity seed amortizes over.
	repayFrom     types.DateRec
	firstRepayEff types.DateRec
	nrepay        int

	// running scalars (Pascal module globals)
	f         float64 // GrowthPerPeriod
	truerate  float64
	d         float64 // regular payment
	veryLast  types.DateRec
	hardPay   bool
	abort     bool
	errorflag bool
	// overflowflag mirrors the DOS global of the same name (INTSUTIL.pas). It is a
	// LATCH: exxp's >70 guard (:1153-1156), sqrrt's negative guard (:1130-1136) and
	// lnn's non-positive guard (:1164-1171) all set it alongside errorflag, and
	// nothing clears it mid-screen — Amortize.pas:203 (FirstPass) is the only reset.
	// It is read at the top of every Iterate pass (AMORTOP.pas:1453-1454), where it
	// aborts the secant outright; see iterate().
	overflowflag bool

	// payment / nextpayment objects
	payment     dpPayment
	nextPayment dpPayment

	// Re_Amortize save/restore state
	oldNextBalloon int
	oldNpre        int
	oldPre         []dpPrepay

	// stopdate is local to RepayFancyLoan in Pascal but CheckOffBalloon reads it;
	// thread it through the engine for the duration of a walk.
	stopdate types.DateRec

	// subFirstDay carries the RAW day-of-month of a PHANTOM first-payment date
	// handed to a sub-walk. 0 means "none — use firstdate.Time.Day()".
	//
	// Re_Amortize's amount branch snaps its sub-walk's first-payment date onto the
	// loan's grid with NumberOfInstallments (AMORTOP.pas:1575), whose monthly branch
	// ends at INTSUTIL.pas:1013 with
	//
	//	if (flast) then l.d := daysinm(l) else l.d := f.d;
	//
	// — an unconditional assignment with NO clamp. So for a day-29 loan snapping
	// into February, DOS's `l` becomes the daterec 29/2/2030, a date that does not
	// exist. RepayFancyLoan then steps that phantom back one period using its OWN
	// day field (AMORTOP.pas:1149-1150, `AddPeriod(t, h^.peryr, firstdate.d,
	// subtract)`), and AddPeriod restores d := orig_day = 29 before moving the
	// month, so the sub-walk's base_date is 29/11/2029 and its first row lands on
	// 28/2/2030 (clamped only at display time by CheckForDaysTooLarge).
	//
	// Go's types.DateRec cannot hold 29/2/2030 — time.Date NORMALIZES it to
	// 1/3/2030 — so the phantom's day is carried alongside the clamped date
	// instead. Passing the clamped date (28/2/2030) with origDay 29 reproduces
	// DOS's arithmetic exactly, because AddPeriod's orig_day restore discards the
	// incoming day field before stepping.
	//
	// Without this the port back-stepped from the NORMALIZED 1/3/2030 with
	// origDay 1, put base_date on 1/12/2029, and started the sub-walk a whole
	// quarter late on 29/3/2030 — a different terminal function, hence a different
	// Newton root. 2026-07-29 fuzzer5 seed 40024, verified vs the real DOS engine:
	//
	//	amort_oracle 330927.17 0.0462590000 76 4 plusreg loandmy=29.5.2025 \
	//	  firstdmy=29.8.2025 pre=81:76:12:281.98 adj=57:0.1222290000:
	//
	// DOS re-amortizes the 281757.911004 balance at 12.2229% to 10039.7368430880
	// (trace: `RFL p=281757.911004 ld=11/29/129 fd=2/29/130` — note the phantom
	// 2/29/130 in DOS's own dump); the port landed on 10140.44 and under-charged
	// 9649.92 of interest over the tail.
	subFirstDay int
}

// firstDay returns h^.firstdate.d, the day-of-month AddPeriod steps on.
func (e *dosEng) firstDay() int { return e.loan.FirstDate.Time.Day() }

// findNextExtra mirrors FindNextExtra (AMORTOP.pas:486-543): find the next dated
// extra (balloon and/or coincident prepayment series) and report its source bits.
func (e *dosEng) findNextExtra() (xsource byte, nextextra dpBalloon) {
	if e.npre == 0 {
		if e.nextBalloon > e.nballoons {
			xsource = 0
		} else {
			nextextra = e.balloons[e.nextBalloon]
			xsource = frBalloon
		}
		return
	}
	// npre > 0: start from prepay series 1, merge coincident series and the balloon.
	nextextra.date = e.pres[1].nextdate
	xsource = 1 << 1
	nextextra.amount = e.pres[1].payment
	for i := 2; i <= e.npre; i++ {
		switch dateutil.DateComp(e.pres[i].nextdate, nextextra.date) {
		case 0:
			xsource |= 1 << uint(i)
			nextextra.amount += e.pres[i].payment
		case -1:
			xsource = 1 << uint(i)
			nextextra.date = e.pres[i].nextdate
			nextextra.amount = e.pres[i].payment
		}
	}
	if e.nextBalloon <= e.nballoons {
		switch dateutil.DateComp(e.balloons[e.nextBalloon].date, nextextra.date) {
		case 0:
			xsource |= frBalloon
			if e.set.PlusRegular {
				nextextra.amount += e.balloons[e.nextBalloon].amount
			} else {
				nextextra.amount = e.balloons[e.nextBalloon].amount
			}
		case -1:
			xsource = frBalloon
			nextextra.date = e.balloons[e.nextBalloon].date
			nextextra.amount = e.balloons[e.nextBalloon].amount
		}
	}
	return
}

// checkOffBalloon mirrors CheckOffBalloon (AMORTOP.pas:545-572): advance the
// counters for whichever extras were just consumed, retiring exhausted prepay
// series (those whose next date passes stopdate).
func (e *dosEng) checkOffBalloon(xsource byte) {
	if xsource&frBalloon == frBalloon {
		e.nextBalloon++
	}
	i := 1
	for i <= e.npre {
		if (1<<uint(i))&xsource > 0 {
			pp := &e.pres[i]
			if nd, err := dateutil.AddPeriod(pp.nextdate, pp.peryr, pp.startdate.Time.Day(), false); err == nil {
				pp.nextdate = nd
			}
			// DOS retires a prepay series against its OWN stopdate (AMORTOP.pas:560,
			// inside `with pre[i]^`), which CheckPrepayments derived from NN. Fall back
			// to the schedule stopdate only for an unbounded series.
			stop := e.stopdate
			if pp.stopOK {
				stop = pp.stopdate
			}
			if dateutil.DateComp(pp.nextdate, stop) > 0 {
				// retire series i: shift later series down, fix xsource bits.
				e.npre--
				for j := i; j <= e.npre; j++ {
					e.pres[j] = e.pres[j+1]
				}
				i--
				xsource = (xsource / 2) & ((xsource & 1) | 254)
			}
		}
		i++
	}
}

// computeNext mirrors Paymenttype.ComputeNext (AMORTOP.pas:574-664): advance to
// the next payment date (including balloons), compute the period interest, and
// resolve the payment amount under skip / balloon-or-prepay / moratorium / target.
func (e *dosEng) computeNext(pt *dpPayment, p, usap *float64) {
	// date := base_date; AddPeriod(date, peryr, firstdate.d, add)
	date, _ := dateutil.AddPeriod(pt.baseDate, e.loan.PerYr, e.firstDay(), false)
	pt.date = date
	if e.skipSet[int(pt.date.Time.Month())] {
		pt.payamt = 0
	} else {
		pt.payamt = e.d
	}

	xsource, nextextra := e.findNextExtra()
	// balloonpos classifies where the next dated extra (balloon / prepayment) sits
	// relative to this regular payment date (the sign of DateComp): <0 = the extra
	// falls BEFORE this date (off-cycle, emit it at its own earlier date), 0 = SAME
	// date (merge with the regular payment), >0 = the extra is still in the future
	// (pay the regular amount, leave the extra pending). Default 1 = "no extra yet."
	balloonpos := 1
	if xsource > 0 {
		balloonpos = dateutil.DateComp(nextextra.date, pt.date)
		// A regular payment date that has run PAST the last scheduled payment date is
		// forced to off-cycle (balloonpos = -1): a trailing balloon beyond the term
		// must be emitted at its own date, not folded into a phantom regular payment.
		if e.loan.LastOK && dateutil.DateComp(pt.date, e.loan.LastDate) > 0 {
			balloonpos = -1
		}
		if balloonpos < 0 {
			// Off-cycle extra: this row IS the extra, dated at the extra's own date.
			pt.payamt = nextextra.amount
			pt.date = nextextra.date
			e.checkOffBalloon(xsource)
		} else if balloonpos == 0 {
			if e.set.PlusRegular {
				pt.payamt += nextextra.amount
			} else {
				pt.payamt = nextextra.amount
			}
			e.checkOffBalloon(xsource)
		}
	}
	if balloonpos >= 0 {
		pt.baseDate = pt.date
	}

	// interest for [prevdate, date]
	if e.set.Daily {
		yd := dateutil.YearsDif(pt.date, pt.prevdate, e.set.Basis, e.set.YrInv, true)
		expv, _ := interest.Exxp(e.truerate * yd)
		pt.interest = (expv - 1) * (*p - *usap)
	} else {
		// periodYearFraction is the ported DaysCloseEnough/whole-period timedif.
		td := periodYearFraction(pt.prevdate, pt.date, e.loan.PerYr, &e.set)
		pt.interest = e.loan.LoanRate * td * (*p - *usap)
	}
	if e.hardPay {
		pt.interest = interest.Round2(pt.interest)
	}

	// case balloonpos: moratorium has precedence over target (an else-if chain).
	switch balloonpos {
	case 0: // payamt came in as (regular + balloon_or_pre)
		if e.morPresent && dateutil.DateComp(pt.date, e.morFirstRepay) < 0 {
			pt.payamt = pt.payamt - e.d + pt.interest
		} else if pt.payamt-pt.interest < e.targValue {
			pt.payamt = pt.payamt - e.d + e.targValue + pt.interest
		}
	case 1: // regular payment only
		if e.morPresent && dateutil.DateComp(pt.date, e.morFirstRepay) < 0 {
			pt.payamt = pt.interest
		} else if pt.payamt-pt.interest < e.targValue {
			pt.payamt = e.targValue + pt.interest
		}
	}

	pt.prevdate = pt.date
	*p = *p + pt.interest - pt.payamt
	if e.set.USARule {
		*usap = *usap + pt.interest - pt.payamt
		if *usap < 0 {
			*usap = 0
		}
	}
	pt.principal = *p
	pt.usap = *usap
}
