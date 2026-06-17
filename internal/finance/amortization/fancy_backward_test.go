package amortization

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// midTermDate returns the date of payment period k for a monthly loan
// whose first payment is 2024-02-01, used to place a balloon strictly
// inside the loan term.
func midTermDate(k int) types.DateRec {
	d := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC).AddDate(0, k-1, 0)
	return types.NewDateRec(d.Year(), d.Month(), d.Day())
}

// Round-trip validation of the fancy backward solvers (amount / rate /
// payment under balloons, prepayments, and rate adjustments) implemented
// by bisection against the forward engine. The test pattern: build a
// fancy loan, solve its payment so the system is self-consistent, then
// blank one field at a time and confirm the solver recovers it, and that
// the recovered loan amortizes (the forward schedule retires it over the
// full term with no leftover balance).

func fancyBase(amount, rate float64, n, perYr int) LoanInput {
	return LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, 2, 1),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360},
		Fancy:    true,
	}
}

// amortizesCleanly reports whether the loan (with the given regular
// payment) retires over its full scheduled term with essentially no
// leftover balance — the defining property of a correctly-solved field.
func amortizesCleanly(t *testing.T, in LoanInput, payment float64) {
	t.Helper()
	chk := in
	chk.Loan.AmountStatus = types.InOutInput
	chk.Loan.LoanRateStatus = types.InOutInput
	chk.Loan.PayAmtStatus = types.InOutInput
	chk.Loan.PayAmt = payment
	res := Amortize(chk)
	if res.Err != nil {
		t.Fatalf("plug-back Amortize: %v", res.Err)
	}
	if res.FinalPrinc > 0.5 || res.FinalPrinc < -0.5 {
		t.Errorf("loan does not amortize: FinalPrinc=%.4f", res.FinalPrinc)
	}
	if len(res.Schedule) < in.Loan.NPeriods {
		t.Errorf("loan retired early: %d of %d periods", len(res.Schedule), in.Loan.NPeriods)
	}
}

func withBalloon(in LoanInput, amount float64) LoanInput {
	// Place the balloon strictly inside the term (mid-point) so it lands
	// on a real payment period rather than past the last payment.
	in.Balloons = []BalloonPayment{{
		DateStatus: types.InOutInput, Date: midTermDate(in.Loan.NPeriods / 2),
		AmountStatus: types.InOutInput, Amount: amount,
	}}
	return in
}

func withPrepayment(in LoanInput, perPmt float64, nn int) LoanInput {
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2025, 2, 1),
		PerYrStatus: types.InOutInput, PerYr: 12,
		PaymentStatus: types.InOutInput, Payment: perPmt,
		NNStatus: types.InOutInput, NN: nn,
	}}
	return in
}

// TestFancyBackwardBalloonRoundTrip: under a balloon, solve payment, then
// recover amount and rate from it.
func TestFancyBackwardBalloonRoundTrip(t *testing.T) {
	const amount, rate, n, perYr = 200000.0, 0.06, 120, 12
	base := withBalloon(fancyBase(amount, rate, n, perYr), 40000)

	// Solve the payment that amortizes the balloon loan.
	payIn := base
	payIn.Loan.PayAmtStatus = types.StatusEmpty
	d, err := SolvePayment(payIn)
	if err != nil {
		t.Fatalf("SolvePayment: %v", err)
	}
	amortizesCleanly(t, base, d)

	// Recover the amount from (rate, payment, balloon).
	amtIn := base
	amtIn.Loan.AmountStatus = types.StatusEmpty
	amtIn.Loan.PayAmtStatus = types.InOutInput
	amtIn.Loan.PayAmt = d
	gotAmt, conv, err := SolveLoanAmount(amtIn)
	if err != nil || !conv {
		t.Fatalf("SolveLoanAmount: conv=%v err=%v", conv, err)
	}
	if rd := math.Abs(gotAmt-amount) / amount; rd > 1e-3 {
		t.Errorf("amount round-trip: got %.2f want %.2f (rel %.2e)", gotAmt, amount, rd)
	}

	// Recover the rate from (amount, payment, balloon).
	rateIn := base
	rateIn.Loan.LoanRateStatus = types.StatusEmpty
	rateIn.Loan.PayAmtStatus = types.InOutInput
	rateIn.Loan.PayAmt = d
	gotRate, conv, err := SolveRate(rateIn)
	if err != nil || !conv {
		t.Fatalf("SolveRate: conv=%v err=%v", conv, err)
	}
	if math.Abs(gotRate-rate) > 1e-4 {
		t.Errorf("rate round-trip: got %.6f want %.6f", gotRate, rate)
	}
}

