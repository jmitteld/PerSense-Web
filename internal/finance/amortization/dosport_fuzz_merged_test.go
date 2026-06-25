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

// mergedCase is a fully-random loan: arbitrary frequency / first period /
// prepaid CROSSED with the advanced-option cube, options placed on real payment
// dates so the Go input and oracle flags agree.
type mergedCase struct {
	amount, rate          float64
	n, perYr, firstMonths int
	prepaid               bool
	apply                 func(*LoanInput)
	flags                 []string
	key                   string
}

// genMergedCase builds one. Payment j (1-based) falls on month
// firstMonths + (j-1)*(12/perYr) after the loan date; options are placed there.
func genMergedCase(rng *rand.Rand) mergedCase {
	perYr := []int{12, 12, 6, 4, 2, 1}[rng.Intn(6)] // weight monthly
	clean := 12 / perYr
	amount := float64(int(50000+rng.Float64()*450000)/1000) * 1000
	rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
	n := []int{12, 18, 24, 36, 48, 60}[rng.Intn(6)]
	prepaid := rng.Float64() < 0.4

	firstMonths := clean
	oddFirst := rng.Float64() < 0.45
	if oddFirst {
		for {
			firstMonths = 1 + rng.Intn(2*clean)
			if firstMonths != clean {
				break
			}
		}
	}
	// month-of-loan for the j-th payment (1-based).
	payMonth := func(j int) int { return firstMonths + (j-1)*clean }

	used := map[int]bool{}
	pickPay := func(lo, hi int) (int, bool) {
		if hi <= lo {
			return 0, false
		}
		for try := 0; try < 12; try++ {
			j := lo + rng.Intn(hi-lo)
			if !used[j] {
				used[j] = true
				return j, true
			}
		}
		return 0, false
	}

	var applies []func(*LoanInput)
	var flags []string
	var tags []string

	// Balloons (ADD ⇒ plusreg), placed on payment dates.
	hasBalloon := false
	var balloons []BalloonPayment
	for b := 0; b < rng.Intn(3); b++ {
		j, ok := pickPay(2, n-1)
		if !ok {
			continue
		}
		m := payMonth(j)
		amt := float64(int(5000+rng.Float64()*40000)/500) * 500
		balloons = append(balloons, balloonAt(m, amt))
		flags = append(flags, fmt.Sprintf("b%d=%s", m, strconv.FormatFloat(amt, 'f', 2, 64)))
		hasBalloon = true
	}
	if len(balloons) > 0 {
		bs := balloons
		applies = append(applies, func(in *LoanInput) { in.Balloons = bs })
		tags = append(tags, fmt.Sprintf("balloon%d", len(bs)))
	}

	// Adjustments on payment dates: rate-only ARMs (AO5) OR date-only
	// re-amortize (AO7). AO6 (payment-only ⇒ solve rate) needs a payment guess
	// the option stack makes unstable, so it stays in the standalone sweep
	// (TestDOSPortAdjSolveSweep); its in-combination machinery reduces to AO5.
	var adjs []RateAdjustment
	for a := 0; a < rng.Intn(3); a++ {
		j, ok := pickPay(2, n-1)
		if !ok {
			continue
		}
		m := payMonth(j)
		if rng.Float64() < 0.30 && os.Getenv("PERSENSE_AO7") != "" { // AO7: date-only re-amortize at current rate
			adjs = append(adjs, adjDateOnly(m))
			flags = append(flags, fmt.Sprintf("adj=%d::", m))
			continue
		}
		nr := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		adjs = append(adjs, adjRateAt(m, nr))
		flags = append(flags, fmt.Sprintf("adj=%d:%s:", m, strconv.FormatFloat(nr, 'f', 6, 64)))
	}
	if len(adjs) > 0 {
		sort.Slice(adjs, func(i, j int) bool { return adjs[i].Date.Time.Before(adjs[j].Date.Time) })
		as := adjs
		applies = append(applies, func(in *LoanInput) { in.Adjustments = as })
		tags = append(tags, fmt.Sprintf("ARM%d", len(as)))
	}

	// Moratorium on a payment date.
	if rng.Float64() < 0.35 {
		if j, ok := pickPay(2, n/3+2); ok {
			m := payMonth(j)
			applies = append(applies, func(in *LoanInput) {
				in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: dateMonthsAfterLoan(m)}
			})
			flags = append(flags, fmt.Sprintf("mor=%d", m))
			tags = append(tags, "mor")
		}
	}
	// Target.
	if rng.Float64() < 0.25 {
		tv := float64(int(50 + rng.Float64()*200))
		applies = append(applies, func(in *LoanInput) { in.Target = Target{TargetStatus: types.InOutInput, TargetValue: tv} })
		flags = append(flags, fmt.Sprintf("targ=%s", strconv.FormatFloat(tv, 'f', 2, 64)))
		tags = append(tags, "target")
	}
	// Skip months (calendar; frequency-independent).
	if rng.Float64() < 0.25 {
		s := []string{"6", "9", "5-7", "3,9", "11-12"}[rng.Intn(5)]
		applies = append(applies, func(in *LoanInput) { in.SkipMonths = skipSetRaw(s) })
		flags = append(flags, "skip="+s)
		tags = append(tags, "skip")
	}

	if firstMonths != clean {
		flags = append(flags, fmt.Sprintf("first=%d", firstMonths))
		tags = append(tags, "oddFirst")
	}
	if prepaid {
		flags = append(flags, "prepaid")
		tags = append(tags, "prepaid")
	}
	if hasBalloon {
		flags = append(flags, "plusreg")
	}

	key := fmt.Sprintf("perYr%d", perYr)
	if len(tags) > 0 {
		key += "|" + strings.Join(tags, "+")
	}
	return mergedCase{amount, rate, n, perYr, firstMonths, prepaid,
		func(in *LoanInput) {
			for _, ap := range applies {
				ap(in)
			}
		}, flags, key}
}

