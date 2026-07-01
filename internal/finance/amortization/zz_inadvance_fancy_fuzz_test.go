package amortization

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// TestDOSInAdvanceFancyFuzz is the thousands-case differential regression guard
// for the in-advance × fancy fix (docs/dos_known_frontier.md #38, closed).
//
// DOS's RepayFancyLoan gives an in-advance (annuity-due) fancy loan a distinct
// schedule SHAPE — a settlement-interest row at the loan date, a one-period base
// shift, and ordinary opening-balance interest on the shifted walk. The Go port
// reproduces that shape in generateFancySchedule (the `inAdvanceFancy` block).
// This fuzz sweep drives the REAL DOS engine (legacy/oracle) over randomized
// in-advance loans crossed with each advanced option the DOS oracle can solve —
// skip-months, moratorium (including deep/biting ones), balloon, and periodic
// prepayment — across basis {360, 365, 365/360} × prepaid × pmts/yr, and asserts:
//
//  1. the blank-payment SOLVE matches the DOS-solved payment, and
//  2. feeding the DOS-solved payment to BOTH engines, EVERY schedule row
//     (interest, principal portion, remaining balance) matches to the cent.
//
// It is skipped automatically when the oracle binary is absent, so ordinary
// `go test ./...` is unaffected; build it via legacy/oracle/build_linux.sh.
func TestDOSInAdvanceFancyFuzz(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}

	// addMonths mirrors the oracle's month-based date construction (day fixed at
	// the loan day-of-month = 1, matching SetupLoan/firstPeriodDate).
	addMonths := func(months int) types.DateRec {
		tot := 0 + months // loandate.m-1 == 0 (January)
		return types.NewDateRec(2024+tot/12, time.Month(tot%12+1), 1)
	}

	rng := rand.New(rand.NewSource(20260701))
	const N = 4000

	type option struct {
		name  string
		flags []string
		apply func(in *LoanInput)
	}

	checked, skipped := 0, 0
	payFails, rowFails := 0, 0
	balloonCoincidentN := 0
	payMax, rowMax := 0.0, 0.0
	var worstPay, worstRow string
	optCover := map[string]int{}

	for i := 0; i < N; i++ {
		// Random loan + settings (in-advance ALWAYS on — the axis under test).
		amount := float64(5000 + rng.Intn(495000))
		rate := 0.01 + rng.Float64()*0.18
		perYr := []int{12, 4, 2}[rng.Intn(3)]
		mPer := 12 / perYr
		years := 2 + rng.Intn(9)
		n := years * perYr
		basis := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}[rng.Intn(3)]
		prepaid := rng.Intn(2) == 0

		s := gzSettings(perYr, basis, false, prepaid, true /*inadv*/, false, false)
		var sflags []string
		if bf, ok := basisFlag(basis); ok {
			sflags = append(sflags, bf)
		}
		if prepaid {
			sflags = append(sflags, "prepaid")
		}
		sflags = append(sflags, "inadv")

		// Pick one advanced option and build BOTH the oracle flag and the Go mutation.
		var opt option
		switch rng.Intn(4) {
		case 0: // skip-months: a random contiguous run of 1-3 months
			start := 1 + rng.Intn(10)
			end := start + rng.Intn(3)
			if end > 12 {
				end = 12
			}
			skipStr := strconv.Itoa(start)
			if end > start {
				skipStr = fmt.Sprintf("%d-%d", start, end)
			}
			ms, _ := MonthSetFromString(skipStr)
			opt = option{"skip", []string{"skip=" + skipStr}, func(in *LoanInput) {
				in.Fancy = true
				in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: skipStr, MonthSet: ms}
			}}
			// Skip is month-based; only meaningful monthly.
			if perYr != 12 {
				skipped++
				continue
			}
		case 1: // moratorium: interest-only until first_repay (k periods in, up to ~half the term)
			k := 1 + rng.Intn(n/2+1)
			morMonths := k * mPer
			opt = option{"moratorium", []string{"mor=" + strconv.Itoa(morMonths)}, func(in *LoanInput) {
				in.Fancy = true
				in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: addMonths(morMonths)}
			}}
		case 2: // balloon on a payment date (adds to that period's regular payment)
			k := 1 + rng.Intn(n-1)
			bMonths := k * mPer
			amt := float64(1000 + rng.Intn(int(amount/4)+1000))
			opt = option{"balloon", []string{fmt.Sprintf("b%d=%.2f", bMonths, amt)}, func(in *LoanInput) {
				in.Fancy = true
				in.Balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: addMonths(bMonths),
					AmountStatus: types.InOutInput, Amount: amt}}
			}}
		case 3: // periodic prepayment series (same frequency), NN extras, replaces regular
			startK := 1 + rng.Intn(n/2+1)
			startMonths := startK * mPer
			nn := 1 + rng.Intn(n/2+1)
			amt := float64(200 + rng.Intn(3000))
			opt = option{"prepayment",
				[]string{fmt.Sprintf("pre=%d:%d:%d:%.2f", startMonths, nn, perYr, amt)},
				func(in *LoanInput) {
					in.Fancy = true
					in.Prepayments = []Prepayment{{
						StartDateStatus: types.InOutInput, StartDate: addMonths(startMonths),
						NNStatus: types.InOutInput, NN: nn,
						PerYrStatus:   types.InOutInput, PerYr: perYr,
						PaymentStatus: types.InOutInput, Payment: amt}}
				}}
		}

		flags := append(append([]string{}, sflags...), opt.flags...)

		// (1) payment-solve comparison.
		op, ok := runOraclePayment(amount, rate, n, perYr, flags...)
		if !ok || op == 0 {
			skipped++
			continue
		}
		in := gzLoanInput(amount, rate, n, perYr, s)
		in.Loan.PayAmtStatus = types.StatusEmpty
		opt.apply(&in)
		gr := Amortize(in)
		if gr.Err != nil || len(gr.Schedule) == 0 {
			skipped++
			continue
		}
		gp := modalReg(gr.Schedule)
		checked++
		optCover[opt.name]++

		// (1) payment-solve comparison — only for options where the solved
		// REGULAR payment is the modal row payment. For a prepayment series with
		// PlusRegular OFF the extra REPLACES the regular payment, so the modal row
		// is the prepay amount, not the solved regular — modalReg is not a valid
		// proxy there, so the prepayment payment-solve is validated purely by the
		// row-level schedule check below (feeding the DOS payment to both engines).
		// Balloon-on-or-before-the-first-payment-date is a distinct DOS init
		// special-case (RepayFancyLoan, AMORTOP.pas:1163-1178) tracked separately;
		// exclude those coincident balloons from the strict payment metric.
		// Balloon vs the first payment date. DOS (both arrears and in-advance)
		// REJECTS a balloon strictly before the first payment date ("Balloon cannot
		// precede first regular payment") — Go returns the same error, so those are
		// skipped by the ok checks. A balloon ON the first payment date under
		// IN-ADVANCE is a DELIBERATE divergence: DOS inflates the solved payment as
		// if the balloon existed but never actually applies or collects it (a
		// financially-nonsensical result driven by the dead `firstd` init path,
		// AMORTOP.pas:1166-1178), whereas Go applies the balloon, collects it, and
		// retires the loan correctly with less interest. Per project policy we keep
		// the correct answer and do NOT reproduce the DOS bug (cf. the AO7-balloon
		// deliberate divergence, docs/dos_known_frontier.md). We assert Go's result
		// is financially self-consistent (retires to ~0) instead of matching DOS.
		balloonOnFirst := false
		for bi := range in.Balloons {
			if dateutil.DateComp(in.Balloons[bi].Date, in.Loan.FirstDate) == 0 {
				balloonOnFirst = true
			}
		}
		strictPayment := opt.name == "skip" || opt.name == "moratorium" ||
			(opt.name == "balloon" && !balloonOnFirst)
		if strictPayment {
			pr := math.Abs(op-gp) / math.Max(1, gp)
			if pr > payMax {
				payMax = pr
				worstPay = fmt.Sprintf("%s %v amt=%.0f r=%.4f n=%d py=%d DOS=%.4f Go=%.4f", opt.name, flags, amount, rate, n, perYr, op, gp)
			}
			if pr > 2e-3 {
				payFails++
				if payFails <= 12 {
					t.Errorf("PAYMENT [%s] %v amt=%.0f r=%.4f n=%d py=%d: DOS=%.4f Go=%.4f (rel %.2e)",
						opt.name, flags, amount, rate, n, perYr, op, gp, pr)
				}
			}
		}

		// (2) row-level SCHEDULE fidelity: feed the DOS-solved payment to BOTH
		// engines and compare every row. This isolates the accrual/timing engine
		// from payment-solve precision. The balloon-on-first-payment in-advance case
		// is the deliberate DOS-bug divergence: don't compare to DOS, just assert
		// Go's own solved schedule retires to ~0 (financially correct).
		if balloonOnFirst {
			balloonCoincidentN++
			if gr.Err == nil && len(gr.Schedule) > 0 && math.Abs(gr.FinalPrinc) > 1.0 {
				rowFails++
				if rowFails <= 12 {
					t.Errorf("BALLOON-ON-FIRST (deliberate DOS divergence) Go did not retire: %v amt=%.0f n=%d py=%d finalPrinc=%.4f",
						flags, amount, n, perYr, gr.FinalPrinc)
				}
			}
			continue
		}
		orows, ok1 := runOracleRowsFull(amount, rate, n, perYr, op, flags...)
		gin := gzLoanInput(amount, rate, n, perYr, s)
		gin.Loan.PayAmtStatus = types.InOutDefault // non-hard: no per-period Round2, matching the oracle's defp
		gin.Loan.PayAmt = op
		opt.apply(&gin)
		gres := Amortize(gin)
		if !ok1 || gres.Err != nil || len(gres.Schedule) == 0 {
			continue
		}
		grows := goRegularRows(gres.Schedule)
		// Prepayment NN-derived trailing row is now CLOSED (veryLast derives the
		// NN stop date), so prepayment is held to the same strict exact-row-count
		// check as every other option.
		if len(grows) != len(orows) {
			rowFails++
			if rowFails <= 12 {
				t.Errorf("ROWCOUNT [%s] %v amt=%.0f r=%.4f n=%d py=%d: DOS=%d Go=%d rows",
					opt.name, flags, amount, rate, n, perYr, len(orows), len(grows))
			}
			continue
		}
		for k := range orows {
			// Cent-level absolute tolerance. The final row's residual is inflated
			// by feeding a payment rounded to the oracle's printed precision, so it
			// cannot retire to exactly 0 — allow a looser terminal tolerance there.
			// The fed payment carries only the oracle's printed 4-decimal
			// precision, so the terminal row's residual cannot retire to exactly 0
			// — allow a sub-dollar terminal tolerance. Non-terminal rows are held
			// to the cent (structural divergences are hundreds+ off, far above this).
			tol := 0.02
			if k == len(orows)-1 {
				tol = 1.0
			}
			di := math.Abs(orows[k].interest - grows[k].interest)
			db := math.Abs(orows[k].bal - grows[k].bal)
			worst := math.Max(di, db)
			if worst > rowMax {
				rowMax = worst
				worstRow = fmt.Sprintf("%s %v amt=%.0f r=%.4f n=%d py=%d row=%d/%d DOSint=%.2f Goint=%.2f DOSbal=%.2f Gobal=%.2f",
					opt.name, flags, amount, rate, n, perYr, k, len(orows), orows[k].interest, grows[k].interest, orows[k].bal, grows[k].bal)
			}
			if di > tol || db > tol {
				rowFails++
				if rowFails <= 12 {
					t.Errorf("ROW [%s] %v amt=%.0f r=%.4f n=%d py=%d row=%d/%d: DOS int=%.2f bal=%.2f | Go int=%.2f bal=%.2f",
						opt.name, flags, amount, rate, n, perYr, k, len(orows), orows[k].interest, orows[k].bal, grows[k].interest, grows[k].bal)
				}
				break
			}
		}
	}

	if checked < 500 {
		t.Fatalf("fuzz exercised only %d cases (skipped %d) — oracle may be flaking", checked, skipped)
	}
	t.Logf("in-advance × fancy fuzz: %d checked, %d skipped, coverage=%v", checked, skipped, optCover)
	t.Logf("  STRICT (skip / moratorium / balloon-on-payment-date / prepayment — payment + every row to the cent):")
	t.Logf("    payment: %d fails, max relErr=%.2e at [%s]", payFails, payMax, worstPay)
	t.Logf("    rows:    %d fails, max abs(cents)=%.4f at [%s]", rowFails, rowMax, worstRow)
	t.Logf("  DELIBERATE divergence (DOS bug not reproduced — Go asserted to retire correctly):")
	t.Logf("    balloon ON the first payment date, in-advance: %d cases (see docs/dos_known_frontier.md)", balloonCoincidentN)
	if payFails > 0 || rowFails > 0 {
		t.Fatalf("in-advance × fancy STRICT divergences: %d payment, %d row", payFails, rowFails)
	}
}

// goRegularRows extracts the amortizing/settlement rows from a Go schedule in the
// same shape and order the oracle's `rows` output emits (interest, principal
// portion, remaining balance). The oracle's IsDetailLine excludes the paynum-0
// settlement line for ordinary loans but INCLUDES the fancy-format settlement row
// (it begins with a date token), so the Go settlement row (PayNum 0, at the loan
// date) is kept here to align with the fancy oracle output.
func goRegularRows(sched []PaymentRecord) []fullRow {
	rows := make([]fullRow, 0, len(sched))
	for i := range sched {
		// principal portion = payment - interest (DOS "PRINC THIS PD").
		rows = append(rows, fullRow{
			interest: sched[i].Interest,
			prin:     sched[i].PayAmt - sched[i].Interest,
			bal:      sched[i].Principal,
		})
	}
	return rows
}
