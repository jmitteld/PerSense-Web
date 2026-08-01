package amortization

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Engine-level regression guard for docs/discrepancies.md §56 — the Exact
// setting at the 360-day basis is a no-op for the payment SOLVE but NOT for the
// schedule DISPLAY.
//
// DOS gates four of the five computational readers of `df.c.exact` on the basis
// (AMORTOP.pas:625, :1438, :1464, :1571; Amortize.pas:458), so at x360 the
// payment is solved by the nominal RepayLoan recursion. The DISPLAY dispatch is
// NOT gated:
//
//	Amortize.pas:1493
//	  if (fancy) or ((df.c.exact) and (not df.c.R78))
//	             or (not (df.c.basis=x360)) then RepayFancyLoan
//
// so the ROWS come from the date-walking engine. The port's exactDaily() helper
// (`Exact && Basis != 360`) collapsed all five readers together and left display
// on generateSimpleSchedule.
//
// WHY THIS HID FOR SO LONG. On any grid whose period is a whole number of months
// the two engines agree algebraically, not coincidentally: AddPeriod pins
// d := orig_day every period (INTSUTIL.pas:1240), so DaysCloseEnough is always
// true, ComputeNext takes the nominal branch (AMORTOP.pas:627), and
// timedif = Δm/12 = 1/peryr — which is exactly RepayLoan's
// f-1 = loanrate/RealPerYr(peryr). peryr 26 and 52 never see a real 360 basis at
// all, because coerceSubMonthlyBasis rewrites it to 365 upstream
// (Amortize.pas:297-303) and exactDaily then fires on its own.
//
// That leaves peryr=24 as the only frequency where the ungated gate is
// observable, and cases A and B are it. The semi-monthly AddPeriod branch
// (INTSUTIL.pas:1217-1238) walks d±15 instead of pinning the anchor, so
// DaysCloseEnough fails and timedif comes from YearsDif's 30/360 rules — which
// are not 15/360 across February.
//
// BOTH DIRECTIONS — the gate has THREE independent terms and each one has its
// own failure signature:
//
//	delete the whole arm            -> A and B render their exact-OFF answers
//	                                   (A: 65385.02 / 105991.41 over 600 rows
//	                                   instead of 63873.37 / 104479.76 over 592);
//	                                   C, D, E, F stay green
//	drop `!settings.R78`            -> E alone flips, to 63873.37 / 104479.76
//	drop `!wholeMonthGrid(PerYr)`   -> F alone fails, 176010.29 against DOS's
//	                                   175844.60 and one row short
//
// The four inert cases are what prove the new branch did not swallow the
// whole-month grids. F is the one that was found the hard way: the literal DOS
// gate, with no frequency term, regressed it (it is also round 12's
// zzyear_byte_horizon_test.go case E) because the port's walk dates drift at
// February 2100 while §54 is deferred and DOS's do not.
func TestDOSExact360RendersWithTheDateWalk(t *testing.T) {
	type screen struct {
		amount   float64
		rate     float64
		n, perYr int
		loanY    int
		loanM    time.Month
		loanD    int
		firstY   int
		firstM   time.Month
		firstD   int
		exact    bool
		r78      bool
	}

	run := func(t *testing.T, s screen) AmortResult {
		t.Helper()
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: s.amount,
				LoanRateStatus: types.InOutInput, LoanRate: s.rate,
				NStatus: types.InOutInput, NPeriods: s.n,
				PerYrStatus: types.InOutInput, PerYr: s.perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput,
				LoanDate:       types.NewDateRec(s.loanY, s.loanM, s.loanD),
				FirstStatus:    types.InOutInput,
				FirstDate:      types.NewDateRec(s.firstY, s.firstM, s.firstD),
			},
			Settings: Settings{
				Basis: types.Basis360, PerYr: byte(s.perYr),
				YrDays: 360, YrInv: 1.0 / 360,
				Exact: s.exact, R78: s.r78,
			},
		}
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("Amortize: %v", res.Err)
		}
		return res
	}

	cases := []struct {
		name     string
		s        screen
		wantPmt  float64
		wantInt  float64
		wantPaid float64
		// wantRows is the PORT's schedule length, not DOS's dumpraw `lines`
		// count — dumpraw emits non-payment display lines too, so the two run
		// +2 apart on a full-term schedule and +3 where the loan retires early.
		// It is asserted because it is what pins the early retirement: case A
		// must lose 8 rows relative to its exact-OFF shape (case E's 600).
		wantRows int
		oracle   string
	}{
		{
			// THE FIX. A 29th-anchor semi-monthly grid walks 29th/14th. Every
			// half-period is 15/360 EXCEPT the two that touch February:
			//   2/14/22 -> 2/28/22  the 29th clamped by CheckForDaysTooLarge,
			//                       so YearsDif gives 14/360 (row interest 148.21)
			//   2/28/22 -> 3/14/22  INTSUTIL.pas:800's `(a.m=2) and (a.d>27)`
			//                       correction, 14/360 (row interest 148.11)
			// Every other row is bit-identical to the nominal engine's, which is
			// why the totals differ by only 1511.65 over 600 periods — and why
			// the schedule retires SEVEN rows early.
			name: "A/semimonthly-29th-anchor (the §56 repro)",
			s: screen{
				amount: 40606.39, rate: 0.094051, n: 600, perYr: 24,
				loanY: 2021, loanM: time.June, loanD: 29,
				firstY: 2021, firstM: time.July, firstD: 29,
				exact: true,
			},
			wantPmt: 176.6524, wantInt: 63873.37, wantPaid: 104479.76,
			wantRows: 592,
			oracle: "amort_oracle 40606.39 0.094051 600 24 loandmy=29.6.2021 " +
				"firstdmy=29.7.2021 exact -> payment 176.6524 interest 63873.37 " +
				"paid 104479.76 (dumpraw: lines 595)",
		},
		{
			// An INDEPENDENT screen for the same mechanism, drawn by
			// long_horizon_sweep.py seed 913 stratum A. Different amount, rate,
			// term and anchor month; same 29th anchor, same February leak.
			name: "B/semimonthly-29th-anchor-independent-draw",
			s: screen{
				amount: 274179.66, rate: 0.036833, n: 360, perYr: 24,
				loanY: 2022, loanM: time.October, loanD: 29,
				firstY: 2022, firstM: time.November, firstD: 29,
				exact: true,
			},
			wantPmt: 993.3682, wantInt: 82839.82, wantPaid: 357019.48,
			wantRows: 360,
			oracle: "amort_oracle 274179.66 0.036833 360 24 loandmy=29.10.2022 " +
				"firstdmy=29.11.2022 exact -> payment 993.3682 interest 82839.82 " +
				"paid 357019.48 (dumpraw: lines 363)",
		},
		{
			// INERT 1 — the whole-month grid. The new arm FIRES here (exact is
			// on, R78 is off, basis is 360) and must be a no-op, because
			// DaysCloseEnough holds on every period. This is the case the
			// 2026-07-25 seed-20110 finding was protecting: `exact` must not
			// move a monthly answer at the 360 basis.
			name: "C/inert-on-a-monthly-grid",
			s: screen{
				amount: 40606.39, rate: 0.094051, n: 120, perYr: 12,
				loanY: 2021, loanM: time.June, loanD: 29,
				firstY: 2021, firstM: time.July, firstD: 29,
				exact: true,
			},
			wantPmt: 523.3292, wantInt: 22193.12, wantPaid: 62799.51,
			wantRows: 120,
			oracle: "amort_oracle 40606.39 0.094051 120 12 loandmy=29.6.2021 " +
				"firstdmy=29.7.2021 exact -> payment 523.3292 interest 22193.12 " +
				"paid 62799.51 (dumpraw: lines 122)",
		},
		{
			// INERT 2 — semi-monthly, but on the ONE anchor DOS protects. A 15th
			// anchor walks 15th/30th, and LastDayFn(d=15, peryr=24) is the
			// explicit `(peryr=24) and (d=15)` special case at INTSUTIL.pas:923,
			// so DaysCloseEnough holds and ComputeNext's half-month snap
			// (AMORTOP.pas:628-629) pins timedif at exactly 1/24. Same frequency
			// as A, opposite verdict: the mechanism is the ANCHOR, not peryr=24
			// as such, and a fix that keyed on peryr would have been wrong here.
			name: "D/inert-on-the-15th-anchor-semimonthly-grid",
			s: screen{
				amount: 40606.39, rate: 0.094051, n: 240, perYr: 24,
				loanY: 2021, loanM: time.June, loanD: 15,
				firstY: 2021, firstM: time.July, firstD: 15,
				exact: true,
			},
			wantPmt: 262.3772, wantInt: 22364.13, wantPaid: 62970.52,
			wantRows: 240,
			oracle: "amort_oracle 40606.39 0.094051 240 24 loandmy=15.6.2021 " +
				"firstdmy=15.7.2021 exact -> payment 262.3772 interest 22364.13 " +
				"paid 62970.52 (dumpraw: lines 242)",
		},
		{
			// INERT 3 — R78 SUPPRESSES the exact display gate. This is case A's
			// exact screen with r78 added, and DOS renders A's exact-OFF answer:
			// `(df.c.exact) and (not df.c.R78)` is false, and the other two
			// disjuncts (fancy, non-360) are false too. Dropping `!settings.R78`
			// from the new arm turns this into 63873.37 / 104479.76.
			name: "E/r78-suppresses-the-exact-display-gate",
			s: screen{
				amount: 40606.39, rate: 0.094051, n: 600, perYr: 24,
				loanY: 2021, loanM: time.June, loanD: 29,
				firstY: 2021, firstM: time.July, firstD: 29,
				exact: true, r78: true,
			},
			wantPmt: 176.6524, wantInt: 65385.02, wantPaid: 105991.41,
			wantRows: 600,
			oracle: "amort_oracle 40606.39 0.094051 600 24 loandmy=29.6.2021 " +
				"firstdmy=29.7.2021 exact r78 -> payment 176.6524 interest " +
				"65385.02 paid 105991.41 (dumpraw: lines 602)",
		},
		{
			// INERT 4 — the wholeMonthGrid() term, and the reason it exists. A
			// 1080-period MONTHLY exact loan whose horizon reaches 2119. DOS is
			// indifferent to which of its two engines renders this (they are the
			// same computation on a whole-month grid), so the port must stay on
			// the nominal one: its walk dates drift a month at February 2100
			// because §54 (DOS's leap rule has no century correction) is
			// deferred, and routing this screen through the date walk renders
			// 176010.29 over 1079 rows. Same screen as case E of
			// zzyear_byte_horizon_test.go, asserted here for the opposite
			// reason — there it guards the §55 horizon clamp, here the §56 gate.
			name: "F/inert-on-a-long-monthly-grid-past-feb-2100",
			s: screen{
				amount: 114948.20, rate: 0.025189, n: 1080, perYr: 12,
				loanY: 2029, loanM: time.April, loanD: 29,
				firstY: 2029, firstM: time.May, firstD: 29,
				exact: true,
			},
			wantPmt: 269.2526, wantInt: 175844.60, wantPaid: 290792.80,
			wantRows: 1080,
			oracle: "amort_oracle 114948.20 0.025189 1080 12 loandmy=29.4.2029 " +
				"firstdmy=29.5.2029 exact -> payment 269.2526 interest 175844.60 " +
				"paid 290792.80 (dumpraw: lines 1082)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, tc.s)
			// The payment is solved by RepayLoan on BOTH sides of this gate
			// (AMORTOP.pas:1438 is basis-guarded), so it must be identical in
			// every case here — including the two the display arm moves.
			got := 0.0
			for _, r := range res.Schedule {
				if r.PayNum >= 1 && r.PayAmt > 0 {
					got = r.PayAmt
					break
				}
			}
			if math.Abs(got-tc.wantPmt) > 5e-5 {
				t.Errorf("payment = %.4f, DOS %.4f\n  oracle: %s",
					got, tc.wantPmt, tc.oracle)
			}
			if math.Abs(res.TotalInt-tc.wantInt) > 0.005 {
				t.Errorf("interest = %.2f, DOS %.2f (delta %.2f)\n  oracle: %s",
					res.TotalInt, tc.wantInt,
					res.TotalInt-tc.wantInt, tc.oracle)
			}
			if math.Abs(res.TotalPaid-tc.wantPaid) > 0.005 {
				t.Errorf("paid = %.2f, DOS %.2f (delta %.2f)\n  oracle: %s",
					res.TotalPaid, tc.wantPaid, res.TotalPaid-tc.wantPaid, tc.oracle)
			}
			if tc.wantRows > 0 && len(res.Schedule) != tc.wantRows {
				t.Errorf("rows = %d, want %d\n  oracle: %s",
					len(res.Schedule), tc.wantRows, tc.oracle)
			}
		})
	}
}

