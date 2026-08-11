#!/usr/bin/env python3
"""r53_segment_bound_sweep.py — THE SECOND GENERATOR (R73).

WHY THIS EXISTS. Round 52 measured a fix to `solveSegmentRate`'s horizon bound
that took the standing arm from 25 HARD in 2,091 to 12, with `paired_regression`
booking `NEW = 0` over all 2,211 cases and both engines. It then found — with an
ad-hoc sweep that was NOT committed — that the same fix turned 21 screens DOS
SOLVES into screens the port REFUSES. Rule 4's `NEW = 0` was green and
INSUFFICIENT, because the standing arm scores HARD-vs-not on screens BOTH
engines answered: a screen the port stops answering leaves the population
silently. That is R73, and this file is the instrument it demands.

  🚨 R73 — A FIX IN A SOLVER FAMILY MUST BE RUN THROUGH A SECOND GENERATOR
  BEFORE IT SHIPS, AND THE SECOND GENERATOR MUST BE ABLE TO SEE A REFUSAL.

WHAT IT DOES. A dense factorial grid of SMALL amount-only-adjustment (AO6)
screens — the exact route family the fix touches — each run through:

  * the DOS oracle (`amort_oracle`, the only authority),
  * a PRISTINE `goamort` built from the tree under test,
  * a PATCHED `goamort`,

and bucketed by what happened to the ANSWER, not by how close a number is:

  KEPT_*     both builds answer
  LOST       pristine answers, patched REFUSES   <- the R67 bucket, the point
  GAINED     pristine refuses, patched answers
  BOTH_REFUSE

`LOST` is split by whether DOS itself solves the row (`adjdump` prints
`ratestatus 1`). `LOST` + DOS-solves is a regression against DOS regardless of
how wrong the pristine answer was, and it is the bucket that refused the r52
patch.

⚠️ SIZE THIS INSTRUMENT'S AUTHORITY HONESTLY (CAUTION 2/3/8). Its screens are
EXTREME: adjustment amounts several times the regular payment, deeply negative
solved rates, dozens of prepay extras on a one-to-four-year loan. It is a
REFUTATION instrument. It is NOT a product rate and must never be quoted as
one. Its job is to fail a fix, not to bless one.

⚠️ AND IT COMPARES DEFAULT STDOUT ONLY (rule 7). `adjdump` is verified additive
by `--selftest`: the oracle's `payment/interest/paid` lines are byte-identical
with and without it, so one oracle run yields both the solved-rate status and
the totals.

USAGE
  r53_segment_bound_sweep.py --pristine /tmp/goamort --patched /tmp/goamort_patched \
      --oracle /tmp/oraclebuild/amort_oracle --out /tmp/r53_sweep.json [--jobs 2]
  r53_segment_bound_sweep.py --selftest --oracle ...      # the additivity control

EXIT CODES
  0  no screen lost an answer DOS solves
  1  at least one did  -> the fix under test does not ship (R73)
  2  harness error (rule 12: the harness is a suspect before the engine is)
"""

import argparse
import json
import os
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor

# ---------------------------------------------------------------- the grid
# Factorial over the axes the fix's mechanism actually rides on:
#   perYr and n            fix the loan's payment grid
#   ADJ_PERIOD             sites the amount-only adjustment (the AO6 route)
#   PRE_START/COUNT/FREQ   put a prepay series under the sub-walk, which is what
#                          offsets the SEGMENT grid from the LOAN grid — the
#                          alignment §66's short bound loses a row to
#   ADJ_AMOUNT             sets how hard the implied-rate secant has to work
#   RATE                   two starting rates, because the secant SEEDS at the
#                          rate the loan carries (AMORTOP.pas:1523)
# The loan/first-date pair is a MONTH-END anchor with a six-month offset, which
# is the shape round 52's row-level demonstration (seed 50107 case 165) had.
PER_YR = [1, 2, 4, 6, 12, 26]
N = [12, 24, 48]
ADJ_PERIOD = [2, 6, 12, 18]
PRE_START = [2, 4]
PRE_COUNT = [6, 40, 90]
PRE_FREQ = [12, 26]
ADJ_AMOUNT = [150.00, 900.00, 4000.00, 12000.00]
RATE = ["0.08", "0.1352850000"]
AMOUNT = "100000.00"
LOAN_DMY = "loandmy=28.2.2023"
FIRST_DMY = "firstdmy=28.8.2023"

