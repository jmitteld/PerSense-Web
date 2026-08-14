#!/usr/bin/env python3
"""r56_case_decomposition.py — decompose the in-scope HARD CASES of a fuzzer5 arm,
PER CASE, on a join that respects the fuzzer's own ledger.

WHY THIS EXISTS (round 56)
--------------------------
The standing per-case rate comes from `era_split_arm.py` (12 HARD in scope). The
per-class breakdown comes from `analyze_arm.py`, which counts SIGNAL INSTANCES
over a different unit. Neither answers "what are the twelve cases, individually".
Round 51's decomposition did, but it is four rounds and two engine fixes old and
its top family is closed on one arm and refuted on the other.

THE JOIN, AND WHY THE OBVIOUS ONE IS WRONG (R54)
------------------------------------------------
Grouping `SIG=` lines by repro command DOES NOT PARTITION THE HARD CASE SET, for
TWO independent reasons — the project had recorded only the first:

  (1) LEDGER. `go_solved_dos_date_horizon` (:2276) and `go_solved_dos_refused`
      (:2369) are emitted BEFORE `caseHard := false` (:2389) and `continue`
      before `checked++`, so those cases carry NO FZ5VERDICT line at all.
  (2) EMISSION. The `apr_differs` site (:2912) appends ` apr` to the command, so
      that class's SIG lines do not string-match their own case's dump line.
      Measured: 41 of 51 SIG lines join by EXACT string, 10 need only ` apr`
      stripped, 0 unjoined, 0 ambiguous.
      An early draft claimed non/norate/noterm/lastdmy= were also SIG-only.
      FALSE (pass 1 F4) — the canonical dump line carries them.
      AND PASS 2 RECORDED THE HONEST CAVEAT: the PRE-F4 token set gives the SAME
      answer on this arm. F4 removed a latent collision risk; it did not correct
      a wrong number, and must not be credited as a correction.

The join is the fuzzer's own case identity, transported by `FZ5CASEDUMP=1`:

    FZ5CASE    <idx> <canonical repro command>
    FZ5OUTCOME <idx> bucket= goOK=
    FZ5VERDICT <idx> hard= era= compared= eng= route=

`(seed, idx)` is the case key.

FZ5VERDICT is emitted once per **checked** case, NOT per **compared** case
(pass 1 F5): the `!goOK` branch at :2432 emits `compared=false`. So the in-scope
denominator the project calls 2,091 is honestly 2,089. Both are printed.

THE TARGET IS A COUNT, AND THE BAR IS A RATE
--------------------------------------------
The exit criterion's bar is **1 in 400**. START_HERE 3a.12 adds a POWER
REQUIREMENT: the 95% one-sided upper bound, not the point estimate, must clear
it. That converts the rate bar into a COUNT bar, and the conversion depends on n:

    n must satisfy  UPPER95(k)/n < 1/400,  i.e.  n > 400 * UPPER95(k)

so at k=1 the arm needs **n >= 1898 in-scope compared cases** (Poisson
convention, caution 4). This arm has 2,089 — **only ~9% headroom**. PASS 3
SHOWED THAT LOSING ONE SEED LOG OF TEN TAKES n TO 1,889 AND FLIPS THE EXIT
CRITERION FROM PASS TO FAIL, while the instrument printed COMPLETE. The
denominator is now validated three ways, and the floor is asserted.

`--max-hard N` sets the count target. **Default 1.** An earlier version hardcoded
5 — the RETIRED point-estimate target — and its "minimal sufficient set"
answered a question the project no longer asks.

WHAT THIS INSTRUMENT DOES NOT ESTABLISH (pass 1 F3, sharpened by passes 2 and 3)
-------------------------------------------------------------------------------
The check against the era line is a COUNT identity, not round 51's SET identity,
and cannot be one: `eraHard[caseEra]++` (:2920) and the FZ5VERDICT printf (:2958)
read the SAME `caseHard`/`caseEra` in the same iteration. It has power against
TRANSPORT LOSS and DUPLICATION only — never against the fuzzer computing the
flag wrongly.
NO CONTENT FIELD IS VALIDATED except as listed below. Documented surviving
mutants, each exiting 0: swapping `era=` on two verdict lines; changing `eng=`;
renaming one SIG class on a case that keeps other signals — that last one
destroys the UNIQUENESS of the minimal set, so THE HEADLINE RESTS ON UNVALIDATED
LABELS. `route=` is equally unvalidated.
An early draft said "the 12/3 counts are trustworthy" and "closing this needs the
fuzzer to transport case INDICES". BOTH WERE WRONG: the `hard=` half closes with
no fuzzer change, because `SIG=HARD:` lines are emitted at the sites that set
`caseHard = true`. That check is performed (CONTRADICTION). Only `era=` still
needs indices.

AUDIT HISTORY. Pass 1: 7 findings. Pass 2: 9, five severe — including that pass
1's fix for a fail-open was itself a fail-open. Pass 3: refuted two of pass 2's
six remediations — the F1 gate was fail-open a THIRD time (global counters
cancelling a loss on one seed against a duplication on another) and the honest
denominator was introduced with zero validation — and found one remediation
VACUOUS (bucket-balance was provably the same predicate as CONTRADICTION).
All fixed here. EVERY STRUCTURAL GUARD IS NOW PER SEED.
"""
import argparse
import collections
import itertools
import json
import os
import re
import sys

