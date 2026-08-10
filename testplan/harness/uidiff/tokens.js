// uidiff/tokens.js — the UI-case → amort_oracle argv mapping.
//
// This file owns the ONE mapping from a UI-expressible screen to an oracle
// command line. Before this file existed the mapping was duplicated across
// testplan/harness/run_amz.js:25 (settings flags only, no date tokens, no
// adjustment-rate kicker) and ~10 Go test files; the DATE tokens bdate=/
// adjdmy=/predmy=/mordmy= were CONSTRUCTED NOWHERE and existed only as
// hand-written literals and as Pascal parsers at
// legacy/oracle/amort_oracle.pas:194/269/383/1004.
//
// 🚨 TRAP 2 LIVES HERE (docs/discrepancies.md §89.3, harness bug #2).
// The API kicks the TYPED rate by 365/360 to reach DOS's internal rate
// (internal/api/handlers.go:1949 amzKickerRate, call sites :995 for the loan
// rate and :1184 for the ADJUSTMENT rate). The oracle assigns the rate it is
// given DIRECTLY, unkicked. So on the 365/360 basis the oracle must be handed
// the KICKED rate — and it must be handed a kicked rate for the loan AND for
// every adjustment. Round 45b kicked only the loan rate and measured 63 of 63
// cases diverging: 100%, which is a MAPPING BUG, not a defect.
//
// 🚨 TRAP 3 LIVES HERE. The oracle's date tokens are D.M.Y (day first);
// the UI's fields are MM/DD/YYYY and the UI's own validator (dateValidity,
// cmd/persense/static/index.html:6592) REJECTS ISO. Two different orders and a
// validator that refuses a third. Every conversion below is asserted.

'use strict';

const KICKER = 365 / 360;

class MappingError extends Error {}

// ---------------------------------------------------------------------------
// Dates
// ---------------------------------------------------------------------------

// A case's canonical internal date form is {y, m, d} — never a string, so that
// no ambiguous string ever crosses a boundary unlabelled.
function ymd(y, m, d) {
  if (!Number.isInteger(y) || !Number.isInteger(m) || !Number.isInteger(d)) {
    throw new MappingError(`ymd: non-integer date part ${y}/${m}/${d}`);
  }
  if (m < 1 || m > 12) throw new MappingError(`ymd: month out of range: ${m}`);
  if (d < 1 || d > 31) throw new MappingError(`ymd: day out of range: ${d}`);
  // mordmy= is the only oracle token that range-checks its year (1900..2155,
  // because daterec.y is a byte based at 1900) — NF-6, fixed round 41. Apply
  // the tighter bound everywhere so no token can silently take a bad year.
  if (y < 1900 || y > 2155) throw new MappingError(`ymd: year out of DOS range: ${y}`);
  return { y, m, d };
}

// UI form. MM/DD/YYYY, zero-padded. This is the ONLY string form that may be
// written into a page field.
function toUI(t) {
  const p2 = n => String(n).padStart(2, '0');
  return `${p2(t.m)}/${p2(t.d)}/${t.y}`;
}

// Oracle form. D.M.Y, unpadded, full year — legacy/oracle/amort_oracle.pas:877.
function toOracle(t) {
  return `${t.d}.${t.m}.${t.y}`;
}

// ---------------------------------------------------------------------------
// Rates
// ---------------------------------------------------------------------------

// typed/displayed rate -> the rate the oracle must be given.
// Mirrors internal/api/handlers.go:1949 amzKickerRate.
function oracleRate(typedRate, basis) {
  if (typeof typedRate !== 'number' || !isFinite(typedRate)) {
    throw new MappingError(`oracleRate: non-finite rate ${typedRate}`);
  }
  return basis === '365/360' ? typedRate * KICKER : typedRate;
}

// ---------------------------------------------------------------------------
// argv
// ---------------------------------------------------------------------------

