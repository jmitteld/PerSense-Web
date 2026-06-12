package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// This file locks in the DOS-fidelity fixes found during the 2026-06 oracle
// differential session as plain unit tests — no FPC source-oracle and no Python
// required, so they run in ordinary CI even when the oracle isn't built. Each
// test documents the bug it guards and the DOS-faithful behaviour it pins.
// See: docs/moratorium_finding.md, docs/arm_adjustment_findings.md,
// docs/basis_weekly_finding.md, docs/prepayment_semantics_finding.md.

// TestRegressionMoratoriumKeepsGivenPayment guards the moratorium fix: when the
// regular payment is GIVEN (not blank), an interest-only moratorium must leave
// that payment untouched. The pre-fix engine re-amortised the given payment over
// the post-moratorium term, producing a different (higher) payment. DOS keeps
// the user's payment as-is (AMORTOP.pas) and only recomputes when it was blank.
func TestRegressionMoratoriumKeepsGivenPayment(t *testing.T) {
	in := baseInput30y() // 200000 @ 6%, payment GIVEN = 1199.10, monthly
	// Interest-only until Jan 2025 (the first 11 payments are deferred).
	in.Moratorium = Moratorium{
		FirstRepayStatus: types.InOutInput,
		FirstRepay:       types.NewDateRec(2025, time.January, 1),
	}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatal(r.Err)
	}

	// During the moratorium the balance must not move (interest-only rows keep
	// Principal == 200000), and the given payment must reappear unchanged.
	sawInterestOnly := false
	sawGivenPayment := false
	for _, row := range r.Schedule {
		if math.Abs(row.Principal-200000) < 0.005 {
			sawInterestOnly = true
		}
		if math.Abs(row.PayAmt-1199.10) < 0.005 {
			sawGivenPayment = true
		}
	}
	if !sawInterestOnly {
		t.Error("expected interest-only rows with balance held at 200000 during moratorium")
	}
	if !sawGivenPayment {
		t.Error("given payment 1199.10 must be preserved after moratorium (regression: it was re-amortised)")
	}
}

// TestRegressionSolveAdjRateAllowsNegative guards the payment-only ARM fix
// (AO6 / EstimateAndRefineAdjRate). When an adjustment supplies a new payment
// but no rate, solveAdjRate backs out the rate at which that payment retires the
// balance over the remaining term. If the payment's nominal total falls short of
// the balance, the implied rate is NEGATIVE. The pre-fix code clamped the trial
// rate to >= 0, which stalled the secant and wrongly kept the old positive rate;
// DOS (AMORTOP.pas:1485) allows a negative implied rate bounded by |rate| < 2.
func TestRegressionSolveAdjRateAllowsNegative(t *testing.T) {
	loan := Loan{LoanRate: 0.06, PerYr: 12}
	// 7000/period × 12 = 84000 < 100000 balance, so only a negative interest
	// rate retires it exactly over the 12 periods.
	r, ok := solveAdjRate(100000, 7000, 12, loan, 1.0/360)
	if !ok {
		t.Fatal("solveAdjRate failed to converge on a negative implied rate")
	}
	if r >= 0 {
		t.Errorf("implied rate = %.4f, want < 0 (payment total below balance)", r)
	}
	// Sanity: at that rate the balance is actually retired after 12 periods.
	l := loan
	l.LoanRate = r
	if resid := balanceAfterN(100000, 7000, GrowthPerPeriod(&l, 1.0/360), 12); math.Abs(resid) > 0.01 {
		t.Errorf("balance after 12 periods at solved rate = %.4f, want ≈ 0", resid)
	}
}

