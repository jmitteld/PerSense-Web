// Backward solve paths for amortization: solve for loan amount or rate
// when one of those is left blank but enough other data is on the screen.
//
// DOS dispatch is in Amortize.pas: function ComputeLoanAmount,
// EstimateAndRefineLoanAmount, EstimateAndRefineRate.
//
// Ported from legacy/src/dos_source/Amortize.pas.

package amortization

import (
	"fmt"
	"math"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// CanComputeLoanAmount mirrors DOS function ComputeLoanAmount at
// Amortize.pas:853-858. Returns true when peryr, loanrate, payamt and
// firstdate are all defined and amount is missing.
//
// Ported from legacy/src/dos_source/Amortize.pas: function ComputeLoanAmount.
func CanComputeLoanAmount(loan *Loan) bool {
	return loan.PerYrStatus >= types.InOutDefault &&
		loan.LoanRateStatus >= types.InOutDefault &&
		loan.PayAmtStatus >= types.InOutDefault &&
		loan.FirstStatus >= types.InOutDefault &&
		loan.AmountStatus < types.InOutDefault
}

// CanComputeRate is the symmetric guard: amount, payment, term known
// but rate is missing.
func CanComputeRate(loan *Loan) bool {
	return loan.AmountStatus >= types.InOutDefault &&
		loan.PayAmtStatus >= types.InOutDefault &&
		loan.NStatus >= types.InOutDefault &&
		loan.LoanRateStatus < types.InOutDefault
}

// CanComputePayment is the symmetric guard: amount, rate, term known
// but payment is missing.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefinePayment's pre-check.
func CanComputePayment(loan *Loan) bool {
	return loan.AmountStatus >= types.InOutDefault &&
		loan.LoanRateStatus >= types.InOutDefault &&
		loan.NStatus >= types.InOutDefault &&
		loan.PerYrStatus >= types.InOutDefault &&
		loan.PayAmtStatus < types.InOutDefault
}

// SolvePaymentClosedForm computes the periodic payment amount from amount + rate
// + term using the closed-form annuity formula:
//
//	d = amount * (f - 1) / (1 - 1/f^n)
//
// where f = GrowthPerPeriod. This mirrors DOS
// EstimateAndRefinePayment's fast-path at Amortize.pas:377-430 — the
// closed-form direct assignment that applies when no fancy features
// (prepayments, balloons, adjustments, in_advance, target, skip-months)
// are active.
//
// SCOPE / WARNING: this is the closed-form ESTIMATE only. It is exact for an
// ordinary loan (in-arrears, 30/360 or 365), including an odd first period,
// but it does NOT apply the DOS engine's iterate-refinement (dosIteratePayment
// / the schedule-oracle bisection). For in-advance (annuity-due) and exact
// (actual/365 daily) loans, and for any loan carrying balloons / adjustments /
// prepayments / targets / skip-months, the DOS *engine* refines this estimate
// further, so this function's result is NOT DOS-engine-faithful for those cases
// (the in-advance value can differ by tens of percent). It does match the
// independent Pascal closed form (refdata.pas) — see TestCrossCheckInAdvance.
//
// To get the DOS-engine-faithful payment for ANY loan, call Amortize with a
// blank PayAmt (PayAmtStatus = StatusEmpty); that is the path the API and all
// production code use. This function has no production callers and exists as a
// closed-form reference/building block for tests and the higher-level solvers.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefinePayment.
func SolvePaymentClosedForm(input LoanInput) (float64, error) {
	loan := input.Loan
	settings := input.Settings

	if !CanComputePayment(&loan) {
		return 0, fmt.Errorf("Pmt Amount cannot be solved yet. To solve the payment, " +
			"leave Pmt Amount blank and fill in Amount Borrowed, Loan Rate, " +
			"# Periods and Pmts/Yr.")
	}
	if loan.NPeriods <= 0 {
		return 0, fmt.Errorf("# Periods is blank or zero, so the payment cannot be " +
			"solved. Enter # Periods, or supply 1st Pmt Date and Last Pmt Date so " +
			"Per%%Sense can derive the term.")
	}
	if math.Abs(loan.Amount) < tiny {
		return 0, fmt.Errorf("Amount Borrowed is zero, so there is no payment to " +
			"solve. Enter the loan principal in Amount Borrowed.")
	}

	f := GrowthPerPeriod(&loan, settings.YrInv)
	// Special case: zero rate — even split.
	if math.Abs(f-1) < tiny {
		return loan.Amount / float64(loan.NPeriods), nil
	}

	lnf, err := interest.Lnn(f)
	if err != nil {
		return 0, err
	}
	expVal, err := interest.Exxp(-float64(loan.NPeriods) * lnf)
	if err != nil {
		return 0, err
	}
	denom := 1 - expVal
	if math.Abs(denom) < tiny {
		return 0, fmt.Errorf("The payment cannot be solved with these terms — the " +
			"interest factor is too small to compute. Check Loan Rate, # Periods and " +
			"Pmts/Yr for values that are unusually small or large.")
	}
	pay := loan.Amount * (f - 1) / denom
	// First-period proration. The standard annuity above assumes every period,
	// including the first, is a full period. When the first payment is not
	// exactly one period after the loan date — a short/long odd first stub, or
	// (on the actual/365 basis) any month whose real day count differs from one
	// even period — DOS solves a payment that accounts for the prorated
	// first-period interest. Scaling the closed-form payment by ffFirst/f
	// reproduces that; it is exactly 1.0 for the common firstDate = loanDate +
	// one full period case, so ordinary 30/360 loans are unchanged.
	// First-period proration applies only to DOS's ARREARS branch of RepayLoan
	// (AMORTOP.pas:1284). The IN-ADVANCE branch (AMORTOP.pas:1276-1279) is a flat
	// annuity-due recursion with NO proration, so the in-advance payment is
	// basis-independent. Gate on !InAdvance to match (see docs/ui_sweep_findings.md
	// #A); the in-advance `pay /= f` below then gives DOS's basis-independent value.
	if !settings.InAdvance && dateutil.DateOK(loan.LoanDate) && dateutil.DateOK(loan.FirstDate) {
		ydif := dateutil.YearsDif(loan.FirstDate, loan.LoanDate,
			settings.Basis, settings.YrInv, true)
		if prorate := ydif * float64(loan.PerYr); prorate > 0 {
			ffFirst := 1 + (f-1)*prorate
			pay *= ffFirst / f
		}
	}
	if settings.InAdvance {
		// In-advance (annuity-due): payments fall at the START of each
		// period, so the payment is the in-arrears payment discounted by
		// one period's growth. DOS EstimateAndRefinePayment never takes
		// the early closed-form exit for in_advance — Amortize.pas:402-407
		// gates that exit on `not df.c.in_advance` — and instead
		// Iterate-refines, which for a simple loan converges to d/f.
		pay /= f
	}
	// Fancy schedules (balloons, prepayments, adjustments) and exact non-360
	// interest have no closed form — DOS refines the closed-form seed with its
	// Newton/secant Iterate (AMORTOP.pas:1415) against the schedule's UNFORCED
	// terminal balance. Route both through dosIteratePayment, the port of that
	// single refinement, so payment / amount / rate solves all drive the same
	// DOS-faithful terminal (a shared root ⇒ self-consistent round-trips).
	if exactDaily(&settings) && !settings.InAdvance {
		// Exact (true-daily) in-arrears loan.
		// (Exact × in-advance is an open frontier — see engine.go / findings doc.)
		in := input
		in.Fancy = true
		if refined, ok := dosIteratePayment(in, pay); ok && refined > 0 {
			return refined, nil
		}
	} else if hasFancyOptions(input) || exactDaily(&settings) {
		if refined, ok := dosIteratePayment(input, pay); ok && refined > 0 {
			return refined, nil
		}
	}
	return pay, nil
}

// SolveLoanAmount computes the loan principal from payment + rate +
// term (+ optional balloons), using the closed-form annuity formula:
//
//	amount = (1 - 1/f^n) / (f - 1) * d + Σ balloon[i]*exp(-rate * yrsDif)
//
// where f = GrowthPerPeriod and d = payment per period. Mirrors DOS
// EstimateAndRefineLoanAmount at Amortize.pas:432-465 (without the
// Iterate-refinement step, which only matters for prepayment series
// and adjustments — those still require the fancy engine).
//
// For fancy schedules (balloons, prepayments, adjustments) the closed
// form is only a first estimate; dosIterateAmount (fancybisect.go) then
// refines it with DOS's Newton Iterate (var x = h^.amount, AMORTOP.pas:460)
// over the schedule's unforced terminal balance.
//
// The second return value is a convergence flag: true when the
// closed-form solve was sufficient or the Newton converged and
// converged; false when the Newton did not converge, in
// which case the caller surfaces a "did not converge" warning to the
// user (matching the DOS MessageBox).
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefineLoanAmount + AMORTOP.pas: function Iterate.
func SolveLoanAmount(input LoanInput) (float64, bool, error) {
	defer beginBackwardSolve()()
	loan := input.Loan
	settings := input.Settings

	if !CanComputeLoanAmount(&loan) {
		return 0, false, fmt.Errorf("Amount Borrowed cannot be solved yet. To solve " +
			"the loan amount, leave Amount Borrowed blank and fill in Loan Rate, " +
			"Pmt Amount, Pmts/Yr and 1st Pmt Date.")
	}

	// C-A-10: DOS Amortize.pas:Enter rejects "solve for loan amount"
	// when fancy mode is active AND a target principal-reduction is
	// in force. The two constraints over-determine the system because
	// target requires a known principal to enforce a per-period floor.
	if input.Fancy && input.Target.TargetStatus >= types.InOutDefault &&
		input.Target.TargetValue > 0 {
		return 0, false, fmt.Errorf("Amount Borrowed cannot be solved while a " +
			"principal reduction Target is set — the Target needs a known loan " +
			"amount to work from. Clear the Target, or enter Amount Borrowed " +
			"directly.")
	}

	f := GrowthPerPeriod(&loan, settings.YrInv)
	if math.Abs(f-1) < tiny {
		return 0, false, fmt.Errorf("Amount Borrowed cannot be solved because Loan " +
			"Rate is effectively zero. Enter a non-zero Loan Rate, or enter Amount " +
			"Borrowed directly.")
	}
	if loan.NPeriods <= 0 {
		return 0, false, fmt.Errorf("# Periods is blank or zero, so Amount Borrowed " +
			"cannot be solved. Enter # Periods, or supply 1st Pmt Date and Last Pmt " +
			"Date.")
	}

	rate, err := interest.RateFromYield(loan.LoanRate, byte(loan.PerYr), settings.YrDays)
	if err != nil {
		return 0, false, err
	}

	d := loan.PayAmt
	repayFrom := loan.LoanDate
	var padj float64
	for _, b := range input.Balloons {
		if b.AmountStatus < types.InOutDefault || b.DateStatus < types.InOutDefault {
			continue
		}
		yrsDif := dateutil.YearsDif(b.Date, repayFrom, settings.Basis, settings.YrInv, true)
		expVal, err := interest.Exxp(-rate * yrsDif)
		if err != nil {
			return 0, false, err
		}
		padj += b.Amount * expVal
	}

	lnf, err := interest.Lnn(f)
	if err != nil {
		return 0, false, err
	}
	expVal, err := interest.Exxp(-float64(loan.NPeriods) * lnf)
	if err != nil {
		return 0, false, err
	}
	numerator := 1 - expVal
	estimate := numerator/(f-1)*d + padj

	// Schedule-oracle refinement. The closed-form annuity above assumes a
	// uniform per-period growth factor. That is exact only for the DOS
	// "fast path"; in every other case the real schedule (and DOS) differ
	// from it and the estimate must be refined against the actual schedule
	// terminal (dosIterateAmount drives DOS's Newton Iterate to a zero
	// terminal balance). needScheduleRefine mirrors DOS's exit test at
	// Amortize.pas:459 exactly: DOS returns the closed form WITHOUT Iterate
	// only when ((basis=360) OR !exact) AND prepaid AND no balloons AND no
	// prepayments AND !in_advance; otherwise it refines. The previously
	// missing triggers were the Exact method on a non-360 basis (actual-day
	// accrual — a $100k/12%/360-pmt/365-basis loan solved ~$23 low) and the
	// odd first period of a !prepaid loan. See
	// docs/postmortem_365_exact_interest.md.
	if needScheduleRefine(input) {
		refined, ok := dosIterateAmount(input, estimate)
		if ok && refined > 0 {
			return refined, true, nil
		}
		// The Newton did not converge; return the closed-form estimate with
		// converged=false so the handler surfaces a "did not converge" warning
		// (matching DOS's MessageBox at AMORTOP.pas:1489).
		return estimate, false, nil
	}
	return estimate, true, nil
}

// needScheduleRefine reports whether a backward Amount/Rate solve must refine
// its closed-form estimate against the real schedule (DOS's Iterate), rather
// than returning the closed form directly.
//
// DOS's fast-path exit in EstimateAndRefineLoanAmount (Amortize.pas:459) returns
// the closed form WITHOUT Iterate only when
//
//	((basis=x360) OR !exact) AND prepaid AND nballoons=0 AND npre=0 AND !in_advance
//
// i.e. DOS refines whenever the loan is exact-on-a-non-360-basis, in advance, not
// prepaid, or carries advanced options. This mirrors that: refine unless the loan
// is the plain, prepaid, full-first-period, arrears, non-exact case whose closed
// form is already exact. The trigger set is:
//
//   - hasFancyOptions — balloons / prepayments / adjustments (refined via the
//     fancy schedule terminal).
//   - exactDaily — the Exact method on a non-360 basis (actual-day accrual; the
//     originally reported $100k/12%/365 gap). Refined via the exact terminal.
//   - InAdvance — annuity-due timing: the closed-form annuity assumes payments in
//     arrears. Refined via RepayLoan's in-advance branch (dosIterateAmount).
//   - oddFirstPeriod — the first payment is not exactly one period after the loan
//     date (a short/long stub, or a mid-month loan date on a day-count basis), so
//     the closed form's full-first-period assumption is wrong. Refined via
//     RepayLoan's prorate.
//
// The InAdvance/oddFirstPeriod triggers are exactly the forward payment-solve's
// own shortcut predicate (needPaymentRefine): the backward solve must refine iff
// the forward refined, so the two invert the SAME schedule. In particular
// needPaymentRefine is false when Rule-of-78 is active — R78 uses a whole-period
// sum-of-digits split with the plain in-arrears payment, which the backward CLOSED
// FORM already matches — so the backward solve must NOT refine R78 either (routing
// it through RepayLoan's in-advance/prorate branch would break the symmetry). For
// every refined non-fancy, non-exactDaily case the terminal is RepayLoan, the SAME
// recursion the plain forward schedule uses, so the solved Amount/Rate round-trips
// with the forward schedule the user sees (verified to ~1e-13). The one residual
// is a !prepaid FULL-first-period loan on a non-360 basis, where RepayLoan's
// whole-month prorate and the forward day-count first period differ by ~1e-4 (a
// bounded, documented frontier — docs/postmortem_365_exact_interest.md §8; down
// from ~0.6% before this change).
func needScheduleRefine(input LoanInput) bool {
	s := &input.Settings
	// DOS's amount-solve shortcut (Amortize.pas:458-459) returns the closed
	// form WITHOUT Iterate only when
	//
	//	((basis=x360) or (not exact)) and prepaid and (nballoons=0)
	//	and (npre=0) and (not in_advance)
	//
	// — i.e. it ALWAYS Iterates for a NON-PREPAID loan (and the rate solve
	// always Iterates, Amortize.pas:467-491). 2026-07-11 pass-2 finding 3: the
	// previous gate skipped refinement for plain non-prepaid loans, leaving the
	// uniform-growth closed form, which on a non-360 basis misses the
	// actual-day first-period prorate DOS's RepayLoan terminal carries
	// (`amort_oracle 120000 0.11 36 12 b365 noamt pay=3929.2311` →
	// solvedamount 120000.0012; the unrefined port returned 120017.87).
	if hasFancyOptions(input) || exactDaily(s) || s.InAdvance {
		return true
	}
	if !s.Prepaid {
		return true
	}
	// Prepaid, option-free, arrears, (360 or non-exact): DOS's fast path.
	return needPaymentRefine(&input.Loan, s)
}

// SolveRate computes the loan rate from amount + payment + term via
// Newton iteration. First guess is payamt * peryr / amount, clamped
// to >= 0.02 since the iteration won't progress from zero.
//
// Mirrors DOS EstimateAndRefineRate at Amortize.pas:467-491 (the
// Iterate step is replaced here with a direct Newton loop on the
// closed-form RepayLoan residual, which is sufficient for plain loans
// without prepays or adjustments).
//
// For fancy schedules (balloons, prepayments, adjustments),
// dosIterateRate (fancybisect.go) refines the closed-form estimate with
// DOS's Newton Iterate (var x = loanrate, AMORTOP.pas:477) over the
// schedule's unforced terminal after the closed-form Newton loop converges.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefineRate + AMORTOP.pas: function Iterate.
func SolveRate(input LoanInput) (float64, bool, error) {
	defer beginBackwardSolve()()
	loan := input.Loan
	settings := input.Settings
	if !CanComputeRate(&loan) {
		return 0, false, fmt.Errorf("Loan Rate cannot be solved yet. To solve the " +
			"rate, leave Loan Rate blank and fill in Amount Borrowed, Pmt Amount, " +
			"# Periods and Pmts/Yr.")
	}
	if math.Abs(loan.Amount) < tiny {
		return 0, false, fmt.Errorf("Amount Borrowed is zero, so there is no rate to " +
			"solve. Enter the loan principal in Amount Borrowed.")
	}

	rate := loan.PayAmt * float64(loan.PerYr) / loan.Amount
	if rate < 0.02 {
		rate = 0.02
	}
	// DOS seeds its rate Iterate with exactly this first guess and lets the secant
	// converge or DIVERGE from there (Amortize.pas:9-10 `loanrate := payamt*peryr/
	// amount; if <0.02 then 0.02`). Go's closed-form pre-solve below always finds the
	// root, so refining dosIterateRate from that pre-solved rate hid DOS's non-
	// convergence: when the payment is far below principal/term the implied rate is
	// deeply negative, and DOS's secant — starting at the floored +2% — diverges
	// ("Computation of payment amount or interest rate did not converge.") where Go
	// silently landed the root. Refine from the DOS seed so we diverge where DOS does.
	dosSeed := rate

	// Newton-style iteration: residual = RepayLoan(amount, payment) at
	// candidate rate. Want residual ≈ 0 (loan paid off exactly at
	// term).
	const maxIter = 30
	delta := small
	loan.LoanRate = rate
	residual0 := RepayLoan(loan.Amount, loan.PayAmt, &loan, &settings, settings.YrInv)
	loan.LoanRate = rate + delta
	for count := 0; count < maxIter; count++ {
		residual := RepayLoan(loan.Amount, loan.PayAmt, &loan, &settings, settings.YrInv)
		denom := residual - residual0
		var step float64
		if math.Abs(denom) > teeny {
			step = -residual * delta / denom
		} else {
			step = small
		}
		residual0 = residual
		rate += step
		delta = step
		// DOS's Iterate does NOT clamp the rate to positive — it lets the Newton
		// run into negative territory and only bails when |rate| exceeds 2 (±200%,
		// AMORTOP.pas:1485 `until ... or (abs(h^.loanrate) > 2)`). A loan whose
		// payments total less than the principal (e.g. 120×$750 < $100,000) has a
		// genuinely NEGATIVE implied rate; the old `if rate < 0 { rate = small }`
		// clamp pinned the solve at ~0 and reported non-convergence for exactly
		// those loans. Keep DOS's divergence bound instead of the positive clamp.
		if rate > 2 || rate < -2 {
			break
		}
		loan.LoanRate = rate
		if math.Abs(step) < teeny {
			// Closed-form converged. Refine against the real schedule for
			// the same cases DOS does (needScheduleRefine, mirroring DOS's
			// EstimateAndRefineRate, which always Iterates): the closed-form
			// RepayLoan residual uses a uniform per-period growth factor, so
			// for the Exact method on a non-360 basis (actual-day accrual), a
			// !prepaid odd first period, in-advance timing, or any advanced
			// option, the rate it lands on can be off. dosIterateRate refines
			// against the real schedule (symmetric with SolveLoanAmount; see
			// docs/postmortem_365_exact_interest.md).
			if needScheduleRefine(input) {
				refined, ok := dosIterateRate(input, dosSeed)
				// Accept a converged refinement even when it is NEGATIVE (an
				// under-funded loan): DOS returns the negative rate here. The old
				// `refined > 0` guard discarded a correct negative root and fell
				// through to the non-converged warning. Bound it by DOS's |rate|<=2.
				if ok && refined != 0 && refined > -2 && refined < 2 {
					return refined, true, nil
				}
				// The Newton did not converge; return the closed-form rate with
				// converged=false so the handler surfaces a "did not converge"
				// warning (matching DOS's MessageBox at AMORTOP.pas:1489).
				return rate, false, nil
			}
			// Plain path: the closed form is exact for this loan, but DOS's rate
			// solve is ALWAYS its Iterate (seeded at dosSeed, Amortize.pas:11). Gate
			// convergence on whether that Iterate converges too, so a degenerate
			// under-funded loan (payment far below principal/term → a deeply negative
			// implied rate DOS's secant can't reach) refuses exactly as DOS does even
			// on the plain path. The returned rate stays the exact closed form.
			if _, ok := dosIterateRate(input, dosSeed); !ok {
				return rate, false, nil
			}
			return rate, true, nil
		}
	}
	// DOS refuses a rate solve that runs past ±200%: Iterate's stop condition
	// `until (count >= 20) or (bestp < halfpenny) or (abs(h^.loanrate) > 2)`
	// (AMORTOP.pas:1485) followed by the "did not converge" MessageBox
	// (AMORTOP.pas:1489). 2026-07-11 audit finding A10 — the port previously
	// returned the out-of-range rate with converged=false; now it refuses with
	// DOS's message. Verified: `amort_oracle 10000 0 12 12 norate pay=2500` →
	// "ERR Computation of payment amount or interest rate did not converge."
	if rate > 2 || rate < -2 {
		return 0, false, fmt.Errorf("Computation of payment amount or interest " +
			"rate did not converge. The payment implies a rate beyond ±200%% — " +
			"check Pmt Amount and Amount Borrowed.")
	}
	return rate, false, nil
}

// solveNPeriodsFromPayment derives the number of payment periods from
// a known regular payment amount, for a simple (non-fancy) loan.
//
// Closed form (AMORTOP.pas:1382-1397, DetermineLastPaymentDate, the
// "not fancy" branch):
//
//	p1 := p*ff - d           {principal after the first payment}
//	ff := 1/f
//	n  := round(1.4999 + ln(1 - p1*(1-ff)/(ff*d)) / ln(ff))
//
// where f = GrowthPerPeriod and ff on the first line is the
// first-period growth factor 1+(f-1)*prorate. The +1.4999 rounds up
// and accounts for the first period being separated out. When the
// rate is effectively zero (ff≈1) the term is the straight-line
// p1/d. The payment must exceed the first period's interest or the
// loan never amortizes — DOS aborts with "payment too small".
//
// Ported from legacy/src/dos_source/AMORTOP.pas: function
// DetermineLastPaymentDate (non-fancy branch).
func solveNPeriodsFromPayment(loan *Loan, settings *Settings, f float64) (int, error) {
	d := loan.PayAmt
	p := loan.Amount
	if d <= 0 {
		return 0, fmt.Errorf("Pmt Amount is blank, so the number of periods cannot " +
			"be derived. Enter Pmt Amount, or enter # Periods directly.")
	}
	// Payment must beat the first period's interest (DOS guard,
	// AMORTOP.pas:1385: d*peryr < 1.001*p*loanrate).
	if d*float64(loan.PerYr) < 1.001*p*loan.LoanRate {
		return 0, fmt.Errorf(
			"Pmt Amount is too small to pay off the loan — it does not even cover " +
				"the interest, so the loan would never amortize. Raise the Pmt Amount, " +
				"or enter # Periods directly.")
	}

	// First-period prorate: fraction of a full period between the
	// loan date and the first payment date (1.0 for the common case
	// of firstDate = loanDate + one period).
	//
	// Prepaid (non-in-advance): DOS pins the solve's prorate to 1 when the
	// settlement stub absorbs the odd first gap (repay_from := firstdate − 1
	// period; prorate := 1 — Amortize.pas:1277-1282; the global prorate feeds
	// the term solve at AMORTOP.pas:1388). 2026-07-11 audit finding A8:
	// without this the term solve ran one period long on prepaid odd-first
	// loans. Verified vs the real DOS engine:
	//
	//	amort_oracle 10000 0.12 0 12 noterm pay=888.4879 prepaid first=3 → solvedterm 12 last 2025-3-1
	//
	// (was n=13 / 2025-04-01). The shift applies only in the stub case
	// (loan taken more than one period before firstDate), mirroring
	// prepaidNaturalStartShift in fancybisect.go.
	prorate := 1.0
	if dateutil.DateOK(loan.LoanDate) && dateutil.DateOK(loan.FirstDate) {
		ydif := dateutil.YearsDif(loan.FirstDate, loan.LoanDate,
			settings.Basis, settings.YrInv, true)
		if pr := ydif * float64(loan.PerYr); pr > 0 {
			prorate = pr
		}
	}
	// Prepaid pins the prorate to EXACTLY 1 (Amortize.pas:1277-1282 — the
	// same global that feeds this closed form at AMORTOP.pas:1388), not to
	// the shifted natural-period YearsDif, which is 1 only on the 360 basis
	// (a leap January month is 31/365.25·12 ≈ 1.0164; a biweekly period is
	// 14/365.25·26 ≈ 0.9966). The old prepaidNaturalStartShift approximation
	// crossed the round(1.4999+…) boundary. 2026-07-12 pass-3 finding P3-F2 —
	// verified vs the real DOS engine:
	//
	//	amort_oracle 10000 0.09 0 12 b365 prepaid payhard=456.85 noterm
	//	→ solvedterm 24 last 2026-1-1 (Go reported 25 / 2026-2-1)
	//	amort_oracle 10000 0.09 0 26 b365 prepaid payhard=210.40 noterm
	//	→ solvedterm 53 last 2026-1-12 (Go reported 52 / 2025-12-29)
	//
	// The pin applies exactly when prepaid survives DOS's short-first
	// clearing (loan on or before the natural period start) — the same gate
	// as RepayLoan's pass-2 F6 pin.
	if settings.Prepaid && !settings.InAdvance &&
		dateutil.DateOK(loan.LoanDate) && dateutil.DateOK(loan.FirstDate) {
		if ns, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr,
			loan.FirstDate.Time.Day(), true); err == nil &&
			dateutil.DateComp(loan.LoanDate, ns) <= 0 {
			prorate = 1
		}
	}

	ffFirst := 1 + (f-1)*prorate
	p1 := p*ffFirst - d // principal remaining after the first payment
	if p1 <= 0 {
		// First payment alone clears the loan.
		return 1, nil
	}
	ff := 1 / f
	var n int
	if math.Abs(1-ff) < teeny {
		n = int(math.Round(1.4999 + p1/d))
	} else {
		arg := 1 - p1*(1-ff)/(ff*d)
		ln1, err := interest.Lnn(arg)
		if err != nil {
			return 0, fmt.Errorf(
				"Pmt Amount is too small to pay off the loan — it does not even cover " +
					"the interest, so the loan would never amortize. Raise the Pmt " +
					"Amount, or enter # Periods directly.")
		}
		ln2, err := interest.Lnn(ff)
		if err != nil {
			return 0, err
		}
		n = int(math.Round(1.4999 + ln1/ln2))
	}
	if n < 1 {
		// (coverage: excluded — defensive/unreachable: the payment-beats-interest
		// guard above (d*peryr >= 1.001*p*loanrate) plus the +1.4999 round-up keep
		// n >= 1 for every input that reaches here; this guards a future formula
		// change.)
		return 0, fmt.Errorf("The Pmt Amount does not produce a valid loan term. " +
			"Check the Pmt Amount and Loan Rate, or enter # Periods directly.")
	}
	return n, nil
}

