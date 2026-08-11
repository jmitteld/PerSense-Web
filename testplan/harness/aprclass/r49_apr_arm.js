// r49_apr_arm.js — THE APR CLASS ARM (round 49).
//
// WHY THIS FILE EXISTS
// -------------------
// `docs/discrepancies.md` §89 §4 records a `USA ∧ prepay ∧ adjustment`
// divergence with six numbers and one reproducing command line. Round 47
// found the command reproduces NONE of the six values, and no results
// artefact was ever committed — so the record was UNBACKED (R60). Round 49's
// work item is to RE-DERIVE the divergence FROM THE INPUT and file it WITH A
// COMMITTED RESULTS ARTEFACT. This file is that artefact's generator.
//
// It also runs §88 §5's still-open measurement: WIDEN THE SEARCH FOR AN
// IN-SCOPE TOTALS-IDENTICAL APR CASE. The only clean isolate the project owns
// (§88 §5) has horizon 2133 — OUT OF SCOPE — so every published in-scope APR
// rate rests on cases that are confounded with a totals divergence.
//
// WHAT IT IS NOT
// --------------
// It is NOT a replacement for `dos_fuzzer5_test.go`. Its population is
// deliberately NARROW and STATED (see POPULATION below). Its numbers must
// never be quoted as an in-scope rate for the product; they are a rate for
// THIS generator (R31, CAUTION 2, CAUTION 8).
//
// THE STANDING TRAPS THIS FILE OBEYS
// ----------------------------------
// * TRAP 1 — `apr` is an OUTPUT MODE that Halt(0)s and SUPPRESSES `adjdump`/
//   `bdump` and the totals line. TWO invocations per case are REQUIRED. This
//   file never builds a mixed line; it delegates to uidiff/tokens.js, whose
//   assertModePurity() refuses one.
// * TRAP 2 — on the 365/360 basis the API kicks the TYPED rate by 365/360 for
//   the loan rate AND for every adjustment rate. tokens.js owns that mapping;
//   this file hands tokens.js TYPED rates and hands the API the same TYPED
//   rates, so the kicker is applied exactly once, on the oracle side.
// * `pts=` IS ALWAYS EMITTED, INCLUDING AT ZERO — otherwise DOS never runs the
//   APR solver and returns the "declined to store" shape `apr 0.000000
//   status 0`, which a naive parser reads as a real zero APR.
// * R61 — A TIMEOUT IS NOT A REFUSAL. Every oracle invocation records its exit
//   CODE and its SIGNAL separately, and a case that produced no output is
//   QUARANTINED, never folded into a rate about what the engine said.
// * R64 — NAME THE CHANNEL. stdout and stderr are captured SEPARATELY and the
//   deciding channel is named on every classification. The APR verdict is
//   decided on STDOUT.
// * R57 — the seed alone does not identify a population. Every run prints a
//   full reproduce line and the results file records every knob.
//
// USAGE
//   node testplan/harness/aprclass/r49_apr_arm.js --mode=ablation
//   node testplan/harness/aprclass/r49_apr_arm.js --mode=sweep --n=400 --seed=49049
'use strict';

const { execFileSync, execFile } = require('child_process');
const fs = require('fs');
const path = require('path');
const http = require('http');

const tokens = require('../uidiff/tokens.js');
const { ymd } = tokens;

const ORACLE = process.env.ORACLE || '/tmp/oraclebuild/amort_oracle';
const API = process.env.API || 'http://localhost:8080/api/amortization/calc';
const ORACLE_TIMEOUT_MS = 20000;

// --------------------------------------------------------------------------
// Tolerances — NAMED, and pinned in Go by TestToleranceValuesArePinned
// (internal/finance/amortization/zzr48_tolerances_test.go). Quote the constant,
// never a bare number (rule 9).
// --------------------------------------------------------------------------
const tolAPR = 2e-6;                                   // Signal 6's tolerance
const tolTotalsFloor = 1.00;
const tolTotalsRel = 5e-4;
const tolTotals = v => Math.max(tolTotalsFloor, tolTotalsRel * Math.abs(v));