// Tokens that Halt(0) and so REPLACE the default stdout.
// legacy/oracle/amort_oracle.pas: payoff :1287, apr :1296, solverate :1308,
// datefrombalance :1318, solveterm :1326, solveballoon :1336, presolve :1345,
// predur :1353.
//
// 🚨 Some are bare and some carry `=VALUE`. The first version of this file
// listed 'payoff' in a Set and matched with .has(), so `payoff=1.1.2030` — the
// only form that can actually appear on a line — did not match and the guard
// silently admitted two output modes. The selftest caught it on its first run.
// That is the project's own standing warning: A GUARD NEEDLE CAN BE A PREFIX OF
// A SURVIVING NEIGHBOUR — match the full token form, both shapes.
const OUTPUT_MODE_BARE = new Set(['apr', 'solverate', 'solveterm']);
const OUTPUT_MODE_PARAM = ['payoff=', 'datefrombalance=', 'solveballoon=', 'presolve=', 'predur='];
const DATA_MODE_TOKENS = new Set(['adjdump', 'bdump', 'pdump', 'rows', 'dumpraw']);

function isOutputMode(a) {
  return OUTPUT_MODE_BARE.has(a) || OUTPUT_MODE_PARAM.some(p => a.startsWith(p));
}

// Build the argv for a generated case. `mode` is 'data' or 'apr'.
//
// 🚨 TRAP 1 IS ENFORCED HERE (docs/discrepancies.md §89.3, harness bug #1).
// `apr` and `bdump`/`adjdump` are SEPARATE OUTPUT MODES. `apr` Halt(0)s at
// amort_oracle.pas:1305, so everything after it — adjdump at :1367, the
// payment/interest/paid totals line at :1400 — is NEVER REACHED.
// `apr adjdump` returns `apr 0.000000 status 0`, which a naive parser reads as
// a real zero APR. Two invocations per case are therefore REQUIRED, and this
// function refuses to build a line that mixes the two.
function buildArgs(c, mode) {
  if (mode !== 'data' && mode !== 'apr') {
    throw new MappingError(`buildArgs: unknown mode ${mode}`);
  }
  const s = c.settings;
  const args = [];

  args.push(String(c.amount));
  args.push(oracleRate(c.rate, s.basis).toFixed(10));
  args.push(String(c.nPeriods));
  args.push(String(s.perYr));

  // Dates. loandmy=/firstdmy= must be applied before the grids are set up;
  // amort_oracle.pas:857-875 records that mis-ordering turned 85 of 95 cases
  // divergent, all harness artefact. The driver scans from ParamCount index 5
  // and order is otherwise irrelevant, but we emit in DOS's documented order.
  if (c.loanDate) args.push('loandmy=' + toOracle(c.loanDate));
  if (c.firstDate) args.push('firstdmy=' + toOracle(c.firstDate));
  if (c.lastDate) args.push('lastdmy=' + toOracle(c.lastDate));

  // Computational settings. The UI defaults are basis=360, prepaid=yes,
  // timing=arrears, exact=no, rule78=no, balloonIncl=no, interestRule=actuarial
  // — mirror them explicitly rather than relying on the oracle's defaults.
  if (s.basis === '365') args.push('b365');
  if (s.basis === '365/360') args.push('b365_360');
  if (s.prepaid === 'yes') args.push('prepaid');
  if (s.timing === 'advance') args.push('inadv');
  if (s.exact === 'yes') args.push('exact');
  if (s.rule78 === 'yes') args.push('r78');
  // 🚨 THE MAPPING IS INVERTED, AND THE PRIOR ART GETS IT BACKWARDS.
  // internal/api/handlers.go:972 is `PlusRegular: !req.BalloonIncludesRegular`,
  // and index.html:3488 sends balloonIncludesRegular ONLY when the select reads
  // 'yes'. So UI 'no' -> PlusRegular TRUE -> the oracle needs `plusreg`, and UI
  // 'yes' -> PlusRegular FALSE -> no token. The label means what it says: if the
  // balloon is INCLUDED IN the regular payment it REPLACES part of it.
  //   testplan/harness/run_amz.js:41 does `if (s.balloonIncl === 'yes')
  //   args.push('plusreg')` — the opposite. That is REACHABLE, not latent:
  //   AMZ-C-57 and AMZ-C-63 both set balloonIncl:'yes', so both are compared
  //   against the wrong DOS configuration today.
  // Consequence of getting this right: the web's SHIPPED default (balloonIncl
  // 'no') is ADD, while DOS's own default is REPLACE (PEDATA.pas:68
  // plus_regular:false). That is decision 3a.14 / item 23, ratified HOLD — DO
  // NOT FLIP. This harness therefore compares the web as shipped against the
  // DOS configuration the web actually asks for, and the ADD/REPLACE decision
  // is NOT re-reported as a defect on every extras screen.
  if (s.balloonIncl === 'no') args.push('plusreg');
  if (s.interestRule === 'usa') args.push('usa');

  // 🚨 `pts=` is ALWAYS emitted, including at zero. amort_oracle.pas:899-906
  // sets pointsstatus := inp, and that is what makes DOS RUN THE APR SOLVER AT
  // ALL — the value may be 0. Omitting it on zero-points screens makes the
  // oracle return `apr 0.000000 status 0` (the "declined to store" shape) on
  // every plain screen while the page paints a perfectly correct APR, so the
  // whole APR axis silently compares nothing. The identity controls caught this
  // on this harness's first run.
  args.push('pts=' + (c.points || 0));
  if (c.payment) args.push('payhard=' + c.payment);
  if (c.target) args.push('targ=' + c.target);
  if (c.skipMonths) args.push('skip=' + c.skipMonths);
  if (c.moratorium) args.push('mordmy=' + toOracle(c.moratorium));

  // Grids, all with DATE-based tokens so that no installment-number
  // translation can manufacture a divergence.
  for (const b of c.balloons || []) {
    args.push(`bdate=${toOracle(b.date)}:${b.amount}`);
  }
  for (const p of c.prepays || []) {
    args.push(`predmy=${toOracle(p.startDate)}:${p.nPmts}:${p.perYr}:${p.amount}`);
  }
  for (const a of c.adjustments || []) {
    // 🚨 THE KICKER MUST REACH THE ADJUSTMENT RATE TOO. This single line is
    // the difference between 0 of 63 and 63 of 63.
    const r = (a.rate === '' || a.rate == null) ? '' : oracleRate(a.rate, s.basis).toFixed(10);
    const amt = (a.amount === '' || a.amount == null) ? '' : String(a.amount);
    args.push(`adjdmy=${toOracle(a.date)}:${r}:${amt}`);
  }

  if (mode === 'data') {
    args.push('adjdump', 'bdump');
  } else {
    args.push('apr');
  }

  assertModePurity(args);
  return args;
}

