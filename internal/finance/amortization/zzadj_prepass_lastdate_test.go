package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Regression guard for the ADJUSTMENT PRE-PASS lastdate leak
// (docs/discrepancies.md §50, docs/termsolve_residual_corpus_2026-07-31.md).
//
// ROOT CAUSE. DOS runs a pre-pass before the display walk, Amortize.pas:1408-1419:
//
//	for i := 1 to nadj do
//	  if (adj[i]^.loanratestatus >= defp) and (adj[i]^.amountstatus < defp) then
//	    begin
//	      if (not EstimateAndRefineAdjPayment(i)) then exit;
//	      d := h^.payamt;
//	    end
//
// Every payment it solves is discarded — EstimateAndRefineAdjPayment
// (Amortize.pas:324-345) restores the rate and the balloon state, and
// Re_Amortize sets `amountstatus := outp` WITHOUT setting `amtok`
// (AMORTOP.pas:1591-1592), so the display walk recomputes each amount anyway.
// The pre-pass is not a no-op only because of what it does NOT restore:
// `h^.lastdate`. Re_Amortize's payment arm calls
//
//	n := NumberOfInstallments(adj[next_adj]^.date, h^.lastdate, h^.peryr, on_or_after);
//
// (AMORTOP.pas:1547) with NO save/restore guard — contrast Amortize.pas:1301-1304,
// which brackets its own call. `l` is a VAR parameter that NumberOfInstallments
// snaps onto the payment grid in place (INTSUTIL.pas:936-941); for a month-end
// adjustment date `if (flast) then l.d := daysinm(l)` (INTSUTIL.pas:1018) pushes
// the day out to the terminal month's length, after which `ddiff > 0` advances
// the month too (INTSUTIL.pas:1003). That mutation survives the pre-pass.
//
// So DOS's display walk re-solves the FIRST adjustment against an already
// snapped lastdate and counts one MORE remaining period than a cold walk. The
// structural port had no pre-pass at all, so it counted one fewer.
//
// TWO CONDITIONS are both necessary, and the controls below pin both:
//   - a LATER adjustment must fall on a month end (so LastDayFn is true), and
//   - the terminal month must be longer than the grid day (so daysinm moves it).
//
// This is why only day-of-month 30 shows it: only 30 can be a month end
// (Apr/Jun/Sep/Nov) while still sitting below a 31-day terminal month.
//
// PROVENANCE. Every golden below is from the real DOS engine via
// legacy/oracle/amort_oracle (built by legacy/oracle/build_linux.sh):
//
//	amort_oracle 100000 0.08 144 12 loandmy=30.6.2023 firstdmy=30.8.2023 \
//	             adj=67:0.04: adj=77:0.11:
//	  -> payment 1089.6211 interest 60857.90 paid 160857.90
func TestDOSAdjustmentPrePassLastDateSnap(t *testing.T) {
	// adj=N in the oracle's token language is N months after the LOAN date
	// (amort_oracle.pas:312-320): date.d = loandate.d, month/year stepped by N.
	adjDate := func(loan types.DateRec, months int) types.DateRec {
		tot := (int(loan.Time.Month()) - 1) + months
		return types.NewDateRec(loan.Time.Year()+tot/12, time.Month(tot%12+1), loan.Time.Day())
	}

	run := func(t *testing.T, loanD, firstD types.DateRec, n int, adjs []struct {
		months int
		rate   float64
	}) AmortResult {
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
		}
		for _, a := range adjs {
			in.Adjustments = append(in.Adjustments, RateAdjustment{
				DateStatus: types.InOutInput, Date: adjDate(loanD, a.months),
				LoanRateStatus: types.InOutInput, LoanRate: a.rate,
			})
		}
		// Leave PayAmt EMPTY and let Amortize solve it, exactly as the oracle
		// does with no pay=/payhard= token. Pre-solving and feeding the answer
		// back in as InOutInput would make it a HARD payment, which takes a
		// different dispatch and rounds differently — that shifts every total by
		// a few cents and would make these goldens unreachable.
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("Amortize: %v", res.Err)
		}
		return res
	}

	type adj = struct {
		months int
		rate   float64
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
		wantInt  float64
		wantPaid float64
		note     string
	}{
		{
			// THE CASE. adj 2 lands 30 Nov 2029 — a month end — and the terminal
			// month is July (31 days) against a day-30 grid, so daysinm moves it.
			// Before the pre-pass was ported the port produced 60504.78.
			name:  "two adjustments, later one on a month end",
			loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{67, 0.04}, {77, 0.11}},
			wantInt:  60857.90,
			wantPaid: 160857.90,
			note:     "port produced 60504.78 before the pre-pass was ported",
		},
		{
			// CONTROL — adj 2 at 30 Oct 2029; October has 31 days so the day-30
			// grid date is NOT a month end, LastDayFn is false, and nothing snaps.
			name:  "two adjustments, later one NOT on a month end",
			loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{67, 0.04}, {76, 0.11}},
			wantInt:  60901.67,
			wantPaid: 160901.67,
		},
		{
			// CONTROL — adj 2 at 30 Apr 2029, a month end, so this one DOES snap.
			name:  "two adjustments, later one on a 30-day month end (April)",
			loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{67, 0.04}, {70, 0.11}},
			wantInt:  63766.02,
			wantPaid: 163766.02,
		},
		{
			// CONTROL — one adjustment cannot trigger it: there is no LATER
			// adjustment to do the snapping before the display walk reaches this
			// one. Must be unchanged by the fix.
			name: "single adjustment", loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{67, 0.04}},
			wantInt:  47095.14,
			wantPaid: 147095.14,
		},
		{
			name: "single later adjustment", loanD: jun30, firstD: aug30, n: 144,
			adjs:     []adj{{77, 0.11}},
			wantInt:  62922.17,
			wantPaid: 162922.17,
		},
		{
			// CONTROL — day-of-month 1 can never be a month end, so the same two
			// adjustments produce the COLD-walk answer, which is exactly the
			// number the buggy port produced for the day-30 case. Keeping this
			// case is what proves the fix is conditional and not a blanket shift.
			name: "day 1 — month end impossible", loanD: jun1, firstD: aug1, n: 144,
			adjs:     []adj{{67, 0.04}, {77, 0.11}},
			wantInt:  60504.78,
			wantPaid: 160504.78,
		},
		{
			// CONTROL — terminal month April (30 days) equals the grid day, so
			// `daysinm` cannot move it even though adj 2 is a month end. Pins the
			// SECOND necessary condition.
			name: "terminal month no longer than the grid day", loanD: jun30, firstD: aug30, n: 141,
			adjs:     []adj{{67, 0.04}, {77, 0.11}},
			wantInt:  58819.96,
			wantPaid: 158819.96,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, tc.loanD, tc.firstD, tc.n, tc.adjs)
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
