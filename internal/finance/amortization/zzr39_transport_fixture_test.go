package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// zzr39_transport_fixture_test.go — R39, THE TRANSPORT RULE, PINNED.
//
// "A cell the original computes must be TRANSPORTED, never RECONSTRUCTED."
// Three defects on 2026-08-07 had one cause: an output cell never left the
// engine, so a consumer guessed it back from the schedule. Two consumers were
// guessing, with the SAME heuristic, and both were wrong:
//
//  1. the WEB UI's top-line Pmt Amount (index.html) — `schedule[0].payment`, or
//     the MODAL row when a target/moratorium/skip was present. On a loan with a
//     rate adjustment the post-adjustment segment is usually the longer one, so
//     the modal IS the adjusted payment. Nate reported exactly that.
//  2. the APR pass (engine.go, the AmortizeDOS arm) — payoffRegularPayment, the
//     same modal, fed to the APR value walk as a hard payment. On a randomized
//     differential over stacked options with the payment left blank, 6-10% of
//     screens diverged from DOS's APR, worst 5.17 PERCENTAGE POINTS, while the
//     schedules agreed to the cent.
//
// And the Rate/Payment Adjustment grid had no transport at all: DOS's
// Re_Amortize writes the re-amortized payment onto the row
// (`adj[i]^.amount := d`, `amountstatus := outp`, AMORTOP.pas:1499-1594) and the
// port had no field for it at any layer, so the "New Amount" cell could never be
// filled no matter what the engine computed.
//
// ⚠️ EVERY `want` BELOW IS AN ORACLE CONSTANT, printed by the command line in its
// own comment. Re-derive, do not adjust.

const r39Tol = 0.01 // one cent / 1e-4 on a rate

// --- 1. THE REGULAR PAYMENT --------------------------------------------------

// TestR39RegularPaymentVsDOS pins AmortResult.RegularPayment — the field the UI
// now reads instead of inspecting rows. Each case is one the old reconstruction
// got WRONG, so a regression to any schedule-derived scheme fails here.
func TestR39RegularPaymentVsDOS(t *testing.T) {
	cases := []struct {
		name   string
		oracle string
		build  func() LoanInput
		want   float64
		// oldWould is what the DELETED index.html reconstruction would have put
		// in the cell for this screen. It is the anti-vacuity anchor: a fixture
		// whose `want` and `oldWould` coincide has stopped testing transport.
		// Naming it per case matters — the two branches fail on DIFFERENT
		// screens, and a single "is it the modal?" check passes vacuously on a
		// row1 case (there the modal is RIGHT and row 1 is wrong).
		oldWould float64
	}{
		{
			// THE MODAL TRAP. A rate adjustment at month 84 of 360: DOS's regular
			// payment is 672.13 and the post-adjustment segment (276 rows) is far
			// longer than the pre-adjustment one (72), so the MODAL row is 730.63
			// — which is what the UI displayed. The moratorium is what opened the
			// modal branch; without it the old code took schedule[0], which on this
			// screen is the interest-only moratorium payment 583.33. BOTH old
			// branches are wrong here, in different directions.
			name:   "modalTrap/moratorium+rateAdjustment",
			oracle: "amort_oracle 100000 0.07 360 12 mor=13 adj=84:0.08:",
			build: func() LoanInput {
				in := r39Loan(100000, 0.07, 360, 12, gzSettings(12, types.Basis360, false, false, false, false, false))
				in.Fancy = true
				in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput,
					FirstRepay: types.NewDateRec(2025, time.February, 1)}
				in.Adjustments = []RateAdjustment{{
					DateStatus: types.InOutInput, Date: types.NewDateRec(2031, time.January, 1),
					LoanRateStatus: types.InOutInput, LoanRate: 0.08}}
				return in
			},
			want:     672.1301,
			oldWould: 730.6334, // the MODAL row — 276 post-adjustment rows vs 72 before
		},
		{
			// THE schedule[0] TRAP. A balloon ON THE FIRST PAYMENT DATE, in DOS's
			// factory REPLACE mode: row 1 IS the balloon, 5000. No target /
			// moratorium / skip, so the old code's modal branch was excluded and it
			// took row 1 — showing 5000 for a loan whose payment is 8996.47, an
			// error of $3,996.47.
			name:   "row1Trap/balloonOnFirstPayment",
			oracle: "amort_oracle 100000 0.07 12 12 b1=5000",
			build: func() LoanInput {
				in := r39Loan(100000, 0.07, 12, 12, gzSettings(12, types.Basis360, false, false, false, false, false))
				in.Fancy = true
				in.Balloons = []BalloonPayment{{
					DateStatus: types.InOutInput, Date: types.NewDateRec(2024, time.February, 1),
					AmountStatus: types.InOutInput, Amount: 5000}}
				return in
			},
			want:     8996.4707,
			oldWould: 5000, // schedule[0] — the balloon itself, in REPLACE mode
		},
		{
			// A prepayment series under plus_regular: every regular row is
			// payment+prepay, so the modal is 3823.13 against a regular payment of
			// 32.87. This is the screen that also moved the APR by 1.8 points.
			name:   "prepaySeries/plusRegular",
			oracle: "amort_oracle 204563.47 0.0754150000 132 12 plusreg pre=5:67:12:3790.26",
			build: func() LoanInput {
				s := gzSettings(12, types.Basis360, false, false, false, false, false)
				s.PlusRegular = true
				in := r39Loan(204563.47, 0.075415, 132, 12, s)
				in.Fancy = true
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.June, 1),
					NNStatus: types.InOutInput, NN: 67,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.InOutInput, Payment: 3790.26}}
				return in
			},
			want:     32.8687,
			oldWould: 3823.13, // the MODAL row = payment + prepay, under plus_regular
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			r := Amortize(c.build())
			if r.Err != nil {
				t.Fatalf("Amortize refused: %v\n  oracle: %s", r.Err, c.oracle)
			}
			if !r.PaymentWasSolved {
				t.Errorf("PaymentWasSolved is false on a screen whose payment WAS blank — "+
					"the UI needs it to decide whether to mark the cell as output\n  oracle: %s", c.oracle)
			}
			if math.Abs(r.RegularPayment-c.want) > r39Tol {
				t.Errorf("RegularPayment %.6f, DOS %.4f (delta %.4f)\n  oracle: %s",
					r.RegularPayment, c.want, r.RegularPayment-c.want, c.oracle)
			}
			// ANTI-VACUITY. The screen must still be one where the old
			// reconstruction gives a DIFFERENT answer, or the fixture proves
			// nothing about transport.
			if math.Abs(c.want-c.oldWould) <= r39Tol {
				t.Errorf("this case no longer distinguishes transport from reconstruction: "+
					"DOS %.4f and the old reconstruction %.4f now agree. Pick another screen.",
					c.want, c.oldWould)
			}
			if math.Abs(r.RegularPayment-c.oldWould) <= r39Tol {
				t.Errorf("RegularPayment %.4f matches what the OLD schedule reconstruction "+
					"would have shown (%.4f) rather than DOS's %.4f — something is deriving "+
					"the payment from the rows again.", r.RegularPayment, c.oldWould, c.want)
			}
		})
	}
}

