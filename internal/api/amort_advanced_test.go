package api

import (
	"bytes"
	"encoding/json"
	"math"
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

// Help/Amortization Example 5 (interest-only $100k at 8% for 5
// years with a $100k balloon at term-end) computes total paid =
// $140,000.20 under the DOS-faithful default: balloon ADDS to the
// regular payment. Pin that default explicitly at the API layer.
//
// Background: the engine's PlusRegular setting was historically
// zero-valued by Go convention (false → balloon REPLACES regular),
// which contradicted the DOS default and broke EX5. The fix added
// a `balloonIncludesRegular` request field defaulting to false (in
// DOS terminology that's "balloon does NOT include regular pmt =
// NO" = balloon ADDS). This test prevents the default from
// silently flipping again.
func TestAPIAmortBalloonIncludesRegular_Default(t *testing.T) {
	body := `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.08,
		"firstDate": "2024-02-01",
		"nPeriods": 60,
		"perYr": 12,
		"payment": 666.67,
		"basis": "360",
		"balloons": [
			{"date": "2029-01-01", "amount": 100000}
		]
	}`
	resp, code := amortCall(t, body)
	if code != 200 {
		t.Fatalf("status=%d", code)
	}
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	if len(resp.Schedule) != 60 {
		t.Fatalf("schedule rows = %d, want 60", len(resp.Schedule))
	}
	last := resp.Schedule[len(resp.Schedule)-1]
	// Default = ADD: last period pays regular ($666.67) + balloon
	// ($100,000) = $100,666.67. Running balance lands near $0.
	if math.Abs(last.Payment-100666.67) > 0.10 {
		t.Errorf("final payment = %.2f, want 100,666.67 (regular + balloon under ADD default)",
			last.Payment)
	}
	if math.Abs(last.Principal) > 1.0 {
		t.Errorf("final running balance = %.4f, want ~0 (loan cleared under ADD default)",
			last.Principal)
	}
	if math.Abs(resp.TotalPaid-140000.20) > 0.50 {
		t.Errorf("total paid = %.2f, want ~140,000.20 (60 × $666.67 + $100k under ADD default)",
			resp.TotalPaid)
	}
}

// Override: setting balloonIncludesRegular=true switches the engine
// to REPLACE semantics (DOS YES setting). At the balloon date, the
// regular payment is omitted and only the balloon amount is paid.
// Total paid is exactly one regular payment less than the ADD case.
func TestAPIAmortBalloonIncludesRegular_Override(t *testing.T) {
	body := `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.08,
		"firstDate": "2024-02-01",
		"nPeriods": 60,
		"perYr": 12,
		"payment": 666.67,
		"basis": "360",
		"balloons": [
			{"date": "2029-01-01", "amount": 100000}
		],
		"balloonIncludesRegular": true
	}`
	resp, code := amortCall(t, body)
	if code != 200 {
		t.Fatalf("status=%d", code)
	}
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	last := resp.Schedule[len(resp.Schedule)-1]
	// REPLACE: last period payment = balloon amount = $100,000.
	if math.Abs(last.Payment-100000.0) > 0.10 {
		t.Errorf("final payment = %.2f, want 100,000.00 (balloon replaces regular under REPLACE)",
			last.Payment)
	}
	// Total paid: 59 regular periods × $666.67 + $100,000 = $139,333.53.
	if math.Abs(resp.TotalPaid-139333.53) > 0.50 {
		t.Errorf("total paid = %.2f, want ~139,333.53 (59 × $666.67 + $100k under REPLACE)",
			resp.TotalPaid)
	}
	// Sanity: the *exact* difference between the two scenarios must
	// be one regular payment. Verify directly so the test fails
	// loudly if engine semantics drift.
	defaultBody := `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.08,
		"firstDate": "2024-02-01",
		"nPeriods": 60,
		"perYr": 12,
		"payment": 666.67,
		"basis": "360",
		"balloons": [
			{"date": "2029-01-01", "amount": 100000}
		]
	}`
	defResp, _ := amortCall(t, defaultBody)
	diff := defResp.TotalPaid - resp.TotalPaid
	if math.Abs(diff-666.67) > 0.10 {
		t.Errorf("ADD-vs-REPLACE total-paid diff = %.4f, want exactly one regular payment (666.67)",
			diff)
	}
}

// Payment adjustment (ARM-style payment recast): a 30-year monthly
// loan whose regular payment is recast at year 5 to a new amount.
// The engine supports this via Adjustments[i].Amount paired with
// AmtOK=true; the API exposes it as the `amount` field on an
// adjustment row. This path had zero coverage prior to this test —
// the rate-adjustment cousin (LoanRate field) is well-covered by
// TestAdvancedRateAdjustment, but the amount-only branch was
// ride-along.
//
// Setup: $200k @ 6% for 30 years monthly, payment $1,199.10. At
// 2029-01-15 (between the Jan and Feb 2029 payments), recast the
// monthly payment to $1,500 (borrower over-pays). The new payment
// kicks in starting with the Feb 2029 payment.
//
// Engine semantics worth pinning: the adjustment-check runs *after*
// the current period is processed, so an adjustment with date D
// fires for the first payment whose period (prevDate, currentDate]
// contains D. Scheduling D mid-period — e.g. 2029-01-15 between
// payments on Jan 1 and Feb 1 — means the Jan payment uses the old
// amount and the Feb payment is the first to use the new amount.
// (If the user instead schedules the adjustment exactly on a
// payment date, the new amount won't appear until the *next*
// payment — caveat documented for future test writers.)
func TestAPIAmortAdvancedPaymentAdjustment(t *testing.T) {
	baselineBody := `{
		"amount": 200000,
		"loanDate": "2024-01-01",
		"firstDate": "2024-02-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"payment": 1199.10
	}`
	baseline, code := amortCall(t, baselineBody)
	if code != 200 || baseline.Error != "" {
		t.Fatalf("baseline failed: code=%d err=%s", code, baseline.Error)
	}

	body := `{
		"amount": 200000,
		"loanDate": "2024-01-01",
		"firstDate": "2024-02-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"payment": 1199.10,
		"adjustments": [
			{"date": "2029-01-15", "amount": 1500}
		]
	}`
	resp, code := amortCall(t, body)
	if code != 200 {
		t.Fatalf("status=%d", code)
	}
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}

	// (1) Over-paying should reduce total interest.
	if resp.TotalInt >= baseline.TotalInt {
		t.Errorf("with $1,500 recast, total interest %.2f should be < baseline %.2f",
			resp.TotalInt, baseline.TotalInt)
	}

	// (2) Locate the Jan 1 2029 row (last row before the adjustment)
	//     and the Feb 1 2029 row (first row after the adjustment).
	var janRow, febRow *PaymentLine
	for i, line := range resp.Schedule {
		switch line.Date {
		case "2029-01-01":
			janRow = &resp.Schedule[i]
		case "2029-02-01":
			febRow = &resp.Schedule[i]
		}
	}
	if janRow == nil {
		t.Fatal("could not find 2029-01-01 row in schedule")
	}
	if febRow == nil {
		t.Fatal("could not find 2029-02-01 row in schedule")
	}

	// (3) Pre-adjustment row still has the old payment.
	if math.Abs(janRow.Payment-1199.10) > 0.10 {
		t.Errorf("Jan 1 2029 payment = %.2f, want 1,199.10 (pre-adjustment)",
			janRow.Payment)
	}
	// (4) Post-adjustment row has the new payment.
	if math.Abs(febRow.Payment-1500.0) > 0.10 {
		t.Errorf("Feb 1 2029 payment = %.2f, want 1,500.00 (post-adjustment)",
			febRow.Payment)
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
