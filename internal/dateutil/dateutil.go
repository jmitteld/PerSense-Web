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
	"errors"
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

// wrapPascalYear truncates a Pascal year to the storage width DOS actually has.
//
// `daterec` is `d,m: shortint; y: byte` (Globals.pas:46-48, VIDEODAT.pas:10), so
// EVERY assignment to the year field in the DOS sources is an assignment to a
// BYTE, compiled with range checking off. `lastdate.y := firstdate.y + nyears`
// (INTSUTIL.pas:1402) evaluates in integer and then truncates mod 256; `inc(y)`
// at y=255 rolls to 0 and `dec(y)` at y=0 rolls to 255. The port stores a
// time.Time whose year is unbounded, so without this every long-horizon date
// arithmetic silently disagrees with the engine it is supposed to reproduce.
//
// This is not a cosmetic difference. Verified against the real DOS engine:
//
//	amort_oracle intutil addn 2023 7 29 1 299   -> last 2066 7 29  (123+299 = 422 -> 166)
//	amort_oracle intutil addn 2023 7 29 1 133   -> last 1900 7 29  (123+133 = 256 -> 0)
//	amort_oracle intutil addn 2023 7 29 1 132   -> last 2155 7 29  (123+132 = 255, no wrap)
//	amort_oracle intutil addn 2150 6 15 12 67   -> last 1900 1 15  (AddPeriod inc(y): 255 -> 0)
//	amort_oracle intutil addn 1901 6 15 12 -24  -> last 2155 6 15  (AddPeriod dec(y): 0 -> 255)
//
// The wrapped year is what the ENGINE then computes with — a loan whose nominal
// terminal is 2322 is amortized by DOS against a very_last of 2066, and
// NumberOfInstallments counts `12*(l.y-f.y)` off the wrapped byte. See
// docs/discrepancies.md §55.
//
// Because the result is always in [0, 255] the calendar year is always in
// [1900, 2155] and therefore always representable by types.DateRec — unlike
// §51/§54 this rule needs no raw y/m/d fields.
func wrapPascalYear(py int) int {
	return ((py % 256) + 256) % 256
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

	// Defensive: a DateRec wraps a normalized time.Time, so Month() is always
	// 1..12 here; this guards malformed inputs. (coverage: excluded)
	if m > 13 || m < 1 {
		return int64(types.UnkByte)
	}

	db := daysBefore(py)
	return (FourYears*int64(py)-1)/4 + int64(db[m]) + int64(day)
}

// julianCeiling is DOS's hard upper bound on a Julian day number
// (VIDEODAT.pas:373). Day 70000 is 26 August 2091.
const julianCeiling = 70000

// ErrJulianCeiling reports that a Julian day number fell outside the range DOS's
// MDY accepts, [0, julianCeiling]. DOS signals this by writing errorbyte (-99)
// into the daterec's month field and returning; the port returns this sentinel
// alongside the unknown DateRec that stands in for that poisoned record.
//
// Two consumer behaviours are correct, and which one applies is NOT a judgement
// call — DOS is asymmetric here and both halves are observable:
//
//   - A per-payment WALK stops and keeps what it has summed. DOS's loop
//     condition is a DateComp against the poisoned date, which reports "later
//     than the terminal", so the loop simply ends. A perpetual (to = the
//     1/12/2149 sentinel) weekly or biweekly stream is therefore TRUNCATED at
//     the ceiling by DOS, and its value is the truncated sum.
//   - A terminal-date DERIVATION refuses. NumberOfInstallments' 26/52 arm
//     (INTSUTIL.pas:952-961) hands MDY's failure straight back through its VAR
//     parameter, and the screen reports "Bad date passed to Julian function"
//     rather than valuing the row.
//
// Use errors.Is to test for it; MDY wraps it with the offending day number.
var ErrJulianCeiling = errors.New("date is beyond the last representable day")

// LastRepresentableDate returns the newest date DOS's MDY will produce: Julian
// day 70000, which is 26 August 2091. Error messages quote it so the user is
// told the actual boundary rather than a day number.
func LastRepresentableDate() types.DateRec {
	d, err := MDY(julianCeiling)
	if err != nil {
		// Unreachable: julianCeiling is by definition inside the range.
		// (coverage: excluded)
		panic("dateutil: julianCeiling rejected by MDY: " + err.Error())
	}
	return d
}

