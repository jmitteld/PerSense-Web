// Regressions for the 2026-07-13 audit PASS-4 findings (docs/discrepancies.md
// §23). Every golden cites the real DOS-engine command that produced it.
package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// P4-1: DOS's payment solve is entirely R78-agnostic (EstimateAndRefinePayment
// never inspects the R78 flag — R78 only changes the interest SPLIT), so an
// R78 loan's payment equals the plain loan's. The prior needPaymentRefine had
// an `if s.R78 { return s.InAdvance }` special-case that skipped the odd-first
// refine for arrears R78, leaving the un-refined estimate.
//
//	amort_oracle 10000 0.1213 36 24 r78 → payment 302.9827, interest 907.38
//	  (first payment ON the loan date — a zero-length first period; the
//	   un-refined estimate was 304.5140. The plain semimonthly payment is
//	   also 302.9827: R78 does not change it.)
//	amort_oracle 10000 0.1213 36 12 r78 → payment 332.7643 (monthly control,
//	  always matched)
func TestPass4R78PaymentIsPlain(t *testing.T) {
	// Semimonthly with the first payment ON the loan date (a zero-length first
	// period), reproducing the goamort/oracle default for peryr=24.
	sSemi := Settings{Basis: types.Basis360, PerYr: 24, YrDays: 360, YrInv: 1.0 / 360, R78: true}
	loanSemi := Loan{
		AmountStatus: types.InOutInput, Amount: 10000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.1213,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 36,
		PerYrStatus: types.InOutInput, PerYr: 24,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 1),
	}
	res := Amortize(LoanInput{Loan: loanSemi, Settings: sSemi})
	var pay float64
	for _, r := range res.Schedule {
		if r.PayNum == 1 {
			pay = r.PayAmt
			break
		}
	}
	if res.Err != nil || math.Abs(pay-302.9827) > 0.005 || math.Abs(res.TotalInt-907.38) > 0.05 {
		t.Errorf("R78 semimonthly: pay=%.4f int=%.2f err=%v, want 302.9827 / 907.38 (oracle; un-refined was 304.5140)",
			pay, res.TotalInt, res.Err)
	}
	// The plain (non-R78) loan must give the SAME payment.
	sPlain := sSemi
	sPlain.R78 = false
	res0 := Amortize(LoanInput{Loan: loanSemi, Settings: sPlain})
	var pay0 float64
	for _, r := range res0.Schedule {
		if r.PayNum == 1 {
			pay0 = r.PayAmt
			break
		}
	}
	if math.Abs(pay-pay0) > 0.005 {
		t.Errorf("R78 payment %.4f != plain payment %.4f — R78 must not change the payment", pay, pay0)
	}
}

// P4-2: R78 SUPPRESSES exact in DOS's display gate `(exact and not R78) or
// non-360` (Amortize.pas:1493), so an R78 × exact × in-advance loan on the 360
// basis uses the plain in-advance R78 schedule (sum-of-digits allocation whose
// total is n·d − principal), NOT the exact settlement-shifted shape. The
// display arm previously fired for any `exact && inadv`, adding a spurious
// exact settlement.
//
//	amort_oracle 100000 0.1173 48 24 r78 exact inadv → payment 2332.1827, interest 11944.77
//	amort_oracle 100000 0.1173 48 24 r78 inadv       → payment 2332.1827, interest 11944.77 (= n·d − principal)
//	  (the exact arm rendered 12477.59, adding a spurious exact settlement)
func TestPass4R78SuppressesExactDisplay(t *testing.T) {
	mk := func(exact bool) LoanInput {
		s := Settings{Basis: types.Basis360, PerYr: 24, YrDays: 360, YrInv: 1.0 / 360, R78: true, InAdvance: true, Exact: exact}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.1173,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 48,
			PerYrStatus: types.InOutInput, PerYr: 24,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 1),
		}, Settings: s}
	}
	resExact := Amortize(mk(true))
	if resExact.Err != nil || math.Abs(resExact.TotalInt-11944.77) > 0.05 {
		t.Errorf("R78+exact+inadv: int=%.2f err=%v, want 11944.77 (oracle; exact arm gave 12477.59)",
			resExact.TotalInt, resExact.Err)
	}
	// Must equal the R78+inadv (no-exact) schedule — R78 suppresses exact.
	resPlain := Amortize(mk(false))
	if math.Abs(resExact.TotalInt-resPlain.TotalInt) > 0.05 {
		t.Errorf("R78+exact+inadv int %.2f != R78+inadv int %.2f — R78 must suppress exact",
			resExact.TotalInt, resPlain.TotalInt)
	}
}

// P4-F2: in-advance payoff reconstructs DOS's balance_calc RepayFancyLoan walk
// (Amortize.pas:1114-1125), whose base_date starts at firstdate (shifted vs
// arrears) and which accrues plain opening-balance interest with NO settlement
// row — structurally different from the display schedule. Reading display rows
// left every in-advance payoff ~0.2–0.7% low.
//
//	amort_oracle 100000 0.0632 48 12 payhard=684.67 payoff=15.6.2025 inadv → 97096.1096 (display-row formula: 96909.2958)
//	amort_oracle 100000 0.0632 48 12 payhard=684.67 payoff=1.6.2025 inadv  → 97540.5700 (on a payment date)
//	amort_oracle 50000 0.1121 60 24 payhard=303.6 payoff=15.6.2025 inadv   → 47410.2224 (semimonthly)
//	amort_oracle 100000 0.0632 48 12 payhard=684.67 payoff=15.6.2025       → 97436.6405 (arrears control, unchanged)
func TestPass4InAdvancePayoffWalk(t *testing.T) {
	mk := func(amt, rate float64, n, peryr int, payh float64, inadv bool) LoanInput {
		yd, yi := 360.0, 1.0/360
		s := Settings{Basis: types.Basis360, PerYr: byte(peryr), YrDays: yd, YrInv: yi, InAdvance: inadv}
		fd := types.NewDateRec(2024, time.February, 1)
		if peryr == 24 {
			// goamort/oracle default first date for peryr=24 is the loan date.
			fd = types.NewDateRec(2024, time.January, 1)
		}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amt,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			PayAmtStatus: types.InOutInput, PayAmt: payh,
			NStatus:      types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: peryr,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: s}
	}
	cases := []struct {
		tag        string
		in         LoanInput
		y, m, dd   int
		want       float64
	}{
		{"inadv mid-period", mk(100000, 0.0632, 48, 12, 684.67, true), 2025, 6, 15, 97096.1096},
		{"inadv on-payment", mk(100000, 0.0632, 48, 12, 684.67, true), 2025, 6, 1, 97540.5700},
		{"inadv semimonthly", mk(50000, 0.1121, 60, 24, 303.6, true), 2025, 6, 15, 47410.2224},
		{"arrears control", mk(100000, 0.0632, 48, 12, 684.67, false), 2025, 6, 15, 97436.6405},
	}
	for _, c := range cases {
		got, err := PayoffBalance(c.in, types.NewDateRec(c.y, time.Month(c.m), c.dd))
		if err != nil || math.Abs(got-c.want) > 0.005 {
			t.Errorf("%s: payoff = %.4f err=%v, want %.4f (oracle)", c.tag, got, err, c.want)
		}
	}
}
