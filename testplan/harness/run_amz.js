// Browser+oracle differential harness — Amortization screen.
// Drives the REAL web UI (headless chromium) against a local server and
// compares each result to the amort_oracle binary spawned in-process.
//
// Robustness: (1) a dialog handler accepts the clear-confirm; (2) an EXHAUSTIVE
// per-case reset wipes every field/grid/setting (not trusting clearAmortization,
// whose confirm() aborts under automation) so there is zero state carryover;
// (3) oracle setting-flags are DERIVED from the same effective settings applied
// to the UI, so the UI default (prepaid=YES, arrears, 360) is always mirrored.
const { chromium } = require('playwright');
const { execFileSync } = require('child_process');
const fs = require('fs');

const ORACLE = '/tmp/oraclebuild/amort_oracle';
const URL = 'http://localhost:8099/';
const cases = JSON.parse(fs.readFileSync(__dirname + '/amz_cases.json', 'utf8'));

const FLAG_TOKENS = new Set(['b365', 'b365_360', 'exact', 'inadv', 'r78', 'prepaid', 'plusreg']);
function kick(r) { return r * 365 / 360; } // amort 365/360 kicker (docs §28)

function effSettings(c) {
  return Object.assign({ basis: '360', prepaid: 'yes', timing: 'arrears', exact: 'no', rule78: 'no', balloonIncl: 'no' }, c.settings || {});
}

function oracleArgs(c) {
  const s = effSettings(c);
  // Strip any embedded flag tokens; re-derive from settings.
  let args = c.oracle.filter(a => !FLAG_TOKENS.has(a));
  // Handle the rate (index 1): explicit KICK: marker, or auto-kick on 365/360 forward.
  if (args[1] && args[1].startsWith('KICK:')) args[1] = kick(parseFloat(args[1].slice(5))).toFixed(10);
  else if (s.basis === '365/360' && c.solve !== 'rate' && /^[\d.]+$/.test(args[1]) && parseFloat(args[1]) > 0) {
    args[1] = kick(parseFloat(args[1])).toFixed(10);
  }
  // Derived setting flags.
  if (s.basis === '365') args.push('b365');
  if (s.basis === '365/360') args.push('b365_360');
  if (s.prepaid === 'yes') args.push('prepaid');
  if (s.timing === 'advance') args.push('inadv');
  if (s.exact === 'yes') args.push('exact');
  if (s.rule78 === 'yes') args.push('r78');
  if (s.balloonIncl === 'yes') args.push('plusreg');
  return args;
}

function runOracle(c) {
  let out = '';
  try { out = execFileSync(ORACLE, oracleArgs(c), { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }); }
  catch (e) { out = (e.stdout || '') + '\nERR exit'; }
  const res = { raw: out.trim(), args: oracleArgs(c) };
  let m;
  if ((m = out.match(/payment ([\d.]+) interest ([\d.-]+) paid ([\d.-]+)/))) { res.payment = +m[1]; res.interest = +m[2]; res.paid = +m[3]; }
  if ((m = out.match(/solvedamount ([\d.-]+)/))) res.solvedamount = +m[1];
  if ((m = out.match(/solvedrate ([\d.-]+)/))) res.solvedrate = +m[1];
  if ((m = out.match(/balloon ([\d.-]+)/))) res.solvedballoon = +m[1];
  if (/^ERR|ENGINE ERROR/m.test(out)) res.err = (out.match(/(?:ERR|ENGINE ERROR:?) ?(.*)/) || [, out])[1].trim();
  return res;
}

