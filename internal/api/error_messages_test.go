// Handler-layer error-message tests.
//
// These cover request-level failures — the cases where the API cannot
// even hand the request to a finance engine because a date cell is
// unparseable, a required field is blank, or the supplied fields are
// insufficient to count a term. The engine-layer error wording is
// covered by error_messages_test.go in each finance/ package; this
// file is the handler complement.
//
// Each assertion checks two things: (1) the message names the field
// or row that is wrong, and (2) it carries an actionable suggestion
// (the help text that tells the user what to do next). The audit
// goal — see docs/dispatch_gaps.md §0.10 — is that no "cannot
// calculate" error is a bare diagnostic; every one explains the fix.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errAmort runs a body through HandleAmortizationCalc and returns the
// decoded response. Unlike the shared postAmort helper, it does not
// require a 200 status — these tests exercise the 400 error paths.
func errAmort(t *testing.T, body string) AmortizationResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)
	var resp AmortizationResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	return resp
}

// errPV runs a body through HandlePVCalc and returns the decoded
// response, tolerating the 400 error paths these tests target.
func errPV(t *testing.T, body string) PVResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandlePVCalc(w, req)
	var resp PVResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	return resp
}

// wantContains fails the test unless err contains every required
// substring (case-insensitive). Used to assert both the field name
// and the suggestion phrase are present.
func wantContains(t *testing.T, err string, parts ...string) {
	t.Helper()
	if err == "" {
		t.Fatalf("expected an error, got none")
	}
	low := strings.ToLower(err)
	for _, p := range parts {
		if !strings.Contains(low, strings.ToLower(p)) {
			t.Errorf("error %q missing expected fragment %q", err, p)
		}
	}
}

// --- Amortization handler: date-cell parse failures ---------------

func TestHandlerAmortLoanDateUnparseable(t *testing.T) {
	resp := errAmort(t, `{
		"amount": 100000, "loanDate": "01-32-2024", "rate": 0.06,
		"firstDate": "2024-02-01", "nPeriods": 12, "perYr": 12
	}`)
	wantContains(t, resp.Error, "Loan Date", "MM/DD/YYYY")
}

func TestHandlerAmortFirstDateUnparseable(t *testing.T) {
	resp := errAmort(t, `{
		"amount": 100000, "loanDate": "2024-01-01", "rate": 0.06,
		"firstDate": "not-a-date", "nPeriods": 12, "perYr": 12
	}`)
	wantContains(t, resp.Error, "1st Pmt Date", "MM/DD/YYYY")
}

func TestHandlerAmortLastDateUnparseable(t *testing.T) {
	resp := errAmort(t, `{
		"amount": 100000, "loanDate": "2024-01-01", "rate": 0.06,
		"firstDate": "2024-02-01", "lastDate": "garbage", "perYr": 12
	}`)
	wantContains(t, resp.Error, "Last Pmt Date", "MM/DD/YYYY")
}

func TestHandlerAmortMoratoriumDateUnparseable(t *testing.T) {
	resp := errAmort(t, `{
		"amount": 100000, "loanDate": "2024-01-01", "rate": 0.06,
		"firstDate": "2024-02-01", "nPeriods": 12, "perYr": 12,
		"moratorium": "xx/xx/xxxx"
	}`)
	wantContains(t, resp.Error, "Moratorium Date", "MM/DD/YYYY")
}

// Skip Months is a free-text field; an unparseable string must be
// rejected with both the field name and an example of valid syntax.
func TestHandlerAmortSkipMonthsInvalid(t *testing.T) {
	resp := errAmort(t, `{
		"amount": 100000, "loanDate": "2024-01-01", "rate": 0.06,
		"firstDate": "2024-02-01", "nPeriods": 12, "perYr": 12,
		"skipMonths": "6,13"
	}`)
	wantContains(t, resp.Error, "Skip Months", "6-8,12")
}

// --- Amortization derive-only handler -----------------------------

