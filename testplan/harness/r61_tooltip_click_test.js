// r61 — tooltip reveal is CLICK-ONLY. Run against a served copy of
// cmd/persense/static. Usage: node r61_tooltip_click_test.js <baseURL>
const { chromium } = require('playwright');

const BASE = process.argv[2];
const TIP_BUBBLE_MAX = 200;

let pass = 0, fail = 0;
const fails = [];
function ck(cond, msg) { if (cond) pass++; else { fail++; fails.push(msg); } }

(async () => {
  const browser = await chromium.launch({ executablePath: process.env.PW_CHROME || '/opt/pw-browsers/chromium-1194/chrome-linux/chrome' });
  const page = await browser.newPage({ viewport: { width: 1400, height: 1000 } });
  // Suppress the first-run tour overlay; it intercepts pointer events.
  await page.goto(BASE + '/index.html');
  await page.evaluate(() => { try { localStorage.setItem('persense-tour-done', '1'); } catch (_) {} });
  await page.goto(BASE + '/index.html');
  await page.waitForTimeout(400);
  await page.evaluate(() => { const o = document.getElementById('tour-overlay'); if (o) o.remove(); });

  const bubbleVisible = () => page.evaluate(() => {
    const b = document.getElementById('tip-hover');
    return !!(b && !b.hidden && b.classList.contains('tip-show'));
  });
  const bubbleExistsNotHidden = () => page.evaluate(() => {
    const b = document.getElementById('tip-hover');
    return !!(b && !b.hidden);
  });
  const modalVisible = () => page.evaluate(() => {
    const m = document.getElementById('tip-modal-overlay');
    return !!(m && !m.hidden);
  });
  // Hard reset between cases, done in JS so a stuck overlay from the tree
  // under test cannot block the next case's real interactions. The dismissal
  // ASSERTIONS below still use genuine clicks / keys.
  const dismissAll = () => page.evaluate(() => {
    const b = document.getElementById('tip-hover');
    if (b) { b.hidden = true; b.classList.remove('tip-show'); }
    const m = document.getElementById('tip-modal-overlay');
    if (m) m.hidden = true;
  });
  // Interactions that record a failure instead of aborting the run, so the
  // NEGATIVE CONTROL enumerates its failures rather than timing out (R77).
  async function safe(fn, what) {
    try { await fn(); return true; }
    catch (e) { fail++; fails.push(what + ' [blocked: ' + String(e).split('\n')[0].slice(0, 90) + ']'); return false; }
  }
  const hoverT = (sel, what) => safe(() => page.hover(sel, { timeout: 2500 }), what);
  const clickT = (sel, what) => safe(() => page.click(sel, { timeout: 2500 }), what);

  for (const screen of ['mortgage', 'amortization', 'presentvalue']) {
    await page.evaluate((s) => {
      showScreen(s);
      document.querySelectorAll('details').forEach(d => d.open = true);
    }, screen);
    await page.waitForTimeout(250);

    // Enumerate visible tips and their independently-computed expected route.
    const tips = await page.evaluate((MAX) => {
      const out = [];
      document.querySelectorAll('.tip').forEach((t, i) => {
        const r = t.getBoundingClientRect();
        if (r.width === 0 || r.height === 0) return;
        if (t.closest('#modal-settings')) return;      // settings modal is closed
        const txt = t.querySelector('.tiptext');
        if (!txt) return;
        const body = txt.innerHTML;
        const d = document.createElement('div'); d.innerHTML = body;
        const plain = d.textContent.replace(/\s+/g, ' ').trim();
        const hasLink = /<a[\s>]/i.test(body) || body.indexOf('tip-more') !== -1;
        const expect = (hasLink || plain.length > MAX) ? 'modal' : 'bubble';
        t.setAttribute('data-r61-probe', String(i));
        out.push({ probe: String(i), expect, len: plain.length, hasLink });
      });
      return out;
    }, TIP_BUBBLE_MAX);

    // Optional stratified cap (used by the mutation harness to keep each
    // mutant run short): N bubble-expected + N modal-expected per screen.
    const CAP = parseInt(process.env.R61_TIPS_PER_SCREEN || '0', 10);
    let tipsUsed = tips;
    if (CAP > 0) {
      tipsUsed = tips.filter(t => t.expect === 'bubble').slice(0, CAP)
        .concat(tips.filter(t => t.expect === 'modal').slice(0, CAP));
    }
    console.log(`  ${screen}: ${tips.length} visible tips` + (CAP ? ` (using ${tipsUsed.length})` : ''));
    let nBubble = 0, nModal = 0;

    for (const t of tipsUsed) {
      const sel = `.tip[data-r61-probe="${t.probe}"]`;
      await dismissAll(); await page.waitForTimeout(30);

      // 1. HOVER reveals nothing.
      await hoverT(sel, `${screen}/${t.probe}: hover action`);
      await page.waitForTimeout(120);
      ck(!(await bubbleExistsNotHidden()), `${screen}/${t.probe}: hover revealed the bubble`);
      ck(!(await modalVisible()),          `${screen}/${t.probe}: hover revealed the modal`);

      // 2. FOCUS reveals nothing.
      await page.focus(sel);
      await page.waitForTimeout(120);
      ck(!(await bubbleExistsNotHidden()), `${screen}/${t.probe}: focus revealed the bubble`);
      ck(!(await modalVisible()),          `${screen}/${t.probe}: focus revealed the modal`);

      // 3. CLICK opens the expected surface, and only that one.
      await clickT(sel, `${screen}/${t.probe}: click action`);
      await page.waitForTimeout(120);
      if (t.expect === 'bubble') {
        nBubble++;
        ck(await bubbleVisible(), `${screen}/${t.probe}: click did NOT open the bubble (len=${t.len})`);
        ck(!(await modalVisible()), `${screen}/${t.probe}: click opened the modal, expected bubble (len=${t.len})`);
        // bubble is anchored near its icon
        // Null-safe: a mutant that never creates the bubble must record a
        // FAILURE, not throw — otherwise a crash masquerades as a kill and the
        // killer-extractor is never exercised (R77).
        const near = await page.evaluate((s) => {
          const be = document.getElementById('tip-hover');
          const te = document.querySelector(s);
          if (!be || !te) return false;
          const b = be.getBoundingClientRect();
          const t = te.getBoundingClientRect();
          const dx = Math.abs((b.left + b.width/2) - (t.left + t.width/2));
          const dy = Math.min(Math.abs(b.bottom - t.top), Math.abs(b.top - t.bottom));
          return dx < 400 && dy < 40;
        }, sel);
        ck(near, `${screen}/${t.probe}: bubble not anchored near its icon`);
        // 4. second click on the SAME tip closes it (toggle)
        await clickT(sel, `${screen}/${t.probe}: click action`); await page.waitForTimeout(80);
        ck(!(await bubbleExistsNotHidden()), `${screen}/${t.probe}: second click did not close the bubble`);
        // 5. click elsewhere dismisses
        await clickT(sel, `${screen}/${t.probe}: click action`); await page.waitForTimeout(80);
        await page.evaluate(() => document.querySelector('h1, .win-button, body').click());
        await page.waitForTimeout(80);
        ck(!(await bubbleExistsNotHidden()), `${screen}/${t.probe}: outside click did not dismiss the bubble`);
        // 6. Escape dismisses
        await clickT(sel, `${screen}/${t.probe}: click action`); await page.waitForTimeout(80);
        await page.keyboard.press('Escape'); await page.waitForTimeout(80);
        ck(!(await bubbleExistsNotHidden()), `${screen}/${t.probe}: Escape did not dismiss the bubble`);
        // 7. Enter on the focused icon opens it
        await page.focus(sel);
        await page.keyboard.press('Enter'); await page.waitForTimeout(120);
        ck(await bubbleVisible(), `${screen}/${t.probe}: Enter did not open the bubble`);
        // 8. scrolling RE-ANCHORS the bubble (a position:fixed bubble would
        //    otherwise drift off its icon). The original rule here was "scroll
        //    dismisses"; it was withdrawn after an audit measured it eating the
        //    user's own click — see the listener comment in index.html.
        await page.evaluate(() => window.scrollBy(0, 60));
        await page.waitForTimeout(200);
        const stillAnchored = await page.evaluate((s) => {
          const b = document.getElementById('tip-hover');
          const el = document.querySelector(s);
          if (!b || b.hidden) return 'closed';
          if (!el) return 'no-tip';
          const br = b.getBoundingClientRect(), tr = el.getBoundingClientRect();
          if (tr.bottom < 0 || tr.top > window.innerHeight) return 'anchored'; // icon off-screen: closing is correct
          const dx = Math.abs((br.left + br.width / 2) - (tr.left + tr.width / 2));
          const dy = Math.min(Math.abs(br.bottom - tr.top), Math.abs(br.top - tr.bottom));
          return (dx < 400 && dy < 40) ? 'anchored' : 'orphaned';
        }, sel);
        ck(stillAnchored === 'anchored' || stillAnchored === 'closed',
           `${screen}/${t.probe}: after a scroll the bubble is ${stillAnchored}`);
        await page.evaluate(() => window.scrollTo(0, 0));
      } else {
        nModal++;
        ck(await modalVisible(), `${screen}/${t.probe}: click did NOT open the modal (len=${t.len}, link=${t.hasLink})`);
        ck(!(await bubbleExistsNotHidden()), `${screen}/${t.probe}: click opened the bubble, expected modal`);
        await page.keyboard.press('Escape'); await page.waitForTimeout(80);
        ck(!(await modalVisible()), `${screen}/${t.probe}: Escape did not dismiss the modal`);
      }
      await dismissAll();
    }
    console.log(`    routed: ${nBubble} bubble / ${nModal} modal`);
  }

  // ⚠️ RELOAD BEFORE THE WHOLE-PAGE SECTIONS. They used to inherit whatever
  // state the per-tip loop left behind, and under R61_TIPS_PER_SCREEN (the
  // mutation harness's cap) that state differs — which produced FALSE KILLS:
  // `capture_flag_dropped` was reported killed by the scroll assertions in a
  // capped run and SURVIVES the full run at 639/0. A section whose result
  // depends on how many tips ran before it is not measuring what it says.
  await page.goto(BASE + '/index.html', { waitUntil: 'networkidle' });
  await page.waitForTimeout(500);
  await page.evaluate(() => { const o = document.getElementById('tour-overlay'); if (o) o.remove(); });
  await page.evaluate(() => window.scrollTo(0, 0));

  // ---- A. A `?` inside a sortable column header must not sort the column ----
  // All 12 mortgage grid headers carry a tip.
  //
  // 🚨 THIS ASSERTION WAS VACUOUS UNTIL AN AUDIT CAUGHT IT, AND THE WAY IT WAS
  // WRONG IS WORTH KEEPING IN FRONT OF YOU. It used to replace
  // `window.onMtgHeaderClick` with a counter and assert the counter stayed 0.
  // But `onMtgHeaderClick` OPENS WITH `if (ev && ev.target.closest('.tip'))
  // return;` — a guard that has been in the page since long before r61. The
  // stub deleted the very guard under test, so the "defect" it reported (a `?`
  // click sorting the column) never existed on any tree, and the r61 claim
  // built on it was withdrawn. **Never instrument by replacing the function
  // whose behaviour you are measuring.** Assert on what a USER can see.
  await page.evaluate(() => { showScreen('mortgage'); initMortgageSortHeaders(); });
  await page.waitForTimeout(200);
  const sortState = () => page.evaluate(() => ({
    indicators: document.querySelectorAll('#mtg-grid thead .sort-indicator').length,
    order: Array.from(document.querySelectorAll('#mtg-body tr')).map(tr => tr.getAttribute('data-row')).join(','),
  }));
  await dismissAll();
  const beforeSort = await sortState();
  await clickT('#mtg-grid thead th[data-sort-field="price"] .tip', 'header-tip click action');
  await page.waitForTimeout(250);
  const afterTip = await sortState();
  ck(afterTip.indicators === beforeSort.indicators && afterTip.order === beforeSort.order,
     `clicking the "?" in a sortable header SORTED the column (indicators ${beforeSort.indicators}->${afterTip.indicators}, order changed: ${beforeSort.order !== afterTip.order})`);
  const helpShown = (await bubbleVisible()) || (await modalVisible());
  ck(helpShown, 'clicking the "?" in a sortable header opened no help at all');
  await dismissAll();
  // POSITIVE CONTROL: the header itself must still sort. Without it, a change
  // that simply broke sorting everywhere would pass the assertion above (R84).
  await page.evaluate(() => {
    const th = document.querySelector('#mtg-grid thead th[data-sort-field="price"]');
    const r = th.getBoundingClientRect();
    th.dispatchEvent(new MouseEvent('click', { bubbles: true, clientX: r.left + 3, clientY: r.top + 3 }));
  });
  await page.waitForTimeout(250);
  const afterHdr = await sortState();
  ck(afterHdr.indicators > 0, 'POSITIVE CONTROL: clicking the header text itself no longer sorts');

  // ---- A2. A click just after a scroll must still show help ----
  // The first cut of the scroll dismisser closed the bubble on ANY scroll event,
  // and chromium delivers the trailing scroll of the user's own wheel gesture
  // AFTER the click it caused — so a click within ~50ms of scrolling opened the
  // bubble and instantly closed it. Measured 3/3 dead at a 0ms delay.
  await dismissAll();
  await page.setViewportSize({ width: 1200, height: 560 });   // make the page scrollable
  await page.evaluate(() => { showScreen('amortization'); document.querySelectorAll('details').forEach(d => d.open = true); });
  await page.waitForTimeout(250);
  const raceSel = await page.evaluate(() => {
    const t = [...document.querySelectorAll('#screen-amortization .tip')].find(x => {
      const r = x.getBoundingClientRect(); if (!r.width) return false;
      const sp = x.querySelector('.tiptext'); if (!sp) return false;
      const d = document.createElement('div'); d.innerHTML = sp.innerHTML;
      const plain = d.textContent.replace(/\s+/g, ' ').trim();
      return plain.length <= 200 && !/<a[\s>]/i.test(sp.innerHTML) && sp.innerHTML.indexOf('tip-more') === -1;
    });
    if (!t) return null;
    t.setAttribute('data-r61-race', '1');
    return '.tip[data-r61-race="1"]';
  });
  ck(!!raceSel, 'no bubble-routed tip available for the scroll-race test');
  if (raceSel) {
    // The regression, stated as the invariant it actually is: A SCROLL EVENT
    // ARRIVING RIGHT AFTER THE CLICK MUST NOT CLOSE THE BUBBLE. Chromium
    // delivers the trailing events of the user's own wheel gesture after the
    // click that gesture ended in, so `scroll -> hideBubble()` ate the click.
    //
    // ⚠️ Driving that with mouse.wheel + mouse click is NOT deterministic:
    // the scroll is not applied to layout by the time the next coordinate read
    // returns, so the click lands where the icon USED to be and misses. The
    // first two versions of this test did exactly that and blamed the page.
    // Dispatching the trailing scroll event directly is deterministic and
    // tests the ordering that broke.
    await clickT(raceSel, 'scroll-race click action');
    await page.evaluate(() => { window.dispatchEvent(new Event('scroll')); document.dispatchEvent(new Event('scroll')); });
    await page.waitForTimeout(400);
    ck(await bubbleVisible(), 'a scroll event arriving just after the click closed the bubble — the dismisser is eating its own gesture');

    // A REAL scroll must RE-ANCHOR the bubble to its icon, not orphan it.
    await page.evaluate(() => window.scrollBy(0, 60));
    await page.waitForTimeout(400);
    const anchored = await page.evaluate((sel) => {
      const b = document.getElementById('tip-hover');
      const t = document.querySelector(sel);
      if (!b || b.hidden) return 'closed';
      const br = b.getBoundingClientRect(), tr = t.getBoundingClientRect();
      const dx = Math.abs((br.left + br.width / 2) - (tr.left + tr.width / 2));
      const dy = Math.min(Math.abs(br.bottom - tr.top), Math.abs(br.top - tr.bottom));
      return (dx < 400 && dy < 40) ? 'anchored' : 'orphaned';
    }, raceSel);
    ck(anchored === 'anchored', `after a real scroll the bubble is ${anchored} instead of re-anchored to its icon`);

    // ...and scrolling the icon right out of the viewport DOES close it,
    // because there is then nothing left to point at.
    await page.evaluate(() => window.scrollBy(0, 3000));
    await page.waitForTimeout(400);
    ck(!(await bubbleExistsNotHidden()), 'scrolling the icon out of view left the bubble on screen');
    await page.evaluate(() => window.scrollTo(0, 0));
    await dismissAll();
  }
  await page.setViewportSize({ width: 1400, height: 1000 });

  // ---- B. The open bubble must own its own pixels ----
  // pointer-events:auto is what stops a click meant to dismiss the bubble from
  // falling through onto whatever control happens to sit underneath it.
  await dismissAll();
  const bubbleSel = await page.evaluate(() => {
    const t = [...document.querySelectorAll('.tip')].find(x => {
      const r = x.getBoundingClientRect();
      if (!r.width) return false;
      const s = x.querySelector('.tiptext'); if (!s) return false;
      const d = document.createElement('div'); d.innerHTML = s.innerHTML;
      const plain = d.textContent.replace(/\s+/g, ' ').trim();
      return plain.length <= 200 && !/<a[\s>]/i.test(s.innerHTML) && s.innerHTML.indexOf('tip-more') === -1;
    });
    if (!t) return null;
    t.setAttribute('data-r61-hit', '1');
    return '.tip[data-r61-hit="1"]';
  });
  ck(!!bubbleSel, 'no bubble-routed tip available for the hit-test');
  if (bubbleSel) {
    await clickT(bubbleSel, 'hit-test click action');
    await page.waitForTimeout(200);
    const owns = await page.evaluate(() => {
      const b = document.getElementById('tip-hover');
      if (!b || b.hidden) return 'bubble-not-open';
      const r = b.getBoundingClientRect();
      const el = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      return (el && b.contains(el)) ? 'owns' : 'falls-through';
    });
    ck(owns === 'owns', `an open bubble does not receive its own clicks: ${owns}`);
  }

  await browser.close();
  console.log(`\nPASS ${pass}  FAIL ${fail}`);
  if (fail) { fails.slice(0, parseInt(process.env.R61_SHOW||'25',10)).forEach(f => console.log('  ✗ ' + f)); if (fails.length > parseInt(process.env.R61_SHOW||'25',10)) console.log(`  ... and ${fails.length - parseInt(process.env.R61_SHOW||'25',10)} more`); }
  process.exit(fail ? 1 : 0);
})();
