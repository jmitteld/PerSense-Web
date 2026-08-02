package amortization

// Harness defect #10 (round 18) — the terminating-balloon tolerance was scaled
// to the LOAN AMOUNT while the value it guards, the tack balance, is not bounded
// by the loan.
//
// R4 test: this file tests the HARNESS, not the engine. A failure here means the
// instrument's adjudication rule changed, which invalidates every
// `balloon_value_differs` count the project has quoted — so it is pinned in BOTH
// directions (the old rule's verdict AND the new rule's verdict are asserted on
// the same measured rows) rather than only forward. Per standing rule 3, each
// row records which component's revert changes the outcome; the rows tagged
// wasHard=true, nowHard=false are exactly the population the fix silences, and
// the rows tagged true/true are the population it must keep.
//
// Every row below is a REAL measurement copied out of round 18's two mode arms
// (seeds 60000-60039, FUZZ_N=400, oracle at /tmp/oraclebuild/amort_oracle).
// Provenance for each is the reproducing oracle command in the `cmd` field
// (standing rule 6).

import (
	"math"
	"testing"
)

// fz5TackTol is the adjudication rule under test. It must stay a literal mirror
// of the expression in dos_fuzzer5_test.go's `case dosHasTack && goHasTack`
// arm — if that expression changes and this one does not, the third assertion
// in TestFz5TackToleranceScaling fails and says so.
func fz5TackTol(loanAmount, dosTack float64) float64 {
	return math.Max(math.Max(0.05, 1e-5*math.Abs(loanAmount)), 5e-4*math.Abs(dosTack))
}

// fz5TackTolPreR18 is the tolerance as it stood through round 17. Kept so the
// fix is verifiable in the reverse direction: it is the ONLY way to assert that
// a silenced row really was being reported before.
func fz5TackTolPreR18(loanAmount, _ float64) float64 {
	return math.Max(0.05, 1e-5*math.Abs(loanAmount))
}

type tackTolCase struct {
	name    string
	cmd     string // provenance: the reproducing oracle command line
	loan    float64
	dos     float64
	goVal   float64
	wasHard bool // the pre-round-18 rule reported this as SIG=HARD
	nowHard bool // the round-18 rule reports it as SIG=HARD
	why     string
}

