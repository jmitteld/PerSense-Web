package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_backward3_test.go covers solveNPeriodsFromPayment edge arms,
// SolvePrepaymentDuration guards, the exact-daily SolvePaymentClosedForm branch, and
// the fancyBisect sign==0 short-circuits.

// TestSolveNPeriodsFirstPaymentClears covers the p1<=0 "first payment alone
// clears the loan" arm (backward.go:395) — a payment larger than the
// principal plus its first-period interest retires the loan in one period.
func TestSolveNPeriodsFirstPaymentClears(t *testing.T) {
	loan := simpleLoan(1000, 0.06, 0, 5000) // payment >> principal
	s := simpleSettings()
	f := GrowthPerPeriod(&loan, s.YrInv)
	n, err := solveNPeriodsFromPayment(&loan, &s, f)
	if err != nil {
		t.Fatalf("solveNPeriodsFromPayment: %v", err)
	}
	if n != 1 {
		t.Errorf("first-payment-clears: n = %d, want 1", n)
	}
}

// TestSolveNPeriodsZeroPayment covers the d<=0 guard (backward.go:368).
func TestSolveNPeriodsZeroPayment(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 0, 0)
	s := simpleSettings()
	f := GrowthPerPeriod(&loan, s.YrInv)
	if _, err := solveNPeriodsFromPayment(&loan, &s, f); err == nil {
		t.Errorf("expected error for zero payment")
	}
}

// TestSolveNPeriodsPaymentBelowInterest covers the "payment doesn't cover
// interest" guard (backward.go:374).
func TestSolveNPeriodsPaymentBelowInterest(t *testing.T) {
	loan := simpleLoan(100000, 0.12, 0, 10) // 10/mo cannot cover 1000/mo interest
	s := simpleSettings()
	f := GrowthPerPeriod(&loan, s.YrInv)
	if _, err := solveNPeriodsFromPayment(&loan, &s, f); err == nil {
		t.Errorf("expected payment-too-small error")
	}
}

// TestSolveNPeriodsZeroRate covers the ff~1 (zero-rate) straight-line arm
// (backward.go:401).
func TestSolveNPeriodsZeroRate(t *testing.T) {
	loan := simpleLoan(12000, 0, 0, 1000)
	s := simpleSettings()
	f := GrowthPerPeriod(&loan, s.YrInv)
	n, err := solveNPeriodsFromPayment(&loan, &s, f)
	if err != nil {
		t.Fatalf("solveNPeriodsFromPayment zero-rate: %v", err)
	}
	if n < 11 || n > 13 {
		t.Errorf("zero-rate 12000/1000 = %d periods, want ~12", n)
	}
}

// TestSolvePrepaymentDurationNegative covers the negative-duration guard
// (backward.go:870): the fixed payments already over-cover the principal, so
// no extra payment duration is needed.
func TestSolvePrepaymentDurationNegative(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 360, 5000) // big payment retires fast
	loan.LastOK = true
	loan.LastDate = types.NewDateRec(2054, time.January, 1)
	in := LoanInput{
		Loan:     loan,
		Settings: simpleSettings(),
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024, time.March, 1),
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         500,
			// no NN / StopDate -> duration unknown
		}},
	}
	if _, _, err := SolvePrepaymentDuration(in, 0); err == nil {
		t.Errorf("expected negative-duration error")
	}
}

// TestSolvePrepaymentDurationPrecondition covers the missing-precondition
// guard (backward.go:813).
func TestSolvePrepaymentDurationPrecondition(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 360, 600)
	loan.AmountStatus = types.StatusEmpty // missing amount -> precondition fails
	in := LoanInput{
		Loan:     loan,
		Settings: simpleSettings(),
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024, time.March, 1),
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         500,
		}},
	}
	if _, _, err := SolvePrepaymentDuration(in, 0); err == nil {
		t.Errorf("expected precondition error")
	}
}

// TestSolvePaymentExactDaily covers the exact-daily SolvePaymentClosedForm branch
// (backward.go:144-151): a non-360-basis exact loan iterates the actual-day
// schedule via dosIteratePayment.
func TestSolvePaymentExactDaily(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 60, 0)
	loan.PayAmtStatus = types.StatusEmpty
	s := Settings{Basis: types.Basis365, PerYr: 12, YrDays: 365, YrInv: 1.0 / 365, Exact: true}
	got, err := SolvePaymentClosedForm(LoanInput{Loan: loan, Settings: s, Fancy: true})
	if err != nil {
		t.Fatalf("SolvePaymentClosedForm exact-daily: %v", err)
	}
	if got <= 0 {
		t.Errorf("exact-daily payment = %.4f, want positive", got)
	}
}

// TestFancyBisectSignZeroAtBracket covers the sLo==0 / sHi==0 immediate
// returns (fancybisect.go:214-220) by handing fancyBisect a sign function
// that is already zero at the low bound.
func TestFancyBisectSignZeroAtBracket(t *testing.T) {
	// zero exactly at lo.
	if x, ok := fancyBisect(func(v float64) int {
		if v <= 10 {
			return 0
		}
		return 1
	}, 10, 20, 0, 100, 1e-6); !ok || x != 10 {
		t.Errorf("sLo==0: got (%v,%v), want (10,true)", x, ok)
	}
	// zero exactly at hi (lo positive, hi zero).
	if x, ok := fancyBisect(func(v float64) int {
		if v >= 20 {
			return 0
		}
		return 1
	}, 10, 20, 0, 100, 1e-6); !ok || x != 20 {
		t.Errorf("sHi==0: got (%v,%v), want (20,true)", x, ok)
	}
	// no sign change anywhere -> (0,false).
	if x, ok := fancyBisect(func(float64) int { return 1 }, 10, 20, 5, 25, 1e-6); ok || x != 0 {
		t.Errorf("no-sign-change: got (%v,%v), want (0,false)", x, ok)
	}
}

// TestFancyBisectExpandHitsZero covers the sign==0 hits found DURING bracket
// expansion (fancybisect.go:236-242): the initial [lo,hi] is all positive,
// and a zero only appears once the bracket expands outward.
func TestFancyBisectExpandHitsZero(t *testing.T) {
	// Positive on [40,60]; zero appears at <=20 after one expansion (span=20).
	x, ok := fancyBisect(func(v float64) int {
		if v <= 20 {
			return 0
		}
		return 1
	}, 40, 60, 0, 200, 1e-6)
	if !ok {
		t.Errorf("expand-to-zero: expected convergence")
	}
	_ = x
}
