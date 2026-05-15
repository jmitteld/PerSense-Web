package api

// Pins what the engine actually does with the PV `rate` request field:
// it always interprets the supplied number as a continuously-
// compounded TRUE rate, regardless of any label the FE might display
// (the Rate Type dropdown — True Rate / Loan Rate / Yield — is
// purely cosmetic until rate-form conversion is plumbed end-to-end).
//
// These three results document the magnitude of the discrepancy for
// Help PV Example 4 (the canonical "annuity with COLA" scenario,
// $2,000/mo for 30 years, 3% COLA) under each interpretation. If
// the FE is later changed to convert Loan Rate / Yield to True Rate
// before submission, the engine output for the converted rate will
// match Interpretation B or C exactly.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPVRateInterpretationsForExample4(t *testing.T) {
	post := func(rate float64) PVResponse {
		body := fmt.Sprintf(`{
			"asOfDate":"2024-01-01",
			"rate":%v,
			"periodics":[{"fromDate":"2024-01-01","toDate":"2054-01-01","perYr":12,"amount":2000,"cola":0.03}]
		}`, rate)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		HandlePVCalc(w, req)
		var resp PVResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)
		return resp
	}

	// (A) Current FE behaviour: dropdown is cosmetic, so 5% typed
	//     in the Rate% field is sent as-is and interpreted as the
	//     continuous true rate. This is the value rendered in the
	//     UI today.
	a := post(0.05)
	// (B) If 5% were meant as a (monthly-compounded) Loan Rate, the
	//     equivalent continuous true rate is 12·ln(1 + 0.05/12).
	bRate := 12 * math.Log(1+0.05/12)
	b := post(bRate)
	// (C) If 5% were meant as an effective annual Yield, the
	//     equivalent continuous true rate is ln(1 + 0.05).
	cRate := math.Log(1.05)
	c := post(cRate)

	t.Logf("(A) 5%% sent as TrueRate (current cosmetic dropdown): SumValue=%.2f", a.SumValue)
	t.Logf("(B) 5%% LoanRate → TrueRate=%.6f:                     SumValue=%.2f  (delta vs A: %+.2f)",
		bRate, b.SumValue, b.SumValue-a.SumValue)
	t.Logf("(C) 5%% Yield    → TrueRate=%.6f:                     SumValue=%.2f  (delta vs A: %+.2f)",
		cRate, c.SumValue, c.SumValue-a.SumValue)

	// Pin the current (cosmetic dropdown) behaviour numerically.
	// If this test ever fails, the engine's treatment of `rate`
	// has changed — either intentionally (conversion plumbing was
	// added; update the test) or unintentionally (find out why).
	const wantA = 532551.46
	if math.Abs(a.SumValue-wantA) > 0.01 {
		t.Errorf("(A) cosmetic-dropdown SumValue = %.2f, want %.2f", a.SumValue, wantA)
	}
	// Sanity ordering: smaller true rate → larger PV.
	if !(a.SumValue < b.SumValue && b.SumValue < c.SumValue) {
		t.Errorf("expected A < B < C (smaller true rate → larger PV), got A=%.2f B=%.2f C=%.2f",
			a.SumValue, b.SumValue, c.SumValue)
	}
}
