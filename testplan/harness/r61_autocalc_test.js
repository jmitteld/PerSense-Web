// r61_autocalc_test.js — auto-calculate is ON by default and fires ONLY on
// Enter. Client UI items #1/#5 (default) and #11 (trigger), decision 3a.19.
//
// Run:  node testplan/harness/r61_autocalc_test.js http://127.0.0.1:8821
// Needs: node with `playwright`, a chromium at $PW_CHROME, and the REAL server
//        (`go build ./cmd/persense && ./persense -port 8821`) — not a static
//        file server, because the hardening assertions need real answers.
//
// 🚨 TAILWIND. The shipped page pulls https://cdn.tailwindcss.com and chromium
// in this container cannot reach it; without Tailwind `.hidden` does nothing
// and the settings dialog covers the viewport (see discrepancies §95's r61
// status update). This harness fulfils that ONE request from a local copy via
// page.route, so it drives the shipped bytes unmodified. Point TW at a copy of
// the CDN script (curl -sSL -o /tmp/tw.js https://cdn.tailwindcss.com).
const { chromium } = require('playwright');
const fs = require('fs');

const BASE = process.argv[2] || 'http://127.0.0.1:8821';
const TW = process.env.TW || '/tmp/tw.js';

let pass = 0, fail = 0; const fails = [];
function ck(c, m) { if (c) pass++; else { fail++; fails.push(m); } }