// The in-scope predicate's ceiling. Decision 3a.5 / item 10: DOS's real
// ceiling is 26 Aug 2091. The standing SCOPE KEY is `reached` = max(last
// schedule row, balloons) — NOT `lastdate` and NOT `horizon` (which also folds
// in LastDate). START_HERE §2, decision 3a.11, executed round 48.
const SCOPE_CEILING_Y = 2091;

// --------------------------------------------------------------------------
// Oracle invocation. R61 + R64 live here.
// --------------------------------------------------------------------------
function runOracle(args) {
  const rec = {
    argv: args.slice(),
    stdout: '', stderr: '',
    exitCode: null, signal: null,
    timedOut: false, spawnError: null,
  };
  try {
    // stdio: stderr is a PIPE, not 'ignore'. uidiff/oracle.js discarded it and
    // that is exactly why a hang and a refusal became indistinguishable (R61)
    // and why a classification read a channel the instrument threw away (R64).
    const out = execFileSync(ORACLE, args, {
      timeout: ORACLE_TIMEOUT_MS,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
      maxBuffer: 64 * 1024 * 1024,
    });
    rec.stdout = out;
    rec.exitCode = 0;
  } catch (e) {
    rec.stdout = (e.stdout || '').toString();
    rec.stderr = (e.stderr || '').toString();
    // execFileSync surfaces a timeout kill as e.signal (SIGTERM) with
    // e.status === null. THAT IS THE DISTINCTION R61 EXISTS FOR: it is not an
    // exit status at all, and a case that reached it produced NO ANSWER.
    rec.exitCode = (typeof e.status === 'number') ? e.status : null;
    rec.signal = e.signal || null;
    rec.timedOut = (e.killed === true) || (e.signal === 'SIGTERM' && e.status === null);
    if (e.code && typeof e.code === 'string' && !e.signal) rec.spawnError = e.code;
  }
  return rec;
}

// DOS's APR line, read from STDOUT (R64: name the channel).
//   amort_oracle.pas:1296-1305 — `apr <value> status <n>`
// status 0 is the "declined to store" shape. It is NOT an APR of zero.
function parseAPR(rec) {
  if (rec.timedOut) return { ok: false, why: 'TIMEOUT', channel: 'stdout' };
  const m = /^apr\s+(-?[\d.]+)\s+status\s+(-?\d+)/m.exec(rec.stdout);
  if (!m) return { ok: false, why: 'NO_APR_LINE', channel: 'stdout' };
  const status = parseInt(m[2], 10);
  if (status === 0) return { ok: false, why: 'STATUS_0_DECLINED', channel: 'stdout' };
  return { ok: true, apr: parseFloat(m[1]), status, channel: 'stdout' };
}

function parseTotals(rec) {
  if (rec.timedOut) return { ok: false, why: 'TIMEOUT', channel: 'stdout' };
  const m = /^payment\s+(-?[\d.]+)\s+interest\s+(-?[\d.]+)\s+paid\s+(-?[\d.]+)/m.exec(rec.stdout);
  if (!m) return { ok: false, why: 'NO_TOTALS_LINE', channel: 'stdout' };
  const lastdate = /^lastdate\s+(\d+)\/(\d+)\/(\d+)/m.exec(rec.stdout);
  const adjrows = [];
  const re = /^adjrow\s+(\d+)\s+date\s+(\d+)\/(\d+)\/(\d+)\s+rate\s+(-?[\d.]+)\s+ratestatus\s+(\d+)\s+amount\s+(-?[\d.]+)\s+amtstatus\s+(\d+)\s+amtok\s+(\w+)/gm;
  let a;
  while ((a = re.exec(rec.stdout)) !== null) {
    adjrows.push({
      n: +a[1], date: `${a[4]}-${String(+a[2]).padStart(2, '0')}-${String(+a[3]).padStart(2, '0')}`,
      rate: parseFloat(a[5]), rateStatus: +a[6],
      amount: parseFloat(a[7]), amtStatus: +a[8], amtOK: a[9] === 'TRUE',
    });
  }
  return {
    ok: true, channel: 'stdout',
    payment: parseFloat(m[1]), interest: parseFloat(m[2]), paid: parseFloat(m[3]),
    lastDateY: lastdate ? +lastdate[3] : null,
    adjrows,
  };
}

