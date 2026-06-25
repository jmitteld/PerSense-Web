package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// basis_effects_test.go — regression + documentation tests for how the Basis
// setting (360 / 365 / 365-360) and the Exact ("true daily") method affect an
// amortization, locked to GOLDEN values produced by the real DOS engine
// (legacy/oracle/amort_oracle). These run WITHOUT the oracle binary so they
// always guard against regressions in CI.
//
// Background: a client reported that selecting the 365 basis appeared not to
// change the payment. These tests pin the actual DOS behaviour so that
// expectation is captured precisely — including the (initially surprising) fact
// that with "1st interest prepaid at settlement" ON, the basis alone does NOT
// change the numbers; only the Exact method does. See
// docs/exact_groundzero_findings.md and docs/postmortem_365_exact_interest.md.

// amortPaymentTI runs the real Amortize dispatch for a blank-payment loan and
// returns the modal regular payment and total interest.
func amortPaymentTI(amount, rate float64, n, perYr int,
	loanY int, loanM time.Month, loanD int,
	firstY int, firstM time.Month, firstD int,
	basis types.BasisType, exact, prepaid bool) (pay, totalInt float64) {

	s := Settings{Basis: types.Basis360, PerYr: byte(perYr), YrDays: 360, YrInv: 1.0 / 360}
	switch basis {
	case types.Basis365:
		s.Basis, s.YrDays, s.YrInv = types.Basis365, 365.25, 1.0/365.25
	case types.Basis365360:
		s.Basis, s.YrDays, s.YrInv = types.Basis365360, 360, 1.0/360
	}
	s.Exact, s.Prepaid = exact, prepaid
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: amount,
		LoanRateStatus: types.InOutInput, LoanRate: rate,
		NStatus: types.InOutInput, NPeriods: n, PerYrStatus: types.InOutInput, PerYr: perYr,
		PayAmtStatus:   types.StatusEmpty,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(loanY, loanM, loanD),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(firstY, firstM, firstD)},
		Settings: s}
	res := Amortize(in)
	if res.Err != nil {
		return 0, 0
	}
	return modalReg(res.Schedule), res.TotalInt
}

// TestExactInterestClientRegression locks the exact scenario the client reported
// (docs/postmortem_365_exact_interest.md): $100,000 / 12% / 360 monthly payments,
// loan 2026-06-19, first payment 2026-08-01 (a 43-day odd first period). Before
// the fix the engine produced the 30/360 payment ($1,028.61) on every basis. The
// golden payments below are the real DOS engine's answers.
func TestExactInterestClientRegression(t *testing.T) {
	const (
		amt, rate, n, py = 100000.0, 0.12, 360, 12
		tol              = 0.01
	)
	cases := []struct {
		name    string
		basis   types.BasisType
		exact   bool
		prepaid bool
		wantPay float64
	}{
		// Prepaid ON (the client's screenshot has a row-0 settlement stub).
		{"prepaid 360", types.Basis360, false, true, 1028.6126},
		{"prepaid 365", types.Basis365, false, true, 1028.6126},
		{"prepaid 365 EXACT", types.Basis365, true, true, 1028.8821},
		{"prepaid 365/360", types.Basis365360, false, true, 1028.6126},
		// Prepaid OFF (odd-first interest rolled into the schedule).
		{"360", types.Basis360, false, false, 1032.6863},
		{"365", types.Basis365, false, false, 1032.8258},
		{"365 EXACT", types.Basis365, true, false, 1032.9003},
		{"365/360", types.Basis365360, false, false, 1033.0258},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pay, _ := amortPaymentTI(amt, rate, n, py, 2026, time.June, 19, 2026, time.August, 1, c.basis, c.exact, c.prepaid)
			if math.Abs(pay-c.wantPay) > tol {
				t.Errorf("payment = %.4f, want DOS %.4f (Δ %.4f)", pay, c.wantPay, pay-c.wantPay)
			}
		})
	}
}

