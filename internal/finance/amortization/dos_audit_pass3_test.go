// Regressions for the 2026-07-12 audit PASS-3 findings (docs/discrepancies.md
// §22, AF1-AF6 forward + P3-F1..F7 backward). Every golden cites the real
// DOS-engine command that produced it.
package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// AF1: a payment dated ON an adjustment date is made BEFORE the adjustment
// applies, so the AO6 implied-rate recompute sees that payment's balance and
// one fewer remaining period.
//
//	amort_oracle 100000 0.09 120 12 payhard=1300 adj=24::1100 payoff=1.2.2024 → payoff 100449.9644
func TestPass3AF1PayoffAdjOnPaymentDate(t *testing.T) {
	in := LoanInput{Loan: pass1Loan(100000, 0.09, 1300, 120), Settings: basicSettings(), Fancy: true,
		Adjustments: []RateAdjustment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2026, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 1100, AmtOK: true}}}
	got, err := PayoffBalance(in, types.NewDateRec(2024, time.February, 1))
	if err != nil || math.Abs(got-100449.9644) > 0.05 {
		t.Errorf("payoff = %.4f err=%v, want 100449.9644 (oracle; strict-< gave 100451.4622)", got, err)
	}
	// Two AO6 rows, payoff between them (negative-implied-rate territory):
	//
	//	amort_oracle 100000 0.09 120 12 payhard=1300 adj=24::1100 adj=48::900 payoff=15.3.2028 → payoff 65537.8966
	in2 := in
	in2.Adjustments = append(append([]RateAdjustment{}, in.Adjustments...),
		RateAdjustment{DateStatus: types.InOutInput, Date: types.NewDateRec(2028, time.January, 1),
			AmountStatus: types.InOutInput, Amount: 900, AmtOK: true})
	got2, err2 := PayoffBalance(in2, types.NewDateRec(2028, time.March, 15))
	if err2 != nil || math.Abs(got2-65537.8966) > 0.05 {
		t.Errorf("two-AO6 payoff = %.4f err=%v, want 65537.8966 (oracle)", got2, err2)
	}
}

// AF2: on the 360 basis the first-period prorate is the RAW 30/360 YearsDif
// (Amortize.pas:1286+1516) — a whole period on ordinary clean pairs but NOT on
// clamped or February month-end pairs. Also covers the prepaid-clearing rule:
// a loan taken strictly after the natural period start clears prepaid outright
// (Amortize.pas:1252-1259).
//
//	amort_oracle 50000 0.10 14 12 loandmy=31.1.2024 firstdmy=29.2.2024 rows
//	  → payment 3797.6090, row1 int 402.78 (29/360), interest 3166.53
//	amort_oracle 50000 0.10 14 12 loandmy=28.2.2025 firstdmy=28.3.2025 rows
//	  → payment 3796.5626, row1 int 388.89 (28/360), interest 3151.88
//	amort_oracle 50000 0.10 14 12 loandmy=31.1.2024 firstdmy=29.2.2024 prepaid
//	  → payment 3797.6090, interest 3166.53 (identical to non-prepaid: cleared)
//	amort_oracle 50000 0.10 14 12 loandmy=15.1.2024 firstdmy=15.2.2024
//	  → payment 3798.6555, row1 416.67 (ordinary pair control, whole month)
func TestPass3AF2Clamped360FirstPeriod(t *testing.T) {
	mk := func(ld, fd types.DateRec, prepaid bool) LoanInput {
		s := basicSettings()
		s.Prepaid = prepaid
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 50000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.10,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 14,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: ld,
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: s}
	}
	check := func(tag string, in LoanInput, wantPay, wantRow1, wantInt float64) {
		t.Helper()
		res := Amortize(in)
		if res.Err != nil {
			t.Fatalf("%s: %v", tag, res.Err)
		}
		var pay, row1 float64
		for _, r := range res.Schedule {
			if r.PayNum == 1 {
				pay, row1 = r.PayAmt, r.Interest
				break
			}
		}
		if math.Abs(pay-wantPay) > 0.005 || (wantRow1 > 0 && math.Abs(row1-wantRow1) > 0.005) ||
			math.Abs(res.TotalInt-wantInt) > 0.05 {
			t.Errorf("%s: pay=%.4f row1=%.4f int=%.2f, want %.4f / %.2f / %.2f (oracle)",
				tag, pay, row1, res.TotalInt, wantPay, wantRow1, wantInt)
		}
	}
	check("clamped Jan31→Feb29", mk(types.NewDateRec(2024, time.January, 31), types.NewDateRec(2024, time.February, 29), false), 3797.6090, 402.78, 3166.53)
	check("Feb-end Feb28→Mar28", mk(types.NewDateRec(2025, time.February, 28), types.NewDateRec(2025, time.March, 28), false), 3796.5626, 388.89, 3151.88)
	check("clamped prepaid (cleared)", mk(types.NewDateRec(2024, time.January, 31), types.NewDateRec(2024, time.February, 29), true), 3797.6090, 402.78, 3166.53)
	check("ordinary pair control", mk(types.NewDateRec(2024, time.January, 15), types.NewDateRec(2024, time.February, 15), false), 3798.6555, 416.67, 3181.18)
}

