package presentvalue

// Error-message coverage for the present-value engine.
//
// Each test below builds a PVInput that drives the engine into one of
// the "this calculation cannot be done" conditions — under-determined,
// over-determined, inconsistent (sign mismatch / dates out of order),
// or a non-convergent solver — and asserts both that an error is
// returned and that its message contains the user-facing key phrase
// and an actionable suggestion.
//
// Helpers reused from other _test.go files in this package:
//   newDate / pvDate / dateOf  — build a DateRec
//   defaultSettings            — fixed-rate PVSettings
//   vrTestSettings             — variable-rate PVSettings
//   actuarialTestCfg           — a small life-contingency config

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// errContains fails the test unless err is non-nil and its message
// contains every supplied phrase.
func errContains(t *testing.T, err error, phrases ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	msg := err.Error()
	for _, p := range phrases {
		if !strings.Contains(msg, p) {
			t.Errorf("error message missing %q\n  got: %s", p, msg)
		}
	}
}

// --- Infinite-series: periodic payment forever with rate <= COLA ---

func TestErrInfiniteSeriesRateBelowCOLA(t *testing.T) {
	settings := defaultSettings()
	asOf := newDate(2024, time.January, 1)
	from := newDate(2024, time.January, 1)
	to := types.LatestDate() // "forever"
	// COLA (0.04) above rate (0.02): the geometric series diverges.
	_, err := PeriodicSummation(0.02, 0.04, asOf, from, to, 12, 1000, &settings)
	errContains(t, err, "infinite present value", "To Date", "raise the Rate")
}

// Note: the "more than one missing field" (over-determined) dispatch
// arm in Calculate is unreachable from the public API — FirstPass's
// Frontward scan disqualifies any row that contains an unknown, so
// Frontward and Backward can never both be true. That message is
// already exercised by TestCanaryC17 in canary_ambiguous_errors_test.go.

// --- Under-determined: not enough inputs to solve at all ---

func TestErrNotEnoughInputs(t *testing.T) {
	// Rate is blank, As-of present, no rows. Neither forward nor
	// backward dispatch can fire.
	input := PVInput{
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput,
			AsOf:       newDate(2024, time.January, 1),
			// Rate and SumValue blank.
		},
		LumpSums: []LumpSumPayment{
			{DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1)},
		},
		Settings: defaultSettings(),
	}
	res := Calculate(input)
	errContains(t, res.Err, "not enough inputs to solve", "Rate", "As-of Date")
}

// --- Forward path with no rate / as-of date ---

func TestErrForwardNeedsRateAndAsOf(t *testing.T) {
	res := forwardOnly(PVInput{
		LumpSums: []LumpSumPayment{
			{
				DateStatus: types.InOutInput, Date: newDate(2025, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 1000,
			},
		},
		Settings: defaultSettings(),
	})
	errContains(t, res.Err, "without both a Rate and an As-of Date")
}

// --- BackwardCalc with a non-backward FirstPass result ---

func TestErrBackwardCalcInsufficientData(t *testing.T) {
	// fp.Backward is false: no SumValue supplied.
	fp := &FirstPassResult{Backward: false}
	res := BackwardCalc(PVInput{Settings: defaultSettings()}, fp)
	errContains(t, res.Err, "not enough information on the screen",
		"leave exactly one field blank")
}

// --- Single-payment row: only the Value filled in ---

func TestErrLumpOnlyValueGiven(t *testing.T) {
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{ValStatus: types.InOutInput, Val: 5000},
		},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			AsOfStatus:     types.InOutInput,
			AsOf:           newDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       5000,
		},
		Settings: defaultSettings(),
	}
	res := FirstPass(&input)
	errContains(t, res.Err, "single payment line 1",
		"only a Value filled in", "Date or the Amount")
}

// Note: the periodic "does not have enough filled in" arm requires a
// contains_unknown row (exactly one missing field) that nonetheless
// lacks both a date and the amount — an impossible field combination,
// so that arm is unreachable. The lump-sum analogue IS reachable and
// is covered by TestErrLumpOnlyValueGiven above.

// --- Single-payment date solve: residual Value vs Amount sign clash ---

func TestErrLumpDateSignMismatch(t *testing.T) {
	// One lump row with only the Amount filled in -> contains_unknown,
	// dispatches to solveLumpDate (PV-2). The residual value (the
	// screen Sum Value minus other rows) is negative while the Amount
	// is positive, so no date can reconcile them.
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{AmtStatus: types.InOutInput, Amt: 10000},
		},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			AsOfStatus:     types.InOutInput,
			AsOf:           newDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       -8000,
		},
		Settings: defaultSettings(),
	}
	res := Calculate(input)
	errContains(t, res.Err, "single payment line 1", "opposite", "agree in sign")
}

// --- Single-payment date solve on a life-contingent row ---