// aprValueCashflows builds the payment stream DOS discounts to solve the APR
// (Amortize.pas:553-556). DOS runs RepayFancyLoan in value_calc mode — the
// FULL-TERM walk — whenever the loan is fancy, uses the exact method, or sits on
// a non-360 basis; only a plain 30/360 loan takes the closed-form
// CalculateValueForPlainLoan, for which the truncated display schedule already is
// the faithful stream. In the full-term case the DISPLAY schedule is wrong for the
// APR: it stops at early payoff, whereas DOS keeps discounting the regular payment
// across all N periods (the balance over-amortizing negative) and tacks on the
// terminal balance as a balloon (AMORTOP.pas:1224-1225). Discounting the truncated
// stream under-counts the tail and skews the APR — negligibly for arrears (its
// truncated tail nearly coincides with the full-term one) but materially for
// in-advance loans, up to whole percentage points on a 360 basis. This rebuilds
// the full-term stream: every regular payment plus a terminal balloon row carrying
// the over-amortized final balance at the last payment date.
func aprValueCashflows(input LoanInput, payment float64, settings *Settings, truerate, f float64, display []PaymentRecord) []PaymentRecord {
	fullTerm := input.Fancy || hasAnyAdvancedOption(input) || settings.Exact ||
		settings.Basis != types.Basis360
	if !fullTerm {
		// Plain 30/360 loan: DOS discounts via the closed-form annuity
		// CalculateValueForPlainLoan (Amortize.pas:493) — N EQUAL regular payments
		// from firstDate to lastDate with NO terminal balloon; it does not model the
		// early payoff of an over-specified payment. On a 360 basis every period is
		// exactly 1/perYr year, so discounting N discrete payments at the period
		// dates equals that geometric sum. The truncated display schedule (which
		// stops at early payoff) is NOT that stream, so build the full annuity here.
		loan := input.Loan
		cf := make([]PaymentRecord, 0, loan.NPeriods)
		dt := loan.FirstDate
		origDay := loan.FirstDate.Time.Day()
		for k := 0; k < loan.NPeriods; k++ {
			cf = append(cf, PaymentRecord{PayNum: 1, Date: dt, PayAmt: payment})
			nd, err := dateutil.AddPeriod(dt, loan.PerYr, origDay, false)
			if err != nil {
				return display
			}
			dt = nd
		}
		return cf
	}
	vs := generateFancyScheduleMode(input, payment, settings, truerate, f, true)
	if vs.Err != nil || len(vs.Schedule) == 0 {
		return display
	}
	cf := make([]PaymentRecord, 0, len(vs.Schedule)+1)
	var lastDate types.DateRec
	haveLast := false
	for i := range vs.Schedule {
		if vs.Schedule[i].PayNum >= 1 {
			cf = append(cf, vs.Schedule[i])
			lastDate = vs.Schedule[i].Date
			haveLast = true
		}
	}
	if haveLast {
		// DOS value_calc discounts NextPayment.principal (the over-amortized final
		// balance) at the last payment date as a terminating balloon.
		cf = append(cf, PaymentRecord{PayNum: 1, Date: lastDate, PayAmt: vs.FinalPrinc})
	}
	return cf
}

