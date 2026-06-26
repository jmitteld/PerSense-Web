package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// Actuarial contingency-SHAPE cube.
//
// Motivation. The amortization engine was hardened by sweeping its option
// space exhaustively ("shapes") and diffing every cell against the real DOS
// oracle (docs/exhaustive_option_sweep_plan.md, TestDOSGroundZeroRowCube).
// The actuarial / life-contingency paths cannot be swept against the DOS
// engine — the ACTUARY unit and the MALE/FEMALE mortality tables are missing
// from this snapshot (docs/actuarial_oracle_blocked.md). This cube is the
// faithful analog within that constraint: it sweeps the full actuarial shape
// space through the production PV dispatch and diffs each forward cell against
// an INDEPENDENT oracle, then round-trips each backward cell.
//
// The shape space (the "cube"):
//
//	payment kind   ∈ { lump, periodic }
//	contingency    ∈ { NotContingent, Living, Dead,
//	                   Only1Living, Only2Living, EitherLiving, BothLiving }
//	lives          ∈ { one life, two lives }   (implied by the contingency)
//	direction      ∈ { forward, solve-amount, solve-rate }
//	rate           ∈ { 0.04, 0.06 }
//
// The injected "special oracle profile". Every other PV actuarial test uses a
// mock quadratic qx curve. This cube instead injects the SOA Standard Ultimate
// Life Table (SULT), generated from Makeham's law (A=0.00022, B=2.7e-6,
// c=1.124) — a second, independently-reproducible mortality profile whose
// values the SOA publishes. Validating on a different profile guards against
// the engine accidentally depending on a quirk of the mock curve, and the
// Makeham closed form lets the oracle be computed in-process with no external
// dependency (so the cube always runs in CI, like TestActuarialSOAPublishedAnchor).
//
// Independence of the oracle. The expected forward PV is built here by an
// explicit summation over payment dates, reading survival straight out of the
// SULT lx column (kp_x = lx[ageAtPayment]/lx[ageNow]) and combining the two
// lives by the documented contingency algebra — it never calls the engine's
// LifeProb / discount / summation helpers. Dates are pinned to Jan-1 so the
// 30/360 year fraction is integral and ages index lx without interpolation,
// matching the engine exactly (verified by the canonical tests to 1e-9).

// sultCubeLx builds the SULT lx column (radix 100,000) from Makeham's law.
func sultCubeLx() []float64 {
	const A, B, c = 0.00022, 2.7e-6, 1.124
	lnc := math.Log(c)
	lx := make([]float64, 131)
	for a := 0; a <= 130; a++ {
		lx[a] = 100000.0 * math.Exp(-(A*float64(a) + (B/lnc)*(math.Pow(c, float64(a))-1)))
	}
	return lx
}

// cubeContingencies enumerates the contingency leg of the cube, tagging which
// cells need a second life (and therefore a second table on the config).
var cubeContingencies = []struct {
	name    string
	code    byte
	twoLife bool
}{
	{"NotContingent", actuarial.NotContingent, false},
	{"Living", actuarial.Living, false},
	{"Dead", actuarial.Dead, false},
	{"Only1Living", actuarial.Only1Living, true},
	{"Only2Living", actuarial.Only2Living, true},
	{"EitherLiving", actuarial.EitherLiving, true},
	{"BothLiving", actuarial.BothLiving, true},
}

// cubeWeight reproduces actuarial.LifeProb's contingency algebra independently,
// given the two single-life survival probabilities s1, s2. Mirrors
// contingency.go LifeProb exactly but is computed here from the raw lx column.
func cubeWeight(code byte, s1, s2 float64) float64 {
	switch code {
	case actuarial.NotContingent:
		return 1
	case actuarial.Living:
		return s1
	case actuarial.Dead:
		return 1 - s1
	case actuarial.Only1Living:
		return s1 * (1 - s2)
	case actuarial.Only2Living:
		return (1 - s1) * s2
	case actuarial.EitherLiving:
		return 1 - (1-s1)*(1-s2)
	case actuarial.BothLiving:
		return s1 * s2
	}
	return 1
}

