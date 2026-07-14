package mortgage

// DOS-differential coverage for the "Monthly Tax+Ins" (tax) field. Every other
// mtg_oracle mode hardcodes tax:=0, so the (monthly - tax) threading — where the
// monthly payment INCLUDES tax+insurance but only (monthly - tax) amortizes the
// loan — went entirely unverified against the DOS engine until the 2026-07-14
// mortgage second-pass audit. Tax flows through:
//   - monthly-from-price:  monthly = (price*(1-pct)-balloonval)/Summation + tax
//   - price-from-monthly:  uses (monthly - tax) via pct OR cash funding
//   - the APR cash flows:  ValueOfPaymentsForTerminatedLoan uses (monthly - tax)
//
// The oracle harness (legacy/oracle/mtg_oracle.pas) was extended with tax-bearing
// modes: taxmonthly / taxprice / taxpricecash / taxapr. Rebuild with
//   TARGET=mtg_oracle legacy/oracle/build_linux.sh
//
// Result: 0 divergences across monthly-solve, price-solve (pct AND cash funding),
// and APR, to the cent (max ~1e-5). Tax threading is faithful.

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

// oracleFields runs an mtg_oracle mode and parses its "label value label value"
// line into a map. Returns false on ERR or a non-numeric line.
func oracleFields(mode string, args ...string) (map[string]float64, bool) {
	out, err := exec.Command(mtgOracleBin(), append([]string{mode}, args...)...).Output()
	if err != nil {
		return nil, false
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) == 0 || f[0] == "ERR" {
		return nil, false
	}
	m := map[string]float64{}
	for i := 0; i+1 < len(f); i += 2 {
		if v, e := strconv.ParseFloat(f[i+1], 64); e == nil {
			m[f[i]] = v
		}
	}
	return m, true
}

