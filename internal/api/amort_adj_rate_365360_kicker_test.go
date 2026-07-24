package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regression guard for the "365/360" basis kicker on a RATE ADJUSTMENT's new
// rate (discrepancies.md §44, client-reported 2026-07-24).
//
// ROOT CAUSE. INTSUTIL.pas PercentValueFromCell (1648-1652) puts the adjustment
// new-rate column in the SAME arm as the top-line loan rate column, and exempts
// only the APR column:
//
//	aratecol,adjratecol,aaprcol : begin
//	                              if (df.c.basis=x365_360) and (col<>aaprcol) then
//	                                 PercentValueFromCell:=ReportedRate(rp^)/kicker
//	                              else PercentValueFromCell:=ReportedRate(rp^);
//	                              end;
//
// ReportedRate (1499-1504) is the identity except for Canadian/daily peryr, so
// on the x365_360 basis a DISPLAYED adjustment rate of 10% is stored internally
// as 10 × 365/360 = 10.13889%. HandleAmortizationCalc applied amzKickerRate to
// req.Rate (§28) but passed req.Adjustments[i].Rate through raw, so every ARM
// row on the 365/360 basis ran ~1.4% light.
//
// PROVENANCE. The client's screenshot pair: $100,000, loan 01/01/2025, rate
// 8.0050%, 1st pmt 02/01/2025, 360 periods, 12/yr, payment $750.00, points 0,
// settings "365/360 Act Arr InclReg PrePd" (⇒ basis 365/360 + Exact + Prepaid,
// arrears, plus_regular OFF), one adjustment 01/01/2030 → new rate 10, new
// amount $1,000.00. DOS app reported APR 9.0493 and a terminating balloon of
// −159,554.53 at 1/1/2055; the web reported 8.9703%.
//
// The headless oracle settles it exactly, on the SAME settings, varying only
// the adjustment rate that reaches the engine:
//
//	adj rate 0.10       (raw — the pre-fix web)   → apr 0.089703  ⟵ web's 8.9703%
//	adj rate 0.1013889  (kicked — DOS truth)      → apr 0.090493  ⟵ DOS's 9.0493
//	                                              balloonrow 1/1/2055 −159554.53
//
// Digit-for-digit on both halves of the report. The amortization engine itself
// was already faithful (it matched the oracle across all 24 basis × exact ×
// prepaid × plus_regular combinations); the defect was purely in the request →
// LoanInput conversion.
func TestAmortAdjustmentRate365360KickerMatchesDOS(t *testing.T) {
	apr := func(basis string) float64 {
		body := `{"amount":100000,"loanDate":"2025-01-01","rate":0.08005,` +
			`"firstDate":"2025-02-01","nPeriods":360,"perYr":12,"payment":750,` +
			`"points":0,"exact":true,"basis":"` + basis + `",` +
			`"adjustments":[{"date":"2030-01-01","rate":0.10,"amount":1000}]}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		HandleAmortizationCalc(w, req)
		var m struct {
			APR   float64 `json:"apr"`
			Error string  `json:"error"`
		}
		if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
			t.Fatalf("%s decode: %v", basis, err)
		}
		if m.Error != "" {
			t.Fatalf("%s error: %s", basis, m.Error)
		}
		return m.APR
	}

	// The headline assertion: the DOS app's 9.0493, to the digit the screen shows.
	if got := apr("365/360"); got < 0.0904925 || got > 0.0904935 {
		t.Errorf("365/360 APR = %.6f, want 0.090493 (DOS app 9.0493); "+
			"0.089703 means the adjustment rate is missing the kicker", got)
	}
	// The kicker is basis-scoped: the 360 basis must be untouched by the fix.
	// (Value is the port's own pre-existing 360-basis result — this arm exists to
	// catch an over-broad kicker, not to assert a DOS-app number.)
	base360 := apr("360")
	if base360 <= 0 {
		t.Fatalf("360 APR = %.6f, want a converged positive APR", base360)
	}
	if base360 > 0.0904925 && base360 < 0.0904935 {
		t.Errorf("360 APR = %.6f — the 365/360 kicker leaked onto the 360 basis", base360)
	}
	t.Logf("APR: 365/360=%.6f 360=%.6f", apr("365/360"), base360)
}
