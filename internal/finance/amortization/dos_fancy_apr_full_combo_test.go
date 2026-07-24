package amortization

import (
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestFancyAPRFullComboVsOracle — 2026-07-24 client-reported APR discrepancy.
// A client screenshot showed the web APR (9.7310%) differing from the DOS *app*
// APR (9.8304%) on a rich negative-amortizing loan: 365/360 + Exact, a solved
// rate, an OFF-CYCLE balloon (10/19, between payment dates), a REPLACE-mode
// semi-monthly prepayment series, and a set-both rate/payment adjustment. The
// client noted the gap appeared only once the adjustment was added.
//
// This test drives that exact combination through the engine and compares to the
// genuine DOS engine (amort_oracle) at the SAME solved internal rate. Conclusion
// (reproduced across ~25 differential probes): the engine matches the DOS oracle
// to the digit (0.097301 vs 0.097302). Every sub-combination — adjustment alone,
// adjustment+prepay, adjustment+balloon, off-cycle vs on-cycle balloon, 360 vs
// 365/360 vs 365/360-Exact, fixed vs solved rate — also matches. The solved rate
// matches exactly (internal 0.091267). So the web is APR-faithful to the DOS
// COMPUTATIONAL ENGINE; the DOS APP's 9.8304 is an app-vs-engine split (the app
// diverges from its own engine only when the adjustment is present — consistent
// with the DOS-app stale-state class the client already confirmed in §33).
//
// The oracle side uses `pts=0` (explicit zero points) so DOS runs
// EstimateAndRefineAPRwithPoints — DOS's MakeTable computes APR only when
// pointsstatus > defp, i.e. points explicitly entered even at 0 (Amortize.pas:1420);
// the amort_oracle `pts=` token sets that status. It also uses the `bdate=D.M.Y:AMT`
// off-cycle balloon token added for this investigation.
func TestFancyAPRFullComboVsOracle(t *testing.T) {
	gateOracle(t)

	d := func(y, m, dd int) types.DateRec { return types.NewDateRec(y, time.Month(m), dd) }
	const internalRate = 0.091267 // 365/360-kicked; displayed ≈ 9.0017%

	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanDateStatus: types.InOutInput, LoanDate: d(2024, 1, 1),
			LoanRateStatus: types.InOutInput, LoanRate: internalRate,
			FirstStatus: types.InOutInput, FirstDate: d(2024, 2, 1),
			NStatus: types.InOutInput, NPeriods: 360,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 733.76,
			PointsStatus: types.InOutInput, Points: 0, // explicit 0 → APR computed
		},
		// 365/360 + Exact; REPLACE mode (balloonIncl=YES ⇒ plus_regular=false).
		Settings: Settings{Basis: types.Basis365360, PerYr: 12, Exact: true,
			Prepaid: true, PlusRegular: false, YrDays: 360, YrInv: 1.0 / 360},
		Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: d(2025, 1, 1),
			NNStatus: types.InOutInput, NN: 107,
			PerYrStatus: types.InOutInput, PerYr: 24,
			PaymentStatus: types.InOutInput, Payment: 500,
		}},
		Balloons: []BalloonPayment{{ // OFF-CYCLE: 10/19, not a payment date
			DateStatus: types.InOutInput, Date: d(2026, 10, 19),
			AmountStatus: types.InOutInput, Amount: 700,
		}},
		Adjustments: []RateAdjustment{{ // set-both: new rate AND new payment
			DateStatus: types.InOutInput, Date: d(2027, 1, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.10,
			AmountStatus: types.InOutInput, Amount: 800, AmtOK: true,
		}},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize: %v", res.Err)
	}

	args := []string{
		"100000", strconv.FormatFloat(internalRate, 'f', 6, 64), "360", "12",
		"b365_360", "exact", "payhard=733.76", "prepaid",
		"pre=12:107:24:500", "bdate=19.10.2026:700", "adj=36:0.10:800", "pts=0", "apr",
	}
	dosAPR, ok := runOracleAPR(t, args)
	if !ok {
		t.Skip("oracle produced no APR after retries")
	}
	if diff := res.APR - dosAPR; diff > 1e-4 || diff < -1e-4 {
		t.Errorf("full-combo fancy APR: engine=%.6f oracle=%.6f (Δ=%+.6f)", res.APR, dosAPR, diff)
	} else {
		t.Logf("full-combo fancy APR: engine=%.6f oracle=%.6f — MATCH (web=DOS engine; app's 9.8304 is the app-vs-engine outlier)", res.APR, dosAPR)
	}
}
