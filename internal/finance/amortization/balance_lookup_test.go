package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestBalanceAtDateWithBalloon verifies dispatch_gaps FP7 / AO14:
// BalanceAtDate reads the engine-recorded balance, so it stays
// correct across a balloon payment — a payment-minus-interest walk
// would miss the balloon's principal drop.
func TestBalanceAtDateWithBalloon(t *testing.T) {
	in := baseInput30y()
	balloonDate := types.NewDateRec(2034, time.January, 1) // 10 years in
	in.Balloons = []BalloonPayment{{
		DateStatus:   types.InOutInput,
		Date:         balloonDate,
		AmountStatus: types.InOutInput,
		Amount:       50000,
	}}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}

	// The balloon month's balance drop dwarfs a normal month's
	// principal reduction — proof the lookup sees the balloon.
	before := BalanceAtDate(res.Schedule, in.Loan.Amount,
		types.NewDateRec(2033, time.December, 1))
	after := BalanceAtDate(res.Schedule, in.Loan.Amount, balloonDate)
	drop := before - after
	if drop < 40000 {
		t.Errorf("balance drop across the balloon = %.2f, expected a balloon-sized drop", drop)
	}

	// Balance must equal a schedule row's recorded principal exactly.
	mid := BalanceAtDate(res.Schedule, in.Loan.Amount,
		types.NewDateRec(2030, time.June, 1))
	var want float64 = -1
	for _, r := range res.Schedule {
		if r.Date.Time.Year() == 2030 && r.Date.Time.Month() == time.June {
			want = r.Principal
		}
	}
	if want >= 0 && math.Abs(mid-want) > 0.001 {
		t.Errorf("BalanceAtDate = %.2f, want the recorded row balance %.2f", mid, want)
	}
}

// TestDateForBalance verifies dispatch_gaps R4-9: the inverse lookup
// — given a target balance, find the date it is first reached.
func TestDateForBalance(t *testing.T) {
	in := baseInput30y()
	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// Pick a target equal to a known row's recorded balance; the
	// lookup must return that row's date (or an earlier one with the
	// same-or-lower balance — balances are monotonically decreasing
	// here, so it is exactly that row).
	mid := res.Schedule[120].Principal
	d, ok := DateForBalance(res.Schedule, mid)
	if !ok {
		t.Fatal("DateForBalance: target not reached")
	}
	got := BalanceAtDate(res.Schedule, in.Loan.Amount, d)
	if got > mid+0.01 {
		t.Errorf("balance at the returned date (%.2f) should be <= target %.2f", got, mid)
	}
	// A target below zero is never reached.
	if _, ok := DateForBalance(res.Schedule, -1); ok {
		t.Errorf("a negative target should not be reportable as reached")
	}
}

// TestBalanceBeforeFirstPayment and TestBalanceOnR78Schedule verify
// dispatch_gaps V6-3: BalanceAtDate is correct in the pre-first-
// payment window and on a Rule-of-78 schedule (it reads the recorded
// per-row principal, which the R78 engine populates correctly).
func TestBalanceBeforeFirstPayment(t *testing.T) {
	in := baseInput30y()
	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// A date before the first payment: balance is the full loan amount.
	early := types.NewDateRec(2024, time.January, 15)
	if got := BalanceAtDate(res.Schedule, in.Loan.Amount, early); got != in.Loan.Amount {
		t.Errorf("balance before first payment = %.2f, want the loan amount %.2f",
			got, in.Loan.Amount)
	}
}

func TestBalanceOnR78Schedule(t *testing.T) {
	in := baseInput30y()
	in.Fancy = false
	in.Settings.R78 = true
	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// Rule-of-78 front-loads interest, so the balance may rise in the
	// early periods (negative amortization) before falling — what
	// matters is that it pays off at term and that BalanceAtDate
	// reads a recorded row balance consistently.
	last := res.Schedule[len(res.Schedule)-1].Principal
	if last < -1 || last > 1 {
		t.Errorf("R78 final balance = %.2f, want ~0", last)
	}
	mid := res.Schedule[180]
	if got := BalanceAtDate(res.Schedule, in.Loan.Amount, mid.Date); got != mid.Principal {
		t.Errorf("BalanceAtDate on the R78 schedule = %.2f, want the recorded %.2f",
			got, mid.Principal)
	}
}
