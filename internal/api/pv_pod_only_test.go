package api

import (
	"encoding/json"
	"math"
	"testing"
)

// podOnlyQx builds a small mock mortality table in the API's wire
// format ([[age, qx], ...]) — same curve as the presentvalue package's
// actuarialTestCfg, so failures here vs. there isolate the API layer.
func podOnlyQx() [][]float64 {
	qx := make([][]float64, 121)
	for a := 0; a <= 120; a++ {
		q := 0.001 + 0.0001*float64(a)*float64(a)/120.0
		if q > 1 {
			q = 1
		}
		qx[a] = []float64{float64(a), q}
	}
	qx[120][1] = 1
	return qx
}

// TestAPIPVPODOnly verifies dispatch_gaps §4.1 S-8: a PV request with
// NO lump sums and NO periodic payments but a Payment on Death is a
// valid forward calculation. DOS FrontwardCalc (PRESVALU.pas:689) adds
// PodValue after the (empty) row loops, so the screen total is exactly
// the POD's actuarial present value. Engine-level counterpart:
// presentvalue/pod_only_test.go: TestPODOnlyForward.
func TestAPIPVPODOnly(t *testing.T) {
	makeBody := func(pod float64) string {
		b, _ := json.Marshal(map[string]any{
			"asOfDate": "2024-01-01",
			"rate":     0.06,
			"actuarial": map[string]any{
				"table1":  podOnlyQx(),
				"dob1":    "1940-10-10",
				"asOfNow": "2024-01-01",
				"pod":     pod,
			},
		})
		return string(b)
	}

	resp, code := pvCall(t, makeBody(20000))
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	if resp.Error != "" {
		t.Fatalf("POD-only request rejected: %s", resp.Error)
	}

	// The total must be the POD value itself — nothing else is on the
	// worksheet — and it must be a real discounted expected benefit:
	// positive, but strictly less than the undiscounted POD amount.
	if resp.PODValue <= 0 {
		t.Fatalf("podValue = %.2f, want > 0", resp.PODValue)
	}
	if resp.PODValue >= 20000 {
		t.Errorf("podValue = %.2f, want < POD amount 20000 (discounting)", resp.PODValue)
	}
	if math.Abs(resp.SumValue-resp.PODValue) > 0.005 {
		t.Errorf("sumValue = %.2f, podValue = %.2f — want equal on a POD-only worksheet",
			resp.SumValue, resp.PODValue)
	}

	// The POD present value is linear in the POD amount (PODValue is a
	// fixed expected-discount factor times the amount): doubling the
	// POD must double the result, up to cent rounding.
	resp2, _ := pvCall(t, makeBody(40000))
	if resp2.Error != "" {
		t.Fatalf("doubled-POD request rejected: %s", resp2.Error)
	}
	if math.Abs(resp2.PODValue-2*resp.PODValue) > 0.02 {
		t.Errorf("podValue(40000) = %.2f, want 2×podValue(20000) = %.2f",
			resp2.PODValue, 2*resp.PODValue)
	}
}
