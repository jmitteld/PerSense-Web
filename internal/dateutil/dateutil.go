// Package dateutil provides date manipulation functions faithfully ported from
// the legacy Delphi/Pascal VIDEODAT.pas and INTSUTIL.pas modules.
//
// The original Pascal code used a custom daterec type where year was stored as
// a byte (0-249 representing 1900-2149). This Go port uses time.Time internally
// but preserves the exact Julian day number calculations and date arithmetic
// behavior of the original to ensure financial calculation fidelity.
//
// Ported from legacy/source/VIDEODAT.pas and legacy/source/INTSUTIL.pas
package dateutil

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// daysInMonth maps month (1-12) to number of days in a non-leap year.
// Index 0 and 13 are sentinels matching the Pascal daysin[0..13] array.
// Ported from legacy/source/VIDEODAT.pas: daysin:array[0..13] of byte
var daysInMonth = [14]int{31, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31, 31}

// MonthAbbr maps month (1-12) to 3-letter abbreviation.
// Ported from legacy/source/VIDEODAT.pas: mon array
var MonthAbbr = [13]string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// MonthNames maps month (0-12) to full name. Index 0 = December (wraps).
// Ported from legacy/source/VIDEODAT.pas: monstr array
var MonthNames = [13]string{"December", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

// ErrorByte is the sentinel value for invalid month in a date.
// Ported from legacy/source/VIDEODAT.pas: errorbyte=-99
const ErrorByte int = -99

// FourYears is the number of days in 4 years including a leap year (1461).
// Ported from legacy/source/VIDEODAT.pas
const FourYears int64 = 1461

// DefaultCenturyDiv is the default century divisor for 2-digit year parsing.
// Years < CenturyDiv are treated as 2000+; years >= CenturyDiv as 1900+.
// Ported from legacy/source/VIDEODAT.pas: centurydiv:byte=50
const DefaultCenturyDiv = 50

// --- Internal Pascal-compatible date representation ---
// The Pascal daterec stored dates as (d: shortint, m: shortint, y: byte)
// where y = calendar_year - 1900. So y=0 means 1900, y=100 means 2000,
// y=249 means 2149 (the maximum). This matches the earliest/latest constants
// and the EvalDateStr century-div logic.
//
// Note: The modernized Delphi SetNow uses "YearOf(CurrentDate) - 1950" which
// appears to be a bug in the modernization — the Julian/MDY formulas and
// leap year detection are designed around base 1900.
//
// The leap year check "(y mod 4 = 0) and (y > 0)" correctly identifies leap
// years for 1901-2099 (where the 4-year rule suffices). y=0 (1900) is
// correctly excluded since 1900 is not a leap year.

// pascalYear converts a calendar year to the Pascal internal y value.
func pascalYear(calendarYear int) int {
	return calendarYear - 1900
}

// calendarYear converts a Pascal y value to a calendar year.
func calendarYear(py int) int {
	return py + 1900
}

// isLeapYearPascal checks leap year using the Pascal convention.
// Ported from legacy/source/VIDEODAT.pas: (wy mod 4 = 0) and (wy>0)
func isLeapYearPascal(py int) bool {
	return (py%4 == 0) && (py > 0)
}

// daysInMonthPascal returns days in month for a Pascal-year date.
// Ported from legacy/source/VIDEODAT.pas: DaysInM function
// Note: DaysInM checks (y mod 4 = 0) without the (y > 0) guard,
// so y=0 (1900) would return 29 for Feb — a minor inaccuracy since
// 1900 is not a leap year. We preserve this behavior exactly.
func daysInMonthPascal(m, py int) int {
	if m == 2 {
		if py%4 == 0 {
			return 29
		}
		return 28
	}
	if m >= 1 && m <= 12 {
		return daysInMonth[m]
	}
	return 30 // avoiding range check errors, per original
}

// daysBefore computes cumulative days before each month for the given Pascal year.
// Matches the Pascal initialization block which builds notleapdaysbefore and
// leapdaysbefore arrays. In the leap case, months 3+ get +1 compared to non-leap.
//
// Ported from legacy/source/VIDEODAT.pas: initialization block
func daysBefore(py int) [14]int {
	var db [14]int
	// First build non-leap version
	db[1] = 0
	for i := 2; i <= 12; i++ {
		db[i] = db[i-1] + daysInMonth[i-1]
	}
	// In leap years, add 1 to months March (3) through December (12)
	// Matches: for i:=3 to 12 do leapdaysbefore[i]:=succ(notleapdaysbefore[i])
	if isLeapYearPascal(py) {
		for i := 3; i <= 12; i++ {
			db[i]++
		}
	}
	db[13] = math.MaxUint16 // sentinel
	return db
}

// Julian computes the Julian day number for a date.
// This is the core date-to-number conversion used throughout the application.
//
// Ported from legacy/source/VIDEODAT.pas:
//
//	function Julian(x:daterec):longint;
//	  daynumber:=(fouryears * longint(y)-1) div 4 + daysbefore^[m] + d;
func Julian(d types.DateRec) int64 {
	if d.IsUnknown() {
		return int64(types.UnkByte)
	}
	py := pascalYear(d.Time.Year())
	m := int(d.Time.Month())
	day := d.Time.Day()

	if m > 13 || m < 1 {
		return int64(types.UnkByte)
	}

	db := daysBefore(py)
	return (FourYears*int64(py) - 1) / 4 + int64(db[m]) + int64(day)
}

// MDY converts a Julian day number back to a DateRec.
// This is the inverse of Julian().
//
// Ported from legacy/source/VIDEODAT.pas:
//
//	procedure MDY(daynumber:longint; var x:daterec);
func MDY(daynumber int64) (types.DateRec, error) {
	// Original Pascal limit was 70000, but with base 1900 (y up to 249 = year 2149),
	// Julian values can reach ~91000. We use 100000 for safety.
	if daynumber < 0 || daynumber > 100000 {
		return types.DateRec{}, fmt.Errorf("day number %d out of range [0, 100000]", daynumber)
	}

	fourx := daynumber * 4 // daynumber shl 2
	py := int(fourx / FourYears)

	db := daysBefore(py)
	days := int((fourx-int64(py)*FourYears)/4) + 1 // succ(...shr 2)

	// Binary search for month, matching original's sequential scan
	var m int
	if days <= db[7] {
		if days <= db[4] {
			m = 1
		} else {
			m = 4
		}
	} else {
		if days <= db[10] {
			m = 7
		} else {
			m = 10
		}
	}
	for m+1 <= 13 && db[m+1] < days {
		m++
	}
	day := days - db[m]

	year := calendarYear(py)
	return types.NewDateRec(year, time.Month(m), day), nil
}

// DaysInM returns the number of days in the month for the given date.
// Ported from legacy/source/VIDEODAT.pas: function DaysInM
func DaysInM(d types.DateRec) int {
	if d.IsUnknown() {
		return 30
	}
	m := int(d.Time.Month())
	y := d.Time.Year()
	py := pascalYear(y)
	return daysInMonthPascal(m, py)
}

// CheckForDaysTooLarge adjusts the day if it exceeds the month's maximum.
// Ported from legacy/source/VIDEODAT.pas: procedure CheckForDaysTooLarge
func CheckForDaysTooLarge(d *types.DateRec) {
	if d.IsUnknown() {
		return
	}
	last := DaysInM(*d)
	if d.Time.Day() > last {
		*d = types.NewDateRec(d.Time.Year(), d.Time.Month(), last)
	}
}

// DateOK returns true if the date has a valid month (1-12).
// Invalid/unknown dates have month values outside this range.
// Ported from legacy/source/VIDEODAT.pas: function dateok
func DateOK(d types.DateRec) bool {
	if d.IsUnknown() {
		return false
	}
	m := int(d.Time.Month())
	return m > 0 && m < 13
}

// DateStr formats a date as "MM/DD/YY".
// Ported from legacy/source/VIDEODAT.pas: function DateStr
func DateStr(d types.DateRec) string {
	if d.IsUnknown() {
		return "  ....  "
	}
	latest := types.LatestDate()
	if d.Time.Equal(latest.Time) {
		return "  ....  "
	}
	py := pascalYear(d.Time.Year())
	yy := py % 100
	return fmt.Sprintf("%2d/%2d/%02d", d.Time.Month(), d.Time.Day(), yy)
}

// Date6 returns a 6-character date string in YYMMDD format.
// Ported from legacy/source/VIDEODAT.pas: function Date6
func Date6(d types.DateRec) string {
	py := pascalYear(d.Time.Year())
	yy := py % 100
	return fmt.Sprintf("%02d%02d%02d", yy, d.Time.Month(), d.Time.Day())
}

// DateComp compares two dates.
// Returns +1 if d1 is later than d2, -1 if earlier, 0 if same.
// Unknown/blank dates are treated as later than everything.
//
// Ported from legacy/source/INTSUTIL.pas: function DateComp
func DateComp(d1, d2 types.DateRec) int {
	ok1 := DateOK(d1)
	ok2 := DateOK(d2)

	if ok1 {
		if !ok2 {
			return -1
		}
		// Both valid — compare by (y, m, d) matching original's longint overlay
		if d1.Time.After(d2.Time) {
			return 1
		}
		if d1.Time.Before(d2.Time) {
			return -1
		}
		return 0
	}
	if ok2 {
		return 1
	}
	return 0
}

// EvalDateStr parses a date string in "M/D/YY" or "MM/DD/YY" format.
// Returns the parsed date and true on success, or an unknown date and false on failure.
// The centuryDiv parameter controls 2-digit year interpretation:
// years < centuryDiv → 2000s, years >= centuryDiv → 1900s.
//
// Ported from legacy/source/VIDEODAT.pas: function EvalDateStr
func EvalDateStr(datestr string, centuryDiv int) (types.DateRec, bool) {
	datestr = strings.TrimSpace(datestr)

	// "..." means latest date
	if strings.Contains(datestr, "...") {
		return types.LatestDate(), true
	}

	if len(datestr) < 5 {
		return types.UnknownDate(), false
	}

	// Split on / or -
	var parts []string
	for _, sep := range []string{"/", "-"} {
		parts = strings.Split(datestr, sep)
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) != 3 {
		return types.UnknownDate(), false
	}

	m, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || m <= 0 || m > 12 {
		return types.UnknownDate(), false
	}

	d, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return types.UnknownDate(), false
	}

	y, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return types.UnknownDate(), false
	}

	// Century conversion: matches Pascal logic exactly.
	// In Pascal, y is stored as calendar_year - 1900.
	// EvalDateStr parses a 2-digit year, then: if (y<centurydiv) then y:=y+100
	// This makes y range from centuryDiv..99 (1900s) and 100..centuryDiv+99 (2000s).
	// We convert to calendar year here.
	if y < 100 {
		if y < centuryDiv {
			// y → y+100 in Pascal internal format → calendar year = y+100+1900 = y+2000
			y = y + 2000
		} else {
			// y stays as-is in Pascal internal format → calendar year = y+1900
			y = y + 1900
		}
	}

	py := pascalYear(y)
	dim := daysInMonthPascal(m, py)
	if d <= 0 || d > dim {
		return types.UnknownDate(), false
	}

	return types.NewDateRec(y, time.Month(m), d), true
}

