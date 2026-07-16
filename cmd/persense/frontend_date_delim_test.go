package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Tests for the frontend "/" or "-" date delimiter support
// (static/index.html: parseDate, dateValidity, maskDateInput, expandDOBYear).
// Like frontend_dob_year_test.go, this extracts the SHIPPED functions from
// index.html and runs them under Node — the code under test is the shipped
// code, not a copy. Skips when node is not installed.
//
// Requirement (client, 2026-07-16): a date field accepts either "/" or "-";
// whichever delimiter the user types first locks the field and the rest stay
// consistent. The dash form uses MM-DD-YYYY order (a mirror of MM/DD/YYYY), so
// "03-15-2026" means the same date as "03/15/2026". A field that mixes the two
// is rejected. ISO YYYY-MM-DD is still accepted by parseDate for
// programmatic/round-tripped values (pv-asOfDate, VR rate rows): a dash value
// is ISO when its first segment is 4 digits (unambiguously a year), the
// MM-DD-YYYY mirror otherwise.
//
// extractJSFunc is defined in frontend_dob_year_test.go (same package).
func TestFrontendDateDelimiterJS(t *testing.T) {
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
// Deterministic clock so the DOB century pivot is stable (pretend it's 2026).
const RealDate = Date;
let mockYear = 2026;
Date = class extends RealDate { getFullYear() { return mockYear; } };

` + extractJSFunc(t, html, "parseDate") + `

` + extractJSFunc(t, html, "dateValidity") + `

` + extractJSFunc(t, html, "maskDateInput") + `

` + extractJSFunc(t, html, "expandDOBYear") + `

// typeInto simulates the real keystroke path: each character lands in the
// field, then the input handler calls maskDateInput with the actual separator
// character (or '' for digits/deletes) — matching the shipped call site.
function typeInto(id, text) {
	const el = {
		id: id, value: '', dataset: {},
		selectionStart: 0,
		setSelectionRange(a) { this.selectionStart = a; },
	};
	for (const ch of text) {
		el.value += ch;
		el.selectionStart = el.value.length;
		maskDateInput(el, false, /\D/.test(ch) ? ch : '');
	}
	return el.value;
}
function blurDOB(v) { const el = { value: v }; expandDOBYear(el); return el.value; }

const out = {
	// parseDate: dashes mirror slashes (MM-DD-YYYY == MM/DD/YYYY); mixed and
	// two-digit years reject; ISO year-first (YYYY-MM-DD) still parses.
	parse: {
		slash:      parseDate('03/15/2026'),   // '2026-03-15'
		dash:       parseDate('03-15-2026'),   // '2026-03-15'
		dashUnpad:  parseDate('3-15-2026'),    // '2026-03-15'
		mixed:      parseDate('03/15-2026'),   // null
		iso:        parseDate('2026-03-15'),   // '2026-03-15' (ISO preserved)
		twoDigit:   parseDate('03/15/50'),     // null
	},
	// dateValidity: accepts either delimiter in MM/DD/YYYY order; rejects mixed
	// and impossible calendar dates.
	validity: {
		dashOK:     dateValidity('03-15-2026').valid,   // true
		slashOK:    dateValidity('03/15/2026').valid,   // true
		mixed:      dateValidity('03/15-2026').valid,   // false
		mixedMsg:   dateValidity('03/15-2026').msg,     // mentions one delimiter
		badMonth:   dateValidity('13-01-2026').valid,   // false
		badDay:     dateValidity('02-30-2026').valid,   // false
		leapOK:     dateValidity('02-29-2024').valid,   // true
	},
	// maskDateInput: first user-typed delimiter locks the field; the other is
	// normalized to it; digits-only defaults to '/'.
	mask: {
		dashKept:      typeInto('amz-loanDate', '03-15-2026'),  // '03-15-2026'
		slashKept:     typeInto('amz-loanDate', '03/15/2026'),  // '03/15/2026'
		digitsDefault: typeInto('amz-loanDate', '03152026'),    // '03/15/2026'
		slashThenDash: typeInto('amz-loanDate', '03/15-2026'),  // '03/15/2026'
		dashThenSlash: typeInto('amz-loanDate', '03-15/2026'),  // '03-15-2026'
	},
	// Two-digit-year expansion works over dashes too, keeping the DOB vs general
	// century split: DOB pivots on the current year, others expand to 20XX.
	yearExpand: {
		dobDash50:  typeInto('actu-dob1', '07-20-50'),   // '07-20-1950'
		dobDash10:  typeInto('actu-dob1', '03-15-10'),   // '03-15-2010'
		genDash50:  typeInto('pv-asOfDate', '01-01-50'), // '01-01-2050'
		genSlash50: typeInto('pv-asOfDate', '01/01/50'), // '01/01/2050'
		blurDash:   blurDOB('07-20-50'),                 // '07-20-1950'
		blurSlash:  blurDOB('07/20/50'),                 // '07/20/1950'
	},
};
console.log(JSON.stringify(out));
`

	cmd := exec.Command(nodePath, "-")
	cmd.Stdin = strings.NewReader(harness)
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, stdout)
	}

	var r struct {
		Parse struct {
			Slash, Dash, DashUnpad, Mixed, ISO, TwoDigit *string
		}
		Validity struct {
			DashOK, SlashOK, Mixed, BadMonth, BadDay, LeapOK bool
			MixedMsg                                         string
		}
		Mask struct {
			DashKept, SlashKept, DigitsDefault, SlashThenDash, DashThenSlash string
		}
		YearExpand struct {
			DobDash50, DobDash10, GenDash50, GenSlash50, BlurDash, BlurSlash string
		}
	}
	if err := json.Unmarshal(stdout, &r); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, stdout)
	}

	eqPtr := func(name string, got *string, want string) {
		if got == nil || *got != want {
			g := "null"
			if got != nil {
				g = *got
			}
			t.Errorf("%s = %q, want %q", name, g, want)
		}
	}
	nilPtr := func(name string, got *string) {
		if got != nil {
			t.Errorf("%s = %q, want null", name, *got)
		}
	}
	eqStr := func(name, got, want string) {
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	isTrue := func(name string, got bool) {
		if !got {
			t.Errorf("%s = false, want true", name)
		}
	}
	isFalse := func(name string, got bool) {
		if got {
			t.Errorf("%s = true, want false", name)
		}
	}

	// parseDate
	eqPtr("parseDate slash", r.Parse.Slash, "2026-03-15")
	eqPtr("parseDate dash", r.Parse.Dash, "2026-03-15")
	eqPtr("parseDate dash unpadded", r.Parse.DashUnpad, "2026-03-15")
	nilPtr("parseDate mixed", r.Parse.Mixed)
	eqPtr("parseDate ISO year-first (YYYY-MM-DD preserved)", r.Parse.ISO, "2026-03-15")
	nilPtr("parseDate two-digit year", r.Parse.TwoDigit)

	// dateValidity
	isTrue("dateValidity dash", r.Validity.DashOK)
	isTrue("dateValidity slash", r.Validity.SlashOK)
	isFalse("dateValidity mixed", r.Validity.Mixed)
	if !strings.Contains(strings.ToLower(r.Validity.MixedMsg), "delimiter") {
		t.Errorf("dateValidity mixed message = %q, want it to mention the delimiter", r.Validity.MixedMsg)
	}
	isFalse("dateValidity bad month", r.Validity.BadMonth)
	isFalse("dateValidity bad day", r.Validity.BadDay)
	isTrue("dateValidity leap day", r.Validity.LeapOK)

	// maskDateInput
	eqStr("mask dash kept", r.Mask.DashKept, "03-15-2026")
	eqStr("mask slash kept", r.Mask.SlashKept, "03/15/2026")
	eqStr("mask digits default to slash", r.Mask.DigitsDefault, "03/15/2026")
	eqStr("mask slash-then-dash normalized to slash", r.Mask.SlashThenDash, "03/15/2026")
	eqStr("mask dash-then-slash normalized to dash", r.Mask.DashThenSlash, "03-15-2026")

	// year expansion over dashes
	eqStr("DOB dash 50 -> 1950", r.YearExpand.DobDash50, "07-20-1950")
	eqStr("DOB dash 10 -> 2010", r.YearExpand.DobDash10, "03-15-2010")
	eqStr("general dash 50 -> 2050", r.YearExpand.GenDash50, "01-01-2050")
	eqStr("general slash 50 -> 2050 (unchanged)", r.YearExpand.GenSlash50, "01/01/2050")
	eqStr("blur DOB dash -> 1950", r.YearExpand.BlurDash, "07-20-1950")
	eqStr("blur DOB slash -> 1950", r.YearExpand.BlurSlash, "07/20/1950")
}
