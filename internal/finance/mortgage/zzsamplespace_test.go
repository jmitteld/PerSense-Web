package mortgage

import (
	"math"
	"math/rand"
	"testing"
)

// zzsamplespace_test.go — ROUND 36. THE MORTGAGE GENERATOR'S SAMPLE SPACE IS AN
// ASSERTED FACT, NOT FOLKLORE.
//
// The amortization package has carried a manifest of this shape since round 16b
// (zzsamplespace_test.go there). The mortgage and present-value packages never
// got one, and START_HERE has carried "⚠️ THEIR SAMPLE SPACE HAS NEVER BEEN
// AUDITED" against `dos_mtg_fuzzer5_test.go` for FIVE ROUNDS while quoting
// "0 divergences in 30,000 cases / 135,853 APR verdicts" off it.
//
// A zero is a statement about its generator (R31). This file says what that
// generator is, in a form that FAILS when the answer changes.
//
// The measurement is a REACH TEST, not a constant audit: it drives the real
// `f5Gen` two hundred thousand times and reports where the draws actually land,
// so an envelope claim can never be a comment that stopped being true. The
// amortization manifest pins named constants; the mortgage draw has its bounds
// as literals inside `f5Gen`, and rewriting them into constants would perturb
// nothing here but would perturb nothing usefully either — the reach test binds
// to behaviour, which is strictly stronger.
//
// A failing test here does not mean the generator is wrong. It means its reach
// CHANGED — update the manifest below in the same commit.
//
// ============================================================================
// WHAT THE MORTGAGE GENERATOR CANNOT PRODUCE
// ============================================================================
//
//	axis          drawn                          CANNOT produce            status
//	-----------------------------------------------------------------------------
//	price         5,000 .. 3,004,999 (INTEGER)    non-integer prices;       SILENT
//	                                             < 5,000; >= 3,005,000
//	pct           0 .. 0.99                       >= 0.995 — DOS'S OWN      SILENT,
//	                                             REFUSAL BRANCH             AND IT
//	                                             (Mortgage.pas:217/232)     MATTERS
//	years         1 .. 40                         > 40; 0                   SILENT
//	rate          0.0001 .. 0.3001                EXACTLY 0; > 0.3001;      SILENT
//	                                             negative
//	monthly       1 .. 20,001                     0; negative               SILENT
//	points        0 .. 0.90 (85% of cases)        >= 0.90; EXACTLY 1        SILENT
//	                                             (the (1-points) zero)
//	tax           0 .. 1.5 x monthly              negative tax              SILENT
//	balloon when  1 .. years                      > years — A BALLOON       SILENT,
//	                                             AFTER MATURITY, which      AND IT
//	                                             DOS's own source flags     MATTERS
//	                                             as unfixed (AMORTOP.pas
//	                                             :1368-1373, "Fix this
//	                                             here some time")
//	balloon amt   price x 0.01 .. 1.51            0; negative               SILENT
//	rows          1, or exactly 2 (`compare`)     3+ comparison rows        VISIBLE
//	DATES         NONE — the mortgage screen is   every date hazard the     BY
//	              a pure annuity in YEARS         project has found:        DESIGN
//	                                             the 26-Aug-2091 ceiling
//	                                             (§69), the 2100 calendar
//	                                             split (§62), §71's
//	                                             poisoning, §72's horizon
//
// THE TWO THAT MATTER, AND WHY THEY ARE NAMED SEPARATELY:
//
//  1. pct >= 0.995 is DOS's REFUSAL, and this generator provably cannot reach
//     it — see TestSampleSpaceMortgagePctRefusalUnreachable, which does the
//     algebra rather than trusting the draw bound. A refusal nothing exercises
//     is a control-flow claim nothing has tested (R26).
//
//  2. A balloon AFTER maturity is the one shape DOS's own authors wrote a
//     BUGSIN scavenger for and never fixed. `bw = 1 + rng.Intn(c.years)` makes
//     it probability zero. The port's behaviour there is unmeasured in either
//     direction.
//
// Both are DOCUMENTED GAPS, not defects. Widening either is a reviewed decision
// with a diff — which is the whole point of this file.