// --------------------------------------------------------------------------
// The port, AT THE CONSUMER — the REST handler the shipped page calls.
// `goamort` REJECTS bdate=/adjdmy=/predmy=/mordmy= entirely, so a date-token
// case CANNOT be compared through the CLI (START_HERE §5). This is the API.
// --------------------------------------------------------------------------
const iso = t => `${t.y}-${String(t.m).padStart(2, '0')}-${String(t.d).padStart(2, '0')}`;

function apiRequest(c) {
  const s = c.settings;
  const req = {
    amount: c.amount,
    rate: c.rate,                       // TYPED rate — the API applies the kicker
    nPeriods: c.nPeriods,
    perYr: s.perYr,
    loanDate: iso(c.loanDate),
    basis: s.basis,
    points: c.points || 0,
  };
  if (c.firstDate) req.firstDate = iso(c.firstDate);
  if (s.timing === 'advance') req.inAdvance = true;
  if (s.interestRule === 'usa') req.usaRule = true;
  if (s.rule78 === 'yes') req.rule78 = true;
  if (s.exact === 'yes') req.exact = true;
  req.firstIntPrepaid = (s.prepaid === 'yes');
  if (s.balloonIncl === 'yes') req.balloonIncludesRegular = true;
  if (c.payment) req.payment = c.payment;
  if (c.target != null) req.targetAmt = c.target;
  if (c.moratorium) req.moratorium = iso(c.moratorium);
  if (c.balloons && c.balloons.length) {
    req.balloons = c.balloons.map(b => ({ date: iso(b.date), amount: b.amount }));
  }
  if (c.prepays && c.prepays.length) {
    req.prepayments = c.prepays.map(p => ({
      startDate: iso(p.startDate), nPmts: p.nPmts, perYr: p.perYr, amount: p.amount,
    }));
  }
  if (c.adjustments && c.adjustments.length) {
    req.adjustments = c.adjustments.map(a => {
      const o = { date: iso(a.date) };
      if (a.rate !== '' && a.rate != null) o.rate = a.rate;      // TYPED
      if (a.amount !== '' && a.amount != null) o.amount = a.amount;
      return o;
    });
  }
  return req;
}

function postAPI(req) {
  return new Promise((resolve) => {
    const body = Buffer.from(JSON.stringify(req));
    const u = new URL(API);
    const r = http.request({
      hostname: u.hostname, port: u.port, path: u.pathname, method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Content-Length': body.length },
      timeout: ORACLE_TIMEOUT_MS,
    }, res => {
      let d = '';
      res.on('data', x => d += x);
      res.on('end', () => {
        try { resolve({ ok: true, status: res.statusCode, body: JSON.parse(d) }); }
        catch (e) { resolve({ ok: false, why: 'BAD_JSON', status: res.statusCode, raw: d.slice(0, 400) }); }
      });
    });
    r.on('timeout', () => { r.destroy(); resolve({ ok: false, why: 'TIMEOUT' }); });
    r.on('error', e => resolve({ ok: false, why: 'HTTP_ERROR', detail: String(e.message) }));
    r.write(body); r.end();
  });
}

