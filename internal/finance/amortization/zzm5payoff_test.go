package amortization

// zzm5payoff_test.go — INVESTIGATION HARNESS for payoff (`payoff=`) divergences.
// Scratch, like zzm5_test.go: not a gate, no assertions of its own.
//
// The amortization fuzzer reports a payoff divergence as a single date and a
// single delta, which is not enough to tell a CONSTANT offset (a rebate applied
// on one side only) from a DRIFTING one (a wrong accrual anchor or rate). This
// sweeps a whole list of payoff dates through both engines and prints them side
// by side, so the shape of the error across the loan is visible at once.
//
//	M5="334211.74 0.0738180000 12 1 b365 prepaid r78 loandmy=17.1.2025 ..." \
//	M5PAYOFF="17.1.2025,17.4.2025,16.7.2025,17.7.2025,18.7.2025,17.1.2026,17.7.2026" \
//	  go test ./internal/finance/amortization/ -run TestM5Payoff -v
//
// M5 takes the oracle argument line exactly as the fuzzer printed it; any
// `payoff=` token already on the line is stripped and folded into the date list,
// so a fuzzer line can be pasted verbatim.

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// m5PayoffOracle asks the oracle for DOS's ComputeBalanceFromDate value on one
// date. Retried like m5Oracle: the oracle has a measured ~5% nondeterministic
// flake rate (see claude/convergence_assessment_2026-07-29b.md).
func m5PayoffOracle(args []string, dmy string) (float64, bool) {
	clean := make([]string, 0, len(args)+1)
	for _, a := range args {
		if a == "bdump" || a == "quiet" || a == "rows" || a == "apr" ||
			strings.HasPrefix(a, "payoff=") {
			continue
		}
		clean = append(clean, a)
	}
	clean = append(clean, "payoff="+dmy)
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, clean...).Output()
		if err != nil || len(out) == 0 {
			continue
		}
		for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			f := strings.Fields(ln)
			if len(f) >= 2 && f[0] == "payoff" {
				v, e := strconv.ParseFloat(f[1], 64)
				if e == nil {
					return v, true
				}
			}
		}
	}
	return 0, false
}

func TestM5Payoff(t *testing.T) {
	gateOracle(t)

	line := os.Getenv("M5")
	if line == "" {
		t.Skip("set M5 to the oracle argument line")
	}

	// Collect payoff dates from M5PAYOFF plus any payoff= tokens on the line.
	var dates []string
	for _, a := range strings.Fields(line) {
		if strings.HasPrefix(a, "payoff=") {
			dates = append(dates, strings.TrimPrefix(a, "payoff="))
		}
	}
	for _, d := range strings.Split(os.Getenv("M5PAYOFF"), ",") {
		if d = strings.TrimSpace(d); d != "" {
			dates = append(dates, d)
		}
	}
	if len(dates) == 0 {
		t.Skip("set M5PAYOFF to a comma-separated list of D.M.Y payoff dates")
	}

	// m5Parse fatals on unrecognised tokens, so payoff= is stripped before parsing.
	var kept []string
	for _, a := range strings.Fields(line) {
		if !strings.HasPrefix(a, "payoff=") {
			kept = append(kept, a)
		}
	}
	in, args := m5Parse(t, strings.Join(kept, " "))

	fmt.Printf("M5PAYOFF sweep — %d dates\n", len(dates))
	worst := 0.0
	for _, dmy := range dates {
		f := strings.Split(dmy, ".")
		if len(f) != 3 {
			t.Fatalf("bad D.M.Y payoff date %q", dmy)
		}
		d, _ := strconv.Atoi(f[0])
		mo, _ := strconv.Atoi(f[1])
		y, _ := strconv.Atoi(f[2])
		asOf := types.NewDateRec(y, time.Month(mo), d)

		dos, ok := m5PayoffOracle(args, dmy)
		if !ok {
			fmt.Printf("%-11s DOS <oracle refused/flaked>\n", dmy)
			continue
		}
		// PayoffBalance takes the screen input by value; hand it a fresh copy
		// each date so no per-date mutation leaks into the next probe.
		got, err := PayoffBalance(in, asOf)
		if err != nil {
			fmt.Printf("%-11s DOS %14.4f | Go ERROR %v\n", dmy, dos, err)
			continue
		}
		delta := got - dos
		if math.Abs(delta) > math.Abs(worst) {
			worst = delta
		}
		fmt.Printf("%-11s DOS %14.4f | Go %14.4f  delta=%+.4f\n", dmy, dos, got, delta)
	}
	fmt.Printf("worst delta=%+.4f\n", worst)
}
