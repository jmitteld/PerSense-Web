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

// goAmortizeOptionsDOS is goAmortizeOptions but through the FAITHFUL PORT
// (AmortizeDOS) instead of the piecewise production engine.
func goAmortizeOptionsDOS(amount, rate float64, n, perYr int, apply func(*LoanInput)) (totalInt, finalBal float64, ok bool) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr),
			YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy: true,
	}
	if apply != nil {
		apply(&in)
	}
	r := AmortizeDOS(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return 0, 0, false
	}
	return r.TotalInt, r.Schedule[len(r.Schedule)-1].Principal, true
}

// TestDOSPortFuzz drives the SAME randomized option-cube as TestDOSOptionCubeFuzz
// but through the faithful port (AmortizeDOS), diffing it against the real DOS
// oracle. This is the acceptance gate for the global-Iterate refactor: it must
// reach 0 divergences (then the port becomes the default). Opt-in like the other
// fuzzer; needs the oracle binary.
func TestDOSPortFuzz(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1 to run the DOS-port fuzzer")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	seed := int64(20260624)
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

	bucketRan := map[string]int{}
	bucketDiv := map[string]int{}
	type div struct {
		desc, cmd     string
		goInt, dosInt float64
		goBal, dosBal float64
	}
	var divs []div
	ran, skipped := 0, 0

	for i := 0; i < nCases; i++ {
		amount, rate, n, perYr, fc := genFuzzCase(rng)
		goInt, goBal, gok := goAmortizeOptionsDOS(amount, rate, n, perYr, fc.apply)
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
		intTol := math.Max(25.0, 5e-4*math.Abs(dosInt))
		const balTol = 5.0
		diverged := false
		if math.Abs(goInt-dosInt) > intTol {
			diverged = true
		}
		if math.Abs(goBal) > balTol && math.Abs(goBal-dosBal) > balTol {
			diverged = true
		}
		if diverged {
			bucketDiv[key]++
			divs = append(divs, div{fc.desc, oracleCmd(amount, rate, n, perYr, fc.flags...), goInt, dosInt, goBal, dosBal})
		}
	}

	t.Logf("DOS-port fuzz: seed=%d cases=%d ran=%d skipped=%d divergences=%d", seed, nCases, ran, skipped, len(divs))
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
	sb.WriteString("DOS-port divergence map (diverged/ran):\n")
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("    %-30s %d/%d\n", k, bucketDiv[k], bucketRan[k]))
	}
	t.Log(sb.String())
	shown := map[string]int{}
	for _, d := range divs {
		k := classKeyFromCmd(d.cmd)
		if shown[k] >= 3 {
			continue
		}
		shown[k]++
		t.Errorf("PORT DIVERGENCE {%s} %s\n    Go %.2f / DOS %.2f (Δ %.2f) | bal Go %.2f / DOS %.2f\n    repro: %s",
			k, d.desc, d.goInt, d.dosInt, d.goInt-d.dosInt, d.goBal, d.dosBal, d.cmd)
	}
}
