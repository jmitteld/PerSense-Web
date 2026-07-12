package presentvalue

import (
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Regression guard for audit finding D1 (discrepancies.md §29): the life and
// variable-rate stepped-COLA paths advanced the COLA anniversary with
// dateutil.AddYears, which CLAMPS month-ends (Feb-29 → Feb-28) and lands the
// COLA step one payment early on a leap-day / month-end fromDate. DOS uses a
// plain year-field increment `inc(coladate.y)` (PRESVALU.pas:289,302), and the
// fixed-rate path (periodicSumAnnualCOLA's incYear) already did — the life/VR
// paths must match. All three now route through nextColaAnniversary.
func TestColaAnniversaryPlainYearIncrement_D1(t *testing.T) {
	if got := nextColaAnniversary(types.NewDateRec(2024, 2, 29)); got.Time != types.NewDateRec(2025, 2, 29).Time {
		t.Errorf("nextColaAnniversary(2024-02-29) = %s, want plain year++ (2025-02-29 → normalized 2025-03-01)",
			got.Time.Format("2006-01-02"))
	}

	sAnn := &PVSettings{COLAMonth: types.COLAAnnual, Basis: types.Basis365, YrDays: 365.25}
	got, err := firstCOLAStepDate(types.NewDateRec(2024, 2, 29), sAnn)
	if err != nil {
		t.Fatal(err)
	}
	want := types.NewDateRec(2025, 2, 29) // normalizes to 2025-03-01
	if got.Time != want.Time {
		t.Errorf("firstCOLAStepDate(2024-02-29, ANN) = %s, want %s (AddYears clamp would give 2025-02-28)",
			got.Time.Format("2006-01-02"), want.Time.Format("2006-01-02"))
	}

	got2, _ := firstCOLAStepDate(types.NewDateRec(2024, 3, 15), sAnn)
	if got2.Time != types.NewDateRec(2025, 3, 15).Time {
		t.Errorf("firstCOLAStepDate(2024-03-15, ANN) = %s, want 2025-03-15", got2.Time.Format("2006-01-02"))
	}
}
