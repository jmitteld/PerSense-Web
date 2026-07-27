package mortgage

// FUZZER 5 — MORTGAGE, ALL ADVANCED OPTIONS SIMULTANEOUSLY.
//
// The amortization and present-value sections each grew a "fuzzer5" that drives
// EVERY optional feature at once, omitting each one independently with
// probability 0.15, and exercising multiple rows where the surface has them.
// Before this file the mortgage section had no equivalent: the existing
// differentials are per-surface and each pins ONE axis at a time.
//
//   TestDOSMtgOracleSweep        monthly/price, points + balloon, tax always 0
//   TestDOSMtgFuzzer3            price/mcash/mfin, NO points, NO tax, NO balloon
//   TestDOSMtgTaxDifferential    tax, but never with a balloon
//   TestDOSMtgBalloonSolve...    balloon, but never with a tax
//   TestDOSMtgDispatchSweep      the 7-bit presence mask, points 0, no balloon
//   TestDOSMtgBalloonCrossover.. two rows, but only row 1 ever has a balloon
//   `aprfin`                     no Go test at all
//
// Nothing crossed points x tax x balloon, and nothing fuzzed the two-row
// compare surface with independently-fuzzed balloons on BOTH rows. Those
// crossings are where the DOS engine threads (monthly - tax) through the
// balloon term and the points discount at the same time, so they are exactly
// where a port drifts.
//
// Surfaces driven, all from one fuzzed screen per case:
//
//	solvehowmuch  the ONLY oracle mode where price, pct, years, rate, monthly,
//	              balloon-when, points AND tax are all live at once (BalloonCalc)
//	eval          random 7-field presence mask x balloon when/howmuch presence
//	              x fuzzed points — dispatch consequence incl. the howmuch cell
//	taxmonthly    monthly-from-price with tax + points
//	taxprice      price-from-monthly (pct funding) with tax + points
//	taxpricecash  price-from-monthly (cash funding) with tax + points
//	taxapr        full-term APR with tax + points
//	aprfin        full-term APR, financed+monthly, no price funding (NEW)
//	monthly       monthly-from-price with points + balloon
//	price         price-from-monthly with points + balloon
//	mcash / mfin  the cash <-> pct <-> financed funding dispatch with points
//	apr           standalone APR of a balloon-bearing loan with points
//	compare       TWO independently fuzzed rows, each with its own optional
//	              balloon and points -> full-term APR1/APR2 + crossover verdict
//
// Honors PERSENSE_REQUIRE_ORACLE, PERSENSE_FUZZ_N and PERSENSE_FUZZ_SEED.
// Anti-vacuity: every surface carries a comparison counter and the test FAILS
// if any surface produced zero comparisons, so a grammar change that silently
// turns a whole surface into "oracle said ERR, skip" cannot pass as a win.

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// f5Omit is the fuzzer5 contract: each optional axis is independently absent
// 15% of the time, so the modal case has them all present at once.
func f5Omit(rng *rand.Rand) bool { return rng.Float64() < 0.15 }

// f5Case is one fuzzed mortgage screen.
type f5Case struct {
	price, pct, cash, financed float64
	monthly                    float64
	years                      int
	rate                       float64

	points  float64
	hasPts  bool
	tax     float64
	hasTax  bool
	bw      int
	bh      float64
	hasBall bool
}

