package amortization

import (
	"fmt"
	"math"

	"github.com/persense/persense-port/internal/types"
)

// appendScheduleWarnings emits the two post-schedule STRING warnings that the
// production Amortize appends inline (Amortize.pas:1040-1088 TackOnFinalBalloon,
// and the unusually-high-rate sanity check). It is factored out so the faithful
// port (AmortizeDOS) produces byte-for-byte identical advisory text — the two
// engines must agree on the advisory layer, with Amortize as the reference.
//
//   - regularPay is the regular (first-segment) payment the schedule was built
//     from; on an over-specified loan the final payment exceeds it by an implied
//     terminating balloon, which the schedule already folds into the last row.
//   - origRateStatus / origRate are the ORIGINAL (pre-ARM) Loan Rate fields; an
//     ARM mutates the running rate, but the high-rate check must see what the
//     user actually entered.
func appendScheduleWarnings(result *AmortResult, regularPay float64, origRateStatus int8, origRate float64) {
	if result.Err != nil || len(result.Schedule) == 0 {
		return
	}
	// A-W9 (inline): the regular payment does not amortize the loan over the
	// stated term, so the last payment absorbs the residual as an implied
	// terminating balloon. Mirrors engine.go's TackOnFinalBalloon block exactly.
	if regularPay > 0 {
		last := result.Schedule[len(result.Schedule)-1]
		if last.PayAmt > regularPay*1.5 && last.PayAmt-regularPay > minPmt {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"The regular payment does not amortize the loan over the stated "+
					"term — the final payment of %.2f includes an implied "+
					"terminating balloon of about %.2f.", last.PayAmt, last.PayAmt-regularPay))
		}
	}
	// Unusually-high-rate sanity check (fires only on a user-entered rate).
	if origRateStatus == types.InOutInput && origRate > unusuallyHighRate {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"Loan Rate of about %.2f%% is unusually high — double-check it was "+
				"entered in percent (for example 6 for 6%%, not 0.06 or 600).",
			origRate*100))
	}
}

// appendResultAdvisories runs the amortization result-sanity pass
// (docs/result_warning_layer_spec.md, A-W4..A-W7). It inspects the solved
// balloon / prepayment amounts and the generated schedule and appends
// non-blocking advisories. A-W9 (residual terminating balloon) is emitted
// inline by Amortize's TackOnFinalBalloon block; the rate/amount backward
// solves (A-W1..A-W3) are flagged by the API handler, where those values
// are solved. It never changes a computed number.
func appendResultAdvisories(result *AmortResult, input *LoanInput, loan *Loan, prepaySolvedAmt float64, prepaySolved, payWasInput bool) {
	if result.Err != nil || len(result.Schedule) == 0 {
		return
	}
	nz := math.Max(10, 0.0001*loan.Amount) // near-zero band, self-scaling

	// A-W11: a real (user-entered) balloon is set, but the Payment Amount is
	// being computed. When the payment is solved it amortizes the loan on its
	// own, so the balloon has nothing to settle and is silently dropped from
	// the schedule. Detect it: the balloon amount appears nowhere as a payment.
	if !payWasInput {
		maxPay := 0.0
		for _, row := range result.Schedule {
			if row.PayAmt > maxPay {
				maxPay = row.PayAmt
			}
		}
		for i := range input.Balloons {
			b := &input.Balloons[i]
			if b.AmountStatus == types.InOutInput && math.Abs(b.Amount) > 1 &&
				maxPay < math.Abs(b.Amount)*0.5 {
				result.add(types.AdvisoryTier, "A-W11", []string{"payment", "balloon"},
					"A balloon is set but the Payment Amount is being computed, so Per%Sense "+
						"solved the payment without the balloon and the balloon was ignored. "+
						"Enter a Payment Amount (for an interest-only loan, principal × rate ÷ "+
						"payments per year) so the balloon settles the remaining principal.")
				break // one is enough
			}
		}
	}

	// A-W4 / A-W5: a solved target balloon that is ~0 or negative.
	for i := range input.Balloons {
		b := &input.Balloons[i]
		if b.AmountStatus != types.InOutOutput {
			continue // user-supplied or absent, not a solved target
		}
		switch {
		case b.Amount < -nz:
			// (coverage: excluded — defensive/unreachable: SolveBalloonAmount
			// clamps every secant iterate to >= 0, so a solved target balloon
			// is never strictly negative; this arm guards a future solver that
			// could return one.)
			result.add(types.AdvisoryTier, "A-W5", []string{"balloon"},
				"The target balloon is negative — the regular payment over-pays before "+
					"this date. Lower the payment or move the balloon date later.")
		case b.Amount < nz:
			result.add(types.AdvisoryTier, "A-W4", []string{"balloon"},
				"The target balloon is essentially zero — the regular payment already "+
					"retires the loan by this date, so no balloon is needed.")
		}
	}

	// A-W7: a solved unknown prepayment amount that is ~0.
	if prepaySolved && math.Abs(prepaySolvedAmt) < nz {
		result.add(types.AdvisoryTier, "A-W7", []string{"prepayment"},
			"The extra payment needed is essentially zero — the loan already retires on "+
				"schedule without it.")
	}

	// A-W6: negative amortization — the balance grows from one regular period to
	// the next. Flag only SUSTAINED growth (a rise somewhere AFTER the first
	// regular period). A long odd first period — especially with prepaid OFF —
	// can make that single first period's interest exceed the constant payment,
	// bumping the balance up once before it amortizes normally; that one-period
	// bump is not negative amortization and shouldn't be flagged. Legitimate
	// sustained neg-am (help Examples 10-11) still surfaces, as a Note. The
	// earlier check fired whenever the balance ever exceeded the original
	// principal, which the odd-first-period bump tripped (client report).
	prevBal := loan.Amount
	firstRegularSeen := false
	sustainedNegAm := false
	for _, row := range result.Schedule {
		if row.PayNum < 1 {
			continue // skip the settlement-stub row
		}
		if firstRegularSeen && row.Principal > prevBal+1.0 {
			sustainedNegAm = true
			break
		}
		prevBal = row.Principal
		firstRegularSeen = true
	}
	if loan.Amount > 0 && sustainedNegAm {
		result.add(types.NoteTier, "A-W6", []string{"payment"},
			"The payment is below the interest due, so the balance grows over time "+
				"(negative amortization). Intended only if that's the structure you want.")
	}
}

// add appends a formatted advisory to the result's Warnings channel.
func (r *AmortResult) add(tier, code string, fields []string, msg string) {
	r.Warnings = append(r.Warnings, types.FormatAdvisory(tier, code, fields, msg))
}
