#!/usr/bin/env python3
"""engine_coexclusion_arm.py — the ENGINE x DIVERGED contingency table AND the
CO-EXCLUSION PROFILE, from ordinary fuzzer5 arm logs.

WHY THIS REPLACES engine_attribution_arm.py (round 34)
------------------------------------------------------
Round 33's attribution ran `goamort` once per case over a FZ5CASEDUMP corpus and
matched numerator to denominator BY ARGUMENT STRING. Three structural holes:

  * `goamort` implements neither `norate` nor `noamt` (note #24). That removed
    2,554 of 7,863 corpus cases (32%) and 17 of 75 divergences.
  * 589 further cases emitted no route line at all.
  * an identical case drawn in two seeds was counted twice.

and it cost ~35 minutes per 20 seeds. The fuzzer already runs both engines and
the oracle on every case; round 34 made it say so. One ordinary arm run at

    FZ5CASEDUMP=1 PERSENSE_FUZZ_FLAKEDUMP=1 DPTRACEENGINE=1

now emits, per case index:

    FZ5ENGBEGIN <c>                                      (the compared Amortize)
    GENGINE  <engine> [reason=<first> reasons=<a,b,c>]   (engine.go, per Amortize)
    FZ5ENGEND   <c>
    FZ5CASE     <c> amort_oracle ...                     (the draw)
    FZ5OUTCOME  <c> bucket=<...> goOK=<...>              (the terminal bucket)
    FZ5VERDICT  <c> hard=<...> era=<...> compared=<...>  (every case past the
                                                          terminal buckets)

Same process, same draw, no re-execution, no argument matching, no blind spot.

⚠️  `cases generated` below is the number of cases that reached the FZ5CASE dump,
which is NOT every case the generator drew: the `skipped-plain` bucket continues
~230 lines earlier (fuzzer5 skips plain loans by design — CAUTION 2), so at
FUZZ_N=400 the count is ~394 per seed, not 400. The shortfall is reported.

WHICH GENGINE LINE IS THE CASE'S ENGINE — AND WHY NEITHER "FIRST" NOR "LAST" IS
------------------------------------------------------------------------------
engine.go prints one GENGINE line per `Amortize` invocation, at the top, just
before the routing branch. A single screen produces several:

  * BEFORE the compared call, the backward modes run SolveBlankCellsPrepared and
    every trial evaluation prints a line. So the FIRST line of a `norate`/`noamt`
    case belongs to the SOLVER.
  * INSIDE the compared call, solveFancyTermFromPayment / SolveBalloonAmount /
    SolvePrepaymentAmount / SolvePrepaymentDuration re-enter Amortize on a clone.
    So the LAST line of a `noterm` case belongs to a NESTED PROBE.

Round 33 took the first. This script's first draft took the last, on the strength
of a round-33 review note that had the nesting backwards; over 160 seeds that
moved 390 compared cases from piecewise to dosport — a 23% inflation of the
faithful port's denominator, in the direction that flatters the port — and erased
an entire clause row. Both were guesses about control flow.

The fuzzer now BRACKETS the compared call (`FZ5ENGBEGIN <c>` / `FZ5ENGEND <c>`),
so the engine is the FIRST GENGINE line strictly inside the bracket: the
outermost invocation of the call whose answer was compared. No guessing.

CROSS-SEED DOUBLE COUNTING (round 33's second defect) is gone by construction:
the key is (seed, case index).

R29 — WHY THE CO-EXCLUSION PROFILE IS THE POINT
-----------------------------------------------
⚠️  "FREED" IS NOT "FIXED". The counterfactual below counts a divergent case as
freed by a clause set when EVERY clause excluding it is in that set — i.e. the
case would reach the faithful port. Whether the faithful port then gets it RIGHT
is a separate question that only a probe port can answer. Round 33's four-clause
narrow probe frees 233 of 475 by this measure (49%) and measured a 33% cut when
actually built. Treat `frees` as an UPPER BOUND on the improvement.

`dosPortRoutes` short-circuited into a single reason is not a work queue. Round
33 ranked clauses by first-match count and planned to widen the top one; a probe
port with R78 removed and nothing else scored 33 HARD over seeds 50100-50107,
identical seed for seed to the shipped 33, because those screens are excluded by
other clauses too. This script therefore computes, over every SUBSET of the
observed clause universe, how many divergent cases that subset would actually
free — and how many non-divergent cases it would move with them.

USAGE
    engine_coexclusion_arm.py <armdir> [<armdir> ...]
    engine_coexclusion_arm.py --era 0 <armdir>        # in-scope only (headline)

A seed log is counted ONLY if it carries the fuzzer's `ledger:` line, which is
printed after the last case. A run still in progress has a partial log whose
counts read as a clean zero (round 33 scored exactly that as a perfect result).
"""

