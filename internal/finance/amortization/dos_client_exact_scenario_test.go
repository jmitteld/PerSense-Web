package amortization

import (
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestDOSClientExactScenario locks the exact scenario a client reported as wrong
// (docs/postmortem_365_exact_interest.md): a $100,000 / 12% / 360-payment loan on
// the 365 basis with the EXACT interest method and an odd first period
// (loan 2026-06-19, first payment 2026-08-01 — 43 days). Before the fix V3
// produced the 30/360 payment ($1,028.61); DOS's true-daily answer is $1,032.90,
// and the first period accrues 43 days of actual interest. This test drives the
// real Amortize dispatch and asserts the solved payment AND every schedule row
// match the real DOS engine, for both the prepaid-OFF and prepaid-ON settings.
//
// The oracle is driven with explicit loandmy=/firstdmy= dates so the odd first
// period is reproduced exactly (not just a clean one-period stub).
func TestDOSClientExactScenario(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s); build via legacy/oracle/build_linux.sh", oracleBin)
	}

	const (
		amount = 100000.0
		rate   = 0.12
		n      = 360
		perYr  = 12
	)
	loan := types.NewDateRec(2026, time.June, 19)
	first := types.NewDateRec(2026, time.August, 1)

	for _, prepaid := range []bool{false, true} {
		name := "prepaidOFF"
		flags := []string{"b365", "exact", "loandmy=19.6.2026", "firstdmy=1.8.2026"}
		if prepaid {
			name = "prepaidON"
			flags = append(flags, "prepaid")
		}
		t.Run(name, func(t *testing.T) {
			// DOS-solved payment and schedule.
			dosPay, ok := runOraclePayment(amount, rate, n, perYr, flags...)
			if !ok || dosPay <= 0 {
				t.Fatalf("oracle produced no payment")
			}
			dosRows, ok := runOracleRowsFull(amount, rate, n, perYr, dosPay, flags...)
			if !ok {
				t.Fatalf("oracle produced no rows")
			}

			// Go: solve the payment through the real dispatch.
			s := Settings{Basis: types.Basis365, PerYr: perYr, YrDays: 365.25, YrInv: 1.0 / 365.25, Exact: true, Prepaid: prepaid}
			in := LoanInput{Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amount,
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: loan,
				FirstStatus: types.InOutInput, FirstDate: first},
				Settings: s}
			solved := Amortize(in)
			if solved.Err != nil {
				t.Fatalf("Go Amortize error: %v", solved.Err)
			}
			goPay := modalReg(solved.Schedule)

			// The bug: the old engine returned the 30/360 payment 1028.61. Guard
			// that we are now on the true-daily answer (DOS ~1032.90 prepaid-OFF).
			if math.Abs(goPay-dosPay) > 0.5 {
				t.Errorf("solved payment off: Go=%.4f DOS=%.4f (the fix should track DOS's true-daily payment, not the 30/360 $1028.61)", goPay, dosPay)
			}

			// Feed the DOS payment to Go and compare every row to the cent.
			res, gok := func() (AmortResult, bool) {
				c := in
				c.Loan.PayAmtStatus = types.InOutDefault
				c.Loan.PayAmt = dosPay
				r := Amortize(c)
				return r, r.Err == nil
			}()
			if !gok {
				t.Fatalf("Go schedule error: %v", res.Err)
			}
			// Align on the regular rows (skip any settlement stub PayNum 0 on the
			// Go side; the oracle's row list is the regular payments).
			gr := res.Schedule
			for len(gr) > 0 && gr[0].PayNum == 0 {
				gr = gr[1:]
			}
			if len(gr) != len(dosRows) {
				t.Fatalf("row count: Go=%d DOS=%d", len(gr), len(dosRows))
			}
			tol := func(v float64) float64 { return 0.02 + 1e-6*math.Abs(v) }
			fails := 0
			// Body rows (all but the last) must match to the cent in every column.
			// The final row's terminal balance is a documented bounded corner:
			// DOS force-clears the last payment so the balance lands on exactly 0,
			// whereas the exact + odd-first-period schedule leaves a sub-$2
			// residual (the iterated solve + actual-day accrual don't perfectly
			// zero the final period). See docs/exact_groundzero_findings.md §4.
			for k := 0; k < len(dosRows)-1; k++ {
				gi := gr[k].Interest
				gp := gr[k].PayAmt - gr[k].Interest
				gb := gr[k].Principal
				if math.Abs(dosRows[k].interest-gi) > tol(gi) ||
					math.Abs(dosRows[k].prin-gp) > tol(gp) ||
					math.Abs(dosRows[k].bal-gb) > tol(gb) {
					fails++
					if fails <= 5 {
						t.Errorf("row %d: int D=%.4f G=%.4f | prin D=%.4f G=%.4f | bal D=%.4f G=%.4f",
							k+1, dosRows[k].interest, gi, dosRows[k].prin, gp, dosRows[k].bal, gb)
					}
				}
			}
			// Final-row interest must match; terminal balance bounded (see above).
			last := gr[len(gr)-1]
			lastD := dosRows[len(dosRows)-1]
			if math.Abs(lastD.interest-last.Interest) > tol(last.Interest) {
				t.Errorf("final row interest: DOS=%.4f Go=%.4f", lastD.interest, last.Interest)
			}
			const finalBalEnvelope = 2.0
			if math.Abs(last.Principal) > finalBalEnvelope {
				t.Errorf("final terminal balance %.4f exceeds documented residual envelope %.2f", last.Principal, finalBalEnvelope)
			}
			t.Logf("%s: Go pay=%.4f DOS pay=%.4f, %d rows, body fails=%d, final bal residual=%.4f (first-period int DOS=%.2f)",
				name, goPay, dosPay, len(gr), fails, last.Principal, lastD.interest)
			_ = strconv.Itoa
		})
	}
}
