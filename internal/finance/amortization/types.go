// Package amortization implements loan amortization calculations ported from
// the legacy Delphi/Pascal Amortize.pas and AMORTOP.pas modules.
//
// The amortization engine supports:
//   - Standard and Rule-of-78 amortization
//   - Prepaid interest
//   - Multiple balloon payments
//   - Rate/payment adjustments (ARMs)
//   - Extra/skipped prepayments
//   - Moratorium (interest-only) periods
//   - Targeted principal reduction
//   - Skip-month schedules
//   - Daily, Canadian, and standard compounding
//   - US Rule interest calculations
//   - 360/365/365-360 day count conventions
//
// All monetary values use float64 internally to match the original Pascal real
// type and preserve exact numerical behavior of iterative algorithms.
//
// Ported from legacy/source/Amortize.pas and legacy/source/AMORTOP.pas
package amortization

import (
	"github.com/persense/persense-port/internal/types"
)

const (
	minPmt = 1.0 // minimum meaningful payment amount
	teeny  = types.Teeny
	tiny   = types.Tiny
	small  = types.Small
	// unusuallyHighRate is the nominal Loan Rate (0.20 = 20%) above
	// which a user-entered rate triggers a soft "looks like a typo"
	// warning. DOS shows this only on the mortgage screen
	// (MortgageScreenUnit.pas:222); we extend it to amortization too.
	unusuallyHighRate = 0.20
)

// Field-presence status idiom (read this before the engine's if-blocks).
//
// Every user-facing field on the amortization screen carries a companion
// `…Status int8` set to one of the InOut codes in internal/types/constants.go:
//
//	InOutBad     = -1  data present but unreadable / out of bounds
//	InOutEmpty   =  0  blank — the user left the cell empty
//	InOutOutput  =  1  a value the engine COMPUTED and wrote back
//	InOutDefault =  2  a value defaulted from settings
//	InOutInput   =  3  a value the user TYPED
//
// The codes are ordered so a single threshold test classifies a field. The
// package uses these three comparisons everywhere, and they are the reason the
// dispatch conditions stack up several clauses:
//
//	Status >= types.InOutDefault  → the field HAS a usable value (typed or
//	                                defaulted). Read as "field is present/known."
//	Status <  types.InOutDefault  → the field is blank for dispatch purposes
//	                                (empty, bad, or only a prior COMPUTED output).
//	                                Read as "field was left blank — solve for it."
//	Status == types.InOutInput    → specifically a HARD, user-typed value (drives
//	                                the DOS "Dav Holle" penny-rounding of interest
//	                                and suppresses re-solving).
//	Status == types.InOutOutput   → a value the engine solved (used when echoing
//	                                solved balloons/prepayments back to the UI).
//
// This is the "field-presence dispatch" that CLAUDE.md describes: the user fills
// in some cells and leaves others blank, and the engine reads these statuses to
// decide which quantity to solve for. So a guard like
//
//	loan.PayAmtStatus < InOutDefault && loan.LoanRateStatus >= InOutDefault
//
// means "payment left blank AND rate supplied" → solve the payment.

// Loan holds the top-level amortization loan parameters.
// Mirrors the Pascal AMZLoan record + supporting global state.
//
// Ported from legacy/source/PETYPES.PAS: AMZLoan record
type Loan struct {
	AmountStatus   int8
	Amount         float64 // loan principal
	LoanDateStatus int8
	LoanDate       types.DateRec
	LoanRateStatus int8
	LoanRate       float64 // nominal interest rate (as yield at peryr)
	FirstStatus    int8
	FirstDate      types.DateRec // date of first regular payment
	NStatus        int8
	NPeriods       int // number of regular payment periods
	LastStatus     int8
	LastDate       types.DateRec // date of last regular payment
	PerYrStatus    int8
	PerYr          int // payments per year
	PayAmtStatus   int8
	PayAmt         float64 // regular payment amount
	PointsStatus   int8
	Points         float64 // points charge for APR
	APRStatus      int8
	APR            float64 // computed APR

	LastOK bool // whether last date is valid/computed
}

