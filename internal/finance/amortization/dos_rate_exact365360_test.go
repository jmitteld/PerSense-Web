package amortization

import (
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// TestRateSolveExact365360Prepaid pins the 2026-07-22 client-reported rate-solve
// "off by 2%" case and records the adjudication: the WEB/engine is correct and
// the shipped DOS APP has a display bug on this exact + 365/360 rate solve.
//
// Scenario (from the client screenshots): Amount 100,000; Payment 750; 360
// periods; 12/yr; loan 01/01/2025, 1st pmt 02/01/2025; Basis 365/360; Exact YES;
// 1st-interest-prepaid YES; arrears; Rate BLANK (solve it).
//
//   - The DOS *headless engine* (pv/amort source-oracle, same Pascal) solves an
//     engine-space rate of 0.0811622 — feeding it forward reproduces the $750
//     payment to the cent (verified via the oracle: forward at 0.081162 → 749.9988).
//   - The Go engine's SolveRate returns the same engine-space rate (within 5e-5);
//     the handler then UN-kicks it for display: 0.081162 × 360/365 ≈ 0.08005, i.e.
//     the 8.0050% the web shows. Re-entering 8.0050% (which the handler re-kicks)
//     reproduces $750 — the display round-trips.
//   - The shipped DOS APP instead displays 6.2160% (APR 6.3021%). That value does
//     NOT round-trip: 6.2160% kicked to 6.3023% engine-space amortizes to a $625
//     payment, not $750 (oracle: forward → 625.01). In fact the app's stored rate
//     is EXACTLY the correct solve for payment 625.00 = 750 × 5/6 — its Newton
//     converged on corrupted money (stale or ×5/6-mis-scaled payment/amount), not
//     on a bad rate. Full adjudication + mechanism: docs/discrepancies.md §33.
//
// This test guards the engine-space solve against the oracle and asserts the
// solved rate round-trips to the $750 payment, so a future change can't silently
// "correct" the port toward the DOS-app display bug.
func TestRateSolveExact365360Prepaid(t *testing.T) {
	gateOracle(t)

	const (
		amount = 100000.0
		nper   = 360
		perYr  = 12
		pay    = 750.0
	)
	loanDate := types.NewDateRec(2025, time.January, 1)
	firstDate := types.NewDateRec(2025, time.February, 1)
	ctx := interest.NewCalcContext(types.Basis365360, perYr)

	mk := func(rateStatus int8, rate float64, payStatus int8, payAmt float64) LoanInput {
		return LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amount,
				LoanDateStatus: types.InOutInput, LoanDate: loanDate,
				LoanRateStatus: rateStatus, LoanRate: rate,
				FirstStatus: types.InOutInput, FirstDate: firstDate,
				NStatus: types.InOutInput, NPeriods: nper,
				PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus: payStatus, PayAmt: payAmt,
				PointsStatus: types.InOutInput, Points: 0,
			},
			Settings: Settings{
				Basis: types.Basis365360, PerYr: perYr,
				Prepaid: true, Exact: true,
				YrDays: ctx.YrDays, YrInv: ctx.YrInv,
			},
		}
	}

	// 1) Engine solves the rate; compare (engine-space) to the DOS oracle.
	goRate, conv, err := SolveRate(mk(types.StatusEmpty, 0, types.InOutInput, pay))
	if err != nil {
		t.Fatalf("SolveRate: %v", err)
	}
	if !conv {
		t.Fatalf("SolveRate did not converge (rate=%.6f)", goRate)
	}
	args := []string{
		strconv.FormatFloat(amount, 'f', 2, 64), "0",
		strconv.Itoa(nper), strconv.Itoa(perYr),
		"payhard=" + strconv.FormatFloat(pay, 'f', 2, 64),
		"loandmy=1.1.2025", "firstdmy=1.2.2025", "solverate",
		"exact", "b365_360", "prepaid",
	}
	dosRate, ok := runOracleRate(t, args)
	if !ok {
		t.Skip("oracle produced no rate after retries (heap glitch)")
	}
	if diff := goRate - dosRate; diff > 5e-5 || diff < -5e-5 {
		t.Errorf("engine-space rate mismatch: engine=%.6f oracle=%.6f (Δ=%+.6f)", goRate, dosRate, diff)
	}
	// Sanity: the correct engine rate is ~8.116%, NOT the DOS-app's 6.216%×365/360.
	if goRate < 0.080 || goRate > 0.082 {
		t.Errorf("engine rate %.6f is outside the expected ~0.08116 band — "+
			"a value near 0.063 would mean the port reproduced the DOS-app solve bug", goRate)
	}

	// 2) Round-trip: feed the solved engine rate forward (rate given, payment
	//    blank) and confirm the schedule's payment retires at $750.
	fwd := Amortize(mk(types.InOutInput, goRate, types.StatusEmpty, 0))
	if fwd.Err != nil {
		t.Fatalf("forward round-trip errored: %v", fwd.Err)
	}
	// The regular payment is the modal per-period payment of the schedule.
	pmt := payoffRegularPayment(fwd, mk(types.InOutInput, goRate, types.StatusEmpty, 0).Loan)
	if pmt < 749.5 || pmt > 750.5 {
		t.Errorf("round-trip payment %.4f, want ≈750.00 (the solved rate must reproduce the input payment)", pmt)
	}
}