// TestRegressionBiweeklyActualDayAccrual guards the weekly/biweekly accrual fix.
// For perYr 26/52 the engine must accrue interest on the ACTUAL number of days
// in the period (simple interest, balance·rate·daysInPeriod/yearDays), matching
// DOS, instead of a constant per-period factor. The first biweekly period of a
// loan dated mid-January 2024 spans 14 days; on a 365.25-day year the first
// interest is balance·rate·14/365.25.
func TestRegressionBiweeklyActualDayAccrual(t *testing.T) {
	const (
		amount = 50000.0
		rate   = 0.06
	)
	in := LoanInput{
		Loan: Loan{
			AmountStatus:   types.InOutInput,
			Amount:         amount,
			LoanRateStatus: types.InOutInput,
			LoanRate:       rate,
			NStatus:        types.InOutInput,
			NPeriods:       26,
			PerYrStatus:    types.InOutInput,
			PerYr:          26, // biweekly
			LoanDateStatus: types.InOutInput,
			LoanDate:       types.NewDateRec(2024, time.January, 15),
			FirstStatus:    types.InOutInput,
			FirstDate:      types.NewDateRec(2024, time.January, 29), // +14 days
			// payment blank → solved
		},
		Settings: Settings{
			Basis:  types.Basis365,
			PerYr:  26,
			YrDays: 365.25,
			YrInv:  1.0 / 365.25,
		},
		Fancy: false,
	}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if len(r.Schedule) == 0 {
		t.Fatal("no schedule rows")
	}
	// The first period (Jan 15 → Jan 29 2024) spans 14 days, all within leap
	// year 2024, so the day-count basis is 366: balance·rate·14/366.
	wantFirstInt := amount * rate * 14.0 / 366.0 // ≈ 114.75
	got := r.Schedule[0].Interest
	if math.Abs(got-wantFirstInt) > 0.02 {
		t.Errorf("biweekly first-period interest = %.4f, want ≈ %.4f (actual-day accrual)", got, wantFirstInt)
	}
	// Regression marker: the pre-fix constant-factor accrual would have used a
	// fixed 365.25-day period factor (≈114.99), differing by >0.2.
	if math.Abs(got-amount*rate*14.0/365.25) < 0.02 {
		t.Errorf("interest %.4f matches the old constant-factor accrual; expected actual-day basis", got)
	}
}

// TestRegressionPrepaymentDurationClosedForm guards Gap B: the prepayment
// DURATION solve (solve for how long an additive prepayment must run) uses the
// DOS closed-form PV formula (AMORTIZE.pas:730-768), not a simulate-to-payoff
// loop. The old loop over-counted badly; the canonical case below — 100000 @ 8%,
// regular payment 600 (below amortising), +500/period additive monthly — solves
// to 42 prepayments under the DOS formula. goSolvePrepayDuration (defined in
// dos_oracle_sweep_test.go) drives the engine directly with no oracle.
func TestRegressionPrepaymentDurationClosedForm(t *testing.T) {
	nn, ok := goSolvePrepayDuration(100000, 0.08, 600, 360, 12, 1, 12, 500)
	if !ok {
		t.Fatal("prepayment duration solve failed")
	}
	if nn != 42 {
		t.Errorf("Gap-B duration solve NN = %d, want 42 (DOS closed form)", nn)
	}
}

// TestRegressionPrepaymentAmountAdditiveGapA guards Gap A: the prepayment-AMOUNT
// solve under the "balloon includes regular payment" (PlusRegular/additive)
// setting. DOS solves the additive extra with a discounted-PV closed form
// (AMORTIZE.pas:670-699) that nets out the regular payment, whereas the default
// (replace) setting solves the full period payment. The defining relationship —
// which the pre-fix additive path got wrong — is that the additive extra equals
// the replace amount minus the regular payment, since the additive extra sits on
// top of the regular payment. Canonical: 100000 @ 8%, regular 600, 24 monthly
// prepayments from month 1.
func TestRegressionPrepaymentAmountAdditiveGapA(t *testing.T) {
	const regPay = 600.0
	add, ok := goSolvePrepayAmount(100000, 0.08, regPay, 360, 12, 1, 24, 12, true) // PlusRegular
	if !ok {
		t.Fatal("additive amount solve failed")
	}
	rep, ok2 := goSolvePrepayAmount(100000, 0.08, regPay, 360, 12, 1, 24, 12, false) // replace
	if !ok2 {
		t.Fatal("replace amount solve failed")
	}
	if math.Abs(add-(rep-regPay)) > 1e-6 {
		t.Errorf("additive=%.6f replace=%.6f: additive should equal replace-regPay=%.6f", add, rep, rep-regPay)
	}
	if math.Abs(add-824.489160) > 1e-4 {
		t.Errorf("additive prepayment amount = %.6f, want 824.489160", add)
	}
}
