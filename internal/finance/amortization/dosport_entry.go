package amortization

// dosport_entry.go — state construction (buildDosEng), the payment-solve seed
// (EstimateAndRefinePayment, Amortize.pas:377-430), and the AmortizeDOS entry
// that mirrors the MakeTable dispatch for the ordinary engine. Parallel to the
// production Amortize(); selected by tests/fuzzer behind a flag.

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// errNoConverge is returned when the Newton payment solve fails to converge,
// mirroring DOS's "Computation of payment ... did not converge" (AMORTOP.pas:1489).
var errNoConverge = errors.New("amortization: payment solve did not converge")

// buildDosEng translates a LoanInput into the Pascal-style engine state.
func buildDosEng(input LoanInput) *dosEng {
	e := &dosEng{set: input.Settings, loan: input.Loan}

	// Derive the last regular payment date if absent: firstDate + (n-1) periods.
	if !e.loan.LastOK || e.loan.LastStatus < types.InOutDefault {
		last := e.loan.FirstDate
		for i := 1; i < e.loan.NPeriods; i++ {
			last, _ = dateutil.AddPeriod(last, e.loan.PerYr, e.loan.FirstDate.Time.Day(), false)
		}
		e.loan.LastDate = last
		e.loan.LastOK = true
	}
	e.veryLast = e.loan.LastDate

	// Balloons, sorted by date, 1-based with index 0 unused. A balloon with a date
	// but no amount is a "target balloon" (AO2) whose amount is solved later; it is
	// included with a 0 placeholder and its sorted index recorded in e.unkBalloon.
	type bb struct {
		date    types.DateRec
		amount  float64
		unknown bool
	}
	var bs []bb
	for i := range input.Balloons {
		b := &input.Balloons[i]
		switch {
		// A $0 balloon with the amount ENTERED is a real extra: in REPLACE mode
		// it substitutes a $0 payment for the regular one (a skipped
		// installment) — DOS ComputeNext balloonpos=0, AMORTOP.pas:614-621.
		// 2026-07-11 pass-2 finding 9: filtering on Amount != 0 dropped it.
		// Verified: `amort_oracle 10000 0.12 24 12 b12=0` → payment 491.2571,
		// interest 1298.91 (the filtered port solved the plain 470.7347).
		case b.AmountStatus >= types.InOutDefault:
			bs = append(bs, bb{date: b.Date, amount: b.Amount})
		case b.DateStatus >= types.InOutDefault && b.AmountStatus < types.InOutDefault:
			bs = append(bs, bb{date: b.Date, unknown: true}) // AO2 target balloon
		}
	}
	sort.Slice(bs, func(i, j int) bool { return dateutil.DateComp(bs[i].date, bs[j].date) < 0 })
	e.balloons = make([]dpBalloon, len(bs)+1)
	for i, x := range bs {
		e.balloons[i+1] = dpBalloon{date: x.date, amount: x.amount}
		if x.unknown {
			e.unkBalloon = i + 1
		}
	}
	e.nballoons = len(bs)
	e.userNballoons = len(bs)
	e.nextBalloon = 1
	for _, b := range bs {
		if dateutil.DateComp(b.date, e.veryLast) > 0 {
			e.veryLast = b.date
		}
	}

	// Prepayment series, 1-based. A series with a start + count (NN) but a BLANK
	// amount is an "unknown prepayment" (AO9) solved later; include it with a 0
	// placeholder and record its index in e.unkPre.
	var ps []dpPrepay
	unkPreLocal := -1
	for i := range input.Prepayments {
		pp := &input.Prepayments[i]
		if pp.StartDateStatus < types.InOutDefault {
			continue
		}
		known := pp.PaymentStatus >= types.InOutDefault
		unknown := !known && pp.NNStatus >= types.InOutDefault // AO9: blank amount, count given
		if !known && !unknown {
			continue
		}
		dp := dpPrepay{nextdate: pp.StartDate, startdate: pp.StartDate, peryr: pp.PerYr, payment: pp.Payment}
		if unknown {
			dp.payment = 0 // placeholder; solved by solveUnknownPrepay
		}
		if pp.StopDateStatus >= types.InOutDefault {
			dp.stopdate, dp.stopOK = pp.StopDate, true
			if dateutil.DateComp(pp.StopDate, e.veryLast) > 0 {
				e.veryLast = pp.StopDate
			}
		}
		if pp.NNStatus >= types.InOutDefault {
			dp.nn, dp.nnOK = pp.NN, true
		}
		// CheckPrepayments (AMORTOP.pas:416-419): when a count (NN) is given but no
		// stop date, derive stopdate = AddNPeriods(startdate, peryr, pred(nn)) and
		// let THAT date bound the series. Without this the series has no per-series
		// bound and the walk applies the prepayment every period to the end of the
		// loan. It must be AddNPeriods, not nn-1 iterated AddPeriod calls: the two
		// disagree for peryr=24 off-grid anchor days (see CheckPrepaymentStops).
		// Normally CheckPrepaymentStops has already filled this in at the Amortize
		// entry; this arm covers standalone AmortizeDOS callers.
		if dp.nnOK && !dp.stopOK && dp.nn > 0 {
			if sd, err := dateutil.AddNPeriods(pp.StartDate, pp.PerYr, dp.nn-1); err == nil {
				dp.stopdate, dp.stopOK = sd, true
				if dateutil.DateComp(sd, e.veryLast) > 0 {
					e.veryLast = sd
				}
			}
		}
		// The mirror of the above (CheckPrepayments, AMORTOP.pas:423-431): when a
		// STOP DATE is given but no count, DOS derives nn on_or_before and marks it
		// an output. The analytic seed's zero-rate branch reads nn.
		if dp.stopOK && !dp.nnOK {
			if n, _ := dateutil.NumberOfInstallments(dp.startdate, dp.stopdate, dp.peryr, types.OnOrBefore); n > 0 {
				dp.nn, dp.nnOK = n, true
			}
		}
		ps = append(ps, dp)
		if unknown {
			unkPreLocal = len(ps) - 1
		}
	}
	e.pres = make([]dpPrepay, len(ps)+1)
	copy(e.pres[1:], ps)
	e.npre = len(ps)
	if unkPreLocal >= 0 {
		e.unkPre = unkPreLocal + 1
	}

	// Adjustments (rate and/or amount), sorted by date, 1-based.
	var as []dpAdj
	for i := range input.Adjustments {
		a := &input.Adjustments[i]
		if a.DateStatus < types.InOutDefault {
			continue
		}
		da := dpAdj{date: a.Date}
		if a.LoanRateStatus >= types.InOutDefault {
			da.loanrate, da.rateOK = a.LoanRate, true
		}
		if a.AmtOK {
			da.amount, da.amtok = a.Amount, true
		}
		as = append(as, da)
	}
	sort.Slice(as, func(i, j int) bool { return dateutil.DateComp(as[i].date, as[j].date) < 0 })
	e.adjs = make([]dpAdj, len(as)+1)
	copy(e.adjs[1:], as)
	e.nadj = len(as)
	e.nextAdj = 1

	if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
		e.morPresent = true
		e.morFirstRepay = input.Moratorium.FirstRepay
	}
	// targ.target: when a target is SET, the per-period payment is floored at
	// (target + interest). When NO target is set, the floor must be INERT — the
	// oracle negative-amortizes a low-payment balloon loan (balance grows, prin<0)
	// rather than flooring to interest-only, so the effective no-target value is
	// -infinity, NOT the literal 0 that ZeroTarget writes (Amortize.pas:82). Using
	// -inf makes `payamt-interest < targValue` never fire, matching DOS.
	if input.Target.TargetStatus >= types.InOutDefault {
		e.targValue = input.Target.TargetValue
	} else {
		e.targValue = -1e300
	}
	e.skipSet = input.SkipMonths.MonthSet

	e.setRepayFrom()

	e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
	e.truerate, _ = ComputeTrueRate(&e.loan, &e.set)
	return e
}

