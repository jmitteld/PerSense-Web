package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestPrepaymentDurationSolved verifies dispatch_gaps FP6 / AO10: a prepayment
// series with a known amount but no stop date and no payment count has its
// duration solved via DOS's closed-form present-value duration
// (DeterminePrepaymentDuration). The regular payment is held BELOW the
// fully-amortizing payment so a residual remains for the additive prepayment to
// retire — otherwise DOS (and now Go) report "principal is more than covered by
// the fixed payments", which the companion case below checks.
func TestPrepaymentDurationSolved(t *testing.T) {
	in := baseInput30y() // 30-year, 360-period base loan (amortizing pmt 1199.10)
	// Below-amortizing regular payment so the duration solve is well posed.
	in.Loan.PayAmt = 800
	// Additive extra payment (on top of the regular payment) — requires
	// plus_regular ON; the default replaces the regular payment (a payment
	// schedule). See docs/prepayment_semantics_finding.md.
	in.Settings.PlusRegular = true
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
	// The engine should have pinned the solved duration (validated against the
	// real DOS engine in TestDOSPrepaymentDurationSolveSweep).
	if in.Prepayments[0].NNStatus < types.InOutDefault || in.Prepayments[0].NN <= 0 {
		t.Errorf("prepayment duration not solved: NNStatus=%d NN=%d",
			in.Prepayments[0].NNStatus, in.Prepayments[0].NN)
	}
	t.Logf("solved prepayment duration: NN=%d, %d schedule rows",
		in.Prepayments[0].NN, len(res.Schedule))
}

// TestPrepaymentDurationRejectsCoveredPrincipal pins the DOS-faithful guard: if
// the regular payment alone already amortizes the loan, the duration of an
// additive prepayment is "negative" and DeterminePrepaymentDuration reports an
// error rather than inventing an early-payoff count (AMORTIZE.pas:748-752).
func TestPrepaymentDurationRejectsCoveredPrincipal(t *testing.T) {
	in := baseInput30y() // PayAmt 1199.10 fully amortizes the loan
	in.Settings.PlusRegular = true
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       types.NewDateRec(2024, time.February, 1),
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         500,
		NextDate:        types.NewDateRec(2024, time.February, 1),
	}}
	res := Amortize(in)
	if res.Err == nil {
		t.Fatalf("expected a 'principal more than covered' error; got NN=%d, %d rows",
			in.Prepayments[0].NN, len(res.Schedule))
	}
}
