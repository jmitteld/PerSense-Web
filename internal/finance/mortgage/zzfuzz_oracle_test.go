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

// TestFuzzMortgageVsDOS aggressively fuzzes the mortgage Calc against the real
// DOS Mortgage engine across a wide parameter space — solve-monthly (with
// points and balloons) and solve-price — looking for any divergence beyond
// floating-point noise.
func TestFuzzMortgageVsDOS(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skip("mortgage oracle not present")
	}
	rng := rand.New(rand.NewSource(0x6d6f7274))

	// Per-section case count; override with PERSENSE_FUZZ_N (default preserves
	// the original fixed counts).
	nMonthly, nPrice := 6000, 4000
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			nMonthly, nPrice = v, v
		}
	}

	// --- Solve monthly ---
	mChecked, mFails, mMax := 0, 0, 0.0
	var worst string
	for i := 0; i < nMonthly; i++ {
		price := math.Round((10000+rng.Float64()*2_000_000)*100) / 100
		pct := 0.01 + rng.Float64()*0.98 // wide: near 0 and near 1
		years := 1 + rng.Intn(50)
		rate := 0.002 + rng.Float64()*0.45
		points := 0.0
		if rng.Intn(2) == 0 {
			points = rng.Float64() * 0.10
		}
		args := []string{"monthly", ff(price), ff(pct), strconv.Itoa(years), ff(rate), ff(points)}
		hasBalloon := rng.Intn(3) == 0
		bWhen, bHow := 0, 0.0
		if hasBalloon && years > 1 {
			bWhen = 1 + rng.Intn(years-1)
			bHow = price * (0.02 + rng.Float64()*0.4)
			args = append(args, strconv.Itoa(bWhen), ff(bHow))
		} else {
			hasBalloon = false
		}
		out, err := exec.Command(bin, args...).Output()
		if err != nil {
			continue
		}
		om, _, _, _, ok := parseMtg(out)
		if !ok {
			continue
		}
		m := MtgLine{
			PriceStatus: types.InOutInput, Price: price,
			PctStatus: types.InOutInput, Pct: pct,
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			PointsStatus: types.InOutInput, Points: points,
			TaxStatus: types.InOutInput, Tax: 0,
			MonthlyStatus: types.StatusEmpty,
		}
		if hasBalloon {
			m.WhenStatus = types.InOutInput
			m.When = bWhen
			m.HowMuchStatus = types.InOutInput
			m.HowMuch = bHow
		}
		r := Calc(m)
		if r.Err != nil {
			continue
		}
		mChecked++
		rel := math.Abs(om-r.Line.Monthly) / math.Max(1, math.Abs(r.Line.Monthly))
		if rel > mMax {
			mMax = rel
			worst = "price=" + ff(price) + " pct=" + strconv.FormatFloat(pct, 'f', 3, 64) +
				" yrs=" + strconv.Itoa(years) + " r=" + strconv.FormatFloat(rate, 'f', 4, 64) +
				" pts=" + strconv.FormatFloat(points, 'f', 3, 64) + " bal=" + strconv.FormatBool(hasBalloon)
		}
		if rel > 1e-6 {
			mFails++
			if mFails <= 12 {
				t.Errorf("MONTHLY price=%.2f pct=%.3f yrs=%d r=%.4f pts=%.3f bal=%v bWhen=%d bHow=%.2f: DOS=%.6f Go=%.6f (absDiff=%.6f rel %.2e)",
					price, pct, years, rate, points, hasBalloon, bWhen, bHow, om, r.Line.Monthly, math.Abs(om-r.Line.Monthly), rel)
			}
		}
	}
	t.Logf("solve-monthly: checked %d, divergences %d, maxRel=%.2e at [%s]", mChecked, mFails, mMax, worst)

	// --- Solve price (pct/years/rate/monthly) ---
	pChecked, pFails, pMax := 0, 0, 0.0
	for i := 0; i < nPrice; i++ {
		// Generate a self-consistent monthly by first solving forward, then
		// asking the engine to recover the price.
		pct := 0.05 + rng.Float64()*0.9
		years := 1 + rng.Intn(50)
		rate := 0.005 + rng.Float64()*0.4
		monthly := math.Round((100+rng.Float64()*40000)*100) / 100
		out, err := exec.Command(bin, "price", ff(pct), strconv.Itoa(years), ff(rate), ff(monthly), ff(0)).Output()
		if err != nil {
			continue
		}
		_, op, _, _, ok := parseMtg(out)
		if !ok || op <= 0 {
			continue
		}
		m := MtgLine{
			PctStatus: types.InOutInput, Pct: pct,
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			MonthlyStatus: types.InOutInput, Monthly: monthly,
			TaxStatus: types.InOutInput, Tax: 0, BalloonStat: types.BalloonBlank,
		}
		r := Calc(m)
		if r.Err != nil || r.Line.Price <= 0 {
			continue
		}
		pChecked++
		rel := math.Abs(op-r.Line.Price) / math.Max(1, math.Abs(r.Line.Price))
		if rel > pMax {
			pMax = rel
		}
		if rel > 1e-6 {
			pFails++
			if pFails <= 12 {
				t.Errorf("PRICE pct=%.3f yrs=%d r=%.4f mo=%.2f: DOS=%.4f Go=%.4f (rel %.2e)",
					pct, years, rate, monthly, op, r.Line.Price, rel)
			}
		}
	}
	t.Logf("solve-price: checked %d, divergences %d, maxRel=%.2e", pChecked, pFails, pMax)
}
