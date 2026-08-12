package api

// DECISION 3a.18, resolved against the ORIGINAL at r55b: what should happen
// when a WHAT-IF GENERATED row leaves a cell's domain?
//
// DOS's answer, read at MortgageScreenUnit.pas:795-900 and ~1050:
//
//   * GenerateMtgRows -> CopyRowWithIncrement adds the increment and writes
//     STRAIGHT INTO THE RECORD, then calls DoCalculation. It NEVER enters
//     MortgageGridVerifyCellString, which is a grid EDIT event. So a generated
//     row is NOT domain-validated: DOS will draw a 130-year mortgage.
//   * The caller then tests `errorflag` after every CopyRowWithIncrement, and
//     on an ENGINE error shows "Error with generated row, aborting row
//     generation", ZeroMortgages that row, repaints it, and EXITS — abandoning
//     the rest of the generation.
//
// Round 55 shipped a validator that fired on generated rows too (because the
// client reposts every generated row through /calc), which refused rows DOS
// draws — §97. This file pins BOTH halves of DOS's behaviour.

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"
)

// ---- half one: a generated row is NOT domain-validated ----

func TestR55bGeneratedRowSkipsDomainValidator(t *testing.T) {
	// Each of these is refused when TYPED (see zzr55_mtg_domain_test.go) and
	// must be ANSWERED when generated, because DOS's generator never consults
	// VerifyCellString.
	cases := []struct {
		name string
		body string
	}{
		{"years", `{"price":200000,"pctDown":0.2,"years":130,"rate":0.06,"generated":true}`},
		{"rate", `{"price":200000,"pctDown":0.2,"years":30,"rate":1.20,"generated":true}`},
		{"points", `{"price":200000,"pctDown":0.2,"years":30,"rate":0.06,"points":0.15,"generated":true}`},
		{"balloonYears", `{"price":200000,"pctDown":0.2,"years":30,"rate":0.06,"balloonYears":120,"balloonAmount":50000,"generated":true}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out := mtgPost(t, HandleMortgageCalc, "/api/mortgage/calc",
				json.RawMessage(c.body))
			msg, _ := out["error"].(string)
			if isMtgDomainMessage(msg) {
				t.Fatalf("generated row refused by the DOMAIN validator (%q) — "+
					"DOS's CopyRowWithIncrement never enters "+
					"MortgageGridVerifyCellString (3a.18)", msg)
			}
			if code != http.StatusOK {
				t.Fatalf("generated row got %d, want 200; error=%q", code, msg)
			}
		})
	}
}

// 🚨 THE FLAG MUST FAIL SAFE. Absent or explicitly false ⇒ VALIDATED. Without
// this, finding 5 is closed only for callers who forget to lie.
func TestR55bGeneratedFlagFailsSafe(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		{"absent", `{"price":200000,"pctDown":0.2,"years":130,"rate":0.06}`},
		{"false", `{"price":200000,"pctDown":0.2,"years":130,"rate":0.06,"generated":false}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, out := mtgPost(t, HandleMortgageCalc, "/api/mortgage/calc",
				json.RawMessage(c.body))
			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 — a row that does not claim "+
					"generation provenance must keep finding 5's guard", code)
			}
			if msg, _ := out["error"].(string); msg != "Years must be between 0 and 100" {
				t.Errorf("error = %q, want the Years domain message", msg)
			}
		})
	}
}

// 🚨 AND IT MUST SUPPRESS ONLY THE DOMAIN CHECK. DOS's generated row still goes
// through DoCalculation, and RecordError still sets errorflag — that engine
// refusal is the ENTIRE trigger for DOS's abort. A flag that silenced the
// engine too would make the abort unreachable and this whole file vacuous.
func TestR55bGeneratedDoesNotSuppressEngineRefusals(t *testing.T) {
	// Amt Borrowed tiny against Price ⇒ solved %Down ≥ 0.995 ⇒ Mortgage.pas:217
	// RecordError. Not a domain violation; the engine's own refusal.
	code, out := mtgPost(t, HandleMortgageCalc, "/api/mortgage/calc",
		json.RawMessage(`{"price":200000,"financed":100,"years":30,"rate":0.06,"generated":true}`))
	msg, _ := out["error"].(string)
	if msg == "" {
		t.Fatalf("a generated row that the ENGINE refuses came back clean "+
			"(status %d) — the generated flag has become a blanket bypass and "+
			"DOS's abort path is now unreachable", code)
	}
	if isMtgDomainMessage(msg) {
		t.Fatalf("expected an ENGINE refusal, got the domain message %q", msg)
	}
}

