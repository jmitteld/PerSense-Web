// Test for dispatch_gaps R4-3: weekly/biweekly loans on a 360-day
// basis are coerced to a 365-day basis (DOS Amortize.pas:297-303).
package api

import (
	"math"
	"strings"
	"testing"
)

func TestBiweeklyBasisCoercion(t *testing.T) {
	// Biweekly loan explicitly requesting the 360 basis.
	coerced, code := amzCall(t, `{"amount":100000,"rate":0.08,
		"loanDate":"2025-01-01","nPeriods":780,"perYr":26,"basis":"360"}`)
	if code != 200 || coerced["error"] != nil {
		t.Fatalf("biweekly calc failed: %v", coerced["error"])
	}
	// A coercion notice must be surfaced.
	warns, _ := coerced["warnings"].([]any)
	found := false
	for _, w := range warns {
		if s, _ := w.(string); strings.Contains(s, "365-day basis") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a 365-basis coercion warning, got %v", coerced["warnings"])
	}
	// The result must match an explicit 365-basis request.
	explicit, _ := amzCall(t, `{"amount":100000,"rate":0.08,
		"loanDate":"2025-01-01","nPeriods":780,"perYr":26,"basis":"365"}`)
	ti1, _ := coerced["totalInterest"].(float64)
	ti2, _ := explicit["totalInterest"].(float64)
	if math.Abs(ti1-ti2) > 0.01 {
		t.Errorf("coerced 360->365 total interest %.2f should match explicit 365 %.2f", ti1, ti2)
	}
}
