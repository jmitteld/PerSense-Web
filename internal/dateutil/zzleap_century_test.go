package dateutil

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestPascalLeapRuleVsDateRecStorage pins a KNOWN, DELIBERATELY UNCLOSED gap
// (docs/discrepancies.md §54): DOS's calendar and Go's disagree on century
// years, and the port's two layers disagree with each other as a result.
//
// DOS's rule has no century correction — VIDEODAT.pas:340-347:
//
//	function DaysInM(f :daterec):byte;
//	         begin with f do begin
//	         if (m=2) then begin
//	            if (y mod 4 = 0) then daysinm:=29 else daysinm:=28;
//	            end
//
// so DOS believes 29 February 2100 exists. `daysInMonthPascal` reproduces that
// faithfully and returns 29. But `types.DateRec` wraps a `time.Time`, and
// `time.Date(2100, February, 29, …)` NORMALIZES to 1 March 2100 — the port can
// compute the DOS answer but cannot STORE the date it implies.
//
// Consequence, measured 2026-07-31 against the real DOS engine: a quarterly
// day-29 schedule long enough to reach February 2100 diverges, and the boundary
// is exactly there.
//
//	amort_oracle 185232.59 0.020861 305 4 loandmy=29.9.2023 firstdmy=29.11.2023 targ=401.73
//	  -> 177231.14 both engines        (last payment 11/2099 — never reaches Feb 2100)
//	amort_oracle 185232.59 0.020861 306 4 loandmy=29.9.2023 firstdmy=29.11.2023 targ=401.73
//	  -> DOS 177735.47, port 177735.54 (last payment  2/2100 — DOS puts it on the 29th)
//
// Day-of-month 15 and 28 agree at every term; 29 and 30 diverge from n=306 on.
// 2100 is the only century non-leap year inside DOS's date range, so this is the
// only place the two calendars can part company going forward; 1900 is the
// mirror case at the bottom of the range and `DecideAboutFeb29` guards it with
// `and (wy>0)` while `DaysInM` does not — DOS is internally inconsistent there.
//
// SCOPE DECISION (Nate, 2026-07-31): document and defer. Closing it means
// giving DateRec raw y/m/d fields like the Pascal record — a port-wide
// refactor — and the affected region is schedules that reach February 2100 on a
// day-29/30 grid. This test exists so the next session finds the limit stated
// rather than rediscovering it as a mystery divergence.
func TestPascalLeapRuleVsDateRecStorage(t *testing.T) {
	feb := func(y int) types.DateRec { return types.NewDateRec(y, time.February, 1) }

	// The arithmetic layer follows DOS: every year divisible by 4, no exceptions.
	for _, y := range []int{1900, 2000, 2096, 2100, 2104, 2200} {
		if got := DaysInM(feb(y)); got != 29 {
			t.Errorf("DaysInM(Feb %d) = %d, want 29 — the port must keep DOS's "+
				"uncorrected `y mod 4 = 0` rule (VIDEODAT.pas:343)", y, got)
		}
	}
	if got := DaysInM(feb(2099)); got != 28 {
		t.Errorf("DaysInM(Feb 2099) = %d, want 28", got)
	}

	// The STORAGE layer cannot hold what the arithmetic layer just computed.
	// These assertions document the gap; they are not an endorsement of it.
	for _, tc := range []struct {
		year     int
		wantDay  int
		wantMon  time.Month
		agreeing bool
	}{
		{2000, 29, time.February, true}, // leap under both rules
		{2096, 29, time.February, true}, // leap under both rules
		{2104, 29, time.February, true}, // leap under both rules
		{2100, 1, time.March, false},    // DOS: 29 Feb. Go: normalizes away
		{1900, 1, time.March, false},    // same, at the bottom of the range
	} {
		got := types.NewDateRec(tc.year, time.February, 29)
		if got.Time.Day() != tc.wantDay || got.Time.Month() != tc.wantMon {
			t.Errorf("NewDateRec(%d, Feb, 29) = %s, want %d %s — if this changed, "+
				"DateRec's representation changed and §54 should be revisited",
				tc.year, got.Time.Format("2006-01-02"), tc.wantDay, tc.wantMon)
		}
		if tc.agreeing && got.Time.Month() != time.February {
			t.Errorf("year %d should be leap under BOTH calendars", tc.year)
		}
	}

	// The payment-grid consequence, isolated: stepping a quarterly day-29 grid
	// from November 2099 lands on DOS's 29 Feb 2100 and on Go's 1 March 2100.
	got, err := AddPeriodFields(2099, 11, 29, 4, 29, false)
	if err != nil {
		t.Fatalf("AddPeriodFields: %v", err)
	}
	if got.Time.Format("2006-01-02") != "2100-03-01" {
		t.Errorf("AddPeriodFields(29.11.2099, perYr=4, anchor=29) = %s, want "+
			"2100-03-01 (the normalized stand-in for DOS's 29.2.2100). A change "+
			"here means the storage gap moved — see §54",
			got.Time.Format("2006-01-02"))
	}
}
