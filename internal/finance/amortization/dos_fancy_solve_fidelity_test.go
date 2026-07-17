package amortization

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Regression guards for the 2026-07-16 fancy-solve fidelity fixes surfaced by the
// adversarial fancy fuzzer (dos_fuzzer3_fancy_test.go) and resolved by crawling
// the DOS Pascal source (per CLAUDE.md "Replicate the DOS logic; do NOT patch").

// A known prepayment series that spans/over-fills the loan leaves no regular
// payment able to retire it, so DOS's Iterate does not converge and it blocks
// with "Computation of payment amount or interest rate did not converge."
// (Amortize.pas:416 → AMORTOP.pas:1489). The port used to keep the option-blind
// annuity seed and render a DEGENERATE schedule whose balance GROWS.
//
// Oracle provenance:
//
//	amort_oracle 360349.90 0.1572370000 6 1 exact inadv pre=12:6:1:356.96
//	  → ERR Computation of payment amount or interest rate did not converge.
func TestFancyPrepaymentNonConvergeMatchesDOS(t *testing.T) {
	s := gzSettings(1, types.Basis360, true, false, true, false, false) // exact, in-advance
	in := gzLoanInput(360349.90, 0.157237, 6, 1, s)
	in.Loan.PayAmtStatus = types.StatusEmpty
	in.Fancy = true
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2025, time.January, 1),
		NNStatus: types.InOutInput, NN: 6, PerYrStatus: types.InOutInput, PerYr: 1,
		PaymentStatus: types.InOutInput, Payment: 356.96}}
	r := Amortize(in)
	if r.Err == nil || !strings.Contains(r.Err.Error(), "did not converge") {
		t.Errorf("want 'did not converge' (DOS refuses the degenerate prepayment); got Err=%v rows=%d",
			r.Err, len(r.Schedule))
	}
	if len(r.Schedule) != 0 {
		t.Errorf("must return NO schedule (DOS blocks); got %d rows", len(r.Schedule))
	}
}

// A dominating balloon (PV ≥ the loan) has no positive-growth rate root: DOS's
// rate Iterate drives the rate below −100%, the per-period growth factor
// 1 + rate/RealPerYr goes non-positive, and DOS's Lnn/yield math aborts with
// "Error: The data you have specified contain an inconsistency." (INTSUTIL.pas:
// 1169) — NOT a converged rate. The port's fancy secant otherwise wandered to
// ~−151% and returned it. SolveRate now rejects a solved rate whose growth
// factor is ≤ 0, so it refuses (converged=false) as DOS does.
//
// Oracle provenance:
//
//	amort_oracle 119881.20 0.1227480000 10 1 b365 inadv b60=260175.49 payhard=13172.48 solverate
//	  → ERR Error: The data you have specified contain an inconsistency.
func TestFancyRateNegativeGrowthRefusesLikeDOS(t *testing.T) {
	s := gzSettings(1, types.Basis365, false, false, true, false, false) // in-advance, basis 365
	in := gzLoanInput(119881.20, 0.122748, 10, 1, s)
	in.Loan.LoanRateStatus = types.StatusEmpty
	in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, 13172.48
	in.Fancy = true
	in.Balloons = []BalloonPayment{{DateStatus: types.InOutInput,
		Date:         types.NewDateRec(2024+60/12, time.Month(60%12+1), 1),
		AmountStatus: types.InOutInput, Amount: 260175.49}}
	rate, conv, err := SolveRate(in)
	if err == nil && conv {
		t.Errorf("want a refusal (DOS: rate math inconsistency, no positive-growth root); "+
			"got converged rate=%.6f (growth factor 1+rate=%.4f)", rate, 1+rate)
	}
}
