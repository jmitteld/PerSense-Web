package amortization

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// Standing differential tracker for ADVANCED-OPTION COMBINATIONS against the real
// DOS engine. The per-feature unit tests and the existing oracle "cubes" sweep
// single options and the core dispatch, but the cross-product of options
// (balloon × ARM, skip-includes-first-payment, …) was under-tested — which is how
// the divergences in docs/amort_option_combo_divergences.md and the post-mortem
// (docs/postmortem_arm_balloon_coverage.md) survived near-100% LINE coverage.
//
// This test pins that cross: for each combination it solves the payment through
// the PRODUCT path (Amortize) and through the DOS oracle, logs the divergence
// (so the current state is always visible with `-v`), and FAILS if the payment
// drifts beyond a cent-level tolerance or the schedule does not retire like DOS.
// Add a combination here whenever a new option pair becomes reachable.
//
// Runs only where the DOS oracle binary is present (PERSENSE_ORACLE or
// /tmp/oraclebuild/amort_oracle); skipped otherwise, like the other oracle tests.
//
// Balloon note: Go's default (balloonIncludesRegular=false ⇒ Settings.PlusRegular
// =true) ADDS the balloon to the regular payment; the oracle's default REPLACES,
// so balloon combos pass `plusreg` to the oracle to match.

func dateMonthsAfterLoan(months int) types.DateRec {
	base := months // loan month is 1 ⇒ (1-1)+months
	return types.NewDateRec(2024+base/12, time.Month(base%12+1), 1)
}

func balloonAt(months int, amt float64) BalloonPayment {
	return BalloonPayment{
		DateStatus: types.InOutInput, Date: dateMonthsAfterLoan(months),
		AmountStatus: types.InOutInput, Amount: amt}
}

func adjRateAt(months int, rate float64) RateAdjustment {
	return RateAdjustment{
		DateStatus: types.InOutInput, Date: dateMonthsAfterLoan(months),
		LoanRateStatus: types.InOutInput, LoanRate: rate}
}

func skipSet(t *testing.T, s string) SkipMonths {
	ms, err := MonthSetFromString(s)
	if err != nil {
		t.Fatalf("skip parse %q: %v", s, err)
	}
	return SkipMonths{SkipStatus: types.InOutInput, SkipStr: s, MonthSet: ms}
}

// runOracleInterestFlags runs the DOS oracle (quiet mode) with option flags and
// returns its total interest — the "interest" field of `payment P interest I
// paid T`. Total interest is the robust cross-option divergence signal: it is a
// single scalar defined for every option type (unlike "the regular payment,"
// which is ambiguous when an ARM or moratorium changes it mid-schedule).
func runOracleInterestFlags(amount, rate float64, n, perYr int, flags ...string) (float64, bool) {
	args := []string{strconv.FormatFloat(amount, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(n), strconv.Itoa(perYr)}
	args = append(args, flags...)
	for try := 0; try < 8; try++ {
		out, err := exec.Command(oracleBin, args...).Output()
		if err != nil {
			continue
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "interest" {
				// The oracle prints "interest -1.00" when its MakeTable output
				// is missing the totals line — an intermittent heap-flake under
				// rapid spawning (NumAfter returns -1). Retry until a real,
				// positive total appears, like runOraclePayment does for pay==0.
				if v, e := strconv.ParseFloat(f[i+1], 64); e == nil && v > 0 {
					return v, true
				}
			}
		}
	}
	return 0, false
}

// goAmortizeOptions runs the product Amortize path (payment blank ⇒ solved) with
// the given advanced options and returns the total interest and final balance.
func goAmortizeOptions(amount, rate float64, n, perYr int, apply func(*LoanInput)) (totalInt, finalBal float64, ok bool) {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: amount,
			LoanRateStatus: types.InOutInput, LoanRate: rate,
			NStatus: types.InOutInput, NPeriods: n,
			PerYrStatus: types.InOutInput, PerYr: perYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(perYr),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(perYr),
			YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy: true,
	}
	if apply != nil {
		apply(&in)
	}
	r := Amortize(in)
	if r.Err != nil || len(r.Schedule) == 0 {
		return 0, 0, false
	}
	return r.TotalInt, r.Schedule[len(r.Schedule)-1].Principal, true
}

