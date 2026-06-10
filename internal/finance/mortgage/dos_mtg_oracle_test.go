package mortgage

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

func mtgOracleBin() string {
	if p := os.Getenv("PERSENSE_MTG_ORACLE"); p != "" {
		return p
	}
	return "/tmp/oraclebuild/mtg_oracle"
}

// parseMtg pulls "monthly <m> price <p> cash <c> financed <f>".
func parseMtg(out []byte) (monthly, price, cash, financed float64, ok bool) {
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) < 8 || f[0] != "monthly" {
		return 0, 0, 0, 0, false
	}
	monthly, _ = strconv.ParseFloat(f[1], 64)
	price, _ = strconv.ParseFloat(f[3], 64)
	cash, _ = strconv.ParseFloat(f[5], 64)
	financed, _ = strconv.ParseFloat(f[7], 64)
	return monthly, price, cash, financed, true
}

func ff(x float64) string { return strconv.FormatFloat(x, 'f', 10, 64) }

// TestDOSMtgOracleSweep validates the mortgage Calc against the real DOS
// Mortgage engine: solve-monthly (with points and balloons) and solve-price.
// The true (continuous) rate is fed to both sides to avoid the APR->true-rate
// conversion as a confound. Skips when the oracle binary is absent; build via
// TARGET=mtg_oracle legacy/oracle/build_linux.sh
func TestDOSMtgOracleSweep(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s); build via TARGET=mtg_oracle legacy/oracle/build_linux.sh", bin)
	}
	rng := rand.New(rand.NewSource(20260614))

	// --- Solve monthly (price/pct/years/rate, optional points & balloon) ---
	mChecked, mFails, mMax := 0, 0, 0.0
	var worst string
	for i := 0; i < 500; i++ {
		price := float64(50000 + rng.Intn(950000))
		pct := 0.05 + rng.Float64()*0.45
		years := 5 + rng.Intn(35)
		rate := 0.01 + rng.Float64()*0.16 // true rate
		points := 0.0
		if rng.Intn(2) == 0 {
			points = rng.Float64() * 0.05
		}
		hasBalloon := rng.Intn(3) == 0
		bWhen, bHow := 0, 0.0
		args := []string{"monthly", ff(price), ff(pct), strconv.Itoa(years), ff(rate), ff(points)}
		if hasBalloon {
			bWhen = 1 + rng.Intn(years-1)
			bHow = price * (0.05 + rng.Float64()*0.3)
			args = append(args, strconv.Itoa(bWhen), ff(bHow))
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
			worst = "price=" + strconv.FormatFloat(price, 'f', 0, 64) + " pct=" + strconv.FormatFloat(pct, 'f', 3, 64) +
				" yrs=" + strconv.Itoa(years) + " r=" + strconv.FormatFloat(rate, 'f', 4, 64) + " bal=" + strconv.FormatBool(hasBalloon)
		}
		if rel > 1e-6 {
			mFails++
			if mFails <= 12 {
				t.Errorf("MONTHLY price=%.0f pct=%.3f yrs=%d r=%.4f pts=%.3f bal=%v: DOS=%.6f Go=%.6f (rel %.2e)",
					price, pct, years, rate, points, hasBalloon, om, r.Line.Monthly, rel)
			}
		}
	}
	t.Logf("solve-monthly: checked %d, divergences %d, max relErr=%.2e at [%s]", mChecked, mFails, mMax, worst)

	// --- Solve price (pct/years/rate/monthly) ---
	pChecked, pFails, pMax := 0, 0, 0.0
	for i := 0; i < 300; i++ {
		pct := 0.05 + rng.Float64()*0.45
		years := 5 + rng.Intn(35)
		rate := 0.01 + rng.Float64()*0.16
		monthly := float64(300 + rng.Intn(8000))
		points := 0.0
		if rng.Intn(2) == 0 {
			points = rng.Float64() * 0.05
		}
		out, err := exec.Command(bin, "price", ff(pct), strconv.Itoa(years), ff(rate), ff(monthly), ff(points)).Output()
		if err != nil {
			continue
		}
		_, op, _, _, ok := parseMtg(out)
		if !ok {
			continue
		}
		m := MtgLine{
			PctStatus: types.InOutInput, Pct: pct,
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			MonthlyStatus: types.InOutInput, Monthly: monthly,
			PointsStatus: types.InOutInput, Points: points,
			TaxStatus: types.InOutInput, Tax: 0,
			PriceStatus: types.StatusEmpty,
		}
		r := Calc(m)
		if r.Err != nil {
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
				t.Errorf("PRICE pct=%.3f yrs=%d r=%.4f monthly=%.2f pts=%.3f: DOS=%.6f Go=%.6f (rel %.2e)",
					pct, years, rate, monthly, points, op, r.Line.Price, rel)
			}
		}
	}
	t.Logf("solve-price: checked %d, divergences %d, max relErr=%.2e", pChecked, pFails, pMax)
}

