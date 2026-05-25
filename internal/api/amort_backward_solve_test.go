// Tests for the amortization handler's backward-solve dispatch: when
// the caller leaves exactly one of {Amount, Rate} blank, the handler
// runs FirstPass to derive the term / first-payment date and then
// dispatches to amortization.SolveLoanAmount / SolveRate.
//
// These complement the C-1 / C-2 canaries (which supply an explicit
// firstDate and nPeriods). Here the date/term fields are deliberately
// left for FirstPass to derive, exercising the FirstPass-on-copy step
// in HandleAmortizationCalc.

package api

import (
	"math"
	"testing"
)

// TestAmortSolveAmountDerivesFirstDate solves for the loan amount when
// the 1st Pmt Date is omitted. The handler must run FirstPass to
// default the first-payment date (loanDate + 1 period) before
// SolveLoanAmount — which requires a known first date — can run.
func TestAmortSolveAmountDerivesFirstDate(t *testing.T) {
	// rate 6%, 360 monthly payments of $1199.10 ⇒ amount ≈ $200,000.
	resp := postAmort(t, `{
		"rate": 0.06,
		"loanDate": "2025-01-01",
		"nPeriods": 360,
		"perYr": 12,
		"payment": 1199.10
	}`)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if math.Abs(resp.Amount-200000) > 1000 {
		t.Errorf("solved Amount = %.2f, want ~200000", resp.Amount)
	}
	if len(resp.Schedule) == 0 {
		t.Fatal("expected a non-empty schedule")
	}
	principal := resp.TotalPaid - resp.TotalInt
	if math.Abs(principal-resp.Amount) > 2.0 {
		t.Errorf("principal (TotalPaid-TotalInt) = %.2f, want ~Amount %.2f",
			principal, resp.Amount)
	}
}

// TestAmortSolveRateFromLastDate solves for the rate when the term is
// given as a Last Pmt Date rather than an explicit # Periods. The
// handler must run FirstPass to derive nPeriods from first+last before
// SolveRate — which requires a known term — can run.
func TestAmortSolveRateFromLastDate(t *testing.T) {
	// amount $200,000, 360 monthly payments of $1199.10 ⇒ rate ≈ 6%.
	resp := postAmort(t, `{
		"amount": 200000,
		"loanDate": "2025-01-01",
		"firstDate": "2025-02-01",
		"lastDate": "2055-01-01",
		"perYr": 12,
		"payment": 1199.10
	}`)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.NPeriods != 360 {
		t.Errorf("derived NPeriods = %d, want 360", resp.NPeriods)
	}
	if math.Abs(resp.Rate-0.06) > 0.005 {
		t.Errorf("solved Rate = %.5f, want ~0.06", resp.Rate)
	}
	if len(resp.Schedule) == 0 {
		t.Fatal("expected a non-empty schedule")
	}
}

// TestAmortSolveAmountInsufficientData confirms a clean, field-named
// error when the screen lacks what SolveLoanAmount needs (here: no
// payment), rather than a silent zero-amount schedule.
func TestAmortSolveAmountInsufficientData(t *testing.T) {
	resp := postAmort(t, `{
		"rate": 0.06,
		"loanDate": "2025-01-01",
		"nPeriods": 360,
		"perYr": 12
	}`)
	if resp.Error == "" {
		t.Fatal("expected an error solving for Amount with no Payment, got none")
	}
	if !containsAny(resp.Error, []string{"Amount"}) {
		t.Errorf("error %q should name the Amount field", resp.Error)
	}
}
