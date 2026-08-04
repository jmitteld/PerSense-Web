package amortization

// §62 — THE LAST PAYMENT ON A CLAMPED FEBRUARY (round 21, 2026-08-03).
//
// The engine bounded its table by `dateutil.NumberOfInstallments(firstDate,
// lastDate, ...)`. That routine is a correct port of DOS's `noi`
// (INTSUTIL.pas:936) — and DOS does not use it to bound a table. DOS walks its
// own payment grid and stops on `DateComp(WhenToStop^.date, stopdate) >= 0`
// (AMORTOP.pas:1221). The two answers come apart exactly when the last payment
// date was CLAMPED, because a clamped date is ON the grid but is short of a
// whole period from the first date:
//
//	amort_oracle intutil noi 2024 3 29 2025 2 28 12 on_or_before
//	  -> n 11 last 2025 1 29          <- the date-difference answer
//	DOS's own table for that screen    -> 12 rows
//
// so the port emitted 11 rows, never charged the twelfth period's interest, and
// folded its principal into row 11. A ROUTINE THAT IS FAITHFUL AT AN UNFAITHFUL
// SITE IS A DEFECT — §59's lesson, one round later, in a different routine.
//
// The class is ordinary and entirely inside the client's 2099 scope: any loan
// anchored on the 29th/30th/31st whose LAST payment lands in a February.
//
// This test pins three things, and each is a different failure:
//
//  1. the row count and the totals against the live oracle, on the smallest
//     repro (n=4 semiannual) and on a long in-scope one (n=146, last 2097);
//  2. the two boundary cases the bound has ever been about — §55's year-byte
//     wrap and §54's Feb-2100 crossing — because the fix replaced the counter
//     they were built for;
//  3. the counter itself, `installmentsOnPaymentGrid`, at field level, so a
//     future change to it fails HERE rather than in a totals comparison twelve
//     mechanisms downstream.
//
// Verified against the unfixed tree (round 21), in fact and not in principle:
// A_smallest_repro, A_monthly and B_long_in_scope all FAIL there on both the row
// count and the total interest (-2387.57, -130.05 and -318.72), while every C
// subtest passes before AND after — which is the whole point of C, since the fix
// replaced the counter those two boundary cases were built around.
// TestSec62InstallmentsOnPaymentGrid cannot be run against the unfixed tree at
// all without shimming the new counter into it, because the counter IS the fix;
// it guards future changes, not this one.

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

type sec62Case struct {
	name                  string
	amount, rate          float64
	n, perYr              int
	loanY, loanM, loanD   int
	firstY, firstM, first int
	extra                 []string
}

func sec62RunOracle(t *testing.T, bin string, c sec62Case) (rows int, pay, interest, paid float64) {
	t.Helper()
	args := []string{
		strconv.FormatFloat(c.amount, 'f', 2, 64),
		strconv.FormatFloat(c.rate, 'f', 6, 64),
		strconv.Itoa(c.n), strconv.Itoa(c.perYr),
		"loandmy=" + strconv.Itoa(c.loanD) + "." + strconv.Itoa(c.loanM) + "." + strconv.Itoa(c.loanY),
		"firstdmy=" + strconv.Itoa(c.first) + "." + strconv.Itoa(c.firstM) + "." + strconv.Itoa(c.firstY),
	}
	args = append(args, c.extra...)
	out, err := exec.Command(bin, append(args, "rows")...).Output()
	if err != nil {
		t.Fatalf("%s: oracle rows failed: %v", c.name, err)
	}
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(ln, "row ") {
			rows++
		}
	}
	out2, err := exec.Command(bin, args...).Output()
	if err != nil {
		t.Fatalf("%s: oracle quiet failed: %v", c.name, err)
	}
	for _, ln := range strings.Split(string(out2), "\n") {
		f := strings.Fields(ln)
		if len(f) >= 6 && f[0] == "payment" && f[2] == "interest" && f[4] == "paid" {
			pay, _ = strconv.ParseFloat(f[1], 64)
			interest, _ = strconv.ParseFloat(f[3], 64)
			paid, _ = strconv.ParseFloat(f[5], 64)
		}
	}
	t.Logf("%s: oracle command\n  amort_oracle %s", c.name, strings.Join(args, " "))
	return
}

