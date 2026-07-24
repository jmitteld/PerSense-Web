// Advanced-options fidelity fuzzer for the Amortization UI.
//
// Two independent checks per random case:
//   (A) CORRECTNESS: fresh calc vs the DOS amort_oracle (forward + AO9 only,
//       where UI↔oracle semantics match cleanly).
//   (B) PATH-INDEPENDENCE (stale detection): compute the SAME inputs two ways —
//       (1) fresh, and (2) via an edit path (perturb an unrelated field, calc,
//       restore it, calc). The two fingerprints MUST be identical. Any drift is
//       a stale-recompute bug (the class of §34/§35). This needs no oracle and
//       covers EVERY advanced option, including solve cases whose green cells
//       froze in the real bugs.
//
// Run: node fuzz_amz_stale.js [N] [seed]

const { chromium } = require('playwright');
const { execFileSync } = require('child_process');

const ORACLE = '/tmp/oraclebuild/amort_oracle';
const URL = 'http://localhost:8099/';
const N = parseInt(process.argv[2] || '120', 10);
let seed = parseInt(process.argv[3] || '20260723', 10);
function rnd() { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x7fffffff; }
function ri(a, b) { return a + Math.floor(rnd() * (b - a + 1)); }
function pick(arr) { return arr[Math.floor(rnd() * arr.length)]; }
function money(x) { return Math.round(x * 100) / 100; }

// annuity payment at fraction rate, monthly, on a 360 basis (matches engine 1+r/12).
function annuity(bal, rateFrac, n, perYr) { const f = rateFrac / perYr; return bal * f / (1 - Math.pow(1 + f, -n)); }

