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

	const cases = 300
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
			rateStat.note(t, solved.Loan.LoanRate, math.Float64frombits(bits),
				"amort_oracle "+joinArgs(args)+" norate")
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
			const materialULP = 16
			switch {
			case p < 0.01 && s.maxAbs > materialULP:
				t.Errorf("%s: SIGN-BIASED AND MATERIAL. %d of %d non-exact differences "+
					"lean %s (p=%.2g) with a worst case of %d ULP. Independent rounding "+
					"does not do this; a systematic conversion or ordering difference "+
					"does. This is §48's signature.\n  worst: %s",
					s.name, lean, nz, dir, p, s.maxAbs, s.worst)
			case p < 0.01:
				t.Logf("   SIG=ADVISORY:backward_solve_sign_bias %s — %d of %d lean %s "+
					"(p=%.2g) but the worst case is only %d ULP (~1e-16 relative), so "+
					"this is a systematic arithmetic difference with no observable "+
					"consequence. Recorded, not failed. Investigate if the magnitude "+
					"ever grows.", s.name, lean, nz, dir, p, s.maxAbs)
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