// The flag is on /calc only. /compare and /whatif take TYPED rows — the compare
// panel is two typed screens and the What-If BASE is the selected source row,
// which DOS validates via CalculateRows before it ever opens the dialog. A
// stray "generated" key in those payloads must change nothing.
func TestR55bGeneratedFlagIsNotHonouredOnTypedEndpoints(t *testing.T) {
	row := `{"price":200000,"pctDown":0.2,"years":130,"rate":0.06,"generated":true}`
	for _, c := range []struct {
		name, path string
		h          http.HandlerFunc
		body       string
	}{
		{"compare", "/api/mortgage/compare", HandleMortgageCompare,
			`{"a":` + row + `,"b":{"price":200000,"pctDown":0.2,"years":30,"rate":0.06}}`},
		{"whatif", "/api/mortgage/whatif", HandleMortgageWhatIf,
			`{"base":` + row + `,"vary":"rate","increment":0.005,"count":3}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, out := mtgPost(t, c.h, c.path, json.RawMessage(c.body))
			if code != http.StatusBadRequest {
				t.Fatalf("%s: status = %d, want 400 — these endpoints take "+
					"TYPED rows and must keep finding 5's guard", c.path, code)
			}
			if msg, _ := out["error"].(string); msg != "Years must be between 0 and 100" {
				t.Errorf("%s: error = %q, want the Years domain message", c.path, msg)
			}
		})
	}
}

// ---- half two: DOS's abort, EXECUTED, not source-scanned ----

// 🚨 THIS RUNS THE SHIPPED JS. A source-scanning guard cannot pin behaviour in
// another language — round 54 wrote two such guards and both could print a
// false green. calcGeneratedRows is extracted from the shipped index.html and
// executed against a stub apiPost whose scripted replies drive each branch.
func runCalcGeneratedRows(t *testing.T, replies string, rows int) map[string]any {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	html := mustReadIndexHTML(t)

	harness := `
'use strict';
` + extractJS(t, html, "blankMtgRowCells") + `
` + extractJS(t, html, "calcGeneratedRows") + `
var MTG_FIELDS = [{key:'price'},{key:'points'},{key:'pctDown'},{key:'cash'},
                  {key:'financed'},{key:'years'},{key:'rate'},{key:'tax'},
                  {key:'monthly'},{key:'balloonYears'},{key:'balloonAmount'}];
var NROWS = ` + itoa(rows) + `;
var CELLS = [], mtgStatus = [];
for (var r = 0; r < NROWS; r++) {
  CELLS.push({}); mtgStatus.push({});
  MTG_FIELDS.concat([{key:'apr'}]).forEach(function(f){
    var cls = [];
    CELLS[r][f.key] = { value: 'seed', dataset: {hardened:'1'},
      classList: { add:function(c){cls.push(c);},
                   remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);},
                   contains:function(c){return cls.indexOf(c)>=0;} } };
  });
}
function getMtgCell(r, k) { return (CELLS[r] || {})[k] || null; }
var ERRTEXT = '';
var document = { getElementById: function(id) {
  return { set textContent(v){ if(id==='mtg-error') ERRTEXT = v; },
           get textContent(){ return ERRTEXT; },
           set innerHTML(v){ if(id==='mtg-error') ERRTEXT = v; } };
}};
// Every generated row starts with real data so getMtgRowData is never empty.
function getMtgRowData(r) { return { price: 200000, years: 30, rate: 0.06 }; }
var POSTED = [];
var REPLIES = ` + replies + `;
var painted = [];
function updateMtgRowUI(r, d) { painted.push(r); }
function markMtgErrorRow(r) {}
function clearFieldErrors() {}
async function apiPost(url, body) {
  POSTED.push({url: url, body: body});
  var i = POSTED.length - 1;
  return REPLIES[i] || {};
}
calcGeneratedRows(1, NROWS - 1).then(function(ok) {
  var blanked = [];
  for (var r = 0; r < NROWS; r++) {
    if (CELLS[r]['price'].value === '') blanked.push(r);
  }
  console.log(JSON.stringify({
    ok: ok, posted: POSTED, painted: painted,
    blanked: blanked, err: ERRTEXT
  }));
});
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	return got
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// Every generated row must carry generated:true, or the server's domain
// validator fires and §97 is back.
func TestR55bGeneratedRowsAreMarkedOnTheWire(t *testing.T) {
	got := runCalcGeneratedRows(t, `[{},{},{}]`, 4)
	posted, _ := got["posted"].([]any)
	if len(posted) != 3 {
		t.Fatalf("posted %d rows, want 3", len(posted))
	}
	for i, p := range posted {
		m, _ := p.(map[string]any)
		body, _ := m["body"].(map[string]any)
		if body["generated"] != true {
			t.Errorf("row %d posted without generated:true (%v) — the server "+
				"will apply finding 5's validator and refuse rows DOS draws",
				i, body)
		}
		if m["url"] != "/api/mortgage/calc" {
			t.Errorf("row %d posted to %v", i, m["url"])
		}
	}
	if got["ok"] != true {
		t.Errorf("ok = %v, want true on an all-clean run", got["ok"])
	}
}

// DOS: on an ENGINE error, blank that row, show the message, and ABANDON the
// rest of the generation.
func TestR55bEngineErrorAbortsTheGeneration(t *testing.T) {
	// row 1 answers, row 2 is refused by the engine, row 3 must never be asked.
	got := runCalcGeneratedRows(t,
		`[{},{error:'Years must be a positive whole number of years.'},{}]`, 4)

	posted, _ := got["posted"].([]any)
	if len(posted) != 2 {
		t.Fatalf("posted %d requests, want 2 — DOS exits GenerateMtgRows at the "+
			"failing row and never creates the rows after it", len(posted))
	}
	if got["err"] != "Error with generated row, aborting row generation" {
		t.Errorf("message = %q, want DOS's own text from "+
			"MortgageScreenUnit.pas", got["err"])
	}
	blanked := toIntSet(got["blanked"])
	for _, r := range []int{2, 3} {
		if !blanked[r] {
			t.Errorf("row %d not blanked — DOS ZeroMortgages the failing row, "+
				"and the rows after it are ones DOS never creates", r)
		}
	}
	if blanked[1] {
		t.Errorf("row 1 was blanked, but it answered — DOS keeps the rows it " +
			"successfully generated before the failure")
	}
	if got["ok"] != false {
		t.Errorf("ok = %v, want false", got["ok"])
	}
}

// 🚨 A TRANSPORT FAILURE IS NOT AN ENGINE REFUSAL (round 54's first audit).
// It must neither blank a row nor claim DOS's abort.
func TestR55bTransportFailureDoesNotBlankOrClaimAbort(t *testing.T) {
	got := runCalcGeneratedRows(t,
		`[{},{error:'network unreachable', transport:true},{}]`, 4)
	if blanked := toIntSet(got["blanked"]); len(blanked) != 0 {
		t.Errorf("a transport failure blanked rows %v — the ENGINE refused "+
			"nothing and correct work must survive a dropped request", blanked)
	}
	if s, _ := got["err"].(string); s == "Error with generated row, aborting row generation" {
		t.Errorf("a transport failure printed DOS's ENGINE-error message; the " +
			"message is a claim and this one would be false")
	}
}

// POSITIVE CONTROL: the clean run must actually paint every row, or the two
// tests above could pass against a function that does nothing at all (R49).
func TestR55bCleanGenerationPaintsEveryRow(t *testing.T) {
	got := runCalcGeneratedRows(t, `[{},{},{}]`, 4)
	painted, _ := got["painted"].([]any)
	if len(painted) != 3 {
		t.Fatalf("painted %d rows, want 3 — the abort tests prove nothing if "+
			"the clean path never paints", len(painted))
	}
	if blanked := toIntSet(got["blanked"]); len(blanked) != 0 {
		t.Errorf("clean run blanked %v", blanked)
	}
}

func toIntSet(v any) map[int]bool {
	out := map[int]bool{}
	arr, _ := v.([]any)
	for _, e := range arr {
		if f, ok := e.(float64); ok {
			out[int(f)] = true
		}
	}
	return out
}