// AF6: the display very-last fold retires a prepay-series loan's residual into
// the final scheduled payment exactly as for plain/balloon loans.
//
//	amort_oracle 50000 0.08 60 12 payhard=900 pre=6:12:12:200
//	  → final row int 138.15 prin 20722.07 bal 0.00, interest 15560.22, paid 65560.22
//	amort_oracle 200000 0.09 480 12 payhard=1600 pre=12:120:12:100 → paid 4258344.89
func TestPass3AF6PrepaySeriesResidualFold(t *testing.T) {
	mk := func(amount, rate, pay float64, n int, preStart types.DateRec, nn int, preAmt float64) LoanInput {
		return LoanInput{Loan: pass1Loan(amount, rate, pay, n), Settings: basicSettings(), Fancy: true,
			Prepayments: []Prepayment{{
				StartDateStatus: types.InOutInput, StartDate: preStart,
				NNStatus: types.InOutInput, NN: nn,
				PerYrStatus: types.InOutInput, PerYr: 12,
				PaymentStatus: types.InOutInput, Payment: preAmt,
			}}}
	}
	res := Amortize(mk(50000, 0.08, 900, 60, types.NewDateRec(2024, time.July, 1), 12, 200))
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	if math.Abs(res.TotalPaid-65560.22) > 0.05 || math.Abs(res.FinalPrinc) > 0.01 {
		t.Errorf("paid=%.2f finalBal=%.4f, want 65560.22 / 0 (oracle; unfixed left bal 19960.22)", res.TotalPaid, res.FinalPrinc)
	}
	last := res.Schedule[len(res.Schedule)-1]
	if math.Abs(last.Interest-138.15) > 0.005 {
		t.Errorf("final row int = %.4f, want 138.15 (oracle)", last.Interest)
	}
	res2 := Amortize(mk(200000, 0.09, 1600, 480, types.NewDateRec(2025, time.January, 1), 120, 100))
	if res2.Err != nil || math.Abs(res2.TotalPaid-4258344.89) > 0.05 || math.Abs(res2.FinalPrinc) > 0.01 {
		t.Errorf("neg-am: paid=%.2f finalBal=%.4f err=%v, want 4258344.89 / 0 (oracle)", res2.TotalPaid, res2.FinalPrinc, res2.Err)
	}
}

