package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// Frontend coverage for DOS's TERMINATING balloon display row
// (TackOnFinalBalloon, Amortize.pas:1040-1088 — see docs/discrepancies.md §46).
//
// DOS computes the balance an over-specified advanced-options loan still owes
// on its terminal date, writes it into the balloon array as an OUTPUT cell, and
// then de-activates it with `dec(nballoons)`. BalloonValues2Grid
// (AmortizationScreenUnit.pas:1691-1713) walks the RAW array, so the row is
// still painted into the Balloon Payments grid — but MakeTable and the APR both
// stop at nballoons and never see it. The port reproduces that: the engine
// keeps the row out of the payment table and the APR (see
// TestTackOnFinalBalloonExcludedFromTableAndAPR), the API flags it `tackedOn`,
// and the front end paints it as a read-only output row.
//
// These tests pin the display half. Like the other frontend tests in this
// package they run the SHIPPED functions out of static/index.html under Node
// with a minimal DOM stub, so the code under test is the real code.

// TestTackBalloonRowIsolatedFromInputGrid pins the structural guarantee that
// makes the display row safe: it lives in its own <tbody>, so no selector keyed
// on #amz-balloon-body can see it. If it were inside #amz-balloon-body, the
// request collector would read its date and — because a green (cell-output)
// amount is deliberately read as blank — re-submit the engine's own computed
// row as a user "target balloon", changing the answer on every recalculation.
func TestTackBalloonRowIsolatedFromInputGrid(t *testing.T) {
	html := readIndexHTML(t)

	i := strings.Index(html, `<tbody id="amz-balloon-body">`)
	if i < 0 {
		t.Fatal("could not locate the #amz-balloon-body tbody")
	}
	j := strings.Index(html, `<tbody id="amz-balloon-tack-body"`)
	if j < 0 {
		t.Fatal("the terminating-balloon display tbody (#amz-balloon-tack-body) is gone; " +
			"DOS paints this row into the Balloon Payments grid (BalloonValues2Grid)")
	}
	// It must open AFTER #amz-balloon-body has closed.
	closeIdx := strings.Index(html[i:], "</tbody>")
	if closeIdx < 0 || i+closeIdx > j {
		t.Error("#amz-balloon-tack-body is nested inside (or opens before the close of) " +
			"#amz-balloon-body. It must be a separate tbody: the request collector, row " +
			"counter, undo snapshot, import and Add Row all select on #amz-balloon-body, " +
			"and none of them may ever see the engine's computed row.")
	}

	// The collector must keep selecting user rows by attribute, never by
	// position or by a bare `tr`.
	if !strings.Contains(html, `document.querySelectorAll('#amz-balloon-body [data-amz-balloon-row]')`) {
		t.Error("the balloon request collector no longer selects " +
			"`#amz-balloon-body [data-amz-balloon-row]`; a looser selector risks " +
			"round-tripping the tacked-on display row back to the engine as a user balloon")
	}

	// The tacked row must never carry the input-grid data attributes.
	fn := extractJSFunc(t, html, "renderAmzTackBalloon")
	for _, attr := range []string{"data-amz-balloon-row", "data-amz-balloon-field"} {
		if strings.Contains(fn, attr) {
			t.Errorf("renderAmzTackBalloon sets %s on the display row; that makes the "+
				"engine's computed balloon indistinguishable from one the user typed", attr)
		}
	}
	if regexp.MustCompile(`createElement\(\s*['"]input['"]`).MatchString(fn) {
		t.Error("renderAmzTackBalloon creates an <input>; the row is display-only " +
			"(DOS paints it as an output cell and de-activates it), and an input here " +
			"would also be picked up by the `#amz-balloon-body input` clear sweeps")
	}

	// The render path must actually call it, or the row never appears.
	if !strings.Contains(html, "renderAmzTackBalloon(data.balloons)") {
		t.Error("the amortization result renderer no longer calls " +
			"renderAmzTackBalloon(data.balloons); the tacked-on row will never be painted")
	}
}

