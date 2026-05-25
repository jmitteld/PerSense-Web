package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestRoundTripPeriodicFromDateWithCOLA verifies dispatch_gaps FP9:
// the PV-6 fromDate solve with a non-zero COLA. The DOS second
// approximation (PRESVALU.pas:1029-1035) is needed for the estimate
// to converge near the true fromDate when COLA != 0.
func TestRoundTripPeriodicFromDateWithCOLA(t *testing.T) {
	asof := newDate(2024, time.January, 1)
	knownFrom := newDate(2024, time.January, 1)
	to := newDate(2030, time.January, 1)
	rate := 0.06
	cola := 0.03
	amount := 1000.0

	forward := PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: knownFrom,
			ToDateStatus: types.InOutInput, ToDate: to,
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus:  types.InOutInput, Amt: amount,
			COLAStatus: types.InOutInput, COLA: cola,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asof,
			R: RateEntry{Status: types.StatusFromRate, Rate: rate},
		},
		Settings: defaultSettings(),
	}
	fwd := Calculate(forward)
	if fwd.Err != nil {
		t.Fatal(fwd.Err)
	}

	backward := PVInput{
		Periodics: []PeriodicPayment{{
			ToDateStatus: types.InOutInput, ToDate: to,
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus:  types.InOutInput, Amt: amount,
			COLAStatus: types.InOutInput, COLA: cola,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asof,
			R:              RateEntry{Status: types.StatusFromRate, Rate: rate},
			SumValueStatus: types.InOutInput, SumValue: fwd.SumValue,
		},
		Settings: defaultSettings(),
	}
	bwd := Calculate(backward)
	if bwd.Err != nil {
		t.Fatalf("fromDate solve failed: %v", bwd.Err)
	}
	got := bwd.Periodics[0].FromDate
	delta := math.Abs(got.Time.Sub(knownFrom.Time).Hours() / 24)
	if delta > 45 {
		t.Errorf("solved fromDate = %s, want %s (delta %.0f days) — "+
			"cola second-approx may be missing",
			got.Time.Format("2006-01-02"),
			knownFrom.Time.Format("2006-01-02"), delta)
	}
}
