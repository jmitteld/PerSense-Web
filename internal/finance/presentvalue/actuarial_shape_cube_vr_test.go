package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// Extensions to the actuarial contingency-shape cube
// (actuarial_shape_cube_test.go): the variable-rate (PVLX) contingent forward
// differential, plus the solve-as-of and standalone-POD-amount directions —
// the follow-ups called out in docs/actuarial_shape_cube_finding.md.
//
// These reuse the cube's shared helpers (cubeContingencies, cubeConfig,
// cubeWeight, kpFromLx, sultCubeLx, relErr) and the same injected SULT profile.

// cubeRateSchedule is a three-segment piecewise-constant true-rate schedule on
// Jan-1 boundaries. The first entry's date is ignored by the engine (its rate
// is the starting rate, in force from -infinity), matching DOS's read-only
// first-row date cell — so 0.05 is the base rate, 0.07 takes over at 2032, and
// 0.04 at 2040.
func cubeRateSchedule() []RateLine {
	return []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.05},
		{Date: dateOf(2032, time.January, 1), Rate: 0.07},
		{Date: dateOf(2040, time.January, 1), Rate: 0.04},
	}
}

// cubeVRIntegral independently integrates the piecewise rate from asOfY to payY
// over whole-year (Jan-1) segments — the same quantity integrateRateForward
// accumulates, but computed here without the engine. For each year Y in
// [asOfY, payY) the rate in force is the base rate (schedule[0]) overridden by
// the latest later entry whose year <= Y.
func cubeVRIntegral(asOfY, payY int) float64 {
	sched := cubeRateSchedule()
	rateInForce := func(year int) float64 {
		r := sched[0].Rate
		for i := 1; i < len(sched); i++ {
			if sched[i].Date.Time.Year() <= year {
				r = sched[i].Rate
			}
		}
		return r
	}
	total := 0.0
	for y := asOfY; y < payY; y++ {
		total += rateInForce(y)
	}
	return total
}

// TestActuarialShapeCube_VRForward sweeps the contingency × payment-kind space
// through the VARIABLE-rate forward dispatch (forwardVariableRate /
// vrPeriodicValue) and diffs each Sum Value against an independent oracle:
// amount × exp(-∫rate) × contingencyWeight, with the integral computed by
// cubeVRIntegral and survival read straight from the SULT lx column.
func TestActuarialShapeCube_VRForward(t *testing.T) {
	asOfY, dob1Y, dob2Y := 2026, 1956, 1961
	asOf := dateOf(asOfY, time.January, 1)
	dob1 := dateOf(dob1Y, time.January, 1)
	dob2 := dateOf(dob2Y, time.January, 1)
	lx := sultCubeLx()
	x1now, x2now := asOfY-dob1Y, asOfY-dob2Y

	expectOne := func(code byte, payY int, amt float64, twoLife bool) float64 {
		s1 := kpFromLx(lx, x1now, payY-dob1Y)
		s2 := 1.0
		if twoLife {
			s2 = kpFromLx(lx, x2now, payY-dob2Y)
		}
		df := math.Exp(-cubeVRIntegral(asOfY, payY))
		return amt * df * cubeWeight(code, s1, s2)
	}

	lumpPayYears := []int{2027, 2033, 2038, 2045} // straddle both rate breaks
	const lumpAmt = 100000.0
	periodFrom, periodTo := 2027, 2041
	const periodAmt = 12000.0

	cells, maxRel := 0, 0.0
	for _, c := range cubeContingencies {
		cfg := cubeConfig(asOf, dob1, dob2, c.twoLife)

		for _, py := range lumpPayYears {
			res := Calculate(PVInput{
				Settings: vrTestSettings(),
				PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
				LumpSums: []LumpSumPayment{{
					DateStatus: types.InOutInput, Date: dateOf(py, time.January, 1),
					AmtStatus: types.InOutInput, Amt: lumpAmt, Act: c.code,
				}},
				RateSchedule: cubeRateSchedule(),
				Actuarial:    cfg,
			})
			if res.Err != nil {
				t.Errorf("VR lump %s py=%d: %v", c.name, py, res.Err)
				continue
			}
			want := expectOne(c.code, py, lumpAmt, c.twoLife)
			cells++
			if rel := relErr(res.SumValue, want); rel > maxRel {
				maxRel = rel
			}
			if rel := relErr(res.SumValue, want); rel > 1e-6 {
				t.Errorf("VR lump %s py=%d: engine=%.6f oracle=%.6f (rel %.2e)",
					c.name, py, res.SumValue, want, rel)
			}
		}

		res := Calculate(PVInput{
			Settings: vrTestSettings(),
			PresVal:  PresValLine{AsOfStatus: types.InOutInput, AsOf: asOf},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(periodFrom, time.January, 1),
				ToDateStatus:   types.InOutInput, ToDate: dateOf(periodTo, time.January, 1),
				PerYrStatus:    types.InOutInput, PerYr: 1,
				AmtStatus:      types.InOutInput, Amt: periodAmt, Act: c.code,
			}},
			RateSchedule: cubeRateSchedule(),
			Actuarial:    cfg,
		})
		if res.Err != nil {
			t.Errorf("VR periodic %s: %v", c.name, res.Err)
			continue
		}
		want := 0.0
		for py := periodFrom; py <= periodTo; py++ {
			want += expectOne(c.code, py, periodAmt, c.twoLife)
		}
		cells++
		if rel := relErr(res.SumValue, want); rel > maxRel {
			maxRel = rel
		}
		if rel := relErr(res.SumValue, want); rel > 1e-6 {
			t.Errorf("VR periodic %s: engine=%.6f oracle=%.6f (rel %.2e)",
				c.name, res.SumValue, want, rel)
		}
	}
	t.Logf("VR forward shape cube vs piecewise-rate SULT oracle: %d cells, max rel err = %.2e", cells, maxRel)
}

