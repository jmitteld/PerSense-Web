package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// zzao9_prepay_solve_fixture_test.go — AO9, the unknown periodic-prepayment
// amount (DOS EstimateAndRefinePeriodicPrepayment, Amortize.pas:665-707), PINNED
// against the real DOS engine in BOTH plus_regular modes and with the option
// axes that used to break it.
//
// WHY THIS FILE EXISTS (2026-08-07). Nate reported that the Amortization screen
// "cannot solve for the additional periodic payment amount while DOS can". Three
// separate defects were behind it, and every one of them was invisible to the
// suite:
//
//  1. The piecewise engine solved the amount and then never wrote it to
//     AmortResult.SolvedPrepay — the field was assigned in exactly ONE place in
//     the tree, dosport_entry.go:1192, the OTHER engine. The API gates its JSON
//     on that field and the UI gates the grid cell on the JSON, so the cell
//     stayed blank on a screen whose schedule was right to the cent.
//  2. With PlusRegular ON the solver returned DOS's closed-form FIRST GUESS raw,
//     with no Iterate refinement. Exact on a plain arrears schedule (where the
//     guess is an identity and DOS's own Iterate exits at its zeroth probe);
//     wrong by up to 18% once a moratorium, skip-months, in-advance or exact
//     interest broke the identity.
//  3. With PlusRegular OFF it ran a bare 40-pass secant against the CENT-QUANTISED
//     display residual, with no convergence verdict at all, and returned its last
//     iterate whatever it was.
//
// And the one instrument that should have caught all three —
// TestDOSSolverOptionsAudit — forced `plusreg := true` for every prepayment
// solver AND skipped any case whose SolvedPrepay was 0, which is every piecewise
// case by defect (1). Opening both halves took its AO9 population from 28 to 138
// adjudicated cases and its value divergences from 0 to 25 before the fix, and to
// 0 after it.
//
// ⚠️ THESE ARE ORACLE CONSTANTS, NOT EXPECTATIONS. Every `want` below is what
// `/tmp/oraclebuild/amort_oracle` printed for the command line in its own
// comment. Re-derive them, do not adjust them. If one of these moves, the port
// moved — go read the oracle before touching the number (rule 6, provenance;
// rule 10, an internal-consistency test never drives a behaviour change).
//
// NEGATIVE DIRECTION — verified in fact on 2026-08-07, one reverted term at a
// time in three separate probe trees (rule 3, R38: more than one control, each
// varying the axis its guard is about):
//
//	probe "noecho"    drop `result.SolvedPrepay = prepaySolvedAmt` (engine.go)
//	                  -> audit both-solved 826 -> 734, DOS-solves-Go-refuses 0 -> 92
//	probe "rawguess"  restore `if PlusRegular { return prepayClosedFormGuess(...) }`
//	                  -> audit value-diverge 0 -> 25, worst rel 3.70e+00
//	                  -> caseMoratoriumAdditive below returns 2637.5298 (want 3207.1632)
//	probe "centterm"  restore generateFancyScheduleMode as the Iterate terminal
//	                  -> audit DOS-solves-Go-refuses 0 -> 11
//	                  -> caseSkipExactAdditive below REFUSES (ErrDidNotConverge)

// ao9Case is one pinned screen. `oracle` is the exact command line, so the
// fixture can be re-derived by hand without reading the constructor.
type ao9Case struct {
	name   string
	oracle string
	build  func() LoanInput
	want   float64
}

// tol is a cent. The oracle prints 4 decimals; the engine is expected to land on
// DOS's answer, not near it. The audit sweep's own floor is max(0.02, 5e-4·|v|),
// which is looser — deliberately, because it scores thousands of random screens;
// a pinned fixture should be tighter than the sweep that found it.
const ao9Tol = 0.01

