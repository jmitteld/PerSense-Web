package amortization

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// zzharnessdates_test.go — R2, docs/harness_policy.md: THE HARNESS NEVER COMPUTES
// A DATE (and where it must, that computation is pinned to DOS).
//
// SIX of the seven harness defects in this project were the harness computing a
// date its own way:
//
//	§51        four cmd/goamort date bugs
//	§55        the oracle driver's option-date year assigned into a Pascal BYTE
//	round 14   firstPeriodDate's `m := 1 + 12/perYr` in INTEGER division, which
//	           collapsed every sub-monthly frequency to month 1 and silently
//	           suppressed 7 of 12 oracle comparisons
//
// Each one read exactly like an engine divergence. None was.
//
// `dos_fuzzer5_test.go` places every whole-month option token (`mor=`, `b<N>=`,
// `adj=`, `pre=`) with one helper, `fz5AddMonths`. Until round 16 that helper was
// a closure inside the test function — unreachable, and therefore untestable.
// This file holds it against DOS's own `AddNPeriods`, reached through
// `amort_oracle intutil addn Y M D PERYR N`.
//
// WHY `addn` IS THE RIGHT AUTHORITY. It is the primitive amort_oracle.pas itself
// uses to place option dates, printed as the record holds it (un-normalized), so
// a disagreement here is a disagreement about the actual bytes the two engines
// are handed.

func harnessOracleBin(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("PERSENSE_ORACLE_DIR")
	if dir == "" {
		dir = "/tmp/oraclebuild"
	}
	bin := filepath.Join(dir, "amort_oracle")
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("oracle required but missing at %s", bin)
		}
		t.Skip("oracle not built; set PERSENSE_ORACLE_DIR or run legacy/oracle/build_linux.sh")
	}
	return bin
}

// dosAddN returns DOS's AddNPeriods(first, peryr, n) as raw y/m/d.
func dosAddN(t *testing.T, bin string, y, m, d, perYr, n int) (int, int, int) {
	t.Helper()
	out, err := exec.Command(bin, "intutil", "addn",
		strconv.Itoa(y), strconv.Itoa(m), strconv.Itoa(d),
		strconv.Itoa(perYr), strconv.Itoa(n)).Output()
	if err != nil {
		t.Fatalf("oracle addn %d-%d-%d perYr=%d n=%d: %v", y, m, d, perYr, n, err)
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) != 4 || f[0] != "last" {
		t.Fatalf("unparseable addn output %q", string(out))
	}
	yy, _ := strconv.Atoi(f[1])
	mm, _ := strconv.Atoi(f[2])
	dd, _ := strconv.Atoi(f[3])
	return yy, mm, dd
}

