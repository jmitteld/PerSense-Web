package amortization

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// runOracleDateFromBalance execs the DOS oracle's `datefrombalance=AMOUNT` query
// and returns the DOS-solved date (month, day, year). Retries the heap glitch.
func runOracleDateFromBalance(t *testing.T, args []string) (m, d, y int, ok bool) {
	t.Helper()
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) < 2 || f[0] != "date" {
			continue
		}
		p := strings.Split(f[1], "/")
		if len(p) != 3 {
			continue
		}
		m, _ = strconv.Atoi(p[0])
		d, _ = strconv.Atoi(p[1])
		y, _ = strconv.Atoi(p[2])
		return m, d, y, true
	}
	return 0, 0, 0, false
}

// TestDateFromBalanceVsOracle_Arrears guards ComputeDateFromBalanceDOS against the
// DOS oracle. This inverse lookup (balance→date) was previously computed only by a
// naive row-snap (DateForBalance, and a client-side JS copy) that lands ~one
// payment off DOS's ComputeDateFromBalance. The arrears rule (principal+payamt <
// target) matches DOS to the day across the balance range.
func TestDateFromBalanceVsOracle_Arrears(t *testing.T) {
	gateOracle(t)

	const (
		amount = 100000.0
		rate   = 0.10
		nper   = 120
		perYr  = 12
		pay    = 1500.0
	)
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
			NStatus: types.InOutInput, NPeriods: nper,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus: types.InOutInput, PayAmt: pay,
			PointsStatus: types.InOutInput,
		},
		Settings: Settings{Basis: types.Basis360, PerYr: perYr, YrDays: 360, YrInv: 1.0 / 360},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize: %v", res.Err)
	}

	for _, tgt := range []float64{80000, 70000, 60000, 50000, 40000, 30000, 20000, 10000} {
		tgt := tgt
		t.Run(strconv.Itoa(int(tgt)), func(t *testing.T) {
			gd, _, ok := ComputeDateFromBalanceDOS(res.Schedule, tgt, false)
			if !ok {
				t.Fatalf("ComputeDateFromBalanceDOS: target %.0f not reached", tgt)
			}
			m, d, y, ok := runOracleDateFromBalance(t, []string{
				strconv.FormatFloat(amount, 'f', 2, 64),
				strconv.FormatFloat(rate, 'f', 6, 64),
				strconv.Itoa(nper), strconv.Itoa(perYr),
				"payhard=" + strconv.FormatFloat(pay, 'f', 2, 64),
				"datefrombalance=" + strconv.FormatFloat(tgt, 'f', 2, 64),
			})
			if !ok {
				t.Skip("oracle produced no date after retries (heap glitch)")
			}
			if int(gd.Time.Month()) != m || gd.Time.Day() != d || gd.Time.Year() != y {
				t.Errorf("date-from-balance %.0f: engine=%02d/%02d/%d oracle=%02d/%02d/%d",
					tgt, gd.Time.Month(), gd.Time.Day(), gd.Time.Year(), m, d, y)
			}
		})
	}
}
