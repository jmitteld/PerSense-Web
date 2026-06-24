package amortization

import (
	"math"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// SolvePaymentClosedForm: standard 30-year 6% on $250K should give ~$1,498.88.
// (DOS continuous-compounding gives a slightly different number than
// the textbook formula; we accept a 1% tolerance band.)
func TestSolvePaymentStandard(t *testing.T) {
	loan := mkLoan(250000, 0.06, 0, 360)
	loan.PayAmtStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	pmt, err := SolvePaymentClosedForm(input)
	if err != nil {
		t.Fatal(err)
	}
	if pmt < 1490 || pmt > 1510 {
		t.Errorf("payment = %.2f, expected ~1498.88", pmt)
	}
}

// SolvePaymentClosedForm: zero-rate path is amount / N.
func TestSolvePaymentZeroRate(t *testing.T) {
	loan := mkLoan(120000, 1e-12, 0, 360)
	loan.PayAmtStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	pmt, err := SolvePaymentClosedForm(input)
	if err != nil {
		t.Fatal(err)
	}
	want := 120000.0 / 360
	if math.Abs(pmt-want) > 0.01 {
		t.Errorf("payment = %.4f, want %.4f", pmt, want)
	}
}

// SolvePaymentClosedForm: missing required fields should error.
func TestSolvePaymentMissingData(t *testing.T) {
	loan := mkLoan(250000, 0.06, 0, 360)
	// PayAmt is already 0 / empty here; clear rate too.
	loan.PayAmtStatus = types.StatusEmpty
	loan.LoanRateStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	_, err := SolvePaymentClosedForm(input)
	if err == nil ||
		!strings.Contains(err.Error(), "cannot be solved yet") {
		t.Errorf("expected insufficient-data error, got %v", err)
	}
}

// SolvePaymentClosedForm: zero amount should error.
func TestSolvePaymentZeroAmount(t *testing.T) {
	loan := mkLoan(0, 0.06, 0, 360)
	loan.PayAmtStatus = types.StatusEmpty
	loan.AmountStatus = types.InOutInput
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	_, err := SolvePaymentClosedForm(input)
	if err == nil ||
		!strings.Contains(err.Error(), "zero") {
		t.Errorf("expected zero-loan error, got %v", err)
	}
}

// SolvePaymentClosedForm + SolveLoanAmount round-trip: solve for payment given
// amount + rate + term, then solve for amount given the resulting
// payment and rate + term — should recover the original amount.
func TestSolvePaymentRoundTripWithSolveLoanAmount(t *testing.T) {
	const origAmount = 250000.0
	loan := mkLoan(origAmount, 0.06, 0, 360)
	loan.PayAmtStatus = types.StatusEmpty
	input := LoanInput{Loan: loan, Settings: basicSettings()}

	pmt, err := SolvePaymentClosedForm(input)
	if err != nil {
		t.Fatal(err)
	}

	// Now go the other way.
	reverse := mkLoan(0, 0.06, pmt, 360)
	reverse.AmountStatus = types.StatusEmpty
	reverseInput := LoanInput{Loan: reverse, Settings: basicSettings()}

	got, _, err := SolveLoanAmount(reverseInput)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-origAmount) > 1.0 {
		t.Errorf("round-trip amount = %.4f, want %.4f (Δ=%.4f)",
			got, origAmount, got-origAmount)
	}
}

// CanComputePayment guard.
func TestCanComputePayment(t *testing.T) {
	loan := mkLoan(250000, 0.06, 0, 360)
	loan.PayAmtStatus = types.StatusEmpty
	if !CanComputePayment(&loan) {
		t.Error("expected CanComputePayment = true when amount+rate+term+peryr known")
	}
	// If payment already set, can't compute.
	loan.PayAmtStatus = types.InOutInput
	if CanComputePayment(&loan) {
		t.Error("expected false when payment already set")
	}
	// If rate missing, can't compute.
	loan.PayAmtStatus = types.StatusEmpty
	loan.LoanRateStatus = types.StatusEmpty
	if CanComputePayment(&loan) {
		t.Error("expected false when rate missing")
	}
}