// SetNow returns a DateRec for the current date.
// Ported from legacy/source/VIDEODAT.pas: procedure SetNow
func SetNow() types.DateRec {
	now := time.Now()
	return types.NewDateRec(now.Year(), now.Month(), now.Day())
}

// --- Date arithmetic functions from INTSUTIL.pas ---

// AddDays adds (or subtracts) a number of days to a date.
// Ported from legacy/source/INTSUTIL.pas: procedure AddDays
func AddDays(d types.DateRec, days int64) (types.DateRec, error) {
	j := Julian(d) + days
	return MDY(j)
}

// Floor returns the floor of a float64, matching Pascal's trunc-based floor.
// Ported from legacy/source/INTSUTIL.pas: function floor
func Floor(x float64) int64 {
	if x > 0 {
		return int64(x)
	}
	tr := int64(x)
	if float64(tr) == x {
		return tr
	}
	return tr - 1
}

// AddYears adds a fractional number of years to a date.
// Behavior depends on the day-count basis:
//   - Basis360: adds years/months/days using 30/360 convention
//   - Basis365/365_360: adds days via Julian day number
//
// yrdays is the number of days per year used for non-360 calculations
// (typically 365.25 for PVL/CHR screens, or context-dependent).
//
// Ported from legacy/source/INTSUTIL.pas: procedure AddYears
func AddYears(d types.DateRec, yrs float64, basis types.BasisType, yrdays float64) (types.DateRec, error) {
	if math.Abs(yrs) > 128 {
		return types.DateRec{}, fmt.Errorf("time period too long: %f years", yrs)
	}
	if !DateOK(d) {
		return types.DateRec{}, fmt.Errorf("invalid date")
	}

	if basis == types.Basis360 {
		py := pascalYear(d.Time.Year())
		m := int(d.Time.Month())
		day := d.Time.Day()

		years := int(Floor(yrs))
		yrs = yrs - float64(years)
		months := int(yrs * 12)
		days := int(math.Round(360 * (yrs - float64(months)/12)))

		py = py + years
		m = m + months
		day = day + days
		if day > 30 {
			day = day - 30
			m++
		}
		for m > 12 {
			m = m - 12
			py++
		}
		for m < 1 || m > 240 {
			m = m + 12
			py--
		}

		// Clamp day before creating DateRec to prevent Go auto-normalization
		dim := daysInMonthPascal(m, py)
		if day > dim {
			day = dim
		}
		return types.NewDateRec(calendarYear(py), time.Month(m), day), nil
	}

	// Basis365 or Basis365_360
	j := Julian(d) + int64(math.Round(yrs*yrdays))
	return MDY(j)
}

