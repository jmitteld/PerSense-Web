package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_guards_test.go — triggers the user-reachable guard branches (the ones
// a person can actually hit with bad-but-well-formed input), as opposed to the
// deep defensive returns that only fire on float overflow / date-arithmetic
// failure. See the coverage note at the end of docs/exact_groundzero_findings.md.

// oddFirstPeriod returns false when a date is blank (the DateOK guard).
func TestOddFirstPeriodBlankDate(t *testing.T) {
	s := cc360()
	if oddFirstPeriod(types.UnknownDate(), types.NewDateRec(2024, time.February, 1), 12, &s) {
		t.Error("blank loan date should not be an odd first period")
	}
	if oddFirstPeriod(types.NewDateRec(2024, time.January, 1), types.UnknownDate(), 12, &s) {
		t.Error("blank first date should not be an odd first period")
	}
}

// SolvePrepaymentDuration: the negative-duration guard (the fixed payments
// already over-cover the principal).
func TestPrepaymentDurationNegativeGuard(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.February, 1),
			PerYrStatus:   types.InOutInput, PerYr: 12,
			PaymentStatus: types.InOutInput, Payment: 500,
			StopDateStatus: types.StatusEmpty, NNStatus: types.StatusEmpty}}}
	// A regular payment well ABOVE the level annuity already retires the loan, so
	// the prepayment duration would be negative — the guard must fire.
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1200
	if r := Amortize(in); r.Err == nil {
		t.Error("expected a negative-duration error when the regular payment over-covers principal")
	}
}

// solveFancyTermFromPayment: a payment too small to ever retire (≤ interest)
// must surface the 80-year-cap error.
func TestFancyTermPaymentTooSmall(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 0, 12), Settings: cc360(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2030, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 1000}}}
	in.Loan.NStatus = types.StatusEmpty
	in.Loan.LastStatus = types.StatusEmpty
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 500 // far below the $1,000/mo interest — never retires
	if r := Amortize(in); r.Err == nil {
		t.Error("expected a 'payment too small' error for a non-amortizing fancy loan")
	}
}

// solveNPeriodsFromPayment: a non-fancy payment too small to retire surfaces an
// error rather than looping forever.
func TestNPeriodsPaymentTooSmall(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 0, 12), Settings: cc360()}
	in.Loan.NStatus = types.StatusEmpty
	in.Loan.LastStatus = types.StatusEmpty
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 500 // ≤ interest-only
	if r := Amortize(in); r.Err == nil {
		t.Error("expected an error when the payment cannot amortize the loan")
	}
}

// Over-determined loan: a fixed payment that does not amortize over the stated
// term leaves a terminating balloon → advisory warning (not an error).
func TestTerminatingBalloonAdvisory(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: cc360()}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1005 // below the ~1028.61 level payment → under-amortizes
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.Warnings) == 0 {
		t.Error("expected a terminating-balloon advisory for an under-amortizing payment")
	}
}
