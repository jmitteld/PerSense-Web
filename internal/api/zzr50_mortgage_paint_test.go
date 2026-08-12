package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// ROUND-50 MORTGAGE CONSUMER COVERAGE — REDONE IN ROUND 54.
//
// 🚨 PROVENANCE. Round 50 fixed mortgage findings 1, 2, 3, 6 and 13 and its
// code was never pushed; the conversation is lost and Nate confirmed at the top
// of round 52 that the work must be REDONE, not recovered. This file is the
// redo. The findings themselves are round 46's read-back audit, and every one
// of them was RE-RUN AT HEAD (364f923) before anything was changed (R60) —
// all five still reproduced.
//
// 🚨 WHY IT IS HERE AT ALL. The mortgage screen's paint layer had, in round 46's
// words, no consumer coverage that could fail on these defects:
//   - TestFrontendMtgOutputEchoSweep:1980 does `if cell == "" { continue }`,
//     which EXCUSES a blank cell — and a blank cell is exactly findings 1, 2
//     and 6. "A guard that skips the empty case cannot find the bug whose
//     symptom is emptiness" (R55).
//   - :1894 marks pctDown as an INPUT in every case, so the sweep never
//     exercises the arms where % Down is a solved OUTPUT.
//   - nothing anywhere posts a mortgage row carrying Tax and a balloon and a
//     comparison together.
//   - `uidiff` is #amz-* selectors only and DOES NOT COVER THIS SCREEN.
//
// Every test below asserts on the DISPLAYED cell produced by the SHIPPED
// updateMtgRowUI / clearMtgOutputs, driven from the REAL handler's wire
// output — R42's "at the consumer", not at the struct.
//
// ⚠️ Each test is written to FAIL on the pre-fix tree; the mutants that drive
// them, and which test each mutant killed, are recorded in the round-54 record.

// callMortgageCompare POSTs to /api/mortgage/compare and returns the decoded
// response. The sibling of callMortgage (verify_web_help_examples_test.go:80);
// no compare equivalent existed, which is part of why finding 3 shipped.
func callMortgageCompare(t *testing.T, body string) MortgageCompareResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/compare",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandleMortgageCompare(w, req)
	var resp MortgageCompareResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v (status %d, body %s)", err, w.Code, w.Body.String())
	}
	return resp
}

// callMortgageRaw returns the handler's RAW response bytes. Needed because an
// omitempty defect is invisible to a test that decodes into a struct and then
// re-marshals it — see TestR50MtgFinding2.
func callMortgageRaw(t *testing.T, body string) []byte {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/calc",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandleMortgageCalc(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	return w.Body.Bytes()
}

// mtgPaint runs the shipped mortgage paint JS over one wire response and
// returns the DISPLAYED cell text and the green (cell-output) flags.
//
// `inputs` names the cells the user typed, exactly as mtgStatus would carry
// them; `pre` optionally seeds cells with text ALREADY on screen, which is what
// makes a stale-output assertion possible at all.
func mtgPaint(t *testing.T, resp any, inputs []string, pre map[string]string) (map[string]string, map[string]bool) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping mortgage paint test")
	}
	html := mustReadIndexHTML(t)
	rb, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	inJSON, _ := json.Marshal(inputs)
	preJSON, _ := json.Marshal(pre)

	harness := `
'use strict';
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtRate") + `
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "updateMtgRowUI") + `
var MTG_ALL = ['price','points','pctDown','cash','financed','years','rate','tax','monthly','balloonYears','balloonAmount','apr'];
var SEL = {};
function mkCell() { var cls=[]; return { value:'', dataset:{}, classList:{ add:function(c){if(cls.indexOf(c)<0)cls.push(c);}, remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);}, contains:function(c){return cls.indexOf(c)>=0;} } }; }
function selFor(f){ return '#mtg-body input[data-row="0"][data-field="'+f+'"]'; }
var document = { querySelector:function(s){ return (s in SEL)?SEL[s]:null; } };
var mtgStatus = [{}];
MTG_ALL.forEach(function(f){ SEL[selFor(f)] = mkCell(); });
var pre = ` + string(preJSON) + `;
for (var f in pre) { SEL[selFor(f)].value = pre[f]; mtgStatus[0][f] = 'output'; SEL[selFor(f)].classList.add('cell-output'); }
` + string(inJSON) + `.forEach(function(f){ mtgStatus[0][f] = 'input'; });
updateMtgRowUI(0, ` + string(rb) + `);
var disp = {}, green = {};
MTG_ALL.forEach(function(f){ disp[f] = SEL[selFor(f)].value; green[f] = SEL[selFor(f)].classList.contains('cell-output'); });
console.log(JSON.stringify({disp: disp, green: green}));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var got struct {
		Disp  map[string]string `json:"disp"`
		Green map[string]bool   `json:"green"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	return got.Disp, got.Green
}

// mtgClearOutputs runs the shipped clearMtgOutputs over a row that already has
// text on screen and returns what is left displayed. This is finding 6's ERROR
// half: the success path had no "status went empty" branch, and the error path
// did no clearing at all.
func mtgClearOutputs(t *testing.T, pre map[string]string, inputs []string) map[string]string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping mortgage paint test")
	}
	html := mustReadIndexHTML(t)
	preJSON, _ := json.Marshal(pre)
	inJSON, _ := json.Marshal(inputs)

	harness := `
'use strict';
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "clearMtgOutputs") + `
var MTG_FIELDS = [{key:'price'},{key:'points'},{key:'pctDown'},{key:'cash'},{key:'financed'},{key:'years'},{key:'rate'},{key:'tax'},{key:'monthly'},{key:'balloonYears'},{key:'balloonAmount'}];
var MTG_ALL = ['price','points','pctDown','cash','financed','years','rate','tax','monthly','balloonYears','balloonAmount','apr'];
var SEL = {};
function mkCell() { var cls=[]; return { value:'', dataset:{}, classList:{ add:function(c){if(cls.indexOf(c)<0)cls.push(c);}, remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);}, contains:function(c){return cls.indexOf(c)>=0;} } }; }
function selFor(f){ return '#mtg-body input[data-row="0"][data-field="'+f+'"]'; }
var document = { querySelector:function(s){ return (s in SEL)?SEL[s]:null; } };
var mtgStatus = [{}];
MTG_ALL.forEach(function(f){ SEL[selFor(f)] = mkCell(); });
var pre = ` + string(preJSON) + `;
for (var f in pre) { SEL[selFor(f)].value = pre[f]; mtgStatus[0][f] = 'output'; }
` + string(inJSON) + `.forEach(function(f){ mtgStatus[0][f] = 'input'; });
clearMtgOutputs(0);
var disp = {};
MTG_ALL.forEach(function(f){ disp[f] = SEL[selFor(f)].value; });
console.log(JSON.stringify(disp));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var disp map[string]string
	if err := json.Unmarshal(out, &disp); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	return disp
}