// AF3: a semimonthly (24/yr) first date on day 31 — DOS's schedule WALK
// re-grids the row dates to the 1st/16th via the AddPeriod(24) round trip
// (INTSUTIL.pas:1208-1237) while the SOLVE keeps the original date's prorate.
//
//	amort_oracle 50000 0.10 26 24 loandmy=15.1.2024 firstdmy=31.1.2024 b365 rows
//	  → payment 2033.5386, rows on 2/1, 2/16, …: row1 int 232.24 (17 actual
//	    days from 1/15), row2 197.54, row3 177.34, row4 182.40; interest 2872.49
//	amort_oracle 50000 0.10 26 24 loandmy=15.1.2024 firstdmy=31.1.2024 b365 targ=0.01
//	  → payment 2033.5562, same row grid, interest 2872.46 (fancy control —
//	    already clean via the DOS-port walk)
func TestPass3AF3SemimonthlyDay31Regrid(t *testing.T) {
	s := Settings{Basis: types.Basis365, PerYr: 24, YrDays: 365.25, YrInv: 1.0 / 365.25}
	loan := Loan{
		AmountStatus: types.InOutInput, Amount: 50000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.10,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 26,
		PerYrStatus: types.InOutInput, PerYr: 24,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 15),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 31),
	}
	res := Amortize(LoanInput{Loan: loan, Settings: s})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	var pay float64
	for _, r := range res.Schedule {
		if r.PayNum == 1 {
			pay = r.PayAmt
			break
		}
	}
	if math.Abs(pay-2033.5386) > 0.005 {
		t.Errorf("payment = %.4f, want 2033.5386 (oracle; the ORIGINAL 31.1 prorate, not the re-gridded 2034.09)", pay)
	}
	if math.Abs(res.TotalInt-2872.49) > 0.05 {
		t.Errorf("totInt = %.2f, want 2872.49 (oracle)", res.TotalInt)
	}
	wantRows := []struct {
		date string
		intr float64
	}{{"02-01", 232.24}, {"02-16", 197.54}, {"03-01", 177.34}, {"03-16", 182.40}}
	for i, w := range wantRows {
		r := res.Schedule[i]
		if r.Date.Time.Format("01-02") != w.date || math.Abs(r.Interest-w.intr) > 0.005 {
			t.Errorf("row%d = %s int %.4f, want %s int %.2f (oracle re-gridded walk)",
				i+1, r.Date.Time.Format("01-02"), r.Interest, w.date, w.intr)
		}
	}
}

// AF4: USA-rule never changes the SOLVED payment — DOS's Iterate terminal
// stays on the plain RepayLoan (AMORTOP.pas:1438; usap only shapes the
// DISPLAYED rows). The port now routes only the display to the usap-aware
// fancy walk (usaFancyDisplay) instead of forcing the whole loan fancy.
//
//	amort_oracle 100000 0.09 120 12 usa inadv b365   → payment 1260.9130, interest 53185.09 (= plain inadv)
//	amort_oracle 200000 0.09 1040 26 b365 usa        → payment 709.6785, interest 537519.09 (= plain)
//	amort_oracle 474551.76 0.22106832 25 26 b365 usa → payment 21142.6074 (pass-3 fuzz cluster)
func TestPass3AF4USASolveIsPlain(t *testing.T) {
	pay1 := func(res AmortResult) float64 {
		for _, r := range res.Schedule {
			if r.PayNum == 1 {
				return r.PayAmt
			}
		}
		return 0
	}
	mk := func(amount, rate float64, n, peryr int, inadv bool, fd types.DateRec) LoanInput {
		s := Settings{Basis: types.Basis365, PerYr: byte(peryr), YrDays: 365.25, YrInv: 1.0 / 365.25,
			USARule: true, InAdvance: inadv}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: peryr,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: s}
	}
	r1 := Amortize(mk(100000, 0.09, 120, 12, true, types.NewDateRec(2024, time.February, 1)))
	if r1.Err != nil || math.Abs(pay1(r1)-1260.9130) > 0.005 || math.Abs(r1.TotalInt-53185.09) > 0.05 {
		t.Errorf("usa inadv b365: pay=%.4f int=%.2f err=%v, want 1260.9130 / 53185.09 (oracle; fancy-terminal solve gave 1273.34)",
			pay1(r1), r1.TotalInt, r1.Err)
	}
	r2 := Amortize(mk(200000, 0.09, 1040, 26, false, types.NewDateRec(2024, time.January, 15)))
	if r2.Err != nil || math.Abs(pay1(r2)-709.6785) > 0.005 || math.Abs(r2.TotalInt-537519.09) > 0.05 {
		t.Errorf("usa biweekly b365: pay=%.4f int=%.2f err=%v, want 709.6785 / 537519.09 (oracle)",
			pay1(r2), r2.TotalInt, r2.Err)
	}
	r3 := Amortize(mk(474551.76, 0.22106832, 25, 26, false, types.NewDateRec(2024, time.January, 15)))
	if r3.Err != nil || math.Abs(pay1(r3)-21142.6074) > 0.005 {
		t.Errorf("usa fuzz: pay=%.4f err=%v, want 21142.6074 (oracle; was 21139.0057)", pay1(r3), r3.Err)
	}
}

