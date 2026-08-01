package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Regression guard for the PIECEWISE half of the adjustment pre-pass lastdate
// leak (docs/discrepancies.md §50; round 8 closed the STRUCTURAL half, guarded
// by TestDOSAdjustmentPrePassLastDateSnap in zzadj_prepass_lastdate_test.go).
//
// Read that test first — it carries the full DOS derivation. In brief: DOS's
// pre-pass (Amortize.pas:1408-1412) calls EstimateAndRefineAdjPayment once per
// rate-only adjustment; every payment it solves is discarded, but the
// unguarded VAR parameter at
//
//	n := NumberOfInstallments(adj[next_adj]^.date, h^.lastdate, h^.peryr, on_or_after);
//	                                                     {AMORTOP.pas:1547}
//
// snaps `h^.lastdate` onto the payment grid IN PLACE (INTSUTIL.pas:1003 + :1018)
// and nothing restores it. Every later Re_Amortize counts against the snapped
// date.
//
// THE PIECEWISE ENGINE HAD THE SAME DEFECT TWICE, and a case with a prepayment
// routes here rather than to the structural port (dosPortCanHandle declines
// prepayments), so round 8's fix did not reach it:
//
//  1. engine.go gated runAdjustmentPrePass on `hardPayment`. DOS's SOLVE LOOP
//     (Amortize.pas:1408) has no such gate — only the Dav Holle Round2 sweep
//     that follows it (Amortize.pas:1430-1435 `if (hard_payment) and (fancy)`)
//     is hard-payment conditional. With the payment solved rather than given,
//     the pre-pass never ran and the display walk started from a pristine
//     lastdate.
//  2. The snap the walk itself performed was written to
//     result.reAmortLastDate — a DISPLAY cell — and never fed back to the
//     NumberOfInstallmentsRaw call that the NEXT adjustment makes.
//
// Both halves are load-bearing and were verified independently: with only (1)
// applied the primary case still reported 60283.55, and with only (2) applied
// it still reported 60283.55. The payhard case below fails on (2) alone, which
// is why it is here.
//
// WHY NOT JUST WRITE THE SNAP INTO loan.LastDate. That was tried on 2026-07-30
// and moved an otherwise-exact schedule by 6838.28 — the piecewise walk reads
// loan.LastDate at sites where DOS reads `very_last`, and unlike the structural
// port its horizon is not pinned before the walk. The fix therefore threads the
// snapped date through a dedicated `adjLastDate` local that ONLY the
// AMORTOP.pas:1547 counterpart reads. See
// claude/lost_session_recovery_and_reamortize_correction_2026-07-30.md.
//
// PROVENANCE. Every golden is from the real DOS engine via
// legacy/oracle/amort_oracle (built by legacy/oracle/build_linux.sh), token
// language `<amount> <rate> <n> <peryr> loandmy= firstdmy= adj=<months>:<rate>:
// pre=<startMonths>:<NN>:<perYr>:<amount>`:
//
//	amort_oracle 100000 0.08 144 12 loandmy=30.6.2023 firstdmy=30.8.2023 \
//	             adj=67:0.04: adj=77:0.11: pre=20:24:12:150
//	  -> payment 1078.3207 interest 60633.01 paid 160633.01
func TestDOSAdjustmentPrePassPiecewise(t *testing.T) {
	// adj=N / pre=N in the oracle's token language is N months after the LOAN
	// date (amort_oracle.pas:172-176, :254-258, :310-314): the day is carried
	// over unchanged, the month/year stepped by N.
	monthsAfter := func(loan types.DateRec, months int) types.DateRec {
		tot := (int(loan.Time.Month()) - 1) + months
		y := loan.Time.Year() + tot/12
		m := time.Month(tot%12 + 1)
		d := loan.Time.Day()
		// CheckForDaysTooLarge clamps DOWN to the month length; it does not
		// roll into the next month the way time.Date would.
		if dim := types.NewDateRec(y, m, 1).Time.AddDate(0, 1, -1).Day(); d > dim {
			d = dim
		}
		return types.NewDateRec(y, m, d)
	}

	type adj struct {
		months int
		rate   float64
	}

	run := func(t *testing.T, loanD, firstD types.DateRec, n int, adjs []adj, hardPay float64) AmortResult {
		t.Helper()
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 100000,
				LoanRateStatus: types.InOutInput, LoanRate: 0.08,
				NStatus: types.InOutInput, NPeriods: n,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: loanD,
				FirstStatus: types.InOutInput, FirstDate: firstD,
			},
			Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
			Fancy:    true,
			// The prepayment is what forces the PIECEWISE engine — without it
			// dosPortCanHandle routes the screen to the structural port, which
			// round 8 already fixed. It also satisfies DOS's own gate for the
			// nested til_adj Iterate (`(user_nballoons > 0) or (npre > 0) or
			// ((exact) and (basis<>x360))`, AMORTOP.pas:1571), which is what
			// makes the pre-pass store an amount at all.
			Prepayments: []Prepayment{{
				StartDateStatus: types.InOutInput, StartDate: monthsAfter(loanD, 20),
				NNStatus: types.InOutInput, NN: 24,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: 150,
			}},
		}
		if hardPay > 0 {
			// payhard= — a user-supplied regular payment. This is the arm where
			// the Dav Holle Round2 sweep fires, so it exercises the rounding
			// split the fix introduced inside runAdjustmentPrePass.
			in.Loan.PayAmtStatus, in.Loan.PayAmt = types.InOutInput, hardPay
		}
		for _, a := range adjs {
			in.Adjustments = append(in.Adjustments, RateAdjustment{
				DateStatus: types.InOutInput, Date: monthsAfter(loanD, a.months),
				LoanRateStatus: types.InOutInput, LoanRate: a.rate,
			})
		}
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("Amortize: %v", res.Err)
		}
		return res
	}

	jun30 := types.NewDateRec(2023, time.June, 30)
	aug30 := types.NewDateRec(2023, time.August, 30)
	jun1 := types.NewDateRec(2023, time.June, 1)
	aug1 := types.NewDateRec(2023, time.August, 1)

	cases := []struct {
		name     string
		loanD    types.DateRec
		firstD   types.DateRec
		n        int
		adjs     []adj
		hardPay  float64
		wantInt  float64
		wantPaid float64
		note     string
	}{
		{
			// THE CASE. adj 2 lands 30 Nov 2029 — a month end — and the terminal
			// month is July (31 days) against a day-30 grid, so daysinm moves it.
			name:  "solved payment, later adjustment on a month end",
			loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{67, 0.04}, {77, 0.11}},
			wantInt:  60633.01,
			wantPaid: 160633.01,
			note:     "port produced 60283.55 before the pre-pass gate was removed",
		},
		{
			// Same shape, adj 2 on 30 Apr 2029 — also a month end, also snaps.
			name:  "solved payment, later adjustment on a 30-day month end (April)",
			loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{67, 0.04}, {70, 0.11}},
			wantInt:  63510.97,
			wantPaid: 163510.97,
			note:     "port produced 63153.71 before the fix",
		},
		{
			// THE SECOND HALF, ISOLATED. hardPayment is true here, so the
			// pre-pass ALREADY ran before this change — this case fails on the
			// display-cell leak alone, and is the reason the adjLastDate carrier
			// exists rather than just the gate removal.
			name:  "hard payment, later adjustment on a month end",
			loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{67, 0.04}, {77, 0.11}},
			hardPay:  1078.32,
			wantInt:  60632.98,
			wantPaid: 160632.98,
			note:     "port produced 60283.51 before the snap was threaded to the next adjustment",
		},
		{
			// CONTROL — adj 2 at 30 Oct 2029; October has 31 days so the day-30
			// grid date is NOT a month end, LastDayFn is false, nothing snaps.
			// Unchanged by the fix.
			name:  "later adjustment NOT on a month end",
			loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{67, 0.04}, {76, 0.11}},
			wantInt:  60676.32,
			wantPaid: 160676.32,
		},
		{
			// CONTROL — one adjustment cannot trigger it: there is no LATER
			// adjustment to snap the date before the walk reaches this one.
			name: "single early adjustment", loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{67, 0.04}},
			wantInt:  47012.98,
			wantPaid: 147012.98,
		},
		{
			name: "single late adjustment", loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{77, 0.11}},
			wantInt:  62675.88,
			wantPaid: 162675.88,
		},
		{
			// CONTROL — day-of-month 1 can never be a month end. Proves the fix
			// is conditional and not a blanket shift.
			name: "day 1 — month end impossible", loanD: jun1, firstD: aug1, n: 144,
			adjs:     []adj{{67, 0.04}, {77, 0.11}},
			wantInt:  69190.47,
			wantPaid: 169190.47,
		},
		{
			// CONTROL — terminal month April (30 days) equals the grid day, so
			// daysinm cannot move it even though adj 2 is a month end. Pins the
			// SECOND necessary condition.
			name: "terminal month no longer than the grid day", loanD: jun30, firstD: aug30, n: 141,
			adjs:     []adj{{67, 0.04}, {77, 0.11}},
			wantInt:  58625.18,
			wantPaid: 158625.18,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, tc.loanD, tc.firstD, tc.n, tc.adjs, tc.hardPay)
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
}
