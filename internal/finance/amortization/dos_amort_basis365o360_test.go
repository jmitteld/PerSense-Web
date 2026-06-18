package amortization

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// b365o360 sets the actual/360 hybrid day-count (UI "365/360", types.Basis365360)
// on a Settings, matching the oracle's `b365_360` flag.
func b365o360(s *Settings) {
	s.Basis, s.YrDays, s.YrInv = types.Basis365360, 360, 1.0/360
}

// TestDOSAmort365o360BasisSweep closes the documented settings gap that the
// "365/360" day-count option was never swept end-to-end vs DOS (it was only
// unit-tested in dateutil/rates). Two parts:
//
//  1. Clean-boundary per-row check: on whole monthly/quarterly periods the
//     regular-period interest is basis-independent (GrowthPerPeriod = 1 +
//     rate/RealPerYr, AMORTOP.pas:1247), so 365/360 must match DOS exactly —
//     a guard that nothing accidentally diverges.
//  2. Odd-DAYS first period: this is where 365/360 actually bites — the partial
//     first period accrues ACTUAL calendar days over a 360-day year, unlike
//     30/360. Mirrors TestDOSOddDaysFirstPeriodSweep but with the hybrid basis
//     on both engines, comparing the solved payment.
func TestDOSAmort365o360BasisSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}

	// Part 1 — clean-boundary per-row (basis-independent; strict 0 divergence).
	perRowFlagSweep(t, "b365_360 clean per-row", 0x365360, false, b365o360, b365o360, "b365_360")

	// Part 2 — odd-DAYS first period (the hybrid's actual-day accrual).
	rng := rand.New(rand.NewSource(0x365361))
	checked, fails := 0, 0
	maxRel, worst := 0.0, ""
	for i := 0; i < 400; i++ {
		amount := float64(20000 + rng.Intn(480000))
		rate := 0.02 + rng.Float64()*0.13
		perYr := []int{12, 4}[rng.Intn(2)]
		mPer := 12 / perYr
		nPeriods := (3 + rng.Intn(25)) * mPer

		loanY, loanM, loanD := 2024, 1+rng.Intn(12), 1+rng.Intn(27)
		fMonthAbs := (loanM - 1) + mPer
		firstY, firstM := loanY+fMonthAbs/12, fMonthAbs%12+1
		firstD := 1 + rng.Intn(27)
		if firstD == loanD {
			firstD = 1 + (firstD % 27)
		}

		op, ok := runOraclePayment(amount, rate, nPeriods, perYr,
			fmt.Sprintf("loandmy=%d.%d.%d", loanD, loanM, loanY),
			fmt.Sprintf("firstdmy=%d.%d.%d", firstD, firstM, firstY),
			"b365_360")
		if !ok {
			continue
		}
		set := Settings{PerYr: byte(perYr)}
		b365o360(&set)
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amount,
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				NStatus: types.InOutInput, NPeriods: nPeriods,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(loanY, time.Month(loanM), loanD),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(firstY, time.Month(firstM), firstD)},
			Settings: set,
		}
		res := Amortize(in)
		if res.Err != nil || len(res.Schedule) == 0 {
			continue
		}
		gp := modalReg(res.Schedule)
		checked++
		rel := math.Abs(op-gp) / math.Max(1, gp)
		if rel > maxRel {
			maxRel, worst = rel, fmt.Sprintf("amt=%.0f r=%.4f n=%d py=%d loan=%d.%d.%d first=%d.%d.%d DOS=%.4f Go=%.4f",
				amount, rate, nPeriods, perYr, loanD, loanM, loanY, firstD, firstM, firstY, op, gp)
		}
		if rel > 1e-3 {
			fails++
			if fails <= 15 {
				t.Errorf("365/360 ODD-DAYS amt=%.0f r=%.4f n=%d py=%d loan=%d.%d.%d first=%d.%d.%d: DOS=%.4f Go=%.4f (rel %.2e)",
					amount, rate, nPeriods, perYr, loanD, loanM, loanY, firstD, firstM, firstY, op, gp, rel)
			}
		}
	}
	if checked < 150 {
		t.Fatalf("365/360 odd-days: only %d checked — oracle may be flaking", checked)
	}
	t.Logf("365/360 odd-DAYS first-period payment solve vs DOS: checked %d, divergences %d, max relErr=%.2e [%s]",
		checked, fails, maxRel, worst)
}
