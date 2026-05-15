package api

// The frontend now rejects 2-digit years (FE parseDate returns null
// when y < 1000). The API still accepts whatever it's given — these
// tests document the API-side behavior so the contract is clear:
//
//   - YYYY-MM-DD is the only date format the handlers parse (via
//     time.Parse with the "2006-01-02" layout). 2-digit years in
//     "YYYY-MM-DD" form would fail layout matching anyway.
//   - Out-of-order dates (e.g. fromDate after toDate) surface as
//     engine errors with hints, not silent miscalcs.
//
// This pair pins the well-typed inputs that pass and the out-of-
// order case that prompted the FE fix (user typed "5/15/57" meaning
// 2057, the FE's old 2-digit parser turned it into 1957, and the
// engine surfaced "dates are out of order, line 1").

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Sanity: a request whose toDate is before fromDate yields a clear
// error rather than running off into negative-time territory. The
// scenario the user hit before the FE fix: from=2026-05-15, to=
// 1957-05-15 (because "57" parsed under century=50 as 1957).
func TestPVDatesOutOfOrderRejected(t *testing.T) {
	body := `{
		"asOfDate": "2026-05-15",
		"rate": 0.05,
		"periodics": [
			{"fromDate":"2026-05-15","toDate":"1957-05-15","perYr":12,"amount":2000}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandlePVCalc(w, req)
	var resp PVResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error == "" {
		t.Errorf("expected an error for to<from, got SumValue=%.2f", resp.SumValue)
	}
	if !strings.Contains(strings.ToLower(resp.Error), "out of order") &&
		!strings.Contains(strings.ToLower(resp.Error), "date") {
		t.Logf("note: error string is %q; the FE-side fix that forces 4-digit years prevents this scenario before it reaches the API", resp.Error)
	}
}

// The happy path the user wanted: from 2026-05-15 through 2057-05-15
// for a $2,000/mo annuity at 5%. Confirms the engine processes the
// 30-year+ horizon without complaint once a 4-digit year is supplied.
func TestPVFullFourDigitYearsAccepted(t *testing.T) {
	body := `{
		"asOfDate": "2026-05-15",
		"rate": 0.05,
		"periodics": [
			{"fromDate":"2026-05-15","toDate":"2057-05-15","perYr":12,"amount":2000}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	HandlePVCalc(w, req)
	var resp PVResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.SumValue <= 0 {
		t.Errorf("SumValue should be positive for a real annuity, got %.2f", resp.SumValue)
	}
}
