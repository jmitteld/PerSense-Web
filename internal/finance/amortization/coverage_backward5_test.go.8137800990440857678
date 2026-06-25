package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_backward5_test.go drives the non-tiny (discounted-PV) closed-form
// paths of solvePrepayAmountAdditive and SolvePrepaymentDuration with
// LastOK=false so the internal last-date derivation, balloon and
// other-series subtraction, and the duration solve all run.

func longLoanNoLast(rate float64, pay float64) Loan {
	l := simpleLoan(200000, rate, 360, pay)
	l.LastOK = false           // force the AddNPeriods last-date derivation
	l.LastStatus = types.StatusEmpty
	return l
}

// TestAdditivePrepayNonTinyFullPath covers solvePrepayAmountAdditive's
// non-tiny branch including the !LastOK last-date derivation (backward.go:739),
// balloon subtraction (:753) and other-series subtraction (:766).
func TestAdditivePrepayNonTinyFullPath(t *testing.T) {
	loan := longLoanNoLast(0.06, 1199.10)
	in := LoanInput{
		Loan:     loan,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:    true,
		Balloons: []BalloonPayment{{
			DateStatus:   types.InOutInput,
			Date:         types.NewDateRec(2030, time.January, 1),
			AmountStatus: types.InOutInput,
			Amount:       10000,
		}},
		Prepayments: []Prepayment{
			{ // index 0: unknown amount
				StartDateStatus: types.InOutInput,
				StartDate:       types.NewDateRec(2025, time.January, 1),
				NNStatus:        types.InOutInput,
				NN:              24,
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
			},
			{ // index 1: known second series
				StartDateStatus: types.InOutInput,
				StartDate:       types.NewDateRec(2026, time.January, 1),
				NNStatus:        types.InOutInput,
				NN:              12,
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
				PaymentStatus:   types.InOutInput,
				Payment:         200,
			},
		},
	}
	got, err := solvePrepayAmountAdditive(in, 0)
	if err != nil {
		t.Fatalf("solvePrepayAmountAdditive non-tiny: %v", err)
	}
	if got == 0 {
		t.Errorf("expected a non-zero solved additive prepayment amount")
	}
}

// TestSolvePrepaymentDurationFullPath covers SolvePrepaymentDuration's
// non-tiny success path with LastOK=false: the last-date derivation
// (backward.go:824), balloon and other-series subtraction, and the
// lastFactor/AddYears/NumberOfInstallments solve. Driven through the engine
// dispatch with a regular payment below the level annuity so the extra
// series must run for a solvable, positive number of periods.
func TestSolvePrepaymentDurationFullPath(t *testing.T) {
	loan := simpleLoan(100000, 0.12, 360, 1010)
	loan.LastOK = false
	loan.LastStatus = types.StatusEmpty
	in := LoanInput{
		Loan:     loan,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024, time.February, 1),
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         600,
		}},
	}
	nn, _, err := SolvePrepaymentDuration(in, 0)
	if err != nil {
		t.Fatalf("SolvePrepaymentDuration full path: %v", err)
	}
	if nn <= 0 {
		t.Errorf("expected a positive prepayment duration, got %d", nn)
	}
}
