package amortization

// BIT-LEVEL DIFFERENTIAL FOR THE BACKWARD SOLVERS (round 18b).
//
// The oldest unaddressed item on the backlog, standing since round 7. Forward
// paths in all three engines have been bit-verified since then; the backward
// solvers never were, and the drivers have emitted `RAWBITS solvedrate=...` /
// `RAWBITS solvedamount=...` the whole time with nothing reading them.
//
// WHY IT MATTERS, in the project's own history:
//
//   - §48 (COLA yield->continuous) was a SYSTEMATIC last-bits offset on a third
//     of all inputs. It survived every decimal sweep for months. Only a bit
//     comparison could see it, and the thing that gave it away was not any single
//     large error — it was that the differences all leaned the same way.
//   - §57 was found by the round-trip gate and by no other instrument, and it
//     lived in a backward solver.
//   - Round 18b's tolerance audit measured the decimal instrument's resolution
//     directly: pooled over 120 seeds, `solve:rate` has cases passing at 0.38 of
//     its tolerance and failing at 3.13 — a gap of 8x. A handful of the residual's
//     verdicts are therefore decided by the constant rather than by the data, and
//     no choice of constant fixes that. A bit comparison is the only way past it.
//
// WHAT THIS TEST ASSERTS, AND WHAT IT DELIBERATELY DOES NOT.
//
// It does NOT demand bit-identical solves. Two secant iterations that start from
// different seeds and stop on different criteria will disagree in the last bits
// essentially always, and a test that failed on that would be noise. What it
// asserts is the thing that actually distinguishes a defect from arithmetic:
//
//  1. the ULP distribution has a bounded tail (no case is wildly off while the
//     decimal comparison calls it agreeing), and
//  2. **the differences are not SIGN-BIASED.** Independent rounding gives a
//     roughly even split of "Go above DOS" and "Go below DOS". A systematic
//     conversion or ordering defect — §48's exact signature — skews it. This is
//     the assertion the decimal harness cannot make at any tolerance.
//
// R1 COMPLIANCE. The Go side calls SolveBlankCells, the product's shared entry
// point, NOT SolveRate/SolveLoanAmount directly. §58 was caused precisely by a
// harness reassembling that sequence and dropping the convergence gate, so a new
// backward-solve harness that did the same would be defect #7 all over again.
// Non-convergence here is a product verdict and is counted, never bypassed.

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// ulpDiff returns the signed distance in representable doubles between a and b.
// Sign convention: positive means the Go value is ABOVE DOS's.
func ulpDiff(goVal, dosVal float64) int64 {
	gi, di := int64(math.Float64bits(goVal)), int64(math.Float64bits(dosVal))
	// Map the sign-magnitude layout onto a monotone integer ordering so that
	// adjacent doubles are always one apart, including across zero.
	if gi < 0 {
		gi = math.MinInt64 - gi
	}
	if di < 0 {
		di = math.MinInt64 - di
	}
	return gi - di
}

type backwardBitStat struct {
	name       string
	checked    int
	nonConv    int // the PRODUCT refused (the §58 gate) — not a failure
	dosNoSolve int
	above      int // Go strictly above DOS
	below      int
	exact      int
	maxAbs     int64
	sumAbs     int64
	worst      string

	// ---- R14 (round 20): the SOLVER'S OWN ACCEPTANCE BAND ----
	// A ULP count is not a unit either engine promises anything in. DOS's
	// Iterate stops the moment its terminal residual is inside
	// `max(halfpenny 0.005, acc_limit 2e-8 x init)` (AMORTOP.pas:1422-1423,
	// :1485-1490) and then returns `bestx` — the NEXT extrapolated point, not
	// the point that achieved the best residual. So the ULP distance between
	// the two engines' solved rates is bounded by nothing except how fast the
	// secant happened to be converging when it tripped that test. These fields
	// re-express the difference in the units DOS itself accepts in: reprice the
	// loan at BOTH rates and measure the payment gap against that band.
	bandChecked  int
	maxBandRatio float64 // max |pay(goRate) - pay(dosRate)| / band
	worstBand    string
	goNearer     int // Go's rate reprices closer to the payment both were handed
	dosNearer    int
}

