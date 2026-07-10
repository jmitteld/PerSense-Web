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

// runOracleAPR execs the DOS source-oracle's `apr` query and returns the
// DOS-computed APR (h^.apr). It mirrors runOracle's retry-on-heap-glitch loop
// (the Pascal New(h)/ZeroAMZLoan path occasionally yields a 0 result on rapid
// spawns; every such case reproduces on a fresh process). The `pts=0` token makes
// the engine treat points as an entered 0 so the APR solver runs
// (Amortize.pas:1420 gates the solve on pointsstatus>defp), and `apr` prints
// "apr <value> status <s>" then halts.
func runOracleAPR(t *testing.T, args []string) (apr float64, ok bool) {
	t.Helper()
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) < 2 || f[0] != "apr" {
			continue
		}
		v, perr := strconv.ParseFloat(f[1], 64)
		if perr != nil || v == 0 {
			continue
		}
		return v, true
	}
	return 0, false
}

// TestAPRVsOracleSweep is the differential guard the APR path lacked. Until the
// oracle grew an `apr` query, APR was the one headline output NOT checked against
// the DOS engine — which is how a large in-advance APR error (up to ~4 points on a
// 360 basis: the engine discounted the TRUNCATED display schedule, DOS discounts
// the FULL-TERM value walk, RepayFancyLoan value_calc) went unnoticed while every
// other output was oracle-clean. This sweeps interest-timing × day-count basis ×
// exact and asserts the engine's APR matches the DOS oracle to the cent-equivalent.
//
// Fixed by aprValueCashflows (backward.go) / the unforced full-term walk
// (engine.go generateExactInAdvanceScheduleMode + generateFancyScheduleMode).
func TestAPRVsOracleSweep(t *testing.T) {
	gateOracle(t)

	// The client's high-rate loan: $500k at 33%, $15k user payment, 360/12,
	// loan 1/1/2028, first 2/1/2028. It retires early (~payment 97), which is
	// exactly what makes the truncated-vs-full-term discounting diverge.
	const (
		amount = 500000.0
		rate   = 0.33
		nper   = 360
		perYr  = 12
		pay    = 15000.0
	)
	loanDate := types.NewDateRec(2028, time.January, 1)
	firstDate := types.NewDateRec(2028, time.February, 1)

	type basisCase struct {
		name  string
		basis types.BasisType
		flag  string // oracle basis flag ("" = default 360)
	}
	bases := []basisCase{
		{"360", types.Basis360, ""},
		{"365", types.Basis365, "b365"},
		{"365/360", types.Basis365360, "b365_360"},
	}
	for _, pts := range []float64{0, 0.03} {
		for _, adv := range []bool{false, true} {
			for _, ex := range []bool{false, true} {
				for _, bc := range bases {
					pts, adv, ex, bc := pts, adv, ex, bc
					name := "arrears"
					if adv {
						name = "advance"
					}
					if ex {
						name += "_exact"
					}
					name += "_" + strings.ReplaceAll(bc.name, "/", "-")
					if pts > 0 {
						name += "_pts"
					}
					t.Run(name, func(t *testing.T) {
						// --- Go engine APR ---
						ctx := interest.NewCalcContext(bc.basis, perYr)
						in := LoanInput{
							Loan: Loan{
								AmountStatus: types.InOutInput, Amount: amount,
								LoanDateStatus: types.InOutInput, LoanDate: loanDate,
								LoanRateStatus: types.InOutInput, LoanRate: rate,
								FirstStatus: types.InOutInput, FirstDate: firstDate,
								NStatus: types.InOutInput, NPeriods: nper,
								PerYrStatus: types.InOutInput, PerYr: perYr,
								PayAmtStatus: types.InOutInput, PayAmt: pay,
								PointsStatus: types.InOutInput, Points: pts,
							},
							Settings: Settings{
								Basis: bc.basis, PerYr: perYr, Prepaid: true,
								InAdvance: adv, Exact: ex,
								YrDays: ctx.YrDays, YrInv: ctx.YrInv,
							},
						}
						res := Amortize(in)
						if res.Err != nil {
							t.Fatalf("Amortize: %v", res.Err)
						}

						// --- DOS oracle APR ---
						args := []string{
							strconv.FormatFloat(amount, 'f', 2, 64),
							strconv.FormatFloat(rate, 'f', 6, 64),
							strconv.Itoa(nper), strconv.Itoa(perYr),
							"payhard=" + strconv.FormatFloat(pay, 'f', 2, 64),
							"loandmy=1.1.2028", "firstdmy=1.2.2028",
							"pts=" + strconv.FormatFloat(pts, 'f', 4, 64), "apr", "prepaid",
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
						dosAPR, ok := runOracleAPR(t, args)
						if !ok {
							t.Skip("oracle produced no APR after retries (heap glitch)")
						}

						// Compare. DOS h^.apr is a fraction (0.33 = 33%); res.APR too.
						// 5e-5 ≈ half a basis point — tighter than any display rounding.
						if diff := res.APR - dosAPR; diff > 5e-5 || diff < -5e-5 {
							t.Errorf("APR mismatch: engine=%.6f oracle=%.6f (Δ=%+.6f)",
								res.APR, dosAPR, diff)
						}
					})
				}
			}
		}
	}
}
