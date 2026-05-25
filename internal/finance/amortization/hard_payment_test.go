package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/interest"
	"github.com/persense/persense-port/internal/types"
)

// TestHardPaymentRoundsInterest verifies the DOS "Dav Holle
// provision" (dispatch_gaps G2): when the regular payment is a
// user-supplied hard number, the fancy schedule rounds every
// per-period interest figure to whole cents (AMORTOP.pas:637).
func TestHardPaymentRoundsInterest(t *testing.T) {
	in := baseInput30y() // baseInput30y supplies PayAmt as InOutInput
	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if len(res.Schedule) == 0 {
		t.Fatal("empty schedule")
	}
	for i, row := range res.Schedule {
		if got := interest.Round2(row.Interest); math.Abs(got-row.Interest) > 1e-9 {
			t.Errorf("row %d interest %.6f is not whole-cent rounded (want %.2f)",
				i+1, row.Interest, got)
		}
	}
}

// TestHardPaymentRoundsBalloon verifies that a sub-cent balloon
// amount is hardened to whole cents before the schedule runs
// (Amortize.pas:1430-1434).
func TestHardPaymentRoundsBalloon(t *testing.T) {
	in := baseInput30y()
	// A balloon amount carrying fractional cents.
	in.Balloons = []BalloonPayment{{
		DateStatus:   types.InOutInput,
		Date:         types.NewDateRec(2034, time.January, 1),
		AmountStatus: types.InOutInput,
		Amount:       25000.4973,
	}}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// Amortize hardens the caller's balloon slice in place.
	if got := in.Balloons[0].Amount; math.Abs(got-25000.50) > 1e-9 {
		t.Errorf("balloon amount = %.6f, want 25000.50 (hardened to cents)", got)
	}
}