// --------------------------------------------------------------------------
// One case, both sides, both modes.
// --------------------------------------------------------------------------
async function measure(c, id) {
  const argsData = tokens.buildArgs(c, 'data');
  const argsAPR = tokens.buildArgs(c, 'apr');
  const recData = runOracle(argsData);
  const recAPR = runOracle(argsAPR);
  const dosTotals = parseTotals(recData);
  const dosAPR = parseAPR(recAPR);
  const req = apiRequest(c);
  const res = await postAPI(req);

  const out = {
    id,
    reproduce: {
      oracleData: [ORACLE].concat(argsData).join(' '),
      oracleAPR: [ORACLE].concat(argsAPR).join(' '),
      apiRequest: req,
    },
    options: c.optionLabels || [],
    oracleProcess: {
      data: { exitCode: recData.exitCode, signal: recData.signal, timedOut: recData.timedOut, stderrBytes: recData.stderr.length },
      apr: { exitCode: recAPR.exitCode, signal: recAPR.signal, timedOut: recAPR.timedOut, stderrBytes: recAPR.stderr.length },
    },
    dos: { totals: dosTotals, apr: dosAPR },
    port: res.ok ? {
      ok: true,
      payment: res.body.payment, interest: res.body.totalInterest, paid: res.body.totalPaid,
      apr: res.body.apr, aprConverged: res.body.aprConverged,
      engineUsed: res.body.engineUsed || null,
      lastDate: res.body.lastDate || null,
      error: res.body.error || null,
      adjustments: res.body.adjustments || null,
      warningCount: (res.body.warnings || []).length,
    } : { ok: false, why: res.why, status: res.status || null },
  };

  // ---- classification ----------------------------------------------------
  // QUARANTINE FIRST (R61). A case whose oracle never returned belongs in no
  // rate about what the engine said.
  if (recData.timedOut || recAPR.timedOut || recData.signal || recAPR.signal) {
    out.verdict = 'QUARANTINED_ORACLE_NO_TERMINATION';
    out.decidingChannel = 'n/a — no output';
    return out;
  }
  // 🚨 R56 APPLIED TO THIS FILE. The round's own adversarial audit found that
  // this arm quarantined ONLY on a timeout, so a NONZERO EXIT or a SPAWN
  // FAILURE (ENOENT — a wrong PERSENSE_ORACLE) fell through to DOS_DECLINED:
  // a statement about what the engine SAID, for a run where it may never have
  // started. That is the exact defect being fixed in uidiff/oracle.js, shipped
  // in the file fixing it. It was INERT in this run (all invocations exited 0)
  // — which is R51, not a defence.
  if (recData.spawnError || recAPR.spawnError ||
      (recData.exitCode !== 0 && recData.exitCode !== null) ||
      (recAPR.exitCode !== 0 && recAPR.exitCode !== null)) {
    out.verdict = 'QUARANTINED_PROCESS_FAILED';
    out.decidingChannel = 'n/a — process outcome, not engine output';
    return out;
  }
  if (!out.port.ok) { out.verdict = 'PORT_TRANSPORT_FAIL'; return out; }

  // In scope, on the STANDING KEY `reached` = max(last schedule row, balloons).
  // The oracle's `lastdate` line is the last schedule row; balloon dates are
  // in the case. `lastdmy=` is never emitted by this generator, so `reached`
  // and `horizon` coincide here — stated, not assumed.
  const balloonYs = (c.balloons || []).map(b => b.date.y);
  const reachedY = Math.max(dosTotals.ok && dosTotals.lastDateY ? dosTotals.lastDateY : 0, ...balloonYs, 0);
  out.scopeKey = 'reached';
  out.reachedYear = reachedY;
  // 🚨 THREE CATEGORIES, NOT TWO. reachedY comes from DOS's `lastdate` line, so
  // it is 0 when DOS declined — and reporting that as "out of scope" conflates
  // BEYOND THE CEILING with UNMEASURABLE. The audit found all 111 "out of
  // scope" rows in the r49 sweep were reachedYear=0 declines and ZERO were
  // above 2091, so on this generator the scope key removed nothing: it is a
  // LABEL, not a filter (R49 — a control that cannot express the difference).
  out.scopeStatus = reachedY === 0 ? 'UNMEASURABLE'
    : (reachedY <= SCOPE_CEILING_Y ? 'IN_SCOPE' : 'BEYOND_CEILING');
  out.inScope = out.scopeStatus === 'IN_SCOPE';

  if (!dosTotals.ok || !dosAPR.ok) {
    out.verdict = 'DOS_DECLINED';
    out.declineWhy = { totals: dosTotals.ok ? null : dosTotals.why, apr: dosAPR.ok ? null : dosAPR.why };
    out.decidingChannel = 'stdout';
    return out;
  }

  const dTot = {
    payment: Math.abs(dosTotals.payment - out.port.payment),
    interest: Math.abs(dosTotals.interest - out.port.interest),
    paid: Math.abs(dosTotals.paid - out.port.paid),
  };
  out.totals = {
    dos: { payment: dosTotals.payment, interest: dosTotals.interest, paid: dosTotals.paid },
    port: { payment: out.port.payment, interest: out.port.interest, paid: out.port.paid },
    delta: dTot,
    // "IDENTICAL" means identical AT THE PRECISION EACH SIDE PRINTS — DOS
    // prints interest/paid to 2dp and payment to 4dp, and the API returns a
    // payment already rounded to 2dp, so the strongest available statement is
    // HALF-CENT agreement on all three. It is labelled as such and MUST NOT be
    // quoted as bit equality (CAUTION 1).
    // 🚨 THE BOUNDARY MUST NOT SIT INSIDE THE NOISE FLOOR. DOS prints
    // interest/paid to 2dp, so its own quantisation is EXACTLY +/-0.005 and a
    // strict `< 0.005` labels a pure rounding artefact a divergence. The audit
    // found two such rows (deltas 0.00515 and 0.00508 on totals of $830k and
    // $928k) carrying the verdict name APR_AGREED_TOTALS_DIVERGE. Half a
    // quantum plus one ulp of slack is the honest line, and `identical` means
    // AGREED AT DOS'S PRINTED PRECISION — never bit equality (CAUTION 1).
    identical: dTot.interest <= 0.005 + 1e-9 && dTot.paid <= 0.005 + 1e-9 &&
               dTot.payment <= 0.005 + 1e-9,
    withinTolTotals: dTot.interest <= tolTotals(dosTotals.interest) &&
                     dTot.paid <= tolTotals(dosTotals.paid),
  };
  const dAPR = Math.abs(dosAPR.apr - out.port.apr);
  out.aprCompare = {
    dos: dosAPR.apr, dosStatus: dosAPR.status,
    port: out.port.apr, portConverged: out.port.aprConverged,
    delta: dAPR, tolerance: 'tolAPR', toleranceValue: tolAPR,
    divergent: dAPR > tolAPR,
  };
  out.decidingChannel = 'stdout (DOS) vs application/json response body (port)';
  out.verdict = out.aprCompare.divergent
    ? (out.totals.identical ? 'APR_DIVERGENT_TOTALS_IDENTICAL' : 'APR_DIVERGENT_TOTALS_ALSO_DIVERGE')
    : (out.totals.identical ? 'AGREED' : 'APR_AGREED_TOTALS_DIVERGE');
  return out;
}

