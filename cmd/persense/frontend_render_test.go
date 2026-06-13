package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// Regression tests for the front-end rendering / clearing / import fixes
// (see docs/frontend_qa_report.md). Like frontend_dob_year_test.go, these
// extract the SHIPPED functions from static/index.html and run them under
// Node with minimal DOM stubs — the code under test is the real code, not
// a copy. They skip when node is not installed.
//
// extractJSFunc lives in frontend_dob_year_test.go (same package).

// readIndexHTML loads the page source or fails the test.
func readIndexHTML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	return string(b)
}

// runNode pipes a harness to `node -` and returns stdout, honoring an
// optional TZ override (needed to prove the timezone-bucketing fix).
func runNode(t *testing.T, harness, tz string) []byte {
	t.Helper()
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend JS test")
	}
	cmd := exec.Command(nodePath, "-")
	cmd.Stdin = strings.NewReader(harness)
	if tz != "" {
		cmd.Env = append(os.Environ(), "TZ="+tz)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	return out
}

// A small synthetic schedule whose row #2 behaves like a balloon: the
// engine's post-payment balance (the `principal` field) does NOT equal
// prevBalance - (payment - interest), so the old "naive walk" renderer
// drifted. The fix reads the engine balance directly and derives the
// principal column as the balance delta.
//
//	amount = 1000
//	#1  pay 100  int 50    -> balance 950   (principalPaid 50)
//	#2  pay 600  int 47.5  -> balance 400   (principalPaid 550)  <- naive: 397.50
//	#3  pay 420  int 20    -> balance 0     (principalPaid 400)
const amzScheduleJSON = `{
  "input":  { "amount": 1000 },
  "result": { "schedule": [
    { "payNum": 1, "date": "2024-01-01", "payment": 100, "interest": 50,   "principal": 950, "intToDate": 50 },
    { "payNum": 2, "date": "2024-04-01", "payment": 600, "interest": 47.5, "principal": 400, "intToDate": 97.5 },
    { "payNum": 3, "date": "2025-01-01", "payment": 420, "interest": 20,   "principal": 0,   "intToDate": 117.5 }
  ] }
}`

