package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_errors_test.go — exercises the validation, error, edge, and helper
// branches across the amortization package so latent paths are covered and
// regressions surface. Numeric paths that have a DOS analogue are validated by
// the oracle cubes; these focus on the guard/derive/helper code.

func ccInput() LoanInput {
	return LoanInput{Loan: ccBaseLoan(100000, 0.12, 360, 12), Settings: cc360()}
}

// ---- ValidateInputs branches --------------------------------------------

func TestValidateInputsErrors(t *testing.T) {
	type tc struct {
		name string
		mut  func(*LoanInput)
	}
	d := func(y int, m time.Month, day int) types.DateRec { return types.NewDateRec(y, m, day) }
	cases := []tc{
		{"first after last", func(in *LoanInput) {
			in.Loan.FirstDate = d(2030, time.January, 1)
			in.Loan.LastDate = d(2025, time.January, 1)
			in.Loan.LastStatus = types.InOutInput
			in.Loan.LastOK = true
			in.Loan.NStatus = types.StatusEmpty
		}},
		{"loan after first", func(in *LoanInput) {
			in.Loan.LoanDate = d(2024, time.March, 1)
			in.Loan.FirstDate = d(2024, time.February, 1)
		}},
		{"prepayment before loan", func(in *LoanInput) {
			in.Fancy = true
			in.Prepayments = []Prepayment{{StartDateStatus: types.InOutInput, StartDate: d(2023, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12, PaymentStatus: types.InOutInput, Payment: 100}}
		}},
		{"in-advance with adjustment", func(in *LoanInput) {
			in.Settings.InAdvance = true
			in.Fancy = true
			in.Adjustments = []RateAdjustment{{DateStatus: types.InOutInput, Date: d(2026, time.January, 1),
				LoanRateStatus: types.InOutInput, LoanRate: 0.10}}
		}},
		{"two adjustments same date", func(in *LoanInput) {
			in.Fancy = true
			in.Adjustments = []RateAdjustment{
				{DateStatus: types.InOutInput, Date: d(2026, time.January, 1), LoanRateStatus: types.InOutInput, LoanRate: 0.10},
				{DateStatus: types.InOutInput, Date: d(2026, time.January, 1), LoanRateStatus: types.InOutInput, LoanRate: 0.11}}
		}},
		{"adjustment before loan date", func(in *LoanInput) {
			in.Fancy = true
			in.Adjustments = []RateAdjustment{{DateStatus: types.InOutInput, Date: d(2023, time.January, 1),
				LoanRateStatus: types.InOutInput, LoanRate: 0.10}}
		}},
		{"balloon before first date", func(in *LoanInput) {
			in.Fancy = true
			in.Balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: d(2024, time.January, 15),
				AmountStatus: types.InOutInput, Amount: 5000}}
		}},
		{"two unknown balloons", func(in *LoanInput) {
			in.Fancy = true
			in.Loan.PayAmtStatus = types.InOutInput
			in.Loan.PayAmt = 1100
			in.Balloons = []BalloonPayment{
				{DateStatus: types.InOutInput, Date: d(2026, time.January, 1), AmountStatus: types.StatusEmpty},
				{DateStatus: types.InOutInput, Date: d(2027, time.January, 1), AmountStatus: types.StatusEmpty}}
		}},
		{"two unknown prepayments", func(in *LoanInput) {
			in.Fancy = true
			in.Loan.PayAmtStatus = types.InOutInput
			in.Loan.PayAmt = 1100
			in.Prepayments = []Prepayment{
				{StartDateStatus: types.InOutInput, StartDate: d(2025, time.January, 1), PerYrStatus: types.InOutInput, PerYr: 12, PaymentStatus: types.StatusEmpty},
				{StartDateStatus: types.InOutInput, StartDate: d(2026, time.January, 1), PerYrStatus: types.InOutInput, PerYr: 12, PaymentStatus: types.StatusEmpty}}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := ccInput()
			c.mut(&in)
			if r := Amortize(in); r.Err == nil {
				t.Errorf("expected a validation error for %q", c.name)
			}
		})
	}
}