// BalloonPayment represents a lump-sum payment at a specific date.
// Ported from legacy/source/PETYPES.PAS: balloonrec
type BalloonPayment struct {
	DateStatus   int8
	Date         types.DateRec
	AmountStatus int8
	Amount       float64
}

// RateAdjustment represents a rate or payment change on a specific date.
// Ported from legacy/source/PETYPES.PAS: adjrec
type RateAdjustment struct {
	DateStatus     int8
	Date           types.DateRec
	LoanRateStatus int8
	LoanRate       float64
	AmountStatus   int8
	Amount         float64
	AmtOK          bool // whether amount was user-specified
}

// Prepayment represents a series of extra (or skipped) payments.
// Ported from legacy/source/PETYPES.PAS: prepaymentrec
type Prepayment struct {
	StartDateStatus int8
	StartDate       types.DateRec
	NNStatus        int8
	NN              int // number of extra payments
	StopDateStatus  int8
	StopDate        types.DateRec
	PerYrStatus     int8
	PerYr           int
	PaymentStatus   int8
	Payment         float64 // amount per extra payment (0 = skip)
	NextDate        types.DateRec

	// anchorDay is the DAY-OF-MONTH anchor DOS passes to AddPeriod as
	// `orig_day` when it steps this series' cursor:
	//
	//	AddPeriod(nextdate, pre[i]^.peryr, pre[i]^.startdate.d, add);
	//	                                   ^^^^^^^^^^^^^^^^^^^^  AMORTOP.pas:559
	//
	// i.e. the day of the series' ORIGINAL start date, never the day of the
	// cursor's current position. It matters only for peryr=24 (semi-monthly),
	// where AddPeriod snaps back onto the anchor whenever the stepped day lands
	// within 3 days of it (INTSUTIL.pas AddPeriod, `if abs(day-orig_day) < 4`).
	//
	// A whole prepayment record is normally its own anchor, so anchorDay is left
	// 0 and `originDay()` falls back to StartDate.Day(). It is set only by
	// clipPrepaymentsForSegment, which re-bases StartDate onto the first extra
	// still ahead of a segment boundary: DOS's Re_Amortize restores the series
	// from old_pre (AMORTOP.pas:1552-1557), which carries the ORIGINAL startdate
	// alongside a partially-advanced nextdate, so the anchor must survive the
	// re-base. 2026-07-26 fuzzer5 seed 20201 — see clipPrepaymentsForSegment.
	anchorDay int
}

// originDay is the `orig_day` argument DOS passes to AddPeriod when stepping
// this series (AMORTOP.pas:559). See Prepayment.anchorDay.
func (p *Prepayment) originDay() int {
	if p.anchorDay > 0 {
		return p.anchorDay
	}
	return p.StartDate.Time.Day()
}

// Moratorium represents an interest-only deferment period.
// Ported from legacy/source/PETYPES.PAS: moratoriumrec
type Moratorium struct {
	FirstRepayStatus int8
	FirstRepay       types.DateRec
}

// Target represents a minimum principal reduction per payment.
// Ported from legacy/source/PETYPES.PAS: targetrec
type Target struct {
	TargetStatus int8
	TargetValue  float64
}

// SkipMonths represents months in which payments are skipped.
// Ported from legacy/source/PETYPES.PAS: skiprec
type SkipMonths struct {
	SkipStatus int8
	SkipStr    string   // e.g. "6-8" or "1,6,12"
	MonthSet   [13]bool // parsed: MonthSet[m] = true if month m is skipped
}

