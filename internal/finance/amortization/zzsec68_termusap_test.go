package amortization

// zzsec68_termusap_test.go — regression gate for discrepancies §68 (round 30).
//
// WHAT BROKE. On a USA-rule loan carrying a rate adjustment, the port's solved
// TERM came back one period long while every rendered row, every total and the
// terminating balloon AMOUNT already matched DOS to the cent. Round 29 named the
// case (fuzzer5 arm 50100, seed 50134) and read the mechanism out of the Pascal:
//
//	AMORTOP.pas:73    var f, f_1, p, usap, d, … : real;      { INTERFACE globals }
//	AMORTOP.pas:1323  function DetermineLastPaymentDate (p, usap: real): boolean;
//	AMORTOP.pas:1413  function Iterate (p, usap: real; …): boolean;
//	AMORTOP.pas:1499  procedure Re_Amortize (var p: real);   { p ONLY }
//
// Re_Amortize is a SIBLING of the two probe routines, not nested inside them.
// Its `p` is a var param and reaches whoever called it; its `usap` advance
// (AMORTOP.pas:1610) binds to the unit GLOBAL. DetermineLastPaymentDate and
// Iterate declare `p` and `usap` as BY-VALUE parameters that SHADOW those
// globals, so on their walks Re_Amortize's balance write-back lands and its
// accumulator advance is thrown away. DOS's own ComputeNext trace shows it:
//
//	CN d=137-2-10 p=54319.94 usap=11517.51 int=1324.80 pout=55335.25 uout=12532.82  <- superseded
//	CN d=137-2-10 p=54319.94 usap=11517.51 int= 540.80 pout=54551.25 uout=11748.82  <- Re_Amortize redo
//	CN d=137-5-10 p=54551.25 usap=12532.82 …                                        <- walk RESUMES
//
// — the redo's balance, the SUPERSEDED row's accumulator. Go emulates DOS's
// Output=nil term probe with a DISPLAY walk (see probeARMRow in engine.go), and a
// display walk computes the row only once, so it carried the redo's accumulator
// forward. That is 9.91 of interest at 2037-05-10 (DOS 530.89, port 540.80),
// compounding to ≈317 by period 24, which flips the retirement test.
//
// This is the exact inverse of §66's trap — a hundred rows exactly right while
// the summary scalar is wrong — and unlike §63 it is worth a whole HARD case,
// because solved_term_differs trips the whole-case classifier.
//
// THE SCOPE IS A CALL SITE, NOT A ROUTINE (R22, and R21 on top of it). Only two
// of RepayFancyLoan's eleven call sites shadow the accumulator. The correction is
// gated on input.termHorizonWalk ALONE. An earlier draft gated it on
// `termHorizonWalk || unforced`, matching probeARMRow's existing gate — that also
// captures EstimateAndRefineBalloon's very_last probe (Amortize.pas:637), which
// is parameterless and therefore passes the REAL globals. Measured on this very
// case, that draft fixed the term and simultaneously moved the terminating
// balloon AMOUNT off DOS, 8566.17 → 8249.26. A correction that moves more than
// the defect has not been confined.
//
// THE SUPERSEDED ROW DIFFERS IN ITS PAYMENT TOO, AND THAT IS NOT VISIBLE ON THE
// §68 CASE. Seed 50134's crossing lands on a PREPAYMENT row, whose payment is the
// prepayment amount in both computations, so a rate-only correction fits it
// exactly. The paired regression then found the case that a rate-only correction
// breaks — 373945.61 …, where the crossing is a REGULAR row and DOS's trace shows
// pay 22559.89 superseded against pay 16924.33 in the redo. Substituting only the
// rate lands between the two and is wrong on both. That case is rung 3 below; it
// is the reason this file's fix rebuilds the payment as well.
//
// THE LADDER. Measured with cmd/goamort built from both trees (2026-08-04):
//
//	rung                              PRE                    POST / DOS
//	§68 repro, term + balloon date    n=25, 11/10/2049       n=24, 11/10/2048   FIXED
//	§68 repro, balloon AMOUNT         8566.17                8566.17            unchanged
//	§68 repro, rendered rows/totals   39 rows, 80913.13      identical          unchanged
//	373945.61 regular-crossing        n=57, 698125.09        identical          unchanged
//	495178.90 render-path control     6235.2488 / 392566.73  identical          unchanged
//
// The four "unchanged" rungs are the load-bearing half of this file: they are
// what shows the change is confined to the term probe. Standing rule 3 — every
// fix ships a regression test verified BOTH directions, and where a component's
// revert does NOT change the outcome, say so.

