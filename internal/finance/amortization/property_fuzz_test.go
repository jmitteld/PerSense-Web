package amortization

import (
	"math"
	"math/rand"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Property-based testing for the amortization engine: thousands of
// randomized loans asserting invariants that must hold for ANY input —
// the schedule fully amortizes, balances fall monotonically, each
// period's interest is the prior balance times the period rate, the
// totals reconcile, and the backward solvers round-trip. Seeded so any
// failure reproduces. This is the engine analogue of the PV property
// rig and the front half of a future binary-oracle differential sweep.

func randVanilla(rng *rand.Rand) (amount, rate float64, n, perYr int) {
	amount = float64(1000 + rng.Intn(2_000_000))
	rate = 0.005 + rng.Float64()*0.175 // 0.5%..18%
	perYr = []int{1, 2, 4, 12}[rng.Intn(4)]
	// Term: 2..40 years, in periods, at least 2.
	years := 2 + rng.Intn(39)
	n = years * perYr
	if n < 2 {
		n = 2
	}
	return
}

func firstDateForPerYr(perYr int) types.DateRec {
	switch perYr {
	case 12:
		return types.NewDateRec(2024, 2, 1)
	case 4:
		return types.NewDateRec(2024, 4, 1)
	case 2:
		return types.NewDateRec(2024, 7, 1)
	default:
		return types.NewDateRec(2025, 1, 1)
	}
}

func vanillaInput(amount, rate float64, n, perYr int) LoanInput {
	return LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstDateForPerYr(perYr),
			PayAmtStatus: types.StatusEmpty, // solved → no hard rounding
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360},
	}
}

func fin(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// TestPropertyVanillaScheduleInvariants sweeps random vanilla loans and
// checks the schedule's structural invariants.
func TestPropertyVanillaScheduleInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(0xA404))
	const N = 3000
	for i := 0; i < N; i++ {
		amount, rate, n, perYr := randVanilla(rng)
		res := Amortize(vanillaInput(amount, rate, n, perYr))
		if res.Err != nil {
			t.Fatalf("iter %d (amt=%.0f r=%.4f n=%d py=%d): %v", i, amount, rate, n, perYr, res.Err)
		}
		if len(res.Schedule) != n {
			t.Fatalf("iter %d: schedule has %d rows, want %d", i, len(res.Schedule), n)
		}
		f := 1 + rate/float64(perYr)

		prevBal := amount
		var sumInt, sumPaid float64
		for k, row := range res.Schedule {
			if !fin(row.Interest) || !fin(row.Principal) || !fin(row.PayAmt) {
				t.Fatalf("iter %d row %d: non-finite (int=%v bal=%v pay=%v)", i, k, row.Interest, row.Principal, row.PayAmt)
			}
			// interest == prior balance * period rate.
			wantInt := prevBal * (f - 1)
			if d := math.Abs(row.Interest - wantInt); d > 1e-6*math.Max(1, math.Abs(wantInt)) {
				t.Fatalf("iter %d row %d: interest=%.8f want prevBal*(f-1)=%.8f", i, k, row.Interest, wantInt)
			}
			// balance falls monotonically (until it hits ~0 at the end).
			if row.Principal > prevBal+1e-6 {
				t.Fatalf("iter %d row %d: balance rose %.4f -> %.4f", i, k, prevBal, row.Principal)
			}
			sumInt += row.Interest
			sumPaid += row.PayAmt
			prevBal = row.Principal
		}
		// Fully amortized and totals reconcile.
		if math.Abs(res.FinalPrinc) > 0.05 {
			t.Fatalf("iter %d: FinalPrinc=%.6f (amt=%.0f r=%.4f n=%d py=%d)", i, res.FinalPrinc, amount, rate, n, perYr)
		}
		if d := math.Abs(sumPaid - sumInt - amount); d > 0.05+1e-7*amount {
			t.Fatalf("iter %d: paid(%.4f)-int(%.4f) != amount(%.4f), off %.4f", i, sumPaid, sumInt, amount, d)
		}
	}
}