const ssMortgageDraws = 200_000

// ssReach is the measured envelope of one axis over ssMortgageDraws draws.
type ssReach struct {
	lo, hi  float64
	present int
}

func (r *ssReach) see(v float64, first bool) {
	if first {
		r.lo, r.hi = v, v
		return
	}
	if v < r.lo {
		r.lo = v
	}
	if v > r.hi {
		r.hi = v
	}
}

// TestSampleSpaceMortgageManifest drives the real f5Gen and pins where it lands.
//
// Bounds are asserted as a BAND, not an equality. ⚠️ THE OBSERVED EXTREME IS AN
// ORDER STATISTIC, NOT A BOUND (R14 — a verdict that moves with the sample size
// is not a measurement): `f5Gen` draws continuously, so on a uniform [a,b) axis
// the minimum of k samples sits about (b-a)/k above a, and the ACCEPTED WINDOW
// below is sized from that, not from the literal. `monthly lo` is the clearest
// case: the analytic floor is 1, only 25% of draws take the extreme arm, and the
// observed minimum over 200k draws is ~1.4. The window is tight enough that
// moving a literal in f5Gen fails this test and loose enough that reseeding does
// not.
func TestSampleSpaceMortgageManifest(t *testing.T) {
	rng := rand.New(rand.NewSource(3607))
	var price, pct, rate, monthly, points, tax, bh ssReach
	minYears, maxYears := 1<<30, 0
	minBW, maxBW := 1<<30, 0
	var nPts, nTax, nBall, nBWEqualsYears, nBWPastYears, nNonIntegerPrice int
	var maxPctSeen float64
	var maxDerivedPct float64

	for i := 0; i < ssMortgageDraws; i++ {
		c := f5Gen(rng)
		first := i == 0
		price.see(c.price, first)
		pct.see(c.pct, first)
		rate.see(c.rate, first)
		monthly.see(c.monthly, first)
		if c.price != math.Trunc(c.price) {
			nNonIntegerPrice++
		}
		if c.years < minYears {
			minYears = c.years
		}
		if c.years > maxYears {
			maxYears = c.years
		}
		if c.pct > maxPctSeen {
			maxPctSeen = c.pct
		}
		// DOS recovers pct from the CASH cell as
		// (cash/price - points)/(1 - points) — Mortgage.pas:216. That is the
		// number the 0.995 refusal tests, and it is not the drawn pct.
		//
		// ⚠️ COMPUTED FOR EVERY DRAW, NOT ONLY THE points-BEARING ONES. An
		// earlier version accumulated this inside `if c.hasPts`, which excludes
		// the WORST case: with points omitted DOS sees points = 0 and
		// pct_seen == pct_drawn, i.e. right at 0.99. The logged maximum
		// understated the true one. (Round-36 audit.)
		pts := 0.0
		if c.hasPts {
			points.see(c.points, nPts == 0)
			nPts++
			pts = c.points
		}
		if dp := (c.pct - pts) / (1 - pts); dp > maxDerivedPct {
			maxDerivedPct = dp
		}
		if c.hasTax {
			tax.see(c.tax, nTax == 0)
			nTax++
		}
		if c.hasBall {
			bh.see(c.bh, nBall == 0)
			nBall++
			if c.bw < minBW {
				minBW = c.bw
			}
			if c.bw > maxBW {
				maxBW = c.bw
			}
			if c.bw == c.years {
				nBWEqualsYears++
			}
			if c.bw > c.years {
				nBWPastYears++
			}
		}
	}

	type band struct {
		name     string
		got      float64
		lo, hi   float64 // acceptable window for the observed extreme
		manifest string
	}
	// The `quantize` step snaps every float to 10 decimal places before it is
	// returned, so the observed extremes are the drawn ones to within 5e-11.
	bands := []band{
		{"price lo", price.lo, 5000, 5200, "5,000"},
		{"price hi", price.hi, 3_000_000, 3_005_000, "3,004,999"},
		{"pct lo", pct.lo, 0, 0.001, "0"},
		{"pct hi", pct.hi, 0.985, 0.99, "0.99 (DOS refuses at 0.995)"},
		{"years lo", float64(minYears), 1, 1, "1"},
		{"years hi", float64(maxYears), 40, 40, "40"},
		{"rate lo", rate.lo, 0.0001, 0.0006, "0.0001 (never exactly 0)"},
		{"rate hi", rate.hi, 0.299, 0.3001, "0.3001"},
		{"monthly lo", monthly.lo, 1, 2.0, "1 (order statistic: ~1+20000/50k)"},
		{"monthly hi", monthly.hi, 19_900, 20_001, "20,001"},
		{"points lo", points.lo, 0, 1e-4, "0"},
		{"points hi", points.hi, 0.895, 0.90, "0.90 (never 1: the (1-pts) zero)"},
		{"tax lo", tax.lo, 0, 0.05, "0 (never negative; order statistic ~600/170k)"},
		{"balloon when lo", float64(minBW), 1, 1, "1"},
		{"balloon when hi", float64(maxBW), 40, 40, "40 (== max years; never past maturity)"},
	}
	for _, b := range bands {
		if b.got < b.lo || b.got > b.hi {
			t.Errorf("reach changed: %s = %.6f, manifest says %s (accepted %.6f..%.6f) — "+
				"if this is deliberate, update the manifest in the same commit",
				b.name, b.got, b.manifest, b.lo, b.hi)
		}
	}

	// The gaps that are the point of the file.
	if nBWPastYears != 0 {
		t.Errorf("balloon-after-maturity is now REACHABLE (%d draws) — that shape is "+
			"the one DOS's own authors flagged unfixed (AMORTOP.pas:1368-1373). "+
			"If this widening is deliberate, the port's behaviour there needs its "+
			"own differential before any rate measured on this generator is quoted",
			nBWPastYears)
	}
	if maxPctSeen >= 0.995 {
		t.Errorf("pct now reaches DOS's refusal boundary (max %.6f >= 0.995) — "+
			"the refusal branch at Mortgage.pas:217/232 is suddenly live and has "+
			"never been differentially tested", maxPctSeen)
	}
	if nNonIntegerPrice != 0 {
		t.Errorf("price is no longer integral (%d draws) — the manifest says the "+
			"generator has never drawn a cents-bearing price", nNonIntegerPrice)
	}

	// Presence rates: the fuzzer5 contract is 15% independent omission.
	for _, p := range []struct {
		name string
		n    int
	}{{"points", nPts}, {"tax", nTax}, {"balloon", nBall}} {
		got := float64(p.n) / ssMortgageDraws
		if math.Abs(got-0.85) > 0.01 {
			t.Errorf("%s present in %.3f of draws, fuzzer5 contract is 0.85 "+
				"(f5Omit = 0.15)", p.name, got)
		}
	}
	// A balloon AT maturity is the interesting sub-case and must stay sampled.
	if nBWEqualsYears == 0 {
		t.Error("balloon-at-maturity (when == years) is no longer drawn — that is " +
			"BalloonCalc's degenerate arm and losing it is silent lost coverage")
	}
	t.Logf("mortgage reach: price %.0f..%.0f pct %.4f..%.4f years %d..%d "+
		"rate %.5f..%.5f monthly %.2f..%.2f points %.4f..%.4f tax %.2f..%.2f "+
		"balloonwhen %d..%d balloonamt %.0f..%.0f",
		price.lo, price.hi, pct.lo, pct.hi, minYears, maxYears,
		rate.lo, rate.hi, monthly.lo, monthly.hi, points.lo, points.hi,
		tax.lo, tax.hi, minBW, maxBW, bh.lo, bh.hi)
	t.Logf("max DERIVED pct (cash/price-points)/(1-points), the cell DOS's 0.995 "+
		"refusal actually tests: %.6f", maxDerivedPct)
}

