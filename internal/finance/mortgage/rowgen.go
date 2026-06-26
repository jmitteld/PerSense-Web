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
	VaryNone    VaryField = iota
	VaryRate              // increment: rate (e.g. +0.0025 per row)
	VaryYears             // increment: years
	VaryPoints            // increment: points
	VaryPctDown           // increment: percent down
	VaryPrice             // increment: purchase price
	VaryMonthly           // increment: monthly payment
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
// MaxWhatIfRows bounds how many rows a What-If table may generate, guarding
// the API boundary against an adversarial or fat-fingered row count that would
// otherwise allocate unbounded memory and run Calc that many times. A 1,000-row
// table is already far larger than any practical comparison.
const MaxWhatIfRows = 1000

func GenerateRows(base MtgLine, vary VaryField, inc float64, n int) ([]MtgLine, error) {
	if n <= 0 {
		return nil, fmt.Errorf("row count must be positive, got %d", n)
	}
	if n > MaxWhatIfRows {
		return nil, fmt.Errorf("row count %d is too large — the maximum is %d", n, MaxWhatIfRows)
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
				"The What-If table needs a base mortgage that computes one of " +
					"Price, Monthly Total or Balloon. Leave one of those three " +
					"fields blank on the mortgage so there is a result to vary.")
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
		// DOS CopyAndIncrement (Mortgage.pas:992-1002) steps the rate
		// column in YIELD (loan-rate) space, not in continuous
		// true-rate space: convert the true rate to a loan rate, add
		// the increment, convert back. A "+0.5%" step is then 0.5% of
		// loan rate, matching the DOS what-if table. (A zero increment
		// is left untouched to avoid a needless conversion round-trip.)
		if inc != 0 {
			m.Rate = LoanRateToTrueRate(TrueRateToLoanRate(m.Rate) + inc)
		}
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

// GenerateGrid produces a 2-D what-if grid: it steps the secondary
// field (vary2) across count2 rows and, for each setting, generates a
// full primary-field (vary1) row series via GenerateRows. The result
// is grid[j][i] = the cell at secondary step j, primary step i.
//
// A vary2 of VaryNone collapses to a plain 1-D series (one inner
// slice). Mirrors the nested column iteration of DOS
// CopyAndIncrement, which recurses over up to three varied columns.
//
// Ported from legacy/src/dos_source/MortgageRowGenerationDlgUnit +
// Mortgage.pas CopyAndIncrement.
func GenerateGrid(base MtgLine, vary1 VaryField, inc1 float64, count1 int,
	vary2 VaryField, inc2 float64, count2 int) ([][]MtgLine, error) {

	if vary2 == VaryNone || count2 <= 0 {
		rows, err := GenerateRows(base, vary1, inc1, count1)
		if err != nil {
			return nil, err
		}
		return [][]MtgLine{rows}, nil
	}

	// Bound the secondary axis too, so the total cell count (count1 × count2)
	// can't balloon. GenerateRows guards count1 per band below.
	if count2 > MaxWhatIfRows {
		return nil, fmt.Errorf("row count %d is too large — the maximum is %d", count2, MaxWhatIfRows)
	}

	grid := make([][]MtgLine, 0, count2)
	for j := 0; j < count2; j++ {
		// Step the secondary field by inc2*j in one shot (matching
		// DOS basex + count*increment) and re-Calc so GenerateRows
		// sees a fully-computed base.
		b := base
		if j > 0 {
			if err := bumpField(&b, vary2, inc2*float64(j)); err != nil {
				return nil, err
			}
		}
		res := Calc(b)
		if res.Err != nil {
			return nil, res.Err
		}
		rows, err := GenerateRows(res.Line, vary1, inc1, count1)
		if err != nil {
			return nil, err
		}
		grid = append(grid, rows)
	}
	return grid, nil
}
