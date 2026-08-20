// r63_beta_actuarial_browser_test.js — does the Life Contingency (beta) gate
// actually HIDE anything, in a real browser, with cdn.tailwindcss.com
// UNREACHABLE?
//
// 🚨 THIS IS THE ONLY TEST THAT CAN ANSWER THAT, AND IT IS THE WHOLE POINT.
// discrepancies.md §95: the shipped page gets `.hidden { display: none }` from
// the Tailwind CDN, and the inline stylesheet has no such rule. With the CDN
// blocked, 18 elements carrying `class="hidden"` were measured STILL DISPLAYED.
// So a gate implemented with the `.hidden` CLASS would be inert in exactly the
// deployment where it matters most. The gate therefore uses the `hidden`
// ATTRIBUTE, which the UA stylesheet honours with no network at all — and this
// harness proves it, rather than asserting it.
//
// ⚠️ EVERY BROWSER MEASUREMENT OF THIS PAGE MUST STATE WHETHER TAILWIND LOADED.
// This one BLOCKS it deliberately and says so. Run it with `--tailwind` to do
// the opposite arm (fulfilled from a local copy) and confirm the gate holds
// both ways — a hide that only works offline would be just as broken.
//
//   node testplan/harness/r63_beta_actuarial_browser_test.js <url> [--tailwind]
//
// Exit 0 = every assertion held. Non-zero = a real defect.

const { chromium } = require('/root/node_modules/playwright');
const fs = require('fs');

const URL = process.argv[2] || 'http://127.0.0.1:8833/';
const WANT_TAILWIND = process.argv.includes('--tailwind');
const TW_LOCAL = '/tmp/tw.js';

let pass = 0, fail = 0;
function ck(name, cond, detail) {
  if (cond) { pass++; console.log('  ok   ' + name); }
  else { fail++; console.log('  FAIL ' + name + (detail ? '  -- ' + detail : '')); }
}

