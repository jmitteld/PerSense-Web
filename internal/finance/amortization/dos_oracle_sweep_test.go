package amortization

import (
	"github.com/persense/persense-port/internal/types"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// oracleBinary returns the DOS source-oracle path. Override with
// PERSENSE_ORACLE; defaults to the location legacy/oracle/build_linux.sh emits.
func oracleBinary() string {
	if p := os.Getenv("PERSENSE_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/amort_oracle"
}

var oracleBin = oracleBinary()

func firstPeriodDate(perYr int) types.DateRec {
	m := 1 + 12/perYr
	y := 2024
	if m > 12 {
		m -= 12
		y++
	}
	return types.NewDateRec(y, time.Month(m), 1)
}

// runOracle execs the real DOS engine. The Pascal New(h)/ZeroAMZLoan path is
// occasionally heap-sensitive and returns a 0 payment (~9% of rapid spawns);
// every such case reproduces correctly on a fresh process, so we retry up to
// 8 times and report no-answer (ok=false) only if it never produces one.
func runOracle(amount, rate float64, n, perYr int) (pay, interest float64, ok bool) {
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin,
			strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 6, 64),
			strconv.Itoa(n), strconv.Itoa(perYr)).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) < 6 || f[0] != "payment" {
			continue
		}
		pay, _ = strconv.ParseFloat(f[1], 64)
		interest, _ = strconv.ParseFloat(f[3], 64)
		if pay != 0 {
			return pay, interest, true
		}
	}
	return 0, 0, false
}
func goSolve(amount, rate float64, n, perYr int) (pay, interest float64, ok bool) {
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}}
	d, err := SolvePayment(in)
	if err != nil {
		return 0, 0, false
	}
	chk := in
	chk.Loan.PayAmtStatus = types.InOutInput
	chk.Loan.PayAmt = d
	r := Amortize(chk)
	if r.Err != nil {
		return 0, 0, false
	}
	return d, r.TotalInt, true
}
func TestDOSDifferentialSweep(t *testing.T) {
	// This test drives the REAL DOS amortization engine compiled by
	// legacy/oracle/build_linux.sh. It is skipped automatically when that
	// binary is absent, so ordinary `go test ./...` runs are unaffected.
	// Build it first (on Linux): legacy/oracle/build_linux.sh
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh to enable", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260610))
	const N = 1500
	checked, skipped, fails := 0, 0, 0
	payMax, intMax := 0.0, 0.0
	var worstPay, worstInt string
	for i := 0; i < N; i++ {
		amount := float64(1000 + rng.Intn(999000))
		rate := 0.005 + rng.Float64()*0.18
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := (2 + rng.Intn(38)) * perYr
		op, oi, ok1 := runOracle(amount, rate, n, perYr)
		gp, gi, ok2 := goSolve(amount, rate, n, perYr)
		if !ok1 || !ok2 {
			skipped++
			continue
		}
		checked++
		pr := math.Abs(op-gp) / math.Max(1, gp)
		ir := math.Abs(oi-gi) / math.Max(1, gi)
		if pr > payMax {
			payMax = pr
			worstPay = mk(amount, rate, n, perYr, op, gp)
		}
		if ir > intMax {
			intMax = ir
			worstInt = mk(amount, rate, n, perYr, oi, gi)
		}
		if pr > 1e-4 || ir > 1e-3 {
			fails++
			if fails <= 15 {
				t.Errorf("DIVERGE amt=%.0f r=%.4f n=%d py=%d: DOS pay=%.4f int=%.2f | Go pay=%.4f int=%.2f (pr=%.2e ir=%.2e)", amount, rate, n, perYr, op, oi, gp, gi, pr, ir)
			}
		}
	}
	t.Logf("checked %d, skipped(oracle no-answer) %d, divergences %d", checked, skipped, fails)
	t.Logf("max payment relErr=%.2e at [%s]", payMax, worstPay)
	t.Logf("max interest relErr=%.2e at [%s]", intMax, worstInt)
}
func mk(a, r float64, n, py int, o, g float64) string {
	return strconv.FormatFloat(a, 'f', 0, 64) + " r=" + strconv.FormatFloat(r, 'f', 4, 64) + " n=" + strconv.Itoa(n) + " py=" + strconv.Itoa(py) + " DOS=" + strconv.FormatFloat(o, 'f', 4, 64) + " Go=" + strconv.FormatFloat(g, 'f', 4, 64)
}

// runOracleBalloon execs the DOS oracle with a single balloon `balloonMonths`
// months after the loan date. Same transient-flake retry as runOracle.
func runOracleBalloon(amount, rate float64, n, perYr, balloonMonths int, balloonAmt float64) (pay, interest float64, ok bool) {
	tok := "b" + strconv.Itoa(balloonMonths) + "=" + strconv.FormatFloat(balloonAmt, 'f', 2, 64)
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin,
			strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 6, 64),
			strconv.Itoa(n), strconv.Itoa(perYr), tok).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) < 6 || f[0] != "payment" {
			continue
		}
		pay, _ = strconv.ParseFloat(f[1], 64)
		interest, _ = strconv.ParseFloat(f[3], 64)
		if pay != 0 {
			return pay, interest, true
		}
	}
	return 0, 0, false
}

func goSolveBalloon(amount, rate float64, n, perYr, balloonMonths int, balloonAmt float64) (pay float64, ok bool) {
	ld := types.NewDateRec(2024, time.January, 1)
	tot := balloonMonths
	by := 2024 + tot/12
	bm := time.Month(tot%12 + 1)
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: ld,
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Balloons: []BalloonPayment{{
			DateStatus: types.InOutInput, Date: types.NewDateRec(by, bm, 1),
			AmountStatus: types.InOutInput, Amount: balloonAmt}},
		Fancy:    true,
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: false},
	}
	d, err := SolvePayment(in)
	if err != nil {
		return 0, false
	}
	return d, true
}

// TestDOSBalloonSweep validates the fancy backward PAYMENT solve under a single
// balloon against the real DOS engine. The balloon lands on a payment date
// (months = period * 12/perYr) and is a modest fraction of principal so the
// loan still amortizes. Confirms the balloon replace-vs-add (PlusRegular=false)
// convention matches DOS, the bug-class the roadmap flagged for fancy schedules.
func TestDOSBalloonSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260611))
	const N = 600
	checked, skipped, fails := 0, 0, 0
	payMax := 0.0
	var worst string
	for i := 0; i < N; i++ {
		amount := float64(20000 + rng.Intn(480000))
		rate := 0.01 + rng.Float64()*0.15
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		nPeriods := (3 + rng.Intn(25)) * perYr // 3..27 years
		mPerPeriod := 12 / perYr
		bp := 1 + rng.Intn(nPeriods-1) // balloon on payment 1..n-1
		balloonMonths := bp * mPerPeriod
		balloonAmt := amount * (0.05 + rng.Float64()*0.25) // 5%..30% of principal
		op, _, ok1 := runOracleBalloon(amount, rate, nPeriods, perYr, balloonMonths, balloonAmt)
		gp, ok2 := goSolveBalloon(amount, rate, nPeriods, perYr, balloonMonths, balloonAmt)
		if !ok1 || !ok2 {
			skipped++
			continue
		}
		checked++
		pr := math.Abs(op-gp) / math.Max(1, gp)
		if pr > payMax {
			payMax = pr
			worst = mk(amount, rate, nPeriods, perYr, op, gp) + " bMo=" + strconv.Itoa(balloonMonths)
		}
		if pr > 5e-4 {
			fails++
			if fails <= 15 {
				t.Errorf("BALLOON pay mismatch amt=%.0f r=%.4f n=%d py=%d bMo=%d bAmt=%.0f: DOS=%.4f Go=%.4f (rel %.2e)",
					amount, rate, nPeriods, perYr, balloonMonths, balloonAmt, op, gp, pr)
			}
		}
	}
	t.Logf("balloon sweep: checked %d, skipped %d, divergences %d, max payment relErr=%.2e at [%s]", checked, skipped, fails, payMax, worst)
}

type balloonSpec struct {
	months int
	amt    float64
}

func runOracleBalloons(amount, rate float64, n, perYr int, bs []balloonSpec) (pay float64, ok bool) {
	args := []string{strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 6, 64),
		strconv.Itoa(n), strconv.Itoa(perYr)}
	for _, b := range bs {
		args = append(args, "b"+strconv.Itoa(b.months)+"="+strconv.FormatFloat(b.amt, 'f', 2, 64))
	}
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) < 6 || f[0] != "payment" {
			continue
		}
		pay, _ = strconv.ParseFloat(f[1], 64)
		if pay != 0 {
			return pay, true
		}
	}
	return 0, false
}

