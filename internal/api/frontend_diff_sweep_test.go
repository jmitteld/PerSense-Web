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
	"time"
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
	var sawPrepayRows, sawAdjust int
	for i := 0; i < 90; i++ {
		amt := amounts[rng.Intn(len(amounts))]
		rate := rates[rng.Intn(len(rates))]
		nper := npers[rng.Intn(len(npers))]
		var opts []string
		// ~⅓: a known balloon partway through — the case where the naive
		// "balance − (payment − interest)" walk drifted.
		if rng.Intn(3) == 0 {
			by := 1 + rng.Intn(nper/12-1)
			opts = append(opts, fmt.Sprintf(`"balloons":[{"date":"%04d-02-01","amount":%d}]`,
				2024+by, 5000+rng.Intn(40000)))
		}
		// ~⅓: a prepayment series starting MID-MONTH (day 15) so it lands OFF the
		// monthly payment grid — the engine emits off-cycle dated rows for those,
		// a distinct render path (the draining block) not otherwise swept.
		if rng.Intn(3) == 0 {
			sy := 2024 + 1 + rng.Intn(maxInt(1, nper/12-2))
			opts = append(opts, fmt.Sprintf(`"prepayments":[{"startDate":"%04d-05-15","perYr":12,"amount":%d,"nPmts":%d}]`,
				sy, 100+rng.Intn(900), 3+rng.Intn(12)))
		}
		// ~¼: a rate adjustment (ARM) on a payment date — re-amortizes mid-schedule.
		if rng.Intn(4) == 0 {
			ay := 2024 + 1 + rng.Intn(maxInt(1, nper/12-2))
			opts = append(opts, fmt.Sprintf(`"adjustments":[{"date":"%04d-02-01","rate":%g}]`,
				ay, rate+0.01))
		}
		extra := ""
		if len(opts) > 0 {
			extra = "," + strings.Join(opts, ",")
		}
		body := fmt.Sprintf(`{"amount":%g,"loanDate":"2024-01-01","firstDate":"2024-02-01","rate":%g,"perYr":12,"nPeriods":%d%s}`,
			amt, rate, nper, extra)
		resp, code := amortCall(t, body)
		if code != 200 || resp.Error != "" || len(resp.Schedule) == 0 {
			continue
		}
		// Count cases that actually produced an off-cycle (day-15) prepayment row
		// and cases with adjustments, so coverage is asserted, not assumed.
		if strings.Contains(body, "prepayments") {
			for _, r := range resp.Schedule {
				if strings.Contains(r.Date, "-15") {
					sawPrepayRows++
					break
				}
			}
		}
		if strings.Contains(body, "adjustments") {
			sawAdjust++
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
		t.Logf("frontend render reconciles with the engine across %d swept loans (incl. balloons, %d off-cycle prepay, %d adjustment cases)",
			len(cases), sawPrepayRows, sawAdjust)
	}
	if sawPrepayRows == 0 || sawAdjust == 0 {
		t.Errorf("render sweep did not exercise off-cycle prepay (%d) or adjustment (%d) rows — coverage gap", sawPrepayRows, sawAdjust)
	}
}

// maxInt returns the larger of two ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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

// TestFrontendPVRecalcIdempotentSweep is the Present Value analog: after calcPV
// computes a forward worksheet (filling pv-total and the row Value cells green),
// getPVInput must build the SAME request — the computed Present Value is read
// back as blank (not as a backward-solve target) and the green Value cells aren't
// fed back as inputs. Guards the PV side of the green-cell → field-presence
// invariant on recalc.
func TestFrontendPVRecalcIdempotentSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping PV idempotency sweep")
	}
	html := mustReadIndexHTML(t)

	type idemCase struct {
		Fields    map[string]string   `json:"fields"`
		Lumps     []map[string]string `json:"lumps"`
		Periodics []map[string]string `json:"periodics"`
		LsCount   int                 `json:"lsCount"`
		PerCount  int                 `json:"perCount"`
		Response  json.RawMessage     `json:"response"`
	}
	rng := rand.New(rand.NewSource(0x9f1))
	var cases []idemCase
	for i := 0; i < 10; i++ {
		pct := float64([]int{4, 6, 8}[rng.Intn(3)])
		nLump := 1 + rng.Intn(2)
		nPer := rng.Intn(2)
		var lreq []string
		var domLumps []map[string]string
		for j := 0; j < nLump; j++ {
			date := fmt.Sprintf("%04d-01-01", 2027+rng.Intn(12))
			amt := float64(5000 + rng.Intn(90000))
			lreq = append(lreq, fmt.Sprintf(`{"date":"%s","amount":%g,"act":"N"}`, date, amt))
			domLumps = append(domLumps, map[string]string{"date": date, "amount": commafmt(amt)})
		}
		var preq []string
		var domPers []map[string]string
		for j := 0; j < nPer; j++ {
			amt := float64(500 + rng.Intn(2500))
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
			t.Fatalf("PV setup %d: code=%d err=%q", i, code, resp.Error)
		}
		raw, _ := json.Marshal(resp)
		cases = append(cases, idemCase{
			Fields: map[string]string{"pv-asOfDate": "2024-01-01", "pv-rateType": "true",
				"pv-rate": strconv.FormatFloat(pct, 'f', 4, 64), "pv-total": "", "actu-pod": "", "set-colaMonth": "anniversary"},
			Lumps: domLumps, Periodics: domPers, LsCount: nLump, PerCount: nPer, Response: raw,
		})
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
var autoSilent=false, calcGeneration=0, pvLumpBlanks=[], pvPerBlanks=[], pvLsCount=0, pvPerCount=0, CURRENT_RESPONSE=null;
function clearFieldErrors(){} function setAutoCalcHint(){} function blockInvalidDates(){return false;}
function defaultPVReferenceDate(){} function pvContingencyConfigError(){return null;} function markPVErrorFields(){}
function renderAdvisoryHTML(){return '';} function updatePVActiveSummary(){} function readPVRateSchedule(){return [];}
function getActuarialConfig(){return null;} function pvTrueToType(x){return x;}
async function apiPost(){return CURRENT_RESPONSE;}
function mkEl(v){var cls=[];return {value:(v||''),textContent:'',innerHTML:'',classList:{add:function(c){if(cls.indexOf(c)<0)cls.push(c);},remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);},contains:function(c){return cls.indexOf(c)>=0;}}};}
var ELS={}, SEL={};
var document={getElementById:function(id){if(!(id in ELS))ELS[id]=mkEl('');return ELS[id];},
  querySelector:function(s){return (s in SEL)?SEL[s]:null;}, querySelectorAll:function(){return [];}};
