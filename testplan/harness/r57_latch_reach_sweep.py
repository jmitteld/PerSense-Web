#!/usr/bin/env python3
"""r57_latch_reach_sweep.py — R73's second generator for round 57's
per-adjustment implied-rate latch, WITH the positive control that r57's first
attempt at this measurement did not have.

WHY THIS EXISTS, AND WHAT IT REPLACES
=====================================
Round 57 fixed the port's AO6 (amount-given, rate-blank) adjustment branch to
solve the implied rate ONCE per screen and reuse it, as DOS does
(AMORTOP.pas:1515-1541, `adj[next_adj]^.loanratestatus := outp`).

Its first safety measurement swept 4,374 "simple single-adjustment screens" and
reported that the latch changed no output on any of them. **That null had ZERO
POWER and the round's own adversarial auditor refuted it.** The latch can only
change an answer when the REUSE branch is entered, and the reuse branch is
entered only when the screen is walked more than once. The second walk is the
APR value walk (`aprValueCashflows`), which runs only when the screen carries
POINTS. Every screen in that sweep had `Points = 0`. Measured:

    without points: 471 screens latched, REUSED   0
    with    points: 471 screens latched, REUSED 471

So the sweep sampled a stratum on which the fix is inert BY CONSTRUCTION. R69:
a null result can be true by construction; publish the ELIGIBLE COUNT. R76: name
the hypothesis the gate had power over — that one had power over nothing.

The same objection applies to `r53_segment_bound_sweep.py`, which this project
would otherwise reach for as the R73 instrument for anything in the segment
solver family: its factorial grid emits no `pts=` token at all, so its 1,855
KEPT_UNMOVED screens are 1,855 screens on which round 57's fix cannot bite.
Its PASS is real and is about a different hypothesis.

WHAT THIS SWEEP DOES
====================
A factorial grid of screens that each carry an AO6 adjustment, run TWICE — once
with `pts=` and once without — through three binaries:

  * the DOS oracle          — the only authority on whether the screen has an answer
  * a PRISTINE goamort      — built before the fix
  * a PATCHED goamort       — built after it

and it reports, separately for the points stratum and the no-points stratum:

  LOST an answer      pristine answered, patched refused.  Of those, the ones
                      where DOS SOLVES the row are the R67/R73 blocker: a fix
                      must never turn an answer DOS gives into a refusal.
  GAINED an answer    the mirror, reported for symmetry, never celebrated.
  MOVED               both answered and an OUTPUT differs. ⚠️ TWO COLUMNS, AND
                      THE SECOND ONE IS THE LOAD-BEARING ONE. The reuse branch
                      fires ONLY inside the APR value walk, so the fix's effect
                      surfaces in `apr` mode, NOT in default mode's interest
                      column. The first version of this sweep compared only
                      interest and was therefore blind to the one channel the fix
                      can move: a mutant multiplying the latched rate by 1.02
                      left default stdout BYTE-IDENTICAL and moved `apr` from
                      -0.063175 to -0.064715, and this sweep passed it at exit 0.
                      Found by round 57's own SECOND audit pass. R82: fixing a
                      fail-open is where this project introduces fail-opens —
                      pass 1 killed a sweep with no reach, and the replacement
                      proved reach and then measured a column the reach cannot
                      touch. BOTH columns now gate the exit status.
  REACH               how many screens ENTERED the reuse branch, read from
                      goamort's `GOAMORT_ADJLATCH=1` stderr line. THIS IS THE
                      POSITIVE CONTROL. A points stratum with REACH = 0 means
                      the sweep measured nothing and the run FAILS.

EXIT STATUS
  0  no screen DOS solves lost its answer, NO output moved, and the points
     stratum had reach.
  1  a LOST-with-DOS-solving screen, ANY moved output, or a vacuous reach
     control. ⚠️ `moved` GATES. In the first version it was computed, printed,
     and never appended to `fail` — a number with no consequence is not a gate.

USAGE
  r57_latch_reach_sweep.py --pristine /tmp/goamort_pristine \\
      --patched /tmp/goamort_patched --oracle /tmp/oraclebuild/amort_oracle \\
      --out /tmp/r57_latch_sweep.json [--jobs 2] [--limit N]

⚠️ The PRISTINE binary must predate the fix; if it does not, the LOST/GAINED
columns are an arithmetic identity and mean nothing (R78). This is checked as
far as it can be: the two binaries' md5s must DIFFER, and the pristine one must
print no ADJLATCH line under GOAMORT_ADJLATCH=1.
"""

