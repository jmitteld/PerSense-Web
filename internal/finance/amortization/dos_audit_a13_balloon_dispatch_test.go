package amortization

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func a13loan(pay float64) Loan {
	l := Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.06,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 120,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}
	if pay > 0 {
		l.PayAmtStatus = types.InOutInput
		l.PayAmt = pay
	}
	return l
}

// A13a: an unknown balloon (date, no amount) with the payment ALSO blank is
// INSUFFICIENT DATA in DOS — SufficientDataOnScreen (Amortize.pas:889-891)
// admits `unkballoon > 0` only when payamtstatus >= defp. Verified:
// `amort_oracle 100000 0.06 120 12 dateballoon=37` → no schedule, payment
// unsolved, balloon amountstatus stays empty.
func TestA13UnknownBalloonNeedsPayment(t *testing.T) {
	in := LoanInput{Loan: a13loan(0), Settings: basicSettings(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2027, time.February, 1)}}}
	res := Amortize(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "not enough data") {
		t.Errorf("unknown balloon + blank payment must refuse like DOS, got err=%v", res.Err)
	}
	// Same rule for an unknown prepayment (Amortize.pas:893-895). Verified:
	// `amort_oracle 100000 0.06 120 12 presolve=6:12:12` → prepay 0.0000 (unsolved).
	in2 := LoanInput{Loan: a13loan(0), Settings: basicSettings(), Fancy: true,
		Prepayments: []Prepayment{{StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.July, 1),
			PerYrStatus: types.InOutInput, PerYr: 12, NNStatus: types.InOutInput, NN: 12}}}
	res2 := Amortize(in2)
	if res2.Err == nil || !strings.Contains(res2.Err.Error(), "not enough data") {
		t.Errorf("unknown prepayment + blank payment must refuse like DOS, got err=%v", res2.Err)
	}
}

// A13b: iterative solves run UNROUNDED walks — DOS Iterate sets hard_payment
// false "temporarily, for iteration" (AMORTOP.pas:1433); Round2 is display-only.
// Solving against the rounded walk shifted the balloon root ~1.6¢. Verified:
//
//	amort_oracle 100000 0.06 120 12 payhard=1110.21 solveballoon=37 → balloon 1109.6700
//	amort_oracle 100000 0.06 120 12 payhard=800 presolve=6:12:12    → prepay 3265.5268
func TestA13SolveWalksUnrounded(t *testing.T) {
	in := LoanInput{Loan: a13loan(1110.21), Settings: basicSettings(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2027, time.February, 1)}}}
	amt, err := SolveBalloonAmount(in, 0)
	if err != nil || math.Abs(amt-1109.6700) > 0.005 {
		t.Errorf("mid-term balloon solve = %.4f err=%v, want 1109.6700 ±0.005 (DOS)", amt, err)
	}
	in2 := LoanInput{Loan: a13loan(800), Settings: basicSettings(), Fancy: true,
		Prepayments: []Prepayment{{StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.July, 1),
			PerYrStatus: types.InOutInput, PerYr: 12, NNStatus: types.InOutInput, NN: 12}}}
	amt2, err2 := SolvePrepaymentAmount(in2, 0)
	if err2 != nil || math.Abs(amt2-3265.5268) > 0.01 {
		t.Errorf("prepay solve = %.4f err=%v, want 3265.5268 ±0.01 (DOS)", amt2, err2)
	}
}
