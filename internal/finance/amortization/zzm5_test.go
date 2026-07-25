package amortization

// zzm5_test.go — INVESTIGATION HARNESS for fuzzer5 divergences. Scratch: not a
// gate, not part of the suite's assertions. Every test here is driven by the
// M5 environment variable holding the exact oracle argument line the fuzzer
// printed, e.g.
//
//	M5="36689.67 0.0492820000 42 2 b365 inadv mor=36 b54=869.62 targ=209.91" \
//	  go test ./internal/finance/amortization/ -run TestM5Rows -v
//
// It exists because "totals differ by $X" is not enough to walk the Pascal
// against: what identifies the divergent DOS routine is the FIRST ROW where the
// two schedules part company, and which single option token has to be removed
// before they agree again.

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

type m5Row struct {
	label string
	inter float64
	prin  float64 // principal APPLIED this period (DOS) — see note in m5Oracle
	bal   float64
}

// m5Parse turns an oracle argument line into the Go LoanInput the fuzzer would
// have built for it. It is the inverse of dos_fuzzer5_test.go's generator and
// must stay in step with it: same 2024-01-01 loan date, same addMonths grid,
// same status constants.
func m5Parse(t *testing.T, line string) (LoanInput, []string) {
	t.Helper()
	args := strings.Fields(line)
	if len(args) < 4 {
		t.Fatalf("M5 needs at least AMOUNT RATE N PERYR, got %q", line)
	}
	amount, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		t.Fatalf("amount %q: %v", args[0], err)
	}
	rate, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		t.Fatalf("rate %q: %v", args[1], err)
	}
	n, err := strconv.Atoi(args[2])
	if err != nil {
		t.Fatalf("nperiods %q: %v", args[2], err)
	}
	perYr, err := strconv.Atoi(args[3])
	if err != nil {
		t.Fatalf("peryr %q: %v", args[3], err)
	}

	addMonths := func(months int) types.DateRec {
		return types.NewDateRec(2024+months/12, time.Month(months%12+1), 1)
	}
	// parseDMY decodes the oracle's absolute-date form D.M.Y (loandmy=, firstdmy=,
	// predmy=, adjdmy=, bdate=). The month-offset tokens above are the fuzzer's
	// grammar; the hand-written oracle lines in the regression tests use these.
	parseDMY := func(s string) types.DateRec {
		f := strings.Split(s, ".")
		if len(f) != 3 {
			t.Fatalf("bad D.M.Y %q", s)
		}
		d, _ := strconv.Atoi(f[0])
		m, _ := strconv.Atoi(f[1])
		y, _ := strconv.Atoi(f[2])
		return types.NewDateRec(y, time.Month(m), d)
	}

	basis := types.Basis360
	var loanDate, firstDate types.DateRec
	var solveRate bool
	var exact, prepaid, inadv, plusreg, r78, usa bool
	var balloons []BalloonPayment
	var adjs []RateAdjustment
	var pres []Prepayment
	var mor Moratorium
	var targ Target
	var skips SkipMonths
	points := -1.0
	payhard := -1.0
	paydef := -1.0
	// A DEFAULT payment may legitimately be negative (the DOS moratorium +
	// REPLACE-mode prepay regime — see solveSegmentPayment), so `paydef >= 0`
	// cannot double as "was it supplied?".
	paydefSet := false
	fancy := false

	reBalloon := regexp.MustCompile(`^b(\d+)=(.+)$`)

	for _, a := range args[4:] {
		switch {
		case a == "b365":
			basis = types.Basis365
		case a == "b365_360":
			basis = types.Basis365360
		case a == "exact":
			exact = true
		case a == "prepaid":
			prepaid = true
		case a == "inadv":
			inadv = true
		case a == "plusreg":
			plusreg = true
		case a == "r78":
			r78 = true
		case a == "usa":
			usa = true
		case a == "rows" || a == "bdump" || a == "quiet" || a == "apr" ||
			a == "dumpraw" || a == "solverate" || a == "solveterm":
			// output-mode tokens: no effect on the Go input
		case a == "norate":
			solveRate = true
		case strings.HasPrefix(a, "loandmy="):
			loanDate = parseDMY(strings.TrimPrefix(a, "loandmy="))
		case strings.HasPrefix(a, "firstdmy="):
			firstDate = parseDMY(strings.TrimPrefix(a, "firstdmy="))
		case strings.HasPrefix(a, "bdate="):
			f := strings.SplitN(strings.TrimPrefix(a, "bdate="), ":", 2)
			if len(f) != 2 {
				t.Fatalf("bad bdate= token %q", a)
			}
			amt, _ := strconv.ParseFloat(f[1], 64)
			balloons = append(balloons, BalloonPayment{
				DateStatus: types.InOutInput, Date: parseDMY(f[0]),
				AmountStatus: types.InOutInput, Amount: amt,
			})
			fancy = true
		case strings.HasPrefix(a, "predmy="):
			f := strings.Split(strings.TrimPrefix(a, "predmy="), ":")
			if len(f) != 4 {
				t.Fatalf("bad predmy= token %q", a)
			}
			nn, _ := strconv.Atoi(f[1])
			ppy, _ := strconv.Atoi(f[2])
			amt, _ := strconv.ParseFloat(f[3], 64)
			pres = append(pres, Prepayment{
				StartDateStatus: types.InOutInput, StartDate: parseDMY(f[0]),
				NNStatus: types.InOutInput, NN: nn,
				PerYrStatus: types.InOutInput, PerYr: ppy,
				PaymentStatus: types.InOutInput, Payment: amt,
			})
			fancy = true
		case strings.HasPrefix(a, "adjdmy="):
			f := strings.Split(strings.TrimPrefix(a, "adjdmy="), ":")
			if len(f) != 3 {
				t.Fatalf("bad adjdmy= token %q", a)
			}
			adj := RateAdjustment{DateStatus: types.InOutInput, Date: parseDMY(f[0])}
			if f[1] != "" {
				r, _ := strconv.ParseFloat(f[1], 64)
				adj.LoanRateStatus, adj.LoanRate = types.InOutInput, r
			}
			if f[2] != "" {
				v, _ := strconv.ParseFloat(f[2], 64)
				adj.AmountStatus, adj.Amount, adj.AmtOK = types.InOutInput, v, true
			}
			adjs = append(adjs, adj)
			fancy = true
		case strings.HasPrefix(a, "mor="):
			m, _ := strconv.Atoi(strings.TrimPrefix(a, "mor="))
			mor = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: addMonths(m)}
			fancy = true
		case strings.HasPrefix(a, "targ="):
			v, _ := strconv.ParseFloat(strings.TrimPrefix(a, "targ="), 64)
			targ = Target{TargetStatus: types.InOutInput, TargetValue: v}
			fancy = true
		case strings.HasPrefix(a, "skip="):
			skips = skipSetRaw(strings.TrimPrefix(a, "skip="))
			fancy = true
		case strings.HasPrefix(a, "pts="):
			points, _ = strconv.ParseFloat(strings.TrimPrefix(a, "pts="), 64)
			fancy = true
		case strings.HasPrefix(a, "payhard="):
			payhard, _ = strconv.ParseFloat(strings.TrimPrefix(a, "payhard="), 64)
		case strings.HasPrefix(a, "pay="):
			// defp: DOS carries the payment without the per-period Round2 that
			// `inp` (payhard) triggers, so this is the mode to use when isolating
			// a schedule difference from a rounding difference.
			paydef, _ = strconv.ParseFloat(strings.TrimPrefix(a, "pay="), 64)
			paydefSet = true
		case strings.HasPrefix(a, "pre="):
			f := strings.Split(strings.TrimPrefix(a, "pre="), ":")
			if len(f) != 4 {
				t.Fatalf("bad pre= token %q", a)
			}
			m, _ := strconv.Atoi(f[0])
			nn, _ := strconv.Atoi(f[1])
			ppy, _ := strconv.Atoi(f[2])
			amt, _ := strconv.ParseFloat(f[3], 64)
			pres = append(pres, Prepayment{
				StartDateStatus: types.InOutInput, StartDate: addMonths(m),
				NNStatus: types.InOutInput, NN: nn,
				PerYrStatus: types.InOutInput, PerYr: ppy,
				PaymentStatus: types.InOutInput, Payment: amt,
			})
			fancy = true
		case strings.HasPrefix(a, "adj="):
			f := strings.Split(strings.TrimPrefix(a, "adj="), ":")
			if len(f) != 3 {
				t.Fatalf("bad adj= token %q", a)
			}
			m, _ := strconv.Atoi(f[0])
			adj := RateAdjustment{DateStatus: types.InOutInput, Date: addMonths(m)}
			if f[1] != "" {
				r, _ := strconv.ParseFloat(f[1], 64)
				adj.LoanRateStatus, adj.LoanRate = types.InOutInput, r
			}
			if f[2] != "" {
				v, _ := strconv.ParseFloat(f[2], 64)
				adj.AmountStatus, adj.Amount, adj.AmtOK = types.InOutInput, v, true
			}
			adjs = append(adjs, adj)
			fancy = true
		default:
			if mm := reBalloon.FindStringSubmatch(a); mm != nil {
				m, _ := strconv.Atoi(mm[1])
				amt, _ := strconv.ParseFloat(mm[2], 64)
				balloons = append(balloons, balloonAt(m, amt))
				fancy = true
				continue
			}
			t.Fatalf("unrecognised token %q", a)
		}
	}

	s := gzSettings(perYr, basis, exact, prepaid, inadv, r78, usa)
	s.PlusRegular = plusreg
	in := gzLoanInput(amount, rate, n, perYr, s)
	in.Fancy = fancy
	in.Balloons = balloons
	in.Adjustments = adjs
	in.Prepayments = pres
	in.Moratorium = mor
	in.Target = targ
	in.SkipMonths = skips
	if points >= 0 {
		in.Loan.PointsStatus, in.Loan.Points = types.InOutInput, points
	}
	if payhard >= 0 {
		in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, payhard
	}
	if paydefSet {
		in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutDefault, paydef
	}
	if dateutil.DateOK(loanDate) {
		in.Loan.LoanDateStatus, in.Loan.LoanDate = types.InOutInput, loanDate
	}
	if dateutil.DateOK(firstDate) {
		in.Loan.FirstStatus, in.Loan.FirstDate = types.InOutInput, firstDate
	}
	if solveRate {
		in.Loan.LoanRateStatus, in.Loan.LoanRate = types.StatusEmpty, 0
	}
	return in, args
}

