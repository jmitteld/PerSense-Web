package dateutil

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Boundary differential of YearsDif (30/360 day-count) against the REAL DOS
// INTSUTIL YearsDif via amort_oracle `intutil`. The 30/360 convention has
// specific rules for month-end days (31 → 30) that are easy to mis-port, so this
// sweeps month boundaries, leap years, reversed dates and long spans.

func nd(y int, m time.Month, d int) types.DateRec {
	return types.NewDateRec(y, m, d)
}

func ydOracleBin() string {
	if p := os.Getenv("PERSENSE_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/amort_oracle"
}

func runYearsDifOracle(z, a types.DateRec) (float64, bool) {
	args := []string{"intutil", "yearsdif",
		strconv.Itoa(z.Time.Year()), strconv.Itoa(int(z.Time.Month())), strconv.Itoa(z.Time.Day()),
		strconv.Itoa(a.Time.Year()), strconv.Itoa(int(a.Time.Month())), strconv.Itoa(a.Time.Day())}
	out, err := exec.Command(ydOracleBin(), args...).Output()
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(out))
	if strings.HasPrefix(s, "ERR") {
		return 0, false
	}
	v, e := strconv.ParseFloat(s, 64)
	return v, e == nil
}

func TestDOSYearsDifSweep(t *testing.T) {
	if _, err := os.Stat(ydOracleBin()); err != nil {
		t.Skipf("oracle not present (%s); build via legacy/oracle/build_linux.sh", ydOracleBin())
	}

	// Hand-picked 30/360 boundary cases (z = later date, a = earlier date).
	fixed := []struct{ z, a types.DateRec }{
		{nd(2030, 1, 1), nd(2024, 1, 1)},   // whole years
		{nd(2024, 3, 1), nd(2024, 1, 15)},  // partial, mid-month
		{nd(2024, 1, 31), nd(2024, 1, 1)},  // day 31 start-of-month
		{nd(2024, 3, 31), nd(2024, 1, 31)}, // both month-end (31)
		{nd(2024, 2, 29), nd(2023, 2, 28)}, // leap-year Feb-end
		{nd(2025, 2, 28), nd(2024, 2, 29)}, // across a leap boundary
		{nd(2000, 3, 1), nd(1999, 12, 31)}, // century boundary, Dec 31
		{nd(2100, 1, 1), nd(2000, 1, 1)},   // 100-year span
		{nd(2024, 1, 1), nd(2030, 1, 1)},   // reversed → negative
		{nd(2024, 12, 31), nd(2024, 1, 1)}, // year-end
	}
	fails, maxd := 0, 0.0
	for _, c := range fixed {
		dos, ok := runYearsDifOracle(c.z, c.a)
		if !ok {
			t.Fatalf("oracle yearsdif failed for %v,%v", c.z.Time, c.a.Time)
		}
		got := YearsDif(c.z, c.a, types.Basis360, 1.0/360, false)
		d := math.Abs(dos - got)
		if d > maxd {
			maxd = d
		}
		if d > 1e-9 {
			fails++
			t.Errorf("YearsDif(%v,%v): DOS=%.10f Go=%.10f", c.z.Time, c.a.Time, dos, got)
		}
	}

	// Randomized sweep across a wide date range including all month-ends.
	rng := rand.New(rand.NewSource(20260711))
	randDate := func() types.DateRec {
		y := 1950 + rng.Intn(180)
		m := 1 + rng.Intn(12)
		// bias toward month-end days where 30/360 rules bite
		var d int
		switch rng.Intn(3) {
		case 0:
			d = daysInGregMonth(y, m) // last day
		case 1:
			d = 1
		default:
			d = 1 + rng.Intn(28)
		}
		return nd(y, time.Month(m), d)
	}
	checked := 0
	for i := 0; i < 600; i++ {
		z, a := randDate(), randDate()
		dos, ok := runYearsDifOracle(z, a)
		if !ok {
			continue
		}
		got := YearsDif(z, a, types.Basis360, 1.0/360, false)
		checked++
		d := math.Abs(dos - got)
		if d > maxd {
			maxd = d
		}
		if d > 1e-9 {
			fails++
			if fails <= 12 {
				t.Errorf("YearsDif(%v,%v): DOS=%.10f Go=%.10f", z.Time, a.Time, dos, got)
			}
		}
	}
	t.Logf("YearsDif 30/360 sweep: %d fixed + %d random, fails %d, max |err|=%.2e", len(fixed), checked, fails, maxd)
}

// daysInGregMonth returns the last day of a Gregorian month (for fixture dates).
func daysInGregMonth(y, m int) int {
	return time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
