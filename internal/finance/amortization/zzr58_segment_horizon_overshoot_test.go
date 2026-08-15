package amortization

import (
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// ROUND 58 — §99. THE LONG ARM OF THE SEGMENT HORIZON CLAMP HAS NO DO-WHILE
// OVERSHOOT, AND IT COSTS THE LAST ROW OF THE IMPLIED-RATE SUB-WALK.
//
// THE PASCAL. RepayFancyLoan's main loop is a `repeat ... until`
// (AMORTOP.pas:1219-1221):
//
//	until (((not h^.lastok) or (Output<>nil)) and (WhenToStop^.principal = 0))
//	       or ((balance_calc) and (BalanceStop))
//	       or (DateComp(WhenToStop^.date, stopdate) >= 0) or (abort);
//
// The body runs BEFORE the test, so the first row AT OR PAST `stopdate` is
// EMITTED and the walk stops after it.
//
// 🚨 AND `stopdate` HERE IS `very_last`, NOT `h^.lastdate` — r58's first draft
// left the reader to supply `h^.lastdate` and its own audit refuted that.
// Iterate reaches the walk through
//
//	RepayFancyLoan(p, usap, loandate, firstdate, nil, false,
//	               til_adj, no_value_calc, 0)        {AMORTOP.pas:1439}
//
// whose LAST argument is `adjnum`, a literal 0 — `til_adj` is not adjnum, it is
// the boolean constant at AMORTOP.pas:21 bound to Iterate's `entire_or_no`
// parameter (:1415). With adjnum = 0 and Output = nil, :1139-1142 takes
// `stopdate := very_last` and :1130-1133 binds `WhenToStop := @NextPayment`.
// Neither is inside a Pascal comment. THIS IS WHY THE GUARD BELOW IS THE RIGHT
// ONE: inside it, very_last = h^.lastdate, so stopdate IS h^.lastdate. The port's `sub.Loan.LastDate` bound is
// inclusive-below, so a horizon that lands BETWEEN two segment rows costs a row.
// r53 (§92) gave the SHORT arm of the clamp that overshoot and deliberately left
// the LONG arm as round 25 wrote it, on the evidence that an unguarded version
// booked NEW = 1 on seed 50100 case 272.
//
// THAT CASUALTY IS REAL AND IT REPRODUCES — r58 measured it again: an unguarded
// overshoot on the long arm re-breaks 272 exactly, taking the standing arm to 6
// in-scope HARD instead of 5. But it is not evidence that the long arm needs no
// overshoot. It is evidence that the two arms are governed by DIFFERENT DOS
// BOUNDS, and `very_last` decides which (AMORTOP.pas:1297-1304):
//
//	if (nballoons > 0) and (DateComp(balloon[nballoons]^.date, h^.lastdate) > 0) then
//	  very_last := balloon[nballoons]^.date
//	else
//	  very_last := h^.lastdate;
//	for i := 1 to npre do
//	  if (DateComp(pre[i]^.stopdate, very_last) > 0) then
//	    very_last := pre[i]^.stopdate;
//
// So `very_last > h^.lastdate` means EXACTLY that a balloon or a prepayment
// series still pends past the last regular payment. In that state ComputeNext's
// extra branch is live and its own test fires FIRST (AMORTOP.pas:602-607):
//
//	balloonpos := 1;
//	if (xsource > 0) then
//	  begin
//	    balloonpos := DateComp(nextextra.date, date);
//	    if (DateComp(date, h^.lastdate) > 0) then
//	      balloonpos := -1;
//
// — the regular row past `h^.lastdate` is DROPPED and the pending extra taken
// instead. Round 25's clamp APPROXIMATES that, and it is why case 272 must keep
// it.
//
// ⚠️ "APPROXIMATES" IS THE HONEST WORD, and the guard OVER-REJECTS a residual
// class r58's audit found: if the only pendency past `h^.lastdate` is a BALLOON
// dated before the next regular grid row, DOS takes it with `balloonpos = -1`,
// which does NOT advance `base_date` (:623-624), so the next ComputeNext sees
// `xsource = 0` and emits the regular row anyway. SIZED HONESTLY, TWICE OVER
// (caution 2, then R32): r58's audit classified the guard's rejections over the
// two generators that reach this code — r53_segment_bound_sweep's 6,912 screens
// and 40 fuzzer seeds at N=400 — and found ZERO in that class. 🚨 THAT
// CLASSIFIER WAS NOT COMMITTED, and the two long-arm denominators it published
// disagreed with each other, so the exact counts are NOT reproducible from this
// tree and are deliberately not quoted here. Read it as a SHAPE — an UNDER-FIX
// with no instance found in the only two populations anyone has looked at, out
// of fourteen generators — not as a measurement. A committed classifier over
// `SegHorizonStats` is OWED TO r59; the counters are already in place. When `very_last = h^.lastdate` nothing pends beyond the horizon, `xsource`
// is exhausted, the `if (xsource > 0)` guard is FALSE, :606 is UNREACHABLE, and
// the only bound left is the until-clause — which overshoots by one.
//
// THE MEASUREMENT, not an argument. Fuzzer5 seed 50107 case 264, both engines
// pinned at the SHARED trial rate x = 0.0429960000 — r52 §2.2's instrument,
// `DPTRACESEGROWS`'s SEGROW lines against `scripts/build_trace_oracle.sh
// -mode cn`'s CN lines, row for row. The segment pays on Dec 24 against a loan
// on Nov 24, so the clamp lands BETWEEN two segment rows:
//
//	derived = 2048-12-24   h^.lastdate = 2048-11-24 = very_last
//
//	 row   DOS                              GO
//	 ...   (rows 0..130 BYTE-IDENTICAL, to the last digit)
//	 130   2047-12-24  pout= -14162.864973  bal= -14162.864973
//	 131   2048-12-24  pout=-23105.807982   ABSENT
//
// The port's terminal IS DOS's second-to-last balance. DOS's own Iterate trace
// prints `ITR0 seedx=0.0429960000 p=-23105.8079822528` — so restoring the row
// makes the port's residual match DOS's to ten digits, and the port's secant
// then tracks DOS's ITR trajectory step for step onto DOS's root (⚠️ the PRE-
// and POST-fix secants are NOT the same shape — 7 residual evaluations against
// 8; it is the POST-fix secant and DOS's that agree):
//
//	Go 0.0715039032  ->  0.0836119486  =  DOS bestx, exactly.
//
// STANDING ARM (seeds 50100-50109, PERSENSE_FUZZ_N=400, no engine filter, scope
// key `reached`): 11 -> 5 in-scope HARD cases, 1 in 190 -> 1 in 418, pooled
// 14 -> 8. `adj_rate_differs` 8 -> 1 signal instances. NO NEW CASE: the five
// that remain are a strict subset of the eleven. `paired_regression.sh` over the
// same seeds: FIXED 11, STILL BROKEN 26, NEW 0. R73's second generator,
// `r53_segment_bound_sweep.py`: 396 KEPT_CLOSER, 1,459 KEPT_UNMOVED, 0 LOST,
// 0 GAINED over 6,912 screens — a gate with DEMONSTRATED power over this change
// (contrast §98, for which the same sweep is a vacuous green). ⚠️ An
// independent 300-screen randomized corpus reached the extended branch on only
// 3 screens and moved no answer: different population, CAUTION 3, and neither
// figure is a product rate.
//
// 🚨 THE GUARD IS THE WHOLE FIX. Deleting it leaves a change that still clears
// six cases and BREAKS ONE (50100/272) — a mutant this file kills twice over
// (on the counter AND on the rate), see TestR58GuardIsLoadBearing.
//
// 🚨🚨 AND THE GUARD IS `== 0`, NOT `<= 0`. r58's first edition wrote
// `!DateOK(veryLast) || DateComp(veryLast, hLastDate) <= 0`, and the round's
// SECOND audit pass found the admitted set was WIDER THAN THE ARGUMENT — the
// project's round-56/57 signature, in the sentence pass 1 had just added.
// `!DateOK` is Pascal-wrong (AMORTOP.pas:1143-1147 sets a FAR-FUTURE sentinel,
// not h^.lastdate), and `veryLast < hLastDate` is REACHABLE: §53's month-end
// snap can push h^.lastdate past very_last, measured at fuzzer5 seed 51045 case
// 275 (very_last 2036-10-28 vs h^.lastdate 2036-10-31), 1 in 1,299 long-arm
// entries. Narrowing to `== 0` is NEUTRAL on the standing arm (5 in 2,091
// either way) — so it buys correctness of MEANING, not of number.
//
// ⚠️⚠️ AND THE TWO PINS BELOW CANNOT SEE THAT NARROWING. 🚨 r58's THIRD audit
// pass caught this paragraph claiming "both are `eq` screens" — FALSE, and
// refuted by this file's own assertions: pin 1 (case 264) is `eq` and ADMITTED
// (`stats=map[eq:1 eligible:1 extended:1 long:1]`); pin 2 (case 272) is `gt` and
// REJECTED (`stats=map[gt:1 eligible:0 long:1]`), which is exactly what
// TestR58GuardIsLoadBearing's `eligible != 0` fatal asserts — an `eq` pin 2
// would fail on the clean tree. Neither pin is `lt` or `notok`, and THAT is why
// mutants widening the guard back — M10 (`== 0` -> `<= 0`) and M11 (re-adding
// the `!DateOK` disjunct) — change neither verdict and SURVIVE this file. That is R84 in
// its literal form: the pins prove REACH of the eq arm and have NO POWER over
// the lt/notok arms. Closing it needs a THIRD pin over seed 51045 case 275,
// whose LoanInput could not be transcribed this round because the fuzzer's
// dump is taken AFTER Amortize and the engine writes back through the shared
// Adjustments slice (AmountStatus/LoanRateStatus come back as InOutOutput).
// OWED TO r59, WITH THE SCREEN ALREADY LOCATED. The `eq`/`lt`/`gt`/`notok`
// relation counters exist so that pin can assert the arm directly.
//
// ⚠️ UNIT: "5 in-scope HARD" counts CASES. The signal-instance count is a
// different unit (CAUTION 1) and so is the attribution count.

// r58Oracle runs the frozen DOS oracle and returns the ONE solved adjustment
// rate on the screen. Fails closed if there is not exactly one (R49/R52/R69):
// a vacuous assertion is worse than no assertion.
func r58Oracle(t *testing.T, args ...string) float64 {
	t.Helper()
	out, err := exec.Command(oracleBin, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("oracle failed on %v: %v\n%s", args, err, out)
	}
	s := string(out)
	var rate float64
	var have bool
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "adjrow ") {
			continue
		}
		f := strings.Fields(line)
		// Read ratestatus FIRST: `rate` precedes `ratestatus` on the line, and
		// this screen carries TWO user-typed rate adjustments, so a single
		// forward pass would latch a typed rate before testing the status
		// (r53 F10).
		var status string
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "ratestatus" {
				status = f[i+1]
			}
		}
		if status != "1" { // 1 == types.InOutOutput: DOS SOLVED this row
			continue
		}
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "rate" {
				if have {
					t.Fatalf("more than one SOLVED adjrow on %v — the pin would "+
						"be ambiguous (R52)", args)
				}
				rate, _ = strconv.ParseFloat(f[i+1], 64)
				have = true
			}
		}
	}
	if !have {
		t.Fatalf("oracle output carried NO solved adjrow rate on %v — the "+
			"assertion below would be VACUOUS (R49/R69):\n%s", args, s)
	}
	return rate
}