var cases = ` + string(casesJSON) + `;
(async function(){
  var out=[];
  for (var k=0;k<cases.length;k++){
    var c=cases[k]; var lumps=c.lumps||[], pers=c.periodics||[];
    ELS={}; SEL={};
    for (var id in c.fields) ELS[id]=mkEl(c.fields[id]);
    pvLsCount=c.lsCount; pvPerCount=c.perCount;
    for (var i=0;i<lumps.length;i++){
      SEL['input[data-ls="'+i+'"][data-f="date"]']=mkEl(lumps[i].date);
      SEL['input[data-ls="'+i+'"][data-f="amount"]']=mkEl(lumps[i].amount);
      SEL['input[data-ls="'+i+'"][data-f="value"]']=mkEl('');
      SEL['select[data-ls="'+i+'"][data-f="act"]']=mkEl('N');
    }
    for (var j=0;j<pers.length;j++){
      SEL['input[data-per="'+j+'"][data-f="from"]']=mkEl(pers[j].from);
      SEL['input[data-per="'+j+'"][data-f="to"]']=mkEl(pers[j].to);
      SEL['input[data-per="'+j+'"][data-f="perYr"]']=mkEl(pers[j].perYr);
      SEL['input[data-per="'+j+'"][data-f="amount"]']=mkEl(pers[j].amount);
      SEL['input[data-per="'+j+'"][data-f="cola"]']=mkEl(pers[j].cola);
      SEL['input[data-per="'+j+'"][data-f="value"]']=mkEl('');
      SEL['select[data-per="'+j+'"][data-f="act"]']=mkEl('N');
    }
    var b1 = getPVInput();
    CURRENT_RESPONSE = c.response;
    await calcPV();
    var b2 = getPVInput();
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
	var results []struct{ B1, B2 string }
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for i, r := range results {
		if r.B1 != r.B2 {
			t.Errorf("case %d: PV recalc not idempotent\n  before: %s\n  after:  %s", i, r.B1, r.B2)
		}
	}
	t.Logf("PV recalc is idempotent (computed total + Value cells read as blank) across %d swept worksheets", len(results))
}

// TestFrontendMtgRecalcIdempotentSweep is the mortgage analog of the amortization
// recalc-idempotency sweep: after calcMortgageRow populates the computed (green)
// cells, calling getMtgRowData must build the SAME request body — i.e. the
// engine's own outputs (cash/financed/monthly/balloon) are read back as blank,
// not as new inputs. This is the request-mapping side of the green output-cell
// concern: a computed cell wrongly treated as input on recalc would flip the
// field-presence dispatch (e.g. a solved Monthly becoming a hard constraint).
func TestFrontendMtgRecalcIdempotentSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping mortgage idempotency sweep")
	}
	html := mustReadIndexHTML(t)
	mtgFieldsRe := regexp.MustCompile(`(?s)const MTG_FIELDS = \[.*?\];`)
	mtgFields := mtgFieldsRe.FindString(html)

	rng := rand.New(rand.NewSource(0x1de3))
	type idemCase struct {
		Cells    map[string]string `json:"cells"`
		Status   map[string]string `json:"status"`
		Response json.RawMessage   `json:"response"`
	}
	var cases []idemCase
	for i := 0; i < 40; i++ {
		price := float64(80000 + rng.Intn(800000))
		pct := float64(rng.Intn(40)) / 100
		years := 5 + rng.Intn(35)
		rate := 0.03 + rng.Float64()*0.12
		tax := float64(rng.Intn(500))
		points := float64(rng.Intn(400)) / 10000
		cells := map[string]string{
			"price": commafmt(price), "pctDown": strconv.FormatFloat(pct*100, 'f', 4, 64),
			"years": strconv.Itoa(years), "rate": strconv.FormatFloat(rate*100, 'f', 4, 64),
			"tax": commafmt(tax), "points": strconv.FormatFloat(points*100, 'f', 4, 64),
		}
		status := map[string]string{"price": "input", "pctDown": "input", "years": "input", "rate": "input", "tax": "input", "points": "input"}
		body := fmt.Sprintf(`{"price":%f,"pctDown":%f,"years":%d,"rate":%f,"tax":%f,"points":%f}`,
			price, pct, years, rate, tax, points)
		resp := callMortgage(t, body)
		if resp.Error != "" {
			continue
		}
		raw, _ := json.Marshal(resp)
		cases = append(cases, idemCase{Cells: cells, Status: status, Response: raw})
	}
	if len(cases) < 20 {
		t.Fatalf("too few mortgage cases (%d)", len(cases))
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + mtgFields + `
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtRate") + `
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "getMtgRowData") + `
` + extractJS(t, html, "updateMtgRowUI") + `
` + extractJS(t, html, "calcMortgageRow") + `
var mtgSelectedRow = 0, mtgStatus = [{}], calcGeneration = 0, autoSilent = false, CURRENT_RESPONSE = null;
function clearFieldErrors() {} function setAutoCalcHint() {} function markMtgErrorRow() {}
function explainMtgError() { return ''; } function renderAdvisoryHTML() { return ''; }
async function apiPost() { return CURRENT_RESPONSE; }
function mkCell(v) { var cls = []; return { value: (v || ''), classList: { add: function (c) { if (cls.indexOf(c) < 0) cls.push(c); }, remove: function (c) { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); }, contains: function (c) { return cls.indexOf(c) >= 0; } } }; }
var SEL = {}, ELS = {};
function selFor(f) { return '#mtg-body input[data-row="0"][data-field="' + f + '"]'; }
var document = { querySelector: function (s) { return (s in SEL) ? SEL[s] : null; },
  getElementById: function (id) { if (!(id in ELS)) ELS[id] = mkCell(''); return ELS[id]; } };
var ALL = ['price','points','pctDown','cash','financed','years','rate','tax','monthly','balloonYears','balloonAmount','apr'];
var cases = ` + string(casesJSON) + `;
(async function () {
  var out = [];
  for (var k = 0; k < cases.length; k++) {
    var c = cases[k]; SEL = {}; ELS = {};
    ALL.forEach(function (f) { SEL[selFor(f)] = mkCell(''); });
    for (var f in c.cells) SEL[selFor(f)].value = c.cells[f];
    mtgStatus = [{}];
    for (var f in c.status) mtgStatus[0][f] = c.status[f];
    var b1 = getMtgRowData(0);
    CURRENT_RESPONSE = c.response;
    await calcMortgageRow();
    var b2 = getMtgRowData(0);
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
	var results []struct{ B1, B2 string }
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for i, r := range results {
		if r.B1 != r.B2 {
			t.Errorf("case %d: mortgage recalc not idempotent\n  before: %s\n  after:  %s", i, r.B1, r.B2)
		}
	}
	t.Logf("mortgage recalc is idempotent (computed cells read as blank) across %d swept rows", len(results))
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

// TestFrontendPVSolvedEchoSweep closes the PV backward-solve display loop: for
// each random worksheet it forward-computes a target Present Value, then runs the
// SHIPPED calcPV on a backward scenario (blank Rate, blank As-of Date, or blank
// lump Amount, with the target typed into Present Value) and asserts the engine's
// SOLVED value is echoed back into the right green cell, recovering the original
// input (round-trip). The forward value-echo sweep covers display of computed
// Value cells; this covers the solved-cell echo (PV-8 rate, PV-9 as-of, PV-2
// amount) which is a distinct code path in calcPV.
func TestFrontendPVSolvedEchoSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping PV solved-echo sweep")
	}
	html := mustReadIndexHTML(t)

	type solvedCase struct {
		Kind     string          `json:"kind"` // "rate" | "asof" | "amount"
		Date     string          `json:"date"`
		Amount   string          `json:"amount"`
		RatePct  string          `json:"ratePct"`
		AsOf     string          `json:"asof"`  // MM/DD/YYYY
		Total    string          `json:"total"` // target PV
		Response json.RawMessage `json:"response"`
	}
	rng := rand.New(rand.NewSource(50117740))
	var cases []solvedCase
	type expect struct {
		kind string
		want float64 // ratePct, amount, or as-of as days-since-epoch
	}
	var exps []expect
	mdyToDays := func(iso string) float64 { // "YYYY-MM-DD" → day count
		var y, m, d int
		fmt.Sscanf(iso, "%d-%d-%d", &y, &m, &d)
		return float64(y*372 + m*31 + d)
	}
	for i := 0; i < 30; i++ {
		pct := float64([]int{4, 5, 6, 8, 10}[rng.Intn(5)])
		yr := 2027 + rng.Intn(12)
		date := fmt.Sprintf("%04d-01-01", yr)
		amt := float64(10000 + rng.Intn(90000))
		// Forward solve for the target PV at asof 2024-01-01.
		fwd := fmt.Sprintf(`{"asOfDate":"2024-01-01","rate":%g,"lumpSums":[{"date":"%s","amount":%g,"act":"N"}]}`,
			pct/100, date, amt)
		fr, code := pvCall(t, fwd)
		if code != 200 || fr.Error != "" {
			continue
		}
		S := fr.SumValue
		kind := []string{"rate", "asof", "amount"}[i%3]
		var back string
		switch kind {
		case "rate":
			back = fmt.Sprintf(`{"asOfDate":"2024-01-01","sumValue":%g,"lumpSums":[{"date":"%s","amount":%g,"act":"N"}]}`, S, date, amt)
		case "asof":
			back = fmt.Sprintf(`{"rate":%g,"sumValue":%g,"lumpSums":[{"date":"%s","amount":%g,"act":"N"}]}`, pct/100, S, date, amt)
		case "amount":
			back = fmt.Sprintf(`{"asOfDate":"2024-01-01","rate":%g,"sumValue":%g,"lumpSums":[{"date":"%s","act":"N"}]}`, pct/100, S, date)
		}
		br, code := pvCall(t, back)
		if code != 200 || br.Error != "" {
			continue
		}
		raw, _ := json.Marshal(br)
		c := solvedCase{Kind: kind, Date: "01/01/" + fmt.Sprint(yr), Amount: commafmt(amt),
			RatePct: strconv.FormatFloat(pct, 'f', 4, 64), AsOf: "01/01/2024",
			Total: commafmt(S), Response: raw}
		switch kind {
		case "rate":
			c.RatePct = "" // blank → solved
			exps = append(exps, expect{kind: kind, want: pct})
		case "asof":
			c.AsOf = "" // blank → solved
			exps = append(exps, expect{kind: kind, want: mdyToDays("2024-01-01")})
		case "amount":
			c.Amount = "" // blank → solved
			exps = append(exps, expect{kind: kind, want: amt})
		}
		cases = append(cases, c)
	}
	if len(cases) < 15 {
		t.Fatalf("too few PV backward cases (%d)", len(cases))
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + extractJS(t, html, "parseDate") + `
` + extractJS(t, html, "pvRateToTrue") + `
` + extractJS(t, html, "pvTrueToType") + `
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "getPVInput") + `
` + extractJS(t, html, "calcPV") + `
var autoSilent = false, calcGeneration = 0, pvLumpBlanks = [], pvPerBlanks = [], pvLsCount = 1, pvPerCount = 0, CURRENT_RESPONSE = null;
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
    ELS = {}; SEL = {};
    ELS['pv-asOfDate'] = mkEl(c.asof);
    ELS['pv-rateType'] = mkEl('true');
    ELS['pv-rate'] = mkEl(c.ratePct);
    ELS['pv-total'] = mkEl(c.total);
    ELS['actu-pod'] = mkEl('');
    pvLsCount = 1; pvPerCount = 0;
    SEL['input[data-ls="0"][data-f="date"]'] = mkEl(c.date);
    SEL['input[data-ls="0"][data-f="amount"]'] = mkEl(c.amount);
    SEL['input[data-ls="0"][data-f="value"]'] = mkEl('');
    SEL['select[data-ls="0"][data-f="act"]'] = mkEl('N');
    CURRENT_RESPONSE = c.response;
    await calcPV();
    var amtEl = SEL['input[data-ls="0"][data-f="amount"]'];
    out.push({
      rate: ELS['pv-rate'].value,
      asof: ELS['pv-asOfDate'].value,
      amount: amtEl.value,
      rateGreen: ELS['pv-rate'].classList.contains('cell-output'),
      asofGreen: ELS['pv-asOfDate'].classList.contains('cell-output'),
      amountGreen: amtEl.classList.contains('cell-output')
    });
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
	var res []struct {
		Rate        string `json:"rate"`
		Asof        string `json:"asof"`
		Amount      string `json:"amount"`
		RateGreen   bool   `json:"rateGreen"`
		AsofGreen   bool   `json:"asofGreen"`
		AmountGreen bool   `json:"amountGreen"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	mdyDays := func(mdy string) float64 { // "MM/DD/YYYY" → same scale as mdyToDays
		var m, d, y int
		fmt.Sscanf(mdy, "%d/%d/%d", &m, &d, &y)
		return float64(y*372 + m*31 + d)
	}
	for i := range cases {
		switch exps[i].kind {
		case "rate":
			gv, ok := parseFloatLoose(res[i].Rate)
			if !ok || math.Abs(gv-exps[i].want) > 1e-3 {
				t.Errorf("case %d (rate): echoed pv-rate %q → %.4f, want %.4f", i, res[i].Rate, gv, exps[i].want)
			}
			if !res[i].RateGreen {
				t.Errorf("case %d (rate): solved pv-rate cell not marked green (cell-output)", i)
			}
		case "amount":
			gv := parseMoneyStr(res[i].Amount)
			if math.Abs(gv-exps[i].want) > 0.05 {
				t.Errorf("case %d (amount): echoed Amount %q → %.2f, want %.2f", i, res[i].Amount, gv, exps[i].want)
			}
			if !res[i].AmountGreen {
				t.Errorf("case %d (amount): solved lump Amount cell not marked green (cell-output)", i)
			}
		case "asof":
			if res[i].Asof == "" {
				t.Errorf("case %d (asof): no date echoed", i)
				continue
			}
			if math.Abs(mdyDays(res[i].Asof)-exps[i].want) > 35 { // within ~1 month on the synthetic day scale
				t.Errorf("case %d (asof): echoed As-of %q, want ~2024-01-01", i, res[i].Asof)
			}
			if !res[i].AsofGreen {
				t.Errorf("case %d (asof): solved pv-asOfDate cell not marked green (cell-output)", i)
			}
		}
	}
	t.Logf("PV solved-cell echo (rate/as-of/amount) round-trips verified across %d backward worksheets", len(cases))
}

