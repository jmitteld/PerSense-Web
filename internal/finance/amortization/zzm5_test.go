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
	var solveTerm bool
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

	// ---- loan-date PRE-SCAN ----
	// The month-offset option tokens (`b<N>=`, `pre=`, `adj=`, `mor=`) are
	// anchored on the LOAN DATE, exactly as the oracle driver anchors them:
	//
	//	tot := (h^.loandate.m - 1) + monthsVal;
	//	date.d := h^.loandate.d;
	//	date.m := (tot mod 12) + 1;
	//	date.y := h^.loandate.y + (tot div 12);
	//
	// (amort_oracle.pas:172-176, :254-258, :310-314 — note there is NO clamping
	// of the day of month.) So `loandmy=` has to be resolved BEFORE any of those
	// tokens is expanded, which means a pre-scan: the tokens arrive in whatever
	// order the fuzzer printed them, and on the failing lines `loandmy=` sits
	// after some option tokens and before others.
	//
	// 2026-07-25: this closure used to be hardcoded to 2024-01-01, from the era
	// when no M5 line ever carried `loandmy=`. Once fuzzer5 grew a loan-date
	// axis that made M5 irreproducible against the fuzzer — the Go side pinned
	// every option row to 1 January 2024 while the loan itself moved, so M5
	// reported a completely different delta than the run that found the case
	// (seed 20102: fuzzer dInt=+1360.26, M5 dInt=-28969.36). This is the exact
	// mirror of the ordering bug fixed in amort_oracle.pas the same day, and it
	// is worth stating plainly: BOTH sides of a differential rig have to anchor
	// their option dates on the same loan date, or the rig compares two
	// different screens and every verdict it issues is noise.
	for _, a := range args[4:] {
		if strings.HasPrefix(a, "loandmy=") {
			loanDate = parseDMY(strings.TrimPrefix(a, "loandmy="))
		}
	}
	anchor := loanDate
	if !dateutil.DateOK(anchor) {
		// SetupLoan's default (amort_oracle.pas) — 1 January 2024.
		anchor = types.NewDateRec(2024, time.January, 1)
	}
	// The day of month is carried across and then CLAMPED — DOS's own rule for
	// moving a date by whole months (CheckForDaysTooLarge, VIDEODAT.pas:349-354:
	// `last:=DaysInM(f); if (f.d>last) then f.d:=last;`), which every one of the
	// oracle's eleven option-date blocks calls. The clamp has to happen on the
	// RAW day, before types.NewDateRec, because NewDateRec goes through
	// time.Date and NORMALISES: 30 Feb 2026 becomes 2 March 2026 rather than
	// 28 February 2026.
	//
	// 2026-07-28: this closure normalised, and so built a DIFFERENT screen than
	// the oracle for every option date landing in a short month. On
	// `115239.23 0.0367770000 132 12 b365_360 exact loandmy=30.6.2025
	// firstdmy=30.7.2025 adj=8:0.0637620000:` the oracle clamps to 28/2/2026,
	// which Amortize.pas:263's on_or_before snap then pulls back to 30/1/2026;
	// M5 normalised to 2/3/2026 and snapped to 28/2/2026 — i.e. it compared two
	// different adjustments and reported dInt=-273.70 against an engine that
	// actually agrees. Same class as the loan-date anchor mismatch described
	// above: BOTH sides of a differential rig must build the same screen.
	// dos_fuzzer5_test.go's addMonthsFrom already clamped; this is its mirror.
	addMonths := func(months int) types.DateRec {
		tot := (int(anchor.Time.Month()) - 1) + months
		y, m := anchor.Time.Year()+tot/12, time.Month(tot%12+1)
		d := anchor.Time.Day()
		if last := dateutil.DaysInM(types.NewDateRec(y, m, 1)); d > last {
			d = last
		}
		return types.NewDateRec(y, m, d)
	}

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
		case a == "noterm":
			// `noterm` blanks BOTH the period count and the last date
			// (amort_oracle.pas:763-764) and leaves Amortize's own A6 arm
			// (engine.go:637) to derive them — which is what DOS's MakeTable
			// dispatch does at Amortize.pas:1350. LastOK stays false, exactly
			// as it does in DOS's else arm at Amortize.pas:243. No pre-solve
			// helper is needed (unlike `norate`): Amortize solves the term
			// itself.
			solveTerm = true
		case strings.HasPrefix(a, "loandmy="):
			loanDate = parseDMY(strings.TrimPrefix(a, "loandmy="))
		case strings.HasPrefix(a, "firstdmy="):
			firstDate = parseDMY(strings.TrimPrefix(a, "firstdmy="))
		case strings.HasPrefix(a, "first="):
			// `first=MONTHS` — the odd-first-period axis. The oracle sets the
			// first-payment date to MONTHS months after the loan date using the
			// same UNCLAMPED month arithmetic every other offset token uses:
			//
			//	nbal := StrToIntDef(...);
			//	nbal := (h^.loandate.m - 1) + nbal;
			//	h^.firstdate.d := h^.loandate.d;
			//	h^.firstdate.m := (nbal mod 12) + 1;
			//	h^.firstdate.y := h^.loandate.y + (nbal div 12);
			//
			// (amort_oracle.pas:776-789). MONTHS below one period gives a SHORT
			// odd first stub, above it a LONG one, so this is the token that
			// drives the prorated first-period interest — the same territory the
			// seed-20250 Feb-clause defects lived in. addMonths() is exactly the
			// closure above, anchored on the pre-scanned loan date, so `first=`
			// lands on the identical date the oracle computes.
			mo, err := strconv.Atoi(strings.TrimPrefix(a, "first="))
			if err != nil {
				t.Fatalf("bad first= token %q: %v", a, err)
			}
			firstDate = addMonths(mo)
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
			// NO `fancy = true` here. The oracle driver sets its `fancy` global
			// only from the ADVANCED-OPTION blocks — balloons (amort_oracle.pas:183),
			// prepayments (:276), adjustments (:423), mor= (:788), targ= (:802),
			// skip= (:813), solveballoon= (:859), dateballoon= (:879). `pts=` just
			// fills h^.points, an ordinary loan field. Setting fancy here made this
			// harness run the port's FANCY engine against DOS's PLAIN one on any
			// points-bearing screen with no advanced options, which both masked real
			// divergences and manufactured phantom ones (fuzzer5 seed 9306 could not
			// be reproduced through M5 until this was corrected). dos_fuzzer5_test.go
			// already gets this right — see its fancyOpt comment.
			points, _ = strconv.ParseFloat(strings.TrimPrefix(a, "pts="), 64)
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
				// Built inline rather than through balloonAt(): that helper
				// anchors on the package-level dateMonthsAfterLoan, which is
				// pinned to 1 January 2024 and so ignores `loandmy=`.
				balloons = append(balloons, BalloonPayment{
					DateStatus: types.InOutInput, Date: addMonths(m),
					AmountStatus: types.InOutInput, Amount: amt,
				})
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
	if solveTerm {
		in.Loan.NStatus, in.Loan.NPeriods = types.StatusEmpty, 0
		in.Loan.LastStatus, in.Loan.LastOK = types.StatusEmpty, false
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
	if err := m5TryPreSolveRate(in); err != nil {
		t.Fatalf("pre-solve rate: %v", err)
	}
}

