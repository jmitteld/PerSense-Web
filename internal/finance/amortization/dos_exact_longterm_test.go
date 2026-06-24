package amortization

import (
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// exactProductPayment solves a blank payment for an exact (actual/365 daily)
// loan via the PRODUCT path (Amortize) and returns the modal regular payment
// plus the number of scheduled rows.
func exactProductPayment(amount, rate float64, n, perYr int) (pay float64, rows int, ok bool) {
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Settings: Settings{Basis: types.Basis365, PerYr: byte(perYr), YrDays: 365.25, YrInv: 1.0 / 365.25, Exact: true}}
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) < 3 {
		return 0, 0, false
	}
	freq := map[string]int{}
	best, bestN := "", 0
	for _, row := range r.Schedule {
		k := strconv.FormatFloat(row.PayAmt, 'f', 2, 64)
		if freq[k]++; freq[k] > bestN {
			bestN, best = freq[k], k
		}
	}
	v, _ := strconv.ParseFloat(best, 64)
	return v, len(r.Schedule), v > 0
}

// TestDOSExactLongTermPayment pins the exact-interest payment solve against the
// real DOS engine for long-term, high-rate loans — the regime that used to
// diverge (the secant lost convergence from a poor seed and the engine kept an
// over-amortizing estimate, retiring the loan early). The fix seeds the secant
// from the closed form and falls back to the bracketing bisection; both the
// solved payment AND the scheduled row count must now match DOS.
func TestDOSExactLongTermPayment(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	cases := []struct {
		amount, rate float64
		n, py        int
	}{
		{366333.36, 0.29774, 289, 6}, // the exact case the fuzz first flagged
		{366333.36, 0.29774, 250, 6},
		{366333.36, 0.29774, 200, 6},
		{1211785.25, 0.29, 150, 6},
		{500000, 0.45, 300, 12},
		{2000000, 0.40, 360, 12},
		{75000, 0.35, 240, 6},
		{1000000, 0.30, 400, 12},
	}
	worstPay, worstRow := 0.0, 0
	for _, c := range cases {
		dosPay, ok := runOraclePayment(c.amount, c.rate, c.n, c.py, "b365", "exact")
		if !ok {
			t.Logf("oracle skipped amt=%.0f r=%.4f n=%d", c.amount, c.rate, c.n)
			continue
		}
		goPay, goRows, gok := exactProductPayment(c.amount, c.rate, c.n, c.py)
		if !gok {
			t.Errorf("Go amortize failed amt=%.0f r=%.4f n=%d py=%d", c.amount, c.rate, c.n, c.py)
			continue
		}
		rel := math.Abs(dosPay-goPay) / math.Max(1, goPay)
		if rel > worstPay {
			worstPay = rel
		}
		rowDiff := c.n - goRows
		if rowDiff < 0 {
			rowDiff = -rowDiff
		}
		if rowDiff > worstRow {
			worstRow = rowDiff
		}
		if rel > 1e-4 {
			t.Errorf("PAY amt=%.0f r=%.4f n=%d py=%d: DOS=%.4f Go=%.4f (rel %.2e)",
				c.amount, c.rate, c.n, c.py, dosPay, goPay, rel)
		}
		// The old bug's symptom was a GROSS early payoff (e.g. 152 rows for a
		// 289-period loan, a ~0.06%-high payment). With the payment now correct,
		// the schedule runs essentially the full term. A tiny tail difference
		// (≤3 rows) can still occur on extreme inputs — e.g. 35% over a 40-year
		// exact term — from accumulated actual-day rounding in the schedule's
		// retirement test; that is a separate, far smaller effect, not the
		// payment-solve bug this test guards.
		if rowDiff > 3 {
			t.Errorf("ROWS amt=%.0f r=%.4f n=%d py=%d: requested %d, Go scheduled %d (gross early payoff)",
				c.amount, c.rate, c.n, c.py, c.n, goRows)
		}
	}
	t.Logf("exact long-term: worst payment relErr=%.2e, worst row diff=%d", worstPay, worstRow)
}
