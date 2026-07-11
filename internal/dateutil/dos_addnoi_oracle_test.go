package dateutil

import (
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Differential coverage for AddNPeriods and NumberOfInstallments against the
// REAL DOS INTSUTIL engine via amort_oracle `intutil addn` / `intutil noi`.
//
// These guard three findings from the 2026-07-09 DOS-vs-Go logic audit:
//
//   - P1 (FIXED): AddNPeriods dropped DOS's month-clamp on the year-jumped date,
//     so a Feb-29 (or day-29/30/31-in-a-short-month) origin whose anniversary
//     lands in a shorter month normalized FORWARD (Feb 29 -> Mar 1) instead of
//     clamping (Feb 29 -> Feb 28), shifting every later period by a full month.
//   - P5 (FIXED): NumberOfInstallments lacked DOS's "l in the latest/sentinel
//     year (2149) -> maxint, l untouched" short-circuit (INTSUTIL.pas:1026-1028),
//     so a "forever" terminal produced a finite truncated count and snapped the
//     to-date off the sentinel (defeating forever-detection in PV).
//   - P2 (KNOWN, representation-limited): NumberOfInstallments' snapped last date
//     can be a DOS "Feb 30" (raw day = fromDate.day, un-normalized) which Go's
//     time.Time cannot represent; Go normalizes to Mar 2. See the characterization
//     test below and docs/discrepancies.md §15.

func runAddNOracle(t *testing.T, f types.DateRec, peryr, n int) (types.DateRec, bool) {
	args := []string{"intutil", "addn",
		strconv.Itoa(f.Time.Year()), strconv.Itoa(int(f.Time.Month())), strconv.Itoa(f.Time.Day()),
		strconv.Itoa(peryr), strconv.Itoa(n)}
	out, err := exec.Command(ydOracleBin(), args...).Output()
	if err != nil {
		return types.DateRec{}, false
	}
	// format: "last YYYY M D"
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) != 4 || fields[0] != "last" {
		return types.DateRec{}, false
	}
	y, _ := strconv.Atoi(fields[1])
	m, _ := strconv.Atoi(fields[2])
	d, _ := strconv.Atoi(fields[3])
	return types.NewDateRec(y, time.Month(m), d), true
}

// runNOIOracle returns (count, rawYear, rawMonth, rawDay, ok). The last date is
// returned as raw ints because DOS can legitimately emit an un-normalized date
// (e.g. Feb 30) that types.NewDateRec would silently normalize.
func runNOIOracle(f, l types.DateRec, peryr int, z string) (int, int, int, int, bool) {
	args := []string{"intutil", "noi",
		strconv.Itoa(f.Time.Year()), strconv.Itoa(int(f.Time.Month())), strconv.Itoa(f.Time.Day()),
		strconv.Itoa(l.Time.Year()), strconv.Itoa(int(l.Time.Month())), strconv.Itoa(l.Time.Day()),
		strconv.Itoa(peryr), z}
	out, err := exec.Command(ydOracleBin(), args...).Output()
	if err != nil {
		return 0, 0, 0, 0, false
	}
	// format: "n COUNT last YYYY M D"
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) != 6 {
		return 0, 0, 0, 0, false
	}
	n, _ := strconv.Atoi(fields[1])
	y, _ := strconv.Atoi(fields[3])
	m, _ := strconv.Atoi(fields[4])
	d, _ := strconv.Atoi(fields[5])
	return n, y, m, d, true
}

// TestDOSAddNPeriodsSweep is the P1 regression: every monthly-family AddNPeriods
// result must match the real DOS engine to the day, including the month-end and
// leap-day origins that exposed the normalization bug.
func TestDOSAddNPeriodsSweep(t *testing.T) {
	if _, err := os.Stat(ydOracleBin()); err != nil {
		t.Skipf("oracle not present (%s); build via legacy/oracle/build_linux.sh", ydOracleBin())
	}

	years := []int{2023, 2024, 2025, 2027, 2028}
	months := []int{1, 2, 3, 4, 12}
	days := []int{1, 15, 28, 29, 30, 31}
	peryrs := []int{1, 2, 4, 6, 12}
	ns := []int{1, 2, 3, 6, 12, 13, 24, 25, 47, 48}

	cases, mismatches := 0, 0
	for _, y := range years {
		for _, mo := range months {
			for _, d := range days {
				f := types.NewDateRec(y, time.Month(mo), d)
				// Only test genuine source dates (skip inputs that were themselves
				// normalized, e.g. Feb 31 -> Mar 2, which is not a real origin).
				if f.Time.Day() != d || int(f.Time.Month()) != mo || f.Time.Year() != y {
					continue
				}
				for _, py := range peryrs {
					for _, n := range ns {
						want, ok := runAddNOracle(t, f, py, n)
						if !ok {
							t.Fatalf("oracle addn failed for %v py=%d n=%d", f.Time.Format("2006-01-02"), py, n)
						}
						got, err := AddNPeriods(f, py, n)
						if err != nil {
							t.Fatalf("AddNPeriods(%v,%d,%d) error: %v", f.Time.Format("2006-01-02"), py, n, err)
						}
						cases++
						if got.Time.Year() != want.Time.Year() ||
							got.Time.Month() != want.Time.Month() ||
							got.Time.Day() != want.Time.Day() {
							mismatches++
							if mismatches <= 20 {
								t.Errorf("AddNPeriods(%s, py=%d, n=%d): go=%s dos=%s",
									f.Time.Format("2006-01-02"), py, n,
									got.Time.Format("2006-01-02"), want.Time.Format("2006-01-02"))
							}
						}
					}
				}
			}
		}
	}
	t.Logf("AddNPeriods differential: %d cases, %d mismatches", cases, mismatches)
	if mismatches > 0 {
		t.Fatalf("%d/%d AddNPeriods cases diverged from the DOS oracle", mismatches, cases)
	}
}