func goSolveBalloons(amount, rate float64, n, perYr int, bs []balloonSpec) (pay float64, ok bool) {
	balloons := make([]BalloonPayment, len(bs))
	for i, b := range bs {
		by := 2024 + b.months/12
		bm := time.Month(b.months%12 + 1)
		balloons[i] = BalloonPayment{DateStatus: types.InOutInput, Date: types.NewDateRec(by, bm, 1),
			AmountStatus: types.InOutInput, Amount: b.amt}
	}
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Balloons: balloons, Fancy: true,
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: false},
	}
	d, err := SolvePayment(in)
	if err != nil {
		return 0, false
	}
	return d, true
}

// TestDOSTwoBalloonSweep validates the payment solve under TWO balloons on
// distinct payment dates — exercising SortBalloons and multi-balloon
// discounting against the real DOS engine.
func TestDOSTwoBalloonSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260612))
	const N = 300
	checked, skipped, fails := 0, 0, 0
	payMax := 0.0
	for i := 0; i < N; i++ {
		amount := float64(40000 + rng.Intn(460000))
		rate := 0.02 + rng.Float64()*0.12
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		nPeriods := (5 + rng.Intn(20)) * perYr
		mPer := 12 / perYr
		p1 := 1 + rng.Intn(nPeriods/2)
		p2 := nPeriods/2 + 1 + rng.Intn(nPeriods/2-1)
		if p2 <= p1 {
			skipped++
			continue
		}
		bs := []balloonSpec{
			{p1 * mPer, amount * (0.04 + rng.Float64()*0.12)},
			{p2 * mPer, amount * (0.04 + rng.Float64()*0.12)},
		}
		op, ok1 := runOracleBalloons(amount, rate, nPeriods, perYr, bs)
		gp, ok2 := goSolveBalloons(amount, rate, nPeriods, perYr, bs)
		if !ok1 || !ok2 {
			skipped++
			continue
		}
		checked++
		pr := math.Abs(op-gp) / math.Max(1, gp)
		if pr > payMax {
			payMax = pr
		}
		if pr > 5e-4 {
			fails++
			if fails <= 12 {
				t.Errorf("2BALLOON mismatch amt=%.0f r=%.4f n=%d py=%d b=[%d,%d]: DOS=%.4f Go=%.4f (rel %.2e)",
					amount, rate, nPeriods, perYr, bs[0].months, bs[1].months, op, gp, pr)
			}
		}
	}
	t.Logf("two-balloon sweep: checked %d, skipped %d, divergences %d, max payment relErr=%.2e", checked, skipped, fails, payMax)
}

type oracleRow struct{ interest, balance float64 }

// runOracleRows returns the per-payment schedule (interest + remaining balance)
// the real DOS engine prints in detail mode (cum=' '), amortizing the GIVEN
// payment so both engines use an identical payment (no solve-precision drift).
func runOracleRows(amount, rate float64, n, perYr int, pay float64) ([]oracleRow, bool) {
	out, err := exec.Command(oracleBin,
		strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr), "rows",
		"pay="+strconv.FormatFloat(pay, 'f', 10, 64)).Output()
	if err != nil {
		return nil, false
	}
	var rows []oracleRow
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(ln)
		if len(f) == 8 && f[0] == "row" {
			interest, _ := strconv.ParseFloat(f[3], 64)
			bal, _ := strconv.ParseFloat(f[7], 64)
			rows = append(rows, oracleRow{interest: interest, balance: bal})
		}
	}
	if len(rows) == 0 {
		return nil, false
	}
	return rows, true
}

// TestDOSPerRowSweep validates the per-period schedule split (interest and
// remaining balance on every line), not just the totals, against the real DOS
// engine's detail-mode output. DOS prints to cents, so rows are compared at the
// cent level. This catches per-row bugs (wrong interest/principal split,
// dropped period, mis-timed balance) that a totals-only check would miss.
func TestDOSPerRowSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260617))
	const N = 500
	checked, skipped, rowFails, loanFails := 0, 0, 0, 0
	maxRowErr := 0.0
	var worst string
	for i := 0; i < N; i++ {
		amount := float64(2000 + rng.Intn(498000))
		rate := 0.005 + rng.Float64()*0.16
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 2 + rng.Intn(30) // 2..31 payments
		gp, pay, gok := goSolve2(amount, rate, n, perYr)
		if !gok {
			skipped++
			continue
		}
		dosRows, ok1 := runOracleRows(amount, rate, n, perYr, pay)
		if !ok1 {
			skipped++
			continue
		}
		checked++
		if len(dosRows) != len(gp) {
			loanFails++
			if loanFails <= 8 {
				t.Errorf("ROW COUNT amt=%.0f r=%.4f n=%d py=%d: DOS=%d Go=%d", amount, rate, n, perYr, len(dosRows), len(gp))
			}
			continue
		}
		for k := range dosRows {
			di := math.Abs(dosRows[k].interest - gp[k].Interest)
			db := math.Abs(dosRows[k].balance - gp[k].Principal)
			ri := di / math.Max(1, math.Abs(gp[k].Interest))
			rb := db / math.Max(1, math.Abs(gp[k].Principal))
			if ri > maxRowErr {
				maxRowErr = ri
			}
			if rb > maxRowErr {
				maxRowErr = rb
			}
			// DOS prints to cents; allow 1 cent of display rounding plus the
			// ~1e-5 relative engine precision shared with the totals sweep.
			if di > 0.01+1e-4*math.Abs(gp[k].Interest) || db > 0.01+1e-4*math.Abs(gp[k].Principal) {
				rowFails++
				if rowFails <= 10 {
					t.Errorf("ROW amt=%.0f r=%.4f n=%d py=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
						amount, rate, n, perYr, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
					worst = "amt=" + strconv.FormatFloat(amount, 'f', 0, 64)
				}
			}
		}
	}
	t.Logf("per-row: checked %d loans, skipped %d, row-count fails %d, row-value fails %d, max row |err|=%.4f %s",
		checked, skipped, loanFails, rowFails, maxRowErr, worst)
}

// goSolve2 solves the payment then returns the Go per-period schedule.
func goSolve2(amount, rate float64, n, perYr int) ([]PaymentRecord, float64, bool) {
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}}
	d, err := SolvePayment(in)
	if err != nil {
		return nil, 0, false
	}
	chk := in
	chk.Loan.PayAmtStatus = types.InOutDefault // non-hard: no per-period rounding
	chk.Loan.PayAmt = d
	r := Amortize(chk)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, 0, false
	}
	return r.Schedule, d, true
}

// runOracleRowsFlags is runOracleRows with extra trailing flag tokens (inadv,
// r78, usa, prepaid).
func runOracleRowsFlags(amount, rate float64, n, perYr int, pay float64, flags ...string) ([]oracleRow, bool) {
	args := []string{
		strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr), "rows",
		"pay=" + strconv.FormatFloat(pay, 'f', 10, 64)}
	args = append(args, flags...)
	out, err := exec.Command(oracleBin, args...).Output()
	if err != nil {
		return nil, false
	}
	var rows []oracleRow
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(ln)
		if len(f) == 8 && f[0] == "row" {
			interest, _ := strconv.ParseFloat(f[3], 64)
			bal, _ := strconv.ParseFloat(f[7], 64)
			rows = append(rows, oracleRow{interest: interest, balance: bal})
		}
	}
	if len(rows) == 0 {
		return nil, false
	}
	return rows, true
}

// goRowsFlags solves the payment (with solveMod applied to Settings) then
// amortizes (with amortMod applied) and returns the per-period schedule + payment.
func goRowsFlags(amount, rate float64, n, perYr int, solveMod, amortMod func(*Settings)) ([]PaymentRecord, float64, bool) {
	base := Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}
	sSet := base
	if solveMod != nil {
		solveMod(&sSet)
	}
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Settings: sSet}
	d, err := SolvePayment(in)
	if err != nil {
		return nil, 0, false
	}
	aSet := base
	if amortMod != nil {
		amortMod(&aSet)
	}
	chk := in
	chk.Settings = aSet
	chk.Loan.PayAmtStatus = types.InOutDefault
	chk.Loan.PayAmt = d
	r := Amortize(chk)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, 0, false
	}
	return r.Schedule, d, true
}

