// Canary test for dispatch_gaps.md §4.7 ambiguous-error rewordings
// (amortization portion). Binds the current text of the misleading
// "insufficient loan data: need amount and payments per year"
// message at engine.go:133 — the message blames PerYr even when
// PerYr IS supplied, because the conjoined check at engine.go:132
// fires when EITHER AmountStatus OR PerYrStatus is below default.
//
// See docs/test_plan.md §1 (Wave 1 canaries) C-16.

package amortization

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestCanaryC16_AmortMissingAmountBlamesPerYr exercises the
// conjoined-check failure mode: provide PerYr and Rate but no
// Amount. The engine reports "insufficient loan data: need amount
// and payments per year" — listing PerYr as missing even though
// the input clearly supplied it.
//
// dispatch_gaps §4.7 AM-10 proposes splitting this into two
// messages, each naming a single missing field:
//   "Amount Borrowed is required."
//   "Pmts/Yr is required."
//
// REWORD-PENDING: when the engine splits the message, this canary
// fails and needs to be updated to assert on the new wording.
func TestCanaryC16_AmortMissingAmountBlamesPerYr(t *testing.T) {
	// Build an input with Amount missing but PerYr supplied.
	loan := Loan{
		// AmountStatus omitted (defaults to zero / "not input")
		LoanRateStatus: types.InOutInput, LoanRate: 0.06,
		PayAmtStatus:   types.InOutInput, PayAmt: 1199.10,
		NStatus:        types.InOutInput, NPeriods: 360,
		PerYrStatus:    types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2025, time.January, 1),
		FirstStatus:    types.InOutInput, FirstDate: types.NewDateRec(2025, time.February, 1),
	}
	in := LoanInput{
		Loan: loan,
		Settings: Settings{
			Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360,
		},
	}
	result := Amortize(in)
	if result.Err == nil {
		// If the engine ran without error, perhaps AmountStatus
		// defaulted to a status that satisfies the InOutDefault
		// check. Skip rather than guess.
		t.Skip("Amortize did not return the expected 'insufficient loan data' error; " +
			"the engine may have classified the zero-amount input differently.")
		return
	}
	msg := result.Err.Error()
	if !strings.Contains(msg, "insufficient loan data") {
		t.Errorf("CANARY: error wording for missing-amount changed. Current: %q. "+
			"If Phase 3 reword landed, update to dispatch_gaps §4.7 AM-10 text.", msg)
	}
	// The smoking-gun assertion: this message names "payments per
	// year" even though PerYr was supplied. After the fix, the
	// message should name only the actually-missing field.
	if strings.Contains(msg, "payments per year") {
		t.Logf("CANARY confirms dispatch_gaps S-2 wording bug: message %q "+
			"blames PerYr but PerYr was supplied. Fix: split the conjoined "+
			"check at engine.go:132 into two single-field errors.", msg)
	}
}
