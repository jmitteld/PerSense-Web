// Test for dispatch_gaps FP1 / A6: deriving the number of periods
// from a known payment amount (DOS DetermineLastPaymentDate).

package api

import "testing"

// TestSolveNPeriodsFromPayment: a $200k 6% loan with the standard
// 30-year payment but NO term and NO last date supplied. The engine
// must derive ~360 periods from the payment alone.
func TestSolveNPeriodsFromPayment(t *testing.T) {
	resp, code := amzCall(t, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"perYr":12,"payment":1199.10
	}`)
	if code != 200 || resp["error"] != nil {
		t.Fatalf("calc failed: code=%d err=%v", code, resp["error"])
	}
	n, ok := resp["nPeriods"].(float64)
	if !ok {
		t.Fatalf("no nPeriods in response: %v", resp)
	}
	// The exact 30-year payment is ~1199.10; allow a couple periods
	// of slack for payment rounding.
	if n < 358 || n > 362 {
		t.Errorf("derived nPeriods = %v, want ~360", n)
	}
}

// TestSolveNPeriodsFancyWithPrepayment verifies dispatch_gaps R4-7:
// the term can now be derived from a payment even with advanced
// options in use — here a prepayment series retires the loan early,
// so the solved term is well under the 360 a plain 30-year payment
// would imply.
func TestSolveNPeriodsFancyWithPrepayment(t *testing.T) {
	resp, code := amzCall(t, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"perYr":12,"payment":1199.10,
		"prepayments":[{"startDate":"2025-02-01","perYr":12,"amount":600}]
	}`)
	if code != 200 || resp["error"] != nil {
		t.Fatalf("fancy A6 calc failed: code=%d err=%v", code, resp["error"])
	}
	n, ok := resp["nPeriods"].(float64)
	if !ok || n <= 0 {
		t.Fatalf("no nPeriods derived: %v", resp["nPeriods"])
	}
	if n >= 360 {
		t.Errorf("derived term %v should be well under 360 (prepayment accelerates payoff)", n)
	}
}

// TestSolveNPeriodsPaymentTooSmall: a payment that does not cover the
// first period's interest must be rejected, not run forever.
func TestSolveNPeriodsPaymentTooSmall(t *testing.T) {
	resp, code := amzCall(t, `{
		"amount":200000,"rate":0.06,"loanDate":"2025-01-01",
		"perYr":12,"payment":500
	}`)
	if code == 200 && resp["error"] == nil {
		t.Fatalf("expected an error for a too-small payment, got %v", resp)
	}
}
