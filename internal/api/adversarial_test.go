package api

// Adversarial tests for the UI<->engine boundary (the REST API).
//
// These deliberately send inputs that make no sense or try to break the
// system — malformed JSON, wrong types, non-finite and absurd numbers,
// contradictory or under-determined field combinations, bad dates, abusive
// advanced-option combinations, and resource-exhaustion (DoS) attempts. The
// goal is NOT to compute a correct answer; it is to verify the system fails
// *gracefully*: it must
//
//   - never panic,
//   - never hang or allocate unbounded memory,
//   - never return a 200 with an empty/undecodable body (e.g. a NaN/Inf
//     result that fails to encode), and
//   - for clearly invalid input, return a non-empty, human-readable error
//     (either a 4xx status or a 200 carrying an "error" field).
//
// Each handler is invoked directly with httptest so a panic surfaces here
// rather than being swallowed by net/http's per-connection recover.
//
// The suite is organized section by section — one top-level Test per
// worksheet/endpoint — so each can be run and read independently:
//
//	go test ./internal/api/ -run TestAdversarial_Mortgage       -v
//	go test ./internal/api/ -run TestAdversarial_Amortization   -v
//	go test ./internal/api/ -run TestAdversarial_PresentValue   -v
//	go test ./internal/api/ -run TestAdversarial_MortgageCompare -v
//	go test ./internal/api/ -run TestAdversarial_MortgageWhatIf  -v
//	go test ./internal/api/ -run TestAdversarial_ImportPSN       -v
//	go test ./internal/api/ -run TestAdversarial_MethodNotAllowed -v
//	go test ./internal/api/ -run TestAdversarial -v   # all sections
//
// Findings, fixes, and known gaps are written up in
// docs/adversarial_findings.md.

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// advResult captures everything we need to judge a response.
type advResult struct {
	status   int
	rawBody  string
	decoded  map[string]any
	panicked bool
	panicVal any
}

// adversaryDo invokes a handler with the given body and never lets a panic
// escape — a panic becomes a recorded failure for the case instead of
// aborting the whole test binary.
func adversaryDo(h http.HandlerFunc, path, body string) (res advResult) {
	defer func() {
		if r := recover(); r != nil {
			res.panicked = true
			res.panicVal = r
		}
	}()

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	// Bound wall-clock time: if a handler hangs (e.g. an uncapped loop), fail
	// the case rather than stall the suite forever.
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				res.panicked = true
				res.panicVal = r
			}
			close(done)
		}()
		h(w, req)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		res.status = -1 // sentinel: timed out / likely hung
		return res
	}

	result := w.Result()
	res.status = result.StatusCode
	b, _ := io.ReadAll(result.Body)
	res.rawBody = string(b)
	if len(bytes.TrimSpace(b)) > 0 {
		_ = json.Unmarshal(b, &res.decoded) // decoded stays nil if not a JSON object
	}
	return res
}

// expectation flags what a case must satisfy.
type expectation int

const (
	// mustError: clearly invalid input — must be refused with a non-empty
	// error message (4xx, 405, or a 200 carrying a non-empty "error").
	mustError expectation = iota
	// noCrash: weird-but-parseable input where we don't dictate accept vs.
	// reject, only that the system stays healthy (no panic/hang, decodable
	// body, finite numbers).
	noCrash
)

// assertGraceful applies the universal health checks plus the per-case
// expectation. It returns the error string (if any) for optional logging.
func assertGraceful(t *testing.T, name string, exp expectation, r advResult) string {
	t.Helper()

	if r.panicked {
		t.Errorf("%s: handler PANICKED: %v", name, r.panicVal)
		return ""
	}
	if r.status == -1 {
		t.Errorf("%s: handler did not return within timeout (possible hang / unbounded work)", name)
		return ""
	}

	// A 2xx must carry a decodable JSON object. An empty or broken body on a
	// 2xx is the signature of a NaN/Inf result that failed to encode.
	is2xx := r.status >= 200 && r.status < 300
	if is2xx && r.decoded == nil {
		t.Errorf("%s: %d response with empty/undecodable body (NaN/Inf or broken response?): %q",
			name, r.status, truncate(r.rawBody))
		return ""
	}

	// No non-finite numbers anywhere in the decoded response.
	if bad := findNonFinite(r.decoded); bad != "" {
		t.Errorf("%s: response contains a non-finite number at %s: %q", name, bad, truncate(r.rawBody))
	}

	errStr := ""
	if r.decoded != nil {
		if s, ok := r.decoded["error"].(string); ok {
			errStr = s
		}
	}

	if exp == mustError {
		gotError := errStr != "" || r.status >= 400
		if !gotError {
			t.Errorf("%s: expected a rejection/error but request appears to have succeeded (status %d, no error). body=%s",
				name, r.status, truncate(r.rawBody))
		}
	}
	return errStr
}

