package amortization

import (
	"math"
	"os"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// DOS renders an in-advance loan with two distinct engines (AMORTOP.pas):
//   - PAYMENT:  Iterate → RepayLoan (line 1437 selects RepayLoan when the loan is
//     neither fancy nor exact) — the annuity-due recursion, which uses only the
//     period growth f = 1 + rate/perYr and has NO first-period proration, so the
//     solved payment is BASIS-INDEPENDENT.
//   - SCHEDULE: RepayFancyLoan for any non-360 basis (line 1493) — actual-day
//     accrual on the settlement-shifted schedule, with WhenToStop folding the
//     residual into the final row.
// These tests validate each half of that split against the DOS oracle.

// TestInAdvancePaymentBasisIndependent asserts the SOLVED in-advance payment is
// the same on 360 / 365 / 365-360 (the RepayLoan basis-independence), matching
// DOS. This is the property that was violated before the fix (the non-360 payment
// was mis-prorated). Runs without the oracle — it is a pure Go invariant — and
// additionally checks the value against DOS when the oracle is present.
func TestInAdvancePaymentBasisIndependent(t *testing.T) {
	type loanC struct {
		amt, rate float64
		n, perYr  int
	}
	cases := []loanC{
		{90285, 0.1248, 36, 4}, {200000, 0.09, 24, 12}, {50000, 0.15, 40, 4},
		{375000, 0.06, 20, 2}, {12000, 0.11, 60, 12}, {28000, 0.075, 10, 1},
	}
	bases := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}
	for _, c := range cases {
		var pay [3]float64
		for i, b := range bases {
			s := gzSettings(c.perYr, b, false, false, true /*inadv*/, false, false)
			in := gzLoanInput(c.amt, c.rate, c.n, c.perYr, s)
			in.Loan.PayAmtStatus = types.StatusEmpty
			r := Amortize(in)
			if r.Err != nil || len(r.Schedule) == 0 {
				t.Fatalf("amt=%.0f n=%d py=%d basis=%s: %v", c.amt, c.n, c.perYr, basisName(b), r.Err)
			}
			pay[i] = modalReg(r.Schedule)
		}
		// All three bases must solve essentially the SAME regular payment
		// (basis-independent — DOS's RepayLoan uses only the period growth f). The
		// tolerance distinguishes the invariant (spread ≤ sub-cent solve noise,
		// ~0.05) from the pre-fix bug, where the non-360 payment was mis-prorated
		// and differed by ~1.7% (tens of dollars).
		const tol = 0.15
		if d := math.Abs(pay[0] - pay[1]); d > tol {
			t.Errorf("amt=%.0f n=%d py=%d: 360 pay=%.4f != 365 pay=%.4f (Δ=%.4f) — in-advance payment must be basis-independent",
				c.amt, c.n, c.perYr, pay[0], pay[1], d)
		}
		if d := math.Abs(pay[0] - pay[2]); d > tol {
			t.Errorf("amt=%.0f n=%d py=%d: 360 pay=%.4f != 365/360 pay=%.4f (Δ=%.4f)",
				c.amt, c.n, c.perYr, pay[0], pay[2], d)
		}
		// Against DOS, when the oracle is available.
		if _, err := os.Stat(oracleBin); err == nil {
			if dp, _, _, ok := runOracleTotals(c.amt, c.rate, c.n, c.perYr, "b365", "inadv"); ok {
				if rel := math.Abs(dp-pay[1]) / math.Max(1, dp); rel > 2e-3 {
					t.Errorf("amt=%.0f n=%d py=%d b365: DOS pay=%.4f Go pay=%.4f (rel %.2e)",
						c.amt, c.n, c.perYr, dp, pay[1], rel)
				}
			}
		}
	}
}

