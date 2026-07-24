// Regression: hardening the Present Value total (#pv-total) must survive recalc.
// 2026-07-24 client report — "can't harden the Present Value amount." Root cause:
// hardenOutputCell removed the green class but set no dataset.hardened marker, and
// calcPV unconditionally overwrote #pv-total (re-greening it on a forward calc), so
// a forward autocalc reverted the harden instantly. Fix: set dataset.hardened on
// harden; calcPV skips the write when hardened; typing clears it.
//
// Run: node regress_pv_harden_total.js   (server on :8099)
const { chromium } = require('playwright');
const URL = 'http://localhost:8099/';

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  page.on('dialog', d => d.accept().catch(() => {}));
  await page.addInitScript(() => { try { localStorage.clear(); } catch (e) {} });
  await page.route('**/*', r => r.request().url().startsWith(URL) ? r.continue() : r.abort());
  await page.goto(URL, { waitUntil: 'load' });
  await page.evaluate(() => showScreen('presentvalue'));

  const fails = [];
  const check = (name, got, want) => {
    if (got !== want) { fails.push(`${name}: got ${got}, want ${want}`); console.log(`  FAIL ${name}: ${JSON.stringify(got)} != ${JSON.stringify(want)}`); }
    else console.log(`  ok   ${name}: ${JSON.stringify(got)}`);
  };

  // Forward setup: as-of 01/01/2027, loan rate 6, one periodic series.
  await page.evaluate(() => {
    document.getElementById('pv-asOfDate').value = '01/01/2027';
    onPVRateEdit(Object.assign(document.getElementById('pv-rate-loan'), { value: '6.0000' }));
    const per0 = (sel) => document.querySelector(`input[data-per="0"][data-f="${sel}"]`);
    per0('from').value = '05/01/2025'; per0('to').value = '08/01/2030';
    per0('perYr').value = '12'; per0('amount').value = '7,759.36'; per0('cola').value = '0';
    document.getElementById('pv-total').value = '';
    document.getElementById('pv-total').classList.remove('cell-output');
  });
  await page.evaluate(() => calcPV()); await page.waitForTimeout(120);

  const totVal = () => page.evaluate(() => document.getElementById('pv-total').value);
  const totGreen = () => page.evaluate(() => document.getElementById('pv-total').classList.contains('cell-output'));
  const totHard = () => page.evaluate(() => document.getElementById('pv-total').dataset.hardened === '1');

  const computed = await totVal();
  check('forward calc produced a total', !!computed && computed !== '', true);
  check('forward total is green (output)', await totGreen(), true);
  check('forward total not hardened yet', await totHard(), false);

  // Harden it (the H-key path calls hardenOutputCell on the focused green cell).
  await page.evaluate(() => hardenOutputCell(document.getElementById('pv-total')));
  check('after harden: value unchanged', await totVal(), computed);
  check('after harden: not green', await totGreen(), false);
  check('after harden: hardened flag set', await totHard(), true);

  // Recalc (forward, rate still present) — the bug was this reverted the harden.
  await page.evaluate(() => calcPV()); await page.waitForTimeout(120);
  check('after recalc: value STILL frozen', await totVal(), computed);
  check('after recalc: STILL not green', await totGreen(), false);
  check('after recalc: STILL hardened', await totHard(), true);

  // Blank the rate and solve — the frozen total is the backward-solve target.
  await page.evaluate(() => {
    ['pv-rate-true', 'pv-rate-loan', 'pv-rate-yield', 'pv-rate'].forEach(id => {
      const e = document.getElementById(id); if (e) { e.value = ''; e.classList.remove('cell-output'); }
    });
  });
  await page.evaluate(() => calcPV()); await page.waitForTimeout(150);
  check('backward solve: target still frozen', await totVal(), computed);
  const solvedRate = await page.evaluate(() => document.getElementById('pv-rate-loan').value);
  check('backward solve: a rate was solved', !!solvedRate && solvedRate !== '', true);
  console.log('   (solved loan rate =', solvedRate, ')');

  // Typing over the total un-hardens it.
  await page.evaluate(() => {
    const t = document.getElementById('pv-total');
    t.value = '123456.00';
    t.dispatchEvent(new Event('input', { bubbles: true }));
  });
  check('typing clears hardened flag', await totHard(), false);

  console.log(fails.length ? `\nFAILED (${fails.length})` : '\nPASS');
  await browser.close();
  process.exit(fails.length ? 1 : 0);
})().catch(e => { console.error('HARNESS FAIL', e); process.exit(1); });
