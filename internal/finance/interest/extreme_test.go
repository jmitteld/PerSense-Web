package interest

import (
	"math"
	"testing"
)

// --- Exxp edge cases ---

func TestExxpBoundary70(t *testing.T) {
	// Just under overflow boundary
	got, err := Exxp(69.9)
	if err != nil {
		t.Fatalf("Exxp(69.9) should not error: %v", err)
	}
	if got <= 0 || math.IsInf(got, 0) {
		t.Errorf("Exxp(69.9) = %g, want finite positive", got)
	}

	// Just over
	_, err = Exxp(70.1)
	if err == nil {
		t.Error("Exxp(70.1) should return overflow error")
	}
}

func TestExxpTaylorBoundary(t *testing.T) {
	// Right at the small boundary (1e-4)
	for _, x := range []float64{9.99e-5, 1.01e-4, -9.99e-5, -1.01e-4} {
		got, err := Exxp(x)
		if err != nil {
			t.Fatalf("Exxp(%g) error: %v", x, err)
		}
		want := math.Exp(x)
		if math.Abs(got-want)/want > 1e-8 {
			t.Errorf("Exxp(%g) = %g, math.Exp = %g", x, got, want)
		}
	}
}

// --- Lnn edge cases ---

func TestLnnTaylorBoundary(t *testing.T) {
	for _, x := range []float64{1.00009, 0.99991, 1.00011, 0.99989} {
		got, err := Lnn(x)
		if err != nil {
			t.Fatalf("Lnn(%g) error: %v", x, err)
		}
		want := math.Log(x)
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("Lnn(%g) = %g, want %g", x, got, want)
		}
	}
}

func TestLnnVerySmallPositive(t *testing.T) {
	got, err := Lnn(1e-300)
	if err != nil {
		t.Fatal(err)
	}
	if got >= 0 {
		t.Errorf("Lnn(1e-300) = %g, want large negative", got)
	}
}

// --- Sqrrt edge cases ---

func TestSqrrtNearZeroNegative(t *testing.T) {
	// Between -teeny and 0: should return 0 without error
	got, err := Sqrrt(-1e-11)
	if err != nil {
		t.Errorf("Sqrrt(-1e-11) should not error: %v", err)
	}
	if got != 0 {
		t.Errorf("Sqrrt(-1e-11) = %g, want 0", got)
	}

	// Just past -teeny: should error
	_, err = Sqrrt(-1e-9)
	if err == nil {
		t.Error("Sqrrt(-1e-9) should error")
	}
}

// --- Power edge cases ---

func TestPowerZeroExponent(t *testing.T) {
	got, _ := Power(999, 0)
	if math.Abs(got-1) > 1e-10 {
		t.Errorf("Power(999, 0) = %g, want 1", got)
	}
}

func TestPowerZeroBase(t *testing.T) {
	got, _ := Power(0, 5)
	if got != 0 {
		t.Errorf("Power(0, 5) = %g, want 0", got)
	}
}

func TestPowerVeryLargeExponent(t *testing.T) {
	// Should not panic; may return error or large value
	_, err := Power(2, 200)
	// 2^200 ≈ 1.6e60 — exp(200*ln(2)) ≈ exp(138.6) which is > 70, so overflow
	if err == nil {
		t.Log("Power(2, 200) did not error — value may be clamped")
	}
}

// --- Round2 edge cases ---

func TestRound2ExactHalves(t *testing.T) {
	// The original rounds 0.005 DOWN (halfpenny = 0.005 - teeny)
	if Round2(0.005) != 0.00 {
		t.Errorf("Round2(0.005) = %g, want 0.00", Round2(0.005))
	}
	if Round2(0.015) != 0.01 {
		t.Errorf("Round2(0.015) = %g, want 0.01", Round2(0.015))
	}
	// 0.006 rounds UP
	if Round2(0.006) != 0.01 {
		t.Errorf("Round2(0.006) = %g, want 0.01", Round2(0.006))
	}
}

func TestRound2LargeValues(t *testing.T) {
	got := Round2(999999999.994)
	if got != 999999999.99 {
		t.Errorf("Round2(999999999.994) = %g", got)
	}
	got = Round2(999999999.996)
	if got != 1000000000.00 {
		t.Errorf("Round2(999999999.996) = %g", got)
	}
}

func TestRound2NegativeValues(t *testing.T) {
	if Round2(-0.006) != -0.01 {
		t.Errorf("Round2(-0.006) = %g, want -0.01", Round2(-0.006))
	}
	if Round2(-123.456) != -123.46 {
		t.Errorf("Round2(-123.456) = %g, want -123.46", Round2(-123.456))
	}
}

// --- YieldFromRate / RateFromYield extreme values ---

func TestYieldRateZero(t *testing.T) {
	y, err := YieldFromRate(0, 12, 365.25)
	if err != nil || y != 0 {
		t.Errorf("YieldFromRate(0) = %g, err = %v", y, err)
	}
	r, err := RateFromYield(0, 12, 365.25)
	if err != nil || r != 0 {
		t.Errorf("RateFromYield(0) = %g, err = %v", r, err)
	}
}

func TestYieldRateHighRate(t *testing.T) {
	// 50% rate — should still work
	y, err := YieldFromRate(0.50, 12, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	r, err := RateFromYield(y, 12, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(r-0.50) > 1e-10 {
		t.Errorf("round trip at 50%%: got %g", r)
	}
}

func TestYieldRateNegativeRate(t *testing.T) {
	// Negative rates (rare but possible)
	y, err := YieldFromRate(-0.01, 12, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	r, err := RateFromYield(y, 12, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(r-(-0.01)) > 1e-10 {
		t.Errorf("round trip at -1%%: got %g", r)
	}
}

func TestRealPerYrDaily(t *testing.T) {
	// Daily with 360-day year
	got := RealPerYr(64, 360) // 64 = CompoundingDaily
	if got != 360 {
		t.Errorf("RealPerYr(daily, 360) = %g, want 360", got)
	}
	// Daily with 365.25-day year
	got = RealPerYr(64, 365.25)
	if got != 365.25 {
		t.Errorf("RealPerYr(daily, 365.25) = %g, want 365.25", got)
	}
}