// setRepayFrom ports Amortize.pas:1260-1319 — the block that establishes
// repay_from, mor^.first_repay and nrepay before any solve runs. DOS's own
// comment for it:
//
//	{repay_from is the date on which you begin amortizing.}
//	{first_repay is the first payment date that includes principal.}
//	{If first_repay is specified, then repay_from is one period before first_repay.}
//	{If prepaid="Y" is specified, then repay_from is one period before firstdate.}
//	{Otherwise, repay_from is just the loan date.}
//
// Note the non-moratorium arm still WRITES mor^.first_repay (to balloon[1]'s
// date when a balloon precedes the first payment, else to firstdate), and the
// nrepay override below keys off THAT — so a pre-firstdate balloon shortens the
// amortization window exactly as a moratorium does.
//
// `prepaid` here is the already-DEMOTED flag: AmortizeDOS applies
// Amortize.pas:1252-1259 (`if firstdate-1period < loandate and not in_advance
// then prepaid := false`) to input.Settings BEFORE buildDosEng runs, so
// e.set.Prepaid is the value this block sees in DOS.
func (e *dosEng) setRepayFrom() {
	day := e.loan.FirstDate.Time.Day()
	switch {
	case e.morPresent:
		// NumberOfInstallments takes first_repay as a VAR parameter and SNAPS it
		// on_or_after onto the payment grid (Amortize.pas:1263); everything
		// downstream uses the snapped date.
		_, fr := dateutil.NumberOfInstallments(e.loan.FirstDate, e.morFirstRepay, e.loan.PerYr, types.OnOrAfter)
		if !dateutil.DateOK(fr) {
			fr = e.morFirstRepay
		}
		e.morFirstRepay = fr
		e.firstRepayEff = fr
		e.repayFrom, _ = dateutil.AddPeriod(fr, e.loan.PerYr, day, true)
	default:
		if e.nballoons > 0 && dateutil.DateComp(e.balloons[1].date, e.loan.FirstDate) < 0 {
			e.firstRepayEff = e.balloons[1].date
		} else {
			e.firstRepayEff = e.loan.FirstDate
		}
		if e.set.Prepaid {
			e.repayFrom, _ = dateutil.AddPeriod(e.loan.FirstDate, e.loan.PerYr, day, true)
		} else {
			e.repayFrom = e.loan.LoanDate
		}
	}

	// nrepay (Amortize.pas:1299-1319): the REAL number of installments to
	// amortize over. Only overridden when principal repayment starts somewhere
	// other than the first payment date; DOS refuses the loan outright when the
	// override is non-positive, which the port surfaces as a zero nrepay that
	// estimateAndRefinePayment falls back on.
	e.nrepay = e.loan.NPeriods
	if e.loan.LastOK && dateutil.DateOK(e.loan.LastDate) &&
		dateutil.DateOK(e.firstRepayEff) && dateutil.DateComp(e.firstRepayEff, e.loan.FirstDate) != 0 {
		if nr, _ := dateutil.NumberOfInstallments(e.firstRepayEff, e.loan.LastDate, e.loan.PerYr, types.OnOrBefore); nr > 0 {
			e.nrepay = nr
		}
	}
}

