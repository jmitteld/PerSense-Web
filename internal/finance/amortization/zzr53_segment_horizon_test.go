package amortization

import (
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// ROUND 53 — THE SHORT SIDE OF §66's HORIZON CLAMP.
//
// `solveSegmentRate` derives the rate sub-loan's LastDate from a reconstructed
// period count and, before this round, clamped it against the parent's live
// h^.lastdate ONLY DOWNWARD ("Clamp, never extend", round 25). DOS reconstructs
// no count on that path at all: Re_Amortize's rate branch calls
// `Iterate(..., til_adj)` with adjnum = 0 (AMORTOP.pas:1523), so RepayFancyLoan
// stops at very_last (:1140-1142) and ComputeNext decides regular-vs-extra
// against the live h^.lastdate (:606). On the SHORT side the port's sub-walk
// therefore ended one row before DOS's, its terminal was DOS's second-to-last
// balance, and the secant fitted a rate that was too low — which is why 22 of
// the 23 divergent solved rates on the standing arm had Go BELOW DOS.
//
// 🚨 WHAT THIS FILE PINS, AND WHY IT IS A DIFFERENTIAL AND NOT A HARDCODED
// EXPECTED VALUE. Every arm below takes DOS's own answer from the real oracle
// at run time and compares the port to it. A hardcoded number here would be a
// CHANGE DETECTOR, not a correctness pin (START_HERE §5), and it would not
// notice if the oracle itself moved.
//
// BOTH DIRECTIONS, IN FACT (rule 3). Verified by reverting each half of the fix
// in a probe tree at r53:
//   - reverting the SHORT-side extension: TestR53SegmentHorizonShortSide fails
//     (port -0.6158361716 against DOS -0.5459453052).
//   - reverting the LONG-side clamp (the "bolder" variant round 52 measured at
//     1 in 299 with NEW = 1): TestR53SegmentHorizonLongSideClampStands fails.
//   - applying the very_last cap AFTER the do-while overshoot instead of
//     before: TestR53SegmentHorizonShortSide fails (the cap eats the extra row
//     and the change scores nothing at all — round 52's second wrong turn).

// r53Oracle runs the real DOS engine and returns its solved adjustment rate and
// total interest. R61/R64: a timeout and an empty run are their OWN failures,
// never folded into "DOS declined".
func r53Oracle(t *testing.T, args ...string) (rate, interest float64) {
	t.Helper()
	out, err := exec.Command(oracleBin, append(args, "adjdump")...).Output()
	if err != nil {
		t.Fatalf("oracle failed on %v: %v", args, err)
	}
	s := string(out)
	if strings.TrimSpace(s) == "" {
		t.Fatalf("oracle produced NO OUTPUT on %v — that is not a zero (R64)", args)
	}
	var haveRate, haveInt bool
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if strings.HasPrefix(line, "adjrow ") {
			// 🚨 READ ratestatus FIRST. The emission order is
			//	adjrow 1 date 4/28/2023 rate -0.5459453052 ratestatus 1 amount ...
			// so `rate` ALWAYS precedes `ratestatus`. r53's first version tested
			// them in one forward pass and the guard was INERT: it captured the
			// rate and set haveRate before it could ever reach the status,
			// so a USER-TYPED rate (ratestatus 3 = types.InOutInput) would have
			// been compared against the port's SOLVED one. Found by the round's
			// own adversarial audit (F10). R72: read the emission rule — and
			// then make the code implement what the comment claims.
			var status string
			for i := 0; i+1 < len(f); i++ {
				if f[i] == "ratestatus" {
					status = f[i+1]
				}
			}
			if status != "1" {
				continue // DOS did not SOLVE this row (1 == types.InOutOutput).
			}
			for i := 0; i+1 < len(f); i++ {
				if f[i] == "rate" {
					if haveRate {
						t.Fatalf("more than one SOLVED adjustment row on %v — "+
							"this helper would silently keep the LAST one and "+
							"the pin would be ambiguous (R52: check the "+
							"emitter's cardinality)", args)
					}
					rate, _ = strconv.ParseFloat(f[i+1], 64)
					haveRate = true
				}
			}
		}
		if strings.HasPrefix(line, "payment ") {
			for i := 0; i+1 < len(f); i++ {
				if f[i] == "interest" {
					interest, _ = strconv.ParseFloat(f[i+1], 64)
					haveInt = true
				}
			}
		}
	}
	if !haveRate || !haveInt {
		t.Fatalf("oracle output carried no solved adjrow rate and/or no interest "+
			"line on %v — the assertion below would be VACUOUS (R49/R69):\n%s", args, s)
	}
	return rate, interest
}

