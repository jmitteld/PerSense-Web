package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestFancyTerminalMonotone locks in the Iterate-collapse breakthrough
// (docs/iterate_collapse_plan.md): the UNFORCED terminal (fancyTerminal) must be a
// MONOTONE, single-zero function of the regular payment — exactly DOS's Iterate/
// RepayFancyLoan (Output=nil) walk, which runs the FULL term with the full payment
// (no early minpmt stop, no final-row fold). An earlier one-sided `p < minPmt`
// stop grew a SPURIOUS SECOND zero on long-term odd-first balloon loans, which the
// Newton secant could root-switch onto (that was the last ~2e-3 divergence). This
// test asserts the terminal decreases across its root with no second sign change,
// on the exact loans that previously exhibited the spurious zero. If it regresses
// (early stop / fold reintroduced), the terminal becomes non-monotone and this
// fails before any oracle sweep would.
func TestFancyTerminalMonotone(t *testing.T) {
	type tc struct {
		amt, rate         float64
		n, perYr, firstMo int
		balloonMo         int
		balloonAmt        float64
	}
	cases := []tc{
		// The case that root-switched before the fix (n=288 odd-first balloon).
		{147496, 0.1448, 288, 12, 2, 286, 30645.88},
		// The overpay-trace case (annual, odd first, off-cycle balloon).
		{454603, 0.0875, 14, 1, 3, 24, 39407.87},
		// A shorter balloon + odd first for breadth.
		{203914, 0.1483, 48, 2, 2, 138, 30703.80},
	}
	for _, c := range cases {
		fy, fm := 2024+c.firstMo/12, time.Month(c.firstMo%12+1)
		by, bm := 2024+c.balloonMo/12, time.Month(c.balloonMo%12+1)
		in := LoanInput{
			Loan: Loan{AmountStatus: types.InOutInput, Amount: c.amt,
				LoanRateStatus: types.InOutInput, LoanRate: c.rate,
				NStatus: types.InOutInput, NPeriods: c.n, PerYrStatus: types.InOutInput, PerYr: c.perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
				FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(fy, fm, 1)},
			Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(by, bm, 1),
				AmountStatus: types.InOutInput, Amount: c.balloonAmt}},
			Fancy:    true,
			Settings: Settings{Basis: types.Basis360, PerYr: byte(c.perYr), YrDays: 360, YrInv: 1.0 / 360},
		}
		s := &in.Settings
		fg := GrowthPerPeriod(&in.Loan, s.YrInv)
		tr, _ := ComputeTrueRate(&in.Loan, s)
		// The solved payment is the root; sweep a wide band around it and confirm the
		// terminal is monotone NON-INCREASING with exactly one sign change.
		root := modalReg(Amortize(in).Schedule)
		if root <= 0 {
			t.Fatalf("amt=%.0f n=%d: no solved payment", c.amt, c.n)
		}
		prev := math.Inf(1)
		signChanges := 0
		var lastSign int
		lo, hi := root*0.5, root*1.6
		const steps = 60
		for i := 0; i <= steps; i++ {
			x := lo + (hi-lo)*float64(i)/steps
			term := fancyTerminal(in, x, s, tr, fg)
			if term > prev+1e-6 { // must be non-increasing in x
				t.Errorf("amt=%.0f n=%d: terminal NOT monotone at x=%.2f (term=%.2f rose from %.2f) — early stop/fold likely reintroduced",
					c.amt, c.n, x, term, prev)
				break
			}
			prev = term
			sgn := 0
			if term > 0 {
				sgn = 1
			} else if term < 0 {
				sgn = -1
			}
			if sgn != 0 && lastSign != 0 && sgn != lastSign {
				signChanges++
			}
			if sgn != 0 {
				lastSign = sgn
			}
		}
		if signChanges != 1 {
			t.Errorf("amt=%.0f n=%d: terminal has %d sign changes across the payment band (want exactly 1 — a single root)",
				c.amt, c.n, signChanges)
		}
	}
}
