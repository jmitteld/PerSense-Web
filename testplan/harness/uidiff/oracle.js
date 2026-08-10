// uidiff/oracle.js — the TWO-INVOCATION amort_oracle runner.
//
// 🚨 WHY TWO INVOCATIONS (docs/discrepancies.md §89.3, harness bug #1).
// `apr` is an OUTPUT MODE: legacy/oracle/amort_oracle.pas:1296 prints the apr
// line and then Halt(0)s at :1305. Everything after it is unreachable —
// adjdump (:1367), the payment/interest/paid totals line (:1400), rows,
// dumpraw. So `apr adjdump` prints ONLY `apr 0.000000 status 0` when the APR
// did not converge, and a naive parser reads that as a real zero APR while
// also silently seeing zero adjustment rows.
//
// tokens.js refuses to BUILD a mixed line. This file additionally refuses to
// RUN one, and asserts the shape of what came back — a defence at the
// producer and at the consumer, because §89 is precisely the story of a guard
// that checked the producer twice and the consumer never.

'use strict';

const { execFileSync } = require('child_process');
const { buildArgs, assertModePurity } = require('./tokens');

const ORACLE = process.env.PERSENSE_ORACLE || '/tmp/oraclebuild/amort_oracle';

function invoke(args) {
  assertModePurity(args); // consumer-side re-check; see header
  try {
    return execFileSync(ORACLE, args, {
      encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], timeout: 20000,
    });
  } catch (e) {
    return (e.stdout || '') + '\nERR exit';
  }
}

function refusal(out) {
  const m = out.match(/(?:^ERR|ENGINE ERROR:?) ?(.*)/m);
  return m ? (m[1] || 'refused').trim() : null;
}

// Data-mode invocation: totals, payment, the adjustment grid and the balloon
// grid. NO apr token — see the header.
function runData(c) {
  const args = buildArgs(c, 'data');
  const out = invoke(args);
  const res = { mode: 'data', args, raw: out.trim(), err: refusal(out) };
  let m;
  if ((m = out.match(/payment ([\d.-]+) +interest ([\d.-]+) +paid ([\d.-]+)/))) {
    res.payment = +m[1]; res.interest = +m[2]; res.paid = +m[3];
  }
  res.adjrows = [];
  const adjRe = /^adjrow (\d+) date (\d+)\/(\d+)\/(\d+) rate ([\d.eE+-]+) ratestatus (\S+) amount ([\d.eE+-]+) amtstatus (\S+) amtok (\S+)/gm;
  while ((m = adjRe.exec(out))) {
    res.adjrows.push({
      i: +m[1], date: { m: +m[2], d: +m[3], y: +m[4] },
      rate: +m[5], rateStatus: m[6], amount: +m[7], amtStatus: m[8], amtOK: m[9],
    });
  }
  res.balloonrows = [];
  const balRe = /^balloonrow (\d+) date (\d+)\/(\d+)\/(\d+) dstatus (\S+) amount ([\d.eE+-]+) astatus (\S+)/gm;
  while ((m = balRe.exec(out))) {
    res.balloonrows.push({
      i: +m[1], date: { m: +m[2], d: +m[3], y: +m[4] },
      dStatus: m[5], amount: +m[6], aStatus: m[7],
    });
  }
  if ((m = out.match(/^lastdate (\d+)\/(\d+)\/(\d+)/m))) res.lastDate = { m: +m[1], d: +m[2], y: +m[3] };
  if ((m = out.match(/^nperiods (\d+)/m))) res.nPeriods = +m[1];
  if ((m = out.match(/^nballoons (\d+) nlines (\d+)/m))) res.nLines = +m[2];

  // 🚨 DOS SIGNALS "NO TABLE" WITH A -1 SENTINEL, NOT WITH AN ERROR LINE.
  // Measured on stacked-130: `nballoons 0 nlines 0` / `payment 0.0000
  // interest -1.00 paid -1.00`, with NO ERR and NO `ENGINE ERROR` line. A
  // parser that reads the totals line numerically books interest = -$1.00 and
  // paid = -$1.00 as real DOS answers and then reports a $759,203 divergence
  // against a page that painted a perfectly ordinary 140-row schedule. That is
  // a harness fault reported as the largest divergence in the run.
  if (res.payment === 0 && res.interest === -1 && res.paid === -1) {
    res.err = res.err || 'DOS produced no table (-1 sentinel on both totals, nlines ' + res.nLines + ')';
    res.sentinel = true;
    delete res.payment; delete res.interest; delete res.paid;
  }

  // Consumer-side assertion of trap 1: a data-mode run must NEVER carry an apr
  // line. If it does, the oracle's output modes have changed underneath us and
  // every apr number this harness has ever published is suspect.
  if (/^apr /m.test(out)) {
    throw new Error(
      'TRAP 1 VIOLATED: a data-mode invocation returned an `apr` line. ' +
      'The oracle\'s output modes are not what this harness assumes; ' +
      'stop and re-read legacy/oracle/amort_oracle.pas:1296-1305 before ' +
      'believing any APR figure.');
  }
  return res;
}

// APR-mode invocation. Nothing else may be read from it.
function runAPR(c) {
  const args = buildArgs(c, 'apr');
  const out = invoke(args);
  const res = { mode: 'apr', args, raw: out.trim(), err: refusal(out) };
  const m = out.match(/^apr ([\d.eE+-]+) status (\d+)/m);
  if (m) { res.apr = +m[1]; res.aprStatus = +m[2]; }

  // Consumer-side assertion of trap 1, the other direction: an apr-mode run
  // must NOT carry the totals line or adjrows. If it does, the Halt(0) is gone
  // and a future maintainer may be tempted to collapse to one invocation —
  // which is exactly how the zero-APR misread happens.
  if (/^payment .* interest /m.test(out) || /^adjrow /m.test(out)) {
    throw new Error(
      'TRAP 1 VIOLATED: an apr-mode invocation returned data-mode lines. ' +
      'amort_oracle.pas:1305 Halt(0) appears to be gone. Re-derive the ' +
      'two-invocation contract before trusting this run.');
  }
  // `apr 0.000000 status 0` is the shape a naive parser reads as a real zero.
  // Label it explicitly so no downstream comparison can mistake it.
  if (res.apr === 0 && res.aprStatus === 0) res.aprBlank = true;
  return res;
}

function run(c) {
  const data = runData(c);
  const apr = runAPR(c);
  return { data, apr, err: data.err || apr.err };
}

module.exports = { run, runData, runAPR, ORACLE, invoke };
