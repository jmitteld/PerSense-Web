package presentvalue

// zzp5_test.go — INVESTIGATION HARNESS for PV fuzzer5 divergences. Scratch: not
// a gate, not part of the suite's assertions. Every test here is driven by the
// P5 environment variable holding the exact oracle argument line the PV fuzzer
// printed (the part inside the square brackets, minus the leading "table"),
// e.g.
//
//	P5="0.0357760000 360 both 4,10 cnt asof=11.6.2026 lump=3.4.2024:71322.19 \
//	    per=8.2.2027:8.9.2054:1:124983.44:0.0000000000" \
//	  go test ./internal/finance/presentvalue/ -run TestP5 -v
//
// It exists for the same reason zzm5_test.go does in the amortization package:
// "the screen total differs by $X" is not enough to walk the Pascal against.
// What identifies the divergent DOS routine is WHICH ROW carries the delta, so
// TestP5Ablate re-runs the worksheet one row at a time (and with each row
// dropped) to localize it before any Pascal is opened.

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/persense/persense-port/internal/types"
)

// p5Case is a parsed table-surface argument line.
type p5Case struct {
	rate    float64
	basis   types.BasisType
	basisTk string
	detail  string
	cumMon  [13]bool
	cumTk   string
	cola    byte
	colaTk  string
	asOf    types.DateRec
	asOfTk  string
	lumps   []LumpSumPayment
	pers    []PeriodicPayment
	rowToks []string // one token per row, in lumps-then-pers order
}

func p5Date(t *testing.T, s string) types.DateRec {
	t.Helper()
	f := strings.Split(s, ".")
	if len(f) != 3 {
		t.Fatalf("bad d.m.y date %q", s)
	}
	d, _ := strconv.Atoi(f[0])
	m, _ := strconv.Atoi(f[1])
	y, _ := strconv.Atoi(f[2])
	return types.NewDateRec(y, time.Month(m), d)
}

func p5Num(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("bad number %q: %v", s, err)
	}
	return v
}

// p5Parse is the inverse of dos_pv_fuzzer5_test.go's table-surface generator.
func p5Parse(t *testing.T, line string) *p5Case {
	t.Helper()
	args := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "table "))
	if len(args) < 5 {
		t.Fatalf("P5 needs at least RATE BASIS DETAIL CUM COLA, got %q", line)
	}
	c := &p5Case{
		rate:   p5Num(t, args[0]),
		asOf:   types.NewDateRec(2024, time.January, 1),
		colaTk: args[4],
		cumTk:  args[3],
	}
	switch args[1] {
	case "360":
		c.basis = types.Basis360
	case "365":
		c.basis = types.Basis365
	case "365360":
		c.basis = types.Basis365360
	default:
		t.Fatalf("bad basis %q", args[1])
	}
	c.basisTk = args[1]
	switch args[2] {
	case "detail":
		c.detail = TableDetailOnly
	case "both":
		c.detail = TableDetailBoth
	case "summary":
		c.detail = TableSummary
	default:
		t.Fatalf("bad detail %q", args[2])
	}
	if args[3] != "none" {
		for _, s := range strings.Split(args[3], ",") {
			m, err := strconv.Atoi(s)
			if err != nil || m < 1 || m > 12 {
				t.Fatalf("bad cum month %q", s)
			}
			c.cumMon[m] = true
		}
	}
	switch args[4] {
	case "ann":
		c.cola = types.COLAAnnual
	case "cnt":
		c.cola = types.COLAContinuous
	default:
		m, err := strconv.Atoi(args[4])
		if err != nil || m < 1 || m > 12 {
			t.Fatalf("bad cola token %q", args[4])
		}
		c.cola = byte(m)
	}
	for _, tok := range args[5:] {
		switch {
		case strings.HasPrefix(tok, "asof="):
			c.asOf = p5Date(t, strings.TrimPrefix(tok, "asof="))
			c.asOfTk = tok
		case strings.HasPrefix(tok, "lump="):
			f := strings.Split(strings.TrimPrefix(tok, "lump="), ":")
			if len(f) != 2 {
				t.Fatalf("bad lump %q", tok)
			}
			d := p5Date(t, f[0])
			c.lumps = append(c.lumps, tblLump(d.Time.Year(), d.Time.Month(), d.Time.Day(), p5Num(t, f[1])))
			c.rowToks = append(c.rowToks, tok)
		case strings.HasPrefix(tok, "per="):
			f := strings.Split(strings.TrimPrefix(tok, "per="), ":")
			if len(f) != 5 {
				t.Fatalf("bad per %q", tok)
			}
			from, to := p5Date(t, f[0]), p5Date(t, f[1])
			perYr, err := strconv.Atoi(f[2])
			if err != nil {
				t.Fatalf("bad peryr in %q", tok)
			}
			c.pers = append(c.pers, tblPer(
				from.Time.Year(), from.Time.Month(), from.Time.Day(),
				to.Time.Year(), to.Time.Month(), to.Time.Day(),
				perYr, p5Num(t, f[3]), p5Num(t, f[4])))
			c.rowToks = append(c.rowToks, tok)
		default:
			t.Fatalf("unrecognized token %q", tok)
		}
	}
	return c
}