import argparse
import concurrent.futures
import hashlib
import itertools
import json
import os
import re
import subprocess
import sys

ANCHOR = ["loandmy=28.2.2023", "firstdmy=28.8.2023"]

# The grid. Every screen carries exactly one AO6 row (`adj=<n>::<amount>`) so the
# latch has something to latch; the other axes vary the shape around it.
AMOUNTS = ["27055.77", "100000.00", "250000.00"]
RATES = ["0.0306000000", "0.0819564764", "0.1200000000"]
TERMS = [("48", "6"), ("108", "6"), ("120", "12")]
ADJ = [("20", "361.78"), ("40", "900.00"), ("60", "1500.00")]
EXTRAS = [
    [],
    ["mor=12"],
    ["b30=5662.57"],
    ["pre=20:6:12:150.00"],
    ["b30=5662.57", "pre=20:6:12:150.00"],
]
BASES = [["b365_360", "plusreg", "usa"], ["b365"]]
POINTS = [None, "pts=0.000833"]

ADJLATCH_RE = re.compile(r"ADJLATCH entries=(\d+) solves=(\d+) reuses=(\d+)")
INTEREST_RE = re.compile(r"^payment [\d.eE+-]+ interest ([\d.eE+-]+) ", re.M)
ORACLE_INT_RE = re.compile(r"^payment [\d.eE+-]+ interest ([\d.eE+-]+) ", re.M)


def screens(limit=0):
    out = []
    for amt, rate, (n, per), (adjn, adjamt), extra, basis, pts in itertools.product(
        AMOUNTS, RATES, TERMS, ADJ, EXTRAS, BASES, POINTS
    ):
        toks = [amt, rate, n, per] + basis + list(ANCHOR) + list(extra)
        toks.append("adj=%s::%s" % (adjn, adjamt))
        if pts:
            toks.append(pts)
        out.append((toks, bool(pts)))
        if limit and len(out) >= limit:
            return out
    return out


def run(binary, toks, env=None, timeout=60):
    """Return (stdout, stderr, timedout). R61/R64: a timeout is NOT a refusal and
    an empty run is NOT a zero — both are transported so the caller can refuse to
    fold them into a verdict."""
    e = dict(os.environ)
    if env:
        e.update(env)
    try:
        p = subprocess.run([binary] + toks, capture_output=True, text=True,
                           timeout=timeout, env=e)
    except subprocess.TimeoutExpired:
        return "", "", True
    return p.stdout, p.stderr, False


def answered(out):
    """A driver ANSWERED when it printed a payment/interest line. `ERR ...` and an
    EMPTY run are both NOT-answered, and are distinguished by the caller."""
    return INTEREST_RE.search(out) is not None


def interest_of(out):
    m = INTEREST_RE.search(out)
    return float(m.group(1)) if m else None


APR_RE = re.compile(r"^apr ([-\d.eE+]+)", re.M)


def apr_of(out):
    m = APR_RE.search(out)
    return float(m.group(1)) if m else None


def one(args, item):
    toks, has_pts = item
    orc_out, _, orc_to = run(args.oracle, toks)
    pri_out, _, pri_to = run(args.pristine, toks)
    pat_out, pat_err, pat_to = run(args.patched, toks, env={"GOAMORT_ADJLATCH": "1"})
    m = ADJLATCH_RE.search(pat_err or "")
    reuses = int(m.group(3)) if m else 0
    entries = int(m.group(1)) if m else 0
    # THE LOAD-BEARING COLUMN. `apr` is a separate goamort invocation because it
    # is an output MODE that returns early; the reuse branch lives in the APR
    # value walk, so this is the only channel through which the latched rate can
    # reach an output at all. Verified, not assumed: `GOAMORT_ADJLATCH=1 goamort
    # <screen> apr` prints `reuses=1`, so this column watches the SAME execution
    # REACH_reused counts.
    # ⚠️ Run on the points stratum ONLY — and the reason is NOT that `apr` mode
    # has nothing to report without points. It answers fine there (681 eligible
    # screens, measured). The reason is that REACH is 0 without points, so
    # nothing CAN move; an earlier draft of this comment gave the wrong reason
    # and would have told a future round `apr` mode was unavailable.
    priAPR = patAPR = None
    if has_pts:
        pa, _, pto = run(args.pristine, toks + ["apr"])
        qa, _, qto = run(args.patched, toks + ["apr"])
        if not pto:
            priAPR = apr_of(pa)
        if not qto:
            patAPR = apr_of(qa)
    return {
        "toks": toks,
        "points": has_pts,
        "dosAnswered": answered(orc_out) and not orc_to,
        "dosEmpty": (not orc_to) and orc_out.strip() == "",
        "priAnswered": answered(pri_out) and not pri_to,
        "patAnswered": answered(pat_out) and not pat_to,
        "priInt": interest_of(pri_out),
        "patInt": interest_of(pat_out),
        "priAPR": priAPR,
        "patAPR": patAPR,
        "latchEntries": entries,
        "reuses": reuses,
        "timedOut": bool(orc_to or pri_to or pat_to),
    }


