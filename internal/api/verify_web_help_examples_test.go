package api

// Verification harness for the Amortization + Present Value examples
// documented in cmd/persense/static/help.html. Each subtest sends the
// exact input set the help instructs the user to type, then logs what
// the engine actually returns. Numeric assertions are wrapped in
// closeEnough so a small rounding drift surfaces as a test failure
// rather than a silent mismatch with the prose.
//
// When the help text and the engine disagree, the help is the
// document being audited — these tests are how we find that out.
// They're intentionally kept under `verify_*` so they're easy to
// grep for and skip if needed.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// callAmortize POSTs to /api/amortization/calc with the given JSON
// body and returns the decoded response. Fails the test on transport
// or decode errors; surfaces API-level errors as resp.Error for the
// caller to inspect.
func callAmortize(t *testing.T, body string) AmortizationResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp AmortizationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

// callPV POSTs to /api/presentvalue/calc with the given JSON body and
// returns the decoded response.
func callPV(t *testing.T, body string) PVResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandlePVCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp PVResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

// approxEqual reports whether a and b are within tol of each other.
// Use a slightly loose tolerance for help-text comparisons — the
// printed values are rounded for human consumption.
func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// callMortgage POSTs to /api/mortgage/calc with the given JSON body
// and returns the decoded response. Fails the test on transport or
// decode errors; surfaces API-level errors as resp.Error for the
// caller to inspect.
func callMortgage(t *testing.T, body string) MortgageResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/calc",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandleMortgageCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp MortgageResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

// --------------- Mortgage web-help examples ---------------

