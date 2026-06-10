package presentvalue

import (
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

// pvOracleBin is the PV source-oracle built by:
//
//	TARGET=pv_oracle legacy/oracle/build_linux.sh
//
// Override with PERSENSE_PV_ORACLE.
func pvOracleBin() string {
	if p := os.Getenv("PERSENSE_PV_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/pv_oracle"
}

func parsePV(out []byte) (float64, bool) {
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) < 2 || f[0] != "pv" {
		return 0, false
	}
	v, err := strconv.ParseFloat(f[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func runPVLumpOracle(amount, rate float64, months int) (float64, bool) {
	out, err := exec.Command(pvOracleBin(), "lump",
		strconv.FormatFloat(amount, 'f', 2, 64),
		strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(months)).Output()
	if err != nil {
		return 0, false
	}
	return parsePV(out)
}

func runPVPeriodicOracle(amt, rate float64, perYr, n int, cola float64, cnt bool) (float64, bool) {
	args := []string{"periodic",
		strconv.FormatFloat(amt, 'f', 2, 64),
		strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(perYr), strconv.Itoa(n),
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

func goPVLump(amount, rate float64, months int) float64 {
	td := types.NewDateRec(2024+months/12, time.Month(months%12+1), 1)
	in := PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: td,
			AmtStatus: types.InOutInput, Amt: amount, ValStatus: types.StatusEmpty}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
			SumValueStatus: types.StatusEmpty},
		Settings: PVSettings{Basis: types.Basis360, PerYr: 1, YrDays: 360, YrInv: 1.0 / 360, COLAMonth: types.COLAAnnual},
	}
	return Calculate(in).SumValue
}

func goPVPeriodic(amt, rate float64, perYr, n int, cola float64, cnt bool) float64 {
	mPer := 12 / perYr
	totM := n * mPer
	td := types.NewDateRec(2024+totM/12, time.Month(totM%12+1), 1)
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
			FromDateStatus: types.InOutInput, FromDate: types.NewDateRec(2024, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: td, PerYrStatus: types.InOutInput, PerYr: perYr,
			AmtStatus: types.InOutInput, Amt: amt, COLAStatus: cs, COLA: cola, ValStatus: types.StatusEmpty}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
			SumValueStatus: types.StatusEmpty},
		Settings: PVSettings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, COLAMonth: cm},
	}
	return Calculate(in).SumValue
}