// f5Gen draws one screen. Each axis independently takes an EXTREME draw ~25% of
// the time: the boundary regimes are where the DOS engine's quirks live and
// where a port silently drifts —
//
//	rate     -> near zero (the Summation limit degenerates toward n)
//	pct      -> up to 0.99, walking right at the 0.995 refusal boundary
//	           (ComputeCashPctAndFinanced, Mortgage.pas:217/232)
//	years    -> 1 (single-year term; balloon can only sit at maturity)
//	points   -> up to 0.90, where the (1-points) discount nearly vanishes
//	tax      -> can EXCEED the monthly, so (monthly - tax) goes negative and
//	           the loan negatively amortizes
//	balloon  -> when == years (balloon at maturity) or when == 1, howmuch
//	           possibly larger than the price itself
func f5Gen(rng *rand.Rand) f5Case {
	ext := func() bool { return rng.Float64() < 0.25 }
	var c f5Case
	if ext() {
		c.price = float64(5000 + rng.Intn(3000000))
	} else {
		c.price = float64(80000 + rng.Intn(600000))
	}
	if ext() {
		c.pct = rng.Float64() * 0.99
	} else {
		c.pct = 0.05 + rng.Float64()*0.4
	}
	c.cash = c.price * c.pct
	c.financed = c.price - c.cash
	if ext() {
		c.years = 1 + rng.Intn(40)
	} else {
		c.years = 5 + rng.Intn(35)
	}
	if ext() {
		c.rate = 0.0001 + rng.Float64()*0.30
	} else {
		c.rate = 0.02 + rng.Float64()*0.13
	}
	if ext() {
		c.monthly = 1 + rng.Float64()*20000
	} else {
		c.monthly = 1000 + rng.Float64()*3000
	}
	if !f5Omit(rng) {
		c.hasPts = true
		if ext() {
			c.points = rng.Float64() * 0.90
		} else {
			c.points = rng.Float64() * 0.05
		}
	}
	if !f5Omit(rng) {
		c.hasTax = true
		if ext() {
			c.tax = rng.Float64() * c.monthly * 1.5 // may exceed the monthly
		} else {
			c.tax = rng.Float64() * 600
		}
	}
	if !f5Omit(rng) {
		c.hasBall = true
		switch {
		case c.years == 1:
			c.bw = 1
		case ext():
			if rng.Intn(2) == 0 {
				c.bw = c.years // balloon at maturity
			} else {
				c.bw = 1
			}
		default:
			c.bw = 1 + rng.Intn(c.years)
		}
		if ext() {
			c.bh = c.price * (0.01 + rng.Float64()*1.5) // may exceed the price
		} else {
			c.bh = c.price * (0.05 + rng.Float64()*0.4)
		}
	}
	return c.quantize()
}

// quantize snaps every float field to the value the ORACLE will actually see.
//
// The oracle is fed decimal text (`ff`, 10 places); Go is handed the float64.
// Those are not the same number, and on the cancellation tail the gap is not
// cosmetic. BalloonCalc (Mortgage.pas:251-253) computes
//
//	balloonval := price*(1-pct) - (monthly-tax)*Summation(rate, years) - balloonval;
//	howmuch    := balloonval * exxp(rate*when);
//
// where the two leading terms are the SAME order of magnitude and the result is
// their difference. Seed 20338 draws price=552792 pct=0.3302 rate=0.11706
// monthly=3706.1543 years=33 when=33, i.e. 370260.08 - 370131.94 = 128.14 — a
// cancellation factor of ~2890 — and then exxp(0.11706*33) = 47.61 multiplies
// what is left. Total amplification of relative input error: ~137,600x.
//
// Truncating `rate` at the 10th decimal perturbs it by up to 5e-11, which alone
// moves howmuch by 6.9e-3; the observed DOS/Go gap was 7.13e-3. `pct` adds
// another 1.3e-3. So the entire divergence was the harness feeding the two
// engines different loans, not a porting defect — a real DOS user typing
// 0.1170600000 gets DOS's answer, and now so does Go.
//
// cash/financed are derived AFTER price/pct are snapped, then snapped
// themselves, so the identities the DOS solvers invert
// (ComputeCashPctAndFinanced, Mortgage.pas:215-243) hold on both sides.
func (c f5Case) quantize() f5Case {
	q := func(x float64) float64 {
		v, err := strconv.ParseFloat(ff(x), 64)
		if err != nil {
			return x
		}
		return v
	}
	c.price, c.pct = q(c.price), q(c.pct)
	c.cash = q(c.price * c.pct)
	c.financed = q(c.price - c.cash)
	c.rate, c.monthly = q(c.rate), q(c.monthly)
	c.points, c.tax, c.bh = q(c.points), q(c.tax), q(c.bh)
	return c
}

func (c f5Case) ctx() string {
	s := fmt.Sprintf("price=%.0f pct=%.4f y=%d rate=%.5f mo=%.4f", c.price, c.pct, c.years, c.rate, c.monthly)
	if c.hasPts {
		s += fmt.Sprintf(" pts=%.5f", c.points)
	} else {
		s += " pts=-"
	}
	if c.hasTax {
		s += fmt.Sprintf(" tax=%.4f", c.tax)
	} else {
		s += " tax=-"
	}
	if c.hasBall {
		s += fmt.Sprintf(" balloon=%d@%.2f", c.bw, c.bh)
	} else {
		s += " balloon=-"
	}
	return s
}

