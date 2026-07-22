package presentvalue

// Differential validation of the PV payment table (table.go MakeTable) against
// the REAL DOS table code: pv_oracle's `table` mode calls pvltable.pas
// MakePVLTable on the same worksheet and dumps every line. We compare
// line-for-line: payment dates, payment amounts, per-payment values, cumulative
// values, subtotal rows, and the grand totals — across bases, COLA modes,
// same-date merging, the three detail levels, and the 50-year forever cutoff.
//
// The v3 product build compiles the table WITHOUT the ACTU define
// (fold_in_life = const false), so the oracle covers the standard columns; the
// life-mode columns are validated separately by internal consistency
// (table_life_test.go... — see TestPVTableLifeConsistency below).

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/finance/actuarial"
	"github.com/persense/persense-port/internal/types"
)

type oracleTableLine struct {
	kind     string // "payment", "subtotal", "grand"
	dateStr  string // M/D/YY as printed (payments and dated summary lines)
	payment  float64
	value    float64
	cumValue float64
}

// runOracleTable invokes `pv_oracle table` and parses the T| lines.
// Skips the test when the oracle binary is absent (unless PERSENSE_REQUIRE_ORACLE).
func runOracleTable(t *testing.T, args []string) (float64, []oracleTableLine) {
	t.Helper()
	bin := pvOracleBin()
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PERSENSE_REQUIRE_ORACLE") != "" {
			t.Fatalf("PERSENSE_REQUIRE_ORACLE set but oracle missing at %s", bin)
		}
		t.Skipf("pv_oracle not built (%s); run legacy/oracle/build_linux.sh TARGET=pv_oracle", bin)
	}
	out, err := exec.Command(bin, append([]string{"table"}, args...)...).Output()
	if err != nil {
		t.Fatalf("pv_oracle table %v: %v (out=%q)", args, err, out)
	}
	var screen float64
	var lines []oracleTableLine
	for _, ln := range strings.Split(string(out), "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "pv ") {
			screen, _ = strconv.ParseFloat(strings.Fields(ln)[1], 64)
			continue
		}
		if strings.HasPrefix(ln, "ERR") {
			t.Fatalf("oracle refused: %s", ln)
		}
		if !strings.HasPrefix(ln, "T|") {
			continue
		}
		body := strings.TrimSpace(ln[2:])
		if body == "" || strings.HasPrefix(body, "---") {
			continue
		}
		// DOS DateStr space-pads single-digit fields ("2/ 1/24"), which would
		// split the date across whitespace tokens — close the gaps first.
		body = strings.ReplaceAll(body, "/ ", "/")
		f := strings.Fields(body)
		switch {
		case strings.HasPrefix(body, "Grand Totals:"):
			// Grand Totals: <q> <v> <v>
			lines = append(lines, oracleTableLine{kind: "grand",
				payment: atofT(t, f[2]), value: atofT(t, f[3]), cumValue: atofT(t, f[4])})
		case strings.HasPrefix(body, "Subtotals:"):
			// (cum='Y' both-mode subtotal) Subtotals: <q> <v> <cum>
			lines = append(lines, oracleTableLine{kind: "subtotal",
				payment: atofT(t, f[1]), value: atofT(t, f[2]), cumValue: atofT(t, f[3])})
		default:
			// <M/ D/YY> <q> <v> <cum> — a payment line, or (cum='y'
			// summary-only) a dated summary line; the caller disambiguates by
			// the requested view.
			if len(f) < 4 {
				continue
			}
			lines = append(lines, oracleTableLine{kind: "payment", dateStr: f[0],
				payment: atofT(t, f[1]), value: atofT(t, f[2]), cumValue: atofT(t, f[3])})
		}
	}
	return screen, lines
}

func atofT(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		t.Fatalf("bad number %q", s)
	}
	return v
}