// TestDOSPVOracleSweep differentially validates the forward PV engine (lump and
// periodic streams, including annual-stepped and continuous COLA) against the
// real DOS PRESVALU engine compiled headlessly. Skips when the oracle binary is
// absent. Build it with: TARGET=pv_oracle legacy/oracle/build_linux.sh
func TestDOSPVOracleSweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s); build via TARGET=pv_oracle legacy/oracle/build_linux.sh", pvOracleBin())
	}
	rng := rand.New(rand.NewSource(20260613))

	// --- Lump sums ---
	lChecked, lFails, lMax := 0, 0, 0.0
	for i := 0; i < 400; i++ {
		amount := float64(1000 + rng.Intn(499000))
		rate := 0.005 + rng.Float64()*0.15
		months := 1 + rng.Intn(480)
		op, ok := runPVLumpOracle(amount, rate, months)
		if !ok {
			continue
		}
		gp := goPVLump(amount, rate, months)
		lChecked++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > lMax {
			lMax = rel
		}
		if rel > 1e-6 {
			lFails++
			if lFails <= 10 {
				t.Errorf("LUMP amt=%.0f r=%.4f mo=%d: DOS=%.6f Go=%.6f (rel %.2e)", amount, rate, months, op, gp, rel)
			}
		}
	}
	t.Logf("lump: checked %d, divergences %d, max relErr=%.2e", lChecked, lFails, lMax)

	// --- Periodic streams (both COLA modes) ---
	pChecked, pFails, pMax := 0, 0, 0.0
	var worst string
	for i := 0; i < 600; i++ {
		amt := float64(100 + rng.Intn(20000))
		rate := 0.01 + rng.Float64()*0.14
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 1 + rng.Intn(40*perYr)
		cola := 0.0
		if rng.Intn(2) == 0 {
			cola = rng.Float64() * 0.05 // up to 5%, below typical rate
		}
		cnt := rng.Intn(2) == 0
		op, ok := runPVPeriodicOracle(amt, rate, perYr, n, cola, cnt)
		if !ok {
			continue
		}
		gp := goPVPeriodic(amt, rate, perYr, n, cola, cnt)
		pChecked++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > pMax {
			pMax = rel
			worst = "amt=" + strconv.FormatFloat(amt, 'f', 0, 64) + " r=" + strconv.FormatFloat(rate, 'f', 4, 64) +
				" py=" + strconv.Itoa(perYr) + " n=" + strconv.Itoa(n) + " cola=" + strconv.FormatFloat(cola, 'f', 4, 64) +
				" cnt=" + strconv.FormatBool(cnt)
		}
		if rel > 1e-6 {
			pFails++
			if pFails <= 12 {
				t.Errorf("PERIODIC amt=%.0f r=%.4f py=%d n=%d cola=%.4f cnt=%v: DOS=%.6f Go=%.6f (rel %.2e)",
					amt, rate, perYr, n, cola, cnt, op, gp, rel)
			}
		}
	}
	t.Logf("periodic: checked %d, divergences %d, max relErr=%.2e at [%s]", pChecked, pFails, pMax, worst)
}

type rateStep struct {
	year int
	rate float64
}

func runVROracle(amount float64, months int, steps []rateStep) (float64, bool) {
	args := []string{"vr", strconv.FormatFloat(amount, 'f', 2, 64), strconv.Itoa(months), strconv.Itoa(len(steps))}
	for _, s := range steps {
		args = append(args, strconv.Itoa(s.year), strconv.FormatFloat(s.rate, 'f', 10, 64))
	}
	out, err := exec.Command(pvOracleBin(), args...).Output()
	if err != nil {
		return 0, false
	}
	return parsePV(out)
}

func goVRLump(amount float64, months int, steps []rateStep) float64 {
	td := types.NewDateRec(2024+months/12, time.Month(months%12+1), 1)
	sched := make([]RateLine, len(steps))
	for i, s := range steps {
		sched[i] = RateLine{Date: types.NewDateRec(s.year, time.January, 1), Rate: s.rate}
	}
	in := PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: td,
			AmtStatus: types.InOutInput, Amt: amount, ValStatus: types.StatusEmpty}},
		PresVal:      PresValLine{AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1)},
		RateSchedule: sched,
		Settings:     PVSettings{Basis: types.Basis360, PerYr: 1, YrDays: 360, YrInv: 1.0 / 360, COLAMonth: types.COLAAnnual},
	}
	return Calculate(in).SumValue
}

