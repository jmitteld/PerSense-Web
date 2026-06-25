package amortization

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// prepayKnown builds a known prepayment series: NN extra payments of amt each, at
// perYr/yr, starting startMonths after the loan date.
func prepayKnown(startMonths, nn, perYr int, amt float64) Prepayment {
	return Prepayment{
		StartDateStatus: types.InOutInput, StartDate: dateMonthsAfterLoan(startMonths),
		NNStatus: types.InOutInput, NN: nn,
		PerYrStatus: types.InOutInput, PerYr: perYr,
		PaymentStatus: types.InOutInput, Payment: amt,
	}
}

// portPrepayResult runs AmortizeDOS (solved payment) with the given prepayment.
func portPrepayResult(amount, rate float64, n, perYr int, pres []Prepayment) (totalInt, finalBal float64, ok bool) {
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
		Fancy:       true,
		Prepayments: pres,
	}
	r := AmortizeDOS(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return 0, 0, false
	}
	return r.TotalInt, r.Schedule[len(r.Schedule)-1].Principal, true
}

// TestDOSPortPrepayProbe checks forward (known) prepayment series through the port
// vs the DOS oracle, to establish whether the forward prepay walk is faithful
// before the AO9 amount/duration solves are added. Opt-in; needs the oracle.
func TestDOSPortPrepayProbe(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	const (
		amount = 100000.0
		rate   = 0.06
		n      = 24
		perYr  = 12
	)
	cases := []struct {
		name string
		pres []Prepayment
		flag string
	}{
		{"pre@6 x6 500", []Prepayment{prepayKnown(6, 6, 12, 500)}, "pre=6:6:12:500.00"},
		{"pre@3 x12 250", []Prepayment{prepayKnown(3, 12, 12, 250)}, "pre=3:12:12:250.00"},
		{"pre@12 x6 1000", []Prepayment{prepayKnown(12, 6, 12, 1000)}, "pre=12:6:12:1000.00"},
	}
	for _, c := range cases {
		goInt, goBal, gok := portPrepayResult(amount, rate, n, perYr, c.pres)
		dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, c.flag, "plusreg")
		if !gok || !dok {
			t.Logf("%s: skipped (port ok=%v oracle ok=%v)", c.name, gok, dok)
			continue
		}
		d := goInt - dosInt
		t.Logf("%s: PORT int=%.2f bal=%.2f | DOS int=%.2f | Δ=%.2f %s",
			c.name, goInt, goBal, dosInt, d, map[bool]string{true: "OK", false: "DIVERGE"}[math.Abs(d) < 0.5])
	}
}

// TestDOSPortPrepayForwardSweep validates random forward (known-amount) prepayment
// series through the port vs the DOS oracle. Opt-in; needs the oracle.
func TestDOSPortPrepayForwardSweep(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	seed := int64(24680)
	if s := os.Getenv("PERSENSE_FUZZ_SEED"); s != "" {
		if v, e := strconv.ParseInt(s, 10, 64); e == nil {
			seed = v
		}
	}
	rng := rand.New(rand.NewSource(seed))
	nCases := 200
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			nCases = v
		}
	}
	ran, div := 0, 0
	for i := 0; i < nCases; i++ {
		perYr := 12 // monthly loan; the prepay series can be at its own freq
		amount := float64(int(60000+rng.Float64()*400000)/1000) * 1000
		rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		n := []int{24, 36, 48}[rng.Intn(3)]
		start := 1 + rng.Intn(n/2)
		ppPerYr := []int{12, 6, 4}[rng.Intn(3)]
		maxNN := (n - start) * ppPerYr / 12
		if maxNN < 1 {
			continue
		}
		nn := 1 + rng.Intn(maxNN)
		amt := float64(int(100+rng.Float64()*1400)/50) * 50

		pres := []Prepayment{prepayKnown(start, nn, ppPerYr, amt)}
		flag := fmt.Sprintf("pre=%d:%d:%d:%s", start, nn, ppPerYr, strconv.FormatFloat(amt, 'f', 2, 64))
		goInt, goBal, gok := portPrepayResult(amount, rate, n, perYr, pres)
		dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, flag, "plusreg")
		if !gok || !dok {
			continue
		}
		ran++
		tol := math.Max(0.10, 1e-5*math.Abs(dosInt))
		if math.Abs(goInt-dosInt) > tol || math.Abs(goBal) > 5.0 {
			div++
			if div <= 8 {
				t.Errorf("PREPAY-FWD DIVERGE {%s} amt=%.0f r=%.4f n=%d: Go int=%.2f bal=%.2f / DOS int=%.2f (Δ %.2f)",
					flag, amount, rate, n, goInt, goBal, dosInt, goInt-dosInt)
			}
		}
	}
	t.Logf("DOS-port PREPAY-FWD sweep: seed=%d ran=%d divergences=%d", seed, ran, div)
}