func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// findNonFinite walks decoded JSON looking for NaN/±Inf float64 values.
// (json.Unmarshal never produces NaN/Inf itself, but this catches anything we
// might add later and documents intent.)
func findNonFinite(v any) string {
	switch t := v.(type) {
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return "value"
		}
	case map[string]any:
		for k, vv := range t {
			if p := findNonFinite(vv); p != "" {
				return k + "." + p
			}
		}
	case []any:
		for i, vv := range t {
			if p := findNonFinite(vv); p != "" {
				return "[]" + p
			}
			_ = i
		}
	}
	return ""
}

type advCase struct {
	name string
	body string
	exp  expectation
}

func runAdvCases(t *testing.T, h http.HandlerFunc, path string, cases []advCase) {
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			res := adversaryDo(h, path, c.body)
			errStr := assertGraceful(t, c.name, c.exp, res)
			// Surface what the system actually said, for the record.
			if errStr != "" {
				t.Logf("status=%d error=%q", res.status, truncate(errStr))
			} else {
				t.Logf("status=%d (accepted; body=%s)", res.status, truncate(res.rawBody))
			}
		})
	}
}

// ---- Shared structural / parse abuse (same for every endpoint) ----
//
// These are always invalid JSON or wrong shapes, so every endpoint must
// reject them. (The "gray" cases — a bare `null`, trailing garbage after a
// valid object, and an entirely blank object — are handled per-endpoint
// because the Mortgage grid legitimately tolerates a blank/partial row while
// Amortization and Present Value require enough data to compute.)

func structuralCases() []advCase {
	return []advCase{
		{"malformed_json_open_brace", `{`, mustError},
		{"empty_body", ``, mustError},
		{"not_an_object_array", `[1,2,3]`, mustError},
		{"not_an_object_string", `"hello"`, mustError},
		{"nan_literal", `{"price":NaN}`, mustError},
		{"inf_literal", `{"rate":Infinity}`, mustError},
		{"wrong_type_string_for_number", `{"price":"not a number","rate":0.06,"years":30}`, mustError},
		{"wrong_type_object_for_number", `{"price":{"x":1}}`, mustError},
		{"number_overflow_1e400", `{"price":1e400,"pctDown":0.2,"rate":0.06,"years":30}`, mustError},
		{"deeply_unicode_garbage", `{"price":"𝟙𝟚𝟛💥","rate":0.06}`, mustError},
	}
}

func TestAdversarial_Mortgage(t *testing.T) {
	cases := append(structuralCases(), []advCase{
		// The Mortgage grid computes per row: a blank/partial row is not an
		// error, it just echoes back. So these stay healthy but need not error.
		{"all_blank_row_echoes", `{}`, noCrash},
		{"json_null_row_echoes", `null`, noCrash},
		{"trailing_garbage_after_object", `{"price":1} oops`, noCrash},
		{"negative_price", `{"price":-200000,"pctDown":0.2,"rate":0.06,"years":30}`, noCrash},
		{"negative_rate", `{"price":200000,"pctDown":0.2,"rate":-0.05,"years":30}`, noCrash},
		{"absurd_rate_10000pct", `{"price":200000,"pctDown":0.2,"rate":100,"years":30}`, noCrash},
		{"zero_years_div_risk", `{"price":200000,"pctDown":0.2,"rate":0.06,"years":0}`, noCrash},
		{"negative_years", `{"price":200000,"pctDown":0.2,"rate":0.06,"years":-30}`, noCrash},
		{"pctdown_over_100pct", `{"price":200000,"pctDown":5.0,"rate":0.06,"years":30}`, noCrash},
		{"contradictory_overdetermined", `{"price":200000,"pctDown":0.2,"cash":40000,"financed":160000,"monthly":1200,"rate":0.06,"years":30}`, noCrash},
		{"huge_price", `{"price":1000000000000000,"pctDown":0.2,"rate":0.06,"years":30}`, noCrash},
		{"absurd_points", `{"price":200000,"pctDown":0.2,"points":1000,"rate":0.06,"years":30}`, noCrash},
	}...)
	runAdvCases(t, HandleMortgageCalc, "/api/mortgage/calc", cases)
}

