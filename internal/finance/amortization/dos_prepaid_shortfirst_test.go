package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestDOSPrepaidShortFirstVsStub locks the prepaid first-period gating in
// dosIterateSimplePayment (the RepayLoan-terminal Newton that replaced the
// removed schedule-oracle bisection for plain in-advance / odd-first payment
// solves). DOS's global `prorate` (Amortize.pas:1276-1283) is 1 ONLY when the
// loan is taken MORE than one period before the first payment — the settlement
// stub is collected at closing and the first regular period is a FULL period.
// When the loan is taken WITHIN one period of the first payment there is NO stub
// and the first period is the actual SHORT span loanDate→firstDate.
//
// A collapse-step regression: an unconditional loan-date shift (forcing prorate
// = 1 for every prepaid loan) solved the SHORT-first payment high by 1-10%. The
// gate must reproduce BOTH DOS branches. This test drives both against the real
// DOS oracle:
//   - SHORT first: loanDate Jan 1, first payment 3 months out, annual period →
//     first period is a quarter of a period (prorate 0.25), NOT a full period.
//   - STUB: loanDate Jan 1, first payment 15 months out, annual period → more
//     than one full period, so a stub is collected and prorate = 1.
func TestDOSPrepaidShortFirstVsStub(t *testing.T) {
	gateOracle(t)
	type tc struct {
		name       string
		firstMonth int // months from loan date (Jan 1 2024) to first payment
	}
	cases := []tc{
		{"short-first (within one annual period)", 3},
		{"stub (more than one annual period)", 15},
	}
	const amount, rate, n, perYr = 271269.0, 0.1420, 8, 1
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dos, ok := runOraclePayment(amount, rate, n, perYr,
				"prepaid", "first="+itoa(c.firstMonth))
			if !ok {
				t.Skip("oracle produced no payment")
			}
			fy := 2024 + c.firstMonth/12
			fm := time.Month(c.firstMonth%12 + 1)
			in := LoanInput{
				Loan: Loan{
					AmountStatus: types.InOutInput, Amount: amount,
					LoanRateStatus: types.InOutInput, LoanRate: rate,
					NStatus: types.InOutInput, NPeriods: n,
					PerYrStatus: types.InOutInput, PerYr: perYr,
					PayAmtStatus:   types.StatusEmpty,
					LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
					FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(fy, fm, 1),
				},
				Settings: Settings{Basis: types.Basis360, Prepaid: true,
					PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360},
			}
			res := Amortize(in)
			if res.Err != nil || len(res.Schedule) == 0 {
				t.Fatalf("Amortize: err=%v rows=%d", res.Err, len(res.Schedule))
			}
			got := modalReg(res.Schedule)
			rel := math.Abs(dos-got) / math.Max(1, got)
			if rel > 1e-3 {
				t.Errorf("prepaid %s: DOS=%.4f Go=%.4f (rel %.2e) — first-period gate wrong",
					c.name, dos, got, rel)
			}
		})
	}
}
