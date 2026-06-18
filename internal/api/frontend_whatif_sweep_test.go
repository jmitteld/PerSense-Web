package api

// Exhaustive frontend differential sweeps for the Mortgage "What-If" table —
// the one major mortgage flow the other frontend sweeps skipped. The What-If
// feature has two code paths in the shipped JS:
//
//   1-D (vary one column): runWhatIf builds a /api/mortgage/whatif request
//       (base = getMtgRowData(src), vary, increment SCALED by /100 for
//       rate/points/pctDown, count = lines+1), the engine's GenerateRows
//       solves each row, placeWhatIfRow writes them into the grid, then
//       calcAllMortgageRows re-solves every row.
//
//   2-D (vary two columns): a CLIENT-SIDE reimplementation steps both fields
//       in display units and calls /api/mortgage/calc per cell — it never
//       touches the engine's GenerateGrid.
//
// These sweeps capture the requests the shipped JS actually emits and check
// them against the real engine, exhaustively over the option lattice
// (vary field × increment × count × base, and 2-D field pairs). The key risk
// classes — the rate/points/pctDown ÷100 increment scaling, placeWhatIfRow's
// format round-trip, row count (drop/dup), and the 2-D stepping arithmetic —
// are all invisible to the per-row calc/render sweeps and are pinned here.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// whatIfOracle drives the real HandleMortgageWhatIf and returns its rows.
func whatIfOracle(t *testing.T, baseJSON, vary string, frac float64, count int) MortgageWhatIfResponse {
	t.Helper()
	body := fmt.Sprintf(`{"base":%s,"vary":%q,"increment":%v,"count":%d}`, baseJSON, vary, frac, count)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/mortgage/whatif", bytes.NewBufferString(body))
	HandleMortgageWhatIf(w, req)
	var resp MortgageWhatIfResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	return resp
}

// scaledVary reports whether a vary field is sent as a fraction (so a typed
// percentage-point increment is divided by 100 before going to the engine).
func scaledVary(f string) bool { return f == "rate" || f == "points" || f == "pctDown" }

