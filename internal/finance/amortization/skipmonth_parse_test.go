package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Skip-month string parsing robustness and date-cursor edge cases,
// complementing between_months_eval_test.go (which covers the core
// range/wrap/error membership). These focus on messy real-world input
// (stray separators, whitespace, doubled dashes) and on a prepayment
// cursor that starts on a day-of-month that not every month has.

// TestSkipParse_OddSeparators: the DOS parser scans for digits and
// dashes and ignores everything else, so commas, spaces, semicolons
// and doubled separators should all be tolerated and yield the same
// month set as the clean form.
func TestSkipParse_OddSeparators(t *testing.T) {
	clean, err := MonthSetFromString("3,6,9")
	if err != nil {
		t.Fatalf("clean parse: %v", err)
	}
	variants := []string{"3, 6, 9", "3;6;9", " 3 , 6 , 9 ", "3,,6,,9", "3 6 9"}
	for _, v := range variants {
		got, err := MonthSetFromString(v)
		if err != nil {
			t.Errorf("MonthSetFromString(%q): unexpected error %v", v, err)
			continue
		}
		if got != clean {
			t.Errorf("MonthSetFromString(%q) = %v, want same as %q = %v", v, got, "3,6,9", clean)
		}
	}
}

// TestSkipParse_DoubledDash: a doubled dash still reads as a range
// (the parser re-arms thruflag), matching the lenient DOS scan.
func TestSkipParse_DoubledDash(t *testing.T) {
	got, err := MonthSetFromString("6--8")
	if err != nil {
		t.Fatalf("6--8: %v", err)
	}
	want, _ := MonthSetFromString("6-8")
	if got != want {
		t.Errorf("MonthSetFromString(%q) = %v, want %v", "6--8", got, want)
	}
}

// TestSkipParse_WhitespaceAndEmpty: blank-ish strings select no months
// and do not error.
func TestSkipParse_WhitespaceAndEmpty(t *testing.T) {
	for _, s := range []string{"", "   ", "\t", " , , "} {
		got, err := MonthSetFromString(s)
		if err != nil {
			t.Errorf("MonthSetFromString(%q): unexpected error %v", s, err)
			continue
		}
		for m := 1; m <= 12; m++ {
			if got[m] {
				t.Errorf("MonthSetFromString(%q): month %d set, want none", s, m)
			}
		}
	}
}

// TestSkipParse_TwoDigitMonths: the 9/10 and 12 boundaries (single vs
// two-digit) must parse to the exact month.
func TestSkipParse_TwoDigitMonths(t *testing.T) {
	cases := map[string]int{"9": 9, "10": 10, "11": 11, "12": 12}
	for s, want := range cases {
		got, err := MonthSetFromString(s)
		if err != nil {
			t.Errorf("MonthSetFromString(%q): %v", s, err)
			continue
		}
		for m := 1; m <= 12; m++ {
			if (m == want) != got[m] {
				t.Errorf("MonthSetFromString(%q): month %d = %v, want %v", s, m, got[m], m == want)
			}
		}
	}
}

// TestSkipParse_Deterministic: parsing is a pure function — same input,
// same output, every time (guards against any hidden shared state).
func TestSkipParse_Deterministic(t *testing.T) {
	for i := 0; i < 5; i++ {
		got, err := MonthSetFromString("10-2,7")
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		want := [13]bool{}
		for _, m := range []int{10, 11, 12, 1, 2, 7} {
			want[m] = true
		}
		if got != want {
			t.Errorf("iter %d: got %v want %v", i, got, want)
		}
	}
}

// TestEdge_PrepaymentStartsOnDay31 exercises the date cursor when a
// monthly prepayment series starts on the 31st: AddPeriod must advance
// across months that have no 31st without skipping or duplicating an
// extra. The schedule must be deterministic and the extras must
// shorten the term relative to no prepayments.
func TestEdge_PrepaymentStartsOnDay31(t *testing.T) {
	base := amortizingInput(t, 200_000, 360)
	withExtra := base
	withExtra.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       newDate(2024, time.January, 31),
		NNStatus:        types.InOutInput,
		NN:              24,
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         500,
	}}
	runThrice(t, "prepay-day31", withExtra)
	if len(Amortize(withExtra).Schedule) >= len(Amortize(base).Schedule) {
		t.Errorf("day-31 prepayments did not shorten the term")
	}
}
