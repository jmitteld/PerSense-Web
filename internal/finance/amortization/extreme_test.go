package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func nd(y int, m time.Month, d int) types.DateRec {
	return types.NewDateRec(y, m, d)
}

func ds() Settings {
	return Settings{
		Basis: types.Basis360, PerYr: 12, Prepaid: true,
		YrDays: 360, YrInv: 1.0 / 360, CenturyDiv: 50,
	}
}

// --- GrowthPerPeriod extreme ---

func TestGrowthPerPeriodZeroRate(t *testing.T) {
	loan := &Loan{LoanRate: 0, PerYr: 12}
	f := GrowthPerPeriod(loan, 1.0/360)
	if f != 1 {
		t.Errorf("GrowthPerPeriod(0%%) = %f, want 1", f)
	}
}

func TestGrowthPerPeriodHighRate(t *testing.T) {
	loan := &Loan{LoanRate: 1.0, PerYr: 12} // 100%
	f := GrowthPerPeriod(loan, 1.0/360)
	expected := 1 + 1.0/12
	if math.Abs(f-expected) > 1e-10 {
		t.Errorf("GrowthPerPeriod(100%%) = %f, want %f", f, expected)
	}
}

// --- RepayLoan extreme ---

func TestRepayLoanZeroRateExtreme(t *testing.T) {
	loan := Loan{
		Amount: 12000, LoanRate: 0, PerYr: 12, NPeriods: 12,
		LoanDate:  nd(2024, time.January, 1),
		FirstDate: nd(2024, time.February, 1),
	}
	s := ds()
	s.Prepaid = false
	remaining := RepayLoan(12000, 1000, &loan, &s, s.YrInv)
	if math.Abs(remaining) > 0.01 {
		t.Errorf("remaining at 0%% = %f, want ~0", remaining)
	}
}

func TestRepayLoanOnePayment(t *testing.T) {
	loan := Loan{
		Amount: 1000, LoanRate: 0.12, PerYr: 12, NPeriods: 1,
		LoanDate:  nd(2024, time.January, 1),
		FirstDate: nd(2024, time.February, 1),
	}
	s := ds()
	// One payment: pay off principal + one month interest
	// Interest = 1000 * 0.12 * (1/12) = 10, so payment = 1010
	remaining := RepayLoan(1000, 1010, &loan, &s, s.YrInv)
	if math.Abs(remaining) > 1 {
		t.Errorf("one-payment remaining = %f, want ~0", remaining)
	}
}

func TestRepayLoanVeryLongTerm(t *testing.T) {
	// 50-year loan: should not panic or hang
	loan := Loan{
		Amount: 100000, LoanRate: 0.06, PerYr: 12, NPeriods: 600,
		LoanDate:  nd(2024, time.January, 1),
		FirstDate: nd(2024, time.February, 1),
	}
	s := ds()
	payment := 500.0 // rough estimate
	remaining := RepayLoan(100000, payment, &loan, &s, s.YrInv)
	_ = remaining // just checking it completes
}

// --- Amortize extreme ---

func TestAmortizeZeroRate(t *testing.T) {
	input := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 12000,
			LoanDateStatus: types.InOutInput, LoanDate: nd(2024, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0,
			FirstStatus: types.InOutInput, FirstDate: nd(2024, time.February, 1),
			NStatus: types.InOutInput, NPeriods: 12,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 1000,
			LastOK: true,
		},
		Settings: ds(),
	}
	input.Settings.Prepaid = false
	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	// At 0%, total interest should be 0
	if result.TotalInt > 1 {
		t.Errorf("total interest at 0%% = %f, want ~0", result.TotalInt)
	}
	// Total paid should equal loan amount
	if math.Abs(result.TotalPaid-12000) > 10 {
		t.Errorf("total paid at 0%% = %f, want ~12000", result.TotalPaid)
	}
}

func TestAmortizeHighRate(t *testing.T) {
	// 24% interest rate — should still work
	input := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 10000,
			LoanDateStatus: types.InOutInput, LoanDate: nd(2024, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.24,
			FirstStatus: types.InOutInput, FirstDate: nd(2024, time.February, 1),
			NStatus: types.InOutInput, NPeriods: 60,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 300,
			LastOK: true,
		},
		Settings: ds(),
	}
	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	// First payment interest should be substantial: 10000 * 0.24/12 = 200
	if len(result.Schedule) == 0 {
		t.Fatal("no schedule")
	}
	if result.Schedule[0].Interest < 150 || result.Schedule[0].Interest > 250 {
		t.Errorf("first interest at 24%% = %f, expected ~200", result.Schedule[0].Interest)
	}
}