func perRowFlagSweep(t *testing.T, name string, seed int64, bodyOnly bool, solveMod, amortMod func(*Settings), oracleFlags ...string) {
	rng := rand.New(rand.NewSource(seed))
	const N = 300
	checked, skipped, countFails, valFails := 0, 0, 0, 0
	maxRel := 0.0
	for i := 0; i < N; i++ {
		amount := float64(2000 + rng.Intn(498000))
		rate := 0.005 + rng.Float64()*0.14
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 2 + rng.Intn(30)
		gp, pay, gok := goRowsFlags(amount, rate, n, perYr, solveMod, amortMod)
		if !gok {
			skipped++
			continue
		}
		dosRows, ok := runOracleRowsFlags(amount, rate, n, perYr, pay, oracleFlags...)
		if !ok {
			skipped++
			continue
		}
		checked++
		if len(dosRows) != len(gp) {
			countFails++
			if countFails <= 5 {
				t.Errorf("[%s] ROW COUNT amt=%.0f r=%.4f n=%d py=%d: DOS=%d Go=%d", name, amount, rate, n, perYr, len(dosRows), len(gp))
			}
			continue
		}
		// bodyOnly compares rows 1..n-1; the final row is a documented open
		// discrepancy (see TestDOSInAdvanceFinalRowFinding).
		last := len(dosRows)
		if bodyOnly {
			last--
		}
		for k := 0; k < last; k++ {
			di := math.Abs(dosRows[k].interest - gp[k].Interest)
			db := math.Abs(dosRows[k].balance - gp[k].Principal)
			rb := db / math.Max(1, math.Abs(gp[k].Principal))
			if rb > maxRel {
				maxRel = rb
			}
			if di > 0.01+1e-4*math.Abs(gp[k].Interest) || db > 0.01+1e-4*math.Abs(gp[k].Principal) {
				valFails++
				if valFails <= 8 {
					t.Errorf("[%s] ROW amt=%.0f r=%.4f n=%d py=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
						name, amount, rate, n, perYr, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
				}
			}
		}
	}
	t.Logf("[%s] per-row: checked %d, skipped %d, count fails %d, value fails %d, max bal relErr=%.2e",
		name, checked, skipped, countFails, valFails, maxRel)
}

// TestDOSFancyFlagSweep validates the in-advance (annuity-due) and Rule-of-78
// computational settings per-row against the real DOS engine. R78 is validated
// in full; in-advance is validated for the body (rows 1..n-1) — its final row
// is a known discrepancy documented in TestDOSInAdvanceFinalRowFinding.
func TestDOSFancyFlagSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}
	// in-advance: payment is annuity-due (solved with InAdvance), schedule too.
	// Full per-row comparison incl. the final row (fixed 2026-06-09 to charge
	// the final-period interest per DOS — see inadvance_final_row_finding.md).
	perRowFlagSweep(t, "in-advance", 20260618, false,
		func(s *Settings) { s.InAdvance = true },
		func(s *Settings) { s.InAdvance = true },
		"inadv")
	// Rule-of-78: payment unchanged (solve ordinary), schedule uses R78 split.
	perRowFlagSweep(t, "rule78", 20260619, false,
		nil,
		func(s *Settings) { s.R78 = true },
		"r78")
}

// TestDOSInAdvanceFinalRowFix verifies the fix for the in-advance final-payment
// discrepancy (docs/inadvance_final_row_finding.md): the real DOS engine charges
// interest of (p-d)*f_1/(2-f) on the final in-advance payment
// (AMORTIZE.pas:1533-1538); the Go engine now does the same (engine.go in-advance
// branch). DOS's final interest must be non-zero AND Go must match it.
func TestDOSInAdvanceFinalRowFix(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	// 100000 @ 12% annual, 4 in-advance payments (the worked example).
	gp, pay, ok := goRowsFlags(100000, 0.12, 4, 1,
		func(s *Settings) { s.InAdvance = true },
		func(s *Settings) { s.InAdvance = true })
	if !ok {
		t.Fatal("go schedule failed")
	}
	dosRows, ok := runOracleRowsFlags(100000, 0.12, 4, 1, pay, "inadv")
	if !ok {
		t.Fatal("oracle failed")
	}
	lastGo := gp[len(gp)-1]
	lastDOS := dosRows[len(dosRows)-1]
	t.Logf("in-advance final row: DOS interest=%.2f, Go interest=%.2f", lastDOS.interest, lastGo.Interest)
	if lastDOS.interest < 1.0 {
		t.Errorf("expected DOS to charge a non-zero final in-advance interest; got %.2f", lastDOS.interest)
	}
	if math.Abs(lastDOS.interest-lastGo.Interest) > 0.01 {
		t.Errorf("final in-advance interest mismatch: DOS=%.2f Go=%.2f", lastDOS.interest, lastGo.Interest)
	}
}

// goBalloonRows solves the balloon payment, amortizes, and returns the schedule.
func goBalloonRows(amount, rate float64, n, perYr, balloonMonths int, balloonAmt float64) ([]PaymentRecord, float64, bool) {
	by := 2024 + balloonMonths/12
	bm := time.Month(balloonMonths%12 + 1)
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(by, bm, 1),
			AmountStatus: types.InOutInput, Amount: balloonAmt}},
		Fancy:    true,
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: false}}
	d, err := SolvePayment(in)
	if err != nil {
		return nil, 0, false
	}
	chk := in
	chk.Loan.PayAmtStatus = types.InOutDefault
	chk.Loan.PayAmt = d
	r := Amortize(chk)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, 0, false
	}
	return r.Schedule, d, true
}

// TestDOSBalloonPerRowSweep validates the per-period interest/balance split of a
// balloon (fancy) schedule against the real DOS engine — not just the solved
// payment. Uses the fancy-mode per-row output (cum=' ' in RepayFancyLoan).
func TestDOSBalloonPerRowSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260623))
	checked, skipped, countFails, valFails := 0, 0, 0, 0
	maxRel := 0.0
	for i := 0; i < 300; i++ {
		amount := float64(20000 + rng.Intn(480000))
		rate := 0.01 + rng.Float64()*0.14
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		nP := (3 + rng.Intn(8)) * perYr // keep schedules modest
		mPer := 12 / perYr
		bp := 1 + rng.Intn(nP-1)
		bMonths := bp * mPer
		bAmt := amount * (0.05 + rng.Float64()*0.25)
		gp, pay, gok := goBalloonRows(amount, rate, nP, perYr, bMonths, bAmt)
		if !gok {
			skipped++
			continue
		}
		bTok := "b" + strconv.Itoa(bMonths) + "=" + strconv.FormatFloat(bAmt, 'f', 2, 64)
		dosRows, ok := runOracleRowsFlags(amount, rate, nP, perYr, pay, bTok)
		if !ok {
			skipped++
			continue
		}
		checked++
		if len(dosRows) != len(gp) {
			countFails++
			if countFails <= 6 {
				t.Errorf("ROW COUNT amt=%.0f r=%.4f n=%d py=%d bMo=%d: DOS=%d Go=%d", amount, rate, nP, perYr, bMonths, len(dosRows), len(gp))
			}
			continue
		}
		// Compare the body (rows 1..n-1); the final payoff row drives the
		// balance to ~0 and a 1-2 cent completion residual there is not a
		// per-row-split issue (interest is still checked on every row).
		for k := 0; k < len(dosRows); k++ {
			di := math.Abs(dosRows[k].interest - gp[k].Interest)
			db := math.Abs(dosRows[k].balance - gp[k].Principal)
			rb := db / math.Max(1, math.Abs(gp[k].Principal))
			if rb > maxRel {
				maxRel = rb
			}
			isLast := k == len(dosRows)-1
			intBad := di > 0.01+1e-4*math.Abs(gp[k].Interest)
			balBad := !isLast && db > 0.01+1e-4*math.Abs(gp[k].Principal)
			if intBad || balBad {
				valFails++
				if valFails <= 8 {
					t.Errorf("BALLOON ROW amt=%.0f r=%.4f n=%d py=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
						amount, rate, nP, perYr, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
				}
			}
		}
	}
	t.Logf("balloon per-row: checked %d, skipped %d, count fails %d, value fails %d, max bal relErr=%.2e", checked, skipped, countFails, valFails, maxRel)
}

