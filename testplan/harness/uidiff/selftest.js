// uidiff/selftest.js — THE THREE TRAPS AS EXECUTABLE ASSERTIONS.
//
// Round 45b found all three the hard way and wrote them down as prose. Prose
// does not fail a build. Every assertion below runs BEFORE any case is scored
// and ABORTS the run if it fails, and — per R24 — each one carries a POSITIVE
// CONTROL: it demonstrates against the REAL oracle binary that the hazard is
// live in this build, so that a green selftest is evidence rather than a
// tautology. A trap assertion that cannot be seen to fire is not a guard.

'use strict';
const record = require('./record');

const T = require('./tokens');
const O = require('./oracle');

function ok(cond, label, detail) {
  if (!cond) throw new Error(`SELFTEST FAILED — ${label}${detail ? ': ' + detail : ''}`);
}
function throws(fn, label) {
  let threw = false;
  try { fn(); } catch (e) { threw = true; }
  ok(threw, label, 'expected a throw, got none');
}

// A small, entirely on-grid case used by the positive controls. It carries one
// adjustment so the adjustment-rate kicker is exercised.
function probeCase(basis) {
  return {
    id: 'selftest', tier: 'selftest', options: [],
    amount: 100000, rate: 0.07, nPeriods: 60, points: 0.01,
    loanDate: T.ymd(2025, 1, 1), firstDate: T.ymd(2025, 2, 1),
    lastDate: null, payment: null, target: null, skipMonths: null, moratorium: null,
    balloons: [], prepays: [],
    adjustments: [{ date: T.ymd(2027, 2, 1), rate: 0.09, amount: '' }],
    settings: {
      perYr: 12, basis, prepaid: 'yes', timing: 'arrears',
      balloonIncl: 'no', exact: 'no', rule78: 'no', interestRule: 'actuarial',
    },
  };
}

// ---------------------------------------------------------------------------
// TRAP 1 — `apr` and `bdump`/`adjdump` are SEPARATE OUTPUT MODES.
// ---------------------------------------------------------------------------
function trap1(log) {
  // (a) the builder refuses to mix them.
  throws(() => T.assertModePurity(['100000', '0.07', '60', '12', 'apr', 'adjdump']),
    'TRAP 1 (a): assertModePurity accepted `apr adjdump`');
  throws(() => T.assertModePurity(['100000', '0.07', '60', '12', 'apr', 'bdump']),
    'TRAP 1 (a): assertModePurity accepted `apr bdump`');
  throws(() => T.assertModePurity(['100000', '0.07', '60', '12', 'apr', 'payoff=1.1.2030']),
    'TRAP 1 (a): assertModePurity accepted two output modes');
  // 🚨 The PARAMETERISED output modes, each in its only reachable form. The
  // first version of assertModePurity matched a bare-name Set and admitted
  // every one of these silently.
  for (const p of T.OUTPUT_MODE_PARAM) {
    throws(() => T.assertModePurity(['100000', '0.07', '60', '12', p + '1.1.2030', 'bdump']),
      `TRAP 1 (a): assertModePurity accepted the parameterised output mode '${p}' beside bdump`);
  }
  ok(T.isOutputMode('payoff=1.1.2030'), 'TRAP 1 (a)', 'payoff=VALUE not recognised as an output mode');
  ok(!T.isOutputMode('pts=0.01'), 'TRAP 1 (a)', 'pts= misclassified as an output mode');
  ok(!T.isOutputMode('prepaid'), 'TRAP 1 (a)', 'prepaid misclassified as an output mode');
  // and ACCEPTS the two legitimate lines — a guard that refuses everything is
  // not a guard.
  T.assertModePurity(T.buildArgs(probeCase('360'), 'data'));
  T.assertModePurity(T.buildArgs(probeCase('360'), 'apr'));

  // (b) POSITIVE CONTROL against the real binary: demonstrate that the hazard
  // is live in THIS oracle build. `apr adjdump` must return an apr line and
  // NO adjrow — that is the misread 45b nearly published.
  const c = probeCase('360');
  const mixed = T.buildArgs(c, 'data').filter(a => a !== 'bdump' && a !== 'adjdump').concat(['apr', 'adjdump']);
  // invoke() re-asserts purity and would refuse this line — which is the point.
  // The positive control has to call the binary DIRECTLY to demonstrate that
  // the hazard the guard prevents is real in this build.
  const { execFileSync } = require('child_process');
  let raw = '';
  try {
    raw = execFileSync(O.ORACLE, mixed, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], timeout: 20000 });
  } catch (e) { raw = (e.stdout || ''); }
  ok(/^apr /m.test(raw), 'TRAP 1 (b) positive control', '`apr adjdump` produced no apr line at all');
  ok(!/^adjrow /m.test(raw), 'TRAP 1 (b) positive control',
    '`apr adjdump` DID return adjrows — the Halt(0) at amort_oracle.pas:1305 is gone, ' +
    'so the two-invocation contract this harness is built on no longer holds');
  ok(!/^payment .* interest /m.test(raw), 'TRAP 1 (b) positive control',
    '`apr adjdump` returned the totals line');

  // (c) and the two-invocation path DOES yield both.
  const res = O.run(c);
  ok(res.data.payment != null, 'TRAP 1 (c)', 'data-mode run produced no payment line');
  ok(res.data.adjrows.length > 0, 'TRAP 1 (c)', 'data-mode run produced no adjrows');
  ok(res.apr.apr != null, 'TRAP 1 (c)', 'apr-mode run produced no apr line');
  log(`  trap 1 OK — apr-mode suppresses adjrows in this build; two invocations yield ` +
      `payment=${res.data.payment} adjrows=${res.data.adjrows.length} apr=${res.apr.apr}`);
}

