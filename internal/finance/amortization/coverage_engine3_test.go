package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_engine3_test.go covers the last reachable engine arms: the
// off-cycle balloon fold, the empty-prepay-row continue guards, the empty
// balloon echo skip, and the fancyBisect sHi==0-during-expansion arm.

// TestOffCycleBalloonFold covers the engine arm that folds a balloon dated a
// few days BEFORE a regular payment into that payment (engine.go:1534): an
// odd-first-period loan shifts regular dates off the balloon's monthly grid.
func TestOffCycleBalloonFold(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 60, 1199.10)
	loan.LastOK = true
	loan.LastDate = types.NewDateRec(2029, time.February, 15)
	// Odd first period: loan 2024-01-10, first payment 2024-02-15.
	loan.LoanDate = types.NewDateRec(2024, time.January, 10)
	loan.FirstDate = types.NewDateRec(2024, time.February, 15)
	in := LoanInput{
		Loan:     loan,
		Settings: simpleSettings(),
		Fancy:    true,
		Balloons: []BalloonPayment{{
			DateStatus:   types.InOutInput,
			Date:         types.NewDateRec(2025, time.March, 10), // ~5 days before the 3/15 regular date
			AmountStatus: types.InOutInput,
			Amount:       5000,
		}},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize off-cycle balloon: %v", res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Fatalf("schedule empty")
	}
}

// TestEmptyPrepayAndBalloonRowsInSchedule covers the per-period continue
// guards for empty prepayment rows (engine.go:1558-1562) and the empty
// balloon echo skip (engine.go:617): an empty option row carried alongside a
// real one is skipped throughout the schedule walk.
func TestEmptyPrepayAndBalloonRowsInSchedule(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 60, 1199.10)
	loan.LastOK = true
	loan.LastDate = types.NewDateRec(2029, time.January, 1)
	in := LoanInput{
		Loan:     loan,
		Settings: simpleSettings(),
		Fancy:    true,
		Prepayments: []Prepayment{
			{ // empty row -> StartDateStatus/PaymentStatus StatusEmpty -> continue
				StartDate: types.UnknownDate(),
				StopDate:  types.UnknownDate(),
				NextDate:  types.UnknownDate(),
			},
			{ // payment + peryr set but StartDate blank -> passes the PaymentStatus
				// guard and hits the StartDateStatus continue (engine.go:1561).
				StartDate:     types.UnknownDate(),
				StopDate:      types.UnknownDate(),
				NextDate:      types.UnknownDate(),
				PerYrStatus:   types.InOutInput,
				PerYr:         12,
				PaymentStatus: types.InOutInput,
				Payment:       100,
			},
			{ // real series
				StartDateStatus: types.InOutInput,
				StartDate:       types.NewDateRec(2025, time.January, 1),
				NNStatus:        types.InOutInput,
				NN:              6,
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
				PaymentStatus:   types.InOutInput,
				Payment:         300,
			},
		},
		Balloons: []BalloonPayment{
			{Date: types.UnknownDate()}, // empty -> echo loop skips it (engine.go:617)
			{
				DateStatus:   types.InOutInput,
				Date:         types.NewDateRec(2027, time.January, 1),
				AmountStatus: types.InOutInput,
				Amount:       2000,
			},
		},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize with empty option rows: %v", res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Fatalf("schedule empty")
	}
}

// TestFancyBisectSHiZeroDuringExpansion covers the sHi==0-after-expansion
// short-circuit (fancybisect.go:240): the initial bracket is all positive and
// a zero appears at the HIGH side only after the bracket expands outward.
func TestFancyBisectSHiZeroDuringExpansion(t *testing.T) {
	// Positive on [40,60]; zero appears at >=80 after one expansion (span 20).
	_, ok := fancyBisect(func(v float64) int {
		if v >= 80 {
			return 0
		}
		return 1
	}, 40, 60, 0, 200, 1e-6)
	if !ok {
		t.Errorf("expected convergence when sHi hits zero during expansion")
	}
}

// TestSolvePrepaymentDurationLastFactorGuard covers the lastFactor<=0 guard
// (backward.go:891): a large extra payment drives the discounted balance
// non-positive.
func TestSolvePrepaymentDurationLastFactorGuard(t *testing.T) {
	loan := simpleLoan(100000, 0.06, 360, 550)
	loan.LastOK = true
	loan.LastDate = types.NewDateRec(2054, time.January, 1)
	in := LoanInput{
		Loan:     loan,
		Settings: simpleSettings(),
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2025, time.January, 1),
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         50000, // huge extra -> discounted balance non-positive
		}},
	}
	// Either the negative-duration or the lastFactor guard fires; both are the
	// "cannot solve" outcome we want to exercise.
	if _, _, err := SolvePrepaymentDuration(in, 0); err == nil {
		t.Errorf("expected a cannot-solve error for an oversized extra payment")
	}
}
