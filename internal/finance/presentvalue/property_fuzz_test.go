package presentvalue

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// Property-based / differential testing harness for the present-value
// engine. It generates thousands of randomized-but-valid worksheets and
// asserts properties that must hold for ANY input — finiteness,
// additivity, rate monotonicity, contingency ordering, and backward
// round-trip recovery. Hand-written tests check the cases we thought of;
// this checks the cases we didn't, with a fixed seed so any failure
// reproduces exactly.
//
// The same generator is the front half of the planned binary-oracle
// differential rig (docs/fidelity_validation_roadmap.md §3): an Oracle
// implementation that drives legacy Persense.exe via its worksheet
// file I/O can be dropped in below, and `checkAgainstOracle` will diff
// the Go engine against the real product over the same random sweep.
// Until that host-side oracle exists, NoopOracle is used and only the
// self-properties run — all of which execute here, in CI, today.

// Oracle is the pluggable reference implementation the generated
// worksheets are checked against. The Go engine is always checked
// against the self-properties; an external Oracle (the DOS/Windows
// binary) additionally pins absolute values.
type Oracle interface {
	// PV returns the reference Sum Value for a fully-specified forward
	// worksheet, or ok=false if this oracle can't evaluate it.
	PV(in PVInput) (sumValue float64, ok bool)
}

// NoopOracle is the placeholder used until the binary oracle is wired in
// on a Windows/Wine host. It declines every case, so only the
// self-properties run.
type NoopOracle struct{}

func (NoopOracle) PV(PVInput) (float64, bool) { return 0, false }

// activeOracle is swapped for a real binary-backed Oracle on a host that
// has Persense.exe. See docs/fidelity_validation_roadmap.md §3.
var activeOracle Oracle = NoopOracle{}

// fuzzScenario is one randomly generated, fully-specified forward
// worksheet plus the metadata the property checks need.
type fuzzScenario struct {
	input      PVInput
	asOf       types.DateRec
	rate       float64
	contingent bool
}

