// Regressions for the 2026-07-13 audit PASS-4 findings (docs/discrepancies.md
// §23). Every golden cites the real DOS-engine command that produced it.
package amortization

import (
	"math"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// P4-1: DOS's payment solve is entirely R78-agnostic (EstimateAndRefinePayment
// never inspects the R78 flag — R78 only changes the interest SPLIT), so an
// R78 loan's payment equals the plain loan's. The prior needPaymentRefine had
// an `if s.R78 { return s.InAdvance }` special-case that skipped the odd-first
// refine for arrears R78, leaving the un-refined estimate.
//
//	amort_oracle 10000 0.1213 36 24 r78 → payment 302.9827, interest 907.38
//	  (first payment ON the loan date — a zero-length first period; the
//	   un-refined estimate was 304.5140. The plain semimonthly payment is
//	   also 302.9827: R78 does not change it.)
//	amort_oracle 10000 0.1213 36 12 r78 → payment 332.7643 (monthly control,
//	  always matched)
func TestPass4R78PaymentIsPlain(t *testing.T) {
	// Semimonthly with the first payment ON the loan date (a zero-length first
	// period), reproducing the goamort/oracle default for peryr=24.
	sSemi := Settings{Basis: types.Basis360, PerYr: 24, YrDays: 360, YrInv: 1.0 / 360, R78: true}
	loanSemi := Loan{
		AmountStatus: types.InOutInput, Amount: 10000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.1213,
		PayAmtStatus: types.StatusEmpty,
		NStatus:      types.InOutInput, NPeriods: 36,
		PerYrStatus: types.InOutInput, PerYr: 24,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 1),
	}
	res := Amortize(LoanInput{Loan: loanSemi, Settings: sSemi})
	var pay float64
	for _, r := range res.Schedule {
		if r.PayNum == 1 {
			pay = r.PayAmt
			break
		}
	}
	if res.Err != nil || math.Abs(pay-302.9827) > 0.005 || math.Abs(res.TotalInt-907.38) > 0.05 {
		t.Errorf("R78 semimonthly: pay=%.4f int=%.2f err=%v, want 302.9827 / 907.38 (oracle; un-refined was 304.5140)",
			pay, res.TotalInt, res.Err)
	}
	// The plain (non-R78) loan must give the SAME payment.
	sPlain := sSemi
	sPlain.R78 = false
	res0 := Amortize(LoanInput{Loan: loanSemi, Settings: sPlain})
	var pay0 float64
	for _, r := range res0.Schedule {
		if r.PayNum == 1 {
			pay0 = r.PayAmt
			break
		}
	}
	if math.Abs(pay-pay0) > 0.005 {
		t.Errorf("R78 payment %.4f != plain payment %.4f — R78 must not change the payment", pay, pay0)
	}
}

// P4-2: R78 SUPPRESSES exact in DOS's display gate `(exact and not R78) or
// non-360` (Amortize.pas:1493), so an R78 × exact × in-advance loan on the 360
// basis uses the plain in-advance R78 schedule (sum-of-digits allocation whose
// total is n·d − principal), NOT the exact settlement-shifted shape. The
// display arm previously fired for any `exact && inadv`, adding a spurious
// exact settlement.
//
//	amort_oracle 100000 0.1173 48 24 r78 exact inadv → payment 2332.1827, interest 11944.77
//	amort_oracle 100000 0.1173 48 24 r78 inadv       → payment 2332.1827, interest 11944.77 (= n·d − principal)
//	  (the exact arm rendered 12477.59, adding a spurious exact settlement)
func TestPass4R78SuppressesExactDisplay(t *testing.T) {
	mk := func(exact bool) LoanInput {
		s := Settings{Basis: types.Basis360, PerYr: 24, YrDays: 360, YrInv: 1.0 / 360, R78: true, InAdvance: true, Exact: exact}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.1173,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 48,
			PerYrStatus: types.InOutInput, PerYr: 24,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 1),
		}, Settings: s}
	}
	resExact := Amortize(mk(true))
	if resExact.Err != nil || math.Abs(resExact.TotalInt-11944.77) > 0.05 {
		t.Errorf("R78+exact+inadv: int=%.2f err=%v, want 11944.77 (oracle; exact arm gave 12477.59)",
			resExact.TotalInt, resExact.Err)
	}
	// Must equal the R78+inadv (no-exact) schedule — R78 suppresses exact.
	resPlain := Amortize(mk(false))
	if math.Abs(resExact.TotalInt-resPlain.TotalInt) > 0.05 {
		t.Errorf("R78+exact+inadv int %.2f != R78+inadv int %.2f — R78 must suppress exact",
			resExact.TotalInt, resPlain.TotalInt)
	}
}

