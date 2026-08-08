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

// TestR39NateExactAdjustmentScreen pins the screen Nate reported on 2026-08-07
// as "the new amount is never filled in but it is in DOS", END TO END: the
// adjustment echo, the schedule, AND the APR, on one screen.
//
//	amort_oracle 100000 0.10 360 12 loandmy=1.1.2027 firstdmy=15.2.2027 \
//	  payhard=877.40 pts=0.03 adjdmy=15.1.2029:0.10: b365 exact prepaid adjdump apr
//	→ adjrow 1 date 1/15/2029 rate 0.10 ratestatus 3 amount 877.400000 amtstatus 1 amtok FALSE
//	→ paid 319248.23   apr 0.103658
//
// ⚠️ THE SETTINGS ARE THE WHOLE POINT OF THIS FIXTURE. The same screen returns a
// DIFFERENT adjustment amount under every other combination — 881.791201 on the
// bare defaults, 881.905837 on 365 alone, 877.612016 on 365+prepaid — and only
// `b365 exact prepaid` gives 877.40. Reconstructing a reported screen without
// pinning its Settings line is how a support report turns into a phantom defect:
// the first pass at this one compared 877.61 against a screenshot's 877.40 and
// looked like a 21-cent divergence. It was two different screens.
//
// It is ALSO the adjustment-rate-EQUALS-loan-rate case (10% → 10%). DOS still
// re-amortizes; it does not short-circuit. Under exact interest the re-solve
// lands exactly on the typed payment, which is why DOS shows 877.40 twice and
// why this screen looks like nothing happened when in fact a full Re_Amortize
// ran.
func TestR39NateExactAdjustmentScreen(t *testing.T) {
	s := gzSettings(12, types.Basis365, true, true, false, false, false)
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.10,
		NStatus: types.InOutInput, NPeriods: 360, PerYrStatus: types.InOutInput, PerYr: 12,
		PayAmtStatus: types.InOutInput, PayAmt: 877.40,
		PointsStatus: types.InOutInput, Points: 0.03,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2027, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2027, time.February, 15)},
		Settings: s, Fancy: true}
	in.Adjustments = []RateAdjustment{{
		DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 15),
		LoanRateStatus: types.InOutInput, LoanRate: 0.10}}

	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize refused: %v", r.Err)
	}
	if len(r.Adjustments) != 1 || !r.Adjustments[0].AmountSolved {
		t.Fatalf("the adjustment New Amount was not transported: %+v", r.Adjustments)
	}
	if got := r.Adjustments[0].Amount; math.Abs(got-877.40) > r39Tol {
		t.Errorf("adjustment amount %.6f, DOS 877.400000", got)
	}
	if math.Abs(r.TotalPaid-319248.23) > 0.02 {
		t.Errorf("totalPaid %.2f, DOS 319248.23", r.TotalPaid)
	}
	// The APR Nate flagged. 1.5e-6 is the UI's own toFixed(4) resolution.
	if math.Abs(r.APR-0.103658) > 1.5e-6 {
		t.Errorf("APR %.8f, DOS 0.103658", r.APR)
	}
}

