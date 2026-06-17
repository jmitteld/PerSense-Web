package amortization

import (
	"fmt"
	"math"
	"sort"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// GrowthPerPeriod computes the growth factor per payment period.
// This is (1 + rate/n) where n is the effective periods per year,
// with special handling for weekly (52) and biweekly (26) frequencies.
//
// Ported from legacy/source/AMORTOP.pas: function GrowthPerPeriod
func GrowthPerPeriod(loan *Loan, yrinv float64) float64 {
	switch loan.PerYr {
	case 52:
		return 1 + 7*yrinv*loan.LoanRate
	case 26:
		return 1 + 14*yrinv*loan.LoanRate
	default:
		return 1 + loan.LoanRate/interest.RealPerYr(byte(loan.PerYr), 1.0/yrinv)
	}
}

// ComputeTrueRate converts the loan rate to a continuously compounded rate
// for use in daily interest calculations.
//
// Ported from legacy/source/AMORTOP.pas: procedure ComputeTrueRate
func ComputeTrueRate(loan *Loan, settings *Settings) (float64, error) {
	rr, err := interest.ReportedRate(loan.LoanRate, byte(loan.PerYr), settings.PerYr, settings.YrDays)
	if err != nil {
		return 0, err
	}
	return interest.RateFromYield(rr, settings.PerYr, settings.YrDays)
}

// PrepaidInterest computes the prepaid interest amount from loan date
// to first payment date (or one period before first payment).
//
// Ported from legacy/source/AMORTOP.pas: function PrepaidInterest
func PrepaidInterest(loan *Loan, settings *Settings, truerate float64) (float64, error) {
	if !settings.Prepaid {
		return 0, nil
	}
	if settings.InAdvance {
		ydif := dateutil.YearsDif(loan.FirstDate, loan.LoanDate, settings.Basis, settings.YrInv, true)
		return loan.Amount * loan.LoanRate * ydif, nil
	}

	t := loan.FirstDate
	var err error
	t, err = dateutil.AddPeriod(t, loan.PerYr, loan.FirstDate.Time.Day(), true)
	if err != nil {
		return 0, err
	}
	ydif := dateutil.YearsDif(t, loan.LoanDate, settings.Basis, settings.YrInv, true)

	if settings.Daily {
		expVal, err := interest.Exxp(truerate * ydif)
		if err != nil {
			return 0, err
		}
		return loan.Amount * (expVal - 1), nil
	}
	return loan.Amount * loan.LoanRate * ydif, nil
}

// SortBalloons sorts balloon payments by date (ascending).
// Ported from legacy/source/AMORTOP.pas: procedure SortBalloons
func SortBalloons(balloons []BalloonPayment) {
	sort.Slice(balloons, func(i, j int) bool {
		return dateutil.DateComp(balloons[i].Date, balloons[j].Date) < 0
	})
}

// SortAdjustments sorts rate adjustments by date (ascending).
// Ported from legacy/source/AMORTOP.pas: procedure SortAdj
func SortAdjustments(adjustments []RateAdjustment) {
	sort.Slice(adjustments, func(i, j int) bool {
		return dateutil.DateComp(adjustments[i].Date, adjustments[j].Date) < 0
	})
}

// RepayLoan computes the remaining principal after all payments for a
// simple (non-fancy) loan using the closed-form growth formula.
//
// Ported from legacy/source/AMORTOP.pas: procedure RepayLoan
func RepayLoan(principal, payment float64, loan *Loan, settings *Settings, yrinv float64) float64 {
	f := GrowthPerPeriod(loan, yrinv)
	p := principal
	d := payment

	if settings.InAdvance {
		ff := (f - 1) / (2 - f)
		for i := 0; i < loan.NPeriods; i++ {
			p = p + ff*(p-d) - d
		}
	} else {
		// Compute prorate factor for first (possibly short) period — month-based
		// on clean boundaries (basis-independent), actual days only for odd-day
		// stubs, matching the schedule (see firstPeriodProrate).
		prorate := firstPeriodProrate(loan.LoanDate, loan.FirstDate, loan.PerYr, settings)
		ff := 1 + (f-1)*prorate
		p = p*ff - d // first payment
		for i := 1; i < loan.NPeriods; i++ {
			if p < 0 {
				p = p - d
			} else {
				p = p*f - d
			}
		}
	}
	return p
}

// Amortize computes the full amortization schedule for a loan.
// This is the main entry point for amortization calculations.
//
// For simple loans (non-fancy), it uses the closed-form RepayLoan.
// For fancy loans (with balloons, adjustments, prepayments, etc.),
// it uses the period-by-period RepayFancyLoan engine.
//
// Ported from legacy/source/Amortize.pas: procedure Enter + related
func Amortize(input LoanInput) AmortResult {
	var result AmortResult
	loan := input.Loan

	// Captured for the result-sanity advisory pass (A-W7): a solved
	// unknown-prepayment amount, recorded at its solve site below.
	prepaySolvedAmt, prepaySolved := 0.0, false
	// Captured for A-W11: whether the regular payment is a hard user input
	// (vs. computed). A balloon is dropped when the payment is computed.
	payWasInput := loan.PayAmtStatus == types.InOutInput

	// Validate minimum required data
	if loan.AmountStatus < types.InOutDefault {
		result.Err = fmt.Errorf("Amount Borrowed is blank. Enter the loan principal, " +
			"or leave it blank and supply Pmt Amount, Loan Rate and # Periods for " +
			"Per%%Sense to solve the loan amount.")
		return result
	}
	if loan.PerYrStatus < types.InOutDefault {
		result.Err = fmt.Errorf("Pmts/Yr is blank. Enter how many payments are made " +
			"per year (for example 12 for monthly) so a schedule can be built.")
		return result
	}

	if !dateutil.DateOK(loan.LoanDate) {
		result.Err = fmt.Errorf("Loan Date is blank. Enter the date the loan is made " +
			"so the schedule has a starting point.")
		return result
	}

	// FirstPass: derive any of {firstDate, lastDate, nPeriods} the
	// caller left blank but can be computed from the others. Mirrors
	// DOS Amortize.pas: procedure FirstPass.
	if err := FirstPass(&loan); err != nil {
		result.Err = err
		return result
	}
	// Capture the post-FirstPass term + dates so API callers can echo
	// derived values back to the UI (e.g. Help Example 1c — supply
	// first + last dates, get nPeriods back).
	result.NPeriods = loan.NPeriods
	result.FirstDate = loan.FirstDate
	result.LastDate = loan.LastDate
	if !dateutil.DateOK(loan.FirstDate) {
		result.Err = fmt.Errorf("The first payment date could not be determined. " +
			"Fill in 1st Pmt Date, or supply Loan Date and Pmts/Yr so Per%%Sense " +
			"can default it to one period after the loan date.")
		return result
	}
	// At least two regular payments. DOS rejects a single-payment loan
	// (firstDate >= lastDate, Amortize.pas:1221-1226). A loan with exactly one
	// installment — expressed either as NPeriods == 1 or as FirstDate ==
	// LastDate — is not a valid amortization. The reversed-dates case
	// (first > last) is left to the dedicated first-after-last validation.
	// See docs/n1_minimum_term_finding.md.
	if (loan.NStatus >= types.InOutDefault && loan.NPeriods == 1) ||
		(dateutil.DateOK(loan.LastDate) && dateutil.DateComp(loan.FirstDate, loan.LastDate) == 0) {
		result.Err = fmt.Errorf("There must be at least two regular payments. " +
			"Extend the term (# Periods or Last Pmt Date) so the loan has at least " +
			"two installments.")
		return result
	}
	input.Loan = loan

	// Cross-field validations (DOS Amortize.pas: procedure Enter
	// preflight + SortAdj/SortBalloons error arms).
	if err := ValidateInputs(&input); err != nil {
		result.Err = err
		return result
	}
	loan = input.Loan

	settings := input.Settings
	truerate, err := ComputeTrueRate(&loan, &settings)
	if err != nil {
		result.Err = fmt.Errorf("The Loan Rate could not be converted to an "+
			"internal rate (%w). Enter a Loan Rate in a normal range — for "+
			"example 6 for 6%% — and check that Pmts/Yr is set correctly.", err)
		return result
	}
	f := GrowthPerPeriod(&loan, settings.YrInv)

	// Whether the loan term was known on INPUT (before the A6 solve below).
	// DOS's MakeTable dispatch is an else-if chain in which
	// DetermineLastPaymentDate (solve the term) sits AHEAD of the unkpre
	// prepayment-duration branch (AMORTIZE.pas:1350-1367): when the term is
	// being derived, the prepayment duration is NOT separately solved — the
	// prepayment simply runs until the loan retires. AO10 below therefore fires
	// only when the term was already known here.
	termKnownOnInput := loan.NStatus >= types.InOutDefault || loan.LastStatus >= types.InOutDefault

	// A6 (DetermineLastPaymentDate, AMORTOP.pas:1323-1407): when the
	// caller supplied a payment but neither the term nor a last date,
	// derive the number of periods closed-form from the payment.
	if loan.NStatus < types.InOutDefault && loan.LastStatus < types.InOutDefault &&
		loan.PayAmtStatus >= types.InOutDefault && loan.PayAmt > 0 &&
		loan.LoanRateStatus >= types.InOutDefault && dateutil.DateOK(loan.FirstDate) {
		if input.Fancy {
			// Fancy mode: balloons/prepayments/adjustments make the
			// closed form inapplicable — run the schedule unbounded
			// and observe when the loan retires.
			n, last, err := solveFancyTermFromPayment(input)
			if err != nil {
				result.Err = err
				return result
			}
			loan.NPeriods = n
			loan.NStatus = types.InOutOutput
			loan.LastDate = last
			loan.LastStatus = types.InOutOutput
			loan.LastOK = true
			input.Loan = loan
		} else {
			n, err := solveNPeriodsFromPayment(&loan, &settings, f)
			if err != nil {
				result.Err = err
				return result
			}
			loan.NPeriods = n
			loan.NStatus = types.InOutOutput
			if last, err := dateutil.AddNPeriods(loan.FirstDate, loan.PerYr, n-1); err == nil {
				loan.LastDate = last
				loan.LastStatus = types.InOutOutput
				loan.LastOK = true
			}
			input.Loan = loan
		}
	}

	// Default payment amount if not specified
	d := loan.PayAmt
	if loan.PayAmtStatus < types.InOutDefault {
		// Estimate payment
		if loan.LoanRateStatus >= types.InOutDefault && loan.NPeriods > 0 {
			d = estimatePayment(&loan, f)
			// Odd first period, prepaid OFF: DOS augments the regular payment so
			// it absorbs the prorated first-period interest and stays constant
			// over the term (Amortize.pas:1513-1522; the ffFirst/f scaling in
			// EstimateAndRefinePayment). In prepaid mode the odd interest is taken
			// at settlement (the row-0 stub) and the first regular period is a
			// full period, so the payment is NOT augmented. Without this, a loan
			// with an odd first period and prepaid OFF under-amortized and left a
			// terminating balloon.
			if !settings.Prepaid && math.Abs(f-1) > teeny &&
				dateutil.DateOK(loan.LoanDate) && dateutil.DateOK(loan.FirstDate) {
				if prorate := firstPeriodProrate(loan.LoanDate, loan.FirstDate, loan.PerYr, &settings); prorate > 0 &&
					math.Abs(prorate-1) > teeny {
					ffFirst := 1 + (f-1)*prorate
					d *= ffFirst / f
				}
			}
		}
	}

	// Snapshot the post-FirstPass term + dates. The schedule
	// generators below replace `result` wholesale, and they take
	// `&loan` so could in principle mutate it — keep our own copy of
	// what the engine derived so we can echo it back regardless.
	derivedNPeriods := loan.NPeriods
	derivedFirstDate := loan.FirstDate
	derivedLastDate := loan.LastDate

	if !input.Fancy {
		// Blank payment: the closed-form estimate (the ordinary in-arrears
		// annuity, even with the prepaid-OFF odd-first augmentation above) does
		// not match DOS for an odd first period OR for in-advance (annuity-due)
		// loans. DOS's closed-form shortcut applies only to the plain
		// 360 / prepaid / in-arrears / natural-first case and ITERATES otherwise
		// (Amortize.pas:402-416 — the shortcut condition explicitly requires
		// `not in_advance`). Mirror that: refine the estimate with the
		// schedule-oracle bisection, which converges to the level payment that
		// drives the UNFORCED terminal balance of the real forward schedule
		// (which already models in-advance interest timing) to zero. The snap
		// guard keeps an already-exact estimate untouched (no sub-cent bisection
		// noise) and only adopts a materially different refined payment.
		if loan.PayAmtStatus < types.InOutDefault &&
			loan.LoanRateStatus >= types.InOutDefault && loan.NPeriods > 0 &&
			needPaymentRefine(&loan, &settings) {
			refIn := input
			refIn.Loan = loan
			if refined, ok := solveFancyPayment(refIn, d); ok && refined > 0 &&
				math.Abs(refined-d) > 1e-3 {
				d = refined
			}
		}
		// Simple amortization: generate schedule period by period
		result = generateSimpleSchedule(&loan, d, &settings, truerate, f)
	} else {
		// Fancy amortization with full feature set
		SortBalloons(input.Balloons)
		SortAdjustments(input.Adjustments)

		// AO2 (EstimateAndRefineBalloon, Amortize.pas:628-663): a
		// balloon with a date but no amount is a "target balloon" —
		// solve the amount that drives the schedule's final balance
		// to zero.
		unknownBalloon := -1
		for i := range input.Balloons {
			if input.Balloons[i].DateStatus >= types.InOutDefault &&
				input.Balloons[i].AmountStatus < types.InOutDefault {
				if unknownBalloon >= 0 {
					result.Err = fmt.Errorf(
						"More than one Balloon has a date but no amount. " +
							"Per%%Sense can solve only one unknown Balloon amount at a " +
							"time — fill in an amount for all but one of the Balloon rows.")
					return result
				}
				unknownBalloon = i
			}
		}
		if unknownBalloon >= 0 {
			amt, err := SolveBalloonAmount(input, unknownBalloon)
			if err != nil {
				result.Err = fmt.Errorf("The Balloon amount could not be solved: %w. "+
					"Check the Balloon Date and the loan terms, or enter the Balloon "+
					"amount directly.", err)
				return result
			}
			input.Balloons[unknownBalloon].Amount = amt
			input.Balloons[unknownBalloon].AmountStatus = types.InOutOutput
		}

		// AO9 (EstimateAndRefinePeriodicPrepayment, Amortize.pas:665):
		// a prepayment series with a start date but no amount is an
		// "unknown prepayment" — solve the per-payment amount that
		// drives the schedule's final balance to zero.
		unknownPrepay := -1
		for i := range input.Prepayments {
			if input.Prepayments[i].StartDateStatus >= types.InOutDefault &&
				input.Prepayments[i].PaymentStatus < types.InOutDefault {
				if unknownPrepay >= 0 {
					result.Err = fmt.Errorf(
						"More than one Prepayment has a start date but no amount. " +
							"Per%%Sense can solve only one unknown Prepayment amount at a " +
							"time — fill in an amount for all but one of the Prepayment rows.")
					return result
				}
				unknownPrepay = i
			}
		}
		if unknownPrepay >= 0 {
			amt, err := SolvePrepaymentAmount(input, unknownPrepay)
			if err != nil {
				result.Err = fmt.Errorf("The Prepayment amount could not be solved: %w. "+
					"Give the Prepayment a stop date or payment count so the solve is "+
					"bounded, or enter the Prepayment amount directly.", err)
				return result
			}
			input.Prepayments[unknownPrepay].Payment = amt
			prepaySolvedAmt, prepaySolved = amt, true
			// Mark the solved amount as a known input so the schedule
			// engine applies it (the prepayment loop skips a series
			// whose PaymentStatus is below InOutDefault).
			input.Prepayments[unknownPrepay].PaymentStatus = types.InOutInput
		}

		// AO10 (DeterminePrepaymentDuration, Amortize.pas:709-774): a
		// prepayment series with a known amount but no stop date and
		// no payment count — solve how long it must run to retire the
		// loan, then pin NN and the stop date.
		for i := range input.Prepayments {
			pp := &input.Prepayments[i]
			if termKnownOnInput &&
				pp.StartDateStatus >= types.InOutDefault &&
				pp.PaymentStatus >= types.InOutDefault &&
				pp.StopDateStatus < types.InOutDefault &&
				pp.NNStatus < types.InOutDefault {
				nn, stop, err := SolvePrepaymentDuration(input, i)
				if err != nil {
					result.Err = fmt.Errorf(
						"The Prepayment duration could not be solved: %w. Check the "+
							"Prepayment amount and start date, or supply a stop date or "+
							"payment count directly.", err)
					return result
				}
				input.Prepayments[i].NN = nn
				input.Prepayments[i].NNStatus = types.InOutInput
				input.Prepayments[i].StopDate = stop
				input.Prepayments[i].StopDateStatus = types.InOutInput
			}
		}

		// Dav Holle provision (Amortize.pas:1430-1434): when the
		// regular payment is a user-input "hard" number, balloon
		// amounts and adjusted payment amounts are hardened to whole
		// cents so the schedule uses the standard penny treatment.
		if loan.PayAmtStatus == types.InOutInput {
			for i := range input.Balloons {
				input.Balloons[i].Amount = interest.Round2(input.Balloons[i].Amount)
			}
			for i := range input.Adjustments {
				input.Adjustments[i].Amount = interest.Round2(input.Adjustments[i].Amount)
			}
		}

		// When the regular payment was left blank, solve it so the schedule
		// amortizes over the stated term WITH the fancy features active. The
		// closed-form estimatePayment ignores balloons, targets, etc., which
		// left the loan under/over-amortized: a known balloon didn't reduce the
		// payment (it retired early), and a principal-minimum target paid the
		// loan off before the term. Mirrors DOS Amortize.pas' EstimateAndRefine
		// payment-iteration when fancy options are active.
		//
		// Skip this when an unknown balloon/prepayment was just solved — in that
		// field-presence dispatch the balloon/prepayment is the unknown and the
		// (estimated) payment is the known. Skip-months keep their existing,
		// well-tested refinement; everything else (known balloon, target,
		// rate/payment adjustments, known prepayment) uses the general
		// schedule-oracle bisection in solveFancyPayment.
		solvedUnknown := unknownBalloon >= 0 || unknownPrepay >= 0
		skipActive := input.SkipMonths.SkipStatus >= types.InOutDefault &&
			anySkip(input.SkipMonths.MonthSet)
		// A known balloon should REDUCE the regular payment so principal + balloon
		// amortize over the term; a principal-minimum (target) should be solved so
		// the schedule still retires exactly at the term. Rate/payment adjustments
		// re-amortize their own payment, and prepayments are extra payments meant
		// to shorten the term — neither should have the regular payment globally
		// re-solved, so they're excluded here.
		hasKnownBalloon := false
		for i := range input.Balloons {
			if input.Balloons[i].AmountStatus >= types.InOutDefault &&
				math.Abs(input.Balloons[i].Amount) > 0 {
				hasKnownBalloon = true
				break
			}
		}
		targetActive := input.Target.TargetStatus >= types.InOutDefault
		hasPrepay := false
		for i := range input.Prepayments {
			if input.Prepayments[i].PaymentStatus >= types.InOutDefault {
				hasPrepay = true
				break
			}
		}
		if loan.PayAmtStatus < types.InOutDefault && !solvedUnknown &&
			loan.LoanRateStatus >= types.InOutDefault && loan.NPeriods > 0 {
			if skipActive {
				// Prefer the schedule-oracle bisection (solveFancyPayment), which
				// reconstructs the UNFORCED terminal residual via fancyOverUnder.
				// refineFancyPayment alone bisects on FinalPrinc, which
				// generateFancySchedule forces to ~0, so for recurring skip-month
				// loans it solved a payment that over-amortized (~18% high vs DOS).
				// Fall back to the legacy refinement only if the bisection cannot
				// bracket a solution.
				if refined, ok := solveFancyPayment(input, d); ok && refined > 0 {
					d = refined
				} else {
					d = refineFancyPayment(input, d, &settings, truerate, f)
				}
			} else if (hasKnownBalloon || targetActive) &&
				len(input.Adjustments) == 0 && !hasPrepay {
				if refined, ok := solveFancyPayment(input, d); ok {
					d = refined
				}
			} else if len(input.Adjustments) == 0 && !hasPrepay &&
				(settings.InAdvance ||
					oddFirstPeriod(loan.LoanDate, loan.FirstDate, loan.PerYr, &settings)) {
				// Universal non-shortcut refinement: any remaining fancy loan that
				// DOS would iterate rather than close-form — an odd first period OR
				// in-advance (annuity-due) — but with no balloon/target/adjustment/
				// prepayment of its own (e.g. a moratorium, or a plain odd-first
				// fancy loan). Snap-guarded so an already-exact estimate is kept.
				// (In-advance precision here is bounded by the fancyOverUnder
				// in-advance reconstruction — docs/dos_known_frontier.md #38.)
				if refined, ok := solveFancyPayment(input, d); ok && refined > 0 &&
					math.Abs(refined-d) > 1e-3 {
					d = refined
				}
			}
		}

		result = generateFancySchedule(input, d, &settings, truerate, f)
	}

	// A9: when the caller supplied discount points, compute the APR —
	// the rate that equates the present value of the scheduled
	// payments to the borrower's net proceeds (Amortize.pas: function
	// EstimateAndRefineAPRwithPoints).
	if result.Err == nil && loan.PointsStatus >= types.InOutDefault &&
		len(result.Schedule) > 0 {
		prepaid, _ := PrepaidInterest(&loan, &settings, truerate)
		netProceeds := loan.Amount*(1-loan.Points) - prepaid
		apr, conv := ComputeAPRWithPoints(result.Schedule, loan.LoanDate,
			netProceeds, loan.LoanRate, byte(loan.PerYr), &settings)
		result.APR = apr
		result.APRConverged = conv
	}

	// TackOnFinalBalloon (Amortize.pas:1040-1088): when the loan is
	// over-specified — the regular payment does not amortize it over
	// the stated term — the final payment absorbs the residual as an
	// implied terminating balloon. DOS appends it as a balloon row
	// and advises the user; here the residual is already folded into
	// the last scheduled payment, so flag it with an advisory.
	if result.Err == nil && len(result.Schedule) > 0 && d > 0 {
		last := result.Schedule[len(result.Schedule)-1]
		if last.PayAmt > d*1.5 && last.PayAmt-d > minPmt {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"The regular payment does not amortize the loan over the stated "+
					"term — the final payment of %.2f includes an implied "+
					"terminating balloon of about %.2f.", last.PayAmt, last.PayAmt-d))
		}
	}

	// Re-apply the snapshotted post-FirstPass term + dates. The
	// schedule generators return a fresh AmortResult that overwrites
	// the assignments we made earlier, so without this step a
	// successful run would echo NPeriods=0 / zero dates back to the API.
	result.NPeriods = derivedNPeriods
	result.FirstDate = derivedFirstDate
	result.LastDate = derivedLastDate

	// Unusually-high-rate sanity check. DOS shows this warning only on
	// the mortgage screen (MortgageScreenUnit.pas:222); the amortization
	// screen has no equivalent. A typo'd rate is just as damaging here,
	// so we extend the same soft warning to amortization. LoanRate is a
	// nominal fraction (0.06 = 6%), so the threshold is 20% nominal
	// directly. Fire only on a user-entered rate, never a solved one.
	// Appended here (after the schedule generators return a fresh
	// result) so it survives the result reassignment above.
	if loan.LoanRateStatus == types.InOutInput && loan.LoanRate > unusuallyHighRate {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"Loan Rate of about %.2f%% is unusually high — double-check it was "+
				"entered in percent (for example 6 for 6%%, not 0.06 or 600).",
			loan.LoanRate*100))
	}

	// Echo the balloons the engine used — including any "target" balloon whose
	// amount it solved (AmountStatus becomes Output) — so the UI can fill the
	// blank Amount cell with the computed value.
	for i := range input.Balloons {
		b := input.Balloons[i]
		if b.DateStatus < types.InOutDefault || !dateutil.DateOK(b.Date) {
			continue
		}
		result.Balloons = append(result.Balloons, ResolvedBalloon{
			Date:   b.Date,
			Amount: b.Amount,
			Solved: b.AmountStatus == types.InOutOutput,
		})
	}

	appendResultAdvisories(&result, &input, &loan, prepaySolvedAmt, prepaySolved, payWasInput)
	return result
}

