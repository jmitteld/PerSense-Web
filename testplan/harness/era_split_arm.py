#!/usr/bin/env python3
"""era_split_arm.py — pool fuzzer5's ERA SPLIT across the seeds of one or more arms.

WHY THIS EXISTS
---------------
`analyze_arm.py` reports the POOLED HARD rate over ACTUALLY COMPARED. That number
mixes the two eras: DOS's own date arithmetic degrades past the client's 2099
boundary (client decision 2026-08-03: comparison beyond 2099 is NOT REQUIRED), so
a pooled rate is dominated by cases nobody is claiming fidelity on. The headline
claim is stated PER IN-SCOPE CASE (START_HERE §3a decision 3), and only the era
split produces that denominator.

`dos_fuzzer5_test.go` already emits the split, once per seed, as

    era split (cases, horizon keyed on the port's own resolved dates):
      in-scope<=2099 compared=N hard=M | out-of-scope>2099 compared=N hard=M

This script sums those four counters over every seed log in an arm directory.
It does NOT recompute the era from the draw tokens. That is the whole point:

  R2 / standing rule "A HARNESS-COMPUTED DATE WILL MANUFACTURE A FRONTIER" —
  the fuzzer keys the bucket on `fz5MaxYear(gr)`, the PORT'S OWN resolved
  horizon (the engine's answer, not a second derivation of it), which is also
  what sidesteps §55: DOS's `daterec` year is a BYTE based at 1900 and WRAPS,
  so a split keyed on DOS's REPORTED year is wrong — that is exactly the defect
  round 22 found in the era label.

R12 / R13 — an instrument may print only what it has READ, and a skip is not a
pass. A seed whose log carries NO era-split line is counted and NAMED, never
silently dropped: a killed binary reports nothing, and nothing looks like success
(round 19). If any seed is missing the line, the pooled rate is reported as
PARTIAL and the missing seeds are listed.

Usage:
    python3 testplan/harness/era_split_arm.py /tmp/r26/arm/50100 [more dirs...]
"""
import glob
import os
import re
import sys

ERA = re.compile(
    r"era split \(cases, horizon keyed on the port's own resolved dates\): "
    r"in-scope<=2099 compared=(\d+) hard=(\d+) \| "
    r"out-of-scope>2099 compared=(\d+) hard=(\d+)")

# A seed that reached a ledger but produced no era line is a DIFFERENT failure
# from a seed that was killed before either. Distinguish them (R16: a terminal
# bucket must say what it asked).
LEDGER = re.compile(r"ledger: generated (\d+) = ")


def scan(d):
    t = dict(dir=d, seeds=0, with_era=0, with_ledger=0,
             ic=0, ic_hard=0, oc=0, oc_hard=0, missing=[])
    for f in sorted(glob.glob(os.path.join(d, "seed_*.log"))):
        txt = open(f, errors="replace").read()
        t["seeds"] += 1
        if LEDGER.search(txt):
            t["with_ledger"] += 1
        m = ERA.search(txt)
        if not m:
            t["missing"].append(os.path.basename(f))
            continue
        t["with_era"] += 1
        ic, ich, oc, och = (int(x) for x in m.groups())
        t["ic"] += ic
        t["ic_hard"] += ich
        t["oc"] += oc
        t["oc_hard"] += och
    return t


def rate(num, den):
    if den == 0:
        return "n/a (no denominator)"
    if num == 0:
        return f"0 in {den:,} — no events; quote a 95% UPPER BOUND, not a rate"
    return f"{num} in {den:,} = 1 in {den/num:,.0f} = {100*(1-num/den):.4f}%"


def report(t):
    print(f"\n===== {t['dir']} =====")
    print(f"  seeds {t['seeds']}   reached a ledger {t['with_ledger']}   "
          f"emitted an era split {t['with_era']}")
    if t["missing"]:
        print(f"  ⚠ {len(t['missing'])} seed(s) with NO era line — "
              f"PARTIAL, not a clean measurement: {', '.join(t['missing'])}")
    print(f"  IN SCOPE  <=2099   compared {t['ic']:,}   HARD {t['ic_hard']}   "
          f"{rate(t['ic_hard'], t['ic'])}")
    print(f"  OUT SCOPE  >2099   compared {t['oc']:,}   HARD {t['oc_hard']}   "
          f"{rate(t['oc_hard'], t['oc'])}")
    print(f"  pooled             compared {t['ic']+t['oc']:,}   "
          f"HARD {t['ic_hard']+t['oc_hard']}   "
          f"{rate(t['ic_hard']+t['oc_hard'], t['ic']+t['oc'])}")


if __name__ == "__main__":
    dirs = sys.argv[1:]
    if not dirs:
        print(__doc__)
        sys.exit(2)
    tot = dict(dir="POOLED ACROSS ARMS", seeds=0, with_era=0, with_ledger=0,
               ic=0, ic_hard=0, oc=0, oc_hard=0, missing=[])
    for d in dirs:
        t = scan(d)
        report(t)
        for k in ("seeds", "with_era", "with_ledger", "ic", "ic_hard", "oc", "oc_hard"):
            tot[k] += t[k]
        tot["missing"].extend(f"{os.path.basename(d)}/{m}" for m in t["missing"])
    if len(dirs) > 1:
        report(tot)
    print("\nUNITS (CAUTION 1): every figure above counts CASES, not signal "
          "instances and not rows. `analyze_arm.py`'s HARD count is SIGNAL "
          "INSTANCES over the same population and will not agree with these.")
    if tot["missing"]:
        print("VERDICT: PARTIAL — at least one seed contributed no era line.")
        sys.exit(1)
    print("VERDICT: COMPLETE — every seed contributed an era line.")
