package mortgage

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// runMtgPrice drives the oracle's price-solve mode (monthly given, price out).
func runMtgPrice(pct float64, years int, rate, monthly, points float64) (float64, bool) {
	out, err := exec.Command(mtgOracleBin(), "price", ff(pct), strconv.Itoa(years), ff(rate), ff(monthly), ff(points)).Output()
	if err != nil {
		return 0, false
	}
	_, p, _, _, ok := parseMtg(out)
	return p, ok
}

// TestDOSMtgGenerateRowsMultiFieldSweep extends the What-If table's DOS
// validation beyond VaryRate (TestDOSMtgGenerateRowsSweep) to the other
// monthly-output vary fields: Years, Points, %Down, and Price. For each field,
// GenerateRows steps the column and re-solves the monthly per row; every
// generated row's (inputs → monthly) pair must reproduce the real DOS forward
// solve for that row. This closes the documented What-If gap that only VaryRate
// was DOS-grounded (docs/ui_engine_confidence.md §W).
//
// VaryMonthly is the one remaining vary field; it steps the payment and solves
// PRICE (not monthly), so it needs the price-solve oracle rather than the
// monthly oracle and is validated by TestDOSMtgGenerateRowsVaryMonthlySweep.
func TestDOSMtgGenerateRowsMultiFieldSweep(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s)", bin)
	}
	rng := rand.New(rand.NewSource(20260618))
	type vspec struct {
		name string
		vary VaryField
		inc  func(*rand.Rand) float64
	}
	specs := []vspec{
		{"years", VaryYears, func(r *rand.Rand) float64 { return float64(1 + r.Intn(3)) }},
		{"points", VaryPoints, func(r *rand.Rand) float64 { return 0.0025 + r.Float64()*0.005 }},
		{"pctDown", VaryPctDown, func(r *rand.Rand) float64 { return 0.01 + r.Float64()*0.03 }},
		{"price", VaryPrice, func(r *rand.Rand) float64 { return float64(5000 + r.Intn(20000)) }},
	}
	per := map[string]int{}
	checked, fails := 0, 0
	maxRel := 0.0
	for _, s := range specs {
		for i := 0; i < 60; i++ {
			price := float64(100000 + rng.Intn(400000))
			pct := 0.10 + rng.Float64()*0.25
			years := 12 + rng.Intn(20)
			rate := 0.04 + rng.Float64()*0.06
			points := rng.Float64() * 0.03
			base := MtgLine{
				PriceStatus: types.InOutInput, Price: price,
				PctStatus: types.InOutInput, Pct: pct,
				YearsStatus: types.InOutInput, Years: years,
				RateStatus: types.InOutInput, Rate: rate,
				PointsStatus: types.InOutInput, Points: points,
				TaxStatus: types.InOutInput, Tax: 0,
				MonthlyStatus: types.StatusEmpty, // monthly is the computed output
			}
			rb := Calc(base)
			if rb.Err != nil {
				continue
			}
			inc := s.inc(rng)
			rows, err := GenerateRows(rb.Line, s.vary, inc, 5)
			if err != nil {
				continue
			}
			for _, row := range rows {
				// Skip rows the stepping pushed out of a real-loan domain.
				if row.Pct >= 0.95 || row.Pct < 0 || row.Years <= 0 || row.Price <= 0 {
					continue
				}
				dosM, ok := runMtgMonthly(row.Price, row.Pct, row.Years, row.Rate, row.Points)
				if !ok {
					continue
				}
				checked++
				per[s.name]++
				rel := math.Abs(dosM-row.Monthly) / math.Max(1, math.Abs(row.Monthly))
				if rel > maxRel {
					maxRel = rel
				}
				if rel > 1e-5 {
					fails++
					if fails <= 10 {
						t.Errorf("[%s] price=%.0f pct=%.3f yrs=%d r=%.5f pts=%.4f: DOS=%.4f Go=%.4f (rel %.2e)",
							s.name, row.Price, row.Pct, row.Years, row.Rate, row.Points, dosM, row.Monthly, rel)
					}
				}
			}
		}
	}
	for _, s := range specs {
		if per[s.name] == 0 {
			t.Errorf("vary %s never exercised — oracle may be flaking", s.name)
		}
	}
	if checked < 200 {
		t.Fatalf("only %d rows checked — oracle may be flaking", checked)
	}
	t.Logf("What-If multi-field rows vs DOS: checked %d (years %d, points %d, pctDown %d, price %d), divergences %d, max relErr=%.2e",
		checked, per["years"], per["points"], per["pctDown"], per["price"], fails, maxRel)
}

// TestDOSMtgGenerateRowsVaryMonthlySweep covers the last vary field: VaryMonthly
// steps the payment and re-solves PRICE per row. Every generated row's price
// must reproduce the real DOS price-solve for that row's (pct, years, rate,
// monthly, points). Completes What-If DOS coverage across all six vary fields.
func TestDOSMtgGenerateRowsVaryMonthlySweep(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s)", bin)
	}
	rng := rand.New(rand.NewSource(20260619))
	checked, fails := 0, 0
	maxRel := 0.0
	for i := 0; i < 150; i++ {
		pct := 0.10 + rng.Float64()*0.25
		years := 12 + rng.Intn(20)
		rate := 0.04 + rng.Float64()*0.06
		monthly := float64(800 + rng.Intn(3000))
		points := rng.Float64() * 0.03
		base := MtgLine{
			PctStatus: types.InOutInput, Pct: pct,
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			MonthlyStatus: types.InOutInput, Monthly: monthly,
			PointsStatus: types.InOutInput, Points: points,
			TaxStatus: types.InOutInput, Tax: 0,
			PriceStatus: types.StatusEmpty, // price is the computed output
		}
		rb := Calc(base)
		if rb.Err != nil {
			continue
		}
		inc := float64(25 + rng.Intn(75)) // monthly-payment increment
		rows, err := GenerateRows(rb.Line, VaryMonthly, inc, 5)
		if err != nil {
			continue
		}
		for _, row := range rows {
			if row.Monthly <= 0 || row.Price <= 0 {
				continue
			}
			dosP, ok := runMtgPrice(row.Pct, row.Years, row.Rate, row.Monthly, row.Points)
			if !ok {
				continue
			}
			checked++
			rel := math.Abs(dosP-row.Price) / math.Max(1, math.Abs(row.Price))
			if rel > maxRel {
				maxRel = rel
			}
			if rel > 1e-5 {
				fails++
				if fails <= 10 {
					t.Errorf("[monthly] pct=%.3f yrs=%d r=%.5f monthly=%.2f pts=%.4f: DOS price=%.4f Go=%.4f (rel %.2e)",
						row.Pct, row.Years, row.Rate, row.Monthly, row.Points, dosP, row.Price, rel)
				}
			}
		}
	}
	if checked < 100 {
		t.Fatalf("only %d rows checked — oracle may be flaking", checked)
	}
	t.Logf("What-If VaryMonthly rows vs DOS (price solve): checked %d, divergences %d, max relErr=%.2e", checked, fails, maxRel)
}
