package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAmortFullAdvancedMoratoriumTargetMatchesDOS — 2026-07-24 client-reported
// divergence #4 ("this appeared when I entered a moratorium and principal
// minimum"). ADJUDICATED: the port is correct on BOTH the APR and the payoff
// balance; the two screenshots were not the same loan (web Pmt Amount $1,200.00
// vs DOS Payment 750.00). Guarded here so the every-advanced-option-at-once
// configuration — which is the densest one we have — can never drift silently.
//
// Inputs (the web screenshot verbatim): $100,000, loan 01/01/2025, rate 8.0050%,
// 1st pmt 02/01/2025, 360 periods, last 01/01/2055, 12/yr, payment $1,200.00,
// points 0, basis 365/360 + Exact + Prepaid + Arrears + InclReg (⇒ plus_regular
// off). Advanced: two prepayment series (03/01/2028 → 05/01/2033, 24/yr, $150;
// 06/01/2040 → 06/01/2044, 52/yr, $75), a balloon 06/15/2031 $33,333, two
// rate/payment adjustments (01/01/2030 → 10% / $1,000; 06/05/2035 → 3% / $750),
// a moratorium (interest-only) to 06/01/2030, principal minimum $300, and skip
// months 1,3,5. Payoff balance requested as of 06/01/2035.
//
// ORACLE PROVENANCE (real DOS units, `b365_360 exact prepaid`, adjustment rates
// carried through the §44 kicker; tokens mor=65 targ=300 skip=1,3,5
// bdate=15.6.2031:33333 predmy=… adjdmy=… payoff=1.6.2035):
//
//	pay 1200 (the web screenshot)  → apr 0.070967  payoff 128470.8134
//	                                 interest 158833.34  paid 258833.34
//	                                 tack-on balloon 1/1/2055  +4,662.15
//	pay  750 (the DOS screenshot)  → apr 0.072832  payoff 115719.7128
//	                                 tack-on balloon 1/1/2055 −15,913.07
//
// Every one of those matches its screenshot to the digit, on both sides. The
// adjustment-2 date difference between the shots (06/05/2035 vs 6/1/35) moves
// the APR by less than 1e-6 — the payment amount is the whole gap.
func TestAmortFullAdvancedMoratoriumTargetMatchesDOS(t *testing.T) {
	body := `{
	  "amount":100000, "loanDate":"2025-01-01", "rate":0.08005,
	  "firstDate":"2025-02-01", "nPeriods":360, "perYr":12,
	  "payment":1200, "points":0,
	  "basis":"365/360", "exact":true, "balloonIncludesRegular":true,
	  "prepayments":[
	    {"startDate":"2028-03-01","stopDate":"2033-05-01","perYr":24,"amount":150},
	    {"startDate":"2040-06-01","stopDate":"2044-06-01","perYr":52,"amount":75}
	  ],
	  "balloons":[{"date":"2031-06-15","amount":33333}],
	  "adjustments":[
	    {"date":"2030-01-01","rate":0.10,"amount":1000},
	    {"date":"2035-06-05","rate":0.03,"amount":750}
	  ],
	  "moratorium":"2030-06-01",
	  "targetAmt":300,
	  "skipMonths":"1,3,5",
	  "payoffDate":"2035-06-01"
	}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	HandleAmortizationCalc(w, req)

	var m struct {
		APR              float64 `json:"apr"`
		PayoffBalance    float64 `json:"payoffBalance"`
		PayoffValid      bool    `json:"payoffValid"`
		TotalPaid        float64 `json:"totalPaid"`
		TotalInt         float64 `json:"totalInterest"`
		PrepayResolvedNN []int   `json:"prepayResolvedNN"`
		Error            string  `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Error != "" {
		t.Fatalf("error: %s", m.Error)
	}

	// APR — the web screenshot's 7.0967%, = oracle 0.070967.
	if m.APR < 0.0709665 || m.APR > 0.0709675 {
		t.Errorf("APR = %.6f, want 0.070967 (oracle, pay=1200)", m.APR)
	}
	// Payoff balance as of 06/01/2035 — the screenshot's $128,470.81.
	if !m.PayoffValid {
		t.Errorf("payoffValid = false, want a computed balance")
	}
	if m.PayoffBalance < 128470.75 || m.PayoffBalance > 128470.87 {
		t.Errorf("payoffBalance = %.2f, want 128470.81 (oracle 128470.8134)", m.PayoffBalance)
	}
	// Totals — the screenshot's Total Paid $258,833.34 / Total Interest $158,833.34.
	if m.TotalInt < 158833.29 || m.TotalInt > 158833.39 {
		t.Errorf("totalInterest = %.2f, want 158833.34 (oracle)", m.TotalInt)
	}
	if m.TotalPaid < 258833.29 || m.TotalPaid > 258833.39 {
		t.Errorf("totalPaid = %.2f, want 258833.34 (oracle)", m.TotalPaid)
	}
	// The derived # Pmts DOS shows in the grid for the two stop-date-bounded
	// series (DOS "#of Pds" column: 360 / 125 / 209).
	if len(m.PrepayResolvedNN) != 2 || m.PrepayResolvedNN[0] != 125 || m.PrepayResolvedNN[1] != 209 {
		t.Errorf("prepayResolvedNN = %v, want [125 209] (DOS grid)", m.PrepayResolvedNN)
	}
	t.Logf("APR=%.6f payoff=%.2f totalInt=%.2f totalPaid=%.2f nn=%v",
		m.APR, m.PayoffBalance, m.TotalInt, m.TotalPaid, m.PrepayResolvedNN)
}