func TestErrLumpDateLifeContingent(t *testing.T) {
	asOf := dateOf(2024, time.January, 1)
	dob := dateOf(1959, time.January, 1)
	cfg := actuarialTestCfg(asOf, dob)

	input := PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus:     types.InOutInput,
			AsOf:           asOf,
			R:              RateEntry{Status: types.InOutInput, Rate: 0.05},
			SumValueStatus: types.InOutInput,
			SumValue:       60000,
		},
		LumpSums: []LumpSumPayment{
			{
				// Amount only; Date blank -> solveLumpDate (PV-2).
				AmtStatus: types.InOutInput, Amt: 100000,
				Act: actuarial.Living,
			},
		},
		Actuarial: cfg,
	}
	res := Calculate(input)
	errContains(t, res.Err, "life-contingency payment", "solve for the Amount instead")
}

// --- Periodic date solve: Value and Amount opposite signs ---

func TestErrPeriodicDateSignMismatch(t *testing.T) {
	input := PVInput{
		Periodics: []PeriodicPayment{
			{
				FromDateStatus: types.InOutInput, FromDate: newDate(2025, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				// Amount positive, Value negative -> sign mismatch.
				AmtStatus: types.InOutInput, Amt: 1000,
				ValStatus: types.InOutInput, Val: -50000,
			},
		},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			AsOfStatus:     types.InOutInput,
			AsOf:           newDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       -50000,
		},
		Settings: defaultSettings(),
	}
	res := Calculate(input)
	errContains(t, res.Err, "periodic payment line 1", "opposite", "match")
}

// --- As-of date solve with a near-zero rate ---

func TestErrAsOfRateTooSmall(t *testing.T) {
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{
				DateStatus: types.InOutInput, Date: newDate(2030, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 10000,
			},
		},
		PresVal: PresValLine{
			// As-of blank -> solve for it; rate ~0.
			R:              RateEntry{Status: types.InOutInput, Rate: 0},
			SumValueStatus: types.InOutInput,
			SumValue:       9000,
		},
		Settings: defaultSettings(),
	}
	res := Calculate(input)
	errContains(t, res.Err, "As-of Date", "Rate is too small")
}

// --- Rate solve that cannot reach the target Present Value ---

func TestErrRateDoesNotConverge(t *testing.T) {
	// All payments are positive, but the target Present Value is
	// negative. No interest rate can turn a stream of positive cash
	// into a negative present value, so the PV-8 rate solver runs out
	// of iterations without converging.
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{
				DateStatus: types.InOutInput, Date: newDate(2034, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 10000,
			},
		},
		PresVal: PresValLine{
			// Rate blank -> solve for it (PV-8).
			AsOfStatus:     types.InOutInput,
			AsOf:           newDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       -50000,
		},
		Settings: defaultSettings(),
	}
	res := Calculate(input)
	errContains(t, res.Err, `"rate" computation did not converge`,
		"fill in the Rate")
}

// --- Variable-rate mode: a row is missing a required field ---

func TestErrVariableRateMissingLumpField(t *testing.T) {
	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
		{Date: dateOf(2030, time.January, 1), Rate: 0.07},
	}
	input := PVInput{
		Settings:     vrTestSettings(),
		RateSchedule: schedule,
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: dateOf(2024, time.January, 1),
		},
		LumpSums: []LumpSumPayment{
			// Date present, Amount blank — and no SumValue, so this is
			// not a VR backward solve either.
			{DateStatus: types.InOutInput, Date: dateOf(2030, time.January, 1)},
		},
	}
	res := Calculate(input)
	errContains(t, res.Err, "single payment line 1",
		"variable-rate schedule", "Date and the Amount")
}

func TestErrVariableRateMissingAsOf(t *testing.T) {
	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
	}
	input := PVInput{
		Settings:     vrTestSettings(),
		RateSchedule: schedule,
		// As-of date deliberately omitted.
		LumpSums: []LumpSumPayment{
			{
				DateStatus: types.InOutInput, Date: dateOf(2030, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 1000,
			},
		},
	}
	res := Calculate(input)
	errContains(t, res.Err, "variable-rate present value", "As-of Date")
}

func TestErrVariableRatePeriodicBadPerYr(t *testing.T) {
	schedule := []RateLine{
		{Date: dateOf(1900, time.January, 1), Rate: 0.04},
	}
	input := PVInput{
		Settings:     vrTestSettings(),
		RateSchedule: schedule,
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: dateOf(2024, time.January, 1),
		},
		Periodics: []PeriodicPayment{
			{
				FromDateStatus: types.InOutInput, FromDate: dateOf(2025, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: dateOf(2030, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 500,
				// PerYr left at zero.
			},
		},
	}
	res := Calculate(input)
	errContains(t, res.Err, "periodic payment line 1", "Pmts-Yr")
}