// AddPeriod advances a date by one payment period.
// The behavior depends on peryr (payments per year):
//   - 26/52: adds 14/7 days respectively via Julian arithmetic
//   - 24: semi-monthly, adds/subtracts 15 days with month wrapping
//   - 1,2,3,4,6,12: adds 12/peryr months, preserving orig_day
//
// If subtract is true, the period is subtracted instead of added.
//
// Ported from legacy/source/INTSUTIL.pas: procedure AddPeriod
func AddPeriod(d types.DateRec, peryr int, origDay int, subtract bool) (types.DateRec, error) {
	py := pascalYear(d.Time.Year())
	m := int(d.Time.Month())
	day := d.Time.Day()

	switch peryr {
	case 26, 52:
		t := Julian(d)
		step := int64(364 / peryr)
		if subtract {
			t -= step
		} else {
			t += step
		}
		return MDY(t)

	case 24:
		// Semi-monthly: add/subtract 15 days with snapping
		if abs(day-origDay) < 4 {
			day = origDay
		}
		if subtract {
			day -= 15
			if day < 1 {
				m--
				day += 30
				if m <= 0 {
					py--
					m += 12
				}
			}
		} else {
			day += 15
			if day >= 31 {
				m++
				day -= 30
				if m > 12 {
					py++
					m -= 12
				}
			}
		}
		if abs(day-origDay) < 4 {
			day = origDay
		}
		// Clamp day before creating DateRec
		dim := daysInMonthPascal(m, py)
		if day > dim {
			day = dim
		}
		return types.NewDateRec(calendarYear(py), time.Month(m), day), nil

	default:
		// peryr = 1, 2, 3, 4, 6, 12
		day = origDay
		monthStep := 12 / peryr
		if subtract {
			m -= monthStep
		} else {
			m += monthStep
		}
		if m < 1 || m > 240 {
			m += 12
			py--
		} else if m > 12 {
			m -= 12
			py++
		}
		// Clamp day to valid range BEFORE creating DateRec to prevent
		// Go's time.Date from auto-normalizing (e.g. Feb 31 → Mar 2)
		dim := daysInMonthPascal(m, py)
		if day > dim {
			day = dim
		}
		return types.NewDateRec(calendarYear(py), time.Month(m), day), nil
	}
}