// runMtgAPROracle computes a mortgage's full-term APR via the real DOS
// ReportAPR/IterateToFindAPR. Returns the APR as a fraction.
func runMtgAPROracle(price, pct float64, years int, rate, points float64) (float64, bool) {
	out, err := exec.Command(mtgOracleBin(), "apr",
		ff(price), ff(pct), strconv.Itoa(years), ff(rate), ff(points)).Output()
	if err != nil {
		return 0, false
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) < 2 || f[0] != "apr" {
		return 0, false
	}
	v, e := strconv.ParseFloat(f[1], 64)
	return v, e == nil
}

// TestDOSMtgAPRSweep validates the full-term APR computation (points included)
// against the real DOS IterateToFindAPR over randomized mortgages.
func TestDOSMtgAPRSweep(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s); build via TARGET=mtg_oracle legacy/oracle/build_linux.sh", bin)
	}
	rng := rand.New(rand.NewSource(20260620))
	checked, fails, maxErr := 0, 0, 0.0
	var worst string
	for i := 0; i < 400; i++ {
		price := float64(50000 + rng.Intn(950000))
		pct := 0.05 + rng.Float64()*0.45
		years := 5 + rng.Intn(35)
		rate := 0.02 + rng.Float64()*0.14
		points := 0.0
		if rng.Intn(2) == 0 {
			points = rng.Float64() * 0.05
		}
		op, ok := runMtgAPROracle(price, pct, years, rate, points)
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
		r := Calc(m)
		if r.Err != nil {
			continue
		}
		gp, conv, err := FullTermAPR(r.Line, 360)
		if err != nil || !conv {
			continue
		}
		checked++
		// DOS reports 100*apr to 4 decimals -> ~1e-6 fraction display rounding.
		e := math.Abs(op - gp)
		if e > maxErr {
			maxErr = e
			worst = "price=" + strconv.FormatFloat(price, 'f', 0, 64) + " pts=" + strconv.FormatFloat(points, 'f', 4, 64)
		}
		if e > 1e-5 {
			fails++
			if fails <= 12 {
				t.Errorf("APR price=%.0f pct=%.3f yrs=%d r=%.4f pts=%.4f: DOS=%.6f Go=%.6f (|e|=%.2e)",
					price, pct, years, rate, points, op, gp, e)
			}
		}
	}
	t.Logf("APR sweep: checked %d, divergences %d, max |err|=%.2e at [%s]", checked, fails, maxErr, worst)
}

// TestDOSMtgDownPaymentDispatch validates the cash<->percent<->financed
// down-payment dispatch (Mortgage.pas ComputeCashPctAndFinanced): solve the
// monthly payment when the down payment is given as cash required or as the
// amount financed (rather than percent down), against the real DOS engine.
func TestDOSMtgDownPaymentDispatch(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s)", bin)
	}
	rng := rand.New(rand.NewSource(20260621))
	for _, kind := range []string{"mcash", "mfin"} {
		checked, fails, maxRel := 0, 0, 0.0
		for i := 0; i < 250; i++ {
			price := float64(50000 + rng.Intn(950000))
			pct := 0.05 + rng.Float64()*0.45
			years := 5 + rng.Intn(35)
			rate := 0.02 + rng.Float64()*0.14
			points := 0.0
			if rng.Intn(2) == 0 {
				points = rng.Float64() * 0.04
			}
			financed := price * (1 - pct)
			cash := price * (pct + (1-pct)*points)
			var downVal float64
			m := MtgLine{
				PriceStatus: types.InOutInput, Price: price,
				YearsStatus: types.InOutInput, Years: years,
				RateStatus: types.InOutInput, Rate: rate,
				PointsStatus: types.InOutInput, Points: points,
				TaxStatus: types.InOutInput, Tax: 0,
				MonthlyStatus: types.StatusEmpty,
			}
			if kind == "mcash" {
				downVal = cash
				m.CashStatus = types.InOutInput
				m.Cash = cash
			} else {
				downVal = financed
				m.FinancedStatus = types.InOutInput
				m.Financed = financed
			}
			out, err := exec.Command(bin, kind, ff(price), ff(downVal), strconv.Itoa(years), ff(rate), ff(points)).Output()
			if err != nil {
				continue
			}
			om, _, _, of, ok := parseMtg(out)
			if !ok {
				continue
			}
			r := Calc(m)
			if r.Err != nil {
				continue
			}
			checked++
			rm := math.Abs(om-r.Line.Monthly) / math.Max(1, r.Line.Monthly)
			rf := math.Abs(of-r.Line.Financed) / math.Max(1, r.Line.Financed)
			if rm > maxRel {
				maxRel = rm
			}
			if rf > maxRel {
				maxRel = rf
			}
			if rm > 1e-6 || rf > 1e-6 {
				fails++
				if fails <= 8 {
					t.Errorf("[%s] price=%.0f down=%.0f yrs=%d r=%.4f: monthly DOS=%.4f Go=%.4f | fin DOS=%.2f Go=%.2f",
						kind, price, downVal, years, rate, om, r.Line.Monthly, of, r.Line.Financed)
				}
			}
		}
		t.Logf("[%s] dispatch: checked %d, divergences %d, max relErr=%.2e", kind, checked, fails, maxRel)
	}
}
