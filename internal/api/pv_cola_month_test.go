// Test for dispatch_gaps R4-6: month-specific and continuous COLA.
// The PV handler used to hard-code annual (anniversary) COLA; it now
// honors a colaMonth from the request.

package api

import (
	"math"
	"testing"
)

// TestCOLAModesDiffer: an anniversary COLA, a continuous COLA, and a
// month-specific COLA over the same periodic payment all produce
// different present values — proof the colaMonth setting is honored.
func TestCOLAModesDiffer(t *testing.T) {
	row := `"periodics":[{"fromDate":"2025-03-15","toDate":"2045-03-15",
		"perYr":12,"amount":1000,"cola":0.03}]`
	calc := func(extra string) float64 {
		resp, code := pvCall(t, `{"asOfDate":"2025-01-01","rate":0.06,`+row+extra+`}`)
		if code != 200 || resp.Error != "" {
			t.Fatalf("PV calc failed: code=%d err=%q", code, resp.Error)
		}
		return resp.SumValue
	}
	anniversary := calc("")               // default
	continuous := calc(`,"colaMonth":98`) // CNT
	january := calc(`,"colaMonth":1`)     // step on Jan 1

	if math.Abs(anniversary-continuous) < 1.0 {
		t.Errorf("continuous COLA (%.2f) should differ from anniversary (%.2f)",
			continuous, anniversary)
	}
	if math.Abs(anniversary-january) < 1.0 {
		t.Errorf("January-stepped COLA (%.2f) should differ from anniversary (%.2f)",
			january, anniversary)
	}
	// All three must still be sane positive present values. The
	// modes legitimately differ (smooth vs stepped intra-year), but
	// none should be wildly off — a continuous COLA that drifts far
	// above anniversary would signal the yield/continuous-rate
	// conversion (dispatch_gaps V6-5) is wrong.
	for _, v := range []float64{anniversary, continuous, january} {
		if v <= 0 || v > 1e9 {
			t.Errorf("implausible present value %.2f", v)
		}
	}
	if math.Abs(continuous-anniversary)/anniversary > 0.03 {
		t.Errorf("continuous COLA (%.2f) is implausibly far from anniversary "+
			"(%.2f) — check the V6-5 yield conversion", continuous, anniversary)
	}
}