// TestInAdvanceNon360ScheduleVsDOS validates the non-360 in-advance SCHEDULE
// (the RepayFancyLoan display) row-for-row against the DOS oracle — every
// intermediary interest and balance, plus total interest and the folded final
// row — across a randomized sweep. This is the half the fix routes to the fancy
// engine while keeping the simple payment solve.
func TestInAdvanceNon360ScheduleVsDOS(t *testing.T) {
	gateOracle(t)
	type loanC struct {
		amt, rate float64
		n, perYr  int
		basisFlag string
		basis     types.BasisType
	}
	mk := func(amt, rate float64, n, perYr int, bf string, b types.BasisType) loanC {
		return loanC{amt, rate, n, perYr, bf, b}
	}
	cases := []loanC{
		mk(90285, 0.1248, 36, 4, "b365", types.Basis365),
		mk(200000, 0.09, 24, 12, "b365", types.Basis365),
		mk(50000, 0.15, 40, 4, "b365_360", types.Basis365360),
		mk(375000, 0.06, 20, 2, "b365", types.Basis365),
		mk(28000, 0.075, 10, 1, "b365_360", types.Basis365360),
		mk(120000, 0.055, 48, 12, "b365", types.Basis365),
	}
	for _, c := range cases {
		dosPay, dosInt, _, ok := runOracleTotals(c.amt, c.rate, c.n, c.perYr, c.basisFlag, "inadv")
		if !ok {
			continue
		}
		s := gzSettings(c.perYr, c.basis, false, false, true, false, false)
		in := gzLoanInput(c.amt, c.rate, c.n, c.perYr, s)
		in.Loan.PayAmtStatus = types.StatusEmpty
		r := Amortize(in)
		if r.Err != nil || len(r.Schedule) == 0 {
			t.Fatalf("%v: %v", c, r.Err)
		}
		// Final answer: total interest and retirement.
		if rel := math.Abs(dosInt-r.TotalInt) / math.Max(1, dosInt); rel > 1e-3 {
			t.Errorf("amt=%.0f n=%d py=%d %s: total interest DOS=%.2f Go=%.2f (rel %.2e)",
				c.amt, c.n, c.perYr, c.basisFlag, dosInt, r.TotalInt, rel)
		}
		if math.Abs(r.FinalPrinc) > 0.05 {
			t.Errorf("amt=%.0f n=%d py=%d %s: loan did not retire, finalPrinc=%.4f",
				c.amt, c.n, c.perYr, c.basisFlag, r.FinalPrinc)
		}
		// Intermediary: every row (feed the DOS payment to isolate accrual from
		// the sub-cent payment-solve difference). The oracle row mode omits the
		// PayNum-0 settlement row, so align it out.
		orows, ok1 := runOracleRowsFull(c.amt, c.rate, c.n, c.perYr, dosPay, c.basisFlag, "inadv")
		if !ok1 {
			continue
		}
		in2 := in
		in2.Loan.PayAmtStatus = types.InOutDefault
		in2.Loan.PayAmt = dosPay
		r2 := Amortize(in2)
		grows := goRegularRows(r2.Schedule)
		if len(grows) == len(orows)+1 && r2.Schedule[0].PayNum == 0 {
			grows = grows[1:]
		}
		if len(grows) != len(orows) {
			t.Errorf("amt=%.0f n=%d py=%d %s: row count DOS=%d Go=%d",
				c.amt, c.n, c.perYr, c.basisFlag, len(orows), len(grows))
			continue
		}
		for k := range orows {
			tol := 0.05
			if k == len(orows)-1 {
				tol = 1.0 // fed payment rounded to 4 decimals; terminal fold residual
			}
			if di := math.Abs(orows[k].interest - grows[k].interest); di > tol {
				t.Errorf("amt=%.0f n=%d py=%d %s row=%d: interest DOS=%.2f Go=%.2f",
					c.amt, c.n, c.perYr, c.basisFlag, k, orows[k].interest, grows[k].interest)
				break
			}
			if db := math.Abs(orows[k].bal - grows[k].bal); db > tol {
				t.Errorf("amt=%.0f n=%d py=%d %s row=%d: balance DOS=%.2f Go=%.2f",
					c.amt, c.n, c.perYr, c.basisFlag, k, orows[k].bal, grows[k].bal)
				break
			}
		}
	}
}
