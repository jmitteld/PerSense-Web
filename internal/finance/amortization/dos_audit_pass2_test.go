// Regressions for the 2026-07-11 audit PASS-2 findings (docs/discrepancies.md
// §21, F1-F10a). Every golden cites the real-DOS-engine command that produced
// it; all were re-derived from the live oracle when this file was written.
package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

func pass2B365() Settings {
	return Settings{Basis: types.Basis365, PerYr: 12, YrDays: 365.25, YrInv: 1.0 / 365.25}
}

// F1: a hard payment below the accruing interest (or otherwise leaving ≥ one
// payment of residual) FOLDS the whole residual into the final scheduled
// payment on the plain USA / exact non-360 paths — DOS's very-last fold
// (Amortize.pas plainFancy) has no "residual < payment" restriction.
//
//	amort_oracle 100000 0.15 36 12 payhard=900 b365 usa   → interest 45000.00 paid 145000.00
//	amort_oracle 100000 0.15 36 12 payhard=900 b365 exact → interest 48179.40 paid 148179.40
func TestPass2F1NegAmResidualFold(t *testing.T) {
	s := pass2B365()
	s.USARule = true
	res := Amortize(LoanInput{Loan: pass1Loan(100000, 0.15, 900, 36), Settings: s})
	if res.Err != nil || math.Abs(res.TotalPaid-145000.00) > 0.05 || math.Abs(res.TotalInt-45000.00) > 0.05 {
		t.Errorf("usa: paid=%.2f int=%.2f err=%v, want 145000.00 / 45000.00 (oracle)", res.TotalPaid, res.TotalInt, res.Err)
	}
	if math.Abs(res.FinalPrinc) > 0.01 {
		t.Errorf("usa: final balance = %.4f, want 0 (DOS fold)", res.FinalPrinc)
	}
	s2 := pass2B365()
	s2.Exact = true
	res2 := Amortize(LoanInput{Loan: pass1Loan(100000, 0.15, 900, 36), Settings: s2})
	if res2.Err != nil || math.Abs(res2.TotalPaid-148179.40) > 0.05 {
		t.Errorf("exact: paid=%.2f err=%v, want 148179.40 (oracle)", res2.TotalPaid, res2.Err)
	}
}

// F2: semimonthly (24/yr) rows on a non-360 basis accrue the DaysCloseEnough-
// gated timedif (AMORTOP.pas:625-632 via the non-360 RepayFancyLoan route),
// not the constant p*(f-1): actual days on the 1st/16th grid, whole ±half-month
// periods on the 15th/month-end grid.
//
//	amort_oracle 10000 0.06 24 24 firstdmy=16.1.2024 b365 pay=429.7945 rows
//	  → row1 int 24.59, row2 int 25.17 (16 actual days), interest 314.56
//	amort_oracle 10000 0.06 24 24 loandmy=15.1.2024 firstdmy=30.1.2024 b365 pay=429.7945
//	  → row1 int 25.00 (whole half-month), interest 315.50
//	amort_oracle 10000 0.06 24 24 firstdmy=16.1.2024 b365_360 pay=429.7945
//	  → interest 320.01
func TestPass2F2SemimonthlyNon360Accrual(t *testing.T) {
	mk := func(loanD, firstD types.DateRec) Loan {
		return Loan{
			AmountStatus: types.InOutInput, Amount: 10000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.06,
			PayAmtStatus: types.InOutDefault, PayAmt: 429.7945,
			NStatus:      types.InOutInput, NPeriods: 24,
			PerYrStatus: types.InOutInput, PerYr: 24,
			LoanDateStatus: types.InOutInput, LoanDate: loanD,
			FirstStatus: types.InOutInput, FirstDate: firstD,
		}
	}
	s := pass2B365()
	s.PerYr = 24
	res := Amortize(LoanInput{Loan: mk(types.NewDateRec(2024, time.January, 1), types.NewDateRec(2024, time.January, 16)), Settings: s})
	if res.Err != nil || math.Abs(res.Schedule[1].Interest-25.17) > 0.005 || math.Abs(res.TotalInt-314.56) > 0.05 {
		t.Errorf("1/16 grid: row2 int=%.4f totInt=%.2f err=%v, want 25.17 / 314.56 (oracle)",
			res.Schedule[1].Interest, res.TotalInt, res.Err)
	}
	res2 := Amortize(LoanInput{Loan: mk(types.NewDateRec(2024, time.January, 15), types.NewDateRec(2024, time.January, 30)), Settings: s})
	if res2.Err != nil || math.Abs(res2.Schedule[0].Interest-25.00) > 0.005 || math.Abs(res2.TotalInt-315.50) > 0.05 {
		t.Errorf("15/30 grid: row1 int=%.4f totInt=%.2f err=%v, want 25.00 / 315.50 (oracle)",
			res2.Schedule[0].Interest, res2.TotalInt, res2.Err)
	}
	s3 := Settings{Basis: types.Basis365360, PerYr: 24, YrDays: 360, YrInv: 1.0 / 360}
	res3 := Amortize(LoanInput{Loan: mk(types.NewDateRec(2024, time.January, 1), types.NewDateRec(2024, time.January, 16)), Settings: s3})
	if res3.Err != nil || math.Abs(res3.TotalInt-320.01) > 0.05 {
		t.Errorf("365/360: totInt=%.2f err=%v, want 320.01 (oracle)", res3.TotalInt, res3.Err)
	}
}

