// Tests for dispatch_gaps G1: the DOS "Interest paid in" (in-advance)
// and "Interest on interest" (US Rule) computational settings are now
// threaded from the API request into the amortization engine. Before
// G1 the engine supported both but no handler ever set them.

package api

import (
	"fmt"
	"testing"
)

// amzCallMap posts an amortization request and returns the decoded
// response map plus the HTTP status.
func amzCall(t *testing.T, body string) (map[string]any, int) {
	t.Helper()
	return postJSON(t, HandleAmortizationCalc, body)
}

// TestInAdvanceChangesSchedule: an annuity-due (in-advance) loan
// accrues interest differently from an ordinary in-arrears loan, so
// the total interest must differ once the setting is honored.
func TestInAdvanceChangesSchedule(t *testing.T) {
	base := `{"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":360,"perYr":12,"payment":1199.10%s}`

	arrears, code := amzCall(t, fmt.Sprintf(base, ""))
	if code != 200 || arrears["error"] != nil {
		t.Fatalf("arrears calc failed: code=%d err=%v", code, arrears["error"])
	}
	advance, code := amzCall(t, fmt.Sprintf(base, `,"inAdvance":true`))
	if code != 200 || advance["error"] != nil {
		t.Fatalf("in-advance calc failed: code=%d err=%v", code, advance["error"])
	}

	ti1, _ := arrears["totalInterest"].(float64)
	ti2, _ := advance["totalInterest"].(float64)
	if ti1 == 0 || ti2 == 0 {
		t.Fatalf("expected non-zero total interest, got %v / %v", ti1, ti2)
	}
	if ti1 == ti2 {
		t.Errorf("inAdvance had no effect: totalInterest identical (%.2f). "+
			"Setting is not reaching the engine.", ti1)
	}
}

// TestInAdvanceAffectsFancySchedule verifies dispatch_gaps R4-2: the
// in-advance setting now also changes a fancy-mode schedule (one with
// advanced options), not only the basic schedule.
func TestInAdvanceAffectsFancySchedule(t *testing.T) {
	// A balloon makes this a fancy-mode run.
	base := `{"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":360,"perYr":12,"payment":1199.10,
		"balloons":[{"date":"2035-01-01","amount":20000}]%s}`

	arrears, code := amzCall(t, fmt.Sprintf(base, ""))
	if code != 200 || arrears["error"] != nil {
		t.Fatalf("fancy arrears calc failed: %v", arrears["error"])
	}
	advance, code := amzCall(t, fmt.Sprintf(base, `,"inAdvance":true`))
	if code != 200 || advance["error"] != nil {
		t.Fatalf("fancy in-advance calc failed: %v", advance["error"])
	}
	ti1, _ := arrears["totalInterest"].(float64)
	ti2, _ := advance["totalInterest"].(float64)
	if ti1 == 0 || ti2 == 0 || ti1 == ti2 {
		t.Errorf("inAdvance had no effect on the fancy schedule: %.2f vs %.2f", ti1, ti2)
	}
}

// TestUSARuleAccepted: the usaRule setting is accepted and produces a
// schedule. (Its numeric effect only shows under negative
// amortization; here we assert the request is honored end-to-end.)
func TestUSARuleAccepted(t *testing.T) {
	resp, code := amzCall(t, `{"amount":200000,"rate":0.06,
		"loanDate":"2025-01-01","nPeriods":360,"perYr":12,
		"payment":1199.10,"usaRule":true}`)
	if code != 200 || resp["error"] != nil {
		t.Fatalf("usaRule calc failed: code=%d err=%v", code, resp["error"])
	}
	if _, ok := resp["totalInterest"].(float64); !ok {
		t.Errorf("expected a schedule with totalInterest, got %v", resp)
	}
}
