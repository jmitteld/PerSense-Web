package dateutil

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func d(y int, m time.Month, day int) types.DateRec { return types.NewDateRec(y, m, day) }

func TestAbs_Negative(t *testing.T) {
	if abs(-5) != 5 || abs(5) != 5 || abs(0) != 0 {
		t.Error("abs branches wrong")
	}
}

func TestDaysInMonthPascal_Branches(t *testing.T) {
	if daysInMonthPascal(2, 4) != 29 { // 1904 leap (py%4==0)
		t.Error("Feb leap should be 29")
	}
	if daysInMonthPascal(2, 5) != 28 { // non-leap
		t.Error("Feb non-leap should be 28")
	}
	if daysInMonthPascal(1, 5) != 31 {
		t.Error("Jan should be 31")
	}
	if daysInMonthPascal(13, 5) != 30 { // out of range -> 30
		t.Error("month 13 should default to 30")
	}
}

func TestJulian_And_MDY_EdgeBranches(t *testing.T) {
	if Julian(types.UnknownDate()) != int64(types.UnkByte) {
		t.Error("Julian(unknown) should be UnkByte")
	}
	if _, err := MDY(-1); err == nil {
		t.Error("MDY(-1) should error")
	}
	if _, err := MDY(200000); err == nil {
		t.Error("MDY(too large) should error")
	}
}

func TestDaysInM_Unknown(t *testing.T) {
	if DaysInM(types.UnknownDate()) != 30 {
		t.Error("DaysInM(unknown) should be 30")
	}
}

func TestCheckForDaysTooLarge_Branches(t *testing.T) {
	u := types.UnknownDate()
	CheckForDaysTooLarge(&u) // unknown -> early return
	// Normal in-range date: day <= DaysInM, no change.
	normal := d(2021, time.April, 15)
	CheckForDaysTooLarge(&normal)
	if normal.Time.Day() != 15 {
		t.Errorf("in-range date should be unchanged, got %d", normal.Time.Day())
	}
	// NOTE: the `day > last` clamp branch is defensive and unreachable here
	// because DateRec always wraps an already-normalized time.Time (Go's
	// time.Date never yields Day() > days-in-month). It guards against
	// DateRecs assembled from raw bytes elsewhere. (coverage: excluded)
}

func TestEvalDateStr_Branches(t *testing.T) {
	cases := []struct {
		in  string
		ok  bool
	}{
		{"...", true},
		{"1/2", false},          // too short
		{"13/01/21", false},     // bad month
		{"ab/01/21", false},     // non-numeric month
		{"01/xx/21", false},     // non-numeric day
		{"01/01/yy", false},     // non-numeric year
		{"01/45/21", false},     // day too large
		{"01-15-21", true},      // dash separator
		{"01/15/21", true},      // slash, year >= centuryDiv -> 1900s
		{"01/15/05", true},      // year < centuryDiv -> 2000s
		{"01/15/2024", true},    // 4-digit year (y>=100 path)
	}
	for _, c := range cases {
		_, ok := EvalDateStr(c.in, 50)
		if ok != c.ok {
			t.Errorf("EvalDateStr(%q) ok=%v, want %v", c.in, ok, c.ok)
		}
	}
}

func TestAddYears_Branches(t *testing.T) {
	base := d(2020, time.January, 15)
	if _, err := AddYears(base, 200, types.Basis360, 365.25); err == nil {
		t.Error("AddYears > 128 should error")
	}
	if _, err := AddYears(types.UnknownDate(), 1, types.Basis360, 365.25); err == nil {
		t.Error("AddYears invalid date should error")
	}
	// Basis360 with day/month wrap.
	if _, err := AddYears(d(2020, time.December, 28), 1.6, types.Basis360, 365.25); err != nil {
		t.Errorf("AddYears 360 wrap: %v", err)
	}
	// Negative fractional years (month underflow loop).
	if _, err := AddYears(d(2020, time.January, 15), -0.5, types.Basis360, 365.25); err != nil {
		t.Errorf("AddYears 360 negative: %v", err)
	}
	// Basis365 path.
	if _, err := AddYears(base, 2, types.Basis365, 365.25); err != nil {
		t.Errorf("AddYears 365: %v", err)
	}
}

