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

// cmpResult holds the parsed output of the oracle's `compare` mode.
type cmpResult struct {
	crossover bool
	crossAPR  float64
	crossTime float64
	apr1      float64
	apr2      float64
}

// runMtgCompareOracle drives the real DOS ReportComparisonOfAPRs over two
// mortgages and returns the parsed crossover APR/time (or always-better).
func runMtgCompareOracle(p1, pc1 float64, y1 int, r1, pt1, p2, pc2 float64, y2 int, r2, pt2 float64) (cmpResult, bool) {
	out, err := exec.Command(mtgOracleBin(), "compare",
		ff(p1), ff(pc1), strconv.Itoa(y1), ff(r1), ff(pt1),
		ff(p2), ff(pc2), strconv.Itoa(y2), ff(r2), ff(pt2)).Output()
	if err != nil {
		return cmpResult{}, false
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) == 0 {
		return cmpResult{}, false
	}
	var res cmpResult
	atof := func(label string) float64 {
		for i := 0; i+1 < len(f); i++ {
			if f[i] == label {
				v, _ := strconv.ParseFloat(f[i+1], 64)
				return v
			}
		}
		return math.NaN()
	}
	switch f[0] {
	case "cross":
		res.crossover = true
		res.crossAPR = atof("cross")
		res.crossTime = atof("time")
		res.apr1 = atof("apr1")
		res.apr2 = atof("apr2")
		return res, true
	case "always":
		res.crossover = false
		res.apr1 = atof("apr1")
		res.apr2 = atof("apr2")
		return res, true
	}
	return cmpResult{}, false
}

func goCompareAPRs(p1, pc1 float64, y1 int, r1, pt1, p2, pc2 float64, y2 int, r2, pt2 float64) (APRComparisonResult, bool) {
	mk := func(price, pct float64, years int, rate, pts float64) (MtgLine, bool) {
		m := MtgLine{
			PriceStatus: types.InOutInput, Price: price,
			PctStatus: types.InOutInput, Pct: pct,
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			PointsStatus: types.InOutInput, Points: pts,
			TaxStatus: types.InOutInput, Tax: 0,
			MonthlyStatus: types.StatusEmpty,
		}
		r := Calc(m)
		if r.Err != nil {
			return MtgLine{}, false
		}
		return r.Line, true
	}
	l1, ok1 := mk(p1, pc1, y1, r1, pt1)
	l2, ok2 := mk(p2, pc2, y2, r2, pt2)
	if !ok1 || !ok2 {
		return APRComparisonResult{}, false
	}
	res, err := CompareAPRs(l1, l2, 360)
	if err != nil {
		return APRComparisonResult{}, false
	}
	return res, true
}

