package presentvalue

import (
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// basisSettings maps the oracle basis code (0=x365, 1=x360, 2=x365_360) to the
// Go BasisType + YrDays/YrInv the DOS SetYrDays produces (365.25 for x365,
// else 360; INTSUTIL.pas:333).
func basisSettings(code int) (types.BasisType, float64, float64) {
	switch code {
	case 0:
		return types.Basis365, 365.25, 1.0 / 365.25
	case 2:
		return types.Basis365360, 360, 1.0 / 360
	default:
		return types.Basis360, 360, 1.0 / 360
	}
}

func colaModeToken(rng *rand.Rand) (string, byte) {
	switch rng.Intn(4) {
	case 0:
		return "cnt", types.COLAContinuous
	case 1:
		m := 1 + rng.Intn(12)
		return strconv.Itoa(m), byte(m)
	default:
		return "ann", types.COLAAnnual
	}
}

func runPVPeriodicGenOracle(amt, rate float64, perYr, n, fromOff, fromDay, asofDay, basis int, cola float64, colamode string) (float64, bool) {
	args := []string{"periodic_gen",
		strconv.FormatFloat(amt, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(perYr), strconv.Itoa(n), strconv.Itoa(fromOff),
		strconv.Itoa(fromDay), strconv.Itoa(asofDay), strconv.Itoa(basis),
		strconv.FormatFloat(cola, 'f', 10, 64), colamode}
	out, err := exec.Command(pvOracleBin(), args...).Output()
	if err != nil {
		return 0, false
	}
	return parsePV(out)
}

func goPVPeriodicGen(amt, rate float64, perYr, n, fromOff, fromDay, asofDay, basis int, cola float64, cm byte) float64 {
	b, yrDays, yrInv := basisSettings(basis)
	addMonths := func(mo, day int) types.DateRec {
		yoff, moff := mo/12, mo%12
		if moff < 0 {
			moff += 12
			yoff--
		}
		return types.NewDateRec(2024+yoff, time.Month(moff+1), day)
	}
	mPer := 12 / perYr
	cs := int8(types.StatusEmpty)
	if cola != 0 {
		cs = types.InOutInput
	}
	in := PVInput{
		Periodics: []PeriodicPayment{{
			FromDateStatus: types.InOutInput, FromDate: addMonths(-fromOff, fromDay),
			ToDateStatus: types.InOutInput, ToDate: addMonths(-fromOff+n*mPer, fromDay),
			PerYrStatus: types.InOutInput, PerYr: perYr,
			AmtStatus: types.InOutInput, Amt: amt, COLAStatus: cs, COLA: cola, ValStatus: types.StatusEmpty}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, asofDay),
			R:              RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
			SumValueStatus: types.StatusEmpty},
		Settings: PVSettings{Basis: b, PerYr: byte(perYr), YrDays: yrDays, YrInv: yrInv, COLAMonth: cm},
	}
	return Calculate(in).SumValue
}

func runPVLumpGenOracle(amt, rate float64, off, day, asofDay, basis int) (float64, bool) {
	out, err := exec.Command(pvOracleBin(), "lump_gen",
		strconv.FormatFloat(amt, 'f', 2, 64), strconv.FormatFloat(rate, 'f', 10, 64),
		strconv.Itoa(off), strconv.Itoa(day), strconv.Itoa(asofDay), strconv.Itoa(basis)).Output()
	if err != nil {
		return 0, false
	}
	return parsePV(out)
}

func goPVLumpGen(amt, rate float64, off, day, asofDay, basis int) float64 {
	b, yrDays, yrInv := basisSettings(basis)
	yoff, moff := off/12, off%12
	if moff < 0 {
		moff += 12
		yoff--
	}
	in := PVInput{
		LumpSums: []LumpSumPayment{{
			DateStatus: types.InOutInput, Date: types.NewDateRec(2024+yoff, time.Month(moff+1), day),
			AmtStatus: types.InOutInput, Amt: amt, ValStatus: types.StatusEmpty}},
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: types.NewDateRec(2024, time.January, asofDay),
			R:              RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
			SumValueStatus: types.StatusEmpty},
		Settings: PVSettings{Basis: b, PerYr: 1, YrDays: yrDays, YrInv: yrInv, COLAMonth: types.COLAAnnual},
	}
	return Calculate(in).SumValue
}

