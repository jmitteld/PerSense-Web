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
	"os"
	"time"

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
	// DOS coerces weekly/biweekly off the 360 basis in MakeTable's preprocessing,
	// upstream of every solve (Amortize.pas:297-303). See coerceSubMonthlyBasis.
	coerceSubMonthlyBasis(&input)
	// DOS snaps the moratorium boundary onto the payment grid in its screen
	// prepass (Amortize.pas:1263), BEFORE dispatching any EstimateAndRefine*
	// solve (:1333-1421). Go's solvers are entered directly, so each repeats it.
	snapMoratoriumFirstRepay(&input)
	CheckPrepaymentStops(input.Prepayments) // CheckPrepayments' count-to-date arm (AMORTOP.pas:416-419), same screen prepass
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
	// DOS coerces weekly/biweekly off the 360 basis in MakeTable's preprocessing,
	// upstream of every solve (Amortize.pas:297-303). See coerceSubMonthlyBasis.
	coerceSubMonthlyBasis(&input)
	input.inBackwardSolve = true // keep this solver's inner trials off the faithful port (per-call, race-free)
	// DOS snaps the moratorium boundary onto the payment grid in its screen
	// prepass (Amortize.pas:1263), BEFORE dispatching any EstimateAndRefine*
	// solve (:1333-1421). Go's solvers are entered directly, so each repeats it.
	snapMoratoriumFirstRepay(&input)
	CheckPrepaymentStops(input.Prepayments) // CheckPrepayments' count-to-date arm (AMORTOP.pas:416-419), same screen prepass
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

	// DOS's prepaid amount-solve shortcut is SKIP-BLIND (2026-07-24 solver×
	// options audit). EstimateAndRefineLoanAmount's fast-path gate
	// (Amortize.pas:458) is
	//
	//	((basis=x360) or (not exact)) and prepaid and (nballoons=0)
	//	and (npre=0) and (not in_advance)
	//
	// — note it checks BALLOONS and PREPAYMENTS but NOT skip-months (the
	// PAYMENT solve's gate at :402 DOES exclude skip and target; the amount
	// gate simply lacks those terms). So when the gate holds, DOS returns the
	// closed-form estimate, which ignores skip entirely — and the schedule
	// then folds the shortfall into the final payment. Oracle-verified:
	//
	//	noamt pay=888.4879 prepaid skip=6-8 (24mo@12%) → DOS 18874.49 (= no-skip)
	//	noamt pay=888.4879        skip=6-8            → DOS 14134.97 (skip-aware)
	//	prepaid mor=6 skip=6-8 → 15305.10 = prepaid mor=6 alone (mor honored
	//	  via nrepay; skip alone is dropped)
	//
	// Since prepaid is the UI default, matching the shipped app means
	// reproducing this: under the gate, solve AS IF there were no skip months
	// (strip SkipMonths and let the normal pipeline — including the mor/adj
	// refinement — run). The engine's "implied terminating balloon" advisory
	// then surfaces the resulting final-payment fold that DOS leaves silent.
	// Adjustment-blindness needs no equivalent here: DOS Iterates til_adj and
	// Go's fancyTerminal already strips adjustments to match.
	hasBalloonOrPrepay := len(input.Prepayments) > 0
	for _, b := range input.Balloons {
		if b.AmountStatus >= types.InOutDefault || b.DateStatus >= types.InOutDefault {
			hasBalloonOrPrepay = true
		}
	}
	if settings.Prepaid && !settings.InAdvance && !exactDaily(&settings) &&
		!hasBalloonOrPrepay &&
		input.SkipMonths.SkipStatus >= types.InOutDefault && input.SkipMonths.SkipStr != "" {
		input.SkipMonths = SkipMonths{}
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
	// DOS coerces weekly/biweekly off the 360 basis in MakeTable's preprocessing,
	// upstream of every solve (Amortize.pas:297-303). See coerceSubMonthlyBasis.
	coerceSubMonthlyBasis(&input)
	input.inBackwardSolve = true // keep this solver's inner trials off the faithful port (per-call, race-free)
	// DOS snaps the moratorium boundary onto the payment grid in its screen
	// prepass (Amortize.pas:1263), BEFORE dispatching any EstimateAndRefine*
	// solve (:1333-1421). Go's solvers are entered directly, so each repeats it.
	snapMoratoriumFirstRepay(&input)
	CheckPrepaymentStops(input.Prepayments) // CheckPrepayments' count-to-date arm (AMORTOP.pas:416-419), same screen prepass
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
				refined, ok, condemned := dosIterateRate(input, dosSeed)
				// DOS-FAITHFUL SCREEN CONDEMNATION. A trial rate inside the
				// secant drove ComputeTrueRate's `1 + yy/nn` non-positive, so
				// lnn raised errorflag/overflowflag and DOS's whole screen
				// refuses — no rate, no table, this exact message
				// (INTSUTIL.pas:1164-1171; Amortize.pas:1457-1458). Returning
				// converged=false here instead would let the caller amortize at
				// the unrefined closed-form rate and print a schedule where DOS
				// prints none — a divergence in the worst direction. See
				// dosIterateRate's doc comment for the seed-21001 repro.
				if condemned {
					return 0, false, fmt.Errorf("Error: The data you have " +
						"specified contain an inconsistency.")
				}
				// Accept a converged refinement even when it is NEGATIVE (an
				// under-funded loan): DOS returns the negative rate here. The old
				// `refined > 0` guard discarded a correct negative root and fell
				// through to the non-converged warning. Bound it by DOS's |rate|<=2.
				//
				// DOS-faithful lower bound: reject a rate that drives the per-period
				// GROWTH FACTOR non-positive (1 + rate/RealPerYr ≤ 0, i.e. rate at or
				// below −RealPerYr). At such a rate DOS's Iterate recomputes
				// GrowthPerPeriod and its Lnn/yield math takes the log of a
				// non-positive number, aborting with "Error: The data you have
				// specified contain an inconsistency." (INTSUTIL.pas:1169), NOT a
				// converged rate. A dominating balloon (PV ≥ the loan) has no
				// positive-growth rate root, so DOS refuses — the port's fancy secant
				// otherwise wandered below −100% and returned e.g. −151%. 2026-07-16
				// fancy fuzzer3. (Genuine mild-negative under-funded rates keep a
				// positive growth factor and are unaffected.)
				if v, good := acceptRefinedRate(input, refined, ok, settings.YrInv); good {
					return v, true, nil
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
			// (The condemnation latch cannot fire on this arm: dosIterateRate
			// arms it only for the RepayFancyLoan terminal, and this branch is
			// reached only when needScheduleRefine said the loan is plain.)
			if _, ok, _ := dosIterateRate(input, dosSeed); !ok {
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

	// THE CLOSED-FORM NEWTON EXHAUSTED maxIter — RUN DOS'S ITERATE ANYWAY.
	// Discrepancies §57 (round 15, 2026-08-01).
	//
	// The loop above is the PORT's pre-solve. DOS has no such thing:
	// EstimateAndRefineRate (Amortize.pas:467-491) seeds
	//
	//	loanrate := payamt * peryr / amount;  if (loanrate<0.02) then 0.02;
	//	if Iterate(amount, usap, loandate, firstdate, loanrate, til_adj) then ...
	//
	// and calls Iterate UNCONDITIONALLY from that seed. The port had made the
	// DOS Iterate reachable only through the `math.Abs(step) < teeny` branch
	// inside the loop, so whenever the port's own pre-solve failed to settle in
	// 30 iterations the port fell out here and returned the UNREFINED estimate
	// with converged=false — on screens where DOS converges cleanly. The DOS
	// Iterate was never even attempted.
	//
	// WHERE IT IS LIVE. The pre-solve stalls when the loan is deep into
	// perpetuity territory — perpetuity depth (1+i)^-n below roughly 1e-5, i.e.
	// the hardened payment is within a hair of A*i and the principal barely
	// amortizes. RepayLoan's terminal balance is then astronomically stiff in
	// the rate, the secant's reused `delta` collapses into cancellation noise,
	// and |step| never falls under teeny. Measured by the round-trip gate's
	// horizon strata (zzroundtrip_test.go): at ~50-75 year spans the port
	// recovered the entered rate to ~1e-4 where DOS recovered it to ~1e-7,
	// 3 cases in 25. The DOS Iterate, run on the identical screen, returns the
	// DOS answer to within 3e-7 — the refinement was correct all along and was
	// simply never reached.
	//
	// NOT a date-layer effect: the failing spans end in 2095-2098, short of
	// both §54's Feb 2100 century-leap divergence and §55's 2155 year byte, and
	// the matched-n control (identical period counts over a short and a long
	// calendar span) puts the failure on the span, not the iteration count.
	if refined, ok, condemned := dosIterateRate(input, dosSeed); condemned {
		// Same DOS-faithful screen condemnation as the converged branch above:
		// a trial rate drove ComputeTrueRate's `1 + yy/nn` non-positive, so DOS
		// refuses the whole screen (INTSUTIL.pas:1164-1171).
		return 0, false, fmt.Errorf("Error: The data you have " +
			"specified contain an inconsistency.")
	} else if v, good := acceptRefinedRate(input, refined, ok, settings.YrInv); good {
		return v, true, nil
	}
	return rate, false, nil
}

// acceptRefinedRate applies the guards DOS's EstimateAndRefineRate applies to a
// rate returned by Iterate. Shared by SolveRate's two call sites so the
// pre-solve-converged and pre-solve-exhausted paths cannot drift apart.
//
//   - a NEGATIVE rate is accepted: an under-funded loan (payments totalling less
//     than principal) has a genuinely negative implied rate and DOS returns it.
//   - |rate| < 2 is DOS's own divergence bound (AMORTOP.pas:1485).
//   - GrowthPerPeriod > 0 rejects a rate at or below −RealPerYr, where DOS's
//     Lnn/yield math takes the log of a non-positive number and aborts rather
//     than converging (INTSUTIL.pas:1169).
func acceptRefinedRate(input LoanInput, refined float64, ok bool, yrInv float64) (float64, bool) {
	gp := input.Loan
	gp.LoanRate = refined
	if ok && refined != 0 && refined > -2 && refined < 2 &&
		GrowthPerPeriod(&gp, yrInv) > 0 {
		return refined, true
	}
	return 0, false
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
		} else {
			// Zero/negative first period (first payment ON or before the loan
			// date, e.g. a semimonthly loan with firstDate = loanDate): mirror the
			// forward RepayLoan terminal (engine.go), which falls back to
			// firstPeriodProrate here, so the term SOLVE uses the SAME first-period
			// treatment as the schedule it renders — and as DOS's closed form, which
			// computes the term with this prorate (AMORTOP.pas:1388). Without it the
			// solve defaulted to prorate 1.0 and over-counted: e.g. a firstDate=
			// loanDate semimonthly loan reported NPeriods=410 while rendering a
			// 377-row schedule (DOS reports 377). 2026-07-16 fuzz hunt.
			prorate = firstPeriodProrate(loan.LoanDate, loan.FirstDate, loan.PerYr, settings)
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
	//
	// IN-ADVANCE pins it too, and unconditionally. FirstPass forces the prepaid
	// global TRUE for every in-advance loan (`if (df.c.in_advance) then prepaid
	// := true`, Amortize.pas:206-209), and MakeTable's clearing prepass is itself
	// gated on `not df.c.in_advance` (:1252-1255) — so an in-advance screen can
	// never arrive at the prorate block with prepaid false, and Amortize.pas:1281
	// gives prorate := 1 with no date test at all. The port required
	// `Prepaid && !InAdvance`, so every in-advance screen kept the date-derived
	// prorate instead. Invisible at monthly, where the odd first period is about
	// one period anyway; severe below monthly, where a two-month stub is ~8.8
	// weekly periods and ffFirst = 1+(f-1)*prorate is off by that factor:
	//
	//	amort_oracle 107014.77 0.0897530000 468 52 b365_360 exact prepaid inadv \
	//	  usa loandmy=6.9.2023 firstdmy=6.11.2023 pts=0.013492 payhard=409.93 noterm
	//	→ DOS solvedterm 349 (7/8/2030); Go reported 356 (8/26/2030)
	//
	// Refeeding DOS's 349 as a FIXED term makes the two agree to the cent
	// (interest 39008.36, paid 146023.13, row for row) — which is what localises
	// the defect to this closed form rather than to the walk.
	//
	// Fourth instance of one defect shape: a DOS global that MakeTable's prepass
	// assigns, which the port reconstructs per call frame (`prepaid` in the payoff
	// path, `h^.lastdate` in Re_Amortize, `basis` across the solve entry points,
	// now `prorate`). 2026-07-30.
	if settings.InAdvance {
		prorate = 1
	} else if settings.Prepaid &&
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
//
// The third result mirrors DOS's `overflowflag`. DOS's exxp does NOT return a
// neutral value when its argument exceeds 70 (INTSUTIL.pas:1145-1152):
//
//	if (x>70) then begin
//	   exxp:=0;
//	   MessageBox('Overflow error: answer too large for this computer''s
//	              numeric format.', DO_ExxpOverflow);
//	   overflowflag:=true; errorflag:=true;
//	   end
//
// It contributes 0 for the offending term, raises the message box, and sets
// BOTH flags. `overflowflag` stops the value walk at the top of the very next
// pass of RepayFancyLoan's repeat loop (`repeat if (overflowflag) then exit;`,
// AMORTOP.pas:1197-1198) and again at the top of every secant pass
// (`if (overflowflag) then goto GET_OUT;`, Amortize.pas:567-568), so the
// refinement stops dead. `errorflag` is the engine-wide abort: once it is set
// the whole screen refuses and MakeTable yields no table at all.
//
// The port used to swallow the overflow with `if err != nil { continue }`,
// which silently DROPPED the offending payment from the discounted stream and
// let the secant wander on to a fabricated APR — and, worse, let the caller
// return a full schedule for a screen the real engine refuses outright.
func ComputeAPRWithPoints(schedule []PaymentRecord, loanDate types.DateRec,
	netProceeds, firstGuess float64, perYr byte, settings *Settings) (apr float64, converged, overflow bool) {

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
				// exxp yields 0 for this term and sets overflowflag; the walk
				// then exits at the top of its next pass (AMORTOP.pas:1197).
				overflow = true
				return sum
			}
			sum += row.PayAmt * ev
			if dpTraceAV {
				fmt.Fprintf(os.Stderr, "GAV pay d=%s amt=%.6f yd=%.8f acc=%.6f\n",
					row.Date.Time.Format("2006-1-2"), row.PayAmt, yd, sum)
			}
		}
		return sum
	}

	vRate := firstGuess
	if vRate <= 0 {
		vRate = 0.1
	}
	oldValue := value(vRate)
	if dpTrace {
		fmt.Fprintf(os.Stderr, "GAPR0 seed=%.10f oldvalue=%.6f target=%.6f ovf=%v rows=%d\n",
			vRate, oldValue, netProceeds, overflow, len(schedule))
	}
	delta := small
	vRate += delta
	count := 0
	for ; count < 20; count++ {
		if overflow {
			// Amortize.pas:567-568 — `goto GET_OUT`: no APR is stored and the
			// engine-wide errorflag has already condemned the screen.
			return 0, false, true
		}
		v := value(vRate)
		denom := v - oldValue
		if math.Abs(denom) > teeny {
			delta = (netProceeds - v) * delta / denom
		} else {
			delta = small
		}
		oldValue = v
		vRate += delta
		if dpTrace {
			fmt.Fprintf(os.Stderr, "GAPR n=%d vrate=%.10f aprvalue=%.6f denom=%.6f "+
				"delta=%.10f newvrate=%.10f ovf=%v\n",
				count+1, vRate-delta, v, denom, delta, vRate, overflow)
		}
		if math.Abs(delta) < teeny {
			converged = true
			break
		}
	}
	if dpTrace {
		fmt.Fprintf(os.Stderr, "GAPRend count=%d delta=%.10f vrate=%.10f ovf=%v\n",
			count+1, delta, vRate, overflow)
	}
	if overflow {
		return 0, false, true
	}
	// DOS reports convergence on `teeny` but STORES the APR on the weaker
	// `tiny` (Amortize.pas:591-604):
	//
	//	if (abs(delta) < teeny) then EstimateAndRefineAPRwithPoints := true
	//	else EstimateAndRefineAPRwithPoints := false;
	//	if (abs(delta) < tiny) then
	//	  begin ... h^.apr := YieldFromRate(v_rate, py); h^.aprstatus := outp; end;
	//
	// The two thresholds are eight orders of magnitude apart (1e-10 vs 1e-5), so
	// a 20-pass run that lands inside `tiny` but outside `teeny` still writes the
	// APR field even though the caller pops "Computation of APR failed to
	// converge." Outside `tiny` the APR field is left UNTOUCHED — DOS shows the
	// schedule with a blank APR, it does not show a fabricated one. The port
	// returned `yld` unconditionally, so a wildly non-converged secant reported
	// whatever rate pass 20 happened to be sitting on.
	if math.Abs(delta) >= tiny {
		return 0, false, false
	}
	yld, err := interest.YieldFromRate(vRate, perYr, settings.YrDays)
	if err != nil {
		return 0, false, false
	}
	return yld, converged, false
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
	// DOS coerces weekly/biweekly off the 360 basis in MakeTable's preprocessing,
	// upstream of every solve (Amortize.pas:297-303). See coerceSubMonthlyBasis.
	coerceSubMonthlyBasis(&input)
	input.inBackwardSolve = true // keep this solver's inner trials off the faithful port (per-call, race-free)
	// DOS snaps the moratorium boundary onto the payment grid in its screen
	// prepass (Amortize.pas:1263), BEFORE dispatching any EstimateAndRefine*
	// solve (:1333-1421). Go's solvers are entered directly, so each repeats it.
	snapMoratoriumFirstRepay(&input)
	CheckPrepaymentStops(input.Prepayments) // CheckPrepayments' count-to-date arm (AMORTOP.pas:416-419), same screen prepass
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
	// DOS coerces weekly/biweekly off the 360 basis in MakeTable's preprocessing,
	// upstream of every solve (Amortize.pas:297-303). See coerceSubMonthlyBasis.
	coerceSubMonthlyBasis(&input)
	input.inBackwardSolve = true // keep this solver's inner trials off the faithful port (per-call, race-free)
	// DOS snaps the moratorium boundary onto the payment grid in its screen
	// prepass (Amortize.pas:1263), BEFORE dispatching any EstimateAndRefine*
	// solve (:1333-1421). Go's solvers are entered directly, so each repeats it.
	snapMoratoriumFirstRepay(&input)
	CheckPrepaymentStops(input.Prepayments) // CheckPrepayments' count-to-date arm (AMORTOP.pas:416-419), same screen prepass
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
	// DOS coerces weekly/biweekly off the 360 basis in MakeTable's preprocessing,
	// upstream of every solve (Amortize.pas:297-303). See coerceSubMonthlyBasis.
	coerceSubMonthlyBasis(&input)
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
	// repay_from — DOS Amortize.pas:1260-1288: the date amortization begins,
	// which anchors EVERY YearsDif in this closed form.
	//   - moratorium set → first_repay snapped to the payment grid
	//     (NumberOfInstallments on_or_after), then ONE PERIOD BACK;
	//   - else prepaid   → firstdate one period back;
	//   - else           → the loan date.
	// The port previously hardcoded the loan date, so the AO10 duration solve
	// ignored a moratorium entirely — the 2026-07-24 solver-options audit
	// measured Go 55 vs DOS 27 extra payments on a 44-month-moratorium case.
	// (The amount/AO9 closed forms don't need this: their estimates are
	// refined against the real schedule; this closed form IS the answer.)
	repayFrom := loan.LoanDate
	if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
		_, snapped := dateutil.NumberOfInstallments(loan.FirstDate, input.Moratorium.FirstRepay,
			loan.PerYr, types.OnOrAfter)
		repayFrom, err = dateutil.AddPeriod(snapped, loan.PerYr, loan.FirstDate.Time.Day(), true)
		if err != nil {
			return 0, types.DateRec{}, err
		}
	} else if s.Prepaid {
		repayFrom, err = dateutil.AddPeriod(loan.FirstDate, loan.PerYr, loan.FirstDate.Time.Day(), true)
		if err != nil {
			return 0, types.DateRec{}, err
		}
	}

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

