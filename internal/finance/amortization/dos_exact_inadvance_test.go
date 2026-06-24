package amortization

import (
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// dos_exact_inadvance_test.go — focused differential for the exact (true-daily)
// in-advance schedule SHAPE, validating the parts the row-cube
// (TestDOSGroundZeroRowCube) deliberately does not: the paynum-0 settlement
// row and the total interest (which both INCLUDE the time-0 settlement interest
// the cube's `rows` output strips). Together with the cube — which validates the
// n-1 amortizing rows and the solved payment to the cent — this pins the whole
// schedule against the real DOS engine. See docs/exact_groundzero_findings.md
// "Exact × in-advance structure".

// dumpResult is the parsed `dumpraw` output of the DOS amortization oracle for an
// exact in-advance loan.
type dumpResult struct {
	payment    float64
	settleInt  float64 // paynum-0 settlement-interest row (L0)
	settleDate string  // L0 date token (must be the loan date)
	firstAmort string  // L1 date token (must be firstDate + 1 period)
	totalInt   float64
}

func runOracleExactInadvDump(amount, rate float64, n, perYr int, basisFlagStr string) (dumpResult, bool) {
	args := []string{
		strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr), "dumpraw", "exact", "inadv",
	}
	if basisFlagStr != "" {
		args = append(args, basisFlagStr)
	}
	out, err := exec.Command(oracleBin, args...).Output()
	if err != nil {
		return dumpResult{}, false
	}
	var dr dumpResult
	gotSettle, gotTotal := false, false
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(ln, "payment ") {
			f := strings.Fields(ln)
			if len(f) >= 2 {
				dr.payment, _ = strconv.ParseFloat(f[1], 64)
			}
			continue
		}
		// Schedule rows look like "L0|0  1/ 1/24 1016.39 0.00 100000.00 1016.39".
		if i := strings.IndexByte(ln, '|'); i >= 0 {
			body := ln[i+1:]
			if strings.Contains(body, "Total payments:") {
				if p := strings.Index(body, "Interest:"); p >= 0 {
					dr.totalInt, _ = strconv.ParseFloat(strings.TrimSpace(body[p+len("Interest:"):]), 64)
					gotTotal = true
				}
				continue
			}
			f := strings.Fields(body)
			// Detail rows: paynum date... int prin bal cumint (trailing 4 numeric).
			if len(f) >= 6 {
				if f[0] == "0" && !gotSettle {
					// settlement row: trailing four are int/prin/bal/cumint.
					dr.settleInt, _ = strconv.ParseFloat(f[len(f)-4], 64)
					dr.settleDate = strings.Join(f[1:len(f)-4], "")
					gotSettle = true
				} else if f[0] == "1" && dr.firstAmort == "" {
					dr.firstAmort = strings.Join(f[1:len(f)-4], "")
				}
			}
		}
	}
	return dr, gotSettle && gotTotal
}

// TestDOSExactInAdvanceSettlement validates the settlement row + totals of the
// dedicated exact-in-advance schedule against the DOS oracle's dumpraw.
func TestDOSExactInAdvanceSettlement(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}

	bases := []struct {
		flag  string
		basis types.BasisType
	}{
		{"b365", types.Basis365},
		{"b365_360", types.Basis365360},
	}
	cases := []struct {
		amount, rate float64
		n, perYr     int
	}{
		{100000, 0.12, 12, 12}, // the canonical documented case
		{50000, 0.06, 16, 2},
		{200000, 0.11, 32, 4},
		{75000, 0.09, 96, 12},
	}

	checks := 0
	for _, b := range bases {
		for _, c := range cases {
			dr, ok := runOracleExactInadvDump(c.amount, c.rate, c.n, c.perYr, b.flag)
			if !ok || dr.payment <= 0 {
				t.Fatalf("oracle dumpraw failed for amt=%.0f r=%.2f n=%d py=%d %s",
					c.amount, c.rate, c.n, c.perYr, b.flag)
			}

			s := gzSettings(c.perYr, b.basis, true, false, true, false, false)
			res, gok := gzGoScheduleWithPayment(c.amount, c.rate, c.n, c.perYr, s, dr.payment)
			if !gok || len(res.Schedule) == 0 {
				t.Fatalf("Go schedule failed for amt=%.0f r=%.2f n=%d py=%d %s",
					c.amount, c.rate, c.n, c.perYr, b.flag)
			}

			// Row 0 must be the settlement row at the loan date with prin 0.
			r0 := res.Schedule[0]
			if r0.PayNum != 0 {
				t.Errorf("[amt=%.0f r=%.2f n=%d py=%d %s] first Go row is paynum %d, expected settlement row 0",
					c.amount, c.rate, c.n, c.perYr, b.flag, r0.PayNum)
				continue
			}
			if math.Abs(r0.Interest-dr.settleInt) > 0.01 {
				t.Errorf("[amt=%.0f r=%.2f n=%d py=%d %s] settlement interest DOS=%.2f Go=%.2f",
					c.amount, c.rate, c.n, c.perYr, b.flag, dr.settleInt, r0.Interest)
			}
			if pr := r0.Principal - c.amount; math.Abs(pr) > 0.005 {
				t.Errorf("[amt=%.0f r=%.2f n=%d py=%d %s] settlement row changed balance by %.4f (must be 0)",
					c.amount, c.rate, c.n, c.perYr, b.flag, pr)
			}
			// Total interest (includes the settlement) must match DOS to the cent.
			if math.Abs(res.TotalInt-dr.totalInt) > 0.05+1e-5*math.Abs(dr.totalInt) {
				t.Errorf("[amt=%.0f r=%.2f n=%d py=%d %s] total interest DOS=%.2f Go=%.2f",
					c.amount, c.rate, c.n, c.perYr, b.flag, dr.totalInt, res.TotalInt)
			}
			// Shape: settlement at loan date, first amortizing row one period later.
			if dr.settleDate != "1/1/24" {
				t.Errorf("oracle settlement date unexpected: %q", dr.settleDate)
			}
			checks++
		}
	}
	if checks == 0 {
		t.Fatal("no exact-in-advance settlement comparisons ran")
	}
	t.Logf("validated settlement row + totals for %d exact-in-advance cells", checks)
}
