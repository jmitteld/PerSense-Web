package amortization

import (
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestFancyAPRAdjustmentOffCyclePrepayVsOracle — 2026-07-24 client-reported APR
// discrepancy #2. A screenshot showed the web APR (9.1677%) differing from the DOS
// *app* APR (9.230%) on a loan whose ONLY exotic ingredient beyond the earlier
// full-combo case is that the semi-monthly REPLACE-mode prepayment series starts
// OFF-CYCLE on the 15th (payments are on the 1st), combined with a set-both
// rate/payment adjustment. The client noted the gap appeared only once the
// adjustment was added.
//
// Inputs (exactly the screenshot): $100k, pay 733.76 (hard, rate SOLVED), 360/12,
// 365/360 + Exact + Prepaid, REPLACE mode (BalloonIncludesRegular ⇒ plus_regular
// off). Prepay start 1/15/2026, NN=97, 24/yr, $100. Adjustment 1/1/2030 → rate
// 0.10, payment $1000. Points 0 (explicit) so the APR solver runs.
//
// Conclusion (this test asserts it): the web engine solves the SAME rate the DOS
// engine solves (internal 0.0870707447, displayed 8.5878%) and computes the SAME
// APR the DOS engine computes (0.091677) — to the digit. The loan is heavily
// over-amortized (the APR value walk ends on a ~-252k terminal balloon), and DOS's
// EstimateAndRefineAPRwithPoints → RepayFancyLoan value_calc discounts that
// full-term stream exactly as the port's aprValueCashflows does. So the web is
// APR-faithful to the DOS COMPUTATIONAL ENGINE; the DOS APP's 9.230 is an
// app-vs-engine split (the DOS app diverges from its own engine only when the
// adjustment is present — the same DOS-app stale-state class as §33/§41).
//
// The oracle side uses the `predmy=D.M.Y:...` (off-cycle prepay start) and
// `adjdmy=D.M.Y:RATE:AMOUNT` (absolute-date adjustment) tokens added for this
// investigation, plus loandmy=/firstdmy= so all dates are consistent at loan-year
// 2025 (the pre=/adj= relative-month forms anchor to the oracle's default 2024
// loan date, which would silently shift the schedule by a year).
func TestFancyAPRAdjustmentOffCyclePrepayVsOracle(t *testing.T) {
	gateOracle(t)
	d := func(y, m, dd int) types.DateRec { return types.NewDateRec(y, time.Month(m), dd) }

	base := func() LoanInput {
		return LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 100000,
				LoanDateStatus: types.InOutInput, LoanDate: d(2025, 1, 1),
				LoanRateStatus: types.InOutEmpty, LoanRate: 0, // SOLVE the rate
				FirstStatus: types.InOutInput, FirstDate: d(2025, 2, 1),
				NStatus: types.InOutInput, NPeriods: 360,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus: types.InOutInput, PayAmt: 733.76,
				PointsStatus: types.InOutInput, Points: 0,
			},
			Settings: Settings{Basis: types.Basis365360, PerYr: 12, Exact: true,
				Prepaid: true, PlusRegular: false, YrDays: 360, YrInv: 1.0 / 360},
			Fancy: true,
			Prepayments: []Prepayment{{
				StartDateStatus: types.InOutInput, StartDate: d(2026, 1, 15),
				NNStatus: types.InOutInput, NN: 97,
				PerYrStatus: types.InOutInput, PerYr: 24,
				PaymentStatus: types.InOutInput, Payment: 100,
			}},
		}
	}

	// Mirror the API rate pre-solve: FirstPass a copy, SolveRate, write it back as
	// an input field, then Amortize so applyAPR runs at the solved rate.
	solveAndAmortize := func(t *testing.T, in LoanInput) AmortResult {
		t.Helper()
		si := in
		sl := in.Loan
		if err := FirstPass(&sl); err != nil {
			t.Fatalf("FirstPass: %v", err)
		}
		if sl.FirstStatus > types.StatusEmpty && sl.FirstStatus < types.InOutDefault {
			sl.FirstStatus = types.InOutDefault
		}
		if sl.NPeriods > 0 && sl.NStatus > types.StatusEmpty && sl.NStatus < types.InOutDefault {
			sl.NStatus = types.InOutDefault
		}
		si.Loan = sl
		solved, _, err := SolveRate(si)
		if err != nil {
			t.Fatalf("SolveRate: %v", err)
		}
		in.Loan.LoanRateStatus = types.InOutInput
		in.Loan.LoanRate = solved
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("Amortize: %v", res.Err)
		}
		return res
	}

	oracleArgs := func(adj ...string) []string {
		a := []string{
			"100000", "0.08", "360", "12", "b365_360", "exact", "prepaid",
			"payhard=733.76", "loandmy=1.1.2025", "firstdmy=1.2.2025",
			"predmy=15.1.2026:97:24:100",
		}
		a = append(a, adj...)
		return append(a, "norate", "pts=0", "apr")
	}

	cases := []struct {
		name string
		mut  func(*LoanInput)
		adj  []string
	}{
		{"set_both_rate_and_payment", func(in *LoanInput) {
			in.Adjustments = []RateAdjustment{{DateStatus: types.InOutInput, Date: d(2030, 1, 1),
				LoanRateStatus: types.InOutInput, LoanRate: 0.10,
				AmountStatus: types.InOutInput, Amount: 1000, AmtOK: true}}
		}, []string{"adjdmy=1.1.2030:0.10:1000"}},
		{"no_adjustment", func(in *LoanInput) {}, nil},
		{"rate_only_adjustment", func(in *LoanInput) {
			in.Adjustments = []RateAdjustment{{DateStatus: types.InOutInput, Date: d(2030, 1, 1),
				LoanRateStatus: types.InOutInput, LoanRate: 0.10}}
		}, []string{"adjdmy=1.1.2030:0.10:"}},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			in := base()
			c.mut(&in)
			res := solveAndAmortize(t, in)

			dosAPR, ok := runOracleAPR(t, oracleArgs(c.adj...))
			if !ok {
				t.Skip("oracle produced no APR after retries")
			}
			if diff := res.APR - dosAPR; diff > 1e-4 || diff < -1e-4 {
				t.Errorf("APR: engine=%.6f oracle=%.6f (Δ=%+.6f)", res.APR, dosAPR, diff)
			} else {
				t.Logf("APR: engine=%.6f oracle=%.6f — MATCH", res.APR, dosAPR)
			}

			// Direct guard on the payment re-solve / schedule (§43): the rate-only
			// adjustment re-solves the post-adjustment regular payment, and the bug
			// was that the piecewise segment sub-loan re-applied the WHOLE prepay
			// series (solving 618.83 vs DOS 814.52, TotalPaid 505,608 vs 298,079).
			// Compare total interest to the oracle's totals line to catch a schedule
			// regression the APR (a discounted-value symptom) might mask.
			totalArgs := oracleArgs(c.adj...)
			totalArgs = totalArgs[:len(totalArgs)-1] // drop the trailing "apr" query
			if dosInt, ok := runOracleInterest(t, totalArgs); ok {
				// Relative tolerance: set_both / no_adjustment match to the cent; the
				// rate-only ARM re-solve leaves a ~$2 terminal-fold residual on ~$198k
				// interest (1e-5 relative — the known task #103 ARM-fold gap). The
				// pre-fix structural bug was ~$207k off (100%+), so 1e-4 catches any
				// regression of the prepay-clip fix decisively while tolerating the fold.
				if d := res.TotalInt - dosInt; math.Abs(d) > 1e-4*dosInt+0.02 {
					t.Errorf("TotalInt: engine=%.2f oracle=%.2f (Δ=%+.2f)", res.TotalInt, dosInt, d)
				} else {
					t.Logf("TotalInt: engine=%.2f oracle=%.2f (Δ=%+.2f) — MATCH", res.TotalInt, dosInt, d)
				}
			}
		})
	}
}

// runOracleInterest execs the oracle with a non-query arg set (a solve, no
// apr/solverate token) and parses the total interest from the totals line
// ("payment <p> interest <i> paid <t>"). Mirrors runOracleAPR's retry loop.
func runOracleInterest(t *testing.T, args []string) (float64, bool) {
	t.Helper()
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(string(out)))
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "interest" {
				if v, perr := strconv.ParseFloat(fields[i+1], 64); perr == nil && v != 0 {
					return v, true
				}
			}
		}
	}
	return 0, false
}
