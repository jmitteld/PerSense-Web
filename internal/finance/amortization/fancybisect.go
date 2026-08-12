package amortization

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// Fancy backward solving via DOS's single Newton refinement (AMORTOP.pas:1415
// `Iterate`), ported here as dosIterate + the per-unknown terminals below.
//
// DOS solves an amount, rate, or payment — under balloons, prepayments, rate
// adjustments, moratoria, and exact interest — with ONE finite-difference secant
// that drives the schedule's *unforced* terminal balance to zero, differing only
// in which `var x` it perturbs. That is exactly what the dosIterate* family does:
// dosIteratePayment / dosIterateSimplePayment (var x = payment), dosIterateAmount
// (var x = h^.amount), dosIterateRate (var x = loanrate). Each evaluates the same
// UNFORCED terminal DOS's Iterate does (RepayFancyLoan / RepayLoan with no forced
// final payment and no early payoff stop), so the solved value matches the DOS
// engine rather than approximating it.
//
// (A prior implementation bisected the over/under-amortization SIGN of the forced
// display schedule. That sign test was a Go invention — DOS has no bisection
// anywhere — so it was removed in favour of this faithful port of Iterate.)

// repayExactTerminal runs the actual-day (exact) schedule for a trial payment x
// over the FULL term and returns the unforced terminal balance — a continuous,
// monotonic function of x (positive ⇒ still owes, negative ⇒ overpaid). It is the
// Go analogue of DOS's RepayFancyLoan used inside Iterate: it does NOT stop early
// or force the final payment, so an overpayment drives the balance negative
// exactly as DOS does (`if p < 0 then p := p - d`, AMORTOP.pas RepayLoan). This
// continuity is what lets the secant in dosIteratePayment converge like DOS;
// reconstructing the residual from the forced/early-stopping display schedule is
// discontinuous and makes the secant misbehave on long terms.
//
// Scope: exact loans (the path dosIteratePayment serves) — ordinary (in-arrears)
// in the main loop, and in-advance (annuity-due) via the settlement-shifted
// early-return branch below.
func repayExactTerminal(input LoanInput, x float64) float64 {
	loan := input.Loan
	s := &input.Settings
	p := loan.Amount
	// See LoanInput.gridAnchorDay — a segment sub-loan anchors on DOS's phantom
	// snapped day, not on the clamped FirstDate's day.
	origDay := input.anchorDayFor(&loan)
	if s.InAdvance {
		// Exact (true-daily) in-advance: DOS shifts the base date one period later
		// (AMORTOP.pas:1159-1177) and amortizes over n-1 rows starting at firstDate
		// + 1 period, each accruing actual-day interest on the shifted period; the
		// time-0 settlement interest is collected at closing and does not change the
		// balance. The continuous (unforced) terminal balance after the n-1 rows is
		// monotone in x — the criterion dosIteratePayment/DOS's Iterate drives to
		// zero. See docs/exact_groundzero_findings.md "Exact × in-advance structure".
		prev := loan.FirstDate
		cur := loan.FirstDate
		for k := 1; k < loan.NPeriods; k++ {
			nd, err := dateutil.AddPeriod(cur, loan.PerYr, origDay, false)
			if err != nil {
				break
			}
			cur = nd
			if p < 0 {
				// Overpaid: DOS subtracts the payment with no further interest.
				p = p - x
			} else {
				yd := periodYearFraction(prev, cur, loan.PerYr, s)
				p = p + loan.LoanRate*yd*p - x
			}
			prev = cur
		}
		return p
	}
	prevDate := loan.LoanDate
	// Prepaid: the settlement stub (loanDate → natural period start) is collected
	// at closing, so the regular schedule's first period is a full period from the
	// natural start, not the odd loanDate→firstDate span. Mirrors the schedule's
	// row-0 stub (engine.go generateFancySchedule prepaid branch).
	if s.Prepaid && !s.InAdvance {
		if naturalStart, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, origDay, true); err == nil &&
			dateutil.DateComp(loan.LoanDate, naturalStart) <= 0 {
			prevDate = naturalStart
		}
	}
	curDate := loan.FirstDate
	for i := 0; i < loan.NPeriods; i++ {
		if p < 0 {
			// Overpaid: DOS subtracts the payment with no further interest.
			p = p - x
		} else {
			yd := periodYearFraction(prevDate, curDate, loan.PerYr, s)
			p = p + loan.LoanRate*yd*p - x
		}
		prevDate = curDate
		nd, err := dateutil.AddPeriod(curDate, loan.PerYr, origDay, false)
		if err != nil {
			break
		}
		curDate = nd
	}
	return p
}

// dosIterate is the general form of DOS's Newton/secant refinement
// (AMORTOP.pas:1415 `Iterate`): it drives the schedule's UNFORCED terminal
// balance to zero by finite-difference secant steps over a single scalar
// unknown, converging when the residual is under half a penny (or after 20
// iterations, keeping the best estimate seen). `terminal(v)` returns that
// balance for the trial value v (>0 under-amortized, <0 over-amortized), and
// `accInit` is DOS's `init` for the relative acceptance clause — always the
// loan amount (the starting balance `p` at entry to Iterate).
//
// DOS's single Iterate solves the payment, the loan amount, and the interest
// rate — it differs only in which `var x` it perturbs (AMORTOP.pas:1437 dispatch
// picks the terminal; :1483 `p:=x` for the amount target). Extracting the secant
// here lets the payment/amount/rate solvers share exactly that one refinement,
// step-for-step (divergence brake, bestx-after-update timing, dual acceptance).
//
// Ported from legacy/src/dos_source/AMORTOP.pas: function Iterate.
func dosIterate(seed, accInit float64, terminal func(float64) float64) (float64, bool) {
	return dosIterateAbort(seed, accInit, terminal, nil)
}

// dosIterateAbort is dosIterate plus DOS's `overflowflag` escape. DOS's Iterate
// re-checks the engine-wide flag at the top of EVERY secant pass
// (AMORTOP.pas:1450-1452):
//
//	repeat
//	  f:=GrowthPerPeriod;
//	  inc(count);
//	  if (overflowflag) then
//	    goto 1;
//
// and `goto 1` jumps PAST both `x := bestx` and the convergence verdict — so the
// refinement stops dead mid-flight and no answer is adopted. The flag is set by
// the guarded math primitives themselves: lnn(x<=0) and sqrrt(x<-teeny) raise
// "Error: The data you have specified contain an inconsistency." and set BOTH
// overflowflag and errorflag (INTSUTIL.pas:1128-1135, 1164-1171), and errorflag
// is the engine-wide abort — the whole screen refuses and no table is produced.
//
// `abort` reports whether that latch has tripped. A nil abort is the plain
// dosIterate: the secants whose terminals cannot reach a guarded primitive.
func dosIterateAbort(seed, accInit float64, terminal func(float64) float64,
	abort func() bool) (float64, bool) {
	return dosIterateCore(seed, accInit, terminal, terminal, abort, false)
}

// dosIterateMixedProbe0Abort is dosIterateAbort for the callers that pass
// `entire` to Iterate — and it exists because DOS'S ZEROTH PROBE IS NOT THE SAME
// FUNCTION AS ITS LOOP PROBES.
//
//	{ AMORTOP.pas:1438-1439, BEFORE the loop }
//	RepayFancyLoan(p, usap, loandate, firstdate, nil, false, til_adj,      no_value_calc, 0)
//	{ AMORTOP.pas:1464-1465, INSIDE the loop }
//	RepayFancyLoan(p, usap, loandate, firstdate, nil, false, entire_or_no, no_value_calc, 0)
//
// `til_adj` is the literal FALSE and `entire` the literal TRUE (AMORTOP.pas:20-21),
// and `entire_or_no` is Iterate's own parameter. So on a caller that passes
// `entire` — EstimateAndRefinePeriodicPrepayment (Amortize.pas:699) is one — the
// value that becomes `final`, and the value the half-penny early accept is tested
// against, come from a walk with NO re-amortization at adjustments, while every
// subsequent probe re-amortizes. The first secant step
// `newdelta := delta * p / (final - p)` is therefore a difference between two
// DIFFERENT functions.
//
// That is almost certainly an oversight in the original — but it is the original,
// it is load-bearing on any screen carrying an adjustment, and rule 7's spirit
// applies: transcribe, do not improve. Found by the 2026-08-07 adversarial review,
// which measured the single-terminal version wrong on 9 of 54 set-both-adjustment
// screens (one of them sign-flipped: DOS 8114.5449, port -13747.2002).
//
// Callers that pass `til_adj` (the payment / amount / rate solves — see
// fancyTerminal's note) have probe0 == terminal by construction and keep using
// dosIterateAbort.
func dosIterateMixedProbe0Abort(seed, accInit float64, probe0, terminal func(float64) float64,
	abort func() bool) (float64, bool) {
	return dosIterateCore(seed, accInit, probe0, terminal, abort, false)
}

// dosIterateRateAbort is dosIterateAbort for a solve whose `var x` IS the loan
// rate — DOS's EstimateAndRefineAdjRate / Re_Amortize rate branch, which pass
// `h^.loanrate` itself to Iterate (AMORTOP.pas:1523).
//
// That aliasing makes the THIRD arm of Iterate's until-clause live:
//
//	until (count >= 20) or (bestp < halfpenny) or (abs(h^.loanrate) > 2);
//
// `h^.loanrate` is `x`, and the test runs at the BOTTOM of the pass — after
// `x := x + delta` and after the `bestx := x` update — so a step that throws the
// trial rate past ±200%/yr stops the exploration. It is an EXIT, not a REJECTION:
// control still falls through to `x := bestx` and the ordinary verdict
// (`bestp > halfpenny` AND `bestp > acc_limit*init` ⇒ "did not converge"), so a
// root found at, say, -1.94 is adopted normally.
//
// On a payment or amount solve `x` is not the rate, so `h^.loanrate` is the
// loan's own fixed rate and the arm is inert unless the SCREEN rate itself
// exceeds 200%/yr — in which case DOS really does bail after one pass. That is
// why the flag is a parameter rather than a blanket `abs(x) > 2`.
func dosIterateRateAbort(seed, accInit float64, terminal func(float64) float64,
	abort func() bool) (float64, bool) {
	return dosIterateCore(seed, accInit, terminal, terminal, abort, true)
}

// dosIterateCore takes TWO terminals: `probe0` for the pre-loop evaluation that
// produces `final` and the half-penny early accept (AMORTOP.pas:1438-1439), and
// `terminal` for every pass inside the repeat (AMORTOP.pas:1464-1465). They are
// the same function for every caller that passes `til_adj`; see
// dosIterateMixedProbe0Abort for why they are not always the same.
func dosIterateCore(seed, accInit float64, probe0, terminal func(float64) float64,
	abort func() bool, rateTarget bool) (float64, bool) {
	const (
		small     = 0.001
		halfpenny = 0.005
		teeny2    = 1e-10
		accLimit  = 2e-8 // DOS acc_limit (AMORTOP.pas:1423)
	)
	tripped := func() bool { return abort != nil && abort() }
	// A zero seed makes `delta := small * x` zero, so the secant cannot start and
	// DOS's own loop would stall. But DOS EVALUATES THE TERMINAL FIRST
	// (AMORTOP.pas:1437-1444) and takes the `if (abs(p) < halfpenny) then goto 1`
	// early exit with `x` untouched — so a screen whose answer IS zero is SOLVED,
	// not refused. Bailing before the probe turned that into a refusal:
	//
	//	amort_oracle 120000 0.0 120 12 plusreg payhard=1000 presolve=6:12:12
	//	→ DOS: prepay 0.0000     (adjp = 120000 - 120*1000 = 0, so the guess is
	//	                          exactly 0 and the terminal is exactly 0)
	//
	// Found by the 2026-08-07 adversarial review. The `return 0, false` stays for
	// a zero seed whose terminal is NOT already at zero — there the secant really
	// cannot move and a refusal is the honest answer.
	if seed == 0 {
		if math.Abs(probe0(0)) < halfpenny {
			return 0, true
		}
		return 0, false
	}
	// DOS's Iterate accepts a result unless BOTH bestp > halfpenny AND
	// bestp > acc_limit*init (AMORTOP.pas:1487), where init is the starting
	// balance p — the loan amount. i.e. it also accepts a RELATIVE residual of
	// 2e-8 × amount. On very large / very steep terminals (e.g. a 573-period 29%
	// exact loan, whose overpay balance reaches billions), the absolute half-penny
	// is unreachable in float64 but the relative tolerance (~$0.04 on a $2.16M
	// loan) is met — so this clause is what lets the Newton converge there.
	//
	// 🚨 NO math.Abs. DOS writes `acc_limit * init` (AMORTOP.pas:1487) on the
	// SIGNED starting balance. When init is negative the product is negative,
	// `bestp > acc_limit*init` is then true for every bestp (bestp is an
	// absolute value, so >= 0), and the relative limb CANNOT rescue a residual
	// the half-penny test already rejected — DOS refuses. Taking the magnitude
	// instead turns that negative threshold positive and lets the port ACCEPT
	// a result DOS declines. The port's OTHER engine already gets this right
	// (dosport_walk.go:586, `bestp > accLimit*p0`, no magnitude), so the two
	// engines disagreed with each other about a single ported line.
	// Item 0j, round 50 — redone in round 54 because round 50's code was lost.
	accTol := accLimit * accInit
	x := seed
	final := probe0(x) // AMORTOP.pas:1438-1439 — til_adj, NOT entire_or_no
	if math.Abs(final) < halfpenny {
		if dpTrace {
			fmt.Fprintf(os.Stderr, "FITR0 seedx=%.17g p=%.17g (accepted at seed)\n", x, final)
		}
		return x, true
	}
	if dpTrace {
		fmt.Fprintf(os.Stderr, "FITR0 seedx=%.17g p=%.17g\n", x, final)
	}
	delta := small * x
	x += delta
	bestp := math.Inf(1)
	bestx := x
	count := 0
	for {
		// AMORTOP.pas:1450-1452 — the overflowflag check sits at the TOP of the
		// pass, BEFORE the terminal is re-evaluated, so a flag raised by the
		// PREVIOUS pass (or by the pre-loop evaluation above) aborts here. `goto 1`
		// skips `x := bestx` and the verdict entirely, so no estimate is adopted.
		if tripped() {
			return 0, false
		}
		p := terminal(x)
		var newdelta float64
		if math.Abs(final-p) > teeny2 {
			newdelta = delta * p / (final - p)
		}
		if math.Abs(delta) < teeny2 || math.Abs(newdelta/delta) > 1 {
			count += 5
		}
		delta = newdelta
		x += delta
		final = p
		if math.Abs(p) < bestp {
			bestp = math.Abs(p)
			bestx = x // DOS assigns bestx AFTER the x update (bug-for-bug faithful)
		}
		count++
		// AMORTOP.pas:1485 —
		//   until (count >= 20) or (bestp < halfpenny) or (abs(h^.loanrate) > 2);
		// The third arm only bites when Iterate's `var x` IS h^.loanrate (a rate
		// solve); see dosIterateRateAbort. It is an EXIT, not a rejection: control
		// still falls through to `x := bestx` and the ordinary verdict below, so a
		// root found beyond ±200%/yr is adopted normally. `x` here is post-update,
		// matching DOS's `x := x + delta` preceding the until-test.
		if dpTrace {
			fmt.Fprintf(os.Stderr, "FITR n=%d p=%.10f delta=%.10f newx=%.10f\n", count, p, delta, x)
		}
		if count >= 20 || bestp < halfpenny || (rateTarget && math.Abs(x) > 2) {
			break
		}
	}
	if dpTrace {
		fmt.Fprintf(os.Stderr, "FITRend bestp=%.10f bestx=%.10f count=%d\n", bestp, bestx, count)
	}
	return bestx, bestp < halfpenny || bestp <= accTol
}

