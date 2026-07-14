package amortization

import (
	"math"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// Fancy backward solving via DOS's single Newton refinement (AMORTOP.pas:1415
// `Iterate`), ported here as dosIterate + the per-unknown terminals below.
//
// DOS solves an amount, rate, or payment — under balloons, prepayments, rate
// adjustments, moratoria, and exact interest — with ONE finite-difference secant
// that drives the schedule's *unforced* terminal balance to zero, differing only
// in which `var x` it perturbs. That is exactly what the dosIterate* family does:
// dosIteratePayment / dosIterateSimplePayment (var x = payment), dosIterateAmount
// (var x = h^.amount), dosIterateRate (var x = loanrate). Each evaluates the same
// UNFORCED terminal DOS's Iterate does (RepayFancyLoan / RepayLoan with no forced
// final payment and no early payoff stop), so the solved value matches the DOS
// engine rather than approximating it.
//
// (A prior implementation bisected the over/under-amortization SIGN of the forced
// display schedule. That sign test was a Go invention — DOS has no bisection
// anywhere — so it was removed in favour of this faithful port of Iterate.)

// repayExactTerminal runs the actual-day (exact) schedule for a trial payment x
// over the FULL term and returns the unforced terminal balance — a continuous,
// monotonic function of x (positive ⇒ still owes, negative ⇒ overpaid). It is the
// Go analogue of DOS's RepayFancyLoan used inside Iterate: it does NOT stop early
// or force the final payment, so an overpayment drives the balance negative
// exactly as DOS does (`if p < 0 then p := p - d`, AMORTOP.pas RepayLoan). This
// continuity is what lets the secant in dosIteratePayment converge like DOS;
// reconstructing the residual from the forced/early-stopping display schedule is
// discontinuous and makes the secant misbehave on long terms.
//
// Scope: exact loans (the path dosIteratePayment serves) — ordinary (in-arrears)
// in the main loop, and in-advance (annuity-due) via the settlement-shifted
// early-return branch below.
func repayExactTerminal(input LoanInput, x float64) float64 {
	loan := input.Loan
	s := &input.Settings
	p := loan.Amount
	origDay := loan.FirstDate.Time.Day()
	if s.InAdvance {
		// Exact (true-daily) in-advance: DOS shifts the base date one period later
		// (AMORTOP.pas:1159-1177) and amortizes over n-1 rows starting at firstDate
		// + 1 period, each accruing actual-day interest on the shifted period; the
		// time-0 settlement interest is collected at closing and does not change the
		// balance. The continuous (unforced) terminal balance after the n-1 rows is
		// monotone in x — the criterion dosIteratePayment/DOS's Iterate drives to
		// zero. See docs/exact_groundzero_findings.md "Exact × in-advance structure".
		prev := loan.FirstDate
		cur := loan.FirstDate
		for k := 1; k < loan.NPeriods; k++ {
			nd, err := dateutil.AddPeriod(cur, loan.PerYr, origDay, false)
			if err != nil {
				break
			}
			cur = nd
			if p < 0 {
				// Overpaid: DOS subtracts the payment with no further interest.
				p = p - x
			} else {
				yd := periodYearFraction(prev, cur, loan.PerYr, s)
				p = p + loan.LoanRate*yd*p - x
			}
			prev = cur
		}
		return p
	}
	prevDate := loan.LoanDate
	// Prepaid: the settlement stub (loanDate → natural period start) is collected
	// at closing, so the regular schedule's first period is a full period from the
	// natural start, not the odd loanDate→firstDate span. Mirrors the schedule's
	// row-0 stub (engine.go generateFancySchedule prepaid branch).
	if s.Prepaid && !s.InAdvance {
		if naturalStart, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, true); err == nil &&
			dateutil.DateComp(loan.LoanDate, naturalStart) <= 0 {
			prevDate = naturalStart
		}
	}
	curDate := loan.FirstDate
	for i := 0; i < loan.NPeriods; i++ {
		if p < 0 {
			// Overpaid: DOS subtracts the payment with no further interest.
			p = p - x
		} else {
			yd := periodYearFraction(prevDate, curDate, loan.PerYr, s)
			p = p + loan.LoanRate*yd*p - x
		}
		prevDate = curDate
		nd, err := dateutil.AddPeriod(curDate, loan.PerYr, origDay, false)
		if err != nil {
			break
		}
		curDate = nd
	}
	return p
}

