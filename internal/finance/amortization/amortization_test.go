package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// --- helpers ---

func newDate(y int, m time.Month, d int) types.DateRec {
	return types.NewDateRec(y, m, d)
}

func defaultSettings() Settings {
	return Settings{
		Basis:       types.Basis360,
		PerYr:       12,
		Prepaid:     true,
		InAdvance:   false,
		PlusRegular: false,
		Exact:       false,
		R78:         false,
		USARule:     false,
		YrDays:      360,
		YrInv:       1.0 / 360,
		CenturyDiv:  50,
		Daily:       false,
	}
}

func makeSimpleLoan() LoanInput {
	return LoanInput{
		Loan: Loan{
			AmountStatus:   types.InOutInput,
			Amount:         100000,
			LoanDateStatus: types.InOutInput,
			LoanDate:       newDate(2024, time.January, 1),
			LoanRateStatus: types.InOutInput,
			LoanRate:       0.06, // 6%
			FirstStatus:    types.InOutInput,
			FirstDate:      newDate(2024, time.February, 1),
			NStatus:        types.InOutInput,
			NPeriods:       360, // 30 years
			LastStatus:     types.InOutOutput,
			LastDate:       newDate(2054, time.January, 1),
			PerYrStatus:    types.InOutInput,
			PerYr:          12,
			PayAmtStatus:   types.InOutInput,
			PayAmt:         599.55, // approximate 30yr 6% payment on $100K
			LastOK:         true,
		},
		Settings: defaultSettings(),
		Fancy:    false,
	}
}

// --- Zero/Empty tests ---

func TestZeroLoan(t *testing.T) {
	var l Loan
	l.Amount = 100000
	l.AmountStatus = types.InOutInput
	ZeroLoan(&l)
	if l.Amount != 0 || l.AmountStatus != 0 {
		t.Error("ZeroLoan should clear all fields")
	}
}

func TestZeroBalloon(t *testing.T) {
	var b BalloonPayment
	b.Amount = 50000
	ZeroBalloon(&b)
	if b.Amount != 0 {
		t.Error("ZeroBalloon should clear amount")
	}
	if !BalloonIsEmpty(&b) {
		t.Error("zeroed balloon should be empty")
	}
}

func TestBalloonIsEmpty(t *testing.T) {
	b := BalloonPayment{}
	if !BalloonIsEmpty(&b) {
		t.Error("default balloon should be empty")
	}
	b.DateStatus = types.InOutInput
	if BalloonIsEmpty(&b) {
		t.Error("balloon with date should not be empty")
	}
}

func TestZeroAdjustment(t *testing.T) {
	var a RateAdjustment
	a.LoanRate = 0.05
	ZeroAdjustment(&a)
	if a.LoanRate != 0 {
		t.Error("ZeroAdjustment should clear rate")
	}
	if !AdjustmentIsEmpty(&a) {
		t.Error("zeroed adjustment should be empty")
	}
}

func TestZeroPrepayment(t *testing.T) {
	var p Prepayment
	p.Payment = 200
	ZeroPrepayment(&p)
	if p.Payment != 0 {
		t.Error("ZeroPrepayment should clear payment")
	}
	if !PrepaymentIsEmpty(&p) {
		t.Error("zeroed prepayment should be empty")
	}
}

// --- MonthSetFromString tests ---

