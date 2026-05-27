// Tests for the PV row-level backward dispatch (PV-1, PV-2, PV-4,
// PV-5, PV-6). The engine has had `solveLumpAmount`, `solveLumpDate`,
// `solvePeriodicAmount`, and `solvePeriodicDate` since the original
// BackwardCalc port, but the `FirstPass` dispatcher gated them on a
// screen-level Sum Value being given — so a user filling a single
// row's Date + Value (or Amount + Value, or two periodic fields +
// Value) could not trigger the per-row solve.  R10 widened the
// gate: a row that supplies its own target Value with exactly one
// of its core fields missing now fires the matching solver,
// regardless of whether the screen-level Sum Value is set.

package api

import (
	"fmt"
	"math"
	"testing"
)

// TestPV1RowLevelLumpAmountSolve: a lump-sum row with Date + Value
// (no Amount) asks the engine to back out the Amount from the row's
// own target Value, with no screen-level Sum Value.
//
// Sanity number: $10,000 paid in exactly one year, discounted at
// 8% continuously, has a PV of 10000 * e^(-0.08) ≈ $9,231.16. So a
// row with Date = +1y, Value = 9231.16 must solve back to
// Amount = $10,000.
func TestPV1RowLevelLumpAmountSolve(t *testing.T) {
	resp, code := pvCall(t, `{
		"asOfDate": "2024-01-01",
		"rate": 0.08,
		"lumpSums": [
			{"date": "2025-01-01", "value": 9231.163463866358}
		]
	}`)
	if code != 200 || resp.Error != "" {
		t.Fatalf("PV-1 row-level solve failed: code=%d err=%q", code, resp.Error)
	}
	if len(resp.LumpSums) == 0 {
		t.Fatalf("no lump-sum row in response")
	}
	amt := resp.LumpSums[0].Amount
	if math.Abs(amt-10000) > 0.01 {
		t.Errorf("PV-1 amount = %.4f, expected 10000.00 — engine did not "+
			"dispatch to solveLumpAmount when the row supplied its own "+
			"target Value", amt)
	}
}

// TestPV2RowLevelLumpDateSolve: a lump-sum row with Amount + Value
// (no Date) asks the engine to solve when the payment occurs.
func TestPV2RowLevelLumpDateSolve(t *testing.T) {
	resp, code := pvCall(t, `{
		"asOfDate": "2024-01-01",
		"rate": 0.08,
		"lumpSums": [
			{"amount": 10000, "value": 9231.163463866358}
		]
	}`)
	if code != 200 || resp.Error != "" {
		t.Fatalf("PV-2 row-level solve failed: code=%d err=%q", code, resp.Error)
	}
	if len(resp.LumpSums) == 0 {
		t.Fatalf("no lump-sum row in response")
	}
	date := resp.LumpSums[0].Date
	if date != "2025-01-01" {
		t.Errorf("PV-2 date = %q, expected \"2025-01-01\" — engine did "+
			"not dispatch to solveLumpDate", date)
	}
}

// TestPV4RowLevelPeriodicAmountSolve: a periodic row with both
// dates + target Value (no Amount) asks the engine to back out the
// payment amount.
//
// A forward calc supplies the reference Value so the round-trip is
// exact rather than depending on a hand-typed approximation.
func TestPV4RowLevelPeriodicAmountSolve(t *testing.T) {
	fwd, _ := pvCall(t, `{
		"asOfDate": "2024-01-01",
		"rate": 0.06,
		"periodics": [
			{"fromDate": "2024-01-01", "toDate": "2025-01-01",
			 "perYr": 12, "amount": 100}
		]
	}`)
	if len(fwd.Periodics) == 0 {
		t.Fatalf("forward reference: no periodic row")
	}
	targetVal := fwd.Periodics[0].Value

	body := fmt.Sprintf(`{
		"asOfDate": "2024-01-01",
		"rate": 0.06,
		"periodics": [
			{"fromDate": "2024-01-01", "toDate": "2025-01-01",
			 "perYr": 12, "value": %.10f}
		]
	}`, targetVal)
	resp, code := pvCall(t, body)
	if code != 200 || resp.Error != "" {
		t.Fatalf("PV-4 row-level solve failed: code=%d err=%q", code, resp.Error)
	}
	amt := resp.Periodics[0].Amount
	if math.Abs(amt-100) > 0.05 {
		t.Errorf("PV-4 amount = %.4f, expected 100.00 — engine did not "+
			"dispatch to solvePeriodicAmount", amt)
	}
}

