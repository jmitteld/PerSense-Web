package api

import "testing"

// Regression guard for the multi-row PV backward-solve dispatch bug.
//
// ROOT CAUSE (2026-07-12 audit, discrepancies.md §26): a value-bearing lump row
// (Date + Value, no Amount) was classified LineContainsUnknown, but DOS
// ComputeLumpsumLineValues (PRESVALU.pas:174-178) makes Date+Value
// fully_specified — its face Amount is DERIVED from the Value and the Value is a
// KNOWN contributor to the screen total. On a multi-row backward screen the
// misclassification let the value-bearing row steal dispatch from the genuine
// single-field unknown: the wrong row got "solved" and the real unknown was left
// at 0, so the screen total failed to reconcile to the target Present Value.
//
// Case: target Present Value 100; Row A = Date 2025-01-01 + Value 40 (known);
// Row B = Date 2026-01-01 only (solve its Amount from the residual). Correct:
// A contributes 40, B is solved so its Value = 60, and 40 + 60 = 100.
func TestPVMultiRowBackwardSolvesGenuineUnknown(t *testing.T) {
	body := `{
		"asOfDate":"2024-01-01","rate":0.05,"sumValue":100,
		"lumpSums":[{"date":"2025-01-01","value":40},{"date":"2026-01-01"}]
	}`
	resp := postPV(t, body)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.LumpSums) != 2 {
		t.Fatalf("expected 2 lump rows, got %d", len(resp.LumpSums))
	}
	rowA, rowB := resp.LumpSums[0], resp.LumpSums[1]

	// Row A stays the known 40; its face Amount is derived (> value, since it is
	// discounted back from a future date).
	if !approxEqual(rowA.Value, 40, 0.01) {
		t.Errorf("Row A value: got %.4f, want 40 (known, unchanged)", rowA.Value)
	}
	if !(rowA.Amount > rowA.Value) {
		t.Errorf("Row A amount %.4f should exceed its present value %.4f", rowA.Amount, rowA.Value)
	}
	// Row B — the genuine unknown — is solved so its value is the residual 60,
	// NOT left at 0 (the pre-fix bug) and NOT the value-bearing row's own number.
	if !approxEqual(rowB.Value, 60, 0.01) {
		t.Errorf("Row B value: got %.4f, want 60 (= target 100 - known 40)", rowB.Value)
	}
	if rowB.Amount == 0 {
		t.Errorf("Row B amount was left at 0 — the genuine unknown was not solved")
	}
	// The screen reconciles to the target.
	if !approxEqual(rowA.Value+rowB.Value, 100, 0.02) {
		t.Errorf("screen total %.4f does not reconcile to target 100", rowA.Value+rowB.Value)
	}
}
