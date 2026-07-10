package amortization

import (
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestFancyAPRVsOracle extends the APR differential guard to FANCY loans — the gap
// left after TestAPRVsOracleSweep (which covers only a plain loan). The APR fix
// (§9) routes fancy loans through the same full-term value walk, but a balloon's
// terminal term or an ARM rate change inside the discount stream was never checked
// against DOS. Points are entered (3%) so the APR genuinely exercises the value
// discounting rather than echoing the nominal rate.
func TestFancyAPRVsOracle(t *testing.T) {
	gateOracle(t)
	const (
		amount = 100000.0
		rate   = 0.10
		nper   = 120
		perYr  = 12
		pay    = 1500.0
		pts    = 0.03
	)
	d := func(y, m, dd int) types.DateRec { return types.NewDateRec(y, time.Month(m), dd) }
	base := func() LoanInput {
		return LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: amount,
				LoanDateStatus: types.InOutInput, LoanDate: d(2024, 1, 1),
				LoanRateStatus: types.InOutInput, LoanRate: rate,
				FirstStatus: types.InOutInput, FirstDate: d(2024, 2, 1),
				NStatus: types.InOutInput, NPeriods: nper, PerYrStatus: types.InOutInput, PerYr: perYr,
				PayAmtStatus: types.InOutInput, PayAmt: pay,
				PointsStatus: types.InOutInput, Points: pts,
			},
			Settings: Settings{Basis: types.Basis360, PerYr: perYr, PlusRegular: false, YrDays: 360, YrInv: 1.0 / 360},
			Fancy:    true,
		}
	}
	oracleBase := []string{
		strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 6, 64),
		strconv.Itoa(nper), strconv.Itoa(perYr),
		"payhard=" + strconv.FormatFloat(pay, 'f', 2, 64), "pts=" + strconv.FormatFloat(pts, 'f', 4, 64), "apr",
	}

	cases := []struct {
		name   string
		mutate func(*LoanInput)
		tokens []string
	}{
		{"balloon", func(in *LoanInput) {
			in.Balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: d(2029, 1, 1), AmountStatus: types.InOutInput, Amount: 20000}}
		}, []string{"b60=20000"}},
		{"arm_rate_up", func(in *LoanInput) {
			in.Adjustments = []RateAdjustment{{DateStatus: types.InOutInput, Date: d(2029, 1, 1), LoanRateStatus: types.InOutInput, LoanRate: 0.13}}
		}, []string{"adj=60:0.13:"}},
		{"arm_rate_down", func(in *LoanInput) {
			in.Adjustments = []RateAdjustment{{DateStatus: types.InOutInput, Date: d(2029, 1, 1), LoanRateStatus: types.InOutInput, LoanRate: 0.06}}
		}, []string{"adj=60:0.06:"}},
		{"prepayment", func(in *LoanInput) {
			in.Prepayments = []Prepayment{{
				StartDateStatus: types.InOutInput, StartDate: d(2025, 1, 1),
				NNStatus: types.InOutInput, NN: 24,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: 500,
			}}
		}, []string{"pre=12:24:12:500"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			in := base()
			c.mutate(&in)
			res := Amortize(in)
			if res.Err != nil {
				t.Fatalf("Amortize: %v", res.Err)
			}
			args := append(append([]string{}, oracleBase...), c.tokens...)
			dosAPR, ok := runOracleAPR(t, args)
			if !ok {
				t.Skip("oracle produced no APR after retries")
			}
			if diff := res.APR - dosAPR; diff > 5e-5 || diff < -5e-5 {
				t.Errorf("fancy APR: engine=%.6f oracle=%.6f (Δ=%+.6f)", res.APR, dosAPR, diff)
			}
		})
	}
}