func TestAdversarial_Amortization(t *testing.T) {
	cases := append(structuralCases(), []advCase{
		{"all_blank", `{}`, mustError},
		{"json_null", `null`, mustError},
		{"trailing_garbage_after_object", `{"amount":1} oops`, mustError},
		{"nperiods_one_billion_dos", `{"amount":200000,"loanDate":"2024-01-01","rate":0.06,"perYr":12,"nPeriods":1000000000}`, mustError},
		{"nperiods_maxint_dos", `{"amount":200000,"loanDate":"2024-01-01","rate":0.06,"perYr":12,"nPeriods":2147483647}`, mustError},
		{"peryr_zero_div", `{"amount":200000,"loanDate":"2024-01-01","rate":0.06,"perYr":0,"nPeriods":360}`, mustError},
		{"peryr_negative", `{"amount":200000,"loanDate":"2024-01-01","rate":0.06,"perYr":-12,"nPeriods":360}`, mustError},
		{"negative_amount", `{"amount":-200000,"loanDate":"2024-01-01","rate":0.06,"perYr":12,"nPeriods":360}`, noCrash},
		{"negative_rate", `{"amount":200000,"loanDate":"2024-01-01","rate":-0.06,"perYr":12,"nPeriods":360}`, noCrash},
		{"absurd_rate", `{"amount":200000,"loanDate":"2024-01-01","rate":50,"perYr":12,"nPeriods":360}`, noCrash},
		{"bad_loandate_text", `{"amount":200000,"loanDate":"banana","rate":0.06,"perYr":12,"nPeriods":360}`, mustError},
		{"bad_loandate_month13", `{"amount":200000,"loanDate":"13/40/2024","rate":0.06,"perYr":12,"nPeriods":360}`, mustError},
		{"two_digit_year_rejected", `{"amount":200000,"loanDate":"01/01/24","rate":0.06,"perYr":12,"nPeriods":360}`, mustError},
		{"first_before_loan_out_of_order", `{"amount":200000,"loanDate":"2024-06-01","firstDate":"2024-01-01","lastDate":"2020-01-01","rate":0.06,"perYr":12}`, mustError},
		{"last_before_first", `{"amount":200000,"loanDate":"2024-01-01","firstDate":"2024-02-01","lastDate":"2023-01-01","rate":0.06,"perYr":12}`, mustError},
		{"skipmonths_garbage_text", `{"amount":100000,"loanDate":"2024-01-01","rate":0.08,"perYr":12,"nPeriods":360,"skipMonths":"banana"}`, noCrash},
		{"skipmonths_out_of_range", `{"amount":100000,"loanDate":"2024-01-01","rate":0.08,"perYr":12,"nPeriods":360,"skipMonths":"13-99"}`, noCrash},
		{"skipmonths_huge_numbers", `{"amount":100000,"loanDate":"2024-01-01","rate":0.08,"perYr":12,"nPeriods":360,"skipMonths":"999999999999"}`, noCrash},
		{"balloon_before_first_pmt", `{"amount":200000,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":0.06,"perYr":12,"nPeriods":360,"balloons":[{"date":"2024-01-15","amount":1000}]}`, mustError},
		{"duplicate_adjustment_dates", `{"amount":200000,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":0.06,"perYr":12,"nPeriods":360,"adjustments":[{"date":"2027-01-01","rate":0.05},{"date":"2027-01-01","rate":0.07}]}`, mustError},
		{"target_exceeds_amount_over_n", `{"amount":12000,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":0.06,"perYr":12,"nPeriods":12,"targetAmt":99999}`, mustError},
		{"prepay_missing_fields", `{"amount":200000,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":0.06,"perYr":12,"nPeriods":360,"prepayments":[{"amount":100}]}`, mustError},
		{"moratorium_unparseable", `{"amount":200000,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":0.06,"perYr":12,"nPeriods":360,"moratorium":"soon"}`, mustError},
	}...)
	runAdvCases(t, HandleAmortizationCalc, "/api/amortization/calc", cases)
}