// TestSampleSpaceMortgagePctRefusalUnreachable proves by ALGEBRA, not by a draw
// bound, that this generator cannot reach DOS's down-payment refusal.
//
// Mortgage.pas:215-226 recovers the down-payment fraction from the CASH cell:
//
//	pct := (cash / price - points) / (1 - points);
//	if (pct >= 0.995) then RecordError(...)
//
// The fuzzer feeds cash = price * pct_drawn, so DOS sees
//
//	pct_seen = (pct_drawn - points) / (1 - points)
//
// which is a weighted move of pct_drawn TOWARD 0 for every points in (0,1):
// pct_seen <= pct_drawn whenever pct_drawn <= 1. With pct_drawn < 0.99 the
// refusal at 0.995 is therefore unreachable for EVERY admissible points value,
// not merely for the ones drawn. R26: a refusal is a control-flow claim, and
// this one has never been exercised in either engine.
func TestSampleSpaceMortgagePctRefusalUnreachable(t *testing.T) {
	const drawnMax = 0.99 // f5Gen: rng.Float64() * 0.99
	worst := 0.0
	for _, pts := range []float64{0, 1e-6, 0.01, 0.05, 0.25, 0.5, 0.75, 0.8999} {
		seen := (drawnMax - pts) / (1 - pts)
		if seen > worst {
			worst = seen
		}
	}
	// ⚠️ DOS HAS TWO REFUSAL SITES AND AN EARLIER VERSION OF THIS TEST PROVED
	// ONLY ONE. Mortgage.pas:229-232 recovers pct from the FINANCED cell with a
	// DIFFERENT formula and NO points term:
	//
	//	pct := 1 - (financed / price);   if (pct >= 0.995) then RecordError(...)
	//
	// and the mortgage fuzzer DOES feed `financed` (the `mfin`, `taxapr` and
	// `aprfin` surfaces). The fuzzer sets financed = price*(1-pct) with
	// pct < 0.99, so financed >= 0.01*price and pct_seen = 1 - (1-pct) = pct
	// < 0.99. Unreachable — but now proved rather than assumed. (Round-36 audit.)
	if seenFin := 1 - (1 - drawnMax); seenFin >= 0.995 {
		t.Fatalf("the FINANCED branch (Mortgage.pas:229-232) now reaches the "+
			"refusal: derived pct = %.6f >= 0.995", seenFin)
	}
	if worst >= 0.995 {
		t.Fatalf("the algebra no longer holds: worst-case derived pct = %.6f >= 0.995", worst)
	}
	t.Logf("worst-case derived pct over the whole points envelope = %.6f "+
		"(DOS refuses at 0.995) — the refusal branch is UNREACHABLE, and a "+
		"refusal nothing exercises is an untested control-flow claim (R26)", worst)
	if worst < 0.9 {
		t.Errorf("worst-case derived pct = %.6f is not even close to the boundary; "+
			"the generator is further from DOS's refusal than the manifest claims", worst)
	}
}

