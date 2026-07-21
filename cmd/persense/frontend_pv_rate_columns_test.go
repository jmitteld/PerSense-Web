package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestPVRateColumns guards the DOS three-column PV rate display (True Rate /
// Loan Rate / Yield): editing any one column fills the other two, and the
// engine-facing canonical (hidden pv-rate + pv-rateType) holds the RAW value and
// type of whichever column was last edited — so a True/Yield entry reaches the
// engine exactly, with no lossy round-trip through the Loan form.
//
// Executes the SHIPPED conversion + sync code extracted from index.html
// (pvRateToTrue/pvTrueToType/onPVRateEdit/pvFillRateTrio/setPVRate) against a
// fake DOM under Node. Skips without node.
func TestPVRateColumns(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend JS test")
	}
	html, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	src := string(html)

	parseRate := regexp.MustCompile(`(?s)function parseRate\(s\) \{.*?\n\}`).FindString(src)
	if parseRate == "" {
		t.Fatal("parseRate not found")
	}
	// pvRateToTrue through the end of setPVRate (just before getPVInput).
	block := regexp.MustCompile(`(?s)(function pvRateToTrue\(pct, type\) \{.*\n\})\n\nfunction getPVInput`).FindStringSubmatch(src)
	if block == nil {
		t.Fatal("pvRateToTrue..setPVRate block not found")
	}
	shipped := block[1]
	for _, want := range []string{"function pvTrueToType", "const PV_RATE_FIELDS",
		"function onPVRateEdit", "function setPVRate"} {
		if !strings.Contains(shipped, want) {
			t.Fatalf("extracted block missing %s", want)
		}
	}

	harness := `
'use strict';
` + parseRate + `
` + shipped + `
function saveStateSoon() {}
const store = {};
function mkEl(id, v) { return { id: id, value: v || '', classList: { _g: false, add() { this._g = true; }, remove() { this._g = false; }, contains() { return this._g; } } }; }
const document = { getElementById(id) { if (!(id in store)) store[id] = mkEl(id, id === 'pv-rateType' ? 'loan' : ''); return store[id]; } };
function resetDom() { for (const k of Object.keys(store)) delete store[k]; ['pv-rate-true','pv-rate-loan','pv-rate-yield','pv-rate','pv-rateType'].forEach(id => document.getElementById(id)); }
function edit(type, v) { const map = { true:'pv-rate-true', loan:'pv-rate-loan', yield:'pv-rate-yield' }; const e = document.getElementById(map[type]); e.value = v; onPVRateEdit(e); }
function snap() { const g = id => document.getElementById(id); return { t:g('pv-rate-true').value, l:g('pv-rate-loan').value, y:g('pv-rate-yield').value, cr:g('pv-rate').value, ct:g('pv-rateType').value, gt:g('pv-rate-true').classList.contains() }; }

const out = {};
resetDom(); edit('true', '5.9850'); out.editTrue = snap();
resetDom(); edit('loan', '6');      out.editLoan = snap();
resetDom(); edit('yield', '10');    out.editYield = snap();
resetDom(); edit('loan', '6'); edit('true', ''); out.cleared = snap();
resetDom(); setPVRate('8'); out.setpv = snap();
console.log(JSON.stringify(out));
`

	cmd := exec.Command(nodePath, "-")
	cmd.Stdin = strings.NewReader(harness)
	nodeOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, nodeOut)
	}
	// JSON keys are lowercase field names from JS.
	var res map[string]struct {
		T  string `json:"t"`
		L  string `json:"l"`
		Y  string `json:"y"`
		Cr string `json:"cr"`
		Ct string `json:"ct"`
		Gt bool   `json:"gt"`
	}
	if err := json.Unmarshal(nodeOut, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, nodeOut)
	}
	eq := func(name, got, want string) {
		if got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
	}

	// Editing True: canonical is the EXACT typed True value+type (precision), and
	// Loan/Yield columns fill with the equivalents.
	et := res["editTrue"]
	eq("editTrue canon rate", et.Cr, "5.9850")
	eq("editTrue canon type", et.Ct, "true")
	eq("editTrue loan col", et.L, "5.9999")
	eq("editTrue yield col", et.Y, "6.1677")

	// Editing Loan 6% -> the documented equivalents; canonical loan/6.
	el := res["editLoan"]
	eq("editLoan canon rate", el.Cr, "6")
	eq("editLoan canon type", el.Ct, "loan")
	eq("editLoan true col", el.T, "5.9850")
	eq("editLoan yield col", el.Y, "6.1678")

	// Editing Yield 10% -> true/loan equivalents; canonical yield/10.
	ey := res["editYield"]
	eq("editYield canon rate", ey.Cr, "10")
	eq("editYield canon type", ey.Ct, "yield")
	eq("editYield true col", ey.T, "9.5310")
	eq("editYield loan col", ey.L, "9.5690")

	// Clearing one column clears all three + the canonical -> "solve for rate".
	c := res["cleared"]
	if c.T != "" || c.L != "" || c.Y != "" || c.Cr != "" {
		t.Errorf("cleared: expected all-blank, got t=%q l=%q y=%q cr=%q", c.T, c.L, c.Y, c.Cr)
	}

	// (The solved-rate echo across the columns is covered by the api package's
	// TestFrontendPVSolvedEchoSweep, which drives the real calcPV.)

	// setPVRate loads a loan-quoted example into the trio.
	sp := res["setpv"]
	eq("setpv canon rate", sp.Cr, "8")
	eq("setpv canon type", sp.Ct, "loan")
	if sp.T == "" || sp.Y == "" {
		t.Errorf("setpv: True/Yield columns should be filled, got t=%q y=%q", sp.T, sp.Y)
	}
}
