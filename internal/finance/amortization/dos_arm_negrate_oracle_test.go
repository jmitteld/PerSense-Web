package amortization

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestARMNegativeRateVsOracle guards ARM rate adjustments that drop to a NEGATIVE
// rate — the regime TestDOSAdjustmentPerRowSweep explicitly excludes (it clamps
// newRate >= 0.01). A negative-rate adjustment produces negative-interest rows
// after the adjustment date; DOS runs it, and the engine must match. (The base
// rate solve and the AO6 implied-rate solve both had negative-value bugs this
// session — §10/§12 — so the negative ARM forward path deserves an explicit guard
// even though it currently matches.)
func TestARMNegativeRateVsOracle(t *testing.T) {
	gateOracle(t)

	const (
		amount   = 100000.0
		rate     = 0.10
		nper     = 120
		perYr    = 12
		adjMonth = 60
	)
	for _, newRate := range []float64{-0.03, -0.10, 0.02} {
		newRate := newRate
		t.Run(strconv.FormatFloat(newRate, 'f', 2, 64), func(t *testing.T) {
			in := LoanInput{
				Loan: Loan{
					AmountStatus: types.InOutInput, Amount: amount,
					LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
					LoanRateStatus: types.InOutInput, LoanRate: rate,
					FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
					NStatus: types.InOutInput, NPeriods: nper,
					PerYrStatus: types.InOutInput, PerYr: perYr,
					PayAmtStatus: types.StatusEmpty, PointsStatus: types.InOutInput,
				},
				Adjustments: []RateAdjustment{{
					DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1),
					LoanRateStatus: types.InOutInput, LoanRate: newRate,
				}},
				Settings: Settings{Basis: types.Basis360, PerYr: perYr, YrDays: 360, YrInv: 1.0 / 360},
				Fancy:    true,
			}
			res := Amortize(in)
			if res.Err != nil {
				t.Fatalf("Amortize: %v", res.Err)
			}

			// DOS rows via the oracle `rows` dump.
			out, err := exec.Command(oracleBin, strconv.FormatFloat(amount, 'f', 2, 64),
				strconv.FormatFloat(rate, 'f', 6, 64), strconv.Itoa(nper), strconv.Itoa(perYr),
				"adj="+strconv.Itoa(adjMonth)+":"+strconv.FormatFloat(newRate, 'f', 10, 64)+":", "rows").Output()
			if err != nil {
				t.Skipf("oracle rows failed: %v", err)
			}
			var dosInt, dosBal []float64
			for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				f := strings.Fields(ln)
				if len(f) < 8 || f[0] != "row" {
					continue
				}
				iv, _ := strconv.ParseFloat(f[3], 64)
				bv, _ := strconv.ParseFloat(f[7], 64)
				dosInt = append(dosInt, iv)
				dosBal = append(dosBal, bv)
			}
			// Collect Go regular rows (PayNum>=1) in order.
			var goInt, goBal []float64
			for i := range res.Schedule {
				if res.Schedule[i].PayNum >= 1 {
					goInt = append(goInt, res.Schedule[i].Interest)
					goBal = append(goBal, res.Schedule[i].Principal)
				}
			}
			if len(dosInt) == 0 {
				t.Skip("oracle emitted no rows")
			}
			if len(goInt) != len(dosInt) {
				t.Fatalf("row count: Go=%d DOS=%d", len(goInt), len(dosInt))
			}
			for k := range dosInt {
				if di := goInt[k] - dosInt[k]; di > 0.02 || di < -0.02 {
					t.Errorf("row %d interest: Go=%.2f DOS=%.2f", k+1, goInt[k], dosInt[k])
				}
				if db := goBal[k] - dosBal[k]; db > 0.02 || db < -0.02 {
					t.Errorf("row %d balance: Go=%.2f DOS=%.2f", k+1, goBal[k], dosBal[k])
				}
			}
		})
	}
}