// dosIterate is the general form of DOS's Newton/secant refinement
// (AMORTOP.pas:1415 `Iterate`): it drives the schedule's UNFORCED terminal
// balance to zero by finite-difference secant steps over a single scalar
// unknown, converging when the residual is under half a penny (or after 20
// iterations, keeping the best estimate seen). `terminal(v)` returns that
// balance for the trial value v (>0 under-amortized, <0 over-amortized), and
// `accInit` is DOS's `init` for the relative acceptance clause — always the
// loan amount (the starting balance `p` at entry to Iterate).
//
// DOS's single Iterate solves the payment, the loan amount, and the interest
// rate — it differs only in which `var x` it perturbs (AMORTOP.pas:1437 dispatch
// picks the terminal; :1483 `p:=x` for the amount target). Extracting the secant
// here lets the payment/amount/rate solvers share exactly that one refinement,
// step-for-step (divergence brake, bestx-after-update timing, dual acceptance).
//
// Ported from legacy/src/dos_source/AMORTOP.pas: function Iterate.
func dosIterate(seed, accInit float64, terminal func(float64) float64) (float64, bool) {
	const (
		small     = 0.001
		halfpenny = 0.005
		teeny2    = 1e-10
		accLimit  = 2e-8 // DOS acc_limit (AMORTOP.pas:1423)
	)
	if seed == 0 {
		return 0, false
	}
	// DOS's Iterate accepts a result unless BOTH bestp > halfpenny AND
	// bestp > acc_limit*init (AMORTOP.pas:1489), where init is the starting
	// balance p — the loan amount. i.e. it also accepts a RELATIVE residual of
	// 2e-8 × amount. On very large / very steep terminals (e.g. a 573-period 29%
	// exact loan, whose overpay balance reaches billions), the absolute half-penny
	// is unreachable in float64 but the relative tolerance (~$0.04 on a $2.16M
	// loan) is met — so this clause is what lets the Newton converge there.
	accTol := accLimit * math.Abs(accInit)
	x := seed
	final := terminal(x)
	if math.Abs(final) < halfpenny {
		return x, true
	}
	delta := small * x
	x += delta
	bestp := math.Inf(1)
	bestx := x
	count := 0
	for {
		p := terminal(x)
		var newdelta float64
		if math.Abs(final-p) > teeny2 {
			newdelta = delta * p / (final - p)
		}
		if math.Abs(delta) < teeny2 || math.Abs(newdelta/delta) > 1 {
			count += 5
		}
		delta = newdelta
		x += delta
		final = p
		if math.Abs(p) < bestp {
			bestp = math.Abs(p)
			bestx = x // DOS assigns bestx AFTER the x update (bug-for-bug faithful)
		}
		count++
		if count >= 20 || bestp < halfpenny {
			break
		}
	}
	return bestx, bestp < halfpenny || bestp <= accTol
}

// paymentTerminal selects the terminal procedure exactly as DOS's Iterate does
// (AMORTOP.pas:1437): RepayFancyLoan — the option-aware walk, evaluated UNFORCED
// via fancyTerminal — for `fancy OR (exact and non-360)`; otherwise the simple
// recursion (repayExactTerminal, the RepayLoan analogue) for a plain 360/365 loan.
//
// EXACT loans MUST use fancyTerminal, not repayExactTerminal: the display engine
// (generateFancyScheduleMode) reproduces DOS's actual-day per-period interest to
// the cent (verified by per-period trace), whereas repayExactTerminal's simplified
// recursion is ~250x less steep in the overpay region on long high-rate exact
// loans — its terminal zero is offset and its slope wrong, so the secant diverged
// (that was the exact-long-term case that needed the bisection fallback). Routing
// exact through fancyTerminal makes the Newton use the same DOS-faithful terminal
// the display does. In-advance exact keeps repayExactTerminal (fancyTerminal's
// early-return path is the forced settlement-shift generator).
func paymentTerminal(input LoanInput) func(float64) float64 {
	useFancy := hasAnyAdvancedOption(input) ||
		(exactDaily(&input.Settings) && !input.Settings.InAdvance)
	if !useFancy {
		return func(v float64) float64 { return repayExactTerminal(input, v) }
	}
	s := &input.Settings
	tr, _ := ComputeTrueRate(&input.Loan, s)
	fg := GrowthPerPeriod(&input.Loan, s.YrInv)
	return func(v float64) float64 { return fancyTerminal(input, v, s, tr, fg) }
}

