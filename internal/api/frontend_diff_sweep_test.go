package api

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Frontend↔engine DIFFERENTIAL SWEEP (see docs/convergence_feasibility.md).
//
// The DOS-oracle sweeps validate the Go ENGINE. This harness extends the same
// idea up into the frontend: it drives random loans through the real
// /api/amortization/calc handler (whose numbers are oracle-validated) and then
// renders each result with the SHIPPED renderAmzSchedule JS, asserting the
// displayed schedule reconciles with the engine across the whole sweep. It is
// the automated form of the manual "try a combination and eyeball the table"
// loop — the balance-column drift, timezone bucketing, and CSV bugs found by
// hand would all have failed here.
//
// Skips when Node is unavailable.

func extractJS(t *testing.T, html, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)(?:async )?function ` + name + `\([^)]*\) \{.*?\n\}`)
	s := re.FindString(html)
	if s == "" {
		t.Fatalf("function %s not found in index.html", name)
	}
	return s
}

// modalRegular returns the most frequent regular-payment amount (payNum >= 1)
// in a schedule — the steady payment the schedule settles to.
func modalRegular(s []PaymentLine) float64 {
	cnt := map[string]int{}
	best := 0
	var bestV float64
	for _, r := range s {
		if r.PayNum < 1 {
			continue
		}
		k := fmt.Sprintf("%.2f", r.Payment)
		cnt[k]++
		if cnt[k] > best {
			best, bestV = cnt[k], r.Payment
		}
	}
	return bestV
}

func parseMoneyStr(s string) float64 {
	s = strings.NewReplacer("$", "", ",", "", " ", "").Replace(strings.TrimSpace(s))
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func TestFrontendAmzScheduleRenderSweepDOS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping frontend differential sweep")
	}
	htmlBytes, err := os.ReadFile("../../cmd/persense/static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(htmlBytes)

	// --- Build the sweep: random loans through the real handler ---
	type sweepCase struct {
		Amount   float64       `json:"amount"`
		Schedule []PaymentLine `json:"schedule"`
	}
	rng := rand.New(rand.NewSource(20260613))
	amounts := []float64{50000, 100000, 175000, 250000, 400000}
	rates := []float64{0.03, 0.06, 0.09, 0.12, 0.15}
	npers := []int{60, 120, 180, 240, 360}

	var cases []sweepCase
	for i := 0; i < 60; i++ {
		amt := amounts[rng.Intn(len(amounts))]
		rate := rates[rng.Intn(len(rates))]
		nper := npers[rng.Intn(len(npers))]
		balloon := ""
		// ~⅓ of cases add a known balloon partway through, which is exactly the
		// case where the naive "balance − (payment − interest)" walk drifted.
		if rng.Intn(3) == 0 {
			by := 1 + rng.Intn(nper/12-1)
			balloon = fmt.Sprintf(`,"balloons":[{"date":"%04d-02-01","amount":%d}]`,
				2024+by, 5000+rng.Intn(40000))
		}
		body := fmt.Sprintf(`{"amount":%g,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":%g,"perYr":12,"nPeriods":%d%s}`,
			amt, rate, nper, balloon)
		resp, code := amortCall(t, body)
		if code != 200 || resp.Error != "" || len(resp.Schedule) == 0 {
			continue
		}
		cases = append(cases, sweepCase{Amount: amt, Schedule: resp.Schedule})
	}
	if len(cases) < 30 {
		t.Fatalf("sweep produced only %d usable cases", len(cases))
	}
	casesJSON, _ := json.Marshal(cases)

	// --- Render every case with the shipped renderAmzSchedule (one Node run) ---
	harness := `
'use strict';
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDateDisplay") + `
` + extractJS(t, html, "renderAmzSchedule") + `
function makeEl() {
  return {
    _html: '', children: [], textContent: '',
    set innerHTML(v) { this._html = v; this.children = []; },
    get innerHTML() { return this._html; },
    appendChild(c) { this.children.push(c); },
  };
}
var els = {
  'amz-schedule-body': makeEl(), 'amz-schedule-head': makeEl(),
  'amz-view-note': makeEl(), 'amz-view-mode': { value: 'payment' },
};
var document = { getElementById: function (id) { return els[id] || null; },
                 createElement: function () { return makeEl(); } };
function tds(el) {
  return (el._html.match(/<td>([\s\S]*?)<\/td>/g) || []).map(function (s) { return s.replace(/<\/?td>/g, ''); });
}
var amzScheduleData;
var cases = ` + string(casesJSON) + `;
var out = cases.map(function (c) {
  amzScheduleData = { input: { amount: c.amount }, result: { schedule: c.schedule } };
  els['amz-schedule-body'] = makeEl();
  renderAmzSchedule();
  return els['amz-schedule-body'].children.map(function (tr) { return tds(tr); });
});
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, outBytes)
	}
	var rendered [][][]string
	if err := json.Unmarshal(outBytes, &rendered); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, outBytes)
	}
	if len(rendered) != len(cases) {
		t.Fatalf("rendered %d cases, expected %d", len(rendered), len(cases))
	}

	// --- Assert: every rendered Balance == engine balance; Principal == delta ---
	mismatches := 0
	for ci := range cases {
		sched := cases[ci].Schedule
		rows := rendered[ci]
		if len(rows) != len(sched) {
			t.Errorf("case %d: rendered %d rows, engine has %d", ci, len(rows), len(sched))
			continue
		}
		prev := cases[ci].Amount
		for ri := range sched {
			engBal := math.Max(0, sched[ri].Principal)
			gotBal := parseMoneyStr(rows[ri][5])
			gotPrinc := parseMoneyStr(rows[ri][4])
			wantPrinc := prev - engBal
			if math.Abs(gotBal-engBal) > 0.005 {
				if mismatches < 8 {
					t.Errorf("case %d row %d: Balance rendered %.2f, engine %.2f", ci, ri, gotBal, engBal)
				}
				mismatches++
			}
			if math.Abs(gotPrinc-wantPrinc) > 0.005 {
				if mismatches < 8 {
					t.Errorf("case %d row %d: Principal rendered %.2f, want balance-delta %.2f", ci, ri, gotPrinc, wantPrinc)
				}
				mismatches++
			}
			prev = engBal
		}
	}
	if mismatches > 0 {
		t.Errorf("frontend render diverged from engine in %d cell(s) across %d swept loans", mismatches, len(cases))
	} else {
		t.Logf("frontend render reconciles with the engine across %d swept loans (incl. balloons)", len(cases))
	}
}

