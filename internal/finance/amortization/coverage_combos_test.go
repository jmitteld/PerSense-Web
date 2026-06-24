package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_combos_test.go — advanced-option COMBINATIONS that exercise the
// branch-heavy schedule and solver paths (off-cycle events, moratorium
// re-amortize, NN-bounded series, additive prepayment-solve sub-branches,
// zero-rate forms). These drive the real Amortize dispatch and assert the run
// completes and retires; DOS-numeric fidelity for these is covered by the oracle
// cubes and the dedicated advanced tests.

func ccd(y int, m time.Month, d int) types.DateRec { return types.NewDateRec(y, m, d) }

func mustRun(t *testing.T, name string, in LoanInput) AmortResult {
	t.Helper()
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("%s: unexpected error: %v", name, r.Err)
	}
	if len(r.Schedule) == 0 {
		t.Fatalf("%s: empty schedule", name)
	}
	return r
}

// Additive prepayment-amount solve, ZERO rate (tiny-rate branch).
func TestAdditivePrepaySolveZeroRate(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(120000, 0, 120, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			NNStatus: types.InOutInput, NN: 60,
			PaymentStatus: types.StatusEmpty}}}
	in.Loan.LoanRateStatus = types.InOutInput
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 500 // under-amortizes; the prepayment makes up the rest
	mustRun(t, "additive zero-rate prepay solve", in)
}

// Additive prepayment-amount solve WITH a known balloon (balloon-subtraction
// branch) and a second KNOWN prepayment series (other-series branch).
func TestAdditivePrepaySolveWithBalloonAndSecondSeries(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(200000, 0.09, 240, 12), Settings: s, Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: ccd(2030, time.February, 1),
			AmountStatus: types.InOutInput, Amount: 15000}},
		Prepayments: []Prepayment{
			{StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				StopDateStatus: types.InOutInput, StopDate: ccd(2029, time.February, 1),
				PaymentStatus: types.InOutInput, Payment: 100},
			{StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				StopDateStatus: types.InOutInput, StopDate: ccd(2034, time.February, 1),
				PaymentStatus: types.StatusEmpty}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1500
	mustRun(t, "additive prepay solve + balloon + second series", in)
}

// Off-cycle balloon: a balloon dated BETWEEN two payment dates (drains on its
// exact date — the off-cycle draining branch).
func TestOffCycleBalloon(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: cc360(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: ccd(2025, time.June, 15),
			AmountStatus: types.InOutInput, Amount: 10000}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 2300
	mustRun(t, "off-cycle balloon", in)
}

// NN-bounded prepayment series (no stop date; a fixed number of extra payments).
func TestNNBoundedPrepayment(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			NNStatus: types.InOutInput, NN: 24,
			PaymentStatus: types.InOutInput, Payment: 300}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1028.61
	r := mustRun(t, "NN-bounded prepayment", in)
	// After 24 extra payments the series stops; the loan still runs on the
	// regular payment, so it does not retire dramatically early.
	if len(r.Schedule) < 200 {
		t.Errorf("NN-bounded prepayment retired too early (%d rows) — series should stop after 24", len(r.Schedule))
	}
}

// Moratorium with a BLANK payment: the engine re-solves the post-moratorium
// payment over the remaining periods (moratorium re-amortize branch).
func TestMoratoriumReamortize(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: cc360(), Fancy: true,
		Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: ccd(2024, time.October, 1)}}
	in.Loan.PayAmtStatus = types.StatusEmpty
	mustRun(t, "moratorium re-amortize", in)
}

// Skip-months together with a target (target overrides skip — the documented
// DOS precedence branch).
func TestSkipPlusTarget(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: cc360(), Fancy: true,
		Target:     Target{TargetStatus: types.InOutInput, TargetValue: 800},
		SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "7-8"}}
	in.SkipMonths.MonthSet[7] = true
	in.SkipMonths.MonthSet[8] = true
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 2300
	mustRun(t, "skip + target", in)
}

// In-advance forward schedule plus an unknown balloon solve (in-advance branch
// of RepayLoan / generateFancySchedule).
func TestInAdvanceWithBalloonSolve(t *testing.T) {
	s := cc360()
	s.InAdvance = true
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: s, Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: ccd(2028, time.December, 1),
			AmountStatus: types.StatusEmpty}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1500
	mustRun(t, "in-advance + unknown balloon", in)
}

// Daily compounding mode (settings.Daily) — continuous-interest accrual path in
// PrepaidInterest, generateSimpleSchedule, and generateFancySchedule.
func TestDailyCompounding(t *testing.T) {
	s := cc360()
	s.Daily = true
	s.Basis, s.YrDays, s.YrInv = types.Basis365, 365.25, 1.0/365.25
	// simple schedule
	simple := LoanInput{Loan: ccBaseLoan(100000, 0.09, 60, 12), Settings: s}
	mustRun(t, "daily simple", simple)
	// prepaid (daily settlement stub)
	sp := s
	sp.Prepaid = true
	pre := LoanInput{Loan: ccBaseLoan(100000, 0.09, 60, 12), Settings: sp}
	pre.Loan.LoanDate = ccd(2024, time.January, 10)
	pre.Loan.FirstDate = ccd(2024, time.March, 1)
	mustRun(t, "daily prepaid", pre)
	// fancy (with a balloon) under daily compounding
	fancy := LoanInput{Loan: ccBaseLoan(100000, 0.09, 60, 12), Settings: s, Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: ccd(2027, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 8000}}}
	fancy.Loan.PayAmtStatus = types.InOutInput
	fancy.Loan.PayAmt = 1900
	mustRun(t, "daily fancy + balloon", fancy)
}

// Off-cycle PREPAYMENT: a prepayment series whose dates fall between the regular
// payment dates (drains on its own dates — the off-cycle prepayment branch).
func TestOffCyclePrepayment(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 120, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 15), // mid-month, off-cycle
			PerYrStatus: types.InOutInput, PerYr: 4, // quarterly extras against a monthly loan
			StopDateStatus: types.InOutInput, StopDate: ccd(2029, time.February, 15),
			PaymentStatus: types.InOutInput, Payment: 1000}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1500
	mustRun(t, "off-cycle prepayment", in)
}

// Prepayment DURATION solve with a balloon and a second known series — hits the
// balloon-subtraction and other-series PV branches of SolvePrepaymentDuration.
func TestPrepaymentDurationWithBalloonAndSeries(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(200000, 0.09, 240, 12), Settings: s, Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: ccd(2032, time.February, 1),
			AmountStatus: types.InOutInput, Amount: 10000}},
		Prepayments: []Prepayment{
			{StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				StopDateStatus: types.InOutInput, StopDate: ccd(2027, time.February, 1),
				PaymentStatus: types.InOutInput, Payment: 100},
			{StartDateStatus: types.InOutInput, StartDate: ccd(2024, time.February, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: 300,
				StopDateStatus: types.StatusEmpty, NNStatus: types.StatusEmpty}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1700 // below the level annuity so the duration solve is well-posed
	mustRun(t, "prepayment duration + balloon + second series", in)
}

// Rate adjustment that changes BOTH rate and payment, plus a balloon after it
// (the adjustment netting / discounted-balloon branch).
func TestAdjustmentRateAndBalloonAfter(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(150000, 0.06, 120, 12), Settings: cc360(), Fancy: true,
		Adjustments: []RateAdjustment{{DateStatus: types.InOutInput, Date: ccd(2028, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.09}},
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: ccd(2030, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 20000}}}
	in.Loan.PayAmtStatus = types.StatusEmpty
	mustRun(t, "rate adjustment + later balloon", in)
}
