// Browser+oracle differential harness — Mortgage screen.
// Drives the REAL web Mortgage grid (row 0) in headless chromium and compares
// against mtg_oracle (the genuine DOS Mortgage.Calc, headless).
//
// Rate convention: the UI's "Loan Rate" is the nominal monthly-compounded rate
// in percent; the oracle takes the TRUE (continuously-compounded) rate as a
// fraction — the same conversion the DOS GUI applies before Calc sees it:
// T = 12·ln(1 + loanPct/100/12).
//
// Reset: addInitScript(localStorage.clear) + reload per case (the app mirrors
// the worksheet to localStorage and restores on load). External requests are
// blocked (cdn.tailwindcss.com stalls 'load' ~13s in the sandbox).
//
// Run: node run_mtg.js
const { chromium } = require('playwright');
const { execFileSync } = require('child_process');
const fs = require('fs');

const ORACLE = '/tmp/oraclebuild/mtg_oracle';
const URL = 'http://localhost:8099/';
const T = pct => 12 * Math.log(1 + pct / 100 / 12); // loan% -> true rate fraction
const T10 = pct => T(pct).toFixed(10);

// ui: fields for row 0 (values as the user would type them).
// oracle: argv, or {twoStep:'apr', ...} for the APR cases.
// expect: [[uiField, oracleKey, tol], ...]  (uiField 'apr' reads the APR cell)
const cases = [
  { id: 'MTG-01', title: 'Solve Monthly (basic 30-yr)',
    ui: { price: '200000', pctDown: '20', years: '30', rate: '7' },
    oracle: ['monthly', '200000', '0.20', '30', T10(7)],
    expect: [['monthly', 'monthly', 0.02], ['cash', 'cash', 0.02], ['financed', 'financed', 0.02]] },
  { id: 'MTG-02', title: 'Monthly with points + tax (help Ex 1)',
    ui: { price: '200000', points: '2', pctDown: '20', years: '20', rate: '8', tax: '200' },
    oracle: ['taxmonthly', '200000', '0.20', '20', T10(8), '200', '0.02'],
    expect: [['monthly', 'monthly', 0.02], ['cash', 'cash', 0.02], ['financed', 'financed', 0.02]] },
  { id: 'MTG-03', title: 'Monthly with a known balloon',
    ui: { price: '200000', pctDown: '20', years: '30', rate: '7', balloonYears: '5', balloonAmount: '50000' },
    oracle: ['monthly', '200000', '0.20', '30', T10(7), '0', '5', '50000'],
    expect: [['monthly', 'monthly', 0.02]] },
  { id: 'MTG-04', title: 'Solve Price from Monthly (% Down funding)',
    ui: { pctDown: '20', years: '30', rate: '6.5', monthly: '1400' },
    oracle: ['price', '0.20', '30', T10(6.5), '1400'],
    expect: [['price', 'price', 0.02], ['cash', 'cash', 0.02], ['financed', 'financed', 0.02]] },
  { id: 'MTG-05', title: 'Price solve with tax netted out',
    ui: { pctDown: '20', years: '30', rate: '6.5', monthly: '1400', tax: '300' },
    oracle: ['taxprice', '0.20', '30', T10(6.5), '1400', '300'],
    expect: [['price', 'price', 0.02]] },
  { id: 'MTG-06', title: 'Solve balloon Amount (When given, HowMuch blank)',
    ui: { price: '200000', pctDown: '20', years: '30', rate: '7', monthly: '1200', balloonYears: '5' },
    oracle: ['solvehowmuch', '200000', '0.20', '30', T10(7), '1200', '5'],
    expect: [['balloonAmount', 'howmuch', 0.5]] },
  { id: 'MTG-07', title: 'Fund by Cash Required (pct derives)',
    ui: { price: '350000', cash: '70000', years: '30', rate: '6.5' },
    oracle: ['monthly', '350000', '0.20', '30', T10(6.5)],
    expect: [['monthly', 'monthly', 0.02], ['financed', 'financed', 0.02]] },
  { id: 'MTG-08', title: 'Fund by Amt Borrowed (pct derives)',
    ui: { price: '250000', financed: '200000', years: '30', rate: '6' },
    oracle: ['monthly', '250000', '0.20', '30', T10(6)],
    expect: [['monthly', 'monthly', 0.02], ['cash', 'cash', 0.02]] },
  { id: 'MTG-09', title: 'Zero down typed explicitly',
    ui: { price: '150000', pctDown: '0', years: '30', rate: '7' },
    oracle: ['monthly', '150000', '0', '30', T10(7)],
    expect: [['monthly', 'monthly', 0.02]] },
  { id: 'MTG-10', title: 'Affordability: price from cash + points + tax',
    ui: { cash: '56000', points: '1.5', years: '30', rate: '8.5', tax: '200', monthly: '1650' },
    oracle: ['taxpricecash', '56000', '30', T10(8.5), '1650', '200', '0.015'],
    expect: [['price', 'price', 0.02]] },
  // APR: two-step — solve the monthly first, then ask the oracle for the APR
  // of that financed/monthly pair (the DOS ReportAPR path).
  { id: 'MTG-11', title: 'APR with 2 points > loan rate',
    ui: { price: '200000', pctDown: '20', points: '2', years: '30', rate: '7' },
    oracle: ['monthly', '200000', '0.20', '30', T10(7)],
    aprStep: m => ['aprfin', '160000', String(m), '30', T10(7), '0.02'],
    expect: [['monthly', 'monthly', 0.02], ['apr', 'apr', 5e-5]] },
  { id: 'MTG-12', title: 'APR with no points ≈ loan rate',
    ui: { price: '200000', pctDown: '20', years: '30', rate: '7' },
    oracle: ['monthly', '200000', '0.20', '30', T10(7)],
    aprStep: m => ['aprfin', '160000', String(m), '30', T10(7), '0'],
    expect: [['monthly', 'monthly', 0.02], ['apr', 'apr', 5e-5]] },
];