// TestPV5RowLevelPeriodicToDateSolve: FromDate + Amount + Value
// (no ToDate) solves the end date.
func TestPV5RowLevelPeriodicToDateSolve(t *testing.T) {
	fwd, _ := pvCall(t, `{
		"asOfDate": "2024-01-01",
		"rate": 0.06,
		"periodics": [
			{"fromDate": "2024-01-01", "toDate": "2026-01-01",
			 "perYr": 12, "amount": 100}
		]
	}`)
	targetVal := fwd.Periodics[0].Value

	body := fmt.Sprintf(`{
		"asOfDate": "2024-01-01",
		"rate": 0.06,
		"periodics": [
			{"fromDate": "2024-01-01",
			 "perYr": 12, "amount": 100, "value": %.10f}
		]
	}`, targetVal)
	resp, code := pvCall(t, body)
	if code != 200 || resp.Error != "" {
		t.Fatalf("PV-5 row-level solve failed: code=%d err=%q", code, resp.Error)
	}
	to := resp.Periodics[0].ToDate
	if to == "" || to < "2025-12-25" || to > "2026-01-08" {
		t.Errorf("PV-5 toDate = %q, expected near 2026-01-01 — engine "+
			"did not dispatch to solvePeriodicDate(solveTo=true)", to)
	}
}

// TestPV6RowLevelPeriodicFromDateSolve: ToDate + Amount + Value
// (no FromDate) solves the start date.
func TestPV6RowLevelPeriodicFromDateSolve(t *testing.T) {
	fwd, _ := pvCall(t, `{
		"asOfDate": "2024-01-01",
		"rate": 0.06,
		"periodics": [
			{"fromDate": "2024-01-01", "toDate": "2026-01-01",
			 "perYr": 12, "amount": 100}
		]
	}`)
	targetVal := fwd.Periodics[0].Value

	body := fmt.Sprintf(`{
		"asOfDate": "2024-01-01",
		"rate": 0.06,
		"periodics": [
			{"toDate": "2026-01-01",
			 "perYr": 12, "amount": 100, "value": %.10f}
		]
	}`, targetVal)
	resp, code := pvCall(t, body)
	if code != 200 || resp.Error != "" {
		t.Fatalf("PV-6 row-level solve failed: code=%d err=%q", code, resp.Error)
	}
	from := resp.Periodics[0].FromDate
	if from == "" || from < "2023-12-25" || from > "2024-01-08" {
		t.Errorf("PV-6 fromDate = %q, expected near 2024-01-01 — engine "+
			"did not dispatch to solvePeriodicDate(solveTo=false)", from)
	}
}

// TestAmortTargetBalloonViaAPI: a balloon row with Date only (no
// Amount) used to be rejected by the frontend; the API has always
// accepted it as a target-balloon backward solve. This pins the API
// contract the new UI relies on.
func TestAmortTargetBalloonViaAPI(t *testing.T) {
	resp, code := amzCall(t, `{
		"amount": 200000, "loanDate": "2025-01-01",
		"firstDate": "2025-02-01", "rate": 0.06,
		"nPeriods": 60, "perYr": 12, "payment": 1199.10,
		"balloons": [{"date": "2030-01-01"}]
	}`)
	if code != 200 || resp["error"] != nil {
		t.Fatalf("target-balloon solve failed: code=%d err=%v", code, resp["error"])
	}
	sched, _ := resp["schedule"].([]any)
	if len(sched) == 0 {
		t.Fatalf("no schedule rows")
	}
	last := sched[len(sched)-1].(map[string]any)
	princ, _ := last["principal"].(float64)
	if math.Abs(princ) > 1.0 {
		t.Errorf("target-balloon final balance = %.2f, expected near 0 — "+
			"the engine did not solve a balloon amount that clears the "+
			"schedule", princ)
	}
}