// F3: plain amount/rate solves on a non-360 basis use DOS's solve-side
// actual-day prorate (`prorate := YearsDif(first_repay, repay_from) * peryr`,
// Amortize.pas:1284-1287), not the whole-period shortcut.
//
//	amort_oracle 120000 0.11 36 12 b365                    → payment 3929.2311
//	amort_oracle 0 0.11 36 12 b365 noamt pay=3929.2311     → solvedamount 120000.0012
//	amort_oracle 120000 0 36 12 b365 norate pay=3929.2311  → solvedrate 0.1100000067
func TestPass2F3PlainSolvesNon360Prorate(t *testing.T) {
	s := pass2B365()
	loan := pass1Loan(120000, 0.11, 0, 36)
	res := Amortize(LoanInput{Loan: loan, Settings: s})
	if res.Err != nil || math.Abs(res.Schedule[1].PayAmt-3929.2311) > 0.005 {
		t.Errorf("payment = %.4f err=%v, want 3929.2311 (oracle)", res.Schedule[1].PayAmt, res.Err)
	}
	l2 := loan
	l2.AmountStatus, l2.Amount = types.StatusEmpty, 0
	l2.PayAmtStatus, l2.PayAmt = types.InOutInput, 3929.2311
	amt, _, err := SolveLoanAmount(LoanInput{Loan: l2, Settings: s})
	if err != nil || math.Abs(amt-120000.0012) > 0.005 {
		t.Errorf("amount = %.4f err=%v, want 120000.0012 (oracle)", amt, err)
	}
	l3 := loan
	l3.LoanRateStatus, l3.LoanRate = types.StatusEmpty, 0
	l3.PayAmtStatus, l3.PayAmt = types.InOutInput, 3929.2311
	r, _, err3 := SolveRate(LoanInput{Loan: l3, Settings: s})
	if err3 != nil || math.Abs(r-0.1100000067) > 5e-6 {
		t.Errorf("rate = %.7f err=%v, want 0.1100000067 (oracle)", r, err3)
	}
}

// F4: exact × moratorium payment solve iterates the exact-accrual terminal
// (Amortize.pas:416 + AMORTOP.pas:625), not the exact-OFF annuity.
//
//	amort_oracle 100000 0.10 24 12 b365 exact mor=6 → payment 5712.4662
func TestPass2F4ExactMoratoriumSolve(t *testing.T) {
	s := pass2B365()
	s.Exact = true
	res := Amortize(LoanInput{Loan: pass1Loan(100000, 0.10, 0, 24), Settings: s, Fancy: true,
		Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2024, time.July, 1)}})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	var pay float64
	for _, r := range res.Schedule {
		if r.Date.Time.Format("2006-01-02") == "2024-09-01" {
			pay = r.PayAmt
		}
	}
	if math.Abs(pay-5712.4662) > 0.005 {
		t.Errorf("post-moratorium payment = %.4f, want 5712.4662 (oracle)", pay)
	}
}