// dosIteratePayment solves the regular payment via DOS's Iterate with `var x = d`
// (AMORTOP.pas:416). It seeds from the closed-form estimate; if that seed diverges
// on an option loan it retries from DOS's own adjusted-principal seed
// (EstimateAndRefinePayment), which converges to the SAME deterministic terminal
// root — so it cannot pick a different answer than DOS.
//
// Ported from legacy/src/dos_source/AMORTOP.pas: function Iterate.
func dosIteratePayment(input LoanInput, estimate float64) (float64, bool) {
	if estimate == 0 {
		return 0, false
	}
	accInit := input.Loan.Amount
	terminal := paymentTerminal(input)
	if r, ok := dosIterate(estimate, accInit, terminal); ok {
		return r, true
	}
	// Seed-fallback (still Newton, not bisection): DOS's Iterate seeds from the
	// non-prorated adjusted-principal annuity (EstimateAndRefinePayment). A prorated
	// or otherwise-offset estimate can make the secant diverge on a very steep
	// long-term balloon terminal; retry from DOS's own seed. Scoped to option loans.
	if hasAnyAdvancedOption(input) {
		fg := GrowthPerPeriod(&input.Loan, input.Settings.YrInv)
		adjpSeed := estimatePayment(&input.Loan, fg) * dosSeedPVFactor(input, &input.Loan, &input.Settings)
		if adjpSeed != estimate {
			if r, ok := dosIterate(adjpSeed, accInit, terminal); ok {
				return r, true
			}
		}
	}
	return dosIterate(estimate, accInit, terminal)
}

// dosIterateSimplePayment solves the regular payment for a PLAIN (non-fancy) loan
// whose closed form is inexact — an in-advance (annuity-due) loan, or one with an
// odd first period — via DOS's Iterate with the RepayLoan terminal (AMORTOP.pas:1437
// else-branch: `RepayLoan(p)`, NOT RepayFancyLoan). RepayLoan is DOS's exact
// recursion — basis-INDEPENDENT for in-advance (the annuity-due form, no day-count,
// no proration) and first-period-prorated for odd-first arrears — so driving its
// terminal to zero reproduces DOS's non-fancy payment directly, with no basis
// substitution. accInit = the loan amount (DOS's init = the starting balance p).
func dosIterateSimplePayment(input LoanInput, estimate float64) (float64, bool) {
	if estimate == 0 {
		return 0, false
	}
	loan := input.Loan
	s := input.Settings
	// Prepaid (non-in-advance) first-period handling, mirroring generateSimpleSchedule
	// (the row-validated DOS-faithful simple schedule) exactly. naturalStart =
	// firstDate − one period. DOS's global `prorate` is 1 ONLY when the loan is taken
	// MORE than one period before the first payment (loanDate < naturalStart): the
	// settlement stub loanDate→naturalStart is collected at closing and the first
	// regular period is a FULL period (Amortize.pas:1276-1283). When the loan is taken
	// WITHIN one period of the first payment (loanDate >= naturalStart), there is no
	// stub and the first period is the actual SHORT span loanDate→firstDate — prorate
	// = firstPeriodProrate, which Go's RepayLoan already computes. So shift the
	// effective loan date to naturalStart (⇒ prorate 1) ONLY in the stub case; leave
	// it otherwise. (An unconditional shift wrongly forced prorate 1 on short-first
	// prepaid loans and solved the payment high.)
	loan = prepaidNaturalStartShift(loan, &s)
	terminal := func(v float64) float64 {
		return RepayLoan(loan.Amount, v, &loan, &s, s.YrInv)
	}
	return dosIterate(estimate, loan.Amount, terminal)
}

