package dateutil

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestNumberOfInstallments_RoundTrip pins the NumberOfInstallments port added
// for the prepayment-duration solver (Gap B). For any frequency and count, the
// n-th payment date is AddNPeriods(first, n-1); counting installments from
// first to that date (on-or-before) must recover n.
func TestNumberOfInstallments_RoundTrip(t *testing.T) {
	first := types.NewDateRec(2024, time.February, 1)
	for _, peryr := range []int{1, 2, 4, 6, 12} {
		for n := 1; n <= 40; n++ {
			last, err := AddNPeriods(first, peryr, n-1)
			if err != nil {
				t.Fatalf("AddNPeriods peryr=%d n=%d: %v", peryr, n, err)
			}
			got, _ := NumberOfInstallments(first, last, peryr, types.OnOrBefore)
			if got != n {
				t.Errorf("peryr=%d n=%d: round-trip count = %d", peryr, n, got)
			}
		}
	}
}

// TestNumberOfInstallments_Known pins a couple of hand-checkable cases and the
// before/on-or-before distinction.
func TestNumberOfInstallments_Known(t *testing.T) {
	jan24 := types.NewDateRec(2024, time.January, 1)
	jan25 := types.NewDateRec(2025, time.January, 1)

	// Jan 2024 .. Jan 2025, monthly, inclusive of both ends → 13.
	if n, _ := NumberOfInstallments(jan24, jan25, 12, types.OnOrBefore); n != 13 {
		t.Errorf("monthly Jan24..Jan25 on-or-before = %d, want 13", n)
	}
	// `before` excludes the final payment date itself → 12.
	if n, _ := NumberOfInstallments(jan24, jan25, 12, types.Before); n != 12 {
		t.Errorf("monthly Jan24..Jan25 before = %d, want 12", n)
	}
	// Quarterly over one year, on-or-before → 5 (Jan, Apr, Jul, Oct, Jan).
	if n, _ := NumberOfInstallments(jan24, jan25, 4, types.OnOrBefore); n != 5 {
		t.Errorf("quarterly Jan24..Jan25 on-or-before = %d, want 5", n)
	}
}
