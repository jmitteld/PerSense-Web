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

// TestDOSPVFuzzer2MultiRow extends the rotate-the-unknown PV fuzzer2 to
// MULTI-ROW worksheets: a random mix of lump and periodic rows sharing one
// rate and as-of date. Each step blanks a DIFFERENT field and solves it with
// the Go BackwardCalc dispatch. Two families of unknown rotate:
//
//   - SCALARS (a lump amount, a periodic amount, or the shared rate): solved,
//     HARDENED into the worksheet, and pinned to DOS by ROUND-TRIP — DOS's
//     bit-identical `multi` FORWARD oracle of Go's solved worksheet must
//     reproduce the target Sum Value (rel ≤ 1e-5). The target is re-anchored to
//     that DOS-canonical value so the chain never drifts off DOS.
//
//   - DATES (a lump payment date, or the shared as-of date): solved from a
//     deliberately-WRONG seed and checked to recover the DOS-canonical planted
//     date to the day. Dates are NOT hardened — the `multi` oracle only accepts
//     integer-month lump dates, so an arbitrary solved date can't be
//     round-tripped through it; instead the planted date IS the DOS-canonical
//     answer (the target was computed by DOS forward from it), and Go must
//     recover it. The worksheet stays integer-month so the scalar steps around
//     it remain round-trippable.
//
// DOS's own multi-row lump-block BackwardCalc faults headlessly
// (pv_oracle.pas:849), so the forward round-trip / planted-date recovery is the
// reference — the same technique the single-row lump backward differentials use.
//
// This stresses computeKnownRowSum ("subtract the KNOWN rows' PV, solve the one
// UNKNOWN row" — backward.go solveLumpAmount / solvePeriodicAmount /
// solveLumpDate) under rotation, plus the multi-row shared rate and as-of solves
// over a mixed lump+periodic stream. The regime (rate ≤ 12%, horizons ≤ 15yr)
// keeps every date solve well-conditioned. Honors FUZZER2_CASES (default 60) and
// PERSENSE_FUZZ_SEED.
func TestDOSPVFuzzer2MultiRow(t *testing.T) {
	if _, err := os.Stat(pvOracleBin()); err != nil {
		t.Skipf("PV oracle not present (%s); build via TARGET=pv_oracle legacy/oracle/build_linux.sh", pvOracleBin())
	}

	seed := int64(20260716)
	if s := os.Getenv("PERSENSE_FUZZ_SEED"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = v
		}
	}
	rng := rand.New(rand.NewSource(seed))

	type lumpRow struct {
		months int
		amt    float64
	}
	type perRow struct {
		perYr, n int
		amt      float64
	}

	statusIf := func(empty bool) int8 {
		if empty {
			return types.StatusEmpty
		}
		return types.InOutInput
	}
	lumpDate := func(months int) types.DateRec {
		return types.NewDateRec(2024+months/12, time.Month(months%12+1), 1)
	}
	perDates := func(perYr, n int) (types.DateRec, types.DateRec) {
		totM := n * (12 / perYr)
		return types.NewDateRec(2024, time.January, 1),
			types.NewDateRec(2024+totM/12, time.Month(totM%12+1), 1)
	}
	asOfPlanted := types.NewDateRec(2024, time.January, 1)
	// Deliberately-wrong seeds so date solves must actually converge.
	lumpDateSeed := types.NewDateRec(2025, time.January, 1)
	asOfSeed := types.NewDateRec(2023, time.January, 1)

	// `multi RATE l<months>=<amt> ... p<amt>:<peryr>:<n> ...` forward oracle.
	tokens := func(rate float64, ls []lumpRow, ps []perRow) []string {
		toks := []string{"multi", strconv.FormatFloat(rate, 'f', 10, 64)}
		for _, l := range ls {
			toks = append(toks, "l"+strconv.Itoa(l.months)+"="+strconv.FormatFloat(l.amt, 'f', 6, 64))
		}
		for _, p := range ps {
			toks = append(toks, "p"+strconv.FormatFloat(p.amt, 'f', 6, 64)+":"+strconv.Itoa(p.perYr)+":"+strconv.Itoa(p.n))
		}
		return toks
	}
	dosForward := func(rate float64, ls []lumpRow, ps []perRow) (float64, bool) {
		args := tokens(rate, ls, ps)
		for try := 0; try < 6; try++ {
			out, err := exec.Command(pvOracleBin(), args...).Output()
			if err != nil {
				continue
			}
			if v, ok := parsePV(out); ok {
				return v, true
			}
		}
		return 0, false
	}

	// Build a Go PVInput; exactly one field is blanked per kind/idx. kinds:
	// "rate" / "lump" (amount) / "per" (amount) / "lumpdate" / "asof" / "" (fwd).
	goInput := func(rate, target float64, ls []lumpRow, ps []perRow, kind string, idx int) PVInput {
		var lumps []LumpSumPayment
		for i, l := range ls {
			d := lumpDate(l.months)
			if kind == "lumpdate" && i == idx {
				d = lumpDateSeed // wrong seed -> engine solves the date
			}
			lumps = append(lumps, LumpSumPayment{
				DateStatus: statusIf(kind == "lumpdate" && i == idx), Date: d,
				AmtStatus: statusIf(kind == "lump" && i == idx), Amt: l.amt,
				ValStatus: types.StatusEmpty})
		}
		var pers []PeriodicPayment
		for i, p := range ps {
			fd, td := perDates(p.perYr, p.n)
			pers = append(pers, PeriodicPayment{
				FromDateStatus: types.InOutInput, FromDate: fd,
				ToDateStatus: types.InOutInput, ToDate: td,
				PerYrStatus: types.InOutInput, PerYr: p.perYr,
				AmtStatus: statusIf(kind == "per" && i == idx), Amt: p.amt,
				COLAStatus: types.StatusEmpty, ValStatus: types.StatusEmpty})
		}
		asOf := asOfPlanted
		if kind == "asof" {
			asOf = asOfSeed
		}
		return PVInput{
			LumpSums: lumps, Periodics: pers,
			PresVal: PresValLine{
				AsOfStatus: statusIf(kind == "asof"), AsOf: asOf,
				R:              RateEntry{Status: statusIf(kind == "rate"), Rate: rate, PerYr: 1},
				SumValueStatus: statusIf(kind == ""), SumValue: target},
			Settings: PVSettings{Basis: types.Basis360, PerYr: 1, YrDays: 360, YrInv: 1.0 / 360, COLAMonth: types.COLAAnnual},
		}
	}
	goSolveScalar := func(rate, target float64, ls []lumpRow, ps []perRow, kind string, idx int) (float64, bool) {
		res := Calculate(goInput(rate, target, ls, ps, kind, idx))
		if res.Err != nil {
			return 0, false
		}
		switch kind {
		case "rate":
			return res.Rate, true
		case "lump":
			if idx < len(res.LumpSums) {
				return res.LumpSums[idx].Amt, true
			}
		case "per":
			if idx < len(res.Periodics) {
				return res.Periodics[idx].Amt, true
			}
		}
		return 0, false
	}
	goSolveDate := func(rate, target float64, ls []lumpRow, ps []perRow, kind string, idx int) (types.DateRec, bool) {
		res := Calculate(goInput(rate, target, ls, ps, kind, idx))
		if res.Err != nil {
			return types.DateRec{}, false
		}
		switch kind {
		case "lumpdate":
			if idx < len(res.LumpSums) {
				return res.LumpSums[idx].Date, true
			}
		case "asof":
			return res.AsOf, true
		}
		return types.DateRec{}, false
	}

	nSeeds := 60
	if v := os.Getenv("FUZZER2_CASES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			nSeeds = n
		}
	}

	type slot struct {
		kind string
		idx  int
	}
	isDate := func(kind string) bool { return kind == "lumpdate" || kind == "asof" }

	cases, flaked := 0, 0
	for c := 0; c < nSeeds; c++ {
		// Rate ≤ 12% and horizons ≤ 15yr keep every date solve well-conditioned
		// (PV stays sensitive to each date). Quantize the rate to 6dp so the
		// oracle (10dp) and Go receive an identical value.
		rate := math.Round((0.02+rng.Float64()*0.10)*1e6) / 1e6
		nL := 1 + rng.Intn(3) // 1..3 lumps
		nP := rng.Intn(3)     // 0..2 periodics
		var ls []lumpRow
		for j := 0; j < nL; j++ {
			ls = append(ls, lumpRow{months: 6 + rng.Intn(174), // 0.5..15yr
				amt: math.Round((1000+rng.Float64()*59000)*100) / 100})
		}
		var ps []perRow
		for j := 0; j < nP; j++ {
			perYr := []int{1, 2, 4, 12}[rng.Intn(4)]
			ps = append(ps, perRow{perYr: perYr, n: 2 + rng.Intn(15*perYr), // ≤15yr
				amt: math.Round((200+rng.Float64()*4800)*100) / 100})
		}

		// Rotatable slots: shared rate + as-of date + each lump amount & date +
		// each periodic amount.
		slots := []slot{{"rate", 0}, {"asof", 0}}
		for j := 0; j < nL; j++ {
			slots = append(slots, slot{"lump", j}, slot{"lumpdate", j})
		}
		for j := 0; j < nP; j++ {
			slots = append(slots, slot{"per", j})
		}

		target, ok := dosForward(rate, ls, ps)
		if !ok || target <= 0 {
			flaked++
			continue
		}

		prev := -1
		steps := 0
		aborted := false
		for s := 0; s < 3; s++ {
			si := rng.Intn(len(slots))
			for si == prev {
				si = rng.Intn(len(slots))
			}
			sl := slots[si]

			if isDate(sl.kind) {
				var planted types.DateRec
				if sl.kind == "asof" {
					planted = asOfPlanted
				} else {
					planted = lumpDate(ls[sl.idx].months)
				}
				gd, gok := goSolveDate(rate, target, ls, ps, sl.kind, sl.idx)
				if !gok {
					t.Errorf("case %d step %d solve %s[%d]: Go FAILED (rate=%.6f nL=%d nP=%d target=%.6f)",
						c, s, sl.kind, sl.idx, rate, nL, nP, target)
					aborted = true
					break
				}
				dd := math.Abs(gd.Time.Sub(planted.Time).Hours() / 24)
				if dd > 2.5 {
					t.Errorf("case %d step %d solve %s[%d]: Go=%s planted=%s (%.1f days) [rate=%.6f nL=%d nP=%d]",
						c, s, sl.kind, sl.idx, gd.Time.Format("2006-01-02"), planted.Time.Format("2006-01-02"), dd, rate, nL, nP)
				}
				// Dates are not hardened (worksheet stays DOS-canonical/integer-month).
				prev = si
				steps++
				continue
			}

			// scalar unknown
			var planted float64
			switch sl.kind {
			case "rate":
				planted = rate
			case "lump":
				planted = ls[sl.idx].amt
			case "per":
				planted = ps[sl.idx].amt
			}
			gx, gok := goSolveScalar(rate, target, ls, ps, sl.kind, sl.idx)
			if !gok {
				t.Errorf("case %d step %d solve %s[%d]: Go FAILED (rate=%.6f nL=%d nP=%d target=%.6f)",
					c, s, sl.kind, sl.idx, rate, nL, nP, target)
				aborted = true
				break
			}
			vtol := math.Max(5e-3, 5e-6*math.Abs(planted))
			if sl.kind == "rate" {
				vtol = 1e-5
			}
			if d := gx - planted; d > vtol || d < -vtol {
				t.Errorf("case %d step %d solve %s[%d]: Go=%.8g planted=%.8g (Δ=%+.3g, tol=%.3g) [rate=%.6f nL=%d nP=%d]",
					c, s, sl.kind, sl.idx, gx, planted, d, vtol, rate, nL, nP)
			}
			switch sl.kind {
			case "rate":
				rate = gx
			case "lump":
				ls[sl.idx].amt = gx
			case "per":
				ps[sl.idx].amt = gx
			}
			nt, ok := dosForward(rate, ls, ps)
			if !ok {
				flaked++
				aborted = true
				break
			}
			if rel := math.Abs(nt-target) / math.Max(1, math.Abs(target)); rel > 1e-5 {
				t.Errorf("case %d step %d after %s[%d]: DOS forward of Go's solution=%.6f != target=%.6f (rel %.2e) — backward solve not DOS-consistent [rate=%.6f nL=%d nP=%d]",
					c, s, sl.kind, sl.idx, nt, target, rel, rate, nL, nP)
			}
			target = nt // re-anchor to the DOS-canonical forward value
			prev = si
			steps++
		}
		if !aborted && steps == 3 {
			cases++
		}
	}

	if cases < 10 {
		t.Fatalf("only %d complete multi-row PV fuzz chains ran (want >=10) — oracle flaky or draws degenerate", cases)
	}
	t.Logf("completed %d full 3-step multi-row PV chains (lump+periodic; rotating rate/as-of/lump-amt/lump-date/per-amt); %d skipped on oracle flake", cases, flaked)
}