func ao9Cases() []ao9Case {
	// The whole-dollar screen Nate reported, in both modes. gzLoanInput anchors
	// the loan at 1 Jan 2024, so the fixtures below use that anchor and the
	// oracle lines carry no loandmy=/firstdmy= override — the shapes match, and
	// the arithmetic does not depend on the calendar year.
	base := func(s Settings, pay float64, startM, nn, perYr int) LoanInput {
		in := gzLoanInput(100000, 0.07, 46, 12, s)
		in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, pay
		in.Fancy = true
		in.Prepayments = []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2024+startM/12, time.Month(startM%12+1), 1),
			NNStatus:        types.InOutInput, NN: nn,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PaymentStatus: types.StatusEmpty}}
		return in
	}
	rep := gzSettings(12, types.Basis360, false, false, false, false, false)
	add := gzSettings(12, types.Basis360, false, false, false, false, false)
	add.PlusRegular = true

	return []ao9Case{
		{
			// THE REPORTED SCREEN. plus_regular=false is DOS's FACTORY default
			// (PEDATA.pas:68) and the web's "Balloon / prepayment includes regular
			// pmt = YES". This is the mode that routes to the PIECEWISE engine via
			// the router clause replace_mode_with_extras — i.e. the one that used
			// to drop the answer.
			name:   "reported/replace",
			oracle: "amort_oracle 100000 0.07 46 12 payhard=800 presolve=13:108:12",
			build:  func() LoanInput { return base(rep, 800, 13, 108, 12) },
			want:   1217.0476,
		},
		{
			name:   "reported/additive",
			oracle: "amort_oracle 100000 0.07 46 12 payhard=800 presolve=13:108:12 plusreg",
			build:  func() LoanInput { return base(add, 800, 13, 108, 12) },
			want:   909.2973,
		},
		{
			// DEFECT (2)'s witness: additive + moratorium + in-advance + prepaid.
			// The closed form is not an identity here, so the raw guess was 17.8%
			// low. Drawn by the opened audit sweep, then pinned.
			name: "caseMoratoriumAdditive",
			oracle: "amort_oracle 321556.83 0.0791700000 72 12 prepaid inadv plusreg " +
				"mor=16 payhard=3690.97 presolve=3:50:12",
			build: func() LoanInput {
				s := gzSettings(12, types.Basis360, false, true, true, false, false)
				s.PlusRegular = true
				in := gzLoanInput(321556.83, 0.07917, 72, 12, s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, 3690.97
				in.Fancy = true
				in.Moratorium = Moratorium{
					FirstRepayStatus: types.InOutInput,
					FirstRepay:       types.NewDateRec(2025, time.May, 1)} // 16 months after 1 Jan 2024
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.April, 1),
					NNStatus: types.InOutInput, NN: 50,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.StatusEmpty}}
				return in
			},
			want: 3207.1632,
		},
		{
			// DEFECT (3)'s witness, and a NEGATIVE solved amount — an over-funded
			// loan. DOS's Iterate admits one; any clamp at zero fails this case.
			// Against the cent-quantised terminal the Iterate could not reach
			// halfpenny here and the screen REFUSED.
			name: "caseSkipExactAdditive",
			oracle: "amort_oracle 188771.32 0.1123020000 96 12 b365_360 exact prepaid plusreg " +
				"skip=2-3 payhard=3876.36 presolve=28:15:12",
			build: func() LoanInput {
				s := gzSettings(12, types.Basis365360, true, true, false, false, false)
				s.PlusRegular = true
				in := gzLoanInput(188771.32, 0.112302, 96, 12, s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, 3876.36
				in.Fancy = true
				ms, _ := MonthSetFromString("2-3")
				in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: "2-3", MonthSet: ms}
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2026, time.May, 1),
					NNStatus: types.InOutInput, NN: 15,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.StatusEmpty}}
				return in
			},
			want: -1130.5359,
		},
		{
			// REPLACE mode with a balloon, on the 365/360 basis — the other half
			// of the mode axis with an option in play.
			name: "caseBalloonReplace",
			oracle: "amort_oracle 223500.80 0.1221430000 156 12 b365_360 b37=57583.14 " +
				"payhard=1375.37 presolve=31:105:12",
			build: func() LoanInput {
				s := gzSettings(12, types.Basis365360, false, false, false, false, false)
				in := gzLoanInput(223500.80, 0.122143, 156, 12, s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, 1375.37
				in.Fancy = true
				in.Balloons = []BalloonPayment{{
					DateStatus: types.InOutInput, Date: types.NewDateRec(2027, time.February, 1),
					AmountStatus: types.InOutInput, Amount: 57583.14}}
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2026, time.August, 1),
					NNStatus: types.InOutInput, NN: 105,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.StatusEmpty}}
				return in
			},
			want: 3033.5240,
		},
		{
			// MU1's KILLER — added 2026-08-07 after the adversarial review found
			// that NOTHING guarded `clone.entireWalk = true`, the single line the
			// solver's longest comment block defends: deleting it left the audit
			// summary BIT-IDENTICAL, because optPool["preamt"] drew no adjustment.
			// This is a SET-BOTH adjustment (new rate AND new payment) — the case
			// the walk must re-amortize at — and it is also the case where the
			// single-terminal Iterate was sign-flipped (DOS 8114.5449, port
			// -13747.2002) before probe0 was split off from the loop probes.
			// R38: the control must vary the axis the guard is about.
			name: "caseSetBothAdjustment",
			oracle: "amort_oracle 139572.77 0.1148320000 60 12 prepaid plusreg " +
				"adj=2:0.088267:4424.23 payhard=2212.57 presolve=21:6:12",
			build: func() LoanInput {
				s := gzSettings(12, types.Basis360, false, true, false, false, false)
				s.PlusRegular = true
				in := gzLoanInput(139572.77, 0.114832, 60, 12, s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, 2212.57
				in.Fancy = true
				in.Adjustments = []RateAdjustment{{
					DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.March, 1),
					LoanRateStatus: types.InOutInput, LoanRate: 0.088267,
					AmountStatus: types.InOutInput, Amount: 4424.23, AmtOK: true}}
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2025, time.October, 1),
					NNStatus: types.InOutInput, NN: 6,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.StatusEmpty}}
				return in
			},
			want: 8114.5449,
		},
		{
			// A SOLVED EXACT ZERO, and a ZERO RATE — two dead spots in one screen.
			// `adjp = 120000 - 120*1000 = 0`, so prepayClosedFormGuess's TINY-RATE
			// branch (Amortize.pas:675-682) returns exactly 0 and dosIterateCore's
			// `if seed == 0` guard used to refuse outright. DOS does not: its
			// Iterate evaluates the terminal FIRST and takes the half-penny early
			// exit with x untouched (AMORTOP.pas:1437-1444). Kills MU4 (the
			// tiny-rate branch was previously unexercised by anything) and pins the
			// zero-seed path.
			//
			// ⚠️ THIS FIXTURE IS ALSO A STANDING REMINDER: a solved exact zero is
			// still indistinguishable from "no solve" once it reaches
			// AmortResult.SolvedPrepay and handlers.go — that needs a bool, and it
			// is filed, not fixed. Do not "fix" this test by asserting a refusal.
			name:   "caseZeroRateSolvedZero",
			oracle: "amort_oracle 120000 0.0 120 12 plusreg payhard=1000 presolve=6:12:12",
			build: func() LoanInput {
				s := gzSettings(12, types.Basis360, false, false, false, false, false)
				s.PlusRegular = true
				in := gzLoanInput(120000, 0, 120, 12, s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, 1000
				in.Fancy = true
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.July, 1),
					NNStatus: types.InOutInput, NN: 12,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.StatusEmpty}}
				return in
			},
			want: 0,
		},
		{
			// A PRINCIPAL-MINIMUM (target) screen. The adversarial review found the
			// solver REFUSED this one when called directly, because the direct call
			// left dosLastOK false while Amortize sets it true — the same input,
			// two answers, depending on the caller. SolvePrepaymentAmount now sets
			// dosLastOK itself (DOS's h^.lastok is TRUE on every path that reaches
			// this routine, Amortize.pas:1350-1355), so this fixture pins that the
			// solver is caller-independent.
			name:   "caseTargetDirectCall",
			oracle: "amort_oracle 197452.12 0.1091900000 96 12 targ=1058.23 payhard=2128.43 presolve=19:28:12",
			build: func() LoanInput {
				s := gzSettings(12, types.Basis360, false, false, false, false, false)
				in := gzLoanInput(197452.12, 0.10919, 96, 12, s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, 2128.43
				in.Fancy = true
				in.Target = Target{TargetStatus: types.InOutInput, TargetValue: 1058.23}
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2025, time.August, 1),
					NNStatus: types.InOutInput, NN: 28,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.StatusEmpty}}
				return in
			},
			want: 4561.1484,
		},
		{
			// THE OTHER HALF OF THE PROBE SPLIT. caseSetBothAdjustment pins that the
			// ZEROTH probe must be til_adj; this one pins that the LOOP probes must
			// be `entire`. Mutation testing on 2026-08-07 showed the suite passing
			// with BOTH probes non-entire — a guard that only fires in one direction
			// is half a guard (R38, and R16's "seen to fail" in both directions).
			//
			// REPLACE mode with a set-both adjustment. With the loop probes forced
			// non-entire the solver returns 7684.5463 against DOS's 5884.0919 —
			// 31% high, because the walk stops re-amortizing at the 2029 adjustment
			// and the residual it roots is a different loan's.
			name: "caseSetBothAdjustmentReplace",
			oracle: "amort_oracle 279165.62 0.0878690000 96 12 " +
				"adj=68:0.057269:4688.19 payhard=2876.07 presolve=7:19:12",
			build: func() LoanInput {
				s := gzSettings(12, types.Basis360, false, false, false, false, false)
				in := gzLoanInput(279165.62, 0.087869, 96, 12, s)
				in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, 2876.07
				in.Fancy = true
				in.Adjustments = []RateAdjustment{{
					DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.September, 1),
					LoanRateStatus: types.InOutInput, LoanRate: 0.057269,
					AmountStatus: types.InOutInput, Amount: 4688.19, AmtOK: true}}
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.August, 1),
					NNStatus: types.InOutInput, NN: 19,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.StatusEmpty}}
				return in
			},
			want: 5884.0919,
		},
	}
}