// Settings holds the computational settings that affect amortization.
// These replace the global df.c and related variables from Pascal.
type Settings struct {
	Basis       types.BasisType
	PerYr       byte    // default compounding frequency from settings
	Prepaid     bool    // prepaid interest
	InAdvance   bool    // payments in advance
	PlusRegular bool    // balloon includes regular payment
	Exact       bool    // exact interest calculations
	R78         bool    // Rule of 78 amortization
	USARule     bool    // US Rule for interest
	YrDays      float64 // days per year
	YrInv       float64 // 1/yrdays
	CenturyDiv  int
	Daily       bool // daily compounding mode
}

// LoanInput bundles all the data needed to compute an amortization.
type LoanInput struct {
	Loan        Loan
	Balloons    []BalloonPayment
	Adjustments []RateAdjustment
	Prepayments []Prepayment
	Moratorium  Moratorium
	Target      Target
	SkipMonths  SkipMonths
	Settings    Settings
	Fancy       bool // whether advanced (fancy) mode is active

	// inBackwardSolve marks that this input is a trial evaluation made INSIDE a
	// production backward solver (SolveLoanAmount, SolveRate, SolveBalloonAmount,
	// SolvePrepaymentAmount). Those solvers were validated against the piecewise
	// forward schedule, so their inner Amortize calls must stay on it —
	// dosPortCanHandle checks this and declines the faithful port. Threaded on the
	// input (per-call) rather than a package global so it is goroutine-safe: the web
	// server runs one goroutine per request, and a shared flag raced (a solve on one
	// request could flip the engine another request selected). It propagates for
	// free because every internal trial Amortize runs on a copy/clone of the
	// solver's input. Unexported: set only by the solvers, never by API callers.
	inBackwardSolve bool

	// entireWalk marks a forward walk that DOS runs with its `entire` parameter
	// TRUE *and* value_calc FALSE and Output=nil. There is exactly one such call
	// site: EstimateAndRefineBalloon's very_last probe,
	//	RepayFancyLoan(p,usap,h^.loandate,h^.firstdate,nil,false,entire,
	//	               no_value_calc,0)                     (Amortize.pas:637)
	// which is what computes a tacked-on terminating balloon.
	//
	// Under those flags DOS's residual fold (AMORTOP.pas:1209-1213)
	//	if ((not h^.lastok) or (entire)) and (WhenToStop^.principal < minpmt)
	//	   and (not value_calc) then begin
	//	     WhenToStop^.payamt := WhenToStop^.payamt + WhenToStop^.principal;
	//	     WhenToStop^.principal := 0;
	//	   end;
	// fires on every row once the balance goes below minpmt. The fold is
	// one-sided (`< minpmt`), so it also fires for arbitrarily NEGATIVE
	// balances, and it rewrites only the reported paymentrec — the var-parameter
	// balance `p` inside RepayFancyLoan keeps marching negative.
	//
	// On its own the fold is invisible here (the probe reads
	// `payamt + principal`, which the fold leaves invariant). It becomes
	// observable through Re_Amortize, whose FIRST statement is
	//	p := Payment.principal;                            (AMORTOP.pas:1508)
	// i.e. a rate/amount adjustment restarts the walk from the previous row's
	// FOLDED (zeroed) principal, not from the accumulated negative balance.
	// (Re_Amortize's tail — NextPayment := Payment; NextPayment.ComputeNext(p,
	// usap); p := NextPayment.principal, AMORTOP.pas:1604-1611 — then recomputes
	// the crossing row at the new rate, which Go already matches.)
	//
	// Measured on the seed-9001 fuzzer5 case
	//	384606.73 0.1398410000 96 12 b365 exact plusreg mor=21 b27=105457.32 \
	//	  b47=71166.50 pre=61:12:12:827.91 adj=76:0.1440280000:6453.09 \
	//	  targ=302.03 skip=2,8,11 payhard=8891.04
	// DOS tacks on -108759.76; without the fold-derived reset Go produced
	// -266799.99.
	//
	// Threaded on the input rather than as another positional parameter on
	// generateFancyScheduleMode (8 call sites), and kept distinct from
	// `unforced` because Iterate's terminals also pass Output=nil but with
	// entire=FALSE and must NOT fold. Unexported: set only by tackOnFinalBalloon.
	entireWalk bool

	// inAdjPrePass marks a walk that stands in for DOS's adjustment PRE-PASSES —
	// the bounded `EstimateAndRefineAdjPayment(i)` walks Amortize.pas:1408-1419
	// runs, one per blank-amount adjustment, BEFORE the table is drawn:
	//
	//	for i:=1 to nadj do
	//	  if (adj[i]^.loanratestatus >= defp) and (adj[i]^.amountstatus < defp) then
	//	    EstimateAndRefineAdjPayment(i);      {-> RepayFancyLoan(...,til_adj,adjnum:=i)}
	//	if (hard_payment) and (fancy) then begin
	//	  for i:=1 to nadj do Round2(adj[i]^.amount);      {the Dav Holle provision}
	//	  for i:=1 to nballoons do Round2(balloon[i]^.amount);
	//	  end;
	//
	// The ORDER is the whole point. Each pre-pass carries the EARLIER
	// adjustments' amounts as Re_Amortize left them — raw secant roots, NOT
	// rounded — because the Round2 sweep fires exactly once, after every
	// pre-pass has finished. Only the display walk (and the later APR value
	// walk) sees the rounded values, which it re-reads through
	// `if (adj[next_adj]^.amtok) then d := adj[...]^.amount` (AMORTOP.pas:1514).
	//
	// The port used to solve each adjustment INLINE during the display walk and
	// round it on the spot, so adjustment k+1 was solved from a rounded-k
	// balance path. 2026-07-27 fuzzer5 seed 20431 (dInt=-23.64):
	//
	//	46143.57 0.0330330000 56 4 b365 exact loandmy=14.7.2025 firstdmy=14.8.2025 \
	//	  mor=25 b58=4291.65 pre=19:201:52:16.02 pre=4:69:12:33.49 \
	//	  adj=22:0.0710580000: adj=76:0.0635840000: targ=152.13 payhard=917.16
	//
	// DOS's own traced passes show the mechanism directly. Pass 2 (adj2's
	// pre-pass) carries d1 = -3073.5167442105; pass 3 (display) carries
	// Round2(d1) = -3073.52, and the two walks first part company at the very
	// next row:
	//
	//	p2 47 CN 8/14/127 int=63.190000 pay=3322.326744 p=50834.243256
	//	p3 47 CN 8/14/127 int=63.190000 pay=3322.330000 p=50834.240000
	//
	// A third of a cent per collision row compounds to 5.88 cents of boundary
	// balance at adj2 (DOS solves from 4563.908837; the port fed it
	// 4563.850000). That is not a penny-scale cosmetic gap: `targ=152.13` makes
	// the segment terminal FLAT there, so the 5.88 cents moved the root off the
	// target-floored plateau, Newton stalled at bestp=0.0499999996, the solve
	// was REJECTED, and the port silently kept the raw annuity seed 192.457478
	// instead of DOS's 154.50 — 23.64 of interest and one whole row.
	//
	// So the pre-pass is modelled honestly: one walk with the inline Round2
	// suppressed solves every blank adjustment along the unrounded path (the
	// k-th such solve sees exactly what DOS's k-th pre-pass sees, since
	// pre-pass k reuses adjustments 1..k-1 unrounded from storage), then the
	// Round2 sweep is applied once to the harvested amounts, then the real walk
	// consumes them through the AmtOK path. Unexported: set only by
	// runAdjustmentPrePass.
	inAdjPrePass bool

	// initUsap seeds the USA-rule exempt-principal accumulator (`usap`) at the
	// start of a fancy walk instead of the usual 0.
	//
	// DOS never re-zeroes usap for a SEGMENT solve. Re_Amortize's Iterate call is
	//
	//	if Iterate(p, usap, Payment.date, t, d, til_adj) then ...   (AMORTOP.pas:1577)
	//
	// where `usap` is the unit-level global holding the accumulator as of the
	// adjustment row; Iterate saves it as `initusa` (:1436) and restores it before
	// EVERY trial walk (:1457) so each trial re-runs the segment from the SAME
	// live accumulator. The port models that bounded segment as a standalone
	// sub-loan (solveSegmentPayment / solveSegmentRate), and a standalone loan
	// starts with usap = 0 — so the trial rows charged interest on `p - 0` where
	// DOS charges it on `p - usap`, and the solved segment payment came out high.
	//
	// Only observable when usap is non-zero AT the adjustment, which needs a row
	// whose payment did not cover its interest — a skip, a moratorium or a target
	// floor. 2026-07-25 fuzzer5 (r78/usa widening) — verified vs the real DOS
	// engine:
	//
	//	amort_oracle 495178.90 0.1032190000 216 12 usa \
	//	  adj=70:0.0369020000: skip=2,8,11 b126=114024.00
	//	  → DOS re-amortizes to 4123.7495; with usap dropped the port solved
	//	    4123.93 and finished 5.02 of interest low.
	//
	// Unexported: set only by the segment solvers.
	initUsap float64

	// segmentSolve marks a synthesised MID-LOAN segment sub-loan — the bounded
	// [adjustment -> lastdate] slice that solveSegmentPayment / solveSegmentRate
	// hand to Iterate. It exists for exactly one reason: DOS's `fancy` is a
	// SCREEN-level global, and Iterate dispatches on it (AMORTOP.pas:1436-1439)
	//
	//	if (fancy) or ((df.c.exact) and (df.c.basis<>x360)) then
	//	  RepayFancyLoan(p, usap, loandate, firstdate, nil, false, til_adj,
	//	                 no_value_calc, 0)
	//	else
	//	  RepayLoan(p);
	//
	// Re_Amortize only ever runs on a fancy screen, so its Iterate call ALWAYS
	// takes the RepayFancyLoan arm — even when every balloon and prepayment lies
	// BEHIND the boundary and the remaining segment is, in shape, a plain
	// annuity. The port's sub-loan is synthesised from only what lies AHEAD, so
	// such a segment looks option-free and paymentTerminal routed it to
	// repayExactTerminal (the RepayLoan recursion) instead.
	//
	// The two walks are NOT interchangeable on a negative balance. RepayLoan
	// carries an explicit overpayment guard (AMORTOP.pas:1286-1289)
	//
	//	for i := 2 to h^.nperiods do
	//	  if (p < 0) then p := p - d      {no interest on a credit balance}
	//	  else p := p * f - d;
	//
	// but ComputeNext, which is what RepayFancyLoan actually walks, has NO such
	// test — it charges `interest := h^.loanrate * timedif * (p - usap)`
	// unconditionally (AMORTOP.pas:636) and lets a negative balance accrue
	// negative interest. A segment whose balance is negative at the adjustment
	// therefore has a terminal of slope -(f^n-1)/(f-1) under DOS and slope -n
	// under the plain recursion — a completely different function, with a
	// different root.
	//
	// 2026-07-27 fuzzer5 rotation cycle 42, amortization seed 20509 — verified
	// against the real DOS engine and an instrumented trace build:
	//
	//	amort_oracle 456231.15 0.0405700000 46 2 b365 prepaid plusreg r78 \
	//	  loandmy=18.11.2023 firstdmy=18.5.2024 b108=115047.97 b138=115795.12 \
	//	  pre=60:289:26:82.53 adj=132:0.1469070000:20832.28 \
	//	  adj=204:0.0202030000: pts=0.029925 payhard=18781.28
	//
	// At the 11/18/2040 rate-only adjustment the balance is -525,389.41 and both
	// engines derive the IDENTICAL analytic seed d = -46,710.1541807200. DOS's
	// Iterate probes it once, finds |p| < halfpenny and takes the zeroth-probe
	// early exit (`if (abs(p) < halfpenny) then goto 1`, AMORTOP.pas:1443) —
	// keeping the seed. The port's option-free terminal returned +35,132.44 at
	// that same seed (its walk never accrued: term(0) came back as exactly the
	// opening -525,389.41 rather than -592,736.26 = bal * f^12), so the early
	// exit could not fire and the secant refined to -43,782.4508. The 2,927.70
	// per-payment error moved the APR value stream's first evaluation from DOS's
	// 391,612.224126 to 392,488.142011, which steered DOS's secant off its
	// overflow path: DOS REFUSED the screen and Go emitted a 192-row schedule.
	//
	// Unexported: set only by the segment solvers.
	segmentSolve bool

	// gridAnchorDay overrides the day-of-month anchor the schedule walk feeds to
	// dateutil.AddPeriod as `origDay`. Everywhere else the port derives that
	// anchor as `loan.FirstDate.Time.Day()`, which is right because DOS derives
	// it the same way. The two part company for a SEGMENT sub-loan, and the
	// reason is worth stating precisely, because DOS uses TWO different days
	// there and it is easy to pick the wrong one.
	//
	// DOS's adjustment solve (AMORTOP.pas:1547-1590) calls
	//
	//	n := NumberOfInstallments(h^.firstdate, t, h^.peryr, on_or_after);
	//
	// purely for the VAR side effect on `t`, then hands that `t` to Iterate.
	// NumberOfInstallments's monthly branch ends (INTSUTIL.pas:1013) with
	//
	//	if (flast) then l.d:=daysinm(l) else l.d:=f.d;
	//
	// — assigned with NO clamp, so `t` can be a PHANTOM daterec that no calendar
	// can hold, e.g. 30/2/2031 (the non-flast branch, copying f.d verbatim) or
	// 31/5/2029 (the flast branch, taking the target month's own last day).
	//
	// RepayFancyLoan then derives base_date from that phantom's OWN day
	// (AMORTOP.pas:1149-1150):
	//
	//	t := firstdate;  { the phantom }
	//	AddPeriod(t, h^.peryr, firstdate.d, subtract);
	//
	// but Paymenttype.ComputeNext steps EVERY row off base_date with the
	// SCREEN's first-payment day (AMORTOP.pas:596-598):
	//
	//	date := base_date;
	//	AddPeriod(date, h^.peryr, h^.firstdate.d, add);
	//
	// `h` is the OUTER loan record. So the model is: base = AddPeriod(phantom,
	// peryr, phantom.d, subtract); row_k = AddPeriod(base, peryr,
	// h^.firstdate.d, add) — the phantom's day is used exactly once, for the
	// backward step, and the grid anchor is always the screen's first-payment
	// day. gridAnchorDay carries the latter; fancybisect.go's subBase/row1
	// derivation carries the former.
	//
	// Two fuzzer5 seeds pin this down, and only together — on the first the two
	// days coincide, so it cannot distinguish the rules:
	//
	// 2026-07-28 seed 40001 — phantom 30/2/2031 (non-flast: f.d copied), and
	// h^.firstdate.d is also 30, so both candidate rules give 30:
	//
	//	amort_oracle 115239.23 0.0367770000 132 12 b365_360 exact \
	//	  loandmy=30.6.2025 firstdmy=30.7.2025 adj=67:0.0637620000:
	//
	// DOS's traced sub-walk runs 2/28/31, 3/30/31, 4/30/31, ... 6/30/36; with
	// the anchor read off the CLAMPED FirstDate (28) the port ran 2/28/31,
	// 3/28/31, ... 6/28/36 — same 65 rows and the same secant, but ~2 days less
	// accrual per row: the terminal came out a constant 28.87 low (DOS
	// T(seed)=200.4429 vs the port's 171.5771, identical slope -77.599) and the
	// refined segment payment landed at 1144.3926 against DOS's 1144.7645.
	//
	// 2026-07-28 seed 40002 — phantom 31/5/2029 (flast: h^.firstdate 30/11/2024
	// IS the last day of November, so l.d := daysinm(May) = 31) while
	// h^.firstdate.d is 30. Here the rules disagree:
	//
	//	amort_oracle 100000 0.0428580000 30 2 loandmy=31.1.2024 \
	//	  firstdmy=30.11.2024 adj=58:0.0237140000: pre=112:59:24:160.93
	//
	// The oracle's CN2 trace shows base 11/30/2028 (the 31 clamped by the
	// backward step) and then rows 5/30/2029, 11/30/2029, 5/30/2030, ... — day
	// 30, i.e. h^.firstdate.d, NOT the phantom's 31. Anchoring on 31 collapsed
	// the 5/31 regular payment into the 5/31 prepayment row, losing a 4334.48
	// payment and putting the terminal at -5617.83 against DOS's -10618.30.
	//
	// Zero means "no override" — derive the anchor from FirstDate as usual. In
	// the ordinary case where the sub-loan's FirstDate is not clamped the
	// override equals FirstDate's own day, so setting it is a no-op.
	//
	// Unexported: set only by the segment solvers.
	gridAnchorDay int
}

