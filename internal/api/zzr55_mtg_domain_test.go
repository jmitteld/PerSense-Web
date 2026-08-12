package api

// Round-46 mortgage audit FINDING 5, closed r55: the five per-cell domain
// validators of MortgageScreenUnit.pas:1202-1250, which the port never had.
//
// 🚨 EVERY ASSERTION IS AT THE CONSUMER (R42) — the real handler, the real
// decoder, the real JSON. And every one is checked in BOTH DIRECTIONS (rule 3):
// a value just OUTSIDE the domain must be refused with DOS's own message, and
// the value just INSIDE it must still be ANSWERED. A one-sided test here would
// pass against a handler that refused everything.
//
// 🚨 AND ON ALL THREE ENDPOINTS. The decoder is shared, and R75 (round 54) was
// written because a change to it silently regressed an endpoint whose tests
// never ran the new path.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mtgPost(t *testing.T, h http.HandlerFunc, path string, body any) (int, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, req)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: response is not JSON (%v): %s", path, err, w.Body.String())
	}
	return w.Code, out
}

// mtgDomainRow is a fully-specified, in-domain mortgage row on the WIRE
// (fractions for rate/pctDown/points — getMtgRowData divides the typed percent
// by 100). Every case below starts from this and perturbs ONE field.
func mtgDomainRow() map[string]any {
	return map[string]any{
		"price": 200000.0, "pctDown": 0.2, "years": 30, "rate": 0.06,
	}
}

// the three endpoints, each wrapping one row in its own request shape.
var mtgDomainEndpoints = []struct {
	name string
	h    http.HandlerFunc
	path string
	wrap func(map[string]any) any
}{
	{"calc", HandleMortgageCalc, "/api/mortgage/calc",
		func(r map[string]any) any { return r }},
	{"compare", HandleMortgageCompare, "/api/mortgage/compare",
		func(r map[string]any) any {
			return map[string]any{"a": r, "b": mtgDomainRow()}
		}},
	{"whatif", HandleMortgageWhatIf, "/api/mortgage/whatif",
		func(r map[string]any) any {
			return map[string]any{"base": r, "vary": "rate",
				"increment": 0.005, "count": 3}
		}},
}

func TestR55MortgageCellDomainsRefuseOutOfDomain(t *testing.T) {
	// field, out-of-domain value, DOS's message. The values are the exact
	// boundary DOS rejects (>= hi, or < lo) plus one well past it — the
	// boundary case is the one a sloppy `>` would let through.
	cases := []struct {
		field string
		bad   any
		msg   string
	}{
		// Points: DoubleVal >= 10 or < 0, in PERCENT -> >= 0.10 or < 0 on wire.
		{"points", 0.10, "Points must be between 0 and 10"},
		{"points", 1.5, "Points must be between 0 and 10"},
		{"points", -0.01, "Points must be between 0 and 10"},
		// %Down: >= 100 or < -9 (percent) -> >= 1.00 or < -0.09 on wire.
		{"pctDown", 1.00, "Percent down must be between -9 and 100"},
		{"pctDown", 1.5, "Percent down must be between -9 and 100"},
		{"pctDown", -0.10, "Percent down must be between -9 and 100"},
		// Years: >= 100 or < 0. A plain count, no scaling.
		{"years", 100, "Years must be between 0 and 100"},
		{"years", 500, "Years must be between 0 and 100"},
		{"years", -5, "Years must be between 0 and 100"},
		// Loan rate: >= 100 or <= -100 (percent) -> >= 1.00 or <= -1.00.
		{"rate", 1.00, "The loan rate must be between -100 and 100"},
		{"rate", 6.0, "The loan rate must be between -100 and 100"},
		{"rate", -1.00, "The loan rate must be between -100 and 100"},
		// Balloon years: >= 99 or < 0. ⚠️ DOS's own MESSAGE says "0 and 100"
		// while its CHECK is 99; both are reproduced exactly.
		{"balloonYears", 99, "Balloon Years must be between 0 and 100"},
		{"balloonYears", 400, "Balloon Years must be between 0 and 100"},
		{"balloonYears", -2, "Balloon Years must be between 0 and 100"},
	}
	for _, ep := range mtgDomainEndpoints {
		for _, c := range cases {
			t.Run(fmt.Sprintf("%s/%s=%v", ep.name, c.field, c.bad), func(t *testing.T) {
				row := mtgDomainRow()
				row[c.field] = c.bad
				code, out := mtgPost(t, ep.h, ep.path, ep.wrap(row))
				if code != http.StatusBadRequest {
					t.Fatalf("%s %s=%v: status = %d, want 400 (the port "+
						"accepted a value DOS's VerifyCellString rejects)",
						ep.path, c.field, c.bad, code)
				}
				got, _ := out["error"].(string)
				if got != c.msg {
					t.Errorf("%s %s=%v: error = %q, want %q",
						ep.path, c.field, c.bad, got, c.msg)
				}
			})
		}
	}
}