// r53Screen builds the port-side input for
//
//	amount rate n perYr loandmy=28.2.2023 firstdmy=28.8.2023 \
//	  pre=<preStart>:<preN>:<preFreq>:150.00 adj=<adjMonth>::<adjAmt>
//
// token-for-token as cmd/goamort/main.go builds it, including the load-bearing
// ordering note there: loandmy=/firstdmy= are applied BEFORE the option dates,
// because amort_oracle.pas:759-779 anchors every option date on h^.loandate.
func r53Screen(amount, rate float64, n, perYr, adjMonth int, adjAmt float64,
	preStart, preN, preFreq int) LoanInput {
	loanDate := types.NewDateRec(2023, time.February, 28)
	firstDate := types.NewDateRec(2023, time.August, 28)
	months := func(base types.DateRec, m int) types.DateRec {
		tot := int(base.Time.Month()) - 1 + m
		y := base.Time.Year() + tot/12
		mo := time.Month(tot%12 + 1)
		d := base.Time.Day()
		if last := time.Date(y, mo+1, 0, 0, 0, 0, 0, time.UTC).Day(); d > last {
			d = last
		}
		return types.NewDateRec(y, mo, d)
	}
	return LoanInput{
		// ⚠️ Fancy IS LOAD-BEARING and omitting it silently builds a DIFFERENT
		// SCREEN: cmd/goamort/main.go:357 sets it whenever any advanced option
		// is present, and DOS's Iterate always drives the FANCY terminal when
		// one is (see hasFancyOptions). Without it this test compared an
		// option-blind closed form against DOS's fancy walk — total interest
		// 20552.33 against DOS's -77887.27 — and would have read as a defect in
		// the port. RULE 12: the harness is a suspect before the engine is.
		Fancy: true,
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: loanDate,
			FirstStatus: types.InOutInput, FirstDate: firstDate,
		},
		// cmd/goamort/main.go:352 coerces the basis for the weekly/biweekly
		// grids. Mirroring it costs nothing and stops this helper manufacturing
		// a false divergence the day someone adds a perYr 26 screen here — the
		// stratum carrying ALL of this fix's GAINED screens and 31 of its 51
		// FARTHER ones. (⚠️ NOT "the whole effect": 546 of the 619 screens the
		// fix moves CLOSER to DOS are at OTHER frequencies. r53 audit N6.)
		Settings: settingsFor(perYr),
		Adjustments: []RateAdjustment{{
			DateStatus: types.InOutInput, Date: months(loanDate, adjMonth),
			AmountStatus: types.InOutInput, Amount: adjAmt, AmtOK: true,
		}},
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: months(loanDate, preStart),
			NNStatus: types.InOutInput, NN: preN,
			PerYrStatus: types.InOutInput, PerYr: preFreq,
			PaymentStatus: types.InOutInput, Payment: 150.00,
		}},
	}
}

// settingsFor mirrors cmd/goamort/main.go's basis selection.
func settingsFor(perYr int) Settings {
	// ⚠️ 365.25, NOT 365. cmd/goamort/main.go:351-352 is the coercion this
	// mirrors; the API reaches the same constant at internal/api/handlers.go:935
	// via internal/finance/interest/rates.go:23-24. (main.go's own comment cites
	// handlers.go:850, which is mortgage what-if code — a stale citation
	// inherited from before a refactor. Do not propagate it. r53 audit N10.)
	if perYr == 26 || perYr == 52 {
		return Settings{Basis: types.Basis365, PerYr: byte(perYr),
			YrDays: 365.25, YrInv: 1.0 / 365.25}
	}
	return Settings{Basis: types.Basis360, PerYr: byte(perYr),
		YrDays: 360, YrInv: 1.0 / 360}
}

// TestR53SegmentHorizonShortSide is the round's headline pin. On this screen
// the pristine port's sub-walk ends one row early and its secant fits
// -0.6158361716; DOS fits -0.5459453052; the port now reproduces DOS to every
// digit the oracle prints, and the schedule's total interest lands on DOS's.
func TestR53SegmentHorizonShortSide(t *testing.T) {
	gateOracle(t)
	const (
		amount = 100000.00
		rate   = 0.08
		n      = 24
		perYr  = 6
	)
	dosRate, dosInt := r53Oracle(t,
		"100000.00", "0.08", "24", "6",
		"loandmy=28.2.2023", "firstdmy=28.8.2023",
		"pre=2:6:12:150.00", "adj=2::900.00")

	res := Amortize(r53Screen(amount, rate, n, perYr, 2, 900.00, 2, 6, 12))
	if res.Err != nil {
		t.Fatalf("port REFUSED a screen DOS solves (rate %.10f): %v", dosRate, res.Err)
	}
	// ⚠️ ASSERT THE ENGINE. There are two, and the one you are reading is
	// probably not the one that answered (START_HERE §5). This defect lives in
	// the piecewise walk; a dosport answer here would make the assertion below
	// a statement about a different code path.
	if got := string(res.EngineUsed); got != "" && got != "piecewise" {
		t.Fatalf("expected the piecewise engine to answer this screen, got %q — "+
			"the pin below would be about a different code path (M11)", got)
	}
	if math.Abs(res.TotalInt-dosInt) > 0.005 {
		t.Errorf("total interest = %.2f, DOS = %.2f (delta %.2f). The rate "+
			"sub-loan's horizon is short again: solveSegmentRate must EXTEND the "+
			"derived LastDate up to min(h^.lastdate, very_last) and then take the "+
			"do-while overshoot row (AMORTOP.pas:1221), not merely clamp downward.",
			res.TotalInt, dosInt, res.TotalInt-dosInt)
	}
	// The rate itself, through the adjustment row the screen paints.
	var got float64
	var found bool
	for _, a := range res.Adjustments {
		// RateSolved is the port's own "this is DOS output, not user input"
		// flag — the analogue of the oracle's `ratestatus 1`, which is the
		// emission rule r53Oracle reads on the other side (R72).
		if a.RateSolved {
			got, found = a.Rate, true
		}
	}
	if !found {
		t.Fatalf("the port painted NO solved rate on the adjustment row, so the "+
			"comparison against DOS's %.10f could not run (R55: a guard that "+
			"skips the empty case cannot find the bug whose symptom is emptiness)",
			dosRate)
	}
	if math.Abs(got-dosRate) > 1e-6 {
		t.Errorf("solved adjustment rate = %.10f, DOS = %.10f (delta %.3e)",
			got, dosRate, got-dosRate)
	}
}

