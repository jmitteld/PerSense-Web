package amortization

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestDOSAmortR78USACube (Phase 2, docs/exhaustive_option_sweep_plan.md) is the
// exhaustive schedule cube for the two interest-allocation settings whose effect
// is on the SCHEDULE split rather than the payment: Rule of 78 and the US Rule.
// TestDOSFancyFlagSweep already validates each per-row at the 360 basis with
// random pmts/yr; this DETERMINISTICALLY crosses them with basis ∈ {360, 365}
// and pmts/yr ∈ {1,2,4,12} over a value grid, comparing every schedule row's
// interest and balance to the real DOS engine. The payment is solved on the Go
// side (with the basis applied) and fed to the oracle, so the comparison isolates
// the interest/principal split under each (method × basis × pmts/yr) combination.
// A coverage map asserts every cell was exercised.
func TestDOSAmortR78USACube(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	amounts := []float64{50000, 200000}
	rates := []float64{0.05, 0.10}
	yearsList := []int{5, 10}
	perYrs := []int{1, 2, 4, 12}

	cover := map[string]int{}
	checked, rowChecks, countFails, valFails := 0, 0, 0, 0
	maxRel := 0.0
	var worst string
	// Both bases are now strict 0 divergence: the 360-basis R78/USA allocation and
	// — after the firstPeriodProrate fix (clean-boundary first periods are
	// month-based regardless of basis) and the R78-on-365 gate — the 365 basis too.

	for _, method := range []string{"r78", "usa"} {
		for _, b365 := range []bool{false, true} {
			// basisMod applies the basis to a Settings (used in both the payment
			// solve and the schedule). The method is applied only to the schedule
			// (amortMod) — R78 and the US rule do not change the payment.
			basisMod := func(s *Settings) {
				if b365 {
					s.Basis, s.YrDays, s.YrInv = types.Basis365, 365, 1.0/365
				}
			}
			amortMod := func(s *Settings) {
				basisMod(s)
				switch method {
				case "r78":
					s.R78 = true
				case "usa":
					s.USARule = true
				}
			}
			var oracleFlags []string
			if b365 {
				oracleFlags = append(oracleFlags, "b365")
			}
			oracleFlags = append(oracleFlags, method)

			for _, perYr := range perYrs {
				cell := fmt.Sprintf("%s|b365=%v|py=%d", method, b365, perYr)
				for _, amount := range amounts {
					for _, rate := range rates {
						for _, ny := range yearsList {
							n := ny * perYr
							gp, pay, gok := goRowsFlags(amount, rate, n, perYr, basisMod, amortMod)
							if !gok {
								continue
							}
							dosRows, ok := runOracleRowsFlags(amount, rate, n, perYr, pay, oracleFlags...)
							if !ok {
								continue
							}
							checked++
							cover[cell]++
							if len(dosRows) != len(gp) {
								countFails++
								if countFails <= 8 {
									t.Errorf("[%s] ROW COUNT amt=%.0f r=%.2f n=%d: DOS=%d Go=%d",
										cell, amount, rate, n, len(dosRows), len(gp))
								}
								continue
							}
							for k := 0; k < len(dosRows); k++ {
								di := math.Abs(dosRows[k].interest - gp[k].Interest)
								db := math.Abs(dosRows[k].balance - gp[k].Principal)
								bad := di > 0.01+1e-4*math.Abs(gp[k].Interest) || db > 0.01+1e-4*math.Abs(gp[k].Principal)
								rb := db / math.Max(1, math.Abs(gp[k].Principal))
								if rb > maxRel {
									maxRel = rb
									worst = fmt.Sprintf("%s amt=%.0f r=%.2f n=%d row=%d", cell, amount, rate, n, k+1)
								}
								rowChecks++
								if bad {
									valFails++
									if valFails <= 12 {
										t.Errorf("[%s] amt=%.0f r=%.2f n=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
											cell, amount, rate, n, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Coverage: every (method × basis × perYr) cell exercised.
	for _, method := range []string{"r78", "usa"} {
		for _, b365 := range []bool{false, true} {
			for _, perYr := range perYrs {
				cell := fmt.Sprintf("%s|b365=%v|py=%d", method, b365, perYr)
				if cover[cell] == 0 {
					t.Errorf("cell %q never exercised — coverage gap", cell)
				}
			}
		}
	}
	t.Logf("R78/USA × basis × pmts/yr cube: %d schedules, %d row checks across 16 cells (both bases strict), count fails %d, value fails %d, max bal relErr=%.2e at [%s]",
		checked, rowChecks, countFails, valFails, maxRel, worst)
	if checked < 100 {
		t.Fatalf("cube exercised only %d schedules — oracle may be flaking", checked)
	}
}