func TestAdversarial_PresentValue(t *testing.T) {
	cases := append(structuralCases(), []advCase{
		{"all_blank", `{}`, mustError},
		{"json_null", `null`, mustError},
		{"trailing_garbage_after_object", `{"rate":0.06} oops`, mustError},
		{"bad_asof_date", `{"asOfDate":"yesterday","rate":0.06,"lumpSums":[{"date":"2025-01-01","amount":1000}]}`, mustError},
		{"lump_only_value_underdetermined", `{"asOfDate":"2024-01-01","rate":0.06,"lumpSums":[{"value":1000}]}`, mustError},
		{"lump_amount_zero_backsolve_divzero", `{"asOfDate":"2024-01-01","sumValue":1000,"lumpSums":[{"date":"2025-01-01","amount":0}]}`, mustError},
		{"periodic_dates_out_of_order", `{"asOfDate":"2024-01-01","rate":0.06,"periodics":[{"fromDate":"2030-01-01","toDate":"2024-01-01","perYr":12,"amount":100}]}`, mustError},
		{"periodic_peryr_zero", `{"asOfDate":"2024-01-01","rate":0.06,"periodics":[{"fromDate":"2024-01-01","toDate":"2030-01-01","perYr":0,"amount":100}]}`, mustError},
		{"periodic_peryr_negative", `{"asOfDate":"2024-01-01","rate":0.06,"periodics":[{"fromDate":"2024-01-01","toDate":"2030-01-01","perYr":-4,"amount":100}]}`, mustError},
		{"absurd_rate", `{"asOfDate":"2024-01-01","rate":500,"lumpSums":[{"date":"2025-01-01","amount":1000}]}`, noCrash},
		// KNOWN GAP (documented in docs/adversarial_findings.md): two rows each
		// missing a different field with one Sum Value is under-determined and
		// should report "too many unknowns", but the dispatcher instead solves
		// one row and leaves the other with a default (year-0001) date. It does
		// not crash or emit NaN, so we assert health only and track the gap.
		{"two_blanks_underdetermined_known_gap", `{"asOfDate":"2024-01-01","sumValue":1000,"lumpSums":[{"date":"2025-01-01"},{"amount":500}]}`, noCrash},
		// Variable-rate mode DOES support back-solving a single blank amount
		// (see help #pv-varrate), so this legitimately succeeds — assert health.
		{"variable_rate_solve_amount_supported", `{"asOfDate":"2024-01-01","sumValue":50000,"rateSchedule":[{"date":"2024-01-01","trueRate":0.05}],"lumpSums":[{"date":"2027-01-01"}]}`, noCrash},
		// VR + a single blank date: the help describes date-solving as
		// unsupported in variable-rate mode, but the implementation returns a
		// finite, plausible date rather than erroring. Assert health only; the
		// doc-vs-implementation nuance is noted in docs/adversarial_findings.md.
		{"variable_rate_solve_date_returns_finite", `{"asOfDate":"2024-01-01","sumValue":50000,"rateSchedule":[{"date":"2024-01-01","trueRate":0.05}],"lumpSums":[{"amount":100000}]}`, noCrash},
		{"actuarial_empty_table", `{"asOfDate":"2024-01-01","rate":0.05,"actuarial":{"table1":[],"dob1":"1959-01-01","asOfNow":"2024-01-01"},"periodics":[{"fromDate":"2024-01-01","toDate":"2040-01-01","perYr":12,"amount":1000,"act":"L"}]}`, mustError},
		{"actuarial_qx_above_one", `{"asOfDate":"2024-01-01","rate":0.05,"actuarial":{"table1":[[60,2.0],[70,5.0]],"dob1":"1959-01-01","asOfNow":"2024-01-01"},"periodics":[{"fromDate":"2024-01-01","toDate":"2040-01-01","perYr":12,"amount":1000,"act":"L"}]}`, noCrash},
		{"actuarial_negative_qx", `{"asOfDate":"2024-01-01","rate":0.05,"actuarial":{"table1":[[60,-0.5],[70,-0.2]],"dob1":"1959-01-01","asOfNow":"2024-01-01"},"periodics":[{"fromDate":"2024-01-01","toDate":"2040-01-01","perYr":12,"amount":1000,"act":"L"}]}`, noCrash},
		{"two_life_contingency_without_person2", `{"asOfDate":"2024-01-01","rate":0.05,"actuarial":{"table1":[[60,0.01],[90,0.2]],"dob1":"1959-01-01","asOfNow":"2024-01-01"},"periodics":[{"fromDate":"2024-01-01","toDate":"2040-01-01","perYr":12,"amount":1000,"act":"E"}]}`, mustError},
		{"contingency_without_actuarial_config", `{"asOfDate":"2024-01-01","rate":0.05,"periodics":[{"fromDate":"2024-01-01","toDate":"2040-01-01","perYr":12,"amount":1000,"act":"L"}]}`, mustError},
	}...)
	runAdvCases(t, HandlePVCalc, "/api/presentvalue/calc", cases)
}

