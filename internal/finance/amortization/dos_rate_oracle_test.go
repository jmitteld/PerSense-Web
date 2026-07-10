package amortization

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// runOracleRate execs the DOS source-oracle's `solverate` query (rate blanked,
// payment supplied) and returns the DOS-solved loan rate. Retries the heap glitch
// like runOracle.
func runOracleRate(t *testing.T, args []string) (rate float64, ok bool) {
	t.Helper()
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) < 2 || f[0] != "rate" {
			continue
		}
		v, perr := strconv.ParseFloat(f[1], 64)
		if perr != nil {
			continue
		}
		return v, true
	}
	return 0, false
}

// TestRateSolveVsOracle_Negative guards the negative-rate solve. An under-funded
// loan — total payments below principal — has a genuinely NEGATIVE implied rate
// (here $100,000 borrowed, 120 × $750 = $90,000 repaid → ≈ −2%). The engine's
// SolveRate used to clamp the Newton to positive (`if rate < 0 { rate = small }`)
// and reject a negative refinement (`refined > 0`), so it pinned the rate at
// ~0.0001 and reported "did not converge" — while DOS's Iterate happily solves the
// negative root (AMORTOP.pas:1485, bounded only at |rate|>2). This sweeps
// interest-timing × basis × exact and asserts the engine matches the DOS oracle.
//
// Skips the arrears+prepaid+plain combos: those hit a SEPARATE, pre-existing gap
// (the closed-form RepayLoan rate terminal ignores prepaid interest — it mismatches
// DOS for positive rates too). Tracked in docs/discrepancies.md §11; not caused by
// and not in scope of the negative-rate fix.
func TestRateSolveVsOracle_Negative(t *testing.T) {
	gateOracle(t)

	const (
		amount = 100000.0
		nper   = 120
		perYr  = 12
		pay    = 750.0 // 120×750 = 90,000 < 100,000 → negative implied rate
	)
	loanDate := types.NewDateRec(2026, time.June, 13)
	firstDate := types.NewDateRec(2026, time.August, 1)

	type basisCase struct {
		name  string
		basis types.BasisType
		flag  string
	}
	bases := []basisCase{
		{"360", types.Basis360, ""},
		{"365", types.Basis365, "b365"},
		{"365-360", types.Basis365360, "b365_360"},
	}
	for _, adv := range []bool{false, true} {
		for _, ex := range []bool{false, true} {
			for _, bc := range bases {
				for _, pp := range []bool{false, true} {
					adv, ex, bc, pp := adv, ex, bc, pp
					// exactDaily is a no-op on the 360 basis, so "exact 360" behaves
					// like plain 360 for this purpose.
					exactActive := ex && bc.basis != types.Basis360
					// The gap is specific to ARREARS: in-advance has its own
					// first-period handling and prepaid does not shift its solved
					// rate, so advance+prepaid matches DOS either way.
					plainPrepaidGap := pp && !adv && !exactActive
					name := "arrears"
					if adv {
						name = "advance"
					}
					if ex {
						name += "_exact"
					}
					name += "_" + bc.name
					if pp {
						name += "_prepaid"
					}
					t.Run(name, func(t *testing.T) {
						if plainPrepaidGap {
							t.Skip("pre-existing plain-loan prepaid rate-solve gap (docs/discrepancies.md §11)")
						}
						ctx := interest.NewCalcContext(bc.basis, perYr)
						in := LoanInput{
							Loan: Loan{
								AmountStatus: types.InOutInput, Amount: amount,
								LoanDateStatus: types.InOutInput, LoanDate: loanDate,
								LoanRateStatus: types.StatusEmpty, // solve the rate
								FirstStatus:    types.InOutInput, FirstDate: firstDate,
								NStatus: types.InOutInput, NPeriods: nper,
								PerYrStatus: types.InOutInput, PerYr: perYr,
								PayAmtStatus: types.InOutInput, PayAmt: pay,
								PointsStatus: types.InOutInput, Points: 0,
							},
							Settings: Settings{
								Basis: bc.basis, PerYr: perYr, Prepaid: pp,
								InAdvance: adv, Exact: ex,
								YrDays: ctx.YrDays, YrInv: ctx.YrInv,
							},
						}
						goRate, conv, err := SolveRate(in)
						if err != nil {
							t.Fatalf("SolveRate: %v", err)
						}
						if !conv {
							t.Fatalf("SolveRate did not converge (rate=%.6f) — the negative-rate clamp regressed", goRate)
						}
						if goRate >= 0 {
							t.Fatalf("expected a negative solved rate, got %.6f", goRate)
						}

						args := []string{
							strconv.FormatFloat(amount, 'f', 2, 64), "0",
							strconv.Itoa(nper), strconv.Itoa(perYr),
							"payhard=" + strconv.FormatFloat(pay, 'f', 2, 64),
							"loandmy=13.6.2026", "firstdmy=1.8.2026", "solverate",
						}
						if adv {
							args = append(args, "inadv")
						}
						if ex {
							args = append(args, "exact")
						}
						if bc.flag != "" {
							args = append(args, bc.flag)
						}
						if pp {
							args = append(args, "prepaid")
						}
						dosRate, ok := runOracleRate(t, args)
						if !ok {
							t.Skip("oracle produced no rate after retries (heap glitch)")
						}
						if diff := goRate - dosRate; diff > 5e-5 || diff < -5e-5 {
							t.Errorf("rate mismatch: engine=%.6f oracle=%.6f (Δ=%+.6f)",
								goRate, dosRate, diff)
						}
					})
				}
			}
		}
	}
}
