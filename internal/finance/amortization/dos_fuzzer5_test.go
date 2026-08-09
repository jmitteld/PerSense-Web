package amortization

// TestDOSFuzzer5AllAdvancedOptions — the ALL-OPTIONS-AT-ONCE differential fuzzer
// for advanced (fancy) amortization.
//
// Every prior fuzzer varies ONE advanced option at a time (dos_fuzzer3_fancy)
// or a small hand-picked cube of two or three (dos_option_cube_fuzz). Neither
// exercises the screen the client actually fills in: a loan carrying balloons
// AND prepayments AND rate/payment adjustments AND a moratorium AND a principal
// minimum AND skip months AND points, all live simultaneously, with MULTIPLE
// rows in each row-based block. That is where the §45/§46 terminating-balloon
// bug lived, and it is the configuration with the least coverage.
//
// GENERATION RULE (the client's spec): every advanced-option BLOCK is drawn
// independently and included with probability 0.85 — i.e. each option has a 15%
// chance of being absent from a given case. Row-based blocks (balloons,
// prepayments, rate adjustments) get MULTIPLE rows when present. The
// COMPUTATIONAL settings (basis, exact, prepaid, in-advance, plus-regular) are
// not "options" in that sense — they are always-present booleans — so they are
// drawn uniformly instead, which keeps every option combination exercised
// across the whole settings cube rather than concentrating on the defaults.
//
// ROW LIMITS come from the DOS source, not from taste:
//
//	PETYPES.PAS:378-380   maxprepay = 2;  maxadj = maxlines;  maxballoon = maxlines
//
// maxprepay is a HARD 2 and the oracle's SetupPrepayments has no bounds check
// (amort_oracle.pas:296-425 does `inc(k)` then writes `pre[k]^`), so a third
// prepayment token would corrupt the oracle's heap rather than fail cleanly.
// Balloons and adjustments are capped at 3 here — well under maxlines (>= 18) —
// because past three rows the cases stop finding new dispatch paths and just
// cost oracle spawns.
//
// PARITY RULES that keep the two engines seeing byte-identical input:
//
//   - Option dates land on payment dates: every option month is a multiple of
//     mPer = 12/perYr. `pre=`, `b<N>=`, `adj=`, `mor=` and the Go DateRecs all
//     derive from the same addMonths grid off the shared 2024-01-01 loan date.
//   - Option months are DISTINCT across all blocks (a `used` set). Two options
//     landing on the same date is a real DOS behaviour, but the collision-order
//     semantics are a separate open item (§40d), so this fuzzer keeps them apart
//     rather than re-reporting a known gap on every case.
//   - `skip=` is month-of-year based, so it is only drawn at perYr == 12.
//   - `plusreg` maps 1:1 onto Settings.PlusRegular. SetupLoan defaults
//     df.c.plus_regular := false (amort_oracle.pas:91) and SetupBalloons resets
//     it to false (:185), but the `plusreg` token is parsed LATER (:735), so the
//     token — and only the token — decides. There is no hidden default to match.
//   - in.Fancy mirrors the oracle's `fancy := true`, which each Setup* routine
//     sets for balloons/prepayments/adjustments/mor/targ/skip — but which `pts=`
//     does NOT set. A points-only case therefore runs the PLAIN engine on both
//     sides. See the fancyOpt comment in the body for why this matters.
//
// PAYMENT MODE is drawn 50/50 between SOLVED (blank payment; oracle solves it)
// and GIVEN (`payhard=` / types.InOutInput at 0.6x-1.6x the fair payment).
// The GIVEN arm is not decoration: TackOnFinalBalloon's gate requires
// PayAmtStatus >= defp (Amortize.pas:1043), so DOS's terminating balloon can
// NEVER fire under a solved payment. Half the cases exist to reach that code.
//
// SIGNALS, in order of how much they mean:
//
//	total interest / total paid  — the two scalars defined for every option
//	  combination. Per-row or "the regular payment" comparisons are meaningless
//	  here: a moratorium row is interest-only, a prepay row is inflated, an
//	  adjustment row is a different payment entirely, and a balloon row can be
//	  negative. One `bdump` spawn yields both (bdump prints before the totals
//	  line and does not Halt, amort_oracle.pas:910-931).
//	terminating balloon        — the `bdump` grid rows with dstatus/astatus 1
//	  (outp) versus Go's ResolvedBalloon.TackedOn echo. This is the §46 row.
//	last date / nperiods       — DOS's post-FirstPass horizon versus Go's.
//
// One-directional HARD rule, as in fuzzer3-fancy: Go must not produce a
// schedule where DOS REFUSES the screen. Date-horizon breakdowns ("Julian",
// "bad date", "last payment not found") are indeterminate, not refusals.
//
// This is a DISCOVERY fuzzer over a combination space that is known to still
// diverge, so — like TestDOSOptionCubeFuzz — it is opt-in (PERSENSE_FUZZ=1) and
// reports by option SIGNATURE, so a failure names the block combination to walk
// in the Pascal rather than a single anonymous seed.

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/dateutil"
	"github.com/persense/persense-port/internal/types"
)

// fz5MaxPrepay mirrors PETYPES.PAS:378 `maxprepay = 2`. Exceeding it overruns
// the oracle's pre[] array.
const fz5MaxPrepay = 2

// ---- THE GENERATOR'S SAMPLE-SPACE ENVELOPE (round 16b audit) ----
//
// Every scalar bound the case generator draws from, as NAMED constants so the
// envelope is a diffable, assertable fact rather than folklore spread over 600
// lines. zzsamplespace_test.go asserts a manifest derived from these; changing
// one here without updating the manifest there is a FAILING TEST, which is the
// point — the envelope has narrowed silently twice (`years := 8+Intn(18)` hid
// everything past 25 years until §52; ppy ∈ {12,24,26,52} hid slow prepayment
// series until round 9) and each time the project quoted a convergence number
// over a space it believed was bigger than it was.
const (
	// Term, drawn in YEARS (stratified; see the draw for band weights).
	fz5YearsBand1Lo, fz5YearsBand1W = 8, 18    // 8..25y  — the original band
	fz5YearsBand2Lo, fz5YearsBand2W = 26, 35   // 26..60y
	fz5YearsBand3Lo, fz5YearsBand3W = 61, 60   // 61..120y
	fz5YearsBand4Lo, fz5YearsBand4W = 121, 180 // 121..300y
	fz5NCap                         = 4000     // explicit row cap (comment at the draw)

	// Principal and rate.
	fz5AmountLo, fz5AmountSpan = 25000.0, 475000.0 // $25k .. <$500k
	fz5RateLo, fz5RateSpan     = 0.03, 0.11        // 3% .. <14%

	// Hardened payment, as a multiple of the closed-form fair payment.
	fz5PayFracLo, fz5PayFracSpan = 0.85, 0.5 // 0.85x .. <1.35x fair

	// Loan-date origin.
	fz5LoanYearLo, fz5LoanYearN = 2023, 3 // 2023..2025

	// Option-block scalar bounds.
	fz5PointsSpan                    = 0.04 // 0 .. <4%
	fz5BalloonFracLo, fz5BalloonSpan = 0.02, 0.28
	fz5BalloonBudgetFrac             = 0.60
	fz5PreAmtFracLo, fz5PreAmtSpan   = 0.03, 0.22
	fz5PreNNCap                      = 400
	// ROUND 36 — THE AXIS THAT HAS NOW PAID THREE TIMES.
	//
	// `startK` was drawn from 1..n*7/10, so a prepayment series STARTING PAST
	// THE ENTERED TERM had probability ZERO. The sample-space audit named that
	// region SILENT on 2026-08-02; §71's non-termination lived there (round 34),
	// and §72's located divergences live there too (round 35). Rule 8's question
	// is now 10 for 10, and R31 says to widen the generator once in the direction
	// of the most recent defect. This is that widening.
	//
	// One draw in fz5PreLateStartOdds starts the series PAST the last scheduled
	// payment, up to fz5PreLateStartMaxMul times the term beyond it, and gives it
	// its own nn draw (the in-term nn cap is meaningless once remMonths < 0).
	//
	// ⚠️ THIS CHANGES THE DRAW STREAM, SO IT CHANGES THE POPULATION. Every
	// standing figure measured on dos_fuzzer5 before this commit — the 475 in
	// 34,967, the contingency table, the faithful port's in-scope zero — was
	// measured over a strictly narrower generator and must be RE-MEASURED, not
	// carried (rule 11).
	//
	// ⚠️ AND IT DID NOT GET WORSE, WHICH IS ITSELF A RESULT. An earlier version
	// of this comment said "expect the headline to get worse; that is the point."
	// Measured head to head at 25 seeds x N=800, the in-scope HARD rate went
	// 1.458% (pristine) -> 1.069% (widened), a ~2.8 sigma IMPROVEMENT, because
	// the late arm moves cases into the OUT-OF-SCOPE bucket (out-of-scope
	// compared +25%) rather than adding in-scope divergences. The widening buys
	// coverage the client has already said is not required, and the divergent
	// case sets before and after are DISJOINT. Do not read the improvement as the
	// port getting better. (Round-36 audit.)
	fz5PreLateStartOdds   = 8   // 1 in 8 prepayment series starts past the term
	fz5PreLateStartMaxMul = 2   // ... by up to 2x the term's period count
	fz5PreLateNNCap       = 300 // its own nn cap: the in-term one cannot apply
	// ⚠️ A MONTH CEILING, BECAUSE THE FIRST CUT WRAPPED. startK = n+1+Intn(n*2)
	// with n up to fz5NCap (4000) produces month offsets up to ~144,000;
	// fz5AddMonths truncates the year mod 256 (faithfully — both engines see the
	// same wrapped date), so 18% of the late arm's draws landed BEFORE the loan
	// date: a shape nobody has adjudicated, and not the shape this axis was
	// widened for. 2,400 months is 200 years — past every horizon in the
	// population and inside the year byte. (Round-36 audit.)
	fz5PreLateMonthCap             = 2400
	fz5AdjRateLo, fz5AdjRateSpan   = 0.02, 0.13
	fz5AdjPayFracLo, fz5AdjPaySpan = 0.75, 0.7
	fz5TargFracLo, fz5TargSpan     = 0.02, 0.23
)

// fz5DrawYears is the stratified term draw, in years — extracted so its reach
// and weights are unit-testable (zzsamplespace_test.go). 60% of draws stay in
// the original 8-25y band to preserve corpus density; the tail reaches the
// three long-horizon regions (see the block comment at the call site).
func fz5DrawYears(rng *rand.Rand) int {
	switch k := rng.Intn(10); {
	case k < 6:
		return fz5YearsBand1Lo + rng.Intn(fz5YearsBand1W)
	case k < 8:
		return fz5YearsBand2Lo + rng.Intn(fz5YearsBand2W)
	case k < 9:
		return fz5YearsBand3Lo + rng.Intn(fz5YearsBand3W)
	default:
		return fz5YearsBand4Lo + rng.Intn(fz5YearsBand4W)
	}
}

// fz5Outcome classifies one oracle spawn.
//
// fz5NonConverge is kept apart from fz5Refused on purpose. "Computation of
// payment amount or interest rate did not converge" is DOS's Iterate giving up
// (AMORTOP.pas), NOT the screen being rejected as ill-formed — the port
// legitimately finds roots DOS's bisection misses, which is why fuzzer3-fancy
// downgrades that same case to a log when Go's schedule actually retires the
// loan. Folding it into "refused" would turn a known solver-fidelity gap into a
// hard failure on nearly every stacked-option case.
const (
	fz5Solved = iota
	fz5Refused
	fz5Flake
	fz5DateHorizon
	fz5NonConverge
	// fz5NoTotals — DOS produced a schedule grid and a payment but declined to
	// report interest/paid (both come back as the -1 sentinel) while its horizon
	// cells stayed VALID. Distinct from fz5DateHorizon, which is the same refusal
	// with a wrapped lastdate / nperiods 0. Round 16 (R8): both were previously
	// binned as "flake" and retried eight times. See runDump.
	fz5NoTotals
	// fz5OracleTimeout — HARNESS DEFECT #9 (round 17). The DOS engine does not
	// terminate on some screens: it enters a loop that writes
	// "ENGINE ERROR: Bad date passed to Julian function: m=-99" to stdout
	// forever. Reproduced deterministically at
	//
	//	amort_oracle 236979.58 0.1082040000 940 4 b365 exact prepaid inadv \
	//	  plusreg r78 usa loandmy=12.12.2023 firstdmy=12.1.2024 \
	//	  b1072=23197.49 pre=1297:283:26:92.30 targ=1502.57 payhard=6758.34 \
	//	  noterm bdump
	//
	// (a 235-year quarterly `noterm` solve whose prepayment series starts at
	// period 1297, past the term). `runDump` used a bare
	// `exec.Command(...).Output()` with NO timeout, so ONE such screen hung the
	// whole test binary until the OUTER wrapper killed it —
	// paired_regression.sh's `timeout 900`, or 1800 in round 17's runner. That
	// wrapper kill discards the ENTIRE SEED: no ledger, no COVERAGE line, no
	// signals, for all 400 cases. The seed simply contributes nothing, and
	// nothing in the output says so.
	//
	// Why this is worse than lost time. The screens that hang are not a random
	// subset — they are long-horizon backward solves with stacked options, i.e.
	// exactly the frontier this fuzzer exists to sample. Killing their seeds
	// biases the surviving population toward benign screens, so every rate
	// computed from a run that lost seeds is computed on a population the run
	// does not describe. It is round 16b's R8 lesson in a new place: a bucket
	// that swallows cases silently makes 5% and 50% look the same.
	//
	// `.Output()` also buffers the runaway stdout in memory with no bound.
	//
	// The fix is a bounded run plus a NAMED TERMINAL BUCKET, so a DOS
	// non-termination is counted and visible rather than fatal to its seed.
	fz5OracleTimeout
)

// fz5OracleBudget bounds one oracle spawn. The 99th-percentile legitimate call
// in this fuzzer is milliseconds — even a 4000-period `bdump` — so this is three
// orders of magnitude of headroom, chosen so that a genuinely slow-but-finite
// screen is never misclassified as a non-termination. See fz5OracleTimeout.
const fz5OracleBudget = 20 * time.Second

// fz5Mode selects which cell of the loan screen is left blank for DOS to work
// back to. fz5ModePaySolve and fz5ModePayGiven are the two arms this fuzzer
// carried from the start; the rest blank a field OTHER than the payment and are
// the 2026-07-29 backward-solve widening.
//
// The mode axis used to be a single coin — the payment was either typed or
// solved — which left every BACKWARD-SOLVE screen outside the stacked-option
// space. That is not a quiet corner. Solving backwards is not a different
// screen; it is the same schedule walk driven from a different unknown, and it
// runs through Iterate, which is where this project's deepest divergences have
// lived (the bracket-free secant's basin selection, the phantom-daterec snap in
// Re_Amortize). The oracle has parsed the tokens since the 2026-07-11 audit
// extension (amort_oracle.pas:752-769) and the pass-2/pass-3 audits exercise
// them ALONE, on plain loans. Stacking them with balloons, prepayments, ARMs,
// moratoria, skips and targets is precisely this fuzzer's remit.
//
// Every backward arm also supplies a hard payment. With the payment blank as
// well the screen is under-determined and DOS refuses outright, so these are
// variations on the `payhard=` arm rather than on the solve arm.
const (
	fz5ModePaySolve = iota // payment blank, DOS solves it
	fz5ModePayGiven        // payment typed, nothing else blank
	fz5ModeTerm            // + `noterm`: BOTH n and the last date blank
	fz5ModeN               // + `non` + `lastdmy=`: n blank, last date typed
	fz5ModeAmount          // + `noamt`: amount borrowed blank
	fz5ModeRate            // + `norate`: loan rate blank
	fz5ModeCount
)

var fz5ModeName = [fz5ModeCount]string{"solve", "pay", "noterm", "non", "noamt", "norate"}

// fz5ModeFilter is the parsed PERSENSE_FUZZ_MODES allow-list (empty = all modes).
// Names match fz5ModeName exactly; an unrecognised name is a hard failure rather
// than a silent no-op, because a typo'd filter would quietly run a UNIFORM sweep
// and the operator would read the clean result as frontier evidence.
var fz5ModeFilter = func() []int {
	raw := os.Getenv("PERSENSE_FUZZ_MODES")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []int
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		found := -1
		for i, n := range fz5ModeName {
			if n == tok {
				found = i
				break
			}
		}
		if found < 0 {
			panic("PERSENSE_FUZZ_MODES: unknown mode " + tok +
				" (want a comma-separated subset of solve,pay,noterm,non,noamt,norate)")
		}
		out = append(out, found)
	}
	return out
}()

// fz5Dump is everything one `bdump` oracle run tells us.
type fz5Dump struct {
	payment   float64
	interest  float64
	paid      float64
	nballoons int
	nlines    int
	rows      []fz5BalloonRow
	lastDate  string // "M/D/YYYY" as DOS prints it
	nPeriods  int

	// Backward-solve answers, written back into the screen cell by MakeTable and
	// echoed after the totals (amort_oracle.pas:1080-1085). Only the cell the
	// run actually blanked is present, hence the has* flags. The SOLVED TERM has
	// no field here on purpose: `noterm` and `non` both leave their answer in
	// h^.nperiods / h^.lastdate, which the bdump block already emits above as
	// `lastdate M/D/YYYY nperiods N` — parsing `solvedterm` as well would be a
	// second name for one number.
	solvedAmt     float64
	solvedRate    float64
	hasSolvedAmt  bool
	hasSolvedRate bool

	// adjRows is the `adjdump` block — present since 2026-08-08, when the
	// standing arm first started asking for it.
	adjRows []fz5AdjRow

	// ROUND 22: DOS's own refusal sentence, carried out of runDump so a terminal
	// bucket can sub-classify on it. The `fz5DateHorizon` bucket turned out to
	// hold two different claims — a representation limit and DOS's own internal
	// error — and telling them apart requires the text, which every caller
	// previously discarded (§65). Empty when DOS did not refuse.
	errLine string
}

type fz5BalloonRow struct {
	idx     int
	date    string // "M/D/YYYY"
	dstatus int
	amount  float64
	astatus int
}

// fz5AdjRow is one `adjrow` line from the oracle's `adjdump`
// (amort_oracle.pas:1290-1296) — the solved Rate/Payment Adjustment cells as
// DOS's own grid displays them. The dump existed for the whole life of this
// project with ZERO consumers (2026-08-08 coverage audit) while the generator
// emitted ~1,079 blank adjustment cells per 3,000 screens; the round-39
// "New Amount never filled in" defect lived exactly there. Statuses are DOS's
// inout bytes: 0=empty, 1=outp (DOS computed it), 2=defp, 3=inp (the user's).
type fz5AdjRow struct {
	idx        int
	date       string // "M/D/YYYY"
	rate       float64
	rateStatus int
	amount     float64
	amtStatus  int
}

// tackPathName names which of TackOnFinalBalloon's two arms produced a row,
// from the DATE status the oracle reports for it (Amortize.pas:1046-1066):
//
//	outp (1) — APPEND. very_last coincided with no user balloon, so DOS did
//	           inc(nballoons) and stamped datestatus := outp itself.
//	inp  (3) — MERGE (merge_w_existing). very_last landed on the LAST user
//	           balloon's date, so DOS re-solved that row in place and the date
//	           keeps the status the user typed.
//
// This distinction is not cosmetic: the two arms differ on whether the row is
// de-activated with dec(nballoons), i.e. on whether the divergent cell takes
// part in the schedule and the APR. Round 18 assumed APPEND for all 27 §59
// cases and got the severity argument backwards for a round.
func tackPathName(dstatus int) string {
	switch int8(dstatus) {
	case types.InOutOutput:
		return "APPEND path"
	case types.InOutInput, types.InOutDefault:
		return "MERGE path (merge_w_existing)"
	default:
		return "UNKNOWN path"
	}
}

// tack returns the terminating-balloon row TackOnFinalBalloon produced: the LAST
// row whose AMOUNT is an OUTPUT cell (status 1 = outp).
//
// TackOnFinalBalloon has two paths (Amortize.pas:1046-1066) and they leave
// different status pairs behind:
//
//   - APPEND — very_last does not coincide with any user balloon, so DOS does
//     `inc(nballoons); balloon[nballoons]^.date := very_last;
//     balloon[nballoons]^.datestatus := outp;` and then solves the amount. Both
//     cells read outp.
//   - MERGE (`merge_w_existing`) — very_last lands exactly on the LAST user
//     balloon's date, so DOS re-solves THAT row in place. Only the amount is
//     rewritten; the date keeps the inp status the user typed.
//
// Requiring both to be outp therefore saw only the append path. It missed
//
//	amort_oracle 57832.55 0.1097610000 9 1 b365 exact inadv plusreg r78 \
//	  loandmy=30.8.2024 firstdmy=30.8.2025 mor=12 b60=13303.20 b84=9046.87 \
//	  targ=2226.61 pts=0.024213 payhard=12935.07 noterm bdump
//
// whose solved term puts the last payment on 2031-08-30 — the b84 date — so the
// merge fires and DOS reports `balloonrow 2 date 8/30/2031 dstatus 3 amount
// -10642.6700 astatus 1`. The port produced 8/30/2031 -10642.67, an exact match,
// and was reported as "the port tacked a terminating balloon DOS did not".
//
// Keying on the amount alone is still specific: the fuzzer always supplies every
// `b<N>=` amount, so an outp amount can only have been written by a solve, and
// the tack-on is the only balloon solve on this path (`unkballoon` is cleared
// before and after — Amortize.pas:1067, and the `unkballoon = 0` gate at :1388
// keeps the two from overlapping).
func (d fz5Dump) tack() (fz5BalloonRow, bool) {
	for i := len(d.rows) - 1; i >= 0; i-- {
		if d.rows[i].astatus == 1 {
			return d.rows[i], true
		}
	}
	return fz5BalloonRow{}, false
}