// --------------------------------------------------------------------------
// Case construction
// --------------------------------------------------------------------------
function baseSettings(over) {
  return Object.assign({
    perYr: 1, basis: '365/360', prepaid: 'no', timing: 'arrears',
    exact: 'no', rule78: 'no', balloonIncl: 'no', interestRule: 'actuarial',
  }, over || {});
}

// §89 §4's case, RE-DERIVED FROM THE INPUT. The rates below are the TYPED
// rates; §89 §4's recorded command line carries them ALREADY KICKED by
// 365/360, which is what tokens.js will re-apply.
const K = tokens.KICKER;
const SEC89_4 = {
  amount: 217463.13,
  rate: 0.1066252194 / K,
  nPeriods: 15,
  settings: baseSettings({ perYr: 1, basis: '365/360', interestRule: 'usa' }),
  loanDate: ymd(2025, 10, 1),
  firstDate: ymd(2026, 10, 1),
  points: 0,
  prepays: [{ startDate: ymd(2029, 10, 1), nPmts: 7, perYr: 4, amount: 1835.20 }],
  adjustments: [
    { date: ymd(2034, 10, 1), rate: 0.0686234472 / K, amount: '' },
    { date: ymd(2030, 10, 1), rate: '', amount: 3723.03 },
  ],
};

// The ablation. THREE factors, and the two adjustments split so an
// adjustment-RATE case and an adjustment-AMOUNT case are separable — §85's
// withdrawn attribution was about the adjustment-rate solve specifically, and
// a 3-factor ablation could not have separated them.
const FACTORS = ['usa', 'prepay', 'adjRate', 'adjAmount'];

function ablationCase(subset) {
  const c = JSON.parse(JSON.stringify(SEC89_4));
  // JSON round-trip loses nothing here: every field is a plain number/object.
  if (!subset.includes('usa')) c.settings.interestRule = 'actuarial';
  if (!subset.includes('prepay')) delete c.prepays;
  const adjs = [];
  if (subset.includes('adjRate')) adjs.push(SEC89_4.adjustments[0]);
  if (subset.includes('adjAmount')) adjs.push(SEC89_4.adjustments[1]);
  if (adjs.length) c.adjustments = JSON.parse(JSON.stringify(adjs)); else delete c.adjustments;
  c.optionLabels = subset.slice();
  return c;
}

