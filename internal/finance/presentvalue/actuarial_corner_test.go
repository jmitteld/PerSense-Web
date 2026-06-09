package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// Corner cases for the life-contingency paths. Where an exact figure is
// awkward to hand-derive these use invariants (bounds, ordering, round
// trips) so they stay robust against benign formula refactors while
// still catching real regressions.

// TestActuarialPaymentAtAsOf: a life-contingent lump dated exactly on
// the as-of date has no discounting and a survival probability of 1
// (the person is alive "now" by construction), so its value is the
// face amount.
func TestActuarialPaymentAtAsOf(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	const amt = 100000.0

	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: 0.06},
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: asOf,
			AmtStatus: types.InOutInput, Amt: amt,
			Act: actuarial.Living,
		}},
		Actuarial: cfg,
	})
	if res.Err != nil {
		t.Fatalf("calc: %v", res.Err)
	}
	if math.Abs(res.LumpSums[0].Val-amt) > 0.01 {
		t.Errorf("value at as-of = %.4f, want face amount %.4f", res.LumpSums[0].Val, amt)
	}
}

// TestActuarialPastPayment: a life-contingent payment dated before the
// as-of date is treated as certain (already-survived → prob 1) and is
// accumulated forward, so its present value exceeds the face amount.
func TestActuarialPastPayment(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	const amt = 100000.0

	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: 0.06},
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2020, time.January, 1),
			AmtStatus: types.InOutInput, Amt: amt,
			Act: actuarial.Living,
		}},
		Actuarial: cfg,
	})
	if res.Err != nil {
		t.Fatalf("calc: %v", res.Err)
	}
	v := res.LumpSums[0].Val
	if !(v > amt) || math.IsInf(v, 0) || math.IsNaN(v) {
		t.Errorf("past contingent payment value = %.4f, want a finite value > %.4f (accumulated)", v, amt)
	}
}

// TestActuarialBeyondHorizonForwardZero: a contingent payment dated when
// the person is past the life table's horizon has survival probability
// 0, so its forward value is 0 (not NaN/Inf), and the screen total is
// just the POD (zero here).
func TestActuarialBeyondHorizonForwardZero(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	// Person born 1900 → ~126 at as-of, well past the 121-row table.
	cfg := actuarialTestCfg(asOf, dateOf(1900, time.January, 1))

	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: 0.06},
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2030, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 100000,
			Act: actuarial.Living,
		}},
		Actuarial: cfg,
	})
	if res.Err != nil {
		t.Fatalf("calc: %v", res.Err)
	}
	if math.Abs(res.LumpSums[0].Val) > 1e-6 {
		t.Errorf("beyond-horizon contingent value = %.6f, want 0", res.LumpSums[0].Val)
	}
}

// TestActuarialBeyondHorizonSolveErrors: solving for the amount of a
// contingent payment whose survival probability is 0 must error
// cleanly (DOS "beyond life span"), not divide by zero.
func TestActuarialBeyondHorizonSolveErrors(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1900, time.January, 1))

	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			SumValueStatus: types.InOutInput, SumValue: 50000,
		},
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2030, time.January, 1),
			// amount blank → solve
			Act: actuarial.Living,
		}},
		Actuarial: cfg,
	})
	if res.Err == nil {
		t.Errorf("expected a 'beyond life span' error, got amount %.4f", res.LumpSums[0].Amt)
	}
}

// TestActuarialColaEqualsRateContingent round-trips a contingent
// periodic amount when COLA == rate (the special branch where the
// discount and escalation cancel).
func TestActuarialColaEqualsRateContingent(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	rate := 0.05
	const wantAmt = 1000.0

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
				COLAStatus: types.InOutInput, COLA: rate, // cola == rate
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
		t.Errorf("cola==rate contingent solved amount = %.4f, want %.4f", res.Periodics[0].Amt, wantAmt)
	}
}

// TestActuarialSingleInstallmentContingent round-trips a contingent
// periodic stream that contains essentially one payment (From and To a
// single period apart).
func TestActuarialSingleInstallmentContingent(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	const wantAmt = 5000.0

	mk := func() PVInput {
		return PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: 0.06},
			},
			Periodics: []PeriodicPayment{{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2030, time.February, 1),
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
	if fwd.SumValue <= 0 {
		t.Fatalf("single-installment forward value = %.4f, want > 0", fwd.SumValue)
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
		t.Errorf("single-installment solved amount = %.4f, want %.4f", res.Periodics[0].Amt, wantAmt)
	}
}

// TestActuarialNegativeContingentPayment: a negative life-contingent
// payment (e.g., a contingent liability) values to a negative present
// value of the same survival-weighted magnitude as its positive twin.
func TestActuarialNegativeContingentPayment(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1))
	pay := func(amt float64) float64 {
		r := Calculate(PVInput{
			Settings: vrTestSettings(),
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: asOf,
				R: RateEntry{Status: types.InOutInput, Rate: 0.06},
			},
			LumpSums: []LumpSumPayment{{
				DateStatus: types.InOutInput, Date: dateOf(2040, time.January, 1),
				AmtStatus: types.InOutInput, Amt: amt,
				Act: actuarial.Living,
			}},
			Actuarial: cfg,
		})
		if r.Err != nil {
			t.Fatalf("calc(%.0f): %v", amt, r.Err)
		}
		return r.SumValue
	}
	pos := pay(100000)
	neg := pay(-100000)
	if math.Abs(pos+neg) > 1e-6 {
		t.Errorf("negative contingent payment not symmetric: +%.4f vs %.4f", pos, neg)
	}
	if !(neg < 0) {
		t.Errorf("negative contingent payment value = %.4f, want < 0", neg)
	}
}

// TestTwoLifeContingencyWithoutSecondTable documents the RAW LifeProb
// behavior when a two-life contingency is evaluated with only the first
// life table: person 2 defaults to certain survival (s2 = 1), so the
// two-life cases collapse to single-life equivalents. This is why the
// Calculate-level guard (checkSecondLifeProvided) now rejects such a
// setup before it can produce silently-wrong numbers — see
// TestTwoLifeGuardRejectsMissingSecondTable. This test pins the
// underlying probability identities that motivate the guard.
func TestTwoLifeContingencyWithoutSecondTable(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1956, time.January, 1)) // Table1 only
	d := dateOf(2040, time.January, 1)

	living := cfg.LifeProb(d, actuarial.Living)
	dead := cfg.LifeProb(d, actuarial.Dead)

	// With s2 == 1: Both = s1, Only2 = (1-s1), Either = 1, Only1 = 0.
	if got := cfg.LifeProb(d, actuarial.BothLiving); math.Abs(got-living) > 1e-12 {
		t.Errorf("BothLiving without Table2 = %v, want Living %v (s2 defaults to 1)", got, living)
	}
	if got := cfg.LifeProb(d, actuarial.Only2Living); math.Abs(got-dead) > 1e-12 {
		t.Errorf("Only2Living without Table2 = %v, want Dead %v", got, dead)
	}
	if got := cfg.LifeProb(d, actuarial.EitherLiving); math.Abs(got-1) > 1e-12 {
		t.Errorf("EitherLiving without Table2 = %v, want 1", got)
	}
	if got := cfg.LifeProb(d, actuarial.Only1Living); math.Abs(got) > 1e-12 {
		t.Errorf("Only1Living without Table2 = %v, want 0", got)
	}
}