# 🚨 --no-dates EXISTS BECAUSE OF r53's OWN AUDIT (finding F1). The round ran a
# third arm without the date tokens and reported it beside two committed arms —
# from an UNCOMMITTED monkey-patched copy of this file. That is exactly the
# rule-6 debt §92 condemns round 52 for. The anchor is now a FLAG, so every arm
# the record quotes can be re-derived from this file.
# ⚠️ AND THE ANCHOR MATTERS: the month-end loan date with a six-month offset is
# what makes the SEGMENT grid fall between the LOAN grid's rows, which is the
# alignment the fix's do-while overshoot row is about. An arm without it is a
# WEAKER probe of this mechanism, not an equivalent one.
DATE_TOKENS = [LOAN_DMY, FIRST_DMY]
# Immutable copies: DATE_TOKENS is emptied in place by --no-dates.
LOAN_DMY_C, FIRST_DMY_C = LOAN_DMY, FIRST_DMY


def random_screens(n, seed):
    """A RANDOMIZED arm over the same axes, with the ranges opened up.

    🚨 WHY BOTH. The factorial grid above is a LATTICE, and a defect can live
    between lattice points — that is CAUTION 8 applied to a generator's SHAPE
    rather than its size. A zero from the grid alone is a statement about the
    lattice. This arm draws continuously over wider ranges so the two arms fail
    independently, and both are reported separately: pooling them would hide
    which one had the power.
    """
    import random
    rng = random.Random(seed)
    for _ in range(n):
        py = rng.choice([1, 2, 3, 4, 6, 12, 24, 26, 52])
        yield [
            "%.2f" % rng.uniform(20000, 500000),
            "%.10f" % rng.uniform(0.02, 0.18),
            str(rng.randint(8, 96)),
            str(py),
            *DATE_TOKENS,
            "pre=%d:%d:%d:%.2f" % (rng.randint(1, 24), rng.randint(1, 120),
                                   rng.choice([1, 2, 4, 6, 12, 24, 26, 52]),
                                   rng.uniform(20, 3000)),
            "adj=%d::%.2f" % (rng.randint(1, 40), rng.uniform(50, 30000)),
        ]


def screens():
    for rate in RATE:
        for py in PER_YR:
            for n in N:
                for ap in ADJ_PERIOD:
                    for ps in PRE_START:
                        for pc in PRE_COUNT:
                            for pf in PRE_FREQ:
                                for aa in ADJ_AMOUNT:
                                    yield [
                                        AMOUNT, rate, str(n), str(py),
                                        *DATE_TOKENS,
                                        "pre=%d:%d:%d:150.00" % (ps, pc, pf),
                                        "adj=%d::%.2f" % (ap, aa),
                                    ]


# ------------------------------------------------------------- running one
# 🚨 R61/R64 — A TIMEOUT IS NOT A REFUSAL AND A RUN THAT PRODUCED NO OUTPUT IS
# NOT A ZERO. Every runner below carries a timeout and reports `timeout` and
# `crash` as their OWN buckets, never folded into `refused`. The whole point of
# this instrument is telling a refusal apart from an answer; a harness that
# reads a hang as a refusal would manufacture the very finding it exists to
# check. `run_amz.js:47` is the sibling that gets this wrong (the R56 class).
TIMEOUT_S = 25


def run(binary, args, extra_env=None):
    env = dict(os.environ)
    if extra_env:
        env.update(extra_env)
    try:
        p = subprocess.run([binary] + args, capture_output=True, text=True,
                           timeout=TIMEOUT_S, env=env)
    except subprocess.TimeoutExpired:
        return {"state": "timeout"}
    if p.returncode != 0:
        return {"state": "crash", "rc": p.returncode,
                "stderr": p.stderr[-400:]}
    out = p.stdout
    if not out.strip():
        return {"state": "empty"}
    rec = {"state": "answered", "stdout": out, "stderr": p.stderr}
    for line in out.splitlines():
        if line.startswith("ERR "):
            rec["state"] = "refused"
            rec["err"] = line[4:].strip()
        if line.startswith("payment "):
            t = line.split()
            try:
                rec["interest"] = float(t[t.index("interest") + 1])
                rec["paid"] = float(t[t.index("paid") + 1])
            except (ValueError, IndexError):
                return {"state": "unparsed", "stdout": out[:400]}
        if line.startswith("adjrow "):
            t = line.split()
            try:
                rec.setdefault("adjrows", []).append(
                    {"rate": float(t[t.index("rate") + 1]),
                     "ratestatus": int(t[t.index("ratestatus") + 1])})
            except (ValueError, IndexError):
                return {"state": "unparsed", "stdout": out[:400]}
    if rec["state"] == "answered" and "interest" not in rec:
        # No totals line and no ERR line: neither an answer nor a refusal.
        return {"state": "unparsed", "stdout": out[:400]}
    return rec


