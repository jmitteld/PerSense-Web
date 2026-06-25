package amortization

import (
	"math"
	"math/rand"
	"os"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// inAdvanceInput builds a PLAIN in-advance (annuity-due) loan with a solved payment.
func inAdvanceInput(amount, rate float64, n, perYr int) LoanInput {
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
			YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true, InAdvance: true},
		Fancy: false,
	}
}

// TestInAdvanceSettlementRow pins the DOS-faithful in-advance settlement (no oracle
// needed): the FIRST period's interest is charged in advance at the loan date as a
// PayNum-0 row (interest = amount·(f-1), principal unchanged), and the total
// interest includes it. Root case: 100k/6%/12mo in-advance → DOS interest 3279.86,
// settlement 500.00. Before the fix the total was 2779.86 (settlement omitted).
func TestInAdvanceSettlementRow(t *testing.T) {
	res := Amortize(inAdvanceInput(100000, 0.06, 12, 12))
	if res.Err != nil || len(res.Schedule) == 0 {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	// Row 0 is the settlement: PayNum 0, interest ≈ 500, principal unchanged.
	r0 := res.Schedule[0]
	if r0.PayNum != 0 {
		t.Fatalf("expected a PayNum-0 settlement row, got PayNum=%d", r0.PayNum)
	}
	if math.Abs(r0.Interest-500.0) > 0.01 || math.Abs(r0.Principal-100000) > 0.01 {
		t.Errorf("settlement row: int=%.2f (want 500.00), principal=%.2f (want 100000)", r0.Interest, r0.Principal)
	}
	// Total interest includes the settlement (DOS oracle = 3279.86).
	if math.Abs(res.TotalInt-3279.86) > 0.05 {
		t.Errorf("in-advance total interest = %.2f, want ~3279.86 (settlement included)", res.TotalInt)
	}
	// The cumulative interest on the final row equals the total (rows sum to total).
	if last := res.Schedule[len(res.Schedule)-1]; math.Abs(last.IntToDate-res.TotalInt) > 0.05 {
		t.Errorf("final IntToDate %.2f != TotalInt %.2f", last.IntToDate, res.TotalInt)
	}
}

// TestProductionInAdvanceBaseline measures the PRODUCTION engine's plain in-advance
// schedule against the DOS oracle, to decide whether the port needs to reproduce
// it or can keep delegating in-advance loans to production. Opt-in; needs oracle.
func TestProductionInAdvanceBaseline(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(2024))
	ran, div := 0, 0
	var maxRel float64
	for i := 0; i < 200; i++ {
		perYr := []int{12, 6, 4, 2, 1}[rng.Intn(5)]
		amount := float64(int(60000+rng.Float64()*400000)/1000) * 1000
		rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		n := []int{12, 24, 36, 48, 60}[rng.Intn(5)]

		res := Amortize(inAdvanceInput(amount, rate, n, perYr))
		if res.Err != nil || len(res.Schedule) == 0 {
			continue
		}
		dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, "inadv")
		if !dok {
			continue
		}
		ran++
		// Production now includes the upfront settlement (row 0), so its total
		// should match the oracle directly.
		rel := math.Abs(res.TotalInt-dosInt) / math.Max(1, math.Abs(dosInt))
		if rel > maxRel {
			maxRel = rel
		}
		if math.Abs(res.TotalInt-dosInt) > math.Max(0.10, 1e-5*math.Abs(dosInt)) {
			div++
			if div <= 10 {
				t.Errorf("IN-ADV DIVERGE amt=%.0f r=%.4f n=%d py=%d: prod=%.2f oracle=%.2f (Δ %.2f)",
					amount, rate, n, perYr, res.TotalInt, dosInt, res.TotalInt-dosInt)
			}
		}
	}
	t.Logf("PRODUCTION in-advance vs oracle: ran=%d divergences=%d maxRel=%.2e", ran, div, maxRel)
}