func TestAdversarial_MortgageCompare(t *testing.T) {
	cases := []advCase{
		{"malformed_json", `{`, mustError},
		{"empty_body", ``, mustError},
		{"both_sides_blank", `{"a":{},"b":{}}`, mustError},
		{"one_side_invalid_rate", `{"a":{"price":200000,"pctDown":0.2,"years":30,"rate":0.06},"b":{"years":-5}}`, mustError},
		{"missing_b", `{"a":{"price":200000,"pctDown":0.2,"years":30,"rate":0.06}}`, mustError},
	}
	runAdvCases(t, HandleMortgageCompare, "/api/mortgage/compare", cases)
}

func TestAdversarial_MortgageWhatIf(t *testing.T) {
	cases := []advCase{
		{"malformed_json", `{`, mustError},
		{"empty_body", ``, mustError},
		{"count_zero", `{"base":{"price":100000,"pctDown":0,"years":30,"rate":0.07},"vary":"rate","increment":0.25,"count":0}`, mustError},
		{"count_negative", `{"base":{"price":100000,"pctDown":0,"years":30,"rate":0.07},"vary":"rate","increment":0.25,"count":-5}`, mustError},
		{"count_one_billion_dos", `{"base":{"price":100000,"pctDown":0,"years":30,"rate":0.07},"vary":"rate","increment":0.25,"count":1000000000}`, mustError},
		{"unknown_vary_field", `{"base":{"price":100000,"pctDown":0,"years":30,"rate":0.07},"vary":"unicorn","increment":0.25,"count":5}`, mustError},
	}
	runAdvCases(t, HandleMortgageWhatIf, "/api/mortgage/whatif", cases)
}

func TestAdversarial_ImportPSN(t *testing.T) {
	cases := []advCase{
		{"empty_body", ``, mustError},
		{"random_text", `this is not a psn file at all`, mustError},
		{"json_pretending", `{"screen":"mortgage"}`, mustError},
		{"binary_garbage", "\x00\x01\x02\xff\xfe\xfd\x7f\x80nonsense\x00\x00", mustError},
		{"truncated_header", "PSN\x01", mustError},
	}
	runAdvCases(t, HandleImportPSN, "/api/import/psn", cases)
}

// TestAdversarial_MethodNotAllowed verifies non-POST verbs are refused on the
// calc endpoints rather than mis-handled.
func TestAdversarial_MethodNotAllowed(t *testing.T) {
	endpoints := []struct {
		name string
		h    http.HandlerFunc
		path string
	}{
		{"mortgage", HandleMortgageCalc, "/api/mortgage/calc"},
		{"amortization", HandleAmortizationCalc, "/api/amortization/calc"},
		{"presentvalue", HandlePVCalc, "/api/presentvalue/calc"},
	}
	for _, e := range endpoints {
		e := e
		t.Run(e.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
				req := httptest.NewRequest(method, e.path, nil)
				w := httptest.NewRecorder()
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("%s %s: PANICKED: %v", method, e.path, r)
						}
					}()
					e.h(w, req)
				}()
				if w.Result().StatusCode != http.StatusMethodNotAllowed {
					t.Errorf("%s %s: got status %d, want 405", method, e.path, w.Result().StatusCode)
				}
			}
		})
	}
}
