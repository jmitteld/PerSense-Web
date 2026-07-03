package api

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAmortPayoffAPI verifies the payoffDate -> payoffBalance wiring and that it
// fixes the reported bug: the user's in-advance + Rule-of-78 loan queried at
// 05/01/2015 must return DOS's 101,422.75 (not the old client-side arrears-only
// 100,480.61), and a payoff before the loan date is rejected.
func TestAmortPayoffAPI(t *testing.T) {
	call := func(body map[string]any) AmortizationResponse {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/api/amortization/calc", bytes.NewReader(b))
		w := httptest.NewRecorder()
		HandleAmortizationCalc(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var resp AmortizationResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return resp
	}

	base := map[string]any{
		"amount": 100000.0, "loanDate": "2015-01-01", "rate": 0.08,
		"firstDate": "2015-02-01", "nPeriods": 360, "perYr": 12, "basis": "365",
		"inAdvance": true, "rule78": true, "exact": true, // prepaid default YES
	}

	// Valid payoff on 05/01/2015.
	b1 := map[string]any{}
	for k, v := range base {
		b1[k] = v
	}
	b1["payoffDate"] = "2015-05-01"
	resp := call(b1)
	if !resp.PayoffValid {
		t.Fatalf("expected PayoffValid, got error %q", resp.PayoffError)
	}
	if math.Abs(resp.PayoffBalance-101422.75) > 0.05 {
		t.Errorf("payoffBalance = %.2f, want DOS 101,422.75 (old buggy value was 100,480.61)", resp.PayoffBalance)
	}

	// Payoff before the loan date is rejected with a message, not a balance.
	b2 := map[string]any{}
	for k, v := range base {
		b2[k] = v
	}
	b2["payoffDate"] = "2014-12-01"
	resp2 := call(b2)
	if resp2.PayoffValid || resp2.PayoffError == "" {
		t.Errorf("expected a payoff error for a pre-loan date, got valid=%v err=%q", resp2.PayoffValid, resp2.PayoffError)
	}

	// No payoffDate -> no payoff fields populated.
	resp3 := call(base)
	if resp3.PayoffValid || resp3.PayoffBalance != 0 {
		t.Errorf("no payoffDate should leave payoff unset, got valid=%v bal=%.2f", resp3.PayoffValid, resp3.PayoffBalance)
	}
}
