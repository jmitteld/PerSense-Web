// uidiff/gen.js — the UI-EXPRESSIBLE screen generator.
//
// Three tiers, and the middle one is the point of this file.
//
//   plain   — identity controls. Every advanced option off, every setting at
//             its UI default. 45b ran 10 of these and all 10 were clean while
//             the grid mapping was wrong, because a plain screen CARRIES NO
//             GRID ROWS AT ALL and is STRUCTURALLY UNABLE to express a
//             grid-mapping error. That is R49 one level down.
//   single  — 🚨 THE NEW TIER. Exactly ONE option away from plain. This is the
//             tier that CAN express a grid-mapping error while still being
//             attributable: if `single/adj-rate` diverges and nothing else
//             does, the adjustment-rate mapping is the suspect and no ablation
//             is needed. Plain controls validate the SCALAR mapping only.
//   stacked — the adversarial population. k options at once.
//
// WHAT THIS GENERATOR CANNOT PRODUCE (standing rule 8 — say it, do not let a
// reader infer coverage the population does not have):
//   * perYr ∈ {24, 26, 52}. Semimonthly/biweekly/weekly periods are day-based,
//     so a generated grid date cannot be guaranteed on-grid without
//     reimplementing DOS's own date walk — and an off-grid date is the thing
//     that made 2 of 45b's 3 findings NOT ADJUDICABLE. perYr is drawn from
//     {1, 2, 4, 6, 12} only.
//   * Day-of-month > 28. Month-end snapping is a live open question (§81 on
//     the PV screen) and this instrument is not the place to adjudicate it.
//   * Off-grid grid dates. Every balloon/prepay/adjustment/moratorium date is
//     an exact payment date by construction. Off-grid behaviour is UNTESTED
//     here.
//   * Horizons beyond 2099 — the in-scope predicate. Loan dates are drawn
//     2020-2030 and terms capped so the last payment lands well inside scope.
//   * Solved (blank) principal/rate/term screens. Every generated screen is
//     forward: amount, rate and term are all supplied. Backward solves are
//     covered by the engine differentials, not by this instrument.

'use strict';

const { ymd } = require('./tokens');