// ---------------------------------------------------------------------------
// TRAP 2 — the 365/360 kicker, on the loan rate AND the adjustment rates.
// ---------------------------------------------------------------------------
function trap2(log) {
  // (a) the scalar mapping.
  ok(Math.abs(T.oracleRate(0.07, '365/360') - 0.07 * 365 / 360) < 1e-15,
    'TRAP 2 (a)', 'oracleRate does not kick on 365/360');
  ok(T.oracleRate(0.07, '360') === 0.07, 'TRAP 2 (a)', 'oracleRate kicked on the 360 basis');
  ok(T.oracleRate(0.07, '365') === 0.07, 'TRAP 2 (a)', 'oracleRate kicked on the 365 basis');

  // (b) THE ONE THAT ACTUALLY BIT: the adjustment rate must be kicked too.
  const args = T.buildArgs(probeCase('365/360'), 'data');
  const adj = args.find(a => a.startsWith('adjdmy='));
  ok(!!adj, 'TRAP 2 (b)', 'no adjdmy= token was emitted');
  const adjRate = parseFloat(adj.split(':')[1]);
  const expect = 0.09 * T.KICKER;
  ok(Math.abs(adjRate - expect) < 1e-9, 'TRAP 2 (b)',
    `the adjustment rate reached the oracle UNKICKED (${adjRate} vs expected ${expect}). ` +
    `This single omission produced 63 of 63 diverging in round 45b — 100%, ` +
    `which is a mapping bug, not a defect.`);
  const loanRate = parseFloat(args[1]);
  ok(Math.abs(loanRate - 0.07 * T.KICKER) < 1e-9, 'TRAP 2 (b)', 'the loan rate reached the oracle unkicked');
  // and NOT kicked on the 360 basis.
  const args360 = T.buildArgs(probeCase('360'), 'data');
  ok(Math.abs(parseFloat(args360[1]) - 0.07) < 1e-12, 'TRAP 2 (b)', '360-basis loan rate was kicked');
  ok(Math.abs(parseFloat(args360.find(a => a.startsWith('adjdmy=')).split(':')[1]) - 0.09) < 1e-12,
    'TRAP 2 (b)', '360-basis adjustment rate was kicked');

  // (c) POSITIVE CONTROL against the real binary: the UNKICKED line must
  // actually disagree with the kicked one, or the kicker is not load-bearing
  // in this build and (b) is a statement about nothing. R51: an inert mutant on
  // a healthy population is a statement about the mutant.
  const c = probeCase('365/360');
  const kicked = O.runData(c);
  const unkickedArgs = T.buildArgs(c, 'data').slice();
  unkickedArgs[1] = (0.07).toFixed(10);
  const iAdj = unkickedArgs.findIndex(a => a.startsWith('adjdmy='));
  const parts = unkickedArgs[iAdj].split(':');
  parts[1] = (0.09).toFixed(10);
  unkickedArgs[iAdj] = parts.join(':');
  const { execFileSync } = require('child_process');
  let raw = '';
  try {
    raw = execFileSync(O.ORACLE, unkickedArgs, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], timeout: 20000 });
  } catch (e) { raw = (e.stdout || ''); }
  const m = raw.match(/payment ([\d.-]+) +interest ([\d.-]+) +paid ([\d.-]+)/);
  ok(!!m, 'TRAP 2 (c) positive control', 'the unkicked control run produced no totals line');
  const unkickedPaid = +m[3];
  ok(Math.abs(unkickedPaid - kicked.paid) > 1.0, 'TRAP 2 (c) positive control',
    `kicked and UNKICKED 365/360 runs agree (paid ${kicked.paid} vs ${unkickedPaid}). ` +
    `The kicker is not load-bearing on this case, so assertion (b) is vacuous — ` +
    `pick a probe case where it bites before trusting this selftest.`);
  // (d) THE OUTBOUND HALF — and this assertion has to exercise THE REAL
  // COMPARISON, not arithmetic it performs itself. The first version of this
  // block wrote `const internal = typed * KICKER` and then asserted
  // `internal / KICKER === typed`: a statement about two lines of its own
  // arithmetic that could not fail for any reason connected to compare.js.
  // R46's adversarial audit called it correctly — deleting the ternary in
  // compare.js:217 left it green. So drive compare.compareCase itself.
  const C = require('./compare');
  const mkCase = (basis) => Object.assign(probeCase(basis), { target: null, moratorium: null });
  const mkUI = (paintedPct) => ({
    inputDateRejects: [], paintedDateRejects: [], refused: false, threw: null, err: '',
    totalPaid: 1, totalInterest: 1, regularPayment: 1, apr: null, aprCell: '',
    paintedAdj: [{ i: 0, date: '02/01/2027', rate: paintedPct, amount: '' }],
    lastBalance: 0,
  });
  const mkOra = (dosInternalRate) => ({
    data: {
      paid: 1, interest: 1, payment: 1, adjrows: [{
        i: 0, date: { y: 2027, m: 2, d: 1 }, rate: dosInternalRate,
        rateStatus: 'outp', amount: 0, amtStatus: 'empty', amtOK: 'false',
      }],
    },
    apr: { aprBlank: true, apr: null }, err: null,
  });
  const verdict = (basis, paintedPct, dosRate) => {
    const cs = C.compareCase(mkCase(basis), mkUI(paintedPct), mkOra(dosRate));
    const k = cs.find(x => x.k.startsWith('adjRate@'));
    ok(!!k, 'TRAP 2 (d)', 'compareCase produced no adjRate check — the probe does not reach the code');
    return k.ok;
  };
  const internal = 0.09 * T.KICKER;
  // On 365/360 the page paints TYPED 9%; DOS's adjdump reports INTERNAL. The
  // comparison must AGREE. Without the re-kick in compare.js:217 it cannot.
  ok(verdict('365/360', '9', internal), 'TRAP 2 (d)',
    'compare.js did NOT re-kick the painted adjustment rate on the 365/360 basis: ' +
    'the page paints TYPED space (handlers.go:1449 amzUnkickerRate, the NF-1c fix) ' +
    'while adjdump reports INTERNAL space, so every adjustment-carrying 365/360 ' +
    'screen will book a false divergence.');
  // and it must NOT re-kick on the 360 basis — a re-kick everywhere would be
  // just as wrong in the other direction.
  ok(verdict('360', '9', 0.09), 'TRAP 2 (d)', 'compare.js disagreed on an identical 360-basis rate');
  ok(!verdict('360', '9', internal), 'TRAP 2 (d)',
    'compare.js re-kicked on the 360 basis, where painted and internal are the SAME space');
  ok(!verdict('365/360', '9', 0.09), 'TRAP 2 (d)',
    'compare.js accepted an UNKICKED DOS rate on the 365/360 basis — the check is inert');
  log(`  trap 2 OK — kicker is load-bearing: kicked paid=${kicked.paid}, ` +
      `unkicked paid=${unkickedPaid} (Δ${(unkickedPaid - kicked.paid).toFixed(2)}); ` +
      `outbound: compare.js re-kicks on 365/360 and not on 360, verified BOTH ways`);
}

