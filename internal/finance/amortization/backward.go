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
		return 0, fmt.Errorf("insufficient data: need amount, rate, term, peryr")
	}
	if loan.NPeriods <= 0 {
		return 0, fmt.Errorf("cannot determine payment - npayments not set")
	}
	if math.Abs(loan.Amount) < tiny {
		return 0, fmt.Errorf("payment cannot be computed for a loan of zero")
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
		return 0, fmt.Errorf("cannot determine payment - summation factor too small")
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
		return 0, fmt.Errorf("insufficient data: need rate, payment, term, peryr, first date")
	}

	// C-A-10: DOS Amortize.pas:Enter rejects "solve for loan amount"
	// when fancy mode is active AND a target principal-reduction is
	// in force. The two constraints over-determine the system because
	// target requires a known principal to enforce a per-period floor.
	if input.Fancy && input.Target.TargetStatus >= types.InOutDefault &&
		input.Target.TargetValue > 0 {
		return 0, fmt.Errorf("cannot solve for loan amount with target principal reduction")
	}

	f := GrowthPerPeriod(&loan, settings.YrInv)
	if math.Abs(f-1) < tiny {
		return 0, fmt.Errorf("cannot determine loan amount - interest rate too small")
	}
	if loan.NPeriods <= 0 {
		return 0, fmt.Errorf("cannot determine loan amount - npayments not set")
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
		return 0, false, fmt.Errorf("insufficient data: need amount, payment, term, peryr")
	}
	if math.Abs(loan.Amount) < tiny {
		return 0, false, fmt.Errorf("rate cannot be computed for a loan of zero")
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
