package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestLoanDateAfterFirstDateRejected verifies dispatch_gaps V6-9: a
// loan date after the first payment date is a dates-out-of-order
// error (DOS Amortize.pas Enter).
func TestLoanDateAfterFirstDateRejected(t *testing.T) {
	in := baseInput30y()
	// firstDate stays 2024-02-01; push the loan date past it.
	in.Loan.LoanDate = types.NewDateRec(2024, time.March, 1)
	res := Amortize(in)
	if res.Err == nil {
		t.Errorf("expected an error for loanDate after firstDate, got none")
	}
}

// TestInAdvanceWithAdjustmentRejected verifies dispatch_gaps V6-10:
// DOS rejects in-advance interest combined with rate adjustments.
func TestInAdvanceWithAdjustmentRejected(t *testing.T) {
	in := baseInput30y()
	in.Settings.InAdvance = true
	in.Adjustments = []RateAdjustment{{
		DateStatus:     types.InOutInput,
		Date:           types.NewDateRec(2029, time.January, 1),
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.07,
	}}
	res := Amortize(in)
	if res.Err == nil {
		t.Errorf("expected an error for in-advance + rate adjustment, got none")
	}
}

// TestPrepaymentBeforeLoanDateRejected verifies V6-9: a prepayment
// series cannot start before the loan exists.
func TestPrepaymentBeforeLoanDateRejected(t *testing.T) {
	in := baseInput30y() // loanDate 2024-01-01
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       types.NewDateRec(2023, time.June, 1), // before loan
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         200,
		NextDate:        types.NewDateRec(2023, time.June, 1),
	}}
	res := Amortize(in)
	if res.Err == nil {
		t.Errorf("expected an error for a prepayment starting before the loan date")
	}
}
