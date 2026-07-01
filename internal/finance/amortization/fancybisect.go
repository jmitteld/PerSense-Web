package amortization

import (
	"math"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// Fancy backward solving by bisection on the over/under-amortization sign.
//
// DOS solves an amount/rate/payment under balloons, prepayments, and rate
// adjustments with a finite-difference Newton refinement (AMORTOP.pas
// Iterate) against the schedule's *unforced* terminal balance. The Go
// forward engine instead forces the final payment to retire the loan and
// stops early on payoff, so that unforced residual is not directly
// observable and a Newton step on it is discontinuous — which is why the
// earlier Newton refinement failed to converge for prepayment series and
// why SolvePaymentClosedForm ignored balloons entirely.
//
// This solves the same problem more robustly without touching the forward
// engine: the *sign* of "does the loan still owe at the scheduled end
// (under-amortized) vs. retire early / fall short (over-amortized)" is
// monotonic in each unknown and changes exactly at the solution. Bisecting
// that sign converges for any fancy schedule the forward engine can run,
// using Amortize itself as the oracle.

const fancyBisectTol = 5e-4 // absolute tolerance on the solved field

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

// dosIteratePayment solves the regular payment with a faithful port of DOS's
// Newton/secant refinement (AMORTOP.pas:1415 `Iterate`): it drives the
// schedule's terminal balance to zero by finite-difference secant steps,
// converging when the residual is under half a penny (or after 20 iterations,
// keeping the best estimate seen). Replicated step-for-step — including the
// divergence brake and DOS's bestx-after-update timing — so the solved payment
// matches the DOS engine rather than merely approximating it.
//
// Ported from legacy/src/dos_source/AMORTOP.pas: function Iterate.
func dosIteratePayment(input LoanInput, estimate float64) (float64, bool) {
	const (
		small     = 0.001
		halfpenny = 0.005
		teeny2    = 1e-10
	)
	if estimate == 0 {
		return 0, false
	}
	x := estimate
	final := repayExactTerminal(input, x)
	if math.Abs(final) < halfpenny {
		return x, true
	}
	delta := small * x
	x += delta
	bestp := math.Inf(1)
	bestx := x
	count := 0
	for {
		p := repayExactTerminal(input, x)
		var newdelta float64
		if math.Abs(final-p) > teeny2 {
			newdelta = delta * p / (final - p)
		}
		// Divergence brake (AMORTOP.pas:1474): if the step is not shrinking,
		// short-circuit toward the iteration cap.
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
	return bestx, bestp < halfpenny
}

// fancyOverUnder reports whether a fully-specified trial loan
// under-amortizes (+1: still owes principal at the scheduled end),
// over-amortizes (-1: retires early, or the final payment falls short of
// a regular one), or amortizes essentially exactly (0). The "regular
// payment" it compares against is in.Loan.PayAmt — which is the known
// payment for amount/rate solves and the trial payment for a payment
// solve, so the same test serves all three.
func fancyOverUnder(in LoanInput) int {
	res := Amortize(in)
	if res.Err != nil || len(res.Schedule) == 0 {
		return 0 // can't evaluate — treat as solved to stop the search
	}
	if len(res.Schedule) < in.Loan.NPeriods {
		return -1 // retired early ⇒ over-amortized
	}
	// Ran the full term. The unforced terminal balance is the residual we
	// want. Two regimes:
	//   - large leftover: the engine does NOT force the final payment, so
	//     FinalPrinc carries the remaining balance and the last payment is
	//     the regular one (lastPay == d).
	//   - small leftover: the engine forces the final regular payment to
	//     clear the balance, leaving FinalPrinc == 0 but lastPay == d + the
	//     amount cleared.
	// FinalPrinc + (lastPay − d), with the last-row correction applied only
	// when the last row is a regular payment, covers both. Positive ⇒ still
	// owed ⇒ under-amortized.
	resid := res.FinalPrinc
	// Use the last ACTUAL payment row for the forced-final-payment correction,
	// skipping trailing zero-payment skip-month rows. A skipped month emits a
	// row with PayAmt 0, so applying `last.PayAmt - in.Loan.PayAmt` to the
	// literal last row would subtract a full regular payment that never
	// happened — which made the skip-month payment solve land ~$1 low and leave
	// a residual when the skip set includes the final period (e.g. skip=1-3,7,
	// where the loan's last row is a skipped January).
	// Find the last REGULAR-payment row for the forced-final correction. Always
	// skip trailing zero-payment skip-month rows. For loans with a PREPAYMENT
	// series, ALSO skip trailing rows whose PayAmt is below the regular payment:
	// when PlusRegular is off a prepayment REPLACES the regular payment, so the
	// trailing rows carry the small prepay amount, not d. Applying the
	// `last.PayAmt - d` correction to such a row subtracts a full regular payment
	// that never happened — for an NN-derived prepayment series that retires the
	// loan on its trailing extras that drove the residual hugely negative and the
	// bisection could never bracket the (large) regular payment. This extra skip
	// is scoped to prepayment loans so it cannot perturb the balloon / skip /
	// moratorium / plain in-advance solves (a forced final regular payment carries
	// PayAmt >= d, and for those loans the last row IS that regular payment).
	hasPre := false
	for i := range in.Prepayments {
		if in.Prepayments[i].PaymentStatus >= types.InOutDefault {
			hasPre = true
			break
		}
	}
	li := len(res.Schedule) - 1
	for li > 0 && (res.Schedule[li].PayAmt == 0 ||
		(hasPre && res.Schedule[li].PayAmt < in.Loan.PayAmt-fancyBisectTol)) {
		li--
	}
	last := res.Schedule[li]
	if last.PayNum >= 1 && last.PayNum <= in.Loan.NPeriods {
		resid += last.PayAmt - in.Loan.PayAmt
	}
	switch {
	case resid > fancyBisectTol:
		return 1
	case resid < -fancyBisectTol:
		return -1
	default:
		return 0
	}
}

// fancyBisect finds x in [minX, maxX] where sign(x) == 0, expanding the
// initial [lo, hi] bracket outward (clamped to the [minX, maxX] domain)
// until it straddles a sign change, then bisecting to tol. Returns the
// solved x and whether it converged. If no sign change exists within the
// domain it returns (0, false) so the caller can fall back to its
// closed-form estimate rather than a runaway value.
func fancyBisect(sign func(float64) int, lo, hi, minX, maxX, tol float64) (float64, bool) {
	if lo < minX {
		lo = minX
	}
	if hi > maxX {
		hi = maxX
	}
	sLo := sign(lo)
	if sLo == 0 {
		return lo, true
	}
	sHi := sign(hi)
	if sHi == 0 {
		return hi, true
	}
	for tries := 0; tries < 50 && sLo == sHi; tries++ {
		span := hi - lo
		nlo := lo - span
		if nlo < minX {
			nlo = minX
		}
		nhi := hi + span
		if nhi > maxX {
			nhi = maxX
		}
		if nlo == lo && nhi == hi {
			break // bracket already spans the whole domain
		}
		lo, hi = nlo, nhi
		sLo = sign(lo)
		if sLo == 0 {
			return lo, true
		}
		sHi = sign(hi)
		if sHi == 0 {
			return hi, true
		}
	}
	if sLo == sHi {
		return 0, false // no sign change in the domain
	}
	for i := 0; i < 100 && hi-lo > tol; i++ {
		mid := 0.5 * (lo + hi)
		sMid := sign(mid)
		if sMid == 0 {
			return mid, true
		}
		if sMid == sLo {
			lo = mid
		} else {
			hi = mid
		}
	}
	return 0.5 * (lo + hi), true
}

// solveFancyAmount refines a candidate loan principal so the fancy
// schedule amortizes exactly. payment and rate are taken as known.
func solveFancyAmount(input LoanInput, estimate float64) (float64, bool) {
	base := input
	base.Loan.AmountStatus = types.InOutInput
	base.Loan.LoanRateStatus = types.InOutInput // keep the known rate honest for Amortize
	sign := func(x float64) int {
		in := base
		in.Loan.Amount = x
		return fancyOverUnder(in)
	}
	lo, hi := 0.5*estimate, 1.5*estimate
	if estimate <= 0 {
		lo, hi = 1, 1e7
	}
	return fancyBisect(sign, lo, hi, fancyBisectTol, 1e10, fancyBisectTol)
}

// solveFancyRate refines a candidate loan rate so the fancy schedule
// amortizes exactly. amount and payment are known.
func solveFancyRate(input LoanInput, estimate float64) (float64, bool) {
	base := input
	base.Loan.AmountStatus = types.InOutInput
	base.Loan.LoanRateStatus = types.InOutInput
	sign := func(x float64) int {
		in := base
		in.Loan.LoanRate = x
		return fancyOverUnder(in)
	}
	lo, hi := 0.5*estimate, 1.5*estimate
	if estimate <= 0 {
		lo, hi = 1e-4, 1.0
	}
	// Cap the rate domain at 200% annual; beyond that there is no sensible
	// loan rate, so report non-convergence and let the caller fall back to
	// its closed-form estimate rather than chasing a runaway value.
	return fancyBisect(sign, lo, hi, 1e-6, 2.0, 1e-7)
}

// solveFancyPayment refines a candidate regular payment so the fancy
// schedule (balloons, prepayments, adjustments) amortizes exactly. amount
// and rate are known. This is the path that previously did not exist —
// SolvePaymentClosedForm returned the no-balloon closed form for fancy loans.
func solveFancyPayment(input LoanInput, estimate float64) (float64, bool) {
	base := input
	base.Loan.AmountStatus = types.InOutInput
	base.Loan.LoanRateStatus = types.InOutInput
	sign := func(x float64) int {
		in := base
		in.Loan.PayAmtStatus = types.InOutInput
		in.Loan.PayAmt = x
		return fancyOverUnder(in)
	}
	lo, hi := 0.5*estimate, 1.5*estimate
	if estimate <= 0 {
		lo, hi = 1, 1e7
	}
	return fancyBisect(sign, lo, hi, fancyBisectTol, 1e9, fancyBisectTol)
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
	if len(futureBalloons) == 0 && len(input.Prepayments) == 0 && !hasSkip {
		return 0, false
	}
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
		Settings:    settings,
		Fancy:       true,
	}
	// Skip months are by calendar month, so they apply unchanged in the sub-loan.
	// (Target is intentionally omitted — see the gate comment above; DOS solves
	// the plain annuity and lets the per-period target bump and the final-fold
	// absorb any residual.)
	if hasSkip {
		sub.SkipMonths = input.SkipMonths
	}
	if refined, ok := solveFancyPayment(sub, seed); ok && refined > 0 {
		return refined, true
	}
	return 0, false
}

// hasFancyOptions reports whether the loan carries any advanced option
// that makes the closed-form backward solve inexact (balloons, prepayment
// series, or rate/payment adjustments).
func hasFancyOptions(input LoanInput) bool {
	if !input.Fancy {
		return false
	}
	if len(input.Prepayments) > 0 || len(input.Adjustments) > 0 {
		return true
	}
	for _, b := range input.Balloons {
		if b.AmountStatus >= types.InOutDefault && math.Abs(b.Amount) > 0 {
			return true
		}
	}
	return false
}
