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

// Balloon + points dispatch differential vs the real DOS engine — the two axes
// the main 128-cell mortgage dispatch sweep held fixed (no balloon, points=0).
// Validates: (a) the balloon-unknown dispatch (When given, HowMuch blank ⇒ DOS
// and Go both SOLVE HowMuch), and (b) points flowing into the Cash Required
// computation. Drives the extended mtg_oracle `eval` args (9=When, 10=HowMuch,
// 11=points value) and compares the solved HowMuch and Cash + their statuses.

func evalBalloon(t *testing.T, pr, pc, mo, ye, ra, whn, hm bool, points float64) (map[string]string, bool) {
	b := func(x bool) string {
		if x {
			return "1"
		}
		return "0"
	}
	args := []string{"eval", b(pr), b(pc), "0", "0", b(mo), b(ye), b(ra), b(whn), b(hm),
		strconv.FormatFloat(points, 'f', 4, 64)}
	out, err := exec.Command(mtgOracleBin(), args...).Output()
	raw := strings.TrimSpace(string(out))
	if err != nil || strings.HasPrefix(raw, "ERR") {
		return nil, false
	}
	f := strings.Fields(raw)
	m := map[string]string{}
	for i := 1; i+1 < len(f); i += 2 {
		m[f[i]] = f[i+1]
	}
	return m, true
}

func TestDOSMtgBalloonPointsDispatch(t *testing.T) {
	if _, err := os.Stat(mtgOracleBin()); err != nil {
		t.Skipf("mtg oracle not present (%s); build via TARGET=mtg_oracle legacy/oracle/build_linux.sh", mtgOracleBin())
	}
	// Mirror the oracle's consistent eval tuple.
	const (
		price   = 200000.0
		pct     = 0.20
		monthly = 1066.683053
		years   = 30
		rate    = 0.07
	)
	pf := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	pi := func(s string) int8 { v, _ := strconv.Atoi(s); return int8(v) }

	checked, fails := 0, 0
	for _, points := range []float64{0, 0.025, 0.05} {
		// Balloon-unknown: Price+Pct+Monthly+Years+Rate present, When given,
		// HowMuch blank ⇒ both engines solve HowMuch.
		dos, ok := evalBalloon(t, true, true, true, true, true, true, false, points)
		if !ok {
			t.Fatalf("oracle eval (balloon-unk, points=%.3f) failed", points)
		}
		m := MtgLine{
			PriceStatus: types.InOutInput, Price: price,
			PctStatus: types.InOutInput, Pct: pct,
			MonthlyStatus: types.InOutInput, Monthly: monthly,
			YearsStatus: types.InOutInput, Years: years,
			RateStatus: types.InOutInput, Rate: rate,
			WhenStatus: types.InOutInput, When: 7, // matches the oracle's balloon year
			PointsStatus: types.InOutInput, Points: points,
			TaxStatus: types.InOutInput, Tax: 0,
		}
		r := Calc(m)
		if r.Err != nil {
			t.Errorf("Go Calc (balloon-unk, points=%.3f): %v", points, r.Err)
			continue
		}
		checked++
		// Balloon solved on both sides (status = output)?
		if pi(dos["hstat"]) != int8(1) || r.Line.HowMuchStatus != types.InOutOutput {
			fails++
			t.Errorf("points=%.3f: balloon-solve status DOS hstat=%s Go=%d (want both output)",
				points, dos["hstat"], r.Line.HowMuchStatus)
		}
		if math.Abs(pf(dos["howmuch"])-r.Line.HowMuch) > 0.01 {
			fails++
			t.Errorf("points=%.3f: HowMuch DOS=%.4f Go=%.4f", points, pf(dos["howmuch"]), r.Line.HowMuch)
		}
		// Points → Cash Required (a meaningful non-zero check: cash rises with points).
		if math.Abs(pf(dos["cash"])-r.Line.Cash) > 0.01 {
			fails++
			t.Errorf("points=%.3f: Cash DOS=%.4f Go=%.4f", points, pf(dos["cash"]), r.Line.Cash)
		}
	}
	t.Logf("mortgage balloon+points dispatch: checked %d, fails %d", checked, fails)
}