// TestDOSPVOracleGenSweep strictly validates the port against DOS across every
// corner the old sweeps pinned: day-of-month (payments AND as-of), day-count
// basis (all three), from-offset sign (pre- and post-as-of), and COLA up to and
// beyond the rate — under every COLA mode. All of it matches DOS to the cent
// after (a) counting installments with NumberOfInstallments and (b) porting DOS's
// three-period SummationForSteppedCola for stepped COLA. This test guards the
// corners that were once the docs/pv_periodic_divergence_frontier.md frontier.
func TestDOSPVOracleGenSweep(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s)", pvOracleBin())
	}
	rng := rand.New(rand.NewSource(20260706))

	type bucket struct {
		checked, fails int
		max            float64
	}
	buckets := map[string]*bucket{}
	pChecked, pFails := 0, 0
	for i := 0; i < 900; i++ {
		amt := float64(100 + rng.Intn(20000))
		rate := 0.02 + rng.Float64()*0.13
		perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
		n := 2 + rng.Intn(24*perYr)
		fromOff := -60 + rng.Intn(300) // -60 (after) .. +239 (before as-of)
		fromDay := 1 + rng.Intn(28)
		asofDay := 1 + rng.Intn(28)
		basis := rng.Intn(3)
		var cola float64
		switch rng.Intn(3) {
		case 0:
			cola = 0
		case 1:
			cola = rate
		default:
			cola = rng.Float64() * rate * 1.5
		}
		token, cm := colaModeToken(rng)
		op, ok := runPVPeriodicGenOracle(amt, rate, perYr, n, fromOff, fromDay, asofDay, basis, cola, token)
		if !ok {
			continue
		}
		gp := goPVPeriodicGen(amt, rate, perYr, n, fromOff, fromDay, asofDay, basis, cola, cm)
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		basisName := map[int]string{0: "x365", 1: "x360", 2: "x365_360"}[basis]
		b := buckets[basisName]
		if b == nil {
			b = &bucket{}
			buckets[basisName] = b
		}
		b.checked++
		if rel > b.max {
			b.max = rel
		}
		pChecked++
		if rel > 1e-6 {
			b.fails++
			pFails++
			if pFails <= 12 {
				t.Errorf("PERIODIC_GEN amt=%.0f r=%.4f py=%d n=%d off=%d fday=%d aday=%d basis=%s cola=%.4f mode=%s: DOS=%.6f Go=%.6f (rel %.2e)",
					amt, rate, perYr, n, fromOff, fromDay, asofDay, basisName, cola, token, op, gp, rel)
			}
		}
	}
	for _, key := range []string{"x360", "x365", "x365_360"} {
		if b := buckets[key]; b != nil {
			t.Logf("periodic_gen basis=%-9s checked=%-4d divergences=%-4d maxRelErr=%.2e", key, b.checked, b.fails, b.max)
		}
	}

	lc, lf, lm := 0, 0, 0.0
	for i := 0; i < 500; i++ {
		amt := float64(1000 + rng.Intn(499000))
		rate := 0.005 + rng.Float64()*0.15
		off := -240 + rng.Intn(720) // pre- and post-as-of
		day := 1 + rng.Intn(28)
		asofDay := 1 + rng.Intn(28)
		basis := rng.Intn(3)
		op, ok := runPVLumpGenOracle(amt, rate, off, day, asofDay, basis)
		if !ok {
			continue
		}
		gp := goPVLumpGen(amt, rate, off, day, asofDay, basis)
		lc++
		rel := math.Abs(op-gp) / math.Max(1, math.Abs(gp))
		if rel > lm {
			lm = rel
		}
		if rel > 1e-6 {
			lf++
			if lf <= 12 {
				t.Errorf("LUMP_GEN amt=%.0f r=%.4f off=%d day=%d aday=%d basis=%d: DOS=%.6f Go=%.6f (rel %.2e)",
					amt, rate, off, day, asofDay, basis, op, gp, rel)
			}
		}
	}
	t.Logf("lump_gen (all bases/days): checked %d, divergences %d, max relErr=%.2e", lc, lf, lm)

	if pChecked == 0 || lc == 0 {
		t.Fatalf("no gen cases ran — oracle at %s may predate the *_gen modes", pvOracleBin())
	}
}
