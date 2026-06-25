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
	"time"

	"github.com/persense/persense-port/internal/types"
)

// monthsAfterLoanStart returns the date `m` months after the fixed loan date
// 2024-01-01 (the same convention as dateMonthsAfterLoan / the oracle's
// month-offset placement).
func monthsAfterLoanStart(m int) types.DateRec {
	return types.NewDateRec(2024+m/12, time.Month(m%12+1), 1)
}

// portBasisResult runs AmortizeDOS for a loan with explicit frequency / first
// period / prepaid and returns total interest + final balance.
func portBasisResult(amount, rate float64, n, perYr, firstMonths int, prepaid bool) (totalInt, finalBal float64, ok bool) {
	s := Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true, Prepaid: prepaid}
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: monthsAfterLoanStart(firstMonths),
		},
		Settings: s,
		Fancy:    true,
	}
	r := AmortizeDOS(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return 0, 0, false
	}
	return r.TotalInt, r.Schedule[len(r.Schedule)-1].Principal, true
}

// TestDOSPortFuzzBasis broadens the port acceptance fuzzer beyond the original
// (solved-payment, monthly, clean-first-period, non-prepaid) domain to the
// dimensions the M3 cutover exposed: non-monthly PERYR, odd/long FIRST PERIODS,
// and PREPAID interest. Plain loans only, so a divergence isolates a
// frequency/first-period/prepaid bug rather than an option interaction. Opt-in
// (PERSENSE_FUZZ=1); needs the oracle.
func TestDOSPortFuzzBasis(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	seed := int64(424242)
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
	type div struct {
		key, cmd      string
		goInt, dosInt float64
		goBal         float64
	}
	var divs []div
	ran := 0

	for i := 0; i < nCases; i++ {
		perYr := perYrs[rng.Intn(len(perYrs))]
		clean := 12 / perYr
		amount := float64(int(50000+rng.Float64()*450000)/1000) * 1000
		rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		n := []int{12, 18, 24, 36, 48, 60}[rng.Intn(6)]
		prepaid := rng.Float64() < 0.5

		// first period: clean, or an odd/long stub of K months (1..2*clean, != clean).
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

		var flags []string
		if firstMonths != clean {
			flags = append(flags, fmt.Sprintf("first=%d", firstMonths))
		}
		if prepaid {
			flags = append(flags, "prepaid")
		}

		goInt, goBal, gok := portBasisResult(amount, rate, n, perYr, firstMonths, prepaid)
		if !gok {
			continue
		}
		dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, flags...)
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

		intTol := math.Max(25.0, 5e-4*math.Abs(dosInt))
		if math.Abs(goInt-dosInt) > intTol || math.Abs(goBal) > 5.0 {
			bucketDiv[key]++
			cmd := oracleCmd(amount, rate, n, perYr, flags...)
			divs = append(divs, div{key, cmd, goInt, dosInt, goBal})
		}
	}

	t.Logf("DOS-port BASIS fuzz: seed=%d ran=%d divergences=%d", seed, ran, len(divs))
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
	sb.WriteString("BASIS divergence map (diverged/ran):\n")
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
		t.Errorf("BASIS DIVERGENCE {%s} Go %.2f / DOS %.2f (Δ %.2f) bal %.2f\n    repro: %s",
			d.key, d.goInt, d.dosInt, d.goInt-d.dosInt, d.goBal, d.cmd)
	}
}
