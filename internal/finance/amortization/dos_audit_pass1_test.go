// Regressions for the 2026-07-11 audit PASS-1 findings (docs/discrepancies.md
// §20a-20g). Every golden cites the real-DOS-engine command that produced it.
package amortization

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func pass1Loan(amount, rate, pay float64, n int) Loan {
	l := Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		PayAmtStatus: types.InOutInput, PayAmt: pay,
		NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}
	if pay <= 0 {
		l.PayAmtStatus = types.StatusEmpty
	}
	return l
}

// 20a: in-advance APR includes DOS's forced-prepaid settlement interest.
//
//	amort_oracle 100000 0.10 120 12 payhard=1500 inadv pts=0.03 apr           → apr 0.141184
//	amort_oracle 100000 0.10 120 12 payhard=1500 inadv b60=20000 pts=0.03 apr → apr 0.110437
func Test20aInAdvanceAPRForcedPrepaid(t *testing.T) {
	s := basicSettings()
	s.InAdvance = true
	loan := pass1Loan(100000, 0.10, 1500, 120)
	loan.PointsStatus = types.InOutInput
	loan.Points = 0.03
	res := Amortize(LoanInput{Loan: loan, Settings: s})
	if res.Err != nil || math.Abs(res.APR-0.141184) > 2e-4 {
		t.Errorf("in-advance APR = %.6f err=%v, want 0.141184 (oracle)", res.APR, res.Err)
	}
	// balloon variant: b60 anchors month 60 = 2029-01-01
	res2 := Amortize(LoanInput{Loan: loan, Settings: s, Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 20000}}})
	if res2.Err != nil || math.Abs(res2.APR-0.110437) > 2e-4 {
		t.Errorf("in-advance balloon APR = %.6f err=%v, want 0.110437 (oracle)", res2.APR, res2.Err)
	}
}

// 20b: REPLACE-mode balloon + under-funded hard payment folds the residual into
// the final scheduled payment.
//
//	amort_oracle 100000 0.08 240 12 payhard=600 usa b60=20000 → interest 135543.41 paid 235543.41 (final row 72743.41 → bal 0)
//	amort_oracle 100000 0.08 240 12 payhard=600 b60=20000     → interest 138513.72 paid 238513.72
func Test20bUnderfundedBalloonFold(t *testing.T) {
	for _, c := range []struct {
		usa            bool
		wantInt, wPaid float64
	}{{true, 135543.41, 235543.41}, {false, 138513.72, 238513.72}} {
		s := basicSettings()
		s.USARule = c.usa
		res := Amortize(LoanInput{Loan: pass1Loan(100000, 0.08, 600, 240), Settings: s, Fancy: true,
			Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1),
				AmountStatus: types.InOutInput, Amount: 20000}}})
		if res.Err != nil {
			t.Fatalf("usa=%v err: %v", c.usa, res.Err)
		}
		if math.Abs(res.TotalInt-c.wantInt) > 0.05 || math.Abs(res.TotalPaid-c.wPaid) > 0.05 {
			t.Errorf("usa=%v: int=%.2f paid=%.2f, want %.2f / %.2f (oracle)", c.usa, res.TotalInt, res.TotalPaid, c.wantInt, c.wPaid)
		}
		if math.Abs(res.FinalPrinc) > 0.01 {
			t.Errorf("usa=%v: final balance = %.4f, want 0 (DOS very-last fold)", c.usa, res.FinalPrinc)
		}
	}
}

// 20c: payoff on a plain USA-rule neg-am loan uses DOS's usap-aware payoff walk
// (which intentionally disagrees with DOS's own displayed balance column).
//
//	amort_oracle 100000 0.08 360 12 payhard=600 usa payoff=1.7.2026  → 102612.9862
//	amort_oracle 100000 0.08 360 12 payhard=600 usa payoff=15.1.2029 → 104323.7562
//	amort_oracle 100000 0.08 360 12 payhard=600 usa payoff=1.1.2054  → 124760.7602
func Test20cUSAPlainPayoffUsapWalk(t *testing.T) {
	s := basicSettings()
	s.USARule = true
	in := LoanInput{Loan: pass1Loan(100000, 0.08, 600, 360), Settings: s}
	for _, c := range []struct {
		y, m, d int
		want    float64
	}{{2026, 7, 1, 102612.9862}, {2029, 1, 15, 104323.7562}, {2054, 1, 1, 124760.7602}} {
		got, err := PayoffBalance(in, types.NewDateRec(c.y, time.Month(c.m), c.d))
		if err != nil || math.Abs(got-c.want) > 0.01 {
			t.Errorf("payoff %04d-%02d-%02d = %.4f err=%v, want %.4f (oracle)", c.y, c.m, c.d, got, err, c.want)
		}
	}
}

