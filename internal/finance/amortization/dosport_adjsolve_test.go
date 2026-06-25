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

// adjDateOnly builds an AO7 adjustment (date only: re-amortize at current rate).
func adjDateOnly(months int) RateAdjustment {
	return RateAdjustment{DateStatus: types.InOutInput, Date: dateMonthsAfterLoan(months)}
}

// adjAmountOnly builds an AO6 adjustment (new payment, no new rate: solve the
// implied rate at which that payment amortizes over the remaining term).
func adjAmountOnly(months int, amt float64) RateAdjustment {
	return RateAdjustment{
		DateStatus: types.InOutInput, Date: dateMonthsAfterLoan(months),
		AmtOK: true, Amount: amt}
}

// portAdjResult runs AmortizeDOS (solved payment) with the given adjustments.
func portAdjResult(amount, rate float64, n, perYr int, adjs []RateAdjustment) (totalInt, finalBal float64, ok bool) {
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
		Adjustments: adjs,
	}
	r := AmortizeDOS(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return 0, 0, false
	}
	return r.TotalInt, r.Schedule[len(r.Schedule)-1].Principal, true
}

// portAdjResultX is portAdjResult with an optional extra option applier (skip/mor).
func portAdjResultX(amount, rate float64, n, perYr int, adjs []RateAdjustment, extra func(*LoanInput)) (totalInt, finalBal float64, ok bool) {
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
		Adjustments: adjs,
	}
	if extra != nil {
		extra(&in)
	}
	r := AmortizeDOS(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return 0, 0, false
	}
	return r.TotalInt, r.Schedule[len(r.Schedule)-1].Principal, true
}

// TestDOSPortAdjSolveProbe checks AO7 (date-only re-amortize) and AO6 (payment-
// only ⇒ solve rate) against the DOS oracle. Opt-in (needs the oracle binary).
func TestDOSPortAdjSolveProbe(t *testing.T) {
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
		name  string
		adjs  []RateAdjustment
		flags []string
	}{
		{"AO7-date-only", []RateAdjustment{adjDateOnly(12)}, []string{"adj=12::"}},
		{"AO6-pay-only-1400", []RateAdjustment{adjAmountOnly(12, 1400)}, []string{"adj=12::1400.00"}},
		{"AO6-pay-only-5000", []RateAdjustment{adjAmountOnly(12, 5000)}, []string{"adj=12::5000.00"}},
	}
	for _, c := range cases {
		goInt, goBal, gok := portAdjResult(amount, rate, n, perYr, c.adjs)
		dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, c.flags...)
		if !gok || !dok {
			t.Logf("%s: skipped (port ok=%v oracle ok=%v)", c.name, gok, dok)
			continue
		}
		d := goInt - dosInt
		t.Logf("%s: PORT int=%.2f bal=%.2f | DOS int=%.2f | Δint=%.2f %s",
			c.name, goInt, goBal, dosInt, d, map[bool]string{true: "OK", false: "DIVERGE"}[math.Abs(d) < 0.5])
	}
}

// TestDOSPortAdjSolveSweep validates the AO6 (payment-only ⇒ solve rate) and AO7
// (date-only ⇒ re-amortize) adjustment paths through the port against the DOS
// oracle over random single-adjustment loans. For AO6 the new payment is a modest
// perturbation of the natural payment so the implied rate stays positive and
// solvable (an over-amortizing payment implies a negative rate the oracle's text
// dump can't report). Opt-in; needs the oracle.
func TestDOSPortAdjSolveSweep(t *testing.T) {
	if os.Getenv("PERSENSE_FUZZ") == "" {
		t.Skip("opt-in: set PERSENSE_FUZZ=1")
	}
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	seed := int64(31337)
	if s := os.Getenv("PERSENSE_FUZZ_SEED"); s != "" {
		if v, e := strconv.ParseInt(s, 10, 64); e == nil {
			seed = v
		}
	}
	rng := rand.New(rand.NewSource(seed))
	nCases := 150
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			nCases = v
		}
	}
	ao6, ao7, div := 0, 0, 0
	for i := 0; i < nCases; i++ {
		perYr := []int{12, 6, 4, 2, 1}[rng.Intn(5)]
		clean := 12 / perYr
		amount := float64(int(50000+rng.Float64()*450000)/1000) * 1000
		rate := math.Round((0.04+rng.Float64()*0.08)*10000) / 10000
		n := []int{12, 18, 24, 36, 48}[rng.Intn(5)]
		// adjustment on a payment date strictly inside the term.
		adjPay := 1 + rng.Intn(n-2)
		adjMonths := adjPay * clean
		isAO6 := rng.Float64() < 0.5

		var adjs []RateAdjustment
		var flags []string
		if isAO6 {
			natural, ok := runOraclePayment(amount, rate, n, perYr)
			if !ok || natural <= 0 {
				continue
			}
			// perturb ±25% but keep it amortizing at a positive rate.
			pay := math.Round(natural*(0.85+rng.Float64()*0.30)*100) / 100
			adjs = []RateAdjustment{adjAmountOnly(adjMonths, pay)}
			flags = []string{fmt.Sprintf("adj=%d::%s", adjMonths, strconv.FormatFloat(pay, 'f', 2, 64))}
		} else {
			adjs = []RateAdjustment{adjDateOnly(adjMonths)}
			flags = []string{fmt.Sprintf("adj=%d::", adjMonths)}
		}
		// Companion NON-balloon advanced option (the gate admits AO6/AO7 with these):
		// a calendar skip or a moratorium, on a clean payment date before the adj.
		var extra func(*LoanInput)
		switch rng.Intn(3) {
		case 1:
			s := []string{"6", "9", "5-7", "11-12"}[rng.Intn(4)]
			extra = func(in *LoanInput) { in.SkipMonths = skipSetRaw(s) }
			flags = append(flags, "skip="+s)
		case 2:
			mm := clean // a moratorium ending at the first payment date
			extra = func(in *LoanInput) {
				in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: dateMonthsAfterLoan(mm)}
			}
			flags = append(flags, fmt.Sprintf("mor=%d", mm))
		}

		goInt, goBal, gok := portAdjResultX(amount, rate, n, perYr, adjs, extra)
		dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, flags...)
		if !gok || !dok {
			continue
		}
		if isAO6 {
			ao6++
		} else {
			ao7++
		}
		tol := math.Max(0.10, 1e-5*math.Abs(dosInt))
		if math.Abs(goInt-dosInt) > tol || math.Abs(goBal) > 5.0 {
			div++
			if div <= 8 {
				t.Errorf("ADJ-SOLVE DIVERGE {%v} amt=%.0f r=%.4f n=%d py=%d: Go int=%.2f bal=%.2f / DOS int=%.2f (Δ %.2f)",
					flags, amount, rate, n, perYr, goInt, goBal, dosInt, goInt-dosInt)
			}
		}
	}
	t.Logf("DOS-port ADJ-SOLVE sweep: seed=%d AO6=%d AO7=%d divergences=%d", seed, ao6, ao7, div)
}