// TestAmzScheduleRenderJS pins: (1) the per-payment Balance column equals
// the engine's post-payment balance and the Principal column is the
// balance delta; (2) quarterly/yearly aggregation buckets by the date's
// own year/quarter, not the local-timezone interpretation of a UTC
// midnight (run under America/New_York, where the old code mislabeled a
// Jan-1 payment as the prior year/quarter).
func TestAmzScheduleRenderJS(t *testing.T) {
	html := readIndexHTML(t)
	harness := `
'use strict';
var amzScheduleData = ` + amzScheduleJSON + `;

` + extractJSFunc(t, html, "fmtMoney") + `
` + extractJSFunc(t, html, "fmtDateDisplay") + `
` + extractJSFunc(t, html, "renderAmzSchedule") + `

// --- Minimal DOM shim ---
function makeEl() {
  return {
    _html: '', children: [], textContent: '',
    set innerHTML(v) { this._html = v; this.children = []; },
    get innerHTML() { return this._html; },
    appendChild(c) { this.children.push(c); },
  };
}
var els = {
  'amz-schedule-body': makeEl(),
  'amz-schedule-head': makeEl(),
  'amz-view-note': makeEl(),
  'amz-view-mode': { value: 'payment' },
};
var document = {
  getElementById: function (id) { return els[id] || null; },
  createElement: function () { return makeEl(); },
};

function tdsOf(el) {
  return (el._html.match(/<td>([\s\S]*?)<\/td>/g) || [])
    .map(function (s) { return s.replace(/<\/?td>/g, ''); });
}
function renderRows(mode) {
  els['amz-view-mode'].value = mode;
  els['amz-schedule-body'] = makeEl();
  renderAmzSchedule();
  return els['amz-schedule-body'].children.map(function (tr) { return tdsOf(tr); });
}

var out = {
  payment: renderRows('payment'),
  year: renderRows('year'),
  quarter: renderRows('quarter'),
};
console.log(JSON.stringify(out));
`
	// Force a west-of-UTC zone so the old new Date("YYYY-MM-DD") path would
	// shift Jan-1 into the previous year; the fix must not.
	out := runNode(t, harness, "America/New_York")

	var res struct {
		Payment [][]string `json:"payment"`
		Year    [][]string `json:"year"`
		Quarter [][]string `json:"quarter"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}

	// Per-payment: Principal column (td[4]) and Balance column (td[5]).
	wantPrincipal := []string{"$50.00", "$550.00", "$400.00"}
	wantBalance := []string{"$950.00", "$400.00", "$0.00"}
	if len(res.Payment) != 3 {
		t.Fatalf("payment view: got %d rows, want 3", len(res.Payment))
	}
	for i, row := range res.Payment {
		if row[4] != wantPrincipal[i] {
			t.Errorf("payment row %d Principal = %q, want %q (must be balance delta, not payment-interest)", i+1, row[4], wantPrincipal[i])
		}
		if row[5] != wantBalance[i] {
			t.Errorf("payment row %d Balance = %q, want %q (must equal engine post-payment balance)", i+1, row[5], wantBalance[i])
		}
	}

	// Yearly buckets: keys are td[1]. Two buckets: 2024 (rows 1-2), 2025 (row 3).
	if len(res.Year) != 2 {
		t.Fatalf("year view: got %d buckets, want 2", len(res.Year))
	}
	if res.Year[0][1] != "2024" {
		t.Errorf("year bucket #1 key = %q, want \"2024\" (timezone bug would give \"2023\")", res.Year[0][1])
	}
	if res.Year[1][1] != "2025" {
		t.Errorf("year bucket #2 key = %q, want \"2025\"", res.Year[1][1])
	}
	// Yearly principal (td[4]) should sum the balance deltas: 600 then 400.
	if res.Year[0][4] != "$600.00" {
		t.Errorf("year bucket #1 Principal = %q, want \"$600.00\"", res.Year[0][4])
	}

	// Quarterly buckets: Q1 2024, Q2 2024, Q1 2025.
	wantQ := []string{"Q1 2024", "Q2 2024", "Q1 2025"}
	if len(res.Quarter) != 3 {
		t.Fatalf("quarter view: got %d buckets, want 3", len(res.Quarter))
	}
	for i, row := range res.Quarter {
		if row[1] != wantQ[i] {
			t.Errorf("quarter bucket #%d key = %q, want %q (timezone bug would shift a quarter-start payment back one quarter)", i+1, row[1], wantQ[i])
		}
	}
}

// TestAmzCSVExportJS pins that the CSV Balance column uses the engine's
// post-payment balance (so it reconciles with the on-screen schedule and
// the payoff tool) rather than the old drifting running-subtraction.
func TestAmzCSVExportJS(t *testing.T) {
	html := readIndexHTML(t)
	harness := `
'use strict';
var amzScheduleData = ` + amzScheduleJSON + `;
var capturedCSV = '';

` + extractJSFunc(t, html, "exportAmzCSV") + `

function makeEl() { return { click: function () {} }; }
var document = {
  getElementById: function () { return { textContent: '' }; },
  createElement: function () { return makeEl(); },
};
var Blob = function (parts) { capturedCSV = parts.join(''); };
var URL = { createObjectURL: function () { return 'blob:x'; }, revokeObjectURL: function () {} };

exportAmzCSV();
// Parse data rows (skip header). Column 5 (0-based) is Balance.
var lines = capturedCSV.trim().split('\n').slice(1);
var balances = lines.map(function (l) { return l.split(',')[5]; });
console.log(JSON.stringify({ balances: balances }));
`
	out := runNode(t, harness, "")
	var res struct {
		Balances []string `json:"balances"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	want := []string{"950.00", "400.00", "0.00"}
	if len(res.Balances) != len(want) {
		t.Fatalf("CSV: got %d data rows, want %d (%v)", len(res.Balances), len(want), res.Balances)
	}
	for i, b := range res.Balances {
		if b != want[i] {
			t.Errorf("CSV row %d Balance = %q, want %q", i+1, b, want[i])
		}
	}
}

// TestClearPVDefaultsJS pins that clearPV resets a periodic row's Pmts/Yr
// to "12" and COLA to "0" (matching the new-row defaults) rather than
// blanking them — a blank Pmts/Yr made a reused row fail validation.
func TestClearPVDefaultsJS(t *testing.T) {
	html := readIndexHTML(t)
	harness := `
'use strict';

` + extractJSFunc(t, html, "clearPV") + `

class Event { constructor() {} }
var pvLsCount = 1, pvPerCount = 1;
function refreshLifeContingencyOptions() {}
function setAutoCalcHint() {}
function updatePVActiveSummary() {}

function inp(v) { return { value: v, classList: { remove: function () {} } }; }
var inputs = {};
['date', 'amount', 'value'].forEach(function (f) {
  inputs['input[data-ls="0"][data-f="' + f + '"]'] = inp('junk');
});
[['from', 'junk'], ['to', 'junk'], ['perYr', '99'], ['amount', 'junk'], ['cola', '99'], ['value', 'junk']].forEach(function (p) {
  inputs['input[data-per="0"][data-f="' + p[0] + '"]'] = inp(p[1]);
});
var byId = {};
['pv-asOfDate', 'pv-rate', 'pv-total', 'actu-dob1', 'actu-dob2', 'actu-now', 'actu-pod',
 'actu-csv1', 'actu-csv2', 'pv-pod-result', 'pv-error'].forEach(function (id) {
  byId[id] = { value: 'junk', textContent: 'junk', classList: { remove: function () {} } };
});
['actu-table1', 'actu-table2'].forEach(function (id) {
  byId[id] = { selectedIndex: 3, dispatchEvent: function () {}, classList: { remove: function () {} } };
});
var document = {
  getElementById: function (id) { return byId[id] || null; },
  querySelector: function (sel) { return inputs[sel] || null; },
  querySelectorAll: function () { return []; },
};

clearPV();
var per = function (f) { return inputs['input[data-per="0"][data-f="' + f + '"]'].value; };
var ls = function (f) { return inputs['input[data-ls="0"][data-f="' + f + '"]'].value; };
console.log(JSON.stringify({
  perYr: per('perYr'), cola: per('cola'),
  from: per('from'), to: per('to'), perAmount: per('amount'), perValue: per('value'),
  lsDate: ls('date'), lsAmount: ls('amount'), lsValue: ls('value'),
}));
`
	out := runNode(t, harness, "")
	var res struct {
		PerYr, Cola, From, To, PerAmount, PerValue string
		LsDate, LsAmount, LsValue                  string
	}
	// Field names map to JSON keys via lowercasing; decode explicitly.
	var raw map[string]string
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	res.PerYr, res.Cola = raw["perYr"], raw["cola"]
	res.From, res.To, res.PerAmount, res.PerValue = raw["from"], raw["to"], raw["perAmount"], raw["perValue"]
	res.LsDate, res.LsAmount, res.LsValue = raw["lsDate"], raw["lsAmount"], raw["lsValue"]

	if res.PerYr != "12" {
		t.Errorf("clearPV: periodic Pmts/Yr = %q, want \"12\" (blank breaks reuse validation)", res.PerYr)
	}
	if res.Cola != "0" {
		t.Errorf("clearPV: periodic COLA = %q, want \"0\"", res.Cola)
	}
	for name, got := range map[string]string{
		"from": res.From, "to": res.To, "periodic amount": res.PerAmount, "periodic value": res.PerValue,
		"lump date": res.LsDate, "lump amount": res.LsAmount, "lump value": res.LsValue,
	} {
		if got != "" {
			t.Errorf("clearPV: %s = %q, want empty", name, got)
		}
	}
}

// TestPSNPrepayColumnOrder pins (without node) that the .psn import maps
// prepayment fields in the same column order the row markup declares:
// startDate, nPmts, stopDate, perYr, amount. A mismatch silently
// scrambles imported prepayments.
func TestPSNPrepayColumnOrder(t *testing.T) {
	html := readIndexHTML(t)

	// 1. Markup order: the first prepay row's data-amz-prepay-field sequence.
	rowRe := regexp.MustCompile(`(?s)<tr data-amz-prepay-row>(.*?)</tr>`)
	rowM := rowRe.FindStringSubmatch(html)
	if rowM == nil {
		t.Fatal("could not find a prepay row in markup")
	}
	fieldRe := regexp.MustCompile(`data-amz-prepay-field="([a-zA-Z]+)"`)
	var markup []string
	for _, m := range fieldRe.FindAllStringSubmatch(rowM[1], -1) {
		markup = append(markup, m[1])
	}
	wantOrder := []string{"startDate", "nPmts", "stopDate", "perYr", "amount"}
	if strings.Join(markup, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("prepay markup column order = %v, want %v", markup, wantOrder)
	}

	// 2. Import mapping: the populateAdvRows('amz-prepay-body', ...) callback
	//    must assign cells in the same order via p.<field>.
	impRe := regexp.MustCompile(`(?s)populateAdvRows\('amz-prepay-body'.*?\}\);`)
	imp := impRe.FindString(html)
	if imp == "" {
		t.Fatal("could not find prepay import mapping")
	}
	cellRe := regexp.MustCompile(`row\.cells\[(\d)\][^=]*=\s*p\.([a-zA-Z]+)`)
	mapping := map[string]string{} // cellIndex -> field
	for _, m := range cellRe.FindAllStringSubmatch(imp, -1) {
		mapping[m[1]] = m[2]
	}
	for i, want := range wantOrder {
		idx := string(rune('0' + i))
		if mapping[idx] != want {
			t.Errorf("import maps cells[%d] = p.%s, want p.%s (column scramble regression)", i, mapping[idx], want)
		}
	}
}

// extractAsyncJSFunc is like extractJSFunc but for `async function name`.
func extractAsyncJSFunc(t *testing.T, html, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)async function ` + name + `\([^)]*\) \{.*?\n\}`)
	src := re.FindString(html)
	if src == "" {
		t.Fatalf("async function %s not found in index.html", name)
	}
	return src
}

// TestAutoCalcStaleGuardJS pins the auto-calculate concurrency fix: a
// silent (auto) calc must DISCARD its response if the user edited a field
// while the request was in flight (calcGeneration changed), so a stale,
// often all-zeros response can't overwrite freshly typed values. It must
// still apply the result when nothing changed. Runs the shipped
// calcMortgageRow under Node with stubbed dependencies; apiPost optionally
// bumps calcGeneration mid-flight to simulate a keystroke.
func TestAutoCalcStaleGuardJS(t *testing.T) {
	html := readIndexHTML(t)
	calcSrc := extractAsyncJSFunc(t, html, "calcMortgageRow")
	harness := `