// args rebuilds the oracle argument vector, optionally restricted to a subset
// of rows (nil => every row).
func (c *p5Case) args(rows []int) []string {
	a := []string{strconv.FormatFloat(c.rate, 'f', 10, 64), c.basisTk,
		c.detail, c.cumTk, c.colaTk}
	if c.asOfTk != "" {
		a = append(a, c.asOfTk)
	}
	if rows == nil {
		return append(a, c.rowToks...)
	}
	for _, i := range rows {
		a = append(a, c.rowToks[i])
	}
	return a
}

// input builds the Go PVInput for a subset of rows (nil => every row). Row
// indices are into rowToks: lumps first, then periodics.
func (c *p5Case) input(rows []int) PVInput {
	lumps, pers := c.lumps, c.pers
	if rows != nil {
		lumps, pers = nil, nil
		for _, i := range rows {
			if i < len(c.lumps) {
				lumps = append(lumps, c.lumps[i])
			} else {
				pers = append(pers, c.pers[i-len(c.lumps)])
			}
		}
	}
	return tblInput(c.rate, c.basis, c.cola, c.asOf, lumps, pers)
}

func (c *p5Case) req() TableRequest {
	return TableRequest{Detail: c.detail, SummaryMonths: c.cumMon}
}

// p5Screen returns (dosScreen, goScreen, dosErr, goErr) for a row subset.
func p5Screen(t *testing.T, bin string, c *p5Case, rows []int) (float64, float64, string, error) {
	t.Helper()
	dos, ok := pvFz5RunTable(bin, c.args(rows))
	if !ok {
		t.Fatalf("oracle flaked on rows %v", rows)
	}
	got := MakeTable(c.input(rows), c.req())
	return dos.screen, got.ScreenValue, dos.errMsg, got.Err
}

// TestP5 runs the whole worksheet and reports the screen totals plus, when the
// screen agrees, the first divergent table line.
func TestP5(t *testing.T) {
	line := os.Getenv("P5")
	if line == "" {
		t.Skip("set P5 to an oracle table argument line")
	}
	bin := pvOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("PV oracle not present (%s)", bin)
	}
	c := p5Parse(t, line)

	dos, ok := pvFz5RunTable(bin, c.args(nil))
	if !ok {
		t.Fatalf("oracle flaked")
	}
	got := MakeTable(c.input(nil), c.req())
	if dos.errMsg != "" || got.Err != nil {
		t.Logf("DOS err=%q | Go err=%v", dos.errMsg, got.Err)
		return
	}
	t.Logf("SCREEN: Go %.6f vs DOS %.6f (Δ%.6f)",
		got.ScreenValue, dos.screen, got.ScreenValue-dos.screen)

	goLines := pvFz5GoLines(got, c.req())
	if len(goLines) != len(dos.lines) {
		t.Logf("LINE COUNT: Go %d vs DOS %d", len(goLines), len(dos.lines))
		return
	}
	shown := 0
	for i := range dos.lines {
		w, g := dos.lines[i], goLines[i]
		tolOf := func(x float64) float64 { return 0.0051 + 1e-9*math.Abs(x) }
		if w.kind != g.kind || (w.kind == "payment" && g.dateStr != "" &&
			normDateTok(w.dateStr) != normDateTok(g.dateStr)) ||
			math.Abs(w.payment-g.payment) > tolOf(w.payment) ||
			math.Abs(w.value-g.value) > tolOf(w.value) ||
			math.Abs(w.cumValue-g.cumValue) > tolOf(w.cumValue) {
			t.Logf("line %d [%s] DOS %s pay=%.2f val=%.2f cum=%.2f | Go %s pay=%.4f val=%.4f cum=%.4f",
				i, w.kind, w.dateStr, w.payment, w.value, w.cumValue,
				g.dateStr, g.payment, g.value, g.cumValue)
			shown++
			if shown >= 12 {
				break
			}
		}
	}
	if shown == 0 {
		t.Logf("all %d table lines agree", len(dos.lines))
	}
}

// TestP5Ablate localizes a screen divergence to a row: it runs each row ALONE,
// then the whole worksheet with each row DROPPED.
func TestP5Ablate(t *testing.T) {
	line := os.Getenv("P5")
	if line == "" {
		t.Skip("set P5 to an oracle table argument line")
	}
	bin := pvOracleBin()
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("PV oracle not present (%s)", bin)
	}
	c := p5Parse(t, line)

	show := func(label string, rows []int) {
		d, g, derr, gerr := p5Screen(t, bin, c, rows)
		switch {
		case derr != "" && gerr != nil:
			t.Logf("%-46s both refuse", label)
		case derr != "":
			t.Logf("%-46s DOS refuses (%s), Go %.6f", label, derr, g)
		case gerr != nil:
			t.Logf("%-46s Go refuses (%v), DOS %.6f", label, gerr, d)
		default:
			t.Logf("%-46s Go %18.6f DOS %18.6f  Δ%14.6f", label, g, d, g-d)
		}
	}

	show("FULL", nil)
	for i, tok := range c.rowToks {
		show(fmt.Sprintf("only[%d] %s", i, tok), []int{i})
	}
	if len(c.rowToks) > 1 {
		for i, tok := range c.rowToks {
			var rows []int
			for j := range c.rowToks {
				if j != i {
					rows = append(rows, j)
				}
			}
			show(fmt.Sprintf("drop[%d] %s", i, tok), rows)
		}
	}
}
