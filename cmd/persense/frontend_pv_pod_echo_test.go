package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// ROUND-42 — the CLIENT half of the PV advanced-option stacking fixes.
//
// Round 39's postmortem was that its display-layer fixes shipped with no
// test at the layer that broke (items A1-A5), so a fix verified at the
// producer was broken at the consumer (R42). These are the consumer-side
// guards for §75 (paint the solved Payment on Death), §77 (a typed zero is
// not "absent") and §79 (paint by DOM row, not by request index).

// TestR42PVActuarialConfigZeroPOD runs the SHIPPED getActuarialConfig from
// index.html against a fake DOM.
//
// §77: omitting `pod` from the request means PODUnknown — "solve for the
// death benefit from the target Sum Value" (handlers.go
// buildActuarialConfig → presentvalue.solveUnknownPOD, DOS
// ComputeUnknownPOD). The shipped guard was `pod > 0`, so a TYPED zero —
// "there is no death benefit" — was transmitted as absent and silently
// became a request to solve for one. Absent is not zero.
//
// Also pins the full-precision stash: once §75 paints a solved POD into
// the cell, re-reading the display-rounded text on the next submit would
// re-send a different number than the engine solved.
func TestR42PVActuarialConfigZeroPOD(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend JS test")
	}
	src := readIndexHTML(t)

	block := regexp.MustCompile(`(?s)function getActuarialConfig\(\) \{.*?\n\}`).FindString(src)
	if block == "" {
		t.Fatal("getActuarialConfig not found in index.html")
	}
	// The extracted block must still be the one carrying the §77 fix; a
	// silent revert to `pod > 0` would otherwise pass by not being tested.
	if !strings.Contains(block, "pod != null && !isNaN(pod)") {
		t.Fatalf("getActuarialConfig no longer contains the §77 absent-vs-zero "+
			"guard; extracted:\n%s", block)
	}
	if strings.Contains(block, "&& pod > 0") {
		t.Fatal("§77 REGRESSED: getActuarialConfig is gating `cfg.pod` on `pod > 0` again")
	}
	parseMoney := regexp.MustCompile(`(?s)function parseMoney\(s\) \{.*?\n\}`).FindString(src)
	if parseMoney == "" {
		t.Fatal("parseMoney not found")
	}
	// Round 63 (decision 3a.20): getActuarialConfig now opens with the
	// client-side beta gate, so the SHIPPED betaActuarialEnabled is extracted
	// alongside it rather than stubbed — R92 (an instrument that replaces a
	// function replaces its guards): a stub here would silently stop covering
	// the gate the config now depends on.
	betaGate := regexp.MustCompile(`(?s)function betaActuarialEnabled\(\) \{.*?\n\}`).FindString(src)
	if betaGate == "" {
		t.Fatal("betaActuarialEnabled not found in index.html")
	}
	if !strings.Contains(betaGate, "set-betaActuarial") {
		t.Fatalf("betaActuarialEnabled no longer reads the set-betaActuarial "+
			"checkbox; extracted:\n%s", betaGate)
	}

	harness := `
'use strict';
` + parseMoney + `
` + betaGate + `
` + block + `
// Stubs for the parts of the DOM/helpers getActuarialConfig leans on. The
// life table and dates are held fixed; only the POD cell varies.
function getActuarialTable(n) { return n === 1 ? [[0, 0.001], [120, 1]] : null; }
function parseDate(s) { return (s && s.trim()) ? s.trim() : null; }
const cells = {};
function mkCell(v, green, raw) {
  return {
    value: v,
    dataset: raw == null ? {} : { pvRaw: String(raw) },
    classList: { contains(c) { return c === 'cell-output' && !!green; } },
  };
}
const document = { getElementById(id) { return cells[id] || mkCell(''); } };

function run(podCell) {
  // The feature is opt-in and OFF by default; this test's subject is the POD
  // cell's absent-vs-zero contract, which only exists downstream of the gate,
  // so the harness turns the real gate ON through the real checkbox id.
  cells['set-betaActuarial'] = { checked: true };
  cells['actu-dob1'] = mkCell('10/10/1940');
  cells['actu-now'] = mkCell('01/01/2024');
  cells['actu-pod'] = podCell;
  const cfg = getActuarialConfig();
  return { hasPod: cfg != null && ('pod' in cfg), pod: cfg ? cfg.pod : null };
}

const out = {
  blank:        run(mkCell('')),
  spaces:       run(mkCell('   ')),
  typedZero:    run(mkCell('0')),
  typedZeroFmt: run(mkCell('$0.00')),
  typedValue:   run(mkCell('$20,000.00')),
  negative:     run(mkCell('-5000')),
  // A solved POD painted green with its full-precision stash: the next
  // submit must re-send the STASH, not the rounded display text.
  greenSolved:  run(mkCell('$52,870.31', true, 52870.313456)),
};
console.log(JSON.stringify(out));
`
	tmp, err := os.CreateTemp("", "r42podcfg-*.js")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(harness); err != nil {
		t.Fatalf("write: %v", err)
	}
	tmp.Close()

	raw, err := exec.Command(nodePath, tmp.Name()).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, raw)
	}
	var got struct {
		Blank, Spaces, TypedZero, TypedZeroFmt, TypedValue, Negative, GreenSolved struct {
			HasPod bool     `json:"hasPod"`
			Pod    *float64 `json:"pod"`
		}
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}

	// A blank cell is the ONLY thing that asks the engine to solve.
	if got.Blank.HasPod {
		t.Errorf("blank POD cell sent pod=%v — a blank cell must omit `pod` so "+
			"the engine solves for it", got.Blank.Pod)
	}
	if got.Spaces.HasPod {
		t.Errorf("whitespace-only POD cell sent pod=%v, want omitted", got.Spaces.Pod)
	}
	// §77: a typed zero must be transmitted as zero.
	for name, c := range map[string]struct {
		HasPod bool
		Pod    *float64
	}{
		"typed 0":     {got.TypedZero.HasPod, got.TypedZero.Pod},
		"typed $0.00": {got.TypedZeroFmt.HasPod, got.TypedZeroFmt.Pod},
	} {
		if !c.HasPod {
			t.Errorf("§77: %s omitted `pod` — the engine reads that as "+
				"PODUnknown and solves for a death benefit the user said was zero", name)
			continue
		}
		if *c.Pod != 0 {
			t.Errorf("§77: %s sent pod=%v, want 0", name, *c.Pod)
		}
	}
	if !got.TypedValue.HasPod || *got.TypedValue.Pod != 20000 {
		t.Errorf("typed $20,000.00 sent hasPod=%v pod=%v, want 20000",
			got.TypedValue.HasPod, got.TypedValue.Pod)
	}
	// A negative entry is also not absent: it must reach the engine (and be
	// judged there) rather than silently turning into a solve request.
	if !got.Negative.HasPod || *got.Negative.Pod != -5000 {
		t.Errorf("typed -5000 sent hasPod=%v pod=%v, want -5000",
			got.Negative.HasPod, got.Negative.Pod)
	}
	// Full precision survives a §75 paint → resubmit round trip.
	if !got.GreenSolved.HasPod || *got.GreenSolved.Pod != 52870.313456 {
		t.Errorf("a solved+painted POD re-sent as %v, want the full-precision "+
			"52870.313456 — resubmitting the rounded display value drifts the total",
			got.GreenSolved.Pod)
	}
}