// anySkip reports whether any month in the set is flagged for skip.
func anySkip(set [13]bool) bool {
	for m := 1; m <= 12; m++ {
		if set[m] {
			return true
		}
	}
	return false
}

// refineFancyPayment bisects on the periodic payment until the
// fancy schedule's final balance lands near zero. Used when fancy
// features (skip-months, etc.) prevent a closed-form solution.
//
// Reasoning for bisection rather than Newton: the schedule walk has
// discontinuities at balloons, adjustments, and skip-month
// boundaries, so the derivative-based methods can be unstable. The
// final balance is monotone in d (higher payment → lower balance),
// so bisection always converges.
func refineFancyPayment(input LoanInput, dInit float64,
	settings *Settings, truerate, f float64) float64 {
	// Bracket: the final balance is monotone decreasing in d. Use
	// the closed-form estimate as a starting point and expand the
	// bracket until balances at lo and hi straddle zero.
	simulate := func(d float64) float64 {
		// Deep-copy slices that the schedule walk mutates so each
		// bisection iteration starts from a clean state.
		in := input
		if len(input.Prepayments) > 0 {
			in.Prepayments = append([]Prepayment(nil), input.Prepayments...)
		}
		r := generateFancySchedule(in, d, settings, truerate, f)
		return r.FinalPrinc
	}

	lo := dInit * 0.5
	hi := dInit * 2.0
	balLo := simulate(lo)
	balHi := simulate(hi)
	for expand := 0; expand < 10 && balLo*balHi > 0; expand++ {
		if balLo > 0 {
			lo *= 0.5
			balLo = simulate(lo)
		}
		if balHi > 0 {
			hi *= 2.0
			balHi = simulate(hi)
		} else if balHi < 0 {
			// Both negative; both d too high. Lower the lo guess.
			hi = lo
			balHi = balLo
			lo *= 0.5
			balLo = simulate(lo)
		}
	}
	if balLo*balHi > 0 {
		// Couldn't bracket; fall back to initial estimate.
		return dInit
	}

	for i := 0; i < 60; i++ {
		mid := 0.5 * (lo + hi)
		balMid := simulate(mid)
		if math.Abs(balMid) < 0.005 || hi-lo < 1e-6 {
			return mid
		}
		if balMid*balLo > 0 {
			lo = mid
			balLo = balMid
		} else {
			hi = mid
			balHi = balMid
		}
	}
	return 0.5 * (lo + hi)
}