// anchorDayFor returns the day-of-month anchor for a schedule walk over `loan`:
// the gridAnchorDay override when a segment sub-loan carries DOS's phantom
// snapped day, otherwise the first payment date's own day. See
// LoanInput.gridAnchorDay.
func (in *LoanInput) anchorDayFor(loan *Loan) int {
	if in.gridAnchorDay > 0 {
		return in.gridAnchorDay
	}
	return loan.FirstDate.Time.Day()
}

// PaymentRecord represents one line of an amortization schedule.
type PaymentRecord struct {
	PayNum    int
	Date      types.DateRec
	PayAmt    float64 // total payment this period (incl. extras)
	Interest  float64 // interest portion
	Principal float64 // remaining principal after this payment
	IntToDate float64 // cumulative interest to date
}

// AmortResult holds the full output of an amortization calculation.
//
// The {NPeriods, FirstDate, LastDate} triple reflects what the engine
// ended up using — either as supplied by the caller or derived by
// FirstPass from the other two. Surfaced so API callers can echo
// computed values back to the UI when the user left a field blank
// (e.g. Help/Amortization Example 1c: supply first + last dates,
// engine returns the derived term).
type AmortResult struct {
	Schedule     []PaymentRecord
	FinalPrinc   float64 // final remaining principal (should be ~0)
	TotalPaid    float64 // sum of all payments
	TotalInt     float64 // sum of all interest
	APR          float64 // computed APR (if points specified)
	APRConverged bool
	NPeriods     int           // post-FirstPass term, derived if input was blank
	FirstDate    types.DateRec // post-FirstPass first payment date
	LastDate     types.DateRec // post-FirstPass last regular payment date
	// Warnings carries non-fatal advisories (e.g. the loan retired
	// before its scheduled term). Empty on a plain run.
	Warnings []string
	// Balloons echoes the balloons the engine actually used, including any
	// "target" balloon whose amount it solved (Solved=true), so the UI can
	// fill the blank Amount cell with the computed value.
	Balloons []ResolvedBalloon
	// SolvedPrepay is the per-payment amount the engine solved for an "unknown
	// prepayment" series (AO9 — a series with a count but a blank amount). Zero
	// when no prepayment amount was solved.
	SolvedPrepay float64
	Err          error

	// rawSettlement is the UNROUNDED interest of the loan-date settlement stub
	// (row 0), recorded by the schedule generators before any hard-payment
	// Round2. applyPointsSettlement needs it because DOS rounds the settlement
	// line as ONE combined sum — PrepaidInterest + points*amount — with a single
	// Round2 (Amortize.pas:1482-1483); rounding the stub and the points charge
	// separately loses a cent whenever the two sub-half-cent fractions add
	// across the boundary. hasRawSettlement guards zero-value ambiguity.
	rawSettlement    float64
	hasRawSettlement bool
}

