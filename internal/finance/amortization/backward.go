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
// TODO: verify logic — the DOS version calls Iterate after the
// closed-form estimate when prepayments or adjustments exist. This
// port only handles the closed-form case and balloons.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefineLoanAmount.
func SolveLoanAmount(input LoanInput) (float64, error) {
	loan := input.Loan
	settings := input.Settings

	if !CanComputeLoanAmount(&loan) {
		return 0, fmt.Errorf("Amount Borrowed cannot be solved yet. To solve the loan " +
			"amount, leave Amount Borrowed blank and fill in Loan Rate, Pmt Amount, " +
			"Pmts/Yr and 1st Pmt Date.")
	}

	// C-A-10: DOS Amortize.pas:Enter rejects "solve for loan amount"
	// when fancy mode is active AND a target principal-reduction is
	// in force. The two constraints over-determine the system because
	// target requires a known principal to enforce a per-period floor.
	if input.Fancy && input.Target.TargetStatus >= types.InOutDefault &&
		input.Target.TargetValue > 0 {
		return 0, fmt.Errorf("Amount Borrowed cannot be solved while a principal " +
			"reduction Target is set — the Target needs a known loan amount to work " +
			"from. Clear the Target, or enter Amount Borrowed directly.")
	}

	f := GrowthPerPeriod(&loan, settings.YrInv)
	if math.Abs(f-1) < tiny {
		return 0, fmt.Errorf("Amount Borrowed cannot be solved because Loan Rate is " +
			"effectively zero. Enter a non-zero Loan Rate, or enter Amount Borrowed " +
			"directly.")
	}
	if loan.NPeriods <= 0 {
		return 0, fmt.Errorf("# Periods is blank or zero, so Amount Borrowed cannot " +
			"be solved. Enter # Periods, or supply 1st Pmt Date and Last Pmt Date.")
	}

	rate, err := interest.RateFromYield(loan.LoanRate, byte(loan.PerYr), settings.YrDays)
	if err != nil {
		return 0, err
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
			return 0, err
		}
		padj += b.Amount * expVal
	}

	lnf, err := interest.Lnn(f)
	if err != nil {
		return 0, err
	}
	expVal, err := interest.Exxp(-float64(loan.NPeriods) * lnf)
	if err != nil {
		return 0, err
	}
	numerator := 1 - expVal
	return numerator/(f-1)*d + padj, nil
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
// TODO: verify logic — DOS uses an "Iterate" helper that runs the full
// fancy schedule and takes derivatives against payment value. For
// fancy loans (with adjustments, balloons, or prepays), this simpler
// port may produce slightly different rates.
//
// Ported from legacy/src/dos_source/Amortize.pas: function
// EstimateAndRefineRate.
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
			return rate, true, nil
		}
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
