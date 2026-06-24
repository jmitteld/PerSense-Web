package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_completion_test.go — broad unit coverage for the amortization package:
// the small zero-helpers, the forward schedules for every computational setting,
// the backward solvers, the balance/payoff lookups, and the error/boundary
// branches. DOS-numeric fidelity is covered exhaustively by the oracle cubes
// (TestDOSGroundZeroRowCube, TestDOSAmortizationUICube, TestDOSFancyFlagSweep,
// TestDOSClientExactScenario); these tests drive the remaining code paths so the
// package is exercised end to end and regressions in any branch surface.

// ---- helpers -------------------------------------------------------------

func ccBaseLoan(amount, rate float64, n, perYr int) Loan {
	return Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}
}

func cc360() Settings {
	return Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360}
}

// ---- zero-helpers --------------------------------------------------------

func TestZeroHelpers(t *testing.T) {
	var m Moratorium
	m.FirstRepayStatus = types.InOutInput
	ZeroMoratorium(&m)
	if m.FirstRepayStatus != 0 || dateutilOK(m.FirstRepay) {
		t.Errorf("ZeroMoratorium did not clear: %+v", m)
	}
	tg := Target{TargetStatus: types.InOutInput, TargetValue: 5}
	ZeroTarget(&tg)
	if tg.TargetStatus != 0 || tg.TargetValue != 0 {
		t.Errorf("ZeroTarget did not clear: %+v", tg)
	}
	sk := SkipMonths{SkipStatus: types.InOutInput, SkipStr: "6-8"}
	sk.MonthSet[6] = true
	ZeroSkipMonths(&sk)
	if sk.SkipStatus != 0 || sk.SkipStr != "" || sk.MonthSet[6] {
		t.Errorf("ZeroSkipMonths did not clear: %+v", sk)
	}
}

func dateutilOK(d types.DateRec) bool { return !d.IsUnknown() }

// ---- forward schedules for each computational setting --------------------

func TestForwardSchedulesAllSettings(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*Settings)
	}{
		{"ordinary", func(s *Settings) {}},
		{"in-advance", func(s *Settings) { s.InAdvance = true }},
		{"r78", func(s *Settings) { s.R78 = true }},
		{"usa-rule", func(s *Settings) { s.USARule = true }},
		{"prepaid", func(s *Settings) { s.Prepaid = true }},
		{"exact-365", func(s *Settings) { s.Basis, s.YrDays, s.YrInv, s.Exact = types.Basis365, 365.25, 1.0/365.25, true }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := cc360()
			c.mod(&s)
			in := LoanInput{Loan: ccBaseLoan(100000, 0.09, 60, 12), Settings: s}
			r := Amortize(in)
			if r.Err != nil {
				t.Fatalf("unexpected error: %v", r.Err)
			}
			if len(r.Schedule) == 0 {
				t.Fatal("empty schedule")
			}
			// Final balance must retire (within a dollar — exact/365 has a tiny tail).
			last := r.Schedule[len(r.Schedule)-1]
			if math.Abs(last.Principal) > 1.0 {
				t.Errorf("loan did not retire: final balance %.2f", last.Principal)
			}
			// Cumulative interest equals the sum of row interest.
			var sum float64
			for _, row := range r.Schedule {
				sum += row.Interest
			}
			if math.Abs(sum-r.TotalInt) > 0.05 {
				t.Errorf("TotalInt %.2f != sum of rows %.2f", r.TotalInt, sum)
			}
		})
	}
}

// ---- backward solvers ----------------------------------------------------

func TestSolvePaymentBasic(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: cc360()}
	pay, err := SolvePaymentClosedForm(in)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(pay-1028.61) > 0.5 {
		t.Errorf("payment %.2f, want ~1028.61", pay)
	}
	// Zero-rate even split.
	in0 := LoanInput{Loan: ccBaseLoan(12000, 0, 12, 12), Settings: cc360()}
	in0.Loan.LoanRateStatus = types.InOutInput
	p0, err := SolvePaymentClosedForm(in0)
	if err != nil || math.Abs(p0-1000) > 0.01 {
		t.Errorf("zero-rate payment %.4f (err %v), want 1000", p0, err)
	}
}