// ---------------------------------------------------------------------------
// TRAP 3 — MM/DD/YYYY for the UI, D.M.Y for the oracle, ISO NEVER.
// ---------------------------------------------------------------------------
function trap3(log) {
  const t = T.ymd(2029, 1, 5); // 5 January 2029 — ambiguous under a swap
  ok(T.toUI(t) === '01/05/2029', 'TRAP 3', `toUI produced "${T.toUI(t)}", not MM/DD/YYYY`);
  ok(T.toOracle(t) === '5.1.2029', 'TRAP 3', `toOracle produced "${T.toOracle(t)}", not D.M.Y`);
  ok(T.toUI(t) !== T.toOracle(t), 'TRAP 3', 'the two date forms are indistinguishable — ' +
    'a swap could not be detected by this probe; pick an asymmetric date');
  // No ISO anywhere.
  ok(!/^\d{4}-/.test(T.toUI(t)) && !/^\d{4}-/.test(T.toOracle(t)),
    'TRAP 3', 'an ISO-shaped date escaped the mapping');
  // The year bound NF-6 fixed in round 41.
  throws(() => T.ymd(2200, 1, 1), 'TRAP 3', 'ymd accepted a year outside DOS\'s 1900..2155 range');
  log('  trap 3 OK — UI 01/05/2029 vs oracle 5.1.2029, distinct and non-ISO');
}