func TestMonthSetFromString(t *testing.T) {
	tests := []struct {
		input    string
		wantSet  []int
		wantErr  bool
	}{
		{"6", []int{6}, false},
		{"1,6,12", []int{1, 6, 12}, false},
		{"6-8", []int{6, 7, 8}, false},
		{"10-2", []int{10, 11, 12, 1, 2}, false}, // wrap around
		{"", nil, false},
		{"13", nil, true}, // out of range
	}
	for _, tt := range tests {
		ms, err := MonthSetFromString(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("MonthSetFromString(%q) should error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("MonthSetFromString(%q) error: %v", tt.input, err)
			continue
		}
		for _, m := range tt.wantSet {
			if !ms[m] {
				t.Errorf("MonthSetFromString(%q): month %d should be set", tt.input, m)
			}
		}
	}
}

// --- GrowthPerPeriod tests ---

func TestGrowthPerPeriod(t *testing.T) {
	loan := &Loan{LoanRate: 0.06, PerYr: 12}
	f := GrowthPerPeriod(loan, 1.0/360)

	// For monthly at 6%: 1 + 0.06/12 = 1.005
	if math.Abs(f-1.005) > 0.0001 {
		t.Errorf("GrowthPerPeriod(monthly, 6%%) = %f, want ~1.005", f)
	}

	// Weekly: 1 + 7/360 * 0.06
	loan52 := &Loan{LoanRate: 0.06, PerYr: 52}
	f52 := GrowthPerPeriod(loan52, 1.0/360)
	expected := 1 + 7.0/360*0.06
	if math.Abs(f52-expected) > 1e-10 {
		t.Errorf("GrowthPerPeriod(weekly) = %f, want %f", f52, expected)
	}
}

// --- RepayLoan tests ---

func TestRepayLoanConverges(t *testing.T) {
	input := makeSimpleLoan()
	loan := input.Loan
	settings := input.Settings

	// Compute the exact payment that zeros the loan
	f := GrowthPerPeriod(&loan, settings.YrInv)

	// Use the annuity formula for exact payment
	lnf, _ := math.Lgamma(1) // dummy
	_ = lnf
	if math.Abs(f-1) > teeny {
		numer := loan.Amount * (f - 1)
		denomExp := math.Pow(f, -float64(loan.NPeriods))
		denom := 1 - denomExp
		exactPayment := numer / denom
		loan.PayAmt = exactPayment

		remaining := RepayLoan(loan.Amount, exactPayment, &loan, &settings, settings.YrInv)
		if math.Abs(remaining) > 1.0 {
			t.Errorf("RepayLoan remaining = %f, want ~0 (payment=%f)", remaining, exactPayment)
		}
	}
}

func TestRepayLoanZeroRate(t *testing.T) {
	loan := Loan{
		Amount:   12000,
		LoanRate: 0,
		PerYr:    12,
		NPeriods: 12,
		LoanDate: newDate(2024, time.January, 1),
		FirstDate: newDate(2024, time.February, 1),
		LastOK:   true,
	}
	settings := defaultSettings()
	settings.Prepaid = false

	remaining := RepayLoan(12000, 1000, &loan, &settings, settings.YrInv)
	// 12 payments of $1000 on $12000 at 0% = exactly 0
	if math.Abs(remaining) > 0.01 {
		t.Errorf("RepayLoan(0%%) remaining = %f, want 0", remaining)
	}
}

// --- SortBalloons tests ---

func TestSortBalloons(t *testing.T) {
	balloons := []BalloonPayment{
		{Date: newDate(2030, time.June, 1), DateStatus: types.InOutInput, Amount: 5000},
		{Date: newDate(2025, time.January, 1), DateStatus: types.InOutInput, Amount: 10000},
		{Date: newDate(2028, time.March, 15), DateStatus: types.InOutInput, Amount: 7500},
	}
	SortBalloons(balloons)
	if balloons[0].Amount != 10000 || balloons[1].Amount != 7500 || balloons[2].Amount != 5000 {
		t.Error("balloons not sorted by date")
	}
}

func TestSortAdjustments(t *testing.T) {
	adjs := []RateAdjustment{
		{Date: newDate(2030, time.January, 1), DateStatus: types.InOutInput, LoanRate: 0.07},
		{Date: newDate(2025, time.January, 1), DateStatus: types.InOutInput, LoanRate: 0.05},
	}
	SortAdjustments(adjs)
	if adjs[0].LoanRate != 0.05 || adjs[1].LoanRate != 0.07 {
		t.Error("adjustments not sorted by date")
	}
}

// --- Amortize simple loan tests ---

func TestAmortizeSimpleLoan(t *testing.T) {
	input := makeSimpleLoan()

	result := Amortize(input)
	if result.Err != nil {
		t.Fatalf("Amortize error: %v", result.Err)
	}

	if len(result.Schedule) != 360 {
		t.Errorf("schedule has %d periods, want 360", len(result.Schedule))
	}

	// First payment should have interest and principal
	first := result.Schedule[0]
	if first.PayNum != 1 {
		t.Errorf("first payment num = %d, want 1", first.PayNum)
	}
	if first.Interest <= 0 {
		t.Error("first payment should have positive interest")
	}
	if first.PayAmt <= 0 {
		t.Error("first payment amount should be positive")
	}

	// Total interest should be roughly $115K for a $100K 30yr 6% loan
	if result.TotalInt < 50000 || result.TotalInt > 250000 {
		t.Errorf("total interest = %f, expected roughly 100K-120K", result.TotalInt)
	}

	// Final principal should be close to 0
	last := result.Schedule[len(result.Schedule)-1]
	if math.Abs(last.Principal) > 1.0 {
		t.Errorf("final principal = %f, want ~0", last.Principal)
	}
}

func TestAmortizeInsufficientData(t *testing.T) {
	result := Amortize(LoanInput{Settings: defaultSettings()})
	if result.Err == nil {
		t.Error("empty loan should produce error")
	}
}

// --- Amortize with balloons ---

func TestAmortizeWithBalloon(t *testing.T) {
	input := makeSimpleLoan()
	input.Fancy = true
	input.Loan.NPeriods = 60 // 5 years
	input.Loan.LastDate = newDate(2029, time.January, 1)
	input.Balloons = []BalloonPayment{
		{
			DateStatus:   types.InOutInput,
			Date:         newDate(2029, time.January, 1),
			AmountStatus: types.InOutInput,
			Amount:       80000, // balloon at end
		},
	}

	result := Amortize(input)
	if result.Err != nil {
		t.Fatalf("Amortize with balloon error: %v", result.Err)
	}

	if len(result.Schedule) == 0 {
		t.Fatal("no schedule generated")
	}

	// Should have paid balloon at some point
	hasBigPayment := false
	for _, rec := range result.Schedule {
		if rec.PayAmt > 50000 {
			hasBigPayment = true
			break
		}
	}
	if !hasBigPayment {
		t.Error("expected a balloon payment > 50000 in schedule")
	}
}

// --- PrepaidInterest tests ---

func TestPrepaidInterest(t *testing.T) {
	loan := Loan{
		Amount:    100000,
		LoanRate:  0.06,
		PerYr:     12,
		LoanDate:  newDate(2024, time.January, 15),
		FirstDate: newDate(2024, time.March, 1),
	}
	settings := defaultSettings()

	pi, err := PrepaidInterest(&loan, &settings, 0.06)
	if err != nil {
		t.Fatal(err)
	}
	// Should be positive: interest from Jan 15 to ~Feb 1 (one period before Mar 1)
	if pi <= 0 {
		t.Errorf("prepaid interest = %f, want > 0", pi)
	}
	// Rough check: ~16 days at 6%/360 on $100K ≈ $266
	if pi < 100 || pi > 1000 {
		t.Errorf("prepaid interest = %f, seems out of range", pi)
	}
}

func TestPrepaidInterestNotPrepaid(t *testing.T) {
	loan := Loan{Amount: 100000, LoanRate: 0.06}
	settings := defaultSettings()
	settings.Prepaid = false

	pi, err := PrepaidInterest(&loan, &settings, 0.06)
	if err != nil {
		t.Fatal(err)
	}
	if pi != 0 {
		t.Errorf("prepaid interest with prepaid=false should be 0, got %f", pi)
	}
}

// --- Schedule properties tests ---

func TestScheduleInterestDecreasing(t *testing.T) {
	// For a fixed-rate, fully-amortizing loan, interest should decrease over time
	input := makeSimpleLoan()
	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	if len(result.Schedule) < 10 {
		t.Skip("schedule too short")
	}

	first := result.Schedule[0].Interest
	last := result.Schedule[len(result.Schedule)-2].Interest // second to last
	if last >= first {
		t.Errorf("interest should decrease: first=%f, second-to-last=%f", first, last)
	}
}

func TestScheduleCumulativeInterest(t *testing.T) {
	input := makeSimpleLoan()
	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	// IntToDate should be monotonically increasing
	prev := 0.0
	for i, rec := range result.Schedule {
		if rec.IntToDate < prev-0.001 {
			t.Errorf("IntToDate decreased at period %d: %f < %f", i+1, rec.IntToDate, prev)
			break
		}
		prev = rec.IntToDate
	}

	// Last IntToDate should match TotalInt
	last := result.Schedule[len(result.Schedule)-1]
	if math.Abs(last.IntToDate-result.TotalInt) > 1.0 {
		t.Errorf("last IntToDate=%f != TotalInt=%f", last.IntToDate, result.TotalInt)
	}
}
