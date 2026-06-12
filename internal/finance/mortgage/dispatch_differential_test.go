package mortgage

import (
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/persense/persense-port/internal/types"
)

// Mortgage field-presence DISPATCH differential. The mortgage Calc reads which
// of {Price, Pct, Cash, Financed, Monthly, Years, Rate} are blank and decides
// what to solve — the funding triangle (Price ↔ Pct/Cash/Financed), Price ↔
// Monthly, or refuses an over-/under-determined screen. The solved VALUES are
// already bit-validated elsewhere; this validates the dispatch DECISION over the
// full presence matrix by consequence: which fields become OUTPUTS (and their
// values) vs a refusal, Go vs the real DOS Mortgage.Calc (mtg_oracle `eval`).
//
// Consistent tuple: 200000, 20% down (cash 40000, financed 160000), 30yr, 7%
// true rate, monthly 1066.683053, Points 0, no balloon. Build the oracle:
//   TARGET=mtg_oracle legacy/oracle/build_linux.sh

const (
	mPrice    = 200000.0
	mPct      = 0.20
	mCash     = 40000.0
	mFinanced = 160000.0
	mMonthly  = 1066.683053
	mYears    = 30
	mRate     = 0.07
)

func mst(b bool) int8 {
	if b {
		return types.InOutInput
	}
	return types.StatusEmpty
}

func mval(b bool, v float64) float64 {
	if b {
		return v
	}
	return 0
}

// mtgLineFromSpec builds a MtgLine for the 7-field presence pattern, zeroing the
// value of every blank field (no "ghost" inputs behind an empty status).
func mtgLineFromSpec(pr, pc, ca, fi, mo, ye, ra bool) MtgLine {
	years := 0
	if ye {
		years = mYears
	}
	return MtgLine{
		PriceStatus:    mst(pr),
		Price:          mval(pr, mPrice),
		PctStatus:      mst(pc),
		Pct:            mval(pc, mPct),
		CashStatus:     mst(ca),
		Cash:           mval(ca, mCash),
		FinancedStatus: mst(fi),
		Financed:       mval(fi, mFinanced),
		MonthlyStatus:  mst(mo),
		Monthly:        mval(mo, mMonthly),
		YearsStatus:    mst(ye),
		Years:          years,
		RateStatus:     mst(ra),
		Rate:           mval(ra, mRate),
		PointsStatus:   types.InOutInput,
		Points:         0,
		TaxStatus:      types.InOutInput,
		Tax:            0,
	}
}

// fieldOutcome is (is this field a computed OUTPUT, its value).
type fieldOutcome struct {
	out bool
	val float64
}

type mtgOutcome struct {
	refused                        bool
	monthly, price, cash, financed fieldOutcome
	raw                            string
}

// goMtgOutcome runs the real Go Calc and reduces to the comparable outcome.
func goMtgOutcome(pr, pc, ca, fi, mo, ye, ra bool) mtgOutcome {
	r := Calc(mtgLineFromSpec(pr, pc, ca, fi, mo, ye, ra))
	if r.Err != nil {
		return mtgOutcome{refused: true, raw: r.Err.Error()}
	}
	o := func(stat int8, v float64) fieldOutcome {
		return fieldOutcome{out: stat == types.InOutOutput, val: v}
	}
	l := r.Line
	return mtgOutcome{
		monthly:  o(l.MonthlyStatus, l.Monthly),
		price:    o(l.PriceStatus, l.Price),
		cash:     o(l.CashStatus, l.Cash),
		financed: o(l.FinancedStatus, l.Financed),
	}
}

// dosMtgEval runs the oracle `eval` mode and parses the same outcome.
func dosMtgEval(pr, pc, ca, fi, mo, ye, ra bool) mtgOutcome {
	b := func(x bool) string {
		if x {
			return "1"
		}
		return "0"
	}
	out, err := exec.Command(mtgOracleBin(), "eval",
		b(pr), b(pc), b(ca), b(fi), b(mo), b(ye), b(ra)).Output()
	raw := strings.TrimSpace(string(out))
	if err != nil {
		return mtgOutcome{refused: true, raw: "FAULT:" + err.Error()}
	}
	if strings.HasPrefix(raw, "ERR") {
		return mtgOutcome{refused: true, raw: raw}
	}
	f := strings.Fields(raw)
	// ok monthly <m> mstat <s> price <p> pstat <s> cash <c> cstat <s> financed <f> fstat <s>
	m := map[string]string{}
	for i := 1; i+1 < len(f); i += 2 {
		m[f[i]] = f[i+1]
	}
	pf := func(k string) float64 { v, _ := strconv.ParseFloat(m[k], 64); return v }
	pi := func(k string) int8 { v, _ := strconv.Atoi(m[k]); return int8(v) }
	fo := func(valKey, statKey string) fieldOutcome {
		return fieldOutcome{out: pi(statKey) == 1, val: pf(valKey)} // DOS outp == 1
	}
	return mtgOutcome{
		monthly:  fo("monthly", "mstat"),
		price:    fo("price", "pstat"),
		cash:     fo("cash", "cstat"),
		financed: fo("financed", "fstat"),
		raw:      raw,
	}
}