// ComputeAPRWithPoints computes the loan's annual percentage rate
// when the borrower paid discount points. The APR is the rate at
// which the present value of the scheduled payments equals the
// borrower's net proceeds (loan amount, less points, less any
// prepaid interest collected at closing).
//
// The schedule's settlement-stub row (PayNum 0) is excluded from the
// discounted payment stream because the prepaid interest it
// represents is already netted out of netProceeds — matching DOS,
// which discounts only the regular NextPayment stream and subtracts
// PrepaidInterest from the target.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefineAPRwithPoints (secant iteration, 20 passes).
func ComputeAPRWithPoints(schedule []PaymentRecord, loanDate types.DateRec,
	netProceeds, firstGuess float64, perYr byte, settings *Settings) (apr float64, converged bool) {

	const small = 0.0001

	// value discounts every regular payment back to the loan date at
	// the trial rate vr.
	value := func(vr float64) float64 {
		var sum float64
		for _, row := range schedule {
			if row.PayNum < 1 {
				continue // skip the settlement-stub row
			}
			yd := dateutil.YearsDif(row.Date, loanDate, settings.Basis, settings.YrInv, true)
			ev, err := interest.Exxp(-vr * yd)
			if err != nil {
				continue
			}
			sum += row.PayAmt * ev
		}
		return sum
	}

	vRate := firstGuess
	if vRate <= 0 {
		vRate = 0.1
	}
	oldValue := value(vRate)
	delta := small
	vRate += delta
	for count := 0; count < 20; count++ {
		v := value(vRate)
		denom := v - oldValue
		if math.Abs(denom) > teeny {
			delta = (netProceeds - v) * delta / denom
		} else {
			delta = small
		}
		oldValue = v
		vRate += delta
		if math.Abs(delta) < teeny {
			converged = true
			break
		}
	}
	yld, err := interest.YieldFromRate(vRate, perYr, settings.YrDays)
	if err != nil {
		return 0, false
	}
	return yld, converged
}