var tackTolCases = []tackTolCase{
	// ---- Population A: runaway balances, agreement is excellent RELATIVE ----
	// These are the rows the fix silences. Each agrees with DOS to between 1e-4
	// and 1e-8 of the compared value while failing a $2-5 absolute tolerance.
	{
		name: "runaway/113x_loan_agrees_to_4e-4",
		cmd: "amort_oracle 282439.70 0.0640300000 1260 12 b365_360 exact inadv plusreg r78 " +
			"loandmy=29.11.2023 firstdmy=29.12.2023 b628=11073.67 pre=98:244:24:109.24 " +
			"targ=81.21 skip=1,7 pts=0.030499 payhard=1916.17 non lastdmy=29.11.2128 bdump",
		loan: 282439.70, dos: -32069176.55, goVal: -32056065.01,
		wasHard: true, nowHard: false,
		why: "13111.54 absolute on a 32M balance is 4.09e-4 — inside the totals' own slope",
	},
	{
		name: "runaway/574x_loan_agrees_to_7e-8",
		cmd: "amort_oracle 304376.43 0.0963980000 4000 24 exact prepaid inadv plusreg r78 " +
			"loandmy=15.11.2024 firstdmy=15.12.2024 targ=125.59 pts=0.012160 " +
			"payhard=1609.43 non lastdmy=15.3.2102 bdump",
		loan: 304376.43, dos: -162273410.69, goVal: -162273398.60,
		wasHard: true, nowHard: false,
		why: "$12.09 apart on $162M; the engines agree to eight significant figures",
	},
	{
		name: "runaway/4785x_loan_agrees_to_8e-8",
		cmd: "amort_oracle 63203.04 0.1038940000 1416 24 r78 loandmy=15.12.2024 " +
			"firstdmy=15.1.2025 targ=45.24 pts=0.030187 payhard=337.75 non " +
			"lastdmy=15.12.2142 bdump",
		loan: 63203.04, dos: -3027687677.37, goVal: -3027687424.65,
		wasHard: true, nowHard: false,
		why: "old tolerance here was $0.63 against a $3.03e9 value",
	},

	// ---- Population B: real disagreements that MUST survive ----
	{
		name: "real/noterm_sign_flip_DOS_negative_Go_positive",
		cmd: "amort_oracle 49726.63 0.1048540000 249 1 b365_360 exact prepaid inadv plusreg " +
			"r78 usa loandmy=12.10.2024 firstdmy=12.10.2025 b264=13027.49 b420=10892.59 " +
			"b1224=5915.90 pre=1536:376:6:61.13 targ=135.78 payhard=5994.80 noterm bdump",
		loan: 49726.63, dos: -321878.17, goVal: 6974.06,
		wasHard: true, nowHard: true,
		why: "opposite signs on a de-activated tack row — the round 18 mechanism",
	},
	{
		name: "real/non_27pct_disagreement",
		cmd: "amort_oracle 401813.12 0.0364930000 388 4 b365_360 exact plusreg r78 usa " +
			"loandmy=7.5.2024 firstdmy=7.8.2024 mor=84 pre=393:11:4:177.99 " +
			"pre=717:93:2:398.30 targ=916.82 pts=0.001661 payhard=5048.20 non " +
			"lastdmy=7.5.2121 bdump",
		loan: 401813.12, dos: -4883231.48, goVal: -3530748.36,
		wasHard: true, nowHard: true,
		why: "27.7% relative — two orders of magnitude past the 5e-4 slope",
	},
	{
		name: "real/non_58pct_disagreement",
		cmd: "amort_oracle 302327.98 0.0592480000 376 4 b365_360 exact inadv plusreg r78 " +
			"usa loandmy=26.5.2025 firstdmy=26.7.2025 mor=104 b275=72349.73 b731=80867.77 " +
			"pre=641:149:4:619.74 pre=377:155:2:787.72 non bdump",
		loan: 302327.98, dos: -18106176.12, goVal: -7650575.76,
		wasHard: true, nowHard: true,
		why: "57.7% relative; survives comfortably despite the 10.5M absolute gap",
	},
	{
		name: "real/non_91pct_on_a_modest_balance",
		cmd: "amort_oracle 40117.65 0.0533410000 1770 6 exact inadv usa " +
			"loandmy=27.7.2025 firstdmy=27.10.2025 mor=3 b309=1531.06 b723=10892.50 " +
			"b1231=1248.55 pre=585:60:4:25.62 targ=35.20 pts=0.032806 payhard=463.28 " +
			"non lastdmy=27.8.2064 bdump",
		loan: 40117.65, dos: -500286.82, goVal: -43908.74,
		wasHard: true, nowHard: true,
		why: "91.2% relative at only 12.5x growth, and a pre-2100 last date",
	},
	// The marginal row. rel = 7.465e-4 sits just PAST the 5e-4 slope, so the fix
	// keeps it — included deliberately, because a tolerance change is only
	// trustworthy if its boundary is pinned as well as its interior.
	{
		name: "boundary/1.57M_x_loan_at_rel_7.5e-4_is_KEPT",
		cmd: "amort_oracle 215422.44 0.1382790000 1344 24 b365 prepaid inadv r78 " +
			"loandmy=30.3.2025 firstdmy=30.5.2025 targ=87.28 pts=0.005845 " +
			"payhard=1629.03 non lastdmy=30.4.2137 bdump",
		loan: 215422.44, dos: -338726031977.9105, goVal: -338473168569.69,
		wasHard: true, nowHard: true,
		why: "largest growth in the arm (1.57e6x) yet still reported: the fix is " +
			"a slope change, not a blanket amnesty for big balances",
	},

	// ---- Population C: the old premise actually holds, tolerance UNCHANGED ----
	// |tack| <= 0.02 x loan, so the loan-scaled term still dominates and the fix
	// is a strict no-op. This is the row that proves no sensitivity was traded
	// away on schedules that amortize.
	{
		name: "amortizing/small_tack_tolerance_is_unchanged",
		cmd:  "synthetic control: |tack| = 0.004 x loan, inside the old premise",
		loan: 250000.00, dos: 1000.00, goVal: 1003.10,
		wasHard: true, nowHard: true,
		why: "$3.10 apart against a $2.50 tolerance under BOTH rules — unchanged",
	},
	{
		name: "amortizing/small_tack_inside_tolerance_under_both",
		cmd:  "synthetic control: same shape, difference below the loan-scaled floor",
		loan: 250000.00, dos: 1000.00, goVal: 1001.20,
		wasHard: false, nowHard: false,
		why: "$1.20 apart against $2.50 — passes under BOTH rules",
	},
}

