// uidiff/driver.js — drives the REAL shipped page and reads DISPLAY space.
//
// The distinction that makes this instrument different from every engine
// differential the project owns, and from testplan/harness/run_amz.js:
// run_amz.js reads `amzScheduleData.result` — the JS OBJECT the fetch returned.
// That is engine space. A value that the engine computes correctly, transports
// correctly, and then PAINTS wrongly is invisible to it. Four of the
// amortization screen's last five defects lived in exactly that gap.
//
// So everything below is read from the DOM the user actually sees:
//   totals   <- #amz-total-paid / #amz-total-int textContent
//   payment  <- the painted schedule table cells
//   apr      <- the #amz-apr input's value
//   adj grid <- the painted [data-amz-adj-field] cell values
//
// 🚨 TRAP 3 IS ENFORCED HERE. Every date written into the page is first fed
// through the page's OWN dateValidity() (index.html:6592) and the run ABORTS if
// the page would refuse it. ISO dates blocked 45b's entire first browser pass.
// And every date the page PAINTS BACK is fed through the same validator on the
// way out — R53: a painted cell is an input to the next submit.

'use strict';

const { toUI } = require('./tokens');

const URL = process.env.PERSENSE_URL || 'http://localhost:8080/';

async function open(chromium) {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1400, height: 1000 } });
  // 🚨 A modal dialog freezes the whole automation channel. We never call
  // clearAmortization() (it calls confirm()), but a stray dialog must not
  // deadlock the run.
  page.on('dialog', d => d.accept().catch(() => {}));
  const pageErrors = [];
  page.on('pageerror', e => pageErrors.push(e.message));
  await page.goto(URL, { waitUntil: 'networkidle' });
  await page.addStyleTag({ content: '.modal-overlay{display:none !important;}' });
  await page.evaluate(() => showScreen('amortization'));
  return { browser, page, pageErrors };
}

// Verify the SERVED page is the page on disk. cmd/persense/main.go:23 is
// //go:embed static, so a running binary serves a SNAPSHOT: a fix on disk is
// not live until the binary is rebuilt. Round 45's broken build was tested for
// a whole session because nobody checked this.
async function assertServedBuild(page, markers) {
  const missing = await page.evaluate((ms) => {
    const html = document.documentElement.outerHTML;
    return ms.filter(m => !html.includes(m));
  }, markers);
  if (missing.length) {
    throw new Error(
      `SERVED BUILD IS STALE: the running server does not serve ${missing.length} ` +
      `expected marker(s): ${missing.join(' | ')}. cmd/persense/main.go:23 is ` +
      `//go:embed static — rebuild and restart the binary before trusting any ` +
      `UI result.`);
  }
}