// TestFrontendAmzRequestMappingSweep sweeps random typed field values through the
// shipped getAmzInput and asserts the request body it builds maps correctly —
// rate ÷ 100, money parsed from $/commas, dates → ISO, points ÷ 100, payment
// present only when entered. This guards the request-translation layer (the side
// that produced the .psn column-scramble class) across the input space.
func TestFrontendAmzRequestMappingSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping request-mapping sweep")
	}
	html := mustReadIndexHTML(t)

	pmoney := func(s string) float64 {
		f, _ := strconv.ParseFloat(strings.NewReplacer("$", "", ",", "", " ", "").Replace(s), 64)
		return f
	}
	mdyToISO := func(s string) string {
		p := strings.Split(s, "/")
		return fmt.Sprintf("%s-%02s-%02s", p[2], p[0], p[1])
	}

	rng := rand.New(rand.NewSource(424242))
	type fields map[string]string
	type expect struct {
		amount, rate, points float64
		loanISO, firstISO    string
		nPeriods, perYr      int
		basis                string
		payment              float64
		hasPayment, hasFirst bool
	}
	var cases []fields
	var exps []expect
	for i := 0; i < 50; i++ {
		amtV := float64(10000 + rng.Intn(890000))
		rateV := float64(rng.Intn(1700)+100) / 100 // 1.00–18.00
		points := []string{"0", "1", "1.5", "2.25"}[rng.Intn(4)]
		nper := []int{60, 120, 180, 240, 360}[rng.Intn(5)]
		basis := []string{"360", "365", "365/360"}[rng.Intn(3)]
		mm, dd, yy := 1+rng.Intn(12), 1+rng.Intn(28), 2024+rng.Intn(3)
		loan := fmt.Sprintf("%02d/%02d/%04d", mm, dd, yy)
		amtStr := fmt.Sprintf("$%s", commafmt(amtV))
		rateStr := strconv.FormatFloat(rateV, 'f', 4, 64)

		f := fields{
			"amz-amount": amtStr, "amz-loanDate": loan, "amz-rate": rateStr,
			"amz-nPeriods": strconv.Itoa(nper), "amz-perYr": "12",
			"amz-points": points, "amz-basis": basis,
			"amz-firstDate": "", "amz-lastDate": "", "amz-payment": "",
			"amz-moratorium": "", "amz-targetAmt": "", "amz-skipMonths": "",
		}
		e := expect{
			amount: amtV, rate: pmoney(rateStr) / 100, points: pmoney(points) / 100,
			loanISO: mdyToISO(loan), nPeriods: nper, perYr: 12, basis: basis,
		}
		if rng.Intn(2) == 0 { // optional explicit first payment date
			fm, fd := 1+rng.Intn(12), 1+rng.Intn(28)
			fdate := fmt.Sprintf("%02d/%02d/%04d", fm, fd, yy+1)
			f["amz-firstDate"] = fdate
			e.firstISO, e.hasFirst = mdyToISO(fdate), true
		}
		if rng.Intn(3) == 0 { // optional entered payment
			pv := float64(500 + rng.Intn(4000))
			f["amz-payment"] = commafmt(pv)
			e.payment, e.hasPayment = pv, true
		}
		cases = append(cases, f)
		exps = append(exps, e)
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + extractJS(t, html, "parseDate") + `
` + extractJS(t, html, "inferAmzDateFromLoan") + `
` + extractJS(t, html, "getAmzInput") + `
var FIELDS = {};
function elFor(id) { return { value: (id in FIELDS ? FIELDS[id] : ''), classList: { contains: function () { return false; } } }; }
var document = { getElementById: elFor, querySelectorAll: function () { return []; } };
var cases = ` + string(casesJSON) + `;
console.log(JSON.stringify(cases.map(function (c) { FIELDS = c; return getAmzInput(); })));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	type amzBody struct {
		PerYr     int      `json:"perYr"`
		Basis     string   `json:"basis"`
		Amount    *float64 `json:"amount"`
		Rate      *float64 `json:"rate"`
		LoanDate  string   `json:"loanDate"`
		FirstDate string   `json:"firstDate"`
		NPeriods  int      `json:"nPeriods"`
		Payment   *float64 `json:"payment"`
		Points    *float64 `json:"points"`
	}
	var results []struct {
		Body  *amzBody `json:"body"`
		Error string   `json:"error"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	approx := func(a, b float64) bool { return math.Abs(a-b) < 0.005 }
	for i := range cases {
		r := results[i]
		if r.Error != "" || r.Body == nil {
			t.Errorf("case %d: getAmzInput error %q", i, r.Error)
			continue
		}
		b, e := r.Body, exps[i]
		if b.Amount == nil || !approx(*b.Amount, e.amount) {
			t.Errorf("case %d: amount = %v, want %.2f", i, b.Amount, e.amount)
		}
		if b.Rate == nil || !approx(*b.Rate, e.rate) {
			t.Errorf("case %d: rate = %v, want %.6f (typed %s ÷100)", i, b.Rate, e.rate, cases[i]["amz-rate"])
		}
		if b.Points == nil || !approx(*b.Points, e.points) {
			t.Errorf("case %d: points = %v, want %.4f", i, b.Points, e.points)
		}
		if b.LoanDate != e.loanISO {
			t.Errorf("case %d: loanDate = %q, want %q", i, b.LoanDate, e.loanISO)
		}
		if b.NPeriods != e.nPeriods || b.PerYr != e.perYr || b.Basis != e.basis {
			t.Errorf("case %d: nPeriods/perYr/basis = %d/%d/%s, want %d/12/%s", i, b.NPeriods, b.PerYr, b.Basis, e.nPeriods, e.basis)
		}
		if e.hasFirst && b.FirstDate != e.firstISO {
			t.Errorf("case %d: firstDate = %q, want %q", i, b.FirstDate, e.firstISO)
		}
		if e.hasPayment != (b.Payment != nil) {
			t.Errorf("case %d: payment present = %v, want %v", i, b.Payment != nil, e.hasPayment)
		} else if e.hasPayment && !approx(*b.Payment, e.payment) {
			t.Errorf("case %d: payment = %v, want %.2f", i, b.Payment, e.payment)
		}
	}
	t.Logf("getAmzInput request mapping verified across %d swept inputs", len(cases))
}