import (
	"fmt"
	"math"
	"testing"
)

// sec68Repro is the §68 case, provenance fuzzer5 arm 50100 seed 50134, `noterm`
// arm, in scope (≤2099). DOS: payment 7582.4600, interest 80913.13, paid
// 158410.02, solvedterm 24 last 2048-11-10.
const sec68Repro = "77496.89 0.0473990000 19 1 b365 prepaid r78 usa " +
	"loandmy=10.9.2025 firstdmy=10.11.2025 mor=2 b14=17529.77 b62=13636.25 " +
	"pre=122:15:4:309.49 adj=26:0.1299630000:5682.71 adj=86:0.1238060000:4812.50 " +
	"adj=134:0.0505390000:8827.81 targ=267.22 pts=0.007932 payhard=7582.46 noterm"

// sec68RenderControl is the case whose long note lives at LoanInput.initUsap: a
// USA-rule loan with a rate adjustment and a skip, rendered rather than probed.
// It exercises the SAME Re_Amortize crossing on the RENDER path, where DOS's
// caller passes the real globals and the accumulator advance DOES land. DOS:
// payment 6235.2488, interest 392566.73, paid 887745.63.
const sec68RenderControl = "495178.90 0.1032190000 216 12 usa " +
	"adj=70:0.0369020000: skip=2,8,11 b126=114024.00"

// sec68RegularCrossing is the case a RATE-ONLY correction regressed, found by
// paired_regression.sh on the 50100 arm before this fix was allowed to land. It
// is the same mechanism as §68 — a USA-rule term probe crossing a rate
// adjustment — but the crossing row is a REGULAR row, so the adjustment's new
// payment (16924.33) replaces the running one (22559.89) in the redo while the
// superseded row kept the old. The port was ALREADY DOS-exact here before round
// 30; nothing about it may move. DOS: payment 22559.8900, interest 698125.09,
// paid 1072070.70, solvedterm 57 last 2042-2-28.
const sec68RegularCrossing = "373945.61 0.1345300000 57 3 b365 exact prepaid r78 usa " +
	"loandmy=30.1.2023 firstdmy=30.6.2023 mor=109 b157=79354.43 b169=107206.85 " +
	"pre=101:12:12:1047.80 adj=121:0.0820360000:16924.33 targ=3050.39 " +
	"pts=0.023524 payhard=22559.89 noterm"

func TestSec68SolvedTermUsapAcrossReAmortize(t *testing.T) {
	in, _ := m5Parse(t, sec68Repro)
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize: %v\n  repro: amort_oracle %s bdump", res.Err, sec68Repro)
	}

	// THE DEFECT. DOS solves 24 periods ending 11/10/2048; the port solved 25
	// ending 11/10/2049 before this fix.
	if res.NPeriods != 24 {
		t.Errorf("solved term = %d periods, DOS says 24 (pre-fix the port said 25)\n"+
			"  repro: amort_oracle %s bdump", res.NPeriods, sec68Repro)
	}
	gotLast := fmt.Sprintf("%d/%d/%d", int(res.LastDate.Time.Month()),
		res.LastDate.Time.Day(), res.LastDate.Time.Year())
	if gotLast != "11/10/2048" {
		t.Errorf("lastdate = %s, DOS says 11/10/2048 (pre-fix the port said 11/10/2049)\n"+
			"  repro: amort_oracle %s bdump", gotLast, sec68Repro)
	}

	// CONTROL 1 — the terminating balloon AMOUNT. It was ALREADY right before the
	// fix; the failure was its DATE, which follows the term. This assertion is
	// what caught the over-broad first draft (8249.26), so it is not decoration.
	var tack ResolvedBalloon
	found := false
	for _, b := range res.Balloons {
		if b.TackedOn {
			tack, found = b, true
		}
	}
	if !found {
		t.Fatalf("no terminating balloon produced\n  repro: amort_oracle %s bdump", sec68Repro)
	}
	if math.Abs(tack.Amount-8566.17) > 0.005 {
		t.Errorf("terminating balloon AMOUNT = %.4f, DOS says 8566.1700 (delta %.4f)\n"+
			"  this value was already correct BEFORE the §68 fix — if it has moved, the\n"+
			"  correction has leaked out of the term probe and into "+
			"EstimateAndRefineBalloon's\n  very_last walk (a draft that gated on "+
			"`unforced` too produced 8249.26 here)\n  repro: amort_oracle %s bdump",
			tack.Amount, tack.Amount-8566.17, sec68Repro)
	}
	gotTackDate := fmt.Sprintf("%d/%d/%d", int(tack.Date.Time.Month()),
		tack.Date.Time.Day(), tack.Date.Time.Year())
	if gotTackDate != "11/10/2048" {
		t.Errorf("terminating balloon DATE = %s, DOS says 11/10/2048\n"+
			"  repro: amort_oracle %s bdump", gotTackDate, sec68Repro)
	}

	// CONTROL 2 — the RENDERED schedule of the very case that was broken. Round 29
	// established that every row already matched DOS to the cent; the fix must not
	// have touched any of them. Totals stand in for the rows here (the full
	// row-for-row diff is the fuzzer's job, and it ran clean).
	if math.Abs(res.TotalInt-80913.13) > 0.005 {
		t.Errorf("total interest = %.2f, DOS says 80913.13 — the RENDER path moved\n"+
			"  repro: amort_oracle %s", res.TotalInt, sec68Repro)
	}
	if math.Abs(res.TotalPaid-158410.02) > 0.005 {
		t.Errorf("total paid = %.2f, DOS says 158410.02 — the RENDER path moved\n"+
			"  repro: amort_oracle %s", res.TotalPaid, sec68Repro)
	}
}

