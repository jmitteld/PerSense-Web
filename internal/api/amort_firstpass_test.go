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

// API: omit nPeriods and supply lastDate; FirstPass should derive
// nPeriods from firstDate + lastDate (DOS A-FP-n). Matches the
// Help/Amortization Example 1c flow: "Enter 1st Pmt Date=02/01/2024,
// Last Pmt Date=01/01/2054, Pmts/Yr=12. Leave # Periods blank."
func TestAmortizationCalcDeriveNPeriodsFromLastDate(t *testing.T) {
	body := `{
		"amount": 250000,
		"loanDate": "2024-01-01",
		"rate": 0.06,
		"firstDate": "2024-02-01",
		"lastDate": "2054-01-01",
		"perYr": 12
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
	// 30-year monthly loan = 360 periods (the help-example answer).
	if resp.NPeriods != 360 {
		t.Errorf("derived nPeriods = %d, want 360", resp.NPeriods)
	}
	if len(resp.Schedule) != 360 {
		t.Errorf("schedule length = %d, want 360", len(resp.Schedule))
	}
	// Echoed firstDate + lastDate should match what the engine used.
	if resp.FirstDate != "2024-02-01" {
		t.Errorf("firstDate = %q, want 2024-02-01", resp.FirstDate)
	}
	if resp.LastDate != "2054-01-01" {
		t.Errorf("lastDate = %q, want 2054-01-01", resp.LastDate)
	}
}

// API: when nPeriods is supplied and lastDate is blank, the response
// should still echo back the derived lastDate so the UI can fill the
// blank cell. (The complement of the Example 1c test above.)
func TestAmortizationCalcEchoesDerivedLastDate(t *testing.T) {
	body := `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.06,
		"firstDate": "2024-02-01",
		"nPeriods": 360,
		"perYr": 12
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
	if resp.NPeriods != 360 {
		t.Errorf("echoed nPeriods = %d, want 360", resp.NPeriods)
	}
	// firstDate + (nPeriods-1) monthly periods from 2024-02-01 lands at 2054-01-01.
	if resp.LastDate != "2054-01-01" {
		t.Errorf("derived lastDate = %q, want 2054-01-01", resp.LastDate)
	}
}

// API: derive-only mode — Amount and Rate both blank means "tell me
// the term, don't compute a schedule." Matches Help/Amortization
// Example 1c exactly as the help originally framed it: enter the
// dates and payment frequency, get back the period count.
func TestAmortizationCalcDeriveOnlyMode(t *testing.T) {
	body := `{
		"firstDate": "2024-02-01",
		"lastDate": "2054-01-01",
		"perYr": 12
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
	if resp.NPeriods != 360 {
		t.Errorf("derived nPeriods = %d, want 360", resp.NPeriods)
	}
	if resp.FirstDate != "2024-02-01" {
		t.Errorf("firstDate = %q, want 2024-02-01", resp.FirstDate)
	}
	if resp.LastDate != "2054-01-01" {
		t.Errorf("lastDate = %q, want 2054-01-01", resp.LastDate)
	}
	// Derive-only should NOT produce a schedule or totals — that's the
	// point of avoiding Amortize. Catching this prevents accidentally
	// running schedule generation with zero amount, which would produce
	// a confusing $0 payment row.
	if len(resp.Schedule) != 0 {
		t.Errorf("schedule should be empty in derive-only mode, got %d rows", len(resp.Schedule))
	}
	if resp.TotalPaid != 0 || resp.TotalInt != 0 {
		t.Errorf("totals should be zero in derive-only mode, got paid=%v int=%v",
			resp.TotalPaid, resp.TotalInt)
	}
}

// API: derive-only mode with insufficient siblings — supplying only
// a loanDate (no firstDate, no lastDate, no nPeriods) should not
// silently succeed. The handler must surface a specific error.
func TestAmortizationCalcDeriveOnlyInsufficientInputs(t *testing.T) {
	body := `{
		"loanDate": "2024-01-01",
		"perYr": 12
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	var resp AmortizationResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp.Error, "insufficient") {
		t.Errorf("expected insufficient-inputs error, got error=%q", resp.Error)
	}
	if resp.NPeriods != 0 {
		t.Errorf("nPeriods should be 0 on insufficient-input path, got %d", resp.NPeriods)
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
	if !strings.Contains(resp.Error, "before the 1st Pmt Date") {
		t.Errorf("expected balloon-before-first-date error, got error=%q",
			resp.Error)
	}
}