// m5Oracle runs the oracle in `rows` mode. The `prin` column is DOS's
// principal-applied-this-period, and `bal` is the balance AFTER the row.
func m5Oracle(t *testing.T, args []string) ([]m5Row, float64, string) {
	t.Helper()
	clean := make([]string, 0, len(args)+1)
	for _, a := range args {
		if a == "bdump" || a == "quiet" || a == "rows" || a == "apr" {
			continue
		}
		clean = append(clean, a)
	}
	clean = append(clean, "rows")
	var out []byte
	var err error
	for try := 0; try < 8; try++ {
		out, err = exec.Command(oracleBin, clean...).Output()
		if err == nil && len(out) > 0 {
			break
		}
	}
	text := strings.TrimSpace(string(out))
	if strings.HasPrefix(text, "ERR") {
		return nil, 0, strings.SplitN(text, "\n", 2)[0]
	}
	var rows []m5Row
	pay := 0.0
	for _, ln := range strings.Split(text, "\n") {
		f := strings.Fields(ln)
		if len(f) == 0 {
			continue
		}
		if f[0] == "payment" && len(f) > 1 {
			pay, _ = strconv.ParseFloat(f[1], 64)
			continue
		}
		if f[0] != "row" || len(f) < 8 {
			continue
		}
		var r m5Row
		r.label = f[1]
		r.inter, _ = strconv.ParseFloat(f[3], 64)
		r.prin, _ = strconv.ParseFloat(f[5], 64)
		r.bal, _ = strconv.ParseFloat(f[7], 64)
		rows = append(rows, r)
	}
	return rows, pay, ""
}