import os
import re
import sys
from collections import Counter, defaultdict
from itertools import combinations

GENG = re.compile(r"^GENGINE\s+(\S+)(?:\s+reason=(\S+))?(?:\s+reasons=(\S+))?")
BEGIN = re.compile(r"^FZ5ENGBEGIN\s+(\d+)")
END = re.compile(r"^FZ5ENGEND\s+(\d+)")
CASE = re.compile(r"^FZ5CASE\s+(\d+)\s+(.*)$")
OUTC = re.compile(r"^FZ5OUTCOME\s+(\d+)\s+bucket=(\S+)\s+goOK=(\S+)")
VERD = re.compile(r"^FZ5VERDICT\s+(\d+)\s+hard=(\S+)\s+era=(\d+)\s+compared=(\S+)")
LEDGER = re.compile(r"ledger:\s+generated")


class Case:
    __slots__ = ("seed", "idx", "cmd", "engine", "reasons", "first_reason",
                 "nested_reasons", "nested_engine", "ngengine", "bucket",
                 "hard", "era", "compared")

    def __init__(self, seed, idx):
        self.seed, self.idx = seed, idx
        self.cmd = ""
        self.engine = None          # "dosport" | "piecewise" | None (router never reached)
        self.reasons = ()
        self.first_reason = None
        self.nested_reasons = ()
        self.nested_engine = None
        self.ngengine = 0
        self.bucket = None
        self.hard = False
        self.era = None
        self.compared = False


def parse_seed_log(path, seed):
    """Return (cases, finished). `finished` is False for a partial log.

    The engine is read from the bracket: the FIRST GENGINE line strictly between
    FZ5ENGBEGIN <c> and FZ5ENGEND <c>. Lines outside a bracket belong to the
    backward solver's trial evaluations; later lines inside it belong to nested
    probes. Both are real Amortize calls and neither answered the compared screen.
    """
    cases = {}
    inside = None         # case index whose compared Amortize is executing
    pending = []          # GENGINE lines seen inside the current bracket
    orphan_gengine = 0    # lines outside any bracket (solver trials) — expected
    finished = False
    with open(path, "r", errors="replace") as fh:
        for line in fh:
            line = line.rstrip("\n")
            if line.startswith("GENGINE"):
                m = GENG.match(line)
                if m:
                    if inside is None:
                        orphan_gengine += 1
                    else:
                        pending.append(m)
                continue
            if line.startswith("FZ5ENGBEGIN"):
                m = BEGIN.match(line)
                if m:
                    inside = int(m.group(1))
                    pending = []
                continue
            if line.startswith("FZ5ENGEND"):
                m = END.match(line)
                if m and inside is not None:
                    idx = int(m.group(1))
                    c = cases.setdefault(idx, Case(seed, idx))
                    c.ngengine = len(pending)
                    if pending:
                        first = pending[0]      # the OUTERMOST call
                        c.engine = first.group(1)
                        c.first_reason = first.group(2)
                        c.reasons = (tuple(first.group(3).split(","))
                                     if first.group(3) else ())
                        # the LAST line, kept ONLY to size the error that taking
                        # it would cause — it is a nested probe, not the answer
                        last = pending[-1]
                        c.nested_reasons = (tuple(last.group(3).split(","))
                                            if last.group(3) else ())
                        c.nested_engine = last.group(1)
                    inside, pending = None, []
                continue
            if line.startswith("FZ5CASE"):
                m = CASE.match(line)
                if m:
                    idx = int(m.group(1))
                    cases.setdefault(idx, Case(seed, idx)).cmd = m.group(2)
                inside, pending = None, []
                continue
            if line.startswith("FZ5OUTCOME"):
                m = OUTC.match(line)
                if m:
                    c = cases.setdefault(int(m.group(1)), Case(seed, int(m.group(1))))
                    c.bucket = m.group(2)
                continue
            if line.startswith("FZ5VERDICT"):
                m = VERD.match(line)
                if m:
                    c = cases.setdefault(int(m.group(1)), Case(seed, int(m.group(1))))
                    c.hard = m.group(2) == "true"
                    c.era = int(m.group(3))
                    c.compared = m.group(4) == "true"
                continue
            if not finished and LEDGER.search(line):
                finished = True
    return cases, finished, orphan_gengine


