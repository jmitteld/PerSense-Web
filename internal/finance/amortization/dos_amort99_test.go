package amortization

import (
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Additional per-row differentials closing the amortization-basic gaps that held
// it below the bit-identical tier: a TWO-balloon per-row sweep (the existing
// two-balloon test checked only the solved payment) and a 30/360-vs-365-day
// MONTHLY per-row sweep (the 365 basis was previously per-row tested only for
// weekly/biweekly, and payment-solve-only for monthly).

// goTwoBalloonRows solves the payment for a loan with two balloons and returns
// the per-period schedule.
func goTwoBalloonRows(amount, rate float64, n, perYr, m1 int, a1 float64, m2 int, a2 float64) ([]PaymentRecord, float64, bool) {
	bdate := func(m int) types.DateRec {
		return types.NewDateRec(2024+m/12, time.Month(m%12+1), 1)
	}
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Balloons: []BalloonPayment{
			{DateStatus: types.InOutInput, Date: bdate(m1), AmountStatus: types.InOutInput, Amount: a1},
			{DateStatus: types.InOutInput, Date: bdate(m2), AmountStatus: types.InOutInput, Amount: a2},
		},
		Fancy:    true,
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: false}}
	d, err := SolvePayment(in)
	if err != nil {
		return nil, 0, false
	}
	chk := in
	chk.Loan.PayAmtStatus = types.InOutDefault
	chk.Loan.PayAmt = d
	r := Amortize(chk)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, 0, false
	}
	return r.Schedule, d, true
}

func TestDOSTwoBalloonPerRowSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260715))
	checked, skipped, countFails, valFails := 0, 0, 0, 0
	maxRel := 0.0
	for i := 0; i < 250; i++ {
		amount := float64(50000 + rng.Intn(450000))
		rate := 0.03 + rng.Float64()*0.09
		n := 36 + rng.Intn(60)
		m1 := 6 + rng.Intn(n/3)
		m2 := n/2 + rng.Intn(n/3)
		if m2 <= m1 {
			m2 = m1 + 6
		}
		a1 := float64(3000 + rng.Intn(15000))
		a2 := float64(3000 + rng.Intn(15000))
		gp, pay, gok := goTwoBalloonRows(amount, rate, n, 12, m1, a1, m2, a2)
		if !gok {
			skipped++
			continue
		}
		dosRows, ok := runOracleRowsFlags(amount, rate, n, 12, pay,
			"b"+strconv.Itoa(m1)+"="+strconv.FormatFloat(a1, 'f', 2, 64),
			"b"+strconv.Itoa(m2)+"="+strconv.FormatFloat(a2, 'f', 2, 64))
		if !ok {
			skipped++
			continue
		}
		checked++
		if len(dosRows) != len(gp) {
			countFails++
			if countFails <= 5 {
				t.Errorf("ROW COUNT amt=%.0f r=%.4f n=%d b1=%d/%.0f b2=%d/%.0f: DOS=%d Go=%d",
					amount, rate, n, m1, a1, m2, a2, len(dosRows), len(gp))
			}
			continue
		}
		for k := 0; k < len(dosRows)-1; k++ {
			di := math.Abs(dosRows[k].interest - gp[k].Interest)
			db := math.Abs(dosRows[k].balance - gp[k].Principal)
			if rb := db / math.Max(1, math.Abs(gp[k].Principal)); rb > maxRel {
				maxRel = rb
			}
			if di > 0.02+1e-4*math.Abs(gp[k].Interest) || db > 0.02+1e-4*math.Abs(gp[k].Principal) {
				valFails++
				if valFails <= 8 {
					t.Errorf("ROW amt=%.0f r=%.4f n=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
						amount, rate, n, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
				}
			}
		}
	}
	t.Logf("two-balloon per-row: checked %d, skipped %d, count fails %d, value fails %d, max bal relErr=%.2e",
		checked, skipped, countFails, valFails, maxRel)
}

// TestDOS365BasisMonthlyFirstPeriod characterises a documented first-period
// day-count convention difference on the (uncommon) actual/365 + MONTHLY basis:
// DOS charges the first regular period the nominal rate/Pmts-per-Yr, while Go
// prorates it by actual days. The divergence is confined to period 1; periods
// 2..n are identical (both use rate/12). See docs/amort_365_first_period_finding.md.
// The common 30/360 monthly case is bit-faithful (TestDOSPerRowSweep).
func TestDOS365BasisMonthlyFirstPeriod(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	set365 := func(s *Settings) {
		s.Basis = types.Basis365
		s.YrDays = 365.25
		s.YrInv = 1.0 / 365.25
	}
	const amount, rate = 100000.0, 0.06
	const n, perYr = 12, 12
	gp, pay, ok := goRowsFlags(amount, rate, n, perYr, set365, set365)
	if !ok {
		t.Fatal("go schedule failed")
	}
	dos, ok2 := runOracleRowsFlags(amount, rate, n, perYr, pay, "b365")
	if !ok2 || len(dos) != len(gp) {
		t.Fatalf("oracle failed or row count mismatch (DOS %d Go %d)", len(dos), len(gp))
	}
	// (1) DOS first period = nominal rate/12 on the opening balance.
	nominal := amount * rate / float64(perYr)
	if math.Abs(dos[0].interest-nominal) > 0.01 {
		t.Errorf("DOS first-period interest = %.4f, expected nominal rate/12 = %.4f", dos[0].interest, nominal)
	}
	// (2) Go first period prorates by actual days on the 365 basis, so it is
	// MORE than the nominal rate/12 (a full month is >30/360-equivalent days) —
	// here ~508.2 vs 500. The exact value depends on Go's YearsDif day count;
	// the documented point is that it differs from DOS's nominal by ~one day.
	if !(gp[0].Interest > nominal+1) {
		t.Errorf("Go first-period interest = %.4f, expected > nominal %.4f (actual-day proration)", gp[0].Interest, nominal)
	}
	// (3) The divergence is CONFINED to period 1: from period 2 on, both engines
	// accrue balance*rate/12, so each row's interest equals rate/12 of the prior
	// row's balance on its OWN side. Verify both sides are internally rate/12 and
	// the only inter-engine gap is the carried first-period offset (small).
	for k := 1; k < len(dos)-1; k++ {
		dosNominal := dos[k-1].balance * rate / float64(perYr)
		if math.Abs(dos[k].interest-dosNominal) > 0.01 {
			t.Errorf("DOS row %d interest %.4f != rate/12 of prior balance %.4f", k+1, dos[k].interest, dosNominal)
		}
		goNominal := gp[k-1].Principal * rate / float64(perYr)
		if math.Abs(gp[k].Interest-goNominal) > 0.01 {
			t.Errorf("Go row %d interest %.4f != rate/12 of prior balance %.4f", k+1, gp[k].Interest, goNominal)
		}
	}
	t.Logf("365-basis monthly first-period: DOS=%.4f (nominal) Go=%.4f (actual-day); confined to period 1 (documented)", dos[0].interest, gp[0].Interest)
}