// non-strict so eval() can define the function in this scope
function run(simEditDuringFlight) {
  var calcGeneration = 0, autoSilent = true, mtgSelectedRow = 0, updated = false;
  function getMtgRowData() { return { price: 200000, years: 30, rate: 0.06 }; }
  function updateMtgRowUI() { updated = true; }
  function clearFieldErrors() {}
  function setAutoCalcHint() {}
  function markMtgErrorRow() {}
  function explainMtgError() { return ''; }
  function renderAdvisoryHTML() { return ''; }
  var document = { getElementById: function () { return { textContent: '', innerHTML: '' }; } };
  async function apiPost() {
    if (simEditDuringFlight) { calcGeneration++; } // a keystroke lands while in flight
    return { price: 200000, points: 0, pctDown: 0, cash: 0, financed: 0, years: 0, rate: 0, tax: 0, monthly: 0 };
  }
  eval(` + "`" + calcSrc + "`" + `);
  return calcMortgageRow().then(function (ret) { return { ret: ret, updated: updated }; });
}
(async function () {
  var stale = await run(true);
  var fresh = await run(false);
  console.log(JSON.stringify({ stale: stale, fresh: fresh }));
})();
`
	out := runNode(t, harness, "")
	var res struct {
		Stale struct {
			Ret     bool `json:"ret"`
			Updated bool `json:"updated"`
		} `json:"stale"`
		Fresh struct {
			Ret     bool `json:"ret"`
			Updated bool `json:"updated"`
		} `json:"fresh"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	if res.Stale.Ret || res.Stale.Updated {
		t.Errorf("stale auto-calc: ret=%v updated=%v, want both false (must drop a result superseded by a newer edit)", res.Stale.Ret, res.Stale.Updated)
	}
	if !res.Fresh.Ret || !res.Fresh.Updated {
		t.Errorf("fresh auto-calc: ret=%v updated=%v, want both true (must apply when inputs unchanged)", res.Fresh.Ret, res.Fresh.Updated)
	}
}

