package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// Corner cases for the variable-rate (PVL-fancy) schedule path.

func vrPeriodicForward(sched []RateLine, asOf, from, to types.DateRec, perYr int, amt float64) PVResult {
	return Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: from,
			ToDateStatus: types.InOutInput, ToDate: to,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			AmtStatus: types.InOutInput, Amt: amt,
		}},
		RateSchedule: sched,
	})
}

// TestVRSingleEntryMatchesFixedRate: a one-line schedule (just the
// starting rate) must equal the fixed-rate calc at that rate.
func TestVRSingleEntryMatchesFixedRate(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	from := dateOf(2030, time.January, 1)
	to := dateOf(2040, time.January, 1)
	rate := 0.05

	vr := vrPeriodicForward([]RateLine{{Date: dateOf(1900, time.January, 1), Rate: rate}},
		asOf, from, to, 12, 1000)
	if vr.Err != nil {
		t.Fatalf("VR forward: %v", vr.Err)
	}
	fixed := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf, R: RateEntry{Status: types.InOutInput, Rate: rate}},
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: from,
			ToDateStatus: types.InOutInput, ToDate: to,
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 1000,
		}},
	})
	if fixed.Err != nil {
		t.Fatalf("fixed forward: %v", fixed.Err)
	}
	if math.Abs(vr.SumValue-fixed.SumValue) > 0.01 {
		t.Errorf("VR single-entry %.4f != fixed-rate %.4f", vr.SumValue, fixed.SumValue)
	}
}

// TestVRUnsortedScheduleMatchesSorted: the engine sorts the schedule
// defensively, so an out-of-order schedule gives the same answer.
func TestVRUnsortedScheduleMatchesSorted(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	from := dateOf(2030, time.January, 1)
	to := dateOf(2040, time.January, 1)
	sorted := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
		{Date: dateOf(2034, time.January, 1), Rate: 0.07},
	}
	unsorted := []RateLine{
		{Date: dateOf(2034, time.January, 1), Rate: 0.07},
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
	}
	a := vrPeriodicForward(sorted, asOf, from, to, 12, 1000)
	b := vrPeriodicForward(unsorted, asOf, from, to, 12, 1000)
	if a.Err != nil || b.Err != nil {
		t.Fatalf("errors: %v / %v", a.Err, b.Err)
	}
	if math.Abs(a.SumValue-b.SumValue) > 1e-9 {
		t.Errorf("unsorted schedule %.6f != sorted %.6f", b.SumValue, a.SumValue)
	}
}

// TestVRZeroRateNoDiscount: a zero-rate schedule does no discounting,
// so a lump's present value is its face amount.
func TestVRZeroRateNoDiscount(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 100000,
		}},
		RateSchedule: []RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.0}},
	})
	if res.Err != nil {
		t.Fatalf("calc: %v", res.Err)
	}
	if math.Abs(res.SumValue-100000) > 0.01 {
		t.Errorf("zero-rate VR lump value = %.4f, want 100000", res.SumValue)
	}
}

// TestVRNegativeRateAccumulates: a negative-rate schedule accumulates,
// so a future lump is worth more than its face amount today.
func TestVRNegativeRateAccumulates(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 100000,
		}},
		RateSchedule: []RateLine{{Date: dateOf(1900, time.January, 1), Rate: -0.02}},
	})
	if res.Err != nil {
		t.Fatalf("calc: %v", res.Err)
	}
	if !(res.SumValue > 100000) || math.IsInf(res.SumValue, 0) {
		t.Errorf("negative-rate VR lump value = %.4f, want a finite value > 100000", res.SumValue)
	}
}

// TestVRStepOnPaymentDate: a rate change landing exactly on a payment
// date produces a finite value bracketed by the all-low and all-high
// single-rate values.
func TestVRStepOnPaymentDate(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	from := dateOf(2030, time.January, 1)
	to := dateOf(2040, time.January, 1)
	step := dateOf(2035, time.January, 1) // a payment date (monthly from Jan 1)

	mixed := vrPeriodicForward([]RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
		{Date: step, Rate: 0.08},
	}, asOf, from, to, 12, 1000)
	low := vrPeriodicForward([]RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.04}}, asOf, from, to, 12, 1000)
	high := vrPeriodicForward([]RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.08}}, asOf, from, to, 12, 1000)
	for _, r := range []PVResult{mixed, low, high} {
		if r.Err != nil {
			t.Fatalf("calc: %v", r.Err)
		}
	}
	if !(mixed.SumValue < low.SumValue && mixed.SumValue > high.SumValue) {
		t.Errorf("step-on-payment value %.4f not between high-rate %.4f and low-rate %.4f",
			mixed.SumValue, high.SumValue, low.SumValue)
	}
}

