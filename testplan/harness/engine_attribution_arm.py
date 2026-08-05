#!/usr/bin/env python3
"""Attribute fuzzer5 cases to the ENGINE that answers them, and cross that with
whether the case DIVERGED — the round-33 contingency table.

WHY (round 33).  Round 32 rebased the stacked in-scope rate to 475 HARD in
35,000 and attributed all of it to ONE signal class (`HARD:divergent_class`).
That is a class of SYMPTOM, not of cause.  Running the row-level localiser
(localise_divergent_row.py) over a 20-seed repro harvest put the FIRST divergent
cell on an interest row just after a re-amortization in every scored case, which
pointed at the faithful port's Re_Amortize -- and then the port's own reAmortize
tracer printed NOTHING, because not one of those cases ever reached the faithful
port at all.  There are TWO engines (START_HERE §5) and `dosPortCanHandle` picks
between them; the divergent corpus is 100% piecewise.

100% IS NOT A FINDING WITHOUT ITS BASE RATE (rule 9).  Most of the fuzzer's
stacked population is piecewise too.  What this script produces is therefore the
CONTINGENCY TABLE, not the co-occurrence:

    engine/reason | cases | diverged | rate

so the conditional divergence rate can be read per engine and per routing
clause, and an enrichment can be told apart from a majority.

METHOD
  1. `FZ5CASEDUMP=1` dumps every generated case for a seed (the same generator
     the arms use), giving the DENOMINATOR.
  2. `PERSENSE_FUZZ_FLAKEDUMP=1` on the same seed emits the divergent cases as
     `SIG=HARD:...` lines, giving the NUMERATOR -- matched back to the corpus by
     the case's argument string, so the two are provably the same population.
  3. `DPTRACEENGINE=1 goamort ... rows` prints `GENGINE <engine> [reason=...]`,
     which is `dosPortRoute`'s own return value (R13: the instrument prints what
     it READ, not a reimplementation of the eleven clauses).

NOTE #24 IS A HOLE IN BOTH COLUMNS, AND IT IS DECLARED.  goamort implements
neither `norate` nor `noamt`, so those cases cannot be routed by this method.
They are EXCLUDED FROM BOTH the numerator and the denominator and the excluded
count is printed -- an exclusion that landed only in the denominator would
deflate every rate on the table.

THE REASON IS THE FIRST REJECTING CLAUSE, NOT THE ONLY ONE.  dosPortRoute
short-circuits, so a case rejected by `in_advance_or_r78_or_daily` may satisfy
three later clauses as well.  A per-reason rate is therefore an upper bound on
that clause's exclusive contribution, and the table says so.

⚠️  SUPERSEDED BY testplan/harness/engine_coexclusion_arm.py (round 34).
This script is kept because round 33's published table came out of it and a
re-run is the only way to reproduce that table exactly.  Do NOT use it for new
measurement: it re-runs `goamort` once per case (~35 min per 20 seeds), it
inherits note #24's 32%-of-the-corpus hole, and it can only ever report the
FIRST rejecting clause, which R29 says is not a work queue.  The replacement
reads the same route out of an ordinary arm run, keyed by case index, with the
full clause SET and no goamort at all.

⚠️  ONE REAL DEFECT AND ONE FALSE ALARM, both raised at the round-33 review:
  1. FALSE ALARM. The review said this script took the wrong GENGINE line. It
     did not: engine.go prints at the TOP of Amortize, so the OUTERMOST call
     prints FIRST and every later line is a nested re-entry that never answered
     the screen. hits[0] was right. Round 34 "fixed" it to hits[-1] and then
     measured the damage that would do (390 of 36,426 compared cases moved to
     the faithful port, a 23% inflation of its denominator) before reverting.
     The script now reports how many screens emit more than one line, so the
     question is visible rather than argued.
  2. REAL. The denominator was a LIST and the numerator a SET, so an identical
     case drawn in two seeds entered the two columns a different number of
     times. Both are now over DISTINCT cases.

USAGE
    python3 testplan/harness/engine_attribution_arm.py \
        --bin /tmp/amorttest --go /tmp/goamort \
        --seeds 50100-50119 --n 400 [--out /tmp/engattr]
"""
import argparse
import collections
import os
import re
import subprocess
import sys

CASE = re.compile(r"^FZ5CASE\s+\d+\s+amort_oracle\s+(.*?)(?:\s+bdump)?\s*$")
SIG = re.compile(r"SIG=(HARD:\S+).*?(amort_oracle\s+.*?)(?:\s+bdump)?\s*$")
GENG = re.compile(r"^GENGINE\s+(\S+)(?:\s+reason=(\S+))?")


def parse_seeds(spec):
    out = []
    for part in spec.split(","):
        if "-" in part:
            a, b = part.split("-")
            out.extend(range(int(a), int(b) + 1))
        else:
            out.append(int(part))
    return out