// estimatePayment computes an initial payment estimate using the annuity formula.
// oddFirstPeriod reports whether the first payment is not exactly one
// compounding period after the loan date — a short or long "odd" first
// period. The closed-form payment estimate assumes a full first period, so
// when this is true the blank-payment solve must refine the estimate against
// the actual (prorated) schedule to match DOS. Ported behavior: DOS's
// EstimateAndRefinePayment iterates (Amortize.pas:416) for every non-trivial
// case; we reproduce that with the schedule-oracle bisection only where the
// closed form is actually inexact, which is the odd-first period.
// needPaymentRefine reports whether the blank-payment closed-form estimate for a
// PLAIN (non-fancy) loan must be refined against the real schedule to match DOS.
// This encodes DOS's payment-solve shortcut (Amortize.pas:402): DOS uses the
// closed-form estimate directly only for the plain case and ITERATES otherwise.
// For a plain loan the closed form is exact iff the first period is calendar-
// natural and the payment is in-arrears — so refine when the first period is odd
// OR the loan is in-advance (annuity-due). R78 has its own precomputed split and
// is never refined here. Centralizing this keeps the "every non-shortcut solve is
// refined" guarantee in one place rather than rediscovering each case as a bug.
func needPaymentRefine(loan *Loan, s *Settings) bool {
	if s.R78 {
		return false
	}
	return s.InAdvance || oddFirstPeriod(loan.LoanDate, loan.FirstDate, loan.PerYr, s)
}

