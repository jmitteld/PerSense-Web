package amortization

// §57 REGRESSION — SolveRate must run DOS's Iterate even when the port's own
// closed-form pre-solve exhausts its iteration budget.
//
// THE DEFECT (round 15, 2026-08-01)
//
// DOS's EstimateAndRefineRate (Amortize.pas:467-491) has NO pre-solve:
//
//	loanrate := payamt * peryr / amount;  if (loanrate<0.02) then 0.02;
//	if Iterate(amount, usap, loandate, firstdate, loanrate, til_adj) then ...
//
// It seeds and calls Iterate, unconditionally. The port added a closed-form
// Newton loop in front of that (backward.go, `const maxIter = 30`) and made the
// DOS Iterate reachable ONLY from inside the loop's `math.Abs(step) < teeny`
// convergence branch. So whenever the port's pre-solve failed to settle in 30
// iterations, SolveRate fell out of the loop and returned the UNREFINED
// closed-form estimate with converged=false — without ever attempting the DOS
// Iterate, which converges on those screens to the last digit DOS prints.
//
// WHERE IT IS LIVE: loans deep into perpetuity territory, where the hardened
// payment is within a hair of A*i and the principal barely amortizes. Measured
// by the perpetuity depth (1+i)^-n; the failures below all sit under 1e-5.
// RepayLoan's terminal balance is astronomically stiff in the rate there, the
// secant's reused `delta` collapses into cancellation noise, and |step| never
// reaches teeny.
//
// HOW IT WAS FOUND, and why that matters: the round-trip gate
// (zzroundtrip_test.go, round 14) reported the port recovering an entered rate
// ~50,000x worse than DOS at long horizons, and round 14 left it unadjudicated
// because §54 (Feb 2100) and §55 (the 2155 year byte) were confounded with it.
// TestRoundTripRateHorizonStrata separated them: the failures appear at spans
// ending 2095-2098 — short of BOTH date boundaries — and the matched-n control
// put the effect on the calendar span rather than the iteration count. It is a
// solver defect, not a date-layer defect. Nothing in this file depends on the
// round-trip harness; it is a plain oracle differential.
//
// COMPONENTS, verified separately (standing rule 3):
//  1. the post-loop dosIterateRate fallback          -> TestSec57PreSolveExhausted
//  2. inert on the pre-solve-CONVERGED branch        -> TestSec57InertWherePreSolveConverges
//  3. the ±200% refusal still fires (order note)     -> TestSec57RefusalStillFires
//  4. a negative refined rate is still accepted      -> TestSec57NegativeRateStillAccepted

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// sec57Input builds a plain arrears 360-basis screen with the rate blanked.
// Deliberately self-contained — this regression must not depend on the
// round-trip harness's helpers.
func sec57Input(amount float64, n, perYr int, pay float64) LoanInput {
	firstMonth := 1 + 12/perYr
	firstYear := 2024
	if firstMonth > 12 {
		firstMonth -= 12
		firstYear++
	}
	return LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.StatusEmpty, LoanRate: 0,
		NStatus: types.InOutInput, NPeriods: n,
		PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus: types.InOutInput, PayAmt: pay,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
		FirstStatus: types.InOutInput,
		FirstDate:   types.NewDateRec(firstYear, time.Month(firstMonth), 1)},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr),
			YrDays: 360, YrInv: 1.0 / 360}}
}

// TestSec57PreSolveExhausted is the defect itself.
//
// Every golden below is `solvedrate` from the real DOS engine. Provenance —
// commands run 2026-08-01 against legacy/oracle/amort_oracle (build_linux.sh):
//
//	$ amort_oracle 77668.37 0.1687130000 864 12 payhard=1091.98 norate
//	payment 1091.9800 interest 864010.58 paid 941678.95
//	solvedrate 0.1687132662
//
//	$ amort_oracle 229095.37 0.1853950000 426 6 payhard=7078.87 norate
//	payment 7078.8700 interest 2786133.45 paid 3015228.82
//	solvedrate 0.1853949316
//
//	$ amort_oracle 328360.37 0.1886200000 888 12 payhard=5161.28 norate
//	payment 5161.2800 interest 4583216.64 paid 4911577.01
//	solvedrate 0.1886198999
//
// Pre-fix the port returned 0.1687354022 / 0.1855860626 / 0.1890050073 with
// converged=false — the raw closed-form estimate, 84x to 3830x further from
// DOS's answer than DOS's own inverse error.
func TestSec57PreSolveExhausted(t *testing.T) {
	cases := []struct {
		name        string
		amount      float64
		n, perYr    int
		pay         float64
		wantRate    float64
		preFixRate  float64 // what the port returned before §57
		wantDepthLT float64 // perpetuity depth, the region marker
	}{
		{"864pmt-monthly-72yr", 77668.37, 864, 12, 1091.98, 0.1687132662, 0.1687354022, 1e-5},
		{"426pmt-bimonthly-71yr", 229095.37, 426, 6, 7078.87, 0.1853949316, 0.1855860626, 1e-5},
		{"888pmt-monthly-74yr", 328360.37, 888, 12, 5161.28, 0.1886198999, 0.1890050073, 1e-5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, converged, err := SolveRate(sec57Input(c.amount, c.n, c.perYr, c.pay))
			if err != nil {
				t.Fatalf("SolveRate errored where DOS solved: %v", err)
			}
			// 5e-6 absolute, the same bar dos_fuzzer5_test.go:1508 uses for a
			// solved rate: tighter than that is below DOS's own refinement stop
			// tolerance and would be reporting the solver's last step.
			if math.Abs(got-c.wantRate) > 5e-6 {
				t.Errorf("solved rate = %.10f, DOS %.10f (delta %+.2e)\n"+
					"  amort_oracle %.2f %.10f %d %d payhard=%.2f norate",
					got, c.wantRate, got-c.wantRate,
					c.amount, c.wantRate, c.n, c.perYr, c.pay)
			}
			if !converged {
				t.Errorf("converged=false where DOS converged — the port would show a "+
					"'did not converge' warning on a screen DOS answers cleanly "+
					"(solved %.10f, DOS %.10f)", got, c.wantRate)
			}
			// The pre-fix answer must NOT be what we return. Stated explicitly so
			// a future refactor that silently restores the unrefined estimate
			// fails here with the reason rather than only on the tolerance.
			if math.Abs(got-c.preFixRate) < 1e-9 {
				t.Errorf("returned the pre-§57 UNREFINED closed-form estimate %.10f — "+
					"the DOS Iterate was not reached", c.preFixRate)
			}
			// Region marker: if a future change makes this case converge in the
			// pre-solve, the test still passes but stops covering §57. Assert the
			// case is still in the region the defect lives in.
			i := c.wantRate / float64(c.perYr)
			if d := math.Pow(1+i, -float64(c.n)); d >= c.wantDepthLT {
				t.Errorf("case drifted out of the §57 region: perpetuity depth %.3e >= %.0e",
					d, c.wantDepthLT)
			}
		})
	}
}