async function fillAndCalc(page, c) {
  return await page.evaluate(async (c) => {
    // ---- exhaustive reset (do NOT rely on clearAmortization's confirm) ----
    const setv = (id, v) => { const e = document.getElementById(id); if (e) { e.value = v; if (e.classList) e.classList.remove('cell-output'); } };
    ['amz-amount', 'amz-loanDate', 'amz-rate', 'amz-firstDate', 'amz-nPeriods', 'amz-lastDate', 'amz-payment', 'amz-moratorium', 'amz-targetAmt', 'amz-skipMonths', 'amz-payoff-date', 'amz-payoff-bal'].forEach(id => setv(id, ''));
    setv('amz-perYr', '12'); setv('amz-points', '0');
    document.querySelectorAll('#amz-balloon-body [data-amz-balloon-field], #amz-prepay-body [data-amz-prepay-field], #amz-adj-body [data-amz-adj-field]').forEach(e => { e.value = ''; if (e.classList) e.classList.remove('cell-output'); });
    // settings to defaults
    const setSel = (id, v) => { const e = document.getElementById(id); if (e) e.value = v; };
    setSel('set-basis', '360'); setSel('set-prepaid', 'yes'); setSel('set-timing', 'arrears');
    setSel('set-balloonIncl', 'no'); setSel('set-exact', 'no'); setSel('set-rule78', 'no');
    if (typeof amzScheduleData !== 'undefined') amzScheduleData = null;

    // ---- apply the case ----
    const f = c.fields;
    setv('amz-amount', f.amount || ''); setv('amz-loanDate', f.loanDate || ''); setv('amz-rate', f.rate || '');
    setv('amz-firstDate', f.firstDate || ''); setv('amz-nPeriods', f.nPeriods || ''); setv('amz-lastDate', f.lastDate || '');
    setv('amz-perYr', f.perYr || '12'); setv('amz-payment', f.payment || ''); setv('amz-points', f.points || '0');
    const s = c.settings || {};
    if (s.basis) setSel('set-basis', s.basis);
    if (s.prepaid) setSel('set-prepaid', s.prepaid);
    if (s.timing) setSel('set-timing', s.timing);
    if (s.balloonIncl) setSel('set-balloonIncl', s.balloonIncl);
    if (s.exact) setSel('set-exact', s.exact);
    if (s.rule78) setSel('set-rule78', s.rule78);
    const fillGrid = (body, sel, rows) => {
      const trs = document.querySelectorAll(body + ' tr');
      (rows || []).forEach((row, i) => { const tr = trs[i]; if (!tr) return; Object.keys(row).forEach(k => { const cell = tr.querySelector('[' + sel + '="' + k + '"]'); if (cell) cell.value = row[k]; }); });
    };
    fillGrid('#amz-balloon-body', 'data-amz-balloon-field', c.balloons);
    fillGrid('#amz-prepay-body', 'data-amz-prepay-field', c.prepays);
    fillGrid('#amz-adj-body', 'data-amz-adj-field', c.adjustments);
    if (c.moratorium) setv('amz-moratorium', c.moratorium);
    if (c.target) setv('amz-targetAmt', c.target);
    if (c.skip) setv('amz-skipMonths', c.skip);

    let threw = null;
    try { await calcAmortization(); } catch (e) { threw = e.message; }

    const errEl = document.getElementById('amz-error');
    const err = errEl ? errEl.textContent.trim() : '';
    const r = (typeof amzScheduleData !== 'undefined' && amzScheduleData) ? amzScheduleData.result : null;
    const readCell = id => { const e = document.getElementById(id); return e ? e.value : ''; };
    let sched = null;
    if (r && r.schedule && r.schedule.length) {
      const last = r.schedule[r.schedule.length - 1];
      const counts = {};
      r.schedule.forEach(row => { const p = Math.round(row.payment * 100); if (p > 0) counts[p] = (counts[p] || 0) + 1; });
      let modal = 0, best = -1;
      Object.keys(counts).forEach(p => { if (counts[p] > best) { best = counts[p]; modal = p / 100; } });
      sched = { n: r.schedule.length, lastBal: last.principal, modalPmt: modal };
    }
    return {
      err, threw,
      totalPaid: r ? r.totalPaid : null, totalInterest: r ? r.totalInterest : null, apr: r ? r.apr : null,
      solvedAmountCell: readCell('amz-amount'), solvedRateCell: readCell('amz-rate'),
      solvedBalloonCells: Array.from(document.querySelectorAll('#amz-balloon-body [data-amz-balloon-field="amount"]')).map(e => e.value),
      sched
    };
  }, c);
}