// TestR2HarnessMonthStepMatchesDOSAddNPeriods pins fz5AddMonths.
//
// The generator only ever places option tokens on WHOLE-MONTH grids — it
// documents this at the `mPer = 12/perYr` comment and skips 24/26/52 for option
// placement precisely because a period is not a whole number of months there. So
// the equivalence asserted is:
//
//	fz5AddMonths(loanDate, k * (12/perYr))  ==  AddNPeriods(loanDate, perYr, k)
//
// over the frequencies where `12/perYr` is exact.
func TestR2HarnessMonthStepMatchesDOSAddNPeriods(t *testing.T) {
	bin := harnessOracleBin(t)

	// Start dates chosen to exercise the traps this family has actually produced:
	// month-end clamping (31st, 30th, 29th), a leap day, and DOS's Feb-2100
	// century disagreement (§54) approached from below.
	starts := []struct{ y, m, d int }{
		{2024, 1, 31}, {2024, 2, 29}, {2024, 3, 30}, {2024, 5, 29},
		{2021, 6, 29}, {2024, 12, 31}, {2023, 8, 6}, {2025, 11, 30},
	}
	// Whole-month frequencies only: 12/perYr is exact for each.
	freqs := []int{1, 2, 3, 4, 6, 12}
	// Period counts spanning short terms through past DOS's year byte (§55),
	// which is where the harness's own wrap logic has to agree.
	counts := []int{0, 1, 2, 5, 12, 40, 120, 360, 1000, 2400, 3600}

	checked, mismatches, sec54 := 0, 0, 0
	// isUnrepresentable reports DOS's §54 case: DOS's leap rule carries NO century
	// correction (`y mod 4 = 0` and nothing else), so DOS holds 29 February in
	// 1900, 2100, 2200… Go's calendar does not, and types.DateRec is backed by
	// time.Time, which rolls such a date to 1 March. That is §54, deferred by
	// decision — not a defect in this helper.
	isUnrepresentable := func(y, m, d int) bool {
		if m != 2 || d != 29 {
			return false
		}
		return y%4 == 0 && !(y%400 == 0 || (y%100 != 0))
	}
	for _, s := range starts {
		ld := types.NewDateRec(s.y, time.Month(s.m), s.d)
		for _, perYr := range freqs {
			mPer := 12 / perYr
			for _, k := range counts {
				wy, wm, wd := dosAddN(t, bin, s.y, s.m, s.d, perYr, k)
				got := fz5AddMonths(ld, k*mPer)
				checked++

				gy := got.Time.Year()
				gm := int(got.Time.Month())
				gd := got.Time.Day()
				if gy != wy || gm != wm || gd != wd {
					// §54 first: DOS's date is real, Go's calendar cannot hold it.
					// Counted and reported, not failed — the refactor that would
					// close it is scoped out (docs/discrepancies.md §54).
					if isUnrepresentable(wy, wm, wd) {
						sec54++
						continue
					}
					mismatches++
					if mismatches <= 12 {
						t.Errorf("fz5AddMonths(%04d-%02d-%02d, %d months) = %04d-%02d-%02d, "+
							"DOS AddNPeriods(perYr=%d, n=%d) = %04d-%02d-%02d\n"+
							"  repro: amort_oracle intutil addn %d %d %d %d %d",
							s.y, s.m, s.d, k*mPer, gy, gm, gd,
							perYr, k, wy, wm, wd,
							s.y, s.m, s.d, perYr, k)
					}
				}
			}
		}
	}
	// The §54 count is a MEASUREMENT, not just an exemption — START_HERE §3 asks
	// for a number on what the deferred date-layer refactor costs, and this is one
	// concrete component of it: the rate at which the harness itself hands DOS and
	// the Go engine different option dates.
	t.Logf("R2: %d harness-vs-DOS date comparisons, %d genuine mismatches, "+
		"%d §54-unrepresentable (%.2f%% — DOS holds 29 Feb in a Gregorian "+
		"non-leap year and types.DateRec rolls it to 1 March)",
		checked, mismatches, sec54, 100*float64(sec54)/float64(checked))
	if checked == 0 {
		t.Error("no comparisons ran — the pin is vacuous")
	}
}

// TestR2HarnessMonthStepIsNotUsedOffTheWholeMonthGrid documents, as an executable
// assertion, the boundary of what the helper above may be used for.
//
// At 24/26/52 a period is NOT a whole number of months, so `12/perYr` is 0 or
// fractional and the month-step helper cannot represent the grid at all. That is
// exactly the shape of round 14's `firstPeriodDate` bug — `m := 1 + 12/perYr` in
// integer division silently collapsed every sub-monthly frequency to month 1 —
// so this records that the harness must NOT reach for a month step there, rather
// than leaving it to a comment.
func TestR2HarnessMonthStepIsNotUsedOffTheWholeMonthGrid(t *testing.T) {
	for _, perYr := range []int{24, 26, 52} {
		if mPer := 12 / perYr; mPer != 0 {
			t.Errorf("perYr=%d: 12/perYr = %d, expected 0 — this test's premise "+
				"(that sub-monthly grids have no whole-month step) has changed",
				perYr, mPer)
		}
	}
	// And the consequence, stated so it cannot be forgotten: a zero month step
	// makes every option land on the loan date itself.
	ld := types.NewDateRec(2024, time.January, 1)
	for _, k := range []int{1, 5, 40} {
		if got := fz5AddMonths(ld, k*(12/24)); !got.Time.Equal(ld.Time) {
			t.Errorf("expected the degenerate collapse onto the loan date, got %v", got.Time)
		}
	}
	_ = fmt.Sprint()
}
