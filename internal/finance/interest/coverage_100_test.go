package interest

import (
	"testing"

	"github.com/persense/persense-port/internal/types"
)

func TestPower_Branches(t *testing.T) {
	// x <= 0 returns 0.
	if v, err := Power(0, 2); err != nil || v != 0 {
		t.Errorf("Power(0,2) = %v, %v; want 0, nil", v, err)
	}
	if v, err := Power(-3, 2); err != nil || v != 0 {
		t.Errorf("Power(-3,2) = %v, %v; want 0, nil", v, err)
	}
	// Normal.
	if v, err := Power(2, 3); err != nil || v < 7.99 || v > 8.01 {
		t.Errorf("Power(2,3) = %v, %v; want ~8", v, err)
	}
	// Overflow via Exxp (n*ln(x) > 70).
	if _, err := Power(2, 200); err == nil {
		t.Error("Power(2,200) should overflow")
	}
}

func TestQuadraticFormula_Branches(t *testing.T) {
	// Real roots.
	if v, err := QuadraticFormula(1, -3, 2); err != nil || v < 0.99 || v > 1.01 {
		t.Errorf("QuadraticFormula(1,-3,2) = %v, %v; want root 1", v, err)
	}
	// Negative discriminant -> Sqrrt error.
	if _, err := QuadraticFormula(1, 0, 1); err == nil {
		t.Error("QuadraticFormula with no real roots should error")
	}
}

func TestYieldFromRate_ErrorBranch(t *testing.T) {
	if _, err := YieldFromRate(1000, 1, 365.25); err == nil {
		t.Error("YieldFromRate huge rate should overflow")
	}
	if _, err := YieldFromRate(0.06, 12, 365.25); err != nil {
		t.Errorf("YieldFromRate normal: %v", err)
	}
}

func TestRateFromYield_ErrorBranch(t *testing.T) {
	// 1 + yy/nn <= 0 -> Lnn error.
	if _, err := RateFromYield(-2, 1, 365.25); err == nil {
		t.Error("RateFromYield(-2) should error")
	}
	if _, err := RateFromYield(0.06, 12, 365.25); err != nil {
		t.Errorf("RateFromYield normal: %v", err)
	}
}

func TestReportedRate_Branches(t *testing.T) {
	// Passthrough (no Canadian/daily flag).
	if v, err := ReportedRate(0.06, 12, 12, 365.25); err != nil || v != 0.06 {
		t.Errorf("ReportedRate passthrough = %v, %v", v, err)
	}
	// Canadian/daily flag path (normal).
	settings := byte(12) | types.CompoundingCanadian
	if _, err := ReportedRate(0.06, 12, settings, 365.25); err != nil {
		t.Errorf("ReportedRate canadian: %v", err)
	}
	// Error path: RateFromYield fails (1 + apr/nn <= 0).
	if _, err := ReportedRate(-2, 1, byte(1)|types.CompoundingCanadian, 365.25); err == nil {
		t.Error("ReportedRate error path should error")
	}
}

func TestInterpretedRate_Branches(t *testing.T) {
	if _, err := InterpretedRate(0.06, 12, 12, 365.25); err != nil {
		t.Errorf("InterpretedRate normal: %v", err)
	}
	// Error path: RateFromYield fails.
	if _, err := InterpretedRate(-2, 1, 1, 365.25); err == nil {
		t.Error("InterpretedRate error path should error")
	}
}

func TestPerYrString_AllCasesAndCaps(t *testing.T) {
	for _, peryr := range []byte{1, 2, 3, 4, 6, 12, 24, 26, 52, 99} {
		for _, cap := range []byte{0, 1, 2} {
			s := PerYrString(peryr, cap)
			if s == "" {
				t.Errorf("PerYrString(%d,%d) empty", peryr, cap)
			}
		}
	}
}

func TestRealPerYr_AllBranches(t *testing.T) {
	if RealPerYr(types.CompoundingDaily, 365.25) != 365.25 {
		t.Error("daily")
	}
	if RealPerYr(52, 364) != 52 {
		t.Error("weekly")
	}
	if RealPerYr(26, 364) != 26 {
		t.Error("biweekly")
	}
	if RealPerYr(12|types.CompoundingCanadian, 365.25) != 12 {
		t.Error("canadian strip")
	}
}