// The page-dependent half of trap 3. Called by run.js once the page is open,
// because only the shipped page owns dateValidity().
async function trap3Live(page, log) {
  const r = await page.evaluate(() => ({
    iso: dateValidity('2029-01-05'),
    us: dateValidity('01/05/2029'),
    dmy: dateValidity('5.1.2029'),
  }));
  ok(r.us.valid, 'TRAP 3 (live)', 'the page REJECTED MM/DD/YYYY — the harness cannot type any date');
  ok(!r.iso.valid, 'TRAP 3 (live)',
    'the page now ACCEPTS ISO. §89 turned on dateValidity rejecting ISO while ' +
    'parseDate accepted it; if that has changed, re-derive the §89 finding ' +
    'before relying on this harness\'s date handling.');
  ok(!r.dmy.valid, 'TRAP 3 (live)',
    'the page accepts the ORACLE\'s D.M.Y form, so a D.M.Y string typed into a ' +
    'field by mistake would pass validation and be silently misread');
  log(`  trap 3 live OK — page: MM/DD/YYYY valid, ISO rejected ("${r.iso.msg}"), D.M.Y rejected`);
}

// The page's scalar parse/format contracts, asserted against the LIVE page.
// Every one of these is a conversion the harness performs on a painted value;
// if the page changes its side the harness must fail loudly rather than
// compare two different spaces. The identity controls caught a 100× APR error
// here on this harness's first run.
async function liveScalarAssertions(page, log) {
  const r = await page.evaluate(() => ({
    ratePct: parseRate('7.0000%'),
    rateBare: parseRate('13'),
    money: parseMoney('$1,234.56'),
    moneyNeg: parseMoney('$-1,234.56'),
    int2: parseInt2('360'),
  }));
  ok(r.ratePct === 7, 'LIVE SCALAR',
    `parseRate('7.0000%') === ${r.ratePct}, expected 7. The page's rate cells are ` +
    `in PERCENT space and driver.js divides by 100 to reach the oracle's DECIMAL ` +
    `space. If parseRate now returns a decimal, that division is a 100× error on ` +
    `every APR comparison — remove it before running again.`);
  ok(r.rateBare === 13, 'LIVE SCALAR', `parseRate('13') === ${r.rateBare}, expected 13`);
  ok(r.money === 1234.56, 'LIVE SCALAR', `parseMoney('$1,234.56') === ${r.money}`);
  ok(r.moneyNeg === -1234.56, 'LIVE SCALAR',
    `parseMoney('$-1,234.56') === ${r.moneyNeg} — negative painted money does not ` +
    `round-trip, so any negative cell this harness reads back is wrong`);
  ok(r.int2 === 360, 'LIVE SCALAR', `parseInt2('360') === ${r.int2}`);
  log(`  live scalar OK — parseRate is PERCENT space (7.0000% -> 7), parseMoney round-trips ±`);
}