// Deterministic PRNG — mulberry32. The harness must reproduce exactly across
// rounds (3a.10, cross-round reproducibility) so nothing here may use
// Math.random().
function rng(seed) {
  let a = seed >>> 0;
  return function () {
    a |= 0; a = (a + 0x6D2B79F5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const PERYR = [1, 2, 4, 6, 12];
const MONTHS_PER_PERIOD = { 1: 12, 2: 6, 4: 3, 6: 2, 12: 1 };

function addMonths(t, n) {
  const zero = (t.y * 12 + (t.m - 1)) + n;
  const y = Math.floor(zero / 12);
  const m = (zero % 12) + 1;
  return ymd(y, m, t.d); // day ≤ 28 by construction, so no clamping is needed
}

const DEFAULT_SETTINGS = {
  perYr: 12, basis: '360', prepaid: 'yes', timing: 'arrears',
  balloonIncl: 'no', exact: 'no', rule78: 'no', interestRule: 'actuarial',
};

function pick(r, arr) { return arr[Math.floor(r() * arr.length)]; }
function round2(v) { return Math.round(v * 100) / 100; }

// The option space. Each entry mutates a case in place and names itself.
// `single` applies exactly one; `stacked` applies several.
const OPTIONS = [
  { key: 'basis-365', apply: (c) => { c.settings.basis = '365'; } },
  { key: 'basis-365-360', apply: (c) => { c.settings.basis = '365/360'; } },
  { key: 'prepaid-no', apply: (c) => { c.settings.prepaid = 'no'; } },
  { key: 'timing-advance', apply: (c) => { c.settings.timing = 'advance'; } },
  { key: 'exact', apply: (c) => { c.settings.exact = 'yes'; } },
  { key: 'rule78', apply: (c) => { c.settings.rule78 = 'yes'; } },
  {
    // 🚨 R49, one more level down: plus_regular has NO EFFECT on a screen with
    // no extras, so a `balloon-incl` single-tier case on an otherwise-plain
    // screen is VACUOUS — it passed cleanly while the mapping was inverted.
    // The option therefore carries a balloon of its own so the tier can
    // actually express the difference it exists to isolate.
    key: 'balloon-incl',
    apply: (c, r) => {
      c.settings.balloonIncl = 'yes';
      if (!c.balloons.length) {  // NB: only helps when this option runs second
        const k = Math.max(2, Math.floor(c.nPeriods * (0.5 + r() * 0.4)));
        c.balloons.push({ date: c.periodDate(k), amount: round2(c.amount * (0.05 + r() * 0.25)) });
      }
    },
  },
  { key: 'usa-rule', apply: (c) => { c.settings.interestRule = 'usa'; } },
  { key: 'points', apply: (c, r) => { c.points = round2(0.005 + r() * 0.04); } },
  {
    key: 'adj-rate',
    apply: (c, r) => {
      const k = 1 + Math.floor(r() * Math.max(1, c.nPeriods - 2));
      c.adjustments.push({ date: c.periodDate(k), rate: round2(c.rate + (r() - 0.5) * 0.06), amount: '' });
    },
  },
  {
    key: 'adj-amount',
    apply: (c, r) => {
      const k = 1 + Math.floor(r() * Math.max(1, c.nPeriods - 2));
      c.adjustments.push({ date: c.periodDate(k), rate: '', amount: round2(c.basePayment * (0.8 + r() * 0.6)) });
    },
  },
  {
    key: 'balloon',
    apply: (c, r) => {
      const k = Math.max(2, Math.floor(c.nPeriods * (0.5 + r() * 0.4)));
      c.balloons.push({ date: c.periodDate(k), amount: round2(c.amount * (0.05 + r() * 0.25)) });
    },
  },
  {
    key: 'prepay',
    apply: (c, r) => {
      const k = 1 + Math.floor(r() * Math.max(1, Math.floor(c.nPeriods / 2)));
      const n = 1 + Math.floor(r() * 6);
      c.prepays.push({
        startDate: c.periodDate(k), nPmts: n, perYr: c.settings.perYr,
        amount: round2(c.basePayment * (0.2 + r() * 0.8)),
      });
    },
  },
  { key: 'moratorium', apply: (c, r) => { c.moratorium = c.periodDate(1 + Math.floor(r() * Math.max(1, Math.floor(c.nPeriods / 3)))); } },
  { key: 'target', apply: (c, r) => { c.target = round2(c.amount / c.nPeriods * (0.3 + r() * 0.5)); } },
  { key: 'peryr', apply: (c, r) => { c.settings.perYr = pick(r, [1, 2, 4, 6]); } },
];

const OPTION_KEYS = OPTIONS.map(o => o.key);

function baseCase(r, id, tier) {
  const amount = Math.round((5000 + r() * 400000) / 100) * 100;
  const rate = round2(0.03 + r() * 0.13);
  const perYr = 12;
  const years = 1 + Math.floor(r() * 25);
  const nPeriods = years * perYr;
  const loanY = 2020 + Math.floor(r() * 11);
  const loanM = 1 + Math.floor(r() * 12);
  const loanD = 1 + Math.floor(r() * 28);
  const loanDate = ymd(loanY, loanM, loanD);

  const c = {
    id, tier, options: [],
    amount, rate, nPeriods, points: 0,
    loanDate,
    firstDate: null,
    lastDate: null, payment: null, target: null, skipMonths: null, moratorium: null,
    balloons: [], prepays: [], adjustments: [],
    settings: Object.assign({}, DEFAULT_SETTINGS, { perYr }),
  };
  // A crude level payment, used only to size generated grid amounts so they are
  // plausible. It is NEVER compared against anything.
  const i = c.rate / perYr;
  c.basePayment = round2(i > 0 ? amount * i / (1 - Math.pow(1 + i, -nPeriods)) : amount / nPeriods);
  // periodDate(k) = the k-th payment date, exactly on grid.
  c.periodDate = function (k) {
    const mpp = MONTHS_PER_PERIOD[this.settings.perYr];
    return addMonths(this.firstDate, (k - 1) * mpp);
  };
  return c;
}

function finalize(c) {
  // firstDate is one period after the loan date, on grid.
  const mpp = MONTHS_PER_PERIOD[c.settings.perYr];
  c.firstDate = addMonths(c.loanDate, mpp);
  // 🚨 RECOMPUTE basePayment FOR THE CURRENT perYr. It was computed once in
  // baseCase() at perYr=12 and never refreshed when the `peryr` option changed
  // the period, so on an ANNUAL screen the generated adjustment amount was a
  // MONTHLY payment — up to 10.9x too small (measured: stacked-18, stale
  // 1248.39 vs correct 13561.32). An adjustment that sets the new installment
  // to a tenth of what the loan needs cannot amortize it at a positive rate,
  // DOS's Iterate declines to converge, and the harness books a refusal-parity
  // divergence it manufactured itself. 69 of the 90 `peryr` stacked cases were
  // in that state, and 7 of the round's 9 attributed findings landed in the
  // single cell `adj-amount ∧ peryr`. R46's adversarial audit caught it.
  const per = c.settings.perYr;
  const i2 = c.rate / per;
  c.basePayment = round2(i2 > 0
    ? c.amount * i2 / (1 - Math.pow(1 + i2, -c.nPeriods))
    : c.amount / c.nPeriods);
  // Re-scale the term so that changing perYr does not blow past the scope
  // ceiling: cap the horizon at 35 years past the loan date.
  const maxPeriods = Math.floor(35 * c.settings.perYr);
  if (c.nPeriods > maxPeriods) c.nPeriods = maxPeriods;
  if (c.nPeriods < 2) c.nPeriods = 2;
  // recompute once more now that nPeriods is final
  c.basePayment = round2(i2 > 0
    ? c.amount * i2 / (1 - Math.pow(1 + i2, -c.nPeriods))
    : c.amount / c.nPeriods);
  return c;
}

// Grid dates depend on firstDate and perYr, so options that touch either must
// be applied BEFORE the grid options. `peryr` is therefore applied first and
// firstDate recomputed, and only then are grid rows generated.
function applyOptions(c, keys, r) {
  const ordered = keys.slice().sort((a, b) => (a === 'peryr' ? -1 : b === 'peryr' ? 1 : 0));
  for (const k of ordered) {
    const o = OPTIONS.find(x => x.key === k);
    if (!o) throw new Error(`unknown option ${k}`);
    if (k === 'peryr') { o.apply(c, r); finalize(c); }
    else o.apply(c, r);
    c.options.push(k);
  }
  return c;
}

// A generated case must be UI-EXPRESSIBLE: the grids have a fixed number of
// rows in the shipped page. Refuse to emit a case the page cannot hold.
const MAX_GRID_ROWS = 3;

function generate({ seed = 46046, plain = 10, singlePer = 2, stacked = 200 } = {}) {
  const r = rng(seed);
  const cases = [];
  let n = 0;

  for (let i = 0; i < plain; i++) {
    cases.push(finalize(baseCase(r, `plain-${i}`, 'plain')));
    n++;
  }

  // One-option-at-a-time: `singlePer` independent screens per option.
  for (const key of OPTION_KEYS) {
    for (let i = 0; i < singlePer; i++) {
      const c = finalize(baseCase(r, `single-${key}-${i}`, 'single'));
      applyOptions(c, [key], r);
      cases.push(c);
      n++;
    }
  }

  for (let i = 0; i < stacked; i++) {
    const c = finalize(baseCase(r, `stacked-${i}`, 'stacked'));
    // Draw k distinct options. Median 7, matching 45b's population.
    const k = 4 + Math.floor(r() * 7);
    const pool = OPTION_KEYS.slice();
    const chosen = [];
    for (let j = 0; j < k && pool.length; j++) {
      chosen.push(pool.splice(Math.floor(r() * pool.length), 1)[0]);
    }
    // basis is one select — never stack two basis options.
    const basisOpts = chosen.filter(x => x.startsWith('basis-'));
    if (basisOpts.length > 1) {
      for (const drop of basisOpts.slice(1)) chosen.splice(chosen.indexOf(drop), 1);
    }
    applyOptions(c, chosen, r);
    if (c.balloons.length > MAX_GRID_ROWS) c.balloons.length = MAX_GRID_ROWS;
    if (c.prepays.length > MAX_GRID_ROWS) c.prepays.length = MAX_GRID_ROWS;
    if (c.adjustments.length > MAX_GRID_ROWS) c.adjustments.length = MAX_GRID_ROWS;
    cases.push(c);
    n++;
  }

  return cases;
}

module.exports = { generate, OPTION_KEYS, DEFAULT_SETTINGS, rng, addMonths, MONTHS_PER_PERIOD };
