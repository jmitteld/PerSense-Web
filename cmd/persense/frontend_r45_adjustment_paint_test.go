package main

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// ROUND-45 — THE CLIENT HALF OF NF-1 AND NF-2.
//
// R42: A TRANSPORT FIX MUST BE VERIFIED AT THE CONSUMER, NOT AT THE PRODUCER.
// This project has shipped that failure four times (round 39, three times; the
// PV screen in round 42). NF-2 is the purest instance yet: the ENGINE was never
// wrong — it snapped the adjustment date onto the payment grid exactly as
// Amortize.pas:258-271 does, and both engines echoed the snapped date correctly.
// The defect was one strict comparison in `index.html`:
//
//	if (rowISO !== a.date) return;
//
// `rowISO` is parsed from the DOM row's own date input, which still holds what
// the USER typed. For any off-grid date that test failed for EVERY row, the
// forEach returned for all of them, and nothing was painted — 16 of 400
// randomized requests in the 39D sweep. A Go-side test can never see this.
//
// This test runs the SHIPPED block from index.html in node against a fake DOM.

// TestR45AdjustmentEchoPaintsOnASnappedDate is NF-2's consumer guard.
func TestR45AdjustmentEchoPaintsOnASnappedDate(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend JS test")
	}
	src := readIndexHTML(t)

	block := regexp.MustCompile(
		`(?s)  if \(Array\.isArray\(data\.adjustments\)\) \{.*?\n  \}`).FindString(src)
	if block == "" {
		t.Fatal("the adjustments echo block was not found in index.html")
	}
	// A cross-file assertion (R50): the expected expression lives HERE, not in
	// the file being read, so a silent revert cannot satisfy it. Use the FULL
	// phrase — R38's round-44 lesson was that a needle which is a prefix of a
	// surviving neighbour leaves the guard green after the record it protects is
	// deleted.
	if !strings.Contains(block, "rowISO !== a.date && rowISO !== a.requestedDate") {
		t.Fatalf("NF-2 REGRESSED: the echo/row match no longer falls back to "+
			"`requestedDate`. A strict compare against `a.date` alone paints "+
			"NOTHING on any off-grid adjustment date. Extracted block:\n%s", block)
	}

	harness := `
'use strict';
function parseDate(s){ return (s && s.trim()) ? s.trim() : null; }
function inferAmzDateFromLoan(){ return null; }
function fmtDollars(v){ return v.toFixed(2); }

function mkCell(v){
  return { value: v, _cls: {},
    classList: { add(c){ this._cls=this._cls||{}; this._cls[c]=true; },
                 contains(c){ return !!(this._cls && this._cls[c]); } } };
}
function mkRow(dateText){
  const cells = { date: mkCell(dateText), rate: mkCell(''), amount: mkCell('') };
  cells.date._cls = {}; cells.rate._cls = {}; cells.amount._cls = {};
  return { cells, querySelector(sel){
      const m = /data-amz-adj-field="([a-z]+)"/.exec(sel);
      return m ? this.cells[m[1]] : null; } };
}

const ROWS = [ mkRow(CASE.rowTyped) ];
const document = {
  getElementById(){ return { value: '2024-01-01' }; },
  querySelectorAll(){ return ROWS; },
};

const data = { adjustments: CASE.echo };
` + block + `
console.log(JSON.stringify({
  date:   ROWS[0].cells.date.value,
  dateOut: ROWS[0].cells.date.classList.contains('cell-output'),
  amount: ROWS[0].cells.amount.value,
  rate:   ROWS[0].cells.rate.value,
}));
`

	type want struct {
		date, amount string
		dateOut      bool
	}
	cases := []struct {
		name string
		js   string
		want want
	}{
		{
			// 🚨 THE NF-2 CASE. The user typed 12 Jan 2029; DOS snaps to 1 Jan 2029
			// and the echo reports the SNAPPED date. Verified round 45:
			//   amort_oracle 100000 0.07 360 12 adjdmy=12.1.2029:0.08: r78 adjdump
			//     → adjrow 1 date 1/1/2029 ... amount 726.522878 amtstatus 1
			// Before the fix: nothing painted at all.
			name: "off_grid_typed_date_paints_and_shows_the_snapped_date",
			js: `const CASE = { rowTyped: '2029-01-12', echo: [
				{date:'2029-01-01', requestedDate:'2029-01-12',
				 amount:726.522878, amountSolved:true}] };`,
			want: want{date: "2029-01-01", amount: "726.52", dateOut: true},
		},
		{
			// THE NEGATIVE CONTROL. An on-grid row carries no `requestedDate`, so
			// the date cell must be left EXACTLY as the user typed it and must NOT
			// be marked as a computed output. A fix that rewrote the date cell
			// unconditionally would pass the case above and quietly relabel every
			// ordinary screen's date as engine output — which the request builder
			// then reads back differently on the next submit.
			name: "on_grid_row_is_painted_without_touching_the_date_cell",
			js: `const CASE = { rowTyped: '2031-01-01', echo: [
				{date:'2031-01-01', amount:723.211515, amountSolved:true}] };`,
			want: want{date: "2031-01-01", amount: "723.21", dateOut: false},
		},
		{
			// A row the echo does not mention must be left alone entirely.
			name: "unrelated_row_is_not_painted",
			js: `const CASE = { rowTyped: '2035-06-01', echo: [
				{date:'2031-01-01', amount:723.211515, amountSolved:true}] };`,
			want: want{date: "2035-06-01", amount: "", dateOut: false},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "*.js")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(c.js + harness); err != nil {
				t.Fatal(err)
			}
			f.Close()
			out, err := exec.Command(nodePath, f.Name()).CombinedOutput()
			if err != nil {
				t.Fatalf("node failed: %v\n%s", err, out)
			}
			got := strings.TrimSpace(string(out))
			for _, probe := range []struct{ key, val string }{
				{`"date":"` + c.want.date + `"`, "date cell"},
				{`"amount":"` + c.want.amount + `"`, "amount cell"},
			} {
				if !strings.Contains(got, probe.key) {
					t.Errorf("%s wrong.\n got: %s\nwant substring: %s",
						probe.val, got, probe.key)
				}
			}
			wantOut := `"dateOut":false`
			if c.want.dateOut {
				wantOut = `"dateOut":true`
			}
			if !strings.Contains(got, wantOut) {
				t.Errorf("date cell-output marking wrong.\n got: %s\nwant substring: %s",
					got, wantOut)
			}
		})
	}
}
