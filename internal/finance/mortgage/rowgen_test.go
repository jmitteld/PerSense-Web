package mortgage

import (
	"math"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// baseMortgageWithComputedMonthly returns a row where price, rate,
// years, and percent-down are inputs and monthly has been computed by
// Calc — the typical state for row generation.
func baseMortgageWithComputedMonthly(t *testing.T) MtgLine {
	t.Helper()
	m := MtgLine{
		PriceStatus:  types.InOutInput,
		Price:        300000,
		PointsStatus: types.InOutInput,
		Points:       0,
		PctStatus:    types.InOutInput,
		Pct:          0.20,
		YearsStatus:  types.InOutInput,
		Years:        30,
		RateStatus:   types.InOutInput,
		Rate:         0.06,
	}
	res := Calc(m)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if res.Line.MonthlyStatus != types.InOutOutput {
		t.Fatalf("expected monthly to be computed; got status %d",
			res.Line.MonthlyStatus)
	}
	return res.Line
}

func TestEnoughDataForRowGeneration(t *testing.T) {
	m := baseMortgageWithComputedMonthly(t)
	if !EnoughDataForRowGeneration(&m) {
		t.Error("expected EnoughDataForRowGeneration = true when monthly is output")
	}

	// Empty row: not enough data.
	var empty MtgLine
	if EnoughDataForRowGeneration(&empty) {
		t.Error("empty row should return false")
	}
}

func TestGenerateRowsByRate(t *testing.T) {
	base := baseMortgageWithComputedMonthly(t)
	baseMonthly := base.Monthly

	rows, err := GenerateRows(base, VaryRate, 0.0025, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(rows))
	}

	// First row should match base (no increment applied).
	if rows[0].Rate != base.Rate {
		t.Errorf("row 0 rate = %.4f, want %.4f", rows[0].Rate, base.Rate)
	}
	// Each subsequent row's rate is +0.0025.
	for i := 1; i < 5; i++ {
		want := base.Rate + float64(i)*0.0025
		if rows[i].Rate < want-1e-9 || rows[i].Rate > want+1e-9 {
			t.Errorf("row %d rate = %.4f, want %.4f", i, rows[i].Rate, want)
		}
		// Higher rate should give higher monthly payment.
		if rows[i].Monthly <= baseMonthly {
			t.Errorf("row %d monthly = %.2f should exceed base %.2f",
				i, rows[i].Monthly, baseMonthly)
		}
	}
}

func TestGenerateRowsZeroCountErrors(t *testing.T) {
	base := baseMortgageWithComputedMonthly(t)
	if _, err := GenerateRows(base, VaryRate, 0.0025, 0); err == nil {
		t.Error("expected error for 0 rows")
	}
}

func TestGenerateRowsVaryNoneErrors(t *testing.T) {
	base := baseMortgageWithComputedMonthly(t)
	if _, err := GenerateRows(base, VaryNone, 0.0025, 3); err == nil {
		t.Error("expected error for VaryNone")
	}
}

func TestGenerateRowsSingleRow(t *testing.T) {
	base := baseMortgageWithComputedMonthly(t)
	rows, err := GenerateRows(base, VaryRate, 0.0025, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Errorf("got %d rows, want 1", len(rows))
	}
	// n=1 should match base exactly (no increment applied).
	if rows[0].Rate != base.Rate {
		t.Errorf("n=1 rate = %.4f, want base %.4f", rows[0].Rate, base.Rate)
	}
}

func TestGenerateRowsZeroIncrement(t *testing.T) {
	base := baseMortgageWithComputedMonthly(t)
	rows, err := GenerateRows(base, VaryRate, 0.0, 3)
	if err != nil {
		t.Fatal(err)
	}
	// All three rows should have identical rates (and therefore
	// identical monthly payments).
	for i := 1; i < len(rows); i++ {
		if rows[i].Rate != rows[0].Rate {
			t.Errorf("zero-increment row %d rate differs: %f vs %f",
				i, rows[i].Rate, rows[0].Rate)
		}
		if math.Abs(rows[i].Monthly-rows[0].Monthly) > 0.01 {
			t.Errorf("zero-increment row %d monthly differs: %.2f vs %.2f",
				i, rows[i].Monthly, rows[0].Monthly)
		}
	}
}

func TestGenerateRowsNegativeIncrementRate(t *testing.T) {
	base := baseMortgageWithComputedMonthly(t)
	// Rate decreasing by 1% each row.
	rows, err := GenerateRows(base, VaryRate, -0.01, 4)
	if err != nil {
		t.Fatal(err)
	}
	// Row 0 = 6%, row 1 = 5%, row 2 = 4%, row 3 = 3%.
	for i := range rows {
		want := base.Rate + float64(i)*-0.01
		if math.Abs(rows[i].Rate-want) > 1e-9 {
			t.Errorf("row %d rate = %.4f, want %.4f", i, rows[i].Rate, want)
		}
		// Lower rates should yield strictly lower monthly payments.
		if i > 0 && rows[i].Monthly >= rows[i-1].Monthly {
			t.Errorf("row %d monthly %.2f should be lower than row %d monthly %.2f",
				i, rows[i].Monthly, i-1, rows[i-1].Monthly)
		}
	}
}

func TestGenerateRowsByYears(t *testing.T) {
	base := baseMortgageWithComputedMonthly(t)
	rows, err := GenerateRows(base, VaryYears, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Years != base.Years {
		t.Errorf("row 0 years = %d, want %d", rows[0].Years, base.Years)
	}
	if rows[1].Years != base.Years+5 {
		t.Errorf("row 1 years = %d, want %d", rows[1].Years, base.Years+5)
	}
	if rows[2].Years != base.Years+10 {
		t.Errorf("row 2 years = %d, want %d", rows[2].Years, base.Years+10)
	}
	// Longer term should reduce monthly payment.
	if rows[2].Monthly >= rows[0].Monthly {
		t.Errorf("longer term should reduce monthly; got %.2f >= %.2f",
			rows[2].Monthly, rows[0].Monthly)
	}
}
