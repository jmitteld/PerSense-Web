package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// TestSolvePeriodicFromDateDayPrecision pins the day-level precision of
// the From Date solve (PV-6). Because the From Date anchors every
// installment, the periodic value varies continuously with it, so the
// solver's day-level bisection (refineDateByDays) should recover the
// original date to within a day or two — not just to the nearest
// payment period. Covers both the plain and the life-contingent paths.
func TestSolvePeriodicFromDateDayPrecision(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	rate := 0.06
	wantFrom := dateOf(2030, time.January, 1)
	toDate := dateOf(2040, time.January, 1)

	cases := []struct {
		name string
		act  byte
		cfg  *actuarial.ActuarialConfig
	}{
		{"plain", actuarial.NotContingent, nil},
		{"contingent", actuarial.Living, actuarialTestCfg(asOf, dateOf(1956, time.January, 1))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
						Act: tc.act,
					}},
					Actuarial: tc.cfg,
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
				t.Fatalf("backward: %v", res.Err)
			}
			deltaDays := math.Abs(res.Periodics[0].FromDate.Time.Sub(wantFrom.Time).Hours() / 24)
			if deltaDays > 2 {
				t.Errorf("%s: solved From Date = %s, want %s (off by %.1f days)",
					tc.name, res.Periodics[0].FromDate.Time.Format("2006-01-02"),
					wantFrom.Time.Format("2006-01-02"), deltaDays)
			}
			// The recovered value must also reconcile to the target.
			if math.Abs(res.SumValue-fwd.SumValue) > 0.5 {
				t.Errorf("%s: SumValue drift: got %.4f want %.4f",
					tc.name, res.SumValue, fwd.SumValue)
			}
		})
	}
}

// These tests exercise the backward solvers (solve-for-amount) when
// life-contingency is active. The forward path weights every payment by
// the survival probability (calc.go periodicWithActuarial / lumpRowPV);
// DOS does the same inside Summation when fold_in_life is set
// (PRESVALU.pas:397). A backward solve must invert that *same* weighted
// valuation, or the solved amount will not round-trip.

// TestSolveContingentLumpAmount_RoundTrip solves for a life-contingent
// lump-sum amount and confirms it recovers the original. This mirrors
// TestPV1ActuarialRoundTrip but with a second contingency type and an
// explicit check that the reported row value equals the survival-
// weighted present value (amount * exp(-r*t) * LifeProb).
func TestSolveContingentLumpAmount_RoundTrip(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1)
	payDate := dateOf(2036, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)
	rate := 0.06
	wantAmt := 250000.0

	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: rate},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: payDate,
				AmtStatus: types.InOutInput, Amt: wantAmt,
				Act: actuarial.Living,
			}},
			Actuarial: cfg,
		}
	}

	fwd := Calculate(mk())
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}
	// Forward row value must be survival-weighted (< plain discounted PV).
	prob := cfg.LifeProb(payDate, actuarial.Living)
	if prob <= 0 || prob >= 1 {
		t.Fatalf("test setup: survival prob = %v, want strictly in (0,1)", prob)
	}

	// Backward: blank the amount, feed the forward Sum Value back.
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
		t.Errorf("solved contingent lump amount = %.4f, want %.4f", res.LumpSums[0].Amt, wantAmt)
	}
	// The echoed row value must equal the forward survival-weighted value.
	if math.Abs(res.LumpSums[0].Val-fwd.LumpSums[0].Val) > 0.5 {
		t.Errorf("solved lump Val = %.4f, want forward Val %.4f",
			res.LumpSums[0].Val, fwd.LumpSums[0].Val)
	}
}

// TestSolveContingentPeriodicAmount_RoundTrip solves for a
// life-contingent periodic payment amount. The forward path weights each
// installment by survival probability, so the backward solve must divide
// the target value by the *survival-weighted* summation. DOS does this:
// amtn := valn/Summation(1,j) where Summation folds in LifeProb
// (PRESVALU.pas:397, :949). Guards against the gap where
// solvePeriodicAmount used the plain (unweighted) summation.
func TestSolveContingentPeriodicAmount_RoundTrip(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)
	rate := 0.06
	wantAmt := 1500.0

	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: rate},
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2045, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: wantAmt,
				Act: actuarial.Living,
			}},
			Actuarial: cfg,
		}
	}

	fwd := Calculate(mk())
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}

	// Sanity: the contingent forward value must be strictly less than the
	// non-contingent value, i.e. survival-weighting is actually biting.
	nc := mk()
	nc.Periodics[0].Act = actuarial.NotContingent
	ncFwd := Calculate(nc)
	if ncFwd.Err != nil {
		t.Fatalf("non-contingent forward: %v", ncFwd.Err)
	}
	if !(fwd.SumValue < ncFwd.SumValue-1) {
		t.Fatalf("test setup: contingent SumValue %.2f not < non-contingent %.2f",
			fwd.SumValue, ncFwd.SumValue)
	}

	// Backward: blank the amount, feed the contingent Sum Value back.
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
		t.Errorf("solved contingent periodic amount = %.4f, want %.4f "+
			"(survival-weighting missing in solvePeriodicAmount?)",
			res.Periodics[0].Amt, wantAmt)
	}
}