// TestFrontendAmzHeadlinePaymentSweep runs the SHIPPED calcAmortization against
// real engine responses and checks the displayed top-line Payment. For a plain
// loan it's the regular payment; for a moratorium it must be the regular
// amortizing payment (not the interest-only first row); for a principal-minimum
// it must be the eventual steady payment (not the high first row). These were
// the client's recent reports.
func TestFrontendAmzHeadlinePaymentSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping headline-payment sweep")
	}
	html := mustReadIndexHTML(t)

	type echoCase struct {
		Fields   map[string]string `json:"fields"`
		Response json.RawMessage   `json:"response"`
	}
	// kind: 0 plain, 1 moratorium, 2 principal-minimum.
	type meta struct {
		kind         int
		wantHeadline float64
		firstReg     float64
	}
	baseFields := func(amt float64, ratePct float64, nper int) map[string]string {
		return map[string]string{
			"amz-amount": commafmt(amt), "amz-loanDate": "01/01/2024",
			"amz-firstDate": "02/01/2024", "amz-rate": strconv.FormatFloat(ratePct, 'f', 4, 64),
			"amz-nPeriods": strconv.Itoa(nper), "amz-perYr": "12", "amz-basis": "360",
			"amz-points": "0", "amz-payment": "", "amz-moratorium": "",
			"amz-targetAmt": "", "amz-skipMonths": "",
		}
	}
	loans := []struct {
		amt     float64
		ratePct float64
		nper    int
		kind    int
	}{
		{100000, 6, 120, 0}, {200000, 9, 180, 0}, {150000, 7.5, 240, 0},
		{100000, 9, 120, 1}, {250000, 6, 180, 1}, // moratorium
		{100000, 6, 120, 2}, {180000, 7, 240, 2}, // principal-minimum
	}
	var cases []echoCase
	var metas []meta
	for _, ln := range loans {
		f := baseFields(ln.amt, ln.ratePct, ln.nper)
		body := fmt.Sprintf(`{"amount":%g,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":%g,"perYr":12,"nPeriods":%d,"basis":"360"`,
			ln.amt, ln.ratePct/100, ln.nper)
		switch ln.kind {
		case 1: // moratorium 24 months in
			f["amz-moratorium"] = "02/01/2026"
			body += `,"moratorium":"2026-02-01"`
		case 2: // principal minimum — reachable (< amount/periods) but binding
			tgt := math.Floor(ln.amt / float64(ln.nper) * 0.9)
			f["amz-targetAmt"] = strconv.FormatFloat(tgt, 'f', 0, 64)
			body += fmt.Sprintf(`,"targetAmt":%g`, tgt)
		}
		body += `}`
		resp, code := amortCall(t, body)
		if code != 200 || resp.Error != "" || len(resp.Schedule) == 0 {
			t.Fatalf("setup loan %+v: code=%d err=%q", ln, code, resp.Error)
		}
		raw, _ := json.Marshal(resp)
		want := firstRegular(resp.Schedule).Payment
		if ln.kind != 0 {
			want = modalRegular(resp.Schedule)
		}
		cases = append(cases, echoCase{Fields: f, Response: raw})
		metas = append(metas, meta{kind: ln.kind, wantHeadline: want, firstReg: firstRegular(resp.Schedule).Payment})
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + extractJS(t, html, "parseDate") + `
` + extractJS(t, html, "inferAmzDateFromLoan") + `
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtDateDisplay") + `
` + extractJS(t, html, "getAmzInput") + `
` + extractJS(t, html, "calcAmortization") + `
var autoSilent = false, calcGeneration = 0, amzScheduleData = null, CURRENT_RESPONSE = null;
function renderAmzSchedule() {}
function updatePayoffBalance() {}
function fillDerivedPrepayStops() {}
function updateAmzAdvBadge() {}
function clearFieldErrors() {}
function setAutoCalcHint() {}
function markAmzErrorFields() {}
function clearAmzScheduleOutput() {}
function renderAdvisoryHTML() { return ''; }
function blockInvalidDates() { return false; }
async function apiPost() { return CURRENT_RESPONSE; }
function mkEl() {
  var cls = [];
  return { value: '', textContent: '', innerHTML: '',
    classList: { add: function (c) { if (cls.indexOf(c) < 0) cls.push(c); },
                 remove: function (c) { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); },
                 toggle: function (c, force) { if (force) { if (cls.indexOf(c) < 0) cls.push(c); } else { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); } },
                 contains: function (c) { return cls.indexOf(c) >= 0; } } };
}
var ELS = {};
function getEl(id) { if (!(id in ELS)) ELS[id] = mkEl(); return ELS[id]; }
var document = { getElementById: getEl, querySelector: function () { return null; },
                 querySelectorAll: function () { return []; }, createElement: function () { return mkEl(); } };