// TestFrontendWhatIf1DSweep — see file header.
func TestFrontendWhatIf1DSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping What-If 1-D sweep")
	}
	html := mustReadIndexHTML(t)

	type baseCfg struct {
		price, pct, rate, tax, points float64
		years                         int
	}
	bases := []baseCfg{
		{price: 300000, pct: 0.20, rate: 0.06, tax: 300, points: 0.01, years: 30},
		{price: 175000, pct: 0.10, rate: 0.045, tax: 150, points: 0.00, years: 15},
	}
	// vary field -> a representative typed (display-unit) increment.
	varyIncs := map[string]float64{
		"rate": 0.25, "pctDown": 2, "points": 0.5, "years": 5, "price": 25000,
	}
	counts := []int{3, 6}

	type wiRow struct {
		Price    float64 `json:"price"`
		Points   float64 `json:"points"`
		PctDown  float64 `json:"pctDown"`
		Cash     float64 `json:"cash"`
		Financed float64 `json:"financed"`
		Years    int     `json:"years"`
		Rate     float64 `json:"rate"`
		Monthly  float64 `json:"monthly"`
	}
	type ncase struct {
		Vary    string            `json:"vary"`
		DispInc float64           `json:"dispInc"`
		Cnt     int               `json:"cnt"`
		Src     map[string]string `json:"src"`
		SrcStat map[string]string `json:"srcStat"`
		Rows    []wiRow           `json:"rows"`
	}
	// Go-side expectation kept parallel to the node cases.
	type expect struct {
		fracInc float64
		base    map[string]float64
		rows    []wiRow
	}

	var cases []ncase
	var exps []expect
	for _, b := range bases {
		baseJSON := fmt.Sprintf(`{"price":%v,"pctDown":%v,"years":%d,"rate":%v,"tax":%v,"points":%v}`,
			b.price, b.pct, b.years, b.rate, b.tax, b.points)
		for vary, dispInc := range varyIncs {
			frac := dispInc
			if scaledVary(vary) {
				frac = dispInc / 100
			}
			for _, cnt := range counts {
				resp := whatIfOracle(t, baseJSON, vary, frac, cnt+1)
				if resp.Error != "" || len(resp.Rows) != cnt+1 {
					t.Fatalf("oracle vary=%s cnt=%d: err=%q rows=%d", vary, cnt, resp.Error, len(resp.Rows))
				}
				rows := make([]wiRow, len(resp.Rows))
				for i, r := range resp.Rows {
					rows[i] = wiRow{r.Price, r.Points, r.PctDown, r.Cash, r.Financed, r.Years, r.Rate, r.Monthly}
				}
				// Source-row display + status (inputs typed, monthly computed/green).
				src := map[string]string{
					"price":   commafmt(b.price),
					"pctDown": strconv.FormatFloat(b.pct*100, 'f', 4, 64),
					"years":   strconv.Itoa(b.years),
					"rate":    strconv.FormatFloat(b.rate*100, 'f', 4, 64),
					"tax":     commafmt(b.tax),
					"points":  strconv.FormatFloat(b.points*100, 'f', 4, 64),
					"monthly": commafmt(rows[0].Monthly),
				}
				srcStat := map[string]string{
					"price": "input", "pctDown": "input", "years": "input",
					"rate": "input", "tax": "input", "points": "input", "monthly": "output",
				}
				cases = append(cases, ncase{Vary: vary, DispInc: dispInc, Cnt: cnt, Src: src, SrcStat: srcStat, Rows: rows})
				exps = append(exps, expect{
					fracInc: frac,
					base: map[string]float64{
						"price": b.price, "pctDown": b.pct, "years": float64(b.years),
						"rate": b.rate, "tax": b.tax, "points": b.points,
					},
					rows: rows,
				})
			}
		}
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtRate") + `
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + extractJS(t, html, "getMtgRowData") + `
` + extractJS(t, html, "placeWhatIfRow") + `
` + extractJS(t, html, "updateMtgRowUI") + `
` + extractJS(t, html, "calcAllMortgageRows") + `
` + extractJS(t, html, "closeWhatIf") + `
` + extractJS(t, html, "runWhatIf") + `
var MTG_FIELDS = [
  { key:'price', type:'money' }, { key:'points', type:'rate' }, { key:'pctDown', type:'rate' },
  { key:'cash', type:'money' }, { key:'financed', type:'money' }, { key:'years', type:'int' },
  { key:'rate', type:'rate' }, { key:'tax', type:'money' }, { key:'monthly', type:'money' },
  { key:'balloonYears', type:'int' }, { key:'balloonAmount', type:'money' } ];
var MTG_MAX_ROWS = 200;
var MTG_ROWS = 1;
var mtgSelectedRow = 0;
var mtgStatus = [{}];
var CELLS = {};
function mkCell(){ var cls=[]; var ds={}; return { value:'', dataset:ds,
  classList:{ add:function(c){if(cls.indexOf(c)<0)cls.push(c);}, remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);}, contains:function(c){return cls.indexOf(c)>=0;}, toggle:function(c,f){ if(f){if(cls.indexOf(c)<0)cls.push(c);}else{var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);} } } }; }
function getMtgCell(row, field){ var k=row+'|'+field; if(!(k in CELLS)) CELLS[k]=mkCell(); return CELLS[k]; }
function ensureMtgRows(n){ if(n>MTG_ROWS) MTG_ROWS=n; return MTG_ROWS; }
function clearFieldErrors(){}
function markMtgErrorRow(){}
function renderAdvisoryHTML(){ return ''; }
var ELS = {};
function getEl(id){ if(!(id in ELS)) ELS[id]={value:'',textContent:'',innerHTML:'',classList:{add:function(){},remove:function(){},contains:function(){return false;}}}; return ELS[id]; }
var document = { getElementById:getEl, querySelector:function(){return null;} };
var CALLS = [];
async function apiPost(url, body){ CALLS.push({url:url, body:body}); if(url.indexOf('/whatif')>=0){ return {rows: CURRENT.rows}; } return {}; }

var cases = ` + string(casesJSON) + `;
var CURRENT = null;
(async function(){
  var out = [];
  for (var k=0;k<cases.length;k++){
    var c = cases[k]; CURRENT = c;
    // reset world
    MTG_ROWS = 1; mtgStatus = [{}]; CELLS = {}; ELS = {}; CALLS = [];
    // source row 0
    for (var f in c.src){ getMtgCell(0,f).value = c.src[f]; }
    for (var f in c.srcStat){ mtgStatus[0][f] = c.srcStat[f]; if(c.srcStat[f]==='output') getMtgCell(0,f).classList.add('cell-output'); }
    // what-if dialog inputs (1-D: no second dim)
    getEl('wi-col1').value = c.vary;
    getEl('wi-inc1').value = String(c.dispInc);
    getEl('wi-cnt1').value = String(c.cnt);
    getEl('wi-col2').value = '';
    getEl('wi-inc2').value = '';
    getEl('wi-cnt2').value = '';
    await runWhatIf();
    var whatif = null, calcs = [];
    CALLS.forEach(function(call){
      if (call.url.indexOf('/whatif')>=0) whatif = call.body;
      else if (call.url.indexOf('/calc')>=0) calcs.push(call.body);
    });
    out.push({ whatif: whatif, calcs: calcs, err: getEl('mtg-error').textContent });
  }
  console.log(JSON.stringify(out));
})();
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, raw)
	}
	var res []struct {
		Whatif map[string]json.RawMessage `json:"whatif"`
		Calcs  []map[string]float64       `json:"calcs"`
		Err    string                     `json:"err"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, raw)
	}
	if len(res) != len(cases) {
		t.Fatalf("got %d results for %d cases", len(res), len(cases))
	}

	approx := func(a, b float64) bool { return math.Abs(a-b) <= 1e-6+1e-6*math.Abs(b) }
	checks := 0
	for i := range cases {
		c, e, r := cases[i], exps[i], res[i]
		tag := fmt.Sprintf("case %d vary=%s inc=%g cnt=%d", i, c.Vary, c.DispInc, c.Cnt)
		if r.Err != "" {
			t.Errorf("%s: runWhatIf surfaced error %q", tag, r.Err)
			continue
		}
		// (a) the /whatif REQUEST the shipped JS built.
		if r.Whatif == nil {
			t.Errorf("%s: no /whatif request captured", tag)
			continue
		}
		var gotVary string
		var gotInc, gotCount float64
		_ = json.Unmarshal(r.Whatif["vary"], &gotVary)
		_ = json.Unmarshal(r.Whatif["increment"], &gotInc)
		_ = json.Unmarshal(r.Whatif["count"], &gotCount)
		if gotVary != c.Vary {
			t.Errorf("%s: /whatif vary=%q, want %q", tag, gotVary, c.Vary)
		}
		if !approx(gotInc, e.fracInc) {
			t.Errorf("%s: /whatif increment=%g, want %g (scaling bug?)", tag, gotInc, e.fracInc)
		}
		if int(gotCount) != c.Cnt+1 {
			t.Errorf("%s: /whatif count=%d, want %d (lines+1)", tag, int(gotCount), c.Cnt+1)
		}
		var gotBase map[string]float64
		_ = json.Unmarshal(r.Whatif["base"], &gotBase)
		for f, want := range e.base {
			if !approx(gotBase[f], want) {
				t.Errorf("%s: /whatif base.%s=%g, want %g", tag, f, gotBase[f], want)
			}
		}
		// (b) one /calc per row (source + cnt placed), each matching the engine row's inputs.
		if len(r.Calcs) != c.Cnt+1 {
			t.Errorf("%s: %d /calc requests, want %d (source + %d generated) — row drop/dup",
				tag, len(r.Calcs), c.Cnt+1, c.Cnt)
			continue
		}
		for k := 0; k < len(e.rows); k++ {
			req := r.Calcs[k]
			row := e.rows[k]
			// Inputs carried/placed: price, pctDown, years, rate, tax, points.
			want := map[string]float64{
				"price": row.Price, "pctDown": row.PctDown, "years": float64(row.Years),
				"rate": row.Rate, "tax": e.base["tax"], "points": row.Points,
			}
			for f, wv := range want {
				if !approx(req[f], wv) {
					t.Errorf("%s row %d: /calc %s=%g, want %g (placement round-trip)", tag, k, f, req[f], wv)
				}
			}
			// Computed fields must NOT be fed back as inputs.
			if _, ok := req["monthly"]; ok {
				t.Errorf("%s row %d: /calc wrongly sent computed monthly as input", tag, k)
			}
			checks++
		}
	}
	t.Logf("What-If 1-D sweep: %d cases, %d generated rows verified (request scaling + placement round-trip + row count)", len(cases), checks)
}