// aprRetryWithoutPoints re-runs an oracle line with its `pts=` cell removed and
// folds the points fee back into the totals by hand. See the call site for why
// an APR non-convergence is not a refusal (Amortize.pas:1419-1422 warns without
// exiting, so DOS still draws the table).
//
// Returns ok=false when there is no `pts=` cell to strip (then the APR message
// came from somewhere the reconstruction does not model, and the caller should
// keep treating it as a refusal) or when the amount cannot be read back.
func aprRetryWithoutPoints(run func([]string) (fz5Dump, int), args []string) (fz5Dump, int, bool) {
	pts, stripped := 0.0, make([]string, 0, len(args))
	found := false
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "pts="); ok {
			f, e := strconv.ParseFloat(v, 64)
			if e != nil {
				return fz5Dump{}, 0, false
			}
			pts, found = f, true
			continue
		}
		stripped = append(stripped, a)
	}
	if !found || len(stripped) == 0 {
		return fz5Dump{}, 0, false
	}
	amount, e := strconv.ParseFloat(stripped[0], 64)
	if e != nil {
		return fz5Dump{}, 0, false
	}
	d, oc := run(stripped)
	if oc != fz5Solved {
		return d, oc, true
	}
	fee := pts * amount
	d.interest += fee
	d.paid += fee
	return d, fz5Solved, true
}

