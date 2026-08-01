package dateutil

import (
	"os"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Regression coverage for docs/discrepancies.md §55 — DOS stores a date's YEAR
// in a BYTE (`daterec = record d,m: shortint; y: byte end`, Globals.pas:46-48
// and VIDEODAT.pas:10), compiled with range checking off, so every assignment
// to that field truncates mod 256.
//
// The port stored an unbounded time.Time year, so any schedule whose horizon
// left [1900, 2155] silently disagreed with the engine it reproduces. This is
// the amortization counterpart of §47 (PV's Julian ceiling) and it is what made
// the 2026-07-31 assessment's worst case — a 300-payment annual loan DOS answers
// cleanly — come out 14x high.
//
// The cases are split by the DOS SITE they exercise, because the sites are
// independent and each must be able to fail on its own:
//
//	A  INTSUTIL.pas:1402  `lastdate.y := firstdate.y + nyears` — the year JUMP
//	B  INTSUTIL.pas:1233/1244/1248  `inc(y)` / `dec(y)` inside AddPeriod
//
// Reverting A alone leaves the whole-year cases wrong and the +/-1 boundary
// cases right; reverting B alone does the opposite. Both were checked by hand
// on 2026-07-31.
//
// Every expectation below is the REAL DOS ENGINE's answer, quoted with its
// command line. They are also asserted live against the oracle in
// TestDOSYearByteWrapVsOracle when it is present.
func TestYearByteWrapGoldens(t *testing.T) {
	cases := []struct {
		name             string
		y, m, d          int
		peryr, n         int
		wy, wm, wd       int
		site             string
		oracleCmd        string
		wantErrSubstring string
	}{
		{
			// 123 + 299 = 422 -> 422-256 = 166 -> 2066. The site-A jump alone;
			// n is a whole number of years so AddPeriod never runs.
			name: "A/annual-300-wraps-once", y: 2023, m: 7, d: 29, peryr: 1, n: 299,
			wy: 2066, wm: 7, wd: 29, site: "A",
			oracleCmd: "amort_oracle intutil addn 2023 7 29 1 299 -> last 2066 7 29",
		},
		{
			// 123 + 133 = 256 -> exactly 0 -> 1900. The boundary itself.
			name: "A/annual-lands-on-zero", y: 2023, m: 7, d: 29, peryr: 1, n: 133,
			wy: 1900, wm: 7, wd: 29, site: "A",
			oracleCmd: "amort_oracle intutil addn 2023 7 29 1 133 -> last 1900 7 29",
		},
		{
			// 123 + 132 = 255 -> no wrap. The guard must stay inert one year short.
			name: "A/annual-one-short-of-wrap", y: 2023, m: 7, d: 29, peryr: 1, n: 132,
			wy: 2155, wm: 7, wd: 29, site: "A",
			oracleCmd: "amort_oracle intutil addn 2023 7 29 1 132 -> last 2155 7 29",
		},
		{
			// 123 + 300 = 423 -> 167 -> 2067, via the monthly arm (n = 300 years
			// exactly, so again no AddPeriod remainder).
			name: "A/monthly-3600-periods", y: 2023, m: 7, d: 29, peryr: 12, n: 3600,
			wy: 2067, wm: 7, wd: 29, site: "A",
			oracleCmd: "amort_oracle intutil addn 2023 7 29 12 3600 -> last 2067 7 29",
		},
		{
			// 250 + 5 = 255 (no site-A wrap), then SEVEN AddPeriod steps carry
			// the month past December and `inc(y)` rolls 255 -> 0. Site B only.
			name: "B/addperiod-inc-rolls-255-to-0", y: 2150, m: 6, d: 15, peryr: 12, n: 67,
			wy: 1900, wm: 1, wd: 15, site: "B",
			oracleCmd: "amort_oracle intutil addn 2150 6 15 12 67 -> last 1900 1 15",
		},
		{
			// One period short of the roll — site B must stay inert.
			name: "B/addperiod-inc-one-short", y: 2150, m: 6, d: 15, peryr: 12, n: 66,
			wy: 2155, wm: 12, wd: 15, site: "B",
			oracleCmd: "amort_oracle intutil addn 2150 6 15 12 66 -> last 2155 12 15",
		},
		{
			// Negative n from py=1: nyears = -2 so site A gives -1 -> 255 -> 2155.
			name: "A/negative-n-wraps-below-zero", y: 1901, m: 6, d: 15, peryr: 12, n: -24,
			wy: 2155, wm: 6, wd: 15, site: "A",
			oracleCmd: "amort_oracle intutil addn 1901 6 15 12 -24 -> last 2155 6 15",
		},
		{
			// The 26/52 arm goes through Julian/MDY instead, where DOS's 70000-day
			// ceiling (VIDEODAT.pas:373) already refuses — restored for PV in §47.
			// The year byte must NOT quietly rescue it into a wrapped answer.
			name: "MDY/weekly-past-julian-ceiling", y: 2023, m: 7, d: 29, peryr: 52, n: 5000,
			site:             "MDY",
			oracleCmd:        "amort_oracle intutil addn 2023 7 29 52 5000 -> last 1900 -99 0 (errorbyte)",
			wantErrSubstring: "70000",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := types.NewDateRec(c.y, time.Month(c.m), c.d)
			got, err := AddNPeriods(f, c.peryr, c.n)
			if c.wantErrSubstring != "" {
				if err == nil {
					t.Fatalf("[site %s] AddNPeriods(%v, %d, %d) = %v, want an error containing %q\n  oracle: %s",
						c.site, f.Time.Format("2006-01-02"), c.peryr, c.n,
						got.Time.Format("2006-01-02"), c.wantErrSubstring, c.oracleCmd)
				}
				if !contains(err.Error(), c.wantErrSubstring) {
					t.Fatalf("[site %s] error %q does not mention %q\n  oracle: %s",
						c.site, err.Error(), c.wantErrSubstring, c.oracleCmd)
				}
				return
			}
			if err != nil {
				t.Fatalf("[site %s] AddNPeriods(%v, %d, %d) error: %v\n  oracle: %s",
					c.site, f.Time.Format("2006-01-02"), c.peryr, c.n, err, c.oracleCmd)
			}
			if got.Time.Year() != c.wy || int(got.Time.Month()) != c.wm || got.Time.Day() != c.wd {
				t.Fatalf("[site %s] AddNPeriods(%s, peryr=%d, n=%d) = %04d-%02d-%02d, want %04d-%02d-%02d\n  oracle: %s",
					c.site, f.Time.Format("2006-01-02"), c.peryr, c.n,
					got.Time.Year(), int(got.Time.Month()), got.Time.Day(), c.wy, c.wm, c.wd,
					c.oracleCmd)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestDOSYearByteWrapVsOracle re-derives every §55 golden from the real engine,
// and additionally sweeps the two byte boundaries (py 255->0 and py 0->255) at
// every monthly-family frequency. The goldens above are what CI checks without
// an oracle; this is what proves they are still DOS's answers.
func TestDOSYearByteWrapVsOracle(t *testing.T) {
	if _, err := os.Stat(ydOracleBin()); err != nil {
		t.Skipf("oracle not present (%s); build via legacy/oracle/build_linux.sh", ydOracleBin())
	}

	type probe struct {
		y, m, d  int
		peryr, n int
	}
	var probes []probe
	// The site-A jump, swept across the wrap on both sides.
	for _, n := range []int{130, 131, 132, 133, 134, 250, 299, 300, 388, 389} {
		probes = append(probes, probe{2023, 7, 29, 1, n})
	}
	// The site-B inc/dec boundary at every monthly-family frequency: start one
	// step short of py=255 (or py=0) so the remainder loop does the rolling.
	for _, py := range []int{1, 2, 4, 6, 12} {
		probes = append(probes,
			probe{2155, 11, 15, py, 1},
			probe{2155, 11, 15, py, 2},
			probe{2155, 12, 15, py, 1},
			probe{1900, 1, 15, py, -1},
			probe{1900, 2, 15, py, -2},
		)
	}
	// Semi-monthly has its own month-carry arm (INTSUTIL.pas:1224/1233).
	for _, n := range []int{1, 2, 3, 24, 25, 6120, 6121} {
		probes = append(probes, probe{2155, 12, 15, 24, n}, probe{2023, 7, 29, 24, n})
	}

	checked, mismatched := 0, 0
	for _, p := range probes {
		f := types.NewDateRec(p.y, time.Month(p.m), p.d)
		want, ok := runAddNOracle(t, f, p.peryr, p.n)
		if !ok {
			t.Fatalf("oracle addn failed for %v py=%d n=%d",
				f.Time.Format("2006-01-02"), p.peryr, p.n)
		}
		got, err := AddNPeriods(f, p.peryr, p.n)
		if err != nil {
			// DOS signals an unrepresentable result with errorbyte in the MONTH
			// field, which runAddNOracle turns into a nonsense date; a Go error
			// here is the matching refusal (§51's rule).
			if int(want.Time.Month()) >= 1 && int(want.Time.Month()) <= 12 &&
				want.Time.Year() >= 1900 && want.Time.Year() <= 2155 {
				t.Errorf("AddNPeriods(%s, py=%d, n=%d) refused (%v) but DOS answered %s",
					f.Time.Format("2006-01-02"), p.peryr, p.n, err,
					want.Time.Format("2006-01-02"))
			}
			continue
		}
		checked++
		if got.Time.Year() != want.Time.Year() ||
			got.Time.Month() != want.Time.Month() ||
			got.Time.Day() != want.Time.Day() {
			mismatched++
			t.Errorf("AddNPeriods(%s, py=%d, n=%d): go=%s dos=%s",
				f.Time.Format("2006-01-02"), p.peryr, p.n,
				got.Time.Format("2006-01-02"), want.Time.Format("2006-01-02"))
		}
	}
	t.Logf("§55 year-byte differential: %d compared, %d mismatched", checked, mismatched)
}
