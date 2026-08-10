// uidiff/compare.js — the comparison, in DISPLAY space, plus the TELL.
//
// TOLERANCES ARE DECLARED HERE, IN ONE PLACE, WITH PROVENANCE.
// CAUTION 1 of START_HERE records five tolerance floors in the fuzzer, none
// pinned, and round 45 caught one of them DECIDING an APR answer (a case read
// "totals-green" while its totals differed by $4.05, under a $27.97 floor).
// This instrument will not add a sixth unpinned floor: every tolerance it uses
// is named below, carries its reason, and is PRINTED WITH EVERY RUN so that no
// rate this harness produces can be quoted without them.

'use strict';

const { KICKER } = require('./tokens');

const TOLERANCES = {
  totalPaid: {
    value: 0.02,
    why: 'display quantum. The page paints fmtMoney (2dp), so a display-space ' +
         'comparison cannot resolve below $0.01; 2 cents allows one rounding ' +
         'step on each side. NOT a relative floor — deliberately, so that a ' +
         'large-balance divergence cannot hide under a percentage.',
  },
  totalInterest: { value: 0.02, why: 'as totalPaid.' },
  payment: {
    value: 0.02,
    why: 'the painted FIRST-SEGMENT regular payment (see driver.js). One cent ' +
         'wider than dos_fuzzer5_test.go:2683 (0.011) because the page paints ' +
         'to 2dp and DOS reports 4.',
  },
  apr: {
    value: 2e-6,
    why: 'rate space. The UI renders (apr*100).toFixed(4), so the display ' +
         'quantum is 5e-7; 2e-6 matches the engine fuzzer\'s flat APR ' +
         'tolerance at dos_fuzzer5_test.go:2909 so the two instruments are ' +
         'comparable. No floor term.',
  },
  adjRate: { value: 5e-6, why: 'matches dos_fuzzer5_test.go:2616/2815 (flat, no floor).' },
  adjAmount: { value: 0.02, why: 'display quantum, as totalPaid.' },
  retiresToZero: {
    value: 1.0,
    why: 'the painted final balance. A whole dollar is deliberately loose: ' +
         'this is a sanity check on the schedule, not a convergence claim.',
  },
};

function near(a, b, tol) {
  return a != null && b != null && isFinite(a) && isFinite(b) && Math.abs(a - b) <= tol;
}

