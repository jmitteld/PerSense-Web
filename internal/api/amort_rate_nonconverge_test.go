package api

import (
	"strings"
	"testing"
)

// TestAmortRateSolveNonConvergeMatchesDOS guards the DOS-fidelity fix for the rate
// backward-solve. When the payment is far below principal ÷ term the implied rate
// is deeply negative, and DOS's secant Iterate — seeded at the floored +2% first
// guess (Amortize.pas:9-11) — cannot reach it, so DOS refuses with "Computation of
// payment amount or interest rate did not converge." The port previously echoed a
// (valid, Go-solved) rate with a soft warning and a full schedule; now SolveRate
// reports non-convergence (it refines dosIterateRate from the DOS seed, which
// diverges as DOS's does) and the handler returns the DOS error with NO schedule.
// Converged mild-negative rates — which DOS DOES solve — still return a schedule.
//
// Oracle provenance:
//
//	amort_oracle 4000000 0.05 360 12 payhard=67.59 solverate loandmy=1.1.2020 firstdmy=1.2.2020
//	  -> ERR Computation of payment amount or interest rate did not converge.
//	amort_oracle 100000 0.06 120 12 payhard=500 solverate loandmy=1.1.2020 firstdmy=1.2.2020
//	  -> rate -0.093695 status 1
func TestAmortRateSolveNonConvergeMatchesDOS(t *testing.T) {
	// Degenerate under-funding: $67.59/mo on $4,000,000 over 360 — DOS refuses.
	deg := postAmort(t, `{"loanDate":"2020-01-01","firstDate":"2020-02-01","perYr":12,
		"amount":4000000,"nPeriods":360,"payment":67.59,"basis":"360"}`)
	if !strings.Contains(deg.Error, "did not converge") {
		t.Errorf("degenerate rate solve: want a 'did not converge' error (DOS refuses); got Error=%q rate=%v schedule=%d rows",
			deg.Error, deg.Rate, len(deg.Schedule))
	}
	if len(deg.Schedule) != 0 {
		t.Errorf("degenerate rate solve must return NO schedule (DOS blocks); got %d rows", len(deg.Schedule))
	}

	// Converged mild-negative: $500/mo on $100,000 over 120 — DOS solves ~-9.37%.
	conv := postAmort(t, `{"loanDate":"2020-01-01","firstDate":"2020-02-01","perYr":12,
		"amount":100000,"nPeriods":120,"payment":500}`)
	if conv.Error != "" {
		t.Errorf("converged negative-rate solve must NOT error (DOS solves -9.37%%); got Error=%q", conv.Error)
	}
	if conv.Rate >= 0 {
		t.Errorf("converged solve: want a negative rate (~-0.094); got %v", conv.Rate)
	}
	if len(conv.Schedule) == 0 {
		t.Errorf("converged negative-rate solve must return a schedule")
	}
}
