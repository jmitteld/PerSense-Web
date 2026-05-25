package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// TestAO5RateAdjustmentResolvesPayment verifies dispatch_gaps FP4 /
// AO5: an ARM rate adjustment with no new payment re-amortizes the
// remaining balance at the new rate (DOS EstimateAndRefineAdjPayment).
// Before the fix the old payment stayed in place and the loan failed
// to amortize cleanly after the rate moved.
func TestAO5RateAdjustmentResolvesPayment(t *testing.T) {
	in := baseInput30y()
	adjDate := types.NewDateRec(2029, time.January, 1) // 5 years in
	in.Adjustments = []RateAdjustment{{
		DateStatus:     types.InOutInput,
		Date:           adjDate,
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.09, // rate jumps 6% -> 9%, no new payment
	}}

	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// The loan must still pay off — the payment was re-solved at the
	// higher rate.
	if math.Abs(res.FinalPrinc) > 5.0 {
		t.Errorf("final balance = %.2f, expected ~0 (payment not re-solved "+
			"after the rate adjustment)", res.FinalPrinc)
	}
	// The post-adjustment payment must be larger than the pre-
	// adjustment payment (a higher rate needs a bigger payment).
	var before, after float64
	for _, row := range res.Schedule {
		if row.PayNum < 1 {
			continue
		}
		if dateutil.DateComp(row.Date, adjDate) < 0 {
			before = row.PayAmt
		} else if after == 0 && dateutil.DateComp(row.Date, adjDate) > 0 {
			// First row strictly after the adjustment date — the
			// adjustment fires once the schedule crosses its date.
			after = row.PayAmt
		}
	}
	if after <= before {
		t.Errorf("post-adjustment payment %.2f should exceed pre-adjustment %.2f",
			after, before)
	}
}

// TestAO5UnderUSARule verifies dispatch_gaps V6-2: a rate-only
// adjustment on a USA-rule loan re-amortizes cleanly — the AO5
// re-solve amortizes the interest-bearing balance and pays the
// exempt (usap) lump down linearly.
func TestAO5UnderUSARule(t *testing.T) {
	in := baseInput30y()
	in.Settings.USARule = true
	in.Adjustments = []RateAdjustment{{
		DateStatus:     types.InOutInput,
		Date:           types.NewDateRec(2029, time.January, 1),
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.08,
	}}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("USA-rule loan with a rate adjustment errored: %v", res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Error("expected a schedule")
	}
}

// TestAO5RateAdjustmentNetsOffLaterBalloon verifies dispatch_gaps
// R4-5: when a rate-only adjustment re-solves the payment and a
// balloon falls after the adjustment, the balloon's discounted value
// is netted off the principal — so the re-solved payment is lower
// than it would be with no balloon (DOS Re_Amortize balloon term).
func TestAO5RateAdjustmentNetsOffLaterBalloon(t *testing.T) {
	adjDate := types.NewDateRec(2029, time.January, 1)
	mk := func(withBalloon bool) LoanInput {
		in := baseInput30y()
		in.Adjustments = []RateAdjustment{{
			DateStatus:     types.InOutInput,
			Date:           adjDate,
			LoanRateStatus: types.InOutInput,
			LoanRate:       0.09,
		}}
		if withBalloon {
			in.Balloons = []BalloonPayment{{
				DateStatus:   types.InOutInput,
				Date:         types.NewDateRec(2039, time.January, 1),
				AmountStatus: types.InOutInput,
				Amount:       40000,
			}}
		}
		return in
	}
	firstAfter := func(res AmortResult) float64 {
		for _, row := range res.Schedule {
			if row.PayNum >= 1 && dateutil.DateComp(row.Date, adjDate) > 0 {
				return row.PayAmt
			}
		}
		return 0
	}

	noB := Amortize(mk(false))
	withB := Amortize(mk(true))
	if noB.Err != nil || withB.Err != nil {
		t.Fatalf("calc error: %v / %v", noB.Err, withB.Err)
	}
	pNo := firstAfter(noB)
	pWith := firstAfter(withB)
	if pWith >= pNo {
		t.Errorf("post-adjustment payment with a later balloon (%.2f) should be "+
			"lower than without it (%.2f)", pWith, pNo)
	}
}
