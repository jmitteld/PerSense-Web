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

// ROUND 57 — DOS SOLVES AN IMPLIED ADJUSTMENT RATE ONCE PER SCREEN. THE PORT
// SOLVED IT ON EVERY WALK AND REPORTED THE LAST ROOT.
//
// AMORTOP.pas:1515-1541, Re_Amortize's amount-given branch:
//
//	if (adj[next_adj]^.amtok) then
//	  begin
//	    d := adj[next_adj]^.amount;
//	    if (adj[next_adj]^.loanratestatus < outp) then
//	      {If it's already outp, we don't want to re-compute it.
//	       This saves time and it's essential for APR value calculation. }
//	      begin
//	        if Iterate(p, usap, payment.date, nextpayment.date,
//	                   h^.loanrate, til_adj) then
//	          begin
//	            adj[next_adj]^.loanrate := h^.loanrate;
//	            adj[next_adj]^.loanratestatus := outp;      <- THE LATCH
//	            f := GrowthPerPeriod;
//	          end
//	        else errorflag := true;
//	        p := h^.amount; usap := 0;
//	      end
//	    else
//	      if (adj[next_adj]^.loanratestatus = outp) then
//	        begin                                           <- THE REUSE
//	          h^.loanrate := adj[next_adj]^.loanrate;
//	          ComputeTrueRate; f := GrowthPerPeriod;
//	        end;
//	  end
//
// The port called solveSegmentRate unconditionally at engine.go's AO6 branch.
// On the screen below that is FOUR solves — the adjustment pre-pass, the display
// walk, and both again inside aprValueCashflows — and those walks do not carry
// the same running balance into the boundary. Traced with DPTRACESEGROWS and a
// call-site probe on fuzzer5 seed 50106 case 318:
//
//	PREPASS bal=10977.7062146021  -> rate 0.0436637379   (DOS's value)
//	REAL    bal=10977.8300000000  -> rate 0.0436587258   (what the port reported)
//	PREPASS bal=10977.8300000000  -> rate 0.0436587258
//	APR     bal=10977.8300000000  -> rate 0.0436587258
//
// The row count was 28 at every trial rate in both walks, so this is NOT a
// period-count difference (§66's horizon clamp, closed at r53); the terminal
// balance itself differs at the SAME trial rate, and DOS keeps the first root.
//
// ⚠️ THE HONEST SCOPE OF THIS FIX, MEASURED, NOT ARGUED — AND CORRECTED IN-ROUND
// BY THIS ROUND'S OWN ADVERSARIAL AUDIT, WHICH REFUTED THREE OF THE FIRST DRAFT'S
// SUPPORTING CLAIMS (rule 12, TWENTY-EIGHTH round).
//
// On the standing arm (seeds 50100-50109, PERSENSE_FUZZ_N=400, key `reached`) it
// flips exactly ONE verdict in 4,000 generated cases — this one — taking the
// in-scope HARD count 12 to 11 and 1 in 174 to 1 in 190, and `adj_rate_differs`
// 9 signal instances to 8. Every per-class count is otherwise identical and
// ACTUALLY COMPARED is unchanged at 2,209.
//
// 🚨 "CHANGES NOTHING ELSE" WOULD BE FALSE, AND WAS WRITTEN HERE FIRST. A count
// identity is not a set identity (R81). ⚠️ AND THE FIRST CORRECTION CLAIMED A
// COMPLETENESS IT DID NOT HAVE — the round's SECOND audit pass found it had
// enumerated three bullets and omitted a whole instrument's worth of movement.
// R70 in its literal form: the correction, not the original, is where the false
// claim sat. What follows is a SHAPE plus counts, not an enumeration, and it
// should be read as such.
//
// VERDICTS: exactly ONE `FZ5VERDICT` line changes across the 4,000 generated
// cases — case 318, `hard=true` -> `hard=false`. `paired_regression.sh` over the
// same seeds: FIXED 1, STILL BROKEN 37, NEW 0.
//
// VALUES, on cases that were already divergent and stay divergent:
//   - seed 50100 divergent class: Go int 357506.90 -> 357506.96 (dInt 16901.37
//     -> 16901.31, BETTER by $0.06)
//   - seed 50103 divergent class: Go int 386609.99 -> 386610.01 (dInt 2165.01
//     -> 2165.03, WORSE by $0.02)
//   - two APR values move at 1e-8 (50100, 50103)
//
// TOLERANCE-MARGIN INSTRUMENT: 13 `tolerance [...]` lines move across SEVEN
// seeds (50101, 50102, 50103, 50105, 50106, 50107, 50109; 50100/50104/50108 are
// unchanged, which is what makes this deterministic rather than noise). NO
// judged/pass count moves on any of them. The only directionally adverse entry
// is seed 50102's apr "passing within one decade of tol" 115 -> 116 — one more
// case now sits within a decade of its tolerance.
//
// Reproduce the whole picture rather than trusting this list:
//	diff <(grep 'tolerance \[' PRISTINE_ARM/seed_N.log) \
//	     <(grep 'tolerance \[' R57_ARM/seed_N.log)
//
// 🚨 "THE OTHER EIGHT SOLVE TO THE SAME ROOT ON EVERY WALK" WAS ALSO FALSE. TWO
// of the eight surviving `adj_rate_differs` rows DO move when the first root is
// latched — 6/30/2040 by 4.6e-8 and 9/21/2030 by 1.2e-7 — but their gaps to DOS
// are 1.68e-3 and 9.85e-3. The correct statement is that the walk-to-walk root
// SPREAD on those rows is four to five orders of magnitude too small to close
// them, so whatever owns them is not this mechanism.
//
// 🚨 AND THE FIRST SAFETY SWEEP HAD ZERO POWER. A sweep of 4,374 "simple"
// single-adjustment screens reported that the latch changes no output anywhere.
// It could not have: the reuse branch is entered only when the screen is walked
// twice, and the second walk is the APR value walk, which runs only when the
// screen carries POINTS. Every screen in that sweep had none. Measured:
// 0 of 471 latched screens reused without points, 471 of 471 with. R69 — a null
// result can be true by construction; publish the ELIGIBLE COUNT.
// It is replaced by `testplan/harness/r57_latch_reach_sweep.py`, which
// stratifies on points and carries the reach control: 495 of 495 points-carrying
// screens entered the reuse branch, 0 of 495 without, 681 eligible per stratum,
// 0 answers lost, 0 gained, 0 moved by more than half a cent.
// ⚠️ NOTE FOR ANY FUTURE ROUND: `r53_segment_bound_sweep.py`, the project's
// standing R73 instrument for the segment solver family, emits NO `pts=` token
// at all, so its 1,855 KEPT_UNMOVED screens are 1,855 screens this fix cannot
// bite. Its PASS is real and is about a different hypothesis (R76).
//
// This fix is narrow and is recorded as narrow.
//
// 🚨 WHAT THIS FILE PINS, AND WHY IT IS A DIFFERENTIAL. The expected adjustment
// rate is taken from the REAL DOS oracle at run time, exactly as
// zzr53_segment_horizon_test.go does. A hardcoded literal would be a change
// detector, not a correctness pin (START_HERE §5), and would not notice the
// oracle moving.
//
// BOTH DIRECTIONS, IN FACT (rule 3). Verified against a pristine tree built from
// HEAD 2d63a19: TestR57AdjRateLatchMatchesDOS FAILS there (port 0.0436587258
// against DOS 0.0436637379) and passes here.

