// r61_datesnap_test.js — an off-grid payment date must SAY it was moved.
// Client UI item #9. Run against the REAL server:
//     go build -o /tmp/persense ./cmd/persense && /tmp/persense -port 8821
//     node testplan/harness/r61_datesnap_test.js http://127.0.0.1:8821
// See r61_autocalc_test.js's header for the Tailwind note — same technique.
const { chromium } = require('playwright');
const fs = require('fs');
const BASE = process.argv[2] || 'http://127.0.0.1:8821';
const TW = process.env.TW || '/tmp/tw.js';
let pass = 0, fail = 0; const fails = [];
function ck(c, m) { if (c) pass++; else { fail++; fails.push(m); } }

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.PW_CHROME || '/opt/pw-browsers/chromium-1194/chrome-linux/chrome' });
  const page = await browser.newPage({ viewport: { width: 1500, height: 1050 } });
  if (fs.existsSync(TW)) {
    await page.route('https://cdn.tailwindcss.com**', r =>
      r.fulfill({ status: 200, contentType: 'application/javascript', body: fs.readFileSync(TW) }));
  }
  await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
  await page.evaluate(() => { try { localStorage.setItem('persense-tour-done', '1'); } catch (_) {} });

  // Each scenario starts from a CLEAN browser: this page persists every field
  // to localStorage and restores it on load, so without the wipe below a value
  // from the previous scenario leaks in and the assertions measure that instead
  // (it happened on the first run — a stale 12/01/2056 made every case "snap").
  const ALL = ['amz-amount', 'amz-rate', 'amz-perYr', 'amz-loanDate',
               'amz-firstDate', 'amz-lastDate', 'amz-nPeriods', 'amz-payment'];
  async function scenario(fields) {
    await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
    await page.evaluate(() => {
      try {
        Object.keys(localStorage).forEach(k => { if (k !== 'persense-tour-done') localStorage.removeItem(k); });
      } catch (_) {}
    });
    await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
    await page.waitForTimeout(500);
    await page.evaluate((all) => {
      const o = document.getElementById('tour-overlay'); if (o) o.remove();
      showScreen('amortization');
      document.getElementById('set-autocalc').checked = false; // drive Calculate explicitly
      all.forEach(id => {
        const el = document.getElementById(id);
        if (el) { el.value = ''; el.classList.remove('cell-output'); }
      });
    }, ALL);
    // Type into date fields with real keystrokes so the input mask runs, exactly
    // as a user would; page.fill() writes the value without it.
    for (const [id, v] of Object.entries(fields)) {
      const isDate = await page.evaluate(i => {
        const el = document.getElementById(i);
        return !!(el && el.getAttribute('placeholder') === 'MM/DD/YYYY');
      }, id);
      // ⚠️ Type DIGITS ONLY into a masked date field. maskDateInput inserts the
      // slashes itself, so typing "01/01/2027" literally yields "01//01/202" and
      // the field fails validation — which is what the first run measured.
      // Clear immediately before typing: entering the loan date fires
      // maybeFillFirstPaymentDefault(), which SOFT-FILLS 1st Pmt Date, and
      // typing then appends to it ("02/01/202010120277" on the first run).
      await page.click('#' + id);
      await page.evaluate(i => {
        const el = document.getElementById(i);
        el.value = '';
        el.dispatchEvent(new Event('input', { bubbles: true }));
        el.focus();
      }, id);
      await page.type('#' + id, isDate ? v.replace(/\D/g, '') : v, { delay: 10 });
      await page.keyboard.press('Tab');
      await page.waitForTimeout(80);
    }
    const flagged = await page.evaluate(() => Array.from(
      document.querySelectorAll('#screen-amortization input.date-invalid'))
      .map(e => e.id + '="' + e.value + '"'));
    if (flagged.length) console.log('   [harness] date fields flagged invalid: ' + flagged.join(', '));
    // Fire and forget: awaiting the in-page promise races with any navigation
    // the page does and playwright reports it as a garbage-collected promise.
    await page.evaluate(() => { calcAmortization(); });
    await page.waitForTimeout(1800);
    return page.evaluate(() => ({
      err: document.getElementById('amz-error').textContent.trim(),
      last: document.getElementById('amz-lastDate').value,
      first: document.getElementById('amz-firstDate').value,
      periods: document.getElementById('amz-nPeriods').value,
    }));
  }

  const common = { 'amz-amount': '100000', 'amz-rate': '7.5', 'amz-perYr': '12',
                   'amz-loanDate': '01/01/2027', 'amz-firstDate': '01/01/2027' };

  // 1. THE CLIENT'S OWN CASE — it refuses, and the refusal must explain why.
  const r1 = await scenario(Object.assign({}, common, { 'amz-lastDate': '01/15/2027' }));
  ck(/at least two regular payments/i.test(r1.err),
     `the client's case no longer refuses; got: ${r1.err.slice(0, 120)}`);
  ck(/not on the payment schedule/i.test(r1.err),
     `a refusal caused by a snapped date did not say the date was moved: ${r1.err.slice(0, 200)}`);
  ck(/01\/01\/2027/.test(r1.err),
     `the note did not name the date the engine actually used: ${r1.err.slice(0, 200)}`);

  // 2. AN OFF-GRID DATE THAT STILL SOLVES — must compute AND say it moved.
  const r2 = await scenario(Object.assign({}, common, { 'amz-lastDate': '06/15/2027' }));
  ck(r2.last === '06/01/2027', `the field was not corrected to the scheduled date (got "${r2.last}")`);
  ck(r2.periods === '6', `wrong term after the snap (got "${r2.periods}")`);
  ck(/not on the payment schedule/i.test(r2.err),
     `a successful calc silently moved the date and said nothing: "${r2.err}"`);
  ck(/06\/15\/2027/.test(r2.err) && /06\/01\/2027/.test(r2.err),
     `the advisory must name BOTH dates; got: "${r2.err}"`);
  ck(/every month/i.test(r2.err), `the advisory should state the payment interval; got: "${r2.err}"`);

  // 3. NEGATIVE CONTROL — an ON-grid date must produce NO snap advisory.
  //    Without this, a note printed unconditionally would pass tests 1 and 2.
  const r3 = await scenario(Object.assign({}, common, { 'amz-lastDate': '06/01/2027' }));
  ck(r3.periods === '6', `on-grid control did not solve (periods "${r3.periods}")`);
  ck(!/not on the payment schedule/i.test(r3.err),
     `an ON-GRID date produced a snap advisory — the note fires unconditionally: "${r3.err}"`);

  // 4. NEGATIVE CONTROL — a derived last date (user left it blank) must not warn.
  const r4 = await scenario(Object.assign({}, common, { 'amz-nPeriods': '12' }));
  ck(r4.last === '12/01/2027', `derived last date wrong (got "${r4.last}")`);
  ck(!/not on the payment schedule/i.test(r4.err),
     `a DERIVED last date produced a snap advisory: "${r4.err}"`);

  await browser.close();
  console.log(`\nPASS ${pass}  FAIL ${fail}`);
  const show = parseInt(process.env.R61_SHOW || '25', 10);
  if (fail) { fails.slice(0, show).forEach(f => console.log('  ✗ ' + f)); }
  process.exit(fail ? 1 : 0);
})();