// AF5: an OVERFUNDED in-advance hard payment retires early — DOS's WhenToStop
// truncates the schedule; the final row pays the remaining balance with zero
// in-advance interest.
//
//	amort_oracle 100000 0.09 120 12 payhard=1300 inadv rows
//	  → 115 rows, row 115 int 0.00 prin 342.74 bal 0.00; interest 49292.74 paid 149292.74
func TestPass3AF5InAdvanceOverfundedTruncation(t *testing.T) {
	s := basicSettings()
	s.InAdvance = true
	res := Amortize(LoanInput{Loan: pass1Loan(100000, 0.09, 1300, 120), Settings: s})
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	var payRows int
	for _, r := range res.Schedule {
		if r.PayNum >= 1 {
			payRows++
		}
	}
	last := res.Schedule[len(res.Schedule)-1]
	if payRows != 115 || math.Abs(last.PayAmt-342.74) > 0.005 || math.Abs(last.Interest) > 0.005 ||
		math.Abs(res.FinalPrinc) > 0.01 {
		t.Errorf("rows=%d lastPay=%.2f lastInt=%.4f finalBal=%.4f, want 115 / 342.74 / 0 / 0 (oracle; was 120 rows bal −6157.26)",
			payRows, last.PayAmt, last.Interest, res.FinalPrinc)
	}
	if math.Abs(res.TotalPaid-149292.74) > 0.05 || math.Abs(res.TotalInt-49292.74) > 0.05 {
		t.Errorf("paid=%.2f int=%.2f, want 149292.74 / 49292.74 (oracle)", res.TotalPaid, res.TotalInt)
	}
}

// P3-F1 + P3-F2: term solves. DOS's DetermineLastPaymentDate takes the
// closed-form log branch for ANY loan without advanced options — exact/USA are
// not triggers (AMORTOP.pas:1383-1397; the REPORTED n comes from the closed
// form even when the rendered walk stops a row earlier). Prepaid pins the
// closed form's prorate to EXACTLY 1 (Amortize.pas:1277-1282).
//
//	amort_oracle 10000 0.09 0 12 b365 usa payhard=456.90 noterm     → solvedterm 25 last 2026-2-1, interest 964.21
//	amort_oracle 10000 0.09 0 12 b365 exact payhard=456.90 noterm   → solvedterm 25 last 2026-2-1, interest 962.48
//	amort_oracle 10000 0.09 0 4 b365 exact payhard=1379.62 noterm   → solvedterm 9 last 2026-4-1, interest 1036.95
//	amort_oracle 10000 0.09 0 4 b365 usa payhard=1379.68 noterm     → solvedterm 8 last 2026-1-1, interest 1038.87
//	amort_oracle 10000 0.09 0 12 b365 prepaid payhard=456.85 noterm → solvedterm 24 last 2026-1-1
//	amort_oracle 10000 0.09 0 26 b365 prepaid payhard=210.40 noterm → solvedterm 53 last 2026-1-12
func TestPass3F1F2TermSolves(t *testing.T) {
	mk := func(pay float64, peryr int, usa, exact, prepaid bool) LoanInput {
		fd := types.NewDateRec(2024, time.February, 1)
		if peryr == 4 {
			fd = types.NewDateRec(2024, time.April, 1)
		}
		if peryr == 26 {
			fd = types.NewDateRec(2024, time.January, 15)
		}
		s := Settings{Basis: types.Basis365, PerYr: byte(peryr), YrDays: 365.25, YrInv: 1.0 / 365.25,
			USARule: usa, Exact: exact, Prepaid: prepaid}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 10000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.09,
			PayAmtStatus: types.InOutInput, PayAmt: pay,
			NStatus:     types.StatusEmpty,
			PerYrStatus: types.InOutInput, PerYr: peryr,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: s}
	}
	cases := []struct {
		tag      string
		in       LoanInput
		wantN    int
		wantLast string
		wantInt  float64
	}{
		{"usa monthly", mk(456.90, 12, true, false, false), 25, "2026-1-2", 964.21},
		{"exact monthly", mk(456.90, 12, false, true, false), 25, "2026-1-2", 962.48},
		{"exact quarterly", mk(1379.62, 4, false, true, false), 9, "2026-4-1", 1036.95},
		{"usa quarterly", mk(1379.68, 4, true, false, false), 8, "2026-1-1", 1038.87},
		{"prepaid monthly", mk(456.85, 12, false, false, true), 24, "2026-1-1", -1},
		{"prepaid biweekly", mk(210.40, 26, false, false, true), 53, "2026-1-12", -1},
	}
	// wantLast strings for the two monthly cases: DOS prints 2026-2-1.
	cases[0].wantLast = "2026-2-1"
	cases[1].wantLast = "2026-2-1"
	for _, c := range cases {
		res := Amortize(c.in)
		if res.Err != nil {
			t.Errorf("%s: err %v", c.tag, res.Err)
			continue
		}
		last := res.LastDate.Time.Format("2006-1-2")
		if res.NPeriods != c.wantN || last != c.wantLast {
			t.Errorf("%s: n=%d last=%s, want %d / %s (oracle solvedterm)", c.tag, res.NPeriods, last, c.wantN, c.wantLast)
		}
		if c.wantInt > 0 && math.Abs(res.TotalInt-c.wantInt) > 0.05 {
			t.Errorf("%s: int=%.2f, want %.2f (oracle)", c.tag, res.TotalInt, c.wantInt)
		}
	}
}

