package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestPrepaymentDurationSolved verifies dispatch_gaps FP6 / AO10: a
// prepayment series with a known amount but no stop date and no
// payment count has its duration solved — the engine runs it until
// the loan retires and pins NN / the stop date (DOS
// DeterminePrepaymentDuration).
func TestPrepaymentDurationSolved(t *testing.T) {
	in := baseInput30y() // 30-year, 360-period base loan
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       types.NewDateRec(2024, time.February, 1),
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         500, // $500/mo extra, no stop date / count
		NextDate:        types.NewDateRec(2024, time.February, 1),
	}}

	res := Amortize(in)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// A $500/mo prepayment retires a 30-year loan well before the
	// 360th regular payment.
	if len(res.Schedule) >= 360 {
		t.Errorf("schedule ran %d rows; the prepayment should retire the loan early",
			len(res.Schedule))
	}
	// The engine should have pinned the solved duration.
	if in.Prepayments[0].NNStatus < types.InOutDefault || in.Prepayments[0].NN <= 0 {
		t.Errorf("prepayment duration not solved: NNStatus=%d NN=%d",
			in.Prepayments[0].NNStatus, in.Prepayments[0].NN)
	}
	t.Logf("solved prepayment duration: NN=%d, %d schedule rows",
		in.Prepayments[0].NN, len(res.Schedule))
}