// ---- pure helpers --------------------------------------------------------

func TestAnnuityPaymentHelper(t *testing.T) {
	if got := annuityPayment(12000, 1.01, 0); got != 12000 {
		t.Errorf("n<=0 should return balance, got %.2f", got)
	}
	if got := annuityPayment(12000, 1.0, 12); math.Abs(got-1000) > 1e-9 {
		t.Errorf("f≈1 should be even split, got %.4f", got)
	}
	if got := annuityPayment(100000, 1.01, 360); math.Abs(got-1028.61) > 0.5 {
		t.Errorf("standard annuity = %.2f, want ~1028.61", got)
	}
}

func TestPrepayStopDateHelper(t *testing.T) {
	// NN-based stop date.
	pp := Prepayment{StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.February, 1),
		PerYr: 12, NN: 6, NNStatus: types.InOutInput}
	if _, err := prepayStopDate(pp); err != nil {
		t.Errorf("NN-based stop date errored: %v", err)
	}
	// Neither stop date nor count → error.
	bad := Prepayment{StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.February, 1), PerYr: 12}
	if _, err := prepayStopDate(bad); err == nil {
		t.Error("expected error when neither stop date nor count is set")
	}
}

func TestPrepayAnnuityHelper(t *testing.T) {
	s := cc360()
	start := types.NewDateRec(2024, time.February, 1)
	stop := types.NewDateRec(2029, time.February, 1)
	repayFrom := types.NewDateRec(2024, time.January, 1)
	if v, err := prepayAnnuity(0.12, start, stop, 12, repayFrom, s); err != nil || v <= 0 {
		t.Errorf("prepayAnnuity = %.4f (err %v), want > 0", v, err)
	}
	// Degenerate: rate ~0 → ff ~1 → error.
	if _, err := prepayAnnuity(0, start, stop, 12, repayFrom, s); err == nil {
		t.Error("expected degenerate-annuity error at zero rate")
	}
}

func TestComputeTrueRateHelper(t *testing.T) {
	loan := ccBaseLoan(100000, 0.12, 360, 12)
	s := cc360()
	if _, err := ComputeTrueRate(&loan, &s); err != nil {
		t.Errorf("ComputeTrueRate errored: %v", err)
	}
}

// ---- FirstPass derivations ----------------------------------------------

func TestFirstPassDerivations(t *testing.T) {
	// Derive NPeriods from first + last date.
	l1 := ccBaseLoan(100000, 0.12, 0, 12)
	l1.NStatus = types.StatusEmpty
	l1.LastDate = types.NewDateRec(2027, time.January, 1)
	l1.LastStatus = types.InOutInput
	l1.LastOK = true
	if err := FirstPass(&l1); err != nil {
		t.Fatalf("derive nperiods: %v", err)
	}
	if l1.NPeriods <= 0 {
		t.Errorf("expected derived NPeriods > 0, got %d", l1.NPeriods)
	}
	// Derive last date from first + nperiods.
	l2 := ccBaseLoan(100000, 0.12, 36, 12)
	l2.LastStatus = types.StatusEmpty
	l2.LastOK = false
	if err := FirstPass(&l2); err != nil {
		t.Fatalf("derive last: %v", err)
	}
	if !l2.LastOK {
		t.Error("expected last date to be derived")
	}
	// Derive first date from loan date + perYr (first left blank).
	l3 := ccBaseLoan(100000, 0.12, 36, 12)
	l3.FirstStatus = types.StatusEmpty
	l3.FirstDate = types.UnknownDate()
	if err := FirstPass(&l3); err != nil {
		t.Fatalf("derive first: %v", err)
	}
	if !dateutilOK(l3.FirstDate) {
		t.Error("expected first date to be derived from loan date")
	}
}

