// Browser+oracle differential harness — Present Value screen.
// Drives the REAL web PV UI (headless chromium) and compares each result to the
// pv_oracle binary (the genuine DOS PV engine, headless). The oracle's as-of is
// fixed at 2024-01-01 with day-1 dates, which is the UI default too, so cases
// are expressed on that grid. The oracle RATE is the CONTINUOUS (true) rate —
// entered in the UI's "True Rate %" box (which is that same convention).
//
// Reset strategy: page.reload() per case — the cleanest possible state
// (defaults restored, 4 blank rows, settings back to default), immune to the
// stale-state class we hunted this week.
//
// Run: node run_pv.js
const { chromium } = require('playwright');
const { execFileSync } = require('child_process');
const fs = require('fs');

const ORACLE = '/tmp/oraclebuild/pv_oracle';
const URL = 'http://localhost:8099/';

// months after 2024-01-01 -> MM/01/YYYY (zero-padded)
function mAfter(k) {
  const m = (k % 12 + 12) % 12, carry = Math.floor(k / 12);
  const mm = String(m + 1).padStart(2, '0');
  return `${mm}/01/${2024 + carry}`;
}

// ---- case table -----------------------------------------------------------
// ui: { settings?, rateTrue?, clearRates?, asOf?, total?, lumps:[{date,amount,value}], pers:[{from,to,perYr,amount,cola,value}], vr?:[{date,trueRate}] }
// oracle: argv array. expect: which oracle output to compare to which UI cell.
const cases = [
  // ---- Group A: forward ----
  { id: 'PV-F-01', title: 'Basic lump 29mo @6%', ui: { rateTrue: 6, lumps: [{ date: mAfter(29), amount: '50000' }] },
    oracle: ['lump', '50000', '0.06', '29'], expect: 'total' },
  { id: 'PV-F-02', title: 'Lump on as-of date (identity)', ui: { rateTrue: 6, lumps: [{ date: mAfter(0), amount: '10000' }] },
    oracle: ['lump', '10000', '0.06', '0'], expect: 'total' },
  { id: 'PV-F-03', title: 'Lump BEFORE as-of (accumulation)', ui: { rateTrue: 6, lumps: [{ date: mAfter(-10), amount: '25000' }] },
    oracle: ['lump_gen', '25000', '0.06', '-10', '1', '1', '1'], expect: 'total' },
  { id: 'PV-F-04', title: 'Monthly annuity 36 pmts', ui: { rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(36), perYr: '12', amount: '1000' }] },
    oracle: ['periodic', '1000', '0.06', '12', '36'], expect: 'total' },
  { id: 'PV-F-05', title: 'Quarterly 12 pmts', ui: { rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(36), perYr: '4', amount: '2500' }] },
    oracle: ['periodic', '2500', '0.06', '4', '12'], expect: 'total' },
  { id: 'PV-F-06', title: 'Annual 4 pmts', ui: { rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(48), perYr: '1', amount: '12000' }] },
    oracle: ['periodic', '12000', '0.06', '1', '4'], expect: 'total' },
  { id: 'PV-F-07', title: 'Semi-annual 6 pmts', ui: { rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(36), perYr: '2', amount: '5000' }] },
    oracle: ['periodic', '5000', '0.06', '2', '6'], expect: 'total' },
  { id: 'PV-F-08', title: 'Mixed 2 lumps + 2 periodics', ui: { rateTrue: 6,
      lumps: [{ date: mAfter(17), amount: '15000' }, { date: mAfter(29), amount: '15000' }],
      pers: [{ from: mAfter(0), to: mAfter(24), perYr: '12', amount: '750' }, { from: mAfter(0), to: mAfter(36), perYr: '4', amount: '3000' }] },
    oracle: ['multi', '0.06', 'l17=15000', 'l29=15000', 'p750:12:24', 'p3000:4:12'], expect: 'total' },
  { id: 'PV-F-09', title: 'Negative lump (outflow) + positive', ui: { rateTrue: 6,
      lumps: [{ date: mAfter(17), amount: '-20000' }, { date: mAfter(29), amount: '30000' }] },
    oracle: ['multi', '0.06', 'l17=-20000', 'l29=30000'], expect: 'total' },
  { id: 'PV-F-10', title: 'COLA 3% anniversary', ui: { rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(36), perYr: '12', amount: '1000', cola: '3' }] },
    oracle: ['periodic', '1000', '0.06', '12', '36', '0.03'], expect: 'total' },
  { id: 'PV-F-11', title: 'COLA 3% continuous', ui: { settings: { 'set-colaMonth': 'continuous' }, rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(36), perYr: '12', amount: '1000', cola: '3' }] },
    oracle: ['periodic', '1000', '0.06', '12', '36', '0.03', 'cnt'], expect: 'total' },
  { id: 'PV-F-12', title: 'COLA 3% January-stepped', ui: { settings: { 'set-colaMonth': '1' }, rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(36), perYr: '12', amount: '1000', cola: '3' }] },
    oracle: ['periodic', '1000', '0.06', '12', '36', '0.03', '1'], expect: 'total' },
  { id: 'PV-F-13', title: '365 basis lump', ui: { settings: { 'set-basis': '365' }, rateTrue: 6, lumps: [{ date: mAfter(29), amount: '50000' }] },
    oracle: ['lump_gen', '50000', '0.06', '29', '1', '1', '0'], expect: 'total' },
  { id: 'PV-F-14', title: '365 basis periodic', ui: { settings: { 'set-basis': '365' }, rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(36), perYr: '12', amount: '1000' }] },
    oracle: ['periodic_gen', '1000', '0.06', '12', '36', '0', '1', '1', '0'], expect: 'total' },
  { id: 'PV-F-15', title: '30-yr monthly precision', ui: { rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(360), perYr: '12', amount: '1500' }] },
    oracle: ['periodic', '1500', '0.06', '12', '360'], expect: 'total' },
  { id: 'PV-F-16', title: 'Zero rate forward', ui: { rateTrue: 0, lumps: [{ date: mAfter(29), amount: '50000' }] },
    oracle: ['lump', '50000', '0', '29'], expect: 'total' },
  { id: 'PV-F-17', title: 'Periodic starting BEFORE as-of (accumulate leg)', ui: { rateTrue: 6, pers: [{ from: mAfter(-12), to: mAfter(12), perYr: '12', amount: '1000' }] },
    oracle: ['periodic_off', '1000', '0.06', '12', '24', '12'], expect: 'total' },
  // ---- Group VR: variable-rate schedule ----
  { id: 'PV-V-01', title: 'VR lump: 5% then 7% from 2026', ui: { vr: [{ trueRate: '5' }, { date: '01/01/2026', trueRate: '7' }], lumps: [{ date: mAfter(48), amount: '50000' }] },
    oracle: ['vr', '50000', '48', '2', '2024', '0.05', '2026', '0.07'], expect: 'total' },
  { id: 'PV-V-02', title: 'VR periodic: 3-step schedule', ui: { vr: [{ trueRate: '4' }, { date: '01/01/2025', trueRate: '6' }, { date: '01/01/2027', trueRate: '5' }], pers: [{ from: mAfter(0), to: mAfter(48), perYr: '12', amount: '1000' }] },
    oracle: ['vrp', '1000', '12', '48', '0', '3', '2024', '0.04', '2025', '0.06', '2027', '0.05'], expect: 'total' },
  // ---- Group B: backward solves ----
  { id: 'PV-S-01', title: 'Solve lump Amount from row Value', ui: { rateTrue: 6, lumps: [{ date: mAfter(29), value: '44000' }] },
    oracle: ['bk_lump_amt', '44000', '0.06', '29'], expect: 'lumpAmount' },
  { id: 'PV-S-02', skipIdem: true, title: 'Solve lump Date', ui: { rateTrue: 6, lumps: [{ amount: '50000', value: '44000' }] },
    oracle: ['bk_lump_date', '44000', '50000', '0.06', '29'], expect: 'lumpDate' },
  { id: 'PV-S-03', title: 'Solve periodic Amount', ui: { rateTrue: 6, pers: [{ from: mAfter(0), to: mAfter(36), perYr: '12', value: '33000' }] },
    oracle: ['bk_per_amt', '33000', '0.06', '12', '36'], expect: 'perAmount' },
  { id: 'PV-S-04', skipIdem: true, title: 'Solve periodic Through date', ui: { rateTrue: 6, pers: [{ from: mAfter(0), perYr: '12', amount: '1000', value: '33000' }] },
    oracle: ['bk_per_todate', '33000', '1000', '0.06', '12', '36'], expect: 'perToDate' },
  { id: 'PV-S-05', skipIdem: true, title: 'Solve periodic From date', ui: { rateTrue: 6, pers: [{ to: mAfter(36), perYr: '12', amount: '1000', value: '20000' }] },
    oracle: ['bk_per_fromdate', '20000', '1000', '0.06', '12', '36'], expect: 'perFromDate' },
  { id: 'PV-S-06', title: 'Solve rate (IRR), lump', ui: { clearRates: true, total: '44000', lumps: [{ date: mAfter(29), amount: '50000' }] },
    oracle: ['bk_rate', '44000', '50000', '29'], expect: 'rate' },
  { id: 'PV-S-07', title: 'Solve As-of date, lump', ui: { rateTrue: 6, asOf: '', total: '44000', lumps: [{ date: mAfter(29), amount: '50000' }] },
    oracle: ['bk_asof', '44000', '50000', '0.06', '29'], expect: 'asOf' },
];

