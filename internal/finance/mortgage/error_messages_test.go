// Error-message tests for the mortgage engine.
//
// Each test builds an MtgLine (or row-generation call) that triggers a
// "calculation cannot be done" error — under-determined, over-determined,
// inconsistent, or too-small-to-compute — and asserts the error is
// non-nil and its message contains the key, user-facing phrase.
//
// These cover the reworded messages added for the error-message
// improvement pass. They guard against silent regressions to vaguer
// wording. Field labels follow the Mortgage screen UI (Price, % Down,
// Cash Required, Amt Borrowed, Years, Loan Rate, Mo Tax+Ins, Monthly
// Total, Balloon Yrs, Balloon Amt).

package mortgage

import (
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

func mustContain(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error message %q does not contain expected phrase %q", err.Error(), want)
	}
}

// Years entered as zero or negative — bad/insufficient term input.
func TestErrMsg_YearsNotPositive(t *testing.T) {
	m := MtgLine{
		YearsStatus: types.InOutInput, Years: 0,
	}
	res := Calc(m)
	mustContain(t, res.Err, "Years must be a positive whole number")
}

// Balloon Amt filled but Balloon Yrs blank — inconsistent balloon inputs.
func TestErrMsg_BalloonAmtWithoutBalloonYears(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 200000,
		HowMuchStatus: types.InOutInput, HowMuch: 50000,
	}
	res := Calc(m)
	mustContain(t, res.Err, "Balloon Amt is filled in but Balloon Yrs is blank")
}

// Price, Monthly Total and a known balloon all filled — over-determined.
func TestErrMsg_OverDeterminedPriceAndMonthly(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 200000,
		PctStatus: types.InOutInput, Pct: 0.20,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.06),
		MonthlyStatus: types.InOutInput, Monthly: 1500,
		WhenStatus: types.InOutInput, When: 10,
		HowMuchStatus: types.InOutInput, HowMuch: 50000,
	}
	res := Calc(m)
	mustContain(t, res.Err, "Price and Monthly Total are both filled in")
}

// Loan Rate so large that the payment summation collapses to zero —
// Monthly Total cannot be computed.
func TestErrMsg_RateEffectivelyZeroSummation(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 200000,
		PctStatus: types.InOutInput, Pct: 0.20,
		YearsStatus: types.InOutInput, Years: 30,
		// True rate large enough that e^(-r/12) underflows, so the
		// Summation factor collapses below the teeny threshold.
		RateStatus: types.InOutInput, Rate: 1000,
	}
	res := Calc(m)
	mustContain(t, res.Err, "Loan Rate is effectively zero")
}

// Monthly Total filled, but no funding field (% Down / Cash Required)
// to anchor Price — under-determined.
func TestErrMsg_PriceFromMonthlyNeedsFunding(t *testing.T) {
	m := MtgLine{
		FinancedStatus: types.InOutInput, Financed: 160000,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.06),
		MonthlyStatus: types.InOutInput, Monthly: 959,
	}
	res := Calc(m)
	mustContain(t, res.Err, "Not enough data to solve for Price from Monthly Total")
}

// Price is zero — cannot derive % Down / Cash Required / Amt Borrowed.
func TestErrMsg_PriceMustBePositive(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 0,
		PctStatus: types.InOutInput, Pct: 0.20,
	}
	res := Calc(m)
	mustContain(t, res.Err, "Price must be greater than zero")
}

// Cash Required nearly equals Price — % Down rounds to 100%, unsolvable.
func TestErrMsg_CashRequiredTooCloseToPrice(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 100000,
		CashStatus: types.InOutInput, Cash: 99800,
		PointsStatus: types.InOutInput, Points: 0,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.06),
	}
	res := Calc(m)
	mustContain(t, res.Err, "Cash Required is within 0.5% of Price")
}

// Amt Borrowed near zero relative to Price — % Down rounds to 100%.
func TestErrMsg_AmtBorrowedTooSmallVsPrice(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 100000,
		FinancedStatus: types.InOutInput, Financed: 400,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.06),
	}
	res := Calc(m)
	mustContain(t, res.Err, "Amt Borrowed is too small next to Price")
}

// APR comparison with an under-specified mortgage A.
func TestErrMsg_CompareAPRsMortgageANotEnoughData(t *testing.T) {
	a := MtgLine{
		FinancedStatus: types.InOutInput, Financed: 160000,
		// Monthly Total / Loan Rate / Years all missing.
	}
	b := MtgLine{
		FinancedStatus: types.InOutInput, Financed: 160000,
		MonthlyStatus: types.InOutInput, Monthly: 959,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.06),
		YearsStatus: types.InOutInput, Years: 30,
	}
	_, err := CompareAPRs(a, b, 360.0)
	mustContain(t, err, "Mortgage A does not have enough data to compute an APR")
}

// APR comparison with an under-specified mortgage B.
func TestErrMsg_CompareAPRsMortgageBNotEnoughData(t *testing.T) {
	a := MtgLine{
		FinancedStatus: types.InOutInput, Financed: 160000,
		MonthlyStatus: types.InOutInput, Monthly: 959,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.06),
		YearsStatus: types.InOutInput, Years: 30,
	}
	b := MtgLine{
		FinancedStatus: types.InOutInput, Financed: 160000,
	}
	_, err := CompareAPRs(a, b, 360.0)
	mustContain(t, err, "Mortgage B does not have enough data to compute an APR")
}
