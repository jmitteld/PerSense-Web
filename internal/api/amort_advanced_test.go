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

	// (1) AO6: a payment-only ARM adjustment is the mirror of AO5 —
	//     DOS EstimateAndRefineAdjRate solves the RATE that makes the
	//     new payment amortize the balance over the remaining term,
	//     keeping the loan on its original term. A $1,500 payment is
	//     well above the ~$1,199 a 6% loan needs, so the implied rate
	//     rises and total interest goes ABOVE the baseline.
	if resp.TotalInt <= baseline.TotalInt {
		t.Errorf("with the $1,500 payment adjustment the implied rate rises, "+
			"so total interest %.2f should exceed baseline %.2f",
			resp.TotalInt, baseline.TotalInt)
	}
	// The loan still ends on its original ~360-period term — an
	// adjustment re-amortizes, it does not change the term.
	if n := len(resp.Schedule); n < 358 || n > 362 {
		t.Errorf("schedule ran %d rows; an AO6 adjustment should keep "+
			"the ~360-period term", n)
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

// TestAPIAmortStubRowVsRegularPayment locks the schedule contract the
// frontend relies on to fill the "Pmt Amount" field: when the loan date
// precedes the first natural period, the engine leads the schedule with
// a settlement-stub row (PayNum 0) carrying ONLY the prepaid odd-days
// interest. The regular payment lives on the first PayNum>=1 row.
//
// Reproduces the reported bug where the Pmt Amount field showed $50
// (the 05/28->06/01 stub interest = 100000 * 6% * 3/360) instead of the
// real $599.55 payment. The frontend must read the first PayNum>=1 row,
// not schedule[0].
func TestAPIAmortStubRowVsRegularPayment(t *testing.T) {
	body := `{
		"amount": 100000,
		"loanDate": "2026-05-28",
		"firstDate": "2026-07-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"basis": "360"
	}`
	resp, code := amortCall(t, body)
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	if len(resp.Schedule) < 2 {
		t.Fatalf("expected schedule with a stub + regular rows, got %d rows", len(resp.Schedule))
	}

	// Row 0 is the settlement stub: PayNum 0, the payment is entirely
	// interest (so the displayed principal, payment-interest, is 0), and
	// it equals the prepaid odd-days interest ~ $50.
	stub := resp.Schedule[0]
	if stub.PayNum != 0 {
		t.Errorf("schedule[0].PayNum = %d, want 0 (settlement stub)", stub.PayNum)
	}
	if math.Abs(stub.Payment-50.0) > 0.01 {
		t.Errorf("stub payment = %.2f, want ~50.00 (odd-days interest)", stub.Payment)
	}
	if math.Abs(stub.Payment-stub.Interest) > 0.01 {
		t.Errorf("stub payment (%.2f) should be all interest (%.2f) — displayed principal must be 0",
			stub.Payment, stub.Interest)
	}

	// The first regular payment (PayNum>=1) is the real ~$599.55 amount
	// that belongs in the Pmt Amount field.
	var regular *PaymentLine
	for i := range resp.Schedule {
		if resp.Schedule[i].PayNum >= 1 {
			regular = &resp.Schedule[i]
			break
		}
	}
	if regular == nil {
		t.Fatalf("no regular (PayNum>=1) payment row found")
	}
	if math.Abs(regular.Payment-599.55) > 0.10 {
		t.Errorf("regular payment = %.2f, want ~599.55", regular.Payment)
	}
}

// TestAPIAmortFirstIntPrepaidNo verifies the "1st interest prepaid at
// settlement = NO" computational setting actually changes the schedule.
// With YES (default) the engine emits a settlement-stub row (PayNum 0)
// for the partial-period interest. With NO that stub must disappear and
// the partial-period interest is rolled into the first regular payment,
// so payment #1's interest exceeds a single full period's interest.
//
// Regression guard for the reported bug where toggling the setting to
// NO had no effect because the frontend never sent it and the handler
// hardcoded Prepaid=true.
func TestAPIAmortFirstIntPrepaidNo(t *testing.T) {
	yesBody := `{
		"amount": 100000,
		"loanDate": "2026-05-28",
		"firstDate": "2026-07-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"basis": "360"
	}`
	noBody := `{
		"amount": 100000,
		"loanDate": "2026-05-28",
		"firstDate": "2026-07-01",
		"rate": 0.06,
		"perYr": 12,
		"nPeriods": 360,
		"basis": "360",
		"firstIntPrepaid": false
	}`

	yes, code := amortCall(t, yesBody)
	if code != 200 || yes.Error != "" {
		t.Fatalf("yes: code=%d err=%s", code, yes.Error)
	}
	no, code := amortCall(t, noBody)
	if code != 200 || no.Error != "" {
		t.Fatalf("no: code=%d err=%s", code, no.Error)
	}

	// YES leads with a PayNum 0 settlement stub; NO does not.
	if len(yes.Schedule) == 0 || yes.Schedule[0].PayNum != 0 {
		t.Fatalf("prepaid=YES: expected a PayNum 0 stub row, got %+v", yes.Schedule[0])
	}
	if len(no.Schedule) == 0 || no.Schedule[0].PayNum != 1 {
		t.Errorf("prepaid=NO: expected no stub row (first PayNum == 1), got PayNum %d",
			no.Schedule[0].PayNum)
	}

	// With NO, the first regular payment's interest carries the whole
	// partial-period span (loan date -> first payment, ~33 days at
	// 30/360), so it exceeds one full month's interest of $500.
	firstNoInterest := no.Schedule[0].Interest
	if firstNoInterest <= 500.0 {
		t.Errorf("prepaid=NO: first payment interest = %.2f, want > 500.00 "+
			"(partial-period interest rolled in)", firstNoInterest)
	}

	// Under NO the partial period extends the first period beyond a full
	// month, so with the same number of payments the loan accrues a bit
	// more total interest than the prepaid (stub-at-closing) arrangement.
	// The difference should be modest (on the order of the rolled-in
	// partial-period interest), not wild.
	if no.TotalInt <= yes.TotalInt {
		t.Errorf("prepaid=NO TotalInt (%.2f) should exceed prepaid=YES (%.2f)",
			no.TotalInt, yes.TotalInt)
	}
	if d := no.TotalInt - yes.TotalInt; d > 1000.0 {
		t.Errorf("interest difference YES->NO = %.2f looks too large", d)
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