// ---------------------------------------------------------------------------
// FINDING 1 — the wire carries VALUES but not STATUSES, so every
// legitimately-computed ZERO is left blank.
// ---------------------------------------------------------------------------

// 1/9. The wire must carry a status for every one of the eleven grid cells.
// TMortgage2Grid (MortgageScreenUnit.pas:427-492) paints on the status and
// never on the value; the port had no status field at all.
func TestR50MtgFinding1_StatusesOnTheWire(t *testing.T) {
	resp := callMortgage(t, `{"price":200000,"financed":200000,"years":30,"rate":0.0725}`)
	if resp.Error != "" {
		t.Fatalf("unexpected engine refusal: %s", resp.Error)
	}
	if resp.Statuses == nil {
		t.Fatalf("finding 1: response carries no statuses at all")
	}
	for _, k := range []string{"price", "points", "pctDown", "cash", "financed",
		"years", "rate", "tax", "monthly", "balloonYears", "balloonAmount"} {
		if _, ok := resp.Statuses[k]; !ok {
			t.Errorf("finding 1: no status for cell %q", k)
		}
	}
	// The whole point: these two are SOLVED and their value is exactly zero.
	// The engine answered them; the wire must say so.
	if got := resp.Statuses["pctDown"]; got != "output" {
		t.Errorf("finding 1: pctDown status = %q, want output (it is a solved 0)", got)
	}
	if got := resp.Statuses["cash"]; got != "output" {
		t.Errorf("finding 1: cash status = %q, want output (it is a solved 0)", got)
	}
	if resp.PctDown != 0 || resp.Cash != 0 {
		t.Fatalf("premise broken: pctDown=%v cash=%v, expected both 0", resp.PctDown, resp.Cash)
	}
}

// 2/9. A solved ZERO must PAINT, not blank. This is the consumer half of
// finding 1 and the one the existing sweep cannot make: its `if cell == ""
// { continue }` explicitly excuses the symptom (R55).
func TestR50MtgFinding1_SolvedZeroPaints(t *testing.T) {
	resp := callMortgage(t, `{"price":200000,"financed":200000,"years":30,"rate":0.0725}`)
	if resp.Error != "" {
		t.Fatalf("unexpected engine refusal: %s", resp.Error)
	}
	disp, green := mtgPaint(t, resp, []string{"price", "financed", "years", "rate"}, nil)

	if disp["pctDown"] == "" {
		t.Errorf("finding 1: %% Down painted BLANK for a solved 0 — the user cannot "+
			"tell 'the engine said 0' from 'the engine did not answer' (disp=%q)", disp["pctDown"])
	}
	if disp["cash"] == "" {
		t.Errorf("finding 1: Cash Required painted BLANK for a solved 0 (disp=%q)", disp["cash"])
	}
	// And they must be marked as engine output, not left looking like input.
	if !green["pctDown"] || !green["cash"] {
		t.Errorf("finding 1: solved zeros not marked cell-output (pctDown=%v cash=%v)",
			green["pctDown"], green["cash"])
	}
}

// ---------------------------------------------------------------------------
// FINDING 2 — `apr,omitempty` drops an exact-zero APR and the consumer blanks
// the cell. Reachable through 0% seller or family financing.
// ---------------------------------------------------------------------------

// 3/9.
func TestR50MtgFinding2_ZeroAPRSurvivesTheWireAndPaints(t *testing.T) {
	resp := callMortgage(t, `{"price":200000,"pctDown":0.2,"years":30,"rate":0}`)
	if resp.Error != "" {
		t.Fatalf("unexpected engine refusal: %s", resp.Error)
	}
	if !resp.APRConverged {
		t.Fatalf("premise broken: APR did not converge on a 0%% loan")
	}
	if resp.APR != 0 {
		t.Fatalf("premise broken: APR = %v, expected exactly 0", resp.APR)
	}
	// The wire must distinguish "computed 0.00%%" from "not computed".
	if got := resp.Statuses["apr"]; got != "output" {
		t.Errorf("finding 2: apr status = %q, want output", got)
	}
	// 🚨 ASSERT ON THE HANDLER'S RAW WIRE BYTES, AT THE TOP LEVEL.
	// The first version of this test re-marshalled the decoded struct and
	// looked for the substring `"apr":`. It was vacuous TWICE OVER and the
	// mutation matrix caught it: `"apr":` also matches the STATUSES map's own
	// `"apr":"output"` entry, and a re-marshal is not what the handler wrote.
	// The mutant restoring `json:"apr,omitempty"` survived. r53's trap — "a
	// test's name can be a false claim about what it pins" — at first attempt.
	raw := callMortgageRaw(t, `{"price":200000,"pctDown":0.2,"years":30,"rate":0}`)
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("decode raw wire: %v (%s)", err, raw)
	}
	v, ok := top["apr"]
	if !ok {
		t.Fatalf("finding 2: top-level `apr` key ABSENT from the wire for an "+
			"exact-zero APR — omitempty dropped it. wire=%s", raw)
	}
	if string(v) != "0" {
		t.Errorf("finding 2: apr = %s on the wire, want 0", v)
	}

	// And the painted cell must carry the actual number, not merely be
	// non-empty: with `apr` dropped the renderer computes undefined*100 and
	// paints "NaN%", which a non-emptiness check happily accepts.
	disp, _ := mtgPaint(t, resp, []string{"price", "pctDown", "years", "rate"}, nil)
	if got := disp["apr"]; got != "0.0000%" {
		t.Errorf("finding 2: APR cell painted %q for a converged 0.00%% APR, want %q",
			got, "0.0000%")
	}
}

