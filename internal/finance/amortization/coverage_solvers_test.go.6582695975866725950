package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_solvers_test.go — additional reachable branches in the prepayment /
// term solvers (error guards and alternate input forms).

// solveFancyTermFromPayment with an unbounded prepayment series (no stop/count):
// the solver must bound it internally and derive the term.
func TestFancyTermWithUnboundedPrepay(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 0, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
			PerYrStatus:   types.InOutInput, PerYr: 12,
			PaymentStatus: types.InOutInput, Payment: 200,
			StopDateStatus: types.StatusEmpty, NNStatus: types.StatusEmpty}}}
	in.Loan.NStatus = types.StatusEmpty
	in.Loan.LastStatus = types.StatusEmpty
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1100
	if r := Amortize(in); r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
}

// SolvePrepaymentDuration with an explicit NN on a second series (other-series
// stopDate-from-NN branch) plus the unknown series.
func TestPrepayDurationOtherSeriesNN(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(200000, 0.09, 240, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{
			{StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				NNStatus: types.InOutInput, NN: 36,
				PaymentStatus: types.InOutInput, Payment: 100},
			{StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
				PerYrStatus:   types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: 250,
				StopDateStatus: types.StatusEmpty, NNStatus: types.StatusEmpty}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1700
	if r := Amortize(in); r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
}

// Unbounded UNKNOWN prepayment amount (no stop, no count) → the additive solver
// must report the unbounded error.
func TestAdditiveUnknownUnbounded(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
			PerYrStatus:    types.InOutInput, PerYr: 12,
			StopDateStatus: types.StatusEmpty, NNStatus: types.StatusEmpty,
			PaymentStatus: types.StatusEmpty}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 950
	// Should error (unbounded unknown prepayment) rather than panic.
	if r := Amortize(in); r.Err == nil {
		t.Error("expected an unbounded-prepayment error")
	}
}

// SolvePrepaymentDuration direct: missing preconditions error branch.
func TestPrepayDurationPreconditionError(t *testing.T) {
	s := cc360()
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: s,
		Prepayments: []Prepayment{{
			StartDateStatus: types.StatusEmpty, // no start date → precondition fails
			PerYrStatus:     types.InOutInput, PerYr: 12,
			PaymentStatus: types.InOutInput, Payment: 200}}}
	if _, _, err := SolvePrepaymentDuration(in, 0); err == nil {
		t.Error("expected a precondition error when the prepayment start date is missing")
	}
}