# Only apr_differs appends a token to the SIG format string (:2912).
_DROP_EXACT = {"apr"}

# 95% one-sided Poisson upper bounds (caution 4's convention). Index = observed k.
UPPER95 = [2.9957, 4.7439, 6.2958, 7.7537, 9.1535, 10.5130, 11.8424, 13.1481,
           14.4346, 15.7052, 16.9622, 18.2075, 19.4425]

SIG_RE = re.compile(r"SIG=(HARD|ADVISORY):(\w+)\s+(.*)$")
# :3316 is t.Logf("divergent option classes: %d of %d compared cases", totalDiverge, checked)
# — group(1) is totalDiverge = sum(cs.diverge): divergent CASES, not classes. The
# emitted LABEL is wrong at the source (filed for r57); the number is the one we want.
DIVCENSUS_RE = re.compile(r"divergent option classes:\s*(\d+) of (\d+) compared cases")
# :3043 — "=> ACTUALLY COMPARED %d — use this as the denominator for any rate".
# 🚨 PASS 4: this file printed an "honest denominator" for three commits without
# ever reading the line the fuzzer emits telling it what that denominator is.
ACTCOMP_RE = re.compile(r"=> ACTUALLY COMPARED (\d+)")
ERA_RE = re.compile(
    r"era split \(cases, SCOPE KEY=(\w+),[^)]*\):\s*"
    r"in-scope<=2099 compared=(\d+) hard=(\d+)\s*\|\s*"
    r"out-of-scope>2099 compared=(\d+) hard=(\d+)"
)


def normalise(cmd):
    return " ".join(t for t in cmd.split() if t not in _DROP_EXACT)


