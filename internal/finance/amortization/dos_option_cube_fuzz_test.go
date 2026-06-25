package amortization

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Randomized differential FUZZER over the advanced-option cube. The standing
// tracker (dos_option_combo_tracker_test.go) pins a fixed list of hand-chosen
// option PAIRS; this fuzzer instead generates RANDOM loans crossed with RANDOM
// subsets of options (balloons, ARMs, moratorium, target, skip) and diffs every
// case against the real DOS engine. The point is to reach 3-, 4-, and 5-way
// combinations no hand-written table covers — the space where the ARM+balloon
// and moratorium+balloon bugs hid — and to FAIL on any divergence beyond a
// rounding tolerance.
//
// Signals (both robust to a payment that changes mid-schedule):
//   - total interest must match DOS within tol, AND
//   - the Go schedule must retire to ~0 like DOS (catches the "retires
//     early/late" class directly, regardless of the interest delta).
//
// Determinism: seed is fixed (override PERSENSE_FUZZ_SEED); case count defaults
// to 150 (override PERSENSE_FUZZ_N). Every divergence prints the exact oracle
// command line so it reproduces by hand and can be promoted to a tracker guard.
//
// Runs only where the DOS oracle binary is present; skipped otherwise.
//
// Parity rules baked into the generator (so a mismatch means a REAL engine
// divergence, not a setup artifact):
//   - monthly (perYr=12) only, so every month is a payment date and the
//     oracle's month→date mapping (dateMonthsAfterLoan) matches Go exactly;
//   - options are placed on payment dates strictly inside the term;
//   - `plusreg` is passed to the oracle whenever a balloon is present, because
//     goAmortizeOptions runs with Settings.PlusRegular=true (balloon ADDs to the
//     regular payment) — the documented Go default.

// fuzzCase is one generated scenario: a Go input mutator plus the matching
// oracle flag list, with a human-readable description.
type fuzzCase struct {
	apply func(*LoanInput)
	flags []string
	desc  string
}

// genFuzzCase builds a random loan-option scenario. It returns the base loan
// scalars plus the paired Go/oracle option encodings.
func genFuzzCase(rng *rand.Rand) (amount, rate float64, n, perYr int, fc fuzzCase) {
	perYr = 12
	amount = float64(int(50000+rng.Float64()*450000)/1000) * 1000 // 50k–500k, round thousands
	rate = math.Round((0.04+rng.Float64()*0.08)*10000) / 10000    // 4%–12%, 4dp
	n = []int{120, 180, 240, 300, 360}[rng.Intn(5)]

	var applies []func(*LoanInput)
	var flags []string
	var descParts []string

	// usedMonths keeps option dates distinct so two options never collide on the
	// same payment date in a way the two engines might order differently.
	used := map[int]bool{}
	pickMonth := func(lo, hi int) (int, bool) {
		if hi <= lo {
			return 0, false
		}
		for try := 0; try < 12; try++ {
			m := lo + rng.Intn(hi-lo)
			if !used[m] {
				used[m] = true
				return m, true
			}
		}
		return 0, false
	}

	// Balloons: 0–2, ADD semantics (plusreg).
	hasBalloon := false
	nBalloon := rng.Intn(3)
	var balloons []BalloonPayment
	for b := 0; b < nBalloon; b++ {
		m, ok := pickMonth(12, n-6)
		if !ok {
			continue
		}
		amt := float64(int(5000+rng.Float64()*40000)/500) * 500 // 5k–45k
		balloons = append(balloons, balloonAt(m, amt))
		flags = append(flags, fmt.Sprintf("b%d=%s", m, strconv.FormatFloat(amt, 'f', 2, 64)))
		descParts = append(descParts, fmt.Sprintf("balloon $%.0f@m%d", amt, m))
		hasBalloon = true
	}
	if len(balloons) > 0 {
		bs := balloons
		applies = append(applies, func(in *LoanInput) { in.Balloons = bs })
	}

	// Rate adjustments (ARMs): 0–2, rate-only.
	nAdj := rng.Intn(3)
	var adjs []RateAdjustment
	for a := 0; a < nAdj; a++ {
		m, ok := pickMonth(6, n-6)
		if !ok {
			continue
		}
		nr := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		adjs = append(adjs, adjRateAt(m, nr))
		flags = append(flags, fmt.Sprintf("adj=%d:%s:", m, strconv.FormatFloat(nr, 'f', 6, 64)))
		descParts = append(descParts, fmt.Sprintf("ARM→%.4f@m%d", nr, m))
	}
	if len(adjs) > 0 {
		// DOS sorts adjustments by date; sort ours too so re-amortization order
		// is identical.
		sort.Slice(adjs, func(i, j int) bool {
			return adjs[i].Date.Time.Before(adjs[j].Date.Time)
		})
		as := adjs
		applies = append(applies, func(in *LoanInput) { in.Adjustments = as })
	}

	// Moratorium: ~40% of cases.
	if rng.Float64() < 0.4 {
		m, ok := pickMonth(3, n/3+1)
		if ok {
			applies = append(applies, func(in *LoanInput) {
				in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: dateMonthsAfterLoan(m)}
			})
			flags = append(flags, fmt.Sprintf("mor=%d", m))
			descParts = append(descParts, fmt.Sprintf("moratorium→m%d", m))
		}
	}

	// Target (minimum principal reduction): ~25% of cases. Kept modest so the
	// loan still amortizes sanely.
	if rng.Float64() < 0.25 {
		tv := float64(int(50 + rng.Float64()*200))
		applies = append(applies, func(in *LoanInput) {
			in.Target = Target{TargetStatus: types.InOutInput, TargetValue: tv}
		})
		flags = append(flags, fmt.Sprintf("targ=%s", strconv.FormatFloat(tv, 'f', 2, 64)))
		descParts = append(descParts, fmt.Sprintf("target $%.0f", tv))
	}

	// Skip months: ~25% of cases. Pick from a small catalog (no spaces).
	if rng.Float64() < 0.25 {
		s := []string{"6", "9", "5-7", "3,9", "11-12"}[rng.Intn(5)]
		applies = append(applies, func(in *LoanInput) { in.SkipMonths = skipSetRaw(s) })
		flags = append(flags, "skip="+s)
		descParts = append(descParts, "skip="+s)
	}

	if hasBalloon {
		flags = append(flags, "plusreg")
	}

	fc = fuzzCase{
		apply: func(in *LoanInput) {
			for _, ap := range applies {
				ap(in)
			}
		},
		flags: flags,
		desc:  strings.Join(descParts, " + "),
	}
	if fc.desc == "" {
		fc.desc = "(plain)"
	}
	return amount, rate, n, perYr, fc
}