// prepaidNaturalStartShift applies the prepaid (non-in-advance) first-period
// rule to a loan headed for the closed-form RepayLoan terminal: when the loan
// is taken MORE than one period before the first payment (loanDate <
// naturalStart = firstDate − one period), the settlement stub
// loanDate→naturalStart is collected at closing and the amortizing walk's
// first period is a FULL period — DOS's `repay_from := firstdate − 1 period;
// prorate := 1` (Amortize.pas:1277-1282). Shifting the effective loan date to
// naturalStart makes firstPeriodProrate return exactly 1. When the loan is
// taken WITHIN one period (short first), there is no stub and the short
// actual span stands — an unconditional shift solved short-first prepaid
// payments high (oracle-validated; see dosIterateSimplePayment's history).
//
// 2026-07-11 audit finding A6: the payment solve had this shift but the
// amount/rate solves did not, so prepaid odd-first loans solved ~2% off.
// Verified vs the real DOS engine:
//
//	amort_oracle 0 0.12 12 12 noamt pay=888.4879 prepaid first=3  → solvedamount 10000.000149
//	amort_oracle 10000 0 12 12 norate pay=888.4879 prepaid first=3 → solvedrate 0.1200000283
func prepaidNaturalStartShift(loan Loan, s *Settings) Loan {
	if s.Prepaid && !s.InAdvance && dateutil.DateOK(loan.FirstDate) {
		if ns, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, loan.FirstDate.Time.Day(), true); err == nil &&
			dateutil.DateComp(loan.LoanDate, ns) < 0 {
			loan.LoanDate = ns
		}
	}
	return loan
}

// dosIterateAmount solves the loan principal via DOS's Iterate with `var x =
// h^.amount` (AMORTOP.pas:460; the `p := x` at :1483 makes the starting balance
// track the trial amount). The terminal runs the UNFORCED schedule from the trial
// principal with the known payment/rate and returns the residual. Faithful analogue:
// fancyTerminal already starts its walk from in.Loan.Amount, so substituting the
// trial amount there is the same terminal DOS's RepayFancyLoan evaluates. accInit
// is the amount seed itself (DOS's init = the starting p = the trial amount).
func dosIterateAmount(input LoanInput, estimate float64) (float64, bool) {
	if estimate == 0 {
		return 0, false
	}
	s := &input.Settings
	tr, _ := ComputeTrueRate(&input.Loan, s)
	fg := GrowthPerPeriod(&input.Loan, s.YrInv)
	pay := input.Loan.PayAmt
	terminal := func(v float64) float64 {
		in := input
		in.Loan.AmountStatus = types.InOutInput
		in.Loan.Amount = v
		if exactInAdvanceUnforced(in) {
			return repayExactTerminal(in, pay)
		}
		if !hasFancyOptions(in) && !exactDaily(s) {
			// Plain (non-fancy, non-exact-daily) loan: its forward schedule IS the
			// closed RepayLoan recursion — the in-advance annuity-due branch, the
			// odd-first-period prorate, and the overpay branch all live there. Refine
			// against RepayLoan so the backward solve matches the exact forward
			// schedule the user sees. fancyTerminal's option-aware walk models those
			// cases differently (it diverges for in-advance and for day-count first
			// periods), so routing them through it would not round-trip. See
			// docs/postmortem_365_exact_interest.md §8.
			// Prepaid odd-first: same natural-start shift as the payment solve
			// (audit finding A6 — DOS prorate:=1, Amortize.pas:1277-1282).
			l := prepaidNaturalStartShift(in.Loan, s)
			return RepayLoan(v, pay, &l, s, s.YrInv)
		}
		return fancyTerminal(in, pay, s, tr, fg)
	}
	return dosIterate(estimate, estimate, terminal)
}

// dosIterateRate solves the loan rate via DOS's Iterate with `var x = h^.loanrate`
// (AMORTOP.pas:477). DOS recomputes `f := GrowthPerPeriod` at the top of each loop
// step (AMORTOP.pas:1451) because the rate is the target, so the terminal here
// recomputes true-rate/growth for every trial rate before running the UNFORCED
// schedule at the known amount/payment. accInit = the loan amount (DOS's init = the
// starting balance p, which is fixed for a rate solve).
func dosIterateRate(input LoanInput, estimate float64) (float64, bool) {
	if estimate == 0 {
		return 0, false
	}
	accInit := input.Loan.Amount
	pay := input.Loan.PayAmt
	terminal := func(v float64) float64 {
		in := input
		in.Loan.LoanRateStatus = types.InOutInput
		in.Loan.LoanRate = v
		s2 := in.Settings
		tr, _ := ComputeTrueRate(&in.Loan, &s2)
		fg := GrowthPerPeriod(&in.Loan, s2.YrInv)
		if exactInAdvanceUnforced(in) {
			return repayExactTerminal(in, pay)
		}
		if !hasFancyOptions(in) && !exactDaily(&s2) {
			// Plain loan: refine against the RepayLoan recursion, the forward
			// schedule's own terminal (see dosIterateAmount for the rationale).
			// Prepaid odd-first: natural-start shift (audit finding A6).
			l := prepaidNaturalStartShift(in.Loan, &s2)
			return RepayLoan(l.Amount, pay, &l, &s2, s2.YrInv)
		}
		return fancyTerminal(in, pay, &s2, tr, fg)
	}
	return dosIterate(estimate, accInit, terminal)
}