// TestTackBalloonRenderJS runs the shipped renderer under Node and pins what it
// paints: the terminating balloon's date and amount as output cells, nothing at
// all when the response carries no tacked row, and a clean removal of a stale
// row from a prior calculation.
//
// The values are the real DOS ones from the differential tests:
//
//	amort_oracle 100000 0.06 360 12 prepaid plusreg pts=0 \
//	  loandmy=1.1.2024 firstdmy=1.2.2024 payhard=1199.10 \
//	  bdate=1.1.2026:50000.00 bdump
//	→ grid row 2 = 1/1/2054  -869413.8700  dstatus 1 astatus 1 (outp/outp)
func TestTackBalloonRenderJS(t *testing.T) {
	html := readIndexHTML(t)

	harness := `
'use strict';
` + extractJSFunc(t, html, "fmtMoney") + `
` + extractJSFunc(t, html, "fmtDollars") + `
` + extractJSFunc(t, html, "fmtDateDisplay") + `
` + extractJSFunc(t, html, "renderAmzTackBalloon") + `
` + extractJSFunc(t, html, "clearAmzTackBalloon") + `

// --- Minimal DOM shim ---
var created = [];
function makeEl(tag) {
  var el = {
    tag: tag, className: '', textContent: '', title: '', hidden: false,
    style: {}, attrs: {}, children: [],
    setAttribute: function (k, v) { this.attrs[k] = v; },
    appendChild: function (c) { this.children.push(c); },
    set innerHTML(v) { this._html = v; if (v === '') this.children = []; },
    get innerHTML() { return this._html || ''; },
  };
  created.push(el);
  return el;
}
var tackBody = makeEl('tbody');
var document = {
  getElementById: function (id) { return id === 'amz-balloon-tack-body' ? tackBody : null; },
  createElement: function (tag) { return makeEl(tag); },
};

// Flatten the painted row into [{text, className, tag}] cell descriptors.
function snapshot() {
  return {
    hidden: tackBody.hidden,
    rows: tackBody.children.map(function (tr) {
      return {
        attrs: Object.keys(tr.attrs),
        title: tr.title,
        cells: tr.children.map(function (td) {
          var inner = td.children[0] || {};
          return { tag: inner.tag, className: inner.className, text: inner.textContent };
        }),
      };
    }),
  };
}

var out = {};

// (1) A user balloon plus DOS's terminating balloon (the API echo order).
renderAmzTackBalloon([
  { date: '2026-01-01', amount: 50000, solved: false },
  { date: '2054-01-01', amount: -869413.87, tackedOn: true },
]);
out.owedNegative = snapshot();

// (2) A positive residual — a balance still owed on the terminal date.
renderAmzTackBalloon([{ date: '2055-01-01', amount: 217436, tackedOn: true }]);
out.owedPositive = snapshot();

// (3) A response with ordinary balloons only: nothing painted.
renderAmzTackBalloon([{ date: '2026-01-01', amount: 50000, solved: true }]);
out.noTack = snapshot();

// (4) A stale row must not survive a reset.
renderAmzTackBalloon([{ date: '2055-01-01', amount: 217436, tackedOn: true }]);
clearAmzTackBalloon();
out.cleared = snapshot();

// (5) Defensive: a response with no balloons array at all.
renderAmzTackBalloon(undefined);
out.undefinedArg = snapshot();

out.anyInputCreated = created.some(function (e) { return e.tag === 'input'; });
console.log(JSON.stringify(out));
`
	out := runNode(t, harness, "")

	type cell struct {
		Tag       string `json:"tag"`
		ClassName string `json:"className"`
		Text      string `json:"text"`
	}
	type row struct {
		Attrs []string `json:"attrs"`
		Title string   `json:"title"`
		Cells []cell   `json:"cells"`
	}
	type snap struct {
		Hidden bool  `json:"hidden"`
		Rows   []row `json:"rows"`
	}
	var res struct {
		OwedNegative    snap `json:"owedNegative"`
		OwedPositive    snap `json:"owedPositive"`
		NoTack          snap `json:"noTack"`
		Cleared         snap `json:"cleared"`
		UndefinedArg    snap `json:"undefinedArg"`
		AnyInputCreated bool `json:"anyInputCreated"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}

	// (1) The DOS row from the differential test, painted as output cells.
	if len(res.OwedNegative.Rows) != 1 {
		t.Fatalf("tacked-on row count = %d, want exactly 1 (DOS tacks at most one "+
			"terminating balloon); snapshot %+v", len(res.OwedNegative.Rows), res.OwedNegative)
	}
	r := res.OwedNegative.Rows[0]
	if res.OwedNegative.Hidden {
		t.Error("the tack tbody stayed hidden while carrying a row")
	}
	if len(r.Cells) != 2 {
		t.Fatalf("tacked-on row has %d cells, want 2 (Date, Amount) to line up with the grid", len(r.Cells))
	}
	if r.Cells[0].Text != "01/01/2054" {
		t.Errorf("tacked-on Date = %q, want %q (DOS grid row 2: 1/1/2054)", r.Cells[0].Text, "01/01/2054")
	}
	if r.Cells[1].Text != "$-869,413.87" {
		t.Errorf("tacked-on Amount = %q, want %q (DOS grid row 2: -869413.8700)",
			r.Cells[1].Text, "$-869,413.87")
	}
	for i, c := range r.Cells {
		if !strings.Contains(c.ClassName, "cell-output") {
			t.Errorf("tacked-on cell %d class = %q, want it to carry cell-output — DOS paints "+
				"both the date and the amount with status `outp`, and the user must see this "+
				"is computed, not something they typed", i, c.ClassName)
		}
		if c.Tag == "input" {
			t.Errorf("tacked-on cell %d is an <input>; the row is display-only", i)
		}
	}
	if _, ok := attrSet(r.Attrs)["data-amz-balloon-tack-row"]; !ok {
		t.Errorf("tacked-on row attrs = %v, want data-amz-balloon-tack-row so the row is "+
			"identifiable and cannot be mistaken for a user row", r.Attrs)
	}
	if !strings.Contains(r.Title, "excluded") {
		t.Errorf("tacked-on row tooltip = %q; it must tell the user the row is excluded from "+
			"the schedule and the APR — otherwise the number looks like an unpaid payment", r.Title)
	}

	// (2) A positive residual reads as a balance still owed.
	if len(res.OwedPositive.Rows) != 1 || res.OwedPositive.Rows[0].Cells[1].Text != "$217,436.00" {
		t.Errorf("positive tacked-on amount = %+v, want a single row showing $217,436.00 "+
			"(the skip-months DOS case)", res.OwedPositive.Rows)
	}
	if !strings.Contains(res.OwedPositive.Rows[0].Title, "still owes") {
		t.Errorf("positive tacked-on tooltip = %q, want it to read as a balance still owed",
			res.OwedPositive.Rows[0].Title)
	}

	// (3)-(5) Nothing painted when there is no tacked row.
	for _, tc := range []struct {
		name string
		s    snap
	}{
		{"ordinary balloons only", res.NoTack},
		{"after clearAmzTackBalloon", res.Cleared},
		{"undefined balloons", res.UndefinedArg},
	} {
		if len(tc.s.Rows) != 0 {
			t.Errorf("%s: painted %d rows, want 0 — a stale terminating balloon would "+
				"misreport the previous loan", tc.name, len(tc.s.Rows))
		}
		if !tc.s.Hidden {
			t.Errorf("%s: the tack tbody is visible with no row in it", tc.name)
		}
	}

	if res.AnyInputCreated {
		t.Error("the renderer created an <input> element somewhere; the display row must " +
			"stay out of every `#amz-balloon-body input` sweep and out of the request")
	}
}

func attrSet(attrs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(attrs))
	for _, a := range attrs {
		m[a] = struct{}{}
	}
	return m
}
