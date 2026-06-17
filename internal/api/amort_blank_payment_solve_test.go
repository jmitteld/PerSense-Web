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

// A long odd first period (6/13 → 8/1) with prepaid OFF makes the single first
// period's interest exceed the constant payment, bumping the balance up once
// before it amortizes normally. That one-period bump must NOT be flagged as
// negative amortization (client report).
func TestAPIAmortNoNegAmOnOddFirstPeriodBump(t *testing.T) {
	body := `{
		"amount": 100000, "loanDate": "2026-06-13", "firstDate": "2026-08-01",
		"rate": 0.12, "perYr": 12, "nPeriods": 120, "firstIntPrepaid": false
	}`
	resp, code := amortCall(t, body)
	if code != 200 || resp.Error != "" {
		t.Fatalf("status=%d err=%q", code, resp.Error)
	}
	if hasWarning(resp.Warnings, "negative amortization") {
		t.Errorf("odd-first-period bump must not be flagged as negative amortization: %v",
			resp.Warnings)
	}
	last := lastRegular(resp.Schedule)
	if last == nil || math.Abs(last.Principal) > 1.0 {
		t.Errorf("loan should still fully amortize; final balance = %+v", last)
	}
}

// The response echoes balloons the engine used: a known amount comes back
// not-solved; a date-only "target" balloon comes back Solved with the computed
// amount (so the UI can fill the blank cell — even when it solves to ~0).
func TestAPIAmortBalloonAmountEchoed(t *testing.T) {
	// Known balloon amount → echoed, not solved.
	r1, _ := amortCall(t, `{"amount":100000,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":0.06,"perYr":12,"nPeriods":360,"payment":1199.10,"balloons":[{"date":"2026-01-01","amount":50000}]}`)
	if len(r1.Balloons) != 1 || r1.Balloons[0].Solved || math.Abs(r1.Balloons[0].Amount-50000) > 0.01 {
		t.Errorf("known balloon echo = %+v, want one entry {50000, solved=false}", r1.Balloons)
	}
	// Date-only balloon on a self-amortizing loan → solved, ~0.
	r2, _ := amortCall(t, `{"amount":100000,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":0.06,"perYr":12,"nPeriods":120,"balloons":[{"date":"2027-02-01"}]}`)
	if len(r2.Balloons) != 1 || !r2.Balloons[0].Solved {
		t.Fatalf("date-only balloon echo = %+v, want one solved entry", r2.Balloons)
	}
	if math.Abs(r2.Balloons[0].Amount) > 1.0 {
		t.Errorf("self-amortizing target balloon = %.2f, want ~0", r2.Balloons[0].Amount)
	}
	// Low entered payment + date-only balloon at the last date → solved, non-zero
	// (the balloon is the remaining balance owed).
	r3, _ := amortCall(t, `{"amount":100000,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":0.06,"perYr":12,"nPeriods":120,"payment":800,"balloons":[{"date":"2034-01-01"}]}`)
	if len(r3.Balloons) != 1 || !r3.Balloons[0].Solved || r3.Balloons[0].Amount <= 0 {
		t.Errorf("low-payment target balloon = %+v, want one solved entry with amount>0", r3.Balloons)
	}
}

// An OFF-CYCLE balloon (one that falls between two regular payment dates, as
// happens when an odd first period shifts the payment dates off the balloon's
// monthly grid) must be applied at its OWN date — accruing partial interest up
// to it and letting the next regular period accrue only from the balloon date —
// not folded into the next regular payment. Verified against the real DOS engine:
//
//	amort_oracle 200000 0.09 10 1 b48=80938.47 first=4  → payment 21036.9256
//
// Loan 2024-01-01, first payment 2024-05-01 (4-month odd first period), annual,
// balloon 2028-01-01 (month 48, between payment 4 @ 2027-05-01 and payment 5 @
// 2028-05-01). Before the fix the Go port folded the balloon into the 2028-05-01
// payment and solved a ~20% different regular payment.
func TestAPIAmortOffCycleBalloonMatchesDOS(t *testing.T) {
	body := `{
		"amount": 200000, "loanDate": "2024-01-01", "firstDate": "2024-05-01",
		"rate": 0.09, "perYr": 1, "nPeriods": 10,
		"balloons": [{"date": "2028-01-01", "amount": 80938.47}]
	}`
	resp, code := amortCall(t, body)
	if code != 200 || resp.Error != "" {
		t.Fatalf("status=%d err=%q", code, resp.Error)
	}
	if hasWarning(resp.Warnings, "retired early") || hasWarning(resp.Warnings, "does not amortize") {
		t.Errorf("off-cycle balloon loan should amortize cleanly: %v", resp.Warnings)
	}
	// DOS solves the regular payment to 21036.93.
	reg := firstRegular(resp.Schedule).Payment
	if math.Abs(reg-21036.93) > 0.10 {
		t.Errorf("regular payment = %.2f, DOS engine says 21036.93", reg)
	}
	// The balloon must appear as its own row dated 2028-01-01 (not 2028-05-01).
	var balloonRow *PaymentLine
	for i := range resp.Schedule {
		if math.Abs(resp.Schedule[i].Payment-80938.47) < 1.0 {
			balloonRow = &resp.Schedule[i]
			break
		}
	}
	if balloonRow == nil {
		t.Fatalf("no balloon row (~80938.47) found in schedule")
	}
	if !strings.HasPrefix(balloonRow.Date, "2028-01-01") {
		t.Errorf("balloon row dated %q, expected its actual date 2028-01-01", balloonRow.Date)
	}
	// Loan retires at exactly payment 10.
	last := lastRegular(resp.Schedule)
	if last == nil || last.PayNum != 10 {
		t.Errorf("expected retirement at payment 10, got %+v", last)
	}
}

// A payment far below the interest due grows the balance every period — genuine
// sustained negative amortization — and must still surface the Note.
func TestAPIAmortNegAmStillFlagged(t *testing.T) {
	body := `{
		"amount": 100000, "loanDate": "2024-01-01", "firstDate": "2024-02-01",
		"rate": 0.12, "perYr": 12, "nPeriods": 120, "payment": 500
	}`
	resp, code := amortCall(t, body)
	if code != 200 || resp.Error != "" {
		t.Fatalf("status=%d err=%q", code, resp.Error)
	}
	if !hasWarning(resp.Warnings, "negative amortization") {
		t.Errorf("sustained negative amortization should be flagged: warnings=%v", resp.Warnings)
	}
}
