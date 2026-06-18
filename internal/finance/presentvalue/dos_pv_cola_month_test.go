package presentvalue

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// runPVPeriodicMonthOracle drives the PV oracle's periodic mode with a
// month-specific COLA schedule (ParamStr 7 = 1..12).
func runPVPeriodicMonthOracle(amt, rate float64, perYr, n int, cola float64, month int) (float64, bool) {
	args := []string{"periodic",
		strconv.FormatFloat(amt, 'f', 2, 64),
		strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(perYr), strconv.Itoa(n),
		strconv.FormatFloat(cola, 'f', 10, 64),
		strconv.Itoa(month)}
	out, err := exec.Command(pvOracleBin(), args...).Output()
	if err != nil {
		return 0, false
	}
	return parsePV(out)
}

func goPVPeriodicMonth(amt, rate float64, perYr, n int, cola float64, month int) float64 {
	mPer := 12 / perYr
	totM := n * mPer
	td := types.NewDateRec(2024+totM/12, time.Month(totM%12+1), 1)
	in := PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: types.NewDateRec(2024, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: td, PerYrStatus: types.InOutInput, PerYr: perYr,
			AmtStatus: types.InOutInput, Amt: amt, COLAStatus: types.InOutInput, COLA: cola, ValStatus: types.StatusEmpty}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
			SumValueStatus: types.StatusEmpty},
		Settings: PVSettings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, COLAMonth: byte(month)},
	}
	return Calculate(in).SumValue
}

// TestDOSPVColaMonthSweep closes the last documented settings gap: month-specific
// COLA escalation (the "COLA escalation month = January…December" option) was
// never DOS-grounded — only the anniversary and continuous modes were. It sweeps
// all 12 calendar months × a grid of amount/rate/perYr/term/cola against the real
// DOS engine (df.c.colamonth = 1..12, SummationForSteppedCola). Anniversary (ANN)
// and continuous (CNT) remain covered by TestDOSPVOracleSweep.
func TestDOSPVColaMonthSweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s)", pvOracleBin())
	}
	rng := rand.New(rand.NewSource(0xc01a3))
	perMonth := map[int]int{}
	checked, fails := 0, 0
	maxRel := 0.0
	var worst string
	for i := 0; i < 1200; i++ {
		month := 1 + rng.Intn(12)
		amt := float64(200 + rng.Intn(15000))
		rate := 0.02 + rng.Float64()*0.12
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 2 + rng.Intn(20*perYr)
		cola := 0.005 + rng.Float64()*0.045 // always non-zero so the month matters

		op, ok := runPVPeriodicMonthOracle(amt, rate, perYr, n, cola, month)
		if !ok {
			continue
		}
		gp := goPVPeriodicMonth(amt, rate, perYr, n, cola, month)
		checked++
		perMonth[month]++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > maxRel {
			maxRel = rel
			worst = fmt.Sprintf("month=%d amt=%.0f r=%.4f py=%d n=%d cola=%.4f DOS=%.6f Go=%.6f", month, amt, rate, perYr, n, cola, op, gp)
		}
		if rel > 1e-6 {
			fails++
			if fails <= 12 {
				t.Errorf("COLA month=%d amt=%.0f r=%.4f py=%d n=%d cola=%.4f: DOS=%.6f Go=%.6f (rel %.2e)",
					month, amt, rate, perYr, n, cola, op, gp, rel)
			}
		}
	}
	for m := 1; m <= 12; m++ {
		if perMonth[m] == 0 {
			t.Errorf("COLA month %d never exercised — oracle may be flaking", m)
		}
	}
	if checked < 600 {
		t.Fatalf("only %d checked — oracle may be flaking", checked)
	}
	t.Logf("PV month-specific COLA vs DOS: checked %d across all 12 months, divergences %d, max relErr=%.2e [%s]",
		checked, fails, maxRel, worst)
}