// r57Oracle runs the real DOS engine and returns the ONE solved adjustment rate.
// R61/R64: a timeout and an empty run are their own failures, never folded into
// "DOS declined". R52: the emitter's cardinality is checked — more than one
// SOLVED row would make the pin ambiguous.
func r57Oracle(t *testing.T, args ...string) float64 {
	t.Helper()
	out, err := exec.Command(oracleBin, append(args, "adjdump")...).Output()
	if err != nil {
		t.Fatalf("oracle failed on %v: %v", args, err)
	}
	s := string(out)
	if strings.TrimSpace(s) == "" {
		t.Fatalf("oracle produced NO OUTPUT on %v — that is not a zero (R64)", args)
	}
	var rate float64
	var have bool
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "adjrow ") {
			continue
		}
		f := strings.Fields(line)
		// READ ratestatus FIRST — `rate` always precedes `ratestatus` on the
		// line, so a single forward pass would latch a USER-TYPED rate before it
		// could ever test the status. This screen carries two typed-rate
		// adjustments, so that bug would silently pin the wrong row (r53 F10).
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

// r57ScreenArgs is the oracle command for fuzzer5 seed 50106 case 318, verbatim
// from its FZ5CASE line.
func r57ScreenArgs() []string {
	return []string{
		"27055.77", "0.0306000000", "108", "6",
		"b365_360", "plusreg", "usa",
		"loandmy=29.5.2023", "firstdmy=29.7.2023",
		"mor=70", "b82=5662.57", "b88=741.10",
		"pre=134:5:1:465.11", "pre=64:13:1:268.70",
		"adj=40:0.1007040000:", "adj=44:0.0941730000:", "adj=160::361.78",
		"pts=0.000833", "payhard=381.37", "norate",
	}
}

