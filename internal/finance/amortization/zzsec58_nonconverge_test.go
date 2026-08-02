package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Discrepancies §58 (round 16, 2026-08-02) — A NON-CONVERGED BACKWARD SOLVE MUST
// END THE SCREEN, AND THE FUZZER MUST HONOUR THAT.
//
// This closes round 13's paired-regression NEW=1, which had been open three
// rounds and was twice recommended for acceptance as a benign advisory. It was
// neither benign nor an engine defect: it was the SEVENTH bug in the harness
// family, and the engine was faithful the whole time.
//
// WHAT WAS WRONG. SolveRate / SolveLoanAmount report failure two ways — a non-nil
// `err` (a DOS-faithful screen refusal) and a `converged=false` bool returned
// ALONGSIDE a nil err, which is DOS's Iterate exhausting its 20 passes with bestp
// over both halfpenny and acc_limit*init (AMORTOP.pas:1485-1492). DOS treats the
// two identically: MessageBox, Iterate:=false, errorflag, and MakeTable's
// `if (errorflag) then exit` draws NO TABLE. The shipped port matches
// (handlers.go:1260 returns DOS's message and never calls Amortize).
// `dos_fuzzer5_test.go` discarded the bool (`v, _, err :=`), amortized at a rate
// the product refuses to display, and scored the resulting table as "Go produced
// a schedule".
//
// THE SCREEN, from round 13's NEW=1 line:
//
//	amort_oracle 291207.99 0.1209560000 <n> 24 exact prepaid \
//	  loandmy=29.5.2024 firstdmy=29.7.2024 pts=0.009110 payhard=1962.94 norate
//
// This is a near-perpetuity rate solve: pay*perYr/amount = 0.16177633 is the
// interest-only rate, and the true root approaches it from below as n grows, so
// the terminal balance becomes astronomically stiff in the rate. DOS answers
// through n=2136 and refuses from n=2160 on.
//
// The four independent components, per the standing rule that each be separately
// revertible with a distinct failure signature:
//
//	A. rate fidelity where DOS ANSWERS — 10 dp, 17 values of n
//	B. the refusal signal where DOS REFUSES — converged=false, nil err
//	C. the refusal must be MONOTONE — no answer past the first refusal
//	D. the rendered table at an unconverged rate is garbage, which is WHY
//	   B is load-bearing rather than cosmetic
func TestSec58RateSolveMatchesDOSAndRefusesWhereDOSRefuses(t *testing.T) {
	mk := func(n int) LoanInput {
		return LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 291207.99,
				LoanRateStatus: types.StatusEmpty,
				NStatus:        types.InOutInput, NPeriods: n,
				PerYrStatus: types.InOutInput, PerYr: 24,
				PayAmtStatus: types.InOutInput, PayAmt: 1962.94,
				PointsStatus:   types.InOutInput,
				Points:         0.009110,
				LoanDateStatus: types.InOutInput,
				LoanDate:       types.NewDateRec(2024, time.May, 29),
				FirstStatus:    types.InOutInput,
				FirstDate:      types.NewDateRec(2024, time.July, 29),
			},
			Settings: Settings{Basis: types.Basis360, PerYr: 24,
				YrDays: 360, YrInv: 1.0 / 360, Exact: true, Prepaid: true},
		}
	}

	// PROVENANCE: every value below is `solvedrate` from
	//   /tmp/oraclebuild/amort_oracle 291207.99 0.1209560000 <n> 24 exact prepaid \
	//     loandmy=29.5.2024 firstdmy=29.7.2024 pts=0.009110 payhard=1962.94 norate
	// run 2026-08-02. A rate of 0 marks the screens where that command prints
	// "ERR Computation of payment amount or interest rate did not converge."
	answers := []struct {
		n    int
		rate float64 // 0 => DOS REFUSES
	}{
		{240, 0.1050801433},
		{480, 0.1543144676},
		{720, 0.1604412462},
		{960, 0.1615178293},
		{1200, 0.1617251767},
		{1440, 0.1617661490},
		{1680, 0.1617743009},
		{1920, 0.1617759257},
		{1944, 0.1617759860},
		{1968, 0.1617760373},
		{1992, 0.1617760809},
		{2016, 0.1617761181},
		{2040, 0.1617761497},
		{2064, 0.1617761766},
		{2088, 0.1617761995},
		{2112, 0.1617762190},
		{2136, 0.1617762356},
		{2160, 0}, // first refusal
		{2400, 0},
		{2688, 0}, // round 13's NEW=1 case
	}

	sawRefusal := false
	for _, a := range answers {
		v, conv, err := SolveRate(mk(a.n))

		if a.rate != 0 {
			// COMPONENT A — DOS answers, so the port must answer the same rate.
			if err != nil {
				t.Errorf("n=%d: SolveRate refused a screen DOS answers: %v", a.n, err)
				continue
			}
			if !conv {
				t.Errorf("n=%d: SolveRate reported non-convergence where DOS converges "+
					"(DOS solvedrate %.10f)", a.n, a.rate)
				continue
			}
			if math.Abs(v-a.rate) > 5e-10 {
				t.Errorf("n=%d: solved rate = %.10f, DOS %.10f (delta %.3e)",
					a.n, v, a.rate, v-a.rate)
			}
			// COMPONENT C — a refusal must not be followed by an answer. DOS's
			// Iterate gets monotonically stiffer in n on this screen; an answer
			// past the first refusal would mean the port re-acquired a root DOS
			// lost, which on this family is a spurious root, not a better solver.
			if sawRefusal {
				t.Errorf("n=%d: port answered AFTER the first DOS refusal — "+
					"non-monotone refusal boundary", a.n)
			}
			continue
		}

		// COMPONENT B — DOS refuses, so the port must NOT report a usable answer.
		// Either signal is acceptable (both block the table at handlers.go:1260);
		// what must never happen is err==nil AND conv==true.
		sawRefusal = true
		if err == nil && conv {
			t.Errorf("n=%d: SolveRate returned CONVERGED rate %.10f on a screen DOS "+
				"refuses — the port would draw a table DOS does not", a.n, v)
		}
	}
}