var cases = ` + string(casesJSON) + `;
(async function () {
  var out = [];
  for (var k = 0; k < cases.length; k++) {
    ELS = {};
    var c = cases[k];
    for (var id in c.fields) { getEl(id).value = c.fields[id]; }
    CURRENT_RESPONSE = c.response;
    await calcAmortization();
    out.push({ payment: getEl('amz-payment').value, firstDate: getEl('amz-firstDate').value });
  }
  console.log(JSON.stringify(out));
})();
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var results []struct {
		Payment   string `json:"payment"`
		FirstDate string `json:"firstDate"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	kindName := []string{"plain", "moratorium", "principal-minimum"}
	for i := range cases {
		got := parseMoneyStr(results[i].Payment)
		m := metas[i]
		if math.Abs(got-m.wantHeadline) > 0.02 {
			t.Errorf("case %d (%s): headline payment displayed %.2f, want %.2f",
				i, kindName[m.kind], got, m.wantHeadline)
		}
		if m.kind != 0 && math.Abs(got-m.firstReg) < 0.02 {
			t.Errorf("case %d (%s): headline %.2f equals the first-row payment %.2f — should show the steady payment instead",
				i, kindName[m.kind], got, m.firstReg)
		}
	}
	t.Logf("headline payment verified across %d swept loans (plain / moratorium / principal-minimum)", len(cases))
}

// TestFrontendPVRequestMappingSweep sweeps random Present Value inputs through the
// shipped getPVInput and asserts the request body maps correctly — especially the
// rate-type conversion (True / Loan / Yield → the continuous True rate the engine
// expects), plus the as-of date and lump-sum mapping. Extends the differential
// harness to the PV screen.
func TestFrontendPVRequestMappingSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping PV request-mapping sweep")
	}
	html := mustReadIndexHTML(t)

	mdyToISO := func(s string) string {
		p := strings.Split(s, "/")
		return fmt.Sprintf("%s-%02s-%02s", p[2], p[0], p[1])
	}
	// Mirror pvRateToTrue(pct,type)/100 independently (so a conversion bug fails).
	expTrue := func(pct float64, typ string) float64 {
		switch typ {
		case "yield":
			return math.Log(1 + pct/100)
		case "loan":
			return 12 * math.Log(1+(pct/100)/12)
		default: // "true"
			return pct / 100
		}
	}

	rng := rand.New(rand.NewSource(99887766))
	type pcase struct {
		AsOf       string `json:"asOf"`
		RateType   string `json:"rateType"`
		Rate       string `json:"rate"`
		LumpDate   string `json:"lumpDate"`
		LumpAmount string `json:"lumpAmount"`
	}
	rateTypes := []string{"true", "loan", "yield"}
	var cases []pcase
	type exp struct {
		rate  float64
		asOf  string
		lDate string
		lAmt  float64
	}
	var exps []exp
	for i := 0; i < 45; i++ {
		typ := rateTypes[rng.Intn(3)]
		pct := float64(rng.Intn(1400)+100) / 100 // 1.00–15.00
		am, ad, ay := 1+rng.Intn(12), 1+rng.Intn(28), 2020+rng.Intn(6)
		asof := fmt.Sprintf("%02d/%02d/%04d", am, ad, ay)
		lm, ld, ly := 1+rng.Intn(12), 1+rng.Intn(28), ay+1+rng.Intn(20)
		lump := fmt.Sprintf("%02d/%02d/%04d", lm, ld, ly)
		amt := float64(1000 + rng.Intn(500000))
		cases = append(cases, pcase{
			AsOf: asof, RateType: typ, Rate: strconv.FormatFloat(pct, 'f', 4, 64),
			LumpDate: lump, LumpAmount: commafmt(amt),
		})
		exps = append(exps, exp{rate: expTrue(pct, typ), asOf: mdyToISO(asof), lDate: mdyToISO(lump), lAmt: amt})
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseDate") + `
` + extractJS(t, html, "pvRateToTrue") + `
` + extractJS(t, html, "getPVInput") + `
var pvLumpBlanks = [], pvPerBlanks = [], pvLsCount = 1, pvPerCount = 0;
function readPVRateSchedule() { return []; }
function getActuarialConfig() { return null; }
function mkEl(v) { return { value: (v || ''), textContent: '', classList: { add: function () {}, remove: function () {}, toggle: function () {}, contains: function () { return false; } } }; }
var ELS = {}, SEL = {};
function gid(id) { if (!(id in ELS)) ELS[id] = mkEl(''); return ELS[id]; }
var document = { getElementById: gid, querySelector: function (s) { return (s in SEL) ? SEL[s] : null; }, querySelectorAll: function () { return []; } };
var cases = ` + string(casesJSON) + `;
var out = cases.map(function (c) {
  ELS = {}; SEL = {};
  gid('pv-asOfDate').value = c.asOf;
  gid('pv-rateType').value = c.rateType;
  gid('pv-rate').value = c.rate;
  gid('pv-total').value = '';
  gid('actu-pod').value = '';
  gid('set-colaMonth').value = 'anniversary';
  SEL['input[data-ls="0"][data-f="date"]'] = mkEl(c.lumpDate);
  SEL['input[data-ls="0"][data-f="amount"]'] = mkEl(c.lumpAmount);
  SEL['input[data-ls="0"][data-f="value"]'] = mkEl('');
  SEL['select[data-ls="0"][data-f="act"]'] = mkEl('N');
  return getPVInput();
});
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	type lump struct {
		Act    string   `json:"act"`
		Date   string   `json:"date"`
		Amount *float64 `json:"amount"`
	}
	var results []struct {
		AsOfDate string   `json:"asOfDate"`
		Rate     *float64 `json:"rate"`
		LumpSums []lump   `json:"lumpSums"`
		Error    string   `json:"error"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for i := range cases {
		r := results[i]
		if r.Error != "" {
			t.Errorf("case %d (%s): getPVInput error %q", i, cases[i].RateType, r.Error)
			continue
		}
		e := exps[i]
		if r.Rate == nil || math.Abs(*r.Rate-e.rate) > 1e-9 {
			t.Errorf("case %d (%s %s): rate = %v, want true-rate %.10f", i, cases[i].RateType, cases[i].Rate, r.Rate, e.rate)
		}
		if r.AsOfDate != e.asOf {
			t.Errorf("case %d: asOfDate = %q, want %q", i, r.AsOfDate, e.asOf)
		}
		if len(r.LumpSums) != 1 {
			t.Errorf("case %d: %d lump sums, want 1", i, len(r.LumpSums))
			continue
		}
		ls := r.LumpSums[0]
		if ls.Date != e.lDate || ls.Amount == nil || math.Abs(*ls.Amount-e.lAmt) > 0.005 || ls.Act != "N" {
			t.Errorf("case %d: lump = {act:%s date:%s amount:%v}, want {N %s %.2f}", i, ls.Act, ls.Date, ls.Amount, e.lDate, e.lAmt)
		}
	}
	t.Logf("getPVInput request mapping (incl. True/Loan/Yield rate conversion) verified across %d swept inputs", len(cases))
}

// TestFrontendMtgRequestMappingSweep sweeps random mortgage-grid field sets through
// the shipped getMtgRowData and asserts the request body maps correctly: rate /
// pctDown / points ÷ 100, money parsed, ints parsed, and only cells marked as
// user input are sent. Rounds out the request-translation coverage to all three
// screens.
func TestFrontendMtgRequestMappingSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping mortgage request-mapping sweep")
	}
	html := mustReadIndexHTML(t)

	mtgFieldsRe := regexp.MustCompile(`(?s)const MTG_FIELDS = \[.*?\];`)
	mtgFields := mtgFieldsRe.FindString(html)
	if mtgFields == "" {
		t.Fatal("MTG_FIELDS not found")
	}
	type ftype int
	const (
		money ftype = iota
		rate
		intf
	)
	types := map[string]ftype{
		"price": money, "cash": money, "financed": money, "tax": money, "monthly": money, "balloonAmount": money,
		"points": rate, "pctDown": rate, "rate": rate,
		"years": intf, "balloonYears": intf,
	}
	order := []string{"price", "points", "pctDown", "cash", "financed", "years", "rate", "tax", "monthly", "balloonYears", "balloonAmount"}

	rng := rand.New(rand.NewSource(7654321))
	type mcase struct {
		Status map[string]string `json:"status"`
		Cells  map[string]string `json:"cells"`
	}
	var cases []mcase
	var exps []map[string]float64
	for i := 0; i < 45; i++ {
		st := map[string]string{}
		cells := map[string]string{}
		exp := map[string]float64{}
		for _, f := range order {
			if rng.Intn(10) < 6 { // ~60% of fields are user input
				st[f] = "input"
				switch types[f] {
				case money:
					v := float64(1000 + rng.Intn(900000))
					if rng.Intn(2) == 0 {
						cells[f] = "$" + commafmt(v)
					} else {
						cells[f] = commafmt(v)
					}
					exp[f] = v
				case rate:
					v := float64(rng.Intn(1500)+50) / 100
					cells[f] = strconv.FormatFloat(v, 'f', 4, 64)
					exp[f] = v / 100
				case intf:
					v := 1 + rng.Intn(40)
					cells[f] = strconv.Itoa(v)
					exp[f] = float64(v)
				}
			}
		}
		cases = append(cases, mcase{Status: st, Cells: cells})
		exps = append(exps, exp)
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + mtgFields + `
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "getMtgRowData") + `
var mtgStatus = [{}];
var SEL = {};
function mkEl(v) { return { value: (v || '') }; }
function selFor(f) { return '#mtg-body input[data-row="0"][data-field="' + f + '"]'; }
var document = { querySelector: function (s) { return (s in SEL) ? SEL[s] : null; } };
var cases = ` + string(casesJSON) + `;
var out = cases.map(function (c) {
  mtgStatus = [c.status];
  SEL = {};
  for (var f in c.cells) { SEL[selFor(f)] = mkEl(c.cells[f]); }
  return getMtgRowData(0);
});
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var bodies []map[string]float64
	if err := json.Unmarshal(out, &bodies); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for i := range cases {
		got, want := bodies[i], exps[i]
		for f, wv := range want {
			gv, ok := got[f]
			if !ok || math.Abs(gv-wv) > 1e-9 {
				t.Errorf("case %d field %s: body = %v (present=%v), want %.10f", i, f, gv, ok, wv)
			}
		}
		for f := range got {
			if _, ok := want[f]; !ok {
				t.Errorf("case %d: body has unexpected field %s (not marked input)", i, f)
			}
		}
	}
	t.Logf("getMtgRowData request mapping (rate/pctDown/points ÷100) verified across %d swept inputs", len(cases))
}

// TestFrontendAmzRecalcIdempotentSweep checks that a recalculation is idempotent:
// after calcAmortization populates the computed (green) cells, calling getAmzInput
// again must build the SAME request — green cells read as blank (so the same
// field-presence dispatch repeats) and user cells survive. A bug here (a solved
// value fed back as a hard input, or a user field wrongly greened) would shift the
// result on every recalc.
func TestFrontendAmzRecalcIdempotentSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping idempotency sweep")
	}
	html := mustReadIndexHTML(t)

	type idemCase struct {
		Fields   map[string]string `json:"fields"`
		Response json.RawMessage   `json:"response"`
	}
	base := func(amt, ratePct float64, nper int) map[string]string {
		return map[string]string{
			"amz-amount": commafmt(amt), "amz-loanDate": "01/01/2024", "amz-firstDate": "02/01/2024",
			"amz-rate": strconv.FormatFloat(ratePct, 'f', 4, 64), "amz-nPeriods": strconv.Itoa(nper),
			"amz-perYr": "12", "amz-basis": "360", "amz-points": "0", "amz-payment": "",
			"amz-moratorium": "", "amz-targetAmt": "", "amz-skipMonths": "",
		}
	}
	type spec struct {
		amt, ratePct float64
		nper, kind   int // 0 plain (blank pmt), 1 moratorium, 2 amount-solved (blank amount)
	}
	specs := []spec{
		{100000, 6, 120, 0}, {200000, 9, 180, 0},
		{100000, 9, 120, 1},
		{0, 6, 120, 2}, // amount blank → engine solves it from payment
	}
	var cases []idemCase
	for _, s := range specs {
		f := base(s.amt, s.ratePct, s.nper)
		body := fmt.Sprintf(`{"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":%g,"perYr":12,"nPeriods":%d,"basis":"360"`, s.ratePct/100, s.nper)
		switch s.kind {
		case 0:
			body = fmt.Sprintf(`{"amount":%g,`, s.amt) + body[1:]
		case 1:
			body = fmt.Sprintf(`{"amount":%g,`, s.amt) + body[1:] + `,"moratorium":"2026-02-01"`
			f["amz-moratorium"] = "02/01/2026"
		case 2: // amount-solve: blank amount, supply a payment
			f["amz-amount"] = ""
			f["amz-payment"] = "1110.21"
			body += `,"payment":1110.21`
		}
		body += `}`
		resp, code := amortCall(t, body)
		if code != 200 || resp.Error != "" || len(resp.Schedule) == 0 {
			t.Fatalf("setup %+v: code=%d err=%q", s, code, resp.Error)
		}
		raw, _ := json.Marshal(resp)
		cases = append(cases, idemCase{Fields: f, Response: raw})
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + extractJS(t, html, "parseDate") + `
` + extractJS(t, html, "inferAmzDateFromLoan") + `
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtDateDisplay") + `
` + extractJS(t, html, "getAmzInput") + `
` + extractJS(t, html, "calcAmortization") + `
var autoSilent = false, calcGeneration = 0, amzScheduleData = null, CURRENT_RESPONSE = null;
function renderAmzSchedule() {}
function updatePayoffBalance() {}
function fillDerivedPrepayStops() {}
function updateAmzAdvBadge() {}
function clearFieldErrors() {}
function setAutoCalcHint() {}
function markAmzErrorFields() {}
function clearAmzScheduleOutput() {}
function renderAdvisoryHTML() { return ''; }
function blockInvalidDates() { return false; }
async function apiPost() { return CURRENT_RESPONSE; }
function mkEl() {
  var cls = [];
  return { value: '', textContent: '', innerHTML: '',
    classList: { add: function (c) { if (cls.indexOf(c) < 0) cls.push(c); },
                 remove: function (c) { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); },
                 toggle: function (c, f) { if (f) { if (cls.indexOf(c) < 0) cls.push(c); } else { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); } },
                 contains: function (c) { return cls.indexOf(c) >= 0; } } };
}
var ELS = {};
function getEl(id) { if (!(id in ELS)) ELS[id] = mkEl(); return ELS[id]; }
var document = { getElementById: getEl, querySelector: function () { return null; },
                 querySelectorAll: function () { return []; }, createElement: function () { return mkEl(); } };
var cases = ` + string(casesJSON) + `;
(async function () {
  var out = [];
  for (var k = 0; k < cases.length; k++) {
    ELS = {};
    var c = cases[k];
    for (var id in c.fields) { getEl(id).value = c.fields[id]; }
    var b1 = getAmzInput().body;
    CURRENT_RESPONSE = c.response;
    await calcAmortization();
    var b2 = getAmzInput().body;
    out.push({ b1: JSON.stringify(b1), b2: JSON.stringify(b2) });
  }
  console.log(JSON.stringify(out));
})();
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var results []struct {
		B1 string `json:"b1"`
		B2 string `json:"b2"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for i, r := range results {
		if r.B1 != r.B2 {
			t.Errorf("case %d: recalc not idempotent\n  before: %s\n  after:  %s", i, r.B1, r.B2)
		}
	}
	t.Logf("recalc is idempotent (green cells read as blank) across %d swept loans", len(results))
}

// TestFrontendClearPVStateSweep fills every field clearPV touches with junk
// (lump/periodic rows, contingency dropdowns, actuarial section incl. POD, the
// variable-rate schedule) across a few row counts, runs the shipped clearPV, and
// asserts a full reset — perYr→12, COLA→0, everything else blank, dropdowns→None.
// This guards the leftover-state class (POD/COLA/perYr) that silently corrupted
// totals when a worksheet was reused.
func TestFrontendClearPVStateSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping clear-state sweep")
	}
	html := mustReadIndexHTML(t)

	type combo struct {
		LS  int `json:"ls"`
		Per int `json:"per"`
	}
	combos := []combo{{1, 1}, {2, 3}, {3, 2}}
	combosJSON, _ := json.Marshal(combos)

	harness := `
'use strict';
` + extractJS(t, html, "clearPV") + `
class Event { constructor() {} }
var pvLsCount = 0, pvPerCount = 0;
function refreshLifeContingencyOptions() {}
function setAutoCalcHint() {}
function updatePVActiveSummary() {}
function inp(v) { var cls = ['cell-output']; return { value: v, textContent: v, disabled: false, selectedIndex: 3, dispatchEvent: function () {},
  classList: { add: function (c) { if (cls.indexOf(c) < 0) cls.push(c); }, remove: function (c) { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); }, contains: function (c) { return cls.indexOf(c) >= 0; } } }; }
var combos = ` + string(combosJSON) + `;
var out = combos.map(function (cb) {
  pvLsCount = cb.ls; pvPerCount = cb.per;
  var ELS = {}, SEL = {};
  ['pv-asOfDate','pv-rate','pv-total','actu-dob1','actu-dob2','actu-now','actu-pod','actu-csv1','actu-csv2','pv-pod-result','pv-error','actu-table1','actu-table2'].forEach(function (id) { ELS[id] = inp('junk'); });
  for (var i = 0; i < cb.ls; i++) { ['date','amount','value'].forEach(function (f) { SEL['input[data-ls="'+i+'"][data-f="'+f+'"]'] = inp('junk'); }); }
  for (var j = 0; j < cb.per; j++) { ['from','to','perYr','amount','cola','value'].forEach(function (f) { SEL['input[data-per="'+j+'"][data-f="'+f+'"]'] = inp('junk'); }); }
  var actSelects = [inp('B'), inp('L')];
  var rateInputs = [inp('5'), inp('6.5')];
  var document = {
    getElementById: function (id) { return ELS[id] || null; },
    querySelector: function (s) { return (s in SEL) ? SEL[s] : null; },
    querySelectorAll: function (s) {
      if (s.indexOf('select[data-f="act"]') >= 0) return actSelects;
      if (s.indexOf('#pv-rateSched') >= 0) return rateInputs;
      return [];
    },
  };
  globalThis.document = document;
  clearPV();
  function rows(prefix, n, fs) { var r = []; for (var k = 0; k < n; k++) { var o = {}; fs.forEach(function (f) { o[f] = SEL[prefix+k+'"][data-f="'+f+'"]'].value; }); r.push(o); } return r; }
  return {
    asOf: ELS['pv-asOfDate'].value, rate: ELS['pv-rate'].value, total: ELS['pv-total'].value,
    totalGreen: ELS['pv-total'].classList.contains('cell-output'),
    ls: rows('input[data-ls="', cb.ls, ['date','amount','value']),
    per: rows('input[data-per="', cb.per, ['from','to','perYr','amount','cola','value']),
    act: actSelects.map(function (s) { return s.value; }),
    rateSched: rateInputs.map(function (s) { return s.value; }),
    actuPod: ELS['actu-pod'].value, actuDob1: ELS['actu-dob1'].value, actuCsv1: ELS['actu-csv1'].value,
    actuNow: ELS['actu-now'].value, podResult: ELS['pv-pod-result'].textContent,
    table1: ELS['actu-table1'].selectedIndex, table2: ELS['actu-table2'].selectedIndex,
  };
});
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	type perRow struct {
		From, To, PerYr, Amount, Cola, Value string
	}
	type lsRow struct{ Date, Amount, Value string }
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	getStr := func(m map[string]json.RawMessage, k string) string {
		var s string
		_ = json.Unmarshal(m[k], &s)
		return s
	}
	for ci, m := range raw {
		for _, k := range []string{"asOf", "rate", "total", "actuPod", "actuDob1", "actuCsv1", "actuNow", "podResult"} {
			if v := getStr(m, k); v != "" {
				t.Errorf("combo %d: %s = %q after clear, want empty", ci, k, v)
			}
		}
		var totalGreen bool
		_ = json.Unmarshal(m["totalGreen"], &totalGreen)
		if totalGreen {
			t.Errorf("combo %d: pv-total still has cell-output (green) after clear", ci)
		}
		var ls []lsRow
		_ = json.Unmarshal(m["ls"], &ls)
		for i, r := range ls {
			if r.Date != "" || r.Amount != "" || r.Value != "" {
				t.Errorf("combo %d ls row %d not cleared: %+v", ci, i, r)
			}
		}
		var per []perRow
		_ = json.Unmarshal(m["per"], &per)
		for i, r := range per {
			if r.From != "" || r.To != "" || r.Amount != "" || r.Value != "" {
				t.Errorf("combo %d per row %d not cleared: %+v", ci, i, r)
			}
			if r.PerYr != "12" {
				t.Errorf("combo %d per row %d: perYr = %q, want \"12\"", ci, i, r.PerYr)
			}
			if r.Cola != "0" {
				t.Errorf("combo %d per row %d: cola = %q, want \"0\"", ci, i, r.Cola)
			}
		}
		var act, rateSched []string
		_ = json.Unmarshal(m["act"], &act)
		_ = json.Unmarshal(m["rateSched"], &rateSched)
		for i, v := range act {
			if v != "N" {
				t.Errorf("combo %d: act dropdown %d = %q, want \"N\"", ci, i, v)
			}
		}
		for i, v := range rateSched {
			if v != "" {
				t.Errorf("combo %d: rate-schedule input %d = %q, want empty", ci, i, v)
			}
		}
		var t1, t2 int
		_ = json.Unmarshal(m["table1"], &t1)
		_ = json.Unmarshal(m["table2"], &t2)
		if t1 != 0 || t2 != 0 {
			t.Errorf("combo %d: actuarial table selectedIndex = %d/%d, want 0/0", ci, t1, t2)
		}
	}
	t.Logf("clearPV fully resets state across %d row-count combos", len(raw))
}