// SolveBalloonAmount solves the amount of the balloon at unknownIdx so
// that the schedule's final balance lands at zero — the DOS "target
// balloon". For a balloon on the last payment date this is just the
// outstanding balance; for an intermediate date the balloon changes
// the subsequent amortization, so the amount is found by a secant
// iteration over the final-balance function.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefineBalloon.
func SolveBalloonAmount(input LoanInput, unknownIdx int) (float64, error) {
	defer beginBackwardSolve()()
	// DOS refuses an unknown balloon when a coexisting adjustment row is not
	// FULLY specified (date+rate+amount): SufficientDataOnScreen requires
	// adj_fully_specified (Amortize.pas:889-890 + AMORTOP.pas:393). With a
	// rate-only ARM the post-adjustment payment re-solve retires the loan for
	// ANY balloon, so the residual is degenerate and the "solved" value is
	// arbitrary. 2026-07-12 pass-3 finding P3-F3 — verified vs the real DOS
	// engine:
	//
	//	amort_oracle 100000 0.08 120 12 adj=24:0.09: payhard=1050 solveballoon=60
	//	→ refused (balloon 0.0000 status 0; Go returned 50000.0000 = the
	//	  half-amount seed). Fully-specified control: adj=24:0.09:1100 →
	//	  20696.5200, which the port matches.
	if adjRowsNotFullySpecified(input.Adjustments) {
		return 0, fmt.Errorf("A Balloon amount cannot be solved while a Rate " +
			"Adjustment row is incomplete. Fill in the date, rate AND payment " +
			"on every Adjustment row, or clear the incomplete row.")
	}
	// eval runs the full schedule with the unknown balloon pinned to
	// amt and returns the residual final balance.
	eval := func(amt float64) (float64, error) {
		clone := input
		bs := make([]BalloonPayment, len(input.Balloons))
		copy(bs, input.Balloons)
		bs[unknownIdx].Amount = amt
		bs[unknownIdx].AmountStatus = types.InOutInput
		clone.Balloons = bs
		// DOS EstimateAndRefineBalloon has TWO paths (Amortize.pas:628-663):
		//
		//   - balloon ON very_last (a TERMINATING balloon): closed form from the
		//     schedule run at balloon=0 — OUTSIDE Iterate, so `hard_payment`
		//     stays as supplied and the walk's per-row Round2 applies. The
		//     −300757.72 over-funded golden (TestSolveBalloonAmountOverFundedNegative)
		//     comes from this ROUNDED walk.
		//   - balloon MID-TERM: Iterate, which runs its solve walks with
		//     hard_payment temporarily OFF (AMORTOP.pas:1433 `hard_payment :=
		//     false; {temporarily, for iteration}`) — Round2 is display-only.
		//     Solving against the rounded walk quantizes the terminal (~2¢) and
		//     shifts the root: with payment 1110.21 on the audit loan the
		//     balloon solved 1109.6863 while DOS solves 1109.6700
		//     (`amort_oracle 100000 0.06 120 12 payhard=1110.21 solveballoon=37`).
		//
		// Evaluate the UNFORCED full-term terminal (DOS RepayFancyLoan Output=nil),
		// NOT Amortize's truncated display schedule. An over-amortizing loan retires
		// early in the display, so a balloon at/after that point leaves the truncated
		// FinalPrinc ~0 and the secant returns 0 — while DOS runs the full term (the
		// balance over-amortizes negative) and solves the balloon, often NEGATIVE,
		// that lands the terminal balance on zero. Mirror DOS EstimateAndRefineBalloon.
		s := clone.Settings
		tr, _ := ComputeTrueRate(&clone.Loan, &s)
		fg := GrowthPerPeriod(&clone.Loan, s.YrInv)
		if !clone.Loan.LastOK && clone.Loan.NPeriods > 0 && dateutil.DateOK(clone.Loan.FirstDate) {
			day := clone.Loan.FirstDate.Time.Day()
			last := clone.Loan.FirstDate
			for k := 1; k < clone.Loan.NPeriods; k++ {
				if nd, e := dateutil.AddPeriod(last, clone.Loan.PerYr, day, false); e == nil {
					last = nd
				}
			}
			clone.Loan.LastDate = last
			clone.Loan.LastOK = true
		}
		// Blank the hard status (the value still drives the walk, exactly like
		// fancyTerminal) ONLY for the mid-term/Iterate case — a balloon dated
		// before the last regular payment. Must run AFTER the LastDate
		// derivation above, since callers usually leave it to be derived.
		if clone.Loan.PayAmtStatus == types.InOutInput &&
			clone.Loan.LastOK && dateutil.DateOK(clone.Loan.LastDate) &&
			dateutil.DateComp(bs[unknownIdx].Date, clone.Loan.LastDate) < 0 {
			clone.Loan.PayAmtStatus = types.InOutDefault
		}
		res := generateFancyScheduleMode(clone, clone.Loan.PayAmt, &s, tr, fg, true)
		if res.Err != nil {
			return 0, res.Err
		}
		return res.FinalPrinc, nil
	}

	a0 := 0.0
	f0, err := eval(a0)
	if err != nil {
		return 0, err
	}
	if math.Abs(f0) < 0.005 {
		return 0, nil
	}
	a1 := input.Loan.Amount * 0.5 // DOS first guess: half the loan
	for iter := 0; iter < 40; iter++ {
		f1, err := eval(a1)
		if err != nil {
			return 0, err
		}
		if math.Abs(f1) < 0.005 {
			return a1, nil
		}
		denom := f1 - f0
		if math.Abs(denom) < teeny {
			break
		}
		a2 := a1 - f1*(a1-a0)/denom
		// No non-negative clamp: DOS EstimateAndRefineBalloon → Iterate
		// (AMORTOP.pas:1415) has none, and a terminating balloon on an
		// OVER-amortizing loan legitimately solves NEGATIVE (the final row refunds
		// the overpayment to land the balance exactly on zero). The old
		// `if a2 < 0 { a2 = 0 }` pinned the secant at 0 and returned a wrong
		// balloon for every over-funded loan (verified vs the oracle's solveballoon
		// query: DOS −136,985.82 vs old Go 0.00 on 100k@10%/$2000/120).
		a0, f0 = a1, f1
		a1 = a2
	}
	return a1, nil
}