function runOracle(argv) {
  let out = '';
  try { out = execFileSync(ORACLE, argv, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], timeout: 20000 }); }
  catch (e) { out = (e.stdout || '') + '\nERR spawn'; }
  const r = { raw: out.trim().split('\n').pop() };
  let m;
  if ((m = out.match(/monthly ([\d.eE+-]+) price ([\d.eE+-]+) cash ([\d.eE+-]+) financed ([\d.eE+-]+)/)))
    Object.assign(r, { monthly: +m[1], price: +m[2], cash: +m[3], financed: +m[4] });
  if ((m = out.match(/howmuch ([\d.eE+-]+)/))) r.howmuch = +m[1];
  if ((m = out.match(/^apr ([\d.eE+-]+)/m))) r.apr = +m[1];
  if (/^ERR/m.test(out)) r.err = (out.match(/ERR (.*)/) || [, out])[1];
  return r;
}

async function runUI(page, c) {
  await page.reload({ waitUntil: 'load' });
  await page.waitForFunction(() => typeof calcMortgageRow === 'function');
  await page.evaluate(() => showScreen('mortgage'));
  return await page.evaluate(async (ui) => {
    const set = (field, v) => {
      const cell = document.querySelector(`#mtg-body input[data-row="0"][data-field="${field}"]`);
      if (!cell) return 'MISSING:' + field;
      cell.value = v;
      // Route through the app's own edit hook so mtgStatus marks the cell
      // 'input' (the collector only sends 'input' cells — that status
      // machinery is exactly what makes mortgage recalcs stale-proof).
      if (typeof onMtgCellEdit === 'function') onMtgCellEdit(0, field);
      cell.dispatchEvent(new Event('input', { bubbles: true }));
      return 'ok';
    };
    Object.entries(ui).forEach(([f, v]) => set(f, v));
    let threw = null;
    try { await calcMortgageRow(); } catch (e) { threw = e.message; }
    const read = f => {
      const cell = document.querySelector(`#mtg-body input[data-row="0"][data-field="${f}"]`);
      return cell ? cell.value : null;
    };
    const aprCell = document.querySelector('#mtg-body input[data-row="0"][data-field="apr"]');
    const snap1 = {
      monthly: read('monthly'), price: read('price'), cash: read('cash'),
      financed: read('financed'), pctDown: read('pctDown'),
      balloonAmount: read('balloonAmount'),
      apr: aprCell ? aprCell.value : null,
    };
    // Recalc untouched — the status machinery must keep this idempotent.
    try { await calcMortgageRow(); } catch (e) { /* keep first */ }
    const snap2 = {
      monthly: read('monthly'), price: read('price'), cash: read('cash'),
      financed: read('financed'), pctDown: read('pctDown'),
      balloonAmount: read('balloonAmount'),
      apr: aprCell ? aprCell.value : null,
    };
    const err = (document.getElementById('mtg-error') || {}).textContent.trim();
    return { threw, err, snap1, snap2 };
  }, c.ui);
}