// TestVRFarFutureStepIgnored: a rate step dated after every payment has
// no effect — the result equals the single starting-rate calc.
func TestVRFarFutureStepIgnored(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	from := dateOf(2030, time.January, 1)
	to := dateOf(2040, time.January, 1)
	withStep := vrPeriodicForward([]RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.05},
		{Date: dateOf(2200, time.January, 1), Rate: 0.09},
	}, asOf, from, to, 12, 1000)
	starting := vrPeriodicForward([]RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.05}}, asOf, from, to, 12, 1000)
	if withStep.Err != nil || starting.Err != nil {
		t.Fatalf("errors: %v / %v", withStep.Err, starting.Err)
	}
	if math.Abs(withStep.SumValue-starting.SumValue) > 1e-9 {
		t.Errorf("far-future step changed the value: %.6f vs %.6f", withStep.SumValue, starting.SumValue)
	}
}

// TestVRPastPaymentAccumulates: a payment dated before the as-of date is
// accumulated forward through the schedule, so its value exceeds face.
func TestVRPastPaymentAccumulates(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2020, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 100000,
		}},
		RateSchedule: []RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.05}},
	})
	if res.Err != nil {
		t.Fatalf("calc: %v", res.Err)
	}
	if !(res.SumValue > 100000) || math.IsInf(res.SumValue, 0) || math.IsNaN(res.SumValue) {
		t.Errorf("past VR payment value = %.4f, want a finite value > 100000", res.SumValue)
	}
}

// TestVRDuplicateScheduleDates: two schedule entries on the same date
// must not crash and must yield a finite value.
func TestVRDuplicateScheduleDates(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	res := vrPeriodicForward([]RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
		{Date: dateOf(2032, time.January, 1), Rate: 0.07},
		{Date: dateOf(2032, time.January, 1), Rate: 0.09},
	}, asOf, dateOf(2030, time.January, 1), dateOf(2040, time.January, 1), 12, 1000)
	if res.Err != nil {
		t.Fatalf("duplicate-date schedule errored: %v", res.Err)
	}
	if res.SumValue <= 0 || math.IsInf(res.SumValue, 0) || math.IsNaN(res.SumValue) {
		t.Errorf("duplicate-date schedule value = %.4f, want finite > 0", res.SumValue)
	}
}

// TestVROutOfOrderDatesError: a periodic row whose From Date is after
// its To Date must error cleanly under a variable-rate schedule.
func TestVROutOfOrderDatesError(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	res := vrPeriodicForward([]RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.05}},
		asOf, dateOf(2040, time.January, 1), dateOf(2030, time.January, 1), 12, 1000)
	if res.Err == nil {
		t.Errorf("expected an out-of-order dates error, got SumValue %.4f", res.SumValue)
	}
}

// TestVRTwoLifeOrdering: under a rate schedule, two-life contingency
// values keep the expected ordering Both ≤ Living ≤ Either ≤ None.
func TestVRTwoLifeOrdering(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := twoLifeCfg(asOf, dateOf(1956, time.January, 1), dateOf(1961, time.January, 1))
	sched := vrAuditSchedule()

	val := func(act byte) float64 {
		r := Calculate(PVInput{
			Settings: vrTestSettings(),
			PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: dateOf(2042, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 100000, Act: act,
			}},
			RateSchedule: sched,
			Actuarial:    cfg,
		})
		if r.Err != nil {
			t.Fatalf("VR act=%d: %v", act, r.Err)
		}
		return r.SumValue
	}
	none := val(actuarial.NotContingent)
	either := val(actuarial.EitherLiving)
	living := val(actuarial.Living)
	both := val(actuarial.BothLiving)
	if !(none >= either-1e-6 && either >= living-1e-6 && living >= both-1e-6) {
		t.Errorf("VR two-life ordering violated: None=%.2f Either=%.2f Living=%.2f Both=%.2f",
			none, either, living, both)
	}
	if !(none > both) {
		t.Errorf("expected VR None (%.2f) > Both (%.2f)", none, both)
	}
}
