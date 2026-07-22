package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Regression guards for the 2026-07-22 manual-testing finding (client-run
// side-by-side sweep, penny-scale loan): two DOS display behaviors were missing
// from the port's schedule renderers.
//
//  1. THE SUB-$1 RESIDUAL FOLD. DOS's display walks fold ANY remaining balance
//     below minpmt (= $1.00, AMORTOP.pas:14) into the CURRENT row's payment and
//     retire the loan there — fancy walk: AMORTOP.pas:1208-1211
//     (`if ((not lastok) or entire) and (WhenToStop^.principal < minpmt) then
//     payamt += principal; principal := 0`), simple loop: Amortize.pas:1546-1550.
//     The port's dosport_walk.go had this fold, but the display generators
//     (generateExactInAdvanceScheduleMode, generateFancyScheduleMode,
//     generateSimpleSchedule) folded only when the balance hit <= 0 — a positive
//     sub-$1 residual before the last scheduled row was silently DROPPED: the
//     schedule stopped (or amortized on) with the loan unretired and totals
//     short. Only reachable when the balance lands in (0,1) before the final
//     row — i.e. payment > balance/n at dollar scale ~ $1 — which is why every
//     realistic-scale oracle sweep missed it.
//
//  2. SETTLEMENT-ROW ROUNDING COMPOSITION. DOS builds the loan-date settlement
//     line as ONE sum rounded ONCE: `interest := PrepaidInterest +
//     points*amount; if hard_payment then Round2(interest)` (Amortize.pas:
//     1482-1483). The port rounded the prepaid/in-advance stub and the points
//     charge SEPARATELY (each in its own Round2), losing a cent whenever the
//     two sub-half-cent fractions sum across the boundary.
//
// Oracle provenance (Linux DOS oracle, legacy/oracle/build_linux.sh):
//
//	amort_oracle 1.15 0.03 5 12 b365 exact inadv prepaid payhard=0.29 loandmy=15.10.2026 firstdmy=1.12.2026 rows dumpraw
//	  → L0|0 10/15/26 0.00 0.00 1.15 0.00
//	    L1|1  1/ 1/27 0.00 1.15 0.00 0.00   (final payment 1.15)
//	    Total payments: 1.15  Principal: 1.15  Interest: 0.00
//	amort_oracle 1.15 0.03 5 12 b365 exact prepaid payhard=0.29 loandmy=15.10.2026 firstdmy=1.12.2026 rows dumpraw
//	  → L1|1 12/ 1/26 0.00 1.15 0.00 0.00   (final payment 1.15; same totals)
//	amort_oracle 1.15 0.03 5 12 b365 prepaid payhard=0.29 loandmy=15.10.2026 firstdmy=1.12.2026 rows dumpraw
//	  → L1|1 12/ 1/26 0.00 1.15 0.00 0.00   (final payment 1.15; same totals)
//
// (Pre-fix the port left balance 0.86 outstanding on the exact paths — the
// in-advance schedule stopped after one $0.29 row with Total Paid $0.39 — and
// amortized the plain-365 loan over 4 rows instead of folding at row 1.)
func TestDOSPennySubDollarFold(t *testing.T) {
	mk := func(exact, inAdv bool) LoanInput {
		return LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: 1.15,
				LoanRateStatus: types.InOutInput, LoanRate: 0.03,
				PayAmtStatus: types.InOutInput, PayAmt: 0.29,
				NStatus: types.InOutInput, NPeriods: 5,
				PerYrStatus: types.InOutInput, PerYr: 12,
				LoanDateStatus: types.InOutInput,
				LoanDate:       types.NewDateRec(2026, time.October, 15),
				FirstStatus:    types.InOutInput,
				FirstDate:      types.NewDateRec(2026, time.December, 1),
			},
			Settings: Settings{
				Basis:     types.Basis365,
				Exact:     exact,
				InAdvance: inAdv,
				Prepaid:   true,
			},
		}
	}

	cases := []struct {
		name         string
		exact, inAdv bool
		foldDate     types.DateRec // date of the row DOS folds on
	}{
		// User-reported case: exact × in-advance (base-date shift → fold at 1/1/27).
		{"exact_inadvance", true, true, types.NewDateRec(2027, time.January, 1)},
		// Exact arrears (fancy walk): fold at the first payment date.
		{"exact_arrears", true, false, types.NewDateRec(2026, time.December, 1)},
		// Plain 365 (simple schedule): fold at the first payment date.
		{"plain_365", false, false, types.NewDateRec(2026, time.December, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Amortize(mk(tc.exact, tc.inAdv))
			if res.Err != nil {
				t.Fatalf("Amortize errored: %v", res.Err)
			}
			if len(res.Schedule) == 0 {
				t.Fatal("empty schedule")
			}
			last := res.Schedule[len(res.Schedule)-1]
			// DOS folds the whole $1.15 into the row at foldDate: payment 1.15,
			// balance 0.00, total paid 1.15, total interest 0.00 (all interest
			// rounds to zero at this scale under the hard payment).
			if last.Principal > 0.005 || last.Principal < -0.005 {
				t.Errorf("schedule ends unretired: final balance %.4f, want 0.00 (DOS folds sub-$1 residual, AMORTOP.pas:1208-1211)", last.Principal)
			}
			if math.Abs(last.PayAmt-1.15) > 0.005 {
				t.Errorf("final payment %.4f, want 1.15 (DOS final-payment fold)", last.PayAmt)
			}
			if dcomp := last.Date; !sameDate(dcomp, tc.foldDate) {
				t.Errorf("fold row date %v, want %v (DOS folds at the FIRST sub-$1 row, not the scheduled term end)", dcomp, tc.foldDate)
			}
			if math.Abs(res.TotalPaid-1.15) > 0.005 {
				t.Errorf("TotalPaid %.4f, want 1.15 (DOS: Total payments: 1.15)", res.TotalPaid)
			}
			if math.Abs(res.TotalInt-0.00) > 0.005 {
				t.Errorf("TotalInt %.4f, want 0.00", res.TotalInt)
			}
		})
	}
}