// kpFromLx is the independent k-year survival from age x0 to age `age`,
// read straight out of the lx column (no engine helper).
func kpFromLx(lx []float64, x0, age int) float64 {
	if age <= x0 {
		return 1
	}
	if x0 < 0 || x0 >= len(lx) || lx[x0] == 0 || age >= len(lx) {
		return 0
	}
	return lx[age] / lx[x0]
}

// cubeConfig builds an ActuarialConfig that injects the SULT profile. For
// two-life cells it reuses the SAME SULT table for person 2 at a younger DOB,
// so the joint composition exercises genuinely different ages while keeping the
// oracle a single, known lx column.
func cubeConfig(asOf, dob1, dob2 types.DateRec, twoLife bool) *actuarial.ActuarialConfig {
	lx := sultCubeLx()
	cfg := &actuarial.ActuarialConfig{
		Table1: actuarial.NewLifeTableFromLx("SULT", lx),
		DOB1:   dob1,
		Now:    asOf,
	}
	if twoLife {
		cfg.Table2 = actuarial.NewLifeTableFromLx("SULT", lx)
		cfg.DOB2 = dob2
	}
	return cfg
}

// TestActuarialShapeCube_Forward is the core differential sweep. For every
// (payment-kind × contingency × rate) cell it forward-values the screen through
// the production Calculate and compares Sum Value to the independent SULT oracle.
func TestActuarialShapeCube_Forward(t *testing.T) {
	asOfY, dob1Y, dob2Y := 2026, 1956, 1961 // person 1 age 70, person 2 age 65
	asOf := dateOf(asOfY, time.January, 1)
	dob1 := dateOf(dob1Y, time.January, 1)
	dob2 := dateOf(dob2Y, time.January, 1)
	lx := sultCubeLx()
	x1now := asOfY - dob1Y
	x2now := asOfY - dob2Y

	// independent expected PV of a single dated payment
	expectOne := func(code byte, payY int, amt, rate float64, twoLife bool) float64 {
		s1 := kpFromLx(lx, x1now, payY-dob1Y)
		s2 := 1.0
		if twoLife {
			s2 = kpFromLx(lx, x2now, payY-dob2Y)
		}
		return amt * math.Exp(-rate*float64(payY-asOfY)) * cubeWeight(code, s1, s2)
	}

	lumpPayYears := []int{2027, 2031, 2036, 2046} // k = 1, 5, 10, 20
	const lumpAmt = 100000.0
	periodFrom, periodTo := 2027, 2041 // 15 annual payments
	const periodAmt = 12000.0
	rates := []float64{0.04, 0.06}

	cells, maxRel := 0, 0.0
	for _, c := range cubeContingencies {
		cfg := cubeConfig(asOf, dob1, dob2, c.twoLife)
		for _, rate := range rates {
			// ---- lump shape: one row per pay date ----
			for _, py := range lumpPayYears {
				res := Calculate(PVInput{
					Settings: vrTestSettings(),
					PresVal: PresValLine{
						AsOfStatus: types.InOutInput, AsOf: asOf,
						R: RateEntry{Status: types.InOutInput, Rate: rate},
					},
					LumpSums: []LumpSumPayment{{
						DateStatus: types.InOutInput, Date: dateOf(py, time.January, 1),
						AmtStatus: types.InOutInput, Amt: lumpAmt,
						Act: c.code,
					}},
					Actuarial: cfg,
				})
				if res.Err != nil {
					t.Errorf("lump %s r=%.2f py=%d: forward err: %v", c.name, rate, py, res.Err)
					continue
				}
				want := expectOne(c.code, py, lumpAmt, rate, c.twoLife)
				cells++
				rel := relErr(res.SumValue, want)
				if rel > maxRel {
					maxRel = rel
				}
				if rel > 1e-6 {
					t.Errorf("lump %s r=%.2f py=%d: engine=%.6f oracle=%.6f (rel %.2e)",
						c.name, rate, py, res.SumValue, want, rel)
				}
			}

			// ---- periodic shape: one annual stream ----
			res := Calculate(PVInput{
				Settings: vrTestSettings(),
				PresVal: PresValLine{
					AsOfStatus: types.InOutInput, AsOf: asOf,
					R: RateEntry{Status: types.InOutInput, Rate: rate},
				},
				Periodics: []PeriodicPayment{{
					FromDateStatus: types.InOutInput, FromDate: dateOf(periodFrom, time.January, 1),
					ToDateStatus: types.InOutInput, ToDate: dateOf(periodTo, time.January, 1),
					PerYrStatus: types.InOutInput, PerYr: 1,
					AmtStatus: types.InOutInput, Amt: periodAmt,
					Act: c.code,
				}},
				Actuarial: cfg,
			})
			if res.Err != nil {
				t.Errorf("periodic %s r=%.2f: forward err: %v", c.name, rate, res.Err)
				continue
			}
			want := 0.0
			for py := periodFrom; py <= periodTo; py++ {
				want += expectOne(c.code, py, periodAmt, rate, c.twoLife)
			}
			cells++
			rel := relErr(res.SumValue, want)
			if rel > maxRel {
				maxRel = rel
			}
			if rel > 1e-6 {
				t.Errorf("periodic %s r=%.2f: engine=%.6f oracle=%.6f (rel %.2e)",
					c.name, rate, res.SumValue, want, rel)
			}
		}
	}
	t.Logf("forward shape cube vs SULT oracle: %d cells, max rel err = %.2e", cells, maxRel)
}

