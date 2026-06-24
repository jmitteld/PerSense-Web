package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_engine2_test.go targets forward-engine branches that the
// existing suite does not reach: the in-advance RepayLoan loop, daily
// compounding, US-Rule, the exact-in-advance early-payoff and guard arms,
// and off-cycle balloon folding.

func simpleSettings() Settings {
	return Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360}
}

func simpleLoan(amount, rate float64, n int, pay float64) Loan {
	return Loan{
		AmountStatus:   types.InOutInput,
		Amount:         amount,
		LoanRateStatus: types.InOutInput,
		LoanRate:       rate,
		PayAmtStatus:   types.InOutInput,
		PayAmt:         pay,
		NStatus:        types.InOutInput,
		NPeriods:       n,
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		LoanDateStatus: types.InOutInput,
		LoanDate:       types.NewDateRec(2024, time.January, 1),
		FirstStatus:    types.InOutInput,
		FirstDate:      types.NewDateRec(2024, time.February, 1),
	}
}

// TestRepayLoanInAdvanceBranch covers the in-advance accumulation loop in
// RepayLoan (engine.go:108-112) by solving a non-fancy in-advance payment.
func TestRepayLoanInAdvanceBranch(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 60, 0)
	loan.PayAmtStatus = types.StatusEmpty
	s := simpleSettings()
	s.InAdvance = true
	// Direct RepayLoan call with an in-advance setting exercises the
	// annuity-due loop.
	resid := RepayLoan(100000, 1900, &loan, &s, s.YrInv)
	if resid == 0 {
		t.Errorf("RepayLoan in-advance returned exactly 0 residual; expected a real number")
	}
	// And through SolvePaymentClosedForm so the in-advance closed-form divide also runs.
	got, err := SolvePaymentClosedForm(LoanInput{Loan: loan, Settings: s})
	if err != nil {
		t.Fatalf("SolvePaymentClosedForm in-advance: %v", err)
	}
	if got <= 0 {
		t.Errorf("SolvePaymentClosedForm in-advance = %.4f, want positive", got)
	}
}

// TestDailyCompoundingSchedule drives the Daily-compounding accrual arms of
// generateFancySchedule (the settlement-stub and per-period exp branches).
func TestDailyCompoundingSchedule(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 24, 0)
	loan.PayAmtStatus = types.StatusEmpty
	s := simpleSettings()
	s.Prepaid = true
	s.Daily = true
	// Loan date well before the first payment so the prepaid settlement stub
	// (Case A, daily branch) is emitted.
	loan.LoanDate = types.NewDateRec(2024, time.January, 10)
	in := LoanInput{Loan: loan, Settings: s, Fancy: true}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize daily: %v", res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Fatalf("daily schedule empty")
	}
}

// TestDailyOffCyclePrepaymentAndUSARule covers the daily-compounding and
// US-Rule off-cycle prepayment-drain arms (engine.go:1417-1436): an
// off-cycle prepayment series under daily compounding + US Rule emits dated
// drain rows.
func TestDailyOffCyclePrepaymentAndUSARule(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 36, 1500)
	s := simpleSettings()
	s.Daily = true
	s.USARule = true
	s.Prepaid = true
	in := LoanInput{
		Loan:     loan,
		Settings: s,
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024, time.February, 15), // mid-month: off-cycle
			NNStatus:        types.InOutInput,
			NN:              6,
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         300,
		}},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize daily+USARule off-cycle: %v", res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Fatalf("schedule empty")
	}
}

// TestDailyOffCycleMultiSeries covers the off-cycle prepayment-drain inner
// loop's per-series guards (engine.go:1400-1411): two off-cycle series, one
// bounded by a stop date and one by an NN count, so the StopDate-passed and
// NN-exhausted continues both fire while the other series still drains.
func TestDailyOffCycleMultiSeries(t *testing.T) {
	loan := simpleLoan(120000, 0.06, 48, 1800)
	s := simpleSettings()
	s.Daily = true
	s.Prepaid = true
	in := LoanInput{
		Loan:     loan,
		Settings: s,
		Fancy:    true,
		Prepayments: []Prepayment{
			{ // off-cycle, NN-bounded, exhausts early
				StartDateStatus: types.InOutInput,
				StartDate:       types.NewDateRec(2024, time.February, 12),
				NNStatus:        types.InOutInput,
				NN:              3,
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
				PaymentStatus:   types.InOutInput,
				Payment:         200,
			},
			{ // off-cycle, stop-date-bounded, runs longer
				StartDateStatus: types.InOutInput,
				StartDate:       types.NewDateRec(2024, time.February, 20),
				StopDateStatus:  types.InOutInput,
				StopDate:        types.NewDateRec(2025, time.February, 20),
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
				PaymentStatus:   types.InOutInput,
				Payment:         150,
			},
		},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize daily off-cycle multi-series: %v", res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Fatalf("schedule empty")
	}
}

// TestExactInAdvanceEarlyPayoff covers generateExactInAdvanceSchedule's
// early-payoff break (engine.go:1164): an over-amortizing payment retires
// the loan before the final period.
func TestExactInAdvanceEarlyPayoff(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 60, 0)
	s := Settings{Basis: types.Basis365, PerYr: 12, YrDays: 365, YrInv: 1.0 / 365, Exact: true, InAdvance: true}
	// A deliberately large payment over-amortizes, tripping the early break.
	res := generateExactInAdvanceSchedule(LoanInput{Loan: loan, Settings: s}, 20000, &s)
	if res.Err != nil {
		t.Fatalf("generateExactInAdvanceSchedule: %v", res.Err)
	}
	if len(res.Schedule) >= loan.NPeriods {
		t.Errorf("expected early payoff (fewer than %d rows), got %d", loan.NPeriods, len(res.Schedule))
	}
}

// TestExactInAdvanceMaxGuard covers the oversized-term guard
// (engine.go:1100): an absurd period count returns an error.
func TestExactInAdvanceMaxGuard(t *testing.T) {
	loan := simpleLoan(100000, 0.06, MaxSchedulePeriods+1, 1000)
	s := Settings{Basis: types.Basis365, PerYr: 12, YrDays: 365, YrInv: 1.0 / 365, Exact: true, InAdvance: true}
	res := generateExactInAdvanceSchedule(LoanInput{Loan: loan, Settings: s}, 1000, &s)
	if res.Err == nil {
		t.Errorf("expected oversized-term error, got none")
	}
}

// TestExactInAdvanceForwardSchedule drives the full exact-in-advance path
// through Amortize (settlement row + n-1 amortizing rows).
func TestExactInAdvanceForwardSchedule(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 24, 0)
	loan.PayAmtStatus = types.StatusEmpty
	s := Settings{Basis: types.Basis365, PerYr: 12, YrDays: 365, YrInv: 1.0 / 365, Exact: true, InAdvance: true}
	res := Amortize(LoanInput{Loan: loan, Settings: s})
	if res.Err != nil {
		t.Fatalf("Amortize exact in-advance: %v", res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Fatalf("exact in-advance schedule empty")
	}
	if res.Schedule[0].PayNum != 0 {
		t.Errorf("expected a row-0 settlement row, got PayNum %d", res.Schedule[0].PayNum)
	}
}