// TestR53SegmentHorizonLongSideClampStands is the NEGATIVE half. Round 25's
// DOWNWARD clamp must survive untouched: the "bolder" variant round 52 measured
// — which REPLACED the clamp instead of complementing it — reached 1 in 299 on
// the standing arm and booked paired NEW = 1. Re-measured at r53 with the
// refusal question settled, it STILL books a paired regression, so it still
// does not ship.
//
// 🚨 THIS SCREEN WAS FOUND BY MEASUREMENT, NOT CHOSEN. The round's first
// attempt at this guard used the same shape as the short-side test and named
// itself after the long side; a probe tree that removed ONLY round 25's clamp
// left it GREEN, so the name was a false claim about what it pinned (START_HERE
// §5: a guard's failure message is a claim — verify the WHY). This screen is
// one of the 43 — out of 449 the two variants disagree on — where dropping the
// clamp lands FARTHER from DOS: shipped 76458.06 == DOS 76458.06 exactly, the
// bolder variant 42072.07.
func TestR53SegmentHorizonLongSideClampStands(t *testing.T) {
	gateOracle(t)
	dosRate, dosInt := r53Oracle(t,
		"100000.00", "0.08", "12", "4",
		"loandmy=28.2.2023", "firstdmy=28.8.2023",
		"pre=4:90:26:150.00", "adj=6::12000.00")
	res := Amortize(r53Screen(100000.00, 0.08, 12, 4, 6, 12000.00, 4, 90, 26))
	if res.Err != nil {
		t.Fatalf("port REFUSED a screen DOS solves (rate %.10f): %v", dosRate, res.Err)
	}
	if math.Abs(res.TotalInt-dosInt) > 0.005 {
		t.Errorf("total interest = %.2f, DOS = %.2f (delta %.2f) — round 25's "+
			"DOWNWARD clamp at solveSegmentRate has been removed or weakened. "+
			"The short-side extension COMPLEMENTS it; it does not replace it.",
			res.TotalInt, dosInt, res.TotalInt-dosInt)
	}
}

// TestR53SegmentBoundSweepIsCommitted is the R73/rule-6 guard: the SECOND
// GENERATOR is part of the claim, and round 52's was not committed, which is
// why its blocking finding could not be re-derived at r53.
//
// 🚨 IT ASSERTS ACROSS FILES (R50). A guard that reads the file it lives in is
// unconditionally true. This one reads a Python harness from a Go test and
// checks the properties that make it a REFUTATION instrument: it must classify
// a lost answer as its own bucket, it must gate that verdict on DOS having
// solved the row, and it must fail closed.
func TestR53SegmentBoundSweepIsCommitted(t *testing.T) {
	const p = "../../../testplan/harness/r53_segment_bound_sweep.py"
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("the second generator is missing (%s): a fix in this solver "+
			"family may not ship without one (R73), and the instrument is part "+
			"of the claim (rule 6): %v", p, err)
	}
	s := string(b)
	for _, want := range []string{
		`return "LOST"`,      // the R67 bucket exists
		`dosSolved`,          // and is gated on DOS having solved the row
		`ratestatus`,         // via the emission rule, not a proxy (R72)
		`TimeoutExpired`,     // a hang is not a refusal (R61/R64)
		`"state": "timeout"`, // and it gets its own bucket
		`def selftest`,       // adjdump additivity is CONTROLLED, not assumed
	} {
		if !strings.Contains(s, want) {
			t.Errorf("second generator lost a load-bearing property: %q absent "+
				"from %s", want, p)
		}
	}
}