// TestSec57InertWherePreSolveConverges proves the fix did not disturb the branch
// it sits behind — an ordinary 30-year mortgage, where the closed-form pre-solve
// converges and the refinement path was already correct. Component 2.
//
//	$ amort_oracle 150000.37 0.0925000000 360 12 payhard=1234.53 norate
//	payment 1234.5300 interest 294429.65 paid 444430.02
//	solvedrate 0.0925472664
func TestSec57InertWherePreSolveConverges(t *testing.T) {
	got, converged, err := SolveRate(sec57Input(150000.37, 360, 12, 1234.53))
	if err != nil {
		t.Fatalf("SolveRate errored: %v", err)
	}
	if math.Abs(got-0.0925472664) > 5e-6 {
		t.Errorf("solved rate = %.10f, DOS 0.0925472664 (delta %+.2e)", got, got-0.0925472664)
	}
	if !converged {
		t.Error("converged=false on an ordinary 30-year loan")
	}
	// The pre-solve must genuinely be the path taken here, or this case proves
	// nothing about inertness. Depth well above the §57 region.
	if d := math.Pow(1+0.0925472664/12, -360); d < 1e-5 {
		t.Errorf("control case is inside the §57 region (depth %.3e); it cannot "+
			"witness the untouched branch", d)
	}
}

// TestSec57RefusalStillPrecedesFallback pins the ORDER. DOS's Iterate bails on
// `abs(loanrate) > 2` and EstimateAndRefineRate then refuses the screen
// (AMORTOP.pas:1485-1489), and the new fallback must not rescue that.
// Component 3.
//
//	$ amort_oracle 10000 0 12 12 norate pay=2500
//	ERR Computation of payment amount or interest rate did not converge.
//
// HONEST LIMIT OF THIS CASE, recorded rather than papered over: the fix places
// the fallback AFTER the ±200% check, and this test passes with the two SWAPPED
// as well (verified by hand, round 15) — on this screen the DOS Iterate does not
// return an in-range rate either, so the ordering is not observable here. It
// therefore witnesses that the refusal STILL FIRES, not that the order is
// load-bearing. Why no such case was found is itself a live question: the port's
// ±200% refusal is triggered by the PORT's pre-solve wandering out of range,
// whereas DOS's is triggered by DOS's own Iterate — the same "a Go-only
// pre-solve is gating a DOS decision" shape as §57. See §57's "still open" note
// in docs/discrepancies.md.
func TestSec57RefusalStillFires(t *testing.T) {
	in := sec57Input(10000, 12, 12, 2500)
	got, converged, err := SolveRate(in)
	if err == nil {
		t.Fatalf("expected DOS's non-convergence refusal, got rate %.10f converged=%v",
			got, converged)
	}
	if !strings.Contains(err.Error(), "did not converge") {
		t.Errorf("wrong refusal: %v", err)
	}
	if converged {
		t.Error("converged=true on a refused screen")
	}
}

// TestSec57NegativeRateStillAccepted guards acceptRefinedRate's negative arm,
// which the §57 refactor moved out of SolveRate's body. An under-funded loan
// (120 x 750 = 90,000 against 100,000 principal) has a genuinely negative
// implied rate and DOS returns it. Component 4.
//
//	$ amort_oracle 100000 0.05 120 12 payhard=750.00 norate
//	payment 750.0000 interest -10000.02 paid 89999.98
//	solvedrate -0.0205315356
func TestSec57NegativeRateStillAccepted(t *testing.T) {
	got, converged, err := SolveRate(sec57Input(100000, 120, 12, 750.00))
	if err != nil {
		t.Fatalf("SolveRate errored on an under-funded loan DOS solves: %v", err)
	}
	if got >= 0 {
		t.Errorf("solved rate = %.10f, want the NEGATIVE root DOS returns (-0.0205315356)", got)
	}
	if math.Abs(got-(-0.0205315356)) > 5e-6 {
		t.Errorf("solved rate = %.10f, DOS -0.0205315356 (delta %+.2e)", got, got+0.0205315356)
	}
	if !converged {
		t.Error("converged=false on a loan DOS solves cleanly")
	}
}