// estimateAndRefinePayment mirrors Amortize.pas:377-430: analytic annuity seed
// over the balloon-netted balance, then Iterate(til_adj) to refine. Sets e.d.
func (e *dosEng) estimateAndRefinePayment() bool {
	p := e.loan.Amount
	usap := 0.0
	adjp := e.loan.Amount
	rate, _ := interest.RateFromYield(e.loan.LoanRate, byte(e.loan.PerYr), e.set.YrDays)
	for i := 1; i <= e.userNballoons; i++ {
		// isLoanCalc=TRUE: DOS's YearsDif takes its non-360 form from the SCREEN
		// (INTSUTIL.pas:805) and this is iAMZ, so each calendar year is divided by
		// its own 365/366 rather than by yrinv's 365.25.
		//
		// The discount ORIGIN is repay_from, not the loan date (Amortize.pas:387).
		// They coincide only on a non-prepaid loan with no moratorium.
		yd := dateutil.YearsDif(e.balloons[i].date, e.repayFrom, e.set.Basis, e.set.YrInv, true)
		disc, _ := interest.Exxp(-rate * yd)
		adjp -= e.balloons[i].amount * disc
	}

	// Prepayment-series PV (Amortize.pas:388-396 with FirstLastAndFF at :370-375):
	//
	//	first := exxp(-rate * YearsDif(pre[i]^.startdate, repay_from));
	//	last  := exxp(-rate * YearsDif(pre[i]^.stopdate,  repay_from));
	//	ff    := exxp(-rate / pre[i]^.peryr);
	//	if (abs(1-ff)>teeny) then
	//	  adjp := adjp - pre[i]^.payment * (first - last * ff) / (1 - ff)
	//	else adjp := adjp - pre[i]^.payment * pre[i]^.nn;
	//
	// (first - last*ff)/(1-ff) is the closed form for the geometric run of
	// discount factors from startdate through stopdate INCLUSIVE, stepping one
	// series period at a time; the |1-ff| guard is the zero-rate limit, where the
	// sum degenerates to the plain count.
	//
	// This loop was previously absent, on the premise recorded in the deleted
	// comment that "Iterate refines" it away. It does not. The payment-solve
	// objective is genuinely MULTI-ROOTED once skips and a target are in play —
	// ComputeNext's balloonpos=0 arm subtracts `d` unconditionally
	// (AMORTOP.pas:614-621), even on a skip month where payamt never contained
	// `d`, so a skip+prepayment row's principal moves OPPOSITE to `d` — and
	// Newton then lands in whichever basin the SEED points at. With a long
	// series the omitted PV term is large enough to change basins outright.
	//
	// 2026-07-25 fuzzer5 seed 8918 — verified against the real DOS engine:
	//
	//	amort_oracle 303282.89 0.1336490000 216 12 b365 plusreg mor=33 \
	//	  b100=64706.92 b112=17670.36 b127=34083.30 pre=98:88:12:638.50 \
	//	  pre=11:16:26:303.75 targ=913.15 skip=2,8,11 pts=0.017783
	//	  DOS  d = 2083.0747  int=448678.64  paid=751961.53
	//	  port d = 1960.56    int=445404.52  paid=748687.41
	//
	// Both walks emit 231 rows and agree exactly through row 113; scanning the
	// oracle with pay= over 1900..2100 shows the unfolded terminal residual
	// crossing zero at BOTH 1960.56 and 2083.07. Neither walk is wrong — only the
	// seed was.
	for i := 1; i <= e.npre; i++ {
		pr := &e.pres[i]
		if pr.peryr <= 0 || pr.payment == 0 {
			continue
		}
		ffExp, _ := interest.Exxp(-rate / float64(pr.peryr))
		if math.Abs(1-ffExp) > types.Teeny {
			ydF := dateutil.YearsDif(pr.startdate, e.repayFrom, e.set.Basis, e.set.YrInv, true)
			first, _ := interest.Exxp(-rate * ydF)
			// A series with neither a stop date nor a count runs to the end of the
			// loan; DOS leaves stopdate.m = unkbyte there (AMORTOP.pas:433) and the
			// walk bounds it at very_last, so use very_last as the PV horizon.
			stop := pr.stopdate
			if !pr.stopOK || !dateutil.DateOK(stop) {
				stop = e.veryLast
			}
			ydL := dateutil.YearsDif(stop, e.repayFrom, e.set.Basis, e.set.YrInv, true)
			last, _ := interest.Exxp(-rate * ydL)
			adjp -= pr.payment * (first - last*ffExp) / (1 - ffExp)
		} else {
			adjp -= pr.payment * float64(pr.nn)
		}
	}

	// nrepay: DOS amortizes over the number of PAYING installments, not the full
	// term (Amortize.pas:1299-1319) — see setRepayFrom, which computes it.
	nrepay := e.nrepay
	if nrepay <= 0 {
		nrepay = e.loan.NPeriods
	}
	e.d = annuityPayment(adjp, e.f, nrepay)

	// DOS EARLY-EXIT (Amortize.pas:402-407): a PREPAID loan with none of exact /
	// in-advance / balloon / prepayment / target / skip takes the closed-form
	// annuity over nrepay and EXITS — it does NOT Iterate. (in-advance and R78
	// never reach AmortizeDOS — dosPortCanHandle excludes them. `exact` DOES
	// reach it now, on the 360 basis where DOS treats it as inert everywhere
	// else; this is the one site whose `not df.c.exact` term is NOT basis-gated,
	// so it is carried verbatim below.) Moratorium
	// is NOT excluded, so a prepaid moratorium loan keeps this uniform-period
	// closed form; at a day-count frequency it differs from the actual-day
	// Iterate the non-prepaid case uses. 2026-07-13 pass-4 — verified vs the
	// real DOS engine:
	//
	//	amort_oracle 100000 0.10 104 52 prepaid mor=3 → payment 1186.6343
	//	(the closed form over nrepay=92 on the weekly-forced 365 basis; the
	//	 day-count Iterate gives the mor-alone 1186.5113, which DOS uses only
	//	 WITHOUT prepaid)
	noTarget := e.targValue <= -1e299
	if !e.set.Exact && e.set.Prepaid && e.userNballoons == 0 && e.npre == 0 &&
		noTarget && !anySkip(e.skipSet) {
		return true
	}
	return e.iterate(p, usap, e.loan.LoanDate, e.loan.FirstDate, &e.d, false, false)
}