func TestSolveLoanAmountAndRate(t *testing.T) {
	// Solve loan amount from a known payment.
	la := LoanInput{Loan: ccBaseLoan(0, 0.12, 360, 12), Settings: cc360()}
	la.Loan.AmountStatus = types.StatusEmpty
	la.Loan.PayAmtStatus = types.InOutInput
	la.Loan.PayAmt = 1028.6126
	amt, conv, err := SolveLoanAmount(la)
	if err != nil || !conv || math.Abs(amt-100000) > 5 {
		t.Errorf("SolveLoanAmount = %.2f (conv %v err %v), want ~100000", amt, conv, err)
	}
	// Solve rate from a known payment.
	rt := LoanInput{Loan: ccBaseLoan(100000, 0, 360, 12), Settings: cc360()}
	rt.Loan.LoanRateStatus = types.StatusEmpty
	rt.Loan.PayAmtStatus = types.InOutInput
	rt.Loan.PayAmt = 1028.6126
	rate, conv, err := SolveRate(rt)
	if err != nil || !conv || math.Abs(rate-0.12) > 0.001 {
		t.Errorf("SolveRate = %.5f (conv %v err %v), want ~0.12", rate, conv, err)
	}
}

func TestSolveNPeriodsFromPayment(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 0, 12), Settings: cc360()}
	in.Loan.NStatus = types.StatusEmpty
	in.Loan.LastStatus = types.StatusEmpty
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1028.62
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	if r.NPeriods < 350 || r.NPeriods > 370 {
		t.Errorf("derived NPeriods %d, want ~360", r.NPeriods)
	}
}

// ---- balloons, prepayments, adjustments, target, moratorium, skip --------

func TestKnownBalloonReducesPayment(t *testing.T) {
	plain := Amortize(LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: cc360()})
	withB := LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: cc360(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2026, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 20000}}}
	rb := Amortize(withB)
	if rb.Err != nil {
		t.Fatalf("balloon err: %v", rb.Err)
	}
	if modalReg(rb.Schedule) >= modalReg(plain.Schedule) {
		t.Errorf("a known balloon should reduce the regular payment: plain=%.2f balloon=%.2f",
			modalReg(plain.Schedule), modalReg(rb.Schedule))
	}
}

func TestUnknownBalloonSolved(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: cc360(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2028, time.December, 1),
			AmountStatus: types.StatusEmpty}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1500
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	if len(r.Balloons) == 0 || !r.Balloons[0].Solved {
		t.Errorf("expected a solved balloon amount, got %+v", r.Balloons)
	}
}

func TestPrepaymentKnownAndDuration(t *testing.T) {
	// Known prepayment amount ADDED on top of the regular payment shortens the
	// term (PlusRegular = the extra adds to, not replaces, the regular payment).
	s := cc360()
	s.PlusRegular = true
	in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: s, Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.February, 1),
			PaymentStatus: types.InOutInput, Payment: 500,
			PerYrStatus: types.InOutInput, PerYr: 12,
			StopDateStatus: types.InOutInput, StopDate: types.NewDateRec(2044, time.February, 1)}}}
	in.Loan.PayAmtStatus = types.InOutInput
	in.Loan.PayAmt = 1028.61
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("prepayment err: %v", r.Err)
	}
	// Baseline without the prepayment, same hard payment.
	base := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: cc360()}
	base.Loan.PayAmtStatus = types.InOutInput
	base.Loan.PayAmt = 1028.61
	rb := Amortize(base)
	if r.TotalInt >= rb.TotalInt {
		t.Errorf("additive prepayments should cut total interest: with=%.2f without=%.2f", r.TotalInt, rb.TotalInt)
	}
}

func TestRateAdjustmentARM(t *testing.T) {
	in := LoanInput{Loan: ccBaseLoan(100000, 0.06, 120, 12), Settings: cc360(), Fancy: true,
		Adjustments: []RateAdjustment{{
			DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.10}}}
	in.Loan.PayAmtStatus = types.StatusEmpty
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("ARM err: %v", r.Err)
	}
	if math.Abs(r.Schedule[len(r.Schedule)-1].Principal) > 1.0 {
		t.Errorf("ARM did not retire: %.2f", r.Schedule[len(r.Schedule)-1].Principal)
	}
}

