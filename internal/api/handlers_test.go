package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMortgageCalcBasic(t *testing.T) {
	body := `{
		"price": 200000,
		"pctDown": 0.20,
		"years": 30,
		"rate": 0.06,
		"tax": 0
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/calc", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	HandleMortgageCalc(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp MortgageResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Monthly < 900 || resp.Monthly > 1100 {
		t.Errorf("monthly = %f, expected ~959", resp.Monthly)
	}
	if resp.Financed < 159000 || resp.Financed > 161000 {
		t.Errorf("financed = %f, expected ~160000", resp.Financed)
	}
	if resp.APR <= 0 {
		t.Error("APR should be computed")
	}
}

func TestMortgageCalcMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/mortgage/calc", nil)
	w := httptest.NewRecorder()
	HandleMortgageCalc(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestMortgageCalcBadJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/calc", bytes.NewBufferString("{bad"))
	w := httptest.NewRecorder()
	HandleMortgageCalc(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAmortizationCalcBasic(t *testing.T) {
	body := `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.06,
		"firstDate": "2024-02-01",
		"nPeriods": 360,
		"perYr": 12,
		"payment": 599.55
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc", bytes.NewBufferString(body))
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
	if len(resp.Schedule) != 360 {
		t.Errorf("schedule has %d periods, want 360", len(resp.Schedule))
	}
	if resp.TotalPaid < 100000 {
		t.Error("total paid should exceed loan amount")
	}
	if resp.TotalInt < 50000 {
		t.Errorf("total interest = %f, expected > 50000", resp.TotalInt)
	}
}

func TestAmortizationCalcBadDate(t *testing.T) {
	body := `{"amount": 100000, "loanDate": "bad", "rate": 0.06, "firstDate": "2024-02-01", "nPeriods": 12, "perYr": 12}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPVCalcLumpSums(t *testing.T) {
	body := `{
		"asOfDate": "2024-01-01",
		"rate": 0.06,
		"lumpSums": [
			{"date": "2025-01-01", "amount": 10000},
			{"date": "2026-01-01", "amount": 10000}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	HandlePVCalc(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp PVResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.SumValue >= 20000 || resp.SumValue < 15000 {
		t.Errorf("sumValue = %f, expected ~18000-19500", resp.SumValue)
	}
	if len(resp.LumpSums) != 2 {
		t.Errorf("expected 2 lump sum results, got %d", len(resp.LumpSums))
	}
}

func TestPVCalcPeriodic(t *testing.T) {
	body := `{
		"asOfDate": "2023-12-01",
		"rate": 0.06,
		"periodics": [
			{"fromDate": "2024-01-01", "toDate": "2053-12-01", "perYr": 12, "amount": 1000}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	HandlePVCalc(w, req)

	var resp PVResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Periodics) != 1 {
		t.Fatal("expected 1 periodic result")
	}
	// $1000/mo for 30yr at 6%: PV ≈ $166,791
	if resp.Periodics[0].Value < 100000 || resp.Periodics[0].Value > 250000 {
		t.Errorf("periodic PV = %f, expected ~167000", resp.Periodics[0].Value)
	}
}