// THE OTHER DIRECTION, AND IT IS NOT OPTIONAL. Every value just INSIDE the
// domain must still be ANSWERED. Without this the refusal test above passes
// against a handler that refuses every request, which is exactly the vacuous
// green r54's finding-2 test shipped with.
func TestR55MortgageCellDomainsAcceptInDomain(t *testing.T) {
	cases := []struct {
		field string
		good  any
	}{
		{"points", 0.0}, {"points", 0.0999}, {"points", 0.015},
		{"pctDown", 0.0}, {"pctDown", 0.999}, {"pctDown", -0.09},
		{"years", 0}, {"years", 99}, {"years", 30},
		{"rate", 0.0}, {"rate", 0.999}, {"rate", -0.999},
		{"balloonYears", 0}, {"balloonYears", 98}, {"balloonYears", 5},
	}
	for _, ep := range mtgDomainEndpoints {
		for _, c := range cases {
			t.Run(fmt.Sprintf("%s/%s=%v", ep.name, c.field, c.good), func(t *testing.T) {
				row := mtgDomainRow()
				row[c.field] = c.good
				// A balloon YEAR alone leaves the row underdetermined for an
				// APR, so /compare refuses it for want of data — which made
				// every balloonYears subtest on that endpoint prove nothing
				// (audit 2, finding 6). Give the balloon an AMOUNT so the row
				// is determinate and the acceptance is a real acceptance. This
				// is test DATA, not a relaxation: the assertion below is
				// unchanged.
				if c.field == "balloonYears" {
					row["balloonAmount"] = 50000.0
				}
				code, out := mtgPost(t, ep.h, ep.path, ep.wrap(row))
				msg, _ := out["error"].(string)
				if isMtgDomainMessage(msg) {
					t.Fatalf("%s %s=%v: REFUSED by the domain validator "+
						"(%q) — this value is INSIDE DOS's domain and the "+
						"fix has over-reached (R67)",
						ep.path, c.field, c.good, msg)
				}
				// 🚨 AND RECORD WHETHER IT ACTUALLY ANSWERED. Audit pass 2,
				// finding 6: this arm only ever asserted "not refused by the
				// DOMAIN validator", so a subtest whose row the ENGINE refuses
				// for an unrelated reason passed without demonstrating that any
				// in-domain value is accepted. On /compare that was true of
				// EVERY balloonYears value, so a wrong balloonYears bound would
				// not have been caught there at all. An answer is a 200 with NO
				// error field — /calc returns 200 CARRYING an error for years=0,
				// which the old `code != 400` guard scored as a pass.
				if code == http.StatusOK && msg == "" {
					mtgInDomainAnswered(ep.name, c.field)
				}
			})
		}
	}
}

// isMtgDomainMessage reports whether an error came from validateMtgDomains
// rather than from the engine. The in-domain test must not be satisfied by an
// engine refusal that happens to carry a 400 — R54, a "pass" that means
// "nothing was compared".
func isMtgDomainMessage(msg string) bool {
	for _, m := range []string{
		"Points must be between 0 and 10",
		"Percent down must be between -9 and 100",
		"Years must be between 0 and 100",
		"The loan rate must be between -100 and 100",
		"Balloon Years must be between 0 and 100",
	} {
		if msg == m {
			return true
		}
	}
	return false
}