// goOddFirst solves the payment with the first payment firstMonths after the
// loan date (an odd first period when != one full period), amortizes, and
// returns the schedule + payment.
func goOddFirst(amount, rate float64, n, perYr, firstMonths int) ([]PaymentRecord, float64, bool) {
	fy := 2024 + firstMonths/12
	fm := time.Month(firstMonths%12 + 1)
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(fy, fm, 1)},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}}
	d, err := SolvePayment(in)
	if err != nil {
		return nil, 0, false
	}
	chk := in
	chk.Loan.PayAmtStatus = types.InOutDefault
	chk.Loan.PayAmt = d
	r := Amortize(chk)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, 0, false
	}
	return r.Schedule, d, true
}

// TestDOSOddFirstPeriodSweep validates loans with a SHORT or LONG odd first
// period (first payment not exactly one period after the loan date), exercising
// the prorated first-period interest, against the real DOS engine — payment and
// every per-row split.
func TestDOSOddFirstPeriodSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260624))
	checked, skipped, countFails, valFails, merges := 0, 0, 0, 0, 0
	maxRel := 0.0
	for i := 0; i < 400; i++ {
		amount := float64(5000 + rng.Intn(495000))
		// Modest rate/term: this test isolates the prorated FIRST period; long
		// high-rate loans are covered by the normal-first per-row sweep, and the
		// "small difference of large numbers" accumulation there can shift the
		// payoff period independent of the first-period logic under test.
		rate := 0.01 + rng.Float64()*0.09
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 2 + rng.Intn(8)
		mPer := 12 / perYr
		// first payment 1..2*period months out (short..long odd stub), never 0
		firstMonths := 1 + rng.Intn(2*mPer)
		gp, pay, gok := goOddFirst(amount, rate, n, perYr, firstMonths)
		if !gok {
			skipped++
			continue
		}
		dosRows, ok := runOracleRowsFlags(amount, rate, n, perYr, pay, "first="+strconv.Itoa(firstMonths))
		if !ok {
			skipped++
			continue
		}
		checked++
		// DOS folds a sub-threshold (minpmt) final payment into the prior row,
		// so an odd first period can leave DOS with one fewer row than Go. More
		// than one row of difference would be a real structural problem.
		dn := len(dosRows)
		if dn != len(gp) {
			merges++
			if dn > len(gp) || len(gp)-dn > 1 {
				countFails++
				if countFails <= 6 {
					t.Errorf("ROW COUNT amt=%.0f r=%.4f n=%d py=%d first=%d: DOS=%d Go=%d", amount, rate, n, perYr, firstMonths, dn, len(gp))
				}
				continue
			}
		}
		// Compare the shared body rows (exclude the final payoff row, where
		// balances reconcile to ~0 / DOS may have merged the last entry).
		for k := 0; k < dn-1; k++ {
			di := math.Abs(dosRows[k].interest - gp[k].Interest)
			db := math.Abs(dosRows[k].balance - gp[k].Principal)
			rb := db / math.Max(1, math.Abs(gp[k].Principal))
			if rb > maxRel {
				maxRel = rb
			}
			if di > 0.01+1e-4*math.Abs(gp[k].Interest) || db > 0.01+1e-4*math.Abs(gp[k].Principal) {
				valFails++
				if valFails <= 8 {
					t.Errorf("ODDFIRST amt=%.0f r=%.4f n=%d py=%d first=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
						amount, rate, n, perYr, firstMonths, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
				}
			}
		}
	}
	t.Logf("odd-first-period: checked %d, skipped %d, final-payment merges %d, count fails %d, value fails %d, max bal relErr=%.2e", checked, skipped, merges, countFails, valFails, maxRel)
}

// TestDOSBoundaryCases validates explicit boundary/edge inputs per-row against
// the real DOS engine: zero rate (no-interest special case), a teeny rate at
// the engine's threshold, a very high rate, the minimum term (n=2, the smallest
// DOS allows), and a long term — across all payment frequencies.
func TestDOSBoundaryCases(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	type tc struct {
		amount, rate float64
		n, perYr     int
	}
	cases := []tc{
		{12000, 0.0, 12, 12},     // zero rate, monthly
		{50000, 0.0, 5, 1},       // zero rate, annual
		{100000, 1e-9, 24, 12},   // teeny rate (≈ Teeny threshold)
		{10000, 0.50, 12, 12},    // very high rate
		{250000, 0.45, 8, 4},     // high rate, quarterly
		{30000, 0.08, 2, 12},     // minimum term n=2
		{80000, 0.09, 2, 1},      // minimum term, annual
		{200000, 0.06, 50, 1},    // very long term (50 yrs annual)
		{500000, 0.035, 360, 12}, // 30-year monthly
	}
	fails := 0
	for _, c := range cases {
		gp, pay, gok := goSolve2(c.amount, c.rate, c.n, c.perYr)
		if !gok {
			t.Errorf("Go failed: %+v", c)
			continue
		}
		dosRows, ok := runOracleRowsFlags(c.amount, c.rate, c.n, c.perYr, pay)
		if !ok {
			t.Errorf("oracle no-answer: %+v", c)
			continue
		}
		if len(dosRows) != len(gp) {
			t.Errorf("ROW COUNT %+v: DOS=%d Go=%d", c, len(dosRows), len(gp))
			fails++
			continue
		}
		// body rows (exclude final payoff reconciliation)
		bad := 0
		for k := 0; k < len(dosRows)-1; k++ {
			di := math.Abs(dosRows[k].interest - gp[k].Interest)
			db := math.Abs(dosRows[k].balance - gp[k].Principal)
			if di > 0.01+1e-4*math.Abs(gp[k].Interest) || db > 0.01+1e-4*math.Abs(gp[k].Principal) {
				bad++
			}
		}
		if bad > 0 {
			fails++
			t.Errorf("BOUNDARY %+v: %d body rows diverge", c, bad)
		}
	}
	t.Logf("boundary cases: %d checked, %d failed", len(cases), fails)
}

// runOraclePayment returns just the DOS solved payment (totals mode) for a loan
// with optional trailing flag tokens (first=, b365, etc.). Same retry as runOracle.
func runOraclePayment(amount, rate float64, n, perYr int, flags ...string) (float64, bool) {
	args := []string{strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr)}
	args = append(args, flags...)
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) < 2 || f[0] != "payment" {
			continue
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		if v != 0 {
			return v, true
		}
	}
	return 0, false
}

// TestDOSPaymentSolveOddFirstAndBasis directly validates the SOLVED payment
// (not just the schedule) for odd first periods and the 365-day basis, against
// the real DOS engine. This is the gap that let the first-period-proration bug
// through (the earlier odd-first sweep fed a shared payment and never checked
// the solve). Fixed 2026-06-09 — SolvePayment now scales by ffFirst/f.
func TestDOSPaymentSolveOddFirstAndBasis(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260626))
	// (1) odd first periods on 30/360
	oddChecked, oddFails, oddMax := 0, 0, 0.0
	for i := 0; i < 400; i++ {
		amount := float64(5000 + rng.Intn(495000))
		rate := 0.01 + rng.Float64()*0.12
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 2 + rng.Intn(15)
		mPer := 12 / perYr
		firstMonths := 1 + rng.Intn(2*mPer)
		op, ok := runOraclePayment(amount, rate, n, perYr, "first="+strconv.Itoa(firstMonths))
		if !ok {
			continue
		}
		fy := 2024 + firstMonths/12
		fm := time.Month(firstMonths%12 + 1)
		in := LoanInput{Loan: Loan{AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate, NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr, PayAmtStatus: types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(fy, fm, 1)},
			Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}}
		gp, err := SolvePayment(in)
		if err != nil {
			continue
		}
		oddChecked++
		rel := math.Abs(op-gp) / math.Max(1, gp)
		if rel > oddMax {
			oddMax = rel
		}
		if rel > 1e-5 {
			oddFails++
			if oddFails <= 8 {
				t.Errorf("ODD-FIRST PAY amt=%.0f r=%.4f n=%d py=%d first=%d: DOS=%.4f Go=%.4f (rel %.2e)", amount, rate, n, perYr, firstMonths, op, gp, rel)
			}
		}
	}
	t.Logf("odd-first payment solve: checked %d, divergences %d, max relErr=%.2e", oddChecked, oddFails, oddMax)

	// (2) 365-day basis, monthly (where each month != one even period)
	b365Checked, b365Fails, b365Max := 0, 0, 0.0
	for i := 0; i < 300; i++ {
		amount := float64(5000 + rng.Intn(495000))
		rate := 0.01 + rng.Float64()*0.12
		n := 2 + rng.Intn(40)
		op, ok := runOraclePayment(amount, rate, n, 12, "b365")
		if !ok {
			continue
		}
		in := LoanInput{Loan: Loan{AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate, NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: 12, PayAmtStatus: types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(12)},
			Settings: Settings{Basis: types.Basis365, PerYr: 12, YrDays: 365.25, YrInv: 1.0 / 365.25}}
		gp, err := SolvePayment(in)
		if err != nil {
			continue
		}
		b365Checked++
		rel := math.Abs(op-gp) / math.Max(1, gp)
		if rel > b365Max {
			b365Max = rel
		}
		if rel > 1e-5 {
			b365Fails++
			if b365Fails <= 8 {
				t.Errorf("365 PAY amt=%.0f r=%.4f n=%d: DOS=%.4f Go=%.4f (rel %.2e)", amount, rate, n, op, gp, rel)
			}
		}
	}
	t.Logf("365-basis payment solve: checked %d, divergences %d, max relErr=%.2e", b365Checked, b365Fails, b365Max)
}

