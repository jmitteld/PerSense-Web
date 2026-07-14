package mortgage

// Pass-3 (2026-07-14) DOS-differential coverage for balloon-bearing SOLVE paths
// that no existing sweep exercised directly against the oracle:
//   1. standalone single-mortgage APR of a loan that HAS a balloon (the
//      ReportAPR path, not just the crossover's ValueOfPaymentsForTerminatedLoan)
//   2. the balloon-AMOUNT solve (BalloonCalc) across the input space — only a
//      single fixed tuple was covered before (TestDOSMtgBalloonPointsDispatch)
//   3. price-from-monthly WITH a known balloon (the balloonval term in the
//      price solve, Mortgage.pas:294)
//   4. the pct>=0.995 %-down refusal boundary for cash and financed inputs
//
// The oracle harness (legacy/oracle/mtg_oracle.pas) gained a `solvehowmuch`
// mode and optional balloon axes on the `price` mode for this; the `apr` mode
// already accepted balloon args. Result: 0 divergences to the cent across all
// four. See the 2026-07-14 mortgage audit doc, "Third pass".

import (
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

func TestDOSMtgBalloonSolveDifferential(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("mortgage oracle not present (%s); build via TARGET=mtg_oracle legacy/oracle/build_linux.sh", bin)
	}
	rng := rand.New(rand.NewSource(20260714))
	itoa := strconv.Itoa
	cAPR, fAPR, mxAPR := 0, 0, 0.0
	cHM, fHM, mxHM := 0, 0, 0.0
	cPB, fPB, mxPB := 0, 0, 0.0

	for i := 0; i < 1500; i++ {
		price := float64(80000 + rng.Intn(600000))
		pct := 0.05 + rng.Float64()*0.4
		years := 5 + rng.Intn(35)
		rate := 0.02 + rng.Float64()*0.13
		points := rng.Float64() * 0.05
		bw := 1 + rng.Intn(years-1)
		bh := price * (0.05 + rng.Float64()*0.4)
		monthly := 1000.0 + rng.Float64()*3000

		// (1) standalone balloon APR (apr mode with balloon + points)
		if o, ok := oracleFields("apr", ff(price), ff(pct), itoa(years), ff(rate), ff(points), itoa(bw), ff(bh)); ok {
			r := Calc(MtgLine{
				PriceStatus: types.InOutInput, Price: price, PctStatus: types.InOutInput, Pct: pct,
				YearsStatus: types.InOutInput, Years: years, RateStatus: types.InOutInput, Rate: rate,
				PointsStatus: types.InOutInput, Points: points, TaxStatus: types.InOutInput, Tax: 0,
				WhenStatus: types.InOutInput, When: bw, HowMuchStatus: types.InOutInput, HowMuch: bh,
				MonthlyStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				if gp, conv, err := FullTermAPR(r.Line, 360); err == nil && conv {
					cAPR++
					e := math.Abs(o["apr"] - gp)
					if e > mxAPR {
						mxAPR = e
					}
					if e > 1e-5 {
						fAPR++
						if fAPR <= 6 {
							t.Errorf("BALLOON-APR price=%.0f pct=%.3f y=%d r=%.4f pts=%.4f bw=%d bh=%.0f: DOS=%.6f Go=%.6f",
								price, pct, years, rate, points, bw, bh, o["apr"], gp)
						}
					}
				}
			}
		}

		// (2) balloon-amount solve (solvehowmuch)
		if o, ok := oracleFields("solvehowmuch", ff(price), ff(pct), itoa(years), ff(rate), ff(monthly), itoa(bw), ff(points)); ok {
			r := Calc(MtgLine{
				PriceStatus: types.InOutInput, Price: price, PctStatus: types.InOutInput, Pct: pct,
				YearsStatus: types.InOutInput, Years: years, RateStatus: types.InOutInput, Rate: rate,
				MonthlyStatus: types.InOutInput, Monthly: monthly, WhenStatus: types.InOutInput, When: bw,
				PointsStatus: types.InOutInput, Points: points, TaxStatus: types.InOutInput, Tax: 0,
				HowMuchStatus: types.StatusEmpty,
			})
			if r.Err == nil && r.Line.HowMuchStatus == types.InOutOutput {
				cHM++
				e := math.Abs(o["howmuch"] - r.Line.HowMuch)
				if e > mxHM {
					mxHM = e
				}
				// howmuch = balloonval·exxp(rate·when) amplifies input precision;
				// use a relative gate for the large-balloon tail.
				if e/math.Max(1, math.Abs(o["howmuch"])) > 1e-6 && e > 5e-3 {
					fHM++
					if fHM <= 6 {
						t.Errorf("HOWMUCH price=%.0f pct=%.3f y=%d r=%.4f mo=%.2f bw=%d pts=%.4f: DOS=%.6f Go=%.6f",
							price, pct, years, rate, monthly, bw, points, o["howmuch"], r.Line.HowMuch)
					}
				}
			}
		}

		// (3) price-from-monthly WITH a known balloon
		if o, ok := oracleFields("price", ff(pct), itoa(years), ff(rate), ff(monthly), ff(points), itoa(bw), ff(bh)); ok {
			r := Calc(MtgLine{
				PctStatus: types.InOutInput, Pct: pct, YearsStatus: types.InOutInput, Years: years,
				RateStatus: types.InOutInput, Rate: rate, MonthlyStatus: types.InOutInput, Monthly: monthly,
				PointsStatus: types.InOutInput, Points: points, TaxStatus: types.InOutInput, Tax: 0,
				WhenStatus: types.InOutInput, When: bw, HowMuchStatus: types.InOutInput, HowMuch: bh,
				PriceStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				cPB++
				e := math.Abs(o["price"] - r.Line.Price)
				if e > mxPB {
					mxPB = e
				}
				if e/math.Max(1, math.Abs(o["price"])) > 1e-6 && e > 5e-3 {
					fPB++
					if fPB <= 6 {
						t.Errorf("PRICE+BALLOON pct=%.3f y=%d r=%.4f mo=%.2f pts=%.4f bw=%d bh=%.0f: DOS=%.6f Go=%.6f",
							pct, years, rate, monthly, points, bw, bh, o["price"], r.Line.Price)
					}
				}
			}
		}
	}
	t.Logf("balloon solve differential — APR: n=%d fails=%d max=%.2e | howmuch: n=%d fails=%d max=%.2e | price+balloon: n=%d fails=%d max=%.2e",
		cAPR, fAPR, mxAPR, cHM, fHM, mxHM, cPB, fPB, mxPB)
}

// TestDOSMtgPctDownBoundary pins the %-down refusal boundary
// (ComputeCashPctAndFinanced, Mortgage.pas:217/232): DOS computes normally for
// pct<0.995 and sets errorflag (refuses) at pct>=0.995, for both cash-required
// and amount-financed inputs. Pure Go; goldens transcribed from the oracle:
//
//	mcash 200000 198000 30 0.07 -> monthly 13.333538 (valid)
//	mcash 200000 199500 30 0.07 -> ERR errorflag
//	mfin  200000 2000   30 0.07 -> monthly 13.333538 (valid)
//	mfin  200000 500    30 0.07 -> ERR errorflag
func TestDOSMtgPctDownBoundary(t *testing.T) {
	base := func() MtgLine {
		return MtgLine{
			PriceStatus: types.InOutInput, Price: 200000, YearsStatus: types.InOutInput, Years: 30,
			RateStatus: types.InOutInput, Rate: 0.07, PointsStatus: types.InOutInput, Points: 0,
			TaxStatus: types.InOutInput, Tax: 0, MonthlyStatus: types.StatusEmpty,
		}
	}
	cases := []struct {
		name    string
		cash    bool
		val     float64
		wantErr bool
	}{
		{"cash pct=0.990 valid", true, 198000, false},
		{"cash pct=0.9975 refuse", true, 199500, true},
		{"financed pct=0.990 valid", false, 2000, false},
		{"financed pct=0.9975 refuse", false, 500, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := base()
			if c.cash {
				m.CashStatus = types.InOutInput
				m.Cash = c.val
			} else {
				m.FinancedStatus = types.InOutInput
				m.Financed = c.val
			}
			r := Calc(m)
			if (r.Err != nil) != c.wantErr {
				t.Fatalf("err=%v want refuse=%v", r.Err, c.wantErr)
			}
			if !c.wantErr && math.Abs(r.Line.Monthly-13.333538) > 5e-3 {
				t.Errorf("monthly=%.6f want ~13.333538 (DOS)", r.Line.Monthly)
			}
		})
	}
}
