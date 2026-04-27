package interest

import (
	"math"
	"testing"
)

// --- Exxp tests ---

func TestExxp(t *testing.T) {
	tests := []struct {
		name    string
		x       float64
		wantErr bool
		check   func(float64) bool
	}{
		{"zero", 0, false, func(v float64) bool { return math.Abs(v-1) < 1e-12 }},
		{"one", 1, false, func(v float64) bool { return math.Abs(v-math.E) < 1e-10 }},
		{"negative", -1, false, func(v float64) bool { return math.Abs(v-1/math.E) < 1e-10 }},
		{"overflow", 71, true, nil},
		{"very negative", -71, false, func(v float64) bool { return v == 1e-32 }},
		// Taylor series region (|x| < 1e-4)
		{"tiny positive", 1e-5, false, func(v float64) bool {
			return math.Abs(v-math.Exp(1e-5)) < 1e-15
		}},
		{"tiny negative", -1e-5, false, func(v float64) bool {
			return math.Abs(v-math.Exp(-1e-5)) < 1e-15
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Exxp(tt.x)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.check(got) {
				t.Errorf("Exxp(%g) = %g", tt.x, got)
			}
		})
	}
}

func TestExxpTaylorAccuracy(t *testing.T) {
	// Verify Taylor series matches math.Exp in the small region
	for _, x := range []float64{1e-5, 5e-5, 9.9e-5, -1e-5, -5e-5, -9.9e-5} {
		got, _ := Exxp(x)
		want := math.Exp(x)
		if math.Abs(got-want)/want > 1e-12 {
			t.Errorf("Exxp(%g) = %g, math.Exp = %g, rel error = %g",
				x, got, want, math.Abs(got-want)/want)
		}
	}
}

// --- Lnn tests ---

func TestLnn(t *testing.T) {
	tests := []struct {
		name    string
		x       float64
		wantErr bool
		check   func(float64) bool
	}{
		{"one", 1, false, func(v float64) bool { return math.Abs(v) < 1e-12 }},
		{"e", math.E, false, func(v float64) bool { return math.Abs(v-1) < 1e-10 }},
		{"ten", 10, false, func(v float64) bool { return math.Abs(v-math.Log(10)) < 1e-10 }},
		{"zero", 0, true, nil},
		{"negative", -1, true, nil},
		// Taylor series region
		{"near one", 1.00005, false, func(v float64) bool {
			return math.Abs(v-math.Log(1.00005)) < 1e-15
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Lnn(tt.x)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.check(got) {
				t.Errorf("Lnn(%g) = %g", tt.x, got)
			}
		})
	}
}

// --- Sqrrt tests ---

func TestSqrrt(t *testing.T) {
	got, err := Sqrrt(4)
	if err != nil || got != 2 {
		t.Errorf("Sqrrt(4) = %g, err = %v", got, err)
	}

	got, err = Sqrrt(0)
	if err != nil || got != 0 {
		t.Errorf("Sqrrt(0) = %g, err = %v", got, err)
	}

	// Small negative: no error, returns 0
	got, err = Sqrrt(-1e-11)
	if err != nil || got != 0 {
		t.Errorf("Sqrrt(-1e-11) = %g, err = %v", got, err)
	}

	// Large negative: error
	_, err = Sqrrt(-1)
	if err == nil {
		t.Error("Sqrrt(-1) should return error")
	}
}

// --- Power tests ---

func TestPower(t *testing.T) {
	got, _ := Power(2, 10)
	if math.Abs(got-1024) > 0.01 {
		t.Errorf("Power(2,10) = %g, want 1024", got)
	}

	got, _ = Power(10, 0)
	if math.Abs(got-1) > 1e-10 {
		t.Errorf("Power(10,0) = %g, want 1", got)
	}

	got, _ = Power(2, 0.5)
	if math.Abs(got-math.Sqrt(2)) > 1e-10 {
		t.Errorf("Power(2,0.5) = %g, want sqrt(2)", got)
	}

	// Negative base returns 0
	got, _ = Power(-1, 2)
	if got != 0 {
		t.Errorf("Power(-1,2) = %g, want 0", got)
	}
}

// --- QuadraticFormula tests ---

func TestQuadraticFormula(t *testing.T) {
	// x² - 5x + 6 = 0 → roots 2, 3. Formula returns (-(-5) - sqrt(25-24)) / 2 = (5-1)/2 = 2
	got, err := QuadraticFormula(1, -5, 6)
	if err != nil || math.Abs(got-2) > 1e-10 {
		t.Errorf("QuadraticFormula(1,-5,6) = %g, err = %v, want 2", got, err)
	}

	// x² - 2x + 1 = 0 → double root at 1
	got, err = QuadraticFormula(1, -2, 1)
	if err != nil || math.Abs(got-1) > 1e-10 {
		t.Errorf("QuadraticFormula(1,-2,1) = %g, want 1", got)
	}
}

// --- Round2 tests ---

func TestRound2(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{1.234, 1.23},
		{1.235, 1.23},   // halfpenny = 0.005 - teeny, so 1.235 + 0.00499... = 1.23999 → truncates to 1.23
		{1.236, 1.24},   // 1.236 + 0.00499... = 1.24099 → truncates to 1.24
		{-1.234, -1.23},
		{-1.236, -1.24},
		{100.00, 100.00},
		{0.005, 0.00},   // Exact half penny rounds DOWN (not up)
		{0.006, 0.01},
	}
	for _, tt := range tests {
		got := Round2(tt.input)
		if got != tt.want {
			t.Errorf("Round2(%g) = %g, want %g", tt.input, got, tt.want)
		}
	}
}

// --- OK tests ---

func TestOK(t *testing.T) {
	if !OK(100) {
		t.Error("OK(100) should be true")
	}
	if !OK(0) {
		t.Error("OK(0) should be true")
	}
	if OK(-8888) {
		t.Error("OK(-8888) should be false (unk/error)")
	}
	if OK(-7777) {
		t.Error("OK(-7777) should be false (blank)")
	}
}

// --- Floor tests ---

func TestFloor(t *testing.T) {
	tests := []struct {
		input float64
		want  int64
	}{
		{2.7, 2},
		{2.0, 2},
		{-2.3, -3},
		{-2.0, -2},
		{0.0, 0},
		{0.5, 0},
		{-0.5, -1},
	}
	for _, tt := range tests {
		got := Floor(tt.input)
		if got != tt.want {
			t.Errorf("Floor(%g) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
