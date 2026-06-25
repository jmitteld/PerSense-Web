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

// portGivenPayResult runs AmortizeDOS with a USER-GIVEN payment (hardPay=true ⇒
// per-period interest Round2'd) for a plain loan with explicit frequency / first
// period / prepaid.
func portGivenPayResult(amount, rate, pay float64, n, perYr, firstMonths int, prepaid bool) (totalInt, finalBal float64, ok bool) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus: types.InOutInput, PayAmt: pay, // GIVEN (hard)
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: monthsAfterLoanStart(firstMonths),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true, Prepaid: prepaid},
		Fancy:    true,
	}
	r := AmortizeDOS(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return 0, 0, false
	}
	return r.TotalInt, r.Schedule[len(r.Schedule)-1].Principal, true
}

// TestDOSPortFuzzGivenPay validates the GIVEN-payment (hard_payment) path: a
// user-entered payment rounds each period's interest to cents (DOS Round2). It
// solves the payment, rounds it to cents (as a user would enter), then feeds it
// HARD to both the port and the oracle (payhard= token) and diffs. Plain loans
// over frequency / first period / prepaid, to isolate Round2 propagation. Opt-in.
func TestDOSPortFuzzGivenPay(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	seed := int64(717171)
	if s := os.Getenv("PERSENSE_FUZZ_SEED"); s != "" {
		if v, e := strconv.ParseInt(s, 10, 64); e == nil {
			seed = v
		}
	}
	nCases := 150
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			nCases = v
		}
	}
	rng := rand.New(rand.NewSource(seed))
	perYrs := []int{12, 6, 4, 2, 1}
	bucketRan := map[string]int{}
	bucketDiv := map[string]int{}
	var divs []gpDiv
	ran := 0

	for i := 0; i < nCases; i++ {
		perYr := perYrs[rng.Intn(len(perYrs))]
		clean := 12 / perYr
		amount := float64(int(50000+rng.Float64()*450000)/1000) * 1000
		rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		n := []int{12, 18, 24, 36, 48}[rng.Intn(5)]
		prepaid := rng.Float64() < 0.5
		firstMonths := clean
		oddFirst := rng.Float64() < 0.5
		if oddFirst {
			for {
				firstMonths = 1 + rng.Intn(2*clean)
				if firstMonths != clean {
					break
				}
			}
		}
		var baseFlags []string
		if firstMonths != clean {
			baseFlags = append(baseFlags, fmt.Sprintf("first=%d", firstMonths))
		}
		if prepaid {
			baseFlags = append(baseFlags, "prepaid")
		}
		// Solve the payment, then round to cents (a realistic user-entered value).
		solved, ok := runOraclePayment(amount, rate, n, perYr, baseFlags...)
		if !ok || solved <= 0 {
			continue
		}
		pay := math.Round(solved*100) / 100

		goInt, goBal, gok := portGivenPayResult(amount, rate, pay, n, perYr, firstMonths, prepaid)
		if !gok {
			continue
		}
		hardFlags := append([]string{fmt.Sprintf("payhard=%s", strconv.FormatFloat(pay, 'f', 2, 64))}, baseFlags...)
		dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, hardFlags...)
		if !dok {
			continue
		}
		ran++
		key := fmt.Sprintf("perYr%d", perYr)
		if oddFirst {
			key += "+oddFirst"
		}
		if prepaid {
			key += "+prepaid"
		}
		bucketRan[key]++
		// Hard-payment tolerance: per-period cent rounding accumulates a few cents
		// over a long schedule; allow $0.10 + a tiny relative term. Anything beyond
		// is a real Round2 propagation divergence.
		intTol := math.Max(0.10, 1e-5*math.Abs(dosInt))
		if math.Abs(goInt-dosInt) > intTol || math.Abs(goBal) > 5.0 {
			bucketDiv[key]++
			divs = append(divs, gpDiv{key, oracleCmd(amount, rate, n, perYr, hardFlags...), goInt, dosInt, goBal})
		}
	}

	reportGivenPay(t, "GIVEN-PAY", seed, ran, bucketRan, bucketDiv, divs)
}

