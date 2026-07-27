package presentvalue

// TestDOSPVFuzzer5AllAdvancedOptions is the PV analogue of the amortization
// dos_fuzzer5: it fuzzes EVERY advanced option of the Present Value screen
// SIMULTANEOUSLY, with a 15% chance that any given option is left unused (i.e.
// falls back to its DOS default), and exercises MULTIPLE ROWS wherever the
// worksheet supports rows.
//
// Two oracle surfaces are drawn per case, because the DOS PV screen's option
// space does not fit through a single pv_oracle mode (and fpc is not available
// here, so the oracle's CLI grammar is frozen):
//
//   - `table` — the full worksheet under the Computational Settings and Table
//     Options dialogs: as-of date, day-count basis (360 / 365 / 365-360),
//     COLA month mode (annual / continuous / a specific calendar month),
//     table detail level (detail / detail+summary / summary-only), the
//     cumulative-summary month set, several lump-sum rows, several periodic
//     rows, per-row COLA, per-row payments-per-year (including the 26/52
//     day-stepped frequencies), off-grid day-of-month, and open-ended
//     ("forever") streams. This surface diffs the screen total AND every
//     single table line — date, payment, value, cumulative value — against
//     the REAL pvltable.pas MakePVLTable.
//
//   - `vr_multi` — the PV screen's ADVANCED toggle proper. In DOS,
//     PresentValueScreenUnit.pas:331/336/339 tie the Advanced button directly
//     to `pvlfancy`, which swaps the single Present Value line for the
//     variable-rate schedule (Effective Date / True Rate / Loan Rate / Yield).
//     This surface fuzzes the number of rate steps, their effective years and
//     rates, several lump rows, several periodic rows and per-row COLA, and
//     diffs the total plus each row's present value.
//
// Deliberate exclusions, and why:
//
//   - detail-only + a non-empty cumulative month set is NOT generated. DOS
//     pvltable.pas:511 calls PrintSummary whenever TimeIsRipe fires, regardless
//     of `cum`, so that combination WOULD emit dated summary lines — but
//     TableOptionsDlgUnit.pas OKBtnClick (line 105) clears cumset to [] on
//     every OK and only refills it on the Annual/Semi/Quarterly/Monthly
//     branches, which are exactly the branches that also set `cum` to a letter.
//     The combination is therefore unreachable from the UI, and table.go
//     normalizes it away (MakeTable: "DOS detail-only leaves cumset empty").
//     Fuzzing it would manufacture a divergence the product cannot reach.
//   - `cum` letters S/Q/M behave identically to Y in pvltable.pas (every test
//     is `cum in ['A'..'Z']` / `[' ','A'..'Z']` / `cum > ' '`), so the oracle's
//     detail/both/summary triple covers all four UI summary periods.
//   - Simple-vs-compound interest and df.c.exact are pinned inside the oracle
//     modes (SetupVRMulti forces simple := false) and cannot be reached
//     through the frozen CLI; they stay covered by the dedicated sweeps.
//
// Assertions, matching fuzzer3's one-directional hardness rule: Go must not
// SOLVE a worksheet DOS REFUSES (hard error). DOS-solves-Go-refuses and value
// or line divergences are hard errors too on this surface, because every draw
// is a fully-specified FORWARD worksheet — there is no ill-conditioned
// backward solve for either engine to legitimately give up on.
//
// Honors PERSENSE_REQUIRE_ORACLE, PERSENSE_FUZZ_N and PERSENSE_FUZZ_SEED.

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// pvFz5OmitProb is Nate's spec: "a 15% chance that the given option is not used".
const pvFz5OmitProb = 0.15

type pvFz5Table struct {
	screen float64
	lines  []oracleTableLine
	errMsg string // non-empty => the DOS screen refused the worksheet
}