// Compare one case. Returns a list of named checks, each with its own verdict,
// so that the TELL below can ask which SET of cases each check failed on.
function compareCase(c, ui, ora) {
  const checks = [];
  const add = (k, ok, detail, tol) => checks.push({ k, ok, detail, tol });

  // A date the page refused is a harness-or-product fault, never a divergence.
  if (ui.inputDateRejects && ui.inputDateRejects.length) {
    add('input-dates-accepted', false,
      'the page REFUSED a date this harness typed: ' + ui.inputDateRejects.join('; '));
    return checks;
  }
  if (ui.gridOverflow) {
    add('ui-expressible', false, 'the case carries more grid rows than the page has');
    return checks;
  }

  // R53, at the consumer: a painted cell is an input to the next submit.
  add('painted-dates-resubmittable', !(ui.paintedDateRejects || []).length,
    (ui.paintedDateRejects || []).join('; ') || 'all painted dates pass dateValidity()');

  // 🚨 An advisory is NOT a refusal. index.html:3731 writes warnings into the
  // SAME #amz-error element as :3718 writes errors into, so the element's text
  // cannot tell them apart. The display-space test for "the app refused" is
  // that IT PAINTED NO SCHEDULE.
  const uiRefused = !!ui.threw || !!ui.refused;
  if (ora.err) {
    // DOS refused. The port must refuse too — refusal parity is a real result
    // and 45b established one such boundary (in-advance refuses ANY adjustment).
    add('both-refuse', uiRefused,
      `oracle refused ("${String(ora.err).slice(0, 60)}"), ui.err="${ui.err}" threw=${ui.threw}`);
    return checks;
  }
  if (uiRefused) {
    add('port-refused-dos-did-not', false,
      `ui.err="${ui.err}" threw=${ui.threw} while the oracle returned ` +
      `paid=${ora.data.paid}`);
    return checks;
  }

  const d = ora.data;
  if (d.paid != null) {
    add('totalPaid', near(ui.totalPaid, d.paid, TOLERANCES.totalPaid.value),
      `display="${ui.totalPaidText}" -> ${ui.totalPaid}  dos=${d.paid}`, TOLERANCES.totalPaid.value);
  }
  if (d.interest != null) {
    add('totalInterest', near(ui.totalInterest, d.interest, TOLERANCES.totalInterest.value),
      `display="${ui.totalIntText}" -> ${ui.totalInterest}  dos=${d.interest}`, TOLERANCES.totalInterest.value);
  }
  // The oracle's `payment` is the regular payment of the FIRST SEGMENT — see
  // the long note in driver.js. first and modal are reported for diagnosis.
  // 🚨 WHAT THIS CHECK CANNOT JUDGE, SAID OUT LOUD (standing rule 8).
  // A TARGET (minimum principal reduction per payment) and a MORATORIUM both
  // make the installment vary period to period, so the screen HAS no level
  // regular payment and DOS's `payment` line is the underlying solve the
  // option then overrides. Comparing them is not a divergence measurement, it
  // is a category error. Measured: every one of the 11 payment divergences in
  // the first clean stacked run carried `target` (8) or `moratorium` (3), and
  // NONE of them moved either money total by a cent.
  const paymentNotJudged = (c.target != null && c.target !== '') || !!c.moratorium;
  if (paymentNotJudged) {
    add('payment-not-judged', true,
      `payment NOT compared: this screen carries ` +
      `${[c.target ? 'a target' : null, c.moratorium ? 'a moratorium' : null].filter(Boolean).join(' and ')}` +
      `, so it has no level regular payment. Totals, APR and adjustment cells ARE compared.`);
  } else if (d.payment != null && ui.regularPayment != null) {
    add('payment', near(ui.regularPayment, d.payment, TOLERANCES.payment.value),
      `paintedRegular=${ui.regularPayment} (first=${ui.firstPayment} modal=${ui.modalPayment} ` +
      `seg=${ui.segmentRows}rows)  dos=${d.payment}`, TOLERANCES.payment.value);
  }

  // APR. `apr 0.000000 status 0` is the oracle declining to store a value
  // (amort_oracle.pas:1296; the engine stores only within `tiny`), NOT a real
  // zero — oracle.js labels it aprBlank. Comparing against it is the misread
  // trap 1 exists to prevent, so we score it as a separate, named check.
  if (ora.apr.aprBlank) {
    add('apr-blank-parity', ui.apr == null || ui.apr === 0,
      `oracle declined to store an APR (apr 0.000000 status 0); ui apr cell="${ui.aprCell}"`);
  } else if (ora.apr.apr != null && ui.apr != null) {
    add('apr', near(ui.apr, ora.apr.apr, TOLERANCES.apr.value),
      `displayCell="${ui.aprCell}" -> ${ui.apr}  dos=${ora.apr.apr}`, TOLERANCES.apr.value);
  }

  // Painted adjustment cells against the oracle's own adjustment grid.
  //
  // 🚨 JOIN ON THE DATE, NOT ON THE ROW INDEX (R52 — normalise the join key).
  // DOS SORTS the adjustment grid into date order; the page's grid keeps the
  // rows in the order the user typed them. Pairing by index on a two-row
  // screen whose rows were entered out of date order compares each side's
  // adjustment against the OTHER one and reports two divergences where there
  // are none — measured, on stacked-31: painted[0]=$3,893.57 vs dos[0]=4684.68
  // AND painted[1]=4684.68 vs dos[1]=3893.572138, i.e. an exact swap. That is
  // also the shape NF-4 (echo pairing by sort accident) describes, so an
  // index-paired comparison here could not tell a harness artefact from the
  // open defect.
  const dosAdj = (d.adjrows || []).filter(r => r.rateStatus !== 'empty' || r.amtStatus !== 'empty');
  const key = t => `${t.y}-${t.m}-${t.d}`;
  const uiKey = s2 => { const m2 = String(s2).match(/^(\d+)\/(\d+)\/(\d+)$/); return m2 ? `${+m2[3]}-${+m2[1]}-${+m2[2]}` : null; };
  if (dosAdj.length && (ui.paintedAdj || []).length) {
    const byDate = new Map();
    dosAdj.forEach(r => byDate.set(key(r.date), r));
    let matched = 0;
    for (const pr of ui.paintedAdj) {
      const k = uiKey(pr.date);
      const dr = k ? byDate.get(k) : null;
      if (!dr) continue;
      matched++;
      if (pr.amount !== '') {
        const painted = parseFloat(String(pr.amount).replace(/[$,]/g, ''));
        add(`adjAmount@${pr.date}`, near(painted, dr.amount, TOLERANCES.adjAmount.value),
          `painted="${pr.amount}" -> ${painted}  dos=${dr.amount}`, TOLERANCES.adjAmount.value);
      }
      if (pr.rate !== '') {
        // 🚨 TRAP 2, THE OTHER DIRECTION — AND IT IS EASY TO MISS.
        // The kicker is applied on the way IN (tokens.js). It must also be
        // applied on the way OUT: the page paints the adjustment rate in TYPED
        // space (handlers.go:1449 amzUnkickerRate — that unkick IS the NF-1c
        // fix), while the oracle's adjdump reports DOS's INTERNAL rate. On the
        // 365/360 basis the two differ by exactly 365/360 and a naive
        // comparison books every adjustment-carrying 365/360 screen as a
        // divergence. Measured: 13 of 13 such failures satisfied
        // painted × 365/360 == dos EXACTLY — which is not a defect, it is
        // confirmation that NF-1c's unkick is correct and complete.
        const paintedTyped = parseFloat(String(pr.rate).replace(/[%,]/g, '')) / 100;
        const paintedInternal = c.settings.basis === '365/360' ? paintedTyped * KICKER : paintedTyped;
        add(`adjRate@${pr.date}`, near(paintedInternal, dr.rate, TOLERANCES.adjRate.value),
          `painted="${pr.rate}" -> typed ${paintedTyped} -> internal ${paintedInternal}  ` +
          `dos=${dr.rate} (basis ${c.settings.basis})`, TOLERANCES.adjRate.value);
      }
    }
    // A join that finds nothing is not a clean result — it is a broken join.
    // Prove the key normalises before believing any adjustment verdict.
    add('adj-join-nonvacuous', matched > 0,
      `${matched} of ${ui.paintedAdj.length} painted adjustment row(s) matched a ` +
      `DOS adjrow by date; DOS emitted ${dosAdj.length}. A zero here means the ` +
      `date key does not normalise and every adjustment check above is vacuous.`);
  }

  if (ui.lastBalance != null && (c.adjustments.length || c.balloons.length || c.prepays.length)) {
    add('retires-to-zero', Math.abs(ui.lastBalance) < TOLERANCES.retiresToZero.value,
      `painted final balance=${ui.lastBalance}`, TOLERANCES.retiresToZero.value);
  }

  return checks;
}