// dosPortEnabled routes the production Amortize fancy path through the faithful
// port (AmortizeDOS). The port is validated to ZERO oracle divergence on the
// SOLVED-PAYMENT, monthly option cube (TestDOSPortFuzz, N=1000). Flipping it ON
// as the universal default, however, surfaced feature-parity gaps the existing
// suite depends on — these are the scoped blockers for the full cutover (M3):
//
//   - Advisory layer: DONE — AmortizeDOS now reproduces the early-payoff warning,
//     A-W9 (implied terminating balloon), the unusually-high-rate warning, the
//     balloon echo, and appendResultAdvisories (A-W4/5/6/7/11). Differentially
//     validated vs the piecewise engine (TestDOSPortAdvisoryParity{,Fuzz}): on
//     row-by-row-identical schedules the two engines emit identical advisories
//     (A-W9's exact balloon-cents on multi-segment ARM/moratorium loans excepted,
//     where "the regular payment" baseline is inherently ambiguous).
//   - AO6 (payment-only ⇒ solve rate) and AO7 (date-only re-amortize): DONE —
//     ported + validated vs the oracle (TestDOSPortAdjSolveSweep, 0 across ~1400
//     cases incl. skip/moratorium companions). AO6/AO7 + balloon stay GATED to the
//     piecewise engine (a shared DOS early-payoff gap; see dosPortCanHandle).
//   - AO2 (date-only target balloon ⇒ solve amount): DONE — solveUnknownBalloon
//     drives the (oracle-exact) forward walk's terminal to zero via the generic
//     Iterate (Amortize.pas:628 EstimateAndRefineBalloon). Validated by feeding the
//     solved balloon back to the oracle (TestDOSPortAO2BalloonSolve, 0/300). Allowed
//     only with a GIVEN payment (a blank payment + blank balloon is under-determined).
//   - Prepayment SERIES forward (known amount) + AO9 (blank amount ⇒ solve the
//     per-payment amount, Amortize.pas:665): DONE — the forward-walk fidelity bug
//     (a series with NN but no stop date ran to the loan end instead of NN payments)
//     is fixed (derive the per-series stop date from NN; retire against it), and AO9
//     solves the amount via the generic Iterate. Both validated vs the oracle
//     (TestDOSPortPrepayForwardSweep 0/600, TestDOSPortAO9PrepaySolve 0/250).
//   - Prepayment DURATION solve (AO10, known amount, blank NN+stop ⇒ solve the
//     count, DeterminePrepaymentDuration Amortize.pas:709): DONE — AmortizeDOS
//     reuses the oracle-validated closed-form SolvePrepaymentDuration up front,
//     pins NN+stop, and the forward walk runs the bounded series
//     (TestDOSPortAO10Duration, 0/181). The whole prepayment area is now ported.
//   - hard_payment rounding of the BALLOON amount to cents (the port rounds
//     per-period interest but not the balloon).
//   - GIVEN-payment + odd/long first period on a non-monthly basis (e.g. AM_EX15
//     quarterly target loan) and the per-row balance rounding tail on
//     given-payment balloon sweeps — unvalidated by the (solved, monthly) fuzzer.
//   - degenerate loans (skip-every-month) where the port's Newton can't converge.
//   - IN-ADVANCE (annuity-due) is a non-fancy mode (DOS disables it for fancy
//     loans) handled by the production generateSimpleSchedule; the port delegates
//     it (s.InAdvance ⇒ false below). Production's in-advance was made DOS-faithful
//     this session — it had been omitting the upfront settlement interest
//     (amount·(f-1)) from the total in EVERY in-advance loan; now emitted as a
//     PayNum-0 row, validated 0/200 vs the oracle (TestInAdvanceSettlementRow).
//
// NOW THE DEFAULT (2026-06-25). The port serves its validated forward / payment-
// solve / backward-solve domain via dosPortCanHandle; everything outside it
// (in-advance, R78, exact, solve-for-amount/rate, AO6, REPLACE mode, off-cycle
// balloons, degenerate terms, and the production backward solvers' internal trial
// evaluations) routes to the piecewise Amortize, which remains the entry point and
// the required fallback. The full suite + every differential fuzzer are green with
// this on. See docs/global_iterate_refactor.md §Step(1m).
var dosPortEnabled = true

