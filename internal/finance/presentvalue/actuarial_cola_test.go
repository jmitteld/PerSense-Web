package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// TestActuarialSteppedCOLA verifies dispatch_gaps V6-4: a
// life-contingent periodic payment with a COLA now applies the COLA
// in stepped (anniversary / month-specific) mode, not only
// continuously — so the result differs from the continuous-COLA run.
func TestActuarialSteppedCOLA(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1959, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)

	calc := func(colaMonth byte) float64 {
		s := vrTestSettings()
		s.COLAMonth = colaMonth
		in := PVInput{
			Settings: s,
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: 0.05},
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2024, time.July, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2044, time.July, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus:  types.InOutInput, Amt: 2000,
				COLAStatus: types.InOutInput, COLA: 0.03,
				Act: actuarial.Living,
			}},
			Actuarial: cfg,
		}
		r := Calculate(in)
		if r.Err != nil {
			t.Fatalf("calc (colaMonth=%d): %v", colaMonth, r.Err)
		}
		return r.SumValue
	}

	anniversary := calc(types.COLAAnnual)
	continuous := calc(types.COLAContinuous)
	if anniversary <= 0 || continuous <= 0 {
		t.Fatalf("implausible values: %.2f / %.2f", anniversary, continuous)
	}
	// Stepped (anniversary) and continuous COLA must produce
	// different present values on a contingent payment.
	if math.Abs(anniversary-continuous) < 1.0 {
		t.Errorf("stepped COLA (%.2f) should differ from continuous (%.2f) "+
			"on a life-contingent payment", anniversary, continuous)
	}
}
