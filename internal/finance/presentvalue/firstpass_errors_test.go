package presentvalue

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func pvDate(y int, m time.Month, d int) types.DateRec {
	return types.NewDateRec(y, m, d)
}

// C-P-2: a lump-sum row with only amt=0 supplied (val and date blank)
// is unsolvable — DOS records this as an amount-column error.
func TestFirstPassLumpSumZeroAmountError(t *testing.T) {
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{AmtStatus: types.InOutInput, Amt: 0},
		},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			AsOfStatus:     types.InOutInput,
			AsOf:           pvDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       1000,
		},
	}
	res := FirstPass(&input)
	if res.Err == nil ||
		!strings.Contains(res.Err.Error(), "amount cannot be zero") {
		t.Errorf("expected zero-amount error, got %v", res.Err)
	}
}

// C-P-3: a lump-sum row with only val=0 supplied (amt and date blank)
// is unsolvable.
func TestFirstPassLumpSumZeroValueError(t *testing.T) {
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{ValStatus: types.InOutInput, Val: 0},
		},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			AsOfStatus:     types.InOutInput,
			AsOf:           pvDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       1000,
		},
	}
	res := FirstPass(&input)
	if res.Err == nil ||
		!strings.Contains(res.Err.Error(), "value cannot be zero") {
		t.Errorf("expected zero-value error, got %v", res.Err)
	}
}

// C-P-4: a lump-sum row with all three of {date, amount, value}
// supplied is over-specified. Per dispatch_gaps §4.7 PV-warning this
// is a soft, non-fatal warning (DOS PRESVALU.pas:1166-1189 records a
// cancelable warning) — NOT a hard error. FirstPass should record a
// warning, classify the row as fully specified, and proceed.
func TestFirstPassLumpSumOverSpecifiedWarns(t *testing.T) {
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{
				DateStatus: types.InOutInput, Date: pvDate(2030, time.January, 1),
				AmtStatus: types.InOutInput, Amt: 1000,
				ValStatus: types.InOutInput, Val: 800,
			},
		},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			AsOfStatus:     types.InOutInput,
			AsOf:           pvDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       1000,
		},
	}
	res := FirstPass(&input)
	if res.Err != nil {
		t.Errorf("over-specified row should be a warning, not an error; got %v", res.Err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "over-specified") {
		t.Errorf("expected one over-specified warning, got %v", res.Warnings)
	}
	if res.LumpSumStatus[0] != types.LineFullySpecified {
		t.Errorf("over-specified lump row should classify as fully specified, got %d",
			res.LumpSumStatus[0])
	}
}

// C-P-4 (periodic): a periodic row with all four of {fromDate, toDate,
// amount, value} supplied is over-specified. Same treatment as the
// lump-sum case — a soft warning, not a hard error.
func TestFirstPassPeriodicOverSpecifiedWarns(t *testing.T) {
	input := PVInput{
		Periodics: []PeriodicPayment{
			{
				FromDateStatus: types.InOutInput, FromDate: pvDate(2025, time.January, 1),
				ToDateStatus: types.InOutInput, ToDate: pvDate(2030, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 100,
				ValStatus: types.InOutInput, Val: 5000,
			},
		},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			AsOfStatus:     types.InOutInput,
			AsOf:           pvDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       10000,
		},
	}
	res := FirstPass(&input)
	if res.Err != nil {
		t.Errorf("over-specified periodic row should be a warning, not an error; got %v", res.Err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "over-specified") {
		t.Errorf("expected one over-specified warning, got %v", res.Warnings)
	}
}

// C-P-2 (periodic): contains_unknown periodic row with amt=0 supplied
// is unsolvable.
func TestFirstPassPeriodicZeroAmountError(t *testing.T) {
	input := PVInput{
		Periodics: []PeriodicPayment{
			{
				FromDateStatus: types.InOutInput, FromDate: pvDate(2025, time.January, 1),
				PerYrStatus: types.InOutInput, PerYr: 12,
				AmtStatus: types.InOutInput, Amt: 0,
			},
		},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			AsOfStatus:     types.InOutInput,
			AsOf:           pvDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       10000,
		},
	}
	res := FirstPass(&input)
	if res.Err == nil ||
		!strings.Contains(res.Err.Error(), "amount cannot be zero") {
		t.Errorf("expected zero-amount periodic error, got %v", res.Err)
	}
}

// Sanity: a well-formed lump-sum contains_unknown row with date only
// supplied (no amt, no val) should NOT trip any of the new errors.
func TestFirstPassWellFormedContainsUnknownPasses(t *testing.T) {
	input := PVInput{
		LumpSums: []LumpSumPayment{
			{DateStatus: types.InOutInput, Date: pvDate(2030, time.January, 1)},
		},
		PresVal: PresValLine{
			R:              RateEntry{Status: types.InOutInput, Rate: 0.06},
			AsOfStatus:     types.InOutInput,
			AsOf:           pvDate(2024, time.January, 1),
			SumValueStatus: types.InOutInput,
			SumValue:       1000,
		},
	}
	res := FirstPass(&input)
	if res.Err != nil {
		t.Errorf("well-formed row should not error, got %v", res.Err)
	}
	if !res.Backward || res.BackwardKind != BackwardLumpAmount {
		t.Errorf("expected BackwardLumpAmount, got kind=%d, backward=%v",
			res.BackwardKind, res.Backward)
	}
}