// dosDateStr formats a DateRec the way the DOS table prints it (M/ D/YY with
// single-digit fields space-padded, e.g. " 2/ 1/24") — then compacted for
// comparison against the whitespace-split token (fields collapse the pad).
func dosDateStr(d types.DateRec) string {
	y := d.Time.Year() % 100
	return fmt.Sprintf("%d/%d/%02d", int(d.Time.Month()), d.Time.Day(), y)
}

func normDateTok(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

// diffTable runs the same worksheet through Go MakeTable and the oracle and
// compares every line.
func diffTable(t *testing.T, input PVInput, req TableRequest, args []string) {
	t.Helper()
	screen, want := runOracleTable(t, args)
	got := MakeTable(input, req)
	if got.Err != nil {
		t.Fatalf("MakeTable errored: %v", got.Err)
	}
	if math.Abs(got.ScreenValue-screen) > 0.01 {
		t.Errorf("screen value: go %.6f vs oracle %.6f", got.ScreenValue, screen)
	}
	// The Go rows plus a final grand-totals record must match the oracle lines.
	goLines := make([]oracleTableLine, 0, len(got.Rows)+1)
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
		goLines = append(goLines, ol)
	}
	goLines = append(goLines, oracleTableLine{kind: "grand",
		payment: got.GrandPayment, value: got.GrandValue, cumValue: got.GrandValue})

	if len(goLines) != len(want) {
		t.Fatalf("line count: go %d vs oracle %d\ngo:  %+v\ndos: %+v", len(goLines), len(want), goLines, want)
	}
	for i := range want {
		w, g := want[i], goLines[i]
		if w.kind != g.kind {
			t.Errorf("line %d kind: go %s vs oracle %s", i, g.kind, w.kind)
			continue
		}
		if w.kind == "payment" && g.dateStr != "" &&
			normDateTok(w.dateStr) != normDateTok(g.dateStr) {
			t.Errorf("line %d date: go %s vs oracle %s", i, g.dateStr, w.dateStr)
		}
		// The oracle prints 2dp; compare at a half-cent.
		if math.Abs(w.payment-g.payment) > 0.005+1e-9 {
			t.Errorf("line %d payment: go %.4f vs oracle %.2f", i, g.payment, w.payment)
		}
		if math.Abs(w.value-g.value) > 0.005+1e-9 {
			t.Errorf("line %d value: go %.4f vs oracle %.2f", i, g.value, w.value)
		}
		if math.Abs(w.cumValue-g.cumValue) > 0.005+1e-9 {
			t.Errorf("line %d cum: go %.4f vs oracle %.2f", i, g.cumValue, w.cumValue)
		}
	}
}

// --- worksheet builders (mirror the oracle arg encoding) -------------------

func tblInput(rate float64, basis types.BasisType, colaMonth byte,
	asof types.DateRec, lumps []LumpSumPayment, pers []PeriodicPayment) PVInput {
	s := PVSettings{Basis: basis, PerYr: 12, COLAMonth: colaMonth}
	// DOS SetYrDays (INTSUTIL.pas:333): 365.25 for x365, 360 otherwise —
	// including x365_360 (actual-day count over a 360 denominator).
	switch basis {
	case types.Basis365:
		s.YrDays, s.YrInv = 365.25, 1/365.25
	default:
		s.YrDays, s.YrInv = 360, 1.0/360
	}
	return PVInput{
		LumpSums:  lumps,
		Periodics: pers,
		PresVal: PresValLine{
			AsOfStatus: types.InOutInput, AsOf: asof,
			R: RateEntry{Status: types.InOutInput, Rate: rate, PerYr: 1},
		},
		Settings: s,
	}
}

func tblLump(y int, m time.Month, d int, amt float64) LumpSumPayment {
	return LumpSumPayment{
		DateStatus: types.InOutInput, Date: types.NewDateRec(y, m, d),
		AmtStatus: types.InOutInput, Amt: amt,
	}
}

