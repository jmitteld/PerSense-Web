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

// TestMortgageCompareAcceptsFinancedMonthlyRows is the audit finding F2
// regression: DOS's compare path (MortgageScreenUnit.pas:780-791) runs Calc for
// side effects but gates the comparison ONLY on EnoughDataForAPR — so a row
// given as {Amt Borrowed, Monthly, Rate, Years} (no Price funding) IS compared,
// even though Calc "refuses" to solve its price. Verified against the real DOS
// engine: `mtg_oracle aprfin 160000 1100 30 0.07` -> "apr 0.0732840000" and
// `... 1080 30 0.0675` -> "apr 0.0714400000". The handler previously aborted
// with a 400 on Calc's refusal; it must now return 200 with both APRs.
func TestMortgageCompareAcceptsFinancedMonthlyRows(t *testing.T) {
	body := `{
		"a": {"financed":160000,"monthly":1100,"years":30,"rate":0.07},
		"b": {"financed":160000,"monthly":1080,"years":30,"rate":0.0675}
	}`
	resp, code := postJSON(t, HandleMortgageCompare, body)
	if code != 200 {
		t.Fatalf("compare of financed+monthly rows returned %d: %v (want 200; DOS compares these)", code, resp["error"])
	}
	apr1, ok1 := resp["apr1"].(float64)
	apr2, ok2 := resp["apr2"].(float64)
	if !ok1 || !ok2 || apr1 <= 0 || apr2 <= 0 {
		t.Fatalf("expected positive apr1/apr2, got %v / %v", resp["apr1"], resp["apr2"])
	}
	// Anchored to the DOS oracle values (0.07328 / 0.07144); loose band tolerates
	// the loan-rate/true-rate boundary conversion without being brittle.
	if apr1 < 0.070 || apr1 > 0.077 {
		t.Errorf("apr1 = %.5f, want ~0.0733 (DOS oracle aprfin 160000 1100 30 0.07)", apr1)
	}
	// Same principal & term, higher monthly (1100 > 1080) ⇒ higher APR.
	if apr1 <= apr2 {
		t.Errorf("apr1 (%.5f) should exceed apr2 (%.5f) — higher monthly on the same 160k/30yr", apr1, apr2)
	}
}

// TestMortgageCompareStillRejectsUnderspecified confirms the F2 fix did not
// over-open the handler: a row lacking Monthly (so EnoughDataForAPR is false)
// must still be refused, matching DOS's "Fill out both lines completely" gate.
func TestMortgageCompareStillRejectsUnderspecified(t *testing.T) {
	body := `{
		"a": {"financed":160000,"years":30,"rate":0.07},
		"b": {"financed":160000,"monthly":1080,"years":30,"rate":0.0675}
	}`
	resp, code := postJSON(t, HandleMortgageCompare, body)
	if code != http.StatusBadRequest {
		t.Fatalf("compare with an under-specified row (no monthly) returned %d, want 400", code)
	}
	if resp["error"] == nil {
		t.Errorf("expected an error message for the under-specified compare")
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