// TestBasisEffectsOnAmortization documents and pins, against DOS golden values,
// how each basis × prepaid × exact combination changes the payment and total
// interest for a CLEAN loan ($100,000 / 12% / 360 monthly, first payment exactly
// one month after the loan date). It also asserts the key RELATIONSHIPS a user
// should understand.
func TestBasisEffectsOnAmortization(t *testing.T) {
	const (
		amt, rate, n, py = 100000.0, 0.12, 360, 12
		tolPay = 0.01
		// Total interest matches DOS to the cent on every basis (the 365/360
		// residual was closed by the simple-schedule early-payoff fix — the
		// over-amortizing 365/360 payment retires one period early, as in DOS).
		tolInt = 0.05
	)
	// loan 2024-01-01, first payment 2024-02-01 (clean one-month first period).
	pay := func(basis types.BasisType, exact, prepaid bool) (float64, float64) {
		return amortPaymentTI(amt, rate, n, py, 2024, time.January, 1, 2024, time.February, 1, basis, exact, prepaid)
	}

	cases := []struct {
		name           string
		basis          types.BasisType
		exact, prepaid bool
		wantPay, wantI float64
	}{
		// Prepaid OFF — the basis DOES change the payment (actual-day first-period
		// prorate): 365 and 365/360 raise it above the 30/360 baseline.
		{"360 / prepaid-off", types.Basis360, false, false, 1028.6126, 270300.53},
		{"365 / prepaid-off", types.Basis365, false, false, 1028.7796, 269777.14},
		{"365-exact / prepaid-off", types.Basis365, true, false, 1028.3796, 270216.67},
		{"365-360 / prepaid-off", types.Basis365360, false, false, 1028.9521, 269237.85},
		// Prepaid ON — with the first interest prepaid at settlement, the basis
		// alone does NOT change the numbers; only Exact does.
		{"360 / prepaid-on", types.Basis360, false, true, 1028.6126, 270300.53},
		{"365 / prepaid-on", types.Basis365, false, true, 1028.6126, 270300.53},
		{"365-exact / prepaid-on", types.Basis365, true, true, 1028.3796, 270216.67},
		{"365-360 / prepaid-on", types.Basis365360, false, true, 1028.6126, 270300.53},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, ti := pay(c.basis, c.exact, c.prepaid)
			if math.Abs(p-c.wantPay) > tolPay {
				t.Errorf("payment = %.4f, want DOS %.4f", p, c.wantPay)
			}
			if math.Abs(ti-c.wantI) > tolInt {
				t.Errorf("total interest = %.2f, want DOS %.2f", ti, c.wantI)
			}
		})
	}

	// Relationship assertions (the user-facing takeaways) ------------------------

	// (1) On the 360 basis, Exact is a NO-OP (DOS requires a non-360 basis).
	p360, _ := pay(types.Basis360, false, false)
	p360x, _ := pay(types.Basis360, true, false)
	if math.Abs(p360-p360x) > tolPay {
		t.Errorf("Exact changed the 360-basis payment (%.4f vs %.4f); it must be a no-op on 360", p360, p360x)
	}

	// (2) With prepaid ON, changing ONLY the basis (no Exact) does NOT change the
	//     payment — the surprise the client hit. 360 == 365 == 365/360.
	on360, _ := pay(types.Basis360, false, true)
	on365, _ := pay(types.Basis365, false, true)
	on365360, _ := pay(types.Basis365360, false, true)
	if math.Abs(on360-on365) > tolPay || math.Abs(on360-on365360) > tolPay {
		t.Errorf("prepaid basis payments differ (360=%.4f 365=%.4f 365/360=%.4f); DOS makes them equal", on360, on365, on365360)
	}

	// (3) Turning Exact ON with prepaid ON DOES change the payment (true daily).
	on365x, _ := pay(types.Basis365, true, true)
	if math.Abs(on365x-on365) < tolPay {
		t.Errorf("Exact did not change the prepaid 365 payment (%.4f); it should give true-daily interest", on365x)
	}

	// (4) With prepaid OFF, the basis DOES change the payment (actual-day prorate).
	off360, _ := pay(types.Basis360, false, false)
	off365, _ := pay(types.Basis365, false, false)
	if math.Abs(off360-off365) < tolPay {
		t.Errorf("prepaid-off 360 and 365 payments are equal (%.4f); DOS differentiates them", off360)
	}
}