// paymentTerminal selects the terminal procedure exactly as DOS's Iterate does
// (AMORTOP.pas:1437): RepayFancyLoan — the option-aware walk, evaluated UNFORCED
// via fancyTerminal — for `fancy OR (exact and non-360)`; otherwise the simple
// recursion (repayExactTerminal, the RepayLoan analogue) for a plain 360/365 loan.
//
// EXACT loans MUST use fancyTerminal, not repayExactTerminal: the display engine
// (generateFancyScheduleMode) reproduces DOS's actual-day per-period interest to
// the cent (verified by per-period trace), whereas repayExactTerminal's simplified
// recursion is ~250x less steep in the overpay region on long high-rate exact
// loans — its terminal zero is offset and its slope wrong, so the secant diverged
// (that was the exact-long-term case that needed the bisection fallback). Routing
// exact through fancyTerminal makes the Newton use the same DOS-faithful terminal
// the display does. In-advance exact keeps repayExactTerminal (fancyTerminal's
// early-return path is the forced settlement-shift generator).
func paymentTerminal(input LoanInput) func(float64) float64 {
	// A live US-Rule accumulator also forces the fancy walk. DOS's `fancy` is a
	// SCREEN-level global: Re_Amortize's `Iterate(p, usap, ...)` (AMORTOP.pas:1577)
	// still dispatches to RepayFancyLoan at :1437 even when every option lies
	// BEHIND the boundary, and RepayFancyLoan's ComputeNext charges interest on
	// `p - usap` (AMORTOP.pas:656). The port's segment sub-loan is synthesised from
	// only what lies AHEAD of the boundary, so a segment whose balloons and
	// prepayments are all spent looks option-free and flipped this dispatch to
	// repayExactTerminal — a plain annuity recursion with no usap concept at all.
	// The seed then IS its own root and the secant never moves.
	//
	// 2026-07-25 fuzzer5 seed 9015 — verified vs the real DOS engine:
	//
	//	amort_oracle 403395.75 0.0796810000 84 4 b365_360 r78 usa mor=36 \
	//	  b51=75125.93 pre=87:130:24:170.03 adj=153:0.0902590000: \
	//	  adj=186:0.1365840000: pts=0.021178 payhard=9799.37
	//	A replace-mode semi-monthly series pays 170.03 against ~950 of interest
	//	from 4/1/31 to 8/16/36, freezing principal at 286,969.78 and banking
	//	101,481.87 of unpaid interest. At the 10/1/36 rate adjustment DOS Iterates
	//	the 16,819.3031 annuity seed down to 16,305.4874, because the segment
	//	accrues on the FROZEN principal (286,969.78 x 9.0259%/4 = 6,475.40 flat)
	//	rather than on the 388,451.65 displayed balance. The port kept the seed —
	//	dInt 2,541.40.
	// ...and so does the segment itself, unconditionally. `input.initUsap != 0`
	// above catches only the sub-case where the USA-rule accumulator happens to
	// be live at the boundary; the dispatch DOS actually makes does not consult
	// usap at all. Re_Amortize runs only on a fancy screen, so its Iterate call
	// always lands on RepayFancyLoan — and ComputeNext, unlike RepayLoan's
	// :1286-1289 recursion, has no `p < 0` overpayment guard. On a segment whose
	// balance is negative at the adjustment the two disagree completely: the
	// plain recursion returns bal - n*d where DOS returns bal*f^n - d*(f^n-1)/(f-1).
	// See LoanInput.segmentSolve (seed 20509, dropped in-exact 2026-07-27).
	useFancy := input.segmentSolve || hasAnyAdvancedOption(input) || input.initUsap != 0 ||
		(exactDaily(&input.Settings) && !input.Settings.InAdvance)
	if !useFancy {
		return func(v float64) float64 { return repayExactTerminal(input, v) }
	}
	s := &input.Settings
	tr, _ := ComputeTrueRate(&input.Loan, s)
	fg := GrowthPerPeriod(&input.Loan, s.YrInv)
	return func(v float64) float64 { return fancyTerminal(input, v, s, tr, fg) }
}

// dosIteratePayment solves the regular payment via DOS's Iterate with `var x = d`
// (AMORTOP.pas:416). It seeds from the closed-form estimate; if that seed diverges
// on an option loan it retries from DOS's own adjusted-principal seed
// (EstimateAndRefinePayment), which converges to the SAME deterministic terminal
// root — so it cannot pick a different answer than DOS.
//
// Ported from legacy/src/dos_source/AMORTOP.pas: function Iterate.
func dosIteratePayment(input LoanInput, estimate float64) (float64, bool) {
	if estimate == 0 {
		return 0, false
	}
	accInit := input.Loan.Amount
	terminal := paymentTerminal(input)
	if r, ok := dosIterate(estimate, accInit, terminal); ok {
		return r, true
	}
	// Seed-fallback (still Newton, not bisection): DOS's Iterate seeds from the
	// non-prorated adjusted-principal annuity (EstimateAndRefinePayment). A prorated
	// or otherwise-offset estimate can make the secant diverge on a very steep
	// long-term balloon terminal; retry from DOS's own seed. Scoped to option loans.
	//
	// The seed here is built by dosSeedPayment, i.e. in DOS's OWN shape — one
	// additive adjp over one quotient (Amortize.pas:397-401) — and NOT by
	// multiplying the plain annuity by an adjp/amount ratio. The two are
	// algebraically identical and differ by ~2 ULP, and on this terminal's flat
	// plateaus 2 ULP decides which root the bracket-free secant walks to. Since
	// the whole point of this fallback is to reproduce DOS's walk, seeding it
	// with anything other than DOS's exact bits defeats it: on fuzzer5 seed
	// 20622 the multiplicative form seeded 44838.166556627089 where DOS seeds
	// 44838.166556627104, and the two engines converged stably to different
	// roots (dInt 5,634.44 on an 885,407.24 loan).
	if hasAnyAdvancedOption(input) {
		fg := GrowthPerPeriod(&input.Loan, input.Settings.YrInv)
		if adjpSeed, ok := dosSeedPayment(input, &input.Loan, &input.Settings, fg); ok &&
			adjpSeed != estimate {
			if r, ok := dosIterate(adjpSeed, accInit, terminal); ok {
				return r, true
			}
		}
	}
	return dosIterate(estimate, accInit, terminal)
}

// dosIterateSimplePayment solves the regular payment for a PLAIN (non-fancy) loan
// whose closed form is inexact — an in-advance (annuity-due) loan, or one with an
// odd first period — via DOS's Iterate with the RepayLoan terminal (AMORTOP.pas:1437
// else-branch: `RepayLoan(p)`, NOT RepayFancyLoan). RepayLoan is DOS's exact
// recursion — basis-INDEPENDENT for in-advance (the annuity-due form, no day-count,
// no proration) and first-period-prorated for odd-first arrears — so driving its
// terminal to zero reproduces DOS's non-fancy payment directly, with no basis
// substitution. accInit = the loan amount (DOS's init = the starting balance p).
func dosIterateSimplePayment(input LoanInput, estimate float64) (float64, bool) {
	if estimate == 0 {
		return 0, false
	}
	loan := input.Loan
	s := input.Settings
	// Prepaid (non-in-advance) first-period handling, mirroring generateSimpleSchedule
	// (the row-validated DOS-faithful simple schedule) exactly. naturalStart =
	// firstDate − one period. DOS's global `prorate` is 1 ONLY when the loan is taken
	// MORE than one period before the first payment (loanDate < naturalStart): the
	// settlement stub loanDate→naturalStart is collected at closing and the first
	// regular period is a FULL period (Amortize.pas:1276-1283). When the loan is taken
	// WITHIN one period of the first payment (loanDate >= naturalStart), there is no
	// stub and the first period is the actual SHORT span loanDate→firstDate — prorate
	// = firstPeriodProrate, which Go's RepayLoan already computes. So shift the
	// effective loan date to naturalStart (⇒ prorate 1) ONLY in the stub case; leave
	// it otherwise. (An unconditional shift wrongly forced prorate 1 on short-first
	// prepaid loans and solved the payment high.)
	loan = prepaidNaturalStartShift(loan, &s)
	terminal := func(v float64) float64 {
		return RepayLoan(loan.Amount, v, &loan, &s, s.YrInv)
	}
	return dosIterate(estimate, loan.Amount, terminal)
}

// prepaidNaturalStartShift applies the prepaid (non-in-advance) first-period
// rule to a loan headed for the closed-form RepayLoan terminal: when the loan
// is taken MORE than one period before the first payment (loanDate <
// naturalStart = firstDate − one period), the settlement stub
// loanDate→naturalStart is collected at closing and the amortizing walk's
// first period is a FULL period — DOS's `repay_from := firstdate − 1 period;
// prorate := 1` (Amortize.pas:1277-1282). Shifting the effective loan date to
// naturalStart makes firstPeriodProrate return exactly 1. When the loan is
// taken WITHIN one period (short first), there is no stub and the short
// actual span stands — an unconditional shift solved short-first prepaid
// payments high (oracle-validated; see dosIterateSimplePayment's history).
//
// 2026-07-11 audit finding A6: the payment solve had this shift but the
// amount/rate solves did not, so prepaid odd-first loans solved ~2% off.
// Verified vs the real DOS engine:
//
//	amort_oracle 0 0.12 12 12 noamt pay=888.4879 prepaid first=3  → solvedamount 10000.000149
//	amort_oracle 10000 0 12 12 norate pay=888.4879 prepaid first=3 → solvedrate 0.1200000283
func prepaidNaturalStartShift(loan Loan, s *Settings) Loan {
	if s.Prepaid && !s.InAdvance && dateutil.DateOK(loan.FirstDate) {
		if ns, err := dateutil.AddPeriod(loan.FirstDate, loan.PerYr, loan.FirstDate.Time.Day(), true); err == nil &&
			dateutil.DateComp(loan.LoanDate, ns) < 0 {
			loan.LoanDate = ns
		}
	}
	return loan
}

// dosIterateAmount solves the loan principal via DOS's Iterate with `var x =
// h^.amount` (AMORTOP.pas:460; the `p := x` at :1483 makes the starting balance
// track the trial amount). The terminal runs the UNFORCED schedule from the trial
// principal with the known payment/rate and returns the residual. Faithful analogue:
// fancyTerminal already starts its walk from in.Loan.Amount, so substituting the
// trial amount there is the same terminal DOS's RepayFancyLoan evaluates. accInit
// is the amount seed itself (DOS's init = the starting p = the trial amount).
func dosIterateAmount(input LoanInput, estimate float64) (float64, bool) {
	if estimate == 0 {
		return 0, false
	}
	s := &input.Settings
	tr, _ := ComputeTrueRate(&input.Loan, s)
	fg := GrowthPerPeriod(&input.Loan, s.YrInv)
	pay := input.Loan.PayAmt
	terminal := func(v float64) float64 {
		in := input
		in.Loan.AmountStatus = types.InOutInput
		in.Loan.Amount = v
		if exactInAdvanceUnforced(in) {
			return repayExactTerminal(in, pay)
		}
		if !hasFancyOptions(in) && !exactDaily(s) {
			// Plain (non-fancy, non-exact-daily) loan: its forward schedule IS the
			// closed RepayLoan recursion — the in-advance annuity-due branch, the
			// odd-first-period prorate, and the overpay branch all live there. Refine
			// against RepayLoan so the backward solve matches the exact forward
			// schedule the user sees. fancyTerminal's option-aware walk models those
			// cases differently (it diverges for in-advance and for day-count first
			// periods), so routing them through it would not round-trip. See
			// docs/postmortem_365_exact_interest.md §8.
			// Prepaid odd-first: same natural-start shift as the payment solve
			// (audit finding A6 — DOS prorate:=1, Amortize.pas:1277-1282).
			l := prepaidNaturalStartShift(in.Loan, s)
			return RepayLoan(v, pay, &l, s, s.YrInv)
		}
		return fancyTerminal(in, pay, s, tr, fg)
	}
	return dosIterate(estimate, estimate, terminal)
}

// dosIterateRate solves the loan rate via DOS's Iterate with `var x = h^.loanrate`
// (AMORTOP.pas:477). DOS recomputes `f := GrowthPerPeriod` at the top of each loop
// step (AMORTOP.pas:1451) because the rate is the target, so the terminal here
// recomputes true-rate/growth for every trial rate before running the UNFORCED
// schedule at the known amount/payment. accInit = the loan amount (DOS's init = the
// starting balance p, which is fixed for a rate solve).
// THE THIRD RESULT IS DOS'S SCREEN CONDEMNATION. On the fancy arm DOS's trial
// walk is RepayFancyLoan, whose EPILOGUE unconditionally runs
//
//	h^.loanrate := saverate;
//	ComputeTrueRate;                          {AMORTOP.pas:1234-1235}
//
// on EVERY pass of the secant — so a trial rate alone is enough to reach
// `RateFromYield(rr, peryr) = nn*lnn(1 + rr/nn)` (INTSUTIL.pas:1271-1275) with a
// non-positive argument, and lnn then sets BOTH errorflag and overflowflag
// (INTSUTIL.pas:1164-1171). Iterate's `if (overflowflag) then goto 1`
// (AMORTOP.pas:1453-1454) lands past `x := bestx` AND past the convergence
// verdict, so no rate is adopted, and MakeTable's `if (errorflag) then exit`
// (Amortize.pas:1457-1458) refuses the SCREEN — not merely the solve. The caller
// must therefore surface the "inconsistency" error rather than a "did not
// converge" warning: converged=false alone would let the port amortize at the
// unrefined closed-form rate and print a table where DOS prints none.
//
// 2026-07-29 fuzzer5 seed 21001, class adj1+balloon2+mor+prepay1+targ|first<|norate
// — verified against the real DOS engine and against a purpose-built lnn /
// RateFromYield trace oracle, which caught the offending probe directly
// (`TRACE RFY yy=-1.98491511 nn=1` ⇒ `TRACE lnn x=-0.98491511`):
//
//	amort_oracle 286646.25 0.0825230000 11 1 r78 loandmy=10.5.2024 \
//	  firstdmy=10.7.2024 mor=50 b74=9452.00 b86=17640.25 pre=38:67:52:25.09 \
//	  adj=14::39030.00 targ=6146.07 payhard=38280.38 norate
//	  DOS:  ERR Error: The data you have specified contain an inconsistency.
//	  port: int=-121949.03 paid=164697.22 rows=77
//
// This is an ANNUAL loan (peryr=1 ⇒ nn=1), which is why an ordinary secant
// excursion reaches it: Iterate's own magnitude escape is `abs(h^.loanrate) > 2`,
// so -1.98 is still in bounds while `1 + yy/1` has already gone negative. The
// pre-existing GrowthPerPeriod guard in SolveRate cannot catch this — it tests
// only the FINAL refined rate, never the intermediate probes.
//
// Same defect class as the structural-port fix in dosport_walk.go's
// repayFancyLoan epilogue (seed 20303) and solveSegmentRate's `condemned` latch
// (seed 8900); this closes the backward-rate-solve route.
//
// The latch is armed only on the FANCY arm, because that is the only arm whose
// DOS terminal is RepayFancyLoan. Iterate's dispatch is
// `if (fancy or (exact and basis<>x360)) then RepayFancyLoan else RepayLoan`
// (AMORTOP.pas:1437-1441), and RepayLoan has no ComputeTrueRate epilogue — a
// plain loan's trial rates cannot raise the flag mid-secant, and condemning one
// here would refuse screens DOS accepts. (Such a loan can still die on the FINAL
// ComputeTrueRate in EstimateAndRefineRate, but that is the existing
// GrowthPerPeriod guard's territory.)
func dosIterateRate(input LoanInput, estimate float64) (float64, bool, bool) {
	if estimate == 0 {
		return 0, false, false
	}
	accInit := input.Loan.Amount
	pay := input.Loan.PayAmt
	// Sticky, exactly as the DOS globals are: nothing in Iterate clears
	// overflowflag once a guarded primitive has raised it.
	var condemned bool
	terminal := func(v float64) float64 {
		in := input
		in.Loan.LoanRateStatus = types.InOutInput
		in.Loan.LoanRate = v
		s2 := in.Settings
		tr, trErr := ComputeTrueRate(&in.Loan, &s2)
		fg := GrowthPerPeriod(&in.Loan, s2.YrInv)
		if exactInAdvanceUnforced(in) {
			return repayExactTerminal(in, pay)
		}
		if !hasFancyOptions(in) && !exactDaily(&s2) {
			// Plain loan: refine against the RepayLoan recursion, the forward
			// schedule's own terminal (see dosIterateAmount for the rationale).
			// Prepaid odd-first: natural-start shift (audit finding A6).
			l := prepaidNaturalStartShift(in.Loan, &s2)
			return RepayLoan(l.Amount, pay, &l, &s2, s2.YrInv)
		}
		// RepayFancyLoan's epilogue — see the doc comment above.
		if trErr != nil {
			condemned = true
		}
		return fancyTerminal(in, pay, &s2, tr, fg)
	}
	r, ok := dosIterateAbort(estimate, accInit, terminal,
		func() bool { return condemned })
	return r, ok, condemned
}

