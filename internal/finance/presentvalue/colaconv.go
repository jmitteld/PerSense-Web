package presentvalue

import (
	"github.com/persense/persense-port/internal/finance/interest"
)

// colaContinuous converts an entered COLA *yield* into the continuous-rate form
// the PV formulas consume, using DOS's own conversion.
//
// DOS stores `periodic.cola` already-continuous — PRESVALU.pas:281 applies
// `exp_cola := exxp(cola)` with no conversion — and re-renders the cell with
// `PercentValueFromCell`'s COLAcol arm, `YieldFromRate(rp^, 1)`
// (INTSUTIL.pas:1601-1606). Since `YieldFromRate(rr,1) = 1*(exxp(rr/1)-1)`
// (INTSUTIL.pas:1263-1268), the stored value's unique inverse is
// `RateFromYield(yield, 1) = 1*lnn(1 + yield/1)` (INTSUTIL.pas:1270-1275) — the
// n=1 form, NOT peryr*lnn(1+y/peryr).
//
// The port applies the conversion at point of use rather than at cell entry
// (the DOS TUI's keystroke unit, INPUT.pas/PEPANE.pas, is not in the surviving
// checkout), so it must use the same arithmetic to land on the same double.
// `math.Log1p(y)` — what these sites used before — is a *different* function
// from `lnn(1+y)`: log1p never forms `1+y`, so it does not incur that rounding,
// and it disagrees with DOS in the last bits on roughly a third of non-zero
// COLA inputs. See docs/discrepancies.md §48.
func colaContinuous(cola float64, yrDays float64) (float64, error) {
	if cola == 0 {
		return 0, nil
	}
	return interest.RateFromYield(cola, 1, yrDays)
}