function approx(a, b, tol) { return a != null && b != null && Math.abs(a - b) <= tol; }

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1200, height: 900 } });
  page.on('dialog', d => d.accept().catch(() => {}));
  await page.goto(URL, { waitUntil: 'networkidle' });
  await page.addStyleTag({ content: '.modal-overlay{display:none !important;}' });
  await page.evaluate(() => showScreen('amortization'));

  const results = [];
  for (const c of cases) {
    let rec = { id: c.id, group: c.group, title: c.title };
    try {
      const ui = await fillAndCalc(page, c);
      const ora = runOracle(c);
      rec.ui = ui; rec.oracle = ora;
      const checks = [];
      const T = 0.02;
      if (ora.err) {
        checks.push({ k: 'both-refuse', ok: !!(ui.err || ui.threw), detail: `ui.err="${ui.err}" oracle="${ora.err.slice(0, 50)}"` });
      } else {
        if (c.solve === 'amount' && ora.solvedamount != null) {
          const uiAmt = parseFloat((ui.solvedAmountCell || '').replace(/[$,]/g, ''));
          checks.push({ k: 'solved-amount', ok: approx(uiAmt, ora.solvedamount, 0.5), detail: `ui=${uiAmt} oracle=${ora.solvedamount}` });
        }
        if (c.solve === 'rate' && ora.solvedrate != null) {
          const uiRate = parseFloat((ui.solvedRateCell || '').replace(/[%,]/g, '')) / 100;
          checks.push({ k: 'solved-rate', ok: approx(uiRate, ora.solvedrate, 5e-5), detail: `ui=${uiRate} oracle=${ora.solvedrate}` });
        }
        if (c.solveBalloon && ora.solvedballoon != null) {
          const uiBal = parseFloat((ui.solvedBalloonCells.find(v => v && v.trim()) || '0').replace(/[$,]/g, ''));
          checks.push({ k: 'solved-balloon', ok: approx(uiBal, ora.solvedballoon, 0.5), detail: `ui=${uiBal} oracle=${ora.solvedballoon}` });
        }
        if (ora.paid != null && !c.solve) {
          checks.push({ k: 'totalPaid', ok: approx(ui.totalPaid, ora.paid, T), detail: `ui=${ui.totalPaid} oracle=${ora.paid}` });
          checks.push({ k: 'totalInterest', ok: approx(ui.totalInterest, ora.interest, T), detail: `ui=${ui.totalInterest} oracle=${ora.interest}` });
        }
        if (ora.payment != null && ui.sched && !c.solve) {
          checks.push({ k: 'modalPayment', ok: approx(ui.sched.modalPmt, ora.payment, 0.02), detail: `ui=${ui.sched && ui.sched.modalPmt} oracle=${ora.payment}` });
        }
        if (ui.sched && (c.balloons || c.prepays || c.adjustments || c.moratorium || c.target || c.skip)) {
          checks.push({ k: 'retires-to-zero', ok: Math.abs(ui.sched.lastBal) < 1.0, detail: `lastBal=${ui.sched.lastBal}` });
        }
      }
      rec.checks = checks;
      rec.pass = checks.length > 0 && checks.every(c2 => c2.ok);
      rec.na = checks.length === 0;
      rec.adjudication = c.adjudication || null;
    } catch (e) { rec.error = e.message; rec.pass = false; }
    results.push(rec);
    const status = rec.error ? 'ERR' : rec.adjudication ? 'FLAG' : rec.na ? 'n/a' : rec.pass ? 'PASS' : 'FAIL';
    console.log(`${status.padEnd(4)} ${rec.id.padEnd(12)} ${rec.title}`);
    if (!rec.pass && rec.checks) rec.checks.filter(x => !x.ok).forEach(x => console.log(`      ✗ ${x.k}: ${x.detail}`));
    if (rec.ui && (rec.ui.err || rec.ui.threw) && !rec.pass) console.log(`      ui.err="${rec.ui.err}" threw=${rec.ui.threw}`);
    if (rec.error) console.log(`      ERROR ${rec.error}`);
  }
  fs.writeFileSync(__dirname + '/amz_results.json', JSON.stringify(results, null, 2));
  const flagged = results.filter(r => r.adjudication);
  const core = results.filter(r => !r.adjudication);
  const pass = core.filter(r => r.pass).length, fail = core.filter(r => !r.pass && !r.na).length, na = core.filter(r => r.na).length;
  console.log(`\n=== AMZ core: ${pass} pass, ${fail} fail, ${na} n/a of ${core.length}  |  ${flagged.length} flagged for adjudication ===`);
  await browser.close();
})().catch(e => { console.error('HARNESS FAIL:', e); process.exit(1); });
