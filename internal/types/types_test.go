package types

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestBasisTypeString(t *testing.T) {
	tests := []struct {
		b    BasisType
		want string
	}{
		{Basis365, "365"},
		{Basis360, "360"},
		{Basis365360, "365/360"},
		{BasisType(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.b.String(); got != tt.want {
			t.Errorf("BasisType(%d).String() = %q, want %q", tt.b, got, tt.want)
		}
	}
}

func TestMethodTypeString(t *testing.T) {
	tests := []struct {
		m    MethodType
		want string
	}{
		{MethodContinuous, "CONTINUOUS"},
		{MethodPerdcCont, "PERDC/CONT"},
		{MethodDaily, "DAILY"},
		{MethodPeriodic, "PERIODIC"},
		{MethodPmtToPmt, "PMT-TO-PMT"},
		{MethodSkipPmt, "SKIP-PMT"},
		{MethodUSRule, "US RULE"},
		{MethodPBeforeI, "P BEFORE I"},
		{MethodType(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.m.String(); got != tt.want {
			t.Errorf("MethodType(%d).String() = %q, want %q", tt.m, got, tt.want)
		}
	}
}

func TestDateRec(t *testing.T) {
	d := NewDateRec(2024, time.March, 15)
	if d.IsUnknown() {
		t.Error("valid date should not be unknown")
	}
	if d.Time.Year() != 2024 || d.Time.Month() != time.March || d.Time.Day() != 15 {
		t.Errorf("got %v, want 2024-03-15", d.Time)
	}

	unk := UnknownDate()
	if !unk.IsUnknown() {
		t.Error("unknown date should be unknown")
	}

	earliest := EarliestDate()
	if earliest.Time.Year() != 1900 {
		t.Errorf("earliest year = %d, want 1900", earliest.Time.Year())
	}

	latest := LatestDate()
	if latest.Time.Year() != 2149 {
		t.Errorf("latest year = %d, want 2149", latest.Time.Year())
	}
}

func TestDefaultCompDefaults(t *testing.T) {
	d := DefaultCompDefaults()
	if d.COLAMonth != COLAAnnual {
		t.Errorf("COLAMonth = %d, want %d", d.COLAMonth, COLAAnnual)
	}
	if d.CenturyDiv != 50 {
		t.Errorf("CenturyDiv = %d, want 50", d.CenturyDiv)
	}
	if d.PerYr != 12 {
		t.Errorf("PerYr = %d, want 12", d.PerYr)
	}
	if d.Basis != Basis360 {
		t.Errorf("Basis = %v, want Basis360", d.Basis)
	}
	if !d.Prepaid {
		t.Error("Prepaid should default to true")
	}
	if d.R78 {
		t.Error("R78 should default to false")
	}
}

func TestDefaultAppDefaults(t *testing.T) {
	d := DefaultAppDefaults()
	if d.RMethod != MethodPerdcCont {
		t.Errorf("RMethod = %v, want MethodPerdcCont", d.RMethod)
	}
	if !d.Commas {
		t.Error("Commas should default to true")
	}
}

func TestIsValidPerYr(t *testing.T) {
	valid := []int{1, 2, 3, 4, 6, 12, 24, 26, 52}
	for _, v := range valid {
		if !IsValidPerYr(v) {
			t.Errorf("IsValidPerYr(%d) = false, want true", v)
		}
	}
	invalid := []int{0, 5, 7, 8, 13, 25, 53, 100}
	for _, v := range invalid {
		if IsValidPerYr(v) {
			t.Errorf("IsValidPerYr(%d) = true, want false", v)
		}
	}
}

func TestMortgageLineDecimal(t *testing.T) {
	// Verify that monetary fields use decimal.Decimal, not float64
	m := MortgageLine{
		PriceStatus: InOutInput,
		Price:       decimal.NewFromFloat(250000.00),
		RateStatus:  InOutInput,
		Rate:        decimal.NewFromFloat(6.5),
		Years:       30,
		YearsStatus: InOutInput,
	}
	if !m.Price.Equal(decimal.NewFromInt(250000)) {
		t.Errorf("Price = %s, want 250000", m.Price)
	}
	if m.PriceStatus != InOutInput {
		t.Errorf("PriceStatus = %d, want %d", m.PriceStatus, InOutInput)
	}
}

func TestAMZLoanDecimal(t *testing.T) {
	loan := AMZLoan{
		AmountStatus:   InOutInput,
		Amount:         decimal.NewFromFloat(100000.00),
		LoanRateStatus: InOutInput,
		LoanRate:       decimal.NewFromFloat(5.25),
		PerYr:          12,
		PerYrStatus:    InOutInput,
		NPeriods:       360,
		NStatus:        InOutInput,
	}
	if !loan.Amount.Equal(decimal.NewFromInt(100000)) {
		t.Errorf("Amount = %s, want 100000", loan.Amount)
	}
	if loan.PerYr != 12 {
		t.Errorf("PerYr = %d, want 12", loan.PerYr)
	}
}

func TestScreenDataInit(t *testing.T) {
	sd := NewScreenData()
	if sd == nil {
		t.Fatal("NewScreenData returned nil")
	}
	// All pointers should be nil initially
	if sd.AMZ != nil {
		t.Error("AMZ should be nil initially")
	}
	if sd.LumpSums[0] != nil {
		t.Error("LumpSums[0] should be nil initially")
	}
	// NLines should be zero
	for i := range sd.NLines {
		if sd.NLines[i] != 0 {
			t.Errorf("NLines[%d] = %d, want 0", i, sd.NLines[i])
		}
	}
}

func TestConstants(t *testing.T) {
	// Verify key constants match Pascal originals
	if MaxLines != 127 {
		t.Errorf("MaxLines = %d, want 127", MaxLines)
	}
	if NBlocks != 15 {
		t.Errorf("NBlocks = %d, want 15", NBlocks)
	}
	if NCols != 79 {
		t.Errorf("NCols = %d, want 79", NCols)
	}
	if ErrorVal != -8888 {
		t.Errorf("ErrorVal = %d, want -8888", ErrorVal)
	}
	if Blank != -7777 {
		t.Errorf("Blank = %d, want -7777", Blank)
	}
}

func TestColumnIdentifiers(t *testing.T) {
	// Verify column IDs match Pascal originals exactly
	// These are critical: wrong column IDs would break all data access
	if ColDate != 1 {
		t.Errorf("ColDate = %d, want 1", ColDate)
	}
	if ColPrice != 20 {
		t.Errorf("ColPrice = %d, want 20", ColPrice)
	}
	if ColAAmount != 50 {
		t.Errorf("ColAAmount = %d, want 50", ColAAmount)
	}
	if ColAAPR != 59 {
		t.Errorf("ColAAPR = %d, want 59", ColAAPR)
	}
	if ColSkipMonth != 79 {
		t.Errorf("ColSkipMonth = %d, want 79", ColSkipMonth)
	}
	// Verify sequential amortization columns
	if ColLoanDate != ColAAmount+1 {
		t.Error("ColLoanDate should be ColAAmount+1")
	}
	if ColARate != ColAAmount+2 {
		t.Error("ColARate should be ColAAmount+2")
	}
}