// F5: exact × in-advance on the 360 basis is a DISPLAY split, not a solve
// split: DOS's Iterate gate is `fancy or (exact and basis<>x360)`
// (AMORTOP.pas:1438) so the payment stays the plain annuity-due, while the
// display gate `fancy or (exact and not R78)` (Amortize.pas:1493) still routes
// the SCHEDULE through the shifted exact in-advance shape.
//
//	amort_oracle 104844 0.0593783730 36 12 inadv exact
//	  → payment 3172.2326, first amortizing row 2024-03-01, interest 10421.61
//	amort_oracle 52287 0.0748745490 12 12 inadv exact prepaid → interest 2452.41
func TestPass2F5ExactInAdvance360DisplaySplit(t *testing.T) {
	s := basicSettings()
	s.Exact = true
	s.InAdvance = true
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 104844,
		LoanRateStatus: types.InOutInput, LoanRate: 0.0593783730,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 36,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}
	res := Amortize(LoanInput{Loan: loan, Settings: s})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	var pay float64
	var firstAmort string
	for _, r := range res.Schedule {
		if r.PayNum == 1 {
			pay, firstAmort = r.PayAmt, r.Date.Time.Format("2006-01-02")
			break
		}
	}
	if math.Abs(pay-3172.2326) > 0.005 {
		t.Errorf("payment = %.4f, want 3172.2326 (oracle; the plain annuity-due, NOT the exact-solved 3269.81)", pay)
	}
	if firstAmort != "2024-03-01" {
		t.Errorf("first amortizing row = %s, want 2024-03-01 (shifted exact in-advance shape)", firstAmort)
	}
	if math.Abs(res.TotalInt-10421.61) > 0.05 {
		t.Errorf("totInt = %.2f, want 10421.61 (oracle)", res.TotalInt)
	}
	s2 := basicSettings()
	s2.Exact, s2.InAdvance, s2.Prepaid = true, true, true
	loan2 := loan
	loan2.Amount, loan2.LoanRate, loan2.NPeriods = 52287, 0.0748745490, 12
	res2 := Amortize(LoanInput{Loan: loan2, Settings: s2})
	if res2.Err != nil || math.Abs(res2.TotalInt-2452.41) > 0.05 {
		t.Errorf("prepaid variant: totInt = %.2f err=%v, want 2452.41 (oracle)", res2.TotalInt, res2.Err)
	}
}

// F6: prepaid pins the solve-side prorate to EXACTLY 1 (Amortize.pas:1277-1282)
// even at day-count frequencies where a natural period ≠ 1 under YearsDif
// (7·52 = 364 ≠ 365.25).
//
//	amort_oracle 250000 0.145 26 26 b365 prepaid               → payment 10353.4897
//	amort_oracle 0 0.145 26 26 b365 prepaid noamt pay=10353.4897  → solvedamount 250000.0004
//	amort_oracle 250000 0 26 26 b365 prepaid norate pay=10353.4897 → solvedrate 0.1450000029
//	amort_oracle 10000 0.06 24 24 firstdmy=16.1.2024 b365 prepaid  → payment 429.8121
func TestPass2F6PrepaidProratePinDayCount(t *testing.T) {
	s := Settings{Basis: types.Basis365, PerYr: 26, YrDays: 365.25, YrInv: 1.0 / 365.25, Prepaid: true}
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 250000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.145,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 26,
		PerYrStatus: types.InOutInput, PerYr: 26,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 15),
	}
	res := Amortize(LoanInput{Loan: loan, Settings: s})
	if res.Err != nil || math.Abs(res.Schedule[1].PayAmt-10353.4897) > 0.005 {
		t.Errorf("biweekly payment = %.4f err=%v, want 10353.4897 (oracle)", res.Schedule[1].PayAmt, res.Err)
	}
	l2 := loan
	l2.AmountStatus, l2.Amount = types.StatusEmpty, 0
	l2.PayAmtStatus, l2.PayAmt = types.InOutInput, 10353.4897
	amt, _, _ := SolveLoanAmount(LoanInput{Loan: l2, Settings: s})
	if math.Abs(amt-250000.0004) > 0.01 {
		t.Errorf("amount = %.4f, want 250000.0004 (oracle)", amt)
	}
	l3 := loan
	l3.LoanRateStatus, l3.LoanRate = types.StatusEmpty, 0
	l3.PayAmtStatus, l3.PayAmt = types.InOutInput, 10353.4897
	r, _, _ := SolveRate(LoanInput{Loan: l3, Settings: s})
	if math.Abs(r-0.1450000029) > 5e-6 {
		t.Errorf("rate = %.7f, want 0.1450000029 (oracle)", r)
	}
	s4 := Settings{Basis: types.Basis365, PerYr: 24, YrDays: 365.25, YrInv: 1.0 / 365.25, Prepaid: true}
	loan4 := loan
	loan4.Amount, loan4.LoanRate, loan4.NPeriods, loan4.PerYr = 10000, 0.06, 24, 24
	loan4.FirstDate = types.NewDateRec(2024, time.January, 16)
	res4 := Amortize(LoanInput{Loan: loan4, Settings: s4})
	var pay4 float64
	for _, row := range res4.Schedule {
		if row.PayNum == 1 {
			pay4 = row.PayAmt
			break
		}
	}
	if res4.Err != nil || math.Abs(pay4-429.8121) > 0.005 {
		t.Errorf("semimonthly payment = %.4f err=%v, want 429.8121 (oracle)", pay4, res4.Err)
	}
}