// TestFrontendClearAmzStateSweep fills everything clearAmortization touches with
// junk (basic fields, payoff tool, advanced-option rows, moratorium/target/skip)
// and asserts a full reset — perYr→12, points→0, computed cells de-greened,
// summary hidden, advanced options blank, schedule cleared. Guards the
// leftover-advanced-options class (a stray target/moratorium/skip silently
// altering the next loan).
func TestFrontendClearAmzStateSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping clearAmortization sweep")
	}
	html := mustReadIndexHTML(t)

	harness := `
'use strict';
` + extractJS(t, html, "clearAmortization") + `
var AMZ_INPUT_CELLS = [], amzScheduleData = {}, payoffInputField = 'bal';
function confirm() { return true; }
function setAutoCalcHint() {}
function updateAmzAdvBadge() {}
function el(v) {
  var cls = ['cell-output'];
  return { value: v, textContent: v, innerHTML: 'x',
    classList: { add: function (c) { if (cls.indexOf(c) < 0) cls.push(c); },
                 remove: function (c) { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); },
                 contains: function (c) { return cls.indexOf(c) >= 0; } } };
}
var combos = [3, 6];
var out = combos.map(function (nAdv) {
  amzScheduleData = {}; payoffInputField = 'bal';
  var ELS = {};
  ['amz-amount','amz-loanDate','amz-rate','amz-firstDate','amz-nPeriods','amz-perYr','amz-payment','amz-points','amz-apr','amz-lastDate','amz-payoff-date','amz-payoff-bal','amz-schedule-body','amz-moratorium','amz-targetAmt','amz-skipMonths','amz-summary','amz-error'].forEach(function (id) { ELS[id] = el('junk'); });
  var advInputs = []; for (var i = 0; i < nAdv; i++) advInputs.push(el('junk'));
  globalThis.document = { getElementById: function (id) { return ELS[id] || null; },
    querySelectorAll: function (s) { return s.indexOf('amz-prepay-body') >= 0 ? advInputs : []; } };
  clearAmortization();
  return {
    amount: ELS['amz-amount'].value, loanDate: ELS['amz-loanDate'].value, rate: ELS['amz-rate'].value,
    firstDate: ELS['amz-firstDate'].value, nPeriods: ELS['amz-nPeriods'].value, payment: ELS['amz-payment'].value,
    lastDate: ELS['amz-lastDate'].value, apr: ELS['amz-apr'].value,
    perYr: ELS['amz-perYr'].value, points: ELS['amz-points'].value,
    moratorium: ELS['amz-moratorium'].value, targetAmt: ELS['amz-targetAmt'].value, skipMonths: ELS['amz-skipMonths'].value,
    payoffDate: ELS['amz-payoff-date'].value, payoffBal: ELS['amz-payoff-bal'].value,
    paymentGreen: ELS['amz-payment'].classList.contains('cell-output'), aprGreen: ELS['amz-apr'].classList.contains('cell-output'),
    summaryHidden: ELS['amz-summary'].classList.contains('hidden'),
    error: ELS['amz-error'].textContent, payoffInputField: payoffInputField,
    schedBody: ELS['amz-schedule-body'].innerHTML, adv: advInputs.map(function (x) { return x.value; }),
  };
});
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var states []struct {
		Amount, LoanDate, Rate, FirstDate, NPeriods, Payment, LastDate, APR string
		PerYr, Points                                                       string
		Moratorium, TargetAmt, SkipMonths                                   string
		PayoffDate, PayoffBal                                               string
		PaymentGreen, AprGreen, SummaryHidden                               bool
		Error, PayoffInputField, SchedBody                                  string
		Adv                                                                 []string
	}
	if err := json.Unmarshal(out, &states); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for ci, s := range states {
		blanks := map[string]string{
			"amount": s.Amount, "loanDate": s.LoanDate, "rate": s.Rate, "firstDate": s.FirstDate,
			"nPeriods": s.NPeriods, "payment": s.Payment, "lastDate": s.LastDate, "apr": s.APR,
			"moratorium": s.Moratorium, "targetAmt": s.TargetAmt, "skipMonths": s.SkipMonths,
			"payoffDate": s.PayoffDate, "payoffBal": s.PayoffBal, "error": s.Error, "schedBody": s.SchedBody,
		}
		for k, v := range blanks {
			if v != "" {
				t.Errorf("combo %d: %s = %q after clear, want empty", ci, k, v)
			}
		}
		if s.PerYr != "12" {
			t.Errorf("combo %d: perYr = %q, want \"12\"", ci, s.PerYr)
		}
		if s.Points != "0" {
			t.Errorf("combo %d: points = %q, want \"0\"", ci, s.Points)
		}
		if s.PaymentGreen || s.AprGreen {
			t.Errorf("combo %d: computed cells still green (payment=%v apr=%v)", ci, s.PaymentGreen, s.AprGreen)
		}
		if !s.SummaryHidden {
			t.Errorf("combo %d: summary not hidden after clear", ci)
		}
		if s.PayoffInputField != "date" {
			t.Errorf("combo %d: payoffInputField = %q, want \"date\"", ci, s.PayoffInputField)
		}
		for i, v := range s.Adv {
			if v != "" {
				t.Errorf("combo %d: advanced-option input %d = %q, want empty", ci, i, v)
			}
		}
	}
	t.Logf("clearAmortization fully resets state (incl. advanced options) across %d combos", len(states))
}

// TestFrontendPVValueEchoSweep runs the real calcPV against engine responses and
// asserts the displayed Value cells reconcile with the engine: sumValue → pv-total,
// each lumpSums[i].value → its row's Value cell, each periodics[i].value → its
// row's Value cell. The PV analog of the amortization render/headline sweeps —
// closes the PV display loop.
func TestFrontendPVValueEchoSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping PV value-echo sweep")
	}
	html := mustReadIndexHTML(t)

	type pvEchoCase struct {
		Fields    map[string]string   `json:"fields"`
		Lumps     []map[string]string `json:"lumps"`
		Periodics []map[string]string `json:"periodics"`
		LsCount   int                 `json:"lsCount"`
		PerCount  int                 `json:"perCount"`
		Response  json.RawMessage     `json:"response"`
	}
	rng := rand.New(rand.NewSource(31415926))
	var cases []pvEchoCase
	type want struct {
		sum   float64
		lumps []float64
		pers  []float64
	}
	var wants []want
	for i := 0; i < 8; i++ {
		pct := float64([]int{4, 6, 8, 10}[rng.Intn(4)])
		nLump := 1 + rng.Intn(2)
		nPer := rng.Intn(2)
		// Build request + DOM rows.
		var lreq, lrows []string
		var domLumps []map[string]string
		for j := 0; j < nLump; j++ {
			yr := 2026 + rng.Intn(15)
			date := fmt.Sprintf("%04d-01-01", yr)
			amt := float64(5000 + rng.Intn(95000))
			lreq = append(lreq, fmt.Sprintf(`{"date":"%s","amount":%g,"act":"N"}`, date, amt))
			_ = lrows
			domLumps = append(domLumps, map[string]string{"date": date, "amount": commafmt(amt)})
		}
		var preq []string
		var domPers []map[string]string
		for j := 0; j < nPer; j++ {
			amt := float64(500 + rng.Intn(3000))
			preq = append(preq, fmt.Sprintf(`{"fromDate":"2024-02-01","toDate":"2034-01-01","perYr":12,"amount":%g,"cola":0,"act":"N"}`, amt))
			domPers = append(domPers, map[string]string{"from": "2024-02-01", "to": "2034-01-01", "perYr": "12", "amount": commafmt(amt), "cola": "0"})
		}
		body := fmt.Sprintf(`{"asOfDate":"2024-01-01","rate":%g,"lumpSums":[%s]`, pct/100, strings.Join(lreq, ","))
		if len(preq) > 0 {
			body += `,"periodics":[` + strings.Join(preq, ",") + `]`
		}
		body += `}`
		resp, code := pvCall(t, body)
		if code != 200 || resp.Error != "" {
			t.Fatalf("setup PV case %d: code=%d err=%q", i, code, resp.Error)
		}
		raw, _ := json.Marshal(resp)
		w := want{sum: resp.SumValue}
		for _, ls := range resp.LumpSums {
			w.lumps = append(w.lumps, ls.Value)
		}
		for _, pp := range resp.Periodics {
			w.pers = append(w.pers, pp.Value)
		}
		cases = append(cases, pvEchoCase{
			Fields: map[string]string{
				"pv-asOfDate": "2024-01-01", "pv-rateType": "true",
				"pv-rate": strconv.FormatFloat(pct, 'f', 4, 64), "pv-total": "",
				"actu-pod": "", "set-colaMonth": "anniversary",
			},
			Lumps: domLumps, Periodics: domPers, LsCount: nLump, PerCount: nPer, Response: raw,
		})
		wants = append(wants, w)
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + extractJS(t, html, "parseDate") + `
` + extractJS(t, html, "pvRateToTrue") + `
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "getPVInput") + `
` + extractJS(t, html, "calcPV") + `
var autoSilent = false, calcGeneration = 0, pvLumpBlanks = [], pvPerBlanks = [], pvLsCount = 0, pvPerCount = 0, CURRENT_RESPONSE = null;
function clearFieldErrors() {}
function setAutoCalcHint() {}
function blockInvalidDates() { return false; }
function defaultPVReferenceDate() {}
function pvContingencyConfigError() { return null; }
function markPVErrorFields() {}
function renderAdvisoryHTML() { return ''; }
function updatePVActiveSummary() {}
function readPVRateSchedule() { return []; }
function getActuarialConfig() { return null; }
function pvTrueToType() { return 0; }
async function apiPost() { return CURRENT_RESPONSE; }
function mkEl(v) { var cls = []; return { value: (v || ''), textContent: '', innerHTML: '',
  classList: { add: function (c) { if (cls.indexOf(c) < 0) cls.push(c); }, remove: function (c) { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); }, contains: function (c) { return cls.indexOf(c) >= 0; } } }; }
var ELS = {}, SEL = {};
var document = { getElementById: function (id) { if (!(id in ELS)) ELS[id] = mkEl(''); return ELS[id]; },
  querySelector: function (s) { return (s in SEL) ? SEL[s] : null; }, querySelectorAll: function () { return []; } };
var cases = ` + string(casesJSON) + `;
(async function () {
  var out = [];
  for (var k = 0; k < cases.length; k++) {
    var c = cases[k];
    var lumps = c.lumps || [], pers = c.periodics || [];
    ELS = {}; SEL = {};
    for (var id in c.fields) { ELS[id] = mkEl(c.fields[id]); }
    pvLsCount = c.lsCount; pvPerCount = c.perCount;
    for (var i = 0; i < lumps.length; i++) {
      SEL['input[data-ls="' + i + '"][data-f="date"]'] = mkEl(lumps[i].date);
      SEL['input[data-ls="' + i + '"][data-f="amount"]'] = mkEl(lumps[i].amount);
      SEL['input[data-ls="' + i + '"][data-f="value"]'] = mkEl('');
      SEL['select[data-ls="' + i + '"][data-f="act"]'] = mkEl('N');
    }
    for (var j = 0; j < pers.length; j++) {
      SEL['input[data-per="' + j + '"][data-f="from"]'] = mkEl(pers[j].from);
      SEL['input[data-per="' + j + '"][data-f="to"]'] = mkEl(pers[j].to);
      SEL['input[data-per="' + j + '"][data-f="perYr"]'] = mkEl(pers[j].perYr);
      SEL['input[data-per="' + j + '"][data-f="amount"]'] = mkEl(pers[j].amount);
      SEL['input[data-per="' + j + '"][data-f="cola"]'] = mkEl(pers[j].cola);
      SEL['input[data-per="' + j + '"][data-f="value"]'] = mkEl('');
      SEL['select[data-per="' + j + '"][data-f="act"]'] = mkEl('N');
    }
    CURRENT_RESPONSE = c.response;
    await calcPV();
    var lv = [], pv = [];
    for (var i = 0; i < lumps.length; i++) lv.push(SEL['input[data-ls="' + i + '"][data-f="value"]'].value);
    for (var j = 0; j < pers.length; j++) pv.push(SEL['input[data-per="' + j + '"][data-f="value"]'].value);
    out.push({ total: ELS['pv-total'].value, lumpVals: lv, perVals: pv });
  }
  console.log(JSON.stringify(out));
})();
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var results []struct {
		Total    string   `json:"total"`
		LumpVals []string `json:"lumpVals"`
		PerVals  []string `json:"perVals"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for i := range cases {
		r, w := results[i], wants[i]
		if math.Abs(parseMoneyStr(r.Total)-w.sum) > 0.02 {
			t.Errorf("case %d: pv-total displayed %.2f, engine sumValue %.2f", i, parseMoneyStr(r.Total), w.sum)
		}
		for j, wv := range w.lumps {
			if j >= len(r.LumpVals) || math.Abs(parseMoneyStr(r.LumpVals[j])-wv) > 0.02 {
				t.Errorf("case %d lump %d: Value displayed %q, engine %.2f", i, j, safeIdx(r.LumpVals, j), wv)
			}
		}
		for j, wv := range w.pers {
			if j >= len(r.PerVals) || math.Abs(parseMoneyStr(r.PerVals[j])-wv) > 0.02 {
				t.Errorf("case %d periodic %d: Value displayed %q, engine %.2f", i, j, safeIdx(r.PerVals, j), wv)
			}
		}
	}
	t.Logf("PV value cells reconcile with the engine across %d swept worksheets", len(cases))
}

func safeIdx(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "<missing>"
}

// commafmt renders a float with thousands separators and two decimals.
func commafmt(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	dot := strings.IndexByte(s, '.')
	intPart, frac := s[:dot], s[dot:]
	var b strings.Builder
	n := len(intPart)
	for i, c := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String() + frac
}

// mustReadIndexHTML reads the shipped frontend or fails the test.
func mustReadIndexHTML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../cmd/persense/static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	return string(b)
}
