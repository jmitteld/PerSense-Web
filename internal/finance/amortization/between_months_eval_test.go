package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Reconstructed test for the skip-months "between months" range
// evaluation. The original file was lost to on-disk corruption (it
// was committed as NUL bytes), so this rebuilds coverage from the
// current engine behavior in two layers:
//
//  1. Parse-level: MonthSetFromString turns a range string like
//     "6-8" into the correct month membership, including wrap-around
//     and multi-segment strings. Unlike the lighter TestMonthSetFromString,
//     this asserts both which months ARE set and which are NOT, so a
//     range that over- or under-fills is caught.
//  2. Engine-level: a fancy schedule built with those skip months
//     actually zeroes the payment in every skipped month and leaves
//     non-skipped months paying.
//
// Range parsing is ported from legacy/source/Amortize.pas:
// function MonthSetFromString; the engine skip application lives in
// engine.go: generateFancySchedule (the input.SkipMonths.MonthSet
// check).

// expectMonthSet asserts that exactly the months in `want` are set in
// the [13]bool month set (index 0 is unused; months are 1-12).
func expectMonthSet(t *testing.T, input string, got [13]bool, want []int) {
	t.Helper()
	wantMap := make(map[int]bool, len(want))
	for _, m := range want {
		wantMap[m] = true
	}
	for m := 1; m <= 12; m++ {
		if got[m] && !wantMap[m] {
			t.Errorf("MonthSetFromString(%q): month %d set but should not be", input, m)
		}
		if !got[m] && wantMap[m] {
			t.Errorf("MonthSetFromString(%q): month %d not set but should be", input, m)
		}
	}
}

// TestBetweenMonthsRangeMembership checks exact membership for range
// and list strings, including the wrap-around case where the end
// month is earlier than the start.
func TestBetweenMonthsRangeMembership(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"6-8", []int{6, 7, 8}},                                // simple between-range
		{"1-3", []int{1, 2, 3}},                                // range at the start of the year
		{"10-12", []int{10, 11, 12}},                           // range at the end of the year
		{"7-7", []int{7}},                                      // degenerate range = single month
		{"1-12", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}}, // whole year
		{"10-2", []int{10, 11, 12, 1, 2}},                      // wrap-around: Oct..Feb
		{"11-1", []int{11, 12, 1}},                             // wrap-around spanning year end
		{"6-8,12", []int{6, 7, 8, 12}},                         // range plus a single month
		{"1,3,5", []int{1, 3, 5}},                              // plain list, no range
		{"3-5,9-11", []int{3, 4, 5, 9, 10, 11}},                // two ranges
		{"6", []int{6}},                                        // single month
		{"", nil},                                              // empty -> nothing set
	}
	for _, tt := range tests {
		got, err := MonthSetFromString(tt.input)
		if err != nil {
			t.Errorf("MonthSetFromString(%q) unexpected error: %v", tt.input, err)
			continue
		}
		expectMonthSet(t, tt.input, got, tt.want)
	}
}

// TestBetweenMonthsParseErrors covers the two documented failure
// modes: an out-of-range month number and a range dash with no
// starting month.
func TestBetweenMonthsParseErrors(t *testing.T) {
	tests := []string{
		"13",   // month out of range (> 12)
		"6,13", // valid month then out-of-range
		"0-3",  // zero is out of range
		"-8",   // dash with no starting month
	}
	for _, input := range tests {
		if _, err := MonthSetFromString(input); err == nil {
			t.Errorf("MonthSetFromString(%q): expected an error, got nil", input)
		}
	}
}

// TestBetweenMonthsEngineSkipsPayments drives a fancy amortization
// with skip months "6-8" and verifies the engine zeroes the payment
// on exactly those months while continuing to pay every other month.
// This confirms the between-months range flows all the way through to
// the generated schedule, not just the parser.
func TestBetweenMonthsEngineSkipsPayments(t *testing.T) {
	monthSet, err := MonthSetFromString("6-8")
	if err != nil {
		t.Fatalf("MonthSetFromString: %v", err)
	}

	in := LoanInput{
		Loan: Loan{
			AmountStatus:   types.InOutInput,
			Amount:         120_000,
			LoanDateStatus: types.InOutInput,
			LoanDate:       newDate(2000, time.January, 1),
			LoanRateStatus: types.InOutInput,
			LoanRate:       0.06,
			FirstStatus:    types.InOutInput,
			FirstDate:      newDate(2000, time.February, 1),
			NStatus:        types.InOutInput,
			NPeriods:       36,
			PerYrStatus:    types.InOutInput,
			PerYr:          12,
		},
		SkipMonths: SkipMonths{
			SkipStatus: types.InOutInput,
			SkipStr:    "6-8",
			MonthSet:   monthSet,
		},
		Settings: helpSettings(),
		Fancy:    true,
	}

	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize: %v", r.Err)
	}

	skipped, paid := 0, 0
	for _, p := range r.Schedule {
		if p.PayNum < 1 {
			continue // skip the settlement-stub row
		}
		m := p.Date.Time.Month()
		inSkipRange := m >= time.June && m <= time.August
		if inSkipRange {
			if p.PayAmt > 0.01 {
				t.Errorf("payment %d on %s (month %d) is in skip range 6-8 "+
					"but PayAmt=%.2f; expected 0", p.PayNum, p.Date.Time.Format("2006-01-02"), m, p.PayAmt)
			}
			skipped++
		} else {
			if p.PayAmt <= 0.01 {
				t.Errorf("payment %d on %s (month %d) is outside skip range "+
					"but PayAmt=%.2f; expected a positive payment", p.PayNum, p.Date.Time.Format("2006-01-02"), m, p.PayAmt)
			}
			paid++
		}
	}

	// Sanity: a 36-month loan starting Feb 2000 covers three full
	// Jun-Aug windows, so we expect skipped months to have been seen.
	if skipped == 0 {
		t.Fatalf("expected some Jun-Aug payments to be skipped, saw none (schedule len=%d)", len(r.Schedule))
	}
	if paid == 0 {
		t.Fatalf("expected some non-skipped payments, saw none")
	}
	t.Logf("skip 6-8 over 36 months: %d skipped rows, %d paid rows", skipped, paid)
}