(async () => {
  const browser = await chromium.launch({
    executablePath: '/opt/pw-browsers/chromium-1194/chrome-linux/chrome',
    args: ['--no-sandbox'],
  });
  const page = await browser.newPage();

  // ---- the Tailwind arm, stated explicitly ----
  let tailwindServed = false;
  await page.route('**://cdn.tailwindcss.com/**', route => {
    if (WANT_TAILWIND && fs.existsSync(TW_LOCAL)) {
      tailwindServed = true;
      route.fulfill({ status: 200, contentType: 'application/javascript',
                      body: fs.readFileSync(TW_LOCAL, 'utf8') });
    } else {
      route.abort();
    }
  });

  await page.addInitScript(() => {
    try { localStorage.setItem('persense-tour-done', '1'); } catch (e) {}
  });
  await page.goto(URL, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1200);

  // Prove which world we are measuring in. CAUTION 23.
  const twLoaded = await page.evaluate(() =>
    typeof window.tailwind !== 'undefined');
  console.log('\nTAILWIND LOADED: ' + twLoaded +
              (WANT_TAILWIND ? '  (arm: local SHIM at ' + TW_LOCAL + ' supplying .hidden{display:none} — NOT real Tailwind)'
                             : '  (arm: CDN BLOCKED — the offline case)'));
  if (!WANT_TAILWIND) {
    ck('tailwind is genuinely absent (the arm is what it claims)', !twLoaded,
       'tailwind loaded despite the block — this run measures the wrong world');
  }
  // The §95 control: with Tailwind absent the `.hidden` CLASS is inert. If this
  // ever starts passing, §95 has been fixed and this control must be retired.
  if (!twLoaded) {
    const classHideInert = await page.evaluate(() => {
      const d = document.createElement('div');
      d.className = 'hidden'; d.textContent = 'probe';
      document.body.appendChild(d);
      const v = getComputedStyle(d).display !== 'none';
      d.remove(); return v;
    });
    ck('§95 control: the .hidden CLASS is inert offline (so the attribute is required)',
       classHideInert, 'the class now works offline — §95 may be fixed; re-read this harness');
  }

  await page.evaluate(() => showScreen('presentvalue'));
  await page.waitForTimeout(400);

  const vis = sel => page.evaluate(s => {
    const el = document.querySelector(s);
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return !!(el.offsetParent !== null || r.width || r.height);
  }, sel);

  // ================= ARM 1 — SHIPPED DEFAULT: FEATURE OFF =================
  console.log('\n-- ARM 1: shipped default (beta OFF) --');
  const boxState = await page.evaluate(() => {
    const el = document.getElementById('set-betaActuarial');
    return el ? el.checked : null;
  });
  ck('the beta checkbox exists and ships UNCHECKED', boxState === false,
     'checked=' + boxState);

  ck('the Life Contingency panel is NOT visible',
     (await vis('#pv-actuarial-section')) === false);
  ck('the lump-sum Life column header is NOT visible',
     (await vis('#pv-ls-grid th[data-beta-actu]')) === false);
  ck('the periodic Life column header is NOT visible',
     (await vis('#pv-per-grid th[data-beta-actu]')) === false);
  ck('the per-row Life select is NOT visible',
     (await vis('td[data-beta-actu]')) === false);
  ck('the Life Contingency journey group is NOT visible',
     (await vis('.journey-group[data-beta-actu]')) === false);

  // 🚨 The regression this must NOT cause. Variable rate is {$ifdef PVLX},
  // which the oracle BUILDS, with five passing DOS sweeps; all 14 COLA options
  // are oracle-exercised. Neither is actuarial; neither may be hidden.
  ck('VARIABLE RATE stays visible (DOS-validated, must not be gated)',
     (await vis('#pv-vr-section')) === true);
  ck('the COLA % column stays visible (DOS-validated, must not be gated)',
     await page.evaluate(() => {
       const ths = [...document.querySelectorAll('#pv-per-grid th')];
       const th = ths.find(t => /COLA/.test(t.textContent));
       return !!(th && th.offsetParent !== null);
     }));

  // A hidden control must also be out of the tab order — `display:none` gives
  // that for free, which a class-based hide would not have.
  ck('hidden actuarial inputs are NOT focusable',
     await page.evaluate(() => {
       const el = document.getElementById('actu-dob1');
       if (!el) return true;
       el.focus();
       return document.activeElement !== el;
     }));

  // ================= ARM 2 — FEATURE TURNED ON =================
  console.log('\n-- ARM 2: beta ON (the positive control) --');
  await page.evaluate(() => {
    const el = document.getElementById('set-betaActuarial');
    el.checked = true;
    applyBetaActuarialGate();
  });
  await page.waitForTimeout(250);

  ck('the Life Contingency panel becomes visible',
     (await vis('#pv-actuarial-section')) === true);
  ck('the Life column headers become visible',
     (await vis('#pv-ls-grid th[data-beta-actu]')) === true);
  ck('the per-row Life select becomes visible',
     (await vis('td[data-beta-actu]')) === true);
  ck('VARIABLE RATE is still visible with the feature on',
     (await vis('#pv-vr-section')) === true);

  // ================= ARM 3 — THE DATA BOUNDARY =================
  // Pixels are not a boundary. Turning the feature off must also stop the
  // request from carrying contingency input, because localStorage restores
  // values into hidden controls.
  console.log('\n-- ARM 3: the request body, not the pixels --');
  // getPVInput() returns null on an empty worksheet, so the row must carry
  // real content before the request body means anything. (The first cut of
  // this harness asserted against `null` three times and called it a failure.)
  await page.evaluate(() => {
    const d = document.querySelector('input[data-ls="0"][data-f="date"]');
    const a = document.querySelector('input[data-ls="0"][data-f="amount"]');
    if (d) d.value = '01/01/2036';
    if (a) a.value = '100000';
    const sel = document.querySelector('select[data-ls="0"][data-f="act"]');
    if (sel) sel.value = 'L';
    ['actu-dob1', 'actu-now'].forEach(function (id, i) {
      const el = document.getElementById(id);
      if (el) el.value = i === 0 ? '01/01/1966' : '01/01/2026';
    });
    const t1 = document.getElementById('actu-table1');
    if (t1 && t1.options.length > 1) t1.selectedIndex = 1;
  });
  const withOn = await page.evaluate(() => {
    const b = (typeof getPVInput === 'function') ? getPVInput() : null;
    return b ? { act: (b.lumpSums[0] || {}).act, beta: !!b.betaActuarial } : null;
  });
  ck('with the feature ON a contingency reaches the request',
     withOn && withOn.act === 'L', JSON.stringify(withOn));
  // 🚨 THIS ASSERTION WAS MISSING AND IT IS THE ONE THAT MATTERS. `withOn.beta`
  // was computed above and then never checked, so deleting the single line
  // `body.betaActuarial = true` from getPVInput left this harness reporting
  // "ALL 18 ASSERTIONS HELD" while the server refused every request the UI
  // made — the feature 100% dead for every user, and nothing in the repository
  // noticed. Round 63 audit pass D.
  ck('with the feature ON the request carries the server opt-in',
     withOn && withOn.beta === true,
     'betaActuarial missing — the server will refuse this: ' + JSON.stringify(withOn));

  await page.evaluate(() => {
    const el = document.getElementById('set-betaActuarial');
    el.checked = false;
    applyBetaActuarialGate();
  });
  await page.waitForTimeout(250);
  const withOff = await page.evaluate(() => {
    const b = (typeof getPVInput === 'function') ? getPVInput() : null;
    return b ? { act: (b.lumpSums[0] || {}).act, beta: !!b.betaActuarial,
                 hasActuarial: !!b.actuarial } : null;
  });
  ck('turning it OFF forces the stored contingency back to N',
     withOff && withOff.act === 'N', JSON.stringify(withOff));
  ck('turning it OFF sends no actuarial block and no opt-in',
     withOff && !withOff.hasActuarial && !withOff.beta, JSON.stringify(withOff));

  // ============ ARM 4 — UNDO, THE MEASURED BYPASS ============
  // applyState() restores BOTH the checkbox and the per-row act values, and is
  // reached by undoLast(). Before the fix, one Undo click after switching the
  // feature off re-posted a contingency with every gated control still hidden.
  console.log('\n-- ARM 4: Undo must not desync the gate from the data --');
  await page.evaluate(() => {
    const el = document.getElementById('set-betaActuarial');
    el.checked = true; applyBetaActuarialGate();
    const sel = document.querySelector('select[data-ls="0"][data-f="act"]');
    if (sel) sel.value = 'L';
    if (typeof pushUndo === 'function') pushUndo();
    el.checked = false; applyBetaActuarialGate();
  });
  await page.waitForTimeout(200);
  const undone = await page.evaluate(() => {
    if (typeof undoLast === 'function') undoLast();
    const el = document.getElementById('set-betaActuarial');
    const b = (typeof getPVInput === 'function') ? getPVInput() : null;
    const marked = [...document.querySelectorAll('[data-beta-actu]')];
    return {
      checked: !!(el && el.checked),
      visible: marked.filter(e => e.offsetParent !== null).length,
      total: marked.length,
      act: b ? (b.lumpSums[0] || {}).act : null,
      beta: !!(b && b.betaActuarial),
    };
  });
  // Whatever Undo restores, the PIXELS and the DATA must agree afterwards.
  ck('after Undo the gate and the data agree (no hidden contingency, no ' +
     'discarded visible one)',
     undone && ((undone.checked && undone.visible === undone.total) ||
                (!undone.checked && undone.visible === 0 && undone.act === 'N' &&
                 !undone.beta)),
     JSON.stringify(undone));

  // ============ ARM 5 — A RESULT ALREADY ON SCREEN ============
  // Hiding the inputs does not un-paint the answer.
  console.log('\n-- ARM 5: switching off must not leave a survival-weighted total --');
  const stale = await page.evaluate(() => {
    const el = document.getElementById('set-betaActuarial');
    el.checked = true; applyBetaActuarialGate();
    // Paint a life-contingent-looking engine output, exactly as the response
    // handler does: a green cell carrying the survival annotation.
    const cell = document.querySelector('input[data-ls="0"][data-f="value"]');
    if (cell) { cell.value = '$253,343.30 (avg p=47.0%)'; cell.classList.add('cell-output'); }
    const tot = document.getElementById('pv-total');
    if (tot) { tot.value = '$253,343.30'; tot.classList.add('cell-output'); }
    el.checked = false; applyBetaActuarialGate();
    const root = document.getElementById('screen-presentvalue');
    const greens = [...root.querySelectorAll('.cell-output')]
      .map(e => (e.value != null ? e.value : e.textContent) || '');
    return { greens: greens, anyProb: greens.some(v => /\bp=/.test(v)) };
  });
  ck('no survival probability is left painted on the screen',
     stale && stale.anyProb === false, JSON.stringify(stale));
  ck('no engine-output cell survives the switch-off',
     stale && stale.greens.length === 0, JSON.stringify(stale));

  console.log('\n' + (fail === 0 ? 'ALL ' + pass + ' ASSERTIONS HELD'
                                 : pass + ' passed, ' + fail + ' FAILED'));
  await browser.close();
  process.exit(fail === 0 ? 0 : 1);
})().catch(e => { console.error('HARNESS ERROR: ' + e.message); process.exit(2); });