func genScenario(rng *rand.Rand) fuzzScenario {
	// Vary the as-of date instead of pinning it — exercises the age/date
	// arithmetic across years and months. Payments are generated strictly
	// after as-of (see below) so the "all rows future" assumption the
	// monotonicity property relies on still holds.
	asOfYear := 2024 + rng.Intn(6) // 2024..2029
	asOf := dateOf(asOfYear, time.Month(1+rng.Intn(12)), 1)
	rate := 0.005 + rng.Float64()*0.18 // 0.5%..18.5%

	in := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: rate},
		},
	}

	// Optional life contingency. ~40% of scenarios are contingent; of those,
	// ~35% configure a SECOND life so the two-life contingency codes
	// (Only 1 / Only 2 / Either / Both Living) are exercised — previously this
	// generator only ever produced single-life Living/Dead, leaving the
	// combinatorial two-life routing to a single hand-written example.
	var cfg *actuarial.ActuarialConfig
	contingent := rng.Float64() < 0.4
	twoLife := false
	if contingent {
		dobYear := 1940 + rng.Intn(30) // age 56..86 at as-of
		cfg = actuarialTestCfg(asOf, dateOf(dobYear, time.January, 1))
		if rng.Float64() < 0.35 {
			twoLife = true
			cfg.Table2 = secondMockTable()
			cfg.DOB2 = dateOf(1940+rng.Intn(30), time.Month(1+rng.Intn(12)), 1)
		}
		in.Actuarial = cfg
	}
	pickAct := func() byte {
		if !contingent || rng.Float64() >= 0.7 {
			return actuarial.NotContingent
		}
		if twoLife {
			switch rng.Intn(6) {
			case 0:
				return actuarial.Only1Living
			case 1:
				return actuarial.Only2Living
			case 2:
				return actuarial.EitherLiving
			case 3:
				return actuarial.BothLiving
			case 4:
				return actuarial.Living
			default:
				return actuarial.Dead
			}
		}
		if rng.Float64() < 0.5 {
			return actuarial.Living
		}
		return actuarial.Dead
	}

	// 1..3 lump sums, all strictly after as-of.
	for n := rng.Intn(3) + 1; n > 0; n-- {
		yr := asOfYear + 1 + rng.Intn(25)
		in.LumpSums = append(in.LumpSums, LumpSumPayment{
			DateStatus: types.InOutInput, Date: dateOf(yr, time.Month(1+rng.Intn(12)), 1),
			AmtStatus: types.InOutInput, Amt: float64(1000 + rng.Intn(100000)),
			Act: pickAct(),
		})
	}
	// 0..2 periodic streams, all strictly after as-of.
	for n := rng.Intn(3); n > 0; n-- {
		from := asOfYear + 1 + rng.Intn(10)
		to := from + 1 + rng.Intn(20)
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		in.Periodics = append(in.Periodics, PeriodicPayment{
			FromDateStatus: types.InOutInput, FromDate: dateOf(from, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(to, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: perYr,
			AmtStatus: types.InOutInput, Amt: float64(100 + rng.Intn(5000)),
			Act: pickAct(),
		})
	}
	return fuzzScenario{input: in, asOf: asOf, rate: rate, contingent: contingent}
}

// secondMockTable builds a distinct single-life table for Person 2 so two-life
// contingencies combine two genuinely different survival curves (slightly
// lighter mortality than actuarialTestCfg's Person-1 mock).
func secondMockTable() *actuarial.LifeTable {
	qx := make([]float64, 121)
	for i := range qx {
		qx[i] = 0.0008 + 0.00008*float64(i)*float64(i)/120.0
		if qx[i] > 1 {
			qx[i] = 1
		}
	}
	qx[120] = 1
	return actuarial.NewLifeTableFromQx("mock2", qx)
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// TestPropertyForwardInvariants sweeps random worksheets and asserts the
// always-true forward properties: every value is finite, and the screen
// Sum Value equals the sum of the row values plus any POD.
func TestPropertyForwardInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(0xC0FFEE))
	const N = 3000
	for i := 0; i < N; i++ {
		sc := genScenario(rng)
		res := Calculate(sc.input)
		if res.Err != nil {
			continue // an under/over-determined random draw; not a forward case
		}
		sum := 0.0
		for _, ls := range res.LumpSums {
			if !finite(ls.Val) {
				t.Fatalf("seed-iter %d: non-finite lump value %v", i, ls.Val)
			}
			sum += ls.Val
		}
		for _, pp := range res.Periodics {
			if !finite(pp.Val) {
				t.Fatalf("seed-iter %d: non-finite periodic value %v", i, pp.Val)
			}
			sum += pp.Val
		}
		sum += res.PODValue
		if !finite(res.SumValue) {
			t.Fatalf("seed-iter %d: non-finite SumValue", i)
		}
		if d := math.Abs(res.SumValue - sum); d > 0.01+1e-9*math.Abs(res.SumValue) {
			t.Fatalf("seed-iter %d: additivity broken: SumValue=%.6f rows+POD=%.6f (diff %.6f)",
				i, res.SumValue, sum, d)
		}
		if sv, ok := activeOracle.PV(sc.input); ok {
			if rel := math.Abs(res.SumValue-sv) / (1 + math.Abs(sv)); rel > 1e-6 {
				t.Fatalf("seed-iter %d: ORACLE MISMATCH engine=%.6f oracle=%.6f", i, res.SumValue, sv)
			}
		}
	}
}