// P4-F2: in-advance payoff reconstructs DOS's balance_calc RepayFancyLoan walk
// (Amortize.pas:1114-1125), whose base_date starts at firstdate (shifted vs
// arrears) and which accrues plain opening-balance interest with NO settlement
// row — structurally different from the display schedule. Reading display rows
// left every in-advance payoff ~0.2–0.7% low.
//
//	amort_oracle 100000 0.0632 48 12 payhard=684.67 payoff=15.6.2025 inadv → 97096.1096 (display-row formula: 96909.2958)
//	amort_oracle 100000 0.0632 48 12 payhard=684.67 payoff=1.6.2025 inadv  → 97540.5700 (on a payment date)
//	amort_oracle 50000 0.1121 60 24 payhard=303.6 payoff=15.6.2025 inadv   → 47410.2224 (semimonthly)
//	amort_oracle 100000 0.0632 48 12 payhard=684.67 payoff=15.6.2025       → 97436.6405 (arrears control, unchanged)
func TestPass4InAdvancePayoffWalk(t *testing.T) {
	mk := func(amt, rate float64, n, peryr int, payh float64, inadv bool) LoanInput {
		yd, yi := 360.0, 1.0/360
		s := Settings{Basis: types.Basis360, PerYr: byte(peryr), YrDays: yd, YrInv: yi, InAdvance: inadv}
		fd := types.NewDateRec(2024, time.February, 1)
		if peryr == 24 {
			// goamort/oracle default first date for peryr=24 is the loan date.
			fd = types.NewDateRec(2024, time.January, 1)
		}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amt,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			PayAmtStatus: types.InOutInput, PayAmt: payh,
			NStatus:      types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: peryr,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: s}
	}
	cases := []struct {
		tag        string
		in         LoanInput
		y, m, dd   int
		want       float64
	}{
		{"inadv mid-period", mk(100000, 0.0632, 48, 12, 684.67, true), 2025, 6, 15, 97096.1096},
		{"inadv on-payment", mk(100000, 0.0632, 48, 12, 684.67, true), 2025, 6, 1, 97540.5700},
		{"inadv semimonthly", mk(50000, 0.1121, 60, 24, 303.6, true), 2025, 6, 15, 47410.2224},
		{"arrears control", mk(100000, 0.0632, 48, 12, 684.67, false), 2025, 6, 15, 97436.6405},
	}
	for _, c := range cases {
		got, err := PayoffBalance(c.in, types.NewDateRec(c.y, time.Month(c.m), c.dd))
		if err != nil || math.Abs(got-c.want) > 0.005 {
			t.Errorf("%s: payoff = %.4f err=%v, want %.4f (oracle)", c.tag, got, err, c.want)
		}
	}
}