// exactInAdvanceUnforced reports whether this loan's UNFORCED terminal must be
// the settlement-shifted in-advance exact recursion (repayExactTerminal) rather
// than the option-aware fancyTerminal. It mirrors the predicate that
// generateFancyScheduleMode / paymentTerminal use to route exact × in-advance
// (no advanced options) to the dedicated settlement-shift generator: because
// that generator is a FORCED schedule (it folds the residual into the last row),
// fancyTerminal cannot supply the smooth unforced terminal the secant needs, so
// the amount/rate solvers evaluate the residual via repayExactTerminal instead.
// See paymentTerminal (fancybisect.go) and generateExactInAdvanceSchedule.
func exactInAdvanceUnforced(input LoanInput) bool {
	// SOLVE terminal gate: non-360 ONLY. DOS's Iterate routing is `fancy or
	// (exact and basis<>x360)` (AMORTOP.pas:1438) — an exact in-advance loan at
	// 360 solves its payment on the PLAIN annuity-due RepayLoan terminal, while
	// its DISPLAY takes the settlement-shift shape (pass-2 finding 5: DOS's
	// payment is identical exact-ON/OFF at 360, 3172.2326 — another
	// solve/display split, like the USA rule).
	return exactDaily(&input.Settings) && input.Settings.InAdvance &&
		!hasAnyAdvancedOption(input)
}

// (refineAdjustmentPayment was removed in the M1 step of the global-Iterate
// refactor — see docs/global_iterate_refactor.md. It bisected the adjustment
// payment against the ENTIRE schedule's terminal, which is ill-posed once a
// second ARM re-amortizes downstream, so it had to be gated to a single
// adjustment. solveSegmentPayment below replaces it with DOS's til_adj SEGMENT
// solve, which composes for any number of adjustments.)