// ---- oracle ---------------------------------------------------------------
function runOracle(argv) {
  let out = '';
  try { out = execFileSync(ORACLE, argv, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], timeout: 20000 }); }
  catch (e) { out = (e.stdout || '') + '\nERR spawn'; }
  const r = { raw: out.trim().split('\n')[0] };
  let m;
  if ((m = out.match(/^pv ([\d.eE+-]+)/m))) r.pv = +m[1];
  if ((m = out.match(/^amt ([\d.eE+-]+)/m))) r.amt = +m[1];
  if ((m = out.match(/^rate ([\d.eE+-]+)/m))) r.rate = +m[1];
  if ((m = out.match(/^date (\d+) (\d+) (\d+)/m))) r.date = { y: +m[1] + 1900, mo: +m[2], d: +m[3] };
  if ((m = out.match(/^asof (\d+) (\d+) (\d+)/m))) r.asof = { y: +m[1] + 1900, mo: +m[2], d: +m[3] };
  if (/^ERR/m.test(out)) r.err = (out.match(/ERR (.*)/) || [, out])[1];
  return r;
}

// ---- UI -------------------------------------------------------------------
async function runUI(page, c) {
  // True factory reset. The app mirrors the whole worksheet to localStorage,
  // restores it on load, AND re-saves on a ~400ms debounce — so clearing from
  // the old page loses the race with the pending save. The addInitScript
  // registered in main() clears storage BEFORE any page script runs, so every
  // reload starts from pristine defaults.
  await page.reload({ waitUntil: 'load' });
  await page.waitForFunction(() => typeof calcPV === 'function');
  await page.evaluate(() => showScreen('presentvalue'));
  return await page.evaluate(async (ui) => {
    const fire = el => { el.dispatchEvent(new Event('input', { bubbles: true })); el.dispatchEvent(new Event('change', { bubbles: true })); };
    const setv = (sel, v) => { const e = document.querySelector(sel); if (!e) return false; e.value = v; if (e.classList) e.classList.remove('cell-output'); fire(e); return true; };

    // settings
    Object.entries(ui.settings || {}).forEach(([id, v]) => { const e = document.getElementById(id); if (e) { e.value = v; fire(e); } });

    // rate boxes
    if (ui.clearRates) {
      ['pv-rate-true', 'pv-rate-loan', 'pv-rate-yield'].forEach(id => setv('#' + id, ''));
      const hidden = document.getElementById('pv-rate'); if (hidden) hidden.value = '';
    } else if (ui.rateTrue != null) {
      setv('#pv-rate-true', String(ui.rateTrue));
    }
    if (ui.asOf != null) setv('#pv-asOfDate', ui.asOf);
    if (ui.total != null) setv('#pv-total', ui.total);

    // rows
    (ui.lumps || []).forEach((row, i) => {
      if (row.date != null) setv(`input[data-ls="${i}"][data-f="date"]`, row.date);
      if (row.amount != null) setv(`input[data-ls="${i}"][data-f="amount"]`, row.amount);
      if (row.value != null) setv(`input[data-ls="${i}"][data-f="value"]`, row.value);
    });
    (ui.pers || []).forEach((row, i) => {
      if (row.from != null) setv(`input[data-per="${i}"][data-f="from"]`, row.from);
      if (row.to != null) setv(`input[data-per="${i}"][data-f="to"]`, row.to);
      if (row.perYr != null) setv(`input[data-per="${i}"][data-f="perYr"]`, row.perYr);
      if (row.amount != null) setv(`input[data-per="${i}"][data-f="amount"]`, row.amount);
      if (row.cola != null) setv(`input[data-per="${i}"][data-f="cola"]`, row.cola);
      if (row.value != null) setv(`input[data-per="${i}"][data-f="value"]`, row.value);
    });
    // variable-rate schedule
    (ui.vr || []).forEach((row, i) => {
      if (row.date != null) setv(`#pv-rateSched input[data-rs="${i}"][data-f="date"]`, row.date);
      if (row.trueRate != null) setv(`#pv-rateSched input[data-rs="${i}"][data-f="trueRate"]`, row.trueRate);
    });

    let threw = null;
    try { await calcPV(); } catch (e) { threw = e.message; }
    // second calc for idempotency
    let total1 = (document.getElementById('pv-total') || {}).value || '';
    try { await calcPV(); } catch (e) { /* keep first threw */ }

    const read = sel => { const e = document.querySelector(sel); return e ? e.value : null; };
    const err = (document.getElementById('pv-error') || {}).textContent.trim();
    return {
      threw, err,
      total: read('#pv-total'), total1,
      rateTrue: read('#pv-rate-true'),
      asOf: read('#pv-asOfDate'),
      ls0: { date: read('input[data-ls="0"][data-f="date"]'), amount: read('input[data-ls="0"][data-f="amount"]'), value: read('input[data-ls="0"][data-f="value"]') },
      per0: { from: read('input[data-per="0"][data-f="from"]'), to: read('input[data-per="0"][data-f="to"]'), amount: read('input[data-per="0"][data-f="amount"]'), value: read('input[data-per="0"][data-f="value"]') },
    };
  }, c.ui);
}