func tblPer(fy int, fm time.Month, fd, ty int, tm time.Month, td, peryr int, amt, cola float64) PeriodicPayment {
	p := PeriodicPayment{
		FromDateStatus: types.InOutInput, FromDate: types.NewDateRec(fy, fm, fd),
		ToDateStatus: types.InOutInput, ToDate: types.NewDateRec(ty, tm, td),
		PerYrStatus: types.InOutInput, PerYr: peryr,
		AmtStatus: types.InOutInput, Amt: amt,
	}
	if cola != 0 {
		p.COLAStatus = types.InOutInput
		p.COLA = cola
	}
	return p
}

// --- the differential cases ------------------------------------------------

func TestDOSPVTableDifferential(t *testing.T) {
	asof := types.NewDateRec(2024, time.January, 1)

	cases := []struct {
		name   string
		input  PVInput
		req    TableRequest
		oracle []string
	}{
		{
			"lump_plus_periodic_detail",
			tblInput(0.06, types.Basis360, types.COLAAnnual, asof,
				[]LumpSumPayment{tblLump(2025, time.June, 1, 20000)},
				[]PeriodicPayment{tblPer(2024, time.February, 1, 2025, time.January, 1, 12, 1000, 0)}),
			TableRequest{Detail: TableDetailOnly},
			[]string{"0.06", "360", "detail", "none", "ann",
				"asof=1.1.2024", "lump=1.6.2025:20000", "per=1.2.2024:1.1.2025:12:1000:0"},
		},
		{
			"same_date_merge",
			tblInput(0.06, types.Basis360, types.COLAAnnual, asof,
				[]LumpSumPayment{tblLump(2024, time.June, 1, 5000), tblLump(2024, time.June, 1, 2500)},
				[]PeriodicPayment{tblPer(2024, time.February, 1, 2025, time.June, 1, 4, 3000, 0)}),
			TableRequest{Detail: TableDetailOnly},
			[]string{"0.06", "360", "detail", "none", "ann",
				"asof=1.1.2024", "lump=1.6.2024:5000", "lump=1.6.2024:2500",
				"per=1.2.2024:1.6.2025:4:3000:0"},
		},
		{
			"cola_stepped_365",
			tblInput(0.06, types.Basis365, types.COLAAnnual, asof, nil,
				[]PeriodicPayment{tblPer(2024, time.February, 1, 2027, time.January, 1, 12, 1000, 0.03)}),
			TableRequest{Detail: TableDetailOnly},
			[]string{"0.06", "365", "detail", "none", "ann",
				"asof=1.1.2024", "per=1.2.2024:1.1.2027:12:1000:0.03"},
		},
		{
			"cola_continuous",
			tblInput(0.06, types.Basis360, types.COLAContinuous, asof, nil,
				[]PeriodicPayment{tblPer(2024, time.February, 1, 2026, time.January, 1, 12, 1000, 0.03)}),
			TableRequest{Detail: TableDetailOnly},
			[]string{"0.06", "360", "detail", "none", "cnt",
				"asof=1.1.2024", "per=1.2.2024:1.1.2026:12:1000:0.03"},
		},
		{
			"cola_month_specific_january",
			tblInput(0.06, types.Basis360, 1, asof, nil,
				[]PeriodicPayment{tblPer(2024, time.March, 15, 2026, time.December, 15, 12, 800, 0.04)}),
			TableRequest{Detail: TableDetailOnly},
			[]string{"0.06", "360", "detail", "none", "1",
				"asof=1.1.2024", "per=15.3.2024:15.12.2026:12:800:0.04"},
		},
		{
			"weekly_365360",
			tblInput(0.075, types.Basis365360, types.COLAAnnual, asof, nil,
				[]PeriodicPayment{tblPer(2024, time.January, 5, 2025, time.June, 27, 52, 610, 0)}),
			TableRequest{Detail: TableDetailOnly},
			[]string{"0.075", "365360", "detail", "none", "ann",
				"asof=1.1.2024", "per=5.1.2024:27.6.2025:52:610:0"},
		},
		{
			"subtotals_annual_both",
			tblInput(0.06, types.Basis360, types.COLAAnnual, asof,
				[]LumpSumPayment{tblLump(2024, time.December, 1, 85000)},
				[]PeriodicPayment{tblPer(2024, time.February, 1, 2026, time.June, 1, 12, 610, 0.03)}),
			TableRequest{Detail: TableDetailBoth, SummaryMonths: monthSet(1)},
			[]string{"0.06", "360", "both", "1", "ann",
				"asof=1.1.2024", "lump=1.12.2024:85000", "per=1.2.2024:1.6.2026:12:610:0.03"},
		},
		{
			"summary_only_quarterly",
			tblInput(0.06, types.Basis360, types.COLAAnnual, asof, nil,
				[]PeriodicPayment{tblPer(2024, time.February, 1, 2026, time.June, 1, 12, 610, 0)}),
			TableRequest{Detail: TableSummary, SummaryMonths: monthSet(1, 4, 7, 10)},
			[]string{"0.06", "360", "summary", "1,4,7,10", "ann",
				"asof=1.1.2024", "per=1.2.2024:1.6.2026:12:610:0"},
		},
		{
			"accumulation_past_payments",
			tblInput(0.06, types.Basis360, types.COLAAnnual, types.NewDateRec(2025, time.June, 15),
				[]LumpSumPayment{tblLump(2024, time.March, 1, 10000)},
				[]PeriodicPayment{tblPer(2024, time.February, 1, 2026, time.January, 1, 12, 500, 0)}),
			TableRequest{Detail: TableDetailOnly},
			[]string{"0.06", "360", "detail", "none", "ann",
				"asof=15.6.2025", "lump=1.3.2024:10000", "per=1.2.2024:1.1.2026:12:500:0"},
		},
		{
			"forever_50yr_cutoff",
			tblInput(0.08, types.Basis360, types.COLAAnnual, asof, nil,
				[]PeriodicPayment{tblPer(2024, time.February, 1,
					types.LatestDate().Time.Year(), types.LatestDate().Time.Month(), types.LatestDate().Time.Day(),
					1, 5000, 0)}),
			TableRequest{Detail: TableDetailOnly},
			[]string{"0.08", "360", "detail", "none", "ann",
				"asof=1.1.2024",
				fmt.Sprintf("per=1.2.2024:%d.%d.%d:1:5000:0",
					types.LatestDate().Time.Day(), int(types.LatestDate().Time.Month()), types.LatestDate().Time.Year())},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diffTable(t, tc.input, tc.req, tc.oracle)
		})
	}
}