// ⚠️ KNOWN-UNPINNED, 2026-08-07, recorded rather than faked. Mutation testing
// found two mutants this file does NOT kill, and no honest fixture was available
// for either within the round:
//
//   - DELETING THE `!ok` REFUSAL ARM in SolvePrepaymentAmount. Nothing in the repo
//     asserts that a non-converged AO9 solve refuses the screen; the audit's
//     over-refusal bucket is a t.Logf, not a t.Errorf. Needs a screen where DOS
//     itself refuses the prepay solve — none was found in 6,000 audit draws.
//   - PASSING `seed` INSTEAD OF `input.Loan.Amount` as Iterate's `accInit`. The
//     relative acc_limit arm (bestp <= 2e-8*init) only decides a case when bestp
//     lands BETWEEN the two tolerances, which no drawn screen did.
//
// Both are transcription claims with a Pascal citation and no executable guard.
// R35: a documented trap is not a guard — these are on the backlog, not closed.

// TestAO9SolverIsCallerIndependent is the adversarial review's finding turned
// into a guard: SolvePrepaymentAmount called DIRECTLY must return what it returns
// when reached through Amortize. Before 2026-08-07 it did not — `dosLastOK` is
// read only under `entireWalk` (engine.go:5042, :5811), engine.go:380 sets it and
// a direct call left it false, so the same screen produced 7959.677292 one way
// and -13747.200219 the other.
//
// ⚠️ This is what TestAO9PrepaySolveVsDOS's own header USED to claim it did.
// It didn't: every one of its fixtures happened to be insensitive to the flag, so
// the file "pinned the solver" in a configuration the product never builds.
func TestAO9SolverIsCallerIndependent(t *testing.T) {
	for _, c := range ao9Cases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			direct, dErr := SolvePrepaymentAmount(c.build(), 0)
			r := Amortize(c.build())
			if (dErr != nil) != (r.Err != nil) {
				t.Fatalf("direct call and Amortize disagree about REFUSAL: direct err=%v, Amortize err=%v\n  oracle: %s",
					dErr, r.Err, c.oracle)
			}
			if dErr != nil {
				return
			}
			if math.Abs(direct-r.SolvedPrepay) > ao9Tol {
				t.Errorf("caller-dependent answer: direct %.6f vs Amortize %.6f\n  oracle: %s",
					direct, r.SolvedPrepay, c.oracle)
			}
		})
	}
}

