package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestTargetBalloonSolved verifies dispatch_gaps FP3 / AO2: a balloon
// with a date but no amount is a "target balloon" — the engine solves
// the amount that drives the schedule's final balance to zero (DOS
// EstimateAndRefineBalloon).
func TestTargetBalloonSolved(t *testing.T) {
	in := baseInput30y()
	// A 5-year term but paid at the 30-year payment, so a large
	// balance remains at year 5 — the classic balloon-loan setup.
	in.Loan.NPeriods = 60
	in.Loan.LastDate = types.NewDateRec(2029, time.January, 1)
	in.Loan.LastOK = true
	// Balloon at the last payment date, amount left blank.
	in.Balloons = []BalloonPayment{{
		DateStatus: types.InOutInput,
		Date:       types.NewDateRec(2029, time.January, 1),
		// AmountStatus deliberately zero -> unknown.
	}}

	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// The solved balloon should be roughly the outstanding balance of
	// a 30-yr 6% loan after 5 years (~$186k).
	solved := in.Balloons[0].Amount
	if solved < 150000 || solved > 200000 {
		t.Errorf("solved balloon = %.2f, expected the ~$186k 5-year balance", solved)
	}
	// With the target balloon applied, the loan must pay off: the
	// final balance lands at zero.
	if math.Abs(res.FinalPrinc) > 1.0 {
		t.Errorf("final balance = %.2f, expected ~0 after the target balloon",
			res.FinalPrinc)
	}
}