// TestDOSVROracleSweep validates the variable-rate (PVLfancy) discounting path
// against the real DOS fancy engine: a lump sum discounted through a randomized
// multi-step rate schedule to the as-of date. Skips when the oracle is absent.
func TestDOSVROracleSweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s); build via TARGET=pv_oracle legacy/oracle/build_linux.sh", pvOracleBin())
	}
	rng := rand.New(rand.NewSource(20260615))
	checked, fails, maxRel := 0, 0, 0.0
	var worst string
	for i := 0; i < 500; i++ {
		amount := float64(1000 + rng.Intn(499000))
		months := 1 + rng.Intn(440)
		nSteps := 1 + rng.Intn(5)
		steps := make([]rateStep, nSteps)
		// First step effective well before the as-of date; subsequent steps
		// at strictly-ascending years (DOS rejects out-of-order rate lines).
		steps[0] = rateStep{2000, 0.01 + rng.Float64()*0.14}
		yr := 2024
		for s := 1; s < nSteps; s++ {
			yr += 1 + rng.Intn(7)
			steps[s] = rateStep{yr, 0.01 + rng.Float64()*0.14}
		}
		op, ok := runVROracle(amount, months, steps)
		if !ok {
			continue
		}
		gp := goVRLump(amount, months, steps)
		checked++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > maxRel {
			maxRel = rel
			worst = "amt=" + strconv.FormatFloat(amount, 'f', 0, 64) + " mo=" + strconv.Itoa(months) + " steps=" + strconv.Itoa(nSteps)
		}
		if rel > 1e-6 {
			fails++
			if fails <= 12 {
				t.Errorf("VR amt=%.0f mo=%d nsteps=%d: DOS=%.6f Go=%.6f (rel %.2e)", amount, months, nSteps, op, gp, rel)
			}
		}
	}
	t.Logf("VR sweep: checked %d, divergences %d, max relErr=%.2e at [%s]", checked, fails, maxRel, worst)
}

// --- Backward solves ---

// runBkRateOracle solves the discount rate via the real DOS engine (FrontwardCalc
// Newton branch): a single lump of `amount` at `months`, target sumvalue.
func runBkRateOracle(sumvalue, amount float64, months int) (float64, bool) {
	out, err := exec.Command(pvOracleBin(), "bk_rate",
		strconv.FormatFloat(sumvalue, 'f', 10, 64),
		strconv.FormatFloat(amount, 'f', 10, 64),
		strconv.Itoa(months)).Output()
	if err != nil {
		return 0, false
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) < 2 || f[0] != "rate" {
		return 0, false
	}
	v, e := strconv.ParseFloat(f[1], 64)
	return v, e == nil
}

func goBkRate(sumvalue, amount float64, months int) (float64, bool) {
	td := types.NewDateRec(2024+months/12, time.Month(months%12+1), 1)
	in := PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: td,
			AmtStatus: types.InOutInput, Amt: amount, ValStatus: types.StatusEmpty}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: types.StatusEmpty}, // blank rate -> solve
			SumValueStatus: types.InOutInput, SumValue: sumvalue},
		Settings: PVSettings{Basis: types.Basis360, PerYr: 1, YrDays: 360, YrInv: 1.0 / 360, COLAMonth: types.COLAAnnual},
	}
	r := Calculate(in)
	if r.Err != nil {
		return 0, false
	}
	return r.Rate, true
}

// goBkLumpAmount solves a blank lump amount given a target sumvalue (PV-1).
func goBkLumpAmount(sumvalue, rate float64, months int) (float64, bool) {
	td := types.NewDateRec(2024+months/12, time.Month(months%12+1), 1)
	in := PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: td,
			AmtStatus: types.StatusEmpty, ValStatus: types.StatusEmpty}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
			SumValueStatus: types.InOutInput, SumValue: sumvalue},
		Settings: PVSettings{Basis: types.Basis360, PerYr: 1, YrDays: 360, YrInv: 1.0 / 360, COLAMonth: types.COLAAnnual},
	}
	r := Calculate(in)
	if r.Err != nil || len(r.LumpSums) == 0 {
		return 0, false
	}
	return r.LumpSums[0].Amt, true
}

// goBkPeriodicAmount solves a blank periodic amount given a target sumvalue (PV-4).
func goBkPeriodicAmount(sumvalue, rate float64, perYr, n int) (float64, bool) {
	mPer := 12 / perYr
	totM := n * mPer
	td := types.NewDateRec(2024+totM/12, time.Month(totM%12+1), 1)
	in := PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: types.NewDateRec(2024, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: td, PerYrStatus: types.InOutInput, PerYr: perYr,
			AmtStatus: types.StatusEmpty, COLAStatus: types.StatusEmpty, ValStatus: types.StatusEmpty}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1),
			R:              RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
			SumValueStatus: types.InOutInput, SumValue: sumvalue},
		Settings: PVSettings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, COLAMonth: types.COLAAnnual},
	}
	r := Calculate(in)
	if r.Err != nil || len(r.Periodics) == 0 {
		return 0, false
	}
	return r.Periodics[0].Amt, true
}

