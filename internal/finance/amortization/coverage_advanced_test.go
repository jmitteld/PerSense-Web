package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_advanced_test.go — targeted coverage for the advanced backward
// solvers (unknown prepayment amount in both additive and replace modes,
// prepayment duration, fancy term-from-payment) and the prepaid-interest and
// odd-first-period helpers.

// Unknown prepayment amount, REPLACE mode (PlusRegular off): solve the per-period
// extra-payment amount that retires the loan by the stop date.
func TestSolveUnknownPrepaymentReplace(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: cc360(), Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.February, 1),
			PerYrStatus:    types.InOutInput, PerYr: 12,
			StopDateStatus: types.InOutInput, StopDate: types.NewDateRec(2034, time.February, 1),
			PaymentStatus: types.StatusEmpty}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1028.61
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	if math.Abs(r.Schedule[len(r.Schedule)-1].Principal) > 1.0 {
		t.Errorf("solved prepayment should retire the loan: final %.2f", r.Schedule[len(r.Schedule)-1].Principal)
	}
}

// Unknown prepayment amount, ADDITIVE mode (PlusRegular on) — exercises
// solvePrepayAmountAdditive.
func TestSolveUnknownPrepaymentAdditive(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	// A regular payment BELOW the level annuity (1028.61) — it alone would not
	// retire the loan — so a positive additive prepayment is required.
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.February, 1),
			PerYrStatus:    types.InOutInput, PerYr: 12,
			StopDateStatus: types.InOutInput, StopDate: types.NewDateRec(2054, time.January, 1),
			PaymentStatus: types.StatusEmpty}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 950
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	if math.Abs(r.Schedule[len(r.Schedule)-1].Principal) > 1.0 {
		t.Errorf("additive solved prepayment should retire the loan: final %.2f", r.Schedule[len(r.Schedule)-1].Principal)
	}
}

// Prepayment DURATION: known amount, no stop date / count, term known — solve how
// long it must run (AO10, SolvePrepaymentDuration).
func TestSolvePrepaymentDurationCov(t *testing.T) {
	s := cc360()
	s.PlusRegular = true
	// Regular payment below the level annuity so the loan would not retire on its
	// own; a fixed additive prepayment of 600/mo must run for some number of
	// periods (the duration the solver derives) to retire it by the term.
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.February, 1),
			PerYrStatus:   types.InOutInput, PerYr: 12,
			PaymentStatus: types.InOutInput, Payment: 600,
			StopDateStatus: types.StatusEmpty, NNStatus: types.StatusEmpty}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1010
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	if len(r.Schedule) == 0 {
		t.Fatal("empty schedule")
	}
}

// Fancy term-from-payment: blank term + a balloon, supply a payment, let the
// engine run until retirement (solveFancyTermFromPayment).
func TestSolveFancyTermFromPayment(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 0, 12), Settings: cc360(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2030, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 10000}}}
	in.Loan.NStatus = types.StatusEmpty
	in.Loan.LastStatus = types.StatusEmpty
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1100
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	if r.NPeriods <= 0 {
		t.Errorf("expected a derived term, got %d", r.NPeriods)
	}
}

// PrepaidInterest variants: in-advance and daily compounding branches.
func TestPrepaidInterestVariants(t *testing.T) {
	loan := ccBaseLoan(100000, 0.12, 360, 12)
	loan.LoanDate = types.NewDateRec(2024, time.January, 10) // odd day → real stub
	loan.FirstDate = types.NewDateRec(2024, time.March, 1)
	tr := 0.12

	// in-advance prepaid
	sInadv := cc360()
	sInadv.Prepaid, sInadv.InAdvance = true, true
	if v, err := PrepaidInterest(&loan, &sInadv, tr); err != nil || v <= 0 {
		t.Errorf("in-advance prepaid interest = %.2f (err %v), want > 0", v, err)
	}
	// daily prepaid
	sDaily := cc360()
	sDaily.Prepaid, sDaily.Daily = true, true
	if v, err := PrepaidInterest(&loan, &sDaily, tr); err != nil || v <= 0 {
		t.Errorf("daily prepaid interest = %.2f (err %v), want > 0", v, err)
	}
	// not prepaid → zero
	sOff := cc360()
	if v, err := PrepaidInterest(&loan, &sOff, tr); err != nil || v != 0 {
		t.Errorf("non-prepaid interest = %.2f (err %v), want 0", v, err)
	}
}

// oddFirstPeriod: true for a day-mismatch stub, false for a clean month.
func TestOddFirstPeriodHelper(t *testing.T) {
	s := cc360()
	clean := oddFirstPeriod(types.NewDateRec(2024, time.January, 1), types.NewDateRec(2024, time.February, 1), 12, &s)
	if clean {
		t.Error("a clean one-month first period should not be odd on the 360 basis")
	}
	odd := oddFirstPeriod(types.NewDateRec(2024, time.January, 15), types.NewDateRec(2024, time.March, 1), 12, &s)
	if !odd {
		t.Error("a 45-day first period should be odd")
	}
}