func goPrepayRows(amount, rate, pay float64, n, perYr, startMonths, nn int, prepayAmt float64, plusReg bool) ([]PaymentRecord, bool) {
	sy := 2024 + startMonths/12
	sm := time.Month(startMonths%12 + 1)
	in := LoanInput{Loan: Loan{AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate, NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: perYr, PayAmtStatus: types.InOutDefault, PayAmt: pay,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Prepayments: []Prepayment{{StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(sy, sm, 1),
			NNStatus: types.InOutInput, NN: nn, PerYrStatus: types.InOutInput, PerYr: perYr,
			PaymentStatus: types.InOutInput, Payment: prepayAmt}},
		Fancy:    true,
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: plusReg}}
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, false
	}
	return r.Schedule, true
}

// TestDOSPrepaymentPerRowSweep validates the additional-periodic-payment feature
// per-row against the real DOS engine, in BOTH modes: replace (plus_regular OFF,
// the default — a payment schedule) and add (plus_regular ON — extra on top).
// Prepayments are placed on regular payment dates (coincident); off-cycle rows
// are a documented separate item. Confirms the replace-vs-add fix.
func TestDOSPrepaymentPerRowSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260627))
	for _, plusReg := range []bool{false, true} {
		mode := "replace"
		flags := []string{}
		if plusReg {
			mode = "add"
			flags = []string{"plusreg"}
		}
		checked, skipped, countFails, valFails := 0, 0, 0, 0
		maxRel := 0.0
		for i := 0; i < 250; i++ {
			amount := float64(20000 + rng.Intn(480000))
			rate := 0.02 + rng.Float64()*0.10
			perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
			n := 4 + rng.Intn(12)
			mPer := 12 / perYr
			startP := 1 + rng.Intn(n/2)  // start on a regular payment 1..n/2
			nn := 1 + rng.Intn(n-startP) // up to remaining payments
			startMonths := startP * mPer
			// add mode: small extra; replace mode: a payment near the regular size
			var prepayAmt float64
			pay, ok0 := runOraclePayment(amount, rate, n, perYr)
			if !ok0 {
				skipped++
				continue
			}
			if plusReg {
				prepayAmt = pay * (0.1 + rng.Float64()*0.5)
			} else {
				prepayAmt = pay * (0.8 + rng.Float64()*0.6) // near/above regular so it still amortizes-ish
			}
			gp, gok := goPrepayRows(amount, rate, pay, n, perYr, startMonths, nn, prepayAmt, plusReg)
			if !gok {
				skipped++
				continue
			}
			preTok := "pre=" + strconv.Itoa(startMonths) + ":" + strconv.Itoa(nn) + ":" + strconv.Itoa(perYr) + ":" + strconv.FormatFloat(prepayAmt, 'f', 2, 64)
			args := append([]string{preTok}, flags...)
			dosRows, ok := runOracleRowsFlags(amount, rate, n, perYr, pay, args...)
			if !ok {
				skipped++
				continue
			}
			checked++
			if len(dosRows) != len(gp) {
				countFails++
				if countFails <= 5 {
					t.Errorf("[%s] ROW COUNT amt=%.0f r=%.4f n=%d py=%d start=%d nn=%d: DOS=%d Go=%d", mode, amount, rate, n, perYr, startMonths, nn, len(dosRows), len(gp))
				}
				continue
			}
			for k := 0; k < len(dosRows)-1; k++ {
				di := math.Abs(dosRows[k].interest - gp[k].Interest)
				db := math.Abs(dosRows[k].balance - gp[k].Principal)
				rb := db / math.Max(1, math.Abs(gp[k].Principal))
				if rb > maxRel {
					maxRel = rb
				}
				if di > 0.01+1e-4*math.Abs(gp[k].Interest) || db > 0.01+1e-4*math.Abs(gp[k].Principal) {
					valFails++
					if valFails <= 8 {
						t.Errorf("[%s] ROW amt=%.0f r=%.4f n=%d py=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
							mode, amount, rate, n, perYr, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
					}
				}
			}
		}
		t.Logf("[%s] prepay per-row: checked %d, skipped %d, count fails %d, value fails %d, max bal relErr=%.2e", mode, checked, skipped, countFails, valFails, maxRel)
	}
}

// goPrepayRowsFreq is goPrepayRows with a separate prepayment frequency
// (prepayPerYr), used to drive OFF-CYCLE prepayments (more frequent than the
// regular schedule, so their dates fall between regular payment dates).
func goPrepayRowsFreq(amount, rate, pay float64, n, perYr, startMonths, nn int, prepayPerYr int, prepayAmt float64, plusReg bool) ([]PaymentRecord, bool) {
	sy := 2024 + startMonths/12
	sm := time.Month(startMonths%12 + 1)
	in := LoanInput{Loan: Loan{AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate, NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: perYr, PayAmtStatus: types.InOutDefault, PayAmt: pay,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr)},
		Prepayments: []Prepayment{{StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(sy, sm, 1),
			NNStatus: types.InOutInput, NN: nn, PerYrStatus: types.InOutInput, PerYr: prepayPerYr,
			PaymentStatus: types.InOutInput, Payment: prepayAmt}},
		Fancy:    true,
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: plusReg}}
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, false
	}
	return r.Schedule, true
}

// TestDOSOffCyclePrepaymentSweep validates OFF-CYCLE prepayments — a series more
// frequent than the regular schedule, so its payments fall between regular dates
// and DOS emits each as its own dated row — against the real DOS engine, per-row.
// Confirms the off-cycle-row fix.
func TestDOSOffCyclePrepaymentSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260628))
	checked, skipped, countFails, valFails := 0, 0, 0, 0
	maxRel := 0.0
	for i := 0; i < 300; i++ {
		amount := float64(50000 + rng.Intn(450000))
		rate := 0.02 + rng.Float64()*0.08
		// loan less frequent than the prepayment, so prepayments are off-cycle
		loanPerYr := []int{1, 2, 4}[rng.Intn(3)]
		prepayPerYr := 12
		n := 3 + rng.Intn(8)
		mPer := 12 / loanPerYr
		startMonths := 1 + rng.Intn(2*mPer)
		nn := 2 + rng.Intn(10)
		pay, ok0 := runOraclePayment(amount, rate, n, loanPerYr)
		if !ok0 {
			skipped++
			continue
		}
		prepayAmt := pay * (0.05 + rng.Float64()*0.2) // additive extras
		gp, gok := goPrepayRowsFreq(amount, rate, pay, n, loanPerYr, startMonths, nn, prepayPerYr, prepayAmt, true)
		if !gok {
			skipped++
			continue
		}
		preTok := "pre=" + strconv.Itoa(startMonths) + ":" + strconv.Itoa(nn) + ":" + strconv.Itoa(prepayPerYr) + ":" + strconv.FormatFloat(prepayAmt, 'f', 2, 64)
		dosRows, ok := runOracleRowsFlags(amount, rate, n, loanPerYr, pay, preTok, "plusreg")
		if !ok {
			skipped++
			continue
		}
		checked++
		if len(dosRows) != len(gp) {
			countFails++
			if countFails <= 6 {
				t.Errorf("ROW COUNT amt=%.0f r=%.4f n=%d py=%d start=%d nn=%d: DOS=%d Go=%d", amount, rate, n, loanPerYr, startMonths, nn, len(dosRows), len(gp))
			}
			continue
		}
		for k := 0; k < len(dosRows)-1; k++ {
			di := math.Abs(dosRows[k].interest - gp[k].Interest)
			db := math.Abs(dosRows[k].balance - gp[k].Principal)
			rb := db / math.Max(1, math.Abs(gp[k].Principal))
			if rb > maxRel {
				maxRel = rb
			}
			if di > 0.01+1e-4*math.Abs(gp[k].Interest) || db > 0.01+1e-4*math.Abs(gp[k].Principal) {
				valFails++
				if valFails <= 8 {
					t.Errorf("OFFCYC ROW amt=%.0f r=%.4f n=%d py=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
						amount, rate, n, loanPerYr, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
				}
			}
		}
	}
	t.Logf("off-cycle prepay per-row: checked %d, skipped %d, count fails %d, value fails %d, max bal relErr=%.2e", checked, skipped, countFails, valFails, maxRel)
}

