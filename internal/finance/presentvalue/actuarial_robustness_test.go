package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// twoLifeCfg builds a life-contingency config with two distinct lives:
// person 1 with the standard mock table, person 2 with heavier
// mortality, so two-life contingencies (Only1/Only2/Either/Both) have
// non-degenerate, distinguishable probabilities.
func twoLifeCfg(asOf, dob1, dob2 types.DateRec) *actuarial.ActuarialConfig {
	cfg := actuarialTestCfg(asOf, dob1)
	qx2 := make([]float64, 121)
	for i := range qx2 {
		qx2[i] = 0.002 + 0.00025*float64(i)*float64(i)/120.0 // heavier than table1
		if qx2[i] > 1 {
			qx2[i] = 1
		}
	}
	qx2[120] = 1
	cfg.Table2 = actuarial.NewLifeTableFromQx("mock2", qx2)
	cfg.DOB2 = dob2
	return cfg
}

// allContingencies is the full set of life-contingency codes the engine
// supports (excluding NotContingent, which is the no-op baseline).
var allContingencies = []struct {
	name string
	code byte
}{
	{"Living", actuarial.Living},
	{"Dead", actuarial.Dead},
	{"Only1Living", actuarial.Only1Living},
	{"Only2Living", actuarial.Only2Living},
	{"EitherLiving", actuarial.EitherLiving},
	{"BothLiving", actuarial.BothLiving},
}

// --- A. LifeProb pure-function invariants -------------------------------

// TestLifeProbInvariants checks the probability algebra across all
// contingency types at several future dates: bounds in [0,1], the
// identities that tie the two-life cases together, and the expected
// ordering. Catches sign/formula slips in LifeProb (contingency.go).
func TestLifeProbInvariants(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := twoLifeCfg(asOf, dateOf(1956, time.January, 1), dateOf(1961, time.January, 1))

	for _, yr := range []int{2027, 2035, 2050, 2070} {
		d := dateOf(yr, time.January, 1)
		living := cfg.LifeProb(d, actuarial.Living)
		dead := cfg.LifeProb(d, actuarial.Dead)
		only1 := cfg.LifeProb(d, actuarial.Only1Living)
		only2 := cfg.LifeProb(d, actuarial.Only2Living)
		either := cfg.LifeProb(d, actuarial.EitherLiving)
		both := cfg.LifeProb(d, actuarial.BothLiving)

		all := map[string]float64{
			"Living": living, "Dead": dead, "Only1": only1,
			"Only2": only2, "Either": either, "Both": both,
		}
		for name, p := range all {
			if p < -1e-12 || p > 1+1e-12 {
				t.Errorf("year %d: LifeProb(%s) = %v, want in [0,1]", yr, name, p)
			}
		}

		// NotContingent is always 1.
		if got := cfg.LifeProb(d, actuarial.NotContingent); math.Abs(got-1) > 1e-12 {
			t.Errorf("year %d: LifeProb(NotContingent) = %v, want 1", yr, got)
		}
		// Dead = 1 - Living (person 1).
		if math.Abs(dead-(1-living)) > 1e-9 {
			t.Errorf("year %d: Dead %v != 1-Living %v", yr, dead, 1-living)
		}
		// Living = Only1Living + BothLiving  (s1 = s1(1-s2) + s1 s2).
		if math.Abs(living-(only1+both)) > 1e-9 {
			t.Errorf("year %d: Living %v != Only1 %v + Both %v", yr, living, only1, both)
		}
		// EitherLiving = Only1 + Only2 + Both.
		if math.Abs(either-(only1+only2+both)) > 1e-9 {
			t.Errorf("year %d: Either %v != Only1+Only2+Both %v", yr, either, only1+only2+both)
		}
		// Ordering: Both <= Living <= Either, and Both <= Either.
		if !(both <= living+1e-9 && living <= either+1e-9) {
			t.Errorf("year %d: ordering violated Both=%v Living=%v Either=%v", yr, both, living, either)
		}
	}
}

