// Canary tests for dispatch_gaps.md S-3: the frontend silently drops
// half-filled Advanced Options rows (prepayment, balloon, adjustment)
// before submitting. These canaries exercise the API directly to bind
// the contract that the *backend* should validate and reject such
// rows, so the frontend can stop dropping them and instead surface
// the API's error to the user.
//
// EXPECTED TO FAIL TODAY. Today the API accepts a prepayment row
// with amount=0 (Go's zero value when the JSON field is missing) and
// silently treats it as a no-op series. After Phase 1 / Phase 2 of
// docs/dispatch_gaps.md (per-row validation with field-named errors),
// these canaries flip to green.
//
// See docs/test_plan.md §1 (Wave 1 canaries) C-3, C-4, C-5.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCanaryC3_PrepaymentMissingAmountSilentlyAccepted documents
// dispatch_gaps S-3: a prepayment row with a startDate but no amount
// silently becomes a 0-amount series and produces no error. The
// schedule comes back as if no prepayment were applied.
//
// Compare against the same loan with a real $100 prepayment: total
// interest should be measurably lower. If the canary loan's total
// interest matches a no-prepayment baseline, the row was silently
// dropped (or zeroed).
//
// Pairs with: dispatch_gaps S-3 first bullet (prepayments).
// Fix: handler should reject prepayment rows missing required
// fields with "Prepayment row 1: Amount is required."
func TestCanaryC3_PrepaymentMissingAmountSilentlyAccepted(t *testing.T) {
	// Note: "amount" is intentionally omitted from the prepayment row.
	body := `{
		"amount": 200000,
		"loanDate": "2025-01-01",
		"firstDate": "2025-02-01",
		"rate": 0.06,
		"nPeriods": 360,
		"perYr": 12,
		"payment": 1199.10,
		"prepayments": [
			{"startDate": "2025-03-01", "perYr": 12}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	var resp AmortizationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Error != "" {
		// Post-fix: error must name the prepayment row and the
		// missing field.
		if !strings.Contains(strings.ToLower(resp.Error), "prepayment") {
			t.Errorf("error %q does not mention the prepayment row", resp.Error)
		}
		return
	}

	// No error — the silent-drop bug is active. The row was either
	// dropped or treated as amount=0 (no-op). Confirm by checking
	// that no extra interest reduction happened.
	if len(resp.Schedule) == 0 {
		t.Fatalf("expected a schedule; got 0 rows")
	}
	t.Errorf("CANARY: prepayment row with missing Amount was silently accepted "+
		"(dispatch_gaps S-3). Schedule ran without error and produced %d rows; "+
		"after the fix the API should return an error naming Prepayment row 1.",
		len(resp.Schedule))
}

// TestCanaryC4_BalloonMissingDateSilentlyAccepted documents
// dispatch_gaps S-3: a balloon row with an amount but no date.
//
// Today the AmortBalloonReq.Date field is a string; a JSON null
// or omitted field becomes "". The handler at handlers.go:491
// attempts to time.Parse(""), gets an error, and reports "invalid
// balloon date" — which is mildly useful, but doesn't include the
// row index and doesn't distinguish "user typed garbage" from
// "user left the cell blank."
//
// The canary's failing assertion: error should explicitly name
// "Balloon row 1" so the UI can highlight that specific cell.
//
// Pairs with: dispatch_gaps S-3 second bullet (balloons).
// Fix: include row index in the error.
func TestCanaryC4_BalloonMissingDateSilentlyAccepted(t *testing.T) {
	body := `{
		"amount": 200000,
		"loanDate": "2025-01-01",
		"firstDate": "2025-02-01",
		"rate": 0.06,
		"nPeriods": 360,
		"perYr": 12,
		"payment": 1199.10,
		"balloons": [
			{"amount": 50000}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)

	// Today: 400 with "invalid balloon date" (no row index).
	// Post-fix: 400 with "Balloon row 1: Date is required" or
	// similar, and the message must include "row" or "1".
	var resp AmortizationResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error == "" {
		t.Fatalf("expected an error; got none. Body: %s", w.Body.String())
	}
	lower := strings.ToLower(resp.Error)
	if !strings.Contains(lower, "balloon") {
		t.Errorf("error %q does not mention 'balloon'", resp.Error)
	}
	if !strings.Contains(lower, "row") && !strings.Contains(lower, "1") {
		t.Errorf("CANARY: error %q does not include a row index. "+
			"dispatch_gaps S-3 / §4.7 AM-6 — user can't tell which row is bad.",
			resp.Error)
	}
	// Also: the error should name the *field* (Date), not just the
	// problem (invalid).
	if !strings.Contains(lower, "date") {
		t.Errorf("CANARY: error %q does not name the Date field", resp.Error)
	}
}

// TestCanaryC5_AdjustmentMissingRateAndAmountSilentlyAccepted
// documents dispatch_gaps S-3 (third bullet) and AO7: an adjustment
// row with a date but neither a new rate nor a new amount is
// effectively a no-op. DOS uses this as a "re-amortize at current
// rate" signal (AO7); the Go port has no AO7 path, so the row
// should be rejected explicitly until that's ported.
//
// Today: the handler accepts the row (both Rate and Amount pointers
// are nil), pushes it into Adjustments, and the engine applies a
// no-change adjustment — silent no-op.
//
// Pairs with: dispatch_gaps AO7, S-3 third bullet.
// Fix: reject with "Adjustment row 1: supply at least one of new
// Rate or new Pmt Amount" until AO7 is implemented.
func TestCanaryC5_AdjustmentMissingRateAndAmountSilentlyAccepted(t *testing.T) {
	body := `{
		"amount": 200000,
		"loanDate": "2025-01-01",
		"firstDate": "2025-02-01",
		"rate": 0.06,
		"nPeriods": 360,
		"perYr": 12,
		"payment": 1199.10,
		"adjustments": [
			{"date": "2030-01-01"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)

	var resp AmortizationResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "" {
		lower := strings.ToLower(resp.Error)
		if !strings.Contains(lower, "adjustment") {
			t.Errorf("error %q does not mention the adjustment row", resp.Error)
		}
		return
	}
	// No error → silent no-op bug active.
	t.Errorf("CANARY: adjustment row with no Rate and no Amount was silently " +
		"accepted (dispatch_gaps S-3 / AO7). After the fix, the API must return " +
		"an error explaining that AO7 (re-amortize at current rate) is not yet " +
		"supported and asking the user to supply either Rate or Amount.")
}
