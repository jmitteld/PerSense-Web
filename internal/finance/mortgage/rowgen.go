// Mortgage row generation: produce N rows from a base row by varying
// one field by a fixed increment. The DOS application offers this via
// the MortgageRowGenerationDlgUnit dialog; here we expose it as a
// pure data transformation suitable for an HTTP endpoint.
//
// Ported from legacy/src/dos_source/Mortgage.pas: function
// EnoughDataForRowGeneration, plus the row-iteration logic in
// MortgageRowGenerationDlgUnit.

package mortgage

import (
	"fmt"

	"github.com/persense/persense-port/internal/types"
)

// VaryField identifies which input field is varied across generated
// rows. Mirrors the dropdown choices in the DOS row-generation dialog.
type VaryField int

const (
	VaryNone     VaryField = iota
	VaryRate               // increment: rate (e.g. +0.0025 per row)
	VaryYears              // increment: years
	VaryPoints             // increment: points
	VaryPctDown            // increment: percent down
	VaryPrice              // increment: purchase price
	VaryMonthly            // increment: monthly payment
)

// EnoughDataForRowGeneration returns true when the base row has at
// least one of price, monthly, or balloon amount as a computed (output)
// value. DOS guard at Mortgage.pas:839-841.
//
// Ported from legacy/src/dos_source/Mortgage.pas: function
// EnoughDataForRowGeneration.
func EnoughDataForRowGeneration(m *MtgLine) bool {
	return m.PriceStatus == types.InOutOutput ||
		m.MonthlyStatus == types.InOutOutput ||
		m.HowMuchStatus == types.InOutOutput
}

// GenerateRows produces n rows starting from base, incrementing the
// chosen field by `inc` between each row, then runs Calc on each.
//
// The varied field is treated as InOutInput on every generated row so
// the row's other dependent fields are recomputed.
//
// Ported from MortgageRowGenerationDlgUnit + the Calc loop that DOS
// runs over the generated lines.
func GenerateRows(base MtgLine, vary VaryField, inc float64, n int) ([]MtgLine, error) {
	if n <= 0 {
		return nil, fmt.Errorf("row count must be positive, got %d", n)
	}
	if vary == VaryNone {
		return nil, fmt.Errorf("must specify which field to vary")
	}

	rows := make([]MtgLine, 0, n)
	current := base
	for i := 0; i < n; i++ {
		// Apply increment after the first row (first row is base).
		if i > 0 {
			if err := bumpField(&current, vary, inc); err != nil {
				return nil, err
			}
		}
		// To re-compute the dependent field (price OR monthly OR balloon),
		// blank out exactly one of those so Calc has something to solve
		// for. We follow EnoughDataForRowGeneration's intent: whichever
		// of {Price, Monthly, HowMuch} was the OUTPUT in the base row is
		// the one we re-solve for in each generated row.
		row := current
		switch {
		case base.PriceStatus == types.InOutOutput:
			row.PriceStatus = types.StatusEmpty
			row.Price = 0
		case base.MonthlyStatus == types.InOutOutput:
			row.MonthlyStatus = types.StatusEmpty
			row.Monthly = 0
		case base.HowMuchStatus == types.InOutOutput:
			row.HowMuchStatus = types.StatusEmpty
			row.HowMuch = 0
		default:
			return nil, fmt.Errorf(
				"base row has no computed price/monthly/balloon to re-solve; " +
					"call EnoughDataForRowGeneration first")
		}
		res := Calc(row)
		if res.Err != nil {
			return rows, res.Err
		}
		rows = append(rows, res.Line)
	}
	return rows, nil
}

// bumpField increments the requested field by inc on m, marking it
// as input so Calc recomputes the dependent field.
func bumpField(m *MtgLine, vary VaryField, inc float64) error {
	switch vary {
	case VaryRate:
		m.Rate += inc
		m.RateStatus = types.InOutInput
	case VaryYears:
		m.Years += int(inc)
		m.YearsStatus = types.InOutInput
	case VaryPoints:
		m.Points += inc
		m.PointsStatus = types.InOutInput
	case VaryPctDown:
		m.Pct += inc
		m.PctStatus = types.InOutInput
	case VaryPrice:
		m.Price += inc
		m.PriceStatus = types.InOutInput
	case VaryMonthly:
		m.Monthly += inc
		m.MonthlyStatus = types.InOutInput
	default:
		return fmt.Errorf("unknown vary field: %d", vary)
	}
	return nil
}
