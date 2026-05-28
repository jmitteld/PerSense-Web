package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Idempotency / aliasing tests.
//
// Amortize takes its LoanInput by value, but the advanced-option
// fields (Prepayments, Balloons, Adjustments) are slices whose backing
// arrays are shared with the caller. Any in-place mutation of those
// elements therefore leaks back out and can make a second Amortize on
// the "same" input behave differently. That non-idempotency is exactly
// what broke the iterateNewton backward solver (it evaluates many
// trials against one shared input). These tests pin the invariant:
//
//   - Calling Amortize repeatedly on one input must yield byte-for-byte
//     identical schedules and result aggregates.
//   - A backward solve repeated on one input must return the same value.
//
// They cover each advanced option in isolation and several stacked
// combinations, so a future mutation-leak in any option path is caught.

// schedulesEqual reports the first structural difference between two
// schedules, or "" if they are identical.
func schedulesEqual(a, b []PaymentRecord) (string, bool) {
	if len(a) != len(b) {
		return "length differs", false
	}
	for i := range a {
		switch {
		case a[i].PayNum != b[i].PayNum:
			return "PayNum differs at row " + itoa(i), false
		case !a[i].Date.Time.Equal(b[i].Date.Time):
			return "Date differs at row " + itoa(i), false
		case !floatEq(a[i].PayAmt, b[i].PayAmt):
			return "PayAmt differs at row " + itoa(i), false
		case !floatEq(a[i].Interest, b[i].Interest):
			return "Interest differs at row " + itoa(i), false
		case !floatEq(a[i].Principal, b[i].Principal):
			return "Principal differs at row " + itoa(i), false
		case !floatEq(a[i].IntToDate, b[i].IntToDate):
			return "IntToDate differs at row " + itoa(i), false
		}
	}
	return "", true
}

func floatEq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// runThrice calls Amortize three times on the same input and fails if
// any run diverges from the first.
func runThrice(t *testing.T, name string, in LoanInput) {
	t.Helper()
	first := Amortize(in)
	if first.Err != nil {
		t.Fatalf("%s: first Amortize errored: %v", name, first.Err)
	}
	for call := 2; call <= 3; call++ {
		r := Amortize(in)
		if r.Err != nil {
			t.Fatalf("%s: call %d errored: %v", name, call, r.Err)
		}
		if msg, ok := schedulesEqual(first.Schedule, r.Schedule); !ok {
			t.Errorf("%s: call %d schedule differs from call 1 (%s): "+
				"len1=%d len%d=%d", name, call, msg, len(first.Schedule), call, len(r.Schedule))
		}
		if !floatEq(first.FinalPrinc, r.FinalPrinc) {
			t.Errorf("%s: call %d FinalPrinc=%.6f differs from call 1 FinalPrinc=%.6f",
				name, call, r.FinalPrinc, first.FinalPrinc)
		}
		if !floatEq(first.TotalPaid, r.TotalPaid) {
			t.Errorf("%s: call %d TotalPaid=%.6f differs from call 1 TotalPaid=%.6f",
				name, call, r.TotalPaid, first.TotalPaid)
		}
		if !floatEq(first.TotalInt, r.TotalInt) {
			t.Errorf("%s: call %d TotalInt=%.6f differs from call 1 TotalInt=%.6f",
				name, call, r.TotalInt, first.TotalInt)
		}
	}
}

func baseFancyInput() LoanInput {
	return LoanInput{
		Loan:     mkFancyLoan(200_000, 0.06, 120, 0),
		Settings: fancyTestSettings(),
		Fancy:    true,
	}
}

func TestIdempotent_PlainLoan(t *testing.T) {
	in := LoanInput{
		Loan:     mkFancyLoan(200_000, 0.06, 120, 0),
		Settings: fancyTestSettings(),
		Fancy:    false,
	}
	runThrice(t, "plain", in)
}

func TestIdempotent_SinglePrepayment(t *testing.T) {
	in := baseFancyInput()
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       newDate(2024, time.March, 1),
		StopDateStatus:  types.InOutInput,
		StopDate:        newDate(2027, time.March, 1),
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         150,
	}}
	runThrice(t, "single-prepay", in)
}

func TestIdempotent_MultiplePrepayments(t *testing.T) {
	in := baseFancyInput()
	in.Prepayments = []Prepayment{
		{
			StartDateStatus: types.InOutInput,
			StartDate:       newDate(2024, time.March, 1),
			StopDateStatus:  types.InOutInput,
			StopDate:        newDate(2026, time.March, 1),
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         100,
		},
		{
			StartDateStatus: types.InOutInput,
			StartDate:       newDate(2027, time.January, 1),
			NNStatus:        types.InOutInput,
			NN:              12,
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         200,
		},
	}
	runThrice(t, "multi-prepay", in)
}