func TestDOSOptionComboDivergenceTracker(t *testing.T) {
	if _, err := os.Stat(oracleBin); err != nil {
		t.Skipf("DOS oracle binary not present (%s)", oracleBin)
	}
	const (
		amount = 100000.0
		rate   = 0.08
		n      = 360
		perYr  = 12
	)

	// intTol: max |Go totalInterest − DOS totalInterest|. balTol: max |Go final
	// balance| (DOS retires to 0). The fixed combos agree to within a few dollars
	// (DOS-vs-port rounding tail). `known` documents a combo that still diverges
	// — it is TRACKED (logged, asserted only against its loose bound) rather than
	// failing CI, so the gap stays visible until fixed.
	combos := []struct {
		name   string
		apply  func(*LoanInput)
		flags  []string
		intTol float64
		balTol float64
		known  string
	}{
		{name: "single_balloon", apply: func(in *LoanInput) { in.Balloons = []BalloonPayment{balloonAt(120, 50000)} },
			flags: []string{"plusreg", "b120=50000"}, intTol: 10, balTol: 5},
		{name: "single_ARM_rate", apply: func(in *LoanInput) { in.Adjustments = []RateAdjustment{adjRateAt(48, 0.09)} },
			flags: []string{"adj=48:0.09:"}, intTol: 10, balTol: 5},
		{name: "balloon_plus_ARM", apply: func(in *LoanInput) {
			in.Balloons = []BalloonPayment{balloonAt(120, 40000)}
			in.Adjustments = []RateAdjustment{adjRateAt(48, 0.09)}
		}, flags: []string{"plusreg", "b120=40000", "adj=48:0.09:"}, intTol: 10, balTol: 5},
		{name: "two_balloons_plus_ARM", apply: func(in *LoanInput) {
			in.Balloons = []BalloonPayment{balloonAt(72, 15000), balloonAt(192, 15000)}
			in.Adjustments = []RateAdjustment{adjRateAt(132, 0.07)}
		}, flags: []string{"plusreg", "b72=15000", "b192=15000", "adj=132:0.07:"}, intTol: 10, balTol: 5},
		{name: "skip_excludes_first_pmt", apply: func(in *LoanInput) { in.SkipMonths = skipSet(t, "6-8") },
			flags: []string{"skip=6-8"}, intTol: 10, balTol: 5},
		{name: "skip_includes_first_pmt", apply: func(in *LoanInput) { in.SkipMonths = skipSet(t, "1-3,7") },
			flags: []string{"skip=1-3,7"}, intTol: 10, balTol: 5},
		// FIXED 2026-06-24: the moratorium re-amortization now solves a
		// balloon-aware payment (solveMoratoriumPayment), matching DOS's single
		// schedule-retiring payment. Was Go $102,258 vs DOS $157,192 (Δ -$55k);
		// now agrees to the cent. Tight guard — see
		// docs/amort_option_combo_divergences.md §3.
		{name: "moratorium_plus_balloon", apply: func(in *LoanInput) {
			in.Moratorium = Moratorium{FirstRepayStatus: types.InOutInput, FirstRepay: dateMonthsAfterLoan(24)}
			in.Balloons = []BalloonPayment{balloonAt(120, 30000)}
		}, flags: []string{"plusreg", "mor=24", "b120=30000"}, intTol: 10, balTol: 5},
	}

	for _, c := range combos {
		c := c
		t.Run(c.name, func(t *testing.T) {
			goInt, goBal, gok := goAmortizeOptions(amount, rate, n, perYr, c.apply)
			if !gok {
				t.Fatalf("Go produced no usable schedule")
			}
			dosInt, dok := runOracleInterestFlags(amount, rate, n, perYr, c.flags...)
			if !dok {
				t.Skip("oracle returned no result for this combo")
			}
			dosBal := 0.0
			if dosPay, pok := runOraclePayment(amount, rate, n, perYr, c.flags...); pok {
				if rows, rok := runOracleRowsFlags(amount, rate, n, perYr, dosPay, c.flags...); rok && len(rows) > 0 {
					dosBal = rows[len(rows)-1].balance
				}
			}

			// TRACK: always log the current divergence so `-v` shows the state.
			tag := ""
			if c.known != "" {
				tag = "  [KNOWN-OPEN: " + c.known + "]"
			}
			t.Logf("totalInterest Go %.2f / DOS %.2f (Δ %.2f) | finalBal Go %.2f / DOS %.2f%s",
				goInt, dosInt, goInt-dosInt, goBal, dosBal, tag)

			if c.known != "" {
				// Tracked-but-not-yet-fixed: surfaced (logged above) so the gap
				// stays visible, but it does not fail CI. When the combo is
				// fixed, drop `known` and it becomes a tight regression guard.
				return
			}
			if d := goInt - dosInt; d < -c.intTol || d > c.intTol {
				t.Errorf("total-interest divergence: Go %.2f vs DOS %.2f (Δ %.2f) exceeds ±%.2f",
					goInt, dosInt, goInt-dosInt, c.intTol)
			}
			if goBal < -c.balTol || goBal > c.balTol {
				t.Errorf("schedule did not retire like DOS: Go final balance %.2f exceeds ±%.2f "+
					"(DOS final %.2f)", goBal, c.balTol, dosBal)
			}
		})
	}
}
