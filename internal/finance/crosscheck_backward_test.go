// Cross-check tests for backward (solve-for-unknown) paths against
// DOS reference data. Reuses refdata.json's mortgage_calc table —
// each row gives a known {price, pct, years, rate} → {financed,
// monthly} mapping that we run *backward* to verify the new solvers
// recover the original inputs.
//
// These tests catch systematic biases that round-trip tests can't:
// if both forward and backward share an off-by-one error, round-trip
// passes but the absolute numbers don't match DOS.

package finance

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/finance/amortization"
	"github.com/persense/persense-port/internal/finance/mortgage"
	"github.com/persense/persense-port/internal/types"
)

// TestCrossCheckMortgageBackwardPrice uses the forward mortgage_calc
// table in reverse: given the DOS-known monthly + financed + rate +
// years, verify mortgage.Calc recovers the price (or matches the
// computed price within rounding).
//
// The reference table fields:
//   price=200000, pct=0.20, years=30, rate=0.06
//     → financed=160000, monthly=960.826966907258
//
// We reconstruct the row with monthly known and price unknown, then
// confirm Calc produces the same price.
func TestCrossCheckMortgageBackwardPrice(t *testing.T) {
	ref := loadRefData(t)
	for _, tc := range ref.MortgageCalc {
		// Skip rows with 0% rate (zero-rate mortgages have a different
		// degenerate path that the Calc reverse doesn't currently
		// exercise via this combination).
		if tc.Rate == 0 {
			continue
		}
		// Provide monthly + cash (which is price * pct since points=0)
		// + years + rate, leave price blank.
		// At points=0, cash = price * pct.
		expectedCash := tc.Price * tc.Pct
		m := mortgage.MtgLine{
			MonthlyStatus: types.InOutInput,
			Monthly:       tc.Monthly,
			CashStatus:    types.InOutInput,
			Cash:          expectedCash,
			YearsStatus:   types.InOutInput,
			Years:         tc.Years,
			RateStatus:    types.InOutInput,
			Rate:          tc.Rate,
			TaxStatus:     types.InOutInput,
			Tax:           0,
			BalloonStat:   types.BalloonBlank,
		}
		result := mortgage.Calc(m)
		if result.Err != nil {
			t.Errorf("backward Calc (monthly=%g, cash=%g, yrs=%d, rate=%g): %v",
				tc.Monthly, expectedCash, tc.Years, tc.Rate, result.Err)
			continue
		}
		// Solved price should match within $0.01.
		if math.Abs(result.Line.Price-tc.Price) > 0.01 {
			t.Errorf("backward price (DOS row): got %.4f, want %.4f (delta %.4f)",
				result.Line.Price, tc.Price,
				result.Line.Price-tc.Price)
		}
	}
}

// TestCrossCheckAmortBackwardLoanAmount verifies SolveLoanAmount
// recovers the financed amount from each DOS reference row's
// {monthly, rate, years} inputs.
//
// The Pascal Mortgage.Summation formula uses continuous compounding,
// so the standard annuity SolveLoanAmount produces slightly different
// numbers (documented in docs/discrepancies.md §1). We accept a 1%
// tolerance here because the comparison is across formula families.
func TestCrossCheckAmortBackwardLoanAmount(t *testing.T) {
	ref := loadRefData(t)
	for _, tc := range ref.MortgageCalc {
		if tc.Rate == 0 || tc.Financed == 0 {
			// SolveLoanAmount errors on zero rate.
			continue
		}
		loan := amortization.Loan{
			LoanRateStatus: types.InOutInput,
			LoanRate:       tc.Rate,
			PayAmtStatus:   types.InOutInput,
			PayAmt:         tc.Monthly,
			NStatus:        types.InOutInput,
			NPeriods:       tc.Years * 12,
			PerYrStatus:    types.InOutInput,
			PerYr:          12,
			LoanDateStatus: types.InOutInput,
			LoanDate:       types.NewDateRec(2024, 1, 1),
			FirstStatus:    types.InOutInput,
			FirstDate:      types.NewDateRec(2024, 2, 1),
			AmountStatus:   types.StatusEmpty,
		}
		input := amortization.LoanInput{
			Loan: loan,
			Settings: amortization.Settings{
				Basis:  types.Basis360,
				PerYr:  12,
				YrDays: 360,
				YrInv:  1.0 / 360,
			},
		}
		got, _, err := amortization.SolveLoanAmount(input)
		if err != nil {
			t.Errorf("SolveLoanAmount(rate=%g, pmt=%g, yrs=%d): %v",
				tc.Rate, tc.Monthly, tc.Years, err)
			continue
		}
		// Tolerance: 1% — see docs/discrepancies.md §1 about the
		// Pascal continuous-compounding flavor vs. standard annuity.
		relErr := math.Abs(got-tc.Financed) / tc.Financed
		if relErr > 0.01 {
			t.Errorf("SolveLoanAmount(rate=%g, pmt=%g, yrs=%d): got %.2f, "+
				"want ~%.2f (relErr=%.2e)",
				tc.Rate, tc.Monthly, tc.Years, got, tc.Financed, relErr)
		}
	}
}

// TestCrossCheckAmortBackwardRate verifies SolveRate recovers the
// rate from each DOS reference row's {financed, monthly, years}.
//
// Tolerance ±0.01 on the rate (1 percentage point) reflects the
// continuous-vs-discrete compounding gap plus the Newton iteration's
// own convergence band. The point of the test is to catch order-of-
// magnitude regressions, not to pin down the exact DOS rate.
func TestCrossCheckAmortBackwardRate(t *testing.T) {
	ref := loadRefData(t)
	for _, tc := range ref.MortgageCalc {
		if tc.Rate == 0 {
			continue
		}
		loan := amortization.Loan{
			AmountStatus:   types.InOutInput,
			Amount:         tc.Financed,
			PayAmtStatus:   types.InOutInput,
			PayAmt:         tc.Monthly,
			NStatus:        types.InOutInput,
			NPeriods:       tc.Years * 12,
			PerYrStatus:    types.InOutInput,
			PerYr:          12,
			LoanDateStatus: types.InOutInput,
			LoanDate:       types.NewDateRec(2024, 1, 1),
			FirstStatus:    types.InOutInput,
			FirstDate:      types.NewDateRec(2024, 2, 1),
			LoanRateStatus: types.StatusEmpty,
		}
		input := amortization.LoanInput{
			Loan: loan,
			Settings: amortization.Settings{
				Basis:  types.Basis360,
				PerYr:  12,
				YrDays: 360,
				YrInv:  1.0 / 360,
			},
		}
		got, _, err := amortization.SolveRate(input)
		if err != nil {
			t.Errorf("SolveRate(amount=%g, pmt=%g, yrs=%d): %v",
				tc.Financed, tc.Monthly, tc.Years, err)
			continue
		}
		if math.Abs(got-tc.Rate) > 0.01 {
			t.Errorf("SolveRate(amount=%g, pmt=%g, yrs=%d): got %.4f, "+
				"want ~%.4f", tc.Financed, tc.Monthly, tc.Years,
				got, tc.Rate)
		}
	}
}