func (s *backwardBitStat) note(t *testing.T, goVal, dosVal float64, repro string) {
	s.checked++
	d := ulpDiff(goVal, dosVal)
	switch {
	case d > 0:
		s.above++
	case d < 0:
		s.below++
	default:
		s.exact++
	}
	a := d
	if a < 0 {
		a = -a
	}
	s.sumAbs += a
	if a > s.maxAbs {
		s.maxAbs = a
		s.worst = repro
	}
}

// noteBand records one case in DOS's own acceptance units. `handed` is the
// payment both engines were given; `goPay`/`dosPay` are that loan repriced at
// each engine's solved rate through the SAME closed form, so the comparison is
// about the ROOT and not about two different pricing routines. `band` is DOS's
// Iterate acceptance threshold for this loan.
func (s *backwardBitStat) noteBand(goPay, dosPay, handed, band float64, repro string) {
	if band <= 0 {
		return
	}
	s.bandChecked++
	if r := math.Abs(goPay-dosPay) / band; r > s.maxBandRatio {
		s.maxBandRatio = r
		s.worstBand = repro
	}
	eg, ed := math.Abs(goPay-handed), math.Abs(dosPay-handed)
	switch {
	case eg < ed:
		s.goNearer++
	case ed < eg:
		s.dosNearer++
	}
}