// AddNPeriods advances a date by n payment periods.
// For monthly-type frequencies (1,2,3,4,6,12,24), it optimizes by first
// adding whole years, then iterating remaining periods.
// For weekly-type (26,52), it uses direct day arithmetic.
//
// Ported from legacy/source/INTSUTIL.pas: procedure AddNPeriods
func AddNPeriods(firstDate types.DateRec, peryr int, n int) (types.DateRec, error) {
	switch peryr {
	case 1, 2, 3, 4, 6, 12, 24:
		py := pascalYear(firstDate.Time.Year())
		m := int(firstDate.Time.Month())
		day := firstDate.Time.Day()

		nyears := n / peryr
		if n%peryr < 0 {
			nyears--
		}
		lastPY := py + nyears
		lastDate := types.NewDateRec(calendarYear(lastPY), time.Month(m), day)

		remaining := n - peryr*nyears
		if remaining == 0 {
			CheckForDaysTooLarge(&lastDate)
			return lastDate, nil
		}
		var err error
		for i := 0; i < remaining; i++ {
			lastDate, err = AddPeriod(lastDate, peryr, firstDate.Time.Day(), false)
			if err != nil {
				return types.DateRec{}, err
			}
		}
		return lastDate, nil

	default: // 26, 52
		daysPerPeriod := int64(365 / peryr)
		ndays := int64(n) * daysPerPeriod
		return MDY(ndays + Julian(firstDate))
	}
}

// LastDayFn returns true if the date falls on the last day of its month,
// or for semi-monthly (peryr=24), if the day is the 15th.
//
// Ported from legacy/source/INTSUTIL.pas: function LastDayFn
func LastDayFn(d types.DateRec, peryr int) bool {
	if d.IsUnknown() {
		return false
	}
	dim := DaysInM(d)
	if d.Time.Day() == dim {
		return true
	}
	if peryr == 24 && d.Time.Day() == 15 {
		return true
	}
	return false
}

