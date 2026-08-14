#!/usr/bin/env python3
"""r56_case_decomposition.py — decompose the in-scope HARD CASES of a fuzzer5 arm,
PER CASE, on a join that respects the fuzzer's own ledger.

WHY THIS EXISTS (round 56)
--------------------------
The project's standing per-case rate comes from `era_split_arm.py` (12 HARD in
2,091 in scope). Its per-class breakdown comes from `analyze_arm.py`, which
counts SIGNAL INSTANCES (48) over a different unit. Neither answers "what are
the twelve cases, individually". Round 51's decomposition did, but it is four
rounds and two engine fixes old and its top family has since been closed on one
arm and refuted on the other.

THE JOIN, AND WHY THE OBVIOUS ONE IS WRONG (R54)
------------------------------------------------
Grouping `SIG=` lines by their repro command DOES NOT PARTITION THE HARD CASE
SET, for TWO independent reasons — the project had recorded only the first:

  (1) LEDGER. `go_solved_dos_date_horizon` and `go_solved_dos_refused` `continue`
      before `checked++`, so those cases are NOT in `compared` and carry no
      FZ5VERDICT line at all. They cannot appear in a compared-denominator rate.

  (2) 🆕 EMISSION. The `apr_differs` site (:2912) appends ` apr` to the command,
      so that one class's SIG lines do not string-match their own case's dump
      line. Grouping on the raw string therefore SPLITS those cases off from
      their siblings. Measured on this arm: 50 of 51 SIG lines join by EXACT
      string; only the 10 apr_differs lines need anything stripped.
      ⚠️ An earlier draft of this file claimed non/norate/noterm/lastdmy= were
      also SIG-only. THAT WAS FALSE (audit F4) — the canonical dump line carries
      them too, and dropping them deleted the solve mode from the case key.

The join used here is the fuzzer's OWN case identity, which `FZ5CASEDUMP=1`
already transports and which nothing has to reconstruct:

    FZ5CASE    <idx> <canonical repro command>
    FZ5OUTCOME <idx> bucket=<...> goOK=<...>
    FZ5VERDICT <idx> hard=<bool> era=<0|1> compared=<bool> eng=<...> route=<...>

⚠️ FZ5VERDICT is emitted once per **checked** case, NOT per **compared** case
(audit F5). The `!goOK` branch at :2432 emits `compared=false`. On this arm that
is 2,211 verdict lines against 2,209 actually compared, so the in-scope
denominator the project calls 2,091 is honestly **2,089**. Both are printed
below. The fuzzer's own comment at :2974-2981 says the same thing.

`(seed, idx)` is the case key.

🚨 WHAT THIS INSTRUMENT DOES **NOT** ESTABLISH (audit F3, sharpened by pass 2).
The check against the arm's era line is a COUNT identity, not the SET identity
round 51 performed, and it cannot be one: `eraHard[caseEra]++` (:2920) and the
FZ5VERDICT printf (:2958) read the SAME `caseHard`/`caseEra` in the same
iteration, 38 lines apart. **So the count identity has power against TRANSPORT
LOSS and DUPLICATION only — never against the fuzzer computing the flag wrongly.**
⚠️ **NO CONTENT FIELD ON ANY LINE IS VALIDATED except as listed below.**
Surviving mutants, each exiting 0: swapping `era=` on two verdict lines (changes
WHICH twelve are in scope); changing `eng=` on one (falsifies "all twelve are
piecewise"); renaming one SIG class on a case that keeps other signals
(destroys UNIQUEness of the minimal set). `route=` is equally unvalidated.
⚠️ **AN EARLIER DRAFT SAID "the 12/3 counts are trustworthy" AND "closing this
needs the fuzzer to transport case INDICES". BOTH WERE WRONG.** The `hard=` half
closes with NO fuzzer change, because `SIG=HARD:` lines are emitted at the very
sites that set `caseHard = true` — so a HARD signal on a case whose verdict says
`hard != true` is a contradiction the log can already detect. That check is now
performed (see CONTRADICTION below). Only the `era=` half still needs indices.

FAILS CLOSED on: an unjoined HARD SIG line; per-era HARD counts disagreeing with
the era line; an ambiguous case key; a missing era line; an in-scope HARD case
with NO HARD signal (F2); divergent_class instances not matching the arm's own
divergent-case census (F1); **the census being ABSENT at all** (pass 2 F1 — the
first version's guard was gated on a truthy census total and switched itself
off when the census went missing, which is round 51's fail-open verbatim);
**a HARD signal on a case whose verdict says `hard != true`** (pass 2 F4); and
**any HARD signal landing in no bucket at all** (pass 2 F5 — a `compared=false`
verdict line carries a hardcoded `hard=false`).

TARGET. The bar is a CASE COUNT and it is a PARAMETER, not a constant: pass
`--max-hard N`. **Default 1**, per the power requirement added to the exit
criterion (START_HERE §3a.12): the 95% one-sided bound, not the point estimate,
must clear 1 in 400, which at this population means <=1 HARD case. ⚠️ **An
earlier version hardcoded 5 — the RETIRED point-estimate target — and its
"minimal sufficient set" answered a question the project no longer asks.**
"""
import os, re, sys, json, collections