function subsets(arr) {
  const out = [];
  for (let m = 0; m < (1 << arr.length); m++) {
    out.push(arr.filter((_, i) => m & (1 << i)));
  }
  return out;
}

// --------------------------------------------------------------------------
// The sweep generator. STATED POPULATION — quote it with every number.
// --------------------------------------------------------------------------
function mulberry32(a) {
  return function () {
    a |= 0; a = a + 0x6D2B79F5 | 0;
    let t = Math.imul(a ^ a >>> 15, 1 | a);
    t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
    return ((t ^ t >>> 14) >>> 0) / 4294967296;
  };
}

const POPULATION = {
  what: 'APR-bearing screens: points ALWAYS > 0 so DOS runs the APR solver, ' +
        'loan dates 2020-2030 and terms bounded so the reached horizon stays ' +
        'well inside the 2091 ceiling BY CONSTRUCTION.',
  cannotProduce: [
    'lastdmy= (so the §88 §5 isolate\'s 2133 horizon is unreachable here BY DESIGN)',
    'off-grid adjustment/prepay/balloon dates (every date is on the payment grid)',
    'perYr in {24,26,52}',
    'backward solves (payment, rate and amount are always supplied)',
    'moratorium and skip-months',
    'day-of-month > 28',
  ],
};

function genCase(rnd, i) {
  const perYr = [1, 2, 4, 6, 12][Math.floor(rnd() * 5)];
  const basis = ['360', '365', '365/360'][Math.floor(rnd() * 3)];
  const usa = rnd() < 0.5;
  const inadv = rnd() < 0.35;
  const r78 = rnd() < 0.2;
  const exact = basis !== '360' && rnd() < 0.25;
  const prepaid = rnd() < 0.5 ? 'yes' : 'no';
  const y = 2020 + Math.floor(rnd() * 11);
  const m = 1 + Math.floor(rnd() * 12);
  const d = 1 + Math.floor(rnd() * 28);
  const years = 1 + Math.floor(rnd() * 25);
  const nPeriods = Math.max(2, years * perYr);
  const loanDate = ymd(y, m, d);
  const fy = y + Math.floor(12 / perYr / 12) ;
  const firstMonths = Math.round(12 / perYr);
  const fm = ((m - 1 + firstMonths) % 12) + 1;
  const fyy = y + Math.floor((m - 1 + firstMonths) / 12);
  const firstDate = ymd(fyy, fm, d);
  void fy;
  const c = {
    amount: Math.round((5000 + rnd() * 400000) * 100) / 100,
    rate: Math.round((0.02 + rnd() * 0.16) * 1e10) / 1e10,
    nPeriods,
    settings: baseSettings({
      perYr, basis,
      prepaid, timing: inadv ? 'advance' : 'arrears',
      exact: exact ? 'yes' : 'no', rule78: r78 ? 'yes' : 'no',
      interestRule: usa ? 'usa' : 'actuarial',
    }),
    loanDate, firstDate,
    // POINTS ARE ALWAYS NON-ZERO IN THE SWEEP. `pts=` is emitted even at zero
    // (tokens.js), but a zero-points screen makes DOS's APR the trivial one;
    // the class under study is the points APR.
    points: Math.round((0.005 + rnd() * 0.045) * 1e6) / 1e6,
    optionLabels: [],
  };
  const labels = [`perYr${perYr}`, `basis${basis}`];
  if (usa) labels.push('usa');
  if (inadv) labels.push('inadv');
  if (r78) labels.push('r78');
  if (exact) labels.push('exact');
  if (prepaid === 'yes') labels.push('prepaid');

  // Advanced options, each independently. The point of the sweep is to reach
  // the USA ∧ prepay ∧ adjustment corner OFTEN, so those three are weighted up.
  const gridDate = (k) => {
    const months = Math.round(12 / perYr) * k;
    const mm = ((m - 1 + months) % 12) + 1;
    const yy = y + Math.floor((m - 1 + months) / 12);
    return (yy <= 2085) ? ymd(yy, mm, d) : null;
  };
  if (rnd() < 0.55) {
    const k = 1 + Math.floor(rnd() * Math.max(1, nPeriods - 1));
    const dt = gridDate(k);
    if (dt) {
      c.prepays = [{ startDate: dt, nPmts: 1 + Math.floor(rnd() * 8), perYr, amount: Math.round(rnd() * 3000 * 100) / 100 }];
      labels.push('prepay');
    }
  }
  if (rnd() < 0.55) {
    const k = 1 + Math.floor(rnd() * Math.max(1, nPeriods - 1));
    const dt = gridDate(k);
    if (dt) {
      c.adjustments = c.adjustments || [];
      c.adjustments.push({ date: dt, rate: Math.round((0.02 + rnd() * 0.16) * 1e10) / 1e10, amount: '' });
      labels.push('adj-rate');
    }
  }
  if (rnd() < 0.45) {
    const k = 1 + Math.floor(rnd() * Math.max(1, nPeriods - 1));
    const dt = gridDate(k);
    if (dt) {
      c.adjustments = c.adjustments || [];
      c.adjustments.push({ date: dt, rate: '', amount: Math.round(rnd() * 5000 * 100) / 100 });
      labels.push('adj-amount');
    }
  }
  if (rnd() < 0.25) {
    const k = 1 + Math.floor(rnd() * Math.max(1, nPeriods - 1));
    const dt = gridDate(k);
    if (dt) {
      c.balloons = [{ date: dt, amount: Math.round(rnd() * 60000 * 100) / 100 }];
      labels.push('balloon');
    }
  }
  c.optionLabels = labels;
  c.caseIndex = i;
  return c;
}

