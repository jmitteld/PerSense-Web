package amortization

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// baseInput30y returns a vanilla 30-year, 6%, $200,000 loan in fancy mode.
func baseInput30y() LoanInput {
	loan := Loan{
		AmountStatus:   types.InOutInput,
		Amount:         200000,
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.06,
		PayAmtStatus:   types.InOutInput,
		PayAmt:         1199.10, // standard 30y annuity at 6%
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
	return LoanInput{
		Loan: loan,
		Settings: Settings{
			Basis:  types.Basis360,
			PerYr:  12,
			YrDays: 360,
			YrInv:  1.0 / 360,
		},
		Fancy: true,
	}
}

func TestAdvancedPrepaymentReducesPrincipal(t *testing.T) {
	// Without prepayment.
	noPre := baseInput30y()
	noPreResult := Amortize(noPre)
	if noPreResult.Err != nil {
		t.Fatal(noPreResult.Err)
	}

	// With $100/mo extra prepayment for 5 years. These are ADDITIVE extra
	// payments (on top of the regular payment). In DOS that requires the
	// "Balloon includes regular payment" setting ON (plus_regular); the default
	// (off) treats additional periodic payments as a payment SCHEDULE that
	// replaces the regular payment. See docs/prepayment_semantics_finding.md.
	withPre := baseInput30y()
	withPre.Settings.PlusRegular = true
	withPre.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       types.NewDateRec(2024, time.February, 1),
		StopDateStatus:  types.InOutInput,
		StopDate:        types.NewDateRec(2029, time.February, 1),
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         100,
	}}
	withPreResult := Amortize(withPre)
	if withPreResult.Err != nil {
		t.Fatal(withPreResult.Err)
	}

	// Total interest paid should be lower with prepayments.
	if withPreResult.TotalInt >= noPreResult.TotalInt {
		t.Errorf("prepayment should reduce total interest: noPre=%.2f, withPre=%.2f",
			noPreResult.TotalInt, withPreResult.TotalInt)
	}
	// Total paid should differ (more total cash out due to extras, but
	// less interest — net comparison is rate-dependent). The clean
	// invariant is: total interest is strictly lower.
}

