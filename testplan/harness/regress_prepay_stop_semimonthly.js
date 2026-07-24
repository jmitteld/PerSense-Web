// Regression: derived prepayment Stop Date must handle 24/yr (semi-monthly) and
// not go stale when Pmts/Yr changes. 2026-07-24 client report — a prepay row
// showed #Pmts 107 @ 24/yr with Stop 11/01/2034 (the 12/yr answer): the
// frontend addPeriodsForFreq() returned null for 24/yr (12 % 24 !== 0), so a
// Stop derived at 12/yr never re-derived when Pmts/Yr flipped to 24. Fixed by
// adding a semi-monthly case (±15-day stepping mirroring dateutil.AddPeriod) and
// clearing a stale derived Stop when it can't be recomputed. See discrepancies §42.
//
// Run: node regress_prepay_stop_semimonthly.js   (server on :8099)
const { chromium } = require('playwright');
const URL = 'http://localhost:8099/';

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  page.on('dialog', d => d.accept().catch(() => {}));
  await page.route('**/*', r => r.request().url().startsWith(URL) ? r.continue() : r.abort());
  await page.goto(URL, { waitUntil: 'load' });
  await page.evaluate(() => showScreen('amortization'));

  const fails = [];
  const check = (name, got, want) => {
    if (got !== want) { fails.push(`${name}: got ${got}, want ${want}`); console.log(`  FAIL ${name}: ${got} != ${want}`); }
    else console.log(`  ok   ${name}: ${got}`);
  };

  // Pure function coverage: addPeriodsForFreq for every supported frequency.
  const f = (perYr, n) => page.evaluate(({ perYr, n }) => addPeriodsForFreq('2026-01-01', perYr, n), { perYr, n });
  check('addPeriodsForFreq(12,106)', await f(12, 106), '2034-11-01');
  check('addPeriodsForFreq(24,106)', await f(24, 106), '2030-06-01');   // was null (bug)
  check('addPeriodsForFreq(24,3)', await f(24, 3), '2026-02-16');
  check('addPeriodsForFreq(4,3)', await f(4, 3), '2026-10-01');

  // End-to-end: #Pmts=107, flip Pmts/Yr 12 -> 24, the derived Stop must update.
  const setup = async () => page.evaluate(() => {
    const setv = (id, v) => { const e = document.getElementById(id); if (e) { e.value = v; if (e.classList) e.classList.remove('cell-output'); e.dispatchEvent(new Event('input', { bubbles: true })); } };
    document.querySelectorAll('#amz-prepay-body [data-amz-prepay-field]').forEach(e => { e.value = ''; if (e.classList) e.classList.remove('cell-output'); });
    setv('amz-amount', '100000'); setv('amz-rate', '8'); setv('amz-payment', '');
    setv('amz-perYr', '12'); setv('amz-loanDate', '01/01/2025'); setv('amz-firstDate', '02/01/2025'); setv('amz-nPeriods', '360');
    const pr = document.querySelectorAll('#amz-prepay-body tr')[0];
    const put = (k, v) => { const c = pr.querySelector('[data-amz-prepay-field="' + k + '"]'); if (c) { c.value = v; c.dispatchEvent(new Event('input', { bubbles: true })); } };
    put('startDate', '01/01/2026'); put('perYr', '12'); put('amount', '500'); put('nPmts', '107');
  });
  const stop = () => page.evaluate(() => (document.querySelector('#amz-prepay-body [data-amz-prepay-field="stopDate"]') || {}).value);
  await setup();
  await page.evaluate(() => calcAmortization()); await page.waitForTimeout(80);
  check('12/yr derived stop', await stop(), '11/01/2034');
  await page.evaluate(() => { const c = document.querySelector('#amz-prepay-body [data-amz-prepay-field="perYr"]'); c.value = '24'; c.dispatchEvent(new Event('input', { bubbles: true })); });
  await page.evaluate(() => calcAmortization()); await page.waitForTimeout(80);
  check('after flip->24, stop re-derives', await stop(), '06/01/2030');   // was stale 11/01/2034

  console.log(fails.length ? `\nFAILED (${fails.length})` : '\nPASS');
  await browser.close();
  process.exit(fails.length ? 1 : 0);
})().catch(e => { console.error('HARNESS FAIL', e); process.exit(1); });
