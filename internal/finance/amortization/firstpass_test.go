package amortization

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// --- FirstPass: date/period derivation ---

// A-FP-defFirst (DOS DefaultFirstPaymentDate, Amortize.pas:184-194): when
// firstDate is blank, default to the first of the SECOND following month when
// the loan day > 1 (snap to the 1st, advance two periods). Loan 2024-01-15
// (day 15 > 1) -> 2024-03-01. (AM_EX1.html shows the same rule: 6/21 -> 8/1.)
func TestFirstPassDefaultFirstPaymentDate(t *testing.T) {
	loan := Loan{
		AmountStatus:   types.InOutInput,
		Amount:         100000,
		LoanDateStatus: types.InOutInput,
		LoanDate:       newDate(2024, time.January, 15),
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		FirstStatus:    types.StatusEmpty,
		FirstDate:      types.UnknownDate(),
		NStatus:        types.InOutInput,
		NPeriods:       12,
	}
	if err := FirstPass(&loan); err != nil {
		t.Fatal(err)
	}
	if loan.FirstStatus < types.InOutDefault {
		t.Error("FirstStatus should have been bumped to default")
	}
	got := loan.FirstDate.Time
	if got.Year() != 2024 || got.Month() != time.March || got.Day() != 1 {
		t.Errorf("default firstDate = %s, expected 2024-03-01",
			got.Format("2006-01-02"))
	}
}

// A-FP-last: when firstDate and N are known but lastDate is blank,
// FirstPass should derive lastDate = firstDate + (N-1) periods and set
// LastOK = true.
func TestFirstPassDeriveLastDate(t *testing.T) {
	loan := Loan{
		LoanDateStatus: types.InOutInput,
		LoanDate:       newDate(2024, time.January, 1),
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		FirstStatus:    types.InOutInput,
		FirstDate:      newDate(2024, time.February, 1),
		NStatus:        types.InOutInput,
		NPeriods:       360,
		LastStatus:     types.StatusEmpty,
		LastDate:       types.UnknownDate(),
	}
	if err := FirstPass(&loan); err != nil {
		t.Fatal(err)
	}
	if !loan.LastOK {
		t.Error("LastOK should be true after deriving lastDate")
	}
	got := loan.LastDate.Time
	// 360 monthly payments starting 2024-02-01 -> 2054-01-01
	if got.Year() != 2054 || got.Month() != time.January || got.Day() != 1 {
		t.Errorf("derived lastDate = %s, expected 2054-01-01",
			got.Format("2006-01-02"))
	}
	if loan.LastStatus != types.InOutOutput {
		t.Errorf("LastStatus = %d, want InOutOutput=%d",
			loan.LastStatus, types.InOutOutput)
	}
}

// A-FP-n: when firstDate and lastDate are known but N is blank,
// FirstPass should derive N = NumberOfInstallments(first, last).
func TestFirstPassDeriveNPeriods(t *testing.T) {
	loan := Loan{
		LoanDateStatus: types.InOutInput,
		LoanDate:       newDate(2024, time.January, 1),
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		FirstStatus:    types.InOutInput,
		FirstDate:      newDate(2024, time.February, 1),
		LastStatus:     types.InOutInput,
		LastDate:       newDate(2054, time.January, 1),
		NStatus:        types.StatusEmpty,
	}
	if err := FirstPass(&loan); err != nil {
		t.Fatal(err)
	}
	if !loan.LastOK {
		t.Error("LastOK should be true when lastDate is supplied")
	}
	if loan.NPeriods != 360 {
		t.Errorf("derived NPeriods = %d, expected 360", loan.NPeriods)
	}
	if loan.NStatus != types.InOutOutput {
		t.Errorf("NStatus = %d, want InOutOutput=%d",
			loan.NStatus, types.InOutOutput)
	}
}

// FirstPass should be a no-op when all three fields are already given.
func TestFirstPassNoOpWhenAllSpecified(t *testing.T) {
	loan := Loan{
		LoanDateStatus: types.InOutInput,
		LoanDate:       newDate(2024, time.January, 1),
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		FirstStatus:    types.InOutInput,
		FirstDate:      newDate(2024, time.February, 1),
		LastStatus:     types.InOutInput,
		LastDate:       newDate(2054, time.January, 1),
		NStatus:        types.InOutInput,
		NPeriods:       360,
	}
	orig := loan
	if err := FirstPass(&loan); err != nil {
		t.Fatal(err)
	}
	if loan.FirstDate != orig.FirstDate ||
		loan.LastDate != orig.LastDate ||
		loan.NPeriods != orig.NPeriods {
		t.Error("FirstPass should not modify a fully-specified loan")
	}
	if !loan.LastOK {
		t.Error("LastOK should be true when lastDate is supplied")
	}
}