// --- 2. THE ADJUSTMENT ROW ---------------------------------------------------

// TestR39AdjustmentEchoVsDOS pins the Rate/Payment Adjustment echo. DOS paints
// the re-amortized payment into its own grid; before this there was nowhere for
// it to go.
//
// ⚠️ THE "has a value" TEST IS THE DISPLAY STATUS (`amountstatus = outp`), NOT
// `amtok`. DOS sets amtok only behind the balloons-or-prepay-or-exact gate at
// AMORTOP.pas:1571-1581 — its job is "a later walk may reuse this" — so a plain
// rate-only adjustment reads `amount 730.633360 amtstatus 1 amtok FALSE`. Keying
// the echo on amtok lost exactly the row DOS paints; the first version of this
// fix did that and the cell stayed blank.
func TestR39AdjustmentEchoVsDOS(t *testing.T) {
	// oracle: amort_oracle 100000 0.07 360 12 mor=13 adj=84:0.08: adjdump
	//   → adjrow 1 date 1/1/2031 rate 0.08 ratestatus 3 amount 730.633360 amtstatus 1 amtok FALSE
	in := r39Loan(100000, 0.07, 360, 12, gzSettings(12, types.Basis360, false, false, false, false, false))
	in.Fancy = true
	in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput,
		FirstRepay: types.NewDateRec(2025, time.February, 1)}
	in.Adjustments = []RateAdjustment{{
		DateStatus: types.InOutInput, Date: types.NewDateRec(2031, time.January, 1),
		LoanRateStatus: types.InOutInput, LoanRate: 0.08}}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize refused: %v", r.Err)
	}
	if len(r.Adjustments) != 1 {
		t.Fatalf("expected 1 echoed adjustment, got %d — the grid cannot be filled without it", len(r.Adjustments))
	}
	a := r.Adjustments[0]
	if !a.AmountSolved {
		t.Errorf("AmountSolved is false; DOS reports amtstatus=1 (outp) on this row. " +
			"If this fails after a refactor, check that the echo keys on the DISPLAY " +
			"status and not on amtok — amtok is FALSE here in DOS too.")
	}
	if math.Abs(a.Amount-730.6334) > r39Tol {
		t.Errorf("echoed adjustment amount %.6f, DOS 730.633360", a.Amount)
	}
	// The rate was the USER's, so it must echo as NOT solved — otherwise the UI
	// paints it green and the request builder then reads it back as blank.
	if a.RateSolved {
		t.Errorf("RateSolved is true on a rate the CALLER supplied")
	}
	if math.Abs(a.Rate-0.08) > 1e-9 {
		t.Errorf("echoed adjustment rate %.10f, want the caller's 0.08", a.Rate)
	}
}

// --- 3. THE APR ---------------------------------------------------------------