// f5Stat accumulates one surface's comparison count, failure count and worst
// absolute error. n==0 at the end of the run is itself a failure.
type f5Stat struct {
	name     string
	n, fails int
	max      float64
}

// check compares one scalar. absTol is the hard gate; when relTol > 0 an error
// that is within relTol RELATIVE to the DOS value is forgiven (the large-value
// price and balloon solves amplify input precision — same policy as the
// existing per-surface differentials).
func (s *f5Stat) check(t *testing.T, dos, got, absTol, relTol float64, ctx string) {
	t.Helper()
	s.n++
	e := math.Abs(dos - got)
	if e > s.max {
		s.max = e
	}
	if e <= absTol {
		return
	}
	if relTol > 0 && e/math.Max(1, math.Abs(dos)) <= relTol {
		return
	}
	s.fails++
	if s.fails <= 5 {
		t.Errorf("[%s] DOS=%.6f Go=%.6f (|e|=%.2e)  %s", s.name, dos, got, e, ctx)
	}
}

// --- APR convergence VERDICT, not just magnitude ---------------------------
//
// DOS's IterateToFindAPRofTerminatedLoan (Mortgage.pas:349-383) is a 20-step
// secant that BREAKS on abs(delta) < teeny but returns its success flag on the
// tighter abs(delta) < tiny, so "ran out of iterations" is a real, reachable
// outcome the user sees as "APR computation did not converge." Go's
// IterateToFindAPR ports that exactly and returns the same `converged` bool.
//
// Neither was ever compared. Both call sites erase the distinction on the way
// out of the oracle, in two DIFFERENT ways:
//
//   - the standalone apr/taxapr/aprfin modes route ReportAPR's string through
//     MessageBox and the driver turns it into "ERR apr did not converge"
//   - ReportComparisonOfAPRs (Mortgage.pas:645-660) writes
//     "Mortgage N: APR computation did not converge." into Result1/Result2,
//     which has no "APR =" substring, so the driver's FloatAfter returns its
//     -1 NOT-FOUND sentinel and the mode prints apr1/apr2 as -1/100 = -0.01
//
// fpc is not installed here, so the oracle cannot be rebuilt to report the flag
// directly; both sentinels are recognised below instead. This is what turns
// "DOS refused, skip the case" into a real assertion — a port that always
// converges would otherwise pass every one of those cases vacuously.

// f5DosNoConverge is the compare-mode sentinel: FloatAfter found no "APR ="
// and returned -1, which the driver printed as -1/100.
func f5DosNoConverge(v float64) bool { return math.Abs(v+0.01) < 1e-12 }

// f5OracleAPR runs an APR-reporting mode and separates DOS's three outcomes:
// converged (value), did-not-converge (an assertable verdict), and gated /
// refused (EnoughDataForAPR or a Calc refusal — not comparable, skipped).
func f5OracleAPR(mode string, args ...string) (apr float64, converged, comparable bool) {
	out, err := exec.Command(mtgOracleBin(), append([]string{mode}, args...)...).Output()
	if err != nil {
		return 0, false, false
	}
	raw := strings.TrimSpace(string(out))
	if strings.Contains(raw, "did not converge") {
		return 0, false, true
	}
	if strings.HasPrefix(raw, "ERR") {
		return 0, false, false // "insufficient" / "setup" / parse: a gate, not a verdict
	}
	f := strings.Fields(raw)
	for i := 0; i+1 < len(f); i += 2 {
		if f[i] == "apr" {
			v, e := strconv.ParseFloat(f[i+1], 64)
			return v, true, e == nil
		}
	}
	return 0, false, false
}

// --- eval surface: presence mask x balloon x points ------------------------

type f5EvalOutcome struct {
	refused                                 bool
	monthly, price, cash, financed, howmuch fieldOutcome
	raw                                     string
}

func f5GoEval(pr, pc, ca, fi, mo, ye, ra, bw, bh bool, points float64) f5EvalOutcome {
	m := mtgLineFromSpec(pr, pc, ca, fi, mo, ye, ra)
	m.Points = points
	if bw {
		m.WhenStatus = types.InOutInput
		m.When = 7
	}
	if bh {
		m.HowMuchStatus = types.InOutInput
		m.HowMuch = 50000
	}
	r := Calc(m)
	if r.Err != nil {
		return f5EvalOutcome{refused: true, raw: r.Err.Error()}
	}
	o := func(stat int8, v float64) fieldOutcome {
		return fieldOutcome{out: stat == types.InOutOutput, val: v}
	}
	l := r.Line
	return f5EvalOutcome{
		monthly:  o(l.MonthlyStatus, l.Monthly),
		price:    o(l.PriceStatus, l.Price),
		cash:     o(l.CashStatus, l.Cash),
		financed: o(l.FinancedStatus, l.Financed),
		howmuch:  o(l.HowMuchStatus, l.HowMuch),
	}
}

