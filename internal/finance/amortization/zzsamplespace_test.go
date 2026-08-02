package amortization

import (
	"math"
	"math/rand"
	"testing"
)

// zzsamplespace_test.go — round 16b: THE GENERATOR'S SAMPLE SPACE IS AN
// ASSERTED FACT, NOT FOLKLORE.
//
// Twice now the project has quoted a convergence number over a space it
// believed was the whole screen domain and was not:
//
//   - `years := 8 + rng.Intn(18)` meant NO schedule longer than 25 years had
//     ever been generated; removing the bound moved the measured divergence
//     rate from 1 in 3,600 to 1 in 290 with no code change (§52).
//   - `ppy ∈ {12,24,26,52}` meant a prepayment series slower than monthly had
//     probability ZERO; round 9 measured the unreachable region at 30%
//     divergent once it could be reached.
//
// Both bounds were visible in the code and invisible in every report. These
// tests turn the envelope (the fz5* constants in dos_fuzzer5_test.go) into a
// MANIFEST that fails loudly when a bound moves, so an envelope change is a
// reviewed decision with a diff, never a drive-by.
//
// A failing test here does not mean the generator is wrong. It means the
// generator's reach CHANGED — update the manifest in the same commit, and
// update the audit (docs/fuzzer_sample_space_audit_2026-08-02.md) if a
// documented gap opened or closed.

// TestSampleSpaceManifest pins the scalar envelope.
//
// WHAT THE GENERATOR CANNOT PRODUCE (the audit's core table, executable form —
// full analysis with status and rationale per row in
// docs/fuzzer_sample_space_audit_2026-08-02.md):
//
//	terms shorter than 8 YEARS            silent floor since day one
//	non-whole-year terms (n ≠ years*perYr) silent lattice; only the 4000 cap breaks it
//	amounts outside [$25k, $500k)         silent
//	rates outside [3%, 14%)               silent — the bit harness itself uses a
//	                                      29% screen, so DOS's domain is wider
//	rate = 0                              silent (classic closed-form edge)
//	payments outside 0.85x-1.35x fair     documented for the forward mode; the
//	                                      CONSEQUENCE for backward modes is that
//	                                      a solved cell can only land near its
//	                                      entered value (see the reach test)
//	loan dates outside 2023-2025          visible in the draw, consequence not
//	                                      documented: origin-side century
//	                                      boundaries are never sampled
//	balloons in the last 20% of the term  silent — TackOnFinalBalloon's
//	                                      interaction with a LATE typed balloon
//	                                      is unsampled
//	negative/zero adjustment rates        silent; adj rates draw [2%, 15%)
//	prepay series STARTING past the term  unreachable — and it is stratum A's
//	                                      one known real remainder (backlog #8)
//	skip patterns beyond 6 fixed strings  visible; monthly only
//	points ≥ 4%                           silent
//	Daily compounding                     never set by gzSettings — whole axis
//	                                      unfuzzed here (dedicated tests only)
//	sub-monthly × month-anchored options  documented; blocked on absolute-date
//	                                      tokens (backlog #13)
//	payoff screens                        not this fuzzer's axis (backlog #15)
func TestSampleSpaceManifest(t *testing.T) {
	type bound struct {
		name string
		got  float64
		want float64
	}
	manifest := []bound{
		// Term bands: contiguous 8..300 years, whole-band arithmetic.
		{"min term (years)", float64(fz5YearsBand1Lo), 8},
		{"band1 hi", float64(fz5YearsBand1Lo + fz5YearsBand1W - 1), 25},
		{"band2 lo", float64(fz5YearsBand2Lo), 26},
		{"band2 hi", float64(fz5YearsBand2Lo + fz5YearsBand2W - 1), 60},
		{"band3 lo", float64(fz5YearsBand3Lo), 61},
		{"band3 hi", float64(fz5YearsBand3Lo + fz5YearsBand3W - 1), 120},
		{"band4 lo", float64(fz5YearsBand4Lo), 121},
		{"max term (years)", float64(fz5YearsBand4Lo + fz5YearsBand4W - 1), 300},
		{"row cap", float64(fz5NCap), 4000},
		// Money.
		{"amount lo", fz5AmountLo, 25000},
		{"amount hi (excl)", fz5AmountLo + fz5AmountSpan, 500000},
		{"rate lo", fz5RateLo, 0.03},
		{"rate hi (excl)", fz5RateLo + fz5RateSpan, 0.14},
		{"pay frac lo", fz5PayFracLo, 0.85},
		{"pay frac hi (excl)", fz5PayFracLo + fz5PayFracSpan, 1.35},
		// Dates.
		{"loan year lo", float64(fz5LoanYearLo), 2023},
		{"loan year hi", float64(fz5LoanYearLo + fz5LoanYearN - 1), 2025},
		// Options.
		{"points hi (excl)", fz5PointsSpan, 0.04},
		{"balloon frac lo", fz5BalloonFracLo, 0.02},
		{"balloon frac hi (excl)", fz5BalloonFracLo + fz5BalloonSpan, 0.30},
		{"balloon budget frac", fz5BalloonBudgetFrac, 0.60},
		{"pre amt frac lo", fz5PreAmtFracLo, 0.03},
		{"pre amt frac hi (excl)", fz5PreAmtFracLo + fz5PreAmtSpan, 0.25},
		{"pre nn cap", float64(fz5PreNNCap), 400},
		{"adj rate lo", fz5AdjRateLo, 0.02},
		{"adj rate hi (excl)", fz5AdjRateLo + fz5AdjRateSpan, 0.15},
		{"adj pay frac lo", fz5AdjPayFracLo, 0.75},
		{"adj pay frac hi (excl)", fz5AdjPayFracLo + fz5AdjPaySpan, 1.45},
		{"targ frac lo", fz5TargFracLo, 0.02},
		{"targ frac hi (excl)", fz5TargFracLo + fz5TargSpan, 0.25},
		{"max prepay series", float64(fz5MaxPrepay), 2},
	}
	for _, b := range manifest {
		if math.Abs(b.got-b.want) > 1e-12 {
			t.Errorf("envelope changed: %s = %v, manifest says %v — if this is "+
				"deliberate, update the manifest AND the audit doc in the same "+
				"commit (docs/fuzzer_sample_space_audit_2026-08-02.md)",
				b.name, b.got, b.want)
		}
	}
	// Band contiguity: a hole between bands would be an unsampled interior
	// region, which is worse than a bounded exterior one because nothing in any
	// report would hint at it.
	if fz5YearsBand1Lo+fz5YearsBand1W != fz5YearsBand2Lo ||
		fz5YearsBand2Lo+fz5YearsBand2W != fz5YearsBand3Lo ||
		fz5YearsBand3Lo+fz5YearsBand3W != fz5YearsBand4Lo {
		t.Error("term bands are no longer contiguous — a hole in the interior " +
			"of the term axis is invisible to every rate this fuzzer reports")
	}
}