// TestDOSPVBackwardSweep validates the PV backward solvers. The RATE solve is
// diffed directly against the real DOS engine (FrontwardCalc Newton). The lump
// and periodic AMOUNT solves are validated by round-tripping through the
// bit-identical forward oracle: forward-compute the PV with the DOS engine,
// then confirm the Go backward solver recovers the original amount. Because the
// DOS amount-solve is the exact closed-form inverse of that forward, recovering
// the input through the DOS-faithful forward is equivalent to matching DOS.
func TestDOSPVBackwardSweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s); build via TARGET=pv_oracle legacy/oracle/build_linux.sh", pvOracleBin())
	}
	rng := rand.New(rand.NewSource(20260616))

	// (1) Rate solve — direct diff vs DOS.
	rChecked, rFails, rMax := 0, 0, 0.0
	for i := 0; i < 400; i++ {
		amount := float64(1000 + rng.Intn(499000))
		rate := 0.005 + rng.Float64()*0.14
		months := 6 + rng.Intn(360)
		sv, ok := runPVLumpOracle(amount, rate, months) // forward to get a consistent target
		if !ok {
			continue
		}
		dosR, ok1 := runBkRateOracle(sv, amount, months)
		goR, ok2 := goBkRate(sv, amount, months)
		if !ok1 || !ok2 {
			continue
		}
		rChecked++
		rel := math.Abs(dosR-goR) / math.Max(1e-6, math.Abs(goR))
		if rel > rMax {
			rMax = rel
		}
		if rel > 1e-6 {
			rFails++
			if rFails <= 10 {
				t.Errorf("RATE-solve amt=%.0f mo=%d sv=%.4f: DOS=%.8f Go=%.8f (rel %.2e)", amount, months, sv, dosR, goR, rel)
			}
		}
	}
	t.Logf("rate-solve (direct vs DOS): checked %d, divergences %d, max relErr=%.2e", rChecked, rFails, rMax)

	// (2) Lump amount solve — round-trip through the bit-validated forward oracle.
	lChecked, lFails, lMax := 0, 0, 0.0
	for i := 0; i < 400; i++ {
		amount := float64(1000 + rng.Intn(499000))
		rate := 0.005 + rng.Float64()*0.14
		months := 1 + rng.Intn(420)
		sv, ok := runPVLumpOracle(amount, rate, months)
		if !ok {
			continue
		}
		got, ok2 := goBkLumpAmount(sv, rate, months)
		if !ok2 {
			continue
		}
		lChecked++
		rel := math.Abs(got-amount) / math.Max(1, amount)
		if rel > lMax {
			lMax = rel
		}
		if rel > 1e-6 {
			lFails++
			if lFails <= 10 {
				t.Errorf("LUMP-amount amt=%.0f r=%.4f mo=%d: recovered=%.6f (rel %.2e)", amount, rate, months, got, rel)
			}
		}
	}
	t.Logf("lump-amount (round-trip thru DOS forward): checked %d, divergences %d, max relErr=%.2e", lChecked, lFails, lMax)

	// (3) Periodic amount solve — round-trip through the forward oracle.
	pChecked, pFails, pMax := 0, 0, 0.0
	for i := 0; i < 400; i++ {
		amt := float64(100 + rng.Intn(20000))
		rate := 0.01 + rng.Float64()*0.13
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 1 + rng.Intn(30*perYr)
		sv, ok := runPVPeriodicOracle(amt, rate, perYr, n, 0, false)
		if !ok {
			continue
		}
		got, ok2 := goBkPeriodicAmount(sv, rate, perYr, n)
		if !ok2 {
			continue
		}
		pChecked++
		rel := math.Abs(got-amt) / math.Max(1, amt)
		if rel > pMax {
			pMax = rel
		}
		if rel > 1e-6 {
			pFails++
			if pFails <= 10 {
				t.Errorf("PERIODIC-amount amt=%.0f r=%.4f py=%d n=%d: recovered=%.6f (rel %.2e)", amt, rate, perYr, n, got, rel)
			}
		}
	}
	t.Logf("periodic-amount (round-trip thru DOS forward): checked %d, divergences %d, max relErr=%.2e", pChecked, pFails, pMax)
}