// TestAO9PrepaySolveVsDOS pins the SOLVER — SolvePrepaymentAmount called
// directly, so a failure names the solver and not the screen pipeline around it.
func TestAO9PrepaySolveVsDOS(t *testing.T) {
	for _, c := range ao9Cases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got, err := SolvePrepaymentAmount(c.build(), 0)
			if err != nil {
				t.Fatalf("solver refused a screen DOS answers: %v\n  oracle: %s\n  DOS: %.4f",
					err, c.oracle, c.want)
			}
			if math.Abs(got-c.want) > ao9Tol {
				t.Errorf("AO9 solve: got %.6f, DOS %.4f (delta %.6f)\n  oracle: %s",
					got, c.want, got-c.want, c.oracle)
			}
		})
	}
}

// TestAO9SolvedPrepayIsTransported is defect (1)'s own guard, and it is
// deliberately separate from the value test above: the value can be perfect and
// the product still show a blank cell, which is exactly what Nate saw. It asserts
// the value reaches AmortResult through the WHOLE engine — including the
// wholesale `result = generate...Schedule(...)` reassignments that silently
// clobbered the first attempt at this fix (engine.go:1046/1074/1167/1170/1687).
//
// It runs BOTH modes on purpose. The additive screen routes to `dosport` and the
// replace screen to `piecewise` (router clause replace_mode_with_extras), so one
// case alone would pass on either engine's plumbing while the other stayed
// broken — which was the pre-2026-08-07 state exactly.
func TestAO9SolvedPrepayIsTransported(t *testing.T) {
	for _, c := range ao9Cases()[:2] {
		c := c
		t.Run(c.name, func(t *testing.T) {
			r := Amortize(c.build())
			if r.Err != nil {
				t.Fatalf("Amortize refused: %v\n  oracle: %s", r.Err, c.oracle)
			}
			if r.SolvedPrepay == 0 {
				t.Fatalf("AmortResult.SolvedPrepay is 0 — the solved amount did not "+
					"reach the result, so the API omits `solvedPrepay` and the UI "+
					"leaves the Amount cell blank.\n  oracle: %s\n  DOS: %.4f",
					c.oracle, c.want)
			}
			if math.Abs(r.SolvedPrepay-c.want) > ao9Tol {
				t.Errorf("transported SolvedPrepay %.6f, DOS %.4f\n  oracle: %s",
					r.SolvedPrepay, c.want, c.oracle)
			}
		})
	}
}