// P4-F: prepaid × moratorium × day-count frequency — DOS's EARLY-EXIT
// (Amortize.pas:402-407): a prepaid loan with none of exact/in-advance/balloon/
// prepayment/target/skip takes the closed-form annuity over the amortizing
// count nrepay (Amortize.pas:1302) and does NOT Iterate. At a day-count
// frequency this differs from the actual-day Iterate the non-prepaid case uses.
//
//	amort_oracle 100000 0.10 104 52 prepaid mor=3 → payment 1186.6343, interest 11453.07
//	amort_oracle 250000 0.1397 120 26 prepaid mor=4 → payment 2973.7798
//	amort_oracle 100000 0.10 104 52 mor=3 → payment 1186.5113 (mor-alone Iterate, unchanged)
func TestPass4PrepaidMoratoriumEarlyExit(t *testing.T) {
	mk := func(amt, rate float64, n, peryr, mor int, prepaid bool) LoanInput {
		// weekly/biweekly force the 365 basis (Amortize.pas:300-305).
		basis, yd, yi := types.Basis360, 360.0, 1.0/360
		if peryr == 26 || peryr == 52 {
			basis, yd, yi = types.Basis365, 365.25, 1.0/365.25
		}
		s := Settings{Basis: basis, PerYr: byte(peryr), YrDays: yd, YrInv: yi, Prepaid: prepaid}
		fd := types.DateRec{Time: types.NewDateRec(2024, time.January, 1).Time.AddDate(0, 0, 364/peryr)}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amt,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: peryr,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: fd,
		}, Settings: s, Fancy: true,
			Moratorium: Moratorium{FirstRepayStatus: types.InOutInput,
				FirstRepay: types.DateRec{Time: types.NewDateRec(2024, time.January, 1).Time.AddDate(0, mor, 0)}}}
	}
	// Modal (most common) regular payment — skips the interest-only moratorium
	// rows and any final fold.
	pay1 := func(res AmortResult) float64 {
		counts := map[int64]int{}
		amt := map[int64]float64{}
		bestK, bestN := int64(0), 0
		for _, r := range res.Schedule {
			if r.PayNum < 1 || r.PayAmt <= 0 {
				continue
			}
			k := int64(r.PayAmt*100 + 0.5)
			counts[k]++
			amt[k] = r.PayAmt
			if counts[k] > bestN {
				bestN, bestK = counts[k], k
			}
		}
		return amt[bestK]
	}
	// prepaid+mor takes the closed form (higher payment).
	r1 := Amortize(mk(100000, 0.10, 104, 52, 3, true))
	if r1.Err != nil || math.Abs(pay1(r1)-1186.6343) > 0.005 {
		t.Errorf("prepaid+mor weekly: pay=%.4f err=%v, want 1186.6343 (oracle; the Iterate gave the mor-alone 1186.5113)", pay1(r1), r1.Err)
	}
	r2 := Amortize(mk(250000, 0.1397, 120, 26, 4, true))
	if r2.Err != nil || math.Abs(pay1(r2)-2973.7798) > 0.005 {
		t.Errorf("prepaid+mor biweekly: pay=%.4f err=%v, want 2973.7798 (oracle)", pay1(r2), r2.Err)
	}
	// mor-alone (non-prepaid) still Iterates — unchanged.
	r3 := Amortize(mk(100000, 0.10, 104, 52, 3, false))
	if r3.Err != nil || math.Abs(pay1(r3)-1186.5113) > 0.005 {
		t.Errorf("mor-alone weekly: pay=%.4f err=%v, want 1186.5113 (oracle, unchanged)", pay1(r3), r3.Err)
	}
	// R78 routes through the PIECEWISE engine (excluded from AmortizeDOS), so
	// the early-exit must also fire in the mid-schedule moratorium recompute.
	// R78 does not change the payment, so R78+prepaid+mor = prepaid+mor.
	//   amort_oracle 250000 0.0711 60 26 r78 prepaid mor=6 → payment 5563.4990
	r4in := mk(250000, 0.0711, 60, 26, 6, true)
	r4in.Settings.R78 = true
	r4 := Amortize(r4in)
	if r4.Err != nil || math.Abs(pay1(r4)-5563.4990) > 0.005 {
		t.Errorf("R78+prepaid+mor biweekly: pay=%.4f err=%v, want 5563.4990 (oracle; the day-count segment Iterate gave 5563.2471)", pay1(r4), r4.Err)
	}
}

