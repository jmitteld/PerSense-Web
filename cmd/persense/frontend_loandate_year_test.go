package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestFrontendLoanDateYearJS guards the regression where a general (non-DOB)
// date field left with a bare two-digit year — e.g. the amortization loan date
// typed as "05/01/20" — was rejected as an incomplete date instead of being
// completed to 20XX. maskDateInput expands 2-digit years live but DELIBERATELY
// defers "19"/"20" (they begin a full 4-digit year the user may still be typing);
// the on-blur finalizer expandGeneralDateYear must complete them. The dash-
// delimiter refactor had dropped the general expander, leaving only the DOB and
// amz first/last ones — so the loan date never completed. Runs the SHIPPED JS
// (extracted from static/index.html) under node; skips when node is absent.
func TestFrontendLoanDateYearJS(t *testing.T) {
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
function saveStateSoon() {}
` + extractJSFunc(t, html, "parseDate") + `
` + extractJSFunc(t, html, "expandGeneralDateYear") + `
function blur(v){ const el = { value: v, id: 'amz-loanDate' }; expandGeneralDateYear(el); return el.value; }
const cases = [
  ['05/01/20',  '05/01/2020'],   // the reported bug: 2-digit "20" completes to 2020
  ['05/01/19',  '05/01/2019'],   // the other deferred prefix
  ['3-15-20',   '3-15-2020'],    // dash delimiter preserved
  ['05/01/2020','05/01/2020'],   // already 4-digit -> no-op
  ['08/01/2021','08/01/2021'],   // full date untouched
  ['',          ''],             // empty -> no-op
];
let fails = 0;
for (const [inp, want] of cases) {
  const got = blur(inp);
  if (got !== want) { console.log('EXPAND FAIL "'+inp+'" -> "'+got+'" want "'+want+'"'); fails++; }
  if (got && got.length >= 8 && parseDate(got) === null) { console.log('PARSE FAIL "'+got+'" rejected by parseDate'); fails++; }
}
// Confirm the setup: parseDate rejects the bare 2-digit year (that is WHY the
// blur finalizer is needed — the UI must expand before parseDate is reached).
if (parseDate('05/01/20') !== null) { console.log('SETUP FAIL: parseDate should reject a bare 2-digit year'); fails++; }
console.log(fails ? ('FAILURES=' + fails) : 'ALLPASS');
`
	cmd := exec.Command(nodePath, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ALLPASS") {
		t.Fatalf("loan-date 2-digit-year completion regression:\n%s", out)
	}
}