// r58ScreenArgs is the oracle command for fuzzer5 seed 50107 case 264, verbatim
// from its FZ5CASE line.
func r58ScreenArgs() []string {
	return []string{
		"163004.39", "0.0380620000", "25", "1",
		"b365", "exact", "plusreg",
		"loandmy=24.7.2023", "firstdmy=24.11.2024",
		"pre=112:258:24:81.22", "pre=88:65:4:536.74",
		"adj=28:0.0429960000:9583.73", "adj=184::8333.96",
		"adj=208:0.0688670000:11675.33",
		"pts=0.003986", "adjdump", "bdump",
	}
}

func r58Date(y int, m time.Month, d int) types.DateRec { return types.NewDateRec(y, m, d) }

// r58Screen is the port-side input for the same case, field-for-field as the
// fuzzer built it — captured by dumping the LoanInput at the fuzzer's own
// Amortize call, so no field is guessed (START_HERE §5: a missing struct field
// can read as an engine defect).
func r58Screen() LoanInput {
	return LoanInput{
		Fancy: true,
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 163004.39,
			LoanDateStatus: types.InOutInput, LoanDate: r58Date(2023, time.July, 24),
			LoanRateStatus: types.InOutInput, LoanRate: 0.038062,
			FirstStatus: types.InOutInput, FirstDate: r58Date(2024, time.November, 24),
			NStatus: types.InOutInput, NPeriods: 25,
			PerYrStatus:  types.InOutInput,
			PerYr:        1,
			PointsStatus: types.InOutInput, Points: 0.003986,
		},
		Adjustments: []RateAdjustment{
			{DateStatus: types.InOutInput, Date: r58Date(2025, time.November, 24),
				LoanRateStatus: types.InOutInput, LoanRate: 0.042996,
				AmountStatus: types.InOutInput, Amount: 9583.73, AmtOK: true},
			// THE AO6 ROW: an amount with no rate. DOS solves its implied rate
			// over the segment whose horizon this fix corrects.
			{DateStatus: types.InOutInput, Date: r58Date(2038, time.November, 24),
				AmountStatus: types.InOutInput, Amount: 8333.96, AmtOK: true},
			{DateStatus: types.InOutInput, Date: r58Date(2040, time.November, 24),
				LoanRateStatus: types.InOutInput, LoanRate: 0.068867,
				AmountStatus: types.InOutInput, Amount: 11675.33, AmtOK: true},
		},
		Prepayments: []Prepayment{
			{StartDateStatus: types.InOutInput, StartDate: r58Date(2032, time.November, 24),
				NNStatus: types.InOutInput, NN: 258,
				PerYrStatus: types.InOutInput, PerYr: 24,
				PaymentStatus: types.InOutInput, Payment: 81.22},
			{StartDateStatus: types.InOutInput, StartDate: r58Date(2030, time.November, 24),
				NNStatus: types.InOutInput, NN: 65,
				PerYrStatus: types.InOutInput, PerYr: 4,
				PaymentStatus: types.InOutInput, Payment: 536.74},
		},
		Settings: Settings{
			Basis: types.Basis365, PerYr: 1, PlusRegular: true, Exact: true,
			YrDays: 365.25, YrInv: 1.0 / 365.25,
		},
	}
}