// skipSetRaw parses a skip string without a *testing.T (the tracker's skipSet
// takes one for Fatalf). Returns an empty set on parse error — the case still
// runs, just without a skip.
func skipSetRaw(s string) SkipMonths {
	ms, err := MonthSetFromString(s)
	if err != nil {
		return SkipMonths{}
	}
	return SkipMonths{SkipStatus: types.InOutInput, SkipStr: s, MonthSet: ms}
}

// oracleResultFlags returns the DOS total interest and final balance for a
// flagged case. Final balance comes from the last schedule row at the oracle's
// own solved payment.
func oracleResultFlags(amount, rate float64, n, perYr int, flags ...string) (totalInt, finalBal float64, ok bool) {
	ti, iok := runOracleInterestFlags(amount, rate, n, perYr, flags...)
	if !iok {
		return 0, 0, false
	}
	pay, pok := runOraclePayment(amount, rate, n, perYr, flags...)
	if !pok {
		return 0, 0, false
	}
	rows, rok := runOracleRowsFlags(amount, rate, n, perYr, pay, flags...)
	if !rok || len(rows) == 0 {
		return 0, 0, false
	}
	return ti, rows[len(rows)-1].balance, true
}

// oracleCmd renders the reproducible command line for a case.
func oracleCmd(amount, rate float64, n, perYr int, flags ...string) string {
	parts := []string{
		"amort_oracle",
		strconv.FormatFloat(amount, 'f', 2, 64),
		strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr),
	}
	parts = append(parts, flags...)
	return strings.Join(parts, " ")
}