func TestDOSMtgTaxDifferential(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s); build via TARGET=mtg_oracle legacy/oracle/build_linux.sh", bin)
	}
	// NOTE: inputs are fed to BOTH engines via ff (10 decimals). Feeding the
	// oracle a lower-precision rate than Go uses manufactures a rate-rounding
	// artifact of order P/12·Δrate (cents on the monthly, dollars on the price);
	// ff's precision is what keeps the comparison honest.
	rng := rand.New(rand.NewSource(20260714))
	cM, cP, cC, cA := 0, 0, 0, 0
	fM, fP, fC, fA := 0, 0, 0, 0
	mxM, mxP, mxC, mxA := 0.0, 0.0, 0.0, 0.0
	itoa := strconv.Itoa

	for i := 0; i < 1500; i++ {
		price := float64(80000 + rng.Intn(600000))
		pct := 0.05 + rng.Float64()*0.4
		years := 5 + rng.Intn(35)
		rate := 0.02 + rng.Float64()*0.13
		tax := rng.Float64() * 600
		points := 0.0
		if rng.Intn(2) == 0 {
			points = rng.Float64() * 0.05
		}

		// (1) monthly-from-price with tax
		if o, ok := oracleFields("taxmonthly", ff(price), ff(pct), itoa(years), ff(rate), ff(tax), ff(points)); ok {
			r := Calc(MtgLine{
				PriceStatus: types.InOutInput, Price: price, PctStatus: types.InOutInput, Pct: pct,
				YearsStatus: types.InOutInput, Years: years, RateStatus: types.InOutInput, Rate: rate,
				TaxStatus: types.InOutInput, Tax: tax, PointsStatus: types.InOutInput, Points: points,
				MonthlyStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				cM++
				e := math.Abs(o["monthly"] - r.Line.Monthly)
				if e > mxM {
					mxM = e
				}
				if e > 5e-3 {
					fM++
					if fM <= 6 {
						t.Errorf("MONTHLY price=%.0f pct=%.3f y=%d r=%.4f tax=%.2f pts=%.4f: DOS=%.6f Go=%.6f",
							price, pct, years, rate, tax, points, o["monthly"], r.Line.Monthly)
					}
				}
			}
		}

		monthly := 1000.0 + rng.Float64()*3000
		// (2) price-from-monthly via PCT, with tax
		if o, ok := oracleFields("taxprice", ff(pct), itoa(years), ff(rate), ff(monthly), ff(tax), ff(points)); ok {
			r := Calc(MtgLine{
				PctStatus: types.InOutInput, Pct: pct, YearsStatus: types.InOutInput, Years: years,
				RateStatus: types.InOutInput, Rate: rate, MonthlyStatus: types.InOutInput, Monthly: monthly,
				TaxStatus: types.InOutInput, Tax: tax, PointsStatus: types.InOutInput, Points: points,
				PriceStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				cP++
				e := math.Abs(o["price"] - r.Line.Price)
				if e > mxP {
					mxP = e
				}
				if e/math.Max(1, o["price"]) > 1e-6 && e > 5e-3 {
					fP++
					if fP <= 6 {
						t.Errorf("PRICE(pct) pct=%.3f y=%d r=%.4f mo=%.2f tax=%.2f pts=%.4f: DOS=%.6f Go=%.6f",
							pct, years, rate, monthly, tax, points, o["price"], r.Line.Price)
					}
				}
			}
		}

		cash := float64(10000 + rng.Intn(200000))
		// (3) price-from-monthly via CASH, with tax (the funding branch the
		// plain `price` mode never exercises)
		if o, ok := oracleFields("taxpricecash", ff(cash), itoa(years), ff(rate), ff(monthly), ff(tax), ff(points)); ok {
			r := Calc(MtgLine{
				CashStatus: types.InOutInput, Cash: cash, YearsStatus: types.InOutInput, Years: years,
				RateStatus: types.InOutInput, Rate: rate, MonthlyStatus: types.InOutInput, Monthly: monthly,
				TaxStatus: types.InOutInput, Tax: tax, PointsStatus: types.InOutInput, Points: points,
				PriceStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				cC++
				e := math.Abs(o["price"] - r.Line.Price)
				if e > mxC {
					mxC = e
				}
				if e/math.Max(1, o["price"]) > 1e-6 && e > 5e-3 {
					fC++
					if fC <= 6 {
						t.Errorf("PRICE(cash) cash=%.0f y=%d r=%.4f mo=%.2f tax=%.2f pts=%.4f: DOS=%.6f Go=%.6f",
							cash, years, rate, monthly, tax, points, o["price"], r.Line.Price)
					}
				}
			}
		}

		// (4) full-term APR with tax
		financed := float64(80000 + rng.Intn(500000))
		moA := financed*rate/12*(1.2+rng.Float64()) + tax
		if o, ok := oracleFields("taxapr", ff(financed), ff(moA), itoa(years), ff(rate), ff(tax), ff(points)); ok {
			gp, conv, err := FullTermAPR(MtgLine{
				FinancedStatus: types.InOutInput, Financed: financed, MonthlyStatus: types.InOutInput, Monthly: moA,
				YearsStatus: types.InOutInput, Years: years, RateStatus: types.InOutInput, Rate: rate,
				TaxStatus: types.InOutInput, Tax: tax, PointsStatus: types.InOutInput, Points: points,
			}, 360)
			if err == nil && conv {
				cA++
				e := math.Abs(o["apr"] - gp)
				if e > mxA {
					mxA = e
				}
				if e > 1e-5 {
					fA++
					if fA <= 6 {
						t.Errorf("APR fin=%.0f mo=%.2f y=%d r=%.4f tax=%.2f pts=%.4f: DOS=%.6f Go=%.6f",
							financed, moA, years, rate, tax, points, o["apr"], gp)
					}
				}
			}
		}
	}
	t.Logf("tax differential — monthly: n=%d fails=%d max=%.2e | price(pct): n=%d fails=%d max=%.2e | price(cash): n=%d fails=%d max=%.2e | apr: n=%d fails=%d max=%.2e",
		cM, fM, mxM, cP, fP, mxP, cC, fC, mxC, cA, fA, mxA)
}
