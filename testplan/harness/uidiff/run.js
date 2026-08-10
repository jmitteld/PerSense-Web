// uidiff/run.js — the DOS-anchored UI differential. Item 24, committed.
//
//   node testplan/harness/uidiff/run.js [--stacked=N] [--seed=S] [--only=tier]
//
// Requires: a running server (PERSENSE_URL, default http://localhost:8080/),
// the amort_oracle binary (PERSENSE_ORACLE, default /tmp/oraclebuild/amort_oracle),
// and playwright. Exits non-zero on any divergence, on a stale served build, or
// on a selftest failure.
//
// WHAT THIS INSTRUMENT IS FOR, precisely:
// the engine differentials compare the Go engine to the DOS engine. The
// frontend sweeps (internal/api/frontend_diff_sweep_test.go:447) compare the
// display to THE ENGINE'S OWN RESPONSE — self-referentially, by design and by
// documentation (docs/frontend_differential_harness.md:15). testplan/harness/
// run_amz.js does drive the real page against the real oracle, but it reads
// `amzScheduleData.result` — engine space — over 29 hand-written cases with a
// single oracle invocation and no adjustment-rate kicker.
// NOTHING compared what the USER SEES against DOS over a generated stacked
// population. That gap is where four of the amortization screen's last five
// defects lived. This is that comparison.

'use strict';

const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright');

const gen = require('./gen');
const oracle = require('./oracle');
const driver = require('./driver');
const compare = require('./compare');
const selftest = require('./selftest');

const argv = process.argv.slice(2);
const arg = (k, d) => {
  const a = argv.find(x => x.startsWith(`--${k}=`));
  return a ? a.split('=')[1] : d;
};

// Markers proving the served build carries the fixes this harness assumes.
// §89's fix (round 45b) and §88's (round 45). If the running binary predates
// them the harness is testing a build whose defects are already recorded.
const BUILD_MARKERS = [
  'dEl.value = fmtDateDisplay(a.date);',
  'function dateValidity',
];

function log(s) { console.log(s); }