// MDY converts a Julian day number back to a DateRec.
// This is the inverse of Julian().
//
// Ported from legacy/source/VIDEODAT.pas:
//
//	procedure MDY(daynumber:longint; var x:daterec);
func MDY(daynumber int64) (types.DateRec, error) {
	// DOS refuses outside [0, 70000] (VIDEODAT.pas:373):
	//
	//	if (daynumber<0) or (daynumber>70000) then begin x.m:=errorbyte; exit; end;
	//
	// Day 70000 is 26 Aug 2091. The port previously raised the ceiling to
	// 100000, reasoning that "with base 1900 (y up to 249 = year 2149) Julian
	// values can reach ~91000" — which is exactly backwards. DOS's own
	// perpetual sentinel 1/12/2149 is Julian 91282, i.e. OUTSIDE the range its
	// own MDY accepts, so every DOS date walk that steps through Julian (the
	// weekly/biweekly arms of AddPeriod, AddNPeriods and NumberOfInstallments —
	// INTSUTIL.pas:1213, :960) silently DIES at day 70000 rather than running to
	// 2149. Raising the ceiling did not "fix" a limitation; it removed a
	// behaviour the port has to reproduce.
	//
	// DOS's failure mode is a poisoned record, not an abort: `exit` leaves x.y
	// and x.d untouched and sets only x.m := errorbyte (-99), so the record
	// stays readable but fails dateok, and DateComp reports it as LATER than
	// every real date (INTSUTIL.pas:829-830, 836-845). A `while
	// DateComp(t, todate) <= 0` payment walk therefore EXITS and keeps the sum
	// accumulated so far. Callers must reproduce that "stop and keep" — see the
	// summation loops in presentvalue/calc.go and the table walk in
	// presentvalue/table.go — and must NOT convert it into a hard error.
	//
	// The zero DateRec returned here is the port's analogue of the poisoned
	// record: it is IsUnknown, so DateOK is false and DateComp orders it after
	// every real date, exactly as DOS does.
	if daynumber < 0 || daynumber > julianCeiling {
		return types.DateRec{}, fmt.Errorf("%w: day number %d is outside DOS's [0, %d] Julian range",
			ErrJulianCeiling, daynumber, julianCeiling)
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
	// Defensive clamp: unreachable when d holds a normalized time.Time (Go never
	// yields Day() > days-in-month); guards DateRecs built from raw bytes
	// elsewhere. (coverage: excluded)
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
		// Defensive: m is already in 1..12 after the wrap loop above (months is
		// non-negative), so this branch does not execute in practice; it mirrors
		// the original Pascal guard. (coverage: excluded)
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
	return AddPeriodFields(d.Time.Year(), int(d.Time.Month()), d.Time.Day(),
		peryr, origDay, subtract)
}

// AddPeriodFields is AddPeriod applied to RAW (year, month, day) fields — the
// form DOS actually works in. A Pascal `daterec` is three independent byte
// fields, so DOS routines routinely hold a record that is not a real calendar
// date (most often 29 February in a non-leap year, produced by a bare
// `x.y := <something>` year-field assignment with no CheckForDaysTooLarge).
// AddPeriod itself then re-derives the day from orig_day and steps the MONTH
// field, so the invalid day never propagates — but the MONTH it steps from is
// the raw one.
//
// Go's DateRec wraps a normalized time.Time, so constructing one from an
// invalid triple rolls 29/2/2065 forward to 1/3/2065 and CHANGES THE MONTH,
// which shifts every subsequent period by a full month. Callers holding a raw
// DOS record must therefore step it through this entry point rather than
// materializing a DateRec first. For any triple that is a real calendar date
// the two functions are identical.
//
// Ported from legacy/source/INTSUTIL.pas: procedure AddPeriod
func AddPeriodFields(year, month, day, peryr, origDay int, subtract bool) (types.DateRec, error) {
	py := pascalYear(year)
	m := month

	switch peryr {
	case 26, 52:
		// Julian() is linear in the day field (VIDEODAT.pas), so an
		// out-of-range day resolves to the same day number DOS computes.
		t := Julian(types.NewDateRec(year, time.Month(month), day))
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
					// `dec(y)` on a byte: 0 rolls to 255. §55.
					py = wrapPascalYear(py - 1)
					m += 12
				}
			}
		} else {
			day += 15
			if day >= 31 {
				m++
				day -= 30
				if m > 12 {
					// `inc(y)` on a byte: 255 rolls to 0. §55.
					py = wrapPascalYear(py + 1)
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
			// `dec(y)` on a byte: 0 rolls to 255. §55.
			py = wrapPascalYear(py - 1)
		} else if m > 12 {
			m -= 12
			// `inc(y)` on a byte: 255 rolls to 0. §55.
			py = wrapPascalYear(py + 1)
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
		// `lastdate.y := firstdate.y + nyears` assigns into a BYTE field, so the
		// sum truncates mod 256 (INTSUTIL.pas:1402). Everything downstream —
		// CheckForDaysTooLarge's leap test, the remaining AddPeriod steps, and
		// the caller's NumberOfInstallments — reads the TRUNCATED year. §55.
		lastPY := wrapPascalYear(py + nyears)
		// DOS keeps firstdate.d on the year-jumped date even when it overflows
		// the target month (e.g. Feb 29 in a non-leap year survives as a raw
		// "Feb 29"), then either CheckForDaysTooLarge clamps it (whole-year case)
		// or AddPeriod re-derives the day from origDay (partial case). Go's
		// time.Date would instead NORMALIZE the overflow forward (Feb 29 -> Mar 1),
		// which corrupts the base MONTH and shifts every subsequent AddPeriod by a
		// full month. Clamp the day to the target month here so the month is
		// preserved, matching INTSUTIL.pas:1398-1405 + CheckForDaysTooLarge.
		// Verified vs the real DOS engine: `amort_oracle intutil addn 2024 2 29
		// 12 12` -> 2025-02-28 and `... addn 2024 2 29 12 13` -> 2025-03-29.
		clampedDay := day
		if dim := daysInMonthPascal(m, lastPY); clampedDay > dim {
			clampedDay = dim
		}
		lastDate := types.NewDateRec(calendarYear(lastPY), time.Month(m), clampedDay)

		remaining := n - peryr*nyears
		if remaining == 0 {
			CheckForDaysTooLarge(&lastDate)
			return lastDate, nil
		}
		var err error
		for i := 0; i < remaining; i++ {
			lastDate, err = AddPeriod(lastDate, peryr, firstDate.Time.Day(), false)
			// Defensive: this monthly/semi-monthly AddPeriod path returns a
			// constructed DateRec and never errors (only the 26/52 path can,
			// and that is handled in the default case below). (coverage: excluded)
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
		return yearsDif360(
			pascalYear(z.Time.Year()), int(z.Time.Month()), z.Time.Day(),
			pascalYear(a.Time.Year()), int(a.Time.Month()), a.Time.Day())
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

// yearsDif360 is the 30/360 body of INTSUTIL.pas YearsDif (:760-763), working
// on raw Pascal-year/month/day fields with z already known to be on or after a.
func yearsDif360(zpy, zm, zd, apy, am, ad int) float64 {
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

// YearsDifRawZ is YearsDif(z, a) where z is a RAW Pascal `daterec` — three
// independent fields that need not form a real calendar date. The case that
// occurs in practice is 29 February in a non-leap year, which PRESVALU.pas
// manufactures in SummationForSteppedCola with the bare year-field assignment
// `t.y := t.y + nfullyears` (:352) and then keeps computing with.
//
// The distinction matters ONLY on the 360 basis. There YearsDif reads the month
// and day straight out of the record (INTSUTIL.pas:760-763), so 29/2/2029 and
// its normalization 1/3/2029 are two DIFFERENT day counts: month 2 with
// (29-ad)/360 and the Feb-28-or-29 kicker, versus month 3 with (1-ad)/360 — a
// 2/360-year gap. On every other basis YearsDif goes through Julian(), which is
// linear in the day field (daysbefore[m]+d, VIDEODAT.pas:364), so the raw record
// and its normalization produce the identical day number and plain YearsDif is
// already exact; this delegates there rather than duplicating the logic.
//
// 2026-07-25, PV fuzzer5 seed 8906 (N=1000):
//
//	pv_oracle table 0.0373100000 360 summary 1,...,12 2 asof=9.8.2024 \
//	  per=29.11.2023:29.11.2029:12:-82241.93:0.0128670000
//	DOS -5750173.988234   port -5750158.460808
//
// Period III of the stepped-COLA summation opens on the raw 29/2/2029. Valuing
// it as 1/3/2029 shortened that one payment's discount span by 2/360 of a year,
// worth 15.53 on a 5.75M valuation.
func YearsDifRawZ(zy, zm, zd int, a types.DateRec,
	basis types.BasisType, yrinv float64, isLoanCalc bool) float64 {
	if basis != types.Basis360 {
		return YearsDif(types.NewDateRec(zy, time.Month(zm), zd), a,
			basis, yrinv, isLoanCalc)
	}
	apy, am, ad := pascalYear(a.Time.Year()), int(a.Time.Month()), a.Time.Day()
	zpy := pascalYear(zy)
	// DOS DateComp overlays the record as a longint of (d, m, y) little-endian
	// (INTSUTIL.pas:828): compare year, then month, then day, on raw fields.
	after := false
	switch {
	case apy != zpy:
		after = apy > zpy
	case am != zm:
		after = am > zm
	default:
		after = ad > zd
	}
	if after {
		// DOS's `YearsDif := -YearsDif(a,z)` — a takes the z role and the
		// 30/360 end-of-month kickers key off the RAW date instead.
		return -yearsDif360(apy, am, ad, zpy, zm, zd)
	}
	return yearsDif360(zpy, zm, zd, apy, am, ad)
}

// YearsDifRawA is YearsDif(z, a) where the SUBTRAHEND a is a RAW Pascal
// `daterec` rather than z. It is the mirror of YearsDifRawZ and exists for the
// same reason: DOS routines pass around daterecs whose day field can name a day
// past the end of its month (see NumberOfInstallmentsRaw), and on the 360 basis
// YearsDif reads that field directly.
//
// DOS's own antisymmetry supplies the implementation: INTSUTIL.pas opens the 360
// branch with `if DateComp(a,z)>0 then YearsDif := -YearsDif(a,z)`, i.e. the two
// arguments are interchangeable up to sign — including the end-of-month kickers,
// which always key off whichever date ended up in the `a` slot.
func YearsDifRawA(z types.DateRec, ay, am, ad int,
	basis types.BasisType, yrinv float64, isLoanCalc bool) float64 {
	return -YearsDifRawZ(ay, am, ad, z, basis, yrinv, isLoanCalc)
}

// abs returns the absolute value of an int.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// NumberOfInstallments returns the number of payments between f and l at the
// given frequency, AND adjusts the last date l to fall exactly on a payment
// day "in the vicinity" of the input l, as selected by z:
//
//	Before       — the payment strictly before l
//	OnOrBefore   — the payment on or before l
//	After        — the payment strictly after l
//	OnOrAfter    — the payment on or after l
//
// The (possibly adjusted) last date is returned alongside the count.
//
// Ported from legacy/src/dos_source/INTSUTIL.pas: function NumberOfInstallments.
//
// The returned date is NORMALIZED to a real calendar date. DOS's snap can emit
// an un-representable `daterec` — see NumberOfInstallmentsRaw, which is the same
// routine returning the last date as three raw fields, for when that matters.
func NumberOfInstallments(f, l types.DateRec, peryr int, z types.Upto) (int, types.DateRec) {
	n, d, _ := NumberOfInstallmentsE(f, l, peryr, z)
	return n, d
}

// NumberOfInstallmentsE is NumberOfInstallments with DOS's failure signal kept
// instead of discarded. See NumberOfInstallmentsRawE for what the error means
// and why every caller must decide about it deliberately.
func NumberOfInstallmentsE(f, l types.DateRec, peryr int, z types.Upto) (int, types.DateRec, error) {
	n, ry, rm, rd, err := NumberOfInstallmentsRawE(f, l, peryr, z)
	return n, types.NewDateRec(ry, time.Month(rm), rd), err
}

// NumberOfInstallmentsRaw is NumberOfInstallments returning the snapped last
// date as three INDEPENDENT fields (calendar year, month 1-12, day) that need
// not form a real calendar date.
//
// DOS takes `l` as a VAR parameter and, for peryr in [1,2,3,4,6,12], ends
// ChoosePaymentDate with
//
//	if (flast) then l.d:=daysinm(l) else l.d:=f.d;
//
// (INTSUTIL.pas:1013). That final assignment copies the FROM-date's day of month
// onto the snapped month with NO clamping, so a from-day of 29/30/31 landing on
// February writes back a phantom such as 30/2/2037. The caller's `todate` is
// that phantom, and PRESVALU.pas's Summation then reads it through YearsDif —
// which on the 360 basis consumes the month and day fields directly and so
// distinguishes 30/2/2037 from Go's time.Time normalization 2/3/2037 by
// 2/360 of a year.
//
// 2026-07-25, PV fuzzer5 seed 20404 (N=1000):
//
//	pv_oracle table 0.0357760000 360 both 4,10 cnt asof=11.6.2026 \
//	  ... per=30.8.2024:30.4.2048:2:163986.02:0.0215740000 ...
//	DOS 12667674.552427   port 12667389.295758
//
// Every table line agreed; only the closed-form screen total was wrong, by
// 285.26. Summation's `since_from := false` branch is the sole reader of
// `todate` through YearsDif, and it reads it twice — once as
// `since := YearsDif(asof, todate)` and once inside
// `sum := sum * exxp(YearsDif(todate, fromdate) * cola)` — so both the discount
// exponent and the COLA range were short by 2/360.
//
// Callers that only need a date to COMPARE (DateComp, LastDayFn, the table walk)
// should keep using NumberOfInstallments: the phantom differs from its
// normalization only by naming a day past the end of the month, and every
// payment on the grid is anchored to the from-date's day, so no payment date can
// fall strictly between the two and the comparisons are observationally
// identical.
func NumberOfInstallmentsRaw(f, l types.DateRec, peryr int, z types.Upto) (int, int, int, int) {
	n, ry, rm, rd, _ := NumberOfInstallmentsRawE(f, l, peryr, z)
	return n, ry, rm, rd
}

// NumberOfInstallmentsRawE is NumberOfInstallmentsRaw with DOS's failure signal
// RETURNED rather than swallowed.
//
// Only the 26/52 arm can fail, and only through MDY: DOS snaps a weekly or
// biweekly terminal by Julian arithmetic and converts back with
//
//	MDY(last,l);          { INTSUTIL.pas:960 }
//
// so a terminal past Julian 70000 (27 Aug 2091) leaves `l` poisoned with
// m := errorbyte. DOS does NOT check — it hands the poisoned record back through
// its VAR parameter, `theresult` is computed from Julian(l) = -88, and the
// caller's screen refuses the row with "Bad date passed to Julian function".
//
// This routine reproduces that faithfully: the returned (ry, rm, rd) is the
// unknown date MDY produced, exactly as DOS's caller sees an unusable daterec,
// AND the error says why. The plain NumberOfInstallmentsRaw wrapper drops the
// error, which is what every caller that only needs the count and a date to
// COMPARE wants — an unknown date sorts after every real one under DateComp,
// which is DOS's own behaviour. Callers that must refuse the row instead (the PV
// periodic valuation, PRESVALU.pas's consumer) use this variant.
//
// 2026-07-30 rounds 2-4: four attempts to re-derive DOS's condition with a
// `Julian(toDate) > 70000` guard inside the PV code all failed, because by the
// time any of those guards ran the to-date had ALREADY been replaced by the
// unknown date this function returns — the guard was reading -88, not 70096.
// Propagating the error instead of re-deriving the condition is the fix.
func NumberOfInstallmentsRawE(f, l types.DateRec, peryr int, z types.Upto) (int, int, int, int, error) {
	var ceilErr error
	rawOf := func(d types.DateRec) (int, int, int) {
		return d.Time.Year(), int(d.Time.Month()), d.Time.Day()
	}
	// DOS short-circuits a "forever" terminal: when l is in the latest/sentinel
	// year (2149) it returns maxint WITHOUT snapping l (INTSUTIL.pas:1026-1028:
	// "if (l.y=latest.y) then begin NumberOfInstallments:=maxint; exit; end").
	// The port previously computed a finite count (~1482) AND snapped l off the
	// sentinel — which both truncates a converging perpetuity and defeats the
	// forever-detection in PeriodicSummation (which keys on toDate==latest). Keep
	// l unchanged and return the sentinel count; downstream valuation is
	// closed-form (SumFormula) so the large count yields the convergent limit
	// rather than iterating. math.MaxInt32 matches the FPC oracle's maxint.
	if l.Time.Year() == types.LatestDate().Time.Year() {
		ry, rm, rd := rawOf(l)
		return math.MaxInt32, ry, rm, rd, nil
	}
	fy, fm, fd := f.Time.Year(), int(f.Time.Month()), f.Time.Day()
	// The snapped terminal, carried as raw fields. Every branch below writes it;
	// only the peryr in [1,2,3,4,6,12] branch can make it un-representable.
	ry, rm, rd := rawOf(l)
	var theresult int
	monthsbtwn := 1
	if peryr <= 12 {
		monthsbtwn = 12 / peryr
	}

	switch peryr {
	case 26, 52:
		ddiff := int(Julian(l) - Julian(f))
		daze := 364 / peryr
		ddiff = ddiff % daze
		if ddiff == 0 && (z == types.Before || z == types.OnOrAfter) {
			ddiff = daze
		}
		var last int64
		if z == types.Before || z == types.OnOrBefore {
			last = Julian(l) - int64(ddiff)
		} else {
			last = Julian(l) + int64(daze) - int64(ddiff)
		}
		// DOS: `MDY(last,l)` with no check (INTSUTIL.pas:960). On failure the
		// poisoned record flows on and Julian(l) reads -88, so `theresult` is
		// garbage too — reproduce both, and hand the reason to the caller.
		l, ceilErr = MDY(last)
		ry, rm, rd = rawOf(l)
		theresult = int((Julian(l) - Julian(f)) / int64(daze))

	case 24:
		ly, lm := l.Time.Year(), int(l.Time.Month())
		theresult = 2*(lm-fm) + 24*(ly-fy) // first estimate
		switch z {
		case types.Before, types.OnOrBefore:
			theresult += 2
			atry, _ := AddNPeriods(f, peryr, theresult)
			for !Criterion(atry, l, z) {
				atry, _ = AddPeriod(atry, peryr, fd, true)
				theresult--
			}
			l = atry
			ry, rm, rd = rawOf(l)
		case types.After, types.OnOrAfter:
			theresult -= 2
			atry, _ := AddNPeriods(f, peryr, theresult)
			for !Criterion(atry, l, z) {
				atry, _ = AddPeriod(atry, peryr, fd, false)
				theresult++
			}
			l = atry
			ry, rm, rd = rawOf(l)
		}

	default: // peryr in [1,2,3,4,6,12]
		ly, lm, ld := l.Time.Year(), int(l.Time.Month()), l.Time.Day()
		flast := LastDayFn(f, peryr)
		llast := LastDayFn(l, peryr)
		ddiff := ld - fd
		mdiff := (lm - fm) % monthsbtwn
		for mdiff < 0 {
			mdiff += monthsbtwn
		}
		if mdiff == 0 {
			switch z {
			case types.Before:
				if ddiff <= 0 || (flast && llast) {
					lm -= monthsbtwn
				}
			case types.OnOrBefore:
				if ddiff < 0 && !(flast && llast) {
					lm -= monthsbtwn
				}
			case types.After:
				if ddiff >= 0 || (flast && llast) {
					lm += monthsbtwn
				}
			case types.OnOrAfter:
				if ddiff > 0 && !(flast && llast) {
					lm += monthsbtwn
				}
			}
		} else {
			switch z {
			case types.OnOrBefore, types.Before:
				lm -= mdiff
			case types.OnOrAfter, types.After:
				lm += monthsbtwn - mdiff
			}
		}
		// Correct for year overflow.
		if lm <= 0 {
			ly--
			lm += 12
		} else if lm > 12 {
			ly++
			lm -= 12
		}
		var newDay int
		if flast {
			newDay = daysInMonthPascal(lm, pascalYear(ly))
		} else {
			// DOS: `l.d := f.d` — no clamp. newDay can exceed the length of
			// month lm, which is exactly the phantom this function preserves.
			newDay = fd
		}
		ry, rm, rd = ly, lm, newDay
		theresult = (12*(ly-fy) + (lm - fm)) / monthsbtwn
	}

	return theresult + 1, ry, rm, rd, ceilErr
}
