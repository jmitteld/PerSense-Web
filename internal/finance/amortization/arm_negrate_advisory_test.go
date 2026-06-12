package amortization

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestAO6NegativeImpliedRateAdvisory verifies A-W12: a payment-only ARM
// adjustment whose new payment is too LOW to amortize the balance over the
// remaining term implies a NEGATIVE rate (DOS computes and runs it,
// producing negative interest after the adjustment). The number is left
// exactly as DOS produces it; the engine only adds a non-blocking Note so
// the negative interest is explained. See docs/arm_adjustment_findings.md (AO6).
func TestAO6NegativeImpliedRateAdvisory(t *testing.T) {
	in := baseInput30y()
	// Payment-only adjustment 5 years in. The remaining ~$186k over ~300
	// periods needs ~$620/mo even at zero interest; a $400/mo payment is
	// below that, so solveAdjRate fits a NEGATIVE implied rate and the
	// post-adjustment interest rows go negative.
	in.Adjustments = []RateAdjustment{{
		DateStatus:   types.InOutInput,
		Date:         types.NewDateRec(2029, time.January, 1),
		AmountStatus: types.InOutInput,
		Amount:       400,
		AmtOK:        true,
	}}

	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}

	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "A-W12") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an A-W12 negative-implied-rate Note; warnings = %v", res.Warnings)
	}

	// Sanity: the advisory should be a Note tier (not a blocking error),
	// and the schedule must still be produced.
	if len(res.Schedule) == 0 {
		t.Error("expected a schedule alongside the advisory")
	}
}

// TestAO6PositiveRateNoAdvisory is the negative control: a modest
// payment-only adjustment implies a normal positive rate and must NOT
// raise the A-W12 Note.
func TestAO6PositiveRateNoAdvisory(t *testing.T) {
	in := baseInput30y()
	in.Adjustments = []RateAdjustment{{
		DateStatus:   types.InOutInput,
		Date:         types.NewDateRec(2029, time.January, 1),
		AmountStatus: types.InOutInput,
		Amount:       1400, // a little higher than 1199.10 -> positive rate
		AmtOK:        true,
	}}

	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "A-W12") {
			t.Errorf("did not expect A-W12 for a positive implied rate; warnings = %v", res.Warnings)
		}
	}
}