def dos_solved(rec):
    """DOS printed a rate IT SOLVED for the adjustment row.

    `ratestatus 1` is DOS *displaying a rate it solved* — the same emission rule
    `dos_fuzzer5_test.go:2814` keys `adj_rate_differs` on. R72: read the
    emission rule before you count. A row DOS did not solve is NOT evidence
    about a port refusal, so it is excluded from the LOST verdict rather than
    counted against the port.
    """
    return any(r["ratestatus"] == 1 for r in rec.get("adjrows", []))


def one(args, oracle, pristine, patched):
    o = run(oracle, args + ["adjdump"])
    a = run(pristine, args)
    b = run(patched, args)
    rec = {"cmd": " ".join(args), "dos": o["state"],
           "pristine": a["state"], "patched": b["state"],
           "dosSolved": dos_solved(o) if o["state"] == "answered" else False}
    if o["state"] == "answered":
        rec["dosInterest"] = o.get("interest")
        rec["dosRates"] = [r["rate"] for r in o.get("adjrows", [])
                           if r["ratestatus"] == 1]
    for k, r in (("pristineInterest", a), ("patchedInterest", b)):
        if r["state"] == "answered":
            rec[k] = r.get("interest")
    if b["state"] == "refused":
        rec["patchedErr"] = b.get("err", "")[:120]
    if a["state"] == "refused":
        rec["pristineErr"] = a.get("err", "")[:120]
    return rec


def bucket(r):
    pa, pb = r["pristine"], r["patched"]
    if pa in ("timeout", "crash", "empty", "unparsed") or \
       pb in ("timeout", "crash", "empty", "unparsed") or \
       r["dos"] != "answered":
        return "EXCLUDED"
    if pa == "answered" and pb == "refused":
        return "LOST"
    if pa == "refused" and pb == "answered":
        return "GAINED"
    if pa == "refused" and pb == "refused":
        return "BOTH_REFUSE"
    if r.get("pristineInterest") is None or r.get("patchedInterest") is None or \
       r.get("dosInterest") is None:
        return "EXCLUDED"
    if abs(r["pristineInterest"] - r["patchedInterest"]) < 5e-3:
        return "KEPT_UNMOVED"
    da = abs(r["pristineInterest"] - r["dosInterest"])
    db = abs(r["patchedInterest"] - r["dosInterest"])
    if db < da - 5e-3:
        return "KEPT_CLOSER"
    if db > da + 5e-3:
        return "KEPT_FARTHER"
    return "KEPT_MOVED_EQUIDISTANT"