// TestFancyBackwardPrepaymentRoundTrip: under a prepayment series, solve
// payment, then recover amount from it.
func TestFancyBackwardPrepaymentRoundTrip(t *testing.T) {
	const amount, rate, n, perYr = 250000.0, 0.05, 180, 12
	base := withPrepayment(fancyBase(amount, rate, n, perYr), 150, 36)

	payIn := base
	payIn.Loan.PayAmtStatus = types.StatusEmpty
	d, err := SolvePayment(payIn)
	if err != nil {
		t.Fatalf("SolvePayment: %v", err)
	}
	amortizesCleanly(t, base, d)

	amtIn := base
	amtIn.Loan.AmountStatus = types.StatusEmpty
	amtIn.Loan.PayAmtStatus = types.InOutInput
	amtIn.Loan.PayAmt = d
	gotAmt, conv, err := SolveLoanAmount(amtIn)
	if err != nil || !conv {
		t.Fatalf("SolveLoanAmount: conv=%v err=%v", conv, err)
	}
	if rd := math.Abs(gotAmt-amount) / amount; rd > 1e-3 {
		t.Errorf("amount round-trip (prepay): got %.2f want %.2f (rel %.2e)", gotAmt, amount, rd)
	}
}

// TestFancyBackwardPropertyRoundTrip sweeps random balloon loans: solve
// the payment, recover the amount, and confirm both the recovery and that
// the loan amortizes cleanly.
func TestFancyBackwardPropertyRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0xBA110011))
	const N = 400
	checked := 0
	for i := 0; i < N; i++ {
		amount := float64(50000 + rng.Intn(950000))
		rate := 0.02 + rng.Float64()*0.10
		years := 5 + rng.Intn(25)
		n := years * 12
		base := withBalloon(fancyBase(amount, rate, n, 12), amount*(0.1+0.3*rng.Float64()))

		payIn := base
		payIn.Loan.PayAmtStatus = types.StatusEmpty
		d, err := SolvePayment(payIn)
		if err != nil || d <= 0 {
			continue
		}
		// The solved payment must amortize the loan.
		chk := base
		chk.Loan.PayAmtStatus = types.InOutInput
		chk.Loan.PayAmt = d
		r := Amortize(chk)
		if r.Err != nil {
			continue
		}
		if math.Abs(r.FinalPrinc) > 1.0 {
			t.Fatalf("iter %d: solved payment %.4f leaves FinalPrinc=%.4f (amt=%.0f r=%.4f n=%d)", i, d, r.FinalPrinc, amount, rate, n)
		}

		// Recover the amount from the solved payment.
		amtIn := base
		amtIn.Loan.AmountStatus = types.StatusEmpty
		amtIn.Loan.PayAmtStatus = types.InOutInput
		amtIn.Loan.PayAmt = d
		gotAmt, conv, err := SolveLoanAmount(amtIn)
		if err != nil || !conv {
			t.Fatalf("iter %d: SolveLoanAmount conv=%v err=%v", i, conv, err)
		}
		if rd := math.Abs(gotAmt-amount) / amount; rd > 2e-3 {
			t.Fatalf("iter %d: amount round-trip got %.2f want %.2f (rel %.2e) [r=%.4f n=%d]", i, gotAmt, amount, rd, rate, n)
		}
		checked++
	}
	if checked < 100 {
		t.Fatalf("only %d fancy round trips exercised", checked)
	}
}