function mkCase(i) {
  // Base loan
  const amount = ri(50, 400) * 1000;
  const rateFrac = ri(400, 1200) / 10000;        // 4%–12%
  const perYr = 12;
  const n = pick([24, 36, 48, 60, 120, 180, 240, 360]);
  const loan = { y: 2024, m: 1, d: 1 };
  const first = { y: 2024, m: 2, d: 1 };
  const kind = pick([
    'forward', 'forward', 'forward-mor', 'forward-skip', 'forward-target',
    'ao9', 'ao9', 'balloon-mid', 'prepay-known', 'stacked',
    'balloon-solve', 'balloon-solve', 'fwd-basis', 'fwd-inadv',
  ]);
  const c = { i, kind, amount, rateFrac, perYr, n, loan, first,
    fields: {}, oracle: [], settings: {} };
  // percent string for the UI rate cell
  const ratePct = (rateFrac * 100);
  const pad = x => String(x).padStart(2, '0');
  c.fields = {
    amount: String(amount),
    rate: String(+ratePct.toFixed(6)),
    perYr: String(perYr),
    loanDate: `${pad(loan.m)}/${pad(loan.d)}/${loan.y}`,
    firstDate: `${pad(first.m)}/${pad(first.d)}/${first.y}`,
    nPeriods: String(n),
  };
  c.oracle = [String(amount), String(rateFrac), String(n), String(perYr),
    `loandmy=${loan.d}.${loan.m}.${loan.y}`, `firstdmy=${first.d}.${first.m}.${first.y}`, 'prepaid'];

  const natural = annuity(amount, rateFrac, n, perYr);
  const monthlyInt = amount * rateFrac / perYr;
  // an under-amortizing but interest-covering payment (strictly between interest and natural)
  const underPay = () => money(monthlyInt + (natural - monthlyInt) * (0.30 + rnd() * 0.55));

  if (kind === 'forward') {
    // plain
  } else if (kind === 'forward-mor') {
    const mm = ri(2, Math.min(8, Math.floor(n / 4)));
    c.moratorium = { months: mm };
    c.oracle.push(`mor=${mm}`);
  } else if (kind === 'forward-skip') {
    const s = ri(3, Math.min(9, n - 4)); const e = s + ri(0, 2);
    c.skip = `${s}-${e}`;
    c.oracle.push(`skip=${s}-${e}`);
  } else if (kind === 'forward-target') {
    const t = money(natural * (0.3 + rnd() * 0.3)); // min principal reduction below natural
    c.target = t;
    c.oracle.push(`targ=${t.toFixed(2)}`);
  } else if (kind === 'ao9') {
    // hard under-amortizing payment + unknown prepay (count given, amount blank)
    const pay = underPay();
    const start = 1 + ri(0, Math.floor(n / 3));
    const ppPerYr = pick([12, 6]);
    const maxNN = Math.max(1, Math.floor((n - start) * ppPerYr / 12));
    const nn = 1 + ri(0, Math.min(maxNN - 1, 20));
    c.payment = pay; c.solveKind = 'ao9';
    c.prepay = { start, ppPerYr, nn };
    c.fields.payment = String(pay);
    c.oracle.push(`payhard=${pay.toFixed(2)}`, `presolve=${start}:${nn}:${ppPerYr}`, 'plusreg');
  } else if (kind === 'balloon-mid') {
    // date-only balloon partway; ADD mode (balloonIncl=no default). No clean oracle → stale-only.
    const bm = ri(Math.floor(n / 3), Math.floor((2 * n) / 3));
    const bamt = ri(5, 40) * 1000;
    c.balloonKnown = { months: bm, amount: bamt };
    c.noOracle = true;
  } else if (kind === 'prepay-known') {
    const start = 1 + ri(0, Math.floor(n / 4));
    const ppPerYr = pick([12, 6]);
    const maxNN = Math.max(1, Math.floor((n - start) * ppPerYr / 12));
    const nn = 1 + ri(0, Math.min(maxNN - 1, 24));
    const amt = ri(50, 500) * 10;
    c.prepay = { start, ppPerYr, nn, amount: amt };
    c.oracle.push(`pre=${start}:${nn}:${ppPerYr}:${amt.toFixed(2)}`, 'plusreg');
  } else if (kind === 'stacked') {
    // moratorium + skip together (both forward, additive semantics)
    const mm = ri(2, 5); const s = mm + ri(2, 4); const e = s + ri(0, 2);
    c.moratorium = { months: mm }; c.skip = `${s}-${e}`;
    c.oracle.push(`mor=${mm}`, `skip=${s}-${e}`);
    c.noOracle = true; // stacked mor+skip semantics: stale-only to be safe
  } else if (kind === 'balloon-solve') {
    // date-only balloon partway with an under-amortizing hard payment ⇒ engine
    // solves the balloon amount (green cell, the §34 class). Stale-only + retire.
    const pay = underPay();
    const bm = ri(Math.floor(n / 2), n - 1);
    c.payment = pay; c.solveKind = 'balloon';
    c.fields.payment = String(pay);
    c.balloonSolve = { months: bm };
    c.noOracle = true; // terminating-vs-mid semantics differ from solveballoon=
  } else if (kind === 'fwd-basis') {
    const b = pick(['365', '365/360']);
    c.settings.basis = b;
    if (b === '365') c.oracle.push('b365');
    else { c.oracle.push('b365_360'); c.kick365360 = true; }
  } else if (kind === 'fwd-inadv') {
    c.settings.timing = 'advance';
    c.oracle.push('inadv');
  }
  // 365/360 forward kicker: DOS discounts with rate*365/360 (docs §28).
  if (c.kick365360) c.oracle[1] = (rateFrac * 365 / 360).toFixed(10);
  return c;
}

// date helper: months after loan → mm/dd/yyyy
function monthsAfter(loan, k) { const tot = (loan.m - 1) + k; return `${(tot % 12) + 1}/${loan.d}/${loan.y + Math.floor(tot / 12)}`; }