func runVRPOracle(amt float64, perYr, n int, cola float64, steps []rateStep) (float64, bool) {
	args := []string{"vrp", strconv.FormatFloat(amt, 'f', 2, 64), strconv.Itoa(perYr),
		strconv.Itoa(n), strconv.FormatFloat(cola, 'f', 10, 64), strconv.Itoa(len(steps))}
	for _, s := range steps {
		args = append(args, strconv.Itoa(s.year), strconv.FormatFloat(s.rate, 'f', 10, 64))
	}
	out, err := exec.Command(pvOracleBin(), args...).Output()
	if err != nil {
		return 0, false
	}
	return parsePV(out)
}

func goVRPeriodic(amt float64, perYr, n int, cola float64, steps []rateStep) float64 {
	mPer := 12 / perYr
	totM := n * mPer
	td := types.NewDateRec(2024+totM/12, time.Month(totM%12+1), 1)
	cs := int8(types.StatusEmpty)
	if cola != 0 {
		cs = types.InOutInput
	}
	sched := make([]RateLine, len(steps))
	for i, s := range steps {
		sched[i] = RateLine{Date: types.NewDateRec(s.year, time.January, 1), Rate: s.rate}
	}
	in := PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: types.NewDateRec(2024, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: td, PerYrStatus: types.InOutInput, PerYr: perYr,
			AmtStatus: types.InOutInput, Amt: amt, COLAStatus: cs, COLA: cola, ValStatus: types.StatusEmpty}},
		PresVal:      PresValLine{AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1)},
		RateSchedule: sched,
		Settings:     PVSettings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, COLAMonth: types.COLAAnnual},
	}
	return Calculate(in).SumValue
}

// TestDOSVRPeriodicSweep validates a variable-rate PERIODIC stream (optionally
// COLA-escalating) discounted through a randomized multi-step rate schedule
// against the real DOS fancy engine (FancySummation).
func TestDOSVRPeriodicSweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s); build via TARGET=pv_oracle legacy/oracle/build_linux.sh", pvOracleBin())
	}
	rng := rand.New(rand.NewSource(20260622))
	checked, fails, maxRel := 0, 0, 0.0
	var worst string
	for i := 0; i < 500; i++ {
		amt := float64(100 + rng.Intn(20000))
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 2 + rng.Intn(30*perYr)
		cola := 0.0
		if rng.Intn(2) == 0 {
			cola = rng.Float64() * 0.04
		}
		nSteps := 1 + rng.Intn(4)
		steps := make([]rateStep, nSteps)
		steps[0] = rateStep{2000, 0.02 + rng.Float64()*0.12}
		yr := 2024
		for s := 1; s < nSteps; s++ {
			yr += 1 + rng.Intn(6)
			steps[s] = rateStep{yr, 0.02 + rng.Float64()*0.12}
		}
		op, ok := runVRPOracle(amt, perYr, n, cola, steps)
		if !ok {
			continue
		}
		gp := goVRPeriodic(amt, perYr, n, cola, steps)
		checked++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > maxRel {
			maxRel = rel
			worst = "amt=" + strconv.FormatFloat(amt, 'f', 0, 64) + " py=" + strconv.Itoa(perYr) + " n=" + strconv.Itoa(n) + " cola=" + strconv.FormatFloat(cola, 'f', 4, 64)
		}
		if rel > 1e-6 {
			fails++
			if fails <= 12 {
				t.Errorf("VRP amt=%.0f py=%d n=%d cola=%.4f steps=%d: DOS=%.6f Go=%.6f (rel %.2e)", amt, perYr, n, cola, nSteps, op, gp, rel)
			}
		}
	}
	t.Logf("VR periodic sweep: checked %d, divergences %d, max relErr=%.2e at [%s]", checked, fails, maxRel, worst)
}

