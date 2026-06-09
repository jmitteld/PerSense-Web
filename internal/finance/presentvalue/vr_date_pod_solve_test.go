package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// These tests cover the variable-rate backward date and Payment-on-Death
// solves added to close the DOS re-review gaps: a blank periodic From/To
// date, a blank lump date, and an unknown POD, all under a rate
// schedule. Each is a round trip — forward-value with the field known,
// blank it, solve, and recover — so they pin that the solver inverts the
// same variable-rate forward value (schedule + COLA + contingency) that
// produced the target.

func vrDateDays(a, b types.DateRec) float64 {
	return math.Abs(a.Time.Sub(b.Time).Hours() / 24)
}

// TestVRSolvePeriodicToDate round-trips the variable-rate To Date solve,
// plain and life-contingent.
func TestVRSolvePeriodicToDate(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	from := dateOf(2030, time.January, 1)
	wantTo := dateOf(2042, time.January, 1)

	for _, tc := range []struct {
		name string
		act  byte
		cfg  *actuarial.ActuarialConfig
	}{
		{"plain", actuarial.NotContingent, nil},
		{"contingent", actuarial.Living, actuarialTestCfg(asOf, dateOf(1956, time.January, 1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := func() PVInput {
				return PVInput{
					Settings: vrTestSettings(),
					PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
					Periodics: []PeriodicPayment{{
						FromDateStatus: types.InOutInput, FromDate: from,
						ToDateStatus: types.InOutInput, ToDate: wantTo,
						PerYrStatus: types.InOutInput, PerYr: 12,
						AmtStatus: types.InOutInput, Amt: 1800,
						Act: tc.act,
					}},
					RateSchedule: vrAuditSchedule(),
					Actuarial:    tc.cfg,
				}
			}
			fwd := Calculate(base())
			if fwd.Err != nil {
				t.Fatalf("VR forward: %v", fwd.Err)
			}
			bwd := base()
			bwd.Periodics[0].ToDateStatus = types.StatusEmpty
			bwd.Periodics[0].ToDate = types.DateRec{}
			bwd.PresVal.SumValueStatus = types.InOutInput
			bwd.PresVal.SumValue = fwd.SumValue
			res := Calculate(bwd)
			if res.Err != nil {
				t.Fatalf("VR To Date solve: %v", res.Err)
			}
			if d := vrDateDays(res.Periodics[0].ToDate, wantTo); d > 5 {
				t.Errorf("%s: solved To Date = %s, want %s (off %.0f days)",
					tc.name, res.Periodics[0].ToDate.Time.Format("2006-01-02"),
					wantTo.Time.Format("2006-01-02"), d)
			}
		})
	}
}

// TestVRSolvePeriodicFromDate round-trips the variable-rate From Date
// solve. From Date anchors every installment, so it resolves to ~1 day.
func TestVRSolvePeriodicFromDate(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	wantFrom := dateOf(2030, time.January, 1)
	to := dateOf(2042, time.January, 1)

	base := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: wantFrom,
				ToDateStatus: types.InOutInput, ToDate: to,
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 1800,
			}},
			RateSchedule: vrAuditSchedule(),
		}
	}
	fwd := Calculate(base())
	if fwd.Err != nil {
		t.Fatalf("VR forward: %v", fwd.Err)
	}
	bwd := base()
	bwd.Periodics[0].FromDateStatus = types.StatusEmpty
	bwd.Periodics[0].FromDate = types.DateRec{}
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("VR From Date solve: %v", res.Err)
	}
	if d := vrDateDays(res.Periodics[0].FromDate, wantFrom); d > 3 {
		t.Errorf("solved From Date = %s, want %s (off %.0f days)",
			res.Periodics[0].FromDate.Time.Format("2006-01-02"),
			wantFrom.Time.Format("2006-01-02"), d)
	}
}

// TestVRSolveLumpDate round-trips the variable-rate lump-sum date solve.
func TestVRSolveLumpDate(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	wantDate := dateOf(2038, time.July, 1)

	base := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: wantDate,
				AmtStatus: types.InOutInput, Amt: 100000,
			}},
			RateSchedule: vrAuditSchedule(),
		}
	}
	fwd := Calculate(base())
	if fwd.Err != nil {
		t.Fatalf("VR forward: %v", fwd.Err)
	}
	bwd := base()
	bwd.LumpSums[0].DateStatus = types.StatusEmpty
	bwd.LumpSums[0].Date = types.DateRec{}
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("VR lump date solve: %v", res.Err)
	}
	if d := vrDateDays(res.LumpSums[0].Date, wantDate); d > 3 {
		t.Errorf("solved lump Date = %s, want %s (off %.0f days)",
			res.LumpSums[0].Date.Time.Format("2006-01-02"),
			wantDate.Time.Format("2006-01-02"), d)
	}
}

// TestVRSolveUnknownPOD round-trips the variable-rate unknown-POD solve.
// Routing through the VR forward means the death benefit is discounted
// through the schedule, so the solved POD must match the input exactly.
func TestVRSolveUnknownPOD(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1)
	const wantPOD = 75000.0

	base := func() PVInput {
		cfg := actuarialTestCfg(asOf, dob)
		cfg.POD = wantPOD
		return PVInput{
			Settings: vrTestSettings(),
			PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2040, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 1000,
				Act: actuarial.Living,
			}},
			RateSchedule: vrAuditSchedule(),
			Actuarial:    cfg,
		}
	}
	fwd := Calculate(base())
	if fwd.Err != nil {
		t.Fatalf("VR forward: %v", fwd.Err)
	}
	if fwd.PODValue <= 0 {
		t.Fatalf("expected positive POD value, got %.4f", fwd.PODValue)
	}
	bwd := base()
	bwd.Actuarial.POD = 0
	bwd.Actuarial.PODUnknown = true
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("VR unknown-POD solve: %v", res.Err)
	}
	if math.Abs(res.POD-wantPOD) > 1.0 {
		t.Errorf("solved VR POD = %.4f, want %.4f", res.POD, wantPOD)
	}
}

// TestVRDateSolveUnreachableTarget confirms a clean error (not a wild
// answer) when the target Present Value can't be reached at any date.
func TestVRDateSolveUnreachableTarget(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	in := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			// A single $1000 lump can never be worth $1,000,000 today.
			SumValueStatus: types.InOutInput, SumValue: 1000000,
		},
		LumpSums: []LumpSumPayment{{
			AmtStatus: types.InOutInput, Amt: 1000,
		}},
		RateSchedule: vrAuditSchedule(),
	}
	res := Calculate(in)
	if res.Err == nil {
		t.Errorf("expected an unreachable-target error, got date %s",
			res.LumpSums[0].Date.Time.Format("2006-01-02"))
	}
}
