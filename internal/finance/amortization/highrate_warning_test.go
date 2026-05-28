package amortization

import (
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Tests for the unusually-high-rate soft warning on the amortization
// screen. DOS shows this only on the mortgage screen; we extend it here
// using the equivalent 20% nominal threshold (LoanRate is a nominal
// fraction, so 0.20 = 20%).

func hasHighRate(ws []string) bool {
	for _, w := range ws {
		if strings.Contains(w, "unusually high") {
			return true
		}
	}
	return false
}

func TestAmortHighRateWarning_Fires(t *testing.T) {
	in := LoanInput{Loan: mkFancyLoan(200_000, 0.25, 120, 0), Settings: defaultSettings()}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize errored, expected a warning not an error: %v", r.Err)
	}
	if !hasHighRate(r.Warnings) {
		t.Errorf("expected an 'unusually high' rate warning at 25%%, got: %v", r.Warnings)
	}
}

func TestAmortHighRateWarning_QuietAtNormalRate(t *testing.T) {
	in := LoanInput{Loan: mkFancyLoan(200_000, 0.06, 120, 0), Settings: defaultSettings()}
	r := Amortize(in)
	if hasHighRate(r.Warnings) {
		t.Errorf("did not expect a high-rate warning at 6%%, got: %v", r.Warnings)
	}
}

// Gated on a user-entered rate: a high LoanRate whose status is not
// InOutInput (e.g. a solved/derived rate) must not warn.
func TestAmortHighRateWarning_NotOnSolvedRate(t *testing.T) {
	loan := mkFancyLoan(200_000, 0.25, 120, 0)
	loan.LoanRateStatus = types.InOutOutput
	r := Amortize(LoanInput{Loan: loan, Settings: defaultSettings()})
	if hasHighRate(r.Warnings) {
		t.Errorf("high-rate warning should not fire on a non-user-entered rate, got: %v", r.Warnings)
	}
}
