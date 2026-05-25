package mortgage

import (
	"math"
	"testing"
)

// TestIterateAPRofTerminatedLoan verifies dispatch_gaps FP10's
// building block: for a no-points mortgage terminated at its full
// term, the terminated-loan APR (IterateToFindAPR — the faithful DOS
// port that tryBalloonDates now uses) equals the full-term APR.
func TestIterateAPRofTerminatedLoan(t *testing.T) {
	res := Calc(makeBasicMortgage())
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	line := res.Line
	full, conv, err := FullTermAPR(line, 365.25)
	if err != nil || !conv {
		t.Fatalf("FullTermAPR failed: conv=%v err=%v", conv, err)
	}
	apr, conv2, err := IterateToFindAPR(line, float64(line.Years), 365.25)
	if err != nil || !conv2 {
		t.Fatalf("IterateToFindAPR did not converge: conv=%v err=%v", conv2, err)
	}
	if math.Abs(apr-full) > 0.005 {
		t.Errorf("terminated-loan APR %.5f should match full-term APR %.5f", apr, full)
	}
}

// TestTryBalloonDatesNoBalloon: with no balloons there is nothing to
// pin to, so the fallback declines cleanly (ok=false) rather than
// panicking.
func TestTryBalloonDatesNoBalloon(t *testing.T) {
	a := Calc(makeBasicMortgage())
	b := Calc(makeBasicMortgage())
	if a.Err != nil || b.Err != nil {
		t.Fatal("setup calc failed")
	}
	if _, _, ok := tryBalloonDates(a.Line, b.Line, 365.25); ok {
		t.Errorf("tryBalloonDates should decline when neither mortgage has a balloon")
	}
}

// TestCompareAPRsRunsClean: an APR comparison between two fully
// computed mortgages completes without error and yields a summary.
func TestCompareAPRsRunsClean(t *testing.T) {
	mk := func(rate float64) MtgLine {
		m := makeBasicMortgage()
		m.Rate = rate
		return Calc(m).Line
	}
	res, err := CompareAPRs(mk(0.06), mk(0.065), 365.25)
	if err != nil {
		t.Fatalf("CompareAPRs errored: %v", err)
	}
	if res.Summary == "" {
		t.Errorf("expected a comparison summary")
	}
}

// TestCompareAPRsRejectsUnderspecified verifies the §1.5-4 fix:
// CompareAPRs now gates on EnoughDataForAPR and reports an error
// rather than churning the iteration against an empty mortgage.
func TestCompareAPRsRejectsUnderspecified(t *testing.T) {
	good := Calc(makeBasicMortgage()).Line
	var empty MtgLine // no data at all
	if _, err := CompareAPRs(good, empty, 365.25); err == nil {
		t.Errorf("expected an error comparing against an under-specified mortgage")
	}
}
