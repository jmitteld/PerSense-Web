package types

import "testing"

func TestFormatAdvisory(t *testing.T) {
	got := FormatAdvisory(AdvisoryTier, "M-W1", []string{"price", "monthly"}, "Check the balloon.")
	want := "@@ADV|advisory|M-W1|price,monthly@@ Check the balloon."
	if got != want {
		t.Errorf("FormatAdvisory =\n  %q\nwant\n  %q", got, want)
	}
	// Empty fields.
	got2 := FormatAdvisory(NoteTier, "N-1", nil, "Heads up.")
	want2 := "@@ADV|note|N-1|@@ Heads up."
	if got2 != want2 {
		t.Errorf("FormatAdvisory empty fields = %q, want %q", got2, want2)
	}
}
