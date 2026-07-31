package dateutil

import (
	"errors"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// --- Julian/MDY stress ---

// TestJulianMDYFullRange round-trips every Jan 1 from 1901 to 2149, and asserts
// where DOS stops being able to.
//
// This test used to require a successful round-trip for all 249 years. It
// passed only because the port had raised MDY's ceiling to Julian day 100000 —
// DOS's own limit is 70000 (VIDEODAT.pas:373), which falls in 2091, so from
// 2092 onward the DOS engine cannot convert a day number back to a date at all.
// Requiring the round-trip there was requiring the port to do something the
// original does not, and it is what let the PV Julian-ceiling defect survive:
// a perpetual biweekly stream ran to 2149 in the port and truncated in DOS.
// See internal/finance/presentvalue/zzjulian_ceiling_test.go.
func TestJulianMDYFullRange(t *testing.T) {
	sawCeiling := false
	for year := 1901; year <= 2149; year++ {
		d := types.NewDateRec(year, time.January, 1)
		j := Julian(d)
		got, err := MDY(j)
		if j > 70000 {
			if !errors.Is(err, ErrJulianCeiling) {
				t.Fatalf("year %d (Julian %d): MDY error = %v, want ErrJulianCeiling", year, j, err)
			}
			sawCeiling = true
			continue
		}
		if err != nil {
			t.Fatalf("year %d: MDY error: %v", year, err)
		}
		if got.Time.Year() != year || got.Time.Month() != time.January || got.Time.Day() != 1 {
			t.Fatalf("year %d: round trip = %v", year, got.Time)
		}
	}
	if !sawCeiling {
		t.Fatal("the loop never crossed the ceiling — the range no longer covers 2092+")
	}
}

// TestJulianMDYCeiling pins the boundary itself, in day numbers rather than
// years, so a change to the constant cannot pass silently.
func TestJulianMDYCeiling(t *testing.T) {
	if _, err := MDY(70000); err != nil {
		t.Fatalf("MDY(70000) must succeed (DOS accepts <= 70000): %v", err)
	}
	if _, err := MDY(70001); !errors.Is(err, ErrJulianCeiling) {
		t.Fatalf("MDY(70001) = %v, want ErrJulianCeiling", err)
	}
	last := LastRepresentableDate()
	if last.Time.Year() != 2091 || last.Time.Month() != time.August || last.Time.Day() != 26 {
		t.Errorf("LastRepresentableDate = %v, want 26 Aug 2091", last.Time)
	}
}

func TestJulianLeapYears(t *testing.T) {
	// Verify Feb 29 exists in leap years and round-trips. 2096 is PAST DOS's
	// MDY ceiling (Julian 71648 > 70000), so it must refuse rather than
	// round-trip — same reason as TestJulianMDYFullRange above.
	leapYears := []int{1904, 1952, 2000, 2004, 2024}
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
	beyond := types.NewDateRec(2096, time.February, 29)
	if _, err := MDY(Julian(beyond)); !errors.Is(err, ErrJulianCeiling) {
		t.Errorf("29 Feb 2096 is past the ceiling; MDY error = %v, want ErrJulianCeiling", err)
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
		{"12/31/99", true}, // Dec 31, 1999
		{"1/1/00", true},   // Jan 1, 2000
		{"2/29/00", true},  // Feb 29, 2000 (leap year)
		{"2/29/01", false}, // Feb 29, 2001 (NOT leap year)
		{"0/1/24", false},  // invalid month 0
		{"1/0/24", false},  // invalid day 0
		{"1/32/24", false}, // invalid day 32
		{"...", true},      // latest date sentinel
		{"12", false},      // too short
		{"a/b/c", false},   // non-numeric
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
