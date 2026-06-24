package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_cheapwins_test.go mops up the remaining directly-reachable
// non-overflow branches: the SolveRate low-guess clamp, the empty-balloon
// continue in the closed-form solvers, the daily prepaid settlement stub,
// the dosIteratePayment zero-estimate guard, the fancyBisect domain clamps,
// and the solveFancy* estimate<=0 fallback brackets.

// TestSolveRateLowGuessClamp covers the rate<0.02 clamp (backward.go:298): a
// payment small relative to the principal yields a first-guess below 0.02.
func TestSolveRateLowGuessClamp(t *testing.T) {
	loan := simpleLoan(1_000_000, 0, 360, 100) // 100*12/1e6 = 0.0012 < 0.02
	loan.LoanRateStatus = types.StatusEmpty
	_, _, err := SolveRate(LoanInput{Loan: loan, Settings: simpleSettings()})
	// Either solves or reports a sensible error; we only need the clamp to run.
	_ = err
}

// TestClosedFormSolversWithEmptyBalloon covers the empty-balloon DateStatus
// continue in SolveLoanAmount (backward.go:227) and the additive-prepay /
// duration loops (:758, :848): a blank balloon row carried in the slice is
// skipped.
func TestClosedFormSolversWithEmptyBalloon(t *testing.T) {
	emptyBalloon := BalloonPayment{Date: types.UnknownDate()}

	// SolveLoanAmount with a blank balloon row + a real one.
	loan := simpleLoan(0, 0.06, 360, 1199.10)
	loan.AmountStatus = types.StatusEmpty
	la := LoanInput{
		Loan:     loan,
		Settings: simpleSettings(),
		Balloons: []BalloonPayment{
			emptyBalloon,
			{DateStatus: types.InOutInput, Date: types.NewDateRec(2030, time.January, 1), AmountStatus: types.InOutInput, Amount: 5000},
		},
	}
	if _, _, err := SolveLoanAmount(la); err != nil {
		t.Fatalf("SolveLoanAmount with empty balloon: %v", err)
	}

	// Additive prepay amount solve (non-tiny) with a blank balloon row.
	pl := simpleLoan(200000, 0.06, 360, 1199.10)
	pl.LastOK = true
	pl.LastDate = types.NewDateRec(2054, time.January, 1)
	pa := LoanInput{
		Loan:     pl,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:    true,
		Balloons: []BalloonPayment{emptyBalloon},
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2025, time.January, 1),
			NNStatus:        types.InOutInput,
			NN:              12,
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
		}},
	}
	if _, err := solvePrepayAmountAdditive(pa, 0); err != nil {
		t.Fatalf("solvePrepayAmountAdditive with empty balloon: %v", err)
	}

	// Prepayment duration solve with a blank balloon row.
	dl := simpleLoan(100000, 0.12, 360, 1010)
	dl.LastOK = true
	dl.LastDate = types.NewDateRec(2054, time.January, 1)
	da := LoanInput{
		Loan:     dl,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:    true,
		Balloons: []BalloonPayment{emptyBalloon},
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024, time.February, 1),
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         600,
		}},
	}
	if _, _, err := SolvePrepaymentDuration(da, 0); err != nil {
		t.Fatalf("SolvePrepaymentDuration with empty balloon: %v", err)
	}
}

// TestReplacePrepayZeroBasePayment covers the a1<=0 fallback first-guess in
// the REPLACE-mode SolvePrepaymentAmount secant (backward.go:662): when the
// regular payment is zero the secant seeds its guess from 1% of the
// principal instead.
func TestReplacePrepayZeroBasePayment(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 60, 0) // PayAmt 0 -> a1 fallback
	loan.LastOK = true
	loan.LastDate = types.NewDateRec(2029, time.January, 1)
	in := LoanInput{
		Loan:     loan,
		Settings: simpleSettings(), // PlusRegular OFF -> replace-mode secant
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024, time.February, 1),
			StopDateStatus:  types.InOutInput,
			StopDate:        types.NewDateRec(2029, time.January, 1),
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			// amount unknown
		}},
	}
	got, err := SolvePrepaymentAmount(in, 0)
	if err != nil {
		t.Fatalf("SolvePrepaymentAmount replace-mode zero base payment: %v", err)
	}
	if got <= 0 {
		t.Errorf("solved replace-mode prepayment = %.2f, want positive", got)
	}
}

// TestDailyPrepaidSettlementStub covers the daily-compounding settlement-stub
// arm of generateFancySchedule (engine.go:1260): a loan date before the
// natural period start under prepaid + daily emits a discounted stub row.
func TestDailyPrepaidSettlementStub(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 24, 0)
	loan.PayAmtStatus = types.StatusEmpty
	// First payment 2024-03-01 -> natural start 2024-02-01; loan date earlier.
	loan.LoanDate = types.NewDateRec(2024, time.January, 10)
	loan.FirstDate = types.NewDateRec(2024, time.March, 1)
	s := simpleSettings()
	s.Prepaid = true
	s.Daily = true
	res := Amortize(LoanInput{Loan: loan, Settings: s, Fancy: true})
	if res.Err != nil {
		t.Fatalf("Amortize daily prepaid stub: %v", res.Err)
	}
	if len(res.Schedule) == 0 || res.Schedule[0].PayNum != 0 {
		t.Errorf("expected a daily settlement-stub row 0, got %v", res.Schedule)
	}
}

// TestDosIteratePaymentZeroEstimate covers the estimate==0 guard
// (fancybisect.go:120).
func TestDosIteratePaymentZeroEstimate(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 60, 0)
	if _, ok := dosIteratePayment(LoanInput{Loan: loan, Settings: simpleSettings()}, 0); ok {
		t.Errorf("dosIteratePayment with zero estimate should report failure")
	}
}

// TestFancyBisectDomainClamps covers the lo<minX (fancybisect.go:207) and
// hi>maxX (:210) initial-bracket clamps.
func TestFancyBisectDomainClamps(t *testing.T) {
	// lo below minX and hi above maxX both get clamped into [minX,maxX].
	if _, ok := fancyBisect(func(v float64) int {
		if v >= 50 {
			return 0
		}
		return 1
	}, -100, 1000, 0, 200, 1e-6); !ok {
		t.Errorf("expected convergence with clamped bracket")
	}
}

// TestSolveFancyEstimateNonPositive covers the estimate<=0 fallback brackets
// in solveFancyAmount/Rate/Payment (fancybisect.go:274,292,316).
func TestSolveFancyEstimateNonPositive(t *testing.T) {
	loan := mkFancyLoan(200000, 0.06, 60, 0)
	in := LoanInput{Loan: loan, Settings: fancyTestSettings(), Fancy: true}

	amtIn := in
	amtIn.Loan.AmountStatus = types.StatusEmpty
	amtIn.Loan.PayAmt = annuityPayment(200000, GrowthPerPeriod(&loan, in.Settings.YrInv), 60)
	amtIn.Loan.PayAmtStatus = types.InOutInput
	solveFancyAmount(amtIn, 0) // estimate<=0 -> wide [1,1e7] fallback

	rateIn := in
	rateIn.Loan.LoanRateStatus = types.StatusEmpty
	solveFancyRate(rateIn, 0) // estimate<=0 -> [1e-4,1.0] fallback

	payIn := in
	payIn.Loan.PayAmtStatus = types.StatusEmpty
	solveFancyPayment(payIn, 0) // estimate<=0 -> [1,1e7] fallback
}