// TestActuarialShapeCube_Backward round-trips the same shape space through the
// backward dispatch: forward-value, blank the solved-for field, feed the Sum
// Value back, and confirm the original input is recovered. Covers the
// solve-amount path (lump and periodic) and the solve-rate path across every
// contingency. Cells whose survival weight is too small for a stable inverse
// are skipped (the same guard the existing per-contingency tests use).
func TestActuarialShapeCube_Backward(t *testing.T) {
	asOfY, dob1Y, dob2Y := 2026, 1956, 1961
	asOf := dateOf(asOfY, time.January, 1)
	dob1 := dateOf(dob1Y, time.January, 1)
	dob2 := dateOf(dob2Y, time.January, 1)

	lumpPay := dateOf(2038, time.January, 1)    // k = 12
	periodFrom := dateOf(2030, time.January, 1) // mid ~2035
	periodTo := dateOf(2040, time.January, 1)
	probeMid := dateOf(2035, time.January, 1)
	const lumpAmt, periodAmt, wantRate = 100000.0, 6000.0, 0.055

	amtCells, rateCells := 0, 0
	for _, c := range cubeContingencies {
		cfg := cubeConfig(asOf, dob1, dob2, c.twoLife)

		// Skip degenerate cells: a near-zero contingency weight makes the
		// amount/rate inverse ill-conditioned (DOS guards this too).
		if p := cfg.LifeProb(probeMid, c.code); p < 0.02 {
			t.Logf("%s: skip backward (mid-stream prob %.4f too small)", c.name, p)
			continue
		}

		// ---- solve lump amount ----
		t.Run("lumpAmt/"+c.name, func(t *testing.T) {
			mk := func() PVInput {
				return PVInput{
					Settings: vrTestSettings(),
					PresVal: PresValLine{
						AsOfStatus: types.InOutInput, AsOf: asOf,
						R: RateEntry{Status: types.InOutInput, Rate: 0.06},
					},
					LumpSums: []LumpSumPayment{{
						DateStatus: types.InOutInput, Date: lumpPay,
						AmtStatus: types.InOutInput, Amt: lumpAmt, Act: c.code,
					}},
					Actuarial: cfg,
				}
			}
			fwd := Calculate(mk())
			if fwd.Err != nil {
				t.Fatalf("forward: %v", fwd.Err)
			}
			bwd := mk()
			bwd.LumpSums[0].AmtStatus = types.StatusEmpty
			bwd.LumpSums[0].Amt = 0
			bwd.PresVal.SumValueStatus = types.InOutInput
			bwd.PresVal.SumValue = fwd.SumValue
			res := Calculate(bwd)
			if res.Err != nil {
				t.Fatalf("backward: %v", res.Err)
			}
			if math.Abs(res.LumpSums[0].Amt-lumpAmt) > 0.5 {
				t.Errorf("solved lump amount = %.4f, want %.1f", res.LumpSums[0].Amt, lumpAmt)
			}
			amtCells++
		})

		// ---- solve periodic amount ----
		t.Run("periodicAmt/"+c.name, func(t *testing.T) {
			mk := func() PVInput {
				return PVInput{
					Settings: vrTestSettings(),
					PresVal: PresValLine{
						AsOfStatus: types.InOutInput, AsOf: asOf,
						R: RateEntry{Status: types.InOutInput, Rate: 0.06},
					},
					Periodics: []PeriodicPayment{{
						FromDateStatus: types.InOutInput, FromDate: periodFrom,
						ToDateStatus: types.InOutInput, ToDate: periodTo,
						PerYrStatus: types.InOutInput, PerYr: 12,
						AmtStatus: types.InOutInput, Amt: periodAmt, Act: c.code,
					}},
					Actuarial: cfg,
				}
			}
			fwd := Calculate(mk())
			if fwd.Err != nil {
				t.Fatalf("forward: %v", fwd.Err)
			}
			bwd := mk()
			bwd.Periodics[0].AmtStatus = types.StatusEmpty
			bwd.Periodics[0].Amt = 0
			bwd.PresVal.SumValueStatus = types.InOutInput
			bwd.PresVal.SumValue = fwd.SumValue
			res := Calculate(bwd)
			if res.Err != nil {
				t.Fatalf("backward: %v", res.Err)
			}
			if math.Abs(res.Periodics[0].Amt-periodAmt) > 0.5 {
				t.Errorf("solved periodic amount = %.4f, want %.1f", res.Periodics[0].Amt, periodAmt)
			}
			amtCells++
		})

		// ---- solve rate (mixed lump+periodic screen) ----
		t.Run("rate/"+c.name, func(t *testing.T) {
			mk := func() PVInput {
				return PVInput{
					Settings: vrTestSettings(),
					PresVal: PresValLine{
						AsOfStatus: types.InOutInput, AsOf: asOf,
						R: RateEntry{Status: types.InOutInput, Rate: wantRate},
					},
					LumpSums: []LumpSumPayment{{
						DateStatus: types.InOutInput, Date: lumpPay,
						AmtStatus: types.InOutInput, Amt: 50000, Act: c.code,
					}},
					Periodics: []PeriodicPayment{{
						FromDateStatus: types.InOutInput, FromDate: periodFrom,
						ToDateStatus: types.InOutInput, ToDate: periodTo,
						PerYrStatus: types.InOutInput, PerYr: 12,
						AmtStatus: types.InOutInput, Amt: 1000, Act: c.code,
					}},
					Actuarial: cfg,
				}
			}
			fwd := Calculate(mk())
			if fwd.Err != nil {
				t.Fatalf("forward: %v", fwd.Err)
			}
			bwd := mk()
			bwd.PresVal.R = RateEntry{Status: types.StatusEmpty}
			bwd.PresVal.SumValueStatus = types.InOutInput
			bwd.PresVal.SumValue = fwd.SumValue
			res := Calculate(bwd)
			if res.Err != nil {
				t.Fatalf("backward (rate): %v", res.Err)
			}
			if math.Abs(res.Rate-wantRate) > 1e-4 {
				t.Errorf("solved rate = %.6f, want %.6f", res.Rate, wantRate)
			}
			rateCells++
		})
	}
	t.Logf("backward shape cube: %d amount round-trips, %d rate round-trips", amtCells, rateCells)
}

// relErr is a relative error with an absolute floor so that near-zero expected
// values (e.g. a deep-future "Living" lump) don't blow the denominator up.
func relErr(got, want float64) float64 {
	d := math.Abs(got - want)
	if a := math.Abs(want); a > 1 {
		return d / a
	}
	return d
}
