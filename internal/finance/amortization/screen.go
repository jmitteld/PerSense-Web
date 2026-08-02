package amortization

import (
	"errors"
	"fmt"

	"github.com/persense/persense-port/internal/types"
)

// screen.go — THE ONE PLACE A BLANK CELL IS SOLVED AND A BACKWARD SOLVE IS
// JUDGED. R1, docs/harness_policy.md.
//
// WHY THIS FILE EXISTS. Every fidelity harness in this project re-implemented the
// product's screen pipeline instead of driving it, and seven defects came out of
// the gap between the two. §58 (2026-08-02) is the one that forced the issue:
// `handlers.go` does solve -> CHECK THE CONVERGENCE FLAG -> amortize, and
// `dos_fuzzer5_test.go` reassembled that sequence from parts and dropped the
// middle step, so it amortized at rates the product refuses to display and scored
// the resulting tables as port output. Round 13's paired-regression NEW=1 sat in
// the residual for three rounds on the strength of it, and was twice proposed for
// acceptance as a genuine known departure.
//
// The rule that follows: the decision lives HERE, and both the product and the
// harness call it. A future change to DOS's refusal semantics lands in one place
// and cannot drift between them.

// ErrDidNotConverge is DOS's own MessageBox text (AMORTOP.pas:1489), returned
// when Iterate exhausts its 20 passes with bestp over BOTH halfpenny and
// acc_limit*init. DOS sets Iterate := false, which raises errorflag, and
// MakeTable's `if (errorflag) then exit` draws NO TABLE — a non-converged
// backward solve ENDS THE SCREEN exactly as a refusal does.
var ErrDidNotConverge = errors.New(
	"Computation of payment amount or interest rate did not converge.")

// BlankCellError wraps a solver's own refusal so a caller can name the cell in
// its message without re-deriving which solver failed.
type BlankCellError struct {
	Cell string // "Amount Borrowed" or "Rate"
	Err  error
}

func (e *BlankCellError) Error() string {
	return e.Cell + " is blank and could not be solved (" + e.Err.Error() + ")"
}

func (e *BlankCellError) Unwrap() error { return e.Err }

// PrepareSolverInput returns the throwaway copy the backward solvers need: the
// term and first-payment date derived by FirstPass, with the statuses of any
// field FirstPass DERIVED promoted so the solver guards (CanComputeRate /
// CanComputeLoanAmount) count them as known.
//
// FirstPass marks derived fields InOutOutput; the guards require InOutDefault or
// higher. A field FirstPass could not derive keeps StatusEmpty and still —
// correctly — fails the guard.
//
// The copy is deliberate: the caller's input is left untouched for the Amortize
// call afterwards, which runs its own FirstPass.
func PrepareSolverInput(in LoanInput) (LoanInput, error) {
	solverInput := in
	solverLoan := in.Loan
	if err := FirstPass(&solverLoan); err != nil {
		return in, err
	}
	if solverLoan.FirstStatus > types.StatusEmpty &&
		solverLoan.FirstStatus < types.InOutDefault {
		solverLoan.FirstStatus = types.InOutDefault
	}
	if solverLoan.NPeriods > 0 && solverLoan.NStatus > types.StatusEmpty &&
		solverLoan.NStatus < types.InOutDefault {
		solverLoan.NStatus = types.InOutDefault
	}
	solverInput.Loan = solverLoan
	return solverInput, nil
}

// SolveBlankCells solves whichever of {Amount, Rate} the caller reports blank and
// applies DOS's convergence gate, returning the input with the solved cells
// written back as InOutInput fields so the schedule engine runs normally.
//
// This is the PRODUCTION entry point: it derives its own solver input via
// PrepareSolverInput. A harness that constructs a fully-specified screen itself
// should call SolveBlankCellsPrepared instead, so that it shares the GATE without
// inheriting a derivation it did not ask for.
func SolveBlankCells(in LoanInput, solveAmount, solveRate bool) (LoanInput, error) {
	if !solveAmount && !solveRate {
		return in, nil
	}
	solverInput, err := PrepareSolverInput(in)
	if err != nil {
		return in, err
	}
	return SolveBlankCellsPrepared(in, solverInput, solveAmount, solveRate)
}

// SolveBlankCellsPrepared is SolveBlankCells with the solver input supplied by
// the caller.
//
// ORDER IS LOAD-BEARING and mirrors handlers.go's original block exactly: both
// solves run against the SAME prepared input (the amount solve does not re-derive
// for the rate solve), a solver's own error returns immediately and names its
// cell, and the convergence gate is applied ONCE, AFTER both — so a converged
// amount followed by a non-converged rate reports DOS's generic non-convergence
// message rather than an amount-specific one.
func SolveBlankCellsPrepared(in, solverInput LoanInput, solveAmount, solveRate bool) (LoanInput, error) {
	// Default true: a cell the caller did not ask to solve is trivially converged.
	amountConverged, rateConverged := true, true
	out := in

	if solveAmount {
		solved, conv, err := SolveLoanAmount(solverInput)
		if err != nil {
			return in, &BlankCellError{Cell: "Amount Borrowed", Err: err}
		}
		out.Loan.AmountStatus = types.InOutInput
		out.Loan.Amount = solved
		// DOS sets `h^.amountstatus := outp` right here (Amortize.pas:1377), which
		// closes the TackOnFinalBalloon gate at :1386. The status itself has to
		// stay InOutInput for the rest of the pipeline, so the fact travels on the
		// input — see types.go, AmountWasSolved.
		out.AmountWasSolved = true
		amountConverged = conv
	}

	if solveRate {
		solved, conv, err := SolveRate(solverInput)
		if err != nil {
			return in, &BlankCellError{Cell: "Rate", Err: err}
		}
		out.Loan.LoanRateStatus = types.InOutInput
		out.Loan.LoanRate = solved
		out.RateWasSolved = true // Amortize.pas:1377's rate arm; see above
		rateConverged = conv
	}

	// THE GATE §58 WAS ABOUT. DOS blocks here — no schedule, not a best estimate.
	if !amountConverged || !rateConverged {
		return in, ErrDidNotConverge
	}
	return out, nil
}

// RunScreen is the whole product pipeline for one screen: solve the blank cells,
// apply DOS's gate, and draw the table. A harness that wants to know "what would
// the user see for this screen?" must call THIS, not reassemble it.
func RunScreen(in LoanInput, solveAmount, solveRate bool) (AmortResult, error) {
	solved, err := SolveBlankCells(in, solveAmount, solveRate)
	if err != nil {
		return AmortResult{}, err
	}
	r := Amortize(solved)
	if r.Err != nil {
		return r, fmt.Errorf("%w", r.Err)
	}
	return r, nil
}