// pvFz5RunTable execs `pv_oracle table`, retrying transient exec failures, and
// parses the screen total plus every T| line. ok=false means the oracle flaked
// (never produced a complete, parseable run).
func pvFz5RunTable(bin string, args []string) (pvFz5Table, bool) {
	for try := 0; try < 6; try++ {
		out, err := exec.Command(bin, append([]string{"table"}, args...)...).Output()
		if err != nil {
			continue
		}
		var res pvFz5Table
		sawPV, sawEnd, bad := false, false, false
		for _, ln := range strings.Split(string(out), "\n") {
			ln = strings.TrimSpace(ln)
			switch {
			case strings.HasPrefix(ln, "ERR"):
				res.errMsg = strings.TrimSpace(strings.TrimPrefix(ln, "ERR"))
				if res.errMsg == "" {
					res.errMsg = "refused"
				}
				return res, true
			case strings.HasPrefix(ln, "pv "):
				v, e := strconv.ParseFloat(strings.TrimSpace(ln[3:]), 64)
				if e != nil {
					bad = true
					continue
				}
				res.screen, sawPV = v, true
			case ln == "end":
				sawEnd = true
			case strings.HasPrefix(ln, "T|"):
				// DOS DateStr space-pads single-digit fields ("2/ 1/24"); close
				// the gaps so the date stays one whitespace token.
				body := strings.ReplaceAll(strings.TrimSpace(ln[2:]), "/ ", "/")
				if body == "" || strings.HasPrefix(body, "---") {
					continue
				}
				f := strings.Fields(body)
				num := func(i int) float64 {
					if i >= len(f) {
						bad = true
						return 0
					}
					v, e := strconv.ParseFloat(strings.ReplaceAll(f[i], ",", ""), 64)
					if e != nil {
						bad = true
					}
					return v
				}
				switch {
				case strings.HasPrefix(body, "Grand Totals:"):
					res.lines = append(res.lines, oracleTableLine{kind: "grand",
						payment: num(2), value: num(3), cumValue: num(4)})
				case strings.HasPrefix(body, "Subtotals:"):
					res.lines = append(res.lines, oracleTableLine{kind: "subtotal",
						payment: num(1), value: num(2), cumValue: num(3)})
				default:
					if len(f) < 4 {
						continue
					}
					res.lines = append(res.lines, oracleTableLine{kind: "payment",
						dateStr: f[0], payment: num(1), value: num(2), cumValue: num(3)})
				}
			}
		}
		if sawPV && sawEnd && !bad {
			return res, true
		}
	}
	return pvFz5Table{}, false
}

// pvFz5GoLines renders a Go TableResult in the oracle's line shape.
func pvFz5GoLines(got TableResult, req TableRequest) []oracleTableLine {
	out := make([]oracleTableLine, 0, len(got.Rows)+1)
	for _, r := range got.Rows {
		kind := "payment"
		if r.Kind == TableRowSubtotal {
			kind = "subtotal"
			if req.Detail == TableSummary {
				kind = "payment" // summary-only lines print dated, like payments
			}
		}
		ol := oracleTableLine{kind: kind, payment: r.Payment, value: r.Value, cumValue: r.CumValue}
		if r.HasDate {
			ol.dateStr = dosDateStr(r.Date)
		}
		out = append(out, ol)
	}
	return append(out, oracleTableLine{kind: "grand",
		payment: got.GrandPayment, value: got.GrandValue, cumValue: got.GrandValue})
}