// dosPortCanHandle reports whether the faithful port may serve this loan. It is
// deliberately narrowed to the domain TestDOSPortFuzz exercised: ordinary basis
// (no in-advance / Rule-of-78 / exact-daily), amount+rate+term known (the port
// solves/uses only the PAYMENT, not amount or rate), known-amount balloons,
// rate-only ARMs, and no prepayment series. Everything outside this stays on the
// piecewise engine until those paths are ported and fuzzed.
// A production backward solver (SolveLoanAmount, SolveRate, SolveBalloonAmount,
// SolvePrepaymentAmount) sets LoanInput.inBackwardSolve on its input so its inner
// trial evaluations stay on the piecewise engine they were validated against —
// routing trials through the port could shift the converged result on edge inputs.
// It rides the input (per-call) rather than a package global so concurrent requests
// can never flip each other's engine selection (goroutine-safe by construction);
// see TestConcurrentBackwardSolveNoRace.
func dosPortCanHandle(in LoanInput, loan Loan, s *Settings) bool {
	if !dosPortEnabled || !in.Fancy || in.inBackwardSolve {
		return false
	}
	// Degenerate term beyond the schedule safety bound — the piecewise engine has
	// the explicit 10000-period error; keep it there.
	if loan.NPeriods > MaxSchedulePeriods {
		return false
	}
	if s.InAdvance || s.R78 || s.Daily {
		return false
	}
	// `exact` is INERT in DOS for a FANCY loan on the 360 basis. Every site that
	// reads df.c.exact is either basis-gated or dominated by `fancy`:
	//
	//	AMORTOP.pas:625   if ((df.c.basis=x360) or (not df.c.exact)) and
	//	                     DaysCloseEnough(...)                  -- basis-gated
	//	AMORTOP.pas:1438  if (fancy) or ((df.c.exact) and (df.c.basis<>x360))
	//	AMORTOP.pas:1464  if (fancy) or ((df.c.exact) and (df.c.basis<>x360))
	//	AMORTOP.pas:1571  if (user_nballoons>0) or (npre>0) or
	//	                     ((df.c.exact) and (df.c.basis<>x360))
	//	Amortize.pas:458  if ((df.c.basis=x360) or (not df.c.exact)) and ...
	//	Amortize.pas:553  if (fancy) or (df.c.exact) or (not (df.c.basis=x360))
	//	Amortize.pas:572  ditto
	//	Amortize.pas:1493 if (fancy) or ((df.c.exact) and (not df.c.R78)) or
	//	                     (not (df.c.basis=x360))
	//
	// and INTSUTIL.pas:422 is a status-line legend. The ONE site where `exact`
	// bites at the 360 basis without a `fancy` disjunct is the closed-form escape
	// in EstimateAndRefinePayment (Amortize.pas:402), and dosSolvePayment carries
	// that `!e.set.Exact` term verbatim.
	//
	// Routing exact × 360 to the piecewise engine was therefore not a fidelity
	// choice, it was a fidelity LOSS: `exact` silently swapped engines on a loan
	// where DOS's answer does not move at all. 2026-07-25 fuzzer5 seed 20110 —
	// verified vs the real DOS engine (dropping `exact` leaves DOS's totals
	// BIT-IDENTICAL while the port's answer changed by $982.27):
	//
	//	amort_oracle 202124.13 0.0706850000 192 12 exact prepaid plusreg usa \
	//	  loandmy=15.6.2023 firstdmy=15.7.2023 mor=89 b103=41267.45 \
	//	  b104=55126.12 pre=56:233:24:188.81 pre=71:49:52:16.90 \
	//	  adj=80:0.0329640000: adj=133:0.0352250000: targ=69.69 skip=5-7 \
	//	  pts=0.009807
	//	→ DOS int=126171.05 paid=328295.18 both WITH and WITHOUT `exact`;
	//	  the port gave 126171.05 without it and 127153.32 with it.
	//
	// The gap was the payment re-solved at the rate-only adjustment (adj=80),
	// invisible until the moratorium's first-repay row (mor=89) because every row
	// between the two is interest-only: DOS 1049.12 vs piecewise 1066.26 on the
	// reduced repro
	//
	//	100000 0.10 192 12 exact plusreg loandmy=15.6.2023 firstdmy=15.7.2023 \
	//	  mor=89 pre=56:233:24:188.81 adj=80:0.05:
	//	→ DOS int=91843.90, piecewise 91409.82.
	//
	// (The piecewise segment-payment solve is still wrong for exact × NON-360,
	// where DOS genuinely does take a different path; that axis is unchanged
	// here and remains open.)
	if s.Exact && s.Basis != types.Basis360 {
		return false
	}
	// The port solves/uses only the payment: amount and rate must be known.
	if in.Loan.AmountStatus < types.InOutDefault || in.Loan.LoanRateStatus < types.InOutDefault {
		return false
	}
	if loan.NPeriods <= 0 || !loan.LastOK || loan.PerYr <= 0 {
		return false
	}
	// REPLACE mode (plus_regular=false: a balloon/prepayment REPLACES the regular
	// payment rather than ADDING to it) is unvalidated through the port — every
	// fuzzer used plus_regular=true. Route extras-in-REPLACE-mode to piecewise. This
	// also keeps the piecewise backward-solvers (SolveBalloonAmount /
	// SolvePrepaymentAmount, which call Amortize internally with trial values) off
	// the port for REPLACE-mode loans, where its forward schedule would differ.
	if !s.PlusRegular && (len(in.Balloons) > 0 || len(in.Prepayments) > 0) {
		return false
	}
	// Prepayment series: forward (known amount, bounded by NN or stop date) and AO9
	// (blank amount + count, with a given payment ⇒ solve the amount) are validated
	// vs the oracle (TestDOSPortPrepayForwardSweep, TestDOSPortAO9PrepaySolve). The
	// DURATION solve (known amount, blank NN AND blank stop date ⇒ solve the count)
	// is NOT ported — route it to the piecewise engine.
	for i := range in.Prepayments {
		pp := &in.Prepayments[i]
		if pp.StartDateStatus < types.InOutDefault {
			continue // empty row
		}
		amtKnown := pp.PaymentStatus >= types.InOutDefault
		bounded := pp.NNStatus >= types.InOutDefault || pp.StopDateStatus >= types.InOutDefault
		payGiven := in.Loan.PayAmtStatus >= types.InOutDefault
		switch {
		case amtKnown && bounded:
			// forward known, bounded series
		case !amtKnown && pp.NNStatus >= types.InOutDefault && payGiven:
			// AO9 unknown amount (payment given)
		case amtKnown && !bounded && payGiven && in.Loan.NStatus >= types.InOutDefault:
			// AO10 duration solve (known amount, blank count+stop, payment + term given)
		default:
			return false // an unsupported / unbounded prepayment shape
		}
	}
	for i := range in.Balloons {
		b := &in.Balloons[i]
		if b.DateStatus < types.InOutDefault {
			continue
		}
		if b.AmountStatus < types.InOutDefault {
			// AO2 target balloon: the port solves the amount, but only when the
			// payment is GIVEN (a blank payment + blank balloon is under-determined).
			if in.Loan.PayAmtStatus < types.InOutDefault {
				return false
			}
		}
		// OFF-CYCLE balloon (a date that does not land on a payment date) → piecewise.
		// The fuzzers only placed balloons ON payment dates; the port applies an
		// off-cycle balloon at the next payment instead of its own date, where the
		// piecewise engine drains it at the exact date (the Rev-10 off-cycle fix).
		if !dateutil.DateOK(b.Date) {
			return false
		}
		d := loan.FirstDate
		onGrid := false
		for k := 0; k <= loan.NPeriods+1; k++ {
			c := dateutil.DateComp(d, b.Date)
			if c == 0 {
				onGrid = true
				break
			}
			if c > 0 {
				break // walked past the balloon date without a match
			}
			nd, err := dateutil.AddPeriod(d, loan.PerYr, loan.FirstDate.Time.Day(), false)
			if err != nil {
				break
			}
			d = nd
		}
		if !onGrid {
			return false
		}
	}
	// Adjustment shapes validated through the port vs the DOS oracle: rate-only
	// (AO5) and set-both (rate+amount) — including with balloons; date-only
	// re-amortize (AO7) and payment-only ⇒ solve implied rate (AO6, AMORTOP.pas:1521
	// EstimateAndRefineAdjRate) — validated standalone and across every NON-balloon
	// option combo (TestDOSPortAdjSolve{Probe,Sweep}).
	//
	// EXCEPTION — the ONE deliberate divergence from DOS (a confirmed DOS BUG, not
	// reproduced by decision): a date-only (AO7) or payment-only (AO6) adjustment
	// combined with a balloon dated AFTER it makes DOS retire the loan EARLY with a
	// bogus "re-computed at 0.0000%" final row (100k/24mo/6% + balloon@12 + adj@6::
	// → DOS interest 3172.08, payoff at month 7). Instrumenting the DOS engine
	// proved Re_Amortize is BYTE-IDENTICAL to the normal case (same payment 3597.14)
	// — the corruption is in DOS's build-path print recursion (DecideWhetherToPrint-
	// ALine/PrintAndReset), where the post-adjustment row's date corrupts to
	// very_last and trips the payoff fold. BOTH Go engines produce the financially
	// correct ~6331.47 and agree with each other; we intentionally keep that. Route
	// AO6/AO7 + balloon to the piecewise engine (behavior-preserving). Full writeup +
	// instrumentation findings: docs/dos_known_frontier.md ("ONE deliberate
	// divergence"); guarded by TestAO7BalloonDOSBugCharacterization.
	hasBalloon := false
	for i := range in.Balloons {
		b := &in.Balloons[i]
		if b.AmountStatus >= types.InOutDefault { // presence by status (pass-2 finding 9)
			hasBalloon = true
			break
		}
	}
	if hasBalloon {
		for i := range in.Adjustments {
			if in.Adjustments[i].LoanRateStatus < types.InOutDefault { // AO6 / AO7
				return false
			}
		}
	}
	// AO6 (a payment-bearing adjustment ⇒ solve the implied rate) carries the A-W12
	// negative-implied-rate Note that the port does not emit; route it to piecewise.
	for i := range in.Adjustments {
		if in.Adjustments[i].AmtOK {
			return false
		}
	}
	// Degenerate: every calendar month skipped — the loan never amortizes. The
	// piecewise engine has the explicit "does not retire" handling and the
	// 10000-period safety; route there.
	if anySkip(in.SkipMonths.MonthSet) {
		allSkip := true
		for m := 1; m <= 12; m++ {
			if !in.SkipMonths.MonthSet[m] {
				allSkip = false
				break
			}
		}
		if allSkip {
			return false
		}
	}
	return true
}