const money = s => s == null ? NaN : parseFloat(String(s).replace(/[$,%]/g, '').replace(/,/g, ''));

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1400, height: 900 } });
  page.on('dialog', d => d.accept().catch(() => {}));
  await page.addInitScript(() => { try { localStorage.clear(); } catch (e) {} });
  await page.route('**/*', route =>
    route.request().url().startsWith(URL) ? route.continue() : route.abort());
  await page.goto(URL, { waitUntil: 'load' });

  const results = [];
  for (const c of cases) {
    const rec = { id: c.id, title: c.title };
    try {
      const ui = await runUI(page, c);
      const ora = runOracle(c.oracle);
      if (c.aprStep && ora.monthly != null) {
        const apr = runOracle(c.aprStep(ora.monthly.toFixed(6)));
        ora.apr = apr.apr; ora.aprErr = apr.err;
      }
      rec.ui = ui; rec.oracle = ora;
      const checks = [];
      if (ora.err) {
        checks.push({ k: 'both-refuse', ok: !!(ui.err || ui.threw), detail: `oracle="${ora.err}" ui="${ui.err}"` });
      } else {
        for (const [uf, ok_, tol] of c.expect) {
          let uiVal = money(ui.snap1[uf]);
          let oraVal = ora[ok_];
          if (uf === 'apr') uiVal = uiVal / 100; // APR cell shows percent
          checks.push({ k: uf, ok: Math.abs(uiVal - oraVal) <= tol, detail: `ui=${uiVal} oracle=${oraVal}` });
        }
        // Idempotency: second Calculate Row must not change any cell.
        const drift = Object.keys(ui.snap1).filter(k => ui.snap1[k] !== ui.snap2[k]);
        checks.push({ k: 'idempotent', ok: drift.length === 0, detail: drift.map(k => `${k}: ${ui.snap1[k]} -> ${ui.snap2[k]}`).join('; ') || 'stable' });
      }
      rec.checks = checks;
      rec.pass = checks.length > 0 && checks.every(x => x.ok);
    } catch (e) { rec.error = e.message; rec.pass = false; }
    results.push(rec);
    const status = rec.error ? 'ERR ' : rec.pass ? 'PASS' : 'FAIL';
    console.log(`${status} ${rec.id.padEnd(8)} ${rec.title}`);
    if (!rec.pass && rec.checks) rec.checks.filter(x => !x.ok).forEach(x => console.log(`      ✗ ${x.k}: ${x.detail}`));
    if (!rec.pass && rec.ui && (rec.ui.err || rec.ui.threw)) console.log(`      ui.err="${rec.ui.err}" threw=${rec.ui.threw}`);
  }
  fs.writeFileSync(__dirname + '/mtg_results.json', JSON.stringify(results, null, 2));
  const pass = results.filter(r => r.pass).length;
  console.log(`\n=== MTG: ${pass}/${results.length} pass ===`);
  await browser.close();
})().catch(e => { console.error('HARNESS FAIL:', e); process.exit(1); });
