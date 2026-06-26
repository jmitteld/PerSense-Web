package amortization

import (
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// r78Input builds a PLAIN Rule-of-78 (sum-of-digits) loan with a solved payment.
func r78Input(amount, rate float64, n, perYr int) LoanInput {
	return LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr),
			YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true, R78: true},
		Fancy: false,
	}
}

// TestProductionR78Baseline measures the PRODUCTION engine's plain Rule-of-78
// schedule against the DOS oracle (r78 token). Opt-in; needs the oracle.
func TestProductionR78Baseline(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(7878))
	nCases := 200
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			nCases = v
		}
	}
	ran, div := 0, 0
	var maxRel float64
	for i := 0; i < nCases; i++ {
		perYr := []int{12, 6, 4, 2, 1}[rng.Intn(5)]
		amount := float64(int(60000+rng.Float64()*400000)/1000) * 1000
		rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		n := []int{12, 24, 36, 48, 60}[rng.Intn(5)]

		res := Amortize(r78Input(amount, rate, n, perYr))
		if res.Err != nil || len(res.Schedule) == 0 {
			continue
		}
		dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, "r78")
		if !dok {
			continue
		}
		ran++
		rel := math.Abs(res.TotalInt-dosInt) / math.Max(1, math.Abs(dosInt))
		if rel > maxRel {
			maxRel = rel
		}
		if math.Abs(res.TotalInt-dosInt) > math.Max(0.10, 1e-5*math.Abs(dosInt)) {
			div++
			if div <= 10 {
				t.Errorf("R78 DIVERGE amt=%.0f r=%.4f n=%d py=%d: prod=%.2f oracle=%.2f (Δ %.2f, rel %.2e)",
					amount, rate, n, perYr, res.TotalInt, dosInt, res.TotalInt-dosInt, rel)
			}
		}
	}
	t.Logf("PRODUCTION R78 vs oracle: ran=%d divergences=%d maxRel=%.2e", ran, div, maxRel)
}