// ---------------------------------------------------------------------------
// FINDING 3 — /api/mortgage/compare cannot carry Tax or the balloon; the UI
// sends them and the decoder drops them silently.
// ---------------------------------------------------------------------------

// 4/9. Measured at 215.20 bp before the fix. DOS computes on (monthly - tax)
// at Mortgage.pas:337 and takes the whole record at :628-631.
func TestR50MtgFinding3_CompareCarriesTax(t *testing.T) {
	const row = `{"financed":200000,"monthly":1664.35,"years":30,"rate":0.0725,"points":0.015,"tax":300}`
	withTax := callMortgageCompare(t, fmt.Sprintf(`{"a":%s,"b":%s}`, row, row))
	if withTax.Error != "" {
		t.Fatalf("compare refused: %s", withTax.Error)
	}
	const rowNoTax = `{"financed":200000,"monthly":1664.35,"years":30,"rate":0.0725,"points":0.015}`
	noTax := callMortgageCompare(t, fmt.Sprintf(`{"a":%s,"b":%s}`, rowNoTax, rowNoTax))
	if noTax.Error != "" {
		t.Fatalf("compare refused: %s", noTax.Error)
	}
	// If Tax is still being dropped these are byte-identical, which is
	// exactly how the defect was measured at HEAD.
	if withTax.APR1 == noTax.APR1 {
		t.Fatalf("finding 3: compare APR is IDENTICAL with and without $300 of "+
			"monthly tax (%v) — the field is still being dropped", withTax.APR1)
	}
	// And the tax-bearing answer must be the true one: the same row through
	// /calc, which has always carried Tax. Reference measured at HEAD:
	// true 7.402711%, compare printed 9.554695% — 215.20 bp.
	ref := callMortgage(t, `{"pctDown":0,"financed":200000,"monthly":1664.35,"years":30,"rate":0.0725,"points":0.015,"tax":300}`)
	if ref.Error != "" {
		t.Fatalf("reference row refused: %s", ref.Error)
	}
	if d := math.Abs(withTax.APR1 - ref.APR); d > 1e-5 {
		t.Errorf("finding 3: compare APR %.8f disagrees with calc APR %.8f by %.2f bp",
			withTax.APR1, ref.APR, d*10000)
	}
}

// 5/9. Finding 3's SECOND consequence: with no balloon fields on the compare
// input, BalloonStat could never be BalloonKnown/BalloonUnk there, so
// tryBalloonDates (mortgage.go:796-823 — a faithful port of
// Mortgage.pas:462-508, WITH ITS OWN TEST FILE) was structurally unreachable
// from the shipped Compare APR button. R40 / CAUTION 11.
func TestR50MtgFinding3_CompareCarriesTheBalloon(t *testing.T) {
	const plain = `{"financed":200000,"monthly":1400,"years":30,"rate":0.0725}`
	const balloon = `{"financed":200000,"monthly":1400,"years":30,"rate":0.0725,"balloonYears":10,"balloonAmount":150000}`
	a := callMortgageCompare(t, fmt.Sprintf(`{"a":%s,"b":%s}`, plain, plain))
	b := callMortgageCompare(t, fmt.Sprintf(`{"a":%s,"b":%s}`, balloon, plain))
	if a.Error != "" || b.Error != "" {
		t.Fatalf("compare refused: %q / %q", a.Error, b.Error)
	}
	if a.APR1 == b.APR1 {
		t.Fatalf("finding 3: a $150,000 balloon at year 10 changed the compare APR "+
			"not at all (%v) — the balloon is still being dropped", a.APR1)
	}
}

// ---------------------------------------------------------------------------
// FINDING 6 — no "clear the cell whose status went empty" branch, on EITHER
// the success or the error path, so stale outputs survive silently.
// ---------------------------------------------------------------------------

// 6/9. THE SUCCESS PATH. Solve a row, then type Balloon Yrs: the wire comes
// back with monthly 0 and no APR, and the screen kept painting the previous
// solve's $1,364.35. The original blanks the Monthly cell on exactly that edit
// (MortgageScreenUnit.pas:215-219). §81(1) family.
func TestR50MtgFinding6_StaleOutputClearedOnSuccess(t *testing.T) {
	solved := callMortgage(t, `{"price":250000,"pctDown":0.2,"years":30,"rate":0.0725}`)
	if solved.Error != "" || solved.Monthly == 0 {
		t.Fatalf("premise broken: %q monthly=%v", solved.Error, solved.Monthly)
	}
	staleMonthly := fmt.Sprintf("$%.2f", solved.Monthly)

	// Now the user types Balloon Yrs 10 on the same row.
	next := callMortgage(t, `{"price":250000,"pctDown":0.2,"years":30,"rate":0.0725,"balloonYears":10}`)
	if next.Error != "" {
		t.Fatalf("unexpected refusal: %s", next.Error)
	}
	if next.Monthly != 0 {
		t.Fatalf("premise broken: expected monthly 0 on the balloon edit, got %v", next.Monthly)
	}
	if got := next.Statuses["monthly"]; got != "empty" {
		t.Fatalf("premise broken: monthly status = %q, want empty", got)
	}

	disp, _ := mtgPaint(t, next,
		[]string{"price", "pctDown", "years", "rate", "balloonYears"},
		map[string]string{"monthly": staleMonthly, "apr": "7.2500%"})

	if disp["monthly"] != "" {
		t.Errorf("finding 6: Monthly still shows %q after an edit the engine did not "+
			"answer (stale value was %q)", disp["monthly"], staleMonthly)
	}
	if disp["apr"] != "" {
		t.Errorf("finding 6: APR still shows %q after an unanswered edit", disp["apr"])
	}
}

