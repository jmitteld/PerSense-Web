package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAmortARMBalloonReconciliation guards the fancy-engine fix for a rate
// adjustment (ARM) combined with a balloon. Before the fix, "solve payment +
// balloon + rate-only adjustment" kept the balloon-BLIND plain payment as the
// initial payment (the dispatch had no branch for adjustments-with-balloon) and
// the AO5 re-amortization used only the analytic seed, so the schedule did not
// retire — leaving a large residual and total interest off by $12k–$25k vs DOS
// (docs/amort_option_combo_divergences.md; the deferred "task #103" gap).
//
// The fix (engine.go + fancybisect.go):
//   - solves the INITIAL payment balloon-aware at the ORIGINAL rate by stripping
//     the adjustment for the base-payment solve (matching DOS's d_init), and
//   - Iterate-refines the AO5 re-amortized payment so the tail retires, mirroring
//     DOS calling Iterate after the analytic Re_Amortize seed (AMORTOP.pas:1577).
//
// Goldens are the real DOS engine (legacy/oracle/amort_oracle ... plusreg):
//
//	balloon $40k@m120 + ARM→9%@m48 : initial payment 601.53, final balance ~0
//	two balloons $15k@m72/@m192 + ARM→7%@m132 : initial payment 634.82, final ~0
//
// To confirm this is a real guard, temporarily revert engine.go/fancybisect.go:
// the initial payment reverts to 733.76 and the final balance to ~$1,322 / $192,
// failing both assertions.
func TestAmortARMBalloonReconciliation(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantInitial float64 // DOS initial (pre-adjustment) payment
		maxFinalBal float64 // must retire close to zero
	}{
		{
			name: "balloon40k_m120_ARM9pct_m48",
			body: `{"amount":100000,"loanDate":"2024-01-01","rate":0.08,"firstDate":"2024-02-01",
			        "nPeriods":360,"perYr":12,"basis":"360",
			        "balloons":[{"date":"2034-01-01","amount":40000}],
			        "adjustments":[{"date":"2028-01-01","rate":0.09}]}`,
			wantInitial: 601.53,
			maxFinalBal: 5.0,
		},
		{
			name: "two_balloons_ARM7pct_m132",
			body: `{"amount":100000,"loanDate":"2024-01-01","rate":0.08,"firstDate":"2024-02-01",
			        "nPeriods":360,"perYr":12,"basis":"360",
			        "balloons":[{"date":"2030-01-01","amount":15000},{"date":"2040-01-01","amount":15000}],
			        "adjustments":[{"date":"2035-01-01","rate":0.07}]}`,
			wantInitial: 634.82,
			maxFinalBal: 100.0, // DOS retires to 0; the port lands within $100 (was $192)
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
				bytes.NewReader([]byte(c.body)))
			req.Header.Set("Content-Type", "application/json")
			HandleAmortizationCalc(rec, req)

			var resp AmortizationResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("bad response: %v\n%s", err, rec.Body.String())
			}
			if resp.Error != "" {
				t.Fatalf("unexpected error: %s", resp.Error)
			}
			if len(resp.Schedule) == 0 {
				t.Fatal("empty schedule")
			}

			initial := resp.Schedule[0].Payment
			if diff := initial - c.wantInitial; diff < -0.5 || diff > 0.5 {
				t.Errorf("initial payment = %.2f, want DOS %.2f (±0.50). A value near "+
					"733.76 means the balloon-blind base payment regressed.",
					initial, c.wantInitial)
			}

			finalBal := resp.Schedule[len(resp.Schedule)-1].Principal
			if finalBal < -c.maxFinalBal || finalBal > c.maxFinalBal {
				t.Errorf("final balance = %.2f, want |bal| <= %.2f (DOS retires to ~0). "+
					"A value near $1,322 / $192 means the ARM+balloon re-amortization "+
					"regressed.", finalBal, c.maxFinalBal)
			}
		})
	}
}
