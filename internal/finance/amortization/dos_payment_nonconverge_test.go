package amortization

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestAmortPaymentSolveNonConvergeMatchesDOS guards the 2026-07-16 fuzzer3
// finding: a blank-payment solve the DOS engine's Iterate cannot converge.
//
// DOS solves the regular payment with Iterate over RepayLoan (AMORTOP.pas:1437);
// EstimateAndRefinePayment takes the closed-form shortcut ONLY for a
// `(not exact) and prepaid and (not in_advance)` loan (Amortize.pas:402-408),
// so an ordinary in-arrears loan STILL iterates. On an ill-conditioned
// high-rate/long-term loan the schedule's terminal balance is astronomically
// steep in the payment (a knife-edge root), so Iterate exhausts its 20-step
// budget above tolerance and DOS blocks with "Computation of payment amount or
// interest rate did not converge." and NO schedule (AMORTOP.pas:1489).
//
// The port previously SKIPPED Iterate for that arrears natural-first case
// (needPaymentRefine was false), used the closed-form annuity payment, and
// rendered a degenerate schedule — negative amortization with a terminating
// balloon many times the principal (fuzzer3: a $37,188,140 balloon on a $3.9M
// loan). Now the blank-payment solve runs DOS's own Iterate (seeded from DOS's
// raw annuity estimate) and honors its convergence verdict, so the two agree.
//
// Oracle provenance (amort_oracle, loandmy=1.1.2020 firstdmy=1.4.2020 b365):
//
//	3893589.94 0.2757590000 153 4 solvepayment → ERR …did not converge   (refuses)
//	 500000    0.2800000000 200 4 solvepayment → ERR …did not converge   (refuses)
//	 500000    0.2800000000 120 4 solvepayment → payment 34997.9108      (solves)
//	 100000    0.2500000000  40 4 solvepayment → payment  6854.4709      (solves)
func TestAmortPaymentSolveNonConvergeMatchesDOS(t *testing.T) {
	ld := types.NewDateRec(2020, time.January, 1)
	fd := types.NewDateRec(2020, time.April, 1)
	mkLoan := func(amount, rate float64, nper int) LoanInput {
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: nper,
			PerYrStatus: types.InOutInput, PerYr: 4,
			LoanDateStatus: types.InOutInput, LoanDate: ld,
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: Settings{Basis: types.Basis365}}
	}

	// Ill-conditioned: DOS refuses — the port must too (Err, no schedule).
	for _, tc := range []struct {
		tag          string
		amount, rate float64
		nper         int
	}{
		{"case#1 3.9M@27.58%/153q", 3893589.94, 0.275759, 153},
		{"500k@28%/200q", 500000, 0.28, 200},
	} {
		res := Amortize(mkLoan(tc.amount, tc.rate, tc.nper))
		if res.Err == nil || !strings.Contains(res.Err.Error(), "did not converge") {
			t.Errorf("%s: want a 'did not converge' error (DOS refuses); got Err=%v", tc.tag, res.Err)
		}
		if len(res.Schedule) != 0 {
			t.Errorf("%s: must return NO schedule (DOS blocks); got %d rows", tc.tag, len(res.Schedule))
		}
	}

	// Well-conditioned high-rate: DOS solves — the port must return a schedule
	// with the DOS payment (from the FIRST regular row), not refuse.
	for _, tc := range []struct {
		tag          string
		amount, rate float64
		nper         int
		wantPay      float64
	}{
		{"500k@28%/120q", 500000, 0.28, 120, 34997.9108},
		{"100k@25%/40q", 100000, 0.25, 40, 6854.4709},
	} {
		res := Amortize(mkLoan(tc.amount, tc.rate, tc.nper))
		if res.Err != nil {
			t.Errorf("%s: must NOT error (DOS solves %.4f); got Err=%v", tc.tag, tc.wantPay, res.Err)
			continue
		}
		if len(res.Schedule) == 0 {
			t.Errorf("%s: must return a schedule", tc.tag)
			continue
		}
		got := res.Schedule[0].PayAmt
		if diff := got - tc.wantPay; diff > 0.02 || diff < -0.02 {
			t.Errorf("%s: regular payment = %.4f, want DOS %.4f (Δ=%.4f)", tc.tag, got, tc.wantPay, diff)
		}
	}
}
