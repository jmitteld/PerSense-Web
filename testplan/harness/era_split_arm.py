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

# 🚨 ROUND 51 — THIS REGEX WAS BROKEN FROM ROUND 48 UNTIL ROUND 51 AND THE
# SCRIPT SILENTLY MEASURED NOTHING FOR THREE ROUNDS.
#
# Round 48 added the scope key and changed the emission at
# dos_fuzzer5_test.go:3001 from
#     "era split (cases, horizon keyed on the port's own resolved dates): ..."
# to
#     "era split (cases, SCOPE KEY=reached, keyed on the port's own resolved dates): ..."
# The old pattern matched neither, so `with_era` was 0 on every seed, every
# counter stayed 0, and the tool printed "compared 0 HARD 0 n/a".
#
# ✅ IT FAILED CLOSED, WHICH IS WHY THIS NEVER BECAME A WRONG PUBLISHED NUMBER:
# `missing` was populated, the verdict printed PARTIAL and the exit status was 1
# (VERIFIED r51 — `python3 era_split_arm.py <dir>; echo $?` → 1). The defect was
# that no round RAN it after r48, so the standing 25-in-2,091 baseline was
# hand-summed from the per-seed logs for three rounds instead. R12's design held;
# the operator did not. → the fix is the regex, and the lesson is R37.
#
# The key is CAPTURED, not discarded, because rule 9 forbids quoting an in-scope
# rate without its scope key (R34) — and because a pooled run over two arms
# scored under DIFFERENT keys would otherwise silently sum into one figure.
# The pre-r48 form is still accepted so archived arm directories remain readable;
# those logs are keyed on what is now called `horizon`, and are NORMALISED to that
# name — see LOGFORM below for why the name and the log format must not be conflated.
#
# 🚨 r51 SECOND AUDIT — three holes in the FIRST version of this fix:
#   H3 a directory with no `seed_*.log` (empty, mistyped, or the `.out` of a
#      FZ5CASEDUMP run) scored zero seeds and still printed "VERDICT: COMPLETE",
#      exit 0 — and pooled with a good arm it produced a clean figure from an arm
#      that contributed nothing. That is verbatim the R19/R12 failure this
#      module's docstring says it exists to prevent, and the first fix's own
#      comment claimed to have closed it. Fixed below in scan()/__main__.
#   H5 labelling pre-r48 logs `horizon(pre-r48 log)` made two arms of the SAME
#      population (`horizon`) collide as "MIXED KEYS" and be refused. The key and
#      the log format are different facts; they are now tracked separately.
#   H6 `ERA.search` took the FIRST match only, so a log carrying two era lines
#      under two different keys silently discarded the second — defeating the
#      very control the first fix added. Now `finditer`, and >1 era line in one
#      seed is itself a PARTIAL.
ERA = re.compile(
    r"era split \(cases, (?:SCOPE KEY=(?P<key>\w+), keyed|(?P<old>horizon) keyed) "
    r"on the port's own resolved dates\): "
    r"in-scope<=2099 compared=(\d+) hard=(\d+) \| "
    r"out-of-scope>2099 compared=(\d+) hard=(\d+)")

# The three keys the fuzzer will accept (zzr48_scopekey_test.go:62). A key outside
# this set means the log was hand-edited or the test changed under us; either way
# the figure is not quotable, so it is named rather than printed as authoritative.
VALID_KEYS = {"reached", "horizon", "lastdate"}

# A seed that reached a ledger but produced no era line is a DIFFERENT failure
# from a seed that was killed before either. Distinguish them (R16: a terminal
# bucket must say what it asked).
LEDGER = re.compile(r"ledger: generated (\d+) = ")