// P3-F6: under prepaid with a settlement stub, row 1 of the day-count accrual
// branches anchors on the SHIFTED start (repay_from = firstdate − 1 period,
// Amortize.pas:1277-1281) — the loan→first span was already collected in
// row 0; anchoring on the loan date charged it AGAIN and capitalized it.
//
//	amort_oracle 50000 0.11 52 26 b365 prepaid loandmy=1.1.2024 firstdmy=1.1.2027 dumpraw
//	  → payment 1072.8119, L0 settlement 16289.04, row1 int 210.96, interest 22075.72
//	    (Go's row1 was 16500.00 and totals 42260.55, ~2×)
//	amort_oracle 50000 0.11 52 26 b365 prepaid loandmy=5.1.2024 firstdmy=20.1.2024  → interest 5792.12
//	amort_oracle 50000 0.11 52 24 b365 prepaid loandmy=1.1.2024 firstdmy=1.1.2027   → payment 1082.8605, interest 22570.04
//	amort_oracle 50000 0.11 52 26 b365_360 prepaid loandmy=1.1.2024 firstdmy=1.1.2027 → payment 1074.4912, interest 22404.10
func TestPass3F6PrepaidDayCountStub(t *testing.T) {
	mk := func(peryr int, basis types.BasisType, ld, fd types.DateRec) LoanInput {
		yd, yi := 365.25, 1.0/365.25
		if basis == types.Basis365360 {
			yd, yi = 360.0, 1.0/360
		}
		s := Settings{Basis: basis, PerYr: byte(peryr), YrDays: yd, YrInv: yi, Prepaid: true}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 50000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.11,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 52,
			PerYrStatus: types.InOutInput, PerYr: peryr,
			LoanDateStatus: types.InOutInput, LoanDate: ld,
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: s}
	}
	r1 := Amortize(mk(26, types.Basis365, types.NewDateRec(2024, time.January, 1), types.NewDateRec(2027, time.January, 1)))
	if r1.Err != nil {
		t.Fatalf("err: %v", r1.Err)
	}
	var row0, row1 float64
	for _, r := range r1.Schedule {
		if r.PayNum == 0 {
			row0 = r.Interest
		}
		if r.PayNum == 1 {
			row1 = r.Interest
		}
	}
	if math.Abs(row0-16289.04) > 0.05 || math.Abs(row1-210.96) > 0.05 || math.Abs(r1.TotalInt-22075.72) > 0.05 {
		t.Errorf("3yr biweekly: row0=%.2f row1=%.2f int=%.2f, want 16289.04 / 210.96 / 22075.72 (oracle; unfixed row1 16500.00, int 42260.55)",
			row0, row1, r1.TotalInt)
	}
	checks := []struct {
		tag     string
		in      LoanInput
		wantInt float64
	}{
		{"odd-day 26/yr", mk(26, types.Basis365, types.NewDateRec(2024, time.January, 5), types.NewDateRec(2024, time.January, 20)), 5792.12},
		{"3yr semimonthly", mk(24, types.Basis365, types.NewDateRec(2024, time.January, 1), types.NewDateRec(2027, time.January, 1)), 22570.04},
		{"3yr 365/360", mk(26, types.Basis365360, types.NewDateRec(2024, time.January, 1), types.NewDateRec(2027, time.January, 1)), 22404.10},
	}
	for _, c := range checks {
		res := Amortize(c.in)
		if res.Err != nil || math.Abs(res.TotalInt-c.wantInt) > 0.05 {
			t.Errorf("%s: int=%.2f err=%v, want %.2f (oracle)", c.tag, res.TotalInt, res.Err, c.wantInt)
		}
	}
}