// Derive-only mode (Amount and Rate both blank) still needs Pmts/Yr
// to count periods between two dates; omitting it must explain why.
func TestHandlerAmortDeriveOnlyMissingPerYr(t *testing.T) {
	resp := errAmort(t, `{
		"firstDate": "2024-02-01", "lastDate": "2054-01-01"
	}`)
	wantContains(t, resp.Error, "Pmts/Yr", "per year")
}

// Derive-only with only a loanDate — not enough siblings to count a
// term. The message must name the alternative inputs that would work.
func TestHandlerAmortDeriveOnlyInsufficient(t *testing.T) {
	resp := errAmort(t, `{
		"loanDate": "2024-01-01", "perYr": 12
	}`)
	wantContains(t, resp.Error, "insufficient", "# Periods", "Last Pmt Date")
}

// --- Mortgage What-If handler -------------------------------------

// A What-If table needs a positive row count; zero/negative cannot
// produce a grid, so the handler must say what the field controls.
func TestHandlerMortgageWhatIfBadCount(t *testing.T) {
	body := `{"vary": "rate", "count": 0, "rate": 0.06}`
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/whatif",
		bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandleMortgageWhatIf(w, req)
	var resp MortgageWhatIfResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	wantContains(t, resp.Error, "row count", "positive")
}

// --- Present Value handler: date-cell parse failures --------------

func TestHandlerPVAsOfDateUnparseable(t *testing.T) {
	resp := errPV(t, `{
		"asOfDate": "31/31/2024", "rate": 0.05,
		"lumpSums": [{"date": "2025-01-01", "amount": 1000}]
	}`)
	wantContains(t, resp.Error, "As-of Date", "MM/DD/YYYY", "discounted")
}

func TestHandlerPVLumpSumDateUnparseable(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"lumpSums": [{"date": "soon", "amount": 1000}]
	}`)
	wantContains(t, resp.Error, "Lump Sum row 1", "Date", "MM/DD/YYYY")
}

func TestHandlerPVPeriodicFromDateUnparseable(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"periodics": [{"fromDate": "later", "toDate": "2026-01-01",
			"perYr": 12, "amount": 100}]
	}`)
	wantContains(t, resp.Error, "Periodic row 1", "From Date", "MM/DD/YYYY")
}

func TestHandlerPVPeriodicToDateUnparseable(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"periodics": [{"fromDate": "2025-01-01", "toDate": "never",
			"perYr": 12, "amount": 100}]
	}`)
	wantContains(t, resp.Error, "Periodic row 1", "To Date", "MM/DD/YYYY")
}

// Variable-rate schedule: a row with no date cannot anchor a rate
// change, so the handler must name the row and say what the date is.
func TestHandlerPVRateScheduleDateMissing(t *testing.T) {
	resp := errPV(t, `{
		"lumpSums": [{"date": "2026-01-01", "amount": 1000}],
		"rateSchedule": [{"trueRate": 0.05}]
	}`)
	wantContains(t, resp.Error, "Variable-rate schedule row 1", "Date")
}

func TestHandlerPVRateScheduleDateUnparseable(t *testing.T) {
	resp := errPV(t, `{
		"lumpSums": [{"date": "2026-01-01", "amount": 1000}],
		"rateSchedule": [{"date": "bad", "trueRate": 0.05}]
	}`)
	wantContains(t, resp.Error, "Variable-rate schedule row 1", "MM/DD/YYYY")
}

// --- Present Value handler: actuarial (life-contingency) config ---

// Actuarial mode needs Person 1's table, date of birth, and the
// as-of date. Omitting them must name all three and offer the
// fall-back of removing the actuarial settings.
func TestHandlerPVActuarialMissingInputs(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"lumpSums": [{"date": "2030-01-01", "amount": 1000}],
		"actuarial": {"table1": [], "dob1": "", "asOfNow": ""}
	}`)
	wantContains(t, resp.Error, "life table", "date of birth", "as-of")
}