// TestDOSAddNPeriodsFeb29Clamp pins the exact P1 trigger to the DOS oracle
// values so the fix is guarded even when the oracle binary is absent.
func TestDOSAddNPeriodsFeb29Clamp(t *testing.T) {
	feb29 := types.NewDateRec(2024, time.February, 29)
	cases := []struct {
		n           int
		wy, wm, wdd int
	}{
		{12, 2025, 2, 28}, // amort_oracle intutil addn 2024 2 29 12 12 -> 2025 2 28
		{13, 2025, 3, 29}, // amort_oracle intutil addn 2024 2 29 12 13 -> 2025 3 29
		{1, 2024, 3, 29},  // amort_oracle intutil addn 2024 2 29 12 1  -> 2024 3 29
		{48, 2028, 2, 29}, // 4 whole years -> leap again, day preserved
	}
	for _, c := range cases {
		got, err := AddNPeriods(feb29, 12, c.n)
		if err != nil {
			t.Fatalf("AddNPeriods n=%d: %v", c.n, err)
		}
		if got.Time.Year() != c.wy || int(got.Time.Month()) != c.wm || got.Time.Day() != c.wdd {
			t.Errorf("AddNPeriods(2024-02-29,12,%d) = %s, want %04d-%02d-%02d",
				c.n, got.Time.Format("2006-01-02"), c.wy, c.wm, c.wdd)
		}
	}
}

// TestDOSNumberOfInstallmentsForeverGuard is the P5 regression: a terminal in the
// sentinel/latest year returns the DOS maxint sentinel with the to-date left
// untouched, matching INTSUTIL.pas:1026-1028.
func TestDOSNumberOfInstallmentsForeverGuard(t *testing.T) {
	from := types.NewDateRec(2026, time.January, 1)
	to := types.NewDateRec(2149, time.June, 15) // latest.y == 2149
	n, l := NumberOfInstallments(from, to, 12, types.OnOrBefore)
	if n != math.MaxInt32 {
		t.Errorf("forever NumberOfInstallments count = %d, want %d (DOS maxint)", n, math.MaxInt32)
	}
	if l.Time.Year() != 2149 || l.Time.Month() != time.June || l.Time.Day() != 15 {
		t.Errorf("forever to-date snapped to %s, want it left untouched at 2149-06-15", l.Time.Format("2006-01-02"))
	}

	// Cross-check against the real DOS engine when present.
	if _, err := os.Stat(ydOracleBin()); err == nil {
		on, oy, om, od, ok := runNOIOracle(from, to, 12, "on_or_before")
		if ok {
			if on != math.MaxInt32 {
				t.Errorf("oracle forever count = %d, want %d", on, math.MaxInt32)
			}
			if oy != 2149 || om != 6 || od != 15 {
				t.Errorf("oracle forever to-date = %04d-%02d-%02d, want 2149-06-15 (untouched)", oy, om, od)
			}
		}
	}
}

// TestNumberOfInstallmentsFeb30KnownDivergence characterizes P2: a monthly cycle
// whose snapped terminal lands on a DOS "Feb 30" (raw day = fromDate.day). DOS
// keeps Feb 30 (its 30/360 YearsDif treats it as a clean whole month); Go's
// time.Time normalizes it to Mar 2, adding ~2/360 year of interest on that one
// anchor. This is a REPRESENTATION-LIMITED known divergence (documented in
// docs/discrepancies.md §15), not yet fixed — this test pins the current Go
// behavior AND records the DOS value so any future change (or a real fix) is
// noticed. The installment COUNT is unaffected and DOES match DOS.
func TestNumberOfInstallmentsFeb30KnownDivergence(t *testing.T) {
	from := types.NewDateRec(2025, time.January, 30)
	to := types.NewDateRec(2025, time.February, 5)
	n, l := NumberOfInstallments(from, to, 12, types.OnOrAfter)

	// The count matches DOS (oracle: n 2 last 2025 2 30).
	if n != 2 {
		t.Errorf("count = %d, want 2 (matches DOS)", n)
	}
	// Current Go behavior: DOS "Feb 30" normalizes to Mar 2. If this ever changes
	// to Feb 28 or an actual Feb-30-equivalent, revisit docs/discrepancies.md §15.
	if !(l.Time.Year() == 2025 && l.Time.Month() == time.March && l.Time.Day() == 2) {
		t.Errorf("snapped to-date = %s; expected the known Go normalization 2025-03-02 "+
			"(DOS keeps un-representable 2025-02-30). If this changed intentionally, update docs §15.",
			l.Time.Format("2006-01-02"))
	}

	// Confirm the divergence's magnitude against the oracle when present: DOS's
	// 30/360 YearsDif(Feb30, Jan30) = exactly 1/12; Go's (Mar2, Jan30) is larger.
	if _, err := os.Stat(ydOracleBin()); err == nil {
		on, oy, om, od, ok := runNOIOracle(from, to, 12, "on_or_after")
		if ok {
			if on != 2 || oy != 2025 || om != 2 || od != 30 {
				t.Logf("oracle noi = n=%d last=%04d-%02d-%02d (expected n=2 last=2025-02-30)", on, oy, om, od)
			}
		}
	}
}
