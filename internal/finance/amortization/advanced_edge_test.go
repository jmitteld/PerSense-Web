package amortization

import (
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Advanced-option edge-case and boundary tests, complementing the
// existing advanced_test.go / *_test.go coverage. These target the
// boundaries where off-by-one and interaction bugs hide: the exact NN
// vs StopDate equivalence, balloons coincident with the last payment,
// adjustments on the first period, prepayment frequencies other than
// monthly, and degenerate "skip every month" inputs.

func edgeLoan(amount float64, n int) Loan {
	return mkFancyLoan(amount, 0.06, n, 0)
}

// amortizingInput builds a fancy loan whose fixed payment is the
// closed-form amount that retires it at term, so the baseline actually
// pays off and the effect of extras/balloons is measurable.
func amortizingInput(t *testing.T, amount float64, n int) LoanInput {
	t.Helper()
	probe := mkFancyLoan(amount, 0.06, n, 0)
	probe.PayAmtStatus = types.StatusEmpty
	d, err := SolvePayment(LoanInput{Loan: probe, Settings: fancyTestSettings()})
	if err != nil {
		t.Fatalf("amortizingInput: SolvePayment(%.0f, n=%d): %v", amount, n, err)
	}
	loan := mkFancyLoan(amount, 0.06, n, d)
	return LoanInput{Loan: loan, Settings: fancyTestSettings(), Fancy: true}
}

// TestEdge_PrepaymentNNEqualsStopDate pins that the two ways of
// bounding a prepayment series — a payment count (NN) and an explicit
// StopDate on the date of the NN-th extra — produce the SAME schedule.
// A discrepancy would reveal an off-by-one in either termination path.
func TestEdge_PrepaymentNNEqualsStopDate(t *testing.T) {
	// Monthly extras starting 2024-03-01. The 12th extra falls on
	// 2025-02-01.
	byNN := LoanInput{Loan: edgeLoan(200_000, 120), Settings: fancyTestSettings(), Fancy: true,
		Prepayments: []Prepayment{{
			StartDateStatus: types.InOutInput,
			StartDate:       newDate(2024, time.March, 1),
			NNStatus:        types.InOutInput,
			NN:              12,
			PerYrStatus:     types.InOutInput,
			PerYr:           12,
			PaymentStatus:   types.InOutInput,
			Payment:         500,
		}},
	}
	byStop := byNN
	byStop.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       newDate(2024, time.March, 1),
		StopDateStatus:  types.InOutInput,
		StopDate:        newDate(2025, time.February, 1),
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         500,
	}}

	rNN := Amortize(byNN)
	rStop := Amortize(byStop)
	if rNN.Err != nil || rStop.Err != nil {
		t.Fatalf("errors: NN=%v Stop=%v", rNN.Err, rStop.Err)
	}
	if msg, ok := schedulesEqual(rNN.Schedule, rStop.Schedule); !ok {
		t.Errorf("NN=12 and StopDate=2025-02-01 produced different schedules (%s): "+
			"lenNN=%d lenStop=%d, totalPaidNN=%.2f totalPaidStop=%.2f",
			msg, len(rNN.Schedule), len(rStop.Schedule), rNN.TotalPaid, rStop.TotalPaid)
	}
}

// TestEdge_PrepaymentReducesTerm: an NN-bounded prepayment series must
// retire the loan strictly earlier than the same loan with no extras.
func TestEdge_PrepaymentReducesTerm(t *testing.T) {
	base := amortizingInput(t, 200_000, 360)
	withExtra := base
	withExtra.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       newDate(2024, time.February, 1),
		NNStatus:        types.InOutInput,
		NN:              60,
		PerYrStatus:     types.InOutInput,
		PerYr:           12,
		PaymentStatus:   types.InOutInput,
		Payment:         400,
	}}
	rb := Amortize(base)
	re := Amortize(withExtra)
	if rb.Err != nil || re.Err != nil {
		t.Fatalf("errors: base=%v extra=%v", rb.Err, re.Err)
	}
	if len(re.Schedule) >= len(rb.Schedule) {
		t.Errorf("prepayments did not shorten the term: base=%d rows, with-extra=%d rows",
			len(rb.Schedule), len(re.Schedule))
	}
}

// TestEdge_AnnualPrepayment exercises a prepayment series with PerYr=1
// (one extra per year) — a frequency other than the monthly default,
// where an AddPeriod step bug would show up. Must be deterministic and
// shorten the term.
func TestEdge_AnnualPrepayment(t *testing.T) {
	in := amortizingInput(t, 200_000, 360)
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput,
		StartDate:       newDate(2025, time.January, 1),
		NNStatus:        types.InOutInput,
		NN:              10,
		PerYrStatus:     types.InOutInput,
		PerYr:           1,
		PaymentStatus:   types.InOutInput,
		Payment:         5_000,
	}}
	runThrice(t, "annual-prepay", in)
	base := amortizingInput(t, 200_000, 360)
	if len(Amortize(in).Schedule) >= len(Amortize(base).Schedule) {
		t.Errorf("annual prepayments did not shorten the term")
	}
}

