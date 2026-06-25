package amortization

import (
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// advisorySig reduces a Warnings slice to a comparable CATEGORY signature: coded
// advisories collapse to their code (A-W4/5/6/7/11/12), the deterministic string
// warnings to stable tokens. The A-W9 "implied terminating balloon of about X"
// message keeps no dollar amount — its estimate is `finalPayment − theRegularPayment`,
// and "the regular payment" is an engine-internal baseline that legitimately
// differs between the piecewise engine and the DOS-faithful port on a loan whose
// payment changes mid-schedule (ARM / moratorium). When dropAW9 is set (such a
// multi-segment loan) A-W9 is omitted entirely; the handcrafted parity cases pin
// A-W9's exact text on single-segment loans where the baseline is unambiguous.
func advisorySig(ws []string, dropAW9 bool) []string {
	var out []string
	for _, w := range ws {
		switch {
		case strings.HasPrefix(w, "@@ADV|"): // coded: @@ADV|tier|CODE|fields@@ msg
			parts := strings.SplitN(w, "|", 4)
			if len(parts) >= 3 {
				out = append(out, parts[2])
			}
		case strings.HasPrefix(w, "The regular payment does not amortize"):
			if !dropAW9 {
				out = append(out, "AW9")
			}
		case strings.HasPrefix(w, "Loan Rate of about"):
			out = append(out, "HIGH-RATE")
		default:
			out = append(out, w) // early-payoff: deterministic from row count
		}
	}
	sort.Strings(out)
	return out
}

// advisoryCase builds a fancy LoanInput in the port's domain so the same input
// can be fed to BOTH the production Amortize and the faithful port AmortizeDOS.
type advisoryCase struct {
	name   string
	amount float64
	rate   float64
	n      int
	perYr  int
	pay    float64 // 0 ⇒ solved (blank)
	apply  func(*LoanInput)
}

func (c advisoryCase) build() LoanInput {
	in := LoanInput{
		Loan: Loan{
			AmountStatus: types.InOutInput, Amount: c.amount,
			LoanRateStatus: types.InOutInput, LoanRate: c.rate,
			NStatus: types.InOutInput, NPeriods: c.n,
			PerYrStatus: types.InOutInput, PerYr: c.perYr,
			PayAmtStatus:   types.StatusEmpty,
			LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
			FirstStatus: types.InOutInput, FirstDate: firstPeriodDate(c.perYr),
		},
		Settings: Settings{Basis: types.Basis360, PerYr: byte(c.perYr),
			YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true},
		Fancy: true,
	}
	if c.pay > 0 {
		in.Loan.PayAmtStatus = types.InOutInput
		in.Loan.PayAmt = c.pay
	}
	if c.apply != nil {
		c.apply(&in)
	}
	return in
}

func advisoryCases() []advisoryCase {
	return []advisoryCase{
		{name: "plain-solved", amount: 100000, rate: 0.06, n: 12, perYr: 12},
		{name: "high-rate-solved", amount: 100000, rate: 0.25, n: 12, perYr: 12},
		{name: "high-rate-given", amount: 100000, rate: 0.25, n: 12, perYr: 12, pay: 9000},
		// Over-specified: a given payment too low to amortize ⇒ implied
		// terminating balloon (A-W9) and negative amortization (A-W6).
		{name: "underpay-balloon", amount: 100000, rate: 0.06, n: 12, perYr: 12, pay: 200},
		// Mild over-specification: payment a bit low ⇒ terminating balloon
		// without sustained neg-am.
		{name: "slight-underpay", amount: 100000, rate: 0.06, n: 12, perYr: 12, pay: 8000},
		// Balloon set but payment COMPUTED ⇒ A-W11 territory (does the solved
		// payment leave the balloon unused?).
		{name: "balloon-solved", amount: 100000, rate: 0.06, n: 12, perYr: 12,
			apply: func(in *LoanInput) { in.Balloons = []BalloonPayment{balloonAt(6, 30000)} }},
		// Balloon set WITH a user-entered payment ⇒ A-W11 must NOT fire.
		{name: "balloon-given", amount: 100000, rate: 0.06, n: 12, perYr: 12, pay: 8000,
			apply: func(in *LoanInput) { in.Balloons = []BalloonPayment{balloonAt(6, 30000)} }},
		// Plain rate-only ARM, payment solved ⇒ no advisory in either engine.
		{name: "arm-solved", amount: 100000, rate: 0.06, n: 24, perYr: 12,
			apply: func(in *LoanInput) { in.Adjustments = []RateAdjustment{adjRateAt(12, 0.08)} }},
	}
}

// TestDOSPortAdvisoryProbe prints both engines' advisories side by side so any
// divergence in the ported advisory layer is visible. Not a strict gate.
func TestDOSPortAdvisoryProbe(t *testing.T) {
	for _, c := range advisoryCases() {
		in := c.build()
		prod := Amortize(in)
		port := AmortizeDOS(in)
		t.Logf("CASE %s\n  PROD(%d): %s\n  PORT(%d): %s", c.name,
			len(prod.Warnings), strings.Join(prod.Warnings, " | "),
			len(port.Warnings), strings.Join(port.Warnings, " | "))
	}
}

// sameTerminal reports whether two schedules are effectively identical — same
// length and same per-row payment / interest / balance to the cent. The advisory
// predicates read the whole schedule (final-payment for A-W9, the entire balance
// trajectory for A-W6 negative amortization, the row count for early payoff), so
// they can only be expected to agree when the schedules agree ROW BY ROW. A
// terminal-only check is too weak: the piecewise engine and the DOS-faithful port
// can land on the same final row while differing mid-schedule on a multi-option
// stack (where the piecewise engine is the non-DOS one), and that legitimately
// changes a trajectory-sensitive advisory.
func sameTerminal(a, b AmortResult) bool {
	if len(a.Schedule) == 0 || len(a.Schedule) != len(b.Schedule) {
		return false
	}
	d := func(x, y float64) bool { return x-y < 0.01 && y-x < 0.01 }
	for i := range a.Schedule {
		ra, rb := a.Schedule[i], b.Schedule[i]
		if !d(ra.PayAmt, rb.PayAmt) || !d(ra.Interest, rb.Interest) || !d(ra.Principal, rb.Principal) {
			return false
		}
	}
	return true
}

// TestDOSPortAdvisoryParity asserts the faithful port emits EXACTLY the same
// advisories as the production engine wherever the two engines agree on the
// terminal schedule (Amortize is the reference for the Go-port advisory layer;
// the DOS oracle has no notion of these warnings). Where the schedules
// legitimately differ — e.g. a degenerate underpayment, for which the port is
// DOS-faithful and folds the whole residual into the final payment while
// production leaves a balance — the advisories may differ as a CONSEQUENCE of the
// schedule difference, not an advisory-layer bug; those are logged, not failed.
func TestDOSPortAdvisoryParity(t *testing.T) {
	for _, c := range advisoryCases() {
		in := c.build()
		prod := Amortize(in)
		port := AmortizeDOS(in)
		if prod.Err != nil || port.Err != nil {
			t.Fatalf("%s: unexpected error prod=%v port=%v", c.name, prod.Err, port.Err)
		}
		if !sameTerminal(prod, port) {
			t.Logf("%s: schedules differ at the terminal (port DOS-faithful) — "+
				"advisory comparison skipped\n  PROD: %v\n  PORT: %v",
				c.name, prod.Warnings, port.Warnings)
			continue
		}
		if !reflect.DeepEqual(prod.Warnings, port.Warnings) {
			t.Errorf("%s: advisory mismatch on identical schedule\n  PROD: %#v\n  PORT: %#v",
				c.name, prod.Warnings, port.Warnings)
		}
	}
}

// TestDOSPortAdvisoryParityFuzz drives the SAME randomized merged option cube the
// numeric fuzzer uses (genMergedCase) through BOTH engines and asserts identical
// advisories wherever they retire to the same terminal schedule. Pure Go-vs-Go
// (no oracle needed), so it runs in the default suite. Where the piecewise engine
// and the DOS-faithful port legitimately produce different terminal schedules
// (the known multi-option-stack divergences), the advisory comparison is skipped.
func TestDOSPortAdvisoryParityFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(424242))
	const nCases = 500
	compared, skipped := 0, 0
	for i := 0; i < nCases; i++ {
		c := genMergedCase(rng)
		in := LoanInput{
			Loan: Loan{
				AmountStatus: types.InOutInput, Amount: c.amount,
				LoanRateStatus: types.InOutInput, LoanRate: c.rate,
				NStatus: types.InOutInput, NPeriods: c.n,
				PerYrStatus: types.InOutInput, PerYr: c.perYr,
				PayAmtStatus:   types.StatusEmpty,
				LoanDateStatus: types.InOutInput, LoanDate: types.NewDateRec(2024, 1, 1),
				FirstStatus: types.InOutInput, FirstDate: monthsAfterLoanStart(c.firstMonths),
			},
			Settings: Settings{Basis: types.Basis360, PerYr: byte(c.perYr),
				YrDays: 360, YrInv: 1.0 / 360, PlusRegular: true, Prepaid: c.prepaid},
			Fancy: true,
		}
		c.apply(&in)
		prod := Amortize(in)
		port := AmortizeDOS(in)
		if prod.Err != nil || port.Err != nil || len(prod.Schedule) == 0 || len(port.Schedule) == 0 {
			continue
		}
		if !sameTerminal(prod, port) {
			skipped++
			continue
		}
		compared++
		// "the regular payment" is ambiguous on a multi-segment loan whose payment
		// changes mid-schedule (ARM / moratorium), so A-W9's baseline is dropped
		// from the category comparison for those; single-segment A-W9 text is pinned
		// by the handcrafted parity cases.
		dropAW9 := strings.Contains(c.key, "ARM") || strings.Contains(c.key, "mor")
		ps, qs := advisorySig(prod.Warnings, dropAW9), advisorySig(port.Warnings, dropAW9)
		if !reflect.DeepEqual(ps, qs) {
			t.Errorf("advisory category mismatch {%s} on identical schedule\n  flags: %v\n  PROD: %v\n  PORT: %v",
				c.key, c.flags, prod.Warnings, port.Warnings)
		}
	}
	t.Logf("advisory parity fuzz: compared=%d skipped(schedule differs)=%d", compared, skipped)
	if compared == 0 {
		t.Fatal("no cases compared — sameTerminal gate too strict or generation broken")
	}
}