// m5PreSolveRate mirrors the API's rate pre-solve (FirstPass → SolveRate → write
// the solved rate back as an INPUT field) so a `norate` M5 line behaves the way
// the real handler and the regression tests do. Amortize alone does not solve the
// rate: it would walk the schedule at 0%.
func m5PreSolveRate(t *testing.T, in *LoanInput) {
	t.Helper()
	if in.Loan.LoanRateStatus > types.StatusEmpty {
		return
	}
	si := *in
	sl := in.Loan
	if err := FirstPass(&sl); err != nil {
		t.Fatalf("FirstPass: %v", err)
	}
	if sl.FirstStatus > types.StatusEmpty && sl.FirstStatus < types.InOutDefault {
		sl.FirstStatus = types.InOutDefault
	}
	if sl.NPeriods > 0 && sl.NStatus > types.StatusEmpty && sl.NStatus < types.InOutDefault {
		sl.NStatus = types.InOutDefault
	}
	si.Loan = sl
	solved, _, err := SolveRate(si)
	if err != nil {
		t.Fatalf("SolveRate: %v", err)
	}
	t.Logf("Go pre-solved rate %.10f", solved)
	in.Loan.LoanRateStatus = types.InOutInput
	in.Loan.LoanRate = solved
}

// TestM5Rows prints DOS's schedule beside Go's and names the first row where
// they part company. This is the input to a Pascal walk.
func TestM5Rows(t *testing.T) {
	gateOracle(t)
	line := os.Getenv("M5")
	if line == "" {
		t.Skip("set M5=<oracle arg line>")
	}
	in, args := m5Parse(t, line)
	m5PreSolveRate(t, &in)
	dosRows, dosPay, dosErr := m5Oracle(t, args)
	if dosErr != "" {
		t.Logf("DOS refused: %s", dosErr)
	}
	gr := Amortize(in)
	if gr.Err != nil {
		t.Logf("Go refused: %v", gr.Err)
	}
	t.Logf("DOS payment %.4f rows %d | Go rows %d totalInt %.2f totalPaid %.2f finalPrinc %.2f",
		dosPay, len(dosRows), len(gr.Schedule), gr.TotalInt, gr.TotalPaid, gr.FinalPrinc)

	nShow, _ := strconv.Atoi(os.Getenv("M5N"))
	if nShow == 0 {
		nShow = 12
	}
	first := -1
	max := len(dosRows)
	if len(gr.Schedule) > max {
		max = len(gr.Schedule)
	}
	for i := 0; i < max; i++ {
		var d m5Row
		var okD bool
		if i < len(dosRows) {
			d, okD = dosRows[i], true
		}
		var g PaymentRecord
		var okG bool
		if i < len(gr.Schedule) {
			g, okG = gr.Schedule[i], true
		}
		bad := !okD || !okG ||
			math.Abs(d.inter-g.Interest) > 0.011 || math.Abs(d.bal-g.Principal) > 0.011
		if bad && first < 0 {
			first = i
		}
		if first >= 0 && i < first+nShow {
			ds, gs := "        (none)", "        (none)"
			if okD {
				ds = fmt.Sprintf("%-10s int %12.2f prin %12.2f bal %14.2f", d.label, d.inter, d.prin, d.bal)
			}
			if okG {
				gs = fmt.Sprintf("#%-4d %s pay %11.2f int %12.2f bal %14.2f",
					g.PayNum, g.Date.Time.Format("2006-01-02"), g.PayAmt, g.Interest, g.Principal)
			}
			t.Logf("  [%3d] DOS %s\n        GO  %s", i, ds, gs)
		}
	}
	if first < 0 {
		t.Logf("schedules AGREE row-for-row (%d rows)", len(dosRows))
	} else {
		t.Logf("FIRST DIVERGENCE at row index %d of %d DOS / %d Go", first, len(dosRows), len(gr.Schedule))
	}
}

