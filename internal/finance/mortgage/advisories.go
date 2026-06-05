package mortgage

import (
	"fmt"
	"math"
	"strings"

	"github.com/persense/persense-port/internal/types"
)

// appendResultAdvisories runs the result-sanity pass (docs/
// result_warning_layer_spec.md, Mortgage M-W1..M-W7) after a successful
// Calc. It inspects the solved cells and appends non-blocking advisories to
// result.Warnings. It never changes a computed number. M-W8 (non-converged
// APR) is emitted by the API handler, where the APR is actually computed.
func appendResultAdvisories(r *CalcResult) {
	if r.Err != nil {
		return
	}
	ei := &r.Line

	monthlyOut := ei.MonthlyStatus == types.InOutOutput
	priceOut := ei.PriceStatus == types.InOutOutput
	balloonOut := ei.BalloonStat == types.BalloonUnk && ei.HowMuchStatus == types.InOutOutput
	pctOut := ei.PctStatus == types.InOutOutput
	cashOut := ei.CashStatus == types.InOutOutput

	// Only inspect once a real solve has happened; a half-entered row that
	// Calc returns without error should stay quiet.
	if !(monthlyOut || priceOut || balloonOut || pctOut || cashOut) {
		return
	}

	pmt := ei.Monthly - ei.Tax // regular principal+interest portion
	onePmt := math.Max(10, math.Abs(pmt))

	// --- Balloon degeneracies (M-W1, M-W2, M-W3) ---
	if balloonOut {
		switch {
		case ei.HowMuch < -onePmt:
			// M-W2: solved balloon is meaningfully negative.
			r.add(types.AdvisoryTier, "M-W2", []string{"balloonAmount"}, fmt.Sprintf(
				"Balloon Amt came out negative — your Monthly Total more than pays the loan "+
					"off before year %d. Lower the Monthly Total, or remove the balloon.", ei.When))
		case math.Abs(ei.HowMuch) < onePmt:
			// M-W1: solved balloon is within rounding of zero.
			r.add(types.AdvisoryTier, "M-W1", []string{"balloonAmount", "monthly"},
				"Balloon Amt is essentially zero — the Monthly Total you supplied already pays "+
					"the loan off by the balloon date, so no balloon is needed. To size a real "+
					"balloon, enter a Monthly Total below the full payment.")
		}
		// M-W3: balloon exceeds the amount borrowed (payment below interest).
		if ei.Financed > 0 && ei.HowMuch >= ei.Financed {
			r.add(types.NoteTier, "M-W3", []string{"balloonAmount", "monthly"},
				"Balloon Amt is larger than the amount borrowed — the Monthly Total doesn't "+
					"cover interest, so the balance grows until the balloon (negative "+
					"amortization). Intended only if that's the structure you want.")
		}
	}

	// M-W4 (interest-only monthly) intentionally omitted on the Mortgage
	// screen: a *solved* Monthly Total is always the amortizing payment, and
	// a user-entered sub-amortizing payment is the legitimate balloon case
	// (it would false-positive on help Example 3). The interest-only advisory
	// lives on the Amortization screen as A-W6, which is its correct home.

	// --- Negative % Down (M-W5) — financed exceeds price ---
	if pctOut && ei.Pct < 0 && !hasAmountExceedsPriceWarning(r.Warnings) {
		r.add(types.AdvisoryTier, "M-W5", []string{"pctDown"},
			"Amount Borrowed exceeds Price, so % Down is negative. Check Price, Cash Required, "+
				"or Amt Borrowed — exactly one of the three should be your input.")
	}

	// --- Non-positive solved Price / negative Cash Required (M-W6) ---
	if priceOut && ei.Price <= 0 {
		r.add(types.AdvisoryTier, "M-W6", []string{"price"},
			"Price solved to a non-positive value, which isn't a real loan. Re-check the "+
				"inputs feeding it.")
	}
	if cashOut && ei.Cash < 0 {
		r.add(types.AdvisoryTier, "M-W6", []string{"cash"},
			"Cash Required solved to a negative value, which isn't a real loan. Re-check the "+
				"inputs feeding it.")
	}

	// --- Balloon at or after the final year (M-W7) ---
	if ei.BalloonStat != types.BalloonBlank && ei.WhenStatus == types.InOutInput &&
		ei.Years > 0 && ei.When >= ei.Years {
		r.add(types.AdvisoryTier, "M-W7", []string{"balloonYears"}, fmt.Sprintf(
			"Balloon Yrs is on or after the loan's final year, so the balloon never takes "+
				"effect. Set Balloon Yrs earlier than %d.", ei.Years))
	}
}

// add appends a formatted advisory to the result's Warnings channel.
func (r *CalcResult) add(tier, code string, fields []string, msg string) {
	r.Warnings = append(r.Warnings, types.FormatAdvisory(tier, code, fields, msg))
}

// hasAmountExceedsPriceWarning reports whether the DOS-faithful
// "amount borrowed exceeds price" warning is already present, so M-W5
// doesn't duplicate it.
func hasAmountExceedsPriceWarning(ws []string) bool {
	for _, w := range ws {
		if strings.Contains(w, "Amount borrowed exceeds price") {
			return true
		}
	}
	return false
}
