package amortization

import (
	"math"
	"math/rand"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// First-principles validation of the FANCY schedule's interest accrual.
//
// A hand-transcribed Pascal reimplementation of the fancy engine would
// be fragile — the balloon "replace vs add" rule (PlusRegular),
// prepayment counting, adjustments and prepaid stubs are each easy to
// mis-encode, which would yield a cross-check that agrees with the port
// only because both share the same transcription error (see
// docs/fidelity_validation_roadmap.md §1). The binary oracle is the
// right tool for full fancy-schedule fidelity.
//
// What IS independently checkable in CI today, without re-deriving the
// engine, is the accounting law every amortization schedule must obey:
// the interest charged in a period equals the OUTSTANDING balance going
// into that period times the period rate. This holds regardless of how
// the payment is composed (regular, balloon, prepayment), so it catches
// a wrong balance being used for interest, a dropped period, or a rate
// applied at the wrong point — the real schedule-construction bugs —
// without encoding the engine's payment conventions.

// fancyAccrualCheck runs a fancy loan and asserts interest_k ==
// prevBalance·(f-1) for every row, where the prior balance of row 1 is
// the loan amount and otherwise the previous row's remaining balance.
// A user-supplied payment makes the engine round per-period interest to
// cents (hard payment), so the comparison allows one cent of slack.
func fancyAccrualCheck(t *testing.T, in LoanInput, label string) {
	t.Helper()
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("%s: %v", label, res.Err)
	}
	f := 1 + in.Loan.LoanRate/float64(in.Loan.PerYr)
	prevBal := in.Loan.Amount
	for k, row := range res.Schedule {
		if row.PayNum == 0 {
			// Settlement-stub row (prepaid mode): interest only, balance
			// unchanged. Not a regular accrual period.
			prevBal = row.Principal
			continue
		}
		want := prevBal * (f - 1)
		if d := math.Abs(row.Interest - want); d > 0.01 {
			t.Fatalf("%s row %d: interest=%.6f, want prevBal·(f-1)=%.6f (prevBal=%.4f, diff=%.4f)",
				label, k, row.Interest, want, prevBal, d)
		}
		prevBal = row.Principal
	}
}

// TestFancyInterestAccrualBalloon: interest accrues correctly across a
// balloon period (whether the balloon replaces or augments the regular
// payment), for both PlusRegular settings.
func TestFancyInterestAccrualBalloon(t *testing.T) {
	for _, plusReg := range []bool{false, true} {
		base := fancyBase(200000, 0.06, 120, 12)
		base.Loan.PayAmtStatus = types.InOutInput
		base.Loan.PayAmt = 2220.41
		base.Settings.PlusRegular = plusReg
		base = withBalloon(base, 40000)
		label := "balloon replace"
		if plusReg {
			label = "balloon add"
		}
		fancyAccrualCheck(t, base, label)
	}
}

// TestFancyInterestAccrualPrepayment: interest accrues correctly across a
// prepayment series.
func TestFancyInterestAccrualPrepayment(t *testing.T) {
	base := fancyBase(250000, 0.05, 180, 12)
	base.Loan.PayAmtStatus = types.InOutInput
	base.Loan.PayAmt = 2000
	base = withPrepayment(base, 200, 36)
	fancyAccrualCheck(t, base, "prepayment")
}

// TestPropertyFancyInterestAccrual sweeps random balloon and prepayment
// loans (constant rate — no adjustments) and checks the accrual law on
// every row.
func TestPropertyFancyInterestAccrual(t *testing.T) {
	rng := rand.New(rand.NewSource(0xACC4)) // accrual
	const N = 500
	for i := 0; i < N; i++ {
		amount := float64(50000 + rng.Intn(950000))
		rate := 0.02 + rng.Float64()*0.10
		years := 5 + rng.Intn(25)
		n := years * 12
		base := fancyBase(amount, rate, n, 12)
		base.Loan.PayAmtStatus = types.InOutInput
		// A roughly-amortizing payment keeps the schedule on its feet.
		f := 1 + rate/12
		base.Loan.PayAmt = amount * (f - 1) / (1 - math.Pow(f, -float64(n)))
		base.Settings.PlusRegular = rng.Intn(2) == 0

		if rng.Intn(2) == 0 {
			base = withBalloon(base, amount*(0.05+0.2*rng.Float64()))
		} else {
			base = withPrepayment(base, 50+float64(rng.Intn(400)), 12+rng.Intn(60))
		}
		fancyAccrualCheck(t, base, "property")
	}
}