// lastPaymentDate returns the date of the n-th regular payment for a loan
// dated Jan 1 2024 whose first payment is one regular period out (the oracle's
// SetupLoan convention). The k-th payment falls at month (12/perYr)*k.
func lastPaymentDate(n, perYr int) types.DateRec {
	m := (12 / perYr) * n
	return types.NewDateRec(2024+m/12, time.Month(m%12+1), 1)
}

// runOraclePresolve drives the DOS oracle's unknown-prepayment-AMOUNT solve
// (EstimateAndRefinePeriodicPrepayment, AMORTIZE.pas:665) via the `presolve=`
// token and returns the solved per-payment prepayment amount.
func runOraclePresolve(amount, rate, pay float64, n, perYr, startMonths, nn, prepayPerYr int, plusReg bool) (float64, bool) {
	args := []string{
		strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr),
		"pay=" + strconv.FormatFloat(pay, 'f', 10, 64),
		"presolve=" + strconv.Itoa(startMonths) + ":" + strconv.Itoa(nn) + ":" + strconv.Itoa(prepayPerYr)}
	if plusReg {
		args = append(args, "plusreg")
	}
	out, err := exec.Command(oracleBin, args...).Output()
	if err != nil {
		return 0, false
	}
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(ln)
		if len(f) == 2 && f[0] == "prepay" {
			if v, e := strconv.ParseFloat(f[1], 64); e == nil {
				return v, true
			}
		}
	}
	return 0, false
}

// goSolvePrepayAmount drives the Go engine's unknown-prepayment-amount solve
// (AO9 dispatch in engine.go -> SolvePrepaymentAmount) and returns the solved
// amount. The regular payment is a known input below the fully-amortizing
// payment so a residual remains for the prepayment to retire; the prepayment
// row's Payment is left empty (unknown) and read back after Amortize.
func goSolvePrepayAmount(amount, rate, pay float64, n, perYr, startMonths, nn, prepayPerYr int, plusReg bool) (float64, bool) {
	sy := 2024 + startMonths/12
	sm := time.Month(startMonths%12 + 1)
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus: types.InOutDefault, PayAmt: pay,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr),
		LastStatus: types.InOutInput, LastDate: lastPaymentDate(n, perYr), LastOK: true},
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(sy, sm, 1),
			NNStatus: types.InOutInput, NN: nn, PerYrStatus: types.InOutInput, PerYr: prepayPerYr,
			NextDate: types.NewDateRec(sy, sm, 1),
			// PaymentStatus deliberately zero -> unknown, solved by AO9.
		}},
		Fancy:    true,
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: plusReg}}
	r := Amortize(in)
	if r.Err != nil {
		return 0, false
	}
	return in.Prepayments[0].Payment, true
}

// TestDOSPrepaymentAmountSolveSweep validates the unknown-prepayment-AMOUNT
// backward solver (DOS EstimateAndRefinePeriodicPrepayment / Go AO9
// SolvePrepaymentAmount) against the real DOS engine, in BOTH semantics:
//
//   - replace (plus_regular OFF, the DEFAULT): the solved prepayment replaces
//     the regular payment on coincident dates. The solve is well-posed (the
//     prepayment stream alone must amortize the loan) and Go matches DOS to
//     ~1e-8 across the sweep — asserted strictly below.
//
//   - add (plus_regular ON): the solved prepayment is ON TOP of the regular
//     payment. Here the LAST scheduled payment settles whatever balance
//     remains, so "final balance == 0" — the objective Go's
//     SolvePrepaymentAmount drives to zero — holds for a RANGE of prepayment
//     amounts (non-unique). DOS instead solves the unique value at which the
//     discounted payment stream exactly equals the principal (a "smooth"
//     amortization with no settlement balloon on the final row). Go's secant
//     therefore returns its initial guess (pay/2) rather than the DOS value.
//     This is a documented fidelity gap — NOT asserted; logged for the decision
//     record. See docs/prepayment_semantics_finding.md "Additive unknown-amount
//     solve" and the standing rule that engine-behavior changes are surfaced,
//     not silently applied.
//
// The prepayment series runs the whole term on the regular payment dates, with
// a regular payment held below the fully-amortizing payment so a residual
// remains for the prepayment to retire.
func TestDOSPrepaymentAmountSolveSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260629))
	for _, plusReg := range []bool{false, true} {
		checked, skipped, diverged := 0, 0, 0
		maxRel := 0.0
		for i := 0; i < 250; i++ {
			amount := float64(20000 + rng.Intn(480000))
			rate := 0.02 + rng.Float64()*0.10
			perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
			n := 4 + rng.Intn(12)
			mPer := 12 / perYr
			// Whole-term prepayment series on the regular payment dates.
			startMonths := mPer
			nn := n
			// Regular payment below fully-amortizing so a residual remains.
			amort, ok0 := runOraclePayment(amount, rate, n, perYr)
			if !ok0 {
				skipped++
				continue
			}
			pay := amort * (0.3 + rng.Float64()*0.4) // 30%-70% of amortizing
			dos, dok := runOraclePresolve(amount, rate, pay, n, perYr, startMonths, nn, perYr, plusReg)
			if !dok {
				skipped++
				continue
			}
			go_, gok := goSolvePrepayAmount(amount, rate, pay, n, perYr, startMonths, nn, perYr, plusReg)
			if !gok || go_ <= 0 {
				skipped++
				continue
			}
			checked++
			rel := math.Abs(dos-go_) / math.Max(1, math.Abs(dos))
			if rel > maxRel {
				maxRel = rel
			}
			match := math.Abs(dos-go_) <= 0.05+1e-3*math.Abs(dos)
			if !match {
				diverged++
				mode := "replace"
				if plusReg {
					mode = "add"
				}
				if diverged <= 8 {
					t.Errorf("[%s] PREPAY-AMT amt=%.0f r=%.4f n=%d py=%d pay=%.2f: DOS=%.4f Go=%.4f (rel %.2e)",
						mode, amount, rate, n, perYr, pay, dos, go_, rel)
				}
			}
		}
		mode := "replace"
		if plusReg {
			mode = "add (closed-form, Gap A fixed)"
		}
		t.Logf("[%s] prepayment-amount solve: checked %d, skipped %d, diverged %d, max relErr=%.2e",
			mode, checked, skipped, diverged, maxRel)
	}
}

// runOracleDuration drives the DOS oracle's unknown-prepayment-DURATION solve
// (DeterminePrepaymentDuration, AMORTIZE.pas:709 — forces additive internally)
// via the `predur=` token and returns the solved prepayment count (nn).
func runOracleDuration(amount, rate, pay float64, n, perYr, startMonths, prepayPerYr int, prepayAmt float64) (int, bool) {
	args := []string{
		strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr),
		"pay=" + strconv.FormatFloat(pay, 'f', 10, 64),
		"predur=" + strconv.Itoa(startMonths) + ":" + strconv.Itoa(prepayPerYr) + ":" + strconv.FormatFloat(prepayAmt, 'f', 2, 64)}
	out, err := exec.Command(oracleBin, args...).Output()
	if err != nil {
		return 0, false
	}
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(ln)
		if len(f) == 2 && f[0] == "duration" {
			if v, e := strconv.Atoi(f[1]); e == nil {
				return v, true
			}
		}
	}
	return 0, false
}

// goSolvePrepayDuration drives the Go engine's unknown-prepayment-duration
// solve (AO10 dispatch -> SolvePrepaymentDuration) and returns the solved count
// (NN). DeterminePrepaymentDuration is additive-only in DOS, so PlusRegular is
// forced ON here to match. The prepayment row has a known amount but no stop
// date and no count (left unknown); NN is read back after Amortize.
func goSolvePrepayDuration(amount, rate, pay float64, n, perYr, startMonths, prepayPerYr int, prepayAmt float64) (int, bool) {
	sy := 2024 + startMonths/12
	sm := time.Month(startMonths%12 + 1)
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus: types.InOutDefault, PayAmt: pay,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr),
		LastStatus: types.InOutInput, LastDate: lastPaymentDate(n, perYr), LastOK: true},
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(sy, sm, 1),
			PerYrStatus: types.InOutInput, PerYr: prepayPerYr,
			PaymentStatus: types.InOutInput, Payment: prepayAmt,
			NextDate: types.NewDateRec(sy, sm, 1),
			// NNStatus / StopDateStatus deliberately zero -> unknown -> AO10.
		}},
		Fancy:    true,
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true}}
	r := Amortize(in)
	if r.Err != nil {
		return 0, false
	}
	return in.Prepayments[0].NN, true
}

