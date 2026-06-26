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
		case b.AmountStatus >= types.InOutDefault && b.Amount != 0:
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
		// CheckPrepayments (AMORTOP.pas:416-422): when a count (NN) is given but no
		// stop date, derive stopdate = startdate + (NN-1) periods so the series runs
		// EXACTLY NN payments. Without this the series has no per-series bound and the
		// walk applies the prepayment every period to the end of the loan.
		if dp.nnOK && !dp.stopOK && dp.nn > 0 {
			sd := pp.StartDate
			for k := 1; k < dp.nn; k++ {
				nd, err := dateutil.AddPeriod(sd, pp.PerYr, pp.StartDate.Time.Day(), false)
				if err != nil {
					break
				}
				sd = nd
			}
			dp.stopdate, dp.stopOK = sd, true
			if dateutil.DateComp(sd, e.veryLast) > 0 {
				e.veryLast = sd
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

	e.f = GrowthPerPeriod(&e.loan, e.set.YrInv)
	e.truerate, _ = ComputeTrueRate(&e.loan, &e.set)
	return e
}

// estimateAndRefinePayment mirrors Amortize.pas:377-430: analytic annuity seed
// over the balloon-netted balance, then Iterate(til_adj) to refine. Sets e.d.
func (e *dosEng) estimateAndRefinePayment() bool {
	p := e.loan.Amount
	usap := 0.0
	adjp := e.loan.Amount
	rate, _ := interest.RateFromYield(e.loan.LoanRate, byte(e.loan.PerYr), e.set.YrDays)
	for i := 1; i <= e.userNballoons; i++ {
		yd := dateutil.YearsDif(e.balloons[i].date, e.loan.LoanDate, e.set.Basis, e.set.YrInv, false)
		disc, _ := interest.Exxp(-rate * yd)
		adjp -= e.balloons[i].amount * disc
	}
	// (Prepayment seed terms omitted — Iterate refines; the fuzzer does not
	// generate prepayment series. TODO: port FirstLastAndFF for completeness.)
	e.d = annuityPayment(adjp, e.f, e.loan.NPeriods)
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
// inBackwardSolve is set while a production backward solver (SolveLoanAmount,
// SolveRate, SolveBalloonAmount, SolvePrepaymentAmount) is running its internal
// trial evaluations through Amortize. Those solvers were validated against the
// piecewise forward schedule, so their inner calls must stay on it — routing
// trials through the port could shift the converged result on edge inputs.
var inBackwardSolve bool

// beginBackwardSolve marks the start of a piecewise backward solve; the returned
// func restores the previous state. Usage: `defer beginBackwardSolve()()`.
func beginBackwardSolve() func() {
	prev := inBackwardSolve
	inBackwardSolve = true
	return func() { inBackwardSolve = prev }
}

func dosPortCanHandle(in LoanInput, loan Loan, s *Settings) bool {
	if !dosPortEnabled || !in.Fancy || inBackwardSolve {
		return false
	}
	// Degenerate term beyond the schedule safety bound — the piecewise engine has
	// the explicit 10000-period error; keep it there.
	if loan.NPeriods > MaxSchedulePeriods {
		return false
	}
	if s.InAdvance || s.R78 || s.Exact || s.Daily {
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
		if b.AmountStatus >= types.InOutDefault && b.Amount != 0 {
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

	var res AmortResult
	cumInt := 0.0
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
	// Prepaid interest collected at closing (non-in-advance). The schedule above
	// starts at paidthru = max(loanDate, firstDate-1period); the interest over the
	// stub [loanDate, paidthru] is paid up front and DOS INCLUDES it in the
	// reported total. On a clean/short first period paidthru = loanDate so this is
	// zero; on a LONG first period it is the excess beyond one period (verified vs
	// oracle: e.g. annual loan, 16-month first, prepaid — DOS total = schedule +
	// rate·4/12·amount).
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
			res.TotalInt += pre
			res.TotalPaid += pre
		}
	}

	if len(res.Schedule) > 0 {
		res.FinalPrinc = res.Schedule[len(res.Schedule)-1].Principal
	}
	res.NPeriods = e.loan.NPeriods
	res.FirstDate = e.loan.FirstDate
	res.LastDate = e.loan.LastDate
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