// TestDOSMtgCompareSweep validates the two-mortgage APR comparison — both the
// per-mortgage full-term APRs AND the crossover APR/time (the 2-D Newton in
// iterateToFindCrossoverAPRandTime) — directly against the real DOS
// ReportComparisonOfAPRs.
func TestDOSMtgCompareSweep(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s)", bin)
	}
	rng := rand.New(rand.NewSource(20260705))
	checked, classMis, aprFails, crossFails := 0, 0, 0, 0
	maxAprErr, maxCrossErr, maxTimeErr := 0.0, 0.0, 0.0
	for i := 0; i < 500; i++ {
		price := float64(100000 + rng.Intn(400000))
		pct := 0.10 + rng.Float64()*0.3
		years := 10 + rng.Intn(25)
		// One mortgage with a lower rate but higher points, the other the
		// reverse — the configuration that produces a crossover.
		rA := 0.04 + rng.Float64()*0.06
		ptA := rng.Float64() * 0.05
		rB := rA + (rng.Float64()-0.3)*0.02
		if rB < 0.02 {
			rB = 0.02
		}
		ptB := rng.Float64() * 0.05
		dos, ok := runMtgCompareOracle(price, pct, years, rA, ptA, price, pct, years, rB, ptB)
		if !ok {
			continue
		}
		go_, ok2 := goCompareAPRs(price, pct, years, rA, ptA, price, pct, years, rB, ptB)
		if !ok2 {
			continue
		}
		checked++
		goCross := go_.CrossoverTime > 0 && go_.CrossoverAPR > 0
		if goCross != dos.crossover {
			classMis++
			if classMis <= 8 {
				t.Errorf("CLASS price=%.0f yrs=%d rA=%.4f ptA=%.4f rB=%.4f ptB=%.4f: DOS cross=%v Go cross=%v (%q)",
					price, years, rA, ptA, rB, ptB, dos.crossover, goCross, go_.Summary)
			}
			continue
		}
		// Full-term APRs always compared.
		for _, p := range []struct{ d, g float64 }{{dos.apr1, go_.APR1}, {dos.apr2, go_.APR2}} {
			e := math.Abs(p.d - p.g)
			if e > maxAprErr {
				maxAprErr = e
			}
			if e > 1e-5 {
				aprFails++
				if aprFails <= 8 {
					t.Errorf("APR price=%.0f yrs=%d: DOS=%.6f Go=%.6f (|e|=%.2e)", price, years, p.d, p.g, e)
				}
			}
		}
		if dos.crossover {
			ce := math.Abs(dos.crossAPR - go_.CrossoverAPR)
			te := math.Abs(dos.crossTime - go_.CrossoverTime)
			if ce > maxCrossErr {
				maxCrossErr = ce
			}
			if te > maxTimeErr {
				maxTimeErr = te
			}
			// Crossover is a 2-D secant on both engines; allow small slack.
			if ce > 1e-4 || te > 0.05 {
				crossFails++
				if crossFails <= 10 {
					t.Errorf("CROSS price=%.0f yrs=%d rA=%.4f ptA=%.4f rB=%.4f ptB=%.4f: DOS apr=%.6f t=%.4f | Go apr=%.6f t=%.4f",
						price, years, rA, ptA, rB, ptB, dos.crossAPR, dos.crossTime, go_.CrossoverAPR, go_.CrossoverTime)
				}
			}
		}
	}
	t.Logf("mtg compare: checked %d, class mismatches %d, APR fails %d (max %.2e), crossover fails %d (max apr %.2e, max time %.2e yr)",
		checked, classMis, aprFails, maxAprErr, crossFails, maxCrossErr, maxTimeErr)
}

func runMtgMonthly(price, pct float64, years int, rate, points float64) (float64, bool) {
	out, err := exec.Command(mtgOracleBin(), "monthly",
		ff(price), ff(pct), strconv.Itoa(years), ff(rate), ff(points)).Output()
	if err != nil {
		return 0, false
	}
	m, _, _, _, ok := parseMtg(out)
	return m, ok
}

// TestDOSMtgGenerateRowsSweep validates the mortgage What-If table end-to-end:
// GenerateRows steps the rate in yield space (bumpField/VaryRate) and re-solves
// the monthly for each row; every generated row's (rate, monthly) pair must
// reproduce the real DOS monthly solve for that row's true rate. This confirms
// the rows a user sees in the What-If table are individually DOS-faithful.
func TestDOSMtgGenerateRowsSweep(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s)", bin)
	}
	rng := rand.New(rand.NewSource(20260706))
	checked, fails := 0, 0
	maxRel := 0.0
	for i := 0; i < 150; i++ {
		price := float64(100000 + rng.Intn(400000))
		pct := 0.10 + rng.Float64()*0.3
		years := 10 + rng.Intn(25)
		rate := 0.04 + rng.Float64()*0.06
		base := MtgLine{
			PriceStatus: types.InOutInput, Price: price,
			PctStatus: types.InOutInput, Pct: pct,
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			PointsStatus: types.InOutInput, Points: 0,
			TaxStatus: types.InOutInput, Tax: 0,
			MonthlyStatus: types.StatusEmpty, // monthly is the output to vary
		}
		rb := Calc(base)
		if rb.Err != nil {
			continue
		}
		inc := 0.0025 + rng.Float64()*0.005 // yield-space rate increment
		rows, err := GenerateRows(rb.Line, VaryRate, inc, 5)
		if err != nil {
			continue
		}
		for _, row := range rows {
			dosM, ok := runMtgMonthly(price, pct, years, row.Rate, 0)
			if !ok {
				continue
			}
			checked++
			rel := math.Abs(dosM-row.Monthly) / math.Max(1, math.Abs(row.Monthly))
			if rel > maxRel {
				maxRel = rel
			}
			if rel > 1e-5 {
				fails++
				if fails <= 8 {
					t.Errorf("ROW price=%.0f pct=%.3f yrs=%d r=%.6f: DOS monthly=%.4f Go=%.4f (rel %.2e)",
						price, pct, years, row.Rate, dosM, row.Monthly, rel)
				}
			}
		}
	}
	t.Logf("mtg What-If rows (VaryRate, end-to-end vs DOS): checked %d, divergences %d, max relErr=%.2e",
		checked, fails, maxRel)
}
