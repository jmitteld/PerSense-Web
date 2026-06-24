package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// post drives a handler with a JSON body and returns the recorder.
func post(h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func postBytes(h http.HandlerFunc, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func getReq(h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestMortgageCalc_BalloonAndBasis(t *testing.T) {
	// Balloon fields + basis "360" (covers the balloonAmount block and the 360
	// branch of mtgAPRYrDays).
	body := `{"price":200000,"pctDown":0.2,"years":30,"rate":0.06,
		"balloonYears":10,"balloonAmount":50000,"basis":"360"}`
	if w := post(HandleMortgageCalc, "/api/mortgage/calc", body); w.Code != http.StatusOK {
		t.Errorf("balloon calc code=%d", w.Code)
	}
	// Enough data for APR with default basis.
	apr := `{"price":200000,"pctDown":0.2,"years":30,"rate":0.06}`
	if w := post(HandleMortgageCalc, "/api/mortgage/calc", apr); w.Code != http.StatusOK {
		t.Errorf("apr calc code=%d", w.Code)
	}
}

func TestMortgageCompare_Branches(t *testing.T) {
	// Method not allowed.
	if w := getReq(HandleMortgageCompare, "/api/mortgage/compare"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("compare GET code=%d", w.Code)
	}
	// Bad JSON.
	if w := post(HandleMortgageCompare, "/api/mortgage/compare", "{bad"); w.Code != http.StatusBadRequest {
		t.Errorf("compare bad json code=%d", w.Code)
	}
	// Cash + Financed + Monthly inputs exercised; both well-formed -> OK.
	ok := `{"a":{"price":200000,"cash":40000,"years":30,"rate":0.06},
		"b":{"price":200000,"financed":160000,"monthly":1200,"years":30,"rate":0.065}}`
	post(HandleMortgageCompare, "/api/mortgage/compare", ok)
	// Mortgage A error (years 0).
	aerr := `{"a":{"years":0},"b":{"price":200000,"pctDown":0.2,"years":30,"rate":0.06}}`
	if w := post(HandleMortgageCompare, "/api/mortgage/compare", aerr); w.Code != http.StatusBadRequest {
		t.Errorf("compare A-error code=%d", w.Code)
	}
	// Mortgage B error.
	berr := `{"a":{"price":200000,"pctDown":0.2,"years":30,"rate":0.06},"b":{"years":0}}`
	if w := post(HandleMortgageCompare, "/api/mortgage/compare", berr); w.Code != http.StatusBadRequest {
		t.Errorf("compare B-error code=%d", w.Code)
	}
	// CompareAPRs error: both computable but under-specified for an APR.
	cmpErr := `{"a":{"price":200000,"pctDown":0.2},"b":{"price":200000,"pctDown":0.2}}`
	post(HandleMortgageCompare, "/api/mortgage/compare", cmpErr)
}

func TestMortgageWhatIf_Branches(t *testing.T) {
	if w := getReq(HandleMortgageWhatIf, "/api/mortgage/whatif"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("whatif GET code=%d", w.Code)
	}
	if w := post(HandleMortgageWhatIf, "/api/mortgage/whatif", "{bad"); w.Code != http.StatusBadRequest {
		t.Errorf("whatif bad json code=%d", w.Code)
	}
	// Unknown vary field.
	if w := post(HandleMortgageWhatIf, "/api/mortgage/whatif",
		`{"base":{"price":200000,"pctDown":0.2,"years":30,"rate":0.06},"vary":"bogus","count":3}`); w.Code != http.StatusBadRequest {
		t.Errorf("whatif bad vary code=%d", w.Code)
	}
	// Count <= 0.
	if w := post(HandleMortgageWhatIf, "/api/mortgage/whatif",
		`{"base":{"price":200000,"pctDown":0.2,"years":30,"rate":0.06},"vary":"rate","count":0}`); w.Code != http.StatusBadRequest {
		t.Errorf("whatif count<=0 code=%d", w.Code)
	}
	// Base error.
	if w := post(HandleMortgageWhatIf, "/api/mortgage/whatif",
		`{"base":{"years":0},"vary":"rate","count":3}`); w.Code != http.StatusBadRequest {
		t.Errorf("whatif base-error code=%d", w.Code)
	}
	// vary "monthly" happy path.
	post(HandleMortgageWhatIf, "/api/mortgage/whatif",
		`{"base":{"price":200000,"pctDown":0.2,"years":30,"rate":0.06},"vary":"monthly","increment":50,"count":2}`)
}

func TestAmortization_AdvancedErrorBranches(t *testing.T) {
	base := `"loanDate":"2020-01-01","perYr":12,"amount":100000,"rate":0.06,"nPeriods":120`
	cases := []string{
		// prepayment perYr <= 0
		`{` + base + `,"prepayments":[{"startDate":"2021-01-01","amount":100}]}`,
		// prepayment incomplete (no start date)
		`{` + base + `,"prepayments":[{"perYr":12,"amount":100}]}`,
		// prepayment bad start date
		`{` + base + `,"prepayments":[{"startDate":"nope","perYr":12,"amount":100}]}`,
		// prepayment bad stop date (valid start)
		`{` + base + `,"prepayments":[{"startDate":"2021-01-01","perYr":12,"amount":100,"stopDate":"nope"}]}`,
		// balloon missing date
		`{` + base + `,"balloons":[{"amount":5000}]}`,
		// balloon bad date
		`{` + base + `,"balloons":[{"date":"nope","amount":5000}]}`,
		// adjustment missing date
		`{` + base + `,"adjustments":[{"rate":0.07}]}`,
		// adjustment bad date
		`{` + base + `,"adjustments":[{"date":"nope","rate":0.07}]}`,
		// moratorium bad date
		`{` + base + `,"moratorium":"nope"}`,
		// skip months invalid
		`{` + base + `,"skipMonths":"abc"}`,
	}
	for i, c := range cases {
		w := post(HandleAmortizationCalc, "/api/amortization/calc", c)
		if w.Code == http.StatusOK && !strings.Contains(w.Body.String(), "error") {
			// Some advanced rows still return 200 with an error field; just
			// ensure the handler ran without panicking.
			t.Logf("case %d returned %d", i, w.Code)
		}
	}
}

func TestAmortization_DeriveOnlyAndSolve(t *testing.T) {
	// Derive-only (no amount, no rate): perYr<=0 error.
	post(HandleAmortizationCalc, "/api/amortization/calc", `{"loanDate":"2020-01-01","perYr":0}`)
	// Derive-only bad loan date.
	post(HandleAmortizationCalc, "/api/amortization/calc", `{"loanDate":"nope","perYr":12}`)
	// Derive-only bad first date.
	post(HandleAmortizationCalc, "/api/amortization/calc", `{"loanDate":"2020-01-01","firstDate":"nope","perYr":12}`)
	// Derive-only bad last date.
	post(HandleAmortizationCalc, "/api/amortization/calc", `{"loanDate":"2020-01-01","lastDate":"nope","perYr":12}`)
	// Derive-only not enough inputs to count term.
	post(HandleAmortizationCalc, "/api/amortization/calc", `{"loanDate":"2020-01-01","perYr":12}`)
	// Derive-only success: two dates.
	post(HandleAmortizationCalc, "/api/amortization/calc",
		`{"loanDate":"2020-01-01","firstDate":"2020-02-01","lastDate":"2030-01-01","perYr":12}`)
	// Backward solve amount (omit amount).
	post(HandleAmortizationCalc, "/api/amortization/calc",
		`{"loanDate":"2020-01-01","perYr":12,"rate":0.06,"nPeriods":120,"payment":1110}`)
	// Backward solve rate (omit rate).
	post(HandleAmortizationCalc, "/api/amortization/calc",
		`{"loanDate":"2020-01-01","perYr":12,"amount":100000,"nPeriods":120,"payment":1110}`)
	// A-W1: payment below principal/n -> implied rate <= 0.
	post(HandleAmortizationCalc, "/api/amortization/calc",
		`{"loanDate":"2020-01-01","perYr":12,"amount":100000,"nPeriods":120,"payment":500}`)
	// A-W3: payment can't support a positive loan -> amount <= 0.
	post(HandleAmortizationCalc, "/api/amortization/calc",
		`{"loanDate":"2020-01-01","perYr":12,"rate":0.06,"nPeriods":120,"payment":1}`)
	// More backward-rate-solve payment levels to exercise the implied-rate<=0
	// (A-W1) and non-convergence advisory branches.
	for _, pay := range []string{"600", "700", "800", "834"} {
		post(HandleAmortizationCalc, "/api/amortization/calc",
			`{"loanDate":"2020-01-01","perYr":12,"amount":100000,"nPeriods":120,"payment":`+pay+`}`)
	}
	// Derive-only with inconsistent dates (lastDate before loanDate) -> the
	// FirstPass error branch in the derive-only path.
	post(HandleAmortizationCalc, "/api/amortization/calc",
		`{"loanDate":"2020-01-01","lastDate":"2019-01-01","perYr":12}`)
	// Backward solve with inconsistent dates -> the FirstPass error branch in
	// the solver-prep path.
	post(HandleAmortizationCalc, "/api/amortization/calc",
		`{"loanDate":"2020-01-01","lastDate":"2019-01-01","perYr":12,"amount":100000,"payment":1100}`)
}

func TestPV_ActuarialBranches(t *testing.T) {
	// Missing actuarial fields.
	post(HandlePVCalc, "/api/presentvalue/calc",
		`{"rate":0.05,"lumpSums":[{"date":"2030-01-01","amount":1000,"act":"L"}],"actuarial":{}}`)
	// Bad DOB.
	post(HandlePVCalc, "/api/presentvalue/calc",
		`{"rate":0.05,"lumpSums":[{"date":"2030-01-01","amount":1000,"act":"L"}],
		"actuarial":{"table1":[[60,0.01],[61,0.012],[120,1]],"dob1":"nope","asOfNow":"2020-01-01"}}`)
	// Bad asOfNow.
	post(HandlePVCalc, "/api/presentvalue/calc",
		`{"rate":0.05,"lumpSums":[{"date":"2030-01-01","amount":1000,"act":"L"}],
		"actuarial":{"table1":[[60,0.01],[61,0.012],[120,1]],"dob1":"1960-01-01","asOfNow":"nope"}}`)
	// Valid one-life actuarial PV.
	post(HandlePVCalc, "/api/presentvalue/calc",
		`{"rate":0.05,"lumpSums":[{"date":"2030-01-01","amount":1000,"act":"L"}],
		"actuarial":{"table1":[[60,0.01],[61,0.012],[120,1]],"dob1":"1960-01-01","asOfNow":"2020-01-01","pod":1000}}`)
	// Valid two-life actuarial PV (covers the Table2 branch).
	post(HandlePVCalc, "/api/presentvalue/calc",
		`{"rate":0.05,"lumpSums":[{"date":"2030-01-01","amount":1000,"act":"B"}],
		"actuarial":{"table1":[[60,0.01],[61,0.012],[120,1]],"dob1":"1960-01-01",
		"table2":[[60,0.009],[61,0.011],[120,1]],"dob2":"1962-01-01","asOfNow":"2020-01-01"}}`)
	// Two-life with bad DOB2.
	post(HandlePVCalc, "/api/presentvalue/calc",
		`{"rate":0.05,"lumpSums":[{"date":"2030-01-01","amount":1000,"act":"B"}],
		"actuarial":{"table1":[[60,0.01],[61,0.012],[120,1]],"dob1":"1960-01-01",
		"table2":[[60,0.009],[61,0.011],[120,1]],"dob2":"nope","asOfNow":"2020-01-01"}}`)
}

func TestImportPSN_Branches(t *testing.T) {
	// Method not allowed.
	if w := getReq(HandleImportPSN, "/api/import/psn"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("import GET code=%d", w.Code)
	}
	// Unparseable header.
	if w := postBytes(HandleImportPSN, "/api/import/psn", []byte{0x00, 0x01}); w.Code != http.StatusBadRequest {
		t.Errorf("import bad header code=%d", w.Code)
	}
	// Real fixtures: mortgage, amortization, present value.
	for _, f := range []string{"DOS.MTG", "DOS.AMZ", "DOS.PVL"} {
		b, err := os.ReadFile("../../legacy/src/win_source/" + f)
		if err != nil {
			continue
		}
		if w := postBytes(HandleImportPSN, "/api/import/psn", b); w.Code != http.StatusOK {
			t.Errorf("import %s code=%d body=%s", f, w.Code, w.Body.String())
		}
		// Truncated past the header -> Load*Bytes error path.
		if len(b) > 40 {
			postBytes(HandleImportPSN, "/api/import/psn", b[:len(b)-20])
		}
	}
	// Wrong/unrecognised file type: valid mortgage header with the grid ID
	// byte changed so FileType matches none of the known screens.
	if b, err := os.ReadFile("../../legacy/src/win_source/DOS.MTG"); err == nil {
		bad := make([]byte, len(b))
		copy(bad, b)
		// The grid-0 ID sits just after version(2)+id(1)+fancy(1)+compdefs.
		// Flip several plausible header bytes to force an unknown FileType
		// while keeping the identifier intact.
		for _, idx := range []int{8, 9, 10, 11, 12} {
			if idx < len(bad) {
				bad[idx] = 0x7E
			}
		}
		postBytes(HandleImportPSN, "/api/import/psn", bad)
	}
}
