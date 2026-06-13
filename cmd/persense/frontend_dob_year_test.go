package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// Tests for the frontend DOB two-digit-year expansion
// (static/index.html: expandDOBYear). There is no JS test harness in
// this project, so this test extracts the function source from
// index.html and executes it under Node — the code under test is the
// shipped code, not a copy. Skips when node is not installed.
//
// Properties pinned:
//  1. Pivot is the CURRENT year (birth dates are never future):
//     yy > currentYY → 19yy, else 20yy. Verified under two mocked
//     "current years" (2026 and 2050) to prove the pivot is dynamic,
//     not hard-coded.
//  2. Only exact two-digit years expand: 4-digit years, partial input,
//     and non-date text are untouched.
//  3. saveStateSoon fires only when an expansion actually happened.
//  4. The DOB exception does NOT leak into general date parsing:
//     parseDate (also extracted) still rejects two-digit years.

// extractJSFunc pulls a top-level `function name(...) {...}` block out
// of the page source. Functions in index.html close with a brace at
// column 0, so the non-greedy match to "\n}" is exact.
func extractJSFunc(t *testing.T, html, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)function ` + name + `\([^)]*\) \{.*?\n\}`)
	src := re.FindString(html)
	if src == "" {
		t.Fatalf("function %s not found in index.html", name)
	}
	return src
}

func TestExpandDOBYearJS(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend JS test")
	}
	htmlBytes, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(htmlBytes)

	harness := `
'use strict';
let savedCount = 0;
function saveStateSoon() { savedCount++; }

` + extractJSFunc(t, html, "expandDOBYear") + `

` + extractJSFunc(t, html, "parseDate") + `

` + extractJSFunc(t, html, "maskDateInput") + `

// typeInto simulates the real keystroke path: each character lands in
// the field, then the global input handler calls maskDateInput — the
// same sequence the browser produces while typing. This is the layer
// the original focusout-only fix missed: the mask expands two-digit
// years LIVE (hard-coded 20XX before the fix), so a later focusout
// expansion never saw a two-digit year.
function typeInto(id, text) {
	const el = {
		id: id, value: '', dataset: {},
		selectionStart: 0,
		setSelectionRange(a, b) { this.selectionStart = a; },
	};
	for (const ch of text) {
		el.value += ch;
		el.selectionStart = el.value.length;
		// Mirrors the global input handler: sep = single non-digit char.
		maskDateInput(el, false, /\D/.test(ch));
	}
	return el.value;
}

// Mock the clock so the pivot is deterministic, switchable per case.
const RealDate = Date;
let mockYear = 2026;
// eslint-disable-next-line no-global-assign
Date = class extends RealDate {
	getFullYear() { return mockYear; }
};

function run(value, year) {
	mockYear = year;
	const before = savedCount;
	const el = { value: value };
	expandDOBYear(el);
	return { out: el.value, saved: savedCount > before };
}

const results = {
	cases: [
		// [input, mockYear, expected, expectSave]
		['01/01/50',   2026, '01/01/1950', true],  // the motivating example
		['01/01/27',   2026, '01/01/1927', true],  // just past the pivot
		['01/01/26',   2026, '01/01/2026', true],  // pivot year itself → newborn
		['01/01/10',   2026, '01/01/2010', true],
		['01/01/00',   2026, '01/01/2000', true],
		['01/01/99',   2026, '01/01/1999', true],
		['01/01/50',   2050, '01/01/2050', true],  // dynamic pivot: in 2050, 50 is current
		['01/01/51',   2050, '01/01/1951', true],
		['01/01/1950', 2026, '01/01/1950', false], // 4-digit untouched
		['01/01/5',    2026, '01/01/5',    false], // partial untouched
		['1/1/50',     2026, '1/1/1950',   true],  // unpadded month/day still expands
		['  01/01/50', 2026, '01/01/1950', true],  // trimmed
		['garbage',    2026, 'garbage',    false],
		['',           2026, '',           false],
	].map(([inp, yr, want, wantSave]) => {
		const r = run(inp, yr);
		return { inp, yr, want, wantSave, got: r.out, saved: r.saved };
	}),
	// Property 4: general parsing still rejects two-digit years — the
	// DOB convenience must not weaken parseDate's 4-digit rule.
	parseRejects: parseDate('01/01/50') === null && parseDate('5/15/57') === null,
	parseAccepts: parseDate('01/01/1950') === '1950-01-01',
	// Property 5: the live typing mask. DOB fields pivot on the current
	// year; other date fields keep the 20XX expansion; literal 4-digit
	// years type through unchanged on both.
	typed: (() => {
		mockYear = 2026;
		return [
			{ id: 'actu-dob1',  text: '01/01/50',   want: '01/01/1950' }, // the user's repro
			{ id: 'actu-dob2',  text: '01/01/50',   want: '01/01/1950' },
			{ id: 'actu-dob1',  text: '01/01/10',   want: '01/01/2010' },
			{ id: 'actu-dob1',  text: '10/10/1940', want: '10/10/1940' }, // literal 19XX waits, no double-expand
			{ id: 'actu-dob1',  text: '01/01/2010', want: '01/01/2010' },
			{ id: 'pv-asOfDate', text: '01/01/50',  want: '01/01/2050' }, // non-DOB keeps 20XX
			{ id: 'pv-asOfDate', text: '01/01/2050', want: '01/01/2050' },
		].map(c => ({ ...c, got: typeInto(c.id, c.text) }));
	})(),
};
console.log(JSON.stringify(results));
`

	cmd := exec.Command(nodePath, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}

	var res struct {
		Cases []struct {
			Inp      string `json:"inp"`
			Yr       int    `json:"yr"`
			Want     string `json:"want"`
			WantSave bool   `json:"wantSave"`
			Got      string `json:"got"`
			Saved    bool   `json:"saved"`
		} `json:"cases"`
		ParseRejects bool `json:"parseRejects"`
		ParseAccepts bool `json:"parseAccepts"`
		Typed        []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
			Want string `json:"want"`
			Got  string `json:"got"`
		} `json:"typed"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}

	for _, c := range res.Cases {
		if c.Got != c.Want {
			t.Errorf("expandDOBYear(%q) in %d = %q, want %q", c.Inp, c.Yr, c.Got, c.Want)
		}
		if c.Saved != c.WantSave {
			t.Errorf("expandDOBYear(%q) in %d: saveStateSoon fired = %v, want %v",
				c.Inp, c.Yr, c.Saved, c.WantSave)
		}
	}
	if !res.ParseRejects {
		t.Error("parseDate accepted a two-digit year — the DOB-only exception leaked into general date parsing")
	}
	if !res.ParseAccepts {
		t.Error("parseDate failed on a valid 4-digit date")
	}
	if len(res.Typed) == 0 {
		t.Fatal("typing-mask cases missing from node output")
	}
	for _, c := range res.Typed {
		if c.Got != c.Want {
			t.Errorf("typing %q into #%s = %q, want %q", c.Text, c.ID, c.Got, c.Want)
		}
	}
}