// r58SolvedAdjRate returns the port's echoed rate for the AO6 adjustment row.
func r58SolvedAdjRate(t *testing.T, res AmortResult) float64 {
	t.Helper()
	var got float64
	var n int
	for _, a := range res.Adjustments {
		if a.RateSolved {
			got = a.Rate
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly ONE port-SOLVED adjustment row, got %d — the "+
			"comparison below would be picking an arbitrary row (R52)", n)
	}
	return got
}

// TestR58SegmentHorizonOvershootMatchesDOS is the round's headline pin. It is a
// DOS-ANCHORED DIFFERENTIAL: the expected value is read from the real oracle at
// run time, never hard-coded, so the test cannot drift away from DOS.
//
// PRE-FIX this test FAILS at 0.0715039032 against DOS's 0.0836119486, a delta of
// -1.21e-02 — seen to fail, rule 3.
func TestR58SegmentHorizonOvershootMatchesDOS(t *testing.T) {
	gateOracle(t)
	dosRate := r58Oracle(t, r58ScreenArgs()...)

	res := Amortize(r58Screen())
	if res.Err != nil {
		t.Fatalf("port refused a screen DOS answers: %v (R67 — a fix must not "+
			"turn an answer into a refusal)", res.Err)
	}

	// POSITIVE CONTROLS FIRST (R51/R76/R84). An assertion on the rate alone is
	// green on any tree where the long arm is never reached at all, so prove the
	// branch was ENTERED and that it MOVED THE BOUND before believing the value.
	// A nil stats map reads as zero here, so this fails closed.
	st := res.SegHorizonStats()
	if st["long"] < 1 {
		t.Fatalf("§99's LONG arm of the horizon clamp was never reached on this "+
			"screen (long=%d) — the rate assertion below would have NO POWER "+
			"(R84: reach is not power, and no reach is not even reach). "+
			"stats=%v", st["long"], st)
	}
	if st["eligible"] < 1 {
		t.Fatalf("the LONG arm fired but the very_last guard REJECTED this "+
			"screen (long=%d eligible=%d) — this screen is supposed to have "+
			"very_last == h^.lastdate (AMORTOP.pas:1297-1304). stats=%v",
			st["long"], st["eligible"], st)
	}
	if st["extended"] < 1 {
		t.Fatalf("the guard admitted the screen but the overshoot never MOVED "+
			"the bound (eligible=%d extended=%d) — §99 is inert here and the "+
			"rate below is matching DOS for some other reason. stats=%v",
			st["eligible"], st["extended"], st)
	}

	got := r58SolvedAdjRate(t, res)
	// tolAdjRate is the fuzzer's own pinned adjustment-rate tolerance; use it so
	// this pin and the arm cannot disagree about what "matches" means.
	if math.Abs(got-dosRate) > tolAdjRate {
		t.Fatalf("§99: solved AO6 adjustment rate differs from DOS\n"+
			"  DOS %.10f | Go %.10f (delta=%+.2e, tol=%.1e)\n"+
			"  stats=%v\n"+
			"  This is the LONG-arm horizon overshoot (fancybisect.go, §99). The "+
			"port's sub-walk terminal is DOS's SECOND-TO-LAST balance.",
			dosRate, got, got-dosRate, tolAdjRate, st)
	}
}