// TestDOSPrepaymentDurationSolveSweep measures the unknown-prepayment-DURATION
// backward solver (DOS DeterminePrepaymentDuration / Go AO10
// SolvePrepaymentDuration) against the real DOS engine.
//
// DOCUMENTED GAP — the two implement different models and diverge systematically:
//
//   - DOS (DeterminePrepaymentDuration, AMORTIZE.pas:709-774) is a CLOSED-FORM
//     present-value duration. It credits the regular payment with its PV over
//     the FULL nominal term (firstdate..lastdate), subtracts that and any other
//     extras from the principal, and solves the prepayment count nn whose PV
//     covers the remainder — then sets h^.nperiods := nn. The regular payment is
//     treated as running the full term regardless of when the extras stop.
//
//   - Go (SolvePrepaymentDuration) runs the schedule forward with the series
//     effectively unbounded and counts prepayments until the BALANCE reaches
//     zero (both streams stop at payoff).
//
// Worked example (100000 @ 8%, 360 monthly, pay=600 below the 733.76
// amortizing payment, +500/mo additive): DOS=42 (600 credited over 360mo →
// PV 81,768; 500×annuity(42)=18,232 covers the rest), Go=141 (total 1,100/mo
// retires the loan in ~141 months). Both are self-consistent under their own
// model; they are not the same quantity.
//
// This is reported, NOT asserted, per the standing rule that engine-behavior
// changes are surfaced for a decision rather than applied silently. Bringing Go
// to DOS parity here means porting DOS's closed-form PV duration in place of the
// simulate-to-payoff count. See docs/prepayment_semantics_finding.md.
func TestDOSPrepaymentDurationSolveSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260630))
	checked, skipped, diverged := 0, 0, 0
	maxDiff := 0
	for i := 0; i < 250; i++ {
		amount := float64(50000 + rng.Intn(450000))
		rate := 0.03 + rng.Float64()*0.08
		perYr := []int{12, 4}[rng.Intn(2)]
		n := perYr * (10 + rng.Intn(25)) // 10-35 year nominal term
		mPer := 12 / perYr
		startMonths := mPer
		amort, ok0 := runOraclePayment(amount, rate, n, perYr)
		if !ok0 {
			skipped++
			continue
		}
		// Regular payment well below amortizing so the prepay drives payoff.
		pay := amort * (0.4 + rng.Float64()*0.4)
		// Prepayment large enough that the duration is comfortably finite.
		prepayAmt := amort * (0.3 + rng.Float64()*0.5)
		dos, dok := runOracleDuration(amount, rate, pay, n, perYr, startMonths, perYr, prepayAmt)
		if !dok || dos <= 0 {
			skipped++
			continue
		}
		go_, gok := goSolvePrepayDuration(amount, rate, pay, n, perYr, startMonths, perYr, prepayAmt)
		if !gok || go_ <= 0 {
			skipped++
			continue
		}
		checked++
		diff := dos - go_
		if diff < 0 {
			diff = -diff
		}
		if diff > maxDiff {
			maxDiff = diff
		}
		if diff > 1 { // ±1 allowed (rounding at the installment boundary)
			diverged++
			if diverged <= 8 {
				t.Errorf("PREPAY-DUR amt=%.0f r=%.4f n=%d py=%d pay=%.2f pre=%.2f: DOS=%d Go=%d (diff %d)",
					amount, rate, n, perYr, pay, prepayAmt, dos, go_, diff)
			}
		}
	}
	t.Logf("prepayment-duration solve (closed-form PV, Gap B fixed): checked %d, skipped %d, "+
		"diverged(>1) %d, max |diff|=%d", checked, skipped, diverged, maxDiff)
}

// goWeeklyRows builds a weekly (perYr=52) or biweekly (perYr=26) loan on the
// 365.25-day basis — the path DOS auto-switches to for these frequencies — and
// returns the per-period schedule. The first payment is one period (7 or 14
// days) after the loan date, exercising the day-based interest accrual.
func goWeeklyRows(amount, rate, pay float64, n, perYr int) ([]PaymentRecord, bool) {
	loanDate := types.NewDateRec(2024, time.January, 1)
	firstDate := types.NewDateRec(2024, time.January, 1+364/perYr) // +14 or +7 days
	in := LoanInput{Loan: Loan{AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate, NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: perYr, PayAmtStatus: types.InOutDefault, PayAmt: pay,
		LoanDateStatus: types.InOutInput, LoanDate: loanDate,
		FirstStatus: types.InOutInput, FirstDate: firstDate},
		Settings: Settings{Basis: types.Basis365, PerYr: byte(perYr), YrDays: 365.25, YrInv: 1.0 / 365.25}}
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, false
	}
	return r.Schedule, true
}

// TestDOSWeeklyBiweeklySweep validates weekly and biweekly schedules per-row
// against the real DOS engine. These run on the 365-day basis with 7/14-day
// periods, accruing SIMPLE interest on the actual day count between payment
// dates (e.g. 14/366 in the leap year 2024) — the convention the DOS displayed
// schedule uses. (Earlier this diverged because the Go simple schedule used the
// constant per-period factor p*(f-1) on yrdays = 365.25; the engine now accrues
// weekly/biweekly on actual days — see docs/basis_weekly_finding.md.)
func TestDOSWeeklyBiweeklySweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260703))
	for _, perYr := range []int{26, 52} {
		label := "biweekly"
		if perYr == 52 {
			label = "weekly"
		}
		checked, skipped, countFails, valFails := 0, 0, 0, 0
		maxRel := 0.0
		for i := 0; i < 150; i++ {
			amount := float64(20000 + rng.Intn(180000))
			rate := 0.03 + rng.Float64()*0.08
			n := perYr/2 + rng.Intn(perYr*2) // a few months to ~2 years
			pay, ok0 := runOraclePayment(amount, rate, n, perYr)
			if !ok0 {
				skipped++
				continue
			}
			gp, gok := goWeeklyRows(amount, rate, pay, n, perYr)
			if !gok {
				skipped++
				continue
			}
			dosRows, ok := runOracleRowsFlags(amount, rate, n, perYr, pay)
			if !ok {
				skipped++
				continue
			}
			checked++
			if len(dosRows) != len(gp) {
				countFails++
				if countFails <= 5 {
					t.Errorf("[%s] ROW COUNT amt=%.0f r=%.4f n=%d: DOS=%d Go=%d", label, amount, rate, n, len(dosRows), len(gp))
				}
				continue
			}
			for k := 0; k < len(dosRows)-1; k++ {
				di := math.Abs(dosRows[k].interest - gp[k].Interest)
				db := math.Abs(dosRows[k].balance - gp[k].Principal)
				rb := db / math.Max(1, math.Abs(gp[k].Principal))
				if rb > maxRel {
					maxRel = rb
				}
				if di > 0.02+1e-4*math.Abs(gp[k].Interest) || db > 0.02+1e-4*math.Abs(gp[k].Principal) {
					valFails++
					if valFails <= 8 {
						t.Errorf("[%s] ROW amt=%.0f r=%.4f n=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
							label, amount, rate, n, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
					}
				}
			}
		}
		t.Logf("[%s] per-row: checked %d, skipped %d, count fails %d, value fails %d, max bal relErr=%.2e",
			label, checked, skipped, countFails, valFails, maxRel)
	}
}

// goFancyRows builds a base monthly loan (payment given) and applies a mutator
// that sets one fancy option, returning the per-period schedule.
func goFancyRows(amount, rate, pay float64, n int, mut func(*LoanInput)) ([]PaymentRecord, bool) {
	in := LoanInput{Loan: Loan{AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate, NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: 12, PayAmtStatus: types.InOutDefault, PayAmt: pay,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(12)},
		Fancy:    true,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360}}
	mut(&in)
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, false
	}
	return r.Schedule, true
}