// P4-N1: in-advance × skip × moratorium — the in-advance base-date shift can
// land the moratorium FirstRepay boundary on a SKIPPED month. DOS's ComputeNext
// zeroes payamt for a skipped month first, and the past-moratorium arm leaves
// that zero (AMORTOP.pas:596,648-653); the piecewise moratorium recompute
// (used for in-advance, which bypasses AmortizeDOS) was overriding it with the
// recomputed payment. Now re-applies the skip after the recompute.
//
//	amort_oracle 100000 0.10 36 12 inadv skip=4-6 mor=3 → payment 4659.3825, interest 18151.23
//	  (the 4/1 boundary row is a SKIP; Go paid there and gave 16695.51)
//	amort_oracle 100000 0.10 36 12 skip=4-6 mor=3 → interest 18151.23 (non-inadv, unchanged)
func TestPass4InAdvanceSkipMoratorium(t *testing.T) {
	ms, _ := MonthSetFromString("4-6")
	mk := func(inadv bool) LoanInput {
		s := Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, InAdvance: inadv}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 100000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.10,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 36,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
		}, Settings: s, Fancy: true,
			Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2024, time.April, 1)},
			SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "4-6", MonthSet: ms}}
	}
	res := Amortize(mk(true))
	if res.Err != nil || math.Abs(res.TotalInt-18151.23) > 0.05 {
		t.Errorf("inadv+skip+mor: int=%.2f err=%v, want 18151.23 (oracle; Go paid the 4/1 boundary skip row and gave 16695.51)",
			res.TotalInt, res.Err)
	}
	// The 4/1 boundary row must be a skip (pay 0).
	for _, r := range res.Schedule {
		if r.Date.Time.Format("2006-01-02") == "2024-04-01" && r.PayAmt != 0 {
			t.Errorf("4/1 boundary row pays %.2f, want 0 (skipped month)", r.PayAmt)
		}
	}
	// non-inadv control unchanged.
	res0 := Amortize(mk(false))
	if math.Abs(res0.TotalInt-18151.23) > 0.05 {
		t.Errorf("non-inadv skip+mor: int=%.2f, want 18151.23 (unchanged)", res0.TotalInt)
	}
}

// TestPass4InAdvanceTargetSkipMoratoriumQuad guards P4-N1b: the four-option
// in-advance × target × skip × moratorium combo. DOS's Iterate walks the full
// fancy schedule with BOTH skip and target applied — the target floor
// (AMORTOP.pas:643, target overrides skip) converts the skipped months from
// negative-amortizing rows into tiny-principal rows, which lowers the retiring
// payment. The in-advance moratorium segment solve had deliberately omitted the
// target, so the skip rows were solved as pure negative-am and the payment came
// out at the no-target inadv+skip+mor value (3706.5934) instead of the DOS value
// (3500.3264, which equals the non-in-advance quad — in-advance only prepends an
// interest-only settlement row and does not change the amortizing payment).
//
//	amort_oracle 50000 0.164 24 12 inadv targ=0.01 skip=4-6 mor=3 → 3500.3264
//	amort_oracle 50000 0.164 24 12 targ=0.01 skip=4-6 mor=3       → 3500.3264
//	amort_oracle 50000 0.164 24 12 inadv skip=4-6 mor=3           → 3706.5934
func TestPass4InAdvanceTargetSkipMoratoriumQuad(t *testing.T) {
	ms, _ := MonthSetFromString("4-6")
	mk := func(inadv bool, targ bool) LoanInput {
		s := Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, InAdvance: inadv}
		li := LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 50000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.164,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 24,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
		}, Settings: s, Fancy: true,
			Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2024, time.April, 1)},
			SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "4-6", MonthSet: ms}}
		if targ {
			li.Target = Target{TargetStatus: types.InOutInput, TargetValue: 0.01}
		}
		return li
	}
	pay := func(res AmortResult) float64 {
		// modal amortizing payment: the largest payment row (skips the
		// moratorium/target-floored rows).
		var m float64
		for _, r := range res.Schedule {
			if r.PayAmt > m {
				m = r.PayAmt
			}
		}
		return m
	}
	// The quad: DOS 3500.3264 (= non-in-advance quad).
	q := Amortize(mk(true, true))
	if q.Err != nil || math.Abs(pay(q)-3500.33) > 0.01 {
		t.Errorf("inadv+targ+skip+mor: pay=%.2f err=%v, want 3500.33 (oracle; the target-omitting segment solve gave 3706.59)", pay(q), q.Err)
	}
	// Non-in-advance quad control (unchanged).
	nq := Amortize(mk(false, true))
	if math.Abs(pay(nq)-3500.33) > 0.01 {
		t.Errorf("non-inadv quad: pay=%.2f, want 3500.33 (unchanged)", pay(nq))
	}
	// inadv+skip+mor WITHOUT target must stay at 3706.59 (target absent ⇒ skip
	// rows negative-am ⇒ higher payment).
	t3 := Amortize(mk(true, false))
	if math.Abs(pay(t3)-3706.59) > 0.01 {
		t.Errorf("inadv+skip+mor (no target): pay=%.2f, want 3706.59 (unchanged)", pay(t3))
	}
}

