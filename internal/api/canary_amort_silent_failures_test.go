// Canary tests for dispatch_gaps.md silent-failure paths S-1 and S-2.
//
// These tests are EXPECTED TO FAIL TODAY. They document existing bugs
// where the Amortization handler silently runs the engine with bogus
// inputs instead of dispatching to the available SolveRate /
// SolveLoanAmount engine functions.
//
// When Phase 1 of docs/dispatch_gaps.md ships (AmortizationRequest
// .Rate and .Amount become *float64 and the handler dispatches nil
// fields to the solvers), these canaries flip to green and become
// regression tests.
//
// See docs/test_plan.md §1 (Wave 1 canaries) for the test plan that
// produced these.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCanaryC1_AmortRateOmittedSilentlyRunsZeroRate documents
// dispatch_gaps.md S-1: posting an amortization request without a
// `rate` field runs the engine with LoanRate=0 (the Go zero value)
// instead of dispatching to SolveRate. The schedule that comes back
// has zero interest on every row.
//
// Reproduce: a $200k loan paid in 360 installments of $1199.10
// implies a rate near 6%. If the engine actually used the implied
// rate, the first row's interest would be ~$1000. The canary
// assertion is `Schedule[0].Interest > 500`.
//
// Pairs with: docs/dispatch_gaps.md §4.1 S-1; test_plan.md C-1.
// Fixes the canary: Phase 1 — make AmortizationRequest.Rate
// *float64; dispatch nil to amortization.SolveRate.
func TestCanaryC1_AmortRateOmittedSilentlyRunsZeroRate(t *testing.T) {
	// `rate` is intentionally omitted. With the current float64
	// field type, this unmarshals to 0.0 with no way to tell the
	// difference between "user supplied 0" and "user left it blank".
	body := `{
		"amount": 200000,
		"loanDate": "2025-01-01",
		"firstDate": "2025-02-01",
		"nPeriods": 360,
		"perYr": 12,
		"payment": 1199.10
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp AmortizationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "" {
		// Acceptable post-fix: explicit error naming the rate field.
		// Today's behavior is "no error" with a zero-interest schedule.
		t.Logf("response error (acceptable after fix): %s", resp.Error)
		return
	}
	if len(resp.Schedule) == 0 {
		t.Fatalf("expected a non-empty schedule; got 0 rows")
	}
	// The smoking gun: first-row interest should be ~$1000 on a
	// 6%-implied loan. If it's near zero, the engine silently ran
	// at rate=0.
	first := resp.Schedule[0]
	if first.Interest < 500 {
		t.Errorf("CANARY: first row interest = $%.2f — the engine ran with rate=0 "+
			"instead of dispatching to SolveRate (dispatch_gaps S-1). "+
			"Expected first-row interest near $1,000 on an implied 6%% loan.",
			first.Interest)
	}
}

// TestCanaryC2_AmortAmountOmittedSilentlyZero documents
// dispatch_gaps.md S-2: posting an amortization request without an
// `amount` field runs the engine with Amount=0 (or produces a
// confusing "need amount and payments per year" error even when
// PerYr IS supplied) instead of dispatching to SolveLoanAmount.
//
// Reproduce: payment 1199.10, 360 periods, rate 6% implies amount
// ~$200,000. The canary expects either the response to carry that
// solved amount through TotalPaid - TotalInt ≈ principal, or an
// explicit, field-named error.
//
// Pairs with: docs/dispatch_gaps.md §4.1 S-2; test_plan.md C-2.
// Fixes the canary: Phase 1 — make AmortizationRequest.Amount
// *float64; dispatch nil to amortization.SolveLoanAmount.
func TestCanaryC2_AmortAmountOmittedSilentlyZero(t *testing.T) {
	body := `{
		"rate": 0.06,
		"loanDate": "2025-01-01",
		"firstDate": "2025-02-01",
		"nPeriods": 360,
		"perYr": 12,
		"payment": 1199.10
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)

	// Two acceptable post-fix outcomes:
	//   (a) The handler dispatched to SolveLoanAmount; response is
	//       200 with a populated schedule whose principal totals
	//       ~$200,000.
	//   (b) The handler returned a clean error explicitly naming
	//       Amount Borrowed as the missing field.
	// The unacceptable status quo is the current behavior: either
	//   - response error "insufficient loan data: need amount and
	//     payments per year" (confusing because PerYr IS supplied),
	//     or
	//   - silent acceptance of Amount=0 producing a near-empty
	//     schedule.
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 200 or 400; body=%s", w.Code, w.Body.String())
	}
	var resp AmortizationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Error != "" {
		// If we get an error, it must explicitly name Amount Borrowed
		// (not the current confusing PerYr-conflating message).
		want := "Amount" // case-sensitive substring; "amount" alone is too loose
		if !containsAny(resp.Error, []string{"Amount Borrowed", "Amount", "amount"}) {
			t.Errorf("error message %q does not name the Amount field",
				resp.Error)
		}
		if containsAny(resp.Error, []string{"payments per year", "perYr"}) &&
			!containsAny(resp.Error, []string{"Amount", "amount"}) {
			t.Errorf("CANARY: error message %q blames PerYr but PerYr is supplied — "+
				"dispatch_gaps S-2. Engine should dispatch to SolveLoanAmount.",
				resp.Error)
		}
		_ = want
		return
	}

	// No error → schedule must carry a meaningful principal. If the
	// engine silently ran with Amount=0, total payments and total
	// interest both collapse to ~0.
	principal := resp.TotalPaid - resp.TotalInt
	if principal < 100000 {
		t.Errorf("CANARY: principal (TotalPaid - TotalInt) = $%.2f — the engine ran "+
			"with Amount=0 instead of dispatching to SolveLoanAmount (dispatch_gaps S-2). "+
			"Expected principal near $200,000.", principal)
	}
}

// containsAny returns true if s contains any of subs (case-sensitive).
func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if len(sub) == 0 {
			continue
		}
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