func parseFloatLoose(s string) (float64, bool) {
	s = strings.TrimSpace(strings.NewReplacer("$", "", ",", "", "%", "", " ", "").Replace(s))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
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

// TestFrontendMtgOutputEchoSweep closes the mortgage display loop: it drives
// random mortgage rows through the real /api/mortgage/calc handler (engine numbers
// are DOS-oracle-validated) and then runs the SHIPPED updateMtgRowUI to place the
// response into the grid cells, asserting every displayed cell parses back to the
// engine value with the correct scaling (points/pctDown/rate ÷100 on display,
// money via fmtDollars, ints as-is, APR ×100 with a % suffix). This is the
// mortgage analog of the amortization render sweep and the PV value-echo sweep —
// it catches a wrong cell mapping or a wrong scale in the output path, which
// request-mapping coverage alone cannot see.
func TestFrontendMtgOutputEchoSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping mortgage output-echo sweep")
	}
	html := mustReadIndexHTML(t)

	rng := rand.New(rand.NewSource(0x309a6e))
	type echoCase struct {
		Status map[string]bool `json:"status"`
		Resp   json.RawMessage `json:"resp"`
	}
	var cases []echoCase
	var resps []MortgageResponse
	for i := 0; i < 60; i++ {
		price := float64(80000 + rng.Intn(900000))
		pct := float64(rng.Intn(40)) / 100
		years := 5 + rng.Intn(35)
		rate := 0.03 + rng.Float64()*0.12
		tax := float64(rng.Intn(600))
		points := float64(rng.Intn(500)) / 10000
		status := map[string]bool{"price": true, "pctDown": true, "years": true, "rate": true, "tax": true, "points": true}
		body := fmt.Sprintf(`{"price":%f,"pctDown":%f,"years":%d,"rate":%f,"tax":%f,"points":%f`,
			price, pct, years, rate, tax, points)
		if years > 4 && rng.Intn(2) == 0 {
			by := 3 + rng.Intn(years-2)
			body += fmt.Sprintf(`,"balloonYears":%d`, by)
			status["balloonYears"] = true
		}
		body += "}"
		resp := callMortgage(t, body)
		if resp.Error != "" {
			continue
		}
		rb, _ := json.Marshal(resp)
		cases = append(cases, echoCase{Status: status, Resp: rb})
		resps = append(resps, resp)
	}
	if len(cases) < 30 {
		t.Fatalf("too few mortgage responses (%d)", len(cases))
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "fmtRate") + `
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "updateMtgRowUI") + `
var MTG_ALL = ['price','points','pctDown','cash','financed','years','rate','tax','monthly','balloonYears','balloonAmount','apr'];
var mtgStatus = [{}];
var SEL = {};
function mkCell() { var cls=[]; return { value:'', classList:{ add:function(c){if(cls.indexOf(c)<0)cls.push(c);}, remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);}, contains:function(c){return cls.indexOf(c)>=0;} } }; }
function selFor(f){ return '#mtg-body input[data-row="0"][data-field="'+f+'"]'; }
var document = { querySelector:function(s){ return (s in SEL)?SEL[s]:null; } };
var cases = ` + string(casesJSON) + `;
var out = cases.map(function(c){
  mtgStatus = [{}];
  for (var f in c.status) mtgStatus[0][f] = 'input';
  SEL = {};
  MTG_ALL.forEach(function(f){ SEL[selFor(f)] = mkCell(); });
  updateMtgRowUI(0, c.resp);
  var disp = {}, green = {};
  MTG_ALL.forEach(function(f){ disp[f] = SEL[selFor(f)].value; green[f] = SEL[selFor(f)].classList.contains('cell-output'); });
  return { disp: disp, green: green };
});
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var rows []struct {
		Disp  map[string]string `json:"disp"`
		Green map[string]bool   `json:"green"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	disp := make([]map[string]string, len(rows))
	for i := range rows {
		disp[i] = rows[i].Disp
	}

	parseNum := func(s string) (float64, bool) {
		s = strings.TrimSpace(strings.NewReplacer("$", "", ",", "", "%", "", " ", "").Replace(s))
		if s == "" {
			return 0, false
		}
		v, err := strconv.ParseFloat(s, 64)
		return v, err == nil
	}
	money := map[string]bool{"price": true, "cash": true, "financed": true, "tax": true, "monthly": true, "balloonAmount": true}
	rateF := map[string]bool{"points": true, "pctDown": true, "rate": true}
	checks := 0
	for i := range cases {
		r := resps[i]
		want := map[string]float64{
			"price": r.Price, "points": r.Points, "pctDown": r.PctDown, "cash": r.Cash,
			"financed": r.Financed, "years": float64(r.Years), "rate": r.Rate, "tax": r.Tax,
			"monthly": r.Monthly, "balloonYears": float64(r.BalloonYears), "balloonAmount": r.BalloonAmount,
		}
		for f, wv := range want {
			cell := disp[i][f]
			if cell == "" {
				continue // skipped (null/zero-non-input) — request-mapping sweep covers omission
			}
			gv, ok := parseNum(cell)
			if !ok {
				t.Errorf("case %d field %s: unparseable cell %q", i, f, cell)
				continue
			}
			if rateF[f] {
				gv /= 100
			}
			tol := 1e-4
			if money[f] {
				tol = 0.01
			}
			if math.Abs(gv-wv) > tol {
				t.Errorf("case %d field %s: displayed %q → %.6f, engine %.6f (Δ%.2e)", i, f, cell, gv, wv, math.Abs(gv-wv))
			}
			checks++
		}
		// APR cell, when the engine converged.
		if r.APRConverged && r.APR != 0 {
			if gv, ok := parseNum(disp[i]["apr"]); ok {
				if math.Abs(gv/100-r.APR) > 1e-4 {
					t.Errorf("case %d apr: displayed %q → %.6f, engine %.6f", i, disp[i]["apr"], gv/100, r.APR)
				}
				checks++
			}
		}
		// Green output-cell state (user-flagged watch-item): a cell the engine
		// COMPUTED (not user input) and actually displayed must be marked green
		// (cell-output); a cell the USER typed must NOT be green. A computed
		// output that fails to turn green — or an input that wrongly shows as
		// output — corrupts the next solve's field-presence dispatch.
		for f := range want {
			displayed := disp[i][f] != ""
			isInput := cases[i].Status[f]
			green := rows[i].Green[f]
			if isInput && green {
				t.Errorf("case %d field %s: user-input cell wrongly marked green (output)", i, f)
			}
			if !isInput && displayed && !green {
				t.Errorf("case %d field %s: computed output %q failed to turn green", i, f, disp[i][f])
			}
		}
	}
	t.Logf("mortgage output echo verified across %d cells in %d swept rows", checks, len(cases))
}

// TestFrontendAmzAdvancedRowMappingSweep sweeps random Advanced-Option rows
// (prepayments, balloons, rate/payment adjustments) plus the scalar
// moratorium/target/skip fields through the SHIPPED getAmzInput and asserts each
// maps into the request body correctly: dates → ISO, money parsed, adjustment
// rate ÷100, optional fields omitted when blank, and target balloons (date-only)
// carried with no amount. This is the advanced-option analog of the core
// request-mapping sweep — the row-placement layer that request-mapping coverage
// did not previously reach.
func TestFrontendAmzAdvancedRowMappingSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping advanced-row mapping sweep")
	}
	html := mustReadIndexHTML(t)

	mdy := func(d time.Time) string { return fmt.Sprintf("%02d/%02d/%04d", d.Month(), d.Day(), d.Year()) }
	iso := func(d time.Time) string { return d.Format("2006-01-02") }

	type row map[string]string
	type acase struct {
		Prepay []row  `json:"prepay"`
		Ball   []row  `json:"ball"`
		Adj    []row  `json:"adj"`
		Mor    string `json:"mor"`
		Target string `json:"target"`
		Skip   string `json:"skip"`
	}
	rng := rand.New(rand.NewSource(0xAD7))
	var cases []acase
	var expects []map[string]interface{}
	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 40; i++ {
		c := acase{}
		exp := map[string]interface{}{}
		// Prepayments
		var preExp []map[string]interface{}
		for j := 0; j < 1+rng.Intn(2); j++ {
			start := base.AddDate(0, rng.Intn(24), 0)
			perY := []int{1, 4, 12}[rng.Intn(3)]
			amt := float64(100 + rng.Intn(2000))
			r := row{"startDate": mdy(start), "perYr": strconv.Itoa(perY), "amount": commafmt(amt)}
			e := map[string]interface{}{"startDate": iso(start), "perYr": float64(perY), "amount": amt}
			if rng.Intn(2) == 0 {
				n := 1 + rng.Intn(24)
				r["nPmts"] = strconv.Itoa(n)
				e["nPmts"] = float64(n)
			}
			if rng.Intn(2) == 0 {
				stop := start.AddDate(rng.Intn(5)+1, 0, 0)
				r["stopDate"] = mdy(stop)
				e["stopDate"] = iso(stop)
			}
			c.Prepay = append(c.Prepay, r)
			preExp = append(preExp, e)
		}
		exp["prepayments"] = preExp
		// Balloons
		var balExp []map[string]interface{}
		for j := 0; j < 1+rng.Intn(2); j++ {
			d := base.AddDate(0, 6+rng.Intn(60), 0)
			r := row{"date": mdy(d)}
			e := map[string]interface{}{"date": iso(d)}
			if rng.Intn(2) == 0 { // amount present; else target balloon (date only)
				amt := float64(5000 + rng.Intn(50000))
				r["amount"] = "$" + commafmt(amt)
				e["amount"] = amt
			}
			c.Ball = append(c.Ball, r)
			balExp = append(balExp, e)
		}
		exp["balloons"] = balExp
		// Adjustments
		var adjExp []map[string]interface{}
		for j := 0; j < 1+rng.Intn(2); j++ {
			d := base.AddDate(0, 6+rng.Intn(60), 0)
			r := row{"date": mdy(d)}
			e := map[string]interface{}{"date": iso(d)}
			if rng.Intn(2) == 0 {
				rt := float64(rng.Intn(1500)+100) / 100
				r["rate"] = strconv.FormatFloat(rt, 'f', 4, 64)
				e["rate"] = rt / 100
			}
			if rng.Intn(2) == 0 {
				amt := float64(500 + rng.Intn(3000))
				r["amount"] = commafmt(amt)
				e["amount"] = amt
			}
			c.Adj = append(c.Adj, r)
			adjExp = append(adjExp, e)
		}
		exp["adjustments"] = adjExp
		// Scalars
		if rng.Intn(2) == 0 {
			d := base.AddDate(0, 2+rng.Intn(10), 0)
			c.Mor = mdy(d)
			exp["moratorium"] = iso(d)
		}
		if rng.Intn(2) == 0 {
			tv := float64(200 + rng.Intn(1500))
			c.Target = commafmt(tv)
			exp["targetAmt"] = tv
		}
		if rng.Intn(2) == 0 {
			c.Skip = "6-8,12"
			exp["skipMonths"] = "6-8,12"
		}
		cases = append(cases, c)
		expects = append(expects, exp)
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
function mkEl(v){ var cls=[]; return { value:(v||''), classList:{ add:function(c){if(cls.indexOf(c)<0)cls.push(c);}, remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);}, contains:function(c){return cls.indexOf(c)>=0;} } }; }
// Fixed, valid core fields so getAmzInput reaches the advanced-row parsing.
var CORE = { 'amz-amount':'100000','amz-loanDate':'01/01/2024','amz-rate':'6.0000',
  'amz-firstDate':'','amz-lastDate':'','amz-nPeriods':'120','amz-perYr':'12',
  'amz-payment':'','amz-basis':'360','amz-points':'0','amz-moratorium':'','amz-targetAmt':'','amz-skipMonths':'' };
var ELS = {};
function mkRow(fieldAttr, fields){ return { querySelector:function(sel){ var m=sel.match(/="([^"]+)"/); var f=m?m[1]:''; return { value: (fields[f]||'') }; } }; }
var QSA = {};
var document = {
  getElementById:function(id){ if(!(id in ELS)) ELS[id]=mkEl(''); return ELS[id]; },
  querySelectorAll:function(sel){ return QSA[sel]||[]; }
};
var cases = ` + string(casesJSON) + `;
var out = cases.map(function(c){
  ELS = {};
  for (var id in CORE) ELS[id] = mkEl(CORE[id]);
  if (c.mor)    ELS['amz-moratorium'] = mkEl(c.mor);
  if (c.target) ELS['amz-targetAmt']  = mkEl(c.target);
  if (c.skip)   ELS['amz-skipMonths'] = mkEl(c.skip);
  QSA = {};
  QSA['#amz-prepay-body [data-amz-prepay-row]'] = (c.prepay||[]).map(function(r){ return mkRow('prepay', r); });
  QSA['#amz-balloon-body [data-amz-balloon-row]'] = (c.ball||[]).map(function(r){ return mkRow('balloon', r); });
  QSA['#amz-adj-body [data-amz-adj-row]'] = (c.adj||[]).map(function(r){ return mkRow('adj', r); });
  return getAmzInput();
});
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	// getAmzInput returns { body } (or { error }); unwrap to the body map.
	var wrapped []map[string]json.RawMessage
	if err := json.Unmarshal(out, &wrapped); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	bodies := make([]map[string]json.RawMessage, len(wrapped))
	for i, w := range wrapped {
		if raw, ok := w["body"]; ok {
			_ = json.Unmarshal(raw, &bodies[i])
		} else {
			bodies[i] = w // error case — keep so the error check below fires
		}
	}

	approx := func(a, b float64) bool { return math.Abs(a-b) < 0.005 }
	for i := range cases {
		if errRaw, ok := bodies[i]["error"]; ok {
			t.Errorf("case %d: getAmzInput returned error %s", i, errRaw)
			continue
		}
		// Compare each advanced collection.
		for _, key := range []string{"prepayments", "balloons", "adjustments"} {
			wantArr, _ := expects[i][key].([]map[string]interface{})
			var got []map[string]interface{}
			if raw, ok := bodies[i][key]; ok {
				_ = json.Unmarshal(raw, &got)
			}
			if len(got) != len(wantArr) {
				t.Errorf("case %d %s: got %d rows, want %d", i, key, len(got), len(wantArr))
				continue
			}
			for r := range wantArr {
				for f, wv := range wantArr[r] {
					gv, present := got[r][f]
					if !present {
						t.Errorf("case %d %s[%d]: missing field %s", i, key, r, f)
						continue
					}
					switch wt := wv.(type) {
					case string:
						if gs, _ := gv.(string); gs != wt {
							t.Errorf("case %d %s[%d].%s = %q, want %q", i, key, r, f, gs, wt)
						}
					case float64:
						if gf, _ := gv.(float64); !approx(gf, wt) {
							t.Errorf("case %d %s[%d].%s = %v, want %v", i, key, r, f, gv, wt)
						}
					}
				}
				for f := range got[r] {
					if _, ok := wantArr[r][f]; !ok {
						t.Errorf("case %d %s[%d]: unexpected field %s = %v", i, key, r, f, got[r][f])
					}
				}
			}
		}
		// Scalars.
		for _, key := range []string{"moratorium", "skipMonths"} {
			if wv, ok := expects[i][key]; ok {
				var gs string
				if raw, ok := bodies[i][key]; ok {
					_ = json.Unmarshal(raw, &gs)
				}
				if gs != wv.(string) {
					t.Errorf("case %d %s = %q, want %q", i, key, gs, wv)
				}
			} else if _, present := bodies[i][key]; present {
				t.Errorf("case %d: unexpected %s in body", i, key)
			}
		}
		if wv, ok := expects[i]["targetAmt"]; ok {
			var gf float64
			if raw, ok := bodies[i]["targetAmt"]; ok {
				_ = json.Unmarshal(raw, &gf)
			}
			if !approx(gf, wv.(float64)) {
				t.Errorf("case %d targetAmt = %v, want %v", i, gf, wv)
			}
		}
	}
	t.Logf("advanced-option row mapping (prepay/balloon/adjust + mor/target/skip) verified across %d worksheets", len(cases))
}

// TestFrontendClearMortgageStateSweep is the mortgage analog of the PV/Amz
// clear-state sweeps: it fills every cell in a row (plus the APR cell and the
// per-row status map) with junk and a stale cell-output marker, runs the SHIPPED
// clearMortgageRow, and asserts a complete reset — every cell blanked and
// de-greened, the APR cell cleared, and mtgStatus[row] emptied. Guards the
// leftover-mortgage-row class (a stale computed value or status surviving a Clear
// Row and corrupting the next solve).
func TestFrontendClearMortgageStateSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping mortgage clear-state sweep")
	}
	html := mustReadIndexHTML(t)
	mtgFieldsRe := regexp.MustCompile(`(?s)const MTG_FIELDS = \[.*?\];`)
	mtgFields := mtgFieldsRe.FindString(html)
	if mtgFields == "" {
		t.Fatal("MTG_FIELDS not found")
	}
	keys := []string{"price", "points", "pctDown", "cash", "financed", "years", "rate", "tax", "monthly", "balloonYears", "balloonAmount"}

	harness := `
'use strict';
` + mtgFields + `
` + extractJS(t, html, "getMtgCell") + `
` + extractJS(t, html, "clearMortgageRow") + `
var mtgSelectedRow = 0;
var mtgStatus = [{}];
function clearFieldErrors() {}
function setAutoCalcHint() {}
function mkCell() { var cls = ['cell-output']; return { value: 'JUNK', classList: { add: function (c) { if (cls.indexOf(c) < 0) cls.push(c); }, remove: function (c) { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); }, contains: function (c) { return cls.indexOf(c) >= 0; } }, _cls: cls }; }
var SEL = {}, ELS = {};
function selFor(f) { return '#mtg-body input[data-row="0"][data-field="' + f + '"]'; }
var document = { querySelector: function (s) { return (s in SEL) ? SEL[s] : null; },
  getElementById: function (id) { if (!(id in ELS)) ELS[id] = { value: '', textContent: 'stale' }; return ELS[id]; } };
