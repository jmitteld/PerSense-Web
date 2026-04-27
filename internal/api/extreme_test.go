package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMortgageCalcZeroRate(t *testing.T) {
	body := `{"price":120000,"pctDown":0,"years":10,"rate":0,"tax":0}`
	w := httptest.NewRecorder()
	HandleMortgageCalc(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	var resp MortgageResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	// At 0%, monthly = 120000/120 = 1000
	if resp.Monthly < 999 || resp.Monthly > 1001 {
		t.Errorf("monthly at 0%% = %f, want ~1000", resp.Monthly)
	}
}

func TestMortgageCalcLargeValues(t *testing.T) {
	body := `{"price":50000000,"pctDown":0.30,"years":30,"rate":0.045,"tax":2000}`
	w := httptest.NewRecorder()
	HandleMortgageCalc(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	var resp MortgageResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	if resp.Monthly < 100000 {
		t.Errorf("monthly on $50M = %f, expected > $100K", resp.Monthly)
	}
}

func TestMortgageCalcMissingFields(t *testing.T) {
	// Only price — not enough to compute anything, but should not crash
	body := `{"price":200000}`
	w := httptest.NewRecorder()
	HandleMortgageCalc(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	var resp MortgageResponse
	json.NewDecoder(w.Body).Decode(&resp)
	// Should return something without panicking
	if w.Code != 200 {
		t.Errorf("status = %d", w.Code)
	}
}

func TestMortgageCalcEmptyBody(t *testing.T) {
	body := `{}`
	w := httptest.NewRecorder()
	HandleMortgageCalc(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	if w.Code != 200 {
		t.Errorf("empty body should not crash, got status %d", w.Code)
	}
}

func TestAmortizationCalcZeroPayment(t *testing.T) {
	// Zero payment field — should use estimated payment
	body := `{"amount":100000,"loanDate":"2024-01-01","rate":0.06,"firstDate":"2024-02-01","nPeriods":360,"perYr":12}`
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	var resp AmortizationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	// Should not crash; might error gracefully
	if w.Code != 200 {
		t.Errorf("status = %d", w.Code)
	}
}

func TestAmortizationCalcWeekly(t *testing.T) {
	body := `{"amount":10000,"loanDate":"2024-01-01","rate":0.06,"firstDate":"2024-01-08","nPeriods":52,"perYr":52,"payment":200}`
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	var resp AmortizationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "" {
		t.Fatalf("weekly amortization error: %s", resp.Error)
	}
	if len(resp.Schedule) != 52 {
		t.Errorf("weekly schedule length = %d, want 52", len(resp.Schedule))
	}
}

func TestPVCalcNoPayments(t *testing.T) {
	body := `{"asOfDate":"2024-01-01","rate":0.06}`
	w := httptest.NewRecorder()
	HandlePVCalc(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	var resp PVResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	if resp.SumValue != 0 {
		t.Errorf("PV with no payments = %f, want 0", resp.SumValue)
	}
}

func TestPVCalcBadDate(t *testing.T) {
	body := `{"asOfDate":"not-a-date","rate":0.06}`
	w := httptest.NewRecorder()
	HandlePVCalc(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	if w.Code != 400 {
		t.Errorf("bad date should return 400, got %d", w.Code)
	}
}

func TestPVCalcManyLumpSums(t *testing.T) {
	// 50 lump sums — stress test
	var ls []map[string]any
	for i := 1; i <= 50; i++ {
		ls = append(ls, map[string]any{"date": "2025-01-01", "amount": 1000})
	}
	body := map[string]any{"asOfDate": "2024-01-01", "rate": 0.06, "lumpSums": ls}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	HandlePVCalc(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(b)))
	var resp PVResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	if len(resp.LumpSums) != 50 {
		t.Errorf("got %d results, want 50", len(resp.LumpSums))
	}
}