// MS Example 1: $200k house, 20yr, 8%, 2 pts, 20% down, $200 tax.
// Help claims Cash = $43,200, Financed = $160,000, Monthly = $1,538.30.
func TestVerifyWebMS_EX1_ComputeMonthly(t *testing.T) {
	resp := callMortgage(t, `{
		"price": 200000,
		"points": 0.02,
		"pctDown": 0.20,
		"years": 20,
		"rate": 0.08,
		"tax": 200
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("MS EX1 → Cash=%.2f (help 43,200) Financed=%.2f (help 160,000) Monthly=%.2f (help 1,538.30)",
		resp.Cash, resp.Financed, resp.Monthly)
	if !approxEqual(resp.Cash, 43200, 0.50) {
		t.Errorf("cash = %.2f, help says 43,200", resp.Cash)
	}
	if !approxEqual(resp.Financed, 160000, 0.50) {
		t.Errorf("financed = %.2f, help says 160,000", resp.Financed)
	}
	if !approxEqual(resp.Monthly, 1538.30, 0.10) {
		t.Errorf("monthly = %.2f, help says 1,538.30", resp.Monthly)
	}
}

// MS Example 2: Solve for Price from Cash + Monthly. Cash=56,000,
// Pts=1.5, Years=30, Rate=8.5, Tax=200, Monthly=1,650. Help claims
// Price = $241,749.12, % Down = 21.9944, Financed = $188,577.78.
func TestVerifyWebMS_EX2_SolvePrice(t *testing.T) {
	resp := callMortgage(t, `{
		"points": 0.015,
		"cash": 56000,
		"years": 30,
		"rate": 0.085,
		"tax": 200,
		"monthly": 1650
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("MS EX2 → Price=%.2f (help 241,749.12) PctDown=%.4f (help 21.9944) Financed=%.2f (help 188,577.78)",
		resp.Price, 100*resp.PctDown, resp.Financed)
	if !approxEqual(resp.Price, 241749.12, 1.0) {
		t.Errorf("price = %.2f, help says 241,749.12", resp.Price)
	}
	if !approxEqual(100*resp.PctDown, 21.9944, 0.01) {
		t.Errorf("%%Down = %.4f, help says 21.9944", 100*resp.PctDown)
	}
	if !approxEqual(resp.Financed, 188577.78, 1.0) {
		t.Errorf("financed = %.2f, help says 188,577.78", resp.Financed)
	}
}

// MS Example 3: Solve for Balloon. Price=280k, Pts=2.5, %Down=20,
// Years=30, Rate=8.25, Tax=300, Monthly=1,600, BalloonYears=8.
// Help claims Balloon = $98,372.47.
func TestVerifyWebMS_EX3_BalloonAmount(t *testing.T) {
	resp := callMortgage(t, `{
		"price": 280000,
		"points": 0.025,
		"pctDown": 0.20,
		"years": 30,
		"rate": 0.0825,
		"tax": 300,
		"monthly": 1600,
		"balloonYears": 8
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("MS EX3 → Cash=%.2f (help 61,600) Financed=%.2f (help 224,000) Balloon=%.2f (help 98,372.47)",
		resp.Cash, resp.Financed, resp.BalloonAmount)
	if !approxEqual(resp.Cash, 61600, 0.50) {
		t.Errorf("cash = %.2f, help says 61,600", resp.Cash)
	}
	if !approxEqual(resp.Financed, 224000, 0.50) {
		t.Errorf("financed = %.2f, help says 224,000", resp.Financed)
	}
	if !approxEqual(resp.BalloonAmount, 98372.47, 0.50) {
		t.Errorf("balloon = %.2f, help says 98,372.47", resp.BalloonAmount)
	}
}

// MS Example 4 step 1 + step 2 together. EX1: Price=240k, %Down=0,
// Years=30, Rate=8.1 → Monthly $1,777.79. EX2 (after hardening
// monthly): Years=15, BalloonYrs=15 → Balloon $184,912.27.
func TestVerifyWebMS_EX4_30yrPaymentsWith15yrBalloon(t *testing.T) {
	step1 := callMortgage(t, `{
		"price": 240000,
		"points": 0,
		"pctDown": 0,
		"years": 30,
		"rate": 0.081
	}`)
	if step1.Error != "" {
		t.Fatalf("step1 error: %s", step1.Error)
	}
	t.Logf("MS EX4 step1 → Monthly=%.2f (help 1,777.79)", step1.Monthly)
	if !approxEqual(step1.Monthly, 1777.79, 0.10) {
		t.Errorf("step1 monthly = %.2f, help says 1,777.79", step1.Monthly)
	}

	step2 := callMortgage(t, `{
		"price": 240000,
		"points": 0,
		"pctDown": 0,
		"years": 15,
		"rate": 0.081,
		"monthly": 1777.79,
		"balloonYears": 15
	}`)
	if step2.Error != "" {
		t.Fatalf("step2 error: %s", step2.Error)
	}
	t.Logf("MS EX4 step2 → Balloon=%.2f (help 184,912.27)", step2.BalloonAmount)
	if !approxEqual(step2.BalloonAmount, 184912.27, 0.10) {
		t.Errorf("step2 balloon = %.2f, help says 184,912.27", step2.BalloonAmount)
	}
}

// MS Example 5: APR comparison. Two rows, both with $100k at 0%
// down, 30 years; A is 8.1% with 3 pts, B is 8.5% with 1 pt. Help
// claims APR_A = 8.4257%, APR_B = 8.6094%. The "APRs cross at 6 yrs
// 10 mo" claim is engine-only (no API yet) — covered separately by
// TestHelpMS_EX5_APRComparison in the mortgage package.
func TestVerifyWebMS_EX5_APRComparisonRows(t *testing.T) {
	mortgageA := callMortgage(t, `{
		"price": 100000,
		"pctDown": 0,
		"years": 30,
		"rate": 0.081,
		"points": 0.03
	}`)
	mortgageB := callMortgage(t, `{
		"price": 100000,
		"pctDown": 0,
		"years": 30,
		"rate": 0.085,
		"points": 0.01
	}`)
	if mortgageA.Error != "" || mortgageB.Error != "" {
		t.Fatalf("errors: A=%q B=%q", mortgageA.Error, mortgageB.Error)
	}
	aprA := 100 * mortgageA.APR
	aprB := 100 * mortgageB.APR
	t.Logf("MS EX5 → APR_A=%.4f%% (help 8.4257) APR_B=%.4f%% (help 8.6094)", aprA, aprB)
	if !approxEqual(aprA, 8.4257, 0.005) {
		t.Errorf("APR_A = %.4f%%, help says 8.4257%%", aprA)
	}
	if !approxEqual(aprB, 8.6094, 0.005) {
		t.Errorf("APR_B = %.4f%%, help says 8.6094%%", aprB)
	}
	// Per the help: "If you hold the mortgage longer than [6 yrs 10
	// mo], Mortgage A is the better deal." Mortgage A's full-term
	// APR is lower than B's, so this should match.
	if aprA >= aprB {
		t.Errorf("expected APR_A < APR_B, got %.4f%% vs %.4f%%", aprA, aprB)
	}
}

// MS Example 6: What-If Table. $100k, 0% down, 30yr. Verify each
// rate point in the table. The FE generates this by iterating rate
// from 7.0 to 9.0 in 0.25 increments; the API just does one calc
// per rate.
func TestVerifyWebMS_EX6_WhatIfTable(t *testing.T) {
	expected := []struct {
		rate    float64
		monthly float64
	}{
		{0.0700, 665.30},
		{0.0725, 682.18},
		{0.0750, 699.21},
		{0.0775, 716.41},
		{0.0800, 733.76},
		{0.0825, 751.27},
		{0.0850, 768.91},
		{0.0875, 786.70},
		{0.0900, 804.62},
	}
	for _, c := range expected {
		body := `{
			"price": 100000,
			"pctDown": 0,
			"years": 30,
			"rate": ` + formatFloat(c.rate) + `
		}`
		resp := callMortgage(t, body)
		if resp.Error != "" {
			t.Errorf("rate=%.4f: error=%s", c.rate, resp.Error)
			continue
		}
		if !approxEqual(resp.Monthly, c.monthly, 0.02) {
			t.Errorf("rate=%.4f%%: monthly=%.2f, help says %.2f",
				100*c.rate, resp.Monthly, c.monthly)
		}
	}
}

// formatFloat — tiny helper because the test isn't picky about JSON
// number formatting and Sprintf %.6f for 0.0825 introduces trailing
// zeros that some strict parsers would complain about. Go's
// encoding/json is fine with either, so plain %g is fine.
func formatFloat(f float64) string {
	return fmt.Sprintf("%g", f)
}

// --------------- Amortization web-help examples ---------------

// AM Example 1: $100k @ 8%, 30 years monthly, 360 basis. Help claims
// Payment = $733.76, total interest = $161,499.77.
func TestVerifyWebAM_EX1_Simple(t *testing.T) {
	resp := callAmortize(t, `{
		"amount": 100000,
		"loanDate": "2024-02-12",
		"rate": 0.08,
		"firstDate": "2024-03-01",
		"nPeriods": 360,
		"perYr": 12,
		"basis": "360"
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	pmt := resp.Schedule[0].Payment
	t.Logf("AM EX1 → payment=%.2f (help 733.76)  total_int=%.2f (help 161,499.77)  pmt1_int=%.2f (help 422.22)",
		pmt, resp.TotalInt, resp.Schedule[0].Interest)
	if !approxEqual(pmt, 733.76, 0.01) {
		t.Errorf("payment = %.2f, help says 733.76", pmt)
	}
	if !approxEqual(resp.TotalInt, 161499.77, 1.0) {
		t.Errorf("total interest = %.2f, help says 161,499.77", resp.TotalInt)
	}
	if !approxEqual(resp.Schedule[0].Interest, 422.22, 0.01) {
		t.Errorf("pmt#1 interest = %.2f, help says 422.22", resp.Schedule[0].Interest)
	}
}

// AM Example 1b: same as EX1 but with firstDate blank. Help claims
// the engine defaults firstDate to 03/12/2024 (loanDate + 1 month).
func TestVerifyWebAM_EX1b_OmitFirstDate(t *testing.T) {
	resp := callAmortize(t, `{
		"amount": 100000,
		"loanDate": "2024-02-12",
		"rate": 0.08,
		"nPeriods": 360,
		"perYr": 12,
		"basis": "360"
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("AM EX1b → firstDate echoed=%q (help 03/12/2024 = 2024-03-12)  payment=%.2f",
		resp.FirstDate, resp.Schedule[0].Payment)
	if resp.FirstDate != "2024-03-12" {
		t.Errorf("derived firstDate = %q, help says 2024-03-12", resp.FirstDate)
	}
}

// AM Example 1c: derive-only term query — supply first + last dates,
// get nPeriods back. Help claims # Periods = 360.
func TestVerifyWebAM_EX1c_DeriveTerm(t *testing.T) {
	resp := callAmortize(t, `{
		"firstDate": "2024-02-01",
		"lastDate": "2054-01-01",
		"perYr": 12
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("AM EX1c → derived nPeriods=%d (help 360)", resp.NPeriods)
	if resp.NPeriods != 360 {
		t.Errorf("derived nPeriods = %d, help says 360", resp.NPeriods)
	}
	if len(resp.Schedule) != 0 {
		t.Errorf("derive-only response should have empty schedule, got %d rows",
			len(resp.Schedule))
	}
}

// AM Example 1d: solve for payment. Help claims Pmt = $1,498.88.
func TestVerifyWebAM_EX1d_SolvePayment(t *testing.T) {
	resp := callAmortize(t, `{
		"amount": 250000,
		"loanDate": "2024-01-01",
		"rate": 0.06,
		"firstDate": "2024-02-01",
		"nPeriods": 360,
		"perYr": 12,
		"basis": "360"
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	pmt := resp.Schedule[0].Payment
	t.Logf("AM EX1d → payment=%.2f (help 1,498.88)", pmt)
	if !approxEqual(pmt, 1498.88, 0.05) {
		t.Errorf("payment = %.2f, help says 1,498.88", pmt)
	}
}

// AM Example 2: APR with points. Help AM_EX2 — a $250,000 loan at
// 12.4% with 2.5 discount points — expects an APR of 12.7499%. The
// /api/amortization/calc endpoint computes the APR (DOS
// EstimateAndRefineAPRwithPoints) whenever the request supplies
// non-zero Points. This verifies the handler plumbs Points through
// and returns the help's APR.
func TestVerifyWebAM_EX2_APRWithPoints(t *testing.T) {
	resp := callAmortize(t, `{
		"amount": 250000,
		"loanDate": "1994-06-21",
		"rate": 0.124,
		"firstDate": "1994-08-01",
		"nPeriods": 360,
		"perYr": 12,
		"points": 0.025
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	if !resp.APRConverged {
		t.Fatalf("APR solve did not converge")
	}
	aprPct := resp.APR * 100
	t.Logf("AM EX2 → APR=%.4f%% (help 12.7499%%)", aprPct)
	if !approxEqual(aprPct, 12.7499, 0.02) {
		t.Errorf("APR = %.4f%%, help says 12.7499%%", aprPct)
	}
}

// AM Example 3: Weekly payments. $100k @ 8%, 30 years × 52 = 1560
// weekly payments, 365-day basis. Help claims weekly payment ≈ $168.87.
func TestVerifyWebAM_EX3_Weekly(t *testing.T) {
	resp := callAmortize(t, `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.08,
		"nPeriods": 1560,
		"perYr": 52,
		"basis": "365"
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	pmt := resp.Schedule[0].Payment
	t.Logf("AM EX3 → payment=%.4f (help 168.87)  schedule_rows=%d",
		pmt, len(resp.Schedule))
	// Help (post-verification) says ~$168.79.
	if !approxEqual(pmt, 168.79, 0.05) {
		t.Errorf("weekly payment = %.4f, help says 168.79", pmt)
	}
}

// AM Example 4 (Monthly leg): $100k @ 8%, 30 years monthly.
// Help claims Payment ≈ $733.76, total interest ≈ $164,156.
// (Note: the AM EX1 leg of the same loan with a partial first period
// produces total_int = 161,499.77 — the $164k figure assumes a clean
// "first payment exactly one period after loan date" alignment.)
func TestVerifyWebAM_EX4_Monthly(t *testing.T) {
	resp := callAmortize(t, `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.08,
		"nPeriods": 360,
		"perYr": 12,
		"basis": "360"
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	pmt := resp.Schedule[0].Payment
	t.Logf("AM EX4 monthly → payment=%.2f (help 733.76)  total_int=%.2f (help ~164,156)",
		pmt, resp.TotalInt)
	if !approxEqual(pmt, 733.76, 0.01) {
		t.Errorf("monthly payment = %.2f, help says 733.76", pmt)
	}
}

// AM Example 4 (Biweekly leg): $100k @ 8%, 30 × 26 = 780 biweekly
// payments. Help is hand-wavy about exact numbers ("substantially
// less interest") so the test just records the engine's output.
func TestVerifyWebAM_EX4_Biweekly(t *testing.T) {
	resp := callAmortize(t, `{
		"amount": 100000,
		"loanDate": "2024-01-01",
		"rate": 0.08,
		"nPeriods": 780,
		"perYr": 26,
		"basis": "360"
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("AM EX4 biweekly → payment=%.2f  total_int=%.2f  schedule_rows=%d",
		resp.Schedule[0].Payment, resp.TotalInt, len(resp.Schedule))
}

// AM Example 5: Interest-only 5-year loan with $100k balloon at end.
// Help claims Payment = $666.67 (interest each month), final-period
// balloon = $100,000.
func TestVerifyWebAM_EX5_InterestOnlyBalloon(t *testing.T) {
	resp := callAmortize(t, `{
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
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("AM EX5 → rows=%d  total_paid=%.2f  total_int=%.2f",
		len(resp.Schedule), resp.TotalPaid, resp.TotalInt)
	// Final-period payment should reflect balloon-ADDED-to-regular
	// (DOS default): pmt[60] = regular $666.67 + balloon $100,000 =
	// $100,666.67, and the post-payment running balance should be ~0.
	last := resp.Schedule[len(resp.Schedule)-1]
	t.Logf("  final row payNum=%d date=%s pmt=%.4f int=%.4f post-bal=%.4f",
		last.PayNum, last.Date, last.Payment, last.Interest, last.Principal)
	if !approxEqual(last.Payment, 100666.67, 0.10) {
		t.Errorf("final payment = %.4f, want ~100,666.67 (regular + balloon)",
			last.Payment)
	}
	if math.Abs(last.Principal) > 1.0 {
		t.Errorf("final running balance = %.4f, want ~0 (loan cleared)",
			last.Principal)
	}
	// Total paid: 60 × $666.67 (regular each period) + $100,000 (balloon)
	// = $40,000.20 + $100,000 = $140,000.20.
	if !approxEqual(resp.TotalPaid, 140000.20, 0.50) {
		t.Errorf("total paid = %.2f, want ~140,000.20", resp.TotalPaid)
	}
}

// --------------- Present Value web-help examples ---------------

// PV Example 1: Lump sum $10,000 paid 01/01/2025, as-of 01/01/2024,
// rate 8%. Help claims PV ≈ $9,231.
func TestVerifyWebPV_EX1_LumpSum(t *testing.T) {
	resp := callPV(t, `{
		"asOfDate": "2024-01-01",
		"rate": 0.08,
		"lumpSums": [
			{"date": "2025-01-01", "amount": 10000}
		]
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("PV EX1 → SumValue=%.2f (help ~9,231)  LumpSums[0].Value=%.2f",
		resp.SumValue, valueOfFirstLump(resp))
	if !approxEqual(resp.SumValue, 9231, 5) {
		t.Errorf("SumValue = %.2f, help says ~9,231", resp.SumValue)
	}
}

// PV Example 2: $1,000/month for 10 years (120 payments), as-of
// 01/01/2024, rate 6%. Help is hand-wavy on the number but a
// regression test should still snapshot it.
func TestVerifyWebPV_EX2_MonthlyAnnuity(t *testing.T) {
	resp := callPV(t, `{
		"asOfDate": "2024-01-01",
		"rate": 0.06,
		"periodics": [
			{"fromDate": "2024-01-01", "toDate": "2034-01-01", "perYr": 12, "amount": 1000}
		]
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("PV EX2 → SumValue=%.2f  Periodics[0].Value=%.2f",
		resp.SumValue, valueOfFirstPeriodic(resp))
}

// PV Example 3: $500/month for 20 years. Probe the engine's behavior
// at three different as-of dates to see whether it actually computes
// "future value" when the as-of sits past the cash flows.
func TestVerifyWebPV_EX3_FutureValue(t *testing.T) {
	makeBody := func(asOf string) string {
		return `{"asOfDate":"` + asOf + `","rate":0.07,"periodics":[` +
			`{"fromDate":"2024-01-01","toDate":"2044-01-01","perYr":12,"amount":500}]}`
	}
	at2024 := callPV(t, makeBody("2024-01-01"))
	at2034 := callPV(t, makeBody("2034-01-01"))
	at2044 := callPV(t, makeBody("2044-01-01"))
	t.Logf("PV EX3 as-of 2024 (start):  SumValue=%.2f", at2024.SumValue)
	t.Logf("PV EX3 as-of 2034 (middle): SumValue=%.2f", at2034.SumValue)
	t.Logf("PV EX3 as-of 2044 (end):    SumValue=%.2f", at2044.SumValue)
	// Expected FV at 2044 (per textbook): PMT × ((1+r)^n - 1)/r ≈ $260,463
	// for 240 monthly payments at 7%/12. If the engine actually compounds
	// forward, SumValue at 2044 should equal that. If it instead returns
	// PV regardless of as-of, SumValue will be ~$64k (the textbook PV of
	// the same annuity), which is what we expect to see here — and the
	// help text needs adjusting accordingly.
}

// PV Example 4: $2,000/month with 3% COLA for 30 years, as-of
// 01/01/2024, rate 5%.
func TestVerifyWebPV_EX4_AnnuityWithCOLA(t *testing.T) {
	resp := callPV(t, `{
		"asOfDate": "2024-01-01",
		"rate": 0.05,
		"periodics": [
			{"fromDate": "2024-01-01", "toDate": "2054-01-01", "perYr": 12, "amount": 2000, "cola": 0.03}
		]
	}`)
	if resp.Error != "" {
		t.Fatalf("API error: %s", resp.Error)
	}
	t.Logf("PV EX4 (with COLA) → SumValue=%.2f", resp.SumValue)

	// Sanity check: the same annuity without COLA should yield a smaller PV.
	respNoCola := callPV(t, `{
		"asOfDate": "2024-01-01",
		"rate": 0.05,
		"periodics": [
			{"fromDate": "2024-01-01", "toDate": "2054-01-01", "perYr": 12, "amount": 2000}
		]
	}`)
	t.Logf("PV EX4 (no COLA, sanity) → SumValue=%.2f", respNoCola.SumValue)
	if resp.SumValue <= respNoCola.SumValue {
		t.Errorf("COLA=3 PV (%.2f) should exceed COLA=0 PV (%.2f)",
			resp.SumValue, respNoCola.SumValue)
	}
}

// Helpers — pluck Value out of the first lump or periodic. Defined
// inline rather than imported because the test file only needs them
// here, and the PV response shape doesn't expose a public accessor.

func valueOfFirstLump(r PVResponse) float64 {
	for _, ls := range r.LumpSums {
		return ls.Value
	}
	return math.NaN()
}

func valueOfFirstPeriodic(r PVResponse) float64 {
	for _, p := range r.Periodics {
		return p.Value
	}
	return math.NaN()
}

// Quick compile-time check that strings is still imported by the file.
var _ = strings.TrimSpace

// --------------- Life Contingency (Actuarial) web-help examples ---------------
//
// The actuarial PV API takes a life table inline as [[age,qx], ...]. To
// avoid duplicating the SSA 2021 tables here, parse them out of the
// shipped frontend file `cmd/persense/static/lifetables.js` at test
// time. The format there is regular enough that a single regex over
// the [age,qx] pairs is sufficient.

// loadSSAQx returns the qx pairs from one of the two SSA arrays
// defined in lifetables.js. variable should be "SSA_2021_MALE_QX" or
// "SSA_2021_FEMALE_QX". Returns [][]float64{{age, qx}, ...}.
func loadSSAQx(t *testing.T, variable string) [][]float64 {
	t.Helper()
	const jsPath = "../../cmd/persense/static/lifetables.js"
	raw, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("read lifetables.js: %v", err)
	}
	src := string(raw)
	startIdx := strings.Index(src, variable+" = [")
	if startIdx < 0 {
		t.Fatalf("could not find %q in lifetables.js", variable)
	}
	endIdx := strings.Index(src[startIdx:], "];")
	if endIdx < 0 {
		t.Fatalf("could not find end of %q array", variable)
	}
	block := src[startIdx : startIdx+endIdx]
	pairRE := regexp.MustCompile(`\[(\d+),(0?\.\d+|1\.0+|1)\]`)
	matches := pairRE.FindAllStringSubmatch(block, -1)
	if len(matches) < 100 {
		t.Fatalf("parsed only %d qx rows from %s — likely a regex/format mismatch",
			len(matches), variable)
	}
	out := make([][]float64, 0, len(matches))
	for _, m := range matches {
		age, err1 := strconv.ParseFloat(m[1], 64)
		qx, err2 := strconv.ParseFloat(m[2], 64)
		if err1 != nil || err2 != nil {
			t.Fatalf("parse error on %v: %v / %v", m, err1, err2)
		}
		out = append(out, []float64{age, qx})
	}
	return out
}

// buildActuarialPVBody returns a JSON request body for an
// actuarial-flavored PV calc. Keeps the test cases below readable
// without macro-ing a huge JSON template inline.
func buildActuarialPVBody(asOfDate string, rate float64,
	fromDate, toDate string, perYr int, amount float64, act string,
	tableName string, dob1 string, asOfNow string, pod float64,
	t *testing.T) string {
	t.Helper()
	tbl := loadSSAQx(t, tableName)
	tblJSON, _ := json.Marshal(tbl)

	body := map[string]interface{}{
		"asOfDate": asOfDate,
		"rate":     rate,
		"periodics": []map[string]interface{}{{
			"fromDate": fromDate,
			"toDate":   toDate,
			"perYr":    perYr,
			"amount":   amount,
			"act":      act,
		}},
		"actuarial": map[string]interface{}{
			"table1":  json.RawMessage(tblJSON),
			"dob1":    dob1,
			"asOfNow": asOfNow,
			"pod":     pod,
		},
	}
	bts, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal actuarial body: %v", err)
	}
	return string(bts)
}

// Actuarial Example 1: Valuing a Lifetime Pension. 65-year-old male,
// $2,000/mo for life, 5% discount. Help promises only "significantly
// less than the non-contingent present value." This test pins the
// actual numbers and verifies the complementarity property the help
// advertises (Living + Dead = non-contingent), which is the cheapest
// useful regression signal for actuarial code.
func TestVerifyWebActuarial_LifetimePension(t *testing.T) {
	const asOf = "2024-01-01"
	const dob = "1959-01-01" // 65 years before asOf
	const through = "2059-01-01" // age 100

	// Non-contingent baseline (no actuarial section, no Life code).
	plainBody := `{
		"asOfDate": "` + asOf + `",
		"rate": 0.05,
		"periodics": [
			{"fromDate": "` + asOf + `", "toDate": "` + through + `", "perYr": 12, "amount": 2000}
		]
	}`
	plain := callPV(t, plainBody)
	if plain.Error != "" {
		t.Fatalf("plain baseline error: %s", plain.Error)
	}

	livingBody := buildActuarialPVBody(asOf, 0.05, asOf, through, 12, 2000, "L",
		"SSA_2021_MALE_QX", dob, asOf, 0, t)
	living := callPV(t, livingBody)
	if living.Error != "" {
		t.Fatalf("living error: %s", living.Error)
	}

	deadBody := buildActuarialPVBody(asOf, 0.05, asOf, through, 12, 2000, "D",
		"SSA_2021_MALE_QX", dob, asOf, 0, t)
	dead := callPV(t, deadBody)
	if dead.Error != "" {
		t.Fatalf("dead error: %s", dead.Error)
	}

	t.Logf("Actuarial Lifetime Pension (65M, $2k/mo, 5%%):")
	t.Logf("  non-contingent SumValue = %.2f", plain.SumValue)
	t.Logf("  Living SumValue         = %.2f  (%.1f%% of non-contingent)",
		living.SumValue, 100*living.SumValue/plain.SumValue)
	t.Logf("  Dead SumValue           = %.2f", dead.SumValue)
	t.Logf("  Living + Dead           = %.2f  (complementarity target = %.2f)",
		living.SumValue+dead.SumValue, plain.SumValue)

	// Pin help-documented numbers exactly (rounded to nearest dollar):
	//   non-contingent = $397,763, Living = $253,135, Dead = $144,628.
	if !approxEqual(plain.SumValue, 397762.85, 1.0) {
		t.Errorf("non-contingent PV = %.2f, help documents 397,763", plain.SumValue)
	}
	if !approxEqual(living.SumValue, 253135.24, 1.0) {
		t.Errorf("Living PV = %.2f, help documents 253,135", living.SumValue)
	}
	if !approxEqual(dead.SumValue, 144627.62, 1.0) {
		t.Errorf("Dead PV = %.2f, help documents 144,628", dead.SumValue)
	}
	// Complementarity: Living + Dead ≈ non-contingent. This is a
	// mathematical identity, not just a help-text promise — if it
	// breaks, the engine has a real bug regardless of the documented
	// numbers.
	gap := math.Abs((living.SumValue + dead.SumValue) - plain.SumValue)
	if gap > 0.01 {
		t.Errorf("complementarity broken: Living + Dead = %.2f, non-contingent = %.2f, gap = %.2f",
			living.SumValue+dead.SumValue, plain.SumValue, gap)
	}
}

// Actuarial Example 2: Wrongful Death with Payment on Death.
// 40-year-old, $5,000/mo lost support to age 67 (Living contingency),
// plus $50,000 POD payable at death. 4% discount. Pins the SumValue,
// PODValue, and the relationship between them (POD should be a
// fraction of $50k since not everyone dies in the projection window).
func TestVerifyWebActuarial_WrongfulDeathPOD(t *testing.T) {
	const asOf = "2024-01-01"
	const dob = "1984-01-01"      // 40 years before asOf
	const through = "2051-01-01"  // age 67, retirement

	body := buildActuarialPVBody(asOf, 0.04, asOf, through, 12, 5000, "L",
		"SSA_2021_MALE_QX", dob, asOf, 50000, t)
	resp := callPV(t, body)
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}

	t.Logf("Actuarial Wrongful Death (40M, $5k/mo to age 67, 4%%, $50k POD):")
	t.Logf("  SumValue (lost support, Living)  = %.2f", resp.SumValue)
	t.Logf("  PODValue (expected death benefit)= %.2f", resp.PODValue)

	// Pin help-documented numbers exactly:
	//   SumValue (lost support, Living) = $959,540
	//   PODValue (expected death benefit) = $12,617
	if !approxEqual(resp.SumValue, 959539.54, 1.0) {
		t.Errorf("SumValue = %.2f, help documents 959,540", resp.SumValue)
	}
	if !approxEqual(resp.PODValue, 12617.34, 1.0) {
		t.Errorf("PODValue = %.2f, help documents 12,617", resp.PODValue)
	}
}

// Quick compile-time use of fmt so unused-import never bites if I
// later remove the only fmt-using line.
var _ = fmt.Sprintf