// TestActuarialShapeCube_SolveAsOf round-trips the solve-as-of direction (PV-9)
// across every contingency: forward-value a contingent lump, blank the as-of
// date, feed the Sum Value back, and confirm the original as-of is recovered.
func TestActuarialShapeCube_SolveAsOf(t *testing.T) {
	asOfY := 2026
	asOf := dateOf(asOfY, time.January, 1)
	dob1 := dateOf(1956, time.January, 1)
	dob2 := dateOf(1961, time.January, 1)
	payDate := dateOf(2040, time.January, 1)
	probe := dateOf(2040, time.January, 1)
	const rate = 0.06

	solved := 0
	for _, c := range cubeContingencies {
		cfg := cubeConfig(asOf, dob1, dob2, c.twoLife)
		if p := cfg.LifeProb(probe, c.code); p < 0.02 {
			t.Logf("%s: skip as-of solve (prob %.4f too small)", c.name, p)
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			mk := func() PVInput {
				return PVInput{
					Settings: vrTestSettings(),
					PresVal: PresValLine{
						AsOfStatus: types.InOutInput, AsOf: asOf,
						R: RateEntry{Status: types.InOutInput, Rate: rate},
					},
					LumpSums: []LumpSumPayment{{
						DateStatus: types.InOutInput, Date: payDate,
						AmtStatus: types.InOutInput, Amt: 80000, Act: c.code,
					}},
					Actuarial: cfg,
				}
			}
			fwd := Calculate(mk())
			if fwd.Err != nil {
				t.Fatalf("forward: %v", fwd.Err)
			}
			bwd := mk()
			bwd.PresVal.AsOfStatus = types.StatusEmpty
			bwd.PresVal.AsOf = types.DateRec{}
			bwd.PresVal.SumValueStatus = types.InOutInput
			bwd.PresVal.SumValue = fwd.SumValue
			res := Calculate(bwd)
			if res.Err != nil {
				t.Fatalf("backward (as-of): %v", res.Err)
			}
			if off := math.Abs(yearsBetween(res.AsOf, asOf)); off > 0.05 {
				t.Errorf("%s: solved as-of = %v, want %v (off by %.3f yr)",
					c.name, res.AsOf.Time, asOf.Time, off)
			}
			solved++
		})
	}
	t.Logf("as-of solve round-trips across contingencies: %d solved", solved)
}

// TestActuarialShapeCube_SolvePOD round-trips the standalone Payment-on-Death
// amount solve (DOS ComputeUnknownPOD) while a life-contingent companion row is
// on the screen — once per contingency. The POD itself is not contingent, but
// this confirms the POD inversion folds correctly out of a Sum Value that also
// carries a survival-weighted row of each shape.
func TestActuarialShapeCube_SolvePOD(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob1 := dateOf(1956, time.January, 1)
	dob2 := dateOf(1961, time.January, 1)
	probe := dateOf(2035, time.January, 1)
	const wantPOD = 75000.0

	solved := 0
	for _, c := range cubeContingencies {
		base := cubeConfig(asOf, dob1, dob2, c.twoLife)
		if p := base.LifeProb(probe, c.code); p < 0.02 {
			t.Logf("%s: skip POD solve (companion-row prob %.4f too small)", c.name, p)
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			mkInput := func(a *actuarial.ActuarialConfig) PVInput {
				return PVInput{
					Settings: vrTestSettings(),
					PresVal: PresValLine{
						AsOfStatus: types.InOutInput, AsOf: asOf,
						R: RateEntry{Status: types.InOutInput, Rate: 0.05},
					},
					Periodics: []PeriodicPayment{{
						FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
						ToDateStatus:   types.InOutInput, ToDate: dateOf(2040, time.January, 1),
						PerYrStatus:    types.InOutInput, PerYr: 12,
						AmtStatus:      types.InOutInput, Amt: 1000, Act: c.code,
					}},
					Actuarial: a,
				}
			}
			known := *base
			known.POD = wantPOD
			fwd := Calculate(mkInput(&known))
			if fwd.Err != nil {
				t.Fatalf("forward: %v", fwd.Err)
			}
			unknown := *base
			unknown.POD = 0
			unknown.PODUnknown = true
			bwd := mkInput(&unknown)
			bwd.PresVal.SumValueStatus = types.InOutInput
			bwd.PresVal.SumValue = fwd.SumValue
			res := Calculate(bwd)
			if res.Err != nil {
				t.Fatalf("backward (POD): %v", res.Err)
			}
			if math.Abs(res.POD-wantPOD) > 1.0 {
				t.Errorf("%s: solved POD = %.4f, want %.1f", c.name, res.POD, wantPOD)
			}
			solved++
		})
	}
	t.Logf("standalone-POD solve round-trips across contingency companion rows: %d solved", solved)
}