func TestTargetAndMoratoriumAndSkip(t *testing.T) {
	// Moratorium: interest-only until a date.
	mor := LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: cc360(), Fancy: true,
		Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2024, time.August, 1)}}
	mor.Loan.PayAmtStatus = types.StatusEmpty
	if r := Amortize(mor); r.Err != nil {
		t.Errorf("moratorium err: %v", r.Err)
	}
	// Skip months.
	sk := LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: cc360(), Fancy: true,
		SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "7-8"}}
	sk.SkipMonths.MonthSet[7] = true
	sk.SkipMonths.MonthSet[8] = true
	sk.Loan.PayAmtStatus = types.StatusEmpty
	if r := Amortize(sk); r.Err != nil {
		t.Errorf("skip-months err: %v", r.Err)
	}
	// Target minimum principal reduction.
	tg := LoanInput{Loan: ccBaseLoan(100000, 0.12, 60, 12), Settings: cc360(), Fancy: true,
		Target: Target{TargetStatus: types.InOutInput, TargetValue: 1000}}
	tg.Loan.PayAmtStatus = types.InOutInput
	tg.Loan.PayAmt = 1500
	if r := Amortize(tg); r.Err != nil {
		t.Errorf("target err: %v", r.Err)
	}
}

// ---- balance / payoff lookups -------------------------------------------

func TestBalanceAndDateLookups(t *testing.T) {
	r := Amortize(LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: cc360()})
	if r.Err != nil {
		t.Fatalf("schedule err: %v", r.Err)
	}
	bal := BalanceAtDate(r.Schedule, 100000, types.NewDateRec(2025, time.February, 1))
	if bal <= 0 || bal >= 100000 {
		t.Errorf("BalanceAtDate = %.2f, want a partial balance", bal)
	}
	d, ok := DateForBalance(r.Schedule, bal)
	if !ok || !dateutilOK(d) {
		t.Errorf("DateForBalance failed (ok %v, date %v)", ok, d)
	}
}

// ---- error & boundary branches ------------------------------------------

func TestErrorBranches(t *testing.T) {
	mk := func(mut func(*LoanInput)) AmortResult {
		in := LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: cc360()}
		mut(&in)
		return Amortize(in)
	}
	if r := mk(func(in *LoanInput) { in.Loan.AmountStatus = types.StatusEmpty; in.Loan.PayAmtStatus = types.StatusEmpty }); r.Err == nil {
		t.Error("expected error when amount and payment both blank")
	}
	if r := mk(func(in *LoanInput) { in.Loan.PerYrStatus = types.StatusEmpty }); r.Err == nil {
		t.Error("expected error when Pmts/Yr blank")
	}
	if r := mk(func(in *LoanInput) { in.Loan.LoanDate = types.UnknownDate() }); r.Err == nil {
		t.Error("expected error when loan date blank")
	}
	// Single payment is rejected.
	if r := mk(func(in *LoanInput) { in.Loan.NPeriods = 1 }); r.Err == nil {
		t.Error("expected error for a single-payment loan")
	}
	// Oversized period count is refused.
	if r := mk(func(in *LoanInput) { in.Loan.NPeriods = MaxSchedulePeriods + 5 }); r.Err == nil {
		t.Error("expected error for an oversized period count")
	}
}

func TestBoundaryRatesAndTerms(t *testing.T) {
	// Zero rate.
	z := Amortize(LoanInput{Loan: ccBaseLoan(120000, 0, 120, 12), Settings: cc360()})
	if z.Err != nil {
		t.Fatalf("zero-rate err: %v", z.Err)
	}
	if math.Abs(modalReg(z.Schedule)-1000) > 0.5 {
		t.Errorf("zero-rate payment %.2f, want ~1000", modalReg(z.Schedule))
	}
	// Very high rate triggers the advisory but still computes.
	h := Amortize(LoanInput{Loan: ccBaseLoan(100000, 0.55, 60, 12), Settings: cc360()})
	if h.Err != nil {
		t.Fatalf("high-rate err: %v", h.Err)
	}
	if len(h.Warnings) == 0 {
		t.Error("expected an unusually-high-rate warning")
	}
}
