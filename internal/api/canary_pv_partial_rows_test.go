// Canary tests for dispatch_gaps.md S-4 / S-5: the PV frontend
// silently drops half-filled lump-sum and periodic rows before
// submitting (index.html:2264, 2283). These canaries exercise the
// API directly to bind the contract: the backend already supports
// these rows via backward-solve, so the silent-drop is a pure UI
// bug. When the frontend stops dropping the rows, the API behavior
// these tests pin down is what the user will see.
//
// Per the test plan, C-6 and C-7 are "binding" canaries — they
// document existing API support so the eventual frontend fix has a
// safety net. They may pass today.
//
// See docs/test_plan.md §1 (Wave 1 canaries) C-6, C-7.

package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"
)

// TestCanaryC6_PVLumpDateOnlyRowAcceptedByAPI binds the contract
// that a lump-sum row with only a Date (no Amount) is a valid input
// when SumValue is supplied — the backward solver (PV-1) returns
// the amount that yields that value at the given rate.
//
// The frontend currently filters this row out before posting; this
// canary proves the API already supports it.
//
// Pairs with: dispatch_gaps S-4. Fix: remove the JS filter at
// index.html:2264; the API behavior bound here is what users will
// see.
func TestCanaryC6_PVLumpDateOnlyRowAcceptedByAPI(t *testing.T) {
	// Forward at 6% to determine the target SumValue.
	fwd, _ := pvCall(t, `{
		"asOfDate":"2024-01-01",
		"rate":0.06,
		"lumpSums":[{"date":"2025-01-01","amount":10000}]
	}`)
	if fwd.Error != "" {
		t.Fatalf("forward error: %s", fwd.Error)
	}

	// Backward: lump row carries Date and Value, but Amount is omitted.
	// (Per the JSON contract, omitting `amount` makes the pointer nil
	// → the backward solver treats it as the unknown.)
	body, _ := json.Marshal(map[string]any{
		"asOfDate": "2024-01-01",
		"rate":     0.06,
		"sumValue": fwd.SumValue,
		"lumpSums": []map[string]any{
			{"date": "2025-01-01"},
		},
	})
	bwd, code := pvCall(t, string(body))
	if code != http.StatusOK {
		t.Fatalf("status = %d, body: <%s>", code, bwd.Error)
	}
	if bwd.Error != "" {
		t.Fatalf("CANARY: API rejected a lump row with Date but no Amount: %q. "+
			"PV-1 backward solve should accept this. (dispatch_gaps S-4)",
			bwd.Error)
	}
	if len(bwd.LumpSums) != 1 {
		t.Fatalf("got %d lump sums in response, want 1", len(bwd.LumpSums))
	}
	if math.Abs(bwd.LumpSums[0].Amount-10000) > 0.5 {
		t.Errorf("solved amount = %.2f, want ~10000", bwd.LumpSums[0].Amount)
	}
}

// TestCanaryC7_PVEmptyRowProducesUsefulErrorNotMisleadingMessage
// documents dispatch_gaps S-5: when the user enters multiple rows
// and the frontend drops them all (because each is half-filled),
// the user sees "Enter an as-of date and at least one lump sum or
// periodic payment" — but from the user's perspective, they DID
// enter rows. The Go test exercises the API equivalent: an
// explicitly-empty rows array. The API today returns the error
// "Enter ... at least one row" message at the JS layer; the canary
// asserts that when a row IS sent with no fields at all (the closest
// API analogue to "user typed but got dropped"), the response is
// either a specific per-row error or a clear screen-level message.
//
// Today, sending `{"lumpSums":[{}]}` with rate+asOf set ... behavior
// depends on the FirstPass classification. The canary's job is to
// pin whatever happens so the eventual fix is visible as a diff.
//
// Pairs with: dispatch_gaps S-5. This is a binding test more than a
// hard-fail test.
func TestCanaryC7_PVCompletelyEmptyRowDoesNotCrashOrMislead(t *testing.T) {
	body := `{
		"asOfDate": "2024-01-01",
		"rate": 0.06,
		"lumpSums": [{}]
	}`
	resp, code := pvCall(t, body)
	if code != http.StatusOK && code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 200 or 400; error: %s", code, resp.Error)
	}
	// We expect EITHER:
	//  (a) a specific per-row error mentioning "row 1" or "line 1", OR
	//  (b) a successful response that simply ignores the empty row
	//      (current observed behavior — also acceptable post-fix as
	//      long as the frontend stops creating phantom empty rows).
	if resp.Error == "" {
		// Acceptable today.
		return
	}
	lower := strings.ToLower(resp.Error)
	if strings.Contains(lower, "at least one lump sum") &&
		!strings.Contains(lower, "row 1") &&
		!strings.Contains(lower, "line 1") {
		t.Errorf("CANARY: API rejects an empty row with the misleading message %q "+
			"(dispatch_gaps S-5). It should either ignore empty rows or name "+
			"which row is problematic.", resp.Error)
	}
}