function runOracleForward(c) {
  try {
    const out = execFileSync(ORACLE, c.oracle, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
    let m;
    if ((m = out.match(/prepay ([\d.-]+)/))) return { prepay: +m[1], raw: out.trim() };
    if ((m = out.match(/payment ([\d.]+) interest ([\d.-]+) paid ([\d.-]+)/))) return { payment: +m[1], interest: +m[2], paid: +m[3], raw: out.trim() };
    return { raw: out.trim() };
  } catch (e) { return { err: (e.stdout || '') + ' ' + e.message }; }
}

async function apply(page, c, opts) {
  return await page.evaluate(async ({ c, opts, monthsAfterStr }) => {
    const monthsAfter = new Function('loan', 'k', monthsAfterStr);
    const setv = (id, v) => { const e = document.getElementById(id); if (e) { e.value = v; if (e.classList) e.classList.remove('cell-output'); e.dispatchEvent(new Event('input', { bubbles: true })); } };
    // full reset
    document.querySelectorAll('#amz-balloon-body [data-amz-balloon-field], #amz-prepay-body [data-amz-prepay-field], #amz-adj-body [data-amz-adj-field]').forEach(e => { e.value = ''; if (e.classList) e.classList.remove('cell-output'); });
    ['amz-amount', 'amz-loanDate', 'amz-rate', 'amz-firstDate', 'amz-nPeriods', 'amz-lastDate', 'amz-payment', 'amz-moratorium', 'amz-targetAmt', 'amz-skipMonths', 'amz-payoff-date', 'amz-payoff-bal'].forEach(id => setv(id, ''));
    setv('amz-perYr', '12'); setv('amz-points', '0');
    const setSel = (id, v) => { const e = document.getElementById(id); if (e) e.value = v; };
    setSel('set-basis', '360'); setSel('set-prepaid', 'yes'); setSel('set-timing', 'arrears');
    setSel('set-balloonIncl', 'no'); setSel('set-exact', 'no'); setSel('set-rule78', 'no');
    if (typeof amzScheduleData !== 'undefined') amzScheduleData = null;

    // settings from the case
    const s = c.settings || {};
    if (s.basis) setSel('set-basis', s.basis);
    if (s.timing) setSel('set-timing', s.timing);
    if (s.prepaid) setSel('set-prepaid', s.prepaid);
    if (s.balloonIncl) setSel('set-balloonIncl', s.balloonIncl);
    if (s.exact) setSel('set-exact', s.exact);
    if (s.rule78) setSel('set-rule78', s.rule78);

    // core fields
    const f = c.fields;
    Object.keys(f).forEach(k => setv('amz-' + k, f[k]));

    // advanced
    if (c.moratorium) setv('amz-moratorium', monthsAfter(c.loan, c.moratorium.months));
    if (c.skip) setv('amz-skipMonths', c.skip);
    if (c.target != null) setv('amz-targetAmt', String(c.target));
    if (c.prepay) {
      const tr = document.querySelectorAll('#amz-prepay-body tr')[0];
      const put = (k, v) => { const cc = tr.querySelector('[data-amz-prepay-field="' + k + '"]'); if (cc) { cc.value = v; cc.dispatchEvent(new Event('input', { bubbles: true })); } };
      put('startDate', monthsAfter(c.loan, c.prepay.start));
      put('perYr', String(c.prepay.ppPerYr));
      put('nPmts', String(c.prepay.nn));
      if (c.prepay.amount != null) put('amount', String(c.prepay.amount));
    }
    if (c.balloonKnown) {
      const tr = document.querySelectorAll('#amz-balloon-body tr')[0];
      const put = (k, v) => { const cc = tr.querySelector('[data-amz-balloon-field="' + k + '"]'); if (cc) { cc.value = v; cc.dispatchEvent(new Event('input', { bubbles: true })); } };
      put('date', monthsAfter(c.loan, c.balloonKnown.months));
      put('amount', String(c.balloonKnown.amount));
    }
    if (c.balloonSolve) {
      const tr = document.querySelectorAll('#amz-balloon-body tr')[0];
      const put = (k, v) => { const cc = tr.querySelector('[data-amz-balloon-field="' + k + '"]'); if (cc) { cc.value = v; cc.dispatchEvent(new Event('input', { bubbles: true })); } };
      put('date', monthsAfter(c.loan, c.balloonSolve.months)); // amount blank ⇒ solve
    }

    // optional perturbation for the edit path. Two modes:
    //  'nperiods' — bump the term then restore (re-solve should reproduce fresh).
    //  'advfield' — add a temporary Skip range, calc, then clear it (reproduces the
    //               §34 trigger: an edit AFTER a solve must re-run, not freeze).
    if (opts && opts.perturb) {
      const mode = opts.mode || 'nperiods';
      if (mode === 'nperiods') {
        const cur = document.getElementById('amz-nPeriods').value;
        setv('amz-nPeriods', String(parseInt(cur || '0', 10) + 12));
        await calcAmortization();
        setv('amz-nPeriods', cur);
      } else if (mode === 'advfield') {
        const cur = document.getElementById('amz-skipMonths').value;
        setv('amz-skipMonths', cur ? cur : '2-2'); // add or alter a skip
        await calcAmortization();
        setv('amz-skipMonths', cur); // restore original (blank or prior)
      }
    }

    let threw = null;
    try { await calcAmortization(); } catch (e) { threw = e.message; }
    const errEl = document.getElementById('amz-error');
    const r = (typeof amzScheduleData !== 'undefined' && amzScheduleData) ? amzScheduleData.result : null;
    let sched = null;
    if (r && r.schedule && r.schedule.length) {
      const last = r.schedule[r.schedule.length - 1];
      const counts = {};
      r.schedule.forEach(row => { const p = Math.round(row.payment * 100); if (p > 0) counts[p] = (counts[p] || 0) + 1; });
      let modal = 0, best = -1; Object.keys(counts).forEach(p => { if (counts[p] > best) { best = counts[p]; modal = p / 100; } });
      sched = { n: r.schedule.length, lastBal: last.principal, modalPmt: modal };
    }
    const readCell = id => { const e = document.getElementById(id); return e ? e.value : ''; };
    return {
      threw, err: errEl ? errEl.textContent.trim() : '',
      totalPaid: r ? r.totalPaid : null, totalInterest: r ? r.totalInterest : null,
      solvedPrepay: r ? r.solvedPrepay : null,
      prepayAmtCell: (document.querySelector('#amz-prepay-body [data-amz-prepay-field="amount"]') || {}).value || '',
      balloonCells: Array.from(document.querySelectorAll('#amz-balloon-body [data-amz-balloon-field="amount"]')).map(e => e.value).filter(v => v),
      sched,
    };
  }, { c, opts, monthsAfterStr: 'const p=x=>String(x).padStart(2,"0"); const tot=(loan.m-1)+k; return `${p((tot%12)+1)}/${p(loan.d)}/${loan.y+Math.floor(tot/12)}`;' });
}

function fp(u) {
  // fingerprint used for path-independence comparison
  return JSON.stringify({
    tp: u.totalPaid == null ? null : Math.round(u.totalPaid * 100),
    ti: u.totalInterest == null ? null : Math.round(u.totalInterest * 100),
    sp: u.solvedPrepay == null ? null : Math.round(u.solvedPrepay * 100),
    n: u.sched ? u.sched.n : null,
    lb: u.sched ? Math.round(u.sched.lastBal * 100) : null,
    mp: u.sched ? Math.round(u.sched.modalPmt * 100) : null,
    err: u.err || u.threw || '',
  });
}

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1200, height: 900 } });
  page.on('dialog', d => d.accept().catch(() => {}));
  await page.goto(URL, { waitUntil: 'networkidle' });
  await page.addStyleTag({ content: '.modal-overlay{display:none !important;}' });
  await page.evaluate(() => showScreen('amortization'));

  const stale = [], wrong = [], errs = [], skipped = [];
  let ok = 0, checked = 0;
  for (let i = 0; i < N; i++) {
    const c = mkCase(i);
    let fresh, edN, edA;
    try {
      fresh = await apply(page, c, { perturb: false });
      edN = await apply(page, c, { perturb: true, mode: 'nperiods' });
      edA = await apply(page, c, { perturb: true, mode: 'advfield' });
    } catch (e) { errs.push({ i, kind: c.kind, e: e.message }); continue; }

    // skip cases that error on the fresh pass (undetermined / unsupported shape)
    if (fresh.err || fresh.threw) { skipped.push({ i, kind: c.kind, err: fresh.err || fresh.threw, c: adv(c) }); continue; }
    checked++;

    // (B) path-independence — both edit paths must reproduce the fresh fingerprint
    if (fp(fresh) !== fp(edN)) stale.push({ i, kind: c.kind, mode: 'nperiods', c: { amount: c.amount, rateFrac: c.rateFrac, n: c.n, ...adv(c) }, fresh: fp(fresh), edited: fp(edN) });
    if (fp(fresh) !== fp(edA)) stale.push({ i, kind: c.kind, mode: 'advfield', c: { amount: c.amount, rateFrac: c.rateFrac, n: c.n, ...adv(c) }, fresh: fp(fresh), edited: fp(edA) });

    // (A) correctness vs oracle (forward + ao9 only, semantics match)
    if (!c.noOracle) {
      const ora = runOracleForward(c);
      if (ora.prepay != null && fresh.solvedPrepay != null) {
        if (Math.abs(ora.prepay - fresh.solvedPrepay) > 0.5)
          wrong.push({ i, kind: c.kind, check: 'ao9-prepay', ui: fresh.solvedPrepay, oracle: ora.prepay, c: adv(c) });
        else ok++;
      } else if (ora.paid != null && fresh.totalPaid != null && !c.solveKind) {
        const dPaid = Math.abs(ora.paid - fresh.totalPaid), dInt = Math.abs(ora.interest - fresh.totalInterest);
        if (dPaid > 0.03 || dInt > 0.03)
          wrong.push({ i, kind: c.kind, check: 'forward', uiPaid: fresh.totalPaid, oraPaid: ora.paid, uiInt: fresh.totalInterest, oraInt: ora.interest, dPaid, dInt, c: adv(c) });
        else ok++;
      }
    }
  }

  function adv(c) { const o = {}; if (c.moratorium) o.mor = c.moratorium.months; if (c.skip) o.skip = c.skip; if (c.target != null) o.target = c.target; if (c.prepay) o.prepay = c.prepay; if (c.balloonKnown) o.balloon = c.balloonKnown; if (c.payment) o.payment = c.payment; return o; }

  console.log(`\n=== FUZZ done: ${N} generated, ${checked} checked, ${skipped.length} skipped(fresh-error) ===`);
  console.log(`Correctness vs oracle: ${ok} matched, ${wrong.length} DIVERGED`);
  console.log(`Path-independence: ${stale.length} STALE (of ${checked}×2 edit paths)`);
  if (errs.length) console.log(`(harness errors: ${errs.length})`);
  if (wrong.length) { console.log('\n--- CORRECTNESS DIVERGENCES ---'); wrong.slice(0, 25).forEach(w => console.log(JSON.stringify(w))); }
  if (stale.length) { console.log('\n--- STALE (path-dependent) ---'); stale.slice(0, 25).forEach(s => console.log(JSON.stringify(s))); }
  // skip-reason histogram
  const skHist = {}; skipped.forEach(s => { const key = (s.err || '').slice(0, 45); skHist[key] = (skHist[key] || 0) + 1; });
  if (skipped.length) { console.log('\n--- SKIP REASONS ---'); Object.entries(skHist).sort((a, b) => b[1] - a[1]).forEach(([k, v]) => console.log(`  ${v}× ${k}`)); }
  require('fs').writeFileSync(__dirname + '/fuzz_amz_stale_results.json', JSON.stringify({ wrong, stale, errs, skipped }, null, 2));
  await browser.close();
})().catch(e => { console.error('HARNESS FAIL', e); process.exit(1); });