func oddFirstPeriod(loanDate, firstDate types.DateRec, perYr int, s *Settings) bool {
	if !dateutil.DateOK(loanDate) || !dateutil.DateOK(firstDate) {
		return false
	}
	return math.Abs(firstPeriodProrate(loanDate, firstDate, perYr, s)-1) > 1e-6
}

// firstPeriodProrate returns the first period's length as a fraction of one
// payment period. DOS uses an exact MONTH-based fraction on clean period
// boundaries — matching day-of-month and a month-dividing frequency — regardless
// of the basis: months / (12/perYr). Only a genuine odd-DAY stub (loan
// day-of-month ≠ first-payment day) uses the basis-specific actual-day count.
//
// This matters on the 365 basis: a calendar-natural first period is not exactly
// 1/perYr of the actual (366-day leap) year — YearsDif*perYr = 182/366*2 = 0.9945
// instead of 1.0 — which skewed the first schedule row. DOS treats it as a whole
// period (prorate = 1). On the 360 basis the two already agree (30/360 makes a
// whole month exactly 1/12 of a year), so this changes only 365-basis behavior,
// and only on clean boundaries — odd-day stubs (already DOS-faithful) are
// untouched. Weekly/biweekly (perYr not dividing 12) keep the day-based count.
func firstPeriodProrate(loanDate, firstDate types.DateRec, perYr int, s *Settings) float64 {
	if perYr > 0 && 12%perYr == 0 && loanDate.Time.Day() == firstDate.Time.Day() {
		months := (firstDate.Time.Year()-loanDate.Time.Year())*12 +
			(int(firstDate.Time.Month()) - int(loanDate.Time.Month()))
		if months >= 0 {
			return float64(months) / float64(12/perYr)
		}
	}
	return dateutil.YearsDif(firstDate, loanDate, s.Basis, s.YrInv, true) * float64(perYr)
}

// periodYearFraction returns the length of the [prev, cur] interval in YEARS for
// per-period interest accrual. On a clean month boundary (matching day-of-month,
// month-dividing frequency) it returns the exact month-based fraction
// `months / 12` — basis-independent, matching DOS's per-period accrual
// (`p*(f-1)`); only a genuine partial/odd-day span (an off-cycle balloon or
// prepayment remainder, or a day stub) uses the basis-specific actual-day
// `YearsDif`. This is the fancy-schedule analog of firstPeriodProrate: it keeps
// the 365 basis from skewing each row's interest split (31- vs 28-day months) on
// balloon/prepayment/moratorium/skip loans, and makes the in-advance accrual
// around skipped months match DOS. (Daily compounding still needs the true day
// count and is handled separately by the caller.)
func periodYearFraction(prev, cur types.DateRec, perYr int, s *Settings) float64 {
	if perYr > 0 && 12%perYr == 0 && prev.Time.Day() == cur.Time.Day() {
		months := (cur.Time.Year()-prev.Time.Year())*12 +
			(int(cur.Time.Month()) - int(prev.Time.Month()))
		if months > 0 {
			return float64(months) / 12.0
		}
	}
	return dateutil.YearsDif(cur, prev, s.Basis, s.YrInv, true)
}

func estimatePayment(loan *Loan, f float64) float64 {
	if math.Abs(f-1) < teeny {
		return loan.Amount / float64(loan.NPeriods)
	}
	numer := loan.Amount * (f - 1)
	lnf, _ := interest.Lnn(f)
	expVal, _ := interest.Exxp(-float64(loan.NPeriods) * lnf)
	denom := 1 - expVal
	if math.Abs(denom) < teeny {
		return loan.Amount / float64(loan.NPeriods)
	}
	return numer / denom
}