// TestSec68RegularCrossingUnchanged is the R21 control that a rate-only
// correction FAILS. Same defect site, same walk, but the crossing row is a
// regular row whose payment the adjustment replaces — so the superseded
// accumulator was formed from the old payment as well as the old rate. This case
// was DOS-exact before round 30 and must stay so.
func TestSec68RegularCrossingUnchanged(t *testing.T) {
	in, _ := m5Parse(t, sec68RegularCrossing)
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize: %v\n  repro: amort_oracle %s bdump", res.Err, sec68RegularCrossing)
	}
	if res.NPeriods != 57 {
		t.Errorf("solved term = %d periods, DOS says 57\n"+
			"  a §68 correction that substitutes the superseded RATE but not the\n"+
			"  superseded PAYMENT produces 56 here\n  repro: amort_oracle %s bdump",
			res.NPeriods, sec68RegularCrossing)
	}
	gotLast := fmt.Sprintf("%d/%d/%d", int(res.LastDate.Time.Month()),
		res.LastDate.Time.Day(), res.LastDate.Time.Year())
	if gotLast != "2/28/2042" {
		t.Errorf("lastdate = %s, DOS says 2/28/2042 (the rate-only draft said 10/31/2041)\n"+
			"  repro: amort_oracle %s bdump", gotLast, sec68RegularCrossing)
	}
	if math.Abs(res.TotalInt-698125.09) > 0.005 {
		t.Errorf("total interest = %.2f, DOS says 698125.09 (delta %.4f)\n"+
			"  repro: amort_oracle %s", res.TotalInt, res.TotalInt-698125.09, sec68RegularCrossing)
	}
	if math.Abs(res.TotalPaid-1072070.70) > 0.005 {
		t.Errorf("total paid = %.2f, DOS says 1072070.70 (delta %.4f)\n"+
			"  repro: amort_oracle %s", res.TotalPaid, res.TotalPaid-1072070.70, sec68RegularCrossing)
	}
}

// TestSec68RenderPathUnchanged is the R21 negative control: the same
// Re_Amortize-crossing-with-usap machinery reached by a caller that passes DOS's
// real globals. Nothing here may move.
func TestSec68RenderPathUnchanged(t *testing.T) {
	in, _ := m5Parse(t, sec68RenderControl)
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize: %v\n  repro: amort_oracle %s", res.Err, sec68RenderControl)
	}
	if math.Abs(res.TotalInt-392566.73) > 0.005 {
		t.Errorf("total interest = %.2f, DOS says 392566.73 (delta %.4f)\n"+
			"  the §68 correction has leaked onto the RENDER walk, where DOS's caller\n"+
			"  passes the real globals and Re_Amortize's usap advance DOES land\n"+
			"  repro: amort_oracle %s", res.TotalInt, res.TotalInt-392566.73, sec68RenderControl)
	}
	if math.Abs(res.TotalPaid-887745.63) > 0.005 {
		t.Errorf("total paid = %.2f, DOS says 887745.63 (delta %.4f)\n"+
			"  repro: amort_oracle %s", res.TotalPaid, res.TotalPaid-887745.63, sec68RenderControl)
	}
}