// solveSegmentPayment solves the regular payment for the REMAINING SEGMENT of a
// schedule so it retires to zero, accounting for the balloons/prepayments/skip
// that lie ahead. It is the Go analogue of DOS's Re_Amortize calling
// Iterate(..., til_adj) (AMORTOP.pas:1571-1587 / 1415): that inner solve runs the
// schedule from the boundary to very_last WITHOUT re-amortizing at any LATER
// adjustment (adjnum=0 ⇒ Re_Amortize is never re-entered, AMORTOP.pas:1215) and
// WITHOUT the final-fold, driving just this segment's terminal to zero.
//
// Two callers use it, both passing the balance at a mid-schedule boundary:
//   - the MORATORIUM boundary (FirstRepay): the post-moratorium payment must
//     retire the remaining schedule like DOS's single solved payment
//     (docs/amort_option_combo_divergences.md §3); and
//   - each ARM adjustment (AO5): the segment payment after a rate reset, so a
//     loan with TWO+ ARMs composes correctly — each adjustment solves its own
//     segment independently, ignoring later adjustments, exactly as til_adj does
//     (the entire-schedule refineAdjustmentPayment was ill-posed with 2+ ARMs).
//
// It builds a sub-loan for the remaining term — balance `bal` amortized over
// `remaining` periods at the current rate, starting at firstPay with its prior
// period at prevDate — carrying the not-yet-applied balloons/prepayments (and
// skip months). The single regular period prevDate→firstPay reproduces the main
// schedule's first segment period exactly, so the solved payment is what the main
// schedule needs after the boundary. Returns ok=false (caller keeps the analytic
// seed) when there is nothing ahead to account for or the solve cannot bracket.
func solveSegmentPayment(input LoanInput, loan Loan, settings Settings,
	bal float64, prevDate, firstPay types.DateRec, remaining int, seed float64) (float64, bool) {
	if remaining <= 0 || bal <= 0 {
		return 0, false
	}
	// Only the balloons that still lie ahead of the boundary remain to be paid;
	// any balloon inside the moratorium has already reduced `bal`.
	var futureBalloons []BalloonPayment
	for _, b := range input.Balloons {
		if b.AmountStatus >= types.InOutDefault && math.Abs(b.Amount) > 0 &&
			dateutil.DateComp(b.Date, prevDate) > 0 {
			futureBalloons = append(futureBalloons, b)
		}
	}
	// A plain moratorium re-amortizes exactly with the analytic annuity seed.
	// Engage the schedule-oracle solve only when a DOWNSTREAM option changes the
	// REQUIRED post-moratorium payment: a later balloon/prepayment, or skipped
	// months (fewer paying periods ⇒ a higher retiring payment). A TARGET is
	// deliberately NOT included: DOS keeps the plain annuity for a target (the
	// target only bumps the individual periods that fall below it, never the base
	// solve), so a moratorium loan with a target retires at the SAME payment as a
	// plain moratorium. Folding the target into this solve perturbs the payment
	// even when it never binds — e.g. mor=74 + targ=61 on $261k/240: pure mor
	// solves DOS's 2297.73 to the cent, but adding the (inactive) target dropped
	// it to 2258.53 and lost ~$2,885 of interest. See
	// docs/amort_option_combo_divergences.md.
	hasSkip := anySkip(input.SkipMonths.MonthSet)
	// The EXACT method on a non-360 basis also invalidates the analytic seed:
	// the remaining segment accrues actual-day interest (DOS solves the
	// moratorium payment via Iterate over the exact RepayFancyLoan terminal,
	// Amortize.pas:416 + AMORTOP.pas:625). 2026-07-11 pass-2 finding 4 —
	// verified vs the real DOS engine:
	//
	//	amort_oracle 100000 0.10 24 12 b365 exact mor=6 → payment 5712.4662
	//	(the analytic seed left the exact-OFF 5712.6693; with DOS's payment the
	//	port's rows already matched to the cent — solve only)
	// Day-count frequencies (semimonthly/biweekly/weekly) on a non-360 basis
	// also invalidate the analytic seed: the rows accrue ACTUAL days (14/366 in
	// a leap year) while the annuity's constant f uses 14/365.25 — DOS's
	// Iterate drives the real actual-day walk. 2026-07-11 pass-2 finding 8 —
	// verified vs the real DOS engine:
	//
	//	amort_oracle 100000 0.10 52 26 b365 inadv mor=3 → payment 2375.0973
	//	(the analytic seed left 2375.3444; rows at DOS's payment already
	//	matched row-for-row, totals 11549.56 exact)
	dayCount := loan.PerYr == 24 || loan.PerYr == 26 || loan.PerYr == 52
	if len(futureBalloons) == 0 && len(input.Prepayments) == 0 && !hasSkip &&
		!exactDaily(&settings) &&
		!(dayCount && settings.Basis != types.Basis360) {
		return 0, false
	}
	// The sub-loan is a MID-LOAN segment: the in-advance settlement interest and
	// one-period base-date shift happened at the ORIGINAL loan date and must not
	// be re-applied here. DOS's til_adj Iterate walks ComputeNext, which accrues
	// ordinary opening-balance interest even when in_advance is set
	// (AMORTOP.pas:636) — the in_advance flag only shapes the loan's start, which
	// is already behind the boundary. Clearing it makes the sub-loan the plain
	// bounded segment DOS solves. (Pass-2 finding 8: with InAdvance carried in,
	// the sub-solve re-shifted the segment and gave 2423.3853 vs DOS 2375.0973.)
	segSettings := settings
	segSettings.InAdvance = false
	sub := LoanInput{
		Loan: Loan{
			AmountStatus:   types.InOutInput,
			Amount:         bal,
			LoanRateStatus: types.InOutInput,
			LoanRate:       loan.LoanRate,
			NStatus:        types.InOutInput,
			NPeriods:       remaining,
			PerYrStatus:    types.InOutInput,
			PerYr:          loan.PerYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput,
			LoanDate:       prevDate,
			FirstStatus:    types.InOutInput,
			FirstDate:      firstPay,
		},
		Balloons:    futureBalloons,
		Prepayments: input.Prepayments,
		Settings:    segSettings,
		Fancy:       true,
	}
	// Skip months are by calendar month, so they apply unchanged in the sub-loan.
	// (Target is intentionally omitted for the plain moratorium — see the gate
	// comment above; DOS solves the plain annuity and lets the per-period target
	// bump and the final-fold absorb any residual.)
	if hasSkip {
		sub.SkipMonths = input.SkipMonths
		// When a target ALSO binds, it converts the skipped months from
		// negative-amortizing (pay 0, balance grows) into target-floored rows
		// (pay interest + the minimum principal reduction), which LOWERS the
		// retiring payment. DOS's Iterate walks the full fancy schedule with both
		// skip and target applied (ComputeNext AMORTOP.pas:643 — target overrides
		// skip), so the segment solve must carry the target too, otherwise the
		// skip rows are solved as pure negative-am and the payment comes out at
		// the no-target value. Only threaded when skip is present: a skip-free
		// moratorium+target retires at the plain annuity (target never binds the
		// base solve) and is handled by the early gate return above. 2026-07-13
		// pass-4 P4-N1b — verified vs the real DOS engine:
		//
		//	amort_oracle 50000 0.164 24 12 inadv targ=0.01 skip=4-6 mor=3
		//	→ payment 3500.3264 (= the non-in-advance quad; the skip rows are
		//	  target-floored, not negative-am. Without the target the segment
		//	  solved 3706.5934 = the no-target inadv+skip+mor value.)
		if input.Target.TargetStatus >= types.InOutDefault {
			sub.Target = input.Target
		}
	}
	// DOS solves the segment payment with Iterate(..., til_adj) — the same Newton,
	// over RepayFancyLoan run only to the next boundary (AMORTOP.pas:1571-1587/1415).
	// The sub-loan IS that bounded segment as a standalone fancy loan, so
	// dosIteratePayment (Newton over the sub-loan's UNFORCED fancy terminal) drives
	// the identical residual to zero.
	if refined, ok := dosIteratePayment(sub, seed); ok && refined > 0 {
		return refined, true
	}
	return 0, false
}

