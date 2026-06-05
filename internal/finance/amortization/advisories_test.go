package amortization

import (
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func advCodes(ws []string) []string {
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
		if parts := strings.Split(head, "|"); len(parts) >= 2 {
			out = append(out, parts[1])
		}
	}
	return out
}

func hasAdvCode(ws []string, code string) bool {
	for _, c := range advCodes(ws) {
		if c == code {
			return true
		}
	}
	return false
}

// A healthy fully-amortizing loan must produce no advisories.
func TestAmortAdvisoryNoneOnHealthy(t *testing.T) {
	r := Amortize(makeSimpleLoan())
	if r.Err != nil {
		t.Fatalf("amortize: %v", r.Err)
	}
	if c := advCodes(r.Warnings); len(c) != 0 {
		t.Errorf("expected no advisories on a healthy loan; got %v", c)
	}
}

// A-W6: a payment below the first period's interest grows the balance.
func TestAmortAdvisoryAW6_NegAm(t *testing.T) {
	in := makeSimpleLoan()
	in.Loan.PayAmt = 400 // below the ~$500 first-period interest on $100k @ 6%
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("amortize: %v", r.Err)
	}
	if !hasAdvCode(r.Warnings, "A-W6") {
		t.Errorf("expected A-W6 (negative amortization); got %v", advCodes(r.Warnings))
	}
}

// ex5Loan builds Amortization Example 5 (100k, 8%, 60 monthly periods)
// with a balloon at month 60; the caller decides whether the payment is a
// hard input.
func ex5Loan(payInput bool, payAmt, balloon float64) LoanInput {
	l := Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		LoanRateStatus: types.InOutInput, LoanRate: 0.08,
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
		NStatus: types.InOutInput, NPeriods: 60,
		PerYrStatus: types.InOutInput, PerYr: 12,
	}
	if payInput {
		l.PayAmtStatus = types.InOutInput
		l.PayAmt = payAmt
	}
	return LoanInput{
		Loan:     l,
		Settings: Settings{Basis: types.Basis360, PerYr: 12, Prepaid: true, YrDays: 360, YrInv: 1.0 / 360, CenturyDiv: 50},
		Fancy:    true,
		Balloons: []BalloonPayment{{
			DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1),
			AmountStatus: types.InOutInput, Amount: balloon,
		}},
	}
}

// A-W11: a balloon is set but the payment is computed -> balloon dropped.
func TestAmortAdvisoryAW11_BalloonDroppedWhenPaymentComputed(t *testing.T) {
	r := Amortize(ex5Loan(false, 0, 500000)) // payment blank
	if r.Err != nil {
		t.Fatalf("amortize: %v", r.Err)
	}
	if !hasAdvCode(r.Warnings, "A-W11") {
		t.Errorf("expected A-W11 (balloon ignored, payment computed); got %v", advCodes(r.Warnings))
	}
}

// A-W11 must NOT fire when the payment is a user input (Example 5 as written):
// the interest-only payment is supplied and the balloon is applied.
func TestAmortAdvisoryAW11_NotWhenPaymentSupplied(t *testing.T) {
	r := Amortize(ex5Loan(true, 666.67, 100000)) // interest-only payment input
	if r.Err != nil {
		t.Fatalf("amortize: %v", r.Err)
	}
	if hasAdvCode(r.Warnings, "A-W11") {
		t.Errorf("did not expect A-W11 when the payment is supplied; got %v", advCodes(r.Warnings))
	}
}