// TestLifeProbMonotoneDecreasing confirms that survival-type
// probabilities fall as the payment date moves further out.
func TestLifeProbMonotoneDecreasing(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := twoLifeCfg(asOf, dateOf(1956, time.January, 1), dateOf(1961, time.January, 1))
	prev := map[byte]float64{}
	for _, yr := range []int{2027, 2030, 2040, 2055, 2075} {
		d := dateOf(yr, time.January, 1)
		for _, c := range []byte{actuarial.Living, actuarial.BothLiving, actuarial.EitherLiving} {
			p := cfg.LifeProb(d, c)
			if p0, ok := prev[c]; ok && p > p0+1e-9 {
				t.Errorf("contingency %d not monotone: year %d prob %v > previous %v", c, yr, p, p0)
			}
			prev[c] = p
		}
	}
}

// --- B. Forward valuation ordering --------------------------------------

// TestForwardValueOrderingByContingency verifies that, for a positive
// payment, present value falls as the contingency becomes more
// restrictive: NotContingent >= EitherLiving >= Living >= BothLiving.
func TestForwardValueOrderingByContingency(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := twoLifeCfg(asOf, dateOf(1956, time.January, 1), dateOf(1961, time.January, 1))

	val := func(act byte) float64 {
		in := PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: 0.06},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 100000,
				Act: act,
			}},
			Actuarial: cfg,
		}
		r := Calculate(in)
		if r.Err != nil {
			t.Fatalf("calc act=%d: %v", act, r.Err)
		}
		return r.SumValue
	}

	none := val(actuarial.NotContingent)
	either := val(actuarial.EitherLiving)
	living := val(actuarial.Living)
	both := val(actuarial.BothLiving)

	if !(none >= either-1e-6 && either >= living-1e-6 && living >= both-1e-6) {
		t.Errorf("value ordering violated: None=%.2f Either=%.2f Living=%.2f Both=%.2f",
			none, either, living, both)
	}
	// And contingency must actually reduce value (probabilities < 1 here).
	if !(none > both) {
		t.Errorf("expected None (%.2f) > Both (%.2f)", none, both)
	}
}

// --- C. Per-contingency amount round trips ------------------------------