// exactInAdvanceUnforced reports whether this loan's UNFORCED terminal must be
// the settlement-shifted in-advance exact recursion (repayExactTerminal) rather
// than the option-aware fancyTerminal. It mirrors the predicate that
// generateFancyScheduleMode / paymentTerminal use to route exact × in-advance
// (no advanced options) to the dedicated settlement-shift generator: because
// that generator is a FORCED schedule (it folds the residual into the last row),
// fancyTerminal cannot supply the smooth unforced terminal the secant needs, so
// the amount/rate solvers evaluate the residual via repayExactTerminal instead.
// See paymentTerminal (fancybisect.go) and generateExactInAdvanceSchedule.
func exactInAdvanceUnforced(input LoanInput) bool {
	// SOLVE terminal gate: non-360 ONLY. DOS's Iterate routing is `fancy or
	// (exact and basis<>x360)` (AMORTOP.pas:1438) — an exact in-advance loan at
	// 360 solves its payment on the PLAIN annuity-due RepayLoan terminal, while
	// its DISPLAY takes the settlement-shift shape (pass-2 finding 5: DOS's
	// payment is identical exact-ON/OFF at 360, 3172.2326 — another
	// solve/display split, like the USA rule).
	return exactDaily(&input.Settings) && input.Settings.InAdvance &&
		!hasAnyAdvancedOption(input)
}

// (refineAdjustmentPayment was removed in the M1 step of the global-Iterate
// refactor — see docs/global_iterate_refactor.md. It bisected the adjustment
// payment against the ENTIRE schedule's terminal, which is ill-posed once a
// second ARM re-amortizes downstream, so it had to be gated to a single
// adjustment. solveSegmentPayment below replaces it with DOS's til_adj SEGMENT
// solve, which composes for any number of adjustments.)

// solveSegmentPayment solves the regular payment for the REMAINING SEGMENT of a
// schedule so it retires to zero, accounting for the balloons/prepayments/skip
// that lie ahead. It is the Go analogue of DOS's Re_Amortize calling
// Iterate(..., til_adj) (AMORTOP.pas:1571-1587 / 1415): that inner solve runs the
// schedule from the boundary to very_last WITHOUT re-amortizing at any LATER
// adjustment (adjnum=0 ⇒ Re_Amortize is never re-entered, AMORTOP.pas:1215) and
// WITHOUT the final-fold, driving just this segment's terminal to zero.
//
// Two callers use it, both passing the balance at a mid-schedule boundary:
//   - the MORATORIUM boundary (FirstRepay): the post-moratorium payment must
//     retire the remaining schedule like DOS's single solved payment
//     (docs/amort_option_combo_divergences.md §3); and
//   - each ARM adjustment (AO5): the segment payment after a rate reset, so a
//     loan with TWO+ ARMs composes correctly — each adjustment solves its own
//     segment independently, ignoring later adjustments, exactly as til_adj does
//     (the entire-schedule refineAdjustmentPayment was ill-posed with 2+ ARMs).
//
// It builds a sub-loan for the remaining term — balance `bal` amortized over
// `remaining` periods at the current rate, starting at firstPay with its prior
// period at prevDate — carrying the not-yet-applied balloons/prepayments (and
// skip months). The single regular period prevDate→firstPay reproduces the main
// schedule's first segment period exactly, so the solved payment is what the main
// schedule needs after the boundary. Returns ok=false (caller keeps the analytic
// seed) when there is nothing ahead to account for or the solve cannot bracket.
// clipPrepaymentsForSegment restricts a prepayment series to the extras that
// fall STRICTLY AFTER `boundary` (a rate/payment adjustment date). DOS's
// Re_Amortize re-solves the post-adjustment segment payment (and, for AO6, the
// implied rate) using the prepay array in its PARTIALLY-CONSUMED state — old_pre,
// saved during the forward walk at the adjustment (AMORTOP.pas:1552-1557). The
// piecewise segment sub-loans are built from a fresh copy of input.Prepayments and
// re-run from the boundary as their own loan date, so without this clip they
// re-apply the WHOLE series from its original StartDate, over-prepaying the segment
// and mis-solving the payment/rate. Each kept series is rebuilt as an NN-bounded
// series starting on its first surviving extra so the sub-loan's fancy walk emits
// exactly the remaining extras (mirrors old_pre's advanced nextDate + shrunk nn).
func clipPrepaymentsForSegment(pps []Prepayment, boundary types.DateRec) []Prepayment {
	var out []Prepayment
	for _, pp := range pps {
		if pp.StartDateStatus < types.InOutDefault || pp.PerYr <= 0 {
			continue
		}
		// The series bound is a DATE, never a count. DOS's CheckPrepayments
		// (AMORTOP.pas:416-419) converts an NN-specified series to
		// stopdate = AddNPeriods(startdate, peryr, pred(nn)) once, up front, and
		// CheckOffBalloon then retires the series solely on nextdate > stopdate —
		// so the emitted count is whatever fits on or before that date, which for a
		// peryr=24 off-grid anchor is NOT nn (see CheckPrepaymentStops). Bounding
		// this enumeration by the count instead re-introduced the extra row.
		hasNN := pp.NNStatus >= types.InOutDefault && pp.NN > 0
		hasStop := pp.StopDateStatus >= types.InOutDefault
		bound := pp.StopDate
		if !hasStop && hasNN {
			if sd, err := dateutil.AddNPeriods(pp.StartDate, pp.PerYr, pp.NN-1); err == nil {
				bound, hasStop = sd, true
			}
		}
		if !hasStop {
			continue // unbounded/malformed — nothing sensible to clip
		}
		day := pp.originDay()
		dt := pp.StartDate
		var firstRemaining types.DateRec
		remaining := 0
		for k := 0; ; k++ {
			if dateutil.DateComp(dt, bound) > 0 || k > MaxSchedulePeriods {
				break
			}
			if dateutil.DateComp(dt, boundary) > 0 {
				if remaining == 0 {
					firstRemaining = dt
				}
				remaining++
			}
			nd, err := dateutil.AddPeriod(dt, pp.PerYr, day, false)
			if err != nil {
				break
			}
			dt = nd
		}
		if remaining == 0 {
			continue // fully consumed before the segment
		}
		clip := pp
		// Carry the ORIGINAL day-of-month anchor across the re-base. DOS's
		// Re_Amortize does not rebuild the series at all — it restores
		// `pre[i]^ := old_pre[i]^` (AMORTOP.pas:1552-1557) from a snapshot whose
		// `startdate` is still the user's start date and whose `nextdate` is
		// wherever the walk had advanced it. Every subsequent step is
		//
		//	AddPeriod(nextdate, pre[i]^.peryr, pre[i]^.startdate.d, add);
		//
		// so `orig_day` stays the ORIGINAL start day forever. Re-basing StartDate
		// onto `firstRemaining` without preserving that anchor silently re-anchors
		// a SEMI-MONTHLY series to the cursor's own day, and AddPeriod's
		// snap-back window (`if abs(day-orig_day) < 4`) then lands the rest of the
		// series on different dates. 2026-07-26 fuzzer5 seed 20201 — verified vs
		// the real DOS engine:
		//
		//	amort_oracle 100000 0.12 10 1 loandmy=15.2.2025 firstdmy=15.2.2026 \
		//	  mor=48 pre=12:27:24:254.56 adj=24:0.08: payhard=16000
		//
		// The 27-extra semi-monthly series (anchor day 15) is 25 extras deep at
		// the 2/15/2027 rate-only adjustment, so `firstRemaining` is 2/28/2027.
		// With the anchor lost, the clipped series stepped 2/28 → 3/13/2027
		// (28+15 = 43 ⇒ carry to 13 March, and |13-28| = 15 is outside the
		// snap window) instead of DOS's 2/28 → 3/15/2027 (|13-15| = 2 snaps back
		// to the 15th). Two days of interest on a $139k balance moved the
		// segment's terminal by 55.64, and the re-solved post-moratorium payment
		// came out 26809.75 against DOS's 26821.65 — dInt 22.87 on the minimal
		// repro and 136,226.94 on the seed case (the port's interest went
		// NEGATIVE: -115,263.81 vs DOS's 20,963.13).
		clip.anchorDay = pp.originDay()
		clip.StartDate = firstRemaining
		clip.StartDateStatus = types.InOutInput
		clip.NextDate = firstRemaining
		clip.NN = remaining
		clip.NNStatus = types.InOutInput
		// Keep the ORIGINAL bound date. DOS's Re_Amortize restores old_pre, whose
		// `stopdate` is still the one CheckPrepayments derived from the FULL series;
		// re-deriving it from the re-based StartDate and the shrunken count would
		// land somewhere else the moment AddNPeriods' year shortcut applies.
		clip.StopDate = bound
		clip.StopDateStatus = types.InOutInput
		out = append(out, clip)
	}
	return out
}

// moratoriumForSegment returns the moratorium as it still applies to a mid-loan
// segment starting at `boundary`. DOS's segment solves (EstimateAndRefineAdjRate /
// EstimateAndRefineAdjPayment, Amortize.pas:311-366) run RepayFancyLoan over the
// WHOLE screen and Iterate at the adjustment, so ComputeNext's interest-only
// moratorium branch (AMORTOP.pas:640-652) still shapes every row of the segment
// that precedes first_repay. A moratorium already fully consumed by the boundary
// no longer constrains the sub-loan, so it is dropped.
func moratoriumForSegment(m Moratorium, boundary types.DateRec) (Moratorium, bool) {
	if m.FirstRepayStatus < types.InOutDefault || !dateutil.DateOK(m.FirstRepay) {
		return Moratorium{}, false
	}
	if dateutil.DateComp(m.FirstRepay, boundary) <= 0 {
		return Moratorium{}, false
	}
	return m, true
}

// segmentGrid reproduces the DATE GRID DOS hands a mid-loan segment solve.
//
// Both Re_Amortize branches call Iterate with the boundary's NEXT ROW as the
// sub-loan's `firstdate` (AMORTOP.pas:1523 for the rate solve, :1577 for the
// payment solve — `t := NextPayment.date`). RepayFancyLoan then seeds
//
//	t := firstdate; AddPeriod(t, h^.peryr, firstdate.d, subtract)   {AMORTOP.pas:1148}
//	paidthru := firstdate; AddPeriod(paidthru, ..., subtract)       {:1153-1156}
//
// so the sub-walk's base date and prevdate are BOTH one period before that next
// row — and `loandate` is never read at all in prepaid mode. But the regular
// payment grid inside the walk is NOT anchored on that date: ComputeNext advances
// with `AddPeriod(date, h^.peryr, h^.firstdate.d, add)` (AMORTOP.pas:594), and
// for a monthly/quarterly/annual frequency AddPeriod FORCES `d := orig_day`
// (INTSUTIL.pas:1239). h^ is the global loan record, untouched by the sub-walk,
// so the grid stays on the ORIGINAL loan's first-payment day.
//
// That distinction only shows when the row after the boundary is OFF-GRID — an
// extra-only or balloon-only row, which happens whenever a prepayment series runs
// at a higher frequency than the loan. Building the sub-loan with FirstDate = that
// off-grid row (as the port did) moves every regular payment in the segment onto
// the extra's day-of-month and solves a different schedule than DOS. 2026-07-25
// fuzzer5 — verified vs the real DOS engine:
//
//	amort_oracle 328690.81 0.1228420000 156 12 plusreg \
//	  pre=27:231:24:196.54 adj=69::4953.43
//	The adjustment lands 10/1/2029; the next row is the semi-monthly extra at
//	10/16/2029. DOS's sub-walk therefore runs base 9/16 → first row 10/1 on the
//	day-1 grid and fits 19.935%; anchoring on the 16th fitted 19.238% and lost
//	$11,161 of interest.
//
// Returns the sub-loan's LoanDate (= base = prevdate seed) and FirstDate (= the
// first regular row of the segment). For weekly/biweekly frequencies AddPeriod is
// pure julian arithmetic and ignores orig_day, so this degenerates to
// (boundary-1period, boundary) — i.e. the previous behaviour.
// segmentNextRow is DOS's `NextPayment.date` at the moment Re_Amortize fires —
// the EARLIEST of the next regular payment, the next pending extra and the next
// pending balloon (ComputeNext / FindNextExtra, AMORTOP.pas:497-534, 601-616).
// The Go walk iterates regular periods and emits off-cycle extras inside the
// period, so the earliest pending off-cycle date has to be folded in explicitly.
func segmentNextRow(nextRegular types.DateRec,
	segPre []Prepayment, futureBalloons []BalloonPayment) types.DateRec {
	nextRow := nextRegular
	for i := range segPre {
		if dateutil.DateOK(segPre[i].StartDate) &&
			dateutil.DateComp(segPre[i].StartDate, nextRow) < 0 {
			nextRow = segPre[i].StartDate
		}
	}
	for i := range futureBalloons {
		if dateutil.DateOK(futureBalloons[i].Date) &&
			dateutil.DateComp(futureBalloons[i].Date, nextRow) < 0 {
			nextRow = futureBalloons[i].Date
		}
	}
	return nextRow
}