func f5DosEval(pr, pc, ca, fi, mo, ye, ra, bw, bh bool, points float64) (f5EvalOutcome, bool) {
	b := func(x bool) string {
		if x {
			return "1"
		}
		return "0"
	}
	out, err := exec.Command(mtgOracleBin(), "eval",
		b(pr), b(pc), b(ca), b(fi), b(mo), b(ye), b(ra), b(bw), b(bh), ff(points)).Output()
	raw := strings.TrimSpace(string(out))
	if err != nil {
		return f5EvalOutcome{}, false // oracle fault: not a divergence, counted separately
	}
	if strings.HasPrefix(raw, "ERR") {
		return f5EvalOutcome{refused: true, raw: raw}, true
	}
	f := strings.Fields(raw)
	m := map[string]string{}
	for i := 1; i+1 < len(f); i += 2 {
		m[f[i]] = f[i+1]
	}
	pf := func(k string) float64 { v, _ := strconv.ParseFloat(m[k], 64); return v }
	pi := func(k string) int8 { v, _ := strconv.Atoi(m[k]); return int8(v) }
	fo := func(valKey, statKey string) fieldOutcome {
		return fieldOutcome{out: pi(statKey) == 1, val: pf(valKey)} // DOS outp == 1
	}
	return f5EvalOutcome{
		monthly:  fo("monthly", "mstat"),
		price:    fo("price", "pstat"),
		cash:     fo("cash", "cstat"),
		financed: fo("financed", "fstat"),
		howmuch:  fo("howmuch", "hstat"),
		raw:      raw,
	}, true
}

// --- the fuzzer ------------------------------------------------------------

