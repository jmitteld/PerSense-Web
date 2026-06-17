package mortgage

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// TestDOSMortgageDispatchCube is the exhaustive option-cube sweep for the
// mortgage engine (see docs/exhaustive_option_sweep_plan.md, Phase 1). Where the
// other mortgage oracle tests RANDOMLY sample within a shape, this DETERMINISTICALLY
// enumerates every parametrically-drivable dispatch shape —
//
//	solve direction / funding-unknown ∈ {solve-monthly-from-%down, -from-cash,
//	    -from-financed, solve-price-from-monthly}
//	× balloon ∈ {none, given}   (balloon is added on the solve-monthly path)
//	× points  ∈ {0, nonzero}
//
// over a fixed value grid (price × %down × years × rate), and asserts (a) zero
// divergence from the real DOS engine on every cell and (b) that every shape cell
// was actually exercised (a coverage map, so a generator gap can't pass silently).
//
// Not yet in the cube (Phase 2, needs oracle work — tracked in the plan): the
// 30/360-hybrid basis and daily compounding (no amort-style equivalent here),
// property tax in the Monthly Total (oracle modes take no tax arg), and
// parametrized balloon-AMOUNT-solve (covered fixed-input by
// TestDOSMtgBalloonPointsDispatch via the `eval` mode).
func TestDOSMortgageDispatchCube(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s); build via TARGET=mtg_oracle legacy/oracle/build_linux.sh", bin)
	}

	prices := []float64{75000, 200000, 500000}
	pcts := []float64{0.05, 0.20, 0.40}
	yearsList := []int{15, 30}
	rates := []float64{0.04, 0.08, 0.12}
	pointsOpts := []float64{0, 0.025}

	run := func(args ...string) (m, p, c, fi float64, ok bool) {
		out, err := exec.Command(bin, args...).Output()
		if err != nil {
			return 0, 0, 0, 0, false
		}
		return parseMtg(out)
	}

	cover := map[string]int{}
	checked, fails := 0, 0
	maxRel := 0.0
	var worst string
	fail := func(shape string, relM, relF float64, dm, gm, df, gf float64, ctx string) {
		r := math.Max(relM, relF)
		if r > maxRel {
			maxRel, worst = r, shape+" "+ctx
		}
		if r > 1e-6 {
			fails++
			if fails <= 15 {
				t.Errorf("[%s] %s: monthly DOS=%.6f Go=%.6f (rel %.2e) | financed DOS=%.4f Go=%.4f (rel %.2e)",
					shape, ctx, dm, gm, relM, df, gf, relF)
			}
		}
	}

	// --- solve-monthly family: from %down, from cash, from financed ---
	for _, mode := range []string{"monthly", "mcash", "mfin"} {
		for _, balloon := range []bool{false, true} {
			if balloon && mode != "monthly" {
				continue // oracle takes balloon args only on the `monthly` mode
			}
			for _, points := range pointsOpts {
				shape := fmt.Sprintf("%s|balloon=%v|points=%v", mode, balloon, points > 0)
				for _, price := range prices {
					for _, pct := range pcts {
						for _, years := range yearsList {
							for _, rate := range rates {
								financed := price * (1 - pct)
								cash := price * (pct + (1-pct)*points)
								var oargs []string
								m := MtgLine{
									PriceStatus: types.InOutInput, Price: price,
									YearsStatus: types.InOutInput, Years: years,
									RateStatus: types.InOutInput, Rate: rate,
									PointsStatus: types.InOutInput, Points: points,
									TaxStatus: types.InOutInput, Tax: 0,
									MonthlyStatus: types.StatusEmpty,
								}
								switch mode {
								case "monthly":
									m.PctStatus, m.Pct = types.InOutInput, pct
									oargs = []string{"monthly", ff(price), ff(pct), strconv.Itoa(years), ff(rate), ff(points)}
									if balloon {
										bWhen := 1 + years/2
										bHow := price * 0.15
										m.WhenStatus, m.When = types.InOutInput, bWhen
										m.HowMuchStatus, m.HowMuch = types.InOutInput, bHow
										oargs = append(oargs, strconv.Itoa(bWhen), ff(bHow))
									}
								case "mcash":
									m.CashStatus, m.Cash = types.InOutInput, cash
									oargs = []string{"mcash", ff(price), ff(cash), strconv.Itoa(years), ff(rate), ff(points)}
								case "mfin":
									m.FinancedStatus, m.Financed = types.InOutInput, financed
									oargs = []string{"mfin", ff(price), ff(financed), strconv.Itoa(years), ff(rate), ff(points)}
								}
								om, _, _, of, ok := run(oargs...)
								if !ok {
									continue
								}
								r := Calc(m)
								if r.Err != nil {
									continue
								}
								checked++
								cover[shape]++
								relM := math.Abs(om-r.Line.Monthly) / math.Max(1, r.Line.Monthly)
								relF := math.Abs(of-r.Line.Financed) / math.Max(1, r.Line.Financed)
								fail(shape, relM, relF, om, r.Line.Monthly, of, r.Line.Financed,
									fmt.Sprintf("price=%.0f pct=%.2f yrs=%d r=%.2f", price, pct, years, rate))
							}
						}
					}
				}
			}
		}
	}

	// --- solve-price from a known monthly ---
	monthlies := []float64{800, 1500, 3000}
	for _, points := range pointsOpts {
		shape := fmt.Sprintf("price|points=%v", points > 0)
		for _, pct := range pcts {
			for _, years := range yearsList {
				for _, rate := range rates {
					for _, monthly := range monthlies {
						om, op, _, _, ok := run("price", ff(pct), strconv.Itoa(years), ff(rate), ff(monthly), ff(points))
						_ = om
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
						checked++
						cover[shape]++
						rel := math.Abs(op-r.Line.Price) / math.Max(1, r.Line.Price)
						if rel > maxRel {
							maxRel, worst = rel, shape+fmt.Sprintf(" pct=%.2f yrs=%d r=%.2f m=%.0f", pct, years, rate, monthly)
						}
						if rel > 1e-6 {
							fails++
							if fails <= 15 {
								t.Errorf("[%s] pct=%.2f yrs=%d r=%.2f monthly=%.0f: price DOS=%.4f Go=%.4f (rel %.2e)",
									shape, pct, years, rate, monthly, op, r.Line.Price, rel)
							}
						}
					}
				}
			}
		}
	}

	// --- coverage: every enumerated shape cell must have been exercised ---
	wantShapes := []string{
		"monthly|balloon=false|points=false", "monthly|balloon=false|points=true",
		"monthly|balloon=true|points=false", "monthly|balloon=true|points=true",
		"mcash|balloon=false|points=false", "mcash|balloon=false|points=true",
		"mfin|balloon=false|points=false", "mfin|balloon=false|points=true",
		"price|points=false", "price|points=true",
	}
	for _, s := range wantShapes {
		if cover[s] == 0 {
			t.Errorf("dispatch shape %q never exercised — coverage gap", s)
		}
	}
	t.Logf("mortgage dispatch cube: %d cells checked across %d shapes, divergences %d, max relErr=%.2e at [%s]",
		checked, len(wantShapes), fails, maxRel, worst)
	if checked < 500 {
		t.Fatalf("cube exercised only %d cells — oracle may be flaking", checked)
	}
}
