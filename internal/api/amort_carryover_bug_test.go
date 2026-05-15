package api

// Reproduces a real user-reported bug: on the Amortization screen,
// after running Help Example 4's Monthly variant first, the computed
// Payment ($733.76) is left in the cell with cell-output styling.
// Switching to the Biweekly variant by editing Pmts/Yr and Periods —
// but leaving Payment alone — sends a stale monthly payment with a
// biweekly schedule to the API. The engine honors the supplied
// payment, over-pays principal each period, and the schedule rows
// show negative interest and negative balance.
//
// This is a frontend stale-state problem, not an engine bug — the
// engine is doing exactly what it was told. The fix belongs in the
// FE (clear or invalidate the Payment cell when Pmts/Yr or # Periods
// changes after a calc). This test documents the engine behavior so
// we have a baseline to point at when the FE fix lands.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAmortStaleMonthlyPaymentAppliedToBiweekly(t *testing.T) {
	// Step 1: do the monthly run, leave Payment blank → engine
	// computes $733.76. The FE puts this in the Payment cell with
	// cell-output styling.
	monthly := postAmort(t, `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.08,
		"nPeriods": 360,
		"perYr": 12,
		"basis": "360"
	}`)
	if monthly.Error != "" {
		t.Fatalf("monthly setup: %s", monthly.Error)
	}
	monthlyPmt := monthly.Schedule[0].Payment
	t.Logf("monthly computed pmt = %.2f (expected 733.76)", monthlyPmt)

	// Step 2: user changes Pmts/Yr to 26 and Periods to 780. They
	// leave Payment alone. The FE re-submits with the stale value.
	resp := postAmort(t, `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.08,
		"nPeriods": 780,
		"perYr": 26,
		"payment": 733.76,
		"basis": "360"
	}`)
	if resp.Error != "" {
		t.Fatalf("stale-payment scenario errored: %s", resp.Error)
	}

	// Count negative-interest and negative-balance rows.
	negInt, negBal := 0, 0
	for _, r := range resp.Schedule {
		if r.Interest < 0 {
			negInt++
		}
		if r.Principal < 0 { // PaymentLine.Principal is the running balance after payment
			negBal++
		}
	}

	// Find the row where the balance first goes negative.
	flipAt := -1
	for i, r := range resp.Schedule {
		if r.Principal < 0 {
			flipAt = i
			break
		}
	}
	t.Logf("schedule rows = %d, total paid = %.2f, total interest = %.2f",
		len(resp.Schedule), resp.TotalPaid, resp.TotalInt)
	t.Logf("rows with negative interest = %d, with negative balance = %d, first negative at row index %d",
		negInt, negBal, flipAt)
	if flipAt >= 0 {
		r := resp.Schedule[flipAt]
		t.Logf("first-flip row: payNum=%d date=%s pmt=%.4f int=%.4f bal=%.4f",
			r.PayNum, r.Date, r.Payment, r.Interest, r.Principal)
	}

	// Assert the bug condition: with a $733.76 biweekly payment on
	// a $100k 8% loan, the schedule should over-amortize and the
	// balance should turn negative well before period 780.
	if negBal == 0 {
		t.Errorf("expected negative balance rows from stale-payment scenario, got 0")
	}
	if negInt == 0 {
		t.Errorf("expected negative interest rows from stale-payment scenario, got 0")
	}

	// Sanity: confirm that with payment blank the engine produces a
	// well-formed biweekly schedule (no negatives).
	good := postAmort(t, `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.08,
		"nPeriods": 780,
		"perYr": 26,
		"basis": "360"
	}`)
	if good.Error != "" {
		t.Fatalf("correct biweekly setup: %s", good.Error)
	}
	t.Logf("correct biweekly: pmt = %.2f, total paid = %.2f, total interest = %.2f",
		good.Schedule[0].Payment, good.TotalPaid, good.TotalInt)
	for _, r := range good.Schedule {
		if r.Interest < 0 {
			t.Errorf("correct biweekly schedule shouldn't have negative interest, got %.4f at payNum=%d",
				r.Interest, r.PayNum)
			break
		}
	}
}

func postAmort(t *testing.T, body string) AmortizationResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp AmortizationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}
