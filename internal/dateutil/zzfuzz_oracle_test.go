package dateutil

import (
	"fmt"
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

func oracleBin() string {
	if p := os.Getenv("PERSENSE_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/amort_oracle"
}

func oracleYearsDif(y1, m1, d1, y2, m2, d2 int) (float64, bool) {
	out, err := exec.Command(oracleBin(), "intutil", "yearsdif",
		strconv.Itoa(y1), strconv.Itoa(m1), strconv.Itoa(d1),
		strconv.Itoa(y2), strconv.Itoa(m2), strconv.Itoa(d2)).Output()
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// TestFuzzYearsDifVsDOS compares dateutil.YearsDif (30/360) to the real DOS
// YearsDif across a broad random sweep plus targeted Y2K / leap-day / century
// boundary dates (1900, 2000, 2100 — where Pascal's leap-year convention has
// its known quirks).
func TestFuzzYearsDifVsDOS(t *testing.T) {
	if _, err := os.Stat(oracleBin()); err != nil {
		t.Skip("oracle not present")
	}

	type ymd struct{ y, m, d int }
	checked, fails, maxAbs := 0, 0, 0.0

	check := func(a, b ymd) {
		// Valid day-of-month only.
		if a.d > daysInMonthPascal(a.m, pascalYear(a.y)) || b.d > daysInMonthPascal(b.m, pascalYear(b.y)) {
			return
		}
		ora, ok := oracleYearsDif(a.y, a.m, a.d, b.y, b.m, b.d)
		if !ok {
			return
		}
		checked++
		// Oracle computes YearsDif(d1, d2); Go YearsDif(z, a) = z - a, so z=d1, a=d2.
		z := types.NewDateRec(a.y, time.Month(a.m), a.d)
		ad := types.NewDateRec(b.y, time.Month(b.m), b.d)
		got := YearsDif(z, ad, types.Basis360, 1.0/360.0, false)
		diff := math.Abs(ora - got)
		if diff > maxAbs {
			maxAbs = diff
		}
		if diff > 1e-9 {
			fails++
			if fails <= 25 {
				t.Errorf("YearsDif(%v,%v): DOS=%.12f Go=%.12f (|d| %.2e)", a, b, ora, got, diff)
			}
		}
	}

	// Targeted boundary dates.
	bounds := []ymd{
		{1900, 1, 1}, {1900, 2, 28}, {1900, 3, 1}, {1900, 12, 31},
		{1999, 12, 31}, {2000, 1, 1}, {2000, 2, 29}, {2000, 3, 1},
		{2024, 2, 29}, {2024, 3, 31}, {2100, 2, 28}, {2100, 3, 1},
		{2155, 12, 31}, {1996, 2, 29}, {2001, 1, 31},
	}
	for _, a := range bounds {
		for _, b := range bounds {
			check(a, b)
		}
	}

	// Broad random sweep across the representable range.
	rng := rand.New(rand.NewSource(0x59656172))
	for i := 0; i < 6000; i++ {
		mk := func() ymd {
			return ymd{1900 + rng.Intn(255), 1 + rng.Intn(12), 1 + rng.Intn(31)}
		}
		check(mk(), mk())
	}
	t.Logf("YearsDif vs DOS: checked %d, divergences %d, maxAbs=%.2e", checked, fails, maxAbs)
	_ = fmt.Sprint
}