// TestSolveLumpAmount_AllContingencies forward-values a lump under each
// contingency type, blanks the amount, solves, and expects recovery.
// Exercises solveLumpAmount's LifeProb division for every type,
// including the two-life cases.
func TestSolveLumpAmount_AllContingencies(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := twoLifeCfg(asOf, dateOf(1956, time.January, 1), dateOf(1961, time.January, 1))
	payDate := dateOf(2042, time.January, 1)
	const wantAmt = 100000.0

	for _, c := range allContingencies {
		t.Run(c.name, func(t *testing.T) {
			prob := cfg.LifeProb(payDate, c.code)
			if prob < 0.02 {
				t.Skipf("probability %.4f too small for a stable round trip", prob)
			}
			mk := func() PVInput {
				return PVInput{
					Settings: vrTestSettings(),
					PresVal: PresValLine{
						AsOfStatus: types.InOutInput, AsOf: asOf,
						R: RateEntry{Status: types.InOutInput, Rate: 0.06},
					},
					LumpSums: []LumpSumPayment{{
						DateStatus: types.InOutInput, Date: payDate,
						AmtStatus: types.InOutInput, Amt: wantAmt,
						Act: c.code,
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
			if math.Abs(res.LumpSums[0].Amt-wantAmt) > 0.5 {
				t.Errorf("%s: solved amount = %.4f, want %.4f", c.name, res.LumpSums[0].Amt, wantAmt)
			}
		})
	}
}

// TestSolvePeriodicAmount_AllContingencies is the periodic analogue:
// forward-value a contingent stream, blank the amount, solve, recover.
func TestSolvePeriodicAmount_AllContingencies(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := twoLifeCfg(asOf, dateOf(1956, time.January, 1), dateOf(1961, time.January, 1))
	const wantAmt = 2000.0

	for _, c := range allContingencies {
		t.Run(c.name, func(t *testing.T) {
			// Mid-stream probability sanity (use the mid date).
			if p := cfg.LifeProb(dateOf(2035, time.January, 1), c.code); p < 0.02 {
				t.Skipf("probability %.4f too small for a stable round trip", p)
			}
			mk := func() PVInput {
				return PVInput{
					Settings: vrTestSettings(),
					PresVal: PresValLine{
						AsOfStatus: types.InOutInput, AsOf: asOf,
						R: RateEntry{Status: types.InOutInput, Rate: 0.06},
					},
					Periodics: []PeriodicPayment{{
						FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
						ToDateStatus: types.InOutInput, ToDate: dateOf(2040, time.January, 1),
						PerYrStatus: types.InOutInput, PerYr: 12,
						AmtStatus: types.InOutInput, Amt: wantAmt,
						Act: c.code,
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
			if math.Abs(res.Periodics[0].Amt-wantAmt) > 0.5 {
				t.Errorf("%s: solved amount = %.4f, want %.4f", c.name, res.Periodics[0].Amt, wantAmt)
			}
		})
	}
}

// --- D. Rate / as-of solves under contingency ---------------------------

// TestSolveRate_ContingentRoundTrip forward-values a contingent screen at
// a known rate, then blanks the rate and solves (PV-8), expecting the
// original rate back. evaluatePVAt must value contingent rows with
// survival weighting for this to converge.
func TestSolveRate_ContingentRoundTrip(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := twoLifeCfg(asOf, dateOf(1956, time.January, 1), dateOf(1961, time.January, 1))
	const wantRate = 0.055

	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: wantRate},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 50000, Act: actuarial.Living,
			}},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2038, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 1000, Act: actuarial.BothLiving,
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
}

// TestSolveAsOf_ContingentRoundTrip blanks the as-of date and solves
// (PV-9) on a contingent screen, expecting the original as-of back.
func TestSolveAsOf_ContingentRoundTrip(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	rate := 0.06

	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: rate},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 80000, Act: actuarial.Living,
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
	gotYrs := math.Abs(yearsBetween(res.AsOf, asOf))
	if gotYrs > 0.05 {
		t.Errorf("solved as-of = %v, want %v (off by %.3f yr)", res.AsOf.Time, asOf.Time, gotYrs)
	}
}

func yearsBetween(a, b types.DateRec) float64 {
	return a.Time.Sub(b.Time).Hours() / (24 * 365.25)
}

// --- E. POD interactions ------------------------------------------------

// TestSolveRateWithPOD_RoundTrip confirms a Payment-on-Death folds into
// the rate solve: forward-value with POD at a known rate, blank the
// rate, recover it.
func TestSolveRateWithPOD_RoundTrip(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	cfg.POD = 50000
	const wantRate = 0.06

	mk := func() PVInput {
		a := *cfg
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: wantRate},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 50000, Act: actuarial.Living,
			}},
			Actuarial: &a,
		}
	}
	fwd := Calculate(mk())
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}
	if fwd.PODValue <= 0 {
		t.Fatalf("expected positive POD value, got %.4f", fwd.PODValue)
	}
	bwd := mk()
	bwd.PresVal.R = RateEntry{Status: types.StatusEmpty}
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("backward (rate w/ POD): %v", res.Err)
	}
	if math.Abs(res.Rate-wantRate) > 1e-4 {
		t.Errorf("solved rate w/ POD = %.6f, want %.6f", res.Rate, wantRate)
	}
}

// TestPODValueMonotoneInAmount checks the POD present value scales
// linearly and positively with the POD face amount.
func TestPODValueMonotoneInAmount(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1)
	rate := 0.06

	pv := func(pod float64) float64 {
		c := actuarialTestCfg(asOf, dob)
		c.POD = pod
		return c.PODValue(asOf, rate)
	}
	v1 := pv(10000)
	v2 := pv(20000)
	if v1 <= 0 {
		t.Fatalf("POD value for 10000 = %.4f, want > 0", v1)
	}
	if math.Abs(v2-2*v1) > 0.02*v2 {
		t.Errorf("POD value not linear: pv(20000)=%.2f, 2*pv(10000)=%.2f", v2, 2*v1)
	}
}

// --- F. Boundary / error paths ------------------------------------------

