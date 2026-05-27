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
	"fmt"
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

// TestAO7AdjustmentReamortizesAtCurrentRate verifies AO7
// ("re-amortize at current rate"): a date-only adjustment row — no
// new rate, no new payment — used to be rejected with a "supply at
// least one of new Rate or new Pmt Amount" error.  DOS uses such a
// row to ask the engine to re-solve the regular payment over the
// remaining term at the unchanged rate, which matters when an
// upcoming balloon (or drift left over from a prior adjustment)
// means the running payment no longer amortizes the loan cleanly.
//
// The engine now handles this via the same re-amortize branch that
// serves AO5 (rate-only adjustments).  When no rate is supplied,
// `f` and `truerate` keep their pre-adjustment values, so the solve
// uses the current rate.  This test pins the contract from two
// angles:
//
//  1. The API accepts a date-only adjustment and returns a schedule
//     (no AO7-rejection error).
//  2. On a loan with a balloon dated after the adjustment, the AO7
//     re-amortize meaningfully lowers the post-adjustment payment —
//     because the future balloon takes principal off what the
//     regular payment must retire.  Without AO7 (no adjustment row),
//     the running payment carries on unchanged.
//
// Pairs with dispatch_gaps AO7 / §0.9.5.
func TestAO7AdjustmentReamortizesAtCurrentRate(t *testing.T) {
	// Loan with a $100k balloon at year 10 of a 30-year, 6% schedule.
	// The base payment ($1,199.10) is sized to amortize $200k over 30
	// years — too high for what's needed once the balloon discount
	// kicks in.  An AO7 adjustment one year before the balloon should
	// re-solve the payment downward.
	base := `{
		"amount": 200000,
		"loanDate": "2025-01-01",
		"firstDate": "2025-02-01",
		"rate": 0.06,
		"nPeriods": 360,
		"perYr": 12,
		"balloons": [{"date": "2035-01-01", "amount": 100000}]
		%s
	}`

	// 1. Baseline: balloon present, no AO7 row. Schedule runs with
	// the original payment throughout.
	noAdj, code := amzCall(t, fmt.Sprintf(base, ""))
	if code != 200 || noAdj["error"] != nil {
		t.Fatalf("baseline (no adjustment) failed: code=%d err=%v",
			code, noAdj["error"])
	}
	schedNoAdj, _ := noAdj["schedule"].([]any)
	if len(schedNoAdj) == 0 {
		t.Fatalf("baseline schedule is empty")
	}
	basePmt, _ := schedNoAdj[0].(map[string]any)["payment"].(float64)

	// 2. AO7: a date-only adjustment one year before the balloon.
	// Used to be rejected; now must succeed.
	withAO7, code := amzCall(t, fmt.Sprintf(base, `,"adjustments":[{"date":"2034-01-01"}]`))
	if code != 200 || withAO7["error"] != nil {
		t.Fatalf("AO7 adjustment was rejected: code=%d err=%v "+
			"(date-only adjustment rows must be accepted as AO7 "+
			"re-amortize-at-current-rate)", code, withAO7["error"])
	}
	schedAO7, _ := withAO7["schedule"].([]any)
	if len(schedAO7) == 0 {
		t.Fatalf("AO7 schedule is empty")
	}

	// 3. Find the post-adjustment payment. Adjustment is dated
	// 2034-01-01; pick a payment a few months after that.
	var postPmt float64
	for _, row := range schedAO7 {
		r, _ := row.(map[string]any)
		date, _ := r["date"].(string)
		if date >= "2034-02-01" && date < "2034-12-01" {
			postPmt, _ = r["payment"].(float64)
			break
		}
	}
	if postPmt == 0 {
		t.Fatalf("could not locate a post-adjustment payment in the AO7 schedule")
	}
	if postPmt >= basePmt {
		t.Errorf("AO7 re-amortize had no effect: post-adjustment payment "+
			"%.2f did not drop below baseline payment %.2f. With a "+
			"$100k balloon dated after the adjustment, the future "+
			"balloon should discount the principal the regular "+
			"payment must retire, so the AO7 re-amortize should "+
			"lower the payment.", postPmt, basePmt)
	}
}