// TestR58GuardIsLoadBearing pins the OTHER half of §99: the `very_last <=
// h^.lastdate` guard. Seed 50100 case 272 is the screen r52 broke by applying
// this overshoot unguarded, and it is the ONLY reason round 25's clamp survives
// on the long arm. It carries `pre=315:74:12:353.27`, a prepayment series whose
// stopdate is FOURTEEN YEARS past h^.lastdate, so very_last > h^.lastdate,
// AMORTOP.pas:606 is live, and DOS DROPS the regular row rather than emitting
// it.
//
// The assertion is the same DOS-anchored differential. The positive control is
// the INVERSE of the headline test's: the long arm must be reached and the guard
// must REJECT. Deleting the guard makes `eligible` rise to match `long` and the
// rate go wrong — both observable, so this test kills that mutant twice over.
func TestR58GuardIsLoadBearing(t *testing.T) {
	gateOracle(t)
	args := []string{
		"217484.39", "0.0640200000", "54", "3",
		"b365_360", "prepaid", "plusreg", "r78", "usa",
		"loandmy=31.5.2024", "firstdmy=31.8.2024",
		"b43=46698.48", "b163=21741.52",
		"pre=95:38:12:153.25", "pre=315:74:12:353.27",
		"adj=107:0.0810110000:5483.16", "adj=119::9707.32",
		"adj=123:0.0422660000:",
		"targ=629.38", "pts=0.031316", "adjdump", "bdump",
	}
	dosRate := r58Oracle(t, args...)

	res := Amortize(r58Case272Screen())
	if res.Err != nil {
		t.Fatalf("port refused a screen DOS answers: %v (R67)", res.Err)
	}
	st := res.SegHorizonStats()
	if st["long"] < 1 {
		t.Fatalf("§99's LONG arm was never reached on case 272 (stats=%v) — "+
			"this test would have no power over the guard (R84)", st)
	}
	if st["eligible"] != 0 {
		t.Fatalf("the very_last guard ADMITTED case 272 (long=%d eligible=%d) — "+
			"it must not: this screen's prepayment series pends 14 years past "+
			"h^.lastdate, so AMORTOP.pas:606 drops the regular row and the "+
			"overshoot is wrong here. stats=%v", st["long"], st["eligible"], st)
	}
	got := r58SolvedAdjRate(t, res)
	if math.Abs(got-dosRate) > tolAdjRate {
		t.Fatalf("§99 guard: case 272's solved AO6 rate differs from DOS\n"+
			"  DOS %.10f | Go %.10f (delta=%+.2e, tol=%.1e)\n  stats=%v",
			dosRate, got, got-dosRate, tolAdjRate, st)
	}
}

