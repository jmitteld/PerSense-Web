package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regression guard for the "365/360" basis kicker on the Amortization payment.
//
// ROOT CAUSE (2026-07-12 audit, discrepancies.md §28): the DOS 365/360 "kicker"
// (kicker = 365/360, PEDATA.pas:141) scales the internal loan rate up by 365/360
// on the x365_360 basis — a UI cell-layer transform (INTSUTIL.pas
// PercentValueFromCell lratecol/x365_360) that the DOS *app* applies but the
// headless oracle bypasses (amort_oracle.pas:50 `loanrate := pRate`). The port
// followed the oracle, so the 365/360 payment was ~1% low. amzKickerRate now
// applies the plain ×365/360 scale to the nominal loan rate at the handler
// boundary; amzUnkickerRate undoes it on the echoed/solved rate.
//
// PROVENANCE (real DOS engine, Amortization screen, basis 365/360, Exact=YES):
// $1,000,000, loan 01/15/2001, rate 8.0000%, 1st pmt 05/01/2001, 360 periods,
// 12/yr → DOS regular payment 7,498.56 (last pmt 04/01/2031). Before the fix the
// port returned 7,419.50. The 360 basis (7,337.65) must be unaffected.
func TestAmortBasis365360KickerMatchesDOS(t *testing.T) {
	pay := func(basis string) float64 {
		body := `{"amount":1000000,"loanDate":"2001-01-15","rate":0.08,"firstDate":"2001-05-01",` +
			`"nPeriods":360,"perYr":12,"basis":"` + basis + `","exact":true}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		HandleAmortizationCalc(w, req)
		var m struct {
			Schedule []struct {
				PayNum  int     `json:"payNum"`
				Payment float64 `json:"payment"`
			} `json:"schedule"`
			Error string `json:"error"`
		}
		json.NewDecoder(w.Body).Decode(&m)
		if m.Error != "" {
			t.Fatalf("%s error: %s", basis, m.Error)
		}
		for _, r := range m.Schedule {
			if r.PayNum == 1 {
				return r.Payment
			}
		}
		t.Fatalf("%s: no regular payment row", basis)
		return 0
	}

	if p := pay("365/360"); p < 7498.555 || p > 7498.565 {
		t.Errorf("365/360 payment = %.2f, want 7498.56 (DOS app)", p)
	}
	if p := pay("360"); p < 7337.645 || p > 7337.655 {
		t.Errorf("360 payment = %.2f, want 7337.65 (unchanged by the kicker)", p)
	}
}