type gpDiv = struct {
	key, cmd      string
	goInt, dosInt float64
	goBal         float64
}

func reportGivenPay(t *testing.T, label string, seed int64, ran int, bucketRan, bucketDiv map[string]int, divs []gpDiv) {
	t.Helper()
	t.Logf("DOS-port %s fuzz: seed=%d ran=%d divergences=%d", label, seed, ran, len(divs))
	if ran == 0 {
		t.Skip("oracle produced no usable results")
	}
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
	sb.WriteString("GIVEN-PAY divergence map (diverged/ran):\n")
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("    %-26s %d/%d\n", k, bucketDiv[k], bucketRan[k]))
	}
	t.Log(sb.String())
	shown := map[string]int{}
	for _, d := range divs {
		if shown[d.key] >= 2 {
			continue
		}
		shown[d.key]++
		t.Errorf("%s DIVERGENCE {%s} Go %.4f / DOS %.4f (Δ %.4f) bal %.2f\n    repro: %s",
			label, d.key, d.goInt, d.dosInt, d.goInt-d.dosInt, d.goBal, d.cmd)
	}
}

// TestDOSPortFuzzGivenPayMerged: GIVEN hard payment CROSSED with the full option
// cube (the cutover's per-row failures were given-payment balloon loans). Solve
// the base payment, round to cents, feed it hard to both engines + the same
// options. Drives the port's hard-payment path to zero over the merged domain.
func TestDOSPortFuzzGivenPayMerged(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	seed := int64(818181)
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
	var divs []gpDiv
	ran := 0

	for i := 0; i < nCases; i++ {
		c := genMergedCase(rng)
		// Base payment solved by the oracle WITH the options, rounded to cents.
		solved, ok := runOraclePayment(c.amount, c.rate, c.n, c.perYr, c.flags...)
		if !ok || solved <= 0 {
			continue
		}
		pay := math.Round(solved*100) / 100

		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: c.amount,
				LoanRateStatus: types.InOutInput, LoanRate: c.rate,
				NStatus: types.InOutInput, NPeriods: c.n,
				PerYrStatus: types.InOutInput, PerYr: c.perYr,
				PayAmtStatus: types.InOutInput, PayAmt: pay, // GIVEN hard
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
				FirstStatus: types.InOutInput, FirstDate: monthsAfterLoanStart(c.firstMonths),
			},
			Settings: Settings{Basis: types.Basis360, PerYr: byte(c.perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true, Prepaid: c.prepaid},
			Fancy:    true,
		}
		c.apply(&in)
		r := AmortizeDOS(in)
		if r.Err != nil || len(r.Schedule) == 0 {
			continue
		}
		goInt, goBal := r.TotalInt, r.Schedule[len(r.Schedule)-1].Principal

		// Oracle: the option flags with `pay=` replaced by `payhard=`.
		hardFlags := make([]string, 0, len(c.flags)+1)
		for _, f := range c.flags {
			hardFlags = append(hardFlags, f)
		}
		hardFlags = append(hardFlags, fmt.Sprintf("payhard=%s", strconv.FormatFloat(pay, 'f', 2, 64)))
		dosInt, dok := runOracleInterestFlags(c.amount, c.rate, c.n, c.perYr, hardFlags...)
		if !dok {
			continue
		}
		ran++
		ck := c.key
		if p := strings.Index(ck, "|"); p >= 0 {
			ck = ck[p+1:]
		} else {
			ck = "(plain)"
		}
		bucketRan[ck]++
		intTol := math.Max(0.10, 2e-5*math.Abs(dosInt))
		if math.Abs(goInt-dosInt) > intTol || math.Abs(goBal) > 5.0 {
			bucketDiv[ck]++
			divs = append(divs, gpDiv{ck, oracleCmd(c.amount, c.rate, c.n, c.perYr, hardFlags...), goInt, dosInt, goBal})
		}
	}
	reportGivenPay(t, "GIVEN-PAY-MERGED", seed, ran, bucketRan, bucketDiv, divs)
}