// r58Case272Screen is seed 50100 case 272, field-for-field from the fuzzer's own
// LoanInput dump.
func r58Case272Screen() LoanInput {
	return LoanInput{
		Fancy: true,
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 217484.39,
			LoanDateStatus: types.InOutInput, LoanDate: r58Date(2024, time.May, 31),
			LoanRateStatus: types.InOutInput, LoanRate: 0.06402,
			FirstStatus: types.InOutInput, FirstDate: r58Date(2024, time.August, 31),
			NStatus: types.InOutInput, NPeriods: 54,
			PerYrStatus:  types.InOutInput,
			PerYr:        3,
			PointsStatus: types.InOutInput, Points: 0.031316,
		},
		Balloons: []BalloonPayment{
			{DateStatus: types.InOutInput, Date: r58Date(2027, time.December, 31),
				AmountStatus: types.InOutInput, Amount: 46698.48},
			{DateStatus: types.InOutInput, Date: r58Date(2037, time.December, 31),
				AmountStatus: types.InOutInput, Amount: 21741.52},
		},
		Adjustments: []RateAdjustment{
			{DateStatus: types.InOutInput, Date: r58Date(2033, time.April, 30),
				LoanRateStatus: types.InOutInput, LoanRate: 0.081011,
				AmountStatus: types.InOutInput, Amount: 5483.16, AmtOK: true},
			{DateStatus: types.InOutInput, Date: r58Date(2034, time.April, 30),
				AmountStatus: types.InOutInput, Amount: 9707.32, AmtOK: true},
			{DateStatus: types.InOutInput, Date: r58Date(2034, time.August, 31),
				LoanRateStatus: types.InOutInput, LoanRate: 0.042266},
		},
		Prepayments: []Prepayment{
			{StartDateStatus: types.InOutInput, StartDate: r58Date(2032, time.April, 30),
				NNStatus: types.InOutInput, NN: 38,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: 153.25},
			{StartDateStatus: types.InOutInput, StartDate: r58Date(2050, time.August, 31),
				NNStatus: types.InOutInput, NN: 74,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: 353.27},
		},
		Target: Target{TargetStatus: types.InOutInput, TargetValue: 629.38},
		Settings: Settings{
			Basis: types.Basis365360, PerYr: 3, Prepaid: true, PlusRegular: true,
			R78: true, USARule: true, YrDays: 360, YrInv: 1.0 / 360,
		},
	}
}