// TestPropertyBackwardRoundTrips sweeps random vanilla loans and checks
// the common backward solvers invert each other: solve the payment from
// (amount, rate, n); recover the amount from that payment; recover the
// rate from amount+payment.
func TestPropertyBackwardRoundTrips(t *testing.T) {
	rng := rand.New(rand.NewSource(0xB17E))
	const N = 2000
	for i := 0; i < N; i++ {
		amount, rate, n, perYr := randVanilla(rng)
		base := vanillaInput(amount, rate, n, perYr)

		// Solve payment.
		d, err := SolvePaymentClosedForm(base)
		if err != nil || !fin(d) || d <= 0 {
			t.Fatalf("iter %d: SolvePaymentClosedForm failed: d=%v err=%v", i, d, err)
		}

		// Recover amount from the solved payment.
		amtIn := base
		amtIn.Loan.AmountStatus = types.StatusEmpty
		amtIn.Loan.Amount = 0
		amtIn.Loan.PayAmtStatus = types.InOutInput
		amtIn.Loan.PayAmt = d
		gotAmt, _, err := SolveLoanAmount(amtIn)
		if err != nil {
			t.Fatalf("iter %d: SolveLoanAmount: %v", i, err)
		}
		if rd := math.Abs(gotAmt-amount) / amount; rd > 1e-6 {
			t.Fatalf("iter %d: amount round-trip got %.4f want %.4f (rel %.2e) [r=%.4f n=%d py=%d d=%.4f]",
				i, gotAmt, amount, rd, rate, n, perYr, d)
		}

		// Recover rate from amount + payment.
		rateIn := base
		rateIn.Loan.LoanRateStatus = types.StatusEmpty
		rateIn.Loan.LoanRate = 0
		rateIn.Loan.PayAmtStatus = types.InOutInput
		rateIn.Loan.PayAmt = d
		gotRate, _, err := SolveRate(rateIn)
		if err != nil {
			t.Fatalf("iter %d: SolveRate: %v", i, err)
		}
		if math.Abs(gotRate-rate) > 1e-6 {
			t.Fatalf("iter %d: rate round-trip got %.8f want %.8f [amt=%.0f n=%d py=%d]",
				i, gotRate, rate, amount, n, perYr)
		}
	}
}

// TestPropertyFancyForwardInvariants exercises the FANCY engine (a
// balloon and/or an extra prepayment series) over random loans, checking
// only the forward invariants that must always hold: finiteness, full
// amortization, and that principal paid equals the loan amount. Backward
// solving under advanced options is the deferred Iterate gap and is not
// asserted here.
func TestPropertyFancyForwardInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(0xFA17))
	const N = 1500
	checked := 0
	for i := 0; i < N; i++ {
		amount, rate, n, perYr := randVanilla(rng)
		if perYr != 12 || n < 24 {
			continue // keep fancy cases on a monthly grid with room for a balloon
		}
		in := vanillaInput(amount, rate, n, perYr)
		// Solve a base payment, then supply it so the loan is well-posed
		// with the balloon reducing the term/last payment.
		d, err := SolvePaymentClosedForm(in)
		if err != nil {
			continue
		}
		in.Loan.PayAmtStatus = types.InOutInput
		in.Loan.PayAmt = d
		// A balloon at the midpoint for a fraction of the amount.
		balloonDate := types.NewDateRec(2024+(n/12)/2, 2, 1)
		in.Balloons = []BalloonPayment{{
			DateStatus: types.InOutInput, Date: balloonDate,
			AmountStatus: types.InOutInput, Amount: amount * 0.1,
		}}
		in.Fancy = true

		res := Amortize(in)
		if res.Err != nil {
			continue // some random combos are legitimately rejected
		}
		for k, row := range res.Schedule {
			if !fin(row.Interest) || !fin(row.Principal) || !fin(row.PayAmt) {
				t.Fatalf("iter %d row %d: non-finite in fancy schedule", i, k)
			}
		}
		if math.Abs(res.FinalPrinc) > 1.0 {
			t.Fatalf("iter %d: fancy FinalPrinc=%.4f (amt=%.0f r=%.4f n=%d)", i, res.FinalPrinc, amount, rate, n)
		}
		if d := math.Abs(res.TotalPaid - res.TotalInt - amount); d > 1.0+1e-6*amount {
			t.Fatalf("iter %d: fancy paid-int != amount, off %.4f", i, d)
		}
		checked++
	}
	if checked < 100 {
		t.Fatalf("only %d fancy cases exercised", checked)
	}
}
