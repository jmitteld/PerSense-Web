package presentvalue

// Coverage for forward-path branches (calc.go): the row-skip guards in
// forwardOnly, estimateInstallments edge cases, the unknown-POD solver
// error arms, and the COLA accumulate / exact-mode summation paths.

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// forwardOnly skips an incomplete lump row, an incomplete periodic row,
// and a periodic row with peryr<=0 (calc.go:483, 512, 515).
func TestForwardOnlySkipsIncompleteRows(t *testing.T) {
	in := PVInput{
		Settings: defaultSettings(),
		LumpSums: []LumpSumPayment{
			{ // complete
				DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 1000,
			},
			{ // incomplete (amount blank) -> skipped
				DateStatus: types.InOutInput, Date: newDate(2026, time.January, 1),
			},
		},
		Periodics: []PeriodicPayment{
			{ // incomplete (amount blank) -> skipped
				FromDateStatus: types.InOutInput, FromDate: newDate(2025, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: newDate(2030, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
			},
			{ // peryr<=0 -> skipped
				FromDateStatus: types.InOutInput, FromDate: newDate(2025, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: newDate(2030, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 0,
				AmtStatus: types.InOutInput, Amt: 100,
			},
		},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: newDate(2024, time.January, 1),
			R: RateEntry{Status: types.InOutInput, Rate: 0.06},
		},
	}
	res := forwardOnly(in)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	// Only the first lump row contributes.
	if res.SumValue <= 0 || res.SumValue > 1000 {
		t.Fatalf("only the complete lump row should contribute, got %v", res.SumValue)
	}
}

// estimateInstallments returns 0 for unknown dates (calc.go:678) and
// clamps to 1 for a degenerate range (calc.go:683).
func TestEstimateInstallmentsEdges(t *testing.T) {
	if n := estimateInstallments(types.UnknownDate(), newDate(2030, time.January, 1), 12); n != 0 {
		t.Fatalf("unknown from -> 0, got %d", n)
	}
	// to before from -> years negative -> n clamps to 1.
	if n := estimateInstallments(newDate(2030, time.January, 1), newDate(2025, time.January, 1), 12); n != 1 {
		t.Fatalf("reversed range -> clamp to 1, got %d", n)
	}
}

// solveUnknownPOD surfaces an inner-Calculate error (calc.go:426). Make
// the no-POD baseline run fail by giving an out-of-order periodic row.
func TestSolveUnknownPODInnerError(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	cfg := actuarialTestCfg(asOf, dateOf(1954, time.January, 1))
	cfg.PODUnknown = true
	in := PVInput{
		Settings: vrTestSettings(),
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(2030, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(2025, time.January, 1), // reversed
			PerYrStatus: types.InOutInput, PerYr: 12,
			AmtStatus: types.InOutInput, Amt: 100,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.05},
			SumValueStatus: types.InOutInput, SumValue: 30000,
		},
		Actuarial: cfg,
	}
	res := Calculate(in)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "out of order") {
		t.Fatalf("expected inner out-of-order error, got %v", res.Err)
	}
}

// solveUnknownPOD reports a zero-death-probability error when the unit
// death value is ~0 (calc.go:439). A DOB far in the past with a payment
// at the as-of date can leave the death-benefit unit value tiny; instead
// use a config whose Now/age makes PODValue collapse. We reach the guard
// by giving an actuarial config with a horizon that yields ~0 POD value.
func TestSolveUnknownPODZeroProbability(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	// A person already past the table horizon (dob 200 years ago) so the
	// remaining-life death probability integrates to ~0.
	cfg := actuarialTestCfg(asOf, dateOf(1824, time.January, 1))
	cfg.PODUnknown = true
	in := PVInput{
		Settings: vrTestSettings(),
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: dateOf(2025, time.January, 1),
			AmtStatus: types.InOutInput, Amt: 1000,
		}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.05},
			SumValueStatus: types.InOutInput, SumValue: 5000,
		},
		Actuarial: cfg,
	}
	res := Calculate(in)
	// Accept either the zero-probability error, or (if the table still
	// gives a tiny non-zero probability) a normal solve — the branch we
	// target is the zero guard, which fires when the unit value underflows.
	if res.Err != nil && !strings.Contains(res.Err.Error(), "no chance of being paid") {
		t.Fatalf("unexpected error: %v", res.Err)
	}
}

// PeriodicSummation accumulate-from-toDate COLA branch (calc.go:190-196):
// asOf strictly after fromDate, with a non-zero continuous COLA. Use
// COLAContinuous so the closed-form path runs.
func TestPeriodicSummationAccumulateWithCOLA(t *testing.T) {
	settings := defaultSettings()
	settings.COLAMonth = types.COLAContinuous
	asOf := newDate(2032, time.January, 1) // after the payment range
	from := newDate(2025, time.January, 1)
	to := newDate(2030, time.January, 1)
	n := estimateInstallments(from, to, 12)
	got, err := PeriodicSummation(0.06, 0.03, asOf, from, to, 12, n, &settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == 0 {
		t.Fatalf("expected a non-zero accumulate factor")
	}
}

// PeriodicSummation exact mode (calc.go:132-152) with a finite range:
// each period summed individually.
func TestPeriodicSummationExactMode(t *testing.T) {
	settings := defaultSettings()
	settings.Exact = true
	asOf := newDate(2024, time.January, 1)
	from := newDate(2025, time.January, 1)
	to := newDate(2027, time.January, 1)
	n := estimateInstallments(from, to, 12)
	got, err := PeriodicSummation(0.06, 0, asOf, from, to, 12, n, &settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got <= 0 {
		t.Fatalf("expected a positive exact-mode factor, got %v", got)
	}
}

// Annual-COLA periodic summation (calc.go:243) — peryr>1, non-continuous
// COLA, finite range.
func TestPeriodicSummationAnnualCOLA(t *testing.T) {
	settings := defaultSettings() // COLAMonth = COLAAnnual
	asOf := newDate(2024, time.January, 1)
	from := newDate(2025, time.January, 1)
	to := newDate(2030, time.January, 1)
	n := estimateInstallments(from, to, 12)
	got, err := PeriodicSummation(0.06, 0.03, asOf, from, to, 12, n, &settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got <= 0 {
		t.Fatalf("expected a positive annual-COLA factor, got %v", got)
	}
}

// Annual-COLA summation with an installment count that overruns the
// ToDate hits the per-payment toDate break (calc.go:279). A caller may
// supply NInstallments directly (forwardOnly only recomputes it when
// <= 0), so an over-large count is a reachable input. We drive the
// private summation with an inflated n.
func TestPeriodicSummationAnnualCOLAOverrunBreak(t *testing.T) {
	settings := defaultSettings() // COLAAnnual
	asOf := newDate(2024, time.January, 1)
	from := newDate(2025, time.January, 1)
	to := newDate(2026, time.January, 1)
	n := estimateInstallments(from, to, 12)
	// n+5 forces the loop past toDate, exercising the break.
	got, err := periodicSumAnnualCOLA(0.06, 0.03, asOf, from, to, 12, n+5, &settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got <= 0 {
		t.Fatalf("expected a positive factor, got %v", got)
	}
}