def main():
    args = [a for a in sys.argv[1:]]
    era_filter = None
    if "--era" in args:
        i = args.index("--era")
        if i + 1 >= len(args):
            print("--era needs a value (0 = in scope <=2099, 1 = out of scope)")
            sys.exit(2)
        era_filter = int(args[i + 1])
        del args[i:i + 2]
    if not args:
        print(__doc__)
        sys.exit(2)

    all_cases = []
    seeds_done, seeds_partial, orphans = 0, [], 0
    for d in args:
        for fn in sorted(os.listdir(d)):
            if not (fn.startswith("seed_") and fn.endswith(".log")):
                continue
            seed = fn[len("seed_"):-len(".log")]
            cases, finished, orph = parse_seed_log(os.path.join(d, fn), seed)
            if not finished:
                seeds_partial.append(f"{d}/{fn}")
                continue
            seeds_done += 1
            orphans += orph
            all_cases.extend(cases.values())

    print(f"seeds counted (ledger line present) = {seeds_done}")
    if seeds_partial:
        print(f"⚠️  seeds SKIPPED as unfinished = {len(seeds_partial)}")
        for p in seeds_partial:
            print(f"     {p}")
    print(f"cases reaching the FZ5CASE dump     = {len(all_cases)}")
    print(f"  (`skipped-plain` cases continue earlier and never reach it — the")
    print(f"   generator drew more than this; see the fuzzer's own ledger line)")

    # ---- instrument health -------------------------------------------------
    multi = [c for c in all_cases if c.ngengine > 1]
    # How wrong would "take the LAST line in the bracket" have been? That is a
    # NESTED probe (solveFancyTermFromPayment etc.), not the compared answer.
    nested_diff = [c for c in multi if c.nested_reasons != c.reasons]
    nested_flip = [c for c in multi
                   if c.nested_engine == "dosport" and c.engine != "dosport"]
    noroute = [c for c in all_cases if c.engine is None]
    print()
    print("--- instrument health ---")
    print(f"GENGINE lines OUTSIDE any bracket   = {orphans}")
    print("     (backward-solver trial evaluations — real Amortize calls that did")
    print("      not answer any compared screen; correctly attributed to nobody)")
    print(f"cases with >1 GENGINE in the bracket = {len(multi)}"
          f" ({pct(len(multi), len(all_cases))})")
    print(f"  ... where the NESTED probe's clause set differs = {len(nested_diff)}")
    print(f"  ... where taking the LAST line would have moved the case TO dosport"
          f" = {len(nested_flip)}")
    print("      <- the size of the error this script's first draft would have made,")
    print("         and it runs in the direction that FLATTERS the port (rule 12)")
    print(f"cases with NO GENGINE line          = {len(noroute)}"
          f" ({pct(len(noroute), len(all_cases))})")
    print("     (the port returned before reaching the router — a failed backward")
    print("      solve or an early error; no engine ever answered, so these are")
    print("      correctly in NEITHER column)")
    nb = Counter(c.bucket for c in noroute)
    for b, n in nb.most_common():
        print(f"       bucket={b}: {n}")

    # ---- the population ----------------------------------------------------
    pop = [c for c in all_cases if c.compared and c.engine is not None]
    if era_filter is not None:
        pop = [c for c in pop if c.era == era_filter]
    label = "ALL ERAS" if era_filter is None else (
        "IN SCOPE (<=2099)" if era_filter == 0 else "OUT OF SCOPE (>2099)")
    print()
    print(f"=== POPULATION: COMPARED cases, {label} ===")
    print(f"compared cases = {len(pop)}   HARD = {sum(1 for c in pop if c.hard)}")
    if not pop:
        return
    _h = sum(1 for c in pop if c.hard)
    print(f"rate = 1 in {len(pop) / _h:.0f}" if _h else
          "rate = NO EVENTS — quote a bound, not a rate (CAUTION 4)")

    # ---- round-33-comparable table: engine x FIRST reason ------------------
    print()
    print("--- ENGINE x DIVERGED, keyed on the FIRST rejecting clause "
          "(round-33-comparable) ---")
    print(f"{'engine / first rejecting clause':<48}{'cases':>7}{'diverged':>10}{'rate':>12}")
    byfirst = defaultdict(lambda: [0, 0])
    for c in pop:
        k = "dosport" if c.engine == "dosport" else f"piecewise:{c.first_reason}"
        byfirst[k][0] += 1
        byfirst[k][1] += 1 if c.hard else 0
    for k, (n, d) in sorted(byfirst.items(), key=lambda kv: -kv[1][1]):
        print(f"{k:<48}{n:>7}{d:>10}{rate(n, d):>12}")
    tn = sum(v[0] for v in byfirst.values())
    td = sum(v[1] for v in byfirst.values())
    print(f"{'TOTAL':<48}{tn:>7}{td:>10}{rate(tn, td):>12}")
    print("⚠️  R29: these per-clause counts are NOT additive and NOT a work queue.")

    # ---- the co-exclusion profile -----------------------------------------
    div = [c for c in pop if c.hard]
    print()
    print(f"--- CO-EXCLUSION PROFILE: the FULL clause set of each of the "
          f"{len(div)} divergent cases ---")
    # Canonicalise: dosPortRouteSet emits the three BALLOON clauses in
    # balloon-index order, not clause order, so the same SET can arrive in more
    # than one order and an ordered key would split one row into several.
    prof = Counter(tuple(sorted(c.reasons)) for c in div)
    print(f"{'clauses excluding the case (full set)':<78}{'cases':>6}")
    for rs, n in prof.most_common():
        print(f"{('+'.join(rs) if rs else '(none — dosport answered it)'):<78}{n:>6}")
    sizes = Counter(len(c.reasons) for c in div)
    print()
    print("divergent cases by NUMBER of excluding clauses: " +
          "  ".join(f"{k}:{v}" for k, v in sorted(sizes.items())))
    print("(round 33 planned its remedy as if every one of these were 1)")

    # ---- the counterfactual: what does removing a SET actually free? -------
    universe = sorted({r for c in pop for r in c.reasons})
    div_sets = [frozenset(c.reasons) for c in div]
    pop_sets = [frozenset(c.reasons) for c in pop if c.reasons]
    print()
    print("--- WHAT A CLAUSE SET WOULD ACTUALLY FREE (EXHAUSTIVE over all "
          f"2^{len(universe)} = {2 ** len(universe)} subsets) ---")
    print("A case reaches the faithful port only when EVERY clause excluding it is")
    print("removed. `frees` counts divergent compared cases — an UPPER BOUND on the")
    print("improvement, since a freed case still has to be answered CORRECTLY by the")
    print("faithful port. `moves` counts ALL compared cases the set would re-route:")
    print("the risk side, and the amount of oracle validation the change would owe.")
    print(f"{'clause set removed':<74}{'frees':>7}{'moves':>7}")
    best_by_k = defaultdict(list)
    for k in range(1, len(universe) + 1):
        for combo in combinations(universe, k):
            s = frozenset(combo)
            frees = sum(1 for r in div_sets if r <= s)
            if frees == 0:
                continue
            moves = sum(1 for r in pop_sets if r <= s)
            best_by_k[k].append((frees, moves, combo))
    for k in sorted(best_by_k):
        rows = sorted(best_by_k[k], key=lambda t: (-t[0], t[1]))[:3]
        for frees, moves, combo in rows:
            print(f"{'+'.join(combo):<74}{frees:>7}{moves:>7}")
        print()
    print(f"total divergent compared cases = {len(div)};  "
          f"removing EVERY clause would free all of them by construction.")
    # The efficiency ranking is what a remedy plan actually wants: divergences
    # freed per compared case re-routed (per case of validation work owed).
    print()
    print("--- BEST RATIO: divergences freed per 100 cases re-routed ---")
    ranked = []
    for k in sorted(best_by_k):
        for frees, moves, combo in best_by_k[k]:
            if moves:
                ranked.append((100.0 * frees / moves, frees, moves, combo))
    ranked.sort(key=lambda t: (-t[0], -t[1]))
    print(f"{'clause set removed':<74}{'per100':>7}{'frees':>7}{'moves':>7}")
    for eff, frees, moves, combo in ranked[:8]:
        print(f"{'+'.join(combo):<74}{eff:>7.1f}{frees:>7}{moves:>7}")


def pct(a, b):
    return f"{100.0 * a / b:.1f}%" if b else "n/a"


def rate(n, d):
    return f"1 in {n / d:.0f}" if d else "none"


if __name__ == "__main__":
    main()