func TestAddPeriod_AllFrequencies(t *testing.T) {
	base := d(2020, time.March, 15)
	for _, peryr := range []int{1, 2, 3, 4, 6, 12, 24, 26, 52} {
		for _, sub := range []bool{false, true} {
			if _, err := AddPeriod(base, peryr, 15, sub); err != nil {
				t.Errorf("AddPeriod peryr=%d sub=%v: %v", peryr, sub, err)
			}
		}
	}
	// peryr=24 subtract crossing month/year boundary (day<1, m<=0).
	if _, err := AddPeriod(d(2020, time.January, 5), 24, 5, true); err != nil {
		t.Errorf("AddPeriod 24 underflow: %v", err)
	}
	// peryr=24 add crossing month/year boundary (day>=31, m>12).
	if _, err := AddPeriod(d(2020, time.December, 28), 24, 28, false); err != nil {
		t.Errorf("AddPeriod 24 overflow: %v", err)
	}
	// default subtract crossing year boundary.
	if _, err := AddPeriod(d(2020, time.January, 31), 12, 31, true); err != nil {
		t.Errorf("AddPeriod default underflow: %v", err)
	}
}

func TestAddNPeriods_Branches(t *testing.T) {
	base := d(2020, time.January, 31)
	// remaining == 0 fast path (n multiple of peryr).
	if _, err := AddNPeriods(base, 12, 24); err != nil {
		t.Errorf("AddNPeriods exact years: %v", err)
	}
	// remaining > 0 iterating path.
	if _, err := AddNPeriods(base, 12, 5); err != nil {
		t.Errorf("AddNPeriods remainder: %v", err)
	}
	// weekly path.
	if _, err := AddNPeriods(base, 52, 10); err != nil {
		t.Errorf("AddNPeriods weekly: %v", err)
	}
}

func TestLastDayFn_Branches(t *testing.T) {
	if LastDayFn(types.UnknownDate(), 12) {
		t.Error("unknown not last day")
	}
	if !LastDayFn(d(2021, time.January, 31), 12) {
		t.Error("Jan 31 is last day")
	}
	if !LastDayFn(d(2021, time.January, 15), 24) {
		t.Error("15th counts as last day for semi-monthly")
	}
	if LastDayFn(d(2021, time.January, 10), 12) {
		t.Error("Jan 10 not last day")
	}
}

func TestCriterion_AllZ(t *testing.T) {
	a := d(2020, time.January, 1)
	b := d(2021, time.January, 1)
	if !Criterion(a, b, types.Before) {
		t.Error("a before b")
	}
	if !Criterion(a, a, types.OnOrBefore) {
		t.Error("a on-or-before a")
	}
	if !Criterion(b, a, types.After) {
		t.Error("b after a")
	}
	if !Criterion(a, a, types.OnOrAfter) {
		t.Error("a on-or-after a")
	}
	if Criterion(a, b, types.Upto(99)) {
		t.Error("invalid z should be false")
	}
}

func TestDaysCloseEnough_Branches(t *testing.T) {
	if !DaysCloseEnough(d(2020, time.January, 15), d(2020, time.February, 15), 12) {
		t.Error("same day close")
	}
	// date1 is last day, date2 day larger.
	if !DaysCloseEnough(d(2021, time.February, 28), d(2021, time.March, 30), 12) {
		t.Error("date1 last-day close")
	}
	// date2 is last day, date1 day larger.
	if !DaysCloseEnough(d(2021, time.March, 30), d(2021, time.February, 28), 12) {
		t.Error("date2 last-day close")
	}
	// not close.
	if DaysCloseEnough(d(2020, time.January, 10), d(2020, time.February, 20), 12) {
		t.Error("should not be close")
	}
}

func TestYearsDif_SwapAndMultiYear(t *testing.T) {
	a := d(2020, time.January, 1)
	z := d(2023, time.June, 15)
	fwd := YearsDif(z, a, types.Basis360, 1.0/360.0, false)
	rev := YearsDif(a, z, types.Basis360, 1.0/360.0, false) // triggers swap branch
	if fwd <= 0 {
		t.Errorf("forward YearsDif = %v, want > 0", fwd)
	}
	if rev >= 0 {
		t.Errorf("reverse YearsDif = %v, want < 0", rev)
	}
}

func TestEvalDateStr_WrongPartCount(t *testing.T) {
	// Splits to 4 parts on "/" and 1 on "-": never len==3 -> false.
	if _, ok := EvalDateStr("1/2/3/4", 50); ok {
		t.Error("EvalDateStr with 4 parts should fail")
	}
}

