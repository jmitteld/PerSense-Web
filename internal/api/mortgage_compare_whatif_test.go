// Functional tests for the mortgage compare / what-if endpoints
// (dispatch_gaps QW3 — frontend now calls these instead of the local
// heuristic / client-side loop). These exercise the handlers directly.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func postJSON(t *testing.T, h http.HandlerFunc, body string) (map[string]any, int) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	h(w, req)
	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m, w.Code
}

// TestMortgageCompareEndpoint: two mortgages with different rates are
// compared; the handler must return both full-term APRs.
func TestMortgageCompareEndpoint(t *testing.T) {
	body := `{
		"a": {"price":200000,"pctDown":0.20,"years":30,"rate":0.06,"points":0.01},
		"b": {"price":200000,"pctDown":0.20,"years":30,"rate":0.0625,"points":0.0}
	}`
	resp, code := postJSON(t, HandleMortgageCompare, body)
	if code != 200 {
		t.Fatalf("compare returned %d: %v", code, resp["error"])
	}
	apr1, ok1 := resp["apr1"].(float64)
	apr2, ok2 := resp["apr2"].(float64)
	if !ok1 || !ok2 || apr1 <= 0 || apr2 <= 0 {
		t.Fatalf("expected positive apr1/apr2, got %v / %v", resp["apr1"], resp["apr2"])
	}
	// Mortgage A pays a point, so its APR should exceed its 6% note rate.
	if apr1 <= 0.06 {
		t.Errorf("apr1 = %.5f, expected > 0.06 (1 point should lift APR)", apr1)
	}
}

// TestMortgageWhatIfEndpoint: vary the rate across 3 rows and confirm
// the engine steps the rate and re-solves the monthly payment.
func TestMortgageWhatIfEndpoint(t *testing.T) {
	body := `{
		"base": {"price":200000,"pctDown":0.20,"years":30,"rate":0.06},
		"vary": "rate",
		"increment": 0.005,
		"count": 3
	}`
	resp, code := postJSON(t, HandleMortgageWhatIf, body)
	if code != 200 {
		t.Fatalf("whatif returned %d: %v", code, resp["error"])
	}
	rows, ok := resp["rows"].([]any)
	if !ok || len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %v", resp["rows"])
	}
	r0 := rows[0].(map[string]any)
	r1 := rows[1].(map[string]any)
	// Row 0 is the base (6%); row 1 is +0.5%.
	if got := r0["rate"].(float64); got < 0.0599 || got > 0.0601 {
		t.Errorf("row0 rate = %.5f, want 0.06", got)
	}
	if got := r1["rate"].(float64); got < 0.0649 || got > 0.0651 {
		t.Errorf("row1 rate = %.5f, want 0.065", got)
	}
	// A higher rate must re-solve to a higher monthly payment.
	if r1["monthly"].(float64) <= r0["monthly"].(float64) {
		t.Errorf("row1 monthly (%.2f) should exceed row0 monthly (%.2f)",
			r1["monthly"], r0["monthly"])
	}
}

// TestMortgageWhatIfRejectsUnknownVary: an unsupported vary field is a
// clean 400, not a panic.
func TestMortgageWhatIfRejectsUnknownVary(t *testing.T) {
	body := `{"base":{"price":200000,"pctDown":0.2,"years":30,"rate":0.06},
		"vary":"tax","increment":10,"count":2}`
	resp, code := postJSON(t, HandleMortgageWhatIf, body)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown vary field, got %d", code)
	}
	if resp["error"] == nil {
		t.Errorf("expected an error message for unknown vary field")
	}
}
