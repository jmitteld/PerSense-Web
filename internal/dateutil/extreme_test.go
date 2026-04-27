package dateutil

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// --- Julian/MDY stress ---

func TestJulianMDYFullRange(t *testing.T) {
	// Test every Jan 1 from 1901 to 2149 round-trips correctly
	for year := 1901; year <= 2149; year++ {
		d := types.NewDateRec(year, time.January, 1)
		j := Julian(d)
		got, err := MDY(j)
		if err != nil {
			t.Fatalf("year %d: MDY error: %v", year, err)
		}
		if got.Time.Year() != year || got.Time.Month() != time.January || got.Time.Day() != 1 {
			t.Fatalf("year %d: round trip = %v", year, got.Time)
		}
	}
}

func TestJulianLeapYears(t *testing.T) {
	// Verify Feb 29 exists in leap years and round-trips
	leapYears := []int{1904, 1952, 2000, 2004, 2024, 2096}
	for _, year := range leapYears {
		d := types.NewDateRec(year, time.February, 29)
		j := Julian(d)
		got, err := MDY(j)
		if err != nil {
			t.Fatalf("leap year %d: %v", year, err)
		}
		if got.Time.Month() != time.February || got.Time.Day() != 29 {
			t.Errorf("leap year %d: Feb 29 round trip = %v", year, got.Time)
		}
	}
}

func TestJulianNonLeapFeb28(t *testing.T) {
	// Non-leap years: Feb 28 → Mar 1 is consecutive.
	// Note: 2100 is excluded because the Pascal leap year check (y%4==0 && y>0)
	// incorrectly treats 2100 as a leap year. This is a known limitation
	// of the simplified 4-year rule used in the original code.
	nonLeap := []int{1901, 1999, 2001, 2023}
	for _, year := range nonLeap {
		feb28 := Julian(types.NewDateRec(year, time.February, 28))
		mar1 := Julian(types.NewDateRec(year, time.March, 1))
		if mar1-feb28 != 1 {
			t.Errorf("year %d: Mar1 - Feb28 = %d, want 1", year, mar1-feb28)
		}
	}
}

// --- AddPeriod stress ---

func TestAddPeriod12MonthsEqualsYear(t *testing.T) {
	// Adding 12 monthly periods should land on the same day next year
	base := types.NewDateRec(2024, time.March, 15)
	d := base
	var err error
	for i := 0; i < 12; i++ {
		d, err = AddPeriod(d, 12, 15, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	if d.Time.Year() != 2025 || d.Time.Month() != time.March || d.Time.Day() != 15 {
		t.Errorf("12 monthly periods = %v, want 2025-03-15", d.Time)
	}
}

func TestAddPeriodEndOfMonth31st(t *testing.T) {
	// Starting Jan 31, monthly periods should snap to end of shorter months
	d := types.NewDateRec(2024, time.January, 31)
	months := make([]time.Month, 0)
	var err error
	for i := 0; i < 12; i++ {
		d, err = AddPeriod(d, 12, 31, false)
		if err != nil {
			t.Fatal(err)
		}
		months = append(months, d.Time.Month())
		// Day should never exceed month's max
		dim := DaysInM(d)
		if d.Time.Day() > dim {
			t.Errorf("month %d: day %d > max %d", d.Time.Month(), d.Time.Day(), dim)
		}
	}
}

func TestAddNPeriodsLargeN(t *testing.T) {
	// 360 monthly periods = 30 years
	base := types.NewDateRec(2000, time.January, 15)
	d, err := AddNPeriods(base, 12, 360)
	if err != nil {
		t.Fatal(err)
	}
	if d.Time.Year() != 2030 || d.Time.Month() != time.January || d.Time.Day() != 15 {
		t.Errorf("360 monthly periods = %v, want 2030-01-15", d.Time)
	}
}

// --- YearsDif consistency ---

func TestYearsDifSymmetry(t *testing.T) {
	a := types.NewDateRec(2024, time.January, 1)
	z := types.NewDateRec(2030, time.June, 15)

	for _, basis := range []types.BasisType{types.Basis360, types.Basis365, types.Basis365360} {
		pos := YearsDif(z, a, basis, 1.0/365.25, false)
		neg := YearsDif(a, z, basis, 1.0/365.25, false)
		if pos+neg > 0.001 || pos+neg < -0.001 {
			t.Errorf("basis %v: YearsDif(%v,%v) + YearsDif(%v,%v) = %f, want ~0",
				basis, z.Time, a.Time, a.Time, z.Time, pos+neg)
		}
	}
}

func TestYearsDifOneYear(t *testing.T) {
	a := types.NewDateRec(2024, time.January, 1)
	z := types.NewDateRec(2025, time.January, 1)

	// All bases should give ~1.0 for exactly one year
	for _, basis := range []types.BasisType{types.Basis360, types.Basis365} {
		diff := YearsDif(z, a, basis, 1.0/365.25, false)
		if diff < 0.99 || diff > 1.01 {
			t.Errorf("basis %v: 1 year = %f", basis, diff)
		}
	}
}

// --- EvalDateStr edge cases ---

func TestEvalDateStrEdgeCases(t *testing.T) {
	tests := []struct {
		input  string
		wantOK bool
	}{
		{"12/31/99", true},   // Dec 31, 1999
		{"1/1/00", true},     // Jan 1, 2000
		{"2/29/00", true},    // Feb 29, 2000 (leap year)
		{"2/29/01", false},   // Feb 29, 2001 (NOT leap year)
		{"0/1/24", false},    // invalid month 0
		{"1/0/24", false},    // invalid day 0
		{"1/32/24", false},   // invalid day 32
		{"...", true},        // latest date sentinel
		{"12", false},        // too short
		{"a/b/c", false},     // non-numeric
	}
	for _, tt := range tests {
		_, ok := EvalDateStr(tt.input, DefaultCenturyDiv)
		if ok != tt.wantOK {
			t.Errorf("EvalDateStr(%q) = %v, want %v", tt.input, ok, tt.wantOK)
		}
	}
}

// --- ExtendedJulian basis comparison ---

func TestExtendedJulianBasisDifference(t *testing.T) {
	d := types.NewDateRec(2024, time.June, 15)

	ej360 := ExtendedJulian(d, types.Basis360)
	ej365 := ExtendedJulian(d, types.Basis365)

	// 360-basis uses synthetic calendar, 365 uses Julian
	// They should differ significantly
	if ej360 == ej365 {
		t.Error("ExtendedJulian should differ between 360 and 365 bases")
	}
}