// --------------------------------------------------------------------------
// main
// --------------------------------------------------------------------------
function arg(name, dflt) {
  const p = process.argv.find(a => a.startsWith(`--${name}=`));
  return p ? p.split('=').slice(1).join('=') : dflt;
}

async function main() {
  const mode = arg('mode', 'ablation');
  const outDir = __dirname;

  console.log('='.repeat(78));
  console.log(`r49_apr_arm — mode=${mode}`);
  console.log(`ORACLE ${ORACLE} — build flags -Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX.`);
  console.log('  -dACTU is ABSENT AND UNBUILDABLE (R47). This is the amortization oracle;');
  console.log('  the ACTUARIAL axis does not exist on any surface and cannot.');
  console.log(`PORT ${API} — the REST handler the shipped page calls (the CONSUMER, R42).`);
  console.log(`TOLERANCE tolAPR = ${tolAPR} (pinned by TestToleranceValuesArePinned).`);
  console.log(`SCOPE KEY 'reached' (standing since round 48), ceiling year ${SCOPE_CEILING_Y}.`);
  console.log('='.repeat(78));

  if (mode === 'ablation') {
    const all = subsets(FACTORS);
    const results = [];
    for (const s of all) {
      const c = ablationCase(s);
      const id = s.length ? s.join('+') : 'none';
      const r = await measure(c, id);
      results.push(r);
      const tag = r.verdict;
      const dA = r.aprCompare ? r.aprCompare.delta.toExponential(3) : '-';
      const dI = r.totals ? r.totals.delta.interest.toFixed(2) : '-';
      console.log(`${id.padEnd(34)} ${String(tag).padEnd(38)} dAPR=${dA}  dInterest=${dI}`);
    }
    const out = {
      arm: 'r49_apr_ablation',
      what: 'docs/discrepancies.md §89 §4 RE-DERIVED FROM THE INPUT at HEAD (R60). ' +
            'Four factors, all 16 subsets. The record claimed every option agrees ' +
            'in isolation and in every PAIR and only the triple diverges; this ' +
            'file measures that claim rather than repeating it.',
      scopeKey: 'reached',
      ceilingYear: SCOPE_CEILING_Y,
      tolerances: { tolAPR, tolTotalsFloor, tolTotalsRel },
      oracle: ORACLE,
      oracleBuildFlags: '-Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX (-dACTU ABSENT AND UNBUILDABLE)',
      api: API,
      factors: FACTORS,
      baseCase: SEC89_4,
      results,
    };
    fs.writeFileSync(path.join(outDir, 'r49_apr_ablation.json'), JSON.stringify(out, null, 1));
    console.log(`\nresults -> ${path.join(outDir, 'r49_apr_ablation.json')}`);
    return;
  }

  if (mode === 'sweep') {
    const n = parseInt(arg('n', '400'), 10);
    const seed = parseInt(arg('seed', '49049'), 10);
    const rnd = mulberry32(seed);
    console.log(`POPULATION seed=${seed} n=${n}`);
    console.log(`🚨 REPRODUCE THIS EXACT POPULATION WITH: node testplan/harness/aprclass/r49_apr_arm.js --mode=sweep --n=${n} --seed=${seed}   (R57 — the seed alone does NOT identify it)`);
    console.log(`POPULATION CANNOT PRODUCE: ${POPULATION.cannotProduce.join(' · ')}`);
    const results = [];
    const tally = {};
    for (let i = 0; i < n; i++) {
      const c = genCase(rnd, i);
      let r;
      try {
        r = await measure(c, `sweep-${i}`);
      } catch (e) {
        r = { id: `sweep-${i}`, verdict: 'HARNESS_MAPPING_ERROR', error: String(e.message), options: c.optionLabels };
      }
      results.push(r);
      tally[r.verdict] = (tally[r.verdict] || 0) + 1;
      if (r.verdict === 'APR_DIVERGENT_TOTALS_IDENTICAL') {
        console.log(`⭐ ${r.id} ${r.inScope ? 'IN SCOPE' : 'OUT OF SCOPE'} reached=${r.reachedYear} dAPR=${r.aprCompare.delta.toExponential(3)}  [${r.options.join(',')}]`);
        console.log(`   ${r.reproduce.oracleAPR}`);
      }
      if ((i + 1) % 50 === 0) process.stdout.write(`  ${i + 1}/${n}\n`);
    }
    const scored = results.filter(r => r.aprCompare);
    const inScopeScored = scored.filter(r => r.inScope);
    const summary = {
      n,
      byVerdict: tally,
      quarantinedOracleNoTermination: results.filter(r => r.verdict === 'QUARANTINED_ORACLE_NO_TERMINATION').length,
      aprComparedAtAll: scored.length,
      aprComparedInScope: inScopeScored.length,
      aprDivergentInScope: inScopeScored.filter(r => r.aprCompare.divergent).length,
      aprDivergentInScopeTotalsIdentical: inScopeScored.filter(r => r.aprCompare.divergent && r.totals.identical).length,
      aprDivergentInScopeTotalsWithinTol: inScopeScored.filter(r => r.aprCompare.divergent && r.totals.withinTolTotals).length,
      outOfScopeScored: scored.length - inScopeScored.length,
    };
    console.log('\n' + '='.repeat(78));
    console.log(JSON.stringify(summary, null, 1));
    console.log('🚨 A `DOS_DECLINED` or `QUARANTINED_*` row COMPARED NO APR. It is not in');
    console.log('   any denominator here (R54, R61).');
    const out = {
      arm: 'r49_apr_sweep',
      what: '§88 §5\'s OPEN measurement, widened: does an IN-SCOPE, totals-IDENTICAL ' +
            'APR-divergent case exist? The only clean isolate the project owns ' +
            '(§88 §5) has horizon 2133 and is OUT OF SCOPE.',
      population: POPULATION,
      seed, n,
      reproduce: `node testplan/harness/aprclass/r49_apr_arm.js --mode=sweep --n=${n} --seed=${seed}`,
      scopeKey: 'reached',
      ceilingYear: SCOPE_CEILING_Y,
      tolerances: { tolAPR, tolTotalsFloor, tolTotalsRel },
      oracle: ORACLE,
      oracleBuildFlags: '-Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX (-dACTU ABSENT AND UNBUILDABLE)',
      api: API,
      summary,
      results,
    };
    fs.writeFileSync(path.join(outDir, 'r49_apr_sweep.json'), JSON.stringify(out, null, 1));
    console.log(`results -> ${path.join(outDir, 'r49_apr_sweep.json')}`);
    return;
  }

  throw new Error(`unknown --mode=${mode}`);
}

if (require.main === module) {
  main().then(() => process.exit(0)).catch(e => { console.error(e); process.exit(2); });
}

module.exports = { measure, ablationCase, genCase, subsets, FACTORS, SEC89_4, POPULATION, runOracle, parseAPR, parseTotals, apiRequest, tolAPR };