func TestFz5TackToleranceScaling(t *testing.T) {
	silenced, kept, unchanged := 0, 0, 0

	for _, c := range tackTolCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			diff := math.Abs(c.goVal - c.dos)

			gotNow := diff > fz5TackTol(c.loan, c.dos)
			gotWas := diff > fz5TackTolPreR18(c.loan, c.dos)

			// Direction 1 — the rule in force today.
			if gotNow != c.nowHard {
				t.Errorf("round-18 rule: got HARD=%v, want %v\n"+
					"  diff=%.4f tol=%.4f rel=%.3e\n  %s\n  %s",
					gotNow, c.nowHard, diff, fz5TackTol(c.loan, c.dos),
					diff/math.Abs(c.dos), c.why, c.cmd)
			}

			// Direction 2 — the rule it replaced. Without this a "fix" that
			// silences a row nobody was reporting would look identical to one
			// that removes a real false positive.
			if gotWas != c.wasHard {
				t.Errorf("pre-round-18 rule: got HARD=%v, want %v (diff=%.4f tol=%.4f)\n  %s",
					gotWas, c.wasHard, diff, fz5TackTolPreR18(c.loan, c.dos), c.cmd)
			}

			switch {
			case c.wasHard && !c.nowHard:
				silenced++
				// A silenced row must be silenced for the STATED reason — value
				// -relative agreement — and not because some other term of the
				// max() happened to swallow it. Anything the fix removes has to
				// be inside the totals' own slope.
				if rel := diff / math.Abs(c.dos); rel > 5e-4 {
					t.Errorf("row silenced but rel=%.3e exceeds the 5e-4 slope — "+
						"the fix is hiding a real disagreement", rel)
				}
			case c.wasHard && c.nowHard:
				kept++
			}
			if fz5TackTol(c.loan, c.dos) == fz5TackTolPreR18(c.loan, c.dos) {
				unchanged++
			}
		})
	}

	// The fix has to actually do something in both directions, or the table has
	// silently stopped covering the defect.
	if silenced == 0 {
		t.Error("no row is silenced by the round-18 rule — the table no longer " +
			"covers harness defect #10")
	}
	if kept == 0 {
		t.Error("no row survives the round-18 rule — the tolerance is now too loose " +
			"to detect anything")
	}
	if unchanged == 0 {
		t.Error("no row leaves the tolerance unchanged — the control population " +
			"proving amortizing schedules lost no sensitivity is missing")
	}
	t.Logf("tack tolerance: %d rows silenced (all within 5e-4 relative), "+
		"%d kept, %d with an identical tolerance under both rules",
		silenced, kept, unchanged)
}