func segmentGrid(loan Loan, nextRegular types.DateRec,
	segPre []Prepayment, futureBalloons []BalloonPayment) (types.DateRec, types.DateRec, bool) {
	if !dateutil.DateOK(nextRegular) || !dateutil.DateOK(loan.FirstDate) {
		return types.DateRec{}, types.DateRec{}, false
	}
	// DOS's `firstdate` for the sub-walk is NextPayment.date — the next ROW, which
	// ComputeNext resolves as the EARLIEST of the next regular payment, the next
	// pending extra and the next pending balloon (AMORTOP.pas:497-534, 601-616).
	// The Go walk iterates regular periods and emits off-cycle extras inside the
	// period, so the earliest pending off-cycle date has to be folded in here.
	nextRow := segmentNextRow(nextRegular, segPre, futureBalloons)
	base, err := dateutil.AddPeriod(nextRow, loan.PerYr, nextRow.Time.Day(), true)
	if err != nil {
		return types.DateRec{}, types.DateRec{}, false
	}
	first, err := dateutil.AddPeriod(base, loan.PerYr, loan.FirstDate.Time.Day(), false)
	if err != nil {
		return types.DateRec{}, types.DateRec{}, false
	}
	return base, first, true
}

// segmentPeriods counts the regular payments the sub-walk still emits: DOS stops
// on `DateComp(WhenToStop^.date, stopdate) >= 0` with stopdate = the WHOLE loan's
// very_last (AMORTOP.pas:1140-1142, 1225), so the segment runs to the original
// last payment date, not to `remaining` periods off the boundary row.
func segmentPeriods(loan Loan, first types.DateRec, fallback int) int {
	if !dateutil.DateOK(first) || !dateutil.DateOK(loan.LastDate) || !loan.LastOK {
		return fallback
	}
	day := loan.FirstDate.Time.Day()
	n := 1
	dt := first
	for dateutil.DateComp(dt, loan.LastDate) < 0 && n < 10000 {
		nd, err := dateutil.AddPeriod(dt, loan.PerYr, day, false)
		if err != nil {
			return fallback
		}
		dt = nd
		n++
	}
	if n <= 0 {
		return fallback
	}
	return n
}

