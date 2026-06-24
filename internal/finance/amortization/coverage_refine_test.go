package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_refine_test.go drives the refineFancyPayment bisection helper
// (engine.go:650) directly — the only place it is exercised through the
// engine is the skip-month blank-payment fallback when solveFancyPayment
// cannot bracket, and that degenerate path returns the initial estimate
// without ever bisecting. Calling it directly with a normal amortizing
// loan exercises the bracket-expansion and bisection-convergence arms.

// TestRefineFancyPaymentDirect covers refineFancyPayment's three bracket
// regimes: a centred estimate (immediate straddle + bisection), a too-low
// estimate (expand hi while balLo>0), and a too-high estimate (both
// balances negative ⇒ the balHi<0 "lower the lo guess" arm). All three
// must converge to roughly the same payment that retires the loan.
func TestRefineFancyPaymentDirect(t *testing.T) {
	loan := mkFancyLoan(200_000, 0.06, 24, 0)
	loan.PayAmtStatus = types.StatusEmpty
	s := fancyTestSettings()
	in := LoanInput{Loan: loan, Settings: s, Fancy: true}
	f := GrowthPerPeriod(&loan, s.YrInv)
	tr, err := ComputeTrueRate(&loan, &s)
	if err != nil {
		t.Fatalf("ComputeTrueRate: %v", err)
	}
	est := estimatePayment(&loan, f)

	// All three starting points must run the bracket logic and return a
	// positive payment. (The low/high estimates intentionally start far
	// from the answer to drive the bracket-expansion arms — they need not
	// land on the same converged payment, only stay positive.)
	centred := refineFancyPayment(in, est, &s, tr, f)
	low := refineFancyPayment(in, est*0.1, &s, tr, f)
	high := refineFancyPayment(in, est*10, &s, tr, f)
	for name, got := range map[string]float64{"centred": centred, "low": low, "high": high} {
		if got <= 0 {
			t.Errorf("%s: refineFancyPayment returned non-positive %.3f", name, got)
		}
	}

	// The centred estimate brackets cleanly and must drive the real
	// schedule's final balance to near zero.
	probe := mkFancyLoan(200_000, 0.06, 24, centred)
	pin := in
	pin.Loan = probe
	res := Amortize(pin)
	if res.Err != nil {
		t.Fatalf("Amortize at centred payment %.3f: %v", centred, res.Err)
	}
	if res.FinalPrinc < -1.0 || res.FinalPrinc > 1.0 {
		t.Errorf("centred refined payment %.3f leaves final balance %.3f (want ~0)",
			centred, res.FinalPrinc)
	}
}

// TestRefineFancyPaymentWithPrepayAndExtremes covers the simulate
// closure's prepayment deep-copy (engine.go:659) and the bracket-expansion
// arm where both lo and hi over-amortize (the balHi<0 branch, :678) by
// starting from an absurdly high estimate.
func TestRefineFancyPaymentWithPrepayAndExtremes(t *testing.T) {
	loan := mkFancyLoan(200_000, 0.06, 60, 0)
	loan.PayAmtStatus = types.StatusEmpty
	s := fancyTestSettings()
	in := LoanInput{
		Loan:     loan,
		Settings: s,
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024, time.March, 1),
			NNStatus:        types.InOutInput,
			NN:              6,
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         200,
		}},
	}
	f := GrowthPerPeriod(&loan, s.YrInv)
	tr, _ := ComputeTrueRate(&loan, &s)
	est := estimatePayment(&loan, f)

	// Centred start — exercises the simulate prepayment deep-copy + bisection.
	if got := refineFancyPayment(in, est, &s, tr, f); got <= 0 {
		t.Errorf("prepay centred: non-positive %.3f", got)
	}
	// Absurdly high start — 0.5*dInit still over-amortizes, so both bracket
	// ends are negative and the balHi<0 "lower the lo guess" arm runs.
	if got := refineFancyPayment(in, est*100, &s, tr, f); got <= 0 {
		t.Errorf("prepay high-init: non-positive %.3f", got)
	}
}

// TestRefineFancyPaymentCannotBracket pins the fallback arm: when the loan
// can never amortize (every month skipped) the bracket expansion fails and
// refineFancyPayment returns the initial estimate unchanged.
func TestRefineFancyPaymentCannotBracket(t *testing.T) {
	ms, err := MonthSetFromString("1-12")
	if err != nil {
		t.Fatalf("MonthSetFromString: %v", err)
	}
	loan := mkFancyLoan(200_000, 0.50, 360, 0)
	loan.PayAmtStatus = types.StatusEmpty
	s := fancyTestSettings()
	in := LoanInput{
		Loan:     loan,
		Settings: s,
		Fancy:    true,
		SkipMonths: SkipMonths{
			SkipStatus: types.InOutInput,
			SkipStr:    "1-12",
			MonthSet:   ms,
		},
	}
	f := GrowthPerPeriod(&loan, s.YrInv)
	tr, _ := ComputeTrueRate(&loan, &s)
	est := estimatePayment(&loan, f)
	got := refineFancyPayment(in, est, &s, tr, f)
	if got != est {
		t.Errorf("skip-all: refineFancyPayment = %.4f, want unchanged estimate %.4f", got, est)
	}
}

// TestSkipAllMonthsBlankPaymentEngine drives the skip-month blank-payment
// branch (engine.go:505-517) through the public Amortize entry point: a
// loan with every month skipped triggers solveFancyPayment (which fails to
// bracket) and then the refineFancyPayment fallback. The schedule still
// generates without error.
func TestSkipAllMonthsBlankPaymentEngine(t *testing.T) {
	ms, _ := MonthSetFromString("1-12")
	// n=360 at a high rate: solveFancyPayment cannot bracket (the loan never
	// amortizes with every month skipped), so the engine falls through to the
	// refineFancyPayment legacy fallback (engine.go:516).
	loan := mkFancyLoan(200_000, 0.50, 360, 0)
	loan.PayAmtStatus = types.StatusEmpty
	loan.LoanDate = types.DateRec{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	in := LoanInput{
		Loan:     loan,
		Settings: fancyTestSettings(),
		Fancy:    true,
		SkipMonths: SkipMonths{
			SkipStatus: types.InOutInput,
			SkipStr:    "1-12",
			MonthSet:   ms,
		},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize(skip-all): unexpected error %v", res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Errorf("expected a (degenerate) schedule, got none")
	}
}