// TestR39NateStackedMoratoriumTargetAdjustment is the deepest stack Nate has
// reported: moratorium + principal minimum + rate adjustment, on exact / 365 /
// prepaid / plus_regular, with points. He reported it as "worked fine with
// moratorium and principal, but diverged when I added the rate" — the APR moved
// from DOS's 11.9140% to 11.9076% and the New Amount stayed blank.
//
//	amort_oracle 100000 0.10 360 12 loandmy=1.1.2027 firstdmy=15.2.2027 \
//	  payhard=877.40 pts=0.03 mordmy=15.2.2029 targ=200 adjdmy=15.1.2029:0.12: \
//	  b365 exact prepaid plusreg adjdump apr
//	→ adjrow 1 date 1/15/2029 rate 0.12 ratestatus 3 amount 808.120000 amtstatus 1 amtok TRUE
//	→ payment 877.4000  interest 233530.63  paid 333530.63   apr 0.119140
//
// It needed a NEW ORACLE TOKEN to drive at all. `mor=MONTHS` derives the date as
// loandate + MONTHS carrying `h^.loandate.d`, so on a 1-Jan origination with a
// 15-Feb first payment it can only express day-1 moratoria — and DOS puts the
// "Int only til" date on a PAYMENT date. `mordmy=D.M.Y` (amort_oracle.pas,
// 2026-08-07) is the absolute twin, exactly as `adjdmy=` is to `adj=`.
// ⚠️ A REPORTED SCREEN THAT THE HARNESS CANNOT EXPRESS IS NOT A CLEAN SURFACE;
// it is an unmeasured one. This whole option shape was unreachable.
//
// ⚠️ `plusreg` HERE, unlike the other fixtures in this file. Nate's DOS has
// "Stated balloon includes regular pmt" = NO, and DOS's dialog inverts
// (`m_Settings.plus_regular := (box = 'No')`, ComputationalSettingsDlgUnit.pas:188),
// so NO means plus_regular TRUE. The web's NO maps to the same thing. They agree
// — do not "fix" this to match the other cases.
//
// The moratorium date is also a red herring worth recording: the web screenshot
// showed 01/27/2029 against DOS's 2/15/29, and it makes NO difference — both
// snap to the same first repay on the payment grid (snapMoratoriumFirstRepay,
// Amortize.pas:1263). Two of the three screens Nate reported carried an apparent
// date mismatch that turned out to be inert; check the snap before chasing one.
func TestR39NateStackedMoratoriumTargetAdjustment(t *testing.T) {
	s := gzSettings(12, types.Basis365, true, true, false, false, false)
	s.PlusRegular = true
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.10,
		NStatus: types.InOutInput, NPeriods: 360, PerYrStatus: types.InOutInput, PerYr: 12,
		PayAmtStatus: types.InOutInput, PayAmt: 877.40,
		PointsStatus: types.InOutInput, Points: 0.03,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2027, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2027, time.February, 15)},
		Settings: s, Fancy: true}
	in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput,
		FirstRepay: types.NewDateRec(2029, time.February, 15)}
	in.Target = Target{TargetStatus: types.InOutInput, TargetValue: 200}
	in.Adjustments = []RateAdjustment{{
		DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 15),
		LoanRateStatus: types.InOutInput, LoanRate: 0.12}}

	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize refused: %v", r.Err)
	}
	if len(r.Adjustments) != 1 || !r.Adjustments[0].AmountSolved {
		t.Fatalf("the adjustment New Amount was not transported: %+v", r.Adjustments)
	}
	if got := r.Adjustments[0].Amount; math.Abs(got-808.12) > r39Tol {
		t.Errorf("adjustment amount %.6f, DOS 808.120000", got)
	}
	if math.Abs(r.TotalPaid-333530.63) > 0.02 {
		t.Errorf("totalPaid %.2f, DOS 333530.63", r.TotalPaid)
	}
	if math.Abs(r.APR-0.119140) > 1.5e-6 {
		t.Errorf("APR %.8f, DOS 0.119140 — this is the number Nate saw as 11.9076%%", r.APR)
	}

	// The CONTROL Nate supplied himself: the same screen WITHOUT the adjustment,
	// which he said "worked fine". Keeping it here means a future regression can
	// be localised to the adjustment rather than to the stack.
	//	amort_oracle … mordmy=15.2.2029 targ=200 b365 exact prepaid plusreg apr → 0.104100
	ctl := in
	ctl.Adjustments = nil
	rc := Amortize(ctl)
	if rc.Err != nil {
		t.Fatalf("control (no adjustment) refused: %v", rc.Err)
	}
	if math.Abs(rc.APR-0.104100) > 1.5e-6 {
		t.Errorf("control APR %.8f, DOS 0.104100", rc.APR)
	}
}