// `usap` is the LIVE USA-rule exempt-principal accumulator as of the boundary
// row. DOS's Re_Amortize passes the unit-level global straight into Iterate
// (`Iterate(p, usap, Payment.date, t, d, til_adj)`, AMORTOP.pas:1577), which
// saves it as `initusa` (:1436) and restores it before EVERY trial walk (:1457)
// — so each trial re-runs the bounded segment from the same accumulator the
// main walk had reached. Modelling the segment as a standalone sub-loan starts
// that accumulator at 0, so the trial rows charge interest on `p - 0` where DOS
// charges `p - usap`, and the solved segment payment comes out high. Seed the
// sub-loan with it (LoanInput.initUsap). Only observable when usap is non-zero
// at the adjustment, which needs a row whose payment did not cover its interest
// — a skip, a moratorium or a target floor.
// The third result, `bad`, distinguishes DOS's two very different "no refined
// payment" outcomes, which the old two-value signature collapsed into one:
//
//   - GATE NOT FIRED (bad=false). DOS's Re_Amortize only Iterates under
//     `(user_nballoons > 0) or (npre > 0) or ((exact) and (basis<>x360))`
//     (AMORTOP.pas:1571). Outside that gate it never calls Iterate at all and
//     simply keeps the analytic annuity seed it computed at :1565-1569. The
//     caller must do the same.
//
//   - ITERATE FAILED (bad=true). Inside the gate, `if Iterate(...) then ...
//     else begin abort := true; errorflag := true; end` (AMORTOP.pas:1577-1587).
//     `abort` terminates the RepayFancyLoan walk on the spot (the until-clause
//     at :1221 tests it) and `errorflag` is the engine-wide condemnation:
//     EstimateAndRefineAdjPayment reports `(not errorflag)` (Amortize.pas:338)
//     and MakeTable/Enter both bail on `if (errorflag) then exit`
//     (Amortize.pas:1204, :1219, :1458). DOS produces NO TABLE — the screen
//     shows the non-convergence message and nothing else.
//
// The port used to drop the second case on the floor and continue the schedule
// at the UNREFINED annuity seed, emitting an answer where DOS refuses to answer
// — the worst divergence direction, and the exact asymmetry the AO6 rate branch
// had already been fixed for (engine.go:3528, "DOS-FAITHFUL FAILURE
// PROPAGATION"). Unlike that branch, dosIteratePayment is a bug-for-bug port of
// DOS's own 20-step secant rather than a stronger solver, so {Go fails} is NOT a
// subset of {DOS fails}: DOS can converge where the port stalls. See
// 2026-07-28 fuzzer5 seed 20572, where DOS's terminal evaluation carries a
// 1-ULP residue that just clears `teeny`, letting its secant overflow to
// -1.97e18, cancel back into a second root's basin and converge, while the
// port's terminal is bit-exactly flat (the row sensitivities cancel +1+1-1-1
// exactly, and the interest is prepaid so no compounding re-introduces any) and
// its secant cannot move. Refusing there is still strictly better than the
// 62,740.69 of invented interest the seed fallback produced.
func solveSegmentPayment(input LoanInput, loan Loan, settings Settings,
	bal float64, prevDate, firstPay types.DateRec, remaining int, seed, usap float64,
	hLastDate types.DateRec) (val float64, ok bool, bad bool) {
	// NO SIGN GATE ON `bal`. DOS's Re_Amortize refines the analytic seed with
	// Iterate under `(user_nballoons > 0) or (npre > 0) or ((exact) and
	// (basis<>x360))` and NOTHING else (AMORTOP.pas:1571) — there is no test on
	// the sign of `p`. A NEGATIVE balance at the adjustment is a real DOS regime:
	// a moratorium that suspends the regular payment while a prepayment series
	// keeps running (or a balloon lands) over-funds the loan, so `p` goes
	// negative and the re-solved payment is a REFUND the borrower receives. The
	// old `bal <= 0` early return silently fell back to the analytic annuity
	// seed there, which is a first-order approximation that ignores the
	// moratorium's suppressed rows — it spreads the refund over every remaining
	// installment instead of only the paying ones, so the tail never retires.
	// 2026-07-25 fuzzer5 seed 9007 — verified vs the real DOS engine:
	//
	//	amort_oracle 400081.60 0.0486750000 14 1 b365 exact prepaid mor=84 \
	//	  b96=18235.30 b120=13333.00 pre=12:140:12:348.66 \
	//	  adj=108:0.0999050000:48630.76 adj=132:0.1212590000: pts=0.023384
	//	At the 1/1/2035 rate-only adjustment p = -148289.84. DOS Iterates to
	//	d = -103709.8237 and the 1/1/2038 terminal balance is exactly 0; the
	//	gated port kept the seed -61873.9419 and left -88744.74 outstanding,
	//	which moved the APR value stream's first evaluation by ~1510.69 and
	//	sent the two secants down completely different trajectories.
	if remaining <= 0 || bal == 0 {
		return 0, false, false
	}
	// Only the balloons that still lie ahead of the boundary remain to be paid;
	// any balloon inside the moratorium has already reduced `bal`.
	var futureBalloons []BalloonPayment
	for _, b := range input.Balloons {
		if b.AmountStatus >= types.InOutDefault && math.Abs(b.Amount) > 0 &&
			dateutil.DateComp(b.Date, prevDate) > 0 {
			futureBalloons = append(futureBalloons, b)
		}
	}
	// A plain moratorium re-amortizes exactly with the analytic annuity seed.
	// Engage the schedule-oracle solve only when a DOWNSTREAM option changes the
	// REQUIRED post-moratorium payment: a later balloon/prepayment, or skipped
	// months (fewer paying periods ⇒ a higher retiring payment). A TARGET is
	// deliberately NOT included: DOS keeps the plain annuity for a target (the
	// target only bumps the individual periods that fall below it, never the base
	// solve), so a moratorium loan with a target retires at the SAME payment as a
	// plain moratorium. Folding the target into this solve perturbs the payment
	// even when it never binds — e.g. mor=74 + targ=61 on $261k/240: pure mor
	// solves DOS's 2297.73 to the cent, but adding the (inactive) target dropped
	// it to 2258.53 and lost ~$2,885 of interest. See
	// docs/amort_option_combo_divergences.md.
	hasSkip := anySkip(input.SkipMonths.MonthSet)
	// The EXACT method on a non-360 basis also invalidates the analytic seed:
	// the remaining segment accrues actual-day interest (DOS solves the
	// moratorium payment via Iterate over the exact RepayFancyLoan terminal,
	// Amortize.pas:416 + AMORTOP.pas:625). 2026-07-11 pass-2 finding 4 —
	// verified vs the real DOS engine:
	//
	//	amort_oracle 100000 0.10 24 12 b365 exact mor=6 → payment 5712.4662
	//	(the analytic seed left the exact-OFF 5712.6693; with DOS's payment the
	//	port's rows already matched to the cent — solve only)
	// Day-count frequencies (semimonthly/biweekly/weekly) on a non-360 basis
	// also invalidate the analytic seed: the rows accrue ACTUAL days (14/366 in
	// a leap year) while the annuity's constant f uses 14/365.25 — DOS's
	// Iterate drives the real actual-day walk. 2026-07-11 pass-2 finding 8 —
	// verified vs the real DOS engine:
	//
	//	amort_oracle 100000 0.10 52 26 b365 inadv mor=3 → payment 2375.0973
	//	(the analytic seed left 2375.3444; rows at DOS's payment already
	//	matched row-for-row, totals 11549.56 exact)
	// Clip the prepayment series to only the extras that fall AFTER the segment
	// boundary. DOS's Re_Amortize re-solves the post-adjustment payment using the
	// prepay array in its PARTIALLY-CONSUMED state (old_pre, AMORTOP.pas:1552-1557):
	// the extras already paid before the adjustment are gone; only the remaining
	// ones bound the segment. The sub-loan below is built from a fresh copy of the
	// series, so without this clip it re-applies the ENTIRE series from its original
	// StartDate — over-prepaying the segment and solving too LOW a regular payment
	// (rate-only ARM at 1/1/2030 with a 97-extra semi-monthly series 1/15/2026 →
	// 1/15/2030 solved 618.83 vs DOS 814.52 — the balance never retired). §43.
	segmentPre := clipPrepaymentsForSegment(input.Prepayments, prevDate)
	// DOS's Re_Amortize refines the analytic seed with Iterate ONLY under
	// (AMORTOP.pas:1571)
	//
	//	if (user_nballoons > 0) or (npre > 0) or ((df.c.exact) and (df.c.basis<>x360))
	//
	// and nothing else. Three notes on reading that condition literally:
	//
	//   - `user_nballoons` is the USER's balloon count, not the count still
	//     ahead of the boundary, so a loan whose balloons are all behind it
	//     still refines (the walk itself then just reproduces the annuity,
	//     shaped by whatever skip/target rows it crosses).
	//   - `npre` here is `old_npre`, restored at :1552-1557 from
	//     SaveDataForReAmortize (:1201), which snapshots the LIVE series count
	//     at the current payment. CheckOffBalloon decrements npre as each
	//     series runs out (:562), so an EXHAUSTED prepayment series does not
	//     keep the refinement alive — hence the clip to segmentPre.
	//   - SKIPPED MONTHS ARE ABSENT. DOS keeps the plain annuity seed across a
	//     skip and lets the tail absorb the shortfall.
	//
	// This gate used to also fire on `hasSkip` and on a day-count frequency at
	// a non-360 basis. Both were tuned for the moratorium-boundary re-solve
	// that FIX #10 deleted; with this function down to a single caller (the
	// AO5/AO7 adjustment re-amortize) they are simply non-DOS. The skip clause
	// in particular cost 4,438.45 of interest on
	//
	//	amort_oracle 492520.94 0.1346680000 240 12 b365_360 \
	//	  pre=107:94:52:269.90 adj=168:0.1369020000: adj=188:0.0304450000: \
	//	  targ=504.97 skip=1,7 pts=0.023116
	//
	// where the 1/1/2032 series is long exhausted by the 1/1/2038 adjustment:
	// DOS keeps its 5,625.76 annuity seed, the skip-aware refinement solved
	// 6,261.28.
	userBalloons := 0
	for i := range input.Balloons {
		if input.Balloons[i].AmountStatus >= types.InOutDefault {
			userBalloons++
		}
	}
	if userBalloons == 0 && len(segmentPre) == 0 && !exactDaily(&settings) {
		return 0, false, false
	}
	// The AMOUNT branch does NOT inherit the rate branch's off-grid sub-walk.
	// DOS's Re_Amortize (AMORTOP.pas:1573-1575) does
	//
	//	t := NextPayment.date;
	//	n := NumberOfInstallments(h^.firstdate, t, h^.peryr, on_or_after);
	//	if Iterate(p, usap, Payment.date, t, d, til_adj) then ...
	//
	// and NumberOfInstallments is declared `(var f, l: daterec; ...)` — `l` is a
	// VAR parameter that the routine SNAPS onto the payment grid derived from `f`
	// (INTSUTIL.pas:936-941, "adjusts l to be exactly on a payment day … as
	// specified by z"). With z = on_or_after and f = h^.firstdate, `t` is rounded
	// FORWARD onto the loan's own regular grid before Iterate ever sees it. The
	// value assigned to `n` is then dead — the snap of `t` is the whole point of
	// the call. So the AMOUNT branch's sub-walk always starts at the next REGULAR
	// payment, i.e. firstPay, and its base_date/paidthru follow from that; the
	// off-cycle extra that would shift the grid in the rate branch is snapped
	// away here.
	//
	// (The rate branch at :1523 passes nextpayment.date RAW, with no such call —
	// which is exactly why solveSegmentRate must apply segmentGrid and this must
	// not. Applying it here regressed the 2026-07-24 rate-only ARM case
	// TestFancyAPRAdjustmentOffCyclePrepayVsOracle/rate_only_adjustment: the
	// 1/15/2030 extra pulled the sub-loan onto a 12/15/2029 grid and solved
	// 810.92 against DOS's 814.52.)
	subLoanDate, subFirstDate := prevDate, firstPay
	// ...and the sub-walk's GRID is not simply firstPay. DOS does two things here,
	// and both matter:
	//
	//  1. NumberOfInstallments(h^.firstdate, t, h^.peryr, on_or_after) rounds `t`
	//     FORWARD through its VAR parameter. `t := NextPayment.date` is the
	//     LOOKAHEAD row (DecideWhetherToPrintALine, AMORTOP.pas:1062-1077, tests the
	//     row about to be printed), so it can be an OFF-CYCLE extra or balloon; this
	//     is what puts it back on a payment date.
	//  2. RepayFancyLoan then derives base_date from that snapped `t`
	//     (`t := firstdate; AddPeriod(t, h^.peryr, firstdate.d, subtract)`,
	//     AMORTOP.pas:1150-1152) — anchored on the SNAPPED date's own day — but
	//     every row after that comes from ComputeNext, which re-anchors on the
	//     ORIGINAL loan's day: `date := base_date; AddPeriod(date, h^.peryr,
	//     h^.firstdate.d, add)` (AMORTOP.pas:597-598). h^.firstdate is untouched by
	//     the call — RepayFancyLoan's `firstdate` is a plain value PARAMETER that
	//     shadows nothing.
	//
	// So the sub-walk's first row is base_date + one period at the ORIGINAL anchor
	// day, not the snapped date itself. Step 2 usually washes step 1 back out, but
	// not always: an extra at 5/29 against a day-28 loan snaps forward to 6/28,
	// where the raw date would have regenerated the already-emitted 5/28.
	//
	// Verified against an instrumented DOS build
	// (scripts/build_trace_oracle.sh -mode ra, which prints `t` on both sides of the
	// NumberOfInstallments call, plus -mode cn for the rows) on
	//
	//	amort_oracle 100000 0.05 60 12 b365 exact loandmy=28.1.2025 \
	//	  firstdmy=28.2.2025 adj=3:0.10:
	//
	// DOS snaps t 5/28/2025 -> 5/31/2025 (firstdate 2/28 is a month end, so
	// INTSUTIL.pas:1018 stretches the day to daysinm) and then walks 5/28, 6/28,
	// 7/28 ... — the day-31 stretch never reaches a row. Modelling the sub-loan
	// with FirstDate = the snapped 5/31 puts it on a month-end grid and solves
	// 2114.48 against DOS's 2113.06 (dInt 22.12 on the full screen; 2471.02 on the
	// seed-20217 fuzzer case). Modelling it with the raw 5/28 happens to be right
	// here but breaks the seed-20214 off-cycle case by 9154.57. Only the two-step
	// derivation is right for both.
	// ...and the snap has to be taken RAW. NumberOfInstallments' monthly branch
	// ends (INTSUTIL.pas:1013) with
	//
	//	if (flast) then l.d:=daysinm(l) else l.d:=f.d;
	//
	// which copies the FROM-date's day onto the snapped month with NO clamp, so a
	// day-29/30/31 loan snapping into February writes back a PHANTOM daterec such
	// as 30/2/2031. dateutil.NumberOfInstallments normalizes that to 2/3/2031;
	// NumberOfInstallmentsRaw is the same routine returning the three fields
	// un-normalized, and here the difference is not cosmetic — it moves the
	// sub-walk a whole month and re-derives `remaining` off the wrong row:
	//
	//	DOS   t = 30/2/2031 → base = 30/1/2031 → row1 = 30/2/2031 → CLAMP → 28/2/2031
	//	port  t =  2/3/2031 → base =  2/2/2031 → row1 = 30/3/2031
	//
	// Both AddPeriod steps overwrite the day from their origDay argument, so the
	// phantom's only job is to keep the MONTH one step back; normalizing rolls the
	// month forward and the sub-walk starts a period late.
	//
	// 2026-07-28, fuzzer5 seed 40001 on the newly-opened day-29/30/31 loan-date
	// axis. Minimal reproducer:
	//
	//	amort_oracle 115239.23 0.0367770000 132 12 b365_360 exact \
	//	  loandmy=30.6.2025 firstdmy=30.7.2025 adj=67:0.0637620000:
	//	→ DOS int 30575.26 / paid 145814.49, Go int 30311.12 / paid 145550.35
	//	  (dInt −264.14)
	//
	// The instrumented DOS build (/tmp/build_ra.sh, the RAB/RAI trace) shows the
	// two sides agreeing on the analytic seed to ten decimals and parting company
	// on `t` alone:
	//
	//	RAB n=66 p=62638.7376558070 adjp=62638.7376558070 f=1.005313500000
	//	    nb=0 npre=0 adjdate=1/30/131 lastdate=6/30/136
	//	RAI seed_d=1142.1814887797 pdate=1/30/131 t=2/30/131 ...
	//	RAIout d=1144.7644816040
	//
	// against the port's `GRA refined=1165.932871 (seed 1142.181489)` — same seed,
	// 21.17 apart on the refined payment, which is exactly the per-row gap
	// TestM5Rows reports from the 2/28/31 row onward.
	//
	// The three ingredients the option sweep isolated all land on this line. A
	// day-29/30/31 loan date is required because it is the only way `l.d := f.d`
	// can overflow the snapped month. `exact` is required because
	// solveSegmentPayment's gate above (`userBalloons == 0 && len(segmentPre) == 0
	// && !exactDaily`) is otherwise the early return — with no balloons and no
	// prepayments, the exact×non-360 disjunct of AMORTOP.pas:1571 is the only
	// thing that reaches this code at all. And the adjustment has to land in a
	// month whose snap crosses February: the sweep's adj=5, 6, 9 (November,
	// December, March) snap to themselves and agree to the cent, while adj=8, 20,
	// 32, 44, 56, 68 all resolve into a February and diverge.
	//
	// The sweep's other signature — DOS returning ONE total for adj=7 and adj=8
	// (44020.01) and one for adj=67 and adj=68 (30575.26) where the port returned
	// two — is the Amortize.pas:258-271 entry snap, and it is NOT a second bug:
	// month 8 is entered at 28/2/2026 (day 30 clamped by CheckForDaysTooLarge) and
	// walks back on_or_before to 30/1/2026, month 7's date exactly. `adjdump` on
	// the oracle confirms the port snaps to the same 1/30/2031; the pairs differed
	// only because the UNREFINED payment differs.
	snapT := firstPay
	snapY, snapM, snapD := firstPay.Time.Year(), int(firstPay.Time.Month()), firstPay.Time.Day()
	snapOK := dateutil.DateOK(firstPay)
	if loan.PerYr > 0 && dateutil.DateOK(loan.FirstDate) && dateutil.DateOK(firstPay) {
		_, ry, rm, rd := dateutil.NumberOfInstallmentsRaw(loan.FirstDate, firstPay,
			loan.PerYr, types.OnOrAfter)
		if rm >= 1 && rm <= 12 && rd >= 1 {
			snapY, snapM, snapD = ry, rm, rd
			// snapT is the NORMALIZED view of the same snap, kept for the callers
			// below that need a real DateRec (the prepaid accrual anchor). It is
			// unchanged from before this fix; only the base/row1 derivation moved
			// onto the raw fields.
			if snapped := types.NewDateRec(ry, time.Month(rm), rd); dateutil.DateOK(snapped) {
				snapT = snapped
			}
		} else {
			snapOK = false
		}
	}
	subBase, baseOK := types.DateRec{}, false
	if snapOK {
		// DOS: `t := firstdate; AddPeriod(t, h^.peryr, firstdate.d, subtract)`
		// (AMORTOP.pas:1150-1152) — the anchor day is the SNAPPED date's own day,
		// i.e. the raw, possibly-overflowing one.
		if b, err := dateutil.AddPeriodFields(snapY, snapM, snapD,
			loan.PerYr, snapD, true); err == nil {
			subBase, baseOK = b, true
			if row1, err := dateutil.AddPeriod(b, loan.PerYr,
				loan.FirstDate.Time.Day(), false); err == nil && dateutil.DateOK(row1) {
				if dateutil.DateComp(row1, firstPay) != 0 {
					remaining = segmentPeriods(loan, row1, remaining)
					// §66's AO7 ARM, CLOSED (round 28). THE RECOUNT MUST NOT
					// OUTRUN THE PARENT'S h^.lastdate.
					//
					// DOS has NO period count at this site. Its Re_Amortize calls
					// Iterate(p, usap, Payment.date, t, d, til_adj)
					// (AMORTOP.pas:1577), Iterate re-enters RepayFancyLoan, and
					// RepayFancyLoan's ComputeNext decides regular-vs-extra ROW BY
					// ROW against the h^.lastdate GLOBAL:
					//
					//	balloonpos := 1;
					//	if (xsource > 0) then begin
					//	  balloonpos := DateComp(nextextra.date, date);
					//	  if (DateComp(date, h^.lastdate) > 0) then
					//	    balloonpos := -1;   {AMORTOP.pas:606 — the regular row
					//	                         is DROPPED and the pending extra
					//	                         taken instead}
					//
					// The CALLER already reconstructs that decision as a count:
					// `segN`, from NumberOfInstallmentsRaw(adjDate, adjLastDate, …)
					// capped by veryLast (engine.go), which is the whole of §53.
					// segmentPeriods DISCARDS it — legitimately, because a phantom
					// snap has moved row1 off the boundary date and the incoming
					// count no longer measures the walk that will actually run —
					// but it recounts against `loan.LastDate` with a strict `<`,
					// i.e. as a CEILING: it stops at the first grid date AT OR PAST
					// the bound. That is exact for an ON-GRID bound and one whole
					// period too many for a bound the INTSUTIL.pas:1018 month-end
					// snap has pushed OFF-grid, because the true last regular date
					// is then strictly below it and the ceiling steps past.
					//
					// Scoped to the recount on purpose. The blanket version of this
					// clamp — applied to the sub-loan's final derived LastDate the
					// way solveSegmentRate applies it — ALSO shortens the cases
					// where segmentPeriods never fired and `segN` stands on its own,
					// double-correcting a count that was already snap-aware. That
					// was measured, not reasoned: paired_regression 40000-40039
					// returned NEW=1 on
					//
					//	amort_oracle 494177.99 0.0862230000 1512 12 b365_360 exact \
					//	  prepaid usa loandmy=3.9.2023 firstdmy=3.11.2023 \
					//	  b279=134321.10 b1064=66184.58 pre=147:215:6:741.22 \
					//	  pre=8:226:12:792.20 adj=79:0.0651460000:3690.70 \
					//	  adj=641:0.1063080000: adj=938::4964.85 targ=580.62 \
					//	  skip=6 pts=0.020199 payhard=3924.15
					//
					// where DOS answers (interest 5190503.46) and the over-clamped
					// port REFUSED with "Computation of payment amount or interest
					// rate did not converge" — an answer withheld where DOS gives
					// one, the worst divergence direction.
					//
					// c3, 2026-08-04 — verified against the real DOS engine with the
					// `-mode cn` trace oracle (CN lines = DOS's per-row ComputeNext)
					// diffed against the port's DPTRACETERMROWS=1 TERMROW lines:
					//
					//	amort_oracle 393752.15 0.0477520000 26 2 prepaid usa \
					//	  loandmy=29.4.2023 firstdmy=29.2.2024 mor=70 \
					//	  b94=82687.19 b106=93767.40 b118=59796.70 \
					//	  pre=10:89:6:507.99 adj=34:0.0762230000: \
					//	  adj=112:0.0437960000:15897.68 targ=3910.14 pts=0.030192 \
					//	  payhard=26754.42 non lastdmy=29.8.2036
					//
					// lastdmy 2036-8-29; the snap carries h^.lastdate to 2036-8-31.
					// Caller segN = 21; segmentPeriods recounted 22 because
					// 2036-8-29 < 2036-8-31 buys one more semi-annual step to
					// 2037-2-28. BOTH sub-walks are 76 rows (the horizon is
					// very_last = 2038-10-29, set by the prepayment series, and it
					// is NOT the divergent bound — which is why counting rows alone
					// finds nothing). Rows 0-64 byte-identical; row 65 is
					// 2037-2-28, where DOS emits bpos=-1 pay 507.99 and the port
					// emitted a 22nd REGULAR payment of -18162.04. That one row
					// moved the terminal at the SHARED seed and the two secants
					// converged on different roots: DOS -21236.435395 against the
					// port's -20178.789827 (displayed row 17, 26746.59 vs 25688.94).
					//
					// SHORTEN ONLY. A count that is already at or below the bound is
					// the port's reconstruction of a walk DOS bounds by very_last;
					// lengthening it here would re-introduce the extra row from the
					// other side. §53 is the case where the snap legitimately pushes
					// the global PAST the last scheduled payment and DOS DOES emit a
					// regular row there, so the pristine loan.LastDate is the wrong
					// bound and hLastDate — the caller's live adjLastDate — is right.
					if dateutil.DateOK(hLastDate) {
						rem0 := remaining
						_ = rem0
						n := 1
						dt := row1
						for n < remaining {
							nd, e := dateutil.AddPeriod(dt, loan.PerYr,
								loan.FirstDate.Time.Day(), false)
							if e != nil || dateutil.DateComp(nd, hLastDate) > 0 {
								break
							}
							dt = nd
							n++
						}
						if n < remaining {
							remaining = n
						}
						// 🚨 r55 REACH PROBE, SECOND EDITION — AND THE FIRST ONE
						// WAS A TRUE-BY-CONSTRUCTION NULL (R69).
						//
						// The question item 1 turns on is whether the un-taken
						// "extend" side of this rule is reachable: the loop above
						// is `for n < remaining`, so n can never exceed remaining
						// and an extend can never be proposed.
						//
						// ⚠️ THE FIRST PROBE ANSWERED THAT BY RE-WALKING TO
						// hLastDate WITHOUT THE CAP AND SUBTRACTING `remaining`.
						// That is an ARITHMETIC IDENTITY, not a measurement.
						// `remaining` is already segmentPeriods(loan, row1, …)
						// (:1199), a CEILING count against loan.LastDate — its
						// loop runs `for DateComp(dt, loan.LastDate) < 0`, so it
						// stops on the first row AT OR PAST the bound. The old
						// probe counted a FLOOR against hLastDate (it broke when
						// the NEXT date exceeded the bound). floor(B) - ceil(A) is
						// <= 0 whenever B <= A, so EXT was pinned at 0 by algebra
						// and the round's first headline had no power at all.
						// Round 55 audit pass 1, finding 1.
						//
						// nCeil is now the SAME KIND OF COUNT as `remaining` —
						// a ceiling — taken against hLastDate instead of
						// loan.LastDate. That makes EXT = nCeil - remaining a
						// real comparison of two bounds rather than of two
						// counting conventions.
						//
						// 🚨 DO NOT READ EXT>0 AS "THE PORT IS SHORT OF DOS".
						// r55's own audit pass 2 REFUTED the warrant this probe
						// was first published with. That warrant said
						// AMORTOP.pas:1221's until-clause runs after ComputeNext
						// so the first row at or past the horizon is emitted — a
						// CEILING. For THIS walk that is false. Iterate calls
						// RepayFancyLoan with Output=nil and adjnum=0
						// (AMORTOP.pas:1439/1465), and :1130-1142 then sets
						// `WhenToStop := @NextPayment`, DOS's own comment saying
						// it stops when nextpayment.date = very_last and goes
						// "one further" only when PRINTING. So the modelled
						// sub-walk is a FLOOR; the until-clause tests `stopdate`
						// (= very_last at adjnum=0), NOT h^.lastdate; and
						// ComputeNext's h^.lastdate clause (:606) fires only when
						// `xsource > 0`.
						//
						// EXT is therefore A DIFFERENCE BETWEEN TWO BOUNDS AND
						// NOTHING MORE. It is NOT an eligible stratum and it has
						// NO positive predictive value for a divergence: pass 2
						// built the variant that TAKES the extend and measured the
						// standing arm at 12 HARD in 2,091 — UNCHANGED, signature
						// for signature — while the answer on the one EXT=1 case
						// moved 17x FURTHER from DOS. Read this probe as "how
						// often do the two bounds disagree at all", full stop.
						if dpTraceSegAmt {
							const probeMax = 100000
							// POSITIVE CONTROL (R49/R69/R73). DPTRACESEGAMTPC=k
							// moves the probe's bound k periods past hLastDate.
							// ⚠️ IT PROVES THE SUBTRACTION, NOT THE REACH: the
							// natural population's hLastDate/loan.LastDate gap is
							// a month-end SNAP of a few DAYS, and this control
							// injects WHOLE PERIODS. Read it as an arithmetic
							// self-test of the probe, never as evidence that a
							// genuine extend is expressible. Audit pass 1, f.2.
							probeBound := hLastDate
							if pcs := os.Getenv("DPTRACESEGAMTPC"); pcs != "" {
								if k, e := strconv.Atoi(pcs); e == nil && k > 0 {
									for i := 0; i < k; i++ {
										nd, e2 := dateutil.AddPeriod(probeBound,
											loan.PerYr,
											loan.FirstDate.Time.Day(), false)
										if e2 != nil {
											break
										}
										probeBound = nd
									}
								}
							}
							nCeil, dtC := 1, row1
							for i := 0; i < probeMax; i++ {
								if dateutil.DateComp(dtC, probeBound) >= 0 {
									break
								}
								nd, e := dateutil.AddPeriod(dtC, loan.PerYr,
									loan.FirstDate.Time.Day(), false)
								if e != nil || dateutil.DateComp(nd, dtC) <= 0 {
									break
								}
								dtC = nd
								nCeil++
							}
							// hEQ records WHY: when hLastDate and loan.LastDate
							// coincide the two counts agree by definition, and a
							// zero here is a fact about the POPULATION, not about
							// the rule.
							hEQ := 0
							if loan.LastOK &&
								dateutil.DateComp(hLastDate, loan.LastDate) == 0 {
								hEQ = 1
							}
							fmt.Fprintf(os.Stderr,
								"SEGAMT rem0=%d remaining=%d n=%d nCeil=%d EXT=%d "+
									"short=%d hEQ=%d perYr=%d\n",
								rem0, remaining, n, nCeil, nCeil-remaining,
								rem0-remaining, hEQ, loan.PerYr)
						}
					}
				}
				subFirstDate = row1
			}
		}
	}
	// ...but the ACCRUAL anchor still has to follow DOS. RepayFancyLoan's prologue
	// (AMORTOP.pas:1149-1157) reads `loandate` — the Payment.date boundary — ONLY
	// when the loan is not prepaid:
	//
	//	if (prepaid) then
	//	  begin
	//	    paidthru := firstdate;
	//	    if (not df.c.in_advance) then
	//	      AddPeriod(paidthru, h^.peryr, firstdate.d, subtract);
	//	  end
	//	else
	//	  paidthru := loandate;
	//
	// `firstdate` there is the SNAPPED t and the AddPeriod anchor is that date's own
	// day — i.e. exactly base_date — so a prepaid screen accrues the sub-walk's first
	// row from base_date (or from the snapped t itself in advance) rather than from
	// the boundary row. Those differ exactly when the boundary is an off-cycle extra
	// — the same gap that produced the 2026-07-25 seed-20102 divergence in
	// solveSegmentRate; this branch had the identical latent hole, just reached by a
	// different snap of `firstdate`.
	//
	// As in the rate branch, the anchor is carried by LoanDate with Prepaid
	// CLEARED (below): inside AMORTOP.pas `prepaid` has only two readers — the
	// display-side PrepaidInterest at :177 and this paidthru assignment — so the
	// port's prepaid settlement stub has no DOS counterpart in a solver sub-walk.
	if settings.Prepaid && dateutil.DateOK(snapT) {
		subLoanDate = snapT
		if !settings.InAdvance && baseOK {
			subLoanDate = subBase
		}
	}
	// The sub-loan is a MID-LOAN segment: the in-advance settlement interest and
	// one-period base-date shift happened at the ORIGINAL loan date and must not
	// be re-applied here. DOS's til_adj Iterate walks ComputeNext, which accrues
	// ordinary opening-balance interest even when in_advance is set
	// (AMORTOP.pas:636) — the in_advance flag only shapes the loan's start, which
	// is already behind the boundary. Clearing it makes the sub-loan the plain
	// bounded segment DOS solves. (Pass-2 finding 8: with InAdvance carried in,
	// the sub-solve re-shifted the segment and gave 2423.3853 vs DOS 2375.0973.)
	segSettings := settings
	segSettings.InAdvance = false
	// Prepaid likewise: subLoanDate above now IS DOS's paidthru, so re-applying the
	// port's prepaid start-shaping on top of it would double-count the anchor.
	segSettings.Prepaid = false
	sub := LoanInput{
		Loan: Loan{
			AmountStatus:   types.InOutInput,
			Amount:         bal,
			LoanRateStatus: types.InOutInput,
			LoanRate:       loan.LoanRate,
			NStatus:        types.InOutInput,
			NPeriods:       remaining,
			PerYrStatus:    types.InOutInput,
			PerYr:          loan.PerYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput,
			LoanDate:       subLoanDate,
			FirstStatus:    types.InOutInput,
			FirstDate:      subFirstDate,
		},
		Balloons:     futureBalloons,
		Prepayments:  segmentPre,
		Settings:     segSettings,
		Fancy:        true,
		initUsap:     usap,
		segmentSolve: true,
		// DOS's sub-walk uses TWO different day-of-month anchors, and they are
		// not the same one:
		//
		//   base_date: RepayFancyLoan steps BACK off the phantom snapped date
		//     `t` using the PHANTOM's own day (AMORTOP.pas:1149-1150,
		//     `t := firstdate; AddPeriod(t, h^.peryr, firstdate.d, subtract)`
		//     where the local `firstdate` IS the phantom). That is snapD, and
		//     it is already applied by the subBase derivation above.
		//
		//   every row: Paymenttype.ComputeNext steps FORWARD off base_date
		//     using the SCREEN's first-payment day (AMORTOP.pas:596-598,
		//     `date := base_date; AddPeriod(date, h^.peryr, h^.firstdate.d, add)`).
		//     `h` is the outer loan record, so `h^.firstdate.d` is the real
		//     screen first-payment day — never the phantom's, never the
		//     sub-loan's own clamped day.
		//
		// So the grid anchor for the walk is loan.FirstDate's day. See
		// LoanInput.gridAnchorDay for the two seeds that pin this down.
		gridAnchorDay: loan.FirstDate.Time.Day(),
	}
	// Skip months are by calendar month, so they apply unchanged in the sub-loan.
	// (Target is intentionally omitted for the plain moratorium — see the gate
	// comment above; DOS solves the plain annuity and lets the per-period target
	// bump and the final-fold absorb any residual.)
	if input.Target.TargetStatus >= types.InOutDefault {
		// Once the schedule-oracle solve IS engaged (a downstream balloon /
		// prepayment / skip / exact-daycount reshapes the segment), DOS's Iterate
		// walks the WHOLE fancy schedule — ComputeNext applies the target floor on
		// every row it evaluates (AMORTOP.pas:640-652). Dropping the target from
		// the sub-loan therefore solves a DIFFERENT schedule than DOS does. The
		// early gate above still keeps a target-only moratorium on the plain
		// analytic annuity (where DOS likewise never re-solves), so this does not
		// reintroduce the mor=74+targ=61 regression that comment describes.
		// 2026-07-24 fuzzer5 — verified vs the real DOS engine:
		//
		//	amort_oracle 327984.19 0.0554450000 60 4 mor=75 \
		//	  pre=108:16:24:211.29 targ=1188.15 → payment 13753.1217
		//	(without the target threaded the segment solved 12662.1107, which is
		//	 exactly DOS's answer for the SAME screen with targ removed — i.e. the
		//	 port was solving the no-target loan. dInt was 8686.97.)
		sub.Target = input.Target
	}
	// A moratorium that has NOT yet reached first_repay at this boundary still
	// forces interest-only rows inside the segment DOS solves over, so the
	// sub-loan must carry it (see moratoriumForSegment).
	if mor, ok := moratoriumForSegment(input.Moratorium, prevDate); ok {
		sub.Moratorium = mor
	}
	if hasSkip {
		sub.SkipMonths = input.SkipMonths
		// When a target ALSO binds, it converts the skipped months from
		// negative-amortizing (pay 0, balance grows) into target-floored rows
		// (pay interest + the minimum principal reduction), which LOWERS the
		// retiring payment. DOS's Iterate walks the full fancy schedule with both
		// skip and target applied (ComputeNext AMORTOP.pas:643 — target overrides
		// skip), so the segment solve must carry the target too, otherwise the
		// skip rows are solved as pure negative-am and the payment comes out at
		// the no-target value. Only threaded when skip is present: a skip-free
		// moratorium+target retires at the plain annuity (target never binds the
		// base solve) and is handled by the early gate return above. 2026-07-13
		// pass-4 P4-N1b — verified vs the real DOS engine:
		//
		//	amort_oracle 50000 0.164 24 12 inadv targ=0.01 skip=4-6 mor=3
		//	→ payment 3500.3264 (= the non-in-advance quad; the skip rows are
		//	  target-floored, not negative-am. Without the target the segment
		//	  solved 3706.5934 = the no-target inadv+skip+mor value.)
		if input.Target.TargetStatus >= types.InOutDefault {
			sub.Target = input.Target
		}
	}
	// DOS solves the segment payment with Iterate(..., til_adj) — the same Newton,
	// over RepayFancyLoan run only to the next boundary (AMORTOP.pas:1571-1587/1415).
	// The sub-loan IS that bounded segment as a standalone fancy loan, so
	// dosIteratePayment (Newton over the sub-loan's UNFORCED fancy terminal) drives
	// the identical residual to zero.
	// No positivity gate: DOS's Iterate has none, and a NEGATIVE segment payment
	// is a real DOS regime, not a solver artefact. When a REPLACE-mode prepayment
	// collides with the regular payment date while a moratorium is still in force,
	// ComputeNext's balloonpos=0 arm rewrites the row as
	//
	//	payamt := payamt - d + interest        {AMORTOP.pas:641-642}
	//
	// with payamt already replaced by the extra's amount. The row's principal
	// reduction is therefore (extra - d): it DECREASES as d increases. To retire
	// a balance far larger than the extra, DOS's secant must drive d NEGATIVE. A
	// `refined > 0` filter rejected exactly those roots and silently fell back to
	// the analytic annuity seed, which under-amortizes forever. 2026-07-25
	// fuzzer5 pass 2 — verified vs the real DOS engine:
	//
	//	amort_oracle 197375.26 0.0541370000 144 12 exact mor=34 \
	//	  b70=51002.62 b71=32638.92 pre=1:231:24:153.48 \
	//	  adj=11:0.0634470000: adj=26:0.0686800000:2145.56 skip=5-7 \
	//	  pts=0.033234 payhard=1906.15
	//	DOS re-amortizes at the 12/1/2024 rate adjustment to d ≈ -14498.94, so
	//	every post-adjustment collision row shows prin = 153.48 - d = 14652.42
	//	and the loan retires in 140 rows. The port's terminal already brackets
	//	that root (term(-14498.94) = -452, term(0) = +245717); only the gate
	//	stopped it, leaving the seed 1661.38 and a never-retiring 260-row walk.
	// DPTRACESEG=1 sweeps the segment terminal over a wide grid before the
	// secant runs. The segment terminal is NOT monotone once a target floor or a
	// replace-mode prepayment series is in play — seed 9011's had three roots and
	// a flat plateau sitting exactly on DOS's analytic seed — so when the port and
	// DOS disagree on a re-amortized payment, the first thing to establish is
	// whether they are solving the same function (same roots) and merely landing
	// in different basins, or genuinely walking different schedules.
	if dpTraceSeg {
		term := paymentTerminal(sub)
		fmt.Fprintf(os.Stderr, "SEGTERM bal=%.4f remaining=%d rate=%.10f seed=%.6f "+
			"prev=%s firstPay=%s subLoan=%s subFirst=%s npre=%d mor=%v/%s\n",
			bal, remaining, loan.LoanRate, seed,
			prevDate.Time.Format("2006-1-2"), firstPay.Time.Format("2006-1-2"),
			subLoanDate.Time.Format("2006-1-2"), subFirstDate.Time.Format("2006-1-2"),
			len(segmentPre), sub.Moratorium.FirstRepayStatus,
			sub.Moratorium.FirstRepay.Time.Format("2006-1-2"))
		step := math.Max(1, math.Abs(seed)) / 4
		if f, err := strconv.ParseFloat(os.Getenv("DPTRACESEGSTEP"), 64); err == nil && f > 0 {
			step = f
		}
		for i := -20; i <= 20; i++ {
			v := seed + float64(i)*step
			fmt.Fprintf(os.Stderr, "SEGTERM   d=%16.4f term=%20.6f\n", v, term(v))
		}
	}
	// PAST THIS POINT DOS'S GATE HAS FIRED, so DOS really does call Iterate and a
	// failure really is `abort := true; errorflag := true`.
	refined, ok := dosIteratePayment(sub, seed)
	if !ok {
		return 0, false, true
	}
	if refined == 0 {
		// Iterate SUCCEEDED and its root is zero. DOS stores it
		// (`adj[next_adj]^.amount := d`, AMORTOP.pas:1579) rather than
		// condemning the screen, so this is not the `bad` arm — but the
		// caller's long-standing `refined != 0` filter keeps the seed here,
		// and changing that is a separate question from failure propagation.
		return 0, false, false
	}
	return refined, true, false
}