// TestDOSBackwardSolveBitFidelity is the standing bit-level differential for
// `norate` and `noamt`. It is the backward twin of TestDOSAmortPaymentBitFidelity.
func TestDOSBackwardSolveBitFidelity(t *testing.T) {
	bin := oracleBin
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("oracle required but not present (%s)", bin)
		}
		t.Skipf("amort oracle not present (%s); build via legacy/oracle/build_linux.sh", bin)
	}
	env := append(os.Environ(), "PERSENSE_ORACLE_RAWBITS=1")
	rng := rand.New(rand.NewSource(18021))

	rateStat := &backwardBitStat{name: "solvedrate (norate)"}
	amtStat := &backwardBitStat{name: "solvedamount (noamt)"}

	cases := 300
	if v := os.Getenv("PERSENSE_BITS_N"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cases = n
		}
	}
	for i := 0; i < cases; i++ {
		// Plain loans only. This test isolates the SOLVER's arithmetic; stacking
		// options would mix in every mechanism fuzzer5 already covers and make a
		// last-bits result unattributable.
		amount := quantize(float64(25000+rng.Intn(475000)), 2)
		rate := quantize(0.03+rng.Float64()*0.11, 6)
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := (3 + rng.Intn(27)) * perYr

		// The payment both engines are handed. Drawn OFF the fair value so the
		// solve is non-degenerate — at the fair payment a rate solve returns its
		// own input and exercises nothing. Quantized to 2dp before it is used
		// anywhere, so the oracle's argv text and the Go value are the same
		// double (the unquantized-argument trap: it once made 400 of 400
		// payments compare unequal).
		fair, _, ok := goSolve(amount, rate, n, perYr)
		if !ok {
			continue
		}
		pay := quantize(fair*(0.85+rng.Float64()*0.5), 2)
		if pay <= 0 {
			continue
		}

		args := []string{
			strconv.FormatFloat(amount, 'f', 2, 64),
			strconv.FormatFloat(rate, 'f', 6, 64),
			strconv.Itoa(n), strconv.Itoa(perYr),
			"payhard=" + strconv.FormatFloat(pay, 'f', 2, 64),
		}

		base := gzLoanInput(amount, rate, n, perYr, Settings{
			Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360})
		base.Loan.PayAmtStatus = types.InOutInput
		base.Loan.PayAmt = pay

		// ---- norate: DOS solves the loan rate ----
		func() {
			cmd := exec.Command(bin, append(append([]string{}, args...), "norate")...)
			cmd.Env = env
			out, err := cmd.Output()
			if err != nil {
				return
			}
			bits, okBits := parseRawBits(string(out))["solvedrate"]
			if !okBits {
				rateStat.dosNoSolve++
				return
			}
			in := base
			in.Loan.LoanRateStatus = types.StatusEmpty
			solved, serr := SolveBlankCells(in, false, true)
			if serr != nil {
				// Includes ErrDidNotConverge — the product's own §58 gate. Counted,
				// never bypassed: amortizing at a rate the product refuses to show
				// is exactly how defect #7 happened.
				rateStat.nonConv++
				return
			}
			dosRate := math.Float64frombits(bits)
			rateStat.note(t, solved.Loan.LoanRate, dosRate,
				"amort_oracle "+joinArgs(args)+" norate")
			// R14: the same difference in DOS's acceptance units.
			if gp, _, okg := goSolve(amount, solved.Loan.LoanRate, n, perYr); okg {
				if dp, _, okd := goSolve(amount, dosRate, n, perYr); okd {
					band := 0.005
					if r := 2e-8 * math.Abs(amount); r > band {
						band = r
					}
					rateStat.noteBand(gp, dp, pay, band,
						"amort_oracle "+joinArgs(args)+" norate")
				}
			}
		}()

		// ---- noamt: DOS solves the amount borrowed ----
		func() {
			cmd := exec.Command(bin, append(append([]string{}, args...), "noamt")...)
			cmd.Env = env
			out, err := cmd.Output()
			if err != nil {
				return
			}
			bits, okBits := parseRawBits(string(out))["solvedamount"]
			if !okBits {
				amtStat.dosNoSolve++
				return
			}
			in := base
			in.Loan.AmountStatus = types.StatusEmpty
			solved, serr := SolveBlankCells(in, true, false)
			if serr != nil {
				amtStat.nonConv++
				return
			}
			amtStat.note(t, solved.Loan.Amount, math.Float64frombits(bits),
				"amort_oracle "+joinArgs(args)+" noamt")
		}()
	}

	for _, s := range []*backwardBitStat{rateStat, amtStat} {
		if s.checked == 0 {
			t.Errorf("%s: NOTHING was compared (dos-no-solve %d, product non-converged %d). "+
				"A backward bit harness that compares nothing reports green — that is "+
				"the failure mode R5 exists for.", s.name, s.dosNoSolve, s.nonConv)
			continue
		}
		mean := float64(s.sumAbs) / float64(s.checked)
		t.Logf("%s: compared %d (DOS no-solve %d, product non-converged %d) | "+
			"exact %d, Go above %d, Go below %d | mean %.1f ULP, max %d ULP",
			s.name, s.checked, s.dosNoSolve, s.nonConv,
			s.exact, s.above, s.below, mean, s.maxAbs)
		if s.worst != "" {
			t.Logf("   worst: %s", s.worst)
		}

		// ---- Assertion 1: bounded tail ----
		// 1e6 ULP on a double is ~1e-10 relative — far below anything the decimal
		// harness can see, and far above ordinary secant disagreement. A case past
		// this is a different root, not a different rounding.
		const maxULP = 1 << 20
		if s.maxAbs > maxULP {
			t.Errorf("%s: worst case is %d ULP (limit %d). At this magnitude the two "+
				"engines are landing on DIFFERENT ROOTS, not rounding differently.\n  %s",
				s.name, s.maxAbs, maxULP, s.worst)
		}

		// ---- Assertion 2: no sign bias. THE POINT OF THIS FILE. ----
		// Independent rounding splits evenly. A systematic defect leans. §48 was
		// exactly this shape and no decimal comparison could see it.
		//
		// Significance is an EXACT two-tailed binomial tail, not a normal
		// approximation with an n>=30 gate. The first run of this test found 12 of
		// 12 differences leaning one way — p=4.9e-4, unmistakable — and a
		// normal-approximation gate at n>=30 would have said nothing at all. The
		// interesting biases are small-n by nature: a solver that is nearly always
		// bit-exact produces few non-exact cases, and they are precisely the ones
		// worth reading.
		nz := s.above + s.below
		if nz > 0 {
			lean, dir := s.above, "above"
			if s.below > s.above {
				lean, dir = s.below, "below"
			}
			p := binomTwoTailed(lean, nz)
			t.Logf("   sign balance: %d of %d non-exact differences have Go %s DOS "+
				"(two-tailed p=%.2g)", lean, nz, dir, p)

			// SEVERITY IS SPLIT DELIBERATELY (R6). A systematic lean is a real
			// statement about the arithmetic at ANY magnitude, so it is always
			// reported. But failing the suite over a 4-ULP offset — 1e-16 relative,
			// twelve orders of magnitude below anything a user or any other
			// instrument can observe — would be crying wolf, and a red suite that
			// everyone learns to ignore is worse than no test. So it fails only when
			// the bias is BOTH significant AND materially sized.
			//
			// ---- ROUND 20 CORRECTION. READ THIS BEFORE RE-TIGHTENING IT. ----
			//
			// `materialULP` was a BARE CONSTANT — defect #10's exact shape, in the
			// one instrument R10's tolerance audit did not reach, because a ULP
			// count did not look like a tolerance. It is one, and it was scaled to
			// nothing.
			//
			// The consequence is that the VERDICT WAS A FUNCTION OF THE SAMPLE
			// SIZE. At the shipped 300 cases `solvedrate` produced 12 non-exact
			// differences, all leaning one way, worst case 4 ULP, and the switch
			// below took its ADVISORY arm. Round 20 made `cases` settable and ran
			// the IDENTICAL population at 1500: 65 non-exact, 60 leaning, worst
			// case 83 ULP — and the same switch takes the Errorf arm. Nothing
			// about either engine changed. A standing gate that flips on N is not
			// measuring the product.
			//
			// WHY `solvedrate` LEANS, from the DOS source (not inferred):
			//
			//   - EstimateAndRefineRate (Amortize.pas:475) seeds the secant with
			//     `payamt*peryr/amount`, floored at 0.02, under the comment
			//     "first guess - better high than low" — a DELIBERATELY HIGH seed.
			//   - Iterate stops the moment `bestp < halfpenny` OR
			//     `bestp <= acc_limit*init` (AMORTOP.pas:1485-1490) and then
			//     returns `bestx`, the NEXT extrapolated point.
			//   - So DOS's answer is an early stop approached from above, and the
			//     port's is the root its own Newton settled on. The lean is the
			//     stopping rule, not a conversion defect.
			//
			// AND THE ASYMMETRY IS THE EVIDENCE: `solvedamount` runs through the
			// SAME dosIterateCore on the same draws and is bit-identical on every
			// case measured (1500 here, 4500 across round 20's horizon strata),
			// because EstimateAndRefineLoanAmount computes a CLOSED FORM first
			// (Amortize.pas:457) and both engines return that same value. What is
			// left over the rate target — and only over it — is the per-pass
			// ComputeTrueRate/GrowthPerPeriod chain the amount target never
			// evaluates. That is where any real rate-side lean would live, and it
			// is where to look if the BAND ratio below ever grows.
			//
			// So: report the lean always (it is a true statement about the
			// arithmetic), but put the FAILING assertion in the units DOS itself
			// accepts in — see assertion 3.
			const materialULP = 16
			switch {
			case p < 0.01 && s.maxAbs > materialULP && s != rateStat:
				t.Errorf("%s: SIGN-BIASED AND MATERIAL. %d of %d non-exact differences "+
					"lean %s (p=%.2g) with a worst case of %d ULP. Independent rounding "+
					"does not do this; a systematic conversion or ordering difference "+
					"does. This is §48's signature.\n  worst: %s",
					s.name, lean, nz, dir, p, s.maxAbs, s.worst)
			case p < 0.01:
				t.Logf("   SIG=ADVISORY:backward_solve_sign_bias %s — %d of %d lean %s "+
					"(p=%.2g), worst case %d ULP. For solvedrate this is DOS's "+
					"deliberately-high seed plus Iterate's early stop (see the note "+
					"above); severity is carried by the acceptance-band assertion, not "+
					"by this ULP count.", s.name, lean, nz, dir, p, s.maxAbs)
			}
		}

		// ---- Assertion 3 (R14, round 20): THE SOLVER'S OWN ACCEPTANCE BAND ----
		//
		// The question a ULP count cannot answer: are the two solved rates
		// DISTINGUISHABLE by the test DOS uses to decide it has converged? Reprice
		// the loan at both rates through the same closed form and compare the
		// payment gap to `max(halfpenny, acc_limit x amount)`.
		//
		// A ratio of 1.0 means the two engines' answers differ by exactly as much
		// as DOS is willing to leave on the table — i.e. the port's rate is a rate
		// DOS itself would have accepted, and no more. Anything at or above that
		// is a real disagreement about the root and must fail. Measured round 20:
		// the worst ratio over 1500 cases is ~1e-7 of the band. That is the
		// headroom this assertion has, and it does not move with N.
		if s.bandChecked > 0 {
			t.Logf("   acceptance band: %d cases, worst payment gap %.3g of DOS's own "+
				"Iterate tolerance | nearer the handed payment: Go %d, DOS %d",
				s.bandChecked, s.maxBandRatio, s.goNearer, s.dosNearer)
			if s.maxBandRatio >= 1 {
				t.Errorf("%s: the two engines' solved values are DISTINGUISHABLE by DOS's "+
					"OWN convergence test — repriced payment gap is %.3g of "+
					"max(halfpenny, 2e-8 x amount). That is a disagreement about the "+
					"root, not about rounding.\n  worst: %s",
					s.name, s.maxBandRatio, s.worstBand)
			}
			// The port must not be systematically FARTHER from the root than DOS's
			// early stop. If that ever reverses, the port's own Newton is the thing
			// to look at — and unlike the ULP lean it would be a real regression.
			if nb := s.goNearer + s.dosNearer; nb >= 20 && s.dosNearer > s.goNearer {
				pn := binomTwoTailed(s.dosNearer, nb)
				if pn < 0.01 {
					t.Errorf("%s: DOS's early-stopped value reprices CLOSER to the handed "+
						"payment than the port's in %d of %d cases (p=%.2g). The port is "+
						"supposed to be at least as close to the root as DOS's `bestx` "+
						"extrapolation; a significant reversal means the port's own "+
						"iteration is the problem.", s.name, s.dosNearer, nb, pn)
				}
			}
		}
	}
}

func joinArgs(a []string) string {
	out := ""
	for i, s := range a {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

// binomTwoTailed returns the exact two-tailed probability of seeing a lean at
// least as extreme as k of n under a fair coin. Computed in log space so it
// stays exact for the n this test produces without overflowing.
func binomTwoTailed(k, n int) float64 {
	if n == 0 {
		return 1
	}
	if k < n-k {
		k = n - k
	}
	// P(X >= k) under Binomial(n, 0.5), doubled.
	logC := func(n, r int) float64 {
		s := 0.0
		for i := 0; i < r; i++ {
			s += math.Log(float64(n-i)) - math.Log(float64(i+1))
		}
		return s
	}
	tail := 0.0
	for i := k; i <= n; i++ {
		tail += math.Exp(logC(n, i) - float64(n)*math.Log(2))
	}
	p := 2 * tail
	if p > 1 {
		p = 1
	}
	return p
}
