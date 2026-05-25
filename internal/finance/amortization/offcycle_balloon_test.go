package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestOffCycleBalloonApplied verifies dispatch_gaps V6-1: a balloon
// dated between two regular payment dates is no longer silently
// dropped — its principal reduction is folded into the schedule.
func TestOffCycleBalloonApplied(t *testing.T) {
	// baseInput30y pays on the 1st of each month; date the balloon on
	// the 15th so it falls strictly between two payment dates.
	in := baseInput30y()
	in.Balloons = []BalloonPayment{{
		DateStatus:   types.InOutInput,
		Date:         types.NewDateRec(2034, time.January, 15),
		AmountStatus: types.InOutInput,
		Amount:       60000,
	}}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}

	// A $60k balloon on a fully-amortizing 30-year loan accelerates
	// payoff, so the schedule must end before the 360th payment.
	if len(res.Schedule) >= 360 {
		t.Errorf("schedule ran %d rows — the off-cycle balloon was dropped "+
			"(expected early payoff under 360)", len(res.Schedule))
	}

	// The balloon's principal must actually be reflected: total
	// interest is materially lower than the no-balloon baseline.
	noBalloon := Amortize(baseInput30y())
	if res.TotalInt >= noBalloon.TotalInt {
		t.Errorf("total interest %.2f should be below the no-balloon baseline %.2f",
			res.TotalInt, noBalloon.TotalInt)
	}
}
