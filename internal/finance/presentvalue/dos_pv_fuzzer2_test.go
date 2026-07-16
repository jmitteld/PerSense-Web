package presentvalue

import (
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

// TestDOSPVFuzzer2 is a rotate-the-unknown differential fuzzer for the LUMP
// present-value worksheet, mirroring the amortization TestDOSFuzzer2. Each case:
//   - solves the Sum Value (present value) from a random amount/rate/term (the
//     initial forward solve),
//   - then TWICE hardens the just-solved DOS-canonical output and clears a
//     DIFFERENT field, solving for it instead.
//
// The rotatable unknowns are {sumvalue, amount, rate} with the horizon (months)
// held fixed. Every solve is checked against the real DOS PRESVALU engine, and
// after each harden the Go forward PV is recomputed from the now-determined
// {amount, rate, months} and must reproduce the chain's Sum Value — so the
// forward evaluator and the two backward solvers (bk_lump_amt, bk_rate) are
// proven mutually consistent and DOS-faithful when CHAINED, not merely
// individually (which the per-solver differentials in dos_pv_oracle_test.go and
// dos_pv_backward_boundary_test.go already cover).
//
// Because each step hardens the DOS-canonical value and feeds it IDENTICALLY to
// both engines (the oracle prints pv:0:6 / amt:0:6 / rate:0:10, so no precision
// is lost), any residual is a true Go-vs-DOS differential, not a rounding
// artifact — the same quantize-then-compare discipline as the amort fuzzer2.
//
// Seed is fixed (reproducible) but overridable with PERSENSE_FUZZ_SEED; the case
// count is 60, overridable with FUZZER2_CASES.
func TestDOSPVFuzzer2(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s); build via TARGET=pv_oracle legacy/oracle/build_linux.sh", pvOracleBin())
	}

	seed := int64(20260715)
	if s := os.Getenv("PERSENSE_FUZZ_SEED"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = v
		}
	}
	rng := rand.New(rand.NewSource(seed))

	// The pv_oracle is occasionally flaky under load; retry so a transient exec
	// hiccup is not mistaken for a refusal. A genuine ERR is deterministic and
	// survives the retries (caught by the one-sided check below).
	retry := func(f func() (float64, bool)) (float64, bool) {
		for try := 0; try < 6; try++ {
			if v, ok := f(); ok {
				return v, true
			}
		}
		return 0, false
	}

	type vals map[string]float64

	goSolve := func(v vals, months int, unknown string) (float64, bool) {
		switch unknown {
		case "sumvalue":
			return goPVLump(v["amount"], v["rate"], months), true
		case "amount":
			return goBkLumpAmount(v["sumvalue"], v["rate"], months)
		case "rate":
			return goBkRate(v["sumvalue"], v["amount"], months)
		}
		return 0, false
	}
	dosSolve := func(v vals, months int, unknown string) (float64, bool) {
		switch unknown {
		case "sumvalue":
			return retry(func() (float64, bool) { return runPVLumpOracle(v["amount"], v["rate"], months) })
		case "amount":
			return retry(func() (float64, bool) { return runBkLumpAmtOracle(v["sumvalue"], v["rate"], months) })
		case "rate":
			return retry(func() (float64, bool) { return runBkRateOracle(v["sumvalue"], v["amount"], months) })
		}
		return 0, false
	}

	// Sum Value and amount are well-conditioned closed forms (relative tol);
	// the rate is recovered by Newton and printed to 10dp (absolute tol).
	tolOf := func(field string, dosVal float64) float64 {
		switch field {
		case "sumvalue":
			return math.Max(5e-4, 1e-6*math.Abs(dosVal))
		case "amount":
			return math.Max(5e-4, 5e-6*math.Abs(dosVal))
		case "rate":
			return 1e-6
		}
		return 1e-6
	}

	fields := []string{"sumvalue", "amount", "rate"}

	nSeeds := 60
	if v := os.Getenv("FUZZER2_CASES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			nSeeds = n
		}
	}

	cases, flaked, oneSided := 0, 0, 0
	for c := 0; c < nSeeds; c++ {
		// Well-conditioned regime: positive amount, moderate rate, 1..~40yr, so
		// BOTH engines reliably solve every rotation. Quantize the rate to 6dp so
		// the oracle (which formats to 10dp) and Go receive an identical value.
		amount := math.Round((1000+rng.Float64()*999000)*100) / 100
		rate := math.Round((0.01+rng.Float64()*0.29)*1e6) / 1e6
		months := 12 + rng.Intn(469) // ~1..40 years
		v := vals{"amount": amount, "rate": rate, "sumvalue": 0}

		unknown := "sumvalue"
		prev := ""
		steps := 0
		aborted := false
		for s := 0; s < 3; s++ {
			goVal, gok := goSolve(v, months, unknown)
			dosVal, dok := dosSolve(v, months, unknown)

			if !dok {
				// Oracle declined after retries. In this well-conditioned regime a
				// genuine refusal is unexpected; if Go solved, that is a real
				// one-sided divergence (flagged below). Either way, end this chain.
				if gok {
					oneSided++
				}
				flaked++
				aborted = true
				break
			}
			if !gok {
				// DOS solved but Go could not — a real one-sided divergence.
				t.Errorf("case %d step %d solve %s: Go FAILED but DOS solved=%.10g (amount=%.2f rate=%.6f months=%d sumvalue=%.6f)",
					c, s, unknown, dosVal, v["amount"], v["rate"], months, v["sumvalue"])
				aborted = true
				break
			}

			tv := tolOf(unknown, dosVal)
			if d := goVal - dosVal; d > tv || d < -tv {
				t.Errorf("case %d step %d solve %s: Go=%.10g DOS=%.10g (Δ=%+.3g, tol=%.3g) [amount=%.2f rate=%.6f months=%d sumvalue=%.6f]",
					c, s, unknown, goVal, dosVal, d, tv, v["amount"], v["rate"], months, v["sumvalue"])
			}

			// Harden the DOS-canonical solved value into the chain.
			v[unknown] = dosVal

			// Intermediary self-consistency: with {amount, rate, months} now all
			// determined, the Go forward PV must reproduce the chain's Sum Value —
			// proving the backward-solved field actually inverts the forward map.
			if sv := v["sumvalue"]; sv > 0 {
				fwd := goPVLump(v["amount"], v["rate"], months)
				itol := math.Max(1e-2, 1e-5*math.Abs(sv))
				if d := fwd - sv; d > itol || d < -itol {
					t.Errorf("case %d step %d after %s: forward PV=%.6f != Sum Value=%.6f (Δ=%.6f)", c, s, unknown, fwd, sv, d)
				}
			}
			steps++

			// Rotate to a field different from the one just solved.
			prev = unknown
			for {
				cand := fields[rng.Intn(len(fields))]
				if cand != prev {
					unknown = cand
					break
				}
			}
		}
		if !aborted && steps == 3 {
			cases++
		}
	}

	if oneSided > 0 {
		t.Errorf("%d one-sided solves (Go solved but the DOS oracle declined in the well-conditioned regime) — investigate", oneSided)
	}
	if cases < 10 {
		t.Fatalf("only %d complete PV fuzz chains ran (want >=10) — oracle flaky or draws unsolvable", cases)
	}
	t.Logf("completed %d full 3-step PV chains; %d chains ended early on oracle decline/flake", cases, flaked)
}