def selftest(oracle):
    """`adjdump` must be purely ADDITIVE, or one oracle run cannot serve both
    questions. A CONTROL, not a formality: if it ever stops holding, every
    totals comparison in this file is against a different stdout."""
    args = [AMOUNT, "0.08", "48", "12", LOAN_DMY, FIRST_DMY,
            "pre=2:6:12:150.00", "adj=12::900.00"]
    with_d = run(oracle, args + ["adjdump"])
    without = run(oracle, args)
    if with_d["state"] != "answered" or without["state"] != "answered":
        print("SELFTEST FAIL: oracle did not answer the control screen")
        return 2
    ok = (with_d.get("interest") == without.get("interest") and
          with_d.get("paid") == without.get("paid"))
    print("SELFTEST adjdump additive: %s (interest %s vs %s)" %
          (ok, with_d.get("interest"), without.get("interest")))
    if not with_d.get("adjrows"):
        print("SELFTEST FAIL: adjdump printed no adjrow — the rate parser has "
              "no input and every dosSolved would be False BY CONSTRUCTION (R69)")
        return 2
    return 0 if ok else 2


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--oracle", default="/tmp/oraclebuild/amort_oracle")
    ap.add_argument("--pristine", default="/tmp/goamort")
    ap.add_argument("--patched", default="/tmp/goamort_patched")
    ap.add_argument("--out", default="/tmp/r53_sweep.json")
    ap.add_argument("--jobs", type=int, default=max(1, (os.cpu_count() or 2)))
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--selftest", action="store_true")
    ap.add_argument("--no-dates", action="store_true", dest="no_dates",
                    help="drop the loandmy=/firstdmy= anchor (audit F1: this arm "
                         "must be producible from the committed file)")
    ap.add_argument("--random", type=int, default=0,
                    help="run N randomized screens instead of the factorial grid")
    ap.add_argument("--seed", type=int, default=53053)
    a = ap.parse_args()

    for p in (a.oracle, a.pristine, a.patched):
        if a.selftest and p != a.oracle:
            continue
        if not os.path.exists(p):
            print("MISSING BINARY: %s" % p)
            return 2
    if a.selftest:
        return selftest(a.oracle)
    if selftest(a.oracle) != 0:
        return 2

    if a.no_dates:
        del DATE_TOKENS[:]
    grid = list(random_screens(a.random, a.seed)) if a.random else list(screens())
    print("ARM: %s%s" % ("randomized n=%d seed=%d" % (a.random, a.seed)
                         if a.random else "factorial grid",
                         "  [NO DATE ANCHOR]" if a.no_dates else
                         "  [anchor %s %s]" % (LOAN_DMY, FIRST_DMY)))
    if a.limit:
        grid = grid[:a.limit]
    print("screens: %d   jobs: %d" % (len(grid), a.jobs))
    with ThreadPoolExecutor(max_workers=a.jobs) as ex:
        recs = list(ex.map(lambda g: one(g, a.oracle, a.pristine, a.patched),
                           grid))
    for r in recs:
        r["bucket"] = bucket(r)

    counts = {}
    for r in recs:
        counts[r["bucket"]] = counts.get(r["bucket"], 0) + 1
    lost = [r for r in recs if r["bucket"] == "LOST"]
    lost_dos = [r for r in lost if r["dosSolved"]]
    gained = [r for r in recs if r["bucket"] == "GAINED"]
    gained_dos = [r for r in gained if r["dosSolved"]]

    # 🚨 PROVENANCE. An instrument committed expressly to discharge rule 6 must
    # say WHICH BUILDS it compared, or the artefact cannot be tied to a tree and
    # the debt is only half paid (r53 audit pass 2, finding N9). md5 of each
    # binary, plus the exact argv, so any later reader can re-derive the run.
    def _md5(path):
        try:
            import hashlib
            return hashlib.md5(open(path, "rb").read()).hexdigest()
        except OSError:
            return None
    json.dump({"arm": ("randomized n=%d seed=%d" % (a.random, a.seed)
                       if a.random else "factorial grid"),
               "dateAnchor": None if a.no_dates else [LOAN_DMY_C, FIRST_DMY_C],
               "argv": sys.argv,
               "binaries": {"oracle": {"path": a.oracle, "md5": _md5(a.oracle)},
                            "pristine": {"path": a.pristine, "md5": _md5(a.pristine)},
                            "patched": {"path": a.patched, "md5": _md5(a.patched)}},
               "screens": len(recs), "counts": counts,
               "lostDosSolved": len(lost_dos), "gainedDosSolved": len(gained_dos),
               "records": recs}, open(a.out, "w"), indent=1)

    print("=" * 62)
    for k in sorted(counts):
        print("  %-24s %5d" % (k, counts[k]))
    print("=" * 62)
    print("  LOST an answer                    %5d" % len(lost))
    print("  -- of those, DOS SOLVES the row   %5d   <- R67/R73 verdict" %
          len(lost_dos))
    print("  GAINED an answer                  %5d" % len(gained))
    print("  -- of those, DOS SOLVES the row   %5d" % len(gained_dos))
    print("  artefact -> %s" % a.out)
    # 🚨 NO SILENT CAPS: say what was excluded and why.
    exc = counts.get("EXCLUDED", 0)
    if exc:
        st = {}
        for r in recs:
            if r["bucket"] == "EXCLUDED":
                k = "dos=%s pristine=%s patched=%s" % (r["dos"], r["pristine"],
                                                       r["patched"])
                st[k] = st.get(k, 0) + 1
        print("  EXCLUDED breakdown (not folded into any verdict):")
        for k in sorted(st, key=lambda x: -st[x]):
            print("      %-44s %5d" % (k, st[k]))
    if lost_dos:
        print("\nFAIL (R73): %d screens DOS solves lost their answer. Examples:"
              % len(lost_dos))
        for r in lost_dos[:5]:
            print("   %s" % r["cmd"])
            print("      DOS rate(s) %s | patched: %s" %
                  (r.get("dosRates"), r.get("patchedErr", "")[:90]))
        return 1
    print("\nPASS (R73): no screen DOS solves lost its answer.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