// TestDOSPVMultiRowSweep validates multi-row PV worksheets (random mixes of
// lump and periodic lines, single rate) against the real DOS engine — the
// multi-line classification and cross-row summation.
func TestDOSPVMultiRowSweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s)", pvOracleBin())
	}
	rng := rand.New(rand.NewSource(20260625))
	checked, fails, maxRel := 0, 0, 0.0
	for i := 0; i < 500; i++ {
		rate := 0.01 + rng.Float64()*0.13
		nL := rng.Intn(4) // 0..3 lumps
		nP := rng.Intn(4) // 0..3 periodics
		if nL+nP == 0 {
			nL = 1
		}
		var toks []string
		var lumps []LumpSumPayment
		var pers []PeriodicPayment
		for j := 0; j < nL; j++ {
			months := 1 + rng.Intn(360)
			amt := float64(500 + rng.Intn(50000))
			toks = append(toks, "l"+strconv.Itoa(months)+"="+strconv.FormatFloat(amt, 'f', 2, 64))
			lumps = append(lumps, LumpSumPayment{DateStatus: types.InOutInput,
				Date:      types.NewDateRec(2024+months/12, time.Month(months%12+1), 1),
				AmtStatus: types.InOutInput, Amt: amt, ValStatus: types.StatusEmpty})
		}
		for j := 0; j < nP; j++ {
			perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
			n := 1 + rng.Intn(20*perYr)
			amt := float64(100 + rng.Intn(5000))
			toks = append(toks, "p"+strconv.FormatFloat(amt, 'f', 2, 64)+":"+strconv.Itoa(perYr)+":"+strconv.Itoa(n))
			totM := n * (12 / perYr)
			pers = append(pers, PeriodicPayment{FromDateStatus: types.InOutInput, FromDate: types.NewDateRec(2024, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: types.NewDateRec(2024+totM/12, time.Month(totM%12+1), 1),
				PerYrStatus: types.InOutInput, PerYr: perYr, AmtStatus: types.InOutInput, Amt: amt,
				COLAStatus: types.StatusEmpty, ValStatus: types.StatusEmpty})
		}
		args := append([]string{"multi", strconv.FormatFloat(rate, 'f', 10, 64)}, toks...)
		out, err := exec.Command(pvOracleBin(), args...).Output()
		if err != nil {
			continue
		}
		op, ok := parsePV(out)
		if !ok {
			continue
		}
		in := PVInput{
			LumpSums: lumps, Periodics: pers,
			PresVal: PresValLine{AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, 1),
				R: RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1}, SumValueStatus: types.StatusEmpty},
			Settings: PVSettings{Basis: types.Basis360, PerYr: 1, YrDays: 360, YrInv: 1.0 / 360, COLAMonth: types.COLAAnnual},
		}
		gp := Calculate(in).SumValue
		checked++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > maxRel {
			maxRel = rel
		}
		if rel > 1e-6 {
			fails++
			if fails <= 12 {
				t.Errorf("MULTI rate=%.4f nL=%d nP=%d: DOS=%.6f Go=%.6f (rel %.2e) toks=%v", rate, nL, nP, op, gp, rel, toks)
			}
		}
	}
	t.Logf("PV multi-row sweep: checked %d, divergences %d, max relErr=%.2e", checked, fails, maxRel)
}