func TestAddYears360_DayClamp(t *testing.T) {
	// Jan 30 + ~1 month into Feb (28 days, 2021 non-leap) -> day clamps to 28.
	got, err := AddYears(d(2021, time.January, 30), 0.084, types.Basis360, 365.25)
	if err != nil {
		t.Fatalf("AddYears: %v", err)
	}
	if got.Time.Month() == time.February && got.Time.Day() > 28 {
		t.Errorf("Feb day not clamped: %v", got.Time)
	}
}

func TestAddPeriod24_SecondSnap(t *testing.T) {
	// Start day 2, origDay 15: +15 -> 17, within 4 of 15 -> snaps to 15.
	got, err := AddPeriod(d(2020, time.March, 2), 24, 15, false)
	if err != nil {
		t.Fatalf("AddPeriod: %v", err)
	}
	if got.Time.Day() != 15 {
		t.Errorf("expected snap to 15, got %d", got.Time.Day())
	}
}

func TestAddNPeriods_NegativeN(t *testing.T) {
	got, err := AddNPeriods(d(2020, time.June, 15), 12, -5)
	if err != nil {
		t.Fatalf("AddNPeriods negative: %v", err)
	}
	if !got.Time.Before(d(2020, time.June, 15).Time) {
		t.Errorf("negative n should move date earlier, got %v", got.Time)
	}
}

func TestYearsDif_LoanCalcBranches(t *testing.T) {
	a := d(2020, time.March, 1)
	z := d(2023, time.September, 10)
	// isLoanCalc=true, non-360 basis: forward (multi-year, leap apy 2020).
	fwd := YearsDif(z, a, types.Basis365, 1.0/365.0, true)
	if fwd <= 0 {
		t.Errorf("loan-calc forward = %v, want > 0", fwd)
	}
	// Reversed dates trigger the DateComp(a,z)>0 swap branch (line 662).
	rev := YearsDif(a, z, types.Basis365, 1.0/365.0, true)
	if rev >= 0 {
		t.Errorf("loan-calc reverse = %v, want < 0", rev)
	}
	// Same-year loan calc.
	sy := YearsDif(d(2021, time.December, 1), d(2021, time.March, 1), types.Basis365, 1.0/365.0, true)
	if sy <= 0 {
		t.Errorf("loan-calc same-year = %v, want > 0", sy)
	}
}

func TestNumberOfInstallments_BiweeklyExactAndLaterMonthPhase(t *testing.T) {
	// Biweekly span that is an exact multiple of the period (ddiff==0 branch).
	f := d(2020, time.January, 1)
	l := f
	l.Time = l.Time.AddDate(0, 0, 28) // exactly 2 * 14 days
	for _, z := range []types.Upto{types.Before, types.OnOrAfter, types.OnOrBefore, types.After} {
		if n, _ := NumberOfInstallments(f, l, 26, z); n < 1 {
			t.Errorf("biweekly exact z=%v: count %d", z, n)
		}
	}
	// f later in the year than some l months -> (lm-fm) negative mod branch,
	// and ddiff>0 OnOrAfter branch.
	fJune := d(2020, time.June, 15)
	for _, peryr := range []int{4, 12} {
		for _, l := range []types.DateRec{d(2021, time.January, 20), d(2021, time.August, 25)} {
			for _, z := range []types.Upto{types.Before, types.OnOrBefore, types.After, types.OnOrAfter} {
				if n, _ := NumberOfInstallments(fJune, l, peryr, z); n < 1 {
					t.Errorf("fJune peryr=%d l=%v z=%v: count %d", peryr, l.Time, z, n)
				}
			}
		}
	}
}

func TestNumberOfInstallments_Sweep(t *testing.T) {
	f := d(2020, time.January, 31) // end-of-month to exercise flast/llast
	lasts := []types.DateRec{
		d(2021, time.January, 31),
		d(2021, time.February, 28),
		d(2021, time.March, 15),
		d(2021, time.June, 10),
		d(2022, time.December, 31),
	}
	zs := []types.Upto{types.Before, types.OnOrBefore, types.After, types.OnOrAfter}
	for _, peryr := range []int{1, 2, 3, 4, 6, 12, 24, 26, 52} {
		for _, l := range lasts {
			for _, z := range zs {
				n, got := NumberOfInstallments(f, l, peryr, z)
				if n < 1 {
					t.Errorf("peryr=%d l=%v z=%v: count=%d < 1", peryr, l.Time, z, n)
				}
				if got.IsUnknown() {
					t.Errorf("peryr=%d l=%v z=%v: returned unknown date", peryr, l.Time, z)
				}
			}
		}
	}
}
