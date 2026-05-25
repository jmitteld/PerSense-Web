// Test for dispatch_gaps FP2 / A9: amortization APR-with-points
// (DOS EstimateAndRefineAPRwithPoints).

package api

import "testing"

// TestAmortAPRWithPoints: discount points raise the APR above the
// note rate, because the borrower's net proceeds shrink while the
// payment stream is unchanged.
func TestAmortAPRWithPoints(t *testing.T) {
	// No points: APR should be close to the 6% note rate.
	noPts, code := amzCall(t, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":360,"perYr":12,"payment":1199.10,"points":0.0
	}`)
	if code != 200 || noPts["error"] != nil {
		t.Fatalf("no-points calc failed: code=%d err=%v", code, noPts["error"])
	}
	apr0, ok := noPts["apr"].(float64)
	if !ok {
		t.Fatalf("no apr in response: %v", noPts)
	}
	if apr0 < 0.055 || apr0 > 0.065 {
		t.Errorf("zero-points APR = %.5f, expected near the 6%% note rate", apr0)
	}

	// 2 points: APR must rise.
	withPts, code := amzCall(t, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":360,"perYr":12,"payment":1199.10,"points":0.02
	}`)
	if code != 200 || withPts["error"] != nil {
		t.Fatalf("with-points calc failed: code=%d err=%v", code, withPts["error"])
	}
	apr2, _ := withPts["apr"].(float64)
	if apr2 <= apr0 {
		t.Errorf("APR with 2 points (%.5f) should exceed APR with no points (%.5f)",
			apr2, apr0)
	}
	if conv, _ := withPts["aprConverged"].(bool); !conv {
		t.Errorf("APR solve did not converge")
	}
}