var KEYS = ` + mustJSON(append(keys, "apr")) + `;
KEYS.forEach(function (f) { SEL[selFor(f)] = mkCell(); });
KEYS.forEach(function (f) { mtgStatus[0][f] = 'input'; });
clearMortgageRow();
var out = { values: {}, greens: {}, status: Object.keys(mtgStatus[0]).length };
KEYS.forEach(function (f) { var c = SEL[selFor(f)]; out.values[f] = c.value; out.greens[f] = c.classList.contains('cell-output'); });
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var res struct {
		Values map[string]string `json:"values"`
		Greens map[string]bool   `json:"greens"`
		Status int               `json:"status"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for f, v := range res.Values {
		if v != "" {
			t.Errorf("after clear, cell %s = %q, want empty", f, v)
		}
		if res.Greens[f] {
			t.Errorf("after clear, cell %s still has cell-output (green) marker", f)
		}
	}
	if res.Status != 0 {
		t.Errorf("after clear, mtgStatus[row] has %d keys, want 0", res.Status)
	}
	t.Logf("clearMortgageRow fully resets %d cells + APR + status map", len(res.Values))
}

// TestFrontendMoneyReformatSweep verifies the shipped on-blur money reformatting
// is applied to exactly the right fields across all three screens: every money
// input (amz amount/payment/target/payoff, PV total / POD, PV & advanced-option
// Amount cells, mortgage money columns) reformats a committed value to
// $xx,xxx.xx, while rate / int / date / computed-Value cells and non-inputs are
// left untouched. Pairs isMoneyField (the classifier) with formatMoneyField (the
// action) the way the blur handler does.
func TestFrontendMoneyReformatSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping money-reformat sweep")
	}
	html := mustReadIndexHTML(t)
	mtgFieldsRe := regexp.MustCompile(`(?s)const MTG_FIELDS = \[.*?\];`)
	mtgFields := mtgFieldsRe.FindString(html)

	type mcase struct {
		Desc  string            `json:"desc"`
		Tag   string            `json:"tag"`
		ID    string            `json:"id"`
		DS    map[string]string `json:"ds"`
		Raw   string            `json:"raw"`
		Money bool              `json:"money"` // expected isMoneyField
		Want  string            `json:"want"`  // expected value after format (money only)
	}
	cases := []mcase{
		{"amz-amount", "INPUT", "amz-amount", nil, "1234.5", true, "$1,234.50"},
		{"amz-payment", "INPUT", "amz-payment", nil, "1,000", true, "$1,000.00"},
		{"amz-targetAmt", "INPUT", "amz-targetAmt", nil, "800", true, "$800.00"},
		{"pv-total", "INPUT", "pv-total", nil, "$50000", true, "$50,000.00"},
		{"actu-pod", "INPUT", "actu-pod", nil, "25000", true, "$25,000.00"},
		{"amz balloon amount", "INPUT", "", map[string]string{"amzBalloonField": "amount"}, "9999.9", true, "$9,999.90"},
		{"amz prepay amount", "INPUT", "", map[string]string{"amzPrepayField": "amount"}, "150", true, "$150.00"},
		{"amz adj amount", "INPUT", "", map[string]string{"amzAdjField": "amount"}, "1200", true, "$1,200.00"},
		{"pv lump amount", "INPUT", "", map[string]string{"f": "amount"}, "10000", true, "$10,000.00"},
		{"mtg price", "INPUT", "", map[string]string{"field": "price"}, "300000", true, "$300,000.00"},
		{"mtg tax", "INPUT", "", map[string]string{"field": "tax"}, "275", true, "$275.00"},
		{"money but unparseable", "INPUT", "amz-amount", nil, "abc", true, "abc"}, // left as-is
		{"money but empty", "INPUT", "pv-total", nil, "", true, ""},               // left as-is
		// Non-money fields: isMoneyField=false, value untouched.
		{"amz rate", "INPUT", "amz-rate", nil, "6.5", false, "6.5"},
		{"mtg rate column", "INPUT", "", map[string]string{"field": "rate"}, "7.25", false, "7.25"},
		{"mtg years column", "INPUT", "", map[string]string{"field": "years"}, "30", false, "30"},
		{"pv value cell", "INPUT", "", map[string]string{"f": "value"}, "1,234.00 (p=50.0%)", false, "1,234.00 (p=50.0%)"},
		{"pv date cell", "INPUT", "", map[string]string{"f": "date"}, "01/01/2024", false, "01/01/2024"},
		{"select not input", "SELECT", "amz-basis", nil, "360", false, "360"},
	}
	casesJSON, _ := json.Marshal(cases)

	harness := `
'use strict';
` + mtgFields + `
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "fmtMoney") + `
` + extractJS(t, html, "fmtDollars") + `
` + extractJS(t, html, "isMoneyField") + `
` + extractJS(t, html, "formatMoneyField") + `
var cases = ` + string(casesJSON) + `;
var out = cases.map(function (c) {
  var el = { tagName: c.tag, id: c.id || '', dataset: c.ds || {}, value: c.raw };
  var isMoney = isMoneyField(el);
  if (isMoney) formatMoneyField(el);
  return { money: isMoney, value: el.value };
});
console.log(JSON.stringify(out));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var res []struct {
		Money bool   `json:"money"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for i, c := range cases {
		if res[i].Money != c.Money {
			t.Errorf("%s: isMoneyField = %v, want %v", c.Desc, res[i].Money, c.Money)
		}
		if res[i].Value != c.Want {
			t.Errorf("%s: value after blur = %q, want %q", c.Desc, res[i].Value, c.Want)
		}
	}
	t.Logf("money reformat-on-blur verified across %d field cases (classifier + formatter)", len(cases))
}