// COMPONENT D — why component B is load-bearing.
//
// At n=2688 the closed-form pre-solve's unrefined estimate (0.1627399644) sits
// 9.6e-4 ABOVE the interest-only perpetuity rate, so it is not a root at all: the
// loan negatively amortizes for 112 years. Rendering it produces a $21.4 BILLION
// terminal balance on a $291K loan. SolveRate correctly returns converged=false
// here; this test pins what that flag is protecting the user from, so that a
// future change which "helpfully" promotes the estimate to converged fails loudly
// rather than shipping this table.
//
// PROVENANCE: the port's own dosIterateRate trace (DPTRACE=1) at n=2688 ends
// bestp=0.0063180707 against acc_limit*init = 2e-8 * 291207.99 = 0.0058241598 —
// over tolerance, hence non-converged, exactly as DOS reports.
func TestSec58UnconvergedRateWouldRenderAbsurdTable(t *testing.T) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 291207.99,
			LoanRateStatus: types.StatusEmpty,
			NStatus:        types.InOutInput, NPeriods: 2688,
			PerYrStatus: types.InOutInput, PerYr: 24,
			PayAmtStatus: types.InOutInput, PayAmt: 1962.94,
			PointsStatus:   types.InOutInput,
			Points:         0.009110,
			LoanDateStatus: types.InOutInput,
			LoanDate:       types.NewDateRec(2024, time.May, 29),
			FirstStatus:    types.InOutInput,
			FirstDate:      types.NewDateRec(2024, time.July, 29),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: 24,
			YrDays: 360, YrInv: 1.0 / 360, Exact: true, Prepaid: true},
	}

	v, conv, err := SolveRate(in)
	if err != nil {
		return // refused outright: the table can never be reached
	}
	if conv {
		t.Fatalf("SolveRate claims convergence at n=2688 (rate %.10f); "+
			"component B covers the regression, this test covers its consequence", v)
	}

	perpetuity := 1962.94 / 291207.99 * 24
	if v <= perpetuity {
		t.Errorf("unrefined estimate %.10f is at or below the interest-only rate "+
			"%.10f — the premise of this test (that it is not a root) no longer holds",
			v, perpetuity)
	}

	// Render it anyway, the way the fuzzer used to, and show it is absurd.
	in.Loan.LoanRateStatus, in.Loan.LoanRate = types.InOutInput, v
	r := Amortize(in)
	if r.Err != nil {
		return
	}
	bal := r.Schedule[len(r.Schedule)-1].Principal
	if bal < 1e9 {
		t.Errorf("terminal balance %.2f — expected a runaway (>$1e9) negative "+
			"amortization at a non-root rate; the premise of §58 has changed", bal)
	}
}
