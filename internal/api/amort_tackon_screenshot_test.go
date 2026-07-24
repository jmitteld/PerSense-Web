package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAmortFullAdvancedTackOnBalloonMatchesDOSScreenshots — differential
// coverage for DOS's TERMINATING balloon (TackOnFinalBalloon,
// Amortize.pas:1040-1088) on the two loans the client actually photographed in
// the 2026-07-24 moratorium + principal-minimum report (docs/discrepancies.md
// §45), now that the row is surfaced (§46).
//
// The client's DOS screenshot showed a `TackOnFinalBalloon` row 1/1/55
// −15,913.07 in the Balloon Payments grid next to the 6/15/31 33,333.00 the
// user typed. Before §46 the port echoed only the user's balloon, which is what
// made the two screens look different at a glance even where the arithmetic
// agreed. This pins BOTH columns of the §45 adjudication table.
//
// The loan is the densest advanced-options configuration we have: two
// prepayment series, a mid-term balloon, two rate/payment adjustments, a
// moratorium, a principal minimum and skip months, on 365/360 + Exact.
// TestAmortFullAdvancedMoratoriumTargetMatchesDOS pins the pay=1200 column's
// APR / payoff / totals; this test pins the tacked-on balloon on both columns
// and adds the pay=750 (DOS-screenshot) column's APR and payoff, which had no
// coverage at all.
//
// ORACLE PROVENANCE (real DOS units; the displayed 8.0050% and both adjustment
// rates are pre-multiplied by the §28/§44 365/360 kicker because the oracle
// bypasses the app's cell layer; no `plusreg` because the request carries
// balloonIncludesRegular:true):
//
//	amort_oracle 100000 0.08116180555555555 360 12 \
//	  b365_360 exact prepaid loandmy=1.1.2025 firstdmy=1.2.2025 pts=0 \
//	  payhard=<PAY> predmy=1.3.2028:125:24:150 predmy=1.6.2040:209:52:75 \
//	  bdate=15.6.2031:33333.00 skip=1,3,5 mor=65 targ=300 \
//	  adjdmy=1.1.2030:0.10138888888888889:1000 \
//	  adjdmy=5.6.2035:0.030416666666666665:750 bdump
//
//	payhard=1200 (web shot)  nballoons 1
//	  balloonrow 1  6/15/2031  33333.0000  dstatus 3 astatus 3  (inp/inp)
//	  balloonrow 2  1/1/2055    4662.1500  dstatus 1 astatus 1  (outp/outp)
//	  interest 158833.34  paid 258833.34
//	payhard=750 (DOS shot)   nballoons 1
//	  balloonrow 2  1/1/2055  -15913.0700  dstatus 1 astatus 1  (outp/outp)
//	  interest 141902.46  paid 241902.46
//
// `nballoons 1` with a second printed row IS the DOS behaviour under test: the
// row is written into the array and then de-activated by `dec(nballoons)`, so
// BalloonValues2Grid still paints it while MakeTable and the APR never see it.
// The totals and the APR asserted here are therefore the tack-free ones — if
// the tacked row ever leaked into the table or the APR, those would move.
func TestAmortFullAdvancedTackOnBalloonMatchesDOSScreenshots(t *testing.T) {
	cases := []struct {
		name         string
		payment      string
		wantTack     float64 // DOS balloonrow 2 amount at 1/1/2055
		wantAPR      float64
		wantPayoff   float64 // as of 06/01/2035
		wantInt      float64
		wantPaid     float64
		wantFinalPay float64 // DOS `rows` tail: last row's int + prin
		wantFinalInt float64
	}{
		// The web screenshot: Pmt Amount $1,200.00.
		// DOS tail: … row 12/ int 13.65 prin 736.35 bal 4649.97
		//           row  1/ int 12.18 prin 4649.97 bal 0.0000
		{"pay1200_web_shot", "1200", 4662.15, 0.070967, 128470.8134,
			158833.34, 258833.34, 4662.15, 12.18},
		// The DOS screenshot: Payment 750.00. Its residual is NEGATIVE — the
		// schedule overpays, so DOS's terminating row is money the loan has run
		// past rather than money still owed, and the table retires EARLY (599
		// rows) rather than on the terminal date.
		// DOS tail: row 12/ int 1.39 prin 546.81 bal 0.0000
		{"pay750_dos_shot", "750", -15913.07, 0.072832, 115719.7128,
			141902.46, 241902.46, 548.20, 1.39},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{
			  "amount":100000, "loanDate":"2025-01-01", "rate":0.08005,
			  "firstDate":"2025-02-01", "nPeriods":360, "perYr":12,
			  "payment":` + tc.payment + `, "points":0,
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
				APR           float64       `json:"apr"`
				PayoffBalance float64       `json:"payoffBalance"`
				PayoffValid   bool          `json:"payoffValid"`
				TotalPaid     float64       `json:"totalPaid"`
				TotalInt      float64       `json:"totalInterest"`
				Balloons      []BalloonEcho `json:"balloons"`
				Schedule      []struct {
					Date    string  `json:"date"`
					Payment float64 `json:"payment"`
					// `principal` is the engine's POST-payment balance, not the
					// principal portion (see the frontend schedule renderer).
					Principal float64 `json:"principal"`
				} `json:"schedule"`
				Error string `json:"error"`
			}
			if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if m.Error != "" {
				t.Fatalf("error: %s", m.Error)
			}

			// The grid: the user's balloon first, DOS's terminating row after it.
			if len(m.Balloons) != 2 {
				t.Fatalf("balloon echo = %+v, want 2 rows (the user's 6/15/2031 balloon "+
					"and DOS's terminating row at 1/1/2055)", m.Balloons)
			}
			if user := m.Balloons[0]; user.TackedOn || user.Date != "2031-06-15" ||
				math.Abs(user.Amount-33333) > 0.005 {
				t.Errorf("user balloon echo = %+v, want {2031-06-15, 33333, tackedOn=false}", user)
			}
			tack := m.Balloons[1]
			if !tack.TackedOn {
				t.Errorf("second balloon echo = %+v, want tackedOn — this is DOS's "+
					"TackOnFinalBalloon row, not a balloon the user entered", tack)
			}
			if tack.Date != "2055-01-01" {
				t.Errorf("terminating balloon date = %q, want \"2055-01-01\" (DOS lastdate, "+
					"bdump row 2)", tack.Date)
			}
			if math.Abs(tack.Amount-tc.wantTack) > 0.005 {
				t.Errorf("terminating balloon amount = %.4f, want %.2f (DOS bdump row 2) — "+
					"this is the exact figure on the client's screenshot",
					tack.Amount, tc.wantTack)
			}

			// The tacked row must stay OUT of the payment table. Two guards:
			// nothing may be dated past DOS's last date, and the table must end
			// exactly where DOS's does.
			//
			// NOTE for the pay=1200 case: DOS's final scheduled row pays 4662.15
			// — the SAME number as the tacked-on balloon. That is expected, not a
			// leak. TackOnFinalBalloon's amount is the balance owing ON the
			// terminal date (principal + that period's interest), which is
			// precisely what the display fold retires in the last row. DOS shows
			// the figure twice (once paid in the table, once as the de-activated
			// grid row) and counts it ONCE — which is why the totals below, not
			// row-value matching, are the real leak detector.
			if len(m.Schedule) == 0 {
				t.Fatalf("empty schedule")
			}
			for _, r := range m.Schedule {
				if r.Date > "2055-01-01" {
					t.Errorf("schedule runs past DOS's last date with a row at %s; the "+
						"terminating balloon must not extend the table", r.Date)
					break
				}
			}
			last := m.Schedule[len(m.Schedule)-1]
			if math.Abs(last.Payment-tc.wantFinalPay) > 0.005 {
				t.Errorf("final schedule row pays %.2f, want %.2f (DOS `rows` tail: "+
					"int %.2f + prin %.2f, bal 0.0000)", last.Payment, tc.wantFinalPay,
					tc.wantFinalInt, tc.wantFinalPay-tc.wantFinalInt)
			}
			if math.Abs(last.Principal) > 0.005 {
				t.Errorf("final schedule row leaves balance %.4f, want 0.0000 (DOS retires "+
					"the loan in the table; the tacked row is display-only)", last.Principal)
			}

			// APR and totals are the tack-FREE ones (DOS excludes the row from
			// both), so these double as the leak detector.
			if math.Abs(m.APR-tc.wantAPR) > 5e-7 {
				t.Errorf("APR = %.6f, want %.6f (oracle, pay=%s)", m.APR, tc.wantAPR, tc.payment)
			}
			if !m.PayoffValid {
				t.Errorf("payoffValid = false, want a computed balance")
			}
			if math.Abs(m.PayoffBalance-tc.wantPayoff) > 0.06 {
				t.Errorf("payoffBalance = %.4f, want %.4f (oracle, 06/01/2035)",
					m.PayoffBalance, tc.wantPayoff)
			}
			if math.Abs(m.TotalInt-tc.wantInt) > 0.05 {
				t.Errorf("totalInterest = %.2f, want %.2f (oracle)", m.TotalInt, tc.wantInt)
			}
			if math.Abs(m.TotalPaid-tc.wantPaid) > 0.05 {
				t.Errorf("totalPaid = %.2f, want %.2f (oracle)", m.TotalPaid, tc.wantPaid)
			}
			t.Logf("pay=%s APR=%.6f payoff=%.4f int=%.2f paid=%.2f tack=%.2f@%s",
				tc.payment, m.APR, m.PayoffBalance, m.TotalInt, m.TotalPaid,
				tack.Amount, tack.Date)
		})
	}
}