func TestDOSPVFuzzer5AllAdvancedOptions(t *testing.T) {
	bin := pvOracleBin()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("PV oracle required but not present (%s)", bin)
		}
		t.Skip("PV oracle not present")
	}
	rng := rand.New(rand.NewSource(fuzzSeed(0x70763500))) // "pv5"

	N := 200
	if s := os.Getenv("PERSENSE_FUZZ_N"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v > 0 {
			N = v
		}
	}

	// --- draw helpers ------------------------------------------------------
	// used reports whether a given advanced option is exercised on this case.
	used := func() bool { return rng.Float64() >= pvFz5OmitProb }
	q6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	cents := func(x float64) float64 { return math.Round(x*100) / 100 }
	f10 := func(x float64) string { return strconv.FormatFloat(x, 'f', 10, 64) }
	f2 := func(x float64) string { return strconv.FormatFloat(x, 'f', 2, 64) }
	// mkDate clamps the day into the month, so an "off-grid" day-of-month draw
	// never produces an impossible date (DOS would silently normalize it).
	mkDate := func(y int, m time.Month, d int) types.DateRec {
		last := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1).Day()
		if d > last {
			d = last
		}
		if d < 1 {
			d = 1
		}
		return types.NewDateRec(y, m, d)
	}
	addMonths := func(d types.DateRec, n int, day int) types.DateRec {
		base := time.Date(d.Time.Year(), d.Time.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, n, 0)
		return mkDate(base.Year(), base.Month(), day)
	}
	dmy := func(d types.DateRec) string {
		return fmt.Sprintf("%d.%d.%d", d.Time.Day(), int(d.Time.Month()), d.Time.Year())
	}
	// A payment amount; ~10% of rows are outflows (negative), which the DOS
	// worksheet accepts and the table must sign-carry through the cumulative.
	amount := func() float64 {
		v := cents(50 + rng.Float64()*180_000)
		if rng.Float64() < 0.10 {
			v = -v
		}
		return v
	}

	var (
		tableCases, vrCases int
		refusedBoth         int
		flaked              int
		fails               int
		linesDiffed         int // table lines actually compared, DOS vs Go
		rowsDiffed          int // variable-rate row PVs actually compared
	)
	report := func(format string, a ...any) {
		fails++
		if fails <= 25 {
			t.Errorf(format, a...)
		}
	}
	// A flake is silent lost coverage, so surface the first few labels: a
	// systematically malformed argument set would otherwise look like a pass.
	flake := func(label string) {
		flaked++
		if flaked <= 5 {
			t.Logf("oracle flake: [%s]", label)
		}
	}

	for c := 0; c < N; c++ {
		// The Advanced toggle (pvlfancy / variable-rate schedule) selects the
		// oracle surface: it replaces the single Present Value line, so it
		// cannot coexist with the table surface's fixed-rate options.
		if rng.Intn(2) == 0 {
			// ============================ table surface ====================
			rate := q6(0.004 + rng.Float64()*0.28)

			// OPTION: as-of date (default 2024-01-01).
			asOf := types.NewDateRec(2024, time.January, 1)
			asOfTok := ""
			if used() {
				asOf = mkDate(2020+rng.Intn(9), time.Month(1+rng.Intn(12)), 1+rng.Intn(28))
				asOfTok = "asof=" + dmy(asOf)
			}
			// OPTION: day-count basis (default 30/360).
			basis, basisTok := types.Basis360, "360"
			if used() {
				switch rng.Intn(2) {
				case 0:
					basis, basisTok = types.Basis365, "365"
				default:
					basis, basisTok = types.Basis365360, "365360"
				}
			}
			// OPTION: COLA month mode (default annual-stepped).
			colaMonth, colaTok := types.COLAAnnual, "ann"
			if used() {
				if rng.Intn(2) == 0 {
					colaMonth, colaTok = types.COLAContinuous, "cnt"
				} else {
					m := 1 + rng.Intn(12)
					colaMonth, colaTok = byte(m), strconv.Itoa(m)
				}
			}
			// OPTION: table detail level + its cumulative month set. The two
			// move together — see the file header for why detail-only never
			// carries a month set.
			detail, detailTok := TableDetailOnly, "detail"
			cumTok := "none"
			var cumMonths [13]bool
			if used() {
				if rng.Intn(2) == 0 {
					detail, detailTok = TableDetailBoth, "both"
				} else {
					detail, detailTok = TableSummary, "summary"
				}
				// The four UI-reachable month sets (TableOptionsDlgUnit.pas):
				// Annual = one month, Semi = two 6 apart, Quarterly = four 3
				// apart, Monthly = all twelve.
				anchor := 1 + rng.Intn(12)
				var ms []int
				switch rng.Intn(4) {
				case 0:
					ms = []int{anchor}
				case 1:
					ms = []int{anchor, (anchor+5)%12 + 1}
				case 2:
					for k := 0; k < 4; k++ {
						ms = append(ms, (anchor-1+3*k)%12+1)
					}
				default:
					for k := 1; k <= 12; k++ {
						ms = append(ms, k)
					}
				}
				strs := make([]string, len(ms))
				for i, m := range ms {
					cumMonths[m] = true
					strs[i] = strconv.Itoa(m)
				}
				cumTok = strings.Join(strs, ",")
			}
			// OPTION: off-grid day-of-month (default: every row on day 1).
			offGrid := used()
			day := func() int {
				if !offGrid {
					return 1
				}
				return 1 + rng.Intn(31)
			}

			var lumps []LumpSumPayment
			var pers []PeriodicPayment
			var rowToks []string

			// OPTION: lump-sum rows at all; OPTION: MULTIPLE lump rows.
			if used() {
				n := 1
				if used() {
					n = 2 + rng.Intn(3) // 2..4 rows
				}
				for i := 0; i < n; i++ {
					d := addMonths(asOf, -48+rng.Intn(288), day())
					amt := amount()
					lumps = append(lumps, tblLump(d.Time.Year(), d.Time.Month(), d.Time.Day(), amt))
					rowToks = append(rowToks, "lump="+dmy(d)+":"+f2(amt))
				}
			}
			// OPTION: periodic rows at all; OPTION: MULTIPLE periodic rows.
			wantPer := used()
			if !wantPer && len(lumps) == 0 {
				wantPer = true // an empty worksheet is not a worksheet
			}
			if wantPer {
				n := 1
				if used() {
					n = 2 + rng.Intn(2) // 2..3 rows
				}
				for i := 0; i < n; i++ {
					// OPTION: payments per year (default 12).
					perYr := 12
					if used() {
						perYr = []int{1, 2, 3, 4, 6, 12, 26, 52}[rng.Intn(8)]
					}
					// OPTION: an explicit Through date. Unused => the stream
					// runs forever (DOS `latest`), which the table cuts at 50
					// years — bounded here by dropping to a slow frequency.
					forever := !used()
					if forever && perYr > 4 {
						perYr = []int{1, 2, 4}[rng.Intn(3)]
					}
					from := addMonths(asOf, -24+rng.Intn(144), day())
					to := types.LatestDate()
					if !forever {
						maxH := 400 * 12 / perYr
						if maxH > 600 {
							maxH = 600
						}
						if maxH < 2 {
							maxH = 2
						}
						to = addMonths(from, 1+rng.Intn(maxH), from.Time.Day())
					}
					// OPTION: per-row COLA (default none). Held strictly below
					// the discount rate so a long/forever stream converges.
					cola := 0.0
					if used() {
						cola = q6(rng.Float64() * rate * 0.85)
					}
					amt := amount()
					pers = append(pers, tblPer(
						from.Time.Year(), from.Time.Month(), from.Time.Day(),
						to.Time.Year(), to.Time.Month(), to.Time.Day(),
						perYr, amt, cola))
					rowToks = append(rowToks, fmt.Sprintf("per=%s:%s:%d:%s:%s",
						dmy(from), dmy(to), perYr, f2(amt), f10(cola)))
				}
			}

			args := []string{f10(rate), basisTok, detailTok, cumTok, colaTok}
			if asOfTok != "" {
				args = append(args, asOfTok)
			}
			args = append(args, rowToks...)
			label := "table " + strings.Join(args, " ")

			dos, ok := pvFz5RunTable(bin, args)
			if !ok {
				flake(label)
				continue
			}

			in := tblInput(rate, basis, colaMonth, asOf, lumps, pers)
			req := TableRequest{Detail: detail, SummaryMonths: cumMonths}
			got := MakeTable(in, req)

			if dos.errMsg != "" {
				if got.Err == nil {
					report("Go SOLVES where DOS REFUSES (%q): screen=%.6f | [%s]",
						dos.errMsg, got.ScreenValue, label)
				} else {
					refusedBoth++
				}
				continue
			}
			if got.Err != nil {
				report("DOS SOLVES (pv=%.6f) where Go REFUSES (%v) | [%s]", dos.screen, got.Err, label)
				continue
			}
			tableCases++

			if tol := math.Max(0.01, 1e-7*math.Abs(dos.screen)); math.Abs(got.ScreenValue-dos.screen) > tol {
				report("screen value: Go %.6f vs DOS %.6f (Δ%.6f) | [%s]",
					got.ScreenValue, dos.screen, got.ScreenValue-dos.screen, label)
			}
			goLines := pvFz5GoLines(got, req)
			if len(goLines) != len(dos.lines) {
				report("table line count: Go %d vs DOS %d | [%s]", len(goLines), len(dos.lines), label)
				continue
			}
			lineFails := 0
			linesDiffed += len(dos.lines)
			for i := range dos.lines {
				w, g := dos.lines[i], goLines[i]
				if lineFails >= 3 {
					break // one case should not drown the log
				}
				if w.kind != g.kind {
					report("line %d kind: Go %s vs DOS %s | [%s]", i, g.kind, w.kind, label)
					lineFails++
					continue
				}
				if w.kind == "payment" && g.dateStr != "" &&
					normDateTok(w.dateStr) != normDateTok(g.dateStr) {
					report("line %d date: Go %s vs DOS %s | [%s]", i, g.dateStr, w.dateStr, label)
					lineFails++
				}
				// The oracle prints 2dp: a half cent, plus float slack scaled
				// to the magnitude of the accumulated figure.
				tolOf := func(x float64) float64 { return 0.0051 + 1e-9*math.Abs(x) }
				if math.Abs(w.payment-g.payment) > tolOf(w.payment) {
					report("line %d payment: Go %.4f vs DOS %.2f | [%s]", i, g.payment, w.payment, label)
					lineFails++
				}
				if math.Abs(w.value-g.value) > tolOf(w.value) {
					report("line %d value: Go %.4f vs DOS %.2f | [%s]", i, g.value, w.value, label)
					lineFails++
				}
				if math.Abs(w.cumValue-g.cumValue) > tolOf(w.cumValue) {
					report("line %d cum: Go %.4f vs DOS %.2f | [%s]", i, g.cumValue, w.cumValue, label)
					lineFails++
				}
			}
			continue
		}

		// ============================ vr_multi surface =====================
		// OPTION: MULTIPLE rate steps (unused => a single step, i.e. a flat
		// rate expressed through the Advanced schedule).
		//
		// The FIRST rate line's effective date is NOT fuzzable: both DOS screens
		// hard-lock it to `earliest` as a computed cell — PVLXSCRN.pas:83 and
		// PresentValueScreenUnit.pas:321-322, `cc[1]^.date := earliest;
		// cc[1]^.datestatus := defp` ("not sure why, but these are locked in
		// like this"). The lock is load-bearing: PVLUTIL.pas ValueOfOnePayment
		// scans `repeat inc(k) until (DateComp(cc[k]^.date, d^.xasof) > 0) ...;
		// dec(k)` and then dereferences cc[k], so a first rate line dated AFTER
		// the as-of leaves k=0 and reads cc[0] — off the front of the 1..maxlines
		// array. The oracle's SetupVRMulti dates every line including the first,
		// so drawing an out-of-order first year makes it die on an access
		// violation. That state is unreachable in the product, so the fuzzer
		// pins step 0 well before the as-of (the `earliest` stand-in the existing
		// TestDOSVRMultiRowSweep also uses) and fuzzes the rest.
		nRates := 1
		if used() {
			nRates = 2 + rng.Intn(4) // 2..5 steps
		}
		steps := []rateStep{{year: 2000, rate: q6(0.005 + rng.Float64()*0.22)}}
		yr := 2024
		for len(steps) < nRates {
			yr += 1 + rng.Intn(6)
			steps = append(steps, rateStep{year: yr, rate: q6(0.005 + rng.Float64()*0.22)})
		}
		minRate := steps[0].rate
		for _, s := range steps {
			if s.rate < minRate {
				minRate = s.rate
			}
		}

		var vlumps []vrLump
		var vpers []vrPer
		// OPTION: lump rows; OPTION: MULTIPLE lump rows.
		if used() {
			n := 1
			if used() {
				n = 2 + rng.Intn(3)
			}
			for i := 0; i < n; i++ {
				vlumps = append(vlumps, vrLump{months: rng.Intn(300), amt: amount()})
			}
		}
		wantPer := used()
		if !wantPer && len(vlumps) == 0 {
			wantPer = true
		}
		if wantPer {
			n := 1
			if used() {
				n = 2 + rng.Intn(2)
			}
			for i := 0; i < n; i++ {
				// OPTION: payments per year. vr_multi derives the Through date
				// as n*(12 div PERYR) months, so the frequency must divide 12.
				perYr := 12
				if used() {
					perYr = []int{1, 2, 3, 4, 6, 12}[rng.Intn(6)]
				}
				// OPTION: per-row COLA, kept below the lowest scheduled rate.
				cola := 0.0
				if used() {
					cola = q6(rng.Float64() * minRate * 0.85)
				}
				vpers = append(vpers, vrPer{amt: amount(), perYr: perYr,
					n: 1 + rng.Intn(60), cola: cola})
			}
		}

		var lbl strings.Builder
		lbl.WriteString("vr_multi " + strconv.Itoa(len(steps)))
		for _, s := range steps {
			fmt.Fprintf(&lbl, " %d %s", s.year, f10(s.rate))
		}
		for _, l := range vlumps {
			fmt.Fprintf(&lbl, " l%d=%s", l.months, f2(l.amt))
		}
		for _, p := range vpers {
			fmt.Fprintf(&lbl, " p%s:%d:%d", f2(p.amt), p.perYr, p.n)
			if p.cola != 0 {
				fmt.Fprintf(&lbl, ":%s", f10(p.cola))
			}
		}
		label := lbl.String()

		dosVal, dosRows, ok := runVRMultiOracle(steps, vlumps, vpers)
		if !ok {
			flake(label)
			continue
		}
		res := goVRMultiResult(steps, vlumps, vpers)
		if res.Err != nil {
			report("DOS SOLVES (pv=%.6f) where Go REFUSES (%v) | [%s]", dosVal, res.Err, label)
			continue
		}
		vrCases++
		if tol := math.Max(0.01, 1e-7*math.Abs(dosVal)); math.Abs(res.SumValue-dosVal) > tol {
			report("VR total: Go %.6f vs DOS %.6f (Δ%.6f) | [%s]",
				res.SumValue, dosVal, res.SumValue-dosVal, label)
		}
		if len(dosRows.lump) != len(res.LumpSums) || len(dosRows.per) != len(res.Periodics) {
			report("VR row counts: Go %d/%d vs DOS %d/%d | [%s]",
				len(res.LumpSums), len(res.Periodics), len(dosRows.lump), len(dosRows.per), label)
			continue
		}
		rowsDiffed += len(dosRows.lump) + len(dosRows.per)
		for i, w := range dosRows.lump {
			g := res.LumpSums[i].Val
			if tol := math.Max(0.01, 1e-7*math.Abs(w)); math.Abs(g-w) > tol {
				report("VR lump row %d: Go %.6f vs DOS %.6f | [%s]", i+1, g, w, label)
			}
		}
		for i, w := range dosRows.per {
			g := res.Periodics[i].Val
			if tol := math.Max(0.01, 1e-7*math.Abs(w)); math.Abs(g-w) > tol {
				report("VR per row %d: Go %.6f vs DOS %.6f | [%s]", i+1, g, w, label)
			}
		}
	}

	t.Logf("pv fuzzer5 (all advanced options, %.0f%% omit): %d table worksheets (%d lines diffed), "+
		"%d variable-rate worksheets (%d row PVs diffed), %d both-refused, %d oracle flakes, %d divergences",
		pvFz5OmitProb*100, tableCases, linesDiffed, vrCases, rowsDiffed, refusedBoth, flaked, fails)
	if tableCases+vrCases < N/3 {
		t.Fatalf("only %d/%d cases adjudicated — harness/oracle problem", tableCases+vrCases, N)
	}
	// A silent zero here would mean the surfaces ran but compared nothing.
	if tableCases > 0 && linesDiffed == 0 {
		t.Fatalf("%d table worksheets but 0 lines diffed — the line comparison is not running", tableCases)
	}
	if vrCases > 0 && rowsDiffed == 0 {
		t.Fatalf("%d variable-rate worksheets but 0 row PVs diffed — the row comparison is not running", vrCases)
	}
}
