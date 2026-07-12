package api

// Regression guard for the "Exact method for periodic payments" Computational
// Setting being wired through the Present Value screen.
//
// ROOT CAUSE (2026-07-12 audit): HandlePVCalc hardcoded Settings.Exact = false
// and getPVInput never forwarded the set-exact toggle, so selecting DOS's
// "Exact method for periodic payments = YES" silently did nothing on the PV
// screen. The periodic annuity was discounted with the closed-form
// nominal-period formula instead of DOS's period-by-period actual-day
// summation (PeriodicSummation's `if settings.Exact` branch). This is the PV
// twin of the Amortization "Exact method" wiring gap fixed 2026-06-19
// (docs/discrepancies.md §8).
//
// PROVENANCE (DOS oracle = the real DOS engine, Present Value screen):
//   Computational Settings: basis 365, COLA=ANN, interest-on-interest=ACT,
//     1st-int-prepaid=YES, interest-in=ADV, balloon-incl=YES, EXACT=YES, R78=YES
//   As-of 04/25/2006, Rate Type Loan 5.0000% (→ true rate 4.9896%),
//   Periodic: $3,500/mo, from 05/15/2030 through 12/15/2060, per/yr 12, COLA 0.
//   DOS periodic Value = 198,980.58 (screen total 1,914,872.1).
// With Exact OFF the engine returns 198,993.50 (the pre-fix value) — a $12.92
// over-valuation. The exact path reproduces DOS to the cent.

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPVExactPeriodicMatchesDOS(t *testing.T) {
	// Loan Rate 5% → continuous true rate, exactly as the frontend converts it
	// (pvRateToTrue: TrueRate = 12·ln(1 + LoanRate/12)).
	trueRate := 12 * math.Log1p(0.05/12)
	rateLit := strings.TrimRight(strings.TrimRight(
		func() string { b, _ := json.Marshal(trueRate); return string(b) }(), "0"), ".")

	post := func(exact bool) PVResponse {
		exactField := ""
		if exact {
			exactField = `"exact":true,`
		}
		body := `{
			"asOfDate":"2006-04-25",
			"rate":` + rateLit + `,
			"basis":"365",` + exactField + `
			"lumpSums":[{"date":"2027-10-01","amount":5000000}],
			"periodics":[{"fromDate":"2030-05-15","toDate":"2060-12-15","perYr":12,"amount":3500,"cola":0}]
		}`
		req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		HandlePVCalc(w, req)
		var resp PVResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Error != "" {
			t.Fatalf("unexpected error: %s", resp.Error)
		}
		return resp
	}

	const dosPeriodic = 198980.58 // real DOS engine, exact method ON
	const tol = 0.005

	// Exact ON must reproduce DOS to the cent.
	on := post(true)
	if len(on.Periodics) != 1 {
		t.Fatalf("expected 1 periodic, got %d", len(on.Periodics))
	}
	if diff := math.Abs(on.Periodics[0].Value - dosPeriodic); diff > tol {
		t.Errorf("Exact ON periodic value = %.2f, want DOS %.2f (diff %.4f)",
			on.Periodics[0].Value, dosPeriodic, diff)
	}

	// Exact OFF is the pre-fix closed-form value; assert the fix actually
	// changes behavior (guards against the flag silently doing nothing again).
	off := post(false)
	if math.Abs(off.Periodics[0].Value-dosPeriodic) <= tol {
		t.Errorf("Exact OFF unexpectedly matched DOS (%.2f); the flag is not being honored",
			off.Periodics[0].Value)
	}
	if off.Periodics[0].Value <= on.Periodics[0].Value {
		t.Errorf("expected closed-form (%.2f) to over-value vs exact (%.2f)",
			off.Periodics[0].Value, on.Periodics[0].Value)
	}
}