// Amortize end-to-end: caller can omit FirstDate and the schedule
// should still run.
func TestAmortizeWithoutFirstDate(t *testing.T) {
	input := makeSimpleLoan()
	input.Loan.FirstStatus = types.StatusEmpty
	input.Loan.FirstDate = types.UnknownDate()
	input.Loan.LastOK = false

	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.Schedule) != 360 {
		t.Errorf("schedule length = %d, want 360", len(result.Schedule))
	}
}

// Amortize end-to-end: caller can omit LastDate (NPeriods is enough)
// and the schedule should still run; LastOK should be set by FirstPass.
func TestAmortizeFancyWithoutLastDate(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	input.Loan.LastStatus = types.StatusEmpty
	input.Loan.LastDate = types.UnknownDate()
	input.Loan.LastOK = false

	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.Schedule) == 0 {
		t.Fatal("fancy schedule should not be empty when lastDate is omitted")
	}
	// Schedule should terminate around 360 periods (some leeway for the
	// terminator's $1 residual cutoff).
	if len(result.Schedule) < 300 || len(result.Schedule) > 365 {
		t.Errorf("schedule length = %d, expected ~360", len(result.Schedule))
	}
}

// --- Input validation: amortization ---

// C-A-1: two adjustments on the same date.
func TestValidateDuplicateAdjustmentDates(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	r1, r2 := 0.05, 0.04
	input.Adjustments = []RateAdjustment{
		{DateStatus: types.InOutInput, Date: newDate(2030, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: r1},
		{DateStatus: types.InOutInput, Date: newDate(2030, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: r2},
	}
	result := Amortize(input)
	if result.Err == nil ||
		!strings.Contains(result.Err.Error(), "same date") {
		t.Errorf("expected 'same date' error, got %v", result.Err)
	}
}

// C-A-2: adjustment date <= loanDate.
func TestValidateAdjustmentBeforeLoanDate(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	r := 0.05
	input.Adjustments = []RateAdjustment{
		{DateStatus: types.InOutInput,
			Date:           newDate(2023, time.January, 1),
			LoanRateStatus: types.InOutInput,
			LoanRate:       r},
	}
	result := Amortize(input)
	if result.Err == nil ||
		!strings.Contains(result.Err.Error(), "before the Loan Date") {
		t.Errorf("expected adjustment-before-loan error, got %v", result.Err)
	}
}

// C-A-3: adjustment date >= lastDate.
func TestValidateAdjustmentAfterLastDate(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	r := 0.05
	input.Adjustments = []RateAdjustment{
		{DateStatus: types.InOutInput,
			Date:           newDate(2060, time.January, 1),
			LoanRateStatus: types.InOutInput,
			LoanRate:       r},
	}
	result := Amortize(input)
	if result.Err == nil ||
		!strings.Contains(result.Err.Error(), "after") {
		t.Errorf("expected 'after the last payment' error, got %v", result.Err)
	}
}

// C-A-4: balloon date < firstDate.
func TestValidateBalloonBeforeFirstDate(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	input.Balloons = []BalloonPayment{
		{DateStatus: types.InOutInput,
			Date:         newDate(2023, time.December, 1),
			AmountStatus: types.InOutInput,
			Amount:       5000},
	}
	result := Amortize(input)
	if result.Err == nil ||
		!strings.Contains(result.Err.Error(), "before the 1st Pmt") {
		t.Errorf("expected balloon-precedes-first error, got %v", result.Err)
	}
}

// C-A-5: firstDate > lastDate. The Go port allows firstDate == lastDate
// (degenerate 1-payment loan) but rejects strictly out-of-order dates.
func TestValidateFirstAfterLast(t *testing.T) {
	input := makeSimpleLoan()
	input.Loan.LastDate = newDate(2024, time.January, 1) // before firstDate
	input.Loan.LastStatus = types.InOutInput
	input.Loan.LastOK = true
	result := Amortize(input)
	if result.Err == nil ||
		!strings.Contains(result.Err.Error(), "after Last Pmt Date") {
		t.Errorf("expected first-after-last error, got %v", result.Err)
	}
}

// C-A-6: moratorium first-repay < firstDate.
func TestValidateMoratoriumBeforeFirst(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	input.Moratorium = Moratorium{
		FirstRepayStatus: types.InOutInput,
		FirstRepay:       newDate(2023, time.December, 1),
	}
	result := Amortize(input)
	if result.Err == nil ||
		!strings.Contains(result.Err.Error(), "before the 1st Pmt Date") {
		t.Errorf("expected moratorium-precedes-first error, got %v", result.Err)
	}
}

// C-A-7: balloon before moratorium first-repay (and after firstDate).
// firstDate is 2024-02-01, moratorium first-repay 2026-01-01, balloon 2025-06-01.
func TestValidateBalloonBeforeMoratorium(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	input.Moratorium = Moratorium{
		FirstRepayStatus: types.InOutInput,
		FirstRepay:       newDate(2026, time.January, 1),
	}
	input.Balloons = []BalloonPayment{
		{DateStatus: types.InOutInput,
			Date:         newDate(2025, time.June, 1),
			AmountStatus: types.InOutInput,
			Amount:       5000},
	}
	result := Amortize(input)
	if result.Err == nil ||
		!strings.Contains(result.Err.Error(), "Moratorium") {
		t.Errorf("expected balloon-precedes-moratorium error, got %v", result.Err)
	}
}

// C-A-9: target principal reduction > amount / N.
func TestValidateTargetTooHigh(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	// $100K / 360 = ~$278, so a target of $500 is unreachable.
	input.Target = Target{
		TargetStatus: types.InOutInput,
		TargetValue:  500,
	}
	result := Amortize(input)
	if result.Err == nil ||
		!strings.Contains(result.Err.Error(), "Target is too high") {
		t.Errorf("expected target-too-high error, got %v", result.Err)
	}
}

// C-A-10: SolveLoanAmount + fancy + target should error.
func TestSolveLoanAmountRejectedWithTarget(t *testing.T) {
	input := LoanInput{
		Loan: Loan{
			AmountStatus:   types.StatusEmpty, // solving for this
			LoanRateStatus: types.InOutInput, LoanRate: 0.06,
			PayAmtStatus: types.InOutInput, PayAmt: 1500,
			NStatus:     types.InOutInput, NPeriods: 360,
			PerYrStatus: types.InOutInput, PerYr: 12,
			FirstStatus: types.InOutInput,
			FirstDate:   newDate(2024, time.February, 1),
			LoanDate:    newDate(2024, time.January, 1),
		},
		Settings: basicSettings(),
		Fancy:    true,
		Target: Target{
			TargetStatus: types.InOutInput,
			TargetValue:  100,
		},
	}
	_, _, err := SolveLoanAmount(input)
	if err == nil ||
		!strings.Contains(err.Error(), "Target") {
		t.Errorf("expected target-rejection error, got %v", err)
	}
}

// Adjustments equal to loanDate are rejected (DOS uses <=).
func TestValidateAdjustmentEqualsLoanDate(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	r := 0.05
	input.Adjustments = []RateAdjustment{
		{DateStatus: types.InOutInput,
			Date:           newDate(2024, time.January, 1),
			LoanRateStatus: types.InOutInput,
			LoanRate:       r},
	}
	result := Amortize(input)
	if result.Err == nil ||
		!strings.Contains(result.Err.Error(), "before the Loan Date") {
		t.Errorf("expected adjustment-precedes-loan error, got %v", result.Err)
	}
}

// Valid inputs should NOT trip any of the new validations.
func TestValidateValidInputsPass(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	r := 0.05
	input.Adjustments = []RateAdjustment{
		{DateStatus: types.InOutInput,
			Date:           newDate(2029, time.January, 1),
			LoanRateStatus: types.InOutInput,
			LoanRate:       r},
	}
	input.Balloons = []BalloonPayment{
		{DateStatus: types.InOutInput,
			Date:         newDate(2030, time.January, 1),
			AmountStatus: types.InOutInput,
			Amount:       5000},
	}
	result := Amortize(input)
	if result.Err != nil {
		t.Fatalf("valid input rejected: %v", result.Err)
	}
}