func TestDOSFuzzer5AllAdvancedOptions(t *testing.T) {
	// Opt-in: the all-options space still diverges in places (the moratorium +
	// Exact gap of docs/discrepancies.md §45 among them), so this stays off the
	// default `go test ./...` run and is used for hunting.
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1 to run the all-advanced-options fuzzer")
	}
	gateOracle(t)

	rng := rand.New(rand.NewSource(fuzzSeed(0x667a3520))) // "fz5 "

	N := 300
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			N = v
		}
	}

	q6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	cents := func(x float64) float64 { return math.Round(x*100) / 100 }
	// addMonthsFrom mirrors the oracle's month arithmetic off an arbitrary loan
	// date, verbatim from amort_oracle.pas's option-date blocks:
	//
	//	nbal := (h^.loandate.m - 1) + MONTHS;
	//	date.d := h^.loandate.d;
	//	date.m := (nbal mod 12) + 1;
	//	date.y := h^.loandate.y + (nbal div 12);
	//	CheckForDaysTooLarge(date);
	//
	// The day of month is carried across and then CLAMPED, which is DOS's own
	// rule for moving a date by whole months. AddPeriod (INTSUTIL.pas:1208-1252)
	// restores d:=orig_day BEFORE stepping the month and ends with
	// CheckForDaysTooLarge, and that routine (VIDEODAT.pas:349-354) clamps
	// rather than normalises: `last:=DaysInM(f); if (f.d>last) then f.d:=last;`.
	// So 31 Jan → 28 Feb → 31 Mar: the original day is sticky and the month
	// never rolls forward the way Go's time.Date would (31 Feb → 3 Mar).
	//
	// The clamp has to happen on the raw day, BEFORE types.NewDateRec, because
	// NewDateRec goes through time.Date and normalises — a DateRec can never
	// hold the out-of-range day that dateutil.CheckForDaysTooLarge exists to
	// fix. DaysInM is asked on the first of the target month so the leap rule
	// is DOS's own (VIDEODAT.pas: `y mod 4 = 0`, no century correction).
	//
	// THE YEAR IS A BYTE. `date.y := h^.loandate.y + (nbal div 12)` assigns into
	// the byte field of a Pascal daterec, so it truncates mod 256 exactly like
	// every other year assignment in DOS (docs/discrepancies.md §55). Round 12
	// widened the term draw to 300 years, and without this the harness hands the
	// two engines DIFFERENT option dates — the oracle receiving a wrapped year
	// and the Go input an unwrapped one — which reads exactly like an engine
	// divergence and is nothing of the kind. This is the same standing rule that
	// produced §51: any date the harness computes must be computed the way the
	// oracle computes it.
	// R2, docs/harness_policy.md — ONE shared helper, and it is differentially
	// pinned. Extracted from this closure to package scope (round 16) so that
	// zzharnessdates_test.go can hold it against DOS's own AddNPeriods via
	// `amort_oracle intutil addn`. A harness date implementation nobody can test
	// is how six of the seven harness defects happened.
	addMonthsFrom := fz5AddMonths
	// present implements the client's rule: each option block is used unless a
	// 15% coin says otherwise.
	present := func() bool { return rng.Float64() >= 0.15 }

	// oddFirstMonths lists the first-payment offsets that are NOT the default
	// one full period: 1..2*mPer with mPer itself removed. Below mPer the first
	// period is a SHORT stub, above it a LONG one. For monthly loans (mPer=1)
	// the short half is empty, so only long stubs are available.
	oddFirstMonths := func(mPer int) []int {
		out := make([]int, 0, 2*mPer)
		for m := 1; m <= 2*mPer; m++ {
			if m != mPer {
				out = append(out, m)
			}
		}
		return out
	}

	dosLine := func(f []string, name string) (float64, bool) {
		for i := 0; i+1 < len(f); i++ {
			if f[i] == name {
				v, e := strconv.ParseFloat(f[i+1], 64)
				return v, e == nil
			}
		}
		return 0, false
	}

	// runDump execs the oracle in `bdump` mode and parses the whole output:
	// the balloon grid AND the quiet totals line. Retries the known heap flakes
	// (payment 0 / interest -1.00 / paid <= 0) on a fresh process, as every
	// other oracle helper in this package does.
	errBucket := map[string]int{}
	oracleTimeouts := 0
	// Defect #11 accounting. Two DIFFERENT numbers, and only the second is the
	// defect's impact:
	//   noTotalsRetried   — spawns that hit the sentinel and were respawned. A
	//                       deterministic no-totals case contributes 7 of these
	//                       all by itself, so this is a COST measure, not a
	//                       finding.
	//   noTotalsRecovered — cases that hit the sentinel and then went on to
	//                       return real totals. THIS is the count of comparisons
	//                       the old immediate-return was silently throwing away.
	// Both reported even at zero, per R8b: a recovery channel that says nothing
	// when it fires is what hid this in the first place.
	noTotalsRetried, noTotalsRecovered := 0, 0
	var runDump func(args []string) (fz5Dump, int)
	runDump = func(args []string) (fz5Dump, int) {
		sawNoTotals := false
		for try := 0; try < 8; try++ {
			// HARNESS DEFECT #9 (round 17) — see fz5OracleTimeout. A bare
			// exec.Command().Output() here let a non-terminating DOS screen hang
			// the whole binary until an outer wrapper killed it, discarding the
			// entire seed silently. The deadline is per SPAWN, and a timeout
			// returns immediately rather than burning the remaining 7 retries:
			// non-termination is deterministic, so retrying it just multiplies
			// the wait by eight (the same arithmetic mistake R8 found in the
			// flake bucket).
			ctx, cancel := context.WithTimeout(context.Background(), fz5OracleBudget)
			out, err := exec.CommandContext(ctx, oracleBin, args...).Output()
			timedOut := ctx.Err() == context.DeadlineExceeded
			cancel()
			if timedOut {
				oracleTimeouts++
				return fz5Dump{}, fz5OracleTimeout
			}
			if err != nil {
				continue
			}
			text := strings.TrimSpace(string(out))
			if text == "" {
				continue
			}
			if strings.HasPrefix(text, "ERR") {
				first := strings.SplitN(text, "\n", 2)[0]
				errBucket[strings.TrimSpace(strings.TrimPrefix(first, "ERR"))]++
				msg := strings.ToLower(text)
				if strings.Contains(msg, "julian") || strings.Contains(msg, "bad date") ||
					strings.Contains(msg, "last payment not found") {
					return fz5Dump{errLine: first}, fz5DateHorizon
				}
				if strings.Contains(msg, "did not converge") {
					return fz5Dump{errLine: first}, fz5NonConverge
				}
				// "Computation of APR failed to converge." is NOT a refusal.
				// DOS's dispatch is (Amortize.pas:1419-1422)
				//
				//	if (h^.pointsstatus > defp) then
				//	  if (not EstimateAndRefineAPRwithPoints) then
				//	    MessageBox('Computation of APR failed to converge.',
				//	               DA_APRNoConverge);
				//
				// with NO `exit` after the MessageBox — unlike every other
				// failure arm in that routine. The user gets a warning box and
				// the amortization table is still drawn, with a meaningless APR.
				// Only the headless harness turns it fatal: the oracle's Globals
				// stub routes every MessageBox to noteError (legacy/oracle/
				// Globals.pas:99-106), which aborts the run before any output.
				//
				// The points cell does not shape the schedule at all — it is a
				// flat fee folded into the totals, `interest` and `paid` both
				// gaining exactly points x amount (measured:
				// `100000 0.10 120 12 payhard=1500` -> interest 46576.09; the
				// same line + `pts=0.03` -> 49576.09, + `pts=0.027009` ->
				// 49276.99). So re-run without the points cell, which is also
				// what switches the APR solve off ("if =defp, then it's zero by
				// default and we skip the APR computation", Amortize.pas:1419),
				// and add the fee back. That recovers DOS's real answer instead
				// of scoring the case as a refusal.
				//
				// 2026-07-25 seed 11001: both hard failures were this. e.g.
				//	325717.65 0.1332850000 25 1 exact mor=132 b168=32482.08 \
				//	  b204=77376.37 pre=48:177:12:770.15 adj=12::57986.62 \
				//	  adj=120:0.1027860000: targ=3474.07 pts=0.027009 \
				//	  payhard=40676.18
				// reconstructs to 178070.36 + 0.027009*325717.65 = 186867.61,
				// which is what the port already produced.
				if strings.Contains(msg, "apr failed to converge") {
					if d2, oc2, ok := aprRetryWithoutPoints(runDump, args); ok {
						return d2, oc2
					}
				}
				// DO_ExxpOverflow raised inside the APR-with-points refinement
				// is NOT the same non-refusal as the arm above, even though it
				// reaches the caller through the same bare `MessageBox(...,
				// DA_APRNoConverge)`. It is a genuine DOS refusal, and the
				// 2026-07-26 reading below (kept for the mechanism, which is
				// accurate) drew the wrong conclusion from it by stopping the
				// walk one frame too early.
				//
				// The difference is `errorflag`. `exxp` (INTSUTIL.pas:1146-1154)
				// does not merely display a dialog —
				//
				//	if (x>70) then begin
				//	  exxp:=0;
				//	  MessageBox('Overflow error: ...', DO_ExxpOverflow);
				//	  overflowflag:=true; errorflag:=true;
				//	  end
				//
				// — and `errorflag` is a LATCH, cleared only in FirstPass
				// (Amortize.pas:204) at the top of the next Enter. `GET_OUT`
				// (Amortize.pas:606-615) does not clear it; it only frees the
				// saved balloon state. So control does reach TABLE_START with
				// the schedule intact, exactly as described below — but
				// TABLE_START is the tail of `Enter`, not the table. The table
				// lives in `MakeTable` (Amortize.pas:1453-1458), which is
				//
				//	procedure MakeTable( Output: TStringList; ... );
				//	begin
				//	  Enter( no_tab );
				//	  if( errorflag ) then exit;
				//	  if( not SufficientDataOnScreen ) then exit;
				//	  {---------------START MAKING TABLE---------------}
				//
				// and `Enter( no_tab )` cannot fall through to the drawing code
				// itself (`if (code<>make_table) then exit`, :1444). The latched
				// errorflag therefore kills the table at :1458, before a single
				// row is emitted.
				//
				// Verified 2026-07-27 rather than inferred: a scratch oracle
				// that prints `Output.Count` before the ERR line reports
				//
				//	ERRLINES 0 errorflag=TRUE overflowflag=TRUE
				//
				// on all three known repro lines (seed 20236's, and seed 20311's
				// two). MakeTable produced ZERO lines — the refusal comes from
				// the DOS engine's own guard, not from the headless stub.
				//
				// Contrast the pure `count = 20` exit, which is the arm above:
				// no exxp fires, errorflag stays false, MakeTable draws the
				// schedule, and the APR field keeps whatever the weaker
				// `abs(delta) < tiny` test at :595 stored. That case never even
				// reaches this classifier, because the oracle's Globals stub
				// already swallows DA_APRNoConverge outright (legacy/oracle/
				// Globals.pas:118-135).
				//
				// So an "overflow error" from the oracle is DOS refusing, and it
				// must score as fz5Refused. Retrying without the points cell
				// answers a DIFFERENT screen — one where Amortize.pas:1419 skips
				// the APR solve entirely ("if =defp, then it's zero by default")
				// — and reporting that answer as DOS's would enshrine a schedule
				// the DOS engine never draws.
				//
				// The original 2026-07-26 mechanism walk, retained because it is
				// still the correct account of WHERE the overflow comes from:
				//
				// EstimateAndRefineAPRwithPoints (Amortize.pas:516-615) runs an
				// unbounded 20-step secant on `v_rate`, and each step re-walks
				// the whole schedule through RepayFancyLoan(..., value_calc, 0).
				// The two accumulator lines in that walk are
				//
				//	if (value_calc) then
				//	  aprvalue := aprvalue + NextPayment.payamt
				//	              * exxp(-v_rate * YearsDif(NextPayment.date, loandate));
				//
				// at AMORTOP.pas:1195/1218 and the tacked-balloon twin at :1225.
				// `v_rate` is a trial value with no bound at all, so on a long
				// schedule a negative trial makes `-v_rate * years` positive and
				// large, and exxp (INTSUTIL.pas:1146-1154) trips its x>70 guard.
				//
				// That this is expected rather than fatal is written into the
				// DOS routine itself: the secant's loop head is
				//
				//	repeat
				//	  inc(count);
				//	  if (overflowflag) then
				//	    goto GET_OUT;
				//
				// at Amortize.pas:567-570 — the author put an explicit escape
				// hatch there for exactly this. Control lands at GET_OUT:606,
				// the function returns false, and the caller at :1420-1422 is
				// the same bare `MessageBox(..., DA_APRNoConverge)` with no
				// `exit` that the arm above already documents. Execution falls
				// through to TABLE_START — but NOT past MakeTable's errorflag
				// guard, per the correction above.
				//
				// Measured on seed 20236 with a per-call-site exxp trace:
				//	38919.07 0.0838560000 42 2 prepaid plusreg r78 usa \
				//	  loandmy=19.10.2023 firstdmy=19.4.2024 b24=8646.73 \
				//	  adj=42:0.0234180000: adj=84:0.1022710000:2821.95 \
				//	  targ=324.90 pts=0.008288 payhard=2572.79
				// emits `XT 1218 x=60.34 … 70.39` — seven consecutive rows of
				// the value_calc accumulator climbing 1.676 per row (0.5 yr at
				// peryr=2, i.e. v_rate ~= -3.35) until the last row of a 42-year
				// schedule crosses 70. Both trace hits are on the `value_calc`
				// lines; the real table walk never overflows. Dropping `pts` and
				// nothing else makes DOS solve the same line cleanly
				// (interest 15395.79 paid 54314.86) — but that is a different
				// screen, not this one's answer.
				return fz5Dump{errLine: first}, fz5Refused
			}
			var d fz5Dump
			gotTotals := false
			for _, ln := range strings.Split(text, "\n") {
				f := strings.Fields(strings.TrimSpace(ln))
				if len(f) == 0 {
					continue
				}
				switch f[0] {
				case "nballoons":
					if v, ok := dosLine(f, "nballoons"); ok {
						d.nballoons = int(v)
					}
					if v, ok := dosLine(f, "nlines"); ok {
						d.nlines = int(v)
					}
				case "balloonrow":
					// balloonrow I date M/D/YYYY dstatus S amount A astatus S
					var r fz5BalloonRow
					if len(f) > 1 {
						r.idx, _ = strconv.Atoi(f[1])
					}
					for i := 0; i+1 < len(f); i++ {
						switch f[i] {
						case "date":
							r.date = f[i+1]
						case "dstatus":
							r.dstatus, _ = strconv.Atoi(f[i+1])
						case "amount":
							r.amount, _ = strconv.ParseFloat(f[i+1], 64)
						case "astatus":
							r.astatus, _ = strconv.Atoi(f[i+1])
						}
					}
					d.rows = append(d.rows, r)
				case "adjrow":
					// adjrow I date M/D/YYYY rate R ratestatus S amount A amtstatus S amtok B
					var ar fz5AdjRow
					if len(f) > 1 {
						ar.idx, _ = strconv.Atoi(f[1])
					}
					for i := 0; i+1 < len(f); i++ {
						switch f[i] {
						case "date":
							ar.date = f[i+1]
						case "rate":
							ar.rate, _ = strconv.ParseFloat(f[i+1], 64)
						case "ratestatus":
							ar.rateStatus, _ = strconv.Atoi(f[i+1])
						case "amount":
							ar.amount, _ = strconv.ParseFloat(f[i+1], 64)
						case "amtstatus":
							ar.amtStatus, _ = strconv.Atoi(f[i+1])
						}
					}
					d.adjRows = append(d.adjRows, ar)
				case "lastdate":
					if len(f) > 1 {
						d.lastDate = f[1]
					}
					if v, ok := dosLine(f, "nperiods"); ok {
						d.nPeriods = int(v)
					}
				case "solvedamount":
					if v, ok := dosLine(f, "solvedamount"); ok {
						d.solvedAmt, d.hasSolvedAmt = v, true
					}
				case "solvedrate":
					if v, ok := dosLine(f, "solvedrate"); ok {
						d.solvedRate, d.hasSolvedRate = v, true
					}
				case "payment":
					p, pok := dosLine(f, "payment")
					in, iok := dosLine(f, "interest")
					pd, dok := dosLine(f, "paid")
					if !pok || !iok || !dok {
						continue
					}
					d.payment, d.interest, d.paid = p, in, pd
					gotTotals = true
				}
			}
			if !gotTotals {
				continue // no totals line at all — flake, respawn
			}
			// R8, docs/harness_policy.md — SPLIT THE "FLAKE" BUCKET.
			//
			// This sentinel used to read `d.paid <= 0 || d.interest == -1 ||
			// d.payment == 0` and `continue` (i.e. respawn) on all three. But
			// runDump retries EIGHT times, so a genuinely random heap flake at the
			// documented 4-9% would essentially never reach the caller — and yet
			// round 16 measured 10 of 120 cases (8.3%) doing so.
			//
			// Re-running all ten by hand: every one succeeds 5/5 and returns the
			// SAME output. They were never flakes. Nine of the ten are
			//
			//	payment <positive> interest -1.00 paid -1.00
			//	lastdate -88/0/1900 nperiods 0
			//
			// i.e. DOS solved the payment, then its own date arithmetic overflowed
			// (`-88` is the shortint day, 1900 the wrapped year — §55's territory)
			// and it declined to total the schedule. The tenth has the same -1
			// totals with a VALID `lastdate 5/28/2033 nperiods 120`.
			//
			// HARNESS DEFECT #11 (round 18b) — THE TWO ARMS BELOW ARE NOT THE SAME
			// KIND OF THING, AND ONLY ONE OF THEM IS DETERMINISTIC.
			//
			// The paragraph above concluded "neither is noise, and neither improves
			// on retry" from ten hand-re-run cases, and made BOTH arms return
			// immediately. That is correct for the DATE-HORIZON arm and wrong for
			// the NO-TOTALS arm, and the difference is visible in the output: the
			// date-horizon arm carries a STRUCTURAL marker — `nperiods 0` or a
			// wrapped `-88/0/1900` last date, §55's territory — that a resource
			// failure cannot manufacture. The no-totals arm carries no marker at
			// all. It is just "-1/-1 with a valid date", which is also exactly what
			// a transient allocation failure inside DOS looks like.
			//
			// Measured, round 18b. Eighteen `fz5NoTotals` cases were dumped from
			// seeds 50100-50119 and each re-run against the oracle: **12 reproduce
			// the sentinel, 4 return REAL totals, 2 do not parse.** The four were
			// then re-run 24 times each at concurrency 6 on a 2-core box —
			// deliberately oversubscribed, which is the condition the harness
			// itself creates:
			//
			//	nper 540: TOTALS 23, SENTINEL 1
			//	nper  96: TOTALS 24
			//	nper 156: TOTALS 23, SENTINEL 1
			//	nper 300: TOTALS 23, SENTINEL 1
			//
			// So the sentinel appears in roughly 4% of runs on screens DOS can
			// answer perfectly well, and the immediate return converts that
			// transient into a PERMANENT exclusion — the case is never compared,
			// and nothing says a comparable screen was dropped. Same family as
			// defect #9 (silent attrition), same bias direction: the screens that
			// fail this way are the large ones (nper 156-540), so the surviving
			// population skews small.
			//
			// The fix keeps round 16b's real insight — do not burn 8 spawns on a
			// deterministic failure — and applies it only where determinism was
			// actually demonstrated. The date-horizon arm still returns at once.
			// The no-totals arm now RETRIES, and reaches its bucket only if the
			// sentinel survives every attempt, at which point it is genuine.
			if d.payment != 0 && d.interest == -1 && d.paid == -1 {
				if d.nPeriods == 0 || strings.HasPrefix(d.lastDate, "-") {
					return d, fz5DateHorizon
				}
				// Budget, not "use the whole loop". Round 16b's cost objection was
				// right even though its determinism premise was not: a genuinely
				// deterministic no-totals case pays the full retry budget every
				// time, so the budget should be the smallest one that recovers the
				// transient. Measured p(sentinel) is ~4% per spawn on the affected
				// screens, so p(surviving 3 spawns) is ~6e-5 — far below any rate
				// this project reports. Three costs 2 extra spawns per deterministic
				// case; seven cost 6, for no additional recovery.
				const fz5NoTotalsRetries = 3
				if try < fz5NoTotalsRetries-1 {
					noTotalsRetried++
					sawNoTotals = true
					continue
				}
				return d, fz5NoTotals
			}
			// True heap flake: no payment at all, or a malformed/negative total
			// that does not carry the -1 sentinel pair. These DO improve on a
			// respawn, which is why the retry loop exists.
			if d.paid <= 0 || d.interest == -1 || d.payment == 0 {
				continue
			}
			// Reaching real totals after the sentinel IS defect #11's impact: the
			// pre-round-18b harness would have bucketed this case as no-totals and
			// never compared it.
			if sawNoTotals {
				noTotalsRecovered++
			}
			return d, fz5Solved
		}
		return fz5Dump{}, fz5Flake
	}

	// runAPR is Signal 6's SECOND oracle spawn (2026-08-08). It cannot ride on
	// runDump's `bdump` spawn: the `apr` query prints `apr X status Y` and then
	// `Halt(0)`s BEFORE the totals line (amort_oracle.pas:1230-1235) — which is
	// the whole historical reason no arm ever stacked it, and therefore the whole
	// reason a 2.60-percentage-point APR divergence could sit on a screen whose
	// totals matched to $0.0000. Same per-spawn deadline discipline as runDump
	// (harness defect #9: a bare .Output() on a non-terminating screen hangs the
	// binary); a timeout or ERR returns ok=false and the CALLER counts the skip —
	// a probe whose failures are silent is note #43's shape.
	runAPR := func(args []string) (apr float64, status int, ok bool) {
		for try := 0; try < 4; try++ {
			ctx, cancel := context.WithTimeout(context.Background(), fz5OracleBudget)
			out, err := exec.CommandContext(ctx, oracleBin, args...).Output()
			timedOut := ctx.Err() == context.DeadlineExceeded
			cancel()
			if timedOut {
				return 0, 0, false // deterministic; do not burn the retries (R8)
			}
			if err != nil {
				continue
			}
			// SCAN LINES, do not parse the whole text: the args carry `bdump` (and
			// `adjdump`), whose header lines print BEFORE the apr handler runs, so
			// `apr X status Y` is typically the THIRD line, not the first. The
			// first version of this parser read line 1, failed on every screen,
			// and the anti-vacuity guard below caught it on its very first run —
			// 31 eligible, 0 compared. Which is the guard earning its keep on day
			// one; keep it.
			for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				f := strings.Fields(strings.TrimSpace(ln))
				if len(f) >= 4 && f[0] == "apr" && f[2] == "status" {
					v, e1 := strconv.ParseFloat(f[1], 64)
					st, e2 := strconv.Atoi(f[3])
					if e1 == nil && e2 == nil {
						return v, st, true
					}
					return 0, 0, false
				}
			}
			return 0, 0, false
		}
		return 0, 0, false
	}

	// The payment-frequency axis. This was {1, 2, 4, 12} until 2026-07-30, which
	// left DOS's OTHER five supported frequencies — 3, 6, 24, 26, 52 — never
	// sampled, and 26/52 are exactly where the sub-monthly behaviour lives: the
	// engine-level 360->365 basis coercion (Amortize.pas:297-303) and the horizon
	// wall's payment-grid alignment both key off them. A high-severity term-solve
	// horizon defect sat behind that gap for months and was found by AUDIT, not by
	// fuzzing, because no generated case could reach it. Anything claimed about
	// weekly or biweekly fidelity before this widening rests on zero evidence.
	perYrs := []int{1, 2, 3, 4, 6, 12, 24, 26, 52}
	bases := []types.BasisType{types.Basis360, types.Basis365, types.Basis365360}

	// retires reports whether a Go schedule genuinely amortizes (its last
	// remaining balance is ~0 relative to the loan). Same definition as
	// fuzzer3-fancy's: it is what separates a VALID root DOS's less-robust
	// Iterate simply missed from a DEGENERATE spurious solve the port invented.
	retires := func(r AmortResult, principal float64) bool {
		if r.Err != nil || len(r.Schedule) == 0 {
			return false
		}
		bal := r.Schedule[len(r.Schedule)-1].Principal
		return math.Abs(bal) <= math.Max(1.0, 1e-4*math.Abs(principal))
	}

	// Per-case accounting.
	checked, refused, flaked, horizon := 0, 0, 0, 0
	// R7 coverage accumulators — see the block at `checked++`.
	covN, covMinN, covMaxN := 0, 0, 0
	covMinYrs, covMaxYrs := 0, 0
	// GENERATED envelope, tracked separately from the COMPARED envelope: the gap
	// between the two is the attrition profile (refusals, non-converges, horizon,
	// skips), and a widening gap in one region means that region is generated but
	// never measured — which reads as coverage and is not (R7).
	genMinYrs, genMaxYrs, genMinN, genMaxN := 0, 0, 0, 0
	covPerYr := map[int]int{}
	covMode := map[string]int{}
	// R5: cases abandoned before the oracle spawn because every advanced-option
	// coin came up empty. Intended, but it must appear in the ledger.
	skippedPlain := 0
	// R8: DOS answered the payment but refused to total the schedule, with its
	// horizon cells intact. Previously counted as flake.
	noTotals := 0
	// ROUND 22 — the two silent buckets' asymmetry counters. See the long note
	// at `case fz5DateHorizon`. Printed unconditionally, including at zero:
	// R8/R13, and the whole point of the round-22 audit is that a bucket which
	// prints nothing is a bucket nobody re-examines.
	horizonGoSolved, horizonGoSolvedInScope := 0, 0
	horizonGoSolvedInternalErr := 0
	noTotalsGoSolved := 0
	// HARNESS DEFECT #9 (round 17) — cases where the DOS engine did not
	// terminate. Its own terminal bucket in the R5 ledger below.
	oracleTimedOut := 0
	nonConv, nonConvGoRetires, nonConvGoSpurious := 0, 0, 0
	goRefusedDosSolved, goSolvedDosRefused := 0, 0
	tackAgree, tackGoOnly, tackDosOnly, tackValueDiff := 0, 0, 0, 0
	solveChecked, solveDiff := 0, 0
	// Signal 5/6 ledgers (2026-08-08). Every skip path has a counter, per R16 —
	// the fuzzer-coverage audit's central finding was that this file's own
	// `payment` field had been PARSED AND THROWN AWAY since the file was written,
	// and that NO arm in the repo ever asked the oracle for the APR at all. Five
	// user-visible defects lived in exactly those two unread numbers. A probe
	// whose skips are silent is how that happens twice.
	payChecked, payDiff, paySkipZero := 0, 0, 0
	// ROUND 41, item 0m(ii) — SIGNAL 5 MUST BE SPLIT TYPED vs SOLVED.
	//
	// 39e reported "230 checked, 0 differ" and read it as "the transport HOLDS".
	// The round-40 audit called that near-vacuous and it is right: most fz5 modes
	// emit `payhard=`, i.e. they TYPE the payment, and for those the comparison is
	// between two copies of the USER'S OWN INPUT. It cannot distinguish a correct
	// transport from the deleted reconstruction, because both would echo the typed
	// number. The round-39 defect lived ENTIRELY in the solved-payment population
	// (mode == fz5ModePaySolve), which is the only stratum where DOS's payamt is a
	// number the engine had to produce. A pooled "0 differ" hides how many of the
	// 230 were actually load-bearing. CAUTION 9 / R41: report the strata.
	payCheckedTyped, payCheckedSolved := 0, 0
	payDiffTyped, payDiffSolved := 0, 0
	aprCompared, aprDiff, aprEligible := 0, 0, 0
	aprSkipStatus, aprSkipSpawn, aprGoNoConverge := 0, 0, 0
	// ROUND 43, item 0m(i) / R49 — SIGNAL 6's DISCRIMINATING POPULATION.
	//
	// Round 39e reverted the modal-payment fallback, Signal 6 produced the
	// IDENTICAL 4 divergences, and the round recorded "the negative control was
	// INERT." That was read as a statement about the PROBE's sensitivity. It is
	// not. ROUND40_AUDIT §3.1: the control has not been shown to detect anything
	// "until it is re-run on a seed whose population contains a pay-solve ∩
	// points screen on the AmortizeDOS arm where modal ≠ payment." If seed 50100
	// holds no such case then NO mutant could have moved the result — and
	// START_HERE never carried that sentence, so rounds 41, 42 and 43 each
	// re-specified the experiment that had already failed.
	//
	// aprDiscrim counts exactly that population: cases where re-introducing the
	// modal reconstruction WOULD change the number the APR pass is fed. If it is
	// 0, a negative control on this seed is VACUOUS, not negative. → R49.
	// The count is asserted below: it is the positive control, inside the test.
	aprDiscrim, aprDiscrimPaySolve, aprDiscrimPts := 0, 0, 0
	aprDiscrimDosport := 0
	aprDiscrimWorst := 0.0
	adjScreens, adjRowsChecked, adjRowsDiff := 0, 0, 0
	// ROUND 41, item 0m(iii) — SIGNAL 7 MUST BE STRATIFIED BY ENGINE.
	//
	// 39e's "5 findings / 91 rows / 43 screens" cannot be split NF-1 (the known
	// piecewise echo gap, 30-38% of rows) from a NEW regression, because the run
	// pooled both engines. An unsplit count is unusable as a baseline: the next
	// session cannot tell "NF-1, known" from "something broke". Keyed on
	// gr.EngineUsed, which round 41 made a TRANSPORTED field rather than a
	// stderr-parsing reconstruction (see zzr41_engine_transport_test.go).
	adjScreensByEngine := map[string]int{}
	adjRowsByEngine := map[string]int{}
	adjDiffByEngine := map[string]int{}
	// Compared cases per engine — the denominator every per-engine rate in §2 of
	// START_HERE needs, now available in-process instead of by pairing GENGINE
	// lines against the FZ5ENGBEGIN bracket in a python analyzer.
	engCases := map[string]int{}
	engHard := map[string]int{}
	blockCover := map[string]int{}

	// R10 TOLERANCE INSTRUMENTATION (round 18b), and the FIRST version of it was
	// wrong — recorded here because the wrong version is the instructive part.
	//
	// The first attempt measured the SEPARATION GAP: the ratio |delta|/tol for
	// every judged case, expecting a mis-scaled tolerance to show passing and
	// failing populations running into each other. It was validated against
	// defect #10 by restoring the old tack tolerance and re-running — and the old
	// tolerance showed gaps of 30,184x and 815,435,001x. **The metric did not
	// detect the defect it was built from.**
	//
	// Why: defect #10's population is bimodal in ratio space too, just for the
	// wrong reason. Small-balance screens passed by an enormous margin and
	// large-balance screens failed by an enormous margin, because the ratio was
	// tracking the BALANCE SIZE rather than the agreement quality. A clean-looking
	// gap that is really a proxy for "how big is this number" says nothing.
	//
	// What actually characterises defect #10 is that the tolerance's IMPLIED
	// RELATIVE PRECISION was not constant. A tolerance keyed to the value it
	// guards demands the same number of significant figures from every case;
	// tol/|value| is then flat. The old tack tolerance demanded ~1.0 relative on a
	// small balance and 1.4e-11 on a huge one — a spread of eleven orders of
	// magnitude, all of it an accident of which quantity the constant happened to
	// be multiplied by.
	//
	// So the metric is the SPREAD of tol/|value| across the judged population.
	// Narrow means the tolerance asks a consistent question. Orders of magnitude
	// means it asks a different question of different cases, which is what being
	// keyed to the wrong quantity looks like from the outside — and it is
	// detectable without knowing anything about the engine, the units, or which
	// constant is "right". Pinned in TestFz5ToleranceScalingIsConsistent, which
	// asserts the metric flags the old tack tolerance and clears the new one.
	//
	// Always-on rather than behind an env var: defect #10 survived three rounds
	// because nothing in a normal run would have shown it, and the point is that
	// the next mis-scaled constant announces itself in the ordinary output of the
	// first run that exercises it.
	type tolStat struct {
		n              int
		pass           int
		maxPass        float64 // largest |delta|/tol among AGREEING cases
		minFail        float64 // smallest |delta|/tol among DIVERGING cases
		nearMiss       int     // passing cases within one decade of the tolerance
		minRel, maxRel float64 // spread of tol/|value| — the defect-#10 detector
	}
	tolStats := map[string]*tolStat{}
	noteTol := func(name string, delta, tol, value float64) {
		if tol <= 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
			return
		}
		ts := tolStats[name]
		if ts == nil {
			ts = &tolStat{minFail: math.Inf(1), minRel: math.Inf(1)}
			tolStats[name] = ts
		}
		if v := math.Abs(value); v > 0 {
			rel := tol / v
			if rel < ts.minRel {
				ts.minRel = rel
			}
			if rel > ts.maxRel {
				ts.maxRel = rel
			}
		}
		r := delta / tol
		ts.n++
		if delta > tol {
			if r < ts.minFail {
				ts.minFail = r
			}
			return
		}
		ts.pass++
		if r > ts.maxPass {
			ts.maxPass = r
		}
		if r > 0.1 {
			ts.nearMiss++
		}
	}

	type classStat struct {
		n         int
		diverge   int
		worstInt  float64
		worstPaid float64
		worstCmd  string
	}
	classes := map[string]*classStat{}

	// ---- ERA SPLIT (round 19) ----
	// The client's scope boundary is "we do not compare beyond 2099"
	// (2026-08-03 decision). Nate's instruction was to keep GENERATING the long
	// horizons and report them separately rather than narrow the draw: a
	// generator bound is invisible once it is in place, and this project has
	// twice quoted a convergence number over a space it believed was bigger
	// than it was (`years := 8+Intn(18)`, and stratum C sampled at 2109-2120
	// under a 2092-2155 label). Splitting the REPORT costs nothing, because
	// these cases already run, and it keeps a live regression instrument past
	// the boundary.
	//
	// The boundary is keyed on the schedule's own HORIZON — the latest date the
	// walk actually reaches, including trailing balloons — not on the last
	// regular payment date. That is the date §54 and §59 both turn on, and a
	// far-future user balloon can pull the walk past 2100 while the payment
	// grid ends decades earlier (that is precisely §59's population).
	//
	// Index 0 = horizon <= 2099 (IN SCOPE), index 1 = horizon > 2099.
	var eraCompared, eraHard [2]int
	// ROUND 41, R41 — THE FOUR-QUESTION RATE, CARRIED ALONGSIDE THE SEVEN-QUESTION
	// ONE. 39e added three HARD signals (regular payment, APR, adjustment cells)
	// to an instrument whose headline number is HARD-cases-over-compared-cases, so
	// the numerator grew BY CONSTRUCTION and every rate published before 39e stopped
	// being comparable to every rate published after. R41's remedy is to publish
	// BOTH rather than quietly replace one with the other, in the same run, on the
	// same cases — so the question-set effect is a measured quantity instead of a
	// caveat in prose.
	var eraHardQ4 [2]int

	for c := 0; c < N; c++ {
		perYr := perYrs[rng.Intn(len(perYrs))]
		// subMonthly marks the frequencies DOS supports that do NOT divide 12 —
		// 24, 26 and 52. The whole option-date grammar of this rig is expressed in
		// WHOLE MONTHS (the oracle's `b<N>=`, `pre=`, `adj=`, `mor=` tokens are
		// month offsets from the loan date), so a sub-monthly payment grid has no
		// integer months-per-period and those options simply cannot be placed on
		// it: `12/perYr` is 0, which is where this generator panicked with an
		// integer divide by zero the moment 24/26/52 were added to the axis.
		//
		// Rather than skip the frequencies, sub-monthly cases are generated as a
		// STRATUM WITHOUT the four month-anchored options (see pickMonth below),
		// still exercising basis x solve-mode x points x target x odd-first-stub —
		// which is exactly the shape of the term-solve horizon defect that the
		// narrow {1,2,4,12} axis could never reach. Placing month-anchored options
		// on a weekly grid would need the oracle's absolute-date tokens
		// (`predmy=`, `adjdmy=`, `bdate=`); worth doing, and out of scope here.
		subMonthly := 12%perYr != 0
		mPer := 12 / perYr
		if subMonthly {
			mPer = 1 // first-stub axis only; no option date is derived from it
		}
		// TERM DRAW — STRATIFIED, widened in round 12.
		//
		// This was `years := 8 + rng.Intn(18)` — 8..25 years and nothing else —
		// from the day the fuzzer was written, which meant it had NEVER
		// generated a schedule longer than 25 years. The 2026-07-31 assessment
		// measured that unsampled region at 1.5% divergent inside DOS's date
		// range and 5.5% past its Julian ceiling, against ~0.03% where this
		// generator does sample, and the worst screen it found was out by 14x.
		// The mechanism behind it is docs/discrepancies.md §55 (DOS stores a
		// date's year in a BYTE, so every horizon past 2155 wraps mod 256).
		//
		// The bulk of the draw stays in the original band so the existing corpus
		// keeps its density; the tail reaches the three regions beyond it:
		//
		//	26..60y    long, still inside DOS's 70000-day Julian ceiling
		//	61..120y   crosses the Julian ceiling (DOS starts refusing)
		//	121..300y  past the year byte — §55 territory
		years := fz5DrawYears(rng)
		n := years * perYr
		// EXPLICIT CAP, not a silent one: a 300-year weekly loan is 15,600 rows
		// and the sweep drives every one of them through two engines. 4,000
		// periods keeps a case under a second while still reaching the year byte
		// at perYr <= 24 (126 years is 3,024 semi-monthly periods). At 26 and 52
		// the cap binds first — but so does DOS: those frequencies step dates
		// through Julian/MDY, whose 70000-day ceiling refuses past ~191 years
		// anyway, so no §55 case is lost to it.
		if n > fz5NCap {
			n = fz5NCap
			years = n / perYr
		}
		if genMinYrs == 0 || years < genMinYrs {
			genMinYrs = years
		}
		if years > genMaxYrs {
			genMaxYrs = years
		}
		if genMinN == 0 || n < genMinN {
			genMinN = n
		}
		if n > genMaxN {
			genMaxN = n
		}
		amount := cents(fz5AmountLo + rng.Float64()*fz5AmountSpan)
		rate := q6(fz5RateLo + rng.Float64()*fz5RateSpan)

		// fair is the closed-form level payment that would retire this loan with
		// no options at all. Every option amount below is scaled off it rather
		// than off a flat dollar range. That is not cosmetic: a flat "$25..$625
		// per prepayment" is rounding noise on a $500k loan and instantly
		// over-retires a $25k one, and the over-retiring end is exactly where
		// DOS's Iterate gives up with "did not converge" — which is why the first
		// smoke run left only 20% of cases comparable. Scaling keeps the stacked
		// options inside the region where DOS will actually produce a schedule to
		// compare against, without narrowing WHICH options get stacked.
		fair := amount * (rate / float64(perYr)) / (1 - math.Pow(1+rate/float64(perYr), -float64(n)))

		basis := bases[rng.Intn(len(bases))]
		exact := rng.Intn(2) == 0
		prepaid := rng.Intn(2) == 0
		inadv := rng.Intn(2) == 0
		plusreg := rng.Intn(2) == 0
		// R78 and the USA Rule were pinned false when this fuzzer was written, so
		// the two interest-ALLOCATION modes were the only advanced options never
		// stacked with the option blocks below. They are not inert: with a plain
		// 100k/9%/60/12 loan the oracle's row 1 is int 750.00 / prin 1325.84 in
		// ordinary mode and int 804.92 / prin 1270.91 under r78 — the totals line
		// is identical, so a bdump-only comparison cannot see the difference, but
		// every balloon row's balance can. The dedicated cube
		// (dos_amort_r78_usa_cube_test.go) covers them ALONE; what was missing is
		// r78 × balloons × prepayments × adjustments × moratorium × skip × targ.
		// Drawn as coin flips, matching the neighbouring mode flags rather than
		// the 15%-absent convention used for the option BLOCKS.
		r78 := rng.Intn(2) == 0
		usa := rng.Intn(2) == 0

		s := gzSettings(perYr, basis, exact, prepaid, inadv, r78, usa)
		s.PlusRegular = plusreg

		var flags []string
		// R7 / standing rule 9 (round 17). These seven flags are drawn as coin
		// flips OUTSIDE the option-block `note()` path, so until now they had no
		// entry in `blockCover` and therefore NO MEASURED BASE RATE. Round 16
		// characterised its 60-signal residual with lines like "b365 39, usa/inadv
		// 34, prepaid 31, exact 29, r78 26" and had to label the whole list
		// "hypotheses, not enrichments" for exactly that reason. A frequency
		// without its denominator is not evidence, and the fix is one map
		// increment per flag — not a caveat.
		//
		// Counted here rather than in `note()` on purpose: `note()` also sets
		// `anyOpt`/`fancyOpt`, and these flags must NOT make an otherwise plain
		// loan count as an advanced-option screen. Coverage bookkeeping only.
		if bf, ok := basisFlag(basis); ok {
			flags = append(flags, bf)
			blockCover["basis:"+bf]++
		} else {
			blockCover["basis:none"]++
		}
		if exact {
			flags = append(flags, "exact")
			blockCover["exact"]++
		}
		if prepaid {
			flags = append(flags, "prepaid")
			blockCover["prepaid"]++
		}
		if inadv {
			flags = append(flags, "inadv")
			blockCover["inadv"]++
		}
		if plusreg {
			flags = append(flags, "plusreg")
			blockCover["plusreg"]++
		}
		if r78 {
			flags = append(flags, "r78")
			blockCover["r78"]++
		}
		if usa {
			flags = append(flags, "usa")
			blockCover["usa"]++
		}

		// ---- Loan / first-payment date axis ----
		// Every case this fuzzer had ever generated used the oracle's SetupLoan
		// default of 1 January 2024, so the entire date surface was unfuzzed:
		// month-length variation, leap years, and the year rollover inside
		// YearsDif were all pinned to a single point. That matters because the
		// 365 and 365/360 bases accrue on ACTUAL days — a period spanning
		// February is materially shorter than one spanning July — and because
		// DOS's YearsDif (INTSUTIL.pas:787-824) has a screen-dependent branch
		// that only shows up across a year boundary.
		//
		// Day of month draws the full 1..31. It used to stop at 28 because both
		// harnesses synthesised option dates by carrying the loan day across a
		// month step with no clamp, which could hand the engine a 31 Feb — an
		// input no DOS screen can produce, since option dates are typed into
		// validated date cells. Both sides now clamp the way DOS does
		// (CheckForDaysTooLarge, VIDEODAT.pas:349-354; see addMonthsFrom above
		// and the matching calls in amort_oracle.pas), so the month-end axis is
		// a fair comparison: a 31st loan date lands on the 28th/29th in
		// February and returns to the 31st in March, with the original day
		// sticky rather than rolled forward.
		//
		// Day 31 is drawn as often as any other, which deliberately
		// over-samples it relative to the calendar — seven months have a 31st,
		// so on the real distribution a 31st loan date is rarer than the axis
		// makes it here. Over-sampling is the point: the clamp only fires on
		// days 29-31.
		//
		// The draw itself is clamped to the drawn month for the same reason the
		// synthesis is: types.NewDateRec normalises, so an unclamped 31 February
		// would silently become 3 March and the case would not be testing the
		// month-end axis at all — it would be testing a date the user could
		// never have typed.
		ldY, ldM := fz5LoanYearLo+rng.Intn(fz5LoanYearN), time.Month(1+rng.Intn(12))
		ldD := 1 + rng.Intn(31)
		if last := dateutil.DaysInM(types.NewDateRec(ldY, ldM, 1)); ldD > last {
			ldD = last
		}
		loanDate := types.NewDateRec(ldY, ldM, ldD)

		// ---- Odd-first-period axis ----
		// The first payment date used to be pinned at exactly one period after
		// the loan date — the oracle's default relationship — which left the
		// prorated first period completely unfuzzed. That is not a quiet corner:
		// it is where BOTH seed-20250 defects lived (the settlement row's raw
		// PrepaidInterest and the prepaid `prorate := 1`), and DOS reaches it
		// through three different code paths depending on where firstdate sits
		// relative to loandate + one period:
		//
		//	t := h^.firstdate; AddPeriod(t, h^.peryr, h^.firstdate.d, subtract);
		//	if (DateComp(t, h^.loandate) < 0) and (not df.c.in_advance) then
		//	  begin prepaid := false; end;                    {Amortize.pas:1251-1255}
		//
		// A SHORT stub (firstMonths < mPer) puts `t` before the loan date and so
		// silently CLEARS prepaid; the exact boundary takes the unconditional
		// `prorate := 1` arm; a LONG stub (firstMonths > mPer) leaves prepaid set
		// and accrues more than one period of interest into row 1.
		//
		// Drawn as a coin flip rather than through present(), matching the mode
		// flags (exact/prepaid/inadv/r78/usa) above: this is a screen FIELD
		// relationship, not one of the advanced-option BLOCKS the 85% convention
		// governs, and keeping half the corpus on the default relationship
		// preserves the regression value of every case found before this axis
		// existed. Range is 1..2*mPer excluding mPer, so monthly loans (mPer=1)
		// draw only LONG stubs — a short stub is arithmetically impossible when
		// one period is already the minimum offset.
		firstMonths := mPer
		if rng.Intn(2) == 0 {
			if cand := oddFirstMonths(mPer); len(cand) > 0 {
				firstMonths = cand[rng.Intn(len(cand))]
			}
		}
		firstDate := addMonthsFrom(loanDate, firstMonths)
		addMonths := func(months int) types.DateRec { return addMonthsFrom(loanDate, months) }
		flags = append(flags,
			fmt.Sprintf("loandmy=%d.%d.%d", loanDate.Time.Day(), int(loanDate.Time.Month()), loanDate.Time.Year()),
			fmt.Sprintf("firstdmy=%d.%d.%d", firstDate.Time.Day(), int(firstDate.Time.Month()), firstDate.Time.Year()))

		// Distinct option months on the payment grid, so no two option blocks
		// can land on the same date and expose §40d's collision ordering.
		used := map[int]bool{}
		pickMonth := func(loK, hiK int) (int, bool) {
			if subMonthly {
				// No whole-month grid exists for 24/26/52 — see subMonthly above.
				// Refusing here disables the moratorium, balloon, prepayment and
				// adjustment blocks in one place, since each is guarded by this
				// function's ok result.
				return 0, false
			}
			if hiK < loK {
				return 0, false
			}
			for try := 0; try < 40; try++ {
				k := loK + rng.Intn(hiK-loK+1)
				if !used[k] {
					used[k] = true
					// Payment k sits at firstdate + (k-1) periods, i.e.
					// loandate + firstMonths + (k-1)*mPer months. Both sides of
					// the rig expand these tokens off the LOAN date (the oracle
					// at amort_oracle.pas:172-176, addMonths() here), so the
					// month offset has to carry firstMonths — with an odd first
					// period the old k*mPer form lands BETWEEN payments and the
					// option-date axis stops being orthogonal to this one.
					return (k-1)*mPer + firstMonths, true
				}
			}
			return 0, false
		}

		var blocks []string
		var mutators []func(*LoanInput)
		anyOpt := false
		// fancyOpt mirrors the ORACLE's `fancy` flag, which is NOT "any option is
		// present" -- it is the user-facing screen toggle
		// (AmortizationScreenUnit.pas:306 `fancy := false`, :382 `fancy := not
		// fancy`; PEDATA.pas:714 `default_fancy := false`).  amort_oracle.pas sets
		// `fancy := true` only inside the Setup routines for balloons (:183),
		// prepayments (:276), adjustments (:423), mor= (:788), targ= (:802),
		// skip= (:813), solveballoon= (:859) and dateballoon= (:879).  `pts=` does
		// NOT set it -- points are an ordinary loan field, not an advanced option.
		//
		// This distinction is load-bearing, because DOS genuinely answers
		// DIFFERENTLY for the same loan under the two engines: Iterate's terminal
		// (AMORTOP.pas:1437) is
		//	if (fancy) or ((df.c.exact) and (df.c.basis<>x360))
		//	   then RepayFancyLoan else RepayLoan(p)
		// so a plain in-advance loan is repaid by RepayLoan's annuity-due branch
		// (AMORTOP.pas:1275-1281) instead of the prorated fancy walk.  Measured:
		//	amort_oracle 271486.77 0.0926380000 50 2 b365_360 prepaid inadv \
		//	  pts=0.012583
		//	  -> payment 13869.8743 interest 466664.05 paid 738150.82   (plain)
		//	same line + targ=0.01 (a no-op target, but it sets fancy)
		//	  -> payment 14109.5081 interest 436009.96 paid 707496.73   (fancy)
		// Go's Amortize produced 14109.51 / 436009.96 -- i.e. it matched DOS's
		// FANCY answer exactly and was correct; the harness was simply forcing
		// in.Fancy = true while the oracle ran the plain engine.  So mirror the
		// oracle: Fancy iff a fancy-setting option block was actually emitted.
		fancyOpt := false
		note := func(name string) {
			anyOpt = true
			if name != "pts" {
				fancyOpt = true
			}
			blocks = append(blocks, name)
			blockCover[name]++
		}

		// ---- Moratorium (interest-only until first repayment of principal) ----
		// Drawn FIRST even though it is written to the screen last, because it
		// constrains where a balloon may go. AMORTOP.pas rejects the whole screen
		// with "No balloon can precede first repayment of principal", and with
		// mor present 85% of the time and balloons scattered over the first 80%
		// of the term that single rule ate 16 of 40 cases on the previous run.
		// Honouring it is not a weakening of the fuzz: an illegal screen teaches
		// nothing about the port, it just costs a case.
		morK := 0
		if present() {
			if m, ok := pickMonth(1, maxInt(1, n/2)); ok {
				morK = (m-firstMonths)/mPer + 1 // inverse of pickMonth's grid map
				mor := Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: addMonths(m)}
				flags = append(flags, "mor="+strconv.Itoa(m))
				note("mor")
				mutators = append(mutators, func(in *LoanInput) { in.Moratorium = mor })
			}
		}

		// ---- Balloons: 1..3 rows, after any moratorium, inside the first ~80% ----
		if present() {
			k := 1 + rng.Intn(3)
			var months []int
			for i := 0; i < k; i++ {
				if m, ok := pickMonth(morK+1, maxInt(morK+1, n*8/10)); ok {
					months = append(months, m)
				}
			}
			sort.Ints(months)
			var bs []BalloonPayment
			// Cap the SUM of the balloons at 60% of principal. Individually a 40%
			// balloon is fine; three of them stacked retire the loan outright
			// mid-term, and DOS answers that with a non-converge rather than a
			// schedule, so the case is silently lost instead of compared.
			budget := amount * fz5BalloonBudgetFrac
			for _, m := range months {
				if budget <= 0 {
					break
				}
				amt := cents(math.Min(budget, amount*(fz5BalloonFracLo+rng.Float64()*fz5BalloonSpan)))
				if amt < 1 {
					break
				}
				budget -= amt
				flags = append(flags, fmt.Sprintf("b%d=%.2f", m, amt))
				bs = append(bs, BalloonPayment{
					DateStatus: types.InOutInput, Date: addMonths(m),
					AmountStatus: types.InOutInput, Amount: amt})
			}
			if len(bs) > 0 {
				note(fmt.Sprintf("balloon%d", len(bs)))
				mutators = append(mutators, func(in *LoanInput) { in.Balloons = bs })
			}
		}

		// ---- Prepayments: 1..maxprepay(2) series ----
		if present() {
			k := 1 + rng.Intn(fz5MaxPrepay)
			var ps []Prepayment
			for i := 0; i < k; i++ {
				// ROUND 36: one draw in fz5PreLateStartOdds starts PAST the last
				// scheduled payment. See the constant block for why this axis is
				// the one that has paid three times (§52/§53, §71, §72).
				lateStart := rng.Intn(fz5PreLateStartOdds) == 0
				lateNN := false
				var startK int
				if lateStart {
					startK = n + 1 + rng.Intn(maxInt(1, n*fz5PreLateStartMaxMul))
					// Clamp in MONTHS, not periods: payment k sits at
					// loandate + firstMonths + (k-1)*mPer months, and it is that
					// offset the year byte wraps. See fz5PreLateMonthCap.
					maxK := (fz5PreLateMonthCap-firstMonths)/maxInt(1, mPer) + 1
					if maxK <= n {
						// The term itself already fills the month ceiling, so no
						// draw can be BOTH past the term and inside the year byte.
						// Fall back to the ordinary in-term draw rather than emit
						// a wrapped date: a shape that is neither is worse than
						// either. (Round-36 audit.)
						lateStart = false
						startK = 1 + rng.Intn(maxInt(1, n*7/10))
					} else if startK > maxK {
						startK = maxK
					}
				} else {
					startK = 1 + rng.Intn(maxInt(1, n*7/10))
				}
				m, ok := pickMonth(startK, startK)
				if !ok {
					continue
				}
				// nn extra payments at ppy/yr. Keep the series inside the term so
				// CheckPrepayments' derived stop date does not run off the horizon.
				// Widened in round 12: this was {12, 24, 26, 52}, so a
				// prepayment series slower than monthly had probability zero.
				// Round 9 measured `ppy < perYr` at 30% divergent; rounds 9-11
				// closed that family and round 11 re-measured the whole
				// prepayment-frequency region at ZERO inside DOS's date range,
				// which is what makes it safe to sample here now.
				ppy := []int{1, 2, 4, 6, 12, 24, 26, 52}[rng.Intn(8)]
				// Months from the START of the prepayment series to the LAST
				// scheduled payment, which sits at loandate + firstMonths +
				// (n-1)*mPer — not at n*mPer once the first period is odd.
				remMonths := ((n-1)*mPer + firstMonths) - m
				maxNN := (remMonths * ppy) / 12
				if maxNN < 1 {
					maxNN = 1
				}
				// A LATE-STARTING SERIES HAS NO "REMAINING TERM" TO SIZE nn FROM:
				// remMonths is negative by construction, so the cap above would
				// pin every late series to a single payment and the family would
				// be drawn without ever being long enough to matter. §72's three
				// located cases carry nn of 52, 246 and 20,000. Give the late arm
				// its own draw.
				if lateStart {
					maxNN = fz5PreLateNNCap
					lateNN = true
				}
				// OVERSHOOT, widened in round 12. The cap above used to be the
				// whole story, so a series could never run past the term and the
				// `pre[i]^.stopdate > very_last` arm of DetermineVeryLast
				// (AMORTOP.pas:1302) was unreachable from here. That arm is what
				// §52 and §53 turned out to hinge on. One draw in six now
				// overshoots by up to 50%.
				if rng.Intn(6) == 0 {
					maxNN = maxNN + 1 + rng.Intn(maxNN/2+1)
				}
				if maxNN > fz5PreNNCap {
					maxNN = fz5PreNNCap
				}
				// ⚠️ THE LATE CAP MUST BE APPLIED AFTER THE OVERSHOOT, OR IT IS
				// NOT THE REACH. Round 36's first cut set maxNN = 300 before the
				// round-12 overshoot ran, so the late arm's true nn reach was 400
				// (fz5PreNNCap), not the 300 the manifest pinned — and the
				// manifest pinned the CONSTANT, not the reach, so it passed. The
				// manifest's whole purpose is to state the reach. Found by the
				// round-36 audit.
				if lateNN && maxNN > fz5PreLateNNCap {
					maxNN = fz5PreLateNNCap
				}
				nn := 1 + rng.Intn(maxNN)
				// Size the series as a FRACTION OF THE REGULAR PAYMENT FLOW: a
				// prepayment arrives ppy times a year against perYr regular
				// payments, so perYr/ppy converts "fraction of one regular
				// payment's worth of extra money per year" into a per-prepayment
				// amount. 3%-25% of the flow keeps the loan alive to term.
				amt := cents(fair * (fz5PreAmtFracLo + rng.Float64()*fz5PreAmtSpan) * float64(perYr) / float64(ppy))
				if amt < 1 {
					amt = 1
				}
				flags = append(flags, fmt.Sprintf("pre=%d:%d:%d:%.2f", m, nn, ppy, amt))
				ps = append(ps, Prepayment{
					StartDateStatus: types.InOutInput, StartDate: addMonths(m),
					NNStatus: types.InOutInput, NN: nn,
					PerYrStatus: types.InOutInput, PerYr: ppy,
					PaymentStatus: types.InOutInput, Payment: amt,
				})
			}
			if len(ps) > 0 {
				note(fmt.Sprintf("prepay%d", len(ps)))
				mutators = append(mutators, func(in *LoanInput) { in.Prepayments = ps })
			}
		}

		// ---- Rate / payment adjustments: 1..3 rows ----
		// Amortize.pas:1294 rejects the screen on `(df.c.in_advance) and (nadj >
		// 0)` — ANY adjustment row, not merely a rate change, despite the message
		// text saying "change rates". So the block is suppressed under inadv.
		// This is not an untested corner: an earlier sweep of this fuzzer
		// generated 20 inadv+adj screens and the port refused all 20 alongside
		// DOS (Go-solved-DOS-refused = 0), so the gate itself is verified. Left
		// in, it would only burn a third of the run on a known-agreeing refusal.
		if !inadv && present() {
			k := 1 + rng.Intn(3)
			var months []int
			for i := 0; i < k; i++ {
				if m, ok := pickMonth(1, maxInt(1, n*8/10)); ok {
					months = append(months, m)
				}
			}
			sort.Ints(months)
			if len(months) > 0 {
				var as []RateAdjustment
				kinds := make([]int, 0, len(months))
				for _, m := range months {
					// 0 = rate only, 1 = payment only, 2 = both. All three are real
					// DOS screens (adj= accepts a blank rate OR a blank amount) —
					kind := rng.Intn(3)
					kinds = append(kinds, kind)
					a := RateAdjustment{DateStatus: types.InOutInput, Date: addMonths(m)}
					rateStr, amtStr := "", ""
					if kind != 1 {
						nr := q6(fz5AdjRateLo + rng.Float64()*fz5AdjRateSpan)
						a.LoanRateStatus, a.LoanRate = types.InOutInput, nr
						rateStr = strconv.FormatFloat(nr, 'f', 10, 64)
					}
					if kind != 0 {
						// A payment adjustment is a NEW regular payment, so it has to
						// live near the fair payment. amount/n (straight-line
						// principal, no interest) is far below it on a long loan and
						// starves the schedule into a non-converge.
						na := cents(fair * (fz5AdjPayFracLo + rng.Float64()*fz5AdjPaySpan))
						a.AmountStatus, a.Amount, a.AmtOK = types.InOutInput, na, true
						amtStr = strconv.FormatFloat(na, 'f', 2, 64)
					}
					flags = append(flags, fmt.Sprintf("adj=%d:%s:%s", m, rateStr, amtStr))
					as = append(as, a)
				}
				note(fmt.Sprintf("adj%d", len(as)))
				_ = kinds
				mutators = append(mutators, func(in *LoanInput) { in.Adjustments = as })
			}
		}

		// ---- Target: minimum principal reduction per payment ----
		if present() {
			// The target is a MINIMUM principal reduction per payment, so it only
			// binds when it is a plausible slice of the payment. A flat $50-$500
			// is unreachable on a small loan (DOS then has nothing to converge to)
			// and inert on a large one.
			tv := cents(fair * (fz5TargFracLo + rng.Float64()*fz5TargSpan))
			flags = append(flags, fmt.Sprintf("targ=%.2f", tv))
			note("targ")
			mutators = append(mutators, func(in *LoanInput) {
				in.Target = Target{TargetStatus: types.InOutInput, TargetValue: tv}
			})
		}

		// ---- Skip months (month-of-year; monthly schedules only) ----
		if perYr == 12 && present() {
			skipStr := []string{"6", "1,7", "5-7", "2,8,11", "11-12", "1,3,5"}[rng.Intn(6)]
			sm := skipSetRaw(skipStr)
			if sm.SkipStatus != 0 {
				flags = append(flags, "skip="+skipStr)
				note("skip")
				mutators = append(mutators, func(in *LoanInput) { in.SkipMonths = sm })
			}
		}

		// ---- Backward-solve mode (drawn here so the points block can see it) ----
		mode := rng.Intn(fz5ModeCount)
		// PERSENSE_FUZZ_MODES restricts the mode draw to a named subset, so a run
		// can be aimed at a KNOWN FRONTIER instead of spreading uniformly over six
		// modes. The 2026-07-30 800-seed sweep measured where the defects actually
		// live: `noterm` carries 74% of all findings against a 16.6% base rate
		// (4.5x enrichment) and `noterm`+`non` together carry 86%, while plain
		// `pay` produced ZERO failures in ~53k cases
		// (claude/defect_population_estimate_2026-07-30.md). Restricting to
		// `noterm,non` therefore concentrates ~3x more probability on the frontier
		// per seed — one biased seed is worth roughly three unbiased ones for that
		// axis — at the cost of no longer sampling the modes already believed
		// clean. Leave it UNSET for general regression sweeps; set it when hunting.
		//
		//	PERSENSE_FUZZ_MODES=noterm,non PERSENSE_FUZZ=1 PERSENSE_REQUIRE_ORACLE=1 \
		//	  go test ./internal/finance/amortization/ -run TestDOSFuzzer5AllAdvancedOptions
		if len(fz5ModeFilter) > 0 {
			mode = fz5ModeFilter[rng.Intn(len(fz5ModeFilter))]
		}

		// ---- Points (turns the APR solver on in both engines) ----
		//
		// Never stacked with `noamt`. The APR-non-converge recovery path re-runs
		// the line with `pts=` stripped and folds the fee back in by hand as
		// points x ParamStr(1) (aprRetryWithoutPoints above). Under `noamt`,
		// ParamStr(1) is NOT the principal DOS used — the principal is the one
		// DOS solved for, and with the payment drawn anywhere in 0.85x-1.35x of
		// fair the two can differ by a third of the loan. The reconstructed
		// totals would then be off by points x (typed - solved), up to ~$1400 on
		// a large loan, against an interest tolerance of a few tens of dollars:
		// a guaranteed false divergence with nothing wrong in the port. Teaching
		// the retry a second amount source would work, but the cheaper and
		// clearer rule is that a blanked amount cell never carries points.
		points := -1.0
		if mode != fz5ModeAmount && present() {
			points = q6(rng.Float64() * fz5PointsSpan)
			flags = append(flags, fmt.Sprintf("pts=%.6f", points))
			note("pts")
		}

		if !anyOpt {
			// R5, docs/harness_policy.md — COUNTED, not silently dropped.
			// Round 16's ledger showed 2 of 120 generated cases reaching no
			// terminal bucket; this is where they went. The skip itself is
			// intended (this fuzzer exists to stack advanced options), but it
			// shrinks the denominator invisibly, and an invisible denominator is
			// how a 5% divergence rate and a 50% one come to look identical.
			//
			// COVERAGE CONSEQUENCE, and it is the more important half (standing
			// rule 8 — ask what the generator CANNOT produce): this fuzzer can
			// never report a divergence on a PLAIN loan, because it abandons the
			// case before the oracle is ever spawned. Plain-loan fidelity is
			// covered by zzmetafuzz_test.go's forward corpus and by the committed
			// unit suite — NOT by any figure derived from this fuzzer. Do not
			// quote a fuzzer5 rate as if it covered plain loans.
			skippedPlain++
			continue
		}

		// ---- Payment mode ----
		// The fair payment scaled 0.85x-1.35x, so residuals of BOTH signs occur
		// and TackOnFinalBalloon's over- and under-funded arms both get hit —
		// but the schedule still terminates. Below ~0.8x a stacked-option loan
		// negatively amortizes past DOS's horizon; above ~1.4x it retires far
		// early and DOS refuses rather than truncating.
		pay := cents(fair * (fz5PayFracLo + rng.Float64()*fz5PayFracSpan))
		if mode != fz5ModePaySolve {
			flags = append(flags, fmt.Sprintf("payhard=%.2f", pay))
		}
		var lastDate types.DateRec
		switch mode {
		case fz5ModeTerm:
			flags = append(flags, "noterm")
		case fz5ModeN:
			// The last payment date is placed ON the schedule's own grid:
			// firstdate + (n-1) periods, expanded through addMonthsFrom so the
			// day-of-month is carried and CLAMPED exactly as repeated AddPeriod
			// calls would carry it (AddPeriod restores d := orig_day before
			// stepping the month, so k single steps and one k-month jump agree).
			// An off-grid date is a legitimate DOS screen too and worth its own
			// axis later, but it would exercise FirstPass's SNAP rather than its
			// term derivation, and mixing the two would make a first divergence
			// ambiguous.
			// addMonthsFrom already truncates the year to DOS's byte (§55), so
			// the token and the Go-side date carry the SAME record the oracle's
			// `lastdmy=` block will build — `d1.y := StrToInt(Y) - 1900` is
			// itself a byte assignment, so a raw 2311 would reach DOS as 2055.
			lastDate = addMonthsFrom(firstDate, (n-1)*mPer)
			flags = append(flags, "non", fmt.Sprintf("lastdmy=%d.%d.%d",
				lastDate.Time.Day(), int(lastDate.Time.Month()), lastDate.Time.Year()))
		case fz5ModeAmount:
			flags = append(flags, "noamt")
		case fz5ModeRate:
			flags = append(flags, "norate")
		}
		// `adjdump` stacks with `bdump` in ONE spawn (it prints before the totals
		// and does not Halt) — verified 2026-08-08. Rule 7 is untouched: this adds
		// a REQUEST token, not a change to any default output.
		flags = append(flags, "adjdump", "bdump")

		args := []string{
			strconv.FormatFloat(amount, 'f', 2, 64),
			strconv.FormatFloat(rate, 'f', 10, 64),
			strconv.Itoa(n), strconv.Itoa(perYr),
		}
		args = append(args, flags...)

		dos, outcome := runDump(args)

		// ---- The Go side, byte-identical inputs ----
		in := gzLoanInput(amount, rate, n, perYr, s)
		in.Loan.LoanDate, in.Loan.FirstDate = loanDate, firstDate
		in.Fancy = fancyOpt // see the fancyOpt comment above: mirrors the oracle
		if mode != fz5ModePaySolve {
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, pay
		}
		if points >= 0 {
			in.Loan.PointsStatus, in.Loan.Points = types.InOutInput, points
		}
		switch mode {
		case fz5ModeTerm:
			// `noterm` blanks BOTH cells (amort_oracle.pas:763-764) and lets the
			// walk run until the loan retires. LastOK is deliberately left false
			// rather than forced: the oracle leaves h^.lastok false too, and both
			// sides' FirstPass equivalents derive it (amort_oracle.pas:764 vs
			// dosport_entry.go:28-34). Forcing it here would hide a divergence in
			// that derivation instead of exposing one.
			in.Loan.NStatus, in.Loan.NPeriods = types.StatusEmpty, 0
			in.Loan.LastStatus, in.Loan.LastOK = types.StatusEmpty, false
		case fz5ModeN:
			// `non` blanks ONLY n, leaving the typed last date in force, so
			// FirstPass must derive the term through NumberOfInstallments
			// (INTSUTIL.pas:936) — the very routine whose unclamped monthly exit
			// at :1013 produced the seed-40024 phantom daterec. That is why this
			// arm is worth keeping separate from `noterm`, which never calls it.
			// LastOK is left to the port for the same reason as above: the
			// oracle's lastdmy= block (amort_oracle.pas:769) sets laststatus but
			// NOT lastok, unlike the presolve blocks at :399-400 and :435-436.
			in.Loan.NStatus, in.Loan.NPeriods = types.StatusEmpty, 0
			in.Loan.LastStatus, in.Loan.LastDate = types.InOutInput, lastDate
		case fz5ModeAmount:
			in.Loan.AmountStatus, in.Loan.Amount = types.StatusEmpty, 0
		case fz5ModeRate:
			in.Loan.LoanRateStatus, in.Loan.LoanRate = types.StatusEmpty, 0
		}
		for _, m := range mutators {
			m(&in)
		}

		// Amount and rate are the two cells Amortize() will not work back to on
		// its own: the port exposes them as separate entry points
		// (SolveLoanAmount / SolveRate, backward.go:199/:408), mirroring DOS's
		// EstimateAndRefineLoanAmount / EstimateAndRefineRate. So run the same
		// two stages DOS runs — solve the cell, write the answer back as an
		// OUTPUT (which is what the screen holds afterwards), then draw the table
		// from it — and the totals comparison below measures the same composite
		// computation on both sides rather than two different ones. The solve
		// runs AFTER the mutators, because the option blocks are part of the
		// screen it is solving against.
		goSolved, goSolvedOK := 0.0, false
		var goSolveErr error
		switch mode {
		// The write-back status is InOutInput because that is what the SHIPPED
		// app does (handlers.go:1229 and :1241), and the fuzzer's job is to find
		// divergences a user can see. It is NOT what DOS does: Amortize.pas:1377
		// sets `h^.amountstatus := outp` after EstimateAndRefineLoanAmount, and
		// outp(1) < defp(2), so a solved cell reads as ABSENT to every `>= defp`
		// presence filter downstream. Exactly one filter looks at it after the
		// solve — the TackOnFinalBalloon gate at Amortize.pas:1386, which the port
		// already mirrors faithfully (tackon.go:155, and see its :151-154 note on
		// the outp exclusion). So writing InOutInput here should make Go tack a
		// terminating balloon onto solved-amount and solved-rate screens where DOS
		// tacks none, and the tack counters below should report it as Go-only.
		// That prediction is the point: if it holds, the shipped write-back status
		// is the bug; if it does not, the port models the gate somewhere else and
		// the assumption needs revisiting before anything is changed.
		//
		// InOutOutput cannot be used here as a shortcut, because Amortize() itself
		// refuses a below-defp amount at engine.go:230 — a re-check DOS does not
		// have (its equivalent, SufficientDataOnScreen at Amortize.pas:859, tests
		// `(amountstatus >= defp) or ComputeLoanAmount` and runs BEFORE the solve).
		// A NON-CONVERGED BACKWARD SOLVE ENDS THE SCREEN TOO — discrepancies §58
		// (round 16). The solvers report failure two ways: an `err` (a DOS-faithful
		// screen refusal) and a `converged=false` bool alongside a nil err (DOS's
		// Iterate exhausting its 20 passes with bestp over BOTH halfpenny and
		// acc_limit*init, AMORTOP.pas:1485-1492). DOS treats them identically —
		// MessageBox, Iterate:=false, errorflag, and `if (errorflag) then exit`
		// draws NO TABLE — and so does the shipped port: handlers.go:1260 returns
		// "Computation of payment amount or interest rate did not converge." and
		// never reaches amortization.Amortize.
		//
		// This harness discarded the bool (`v, _, err :=`), so it amortized at a
		// rate/amount the product refuses to show, then scored the resulting table
		// as "Go produced a schedule" — attributing to the port a screen no user
		// can ever see. That is the SAME defect the err-arm comment below already
		// describes; only the err half had been closed.
		//
		// Measured: round 13's paired-regression NEW=1 (open three rounds) is
		// exactly this. On
		//	amort_oracle 291207.99 0.1209560000 2688 24 exact prepaid \
		//	  loandmy=29.5.2024 firstdmy=29.7.2024 pts=0.009110 payhard=1962.94 norate
		// DOS refuses; SolveRate returns converged=FALSE with a nil err; the harness
		// amortized anyway and logged a $21.4bn non-retiring table. The port's own
		// dosIterateRate agrees with DOS to 10 dp at every n where DOS answers
		// (n<=2136) and correctly reports bestp=0.0063 > acc_limit*init=0.0058 at
		// n=2160, the first n DOS refuses. The ENGINE was faithful throughout.
		//
		// R1 (docs/harness_policy.md): both arms go through the SAME entry point
		// the product uses — amortization.SolveBlankCellsPrepared, which carries
		// the gate above. `...Prepared` rather than SolveBlankCells because this
		// generator constructs a fully-specified screen itself and must not
		// inherit handlers.go's FirstPass derivation, which would change the draw.
		// The gate is shared; the preparation is not.
		case fz5ModeAmount:
			out, err := SolveBlankCellsPrepared(in, in, true, false)
			if goSolveErr = err; err == nil {
				goSolved, goSolvedOK = out.Loan.Amount, true
				in = out
			}
		case fz5ModeRate:
			out, err := SolveBlankCellsPrepared(in, in, false, true)
			if goSolveErr = err; err == nil {
				goSolved, goSolvedOK = out.Loan.LoanRate, true
				in = out
			}
		}
		// A FAILED BACKWARD SOLVE ENDS THE SCREEN — no table is drawn. That is
		// what DOS does (EstimateAndRefineRate / EstimateAndRefineLoanAmount
		// return false ⇒ errorflag ⇒ MakeTable's `if (errorflag) then exit`,
		// Amortize.pas:1340-1380 / 1457-1458) and what the shipped port does
		// (handlers.go:1223-1247 writes the solver's error and RETURNS, never
		// reaching amortization.Amortize). Calling Amortize anyway handed it a
		// blank rate/amount cell it has no solver for, and the schedule it built
		// from the zeroed cell was then scored as "Go produced a schedule" —
		// attributing to the port a table no user can ever see. 2026-07-29 seed
		// 21001: after the dosIterateRate condemnation latch made SolveRate
		// refuse the `norate` screen exactly as DOS does, this harness step was
		// the only thing still producing rows.
		var gr AmortResult
		if goSolveErr != nil {
			gr.Err = goSolveErr
		} else {
			// ROUND 34 — BRACKET THE CALL WHOSE ANSWER IS COMPARED.
			//
			// engine.go prints one GENGINE line per Amortize invocation, at the top,
			// immediately before the routing branch. A screen produces SEVERAL of
			// them and NEITHER "the first" nor "the last" identifies the one that
			// answered:
			//
			//   * BEFORE this call, the backward modes run
			//     SolveBlankCellsPrepared, whose trial evaluations each print a
			//     line — so the FIRST line of a `norate`/`noamt` case belongs to
			//     the solver, not to the table;
			//   * INSIDE this call, solveFancyTermFromPayment / SolveBalloonAmount
			//     / SolvePrepaymentAmount / SolvePrepaymentDuration call Amortize
			//     again on a clone — so the LAST line of a `noterm` case belongs to
			//     a nested probe, not to the table.
			//
			// Round 33's analyzer took the first; this round's first draft took the
			// last on the strength of a review note that had the nesting backwards.
			// Measured over 160 seeds, "last" moved 390 compared cases from
			// piecewise to dosport — a 23% inflation of the faithful port's
			// denominator, in the direction that FLATTERS THE PORT (rule 12) — and
			// erased the `degenerate_term_or_peryr` row from the table entirely.
			//
			// A bracket settles it without guessing and without a global: the
			// OUTERMOST call is the first GENGINE line strictly inside it. R13 —
			// the fuzzer prints only the bracket it knows; engine.go still prints
			// the only thing that read the router.
			traceCase := os.Getenv("FZ5CASEDUMP") != ""
			if traceCase {
				fmt.Fprintf(os.Stderr, "FZ5ENGBEGIN %d\n", c)
			}
			gr = Amortize(in)
			if traceCase {
				fmt.Fprintf(os.Stderr, "FZ5ENGEND %d\n", c)
			}
		}
		goOK := gr.Err == nil && len(gr.Schedule) > 0

		// The first-period relationship is part of the case's identity, not an
		// option block: it goes into the divergence signature (so a class points
		// at the stub) and into block coverage (so the log proves the axis is
		// actually firing), but never into `blocks`, which drives fancyOpt.
		switch {
		case firstMonths < mPer:
			blockCover["first<"]++
		case firstMonths > mPer:
			blockCover["first>"]++
		}

		sort.Strings(blocks)
		sig := strings.Join(blocks, "+")
		switch {
		case firstMonths < mPer:
			sig += "|first<"
		case firstMonths > mPer:
			sig += "|first>"
		}
		sig += "|" + fz5ModeName[mode]
		blockCover["mode:"+fz5ModeName[mode]]++
		cmd := "amort_oracle " + strings.Join(args, " ")

		// FZ5CASEDUMP=1 prints EVERY generated case with its index, to STDERR,
		// unconditionally — not just the failing ones.
		//
		// Added round 19 because naming a case was impossible when the binary
		// died. Go buffers t.Logf until a test returns, so a run that is killed
		// (OOM, timeout, wrapper kill) emits nothing at all — the same property
		// that made harness defect #9 invisible. When the round-19 build started
		// getting OOM-killed on 5 of 120 seeds, there was no way to ask "which
		// case?" from the inside.
		//
		// The recipe that worked, and is worth keeping: bisect the seed on
		// PERSENSE_FUZZ_N (under `ulimit -v` so it fails fast rather than
		// thrashing) to find the first N that dies, then run at that N with this
		// flag and read the last index. Two minutes, and it turned "some seeds
		// die" into an exact reproducing command.
		//
		// stderr, not t.Logf, is the whole point — it is unbuffered and survives
		// the process being killed. Env-gated and off by default; it cannot
		// affect the signal set or any counted bucket.
		if os.Getenv("FZ5CASEDUMP") != "" {
			fmt.Fprintf(os.Stderr, "FZ5CASE %d %s\n", c, cmd)
			// ROUND 34 — THE CASE'S TERMINAL BUCKET, BY INDEX.
			//
			// Round 33 built its engine × diverged contingency table by re-running
			// `goamort` once per case (~35 min for 20 seeds) and matching numerator
			// to denominator BY ARGUMENT STRING. That instrument had three holes,
			// all of them structural: goamort implements neither `norate` nor
			// `noamt` (note #24), which silently removed 2,554 of 7,863 cases —
			// 32% of the corpus — and 17 of 75 divergences; 589 further cases
			// emitted no route at all; and an identical case drawn in two seeds was
			// counted twice.
			//
			// This fuzzer already runs the port and the oracle on every case, and
			// engine.go already prints the route on stderr under DPTRACEENGINE=1.
			// Emitting the case's BUCKET here, keyed by the same index as the
			// FZ5CASE line, makes one ordinary arm run carry the whole table: same
			// process, same draw, no re-execution, no argument matching, and no
			// third-of-the-population blind spot. R13 — this line prints only what
			// this loop has itself decided.
			//
			// Rule 7 is intact: same env gate, stderr, and `paired_regression.sh`
			// greps `amort_oracle …`, which this line does not contain.
			bucket := "solved"
			switch outcome {
			case fz5Refused:
				bucket = "refused"
			case fz5Flake:
				bucket = "flake"
			case fz5DateHorizon:
				bucket = "date_horizon"
			case fz5NonConverge:
				bucket = "nonconverge"
			case fz5NoTotals:
				bucket = "no_totals"
			case fz5OracleTimeout:
				bucket = "oracle_timeout"
			}
			fmt.Fprintf(os.Stderr, "FZ5OUTCOME %d bucket=%s goOK=%v\n", c, bucket, goOK)
		}

		switch outcome {
		case fz5Flake:
			flaked++
			// R8, docs/harness_policy.md — WHAT IS ACTUALLY IN THE "FLAKE" BUCKET?
			//
			// runDump already retries EIGHT times on a fresh process, so reaching
			// here means eight consecutive failures. A random per-attempt flake of
			// the documented 4-9% would produce that essentially never
			// (0.09^8 ≈ 4e-9), yet round 16 measured 10 of 120 cases (8.3%)
			// landing here. These are therefore DETERMINISTIC per-screen failures
			// mislabelled as noise — and a screen that always fails to produce a
			// totals line is a screen the port is NEVER compared on, which is a
			// coverage hole, not a flake.
			//
			// Gated behind an env var because `cmd` contains an `amort_oracle …`
			// string and paired_regression.sh greps exactly that — emitting it by
			// default would register every flake as a NEW divergence. Rule 7:
			// never change a harness's default output.
			if os.Getenv("PERSENSE_FUZZ_FLAKEDUMP") != "" {
				t.Logf("SIG=INFO:oracle_no_totals_after_8_tries\n  %s", cmd)
			}
			continue
		case fz5OracleTimeout:
			// HARNESS DEFECT #9 (round 17). DOS did not terminate on this screen.
			// This is a property of the DOS engine, not of the port, and it is
			// NOT evidence about fidelity — the port is never compared here. It
			// gets its own terminal bucket so the case is counted rather than
			// taking its whole seed down with it.
			//
			// Same env gate as the flake dump and for the same reason (rule 7):
			// `cmd` starts with "amort_oracle " and paired_regression.sh greps
			// exactly that, so emitting it by default would score every DOS
			// non-termination as a divergence.
			oracleTimedOut++
			if os.Getenv("PERSENSE_FUZZ_FLAKEDUMP") != "" {
				t.Logf("SIG=INFO:oracle_nontermination_%ds\n  %s",
					int(fz5OracleBudget/time.Second), cmd)
			}
			continue
		case fz5DateHorizon:
			horizon++
			// ROUND 22 — R15 APPLIED TO THIS FUZZER'S OWN BUCKETS.
			//
			// THE HOLE THIS CLOSES. Four terminal buckets end in a `continue`.
			// Two of them — fz5Refused and fz5NonConverge — ask the port what it
			// did before continuing, and fz5Refused makes "the port answered a
			// screen DOS rejected" a HARD failure. The other two, this one and
			// fz5NoTotals, asked nothing at all. Nothing recorded why the
			// asymmetry check applied to two buckets and not the other two; it
			// reads as an omission rather than a decision, and between them the
			// two silent buckets are ~10% of every generated population (34 and
			// ~4 per 400 at the standing settings).
			//
			// A defect that lives ONLY here has a specific and plausible shape:
			// DOS's date arithmetic gives up — its Julian routine, its 1900-based
			// year byte (§55), or its "last payment not found" walk — and the
			// port, carrying a proleptic-Gregorian date layer that has none of
			// those limits (§54), sails past the wall and produces a confident
			// schedule for a screen the original cannot express. That is the same
			// class as go_solved_dos_refused, on the exact frontier this project
			// has spent five rounds on, and it was unmeasured.
			//
			// SEVERITY IS NOT THE SAME AS fz5Refused's, and the difference is the
			// whole reason this is a separate counter. A DOS date-horizon refusal
			// is DOS hitting its own representational ceiling, not DOS judging the
			// screen invalid — and the client's decision of 2026-08-03 is that
			// comparison past 2099 is not required. So the port answering here is
			// counted and reported with its reproducing command, and is only
			// HARD when the port's own resolved horizon is IN SCOPE: there, DOS
			// refused a screen it had no representational excuse to refuse, and
			// the port disagreed about a schedule a client can actually reach.
			if goOK {
				horizonGoSolved++
				lastY := fz5MaxYear(gr)
				if lastY > 0 && lastY <= 2099 {
					horizonGoSolvedInScope++
					// SUB-CLASSIFY BEFORE SCORING — the bucket's own label turned
					// out to be two things (§65).
					//
					// `fz5DateHorizon` membership is decided by DOS's message
					// containing "julian", "bad date" OR "last payment not found".
					// The first two are DOS hitting a REPRESENTATION limit — its
					// Julian routine, its 1900-based year byte — and a port with a
					// wider calendar answering there is expected and out of scope.
					// The third is DOS's own INTERNAL ERROR, whose text ends
					// "Please contact Ones & Zeros": DOS is not judging the screen
					// invalid, it is reporting that it failed. Those are not the
					// same claim and they had been sharing a bucket since the
					// bucket existed.
					//
					// Measured on seed 50100: ALL FIVE in-scope cases are the
					// internal-error subclass. Scoring them HARD today would turn
					// the standing gate red on a class nobody has adjudicated,
					// which is the mistake round 17 made when it named a frontier
					// from an unread numerator. So the internal-error subclass is
					// counted and reported and carries its repro; the OTHER
					// subclass — DOS refusing an in-scope screen for a stated
					// calendar reason while the port answers — stays HARD, and is
					// currently zero. §65 is the adjudication.
					if strings.Contains(dos.errLine, "last payment not found") {
						horizonGoSolvedInternalErr++
						// GATED — RULE 7, AND THIS IS THE SECOND TIME IN ONE ROUND.
						// The first ungated per-case line added here produced
						// `paired_regression.sh` NEW 195 against an identical
						// engine; gating it and adding THIS one produced NEW 177.
						// The gate greps `amort_oracle .*` across the whole test
						// output, so it cannot distinguish a new SIGNAL from a new
						// LOG LINE — and every per-case line that quotes a repro
						// reads as a regression, no matter how it is labelled.
						//
						// The COUNT stays in default output (the summary line
						// below carries no oracle command, so the gate does not
						// see it). Only the per-case repro moves behind the env
						// var. §65's reproducing commands come from a
						// PERSENSE_FUZZ_FLAKEDUMP=1 run, and the section says so.
						if os.Getenv("PERSENSE_FUZZ_FLAKEDUMP") != "" {
							t.Logf("DOS returned its own INTERNAL ERROR (\"last payment not "+
								"found … contact Ones & Zeros\") and the port built a %d-row "+
								"schedule ending %d, inside the comparison boundary. Counted, "+
								"not failed, pending §65.\n"+
								"  SIG=ADVISORY:go_solved_dos_internal_error_in_scope %s\n"+
								"  Go: int=%.2f paid=%.2f",
								len(gr.Schedule), lastY, cmd, gr.TotalInt, gr.TotalPaid)
						}
					} else {
						t.Errorf("DOS refused this screen on a DATE HORIZON but the port built a "+
							"%d-row schedule ending %d — INSIDE the comparison boundary, so DOS's "+
							"refusal cannot be explained by its own calendar ceiling.\n"+
							"  SIG=HARD:go_solved_dos_date_horizon %s\n  Go: int=%.2f paid=%.2f",
							len(gr.Schedule), lastY, cmd, gr.TotalInt, gr.TotalPaid)
					}
				} else if os.Getenv("PERSENSE_FUZZ_FLAKEDUMP") != "" {
					// GATED, AND THE GATE IS RULE 7 — NEVER CHANGE DEFAULT HARNESS
					// OUTPUT. This line was ungated for one build, and
					// `paired_regression.sh` promptly reported FIXED 0 / STILL 21 /
					// NEW 195 against an IDENTICAL ENGINE. Nothing had regressed:
					// the gate keys on `grep -oE "amort_oracle .*"` across the
					// WHOLE test output, so it cannot tell a new SIGNAL from a new
					// LOG LINE that happens to quote a reproducing command. Every
					// one of the 195 was this advisory.
					//
					// Two things follow, and both are worth more than the line was.
					// First, the neighbouring fz5NoTotals dump is gated behind this
					// same variable with a comment saying exactly this, four lines
					// away from where the ungated line was added — the rule was
					// written at the site and still got broken, which is why it is
					// restated here rather than cross-referenced.
					// Second, the LIMITATION is now measured rather than assumed:
					// the paired gate's set-difference is over reproducing commands,
					// not over severities, so ANY new per-case log line reads as a
					// regression. Recorded in docs/harness_policy.md R16.
					//
					// The IN-SCOPE branch above stays UNGATED on purpose: it is a
					// t.Errorf, a HARD failure, and a hard failure that only appears
					// under an env var is R12's "a skip is not a pass" wearing a
					// different hat. A gate going red on a genuine new HARD signal
					// is the gate working.
					t.Logf("DOS refused on a date horizon, port answered, port horizon %d "+
						"(out of scope)\n  SIG=ADVISORY:go_solved_dos_date_horizon_out_of_scope %s",
						lastY, cmd)
				}
			}
			continue
		case fz5NoTotals:
			// DOS gave a payment and a valid horizon but refused to total the
			// schedule. Not a flake and not a date overflow — its own class, and
			// currently uninvestigated. Gated dump for the same reason as the
			// flake dump below (rule 7: never change default harness output).
			noTotals++
			// ROUND 22: the second silent bucket. Unlike the date-horizon case
			// this one is NOT a refusal — DOS answered the payment and the
			// horizon and only declined the totals — so the port producing a
			// schedule is expected and is not a signal on its own. What IS worth
			// counting is how often it happens, because round 18b's finding that
			// 4 of 18 dumped no-totals cases return REAL totals when re-run
			// (candidate defect #11, still open) means this bucket is partly
			// mis-labelled, and the size of the population being mis-labelled is
			// the first thing anyone will want when it is finally adjudicated.
			if goOK {
				noTotalsGoSolved++
			}
			if os.Getenv("PERSENSE_FUZZ_FLAKEDUMP") != "" {
				t.Logf("SIG=INFO:dos_payment_but_no_totals\n  %s", cmd)
			}
			continue
		case fz5NonConverge:
			// DOS's Iterate gave up. There is no oracle answer to compare against,
			// so this can never be a hard failure — but it is not nothing either.
			// If Go's schedule genuinely retires the loan, the port found a root
			// DOS's bisection missed (the known solver-fidelity gap, benign). If
			// Go "solved" and the loan does NOT retire, the port invented a
			// schedule out of a screen with no answer, which is the same class of
			// bug as Go-solved-DOS-refused and is worth naming in the report.
			nonConv++
			if goOK {
				if retires(gr, amount) {
					nonConvGoRetires++
				} else {
					nonConvGoSpurious++
					t.Logf("DOS non-converge, Go returned a NON-RETIRING schedule [%s]\n  SIG=ADVISORY:dos_nonconverge_go_nonretiring %s\n"+
						"  Go: final balance %.2f on %d rows (int=%.2f paid=%.2f)",
						sig, cmd, gr.Schedule[len(gr.Schedule)-1].Principal, len(gr.Schedule),
						gr.TotalInt, gr.TotalPaid)
				}
			}
			continue
		case fz5Refused:
			refused++
			// HARD, one-directional: Go must not produce a schedule for a screen
			// DOS refuses outright.
			if goOK {
				goSolvedDosRefused++
				// NOT counted in the era split: this branch `continue`s before
				// `checked++`, so the case never enters ACTUALLY COMPARED, and a
				// numerator counting a case its denominator excludes is the
				// rule-9 mistake in miniature.
				//
				// Note the POOLED HARD rate already has this shape —
				// go_solved_dos_refused is scored against a denominator that
				// omits it. Small, but real, and written down here rather than
				// silently reproduced in the new counter.
				t.Errorf("Go produced a schedule where DOS REFUSED the screen [%s]\n  SIG=HARD:go_solved_dos_refused %s\n"+
					"  Go: int=%.2f paid=%.2f rows=%d", sig, cmd, gr.TotalInt, gr.TotalPaid,
					len(gr.Schedule))
			}
			continue
		}

		checked++

		// Era bucket for this compared case, from the PORT'S OWN resolved
		// horizon. Deliberately not recomputed from the draw tokens: R2 —
		// "any date the harness computes must be computed the way the engine
		// computes it", and the rule that has returned six defects is the one
		// about harness-derived dates. gr.Schedule's last row and gr.Balloons
		// are the engine's answers, not a second derivation of them.
		caseEra := 0
		if fz5MaxYear(gr) > 2099 {
			caseEra = 1
		}
		eraCompared[caseEra]++
		caseHard := false
		// R7, docs/harness_policy.md — COVERAGE OF WHAT WAS ACTUALLY COMPARED.
		// Standing rule 8 ("ask what the generator CANNOT produce") has returned a
		// defect or a harness bug seven times out of seven, but it was applied by
		// hand, by whoever thought to ask. fuzzer5 had never drawn a schedule over
		// 25 years until 2026-07-31 (§52); removing that bound moved the measured
		// divergence rate from 1 in 3,600 to 1 in 290 with NO code change. This
		// records the envelope so the next such bound is a failing assertion
		// rather than a discovery. It counts COMPARED cases only: a range the
		// generator emits but the harness never compares is not coverage.
		covN++
		if in.Loan.NPeriods < covMinN || covMinN == 0 {
			covMinN = in.Loan.NPeriods
		}
		if in.Loan.NPeriods > covMaxN {
			covMaxN = in.Loan.NPeriods
		}
		covPerYr[int(in.Loan.PerYr)]++
		if yrs := covYears(in.Loan); yrs > 0 {
			if yrs < covMinYrs || covMinYrs == 0 {
				covMinYrs = yrs
			}
			if yrs > covMaxYrs {
				covMaxYrs = yrs
			}
		}
		covMode[fz5ModeName[mode]]++

		cs := classes[sig]
		if cs == nil {
			cs = &classStat{}
			classes[sig] = cs
		}
		cs.n++

		if !goOK {
			goRefusedDosSolved++
			// ROUND 34: this branch `continue`s, so the verdict line below is never
			// reached and the analyzer would learn nothing about the case. It is
			// inside `checked` but was NEVER COMPARED (no totals comparison ran),
			// which is the ledger's `actuallyCompared = checked - goRefusedDosSolved`
			// distinction. Emitting it with compared=false keeps the exclusion
			// applied to BOTH columns explicitly rather than by absence.
			if os.Getenv("FZ5CASEDUMP") != "" {
				fmt.Fprintf(os.Stderr, "FZ5VERDICT %d hard=false era=%d compared=false\n",
					c, caseEra)
			}
			errText := "nil"
			if gr.Err != nil {
				errText = gr.Err.Error()
			}
			// A backward mode that never reached Amortize failed in the SOLVER,
			// and the solver's message is the diagnostic one — Amortize's would
			// just say the cell it was handed is still blank.
			if goSolveErr != nil {
				errText = "solve: " + goSolveErr.Error()
			}
			t.Logf("DOS solved, Go refused [%s]: %v\n  SIG=ADVISORY:dos_solved_go_refused %s", sig, errText, cmd)
			continue
		}

		// ---- Signal 1: totals ----
		// Scaled tolerance: the DOS-vs-port rounding tail grows with the number
		// of rows and with the size of the schedule, and a stacked-option
		// schedule can run tens of thousands of rows once weekly prepayments are
		// in play. The floor is a dollar; the slope is 5 basis points of a
		// basis point of DOS's own total.
		intTol := math.Max(1.0, 5e-4*math.Abs(dos.interest))
		paidTol := math.Max(1.0, 5e-4*math.Abs(dos.paid))
		dInt := math.Abs(gr.TotalInt - dos.interest)
		dPaid := math.Abs(gr.TotalPaid - dos.paid)
		noteTol("totals:interest", dInt, intTol, dos.interest)
		noteTol("totals:paid", dPaid, paidTol, dos.paid)
		if dInt > intTol || dPaid > paidTol {
			cs.diverge++
			// A case in a class that will be reported DIVERGENT carries a HARD
			// signal too — the report is per CLASS, but the divergence is per
			// case, and the era split counts cases.
			caseHard = true
			if dInt > cs.worstInt || dPaid > cs.worstPaid {
				if dInt > cs.worstInt {
					cs.worstInt = dInt
				}
				if dPaid > cs.worstPaid {
					cs.worstPaid = dPaid
				}
				cs.worstCmd = fmt.Sprintf("%s\n    DOS int=%.2f paid=%.2f | Go int=%.2f paid=%.2f "+
					"(dInt=%.2f dPaid=%.2f)", cmd, dos.interest, dos.paid, gr.TotalInt, gr.TotalPaid,
					dInt, dPaid)
			}
		}

		// ---- Signal 2: DOS's terminating balloon (§46) ----
		dosTack, dosHasTack := dos.tack()
		var goTack ResolvedBalloon
		goHasTack := false
		for _, b := range gr.Balloons {
			if b.TackedOn {
				goTack, goHasTack = b, true
			}
		}
		switch {
		case dosHasTack && goHasTack:
			tackAgree++
			wantDate := fmt.Sprintf("%d/%d/%d", int(goTack.Date.Time.Month()),
				goTack.Date.Time.Day(), goTack.Date.Time.Year())
			// HARNESS DEFECT #10 (round 18). The tolerance used to read
			//
			//	tackTol := math.Max(0.05, 1e-5*math.Abs(amount))
			//
			// with the comment "the tack amount is a balance, so scale the
			// tolerance to the loan". That premise is FALSE on this generator.
			// The tack is the balance at the schedule's terminating date, and a
			// payment drawn at 0.85x fair over a horizon reaching 166 years does
			// not amortize — the balance grows exponentially instead. Measured
			// over the `non` arm (seeds 60000-60039, N=400, 11,055 compared):
			// |tack| / |amount| has a MEDIAN of 59.9 and a MAXIMUM of 1,572,380.
			// A $2-4 absolute tolerance against a $147,799,574,916 balance demands
			// agreement to 1.4e-11 relative, which nothing downstream of a
			// row-by-row walk can deliver.
			//
			// The effect was not a slow leak: 52 of the 67 `non`-arm balloon
			// signals (78%) agreed to better than 1e-2 RELATIVE, most to 1e-7,
			// and every one was reported as SIG=HARD. `balloon_value_differs` is
			// the largest class in the standing residual, so the mis-scaling was
			// inflating the project's headline defect count — and, because `non`
			// is the mode that produces the runaway balances, it manufactured the
			// x2.40 `non` enrichment that round 17 named the project's only
			// defensible frontier. Measured honestly the REAL rate is HIGHER in
			// `noterm` (1 in 265) than in `non` (1 in 737).
			//
			// The fix compares the tack the way every OTHER value in this test is
			// compared: against a tolerance scaled to the value being compared.
			// The tack is the terminal point of the same row-by-row accumulation
			// that produces TotalInt and TotalPaid, so it inherits their error
			// model and reuses their slope (5e-4) rather than inventing a
			// constant. The loan-scaled term is KEPT, so wherever the old premise
			// actually held (|tack| <= 0.02 x amount) the tolerance is unchanged
			// and no sensitivity is lost.
			//
			// What this must NOT do is silence a real disagreement, so the
			// surviving population was enumerated rather than assumed: 15 `non`
			// cases (rel 1.9e-2 .. 1.0) and 29 of 30 `noterm` cases survive, the
			// latter including all 27 sign-flipped tacks. Pinned both directions
			// in TestFz5TackToleranceScaling.
			tackTol := math.Max(math.Max(0.05, 1e-5*math.Abs(amount)),
				5e-4*math.Abs(dosTack.amount))
			noteTol("balloon:tack", math.Abs(goTack.Amount-dosTack.amount), tackTol, dosTack.amount)
			if math.Abs(goTack.Amount-dosTack.amount) > tackTol || wantDate != dosTack.date {
				tackValueDiff++
				// dstatus/astatus are PRINTED, not asserted. They used to be the
				// hardcoded string "dstatus/astatus outp", which is the append
				// path's signature — and every one of §59's 27 cases is the MERGE
				// path (dstatus=inp), so the message stated the opposite of the
				// truth on the entire population it was describing. Round 18 read
				// that string back as evidence and published a root cause built on
				// it; round 18c caught it only by re-probing the oracle by hand.
				// The parser had both fields all along (see the `tack` helper) and
				// the sibling branch below already printed them.
				//
				// The general rule this is an instance of: an instrument may print
				// only what it has actually read. A constant standing where a
				// measurement belongs is indistinguishable from a measurement in
				// the log, which is what makes it dangerous rather than merely
				// untidy. Harness defects #10, #11 and this one are the same
				// family. See docs/harness_policy.md R13.
				caseHard = true
				t.Errorf("terminating balloon differs [%s]\n  SIG=HARD:balloon_value_differs %s\n"+
					"  DOS: %s %.4f (row %d, dstatus %d astatus %d, %s, nballoons=%d nlines=%d)\n"+
					"  Go : %s %.4f", sig, cmd, dosTack.date, dosTack.amount, dosTack.idx,
					dosTack.dstatus, dosTack.astatus, tackPathName(dosTack.dstatus),
					dos.nballoons, dos.nlines, wantDate, goTack.Amount)
			}
		case dosHasTack && !goHasTack:
			tackDosOnly++
			caseHard = true
			t.Errorf("DOS tacked a terminating balloon the port did not [%s]\n  SIG=HARD:balloon_dos_only %s\n"+
				"  DOS row %d: %s %.4f (dstatus %d astatus %d), nballoons=%d nlines=%d\n"+
				"  This is the §46 gate: Amortize.pas:1043 requires fancy AND "+
				"PayAmtStatus >= defp AND an over-specified schedule.",
				sig, cmd, dosTack.idx, dosTack.date, dosTack.amount,
				dosTack.dstatus, dosTack.astatus, dos.nballoons, dos.nlines)
		case !dosHasTack && goHasTack:
			tackGoOnly++
			caseHard = true
			t.Errorf("the port tacked a terminating balloon DOS did not [%s]\n  SIG=HARD:balloon_go_only %s\n"+
				"  Go: %s %.4f — DOS's grid has no outp/outp row",
				sig, cmd, goTack.Date.Time.Format("1/2/2006"), goTack.Amount)
		}

		// ---- Signal 3: the post-FirstPass horizon ----
		// Soft everywhere EXCEPT the two term-solving modes, where the horizon
		// IS the answer and signal 4 below promotes it to a hard failure.
		if mode != fz5ModeTerm && mode != fz5ModeN &&
			dos.nPeriods > 0 && gr.NPeriods > 0 && dos.nPeriods != gr.NPeriods {
			t.Logf("nperiods differ [%s]: DOS %d, Go %d\n  SIG=ADVISORY:nperiods_differ %s", sig, dos.nPeriods, gr.NPeriods, cmd)
		}

		// ---- Signal 4: the backward-solved cell ----
		// The solved value is written back into the screen cell and is what the
		// user reads, so it has to agree in its own right. The totals comparison
		// cannot stand in for it: near the root the schedule is by construction
		// insensitive to the cell being solved, so a wrong solve on a flat
		// plateau — exactly where the bracket-free secant is least reliable —
		// moves the totals by less than their own tolerance while putting a
		// visibly wrong number on the screen.
		switch mode {
		case fz5ModeAmount:
			if dos.hasSolvedAmt && goSolvedOK {
				solveChecked++
				// Same shape as the pass-3 A4 probe: a cent, plus 2ppm of the
				// balance for the rounding tail on large principals.
				tol := 0.01 + 2e-6*math.Abs(dos.solvedAmt)
				noteTol("solve:amount", math.Abs(goSolved-dos.solvedAmt), tol, dos.solvedAmt)
				if math.Abs(goSolved-dos.solvedAmt) > tol {
					solveDiff++
					caseHard = true
					t.Errorf("solved AMOUNT differs [%s]\n  SIG=HARD:solved_amount_differs %s\n  DOS %.6f | Go %.6f (delta=%+.6f)",
						sig, cmd, dos.solvedAmt, goSolved, goSolved-dos.solvedAmt)
				}
			}
		case fz5ModeRate:
			if dos.hasSolvedRate && goSolvedOK {
				solveChecked++
				// 5e-6 absolute (~6e-5 relative at ordinary rates). Tighter than
				// this is below the stop tolerance of DOS's own refinement, so it
				// would report the solver's last step rather than a divergence.
				noteTol("solve:rate", math.Abs(goSolved-dos.solvedRate), 5e-6, dos.solvedRate)
				if math.Abs(goSolved-dos.solvedRate) > 5e-6 {
					solveDiff++
					caseHard = true
					t.Errorf("solved RATE differs [%s]\n  SIG=HARD:solved_rate_differs %s\n  DOS %.10f | Go %.10f (delta=%+.2e)",
						sig, cmd, dos.solvedRate, goSolved, goSolved-dos.solvedRate)
				}
			}
		case fz5ModeTerm, fz5ModeN:
			// Both cells, not just the count: `non` derives n from the typed last
			// date and `noterm` derives both, and a grid that is off by a period
			// can still land on the right COUNT with the wrong final date.
			if dos.nPeriods > 0 {
				solveChecked++
				goLast := fmt.Sprintf("%d/%d/%d", int(gr.LastDate.Time.Month()),
					gr.LastDate.Time.Day(), gr.LastDate.Time.Year())
				if dos.nPeriods != gr.NPeriods || dos.lastDate != goLast {
					solveDiff++
					caseHard = true
					t.Errorf("solved TERM differs [%s]\n  SIG=HARD:solved_term_differs %s\n  DOS n=%d last=%s | Go n=%d last=%s",
						sig, cmd, dos.nPeriods, dos.lastDate, gr.NPeriods, goLast)
				}
			}
		}

		// ---- R41: SNAPSHOT THE FOUR-QUESTION VERDICT ----
		//
		// ⚠️ ORDERING DEPENDENCY, DELIBERATE AND FRAGILE. Every one of the SEVEN
		// `caseHard = true` sites belonging to the four STANDING questions (the
		// totals class, the three balloon/tack signals, and the three solved
		// amount/rate/term signals) lies ABOVE this line; the ten sites belonging
		// to Signals 5, 6 and 7 all lie BELOW it. So this snapshot is exactly the
		// pre-39e verdict for this case.
		//
		// 🚨 IF YOU ADD A NEW SIGNAL, PUT IT BELOW THIS LINE, or set caseHardQ4
		// explicitly at its site. A four-question signal added below would be
		// silently missing from the Q4 column and the R41 delta would overstate
		// the question set's effect. TestQ4SnapshotPrecedesEveryNewSignal pins the
		// ordering so this cannot rot unnoticed.
		caseHardQ4 := caseHard

		// ---- Signal 5: DOS's regular payment (h^.payamt) ----
		// ADDED 2026-08-08, and the history is the point: `dos.payment` had been
		// PARSED BY THIS FILE AND THROWN AWAY since the file was written — its
		// only readers were the flake sentinels. The one arm in the repo that
		// compared DOS's payment was the PLAIN differential, which by
		// construction has no options; so the top-line payment was unscored on
		// every optioned screen, and two shipped defects lived there (the UI's
		// modal-row reconstruction, and the APR pass being fed the same modal —
		// round 39). The oracle's `payment` field IS `h^.payamt` read after
		// MakeTable (amort_oracle.pas:1289), and `gr.RegularPayment` is the R39
		// transport of the same cell. Zero extra spawns: the number was in the
		// dump all along. R40 candidate: ask which outputs the instrument never
		// reads back.
		if dos.payment == 0 || gr.RegularPayment == 0 {
			paySkipZero++ // counted, not silent (R16)
		} else {
			payChecked++
			paySolvedArm := mode == fz5ModePaySolve
			if paySolvedArm {
				payCheckedSolved++
			} else {
				payCheckedTyped++
			}
			// A cent plus 2ppm for the rounding tail — the solved-amount shape.
			// DOS prints 4 decimals, so print-rounding is 5e-5 at worst.
			pTol := 0.011 + 2e-6*math.Abs(dos.payment)
			dPay := math.Abs(gr.RegularPayment - dos.payment)
			noteTol("payment:regular", dPay, pTol, dos.payment)
			if dPay > pTol {
				payDiff++
				if paySolvedArm {
					payDiffSolved++
				} else {
					payDiffTyped++
				}
				caseHard = true
				t.Errorf("regular payment differs [%s]\n  SIG=HARD:regular_payment_differs %s\n"+
					"  DOS %.4f | Go %.4f (delta=%+.4f)\n"+
					"  This is the TOP-LINE Pmt Amount cell. If the totals agree and this "+
					"does not, check what the transport (AmortResult.RegularPayment) was "+
					"set from — a schedule statistic here is the round-39 defect returning.",
					sig, cmd, dos.payment, gr.RegularPayment, gr.RegularPayment-dos.payment)
			}
		}

		// ---- Signal 7: the solved adjustment cells ----
		// ADDED 2026-08-08. `adjdump` had ZERO consumers for the life of the
		// project (coverage audit, fix B7) — the round-39 "New Amount is never
		// filled in but it is in DOS" report lived in exactly this unread block.
		// The comparison is against the R39 transport (gr.Adjustments), which is
		// what the API echoes and the UI paints — an END-TO-END cell check, not
		// an engine internal.
		//
		// Semantics, from the Pascal (do not "simplify" these):
		//   - both sides are DATE-SORTED (DOS SortAdj; both Go engines sort), so
		//     index alignment is legitimate HERE, unlike in the UI.
		//   - amtStatus==1 (outp) means DOS DISPLAYS a computed amount -> the
		//     echo must carry it with AmountSolved=true. amtStatus==3 (inp) is
		//     the user's own number -> echoed but NOT solved. Same for the rate.
		//   - rate tolerance 5e-6 (the solved-rate signal's); amount a cent +
		//     2ppm (the solved-amount signal's).
		//
		// WARNING - KNOWN RED AT INTRODUCTION (NF-1, ROUND39D section 2): the
		// piecewise engine's echo drops the New Amount on ~30-38% of adjustment
		// rows (AO6-carrying screens and in-advance/R78/daily routes). This
		// signal is DELIBERATELY HARD anyway - it is the regression gate NF-1's
		// fix must turn green, and note #40 already says PERSENSE_FUZZ=1 is red
		// by design. Do NOT downgrade it to advisory to make a run look clean;
		// that is the silent bucket this signal exists to end.
		if len(dos.adjRows) > 0 || len(gr.Adjustments) > 0 {
			adjScreens++
			// ROUND 41 item 0m(iii): the engine label comes from the RESULT, not
			// from a trace. "" means Amortize returned before the routing branch.
			adjEng := gr.EngineUsed
			if adjEng == "" {
				adjEng = "unrouted"
			}
			adjScreensByEngine[adjEng]++
			if len(dos.adjRows) != len(gr.Adjustments) {
				adjRowsDiff++
				adjDiffByEngine[adjEng]++
				caseHard = true
				t.Errorf("adjustment echo COUNT differs [%s]\n  SIG=HARD:adj_echo_count %s\n"+
					"  DOS %d rows | Go %d rows", sig, cmd, len(dos.adjRows), len(gr.Adjustments))
			} else {
				for i, dr := range dos.adjRows {
					ga := gr.Adjustments[i]
					adjRowsChecked++
					adjRowsByEngine[adjEng]++
					goDate := fmt.Sprintf("%d/%d/%d", int(ga.Date.Time.Month()),
						ga.Date.Time.Day(), ga.Date.Time.Year())
					if dr.date != goDate {
						adjRowsDiff++
						adjDiffByEngine[adjEng]++
						caseHard = true
						t.Errorf("adjustment echo DATE differs [%s]\n  SIG=HARD:adj_echo_date %s\n"+
							"  row %d: DOS %s | Go %s", sig, cmd, i+1, dr.date, goDate)
						continue
					}
					aTol := 0.011 + 2e-6*math.Abs(dr.amount)
					switch {
					case dr.amtStatus == 1 && !ga.AmountSolved:
						adjRowsDiff++
						adjDiffByEngine[adjEng]++
						caseHard = true
						t.Errorf("adjustment NEW AMOUNT missing [%s]\n  SIG=HARD:adj_amount_missing %s\n"+
							"  row %d (%s): DOS displays %.6f (amtstatus 1) | Go echoes solved=false\n"+
							"  Round-39 blank-cell class (NF-1 while open).",
							sig, cmd, i+1, dr.date, dr.amount)
					case dr.amtStatus == 1 && math.Abs(ga.Amount-dr.amount) > aTol:
						adjRowsDiff++
						adjDiffByEngine[adjEng]++
						caseHard = true
						t.Errorf("adjustment NEW AMOUNT differs [%s]\n  SIG=HARD:adj_amount_differs %s\n"+
							"  row %d (%s): DOS %.6f | Go %.6f (delta=%+.4f)",
							sig, cmd, i+1, dr.date, dr.amount, ga.Amount, ga.Amount-dr.amount)
					case dr.amtStatus == 0 && ga.AmountSolved:
						adjRowsDiff++
						adjDiffByEngine[adjEng]++
						caseHard = true
						t.Errorf("adjustment amount INVENTED [%s]\n  SIG=HARD:adj_amount_invented %s\n"+
							"  row %d (%s): DOS leaves the cell EMPTY (amtstatus 0) | Go echoes %.6f solved=true",
							sig, cmd, i+1, dr.date, ga.Amount)
					}
					switch {
					case dr.rateStatus == 1 && !ga.RateSolved:
						adjRowsDiff++
						adjDiffByEngine[adjEng]++
						caseHard = true
						t.Errorf("adjustment solved RATE missing [%s]\n  SIG=HARD:adj_rate_missing %s\n"+
							"  row %d (%s): DOS displays %.10f (ratestatus 1) | Go echoes solved=false",
							sig, cmd, i+1, dr.date, dr.rate)
					case dr.rateStatus == 1 && math.Abs(ga.Rate-dr.rate) > 5e-6:
						adjRowsDiff++
						adjDiffByEngine[adjEng]++
						caseHard = true
						t.Errorf("adjustment solved RATE differs [%s]\n  SIG=HARD:adj_rate_differs %s\n"+
							"  row %d (%s): DOS %.10f | Go %.10f (delta=%+.2e)",
							sig, cmd, i+1, dr.date, dr.rate, ga.Rate, ga.Rate-dr.rate)
					case dr.rateStatus == 0 && ga.RateSolved:
						adjRowsDiff++
						adjDiffByEngine[adjEng]++
						caseHard = true
						t.Errorf("adjustment rate INVENTED [%s]\n  SIG=HARD:adj_rate_invented %s\n"+
							"  row %d (%s): DOS leaves the cell EMPTY (ratestatus 0) | Go echoes %.10f solved=true",
							sig, cmd, i+1, dr.date, ga.Rate)
					}
				}
			}
		}

		// ---- Signal 6: the APR ----
		// ADDED 2026-08-08. Until this line NO ARM IN THE REPOSITORY ever asked
		// the oracle for the APR — all five APR-vs-oracle tests are hand fixtures
		// with a TYPED payment, the exact arm where round 39c measured the modal
		// defect at 0/1,770. Meanwhile this generator was already emitting `pts=`
		// on ~73% of screens; only the question was missing. The coverage audit's
		// prototype of this probe, run on the PRE-fix tree, found 19 divergences
		// in 1,298 compared cases, FIVE of them with this file's totals signal
		// green — worst 2.60 PERCENTAGE POINTS at dInt=$0.0000. No tolerance
		// policy over the schedule can reach a defect that lives in a field
		// nobody reads.
		//
		// Tolerance: the oracle prints 6 decimals (5e-7 print rounding) and the
		// UI renders (apr*100).toFixed(4) (1.5e-6 in fraction terms). 2e-6 is
		// one UI digit with headroom; a real defect here measured in whole
		// percentage points.
		// ---- ROUND 43, item 0m(i) / R49: the discriminating population ----
		// Computed BEFORE the APR comparison and independently of it, so it
		// measures the SAMPLE, not the outcome. Three nested counts, so a zero
		// says WHICH clause emptied the population rather than just "zero".
		if gr.Err == nil && points > 0 {
			aprDiscrimPts++
			if mode == fz5ModePaySolve {
				aprDiscrimPaySolve++
				if gr.EngineUsed == "dosport" {
					aprDiscrimDosport++
					// The modal reconstruction, verbatim: payoffRegularPayment's
					// fallback when the payment is not a user input. This is the
					// number the round-39 defect fed the APR pass.
					counts, amtOf := map[int64]int{}, map[int64]float64{}
					bestKey, bestN := int64(0), 0
					for i := range gr.Schedule {
						r := gr.Schedule[i]
						if r.PayNum < 1 || r.PayAmt <= 0 {
							continue
						}
						k := int64(r.PayAmt*100 + 0.5)
						counts[k]++
						amtOf[k] = r.PayAmt
						if counts[k] > bestN {
							bestN, bestKey = counts[k], k
						}
					}
					if bestN > 0 && gr.RegularPayment > 0 {
						modal := amtOf[bestKey]
						d := math.Abs(modal - gr.RegularPayment)
						// Same floor Signal 5 uses for the same cell.
						if d > 0.011+2e-6*math.Abs(gr.RegularPayment) {
							aprDiscrim++
							if d > aprDiscrimWorst {
								aprDiscrimWorst = d
							}
							if os.Getenv("FZ5DISCRIMDUMP") != "" {
								t.Logf("FZ5DISCRIM modal=%.4f regular=%.4f delta=%+.4f %s",
									modal, gr.RegularPayment, modal-gr.RegularPayment, cmd)
							}
						}
					}
				}
			}
		}

		if points >= 0 && gr.Err == nil {
			aprEligible++
			if dosAPR, dosStatus, ok := runAPR(append(append([]string{}, args...), "apr")); !ok {
				aprSkipSpawn++ // ERR / timeout / apr-no-converge — DOS gave no answer
			} else if dosStatus != 1 {
				aprSkipStatus++ // not outp: DOS did not actually solve it
			} else if !gr.APRConverged {
				aprGoNoConverge++
				t.Logf("APR: DOS solved %.6f, Go did not converge [%s]\n"+
					"  SIG=ADVISORY:apr_go_nonconverged_dos_solved %s", dosAPR, sig, cmd)
			} else {
				aprCompared++
				dAPR := math.Abs(gr.APR - dosAPR)
				noteTol("apr", dAPR, 2e-6, dosAPR)
				if dAPR > 2e-6 {
					aprDiff++
					caseHard = true
					t.Errorf("APR differs [%s]\n  SIG=HARD:apr_differs %s apr\n"+
						"  DOS %.6f | Go %.8f (delta=%+.2e = %+.4f pct pts)",
						sig, cmd, dosAPR, gr.APR, gr.APR-dosAPR, (gr.APR-dosAPR)*100)
				}
			}
		}

		if caseHard {
			eraHard[caseEra]++
		}
		if caseHardQ4 {
			eraHardQ4[caseEra]++
		}
		// ROUND 41 — the per-engine contingency denominator, in-process.
		// Until now this pairing existed only in python, by matching GENGINE lines
		// against the FZ5ENGBEGIN/FZ5ENGEND bracket, and round 34's first draft of
		// that pairing inflated the faithful port's denominator by 23%. The label
		// is now transported on the result (R39 applied to the instrument), so the
		// fuzzer can count it directly and the analyzer has something to check
		// itself against (R36 — old instrument and new one, side by side).
		caseEngine := gr.EngineUsed
		if caseEngine == "" {
			caseEngine = "unrouted"
		}
		engCases[caseEngine]++
		if caseHard {
			engHard[caseEngine]++
		}
		// ROUND 34 — the compared case's VERDICT, by index. Together with
		// FZ5OUTCOME above and engine.go's GENGINE line this is the whole
		// contingency table: denominator (compared), numerator (hard), engine and
		// its full co-exclusion set, all from ONE arm run.
		//
		// `caseEra` is emitted because the headline rate is the IN-SCOPE one and a
		// table that pools the eras is measuring a different population (CAUTION 1).
		// `goRefusedDosSolved` cases are inside `checked` but were never compared,
		// so they are marked and the analyzer drops them from BOTH columns — the
		// same exclusion the ledger's `actuallyCompared` already makes, applied to
		// both columns rather than to the denominator alone.
		if os.Getenv("FZ5CASEDUMP") != "" {
			// Reaching here means goOK — the !goOK arm above emits its own
			// compared=false line and continues. So this is `actuallyCompared`.
			// `eng=` and `route=` are ROUND 41 additions and are the TRANSPORTED
			// values, not the bracket's. An analyzer that still derives the engine
			// from GENGINE can compare the two columns and assert they agree — that
			// cross-check is the point of emitting it here (R36).
			fmt.Fprintf(os.Stderr, "FZ5VERDICT %d hard=%v era=%d compared=true eng=%s route=%s\n",
				c, caseHard, caseEra, caseEngine, gr.RouteReason)
		}
	}

	// ---- Report ----
	// R5, docs/harness_policy.md — THE LEDGER MUST BALANCE.
	// Every generated case must land in exactly one terminal bucket. A case that
	// falls out silently is invisible: round 14's `firstPeriodDate` bug suppressed
	// 7 of 12 oracle comparisons in one half of the round-trip gate while the
	// other half reported five spurious failures, and the suppression was the half
	// nobody saw. A 5% divergence rate and a 50% one look identical if the
	// denominator is quietly shrinking, so the shortfall is reported explicitly
	// and a large one FAILS rather than passing quietly.
	// NOTE the shape here, which the first version of this ledger got wrong and
	// the ledger itself caught (seed 50117 reported UNACCOUNTED -1):
	// goRefusedDosSolved is incremented AFTER `checked++`, so it is a SUBSET of
	// checked, not a sibling bucket. Adding it double-counts.
	//
	// It also means the figure this harness has always called "compared" is
	// slightly overstated: a case where DOS solved and the PORT refused is
	// counted in `checked` even though no totals comparison ever ran. The honest
	// denominator for any divergence rate is `actuallyCompared` below.
	accounted := checked + refused + nonConv + horizon + flaked + skippedPlain + noTotals + oracleTimedOut
	unaccounted := N - accounted
	actuallyCompared := checked - goRefusedDosSolved
	t.Logf("cases: %d generated, %d compared | DOS refused %d, non-converged %d, date-horizon %d, flaked %d",
		N, checked, refused, nonConv, horizon, flaked)
	// ERA SPLIT (round 19). Reported as CASES, not as SIG instances: a divergent
	// option class emits one SIG line but represents cs.diverge separate cases,
	// and a rate whose numerator and denominator count different things is the
	// mistake rule 9 exists to prevent. Both columns here are cases.
	//
	// Printed unconditionally, including when a bucket is zero — R8/R13. A
	// bucket that only appears when non-empty is indistinguishable from a
	// bucket nobody measured.
	t.Logf("era split (cases, horizon keyed on the port's own resolved dates): "+
		"in-scope<=2099 compared=%d hard=%d | out-of-scope>2099 compared=%d hard=%d",
		eraCompared[0], eraHard[0], eraCompared[1], eraHard[1])
	// R41 — THE SAME CASES, SCORED BOTH WAYS. Read the two lines together or
	// neither: a rate is a statement about its QUESTION SET (rule 9).
	q4tot := eraHardQ4[0] + eraHardQ4[1]
	q7tot := eraHard[0] + eraHard[1]
	compTot := eraCompared[0] + eraCompared[1]
	t.Logf("R41 QUESTION-SET SPLIT (same cases, same tree, same run): "+
		"FOUR-question HARD in-scope=%d out=%d pooled=%d | "+
		"SEVEN-question HARD in-scope=%d out=%d pooled=%d | "+
		"compared pooled=%d | the three 39e signals add %d HARD cases (%+.0f%% on "+
		"the numerator) with NO code change — that movement is the INSTRUMENT, not "+
		"the port (R41).",
		eraHardQ4[0], eraHardQ4[1], q4tot,
		eraHard[0], eraHard[1], q7tot, compTot,
		q7tot-q4tot, pctDelta(q4tot, q7tot))

	t.Logf("ledger: generated %d = compared %d + refused %d + non-converged %d + "+
		"date-horizon %d + flaked %d + skipped-plain %d + no-totals %d + "+
		"dos-nontermination %d | UNACCOUNTED %d",
		N, checked, refused, nonConv, horizon, flaked, skippedPlain, noTotals,
		oracleTimedOut, unaccounted)
	// Reported unconditionally, including at zero. A run that lost seeds to
	// harness defect #9 was previously indistinguishable from a run that did not,
	// because the losing seeds produced no output at all.
	t.Logf("DOS non-termination (harness defect #9): %d of %d generated cases "+
		"exceeded the %s oracle budget and were bucketed, not fatal. "+
		"PERSENSE_FUZZ_FLAKEDUMP=1 dumps them.",
		oracleTimedOut, N, fz5OracleBudget)
	t.Logf("transient no-totals sentinel (harness defect #11): %d spawns hit the "+
		"-1/-1-with-valid-date sentinel and were respawned; %d CASES then returned "+
		"real totals and were compared. Those %d are comparisons the pre-18b "+
		"immediate-return silently discarded. A deterministic no-totals case "+
		"contributes 2 retries and 0 recoveries, so read the second number, not "+
		"the first.", noTotalsRetried, noTotalsRecovered, noTotalsRecovered)
	if oracleTimeouts != oracleTimedOut {
		t.Errorf("HARNESS BOOKKEEPING: runDump recorded %d oracle timeouts but the "+
			"loop bucketed %d. A timeout that does not reach its terminal bucket is "+
			"exactly the silent attrition defect #9 was about.",
			oracleTimeouts, oracleTimedOut)
	}
	t.Logf("of the %d reaching comparison, %d were Go-refused (no totals compared) "+
		"=> ACTUALLY COMPARED %d — use this as the denominator for any rate",
		checked, goRefusedDosSolved, actuallyCompared)
	if unaccounted < 0 || (N > 0 && float64(unaccounted) > 0.05*float64(N)) {
		t.Errorf("HARNESS LEDGER DOES NOT BALANCE: %d of %d generated cases (%.1f%%) "+
			"reached no terminal bucket. Cases are being dropped silently, which "+
			"makes every rate computed from this run meaningless. See "+
			"docs/harness_policy.md R5.",
			unaccounted, N, 100*float64(unaccounted)/float64(N))
	}
	if N > 0 && checked == 0 {
		t.Errorf("HARNESS COMPARED NOTHING: %d cases generated, 0 reached the oracle "+
			"comparison. A green run here proves nothing. See docs/harness_policy.md R5.", N)
	}
	t.Logf("dispatch: Go-solved-DOS-refused %d (hard fail), DOS-solved-Go-refused %d (logged)",
		goSolvedDosRefused, goRefusedDosSolved)
	t.Logf("DOS non-converge: Go retired the loan %d (benign solver gap), Go non-retiring %d (suspect)",
		nonConvGoRetires, nonConvGoSpurious)
	// ROUND 22 — THE TWO BUCKETS THAT USED TO ASK NOTHING.
	// These lines are the audit trail for R15 applied to this fuzzer's own
	// ledger. `date-horizon` and `no-totals` between them are around a tenth of
	// every generated population and neither had ever been asked what the PORT
	// did with the cases in it. Read the IN-SCOPE figure: an out-of-scope
	// date-horizon answer is the port's proleptic calendar outliving DOS's year
	// byte, which is §54/§55 and known; an IN-SCOPE one would be a screen a
	// client can reach where the two engines disagree about whether an answer
	// exists at all.
	t.Logf("date-horizon bucket (%d cases): port produced a schedule in %d, of which "+
		"IN SCOPE (port horizon <=2099) %d, of which DOS's own INTERNAL ERROR "+
		"(\"last payment not found\", §65) %d. The in-scope NON-internal-error "+
		"remainder is what fails: %d.",
		horizon, horizonGoSolved, horizonGoSolvedInScope, horizonGoSolvedInternalErr,
		horizonGoSolvedInScope-horizonGoSolvedInternalErr)
	t.Logf("no-totals bucket (%d cases): port produced a schedule in %d. Not a signal — "+
		"DOS answered the payment here and only declined the totals — but it sizes the "+
		"population candidate defect #11 says is partly mis-labelled.",
		noTotals, noTotalsGoSolved)

	if len(errBucket) > 0 {
		ekeys := make([]string, 0, len(errBucket))
		for k := range errBucket {
			ekeys = append(ekeys, k)
		}
		sort.Slice(ekeys, func(i, j int) bool { return errBucket[ekeys[i]] > errBucket[ekeys[j]] })
		for _, k := range ekeys {
			t.Logf("oracle ERR x%d: %s", errBucket[k], k)
		}
	}
	t.Logf("terminating balloon: agree %d, value-differs %d, DOS-only %d, Go-only %d",
		tackAgree, tackValueDiff, tackDosOnly, tackGoOnly)
	// Reported even when zero: a run whose backward-solve arms all fell out
	// before signal 4 (oracle refusal, Go solver error, both) would otherwise
	// look identical to a run where they agreed, and "0 checked" is the signal
	// that the widening is not actually exercising anything.
	t.Logf("backward solves: %d checked, %d differ", solveChecked, solveDiff)
	// ROUND 41 item 0m(ii) — the SOLVED stratum is the load-bearing one. The typed
	// stratum compares two copies of the user's input and can only catch a gross
	// plumbing break; do not read a typed-arm zero as evidence about the transport.
	t.Logf("regular payment (Signal 5): %d checked, %d differ, %d skipped (zero on a side)",
		payChecked, payDiff, paySkipZero)
	t.Logf("regular payment (Signal 5) BY STRATUM: SOLVED %d checked / %d differ | "+
		"TYPED %d checked / %d differ  <- the SOLVED column is the one that can "+
		"falsify the R39 transport; the TYPED column compares the input to itself",
		payCheckedSolved, payDiffSolved, payCheckedTyped, payDiffTyped)
	// R16/R20: a stratum that never fills is a silent bucket. 39e's headline "230
	// checked, 0 differ" was pooled, and nobody could tell whether the solved arm
	// contributed 200 cases or 2.
	// ⚠️ CONDITIONED ON THE MODE FILTER — the first version of this guard was a
	// DEFECT and the round-41 audit caught it (reproduced 6/6). A frontier arm run
	// with PERSENSE_FUZZ_MODES=noterm,non has payCheckedSolved == 0 BY
	// CONSTRUCTION, because `solve` is the only mode that leaves the payment
	// blank. The guard would then FAIL a perfectly valid run — and a gate that
	// fails on a legitimate configuration is exactly the kind of thing operators
	// learn to ignore, which is how a real signal gets lost. It fires only on an
	// UNFILTERED sweep, where an empty solved stratum really would mean the
	// generator stopped producing pay-solve screens.
	solveModeReachable := len(fz5ModeFilter) == 0
	if !solveModeReachable {
		for _, m := range fz5ModeFilter {
			if m == fz5ModePaySolve {
				solveModeReachable = true
				break
			}
		}
	}
	if N >= 200 && payChecked > 50 && payCheckedSolved == 0 && solveModeReachable {
		t.Errorf("SIGNAL 5 SOLVED STRATUM IS EMPTY: %d payment checks, ALL of them "+
			"typed, on a run where the pay-solve mode WAS reachable. The transport "+
			"claim then rests entirely on comparing the user's input to itself — the "+
			"near-vacuity the round-40 audit named (CAUTION 9). Either the generator "+
			"stopped drawing pay-solve screens or the stratification broke.",
			payChecked)
	}
	t.Logf("APR probe (Signal 6): %d eligible (pts + Go answered), %d compared, %d differ; "+
		"skipped %d (DOS no-answer/timeout) %d (DOS status!=1); %d Go-nonconverged-DOS-solved",
		aprEligible, aprCompared, aprDiff, aprSkipSpawn, aprSkipStatus, aprGoNoConverge)
	// ROUND 43, item 0m(i) / R49 — SIGNAL 6's CONTROL POPULATION, PRINTED EVERY
	// RUN so a future round can pick a seed instead of re-running 39e's failed
	// experiment. Read it as a funnel: pts>0 -> pay-solve -> dosport -> modal
	// differs. The last number is the only one that matters; the first three say
	// which clause emptied it.
	t.Logf("Signal 6 CONTROL POPULATION (R49): pts>0 %d -> pay-solve %d -> dosport %d "+
		"-> modal != RegularPayment %d (worst delta %.4f). "+
		"A negative control for Signal 6 is meaningful ONLY when the last number is > 0.",
		aprDiscrimPts, aprDiscrimPaySolve, aprDiscrimDosport, aprDiscrim, aprDiscrimWorst)
	// A PROBE THAT COMPARES NOTHING IS THE DEFECT IT EXISTS TO PREVENT (R16, R20,
	// and the 2026-08-08 coverage audit's whole finding). ~73% of generated
	// screens carry `pts=`, so on any real run a zero here means the spawn or the
	// parse broke — e.g. the `apr` token's output format moved — and the signal
	// would otherwise green-light forever. Same guard shape as
	// "HARNESS COMPARED NOTHING" below.
	if N >= 200 && aprEligible > 20 && aprCompared == 0 {
		t.Errorf("APR PROBE COMPARED NOTHING: %d eligible cases, 0 compared "+
			"(%d spawn-skips, %d status-skips). The probe is not measuring; fix it "+
			"or the APR surface silently goes back to unscored.",
			aprEligible, aprSkipSpawn, aprSkipStatus)
	}
	t.Logf("adjustment cells (Signal 7): %d screens with adjustment rows, %d rows checked, %d findings",
		adjScreens, adjRowsChecked, adjRowsDiff)
	// ROUND 41 item 0m(iii) — WITHOUT THIS SPLIT THE FINDINGS ARE UNUSABLE AS A
	// BASELINE. NF-1 is a KNOWN, OPEN piecewise defect (the echo drops the New
	// Amount on 30-38% of rows); a dosport finding is NF-1b or something new.
	// Pooled, the two are indistinguishable and every future run reads "5 findings"
	// with no way to tell progress from regression.
	for _, eng := range []string{"dosport", "piecewise", "unrouted"} {
		if adjScreensByEngine[eng] == 0 && adjRowsByEngine[eng] == 0 {
			continue
		}
		t.Logf("  Signal 7 [%s]: %d screens, %d rows checked, %d findings",
			eng, adjScreensByEngine[eng], adjRowsByEngine[eng], adjDiffByEngine[eng])
	}
	// ROUND 41 — the per-engine contingency table, from the TRANSPORTED label.
	// This is the same split START_HERE §2's table carries, produced in-process
	// instead of by a python pairing over stderr brackets. R36: if the analyzer's
	// bracket-derived numbers disagree with these, one of them is wrong.
	//
	// 🚨 READ THIS BEFORE QUOTING engHard[dosport]. The stamp names the engine that
	// built the TABLE. On the BACKWARD modes (norate/noamt/noterm) the compared
	// cell was solved BEFORE that call, by solvers that dosport_entry.go:532 forces
	// onto piecewise. So a divergence produced by a piecewise SOLVER can be booked
	// against dosport, and the error is in the flatters-the-port direction —
	// round 34's exact failure mode, in a different disguise. The round-41 audit
	// raised this and it is NOT fixed: it is a property of the question "which
	// engine built the table", which is the only question a single label can
	// answer. This caveat applies EQUALLY to every GENGINE-bracket number the
	// project has already published; the transport did not introduce it, it just
	// made it visible. Stratify by MODE before quoting a per-engine rate.
	// SORTED — Go map iteration order is randomised per run, and the round-41 audit
	// measured this loop emitting dosport-first in 3 runs and piecewise-first in 2.
	// A log whose line order moves between runs is a log a diff cannot be taken
	// against, and every cross-round comparison this project makes is a diff.
	engKeys := make([]string, 0, len(engCases))
	for eng := range engCases {
		engKeys = append(engKeys, eng)
	}
	sort.Strings(engKeys)
	for _, eng := range engKeys {
		t.Logf("  engine [%s]: %d compared cases, %d HARD", eng, engCases[eng], engHard[eng])
	}
	if N >= 200 && adjScreens > 10 && adjRowsChecked == 0 {
		t.Errorf("ADJUSTMENT PROBE COMPARED NOTHING: %d adjustment-bearing screens, 0 rows "+
			"checked. adjdump is not being parsed or every screen had a count mismatch — "+
			"either way the cell surface went back to unscored.", adjScreens)
	}
	if N >= 200 && checked > 50 && payChecked == 0 {
		t.Errorf("PAYMENT PROBE COMPARED NOTHING: %d compared cases, 0 payment checks. "+
			"dos.payment or gr.RegularPayment is always zero — the transport or the "+
			"parse broke.", payChecked)
	}

	// ---- R10: tolerance headroom ----
	// `maxpass` is the largest |delta|/tol among cases this tolerance called
	// AGREEING; `minfail` the smallest among those it called DIVERGING. The
	// number that matters is the GAP between them. Orders of magnitude means the
	// constant is reporting a verdict the data already made; a gap near 1 means
	// the constant IS the verdict. `near` counts passing cases inside the top
	// decade (ratio > 0.1) — a tolerance with many of those is load-bearing even
	// if its gap happens to look wide on this seed.
	tolKeys := make([]string, 0, len(tolStats))
	for k := range tolStats {
		tolKeys = append(tolKeys, k)
	}
	sort.Strings(tolKeys)
	for _, k := range tolKeys {
		ts := tolStats[k]
		gap := "n/a (no divergence)"
		if !math.IsInf(ts.minFail, 1) && ts.maxPass > 0 {
			gap = fmt.Sprintf("%.0fx", ts.minFail/ts.maxPass)
		}
		spread := "n/a"
		if !math.IsInf(ts.minRel, 1) && ts.minRel > 0 {
			spread = fmt.Sprintf("%.1e", ts.maxRel/ts.minRel)
		}
		// SPREAD is the defect-#10 detector and the number to read first: the
		// ratio between the loosest and tightest RELATIVE precision this constant
		// demanded anywhere in the run. Single digits means it asked every case
		// the same question. The old tack tolerance scores ~1e10 here.
		t.Logf("tolerance [%s]: judged %d (pass %d) | SPREAD of tol/|value| %s "+
			"(%.2e..%.2e) | maxpass %.2e minfail %.2e gap %s | passing within one "+
			"decade of tol %d",
			k, ts.n, ts.pass, spread, ts.minRel, ts.maxRel,
			ts.maxPass, ts.minFail, gap, ts.nearMiss)
	}

	covKeys := make([]string, 0, len(blockCover))
	for k := range blockCover {
		covKeys = append(covKeys, k)
	}
	sort.Strings(covKeys)
	var cov []string
	for _, k := range covKeys {
		cov = append(cov, fmt.Sprintf("%s=%d", k, blockCover[k]))
	}
	t.Logf("block coverage: %s", strings.Join(cov, " "))

	keys := make([]string, 0, len(classes))
	for k := range classes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := classes[keys[i]], classes[keys[j]]
		if a.diverge != b.diverge {
			return a.diverge > b.diverge
		}
		return keys[i] < keys[j]
	})
	totalDiverge := 0
	for _, k := range keys {
		cs := classes[k]
		totalDiverge += cs.diverge
		if cs.diverge == 0 {
			continue
		}
		t.Errorf("DIVERGENT CLASS %s: %d/%d cases (worst dInt=%.2f dPaid=%.2f)\n    SIG=HARD:divergent_class %s",
			k, cs.diverge, cs.n, cs.worstInt, cs.worstPaid, cs.worstCmd)
	}
	t.Logf("divergent option classes: %d of %d compared cases", totalDiverge, checked)

	// ---- R7: coverage manifest over COMPARED cases, with an asserted envelope ----
	pys := make([]int, 0, len(covPerYr))
	for k := range covPerYr {
		pys = append(pys, k)
	}
	sort.Ints(pys)
	pyParts := make([]string, 0, len(pys))
	for _, k := range pys {
		pyParts = append(pyParts, fmt.Sprintf("%d:%d", k, covPerYr[k]))
	}
	mkeys := make([]string, 0, len(covMode))
	for k := range covMode {
		mkeys = append(mkeys, k)
	}
	sort.Strings(mkeys)
	mParts := make([]string, 0, len(mkeys))
	for _, k := range mkeys {
		mParts = append(mParts, fmt.Sprintf("%s:%d", k, covMode[k]))
	}
	t.Logf("COVERAGE (compared cases only): n=%d..%d  horizon=%d..%d yrs  perYr={%s}  modes={%s}",
		covMinN, covMaxN, covMinYrs, covMaxYrs, strings.Join(pyParts, " "),
		strings.Join(mParts, " "))
	t.Logf("COVERAGE (generated, pre-attrition): n=%d..%d  horizon=%d..%d yrs — "+
		"the compared envelope above shrinking toward the middle of this one is "+
		"attrition eating the extremes, which is where the defects have lived",
		genMinN, genMaxN, genMinYrs, genMaxYrs)

	// The envelope. These are DELIBERATELY loose — they are not a quality bar, they
	// are a tripwire for the generator silently narrowing, which is the failure
	// mode that made a 99.977% figure true and useless (§52). Only assert on runs
	// big enough for the draw to be representative, and only when no mode filter
	// is narrowing it on purpose.
	if covN >= 200 && len(fz5ModeFilter) == 0 {
		if covMaxYrs < 40 {
			t.Errorf("COVERAGE REGRESSION: longest schedule actually COMPARED is %d years. "+
				"The generator has narrowed — this is exactly the bound that made the "+
				"pre-2026-07-31 divergence rate (1 in 3,600) a true statement about a "+
				"region nobody had looked at; widening it measured 1 in 290. See "+
				"docs/harness_policy.md R7.", covMaxYrs)
		}
		if len(covPerYr) < 3 {
			t.Errorf("COVERAGE REGRESSION: only %d distinct payment frequencies COMPARED "+
				"(%s). Sub-monthly frequencies have hidden two harness bugs (§55, "+
				"firstPeriodDate); losing them silently is how they hid. See "+
				"docs/harness_policy.md R7.", len(covPerYr), strings.Join(pyParts, " "))
		}
		if len(covMode) < 4 {
			t.Errorf("COVERAGE REGRESSION: only %d of %d backward-solve modes reached the "+
				"comparison (%s). `noterm`+`non` carry 86%% of all findings; a run that "+
				"stops comparing them measures nothing. See docs/harness_policy.md R7.",
				len(covMode), fz5ModeCount, strings.Join(mParts, " "))
		}
	}
}