func TestHandlerPVActuarialDOBUnparseable(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"lumpSums": [{"date": "2030-01-01", "amount": 1000}],
		"actuarial": {"table1": [[65,0.01]], "dob1": "long ago",
			"asOfNow": "2026-01-01"}
	}`)
	wantContains(t, resp.Error, "Person 1", "Date of Birth", "MM/DD/YYYY")
}

func TestHandlerPVActuarialAsOfUnparseable(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"lumpSums": [{"date": "2030-01-01", "amount": 1000}],
		"actuarial": {"table1": [[65,0.01]], "dob1": "1960-01-01",
			"asOfNow": "whenever"}
	}`)
	wantContains(t, resp.Error, "As-of", "MM/DD/YYYY")
}

// --- Present Value handler: life contingency without a config -----
//
// A row's Life column selects a mortality contingency, but the survival
// weighting is only applied when an actuarial block is present
// (presentvalue/calc.go gates LifeProb on Actuarial != nil). A contingent
// row with no config would therefore be valued as if non-contingent — a
// silent wrong answer. The handler must reject it instead. These tests pin
// that contract on the backend so it holds even when the request does not
// come from the web UI (which separately disables/blocks the same cases).

// fullQxTableJSON returns a JSON [[age,qx],...] table spanning ages 0..120
// (terminal qx = 1) so a contingency calc has full age coverage.
func fullQxTableJSON() string {
	rows := make([][2]float64, 121)
	for i := range rows {
		rows[i] = [2]float64{float64(i), 0.001 + 0.0001*float64(i)}
	}
	rows[120][1] = 1
	b, _ := json.Marshal(rows)
	return string(b)
}

// A lump-sum row asking for "Living" with no actuarial block at all must be
// rejected, naming Person 1, the reference date, and the None fall-back.
func TestHandlerPVContingencyWithoutConfig(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"lumpSums": [{"date": "2030-01-01", "amount": 1000, "act": "L"}]
	}`)
	wantContains(t, resp.Error, "life contingency", "Person 1", "reference date", "None")
}

// Same contract for a periodic row using "Dead".
func TestHandlerPVPeriodicContingencyWithoutConfig(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"periodics": [{"fromDate":"2030-01-01","toDate":"2040-01-01","perYr":12,"amount":100,"act":"D"}]
	}`)
	wantContains(t, resp.Error, "life contingency", "Person 1", "None")
}

// A two-life contingency ("Both") with only Person 1 configured must be
// rejected, naming the two-life options and Person 2.
func TestHandlerPVTwoLifeWithoutPerson2(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"lumpSums": [{"date": "2030-01-01", "amount": 1000, "act": "B"}],
		"actuarial": {"table1": `+fullQxTableJSON()+`, "dob1": "1960-01-01",
			"asOfNow": "2026-01-01"}
	}`)
	wantContains(t, resp.Error, "two-life", "Person 2")
}

// Positive control: a "Living" row with a complete Person 1 config must NOT
// trip the contingency guard — the calculation should run.
func TestHandlerPVContingencyWithConfigOK(t *testing.T) {
	resp := errPV(t, `{
		"rate": 0.05,
		"asOfDate": "2026-01-01",
		"lumpSums": [{"date": "2030-01-01", "amount": 1000, "act": "L"}],
		"actuarial": {"table1": `+fullQxTableJSON()+`, "dob1": "1960-01-01",
			"asOfNow": "2026-01-01"}
	}`)
	if resp.Error != "" {
		t.Fatalf("expected success for Living + full Person 1 config, got error: %q", resp.Error)
	}
}

// Positive control: a two-life "Both" row with both persons configured must
// also run cleanly.
func TestHandlerPVTwoLifeWithPerson2OK(t *testing.T) {
	tbl := fullQxTableJSON()
	resp := errPV(t, `{
		"rate": 0.05,
		"asOfDate": "2026-01-01",
		"lumpSums": [{"date": "2030-01-01", "amount": 1000, "act": "B"}],
		"actuarial": {
			"table1": `+tbl+`, "dob1": "1960-01-01",
			"table2": `+tbl+`, "dob2": "1962-06-15",
			"asOfNow": "2026-01-01"}
	}`)
	if resp.Error != "" {
		t.Fatalf("expected success for Both + Person 1 & 2 configured, got error: %q", resp.Error)
	}
}