// ---- APR with points -----------------------------------------------------

func TestAPRWithPoints(t *testing.T) {
	in := ccInput()
	in.Loan.PointsStatus = types.InOutInput
	in.Loan.Points = 0.02 // 2 points
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	if r.APR <= in.Loan.LoanRate {
		t.Errorf("APR with points (%.5f) should exceed the note rate (%.5f)", r.APR, in.Loan.LoanRate)
	}
}

// ---- ARM payment adjustment (solveAdjRate / re-amortize) -----------------

func TestARMPaymentAdjustmentAndReamortize(t *testing.T) {
	d := func(y int, m time.Month, day int) types.DateRec { return types.NewDateRec(y, m, day) }
	// Date-only adjustment: re-amortize at the same rate (AO7).
	reamort := ccInput()
	reamort.Loan.NPeriods = 120
	reamort.Fancy = true
	reamort.Loan.PayAmtStatus = types.StatusEmpty
	reamort.Adjustments = []RateAdjustment{{DateStatus: types.InOutInput, Date: d(2029, time.January, 1)}}
	if r := Amortize(reamort); r.Err != nil {
		t.Errorf("date-only re-amortize errored: %v", r.Err)
	}
	// Adjustment with a new payment amount.
	withAmt := ccInput()
	withAmt.Loan.NPeriods = 120
	withAmt.Fancy = true
	withAmt.Loan.PayAmtStatus = types.StatusEmpty
	withAmt.Adjustments = []RateAdjustment{{DateStatus: types.InOutInput, Date: d(2029, time.January, 1),
		AmtOK: true, Amount: 1300}}
	if r := Amortize(withAmt); r.Err != nil {
		t.Errorf("payment-adjustment errored: %v", r.Err)
	}
}

// ---- balance / date lookup edges ----------------------------------------

func TestBalanceLookupEdges(t *testing.T) {
	r := Amortize(ccInput())
	if r.Err != nil {
		t.Fatalf("schedule err: %v", r.Err)
	}
	// Date before the first row → full loan amount.
	pre := BalanceAtDate(r.Schedule, 100000, types.NewDateRec(2020, time.January, 1))
	if math.Abs(pre-100000) > 0.01 {
		t.Errorf("balance before schedule = %.2f, want 100000", pre)
	}
	// Date after the last row → ~0.
	post := BalanceAtDate(r.Schedule, 100000, types.NewDateRec(2060, time.January, 1))
	if math.Abs(post) > 1.0 {
		t.Errorf("balance after payoff = %.2f, want ~0", post)
	}
	// DateForBalance with an unreachable (negative) target → not found.
	if _, ok := DateForBalance(r.Schedule, -5); ok {
		t.Error("DateForBalance should not find a negative target")
	}
}

// ---- solver insufficient-data errors ------------------------------------

func TestSolverInsufficientData(t *testing.T) {
	// SolveLoanAmount without a payment.
	la := ccInput()
	la.Loan.AmountStatus = types.StatusEmpty
	la.Loan.PayAmtStatus = types.StatusEmpty
	if _, _, err := SolveLoanAmount(la); err == nil {
		t.Error("SolveLoanAmount should error without a payment")
	}
	// SolveRate with zero amount.
	rt := ccInput()
	rt.Loan.LoanRateStatus = types.StatusEmpty
	rt.Loan.Amount = 0
	rt.Loan.PayAmtStatus = types.InOutInput
	rt.Loan.PayAmt = 1000
	if _, _, err := SolveRate(rt); err == nil {
		t.Error("SolveRate should error with zero amount")
	}
	// SolvePaymentClosedForm without enough data.
	sp := ccInput()
	sp.Loan.AmountStatus = types.StatusEmpty
	if _, err := SolvePaymentClosedForm(sp); err == nil {
		t.Error("SolvePaymentClosedForm should error without amount")
	}
}
