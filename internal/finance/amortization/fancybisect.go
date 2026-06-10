package amortization

import (
	"math"

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
// why SolvePayment ignored balloons entirely.
//
// This solves the same problem more robustly without touching the forward
// engine: the *sign* of "does the loan still owe at the scheduled end
// (under-amortized) vs. retire early / fall short (over-amortized)" is
// monotonic in each unknown and changes exactly at the solution. Bisecting
// that sign converges for any fancy schedule the forward engine can run,
// using Amortize itself as the oracle.

const fancyBisectTol = 5e-4 // absolute tolerance on the solved field

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
	last := res.Schedule[len(res.Schedule)-1]
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
// SolvePayment returned the no-balloon closed form for fancy loans.
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