// solveSegmentRate is the AO6 (payment-only / implied-rate adjustment) analog of
// solveSegmentPayment: it solves the RATE at which a KNOWN new payment amortizes
// the post-adjustment segment [adj -> last] to zero, walking the REAL actual-day
// fancy schedule instead of the uniform-period balanceAfterN.
//
// DOS's EstimateAndRefineAdjRate (Amortize.pas:347-368) solves this rate by
// calling RepayFancyLoan — the actual-day display walk — and driving its
// terminal balance to zero. On a day-count frequency (semimonthly/biweekly/
// weekly) at a non-360 basis, or the exact method, the display accrues ACTUAL
// days (e.g. a 14-day biweekly period vs the uniform 365.25/26 = 14.05) while
// balanceAfterN's constant GrowthPerPeriod does not — so the uniform solveAdjRate
// returns a slightly-off implied rate and the segment interest drifts (~0.08% on
// the biweekly N7 case; pass 6). The sub-loan IS that bounded segment as a
// standalone fancy loan with the new payment hard-set, so the secant over its
// UNFORCED fancy terminal reproduces DOS's Iterate. On the 360 basis uniform ==
// actual, so the caller keeps solveAdjRate there (this engages only on exact /
// day-count-non-360). Mirrors solveAdjRate's secant structure and |rate|<2 clamp.
func solveSegmentRate(input LoanInput, loan Loan, settings Settings,
	bal float64, prevDate, firstPay types.DateRec, remaining int, payment, seedRate float64) (float64, bool) {
	if remaining <= 0 || bal <= 0 || payment <= 0 {
		return 0, false
	}
	var futureBalloons []BalloonPayment
	for _, b := range input.Balloons {
		if b.AmountStatus >= types.InOutDefault && math.Abs(b.Amount) > 0 &&
			dateutil.DateComp(b.Date, prevDate) > 0 {
			futureBalloons = append(futureBalloons, b)
		}
	}
	// Mid-loan segment: the in-advance settlement/base-shift happened at the
	// original loan date and must not be re-applied (see solveSegmentPayment).
	segSettings := settings
	segSettings.InAdvance = false
	sub := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput,
			Amount:       bal,
			// NOT InOutInput: a hard payment triggers DOS's Dav-Holle per-period
			// interest rounding (cent-quantized terminal → the secant can't reach
			// the half-penny tolerance). The MAIN display doesn't round the segment
			// (its regular payment is solved, not user-hard), so the sub-loan must
			// not either — keep the terminal continuous so dosIterate converges.
			PayAmtStatus:   types.InOutDefault,
			PayAmt:         payment,
			NStatus:        types.InOutInput,
			NPeriods:       remaining,
			PerYrStatus:    types.InOutInput,
			PerYr:          loan.PerYr,
			LoanDateStatus: types.InOutInput,
			LoanDate:       prevDate,
			FirstStatus:    types.InOutInput,
			FirstDate:      firstPay,
		},
		Balloons:    futureBalloons,
		Prepayments: input.Prepayments,
		Settings:    segSettings,
		Fancy:       true,
	}
	if anySkip(input.SkipMonths.MonthSet) {
		sub.SkipMonths = input.SkipMonths
		if input.Target.TargetStatus >= types.InOutDefault {
			sub.Target = input.Target
		}
	}
	// generateFancyScheduleMode bounds the walk by LastDate; a solver-built
	// sub-loan hasn't run FirstPass, so derive it: FirstDate + (NPeriods-1)
	// periods (same as fancyTerminal).
	if dateutil.DateOK(sub.Loan.FirstDate) {
		day := sub.Loan.FirstDate.Time.Day()
		last := sub.Loan.FirstDate
		for k := 1; k < sub.Loan.NPeriods; k++ {
			if nd, e := dateutil.AddPeriod(last, sub.Loan.PerYr, day, false); e == nil {
				last = nd
			}
		}
		sub.Loan.LastDate = last
		sub.Loan.LastOK = true
	}
	// terminal(rate) = the unforced terminal balance of the segment at the trial
	// rate, paying the fixed new payment — the residual DOS's Iterate drives to
	// zero. Monotone increasing in rate (higher rate ⇒ slower paydown ⇒ higher
	// terminal).
	terminal := func(rate float64) float64 {
		s := sub
		s.Loan.LoanRateStatus = types.InOutInput
		s.Loan.LoanRate = rate
		tr, _ := ComputeTrueRate(&s.Loan, &segSettings)
		f := GrowthPerPeriod(&s.Loan, segSettings.YrInv)
		return generateFancyScheduleMode(s, payment, &segSettings, tr, f, true).FinalPrinc
	}
	// Seed from the uniform solveAdjRate answer (already near the implied rate).
	// The actual-day terminal is steep — at the original loan rate a big new
	// payment over-amortizes by hundreds of thousands, so a from-scratch secant
	// would not converge; the near-answer seed does. dosIterate is DOS's own
	// Newton (best-x tracking, divergence brake, half-penny / relative acceptance).
	if seedRate <= 0 {
		seedRate = loan.LoanRate
	}
	r, ok := dosIterate(seedRate, bal, terminal)
	if !ok || r < -1.9 || r > 1.9 {
		return 0, false
	}
	return r, true
}