// generateSimpleSchedule builds the schedule for a non-fancy loan.
func generateSimpleSchedule(loan *Loan, payment float64, settings *Settings, truerate, f float64) AmortResult {
	var result AmortResult
	p := loan.Amount
	var cumInt float64

	// hardPayment: the regular payment is a user-supplied "hard"
	// number (not solved by the engine). DOS the "Dav Holle
	// provision" — rounds per-period interest to whole cents in this
	// case so the schedule uses the standard penny treatment
	// (Amortize.pas:1483 `if hard_payment then Round2(interest)`).
	hardPayment := loan.PayAmtStatus == types.InOutInput

	currentDate := loan.FirstDate
	origDay := loan.FirstDate.Time.Day()

	// Compute the natural start of the first regular period (one
	// period before FirstDate). When in prepaid mode and the loan
	// date precedes that natural start, emit a separate "row 0" for
	// the settlement-period interest. This mirrors DOS AMORTOP.pas:
	// PrepaidInterest is collected at closing and the schedule's
	// first regular row spans exactly one full period.
	//
	// Without this split, the first regular row's interest column
	// bundles the settlement-day interest into pmt #1, which
	// distorts the per-row breakdown even though totals match.
	prorate := 1.0
	if settings.Prepaid && !settings.InAdvance {
		naturalStart, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, true)
		if err == nil {
			if dateutil.DateComp(loan.LoanDate, naturalStart) < 0 {
				// Settlement stub: emit row 0.
				stubYd := dateutil.YearsDif(naturalStart, loan.LoanDate,
					settings.Basis, settings.YrInv, true)
				var stubInt float64
				if settings.Daily {
					expVal, _ := interest.Exxp(truerate * stubYd)
					stubInt = p * (expVal - 1)
				} else {
					stubInt = p * loan.LoanRate * stubYd
				}
				cumInt += stubInt
				result.Schedule = append(result.Schedule, PaymentRecord{
					PayNum:    0,
					Date:      loan.LoanDate,
					PayAmt:    stubInt,
					Interest:  stubInt,
					Principal: p,
					IntToDate: cumInt,
				})
				result.TotalPaid += stubInt
				result.TotalInt += stubInt
				// First regular period is now exactly one period long.
				prorate = 1.0
			} else {
				// Loan closes within the first regular period; first
				// period is short. Compute its prorate as the actual
				// fraction of a period.
				ydif := dateutil.YearsDif(loan.FirstDate, loan.LoanDate,
					settings.Basis, settings.YrInv, true)
				prorate = ydif * float64(loan.PerYr)
			}
		}
	} else {
		// Non-prepaid mode: first period accrues interest for the
		// entire LoanDate → FirstDate span, possibly more than one
		// period. Month-based on clean boundaries (basis-independent),
		// actual days only for odd-day stubs — see firstPeriodProrate.
		prorate = firstPeriodProrate(loan.LoanDate, loan.FirstDate, loan.PerYr, settings)
	}

	// In-advance (annuity-due) prorate factor. When set, the payment
	// is made at the START of each period and interest accrues on the
	// post-payment balance: p = p + ff*(p-d) - d. This mirrors the
	// in-advance branch of RepayLoan (AMORTOP.pas: in_advance), so the
	// schedule and the closed-form solvers agree.
	inAdvanceFF := 0.0
	if settings.InAdvance {
		inAdvanceFF = (f - 1) / (2 - f)
	}

	// Rule-of-78 ("sum of the digits") interest allocation. The total
	// interest (n*payment - amount) is front-loaded: period k gets
	// interest proportional to (n+1-k). r78step is decremented from a
	// seed of r78step*(n+1) so the first period's interest is
	// n*r78step. Ported from Amortize.pas:1506-1530.
	//
	// R78 applies ONLY on the 360-day basis. DOS routes any non-360 basis
	// through RepayFancyLoan (Amortize.pas:1493: `… or (not (df.c.basis=x360))
	// then RepayFancyLoan`), the standard per-period walk that does NOT apply
	// the sum-of-digits split — so on the 365 basis R78 is silently a no-op and
	// the borrower gets ordinary amortization interest. Match that.
	r78 := settings.R78 && !settings.InAdvance && loan.NPeriods > 0 &&
		settings.Basis == types.Basis360
	var r78step, r78int float64
	if r78 {
		n := float64(loan.NPeriods)
		r78step = (n*payment - loan.Amount) / (0.5 * n * (n + 1))
		r78int = r78step * (n + 1)
	}

	for i := 0; i < loan.NPeriods; i++ {
		var intThisPd float64
		pmt := payment

		if r78 {
			// Sum-of-digits interest: declines by r78step each period.
			r78int -= r78step
			intThisPd = r78int
			if hardPayment {
				intThisPd = interest.Round2(intThisPd)
			}
			if i == loan.NPeriods-1 {
				pmt = p + intThisPd
			}
			p = p + intThisPd - pmt
		} else if settings.InAdvance {
			// Payment made at the START of the period; interest accrues on
			// the post-payment balance (p - pmt). DOS charges this even on the
			// final period: the regular payment is used for the interest
			// calculation, then the actual final payment clears the remaining
			// balance plus that interest (AMORTOP.pas in_advance branch — the
			// final row carries (p-d)*f_1/(2-f), not zero). The p < pmt guard
			// clamps the near-payoff case where the balance is below the
			// regular payment.
			if p < pmt {
				intThisPd = 0
			} else {
				intThisPd = inAdvanceFF * (p - pmt)
			}
			if hardPayment {
				intThisPd = interest.Round2(intThisPd)
			}
			if i == loan.NPeriods-1 {
				pmt = p + intThisPd
			}
			p = p + intThisPd - pmt
		} else {
			if i == 0 {
				// First period may be short
				ff := 1 + (f-1)*prorate
				intThisPd = p * (ff - 1)
			} else {
				intThisPd = p * (f - 1)
			}

			if settings.Daily {
				// Daily compounding uses truerate and actual day count
				var prevDate types.DateRec
				if i == 0 {
					prevDate = loan.LoanDate
				} else {
					prevDate, _ = dateutil.AddPeriod(currentDate, loan.PerYr, origDay, true)
				}
				yd := dateutil.YearsDif(currentDate, prevDate, settings.Basis, settings.YrInv, true)
				expVal, _ := interest.Exxp(truerate * yd)
				intThisPd = p * (expVal - 1)
			} else if loan.PerYr == 26 || loan.PerYr == 52 {
				// Weekly/biweekly: the displayed DOS schedule accrues SIMPLE
				// interest on the ACTUAL day count between payment dates on the
				// 365-day basis (e.g. 14/366 in a leap year), not the constant
				// per-period factor p*(f-1) above — that factor is the solver's
				// convention (GrowthPerPeriod uses yrdays = 365.25) and differs
				// from the table's actual-day/leap-year accrual. Recompute here
				// so the per-row schedule matches DOS. Monthly/quarterly (360)
				// keep p*(f-1) unchanged.
				var prevDate types.DateRec
				if i == 0 {
					prevDate = loan.LoanDate
				} else {
					prevDate, _ = dateutil.AddPeriod(currentDate, loan.PerYr, origDay, true)
				}
				yd := dateutil.YearsDif(currentDate, prevDate, settings.Basis, settings.YrInv, true)
				intThisPd = p * loan.LoanRate * yd
			}

			if hardPayment {
				intThisPd = interest.Round2(intThisPd)
			}

			// Last payment: adjust to pay off remaining balance
			if i == loan.NPeriods-1 {
				pmt = p + intThisPd
			}

			p = p + intThisPd - pmt
		}
		cumInt += intThisPd

		result.Schedule = append(result.Schedule, PaymentRecord{
			PayNum:    i + 1,
			Date:      currentDate,
			PayAmt:    pmt,
			Interest:  intThisPd,
			Principal: p,
			IntToDate: cumInt,
		})

		result.TotalPaid += pmt
		result.TotalInt += intThisPd

		// Advance date
		if i < loan.NPeriods-1 {
			nextDate, err := dateutil.AddPeriod(currentDate, loan.PerYr, origDay, false)
			if err != nil {
				result.Err = err
				return result
			}
			currentDate = nextDate
		}
	}

	result.FinalPrinc = p
	return result
}