def parse_seed(path, seed, cases, sigs, problems):
    """Parse one seed log. Returns PER-SEED stats — pass 3 finding 1: global
    counters let a loss on one seed cancel a duplication on another."""
    st = {"era_lines": 0, "census_lines": 0, "scope_key": None, "divcases": 0,
          "in_compared": 0, "in_hard": 0, "out_compared": 0, "out_hard": 0,
          "sig_div_instances": 0, "actcomp_lines": 0, "actually_compared": 0,
          "v_era0": 0, "v_era1": 0, "v_compared": 0}
    with open(path, "r", errors="replace") as fh:
        for line in fh:
            s = line.rstrip("\n").strip()
            if s.startswith("FZ5CASE "):
                parts = s.split(" ", 2)
                idx, cmd = int(parts[1]), parts[2]
                rec = cases[(seed, idx)]
                rec["cmd"] = cmd
                rec["norm"] = normalise(cmd)
            elif s.startswith("FZ5OUTCOME "):
                parts = s.split()
                rec = cases[(seed, int(parts[1]))]
                for kv in parts[2:]:
                    k, _, v = kv.partition("=")
                    rec[k] = v
            elif s.startswith("FZ5VERDICT "):
                parts = s.split()
                idx = int(parts[1])
                rec = cases[(seed, idx)]
                if rec.get("has_verdict"):
                    problems.append("seed %s case %s: DUPLICATE FZ5VERDICT line"
                                    % (seed, idx))
                for kv in parts[2:]:
                    k, _, v = kv.partition("=")
                    rec[k] = v
                rec["has_verdict"] = True
                # PASS 4 F1/F2: count the fields that DEFINE n, PER SEED. The
                # earlier "three denominator validations" constrained the CHECKED
                # total, the phantom set and the seed set — never `compared=` or
                # `era=` themselves, so a one-token edit moved n with every guard
                # green. And the one surviving GLOBAL guard cancelled a loss on
                # one seed against an invention on another.
                if rec.get("era") == "0":
                    st["v_era0"] += 1
                elif rec.get("era") == "1":
                    st["v_era1"] += 1
                if rec.get("compared") == "true":
                    st["v_compared"] += 1
            else:
                m = SIG_RE.search(s)
                if m:
                    sev, cls, cmd = m.group(1), m.group(2), m.group(3)
                    if cls == "divergent_class":
                        st["sig_div_instances"] += 1
                    sigs.append({"seed": seed, "sev": sev, "class": cls,
                                 "cmd": cmd, "norm": normalise(cmd)})
                    continue
                m = ERA_RE.search(s)
                if m:
                    st["era_lines"] += 1
                    st["scope_key"] = m.group(1)
                    st["in_compared"] += int(m.group(2))
                    st["in_hard"] += int(m.group(3))
                    st["out_compared"] += int(m.group(4))
                    st["out_hard"] += int(m.group(5))
                    continue
                m = DIVCENSUS_RE.search(s)
                if m:
                    st["census_lines"] += 1
                    st["divcases"] += int(m.group(1))
                    continue
                m = ACTCOMP_RE.search(s)
                if m:
                    st["actcomp_lines"] += 1
                    st["actually_compared"] += int(m.group(1))
    return st


