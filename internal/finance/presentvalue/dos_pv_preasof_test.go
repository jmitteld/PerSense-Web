package presentvalue

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// runPVPeriodicOffOracle drives the DOS PV engine on a periodic stream that
// STARTS fromOffMonths before the as-of date (pv_oracle "periodic_off"), so the
// asof>fromdate accumulate-past leg of PRESVALU.pas Summation is exercised. The
// stock `periodic` command pins fromdate=asof and can never reach that branch.
func runPVPeriodicOffOracle(amt, rate float64, perYr, n, fromOff int, cola float64, cnt bool) (float64, bool) {
	args := []string{"periodic_off",
		strconv.FormatFloat(amt, 'f', 2, 64),
		strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(perYr), strconv.Itoa(n), strconv.Itoa(fromOff),
		strconv.FormatFloat(cola, 'f', 10, 64)}
	if cnt {
		args = append(args, "cnt")
	}
	out, err := exec.Command(pvOracleBin(), args...).Output()
	if err != nil {
		return 0, false
	}
	return parsePV(out)
}

// goPVPeriodicOff mirrors pv_oracle SetupPeriodicPVOff: as-of fixed at
// 2024-01-01, fromdate = as-of minus fromOffMonths, day=1, basis 360.
func goPVPeriodicOff(amt, rate float64, perYr, n, fromOff int, cola float64, cnt bool) float64 {
	addMonths := func(mo int) types.DateRec {
		yoff, moff := mo/12, mo%12
		if moff < 0 {
			moff += 12
			yoff--
		}
		return types.NewDateRec(2024+yoff, time.Month(moff+1), 1)
	}
	mPer := 12 / perYr
	cs := int8(types.StatusEmpty)
	if cola != 0 {
		cs = types.InOutInput
	}
	cm := types.COLAAnnual
	if cnt {
		cm = types.COLAContinuous
	}
	in := PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: addMonths(-fromOff),
			ToDateStatus: types.InOutInput, ToDate: addMonths(-fromOff + n*mPer),
			PerYrStatus: types.InOutInput, PerYr: perYr,
			AmtStatus: types.InOutInput, Amt: amt, COLAStatus: cs, COLA: cola, ValStatus: types.StatusEmpty}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
			SumValueStatus: types.StatusEmpty},
		Settings: PVSettings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, COLAMonth: cm},
	}
	return Calculate(in).SumValue
}

// TestDOSPVOraclePreAsOfSweep is the differential sweep the old periodic sweep
// could not be: it randomizes fromOff>0 so every stream begins BEFORE the as-of
// date, hitting the accumulate-past branch (PRESVALU.pas Summation
// since_from:=false). Half the cases carry COLA=0 — the exact configuration the
// fromdate=asof sweep masked, and where the port previously under-valued by ~2.7x.
func TestDOSPVOraclePreAsOfSweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s); build via TARGET=pv_oracle legacy/oracle/build_linux.sh", pvOracleBin())
	}
	rng := rand.New(rand.NewSource(20260706))
	checked, fails, maxRel := 0, 0, 0.0
	for i := 0; i < 600; i++ {
		amt := float64(100 + rng.Intn(20000))
		rate := 0.02 + rng.Float64()*0.13
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 2 + rng.Intn(30*perYr)
		// Start 1..20 years before the as-of, but never so early the stream
		// ends before the as-of (keep the as-of inside [from,to] most of the time).
		fromOff := (12 + rng.Intn(240))
		cola := 0.0
		if rng.Intn(2) == 0 {
			cola = rng.Float64() * 0.05
		}
		cnt := rng.Intn(2) == 0
		op, ok := runPVPeriodicOffOracle(amt, rate, perYr, n, fromOff, cola, cnt)
		if !ok {
			continue
		}
		gp := goPVPeriodicOff(amt, rate, perYr, n, fromOff, cola, cnt)
		checked++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > maxRel {
			maxRel = rel
		}
		if rel > 1e-6 {
			fails++
			if fails <= 12 {
				t.Errorf("PERIODIC_OFF amt=%.0f r=%.4f py=%d n=%d off=%d cola=%.4f cnt=%v: DOS=%.6f Go=%.6f (rel %.2e)",
					amt, rate, perYr, n, fromOff, cola, cnt, op, gp, rel)
			}
		}
	}
	t.Logf("periodic_off: checked %d, divergences %d, max relErr=%.2e", checked, fails, maxRel)
	if checked == 0 {
		t.Fatalf("no periodic_off cases ran — oracle at %s may predate the periodic_off mode", pvOracleBin())
	}
}

// TestPVCola0PreAsOfRegression pins the COLA=0 accumulate-past value that the
// shared-SumFormula bug got wrong (32,216.88 instead of 87,574.56). Runs without
// the oracle so it guards the fix in every `go test` run.
//
// Provenance (DOS oracle, pv_oracle built from legacy/oracle/pv_oracle.pas):
//
//	pv_oracle periodic_off 300 0.05 12 240 156 0  -> pv 87574.557698
//	pv_oracle periodic_off 1000 0.06 1 20 120 0   -> pv 22413.617444
//
// Before the fix the accumulate branch reused SumFormula(lnf,n) (the
// since_from anchoring); DOS Summation uses SumFormula(-lnf,n) there
// (PRESVALU.pas:438-447).
func TestPVCola0PreAsOfRegression(t *testing.T) {
	cases := []struct {
		amt, rate         float64
		perYr, n, fromOff int
		want              float64
	}{
		{300, 0.05, 12, 240, 156, 87574.557698},
		{1000, 0.06, 1, 20, 120, 22413.617444},
	}
	for _, c := range cases {
		got := goPVPeriodicOff(c.amt, c.rate, c.perYr, c.n, c.fromOff, 0, false)
		if math.Abs(got-c.want) > 0.01 {
			t.Errorf("COLA=0 pre-as-of amt=%.0f r=%.2f py=%d n=%d off=%d: Go=%.6f want DOS=%.6f (diff %.4f)",
				c.amt, c.rate, c.perYr, c.n, c.fromOff, got, c.want, got-c.want)
		}
	}
}