def run_seed(binary, seed, n, dumpvar):
    env = dict(os.environ)
    env.update({"PERSENSE_FUZZ": "1", "PERSENSE_REQUIRE_ORACLE": "1",
                "PERSENSE_FUZZ_SEED": str(seed), "PERSENSE_FUZZ_N": str(n),
                dumpvar: "1"})
    p = subprocess.run([binary, "-test.run",
                        "TestDOSFuzzer5AllAdvancedOptions", "-test.v"],
                       capture_output=True, text=True, env=env, timeout=3600)
    return p.stdout + p.stderr


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--bin", default="/tmp/amorttest")
    ap.add_argument("--go", default="/tmp/goamort")
    ap.add_argument("--seeds", default="50100-50119")
    ap.add_argument("--n", type=int, default=400)
    ap.add_argument("--out", default="/tmp/engattr")
    a = ap.parse_args()

    os.makedirs(a.out, exist_ok=True)
    seeds = parse_seeds(a.seeds)

    corpus, diverged = [], set()
    for s in seeds:
        cpath = os.path.join(a.out, f"case_{s}.txt")
        fpath = os.path.join(a.out, f"flake_{s}.txt")
        if not os.path.exists(cpath):
            open(cpath, "w").write(run_seed(a.bin, s, a.n, "FZ5CASEDUMP"))
        if not os.path.exists(fpath):
            open(fpath, "w").write(run_seed(a.bin, s, a.n,
                                            "PERSENSE_FUZZ_FLAKEDUMP"))
        for line in open(cpath):
            m = CASE.match(line.strip())
            if m:
                corpus.append(m.group(1).strip())
        for line in open(fpath):
            m = SIG.search(line)
            if m:
                diverged.add(m.group(2).replace("amort_oracle", "", 1).strip())
        print(f"seed {s}: corpus={len(corpus)} diverged={len(diverged)}",
              file=sys.stderr)

    # note #24 exclusion, applied to BOTH columns.
    def note24(c):
        t = c.split()
        return "norate" in t or "noamt" in t

    # DEFECT 2 (round-34 fix): the denominator was a LIST and the numerator a
    # SET, so a case drawn identically in two seeds contributed 2 to `cases` and
    # 1 to `diverged`. Both columns are now over DISTINCT cases — the same unit
    # the numerator has always used. The duplicate count is printed rather than
    # silently absorbed.
    dupes = len(corpus) - len(set(corpus))
    corpus = sorted(set(corpus))
    if dupes:
        print(f"deduplicated {dupes} corpus entries drawn identically in more "
              f"than one seed (defect 2)")

    excl_corpus = [c for c in corpus if note24(c)]
    excl_div = {c for c in diverged if note24(c)}
    corpus = [c for c in corpus if not note24(c)]
    diverged = {c for c in diverged if not note24(c)}

    print(f"corpus={len(corpus) + len(excl_corpus)} "
          f"EXCLUDED_note24={len(excl_corpus)} routed={len(corpus)}")
    print(f"diverged={len(diverged) + len(excl_div)} "
          f"EXCLUDED_note24={len(excl_div)} matched={len(diverged)}")

    # divergent cases must be a SUBSET of the corpus, or the two populations are
    # not the same population and no conditional rate off them means anything.
    cset = set(corpus)
    orphans = diverged - cset
    if orphans:
        print(f"⚠️  {len(orphans)} divergent cases NOT found in the corpus dump "
              f"— the two runs are not the same population; rates below are "
              f"UNSOUND. First: {sorted(orphans)[0][:120]}")

    tally = collections.defaultdict(lambda: [0, 0])
    unroutable = 0
    multi_gengine = 0
    multi_disagree = 0
    for i, c in enumerate(corpus):
        p = subprocess.run([a.go] + c.split() + ["rows"], capture_output=True,
                           text=True, env={**os.environ, "DPTRACEENGINE": "1"},
                           timeout=300)
        # DEFECT 1 — AND THE ROUND-33 REVIEW GOT ITS DIRECTION WRONG.
        #
        # The review note said a screen's FIRST GENGINE line could be a pre-solve
        # and that the LAST was the table build, and round 34 first "fixed" this
        # to hits[-1]. That is backwards for `goamort`. engine.go prints the line
        # at the TOP of Amortize, immediately before the routing branch, so the
        # OUTERMOST call prints FIRST; every later line comes from a NESTED
        # re-entry (solveFancyTermFromPayment, SolveBalloonAmount,
        # SolvePrepaymentAmount/Duration) on a clone, which never answers the
        # screen. `goamort` runs no solver pass ahead of the table build — it
        # cannot, since it implements neither `norate` nor `noamt` (note #24) —
        # so here the first line is unambiguously the right one.
        #
        # Round 33's original hits[0] was therefore CORRECT and the "defect" was
        # a mis-reading of the control flow. What is kept from it is the COUNT:
        # a screen with more than one line is a screen where the choice matters.
        hits = [GENG.match(line.strip()) for line in p.stderr.splitlines()]
        hits = [h for h in hits if h]
        if not hits:
            unroutable += 1
            continue
        if len(hits) > 1:
            multi_gengine += 1
            if hits[0].group(0) != hits[-1].group(0):
                multi_disagree += 1
        m = hits[0]
        key = m.group(1) if not m.group(2) else f"piecewise:{m.group(2)}"
        tally[key][0] += 1
        if c in diverged:
            tally[key][1] += 1
        if (i + 1) % 250 == 0:
            print(f"  routed {i + 1}/{len(corpus)}", file=sys.stderr)

    print(f"UNROUTABLE (goamort emitted no GENGINE) = {unroutable}")
    print(f"cases with >1 GENGINE line = {multi_gengine}, of which the FIRST and "
          f"LAST line disagree = {multi_disagree} (defect 1: round 33 read the "
          f"FIRST)\n")
    print(f"{'engine / first rejecting clause':46s} {'cases':>7s} "
          f"{'diverged':>9s} {'rate':>9s}")
    tot = [0, 0]
    for k in sorted(tally, key=lambda x: -tally[x][1]):
        n, d = tally[k]
        tot[0] += n
        tot[1] += d
        r = f"1 in {n / d:.0f}" if d else "0"
        print(f"{k:46s} {n:7d} {d:9d} {r:>9s}")
    r = f"1 in {tot[0] / tot[1]:.0f}" if tot[1] else "0"
    print(f"{'TOTAL':46s} {tot[0]:7d} {tot[1]:9d} {r:>9s}")


if __name__ == "__main__":
    main()