// TestR42PVPaintPairing is a SOURCE-LAYOUT guard (R35: a documented trap is
// not a guard — turn it into an assertion) over the two calcPV changes that
// are not reachable without a full DOM.
//
// §79 — getPVInput `continue`s past an empty grid slot, so body.lumpSums /
// body.periodics are COMPACTED while the grid is not. The response paint
// loops used the REQUEST index as the `data-ls` / `data-per` selector, so
// with row 1 blank and row 2 filled, row 2's solved Date/Amount/Value were
// written into the blank row above it — and a painted cell is a hard input
// on the next submit, so the phantom row was then summed into the total.
//
// §75 — the solved Payment on Death must be painted back into `actu-pod`,
// as DOS does unconditionally after the solve (PRESVALU.pas:1269
// PlacePODOnScreen).
func TestR42PVPaintPairing(t *testing.T) {
	src := readIndexHTML(t)

	// §79 producer side: both blank-trackers must record the DOM row.
	for _, want := range []string{
		"pvLumpBlanks.push({ dom: i,",
		"pvPerBlanks.push({ dom: i,",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("§79 REGRESSED: %q not found — the blank-tracker no longer "+
				"records which GRID row each compacted request row came from", want)
		}
	}

	// §79 consumer side: the RESPONSE PAINT loops must select on the
	// recorded DOM index. Scoped to the paint loops on purpose —
	// getPVInput's READ loops legitimately use `i`, because there `i` IS
	// the grid row. (The first cut of this guard searched the whole file
	// and fired on those read loops. A guard that cannot tell the two
	// loops apart would have to be silenced, and a silenced guard is not
	// a guard — round 41's lesson about a gate that fails on a valid
	// configuration.)
	for _, loop := range []struct{ head, req, dom string }{
		{"(data.lumpSums || []).forEach((ls, i) => {",
			"`input[data-ls=\"${i}\"]", "`input[data-ls=\"${dom}\"]"},
		{"(data.periodics || []).forEach((pp, i) => {",
			"`input[data-per=\"${i}\"]", "`input[data-per=\"${dom}\"]"},
	} {
		start := strings.Index(src, loop.head)
		if start < 0 {
			t.Errorf("§79: paint loop %q not found — the guard can no longer "+
				"see the code it protects", loop.head)
			continue
		}
		end := strings.Index(src[start:], "\n  });")
		if end < 0 {
			t.Errorf("§79: could not delimit the paint loop %q", loop.head)
			continue
		}
		body := src[start : start+end]
		if strings.Contains(body, loop.req) {
			t.Errorf("§79 REGRESSED: the paint loop %q selects %s — that is the "+
				"REQUEST index, and the request array is compacted past empty "+
				"grid slots, so a solved cell lands in the wrong row",
				loop.head, loop.req)
		}
		if !strings.Contains(body, loop.dom) {
			t.Errorf("§79: paint loop %q does not select on %s", loop.head, loop.dom)
		}
	}

	// §75: the solved POD is read off the wire and painted. Assert the BODY,
	// not only the condition — the first cut checked the `if` header alone,
	// which a mutant that empties the block survives (round-42 audit).
	if !strings.Contains(src, "writeOut(podEl2, fmtDollars(data.pod), data.pod);") {
		t.Error("§75 REGRESSED: the solved `pod` is no longer written into the " +
			"Payment on Death cell. The `if` may still be there; the paint is not.")
	}
	// §78: the noise floor that keeps a pre-emptive (inert) POD solve from
	// stamping a spurious green $0.00 into the cell.
	if !strings.Contains(src, "Math.abs(data.pod) >= 0.005") {
		t.Error("§78 REGRESSED: the §75 paint no longer floors at half a cent, so " +
			"solveUnknownPOD's residual noise (~1e-11 on the pre-emptive path) " +
			"will be painted as $0.00 and re-sent as a hard input")
	}
	// §75/C4: the POD cell must be in the global un-green list, or a typed
	// value is shown but never transmitted (the stash keeps winning).
	if !strings.Contains(src, "t.id === 'actu-pod' ||") {
		t.Error("§75 REGRESSED: 'actu-pod' is not in the global input handler's " +
			"cell-output drop list. getActuarialConfig reads dataset.pvRaw while " +
			"the cell is green, so typing over a painted POD would be discarded.")
	}
	if !strings.Contains(src, "pvPodBlank && podEl2 && data.pod != null") {
		t.Error("§75 REGRESSED: calcPV no longer paints the solved `pod` into " +
			"the Payment on Death cell. The engine computes it (solveUnknownPOD), " +
			"the wire carries it, and DOS paints it (PRESVALU.pas:1269) — dropping " +
			"it leaves the one number the run was for invisible.")
	}
	if !strings.Contains(src, "pvPodBlank = (acfg.pod === undefined);") {
		t.Error("§75 REGRESSED: getPVInput no longer records whether the POD cell " +
			"was blank, so calcPV cannot tell a solved POD from the user's own entry")
	}
	// Match the reset IN CONTEXT, alongside the other per-request trackers.
	// A bare "pvPodBlank = false;" also matches the `let` declaration, which
	// made the first cut of this assertion vacuous — the mutant that deleted
	// the reset survived it.
	if !strings.Contains(src, "pvPerBlanks = [];\n  pvPodBlank = false;") {
		t.Error("§75: pvPodBlank is not reset next to pvLumpBlanks/pvPerBlanks at " +
			"the top of getPVInput — a stale true would paint a previous run's " +
			"solved POD over a value the user has since typed")
	}
}
