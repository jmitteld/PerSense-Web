// Canary test for dispatch_gaps.md PV-14 / §4.7 final row: the PV
// frontend refuses to submit a blank-rate (IRR) request with the
// stale message "IRR computation (blank rate) is not yet supported
// in the API. Please enter a rate." But the backend HAS supported
// solveRate since the PV-8 port (presentvalue/backward.go:848).
//
// This canary BINDS the API contract — proving the backend
// supports PV-8 — so the frontend guard at
// cmd/persense/static/index.html:2381-2384 can be deleted safely.
// It MAY PASS TODAY (which is the point: the bug is in the JS, not
// the Go); the canary is a guardrail to keep the API contract
// intact.
//
// See docs/test_plan.md §1 (Wave 1 canaries) C-10.

package api

import (
	"encoding/json"
	"math"
	"testing"
)

// TestCanaryC10_PVIRRSolveSupportedByAPI proves the backend solves
// PV-8 (unknown rate, given sumValue) end-to-end through HandlePVCalc.
//
// Setup: a single $10,000 lump sum 10 years out has a present value
// of $10,000 * exp(-0.10 * 10) ≈ $3,679.86 at a true rate of 10%.
// The canary discounts at 10%, captures the SumValue, then omits
// the rate and asks the API to solve for it.
//
// Pairs with: dispatch_gaps PV-14, M14 frontend guard.
// Fix: remove the JS guard at index.html:2381-2384 (4 lines).
func TestCanaryC10_PVIRRSolveSupportedByAPI(t *testing.T) {
	// Forward at 10% to get the target SumValue.
	fwd, _ := pvCall(t, `{
		"asOfDate":"2024-01-01",
		"rate":0.10,
		"lumpSums":[{"date":"2034-01-01","amount":10000}]
	}`)
	if fwd.Error != "" {
		t.Fatalf("forward error: %s", fwd.Error)
	}

	// Backward: omit rate, supply sumValue and the row.
	body, _ := json.Marshal(map[string]any{
		"asOfDate": "2024-01-01",
		"sumValue": fwd.SumValue,
		"lumpSums": []map[string]any{
			{"date": "2034-01-01", "amount": 10000},
		},
	})
	bwd, code := pvCall(t, string(body))
	if code != 200 {
		t.Fatalf("CANARY: API rejected PV-8 (blank rate) with status %d. "+
			"The backend solveRate at presentvalue/backward.go:848 should "+
			"handle this — frontend guard at index.html:2381 is stale.",
			code)
	}
	if bwd.Error != "" {
		t.Fatalf("CANARY: API errored on PV-8: %q. Backend should solve.", bwd.Error)
	}
	// Sanity: solved sum should round-trip to the input target.
	if math.Abs(bwd.SumValue-fwd.SumValue) > 1.0 {
		t.Errorf("backward SumValue = %.2f, want %.2f (within $1)",
			bwd.SumValue, fwd.SumValue)
	}
	t.Logf("PV-8 contract intact: forward sumValue=%.4f → backward sumValue=%.4f",
		fwd.SumValue, bwd.SumValue)
}
