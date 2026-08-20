// r64_snap_ui_test.js — the round-64 UI surface, measured in a real browser
// with cdn.tailwindcss.com UNREACHABLE by default.
//
// WHAT IT COVERS
//   1. the brown "the engine moved this" cue (`.cell-snapped`) on all three
//      cells that can now carry it: Last Pmt Date, the Additional Periodic
//      Payments Stop Date, and the PV Periodic Payments Through date;
//   2. the two DIFFERENT sentences a moved Last Pmt Date can earn — a genuine
//      grid snap, and the case where # Periods overrode the date entirely
//      (round 64: the second was being reported as the first, which was false);
//   3. the §79 compaction hazard on the prepayment grid — an EMPTY row above a
//      filled one must not send the echo to the wrong row;
//   4. three layout fixes: one screen title (not two), the tooltip badge inside
//      its label box, and #pv-error above the Variable Rate Schedule;
//   5. that a declined auto-calculate says something on a worksheet that has
//      never computed.
//
// 🚨 THE TAILWIND ARM IS PART OF THE RESULT (CAUTION 23, §95). The shipped page
// gets `.hidden { display: none }` — and every other utility class — from
// cdn.tailwindcss.com. A cue built out of utility classes would be INERT in
// exactly the deployment where it matters. This harness BLOCKS the CDN by
// default and asserts the brown cue still renders, then `--tailwind` runs the
// other arm from a local copy. It prints which world it measured in either way.
//
//   node testplan/harness/r64_snap_ui_test.js <url> [--tailwind]
//
// Exit 0 = every assertion held. Non-zero = a real defect.
//
// ⚠️ POSITIVE CONTROLS (R101). Three assertions below are NEGATIVE — "an on-grid
// date earns no cue". A negative that can never fail is worthless, so each is
// paired with the positive case in the same run, over the same fields, changing
// only the date. If the positive stops firing, the pair fails.
// ⚠️ AND THE WHOLE FILE IS MUTATION-TESTED by r64_snap_mutants.sh, which reverts
// each round-64 change in turn and requires a NAMED assertion here to kill it.

const { chromium } = require('/root/node_modules/playwright');
const fs = require('fs');

const URL = process.argv[2] || 'http://127.0.0.1:8833/';
const WANT_TAILWIND = process.argv.includes('--tailwind');
const TW_LOCAL = '/tmp/tw.js';

let pass = 0, fail = 0;
const failures = [];
function ck(name, cond, detail) {
  if (cond) { pass++; console.log('  ok   ' + name); }
  else { fail++; failures.push(name); console.log('  FAIL ' + name + (detail ? '  -- ' + detail : '')); }
}

async function typeInto(page, sel, digits) {
  await page.click(sel);
  await page.fill(sel, '');
  // Date fields are masked and auto-advance; typing the digits one at a time is
  // the only way to get a well-formed date in (r61 paid for this: `fill` with
  // '01/01/2027' yields '01//01/202').
  for (const ch of String(digits)) await page.type(sel, ch, { delay: 8 });
}

async function freshPV(page) {
  await page.evaluate(() => { try { localStorage.removeItem('persense-worksheet-v1'); } catch (e) {} });
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(900);
  await page.evaluate(() => showScreen('presentvalue'));
  await page.waitForTimeout(250);
}

async function freshAmz(page) {
  await page.evaluate(() => { try { localStorage.removeItem('persense-worksheet-v1'); } catch (e) {} });
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(900);
  await page.evaluate(() => {
    showScreen('amortization');
    document.querySelectorAll('#screen-amortization details').forEach(d => { d.open = true; });
  });
  await page.waitForTimeout(250);
}

const PRE = '#amz-prepay-body tr:nth-child(%N%) [data-amz-prepay-field="%F%"]';
const pre = (row, field) => PRE.replace('%N%', row).replace('%F%', field);

