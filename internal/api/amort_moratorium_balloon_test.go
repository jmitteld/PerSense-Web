package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAmortMoratoriumBalloonReconciliation guards the fancy-engine fix for a
// moratorium (interest-only deferment) combined with a later balloon. DOS solves
// the loan as a SINGLE payment that drives the WHOLE schedule — the moratorium
// interest-only periods included — to a zero terminal, so that payment is
// inherently balloon-aware (RepayFancyLoan + Iterate, AMORTOP.pas:1216/1499).
//
// Before the fix, the Go engine re-amortized the payment at the FirstRepay
// boundary with a balloon-BLIND annuity (estimatePayment), so the post-
// moratorium payment was too high: the loan over-paid and RETIRED EARLY (around
// payment 234 of 360), collecting only ~$102,258 total interest versus DOS's
// ~$157,192 — a ~$55k divergence (docs/amort_option_combo_divergences.md §3).
//
// Note the trap: the buggy schedule still ends at ~$0 final balance (it retires,
// just early), so final balance alone does NOT catch this. TOTAL INTEREST and
// running the FULL term are the signals.
//
// The fix (engine.go moratorium recompute + fancybisect.go
// solveMoratoriumPayment): refine the balloon-blind seed against the real
// remaining schedule (a sub-loan carrying the not-yet-applied balloons) so the
// post-moratorium payment retires the loan at term like DOS.
//
// Golden: legacy/oracle/amort_oracle 100000 0.08 360 12 mor=24 b120=30000 plusreg
//
//	→ total interest 157,192.84, retires at the full 360-period term.
//
// To confirm the guard: temporarily revert the solveMoratoriumPayment call in
// engine.go — total interest drops to ~$102,258 and the schedule retires early
// (well under 360 rows), failing both assertions.
func TestAmortMoratoriumBalloonReconciliation(t *testing.T) {
	body := `{"amount":100000,"loanDate":"2024-01-01","rate":0.08,"firstDate":"2024-02-01",
	          "nPeriods":360,"perYr":12,"basis":"360",
	          "moratorium":"2026-01-01",
	          "balloons":[{"date":"2034-01-01","amount":30000}]}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	HandleAmortizationCalc(rec, req)

	var resp AmortizationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response: %v\n%s", err, rec.Body.String())
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Schedule) == 0 {
		t.Fatal("empty schedule")
	}

	// Primary signal: total interest matches DOS. The balloon-blind bug lands at
	// ~$102,258 (loan retires early); DOS is $157,192.84.
	const dosTotalInt = 157192.84
	if d := resp.TotalInt - dosTotalInt; d < -50 || d > 50 {
		t.Errorf("total interest = %.2f, want DOS %.2f (±50). A value near $102,258 means "+
			"the moratorium re-amortization went balloon-blind and the loan retired early.",
			resp.TotalInt, dosTotalInt)
	}

	// The loan must run essentially the FULL term, not retire early. DOS keeps
	// the schedule through period 360; the bug retires at ~234. Find the last
	// row with an actual payment.
	lastPaid := 0
	for _, r := range resp.Schedule {
		if r.Payment > 0 && r.PayNum > lastPaid {
			lastPaid = r.PayNum
		}
	}
	if lastPaid < 355 {
		t.Errorf("loan retired at payment %d, want ~360 (full term). Early retirement "+
			"means the post-moratorium payment was solved balloon-blind.", lastPaid)
	}

	// And it does retire (DOS final balance 0).
	finalBal := resp.Schedule[len(resp.Schedule)-1].Principal
	if finalBal < -5 || finalBal > 5 {
		t.Errorf("final balance = %.2f, want ~0 (DOS retires the loan).", finalBal)
	}
}