func monthSet(months ...int) [13]bool {
	var s [13]bool
	for _, m := range months {
		s[m] = true
	}
	return s
}

// TestPVTableHelpExample17Shape pins the shape documented in the Windows help
// (PV_Tables.html): a weekly $610 stream valued at as-of 2/15/1993 — the first
// table line is the first payment discounted, and the grand-total value equals
// the running cumulative of the last line. (The help's full worksheet spans
// multiple COLA'd segments we don't reproduce verbatim; this pins the
// structural invariants the help demonstrates: first line = first payment,
// CumValue telescopes, Grand payment = Σ payments.)
func TestPVTableHelpExample17Shape(t *testing.T) {
	asof := types.NewDateRec(1993, time.February, 15)
	in := tblInput(0.075, types.Basis365, types.COLAAnnual, asof,
		[]LumpSumPayment{tblLump(1993, time.December, 1, 85000)},
		[]PeriodicPayment{tblPer(1993, time.February, 15, 1994, time.November, 28, 52, 610, 0.03)})
	res := MakeTable(in, TableRequest{Detail: TableDetailOnly})
	if res.Err != nil {
		t.Fatalf("MakeTable: %v", res.Err)
	}
	if len(res.Rows) == 0 {
		t.Fatal("no rows")
	}
	first := res.Rows[0]
	if !first.HasDate || dosDateStr(first.Date) != "2/15/93" || math.Abs(first.Payment-610) > 1e-9 {
		t.Errorf("first line = %+v, want 2/15/93 610.00 (help sample: '2/15/93 610.00 696.77 696.77' shape)", first)
	}
	if math.Abs(first.Value-first.CumValue) > 1e-9 {
		t.Errorf("first line cum %v != value %v", first.CumValue, first.Value)
	}
	var sumQ, sumV float64
	prevCum := 0.0
	for _, r := range res.Rows {
		sumQ += r.Payment
		sumV += r.Value
		if math.Abs(r.CumValue-(prevCum+r.Value)) > 1e-6 {
			t.Fatalf("cum does not telescope at %v", r.Date)
		}
		prevCum = r.CumValue
	}
	if math.Abs(res.GrandPayment-sumQ) > 1e-6 || math.Abs(res.GrandValue-sumV) > 1e-6 {
		t.Errorf("grand totals (%.2f, %.2f) != sums (%.2f, %.2f)", res.GrandPayment, res.GrandValue, sumQ, sumV)
	}
}