// annuityPayment returns the level payment that amortizes `balance`
// over n periods at per-period growth factor f. Same closed form as
// estimatePayment, generalized to an arbitrary balance and term.
func annuityPayment(balance, f float64, n int) float64 {
	if n <= 0 {
		return balance
	}
	if math.Abs(f-1) < teeny {
		return balance / float64(n)
	}
	lnf, _ := interest.Lnn(f)
	expVal, _ := interest.Exxp(-float64(n) * lnf)
	denom := 1 - expVal
	if math.Abs(denom) < teeny {
		return balance / float64(n)
	}
	return balance * (f - 1) / denom
}

// prepayAnnuity returns the present value (per unit payment) of a payment
// stream running from start to stop at perYrEff payments per year, discounted
// to repayFrom at the per-period continuous rate `rate`:
//
//	(first - last*ff) / (1 - ff),  first=e^(-rate*YD(start)), last=e^(-rate*YD(stop)),
//	ff = e^(-rate/perYrEff)
//
// This is DOS's `(first - last*ff)/(1-ff)` annuity factor (the regular-payment
// term at AMORTIZE.pas:688 and FirstLastAndFF streams at :694-695). Because
// stop is the date of the LAST payment, last = first*ff^(k-1) for a k-payment
// stream, so the factor equals first*(1+ff+...+ff^(k-1)) — exactly k discounted
// payments.
func prepayAnnuity(rate float64, start, stop types.DateRec, perYrEff float64, repayFrom types.DateRec, s Settings) (float64, error) {
	ydStart := dateutil.YearsDif(start, repayFrom, s.Basis, s.YrInv, true)
	ydStop := dateutil.YearsDif(stop, repayFrom, s.Basis, s.YrInv, true)
	first, err := interest.Exxp(-rate * ydStart)
	if err != nil {
		return 0, err
	}
	last, err := interest.Exxp(-rate * ydStop)
	if err != nil {
		return 0, err
	}
	ff, err := interest.Exxp(-rate / perYrEff)
	if err != nil {
		return 0, err
	}
	if math.Abs(1-ff) < teeny {
		return 0, fmt.Errorf("annuity factor is degenerate (rate too small)")
	}
	return (first - last*ff) / (1 - ff), nil
}