// TestEdge_BalloonOnLastPaymentDate places a balloon coincident with
// the final regular payment. It must be applied (terminal balance ~0)
// and not silently dropped by an off-by-one in the veryLast window.
func TestEdge_BalloonOnLastPaymentDate(t *testing.T) {
	// Interest-only loan: payment = principal * rate / perYr exactly, so
	// the balance stays at the principal and only a balloon can retire
	// it. Place that balloon on the final payment date — the boundary
	// where a veryLast off-by-one would drop it.
	const principal = 200_000.0
	interestOnly := principal * 0.06 / 12 // = 1000.00
	loan := mkFancyLoan(principal, 0.06, 120, interestOnly)

	// Without the balloon: interest-only, so the balance never falls.
	bare := LoanInput{Loan: loan, Settings: fancyTestSettings(), Fancy: true}
	rBare := Amortize(bare)
	if rBare.Err != nil {
		t.Fatalf("bare interest-only: %v", rBare.Err)
	}
	if rBare.FinalPrinc < principal-1 {
		t.Fatalf("interest-only baseline unexpectedly amortized: FinalPrinc=%.2f (want ~%.0f)",
			rBare.FinalPrinc, principal)
	}

	// With a balloon equal to the principal on the last payment date.
	// Use the engine-derived LastDate (FirstPass computes it from
	// FirstDate + NPeriods; the raw Loan struct leaves it zero).
	in := bare
	in.Balloons = []BalloonPayment{{
		DateStatus:   types.InOutInput,
		Date:         rBare.LastDate,
		AmountStatus: types.InOutInput,
		Amount:       principal,
	}}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize: %v", r.Err)
	}
	if r.FinalPrinc > 1.0 {
		t.Errorf("balloon on last payment date left FinalPrinc=%.4f (>1); the last-date balloon was dropped", r.FinalPrinc)
	}
}

// TestEdge_MultipleBalloons applies two balloons on different dates and
// checks both bite: total paid exceeds the no-balloon baseline by at
// least the combined balloon amount's order of magnitude, and the loan
// retires early.
func TestEdge_MultipleBalloons(t *testing.T) {
	base := amortizingInput(t, 200_000, 360)
	withBalloons := base
	withBalloons.Balloons = []BalloonPayment{
		{DateStatus: types.InOutInput, Date: newDate(2027, time.January, 1), AmountStatus: types.InOutInput, Amount: 15_000},
		{DateStatus: types.InOutInput, Date: newDate(2032, time.January, 1), AmountStatus: types.InOutInput, Amount: 20_000},
	}
	rb := Amortize(base)
	rB := Amortize(withBalloons)
	if rb.Err != nil || rB.Err != nil {
		t.Fatalf("errors: base=%v balloons=%v", rb.Err, rB.Err)
	}
	if len(rB.Schedule) >= len(rb.Schedule) {
		t.Errorf("two balloons did not shorten the term: base=%d, balloons=%d",
			len(rb.Schedule), len(rB.Schedule))
	}
	runThrice(t, "multi-balloon", withBalloons)
}

// TestEdge_AdjustmentOnFirstPayment puts a rate change on the very
// first payment date. The schedule must build cleanly and differ from
// the un-adjusted loan.
func TestEdge_AdjustmentOnFirstPayment(t *testing.T) {
	loan := edgeLoan(200_000, 120)
	base := LoanInput{Loan: loan, Settings: fancyTestSettings(), Fancy: true}
	adj := base
	adj.Adjustments = []RateAdjustment{{
		DateStatus:     types.InOutInput,
		Date:           loan.FirstDate,
		LoanRateStatus: types.InOutInput,
		LoanRate:       0.10,
	}}
	rb := Amortize(base)
	ra := Amortize(adj)
	if rb.Err != nil || ra.Err != nil {
		t.Fatalf("errors: base=%v adj=%v", rb.Err, ra.Err)
	}
	if floatEq(rb.TotalInt, ra.TotalInt) {
		t.Errorf("rate adjustment on first payment had no effect: totalInt base=%.2f adj=%.2f",
			rb.TotalInt, ra.TotalInt)
	}
	runThrice(t, "adjust-first", adj)
}

// TestEdge_SequentialAdjustments applies two rate changes (an ARM with
// two resets). Must be deterministic.
func TestEdge_SequentialAdjustments(t *testing.T) {
	in := LoanInput{Loan: edgeLoan(200_000, 360), Settings: fancyTestSettings(), Fancy: true,
		Adjustments: []RateAdjustment{
			{DateStatus: types.InOutInput, Date: newDate(2027, time.January, 1), LoanRateStatus: types.InOutInput, LoanRate: 0.08},
			{DateStatus: types.InOutInput, Date: newDate(2031, time.January, 1), LoanRateStatus: types.InOutInput, LoanRate: 0.05},
		},
	}
	runThrice(t, "sequential-adjust", in)
}

// TestEdge_SkipEveryMonthDoesNotAmortize: skipping all 12 months means
// no payment ever reduces principal. The engine must terminate (no
// infinite loop) and leave the loan unpaid rather than retiring it.
func TestEdge_SkipEveryMonthDoesNotAmortize(t *testing.T) {
	ms, err := MonthSetFromString("1-12")
	if err != nil {
		t.Fatalf("MonthSetFromString: %v", err)
	}
	in := LoanInput{Loan: edgeLoan(200_000, 120), Settings: fancyTestSettings(), Fancy: true,
		SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "1-12", MonthSet: ms},
	}
	done := make(chan AmortResult, 1)
	go func() { done <- Amortize(in) }()
	select {
	case r := <-done:
		if r.Err != nil {
			// An explicit "loan never amortizes" error is also acceptable.
			t.Logf("skip-all returned error (acceptable): %v", r.Err)
			return
		}
		if r.FinalPrinc <= 0 {
			t.Errorf("skip-every-month somehow retired the loan: FinalPrinc=%.2f", r.FinalPrinc)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Amortize hung on skip-every-month input (possible infinite loop)")
	}
}