// 7/9. THE ERROR PATH. calcMortgageRow printed the message and returned, so a
// REFUSED row went on displaying the previous solve's numbers as though they
// still described what is on screen. Nothing cleared them at all.
func TestR50MtgFinding6_StaleOutputClearedOnError(t *testing.T) {
	// A genuinely refused row, to prove the premise rather than assume it.
	bad := callMortgage(t, `{"financed":200000,"monthly":1664.35,"years":30,"rate":0.0725,"tax":300}`)
	if bad.Error == "" {
		t.Fatalf("premise broken: expected a refusal, got a result")
	}

	disp := mtgClearOutputs(t,
		map[string]string{"monthly": "$1,364.35", "apr": "7.2500%", "cash": "$50,000.00"},
		[]string{"financed", "years", "rate", "tax"})

	for _, k := range []string{"monthly", "apr", "cash"} {
		if disp[k] != "" {
			t.Errorf("finding 6 (error path): %s still shows %q on a refused row", k, disp[k])
		}
	}
}

// 8/9. The error path must NOT eat what the user typed.
func TestR50MtgFinding6_ErrorPathPreservesInputs(t *testing.T) {
	disp := mtgClearOutputs(t,
		map[string]string{"monthly": "$1,364.35", "price": "$250,000.00"},
		[]string{"price"}) // price is INPUT here, monthly is a stale OUTPUT

	if disp["price"] != "$250,000.00" {
		t.Errorf("error path erased a typed input: price = %q", disp["price"])
	}
	if disp["monthly"] != "" {
		t.Errorf("error path left a stale output: monthly = %q", disp["monthly"])
	}
}

// ---------------------------------------------------------------------------
// FINDING 13 — mtgLineFromInput left BalloonStat at Go's zero value, which is
// BalloonKnown (types/enums.go:83). Round 46 filed it as LATENT: "live the day
// a balloon field joins that struct." Finding 3 is that day.
// ---------------------------------------------------------------------------

// 9/9. 🚨 THE REACHABLE ROW IS `Balloon Amt filled, Balloon Yrs blank`.
// Correct is BalloonBlank; the Go zero value is BalloonKnown, which is a KNOWN
// balloon of the given size due at year 0. /api/mortgage/calc reports that
// combination as an error; compare has no error channel and so silently
// computed nonsense from it.
//
// MEASURED, round 54, fixed vs mutant over 702 eligible compare rows: 47
// differ, all of this shape. Exemplar: apr1 11050.183232 (1,105,018%) against
// 0.051762 (5.1762%).
func TestR50MtgFinding13_BalloonAmountWithoutYearsIsNotAKnownBalloon(t *testing.T) {
	const bad = `{"financed":150000,"monthly":1200,"years":15,"rate":0.0725,"balloonAmount":150000}`
	const plain = `{"financed":200000,"monthly":1400,"years":30,"rate":0.0725}`
	got := callMortgageCompare(t, fmt.Sprintf(`{"a":%s,"b":%s}`, bad, plain))
	if got.Error != "" {
		t.Fatalf("compare refused: %s", got.Error)
	}
	// A mortgage APR is a rate, not a four-digit multiple. The mutant returns
	// 19744.98 here; anything of that order means BalloonStat came through as
	// BalloonKnown with When == 0.
	if got.APR1 > 1.0 {
		t.Errorf("finding 13: apr1 = %v on a row with Balloon Amt but no Balloon "+
			"Yrs — BalloonStat defaulted to BalloonKnown at year 0", got.APR1)
	}
	if !got.APR1Converged {
		t.Errorf("finding 13: apr1 did not converge (%v) — the balloon was treated "+
			"as a known balloon due at year 0", got.APR1)
	}
}

// ---------------------------------------------------------------------------
// GUARDS ADDED BY ROUND 54's FIRST ADVERSARIAL PASS.
//
// The nine tests above shipped with four measured coverage holes: three
// mutants survived them (hardened-cell preservation in BOTH clearing paths,
// and the status->cell mapping), and two of this round's own fixes turned an
// ANSWER into a REFUSAL (R67) with nothing watching. These close them.
// ---------------------------------------------------------------------------

