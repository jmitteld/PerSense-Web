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
// NF-2 is the purest instance yet: the ENGINE was never wrong — it snapped the
// adjustment date onto the payment grid exactly as Amortize.pas:258-271 does,
// and both engines echoed the snapped date correctly. The defect was one strict
// comparison in `index.html`, where `rowISO` is parsed from the DOM row's own
// date input and still holds what the USER typed.
//
// 🚨 AND ROUND 45b ADDS THE SEQUEL, WHICH IS WHY THIS FILE NOW REACHES INTO THE
// VALIDATION LAYER. The round-45 fix wrote the snapped date back as `a.date` —
// the RAW WIRE VALUE, which is ISO (`2034-11-30`). This input is validated by
// `dateValidity`, which REJECTS ISO and demands MM/DD/YYYY. So the first
// Calculate succeeded and the SECOND — with the user changing nothing — was
// blocked with "Incomplete date — use MM/DD/YYYY". Measured on the live page:
// 21 of 76 adjustment-carrying screens (28%) became unsubmittable.
//
// The original version of THIS TEST passed throughout, because it asserted the
// painted cell equalled `a.date` against a FAKE DOM with no validator — it
// checked the value it expected instead of asking whether the app would accept
// it. That is R42 committed by the very guard written to enforce R42.
//
// → THE STANDING SHAPE: A PAINTED CELL IS AN INPUT TO THE NEXT SUBMIT. Any test
// that paints a value must feed that value back through the REAL validator, not
// compare it to a literal.

func r45Extract(t *testing.T, src, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)(?:async )?function ` + name + `\([^)]*\) \{.*?\n\}`)
	s := re.FindString(src)
	if s == "" {
		t.Fatalf("function %s not found in index.html", name)
	}
	return s
}

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
	// Cross-file assertions (R50): the expected expressions live HERE, not in the
	// file being read, so a silent revert cannot satisfy them. FULL phrases only
	// (R38's round-44 lesson about prefix needles).
	if !strings.Contains(block, "rowISO !== a.date && rowISO !== a.requestedDate") {
		t.Fatalf("NF-2 REGRESSED: the echo/row match no longer falls back to "+
			"`requestedDate`. A strict compare against `a.date` alone paints "+
			"NOTHING on any off-grid adjustment date. Extracted block:\n%s", block)
	}
	if !strings.Contains(block, "dEl.value = fmtDateDisplay(a.date);") {
		t.Fatalf("NF-2b REGRESSED: the snapped date is being written back RAW " +
			"instead of through `fmtDateDisplay`. The wire carries ISO and this " +
			"field's validator (`dateValidity`) rejects ISO, so the cell ends up " +
			"holding a value the app itself refuses — the screen calculates once " +
			"and is then blocked on the user's next Calculate.")
	}

	harness := `
'use strict';
function parseDate(s){
  if(!s||!s.trim())return null;
  const t=s.trim();
  if(/^\d{4}-\d{2}-\d{2}$/.test(t))return t;
  const m=/^(\d{1,2})([\/-])(\d{1,2})\2(\d{4})$/.exec(t);
  if(!m)return null;
  return m[4]+'-'+String(+m[1]).padStart(2,'0')+'-'+String(+m[3]).padStart(2,'0');
}
function inferAmzDateFromLoan(){ return null; }
function fmtDollars(v){ return v.toFixed(2); }
` + r45Extract(t, src, "fmtDateDisplay") + `
` + r45Extract(t, src, "dateValidity") + `

function mkCell(v){
  return { value: v, _cls: {},
    classList: { add(c){ this._cls=this._cls||{}; this._cls[c]=true; },
                 remove(c){ if(this._cls) delete this._cls[c]; },
                 contains(c){ return !!(this._cls && this._cls[c]); } } };
}
function mkRow(dateText){
  const cells = { date: mkCell(dateText), rate: mkCell(''), amount: mkCell('') };
  return { cells, querySelector(sel){
      const m = /data-amz-adj-field="([a-z]+)"/.exec(sel);
      return m ? this.cells[m[1]] : null; } };
}

const ROWS = [ mkRow(CASE.rowTyped) ];
const document = {
  getElementById(){ return { value: '01/01/2024' }; },
  querySelectorAll(){ return ROWS; },
};

const data = { adjustments: CASE.echo };
` + block + `
// 🚨 THE ASSERTION THAT WAS MISSING: feed the PAINTED cell back through the
// app's OWN validator. A painted cell is an input to the next submit.
const painted = ROWS[0].cells.date.value;
console.log(JSON.stringify({
  date: painted,
  dateOut: ROWS[0].cells.date.classList.contains('cell-output'),
  dateValid: dateValidity(painted).valid,
  dateMsg: dateValidity(painted).msg,
  amount: ROWS[0].cells.amount.value,
  rate: ROWS[0].cells.rate.value,
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
			// 🚨 THE NF-2 CASE. The user typed 12 Jan 2029 (in the field's own
			// MM/DD/YYYY form, as a real user does); DOS snaps to 1 Jan 2029 and
			// the wire reports the SNAPPED date in ISO. Verified round 45:
			//   amort_oracle 100000 0.07 360 12 adjdmy=12.1.2029:0.08: r78 adjdump
			//     → adjrow 1 date 1/1/2029 ... amount 726.522878 amtstatus 1
			// The painted cell must be 01/01/2029 — DISPLAY form — and must pass
			// `dateValidity`.
			name: "off_grid_typed_date_paints_and_shows_the_snapped_date",
			js: `const CASE = { rowTyped: '01/12/2029', echo: [
				{date:'2029-01-01', requestedDate:'2029-01-12',
				 amount:726.522878, amountSolved:true}] };`,
			want: want{date: "01/01/2029", amount: "726.52", dateOut: true},
		},
		{
			// NEGATIVE CONTROL. An on-grid row carries no `requestedDate`, so the
			// date cell must be left EXACTLY as typed and NOT marked computed.
			name: "on_grid_row_is_painted_without_touching_the_date_cell",
			js: `const CASE = { rowTyped: '01/01/2031', echo: [
				{date:'2031-01-01', amount:723.211515, amountSolved:true}] };`,
			want: want{date: "01/01/2031", amount: "723.21", dateOut: false},
		},
		{
			// A row the echo does not mention must be left alone entirely.
			name: "unrelated_row_is_not_painted",
			js: `const CASE = { rowTyped: '06/01/2035', echo: [
				{date:'2031-01-01', amount:723.211515, amountSolved:true}] };`,
			want: want{date: "06/01/2035", amount: "", dateOut: false},
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
			// 🚨 THE CROSS-LAYER ASSERTION. Whatever ends up in that cell — painted
			// or untouched — must be a value this app will accept on the NEXT
			// submit. Round 45 shipped a cell the app's own validator refused.
			if !strings.Contains(got, `"dateValid":true`) {
				t.Errorf("THE PAINTED DATE IS REJECTED BY THE APP'S OWN VALIDATOR. "+
					"The user calculates once, then the next Calculate is blocked "+
					"with an 'Incomplete date' error on a cell THEY did not type. "+
					"got: %s", got)
			}
		})
	}
}
