package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestUnknownPrepaymentSolved verifies dispatch_gaps FP5 / AO9: a
// prepayment series with a start date but no amount is solved so the
// schedule's final balance lands at zero (DOS
// EstimateAndRefinePeriodicPrepayment).
func TestUnknownPrepaymentSolved(t *testing.T) {
	in := baseInput30y()
	// 5-year term paid at the 30-year payment — a large balance
	// remains, which the prepayment series must retire.
	in.Loan.NPeriods = 60
	in.Loan.LastDate = types.NewDateRec(2029, time.January, 1)
	in.Loan.LastOK = true
	// Monthly prepayment series over the whole term, amount unknown.
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       types.NewDateRec(2024, time.February, 1),
		StopDateStatus:  types.InOutInput,
		StopDate:        types.NewDateRec(2029, time.January, 1),
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		NextDate:        types.NewDateRec(2024, time.February, 1),
		// PaymentStatus deliberately zero -> unknown.
	}}

	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	solved := in.Prepayments[0].Payment
	if solved <= 0 {
		t.Errorf("solved prepayment = %.2f, expected a positive amount", solved)
	}
	if math.Abs(res.FinalPrinc) > 5.0 {
		t.Errorf("final balance = %.2f, expected ~0 once the prepayment retires the loan",
			res.FinalPrinc)
	}
}
