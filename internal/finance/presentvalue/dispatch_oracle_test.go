package presentvalue

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

// Dispatch DIFFERENTIAL vs the real DOS engine. The same field-presence pattern
// is fed to both the Go engine and the genuine DOS Enter dispatch (source-oracle
// `eval` mode) and we compare the observable consequence — SOLVABLE vs REFUSED,
// plus the forward value where both solve. This validates the dispatch DECISION
// by its effect, which is robust to internal-taxonomy differences (DOS treats a
// lump "date+value" row as forward-computing the amount; the Go port labels it a
// PV-1 backward solve — same answer, different label, both solvable).
//
// Scope: the rate+as-of-present region with NO screen Sum Value (cspec "RO").
// There the discount context is well-defined, DOS produces a value only via the
// forward path (so its frontward/backward readback is stable — no screen-sum
// backward calc runs to mutate it), and an invalid screen surfaces cleanly as
// ERR / INSUF / a hard fault. The rate-solve, as-of-solve and screen-Sum-driven
// backward cells are already direct-diffed by the other sweeps in this package.
//
// Build the oracle:  TARGET=pv_oracle legacy/oracle/build_linux.sh

type dosOutcome struct {
	solvable bool
	sum      float64
	raw      string
}

// dosEval runs the oracle `eval` mode. A non-zero process exit (a hard engine
// fault, e.g. an invalid periodic row with no Pmts/Yr) is read as REFUSED — the
// DOS engine could not produce a value for that pattern.
func dosEval(lspec, pspec, cspec string) dosOutcome {
	out, err := exec.Command(pvOracleBin(), "eval", lspec, pspec, cspec).Output()
	raw := strings.TrimSpace(string(out))
	if err != nil {
		return dosOutcome{solvable: false, raw: "FAULT:" + err.Error()}
	}
	if raw == "INSUF" || strings.HasPrefix(raw, "ERR") {
		return dosOutcome{solvable: false, raw: raw}
	}
	f := strings.Fields(raw)
	if len(f) >= 3 && f[0] == "ok" && f[1] == "sum" {
		v, _ := strconv.ParseFloat(f[2], 64)
		return dosOutcome{solvable: true, sum: v, raw: raw}
	}
	return dosOutcome{solvable: false, raw: "UNPARSED:" + raw}
}

// screenFromSpecs builds a Go PVInput from the same specs and the SAME concrete
// values the oracle's SetupClassify uses, so numeric comparison is exact.
func screenFromSpecs(lspec, pspec, cspec string) PVInput {
	in := PVInput{
		PresVal: PresValLine{
			AsOfStatus: present(strings.ContainsRune(cspec, 'O')), AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: present(strings.ContainsRune(cspec, 'R')), Rate: 0.08, PerYr: 1},
			SumValueStatus: present(strings.ContainsRune(cspec, 'S')), SumValue: 900,
		},
		Settings: PVSettings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
	}
	if lspec != "-" {
		in.LumpSums = []LumpSumPayment{{
			DateStatus: present(strings.ContainsRune(lspec, 'D')), Date: types.NewDateRec(2025, time.January, 1),
			AmtStatus: present(strings.ContainsRune(lspec, 'A')), Amt: 1000,
			ValStatus: present(strings.ContainsRune(lspec, 'V')), Val: 900,
		}}
	}
	if pspec != "-" {
		in.Periodics = []PeriodicPayment{{
			FromDateStatus: present(strings.ContainsRune(pspec, 'F')), FromDate: types.NewDateRec(2025, time.January, 1),
			ToDateStatus: present(strings.ContainsRune(pspec, 'T')), ToDate: types.NewDateRec(2030, time.January, 1),
			PerYrStatus: present(strings.ContainsRune(pspec, 'P')), PerYr: 12,
			AmtStatus: present(strings.ContainsRune(pspec, 'A')), Amt: 100,
			ValStatus: present(strings.ContainsRune(pspec, 'V')), Val: 5000,
			COLAStatus: present(strings.ContainsRune(pspec, 'C')), COLA: math.Log(1.03),
		}}
	}
	return in
}

func goSolvable(in PVInput) (bool, float64) {
	r := Calculate(in)
	return r.Err == nil, r.SumValue
}

// hasValueColumn reports whether the single populated row carries its own Value
// column. In the no-screen-Sum region this is the marker for the one documented
// behavioural difference: the Go port accepts a row's own Value as a backward
// solve target (solving the amount or a date from it), whereas DOS only solves
// from the screen Sum Value and so refuses these. Over-specified rows (all three
// columns) also carry V. Any Go-solvable / DOS-refused case WITHOUT a Value
// column would be an unexplained dispatch divergence and fails the test.
func hasValueColumn(lspec, pspec string) bool {
	return strings.ContainsRune(lspec, 'V') || strings.ContainsRune(pspec, 'V')
}

func subsets(letters string) []string {
	var out []string
	n := len(letters)
	for mask := 1; mask < (1 << n); mask++ {
		var b strings.Builder
		for i := 0; i < n; i++ {
			if mask&(1<<i) != 0 {
				b.WriteByte(letters[i])
			}
		}
		out = append(out, b.String())
	}
	return out
}

func TestDOSPVDispatchSolvabilitySweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s); build via TARGET=pv_oracle legacy/oracle/build_linux.sh", pvOracleBin())
	}
	const cspec = "RO" // rate + as-of present, no screen Sum Value

	type cell struct{ l, p string }
	var cells []cell
	for _, l := range subsets("DAV") {
		cells = append(cells, cell{l, "-"})
	}
	for _, p := range subsets("FTPAV") {
		cells = append(cells, cell{"-", p})
	}

	var goSolvesDOSRefuses, dosSolvesGoRefuses, bothSolve, bothRefuse int
	var valueTargetDiffs int
	for _, cl := range cells {
		dos := dosEval(cl.l, cl.p, cspec)
		gS, gSum := goSolvable(screenFromSpecs(cl.l, cl.p, cspec))

		switch {
		case gS && dos.solvable:
			bothSolve++
			// Numeric agreement on pure-forward cells (row fully specified by
			// its forward columns; DOS computes the PV, Go must match).
			if (cl.l == "DA" && cl.p == "-") || (cl.p == "FTPA" && cl.l == "-") {
				if math.Abs(dos.sum-gSum) > 1e-4 {
					t.Errorf("forward value mismatch L=%q P=%q: DOS %.6f Go %.6f", cl.l, cl.p, dos.sum, gSum)
				}
			}
		case !gS && !dos.solvable:
			bothRefuse++
		case gS && !dos.solvable:
			// Allowed ONLY when the row carries its own Value column (the
			// documented row-Value-as-target / over-specified difference).
			if hasValueColumn(cl.l, cl.p) {
				valueTargetDiffs++
			} else {
				goSolvesDOSRefuses++
				t.Errorf("UNEXPECTED: Go solves but DOS refuses, no Value column. L=%q P=%q (DOS: %q)",
					cl.l, cl.p, dos.raw)
			}
		case !gS && dos.solvable:
			// DOS solving something Go refuses is always a real gap.
			dosSolvesGoRefuses++
			t.Errorf("GAP: DOS solves but Go refuses. L=%q P=%q (DOS: %q)", cl.l, cl.p, dos.raw)
		}
	}
	t.Logf("PV dispatch sweep (cspec=RO): %d cells | both-solve %d, both-refuse %d, "+
		"row-Value-target diffs (allowed) %d | UNEXPECTED go>dos %d, dos>go %d",
		len(cells), bothSolve, bothRefuse, valueTargetDiffs, goSolvesDOSRefuses, dosSolvesGoRefuses)
}
