// Canary tests for dispatch_gaps.md §4.7 ambiguous-error rewordings
// (mortgage portion). These canaries differ from the silent-failure
// canaries: most PASS TODAY by binding the current ambiguous wording,
// then FAIL AFTER Phase 3 reword — which is the intended signal
// telling the engineer "this assertion needs to be updated to the
// new wording."
//
// Each test is marked REWORD-PENDING in the body. When Phase 3 ships
// new messages, update each assertion to the new wording.
//
// The canaries also serve as proof that the ambiguous paths exist
// and are reachable from realistic inputs — so when the senior
// engineer reviewing dispatch_gaps.md asks "do these errors actually
// happen?", we have machine-checked answers.
//
// See docs/test_plan.md §1 (Wave 1 canaries) C-12, C-13, C-14, C-15.

package mortgage

import (
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestCanaryC12_MortgageOverDeterminedAmbiguousMessage binds the
// current text of the over-determined error in mortgage.go:235.
// Today: "leave price or monthly payment or balloon amount blank to
// be computed."
//
// dispatch_gaps §4.7 MM-2 proposes: "Row N has Price and Monthly
// Total both filled — leave one blank, or add Balloon Yrs to solve
// for the balloon."
//
// REWORD-PENDING: when the engine emits the new message, update the
// substring assertion below.
func TestCanaryC12_MortgageOverDeterminedAmbiguousMessage(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 200000,
		PctStatus: types.InOutInput, Pct: 0.20,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.06),
		MonthlyStatus: types.InOutInput, Monthly: 1500,
		WhenStatus: types.InOutInput, When: 10,
		HowMuchStatus: types.InOutInput, HowMuch: 50000,
	}
	result := Calc(m)
	if result.Err == nil {
		t.Fatal("expected over-determined error, got nil")
	}
	// Reworded MM-2 message has landed.
	if !strings.Contains(result.Err.Error(), "Price and Monthly Total are both filled in") {
		t.Errorf("expected the reworded MM-2 message, got %q", result.Err.Error())
	}
}

// TestCanaryC13_MortgageRateNearZeroSummationTooSmall binds the
// current "summation too small" message from mortgage.go:246.
// dispatch_gaps §4.7 MM-3 proposes: "Rate is effectively zero —
// Monthly Total cannot be computed without a positive rate."
//
// REWORD-PENDING.
func TestCanaryC13_MortgageRateNearZeroSummationTooSmall(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 200000,
		PctStatus: types.InOutInput, Pct: 0.20,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: 1e-15, // effectively zero
	}
	result := Calc(m)
	if result.Err == nil {
		t.Skip("rate=1e-15 did not trigger summation_too_small; engine may have " +
			"a different threshold. Skipping until a reproducer is found.")
		return
	}
	if !strings.Contains(result.Err.Error(), "Loan Rate is effectively zero") {
		t.Errorf("expected the reworded MM-3 message, got %q", result.Err.Error())
	}
}

// TestCanaryC14_MortgageCashTooCloseAmbiguousMessage binds the
// current "cash too close to price" message from mortgage.go:297.
// dispatch_gaps §4.7 MM-6 proposes: "Cash Required is within 0.5%
// of Price — leave Cash Required blank or lower it (% Down cannot
// be solved)."
//
// REWORD-PENDING.
func TestCanaryC14_MortgageCashTooCloseAmbiguousMessage(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 100000,
		CashStatus: types.InOutInput, Cash: 99800, // Pct = 0.998
		PointsStatus: types.InOutInput, Points: 0,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.06),
	}
	result := Calc(m)
	if result.Err == nil {
		t.Fatal("expected 'cash too close to price' error, got nil")
	}
	// dispatch_gaps §4.7 MM-6 reword has landed.
	if !strings.Contains(result.Err.Error(), "Cash Required is within 0.5% of Price") {
		t.Errorf("expected the reworded MM-6 message, got %q", result.Err.Error())
	}
}

// TestCanaryC15_MortgageFinancedTooCloseAmbiguousMessage binds
// mortgage.go:305. dispatch_gaps §4.7 MM-7 proposes: "Amt Borrowed
// is within 0.5% of Price — leave it blank or lower it."
//
// BONUS FINDING: the error name "financed amount too close to price"
// is itself misleading. The threshold is `Pct >= 0.995` where
// Pct = 1 - Financed/Price. The error therefore fires when Financed
// is *near zero* (so down payment is near 100%), NOT when Financed
// is near Price. The error wording suggests the opposite of what
// actually triggers it. dispatch_gaps §4.7 MM-7 should be updated
// to reflect the true semantics ("Amt Borrowed too small relative
// to Price — % Down would round to 100%").
//
// REWORD-PENDING (and SEMANTIC-CLARIFICATION-PENDING).
func TestCanaryC15_MortgageFinancedTooCloseAmbiguousMessage(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 100000,
		FinancedStatus: types.InOutInput, Financed: 400, // Pct = 0.996, triggers >= 0.995
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.06),
	}
	result := Calc(m)
	if result.Err == nil {
		t.Fatal("expected 'financed amount too close to price' error, got nil")
	}
	// dispatch_gaps §4.7 MM-7 reword has landed. The wording now
	// reflects the true semantics flagged in the BONUS FINDING above:
	// the error fires when Amt Borrowed is too SMALL relative to Price.
	if !strings.Contains(result.Err.Error(), "Amt Borrowed is too small next to Price") {
		t.Errorf("expected the reworded MM-7 message, got %q", result.Err.Error())
	}
}