// TestPVTableLifeConsistency validates the life-mode columns (outside the v3
// oracle build — fold_in_life is const false there) by internal consistency
// with the screen's actuarial engine: per-line Value = IfPaid × Prob, lines
// stay UN-merged on shared dates, the POD line carries the screen's PODValue,
// and the table's grand value matches the screen total (per-payment and screen
// paths coincide on the 360 basis).
func TestPVTableLifeConsistency(t *testing.T) {
	asof := types.NewDateRec(2024, time.January, 1)
	cfg := &actuarial.ActuarialConfig{
		Table1: actuarial.Persense1988Male(),
		DOB1:   types.NewDateRec(1958, time.March, 15),
		Now:    asof,
	}
	lump1 := tblLump(2027, time.June, 1, 100000)
	lump1.Act = actuarial.Living
	lump2 := tblLump(2027, time.June, 1, 40000)
	lump2.Act = actuarial.Dead
	per := tblPer(2024, time.February, 1, 2034, time.January, 1, 12, 2000, 0)
	per.Act = actuarial.Living
	in := tblInput(0.06, types.Basis360, types.COLAAnnual, asof,
		[]LumpSumPayment{lump1, lump2}, []PeriodicPayment{per})
	in.Actuarial = cfg
	res := MakeTable(in, TableRequest{Detail: TableDetailOnly})
	if res.Err != nil {
		t.Fatalf("MakeTable: %v", res.Err)
	}
	if !res.LifeMode {
		t.Fatal("expected life mode")
	}
	// The two same-date lumps must NOT merge (different contingencies).
	seen := 0
	var sumV float64
	for _, r := range res.Rows {
		if r.HasDate && dosDateStr(r.Date) == "6/1/27" {
			seen++
		}
		if math.Abs(r.Value-r.IfPaid*r.Prob) > 1e-6 {
			t.Errorf("line %v: Value %.6f != IfPaid %.6f x Prob %.6f", r.Date, r.Value, r.IfPaid, r.Prob)
		}
		sumV += r.Value
	}
	if seen != 3 {
		t.Errorf("same-date life rows merged: got %d lines on 6/1/27, want 3 (2 lumps + the monthly payment, kept separate)", seen)
	}
	if math.Abs(res.GrandValue-sumV) > 1e-6 {
		t.Errorf("grand value %.4f != sum of line values %.4f", res.GrandValue, sumV)
	}
	if math.Abs(res.GrandValue-res.ScreenValue) > 0.02 {
		t.Errorf("life-mode 360-basis table %.4f should match screen %.4f", res.GrandValue, res.ScreenValue)
	}
	if math.Abs(res.GrandProb-res.GrandValue/res.GrandIfPaid) > 1e-9 {
		t.Errorf("grand prob %.6f != sum(v)/sum(ifpd)", res.GrandProb)
	}
}
