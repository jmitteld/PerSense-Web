package api

// End-to-end tests for the Variable Rate Schedule plumbing: HTTP
// handler → engine → response. Engine-level math is already covered
// by internal/finance/presentvalue/variablerate_test.go; the focus
// here is on the request/response shape and the dispatch from
// HandlePVCalc into the variable-rate path.

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

// VR with a single rate-schedule entry should match a regular
// fixed-rate request to the dollar. Same property the engine test
// asserts, but checked here at the API boundary.
func TestPVVariableRate_SingleEntryMatchesFixedRate(t *testing.T) {
	fixed := `{
		"asOfDate": "2024-01-01",
		"rate": 0.08,
		"lumpSums": [{"date": "2025-01-01", "amount": 10000}]
	}`
	vr := `{
		"asOfDate": "2024-01-01",
		"lumpSums": [{"date": "2025-01-01", "amount": 10000}],
		"rateSchedule": [{"date": "1900-01-01", "trueRate": 0.08}]
	}`

	fixedResp := postPV(t, fixed)
	vrResp := postPV(t, vr)

	t.Logf("fixed SumValue=%.6f, vr SumValue=%.6f",
		fixedResp.SumValue, vrResp.SumValue)
	if math.Abs(fixedResp.SumValue-vrResp.SumValue) > 1e-6 {
		t.Errorf("VR with single entry diverged: %.6f vs %.6f",
			vrResp.SumValue, fixedResp.SumValue)
	}
}

// IRS-style use case from the help text: tax interest at different
// rates over different periods. $100,000 owed, paid 3 years later
// under three different yearly rates. Hand-computed integral.
func TestPVVariableRate_IRSStyleTaxInterest(t *testing.T) {
	body := `{
		"asOfDate": "2024-01-01",
		"lumpSums": [{"date": "2027-01-01", "amount": 100000}],
		"rateSchedule": [
			{"date": "1900-01-01", "trueRate": 0.05},
			{"date": "2025-01-01", "trueRate": 0.07},
			{"date": "2026-01-01", "trueRate": 0.10}
		]
	}`
	resp := postPV(t, body)
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	// integral = 0.05 + 0.07 + 0.10 = 0.22 → PV = 100000 × exp(-0.22)
	want := 100000 * math.Exp(-0.22)
	t.Logf("PV=%.4f, want %.4f", resp.SumValue, want)
	if math.Abs(resp.SumValue-want) > 0.01 {
		t.Errorf("PV = %.4f, want %.4f", resp.SumValue, want)
	}
}

// Periodic payments in VR mode: $1,000/month for 3 years across two
// rate regimes. Each payment integrated through the schedule. The
// expected value is just the sum of 36 individually-discounted
// payments; computed inline to keep the test self-contained.
func TestPVVariableRate_PeriodicAcrossRateChange(t *testing.T) {
	body := `{
		"asOfDate": "2024-01-01",
		"lumpSums": [],
		"periodics": [
			{"fromDate": "2024-02-01", "toDate": "2027-01-01",
			 "perYr": 12, "amount": 1000}
		],
		"rateSchedule": [
			{"date": "1900-01-01", "trueRate": 0.06},
			{"date": "2025-06-01", "trueRate": 0.04}
		]
	}`
	resp := postPV(t, body)
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}

	// Hand-replicate the integration. Under Basis360, every month is
	// 1/12 year. The rate is 0.06 from 2024-01-01 to 2025-06-01 and
	// 0.04 thereafter.
	want := 0.0
	for k := 0; k < 36; k++ {
		// Payment date: 2024-02-01 + k months. Years from as-of:
		yrs := (1.0 + float64(k)) / 12.0
		// Rate change at 2025-06-01 = year 17/12 from as-of.
		const switchYrs = 17.0 / 12.0
		var integral float64
		if yrs <= switchYrs {
			integral = 0.06 * yrs
		} else {
			integral = 0.06*switchYrs + 0.04*(yrs-switchYrs)
		}
		want += 1000 * math.Exp(-integral)
	}
	t.Logf("got=%.4f, want=%.4f, diff=%.4f", resp.SumValue, want, resp.SumValue-want)
	if math.Abs(resp.SumValue-want) > 0.01 {
		t.Errorf("PV = %.4f, want %.4f", resp.SumValue, want)
	}
}

// Backward calc rejection. VR mode must not accept a row with a
// missing amount — DOS doesn't support solving in VR mode and we
// surface a clear error rather than silently returning zero.
func TestPVVariableRate_BackwardSolveRejected(t *testing.T) {
	body := `{
		"asOfDate": "2024-01-01",
		"lumpSums": [{"date": "2025-01-01"}],
		"rateSchedule": [{"date": "1900-01-01", "trueRate": 0.05}]
	}`
	resp := postPV(t, body)
	if resp.Error == "" {
		t.Errorf("expected error for missing amount in VR mode, got SumValue=%.4f", resp.SumValue)
	}
	t.Logf("got expected error: %s", resp.Error)
}