// Criterion evaluates a date comparison condition.
// Ported from legacy/source/INTSUTIL.pas: function Criterion
func Criterion(d1, d2 types.DateRec, z types.Upto) bool {
	cmp := DateComp(d1, d2)
	switch z {
	case types.Before:
		return cmp < 0
	case types.OnOrBefore:
		return cmp <= 0
	case types.After:
		return cmp > 0
	case types.OnOrAfter:
		return cmp >= 0
	}
	return false
}

// DaysCloseEnough determines whether two dates are "close enough" to count
// as an exact number of months apart (in 360-day mode), or whether days
// must be counted individually.
//
// Ported from legacy/source/INTSUTIL.pas: function DaysCloseEnough
func DaysCloseEnough(date1, date2 types.DateRec, peryr int) bool {
	if date1.Time.Day() == date2.Time.Day() {
		return true
	}
	if LastDayFn(date1, peryr) && date2.Time.Day() > date1.Time.Day() {
		return true
	}
	if LastDayFn(date2, peryr) && date1.Time.Day() > date2.Time.Day() {
		return true
	}
	return false
}

// ExtendedJulian returns a day number that accounts for the day-count basis.
// For Basis360, it uses the synthetic 360-day calendar.
// For other bases, it falls through to the standard Julian function.
//
// Ported from legacy/source/INTSUTIL.pas: function ExtendedJulian
func ExtendedJulian(d types.DateRec, basis types.BasisType) int64 {
	if basis == types.Basis360 {
		py := pascalYear(d.Time.Year())
		m := int(d.Time.Month())
		day := d.Time.Day()
		return int64(py)*360 + int64(m)*30 + int64(day)
	}
	return Julian(d)
}

// YearsDif computes the difference in years between two dates (z - a).
// The calculation depends on the day-count basis and the screen context.
//
// Parameters:
//   - z, a: the two dates (z - a = result)
//   - basis: the day-count convention in effect
//   - yrinv: 1/yrdays, precomputed inverse of days-per-year
//   - isLoanCalc: true for AMZ/RBT screens (uses 365/366 per actual year),
//     false for PVL/CHR/INV screens (uses fixed yrdays)
//
// Ported from legacy/source/INTSUTIL.pas: function YearsDif (version adopted 12/93)
func YearsDif(z, a types.DateRec, basis types.BasisType, yrinv float64, isLoanCalc bool) float64 {
	if basis == types.Basis360 {
		if DateComp(a, z) > 0 {
			return -YearsDif(a, z, basis, yrinv, isLoanCalc)
		}
		apy := pascalYear(a.Time.Year())
		zpy := pascalYear(z.Time.Year())
		am := int(a.Time.Month())
		zm := int(z.Time.Month())
		ad := a.Time.Day()
		zd := z.Time.Day()

		til := float64(zpy-apy) + float64(zm-am)/12.0 + float64(zd-ad)/360.0
		if ad == 31 && zd < 31 {
			til += 1.0 / 360.0
		} else if ad == 30 && zd == 31 {
			til -= 1.0 / 360.0
		} else if am == 2 && ad > 27 {
			til -= float64(30-ad) / 360.0
		}
		return til
	}

	// Non-360 basis
	if !isLoanCalc || basis == types.Basis365360 {
		// PVL, INV, CHR screens or 365/360 basis: simple Julian diff / yrdays
		return float64(Julian(z)-Julian(a)) * yrinv
	}

	// Loan calculations (AMZ): use 365 and 366 per actual year
	if DateComp(a, z) > 0 {
		return -YearsDif(a, z, basis, yrinv, isLoanCalc)
	}

	apy := pascalYear(a.Time.Year())
	zpy := pascalYear(z.Time.Year())

	var yrdaz float64
	if isLeapYearPascal(apy) {
		yrdaz = 366
	} else {
		yrdaz = 365
	}

	if zpy == apy {
		return float64(Julian(z)-Julian(a)) / yrdaz
	}

	// Multi-year span: recursive year-by-year calculation
	til := float64(zpy - apy - 1)
	wd := types.NewDateRec(calendarYear(apy), time.December, 31)
	til += YearsDif(wd, a, basis, yrinv, isLoanCalc) + 1.0/yrdaz
	wd2 := types.NewDateRec(calendarYear(zpy), time.January, 1)
	til += YearsDif(z, wd2, basis, yrinv, isLoanCalc)
	return til
}

// abs returns the absolute value of an int.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
