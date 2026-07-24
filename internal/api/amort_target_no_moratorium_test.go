package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAmortTargetMoratoriumCubeMatchesDOS — discrepancies.md §45.
//
// The 2×2 moratorium × principal-minimum cube on the client's dense
// advanced-options loan. Its purpose is twofold:
//
//  1. It pins the C-A-9 fix. Before §45 the `target only` arm (no moratorium)
//     was REJECTED outright by internal/finance/amortization/validate.go with
//     "The principal-reduction Target is too high to be reachable" — because
//     100000/360 = 277.78 < the $300 minimum — while the DOS engine computes
//     it happily. DOS runs that guard ONLY inside
//     `if (h^.lastok) and (DateComp(mor^.first_repay, h^.firstdate) <> 0)`
//     (Amortize.pas:1299), and with no moratorium Amortize.pas:1271-1276 has
//     already defaulted first_repay TO firstdate, so the arm is skipped and
//     `nrepay := h^.nperiods` is taken with no validation at all.
//
//  2. It proves the moratorium and the principal minimum are each genuinely
//     load-bearing here — all four APRs are distinct — so a future change that
//     silently drops either option cannot pass this test.
//
// ORACLE PROVENANCE (real DOS units via legacy/oracle/build_linux.sh):
//
//	amort_oracle 100000 0.08116180555555555 360 12 \
//	  b365_360 exact prepaid loandmy=1.1.2025 firstdmy=1.2.2025 pts=0 \
//	  payhard=1200 predmy=1.3.2028:125:24:150 predmy=1.6.2040:209:52:75 \
//	  bdate=15.6.2031:33333.00 skip=1,3,5 \
//	  adjdmy=1.1.2030:0.10138888888888889:1000 \
//	  adjdmy=5.6.2035:0.030416666666666665:750 [mor=65] [targ=300] apr
//
//	                     APR         interest       paid
//	  neither         0.076886     106,619.33   206,619.33
//	  target only     0.076466     113,600.61   213,600.61
//	  moratorium only 0.066486     176,526.75   276,526.75
//	  both            0.070967     158,833.34   258,833.34
//
// Note the oracle rate argument: the oracle bypasses the app's cell layer, so
// the DISPLAYED 8.0050% must be pre-multiplied by the 365/360 kicker
// (8.0050 × 365/360 = 8.11618%) to reproduce what the app feeds its engine —
// §28 for the loan rate, §44 for the two adjustment rates. Passing the raw
// 8.0050 yields 0.070544 on the `both` arm instead of the screenshot's 7.0967%.
func TestAmortTargetMoratoriumCubeMatchesDOS(t *testing.T) {
	run := func(t *testing.T, mor, targ bool) (apr, interest, paid float64) {
		t.Helper()
		var extra string
		if mor {
			extra += `"moratorium":"2030-06-01",`
		}
		if targ {
			extra += `"targetAmt":300,`
		}
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
		  ],` + extra + `
		  "skipMonths":"1,3,5"
		}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		HandleAmortizationCalc(w, req)
		var m struct {
			APR       float64 `json:"apr"`
			TotalPaid float64 `json:"totalPaid"`
			TotalInt  float64 `json:"totalInterest"`
			Error     string  `json:"error"`
		}
		if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if m.Error != "" {
			t.Fatalf("error: %s", m.Error)
		}
		return m.APR, m.TotalInt, m.TotalPaid
	}

	cases := []struct {
		name          string
		mor, targ     bool
		apr, interest float64
		paid          float64
	}{
		{"neither", false, false, 0.076886, 106619.33, 206619.33},
		// The arm the old unconditional C-A-9 guard rejected outright.
		{"target_only", false, true, 0.076466, 113600.61, 213600.61},
		{"moratorium_only", true, false, 0.066486, 176526.75, 276526.75},
		{"both", true, true, 0.070967, 158833.34, 258833.34},
	}
	seen := map[float64]string{}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			apr, interest, paid := run(t, c.mor, c.targ)
			if math.Abs(apr-c.apr) > 5e-7 {
				t.Errorf("APR = %.6f, want %.6f (oracle)", apr, c.apr)
			}
			if math.Abs(interest-c.interest) > 0.02 {
				t.Errorf("totalInterest = %.2f, want %.2f (oracle)", interest, c.interest)
			}
			if math.Abs(paid-c.paid) > 0.02 {
				t.Errorf("totalPaid = %.2f, want %.2f (oracle)", paid, c.paid)
			}
			if prev, dup := seen[c.apr]; dup {
				t.Errorf("APR %.6f collides with arm %q — the cube would no longer "+
					"prove the option is load-bearing", c.apr, prev)
			}
			seen[c.apr] = c.name
			t.Logf("%s: APR=%.6f interest=%.2f paid=%.2f", c.name, apr, interest, paid)
		})
	}
}