// TestPass4SkipRateSingularityIsClean guards P4-N3 (CLASSIFIED deliberate
// divergence). DOS's RepayFancyLoan/MakeTable silently produces ZERO output rows
// (process exits 0, no error, dumpraw → `lines 0`) for a fancy skip-month loan at
// an isolated resonant set of loan rates — e.g. exactly 0.0983 (0.0982 and 0.0984
// build full schedules). It fires with both solved and given payments, is
// independent of amount, and requires skip. This is a DOS engine defect, not
// financial logic; the loans are perfectly valid. Per project policy (prefer the
// financially-correct result over reproducing a DOS engine bug — cf. the
// balloon-on-first in-advance deliberate divergence), the port builds the correct,
// retiring schedule at every one of these rates. This test asserts Go stays clean
// at the singular rate and interpolates smoothly with its neighbours.
//
//	amort_oracle 50000 0.0983 24 12 skip=4-6 → payment 0.0000 lines 0 (DOS: no schedule)
//	amort_oracle 50000 0.0982 24 12 skip=4-6 → interest 5649.03 (DOS builds fine)
//	amort_oracle 50000 0.0984 24 12 skip=4-6 → interest 5660.93 (DOS builds fine)
func TestPass4SkipRateSingularityIsClean(t *testing.T) {
	ms, _ := MonthSetFromString("4-6")
	mk := func(rate float64) LoanInput {
		s := Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 50000,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: 24,
			PerYrStatus: types.InOutInput, PerYr: 12,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
		}, Settings: s, Fancy: true,
			SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "4-6", MonthSet: ms}}
	}
	// The singular rate: Go must build a valid, retiring schedule where DOS emits nothing.
	sing := Amortize(mk(0.0983))
	if sing.Err != nil {
		t.Fatalf("0.0983 skip: unexpected error %v (Go must build the schedule DOS omits)", sing.Err)
	}
	if len(sing.Schedule) == 0 {
		t.Fatalf("0.0983 skip: Go produced zero rows (must not reproduce DOS's zero-output singularity)")
	}
	// Loan must retire: final remaining principal ≈ 0.
	last := sing.Schedule[len(sing.Schedule)-1]
	if math.Abs(last.Principal) > 0.02 {
		t.Errorf("0.0983 skip: final balance %.4f, want ~0 (schedule must retire)", last.Principal)
	}
	// TotalInt must interpolate smoothly between the neighbour rates DOS accepts
	// (5649.03 at 0.0982, 5660.93 at 0.0984) — no discontinuity at the DOS-singular point.
	lo := Amortize(mk(0.0982))
	hi := Amortize(mk(0.0984))
	if sing.TotalInt <= lo.TotalInt || sing.TotalInt >= hi.TotalInt {
		t.Errorf("0.0983 int=%.2f not strictly between neighbours %.2f and %.2f", sing.TotalInt, lo.TotalInt, hi.TotalInt)
	}
	if math.Abs(sing.TotalInt-5654.98) > 0.05 {
		t.Errorf("0.0983 int=%.2f, want ~5654.98 (smooth interpolation)", sing.TotalInt)
	}
}

// TestPass4LongTermInAdvanceAnnualBoundary guards P4-N4 (CLASSIFIED pathological).
// In-advance annual (365-basis) loans match the DOS oracle to the cent for every
// realistic term; the split only appears at ≥95-year terms where the annuity
// payment converges to the interest-only amount r·P and becomes indistinguishable
// from it at cent precision (DOS then keeps interest-only + a final-fold balloon;
// Go amortizes smoothly). This test pins the clean realistic-term values so a
// future change to the in-advance annual path can't silently regress them.
//
//	amort_oracle 10000 0.1715 30 1 b365 inadv → interest 48163.93
//	amort_oracle 10000 0.1715 60 1 b365 inadv → interest 101477.91
//	amort_oracle 10000 0.1715 90 1 b365 inadv → interest 153768.72
func TestPass4LongTermInAdvanceAnnualBoundary(t *testing.T) {
	mk := func(np int) LoanInput {
		s := Settings{Basis: types.Basis365, PerYr: 1, YrDays: 365.25, YrInv: 1.0 / 365.25, InAdvance: true}
		return LoanInput{Loan: Loan{
			AmountStatus: types.InOutInput, Amount: 10000,
			LoanRateStatus: types.InOutInput, LoanRate: 0.1715,
			PayAmtStatus: types.StatusEmpty,
			NStatus:      types.InOutInput, NPeriods: np,
			PerYrStatus: types.InOutInput, PerYr: 1,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
			FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2025, time.January, 1),
		}, Settings: s, Fancy: false}
	}
	cases := []struct {
		np   int
		want float64
	}{{30, 48163.93}, {60, 101477.91}, {90, 153768.72}}
	for _, c := range cases {
		res := Amortize(mk(c.np))
		if res.Err != nil || math.Abs(res.TotalInt-c.want) > 0.05 {
			t.Errorf("%dyr in-advance annual: int=%.2f err=%v, want %.2f (oracle)", c.np, res.TotalInt, res.Err, c.want)
		}
	}
}