func TestIdempotent_NNBoundedPrepayment(t *testing.T) {
	in := baseFancyInput()
	// NN bound, no stop date: exercises the prepayApplied counter path
	// and the NextDate cursor together.
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       newDate(2024, time.February, 1),
		NNStatus:        types.InOutInput,
		NN:              24,
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         300,
	}}
	runThrice(t, "nn-prepay", in)
}

func TestIdempotent_Balloon(t *testing.T) {
	in := baseFancyInput()
	in.Balloons = []BalloonPayment{{
		DateStatus:   types.InOutInput,
		Date:         newDate(2028, time.January, 1),
		AmountStatus: types.InOutInput,
		Amount:       20_000,
	}}
	runThrice(t, "balloon", in)
}

func TestIdempotent_Adjustment(t *testing.T) {
	in := baseFancyInput()
	in.Adjustments = []RateAdjustment{{
		DateStatus:     types.InOutInput,
		Date:           newDate(2026, time.January, 1),
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.045,
	}}
	runThrice(t, "adjustment", in)
}

func TestIdempotent_SkipMonths(t *testing.T) {
	monthSet, err := MonthSetFromString("6-8")
	if err != nil {
		t.Fatalf("MonthSetFromString: %v", err)
	}
	in := baseFancyInput()
	in.SkipMonths = SkipMonths{
		SkipStatus: types.InOutInput,
		SkipStr:    "6-8",
		MonthSet:   monthSet,
	}
	runThrice(t, "skip-months", in)
}

func TestIdempotent_Moratorium(t *testing.T) {
	in := baseFancyInput()
	in.Moratorium = Moratorium{
		FirstRepayStatus: types.InOutInput,
		FirstRepay:       newDate(2025, time.January, 1),
	}
	runThrice(t, "moratorium", in)
}

func TestIdempotent_StackedOptions(t *testing.T) {
	monthSet, _ := MonthSetFromString("12")
	in := baseFancyInput()
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       newDate(2024, time.February, 1),
		StopDateStatus:  types.InOutInput,
		StopDate:        newDate(2029, time.February, 1),
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         100,
	}}
	in.Balloons = []BalloonPayment{{
		DateStatus:   types.InOutInput,
		Date:         newDate(2030, time.January, 1),
		AmountStatus: types.InOutInput,
		Amount:       15_000,
	}}
	in.Adjustments = []RateAdjustment{{
		DateStatus:     types.InOutInput,
		Date:           newDate(2027, time.January, 1),
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.05,
	}}
	in.SkipMonths = SkipMonths{
		SkipStatus: types.InOutInput,
		SkipStr:    "12",
		MonthSet:   monthSet,
	}
	runThrice(t, "stacked", in)
}

// TestIdempotent_UnknownPrepaymentSolve covers the path where Amortize
// solves an unknown prepayment amount (mutating the input to record
// it). The solved value is a fixed point, so repeated calls must still
// produce the same schedule.
func TestIdempotent_UnknownPrepaymentSolve(t *testing.T) {
	in := baseFancyInput()
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       newDate(2024, time.March, 1),
		StopDateStatus:  types.InOutInput,
		StopDate:        newDate(2028, time.March, 1),
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		// Payment intentionally blank -> Amortize solves it.
	}}
	runThrice(t, "unknown-prepay-solve", in)
}

// TestIdempotent_BackwardSolveAmount pins that solving for the loan
// amount on the same fancy input returns the same value each time.
func TestIdempotent_BackwardSolveAmount(t *testing.T) {
	mk := func() LoanInput {
		in := LoanInput{
			Loan:     mkFancyLoan(0, 0.06, 360, 1199.10),
			Settings: fancyTestSettings(),
			Fancy:    true,
			Prepayments: []Prepayment{{
				StartDateStatus: types.InOutInput,
				StartDate:       newDate(2024, time.February, 1),
				StopDateStatus:  types.InOutInput,
				StopDate:        newDate(2029, time.February, 1),
				PerYrStatus:     types.InOutInput,
				PerYr:           12,
				PaymentStatus:   types.InOutInput,
				Payment:         100,
			}},
		}
		in.Loan.AmountStatus = types.StatusEmpty
		return in
	}
	var prev float64
	for call := 1; call <= 3; call++ {
		solved, _, err := SolveLoanAmount(mk())
		if err != nil {
			t.Fatalf("call %d: %v", call, err)
		}
		if call == 1 {
			prev = solved
			continue
		}
		if !floatEq(prev, solved) {
			t.Errorf("call %d solved=%.6f differs from call 1 solved=%.6f", call, solved, prev)
		}
	}
}
