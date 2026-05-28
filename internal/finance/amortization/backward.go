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

// SolvePayment computes the periodic payment amount from amount + rate
// + term using the closed-form annuity formula:
//
//	d = amount * (f - 1) / (1 - 1/f^n)
//
// where f = GrowthPerPeriod. This mirrors DOS
// EstimateAndRefinePayment's fast-path at Amortize.pas:377-430 — the
// closed-form direct assignment that applies when no fancy features
// (prepayments, balloons, adjustments, in_advance, target, skip-months)
// are active. For fancy loans the result is a useful initial estimate
// but exact balloon/adjustment-aware solving still requires iteration
// in the schedule engine.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefinePayment.
func SolvePayment(input LoanInput) (float64, error) {
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
	return loan.Amount * (f - 1) / denom, nil
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
// For fancy schedules (prepayments + adjustments) the closed-form
// answer is the first estimate; iterateRefineAmount then runs a Newton
// refinement against the fancy engine's terminal balance.
//
// The second return value is a convergence flag: true when the
// closed-form solve was sufficient or the fancy refinement converged
// within the iterateHalfpenny tolerance; false when the fancy
// refinement bailed out at iterateMaxCount trials without converging,
// in which case the caller is expected to surface a "did not
// converge" warning to the user (matching the DOS MessageBox).
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefineLoanAmount + AMORTOP.pas: function Iterate.
func SolveLoanAmount(input LoanInput) (float64, bool, error) {
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

	// Fancy-mode refinement (Iterate). When the schedule carries
	// prepayments or adjustments, the closed-form is only a first
	// estimate; iterateRefineAmount runs the fancy engine against
	// trial principals and refines via finite-difference Newton.
	// For plain non-fancy loans this branch is skipped and the
	// closed form is returned directly (converged=true).
	if input.Fancy && len(input.Prepayments)+len(input.Adjustments) > 0 {
		refined, ok := iterateRefineAmount(input, estimate)
		if ok {
			return refined, true, nil
		}
		// Refinement bailed without converging; the caller is
		// expected to surface a "did not converge" warning. Return
		// the best-seen estimate (the refined value) so the
		// downstream schedule call still has something usable.
		return refined, false, nil
	}
	return estimate, true, nil
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
// For fancy schedules (prepayments + adjustments), iterateRefineRate
// refines the closed-form estimate against the fancy engine's
// terminal balance after the closed-form Newton loop converges.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefineRate + AMORTOP.pas: function Iterate.
func SolveRate(input LoanInput) (float64, bool, error) {
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
		if rate < 0 {
			rate = small
		}
		loan.LoanRate = rate
		if math.Abs(step) < teeny {
			// Closed-form converged. For fancy loans, refine against
			// the schedule engine — the closed form ignores prepayments
			// and adjustments, so the rate it lands on can be off.
			if input.Fancy && len(input.Prepayments)+len(input.Adjustments) > 0 {
				refined, ok := iterateRefineRate(input, rate)
				if ok {
					return refined, true, nil
				}
				// Refinement bailed. Return the best-seen refined rate
				// with converged=false so the handler surfaces a "did
				// not converge" warning — matching SolveLoanAmount's
				// behavior on this path.
				return refined, false, nil
			}
			return rate, true, nil
		}
	}
	return rate, false, nil
}

// --- Iterate (Newton's-method refinement against the fancy engine) -------
//
// Ported from legacy/src/dos_source/AMORTOP.pas: function Iterate
// (lines 1415-1497). The DOS version is a generic finite-difference
// Newton refinement that drives the fancy schedule's terminal residual
// balance to zero by adjusting one variable (the principal, a balloon
// amount, a prepayment amount, or the loan rate). This port preserves
// the same iteration constants — 20 trials, halfpenny success
// threshold, acc_limit = 2e-8 — and tracks bestp/bestx so the caller
// gets the best-seen estimate even on non-convergence.
//
// The "residual" here is the principal balance at the end of the
// scheduled term: positive means the loan under-amortizes (we need a
// larger principal / lower rate), zero means the schedule retires
// exactly at term, negative means the engine retired the loan early
// (over-amortized). The sign convention matches the DOS Iterate.

const (
	iterateMaxCount   = 20
	iterateHalfpenny  = 0.005
	iterateAccLimit   = 2e-8
	iterateSmallDelta = 0.001
)

// fancyResidual runs Amortize and returns the terminal balance after
// the engine completes the scheduled term. The signal convention:
//
//   - residual > 0 if the loan still owes principal at end of term.
//   - residual ≈ 0 if the loan amortizes exactly.
//   - residual < 0 if the engine retired the loan early (i.e. the
//     candidate variable caused excess paydown).
//
// We approximate "retired early, by how much" by counting payments
// the engine skipped and multiplying by the per-period payment — that
// gives a finite signed residual that Newton can drive toward zero.
func fancyResidual(input LoanInput) float64 {
	res := Amortize(input)
	if res.Err != nil || len(res.Schedule) == 0 {
		return math.Inf(1)
	}
	last := res.Schedule[len(res.Schedule)-1]
	if len(res.Schedule) < input.Loan.NPeriods {
		// Loan retired early by (NPeriods - len) payments. Treat the
		// signed magnitude as -payment * (skipped count).
		skipped := input.Loan.NPeriods - len(res.Schedule)
		return -float64(skipped) * input.Loan.PayAmt
	}
	return last.Principal
}

// iterateRefineAmount refines a candidate principal `x0` so that
// fancyResidual(input with Amount = x) → 0. Returns the refined
// amount and true on convergence, or the best-seen amount and false
// otherwise.
func iterateRefineAmount(input LoanInput, x0 float64) (float64, bool) {
	getX := func(in *LoanInput) float64 { return in.Loan.Amount }
	setX := func(in *LoanInput, x float64) { in.Loan.Amount = x }
	return iterateNewton(input, getX, setX, x0, true /* target is principal */)
}

// iterateRefineRate refines a candidate rate `r0` so that
// fancyResidual(input with LoanRate = r) → 0.
func iterateRefineRate(input LoanInput, r0 float64) (float64, bool) {
	getX := func(in *LoanInput) float64 { return in.Loan.LoanRate }
	setX := func(in *LoanInput, x float64) { in.Loan.LoanRate = x }
	return iterateNewton(input, getX, setX, r0, false /* target is rate */)
}

// iterateNewton is the generic Newton-with-finite-differences loop
// ported from DOS Iterate. The targetIsPrincipal flag mirrors DOS's
// target_is_loan_amount check — when the iterate variable IS the
// principal, the engine should be re-fed the latest x as principal;
// otherwise the principal stays at its original input value.
func iterateNewton(
	input LoanInput,
	getX func(*LoanInput) float64,
	setX func(*LoanInput, float64),
	x0 float64,
	targetIsPrincipal bool,
) (float64, bool) {
	if x0 == 0 || math.IsNaN(x0) || math.IsInf(x0, 0) {
		return x0, false
	}

	// Snapshot the input so iterations are independent. The closure
	// re-applies the snapshot's principal each iteration unless the
	// target IS the principal.
	origAmount := input.Loan.Amount

	// CRITICAL: We are inside a backward solver — the variable we're
	// iterating on (Amount or LoanRate) has StatusEmpty on the input
	// because that's how the caller asked us to solve for it. But
	// fancyResidual calls Amortize, which rejects inputs whose
	// AmountStatus / LoanRateStatus are not at InOutDefault or higher.
	// Promote both to InOutInput on our local copy so each Amortize
	// trial sees a fully-specified loan. Both fields now hold a
	// candidate value (the closed-form estimate from the caller, plus
	// whatever the iteration is trialing), so InOutInput is honest.
	input.Loan.AmountStatus = types.InOutInput
	input.Loan.LoanRateStatus = types.InOutInput

	// domainOK reports whether a trial value is in the variable's valid
	// range. A loan principal must be positive; an interest rate must be
	// positive and below a sane ceiling (200% annual). Trials outside
	// the domain are treated like an engine refusal so the iteration
	// pulls back instead of chasing a meaningless residual into
	// nonsense (e.g. a negative rate, as seen when the inputs have no
	// positive-rate solution).
	domainOK := func(x float64) bool {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
		if targetIsPrincipal {
			return x > 0
		}
		return x > teeny && x < 2.0
	}

	setX(&input, x0)
	if targetIsPrincipal {
		input.Loan.Amount = x0
	}
	final := fancyResidual(input)
	if math.IsInf(final, 0) {
		return x0, false
	}
	if math.Abs(final) < iterateHalfpenny {
		return x0, true
	}

	delta := iterateSmallDelta * x0
	if delta == 0 {
		delta = iterateSmallDelta
	}
	x := x0 + delta

	bestP := math.Abs(final)
	bestX := x0

	for count := 0; count < iterateMaxCount; count++ {
		setX(&input, x)
		if targetIsPrincipal {
			input.Loan.Amount = x
		} else {
			input.Loan.Amount = origAmount
		}
		// Capture the x we actually measured at, so the best-seen
		// tracker can record it correctly (DOS's original Iterate is
		// careless here and reports bestX as x AFTER the step; that
		// mismatch can leave the caller with a value that doesn't
		// correspond to the measured bestP).
		measuredAtX := x
		var p float64
		if domainOK(x) {
			p = fancyResidual(input)
		} else {
			p = math.Inf(1)
		}
		if math.IsInf(p, 0) {
			// Engine refused this candidate, or the trial left the valid
			// domain (negative/absurd rate or principal). Pull back
			// toward the best-seen x and shrink delta.
			delta = delta / 2
			x = bestX + delta
			continue
		}

		var newDelta float64
		if math.Abs(final-p) > teeny {
			newDelta = delta * p / (final - p)
		}
		if math.Abs(delta) < teeny || math.Abs(newDelta/delta) > 1 {
			// Diverging — DOS bumps count by 5 to bail early.
			count += 5
		}
		delta = newDelta
		x = x + delta
		final = p

		if math.Abs(p) < bestP {
			bestP = math.Abs(p)
			bestX = measuredAtX
		}
		if bestP < iterateHalfpenny {
			break
		}
	}

	// Mirror DOS's success criterion: ok if either |bestP| under the
	// halfpenny floor, or under acc_limit × the *returned* value (a
	// relative tolerance for very large loans). Note this uses bestX,
	// not input.Loan.Amount: the latter holds the last trial value,
	// which on a runaway can be absurdly large and would make the
	// relative test pass spuriously.
	ok := bestP < iterateHalfpenny ||
		bestP < iterateAccLimit*math.Abs(bestX)

	// Trust guard. When the terminal-balance residual is insensitive to
	// the iterate variable — which happens when a rate adjustment
	// recasts the payment to fully amortize whatever principal it is
	// given, flattening the signal — the finite-difference step has no
	// real gradient and Newton wanders far from the true answer while
	// the (near-constant) residual still looks "converged". Detect that
	// by distance from the closed-form estimate x0: a believable
	// refinement stays within a small multiple of it. If the result
	// strayed beyond that band (or went non-positive), discard it and
	// fall back to the closed-form estimate with converged=false, so the
	// caller surfaces a "did not converge" advisory instead of a wild
	// number. A legitimate refinement moves x0 by only a few percent, so
	// this never rejects a good solve.
	const maxStray = 8.0
	strayed := bestX <= 0 ||
		(x0 != 0 && (math.Abs(bestX) > maxStray*math.Abs(x0) ||
			math.Abs(bestX) < math.Abs(x0)/maxStray))
	if strayed {
		return x0, false
	}
	return bestX, ok
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
	prorate := 1.0
	if dateutil.DateOK(loan.LoanDate) && dateutil.DateOK(loan.FirstDate) {
		ydif := dateutil.YearsDif(loan.FirstDate, loan.LoanDate,
			settings.Basis, settings.YrInv, true)
		if pr := ydif * float64(loan.PerYr); pr > 0 {
			prorate = pr
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
		return 0, fmt.Errorf("The Pmt Amount does not produce a valid loan term. " +
			"Check the Pmt Amount and Loan Rate, or enter # Periods directly.")
	}
	return n, nil
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
	// eval runs the full schedule with the unknown balloon pinned to
	// amt and returns the residual final balance.
	eval := func(amt float64) (float64, error) {
		clone := input
		bs := make([]BalloonPayment, len(input.Balloons))
		copy(bs, input.Balloons)
		bs[unknownIdx].Amount = amt
		bs[unknownIdx].AmountStatus = types.InOutInput
		clone.Balloons = bs
		res := Amortize(clone)
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
		if a2 < 0 {
			a2 = 0
		}
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

// SolvePrepaymentAmount solves the per-payment amount of the
// prepayment series at unknownIdx so the schedule's final balance
// lands at zero — the DOS "unknown prepayment". The series must be
// bounded (StopDate or NN) for the solve to be well posed; the amount
// is found by a secant iteration over the final-balance function.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefinePeriodicPrepayment.
func SolvePrepaymentAmount(input LoanInput, unknownIdx int) (float64, error) {
	eval := func(amt float64) (float64, error) {
		clone := input
		ps := make([]Prepayment, len(input.Prepayments))
		copy(ps, input.Prepayments)
		ps[unknownIdx].Payment = amt
		ps[unknownIdx].PaymentStatus = types.InOutInput
		clone.Prepayments = ps
		res := Amortize(clone)
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
		if a2 < 0 {
			a2 = 0
		}
		a0, f0 = a1, f1
		a1 = a2
	}
	return a1, nil
}

// SolvePrepaymentDuration solves how long the prepayment series at
// unknownIdx must run to retire the loan — the DOS "unknown
// prepayment duration". The series has a known amount but no stop
// date and no payment count; the engine runs it effectively
// unbounded, observes when the loan pays off, and reports the number
// of extra payments (NN) and the corresponding stop date.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// DeterminePrepaymentDuration.
func SolvePrepaymentDuration(input LoanInput, unknownIdx int) (int, types.DateRec, error) {
	// Run the schedule with the series effectively unbounded (a very
	// large NN) so the loan pays off as early as the prepayments
	// allow.
	clone := input
	ps := make([]Prepayment, len(input.Prepayments))
	copy(ps, input.Prepayments)
	ps[unknownIdx].NNStatus = types.InOutInput
	ps[unknownIdx].NN = 1 << 20 // effectively unbounded
	clone.Prepayments = ps
	res := Amortize(clone)
	if res.Err != nil {
		return 0, types.DateRec{}, res.Err
	}
	if len(res.Schedule) == 0 {
		return 0, types.DateRec{}, fmt.Errorf("The Prepayment duration could not be " +
			"solved — no schedule rows were produced. Check the loan terms and the " +
			"Prepayment start date.")
	}
	payoff := res.Schedule[len(res.Schedule)-1].Date

	// Count prepayment occurrences from the start date up to the
	// payoff date, stepping by the series period.
	pp := input.Prepayments[unknownIdx]
	count := 0
	last := pp.StartDate
	d := pp.StartDate
	for dateutil.DateComp(d, payoff) <= 0 {
		count++
		last = d
		next, err := dateutil.AddPeriod(d, pp.PerYr, pp.StartDate.Time.Day(), false)
		if err != nil {
			break
		}
		d = next
	}
	return count, last, nil
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
		if r2 < 0 {
			r2 = 0
		}
		r0, g0 = r1, g1
		r1 = r2
	}
	return r1, false
}