// ---- compare --------------------------------------------------------------
const money = s => s == null ? NaN : parseFloat(String(s).replace(/[$,]/g, ''));
const dateEq = (uiStr, o) => {
  if (!uiStr || !o) return false;
  const m = uiStr.match(/(\d+)\/(\d+)\/(\d+)/);
  return m && +m[1] === o.mo && +m[2] === o.d && +m[3] === o.y;
};

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1280, height: 950 } });
  page.on('dialog', d => d.accept().catch(() => {}));
  await page.addInitScript(() => { try { localStorage.clear(); } catch (e) {} });
  // The page pulls cdn.tailwindcss.com, which hangs ~13s in the sandbox until
  // its network timeout and stalls the 'load' event. Abort all external
  // requests — only localhost matters for computation.
  await page.route('**/*', route =>
    route.request().url().startsWith(URL) ? route.continue() : route.abort());
  await page.goto(URL, { waitUntil: 'load' });

  const results = [];
  for (const c of cases) {
    const rec = { id: c.id, title: c.title };
    try {
      const ui = await runUI(page, c);
      const ora = runOracle(c.oracle);
      rec.ui = ui; rec.oracle = ora;
      const checks = [];
      if (ora.err) {
        checks.push({ k: 'both-refuse', ok: !!(ui.err || ui.threw), detail: `oracle="${ora.err.slice(0, 60)}" ui.err="${ui.err}"` });
      } else {
        switch (c.expect) {
          case 'total':
            checks.push({ k: 'total', ok: Math.abs(money(ui.total) - ora.pv) <= 0.02, detail: `ui=${money(ui.total)} oracle=${ora.pv}` });
            break;
          case 'lumpAmount':
            checks.push({ k: 'solved-amount', ok: Math.abs(money(ui.ls0.amount) - ora.amt) <= 0.02, detail: `ui=${money(ui.ls0.amount)} oracle=${ora.amt}` });
            break;
          case 'lumpDate':
            checks.push({ k: 'solved-date', ok: dateEq(ui.ls0.date, ora.date), detail: `ui=${ui.ls0.date} oracle=${ora.date && `${ora.date.mo}/${ora.date.d}/${ora.date.y}`}` });
            break;
          case 'perAmount':
            checks.push({ k: 'solved-amount', ok: Math.abs(money(ui.per0.amount) - ora.amt) <= 0.02, detail: `ui=${money(ui.per0.amount)} oracle=${ora.amt}` });
            break;
          case 'perToDate':
            checks.push({ k: 'solved-to', ok: dateEq(ui.per0.to, ora.date), detail: `ui=${ui.per0.to} oracle=${ora.date && `${ora.date.mo}/${ora.date.d}/${ora.date.y}`}` });
            break;
          case 'perFromDate':
            checks.push({ k: 'solved-from', ok: dateEq(ui.per0.from, ora.date), detail: `ui=${ui.per0.from} oracle=${ora.date && `${ora.date.mo}/${ora.date.d}/${ora.date.y}`}` });
            break;
          case 'rate': {
            const uiFrac = money(ui.rateTrue) / 100;
            checks.push({ k: 'solved-rate', ok: Math.abs(uiFrac - ora.rate) <= 5e-5, detail: `ui=${uiFrac} oracle=${ora.rate}` });
            break;
          }
          case 'asOf':
            checks.push({ k: 'solved-asof', ok: dateEq(ui.asOf, ora.asof), detail: `ui=${ui.asOf} oracle=${ora.asof && `${ora.asof.mo}/${ora.asof.d}/${ora.asof.y}`}` });
            break;
        }
        // Idempotency: a second Calculate must not drift the total. Date
        // solves are exempt: the solved date is day/period-granular, so the
        // follow-up forward pass legitimately lands near (not on) the target
        // — DOS does the same on re-Enter. For those the stability assertion
        // is the solved DATE itself, which is read AFTER the second calc and
        // compared to the oracle above.
        if (!c.skipIdem && ui.total && ui.total1) checks.push({ k: 'idempotent', ok: ui.total === ui.total1, detail: `calc1=${ui.total1} calc2=${ui.total}` });
      }
      rec.checks = checks;
      rec.pass = checks.length > 0 && checks.every(x => x.ok);
    } catch (e) { rec.error = e.message; rec.pass = false; }
    results.push(rec);
    const status = rec.error ? 'ERR ' : rec.pass ? 'PASS' : 'FAIL';
    console.log(`${status} ${rec.id.padEnd(9)} ${rec.title}`);
    if (!rec.pass && rec.checks) rec.checks.filter(x => !x.ok).forEach(x => console.log(`      ✗ ${x.k}: ${x.detail}`));
    if (!rec.pass && rec.ui && (rec.ui.err || rec.ui.threw)) console.log(`      ui.err="${rec.ui.err}" threw=${rec.ui.threw}`);
    if (rec.error) console.log(`      ERROR ${rec.error}`);
  }
  fs.writeFileSync(__dirname + '/pv_results.json', JSON.stringify(results, null, 2));
  const pass = results.filter(r => r.pass).length;
  console.log(`\n=== PV: ${pass}/${results.length} pass ===`);
  await browser.close();
})().catch(e => { console.error('HARNESS FAIL:', e); process.exit(1); });