// TestPropertyRateMonotone: with all payments positive and in the future,
// raising the discount rate cannot raise the present value.
func TestPropertyRateMonotone(t *testing.T) {
	rng := rand.New(rand.NewSource(0xBADF00D))
	const N = 2000
	for i := 0; i < N; i++ {
		sc := genScenario(rng)
		lo := Calculate(sc.input)
		if lo.Err != nil {
			continue
		}
		hiIn := sc.input
		hiIn.PresVal.R.Rate = sc.rate + 0.03
		hi := Calculate(hiIn)
		if hi.Err != nil {
			continue
		}
		// Allow a tiny tolerance for accumulation of past-dated rows
		// (none here: all payment years > as-of year), so strict.
		if hi.SumValue > lo.SumValue+1e-6 {
			t.Fatalf("seed-iter %d: PV rose with rate: %.6f (r=%.4f) -> %.6f (r=%.4f)",
				i, lo.SumValue, sc.rate, hi.SumValue, sc.rate+0.03)
		}
	}
}

// TestPropertyContingencyReducesValue: a single-life "Living" contingency
// can never increase a positive future payment's value versus treating
// it as certain.
func TestPropertyContingencyReducesValue(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5EED))
	asOf := dateOf(2026, time.January, 1)
	const N = 2000
	for i := 0; i < N; i++ {
		cfg := actuarialTestCfg(asOf, dateOf(1940+rng.Intn(30), time.January, 1))
		yr := 2028 + rng.Intn(25)
		amt := float64(1000 + rng.Intn(100000))
		base := func(act byte, withCfg bool) PVResult {
			in := PVInput{
				Settings: vrTestSettings(),
				PresVal: PresValLine{
					AsOfStatus: types.InOutInput, AsOf: asOf,
					R: RateEntry{Status: types.InOutInput, Rate: 0.06},
				},
				LumpSums: []LumpSumPayment{{
					DateStatus: types.InOutInput, Date: dateOf(yr, time.January, 1),
					AmtStatus: types.InOutInput, Amt: amt, Act: act,
				}},
			}
			if withCfg {
				in.Actuarial = cfg
			}
			return Calculate(in)
		}
		certain := base(actuarial.NotContingent, false)
		living := base(actuarial.Living, true)
		if certain.Err != nil || living.Err != nil {
			continue
		}
		if living.SumValue > certain.SumValue+1e-6 {
			t.Fatalf("seed-iter %d: contingent value %.4f exceeds certain %.4f", i, living.SumValue, certain.SumValue)
		}
		if living.SumValue < -1e-9 {
			t.Fatalf("seed-iter %d: negative contingent value %.4f for positive payment", i, living.SumValue)
		}
	}
}

// TestPropertyAmountRoundTrip: forward-value a random worksheet, blank
// one row's amount, solve for it from the Sum Value, and recover the
// original amount. Guards against rows whose present value is too small
// to invert stably.
func TestPropertyAmountRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0x1CE))
	const N = 3000
	checked := 0
	for i := 0; i < N; i++ {
		sc := genScenario(rng)
		fwd := Calculate(sc.input)
		if fwd.Err != nil || !finite(fwd.SumValue) {
			continue
		}
		// Blank exactly one lump amount (the first), require its forward
		// value be meaningfully non-zero so the solve is well-conditioned.
		if len(fwd.LumpSums) == 0 || math.Abs(fwd.LumpSums[0].Val) < 1 {
			continue
		}
		wantAmt := sc.input.LumpSums[0].Amt

		bwd := sc.input
		ls := make([]LumpSumPayment, len(sc.input.LumpSums))
		copy(ls, sc.input.LumpSums)
		ls[0].AmtStatus = types.StatusEmpty
		ls[0].Amt = 0
		bwd.LumpSums = ls
		bwd.PresVal.SumValueStatus = types.InOutInput
		bwd.PresVal.SumValue = fwd.SumValue

		res := Calculate(bwd)
		if res.Err != nil {
			continue // some draws are not single-unknown solvable; skip
		}
		got := res.LumpSums[0].Amt
		if rel := math.Abs(got-wantAmt) / (1 + math.Abs(wantAmt)); rel > 1e-6 {
			t.Fatalf("seed-iter %d: amount round-trip failed: got %.6f want %.6f (rel %.2e)",
				i, got, wantAmt, rel)
		}
		checked++
	}
	if checked < 200 {
		t.Fatalf("only %d round trips actually exercised; generator may be too narrow", checked)
	}
}
