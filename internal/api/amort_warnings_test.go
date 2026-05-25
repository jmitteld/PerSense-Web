// Test for dispatch_gaps ST2: the amortization response now carries a
// Warnings channel (PV already had one). An early-payoff schedule
// surfaces a non-fatal advisory.
package api

import (
	"strings"
	"testing"
)

// TestImpliedTerminatingBalloonWarning verifies dispatch_gaps R4-8:
// an over-specified loan (payment too small for the stated term)
// surfaces an advisory that the final payment carries an implied
// terminating balloon (DOS TackOnFinalBalloon).
func TestImpliedTerminatingBalloonWarning(t *testing.T) {
	// 30-year payment on a 5-year (60-period) term — a large residual
	// remains, absorbed by the final payment.
	resp, code := amzCall(t, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":60,"perYr":12,"payment":1199.10
	}`)
	if code != 200 || resp["error"] != nil {
		t.Fatalf("calc failed: code=%d err=%v", code, resp["error"])
	}
	warns, _ := resp["warnings"].([]any)
	found := false
	for _, w := range warns {
		if s, _ := w.(string); strings.Contains(s, "terminating balloon") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an implied-terminating-balloon warning, got %v", warns)
	}
}

func TestAmortEarlyPayoffWarning(t *testing.T) {
	// A 30-year loan with a large monthly prepayment retires well
	// before its scheduled 360-payment term.
	resp, code := amzCall(t, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"nPeriods":360,"perYr":12,"payment":1199.10,
		"prepayments":[{"startDate":"2025-02-01","perYr":12,"amount":1500}]
	}`)
	if code != 200 || resp["error"] != nil {
		t.Fatalf("calc failed: code=%d err=%v", code, resp["error"])
	}
	warns, ok := resp["warnings"].([]any)
	if !ok || len(warns) == 0 {
		t.Fatalf("expected an early-payoff warning, got %v", resp["warnings"])
	}
	found := false
	for _, w := range warns {
		if s, _ := w.(string); strings.Contains(s, "retired early") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a 'retired early' warning, got %v", warns)
	}
}