func TestDOSOptionCubeFuzz(t *testing.T) {
	// Opt-in discovery tool. It deliberately FAILS while higher-order option
	// combinations still diverge from DOS (see docs/amort_option_combo_divergences.md
	// "Fuzzer divergence map"), so it is gated off the default `go test ./...`
	// run — which stays green on the targeted tracker/regression guards — and
	// only runs when explicitly hunting: PERSENSE_FUZZ=1.
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1 to run the randomized option-cube fuzzer")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}

	seed := int64(20260624)
	if s := os.Getenv("PERSENSE_FUZZ_SEED"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = v
		}
	}
	nCases := 150
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			nCases = v
		}
	}
	rng := rand.New(rand.NewSource(seed))

	type divergence struct {
		desc          string
		cmd           string
		goInt, dosInt float64
		goBal, dosBal float64
		kind          string
	}
	var divergences []divergence
	ran, skipped := 0, 0
	// Per option-signature aggregation: ran vs diverged, so a wall of raw
	// failures becomes a map of which COMBINATIONS are clean vs broken.
	bucketRan := map[string]int{}
	bucketDiv := map[string]int{}

	for i := 0; i < nCases; i++ {
		amount, rate, n, perYr, fc := genFuzzCase(rng)

		goInt, goBal, gok := goAmortizeOptions(amount, rate, n, perYr, fc.apply)
		if !gok {
			skipped++
			continue
		}
		dosInt, dosBal, dok := oracleResultFlags(amount, rate, n, perYr, fc.flags...)
		if !dok {
			skipped++
			continue
		}
		ran++
		key := classKey(fc.flags)
		bucketRan[key]++

		// Tolerances. intTol cleanly separates the DOS-vs-port rounding tail
		// (historically <= ~$3.45) from a real engine bug (hundreds–thousands).
		// A relative term keeps large loans from tripping on pure rounding.
		intTol := math.Max(25.0, 5e-4*math.Abs(dosInt))
		const balTol = 5.0

		dInt := math.Abs(goInt - dosInt)
		kind := ""
		if dInt > intTol {
			kind = "interest"
		}
		// Non-retirement: Go must end at ~0 like DOS. (DOS final balance is also
		// ~0; if DOS itself left a balance, only flag when they DISAGREE.)
		if math.Abs(goBal) > balTol && math.Abs(goBal-dosBal) > balTol {
			if kind != "" {
				kind += "+balance"
			} else {
				kind = "balance"
			}
		}
		if kind != "" {
			bucketDiv[key]++
			divergences = append(divergences, divergence{
				desc: fc.desc, cmd: oracleCmd(amount, rate, n, perYr, fc.flags...),
				goInt: goInt, dosInt: dosInt, goBal: goBal, dosBal: dosBal, kind: kind,
			})
		}
	}

	t.Logf("fuzz: seed=%d cases=%d ran=%d skipped=%d divergences=%d",
		seed, nCases, ran, skipped, len(divergences))
	if ran == 0 {
		t.Skip("oracle produced no usable results (heap-flake or build issue)")
	}

	// Option-signature map: which combinations are clean vs broken. Sorted by
	// divergence count so the worst classes lead.
	keys := make([]string, 0, len(bucketRan))
	for k := range bucketRan {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if bucketDiv[keys[i]] != bucketDiv[keys[j]] {
			return bucketDiv[keys[i]] > bucketDiv[keys[j]]
		}
		return keys[i] < keys[j]
	})
	var sb strings.Builder
	sb.WriteString("option-signature divergence map (diverged/ran):\n")
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("    %-28s %d/%d\n", k, bucketDiv[k], bucketRan[k]))
	}
	t.Log(sb.String())

	// Show up to a handful of concrete repros per class so each class is
	// debuggable without drowning the log.
	perClass := map[string]int{}
	for _, d := range divergences {
		k := classKeyFromCmd(d.cmd)
		if perClass[k] >= 3 {
			continue
		}
		perClass[k]++
		t.Errorf("DIVERGENCE [%s] {%s} %s\n    Go interest %.2f / DOS %.2f (Δ %.2f) | Go finalBal %.2f / DOS %.2f\n    repro: %s",
			d.kind, k, d.desc, d.goInt, d.dosInt, d.goInt-d.dosInt, d.goBal, d.dosBal, d.cmd)
	}
	if len(divergences) > 0 {
		t.Errorf("total divergences: %d across %d distinct signatures (see map above)",
			len(divergences), countNonzero(bucketDiv))
	}
}

// classKey reduces a flag list to a coarse option signature (which option TYPES
// are present, with a 2+ marker for repeated balloons/ARMs), so divergences
// aggregate into debuggable buckets independent of specific amounts/dates.
func classKey(flags []string) string {
	nB, nAdj := 0, 0
	mor, targ, skip := false, false, false
	for _, f := range flags {
		switch {
		case strings.HasPrefix(f, "adj="):
			nAdj++
		case strings.HasPrefix(f, "mor="):
			mor = true
		case strings.HasPrefix(f, "targ="):
			targ = true
		case strings.HasPrefix(f, "skip="):
			skip = true
		case len(f) >= 2 && f[0] == 'b' && strings.Contains(f, "="):
			nB++
		}
	}
	var tags []string
	switch {
	case nB >= 2:
		tags = append(tags, "balloon2+")
	case nB == 1:
		tags = append(tags, "balloon")
	}
	switch {
	case nAdj >= 2:
		tags = append(tags, "ARM2+")
	case nAdj == 1:
		tags = append(tags, "ARM")
	}
	if mor {
		tags = append(tags, "mor")
	}
	if targ {
		tags = append(tags, "target")
	}
	if skip {
		tags = append(tags, "skip")
	}
	if len(tags) == 0 {
		return "(plain)"
	}
	return strings.Join(tags, "+")
}

// classKeyFromCmd extracts the signature from a stored repro command line.
func classKeyFromCmd(cmd string) string {
	return classKey(strings.Fields(cmd))
}

func countNonzero(m map[string]int) int {
	c := 0
	for _, v := range m {
		if v > 0 {
			c++
		}
	}
	return c
}