// m5Totals runs one oracle line quietly and returns interest/paid.
func m5Totals(args []string) (inter, paid float64, errMsg string) {
	clean := make([]string, 0, len(args))
	for _, a := range args {
		if a == "bdump" || a == "rows" || a == "apr" {
			continue
		}
		clean = append(clean, a)
	}
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, clean...).Output()
		if err != nil || len(out) == 0 {
			continue
		}
		text := strings.TrimSpace(string(out))
		if strings.HasPrefix(text, "ERR") {
			return 0, 0, strings.SplitN(text, "\n", 2)[0]
		}
		for _, ln := range strings.Split(text, "\n") {
			f := strings.Fields(ln)
			if len(f) > 0 && f[0] == "payment" {
				var p, in, pd float64
				for i := 0; i+1 < len(f); i++ {
					v, e := strconv.ParseFloat(f[i+1], 64)
					if e != nil {
						continue
					}
					switch f[i] {
					case "payment":
						p = v
					case "interest":
						in = v
					case "paid":
						pd = v
					}
				}
				if pd <= 0 || in == -1 || p == 0 {
					continue
				}
				return in, pd, ""
			}
		}
	}
	return 0, 0, "flake"
}

// TestM5Ablate removes ONE option token at a time and reports whether the
// divergence survives. The smallest surviving screen is the one to walk.
func TestM5Ablate(t *testing.T) {
	gateOracle(t)
	line := os.Getenv("M5")
	if line == "" {
		t.Skip("set M5=<oracle arg line>")
	}
	_, args := m5Parse(t, line)

	optIdx := []int{}
	for i := 4; i < len(args); i++ {
		a := args[i]
		if a == "bdump" || a == "rows" || a == "quiet" || a == "apr" {
			continue
		}
		optIdx = append(optIdx, i)
	}

	run := func(sub []string) string {
		in, _ := m5Parse(t, strings.Join(sub, " "))
		dInt, dPaid, e := m5Totals(sub)
		if e != "" {
			return fmt.Sprintf("DOS %s", e)
		}
		gr := Amortize(in)
		if gr.Err != nil {
			return fmt.Sprintf("DOS int=%.2f paid=%.2f | Go REFUSED: %v", dInt, dPaid, gr.Err)
		}
		return fmt.Sprintf("DOS int=%12.2f paid=%12.2f | Go int=%12.2f paid=%12.2f | dInt=%12.2f",
			dInt, dPaid, gr.TotalInt, gr.TotalPaid, gr.TotalInt-dInt)
	}

	t.Logf("FULL   %s", run(args))
	for _, drop := range optIdx {
		sub := make([]string, 0, len(args)-1)
		for i, a := range args {
			if i == drop {
				continue
			}
			sub = append(sub, a)
		}
		t.Logf("-%-22s %s", args[drop], run(sub))
	}
}
