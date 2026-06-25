package amortization

import (
	"math"
	"os"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestAO7BalloonDOSBugCharacterization pins the ONE place both Go engines
// deliberately diverge from the DOS oracle: a DATE-ONLY adjustment (AO7 — blank
// rate, blank amount) combined with a balloon dated AFTER it. DOS's Re_Amortize
// produces a degenerate early payoff (the blank adjustment's zero-valued rate/
// amount fields leak into the balloon-forced Iterate — DOS even prints
// "re-computed at 0.0000%: Payment fixed at 0.00"), retiring 100k/24mo/6% +
// balloon@12 + adj@6:: with interest ~3172 instead of the financially-correct
// ~6331. BOTH the production piecewise engine AND the faithful port produce the
// correct ~6331 (continue to term), so the gate routes AO6/AO7+balloon to
// piecewise and this is a documented, intentional divergence — reproducing the DOS
// bug would replicate nonsensical output. See docs/dos_known_frontier.md.
func TestAO7BalloonDOSBugCharacterization(t *testing.T) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.06,
			NStatus: types.InOutInput, NPeriods: 24,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(12),
		},
		Settings:    Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:       true,
		Balloons:    []BalloonPayment{balloonAt(12, 20000)},
		Adjustments: []RateAdjustment{adjDateOnly(6)},
	}
	pw := Amortize(in)
	port := AmortizeDOS(in)
	if pw.Err != nil || port.Err != nil {
		t.Fatalf("unexpected error pw=%v port=%v", pw.Err, port.Err)
	}
	// Both Go engines produce the financially-correct full-term result (~6331),
	// and agree with each other; only DOS (a bug) gives ~3172.
	const wantSane = 6331.47
	if math.Abs(pw.TotalInt-wantSane) > 1.0 {
		t.Errorf("production total interest %.2f, want the sane ~%.2f", pw.TotalInt, wantSane)
	}
	if math.Abs(port.TotalInt-pw.TotalInt) > 1.0 {
		t.Errorf("port %.2f and production %.2f should agree (both sane)", port.TotalInt, pw.TotalInt)
	}
	if math.Abs(port.FinalPrinc) > 1.0 {
		t.Errorf("port should retire the loan to ~0, got FinalPrinc=%.2f", port.FinalPrinc)
	}
	t.Logf("AO7+balloon: both Go engines = %.2f (correct); DOS oracle = ~3172.08 (documented bug)", port.TotalInt)
}

// TestAO7BalloonOracleIsBug optionally confirms the DOS oracle still exhibits the
// bug (so the characterization stays honest if the oracle is ever rebuilt). Opt-in.
func TestAO7BalloonOracleIsBug(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	dosInt, ok := runOracleInterestFlags(100000, 0.06, 24, 12, "adj=6::", "b12=20000.00", "plusreg")
	if !ok {
		t.Skip("oracle produced no result")
	}
	// The bug: DOS reports ~3172 (degenerate early payoff), NOT the correct ~6331.
	if math.Abs(dosInt-3172.08) > 5.0 {
		t.Logf("NOTE: oracle no longer reports the ~3172 degenerate value (got %.2f) — "+
			"the DOS AO7+balloon bug behavior may have changed; revisit the gate rationale", dosInt)
	} else {
		t.Logf("confirmed: DOS oracle still exhibits the AO7+balloon early-payoff bug (%.2f vs correct ~6331)", dosInt)
	}
}