// TestSampleSpaceTermDrawReach drives the actual draw function.
// Deterministic (fixed seed): over 200k draws every year value in 8..300 must
// be hit and the band weights must sit at their documented 60/20/10/10.
func TestSampleSpaceTermDrawReach(t *testing.T) {
	rng := rand.New(rand.NewSource(7701))
	const N = 200000
	seen := map[int]int{}
	bands := [4]int{}
	for i := 0; i < N; i++ {
		y := fz5DrawYears(rng)
		seen[y]++
		switch {
		case y <= 25:
			bands[0]++
		case y <= 60:
			bands[1]++
		case y <= 120:
			bands[2]++
		default:
			bands[3]++
		}
	}
	for y := 8; y <= 300; y++ {
		if seen[y] == 0 {
			t.Errorf("year=%d never drawn in %d draws — a hole in the term axis", y, N)
		}
	}
	if lo, hi := minKey(seen), maxKey(seen); lo != 8 || hi != 300 {
		t.Errorf("term draw range = %d..%d, manifest says 8..300", lo, hi)
	}
	for i, want := range [4]float64{0.6, 0.2, 0.1, 0.1} {
		got := float64(bands[i]) / N
		if math.Abs(got-want) > 0.02 {
			t.Errorf("band %d weight = %.3f, documented %.1f — the corpus-density "+
				"tradeoff at the term draw has shifted", i+1, got, want)
		}
	}
}