// TestPass4FancyInAdvancePayoffSkip guards P4-N2: the in-advance payoff walk
// (inAdvancePayoffBalance) now models SKIP months (in addition to moratorium and
// target). DOS's balance_calc RepayFancyLoan walk zeroes payamt for a skipped
// month FIRST (ComputeNext AMORTOP.pas:599), before the moratorium/target arm.
// The walk previously omitted skip, so a fancy in-advance loan with skip months
// fell back to the display-row rebate, which was ~10% off. Balloons / prepayments
// / adjustments are still not modelled by the walk (the caller gates them out and
// keeps the display-row fallback for those).
//
//	amort_oracle 250000 0.1616 48 24 skip=4-6 b365 inadv payhard=2188.33 payoff=15.6.2025
//	→ 257200.5866 (the display-row fallback gave 230541.0151)
//	amort_oracle 100000 0.10 36 12 inadv skip=4-6 mor=3 payhard=4000 payoff=15.9.2024
//	→ 92591.6942 (skip × moratorium × in-advance)
func TestPass4FancyInAdvancePayoffSkip(t *testing.T) {
	ms, _ := MonthSetFromString("4-6")
	// Case 1: skip × in-advance × 365-basis × semimonthly (peryr=24 ⇒ first date
	// is the loan date, matching the oracle default).
	c1 := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: 250000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.1616,
		PayAmtStatus: types.InOutInput, PayAmt: 2188.33,
		NStatus:      types.InOutInput, NPeriods: 48,
		PerYrStatus: types.InOutInput, PerYr: 24,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.January, 1),
	}, Settings: Settings{Basis: types.Basis365, PerYr: 24, YrDays: 365.25, YrInv: 1.0 / 365.25, InAdvance: true},
		SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "4-6", MonthSet: ms}}
	got, err := PayoffBalance(c1, types.NewDateRec(2025, time.June, 15))
	if err != nil || math.Abs(got-257200.5866) > 0.005 {
		t.Errorf("skip×inadv×b365 payoff = %.4f err=%v, want 257200.5866 (oracle; the display-row fallback gave 230541.0151)", got, err)
	}
	// Case 2: skip × moratorium × in-advance (360 basis, monthly).
	c2 := LoanInput{Loan: Loan{
		AmountStatus: types.InOutInput, Amount: 100000,
		LoanRateStatus: types.InOutInput, LoanRate: 0.10,
		PayAmtStatus: types.InOutInput, PayAmt: 4000,
		NStatus:      types.InOutInput, NPeriods: 36,
		PerYrStatus: types.InOutInput, PerYr: 12,
		LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, time.January, 1),
		FirstStatus: types.InOutInput, FirstDate: types.NewDateRec(2024, time.February, 1),
	}, Settings: Settings{Basis: types.Basis360, PerYr: 12, YrDays: 360, YrInv: 1.0 / 360, InAdvance: true},
		Moratorium: Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: types.NewDateRec(2024, time.April, 1)},
		SkipMonths: SkipMonths{SkipStatus: types.InOutInput, SkipStr: "4-6", MonthSet: ms}}
	got2, err2 := PayoffBalance(c2, types.NewDateRec(2024, time.September, 15))
	if err2 != nil || math.Abs(got2-92591.6942) > 0.005 {
		t.Errorf("skip×mor×inadv payoff = %.4f err=%v, want 92591.6942 (oracle)", got2, err2)
	}
}
