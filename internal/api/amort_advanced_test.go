package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func amortCall(t *testing.T, body string) (AmortizationResponse, int) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	HandleAmortizationCalc(w, req)
	var resp AmortizationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp, w.Code
}

func TestAPIAmortAdvancedPrepayment(t *testing.T) {
	body := `{
		"amount": 200000,
		"loanDate": "2024-01-01",
		"firstDate": "2024-02-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"payment": 1199.10,
		"prepayments": [{
			"startDate": "2024-02-01",
			"stopDate":  "2029-02-01",
			"perYr": 12,
			"amount": 100
		}]
	}`
	resp, code := amortCall(t, body)
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	if len(resp.Schedule) == 0 {
		t.Error("expected non-empty schedule")
	}

	// Compare with no-prepay baseline: total interest should be lower.
	baselineBody := `{
		"amount": 200000,
		"loanDate": "2024-01-01",
		"firstDate": "2024-02-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"payment": 1199.10
	}`
	baseline, _ := amortCall(t, baselineBody)
	if resp.TotalInt >= baseline.TotalInt {
		t.Errorf("with prepayment TotalInt = %.2f should be less than baseline %.2f",
			resp.TotalInt, baseline.TotalInt)
	}
}

func TestAPIAmortAdvancedBalloon(t *testing.T) {
	body := `{
		"amount": 200000,
		"loanDate": "2024-01-01",
		"firstDate": "2024-02-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"payment": 1199.10,
		"balloons": [{"date": "2026-01-01", "amount": 50000}]
	}`
	resp, code := amortCall(t, body)
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	// Find balloon payment line
	found := false
	for _, line := range resp.Schedule {
		if line.Date == "2026-01-01" && line.Payment > 40000 {
			found = true
		}
	}
	if !found {
		t.Error("balloon payment not reflected in schedule")
	}
}

func TestAPIAmortAdvancedAllOptions(t *testing.T) {
	// Smoke test: all advanced options at once. We don't combine
	// skipMonths with targetAmt here because target forces a minimum
	// payment (matching DOS behavior), which overrides skip-month
	// zeroing. Each option is asserted individually elsewhere.
	body := `{
		"amount": 200000,
		"loanDate": "2024-01-01",
		"firstDate": "2024-02-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"payment": 1199.10,
		"prepayments": [{"startDate":"2024-02-01","stopDate":"2026-02-01","perYr":12,"amount":50}],
		"balloons":    [{"date":"2030-01-01","amount":10000}],
		"adjustments": [{"date":"2027-01-01","rate":0.05}],
		"moratorium":  "2025-01-01",
		"skipMonths":  "12"
	}`
	resp, code := amortCall(t, body)
	if code != 200 {
		t.Fatalf("status = %d (body: %s)", code, resp.Error)
	}
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	if len(resp.Schedule) == 0 {
		t.Error("expected non-empty schedule")
	}
	// Skip-month December: regular payment is zero, but prepayments
	// still apply (DOS-faithful behavior — skip-months suppress only
	// the regular payment, not extras). With $50/mo prepay active,
	// December payments should equal the prepay amount.
	for _, line := range resp.Schedule {
		if line.Date == "2025-12-01" {
			if line.Payment > 60 {
				t.Errorf("december payment %s = %.2f, expected ~50 (prepay only)",
					line.Date, line.Payment)
			}
			break
		}
	}
	// After the prepay series ends (2026-02-01) and prior to balloon,
	// December lines should have payment = 0.
	for _, line := range resp.Schedule {
		if line.Date == "2027-12-01" {
			if line.Payment > 0.01 {
				t.Errorf("december 2027 payment = %.2f, expected 0",
					line.Payment)
			}
			break
		}
	}
}

func TestAPIAmortAdvancedTargetOverridesSkipMonth(t *testing.T) {
	// Documents the DOS-faithful interaction: when targetAmt is set
	// alongside skipMonths, the target's minimum-principal-reduction
	// requirement raises the skip-month payment to (interest + target).
	// This is intentional, not a bug.
	body := `{
		"amount": 200000,
		"loanDate": "2024-01-01",
		"firstDate": "2024-02-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"payment": 1199.10,
		"targetAmt": 100,
		"skipMonths": "12"
	}`
	resp, code := amortCall(t, body)
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	for _, line := range resp.Schedule {
		if line.Date == "2024-12-01" {
			// Should be ~ interest + 100, not 0.
			if line.Payment < 50 {
				t.Errorf("targetAmt should override skipMonth: got %.2f, expected interest + ~100",
					line.Payment)
			}
			return
		}
	}
}

func TestAPIAmortAdvancedBadDate(t *testing.T) {
	body := `{
		"amount": 200000,
		"loanDate": "2024-01-01",
		"firstDate": "2024-02-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"payment": 1199.10,
		"balloons": [{"date":"not-a-date","amount":50000}]
	}`
	_, code := amortCall(t, body)
	if code != 400 {
		t.Errorf("expected 400 for bad balloon date, got %d", code)
	}
}
