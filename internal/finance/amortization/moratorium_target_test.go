package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestMoratoriumTargetDenominator pins DOS's C-A-9 target-reachability guard
// (Amortize.pas:1299-1317), REWRITTEN 2026-07-24 for discrepancies.md §45.
//
// The guard is NOT unconditional. It lives inside
//
//	if (h^.lastok) and (DateComp(mor^.first_repay, h^.firstdate) <> 0) then …
//	else nrepay := h^.nperiods;   { no validation on this path }
//
// and Amortize.pas:1271-1276 DEFAULTS `mor^.first_repay := h^.firstdate` when
// no moratorium is set — so on a plain loan `DateComp(...) = 0`, the arm is
// skipped, and DOS never checks the target at all. The port previously
// rejected a plain loan whose target exceeded amount/NPeriods; DOS does not.
//
// ORACLE PROVENANCE (real DOS units, b365_360 exact prepaid, loandmy=1.1.2025
// firstdmy=1.2.2025 pts=0):
//
//	200000 0.08 360 12 payhard=1600 targ=600        → apr 0.079996  (accepted;
//	    amount/NPeriods = 555.56 < 600, i.e. exactly the case the old Go guard
//	    rejected)
//	100000 0.08005 360 12 payhard=1200 targ=300     → apr 0.080046  (accepted;
//	    amount/NPeriods = 277.78 < 300 — the client's reported input)
//
// When a moratorium IS set the guard is live, and its denominator is DOS's
// `nrepay = NumberOfInstallments(first_repay, lastdate, peryr, on_or_before)`.
func TestMoratoriumTargetDenominator(t *testing.T) {
	// baseInput30y: amount 200,000 over 360 periods, first 2/1/2024,
	// last 1/1/2054, 12/yr.
	//   amount/NPeriods = 555.56
	//   with first_repay 2/1/2029 (5y in): nrepay = 300, amount/nrepay = 666.67

	// A plain loan is NEVER target-validated by DOS, even when the target
	// exceeds amount/NPeriods. This is the arm the old port got wrong.
	plain := baseInput30y()
	plain.Target.TargetStatus = types.InOutInput
	plain.Target.TargetValue = 600 // > 555.56
	if res := Amortize(plain); res.Err != nil {
		t.Errorf("target 600 on a PLAIN loan must not be rejected — DOS skips the "+
			"C-A-9 arm entirely when no moratorium shifts first_repay "+
			"(Amortize.pas:1299); got %v", res.Err)
	}

	// With a moratorium, the guard is live and the denominator is nrepay (300),
	// so a target under amount/nrepay = 666.67 is reachable.
	ok := baseInput30y()
	ok.Target.TargetStatus = types.InOutInput
	ok.Target.TargetValue = 600
	ok.Moratorium.FirstRepayStatus = types.InOutInput
	ok.Moratorium.FirstRepay = types.NewDateRec(2029, time.February, 1)
	if res := Amortize(ok); res.Err != nil {
		t.Errorf("target 600 with a moratorium should be reachable "+
			"(amount/nrepay = 666.67); got %v", res.Err)
	}

	// …and one above amount/nrepay is rejected, by the moratorium arm.
	tooHigh := baseInput30y()
	tooHigh.Target.TargetStatus = types.InOutInput
	tooHigh.Target.TargetValue = 800 // > 666.67
	tooHigh.Moratorium.FirstRepayStatus = types.InOutInput
	tooHigh.Moratorium.FirstRepay = types.NewDateRec(2029, time.February, 1)
	if res := Amortize(tooHigh); res.Err == nil {
		t.Errorf("target 800 with a moratorium should be rejected " +
			"(exceeds amount/nrepay = 666.67)")
	}
}

// TestMoratoriumRepayAfterLastDate pins DOS's sibling guard at
// Amortize.pas:1306-1310 — `if (nrepay <= 0) then MessageBox('Principal
// repayment must begin before the last payment date.')` — which had no Go
// equivalent before §45. It is independent of the Target: DOS emits it purely
// to stop the `h^.amount / nrepay` divide below it from dividing by zero.
func TestMoratoriumRepayAfterLastDate(t *testing.T) {
	in := baseInput30y()
	in.Moratorium.FirstRepayStatus = types.InOutInput
	// Last pmt date is 1/1/2054; start principal repayment after it.
	in.Moratorium.FirstRepay = types.NewDateRec(2060, time.February, 1)
	res := Amortize(in)
	if res.Err == nil {
		t.Fatalf("moratorium first-repay after Last Pmt Date must be rejected")
	}
	if got := res.Err.Error(); !contains(got, "Principal repayment must begin") {
		t.Errorf("error = %q, want DOS's 'Principal repayment must begin before "+
			"the Last Pmt Date'", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