// POSITIVE CONTROL THAT THE IN-DOMAIN ARM IS NOT VACUOUS. The baseline row
// must ANSWER on every endpoint — if it did not, every subtest in
// TestR55MortgageCellDomainsAcceptInDomain would pass without exercising
// anything (R49: a control can be inert because the sample cannot express the
// difference).
func TestR55MortgageDomainBaselineAnswers(t *testing.T) {
	for _, ep := range mtgDomainEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			code, out := mtgPost(t, ep.h, ep.path, ep.wrap(mtgDomainRow()))
			if code != http.StatusOK {
				t.Fatalf("%s: baseline in-domain row got %d, want 200 — the "+
					"in-domain arm of this file is measuring nothing", ep.path, code)
			}
			if msg, _ := out["error"].(string); msg != "" {
				t.Fatalf("%s: baseline row carried error %q", ep.path, msg)
			}
		})
	}
}

// AND THE UNITS ARE THE HAZARD. The wire is a FRACTION and the screen is a
// PERCENT; a validator written in the wrong space would refuse every real
// screen (rate 0.0725) or accept every broken one (rate 7.25). Pin both.
func TestR55MortgageDomainUnitsAreWireFractions(t *testing.T) {
	realScreen := mtgDomainRow()
	realScreen["rate"] = 0.0725 // 7.25% — the round-46 audit's own example row
	realScreen["points"] = 0.015
	if code, out := mtgPost(t, HandleMortgageCalc, "/api/mortgage/calc",
		realScreen); code != http.StatusOK {
		t.Fatalf("a 7.25%%/1.5pt screen was refused (%d, %v) — the validator "+
			"is in PERCENT space and the wire is FRACTIONS", code, out["error"])
	}
	typedAsPercent := mtgDomainRow()
	typedAsPercent["rate"] = 7.25 // 725% — out of domain on the wire
	if code, _ := mtgPost(t, HandleMortgageCalc, "/api/mortgage/calc",
		typedAsPercent); code != http.StatusBadRequest {
		t.Fatalf("rate 7.25 on the WIRE is 725%% and must be refused, got %d", code)
	}
}

// --- audit pass 2, finding 6: the in-domain arm's own positive control ---

var mtgAnswered = map[string]bool{}

func mtgInDomainAnswered(endpoint, field string) {
	mtgAnswered[endpoint+"/"+field] = true
}

// TestR55MortgageInDomainArmIsNotVacuous fails if any (endpoint, field) pair
// went through TestR55MortgageCellDomainsAcceptInDomain without ONE in-domain
// value actually being ANSWERED. Without this, that test passes vacuously for a
// field the engine happens to refuse on every value — which is exactly what it
// was doing for balloonYears on /compare.
//
// 🚨 IT DEPENDS ON THE ACCEPT TEST HAVING RUN. Go runs tests in file order
// within a package, and this file declares the accept test above; under
// -shuffle that is not guaranteed, so the guard SKIPS rather than lying when
// the map is empty — a skip is not a pass (R12), and it says so.
func TestR55MortgageInDomainArmIsNotVacuous(t *testing.T) {
	if len(mtgAnswered) == 0 {
		t.Skip("TestR55MortgageCellDomainsAcceptInDomain did not run before " +
			"this guard (test order); nothing to verify")
	}
	fields := []string{"points", "pctDown", "years", "rate", "balloonYears"}
	var missing []string
	for _, ep := range mtgDomainEndpoints {
		for _, f := range fields {
			if !mtgAnswered[ep.name+"/"+f] {
				missing = append(missing, ep.name+"/"+f)
			}
		}
	}
	if len(missing) > 0 {
		t.Errorf("no in-domain value was ANSWERED for %d (endpoint,field) "+
			"pairs, so the accept arm proves nothing for them: %v",
			len(missing), missing)
	}
}