// TestR39AdjustmentEchoIsDateOrderedNotRequestOrdered pins the ORDER of the
// adjustment echo, and pairs each solved value with the RIGHT date.
//
//	amort_oracle 100000 0.07 360 12 adj=60:0.08: adj=96:0.09: adjdump
//	→ adjrow 1 date 1/1/2029 rate 0.08 amount 726.522878
//	→ adjrow 2 date 1/1/2032 rate 0.09 amount 785.097272
//
// 🚨 THE ROWS ARE BUILT IN REVERSE (2032 first) ON PURPOSE. Both engines SORT
// the adjustments by date — DOS's SortAdj, dosport_entry.go's sort before the
// copy into e.adjs, validate.go's in-place sort for the piecewise engine — so
// the echo comes back in DATE order while a caller's list may be in any order.
// The first version of the UI paint loop paired echo[i] with DOM row i and put
// each row's solved payment in the OTHER row. A plausible number in the wrong
// cell is the worst kind of wrong: nothing looks broken.
//
// Every fixture in this file before this one had exactly ONE adjustment row,
// which is why the mutation `e.adjs[i+1]` → `e.adjs[len-1]` survived them all.
// R38 — vary the axis the guard is about, and one row cannot vary an ordering.
func TestR39AdjustmentEchoIsDateOrderedNotRequestOrdered(t *testing.T) {
	in := r39Loan(100000, 0.07, 360, 12, gzSettings(12, types.Basis360, false, false, false, false, false))
	in.Fancy = true
	in.Adjustments = []RateAdjustment{
		{DateStatus: types.InOutInput, Date: types.NewDateRec(2032, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.09},
		{DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1),
			LoanRateStatus: types.InOutInput, LoanRate: 0.08},
	}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize refused: %v", r.Err)
	}
	if len(r.Adjustments) != 2 {
		t.Fatalf("expected 2 echoed adjustments, got %d", len(r.Adjustments))
	}
	want := []struct {
		y, m, d int
		rate    float64
		amount  float64
	}{
		{2029, 1, 1, 0.08, 726.522878},
		{2032, 1, 1, 0.09, 785.097272},
	}
	for i, w := range want {
		got := r.Adjustments[i]
		gy, gm, gd := got.Date.Time.Year(), int(got.Date.Time.Month()), got.Date.Time.Day()
		if gy != w.y || gm != w.m || gd != w.d {
			t.Fatalf("echo[%d] date %04d-%02d-%02d, want %04d-%02d-%02d — the echo is not in "+
				"DATE order, so a client matching on date will mispair or drop rows",
				i, gy, gm, gd, w.y, w.m, w.d)
		}
		if math.Abs(got.Rate-w.rate) > 1e-9 {
			t.Errorf("echo[%d] rate %.10f, want %.10f — the row carries another row's rate",
				i, got.Rate, w.rate)
		}
		if math.Abs(got.Amount-w.amount) > r39Tol {
			t.Errorf("echo[%d] (%04d-%02d-%02d) amount %.6f, DOS %.6f — a solved payment is "+
				"paired with the WRONG date", i, gy, gm, gd, got.Amount, w.amount)
		}
		if !got.AmountSolved {
			t.Errorf("echo[%d] AmountSolved false; DOS reports amtstatus=1", i)
		}
	}
}

// TestR39APRWithNegativeRegularPayment is the fixture for the defect a review
// cycle put back in and the next one took out again.
//
//	amort_oracle 100000 0.07 96 12 plusreg pre=1:96:12:2000 pts=0.03
//	→ payment -636.6283   apr 0.078398
//
// DOS's Iterate admits a NEGATIVE regular payment — an over-funded loan, where
// the "payment" is really a draw — and the APR pass briefly carried
// `if pmt <= 0 { pmt = payoffRegularPayment(...) }`, which threw that correct
// answer away and substituted the modal schedule row. Measured cost: 25 of 136
// sampled points-bearing screens, worst 3.82 PERCENTAGE POINTS (this one: DOS
// 0.078398 vs 0.059709). Exactly the same mistake as `SolvedPrepay > 0` in
// handlers.go, one screen up and one review later.
//
// ⚠️ A SIGN IS NOT A "NO ANSWER" SIGNAL. If a future change needs a guard here,
// it must key on something that actually means "the engine produced nothing".
func TestR39APRWithNegativeRegularPayment(t *testing.T) {
	s := gzSettings(12, types.Basis360, false, false, false, false, false)
	s.PlusRegular = true
	in := r39Loan(100000, 0.07, 96, 12, s)
	in.Fancy = true
	in.Loan.PointsStatus, in.Loan.Points = types.InOutInput, 0.03
	in.Prepayments = []Prepayment{{
		StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.February, 1),
		NNStatus: types.InOutInput, NN: 96,
		PerYrStatus: types.InOutInput, PerYr: 12,
		PaymentStatus: types.InOutInput, Payment: 2000}}
	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize refused: %v", r.Err)
	}
	if r.RegularPayment >= 0 {
		t.Fatalf("ANTI-VACUITY: RegularPayment is %.4f — this fixture only tests the "+
			"negative-payment path while DOS's answer (-636.6283) is negative. Pick "+
			"another screen.", r.RegularPayment)
	}
	if math.Abs(r.RegularPayment-(-636.6283)) > r39Tol {
		t.Errorf("RegularPayment %.6f, DOS -636.6283", r.RegularPayment)
	}
	if math.Abs(r.APR-0.078398) > 1.5e-6 {
		t.Errorf("APR %.8f, DOS 0.078398 — a sign test on the payment is back", r.APR)
	}
}