// ResolvedBalloon reports a balloon's date and the amount the engine used.
// Solved is true when the amount was computed by the engine (a date-only
// "target" balloon) rather than supplied by the caller.
type ResolvedBalloon struct {
	Date   types.DateRec
	Amount float64
	Solved bool
	// TackedOn marks DOS's TackOnFinalBalloon row (Amortize.pas:1040-1088): an
	// over-specified loan's implied TERMINATING balloon, painted into the Balloon
	// Payments grid as an output cell (datestatus = amountstatus = outp) but
	// de-activated with dec(nballoons) so it takes no part in the payment table
	// or the APR. BalloonValues2Grid (AmortizationScreenUnit.pas:1691-1713) walks
	// the raw balloon array 1..maxballoon and ignores nballoons entirely, which
	// is why DOS shows a balloon the payment schedule never charges.
	TackedOn bool
}

// --- Zero/Empty functions ---

// ZeroLoan initializes a Loan to empty/zero.
// Ported from legacy/source/Amortize.pas: procedure ZeroAMZLoan
func ZeroLoan(l *Loan) {
	*l = Loan{
		LoanDate:  types.UnknownDate(),
		FirstDate: types.UnknownDate(),
		LastDate:  types.UnknownDate(),
	}
}

// ZeroBalloon initializes a BalloonPayment to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroBalloon
func ZeroBalloon(b *BalloonPayment) {
	*b = BalloonPayment{Date: types.UnknownDate()}
}

