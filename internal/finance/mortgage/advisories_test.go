package mortgage

import (
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// advisoryCodes pulls the structured advisory codes out of a result's
// Warnings channel (entries formatted by types.FormatAdvisory).
func advisoryCodes(ws []string) []string {
	var out []string
	for _, w := range ws {
		if !strings.HasPrefix(w, "@@ADV|") {
			continue
		}
		body := strings.TrimPrefix(w, "@@ADV|")
		head := body
		if i := strings.Index(body, "@@"); i >= 0 {
			head = body[:i]
		}
		parts := strings.Split(head, "|") // tier|code|fields
		if len(parts) >= 2 {
			out = append(out, parts[1])
		}
	}
	return out
}

func hasCode(ws []string, code string) bool {
	for _, c := range advisoryCodes(ws) {
		if c == code {
			return true
		}
	}
	return false
}

// example3Base builds the help Example 3 mortgage (Price 280k, 20% down,
// 30yr @ 8.25%, 2.5 points, tax 300), with Monthly and Balloon Yrs left to
// the caller.
func example3Base() MtgLine {
	return MtgLine{
		PriceStatus: types.InOutInput, Price: 280000,
		PointsStatus: types.InOutInput, Points: 0.025,
		PctStatus: types.InOutInput, Pct: 0.20,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.0825),
		TaxStatus: types.InOutInput, Tax: 300,
	}
}

// M-W1: full payment hardened + balloon → balloon ≈ 0 → advisory fires.
func TestAdvisoryMW1_BalloonNearZero(t *testing.T) {
	m := example3Base()
	m.MonthlyStatus = types.InOutInput
	m.Monthly = 1982.84 // the full amortizing payment incl. tax
	m.WhenStatus = types.InOutInput
	m.When = 8
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	if !hasCode(r.Warnings, "M-W1") {
		t.Errorf("expected M-W1 (near-zero balloon); got codes %v", advisoryCodes(r.Warnings))
	}
}

// M-W1 must NOT fire for the real Example 3 (Monthly 1,600 → balloon ~98k).
func TestAdvisoryMW1_NotOnRealBalloon(t *testing.T) {
	m := example3Base()
	m.MonthlyStatus = types.InOutInput
	m.Monthly = 1600
	m.WhenStatus = types.InOutInput
	m.When = 8
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	for _, c := range []string{"M-W1", "M-W2", "M-W3"} {
		if hasCode(r.Warnings, c) {
			t.Errorf("did not expect %s on the real Example 3 balloon; codes %v", c, advisoryCodes(r.Warnings))
		}
	}
	if r.Line.HowMuch < 90000 || r.Line.HowMuch > 105000 {
		t.Errorf("sanity: balloon = %.2f, want ~98,372", r.Line.HowMuch)
	}
}

// M-W2: an overpaying monthly drives the balloon meaningfully negative.
func TestAdvisoryMW2_NegativeBalloon(t *testing.T) {
	m := example3Base()
	m.MonthlyStatus = types.InOutInput
	m.Monthly = 2600 // well above the ~1,983 full payment
	m.WhenStatus = types.InOutInput
	m.When = 8
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	if !hasCode(r.Warnings, "M-W2") {
		t.Errorf("expected M-W2 (negative balloon); got codes %v, balloon=%.2f", advisoryCodes(r.Warnings), r.Line.HowMuch)
	}
}

// M-W3: a payment well below interest makes the balloon exceed the loan.
func TestAdvisoryMW3_BalloonExceedsLoan(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 280000,
		PointsStatus: types.InOutInput, Points: 0,
		PctStatus: types.InOutInput, Pct: 0.20,
		YearsStatus: types.InOutInput, Years: 30,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.0825),
		TaxStatus: types.InOutInput, Tax: 0,
		MonthlyStatus: types.InOutInput, Monthly: 400, // far below interest
		WhenStatus: types.InOutInput, When: 8,
	}
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	if !hasCode(r.Warnings, "M-W3") {
		t.Errorf("expected M-W3 (balloon > loan); got codes %v, balloon=%.2f", advisoryCodes(r.Warnings), r.Line.HowMuch)
	}
}

// M-W7: balloon scheduled at/after the final year.
func TestAdvisoryMW7_BalloonAfterTerm(t *testing.T) {
	m := example3Base()
	m.MonthlyStatus = types.InOutInput
	m.Monthly = 1600
	m.WhenStatus = types.InOutInput
	m.When = 30 // == Years
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	if !hasCode(r.Warnings, "M-W7") {
		t.Errorf("expected M-W7 (balloon at/after term); got codes %v", advisoryCodes(r.Warnings))
	}
}

// Healthy forward calc (help Example 1) must produce no advisories.
func TestAdvisoryNoneOnHealthyForward(t *testing.T) {
	m := MtgLine{
		PriceStatus: types.InOutInput, Price: 200000,
		PointsStatus: types.InOutInput, Points: 0.02,
		PctStatus: types.InOutInput, Pct: 0.20,
		YearsStatus: types.InOutInput, Years: 20,
		RateStatus: types.InOutInput, Rate: LoanRateToTrueRate(0.08),
		TaxStatus: types.InOutInput, Tax: 200,
	}
	r := Calc(m)
	if r.Err != nil {
		t.Fatalf("calc: %v", r.Err)
	}
	if codes := advisoryCodes(r.Warnings); len(codes) != 0 {
		t.Errorf("expected no advisories on a healthy forward calc; got %v", codes)
	}
}