// P3-F7: a 1-day first stub at weekly/biweekly frequency on the 365/360 basis —
// the payment solve iterates RepayLoan with the actual-day prorate (1/360·peryr)
// and DOS's day-count growth factor (1 + 14·rate/360 biweekly). Was off ~21¢
// when pass 3 probed it; pinned here against the oracle.
//
//	amort_oracle 50000 0.11 52 26 b365_360 loandmy=1.1.2024 firstdmy=2.1.2024 → payment 1070.2449, interest 5652.53
//	amort_oracle 50000 0.11 52 52 b365_360 loandmy=1.1.2024 firstdmy=2.1.2024 → payment 1015.1715, interest 2788.73
func TestPass3F7OneDayStub365360(t *testing.T) {
	for _, c := range []struct {
		peryr            int
		wantPay, wantInt float64
	}{{26, 1070.2449, 5652.53}, {52, 1015.1715, 2788.73}} {
		s := Settings{Basis: types.Basis365360, PerYr: byte(c.peryr), YrDays: 360, YrInv: 1.0 / 360}
		loan := Loan{
			AmountStatus: types.InOutInput, Amount: 50000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.11,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 52,
			PerYrStatus: types.InOutInput, PerYr: c.peryr,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 2),
		}
		res := Amortize(LoanInput{Loan: loan, Settings: s})
		if res.Err != nil {
			t.Fatalf("peryr=%d: %v", c.peryr, res.Err)
		}
		var pay float64
		for _, r := range res.Schedule {
			if r.PayNum == 1 {
				pay = r.PayAmt
				break
			}
		}
		if math.Abs(pay-c.wantPay) > 0.005 || math.Abs(res.TotalInt-c.wantInt) > 0.05 {
			t.Errorf("peryr=%d: pay=%.4f int=%.2f, want %.4f / %.2f (oracle)", c.peryr, pay, res.TotalInt, c.wantPay, c.wantInt)
		}
	}
}