// TestFrontendPVContingencyEchoSweep closes the last PV forward display gap: a
// life-contingent lump (act = Living / Dead) makes the engine return a survival
// probability < 1 and a value discounted by it. This drives REAL contingency PVs
// through the handler (engine prob is actuarial-validated elsewhere) and asserts
// the shipped calcPV renders each Value cell as the discounted value WITH the
// "(p=NN.N%)" probability suffix, and marks it green (cell-output). The forward
// value-echo sweep covered act='N' only; this exercises the contingency suffix
// and the green-output state on a contingent row.
func TestFrontendPVContingencyEchoSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping PV contingency-echo sweep")
	}
	html := mustReadIndexHTML(t)
	qxJSON, _ := json.Marshal(podOnlyQx())

	type cdat struct {
		Fields   map[string]string `json:"fields"`
		Lump     map[string]string `json:"lump"`
		Response json.RawMessage   `json:"response"`
	}
	type want struct {
		value float64
		prob  float64
	}
	var cases []cdat
	var wants []want
	combos := []struct {
		act     string
		yrOut   int
		dob     string
		ratePct float64
	}{
		{"L", 20, "1980-01-01", 6},
		{"D", 15, "1970-01-01", 5},
		{"L", 30, "1990-06-01", 8},
		{"D", 25, "1960-03-15", 4},
		{"L", 10, "1985-01-01", 6},
	}
	for _, cm := range combos {
		date := fmt.Sprintf("%04d-01-01", 2024+cm.yrOut)
		amt := 50000.0
		body := fmt.Sprintf(`{"asOfDate":"2024-01-01","rate":%g,"lumpSums":[{"date":"%s","amount":%g,"act":"%s"}],`+
			`"actuarial":{"table1":%s,"dob1":"%s","asOfNow":"2024-01-01"}}`,
			cm.ratePct/100, date, amt, cm.act, string(qxJSON), cm.dob)
		resp, code := pvCall(t, body)
		if code != 200 || resp.Error != "" || len(resp.LumpSums) == 0 {
			t.Fatalf("contingency setup failed: code=%d err=%q", code, resp.Error)
		}
		raw, _ := json.Marshal(resp)
		cases = append(cases, cdat{
			Fields: map[string]string{"pv-asOfDate": "2024-01-01", "pv-rateType": "true",
				"pv-rate": strconv.FormatFloat(cm.ratePct, 'f', 4, 64), "pv-total": "", "actu-pod": "", "set-colaMonth": "anniversary"},
			Lump:     map[string]string{"date": date, "amount": commafmt(amt), "act": cm.act},
			Response: raw,
		})
		wants = append(wants, want{value: resp.LumpSums[0].Value, prob: resp.LumpSums[0].Prob})
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
var autoSilent=false, calcGeneration=0, pvLumpBlanks=[], pvPerBlanks=[], pvLsCount=1, pvPerCount=0, CURRENT_RESPONSE=null;
function clearFieldErrors(){} function setAutoCalcHint(){} function blockInvalidDates(){return false;}
function defaultPVReferenceDate(){} function pvContingencyConfigError(){return null;} function markPVErrorFields(){}
function renderAdvisoryHTML(){return '';} function updatePVActiveSummary(){} function readPVRateSchedule(){return [];}
function getActuarialConfig(){return {ok:true};} function pvTrueToType(x){return x;}
async function apiPost(){return CURRENT_RESPONSE;}
function mkEl(v){var cls=[];return {value:(v||''),textContent:'',innerHTML:'',classList:{add:function(c){if(cls.indexOf(c)<0)cls.push(c);},remove:function(c){var i=cls.indexOf(c);if(i>=0)cls.splice(i,1);},contains:function(c){return cls.indexOf(c)>=0;}}};}
var ELS={}, SEL={};
var document={getElementById:function(id){if(!(id in ELS))ELS[id]=mkEl('');return ELS[id];},
  querySelector:function(s){return (s in SEL)?SEL[s]:null;}, querySelectorAll:function(){return [];}};
var cases = ` + string(casesJSON) + `;
(async function(){
  var out=[];
  for (var k=0;k<cases.length;k++){
    var c=cases[k]; ELS={}; SEL={};
    for (var id in c.fields) ELS[id]=mkEl(c.fields[id]);
    pvLsCount=1; pvPerCount=0;
    SEL['input[data-ls="0"][data-f="date"]']=mkEl(c.lump.date);
    SEL['input[data-ls="0"][data-f="amount"]']=mkEl(c.lump.amount);
    SEL['input[data-ls="0"][data-f="value"]']=mkEl('');
    SEL['select[data-ls="0"][data-f="act"]']=mkEl(c.lump.act);
    CURRENT_RESPONSE=c.response;
    await calcPV();
    var ve=SEL['input[data-ls="0"][data-f="value"]'];
    out.push({value: ve.value, green: ve.classList.contains('cell-output')});
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
	var res []struct {
		Value string `json:"value"`
		Green bool   `json:"green"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}
	for i, w := range wants {
		// Expected display: $value + " (p=NN.N%)" when 0<prob<1. Strip the
		// suffix before parsing the dollar amount.
		valStr := res[i].Value
		if idx := strings.Index(valStr, " (p="); idx >= 0 {
			valStr = valStr[:idx]
		}
		base := parseMoneyStr(valStr)
		if math.Abs(base-w.value) > 0.02 {
			t.Errorf("case %d: Value cell %q → %.2f, engine value %.2f", i, res[i].Value, base, w.value)
		}
		wantSuffix := ""
		if w.prob > 0 && w.prob < 1.0 {
			wantSuffix = fmt.Sprintf("(p=%.1f%%)", w.prob*100)
		}
		if wantSuffix != "" && !strings.Contains(res[i].Value, wantSuffix) {
			t.Errorf("case %d: Value cell %q missing probability suffix %q (prob=%.4f)", i, res[i].Value, wantSuffix, w.prob)
		}
		if !res[i].Green {
			t.Errorf("case %d: contingent Value cell not marked green (cell-output)", i)
		}
	}
	t.Logf("PV contingency value-echo (probability suffix + green output) verified across %d worksheets", len(cases))
}

// TestFrontendPVVRActuarialMappingSweep exercises the two request-mapping paths
// the other PV sweeps stub out: the variable-rate schedule (readPVRateSchedule)
// and the life-contingency / Payment-on-Death config (getActuarialConfig). It
// runs the SHIPPED getPVInput with a real rate-schedule grid and actuarial
// section in the DOM and asserts body.rateSchedule (dates → ISO, the starting row
// sentinel 1900-01-01, and True/Loan/Yield → continuous True-rate conversion) and
// body.actuarial (custom table parsed to [[age,qx]], DOBs/now → ISO, POD, optional
// second life) map correctly. Closes the request-mapping coverage on the PV
// screen's two fanciest inputs.
func TestFrontendPVVRActuarialMappingSweep(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping PV VR/actuarial mapping sweep")
	}
	html := mustReadIndexHTML(t)

	// Continuous True-rate conversions mirroring readPVRateSchedule (as fractions).
	yieldToTrue := func(pct float64) float64 { return math.Log(1 + pct/100) }
	loanToTrue := func(pct float64) float64 { return 12 * math.Log(1+(pct/100)/12) }

	// One representative worksheet: VR schedule (starting True row + dated Loan and
	// Yield rows) and an actuarial section (custom 2-life table + POD).
	csv1 := "20,0.001\n21,0.0012\n22,0.0015\n120,1"
	csv2 := "20,0.0009\n21,0.0011\n120,1"
	type rsRow struct {
		RS     string `json:"rs"`
		TrueR  string `json:"trueR"`
		LoanR  string `json:"loanR"`
		YieldR string `json:"yieldR"`
		Date   string `json:"date"`
	}
	vr := []rsRow{
		{"0", "6.0", "", "", ""},           // starting True 6% → 0.06, date sentinel
		{"1", "", "7.0", "", "06/01/2026"}, // Loan 7%
		{"2", "", "", "5.0", "01/01/2028"}, // Yield 5%
	}

	harness := `
'use strict';
` + extractJS(t, html, "parseMoney") + `
` + extractJS(t, html, "parseRate") + `
` + extractJS(t, html, "parseInt2") + `
` + extractJS(t, html, "parseDate") + `
` + extractJS(t, html, "pvRateToTrue") + `
` + extractJS(t, html, "getActuarialTable") + `
` + extractJS(t, html, "getActuarialConfig") + `
` + extractJS(t, html, "readPVRateSchedule") + `
` + extractJS(t, html, "getPVInput") + `
var SSA_2021_MALE_QX, SSA_2021_FEMALE_QX; // not used (custom tables)
var ELS = {}, RS = ` + mustJSON(vr) + `;
function mkEl(v) { var cls = []; return { value: (v || ''),
  classList: { add: function (c) { if (cls.indexOf(c) < 0) cls.push(c); }, remove: function (c) { var i = cls.indexOf(c); if (i >= 0) cls.splice(i, 1); }, contains: function (c) { return cls.indexOf(c) >= 0; } } }; }
function rsRowEl(r) { return { dataset: { rs: r.rs }, querySelector: function (sel) {
  var m = sel.match(/="([^"]+)"/); var f = m ? m[1] : '';
  var map = { trueRate: r.trueR, loanRate: r.loanR, yield: r.yieldR, date: r.date };
  return { value: (map[f] || '') }; } }; }
var lumpSEL = {
  'input[data-ls="0"][data-f="date"]': mkEl('2030-01-01'),
  'input[data-ls="0"][data-f="amount"]': mkEl('50000'),
  'input[data-ls="0"][data-f="value"]': mkEl(''),
  'select[data-ls="0"][data-f="act"]': mkEl('L')   // life-contingent → triggers actuarial
};
var document = {
  getElementById: function (id) { if (!(id in ELS)) ELS[id] = mkEl(''); return ELS[id]; },
  querySelector: function (s) { return (s in lumpSEL) ? lumpSEL[s] : null; },
  querySelectorAll: function (sel) {
    if (sel.indexOf('pv-rateSched') >= 0) return RS.map(rsRowEl);
    return [];
  }
};
var pvLumpBlanks = [], pvPerBlanks = [], pvLsCount = 1, pvPerCount = 0;
function readPVRateScheduleDom() {}
ELS['pv-asOfDate'] = mkEl('2024-01-01');
ELS['pv-rateType'] = mkEl('true');
ELS['pv-rate'] = mkEl('6.0000');
ELS['pv-total'] = mkEl('');
ELS['set-colaMonth'] = mkEl('anniversary');
ELS['actu-table1'] = mkEl('custom'); ELS['actu-csv1'] = mkEl(` + jsLit(csv1) + `);
ELS['actu-table2'] = mkEl('custom'); ELS['actu-csv2'] = mkEl(` + jsLit(csv2) + `);
ELS['actu-dob1'] = mkEl('01/15/1960'); ELS['actu-dob2'] = mkEl('03/20/1962');
ELS['actu-now'] = mkEl('01/01/2024'); ELS['actu-pod'] = mkEl('25000');
var body = getPVInput().body || getPVInput();
console.log(JSON.stringify({ rateSchedule: body.rateSchedule, actuarial: body.actuarial }));
`
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(harness)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var body struct {
		RateSchedule []struct {
			Date     string  `json:"date"`
			TrueRate float64 `json:"trueRate"`
		} `json:"rateSchedule"`
		Actuarial *struct {
			Table1  [][]float64 `json:"table1"`
			Table2  [][]float64 `json:"table2"`
			DOB1    string      `json:"dob1"`
			DOB2    string      `json:"dob2"`
			AsOfNow string      `json:"asOfNow"`
			POD     float64     `json:"pod"`
		} `json:"actuarial"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("parse node output: %v\n%s", err, out)
	}

	// --- variable-rate schedule ---
	wantRS := []struct {
		date string
		tr   float64
	}{
		{"1900-01-01", 0.06},
		{"2026-06-01", loanToTrue(7.0)},
		{"2028-01-01", yieldToTrue(5.0)},
	}
	if len(body.RateSchedule) != len(wantRS) {
		t.Fatalf("rateSchedule has %d rows, want %d: %+v", len(body.RateSchedule), len(wantRS), body.RateSchedule)
	}
	for i, w := range wantRS {
		g := body.RateSchedule[i]
		if g.Date != w.date {
			t.Errorf("rateSchedule[%d].date = %q, want %q", i, g.Date, w.date)
		}
		if math.Abs(g.TrueRate-w.tr) > 1e-9 {
			t.Errorf("rateSchedule[%d].trueRate = %.10f, want %.10f", i, g.TrueRate, w.tr)
		}
	}

	// --- actuarial config ---
	a := body.Actuarial
	if a == nil {
		t.Fatal("body.actuarial missing (contingency row should trigger it)")
	}
	if a.DOB1 != "1960-01-15" || a.DOB2 != "1962-03-20" || a.AsOfNow != "2024-01-01" {
		t.Errorf("actuarial dates: dob1=%q dob2=%q now=%q", a.DOB1, a.DOB2, a.AsOfNow)
	}
	if math.Abs(a.POD-25000) > 0.005 {
		t.Errorf("actuarial pod = %.2f, want 25000", a.POD)
	}
	if len(a.Table1) != 4 || a.Table1[0][0] != 20 || math.Abs(a.Table1[1][1]-0.0012) > 1e-12 {
		t.Errorf("actuarial table1 mismapped: %+v", a.Table1)
	}
	if len(a.Table2) != 3 || a.Table2[0][0] != 20 {
		t.Errorf("actuarial table2 mismapped: %+v", a.Table2)
	}
	t.Logf("PV variable-rate schedule (True/Loan/Yield→continuous) and actuarial config (2-life table + POD) request mapping verified")
}

// jsLit renders s as a JSON string literal for safe inlining into a node harness.
func jsLit(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// mustJSON marshals v to a JSON string for inlining into a node harness.
func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
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
