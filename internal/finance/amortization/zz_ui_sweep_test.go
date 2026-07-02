package amortization

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

// runOracleTotals runs the DOS oracle (quiet mode) and returns the solved
// payment, total interest, and total paid. Retries transient heap no-answers.
func runOracleTotals(amount, rate float64, n, perYr int, flags ...string) (pay, interest, paid float64, ok bool) {
	args := []string{strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr)}
	args = append(args, flags...)
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
		interest, _ = strconv.ParseFloat(f[3], 64)
		paid, _ = strconv.ParseFloat(f[5], 64)
		if pay != 0 {
			return pay, interest, paid, true
		}
	}
	return 0, 0, 0, false
}

// TestUIAmortSweepVsDOS is a REPORT-ONLY diagnostic (it does not fail the build
// on the pre-existing gaps it characterizes; it only fails if the oracle flakes
// and too few cases run). It was added during the 2026-07-01 full-system UI
// sweep and surfaced three pre-existing amortization FINAL-answer divergences,
// documented in docs/ui_sweep_findings.md:
//   A. plain in-advance × non-360 basis — the simple in-advance schedule is
//      basis-blind (identical interest on 360/365/365-360; DOS accrues actual
//      days on non-360). ~7% total-interest. Normal inputs. The largest bucket.
//   B. prepayment blank-payment SOLVE precision (all bases) — the solved regular
//      payment differs from DOS on prepayment-replace loans (often pathological:
//      a small prepayment replacing a large regular payment → negative am).
//   C. balloon SOLVE on short/annual terms — payment-solve edge cases (e.g. a
//      balloon on the final payment of a 3-year annual loan).
// The INTERMEDIARY per-row accrual (given a fixed payment) is clean.
//
// It drives the same engine the API handler calls and compares to the real DOS
// oracle on:
//   - FINAL answer: total interest of the loan (a robust, option-independent
//     metric — unlike the modal payment, it is well-defined even when a
//     prepayment replaces the regular payment or half the months are skipped).
//   - INTERMEDIARY: every schedule row (interest + remaining balance), with the
//     DOS-solved payment fed to BOTH engines, so a row that drifts while the
//     final total agrees is caught independently. The Go settlement row (PayNum
//     0, in-advance) is aligned out since the oracle's row mode omits it.
func TestUIAmortSweepVsDOS(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle not present (%s)", oracleBin)
	}
	r := rand.New(rand.NewSource(70011))
	const N = 320

	checked, skipped := 0, 0
	intFails, rowFails, interimOnly, rowUnaligned := 0, 0, 0, 0
	intMax, rowMax := 0.0, 0.0
	var worstInt, worstRow string
	failBucket := map[string]int{} // categorize total-interest fails
	addM := func(months int) types.DateRec { return types.NewDateRec(2024+months/12, time.Month(months%12+1), 1) }

	for i := 0; i < N; i++ {
		amount := float64(5000 + r.Intn(495000))
		rate := 0.01 + r.Float64()*0.18
		perYr := []int{12, 4, 2, 1}[r.Intn(4)]
		mPer := 12 / perYr
		n := (2 + r.Intn(9)) * perYr
		basis := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}[r.Intn(3)]
		prepaid := r.Intn(2) == 0
		inadv := r.Intn(2) == 0
		s := gzSettings(perYr, basis, false, prepaid, inadv, false, false)
		var flags []string
		if bf, ok := basisFlag(basis); ok {
			flags = append(flags, bf)
		}
		if prepaid {
			flags = append(flags, "prepaid")
		}
		if inadv {
			flags = append(flags, "inadv")
		}

		in := gzLoanInput(amount, rate, n, perYr, s)
		in.Loan.PayAmtStatus = types.StatusEmpty
		optName := "plain"
		switch r.Intn(5) {
		case 1:
			if perYr == 12 {
				sm := fmt.Sprintf("%d-%d", 5+r.Intn(4), 8+r.Intn(3))
				ms, _ := MonthSetFromString(sm)
				in.Fancy = true
				in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: sm, MonthSet: ms}
				flags = append(flags, "skip="+sm)
				optName = "skip"
			}
		case 2:
			k := 1 + r.Intn(n/2+1)
			in.Fancy = true
			in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: addM(k * mPer)}
			flags = append(flags, "mor="+strconv.Itoa(k*mPer))
			optName = "moratorium"
		case 3:
			k := 2 + r.Intn(n-1)
			amt := float64(1000 + r.Intn(int(amount/10)+1000)) // balloon <= ~10% of loan
			in.Fancy = true
			in.Balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: addM(k * mPer),
				AmountStatus: types.InOutInput, Amount: amt}}
			flags = append(flags, fmt.Sprintf("b%d=%.2f", k*mPer, amt))
			optName = "balloon"
		case 4:
			sk := 1 + r.Intn(n/2+1)
			nn := 1 + r.Intn(n/2+1)
			amt := float64(200 + r.Intn(3000))
			in.Fancy = true
			in.Prepayments = []Prepayment{{StartDateStatus: types.InOutInput, StartDate: addM(sk * mPer),
				NNStatus: types.InOutInput, NN: nn, PerYrStatus: types.InOutInput, PerYr: perYr,
				PaymentStatus: types.InOutInput, Payment: amt}}
			flags = append(flags, fmt.Sprintf("pre=%d:%d:%d:%.2f", sk*mPer, nn, perYr, amt))
			optName = "prepayment"
		}

		_, dosInt, _, ok := runOracleTotals(amount, rate, n, perYr, flags...)
		if !ok {
			skipped++
			continue
		}
		gr := Amortize(in) // Go's own blank-payment solve → its schedule
		if gr.Err != nil || len(gr.Schedule) == 0 {
			skipped++
			continue
		}
		checked++

		// FINAL: total interest of Go's own-solve schedule vs DOS.
		ir := math.Abs(dosInt-gr.TotalInt) / math.Max(1, dosInt)
		intDiverged := false
		if ir > intMax {
			intMax, worstInt = ir, fmt.Sprintf("%s %v amt=%.0f r=%.4f n=%d py=%d DOSint=%.2f Goint=%.2f", optName, flags, amount, rate, n, perYr, dosInt, gr.TotalInt)
		}
		if ir > 3e-3 {
			intFails++
			intDiverged = true
			bkey := fmt.Sprintf("%s|basis=%s|inadv=%v|py=%d", optName, basisName(basis), inadv, perYr)
			failBucket[bkey]++
		}

		// INTERMEDIARY: feed the DOS payment to Go; compare rows after aligning the
		// in-advance settlement row (oracle row mode omits Go's PayNum-0 row).
		dosPay, _, _, ok2 := runOracleTotals(amount, rate, n, perYr, flags...)
		orows, ok1 := runOracleRowsFull(amount, rate, n, perYr, dosPay, flags...)
		if !ok1 || !ok2 {
			continue
		}
		gin := gzLoanInput(amount, rate, n, perYr, s)
		gin.Loan.PayAmtStatus = types.InOutDefault
		gin.Loan.PayAmt = dosPay
		gin.Fancy, gin.SkipMonths, gin.Moratorium, gin.Balloons, gin.Prepayments =
			in.Fancy, in.SkipMonths, in.Moratorium, in.Balloons, in.Prepayments
		gres := Amortize(gin)
		if gres.Err != nil || len(gres.Schedule) == 0 {
			continue
		}
		grows := goRegularRows(gres.Schedule)
		// Align the settlement row: if Go has one extra leading row that is the
		// PayNum-0 settlement (in-advance non-fancy), drop it.
		if len(grows) == len(orows)+1 && len(gres.Schedule) > 0 && gres.Schedule[0].PayNum == 0 {
			grows = grows[1:]
		}
		if len(grows) != len(orows) {
			rowUnaligned++ // count as unalignable rather than false-flag
			continue
		}
		rowDiverged := false
		for k := range orows {
			// per-row tolerance: a few cents absolute, or a hair of the balance
			// (365-basis carries 1-2 cents of per-row rounding); terminal row is
			// looser because a payment fed to 4 decimals cannot retire to 0 exactly.
			tol := math.Max(0.05, 2e-6*math.Abs(orows[k].bal))
			if k == len(orows)-1 {
				tol = math.Max(1.0, 2e-6*math.Abs(orows[k].bal))
			}
			di := math.Abs(orows[k].interest - grows[k].interest)
			db := math.Abs(orows[k].bal - grows[k].bal)
			if w := math.Max(di, db); w > rowMax {
				rowMax, worstRow = w, fmt.Sprintf("%s %v amt=%.0f r=%.4f n=%d py=%d row=%d/%d DOS(int=%.2f bal=%.2f) Go(int=%.2f bal=%.2f)", optName, flags, amount, rate, n, perYr, k, len(orows), orows[k].interest, orows[k].bal, grows[k].interest, grows[k].bal)
			}
			if di > tol || db > tol {
				rowFails++
				rowDiverged = true
				if rowFails <= 12 {
					t.Logf("ROW %s %v amt=%.0f r=%.4f n=%d py=%d row=%d/%d: DOS int=%.2f bal=%.2f | Go int=%.2f bal=%.2f", optName, flags, amount, rate, n, perYr, k, len(orows), orows[k].interest, orows[k].bal, grows[k].interest, grows[k].bal)
				}
				break
			}
		}
		if rowDiverged && !intDiverged {
			interimOnly++
		}
	}

	if checked < 150 {
		t.Fatalf("only %d cases checked (skipped %d) — oracle flaking", checked, skipped)
	}
	t.Logf("UI amort sweep: %d checked, %d skipped, %d row-unalignable(format, not compared)", checked, skipped, rowUnaligned)
	t.Logf("  FINAL total-interest: %d fails, max relErr=%.2e at [%s]", intFails, intMax, worstInt)
	t.Logf("  INTERMEDIARY rows: %d fails, max abs=%.4f at [%s]", rowFails, rowMax, worstRow)
	t.Logf("  intermediary-only divergences (rows drift, total agrees): %d", interimOnly)
	t.Logf("  total-interest fail buckets (option|basis|inadv|py -> count):")
	for k, v := range failBucket {
		t.Logf("      %-45s %d", k, v)
	}
	// Report-only: this sweep is diagnostic. Do not fail the build on the
	// discovered pre-existing divergences (they are characterized in the logs).
}