func sec62RunGo(t *testing.T, c sec62Case) AmortResult {
	t.Helper()
	s := Settings{Basis: types.Basis360, PerYr: byte(c.perYr), YrDays: 360, YrInv: 1.0 / 360}
	for _, e := range c.extra {
		if e == "exact" {
			s.Exact = true
		}
	}
	in := gzLoanInput(c.amount, c.rate, c.n, c.perYr, s)
	in.Loan.LoanDate = types.NewDateRec(c.loanY, time.Month(c.loanM), c.loanD)
	in.Loan.FirstDate = types.NewDateRec(c.firstY, time.Month(c.firstM), c.first)
	return Amortize(in)
}

func TestSec62LastPaymentOnClampedFebruary(t *testing.T) {
	bin := oracleBin
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("oracle required but not present (%s)", bin)
		}
		t.Skipf("amort oracle not present (%s)", bin)
	}

	cases := []struct {
		group string
		c     sec62Case
	}{
		// A — the defect, smallest form. Loan anchored on a leap day, semiannual,
		// last payment 28 Feb 2026 (day 29 clamped). Four rows, not three.
		{"A_smallest_repro", sec62Case{
			name:   "n=4 semiannual, last 28 Feb 2026",
			amount: 250000.00, rate: 0.0725, n: 4, perYr: 2,
			loanY: 2024, loanM: 2, loanD: 29, firstY: 2024, firstM: 8, first: 29}},
		// A2 — monthly, the commonest shape a user would actually enter.
		{"A_monthly", sec62Case{
			name:   "n=12 monthly, last 28 Feb 2025",
			amount: 250000.00, rate: 0.0725, n: 12, perYr: 12,
			loanY: 2024, loanM: 2, loanD: 29, firstY: 2024, firstM: 3, first: 29}},
		// B — the same defect at a long IN-SCOPE horizon (last payment 2097),
		// which is where round 21's probe found it.
		{"B_long_in_scope", sec62Case{
			name:   "n=146 semiannual, last 28 Feb 2097",
			amount: 250000.00, rate: 0.0725, n: 146, perYr: 2,
			loanY: 2024, loanM: 2, loanD: 29, firstY: 2024, firstM: 8, first: 29}},
		// C1 — §55's year-byte wrap: the case the replaced counter existed for.
		// Must be unchanged by the fix.
		{"C_wrap_unchanged", sec62Case{
			name:   "n=7200 semi-monthly, horizon wraps mod 256",
			amount: 421052.18, rate: 0.047119, n: 7200, perYr: 24,
			loanY: 2029, loanM: 4, loanD: 15, firstY: 2029, firstM: 5, first: 15}},
		// C2 — §54's Feb-2100 crossing, named in the replaced counter's own
		// comment as the case a per-row bound got wrong. The raw-field counter
		// gets it right, which is round 21's §54 result.
		{"C_feb2100_crossing", sec62Case{
			name:   "n=1080 monthly exact, day-29 grid across 1 Mar 2100",
			amount: 114948.20, rate: 0.025189, n: 1080, perYr: 12,
			loanY: 2029, loanM: 4, loanD: 29, firstY: 2029, firstM: 5, first: 29,
			extra: []string{"exact"}}},
		// C3 — the matched control the probe used: day 28 never touches a clamp.
		{"C_control_day28", sec62Case{
			name:   "n=146 semiannual, day 28, no clamp anywhere",
			amount: 250000.00, rate: 0.0725, n: 146, perYr: 2,
			loanY: 2024, loanM: 2, loanD: 28, firstY: 2024, firstM: 8, first: 28}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.group, func(t *testing.T) {
			wantRows, wantPay, wantInt, wantPaid := sec62RunOracle(t, bin, tc.c)
			got := sec62RunGo(t, tc.c)
			if got.Err != nil {
				t.Fatalf("%s: port refused where DOS produced %d rows: %v",
					tc.c.name, wantRows, got.Err)
			}
			gotRows := 0
			for _, r := range got.Schedule {
				if r.PayNum >= 1 {
					gotRows++
				}
			}
			if gotRows != wantRows {
				t.Errorf("%s: ROW COUNT — DOS %d, port %d. A dropped final row means "+
					"its interest is never charged and its principal is folded into the "+
					"row before it.", tc.c.name, wantRows, gotRows)
			}
			// Totals carry the consequence, and they are what a user sees.
			if d := got.TotalInt - wantInt; d > 0.01 || d < -0.01 {
				t.Errorf("%s: TOTAL INTEREST — DOS %.2f, port %.2f (delta %.2f)",
					tc.c.name, wantInt, got.TotalInt, d)
			}
			if d := got.TotalPaid - wantPaid; d > 0.01 || d < -0.01 {
				t.Errorf("%s: TOTAL PAID — DOS %.2f, port %.2f (delta %.2f)",
					tc.c.name, wantPaid, got.TotalPaid, d)
			}
			_ = wantPay
			t.Logf("%s: rows %d, interest %.2f, paid %.2f — agree",
				tc.c.name, gotRows, got.TotalInt, got.TotalPaid)
		})
	}
}