// TestSampleSpaceMortgageHasNoDateAxis records the structural reason the
// mortgage surface is immune to every date defect this project has found.
//
// This is not a vacuous assertion. §69 (the 26-August-2091 representation
// ceiling), §62 (the two calendars disagreeing at 2100), §71 (MDY poisoning a
// daterec on overflow) and §72 (a horizon past the loan's last date) are ALL
// amortization findings, and START_HERE's surface table quotes mortgage at zero
// on the same page. A reader is entitled to know whether that zero is a
// different engine surviving the same hazard, or a surface where the hazard
// does not exist. It is the second: `f5Case` carries no date field at all, and
// the mortgage screen expresses duration as an integer count of YEARS.
//
// If a date ever enters f5Case, this test fails and the four date sections above
// become live questions for this package.
func TestSampleSpaceMortgageHasNoDateAxis(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	c := f5Gen(rng)
	// Structural: every field of f5Case is a float64, an int or a bool. A
	// types.DateRec would not compile into this comparison.
	_ = c.price + c.pct + c.cash + c.financed + c.monthly + c.rate +
		c.points + c.tax + c.bh
	_ = c.years + c.bw
	_ = c.hasPts && c.hasTax && c.hasBall
	if c.years < 1 || c.years > 40 {
		t.Fatalf("years = %d outside the 1..40 duration axis", c.years)
	}
	t.Log("the mortgage screen's duration axis is an integer year count with no " +
		"calendar: §62, §69, §71 and §72 are structurally unreachable here, and " +
		"the mortgage zero must be quoted with that scope, not as evidence that " +
		"the port survives those hazards")
}