// prepayUnknown builds an AO9 series: NN extra payments at perYr/yr starting
// startMonths after the loan, with a BLANK amount for the engine to solve.
func prepayUnknown(startMonths, nn, perYr int) Prepayment {
	return Prepayment{
		StartDateStatus: types.InOutInput, StartDate: dateMonthsAfterLoan(startMonths),
		NNStatus: types.InOutInput, NN: nn,
		PerYrStatus: types.InOutInput, PerYr: perYr,
		PaymentStatus: types.StatusEmpty, // solve this
	}
}

// portAO9Result runs AmortizeDOS with a GIVEN payment and an unknown prepayment.
func portAO9Result(amount, rate, pay float64, n, perYr, start, nn, ppPerYr int) AmortResult {
	return AmortizeDOS(LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus: types.InOutInput, PayAmt: pay,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr),
		},
		Settings:    Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:       true,
		Prepayments: []Prepayment{prepayUnknown(start, nn, ppPerYr)},
	})
}

// TestDOSPortAO9PrepaySolve validates the AO9 (unknown-prepayment-amount) solve:
// (1) round-trip — the solved amount retires the loan (the forward prepay walk is
// now oracle-exact, so a retiring solve is the DOS criterion); (2) oracle
// cross-check — feed the SOLVED amount back to the oracle as a KNOWN prepayment
// (reusing pre=) and confirm it reproduces the port's interest. Oracle opt-in.
func TestDOSPortAO9PrepaySolve(t *testing.T) {
	rng := rand.New(rand.NewSource(13579))
	const nCases = 250
	_, oracleErr := os.Stat(oracleBin)
	oracleOK := os.Getenv("PERSENSE_FUZZ") != "" && oracleErr == nil
	ran, retireFail, oraclesRun, oracleDiv := 0, 0, 0, 0
	for i := 0; i < nCases; i++ {
		perYr := 12
		amount := float64(int(60000+rng.Float64()*400000)/1000) * 1000
		rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		n := []int{24, 36, 48}[rng.Intn(3)]
		start := 1 + rng.Intn(n/2)
		ppPerYr := []int{12, 6}[rng.Intn(2)]
		maxNN := (n - start) * ppPerYr / 12
		if maxNN < 1 {
			continue
		}
		nn := 1 + rng.Intn(maxNN)
		// Under-amortizing payment so a positive prepayment is needed.
		natural := annuityPayment(amount, GrowthPerPeriod(&Loan{LoanRate: rate, PerYr: perYr}, 1.0/360), n)
		pay := math.Round(natural*(0.70+rng.Float64()*0.20)*100) / 100

		r := portAO9Result(amount, rate, pay, n, perYr, start, nn, ppPerYr)
		if r.Err != nil || len(r.Schedule) == 0 || r.SolvedPrepay <= 0 {
			continue
		}
		ran++
		if math.Abs(r.FinalPrinc) > math.Max(1.0, 1e-6*amount) {
			retireFail++
			if retireFail <= 6 {
				t.Errorf("AO9 not retired: amt=%.0f r=%.4f n=%d start=%d nn=%d pay=%.2f → FinalPrinc=%.2f solved=%.2f",
					amount, rate, n, start, nn, pay, r.FinalPrinc, r.SolvedPrepay)
			}
		}
		if oracleOK {
			flags := []string{
				fmt.Sprintf("pre=%d:%d:%d:%s", start, nn, ppPerYr, strconv.FormatFloat(r.SolvedPrepay, 'f', 2, 64)),
				fmt.Sprintf("payhard=%s", strconv.FormatFloat(pay, 'f', 2, 64)),
				"plusreg",
			}
			dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, flags...)
			if dok {
				oraclesRun++
				if math.Abs(r.TotalInt-dosInt) > math.Max(0.10, 1e-5*math.Abs(dosInt)) {
					oracleDiv++
					if oracleDiv <= 8 {
						t.Errorf("AO9 vs ORACLE: amt=%.0f r=%.4f n=%d start=%d nn=%d pay=%.2f solved=%.2f → port int=%.2f oracle int=%.2f (Δ %.2f)",
							amount, rate, n, start, nn, pay, r.SolvedPrepay, r.TotalInt, dosInt, r.TotalInt-dosInt)
					}
				}
			}
		}
	}
	t.Logf("AO9 prepay solve: ran=%d retireFail=%d oraclesRun=%d oracleDiv=%d", ran, retireFail, oraclesRun, oracleDiv)
	if ran == 0 {
		t.Fatal("no AO9 cases ran")
	}
}