// TestSec62InstallmentsOnPaymentGrid pins the counter itself, at field level.
// The totals tests above would catch a regression here too, but only after
// twelve mechanisms have had a chance to obscure it.
func TestSec62InstallmentsOnPaymentGrid(t *testing.T) {
	type row struct {
		name                      string
		fy, fm, fd, ly, lm, ld    int
		perYr, origDay, cap, want int
	}
	rows := []row{
		// The defect: 28 Feb 2025 IS the twelfth payment of a day-29 monthly
		// grid starting 29 Mar 2024. DOS's `noi` says 11 (a date DIFFERENCE);
		// the grid says 12, and DOS's own table emits 12.
		{"clamped_feb_monthly", 2024, 3, 29, 2025, 2, 28, 12, 29, 12, 12},
		{"clamped_feb_semiannual", 2024, 8, 29, 2026, 2, 28, 2, 29, 4, 4},
		// No clamp anywhere: grid and date-difference agree.
		{"day28_no_clamp", 2024, 8, 28, 2026, 2, 28, 2, 28, 4, 4},
		// The horizon is short of the term — this is the case the bound exists
		// for, and it must still bite.
		{"horizon_shorter_than_term", 2024, 1, 15, 2025, 1, 15, 12, 15, 60, 13},
		// A last date BEFORE the first payment is not a bound, it is nonsense:
		// return "no opinion" (0) so the caller keeps its period count rather
		// than truncating the schedule to nothing.
		{"last_before_first", 2024, 6, 15, 2024, 1, 15, 12, 15, 60, 0},
		// 29 Feb 2100 is a real grid point for DOS and an unrepresentable one
		// for types.DateRec. Counting on raw fields gets DOS's answer: the
		// twelfth payment of a day-29 monthly grid from 29 Mar 2099 is
		// 29 Feb 2100, and it is on or before 1 Mar 2100.
		{"feb_2100_grid_point", 2099, 3, 29, 2100, 3, 1, 12, 29, 12, 12},
	}
	for _, r := range rows {
		got := installmentsOnPaymentGrid(
			types.NewDateRec(r.fy, time.Month(r.fm), r.fd),
			types.NewDateRec(r.ly, time.Month(r.lm), r.ld),
			r.perYr, r.origDay, r.cap)
		if got != r.want {
			t.Errorf("%s: installmentsOnPaymentGrid = %d, want %d", r.name, got, r.want)
		}
	}
}
