package amortization

import (
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// goAmortizePay solves a blank payment via the PRODUCT path (Amortize — the same
// entry the API uses), returning the regular payment (median schedule row).
// Faithful Settings per basis + computational flags.
func goAmortizePay(amount, rate float64, n, perYr int, mod func(*Settings)) (float64, bool) {
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
	if r.Err != nil || len(r.Schedule) < 3 {
		return 0, false
	}
	// The regular payment is the modal PayAmt across the schedule; first (odd)
	// and last (adjusted) rows can differ, especially on actual-day bases, so a
	// single-row readback is unreliable for short terms.
	freq := map[string]int{}
	var bestKey string
	bestN := 0
	for _, row := range r.Schedule {
		k := strconv.FormatFloat(row.PayAmt, 'f', 2, 64)
		freq[k]++
		if freq[k] > bestN {
			bestN, bestKey = freq[k], k
		}
	}
	v, _ := strconv.ParseFloat(bestKey, 64)
	if v <= 0 {
		return 0, false
	}
	return v, true
}

// TestFuzzAmortizePaymentVsDOS is an aggressive differential fuzz of the PRODUCT
// payment-solve path (Amortize) against the real DOS engine across a wide
// parameter space and every computational-setting variant. perYr=24 is excluded
// because the shared test date formula (1 + 12 div 24 = month 1) collapses the
// first-payment date onto the loan date — a degenerate zero-length first period
// that is not a realistic input (see TestProbeSemiMonthly).
func TestFuzzAmortizePaymentVsDOS(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skip("oracle not present")
	}
	variants := []struct {
		name  string
		mod   func(*Settings)
		flags []string
		tol   float64
	}{
		{"ordinary360", nil, nil, 1e-4},
		{"b365", func(s *Settings) { s.Basis = types.Basis365 }, []string{"b365"}, 3e-4},
		{"b365_360", func(s *Settings) { s.Basis = types.Basis365360 }, []string{"b365_360"}, 3e-4},
		{"inadvance", func(s *Settings) { s.InAdvance = true }, []string{"inadv"}, 1e-4},
		{"prepaid", func(s *Settings) { s.Prepaid = true }, []string{"prepaid"}, 1e-4},
		// exact-interest (actual/365 daily): formerly had a ~0.06% residual on
		// very long-term high-rate loans (the secant diverged from a poor seed
		// and the engine kept an over-amortizing estimate). Closed by seeding the
		// secant from the closed form + falling back to the bracketing bisection
		// (engine.go exact branch); see TestDOSExactLongTermPayment.
		{"exact365", func(s *Settings) { s.Basis = types.Basis365; s.Exact = true }, []string{"b365", "exact"}, 3e-4},
	}
	perYrChoices := []int{1, 2, 3, 4, 6, 12}
	rng := rand.New(rand.NewSource(0x416d6f72))

	for _, v := range variants {
		checked, fails, maxRel := 0, 0, 0.0
		var worst string
		for i := 0; i < 2500; i++ {
			amount := math.Round((1000+rng.Float64()*4_000_000)*100) / 100
			rate := 0.0005 + rng.Float64()*0.30 // realistic ceiling (30%)
			perYr := perYrChoices[rng.Intn(len(perYrChoices))]
			years := 1 + rng.Intn(50)
			n := 1 + rng.Intn(perYr*years)

			goPay, gok := goAmortizePay(amount, rate, n, perYr, v.mod)
			if !gok {
				continue
			}
			dosPay, ok := runOraclePayment(amount, rate, n, perYr, v.flags...)
			if !ok {
				continue
			}
			checked++
			rel := math.Abs(dosPay-goPay) / math.Max(1, math.Abs(goPay))
			if rel > maxRel {
				maxRel = rel
				worst = "amt=" + strconv.FormatFloat(amount, 'f', 2, 64) + " r=" + strconv.FormatFloat(rate, 'f', 5, 64) +
					" n=" + strconv.Itoa(n) + " py=" + strconv.Itoa(perYr)
			}
			if rel > v.tol {
				fails++
				if fails <= 8 {
					t.Errorf("[%s] amt=%.2f r=%.5f n=%d py=%d: DOS=%.5f Go=%.5f (rel %.2e)",
						v.name, amount, rate, n, perYr, dosPay, goPay, rel)
				}
			}
		}
		t.Logf("[%s] Amortize payment solve: checked %d, divergences %d, maxRel=%.2e at [%s]",
			v.name, checked, fails, maxRel, worst)
	}
}