// generateFancySchedule handles the full-featured amortization engine with
// balloons, adjustments, prepayments, moratoria, targets, and skip months.
//
// This is a simplified port of RepayFancyLoan that generates the schedule
// directly rather than printing to screen. The core payment-by-payment
// logic is preserved.
func generateFancySchedule(input LoanInput, payment float64, settings *Settings, truerate, f float64) AmortResult {
	var result AmortResult
	loan := input.Loan
	p := loan.Amount
	d := payment
	var cumInt float64
	var usap float64      // USA Rule exempt principal
	var negRateNoted bool // A-W12 emitted once if an AO6 adjustment implies a negative rate

	// hardPayment: a user-supplied regular payment triggers the DOS
	// "Dav Holle provision" — per-period interest is rounded to whole
	// cents (AMORTOP.pas:637 `if hard_payment then Round2(interest)`).
	hardPayment := loan.PayAmtStatus == types.InOutInput

	// DetermineVeryLast (AMORTOP.pas:1293-1304): the schedule must run
	// to the LATEST of {lastDate, last balloon date, every prepayment
	// stop date} — not just lastDate. Otherwise a balloon dated after
	// the last regular payment, or a prepayment series whose stop date
	// extends past lastDate, would be silently cut off.
	veryLast := loan.LastDate
	for _, b := range input.Balloons {
		if b.DateStatus >= types.InOutDefault &&
			dateutil.DateComp(b.Date, veryLast) > 0 {
			veryLast = b.Date
		}
	}
	for _, pp := range input.Prepayments {
		if pp.StopDateStatus >= types.InOutDefault &&
			dateutil.DateComp(pp.StopDate, veryLast) > 0 {
			veryLast = pp.StopDate
		}
	}

	origDay := loan.FirstDate.Time.Day()
	currentDate := loan.FirstDate
	prevDate := loan.LoanDate

	// Handle the first-period interest accrual window.
	//
	// In prepaid mode the borrower pays settlement-day interest at
	// closing and the first regular payment then covers exactly one
	// full period. Compute the "natural" start of that first full
	// period — one period before FirstDate.
	//
	// Two situations to distinguish (DOS AMORTOP.pas handles both
	// via PrepaidInterest + a normalized first period):
	//   (A) LoanDate < naturalStart: there is a settlement stub from
	//       LoanDate to naturalStart. Emit a row 0 for that stub
	//       (interest only, balance unchanged) and run the first
	//       regular period from naturalStart to FirstDate.
	//   (B) LoanDate >= naturalStart: the loan starts within the
	//       first regular period (e.g. quarterly loan that closes
	//       only one month before the first quarterly payment). No
	//       stub; the first regular period runs from LoanDate to
	//       FirstDate and will accrue less than a full period of
	//       interest — the day-count-based formula handles this
	//       naturally as long as prevDate stays at LoanDate.
	if settings.Prepaid && !settings.InAdvance {
		naturalStart, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, true)
		if err == nil {
			if dateutil.DateComp(loan.LoanDate, naturalStart) < 0 {
				// Case A: emit settlement-period row 0.
				stubYd := dateutil.YearsDif(naturalStart, loan.LoanDate,
					settings.Basis, settings.YrInv, true)
				var stubInt float64
				if settings.Daily {
					expVal, _ := interest.Exxp(truerate * stubYd)
					stubInt = p * (expVal - 1)
				} else {
					stubInt = p * loan.LoanRate * stubYd
				}
				cumInt += stubInt
				result.Schedule = append(result.Schedule, PaymentRecord{
					PayNum:    0,
					Date:      loan.LoanDate,
					PayAmt:    stubInt,
					Interest:  stubInt,
					Principal: p,
					IntToDate: cumInt,
				})
				result.TotalPaid += stubInt
				result.TotalInt += stubInt
				prevDate = naturalStart
			} else {
				// Case B: short first period; prevDate stays as
				// LoanDate so yd accurately captures the partial
				// period.
				prevDate = loan.LoanDate
			}
		}
	}

	nextBalloon := 0 // index into sorted balloons

	// prepayApplied[i] counts how many extra payments prepayment
	// series i has applied so far. Used to honor Prepayment.NN — a
	// series specified as "NN extra payments" must stop after NN
	// extras even when no StopDate was given. See dispatch_gaps AO8 /
	// CLAUDE.md outstanding item #4.
	prepayApplied := make([]int, len(input.Prepayments))

	// nextDates[i] is the running "next extra due" cursor for prepayment
	// series i. It is kept LOCAL rather than written back into
	// input.Prepayments[i].NextDate: the cursor is transient generation
	// state, and Go shares the Prepayments backing array with the
	// caller, so persisting it would make Amortize non-idempotent. A
	// second run on the same input would see a half-advanced cursor and
	// build a different schedule — which previously defeated the
	// iterateNewton backward solver (it evaluates many trials against
	// one shared input, and the poisoned cursor flattened the residual
	// so Newton's step ran away). Each entry starts Unknown and is
	// seeded from StartDate on first use, matching the prior behavior.
	nextDates := make([]types.DateRec, len(input.Prepayments))
	for i := range nextDates {
		nextDates[i] = types.UnknownDate()
	}

	// Moratorium tracking: moratoriumActive once we observe any
	// interest-only periods; moratoriumRecomputed once we've
	// re-solved d at the FirstRepay boundary so we only do it once.
	moratoriumActive := input.Moratorium.FirstRepayStatus >= types.InOutDefault &&
		dateutil.DateComp(loan.FirstDate, input.Moratorium.FirstRepay) < 0
	moratoriumRecomputed := false

	for payNum := 1; payNum <= loan.NPeriods+len(input.Balloons)+100; payNum++ {
		// Safety limit to prevent infinite loops
		if payNum > 10000 {
			result.Err = fmt.Errorf("The schedule grew past 10000 payments without " +
				"the loan paying off. The Pmt Amount may be too small to cover the " +
				"interest — raise the Pmt Amount, or leave it blank for Per%%Sense to " +
				"compute a payment that retires the loan.")
			break
		}

		// Off-cycle prepayment draining. Any prepayment series whose next due
		// date falls STRICTLY BEFORE this regular payment date is emitted as
		// its own dated row, with its own partial-period interest accrued from
		// the previous row's date — exactly as DOS emits an extra that lands
		// between two regular dates (balloonpos < 0, AMORTOP.pas:608-613).
		// Drain all such rows (possibly several, from one or more series)
		// before the regular period is computed.
		for {
			drainIdx := -1
			var drainDate types.DateRec
			for i := range input.Prepayments {
				pp := &input.Prepayments[i]
				if pp.PaymentStatus < types.InOutDefault || pp.PerYrStatus < types.InOutDefault ||
					pp.StartDateStatus < types.InOutDefault {
					continue
				}
				if nextDates[i].IsUnknown() {
					nextDates[i] = pp.StartDate
				}
				if pp.StopDateStatus >= types.InOutDefault &&
					dateutil.DateComp(nextDates[i], pp.StopDate) > 0 {
					continue
				}
				if pp.NNStatus >= types.InOutDefault && pp.NN > 0 && prepayApplied[i] >= pp.NN {
					continue
				}
				if dateutil.DateComp(nextDates[i], currentDate) >= 0 {
					continue // on or after the regular date — handled below
				}
				if drainIdx < 0 || dateutil.DateComp(nextDates[i], drainDate) < 0 {
					drainIdx = i
					drainDate = nextDates[i]
				}
			}
			// Off-cycle balloon: a balloon dated STRICTLY BEFORE this regular
			// payment is emitted at its own date too. DOS RepayFancyLoan applies a
			// balloon on its exact date — accruing partial interest up to it, then
			// the next regular period accrues only from the balloon date forward
			// (AMORTOP.pas:608-613, the balloonpos<0 branch). An odd first period
			// shifts the regular payment dates off the balloon's monthly grid,
			// which is exactly when a balloon lands between two regular dates; the
			// previous code folded it into the next payment (a few weeks late),
			// diverging from DOS. Pick the balloon date if it precedes the earliest
			// pending prepayment.
			drainBalloon := false
			if nextBalloon < len(input.Balloons) {
				bd := input.Balloons[nextBalloon].Date
				if dateutil.DateComp(bd, currentDate) < 0 &&
					(drainIdx < 0 || dateutil.DateComp(bd, drainDate) < 0) {
					drainBalloon = true
					drainDate = bd
				}
			}
			if drainIdx < 0 && !drainBalloon {
				break
			}
			// Sum every event due exactly at drainDate, advancing each.
			var offPay float64
			if drainBalloon {
				// All balloons sharing this exact off-cycle date combine into one
				// dated row (their amount is the payment; principal reduction is
				// amount − accrued interest, computed below).
				for nextBalloon < len(input.Balloons) &&
					dateutil.DateComp(input.Balloons[nextBalloon].Date, drainDate) == 0 {
					offPay += input.Balloons[nextBalloon].Amount
					nextBalloon++
				}
			} else {
				for i := range input.Prepayments {
					pp := &input.Prepayments[i]
					if pp.PaymentStatus < types.InOutDefault || pp.PerYrStatus < types.InOutDefault ||
						pp.StartDateStatus < types.InOutDefault {
						continue
					}
					if nextDates[i].IsUnknown() {
						nextDates[i] = pp.StartDate
					}
					if pp.StopDateStatus >= types.InOutDefault &&
						dateutil.DateComp(nextDates[i], pp.StopDate) > 0 {
						continue
					}
					if pp.NNStatus >= types.InOutDefault && pp.NN > 0 && prepayApplied[i] >= pp.NN {
						continue
					}
					if dateutil.DateComp(nextDates[i], drainDate) == 0 {
						offPay += pp.Payment
						prepayApplied[i]++
						if next, err := dateutil.AddPeriod(nextDates[i], pp.PerYr,
							pp.StartDate.Time.Day(), false); err == nil {
							nextDates[i] = next
						}
					}
				}
			}
			// Partial-period interest from the previous row's date to drainDate.
			ydOff := dateutil.YearsDif(drainDate, prevDate, settings.Basis, settings.YrInv, true)
			var intOff float64
			if settings.Daily {
				expVal, _ := interest.Exxp(truerate * ydOff)
				intOff = (p - usap) * (expVal - 1)
			} else {
				intOff = loan.LoanRate * ydOff * (p - usap)
			}
			if hardPayment {
				intOff = interest.Round2(intOff)
			}
			offCyclePaidOff := false
			if p+intOff-offPay <= 0 {
				offPay = p + intOff
				offCyclePaidOff = true
			}
			p = p + intOff - offPay
			if settings.USARule {
				usap = usap + intOff - offPay
				if usap < 0 {
					usap = 0
				}
			}
			cumInt += intOff
			result.Schedule = append(result.Schedule, PaymentRecord{
				PayNum:    payNum,
				Date:      drainDate,
				PayAmt:    offPay,
				Interest:  intOff,
				Principal: p,
				IntToDate: cumInt,
			})
			result.TotalPaid += offPay
			result.TotalInt += intOff
			prevDate = drainDate
			if offCyclePaidOff {
				result.FinalPrinc = p
				return result
			}
		}

		// Compute interest for this period
		var intThisPd float64
		yd := dateutil.YearsDif(currentDate, prevDate, settings.Basis, settings.YrInv, true)
		// Per-period (month-based) year fraction for whole/clean periods — used by
		// the non-Daily accrual so the 365 basis doesn't skew the per-row split;
		// actual days only for partial/odd-day spans. Daily needs the true day
		// count (yd) for continuous compounding.
		ydReg := periodYearFraction(prevDate, currentDate, loan.PerYr, settings)

		if settings.Daily {
			expVal, _ := interest.Exxp(truerate * yd)
			intThisPd = (p - usap) * (expVal - 1)
		} else {
			intThisPd = loan.LoanRate * ydReg * (p - usap)
		}
		if hardPayment {
			intThisPd = interest.Round2(intThisPd)
		}

		// Check for balloon at this date
		pmt := d
		if currentDate.Time.Month() > 0 && int(currentDate.Time.Month()) <= 12 {
			if input.SkipMonths.MonthSet[currentDate.Time.Month()] {
				pmt = 0
			}
		}

		// Check moratorium.
		//
		// Before FirstRepay: pay interest only (no principal reduction). At
		// FirstRepay, if the regular payment was being SOLVED (left blank), we
		// re-solve it over the remaining periods on the unchanged principal so
		// the loan still amortizes — without this the post-moratorium payment
		// is too low (AM_EX13's help: the no-moratorium baseline of $2,024.02
		// is wrong; the right answer is $2,152.63). When the user GAVE the
		// payment, DOS uses it AS-IS through the moratorium (the interest-only
		// periods simply defer principal and any residual rolls to the end) —
		// it does not re-amortize. So the recompute is gated on a blank payment.
		if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
			if dateutil.DateComp(currentDate, input.Moratorium.FirstRepay) < 0 {
				pmt = intThisPd // interest-only during moratorium
			} else if !moratoriumRecomputed && moratoriumActive &&
				loan.PayAmtStatus < types.InOutDefault {
				// First period at or after FirstRepay, payment solved — recompute d.
				remaining := loan.NPeriods - payNum + 1
				if remaining > 0 {
					tempLoan := loan
					tempLoan.Amount = p
					tempLoan.NPeriods = remaining
					d = estimatePayment(&tempLoan, f)
					pmt = d
				}
				moratoriumRecomputed = true
			}
		}

		// Accumulate the extra payments (balloons + prepayment series) that
		// fall on THIS regular payment date, plus any off-cycle balloons whose
		// date falls strictly before it. Coincident extras combine with the
		// regular payment per PlusRegular (DOS Paymenttype.ComputeNext,
		// AMORTOP.pas:614-621): ON adds on top of the regular payment, OFF
		// (the default) REPLACES it — an additional-payment schedule. Off-cycle
		// PREPAYMENTS are emitted as their own dated rows by the draining block
		// at the top of this loop, not here.
		var coincidentExtra, offCycleExtra float64
		anyCoincident := false

		for nextBalloon < len(input.Balloons) {
			cmp := dateutil.DateComp(input.Balloons[nextBalloon].Date, currentDate)
			if cmp < 0 {
				// Off-cycle balloon: folded into this period's payment so the
				// principal reduction is not lost (lands a few days later than
				// the DOS row).
				offCycleExtra += input.Balloons[nextBalloon].Amount
				nextBalloon++
			} else if cmp == 0 {
				coincidentExtra += input.Balloons[nextBalloon].Amount
				anyCoincident = true
				nextBalloon++
			} else {
				break
			}
		}

		// Prepayment series coincident with this regular date.
		//
		// Mirrors DOS FindNextExtra at AMORTOP.pas:490-572: each active series
		// has a NextDate starting at StartDate; when NextDate matches the
		// current period it is an extra on (or replacing) the regular payment,
		// then NextDate advances by 12/PerYr months. When NextDate passes
		// StopDate (or NN extras have been applied) the series is exhausted.
		for i := range input.Prepayments {
			pp := &input.Prepayments[i]
			if pp.PaymentStatus < types.InOutDefault || pp.PerYrStatus < types.InOutDefault {
				continue
			}
			if pp.StartDateStatus < types.InOutDefault {
				continue
			}
			if nextDates[i].IsUnknown() {
				nextDates[i] = pp.StartDate
			}
			if pp.StopDateStatus >= types.InOutDefault &&
				dateutil.DateComp(nextDates[i], pp.StopDate) > 0 {
				continue
			}
			// NN-bounded series: once NN extra payments have been applied, the
			// series is exhausted even if no StopDate was supplied. Mirrors DOS
			// Paymenttype.ComputeNext. See dispatch_gaps AO8.
			if pp.NNStatus >= types.InOutDefault && pp.NN > 0 &&
				prepayApplied[i] >= pp.NN {
				continue
			}
			if dateutil.DateComp(nextDates[i], currentDate) == 0 {
				coincidentExtra += pp.Payment
				anyCoincident = true
				prepayApplied[i]++
				next, err := dateutil.AddPeriod(nextDates[i], pp.PerYr,
					pp.StartDate.Time.Day(), false)
				if err == nil {
					nextDates[i] = next
				}
			}
		}

		if anyCoincident {
			if settings.PlusRegular {
				pmt += coincidentExtra
			} else {
				pmt = coincidentExtra
			}
		}
		pmt += offCycleExtra

		// Target principal reduction
		if input.Target.TargetStatus >= types.InOutDefault {
			if pmt-intThisPd < input.Target.TargetValue {
				pmt = input.Target.TargetValue + intThisPd
			}
		}

		// In-advance (annuity-due) accrual: the payment is made at the
		// START of the period, so interest accrues on the balance
		// AFTER the payment, not before. Recompute intThisPd on the
		// post-payment balance now that pmt is final. Mirrors the DOS
		// in_advance schedule, where the payment date leads the
		// interest period (AMORTOP.pas:1159-1191).
		if settings.InAdvance && !settings.Daily {
			postPay := p - usap - pmt
			if postPay < 0 {
				postPay = 0
			}
			intThisPd = loan.LoanRate * ydReg * postPay
			if hardPayment {
				intThisPd = interest.Round2(intThisPd)
			}
		}

		// Early payoff: if this period's payment would clear the
		// balance (or overshoot it negative — which happens once
		// prepayments or a balloon accelerate the loan), trim the
		// payment so the balance lands exactly on zero and stop.
		// Mirrors DOS WhenToStop, which folds the residual principal
		// into the final payment.
		payoffNow := false
		if p+intThisPd-pmt <= 0 {
			pmt = p + intThisPd
			payoffNow = true
		}

		// Apply payment
		p = p + intThisPd - pmt
		if settings.USARule {
			usap = usap + intThisPd - pmt
			if usap < 0 {
				usap = 0
			}
		}
		cumInt += intThisPd

		result.Schedule = append(result.Schedule, PaymentRecord{
			PayNum:    payNum,
			Date:      currentDate,
			PayAmt:    pmt,
			Interest:  intThisPd,
			Principal: p,
			IntToDate: cumInt,
		})

		result.TotalPaid += pmt
		result.TotalInt += intThisPd

		// Check termination conditions
		if payoffNow {
			if payNum < loan.NPeriods {
				result.Warnings = append(result.Warnings, fmt.Sprintf(
					"Loan retired early — paid off at payment %d of a scheduled %d.",
					payNum, loan.NPeriods))
			}
			break
		}
		if p < minPmt && p > -minPmt {
			break
		}
		if loan.LastOK && dateutil.DateComp(currentDate, veryLast) >= 0 {
			break
		}

		// Advance to next date
		prevDate = currentDate
		nextDate, err := dateutil.AddPeriod(currentDate, loan.PerYr, origDay, false)
		if err != nil {
			result.Err = err
			return result
		}
		currentDate = nextDate

		// Check for rate adjustments
		for i := range input.Adjustments {
			adj := &input.Adjustments[i]
			if adj.DateStatus >= types.InOutDefault &&
				dateutil.DateComp(currentDate, adj.Date) > 0 &&
				dateutil.DateComp(prevDate, adj.Date) <= 0 {
				hasRate := adj.LoanRateStatus >= types.InOutDefault
				hasAmt := adj.AmtOK
				remaining := loan.NPeriods - payNum
				if hasRate {
					loan.LoanRate = adj.LoanRate
					truerate, _ = ComputeTrueRate(&loan, settings)
					f = GrowthPerPeriod(&loan, settings.YrInv)
				}
				if hasAmt {
					d = adj.Amount
				}
				// AO5 (EstimateAndRefineAdjPayment): a rate change
				// with no new payment re-amortizes the current
				// balance over the remaining term at the new rate —
				// otherwise the old payment no longer amortizes the
				// loan cleanly after the rate moves. Balloons dated
				// after the adjustment reduce the principal the
				// regular payment must retire, so their value is
				// discounted back to the adjustment date and netted
				// off (DOS Re_Amortize balloon term, AMORTOP.pas:1561).
				//
				// AO7 (re-amortize at current rate): a date-only
				// adjustment row supplies neither a new rate nor a
				// new payment, and asks DOS to re-solve the regular
				// payment over the remaining term at the *unchanged*
				// rate. This is useful when an upcoming balloon (or
				// drift left over from a prior adjustment) means the
				// running payment no longer amortizes the loan
				// cleanly — AO7 resets the payment without changing
				// the rate. The same re-amortize formula handles
				// both AO5 and AO7: when no new rate was supplied,
				// `f` and `truerate` keep their pre-adjustment values
				// (set further up only when `hasRate`), so the solve
				// uses the current rate.
				if !hasAmt && remaining > 0 {
					netBal := p
					for bi := range input.Balloons {
						b := &input.Balloons[bi]
						if b.AmountStatus >= types.InOutDefault &&
							dateutil.DateComp(b.Date, currentDate) > 0 {
							yd := dateutil.YearsDif(b.Date, currentDate,
								settings.Basis, settings.YrInv, false)
							if disc, e := interest.Exxp(-loan.LoanRate * yd); e == nil {
								netBal -= b.Amount * disc
							}
						}
					}
					if netBal < 0 {
						netBal = 0
					}
					// USA-rule carry (V6-2 / R9 fix): the running usap
					// is part of engine state and survives the
					// adjustment naturally — the per-period loop at
					// line 965-969 continues to update it. The new
					// payment matches DOS Re_Amortize at
					// AMORTOP.pas:1545-1569: amortize the full
					// outstanding `netBal` over the remaining term at
					// the new rate, with no special usap split. (An
					// earlier port subtracted usap from netBal and
					// added a linear paydown term, on the theory that
					// usap should retire linearly. The standard
					// per-period rule retires usap much faster than
					// linear, so that adjustment left a large residual
					// on negative-amort + ARM loans; reverting to the
					// DOS formula closes that gap. Exact retirement on
					// the most pathological cases still requires the
					// full DOS Iterate routine — task #103.)
					d = annuityPayment(netBal, f, remaining)
				}
				// AO6 (EstimateAndRefineAdjRate, Amortize.pas:1415):
				// a new payment with no new rate — solve the rate at
				// which that payment amortizes the balance over the
				// remaining term, and continue the schedule at it.
				// This is the mirror image of AO5 (rate given,
				// payment solved), so an adjustment always keeps the
				// loan on its original term.
				if hasAmt && !hasRate && remaining > 0 {
					if r, ok := solveAdjRate(p, d, remaining, loan,
						settings.YrInv); ok {
						loan.LoanRate = r
						truerate, _ = ComputeTrueRate(&loan, settings)
						f = GrowthPerPeriod(&loan, settings.YrInv)
						// AO6 with a new payment too LOW to amortize the
						// balance over the remaining term implies a
						// NEGATIVE rate (the balance can only reach zero on
						// schedule if interest is credited, not charged).
						// DOS computes and runs the negative rate, producing
						// negative interest rows after the adjustment. That
						// is correct but surprising, so surface it as a Note
						// (does not change any number).
						if r < 0 && !negRateNoted {
							result.add(types.NoteTier, "A-W12", []string{"adjustment"},
								"A payment-only adjustment set a new payment too low to amortize the "+
									"loan at a positive rate, so Per%Sense fit a negative interest rate "+
									"for the periods after it — you'll see negative interest from that "+
									"date and the balance barely moving. Raise the new payment if you "+
									"intended a positive rate.")
							negRateNoted = true
						}
					}
				}
			}
		}
	}

	result.FinalPrinc = p
	return result
}