def md5(path):
    h = hashlib.md5()
    with open(path, "rb") as f:
        for blk in iter(lambda: f.read(1 << 20), b""):
            h.update(blk)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--oracle", default="/tmp/oraclebuild/amort_oracle")
    ap.add_argument("--pristine", default="/tmp/goamort_pristine")
    ap.add_argument("--patched", default="/tmp/goamort_patched")
    ap.add_argument("--out", default="/tmp/r57_latch_sweep.json")
    ap.add_argument("--jobs", type=int, default=max(1, (os.cpu_count() or 2) - 1))
    ap.add_argument("--limit", type=int, default=0)
    a = ap.parse_args()

    for p in (a.oracle, a.pristine, a.patched):
        if not os.path.exists(p):
            sys.exit("missing binary: %s" % p)
    mp, mq = md5(a.pristine), md5(a.patched)
    if mp == mq:
        sys.exit("REFUSED: pristine and patched goamort are the SAME BINARY "
                 "(md5 %s) — every column below would be an arithmetic identity "
                 "and the sweep would be a guaranteed green (R78)." % mp)
    # The pristine binary must not know about the latch at all.
    _, perr, _ = run(a.pristine, ["100000", "0.08", "24", "6"],
                     env={"GOAMORT_ADJLATCH": "1"})
    if ADJLATCH_RE.search(perr or ""):
        sys.exit("REFUSED: the PRISTINE binary prints an ADJLATCH line, so it is "
                 "not pre-fix. The LOST/GAINED columns would be meaningless.")

    items = screens(a.limit)
    print("r57 LATCH REACH SWEEP  screens: %d  jobs: %d" % (len(items), a.jobs))
    print("  oracle   %s  %s" % (a.oracle, md5(a.oracle)))
    print("  pristine %s  %s" % (a.pristine, mp))
    print("  patched  %s  %s" % (a.patched, mq))
    recs = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=a.jobs) as ex:
        for r in ex.map(lambda it: one(a, it), items):
            recs.append(r)

    strata = {True: "WITH POINTS", False: "NO POINTS"}
    summary = {}
    fail = []
    for pts in (True, False):
        sub = [r for r in recs if r["points"] == pts]
        eligible = [r for r in sub if r["priAnswered"] or r["patAnswered"]]
        lost = [r for r in sub if r["priAnswered"] and not r["patAnswered"]]
        gained = [r for r in sub if r["patAnswered"] and not r["priAnswered"]]
        lost_dos = [r for r in lost if r["dosAnswered"]]
        gained_dos = [r for r in gained if r["dosAnswered"]]
        moved = [r for r in sub
                 if r["priInt"] is not None and r["patInt"] is not None
                 and abs(r["priInt"] - r["patInt"]) > 0.005]
        # THE APR COLUMN — the one the fix can actually move. Compared at the
        # precision goamort PRINTS (`apr %.6f`), which is this instrument's
        # resolution and is stated rather than implied.
        movedAPR = [r for r in sub
                    if r["priAPR"] is not None and r["patAPR"] is not None
                    and abs(r["priAPR"] - r["patAPR"]) > 5e-7]
        reached = [r for r in sub if r["reuses"] > 0]
        latched = [r for r in sub if r["latchEntries"] > 0]
        timed = [r for r in sub if r["timedOut"]]
        # R66 — the stratum where BOTH events can occur is the screens that
        # LATCHED AND ANSWERED, not every screen that answered. `eligible` is
        # kept for continuity and is NOT the denominator of any verdict.
        bothCanOccur = [r for r in sub if r["latchEntries"] > 0
                        and (r["priAnswered"] or r["patAnswered"])]
        aprComparable = [r for r in sub
                         if r["priAPR"] is not None and r["patAPR"] is not None]
        # R66/R78 — the denominator that belongs BESIDE MOVED_APR is the set where
        # the event can occur: screens that LATCHED and whose APR is comparable.
        # The whole-stratum count is 38% larger and includes screens the reuse
        # branch cannot bite. Both are printed; only this one is a denominator.
        aprComparableLatched = [r for r in aprComparable if r["latchEntries"] > 0]
        summary[strata[pts]] = {
            "screens": len(sub), "eligible_answered": len(eligible),
            "ELIGIBLE_latched_and_answered_R66": len(bothCanOccur),
            "latched": len(latched), "REACH_reused": len(reached),
            "lost": len(lost), "lostDosSolved": len(lost_dos),
            "gained": len(gained), "gainedDosSolved": len(gained_dos),
            "movedInterestOverHalfCent": len(moved),
            "aprComparable_wholeStratum": len(aprComparable),
            "APR_DENOMINATOR_latched_R66": len(aprComparableLatched),
            "MOVED_APR": len(movedAPR),
            "timedOut": len(timed),
        }
        print("\n--- %s ---" % strata[pts])
        for k, v in summary[strata[pts]].items():
            print("    %-22s %d" % (k, v))
        if lost_dos:
            fail.append("%s: %d screen(s) DOS SOLVES lost their answer (R67/R73)"
                        % (strata[pts], len(lost_dos)))
        if timed:
            fail.append("%s: %d timed out — a timeout is NOT a refusal (R61); "
                        "the verdict above is incomplete" % (strata[pts], len(timed)))
        # BOTH movement columns GATE. A computed-but-unused number is not a gate.
        if moved:
            fail.append("%s: %d screen(s) moved their INTEREST by more than half "
                        "a cent" % (strata[pts], len(moved)))
        if movedAPR:
            fail.append("%s: %d screen(s) moved their APR — this is the channel "
                        "the reuse branch acts through and the one this sweep "
                        "exists to watch" % (strata[pts], len(movedAPR)))

    # THE POSITIVE CONTROL. Without this the whole run is r57's original vacuous
    # sweep with more screens.
    wp = summary["WITH POINTS"]
    npx = summary["NO POINTS"]
    print("\n--- POSITIVE CONTROL (R51/R69/R76) ---")
    print("    the reuse branch was entered on %d of %d screens WITH points, "
          "and %d of %d WITHOUT" % (wp["REACH_reused"], wp["latched"],
                                    npx["REACH_reused"], npx["latched"]))
    if wp["REACH_reused"] == 0:
        fail.append("VACUOUS: the reuse branch was NEVER entered on the points "
                    "stratum, so this sweep has no power over the fix at all "
                    "(R69/R76) — exactly the defect it was written to replace.")
    if npx["REACH_reused"] != 0:
        fail.append("the reuse branch was entered WITHOUT points (%d screens); "
                    "the stratification claim in this file's header is wrong and "
                    "must be re-derived before the null above is quoted."
                    % npx["REACH_reused"])

    art = {"summary": summary, "argv": sys.argv, "screens": len(items),
           "binaries": {"oracle": {"path": a.oracle, "md5": md5(a.oracle)},
                        "pristine": {"path": a.pristine, "md5": mp},
                        "patched": {"path": a.patched, "md5": mq}},
           "records": recs}
    with open(a.out, "w") as f:
        json.dump(art, f, indent=1, sort_keys=True)
    print("\n  artefact -> %s" % a.out)

    if fail:
        print("\nFAIL:")
        for m in fail:
            print("  - " + m)
        return 1
    # 🚨 R82, THIRD LINK IN THE SAME CHAIN. Pass 1 killed a sweep with no reach;
    # pass 2 killed a replacement that measured a channel the reach could not
    # touch; pass 3's mutation found that THIS gate was a ZERO-check, not a
    # COVERAGE-check. A defect that silently drops APR extraction on exactly the
    # screens that LATCHED leaves a residue of non-latched screens, a positive
    # `aprComparable`, MOVED_APR 0, and a PASS — with the R66 eligible count
    # printed three lines above, contradicting it and gating nothing. Measured:
    # that mutant survived the zero-check at exit 0 and dies here.
    if wp["APR_DENOMINATOR_latched_R66"] < wp["ELIGIBLE_latched_and_answered_R66"]:
        print("\n⚠️ the APR column covers only %d of the %d screens that LATCHED "
              "and answered — the null below does not speak for the stratum the "
              "fix acts on (R66/R69)."
              % (wp["APR_DENOMINATOR_latched_R66"],
                 wp["ELIGIBLE_latched_and_answered_R66"]))
        return 1
    print("\nPASS (R73): no screen DOS solves lost its answer, no INTEREST and "
          "no APR moved, and the reuse branch was demonstrably reached on the "
          "stratum the fix acts on.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