// TestFrontendWhatIf2DSweep exhaustively exercises the CLIENT-SIDE 2-D grid
// stepping (the path that bypasses the engine's GenerateGrid entirely). For a
// cube of (vary1, vary2) field pairs × increments × line counts × base rows it
// drives the shipped runWhatIf, captures the per-cell /api/mortgage/calc
// requests, and asserts every grid cell's inputs equal the engine-faithful
// stepping: base[col] + step*increment, with the same ÷100 scaling for
// rate/points/pctDown as the 1-D path. This pins the JS reimplementation that
// previously had zero coverage (cell drop/dup, wrong-axis stepping, missing
// scaling all fail here).
func TestFrontendWhatIf2DSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping What-If 2-D sweep")
	}
	html := mustReadIndexHTML(t)

	type baseCfg struct {
		price, pct, rate, tax, points float64
		years                         int
		monthly                       float64
	}
	bases := []baseCfg{
		{price: 300000, pct: 0.20, rate: 0.06, tax: 300, points: 0.01, years: 30},
		{price: 220000, pct: 0.15, rate: 0.05, tax: 200, points: 0.005, years: 20},
	}
	for i := range bases {
		b := &bases[i]
		r := callMortgage(t, fmt.Sprintf(`{"price":%v,"pctDown":%v,"years":%d,"rate":%v,"tax":%v,"points":%v}`,
			b.price, b.pct, b.years, b.rate, b.tax, b.points))
		b.monthly = r.Monthly
	}
	disp := map[string]float64{"rate": 0.25, "pctDown": 2, "points": 0.5, "years": 5, "price": 20000}
	pairs := [][2]string{{"rate", "years"}, {"pctDown", "rate"}, {"points", "price"}, {"years", "rate"}, {"price", "pctDown"}}
	cntPairs := [][2]int{{2, 2}, {3, 2}}

	type ncase struct {
		Col1    string            `json:"col1"`
		Col2    string            `json:"col2"`
		Inc1    float64           `json:"inc1"`
		Inc2    float64           `json:"inc2"`
		Cnt1    int               `json:"cnt1"`
		Cnt2    int               `json:"cnt2"`
		Src     map[string]string `json:"src"`
		SrcStat map[string]string `json:"srcStat"`
	}
	type expect struct {
		base       map[string]float64
		f1, f2     float64 // per-step fraction increment for col1/col2
		col1, col2 string
		cnt1, cnt2 int
	}
	var cases []ncase
	var exps []expect
	for _, b := range bases {
		src := map[string]string{
			"price": commafmt(b.price), "pctDown": strconv.FormatFloat(b.pct*100, 'f', 4, 64),
			"years": strconv.Itoa(b.years), "rate": strconv.FormatFloat(b.rate*100, 'f', 4, 64),
			"tax": commafmt(b.tax), "points": strconv.FormatFloat(b.points*100, 'f', 4, 64),
			"monthly": commafmt(b.monthly),
		}
		srcStat := map[string]string{
			"price": "input", "pctDown": "input", "years": "input",
			"rate": "input", "tax": "input", "points": "input", "monthly": "output",
		}
		baseFrac := map[string]float64{
			"price": b.price, "pctDown": b.pct, "years": float64(b.years),
			"rate": b.rate, "tax": b.tax, "points": b.points,
		}
		for _, p := range pairs {
			for _, cc := range cntPairs {
				c1, c2 := p[0], p[1]
				f1, f2 := disp[c1], disp[c2]
				if scaledVary(c1) {
					f1 = disp[c1] / 100
				}
				if scaledVary(c2) {
					f2 = disp[c2] / 100
				}
				cases = append(cases, ncase{
					Col1: c1, Col2: c2, Inc1: disp[c1], Inc2: disp[c2],
					Cnt1: cc[0], Cnt2: cc[1], Src: src, SrcStat: srcStat,
				})
				exps = append(exps, expect{base: baseFrac, f1: f1, f2: f2, col1: c1, col2: c2, cnt1: cc[0], cnt2: cc[1]})
			}
		}
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtRate") + `
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + extractJS(t, html, "getMtgRowData") + `
` + extractJS(t, html, "placeWhatIfRow") + `
` + extractJS(t, html, "updateMtgRowUI") + `
` + extractJS(t, html, "calcAllMortgageRows") + `
` + extractJS(t, html, "closeWhatIf") + `
` + extractJS(t, html, "runWhatIf") + `
var MTG_FIELDS = [
  { key:'price', type:'money' }, { key:'points', type:'rate' }, { key:'pctDown', type:'rate' },
  { key:'cash', type:'money' }, { key:'financed', type:'money' }, { key:'years', type:'int' },
  { key:'rate', type:'rate' }, { key:'tax', type:'money' }, { key:'monthly', type:'money' },
  { key:'balloonYears', type:'int' }, { key:'balloonAmount', type:'money' } ];
var MTG_MAX_ROWS = 400;
var MTG_ROWS = 1;
var mtgSelectedRow = 0;
var mtgStatus = [{}];
var CELLS = {};
// value is a string-coercing accessor, mirroring a real <input>.value (the 2-D
// stepping assigns a raw number to the years cell and relies on that coercion).
function mkCell(){ var cls=[]; var ds={}; var _v='';
  return { get value(){return _v;}, set value(x){ _v = (x==null?'':String(x)); }, dataset:ds,
  classList:{ add:function(c){if(cls.indexOf(c)<0)cls.push(c);}, remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);}, contains:function(c){return cls.indexOf(c)>=0;}, toggle:function(c,f){ if(f){if(cls.indexOf(c)<0)cls.push(c);}else{var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);} } } }; }
function getMtgCell(row, field){ var k=row+'|'+field; if(!(k in CELLS)) CELLS[k]=mkCell(); return CELLS[k]; }
function ensureMtgRows(n){ if(n>MTG_ROWS) MTG_ROWS=n; return MTG_ROWS; }
function clearFieldErrors(){}
function markMtgErrorRow(){}
function renderAdvisoryHTML(){ return ''; }
var ELS = {};
function getEl(id){ if(!(id in ELS)) ELS[id]={value:'',textContent:'',innerHTML:'',classList:{add:function(){},remove:function(){},contains:function(){return false;}}}; return ELS[id]; }
var document = { getElementById:getEl, querySelector:function(){return null;} };
var CALLS = [];
async function apiPost(url, body){ CALLS.push({url:url, body:body}); return {}; }

var cases = ` + string(casesJSON) + `;
(async function(){
  var out = [];
  for (var k=0;k<cases.length;k++){
    var c = cases[k];
    MTG_ROWS = 1; mtgStatus = [{}]; CELLS = {}; ELS = {}; CALLS = [];
    for (var f in c.src){ getMtgCell(0,f).value = c.src[f]; }
    for (var f in c.srcStat){ mtgStatus[0][f] = c.srcStat[f]; if(c.srcStat[f]==='output') getMtgCell(0,f).classList.add('cell-output'); }
    getEl('wi-col1').value = c.col1; getEl('wi-inc1').value = String(c.inc1); getEl('wi-cnt1').value = String(c.cnt1);
    getEl('wi-col2').value = c.col2; getEl('wi-inc2').value = String(c.inc2); getEl('wi-cnt2').value = String(c.cnt2);
    await runWhatIf();
    var calcs = [];
    CALLS.forEach(function(call){ if (call.url.indexOf('/calc')>=0) calcs.push(call.body); });
    out.push({ calcs: calcs, err: getEl('mtg-error').textContent });
  }
  console.log(JSON.stringify(out));
})();
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, raw)
	}
	var res []struct {
		Calcs []map[string]float64 `json:"calcs"`
		Err   string               `json:"err"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, raw)
	}
	approx := func(a, b float64) bool { return math.Abs(a-b) <= 1e-6+1e-6*math.Abs(b) }
	cells := 0
	for i := range cases {
		e, r := exps[i], res[i]
		tag := fmt.Sprintf("case %d [%s×%s] cnt=%d×%d", i, e.col1, e.col2, e.cnt1, e.cnt2)
		if r.Err != "" {
			t.Errorf("%s: runWhatIf surfaced error %q", tag, r.Err)
			continue
		}
		d1, d2 := e.cnt1+1, e.cnt2+1
		// Expected calc order: (0,0) first (the source row), then the placement
		// loop (j outer, i inner, skipping (0,0)).
		type cell struct{ i, j int }
		order := []cell{{0, 0}}
		for j := 0; j < d2; j++ {
			for ii := 0; ii < d1; ii++ {
				if ii == 0 && j == 0 {
					continue
				}
				order = append(order, cell{ii, j})
			}
		}
		if len(r.Calcs) != d1*d2 {
			t.Errorf("%s: %d /calc requests, want %d (%d×%d grid) — cell drop/dup", tag, len(r.Calcs), d1*d2, d1, d2)
			continue
		}
		for k, cl := range order {
			req := r.Calcs[k]
			want := map[string]float64{}
			for f, v := range e.base {
				want[f] = v
			}
			want[e.col1] = e.base[e.col1] + float64(cl.i)*e.f1
			want[e.col2] = e.base[e.col2] + float64(cl.j)*e.f2
			for _, f := range []string{"price", "pctDown", "years", "rate", "tax", "points"} {
				if !approx(req[f], want[f]) {
					t.Errorf("%s cell(i=%d,j=%d): /calc %s=%g, want %g (stepping)", tag, cl.i, cl.j, f, req[f], want[f])
				}
			}
			if _, ok := req["monthly"]; ok {
				t.Errorf("%s cell(i=%d,j=%d): /calc wrongly sent computed monthly", tag, cl.i, cl.j)
			}
			cells++
		}
	}
	t.Logf("What-If 2-D sweep: %d cases, %d grid cells verified vs engine stepping (client-side 2-D path)", len(cases), cells)
}
