package amortization

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestDOSAmortFancy365RowCube extends the per-row basis cube to FANCY loans
// (a balloon makes the schedule run through generateFancySchedule, a different
// code path than the plain generateSimpleSchedule the R78/USA cube exercises). It
// crosses a balloon with basis {360,365} × pmts/yr over a grid and compares every
// schedule row to the real DOS engine, confirming the firstPeriodProrate fix
// reaches the fancy first-period accrual too (clean-boundary first period is a
// whole period on either basis). Coverage-asserted.
func TestDOSAmortFancy365RowCube(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	amounts := []float64{60000, 250000}
	rates := []float64{0.05, 0.10}
	perYrs := []int{12, 4}
	ld := types.NewDateRec(2024, time.January, 1)

	cover := map[string]int{}
	checked, rowChecks, countFails, valFails := 0, 0, 0, 0
	maxRel := 0.0
	var worst string
	// Both bases are now strict: periodYearFraction makes generateFancySchedule
	// accrue whole-period interest with the per-period (month-based) fraction
	// regardless of basis (reserving actual days for off-cycle partials and odd-day
	// stubs), so the 365-basis per-row oscillation is closed.

	for _, b365 := range []bool{false, true} {
		set := Settings{Basis: types.Basis360, PerYr: 0, YrDays: 360, YrInv: 1.0 / 360}
		basisFlag := ""
		if b365 {
			set.Basis, set.YrDays, set.YrInv = types.Basis365, 365, 1.0/365
			basisFlag = "b365"
		}
		for _, perYr := range perYrs {
			mPer := 12 / perYr
			cell := fmt.Sprintf("b365=%v|py=%d", b365, perYr)
			for _, amount := range amounts {
				for _, rate := range rates {
					ny := 4 // modest schedules — keep post-balloon rounding small (cf. TestDOSBalloonPerRowSweep)
					n := ny * perYr
					bp := n / 2 // balloon at the midpoint payment
					bMonths := bp * mPer
					bAmt := amount * 0.15
					s := set
					s.PerYr = byte(perYr)

					by, bm := 2024+bMonths/12, time.Month(bMonths%12+1)
					in := LoanInput{
						Loan: Loan{
							AmountStatus: types.InOutInput, Amount: amount,
							LoanRateStatus: types.InOutInput, LoanRate: rate,
							NStatus: types.InOutInput, NPeriods: n,
							PerYrStatus: types.InOutInput, PerYr: perYr,
							PayAmtStatus:   types.StatusEmpty,
							LoanDateStatus: types.InOutInput, LoanDate: ld,
							FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
						Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(by, bm, 1),
							AmountStatus: types.InOutInput, Amount: bAmt}},
						Fancy:    true,
						Settings: s,
					}
					res := Amortize(in)
					if res.Err != nil || len(res.Schedule) == 0 {
						continue
					}
					pay := modalReg(res.Schedule)
					bTok := "b" + strconv.Itoa(bMonths) + "=" + strconv.FormatFloat(bAmt, 'f', 2, 64)
					flags := []string{bTok}
					if basisFlag != "" {
						flags = append(flags, basisFlag)
					}
					dosRows, ok := runOracleRowsFlags(amount, rate, n, perYr, pay, flags...)
					if !ok {
						continue
					}
					checked++
					cover[cell]++
					if len(dosRows) != len(res.Schedule) {
						countFails++
						if countFails <= 8 {
							t.Errorf("[%s] ROW COUNT amt=%.0f r=%.2f n=%d bMo=%d: DOS=%d Go=%d",
								cell, amount, rate, n, bMonths, len(dosRows), len(res.Schedule))
						}
						continue
					}
					for k := 0; k < len(dosRows); k++ {
						di := math.Abs(dosRows[k].interest - res.Schedule[k].Interest)
						db := math.Abs(dosRows[k].balance - res.Schedule[k].Principal)
						rb := db / math.Max(1, math.Abs(res.Schedule[k].Principal))
						if rb > maxRel {
							maxRel = rb
							worst = fmt.Sprintf("%s amt=%.0f r=%.2f n=%d row=%d", cell, amount, rate, n, k+1)
						}
						// Interest (the per-row split) is strict on every row; balances
						// in the payoff-completion zone (last few rows) carry a small
						// cumulative balloon-rounding residual, excluded as in
						// TestDOSBalloonPerRowSweep.
						// Interest (the per-row split) is strict on every row, both
						// bases; balances in the payoff-completion zone (last few rows)
						// carry a small cumulative balloon-rounding residual, excluded
						// as in TestDOSBalloonPerRowSweep.
						nearPayoff := k >= len(dosRows)-4
						intBad := di > 0.02+1e-4*math.Abs(res.Schedule[k].Interest)
						balBad := !nearPayoff && db > 0.02+1e-4*math.Abs(res.Schedule[k].Principal)
						rowChecks++
						if intBad || balBad {
							valFails++
							if valFails <= 12 {
								t.Errorf("[%s] amt=%.0f r=%.2f n=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
									cell, amount, rate, n, k+1, dosRows[k].interest, res.Schedule[k].Interest, dosRows[k].balance, res.Schedule[k].Principal)
							}
						}
					}
				}
			}
		}
	}

	for _, b365 := range []bool{false, true} {
		for _, perYr := range perYrs {
			cell := fmt.Sprintf("b365=%v|py=%d", b365, perYr)
			if cover[cell] == 0 {
				t.Errorf("cell %q never exercised — coverage gap", cell)
			}
		}
	}
	t.Logf("fancy (balloon) × basis × pmts/yr row cube: %d schedules, %d row checks (both bases strict), count fails %d, value fails %d, max bal relErr=%.2e at [%s]",
		checked, rowChecks, countFails, valFails, maxRel, worst)
	if checked < 12 {
		t.Fatalf("cube exercised only %d schedules — oracle may be flaking", checked)
	}
}
