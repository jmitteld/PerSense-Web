package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// TestVeryLastExtendsForLateBalloon verifies dispatch_gaps G3: the
// fancy schedule runs to the LATEST of {lastDate, last balloon date,
// prepayment stop dates} — DOS DetermineVeryLast (AMORTOP.pas:1293).
// A balloon dated after the last regular payment must still appear.
func TestVeryLastExtendsForLateBalloon(t *testing.T) {
	in := baseInput30y()
	// Shorten the regular term to 5 years (payment stays the 30-yr
	// annuity, so a large balance remains at the last regular pmt).
	in.Loan.NPeriods = 60
	in.Loan.LastDate = types.NewDateRec(2029, time.January, 1)
	in.Loan.LastOK = true
	// Balloon two years AFTER the last regular payment.
	balloonDate := types.NewDateRec(2031, time.January, 1)
	in.Balloons = []BalloonPayment{{
		DateStatus:   types.InOutInput,
		Date:         balloonDate,
		AmountStatus: types.InOutInput,
		Amount:       150000,
	}}

	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Fatal("empty schedule")
	}
	last := res.Schedule[len(res.Schedule)-1]
	if dateutil.DateComp(last.Date, balloonDate) < 0 {
		t.Errorf("schedule ends %v, before the balloon date %v — "+
			"VeryLast extension did not apply", last.Date.Time, balloonDate.Time)
	}
	// The balloon must be reflected: some row pays well above the
	// regular payment around the balloon date.
	sawBalloon := false
	for _, row := range res.Schedule {
		if dateutil.DateComp(row.Date, balloonDate) == 0 && row.PayAmt > 100000 {
			sawBalloon = true
		}
	}
	if !sawBalloon {
		t.Errorf("no balloon-sized payment found on %v", balloonDate.Time)
	}
}
