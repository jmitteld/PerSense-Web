package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestSettingsChangeRecalc guards the behavior where changing a computational
// setting reflows the active worksheet on modal close — matching DOS, where a
// settings change recomputes the on-screen figures. The recompute is gated: it
// fires only when (a) a setting actually changed, (b) the current screen already
// shows a result, and (c) the screen is Amortization or Present Value (Mortgage
// is always 360-day and unaffected). Otherwise it falls back to scheduleAutoCalc
// (which only acts when Auto-calculate is on), so an empty/partial worksheet is
// never forced to calc and error.
//
// Executes the SHIPPED modal block (openSettings/closeSettings + helpers)
// extracted from index.html against a fake DOM under Node, with the calc
// functions and scheduleAutoCalc stubbed as call counters. Skips without node.
func TestSettingsChangeRecalc(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend JS test")
	}
	htmlBytes, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(htmlBytes)

	defaults := regexp.MustCompile(`(?s)const SETTINGS_DEFAULTS = \{.*?\};`).FindString(html)
	if defaults == "" {
		t.Fatal("SETTINGS_DEFAULTS definition not found in index.html")
	}
	// The whole modal block lives between two banner comments.
	blockRe := regexp.MustCompile(`(?s)// ===== Computational Settings modal.*?=====\n(.*?)\n// ===== end Computational Settings modal =====`)
	m := blockRe.FindStringSubmatch(html)
	if m == nil {
		t.Fatal("Computational Settings modal block not found in index.html")
	}
	shipped := m[1]
	for _, want := range []string{"function openSettings", "function closeSettings",
		"function settingsChangedSinceOpen", "function recalcActiveScreen"} {
		if !strings.Contains(shipped, want) {
			t.Fatalf("extracted modal block did not include %s", want)
		}
	}

	harness := `
'use strict';
` + defaults + `

let currentScreen = 'amortization';
let amzCalls = 0, pvCalls = 0, schedCalls = 0, __stale = false;
function updateSettingsBadge() {}
function saveStateSoon() {}
function calcAmortization() { amzCalls++; return Promise.resolve(true); }
function calcPV() { pvCalls++; return Promise.resolve(true); }
function scheduleAutoCalc() { schedCalls++; }
function hasStaleOutput() { return __stale; }

const store = {};
const noopClass = { add() {}, remove() {} };
const document = {
  getElementById(id) {
    if (id === 'modal-settings') return { classList: noopClass };
    if (!(id in store)) store[id] = { value: (id in SETTINGS_DEFAULTS) ? SETTINGS_DEFAULTS[id] : '', classList: noopClass };
    return store[id];
  }
};

` + shipped + `

function scenario(fn) { amzCalls = 0; pvCalls = 0; schedCalls = 0; fn(); return { amz: amzCalls, pv: pvCalls, sched: schedCalls }; }
const setv = (id, v) => { document.getElementById(id).value = v; };
const out = {};

// A) amortization + result + changed -> forced calcAmortization, no fallback
currentScreen = 'amortization'; __stale = true; setv('set-basis', '360');
out.A = scenario(() => { openSettings(); setv('set-basis', '365'); closeSettings(); });

// B) amortization + result + NO change -> fallback only
out.B = scenario(() => { openSettings(); closeSettings(); });

// C) changed but NO result -> fallback only (never force-calc an empty sheet)
__stale = false;
out.C = scenario(() => { openSettings(); setv('set-prepaid', 'no'); closeSettings(); });

// D) present value + result + changed -> calcPV
currentScreen = 'presentvalue'; __stale = true;
out.D = scenario(() => { openSettings(); setv('set-basis', '360'); closeSettings(); });

// E) mortgage + result + changed -> NOT force-recalced (settings don't affect it)
currentScreen = 'mortgage'; __stale = true;
out.E = scenario(() => { openSettings(); setv('set-basis', '365'); closeSettings(); });

console.log(JSON.stringify(out));
`

	cmd := exec.Command(nodePath, "-")
	cmd.Stdin = strings.NewReader(harness)
	nodeOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, nodeOut)
	}

	type calls struct {
		Amz   int `json:"amz"`
		Pv    int `json:"pv"`
		Sched int `json:"sched"`
	}
	var res map[string]calls
	if err := json.Unmarshal(nodeOut, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, nodeOut)
	}

	want := func(name string, got calls, amz, pv, sched int) {
		if got.Amz != amz || got.Pv != pv || got.Sched != sched {
			t.Errorf("%s: got {amz:%d pv:%d sched:%d}, want {amz:%d pv:%d sched:%d}",
				name, got.Amz, got.Pv, got.Sched, amz, pv, sched)
		}
	}
	// A: exactly one amortization recompute, no auto-calc fallback.
	want("A changed+result (amz)", res["A"], 1, 0, 0)
	// B: nothing changed -> fallback to scheduleAutoCalc.
	want("B no-change", res["B"], 0, 0, 1)
	// C: changed but no visible result -> fallback, no forced calc.
	want("C no-result", res["C"], 0, 0, 1)
	// D: present value recompute.
	want("D changed+result (pv)", res["D"], 0, 1, 0)
	// E: mortgage is unaffected -> fallback only, no forced calc.
	want("E mortgage", res["E"], 0, 0, 1)
}