(async () => {
  const browser = await chromium.launch({
    executablePath: '/opt/pw-browsers/chromium-1194/chrome-linux/chrome',
    args: ['--no-sandbox'],
  });
  const page = await browser.newPage({ viewport: { width: 1500, height: 1000 } });

  const pageErrors = [];
  page.on('pageerror', e => pageErrors.push(e.message));

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
    try { localStorage.clear(); localStorage.setItem('persense-tour-done', '1'); } catch (e) {}
  });
  await page.goto(URL, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1300);

  const twLoaded = await page.evaluate(() => typeof window.tailwind !== 'undefined');
  console.log('\nTAILWIND LOADED: ' + twLoaded +
    (WANT_TAILWIND ? '  (arm: local copy at ' + TW_LOCAL + ' fulfilled via page.route)'
                   : '  (arm: CDN BLOCKED — the offline case)'));
  if (!WANT_TAILWIND) {
    ck('the arm is what it claims (tailwind genuinely absent)', !twLoaded,
       'tailwind loaded despite the block — this run measures the wrong world');
  } else {
    ck('the tailwind arm actually served the local copy', tailwindServed);
  }

  // ------------------------------------------------------------------ §95 ---
  // 🚨 THIS ASSERTION IS THE OPPOSITE OF ROUND 63'S, AND DELIBERATELY SO.
  // r63's harness asserted that with the CDN blocked the `.hidden` CLASS is
  // INERT, and noted that if it ever started hiding, §95 had been fixed and the
  // control was stale. Round 64 fixed that one class in the inline stylesheet —
  // because the inert form is not cosmetic: the CLOSED Computational Settings
  // overlay computed `display:flex`, covered the viewport and swallowed every
  // click, and this harness could not drive the offline page at all until it was
  // fixed (playwright: "#modal-settings … intercepts pointer events").
  //
  // ⚠️ §95 IS NOT RETIRED. Every OTHER utility class on the page is still remote;
  // only `.hidden` has a local fallback. The second probe below is what actually
  // matters — the dialog must be dismissible offline — and the third keeps the
  // scope honest by showing a different utility is still missing.
  if (!twLoaded) {
    const probe = await page.evaluate(() => {
      const d = document.createElement('div');
      d.className = 'hidden'; d.textContent = 'probe';
      document.body.appendChild(d);
      const hides = getComputedStyle(d).display === 'none';
      d.remove();
      const u = document.createElement('div');
      u.className = 'flex'; document.body.appendChild(u);
      const utilLive = getComputedStyle(u).display === 'flex';
      u.remove();
      const modal = document.getElementById('modal-settings');
      return {
        hides: hides,
        utilLive: utilLive,
        modalDisplay: modal ? getComputedStyle(modal).display : 'no-modal',
      };
    });
    ck('§95 fallback: the .hidden CLASS hides with the CDN blocked', probe.hides === true,
       JSON.stringify(probe));
    ck('§95 fallback: the CLOSED settings overlay is not painted over the page',
       probe.modalDisplay === 'none', JSON.stringify(probe));
    // Scope control: the fallback is ONE rule, not a vendored Tailwind.
    ck('§95 is still open — other utility classes are still absent offline',
       probe.utilLive === false,
       'a utility class other than .hidden now resolves offline — has Tailwind been vendored? retire this control if so');
    // ⚠️ AND THE REAL PROOF IS THE REST OF THIS RUN. Every click below happens
    // on the offline page; before the fallback rule, playwright could not click
    // a single worksheet cell — #modal-settings intercepted all of them.
  }

  // ---------------------------------------------------- the cue is not a
  // ---------------------------------------------------- utility class -------
  const cueLive = await page.evaluate(() => {
    const a = document.createElement('input');
    const b = document.createElement('input');
    a.className = 'grid-cell'; b.className = 'grid-cell cell-snapped';
    document.body.appendChild(a); document.body.appendChild(b);
    const ca = getComputedStyle(a), cb = getComputedStyle(b);
    const r = { plain: ca.borderTopColor, snapped: cb.borderTopColor, shadow: cb.boxShadow };
    a.remove(); b.remove(); return r;
  });
  ck('the brown cue renders from the INLINE stylesheet (no CDN needed)',
     cueLive.plain !== cueLive.snapped && cueLive.shadow && cueLive.shadow !== 'none',
     JSON.stringify(cueLive));

  // ================================================== LAYOUT =================
  const titles = await page.evaluate(() => {
    const out = {};
    ['mortgage', 'amortization', 'presentvalue'].forEach(function (s) {
      const root = document.getElementById('screen-' + s);
      out[s] = {
        h2: [...root.querySelectorAll('h2.screen-title')].map(e => e.textContent.trim()),
        toolbarLabels: [...root.querySelectorAll('.screen-toolbar span.font-semibold')]
          .map(e => e.textContent.trim()),
      };
    });
    return out;
  });
  ck('each screen has exactly ONE title heading',
     titles.mortgage.h2.length === 1 && titles.amortization.h2.length === 1 &&
     titles.presentvalue.h2.length === 1, JSON.stringify(titles));
  ck('the duplicate toolbar title is gone on all three screens',
     titles.mortgage.toolbarLabels.length === 0 &&
     titles.amortization.toolbarLabels.length === 0 &&
     titles.presentvalue.toolbarLabels.length === 0, JSON.stringify(titles));
  ck('the titles say the right thing',
     titles.mortgage.h2[0] === 'Mortgage' &&
     titles.amortization.h2[0] === 'Amortization' &&
     titles.presentvalue.h2[0] === 'Present Value', JSON.stringify(titles));

  await page.evaluate(() => showScreen('presentvalue'));
  await page.waitForTimeout(300);

  const order = await page.evaluate(() => {
    const err = document.getElementById('pv-error');
    const vr = document.getElementById('pv-vr-section');
    const per = document.getElementById('pv-per-grid');
    if (!err || !vr || !per) return { missing: true };
    // Node.compareDocumentPosition: 4 = FOLLOWING (other comes after `this`).
    return {
      errAfterGrid: !!(per.compareDocumentPosition(err) & Node.DOCUMENT_POSITION_FOLLOWING),
      errBeforeVarRate: !!(err.compareDocumentPosition(vr) & Node.DOCUMENT_POSITION_FOLLOWING),
    };
  });
  ck('#pv-error sits AFTER the periodic grid and BEFORE the Variable Rate Schedule',
     order.errAfterGrid === true && order.errBeforeVarRate === true, JSON.stringify(order));

  // 🚨 THIS ASSERTION IS ONLY MEANINGFUL IN THE TAILWIND ARM, AND THE MUTATION
  // HARNESS IS WHAT PROVED IT. The labels are `class="grid-header inline-block"`
  // — and `inline-block` is a TAILWIND UTILITY. With the CDN blocked the label
  // computes `display: inline`, on which `width` has no effect at all, so the box
  // grows to fit its content and NOTHING can ever overflow it. Run blind, the
  // pixel check passed against a page with the fix reverted: a vacuous green,
  // R84's "reach is not power" on a layout assertion.
  //
  // So the pixel measurement runs ONLY when Tailwind is loaded, and the blocked
  // arm asserts the CSS contract instead — which is the thing a stylesheet can
  // actually promise offline.
  // ⚠️ AND THE OVERFLOW IS FONT-DEPENDENT. Measured on the pre-fix page in this
  // container: As-of Date +11.9px, True Rate +21.1, Loan Rate +24, Present Value
  // +13.1, Yield inside. The client's report was from macOS, where the metrics
  // differ; a run that finds no overflow on some other font is not evidence the
  // fix was unnecessary, which is why the contract check exists as well.
  const tipLabels = await page.evaluate(() => {
    const out = [];
    document.querySelectorAll('#screen-presentvalue label.grid-header').forEach(function (l) {
      const t = l.querySelector('.tip');
      if (!t) return;
      const lb = l.getBoundingClientRect(), tb = t.getBoundingClientRect();
      out.push({
        text: l.textContent.trim().slice(0, 18),
        overflow: +(tb.right - lb.right).toFixed(1),
        inlineStyle: l.getAttribute('style') || '',
        display: getComputedStyle(l).display,
      });
    });
    return out;
  });
  ck('the PV top labels exist and were measured at all',
     tipLabels.length === 5, JSON.stringify(tipLabels));
  const fixedWidth = tipLabels.filter(l => /(^|;)\s*width\s*:/.test(l.inlineStyle));
  ck('no PV top label pins a FIXED width (the contract, measurable offline)',
     fixedWidth.length === 0,
     JSON.stringify(fixedWidth.map(l => l.text + ' ' + l.inlineStyle)));
  if (twLoaded) {
    const bad = tipLabels.filter(l => l.overflow > 1);
    ck('every PV label keeps its ? badge inside the label box (pixels, tailwind arm)',
       bad.length === 0, JSON.stringify(bad));
    // Positive control for the measurement itself: the labels must be laid out
    // as inline-block, or the box cannot overflow and the check above is empty.
    ck('the pixel check has power (labels are inline-block in this arm)',
       tipLabels.every(l => l.display === 'inline-block'),
       JSON.stringify(tipLabels.map(l => l.text + '=' + l.display)));
  }

  // ============================================== PV PERIODIC THROUGH ========
  // POSITIVE: an off-grid Through date is snapped, repainted, cued and stated.
  await freshPV(page);
  await typeInto(page, '#pv-asOfDate', '08202026');
  await typeInto(page, 'input[data-per="0"][data-f="from"]', '01102026');
  await typeInto(page, 'input[data-per="0"][data-f="to"]', '03232056');
  await typeInto(page, 'input[data-per="0"][data-f="amount"]', '200');
  await page.evaluate(() => { calcPV(); });
  await page.waitForTimeout(1600);
  const pvOff = await page.evaluate(() => {
    const el = document.querySelector('input[data-per="0"][data-f="to"]');
    return {
      value: el.value,
      snapped: el.classList.contains('cell-snapped'),
      green: el.classList.contains('cell-output'),
      title: el.title,
      advisory: document.getElementById('pv-error').textContent,
      total: document.getElementById('pv-total').value,
    };
  });
  ck('PV: an off-grid Through date is repainted with the date the engine used',
     pvOff.value === '03/10/2056', pvOff.value);
  ck('PV: the moved Through cell carries the brown cue', pvOff.snapped === true);
  ck('PV: a snapped cell is NOT marked as computed output (green)', pvOff.green === false);
  ck('PV: the moved Through cell carries the sentence as its title',
     /Through 03\/23\/2056 is not on the payment schedule/.test(pvOff.title), pvOff.title);
  ck('PV: the advisory names the row, the typed date and the date used',
     /Periodic row 1/.test(pvOff.advisory) &&
     /03\/23\/2056/.test(pvOff.advisory) && /03\/10\/2056/.test(pvOff.advisory),
     pvOff.advisory);
  ck('PV: the total is the one the engine computed for the snapped grid',
     pvOff.total === '$34,876.83', pvOff.total);

  // the cue must retract the instant the user types in the cell
  await typeInto(page, 'input[data-per="0"][data-f="to"]', '03102056');
  const pvAfterEdit = await page.evaluate(() => {
    const el = document.querySelector('input[data-per="0"][data-f="to"]');
    return { snapped: el.classList.contains('cell-snapped'), title: el.title };
  });
  ck('PV: typing in a cued cell drops the cue immediately',
     pvAfterEdit.snapped === false && pvAfterEdit.title === '', JSON.stringify(pvAfterEdit));

  // NEGATIVE (paired with the positive above — same fields, on-grid date):
  await page.evaluate(() => { calcPV(); });
  await page.waitForTimeout(1400);
  const pvOn = await page.evaluate(() => {
    const el = document.querySelector('input[data-per="0"][data-f="to"]');
    return {
      value: el.value, snapped: el.classList.contains('cell-snapped'),
      advisory: document.getElementById('pv-error').textContent.trim(),
      total: document.getElementById('pv-total').value,
    };
  });
  ck('PV: an ON-grid Through date earns no cue and no sentence',
     pvOn.snapped === false && pvOn.advisory === '' && pvOn.value === '03/10/2056',
     JSON.stringify(pvOn));
  ck('PV: and the on-grid run gives the same total the snapped one did',
     pvOn.total === '$34,876.83', pvOn.total);

  // ======================================= AMORTIZATION: LAST PMT DATE =======
  // POSITIVE: a genuine grid snap (# Periods cleared so the DATE sets the term).
  await freshAmz(page);
  await page.evaluate(() => { document.getElementById('amz-nPeriods').value = ''; });
  await typeInto(page, '#amz-amount', '100000');
  await typeInto(page, '#amz-loanDate', '01012026');
  await typeInto(page, '#amz-rate', '6');
  await typeInto(page, '#amz-firstDate', '02012026');
  await typeInto(page, '#amz-lastDate', '03232056');
  await page.evaluate(() => { calcAmortization(); });
  await page.waitForTimeout(2500);
  const amzSnap = await page.evaluate(() => {
    const el = document.getElementById('amz-lastDate');
    return {
      value: el.value, snapped: el.classList.contains('cell-snapped'), title: el.title,
      n: document.getElementById('amz-nPeriods').value,
      advisory: document.getElementById('amz-error').textContent,
    };
  });
  ck('AMZ: an off-grid Last Pmt Date is snapped onto the grid',
     amzSnap.value === '03/01/2056' && amzSnap.n === '362', JSON.stringify(amzSnap));
  ck('AMZ: the snapped Last Pmt Date carries the brown cue', amzSnap.snapped === true);
  ck('AMZ: the snap sentence says "nearest scheduled payment"',
     /nearest scheduled payment/.test(amzSnap.advisory), amzSnap.advisory.slice(0, 200));

  // ---- DOS's mutual exclusion (AmortizationScreenUnit.pas:1281-1301) --------
  // Typing into Last Pmt Date must BLANK # Periods, and vice versa. This is what
  // makes the over-determined state unreachable from the keyboard, as it is in
  // DOS. The page ships # Periods pre-filled with 360, so without this rule a
  // typed Last Pmt Date is silently discarded on every fresh worksheet.
  await freshAmz(page);
  const nBefore = await page.evaluate(() => document.getElementById('amz-nPeriods').value);
  ck('the page still ships # Periods pre-filled (the precondition for the trap)',
     nBefore === '360', nBefore);
  await typeInto(page, '#amz-lastDate', '03232056');
  const afterLast = await page.evaluate(() => ({
    n: document.getElementById('amz-nPeriods').value,
    last: document.getElementById('amz-lastDate').value,
  }));
  ck('typing a Last Pmt Date blanks # Periods (DOS AmortGridCellBeforeEdit)',
     afterLast.n === '' && afterLast.last === '03/23/2056', JSON.stringify(afterLast));
  await typeInto(page, '#amz-nPeriods', '240');
  const afterN = await page.evaluate(() => ({
    n: document.getElementById('amz-nPeriods').value,
    last: document.getElementById('amz-lastDate').value,
  }));
  ck('typing # Periods blanks the Last Pmt Date (the other direction)',
     afterN.n === '240' && afterN.last === '', JSON.stringify(afterN));

  // THE OTHER SENTENCE: # Periods present, so the COUNT decides and the date is
  // discarded outright. Reporting this as a grid snap was false (round 64).
  //
  // ⚠️ REACHED PROGRAMMATICALLY ON PURPOSE. With the mutual exclusion above in
  // place a user cannot type their way into this state — but a restored
  // worksheet, a .psn import, a journey preset and any direct API caller still
  // can, so the sentence must still be right. Setting `.value` fires no `input`
  // event, which is exactly how those paths reach it too.
  await freshAmz(page);
  await typeInto(page, '#amz-amount', '100000');
  await typeInto(page, '#amz-loanDate', '01012026');
  await typeInto(page, '#amz-rate', '6');
  await typeInto(page, '#amz-firstDate', '02012026');
  await page.evaluate(() => {
    document.getElementById('amz-nPeriods').value = '360';
    document.getElementById('amz-lastDate').value = '03/23/2056';
  });
  await page.evaluate(() => { calcAmortization(); });
  await page.waitForTimeout(2500);
  const amzOverride = await page.evaluate(() => {
    const el = document.getElementById('amz-lastDate');
    return {
      value: el.value, snapped: el.classList.contains('cell-snapped'),
      n: document.getElementById('amz-nPeriods').value,
      advisory: document.getElementById('amz-error').textContent,
    };
  });
  ck('AMZ: with # Periods filled the count wins and the date is discarded',
     amzOverride.value === '01/01/2056' && amzOverride.n === '360', JSON.stringify(amzOverride));
  ck('AMZ: that case is NOT described as a snap to the nearest payment',
     !/nearest scheduled payment/.test(amzOverride.advisory), amzOverride.advisory.slice(0, 240));
  ck('AMZ: it names # Periods as the reason and says how to change it',
     /# Periods \(360\)/.test(amzOverride.advisory) &&
     /Clear # Periods/.test(amzOverride.advisory), amzOverride.advisory.slice(0, 240));
  ck('AMZ: the overridden Last Pmt Date still carries a cue',
     amzOverride.snapped === true);

  // ================================ AMORTIZATION: PREPAYMENT STOP DATE =======
  await freshAmz(page);
  await page.evaluate(() => { document.getElementById('amz-nPeriods').value = ''; });
  await typeInto(page, '#amz-amount', '100000');
  await typeInto(page, '#amz-loanDate', '01012026');
  await typeInto(page, '#amz-rate', '6');
  await typeInto(page, '#amz-firstDate', '02012026');
  await typeInto(page, '#amz-lastDate', '03012056');
  await typeInto(page, pre(1, 'startDate'), '02012026');
  await typeInto(page, pre(1, 'stopDate'), '03232056');
  await typeInto(page, pre(1, 'perYr'), '12');
  await typeInto(page, pre(1, 'amount'), '300');
  await page.evaluate(() => { calcAmortization(); });
  await page.waitForTimeout(2500);
  const prepayOff = await page.evaluate(() => {
    const el = document.querySelector('#amz-prepay-body tr:nth-child(1) [data-amz-prepay-field="stopDate"]');
    const n = document.querySelector('#amz-prepay-body tr:nth-child(1) [data-amz-prepay-field="nPmts"]');
    return {
      stop: el.value, snapped: el.classList.contains('cell-snapped'), title: el.title,
      nPmts: n.value, nGreen: n.classList.contains('cell-output'),
      advisory: document.getElementById('amz-error').textContent,
    };
  });
  ck('AMZ prepay: an off-grid Stop Date is repainted with the series\' real end',
     prepayOff.stop === '03/01/2056', prepayOff.stop);
  ck('AMZ prepay: the moved Stop Date carries the brown cue', prepayOff.snapped === true);
  ck('AMZ prepay: the advisory names the row and both dates',
     /Additional payments row 1/.test(prepayOff.advisory) &&
     /03\/23\/2056/.test(prepayOff.advisory) && /03\/01\/2056/.test(prepayOff.advisory),
     prepayOff.advisory.slice(0, 300));
  ck('AMZ prepay: the derived # Pmts is still painted green in the same row',
     prepayOff.nPmts === '362' && prepayOff.nGreen === true, JSON.stringify(prepayOff));

  // NEGATIVE, paired: the same row with an ON-grid Stop Date.
  await typeInto(page, pre(1, 'stopDate'), '03012056');
  await page.evaluate(() => { calcAmortization(); });
  await page.waitForTimeout(2500);
  const prepayOn = await page.evaluate(() => {
    const el = document.querySelector('#amz-prepay-body tr:nth-child(1) [data-amz-prepay-field="stopDate"]');
    return { stop: el.value, snapped: el.classList.contains('cell-snapped'),
             advisory: document.getElementById('amz-error').textContent };
  });
  ck('AMZ prepay: an ON-grid Stop Date earns no cue and no sentence',
     prepayOn.snapped === false && !/Additional payments row/.test(prepayOn.advisory),
     JSON.stringify(prepayOn).slice(0, 240));

  // ------------------------------------- §79 compaction on the prepay grid ---
  // An EMPTY row ABOVE a filled one. The request carries ONE prepayment, at
  // index 0; the DOM row is index 1. Every echo indexed by request order must
  // land on DOM row 2, not DOM row 1.
  await freshAmz(page);
  await page.evaluate(() => { document.getElementById('amz-nPeriods').value = ''; });
  await typeInto(page, '#amz-amount', '100000');
  await typeInto(page, '#amz-loanDate', '01012026');
  await typeInto(page, '#amz-rate', '6');
  await typeInto(page, '#amz-firstDate', '02012026');
  await typeInto(page, '#amz-lastDate', '03012056');
  await typeInto(page, pre(2, 'startDate'), '02012026');
  await typeInto(page, pre(2, 'stopDate'), '03232056');
  await typeInto(page, pre(2, 'perYr'), '12');
  await typeInto(page, pre(2, 'amount'), '300');
  await page.evaluate(() => { calcAmortization(); });
  await page.waitForTimeout(2500);
  const compaction = await page.evaluate(() => {
    const cell = (r, f) => document.querySelector(
      '#amz-prepay-body tr:nth-child(' + r + ') [data-amz-prepay-field="' + f + '"]');
    return {
      row1stop: cell(1, 'stopDate').value, row1n: cell(1, 'nPmts').value,
      row1snapped: cell(1, 'stopDate').classList.contains('cell-snapped'),
      row2stop: cell(2, 'stopDate').value, row2n: cell(2, 'nPmts').value,
      row2snapped: cell(2, 'stopDate').classList.contains('cell-snapped'),
      advisory: document.getElementById('amz-error').textContent,
    };
  });
  ck('§79: the echo lands on the FILLED row (2), not the request index (row 1)',
     compaction.row2stop === '03/01/2056' && compaction.row2n === '362' &&
     compaction.row2snapped === true, JSON.stringify(compaction));
  ck('§79: the empty row above it is left completely untouched',
     compaction.row1stop === '' && compaction.row1n === '' &&
     compaction.row1snapped === false, JSON.stringify(compaction));
  ck('§79: and the sentence names row 2',
     /Additional payments row 2/.test(compaction.advisory),
     compaction.advisory.slice(0, 300));

  // ================================= AUTO-CALCULATE DECLINES OUT LOUD ========
  // A worksheet that has never computed: Enter used to do nothing and say
  // nothing. It must now say why.
  await freshPV(page);
  await page.evaluate(() => { document.getElementById('pv-rate-loan').value = ''; onPVRateEdit(document.getElementById('pv-rate-loan')); });
  await typeInto(page, '#pv-asOfDate', '08202026');
  await typeInto(page, '#pv-ls-body tr:nth-child(1) [data-f="date"]', '08202027');
  await typeInto(page, '#pv-ls-body tr:nth-child(1) [data-f="amount"]', '12000');
  await page.press('#pv-ls-body tr:nth-child(1) [data-f="amount"]', 'Enter');
  await page.waitForTimeout(1800);
  const declined = await page.evaluate(() => {
    const h = document.getElementById('pv-autocalc-hint');
    return { text: h.textContent, total: document.getElementById('pv-total').value };
  });
  ck('a declined auto-calculate explains itself on a never-computed worksheet',
     declined.total === '' && declined.text.length > 0, JSON.stringify(declined));

  // And the ordinary case still computes and clears the hint.
  await typeInto(page, '#pv-rate-loan', '6');
  await page.press('#pv-rate-loan', 'Enter');
  await page.waitForTimeout(1800);
  const computed = await page.evaluate(() => ({
    total: document.getElementById('pv-total').value,
    hint: document.getElementById('pv-autocalc-hint').textContent,
  }));
  ck('auto-calculate still computes on Enter and clears the hint',
     computed.total === '$11,302.86' && computed.hint === '', JSON.stringify(computed));

  // --------------------------------------------------------------------------
  ck('no uncaught page errors during the whole run', pageErrors.length === 0,
     pageErrors.join(' | '));

  console.log('\n' + pass + ' passed, ' + fail + ' failed' +
    '   [tailwind=' + (twLoaded ? 'LOADED' : 'BLOCKED') + ']');
  if (fail) console.log('FAILED: ' + failures.join(' ; '));
  await browser.close();
  process.exit(fail ? 1 : 0);
})().catch(e => { console.error(e); process.exit(2); });
