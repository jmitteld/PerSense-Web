package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Regression guard for the DISPLAY VERY-LAST FOLD firing at the last REGULAR
// payment instead of at very_last (docs/discrepancies.md §52).
//
// ROOT CAUSE. DOS's fold is keyed on `very_last` and on nothing else —
// PrintAndReset, AMORTOP.pas:1004:
//
//	if (DateComp(date,very_last)=0) then begin
//	 {Adjust last payment to cover entire remaining principal.}
//	    payamt:=payamt+principal;
//	    cumamt:=cumamt+principal;
//	    principal:=0;
//	    end;
//
// and `very_last` is the LATEST of the last payment date, the last balloon date
// and every prepayment series' stop date — DetermineVeryLast, AMORTOP.pas:1293-1304:
//
//	if (nballoons > 0) and (DateComp(balloon[nballoons]^.date, h^.lastdate) > 0) then
//	  very_last := balloon[nballoons]^.date
//	else
//	  very_last := h^.lastdate;
//	for i := 1 to npre do
//	  if (DateComp(pre[i]^.stopdate, very_last) > 0) then
//	    very_last := pre[i]^.stopdate;
//
// So when a prepayment series runs PAST the last regular payment, DOS keeps
// walking and emits prepayment-only rows until the series stops (or the balance
// retires), and folds any residual only on the very last of them.
//
// In the port, `terminalRow` is `payNum >= loan.NPeriods || atVeryLast`, i.e. it
// goes true at the last REGULAR payment even when veryLast is later. Three of
// the four fold branches in generateFancyScheduleMode carried an `atVeryLast`
// guard; the ARM branch (`terminalRow && len(input.Adjustments) > 0`) did not,
// and it is checked FIRST. On a loan carrying BOTH an adjustment and a trailing
// prepayment series it therefore won the race, folded the residual into the last
// regular row, and ended the schedule there — swallowing every trailing row.
//
// Measured before the guard, on case A below: DOS emitted 200 rows and 68710.43
// of interest; the port emitted 180 rows and 67086.46, dumping 5662.07 of
// principal into row 180.
//
// WHY IT LOOKED EXOTIC. Removing any one of three conditions made the port
// agree — no adjustment (the prepayment branch's own guard then applies), a stop
// date inside the term (nothing to swallow), or a prepayment frequency at or
// above the payment frequency (both engines refuse the overshoot outright). It
// reads as a three-way option interaction and is in fact one missing guard.
//
// The class was invisible to TestDOSFuzzer5AllAdvancedOptions, which cannot
// generate it: that generator draws `ppy` only from {12, 24, 26, 52} and caps
// `maxNN` at `(remMonths * ppy) / 12` so the series can never overshoot.
//
// PROVENANCE. Every golden is from the real DOS engine via
// legacy/oracle/amort_oracle (built by legacy/oracle/build_linux.sh):
//
//	amort_oracle 100000 0.08 180 12 loandmy=1.1.2024 firstdmy=1.2.2024 \
//	             adj=44:0.0706: pre=21:45:2:351.90
//	  -> payment 1038.8268 interest 68710.43 paid 168710.43   (200 rows)
func TestDOSPrepayOvershootVeryLastFold(t *testing.T) {
	// adj=N / pre=N are N months after the LOAN date, day carried over
	// (amort_oracle.pas:254-258, :310-314), clamped down by
	// CheckForDaysTooLarge — never rolled forward.
	monthsAfter := func(loan types.DateRec, months int) types.DateRec {
		tot := (int(loan.Time.Month()) - 1) + months
		y := loan.Time.Year() + tot/12
		m := time.Month(tot%12 + 1)
		d := loan.Time.Day()
		if dim := types.NewDateRec(y, m, 1).Time.AddDate(0, 1, -1).Day(); d > dim {
			d = dim
		}
		return types.NewDateRec(y, m, d)
	}

	type prepay struct {
		startMonths, nn, perYr int
		amount                 float64
	}
	type adj struct {
		months int
		rate   float64
	}

	loanD := types.NewDateRec(2024, time.January, 1)
	firstD := types.NewDateRec(2024, time.February, 1)

	run := func(t *testing.T, adjs []adj, pres []prepay) AmortResult {
		t.Helper()
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 100000,
				LoanRateStatus: types.InOutInput, LoanRate: 0.08,
				NStatus: types.InOutInput, NPeriods: 180,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: loanD,
				FirstStatus: types.InOutInput, FirstDate: firstD,
			},
			Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
			Fancy:    true,
		}
		for _, a := range adjs {
			in.Adjustments = append(in.Adjustments, RateAdjustment{
				DateStatus: types.InOutInput, Date: monthsAfter(loanD, a.months),
				LoanRateStatus: types.InOutInput, LoanRate: a.rate,
			})
		}
		for _, p := range pres {
			in.Prepayments = append(in.Prepayments, Prepayment{
				StartDateStatus: types.InOutInput, StartDate: monthsAfter(loanD, p.startMonths),
				NNStatus: types.InOutInput, NN: p.nn,
				PerYrStatus: types.InOutInput, PerYr: p.perYr,
				PaymentStatus: types.InOutInput, Payment: p.amount,
			})
		}
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("Amortize: %v", res.Err)
		}
		return res
	}

	cases := []struct {
		name     string
		adjs     []adj
		pres     []prepay
		wantInt  float64
		wantPaid float64
		note     string
	}{
		{
			// THE CASE. Stop month 21 + 44*6 = 285, far past the 180-month term.
			name: "adjustment + prepay series overshooting the term",
			adjs: []adj{{44, 0.0706}},
			pres: []prepay{{21, 45, 2, 351.90}},
			// amort_oracle ... adj=44:0.0706: pre=21:45:2:351.90
			wantInt:  68710.43,
			wantPaid: 168710.43,
			note:     "port produced 67086.46 in 180 rows; DOS walks 200 rows to the prepay stop date",
		},
		{
			// THE BOUNDARY, one period past the term: stop month 183 > 180.
			// Smallest overshoot that still diverged before the guard.
			name: "one period past the term",
			adjs: []adj{{44, 0.0706}},
			pres: []prepay{{21, 28, 2, 351.90}},
			// amort_oracle ... pre=21:28:2:351.90
			wantInt:  65201.31,
			wantPaid: 165201.31,
			note:     "port produced 65195.20 before the guard",
		},
		{
			// CONTROL — one period earlier: stop month 177 <= 180, nothing
			// trails the last regular payment, so the fold was already correct.
			// Pins the overshoot as a necessary condition.
			name: "one period inside the term",
			adjs: []adj{{44, 0.0706}},
			pres: []prepay{{21, 27, 2, 351.90}},
			// amort_oracle ... pre=21:27:2:351.90
			wantInt:  65044.19,
			wantPaid: 165044.19,
		},
		{
			// Same boundary at a different prepayment frequency: stop month
			// 21 + 54*3 = 183 > 180. Shows the condition is the overshoot, not
			// a particular ppy.
			name: "quarterly prepay, one period past the term",
			adjs: []adj{{44, 0.0706}},
			pres: []prepay{{21, 55, 4, 351.90}},
			// amort_oracle ... pre=21:55:4:351.90
			wantInt:  63278.86,
			wantPaid: 163278.86,
		},
		{
			// CONTROL — quarterly, stop month exactly 180. Equality is NOT an
			// overshoot; very_last == lastdate and the old code was right.
			name: "quarterly prepay stopping exactly on the last payment",
			adjs: []adj{{44, 0.0706}},
			pres: []prepay{{21, 54, 4, 351.90}},
			// amort_oracle ... pre=21:54:4:351.90
			wantInt:  63118.52,
			wantPaid: 163118.52,
		},
		{
			// CONTROL — the SAME overshooting prepayment with no adjustment.
			// Already correct before the fix (the prepayment fold branch carries
			// its own atVeryLast guard); must stay correct. Pins the adjustment
			// as the third necessary condition and proves the guard did not
			// disturb the branch that already worked.
			name: "overshoot with no adjustment",
			pres: []prepay{{21, 45, 2, 351.90}},
			// amort_oracle 100000 0.08 180 12 loandmy=1.1.2024 firstdmy=1.2.2024 pre=21:45:2:351.90
			wantInt:  74776.00,
			wantPaid: 174776.00,
		},
		{
			// The adjustment's POSITION is irrelevant — here it sits before the
			// prepayment window opens and the case still diverged before the fix.
			name: "adjustment before the prepay window opens",
			adjs: []adj{{12, 0.0706}},
			pres: []prepay{{21, 45, 2, 351.90}},
			// amort_oracle ... adj=12:0.0706: pre=21:45:2:351.90
			wantInt:  66130.12,
			wantPaid: 166130.12,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, tc.adjs, tc.pres)
			if math.Abs(res.TotalInt-tc.wantInt) > 0.005 {
				extra := ""
				if tc.note != "" {
					extra = "\n  " + tc.note
				}
				t.Errorf("TotalInt = %.2f, want DOS %.2f (delta %.2f)%s",
					res.TotalInt, tc.wantInt, res.TotalInt-tc.wantInt, extra)
			}
			if math.Abs(res.TotalPaid-tc.wantPaid) > 0.005 {
				t.Errorf("TotalPaid = %.2f, want DOS %.2f (delta %.2f)",
					res.TotalPaid, tc.wantPaid, res.TotalPaid-tc.wantPaid)
			}
		})
	}

	// The ARM fold branch this guard sits on exists for a real reason — a
	// negative-amortizing ARM whose last SCHEDULED payment must balloon to
	// retire the balance. With no trailing option, very_last == lastdate, so the
	// guard is inert there. This case is the proof it stayed inert.
	//
	//	amort_oracle 100000 0.08 240 12 payhard=600 usa adj=60:0.12:
	//	  -> payment 600.0000 interest 158743.42 paid 258743.42
	t.Run("ARM neg-am fold still fires with no trailing option", func(t *testing.T) {
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 100000,
				LoanRateStatus: types.InOutInput, LoanRate: 0.08,
				NStatus: types.InOutInput, NPeriods: 240,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus: types.InOutInput, PayAmt: 600,
			},
			Settings: Settings{
				Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360,
				USARule: true,
			},
			Fancy: true,
			Adjustments: []RateAdjustment{{
				DateStatus:     types.InOutInput,
				Date:           types.NewDateRec(2029, time.January, 1),
				LoanRateStatus: types.InOutInput, LoanRate: 0.12,
			}},
		}
		in.Loan.LoanDate = types.NewDateRec(2024, time.January, 1)
		in.Loan.LoanDateStatus = types.InOutInput
		in.Loan.FirstDate = types.NewDateRec(2024, time.February, 1)
		in.Loan.FirstStatus = types.InOutInput
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("Amortize: %v", res.Err)
		}
		if len(res.Schedule) == 0 {
			t.Fatal("empty schedule")
		}
		// The point is the FOLD, not the exact total: the last row must retire
		// the loan rather than leave the neg-am residual outstanding.
		last := res.Schedule[len(res.Schedule)-1]
		if math.Abs(last.Principal) > 0.005 {
			t.Errorf("final remaining principal = %.2f, want 0 — the ARM neg-am "+
				"fold did not fire; the atVeryLast guard must be inert when "+
				"nothing trails the last regular payment", last.Principal)
		}
		// amort_oracle 100000 0.08 240 12 payhard=600 usa adj=60:0.12:
		//   -> interest 158743.42 paid 258743.42
		if math.Abs(res.TotalInt-158743.42) > 0.005 {
			t.Errorf("TotalInt = %.2f, want DOS 158743.42", res.TotalInt)
		}
	})
}