# Tokens a SIG line may carry that the canonical FZ5CASE line does not.
# F4 (audit): the canonical FZ5CASE line DOES carry non/norate/noterm/lastdmy=
# (flags are appended to args at dos_fuzzer5_test.go:1857-1890 BEFORE the cmd is
# built at :2086 and printed at :2108). 50 of 51 SIG lines join by EXACT string.
# Only apr_differs appends a token to the format string (:2912). Dropping more
# than that deletes real case identity (the solve mode) for no reason.
_DROP_EXACT = {"apr"}
_DROP_PREFIX = ()


def normalise(cmd):
    out = []
    for tok in cmd.split():
        if tok in _DROP_EXACT:
            continue
        if tok.startswith(_DROP_PREFIX):
            continue
        out.append(tok)
    return " ".join(out)


SIG_RE = re.compile(r"SIG=(HARD|ADVISORY):(\w+)\s+(.*)$")
# F1 (audit): divergent_class is emitted ONCE PER OPTION CLASS (:3313), not per
# case, carrying only cs.worstCmd. One SIG line can represent cs.diverge CASES.
# The 1:1 seen on this arm is luck. This census line is the only way to know.
DIVCLASS_RE = re.compile(r"divergent option classes:\s*(\d+) of (\d+) compared cases")
ERA_RE = re.compile(
    r"era split \(cases, SCOPE KEY=(\w+),[^)]*\):\s*"
    r"in-scope<=2099 compared=(\d+) hard=(\d+)\s*\|\s*"
    r"out-of-scope>2099 compared=(\d+) hard=(\d+)"
)


