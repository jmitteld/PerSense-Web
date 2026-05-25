// Canary test for dispatch_gaps.md AO8 / outstanding-items #4
// (CLAUDE.md): the Prepayment.NN ("nPmts" in the JSON request) field
// is accepted by the API but silently ignored by the engine. The
// engine's prepayment loop at engine.go:608-632 only consults
// StopDateStatus; a series specified by "24 extra payments" runs
// forever (until the loan naturally terminates).
//
// EXPECTED TO FAIL TODAY. When Phase 1 of dispatch_gaps.md adds an
// iteration counter that stops the series once NN payments have
// been applied, this canary flips to green.
//
// See docs/test_plan.md §1 (Wave 1 canaries) C-11.

package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestCanaryC11_PrepaymentNNSilentlyIgnored proves the engine
// ignores Prepayment.NN. Setup: a 30-year, 6%, $200k loan. Add a
// prepayment of $500/mo for 24 months specified ONLY via NN (no
// stopDate). DOS would apply exactly 24 extra payments.
//
// We measure by comparing total interest paid against a baseline
// with the same prepayment but specified by stopDate. If NN is
// honored, the schedules and totals should match (within rounding).
// If NN is ignored, the prepayment runs to the end of the loan and
// total interest is much lower.
//
// Pairs with: dispatch_gaps AO8 / CLAUDE.md outstanding #4.
func TestCanaryC11_PrepaymentNNSilentlyIgnored(t *testing.T) {
	// Baseline: prepayment specified by stopDate. The series starts
	// 2024-02-01 and runs monthly; for the baseline to cover exactly
	// 24 payments (matching NN=24 below) the StopDate must be the
	// date of the 24th payment — StartDate + 23 months = 2026-01-01.
	// The engine's StopDate test is inclusive (a payment ON the stop
	// date still applies), so a "+24 months" stop date of 2026-02-01
	// would apply 25 payments and not match NN=24.
	byStopDate := baseInput30y()
	byStopDate.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       types.NewDateRec(2024, time.February, 1),
		StopDateStatus:  types.InOutInput,
		StopDate:        types.NewDateRec(2026, time.January, 1), // 24th payment date
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         500,
	}}
	byStopResult := Amortize(byStopDate)
	if byStopResult.Err != nil {
		t.Fatal(byStopResult.Err)
	}

	// Same prepayment specified by NN=24 ONLY (no stopDate).
	byNN := baseInput30y()
	byNN.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       types.NewDateRec(2024, time.February, 1),
		NNStatus:        types.InOutInput,
		NN:              24,
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         500,
	}}
	byNNResult := Amortize(byNN)
	if byNNResult.Err != nil {
		t.Fatal(byNNResult.Err)
	}

	// Both runs should pay roughly the same total interest. If NN is
	// ignored, byNN runs the prepayment until the loan terminates —
	// massively reducing total interest compared to stopping at 24
	// payments.
	diff := byStopResult.TotalInt - byNNResult.TotalInt
	if diff > 100.0 || diff < -100.0 {
		t.Errorf("CANARY: NN-specified prepayment produced TotalInt=$%.2f vs "+
			"stopDate-specified $%.2f (diff $%.2f). The engine silently ignored "+
			"NN and ran the prepayment to natural loan termination "+
			"(dispatch_gaps AO8). Add an iteration counter to engine.go:608-632 "+
			"that stops once NN extras have been applied.",
			byNNResult.TotalInt, byStopResult.TotalInt, diff)
	}
}