// The executable form of trap 1. Called on every argv the harness builds AND
// on every argv it is handed, so a future edit cannot reintroduce the mix.
function assertModePurity(args) {
  const out = args.filter(isOutputMode);
  const data = args.filter(a => DATA_MODE_TOKENS.has(a));
  if (out.length > 1) {
    throw new MappingError(
      `TRAP 1: ${out.length} mutually-exclusive OUTPUT MODE tokens on one line ` +
      `(${out.join(', ')}). Each Halt(0)s; only the first can be observed.`);
  }
  if (out.length === 1 && data.length > 0) {
    throw new MappingError(
      `TRAP 1: output-mode token '${out[0]}' mixed with data-mode token(s) ` +
      `'${data.join(', ')}'. '${out[0]}' Halt(0)s at amort_oracle.pas:1305 and ` +
      `SUPPRESSES them — 'apr adjdump' returns 'apr 0.000000 status 0', which a ` +
      `naive parser reads as a real zero APR. Use two invocations.`);
  }
}

module.exports = {
  KICKER, MappingError,
  ymd, toUI, toOracle, oracleRate,
  buildArgs, assertModePurity, isOutputMode,
  OUTPUT_MODE_BARE, OUTPUT_MODE_PARAM, DATA_MODE_TOKENS,
};