// m5TryPreSolveRate is the non-fatal form, for callers (TestM5Ablate) that run
// many derived screens and must survive one of them failing to solve.
// It returns nil and leaves `in` untouched when the rate was supplied.
func m5TryPreSolveRate(in *LoanInput) error {
	if in.Loan.LoanRateStatus > types.StatusEmpty {
		return nil
	}
	si := *in
	sl := in.Loan
	if err := FirstPass(&sl); err != nil {
		return fmt.Errorf("FirstPass: %w", err)
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
		return fmt.Errorf("SolveRate: %w", err)
	}
	in.Loan.LoanRateStatus = types.InOutInput
	in.Loan.LoanRate = solved
	return nil
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
	// Drop Go's leading settlement line before the row-for-row compare. DOS
	// MakeTable emits one (Amortize.pas:1476-1491, `payment.paynum := -1`) when
	//
	//	((prepaid) and (PrepaidInterest>0)) or ((h^.pointsstatus>empty) and (h^.points<>0))
	//
	// but the oracle driver deliberately EXCLUDES it from its detail-line dump
	// (amort_oracle.pas:492-495: "the in-advance / prepaid settlement-interest
	// line begins with paynum 0 (or -1) and is excluded so the row sequence
	// matches the per-payment schedule"). The port emits the same line with
	// PayNum 0 (engine.go applyPointsSettlement / the prepaid stub), so without
	// this trim every DOS row lines up against the Go row one index EARLIER and
	// the comparator reports a phantom divergence at [0] on any prepaid- or
	// points-bearing screen. 2026-07-25 fuzzer5 seed 9306 surfaced this: totals
	// agreed exactly (459909.45 / 877998.02) while every printed row looked
	// wrong by one position.
	// ...but that exclusion only actually FIRES on the ORDINARY (non-fancy)
	// print format. Read IsDetailLine's last line again:
	//
	//	t1 := GetTok(s, 1);
	//	IsDetailLine := IsPosInt(t1) or (Pos('/', t1) > 0);
	//
	// The paynum test is what rejects the settlement row — and MakeTable only
	// puts a paynum in column 1 on the plain walk. RepayFancyLoan prints the
	// DATE first, so on any fancy screen the settlement row's first token is
	// something like `7/27/23`, the `Pos('/', t1) > 0` arm accepts it, and DOS
	// KEEPS the row. The comment above IsDetailLine describes the intent, not
	// the behaviour on fancy output.
	//
	// So the trim has to be conditional on which format DOS actually emitted,
	// and the emitted rows tell us directly: label is column 1, so a leading
	// row whose label parses as an integer means the plain walk (settlement
	// dropped → trim Go to match), and a label containing '/' means fancy
	// (settlement kept → leave Go alone). 2026-07-25 seed 20102 surfaced this:
	// the trim had been written unconditionally off seed 9306, which is a plain
	// points screen, and it silently shifted every fancy schedule by one row,
	// reporting FIRST DIVERGENCE at index 0 on cases that agree.
	dosDroppedSettlement := true
	if len(dosRows) > 0 && strings.Contains(dosRows[0].label, "/") {
		dosDroppedSettlement = false
	}
	goRows := gr.Schedule
	if dosDroppedSettlement {
		for len(goRows) > 0 && goRows[0].PayNum <= 0 {
			goRows = goRows[1:]
		}
	}
	t.Logf("DOS payment %.4f rows %d (fancy-format=%v) | Go rows %d (%d after settlement trim) totalInt %.2f totalPaid %.2f finalPrinc %.2f",
		dosPay, len(dosRows), !dosDroppedSettlement, len(gr.Schedule), len(goRows), gr.TotalInt, gr.TotalPaid, gr.FinalPrinc)

	nShow, _ := strconv.Atoi(os.Getenv("M5N"))
	if nShow == 0 {
		nShow = 12
	}
	first := -1
	// M5FROM forces the dump to begin at a fixed row index instead of at the
	// first divergence, so the rows LEADING UP TO a divergence can be inspected.
	if v, err := strconv.Atoi(os.Getenv("M5FROM")); err == nil && os.Getenv("M5FROM") != "" {
		first = v
	}
	max := len(dosRows)
	if len(goRows) > max {
		max = len(goRows)
	}
	for i := 0; i < max; i++ {
		var d m5Row
		var okD bool
		if i < len(dosRows) {
			d, okD = dosRows[i], true
		}
		var g PaymentRecord
		var okG bool
		if i < len(goRows) {
			g, okG = goRows[i], true
		}
		bad := !okD || !okG ||
			math.Abs(d.inter-g.Interest) > 0.011 || math.Abs(d.bal-g.Principal) > 0.011
		if bad && first < 0 {
			first = i
		}
		if first >= 0 && i >= first && i < first+nShow {
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
		// The Go side must be driven the way the real handler drives it. On a
		// `norate`/`solverate` screen the rate field is BLANK, and Amortize
		// alone does not solve it — it walks the schedule at 0%, which reads
		// out as `Go int=0.00 paid=<principal>` and looks exactly like a
		// catastrophic engine divergence. TestM5Rows already pre-solves
		// (:441); this runner did not, so every rate-solve ablation it printed
		// was a harness artifact. Non-fatal here: an ablated sub-screen may
		// legitimately have no solvable rate, and that must not kill the run.
		if err := m5TryPreSolveRate(&in); err != nil {
			return fmt.Sprintf("Go PRE-SOLVE FAILED: %v", err)
		}
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
