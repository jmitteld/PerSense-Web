package presentvalue

// Coverage for a few remaining reachable edge branches: the direct
// vrPeriodicValue peryr guard, and the zero-contribution guards in the
// VR amount / POD solvers and the fixed-rate periodic-amount solver.

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// A from-date solve on a daily (peryr=365) periodic series drives
// refineDateByDays into the sub-2-day-period guard (backward.go:1148):
// one payment period is a single calendar day, so the day-level
// bisection has no room and keeps the grid date.
func TestRefineDateDailyPeriodGuard(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	fwd := PVInput{
		Settings: defaultSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: newDate(2025, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: newDate(2025, time.March, 1),
			PerYrStatus: types.InOutInput, PerYr: 365,
			AmtStatus: types.InOutInput, Amt: 10,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: 0.08},
		},
	}
	fr := forwardOnly(fwd)
	if fr.Err != nil {
		t.Fatalf("forward setup: %v", fr.Err)
	}
	target := fr.Periodics[0].Val

	solve := fwd
	ps := make([]PeriodicPayment, 1)
	copy(ps, fwd.Periodics)
	ps[0].FromDateStatus = types.StatusEmpty
	ps[0].FromDate = types.UnknownDate()
	ps[0].ValStatus = types.InOutInput
	ps[0].Val = target
	solve.Periodics = ps
	solve.PresVal.SumValueStatus = types.InOutInput
	solve.PresVal.SumValue = target

	res := Calculate(solve)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
}

// vrPeriodicValue rejects peryr<=0 directly (variablerate.go:163). The
// forward driver guards peryr first, so exercise the function itself.
func TestVRPeriodicValuePerYrGuard(t *testing.T) {
	settings := vrTestSettings()
	_, _, _, err := vrPeriodicValue(100, 0, dateOf(2024, time.January, 1),
		dateOf(2025, time.January, 1), dateOf(2030, time.January, 1),
		0, vrSched(), &settings, nil, actuarial.NotContingent)
	if err == nil || !strings.Contains(err.Error(), "Pmts-Yr") {
		t.Fatalf("expected peryr error, got %v", err)
	}
}

// VR amount solve where the unknown row contributes ~0 present value
// triggers the zero-unit guard (variablerate.go:464). A lump sum dated
// astronomically far out under a positive rate discounts to ~0, so the
// unit value (PV per $1) underflows teeny.
func TestSolveVariableRateAmountZeroUnit(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	in := PVInput{
		Settings: vrTestSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2300, time.January, 1), // ~276y out
			// amount blank -> solved
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			SumValueStatus: types.InOutInput, SumValue: 1000,
		},
		RateSchedule: []RateLine{{Date: dateOf(1900, time.January, 1), Rate: 0.30}},
	}
	res := Calculate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "present value of zero") {
		t.Fatalf("expected zero-unit amount-solve error, got %v / amt=%v",
			res.Err, res.LumpSums)
	}
}

// Fixed-rate periodic-amount solve where the stream's present-value
// factor is ~0 triggers the zero-factor guard (backward.go:855). A
// short periodic series dated centuries out at a high rate discounts to
// essentially zero, so dividing the target by it has no answer.
func TestSolvePeriodicAmountZeroFactor(t *testing.T) {
	asOf := newDate(2024, time.January, 1)
	in := PVInput{
		Settings: defaultSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: newDate(2300, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: newDate(2301, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			// amount blank -> solved
			ValStatus: types.InOutInput, Val: 1000,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.30},
			SumValueStatus: types.InOutInput, SumValue: 1000,
		},
	}
	res := Calculate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "essentially zero") {
		t.Fatalf("expected zero-factor periodic-amount error, got %v", res.Err)
	}
}

// VR POD solve where the death benefit has ~0 present value triggers the
// zero-unit POD guard (variablerate.go:519). A person already far past
// the life-table horizon has ~0 remaining death probability.
func TestSolveVariableRatePODZeroUnit(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1700, time.January, 1)) // long past horizon
	cfg.PODUnknown = true
	in := PVInput{
		Settings: vrTestSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2025, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 1000,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			SumValueStatus: types.InOutInput, SumValue: 2000,
		},
		RateSchedule: vrSched(),
		Actuarial:    cfg,
	}
	res := Calculate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "no chance of being paid") {
		t.Fatalf("expected zero-unit POD error, got %v / pod=%v", res.Err, res.POD)
	}
}