def parse_seed(path, seed, cases, sigs, era_tot, problems, true_in, true_out):
    scope_key = None
    with open(path, "r", errors="replace") as fh:
        for line in fh:
            line = line.rstrip("\n")
            s = line.strip()
            if s.startswith("FZ5CASE "):
                parts = s.split(" ", 2)
                idx, cmd = int(parts[1]), parts[2]
                cases[(seed, idx)]["cmd"] = cmd
                cases[(seed, idx)]["norm"] = normalise(cmd)
            elif s.startswith("FZ5OUTCOME "):
                parts = s.split()
                idx = int(parts[1])
                for kv in parts[2:]:
                    k, _, v = kv.partition("=")
                    cases[(seed, idx)][k] = v
            elif s.startswith("FZ5VERDICT "):
                parts = s.split()
                idx = int(parts[1])
                for kv in parts[2:]:
                    k, _, v = kv.partition("=")
                    cases[(seed, idx)][k] = v
                # A second verdict line for the same case would SILENTLY OVERWRITE
                # the first (pass 1 noted this; pass 2's mutG showed it is only
                # tolerated, never detected). The ledger emits exactly one per
                # checked case, so two is an instrument or transport defect.
                if cases[(seed, idx)].get("has_verdict"):
                    problems.append("seed %s case %s: DUPLICATE FZ5VERDICT line" % (seed, idx))
                cases[(seed, idx)]["has_verdict"] = True
                # NB counted into a SET, not a Counter: this is the only quantity
                # here derived from LINES rather than case keys, so a duplicated
                # verdict line would otherwise inflate it (pass 2 finding 6).
                rec = cases[(seed, idx)]
                if rec.get("compared") == "true":
                    (true_in if rec.get("era") == "0" else true_out).add((seed, idx))
            else:
                m = SIG_RE.search(s)
                if m:
                    sev, cls, cmd = m.group(1), m.group(2), m.group(3)
                    sigs.append({"seed": seed, "sev": sev, "class": cls,
                                 "cmd": cmd, "norm": normalise(cmd)})
                    continue
                m = ERA_RE.search(s)
                if m:
                    scope_key = m.group(1)
                    era_tot["in_compared"] += int(m.group(2))
                    era_tot["in_hard"] += int(m.group(3))
                    era_tot["out_compared"] += int(m.group(4))
                    era_tot["out_hard"] += int(m.group(5))
                    era_tot["seeds"] += 1
                    continue
                m = DIVCLASS_RE.search(s)
                if m:
                    era_tot["divcases"] += int(m.group(1))
                    era_tot["divcensus_seen"] += 1
    if scope_key is None:
        problems.append("seed %s: NO ERA LINE" % seed)
    return scope_key