// TestR39BothBlankAdjustmentOnPlainScreen pins the two facts the other fixtures
// leave unguarded: a rate cell DOS leaves BLANK must not be echoed, and a TYPED
// payment must not be reported as solved.
//
//	amort_oracle 100000 0.10 360 12 loandmy=1.1.2027 firstdmy=15.2.2027 \
//	  payhard=877.40 adjdmy=15.1.2029:: adjdump
//	→ adjrow 1 date 1/15/2029 rate 0.0000000000 ratestatus 0 amount 881.791201 amtstatus 1 amtok FALSE
//
// A PLAIN screen — no balloon, no prepayment, 360 basis, not exact — so
// Re_Amortize takes the else branch at AMORTOP.pas:1545, writes the payment with
// `amountstatus := outp`, and never reaches the `amtok := true` latch at :1571-1581.
// The second walk that would solve the RATE therefore never happens and DOS
// prints `ratestatus 0`: the rate cell stays EMPTY.
//
// ⚠️ THE ECHO MUST NOT FILL IT. `ResolvedAdjustment.Rate` is a float, so an
// unguarded echo reports a solved 0 — and the UI would paint "0.0000%" into a
// cell DOS leaves blank, which reads as a 0% rate change. That is why
// resolveAdjustmentEcho gates on a "does it have a value?" flag at all, and why
// the API's AdjustmentEcho uses POINTERS.
//
// Compare TestR39NateExactAdjustmentScreen, the SAME shape with exact+365: there
// the :1571 gate opens, the rate IS latched, and DOS prints 10.0000. The two
// fixtures differ only in the settings, which is the axis the guard is about.
func TestR39BothBlankAdjustmentOnPlainScreen(t *testing.T) {
	s := gzSettings(12, types.Basis360, false, false, false, false, false)
	in := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.10,
		NStatus: types.InOutInput, NPeriods: 360, PerYrStatus: types.InOutInput, PerYr: 12,
		PayAmtStatus: types.InOutInput, PayAmt: 877.40,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2027, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2027, time.February, 15)},
		Settings: s, Fancy: true}
	in.Adjustments = []RateAdjustment{{
		DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 15)}}

	r := Amortize(in)
	if r.Err != nil {
		t.Fatalf("Amortize refused: %v", r.Err)
	}
	// The payment was TYPED. Reporting it as solved would make the UI paint the
	// user's own number green and then read it back as blank on the next calc.
	if r.PaymentWasSolved {
		t.Errorf("PaymentWasSolved is true on a screen whose payment the CALLER typed")
	}
	if len(r.Adjustments) != 1 {
		t.Fatalf("expected 1 echoed adjustment, got %d", len(r.Adjustments))
	}
	a := r.Adjustments[0]
	if a.RateSolved || a.Rate != 0 {
		t.Errorf("the rate cell was echoed (rate=%.10f solved=%v) on a screen where DOS "+
			"reports ratestatus=0 — an empty cell would be painted as 0.0000%%",
			a.Rate, a.RateSolved)
	}
	if !a.AmountSolved || math.Abs(a.Amount-881.791201) > r39Tol {
		t.Errorf("adjustment amount %.6f solved=%v, DOS 881.791201 amtstatus=1",
			a.Amount, a.AmountSolved)
	}
}

// ⚠️ KNOWN-UNPINNED, 2026-08-07, recorded rather than faked. Mutation testing
// leaves ONE mutant alive in this file's scope: relaxing the echo's
// "was this the user's?" test on the adjustment RATE from `>= InOutDefault` to
// `> InOutDefault`. It is unreachable today because nothing sets an adjustment
// rate to `defp` (2) — every caller uses `inp` (3) or leaves it `empty` (0) —
// so no screen can distinguish the two. R35: a documented trap is not a guard;
// this is on the backlog, not closed.