// TestContingentPeriodicBeyondHorizon ensures a contingent stream that
// runs entirely past the life table's horizon does not silently return
// a bogus zero-probability amount when solved for.
func TestContingentPeriodicBeyondHorizon(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	// Person already 95 at as-of (table maxes near 120, qx high), stream
	// far out so survival probability is effectively zero.
	cfg := actuarialTestCfg(asOf, dateOf(1931, time.January, 1))

	in := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			SumValueStatus: types.InOutInput, SumValue: 100000,
		},
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(2050, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(2055, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			// amount blank -> solve for it
			Act: actuarial.Living,
		}},
		Actuarial: cfg,
	}
	res := Calculate(in)
	// Either a clear error, or a finite non-absurd amount — never NaN/Inf.
	if res.Err == nil {
		a := res.Periodics[0].Amt
		if math.IsNaN(a) || math.IsInf(a, 0) {
			t.Errorf("got non-finite solved amount %v with no error", a)
		}
	}
}

// TestMixedContingentNonContingentForward checks that a screen mixing a
// contingent and a plain row sums their (weighted and unweighted)
// present values correctly.
func TestMixedContingentNonContingentForward(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	rate := 0.06

	in := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: rate},
		},
		LumpSums: []LumpSumPayment{
			{DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 100000, Act: actuarial.Living},
			{DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 100000, Act: actuarial.NotContingent},
		},
		Actuarial: cfg,
	}
	res := Calculate(in)
	if res.Err != nil {
		t.Fatalf("calc: %v", res.Err)
	}
	// The contingent row (same date/amount) must be worth strictly less
	// than the plain row, and the total equals their sum.
	cVal := res.LumpSums[0].Val
	pVal := res.LumpSums[1].Val
	if !(cVal < pVal) {
		t.Errorf("contingent lump Val %.4f not < plain lump Val %.4f", cVal, pVal)
	}
	if math.Abs(res.SumValue-(cVal+pVal)) > 1e-6 {
		t.Errorf("SumValue %.4f != row sum %.4f", res.SumValue, cVal+pVal)
	}
}

// --- G. Contingent periodic DATE solves ---------------------------------

// TestSolveContingentPeriodicToDate_RoundTrip solves for the To Date of
// a life-contingent periodic stream (PV-5) and expects recovery. The
// solver must value the stream with survival weighting (like the forward
// path); the plain unweighted summation lands on the wrong date.
func TestSolveContingentPeriodicToDate_RoundTrip(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	rate := 0.06
	fromDate := dateOf(2030, time.January, 1)
	wantTo := dateOf(2040, time.January, 1)

	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: rate},
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: fromDate,
				ToDateStatus: types.InOutInput, ToDate: wantTo,
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 2000,
				Act: actuarial.Living,
			}},
			Actuarial: cfg,
		}
	}
	fwd := Calculate(mk())
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}
	bwd := mk()
	bwd.Periodics[0].ToDateStatus = types.StatusEmpty
	bwd.Periodics[0].ToDate = types.DateRec{}
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("backward (toDate): %v", res.Err)
	}
	off := math.Abs(yearsBetween(res.Periodics[0].ToDate, wantTo))
	if off > 0.05 { // To Date is quantized to installment boundaries
		t.Errorf("solved To Date = %v, want %v (off by %.3f yr)",
			res.Periodics[0].ToDate.Time, wantTo.Time, off)
	}
}

// TestSolveContingentPeriodicFromDate_RoundTrip is the PV-6 analogue,
// solving for the From Date of a contingent stream.
func TestSolveContingentPeriodicFromDate_RoundTrip(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	rate := 0.06
	wantFrom := dateOf(2030, time.January, 1)
	toDate := dateOf(2040, time.January, 1)

	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: rate},
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: wantFrom,
				ToDateStatus: types.InOutInput, ToDate: toDate,
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 2000,
				Act: actuarial.Living,
			}},
			Actuarial: cfg,
		}
	}
	fwd := Calculate(mk())
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}
	bwd := mk()
	bwd.Periodics[0].FromDateStatus = types.StatusEmpty
	bwd.Periodics[0].FromDate = types.DateRec{}
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("backward (fromDate): %v", res.Err)
	}
	off := math.Abs(yearsBetween(res.Periodics[0].FromDate, wantFrom))
	if off > 0.02 { // ~1 week: day-level bisection should land near-exact
		t.Errorf("solved From Date = %v, want %v (off by %.3f yr; day-level refinement?)",
			res.Periodics[0].FromDate.Time, wantFrom.Time, off)
	}
}