// 10/9. HARDENED CELLS MUST SURVIVE RECALCULATION, AND A TYPED INPUT MUST
// SURVIVE AN `empty` STATUS.
//
// ⚠️ WHAT THIS DOES *NOT* PIN, AND WHY. Deleting clearMtgOutputs' hardened
// guard (`index.html`, `clearMtgOutputs`) is an EQUIVALENT MUTANT, not a
// coverage gap: `hardenMtgCell` sets `mtgStatus[row][field] = 'input'` on the
// same cell it marks `dataset.hardened = '1'`, so the very next line
// (`if (mtgStatus[row][key] === 'input') return;`) already returns for every
// reachable hardened cell. The guard is defence in depth and no test can kill
// it without manufacturing a state the app cannot produce. Measured, round 54
// audit pass 1 (mutant A1), and stated rather than papered over.
//
// The two guards below ARE load-bearing:
//   - updateMtgRowUI's hardened `continue` (mutant A3): its `!== 'input'` test
//     controls only the CLASS and the status, not the value write, so without
//     the hardened guard a recalculation overwrites the frozen value.
//   - the empty-branch's `!== 'input'` test (mutant A2): without it, a cell the
//     client holds as input is erased whenever the wire reports `empty`.
func TestR54MtgHardenedCellsSurviveClearing(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	html := mustReadIndexHTML(t)
	// (a) A SOLVED response — monthly is an `output` the paint loop will write.
	// A hardened Monthly must survive it.
	solved := callMortgage(t, `{"price":250000,"pctDown":0.2,"years":30,"rate":0.0725}`)
	if solved.Statuses["monthly"] != "output" || solved.Monthly == 0 {
		t.Fatalf("premise broken: monthly status=%q value=%v",
			solved.Statuses["monthly"], solved.Monthly)
	}
	// (b) A response whose monthly status is `empty` — the branch that blanks.
	empt := callMortgage(t, `{"price":250000,"pctDown":0.2,"years":30,"rate":0.0725,"balloonYears":10}`)
	if empt.Statuses["monthly"] != "empty" {
		t.Fatalf("premise broken: monthly status = %q", empt.Statuses["monthly"])
	}
	solvedJSON, _ := json.Marshal(solved)
	rb, _ := json.Marshal(empt)

	harness := `
'use strict';
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtRate") + `
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "clearMtgOutputs") + `
` + extractJS(t, html, "updateMtgRowUI") + `
var MTG_FIELDS = [{key:'price'},{key:'points'},{key:'pctDown'},{key:'cash'},{key:'financed'},{key:'years'},{key:'rate'},{key:'tax'},{key:'monthly'},{key:'balloonYears'},{key:'balloonAmount'}];
var MTG_ALL = MTG_FIELDS.map(function(f){return f.key;}).concat(['apr']);
var SEL = {};
function mkCell(){ var cls=[]; return {value:'',dataset:{},classList:{add:function(c){if(cls.indexOf(c)<0)cls.push(c);},remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);},contains:function(c){return cls.indexOf(c)>=0;}}}; }
function selFor(f){ return '#mtg-body input[data-row="0"][data-field="'+f+'"]'; }
var document = { querySelector:function(s){ return (s in SEL)?SEL[s]:null; } };
function fresh(){ SEL={}; MTG_ALL.forEach(function(f){SEL[selFor(f)]=mkCell();});
  mtgStatus=[{}];
  // monthly is a HARDENED cell the user froze at a value of their choosing.
  SEL[selFor('monthly')].value='$9,999.99'; SEL[selFor('monthly')].dataset.hardened='1';
  mtgStatus[0]['monthly']='input'; }
var mtgStatus=[{}];
var out={};
fresh(); clearMtgOutputs(0);              out.afterClear  = SEL[selFor('monthly')].value;
fresh(); updateMtgRowUI(0, ` + string(rb) + `); out.afterEmpty = SEL[selFor('monthly')].value;
// A3: a SOLVED response must not overwrite a hardened cell.
fresh(); updateMtgRowUI(0, ` + string(solvedJSON) + `); out.afterPaint = SEL[selFor('monthly')].value;
// A2: a cell the CLIENT holds as input must survive a wire status of 'empty'.
// (Reachable when the client's view and the engine's disagree — a cell typed
// but not posted. Reachability not independently established; the guard's
// contract is "never erase what the user typed" and that is what is pinned.)
fresh(); SEL[selFor('tax')].value='$300.00'; mtgStatus[0]['tax']='input';
updateMtgRowUI(0, ` + string(rb) + `);      out.typedTax = SEL[selFor('tax')].value;
// A1: a hardened cell whose STATUS was reset by a What-If paint. Both
// placeWhatIfRow and the 2-D expansion reset mtgStatus[rowIdx] to an empty
// object and do NOT delete dataset.hardened (unlike clearMortgageRow, which
// deletes both), so a hardened cell really can reach clearMtgOutputs with a
// status that is not input. The hardened guard is its only protection there.
fresh(); mtgStatus[0] = {};   // <- the What-If status reset
clearMtgOutputs(0);           out.afterWhatIfReset = SEL[selFor('monthly')].value;
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var got struct{ AfterClear, AfterEmpty, AfterPaint, TypedTax, AfterWhatIfReset string }
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if got.AfterClear != "$9,999.99" {
		t.Errorf("clearMtgOutputs ERASED a hardened cell: %q", got.AfterClear)
	}
	if got.AfterEmpty != "$9,999.99" {
		t.Errorf("updateMtgRowUI's empty-branch ERASED a hardened cell: %q", got.AfterEmpty)
	}
	// A3 — the one the paint path actually depends on.
	if got.AfterPaint != "$9,999.99" {
		t.Errorf("updateMtgRowUI OVERWROTE a hardened cell with the engine's echo: "+
			"%q (hardened fields must be invariant under recalculation)", got.AfterPaint)
	}
	// A2 — never erase what the user typed.
	if got.TypedTax != "$300.00" {
		t.Errorf("updateMtgRowUI's empty-branch ERASED a cell the client holds as "+
			"a typed INPUT: %q", got.TypedTax)
	}
	// A1 — the case that makes clearMtgOutputs' hardened guard load-bearing.
	if got.AfterWhatIfReset != "$9,999.99" {
		t.Errorf("clearMtgOutputs ERASED a hardened cell whose status had been "+
			"reset by a What-If paint: %q. placeWhatIfRow and the 2-D expansion "+
			"both do `mtgStatus[rowIdx] = {}` WITHOUT deleting dataset.hardened, "+
			"so the 'input' guard does not cover this and the hardened guard is "+
			"the only protection.", got.AfterWhatIfReset)
	}
}

// 11/9. THE STATUS->CELL MAPPING MUST BE THE IDENTITY, AND inOutName MUST NOT
// COLLAPSE INPUT ONTO OUTPUT. Both were unpinned: a wire labelling every cell
// "output", or reporting Monthly's status under the `tax` key, passed all nine.
func TestR54MtgStatusMappingIsFaithful(t *testing.T) {
	// A row where the eleven cells take DIFFERENT statuses, so a mapping that
	// crosses two keys, or a renderer that collapses input onto output, shows.
	resp := callMortgage(t, `{"price":250000,"pctDown":0.2,"years":30,"rate":0.0725,"tax":300}`)
	if resp.Error != "" {
		t.Fatalf("unexpected refusal: %s", resp.Error)
	}
	want := map[string]string{
		"price":         "input",
		"pctDown":       "input",
		"years":         "input",
		"rate":          "input",
		"tax":           "input", // must NOT read back as monthly's status
		"monthly":       "output",
		"cash":          "output",
		"financed":      "output",
		"points":        "empty",
		"balloonYears":  "empty",
		"balloonAmount": "empty",
	}
	for k, w := range want {
		if got := resp.Statuses[k]; got != w {
			t.Errorf("statuses[%q] = %q, want %q", k, got, w)
		}
	}
	// An all-"output" wire must be impossible: the three families are distinct.
	seen := map[string]bool{}
	for _, v := range resp.Statuses {
		seen[v] = true
	}
	if len(seen) < 3 {
		t.Errorf("statuses collapsed to %d distinct values (%v) — inOutName is "+
			"not distinguishing input/output/empty", len(seen), seen)
	}
}

// 12/9. 🚨 R67 — THE WHAT-IF BASE MUST NOT REFUSE ROWS THE PRISTINE TREE
// ANSWERED. Adding Tax/Balloon to the SHARED mtgLineFromInput regressed
// /api/mortgage/whatif: 60 of 400 random bases began refusing, and both
// shapes are UI-reachable (runWhatIf posts getMtgRowData, which emits both
// balloon fields — and explainMtgError actively TELLS the user to fill
// Balloon Yrs). Found by round 54's first adversarial pass.
func TestR54WhatIfBaseIgnoresTheBalloon(t *testing.T) {
	for _, tc := range []struct{ name, base string }{
		{"balloonYears makes the balloon the solved output",
			`{"price":458883,"pctDown":0.28,"years":26,"rate":0.0367,"points":0.0003,"balloonYears":21}`},
		{"balloonAmount without balloonYears is a Calc error",
			`{"price":188078,"pctDown":0.19,"years":35,"rate":0.0845,"balloonAmount":142025}`},
		{"both balloon fields",
			`{"price":300000,"pctDown":0.2,"years":30,"rate":0.07,"balloonYears":10,"balloonAmount":150000}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"base":%s,"vary":"rate","increment":0.005,"count":4}`, tc.base)
			req := httptest.NewRequest(http.MethodPost, "/api/mortgage/whatif",
				bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			HandleMortgageWhatIf(w, req)
			var resp MortgageWhatIfResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Error != "" {
				t.Errorf("R67 regression: What-If refused a base the pristine tree "+
					"answered: %s", resp.Error)
			}
			if len(resp.Rows) != 4 {
				t.Errorf("What-If returned %d rows, want 4", len(resp.Rows))
			}
		})
	}
	// Tax MUST still reach the base — it is meaningful there and dropping it
	// would be finding 3's defect in the other endpoint.
	body := `{"base":{"price":300000,"pctDown":0.2,"years":30,"rate":0.07,"tax":300},"vary":"rate","increment":0.005,"count":3}`
	req := httptest.NewRequest(http.MethodPost, "/api/mortgage/whatif", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandleMortgageWhatIf(w, req)
	var resp MortgageWhatIfResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "" || len(resp.Rows) == 0 {
		t.Errorf("What-If with Tax refused: %q", resp.Error)
	}
}

// 13/9. 🚨 A TRANSPORT FAILURE MUST NOT WIPE THE GRID. apiPost synthesizes an
// error object when the request never reached the server; the ENGINE has not
// refused anything. Before this guard, one dropped request cleared every
// computed cell — silently, in autoSilent mode. Found by the same pass.
func TestR54TransportErrorDoesNotClearTheGrid(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	html := mustReadIndexHTML(t)

	// 🚨 THIS GUARD MUST EXECUTE, NOT SCAN.
	// Its first version asserted three literal substrings existed in
	// index.html. Round 54's SECOND adversarial pass killed it in one move:
	// inserting an UNCONDITIONAL `clearMtgOutputs(row);` immediately ABOVE
	// the guarded line leaves every scanned string intact, so the test
	// PASSED while the real page wiped the grid on a dropped request. A
	// source-scanning guard cannot pin behaviour (R50). This one runs the
	// SHIPPED calcMortgageRow against a stubbed apiPost.
	harness := `