// F7: Rule-of-78 with a HARD payment rounds the recurrence ACCUMULATOR
// (Amortize.pas:1524-1528 — Round2 is a var-procedure, so the next period
// subtracts from the rounded value), so the drift never accumulates.
//
//	amort_oracle 10000 0.1237 24 12 payhard=471.73 r78 rows
//	  → row2 int 101.31, row3 96.90, row4 92.49
func TestPass2F7R78HardAccumulatorRounding(t *testing.T) {
	s := basicSettings()
	s.R78 = true
	res := Amortize(LoanInput{Loan: pass1Loan(10000, 0.1237, 471.73, 24), Settings: s})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	want := []float64{101.31, 96.90, 92.49}
	for i, w := range want {
		if got := res.Schedule[i+1].Interest; math.Abs(got-w) > 0.005 {
			t.Errorf("row%d int = %.4f, want %.2f (oracle)", i+2, got, w)
		}
	}
}

// F8: in-advance × moratorium at a day-count frequency on a non-360 basis:
// the post-moratorium payment comes from Iterate over the actual-day walk,
// with the mid-loan segment solved WITHOUT re-applying the in-advance
// settlement/shift (ComputeNext accrues ordinary interest even when
// in_advance is set, AMORTOP.pas:636).
//
//	amort_oracle 100000 0.10 52 26 b365 inadv mor=3 → payment 2375.0973, interest 11549.56
func TestPass2F8InAdvanceMoratoriumDayCount(t *testing.T) {
	s := Settings{Basis: types.Basis365, PerYr: 26, YrDays: 365.25, YrInv: 1.0 / 365.25, InAdvance: true}
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.10,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 52,
		PerYrStatus: types.InOutInput, PerYr: 26,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 15),
	}
	mor := Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2024, time.April, 1)}
	res := Amortize(LoanInput{Loan: loan, Settings: s, Fancy: true, Moratorium: mor})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	var pay float64
	for _, r := range res.Schedule {
		if r.PayAmt > pay {
			pay = r.PayAmt
		}
	}
	if math.Abs(pay-2375.0973) > 0.005 {
		t.Errorf("post-moratorium payment = %.4f, want 2375.0973 (oracle)", pay)
	}
	// Forward check at DOS's payment: rows and totals match to the cent.
	l2 := loan
	l2.PayAmtStatus, l2.PayAmt = types.InOutDefault, 2375.0973
	res2 := Amortize(LoanInput{Loan: l2, Settings: s, Fancy: true, Moratorium: mor})
	if res2.Err != nil || math.Abs(res2.TotalInt-11549.56) > 0.05 {
		t.Errorf("forward totInt = %.2f err=%v, want 11549.56 (oracle)", res2.TotalInt, res2.Err)
	}
}

// F9: a balloon row whose AMOUNT is explicitly 0 still REPLACES the regular
// payment on its date (presence is by row STATUS, not by amount ≠ 0) — the
// zero-payment period shifts interest, so the solved payment rises.
//
//	amort_oracle 10000 0.12 24 12 b12=0 → payment 491.2571, interest 1298.91
//	(b12 anchors month 12 from 2024-01-01 = 2025-01-01)
func TestPass2F9ZeroAmountBalloonReplaces(t *testing.T) {
	res := Amortize(LoanInput{Loan: pass1Loan(10000, 0.12, 0, 24), Settings: basicSettings(), Fancy: true,
		Balloons: []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2025, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 0}}})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	if math.Abs(res.Schedule[1].PayAmt-491.2571) > 0.005 || math.Abs(res.TotalInt-1298.91) > 0.05 {
		t.Errorf("pay=%.4f int=%.2f, want 491.2571 / 1298.91 (oracle; NOT the plain 470.7347)",
			res.Schedule[1].PayAmt, res.TotalInt)
	}
}

// F10a: skip=1-12 (every month skipped) makes a blank-payment loan unsolvable —
// DOS never converges; the port must refuse with an error rather than emit a
// degenerate schedule.
func TestPass2F10aAllMonthsSkippedRefused(t *testing.T) {
	ms, err := MonthSetFromString("1-12")
	if err != nil {
		t.Fatalf("MonthSetFromString: %v", err)
	}
	res := Amortize(LoanInput{Loan: pass1Loan(10000, 0.12, 0, 24), Settings: basicSettings(), Fancy: true,
		SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "1-12", MonthSet: ms}})
	if res.Err == nil {
		t.Errorf("expected an error for skip=1-12 with a blank payment; got a schedule with %d rows", len(res.Schedule))
	}
}