// fancyTermHorizonPeriods returns the number of regular payments DOS's probe
// walk can emit before it hits its own wall, which is what bounds the fancy
// term solve.
//
// DOS does NOT walk forever, and it does not walk "80 years" either. The walk
// lives in RepayFancyLoan, whose terminator is
//
//	until ( ... ) or (DateComp(WhenToStop^.date, stopdate) >= 0) or (abort);
//	                                                { AMORTOP.pas:1220-1222 }
//
// and whose stopdate is set at AMORTOP.pas:1140-1148:
//
//	if (adjnum > 0) then stopdate := adj[adjnum]^.date
//	else                 stopdate := very_last;
//	if (not dateok(stopdate)) then
//	  begin
//	    stopdate := firstdate;
//	    stopdate.y := 100 + pred(df.c.centurydiv);
//	  end;             {Keep going as long as possible}
//
// DetermineLastPaymentDate calls it with adjnum = 0 (AMORTOP.pas:1340), so the
// horizon is `very_last`. And `very_last` is ALWAYS invalid on this path:
// DetermineVeryLast (AMORTOP.pas:1293-1304) runs before the solve dispatch
// (Amortize.pas:1320) and starts from
//
//	if (nballoons > 0) and (DateComp(balloon[nballoons]^.date, h^.lastdate) > 0)
//	  then very_last := balloon[nballoons]^.date
//	  else very_last := h^.lastdate;
//	for i := 1 to npre do
//	  if (DateComp(pre[i]^.stopdate, very_last) > 0) then very_last := pre[i]^.stopdate;
//
// with h^.lastdate BLANK — that blank is exactly why the term solve was
// dispatched. DateComp documents "Blank or unknown dates are later than
// everything" (INTSUTIL.pas:829-830), so a real balloon date compares -1 against
// the blank and neither the balloon arm nor the prepayment loop ever fires.
// very_last stays blank, dateok fails, and the walk gets the fallback wall.
//
// That wall is firstdate's DAY AND MONTH stamped into Pascal year
// `100 + pred(centurydiv)`. With the shipped centurydiv = 50 (PEDATA.pas:67)
// that is Pascal 149, and pascalYear(cal) = cal - 1900 (dateutil.go:63-71), so
// the wall is calendar 2049 — roughly 24 years of headroom from a 2025 first
// payment, NOT 80. WhenToStop is @NextPayment here (Output = nil, adjnum = 0).
//
// The wall payment is INSIDE the horizon, so this is types.OnOrBefore, not
// types.Before. The `repeat` body at AMORTOP.pas:1196-1222 calls
// `NextPayment.ComputeNext(p, usapart)` — which APPLIES the next payment to p —
// and only THEN evaluates
//
//	until ... or (DateComp(WhenToStop^.date, stopdate) >= 0) or (abort);
//
// so the iteration that lands exactly ON stopdate has already paid that
// payment before the loop notices it has arrived. Evidence, fuzzer5 seed 21005:
//
//	amort_oracle 266514.86 0.0328810000 276 12 exact inadv plusreg r78 usa \
//	  loandmy=17.9.2023 firstdmy=17.11.2023 mor=80 b95=17872.23 b169=59649.32 \
//	  pre=37:77:24:140.16 targ=152.62 skip=5-7 pts=0.017429 payhard=1500.03 noterm
//
// solves `solvedterm 313 last 2049-11-17` — the wall date ITSELF — with the DOS
// row tail reading `row 11/17/49 int 4.7800 prin 1745.1800 bal 0.0000` then
// `end`. types.Before capped this at 312 and the port refused a loan DOS
// retires. The refusal arm in solveFancyTermFromPayment is correspondingly
// `n > cap`, not `n >= cap`: n == cap is the wall payment retiring the loan.
//
// The port used `loan.PerYr * 80` until 2026-07-29 and so retired loans DOS
// refuses. From the widened backward-solve fuzzer (seed 21000) all five of
//
//	amort_oracle 267629.69 0.0804570000 23 1 inadv r78 usa loandmy=15.9.2024 \
//	  firstdmy=15.9.2025 mor=72 b96=12784.81 b192=39886.48 b204=76071.74 \
//	  pre=12:17:24:76.11 targ=6072.09 payhard=23248.48 noterm
//
// and its four siblings answer `ERR Payment amount is too small to compute
// number of periods.` — the `if (p > minpmt) then goto ABORT` at
// AMORTOP.pas:1344-1345, reached because the residual is still unpaid when the
// wall arrives. That line is annual off a 2025-09-15 first payment: 24 payments
// of headroom against the 45 Go needed.
//
// A floor of 1 keeps a first date at or past the wall from producing a
// degenerate clone, and the ceiling keeps daily schedules (365 * 24 = 8760)
// under the engine's 10000-period guard.
func fancyTermHorizonPeriods(firstDate types.DateRec, peryr int, centuryDiv int) int {
	const maxHorizonPeriods = 9000
	if centuryDiv <= 0 {
		centuryDiv = dateutil.DefaultCenturyDiv
	}
	if peryr <= 0 || !dateutil.DateOK(firstDate) {
		return maxHorizonPeriods
	}
	// DOS's wall (AMORTOP.pas:1143-1147, reached because very_last is blank here):
	//
	//	if (not dateok(stopdate)) then
	//	  begin
	//	    stopdate := firstdate;
	//	    stopdate.y := 100 + pred(df.c.centurydiv);
	//
	// written RAW — no CheckForDaysTooLarge — so a 29/30/31 anchor landing in a
	// short month leaves a PHANTOM daterec (29/2/2049 for a 29-Feb loan), and
	// DateComp (INTSUTIL.pas:828-846) compares the packed record lexicographically
	// by (y, m, d) with no Julian normalisation. types.DateRec cannot hold a
	// phantom and types.NewDateRec would normalise 29/2 to 1/3, so the wall is
	// carried as three raw ints and compared field-wise.
	//
	// The terminator is a `repeat … until` whose test runs AFTER ComputeNext has
	// advanced and applied the payment (AMORTOP.pas:1219-1221):
	//
	//	until ... or (DateComp(WhenToStop^.date, stopdate) >= 0) or (abort);
	//
	// so the horizon is the index of the FIRST payment ON OR AFTER the wall — that
	// payment is emitted — and because the test is at the bottom the walk always
	// emits at least 2. This code used types.OnOrBefore, i.e. the LAST payment on
	// or before the wall, and floored at 1. That is one period SHORT whenever the
	// grid does not land exactly on the wall date, which is almost always for
	// peryr 26/52 and for any 29-Feb-anchored grid, and it turned valid DOS
	// answers into hard refusals. Measured (all stable over 3-8 oracle runs):
	//
	//	100000 0.08 1254 52 b365 loandmy=25.12.2024 firstdmy=1.1.2025 targ=0 \
	//	  payhard=179.66 noterm      DOS 1254 (last 2049-1-6) | Go refused  [cap 1253 vs 1254]
	//	100000 0.08 628 26 b365 loandmy=18.12.2024 firstdmy=1.1.2025 targ=0 \
	//	  payhard=359.2 noterm       DOS 628  (last 2049-1-13)| Go refused  [627 vs 628]
	//	100000 0.08 302 12 loandmy=29.1.2024 firstdmy=29.2.2024 targ=0 \
	//	  payhard=770.50 noterm      DOS 302  (last 2049-3-31)| Go refused  [301 vs 302]
	//	100000 0.08 24 12 loandmy=1.12.2054 firstdmy=1.1.2055 targ=0 \
	//	  payhard=100000 noterm      DOS 2    (last 2055-2-1) | Go refused  [floor 1 vs 2]
	//
	// Control proving the mechanism: firstdmy=31.1.2024 makes the wall 31/1/2049 a
	// real date that a payment lands on, and the two agree (both 301).
	//
	// Found by the 2026-07-30 DetermineLastPaymentDate audit, NOT by fuzzing:
	// dos_fuzzer5_test.go drew perYrs {1,2,4,12}, so no generated case could reach
	// a 26/52 grid. Finding A.
	wy := 1900 + 100 + centuryDiv - 1
	wm := int(firstDate.Time.Month())
	wd := firstDate.Time.Day()
	// atOrPastWall reports DateComp(d, stopdate) >= 0 on DOS's field-wise terms.
	atOrPastWall := func(d types.DateRec) bool {
		y, m, dd := d.Time.Year(), int(d.Time.Month()), d.Time.Day()
		if y != wy {
			return y > wy
		}
		if m != wm {
			return m > wm
		}
		return dd >= wd
	}
	n := 1
	cur := firstDate
	for n < maxHorizonPeriods {
		if atOrPastWall(cur) {
			break
		}
		nd, err := dateutil.AddPeriod(cur, peryr, wd, false)
		if err != nil {
			break
		}
		cur = nd
		n++
	}
	// The `until` sits at the bottom of the repeat, so a first payment already at
	// or past the wall still yields two payments.
	if n < 2 {
		n = 2
	}
	if n > maxHorizonPeriods {
		return maxHorizonPeriods
	}
	return n
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
func solveFancyTermFromPayment(input LoanInput) (int, types.DateRec, []Prepayment, error) {
	clone := input
	loan := clone.Loan
	cap := fancyTermHorizonPeriods(loan.FirstDate, loan.PerYr, clone.Settings.CenturyDiv)
	// The probe walk is forced to cap+1, ONE PERIOD PAST the wall, purely so the
	// horizon can be DETECTED. DOS's own refusal test is on the residual —
	// `if (p > minpmt) then goto ABORT` (AMORTOP.pas:1344-1345) — but the port
	// cannot read that residual off this walk: given an explicit term the engine
	// folds whatever is left into the final row (DOS's own very-last fold), so
	// res.FinalPrinc comes back 0.0000 even for a loan whose balance is GROWING.
	// Walking one period long makes the distinction visible in the COUNT instead:
	// a loan that retires on or before the wall stops early (the engine's
	// early-payoff termination) and yields n <= cap; a loan that never retires
	// runs the whole forced term and yields n == cap+1 > cap. The refusal arm
	// below therefore reads `n > cap`, which admits the wall payment itself —
	// see fancyTermHorizonPeriods for why that payment is inside the horizon.
	walkCap := cap + 1
	loan.NPeriods = walkCap
	loan.NStatus = types.InOutInput
	loan.LastStatus = types.StatusEmpty // let FirstPass derive lastDate
	loan.LastOK = false
	clone.Loan = loan
	// FirstPass will set LastOK back to TRUE off the forced n above. DOS's
	// DetermineLastPaymentDate leaves h^.lastok FALSE throughout, and
	// ValidateInputs' C-A-8/C-A-9 arms are gated on exactly that flag — see the
	// termHorizonWalk note in types.go for the seed-21000 failures this closes.
	clone.termHorizonWalk = true

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
				ps[i].NN = walkCap
				ps[i].NNStatus = types.InOutInput
			}
		}
		clone.Prepayments = ps
	}

	res := Amortize(clone)
	if res.Err != nil {
		return 0, types.DateRec{}, nil, res.Err
	}
	rows := 0
	var last types.DateRec
	for i := range res.Schedule {
		if res.Schedule[i].PayNum >= 1 {
			rows++
			last = res.Schedule[i].Date
		}
	}
	if rows == 0 {
		return 0, types.DateRec{}, nil, fmt.Errorf(
			"The loan term could not be derived — no payments were produced. Check " +
				"Amount Borrowed, Loan Rate and the advanced options, or enter " +
				"# Periods directly.")
	}
	// DOS does NOT report the number of rows it walked. It records the terminal
	// DATE and then re-derives the term from the payment GRID:
	//
	//	h^.lastdate  := NextPayment.date;      { AMORTOP.pas:1362 }
	//	h^.nperiods  := NumberOfInstallments(h^.firstdate, h^.lastdate,
	//	                                     h^.peryr, on_or_before);  { :1378 }
	//
	// The two agree for an ordinary in-arrears walk, which is why the row count
	// stood in for it until now. They part company under IN-ADVANCE, where the
	// schedule carries a settlement row at the loan date and the payment-bearing
	// rows therefore span one MORE grid interval than they number: for
	// first=2024-02-01 the walk ends at 2033-03-01, which is 109 months = 110
	// installments inclusive, but only 110 rows of which the last 109 carry a
	// payment number. Go reported 109 and DOS 110 — and Go's own answer was
	// internally inconsistent, since it had just built a schedule whose span
	// implies 110.
	//
	// Found 2026-07-29: after the oracle centurydiv 20 -> 50 fix unblocked fancy
	// term solves, TestDOSSolverOptionsAudit showed 39 divergences all of the
	// identical shape DOS = Go + 1, and all 39 carried inadv. Deriving from the
	// grid the way DOS does closes the whole class at the source rather than
	// patching a +1 onto the in-advance case.
	//
	// The reported date is the SNAP, not the walk's raw terminal. DOS passes
	// h^.lastdate to NumberOfInstallments by REFERENCE, so :1378 rewrites it:
	// ChoosePaymentDate moves it back onto the payment grid and the tail
	// `if (flast) then l.d:=daysinm(l) else l.d:=f.d` (INTSUTIL.pas:1013)
	// re-stamps the day from the FIRST-payment date. Whenever the walk retires
	// on an OFF-GRID row — a trailing balloon, an off-cycle extra — the snap and
	// the terminal are different dates and DOS shows the snap.
	//
	// Found 2026-07-29, fuzzer5 seed 21004:
	//
	//	amort_oracle 153855.26 0.0874300000 180 12 loandmy=29.8.2023 \
	//	  b138=41809.39 targ=36.50 skip=2,8,11 payhard=1979.13 noterm
	//
	// walks to the tacked balloon at 2/28/2035; DOS reports `lastdate 2/1/2035
	// nperiods 133` — the balloon date snapped back onto the 1st-of-month grid
	// of firstdate 1/2/2024. Go reported the balloon date itself.
	//
	// DOS's write-back can name a day past the end of the month (a from-day of
	// 29/30/31 landing on February leaves a phantom such as 29/2/2035 — see
	// dateutil.NumberOfInstallmentsRaw). types.DateRec cannot hold one, and
	// NORMALIZING it would push the date into the next month, further from DOS
	// than the walk terminal was. CLAMP instead, which is what DOS's own
	// CheckForDaysTooLarge does everywhere a daterec reaches the payment grid.
	//
	// Order matters: the prepayment-window rewrite below runs off the UNSNAPPED
	// terminal, because DOS performs it at :1350-1368 — before the :1378 call
	// that does the snapping.
	n, ry, rm, rd := dateutil.NumberOfInstallmentsRaw(loan.FirstDate, last,
		loan.PerYr, types.OnOrBefore)
	snapped := last
	if n > 0 && rm >= 1 && rm <= 12 {
		if dim := dateutil.DaysInM(types.NewDateRec(ry, time.Month(rm), 1)); rd > dim {
			rd = dim
		}
		snapped = types.NewDateRec(ry, time.Month(rm), rd)
	}
	if n <= 0 {
		n = rows
	}
	// Refuse when the walk ran to the horizon WITHOUT retiring the loan —
	// DOS: "Payment amount is too small to compute number of periods."
	//
	// DOS's own test is on the RESIDUAL, not on any count:
	//
	//	RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, entire, ...);
	//	if (p > minpmt) then goto ABORT;      { AMORTOP.pas:1344-1345 }
	//
	// with `minpmt = 1.0` (AMORTOP.pas:14) — which is exactly the 1.0 below.
	//
	// The `n > cap` arm is the horizon: cap is now DOS's OWN wall rather than an
	// invented 80 years (see fancyTermHorizonPeriods), so a walk that reaches it
	// is a walk DOS would also have cut short with the balance unretired — and
	// DOS then takes the same ABORT. The 2026-07-24 solver-options audit caught
	// Go returning a phantom 959-period "term" where DOS refuses; both arms catch
	// it.
	//
	// A THIRD arm, `rows >= cap`, was here until 2026-07-29 and was WRONG: rows
	// counts every emitted line, and in fancy mode that includes the
	// prepayment-only lines that fall BETWEEN regular payments. An annual loan
	// carrying a biweekly series emits ~26 rows per period, so
	//
	//	amort_oracle 404821.37 0.1136290000 10 1 pre=36:179:26:215.61 \
	//	  payhard=67853.97 noterm
	//
	// walked to n=13, last 2037-1-1, FinalPrinc 0.000000 — a clean retirement
	// matching the oracle's `solvedterm 13 last 2037-1-1` — and was refused
	// anyway because it took 191 rows to get there against a cap of 80. rows is
	// simply not commensurable with cap once extras are in play.
	// DOS's OWN refusal test, on the residual (AMORTOP.pas:1343-1344):
	//
	//	RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, entire,
	//	               no_value_calc, 0);
	//	if (p > minpmt) then goto ABORT;
	//
	// The two arms below cannot see this case. Measured on
	//
	//	amort_oracle 492871.04 0.0321770000 80 4 b365 exact inadv plusreg usa \
	//	  loandmy=29.7.2025 firstdmy=29.11.2025 mor=55 targ=1853.29 \
	//	  pts=0.025401 payhard=8283.92 noterm
	//
	// the wall is 29/11/2049 so cap = 97, and the port returns n = 97 for EVERY
	// payment at or below 8450 — even 7000 — because the walk simply saturates at
	// the wall instead of running past it, so `n > cap` never fires; and the
	// forced-term engine folds the unretired balance into the wall payment, so
	// res.FinalPrinc reads 0.0000 (visible in the totals as 758,343.02 paid on a
	// 492,871.04 loan). DOS accepts from 8475 up (n = 97, landing exactly on the
	// wall) and refuses at and below 8450; a payhard bisect confirmed the two
	// engines agree on the schedule at every accepted point, so this is purely a
	// missing refusal and not a schedule divergence.
	//
	// Read the residual the way DOS does: the UNFORCED terminal (Output=nil) of
	// an ENTIRE walk bounded by the wall. Note DOS passes `entire` HERE, whereas
	// Iterate passes til_adj/false (AMORTOP.pas:1439/1465) — so plain
	// fancyTerminal is not a substitute; entireWalk must be set, or an ARM loan's
	// re-amortizations are skipped and the residual is measured on the wrong walk.
	// minpmt is DOS's own constant 1.0 (AMORTOP.pas:14).
	//
	// 2026-07-30: this single defect accounted for 18 of 50 findings (36%) in an
	// 800-seed sweep — see docs/divergence_corpus_2026-07-30.md.
	// UNCONDITIONAL, as in DOS: DetermineLastPaymentDate has no gate on the count
	// before `if (p > minpmt)`. An `n >= cap` gate was tried first and left two of
	// the eighteen repros divergent (Go retiring at n=51 and n=49 on screens DOS
	// refuses), because a walk can retire early in the DISPLAY sense while the
	// ENTIRE walk — the one DOS measures, with Re_Amortize firing at every
	// adjustment — never gets the balance down. A loan that genuinely retires
	// gives a residual at or below minpmt here (the entire walk's own sub-minpmt
	// stop), so the unconditional form cannot manufacture a refusal.
	horizonResidual := 0.0
	{
		probe := input
		pl := probe.Loan
		pl.NStatus, pl.NPeriods = types.InOutInput, cap
		pl.LastStatus, pl.LastOK = types.StatusEmpty, false
		probe.Loan = pl
		probe.entireWalk = true
		probe.termHorizonWalk = true
		probe.Prepayments = clone.Prepayments
		ps := &probe.Settings
		if ptr, err := ComputeTrueRate(&probe.Loan, ps); err == nil {
			pf := GrowthPerPeriod(&probe.Loan, ps.YrInv)
			horizonResidual = fancyTerminal(probe, payAmtForResidual(input), ps, ptr, pf)
		}
	}
	if n > cap || res.FinalPrinc > 1.0 || horizonResidual > 1.0 {
		return 0, types.DateRec{}, nil, fmt.Errorf(
			"Pmt Amount is too small to pay off the loan within the schedule " +
				"horizon. Raise the Pmt Amount, or enter # Periods directly.")
	}
	return n, snapped, rewritePrepayWindowsAfterTermSolve(input.Prepayments, last), nil
}

