package fileio

import (
	"math"
	"testing"
)

// --- Real48 extreme values ---

func TestReal48VerySmall(t *testing.T) {
	for _, v := range []float64{1e-30, 1e-20, 1e-10, 1e-38} {
		b := Float64ToReal48(v)
		got := Real48ToFloat64(b)
		if got == 0 && v != 0 {
			// Underflow to zero is acceptable for very small values
			continue
		}
		relErr := math.Abs(got-v) / v
		if relErr > 1e-8 {
			t.Errorf("Real48 round trip %g: got %g, relErr = %g", v, got, relErr)
		}
	}
}

func TestReal48VeryLarge(t *testing.T) {
	for _, v := range []float64{1e30, 1e35, 1e38} {
		b := Float64ToReal48(v)
		got := Real48ToFloat64(b)
		if got == 0 {
			// Overflow check: if the exponent can't fit, it returns 0
			continue
		}
		relErr := math.Abs(got-v) / v
		if relErr > 1e-8 {
			t.Errorf("Real48 round trip %g: got %g, relErr = %g", v, got, relErr)
		}
	}
}

func TestReal48NegativeValues(t *testing.T) {
	for _, v := range []float64{-1, -0.5, -100000, -0.06, -1e-10} {
		b := Float64ToReal48(v)
		got := Real48ToFloat64(b)
		relErr := math.Abs(got-v) / math.Abs(v)
		if relErr > 1e-8 {
			t.Errorf("Real48 negative round trip %g: got %g", v, got)
		}
		// Sign bit should be set
		if b[5]&0x80 == 0 {
			t.Errorf("Real48(%g) sign bit not set", v)
		}
	}
}

func TestReal48Precision(t *testing.T) {
	// Real48 has 39-bit mantissa ≈ 11.7 decimal digits
	// Test that we get at least 10 digits of precision
	v := 123456789.12
	b := Float64ToReal48(v)
	got := Real48ToFloat64(b)
	if math.Abs(got-v) > 0.01 {
		t.Errorf("Real48 precision: %g -> %g, diff = %g", v, got, math.Abs(got-v))
	}
}

func TestReal48PowersOfTwo(t *testing.T) {
	// Powers of 2 should be exact
	for exp := -10; exp <= 10; exp++ {
		v := math.Pow(2, float64(exp))
		b := Float64ToReal48(v)
		got := Real48ToFloat64(b)
		if got != v {
			t.Errorf("Real48(2^%d) = %g, want exact %g", exp, got, v)
		}
	}
}

func TestReal48TypicalFinancialValues(t *testing.T) {
	// These are the exact kind of values stored in legacy files
	values := []float64{
		200000.00,  // house price
		160000.00,  // financed amount
		959.28,     // monthly payment
		0.06,       // 6% rate
		0.065,      // 6.5% rate
		0.02,       // 2 points
		0.20,       // 20% down
		500000.00,  // half million
		99.99,      // small amount
		12345.67,   // random
		0.001,      // tenth of a percent
		1234567.89, // large amount
	}
	for _, v := range values {
		b := Float64ToReal48(v)
		got := Real48ToFloat64(b)
		// Financial values need at least 2 decimal places of accuracy
		if math.Abs(got-v) > 0.005 {
			t.Errorf("financial value %g round trip: got %g, diff = %g", v, got, math.Abs(got-v))
		}
	}
}

// --- Reader edge cases ---

func TestReadInt8Boundary(t *testing.T) {
	data := []byte{0x7F, 0x80, 0x00, 0x01, 0xFE, 0xFF}
	pos := 0
	tests := []int8{127, -128, 0, 1, -2, -1}
	for i, want := range tests {
		got := readInt8(data, &pos)
		if got != want {
			t.Errorf("byte %d: readInt8(0x%02X) = %d, want %d", i, data[i], got, want)
		}
	}
}

func TestReadBeyondEnd(t *testing.T) {
	data := []byte{0x42}
	pos := 0
	_ = readInt8(data, &pos) // consume the one byte
	got := readInt8(data, &pos) // read past end
	if got != 0 {
		t.Errorf("reading past end should return 0, got %d", got)
	}
}

func TestReadReal48BeyondEnd(t *testing.T) {
	data := []byte{1, 2, 3} // only 3 bytes, need 6
	pos := 0
	got := readReal48(data, &pos)
	if got != 0 {
		t.Errorf("partial Real48 should return 0, got %g", got)
	}
}

func TestReadDateRecBeyondEnd(t *testing.T) {
	data := []byte{15, 3} // only 2 bytes, need 3
	pos := 0
	d := readDateRec(data, &pos)
	if !d.IsUnknown() {
		t.Error("partial date should be unknown")
	}
}