// End-to-end: variable rate × actuarial through the HTTP boundary.
// Pension valuation under a piecewise-changing discount rate — the
// scenario the combined feature is designed for (e.g. a structured
// settlement where statutory discount rates step by year). Asserts
// complementarity (Living + Dead = non-contingent) through the API.
func TestPVVariableRate_WithActuarial_Complementarity(t *testing.T) {
	// Realistic SSA-style table parsed from lifetables.js (the test
	// helper in verify_web_help_examples_test.go owns this) — but
	// since this is a separate file we inline a tiny qx ladder to
	// stay independent. It doesn't need to be SSA-accurate; the
	// complementarity invariant holds for any nonnegative qx with
	// terminal qx=1.
	qxRows := make([][2]float64, 121)
	for i := 0; i < 121; i++ {
		qx := 0.001 + 0.0001*float64(i)*float64(i)/120
		if qx > 1 {
			qx = 1
		}
		qxRows[i] = [2]float64{float64(i), qx}
	}
	qxRows[120][1] = 1

	tableJSON, _ := json.Marshal(qxRows)

	make := func(act string) string {
		return `{
			"asOfDate": "2024-01-01",
			"lumpSums": [],
			"periodics": [
				{"fromDate":"2024-01-01","toDate":"2054-01-01","perYr":12,"amount":2000,"act":"` + act + `"}
			],
			"rateSchedule": [
				{"date":"1900-01-01","trueRate":0.04},
				{"date":"2030-01-01","trueRate":0.06},
				{"date":"2040-01-01","trueRate":0.08}
			],
			"actuarial": {
				"table1": ` + string(tableJSON) + `,
				"dob1": "1959-01-01",
				"asOfNow": "2024-01-01"
			}
		}`
	}

	plain := postPV(t, make("N"))
	living := postPV(t, make("L"))
	dead := postPV(t, make("D"))
	for _, r := range []struct {
		name string
		resp PVResponse
	}{{"plain", plain}, {"living", living}, {"dead", dead}} {
		if r.resp.Error != "" {
			t.Fatalf("%s: %s", r.name, r.resp.Error)
		}
	}
	t.Logf("VR×actuarial: plain=%.2f  living=%.2f  dead=%.2f  living+dead=%.2f",
		plain.SumValue, living.SumValue, dead.SumValue,
		living.SumValue+dead.SumValue)

	gap := math.Abs((living.SumValue + dead.SumValue) - plain.SumValue)
	if gap > 0.01 {
		t.Errorf("complementarity broken in VR×actuarial: living+dead=%.4f, plain=%.4f, gap=%.4f",
			living.SumValue+dead.SumValue, plain.SumValue, gap)
	}
	if living.SumValue >= plain.SumValue {
		t.Errorf("Living PV %.2f should be < non-contingent %.2f (mortality weighting must drag it down)",
			living.SumValue, plain.SumValue)
	}
	if dead.SumValue <= 0 {
		t.Errorf("Dead PV %.2f should be > 0 — the early-break bug would zero it",
			dead.SumValue)
	}
}

// End-to-end: POD under variable rates. With both an actuarial
// config carrying POD > 0 AND a non-trivial rate schedule, the
// response should include a positive PODValue. (The fixed-rate
// path already pins this number in verify_web_help_examples_test
// for the Wrongful Death example; this test ensures the VR variant
// engages the same code path and integrates the schedule.)
func TestPVVariableRate_WithActuarial_PODIntegrated(t *testing.T) {
	qxRows := make([][2]float64, 121)
	for i := 0; i < 121; i++ {
		qx := 0.001 + 0.0001*float64(i)*float64(i)/120
		if qx > 1 {
			qx = 1
		}
		qxRows[i] = [2]float64{float64(i), qx}
	}
	qxRows[120][1] = 1
	tableJSON, _ := json.Marshal(qxRows)

	body := `{
		"asOfDate": "2024-01-01",
		"lumpSums": [
			{"date":"2024-01-02","amount":0.01}
		],
		"rateSchedule": [
			{"date":"1900-01-01","trueRate":0.04},
			{"date":"2040-01-01","trueRate":0.08}
		],
		"actuarial": {
			"table1": ` + string(tableJSON) + `,
			"dob1": "1984-01-01",
			"asOfNow": "2024-01-01",
			"pod": 50000
		}
	}`
	resp := postPV(t, body)
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	t.Logf("VR PODValue=%.2f", resp.PODValue)
	if resp.PODValue <= 0 {
		t.Errorf("VR PODValue=%.2f should be positive — POD integration didn't engage under VR mode",
			resp.PODValue)
	}
	if resp.PODValue > 50000 {
		t.Errorf("VR PODValue=%.2f exceeds face POD ($50,000) — expected value can't be greater than the death benefit",
			resp.PODValue)
	}
}

// postPV is a focused helper — verify-style tests in this package
// already have a callPV, but it's defined in verify_web_help_examples_test.go
// with its own conventions. Use a separate one here to keep the test
// files independent and avoid cross-file coupling.
func postPV(t *testing.T, body string) PVResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandlePVCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp PVResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}
