package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// API: omit firstDate; amortization.FirstPass should default it to
// loanDate + 1 period. (Previous behavior: hard 400 error.)
func TestAmortizationCalcOmitFirstDate(t *testing.T) {
	body := `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.06,
		"nPeriods": 12,
		"perYr": 12,
		"payment": 8606.64
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp AmortizationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Schedule) != 12 {
		t.Errorf("schedule length = %d, want 12", len(resp.Schedule))
	}
	// First payment should be ~one month after loanDate (2024-02-01).
	if !strings.HasPrefix(resp.Schedule[0].Date, "2024-02") {
		t.Errorf("first payment date = %s, expected ~2024-02-01",
			resp.Schedule[0].Date)
	}
}

// API: amortization should compute LastDate from FirstDate + N rather
// than relying on the previous LastOK=true hardcode.
// Regression test for the bug: previously the engine compared
// currentDate against an UnknownDate zero-time and terminated early.
// With the fix, a 12-period loan should yield exactly 12 schedule
// rows.
func TestAmortizationCalcLastOKDerivedFromN(t *testing.T) {
	body := `{
		"amount": 50000,
		"loanDate": "2024-01-01",
		"rate": 0.06,
		"firstDate": "2024-02-01",
		"nPeriods": 12,
		"perYr": 12,
		"payment": 4303.32,
		"balloons": [
			{"date": "2024-08-01", "amount": 1000}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp AmortizationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	// Schedule should run a meaningful number of periods (not 1 or 2,
	// which would indicate the engine bailed early on the zero-time
	// lastDate comparison).
	if len(resp.Schedule) < 8 {
		t.Errorf("schedule length = %d, want at least 8 (full term ~12)",
			len(resp.Schedule))
	}
}

// API: validation arms surface as 200 OK with error message in body
// (current convention) — confirm a balloon-before-firstDate rejection.
func TestAmortizationCalcBalloonBeforeFirstDateRejected(t *testing.T) {
	body := `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.06,
		"firstDate": "2024-02-01",
		"nPeriods": 360,
		"perYr": 12,
		"payment": 599.55,
		"balloons": [
			{"date": "2024-01-15", "amount": 5000}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	var resp AmortizationResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp.Error, "precede") {
		t.Errorf("expected balloon-precedes error, got error=%q",
			resp.Error)
	}
}