func fieldAgrees(a, b fieldOutcome) bool {
	if a.out != b.out {
		return false
	}
	if a.out && math.Abs(a.val-b.val) > 0.01 {
		return false
	}
	return true
}

func TestDOSMtgDispatchSweep(t *testing.T) {
	if _, err := os.Stat(mtgOracleBin()); err != nil {
		t.Skipf("mtg oracle not present (%s); build via TARGET=mtg_oracle legacy/oracle/build_linux.sh", mtgOracleBin())
	}
	var bothRefuse, bothSolve, divergences int
	for bits := 0; bits < 128; bits++ {
		pr := bits&1 != 0
		pc := bits&2 != 0
		ca := bits&4 != 0
		fi := bits&8 != 0
		mo := bits&16 != 0
		ye := bits&32 != 0
		ra := bits&64 != 0

		dos := dosMtgEval(pr, pc, ca, fi, mo, ye, ra)
		got := goMtgOutcome(pr, pc, ca, fi, mo, ye, ra)

		if dos.refused != got.refused {
			divergences++
			if divergences <= 25 {
				t.Errorf("refusal divergence Pr%d Pc%d Ca%d Fi%d Mo%d Ye%d Ra%d: DOS refused=%v Go refused=%v (DOS: %q | Go: %q)",
					b2i(pr), b2i(pc), b2i(ca), b2i(fi), b2i(mo), b2i(ye), b2i(ra),
					dos.refused, got.refused, dos.raw, got.raw)
			}
			continue
		}
		if dos.refused {
			bothRefuse++
			continue
		}
		// Both solved (or partially) — compare each field's output-status + value.
		ok := fieldAgrees(dos.monthly, got.monthly) && fieldAgrees(dos.price, got.price) &&
			fieldAgrees(dos.cash, got.cash) && fieldAgrees(dos.financed, got.financed)
		if !ok {
			divergences++
			if divergences <= 25 {
				t.Errorf("field outcome divergence Pr%d Pc%d Ca%d Fi%d Mo%d Ye%d Ra%d:\n  DOS: %q\n  Go:  m{%v %.4f} p{%v %.4f} c{%v %.4f} f{%v %.4f}",
					b2i(pr), b2i(pc), b2i(ca), b2i(fi), b2i(mo), b2i(ye), b2i(ra), dos.raw,
					got.monthly.out, got.monthly.val, got.price.out, got.price.val,
					got.cash.out, got.cash.val, got.financed.out, got.financed.val)
			}
			continue
		}
		bothSolve++
	}
	t.Logf("mortgage dispatch sweep: 128 cells | both-solve %d, both-refuse %d, divergences %d",
		bothSolve, bothRefuse, divergences)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestMtgDispatchCanonical pins the canonical mortgage dispatch rules without the
// oracle (runs in plain CI). Each case is a rule from the mortgage spec.
func TestMtgDispatchCanonical(t *testing.T) {
	// Solve monthly: Price + Pct + Years + Rate, Monthly blank.
	if o := goMtgOutcome(true, true, false, false, false, true, true); o.refused || !o.monthly.out {
		t.Errorf("Price+Pct+Years+Rate should solve Monthly: %+v", o)
	}
	// Solve monthly via Cash instead of Pct.
	if o := goMtgOutcome(true, false, true, false, false, true, true); o.refused || !o.monthly.out {
		t.Errorf("Price+Cash+Years+Rate should solve Monthly: %+v", o)
	}
	// Solve price: Pct + Years + Rate + Monthly, Price blank.
	if o := goMtgOutcome(false, true, false, false, true, true, true); o.refused || !o.price.out {
		t.Errorf("Pct+Years+Rate+Monthly should solve Price: %+v", o)
	}
	// Over-determined: Price AND Monthly both given (no balloon to absorb the slack).
	if o := goMtgOutcome(true, true, false, false, true, true, true); !o.refused {
		t.Errorf("Price+Monthly+Pct+Years+Rate should be over-determined (refused): %+v", o)
	}
	// Price-from-Monthly needs Pct or Cash; Financed-only cannot solve Price.
	if o := goMtgOutcome(false, false, false, true, true, true, true); !o.refused {
		t.Errorf("Financed+Monthly (no Pct/Cash) should refuse Price solve: %+v", o)
	}
	// Insufficient context: Price+Pct only (no Years/Rate) — the funding triangle
	// fills in (cash/financed become outputs) but no payment is solved.
	if o := goMtgOutcome(true, true, false, false, false, false, false); o.refused ||
		o.monthly.out || !o.cash.out || !o.financed.out {
		t.Errorf("Price+Pct only: expect cash/financed computed, no monthly solve: %+v", o)
	}

	// Years <= 0 is a hard error.
	if r := Calc(MtgLine{YearsStatus: types.InOutInput, Years: 0}); r.Err == nil {
		t.Error("Years<=0 should error")
	}
	// Balloon amount filled but balloon years blank is a hard error.
	bad := mtgLineFromSpec(true, true, false, false, false, true, true)
	bad.HowMuchStatus = types.InOutInput
	bad.HowMuch = 5000
	if r := Calc(bad); r.Err == nil {
		t.Error("Balloon HowMuch without When should error")
	}
}
