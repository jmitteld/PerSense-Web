package presentvalue

import (
	"math"

	"github.com/persense/persense-port/internal/types"
)

// appendResultAdvisories runs the present-value result-sanity pass
// (docs/result_warning_layer_spec.md, P-W3 / P-W4 / P-W7). It inspects the
// solved fields and appends non-blocking advisories. It never changes a
// computed number.
//
// P-W6 (over-specified row) is already emitted by FirstPass. P-W1 (no-sign-
// change IRR) and P-W2 (rate non-convergence) surface as hard errors from
// the rate solver, so they are not duplicated here. The actuarial L-W
// predicates need survival-probability internals and are deferred.
//
// ⚠️ ROUND 42, §76 — this comment used to end "The variable-rate and POD
// early-return paths in Calculate bypass this pass." Both halves were
// load-bearing and one of them was already wrong:
//   - VARIABLE RATE: true, and it was a DEFECT, not a design note. A rate
//     schedule silenced every advisory the same worksheet raises at a
//     fixed rate. Fixed in calc.go's VR branch; the pass runs there now.
//   - POD: never true. solveUnknownPOD returns the result of a nested
//     Calculate, which has already been through this pass.
//
// P-W6 is still NOT emitted under a rate schedule, because FirstPass is
// deliberately skipped in VR mode — filed as §80, together with the DOS
// screen-total over-determination warning (PRESVALU.pas:1160-1167) that
// NEITHER arm reproduces.
func appendResultAdvisories(result *PVResult, input *PVInput) {
	if result.Err != nil {
		return
	}
	rateSolved := input.PresVal.R.Status < types.InOutDefault
	asOfSolved := input.PresVal.AsOfStatus < types.InOutDefault

	// P-W3: the solved IRR is implausibly large.
	if rateSolved && result.Rate > 1.0 {
		result.add(types.NoteTier, "P-W3", []string{"rate"},
			"The solved rate is very high. That can be correct for a deep discount, but "+
				"double-check the dates and amounts.")
	}

	// P-W4: a row whose Amount was the blank (solved) field came out ~0.
	for i := range result.LumpSums {
		if i < len(input.LumpSums) &&
			input.LumpSums[i].AmtStatus < types.InOutDefault && // blank in input -> solved
			math.Abs(result.LumpSums[i].Amt) < 1.0 {
			result.add(types.AdvisoryTier, "P-W4", []string{"lumpAmount"},
				"The solved amount is essentially zero — the other rows already account for "+
					"the target value. Check whether this row is needed.")
		}
	}
	for i := range result.Periodics {
		if i < len(input.Periodics) &&
			input.Periodics[i].AmtStatus < types.InOutDefault && // blank in input -> solved
			math.Abs(result.Periodics[i].Amt) < 1.0 {
			result.add(types.AdvisoryTier, "P-W4", []string{"periodicAmount"},
				"The solved amount is essentially zero — the other rows already account for "+
					"the target value. Check whether this row is needed.")
		}
	}

	// P-W7: a forward value that nets to ~0 while non-zero payments exist.
	if !rateSolved && !asOfSolved {
		anyPayment := false
		for i := range result.LumpSums {
			if math.Abs(result.LumpSums[i].Amt) > 1 {
				anyPayment = true
			}
		}
		for i := range result.Periodics {
			if math.Abs(result.Periodics[i].Amt) > 1 {
				anyPayment = true
			}
		}
		if anyPayment && math.Abs(result.SumValue) < 1.0 {
			result.add(types.NoteTier, "P-W7", []string{"sumValue"},
				"The payments net to about zero at this rate — inflows and outflows cancel. "+
					"Verify the signs are what you intend.")
		}
	}
}

// add appends a formatted advisory to the result's Warnings channel.
func (r *PVResult) add(tier, code string, fields []string, msg string) {
	r.Warnings = append(r.Warnings, types.FormatAdvisory(tier, code, fields, msg))
}