// P3-F3: refusal gates. DOS's SufficientDataOnScreen requires every adjustment
// row to be FULLY specified (date+rate+amount, AMORTOP.pas:393) before
// admitting an unknown balloon (Amortize.pas:889-890) or unknown prepayment
// (:892-894); and a term solve with ANY adjustment aborts ("Payment amount is
// too small to compute number of periods.", AMORTOP.pas:1345-1346).
//
//	amort_oracle 100000 0.08 120 12 adj=24:0.09: payhard=1050 solveballoon=60 → refused
//	amort_oracle 100000 0.08 120 12 adj=24:0.09:1100 payhard=1050 solveballoon=60 → balloon 20696.5200 (control)
//	amort_oracle 100000 0.08 120 12 adj=24:0.09: payhard=1050 presolve=6:12:12 → refused
//	amort_oracle 100000 0.08 0 12 adj=24:0.09:1100 payhard=1050 noterm → ERR
func TestPass3F3RefusalGates(t *testing.T) {
	rateOnlyAdj := []RateAdjustment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2026, time.January, 1),
		LoanRateStatus: types.InOutInput, LoanRate: 0.09}}
	fullAdj := []RateAdjustment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2026, time.January, 1),
		LoanRateStatus: types.InOutInput, LoanRate: 0.09,
		AmountStatus: types.InOutInput, Amount: 1100, AmtOK: true}}
	base := LoanInput{Loan: pass1Loan(100000, 0.08, 1050, 120), Settings: basicSettings(), Fancy: true}

	in1 := base
	in1.Adjustments = rateOnlyAdj
	in1.Balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1)}}
	if _, err := SolveBalloonAmount(in1, 0); err == nil {
		t.Errorf("unknown balloon with a rate-only ARM must refuse (DOS: refused; Go returned the arbitrary 50000 seed)")
	}

	in2 := base
	in2.Adjustments = fullAdj
	in2.Balloons = []BalloonPayment{{DateStatus: types.InOutInput, Date: types.NewDateRec(2029, time.January, 1)}}
	amt, err := SolveBalloonAmount(in2, 0)
	if err != nil || math.Abs(amt-20696.5200) > 0.01 {
		t.Errorf("fully-specified control: %.4f err=%v, want 20696.5200 (oracle)", amt, err)
	}

	in3 := base
	in3.Adjustments = rateOnlyAdj
	in3.Prepayments = []Prepayment{{StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.July, 1),
		NNStatus: types.InOutInput, NN: 12, PerYrStatus: types.InOutInput, PerYr: 12}}
	if _, err := SolvePrepaymentAmount(in3, 0); err == nil {
		t.Errorf("unknown prepayment with a rate-only ARM must refuse (DOS: refused; Go 'solved' 0.0000)")
	}

	l4 := pass1Loan(100000, 0.08, 1050, 120)
	l4.NStatus, l4.NPeriods = types.StatusEmpty, 0
	if res := Amortize(LoanInput{Loan: l4, Settings: basicSettings(), Fancy: true, Adjustments: fullAdj}); res.Err == nil {
		t.Errorf("term solve with a fully-specified ARM must error (DOS ABORT; Go solved n=127)")
	}
	if res := Amortize(LoanInput{Loan: l4, Settings: basicSettings(), Fancy: true, Adjustments: rateOnlyAdj}); res.Err == nil {
		t.Errorf("term solve with an incomplete ARM row must error (DOS: insufficient data)")
	}
}

