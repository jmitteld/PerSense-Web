package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Regression guard for the ARM SEGMENT SOLVE's period count and for the walk's
// regular-vs-extra test, both of which must read DOS's MUTATED `h^.lastdate`
// global rather than the pristine last-payment date (docs/discrepancies.md §53).
//
// This is the third and last reader of the §50 VAR snap. Round 8 ported the
// mutation for the structural engine, round 9 gave the piecewise engine a
// dedicated `adjLastDate` carrier and wired it to ONE reader —
// `NumberOfInstallmentsRaw`, the counterpart of AMORTOP.pas:1547. The other two
// readers were left on `loan.LastDate`:
//
//   - ComputeNext's regular-vs-extra test, AMORTOP.pas:606:
//
//     if (DateComp(date, h^.lastdate) > 0) then
//     balloonpos := -1;
//
//     which decides whether a grid date still gets a regular payment or is
//     forced off-cycle onto the next extra; and
//
//   - the segment Iterate's period count. DOS has no count here at all — its
//     Re_Amortize calls Iterate(…, til_adj) (AMORTOP.pas:1577), which walks
//     RepayFancyLoan, whose ComputeNext applies the same date test row by row.
//     The port passed `remaining` = `loan.NPeriods - payNumNow`.
//
// THE SNAP CAN FIRE TWICE AND COMPOUND. Worked through for case A below
// (adjustments 30.9.2032 and 30.7.2033, last payment 30.7.2034):
//
//	adj 1: f = 30.9.2032. September has 30 days so flast = TRUE; l = 30.7.2034,
//	       July has 31 so llast = FALSE; ddiff = 0 so no month step. Then
//	       `if (flast) then l.d := daysinm(l)` (INTSUTIL.pas:1018) -> 31.7.2034.
//	adj 2: f = 30.7.2033, flast = FALSE; l = 31.7.2034, llast = TRUE now;
//	       ddiff = +1 > 0 and not(flast and llast), so `l.m := l.m + monthsbtwn`
//	       fires (INTSUTIL.pas:1003) -> month 8; then the else-arm restores
//	       `l.d := f.d` = 30. Result: 30.8.2034.
//
// A whole month past the real last payment. DOS therefore emits a THIRTEENTH
// regular payment at 30.8.2034 and solves the second adjustment over 13 periods,
// returning 4720.33; the port emitted 12 rows and solved 5170.29.
//
// AND IT MUST BE CAPPED BY very_last. DOS's segment walk is bounded twice: by
// the snapped date (ComputeNext) AND by `stopdate`, which for the segment
// Iterate's adjnum=0 call is `very_last` (AMORTOP.pas:1140-1142). `very_last`
// is computed by DetermineVeryLast at Amortize.pas:1320, BEFORE the adjustment
// pre-pass at :1408, so it never sees the snap. When the snap pushes past
// very_last the extra period is unreachable — the walk ends first. Case G below
// is exactly that, and it caught a real regression from the uncapped first
// attempt at this fix.
//
// PROVENANCE. Every golden is from the real DOS engine via
// legacy/oracle/amort_oracle (built by legacy/oracle/build_linux.sh).
func TestDOSAdjSegmentHorizonSnap(t *testing.T) {
	// adj=N / pre=N / b<N>= are N months after the LOAN date, day carried over
	// (amort_oracle.pas:172-176, :254-258, :310-314), clamped down by
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

	type adj struct {
		months int
		rate   float64
	}
	type prepay struct {
		startMonths, nn, perYr int
		amount                 float64
	}
	type balloon struct {
		months int
		amount float64
	}

	type screen struct {
		amount   float64
		rate     float64
		n        int
		loanDay  int
		loanMon  time.Month
		loanYear int
		firstMon time.Month
		adjs     []adj
		pres     []prepay
		bals     []balloon
	}

	run := func(t *testing.T, s screen) AmortResult {
		t.Helper()
		loanD := types.NewDateRec(s.loanYear, s.loanMon, s.loanDay)
		firstD := types.NewDateRec(s.loanYear, s.firstMon, s.loanDay)
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: s.amount,
				LoanRateStatus: types.InOutInput, LoanRate: s.rate,
				NStatus: types.InOutInput, NPeriods: s.n,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: loanD,
				FirstStatus: types.InOutInput, FirstDate: firstD,
			},
			Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360},
			Fancy:    true,
		}
		for _, a := range s.adjs {
			in.Adjustments = append(in.Adjustments, RateAdjustment{
				DateStatus: types.InOutInput, Date: monthsAfter(loanD, a.months),
				LoanRateStatus: types.InOutInput, LoanRate: a.rate,
			})
		}
		for _, p := range s.pres {
			in.Prepayments = append(in.Prepayments, Prepayment{
				StartDateStatus: types.InOutInput, StartDate: monthsAfter(loanD, p.startMonths),
				NNStatus: types.InOutInput, NN: p.nn,
				PerYrStatus: types.InOutInput, PerYr: p.perYr,
				PaymentStatus: types.InOutInput, Payment: p.amount,
			})
		}
		for _, b := range s.bals {
			in.Balloons = append(in.Balloons, BalloonPayment{
				DateStatus: types.InOutInput, Date: monthsAfter(loanD, b.months),
				AmountStatus: types.InOutInput, Amount: b.amount,
			})
		}
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("Amortize: %v", res.Err)
		}
		return res
	}

	// The §53 family: a day-30 May loan, 132 months, semiannual prepayment
	// series overshooting the term (so very_last runs out to 2041).
	base := screen{
		amount: 364306.79, rate: 0.104973, n: 132,
		loanDay: 30, loanMon: time.May, loanYear: 2023, firstMon: time.August,
		pres: []prepay{{32, 31, 2, 185.20}},
	}
	withAdjs := func(as ...adj) screen { s := base; s.adjs = as; return s }

	// The cap family: a day-28 February loan with no trailing option, so
	// very_last == the last payment date and the compounded snap overshoots it.
	capBase := screen{
		amount: 90498.48, rate: 0.108453, n: 84,
		loanDay: 28, loanMon: time.February, loanYear: 2023, firstMon: time.May,
		bals: []balloon{{17, 3533.04}},
	}
	withCapAdjs := func(as ...adj) screen { s := capBase; s.adjs = as; return s }

	cases := []struct {
		name     string
		s        screen
		wantInt  float64
		wantPaid float64
		note     string
	}{
		{
			// THE CASE. adj 1 on a September month end snaps 30.7.2034 ->
			// 31.7.2034; adj 2 then snaps that to 30.8.2034. DOS emits 146 rows,
			// the port emitted 145.
			name: "two adjustments, compounded snap, prepay overshoot",
			s:    withAdjs(adj{112, 0.0523}, adj{122, 0.0818}),
			// amort_oracle 364306.79 0.104973 132 12 loandmy=30.5.2023
			//   firstdmy=30.8.2023 adj=112:0.0523: adj=122:0.0818: pre=32:31:2:185.20
			wantInt:  247296.53,
			wantPaid: 611603.32,
			note:     "port produced 247075.78; the segment solved 5170.29 where DOS's Iterate returns 4720.33",
		},
		{
			// The original fuzzer-found screen, three adjustments.
			name: "three adjustments",
			s:    withAdjs(adj{12, 0.1291}, adj{112, 0.0523}, adj{122, 0.0818}),
			// amort_oracle ... adj=122:0.0818: adj=12:0.1291: adj=112:0.0523: pre=32:31:2:185.20
			wantInt:  300901.45,
			wantPaid: 665208.24,
			note:     "port produced 300663.96 after round 10, 300095.42 before it",
		},
		{
			// CONTROL — day 15 can never be a month end, so no snap fires and the
			// segment count is unchanged. The value here, 247075.78, is EXACTLY
			// what the port produced for the day-30 case before this fix — which
			// is the cleanest possible demonstration that the port was walking the
			// un-snapped (cold) schedule.
			name: "day 15 — month end impossible",
			s: func() screen {
				s := withAdjs(adj{112, 0.0523}, adj{122, 0.0818})
				s.loanDay = 15
				return s
			}(),
			// amort_oracle 364306.79 0.104973 132 12 loandmy=15.5.2023 firstdmy=15.8.2023 ...
			wantInt:  247075.78,
			wantPaid: 611382.57,
		},
		{
			// CONTROL — one adjustment cannot compound the snap.
			name:     "only the September-month-end adjustment",
			s:        withAdjs(adj{112, 0.0523}),
			wantInt:  246095.59,
			wantPaid: 610402.38,
		},
		{
			name:     "only the later adjustment",
			s:        withAdjs(adj{122, 0.0818}),
			wantInt:  250283.49,
			wantPaid: 614590.28,
		},
		{
			// CONTROL — no prepayment, so very_last == the last payment date and
			// the snapped period is unreachable. Also the case that the segment
			// solve is not engaged for at all (DOS's AMORTOP.pas:1571 gate).
			name: "no prepayment — very_last does not extend",
			s: func() screen {
				s := withAdjs(adj{112, 0.0523}, adj{122, 0.0818})
				s.pres = nil
				return s
			}(),
			wantInt:  258334.25,
			wantPaid: 622641.04,
		},
		{
			// THE CAP CASE. A February-28 month-end adjustment snaps 28.4.2030 ->
			// 30.4.2030, and the next adjustment snaps that to 28.5.2030 — one
			// month PAST very_last, which is 28.4.2030 because this screen has no
			// trailing option. DOS solves 49 periods; the snapped count says 50.
			// The first version of this fix passed the uncapped count and broke
			// this case; the randomized goamort sweep caught it.
			name: "snap overshoots very_last — segment count must be capped",
			s:    withCapAdjs(adj{18, 0.1434}, adj{24, 0.1013}, adj{37, 0.0380}),
			// amort_oracle 90498.48 0.108453 84 12 loandmy=28.2.2023 firstdmy=28.5.2023
			//   adj=18:0.1434: adj=24:0.1013: adj=37:0.0380: b17=3533.04
			wantInt:  31961.76,
			wantPaid: 122460.24,
			note:     "uncapped seedN gave 32059.15 — this is the guard against re-introducing that",
		},
		{
			// CONTROL for the cap family — two adjustments, no third crossing.
			name:     "cap family, two adjustments",
			s:        withCapAdjs(adj{18, 0.1434}, adj{24, 0.1013}),
			wantInt:  40742.74,
			wantPaid: 131241.22,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, tc.s)
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