func r57Date(y int, m time.Month, d int) types.DateRec { return types.NewDateRec(y, m, d) }

// r57Screen is the port-side input for the same case, field-for-field as the
// fuzzer built it (captured by dumping the LoanInput at the fuzzer's Amortize
// call, so no field is guessed — START_HERE §5: a missing struct field can read
// as an engine defect).
//
// ⚠️ LoanRate here is the rate the fuzzer's `norate` arm put on the screen, at
// the precision it carries; DOS prints the same value as `solvedrate
// 0.0819564764`. The ASSERTION is anchored on DOS's ADJUSTMENT rate read from
// the oracle at run time, not on this number.
func r57Screen() LoanInput {
	return LoanInput{
		Fancy: true,
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 27055.77,
			LoanDateStatus: types.InOutInput, LoanDate: r57Date(2023, time.May, 29),
			LoanRateStatus: types.InOutInput, LoanRate: 0.08195647638467107,
			FirstStatus: types.InOutInput, FirstDate: r57Date(2023, time.July, 29),
			NStatus: types.InOutInput, NPeriods: 108,
			PerYrStatus: types.InOutInput, PerYr: 6,
			PayAmtStatus: types.InOutInput, PayAmt: 381.37,
			PointsStatus: types.InOutInput, Points: 0.000833,
		},
		Balloons: []BalloonPayment{
			{DateStatus: types.InOutInput, Date: r57Date(2030, time.March, 29),
				AmountStatus: types.InOutInput, Amount: 5662.57},
			{DateStatus: types.InOutInput, Date: r57Date(2030, time.September, 29),
				AmountStatus: types.InOutInput, Amount: 741.10},
		},
		Adjustments: []RateAdjustment{
			{DateStatus: types.InOutInput, Date: r57Date(2026, time.September, 29),
				LoanRateStatus: types.InOutInput, LoanRate: 0.100704},
			{DateStatus: types.InOutInput, Date: r57Date(2027, time.January, 29),
				LoanRateStatus: types.InOutInput, LoanRate: 0.094173},
			// THE AO6 ROW: an amount with no rate. This is the one DOS latches.
			{DateStatus: types.InOutInput, Date: r57Date(2036, time.September, 29),
				AmountStatus: types.InOutInput, Amount: 361.78, AmtOK: true},
		},
		Prepayments: []Prepayment{
			{StartDateStatus: types.InOutInput, StartDate: r57Date(2034, time.July, 29),
				NNStatus: types.InOutInput, NN: 5,
				StopDateStatus: types.InOutInput, StopDate: r57Date(2038, time.July, 29),
				PerYrStatus: types.InOutInput, PerYr: 1,
				PaymentStatus: types.InOutInput, Payment: 465.11},
			{StartDateStatus: types.InOutInput, StartDate: r57Date(2028, time.September, 29),
				NNStatus: types.InOutInput, NN: 13,
				StopDateStatus: types.InOutInput, StopDate: r57Date(2040, time.September, 29),
				PerYrStatus: types.InOutInput, PerYr: 1,
				PaymentStatus: types.InOutInput, Payment: 268.70},
		},
		Moratorium: Moratorium{
			FirstRepayStatus: types.InOutInput, FirstRepay: r57Date(2029, time.March, 29),
		},
		Settings: Settings{
			Basis: types.Basis365360, PerYr: 6, PlusRegular: true, USARule: true,
			YrDays: 360, YrInv: 1.0 / 360,
		},
		RateWasSolved: true,
	}
}