func sameDate(a, b types.DateRec) bool {
	ay, am, ad := a.Time.Date()
	by, bm, bd := b.Time.Date()
	return ay == by && am == bm && ad == bd
}

// TestDOSSettlementRoundingComposition guards fix #2 above: the settlement line
// must be rounded as ONE combined sum (Amortize.pas:1482-1483), not
// piece-by-piece. Inputs are crafted so the stub interest (raw 3.8604) and the
// points charge (raw 9.0047) each round DOWN alone (3.86 + 9.00 = 12.86) but
// their sum rounds UP (12.8651 → 12.87), matching DOS.
//
// Oracle provenance:
//
//	amort_oracle 1000 0.03 12 12 b365 exact inadv prepaid pts=0.0090047 payhard=85 loandmy=15.10.2026 firstdmy=1.12.2026 rows dumpraw
//	  → L0|0 10/15/26 12.87 0.00 1000.00 12.87
//	    L1|1  1/ 1/27 2.55 82.45 917.55 15.42
//
// (Pre-fix the port emitted L0 = 12.86 and every IntToDate a cent short.)
func TestDOSSettlementRoundingComposition(t *testing.T) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 1000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.03,
			PayAmtStatus: types.InOutInput, PayAmt: 85,
			NStatus: types.InOutInput, NPeriods: 12,
			PerYrStatus: types.InOutInput, PerYr: 12,
			PointsStatus: types.InOutInput, Points: 0.0090047,
			LoanDateStatus: types.InOutInput,
			LoanDate:       types.NewDateRec(2026, time.October, 15),
			FirstStatus:    types.InOutInput,
			FirstDate:      types.NewDateRec(2026, time.December, 1),
		},
		Settings: Settings{
			Basis:     types.Basis365,
			Exact:     true,
			InAdvance: true,
			Prepaid:   true,
		},
	}
	res := Amortize(in)
	if res.Err != nil {
		t.Fatalf("Amortize errored: %v", res.Err)
	}
	if len(res.Schedule) < 2 {
		t.Fatalf("schedule too short: %d rows", len(res.Schedule))
	}
	l0 := res.Schedule[0]
	if math.Abs(l0.Interest-12.87) > 0.0001 {
		t.Errorf("settlement row interest %.4f, want 12.87 (DOS rounds PrepaidInterest+points ONCE, Amortize.pas:1482-1483; separate rounding gives 12.86)", l0.Interest)
	}
	if math.Abs(l0.PayAmt-12.87) > 0.0001 {
		t.Errorf("settlement row payment %.4f, want 12.87", l0.PayAmt)
	}
	if math.Abs(l0.IntToDate-12.87) > 0.0001 {
		t.Errorf("settlement row IntToDate %.4f, want 12.87", l0.IntToDate)
	}
	l1 := res.Schedule[1]
	if math.Abs(l1.IntToDate-15.42) > 0.0001 {
		t.Errorf("row 1 IntToDate %.4f, want 15.42 (12.87 + 2.55 — accumulates the ROUNDED settlement line)", l1.IntToDate)
	}
}