// solveSegmentRate is the AO6 (payment-only / implied-rate adjustment) analog of
// solveSegmentPayment: it solves the RATE at which a KNOWN new payment amortizes
// the post-adjustment segment [adj -> last] to zero, walking the REAL actual-day
// fancy schedule instead of the uniform-period balanceAfterN.
//
// DOS's EstimateAndRefineAdjRate (Amortize.pas:347-368) solves this rate by
// calling RepayFancyLoan — the actual-day display walk — and driving its
// terminal balance to zero. On a day-count frequency (semimonthly/biweekly/
// weekly) at a non-360 basis, or the exact method, the display accrues ACTUAL
// days (e.g. a 14-day biweekly period vs the uniform 365.25/26 = 14.05) while
// balanceAfterN's constant GrowthPerPeriod does not — so the uniform solveAdjRate
// returns a slightly-off implied rate and the segment interest drifts (~0.08% on
// the biweekly N7 case; pass 6). The sub-loan IS that bounded segment as a
// standalone fancy loan with the new payment hard-set, so the secant over its
// UNFORCED fancy terminal reproduces DOS's Iterate. On the 360 basis uniform ==
// actual, so the caller keeps solveAdjRate there (this engages only on exact /
// day-count-non-360). Mirrors solveAdjRate's secant structure and |rate|<2 clamp.
//
// The third result mirrors DOS's `errorflag`, exactly as ComputeAPRWithPoints's
// third result does (backward.go:735-800). The rate secant evaluates
// ComputeTrueRate at every trial rate, and ComputeTrueRate ends in
// RateFromYield's `lnn(1 + yy/nn)` (rates.go:81). A trial rate low enough to
// drive `1 + yy/nn <= 0` hits DOS's lnn guard, which raises "Error: The data you
// have specified contain an inconsistency." and sets BOTH overflowflag and
// errorflag (INTSUTIL.pas:1164-1171). overflowflag stops Iterate at the top of
// its very next pass (`if (overflowflag) then goto 1;`, AMORTOP.pas:1450-1452)
// — and `goto 1` skips both `x := bestx` and the convergence verdict, so no rate
// is adopted and no "did not converge" message is emitted — while errorflag
// condemns the whole screen: MakeTable yields no table at all.
//
// The port used to write `tr, _ := ComputeTrueRate(...)`, DISCARDING that error.
// The secant then carried on from a zeroed true rate, wandered to a fabricated
// implied rate, and returned a full schedule for a screen the real engine
// refuses outright. 2026-07-25 fuzzer5 seed 8900 (N=1000):
//
//	amort_oracle 490572.51 0.0914250000 17 1 b365_360 exact prepaid plusreg \
//	  mor=36 b84=14634.18 b96=137694.66 b144=89203.11 pre=120:72:12:730.96 \
//	  adj=72:0.0785270000:53788.45 adj=156::78398.28 pts=0.000009
//	  DOS: ERR Error: The data you have specified contain an inconsistency.
//	  port: int=427780.27 paid=918352.78 rows=59
//
// `usap` is the live USA-rule accumulator at the boundary — see
// solveSegmentPayment's note; DOS's rate Iterate (AMORTOP.pas:1520-1531) takes
// it through the same `initusa` save/restore.
// `hLastDate` is the caller's LIVE h^.lastdate — the piecewise walk's
// `adjLastDate`, which carries §50's VAR-parameter snap (AMORTOP.pas:1547) and
// is therefore the date DOS's ComputeNext is actually testing against at
// :606. It is NOT loan.LastDate: §53's whole point is that the snap can push
// the global a month PAST the pristine last payment, and DOS then emits a
// regular row there. Passing the pristine date instead would silently undo
// that. See the horizon clamp below (§66, round 25).
func solveSegmentRate(input LoanInput, loan Loan, settings Settings,
	bal float64, prevDate, firstPay types.DateRec, remaining int, payment, seedRate, usap float64,
	hLastDate types.DateRec, veryLast types.DateRec) (rate float64, ok, inconsistent bool) {
	// NO `bal > 0` GUARD. DOS's rate branch runs Iterate on whatever balance the
	// walk is carrying (AMORTOP.pas:1520-1531) — and the walk that feeds the
	// one-sided-adjustment pre-pass does not stop at zero (see
	// unreachedAdjPrepass), so a payment-only adjustment sited past the retirement
	// point is legitimately solved against a NEGATIVE balance. Bailing out here
	// used to hand that case back to the caller's uniform annuity fast path — a
	// routine DOS does not have — which happily fitted a rate to a screen DOS
	// condemns. A zero balance still bails: there is nothing to amortize and
	// dosIterate's own `seed == 0` guard would refuse it anyway.
	if remaining <= 0 || bal == 0 || payment <= 0 {
		return 0, false, false
	}
	var futureBalloons []BalloonPayment
	for _, b := range input.Balloons {
		if b.AmountStatus >= types.InOutDefault && math.Abs(b.Amount) > 0 &&
			dateutil.DateComp(b.Date, prevDate) > 0 {
			futureBalloons = append(futureBalloons, b)
		}
	}
	// Mid-loan segment: the in-advance settlement/base-shift happened at the
	// original loan date and must not be re-applied (see solveSegmentPayment).
	// The prepay series is clipped to the extras remaining after the boundary,
	// exactly as in solveSegmentPayment (DOS's partially-consumed old_pre). §43.
	segmentPre := clipPrepaymentsForSegment(input.Prepayments, prevDate)
	// DOS's RATE branch calls Iterate WITHOUT first restoring the saved
	// prepay/balloon state: `Iterate(p, usap, payment.date, nextpayment.date,
	// h^.loanrate, til_adj)` at AMORTOP.pas:1523, while
	// `next_balloon := old_next_balloon; ... pre[i]^ := old_pre[i]^; npre := old_npre`
	// only runs at :1592, AFTER the solve. (The AMOUNT branch does restore first,
	// at :1552-1560 — hence solveSegmentPayment clips at the boundary instead.)
	//
	// So the sub-walk inherits the PARTIALLY-CONSUMED array: the ComputeNext that
	// produced NextPayment already ran CheckOffBalloon (:543-570) on any extra or
	// balloon falling ON NextPayment.date, advancing it past. Re-clip at that row.
	// Nothing can fall strictly between the boundary and NextPayment.date — that
	// date is by definition the earliest pending row — so this only ever drops
	// items exactly AT it. 2026-07-25 fuzzer5, case
	// `328690.81 0.1228420000 156 12 plusreg pre=27:231:24:196.54 adj=69::4953.43`:
	// the 10/16/2029 semi-monthly extra is consumed producing NextPayment, so DOS's
	// sub-walk starts its series at 11/1; keeping it fitted 19.961% vs DOS 19.935%.
	nextRow := segmentNextRow(firstPay, segmentPre, futureBalloons)
	if dateutil.DateComp(nextRow, prevDate) > 0 {
		segmentPre = clipPrepaymentsForSegment(input.Prepayments, nextRow)
		kept := futureBalloons[:0:0]
		for _, b := range futureBalloons {
			if dateutil.DateComp(b.Date, nextRow) > 0 {
				kept = append(kept, b)
			}
		}
		futureBalloons = kept
	}
	// Put the sub-loan on DOS's grid (see segmentGrid); prevDate stays the
	// boundary for the clipping above. Grid off the ORIGINAL nextRow (DOS's
	// `firstdate` argument to Iterate), not the re-clipped series — the row was
	// consumed, but it is still what seeds base_date.
	subLoanDate, subFirstDate := prevDate, firstPay
	if base, first, ok := segmentGrid(loan, nextRow, nil, nil); ok {
		if dateutil.DateComp(first, firstPay) != 0 {
			remaining = segmentPeriods(loan, first, remaining)
			subFirstDate = first
		}
		// DOS seeds the sub-walk's ACCRUAL anchor separately from its GRID anchor.
		// RepayFancyLoan's prologue (AMORTOP.pas:1147-1157) is:
		//
		//	t := firstdate;
		//	AddPeriod(t, h^.peryr, firstdate.d, subtract);
		//	if (prepaid) then
		//	  begin
		//	    paidthru := firstdate;
		//	    if (not df.c.in_advance) then
		//	      AddPeriod(paidthru, h^.peryr, firstdate.d, subtract);
		//	  end
		//	else
		//	  paidthru := loandate;
		//
		// `t` (base_date, the GRID anchor) is always one period before firstdate,
		// but `paidthru` — the date the first row's interest ACCRUES FROM — is
		// `loandate` (the boundary row) only when the loan is NOT prepaid. When it
		// IS prepaid, loandate is never read at all: paidthru is firstdate itself
		// in advance, else firstdate minus one period. That assignment is
		// UNCONDITIONAL in DOS — it does not depend on whether the sub-loan's grid
		// happens to differ from the caller's next regular date.
		//
		// The port had it nested inside the `first != firstPay` regrid guard, so on
		// any case where the sub-loan's grid coincides with firstPay the prepaid
		// anchor silently never fired and the sub-walk accrued from the boundary
		// row instead. 2026-07-25 fuzzer5 seed 20102:
		//
		//	amort_oracle 298181.50 0.1246260000 120 12 b365 exact prepaid plusreg
		//	  r78 loandmy=27.7.2023 firstdmy=27.8.2023 mor=58 b85=38817.77
		//	  b94=71764.76 pre=4:49:26:265.19 adj=13::5242.43
		//	  adj=45:0.0869900000:6272.01 adj=60::6077.74 targ=1041.21 skip=6
		//	  pts=0.010588
		//
		// The 8/27/24 boundary is followed by an OFF-CYCLE prepayment row on 9/2/24,
		// which is what Re_Amortize hands Iterate as `firstdate` (AMORTOP.pas:1523
		// passes nextpayment.date RAW). So DOS accrues the sub-walk's first row from
		// 8/2/24 — twenty-five days BEFORE the boundary — where the port accrued
		// from 8/27/24. At the identical seed 0.124626 the terminals were DOS
		// p=59272.9726968171 -> 0.0926961862 vs Go p=53980.1860690819 ->
		// 0.0947571798, and the whole-case delta was dInt=+1360.26. Ablating
		// `prepaid` from the oracle line collapsed DOS onto Go's answer EXACTLY
		// (300181.96), which is the signature of a prepaid-only accrual anchor: the
		// port was insensitive to a flag DOS reads here.
		//
		// The in-advance arm takes `nextRow`, not `first`: DOS's `paidthru :=
		// firstdate` is the RAW Iterate argument, not the snapped grid date.
		//
		// And the anchor is expressed by handing the sub-loan LoanDate = paidthru
		// with Prepaid CLEARED (see segSettings below), not by leaving Prepaid set:
		// inside AMORTOP.pas the flag has exactly two readers — PrepaidInterest at
		// :177, which is display-side, and this paidthru assignment at :1151. There
		// is no prepaid behaviour in the walk itself. The port's prepaid walk, by
		// contrast, emits an interest-only settlement stub for LoanDate..naturalStart
		// and leaves the balance untouched across it (generateFancyScheduleMode's
		// Case A), so moving LoanDate back to 8/2/24 with Prepaid still set bought
		// nothing at all — the terminal came out bit-identical (0.0947571798 either
		// way, verified with DPTRACESEG). Clearing the flag makes LoanDate mean what
		// DOS's `paidthru := loandate` else-arm means: the date the first row's
		// interest accrues from, full stop.
		if settings.Prepaid {
			subLoanDate = base
			if settings.InAdvance {
				subLoanDate = nextRow
			}
		}
	}
	segSettings := settings
	segSettings.InAdvance = false
	// See the paidthru discussion above: subLoanDate now CARRIES the prepaid
	// anchor, so the sub-walk must run as a plain (non-prepaid) loan or it would
	// apply the anchor twice — once as a settlement stub, once as prevdate.
	segSettings.Prepaid = false
	sub := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput,
			Amount:       bal,
			// NOT InOutInput: a hard payment triggers DOS's Dav-Holle per-period
			// interest rounding (cent-quantized terminal → the secant can't reach
			// the half-penny tolerance). The MAIN display doesn't round the segment
			// (its regular payment is solved, not user-hard), so the sub-loan must
			// not either — keep the terminal continuous so dosIterate converges.
			PayAmtStatus:   types.InOutDefault,
			PayAmt:         payment,
			NStatus:        types.InOutInput,
			NPeriods:       remaining,
			PerYrStatus:    types.InOutInput,
			PerYr:          loan.PerYr,
			LoanDateStatus: types.InOutInput,
			LoanDate:       subLoanDate,
			FirstStatus:    types.InOutInput,
			FirstDate:      subFirstDate,
		},
		Balloons:     futureBalloons,
		Prepayments:  segmentPre,
		Settings:     segSettings,
		Fancy:        true,
		initUsap:     usap,
		segmentSolve: true,
		// Same two-day model as the AMOUNT branch: segmentGrid already stepped
		// subFirstDate off base with loan.FirstDate's day (AMORTOP.pas:596-598,
		// `AddPeriod(date, h^.peryr, h^.firstdate.d, add)`), but that step can
		// CLAMP — a day-30 screen snapping into February yields 2/28 — and the
		// walk would then re-derive its anchor from the clamped date and run the
		// whole remaining segment two days early. DOS never does: `h` is the
		// OUTER loan record, so every row of the sub-walk is stepped off
		// `h^.firstdate.d` no matter what the individual row dates clamp to.
		//
		// 2026-07-28 fuzzer5 seed 40006 — verified against the real DOS engine:
		//
		//	amort_oracle 402006.49 0.0824990000 288 12 loandmy=30.8.2023 \
		//	  firstdmy=30.10.2023 pre=200:343:52:160.09 pre=11:111:24:143.23 \
		//	  adj=137::4038.64 targ=672.31 payhard=4046.15
		//
		// The 1/30/2035 adjustment supplies an AMOUNT, so DOS solves the implied
		// RATE over a 152-period target-floored segment starting 2/28/2035. Its
		// traced sub-walk runs 2/28/35, 3/30/35, 4/30/35, ...; anchored on the
		// clamped 28 the port ran 2/28/35, 3/28/35, 4/28/35, ... At the identical
		// seed the terminals were DOS 588862.1169 vs Go 577886.4169 — same secant
		// shape, same 18 passes, but the fitted rate came out -0.0072538910
		// against DOS's -0.0115170078, worth dInt=14973.16 on the whole case.
		gridAnchorDay: loan.FirstDate.Time.Day(),
	}
	if anySkip(input.SkipMonths.MonthSet) {
		sub.SkipMonths = input.SkipMonths
	}
	// DOS's EstimateAndRefineAdjRate does NOT solve a bare annuity: it runs
	// RepayFancyLoan over the real screen and Iterates at the adjustment
	// (Amortize.pas:345-366), so every schedule-shaping option that still bites
	// AFTER the adjustment date shapes the rows the implied rate is fitted to —
	// the moratorium's interest-only rows, the target's principal floor, the skip
	// months, the remaining balloons and the remaining prepayment extras.
	// Carrying the balloons/prepayments but dropping the moratorium/target solved
	// a different loan than DOS did. 2026-07-24 fuzzer5 — verified vs the real DOS
	// engine:
	//
	//	amort_oracle 190352.81 0.1136450000 52 4 mor=75 adj=48::7713.08 \
	//	  payhard=9090.79 → int 125732.90 (the moratorium-blind uniform implied
	//	  rate gave 8.810% where DOS fits 3.569%, and the port reported 201696.08)
	if input.Target.TargetStatus >= types.InOutDefault {
		sub.Target = input.Target
	}
	if mor, ok := moratoriumForSegment(input.Moratorium, prevDate); ok {
		sub.Moratorium = mor
	}
	// generateFancyScheduleMode bounds the walk by LastDate; a solver-built
	// sub-loan hasn't run FirstPass, so derive it: FirstDate + (NPeriods-1)
	// periods (same as fancyTerminal). The step must use the SAME anchor the
	// walk itself uses (sub.anchorDayFor), or the bound lands on a date the walk
	// never reaches and truncates it a row short — see fancyTerminal's identical
	// derivation and the seed-40001 note on LoanInput.gridAnchorDay.
	if dateutil.DateOK(sub.Loan.FirstDate) {
		day := sub.anchorDayFor(&sub.Loan)
		last := sub.Loan.FirstDate
		for k := 1; k < sub.Loan.NPeriods; k++ {
			if nd, e := dateutil.AddPeriod(last, sub.Loan.PerYr, day, false); e == nil {
				last = nd
			}
		}
		sub.Loan.LastDate = last
		sub.Loan.LastOK = true
		// §66 (round 25). THE DERIVED BOUND MUST NOT OUTRUN THE PARENT'S
		// h^.lastdate. DOS's rate branch calls
		//
		//	Iterate(p, usap, payment.date, nextpayment.date, h^.loanrate, til_adj)
		//
		// (AMORTOP.pas:1523) and Iterate re-enters RepayFancyLoan — which walks
		// the SAME globals. Nothing on that path assigns h^.lastdate: only the
		// AMOUNT branch recomputes it (:1547), and the pre-pass snap is confined
		// to adjLastDate. So the sub-walk's ComputeNext is still testing every
		// candidate regular date against the PARENT loan's last payment date:
		//
		//	balloonpos := 1;
		//	if (xsource > 0) then begin
		//	  balloonpos := DateComp(nextextra.date, date);
		//	  if (DateComp(date, h^.lastdate) > 0) then
		//	    balloonpos := -1;          {AMORTOP.pas:606 — regular row DROPPED,
		//	                                the pending extra is taken instead}
		//
		// The port hands the sub-loan a LastDate re-derived from `remaining`,
		// which is a port-only reconstruction with no DOS analogue at this site.
		// On c4 it landed one semi-annual period late — 2037-03-31 against the
		// screen's own lastdmy=28.2.2037 — so the terminal walk emitted an
		// ELEVENTH regular payment of 22916.18 that DOS never emits:
		//
		//	amort_oracle 284917.49 0.0671720000 28 2 b365_360 exact prepaid \
		//	  plusreg r78 loandmy=31.7.2023 firstdmy=31.8.2023 mor=73 \
		//	  pre=55:144:12:323.93 adj=103::22916.18 pts=0.005528 \
		//	  payhard=20219.51 non lastdmy=28.2.2037
		//
		// Rows 0-69 of the sub-walk were byte-identical; row 70 was DOS
		// 2037-04-29 pay 323.93 against the port's 2037-03-31 pay 22916.18.
		// One extra payment moved the residual at the SHARED seed from DOS's
		// +2237.0538681843 to the port's -25540.5339966426, and the two secants
		// — which agree step for step, same seed, same brake, same acceptance —
		// then converged on honestly different roots: 0.0646819059 vs 0.0930233351.
		// That is the whole of §66's AO6 divergence; it was never a basin
		// problem and never a solver problem.
		// The bound is the caller's LIVE h^.lastdate (see the doc comment): the
		// pristine loan.LastDate would undo §53, where the snap legitimately
		// pushes the global PAST the last scheduled payment and DOS emits a
		// regular row there.
		//
		// 🚨 ROUND 52 — "CLAMP, NEVER EXTEND" WAS HALF A RULE, AND THE OTHER
		// HALF WAS THE LARGEST OPEN DEFECT FAMILY IN THE PORT.
		//
		// Round 25 closed the LONG side here and declined the SHORT side, on the
		// reasoning that a derived date shorter than h^.lastdate is the port's own
		// reconstruction of a walk DOS bounds by very_last, so lengthening it
		// would re-introduce the extra row from the other side. That reasoning is
		// right about WHY the port must not simply trust its own count — and wrong
		// that leaving the count short is therefore safe. Measured at r52 on the
		// standing arm (seeds 50100-50109, N=400, key `reached`): of the 170
		// in-scope screens that run this solve, the derivation falls SHORT of
		// h^.lastdate on 16, and 14 of those 16 diverge on the solved rate; 22 of
		// the 23 divergent rows have Go BELOW DOS, which is the signature of a
		// sub-walk one row too short. Closing it took the standing baseline from
		// 25 HARD in 2,091 to 12, with paired NEW = 0 over all 2,211 cases.
		//
		// The row-level demonstration, seed 50107 case 165, both engines at the
		// SAME trial rate x = 0.1052470000:
		//
		//	 2034-12-28   DOS pout=151915.523012   GO bal=151915.523012
		//	  2035-3-28   DOS pout=123710.860488   GO bal=123710.860488
		//	  2035-9-28   DOS pout= 98051.288955   GO bal= 98051.288955
		//	  2036-3-28   DOS pout= 71041.420959   GO           ABSENT
		//
		// Byte-identical to the last digit, then DOS emits one more row and the
		// port stops. The port's terminal IS DOS's second-to-last balance. The
		// secants then converge honestly on different roots: DOS 0.0900951249
		// against the port's 0.0824545777.
		//
		// Two bounds, applied in this order, and they are DIFFERENT OBJECTS:
		//
		//   1. very_last CAPS h^.lastdate — the same reason the AMOUNT path caps
		//      segN (engine.go:4473). The snap can push h^.lastdate past very_last
		//      and the extra period is then unreachable, because RepayFancyLoan's
		//      until-clause ends the walk first.
		//   2. THEN the do-while overshoot. AMORTOP.pas:1221's until-clause runs
		//      AFTER ComputeNext, so the first segment row at or past the horizon
		//      is EMITTED and the walk stops after it. The port's LastDate bound
		//      is inclusive-below, so a horizon falling BETWEEN two segment rows
		//      costs a row. The segment grid can be offset from the loan's —
		//      case 165's segment pays on Mar/Sep 28 against a loan on Feb/Aug 28
		//      — so this is not a rare alignment.
		//
		// ⚠️ THE ORDER IS LOAD-BEARING AND WAS MEASURED, NOT REASONED. Capping
		// after the overshoot instead of before gives NEW 0 / FIXED 0 — the cap
		// eats the extra row and the change does nothing at all.
		// ⚠️ AND THE LONG-SIDE CLAMP MUST BE LEFT EXACTLY AS ROUND 25 LEFT IT.
		// A version of this fix that replaced the clamp rather than complementing
		// it reached 7 HARD (1 in 299) and booked NEW = 1: seed 50100 case 272,
		// where the derived bound legitimately outruns h^.lastdate, re-broke
		// precisely the way round 25's comment predicted.
		if dateutil.DateOK(hLastDate) {
			switch derived := sub.Loan.LastDate; {
			case dateutil.DateComp(derived, hLastDate) > 0:
				// §66 (round 25), UNCHANGED.
				sub.Loan.LastDate = hLastDate
			case dateutil.DateComp(derived, hLastDate) < 0:
				bound := hLastDate
				if dateutil.DateOK(veryLast) && dateutil.DateComp(bound, veryLast) > 0 {
					bound = veryLast
				}
				// ⚠️ BOTH LOOPS BELOW ARE CAPPED, AND THE CAP IS NOT DEFENSIVE
				// PROGRAMMING. They rely on AddPeriod advancing monotonically,
				// and AddPeriodFields wraps the year field mod 256 (§55,
				// wrapPascalYear): a date crossing calendar 2156 rolls back to
				// 1900, at which point `DateComp(dt, bound) < 0` can never stop
				// being true and the second loop would raise NPeriods without
				// bound. Measured at r53 over all 17,824 sweep screens AND the
				// whole standing arm, the observed maxima are 176 and 15
				// iterations, so no reachable instance is known — but "no
				// reachable instance is known" is exactly what §73 said about
				// 29 February 2100. r53 audit pass 2, finding N14.
				const maxSegmentPeriods = 100000
				day := sub.anchorDayFor(&sub.Loan)
				dt := sub.Loan.FirstDate
				for i := 0; dateutil.DateComp(dt, bound) < 0; i++ {
					if i >= maxSegmentPeriods {
						break
					}
					nd, e := dateutil.AddPeriod(dt, sub.Loan.PerYr, day, false)
					if e != nil || dateutil.DateComp(nd, dt) <= 0 {
						break // no forward progress: the calendar wrapped.
					}
					dt = nd
				}
				if dateutil.DateComp(dt, bound) > 0 {
					bound = dt
				}
				if dateutil.DateComp(bound, derived) > 0 {
					sub.Loan.LastDate = bound
					// NPeriods is the port's OTHER bound
					// (generateFancyScheduleMode's `payNum >= loan.NPeriods`
					// terminal row). A count that stops the walk before LastDate
					// re-imposes the short bound through the back door, so raise
					// it to span the new horizon. NEVER lower it.
					n := sub.Loan.NPeriods
					dt := derived
					for i := 0; i < maxSegmentPeriods; i++ {
						nd, e := dateutil.AddPeriod(dt, sub.Loan.PerYr, day, false)
						if e != nil || dateutil.DateComp(nd, bound) > 0 ||
							dateutil.DateComp(nd, dt) <= 0 {
							break
						}
						dt = nd
						n++
					}
					sub.Loan.NPeriods = n
				}
			}
		}
	}
	// terminal(rate) = the unforced terminal balance of the segment at the trial
	// rate, paying the fixed new payment — the residual DOS's Iterate drives to
	// zero. Monotone increasing in rate (higher rate ⇒ slower paydown ⇒ higher
	// terminal).
	//
	// `condemned` is DOS's errorflag/overflowflag latch (see the doc comment): once
	// a trial rate makes ComputeTrueRate's lnn fire, DOS is finished — the secant
	// aborts at the top of its next pass and the screen refuses. Sticky, exactly as
	// the DOS globals are: nothing in Iterate or Re_Amortize clears them.
	var condemned bool
	terminal := func(rate float64) float64 {
		s := sub
		s.Loan.LoanRateStatus = types.InOutInput
		s.Loan.LoanRate = rate
		tr, err := ComputeTrueRate(&s.Loan, &segSettings)
		if err != nil {
			condemned = true
		}
		f := GrowthPerPeriod(&s.Loan, segSettings.YrInv)
		res := generateFancyScheduleMode(s, payment, &segSettings, tr, f, true)
		// DPTRACESEGROWS=1 dumps this terminal's own rows so they can be diffed
		// row-for-row against DOS's, which the `-mode cn` trace oracle prints as
		// CN lines (scripts/build_trace_oracle.sh). §66: the two secants agree
		// step-for-step and still land on different roots, because the RESIDUAL
		// FUNCTION differs — which is a per-row question, not a solver question.
		// Gated, so default output stays byte-identical (rule 7).
		if dpTraceSegRows {
			fmt.Fprintf(os.Stderr, "SEGROW0 rate=%.10f rows=%d final=%.6f\n",
				rate, len(res.Schedule), res.FinalPrinc)
			for i, r := range res.Schedule {
				fmt.Fprintf(os.Stderr, "SEGROW %3d d=%s pay=%.6f int=%.6f bal=%.6f\n",
					i, r.Date.Time.Format("2006-1-2"), r.PayAmt, r.Interest, r.Principal)
			}
		}
		return res.FinalPrinc
	}
	abort := func() bool { return condemned }
	// WHICH SEED. DOS's rate branch is (AMORTOP.pas:1520-1531)
	//
	//	d := adj[next_adj]^.amount;
	//	if (adj[next_adj]^.loanratestatus < outp) then
	//	  if Iterate(p, usap, payment.date, nextpayment.date,
	//	             h^.loanrate, til_adj) then ...
	//
	// and `h^.loanrate` is Iterate's `var x` — so DOS ALWAYS seeds the secant at
	// the rate the loan is currently carrying, never at a pre-fitted estimate.
	// That matters because the segment terminal is NOT monotone once balloons and
	// a prepayment series ride on top of a moratorium: the terminal has more than
	// one zero, and which one a secant lands on is decided entirely by where it
	// starts. Seeding at the port's uniform solveAdjRate answer therefore picked a
	// DIFFERENT ROOT than DOS on
	//
	//	amort_oracle 372783.59 0.0899480000 68 4 exact mor=75 targ=504.47 \
	//	  adj=27::14287.90 b78=106325.12 b87=47359.42 pre=30:172:12:389.40 \
	//	  payhard=11565.40
	//	  DOS  interest -367238.69 paid    5544.90   (implied rate ~ -16.7%/yr)
	//	  port interest 2281913.75 paid 2654697.34   (implied rate ~ +13.0%/yr)
	//
	// Both roots retire the loan in 182 rows — they are genuinely two solutions of
	// the same equation, and DOS's is the negative one only because it starts at
	// 8.9948% and steps down. So try DOS's seed first and take it whenever it
	// converges; the uniform fit stays as a FALLBACK, because it is a port-side
	// fast path with no DOS analogue and its only job is to rescue the steep
	// actual-day terminals where a from-scratch secant cannot get started (a big
	// new payment over-amortizes by hundreds of thousands at the original rate).
	//
	// No magnitude clamp on this path. DOS has no post-hoc rejection of a
	// converged rate: its ONLY rate-magnitude rule is the third arm of Iterate's
	// until-clause (AMORTOP.pas:1485), which stops the exploration but still
	// adopts bestx — see dosIterateRateAbort. A fabricated ±1.9 gate here threw
	// away DOS's own answer on screens whose implied segment rate is large and
	// negative (e.g. the seed-9015 AO6 case, where DOS converges to
	// bestx = -1.9403370826 with bestp = 0.00025) and silently fell through to
	// the port-only fallback seed, landing in a different basin.
	if r, ok := dosIterateRateAbort(loan.LoanRate, bal, terminal, abort); ok {
		if dpTraceSeg {
			fmt.Fprintf(os.Stderr, "SEGRATE bal=%.4f pay=%.4f rem=%d seed=%.10f -> %.10f (ld=%v fd=%v nbal=%d npre=%d)\n",
				bal, payment, remaining, loan.LoanRate, r, subLoanDate.Time.Format("2006-01-02"),
				subFirstDate.Time.Format("2006-01-02"), len(futureBalloons), len(segmentPre))
		}
		return r, true, false
	}
	// The latch is checked BEFORE the fallback seed: DOS has no second seed. Once
	// errorflag is up the screen is already condemned, so re-running the secant
	// from anywhere else would be inventing a recovery DOS does not have.
	if condemned {
		return 0, false, true
	}
	if seedRate <= 0 {
		seedRate = loan.LoanRate
	}
	r, ok := dosIterateAbort(seedRate, bal, terminal, abort)
	if condemned {
		return 0, false, true
	}
	// The magnitude gate survives ONLY here, on the port-only fallback seed. That
	// seed has no DOS analogue at all, so there is no DOS behaviour to be faithful
	// to; the gate is just a sanity bound keeping a runaway uniform-fit secant from
	// reporting a nonsense rate. The DOS-seeded path above is deliberately ungated.
	if !ok || r < -1.9 || r > 1.9 {
		return 0, false, false
	}
	return r, true, false
}

