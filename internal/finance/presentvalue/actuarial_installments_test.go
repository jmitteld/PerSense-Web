package presentvalue

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

// TestActuarialInstallmentsColumn verifies the per-payment-date breakdown
// (PeriodicPayment.Installments) reproduces the DOS PVL table's per-row
// probability column (pvltable.pas PrintNextPayment, lines 514-533):
//   - one installment per scheduled payment date,
//   - each carrying the survival probability LifeProb(t) at that date,
//   - Value == IfPaid * Prob (DOS v = ifpd * prob), and
//   - the installment values reconcile exactly to the row's total Val.
//
// This locks in the realignment from the DOS-fidelity review: DOS prints a
// per-payment probability, not the single stream average the engine also
// reports in PeriodicPayment.Prob.
func TestActuarialInstallmentsColumn(t *testing.T) {
	asOf := dateOf(2026, time.January, 1)
	dob := dateOf(1956, time.January, 1) // age 70
	cfg := actuarialTestCfg(asOf, dob)
	rate := 0.06
	const P = 12000.0
	fromYear, toYear := 2030, 2045 // ages 74..89, annual payments

	res := Calculate(PVInput{
		Settings: vrTestSettings(),
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asOf,
			R: RateEntry{Status: types.InOutInput, Rate: rate},
		},
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: dateOf(fromYear, time.January, 1),
			ToDateStatus: types.InOutInput, ToDate: dateOf(toYear, time.January, 1),
			PerYrStatus: types.InOutInput, PerYr: 1, // annual
			AmtStatus: types.InOutInput, Amt: P,
			Act: actuarial.Living,
		}},
		Actuarial: cfg,
	})
	if res.Err != nil {
		t.Fatalf("annuity: %v", res.Err)
	}
	if len(res.Periodics) != 1 {
		t.Fatalf("expected 1 periodic row, got %d", len(res.Periodics))
	}
	pp := res.Periodics[0]

	// One installment per payment date, fromYear..toYear inclusive.
	wantN := toYear - fromYear + 1
	if len(pp.Installments) != wantN {
		t.Fatalf("installments = %d, want %d (one per payment date)", len(pp.Installments), wantN)
	}

	var sumValue, probSum float64
	for i, in := range pp.Installments {
		// Per-payment probability must equal LifeProb at that exact date —
		// the DOS per-row probability column, not a stream average.
		wantProb := cfg.LifeProb(in.Date, actuarial.Living)
		if math.Abs(in.Prob-wantProb) > 1e-12 {
			t.Errorf("installment %d (%s): Prob=%.10f want LifeProb=%.10f",
				i, in.Date.Time.Format("2006-01-02"), in.Prob, wantProb)
		}
		// DOS relation v = ifpd * prob.
		if math.Abs(in.Value-in.IfPaid*in.Prob) > 1e-9 {
			t.Errorf("installment %d: Value=%.6f != IfPaid*Prob=%.6f", i, in.Value, in.IfPaid*in.Prob)
		}
		// IfPaid must ignore survival (probability divides back out): it is
		// the discounted payment, so IfPaid/amount is in (0,1].
		if in.Prob > 0 && in.IfPaid <= 0 {
			t.Errorf("installment %d: non-positive IfPaid %.6f", i, in.IfPaid)
		}
		sumValue += in.Value
		probSum += in.Prob
	}

	// Per-payment values reconcile to the row's total present value.
	if rel := math.Abs(sumValue-pp.Val) / math.Abs(pp.Val); rel > 1e-9 {
		t.Errorf("sum(installment.Value)=%.6f != row Val=%.6f (rel %.2e)", sumValue, pp.Val, rel)
	}

	// The stream-average Prob the engine also reports is the mean of the
	// per-payment probabilities.
	wantAvg := probSum / float64(len(pp.Installments))
	if math.Abs(pp.Prob-wantAvg) > 1e-12 {
		t.Errorf("row avg Prob=%.10f want mean of installments=%.10f", pp.Prob, wantAvg)
	}
}