// TestDOSPortAO7BalloonDump prints the port's schedule for a balloon+AO7 case so
// it can be diffed against `amort_oracle ... rows` by hand. Opt-in.
func TestDOSPortAO7BalloonDump(t *testing.T) {
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
		Balloons:    []BalloonPayment{balloonAt(12, 20000)},
		Adjustments: []RateAdjustment{adjDateOnly(6)},
	}
	r := AmortizeDOS(in)
	t.Logf("PORT(AmortizeDOS) err=%v totalInt=%.4f rows=%d", r.Err, r.TotalInt, len(r.Schedule))
	pw := Amortize(in)
	t.Logf("PIECEWISE(Amortize) err=%v totalInt=%.4f rows=%d", pw.Err, pw.TotalInt, len(pw.Schedule))
}

// TestDOSPortCanHandleAdjustments guards the dosPortCanHandle routing for the
// adjustment shapes: AO5 (rate-only) and set-both are served by the port with or
// without a balloon; AO6 (payment-only) and AO7 (date-only) are served WITHOUT a
// balloon but routed to the piecewise engine WHEN a balloon is present (the
// surprising-DOS-early-payoff gap, see dosport_entry.go). No oracle needed.
func TestDOSPortCanHandleAdjustments(t *testing.T) {
	base := func(adjs []RateAdjustment, balloons []BalloonPayment) LoanInput {
		return LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 100000,
				LoanRateStatus: types.InOutInput, LoanRate: 0.06,
				NStatus: types.InOutInput, NPeriods: 24,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
				FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(12),
				LastOK: true, LastStatus: types.InOutInput, LastDate: dateMonthsAfterLoan(24),
			},
			Settings:    Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
			Fancy:       true,
			Adjustments: adjs,
			Balloons:    balloons,
		}
	}
	bln := []BalloonPayment{balloonAt(12, 20000)}
	setBoth := RateAdjustment{DateStatus: types.InOutInput, Date: dateMonthsAfterLoan(6),
		LoanRateStatus: types.InOutInput, LoanRate: 0.08, AmtOK: true, Amount: 3000}
	cases := []struct {
		name string
		in   LoanInput
		want bool
	}{
		{"AO5-rate-only no balloon", base([]RateAdjustment{adjRateAt(6, 0.08)}, nil), true},
		{"AO5-rate-only +balloon", base([]RateAdjustment{adjRateAt(6, 0.08)}, bln), true},
		{"set-both +balloon", base([]RateAdjustment{setBoth}, bln), true},
		{"AO6-pay-only no balloon", base([]RateAdjustment{adjAmountOnly(6, 3000)}, nil), true},
		{"AO7-date-only no balloon", base([]RateAdjustment{adjDateOnly(6)}, nil), true},
		{"AO6-pay-only +balloon", base([]RateAdjustment{adjAmountOnly(6, 3000)}, bln), false},
		{"AO7-date-only +balloon", base([]RateAdjustment{adjDateOnly(6)}, bln), false},
	}
	// dosPortCanHandle short-circuits on the package flag; force it on for the test.
	saved := dosPortEnabled
	dosPortEnabled = true
	defer func() { dosPortEnabled = saved }()
	for _, c := range cases {
		got := dosPortCanHandle(c.in, c.in.Loan, &c.in.Settings)
		if got != c.want {
			t.Errorf("%s: dosPortCanHandle=%v want %v", c.name, got, c.want)
		}
	}
}

var _ = fmt.Sprintf
var _ = strconv.Itoa
