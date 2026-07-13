package presentvalue

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Regression guard for audit A1 (discrepancies.md §31): the infinite-series guard
// for a perpetual periodic stream (toDate == latest) compared the RAW yield COLA
// against the rate, but the series converges to a FINITE PV whenever the rate
// exceeds the CONTINUOUS COLA, ln(1+cola). DOS compares the continuous COLA
// (PRESVALU.pas:379). So in the band rate ∈ (ln(1+cola), cola] the port wrongly
// errored where DOS returns a finite value.
func TestInfinitePerpetualGuard_A1(t *testing.T) {
	cola := 0.10
	lnCola := math.Log1p(cola)
	s := &PVSettings{Basis: types.Basis360, PerYr: 12, COLAMonth: types.COLAContinuous, YrDays: 360, YrInv: 1.0 / 360}
	asof := types.NewDateRec(2024, 1, 1)
	from := types.NewDateRec(2024, 1, 1)
	forever := types.LatestDate()

	for _, rate := range []float64{lnCola + 0.001, 0.098, cola} {
		v, err := PeriodicSummation(rate, cola, asof, from, forever, 12, 999999, s)
		if err != nil {
			t.Errorf("rate %.5f (> ln(1+cola)=%.5f): errored, want finite PV: %v", rate, lnCola, err)
		} else if v <= 0 || math.IsInf(v, 0) || math.IsNaN(v) {
			t.Errorf("rate %.5f: non-finite factor %v", rate, v)
		}
	}
	if _, err := PeriodicSummation(lnCola-0.001, cola, asof, from, forever, 12, 999999, s); err == nil {
		t.Errorf("rate below ln(1+cola) should still report the infinite-PV error")
	}
}
