package mortgage

import (
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Tests for the unusually-high-rate soft warning ported from
// legacy/src/dos_source/MortgageScreenUnit.pas:222 (DA_UnusuallyHighRate,
// threshold 0.19835162342 = 20% nominal in true-rate form).

func mtgWith(trueRate float64, rateStatus int8) MtgLine {
	return MtgLine{
		PriceStatus: types.InOutInput, Price: 200000,
		PctStatus:   types.InOutInput, Pct: 0.20,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus:  rateStatus, Rate: trueRate,
		TaxStatus:   types.InOutInput, Tax: 0,
		BalloonStat: types.BalloonBlank,
	}
}

func hasHighRateWarning(ws []string) bool {
	for _, w := range ws {
		if strings.Contains(w, "unusually high") {
			return true
		}
	}
	return false
}

// A user-entered rate above 20% nominal triggers the warning, and the
// calculation still proceeds (soft warning, not a hard error).
func TestHighRateWarning_Fires(t *testing.T) {
	res := Calc(mtgWith(LoanRateToTrueRate(0.25), types.InOutInput)) // 25% nominal
	if res.Err != nil {
		t.Fatalf("Calc errored, expected a warning not an error: %v", res.Err)
	}
	if !hasHighRateWarning(res.Warnings) {
		t.Errorf("expected an 'unusually high' rate warning, got warnings: %v", res.Warnings)
	}
}

// A normal rate produces no such warning.
func TestHighRateWarning_QuietAtNormalRate(t *testing.T) {
	res := Calc(mtgWith(LoanRateToTrueRate(0.06), types.InOutInput)) // 6% nominal
	if hasHighRateWarning(res.Warnings) {
		t.Errorf("did not expect a high-rate warning at 6%%, got: %v", res.Warnings)
	}
}

// The warning is gated on a *user-entered* rate. A high rate that the
// engine computed (status not InOutInput) must not trigger it — DOS
// only warns on cell entry.
func TestHighRateWarning_NotOnSolvedRate(t *testing.T) {
	res := Calc(mtgWith(LoanRateToTrueRate(0.25), types.InOutOutput))
	if hasHighRateWarning(res.Warnings) {
		t.Errorf("high-rate warning should not fire on a non-user-entered rate, got: %v", res.Warnings)
	}
}