// univDOMStub is a permissive DOM shim sufficient to run the amortization /
// PV calc functions end-to-end: getElementById/createElement return a generic
// element that accepts any value/textContent/innerHTML/classList/style write,
// and querySelector returns null (the calc echo loops all guard on null).
const univDOMStub = `
  function makeEl() {
    return {
      value: '', textContent: '', innerHTML: '', open: false, style: {},
      classList: { add: function () {}, remove: function () {}, toggle: function () {}, contains: function () { return false; } },
      focus: function () {}, appendChild: function () {}, setAttribute: function () {},
      querySelectorAll: function () { return []; },
    };
  }
  var document = {
    getElementById: function () { return makeEl(); },
    querySelector: function () { return null; },
    querySelectorAll: function () { return []; },
    createElement: function () { return makeEl(); },
  };
`

// assertStaleGuard checks the {stale, fresh} JSON shape emitted by the
// amortization / PV stale-guard harnesses: a stale run must neither apply nor
// return true; a fresh run must do both.
func assertStaleGuard(t *testing.T, screen string, out []byte) {
	t.Helper()
	var res struct {
		Stale struct {
			Ret     bool `json:"ret"`
			Applied bool `json:"applied"`
		} `json:"stale"`
		Fresh struct {
			Ret     bool `json:"ret"`
			Applied bool `json:"applied"`
		} `json:"fresh"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("%s: parse node output: %v\n%s", screen, err, out)
	}
	if res.Stale.Ret || res.Stale.Applied {
		t.Errorf("%s stale auto-calc: ret=%v applied=%v, want both false (must drop a result superseded by a newer edit)", screen, res.Stale.Ret, res.Stale.Applied)
	}
	if !res.Fresh.Ret || !res.Fresh.Applied {
		t.Errorf("%s fresh auto-calc: ret=%v applied=%v, want both true (must apply when inputs unchanged)", screen, res.Fresh.Ret, res.Fresh.Applied)
	}
}

// jsStringLiteral encodes a JS source string as a double-quoted JS string
// literal (via JSON) so it can be embedded in a harness and run with eval().
// Double-quoted is backtick-safe — important because calcPV contains template
// literals, which would otherwise collide with the Go raw-string delimiters.
func jsStringLiteral(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal js source: %v", err)
	}
	return string(b)
}

// TestAutoCalcStaleGuardAmzJS is the Amortization counterpart of
// TestAutoCalcStaleGuardJS: the shipped calcAmortization must drop a silent
// result when the user edits while the request is in flight, and apply it
// otherwise. `applied` is set by the stubbed renderAdvisoryHTML, which the
// success path reaches only after the stale guard.
func TestAutoCalcStaleGuardAmzJS(t *testing.T) {
	html := readIndexHTML(t)
	srcLit := jsStringLiteral(t, extractAsyncJSFunc(t, html, "calcAmortization"))
	harness := `
function run(simEdit) {
  var calcGeneration = 0, autoSilent = true, amzScheduleData = null, applied = false;
` + univDOMStub + `
  function getAmzInput() { return { body: { amount: 100000, perYr: 12, basis: '360', loanDate: '2024-01-01', firstDate: '2024-02-01', rate: 0.06, nPeriods: 360, payment: 599.55 } }; }
  function blockInvalidDates() { return false; }
  function clearFieldErrors() {}
  function setAutoCalcHint() {}
  function markAmzErrorFields() {}
  function clearAmzScheduleOutput() {}
  function renderAdvisoryHTML() { applied = true; return ''; }
  function fmtMoney() { return '0'; }
  function fmtDollars() { return '$0'; }
  function fmtDateDisplay() { return ''; }
  function renderAmzSchedule() {}
  function updatePayoffBalance() {}
  function updateAmzAdvBadge() {}
  async function apiPost() {
    if (simEdit) { calcGeneration++; } // a keystroke lands while in flight
    return { schedule: [{ payNum: 1, date: '2024-02-01', payment: 599.55, interest: 500, principal: 99900.45, intToDate: 500 }], totalPaid: 215838, totalInterest: 115838, apr: null, firstDate: '2024-02-01', lastDate: '2054-01-01', nPeriods: 360 };
  }
  eval(` + srcLit + `);
  return calcAmortization().then(function (r) { return { ret: r, applied: applied }; });
}
(async function () {
  var stale = await run(true);
  var fresh = await run(false);
  console.log(JSON.stringify({ stale: stale, fresh: fresh }));
})();
`
	assertStaleGuard(t, "amortization", runNode(t, harness, ""))
}

// TestAutoCalcStaleGuardPVJS is the Present Value counterpart. calcPV contains
// template literals, so its source is embedded as a JSON string and eval'd.
func TestAutoCalcStaleGuardPVJS(t *testing.T) {
	html := readIndexHTML(t)
	srcLit := jsStringLiteral(t, extractAsyncJSFunc(t, html, "calcPV"))
	harness := `
function run(simEdit) {
  var calcGeneration = 0, autoSilent = true, pvLumpBlanks = {}, pvPerBlanks = {}, applied = false;
` + univDOMStub + `
  function getPVInput() { return { rate: 0.06, asOfDate: '2024-01-01', lumpSums: [{}], periodics: [], sumValue: null }; }
  function blockInvalidDates() { return false; }
  function defaultPVReferenceDate() {}
  function clearFieldErrors() {}
  function setAutoCalcHint() {}
  function markPVErrorFields() {}
  function pvContingencyConfigError() { return null; }
  function renderAdvisoryHTML() { applied = true; return ''; }
  function fmtDollars() { return '$0'; }
  function fmtMoney() { return '0'; }
  function updatePVActiveSummary() {}
  async function apiPost() {
    if (simEdit) { calcGeneration++; } // a keystroke lands while in flight
    return { sumValue: 9433.61, lumpSums: [{ value: 9433.61 }], periodics: [] };
  }
  eval(` + srcLit + `);
  return calcPV().then(function (r) { return { ret: r, applied: applied }; });
}
(async function () {
  var stale = await run(true);
  var fresh = await run(false);
  console.log(JSON.stringify({ stale: stale, fresh: fresh }));
})();
`
	assertStaleGuard(t, "presentvalue", runNode(t, harness, ""))
}

// TestKeyboardCalcWiring pins the keyboard contract: F10 is a general
// "calculate now" that dispatches to each screen's calc function, and the
// Enter handler triggers an auto-calc (scheduleAutoCalc) in addition to
// moving focus. These are string-level checks because the handler is an
// inline addEventListener, not an extractable function.
func TestKeyboardCalcWiring(t *testing.T) {
	html := readIndexHTML(t)
	sub := func(start, n int) string {
		end := start + n
		if end > len(html) {
			end = len(html)
		}
		return html[start:end]
	}

	// F10 handler exists and dispatches to all three screens' calc functions.
	i := strings.Index(html, "e.key === 'F10'")
	if i < 0 {
		t.Fatal("no F10 keydown handler found (general Calculate key missing)")
	}
	f10block := sub(i, 700)
	for _, fn := range []string{"calcMortgageRow", "calcAmortization", "calcPV"} {
		if !strings.Contains(f10block, fn) {
			t.Errorf("F10 handler does not dispatch to %s", fn)
		}
	}

	// Enter handler still moves focus AND triggers auto-calc. Anchor on the
	// unique nextFocusable(t, e.shiftKey) call — the first "e.key === 'Enter'"
	// in the file belongs to a different (tooltip) handler.
	k := strings.Index(html, "nextFocusable(t, e.shiftKey)")
	if k < 0 {
		t.Fatal("Enter handler no longer advances focus (nextFocusable missing)")
	}
	start := k - 400
	if start < 0 {
		start = 0
	}
	if !strings.Contains(html[start:k], "e.key === 'Enter'") {
		t.Error("nextFocusable is not inside the Enter keydown branch")
	}
	if !strings.Contains(sub(k, 800), "scheduleAutoCalc") {
		t.Error("Enter handler no longer triggers auto-calc on commit (scheduleAutoCalc missing)")
	}
}

// TestAmzDateMoneyHelpersJS pins the amortization date/money conveniences:
//   - computeDefaultFirstPayment reproduces the DOS rule (loan day > 1 → first of
//     the second following month; on the 1st → next period), and returns null for
//     non-month frequencies (left to the engine).
//   - inferAmzDateFromLoan fills a bare MM/DD's year from the loan date, rolling
//     forward a year when needed, and ignores full dates / a missing anchor.
//   - formatMoneyField reformats to $xx,xxx.xx and leaves unparseable text alone.
func TestAmzDateMoneyHelpersJS(t *testing.T) {
	html := readIndexHTML(t)
	harness := `
'use strict';
var __perYr = '12';
var document = { getElementById: function (id) { return id === 'amz-perYr' ? { value: __perYr } : null; } };
` + extractJSFunc(t, html, "parseInt2") + `
` + extractJSFunc(t, html, "parseMoney") + `
` + extractJSFunc(t, html, "fmtMoney") + `
` + extractJSFunc(t, html, "fmtDollars") + `
` + extractJSFunc(t, html, "amzPeriodMonths") + `
` + extractJSFunc(t, html, "amzAddMonths") + `
` + extractJSFunc(t, html, "computeDefaultFirstPayment") + `
` + extractJSFunc(t, html, "inferAmzDateFromLoan") + `
` + extractJSFunc(t, html, "formatMoneyField") + `
function moneyOf(s) { var el = { value: s }; formatMoneyField(el); return el.value; }
__perYr = '12';
var monthly = {
  fp_6_13: computeDefaultFirstPayment('2024-06-13'),
  fp_6_01: computeDefaultFirstPayment('2024-06-01'),
  fp_12_15: computeDefaultFirstPayment('2024-12-15'),
};
__perYr = '4';  var fp_q_6_13 = computeDefaultFirstPayment('2024-06-13');
__perYr = '26'; var fp_biweekly = computeDefaultFirstPayment('2024-06-13');
console.log(JSON.stringify({
  fp_6_13: monthly.fp_6_13, fp_6_01: monthly.fp_6_01, fp_12_15: monthly.fp_12_15,
  fp_q_6_13: fp_q_6_13, fp_biweekly: fp_biweekly,
  infer_81: inferAmzDateFromLoan('8/1', '2024-06-13'),
  infer_11: inferAmzDateFromLoan('1/1', '2024-06-13'),
  infer_full: inferAmzDateFromLoan('08/01/2024', '2024-06-13'),
  money_100000: moneyOf('100000'),
  money_bad: moneyOf('abc'),
}));
`
	out := runNode(t, harness, "")
	var res map[string]*string
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	wantStr := map[string]string{
		"fp_6_13":      "2024-08-01", // client's example: 6/13 → 8/1
		"fp_6_01":      "2024-07-01", // on the 1st → one period
		"fp_12_15":     "2025-02-01", // crosses year boundary
		"fp_q_6_13":    "2024-12-01", // quarterly: +2 quarters
		"infer_81":     "2024-08-01",
		"infer_11":     "2025-01-01", // rolls forward (1/1/2024 precedes loan)
		"money_100000": "$100,000.00",
		"money_bad":    "abc", // unparseable left as-is
	}
	for k, want := range wantStr {
		if res[k] == nil || *res[k] != want {
			got := "null"
			if res[k] != nil {
				got = *res[k]
			}
			t.Errorf("%s = %s, want %s", k, got, want)
		}
	}
	for _, k := range []string{"fp_biweekly", "infer_full"} {
		if res[k] != nil {
			t.Errorf("%s = %q, want null", k, *res[k])
		}
	}
}