// MonthSetFromString parses a skip-months string like "6-8" or "1,6,12"
// into a boolean array indexed by month (1-12).
//
// Ported from legacy/source/Amortize.pas: function MonthSetFromString
func MonthSetFromString(s string) ([13]bool, error) {
	var monthSet [13]bool
	if s == "" {
		return monthSet, nil
	}

	i := 0
	var lastN int
	thruflag := false

	for i < len(s) {
		// Skip non-digit, non-dash chars
		for i < len(s) && !isDigit(s[i]) && s[i] != '-' {
			i++
		}
		if i >= len(s) {
			break
		}

		if s[i] == '-' {
			thruflag = true
			i++
			continue
		}

		// Parse 1-2 digit number
		n := int(s[i] - '0')
		i++
		if i < len(s) && isDigit(s[i]) {
			n = n*10 + int(s[i]-'0')
			i++
		}

		if n < 1 || n > 12 {
			return monthSet, fmt.Errorf("Skip Months contains the month number %d, "+
				"which is out of range. Use month numbers 1 through 12, for example "+
				"\"6-8,12\".", n)
		}

		if thruflag {
			if lastN == 0 {
				return monthSet, fmt.Errorf("Skip Months has a range dash with no " +
					"starting month. Write a range as start-end, for example \"6-8\".")
			}
			if lastN <= n {
				for m := lastN; m <= n; m++ {
					monthSet[m] = true
				}
			} else {
				// Wrap around: e.g. 10-2 means Oct,Nov,Dec,Jan,Feb
				for m := lastN; m <= 12; m++ {
					monthSet[m] = true
				}
				for m := 1; m <= n; m++ {
					monthSet[m] = true
				}
			}
			thruflag = false
		} else {
			monthSet[n] = true
		}
		lastN = n
	}

	return monthSet, nil
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// BalanceAtDate returns the outstanding loan balance as of `date`,
// given a generated schedule. It reads the balance the engine
// already recorded after each payment (PaymentRecord.Principal), so
// the result is correct even when the schedule contains balloons,
// rate adjustments, prepayments, or a moratorium — unlike a
// payment-minus-interest walk, which drifts on those.
//
// Ported from legacy/src/dos_source/Amortize.pas: procedure
// ComputeBalanceFromDate.
func BalanceAtDate(schedule []PaymentRecord, loanAmount float64, date types.DateRec) float64 {
	bal := loanAmount
	for i := range schedule {
		if dateutil.DateComp(schedule[i].Date, date) > 0 {
			break
		}
		bal = schedule[i].Principal
	}
	if bal < 0 {
		bal = 0
	}
	return bal
}

// DateForBalance is the inverse of BalanceAtDate: it returns the
// first payment date on which the outstanding balance has fallen to
// or below `target`. The bool is false when the balance never
// reaches the target within the schedule.
//
// Ported from legacy/src/dos_source/Amortize.pas: procedure
// ComputeDateFromBalance.
func DateForBalance(schedule []PaymentRecord, target float64) (types.DateRec, bool) {
	for i := range schedule {
		if schedule[i].PayNum < 1 {
			continue // skip the settlement-stub row
		}
		if schedule[i].Principal <= target {
			return schedule[i].Date, true
		}
	}
	return types.DateRec{}, false
}
