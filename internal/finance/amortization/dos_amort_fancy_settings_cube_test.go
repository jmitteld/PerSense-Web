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

// TestDOSAmortFancySettingsCube (Phase 2, docs/exhaustive_option_sweep_plan.md)
// crosses the two advanced options whose blank-payment SOLVE the DOS oracle can
// drive — a moratorium (interest-only deferment) and skip-months — with the
// computational settings cube (basis × prepaid × in-advance × pmts/yr), and
// asserts zero divergence from the real DOS engine on the solved payment. The
// existing TestDOSFancyOptionsSweep validates these options only at the default
// 360 / monthly / no-flags settings; this guarantees they compose correctly with
// every settings combination.
//
// Excluded by construction: `exact` (the documented unimplemented setting, see
// docs/discrepancies.md §8) and `target` (a target-minimum loan does not have a
// blank-payment solve — DOS reports non-convergence — so it is a given-payment,
// row-compared case covered elsewhere).
func TestDOSAmortFancySettingsCube(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	ld := types.NewDateRec(2024, time.January, 1)
	amounts := []float64{60000, 250000}
	rates := []float64{0.05, 0.10}

	cover := map[string]int{}
	checked, fails := 0, 0
	maxRel := 0.0
	var worst string
	// in-advance × fancy (skip / moratorium) is now STRICT (was a bounded ~2-3%
	// corner). The DOS-faithful fix implements RepayFancyLoan's in-advance SHAPE in
	// generateFancySchedule (the `inAdvanceFancy` block): a settlement-interest row
	// at the loan date, a one-period base shift, and ordinary opening-balance
	// interest on the shifted walk (AMORTOP.pas:1159-1187 + ComputeNext:636). The
	// moratorium boundary recompute also accounts for the shift (n-1 amortizing
	// rows). Validated to zero divergence here and across thousands of randomized
	// cases in TestDOSInAdvanceFancyFuzz. See docs/dos_known_frontier.md #38 (closed).
	cornerChecked, cornerDiverged := 0, 0
	cornerMax := 0.0

	for _, opt := range []string{"moratorium", "skip"} {
		// Skip-months are month-based; only sweep them monthly. Moratorium lands
		// on a payment date, so sweep it monthly and quarterly.
		perYrs := []int{12}
		if opt == "moratorium" {
			perYrs = []int{12, 4}
		}
		for _, b365 := range []bool{false, true} {
			for _, prepaid := range []bool{false, true} {
				for _, inadv := range []bool{false, true} {
					for _, perYr := range perYrs {
						var sflags []string
						set := Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}
						if b365 {
							sflags = append(sflags, "b365")
							set.Basis, set.YrDays, set.YrInv = types.Basis365, 365, 1.0/365
						}
						if prepaid {
							sflags = append(sflags, "prepaid")
							set.Prepaid = true
						}
						if inadv {
							sflags = append(sflags, "inadv")
							set.InAdvance = true
						}
						cell := fmt.Sprintf("%s|b365=%v|prepaid=%v|inadv=%v|py=%d", opt, b365, prepaid, inadv, perYr)
						mPer := 12 / perYr

						for _, amount := range amounts {
							for _, rate := range rates {
								nyears := 5
								n := nyears * perYr

								var optTok string
								apply := func(in *LoanInput) {}
								switch opt {
								case "moratorium":
									morMonths := 2 * mPer // interest-only for the first 2 periods
									optTok = "mor=" + strconv.Itoa(morMonths)
									my, mm := 2024+morMonths/12, time.Month(morMonths%12+1)
									apply = func(in *LoanInput) {
										in.Fancy = true
										in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput,
											FirstRepay: types.NewDateRec(my, mm, 1)}
									}
								case "skip":
									skipStr := "6-8"
									optTok = "skip=" + skipStr
									ms, _ := MonthSetFromString(skipStr)
									apply = func(in *LoanInput) {
										in.Fancy = true
										in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: skipStr, MonthSet: ms}
									}
								}

								flags := append(append([]string{}, sflags...), optTok)
								op, ok := runOraclePayment(amount, rate, n, perYr, flags...)
								if !ok {
									continue
								}
								in := LoanInput{
									Loan: Loan{
										AmountStatus: types.InOutInput, Amount: amount,
										LoanRateStatus: types.InOutInput, LoanRate: rate,
										NStatus: types.InOutInput, NPeriods: n,
										PerYrStatus: types.InOutInput, PerYr: perYr,
										PayAmtStatus:   types.StatusEmpty,
										LoanDateStatus: types.InOutInput, LoanDate: ld,
										FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
									Settings: set,
								}
								apply(&in)
								res := Amortize(in)
								if res.Err != nil || len(res.Schedule) == 0 {
									continue
								}
								gp := modalReg(res.Schedule)
								checked++
								cover[cell]++
								rel := math.Abs(op-gp) / math.Max(1, gp)
								// in-advance cells are now held to the SAME strict
								// tolerance as everything else (the corner is closed);
								// tracked separately only for the diagnostic log line.
								if inadv {
									cornerChecked++
									if rel > 1e-3 {
										cornerDiverged++
									}
									if rel > cornerMax {
										cornerMax = rel
									}
								}
								if rel > maxRel {
									maxRel = rel
									worst = fmt.Sprintf("%s amt=%.0f r=%.2f n=%d DOS=%.4f Go=%.4f", cell, amount, rate, n, op, gp)
								}
								if rel > 1e-3 {
									fails++
									if fails <= 15 {
										t.Errorf("[%s] amt=%.0f r=%.2f n=%d: DOS=%.4f Go=%.4f (rel %.2e)",
											cell, amount, rate, n, op, gp, rel)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if len(cover) == 0 {
		t.Fatal("no fancy×settings cells exercised — oracle may be flaking")
	}
	t.Logf("amortization fancy×settings cube: %d cells checked across %d settings combos, divergences %d, max relErr=%.2e at [%s]",
		checked, len(cover), fails, maxRel, worst)
	t.Logf("  in-advance × fancy (skip/moratorium): %d checked, %d diverged(>1e-3), max relErr=%.2e — CLOSED/strict, see docs/dos_known_frontier.md #38",
		cornerChecked, cornerDiverged, cornerMax)
	if cornerDiverged > 0 {
		t.Errorf("fancy×in-advance is now strict but %d cell(s) diverged (>1e-3), max relErr=%.2e", cornerDiverged, cornerMax)
	}
	if checked < 60 {
		t.Fatalf("cube exercised only %d cases — oracle may be flaking", checked)
	}
}