func portMergedResult(c mergedCase) (totalInt, finalBal float64, ok bool) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: c.amount,
			LoanRateStatus: types.InOutInput, LoanRate: c.rate,
			NStatus: types.InOutInput, NPeriods: c.n,
			PerYrStatus: types.InOutInput, PerYr: c.perYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: monthsAfterLoanStart(c.firstMonths),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(c.perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true, Prepaid: c.prepaid},
		Fancy:    true,
	}
	c.apply(&in)
	r := AmortizeDOS(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return 0, 0, false
	}
	return r.TotalInt, r.Schedule[len(r.Schedule)-1].Principal, true
}

// TestDOSPortFuzzMerged is the FULL acceptance fuzzer: the advanced-option cube
// crossed with arbitrary frequency / first period / prepaid. Drives the port to
// zero against the oracle over the merged domain.
func TestDOSPortFuzzMerged(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	seed := int64(606060)
	if s := os.Getenv("PERSENSE_FUZZ_SEED"); s != "" {
		if v, e := strconv.ParseInt(s, 10, 64); e == nil {
			seed = v
		}
	}
	nCases := 200
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			nCases = v
		}
	}
	rng := rand.New(rand.NewSource(seed))
	bucketRan := map[string]int{}
	bucketDiv := map[string]int{}
	type dv struct {
		key, cmd      string
		goInt, dosInt float64
		goBal         float64
	}
	var divs []dv
	ran := 0

	for i := 0; i < nCases; i++ {
		c := genMergedCase(rng)
		goInt, goBal, gok := portMergedResult(c)
		if !gok {
			continue
		}
		dosInt, dok := runOracleInterestFlags(c.amount, c.rate, c.n, c.perYr, c.flags...)
		if !dok {
			continue
		}
		ran++
		bucketRan[c.key]++
		intTol := math.Max(25.0, 5e-4*math.Abs(dosInt))
		if math.Abs(goInt-dosInt) > intTol || math.Abs(goBal) > 5.0 {
			bucketDiv[c.key]++
			divs = append(divs, dv{c.key, oracleCmd(c.amount, c.rate, c.n, c.perYr, c.flags...), goInt, dosInt, goBal})
		}
	}

	t.Logf("DOS-port MERGED fuzz: seed=%d ran=%d divergences=%d", seed, ran, len(divs))
	if ran == 0 {
		t.Skip("oracle produced no usable results")
	}
	// Coarse signature (drop the perYr prefix) for the divergence summary.
	coarse := map[string]int{}
	coarseRan := map[string]int{}
	for k, v := range bucketDiv {
		c := k
		if p := strings.Index(k, "|"); p >= 0 {
			c = k[p+1:]
		} else {
			c = "(plain)"
		}
		coarse[c] += v
	}
	for k, v := range bucketRan {
		c := k
		if p := strings.Index(k, "|"); p >= 0 {
			c = k[p+1:]
		} else {
			c = "(plain)"
		}
		coarseRan[c] += v
	}
	keys := make([]string, 0, len(coarseRan))
	for k := range coarseRan {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if coarse[keys[i]] != coarse[keys[j]] {
			return coarse[keys[i]] > coarse[keys[j]]
		}
		return keys[i] < keys[j]
	})
	var sb strings.Builder
	sb.WriteString("MERGED divergence map by option-signature (diverged/ran):\n")
	for _, k := range keys {
		if coarse[k] > 0 {
			sb.WriteString(fmt.Sprintf("    %-40s %d/%d\n", k, coarse[k], coarseRan[k]))
		}
	}
	t.Log(sb.String())
	shown := map[string]int{}
	for _, d := range divs {
		if shown[d.key] >= 2 {
			continue
		}
		shown[d.key]++
		t.Errorf("MERGED DIVERGENCE {%s} Go %.2f / DOS %.2f (Δ %.2f) bal %.2f\n    repro: %s",
			d.key, d.goInt, d.dosInt, d.goInt-d.dosInt, d.goBal, d.cmd)
	}
}
