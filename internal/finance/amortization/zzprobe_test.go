package amortization

import (
	"math"
	"os"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// amortizePayment solves a blank payment via the PRODUCT path (Amortize, the
// same entry the API uses) and returns the regular payment (median schedule row,
// to avoid odd first / adjusted last rows).
func amortizePayment(amount, rate float64, n, perYr int, mod func(*Settings)) (float64, bool) {
	s := Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}
	if mod != nil {
		mod(&s)
	}
	if s.Basis == types.Basis365 {
		s.YrDays = 365.25
		s.YrInv = 1.0 / 365.25
	}
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Settings: s}
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) < 2 {
		return 0, false
	}
	return r.Schedule[len(r.Schedule)/2].PayAmt, true
}

// TestProbeProductPaymentVsDOS confirms the PRODUCT path (Amortize) matches the
// DOS engine for in-advance and exact — the variants where the test-only
// SolvePaymentClosedForm utility (no production callers) diverged. If Amortize matches,
// the divergence is confined to SolvePaymentClosedForm's unrefined estimate.
func TestProbeProductPaymentVsDOS(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skip("oracle not present")
	}
	variants := []struct {
		name  string
		mod   func(*Settings)
		flags []string
	}{
		{"inadvance", func(s *Settings) { s.InAdvance = true }, []string{"inadv"}},
		{"exact365", func(s *Settings) { s.Basis = types.Basis365; s.Exact = true }, []string{"b365", "exact"}},
	}
	for _, v := range variants {
		cases := []struct {
			amount, rate float64
			n, py        int
		}{
			{100000, 0.12, 24, 12}, {500000, 0.06, 60, 12}, {250000, 0.18, 40, 4},
			{1000000, 0.30, 30, 1}, {750000, 0.08, 120, 12},
		}
		worst := 0.0
		for _, c := range cases {
			dos, ok := runOraclePayment(c.amount, c.rate, c.n, c.py, v.flags...)
			if !ok {
				continue
			}
			gp, gok := amortizePayment(c.amount, c.rate, c.n, c.py, v.mod)
			if !gok {
				continue
			}
			rel := math.Abs(dos-gp) / math.Max(1, gp)
			if rel > worst {
				worst = rel
			}
			t.Logf("[%s] amt=%.0f r=%.3f n=%d py=%d: DOS=%.4f Go(Amortize)=%.4f rel=%.2e",
				v.name, c.amount, c.rate, c.n, c.py, dos, gp, rel)
		}
		t.Logf("[%s] PRODUCT-path worst rel = %.2e", v.name, worst)
	}
}

// goSolvePayDated solves the payment with explicit loan + first dates.
func goSolvePayDated(amount, rate float64, n, perYr int, loan, first types.DateRec) (float64, bool) {
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: loan,
		FirstStatus: types.InOutInput, FirstDate: first},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}}
	d, err := SolvePaymentClosedForm(in)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

// TestProbeSemiMonthly isolates whether the py=24 divergence is a real
// semi-monthly bug or an artifact of the degenerate first-date the harness
// formula (1 + 12 div 24 = month 1 = loan date) produces. It pins BOTH engines
// to the SAME explicit, proper semi-monthly first date (15 days after the loan
// date) and compares.
func TestProbeSemiMonthly(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skip("oracle not present")
	}

	type cs struct {
		amount, rate float64
		n            int
	}
	cases := []cs{
		{100000, 0.12, 24}, {500000, 0.06, 48}, {250000, 0.18, 36},
		{1000000, 0.30, 100}, {75000, 0.045, 12},
	}

	// (A) Degenerate first date == loan date (what the fuzz harness used).
	t.Log("--- py=24, firstDate == loanDate (degenerate, as in the fuzz harness) ---")
	for _, c := range cases {
		dos, ok := runOraclePayment(c.amount, c.rate, c.n, 24) // default = loan date
		if !ok {
			continue
		}
		ld := types.NewDateRec(2024, 1, 1)
		gp, gok := goSolvePayDated(c.amount, c.rate, c.n, 24, ld, ld)
		if !gok {
			continue
		}
		rel := math.Abs(dos-gp) / math.Max(1, gp)
		t.Logf("amt=%.0f r=%.3f n=%d: DOS=%.4f Go=%.4f rel=%.2e", c.amount, c.rate, c.n, dos, gp, rel)
	}

	// (B) Proper semi-monthly first date: 15 days after the loan date, pinned
	// identically on BOTH engines via firstdmy=.
	t.Log("--- py=24, firstDate = loanDate + 15 days (proper semi-monthly), pinned both sides ---")
	worst := 0.0
	for _, c := range cases {
		dos, ok := runOraclePayment(c.amount, c.rate, c.n, 24, "firstdmy=16.1.2024")
		if !ok {
			continue
		}
		gp, gok := goSolvePayDated(c.amount, c.rate, c.n, 24,
			types.NewDateRec(2024, 1, 1), types.NewDateRec(2024, 1, 16))
		if !gok {
			continue
		}
		rel := math.Abs(dos-gp) / math.Max(1, gp)
		if rel > worst {
			worst = rel
		}
		t.Logf("amt=%.0f r=%.3f n=%d: DOS=%.4f Go=%.4f rel=%.2e", c.amount, c.rate, c.n, dos, gp, rel)
	}
	t.Logf("proper-first-date worst rel = %.2e", worst)
}