// covYears is the calendar span of a compared case, in years, computed the way
// the schedule actually runs (periods / frequency) rather than from any date the
// harness derives itself — R2, docs/harness_policy.md.
func covYears(l Loan) int {
	if l.PerYr <= 0 || l.NPeriods <= 0 {
		return 0
	}
	return l.NPeriods / int(l.PerYr)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// fz5AddMonths steps a date forward by whole months the way the ORACLE DRIVER
// does — including DOS's year BYTE (§55) and its month-end clamp.
//
// R2, docs/harness_policy.md: the harness must not carry a private date
// implementation. This is the single one, and `zzharnessdates_test.go` pins it
// against `amort_oracle intutil addn` (DOS's AddNPeriods) so it cannot drift.
//
// It is deliberately NOT dateutil.AddNPeriods: that takes a period count at a
// frequency, whereas the option tokens this generator emits (`mor=`, `b<N>=`,
// `adj=`, `pre=`) are all placed in WHOLE MONTHS from the loan date, which is
// how amort_oracle.pas places them. The two agree exactly where a period IS a
// whole number of months, and the test asserts that.
func fz5AddMonths(ld types.DateRec, months int) types.DateRec {
	nbal := (int(ld.Time.Month()) - 1) + months
	py := (ld.Time.Year() - 1900) + nbal/12
	py = ((py % 256) + 256) % 256
	y, m := py+1900, time.Month(nbal%12+1)
	d := ld.Time.Day()
	if last := dateutil.DaysInM(types.NewDateRec(y, m, 1)); d > last {
		d = last
	}
	return types.NewDateRec(y, m, d)
}

// fz5MaxYear is the PORT'S OWN resolved horizon for a case: the latest year
// appearing in the engine's answers. Deliberately not recomputed from the draw
// tokens — R2, "any date the harness computes must be computed the way the
// engine computes it", the rule that has returned six defects. gr.Schedule's
// last row, gr.Balloons and gr.LastDate are the engine's outputs, not a second
// derivation of them.
//
// ROUND 22 extracted this from the era-split block, where it had been inline
// since round 19, because the new date-horizon asymmetry check needs THE SAME
// definition. A second, slightly different horizon calculation a few hundred
// lines away is how a stratification label stops meaning what its name says —
// and the first version of that check DID use a different one (the schedule's
// last row alone), which labelled as IN SCOPE a screen carrying balloons at
// period 222 of a semi-annual series, i.e. the year 2137. A label is a coverage
// claim; two labels spelled the same way are two claims.
//
// ROUND 38 (audit F3): this is now a DELEGATE, not an implementation. The
// round-37 audit found that this function and cmd/goamort's `horizon` token
// were two hand-typed copies of the same three-way max, coupled only by a
// comment that claimed a pin no test performed. HorizonKeys (horizonkeys.go)
// is the single implementation; both consumers call it, and
// zzhorizonkeys_fixture_test.go pins the function itself against fixtures.
func fz5MaxYear(gr AmortResult) int {
	horizon, _, _ := HorizonKeys(gr)
	return horizon
}

// pctDelta is the percentage growth of the HARD numerator from the four-question
// instrument to the seven-question one. Returns 0 when the four-question column
// is empty, because "infinite percent worse" is not a useful thing to print and a
// zero denominator here means the sample was too small to say anything (R14).
func pctDelta(q4, q7 int) float64 {
	if q4 == 0 {
		return 0
	}
	return 100 * float64(q7-q4) / float64(q4)
}