// The §56 display gate reached through a BACKWARD SOLVE — the case that was
// nearly mis-diagnosed as a separate defect, and the strongest evidence the fix
// is right.
//
// DOS solves the rate with Iterate (AMORTOP.pas:1438, basis-GUARDED, so the
// nominal RepayLoan) and then renders the table from the solved answer through
// Amortize.pas:1493 (NOT guarded). The port must do the same, which is why the
// display arm in engine.go carries `!input.inBackwardSolve`: the solver's inner
// trials stay nominal, the final table does not.
//
// Measured against the real DOS engine on the 29th anchor (leaks) and the 15th
// (protected by LastDayFn), at three hard payments:
//
//	amort_oracle 40606.39 0.094051 600 24 loandmy=29.6.2021 firstdmy=29.7.2021 \
//	  exact payhard=<pay> norate
//
//	pay      DOS solvedrate   DOS int / paid        port BEFORE §56
//	176.65   0.0940493440     63871.65 / 104478.04  65383.41 / 105989.80
//	200.00   0.1101240219     76987.20 / 117593.59  79394.02 / 120000.41
//	250.00   0.1427016805    103676.47 / 144282.86 109393.48 / 149999.87
//
// The SOLVED RATE agreed with DOS to ten decimals before and after — the rate
// solve was never the problem. What the port got wrong was the schedule it drew
// from that rate: on the 29th anchor it rendered the 15th anchor's numbers (note
// the "before" column is the 15th anchor's answer, and ran the full 600 rows
// instead of retiring at 592/588/578). §56 closes all three exactly.
//
// The 15th-anchor rows are the inertness half: DaysCloseEnough holds there, so
// the arm fires and must change nothing. They match DOS before and after.
func TestDOSExact360SolvedRateRendersWithTheDateWalk(t *testing.T) {
	cases := []struct {
		anchor            int
		pay               float64
		wantRate          float64
		wantInt, wantPaid float64
		wantRows          int
	}{
		{29, 176.65, 0.0940493440, 63871.65, 104478.04, 592},
		{29, 200.00, 0.1101240219, 76987.20, 117593.59, 588},
		{29, 250.00, 0.1427016805, 103676.47, 144282.86, 578},
		{15, 176.65, 0.0940493440, 65383.41, 105989.80, 600},
		{15, 200.00, 0.1101240219, 79394.02, 120000.41, 600},
		{15, 250.00, 0.1427016805, 109393.48, 149999.87, 600},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("anchor%d/pay%.2f", tc.anchor, tc.pay), func(t *testing.T) {
			in := LoanInput{
				Loan: Loan{
					AmountStatus: types.InOutInput, Amount: 40606.39,
					LoanRateStatus: types.StatusEmpty,
					NStatus:        types.InOutInput, NPeriods: 600,
					PerYrStatus: types.InOutInput, PerYr: 24,
					PayAmtStatus: types.InOutInput, PayAmt: tc.pay,
					LoanDateStatus: types.InOutInput,
					LoanDate:       types.NewDateRec(2021, time.June, tc.anchor),
					FirstStatus:    types.InOutInput,
					FirstDate:      types.NewDateRec(2021, time.July, tc.anchor),
				},
				Settings: Settings{Basis: types.Basis360, PerYr: 24,
					YrDays: 360, YrInv: 1.0 / 360, Exact: true},
			}
			v, _, err := SolveRate(in)
			if err != nil {
				t.Fatalf("SolveRate: %v", err)
			}
			if math.Abs(v-tc.wantRate) > 5e-10 {
				t.Errorf("solved rate = %.10f, DOS %.10f", v, tc.wantRate)
			}
			in.Loan.LoanRateStatus, in.Loan.LoanRate = types.InOutInput, v
			r := Amortize(in)
			if r.Err != nil {
				t.Fatalf("Amortize: %v", r.Err)
			}
			if math.Abs(r.TotalInt-tc.wantInt) > 0.005 {
				t.Errorf("interest = %.2f, DOS %.2f (delta %.2f)",
					r.TotalInt, tc.wantInt, r.TotalInt-tc.wantInt)
			}
			if math.Abs(r.TotalPaid-tc.wantPaid) > 0.005 {
				t.Errorf("paid = %.2f, DOS %.2f (delta %.2f)",
					r.TotalPaid, tc.wantPaid, r.TotalPaid-tc.wantPaid)
			}
			if len(r.Schedule) != tc.wantRows {
				t.Errorf("rows = %d, want %d", len(r.Schedule), tc.wantRows)
			}
		})
	}
}