// One case through the live page. Returns DISPLAY-space values.
async function runCase(page, c) {
  const ui = {
    amount: String(c.amount),
    rate: String(c.rate * 100),        // the page's rate field is a percentage
    nPeriods: String(c.nPeriods),
    perYr: String(c.settings.perYr),
    // 🚨 The page reads amz-points with parseRate (index.html:3423), which is
    // PERCENT space — exactly like the rate field. The oracle's pts= is a
    // FRACTION. Typing the fraction into the field makes 1% of points read as
    // 0.01% and the APR comes back a whisker above the note rate instead of
    // ~17bp above it. The one-option-at-a-time tier attributed this directly.
    points: String((c.points || 0) * 100),
    loanDate: c.loanDate ? toUI(c.loanDate) : '',
    firstDate: c.firstDate ? toUI(c.firstDate) : '',
    moratorium: c.moratorium ? toUI(c.moratorium) : '',
    target: c.target != null ? String(c.target) : '',
    skipMonths: c.skipMonths || '',
    settings: c.settings,
    balloons: (c.balloons || []).map(b => ({ date: toUI(b.date), amount: String(b.amount) })),
    prepays: (c.prepays || []).map(p => ({
      startDate: toUI(p.startDate), nPmts: String(p.nPmts),
      perYr: String(p.perYr), amount: String(p.amount),
    })),
    adjustments: (c.adjustments || []).map(a => ({
      date: toUI(a.date),
      rate: a.rate === '' || a.rate == null ? '' : String(a.rate * 100),
      amount: a.amount === '' || a.amount == null ? '' : String(a.amount),
    })),
    // 🚨 THE DATE-EARLIEST ADJUSTMENT, NOT adjustments[0]. The grid keeps rows
    // in the order the user typed them; DOS walks the schedule in DATE order.
    // Using the insertion-order first adjustment cuts the "first segment" at
    // the WRONG date whenever the later adjustment was typed first, and the
    // modal over two payment regimes is then compared to DOS's single regular
    // payment. Measured on stacked-65: cut at 09/13/2025 while the real first
    // adjustment was 08/13/2024, giving 5280.30 against DOS's 5344.8671 — and
    // the true first-segment payment matched DOS to 0.003. This is the SAME
    // index-vs-date fault compare.js fixes for the adjustment cells, forty
    // lines away, reintroduced in the sibling module. R52 twice in one round.
    prepayDates: (c.prepays || []).flatMap(p => {
      // every date in the series, so those rows can be excluded from the modal
      const out2 = []; const mpp = 12 / p.perYr;
      for (let j = 0; j < p.nPmts; j++) {
        const zero = (p.startDate.y * 12 + p.startDate.m - 1) + j * mpp;
        out2.push(toUI({ y: Math.floor(zero / 12), m: (zero % 12) + 1, d: p.startDate.d }));
      }
      return out2;
    }),
    firstAdjDate: (c.adjustments && c.adjustments.length)
      ? toUI(c.adjustments.slice().sort((a, b) =>
          (a.date.y - b.date.y) || (a.date.m - b.date.m) || (a.date.d - b.date.d))[0].date)
      : null,
  };

  return await page.evaluate(async (ui) => {
    const out = { inputDateRejects: [], paintedDateRejects: [] };

    // ---- TRAP 3, input side: the page must ACCEPT every date we type -------
    const checkIn = (label, v) => {
      if (!v) return;
      const r = dateValidity(v);
      if (!r.valid) out.inputDateRejects.push(`${label}="${v}": ${r.msg}`);
    };

    // ---- reset. We never call clearAmortization(): it calls confirm(). -----
    const setv = (id, v) => {
      const e = document.getElementById(id);
      if (e) { e.value = v; if (e.classList) e.classList.remove('cell-output'); }
    };
    ['amz-amount', 'amz-loanDate', 'amz-rate', 'amz-firstDate', 'amz-nPeriods',
     'amz-lastDate', 'amz-payment', 'amz-moratorium', 'amz-targetAmt',
     'amz-skipMonths', 'amz-payoff-date', 'amz-payoff-bal', 'amz-apr']
      .forEach(id => setv(id, ''));
    setv('amz-perYr', '12'); setv('amz-points', '0');
    document.querySelectorAll(
      '#amz-balloon-body [data-amz-balloon-field], #amz-prepay-body [data-amz-prepay-field], #amz-adj-body [data-amz-adj-field]'
    ).forEach(e => { e.value = ''; if (e.classList) e.classList.remove('cell-output'); });
    // 🚨 The page RESTORES the last session's worksheet, and a BLOCKED calc
    // leaves the PREVIOUS totals standing. 45b nearly scored those as results.
    // Clear the display cells themselves, not just the data behind them.
    const tp = document.getElementById('amz-total-paid');
    const ti = document.getElementById('amz-total-int');
    if (tp) tp.textContent = '';
    if (ti) ti.textContent = '';
    const schedBody = document.querySelector('#amz-schedule tbody');
    if (schedBody) schedBody.innerHTML = '';
    const errEl0 = document.getElementById('amz-error');
    if (errEl0) errEl0.textContent = '';
    if (typeof amzScheduleData !== 'undefined') amzScheduleData = null;
    const setSel = (id, v) => { const e = document.getElementById(id); if (e) e.value = v; };
    setSel('set-perYr', '12'); setSel('set-basis', '360'); setSel('set-prepaid', 'yes');
    setSel('set-timing', 'arrears'); setSel('set-balloonIncl', 'no');
    setSel('set-exact', 'no'); setSel('set-rule78', 'no'); setSel('set-interestRule', 'actuarial');

    // ---- apply the case ---------------------------------------------------
    setv('amz-amount', ui.amount); setv('amz-rate', ui.rate);
    setv('amz-nPeriods', ui.nPeriods); setv('amz-points', ui.points);
    setv('amz-loanDate', ui.loanDate); setv('amz-firstDate', ui.firstDate);
    checkIn('loanDate', ui.loanDate); checkIn('firstDate', ui.firstDate);
    if (ui.moratorium) { setv('amz-moratorium', ui.moratorium); checkIn('moratorium', ui.moratorium); }
    if (ui.target) setv('amz-targetAmt', ui.target);
    if (ui.skipMonths) setv('amz-skipMonths', ui.skipMonths);
    const s = ui.settings;
    setSel('set-perYr', String(s.perYr)); setSel('set-basis', s.basis);
    setSel('set-prepaid', s.prepaid); setSel('set-timing', s.timing);
    setSel('set-balloonIncl', s.balloonIncl); setSel('set-exact', s.exact);
    setSel('set-rule78', s.rule78); setSel('set-interestRule', s.interestRule);
    setv('amz-perYr', String(s.perYr));

    const fillGrid = (body, attr, rows, dateKeys) => {
      const trs = document.querySelectorAll(body + ' tr');
      (rows || []).forEach((row, i) => {
        const tr = trs[i];
        if (!tr) { out.gridOverflow = true; return; }
        Object.keys(row).forEach(k => {
          const cell = tr.querySelector('[' + attr + '="' + k + '"]');
          if (cell) cell.value = row[k];
          if (dateKeys.includes(k)) checkIn(`${attr}[${i}].${k}`, row[k]);
        });
      });
    };
    fillGrid('#amz-balloon-body', 'data-amz-balloon-field', ui.balloons, ['date']);
    fillGrid('#amz-prepay-body', 'data-amz-prepay-field', ui.prepays, ['startDate']);
    fillGrid('#amz-adj-body', 'data-amz-adj-field', ui.adjustments, ['date']);

    if (out.inputDateRejects.length) return out; // abort before submitting

    let threw = null;
    try { await calcAmortization(); } catch (e) { threw = e && e.message; }
    out.threw = threw;

    const errEl = document.getElementById('amz-error');
    out.err = errEl ? errEl.textContent.trim() : '';

    // ---- READ DISPLAY SPACE ----------------------------------------------
    const txt = id => { const e = document.getElementById(id); return e ? e.textContent.trim() : ''; };
    out.totalPaidText = txt('amz-total-paid');
    out.totalIntText = txt('amz-total-int');
    // Parse the PAINTED strings back, through the page's own money parser
    // where the format allows — this is display space, not engine space.
    const money = t => {
      const m = t.match(/\$\s*(-?[\d,]+\.?\d*)/);
      return m ? parseFloat(m[1].replace(/,/g, '')) : null;
    };
    out.totalPaid = money(out.totalPaidText);
    out.totalInterest = money(out.totalIntText);

    const aprEl = document.getElementById('amz-apr');
    out.aprCell = aprEl ? aprEl.value : '';
    // 🚨 The page's own parseRate (index.html:1623) strips '%' and returns
    // PERCENT space — parseRate('7.0000%') === 7, not 0.07. The oracle's apr is
    // in DECIMAL space. The /100 below is the harness's conversion and it is
    // asserted live in selftest.liveScalarAssertions, because a silent change
    // to parseRate would make every APR comparison off by 100× — which is what
    // the identity controls caught on this harness's first run.
    out.apr = out.aprCell ? parseRate(out.aprCell) / 100 : null;

    // Painted schedule: modal payment from the rendered cells.
    const rows = Array.from(document.querySelectorAll('#amz-schedule tbody tr'));
    out.paintedRows = rows.length;
    const counts = {};
    let lastBalance = null;
    let firstPayment = null;
    rows.forEach(tr => {
      const tds = tr.querySelectorAll('td');
      if (tds.length < 6) return;
      const pay = parseMoney(tds[2].textContent);
      const bal = parseMoney(tds[5].textContent);
      if (pay > 0) {
        const k = Math.round(pay * 100);
        counts[k] = (counts[k] || 0) + 1;
        if (firstPayment == null) firstPayment = pay;
      }
      if (bal != null && !isNaN(bal)) lastBalance = bal;
    });
    let modal = null, best = -1;
    Object.keys(counts).forEach(k => { if (counts[k] > best) { best = counts[k]; modal = +k / 100; } });
    out.modalPayment = modal;
    out.lastBalance = lastBalance;
    out.firstPayment = firstPayment;
    // 🚨 WHICH PAINTED PAYMENT IS DOS'S `payment` LINE? NEITHER THE FIRST NOR
    // THE MODAL, AND THIS COST TWO MEASUREMENT PASSES TO GET RIGHT.
    //   * The FIRST painted row is wrong whenever the schedule opens with an
    //     anomalous installment: in-advance, points, a moratorium's
    //     interest-only rows, a target row. Measured: first=2016.08 while DOS
    //     said 2175.3808 and the MODAL matched to four decimals.
    //   * The MODAL is wrong on any adjustment screen, because an adjustment
    //     early in a long term makes the POST-adjustment payment the most
    //     common one while DOS reports the PRE-adjustment regular payment.
    //     Measured: modal=1231.45 while DOS said 1166.037.
    // DOS's `payment` is the REGULAR payment of the FIRST SEGMENT. So: the
    // modal payment over the rows STRICTLY BEFORE the first adjustment date,
    // and over all rows when there is no adjustment. testplan/harness/
    // run_amz.js:160 compares the plain modal and gets away with it only
    // because its 29 hand-written cases are almost all plain.
    let segRows = rows;
    if (ui.firstAdjDate) {
      const cut = rows.findIndex(tr => {
        const tds = tr.querySelectorAll('td');
        return tds.length > 1 && tds[1].textContent.trim() === ui.firstAdjDate;
      });
      if (cut > 0) segRows = rows.slice(0, cut);
    }
    // 🚨 AND EXCLUDE THE PREPAYMENT ROWS. A prepay series covering half a short
    // schedule makes the PREPAY AMOUNT the modal payment — measured on
    // stacked-1: 6 prepay rows in a 12-row schedule gave modal 35844.11
    // against DOS's regular 31659.59. That is a third distinct way the modal
    // is not DOS's `payment`, alongside the anomalous first row and the
    // post-adjustment modal.
    const prepaySet = new Set(ui.prepayDates || []);
    const segCounts = {};
    segRows.forEach(tr => {
      const tds = tr.querySelectorAll('td');
      if (tds.length < 6) return;
      if (prepaySet.has(tds[1].textContent.trim())) return;
      const pay = parseMoney(tds[2].textContent);
      if (pay > 0) { const k = Math.round(pay * 100); segCounts[k] = (segCounts[k] || 0) + 1; }
    });
    let reg = null, regBest = -1;
    Object.keys(segCounts).forEach(k => { if (segCounts[k] > regBest) { regBest = segCounts[k]; reg = +k / 100; } });
    out.regularPayment = reg;
    out.segmentRows = segRows.length;
    // A refusal is a screen with NO painted schedule at all. The page writes
    // ADVISORIES into the same #amz-error element it writes errors into
    // (index.html:3731 vs :3718), so the element's text cannot distinguish
    // them — a negative-amortization NOTE read as a refusal turned an
    // agreeing rule-78 case into a false divergence.
    out.refused = rows.length === 0;

    // Painted adjustment grid, and TRAP 3 / R53 on the way out: every date the
    // page painted back must be a date the page would ACCEPT on the next
    // submit. §89 was precisely a painted cell the app's own validator refused.
    out.paintedAdj = Array.from(document.querySelectorAll('#amz-adj-body tr')).map((tr, i) => {
      const get = k => { const e = tr.querySelector('[data-amz-adj-field="' + k + '"]'); return e ? e.value : ''; };
      const row = { i, date: get('date'), rate: get('rate'), amount: get('amount') };
      if (row.date) {
        const v = dateValidity(row.date);
        if (!v.valid) out.paintedDateRejects.push(`adj[${i}].date="${row.date}": ${v.msg}`);
      }
      return row;
    }).filter(r => r.date || r.rate || r.amount);

    out.paintedBalloon = Array.from(document.querySelectorAll('#amz-balloon-body tr')).map((tr, i) => {
      const get = k => { const e = tr.querySelector('[data-amz-balloon-field="' + k + '"]'); return e ? e.value : ''; };
      const row = { i, date: get('date'), amount: get('amount') };
      if (row.date) {
        const v = dateValidity(row.date);
        if (!v.valid) out.paintedDateRejects.push(`balloon[${i}].date="${row.date}": ${v.msg}`);
      }
      return row;
    }).filter(r => r.date || r.amount);

    return out;
  }, ui);
}