// trap4 — ROUND 48, ITEM 0y. THE RESULTS-FILE GATE, EXERCISED RATHER THAN
// DESCRIBED.
//
// 🚨 WHY THIS IS A LIVE SELFTEST AND NOT A SOURCE GUARD. Round 48's first
// attempt pinned the filename rule with a Go source scanner. The round's own
// adversarial audit killed it with four mutants the scanner could not see —
// including `const errored = []`, which IS round 47's exact bug (an all-errored
// run writing the committed baseline and exiting 0) with every needle still
// present as text. A test that cannot execute the harness cannot pin the
// harness. This runs on EVERY uidiff invocation, before anything is scored.
//
// The truth table is small and total, so it is asserted in full — including the
// precedence question (an errored PARTIAL run must report as ERRORED, because
// that is the more serious fault and the one that must not be silent).
function trap4(log) {
  const cases = [
    { in: { erroredCount: 0, only: null }, want: record.BASELINE,
      why: 'a clean, complete run is the only thing allowed to write the baseline' },
    { in: { erroredCount: 1, only: null }, want: record.ERRORED,
      why: 'ONE errored case is enough — it compared nothing (R54/R58)' },
    { in: { erroredCount: 189, only: null }, want: record.ERRORED,
      why: "round 47's actual run: 189 of 200 stacked errored and it wrote the baseline" },
    { in: { erroredCount: 0, only: 'plain' }, want: record.PARTIAL,
      why: "round 48's own --only=plain smoke run destroyed the 258-case baseline (audit F4)" },
    { in: { erroredCount: 0, only: 'stacked' }, want: record.PARTIAL,
      why: 'any tier filter is a different population' },
    { in: { erroredCount: 3, only: 'stacked' }, want: record.ERRORED,
      why: 'errors take precedence over partial — the more serious fault wins' },
  ];
  for (const c of cases) {
    ok(record.chooseResultsFile(c.in) === c.want,
       `results gate ${JSON.stringify(c.in)} -> ${c.want}`,
       `${c.why}; got ${record.chooseResultsFile(c.in)}`);
  }
  // POSITIVE CONTROL (R24): the three outcomes must be DISTINCT. A rule that
  // returned one filename for everything would satisfy nothing above if the
  // expectations were also collapsed, and this is the assertion that notices.
  ok(new Set([record.BASELINE, record.ERRORED, record.PARTIAL]).size === 3,
     'the three results-file names are distinct',
     'a quarantine path equal to the baseline path quarantines nothing');
  // And the gate must REFUSE a malformed count rather than defaulting to the
  // baseline: `undefined > 0` is false, which would have written the baseline.
  throws(() => record.chooseResultsFile({ erroredCount: undefined, only: null }),
         'a missing erroredCount is refused, not treated as zero');
  throws(() => record.chooseResultsFile({ erroredCount: -1, only: null }),
         'a negative erroredCount is refused');
  log(`  trap4 OK — the results-file gate is exercised on ${cases.length} inputs, ` +
      `not merely described (item 0y, audit F5)`);
}

function runAll(log) {
  log('SELFTEST — the traps, with positive controls against the real oracle:');
  trap1(log); trap2(log); trap3(log); trap4(log);
  log('SELFTEST PASSED.');
}

module.exports = { runAll, trap1, trap2, trap3, trap4, trap3Live, liveScalarAssertions, probeCase };
