// Test for dispatch_gaps ST1 / §4.3: advanced-option row errors now
// carry a structured FieldError so the UI can highlight the exact
// cell instead of regex-matching the message text.

package api

import (
	"encoding/json"
	"testing"
)

func TestFieldErrorOnIncompletePrepayment(t *testing.T) {
	// Prepayment row with a start date but no amount and no bound.
	resp, code := postJSON(t, HandleAmortizationCalc, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":360,"perYr":12,"payment":1199.10,
		"prepayments":[{"startDate":"2026-01-01","perYr":12}]
	}`)
	if code != 400 {
		t.Fatalf("expected 400, got %d", code)
	}
	detail, ok := resp["errorDetail"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured errorDetail, got %v", resp)
	}
	if detail["block"] != "prepayment" {
		t.Errorf("block = %v, want prepayment", detail["block"])
	}
	if detail["rowIdx"].(float64) != 1 {
		t.Errorf("rowIdx = %v, want 1", detail["rowIdx"])
	}
	if detail["code"] == "" || detail["code"] == nil {
		t.Errorf("expected a stable error code")
	}
}

func TestFieldErrorOnBadBalloonDate(t *testing.T) {
	resp, code := postJSON(t, HandleAmortizationCalc, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":360,"perYr":12,"payment":1199.10,
		"balloons":[{"date":"not-a-date","amount":1000}]
	}`)
	if code != 400 {
		t.Fatalf("expected 400, got %d", code)
	}
	b, _ := json.Marshal(resp["errorDetail"])
	var fe FieldError
	if err := json.Unmarshal(b, &fe); err != nil {
		t.Fatalf("errorDetail did not decode: %v", err)
	}
	if fe.Block != "balloon" || fe.RowIdx != 1 {
		t.Errorf("got block=%q rowIdx=%d, want balloon/1", fe.Block, fe.RowIdx)
	}
}
