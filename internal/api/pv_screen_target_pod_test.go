package api

import (
	"encoding/json"
	"math"
	"testing"
)

// TestAPIPVScreenTargetLumpAmountWithPOD verifies dispatch_gaps §4.1
// S-9: with Rate and As-of Date both supplied, a typed screen-level
// Present Value (sumValue) back-solves a lump-sum row's missing Amount
// — including when a Payment on Death and a Living contingency are
// active, where the target must have PODValue subtracted (DOS
// PRESVALU.pas:839-849) and the solve divided by the survival
// probability (PRESVALU.pas:873-883). The round-trip (forward calc with
// the solved amount) must reproduce the target exactly.
func TestAPIPVScreenTargetLumpAmountWithPOD(t *testing.T) {
	actuarial := map[string]any{
		"table1":  podOnlyQx(),
		"dob1":    "1940-10-10",
		"asOfNow": "2024-01-01",
		"pod":     20000.0,
	}

	// Backward: lump row has a Date and a Living contingency but no
	// Amount and no row Value; the screen sumValue is the target.
	bwdBody, _ := json.Marshal(map[string]any{
		"asOfDate":  "2024-01-01",
		"rate":      0.06,
		"sumValue":  50000,
		"lumpSums":  []map[string]any{{"date": "2030-02-02", "act": "L"}},
		"actuarial": actuarial,
	})
	bwd, code := pvCall(t, string(bwdBody))
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	if bwd.Error != "" {
		t.Fatalf("screen-target solve rejected: %s", bwd.Error)
	}
	if len(bwd.LumpSums) != 1 {
		t.Fatalf("got %d lump sums, want 1", len(bwd.LumpSums))
	}
	solved := bwd.LumpSums[0].Amount
	if solved <= 0 {
		t.Fatalf("solved amount = %.2f, want > 0", solved)
	}
	if math.Abs(bwd.SumValue-50000) > 0.005 {
		t.Errorf("backward sumValue = %.2f, want 50000", bwd.SumValue)
	}

	// The POD must have been subtracted from the target before the row
	// solve: the row's own discounted value is the residual, strictly
	// less than the screen total.
	if bwd.LumpSums[0].Value >= 50000 {
		t.Errorf("row value = %.2f, want < 50000 (POD share missing from residual)",
			bwd.LumpSums[0].Value)
	}

	// Round-trip: forward calc with the solved amount reproduces the
	// 50000 target.
	fwdBody, _ := json.Marshal(map[string]any{
		"asOfDate": "2024-01-01",
		"rate":     0.06,
		"lumpSums": []map[string]any{
			{"date": "2030-02-02", "amount": solved, "act": "L"},
		},
		"actuarial": actuarial,
	})
	fwd, _ := pvCall(t, string(fwdBody))
	if fwd.Error != "" {
		t.Fatalf("forward round-trip error: %s", fwd.Error)
	}
	if math.Abs(fwd.SumValue-50000) > 0.01 {
		t.Errorf("round-trip total = %.2f, want 50000", fwd.SumValue)
	}
}