// BalloonIsEmpty returns true if the balloon has no data.
// Ported from legacy/source/Amortize.pas: function BalloonIsEmpty
func BalloonIsEmpty(b *BalloonPayment) bool {
	return b.DateStatus == types.StatusEmpty && b.AmountStatus == types.StatusEmpty
}

// ZeroAdjustment initializes a RateAdjustment to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroAdjustment
func ZeroAdjustment(a *RateAdjustment) {
	*a = RateAdjustment{Date: types.UnknownDate()}
}

// AdjustmentIsEmpty returns true if the adjustment has no data.
// Ported from legacy/source/Amortize.pas: function AdjustmentIsEmpty
func AdjustmentIsEmpty(a *RateAdjustment) bool {
	return a.DateStatus == types.StatusEmpty &&
		a.LoanRateStatus == types.StatusEmpty &&
		a.AmountStatus == types.StatusEmpty
}

// ZeroPrepayment initializes a Prepayment to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroPrepayment
func ZeroPrepayment(p *Prepayment) {
	*p = Prepayment{
		StartDate: types.UnknownDate(),
		StopDate:  types.UnknownDate(),
		NextDate:  types.UnknownDate(),
	}
}

// PrepaymentIsEmpty returns true if the prepayment has no data.
// Ported from legacy/source/Amortize.pas: function PrepaymentIsEmpty
func PrepaymentIsEmpty(p *Prepayment) bool {
	return p.StartDateStatus == types.StatusEmpty &&
		p.NNStatus == types.StatusEmpty &&
		p.StopDateStatus == types.StatusEmpty &&
		p.PerYrStatus == types.StatusEmpty &&
		p.PaymentStatus == types.StatusEmpty
}

// ZeroMoratorium initializes a Moratorium to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroMoratorium
func ZeroMoratorium(m *Moratorium) {
	*m = Moratorium{FirstRepay: types.UnknownDate()}
}

// ZeroTarget initializes a Target to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroTarget
func ZeroTarget(t *Target) {
	*t = Target{}
}

// ZeroSkipMonths initializes a SkipMonths to empty.
// Ported from legacy/source/Amortize.pas: procedure ZeroSkip
func ZeroSkipMonths(s *SkipMonths) {
	*s = SkipMonths{}
}