func TestDOSMtgFuzzer5(t *testing.T) {
	bin := mtgOracleBin()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("PERSENSE_REQUIRE_ORACLE set but mortgage oracle absent (%s)", bin)
		}
		t.Skipf("mortgage oracle not present (%s); build via TARGET=mtg_oracle legacy/oracle/build_linux.sh", bin)
	}
	n := 150
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			n = v
		}
	}
	seed := fuzzSeed(20260725)
	rng := rand.New(rand.NewSource(seed))
	itoa := strconv.Itoa

	sHowMuch := &f5Stat{name: "solvehowmuch"}
	sTaxMonthly := &f5Stat{name: "taxmonthly"}
	sTaxPrice := &f5Stat{name: "taxprice(pct)"}
	sTaxCash := &f5Stat{name: "taxprice(cash)"}
	sTaxAPR := &f5Stat{name: "taxapr"}
	sAprFin := &f5Stat{name: "aprfin"}
	sMonthly := &f5Stat{name: "monthly+pts+balloon"}
	sPrice := &f5Stat{name: "price+pts+balloon"}
	sMCash := &f5Stat{name: "mcash"}
	sMFin := &f5Stat{name: "mfin"}
	sAPR := &f5Stat{name: "apr+pts+balloon"}
	sCmpAPR := &f5Stat{name: "compare full-term APR"}

	evalChecked, evalRefuseBoth, evalDiv, evalFault := 0, 0, 0, 0
	cmpChecked, cmpArtifact := 0, 0
	optAll, optNone := 0, 0
	convDiv, convBothNo := 0, 0

	for i := 0; i < n; i++ {
		c := f5Gen(rng)
		ctx := c.ctx()
		switch {
		case c.hasPts && c.hasTax && c.hasBall:
			optAll++
		case !c.hasPts && !c.hasTax && !c.hasBall:
			optNone++
		}

		// (1) solvehowmuch — price, pct, years, rate, monthly, balloon-when,
		// points and tax ALL live simultaneously (Mortgage.pas:247-257).
		if c.hasBall {
			if o, ok := oracleFields("solvehowmuch", ff(c.price), ff(c.pct), itoa(c.years), ff(c.rate),
				ff(c.monthly), itoa(c.bw), ff(c.points), ff(c.tax)); ok {
				r := Calc(MtgLine{
					PriceStatus: types.InOutInput, Price: c.price,
					PctStatus: types.InOutInput, Pct: c.pct,
					YearsStatus: types.InOutInput, Years: c.years,
					RateStatus: types.InOutInput, Rate: c.rate,
					MonthlyStatus: types.InOutInput, Monthly: c.monthly,
					WhenStatus: types.InOutInput, When: c.bw,
					PointsStatus: types.InOutInput, Points: c.points,
					TaxStatus: types.InOutInput, Tax: c.tax,
					HowMuchStatus: types.StatusEmpty,
				})
				if r.Err == nil && r.Line.HowMuchStatus == types.InOutOutput {
					// howmuch = balloonval * exxp(rate*when) amplifies input
					// precision; relative gate for the large-balloon tail.
					sHowMuch.check(t, o["howmuch"], r.Line.HowMuch, 5e-3, 1e-6, ctx)
				}
			}
		}

		// (2) eval — random presence mask x balloon presence x points value.
		{
			bits := rng.Intn(128)
			pr, pc, ca := bits&1 != 0, bits&2 != 0, bits&4 != 0
			fi, mo := bits&8 != 0, bits&16 != 0
			ye, ra := bits&32 != 0, bits&64 != 0
			// The balloon PAIR is one option: omitted 15% of the time, and when
			// present either half may still be blank (when-only drives the
			// BalloonCalc solve, howmuch-only is the under-specified cell).
			bw, bh := false, false
			if !f5Omit(rng) {
				switch rng.Intn(3) {
				case 0:
					bw = true
				case 1:
					bh = true
				default:
					bw, bh = true, true
				}
			}
			pts := 0.0
			if !f5Omit(rng) {
				pts = rng.Float64() * 0.05
			}
			dos, ok := f5DosEval(pr, pc, ca, fi, mo, ye, ra, bw, bh, pts)
			if !ok {
				evalFault++
			} else {
				got := f5GoEval(pr, pc, ca, fi, mo, ye, ra, bw, bh, pts)
				evalChecked++
				ectx := fmt.Sprintf("mask Pr%d Pc%d Ca%d Fi%d Mo%d Ye%d Ra%d bw=%v bh=%v pts=%.5f",
					b2i(pr), b2i(pc), b2i(ca), b2i(fi), b2i(mo), b2i(ye), b2i(ra), bw, bh, pts)
				switch {
				case dos.refused != got.refused:
					evalDiv++
					if evalDiv <= 8 {
						t.Errorf("[eval] refusal divergence: DOS refused=%v Go refused=%v (DOS %q | Go %q)  %s",
							dos.refused, got.refused, dos.raw, got.raw, ectx)
					}
				case dos.refused:
					evalRefuseBoth++
				default:
					for _, p := range []struct {
						label string
						d, g  fieldOutcome
					}{
						{"monthly", dos.monthly, got.monthly},
						{"price", dos.price, got.price},
						{"cash", dos.cash, got.cash},
						{"financed", dos.financed, got.financed},
						{"howmuch", dos.howmuch, got.howmuch},
					} {
						if !fieldAgrees(p.d, p.g) {
							evalDiv++
							if evalDiv <= 8 {
								t.Errorf("[eval] %s: DOS out=%v %.4f Go out=%v %.4f  %s",
									p.label, p.d.out, p.d.val, p.g.out, p.g.val, ectx)
							}
						}
					}
				}
			}
		}

		// (3) tax-bearing solves, with points crossed in.
		if o, ok := oracleFields("taxmonthly", ff(c.price), ff(c.pct), itoa(c.years), ff(c.rate), ff(c.tax), ff(c.points)); ok {
			r := Calc(MtgLine{
				PriceStatus: types.InOutInput, Price: c.price, PctStatus: types.InOutInput, Pct: c.pct,
				YearsStatus: types.InOutInput, Years: c.years, RateStatus: types.InOutInput, Rate: c.rate,
				TaxStatus: types.InOutInput, Tax: c.tax, PointsStatus: types.InOutInput, Points: c.points,
				MonthlyStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				sTaxMonthly.check(t, o["monthly"], r.Line.Monthly, 5e-3, 0, ctx)
			}
		}
		if o, ok := oracleFields("taxprice", ff(c.pct), itoa(c.years), ff(c.rate), ff(c.monthly), ff(c.tax), ff(c.points)); ok {
			r := Calc(MtgLine{
				PctStatus: types.InOutInput, Pct: c.pct, YearsStatus: types.InOutInput, Years: c.years,
				RateStatus: types.InOutInput, Rate: c.rate, MonthlyStatus: types.InOutInput, Monthly: c.monthly,
				TaxStatus: types.InOutInput, Tax: c.tax, PointsStatus: types.InOutInput, Points: c.points,
				PriceStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				sTaxPrice.check(t, o["price"], r.Line.Price, 5e-3, 1e-6, ctx)
			}
		}
		if o, ok := oracleFields("taxpricecash", ff(c.cash), itoa(c.years), ff(c.rate), ff(c.monthly), ff(c.tax), ff(c.points)); ok {
			r := Calc(MtgLine{
				CashStatus: types.InOutInput, Cash: c.cash, YearsStatus: types.InOutInput, Years: c.years,
				RateStatus: types.InOutInput, Rate: c.rate, MonthlyStatus: types.InOutInput, Monthly: c.monthly,
				TaxStatus: types.InOutInput, Tax: c.tax, PointsStatus: types.InOutInput, Points: c.points,
				PriceStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				sTaxCash.check(t, o["price"], r.Line.Price, 5e-3, 1e-6, ctx)
			}
		}

		// (4) APR on an amount-financed row: with tax (taxapr) and without
		// (aprfin — previously untested). Monthly must clear interest-only for
		// the APR solve to converge.
		moA := c.financed*c.rate/12*(1.2+rng.Float64()) + c.tax
		if dosAPR, dosConv, cmp := f5OracleAPR("taxapr", ff(c.financed), ff(moA), itoa(c.years), ff(c.rate), ff(c.tax), ff(c.points)); cmp {
			gp, conv, err := FullTermAPR(MtgLine{
				FinancedStatus: types.InOutInput, Financed: c.financed, MonthlyStatus: types.InOutInput, Monthly: moA,
				YearsStatus: types.InOutInput, Years: c.years, RateStatus: types.InOutInput, Rate: c.rate,
				TaxStatus: types.InOutInput, Tax: c.tax, PointsStatus: types.InOutInput, Points: c.points,
			}, 360)
			if err == nil {
				if conv != dosConv {
					convDiv++
					if convDiv <= 8 {
						t.Errorf("[taxapr] convergence verdict: DOS=%v Go=%v  %s moA=%.4f", dosConv, conv, ctx, moA)
					}
				} else if conv {
					sTaxAPR.check(t, dosAPR, gp, 1e-5, 0, ctx+fmt.Sprintf(" moA=%.4f", moA))
				} else {
					convBothNo++
				}
			}
		}
		moF := c.financed * c.rate / 12 * (1.2 + rng.Float64())
		if dosAPR, dosConv, cmp := f5OracleAPR("aprfin", ff(c.financed), ff(moF), itoa(c.years), ff(c.rate), ff(c.points)); cmp {
			gp, conv, err := FullTermAPR(MtgLine{
				FinancedStatus: types.InOutInput, Financed: c.financed, MonthlyStatus: types.InOutInput, Monthly: moF,
				YearsStatus: types.InOutInput, Years: c.years, RateStatus: types.InOutInput, Rate: c.rate,
				TaxStatus: types.InOutInput, Tax: 0, PointsStatus: types.InOutInput, Points: c.points,
			}, 360)
			if err == nil {
				if conv != dosConv {
					convDiv++
					if convDiv <= 8 {
						t.Errorf("[aprfin] convergence verdict: DOS=%v Go=%v  %s moF=%.4f", dosConv, conv, ctx, moF)
					}
				} else if conv {
					sAprFin.check(t, dosAPR, gp, 1e-5, 0, ctx+fmt.Sprintf(" moF=%.4f", moF))
				} else {
					convBothNo++
				}
			}
		}

		// (5) monthly-from-price and standalone APR, points + balloon (these
		// oracle modes hardcode tax := 0).
		{
			args := []string{ff(c.price), ff(c.pct), itoa(c.years), ff(c.rate), ff(c.points)}
			line := MtgLine{
				PriceStatus: types.InOutInput, Price: c.price, PctStatus: types.InOutInput, Pct: c.pct,
				YearsStatus: types.InOutInput, Years: c.years, RateStatus: types.InOutInput, Rate: c.rate,
				PointsStatus: types.InOutInput, Points: c.points, TaxStatus: types.InOutInput, Tax: 0,
				MonthlyStatus: types.StatusEmpty,
			}
			if c.hasBall {
				args = append(args, itoa(c.bw), ff(c.bh))
				line.WhenStatus = types.InOutInput
				line.When = c.bw
				line.HowMuchStatus = types.InOutInput
				line.HowMuch = c.bh
			}
			if o, ok := oracleFields("monthly", args...); ok {
				if r := Calc(line); r.Err == nil {
					sMonthly.check(t, o["monthly"], r.Line.Monthly, 5e-3, 0, ctx)
				}
			}
			if dosAPR, dosConv, cmp := f5OracleAPR("apr", args...); cmp {
				if r := Calc(line); r.Err == nil {
					if gp, conv, err := FullTermAPR(r.Line, 360); err == nil {
						if conv != dosConv {
							convDiv++
							if convDiv <= 8 {
								t.Errorf("[apr] convergence verdict: DOS=%v Go=%v  %s", dosConv, conv, ctx)
							}
						} else if conv {
							sAPR.check(t, dosAPR, gp, 1e-5, 0, ctx)
						} else {
							convBothNo++
						}
					}
				}
			}
		}

		// (6) price-from-monthly with points + balloon.
		{
			args := []string{ff(c.pct), itoa(c.years), ff(c.rate), ff(c.monthly), ff(c.points)}
			line := MtgLine{
				PctStatus: types.InOutInput, Pct: c.pct, YearsStatus: types.InOutInput, Years: c.years,
				RateStatus: types.InOutInput, Rate: c.rate, MonthlyStatus: types.InOutInput, Monthly: c.monthly,
				PointsStatus: types.InOutInput, Points: c.points, TaxStatus: types.InOutInput, Tax: 0,
				PriceStatus: types.StatusEmpty,
			}
			if c.hasBall {
				args = append(args, itoa(c.bw), ff(c.bh))
				line.WhenStatus = types.InOutInput
				line.When = c.bw
				line.HowMuchStatus = types.InOutInput
				line.HowMuch = c.bh
			}
			if o, ok := oracleFields("price", args...); ok {
				if r := Calc(line); r.Err == nil {
					sPrice.check(t, o["price"], r.Line.Price, 5e-3, 1e-6, ctx)
				}
			}
		}

		// (7) the cash <-> pct <-> financed funding dispatch, with points.
		if o, ok := oracleFields("mcash", ff(c.price), ff(c.cash), itoa(c.years), ff(c.rate), ff(c.points)); ok {
			r := Calc(MtgLine{
				PriceStatus: types.InOutInput, Price: c.price, CashStatus: types.InOutInput, Cash: c.cash,
				YearsStatus: types.InOutInput, Years: c.years, RateStatus: types.InOutInput, Rate: c.rate,
				PointsStatus: types.InOutInput, Points: c.points, TaxStatus: types.InOutInput, Tax: 0,
				MonthlyStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				sMCash.check(t, o["monthly"], r.Line.Monthly, 5e-3, 0, ctx)
			}
		}
		if o, ok := oracleFields("mfin", ff(c.price), ff(c.financed), itoa(c.years), ff(c.rate), ff(c.points)); ok {
			r := Calc(MtgLine{
				PriceStatus: types.InOutInput, Price: c.price, FinancedStatus: types.InOutInput, Financed: c.financed,
				YearsStatus: types.InOutInput, Years: c.years, RateStatus: types.InOutInput, Rate: c.rate,
				PointsStatus: types.InOutInput, Points: c.points, TaxStatus: types.InOutInput, Tax: 0,
				MonthlyStatus: types.StatusEmpty,
			})
			if r.Err == nil {
				sMFin.check(t, o["monthly"], r.Line.Monthly, 5e-3, 0, ctx)
			}
		}

		// (8) MULTIPLE ROWS: the two-row comparison, each row independently
		// fuzzed with its own optional balloon and points.
		{
			c2 := f5Gen(rng)
			bw1, bh1 := 0, 0.0
			if c.hasBall {
				bw1, bh1 = c.bw, c.bh
			}
			bw2, bh2 := 0, 0.0
			if c2.hasBall {
				bw2, bh2 = c2.bw, c2.bh
			}
			dos, ok1 := runMtgCompareBalloonOracle(c.price, c.pct, c.years, c.rate, c.points, bw1, bh1,
				c2.price, c2.pct, c2.years, c2.rate, c2.points, bw2, bh2)
			go_, ok2 := goCompareBalloon(c.price, c.pct, c.years, c.rate, c.points, bw1, bh1,
				c2.price, c2.pct, c2.years, c2.rate, c2.points, bw2, bh2)
			if ok1 && ok2 {
				cmpChecked++
				cctx := "row1{" + ctx + "} row2{" + c2.ctx() + "}"
				// Convergence VERDICT first (see f5DosNoConverge): comparing the
				// magnitude of a -0.01 sentinel against a real APR is what a
				// naive harness does, and it hides the fact that both engines
				// agree on WHICH rows are unsolvable.
				for _, p := range []struct {
					label  string
					dosAPR float64
					goAPR  float64
					goConv bool
				}{
					{"apr1", dos.apr1, go_.APR1, go_.APR1Converged},
					{"apr2", dos.apr2, go_.APR2, go_.APR2Converged},
				} {
					dosConv := !f5DosNoConverge(p.dosAPR)
					if dosConv != p.goConv {
						convDiv++
						if convDiv <= 8 {
							t.Errorf("[compare %s] convergence verdict: DOS=%v Go=%v (DOS=%.6f Go=%.6f)  %s",
								p.label, dosConv, p.goConv, p.dosAPR, p.goAPR, cctx)
						}
					} else if dosConv {
						sCmpAPR.check(t, p.dosAPR, p.goAPR, 1e-5, 0, cctx)
					} else {
						convBothNo++
					}
				}
				gc := go_.CrossoverTime > 0 && go_.CrossoverAPR > 0
				if gc != dos.crossover {
					cmpArtifact++ // tie-break class flip (bounded, documented)
				} else if dos.crossover &&
					(math.Abs(dos.crossTime-go_.CrossoverTime) > 0.05 ||
						math.Abs(dos.crossAPR-go_.CrossoverAPR) > 1e-4) {
					cmpArtifact++ // TryBalloonDates fallback artifact (bounded)
				}
			}
		}
	}

	// The crossover magnitude in the TryBalloonDates fallback is a documented
	// bounded artifact (DOS reports YieldFromRate of a NON-converged r —
	// Mortgage.pas:462-534). Bound it rather than assert it per case; a spike
	// means a real regression.
	if cmpChecked > 0 && cmpArtifact*100 > cmpChecked*8 {
		t.Errorf("compare crossover artifact rate too high: %d/%d (>8%%) — likely a real regression, not the documented fallback artifact",
			cmpArtifact, cmpChecked)
	}

	surfaces := []*f5Stat{sHowMuch, sTaxMonthly, sTaxPrice, sTaxCash, sTaxAPR, sAprFin,
		sMonthly, sPrice, sMCash, sMFin, sAPR, sCmpAPR}
	var parts []string
	for _, s := range surfaces {
		parts = append(parts, fmt.Sprintf("%s n=%d fails=%d max=%.2e", s.name, s.n, s.fails, s.max))
	}
	t.Logf("mtg fuzzer5 seed=%d cases=%d (all-options %d, no-options %d)\n  %s\n  eval checked=%d both-refused=%d divergences=%d oracle-faults=%d\n  compare checked=%d bounded-artifacts=%d\n  APR convergence verdict: divergences=%d both-nonconverged=%d",
		seed, n, optAll, optNone, strings.Join(parts, "\n  "), evalChecked, evalRefuseBoth, evalDiv, evalFault, cmpChecked, cmpArtifact, convDiv, convBothNo)

	// Anti-vacuity: a surface that produced no comparisons is a broken harness,
	// not a pass.
	for _, s := range surfaces {
		if s.n == 0 {
			t.Fatalf("VACUOUS: surface %q produced 0 comparisons in %d cases (harness broken)", s.name, n)
		}
	}
	if evalChecked == 0 {
		t.Fatalf("VACUOUS: eval surface produced 0 comparisons in %d cases", n)
	}
	if evalFault*4 > n {
		t.Fatalf("eval oracle faulted on %d/%d cases — harness broken", evalFault, n)
	}
	if optAll == 0 && n >= 20 {
		t.Fatalf("VACUOUS: no case had points+tax+balloon all present in %d cases", n)
	}
}