// 20d: payoff RateInForce is DOS's FORWARD scan — the stub accrues at the
// next/only adjustment's rate (base rate unreachable once any ARM exists), and
// an AO6 payment-only row carries the implied rate DOS solved.
//
//	adj=48:0.09: (= 2028-01-01) payoff=1.2.2024 → 100750.0000 (9% stub, not 8%)
//	                            payoff=1.7.2026 → 98595.1227
//	adj=48:0.09: adj=96:0.11: (2032-01-01) payoff=1.2.2029 → 96120.6613 (the SECOND ARM's 11%)
//	adj=48::700 payoff=1.7.2026 → 98470.4382; payoff=1.7.2029 → 95002.5186 (implied rate)
func Test20dRateInForceForwardScan(t *testing.T) {
	oneARM := []RateAdjustment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2028, time.January, 1),
		LoanRateStatus: types.InOutInput, LoanRate: 0.09}}
	twoARM := append(append([]RateAdjustment{}, oneARM...),
		RateAdjustment{DateStatus: types.InOutInput, Date: types.NewDateRec(2032, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.11})
	ao6 := []RateAdjustment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2028, time.January, 1),
		AmountStatus: types.InOutInput, Amount: 700, AmtOK: true}}
	mk := func(adjs []RateAdjustment) LoanInput {
		return LoanInput{Loan: pass1Loan(100000, 0.08, 0, 360), Settings: basicSettings(), Fancy: true, Adjustments: adjs}
	}
	cases := []struct {
		in        LoanInput
		y, m, d   int
		want, tol float64
	}{
		{mk(oneARM), 2024, 2, 1, 100750.0000, 0.01},
		{mk(oneARM), 2026, 7, 1, 98595.1227, 0.01},
		{mk(twoARM), 2029, 2, 1, 96120.6613, 0.01},
		// AO6 implied-rate recompute: within the two engines' Newton tolerances.
		{mk(ao6), 2026, 7, 1, 98470.4382, 0.25},
		{mk(ao6), 2029, 7, 1, 95002.5186, 0.25},
	}
	for _, c := range cases {
		got, err := PayoffBalance(c.in, types.NewDateRec(c.y, time.Month(c.m), c.d))
		if err != nil || math.Abs(got-c.want) > c.tol {
			t.Errorf("payoff %04d-%02d-%02d = %.4f err=%v, want %.4f ±%.2f (oracle)", c.y, c.m, c.d, got, err, c.want, c.tol)
		}
	}
}

// 20e: in-advance payoff before the first payment rebates the settlement
// interest (DOS's forced-prepaid rule).
//
//	amort_oracle 100000 0.10 120 12 inadv payoff=15.1.2024 → 99555.5556
func Test20eInAdvanceEarlyPayoffRebate(t *testing.T) {
	s := basicSettings()
	s.InAdvance = true
	got, err := PayoffBalance(LoanInput{Loan: pass1Loan(100000, 0.10, 0, 120), Settings: s},
		types.NewDateRec(2024, time.January, 15))
	if err != nil || math.Abs(got-99555.5556) > 0.01 {
		t.Errorf("in-advance early payoff = %.4f err=%v, want 99555.5556 (oracle)", got, err)
	}
}

// 20f: a prepayment series starting ON the loan date is refused ("dates out of
// order", DOS Amortize.pas:1231-1237, equality included).
//
//	amort_oracle 100000 0.08 120 12 pre=0:12:12:500 → ERR Your dates are out of order.
func Test20fPrepayOnLoanDateRefused(t *testing.T) {
	res := Amortize(LoanInput{Loan: pass1Loan(100000, 0.08, 0, 120), Settings: basicSettings(), Fancy: true,
		Prepayments: []Prepayment{{StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.January, 1),
			PaymentStatus: types.InOutInput, Payment: 500,
			PerYrStatus: types.InOutInput, PerYr: 12,
			NNStatus: types.InOutInput, NN: 12}}})
	if res.Err == nil || !strings.Contains(res.Err.Error(), "on or before the Loan Date") {
		t.Errorf("prepay starting on the loan date should refuse (DOS), got %v", res.Err)
	}
}

// 20g: deliberate non-reproduction — DOS's moratorium payment solve fails at
// exactly (rate=0.10, n=120, mor=12): a measure-zero FP coincidence (rate ±1e-6,
// n ±1, and mor ±1 all solve in DOS). Go solves and retires; pin that.
func Test20gMorSolveIsolatedDOSFailure(t *testing.T) {
	res := Amortize(LoanInput{Loan: pass1Loan(100000, 0.10, 0, 120), Settings: basicSettings(), Fancy: true,
		Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2025, time.January, 1)}})
	if res.Err != nil || math.Abs(res.FinalPrinc) > 0.01 {
		t.Errorf("Go must solve the mor=12 rate=0.10 loan DOS's Newton chokes on: err=%v final=%.4f", res.Err, res.FinalPrinc)
	}
}
