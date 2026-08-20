package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPVPerYrReachesTheKicker pins round 61's fix for the Present Value
// screen's "Default payments per year" setting.
//
// THE DEFECT (r61): pvInputFromRequest built PVSettings with the LITERAL
// PerYr: 12. PVRequest had no perYr field at all, so nothing the user chose
// could reach the engine — while the on-screen settings strip happily printed
// the chosen value. DOS reads df.c.peryr at every PV rate column
// (INTSUTIL.pas:1581-1590 lratecol, :1591-1606 yieldcol, :1622-1634 vaprcol).
//
// 🚨 WHERE IT CAN BE SEEN, AND WHERE IT CANNOT. pvKickerRate is the IDENTITY
// unless the basis is 365/360. So this test's whole power lives on that
// basis, and the 360/365 cases below are NEGATIVE controls: they must NOT
// move, or the change has leaked somewhere it does not belong.
//
// SEEN TO FAIL: with PerYr pinned back to 12 in pvInputFromRequest, the
// 365/360 subtest fails (both perYr values return the identical sumValue),
// which is exactly the defect.
func pvPost(t *testing.T, body string) PVResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandlePVCalc(w, req)
	var resp PVResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("engine refused: %s (body %s)", resp.Error, body)
	}
	return resp
}

func TestPVPerYrReachesTheKicker(t *testing.T) {
	// One periodic series, one lump sum, a rate the kicker can bite on.
	bodyFor := func(basis string, perYr int) string {
		b := ""
		if basis != "360" {
			b = `"basis":"` + basis + `",`
		}
		p := ""
		if perYr != 12 {
			p = fmt.Sprintf(`"perYr":%d,`, perYr)
		}
		return `{"asOfDate":"2024-01-01","rate":0.06,` + b + p +
			`"lumpSums":[{"date":"2034-01-01","amount":50000}],` +
			`"periodics":[{"fromDate":"2024-02-01","toDate":"2044-01-01","perYr":12,"amount":1000,"cola":0}]}`
	}

	for _, basis := range []string{"360", "365", "365/360"} {
		basis := basis
		t.Run("basis="+basis, func(t *testing.T) {
			base := pvPost(t, bodyFor(basis, 12))
			alt := pvPost(t, bodyFor(basis, 1))

			same := math.Abs(base.SumValue-alt.SumValue) < 1e-9
			if basis == "365/360" {
				if same {
					t.Fatalf("365/360: perYr=1 and perYr=12 produced the SAME sumValue %.10f — "+
						"the setting is not reaching the kicker (this is the r61 defect)", base.SumValue)
				}
				t.Logf("365/360 moved: perYr=12 -> %.6f, perYr=1 -> %.6f", base.SumValue, alt.SumValue)
			} else {
				if !same {
					t.Fatalf("basis %s: perYr moved the answer (%.10f vs %.10f) — the kicker is the "+
						"IDENTITY off 365/360, so nothing here should depend on perYr", basis, base.SumValue, alt.SumValue)
				}
			}
		})
	}
}

// TestPVPerYrRejectsOutOfRange pins the validation added with the field, so a
// hostile or buggy caller cannot overflow the byte conversion silently.
//
// 🚨 128 IS THE ONE THAT MATTERS AND THE FIRST VERSION OF THIS TEST MISSED IT.
// An adversarial audit found that perYr=128 — INSIDE the 1..255 range this
// test was probing the outside of — returned an EMPTY HTTP 200: it is the bare
// CompoundingCanadian flag, RealPerYr strips it to 0, the kicker divides by
// zero, the rate becomes NaN and json.Marshal writes nothing. A range check is
// not a validity check, and probing only outside the range proved only that
// the range check exists.
func TestPVPerYrRejectsOutOfRange(t *testing.T) {
	for _, n := range []int{-1, 256, 100000, 128} {
		body := fmt.Sprintf(`{"asOfDate":"2024-01-01","rate":0.06,"perYr":%d,`+
			`"lumpSums":[{"date":"2034-01-01","amount":50000}]}`, n)
		req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		HandlePVCalc(w, req)
		var resp PVResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Error == "" {
			t.Fatalf("perYr=%d was accepted; expected a refusal", n)
		}
	}
}

// TestPVPerYrNeverReturnsAnEmptyBody is the general form of the 128 case: for
// EVERY frequency in the accepted range the handler must write a decodable JSON
// body — either a result or a refusal, never nothing. An empty 200 is the
// failure mode a status-code check cannot see.
func TestPVPerYrNeverReturnsAnEmptyBody(t *testing.T) {
	for n := 1; n <= 255; n++ {
		body := fmt.Sprintf(`{"asOfDate":"2024-01-01","rate":0.06,"basis":"365/360","perYr":%d,`+
			`"lumpSums":[{"date":"2030-01-01","amount":100000}]}`, n)
		req := httptest.NewRequest(http.MethodPost, "/api/presentvalue/calc", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		HandlePVCalc(w, req)
		if w.Body.Len() == 0 {
			t.Fatalf("perYr=%d produced an EMPTY body with status %d", n, w.Code)
		}
		var resp PVResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("perYr=%d produced an undecodable body: %v", n, err)
		}
		if resp.Error == "" && math.IsNaN(resp.SumValue) {
			t.Fatalf("perYr=%d returned NaN as a result", n)
		}
	}
}