'use strict';
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtRate") + `
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "clearMtgOutputs") + `
` + extractJS(t, html, "updateMtgRowUI") + `
` + extractJS(t, html, "calcMortgageRow") + `
var MTG_FIELDS = [{key:'price'},{key:'points'},{key:'pctDown'},{key:'cash'},{key:'financed'},{key:'years'},{key:'rate'},{key:'tax'},{key:'monthly'},{key:'balloonYears'},{key:'balloonAmount'}];
var MTG_ALL = MTG_FIELDS.map(function(f){return f.key;}).concat(['apr']);
var SEL = {}, EL = {};
function mkCell(){ var cls=[]; return {value:'',dataset:{},classList:{add:function(c){if(cls.indexOf(c)<0)cls.push(c);},remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);},contains:function(c){return cls.indexOf(c)>=0;}}}; }
function selFor(f){ return '#mtg-body input[data-row="0"][data-field="'+f+'"]'; }
var document = {
  querySelector: function(s){ return (s in SEL)?SEL[s]:null; },
  getElementById: function(id){ if(!EL[id]) EL[id]={textContent:'',innerHTML:''}; return EL[id]; }
};
var mtgStatus=[{}], mtgSelectedRow=0, autoSilent=false, calcGeneration=0;
function clearFieldErrors(){}
function setAutoCalcHint(){}
function markMtgErrorRow(){}
function explainMtgError(r,m,b){ return m; }
function renderAdvisoryHTML(){ return ''; }
function getMtgRowData(){ return {price:250000, pctDown:0.2, years:30, rate:0.0725}; }

// THE STUB UNDER TEST: exactly what the SHIPPED apiPost returns when fetch
// throws — verified below by asserting apiPost's own source produces it.
var MODE = 'transport';
async function apiPost(){
  if (MODE === 'transport')
    return { transport: true, error: 'Could not reach the calculator service. Check that the server is still running, then try again.' };
  return { error: 'Price and Monthly Total are both filled in, so there is nothing left to solve.' };
}

function seed(){
  SEL={}; MTG_ALL.forEach(function(f){ SEL[selFor(f)]=mkCell(); });
  mtgStatus=[{}];
  SEL[selFor('monthly')].value='$1,364.35'; mtgStatus[0]['monthly']='output';
  SEL[selFor('apr')].value='7.2500%';
  SEL[selFor('price')].value='$250,000.00'; mtgStatus[0]['price']='input';
}
(async function(){
  var out={};
  MODE='transport'; seed(); await calcMortgageRow();
  out.transportMonthly = SEL[selFor('monthly')].value;
  out.transportPrice   = SEL[selFor('price')].value;
  // POSITIVE CONTROL: a real ENGINE refusal MUST still clear. Without this
  // the test would pass on a build that never clears anything at all.
  MODE='engine'; seed(); await calcMortgageRow();
  out.engineMonthly = SEL[selFor('monthly')].value;
  console.log(JSON.stringify(out));
})();
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var got struct{ TransportMonthly, TransportPrice, EngineMonthly string }
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if got.TransportMonthly != "$1,364.35" {
		t.Errorf("a TRANSPORT failure wiped a computed cell: monthly = %q, want "+
			"$1,364.35. The engine never refused anything — the request did not "+
			"arrive.", got.TransportMonthly)
	}
	if got.TransportPrice != "$250,000.00" {
		t.Errorf("a TRANSPORT failure wiped a typed input: price = %q", got.TransportPrice)
	}
	// POSITIVE CONTROL (R49/R51): the branch must be TAKEN for a real refusal,
	// or the assertion above is satisfied by a build that never clears at all.
	if got.EngineMonthly != "" {
		t.Errorf("positive control FAILED: an ENGINE refusal did NOT clear the "+
			"stale output (monthly = %q). The transport assertion above is "+
			"therefore vacuous.", got.EngineMonthly)
	}
}

// 14/9. 🚨 DRIVE THE *REAL* apiPost, ON BOTH OF ITS SYNTHESIZED-ERROR PATHS.
//
// The test above runs the shipped calcMortgageRow against a STUBBED apiPost,
// so it proves the CONSUMER honours the mark but says nothing about the
// PRODUCER. Its residual check was `strings.Contains(apiPost, "transport:
// true")` — satisfied by ONE occurrence, while apiPost has TWO paths:
// `fetch` throwing (no connection) and `resp.json()` throwing (a 502 or an
// HTML error page from a proxy — the MORE likely failure). Round 54's THIRD
// adversarial pass removed the mark from the json() path alone and the whole
// suite stayed green. This executes both.
func TestR54ApiPostMarksBothTransportPaths(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	html := mustReadIndexHTML(t)
	harness := `
'use strict';
` + extractJS(t, html, "apiPost") + `
function _setApiBusy(){}
var out = {};
(async function(){
  // PATH 1: fetch itself throws — the server is unreachable.
  global.fetch = async function(){ throw new Error('ECONNREFUSED'); };
  out.fetchThrow = await apiPost('/api/mortgage/calc', {});
  // PATH 2: a response arrives but its body is not JSON (502, proxy HTML).
  global.fetch = async function(){
    return { status: 502, json: async function(){ throw new SyntaxError('Unexpected token <'); } };
  };
  out.jsonThrow = await apiPost('/api/mortgage/calc', {});
  // NEGATIVE CONTROL: a real server-sent JSON error is NOT a transport failure
  // and must NOT be marked, or every engine refusal would stop clearing.
  global.fetch = async function(){
    return { status: 400, json: async function(){ return { error: 'Amount borrowed cannot exceed price' }; } };
  };
  out.engineError = await apiPost('/api/mortgage/calc', {});
  console.log(JSON.stringify(out));
})();
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var got struct {
		FetchThrow  map[string]any `json:"fetchThrow"`
		JSONThrow   map[string]any `json:"jsonThrow"`
		EngineError map[string]any `json:"engineError"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if got.FetchThrow["transport"] != true {
		t.Errorf("apiPost did not mark the fetch-throw path as transport: %v", got.FetchThrow)
	}
	if got.JSONThrow["transport"] != true {
		t.Errorf("apiPost did not mark the resp.json()-throw path as transport: %v — "+
			"a 502 or a proxy error page then wipes the user's grid", got.JSONThrow)
	}
	// NEGATIVE CONTROL: an engine refusal must stay unmarked.
	if _, marked := got.EngineError["transport"]; marked {
		t.Errorf("apiPost marked a SERVER-SENT error as transport: %v — engine "+
			"refusals would stop clearing stale output", got.EngineError)
	}
	if got.EngineError["error"] == nil {
		t.Errorf("negative control broken: server error not passed through: %v", got.EngineError)
	}
}

// 15/9. 🚨 THE OTHER TWO CLEARING SITES MUST HONOUR THE MARK TOO.
// calcAllMortgageRows and calcAmortization each carry their own copy of the
// guard, and pass 3 showed BOTH were pinned by nothing: an unconditional
// clear in either left the whole suite green. calcAllMortgageRows is executed
// here; the amortization site is checked structurally, and that limit is
// stated rather than hidden.
func TestR54AllRowsAndAmortizationHonourTransport(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	html := mustReadIndexHTML(t)

	harness := `
'use strict';
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtRate") + `
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "clearMtgOutputs") + `
` + extractJS(t, html, "updateMtgRowUI") + `
` + extractJS(t, html, "calcAllMortgageRows") + `
var MTG_FIELDS = [{key:'price'},{key:'points'},{key:'pctDown'},{key:'cash'},{key:'financed'},{key:'years'},{key:'rate'},{key:'tax'},{key:'monthly'},{key:'balloonYears'},{key:'balloonAmount'}];
var MTG_ALL = MTG_FIELDS.map(function(f){return f.key;}).concat(['apr']);
var MTG_ROWS = 2;
var SEL = {}, EL = {};
function mkCell(){ var cls=[]; return {value:'',dataset:{},classList:{add:function(c){if(cls.indexOf(c)<0)cls.push(c);},remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);},contains:function(c){return cls.indexOf(c)>=0;}}}; }
function selFor(r,f){ return '#mtg-body input[data-row="'+r+'"][data-field="'+f+'"]'; }
var document = {
  querySelector: function(s){ return (s in SEL)?SEL[s]:null; },
  getElementById: function(id){ if(!EL[id]) EL[id]={textContent:'',innerHTML:''}; return EL[id]; }
};
var mtgStatus=[{},{}];
function clearFieldErrors(){}
function markMtgErrorRow(){}
function renderAdvisoryHTML(){ return ''; }
function getMtgRowData(){ return {price:250000, pctDown:0.2, years:30, rate:0.0725}; }
var MODE='transport';
async function apiPost(){
  if (MODE==='transport') return { transport:true, error:'Could not reach the calculator service.' };
  return { error:'Price and Monthly Total are both filled in.' };
}
function seed(){ SEL={}; mtgStatus=[{},{}];
  for (var r=0;r<2;r++){ MTG_ALL.forEach(function(f){ SEL[selFor(r,f)]=mkCell(); });
    SEL[selFor(r,'monthly')].value='$1,364.35'; mtgStatus[r]['monthly']='output'; } }
(async function(){
  var out={};
  MODE='transport'; seed(); await calcAllMortgageRows();
  out.transportRow0 = SEL[selFor(0,'monthly')].value;
  out.transportRow1 = SEL[selFor(1,'monthly')].value;
  MODE='engine'; seed(); await calcAllMortgageRows();
  out.engineRow0 = SEL[selFor(0,'monthly')].value;
  console.log(JSON.stringify(out));
})();
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var got struct{ TransportRow0, TransportRow1, EngineRow0 string }
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if got.TransportRow0 != "$1,364.35" || got.TransportRow1 != "$1,364.35" {
		t.Errorf("calcAllMortgageRows wiped EVERY row on one dropped request: "+
			"row0=%q row1=%q", got.TransportRow0, got.TransportRow1)
	}
	// POSITIVE CONTROL — an engine refusal must still clear, or the above is vacuous.
	if got.EngineRow0 != "" {
		t.Errorf("positive control FAILED: an ENGINE refusal did not clear row 0 "+
			"(%q), so the transport assertion is vacuous", got.EngineRow0)
	}

	// ⚠️ THE AMORTIZATION SITE IS NOT EXECUTED HERE. calcAmortization pulls in
	// the whole schedule-render path; driving it needs the browser harness the
	// project does not own for this screen. This is a STRUCTURAL check and it
	// is therefore WEAKER than the two above — stated, not hidden. It would not
	// catch an unconditional clear inserted ABOVE this line, which is exactly
	// the move that killed the previous version of the mortgage guard.
	if !strings.Contains(html, "if (!data.transport) clearAmzScheduleOutput();") {
		t.Errorf("the amortization screen's transport guard is gone: one dropped " +
			"request destroys the whole schedule while the summary line goes on " +
			"asserting a Total Paid with nothing behind it")
	}
}

// 16/9. 🚨 A WHAT-IF PAINT MUST DROP THE FROZEN-CELL FLAG.
//
// placeWhatIfRow overwrites a row's VALUES unconditionally, so a surviving
// dataset.hardened is a flag with no frozen value behind it. It breaks the
// "hardened fields are invariant under recalculation" contract in one
// direction (the user's freeze is silently overwritten) and then, in the
// other, protects a What-If number from clearMtgOutputs and updateMtgRowUI
// permanently — finding 6's disease, resurrected on a different row.
// clearMortgageRow always did this; the two What-If paths did not.
//
// Found by round 54's THIRD adversarial pass, which also showed that the
// round's earlier A1 guard had pinned an UNREACHABLE state instead of this
// real one. ⚠️ With this fix in place that A1 state is now unreachable BY
// CONSTRUCTION, so TestR54MtgHardenedCellsSurviveClearing's What-If case is
// defence in depth, not a reachability claim. Both are kept deliberately.
func TestR54WhatIfPaintDropsHardenedFlag(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	html := mustReadIndexHTML(t)
	harness := `
'use strict';
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtRate") + `
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "clearMtgOutputs") + `
` + extractJS(t, html, "placeWhatIfRow") + `
var MTG_FIELDS = [{key:'price'},{key:'points'},{key:'pctDown'},{key:'cash'},{key:'financed'},{key:'years'},{key:'rate'},{key:'tax'},{key:'monthly'},{key:'balloonYears'},{key:'balloonAmount'}];
var MTG_ALL = MTG_FIELDS.map(function(f){return f.key;}).concat(['apr']);
var SEL = {};
function mkCell(){ var cls=[]; return {value:'',dataset:{},classList:{add:function(c){if(cls.indexOf(c)<0)cls.push(c);},remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);},contains:function(c){return cls.indexOf(c)>=0;},toggle:function(c,on){if(on)this.add(c);else this.remove(c);}}}; }
function selFor(r,f){ return '#mtg-body input[data-row="'+r+'"][data-field="'+f+'"]'; }
var document = { querySelector:function(s){ return (s in SEL)?SEL[s]:null; } };
var mtgStatus=[{},{}];
for (var r=0;r<2;r++) MTG_ALL.forEach(function(f){ SEL[selFor(r,f)]=mkCell(); });
// Row 0 is the SOURCE. Row 1 has a hardened Monthly the user froze earlier.
mtgStatus[0] = {price:'input', pctDown:'input', years:'input', rate:'input'};
SEL[selFor(1,'monthly')].value='$9,999.99';
SEL[selFor(1,'monthly')].dataset.hardened='1';
mtgStatus[1]['monthly']='input';
placeWhatIfRow(1, {price:250000, points:0, pctDown:0.2, cash:50000, financed:200000, years:30, rate:0.0725, monthly:1418.87}, 0, 'rate');
var out = {
  hardenedAfterPaint: SEL[selFor(1,'monthly')].dataset.hardened || null,
  valueAfterPaint: SEL[selFor(1,'monthly')].value
};
// And the orphaned flag must not then make the cell immortal.
clearMtgOutputs(1);
out.valueAfterClear = SEL[selFor(1,'monthly')].value;
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var got struct {
		HardenedAfterPaint *string `json:"hardenedAfterPaint"`
		ValueAfterPaint    string  `json:"valueAfterPaint"`
		ValueAfterClear    string  `json:"valueAfterClear"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if got.HardenedAfterPaint != nil {
		t.Errorf("placeWhatIfRow left dataset.hardened = %q on a row whose value it "+
			"had already overwritten (now %q) — an orphaned freeze flag",
			*got.HardenedAfterPaint, got.ValueAfterPaint)
	}
	// POSITIVE CONTROL that the paint actually happened, so the assertion above
	// is not satisfied by a no-op.
	if got.ValueAfterPaint == "$9,999.99" || got.ValueAfterPaint == "" {
		t.Fatalf("positive control FAILED: placeWhatIfRow did not paint the row "+
			"(monthly = %q), so the hardened assertion is vacuous", got.ValueAfterPaint)
	}
	if got.ValueAfterClear != "" {
		t.Errorf("an orphaned hardened flag made a What-If number immortal: "+
			"clearMtgOutputs left %q", got.ValueAfterClear)
	}
}
