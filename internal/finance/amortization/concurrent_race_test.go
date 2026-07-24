package amortization

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// TestConcurrentBackwardSolveNoRace guards the web server's concurrent model: it
// serves one goroutine per request, so any package-level mutable state the engine
// touches during a calculation is shared across in-flight requests. This test runs
// a backward rate solve (which used to flip a package-global `inBackwardSolve`
// flag) concurrently with fancy forward calcs (which read that flag in
// dosPortCanHandle to choose the engine). Run under `-race` it detects the
// read/write data race; run plain it asserts every concurrent forward result
// equals the serial baseline, so a routing flip caused by cross-request
// interference would also surface as a wrong number.
//
// Before the fix (global var inBackwardSolve) `go test -race` flagged this.
// After threading the flag onto LoanInput it is race-free.
func TestConcurrentBackwardSolveNoRace(t *testing.T) {
	d := func(y, m, dd int) types.DateRec { return types.NewDateRec(y, time.Month(m), dd) }

	// A fancy loan whose Amortize reaches dosPortCanHandle (Fancy=true, amount+rate
	// +term known, a known-amount balloon). dosPortCanHandle reads the flag first.
	fancyIn := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanDateStatus: types.InOutInput, LoanDate: d(2024, 1, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.08,
			FirstStatus: types.InOutInput, FirstDate: d(2024, 2, 1),
			NStatus: types.InOutInput, NPeriods: 120,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 1300,
		},
		Settings: Settings{Basis: types.Basis360, PerYr: 12, PlusRegular: true, YrDays: 360, YrInv: 1.0 / 360},
		Fancy:    true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: d(2029, 1, 1),
			AmountStatus: types.InOutInput, Amount: 20000}},
	}

	// A rate-solve input (rate blank, payment given) → SolveRate sets the flag.
	solveIn := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanDateStatus: types.InOutInput, LoanDate: d(2024, 1, 1),
			LoanRateStatus: types.InOutEmpty,
			FirstStatus:    types.InOutInput, FirstDate: d(2024, 2, 1),
			NStatus: types.InOutInput, NPeriods: 120,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus: types.InOutInput, PayAmt: 1213.28,
		},
		Settings: Settings{Basis: types.Basis360, PerYr: 12, PlusRegular: true, YrDays: 360, YrInv: 1.0 / 360},
	}

	// Each goroutine works on its OWN deep copy of the input — this models the web
	// server, where every request builds a fresh LoanInput. (Amortize mutates its
	// input's option slices in place, e.g. writing a solved balloon amount back, so
	// sharing one input across goroutines would race on the slice — a test artifact,
	// not a production condition. Isolating that leaves the genuine cross-request
	// race: the package-global inBackwardSolve flag.)
	deepCopy := func(in LoanInput) LoanInput {
		out := in
		out.Balloons = append([]BalloonPayment(nil), in.Balloons...)
		out.Adjustments = append([]RateAdjustment(nil), in.Adjustments...)
		out.Prepayments = append([]Prepayment(nil), in.Prepayments...)
		return out
	}

	// Serial baseline.
	base := Amortize(deepCopy(fancyIn))
	if base.Err != nil {
		t.Fatalf("baseline Amortize: %v", base.Err)
	}

	const workers, iters = 16, 40
	var wg sync.WaitGroup
	errs := make(chan string, workers*iters)
	for w := 0; w < workers; w++ {
		wg.Add(2)
		// Backward-solve goroutines: continually flip the flag.
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				if _, _, err := SolveRate(deepCopy(solveIn)); err != nil {
					errs <- "SolveRate: " + err.Error()
					return
				}
			}
		}()
		// Forward fancy goroutines: read the flag in dosPortCanHandle; must match base.
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				res := Amortize(deepCopy(fancyIn))
				if res.Err != nil {
					errs <- "Amortize: " + res.Err.Error()
					return
				}
				if diff := res.TotalInt - base.TotalInt; diff > 1e-6 || diff < -1e-6 {
					errs <- "TotalInt drifted under concurrency: got " +
						strconv.FormatFloat(res.TotalInt, 'f', 4, 64) + " want " +
						strconv.FormatFloat(base.TotalInt, 'f', 4, 64)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}