def main():
    argv = [a for a in sys.argv[1:]]
    max_hard = 1                      # the LIVE bar (power requirement, §3a.12)
    rest = []
    for a in argv:
        if a.startswith("--max-hard="):
            max_hard = int(a.split("=", 1)[1])
        else:
            rest.append(a)
    armdir = rest[0]
    outjson = rest[1] if len(rest) > 1 else None

    cases = collections.defaultdict(dict)
    sigs = []
    era_tot = collections.Counter()
    true_in, true_out = set(), set()
    problems = []
    scope_keys = set()

    logs = sorted(f for f in os.listdir(armdir) if f.startswith("seed_") and f.endswith(".log"))
    if not logs:
        print("NO SEED LOGS in %s" % armdir); sys.exit(2)
    for f in logs:
        seed = int(f[len("seed_"):-len(".log")])
        k = parse_seed(os.path.join(armdir, f), seed, cases, sigs, era_tot, problems,
                       true_in, true_out)
        if k:
            scope_keys.add(k)

    if len(scope_keys) != 1:
        print("REFUSING TO POOL SCOPE KEYS: %r" % (sorted(scope_keys),)); sys.exit(2)
    scope_key = scope_keys.pop()

    # ---- index normalised command -> case key, per seed (rule 9 / R52: check cardinality)
    by_norm = collections.defaultdict(list)
    for key, rec in cases.items():
        if "norm" in rec:
            by_norm[(key[0], rec["norm"])].append(key)
    ambiguous = {k: v for k, v in by_norm.items() if len(v) > 1}

    # ---- attach signals to cases
    unjoined = []
    for sig in sigs:
        hits = by_norm.get((sig["seed"], sig["norm"]), [])
        if len(hits) == 1:
            sig["case"] = hits[0]
            cases[hits[0]].setdefault("sigs", []).append((sig["sev"], sig["class"]))
        else:
            sig["case"] = None
            unjoined.append(sig)

    # ---- the HARD case sets, straight off the ledger
    hard_in, hard_out, hard_novrd = [], [], []
    for key, rec in sorted(cases.items()):
        if rec.get("hard") == "true":
            (hard_in if rec.get("era") == "0" else hard_out).append(key)
    # cases carrying a HARD signal but NO verdict line = ledger-excluded
    for key, rec in sorted(cases.items()):
        if not rec.get("has_verdict") and any(s[0] == "HARD" for s in rec.get("sigs", [])):
            hard_novrd.append(key)

    print("=" * 78)
    print("r56 CASE DECOMPOSITION — %s" % armdir)
    print("scope key: %s   seeds: %d" % (scope_key, era_tot["seeds"]))
    print("=" * 78)
    print("ARM'S OWN ERA LINE:  in-scope compared=%d hard=%d | out-of-scope compared=%d hard=%d"
          % (era_tot["in_compared"], era_tot["in_hard"], era_tot["out_compared"], era_tot["out_hard"]))
    print("⚠️ HONEST DENOMINATOR (F5): the era line's 'compared' counts CHECKED cases."
          " Actually compared, in scope: %d (out of scope: %d)."
          % (len(true_in), len(true_out)))
    print("TARGET: <= %d in-scope HARD case(s). %s"
          % (max_hard,
             "LIVE BAR — the power requirement, START_HERE §3a.12."
             if max_hard == 1 else
             "⚠️ NOT THE LIVE BAR — the live target is <=1 (§3a.12)."))
    print("THIS JOIN:           in-scope HARD cases=%d | out-of-scope HARD cases=%d"
          % (len(hard_in), len(hard_out)))
    print("LEDGER-EXCLUDED (HARD signal, NOT in compared): %d cases" % len(hard_novrd))
    print()

    fail = False
    if len(hard_in) != era_tot["in_hard"] or len(hard_out) != era_tot["out_hard"]:
        print("🚨 JOIN DISAGREES WITH THE ARM'S OWN ERA LINE — FAILING CLOSED"); fail = True
    if ambiguous:
        print("🚨 AMBIGUOUS CASE KEYS (%d) — normalisation collides" % len(ambiguous))
        for k, v in list(ambiguous.items())[:5]:
            print("    seed %s -> %s" % (k[0], v))
        fail = True
    if unjoined:
        hard_unjoined = [s for s in unjoined if s["sev"] == "HARD"]
        print("UNJOINED SIG LINES: %d (%d HARD)" % (len(unjoined), len(hard_unjoined)))
        for s in unjoined[:10]:
            print("    %s %s:%s" % (s["seed"], s["sev"], s["class"]))
        if hard_unjoined:
            print("🚨 A HARD SIGNAL THAT JOINS TO NO CASE IS AN INSTRUMENT DEFECT")
            fail = True
    for p in problems:
        print("🚨 %s" % p); fail = True

    # F2 (audit): an in-scope HARD case with NO HARD signal would be counted as
    # cleared by fixing ANYTHING (frozenset() <= S is True for every S). An
    # unexplained HARD case must be assumed NOT cleared, and must fail the run.
    silent = [k for k in hard_in
              if not any(sev == "HARD" for sev, _ in cases[k].get("sigs", []))]
    if silent:
        print("🚨 %d IN-SCOPE HARD CASES CARRY NO HARD SIGNAL — the decomposition"
              " does not explain them and MUST NOT count them as clearable:" % len(silent))
        for k in silent:
            print("      seed %d case %d" % k)
        fail = True

    # F1 (audit pass 1), REBUILT AFTER PASS 2. divergent_class is emitted once per
    # option CLASS (:3313); one SIG line can stand for cs.diverge CASES, and the
    # census is the only witness. 🚨 THE FIRST VERSION GATED THIS ON
    # `era_tot["divcases"] and ...`, so a MISSING census silently disabled the
    # guard and printed "N instances == 0 divergent cases" as a PASS. That is
    # round 51's fail-open, reproduced by the fix for a different fail-open.
    div_instances = sum(1 for s_ in sigs if s_["class"] == "divergent_class")
    if era_tot["divcensus_seen"] != era_tot["seeds"]:
        print("🚨 divergent-case census seen on %d of %d seeds — THE F1 GUARD IS BLIND"
              % (era_tot["divcensus_seen"], era_tot["seeds"]))
        fail = True
    elif div_instances != era_tot["divcases"]:
        print("🚨 divergent_class SIG instances (%d) != the arm's divergent-CASE census"
              " (%d) — one SIG line stands for multiple cases (F1); attribution invalid"
              % (div_instances, era_tot["divcases"]))
        fail = True
    else:
        print("divergent_class census check: %d SIG instances == %d divergent CASES on"
              " %d/%d seeds (so every diverge==1 and the per-case 1:1 holds ON THIS ARM)"
              % (div_instances, era_tot["divcases"],
                 era_tot["divcensus_seen"], era_tot["seeds"]))

    # 🆕 CONTRADICTION (pass 2 F4). SIG=HARD: lines are emitted AT the sites that
    # set caseHard = true, so a HARD signal on a case whose verdict says
    # hard != true is a contradiction the log can detect WITHOUT any fuzzer
    # change. This is the independent cross-check the first docstring wrongly
    # claimed did not exist.
    contra = [k for k, r in sorted(cases.items())
              if r.get("has_verdict") and r.get("hard") != "true"
              and any(sev == "HARD" for sev, _ in r.get("sigs", []))]
    if contra:
        print("🚨 %d CASES CARRY A HARD SIGNAL BUT A VERDICT SAYING hard != true:" % len(contra))
        for k in contra[:10]:
            print("      seed %d case %d" % k)
        fail = True

    # 🆕 BUCKET BALANCE (pass 2 F5). Every case carrying a HARD signal must land
    # in exactly one of the three buckets. A `compared=false` verdict line
    # (:2432) hardcodes hard=false, so such a case fell through ALL of them.
    bucketed = set(hard_in) | set(hard_out) | set(hard_novrd)
    carries_hard = {k for k, r in cases.items()
                    if any(sev == "HARD" for sev, _ in r.get("sigs", []))}
    orphans = sorted(carries_hard - bucketed)
    if orphans:
        print("🚨 %d CASES CARRY A HARD SIGNAL AND LAND IN NO BUCKET:" % len(orphans))
        for k in orphans[:10]:
            print("      seed %d case %d compared=%s hard=%s"
                  % (k[0], k[1], cases[k].get("compared"), cases[k].get("hard")))
        fail = True

    # ---- per-class CASE counts (NOT instance counts — CAUTION 1)
    def class_case_counts(keys):
        c = collections.Counter()
        for k in keys:
            for sev, cls in set(cases[k].get("sigs", [])):
                if sev == "HARD":
                    c[cls] += 1
        return c

    print()
    print("--- IN-SCOPE HARD CASES, ONE ROW EACH (%d) ---" % len(hard_in))
    for k in hard_in:
        rec = cases[k]
        sset = sorted({c for sev, c in rec.get("sigs", []) if sev == "HARD"})
        adv = sorted({c for sev, c in rec.get("sigs", []) if sev == "ADVISORY"})
        print("  seed %d case %-4d eng=%-10s route=%-34s signals=%s%s"
              % (k[0], k[1], rec.get("eng", "?"), rec.get("route", "?"),
                 ",".join(sset) or "NONE", ("  adv=" + ",".join(adv)) if adv else ""))
    print()
    print("--- PER-CLASS **CASE** COUNTS, in scope (unit: CASES, not instances) ---")
    cc = class_case_counts(hard_in)
    for cls, n in cc.most_common():
        print("   %-34s %d" % (cls, n))
    print()
    print("--- OUT-OF-SCOPE HARD CASES (%d) ---" % len(hard_out))
    for k in hard_out:
        rec = cases[k]
        sset = sorted({c for sev, c in rec.get("sigs", []) if sev == "HARD"})
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

    # ---- the arithmetic the round-56 prompt asks for
    print()
    if fail:
        print()
        print("🚨🚨 RESULTS BELOW ARE INVALID — A GUARD FAILED ABOVE. DO NOT QUOTE THEM.")
    print("--- CAN A CLASS SUPPLY THE CASES THE BAR NEEDS? (R80) ---")
    print("    ⚠️ R80 (instances >= cases) holds ONLY for classes emitted PER CASE."
          " divergent_class is emitted per option CLASS (:3313), where one instance"
          " can stand for MANY cases — there the instance count is a LOWER bound."
          " The counts below are CASE counts from the join, not instance counts,"
          " so they are exact PROVIDED THE F1 CENSUS CHECK ABOVE PASSED; if it did"
          " not, the divergent_class CASE count is itself a LOWER bound, because"
          " the SIG line carries only cs.worstCmd and joins to exactly one case.")
    need = max(0, len(hard_in) - max_hard)
    print("    bar needs <=%d HARD in %d in-scope compared; at %d, so %d cases must clear."
          % (max_hard, len(true_in), len(hard_in), need))
    for cls, n in cc.most_common():
        print("   %-34s touches %2d in-scope cases -> %s"
              % (cls, n, "POSSIBLE" if n >= need else "🚨 CANNOT (arithmetically)"))

    # ---- WHICH COMBINATIONS OF CLASSES ACTUALLY CLEAR THE BAR
    # A case clears only when EVERY HARD signal on it clears (r51 §1.2). So a set
    # S of classes clears exactly those cases whose signal set is a SUBSET of S.
    import itertools
    classes = sorted(cc)
    sigsets = [frozenset(c for s, c in cases[k].get("sigs", []) if s == "HARD") for k in hard_in]
    print()
    print("--- WHICH CLASS COMBINATIONS CLEAR THE BAR? (a case clears only when ALL its signals clear) ---")
    minimal = []
    for r in range(1, len(classes) + 1):
        for combo in itertools.combinations(classes, r):
            S = frozenset(combo)
            # F2: an EMPTY signal set must NEVER clear — it is an unexplained case.
            cleared = sum(1 for ss in sigsets if ss and ss <= S)
            if cleared >= need:
                if not any(set(m) < S for m in minimal):
                    minimal.append(combo)
                    rem = len(hard_in) - cleared
                    # pass 2 finding 6: use the ACTUALLY-COMPARED denominator.
                    # The era line's 2,091 counts CHECKED cases; quoting it here
                    # would report a rate on two cases nothing was compared on.
                    rate = ("1 in %d (era-line denom: 1 in %d)"
                            % (len(true_in) // rem, era_tot["in_compared"] // rem)) if rem else "0 HARD"
                    print("   ✅ MINIMAL SUFFICIENT SET: {%s} clears %d, leaves %d -> %s"
                          % (", ".join(combo), cleared, rem, rate))
    if not minimal:
        print("   🚨 NO COMBINATION OF OBSERVED CLASSES CLEARS THE BAR.")
    print("   best PAIRS (for contrast):")
    pairs = sorted(((sum(1 for ss in sigsets if ss and ss <= frozenset(p)), p)
                    for p in itertools.combinations(classes, 2)), reverse=True)[:3]
    for n, p in pairs:
        print("      {%s} clears %d" % (", ".join(p), n))

    if outjson:
        payload = {
            "minimal_sufficient_class_sets": [list(m) for m in minimal],
            "armdir": armdir, "scope_key": scope_key,
            "era_line": dict(era_tot),
            "in_scope_hard": [
                {"seed": k[0], "case": k[1], "eng": cases[k].get("eng"),
                 "route": cases[k].get("route"),
                 "signals": sorted({c for s, c in cases[k].get("sigs", []) if s == "HARD"}),
                 "advisory": sorted({c for s, c in cases[k].get("sigs", []) if s == "ADVISORY"}),
                 "cmd": cases[k].get("cmd")}
                for k in hard_in],
            "out_scope_hard": [
                {"seed": k[0], "case": k[1],
                 "signals": sorted({c for s, c in cases[k].get("sigs", []) if s == "HARD"}),
                 "cmd": cases[k].get("cmd")}
                for k in hard_out],
            "ledger_excluded": [
                {"seed": k[0], "case": k[1],
                 "signals": sorted({c for s, c in cases[k].get("sigs", []) if s == "HARD"}),
                 "cmd": cases[k].get("cmd")}
                for k in hard_novrd],
            "per_class_case_counts_in_scope": dict(cc),
        }
        with open(outjson, "w") as fh:
            json.dump(payload, fh, indent=1, sort_keys=True)
        print("\nwrote %s" % outjson)

    print()
    print("VERDICT: %s" % ("FAILED" if fail else "COMPLETE"))
    sys.exit(1 if fail else 0)


if __name__ == "__main__":
    main()
