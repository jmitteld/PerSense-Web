package api

import (
	"math"
	"strings"
	"testing"
)

// Regression tests for the fancy-aware blank-payment solve
// (docs/amortization_dos_fidelity_review.md items 3, 8, 9, 10): when the
// regular payment is left blank, the engine must solve a payment that
// amortizes the loan over the stated term WITH the fancy feature active —
// a known balloon should REDUCE the payment, and a principal-minimum
// (target) should be solved so the loan still retires exactly at the term.

func firstRegular(s []PaymentLine) *PaymentLine {
	for i := range s {
		if s[i].PayNum >= 1 {
			return &s[i]
		}
	}
	return nil
}

func lastRegular(s []PaymentLine) *PaymentLine {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i].PayNum >= 1 {
			return &s[i]
		}
	}
	return nil
}

func hasWarning(ws []string, substr string) bool {
	for _, w := range ws {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// A known balloon with a blank payment must lower the regular payment so
// principal + balloon amortize over the term — not keep the no-balloon
// payment and retire the loan early.
func TestAPIAmortBalloonBlankPaymentSolves(t *testing.T) {
	withBalloon := `{
		"amount": 100000, "loanDate": "2024-01-01", "firstDate": "2024-02-01",
		"rate": 0.06, "perYr": 12, "nPeriods": 120,
		"balloons": [{"date": "2027-02-01", "amount": 20000}]
	}`
	resp, code := amortCall(t, withBalloon)
	if code != 200 || resp.Error != "" {
		t.Fatalf("status=%d err=%q", code, resp.Error)
	}
	if hasWarning(resp.Warnings, "retired early") {
		t.Errorf("balloon+blank payment still retires early: %v", resp.Warnings)
	}
	if hasWarning(resp.Warnings, "does not amortize") {
		t.Errorf("balloon+blank payment does not amortize: %v", resp.Warnings)
	}
	last := lastRegular(resp.Schedule)
	if last == nil || last.PayNum != 120 {
		t.Fatalf("expected schedule to run to payment 120, got last regular %+v", last)
	}

	// The solved payment must be LOWER than the no-balloon baseline payment.
	noBalloon := `{
		"amount": 100000, "loanDate": "2024-01-01", "firstDate": "2024-02-01",
		"rate": 0.06, "perYr": 12, "nPeriods": 120
	}`
	base, _ := amortCall(t, noBalloon)
	withPmt := firstRegular(resp.Schedule).Payment
	basePmt := firstRegular(base.Schedule).Payment
	if !(withPmt < basePmt) {
		t.Errorf("balloon should reduce the payment: with-balloon %.2f, no-balloon %.2f",
			withPmt, basePmt)
	}
}

// A principal-minimum (target) with a blank payment must solve an eventual
// constant payment so the loan retires at exactly the term — the previous
// behavior used the no-target payment and paid the loan off early (e.g.
// 114/120). Early payments ramp down to the eventual constant.
func TestAPIAmortPrincipalMinimumBlankPaymentSolves(t *testing.T) {
	body := `{
		"amount": 100000, "loanDate": "2024-06-13", "firstDate": "2024-08-01",
		"rate": 0.06, "perYr": 12, "nPeriods": 120,
		"targetAmt": 800
	}`
	resp, code := amortCall(t, body)
	if code != 200 || resp.Error != "" {
		t.Fatalf("status=%d err=%q", code, resp.Error)
	}
	if hasWarning(resp.Warnings, "retired early") {
		t.Errorf("principal-minimum loan retires early instead of amortizing to term: %v",
			resp.Warnings)
	}
	last := lastRegular(resp.Schedule)
	if last == nil || last.PayNum != 120 {
		t.Fatalf("expected schedule to run to payment 120, got last regular %+v", last)
	}
	first := firstRegular(resp.Schedule)
	// Early payment is boosted to meet the principal minimum; the eventual
	// (last) payment is the lower steady-state constant.
	if !(first.Payment > last.Payment) {
		t.Errorf("expected payments to ramp down (first %.2f > last %.2f)",
			first.Payment, last.Payment)
	}
}

// Prepaid-OFF with an odd first period: the regular payment must be augmented
// so the loan still amortizes over the term (no terminating balloon), per DOS
// Amortize.pas:1513-1522. Item 12b. Loan dated 2/12 with first payment 4/1
// gives a ~1.6-period odd first period.
func TestAPIAmortPrepaidOffAugmentsPayment(t *testing.T) {
	off := `{
		"amount": 100000, "loanDate": "2024-02-12", "firstDate": "2024-04-01",
		"rate": 0.06, "perYr": 12, "nPeriods": 360, "firstIntPrepaid": false
	}`
	resp, code := amortCall(t, off)
	if code != 200 || resp.Error != "" {
		t.Fatalf("status=%d err=%q", code, resp.Error)
	}
	if hasWarning(resp.Warnings, "does not amortize") {
		t.Errorf("prepaid-OFF odd-first-period loan does not amortize (payment not augmented): %v",
			resp.Warnings)
	}
	last := lastRegular(resp.Schedule)
	if last == nil || last.PayNum != 360 {
		t.Fatalf("expected schedule to run to payment 360, got last %+v", last)
	}
	if math.Abs(last.Principal) > 1.0 {
		t.Errorf("final balance = %.2f, expected ~0 (loan should fully amortize)", last.Principal)
	}

	// The augmented prepaid-OFF payment should exceed the prepaid-ON payment
	// (where the odd interest is taken at settlement instead).
	on := `{
		"amount": 100000, "loanDate": "2024-02-12", "firstDate": "2024-04-01",
		"rate": 0.06, "perYr": 12, "nPeriods": 360, "firstIntPrepaid": true
	}`
	onResp, _ := amortCall(t, on)
	offPmt := firstRegular(resp.Schedule).Payment
	onPmt := firstRegular(onResp.Schedule).Payment
	if !(offPmt > onPmt) {
		t.Errorf("prepaid-OFF payment %.2f should exceed prepaid-ON payment %.2f (augmentation)",
			offPmt, onPmt)
	}
}

// Prepaid-ON with the DOS-default first payment date (which is now first-of-second
// month, creating an odd first period) must emit the settlement-stub row (PayNum
// 0) carrying the prepaid odd-days interest. Item 12a — the stub was previously
// absent because the wrong default produced no odd first period.
func TestAPIAmortPrepaidOnShowsSettlementStub(t *testing.T) {
	// firstDate omitted → defaults to 2024-04-01 (loan day 12 > 1), so 2/12→4/1
	// has odd days the prepaid stub should carry.
	body := `{
		"amount": 100000, "loanDate": "2024-02-12",
		"rate": 0.06, "perYr": 12, "nPeriods": 360, "firstIntPrepaid": true
	}`
	resp, code := amortCall(t, body)
	if code != 200 || resp.Error != "" {
		t.Fatalf("status=%d err=%q", code, resp.Error)
	}
	if len(resp.Schedule) == 0 {
		t.Fatal("empty schedule")
	}
	stub := resp.Schedule[0]
	if stub.PayNum != 0 {
		t.Fatalf("expected a settlement stub (PayNum 0) first, got PayNum %d", stub.PayNum)
	}
	if stub.Interest <= 0 {
		t.Errorf("settlement stub interest = %.2f, expected the prepaid odd-days interest > 0",
			stub.Interest)
	}
	if math.Abs(stub.Payment-stub.Interest) > 0.005 {
		t.Errorf("settlement stub should be interest-only: payment %.2f vs interest %.2f",
			stub.Payment, stub.Interest)
	}
}
