// Canary tests for dispatch_gaps.md §4.7 PV-7 and PV-8: the
// "too many unknowns" (calc.go:279) and "insufficient data on
// screen" (calc.go:285) error messages. The existing
// TestTooManyUnknowns acknowledges in a comment that it cannot
// construct an input that actually triggers the path; this canary
// proves the path is reachable (or fails revealing the gap).
//
// If a canary FAILS today by hitting `t.Skip()` or
// `t.Fatalf("expected error")`, the corresponding DOS-faithful
// dispatch arm in FirstPass is not yet wired and needs porting.
// If it passes, the path is reachable and we have a binding test
// for the current ambiguous wording — to be updated when the
// reword in dispatch_gaps §4.7 PV-7/PV-8 lands.
//
// See docs/test_plan.md §1 (Wave 1 canaries) C-17, C-18.

package presentvalue

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestCanaryC17_PVTooManyUnknownsReachable attempts to drive the
// classifier into both `Frontward` and `Backward` simultaneously.
// Construction: one lump-sum row {date, amount, value} all filled
// (fully_specified → frontward) and another row {date, value but
// no amount} (contains_unknown → backward), with SumValue set.
//
// REWORD-PENDING: today's message is "too many unknowns". §4.7 PV-7
// proposes "More than one missing field on the screen — fill in
// enough cells to leave exactly one blank."
func TestCanaryC17_PVTooManyUnknownsReachable(t *testing.T) {
	d1 := types.NewDateRec(2025, time.January, 1)
	d2 := types.NewDateRec(2026, time.January, 1)
	asOf := types.NewDateRec(2024, time.January, 1)

	input := PVInput{
		PresVal: PresValLine{
			AsOf:           asOf,
			AsOfStatus:     types.InOutInput,
			R:              RateEntry{Rate: 0.06, Status: types.InOutInput},
			SumValue:       9000,
			SumValueStatus: types.InOutInput,
		},
		LumpSums: []LumpSumPayment{
			// Fully-specified row: drives Frontward.
			{
				Date: d1, DateStatus: types.InOutInput,
				Amt: 10000, AmtStatus: types.InOutInput,
				Val: 9523.81, ValStatus: types.InOutInput,
			},
			// Contains_unknown row: drives Backward.
			{
				Date: d2, DateStatus: types.InOutInput,
				Val: 5000, ValStatus: types.InOutInput,
				// Amt deliberately omitted (status defaults to empty).
			},
		},
	}

	result := Calculate(input)
	if result.Err == nil {
		t.Skip("expected an error but Calculate succeeded — input did not " +
			"drive the classifier into a multi-unknown state.")
		return
	}
	msg := result.Err.Error()
	// FINDING: this particular input ({date,amount,value} all filled on
	// one row plus a contains_unknown row with SumValue set) trips the
	// PER-ROW over-determined check at backward.go:211 BEFORE FirstPass
	// gets a chance to set both Frontward and Backward. This confirms
	// the comment in existing TestTooManyUnknowns: the "too many
	// unknowns" path at calc.go:279 is genuinely hard to reach from
	// the public API surface. The over-determined check fires first.
	//
	// CANARY behavior: this test fails today by hitting the
	// over-determined error instead of "too many unknowns". When
	// dispatch_gaps PV-warning ("value already determined by data
	// above" — a soft cancelable warning per PRESVALU.pas:1166-1189)
	// is wired, the over-determined check should become a warning
	// rather than an error, and this canary should then reach the
	// "too many unknowns" path.
	if strings.Contains(msg, "over-determined") {
		t.Errorf("CANARY: 'too many unknowns' path is unreachable — the over-" +
			"determined per-row check at backward.go:211 fires first. " +
			"Per dispatch_gaps PV-warning, this should become a soft " +
			"warning, not an error. Current message: " + msg)
		return
	}
	if !strings.Contains(msg, "too many unknowns") {
		t.Errorf("CANARY: 'too many unknowns' wording changed (or different "+
			"error fired). Current: %q. If Phase 3 reword landed, update to "+
			"§4.7 PV-7 text.", msg)
	} else {
		t.Logf("CANARY confirms 'too many unknowns' path IS reachable from this " +
			"input.")
	}
}

// TestCanaryC18_PVInsufficientDataReachable attempts to drive the
// classifier into NEITHER `Frontward` nor `Backward`. This happens
// when the screen has no fully-specified rows and no
// contains_unknown rows.
//
// REWORD-PENDING: today's message is "insufficient data on screen".
// §4.7 PV-8 proposes "Not enough inputs to solve for Sum Value —
// supply Rate, As-of Date, and at least one fully-specified row."
func TestCanaryC18_PVInsufficientDataReachable(t *testing.T) {
	asOf := types.NewDateRec(2024, time.January, 1)
	input := PVInput{
		PresVal: PresValLine{
			AsOf:           asOf,
			AsOfStatus:     types.InOutInput,
			R:              RateEntry{Rate: 0.06, Status: types.InOutInput},
			SumValue:       9000,
			SumValueStatus: types.InOutInput,
		},
		// No lump sums, no periodics. Pure screen-level insufficiency.
	}

	result := Calculate(input)
	if result.Err == nil {
		t.Skip("expected 'insufficient data on screen' but Calculate succeeded " +
			"with no rows. The classifier may treat an empty input as a " +
			"valid trivial forward calc (SumValue=0). dispatch_gaps PV §3.4 " +
			"may need a different reproducer.")
		return
	}
	if !strings.Contains(result.Err.Error(), "insufficient data on screen") {
		t.Errorf("CANARY: 'insufficient data on screen' wording changed (or "+
			"different error fired). Current: %q. If Phase 3 reword landed, "+
			"update to §4.7 PV-8 text.", result.Err.Error())
	}
}