// The live-page DOUBLE-SUBMIT check — the round-45b process rule, executable.
// Calculate, then calculate again with NOTHING changed, and require that the
// second submit is not blocked and does not move the displayed totals. One
// minute of runtime; it would have caught §89 before it landed.
async function doubleSubmit(page) {
  return await page.evaluate(async () => {
    const txt = id => { const e = document.getElementById(id); return e ? e.textContent.trim() : ''; };
    const before = { paid: txt('amz-total-paid'), int: txt('amz-total-int') };
    let threw = null;
    try { await calcAmortization(); } catch (e) { threw = e && e.message; }
    const errEl = document.getElementById('amz-error');
    // 🚨 THE SAME ADVISORY-IS-NOT-A-REFUSAL RULE compare.js applies. Reading
    // #amz-error's TEXT here booked 44 false "the second Calculate was
    // BLOCKED" verdicts on screens that merely carried a note ("Loan retired
    // early", "negative amortization"). The display-space test for blocked is
    // THE SCHEDULE DISAPPEARED. This is the third time in one round that a
    // rule was applied in one module and not in its sibling.
    return {
      before, threw,
      after: { paid: txt('amz-total-paid'), int: txt('amz-total-int') },
      paintedRows: document.querySelectorAll('#amz-schedule tbody tr').length,
      err: errEl ? errEl.textContent.trim() : '',
    };
  });
}

module.exports = { open, runCase, doubleSubmit, assertServedBuild, URL };