// prepayStopDate returns the date of the last payment of a prepayment series:
// its StopDate if specified, otherwise StartDate advanced by (NN-1) periods.
func prepayStopDate(pp Prepayment) (types.DateRec, error) {
	if pp.StopDateStatus >= types.InOutDefault && dateutil.DateOK(pp.StopDate) {
		return pp.StopDate, nil
	}
	if pp.NN <= 0 {
		return types.DateRec{}, fmt.Errorf("prepayment has neither a stop date nor a payment count")
	}
	return dateutil.AddNPeriods(pp.StartDate, pp.PerYr, pp.NN-1)
}

// SolvePrepaymentAmount solves the per-payment amount of the prepayment series
// at unknownIdx — the DOS "unknown prepayment amount".
//
// The objective differs by semantics, mirroring DOS:
//
//   - REPLACE (PlusRegular OFF, the default): the prepayment replaces the
//     regular payment on coincident dates, so it alone must amortize the loan.
//     The final-balance-zero objective is unique here, and a secant over the
//     real schedule matches DOS to ~1e-8 (TestDOSPrepaymentAmountSolveSweep).
//
//   - ADDITIVE (PlusRegular ON): the prepayment is on top of the regular
//     payment, so the final scheduled payment settles any residual and
//     final-balance-zero holds for a RANGE of amounts (non-unique). DOS instead
//     solves the discounted-PV amount at which principal = PV(regular stream) +
//     PV(extras) + PV(prepayment stream) — the unique "smooth" amortization. We
//     reproduce that closed form (AMORTIZE.pas:684-699).
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefinePeriodicPrepayment.
func SolvePrepaymentAmount(input LoanInput, unknownIdx int) (float64, error) {
	defer beginBackwardSolve()()
	// DOS refuses an unknown prepayment when a coexisting adjustment row is
	// not FULLY specified — SufficientDataOnScreen requires adj_fully_specified
	// (Amortize.pas:892-894 + AMORTOP.pas:393). 2026-07-12 pass-3 finding
	// P3-F3 — verified vs the real DOS engine:
	//
	//	amort_oracle 100000 0.08 120 12 adj=24:0.09: payhard=1050 presolve=6:12:12
	//	→ refused (Go previously "solved" 0.0000 with err=nil)
	if adjRowsNotFullySpecified(input.Adjustments) {
		return 0, fmt.Errorf("A Prepayment amount cannot be solved while a Rate " +
			"Adjustment row is incomplete. Fill in the date, rate AND payment " +
			"on every Adjustment row, or clear the incomplete row.")
	}
	if input.Settings.PlusRegular {
		return solvePrepayAmountAdditive(input, unknownIdx)
	}
	// The residual is the UNFORCED terminal balance — DOS's Iterate evaluates
	// RepayFancyLoan with Output=nil and hard_payment cleared, so no display
	// fold ever runs (AMORTOP.pas:1433-1437). Secanting on the forced display
	// schedule's FinalPrinc broke when the pass-3 AF6 fix taught the display
	// walk to fold a prepay-series residual into the final row: the forced
	// balance became 0 for EVERY trial amount and the solve degenerated to 0.
	// The walk keeps its adjustments: like the balloon-amount solve (and
	// unlike the payment/amount/rate Iterate terminals), DOS's unknown-prepay
	// solve re-amortizes at fully-specified ARMs — verified vs the real DOS
	// engine:
	//
	//	amort_oracle 100000 0.08 120 12 payhard=1050 adj=24:0.09:1100 presolve=6:12:12
	//	→ prepay 2198.1283 (no-adj control 2260.1872)
	//
	// (Partially-specified adjustments were refused above.) Hence the direct
	// generateFancyScheduleMode call rather than fancyTerminal, which strips
	// adjustments per AMORTOP.pas:1215.
	eval := func(amt float64) (float64, error) {
		clone := input
		ps := make([]Prepayment, len(input.Prepayments))
		copy(ps, input.Prepayments)
		ps[unknownIdx].Payment = amt
		ps[unknownIdx].PaymentStatus = types.InOutInput
		clone.Prepayments = ps
		s := clone.Settings
		tr, err := ComputeTrueRate(&clone.Loan, &s)
		if err != nil {
			return 0, err
		}
		fg := GrowthPerPeriod(&clone.Loan, s.YrInv)
		if !clone.Loan.LastOK && clone.Loan.NPeriods > 0 && dateutil.DateOK(clone.Loan.FirstDate) {
			day := clone.Loan.FirstDate.Time.Day()
			last := clone.Loan.FirstDate
			for k := 1; k < clone.Loan.NPeriods; k++ {
				if nd, e := dateutil.AddPeriod(last, clone.Loan.PerYr, day, false); e == nil {
					last = nd
				}
			}
			clone.Loan.LastDate = last
			clone.Loan.LastOK = true
		}
		return generateFancyScheduleMode(clone, clone.Loan.PayAmt, &s, tr, fg, true).FinalPrinc, nil
	}

	a0 := 0.0
	f0, err := eval(a0)
	if err != nil {
		return 0, err
	}
	if math.Abs(f0) < 0.005 {
		return 0, nil
	}
	a1 := input.Loan.PayAmt * 0.5
	if a1 <= 0 {
		a1 = input.Loan.Amount * 0.01
	}
	for iter := 0; iter < 40; iter++ {
		f1, err := eval(a1)
		if err != nil {
			return 0, err
		}
		if math.Abs(f1) < 0.005 {
			return a1, nil
		}
		denom := f1 - f0
		if math.Abs(denom) < teeny {
			break
		}
		a2 := a1 - f1*(a1-a0)/denom
		// No non-negative clamp — same reasoning as SolveBalloonAmount: DOS's
		// Iterate allows a negative solved amount, and an over-funded loan can
		// drive an unknown prepayment negative. Pinning at 0 stalled the secant.
		a0, f0 = a1, f1
		a1 = a2
	}
	return a1, nil
}