// The tack tolerance must never be TIGHTER than the tolerance the same walk's
// totals are held to. If it were, a schedule could pass on its totals and fail
// on its terminal balance for no reason but the constant chosen — which is
// exactly how defect #10 presented.
func TestFz5TackToleranceIsNoTighterThanTotals(t *testing.T) {
	for _, loan := range []float64{25000, 100000, 282439.70, 500000} {
		for _, tack := range []float64{1e2, 1e4, 1e6, 1e9, 1.5e11} {
			totalsSlope := math.Max(1.0, 5e-4*tack) // intTol / paidTol, same walk
			got := fz5TackTol(loan, tack)
			// Allow the loan-scaled floor to make it LOOSER, never tighter,
			// once the value-scaled term is the binding one.
			if 5e-4*tack > 1e-5*loan && got < totalsSlope-1e-9 {
				t.Errorf("loan=%.0f tack=%.0f: tack tol %.4f is tighter than the "+
					"totals tol %.4f on the same schedule", loan, tack, got, totalsSlope)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// R10, round 18b — the tolerance-scaling consistency metric, and its own
// two-directional validation.
//
// The audit that followed defect #10 needed a way to ask "is this constant
// keyed to the right quantity?" without knowing the right answer in advance.
// The FIRST attempt measured the separation gap between passing and failing
// populations, and it FAILED its validation: with the old tack tolerance
// restored, the gap read 30,184x and 815,435,001x — wider than the fixed
// tolerance's. Defect #10 is bimodal in that space too, because the ratio was
// tracking balance magnitude rather than agreement quality.
//
// What does characterise it is that the tolerance's IMPLIED RELATIVE PRECISION
// varied by orders of magnitude across the sample: the old constant demanded
// ~1e-4 relative of a small balance and 2.1e-11 of a large one. A tolerance
// keyed to the value it guards asks every case for the same number of
// significant figures, so tol/|value| is flat.
//
// This test pins that metric in both directions on pure functions — no oracle,
// no fuzzing, runs in microseconds — so the detector cannot silently rot.
// Measured live for comparison (seeds 60000/60001, `non` arm):
//	old tack tolerance: SPREAD 3.2e+07, 2.8e+06
//	new tack tolerance: SPREAD 1.4e+00, 1.2e+00

// tackValuesFromTheArm is the measured envelope of |tack|/|loan| from round 18's
// `non` and `noterm` arms: minimum 0.1, median 59.9, maximum 1,572,380. A
// scaling metric evaluated on a narrower range than the generator actually
// produces would have cleared the old tolerance too.
var tackGrowthEnvelope = []float64{0.1, 0.8, 1.0, 1.6, 12.5, 59.9, 113, 574,
	4785, 82998, 344829, 1572380}

func spreadOf(tol func(loan, value float64) float64, loans, growths []float64) float64 {
	minRel, maxRel := math.Inf(1), 0.0
	for _, loan := range loans {
		for _, g := range growths {
			v := loan * g
			rel := tol(loan, v) / v
			if rel < minRel {
				minRel = rel
			}
			if rel > maxRel {
				maxRel = rel
			}
		}
	}
	return maxRel / minRel
}

func TestFz5ToleranceScalingIsConsistent(t *testing.T) {
	// The generator's actual loan range.
	loans := []float64{25000, 49726.63, 100000, 282439.70, 401813.12, 500000}

	cases := []struct {
		name      string
		tol       func(loan, value float64) float64
		maxSpread float64
		why       string
	}{
		{
			name: "tack/round18-fixed",
			tol:  func(loan, v float64) float64 { return fz5TackTol(loan, v) },
			// Flat at the 5e-4 slope wherever it binds; the loan-scaled floor
			// lifts it only on balances below 2% of the loan.
			maxSpread: 1e3,
			why:       "keyed to the value guarded",
		},
		{
			name:      "tack/pre-round18-DEFECTIVE",
			tol:       func(loan, v float64) float64 { return fz5TackTolPreR18(loan, v) },
			maxSpread: math.Inf(1), // asserted separately, below
			why:       "keyed to the loan while guarding an unbounded balance",
		},
		{
			name: "totals/interest-and-paid",
			tol:  func(_, v float64) float64 { return math.Max(1.0, 5e-4*math.Abs(v)) },
			// Flat at 5e-4 above $2,000; the $1 floor lifts it below that.
			maxSpread: 1e3,
			why:       "keyed to DOS's own total",
		},
		{
			name:      "solve/amount",
			tol:       func(_, v float64) float64 { return 0.01 + 2e-6*math.Abs(v) },
			maxSpread: 1e3,
			why:       "keyed to DOS's solved amount",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			s := spreadOf(c.tol, loans, tackGrowthEnvelope)
			t.Logf("spread of tol/|value| over the measured envelope: %.2e (%s)", s, c.why)
			if math.IsInf(c.maxSpread, 1) {
				return
			}
			if s > c.maxSpread {
				t.Errorf("tolerance %q demands relative precision varying by %.2e "+
					"across the generator's own envelope (limit %.0e). That is the "+
					"defect-#10 signature: the constant is keyed to a quantity that "+
					"does not track the value it guards.", c.name, s, c.maxSpread)
			}
		})
	}

	// DIRECTION 2 — the metric must FLAG the known-bad constant. Without this the
	// thresholds above could be met by a metric that flags nothing at all, which
	// is exactly how the first version of this instrument passed while being
	// useless.
	t.Run("metric/detects-the-known-defect", func(t *testing.T) {
		bad := spreadOf(func(loan, v float64) float64 { return fz5TackTolPreR18(loan, v) },
			loans, tackGrowthEnvelope)
		good := spreadOf(func(loan, v float64) float64 { return fz5TackTol(loan, v) },
			loans, tackGrowthEnvelope)
		t.Logf("pre-round-18 tack tolerance spread %.2e; round-18 %.2e; "+
			"discrimination %.2ex", bad, good, bad/good)
		if bad < 1e6 {
			t.Errorf("the metric scores the KNOWN-DEFECTIVE tolerance at only %.2e — "+
				"it is not detecting defect #10 and cannot be trusted to detect the "+
				"next one", bad)
		}
		if bad/good < 1e3 {
			t.Errorf("the metric barely separates the defective tolerance (%.2e) from "+
				"the fixed one (%.2e); a detector needs orders of magnitude, not a "+
				"factor", bad, good)
		}
	})

	// The rate tolerance is ABSOLUTE (5e-6) rather than value-scaled, which is
	// the defect-#10 shape on its face — so it gets its own check rather than a
	// pass. It survives for a reason the tack tolerance could not claim: a RATE
	// is a bounded quantity. Over the generator's 3-14% band the implied relative
	// precision varies only by the width of that band.
	t.Run("solve/rate-absolute-but-bounded", func(t *testing.T) {
		minRate, maxRate := 0.03, 0.14
		s := (5e-6 / minRate) / (5e-6 / maxRate)
		t.Logf("rate tolerance is absolute; implied relative precision varies %.1fx "+
			"across the 3-14%% band (%.1e at 14%%, %.1e at 3%%)",
			s, 5e-6/maxRate, 5e-6/minRate)
		if s > 10 {
			t.Errorf("the absolute rate tolerance varies %.1fx in relative terms — "+
				"if the generator's rate band widens this must become value-scaled", s)
		}
	})
}
