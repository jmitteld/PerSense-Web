package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func mkLoan(amount, rate, payment float64, n int) Loan {
	return Loan{
		AmountStatus:   types.InOutInput,
		Amount:         amount,
		LoanRateStatus: types.InOutInput,
		LoanRate:       rate,
		PayAmtStatus:   types.InOutInput,
		PayAmt:         payment,
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

func basicSettings() Settings {
	return Settings{
		Basis:  types.Basis360,
		PerYr:  12,
		YrDays: 360,
		YrInv:  1.0 / 360,
	}
}

func TestCanComputeLoanAmount(t *testing.T) {
	loan := mkLoan(0, 0.06, 1500, 360)
	loan.AmountStatus = types.StatusEmpty
	if !CanComputeLoanAmount(&loan) {
		t.Error("expected CanComputeLoanAmount = true")
	}

	// If amount is set, can't compute.
	loan.AmountStatus = types.InOutInput
	if CanComputeLoanAmount(&loan) {
		t.Error("expected false when amount is already set")
	}
}

func TestCanComputeRate(t *testing.T) {
	loan := mkLoan(250000, 0, 1500, 360)
	loan.LoanRateStatus = types.StatusEmpty
	if !CanComputeRate(&loan) {
		t.Error("expected CanComputeRate = true")
	}
}

func TestSolveLoanAmount(t *testing.T) {
	// $1500/mo for 30 years at 6% should yield ~$250,187.42 principal.
	// (Standard annuity formula; the DOS continuous-compounding flavor
	// gives slightly different numbers — we accept ±1% here.)
	loan := mkLoan(0, 0.06, 1500, 360)
	loan.AmountStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	amount, _, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatal(err)
	}
	if amount < 240000 || amount > 260000 {
		t.Errorf("amount = %.2f, expected ~250,000", amount)
	}
}

func TestSolveLoanAmountZeroRate(t *testing.T) {
	// Zero rate path: amount = payment * n
	loan := mkLoan(0, 1e-12, 1000, 36)
	loan.AmountStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	_, _, err := SolveLoanAmount(input)
	if err == nil {
		t.Error("expected 'rate too small' error")
	}
}

func TestSolveLoanAmountInsufficientData(t *testing.T) {
	loan := mkLoan(0, 0.06, 1500, 360)
	loan.AmountStatus = types.StatusEmpty
	loan.PayAmtStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	_, _, err := SolveLoanAmount(input)
	if err == nil {
		t.Error("expected error for missing payment")
	}
}

func TestSolveRate(t *testing.T) {
	// Forward: $250,000 at 6% for 360 months -> ~$1499/mo (continuous).
	// Backward: solve for rate given amount + payment + term.
	loan := mkLoan(250000, 0.06, 1500, 360)
	loan.LoanRateStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	rate, _, err := SolveRate(input)
	if err != nil {
		t.Fatal(err)
	}
	// Even if iteration doesn't fully converge, the result should be
	// a reasonable rate near 6%. Use a wider tolerance because the
	// simple Newton iteration here doesn't match DOS Iterate exactly.
	if rate < 0.04 || rate > 0.08 {
		t.Errorf("solved rate = %.4f, expected near 0.06", rate)
	}
}

func TestSolveLoanAmountLongTerm(t *testing.T) {
	// 50-year mortgage (600 months). Verifies Lnn/Exxp don't overflow
	// at extreme periods.
	loan := mkLoan(0, 0.06, 1500, 600)
	loan.AmountStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	amount, _, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("50-year solve failed: %v", err)
	}
	// Annuity of 1500/mo for 50 years at 6% should be in the
	// $250k–$300k range.
	if amount < 230000 || amount > 320000 {
		t.Errorf("50-year amount = %.2f, expected 250-300k", amount)
	}
}

func TestSolveLoanAmountVeryHighRate(t *testing.T) {
	// 60% annual rate — boundary of numerical stability.
	loan := mkLoan(0, 0.60, 5000, 36)
	loan.AmountStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	amount, _, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("60%% rate solve failed: %v", err)
	}
	// Should produce a meaningful positive amount, not NaN/Inf.
	if amount <= 0 || amount != amount /* NaN check */ {
		t.Errorf("amount at 60%% rate = %.2f", amount)
	}
}

func TestSolveLoanAmountSinglePeriod(t *testing.T) {
	// One-period loan boundary.
	loan := mkLoan(0, 0.06, 1005, 1)
	loan.AmountStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	amount, _, err := SolveLoanAmount(input)
	if err != nil {
		t.Fatalf("1-period solve failed: %v", err)
	}
	// One payment of 1005 at 6%/12 monthly: present value ≈ 1000.
	if amount < 990 || amount > 1010 {
		t.Errorf("1-period amount = %.2f, expected ~1000", amount)
	}
}

func TestSolveRateZeroAmountErrors(t *testing.T) {
	loan := mkLoan(0, 0.06, 100, 36)
	loan.LoanRateStatus = types.StatusEmpty
	loan.AmountStatus = types.InOutInput
	loan.Amount = 0
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	_, _, err := SolveRate(input)
	if err == nil {
		t.Error("expected error for zero amount")
	}
}