// solvePrepayAmountAdditive reproduces DOS's closed-form discounted-PV
// prepayment-amount solve (the non-tiny and tiny-rate branches of
// EstimateAndRefinePeriodicPrepayment, AMORTIZE.pas:670-699). The regular
// payment is credited with its PV over the full term, balloons and other
// prepayments are subtracted at their discounted values, and the unknown
// prepayment is the remainder divided by its own annuity factor.
//
// Used for the ADDITIVE (PlusRegular ON) case only; the replace default keeps
// the unique final-balance secant in SolvePrepaymentAmount.
func solvePrepayAmountAdditive(input LoanInput, unknownIdx int) (float64, error) {
	loan := input.Loan
	s := input.Settings
	rate, err := interest.RateFromYield(loan.LoanRate, byte(loan.PerYr), s.YrDays)
	if err != nil {
		return 0, err
	}
	repayFrom := loan.LoanDate
	unk := input.Prepayments[unknownIdx]
	// The series must be bounded by a count or a stop date.
	unkStop, err := prepayStopDate(unk)
	if err != nil {
		return 0, fmt.Errorf("the unknown Prepayment is unbounded; supply a stop date or " +
			"payment count so its amount can be solved")
	}
	// Count of unknown-series payments (needed only for the tiny-rate branch).
	unkNN := unk.NN
	if unkNN <= 0 {
		unkNN, _ = dateutil.NumberOfInstallments(unk.StartDate, unk.StopDate, unk.PerYr, types.OnOrBefore)
	}

	// Tiny-rate branch (AMORTIZE.pas:675-682): undiscounted balance.
	if math.Abs(rate) < teeny {
		adjp := loan.Amount - float64(loan.NPeriods)*loan.PayAmt
		for _, b := range input.Balloons {
			if b.AmountStatus >= types.InOutDefault {
				adjp -= b.Amount
			}
		}
		for i, pp := range input.Prepayments {
			if i == unknownIdx || pp.PaymentStatus < types.InOutDefault {
				continue
			}
			cnt := pp.NN
			if cnt <= 0 {
				cnt, _ = dateutil.NumberOfInstallments(pp.StartDate, pp.StopDate, pp.PerYr, types.OnOrBefore)
			}
			adjp -= float64(cnt) * pp.Payment
		}
		if unkNN <= 0 {
			return 0, fmt.Errorf("the unknown Prepayment has no resolvable payment count")
		}
		return adjp / float64(unkNN), nil
	}

	// Regular-payment PV over firstdate..lastdate (ff via RealPerYr, :687).
	lastDate := loan.LastDate
	if !loan.LastOK {
		lastDate, err = dateutil.AddNPeriods(loan.FirstDate, loan.PerYr, loan.NPeriods-1)
		if err != nil {
			return 0, err
		}
	}
	annReg, err := prepayAnnuity(rate, loan.FirstDate, lastDate,
		interest.RealPerYr(byte(loan.PerYr), s.YrDays), repayFrom, s)
	if err != nil {
		return 0, err
	}
	adjp := loan.Amount - loan.PayAmt*annReg

	// Subtract balloons at their discounted value (:689-690).
	for _, b := range input.Balloons {
		if b.AmountStatus < types.InOutDefault || b.DateStatus < types.InOutDefault {
			continue
		}
		yd := dateutil.YearsDif(b.Date, repayFrom, s.Basis, s.YrInv, true)
		ev, err := interest.Exxp(-rate * yd)
		if err != nil {
			return 0, err
		}
		adjp -= b.Amount * ev
	}

	// Subtract the other (known) prepayment streams (:691-696).
	for i, pp := range input.Prepayments {
		if i == unknownIdx || pp.PaymentStatus < types.InOutDefault {
			continue
		}
		stop, err := prepayStopDate(pp)
		if err != nil {
			return 0, err
		}
		ann, err := prepayAnnuity(rate, pp.StartDate, stop, float64(pp.PerYr), repayFrom, s)
		if err != nil {
			return 0, err
		}
		adjp -= pp.Payment * ann
	}

	// Unknown prepayment = remainder / its own annuity factor (:697-698).
	annUnk, err := prepayAnnuity(rate, unk.StartDate, unkStop, float64(unk.PerYr), repayFrom, s)
	if err != nil {
		return 0, err
	}
	if math.Abs(annUnk) < teeny {
		return 0, fmt.Errorf("the Prepayment annuity factor is degenerate; cannot solve the amount")
	}
	return adjp / annUnk, nil
}

// SolvePrepaymentDuration solves how many payments the prepayment series at
// unknownIdx must run to retire the loan — the DOS "unknown prepayment
// duration". The series has a known amount but no stop date and no count.
//
// This reproduces DOS's closed-form present-value duration
// (DeterminePrepaymentDuration, AMORTIZE.pas:730-768): the regular payment is
// credited over the full nominal term, balloons and other prepayments are
// subtracted at their discounted values, and the remaining principal fixes the
// number of discounted prepayments. DeterminePrepaymentDuration is additive
// (plus_regular ON) by construction, so the closed form assumes the prepayment
// is on top of the regular payment.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// DeterminePrepaymentDuration.
func SolvePrepaymentDuration(input LoanInput, unknownIdx int) (int, types.DateRec, error) {
	loan := input.Loan
	s := input.Settings
	pp := input.Prepayments[unknownIdx]
	payment := pp.Payment

	// Preconditions (AMORTIZE.pas:716-721): amount, peryr, firstdate present.
	if loan.AmountStatus < types.InOutDefault || loan.PerYr <= 0 || loan.FirstStatus < types.InOutDefault {
		return 0, types.DateRec{}, fmt.Errorf("Amount Borrowed, # Periods/Yr and 1st Pmt " +
			"Date are all required to solve the Prepayment duration")
	}

	rate, err := interest.RateFromYield(loan.LoanRate, byte(loan.PerYr), s.YrDays)
	if err != nil {
		return 0, types.DateRec{}, err
	}
	repayFrom := loan.LoanDate

	lastDate := loan.LastDate
	if !loan.LastOK {
		lastDate, err = dateutil.AddNPeriods(loan.FirstDate, loan.PerYr, loan.NPeriods-1)
		if err != nil {
			return 0, types.DateRec{}, err
		}
	}

	// adjp = principal less the PV of the regular payment stream over the full
	// term. NOTE: DOS uses ff = e^(-rate/peryr) here (h^.peryr directly, not
	// RealPerYr — AMORTIZE.pas:735), which differs from the amount solve.
	adjp := loan.Amount
	annReg, err := prepayAnnuity(rate, loan.FirstDate, lastDate, float64(loan.PerYr), repayFrom, s)
	if err != nil {
		return 0, types.DateRec{}, err
	}
	adjp -= loan.PayAmt * annReg

	// Less balloons (:738-739) and other prepayment streams (:740-745).
	for _, b := range input.Balloons {
		if b.AmountStatus < types.InOutDefault || b.DateStatus < types.InOutDefault {
			continue
		}
		yd := dateutil.YearsDif(b.Date, repayFrom, s.Basis, s.YrInv, true)
		ev, err := interest.Exxp(-rate * yd)
		if err != nil {
			return 0, types.DateRec{}, err
		}
		adjp -= b.Amount * ev
	}
	for i, other := range input.Prepayments {
		if i == unknownIdx || other.PaymentStatus < types.InOutDefault {
			continue
		}
		stop, err := prepayStopDate(other)
		if err != nil {
			return 0, types.DateRec{}, err
		}
		ann, err := prepayAnnuity(rate, other.StartDate, stop, float64(other.PerYr), repayFrom, s)
		if err != nil {
			return 0, types.DateRec{}, err
		}
		adjp -= other.Payment * ann
	}

	// Negative-duration guard (:748-752).
	if adjp < payment {
		return 0, types.DateRec{}, fmt.Errorf("Principal is more than covered by the fixed " +
			"payments — the Prepayment duration would be negative. Lower the regular payment " +
			"or the Prepayment amount.")
	}

	// Solve for the last prepayment date (:755-767).
	ydStart := dateutil.YearsDif(pp.StartDate, repayFrom, s.Basis, s.YrInv, true)
	first, err := interest.Exxp(-rate * ydStart)
	if err != nil {
		return 0, types.DateRec{}, err
	}
	ff, err := interest.Exxp(-rate / float64(pp.PerYr))
	if err != nil {
		return 0, types.DateRec{}, err
	}
	if math.Abs(ff) < tiny {
		return 0, types.DateRec{}, fmt.Errorf("Loan Rate is too small to determine the " +
			"duration of the extra payments")
	}
	lastFactor := (first - adjp*(1-ff)/payment) / ff
	if lastFactor <= 0 {
		return 0, types.DateRec{}, fmt.Errorf("The Prepayment duration could not be solved " +
			"(the discounted balance is non-positive). Check the Prepayment amount and dates")
	}
	lnLast, err := interest.Lnn(lastFactor)
	if err != nil {
		return 0, types.DateRec{}, err
	}
	nyrs := -lnLast/rate - ydStart
	// Rounding nudge that compensates for the round-down in `before` mode below.
	nyrs += 0.5 / interest.RealPerYr(byte(pp.PerYr), s.YrDays)

	stopDate, err := dateutil.AddYears(pp.StartDate, nyrs, s.Basis, s.YrDays)
	if err != nil {
		return 0, types.DateRec{}, err
	}
	nn, adjStop := dateutil.NumberOfInstallments(pp.StartDate, stopDate, pp.PerYr, types.Before)
	if nn <= 0 {
		return 0, types.DateRec{}, fmt.Errorf("The Prepayment duration solved to a " +
			"non-positive count; check the Prepayment amount and start date")
	}
	return nn, adjStop, nil
}

