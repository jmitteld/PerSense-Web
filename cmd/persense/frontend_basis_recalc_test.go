package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestAmzSelectChangeSchedulesRecalc guards the Amortization-screen fix
// where changing the Basis (or Pmts/Yr) dropdown did not auto-recalculate.
//
// Root cause: the only thing that triggers an auto-calc is the global
// `focusout` handler (index.html, near scheduleAutoCalc). A native
// <select> keeps focus after the user picks an option, so focusout never
// fires on the change itself; the select's `change` listener called
// onAmzCellInput (which only blanks stale outputs and hides the summary)
// but never scheduleAutoCalc. The result: changing Basis cleared the
// schedule and waited for a manual Calculate click. The fix adds
// scheduleAutoCalc to the SELECT `change` path only — NOT to the text
// `input` path, so typing into a money/number field still recomputes on
// commit (focusout), not on every keystroke.
//
// This test executes the SHIPPED wiring extracted from index.html
// (AMZ_INPUT_CELLS + its forEach registration block) against a fake DOM
// under Node, then dispatches events and asserts:
//   - a SELECT 'change' (Basis, Pmts/Yr) schedules a recalc, and
//   - a text 'input' (e.g. Amt Borrowed) does NOT.
// Before the fix the first assertion fails (no scheduleAutoCalc call);
// after the fix it passes. Skips when node is not installed.
func TestAmzSelectChangeSchedulesRecalc(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend JS test")
	}
	htmlBytes, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(htmlBytes)

	// Pull the real array + the real listener-registration block out of
	// the page so the test exercises shipped code, not a copy.
	arrRe := regexp.MustCompile(`(?s)const AMZ_INPUT_CELLS = \[.*?\];`)
	arr := arrRe.FindString(html)
	if arr == "" {
		t.Fatal("AMZ_INPUT_CELLS definition not found in index.html")
	}
	wireRe := regexp.MustCompile(`(?s)AMZ_INPUT_CELLS\.forEach\(id => \{.*?\n  \}\);`)
	wire := wireRe.FindString(html)
	if wire == "" {
		t.Fatal("AMZ_INPUT_CELLS.forEach wiring block not found in index.html")
	}

	harness := `
'use strict';
let schedCalls = [];
let cellInputCalls = [];
function scheduleAutoCalc(el) { schedCalls.push(el && el.id); }
function onAmzCellInput(id) { cellInputCalls.push(id); }

` + arr + `

// Minimal DOM: Basis and Pmts/Yr are <select>, everything else <input>.
const SELECTS = new Set(['amz-perYr', 'amz-basis']);
const els = {};
function makeEl(id) {
  const handlers = {};
  return {
    id: id,
    tagName: SELECTS.has(id) ? 'SELECT' : 'INPUT',
    addEventListener(type, fn) { (handlers[type] = handlers[type] || []).push(fn); },
    dispatch(type) { (handlers[type] || []).forEach(fn => fn({ type: type, target: this })); },
    _handlers: handlers,
  };
}
const document = { getElementById(id) { return els[id] || (els[id] = makeEl(id)); } };

` + wire + `

function fire(id, type) {
  schedCalls = []; cellInputCalls = [];
  document.getElementById(id).dispatch(type);
  return { sched: schedCalls.slice(), cell: cellInputCalls.slice() };
}

console.log(JSON.stringify({
  basisChange: fire('amz-basis', 'change'),
  perYrChange: fire('amz-perYr', 'change'),
  amountInput: fire('amz-amount', 'input'),
}));
`

	cmd := exec.Command(nodePath, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}

	type ev struct {
		Sched []string `json:"sched"`
		Cell  []string `json:"cell"`
	}
	var res struct {
		BasisChange ev `json:"basisChange"`
		PerYrChange ev `json:"perYrChange"`
		AmountInput ev `json:"amountInput"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}

	has := func(xs []string, v string) bool {
		for _, x := range xs {
			if x == v {
				return true
			}
		}
		return false
	}

	// The fix: a Basis change must schedule a recalc.
	if !has(res.BasisChange.Sched, "amz-basis") {
		t.Errorf("changing Basis did not schedule an auto-calc "+
			"(scheduleAutoCalc never called); sched=%v — the dropdown "+
			"blanks the result and waits for a manual Calculate click",
			res.BasisChange.Sched)
	}
	if !has(res.BasisChange.Cell, "amz-basis") {
		t.Errorf("changing Basis did not invalidate stale outputs; cell=%v",
			res.BasisChange.Cell)
	}
	// Same gap affected the Pmts/Yr dropdown.
	if !has(res.PerYrChange.Sched, "amz-perYr") {
		t.Errorf("changing Pmts/Yr did not schedule an auto-calc; sched=%v",
			res.PerYrChange.Sched)
	}

	// Guard the other half: a text 'input' must NOT schedule a recalc
	// (recompute happens on commit/focusout, not per keystroke), but it
	// must still invalidate stale outputs.
	if len(res.AmountInput.Sched) != 0 {
		t.Errorf("typing into a text field scheduled an auto-calc on "+
			"'input' (should recompute on commit/focusout only); sched=%v",
			res.AmountInput.Sched)
	}
	if !has(res.AmountInput.Cell, "amz-amount") {
		t.Errorf("typing into Amt Borrowed did not invalidate stale "+
			"outputs; cell=%v", res.AmountInput.Cell)
	}
}