// hasFancyOptions reports whether the loan carries any advanced option
// that makes the closed-form backward solve inexact: balloons, prepayment
// series, rate/payment adjustments, AND the schedule-shaping options —
// skip-months, moratorium, and target. DOS's Iterate always drives the
// FANCY terminal when any option is set (skip/mor/target all set `fancy`,
// and RepayFancyLoan's ComputeNext applies them per period —
// AMORTOP.pas:574-664, 1438/1464), so an amount/rate solve that ignores
// them returns the option-blind closed form.
//
// 2026-07-11 audit finding A5: this previously checked only
// prepayments/adjustments/balloons, so skip/mor/target-only loans solved
// up to 75% off. Verified vs the real DOS engine:
//
//	amort_oracle 0 0.12 24 12 noamt pay=888.4879 skip=6-8 → solvedamount 14134.974937
//	amort_oracle 0 0.12 24 12 noamt pay=500 mor=12        → solvedamount 6066.870036
func hasFancyOptions(input LoanInput) bool {
	if !input.Fancy {
		return false
	}
	if len(input.Prepayments) > 0 || len(input.Adjustments) > 0 {
		return true
	}
	if input.SkipMonths.SkipStatus >= types.InOutDefault && input.SkipMonths.SkipStr != "" {
		return true
	}
	if input.Moratorium.FirstRepayStatus >= types.InOutDefault {
		return true
	}
	if input.Target.TargetStatus >= types.InOutDefault {
		return true
	}
	for _, b := range input.Balloons {
		// Presence by STATUS, not value: DOS sets `fancy := true` for any
		// entered balloon row, and a $0 balloon in REPLACE mode is a real
		// skipped installment (pass-2 finding 9).
		if b.AmountStatus >= types.InOutDefault {
			return true
		}
	}
	return false
}