// ---------------------------------------------------------------------------
// 🚨 THE TELL
// ---------------------------------------------------------------------------
// payment, total-interest and total-paid diverging on EXACTLY the same set is
// the signature of two sides computing DIFFERENT SCREENS — a MAPPING BUG in
// this harness — not of an arithmetic defect in the port. It caught both of
// 45b's mapping bugs. An arithmetic defect moves the money totals while
// leaving the level payment alone, or moves one total and not the other; only
// a different screen moves all three together, case for case.
//
// This function makes the harness SAY SO OUT LOUD rather than leaving it to a
// human to notice the coincidence.
function theTell(results) {
  const setOf = k => new Set(
    results.filter(r => (r.checks || []).some(c => c.k === k && !c.ok)).map(r => r.id));
  const pay = setOf('payment'), int = setOf('totalInterest'), paid = setOf('totalPaid');
  const eq = (a, b) => a.size === b.size && [...a].every(x => b.has(x));
  const identical = eq(pay, int) && eq(int, paid);
  const scored = results.filter(r => (r.checks || []).some(c => c.k === 'totalPaid')).length;

  const verdict = {
    paymentSet: [...pay], interestSet: [...int], paidSet: [...paid],
    identical, scored, n: pay.size,
  };

  if (!identical || pay.size === 0) {
    verdict.level = 'ok';
    verdict.message = pay.size === 0
      ? 'THE TELL: no case diverges on payment — no mapping-bug signature.'
      : `THE TELL: the three sets are NOT identical (payment ${pay.size}, ` +
        `interest ${int.size}, paid ${paid.size}). That is the shape of an ` +
        `arithmetic difference, not of two sides computing different screens.`;
    return verdict;
  }

  // Identical sets. How loud to be depends on how much of the population it is.
  const frac = scored ? pay.size / scored : 0;
  verdict.fraction = frac;
  verdict.level = frac >= 0.30 ? 'mapping-bug' : 'suspect';
  verdict.message =
    `🚨 THE TELL FIRED: payment, total-interest and total-paid diverge on ` +
    `EXACTLY the same ${pay.size} of ${scored} scored cases ` +
    `(${(frac * 100).toFixed(1)}%). That is the signature of TWO SIDES ` +
    `COMPUTING DIFFERENT SCREENS — a mapping bug in THIS HARNESS — not of an ` +
    `arithmetic defect in the port. ` +
    (verdict.level === 'mapping-bug'
      ? `At ${(frac * 100).toFixed(1)}% this is being reported as a HARNESS ` +
        `FAULT and the divergence rate below is NOT a defect rate. Rule 12: ` +
        `the harness is a suspect before the engine is. Check the 365/360 ` +
        `kicker on the loan rate AND the adjustment rates (tokens.js), and ` +
        `check the date-token order (D.M.Y vs MM/DD/YYYY).`
      : `The fraction is small enough that a shared root cause is possible, ` +
        `but ATTRIBUTE IT BEFORE PUBLISHING IT.`);
  return verdict;
}

// The identity controls are the other half of the instrument's self-check.
// A plain-tier divergence is a mapping bug by construction: the scalar path is
// the one thing both sides certainly agree on.
function controlVerdict(results) {
  const plain = results.filter(r => r.tier === 'plain');
  const bad = plain.filter(r => !r.pass && !r.na);
  return {
    n: plain.length, failed: bad.length, ids: bad.map(r => r.id),
    ok: bad.length === 0,
    message: bad.length === 0
      ? `identity controls: ${plain.length} of ${plain.length} clean.`
      : `🚨 ${bad.length} of ${plain.length} PLAIN IDENTITY CONTROLS DIVERGED. ` +
        `The scalar mapping is wrong. NOTHING ELSE IN THIS RUN MEANS ANYTHING ` +
        `until that is fixed.`,
  };
}

module.exports = { TOLERANCES, compareCase, theTell, controlVerdict, near };