// hasFancyOptions reports whether the loan carries any advanced option
// that makes the closed-form backward solve inexact: balloons, prepayment
// series, rate/payment adjustments, AND the schedule-shaping options —
// skip-months, moratorium, and target. DOS's Iterate always drives the
// FANCY terminal when any option is set (skip/mor/target all set `fancy`,
// and RepayFancyLoan's ComputeNext applies them per period —
// AMORTOP.pas:574-664, 1438/1464), so an amount/rate solve that ignores
// them returns the option-blind closed form.
//
// 2026-07-11 audit finding A5: this previously checked only
// prepayments/adjustments/balloons, so skip/mor/target-only loans solved
// up to 75% off. Verified vs the real DOS engine:
//
//	amort_oracle 0 0.12 24 12 noamt pay=888.4879 skip=6-8 → solvedamount 14134.974937
//	amort_oracle 0 0.12 24 12 noamt pay=500 mor=12        → solvedamount 6066.870036
func hasFancyOptions(input LoanInput) bool {
	if !input.Fancy {
		return false
	}
	if len(input.Prepayments) > 0 || len(input.Adjustments) > 0 {
		return true
	}
	if input.SkipMonths.SkipStatus >= types.InOutDefault && input.SkipMonths.SkipStr != "" {
		return true
	}
	if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
		return true
	}
	if input.Target.TargetStatus >= types.InOutDefault {
		return true
	}
	for _, b := range input.Balloons {
		// Presence by STATUS, not value: DOS sets `fancy := true` for any
		// entered balloon row, and a $0 balloon in REPLACE mode is a real
		// skipped installment (pass-2 finding 9).
		if b.AmountStatus >= types.InOutDefault {
			return true
		}
	}
	return false
}
