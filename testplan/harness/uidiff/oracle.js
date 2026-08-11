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

const { spawnSync } = require('child_process');
const { buildArgs, assertModePurity } = require('./tokens');

const ORACLE = process.env.PERSENSE_ORACLE || '/tmp/oraclebuild/amort_oracle';

const ORACLE_TIMEOUT_MS = 20000;

// 🚨 R61 — A TIMEOUT IS NOT A REFUSAL. R64 — NAME THE CHANNEL.
//
// The round-48 version of this function was:
//
//	stdio: ['ignore', 'pipe', 'ignore'], timeout: 20000
//	catch (e) { return (e.stdout || '') + '\nERR exit'; }
//
// Two defects, and they compound:
//
//  1. EVERY failure mode became the single string `ERR exit`. A non-zero exit,
//     a spawn failure and THE 20-SECOND TIMEOUT KILL were indistinguishable.
//     `stacked-96` is an oracle that DOES NOT TERMINATE (zero bytes at 310 s),
//     and it was scored as a refusal — i.e. as a statement about what the
//     engine SAID — in a case where the engine said nothing at all. A case
//     that produced NO OUTPUT belongs in no rate about engine behaviour.
//
//  2. `stdio[2] = 'ignore'` DISCARDED STDERR. A later classification of the
//     both-refuse screens read stderr, which this instrument never sees; the
//     classes overlapped by 17 screens and the conditioned base rate was wrong
//     by 4.4x. An instrument and a classification of its output must read the
//     SAME CHANNEL, and the channel must be named.
//
// So: stderr is captured (not merged — captured SEPARATELY, so `refusal()`
// keeps reading exactly the stdout channel it always read and no rate silently
// changes population), and the process outcome is returned as structured
// fields rather than smuggled into the text.
//
// `run()` below returns `err` UNCHANGED in meaning — a refusal string read
// from STDOUT — and adds `nonTerminating`, so a caller that wants to quarantine
// hangs can, and a caller that does not is not silently given a different
// number than it had before.
function invokeDetailed(args, timeoutMs) {
  assertModePurity(args); // consumer-side re-check; see header
  // spawnSync, NOT execFileSync. execFileSync exposes stderr ONLY on the error
  // object, so a SUCCESSFUL run threw its stderr away no matter what stdio said
  // — which is R64 surviving the very fix that was supposed to end it. The
  // round's own trap5 caught this on its first execution; the literal-shape
  // version of the trap that preceded it could not have.
  const r = spawnSync(ORACLE, args, {
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
    timeout: (typeof timeoutMs === 'number' ? timeoutMs : ORACLE_TIMEOUT_MS),
    maxBuffer: 64 * 1024 * 1024,
  });
  const rec = {
    stdout: r.stdout || '',
    stderr: r.stderr || '',
    // spawnSync reports a timeout kill as signal='SIGTERM' with status===null.
    // A signal is NOT an exit status: an exit status means the engine ran and
    // decided; a signal means it never got to.
    exitCode: (typeof r.status === 'number') ? r.status : null,
    signal: r.signal || null,
    timedOut: false,
    spawnError: null,
  };
  if (r.error) {
    // ETIMEDOUT is what spawnSync sets on the timeout path; ENOENT is a missing
    // binary. They are different facts and must not collapse into one string.
    if (r.error.code === 'ETIMEDOUT') rec.timedOut = true;
    else if (typeof r.error.code === 'string') rec.spawnError = r.error.code;
  }
  if (!rec.timedOut && rec.signal === 'SIGTERM' && rec.exitCode === null) rec.timedOut = true;
  return rec;
}

// Back-compat text form, for callers that only want stdout. It NO LONGER
// appends `ERR exit`: doing so manufactured a refusal line on stdout that the
// oracle never printed, which is how a hang entered a refusal rate. A caller
// that needs to know the process failed must read the structured record.
function invoke(args) {
  return invokeDetailed(args).stdout;
}

// The refusal reader. CHANNEL: STDOUT, and only stdout — this is the channel
// `amort_oracle` prints `ERR ...` / `ENGINE ERROR:` on, and it is the channel
// every published refusal figure was measured over. Any reclassification of
// these screens must read this same channel or say plainly that it does not.
function refusal(out) {
  const m = out.match(/(?:^ERR|ENGINE ERROR:?) ?(.*)/m);
  return m ? (m[1] || 'refused').trim() : null;
}

// The R61 predicate, as a PURE FUNCTION so it can be exercised directly by the
// selftest rather than described in a comment. A source-scanning guard cannot
// pin behaviour in another language (round 48's Go guard survived four
// semantic mutants of this very directory), so the rule has to be code that
// RUNS and a test that CALLS it.
//
// Returns 'NON_TERMINATING' | 'PROCESS_FAILED' | 'OK'. A NON_TERMINATING
// result must never be folded into a refusal-parity rate.
function processOutcome(rec) {
  if (rec.timedOut) return 'NON_TERMINATING';
  if (rec.spawnError) return 'PROCESS_FAILED';
  if (rec.signal) return 'NON_TERMINATING';
  if (rec.exitCode !== 0 && rec.exitCode !== null) return 'PROCESS_FAILED';
  return 'OK';
}

// Data-mode invocation: totals, payment, the adjustment grid and the balloon
// grid. NO apr token — see the header.
function runData(c, timeoutMs) {
  const args = buildArgs(c, 'data');
  const rec = invokeDetailed(args, timeoutMs);
  const out = rec.stdout;
  const res = {
    mode: 'data', args, raw: out.trim(), err: refusal(out),
    // R61/R64: the process facts, kept separate from what the engine SAID.
    process: {
      exitCode: rec.exitCode, signal: rec.signal, timedOut: rec.timedOut,
      spawnError: rec.spawnError, outcome: processOutcome(rec),
      stderrBytes: rec.stderr.length, stderrHead: rec.stderr.slice(0, 200),
    },
    nonTerminating: processOutcome(rec) === 'NON_TERMINATING',
  };
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
function runAPR(c, timeoutMs) {
  const args = buildArgs(c, 'apr');
  const rec = invokeDetailed(args, timeoutMs);
  const out = rec.stdout;
  const res = {
    mode: 'apr', args, raw: out.trim(), err: refusal(out),
    process: {
      exitCode: rec.exitCode, signal: rec.signal, timedOut: rec.timedOut,
      spawnError: rec.spawnError, outcome: processOutcome(rec),
      stderrBytes: rec.stderr.length, stderrHead: rec.stderr.slice(0, 200),
    },
    nonTerminating: processOutcome(rec) === 'NON_TERMINATING',
  };
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

function run(c, timeoutMs) {
  const data = runData(c, timeoutMs);
  const apr = runAPR(c, timeoutMs);
  // 🚨 R61. `err` keeps its OLD meaning exactly — a refusal string read from
  // STDOUT — so no existing rate changes population silently. `nonTerminating`
  // is the NEW, separate fact. A screen with nonTerminating=true produced no
  // answer and must be QUARANTINED, not counted as a refusal.
  return {
    data, apr,
    err: data.err || apr.err,
    nonTerminating: data.nonTerminating || apr.nonTerminating,
    processOutcome: data.nonTerminating || apr.nonTerminating
      ? 'NON_TERMINATING'
      : (data.process.outcome === 'OK' && apr.process.outcome === 'OK' ? 'OK' : 'PROCESS_FAILED'),
  };
}

module.exports = {
  run, runData, runAPR, ORACLE, invoke, invokeDetailed, processOutcome,
  refusal, ORACLE_TIMEOUT_MS,
};