async function newPage(browser) {
  const page = await browser.newPage({ viewport: { width: 1500, height: 1050 } });
  if (fs.existsSync(TW)) {
    await page.route('https://cdn.tailwindcss.com/**', r =>
      r.fulfill({ status: 200, contentType: 'application/javascript', body: fs.readFileSync(TW) }));
    await page.route('https://cdn.tailwindcss.com', r =>
      r.fulfill({ status: 200, contentType: 'application/javascript', body: fs.readFileSync(TW) }));
  }
  return page;
}

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.PW_CHROME || '/opt/pw-browsers/chromium-1194/chrome-linux/chrome' });

  // ---------- 1. The shipped DEFAULT, on a browser with no stored state ----------
  {
    const page = await newPage(browser);
    await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
    await page.waitForTimeout(500);
    const tw = await page.evaluate(() => !!window.tailwind);
    ck(tw, 'Tailwind did not load — every DOM assertion below is measuring the container, not the page');
    const checked = await page.evaluate(() => document.getElementById('set-autocalc').checked);
    ck(checked === true, 'Auto-calculate is NOT on by default on a fresh browser');
    const enabled = await page.evaluate(() => autoCalcEnabled());
    ck(enabled === true, 'autoCalcEnabled() is false on a fresh browser');
    await page.close();
  }

  // ---------- 2. The TRIGGER ----------
  const page = await newPage(browser);
  await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
  await page.evaluate(() => { try { localStorage.setItem('persense-tour-done', '1'); } catch (_) {} });
  await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
  await page.waitForTimeout(600);
  await page.evaluate(() => {
    const o = document.getElementById('tour-overlay'); if (o) o.remove();
    showScreen('amortization');
    // ⚠️ FORCE THE PREFERENCE ON. Section 1 measures the DEFAULT; this section
    // measures the TRIGGER, and on a tree where auto-calculate ships OFF every
    // "it did not fire" assertion below would pass for the wrong reason —
    // a null with no power (R76). Caught by running the negative control.
    document.getElementById('set-autocalc').checked = true;
    // Count auto-calc RUNS without letting them fire. scheduleAutoCalc calls
    // runAutoCalc by identifier, so the global binding is what it resolves.
    window.__autoRuns = 0;
    window.runAutoCalc = function () { window.__autoRuns++; };
  });
  await page.waitForTimeout(300);

  const runs = () => page.evaluate(() => window.__autoRuns);
  const reset = () => page.evaluate(() => { window.__autoRuns = 0; });
  // scheduleAutoCalc debounces 150ms; wait past it every time.
  const settle = () => page.waitForTimeout(400);

  // 2a. Typing then TAB must not recalculate.
  await reset();
  await page.click('#amz-amount');
  await page.fill('#amz-amount', '100000');
  await page.keyboard.press('Tab');
  await settle();
  ck((await runs()) === 0, `Tab out of a field triggered ${await runs()} auto-calc(s) — item #11`);

  // 2b. Typing then clicking a DIFFERENT cell must not recalculate.
  await reset();
  await page.click('#amz-rate');
  await page.fill('#amz-rate', '7.5');
  await page.click('#amz-nPeriods');
  await settle();
  ck((await runs()) === 0, `clicking into another cell triggered ${await runs()} auto-calc(s) — item #11`);

  // 2c. ENTER must recalculate. POSITIVE CONTROL: without this the two nulls
  // above could pass on a build where auto-calculate never runs at all.
  await reset();
  await page.click('#amz-amount');
  await page.keyboard.press('Enter');
  await settle();
  ck((await runs()) >= 1, 'POSITIVE CONTROL FAILED: Enter did not trigger an auto-calc');

  // 2d. Shift+Delete clears the cell but does not recalculate on its own.
  await reset();
  await page.click('#amz-rate');
  await page.keyboard.press('Shift+Delete');
  await settle();
  const cleared = await page.evaluate(() => document.getElementById('amz-rate').value);
  ck(cleared === '', 'Shift+Delete did not clear the focused cell');
  ck((await runs()) === 0, `Shift+Delete triggered ${await runs()} auto-calc(s)`);

  // 2e. A COMPUTATIONAL SETTING change still refreshes — that path was kept
  // deliberately and this assertion is what stops it being removed by accident.
  await reset();
  // The refresh lives in closeSettings(), not updateSettingsBadge() — drive the
  // real open/change/close cycle rather than a function that never scheduled one.
  await page.evaluate(() => {
    openSettings();
    document.getElementById('set-basis').value = '365';
    updateSettingsBadge();
    closeSettings();
  });
  await settle();
  ck((await runs()) >= 1, 'changing a computational setting no longer refreshes the worksheet');
  await page.evaluate(() => {
    openSettings();
    document.getElementById('set-basis').value = '360';
    updateSettingsBadge();
    closeSettings();
  });

  // ---------- 3. HARDENING survives leaving the cell (client item #11) ----------
  // End to end against the real engine: solve a payment, harden it with H,
  // then leave the cell by Tab and by mouse and confirm the frozen value is
  // still there and still hardened.
  await page.evaluate(() => { delete window.runAutoCalc; });
  await page.reload({ waitUntil: 'networkidle' });
  await page.waitForTimeout(600);
  await page.evaluate(() => { const o = document.getElementById('tour-overlay'); if (o) o.remove(); showScreen('amortization');
    document.getElementById('set-autocalc').checked = true; });
  await page.waitForTimeout(200);
  await page.fill('#amz-amount', '100000');
  await page.fill('#amz-rate', '7.5');
  await page.fill('#amz-nPeriods', '360');
  await page.fill('#amz-perYr', '12');
  await page.fill('#amz-loanDate', '01/01/2027');
  await page.evaluate(() => { document.getElementById('amz-payment').value = ''; });
  await page.evaluate(() => calcAmortization());
  await page.waitForTimeout(1500);
  const solved = await page.evaluate(() => {
    const el = document.getElementById('amz-payment');
    return { v: el.value, green: el.classList.contains('cell-output') };
  });
  ck(solved.v !== '' && solved.green, `the engine did not solve the payment (value="${solved.v}", green=${solved.green}) — the hardening assertions below have no power`);

  if (solved.v !== '' && solved.green) {
    await page.click('#amz-payment');
    await page.keyboard.press('h');
    await page.waitForTimeout(300);
    const afterH = await page.evaluate(() => {
      const el = document.getElementById('amz-payment');
      return { v: el.value, green: el.classList.contains('cell-output'), hardened: el.dataset.hardened };
    });
    ck(afterH.v === solved.v, `H changed the value (${solved.v} -> ${afterH.v})`);
    ck(!afterH.green, 'H did not clear the computed-green styling');

    await page.keyboard.press('Tab');
    await page.waitForTimeout(600);
    const afterTab = await page.evaluate(() => document.getElementById('amz-payment').value);
    ck(afterTab === afterH.v, `leaving the hardened cell by Tab changed it (${afterH.v} -> ${afterTab}) — item #11`);

    await page.click('#amz-rate');
    await page.waitForTimeout(600);
    const afterClick = await page.evaluate(() => document.getElementById('amz-payment').value);
    ck(afterClick === afterH.v, `leaving the hardened cell by mouse changed it (${afterH.v} -> ${afterClick}) — item #11`);
  }

  // ---------- 4. Auto-calculate must not compute on a date the app rejects ----------
  await page.reload({ waitUntil: 'networkidle' });
  await page.waitForTimeout(600);
  await page.evaluate(() => { const o = document.getElementById('tour-overlay'); if (o) o.remove(); showScreen('amortization');
    document.getElementById('set-autocalc').checked = true; });
  await page.fill('#amz-amount', '100000');
  await page.fill('#amz-rate', '7.5');
  await page.fill('#amz-nPeriods', '360');
  await page.fill('#amz-perYr', '12');
  await page.evaluate(() => {
    const el = document.getElementById('amz-loanDate');
    el.value = '13/45/2027';                 // month 13, day 45 — the validator rejects it
    el.dispatchEvent(new Event('input', { bubbles: true }));
    document.getElementById('amz-payment').value = '';
    window.__calcAttempts = 0;
    const realFetch = window.fetch;
    window.fetch = function (u, o) { if (String(u).indexOf('/api/') !== -1) window.__calcAttempts++; return realFetch(u, o); };
  });
  await page.click('#amz-amount');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(900);
  const attempts = await page.evaluate(() => window.__calcAttempts);
  ck(attempts === 0, `auto-calculate posted ${attempts} request(s) with an invalid date on screen — the r59 asymmetry`);

  // ---------- 5. HARDENING IS A CONTRACT, NOT THREE BUG FIXES ----------
  // 🚨 The app states it outright at the mortgage grid: "Hardened fields are
  // invariant under recalculation (client requirement)". An r61 adversarial
  // audit measured that it held in exactly three places (mortgage grid,
  // #pv-total, amz-payment) and FAILED on two the help text advertises:
  // PV row Value cells and the amortization date/term cells. Both are covered
  // here. r61 moved the guard into writeOut / echoAmzCell so it is a property
  // of the paint path rather than of whichever cell someone reported.
  {
    const page5 = await newPage(browser);
    await page5.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
    await page5.evaluate(() => { try { localStorage.setItem('persense-tour-done', '1'); } catch (_) {} });
    await page5.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
    await page5.waitForTimeout(600);

    // 5a. A PV lump-sum row's Value cell.
    await page5.evaluate(() => {
      const o = document.getElementById('tour-overlay'); if (o) o.remove();
      showScreen('presentvalue');
      document.getElementById('set-autocalc').checked = true;
    });
    await page5.waitForTimeout(300);
    await page5.evaluate(() => {
      const set = (sel, v) => { const el = document.querySelector(sel); if (el) { el.value = v; el.dispatchEvent(new Event('input', { bubbles: true })); } };
      set('#pv-asOfDate', '01/01/2024');
      set('#pv-rate', '6');
      set('[data-ls="0"][data-f="date"]', '01/01/2030');
      set('[data-ls="0"][data-f="amount"]', '100000');
      calcPV();
    });
    await page5.waitForTimeout(1800);
    const v0 = await page5.evaluate(() => {
      const el = document.querySelector('[data-ls="0"][data-f="value"]');
      return el ? { v: el.value, green: el.classList.contains('cell-output') } : null;
    });
    ck(!!v0 && v0.v !== '' && v0.green, `PV row Value did not compute (${JSON.stringify(v0)}) — the hardening assertion below has no power`);
    if (v0 && v0.v !== '' && v0.green) {
      await page5.click('[data-ls="0"][data-f="value"]');
      await page5.keyboard.press('h');
      await page5.waitForTimeout(300);
      const hard = await page5.evaluate(() => {
        const el = document.querySelector('[data-ls="0"][data-f="value"]');
        return { v: el.value, green: el.classList.contains('cell-output'), flag: el.dataset.hardened };
      });
      ck(hard.flag === '1', 'H did not harden the PV row Value cell');
      // Change the row Amount and recalculate — the one sanctioned trigger.
      await page5.evaluate(() => {
        const el = document.querySelector('[data-ls="0"][data-f="amount"]');
        el.value = '250000'; el.dispatchEvent(new Event('input', { bubbles: true }));
        calcPV();
      });
      await page5.waitForTimeout(1800);
      const after = await page5.evaluate(() => document.querySelector('[data-ls="0"][data-f="value"]').value);
      ck(after === hard.v, `a HARDENED PV row Value was overwritten by the recalculation (${hard.v} -> ${after})`);
    }

    // 5b. The amortization Last Pmt Date — a hardened date must not be
    //     restamped by the engine's echo (nor by the §107 snap).
    await page5.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
    await page5.waitForTimeout(600);
    await page5.evaluate(() => {
      const o = document.getElementById('tour-overlay'); if (o) o.remove();
      showScreen('amortization');
      document.getElementById('set-autocalc').checked = true;
      const set = (id, v) => { const el = document.getElementById(id); el.value = v; el.dispatchEvent(new Event('input', { bubbles: true })); };
      set('amz-amount', '100000'); set('amz-rate', '7.5'); set('amz-perYr', '12');
      set('amz-loanDate', '01/01/2027'); set('amz-firstDate', '01/01/2027');
      set('amz-nPeriods', '360'); document.getElementById('amz-payment').value = '';
      calcAmortization();
    });
    await page5.waitForTimeout(1800);
    const ld = await page5.evaluate(() => {
      const el = document.getElementById('amz-lastDate');
      return { v: el.value, green: el.classList.contains('cell-output') };
    });
    ck(ld.v !== '' && ld.green, `amz-lastDate did not compute (${JSON.stringify(ld)}) — no power below`);
    if (ld.v !== '' && ld.green) {
      await page5.click('#amz-lastDate');
      await page5.keyboard.press('h');
      await page5.waitForTimeout(300);
      const hv = await page5.evaluate(() => document.getElementById('amz-lastDate').value);
      await page5.evaluate(() => {
        const el = document.getElementById('amz-nPeriods');
        el.value = '120'; el.dispatchEvent(new Event('input', { bubbles: true }));
        calcAmortization();
      });
      await page5.waitForTimeout(1800);
      const lv = await page5.evaluate(() => document.getElementById('amz-lastDate').value);
      ck(lv === hv, `a HARDENED amz-lastDate was overwritten by the recalculation (${hv} -> ${lv})`);
    }
    await page5.close();
  }

  // ---------- 6. Flipping the default must not fake a "restored worksheet" ----------
  // collectState() snapshots every input[id] INCLUDING the Auto-calculate
  // checkbox, so every state saved before r61 carries `set-autocalc: false` and
  // differs from the new pristine boot state on that key alone — and the
  // restore notice fired for every returning user. Measured by an r61 audit.
  //
  // ⚠️ THE ASSERTION IS AN ISOLATION, NOT AN ABSOLUTE. A first version asserted
  // "a returning user with an unchanged worksheet sees no notice" and failed —
  // because a snapshot taken after boot has already drifted from the boot state
  // in ways that have nothing to do with this fix (soft-filled defaults), so it
  // was not testing an unchanged worksheet at all. What the fix actually
  // guarantees is narrower and is what is asserted: THE PREFERENCE ALONE MUST
  // NOT CHANGE THE OUTCOME.
  {
    const noticeFor = async (prefValue) => {
      const pg = await newPage(browser);
      await pg.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
      await pg.waitForTimeout(500);
      const snap = await pg.evaluate((pv) => {
        if (typeof saveStateNow === 'function') saveStateNow();
        const st = collectState();
        if (st.fields) st.fields['set-autocalc'] = pv;
        return JSON.stringify(st);
      }, prefValue);
      const key = await pg.evaluate(() =>
        Object.keys(localStorage).find(k => k !== 'persense-tour-done'));
      if (!key) { await pg.close(); return { key: null }; }
      await pg.addInitScript(([k, v]) => {
        try { localStorage.setItem('persense-tour-done', '1'); localStorage.setItem(k, v); } catch (_) {}
      }, [key, snap]);
      await pg.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
      await pg.waitForTimeout(900);
      const out = await pg.evaluate(() => ({
        notice: !!document.body.textContent.match(/Restored your worksheet/i),
        pref: document.getElementById('set-autocalc').checked,
      }));
      await pg.close();
      return Object.assign({ key: key }, out);
    };
    const asPreR61 = await noticeFor(false);   // a state saved before r61
    const asPostR61 = await noticeFor(true);   // the same state saved after r61
    ck(!!asPreR61.key, 'could not find the persisted-state localStorage key');
    if (asPreR61.key) {
      ck(asPreR61.pref === false,
         `a returning user's stored Auto-calculate preference was overwritten (checked=${asPreR61.pref})`);
      ck(asPostR61.pref === true, 'a stored Auto-calculate=true was not honoured');
      // ⚠️ The end-to-end notice comparison is recorded but NOT asserted: it has
      // no power. Verified by mutation — `PREF_IDS = []` SURVIVES it, because
      // post-boot drift makes the notice fire either way. Reporting a null with
      // no power as if it were evidence is exactly what R69 is about.
      console.log(`   [note, not asserted] restore notice: pre-r61 state ${asPreR61.notice}, post-r61 state ${asPostR61.notice}`);
    }

    // THE ASSERTION WITH POWER: the comparison seam itself. Two snapshots that
    // differ only in a preference must reduce to the same worksheet key, and a
    // snapshot that differs in real content must not.
    {
      const pg = await newPage(browser);
      await pg.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
      await pg.waitForTimeout(500);
      const r = await pg.evaluate(() => {
        if (typeof worksheetKey !== 'function') return { missing: true };
        const base = collectState();
        const withFalse = JSON.parse(JSON.stringify(base)); withFalse.fields['set-autocalc'] = false;
        const withTrue = JSON.parse(JSON.stringify(base)); withTrue.fields['set-autocalc'] = true;
        const changed = JSON.parse(JSON.stringify(base));
        changed.fields['amz-amount'] = String((changed.fields['amz-amount'] || '') + '9');
        return {
          prefIgnored: worksheetKey(withFalse) === worksheetKey(withTrue),
          contentSeen: worksheetKey(changed) !== worksheetKey(base),
        };
      });
      await pg.close();
      ck(!r.missing, 'worksheetKey() is not exposed — the restore comparison has no testable seam');
      if (!r.missing) {
        ck(r.prefIgnored, 'two states differing ONLY in the Auto-calculate preference compare as different worksheets');
        ck(r.contentSeen, 'POSITIVE CONTROL: a real worksheet change is no longer seen as a change');
      }
    }
  }

  // ---------- 7. Reset Defaults must restore the Auto-calculate default ----------
  {
    const page7 = await newPage(browser);
    await page7.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
    await page7.waitForTimeout(500);
    const reset = await page7.evaluate(() => {
      const el = document.getElementById('set-autocalc');
      el.checked = false;
      resetSettings();
      return el.checked;
    });
    ck(reset === true, 'Reset Defaults left Auto-calculate off — it no longer restores the shipped default');
    await page7.close();
  }

  await browser.close();
  console.log(`\nPASS ${pass}  FAIL ${fail}`);
  const show = parseInt(process.env.R61_SHOW || '25', 10);
  if (fail) { fails.slice(0, show).forEach(f => console.log('  ✗ ' + f)); if (fails.length > show) console.log(`  ... and ${fails.length - show} more`); }
  process.exit(fail ? 1 : 0);
})();