// solveFancyTermFromPayment derives the number of periods from a
// known payment for a loan that uses advanced options (fancy mode).
// The closed-form solveNPeriodsFromPayment cannot account for
// balloons, prepayments and adjustments, so this runs the fancy
// schedule with an effectively unbounded term and observes when the
// loan retires (the engine's early-payoff termination stops it).
//
// Ported from legacy/src/dos_source/AMORTOP.pas: the fancy branch of
// DetermineLastPaymentDate (lines 1336-1379).
func solveFancyTermFromPayment(input LoanInput) (int, types.DateRec, error) {
	clone := input
	loan := clone.Loan
	// 80 years is longer than any real loan and keeps weekly
	// schedules under the engine's 10000-period guard.
	cap := loan.PerYr * 80
	loan.NPeriods = cap
	loan.NStatus = types.InOutInput
	loan.LastStatus = types.StatusEmpty // let FirstPass derive lastDate
	loan.LastOK = false
	clone.Loan = loan

	// Bound any unbounded prepayment series (no stop date, no count) on the
	// clone so the prepayment-DURATION solve (AO10) is NOT triggered inside this
	// internal term-solve — here the prepayment must simply run until the loan
	// retires, exactly as DOS's DetermineLastPaymentDate uses it. Deep-copy the
	// slice so the caller's input is untouched.
	if len(input.Prepayments) > 0 {
		ps := make([]Prepayment, len(input.Prepayments))
		copy(ps, input.Prepayments)
		for i := range ps {
			if ps[i].StopDateStatus < types.InOutDefault && ps[i].NNStatus < types.InOutDefault {
				ps[i].NN = cap
				ps[i].NNStatus = types.InOutInput
			}
		}
		clone.Prepayments = ps
	}

	res := Amortize(clone)
	if res.Err != nil {
		return 0, types.DateRec{}, res.Err
	}
	n := 0
	var last types.DateRec
	for i := range res.Schedule {
		if res.Schedule[i].PayNum >= 1 {
			n++
			last = res.Schedule[i].Date
		}
	}
	if n == 0 {
		return 0, types.DateRec{}, fmt.Errorf(
			"The loan term could not be derived — no payments were produced. Check " +
				"Amount Borrowed, Loan Rate and the advanced options, or enter " +
				"# Periods directly.")
	}
	if n >= cap {
		return 0, types.DateRec{}, fmt.Errorf(
			"Pmt Amount is too small to pay off the loan — even after 80 years the " +
				"balance is not retired. Raise the Pmt Amount, or enter # Periods " +
				"directly.")
	}
	return n, last, nil
}

// balanceAfterN returns the remaining principal after n level
// payments of d on a starting balance at per-period growth f.
func balanceAfterN(balance, d, f float64, n int) float64 {
	p := balance
	for i := 0; i < n; i++ {
		p = p*f - d
	}
	return p
}

// solveAdjRate fits a loan rate to a known payment: it finds the rate
// at which `payment` amortizes `balance` to zero over n periods.
// Used for an ARM adjustment that supplies a new payment but no new
// rate (DOS EstimateAndRefineAdjRate, Amortize.pas:1415-1418) — the
// mirror image of the AO5 rate-given/payment-solved case, so the
// loan still ends on its original term.
func solveAdjRate(balance, payment float64, n int, loan Loan,
	yrinv float64) (float64, bool) {

	g := func(rate float64) float64 {
		l := loan
		l.LoanRate = rate
		return balanceAfterN(balance, payment, GrowthPerPeriod(&l, yrinv), n)
	}
	r0 := loan.LoanRate
	g0 := g(r0)
	r1 := loan.LoanRate + 0.005
	for i := 0; i < 40; i++ {
		g1 := g(r1)
		if math.Abs(g1) < 0.005 {
			return r1, true
		}
		denom := g1 - g0
		if math.Abs(denom) < teeny {
			break
		}
		r2 := r1 - g1*(r1-r0)/denom
		// DOS's Iterate allows a NEGATIVE implied rate — a new payment that
		// overpays the balance implies rate < 0 — and bounds |rate| < 2
		// (AMORTOP.pas:1485). Clamp to that range rather than to >= 0;
		// clamping at zero made the secant stall on overpaying payments, so the
		// old rate was wrongly retained (payment-only ARM adjustment diverged
		// from DOS, which re-computes at the negative rate).
		if r2 < -1.9 {
			r2 = -1.9
		} else if r2 > 1.9 {
			// (coverage: excluded — defensive/unreachable: the terminal balance
			// balanceAfterN is monotone increasing in rate, so the secant always
			// steps rate DOWN to zero the residual and hits the lower clamp; this
			// upper clamp mirrors DOS's symmetric |rate|<2 bound.)
			r2 = 1.9
		}
		r0, g0 = r1, g1
		r1 = r2
	}
	return r1, false
}

// adjRowsNotFullySpecified reports whether any rate-adjustment row is present
// but missing its date, rate, or amount. DOS `adj_fully_specified :=
// allaprok and allwhenok and allamtok` (AMORTOP.pas:393); SufficientDataOnScreen
// requires it before admitting an unknown balloon (Amortize.pas:889-890), an
// unknown prepayment (:892-894), or a term solve (:884-887). 2026-07-12 pass-3
// finding P3-F3.
func adjRowsNotFullySpecified(adjs []RateAdjustment) bool {
	for i := range adjs {
		a := &adjs[i]
		present := a.DateStatus >= types.InOutDefault ||
			a.LoanRateStatus >= types.InOutDefault ||
			a.AmountStatus >= types.InOutDefault
		if !present {
			continue
		}
		if a.DateStatus < types.InOutDefault ||
			a.LoanRateStatus < types.InOutDefault ||
			a.AmountStatus < types.InOutDefault {
			return true
		}
	}
	return false
}

// anyAdjRowPresent reports whether any rate-adjustment row carries data.
func anyAdjRowPresent(adjs []RateAdjustment) bool {
	for i := range adjs {
		if adjs[i].DateStatus >= types.InOutDefault ||
			adjs[i].LoanRateStatus >= types.InOutDefault ||
			adjs[i].AmountStatus >= types.InOutDefault {
			return true
		}
	}
	return false
}
