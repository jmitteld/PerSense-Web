package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAmortTargetBalanceInverse verifies the balance→date wiring: the handler
// solves the date the balance reaches a target via ComputeDateFromBalanceDOS
// (arrears, exact vs the DOS oracle). Loan 100k @ 10%, $1500/mo, 120, 360-basis;
// target $50,000 reaches on 2029-01-01 per the DOS oracle.
func TestAmortTargetBalanceInverse(t *testing.T) {
	body := `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.10,
		"firstDate": "2024-02-01",
		"nPeriods": 120,
		"perYr": 12,
		"payment": 1500,
		"targetBalance": 50000
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc", strings.NewReader(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp AmortizationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !resp.PayoffDateValid {
		t.Fatalf("payoffDateValid false, error=%q", resp.PayoffDateError)
	}
	if resp.PayoffDateSolved != "2029-01-01" {
		t.Errorf("payoffDateSolved = %q, want 2029-01-01 (DOS oracle)", resp.PayoffDateSolved)
	}
}
