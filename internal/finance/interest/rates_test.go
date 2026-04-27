package interest

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// --- CalcContext / SetYrDays tests ---

func TestNewCalcContext(t *testing.T) {
	// Basis365 → yrdays = 365.25
	ctx := NewCalcContext(types.Basis365, 12)
	if ctx.YrDays != 365.25 {
		t.Errorf("Basis365 yrdays = %g, want 365.25", ctx.YrDays)
	}
	if math.Abs(ctx.YrInv-1.0/365.25) > 1e-15 {
		t.Errorf("Basis365 yrinv = %g", ctx.YrInv)
	}

	// Basis360 → yrdays = 360
	ctx = NewCalcContext(types.Basis360, 12)
	if ctx.YrDays != 360 {
		t.Errorf("Basis360 yrdays = %g, want 360", ctx.YrDays)
	}

	// Basis365_360 → yrdays = 360 (per the code: else yrdays:=360)
	ctx = NewCalcContext(types.Basis365360, 12)
	if ctx.YrDays != 360 {
		t.Errorf("Basis365_360 yrdays = %g, want 360", ctx.YrDays)
	}
}

// --- RealPerYr tests ---

func TestRealPerYr(t *testing.T) {
	const yrdays = 365.25

	tests := []struct {
		n    byte
		want float64
	}{
		{1, 1},
		{2, 2},
		{4, 4},
		{6, 6},
		{12, 12},
		{24, 24},
		{26, yrdays / 14},
		{52, yrdays / 7},
		{types.CompoundingDaily, yrdays},
		// Canadian monthly: 128 | 12 = 140. Strip canadian: 140 & ^128 = 12
		{types.CompoundingCanadian | 12, 12},
	}
	for _, tt := range tests {
		got := RealPerYr(tt.n, yrdays)
		if math.Abs(got-tt.want) > 1e-10 {
			t.Errorf("RealPerYr(%d) = %g, want %g", tt.n, got, tt.want)
		}
	}
}

// --- YieldFromRate / RateFromYield round-trip tests ---

func TestYieldRateRoundTrip(t *testing.T) {
	const yrdays = 365.25

	// For any rate and compounding frequency, converting rate → yield → rate
	// should produce the original value.
	rates := []float64{0.01, 0.05, 0.10, 0.20, 0.001}
	freqs := []byte{1, 2, 4, 12, 26, 52}

	for _, rate := range rates {
		for _, n := range freqs {
			yield, err := YieldFromRate(rate, n, yrdays)
			if err != nil {
				t.Fatalf("YieldFromRate(%g, %d) error: %v", rate, n, err)
			}
			got, err := RateFromYield(yield, n, yrdays)
			if err != nil {
				t.Fatalf("RateFromYield(%g, %d) error: %v", yield, n, err)
			}
			if math.Abs(got-rate) > 1e-12 {
				t.Errorf("round trip(%g, peryr=%d): got %g, want %g", rate, n, got, rate)
			}
		}
	}
}

func TestYieldFromRateZero(t *testing.T) {
	got, err := YieldFromRate(0, 12, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got) > 1e-15 {
		t.Errorf("YieldFromRate(0, 12) = %g, want 0", got)
	}
}

func TestYieldFromRateKnown(t *testing.T) {
	// A 6% continuous rate with monthly compounding:
	// yield = 12 * (exp(0.06/12) - 1) = 12 * (exp(0.005) - 1)
	got, err := YieldFromRate(0.06, 12, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	expected := 12 * (math.Exp(0.06/12) - 1)
	if math.Abs(got-expected) > 1e-12 {
		t.Errorf("YieldFromRate(0.06, 12) = %g, want %g", got, expected)
	}
}

// --- ReportedRate / InterpretedRate tests ---

func TestReportedRatePassthrough(t *testing.T) {
	// Non-Canadian, non-daily: rate passes through unchanged
	got, err := ReportedRate(0.06, 12, 12, 365.25)
	if err != nil || got != 0.06 {
		t.Errorf("ReportedRate passthrough = %g, err = %v, want 0.06", got, err)
	}
}

func TestReportedRateCanadian(t *testing.T) {
	// Canadian compounding: should convert
	canadianPerYr := types.CompoundingCanadian | 2 // Canadian semi-annual
	got, err := ReportedRate(0.06, 12, canadianPerYr, 365.25)
	if err != nil {
		t.Fatal(err)
	}
	// Should differ from input since it's converting between frequencies
	if got == 0.06 {
		t.Error("Canadian ReportedRate should differ from input")
	}
}

func TestReportedInterpretedRoundTrip(t *testing.T) {
	// ReportedRate and InterpretedRate should be inverses
	const yrdays = 365.25
	canadianPerYr := types.CompoundingCanadian | 2
	var loanPerYr byte = 12

	apr := 0.06
	reported, err := ReportedRate(apr, loanPerYr, canadianPerYr, yrdays)
	if err != nil {
		t.Fatal(err)
	}
	backToAPR, err := InterpretedRate(reported, loanPerYr, canadianPerYr, yrdays)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(backToAPR-apr) > 1e-10 {
		t.Errorf("round trip: started %g, got %g", apr, backToAPR)
	}
}

// --- PerYrString tests ---

func TestPerYrString(t *testing.T) {
	tests := []struct {
		peryr byte
		cap   byte
		want  string
	}{
		{1, 0, "yearly"},
		{2, 0, "semi-annually"},
		{4, 0, "quarterly"},
		{6, 0, "bi-monthly"},
		{12, 0, "monthly"},
		{24, 0, "twice-monthly"},
		{26, 0, "biweekly"},
		{52, 0, "weekly"},
		{12, 1, "Monthly"},
		{12, 2, "MONTHLY"},
		{3, 0, "thrice-annually"},
	}
	for _, tt := range tests {
		got := PerYrString(tt.peryr, tt.cap)
		if got != tt.want {
			t.Errorf("PerYrString(%d, %d) = %q, want %q", tt.peryr, tt.cap, got, tt.want)
		}
	}
}