// TestR39APRvsDOSStackedOptions is the second consumer of the same transport.
// Every case leaves the PAYMENT BLANK and supplies POINTS, which is the exact
// configuration the modal heuristic corrupted: with the payment TYPED the same
// randomized differential showed 0.00% divergence at every stacking depth, and
// with it blank, 6-10%.
//
// Tolerance is 1.5e-6 in fraction terms (0.00015 percentage points) — the UI's
// own resolution (`(apr*100).toFixed(4)`) plus the oracle's 6-decimal print.
func TestR39APRvsDOSStackedOptions(t *testing.T) {
	const aprTol = 1.5e-6
	cases := []struct {
		name   string
		oracle string
		build  func() LoanInput
		want   float64
	}{
		{"prepaySeries",
			"amort_oracle 204563.47 0.0754150000 132 12 plusreg pre=5:67:12:3790.26 pts=0.0371630000 apr",
			func() LoanInput {
				s := gzSettings(12, types.Basis360, false, false, false, false, false)
				s.PlusRegular = true
				in := r39Loan(204563.47, 0.075415, 132, 12, s)
				in.Fancy = true
				in.Loan.PointsStatus, in.Loan.Points = types.InOutInput, 0.037163
				in.Prepayments = []Prepayment{{
					StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.June, 1),
					NNStatus: types.InOutInput, NN: 67,
					PerYrStatus: types.InOutInput, PerYr: 12,
					PaymentStatus: types.InOutInput, Payment: 3790.26}}
				return in
			}, 0.088206},
		{"moratorium",
			"amort_oracle 150000 0.085 120 12 plusreg mor=13 pts=0.02 apr",
			func() LoanInput {
				s := gzSettings(12, types.Basis360, false, false, false, false, false)
				s.PlusRegular = true
				in := r39Loan(150000, 0.085, 120, 12, s)
				in.Fancy = true
				in.Loan.PointsStatus, in.Loan.Points = types.InOutInput, 0.02
				in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput,
					FirstRepay: types.NewDateRec(2025, time.February, 1)}
				return in
			}, 0.089425},
		{"balloon",
			"amort_oracle 250000 0.065 180 12 plusreg b60=40000 pts=0.015 apr",
			func() LoanInput {
				s := gzSettings(12, types.Basis360, false, false, false, false, false)
				s.PlusRegular = true
				in := r39Loan(250000, 0.065, 180, 12, s)
				in.Fancy = true
				in.Loan.PointsStatus, in.Loan.Points = types.InOutInput, 0.015
				in.Balloons = []BalloonPayment{{
					DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1),
					AmountStatus: types.InOutInput, Amount: 40000}}
				return in
			}, 0.067463},
		{"rateAdjustment",
			"amort_oracle 180000 0.09 96 12 plusreg adj=36:0.07: pts=0.025 apr",
			func() LoanInput {
				s := gzSettings(12, types.Basis360, false, false, false, false, false)
				s.PlusRegular = true
				in := r39Loan(180000, 0.09, 96, 12, s)
				in.Fancy = true
				in.Loan.PointsStatus, in.Loan.Points = types.InOutInput, 0.025
				in.Adjustments = []RateAdjustment{{
					DateStatus: types.InOutInput, Date: types.NewDateRec(2027, time.January, 1),
					LoanRateStatus: types.InOutInput, LoanRate: 0.07}}
				return in
			}, 0.090039},
		{"target+skip",
			"amort_oracle 300000 0.07 240 12 plusreg targ=900 skip=6-7 pts=0.03 apr",
			func() LoanInput {
				s := gzSettings(12, types.Basis360, false, false, false, false, false)
				s.PlusRegular = true
				in := r39Loan(300000, 0.07, 240, 12, s)
				in.Fancy = true
				in.Loan.PointsStatus, in.Loan.Points = types.InOutInput, 0.03
				in.Target = Target{TargetStatus: types.InOutInput, TargetValue: 900}
				ms, _ := MonthSetFromString("6-7")
				in.SkipMonths = SkipMonths{SkipStatus: types.InOutInput, SkipStr: "6-7", MonthSet: ms}
				return in
			}, 0.074160},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			r := Amortize(c.build())
			if r.Err != nil {
				t.Fatalf("Amortize refused: %v\n  oracle: %s", r.Err, c.oracle)
			}
			if math.Abs(r.APR-c.want) > aprTol {
				t.Errorf("APR %.8f, DOS %.6f (delta %.2e)\n  oracle: %s\n"+
					"  If this regressed, check what payment the APR value walk was fed: "+
					"it must be the ENGINE's RegularPayment, never a schedule statistic.",
					r.APR, c.want, r.APR-c.want, c.oracle)
			}
		})
	}
}

// r39Loan is gzLoanInput with the loan anchored at 1 Jan 2024 (the oracle's own
// default, amort_oracle.pas:63) and the payment BLANK — every fixture here is
// about a value the engine has to solve.
func r39Loan(amount, rate float64, n, perYr int, s Settings) LoanInput {
	return gzLoanInput(amount, rate, n, perYr, s)
}