func TestAdvancedBalloon(t *testing.T) {
	input := baseInput30y()
	input.Balloons = []BalloonPayment{{
		DateStatus:   types.InOutInput,
		Date:         types.NewDateRec(2026, time.January, 1),
		AmountStatus: types.InOutInput,
		Amount:       50000,
	}}
	res := Amortize(input)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// Find the schedule line on or near 2026-01-01 — payment should
	// reflect the balloon (either replacing or augmenting per
	// PlusRegular).
	found := false
	for _, line := range res.Schedule {
		if line.Date.Time.Year() == 2026 && line.Date.Time.Month() == time.January {
			if line.PayAmt > 40000 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("balloon payment not found in schedule")
	}
}

func TestAdvancedRateAdjustment(t *testing.T) {
	input := baseInput30y()
	// Rate drops from 6% to 4% on year 5.
	input.Adjustments = []RateAdjustment{{
		DateStatus:     types.InOutInput,
		Date:           types.NewDateRec(2029, time.January, 1),
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.04,
	}}
	res := Amortize(input)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// At least the schedule should compute and produce some lines.
	if len(res.Schedule) == 0 {
		t.Fatal("empty schedule")
	}
	// Total interest should be lower than the no-adjustment baseline.
	baseline := Amortize(baseInput30y())
	if res.TotalInt >= baseline.TotalInt {
		t.Errorf("rate-drop adjustment should reduce total interest: "+
			"baseline=%.2f, adjusted=%.2f",
			baseline.TotalInt, res.TotalInt)
	}
}

func TestAdvancedMoratorium(t *testing.T) {
	input := baseInput30y()
	// Moratorium: interest-only until 2026-01-01.
	input.Moratorium = Moratorium{
		FirstRepayStatus: types.InOutInput,
		FirstRepay:       types.NewDateRec(2026, time.January, 1),
	}
	res := Amortize(input)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// During moratorium, principal should not decrease.
	for i, line := range res.Schedule {
		if line.Date.Time.Year() < 2026 && i > 0 {
			prev := res.Schedule[i-1].Principal
			if line.Principal < prev-0.01 {
				t.Errorf("principal dropped during moratorium at %s: %.2f -> %.2f",
					line.Date.Time.Format("2006-01-02"), prev, line.Principal)
				break
			}
		}
	}
}

func TestAdvancedSkipMonths(t *testing.T) {
	input := baseInput30y()
	monthSet, err := MonthSetFromString("6-8")
	if err != nil {
		t.Fatal(err)
	}
	input.SkipMonths = SkipMonths{
		SkipStatus: types.InOutInput,
		SkipStr:    "6-8",
		MonthSet:   monthSet,
	}
	res := Amortize(input)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// Lines in June, July, August should have payment = 0.
	for _, line := range res.Schedule {
		m := line.Date.Time.Month()
		if m == time.June || m == time.July || m == time.August {
			if line.PayAmt > 0.01 {
				t.Errorf("skip-month payment at %s = %.2f, want 0",
					line.Date.Time.Format("2006-01-02"), line.PayAmt)
				break
			}
		}
	}
}

// TestAmortizeTrueRateErrorSurfaced verifies that errors from
// ComputeTrueRate are returned to the caller rather than silently
// dropped. The fix at engine.go:143 converts the previous _,_:= form
// into a real error return.
func TestAmortizeTrueRateErrorSurfaced(t *testing.T) {
	// LoanRate = -50 makes 1 + rate/12 negative, which makes Lnn
	// error inside RateFromYield, which propagates through
	// ComputeTrueRate to Amortize().
	input := baseInput30y()
	input.Loan.LoanRate = -50.0
	res := Amortize(input)
	if res.Err == nil {
		t.Fatal("expected ComputeTrueRate error to surface, got nil")
	}
	if !strings.Contains(res.Err.Error(), "Loan Rate") {
		t.Errorf("expected error to mention the Loan Rate field, got: %v", res.Err)
	}
}

// TestAmortizeMaxIterSafety verifies the oversized-term guard fires for
// pathological inputs where the schedule would otherwise run unbounded.
//
// §55 changed WHICH refusal a 12000-period monthly loan gets, and the change is
// the point of the test now. DOS stores a date's year in a BYTE
// (Globals.pas:46-48), so `lastdate.y := firstdate.y + nyears`
// (INTSUTIL.pas:1402) truncates mod 256: 12000 monthly periods from 2024 is
// 1000 years, and 124+1000 = 1124 -> 100 -> the year 2000. The derived Last Pmt
// Date therefore lands BEFORE the 1st Pmt Date and the screen is refused on
// date order, which is exactly what the real DOS engine does:
//
//	amort_oracle 200000 0.06 12000 12 loandmy=1.1.2024 firstdmy=1.2.2024 \
//	    payhard=0.01
//	  -> ERR There must be at least two regular payments.
//
// (DOS's own check is `DateComp(h^.firstdate, h^.lastdate) >= 0`,
// Amortize.pas:1222; the port's C-A-5 in validate.go is the same check with the
// port's friendlier wording and the documented `> 0` relaxation for the
// degenerate one-payment loan.)
//
// The 10000-payment guard itself is still reachable — just not through a term
// whose wrap puts the last date behind the first. 10001 monthly periods wraps
// to 2089, passes the date-order check, and hits the guard.
func TestAmortizeMaxIterSafetyWrappedTermIsRefusedOnDateOrder(t *testing.T) {
	loan := Loan{
		AmountStatus:   types.InOutInput,
		Amount:         200000,
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.06,
		PayAmtStatus:   types.InOutInput,
		PayAmt:         0.01, // tiny payment — principal never falls
		NStatus:        types.InOutInput,
		NPeriods:       12000, // exceeds the 10000 safety threshold
		PerYrStatus:    types.InOutInput,
		PerYr:          12,
		LoanDateStatus: types.InOutInput,
		LoanDate:       types.NewDateRec(2024, time.January, 1),
		FirstStatus:    types.InOutInput,
		FirstDate:      types.NewDateRec(2024, time.February, 1),
		LastOK:         false, // no early termination on lastdate
	}
	input := LoanInput{
		Loan: loan,
		Settings: Settings{
			Basis:  types.Basis360,
			PerYr:  12,
			YrDays: 360,
			YrInv:  1.0 / 360,
		},
		Fancy: true,
	}
	res := Amortize(input)
	if res.Err == nil {
		t.Fatal("expected the wrapped-term screen to be refused, got nil error")
	}
	if !strings.Contains(res.Err.Error(), "after Last Pmt Date") {
		t.Errorf("expected the date-order refusal DOS gives here, got: %v", res.Err)
	}

	// The 10000-payment guard, reached by a term whose wrap still lands after
	// the first payment date (124+833 = 957 -> 189 -> 2089). It lives at the
	// top of generateSimpleSchedule, so this leg runs the plain (non-fancy)
	// generator.
	loan.NPeriods = 10001
	input.Loan = loan
	input.Fancy = false
	res = Amortize(input)
	if res.Err == nil {
		t.Fatal("expected 10000-payment guard to fire, got nil error")
	}
	if !strings.Contains(res.Err.Error(), "10000") {
		t.Errorf("expected error to mention the 10000-payment limit, got: %v", res.Err)
	}
}

func TestAdvancedTargetForcesMinimumPrincipalReduction(t *testing.T) {
	input := baseInput30y()
	input.Target = Target{
		TargetStatus: types.InOutInput,
		TargetValue:  500, // require >= $500/period principal reduction
	}
	res := Amortize(input)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	// Compare against baseline.
	baseline := Amortize(baseInput30y())
	// With target, total interest should be lower (faster paydown).
	if res.TotalInt >= baseline.TotalInt {
		t.Errorf("target should reduce total interest: baseline=%.2f, "+
			"with-target=%.2f", baseline.TotalInt, res.TotalInt)
	}
}