// rewritePrepayWindowsAfterTermSolve ports the post-walk prepayment-window
// rewrite DOS performs inside DetermineLastPaymentDate once the probe walk has
// found the terminal date (AMORTOP.pas:1350-1368):
//
//	npre := save_balloon.save_npre;
//	for i := 1 to npre do
//	  if (pre[i]^.stopdatestatus<defp) then
//	    with pre[i]^ do
//	      begin
//	        calc_pre[i].stopdate := nextdate;
//	        while (DateComp(h^.lastdate, calc_pre[i].stopdate) < 0) do
//	          AddPeriod(calc_pre[i].stopdate, pre[i]^.peryr, startdate.d, subtract);
//	                {go back to on or before last regular payment date}
//	        calc_pre[i].nn := NumberOfInstallments(startdate,
//	                            calc_pre[i].stopdate, pre[i]^.peryr, on_or_before);
//	      end;
//	... { after save_balloon.Restore }
//	    pre[i]^.stopdate := calc_pre[i].stopdate;  pre[i]^.stopdatestatus := outp;
//	    pre[i]^.nn       := calc_pre[i].nn;        pre[i]^.nnstatus       := outp;
//
// The window it writes back is NOT the window the probe walk used. `nextdate` is
// the series CURSOR, and CheckOffBalloon (AMORTOP.pas:555-570) advances it AFTER
// each extra is applied, dropping the series from npre only once the advanced
// cursor passes stopdate — so at walk end the cursor sits exactly ONE series
// period PAST the last extra actually paid. The step-back loop only runs while
// the cursor is LATER than the solved last date, so whenever the series ran to
// exhaustion well inside the term (cursor <= lastdate) nothing steps back and
// the series is re-stamped ONE PERIOD LONGER than the user entered. The table
// DOS finally renders then carries nn+1 extras.
//
// That is a DOS quirk, not a rounding artifact, and it is observable: the
// backward-solve fuzzer (seed 21000, reduced) produced
//
//	amort_oracle 55326.46 0.0596870000 96 12 pre=35:46:12:73.48 \
//	  payhard=951.21 noterm
//
// where DOS solves term 120 (last 2034-1-1) and then applies 47 extras — the
// last on 2030-10-01, one month past the 46th and final entered extra — for
// interest 18533.27, while Go applied the entered 46 and got 18345.90. DOS's own
// FIXED-n=120 table for the same loan gives 18345.90, i.e. DOS disagrees with
// itself precisely by this rewrite; Go matched the fixed-n table.
//
// The gate is `stopdatestatus < defp`, which reads as "the stop date was DERIVED
// rather than entered". CheckPrepayments (AMORTOP.pas:420-421) writes
// `stopdatestatus := outp` for every COUNT-specified series, and outp (1) is
// below defp (2), so a count-specified series is always rewritten while a series
// the user pinned with an explicit stop DATE is left alone.
//
// With two or more series the rewrite is additionally SLOT-CROSSED, and that is
// reproduced here rather than approximated. `pre` is an array of POINTERS, and
// CheckOffBalloon retires an exhausted series by copying record CONTENTS down
// the array — `pre[j]^ := pre[succ(j)]^` — while leaving the pointers put. So
// when series A (slot 1) exhausts first, slot 1's RECORD becomes a copy of
// series B's record as of that instant, the walk goes on advancing B's cursor in
// slot 1, and slot 2 is frozen holding B's mid-walk snapshot. `npre` is then
// restored to its pre-walk value, so the rewrite loop reads BOTH slots: slot 1
// yields B's true final cursor and slot 2 yields B's stale one. save_balloon
// .Restore then puts the ORIGINAL records back (A in slot 1, B in slot 2) before
// calc_pre is written into them — so A inherits B's fully-walked window and B is
// truncated to where it stood when A retired.
//
// Reduced from the same seed-21000 run:
//
//	amort_oracle 138147.06 0.1226820000 144 12 plusreg \
//	  pre=93:31:26:211.81 pre=72:140:24:54.67 payhard=1851.97 noterm
//
// where each series ALONE reproduces DOS exactly (dInt=0.00 both ways) and the
// pair diverges by 911.79 — the signature of a cross-slot effect rather than a
// per-series one.
func rewritePrepayWindowsAfterTermSolve(pres []Prepayment, last types.DateRec) []Prepayment {
	if len(pres) == 0 || !dateutil.DateOK(last) {
		return pres
	}

	// slot mirrors the CONTENTS of one pre[i]^ record through the walk.
	type slot struct {
		start   types.DateRec
		stop    types.DateRec
		bounded bool // false = no stop date and no count: never exhausts
		peryr   int
		day     int  // DOS's `startdate.d` AddPeriod anchor
		status  int8 // stopdatestatus, read by the rewrite's own gate
		derived bool // stop date came from the COUNT (DOS's `stopdatestatus := outp`)
		live    bool // participates in the walk at all
		cursor  types.DateRec
		applied bool // scratch: this slot took the current extra
	}
	recs := make([]slot, len(pres))
	for i := range pres {
		pp := &pres[i]
		s := slot{
			start:   pp.StartDate,
			stop:    pp.StopDate,
			peryr:   pp.PerYr,
			day:     pp.originDay(),
			status:  pp.StopDateStatus,
			derived: pp.stopFromNN,
			cursor:  pp.StartDate,
		}
		s.live = pp.StartDateStatus >= types.InOutDefault &&
			pp.PerYrStatus >= types.InOutDefault && pp.PerYr > 0 &&
			pp.PaymentStatus >= types.InOutDefault && dateutil.DateOK(pp.StartDate)
		switch {
		case pp.StopDateStatus >= types.InOutDefault && dateutil.DateOK(pp.StopDate):
			s.bounded = true
		case pp.NNStatus >= types.InOutDefault && pp.NN > 0:
			// CheckPrepayments, AMORTOP.pas:416-419: the count fixes the window at
			// AddNPeriods(startdate, .., pred(nn)). It must be AddNPeriods and not
			// nn-1 iterated AddPeriod steps — the two part company for peryr=24 with
			// an off-grid anchor day (see CheckPrepaymentStops). Normally the prepass
			// has already filled StopDate in and the case above takes it; this arm
			// covers a record that reached the solver without it.
			s.bounded = true
			s.derived = true
			if sd, e := dateutil.AddNPeriods(pp.StartDate, s.peryr, pp.NN-1); e == nil {
				s.stop = sd
			}
		}
		recs[i] = s
	}
	// Compact the dead entries out the way CheckPrepayments' `blank[]` pass does
	// (AMORTOP.pas:461-465), so slot indices match DOS's.
	live := make([]slot, 0, len(recs))
	for _, s := range recs {
		if s.live {
			live = append(live, s)
		}
	}
	npre := len(live)
	if npre == 0 {
		return pres
	}

	// Walk the merged extra-payment timeline exactly as FindNextExtra +
	// CheckOffBalloon do: take the earliest live cursor, apply every slot
	// sharing that date, advance those cursors, and compact out any slot whose
	// advanced cursor passed its stop date.
	for guard := 0; npre > 0 && guard < 100000; guard++ {
		next := live[0].cursor
		for i := 1; i < npre; i++ {
			if dateutil.DateComp(live[i].cursor, next) < 0 {
				next = live[i].cursor
			}
		}
		if dateutil.DateComp(next, last) > 0 {
			break
		}
		for i := 0; i < npre; i++ {
			live[i].applied = dateutil.DateComp(live[i].cursor, next) == 0
		}
		for i := 0; i < npre; i++ {
			if !live[i].applied {
				continue
			}
			nd, e := dateutil.AddPeriod(live[i].cursor, live[i].peryr, live[i].day, false)
			if e != nil {
				npre = 0
				break
			}
			live[i].cursor = nd
			if live[i].bounded && dateutil.DateComp(nd, live[i].stop) > 0 {
				npre--
				for j := i; j < npre; j++ {
					live[j] = live[j+1]
				}
				i--
			}
		}
	}

	// `npre := save_balloon.save_npre` — the rewrite reads every slot the screen
	// had, including the ones the compaction left holding another series' record.
	out := make([]Prepayment, len(pres))
	copy(out, pres)
	changed := false
	li := 0
	for i := range out {
		pp := &out[i]
		if !recs[i].live {
			continue
		}
		src := live[li] // slot i's CONTENTS at walk end, corruption included
		li++
		// DOS's gate, read from the (possibly crossed) slot.
		//
		//	if (pre[i]^.stopdatestatus < defp) then <rewrite>
		//
		// A stop date DERIVED from the count carries `stopdatestatus := outp` in
		// DOS — below defp — so DOS rewrites it. The port stores derived windows as
		// present (see Prepayment.stopFromNN for why), so the derived-ness has to be
		// read from the flag or every count-specified series would skip the rewrite
		// and its terminating balloon would land years late.
		if src.status >= types.InOutDefault && !src.derived {
			continue
		}
		cursor := src.cursor
		// `while (DateComp(h^.lastdate, calc_pre[i].stopdate) < 0)`
		//
		// DOS's loop has NO floor: it keeps subtracting periods until the cursor is
		// on-or-before h^.lastdate, even when that carries it back PAST the series'
		// own start date. The port used to stop at `start`, which is not a bound
		// DOS has, and the difference is visible whenever a series begins AFTER the
		// solved last payment date — a screen the fuzzer reaches easily because the
		// prepay start offset is drawn independently of the (solved) term:
		//
		//	amort_oracle 495562.25 0.0412700000 100 4 inadv r78 usa \
		//	  loandmy=10.5.2025 firstdmy=10.8.2025 mor=24 b33=35181.54 \
		//	  b159=126040.01 b165=43422.98 pre=204:348:52:100.44 \
		//	  pre=9:140:26:136.08 targ=253.10 pts=0.000115 payhard=9454.33 noterm
		//
		// Series 1 starts 5/10/2042, 204 months after the loan date; DOS solves
		// term 64, last 5/10/2041. Its cursor never moves (the walk ends before the
		// series opens), so the step-back loop runs from 5/10/2042 all the way back
		// to on-or-before 5/10/2041 and the stored stopdate lands BEFORE the start
		// date. DetermineVeryLast (`if (DateComp(pre[i]^.stopdate, very_last) > 0)`)
		// therefore cannot raise very_last, and DOS tacks its terminating balloon on
		// h^.lastdate, 5/10/2041. With the floor in place the port left the cursor
		// at 5/10/2042 and tacked there instead — same amount, 7261.19, one year
		// late. 2026-07-29 fuzzer5 seed 21001.
		//
		// The loop is monotonically decreasing so it terminates on its own; the
		// counter is only a backstop against an AddPeriod that fails to move.
		for guard := 0; dateutil.DateComp(last, cursor) < 0 && guard < 200000; guard++ {
			pd, e := dateutil.AddPeriod(cursor, src.peryr, src.day, true)
			if e != nil || dateutil.DateComp(pd, cursor) >= 0 {
				break
			}
			cursor = pd
		}
		// `calc_pre[i].nn := NumberOfInstallments(startdate, calc_pre[i].stopdate,
		//                      pre[i]^.peryr, on_or_before)` — startdate and peryr
		// also come from the crossed slot, but the value lands in pp, whose own
		// StartDate/PerYr save_balloon.Restore has just put back.
		//
		// When the step-back ran past the start date this count comes out ZERO or
		// NEGATIVE — for peryr 52 DOS computes `succ((Julian(l)-Julian(f)) div 7)`
		// (INTSUTIL.pas:1030/1039) with a negative numerator. DOS stores it anyway:
		// the write-back below is unconditional in the Pascal, and every Go reader
		// of Prepayment.NN already gates on `NN > 0`, so a non-positive count reads
		// as "no count bound" exactly the way DOS's render walk treats it — the
		// window is carried by the stop DATE, which is now before the start.
		//
		// THE VAR-PARAMETER SNAP. `calc_pre[i].stopdate` is passed as the VAR `l`
		// argument (AMORTOP.pas:1358), and NumberOfInstallments MUTATES `l` — its
		// monthly branch ends
		//
		//	if (mdiff=0) then case z of
		//	   ...
		//	   on_or_before : if (ddiff<0) and (not (flast and llast)) then
		//	                     l.m := l.m - monthsbtwn;
		//	   ...
		//	if (l.m<=0) then begin dec(l.y); l.m:=l.m+12; end
		//	else if (l.m>12) then begin inc(l.y); l.m:=l.m-12; end;
		//	if (flast) then l.d:=daysinm(l) else l.d:=f.d;
		//	                                        (INTSUTIL.pas:1004-1018)
		//
		// so the date DOS writes back at :1364 is the SNAPPED one, not the cursor
		// the step-back loop left. The port stored `cursor`, and the two differ
		// whenever the cursor is not already on the series' own grid — which is the
		// normal case, because the step-back loop walks the cursor on the WALK-END
		// slot's grid while the snap re-measures it against `src.start`'s day.
		//
		// 2026-07-29 fuzzer5 seed 21047:
		//
		//	amort_oracle 62345.23 0.0313070000 24 2 b365 exact plusreg \
		//	  loandmy=30.10.2024 firstdmy=28.2.2025 b22=13295.15 b70=5434.74 \
		//	  pre=82:3:12:50.89 targ=424.61 pts=0.035133 payhard=3899.53 noterm
		//
		// The DLPD trace shows both sides agreeing through the step-back —
		// `DLPD pre1 steppedback stop=2/28/131` — and then DOS's stored date coming
		// out as 1/30/2031 (`DLPD pre1 nn=-6 finalstop=1/30/131`, confirmed by
		// `pdump`: `stop 1/30/2031`). By hand: f=8/30/2031, l=2/28/2031, peryr=12,
		// z=on_or_before ⇒ orig_day=30, flast=false, llast=true, ddiff=-2, mdiff=0,
		// so `(ddiff<0) and (not (flast and llast))` fires ⇒ l.m := 1, then
		// `l.d := f.d = 30` ⇒ 1/30/2031.
		//
		// The whole visible symptom was the terminating balloon. With the stop date
		// left at the un-snapped 2/28/2031 it collided with very_last, so
		// veryLastRegularAmount (tackon.go:93, AMORTOP.pas:1306-1320) returned the
		// PREPAYMENT amount 50.89 instead of the regular payment 3899.53, and the
		// PlusRegular subtraction produced 3245.88 - 50.89 = 3194.99 where DOS has
		// 3245.88 - 3899.53 = -653.65. The tack-on WALK itself was byte-perfect.
		//
		// The normalized form is what goes into pp.StopDate: the snap can name a
		// day past the end of its month (a phantom daterec), but every consumer of
		// a prepayment stop date only COMPARES it, and no payment on the series'
		// grid can fall strictly between the phantom and its normalization — see
		// the NumberOfInstallmentsRaw doc comment in internal/dateutil.
		nn, snapped := dateutil.NumberOfInstallments(src.start, cursor, src.peryr,
			types.OnOrBefore)
		if nn == pp.NN && pp.NNStatus >= types.InOutDefault &&
			pp.StopDateStatus >= types.InOutDefault &&
			dateutil.DateComp(pp.StopDate, snapped) == 0 {
			continue
		}
		// The COUNT is written back as DOS writes it — value plus `nnstatus :=
		// outp` — and outp (1) is BELOW the port's ubiquitous `>= InOutDefault`
		// presence threshold, so every count-cap in the render walk
		// (engine.go:3980/4091/4210/4391, `NNStatus >= InOutDefault && NN > 0 &&
		// prepayApplied[i] >= NN`) goes quiet on a rewritten row. That is exactly
		// right: DOS's render walk NEVER consults nn. Its only bound is the stop
		// date —
		//
		//	AddPeriod(nextdate, pre[i]^.peryr, pre[i]^.startdate.d, add);
		//	if (DateComp(nextdate, stopdate) > 0) then begin dec(npre); ... end;
		//	                                     (CheckOffBalloon, AMORTOP.pas:558-561)
		//
		// — and nn is a display cell the screen shows alongside it.
		//
		// Before the rewrite the two agree by construction: CheckPrepayments
		// derives stopdate = AddNPeriods(startdate, peryr, pred(nn))
		// (AMORTOP.pas:414), so capping on either gives the same window and the
		// port's count-cap is harmless. The rewrite is the one place they come
		// apart, because it takes the DATE from the walk-end cursor and the COUNT
		// from NumberOfInstallments over the CROSSED slot's start/peryr. Whenever a
		// slot crossed, the count is measured on a grid that is not this row's.
		//
		// Seed 21001, reduced:
		//
		//	amort_oracle 179963.08 0.0736260000 228 12 pre=20:377:52:18.86 \
		//	  pre=132:41:12:258.95 payhard=1927.08 noterm
		//
		// The rewrite itself already matched DOS cell for cell — `pdump` shows both
		// sides producing `start 9/1/2025 stop 6/1/2038 nn 42 peryr 52` on row 1,
		// nn 42 being series 2's monthly count applied to series 1's WEEKLY row.
		// DOS then walks that row weekly from 9/1/2025 to 6/1/2038, some 666
		// extras; the port stopped at 42 and came in 12551.24 light on interest.
		//
		// An earlier cut of this function did set outp and DID regress
		// (dInt=-187.37 → +3442.68), but that cut stored ONLY the count and left
		// every consumer to re-derive the date from it — so dropping the count
		// below the threshold dropped the whole window. The stop DATE has been
		// written back since 2026-07-29 (see below), and it is written as PRESENT,
		// so the bound the walk actually needs is carried by the cell DOS's walk
		// actually reads. 2026-07-29 fuzzer5 seed 21001.
		pp.NN, pp.NNStatus = nn, types.InOutOutput
		// The stop DATE is written back too, and it must be — DOS's write-back is
		//
		//	pre[i]^.stopdate := calc_pre[i].stopdate;  stopdatestatus := outp;
		//	pre[i]^.nn       := calc_pre[i].nn;        nnstatus       := outp;
		//
		// i.e. the date is stored DIRECTLY, and the count is a second, separately
		// stored fact. Until 2026-07-29 the port stored only the count and left
		// every consumer to RE-DERIVE the date from it. For a single series that is
		// equivalent — cursor was walked on that series' own grid, so
		// start + (nn-1) periods lands back on cursor. Under the slot-crossing
		// described above it is NOT: `nn` is counted on the CROSSED slot's grid
		// (src.start, src.peryr) while every re-derivation site uses the row's OWN
		// StartDate/PerYr, which save_balloon.Restore has just put back. The two
		// grids need not even share a period length.
		//
		// Seed 21000 caught it through the terminating balloon, which
		// tackOnFinalBalloon dates from determineVeryLast (AMORTOP.pas:1293-1304,
		// `if (DateComp(pre[i]^.stopdate, very_last) > 0) then very_last := ...`):
		//
		//	amort_oracle 138147.06 0.1226820000 144 12 b365 exact inadv plusreg \
		//	  usa loandmy=14.3.2024 firstdmy=14.4.2024 mor=35 b95=36687.17 \
		//	  pre=93:31:26:211.81 pre=72:140:24:54.67 targ=439.57 pts=0.017108 \
		//	  payhard=1851.97 noterm
		//
		// DOS tacks 12/14/2034 1212.67 — very_last is simply h^.lastdate, because
		// BOTH rewritten stop dates were stepped back to on-or-before it by the
		// `while (DateComp(h^.lastdate, calc_pre[i].stopdate) < 0)` loop above, so
		// neither can raise very_last. Go tacked 4/27/2036 -19796.40: series 1
		// (start 12/14/2031, peryr 26) had inherited series 2's count across the
		// crossed slot, and 12/14/2031 + 114 fortnights — 1596 days, exactly
		// 114 x 14 — is 4/27/2036. The count was series 2's; the grid it got walked
		// on was series 1's; the product was a date DOS never computes and that is
		// not even on the payment grid.
		//
		// Writing the date removes the re-derivation entirely, and it cannot
		// reintroduce the presence-filter hazard the NN comment above describes:
		// StopDateStatus goes to InOutInput (present) rather than DOS's outp, so
		// consumers that branch on an entered stop date take that branch with the
		// value DOS itself stored there. The post-walk state is exactly DOS's —
		// both cells filled, both marked computed.
		//
		// `snapped`, not `cursor` — see the var-parameter-snap note above the
		// NumberOfInstallments call.
		pp.StopDate, pp.StopDateStatus = snapped, types.InOutInput
		// The window is now DOS's rewritten one, not the count's — clear the flag so
		// a later prepass or a second rewrite leaves it alone.
		pp.stopFromNN = false
		changed = true
	}
	if !changed {
		return pres
	}
	return out
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

// payAmtForResidual is the regular payment DOS's DetermineLastPaymentDate walks
// with — the screen's payment cell, which by construction is present whenever the
// term is being solved FROM the payment.
func payAmtForResidual(input LoanInput) float64 {
	return input.Loan.PayAmt
}