// TestSampleSpaceWholeYearTermLattice is executable documentation of a SILENT
// gap this audit found: n is always years*perYr, so the term axis lives on the
// whole-year lattice. A 30-year monthly loan (n=360) is generated; a 100-period
// one (8y4m) has NEVER been generated by this fuzzer, at any frequency, in any
// round. The only off-lattice values are the fz5NCap clamp.
//
// This is a DOCUMENTED limitation, not an assertion that it is fine: DOS's term
// cell takes any integer, `non` derives n from an arbitrary typed last date in
// production, and the term-solve corpus (docs/termsolve_residual_corpus) is full
// of off-lattice n. If the lattice is ever widened, delete this test and update
// the audit doc — that is the reviewed-decision mechanism working as intended.
func TestSampleSpaceWholeYearTermLattice(t *testing.T) {
	for _, perYr := range []int{1, 2, 3, 4, 6, 12, 24, 26, 52} {
		reachable := map[int]bool{}
		for years := 8; years <= 300; years++ {
			n := years * perYr
			if n > fz5NCap {
				n = fz5NCap
			}
			reachable[n] = true
		}
		// Representative off-lattice terms a real user types every day.
		for _, n := range []int{100, 250, 361} {
			if n%perYr == 0 && n/perYr >= 8 && n/perYr <= 300 {
				continue // on-lattice at this frequency; not a witness here
			}
			if n < fz5NCap && reachable[n] {
				t.Errorf("perYr=%d: n=%d is off the whole-year lattice yet marked "+
					"reachable — the lattice premise has changed; update the audit doc",
					perYr, n)
			}
		}
	}
}

// TestSampleSpaceBackwardSolvedCellBand pins the audit's backward-mode reach
// finding with one concrete, closed-form screen.
//
// The hardened payment is fair(entered rate) × [0.85, 1.35). On a `norate`
// screen the solver then recovers the rate whose fair payment equals that
// draw — so the SOLVED rate the fuzzer can present to the comparison is not
// "any rate" but a band around the entered one. On a plain 30-year monthly
// screen at 9%, that band is (computed here by bisection on the closed form)
// roughly 6.9%..12.6%: `norate` NEVER exercises the solver's recovery of a
// rate far from the entered cell on a plain screen. Round 15 hit exactly this
// wall from the other side — its over-refusal probe had to draw the payment
// INDEPENDENTLY (0.05x-6x of amount/n) before a divergent rate solve was
// reachable at all.
func TestSampleSpaceBackwardSolvedCellBand(t *testing.T) {
	const (
		perYr = 12.0
		n     = 360.0
		rate  = 0.09
	)
	fair := func(r float64) float64 {
		i := r / perYr
		return i / (1 - math.Pow(1+i, -n))
	}
	solveRateForPay := func(pay float64) float64 {
		lo, hi := 1e-6, 1.0
		for k := 0; k < 200; k++ {
			mid := (lo + hi) / 2
			if fair(mid) < pay {
				lo = mid
			} else {
				hi = mid
			}
		}
		return (lo + hi) / 2
	}
	f := fair(rate)
	loRate := solveRateForPay(f * fz5PayFracLo)
	hiRate := solveRateForPay(f * (fz5PayFracLo + fz5PayFracSpan))
	t.Logf("norate reach on a plain 30y/12/9%% screen: solved-rate band %.4f .. %.4f "+
		"(entered 0.0900, pay frac %.2f..%.2f)", loRate, hiRate, fz5PayFracLo,
		fz5PayFracLo+fz5PayFracSpan)
	// Pin the band loosely; the point is its EXISTENCE and rough width, not its
	// fourth decimal. If the pay-frac envelope widens, this moves and the
	// manifest test above already fired.
	if loRate < 0.055 || loRate > 0.080 {
		t.Errorf("low edge of the solved-rate band = %.4f, expected ~0.069 — "+
			"the backward-mode reach analysis in the audit doc is stale", loRate)
	}
	if hiRate < 0.115 || hiRate > 0.140 {
		t.Errorf("high edge of the solved-rate band = %.4f, expected ~0.126 — "+
			"the backward-mode reach analysis in the audit doc is stale", hiRate)
	}
}

func minKey(m map[int]int) int {
	first := true
	lo := 0
	for k := range m {
		if first || k < lo {
			lo, first = k, false
		}
	}
	return lo
}

func maxKey(m map[int]int) int {
	hi := 0
	for k := range m {
		if k > hi {
			hi = k
		}
	}
	return hi
}