// r57SolvedAdjRate returns the port's echoed rate for the AO6 adjustment row.
func r57SolvedAdjRate(t *testing.T, res AmortResult) float64 {
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

// TestR57AdjRateLatchMatchesDOS is the round's headline pin.
func TestR57AdjRateLatchMatchesDOS(t *testing.T) {
	gateOracle(t)
	dosRate := r57Oracle(t, r57ScreenArgs()...)

	res := Amortize(r57Screen())
	if res.Err != nil {
		t.Fatalf("port refused a screen DOS answers: %v (R67 — a fix must not "+
			"turn an answer into a refusal)", res.Err)
	}
	got := r57SolvedAdjRate(t, res)

	// (a) the signal's OWN tolerance — this is the verdict the standing arm makes.
	if d := math.Abs(got - dosRate); d > tolAdjRate {
		t.Fatalf("adjustment solved RATE differs: DOS %.10f | Go %.10f (delta=%.2e) "+
			"> tolAdjRate %.0e", dosRate, got, got-dosRate, tolAdjRate)
	}
	// (b) a TIGHTER pin, because (a) alone is fragile here: the pristine tree
	// misses by 5.01e-06 against a 5e-06 tolerance — 0.2% of the tolerance — so a
	// tolAdjRate-only assertion would be one rounding change away from passing on
	// a broken tree. The latched root reproduces DOS to every digit the oracle
	// prints, so 1e-7 is the honest pin and it is not near any boundary.
	if d := math.Abs(got - dosRate); d > 1e-7 {
		t.Fatalf("adjustment solved rate is inside tolAdjRate but NOT on DOS's "+
			"root: DOS %.10f | Go %.10f (delta=%.2e). The latch is meant to make "+
			"the port adopt DOS's FIRST solve exactly.", dosRate, got, got-dosRate)
	}
}

// TestR57AdjRateLatchWasReachedAndReused is the POSITIVE CONTROL (R51/R76).
// TestR57AdjRateLatchMatchesDOS above is branch-guarded: it would pass on a tree
// where the latch is never written AND the port happened to agree with DOS for
// some other reason. This asserts the branch was ENTERED — one adjustment
// latched, solved exactly once, and the REUSE arm taken at least once.
func TestR57AdjRateLatchWasReachedAndReused(t *testing.T) {
	res := Amortize(r57Screen())
	if res.Err != nil {
		t.Fatalf("port refused: %v", res.Err)
	}
	latch := res.AdjRateLatch()
	if len(latch) != 1 {
		t.Fatalf("expected exactly ONE latched adjustment on this screen "+
			"(one AO6 row, two typed-rate rows), got %d: %+v", len(latch), latch)
	}
	// index 2 is the AO6 row in r57Screen's Adjustments — the KEY is DOS's
	// next_adj, so a mutant keying on anything constant would collide.
	ent, ok := latch[2]
	if !ok {
		t.Fatalf("the latched adjustment is not index 2 (the AO6 row): %+v", latch)
	}
	if ent.Solves != 1 {
		t.Fatalf("the AO6 rate was solved %d times, want exactly 1 — DOS solves it "+
			"once per screen (AMORTOP.pas:1517)", ent.Solves)
	}
	if ent.Reuses < 1 {
		t.Fatalf("the REUSE branch was NEVER TAKEN (Reuses=%d), so "+
			"TestR57AdjRateLatchMatchesDOS is a vacuous green: this screen is "+
			"walked more than once and the latch must have been read (R51/R76)",
			ent.Reuses)
	}
}

// TestR57AdjRateLatchIsPerAdjustment kills the constant-key mutant. A latch keyed
// on anything that does not distinguish adjustments would make the second AO6 row
// reuse the first row's rate. Two AO6 rows on one screen must produce TWO
// entries with DIFFERENT rates.
//
// This arm is deliberately NOT DOS-anchored: it pins the KEY's discrimination,
// which is a property of the port's own bookkeeping, and anchoring it on the
// oracle would confuse a key defect with an arithmetic one.
func TestR57AdjRateLatchIsPerAdjustment(t *testing.T) {
	in := r57Screen()
	// Replace the two typed-rate rows with a SECOND amount-given row, well
	// before the existing one so both are reached.
	in.Adjustments = []RateAdjustment{
		{DateStatus: types.InOutInput, Date: r57Date(2031, time.September, 29),
			AmountStatus: types.InOutInput, Amount: 500.00, AmtOK: true},
		{DateStatus: types.InOutInput, Date: r57Date(2036, time.September, 29),
			AmountStatus: types.InOutInput, Amount: 361.78, AmtOK: true},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Skipf("screen refused (%v) — this arm needs BOTH AO6 rows reached; a "+
			"refusal here is not evidence either way", res.Err)
	}
	latch := res.AdjRateLatch()
	if len(latch) != 2 {
		t.Fatalf("expected TWO latched adjustments (two AO6 rows), got %d: %+v — "+
			"a single entry means the key does not distinguish adjustments",
			len(latch), latch)
	}
	if latch[0].Rate == latch[1].Rate {
		t.Fatalf("the two AO6 rows latched the SAME rate %.10f — the second row "+
			"reused the first's root, which is what a constant key does",
			latch[0].Rate)
	}
	for i, e := range latch {
		if e.Solves != 1 {
			t.Fatalf("adjustment %d solved %d times, want 1", i, e.Solves)
		}
	}
}
