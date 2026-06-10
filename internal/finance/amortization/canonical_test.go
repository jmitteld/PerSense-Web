package amortization

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Canonical (first-principles) validation of the vanilla amortization
// schedule against textbook annuity mathematics, computed independently
// in the test (not via the engine's own helpers). A vanilla loan here is
// arrears, no prepaid/in-advance/R78/daily, with the loan date exactly
// one period before the first payment so the first period is full
// (prorate == 1). For such a loan, with per-period growth f = 1 + i:
//
//	payment d         = A·(f-1) / (1 - f^-n)
//	balance after k    = A·f^k - d·(f^k - 1)/(f - 1)
//	interest in period = balance_before·(f-1)
//
// The engine solves the payment (left blank) so the schedule is not
// penny-rounded, and should match these closed forms to ~1e-7.

type vanilla struct {
	amount float64
	rate   float64
	n      int
	perYr  int
}

func (v vanilla) f() float64 { return 1 + v.rate/float64(v.perYr) }

func (v vanilla) payment() float64 {
	f := v.f()
	return v.amount * (f - 1) / (1 - math.Pow(f, -float64(v.n)))
}

func (v vanilla) balance(k int) float64 {
	f := v.f()
	return v.amount*math.Pow(f, float64(k)) - v.payment()*(math.Pow(f, float64(k))-1)/(f-1)
}

func (v vanilla) run(t *testing.T) AmortResult {
	t.Helper()
	in := LoanInput{
		Loan: Loan{
			AmountStatus:   types.InOutInput, Amount: v.amount,
			LoanRateStatus: types.InOutInput, LoanRate: v.rate,
			NStatus:        types.InOutInput, NPeriods: v.n,
			PerYrStatus:    types.InOutInput, PerYr: v.perYr,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus:    types.InOutInput, FirstDate: onetPeriodAfter(2024, v.perYr),
			PayAmtStatus:   types.StatusEmpty, // engine solves -> no hard rounding
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(v.perYr), YrDays: 360, YrInv: 1.0 / 360},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("%+v: %v", v, res.Err)
	}
	return res
}

// onetPeriodAfter returns 2024-01-01 advanced by one payment period, so
// the first regular period is exactly full.
func onetPeriodAfter(year, perYr int) types.DateRec {
	switch perYr {
	case 12:
		return types.NewDateRec(year, 2, 1) // +1 month
	case 4:
		return types.NewDateRec(year, 4, 1) // +1 quarter
	case 2:
		return types.NewDateRec(year, 7, 1) // +6 months
	case 1:
		return types.NewDateRec(year+1, 1, 1) // +1 year
	default:
		return types.NewDateRec(year, 2, 1)
	}
}

func TestCanonicalVanillaSchedule(t *testing.T) {
	cases := []vanilla{
		{200000, 0.06, 360, 12},
		{50000, 0.08, 60, 12},
		{1000000, 0.045, 180, 12},
		{25000, 0.10, 20, 4},
		{100000, 0.05, 30, 1},
	}
	for _, v := range cases {
		v := v
		t.Run("", func(t *testing.T) {
			res := v.run(t)
			f := v.f()

			// Solved payment matches the annuity formula.
			if got := res.Schedule[0].PayAmt; relDiff(got, v.payment()) > 1e-6 {
				t.Errorf("%+v: payment engine=%.6f want=%.6f", v, got, v.payment())
			}

			// Per-period interest and remaining balance match the closed
			// forms at a sweep of periods (head, middle, tail).
			ks := []int{1, 2, v.n / 2, v.n - 1, v.n}
			for _, k := range ks {
				if k < 1 || k > v.n {
					continue
				}
				wantInt := v.balance(k-1) * (f - 1)
				if got := res.Schedule[k-1].Interest; relDiff(got, wantInt) > 1e-6 {
					t.Errorf("%+v k=%d: interest engine=%.6f want=%.6f", v, k, got, wantInt)
				}
				// Last period's residual balance is forced to 0; the
				// closed form also lands at ~0 there.
				wantBal := v.balance(k)
				if got := res.Schedule[k-1].Principal; math.Abs(got-wantBal) > 0.01 {
					t.Errorf("%+v k=%d: balance engine=%.6f want=%.6f", v, k, got, wantBal)
				}
			}

			// The loan fully amortizes and totals reconcile.
			if math.Abs(res.FinalPrinc) > 0.01 {
				t.Errorf("%+v: FinalPrinc=%.6f, want ~0", v, res.FinalPrinc)
			}
			if d := math.Abs(res.TotalPaid - res.TotalInt - v.amount); d > 0.02 {
				t.Errorf("%+v: TotalPaid(%.4f) - TotalInt(%.4f) != amount(%.4f), off %.4f",
					v, res.TotalPaid, res.TotalInt, v.amount, d)
			}
		})
	}
}

func relDiff(a, b float64) float64 {
	if b == 0 {
		return math.Abs(a)
	}
	return math.Abs(a-b) / math.Abs(b)
}