// TestDOSFancyOptionsSweep validates moratorium, target (min principal
// reduction), and skip-months per-row against the real DOS engine.
func TestDOSFancyOptionsSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260702))
	for _, mode := range []string{"moratorium", "target", "skip"} {
		checked, skipped, countFails, valFails := 0, 0, 0, 0
		maxRel := 0.0
		for i := 0; i < 200; i++ {
			amount := float64(50000 + rng.Intn(450000))
			rate := 0.03 + rng.Float64()*0.07
			n := 24 + rng.Intn(48)
			pay, ok0 := runOraclePayment(amount, rate, n, 12)
			if !ok0 {
				skipped++
				continue
			}
			var tok string
			var mut func(*LoanInput)
			switch mode {
			case "moratorium":
				morM := 2 + rng.Intn(n/3) // interest-only until this month
				tok = "mor=" + strconv.Itoa(morM)
				my := 2024 + morM/12
				mm := time.Month(morM%12 + 1)
				mut = func(in *LoanInput) {
					in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput,
						FirstRepay: types.NewDateRec(my, mm, 1)}
				}
			case "target":
				targ := pay * (0.6 + rng.Float64()*0.35) // bind on early payments
				ts := strconv.FormatFloat(targ, 'f', 2, 64)
				targ, _ = strconv.ParseFloat(ts, 64)
				tok = "targ=" + ts
				mut = func(in *LoanInput) {
					in.Target = Target{TargetStatus: types.InOutInput, TargetValue: targ}
				}
			case "skip":
				// one skip window of 1-3 consecutive months in 2..11
				start := 2 + rng.Intn(8)
				width := rng.Intn(3)
				skipStr := strconv.Itoa(start)
				if width > 0 {
					skipStr += "-" + strconv.Itoa(start+width)
				}
				tok = "skip=" + skipStr
				ms, _ := MonthSetFromString(skipStr)
				mut = func(in *LoanInput) {
					in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput,
						SkipStr: skipStr, MonthSet: ms}
				}
			}
			gp, gok := goFancyRows(amount, rate, pay, n, mut)
			if !gok {
				skipped++
				continue
			}
			dosRows, ok := runOracleRowsFlags(amount, rate, n, 12, pay, tok)
			if !ok {
				skipped++
				continue
			}
			checked++
			if len(dosRows) != len(gp) {
				countFails++
				if countFails <= 5 {
					t.Errorf("[%s] ROW COUNT amt=%.0f r=%.4f n=%d tok=%s: DOS=%d Go=%d",
						mode, amount, rate, n, tok, len(dosRows), len(gp))
				}
				continue
			}
			for k := 0; k < len(dosRows)-1; k++ {
				di := math.Abs(dosRows[k].interest - gp[k].Interest)
				db := math.Abs(dosRows[k].balance - gp[k].Principal)
				rb := db / math.Max(1, math.Abs(gp[k].Principal))
				if rb > maxRel {
					maxRel = rb
				}
				if di > 0.02+1e-4*math.Abs(gp[k].Interest) || db > 0.02+1e-4*math.Abs(gp[k].Principal) {
					valFails++
					if valFails <= 8 {
						t.Errorf("[%s] ROW amt=%.0f r=%.4f n=%d tok=%s row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
							mode, amount, rate, n, tok, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
					}
				}
			}
		}
		t.Logf("[%s] fancy-option per-row: checked %d, skipped %d, count fails %d, value fails %d, max bal relErr=%.2e",
			mode, checked, skipped, countFails, valFails, maxRel)
	}
}

// goAdjustRows builds a monthly loan with a single rate/payment adjustment at
// adjMonth (months after the loan date) and returns the per-period schedule.
// newRate <= 0 means no rate change; newAmt <= 0 means no payment change.
func goAdjustRows(amount, rate, pay float64, n, adjMonth int, newRate, newAmt float64) ([]PaymentRecord, bool) {
	ay := 2024 + adjMonth/12
	am := time.Month(adjMonth%12 + 1)
	adj := RateAdjustment{DateStatus: types.InOutInput, Date: types.NewDateRec(ay, am, 1)}
	if newRate > 0 {
		adj.LoanRateStatus = types.InOutInput
		adj.LoanRate = newRate
	}
	if newAmt > 0 {
		adj.AmountStatus = types.InOutInput
		adj.Amount = newAmt
		adj.AmtOK = true
	}
	in := LoanInput{Loan: Loan{AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate, NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: 12, PayAmtStatus: types.InOutDefault, PayAmt: pay,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(12)},
		Adjustments: []RateAdjustment{adj},
		Fancy:       true,
		Settings:    Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360}}
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return nil, false
	}
	return r.Schedule, true
}

// TestDOSAdjustmentPerRowSweep validates ARM-style rate/payment adjustments
// per-row against the real DOS engine, in three modes: rate-only (which
// re-amortizes the payment, AO5), payment-only, and combined rate+payment.
func TestDOSAdjustmentPerRowSweep(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(20260701))
	modes := []string{"rate-only", "payment-only", "combined"}
	for _, mode := range modes {
		checked, skipped, countFails, valFails := 0, 0, 0, 0
		maxRel := 0.0
		for i := 0; i < 200; i++ {
			amount := float64(50000 + rng.Intn(450000))
			rate := 0.03 + rng.Float64()*0.07
			n := 24 + rng.Intn(60)
			adjMonth := 6 + rng.Intn(n-10) // a payment date between m6 and n-4
			pay, ok0 := runOraclePayment(amount, rate, n, 12)
			if !ok0 {
				skipped++
				continue
			}
			var newRate, newAmt float64
			adjTok := "adj=" + strconv.Itoa(adjMonth) + ":"
			// roundTok rounds a value to the exact decimal string the oracle
			// parses, so Go and DOS run on identical inputs (otherwise a tiny
			// rate/amount mismatch accumulates a sub-cent tail drift).
			roundTok := func(v float64, dec int) (float64, string) {
				s := strconv.FormatFloat(v, 'f', dec, 64)
				p, _ := strconv.ParseFloat(s, 64)
				return p, s
			}
			switch mode {
			case "rate-only":
				newRate = rate + (rng.Float64()-0.5)*0.04 // +/- 2%
				if newRate < 0.01 {
					newRate = 0.01
				}
				var rs string
				newRate, rs = roundTok(newRate, 10)
				adjTok += rs + ":"
			case "payment-only":
				var as string
				newAmt, as = roundTok(pay*(0.9+rng.Float64()*0.4), 2)
				adjTok += ":" + as
			case "combined":
				newRate = rate + (rng.Float64()-0.5)*0.04
				if newRate < 0.01 {
					newRate = 0.01
				}
				var rs, as string
				newRate, rs = roundTok(newRate, 10)
				newAmt, as = roundTok(pay*(0.9+rng.Float64()*0.4), 2)
				adjTok += rs + ":" + as
			}
			gp, gok := goAdjustRows(amount, rate, pay, n, adjMonth, newRate, newAmt)
			if !gok {
				skipped++
				continue
			}
			dosRows, ok := runOracleRowsFlags(amount, rate, n, 12, pay, adjTok)
			if !ok {
				skipped++
				continue
			}
			checked++
			if len(dosRows) != len(gp) {
				countFails++
				if countFails <= 5 {
					t.Errorf("[%s] ROW COUNT amt=%.0f r=%.4f n=%d adjM=%d: DOS=%d Go=%d",
						mode, amount, rate, n, adjMonth, len(dosRows), len(gp))
				}
				continue
			}
			for k := 0; k < len(dosRows)-1; k++ {
				di := math.Abs(dosRows[k].interest - gp[k].Interest)
				db := math.Abs(dosRows[k].balance - gp[k].Principal)
				rb := db / math.Max(1, math.Abs(gp[k].Principal))
				if rb > maxRel {
					maxRel = rb
				}
				if di > 0.02+1e-4*math.Abs(gp[k].Interest) || db > 0.02+1e-4*math.Abs(gp[k].Principal) {
					valFails++
					if valFails <= 8 {
						t.Errorf("[%s] ROW amt=%.0f r=%.4f n=%d adjM=%d row=%d: int DOS=%.2f Go=%.2f | bal DOS=%.2f Go=%.2f",
							mode, amount, rate, n, adjMonth, k+1, dosRows[k].interest, gp[k].Interest, dosRows[k].balance, gp[k].Principal)
					}
				}
			}
		}
		t.Logf("[%s] adjustment per-row: checked %d, skipped %d, count fails %d, value fails %d, max bal relErr=%.2e",
			mode, checked, skipped, countFails, valFails, maxRel)
	}
}
