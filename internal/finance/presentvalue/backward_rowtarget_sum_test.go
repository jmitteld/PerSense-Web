package presentvalue

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// A row-level backward solve (target typed in the ROW's Value cell, screen Sum
// Value blank) must still report the screen total. DOS Enter always follows
// BackwardCalc with FrontwardCalc (PRESVALU.pas:1253), which re-sums the rows
// into sumvalue; the port's date solvers previously echoed PresVal.SumValue = 0,
// so the UI showed a $0.00 total and the P-W7 "payments net to about zero"
// advisory fired on a screen that summed to the target. Found by the 2026-07-24
// UI-vs-oracle differential run (PV-S-02/04/05); oracle-validated solved values.
func TestBackwardRowTargetSumValue(t *testing.T) {
	base := func() PVInput {
		return PVInput{
			Settings: PVSettings{Basis: types.Basis360, YrDays: 360, YrInv: 1.0 / 360, PerYr: 12},
			PresVal: PresValLine{
				AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, 1, 1),
				R: RateEntry{Status: types.InOutInput, Rate: 0.06},
			},
		}
	}

	t.Run("lump date solve (PV-2)", func(t *testing.T) {
		in := base()
		in.LumpSums = []LumpSumPayment{{
			AmtStatus: types.InOutInput, Amt: 50000,
			ValStatus: types.InOutInput, Val: 44000,
		}}
		r := Calculate(in)
		if r.Err != nil {
			t.Fatalf("solve failed: %v", r.Err)
		}
		if math.Abs(r.SumValue-44000) > 0.01 {
			t.Errorf("SumValue = %v, want 44000 (the row target)", r.SumValue)
		}
		for _, w := range r.Warnings {
			if len(w) >= 4 && (w == "P-W7" || (len(w) > 10 && containsPW7(w))) {
				t.Errorf("bogus P-W7 advisory fired: %q", w)
			}
		}
	})

	t.Run("periodic to-date solve (PV-5)", func(t *testing.T) {
		in := base()
		in.Periodics = []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: types.NewDateRec(2024, 1, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 1000,
			ValStatus: types.InOutInput, Val: 33000,
		}}
		r := Calculate(in)
		if r.Err != nil {
			t.Fatalf("solve failed: %v", r.Err)
		}
		if math.Abs(r.SumValue-33000) > 0.01 {
			t.Errorf("SumValue = %v, want 33000 (the row target)", r.SumValue)
		}
	})

	t.Run("periodic from-date solve (PV-6)", func(t *testing.T) {
		in := base()
		in.Periodics = []PeriodicPayment{{
			ToDateStatus: types.InOutInput, ToDate: types.NewDateRec(2027, 1, 1),
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 1000,
			ValStatus: types.InOutInput, Val: 20000,
		}}
		r := Calculate(in)
		if r.Err != nil {
			t.Fatalf("solve failed: %v", r.Err)
		}
		if math.Abs(r.SumValue-20000) > 0.01 {
			t.Errorf("SumValue = %v, want 20000 (the row target)", r.SumValue)
		}
	})

	// Screen-level targets (rate / as-of solves) must keep echoing the typed
	// Sum Value exactly as before.
	t.Run("screen-target rate solve unchanged", func(t *testing.T) {
		in := base()
		in.PresVal.R = RateEntry{Status: types.StatusEmpty}
		in.PresVal.SumValueStatus = types.InOutInput
		in.PresVal.SumValue = 44000
		in.LumpSums = []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: types.NewDateRec(2026, 6, 1),
			AmtStatus:  types.InOutInput, Amt: 50000,
		}}
		r := Calculate(in)
		if r.Err != nil {
			t.Fatalf("solve failed: %v", r.Err)
		}
		if math.Abs(r.SumValue-44000) > 0.01 {
			t.Errorf("SumValue = %v, want the typed 44000", r.SumValue)
		}
	})
}

func containsPW7(s string) bool {
	for i := 0; i+4 <= len(s); i++ {
		if s[i:i+4] == "P-W7" {
			return true
		}
	}
	return false
}
