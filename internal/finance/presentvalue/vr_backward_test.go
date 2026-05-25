package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestVRBackwardSolveAmount verifies dispatch_gaps V6-12: in
// variable-rate mode, a payment amount left blank with the screen
// Sum Value supplied is back-solved (DOS PVLX amtn := valn /
// FancySummation).
func TestVRBackwardSolveAmount(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
		{Date: dateOf(2030, time.January, 1), Rate: 0.07},
	}
	base := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2024, time.February, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2040, time.February, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 1500,
			}},
			RateSchedule: schedule,
		}
	}

	fwd := Calculate(base())
	if fwd.Err != nil {
		t.Fatalf("VR forward: %v", fwd.Err)
	}

	// Backward: blank the amount, feed the sum value back.
	bwd := base()
	bwd.Periodics[0].AmtStatus = types.StatusEmpty
	bwd.Periodics[0].Amt = 0
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("VR backward solve: %v", res.Err)
	}
	if math.Abs(res.Periodics[0].Amt-1500) > 0.5 {
		t.Errorf("solved VR amount = %.2f, want 1500", res.Periodics[0].Amt)
	}
}