// durInput builds an AO10 case: loan with a GIVEN (below-amortizing) payment and a
// prepayment with a KNOWN amount but blank count + stop date (solve the duration).
func durInput(amount, rate, pay float64, n, perYr, start, ppPerYr int, ppAmt float64) LoanInput {
	return LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus: types.InOutInput, PayAmt: pay,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: dateMonthsAfterLoan(start),
			PerYrStatus: types.InOutInput, PerYr: ppPerYr,
			PaymentStatus: types.InOutInput, Payment: ppAmt, // amount known; count/stop blank
		}},
	}
}

// TestDOSPortAO10Duration validates the port's prepayment-DURATION solve (AO10):
// AmortizeDOS resolves the unknown count up front (reusing the oracle-validated
// SolvePrepaymentDuration), pins it, and the now-oracle-exact forward walk runs
// the bounded series. Checks: (1) the loan retires; (2) feeding the solved count
// back to the oracle as a KNOWN prepayment reproduces the port's interest. Oracle
// opt-in. (The closed-form count itself is already validated by
// TestDOSPrepaymentDurationSolveSweep; this confirms the port wires + runs it.)
func TestDOSPortAO10Duration(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	rng := rand.New(rand.NewSource(31415))
	ran, retireFail, intDiv := 0, 0, 0
	for i := 0; i < 200; i++ {
		perYr := 12
		amount := float64(int(60000+rng.Float64()*400000)/1000) * 1000
		rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		n := []int{120, 180, 240, 360}[rng.Intn(4)]
		start := 1
		ppPerYr := 12
		amort, ok0 := runOraclePayment(amount, rate, n, perYr)
		if !ok0 {
			continue
		}
		pay := math.Round(amort*(0.45+rng.Float64()*0.35)*100) / 100
		ppAmt := math.Round(amort*(0.3+rng.Float64()*0.4)*100) / 100

		in := durInput(amount, rate, pay, n, perYr, start, ppPerYr, ppAmt)
		nn, _, err := SolvePrepaymentDuration(in, 0)
		if err != nil || nn <= 0 {
			continue
		}
		res := AmortizeDOS(in)
		if res.Err != nil || len(res.Schedule) == 0 {
			continue
		}
		ran++
		if math.Abs(res.FinalPrinc) > math.Max(1.0, 1e-6*amount) {
			retireFail++
			if retireFail <= 6 {
				t.Errorf("AO10 not retired: amt=%.0f r=%.4f n=%d pay=%.2f pre=%.2f nn=%d → FinalPrinc=%.2f",
					amount, rate, n, pay, ppAmt, nn, res.FinalPrinc)
			}
		}
		flags := []string{
			fmt.Sprintf("pre=%d:%d:%d:%s", start, nn, ppPerYr, strconv.FormatFloat(ppAmt, 'f', 2, 64)),
			fmt.Sprintf("payhard=%s", strconv.FormatFloat(pay, 'f', 2, 64)),
			"plusreg",
		}
		dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, flags...)
		if dok {
			if math.Abs(res.TotalInt-dosInt) > math.Max(0.10, 1e-5*math.Abs(dosInt)) {
				intDiv++
				if intDiv <= 6 {
					t.Errorf("AO10 vs ORACLE: amt=%.0f r=%.4f n=%d pay=%.2f pre=%.2f nn=%d → port int=%.2f oracle int=%.2f (Δ %.2f)",
						amount, rate, n, pay, ppAmt, nn, res.TotalInt, dosInt, res.TotalInt-dosInt)
				}
			}
		}
	}
	t.Logf("AO10 duration solve: ran=%d retireFail=%d intDiv=%d", ran, retireFail, intDiv)
}

// TestDOSPortPrepayDump prints the port's schedule for a known prepayment case so
// it can be diffed against `amort_oracle ... rows`. Opt-in.
func TestDOSPortPrepayDump(t *testing.T) {
	if os.Getenv("PERSENSE_DUMP") == "" {
		t.Skip("opt-in: set PERSENSE_DUMP=1")
	}
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.06,
			NStatus: types.InOutInput, NPeriods: 24,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(12),
		},
		Settings:    Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:       true,
		Prepayments: []Prepayment{prepayKnown(6, 6, 12, 500)},
	}
	r := AmortizeDOS(in)
	t.Logf("PORT totalInt=%.4f rows=%d", r.TotalInt, len(r.Schedule))
	for i, row := range r.Schedule {
		t.Logf("  L%d date=%v pay=%.4f int=%.4f bal=%.4f", i+1,
			row.Date.Time.Format("2006-01-02"), row.PayAmt, row.Interest, row.Principal)
	}
}

var _ = fmt.Sprintf
var _ = strconv.Itoa