func TestAmortizeLargeAmount(t *testing.T) {
	// $10M loan
	input := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 10000000,
			LoanDateStatus: types.InOutInput, LoanDate: nd(2024, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.05,
			FirstStatus: types.InOutInput, FirstDate: nd(2024, time.February, 1),
			NStatus: types.InOutInput, NPeriods: 360,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 53682.16,
			LastOK: true,
		},
		Settings: ds(),
	}
	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.Schedule) != 360 {
		t.Errorf("schedule length = %d, want 360", len(result.Schedule))
	}
	// Interest should be massive
	if result.TotalInt < 5000000 {
		t.Errorf("total interest on $10M = %f, expected > $5M", result.TotalInt)
	}
}

func TestAmortizeShortTerm(t *testing.T) {
	// 1 payment
	input := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 1000,
			LoanDateStatus: types.InOutInput, LoanDate: nd(2024, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.06,
			FirstStatus: types.InOutInput, FirstDate: nd(2024, time.February, 1),
			NStatus: types.InOutInput, NPeriods: 1,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 1005.00,
			LastOK: true,
		},
		Settings: ds(),
	}
	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.Schedule) != 1 {
		t.Errorf("schedule length = %d, want 1", len(result.Schedule))
	}
}

// --- MonthSetFromString extreme ---

func TestMonthSetFullYear(t *testing.T) {
	ms, err := MonthSetFromString("1-12")
	if err != nil {
		t.Fatal(err)
	}
	for m := 1; m <= 12; m++ {
		if !ms[m] {
			t.Errorf("month %d should be set for 1-12", m)
		}
	}
}

func TestMonthSetEmpty(t *testing.T) {
	ms, err := MonthSetFromString("")
	if err != nil {
		t.Fatal(err)
	}
	for m := 1; m <= 12; m++ {
		if ms[m] {
			t.Errorf("month %d should not be set for empty string", m)
		}
	}
}

func TestMonthSetSingle(t *testing.T) {
	ms, err := MonthSetFromString("7")
	if err != nil {
		t.Fatal(err)
	}
	set := 0
	for m := 1; m <= 12; m++ {
		if ms[m] {
			set++
		}
	}
	if set != 1 || !ms[7] {
		t.Error("only month 7 should be set")
	}
}

// --- Schedule properties ---

func TestSchedulePrincipalDecreasing(t *testing.T) {
	input := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanDateStatus: types.InOutInput, LoanDate: nd(2024, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.06,
			FirstStatus: types.InOutInput, FirstDate: nd(2024, time.February, 1),
			NStatus: types.InOutInput, NPeriods: 360,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 599.55,
			LastOK: true,
		},
		Settings: ds(),
	}
	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	// Principal balance should generally decrease (may have slight bumps from rounding)
	maxIncrease := 0.0
	for i := 1; i < len(result.Schedule); i++ {
		diff := result.Schedule[i].Principal - result.Schedule[i-1].Principal
		if diff > maxIncrease {
			maxIncrease = diff
		}
	}
	// Allow tiny rounding increases but nothing significant
	if maxIncrease > 1.0 {
		t.Errorf("principal increased by %f at some point", maxIncrease)
	}
}

func TestSchedulePaymentConstant(t *testing.T) {
	// For a standard fixed-rate loan, all payments should be the same
	// (except possibly the last)
	input := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanDateStatus: types.InOutInput, LoanDate: nd(2024, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.06,
			FirstStatus: types.InOutInput, FirstDate: nd(2024, time.February, 1),
			NStatus: types.InOutInput, NPeriods: 360,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 599.55,
			LastOK: true,
		},
		Settings: ds(),
	}
	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	// All non-last payments should be equal (within rounding)
	for i := 0; i < len(result.Schedule)-1; i++ {
		if math.Abs(result.Schedule[i].PayAmt-599.55) > 0.02 {
			t.Errorf("payment %d = %f, want 599.55", i+1, result.Schedule[i].PayAmt)
			break
		}
	}
}

// --- Basis365 ---

func TestAmortizeBasis365(t *testing.T) {
	s := ds()
	s.Basis = types.Basis365
	s.YrDays = 365.25
	s.YrInv = 1.0 / 365.25

	input := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanDateStatus: types.InOutInput, LoanDate: nd(2024, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.06,
			FirstStatus: types.InOutInput, FirstDate: nd(2024, time.February, 1),
			NStatus: types.InOutInput, NPeriods: 12,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 8607,
			LastOK: true,
		},
		Settings: s,
	}
	result := Amortize(input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.Schedule) != 12 {
		t.Errorf("schedule length = %d, want 12", len(result.Schedule))
	}
	// Interest with 365-day basis may differ slightly from 360
	if result.TotalInt < 2000 || result.TotalInt > 5000 {
		t.Errorf("total interest (365 basis) = %f, out of expected range", result.TotalInt)
	}
}