def scan(d):
    t = dict(dir=d, seeds=0, with_era=0, with_ledger=0,
             ic=0, ic_hard=0, oc=0, oc_hard=0, missing=[], keys=set(),
             oldform=0, badkeys=set(), multi=[], nodir=not os.path.isdir(d))
    for f in sorted(glob.glob(os.path.join(d, "seed_*.log"))):
        txt = open(f, errors="replace").read()
        t["seeds"] += 1
        if LEDGER.search(txt):
            t["with_ledger"] += 1
        # H6: EVERY era line, not just the first. A seed that emitted two is not
        # a seed that emitted one — and if they carry different keys, silently
        # keeping the first is the exact failure the key control exists to stop.
        ms = list(ERA.finditer(txt))
        if not ms:
            t["missing"].append(os.path.basename(f))
            continue
        if len(ms) > 1:
            t["multi"].append("%s(%d era lines)" % (os.path.basename(f), len(ms)))
        t["with_era"] += 1
        for m in ms:
            # H5: a pre-r48 log IS keyed on `horizon`; it just does not say so.
            # The KEY is normalised to `horizon` so two arms of the same
            # population do not collide as "mixed"; the LOG FORM is counted
            # separately, because "this arm was read from archived logs" is a
            # provenance fact worth printing and is not a population fact.
            if m.group("old"):
                t["oldform"] += 1
                key = "horizon"
            else:
                key = m.group("key")
                if key not in VALID_KEYS:
                    t["badkeys"].add(key)
            t["keys"].add(key)
        m = ms[0]
        ic, ich, oc, och = (int(x) for x in m.groups()[2:])
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
    # H3: zero seeds is the loudest thing this tool can be asked to report, and
    # the first fix printed COMPLETE. A directory that does not exist, or holds
    # no `seed_*.log` (e.g. the `.out` files of a FZ5CASEDUMP run), contributed
    # NOTHING — and nothing must not look like success (R19/R12).
    if t.get("nodir"):
        print(f"  🚨 NOT A DIRECTORY — contributed nothing.")
    elif t["seeds"] == 0:
        print(f"  🚨 ZERO seed_*.log FILES — contributed nothing. "
              f"(A FZ5CASEDUMP arm writes seed_*.out; this tool reads seed_*.log.)")
    # rule 9 / R34: an in-scope rate may not be quoted without its scope key.
    keys = sorted(t.get("keys") or [])
    if len(keys) > 1:
        print(f"  🚨 SCOPE KEYS MIXED: {', '.join(keys)} — THESE ARE DIFFERENT "
              f"POPULATIONS AND THE POOLED FIGURE BELOW IS NOT QUOTABLE (R34/R57).")
    elif keys:
        print(f"  SCOPE KEY {keys[0]}")
    if t.get("badkeys"):
        print(f"  🚨 UNRECOGNISED SCOPE KEY(S): {', '.join(sorted(t['badkeys']))} — "
              f"not one of {'|'.join(sorted(VALID_KEYS))}. NOT QUOTABLE.")
    if t.get("multi"):
        print(f"  🚨 SEED(S) WITH MORE THAN ONE ERA LINE: {', '.join(t['multi'])} — "
              f"only the first was summed. NOT QUOTABLE.")
    if t.get("oldform"):
        print(f"  provenance: {t['oldform']} pre-r48-format era line(s), "
              f"keyed on `horizon` implicitly.")
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
               ic=0, ic_hard=0, oc=0, oc_hard=0, missing=[], keys=set(),
               oldform=0, badkeys=set(), multi=[], empty=[])
    for d in dirs:
        t = scan(d)
        report(t)
        for k in ("seeds", "with_era", "with_ledger", "ic", "ic_hard", "oc",
                  "oc_hard", "oldform"):
            tot[k] += t[k]
        tot["missing"].extend(f"{os.path.basename(d)}/{m}" for m in t["missing"])
        tot["keys"] |= t["keys"]
        tot["badkeys"] |= t["badkeys"]
        tot["multi"].extend(f"{os.path.basename(d)}/{m}" for m in t["multi"])
        if t["nodir"] or t["seeds"] == 0:
            tot["empty"].append(d)
    if len(dirs) > 1:
        report(tot)
    print("\nUNITS (CAUTION 1): every figure above counts CASES, not signal "
          "instances and not rows. `analyze_arm.py`'s HARD count is SIGNAL "
          "INSTANCES over the same population and will not agree with these.")
    # H3: an arm that contributed nothing is the strongest PARTIAL there is.
    if tot["empty"]:
        print(f"VERDICT: PARTIAL — {len(tot['empty'])} directory/ies contributed "
              f"NO seed logs at all: {', '.join(tot['empty'])}")
        sys.exit(1)
    if tot["missing"]:
        print("VERDICT: PARTIAL — at least one seed contributed no era line.")
        sys.exit(1)
    if tot["multi"]:
        print(f"VERDICT: NOT QUOTABLE — seed(s) emitted more than one era line "
              f"({', '.join(tot['multi'])}); only the first was summed.")
        sys.exit(1)
    if tot["badkeys"]:
        print(f"VERDICT: NOT QUOTABLE — unrecognised scope key(s): "
              f"{', '.join(sorted(tot['badkeys']))}.")
        sys.exit(1)
    # 🚨 r51: a banner the operator can scroll past is not a control. A run that
    # pooled two scope keys printed "NOT QUOTABLE" and still exited 0, so a CI
    # gate would have passed it — the same shape as the r48-r51 regex break,
    # where the only thing that saved the project was a non-zero exit. Fail closed.
    if len(tot["keys"]) > 1:
        print(f"VERDICT: NOT QUOTABLE — scope keys mixed across arms "
              f"({', '.join(sorted(tot['keys']))}). Re-run each key separately.")
        sys.exit(1)
    print("VERDICT: COMPLETE — every seed contributed an era line.")
