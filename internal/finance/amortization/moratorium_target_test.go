package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestMoratoriumTargetDenominator verifies dispatch_gaps V6-8: the
// target-too-high check uses amount / nrepay (post-moratorium period
// count), not amount / NPeriods. A target between those two bounds is
// unreachable on a plain loan but reachable once a moratorium shrinks
// the repaying-period count.
func TestMoratoriumTargetDenominator(t *testing.T) {
	// baseInput30y: amount 200,000 over 360 periods.
	//   amount/NPeriods = 555.56  (plain-loan floor)
	//   with a 60-month moratorium: nrepay = 300, amount/nrepay = 666.67
	// A target of 600 sits between the two.
	const target = 600.0

	plain := baseInput30y()
	plain.Target.TargetStatus = types.InOutInput
	plain.Target.TargetValue = target
	if res := Amortize(plain); res.Err == nil {
		t.Errorf("target %.0f should be rejected on a plain loan "+
			"(exceeds amount/NPeriods)", target)
	}

	withMor := baseInput30y()
	withMor.Target.TargetStatus = types.InOutInput
	withMor.Target.TargetValue = target
	withMor.Moratorium.FirstRepayStatus = types.InOutInput
	withMor.Moratorium.FirstRepay = types.NewDateRec(2029, time.February, 1) // 5y in
	if res := Amortize(withMor); res.Err != nil {
		t.Errorf("target %.0f should be reachable with a moratorium "+
			"(amount/nrepay is larger); got %v", target, res.Err)
	}
}