// P3-F4/F5: the payment/amount/rate Iterate terminals NEVER re-amortize at
// adjustments — DOS's Re_Amortize gate is `((next_adj <= adjnum) or entire)`
// (AMORTOP.pas:1215) and Iterate runs with entire=til_adj=FALSE, adjnum=0, so
// the solved value is identical with and without the ARM. The balloon-amount
// and unknown-prepayment solves DO honor fully-specified ARMs (their walks
// re-amortize, like DOS's entire/value_calc walks).
//
//	amort_oracle 100000 0.08 120 12 b365 exact adj=24:0.10: → payment 1213.0959 (= plain exact), interest 54113.37,
//	  post-adj payment fixed at 1302.07
//	amort_oracle 0 0.08 120 12 b365 exact adj=24:0.10: pay=1213.0959 noamt   → solvedamount 99999.9973
//	amort_oracle 100000 0 120 12 b365 exact adj=24:0.10: pay=1213.0959 norate → solvedrate 0.0799999938
//	amort_oracle 0 0.08 120 12 b365_360 exact adj=24:0.10: pay=1213.0959 noamt → 99485.1209 (the b365 payment fed
//	  into the b365_360 walk — pins the walk, not a round-trip)
//	amort_oracle 100000 0 120 12 b365 adj=24:0.10: pay=1213.2759 norate → solvedrate 0.0799999918
//	amort_oracle 100000 0.08 120 12 payhard=1050 adj=24:0.09:1100 presolve=6:12:12 → prepay 2198.1283
//	  (no-adj control 2260.1872)
func TestPass3F4F5ARMSolves(t *testing.T) {
	mkAdj := func(rate, amt float64) []RateAdjustment {
		a := RateAdjustment{DateStatus: types.InOutInput, Date: types.NewDateRec(2026, time.January, 1)}
		if rate > 0 {
			a.LoanRateStatus, a.LoanRate = types.InOutInput, rate
		}
		if amt > 0 {
			a.AmountStatus, a.Amount, a.AmtOK = types.InOutInput, amt, true
		}
		return []RateAdjustment{a}
	}
	mk := func(basis types.BasisType, exact bool, pay float64, adjs []RateAdjustment) LoanInput {
		yd, yi := 365.25, 1.0/365.25
		if basis != types.Basis365 {
			yd, yi = 360.0, 1.0/360
		}
		s := Settings{Basis: basis, PerYr: 12, YrDays: yd, YrInv: yi, Exact: exact}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.08,
			PayAmtStatus: types.InOutInput, PayAmt: pay,
			NStatus: types.InOutInput, NPeriods: 120,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
		}, Settings: s, Fancy: true, Adjustments: adjs}
	}

	// Forward exact display: solved base payment = plain exact; adj re-amortizes.
	fwd := mk(types.Basis365, true, 0, mkAdj(0.10, 0))
	fwd.Loan.PayAmtStatus = types.StatusEmpty
	res := Amortize(fwd)
	var pay1, payPost float64
	for _, r := range res.Schedule {
		if r.PayNum == 1 {
			pay1 = r.PayAmt
		}
		if r.Date.Time.Format("2006-01-02") == "2026-02-01" {
			payPost = r.PayAmt
		}
	}
	if res.Err != nil || math.Abs(pay1-1213.0959) > 0.005 || math.Abs(payPost-1302.07) > 0.01 ||
		math.Abs(res.TotalInt-54113.37) > 0.05 {
		t.Errorf("exact ARM fwd: pay1=%.4f post=%.4f int=%.2f err=%v, want 1213.0959 / 1302.07 / 54113.37 (oracle)",
			pay1, payPost, res.TotalInt, res.Err)
	}

	// Amount / rate solves ignore the ARM (exact b365).
	inA := mk(types.Basis365, true, 1213.0959, mkAdj(0.10, 0))
	inA.Loan.AmountStatus, inA.Loan.Amount = types.StatusEmpty, 0
	if amt, _, err := SolveLoanAmount(inA); err != nil || math.Abs(amt-99999.9973) > 0.01 {
		t.Errorf("exact ARM amount = %.4f err=%v, want 99999.9973 (oracle; flat-residual wander gave 26824.60)", amt, err)
	}
	inR := mk(types.Basis365, true, 1213.0959, mkAdj(0.10, 0))
	inR.Loan.LoanRateStatus, inR.Loan.LoanRate = types.StatusEmpty, 0
	if r, _, err := SolveRate(inR); err != nil || math.Abs(r-0.0799999938) > 5e-6 {
		t.Errorf("exact ARM rate = %.8f err=%v, want 0.0799999938 (oracle; was −0.98)", r, err)
	}
	// b365_360 walk pinned with the b365 payment (not a round-trip — golden is
	// DOS's own answer for the same mismatched input).
	inB := mk(types.Basis365360, true, 1213.0959, mkAdj(0.10, 0))
	inB.Loan.AmountStatus, inB.Loan.Amount = types.StatusEmpty, 0
	if amt, _, err := SolveLoanAmount(inB); err != nil || math.Abs(amt-99485.1209) > 0.01 {
		t.Errorf("exact ARM 365/360 amount = %.4f err=%v, want 99485.1209 (oracle)", amt, err)
	}
	// F5: non-exact non-360 rate solve.
	inF := mk(types.Basis365, false, 1213.2759, mkAdj(0.10, 0))
	inF.Loan.LoanRateStatus, inF.Loan.LoanRate = types.StatusEmpty, 0
	if r, _, err := SolveRate(inF); err != nil || math.Abs(r-0.0799999918) > 5e-6 {
		t.Errorf("non-exact ARM rate = %.8f err=%v, want 0.0799999918 (oracle; drifted 2.5e-5)", r, err)
	}
	// Unknown prepayment honors a fully-specified ARM.
	inP := mk(types.Basis360, false, 1050, mkAdj(0.09, 1100))
	inP.Prepayments = []Prepayment{{StartDateStatus: types.InOutInput, StartDate: types.NewDateRec(2024, time.July, 1),
		NNStatus: types.InOutInput, NN: 12, PerYrStatus: types.InOutInput, PerYr: 12}}
	if amt, err := SolvePrepaymentAmount(inP, 0); err != nil || math.Abs(amt-2198.1283) > 0.01 {
		t.Errorf("prepay+ARM = %.4f err=%v, want 2198.1283 (oracle; no-adj control 2260.1872)", amt, err)
	}
}