// AmortizeDOS is the faithful-port entry: it mirrors the MakeTable flow — solve
// the blank payment (EstimateAndRefinePayment) when one is unknown, then build
// the schedule with RepayFancyLoan(entire). It is the parallel engine validated
// against the oracle; the production Amortize remains the default.
func AmortizeDOS(input LoanInput) AmortResult {
	// DOS coerces weekly/biweekly off the 360 basis in MakeTable's preprocessing,
	// upstream of every solve (Amortize.pas:297-303). See coerceSubMonthlyBasis.
	coerceSubMonthlyBasis(&input)
	// AO10 (DeterminePrepaymentDuration, Amortize.pas:709): a prepayment series with
	// a known amount but blank count AND blank stop date — solve how many extra
	// payments retire the loan, then pin NN + stop date so the (oracle-exact) forward
	// walk runs the bounded series. The closed-form solver SolvePrepaymentDuration is
	// the validated shared port of the DOS routine; reuse it as a pure function. The
	// solved NN + stop date are written back into the input prepayment (matching the
	// piecewise engine, engine.go:455-458) so the API/UI read them; the shared slice
	// backing means this propagates to the caller.
	if input.Loan.PayAmtStatus >= types.InOutDefault && input.Loan.NStatus >= types.InOutDefault {
		for i := range input.Prepayments {
			pp := &input.Prepayments[i]
			if pp.StartDateStatus >= types.InOutDefault && pp.PaymentStatus >= types.InOutDefault &&
				pp.StopDateStatus < types.InOutDefault && pp.NNStatus < types.InOutDefault {
				nn, stop, err := SolvePrepaymentDuration(input, i)
				if err != nil {
					return AmortResult{Err: err}
				}
				pp.NN, pp.NNStatus = nn, types.InOutInput
				pp.StopDate, pp.StopDateStatus = stop, types.InOutInput
			}
		}
	}

	// DOS clears prepaid outright when the loan is taken STRICTLY AFTER the
	// natural period start (firstDate minus one period): `if
	// DateComp(natural_start, loandate) < 0 and not in_advance then prepaid :=
	// false` (Amortize.pas). The production Amortize wrapper (engine.go) applies
	// this before delegating, so this is a NO-OP in production; it is applied here
	// too so the faithful port is DOS-correct when invoked standalone — an
	// odd/long first period that pushes the natural start before the loan date
	// (the port fuzzers' oddFirst+prepaid domain). Without it the port keeps
	// prepaid and diverges (e.g. 152000 0.0699 48 2 first=4 prepaid → DOS/prod
	// 160132.93, prepaid-kept port 154865.13).
	if input.Settings.Prepaid && !input.Settings.InAdvance && dateutil.DateOK(input.Loan.FirstDate) {
		if ns, nerr := dateutil.AddPeriod(input.Loan.FirstDate, input.Loan.PerYr,
			input.Loan.FirstDate.Time.Day(), true); nerr == nil &&
			dateutil.DateComp(ns, input.Loan.LoanDate) < 0 {
			input.Settings.Prepaid = false
		}
	}

	e := buildDosEng(input)
	// Capture the ORIGINAL rate before the build: Re_Amortize mutates
	// e.loan.LoanRate / e.truerate at each ARM, but the prepaid first-period stub
	// must use the rate in force over [loanDate, paidthru] — the original rate.
	origRate, origTrueRate := e.loan.LoanRate, e.truerate

	// hard_payment is true only for a USER-GIVEN payment (per-period interest
	// rounding); a solved payment runs unrounded (Iterate sets hard_payment=false,
	// AMORTIZE.pas:1496).
	if input.Loan.PayAmtStatus >= types.InOutDefault {
		e.d = input.Loan.PayAmt
		e.hardPay = true
	} else {
		e.hardPay = false
		if !e.estimateAndRefinePayment() {
			return AmortResult{Err: errNoConverge}
		}
	}

	// AO2: a date-only "target" balloon — solve the amount that retires the loan
	// (Amortize.pas:628 EstimateAndRefineBalloon). The payment is given; the solve
	// drives the terminal balance to zero. Done before the build so the schedule
	// runs with the resolved balloon amount.
	if e.unkBalloon > 0 {
		if !e.solveUnknownBalloon() {
			return AmortResult{Err: errNoConverge}
		}
		// Write the solved amount back into the input balloon (matches the piecewise
		// engine, engine.go:398-399) so the API/UI and the A-W4/A-W5 advisories read
		// it. The shared slice backing propagates this to the caller.
		solved := e.balloons[e.unkBalloon].amount
		for i := range input.Balloons {
			b := &input.Balloons[i]
			if b.DateStatus >= types.InOutDefault && b.AmountStatus < types.InOutDefault {
				b.Amount, b.AmountStatus = solved, types.InOutOutput
				break
			}
		}
	}

	// AO9: an "unknown prepayment" series (count given, amount blank) — solve the
	// per-payment amount that retires the loan (Amortize.pas:665). Payment given.
	prepaySolved := false
	prepaySolvedAmt := 0.0
	if e.unkPre > 0 {
		if !e.solveUnknownPrepay() {
			return AmortResult{Err: errNoConverge}
		}
		prepaySolved = true
		prepaySolvedAmt = e.pres[e.unkPre].payment
		for i := range input.Prepayments {
			pp := &input.Prepayments[i]
			if pp.StartDateStatus >= types.InOutDefault && pp.PaymentStatus < types.InOutDefault &&
				pp.NNStatus >= types.InOutDefault {
				pp.Payment, pp.PaymentStatus = prepaySolvedAmt, types.InOutOutput
				break
			}
		}
	}

	p := e.loan.Amount
	usap := 0.0
	e.hardPay = input.Loan.PayAmtStatus >= types.InOutDefault
	// Capture the regular (first-segment) payment BEFORE the schedule walk: an
	// ARM's Re_Amortize mutates e.d to the later-segment payment, but the A-W9
	// implied-terminating-balloon advisory compares the FINAL row against the
	// original regular payment (see appendScheduleWarnings).
	regularPay := e.d
	rows := e.repayFancyLoan(&p, &usap, e.loan.LoanDate, e.loan.FirstDate, true, true, 0)

	// DOS's errorflag LATCH. Re_Amortize's two Iterate failure arms both do
	//
	//	abort := true;
	//	errorflag := true;
	//
	// (AMORTOP.pas:1526-1531 for the rate solve, :1583-1586 for the payment
	// solve) — and the Iterate that failed has ALREADY put
	// "Computation of payment amount or interest rate did not converge."
	// on the screen (AMORTOP.pas:1489). `abort` only ends the walk; it is
	// `errorflag` that condemns the RESULT: MakeTable is entered as
	// `Enter(no_tab); if (errorflag) then exit;` (Amortize.pas:1457-1458) and
	// Enter itself refuses to mark the screen computed under
	// `if (not errorflag) then screenstatus := screenstatus and computed`
	// (Amortize.pas:1441). So a schedule whose walk aborted at an unsolvable
	// adjustment is never shown — the user gets the message instead.
	//
	// The port had e.abort wired (dosport_walk.go breaks the walk on it) but
	// e.errorflag was written at those two sites and read NOWHERE, so
	// AmortizeDOS returned the TRUNCATED rows as a successful schedule: a loan
	// that stops dead at the adjustment date still holding a balance.
	//
	// 2026-07-25 fuzzer5 seed 8918 — verified against the real DOS engine:
	//
	//	amort_oracle 83886.76 0.1007380000 100 4 plusreg b96=24777.71 \
	//	  b105=19436.00 adj=132:0.0667770000: targ=492.24 pre=114:171:12:141.43
	//	  DOS:  ERR Computation of payment amount or interest rate did not converge.
	//	  port: err=<nil> rows=56 finalPrinc=32218.02
	//	        (truncated at the 1/1/2035 adjustment, 32218.02 still owed)
	//
	// Dropping ANY ONE of those six tokens makes the two engines agree, and
	// without the prepayment series the port already refuses correctly from
	// estimateAndRefinePayment — it is only when the payment solve SUCCEEDS and
	// the failure moves into the walk's Re_Amortize that the latch is the only
	// thing standing between a refusal and a bogus non-retiring schedule.
	if e.errorflag {
		return AmortResult{Err: errNoConverge}
	}

	var res AmortResult

	// ---- Prepaid interest collected at closing (non-in-advance) ----
	//
	// The schedule above starts at paidthru = max(loanDate, firstDate-1period);
	// the interest over the stub [loanDate, paidthru] is paid up front. On a
	// clean/short first period paidthru = loanDate so this is zero; on a LONG
	// first period it is the excess beyond the one period row 1 accrues.
	//
	// DOS does not merely add this to a total — it draws it as the settlement
	// LINE, payment number -1, at the loan date (Amortize.pas:1476-1491):
	//
	//	if ((prepaid) and (PrepaidInterest>0)) or
	//	   ((h^.pointsstatus>empty) and (h^.points<>0)) then
	//	  begin
	//	    ...
	//	    interest := PrepaidInterest + h^.points*h^.amount;
	//	    if hard_payment then Round2(interest);
	//	    ...
	//	  end;
	//
	// Two things follow, and the port used to get both wrong by bumping the
	// totals only:
	//
	//  1. The UI showed no settlement line at all on a long first stub, so the
	//     schedule silently failed to account for interest the totals included.
	//  2. applyPointsSettlement (engine.go:1504) keys off Schedule[0] being a
	//     PayNum-0 row at the loan date. With no such row it took the else
	//     branch and, seeing !hasRawSettlement, called PrepaidInterest and added
	//     the stub a SECOND time — a pure double count whenever `prepaid` and
	//     `points` were both live on a long first period. Fuzzer5 seed 30001
	//     (odd-first-period axis) surfaced five classes of this, all Go-high by
	//     exactly the stub: minimal repro
	//         100000 0.10 10 1 prepaid pts=0.02 targ=1 \
	//           loandmy=5.5.2024 firstdmy=5.2.2026
	//     gave dInt = 7500.00 = 100000 × 0.10 × 9/12, with the ten scheduled
	//     rows agreeing to the cent.
	//
	// So emit the line. The RAW (unrounded) value goes into rawSettlement so
	// that applyPointsSettlement can honour DOS's single rounding of the
	// COMBINED `PrepaidInterest + points*amount` rather than rounding the two
	// halves apart; the row itself is rounded only under a hard payment, which
	// is exactly the `if hard_payment then Round2(interest)` above.
	//
	// The `> 0` in DOS's test gates BLOCK ENTRY only (the body then adds the raw
	// value), so mirror it here as the emission gate: on the 360 basis YearsDif
	// is not a metric — the Feb clause at INTSUTIL.pas can return a small
	// NEGATIVE span — and DOS draws no line in that case.
	cumInt := 0.0
	if e.set.Prepaid && !e.set.InAdvance {
		if fp1, e1 := dateutil.AddPeriod(e.loan.FirstDate, e.loan.PerYr, e.loan.FirstDate.Time.Day(), true); e1 == nil &&
			dateutil.DateComp(fp1, e.loan.LoanDate) > 0 {
			ydif := dateutil.YearsDif(fp1, e.loan.LoanDate, e.set.Basis, e.set.YrInv, true)
			var pre float64
			if e.set.Daily {
				ev, _ := interest.Exxp(origTrueRate * ydif)
				pre = e.loan.Amount * (ev - 1)
			} else {
				pre = e.loan.Amount * origRate * ydif
			}
			if pre > 0 {
				line := pre
				if e.loan.PayAmtStatus == types.InOutInput {
					line = interest.Round2(pre)
				}
				cumInt = line
				res.Schedule = append(res.Schedule, PaymentRecord{
					PayNum:    0,
					Date:      e.loan.LoanDate,
					PayAmt:    line,
					Interest:  line,
					Principal: e.loan.Amount,
					IntToDate: cumInt,
				})
				res.TotalPaid += line
				res.TotalInt += line
				res.rawSettlement, res.hasRawSettlement = pre, true
			}
		}
	}

	for i, r := range rows {
		cumInt += r.interest
		res.Schedule = append(res.Schedule, PaymentRecord{
			PayNum:    i + 1,
			Date:      r.date,
			PayAmt:    r.payamt,
			Interest:  r.interest,
			Principal: r.principal,
			IntToDate: cumInt,
		})
		res.TotalPaid += r.payamt
		res.TotalInt += r.interest
	}

	// Guarded on `rows`, not on the Schedule: the Schedule may now carry a
	// leading PayNum-0 settlement line, and a settlement-only Schedule (no
	// scheduled payments at all) must leave FinalPrinc at zero rather than
	// report the settlement row's Principal, which is the ORIGINAL loan amount.
	if len(rows) > 0 {
		res.FinalPrinc = res.Schedule[len(res.Schedule)-1].Principal
	}
	res.NPeriods = e.loan.NPeriods
	res.FirstDate = e.loan.FirstDate
	res.LastDate = e.loan.LastDate
	// Task #94: Re_Amortize's h^.lastdate snap is a displayed output cell, so it
	// travels out on its own field — Amortize re-echoes derivedLastDate over
	// res.LastDate, and only the reAmortLastDate override beats that.
	res.reAmortLastDate = e.reAmortLastSnap
	if e.unkPre > 0 {
		res.SolvedPrepay = e.pres[e.unkPre].payment
	}

	// Advisory layer — reproduce the production Amortize's post-schedule passes so
	// the two engines emit identical advisories (Amortize is the reference; the
	// DOS oracle has no notion of these Go-port advisories). Use the ORIGINAL loan
	// fields (origRate / input.Loan): an ARM mutates e.loan.LoanRate mid-walk, but
	// the advisories describe what the user entered.
	// Early-payoff advisory (engine.go:1795-1800): when the payment over-amortizes
	// the loan so it retires BEFORE its scheduled term, the production engine warns.
	// The port's RepayFancyLoan stops on payoff, so the schedule has fewer than
	// NPeriods rows and ends on a retired balance. Emitted FIRST to match the
	// production ordering (it is appended during schedule generation, ahead of the
	// post-schedule passes).
	if n := len(res.Schedule); n > 0 {
		lastPayNum := res.Schedule[n-1].PayNum
		if lastPayNum < e.loan.NPeriods && math.Abs(res.Schedule[n-1].Principal) < minPmt {
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"Loan retired early — paid off at payment %d of a scheduled %d.",
				lastPayNum, e.loan.NPeriods))
		}
	}

	origLoan := input.Loan
	origLoan.LoanRate = origRate
	appendScheduleWarnings(&res, regularPay, origLoan.LoanRateStatus, origLoan.LoanRate)

	// Echo the balloons the engine used so the UI can fill any solved Amount cell.
	// A date-only "target" balloon (AO2) had its amount SOLVED — report the solved
	// value (matched by date in the engine's balloon array) with Solved=true.
	for i := range input.Balloons {
		b := input.Balloons[i]
		if b.DateStatus < types.InOutDefault || !dateutil.DateOK(b.Date) {
			continue
		}
		amount, solved := b.Amount, b.AmountStatus == types.InOutOutput
		if b.AmountStatus < types.InOutDefault { // the unknown target balloon
			for j := 1; j <= e.nballoons; j++ {
				if dateutil.DateComp(e.balloons[j].date, b.Date) == 0 {
					amount, solved = e.balloons[j].amount, true
					break
				}
			}
		}
		res.Balloons = append(res.Balloons, ResolvedBalloon{Date: b.Date, Amount: amount, Solved: solved})
	}

	// Result-sanity advisories (A-W4/5/6/7/11). The port solves only the payment —
	// A solved target balloon (AO2) has been written back to input.Balloons with
	// AmountStatus=Output, so A-W4/A-W5 read it; prepaySolved/prepaySolvedAmt carry
	// the AO9 solved prepayment for A-W7. payWasInput reflects a user-entered payment.
	payWasInput := input.Loan.PayAmtStatus >= types.InOutDefault
	appendResultAdvisories(&res, &input, &origLoan, prepaySolvedAmt, prepaySolved, payWasInput)
	return res
}
