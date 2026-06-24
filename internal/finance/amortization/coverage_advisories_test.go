package amortization

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// coverage_advisories_test.go targets the result-advisory arms (A-W4,
// A-W7), the empty-row continue guards in ValidateInputs, the
// balance/date lookup edge branches, and hasAnyAdvancedOption's option
// arms.

func aw4BaseLoan() Loan {
	return Loan{
		AmountStatus:   types.InOutInput,
		Amount:         200000,
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.06,
		PayAmtStatus:   types.InOutInput,
		PayAmt:         1199.10,
		NStatus:        types.InOutInput,
		NPeriods:       360,
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		LoanDateStatus: types.InOutInput,
		LoanDate:       types.NewDateRec(2024, time.January, 1),
		FirstStatus:    types.InOutInput,
		FirstDate:      types.NewDateRec(2024, time.February, 1),
		LastOK:         true,
		LastDate:       types.NewDateRec(2054, time.January, 1),
	}
}

func hasAdvisory(w []string, code string) bool {
	for _, s := range w {
		if strings.Contains(s, code) {
			return true
		}
	}
	return false
}

// TestAdvisoryW4ZeroTargetBalloon: an over-paying loan with a target
// balloon (date only) at the last payment solves the balloon amount to
// essentially zero, firing the A-W4 advisory (advisories.go:58-61).
func TestAdvisoryW4ZeroTargetBalloon(t *testing.T) {
	loan := aw4BaseLoan()
	loan.PayAmt = 1500 // over-pays, so by the last date the balloon needed is ~0
	in := LoanInput{
		Loan:     loan,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
		Fancy:    true,
		Balloons: []BalloonPayment{{
			DateStatus: types.InOutInput,
			Date:       types.NewDateRec(2054, time.January, 1),
		}},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize: %v", res.Err)
	}
	if !hasAdvisory(res.Warnings, "A-W4") {
		t.Errorf("expected A-W4 zero-target-balloon advisory, warnings=%v", res.Warnings)
	}
}

// TestAdvisoryW7ZeroPrepayment: an additive (PlusRegular) loan whose
// regular payment already retires it, with an unknown prepayment series,
// solves the prepayment to ~0 and fires A-W7 (advisories.go:66-70).
func TestAdvisoryW7ZeroPrepayment(t *testing.T) {
	in := LoanInput{
		Loan:     aw4BaseLoan(),
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy:    true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       types.NewDateRec(2025, time.January, 1),
			NNStatus:        types.InOutInput,
			NN:              12,
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			// PaymentStatus empty -> unknown amount
		}},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize: %v", res.Err)
	}
	if !hasAdvisory(res.Warnings, "A-W7") {
		t.Errorf("expected A-W7 zero-prepayment advisory, warnings=%v", res.Warnings)
	}
}

// TestValidateEmptyAdvancedRows covers the empty-row continue guards in
// ValidateInputs (validate.go:97 adjustment, :125 balloon): an empty
// adjustment row and an empty balloon row carried alongside a real one
// are skipped without error.
func TestValidateEmptyAdvancedRows(t *testing.T) {
	in := LoanInput{
		Loan:     aw4BaseLoan(),
		Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
		Fancy:    true,
		Adjustments: []RateAdjustment{
			{Date: types.UnknownDate()}, // empty -> DateStatus StatusEmpty
			{
				DateStatus:     types.InOutInput,
				Date:           types.NewDateRec(2030, time.January, 1),
				LoanRateStatus: types.InOutInput,
				LoanRate:       0.05,
			},
		},
		Balloons: []BalloonPayment{
			{Date: types.UnknownDate()}, // empty -> skipped
			{
				DateStatus:   types.InOutInput,
				Date:         types.NewDateRec(2035, time.January, 1),
				AmountStatus: types.InOutInput,
				Amount:       1000,
			},
		},
	}
	if err := ValidateInputs(&in); err != nil {
		t.Fatalf("ValidateInputs with empty rows: %v", err)
	}
}

// TestBalanceAtDateClampsNegative covers the bal<0 clamp in BalanceAtDate
// (engine.go:1906): a schedule whose final recorded principal is slightly
// negative reads back as zero, never negative.
func TestBalanceAtDateClampsNegative(t *testing.T) {
	sched := []PaymentRecord{
		{PayNum: 1, Date: types.NewDateRec(2024, time.February, 1), Principal: 1000},
		{PayNum: 2, Date: types.NewDateRec(2024, time.March, 1), Principal: -0.5},
	}
	got := BalanceAtDate(sched, 2000, types.NewDateRec(2024, time.April, 1))
	if got != 0 {
		t.Errorf("BalanceAtDate with negative final principal = %.4f, want clamped 0", got)
	}
}

// TestDateForBalanceSkipsStubRow covers the PayNum<1 settlement-stub skip
// in DateForBalance (engine.go:1921): a leading stub row whose principal
// is already at/below the target must not be returned.
func TestDateForBalanceSkipsStubRow(t *testing.T) {
	sched := []PaymentRecord{
		{PayNum: 0, Date: types.NewDateRec(2024, time.January, 15), Principal: 50}, // stub, below target
		{PayNum: 1, Date: types.NewDateRec(2024, time.February, 1), Principal: 80},
		{PayNum: 2, Date: types.NewDateRec(2024, time.March, 1), Principal: 40},
	}
	got, ok := DateForBalance(sched, 50)
	if !ok {
		t.Fatalf("DateForBalance: expected a hit")
	}
	if dateComp := dateForBalanceMonth(got); dateComp != time.March {
		t.Errorf("DateForBalance returned stub/early row (month %v), want the March regular row", dateComp)
	}
}

func dateForBalanceMonth(d types.DateRec) time.Month { return d.Time.Month() }

// TestDateForBalanceNeverReached covers the false return when the balance
// never falls to the target.
func TestDateForBalanceNeverReached(t *testing.T) {
	sched := []PaymentRecord{
		{PayNum: 1, Date: types.NewDateRec(2024, time.February, 1), Principal: 80},
		{PayNum: 2, Date: types.NewDateRec(2024, time.March, 1), Principal: 60},
	}
	if _, ok := DateForBalance(sched, 1); ok {
		t.Errorf("DateForBalance: expected miss when balance never reaches target")
	}
}

// TestHasAnyAdvancedOption exercises each arm of hasAnyAdvancedOption
// (engine.go:1064).
func TestHasAnyAdvancedOption(t *testing.T) {
	if hasAnyAdvancedOption(LoanInput{}) {
		t.Errorf("empty input should report no advanced option")
	}
	cases := []struct {
		name string
		in   LoanInput
	}{
		{"balloon", LoanInput{Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2030, time.January, 1)}}}},
		{"prepay", LoanInput{Prepayments: []Prepayment{{StartDateStatus: types.InOutInput}}}},
		{"adjust", LoanInput{Adjustments: []RateAdjustment{{DateStatus: types.InOutInput}}}},
		{"target", LoanInput{Target: Target{TargetStatus: types.InOutInput, TargetValue: 100}}},
		{"morat", LoanInput{Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2030, time.January, 1)}}},
	}
	for _, c := range cases {
		if !hasAnyAdvancedOption(c.in) {
			t.Errorf("%s: hasAnyAdvancedOption = false, want true", c.name)
		}
	}
	skipset := [13]bool{}
	skipset[6] = true
	if !hasAnyAdvancedOption(LoanInput{SkipMonths: SkipMonths{MonthSet: skipset}}) {
		t.Errorf("skip-months: hasAnyAdvancedOption = false, want true")
	}
}

var _ = math.Abs