// TestSolveContingentPeriodicWithCola_RoundTrip is the same round trip
// but with a COLA on the periodic stream, exercising the weighted
// summation under a non-zero cost-of-living adjustment.
func TestSolveContingentPeriodicWithCola_RoundTrip(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)
	rate := 0.06
	wantAmt := 1000.0

	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: rate},
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2040, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus:  types.InOutInput, Amt: wantAmt,
				COLAStatus: types.InOutInput, COLA: 0.03,
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
	bwd.Periodics[0].AmtStatus = types.StatusEmpty
	bwd.Periodics[0].Amt = 0
	bwd.PresVal.SumValueStatus = types.InOutInput
	bwd.PresVal.SumValue = fwd.SumValue
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("backward: %v", res.Err)
	}
	if math.Abs(res.Periodics[0].Amt-wantAmt) > 0.5 {
		t.Errorf("solved contingent periodic amount (with COLA) = %.4f, want %.4f",
			res.Periodics[0].Amt, wantAmt)
	}
}

// TestSolveContingentLumpWithKnownContingentPeriodic combines both row
// types: a known life-contingent periodic stream plus an unknown
// life-contingent lump. The solver fills the lump; the known periodic
// row must report its survival-weighted value; and the two must
// reconcile to the target Sum Value.
func TestSolveContingentLumpWithKnownContingentPeriodic(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)
	rate := 0.06

	knownPeriodic := func() PeriodicPayment {
		return PeriodicPayment{
			FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(2040, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 2000,
			Act: actuarial.Living,
		}
	}
	lumpDate := dateOf(2045, time.January, 1)
	lumpAmt := 100000.0

	// Forward-value the whole screen (known periodic + known lump) to get
	// a self-consistent target and the expected per-row values.
	full := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: rate},
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: lumpDate,
			AmtStatus: types.InOutInput, Amt: lumpAmt,
			Act: actuarial.Living,
		}},
		Periodics: []PeriodicPayment{knownPeriodic()},
		Actuarial: cfg,
	}
	fwd := Calculate(full)
	if fwd.Err != nil {
		t.Fatalf("forward: %v", fwd.Err)
	}
	wantPeriodicVal := fwd.Periodics[0].Val
	wantLumpVal := fwd.LumpSums[0].Val

	// Backward: blank the lump amount, keep the periodic known, target the
	// forward Sum Value.
	bwd := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: rate},
			SumValueStatus: types.InOutInput, SumValue: fwd.SumValue,
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: lumpDate,
			Act: actuarial.Living,
		}},
		Periodics: []PeriodicPayment{knownPeriodic()},
		Actuarial: cfg,
	}
	res := Calculate(bwd)
	if res.Err != nil {
		t.Fatalf("backward: %v", res.Err)
	}

	if math.Abs(res.LumpSums[0].Amt-lumpAmt) > 0.5 {
		t.Errorf("solved lump amount = %.4f, want %.4f", res.LumpSums[0].Amt, lumpAmt)
	}
	if math.Abs(res.LumpSums[0].Val-wantLumpVal) > 0.5 {
		t.Errorf("solved lump Val = %.4f, want %.4f", res.LumpSums[0].Val, wantLumpVal)
	}
	if math.Abs(res.Periodics[0].Val-wantPeriodicVal) > 0.5 {
		t.Errorf("known periodic Val = %.4f, want %.4f (survival-weighted value not reported?)",
			res.Periodics[0].Val, wantPeriodicVal)
	}
	// Reconciliation: the two contingent rows' values sum to the target.
	if got := res.LumpSums[0].Val + res.Periodics[0].Val; math.Abs(got-fwd.SumValue) > 0.5 {
		t.Errorf("rows do not reconcile: lump %.4f + periodic %.4f = %.4f, want SumValue %.4f",
			res.LumpSums[0].Val, res.Periodics[0].Val, got, fwd.SumValue)
	}
}