(async () => {
  const seed = parseInt(arg('seed', '46046'), 10);
  const stacked = parseInt(arg('stacked', '200'), 10);
  const singlePer = parseInt(arg('singlePer', '2'), 10);
  const plain = parseInt(arg('plain', '10'), 10);
  const only = arg('only', null);

  // ---- 1. the traps, before anything is scored --------------------------
  selftest.runAll(log);

  // ---- 2. tolerances, printed with every run ----------------------------
  log('\nTOLERANCES (declared, with provenance — CAUTION 1 / item 0e):');
  for (const [k, v] of Object.entries(compare.TOLERANCES)) {
    log(`  ${k.padEnd(16)} ${String(v.value).padEnd(8)} ${v.why}`);
  }

  // ---- 3. the population ------------------------------------------------
  let cases = gen.generate({ seed, plain, singlePer, stacked });
  if (only) cases = cases.filter(c => c.tier === only);
  const byTier = {};
  cases.forEach(c => { byTier[c.tier] = (byTier[c.tier] || 0) + 1; });
  log(`\nPOPULATION seed=${seed}: ` +
      Object.entries(byTier).map(([t, n]) => `${t}=${n}`).join(' ') + ` total=${cases.length}`);
  log(`ORACLE ${oracle.ORACLE} — build flags -Mdelphi -Sg -CPPACKRECORD=1 ` +
      `-dV_3 -dSCROLLS -dPVLX. -dACTU is ABSENT AND UNBUILDABLE (R47).`);

  // ---- 4. the page ------------------------------------------------------
  const { browser, page, pageErrors } = await driver.open(chromium);
  await driver.assertServedBuild(page, BUILD_MARKERS);
  log(`SERVED BUILD OK — ${BUILD_MARKERS.length} markers present at ${driver.URL}`);
  await selftest.trap3Live(page, log);
  await selftest.liveScalarAssertions(page, log);

  // ---- 5. the run -------------------------------------------------------
  const results = [];
  let i = 0;
  for (const c of cases) {
    i++;
    const rec = { id: c.id, tier: c.tier, options: c.options };
    try {
      const ui = await driver.runCase(page, c);
      const ora = oracle.run(c);
      rec.checks = compare.compareCase(c, ui, ora);
      rec.pass = rec.checks.length > 0 && rec.checks.every(x => x.ok);
      // 🚨 A PASS THAT COMPARED NO ARITHMETIC IS NOT A PASS, AND `na` WAS DEAD
      // CODE. compareCase always adds at least one check, so `checks.length
      // === 0` never fired and every "both sides refused" case was laundered
      // into the headline as though the numbers had agreed. In the first full
      // run 62 of 188 stacked passes — a THIRD — were in that state, so
      // "188/200" read as 188 screens that agreed numerically when the true
      // figure was 126. R46's adversarial audit caught it. Count the stratum
      // separately and print it.
      const numeric = new Set(['totalPaid', 'totalInterest', 'payment', 'apr']);
      rec.comparedArithmetic = rec.checks.some(x => numeric.has(x.k) || x.k.startsWith('adjAmount@') || x.k.startsWith('adjRate@'));
      rec.na = !rec.comparedArithmetic;
      if (!rec.pass || !rec.comparedArithmetic) {
        rec.ui = ui;
        rec.oracleArgs = { data: ora.data.args, apr: ora.apr.args };
        rec.oracleRaw = { data: ora.data.raw, apr: ora.apr.raw };
      }
      // The live-page DOUBLE-SUBMIT check — the r45b process rule, on every
      // case that produced a screen. It is what would have caught §89.
      // 🚨 Gate on ui.refused (painted no schedule), NOT on ui.err. compare.js
      // establishes that an ADVISORY and an ERROR share #amz-error, so gating
      // on the text skipped the double-submit on every screen carrying a note —
      // at least 62 of 246 passes in the first full run. And record the SKIP,
      // so the artifact can distinguish "checked and fine" from "never ran".
      rec.doubleSubmit = 'skipped:refused-or-threw';
      if (rec.pass && !ui.refused && !ui.threw) {
        rec.doubleSubmit = 'ran';
        const ds = await driver.doubleSubmit(page);
        const blocked = ds.paintedRows === 0 || !!ds.threw;
        const moved = ds.before.paid !== ds.after.paid || ds.before.int !== ds.after.int;
        if (blocked || moved) {
          rec.checks.push({
            k: 'double-submit', ok: false,
            detail: blocked
              ? `the SECOND Calculate, with nothing changed, was BLOCKED: err="${ds.err}" threw=${ds.threw}`
              : `the SECOND Calculate moved the displayed totals: ` +
                `"${ds.before.paid}"->"${ds.after.paid}" / "${ds.before.int}"->"${ds.after.int}"`,
          });
          rec.pass = false;
          rec.ui = ui;
          rec.doubleSubmit = 'FAILED';
        }
      }
    } catch (e) {
      rec.error = e.message; rec.pass = false;
    }
    results.push(rec);
    const st = rec.error ? 'ERR ' : !rec.pass ? 'FAIL' : rec.na ? 'both-refuse' : 'pass';
    if (st !== 'pass' || process.env.UIDIFF_VERBOSE) {
      log(`${st} ${String(i).padStart(4)}/${cases.length} ${rec.id.padEnd(28)} [${(rec.options || []).join(',')}]`);
      (rec.checks || []).filter(x => !x.ok).forEach(x => log(`        ✗ ${x.k}: ${x.detail}`));
      if (rec.error) log(`        ERROR ${rec.error}`);
      if (rec.oracleArgs) log(`        oracle: ${rec.oracleArgs.data.join(' ')}`);
    }
  }

  await browser.close();

  // ---- 6. the verdicts --------------------------------------------------
  const controls = compare.controlVerdict(results);
  const tell = compare.theTell(results);

  log('\n' + '='.repeat(78));
  log(controls.message);
  log(tell.message);
  log('='.repeat(78));

  const perTier = {};
  for (const r of results) {
    const t = perTier[r.tier] || (perTier[r.tier] = { n: 0, agreed: 0, fail: 0, bothRefuse: 0, err: 0 });
    t.n++;
    if (r.error) t.err++;
    else if (!r.pass) t.fail++;
    else if (!r.comparedArithmetic) t.bothRefuse++;
    else t.agreed++;
  }
  log('\nBY TIER — and the DENOMINATOR THAT MATTERS is `agreed + FAIL`, not n:');
  log('  tier      AGREED  FAIL  both-refuse(NO NUMBER COMPARED)  err   n');
  for (const [t, v] of Object.entries(perTier)) {
    const scored = v.agreed + v.fail;
    log(`  ${t.padEnd(9)} ${String(v.agreed).padStart(5)} ${String(v.fail).padStart(5)} ` +
        `${String(v.bothRefuse).padStart(20)} ${String(v.err).padStart(20)} ${String(v.n).padStart(4)}` +
        (scored ? `   -> ${v.fail} in ${scored}` : ''));
  }
  log('  🚨 a `both-refuse` row compared NO arithmetic: DOS declined and so did the');
  log('     page. It is refusal PARITY, a real and useful result, but it is not');
  log('     evidence that any number agreed. Never fold it into a divergence rate.');

  // Attribution aid: which single options are implicated?
  const failedSingles = results.filter(r => r.tier === 'single' && !r.pass && !r.na);
  if (failedSingles.length) {
    const byOpt = {};
    failedSingles.forEach(r => { (r.options || []).forEach(o => { byOpt[o] = (byOpt[o] || 0) + 1; }); });
    log('\n🚨 ONE-OPTION-AT-A-TIME TIER IMPLICATES:');
    Object.entries(byOpt).sort((a, b) => b[1] - a[1])
      .forEach(([o, n]) => log(`  ${o.padEnd(18)} ${n}`));
    log('  (a single-tier failure is directly attributable — no ablation needed)');
  }

  if (pageErrors.length) {
    log(`\n⚠️  ${pageErrors.length} uncaught page error(s): ${[...new Set(pageErrors)].slice(0, 5).join(' | ')}`);
  }

  const outPath = path.join(__dirname, 'uidiff_results.json');
  fs.writeFileSync(outPath, JSON.stringify({
    seed, generatedAt: null, tolerances: compare.TOLERANCES,
    oracle: oracle.ORACLE, url: driver.URL,
    controls, tell, perTier, results,
  }, null, 1));
  log(`\nresults -> ${outPath}`);

  // 🚨 NOT `&& !r.na`. Since `na` now means "compared no arithmetic", a
  // refusal-PARITY divergence (DOS declined, the page answered) is both a real
  // divergence AND `na` — and excluding it made the harness exit 0 with four
  // findings on screen. A reporting bug that hides findings is worse than the
  // findings.
  const fails = results.filter(r => !r.pass).length;
  // A mapping-bug verdict is NOT a defect count. Exit distinctly so a caller
  // cannot mistake one for the other.
  if (tell.level === 'mapping-bug' || !controls.ok) {
    log('\nEXIT 2 — HARNESS FAULT SUSPECTED. The divergence count above is NOT a defect rate.');
    process.exit(2);
  }
  if (fails) { log(`\nEXIT 1 — ${fails} divergence(s).`); process.exit(1); }
  log('\nEXIT 0 — no divergence.');
})().catch(e => { console.error('HARNESS FAIL:', e && e.stack || e); process.exit(3); });