def main():
    ap = argparse.ArgumentParser(description="r56 per-case decomposition")
    ap.add_argument("armdir")
    ap.add_argument("outjson", nargs="?")
    ap.add_argument("--max-hard", type=int, default=1,
                    help="in-scope HARD case target. Default 1 (the LIVE bar).")
    ap.add_argument("--seeds", default=None,
                    help="LO-HI. If given, REFUSE any missing seed log "
                         "(pass 3 finding 2: a lost seed silently shrinks n).")
    args = ap.parse_args()
    if args.max_hard < 0:
        ap.error("--max-hard must be >= 0")
    # PASS 4 F3: `if floor_n:` silently skipped the power assertion for a target
    # beyond the table, so RAISING the target — a strictly weaker claim — turned
    # FAILED into COMPLETE. Fail closed instead.
    if args.max_hard >= len(UPPER95):
        ap.error("--max-hard must be < %d (no tabulated 95%% bound above that)"
                 % len(UPPER95))
    if args.seeds is None:
        ap.error("--seeds=LO-HI is REQUIRED: it is the only witness that no seed "
                 "log is missing, and a lost seed silently shrinks the denominator")
    m = re.fullmatch(r"\s*(\d+)\s*-\s*(\d+)\s*", args.seeds)
    if not m or int(m.group(2)) < int(m.group(1)):
        ap.error("--seeds must be LO-HI with LO <= HI (got %r)" % args.seeds)
    armdir, outjson, max_hard = args.armdir, args.outjson, args.max_hard

    cases = collections.defaultdict(dict)
    sigs, problems = [], []
    per_seed = {}

    logs = sorted(f for f in os.listdir(armdir)
                  if f.startswith("seed_") and f.endswith(".log"))
    if not logs:
        print("NO SEED LOGS in %s" % armdir)
        sys.exit(2)
    for f in logs:
        seed = int(f[len("seed_"):-len(".log")])
        per_seed[seed] = parse_seed(os.path.join(armdir, f), seed, cases, sigs, problems)

    fail = False
    # PASS 4 F4: nine of thirteen failure modes only PRINTED, so the artefact's
    # guards_failed=true carried no reason at all for a machine consumer.
    def failure(msg):
        nonlocal fail
        problems.append(msg)
        print(msg)
        fail = True

    # ---- pass 3 finding 2: an absent seed log silently shrinks the denominator.
    if True:
        lo, hi = m.group(1), m.group(2)
        expect = set(range(int(lo), int(hi) + 1))
        missing = sorted(expect - set(per_seed))
        extra = sorted(set(per_seed) - expect)
        if missing or extra:
            failure("SEED SET MISMATCH — missing %s, unexpected %s" % (missing, extra))

    # ---- PER-SEED structural guards (pass 3 finding 1)
    for seed in sorted(per_seed):
        st = per_seed[seed]
        if st["era_lines"] != 1:
            problems.append("seed %d: %d era lines (expected exactly 1)"
                            % (seed, st["era_lines"]))
        if st["census_lines"] != 1:
            problems.append("seed %d: %d divergent-case census lines (expected exactly 1)"
                            % (seed, st["census_lines"]))
        # F1, PER SEED. divergent_class is emitted once per option CLASS (:3313)
        # carrying only cs.worstCmd, so one SIG line can stand for cs.diverge
        # CASES. instances <= cases always; equality iff every diverge == 1.
        if st["actcomp_lines"] != 1:
            problems.append("seed %d: %d ACTUALLY-COMPARED lines (expected exactly 1)"
                            % (seed, st["actcomp_lines"]))
        elif st["v_compared"] != st["actually_compared"]:
            problems.append(
                "seed %d: %d verdicts say compared=true but the fuzzer reports "
                "ACTUALLY COMPARED %d — the DENOMINATOR is corrupt"
                % (seed, st["v_compared"], st["actually_compared"]))
        if st["era_lines"] == 1 and (st["v_era0"] != st["in_compared"]
                                     or st["v_era1"] != st["out_compared"]):
            problems.append(
                "seed %d: verdict era split %d/%d != the era line's %d/%d"
                % (seed, st["v_era0"], st["v_era1"],
                   st["in_compared"], st["out_compared"]))
        if st["census_lines"] == 1 and st["sig_div_instances"] != st["divcases"]:
            problems.append(
                "seed %d: divergent_class SIG instances %d != divergent CASES %d "
                "— a SIG line stands for multiple cases (F1); attribution invalid"
                % (seed, st["sig_div_instances"], st["divcases"]))

    scope_keys = {st["scope_key"] for st in per_seed.values() if st["scope_key"]}
    if len(scope_keys) != 1:
        print("REFUSING TO POOL SCOPE KEYS: %r" % (sorted(scope_keys),))
        sys.exit(2)
    scope_key = scope_keys.pop()

    era = {k: sum(st[k] for st in per_seed.values())
           for k in ("in_compared", "in_hard", "out_compared", "out_hard")}
    div_instances = sum(st["sig_div_instances"] for st in per_seed.values())
    div_cases = sum(st["divcases"] for st in per_seed.values())

    # ---- join
    by_norm = collections.defaultdict(list)
    for key, rec in cases.items():
        if "norm" in rec:
            by_norm[(key[0], rec["norm"])].append(key)
    ambiguous = {k: v for k, v in by_norm.items() if len(v) > 1}

    unjoined = []
    for sig in sigs:
        hits = by_norm.get((sig["seed"], sig["norm"]), [])
        if len(hits) == 1:
            cases[hits[0]].setdefault("sigs", []).append((sig["sev"], sig["class"]))
        else:
            unjoined.append(sig)

    # ---- buckets, straight off the ledger
    hard_in, hard_out, hard_novrd = [], [], []
    true_in, true_out, n_false = set(), set(), 0
    for key, rec in sorted(cases.items()):
        if rec.get("has_verdict"):
            if rec.get("compared") == "true":
                (true_in if rec.get("era") == "0" else true_out).add(key)
            else:
                n_false += 1
        if rec.get("hard") == "true":
            (hard_in if rec.get("era") == "0" else hard_out).append(key)
        elif not rec.get("has_verdict") and any(s[0] == "HARD" for s in rec.get("sigs", [])):
            hard_novrd.append(key)

    print("=" * 78)
    print("r56 CASE DECOMPOSITION — %s" % armdir)
    print("scope key: %s   seeds: %d" % (scope_key, len(per_seed)))
    print("=" * 78)
    print("ARM'S OWN ERA LINE:  in-scope compared=%d hard=%d | out-of-scope compared=%d hard=%d"
          % (era["in_compared"], era["in_hard"], era["out_compared"], era["out_hard"]))
    print("HONEST DENOMINATOR: the era line's 'compared' counts CHECKED cases."
          " ACTUALLY COMPARED in scope: %d (out of scope: %d, never-compared: %d)."
          % (len(true_in), len(true_out), n_false))
    print("THIS JOIN:           in-scope HARD cases=%d | out-of-scope HARD cases=%d"
          % (len(hard_in), len(hard_out)))
    print("LEDGER-EXCLUDED (HARD signal, NOT in compared): %d cases" % len(hard_novrd))
    print()

    # ---- DENOMINATOR VALIDATION (pass 3 finding 2). Two independent checks.
    if len(true_in) + len(true_out) + n_false != era["in_compared"] + era["out_compared"]:
        failure("VERDICT LINES (%d) != THE ERA LINE'S CHECKED TOTAL (%d)"
                % (len(true_in) + len(true_out) + n_false,
                   era["in_compared"] + era["out_compared"]))
    phantom = [k for k, r in sorted(cases.items()) if r.get("has_verdict") and "norm" not in r]
    if phantom:
        failure("%d FZ5VERDICT LINES WITH NO FZ5CASE LINE — phantom cases inflate n"
                % len(phantom))

    # ---- THE COUNT TARGET, AND THE n IT REQUIRES
    floor_n = int(400 * UPPER95[max_hard]) + 1 if max_hard < len(UPPER95) else None
    print("TARGET: <= %d in-scope HARD case(s).%s" % (
        max_hard,
        "  LIVE BAR — the power requirement, START_HERE 3a.12."
        if max_hard == 1 else "  NOT THE LIVE BAR (live target is <=1, 3a.12)."))
    if floor_n:
        ok = len(true_in) >= floor_n
        print("        the bar is a RATE (1 in 400); at k=%d the 95%% one-sided bound"
              " clears it only if n >= %d. n = %d -> %s"
              % (max_hard, floor_n, len(true_in),
                 ("OK, %.1f%% headroom" % (100.0 * (len(true_in) - floor_n) / floor_n))
                 if ok else "POPULATION TOO SMALL — THE TARGET CANNOT BE DEMONSTRATED"))
        if not ok:
            failure("POPULATION TOO SMALL: n=%d < %d required for k<=%d at 95%%"
                    % (len(true_in), floor_n, max_hard))
    print()

    if div_instances == div_cases:
        print("divergent-case census: %d SIG instances == %d divergent CASES, checked"
              " PER SEED on %d seeds (so every diverge==1 and the 1:1 holds ON THIS ARM)"
              % (div_instances, div_cases, len(per_seed)))
    if len(hard_in) != era["in_hard"] or len(hard_out) != era["out_hard"]:
        failure("JOIN DISAGREES WITH THE ARM'S OWN ERA LINE (%d/%d vs %d/%d)"
                % (len(hard_in), len(hard_out), era["in_hard"], era["out_hard"]))
    if ambiguous:
        failure("AMBIGUOUS CASE KEYS (%d) — normalisation collides" % len(ambiguous))
    hard_unjoined = [s for s in unjoined if s["sev"] == "HARD"]
    if unjoined:
        print("UNJOINED SIG LINES: %d (%d HARD)" % (len(unjoined), len(hard_unjoined)))
        if hard_unjoined:
            failure("%d HARD SIGNALS JOIN TO NO CASE — instrument defect" % len(hard_unjoined))
    if problems:
        for _p in list(problems):
            print("GUARD: %s" % _p)
        fail = True

    # F2 — an unexplained HARD case must never be counted as clearable.
    # pass 3 finding 8: cover BOTH scopes, not just in-scope.
    silent = [k for k in hard_in + hard_out
              if not any(sev == "HARD" for sev, _ in cases[k].get("sigs", []))]
    if silent:
        print("%d HARD CASES CARRY NO HARD SIGNAL — not explained, not clearable:"
              % len(silent))
        for k in silent:
            print("      seed %d case %d" % k)
        failure("%d HARD CASES CARRY NO HARD SIGNAL" % len(silent))

    # CONTRADICTION — SIG=HARD: is emitted AT the sites that set caseHard = true,
    # so this is a genuinely independent witness on the `hard=` field. Pass 3
    # verified the premise across all 19 SIG=HARD sites and confirmed it cannot
    # fire falsely on ledger-excluded cases.
    contra = [k for k, r in sorted(cases.items())
              if r.get("has_verdict") and r.get("hard") != "true"
              and any(sev == "HARD" for sev, _ in r.get("sigs", []))]
    if contra:
        print("%d CASES CARRY A HARD SIGNAL BUT A VERDICT SAYING hard != true:" % len(contra))
        for k in contra[:10]:
            print("      seed %d case %d compared=%s" % (k[0], k[1], cases[k].get("compared")))
        failure("%d CASES CARRY A HARD SIGNAL WITH hard != true" % len(contra))

    # NB pass 3 finding 4: the former "bucket balance" check was PROVABLY the same
    # predicate as CONTRADICTION and had zero independent power. REMOVED rather
    # than kept as a second green light over the same input — two guards reported
    # separately inflate apparent coverage.

    if fail:
        print()
        print("A GUARD FAILED. EVERYTHING BELOW IS INVALID — DO NOT QUOTE IT.")

    print()
    print("--- IN-SCOPE HARD CASES, ONE ROW EACH (%d) ---" % len(hard_in))
    for k in hard_in:
        rec = cases[k]
        sset = sorted({c for sev, c in rec.get("sigs", []) if sev == "HARD"})
        print("  seed %d case %-4d eng=%-10s route=%-34s signals=%s"
              % (k[0], k[1], rec.get("eng", "?"), rec.get("route", "?"),
                 ",".join(sset) or "NONE"))

    def class_case_counts(keys):
        c = collections.Counter()
        for k in keys:
            for sev, cls in set(cases[k].get("sigs", [])):
                if sev == "HARD":
                    c[cls] += 1
        return c

    cc = class_case_counts(hard_in)
    print()
    print("--- PER-CLASS **CASE** COUNTS, in scope (unit: CASES, not instances) ---")
    for cls, n in cc.most_common():
        print("   %-34s %d" % (cls, n))

    print()
    print("--- OUT-OF-SCOPE HARD CASES (%d) ---" % len(hard_out))
    for k in hard_out:
        sset = sorted({c for sev, c in cases[k].get("sigs", []) if sev == "HARD"})
        print("  seed %d case %-4d signals=%s" % (k[0], k[1], ",".join(sset) or "NONE"))

    print()
    print("--- LEDGER-EXCLUDED HARD CASES (%d) — NOT in any compared rate ---" % len(hard_novrd))
    exc = collections.Counter()
    for k in hard_novrd:
        for sev, cls in set(cases[k].get("sigs", [])):
            if sev == "HARD":
                exc[cls] += 1
    for cls, n in exc.most_common():
        print("   %-34s %d cases" % (cls, n))

    # ---- WHICH CLASSES / COMBINATIONS CAN REACH THE TARGET
    need = len(hard_in) - max_hard
    print()
    minimal = []
    if need <= 0:
        print("--- TARGET ALREADY MET: %d in-scope HARD <= %d. Nothing must clear. ---"
              % (len(hard_in), max_hard))
    else:
        print("--- CAN A CLASS SUPPLY THE CASES THE BAR NEEDS? (R80) ---")
        print("    R80 (instances >= cases) holds ONLY for classes emitted PER CASE."
              " divergent_class is emitted per option CLASS (:3313), where one instance"
              " can stand for MANY cases. The counts below are CASE counts from the join,"
              " exact PROVIDED THE PER-SEED CENSUS CHECK ABOVE PASSED; if it did not, the"
              " divergent_class case count is itself a LOWER bound.")
        print("    bar needs <=%d HARD in %d actually-compared; at %d, so %d must clear."
              % (max_hard, len(true_in), len(hard_in), need))
        for cls, n in cc.most_common():
            print("   %-34s touches %2d in-scope cases -> %s"
                  % (cls, n, "POSSIBLE" if n >= need else "CANNOT (arithmetically)"))

        classes = sorted(cc)
        sigsets = [frozenset(c for s, c in cases[k].get("sigs", []) if s == "HARD")
                   for k in hard_in]
        print()
        print("--- WHICH CLASS COMBINATIONS REACH THE TARGET? (a case clears only when ALL its signals clear) ---")
        for r in range(1, len(classes) + 1):
            for combo in itertools.combinations(classes, r):
                S = frozenset(combo)
                # F2: an EMPTY signal set must NEVER clear.
                cleared = sum(1 for ss in sigsets if ss and ss <= S)
                if cleared >= need and not any(set(m) < S for m in minimal):
                    minimal.append(combo)
                    rem = len(hard_in) - cleared
                    rate = ("1 in %d" % (len(true_in) // rem)) if rem else "0 HARD"
                    print("   MINIMAL SUFFICIENT SET: {%s} clears %d, leaves %d -> %s"
                          % (", ".join(combo), cleared, rem, rate))
        if not minimal:
            print("   NO COMBINATION OF OBSERVED CLASSES REACHES THE TARGET.")
        print("   best PAIRS (for contrast):")
        for n, p in sorted(((sum(1 for ss in sigsets if ss and ss <= frozenset(p)), p)
                            for p in itertools.combinations(classes, 2)), reverse=True)[:3]:
            print("      {%s} clears %d" % (", ".join(p), n))

    if outjson:
        # pass 3 finding 3: the artefact was written on FAILED runs with no marker,
        # and a consumer reading only the JSON could not tell.
        payload = {
            "guards_failed": fail,
            "problems": problems,
            "armdir": armdir, "scope_key": scope_key,
            "max_hard_target": max_hard,
            "n_in_scope_actually_compared": len(true_in),
            "n_in_scope_checked_era_line": era["in_compared"],
            "min_n_for_target_at_95pc": floor_n,
            "era_line": era,
            "minimal_sufficient_class_sets": None if fail else [list(m) for m in minimal],
            "in_scope_hard": [
                {"seed": k[0], "case": k[1], "eng": cases[k].get("eng"),
                 "route": cases[k].get("route"),
                 "signals": sorted({c for s, c in cases[k].get("sigs", []) if s == "HARD"}),
                 "cmd": cases[k].get("cmd")} for k in hard_in],
            "out_scope_hard": [
                {"seed": k[0], "case": k[1],
                 "signals": sorted({c for s, c in cases[k].get("sigs", []) if s == "HARD"}),
                 "cmd": cases[k].get("cmd")} for k in hard_out],
            "ledger_excluded": [
                {"seed": k[0], "case": k[1],
                 "signals": sorted({c for s, c in cases[k].get("sigs", []) if s == "HARD"}),
                 "cmd": cases[k].get("cmd")} for k in hard_novrd],
            "per_class_case_counts_in_scope": None if fail else dict(cc),
        }
        # PASS 4 F6: the old refusal left a STALE GREEN artefact on disk for a
        # DIFFERENT arm, so a JSON-polling consumer read a passing result from a
        # failed run. Always write, but never to the canonical path on failure.
        target = (outjson + ".failed.json") if fail else outjson
        try:
            with open(target, "w") as fh:
                json.dump(payload, fh, indent=1, sort_keys=True)
        except OSError as e:
            print("\nCOULD NOT WRITE %s: %s" % (target, e))
            sys.exit(2)
        print("\nwrote %s%s" % (target,
              "  (guards_failed=true — canonical artefact NOT touched)" if fail else ""))

    print()
    print("VERDICT: %s" % ("FAILED" if fail else "COMPLETE"))
    sys.exit(1 if fail else 0)


if __name__ == "__main__":
    main()
