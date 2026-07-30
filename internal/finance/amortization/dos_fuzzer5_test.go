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
)

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
}

type fz5BalloonRow struct {
	idx     int
	date    string // "M/D/YYYY"
	dstatus int
	amount  float64
	astatus int
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
	addMonthsFrom := func(ld types.DateRec, months int) types.DateRec {
		nbal := (int(ld.Time.Month()) - 1) + months
		y, m := ld.Time.Year()+nbal/12, time.Month(nbal%12+1)
		d := ld.Time.Day()
		if last := dateutil.DaysInM(types.NewDateRec(y, m, 1)); d > last {
			d = last
		}
		return types.NewDateRec(y, m, d)
	}
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
	var runDump func(args []string) (fz5Dump, int)
	runDump = func(args []string) (fz5Dump, int) {
		for try := 0; try < 8; try++ {
			out, err := exec.Command(oracleBin, args...).Output()
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
					return fz5Dump{}, fz5DateHorizon
				}
				if strings.Contains(msg, "did not converge") {
					return fz5Dump{}, fz5NonConverge
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
				return fz5Dump{}, fz5Refused
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
			// Heap-flake sentinels: a real schedule never has non-positive paid,
			// and NumAfter returns -1 when the totals line is malformed.
			if d.paid <= 0 || d.interest == -1 || d.payment == 0 {
				continue
			}
			return d, fz5Solved
		}
		return fz5Dump{}, fz5Flake
	}

	perYrs := []int{1, 2, 4, 12}
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
	nonConv, nonConvGoRetires, nonConvGoSpurious := 0, 0, 0
	goRefusedDosSolved, goSolvedDosRefused := 0, 0
	tackAgree, tackGoOnly, tackDosOnly, tackValueDiff := 0, 0, 0, 0
	solveChecked, solveDiff := 0, 0
	blockCover := map[string]int{}

	type classStat struct {
		n         int
		diverge   int
		worstInt  float64
		worstPaid float64
		worstCmd  string
	}
	classes := map[string]*classStat{}

	for c := 0; c < N; c++ {
		perYr := perYrs[rng.Intn(len(perYrs))]
		mPer := 12 / perYr
		years := 8 + rng.Intn(18) // 8..25y — long enough to seat 3 balloons + 3 ARMs
		n := years * perYr
		amount := cents(25000 + rng.Float64()*475000)
		rate := q6(0.03 + rng.Float64()*0.11)

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
		if bf, ok := basisFlag(basis); ok {
			flags = append(flags, bf)
		}
		if exact {
			flags = append(flags, "exact")
		}
		if prepaid {
			flags = append(flags, "prepaid")
		}
		if inadv {
			flags = append(flags, "inadv")
		}
		if plusreg {
			flags = append(flags, "plusreg")
		}
		if r78 {
			flags = append(flags, "r78")
		}
		if usa {
			flags = append(flags, "usa")
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
		ldY, ldM := 2023+rng.Intn(3), time.Month(1+rng.Intn(12))
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
			budget := amount * 0.60
			for _, m := range months {
				if budget <= 0 {
					break
				}
				amt := cents(math.Min(budget, amount*(0.02+rng.Float64()*0.28)))
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
				startK := 1 + rng.Intn(maxInt(1, n*7/10))
				m, ok := pickMonth(startK, startK)
				if !ok {
					continue
				}
				// nn extra payments at ppy/yr. Keep the series inside the term so
				// CheckPrepayments' derived stop date does not run off the horizon.
				ppy := []int{12, 24, 26, 52}[rng.Intn(4)]
				// Months from the START of the prepayment series to the LAST
				// scheduled payment, which sits at loandate + firstMonths +
				// (n-1)*mPer — not at n*mPer once the first period is odd.
				remMonths := ((n-1)*mPer + firstMonths) - m
				maxNN := (remMonths * ppy) / 12
				if maxNN < 1 {
					maxNN = 1
				}
				if maxNN > 400 {
					maxNN = 400
				}
				nn := 1 + rng.Intn(maxNN)
				// Size the series as a FRACTION OF THE REGULAR PAYMENT FLOW: a
				// prepayment arrives ppy times a year against perYr regular
				// payments, so perYr/ppy converts "fraction of one regular
				// payment's worth of extra money per year" into a per-prepayment
				// amount. 3%-25% of the flow keeps the loan alive to term.
				amt := cents(fair * (0.03 + rng.Float64()*0.22) * float64(perYr) / float64(ppy))
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
						nr := q6(0.02 + rng.Float64()*0.13)
						a.LoanRateStatus, a.LoanRate = types.InOutInput, nr
						rateStr = strconv.FormatFloat(nr, 'f', 10, 64)
					}
					if kind != 0 {
						// A payment adjustment is a NEW regular payment, so it has to
						// live near the fair payment. amount/n (straight-line
						// principal, no interest) is far below it on a long loan and
						// starves the schedule into a non-converge.
						na := cents(fair * (0.75 + rng.Float64()*0.7))
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
			tv := cents(fair * (0.02 + rng.Float64()*0.23))
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
			points = q6(rng.Float64() * 0.04)
			flags = append(flags, fmt.Sprintf("pts=%.6f", points))
			note("pts")
		}

		if !anyOpt {
			continue // the 15% coins came up empty for every block — plain loan
		}

		// ---- Payment mode ----
		// The fair payment scaled 0.85x-1.35x, so residuals of BOTH signs occur
		// and TackOnFinalBalloon's over- and under-funded arms both get hit —
		// but the schedule still terminates. Below ~0.8x a stacked-option loan
		// negatively amortizes past DOS's horizon; above ~1.4x it retires far
		// early and DOS refuses rather than truncating.
		pay := cents(fair * (0.85 + rng.Float64()*0.5))
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
			lastDate = addMonthsFrom(firstDate, (n-1)*mPer)
			flags = append(flags, "non", fmt.Sprintf("lastdmy=%d.%d.%d",
				lastDate.Time.Day(), int(lastDate.Time.Month()), lastDate.Time.Year()))
		case fz5ModeAmount:
			flags = append(flags, "noamt")
		case fz5ModeRate:
			flags = append(flags, "norate")
		}
		flags = append(flags, "bdump")

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
		case fz5ModeAmount:
			v, _, err := SolveLoanAmount(in)
			if goSolveErr = err; err == nil {
				goSolved, goSolvedOK = v, true
				in.Loan.AmountStatus, in.Loan.Amount = types.InOutInput, v
				in.AmountWasSolved = true
			}
		case fz5ModeRate:
			v, _, err := SolveRate(in)
			if goSolveErr = err; err == nil {
				goSolved, goSolvedOK = v, true
				in.Loan.LoanRateStatus, in.Loan.LoanRate = types.InOutInput, v
				in.RateWasSolved = true
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
			gr = Amortize(in)
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

		switch outcome {
		case fz5Flake:
			flaked++
			continue
		case fz5DateHorizon:
			horizon++
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
					t.Logf("DOS non-converge, Go returned a NON-RETIRING schedule [%s]\n  %s\n"+
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
				t.Errorf("Go produced a schedule where DOS REFUSED the screen [%s]\n  %s\n"+
					"  Go: int=%.2f paid=%.2f rows=%d", sig, cmd, gr.TotalInt, gr.TotalPaid,
					len(gr.Schedule))
			}
			continue
		}

		checked++
		cs := classes[sig]
		if cs == nil {
			cs = &classStat{}
			classes[sig] = cs
		}
		cs.n++

		if !goOK {
			goRefusedDosSolved++
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
			t.Logf("DOS solved, Go refused [%s]: %v\n  %s", sig, errText, cmd)
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
		if dInt > intTol || dPaid > paidTol {
			cs.diverge++
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
			// The tack amount is a balance, so scale the tolerance to the loan.
			tackTol := math.Max(0.05, 1e-5*math.Abs(amount))
			if math.Abs(goTack.Amount-dosTack.amount) > tackTol || wantDate != dosTack.date {
				tackValueDiff++
				t.Errorf("terminating balloon differs [%s]\n  %s\n"+
					"  DOS: %s %.4f (row %d, dstatus/astatus outp, nballoons=%d nlines=%d)\n"+
					"  Go : %s %.4f", sig, cmd, dosTack.date, dosTack.amount, dosTack.idx,
					dos.nballoons, dos.nlines, wantDate, goTack.Amount)
			}
		case dosHasTack && !goHasTack:
			tackDosOnly++
			t.Errorf("DOS tacked a terminating balloon the port did not [%s]\n  %s\n"+
				"  DOS row %d: %s %.4f (dstatus %d astatus %d), nballoons=%d nlines=%d\n"+
				"  This is the §46 gate: Amortize.pas:1043 requires fancy AND "+
				"PayAmtStatus >= defp AND an over-specified schedule.",
				sig, cmd, dosTack.idx, dosTack.date, dosTack.amount,
				dosTack.dstatus, dosTack.astatus, dos.nballoons, dos.nlines)
		case !dosHasTack && goHasTack:
			tackGoOnly++
			t.Errorf("the port tacked a terminating balloon DOS did not [%s]\n  %s\n"+
				"  Go: %s %.4f — DOS's grid has no outp/outp row",
				sig, cmd, goTack.Date.Time.Format("1/2/2006"), goTack.Amount)
		}

		// ---- Signal 3: the post-FirstPass horizon ----
		// Soft everywhere EXCEPT the two term-solving modes, where the horizon
		// IS the answer and signal 4 below promotes it to a hard failure.
		if mode != fz5ModeTerm && mode != fz5ModeN &&
			dos.nPeriods > 0 && gr.NPeriods > 0 && dos.nPeriods != gr.NPeriods {
			t.Logf("nperiods differ [%s]: DOS %d, Go %d\n  %s", sig, dos.nPeriods, gr.NPeriods, cmd)
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
				if tol := 0.01 + 2e-6*math.Abs(dos.solvedAmt); math.Abs(goSolved-dos.solvedAmt) > tol {
					solveDiff++
					t.Errorf("solved AMOUNT differs [%s]\n  %s\n  DOS %.6f | Go %.6f (delta=%+.6f)",
						sig, cmd, dos.solvedAmt, goSolved, goSolved-dos.solvedAmt)
				}
			}
		case fz5ModeRate:
			if dos.hasSolvedRate && goSolvedOK {
				solveChecked++
				// 5e-6 absolute (~6e-5 relative at ordinary rates). Tighter than
				// this is below the stop tolerance of DOS's own refinement, so it
				// would report the solver's last step rather than a divergence.
				if math.Abs(goSolved-dos.solvedRate) > 5e-6 {
					solveDiff++
					t.Errorf("solved RATE differs [%s]\n  %s\n  DOS %.10f | Go %.10f (delta=%+.2e)",
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
					t.Errorf("solved TERM differs [%s]\n  %s\n  DOS n=%d last=%s | Go n=%d last=%s",
						sig, cmd, dos.nPeriods, dos.lastDate, gr.NPeriods, goLast)
				}
			}
		}
	}

	// ---- Report ----
	t.Logf("cases: %d generated, %d compared | DOS refused %d, non-converged %d, date-horizon %d, flaked %d",
		N, checked, refused, nonConv, horizon, flaked)
	t.Logf("dispatch: Go-solved-DOS-refused %d (hard fail), DOS-solved-Go-refused %d (logged)",
		goSolvedDosRefused, goRefusedDosSolved)
	t.Logf("DOS non-converge: Go retired the loan %d (benign solver gap), Go non-retiring %d (suspect)",
		nonConvGoRetires, nonConvGoSpurious)

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
		t.Errorf("DIVERGENT CLASS %s: %d/%d cases (worst dInt=%.2f dPaid=%.2f)\n    %s",
			k, cs.diverge, cs.n, cs.worstInt, cs.worstPaid, cs.worstCmd)
	}
	t.Logf("divergent option classes: %d of %d compared cases", totalDiverge, checked)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
