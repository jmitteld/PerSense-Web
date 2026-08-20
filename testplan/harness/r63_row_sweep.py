#!/usr/bin/env python3
"""r63_row_sweep.py — the FIRST row-for-row sweep of the standing amortization
arm against the DOS oracle.

WHY THIS EXISTS
---------------
Every gate this project owns compares TOTALS, an APR, or a signal class.
§109 — the off-cycle DRAIN path has no port of `AMORTOP.pas:1004`'s residual
fold — moves a ROW and leaves every total identical to the cent.  It was found
by hand at r62 on two screens and no committed instrument can see it.  This one
can.

WHAT IT MEASURES  (rule 9: population, generator, scope key, question set)
-------------------------------------------------------------------------
  GENERATOR   : TestDOSFuzzer5AllAdvancedOptions, FZ5CASEDUMP=1, over a named
                seed range at a named PERSENSE_FUZZ_N.  The STANDING ARM is
                seeds 50100-50109 at N=400.  The unit is a SCREEN (a generated
                case), NOT a case-level signal and NOT a row.
  QUESTION    : for each screen, are the port's rows and the oracle's rows equal
                position for position on (int, prin, bal)?
  SCOPE       : every screen the generator draws on which BOTH engines produce a
                row table.  Screens where either side refuses, errors or
                produces no table are counted and EXCLUDED, and the exclusion
                counts are published (R64: a run that produced no output is not
                a zero).
  TOLERANCE   : `--tol`, default 0.006 — a hair ABOVE a half cent, and the hair
                is load-bearing.  DOS's printed row is already rounded to cents;
                the port prints the raw quantity.  A tolerance set exactly AT
                0.005 sits ON that tie and resolves it by floating-point luck —
                the first cut of this sweep called 11 of 29 screens divergent
                and every one was a `.xx5` tie.  Comparison is NUMERIC, never by
                string.
                ⚠️ THE REASON USUALLY GIVEN FOR THAT IS WRONG AND IS RETRACTED
                HERE: it is NOT that DOS prints 4 decimals on the fancy format
                and 2 on the ordinary one.  BOTH branches of `PrintAndReset`
                (AMORTOP.pas:1024-1029 and :1031-1033) format through `ftoa2`,
                which is a BY-VALUE function — `x := 0.01*trunc(x*100+half)`
                mutates its own copy.  DOS holds the UNROUNDED value and rounds
                only to display, at 2 decimals, on both formats; amort_oracle
                then re-emits `:0:4`.  The port holding the raw value is
                therefore CORRECT and there is nothing to fix in the engine —
                round 63 item 8, decided by reading `PrintAndReset` in full.

WHAT IT CANNOT SEE  (rule 8)
----------------------------
  * It does not compare DATES — BY OMISSION, NOT BY IMPOSSIBILITY.  An earlier
    cut of this docstring claimed the oracle's row label was "a bare month
    fragment that cannot be aligned".  THAT IS FALSE AND IS RETRACTED: the label
    is `GetTok(line,1)` of a `DateStr` (VIDEODAT.pas:415) emitting
    `str(m:2)+'/'+str(d:2)+'/'+str(y mod 100)`, i.e. a full `10/10/24`.  It
    degrades to `11/` ONLY when the day is 1..9, because `str(d:2)`
    right-justifies a space into the token.  `GOAMORT_ROWDATES` is already live
    at cmd/goamort/main.go:527.  A date comparison is buildable TODAY with the
    binaries this repo already has, on the fancy population.  It is owed.
  * It does not compare the PAYMENT, and it must not be read as having done so.
    `parse_rows` returns it and `classify` discards it.  Over the 1,774 screens
    this sweep compares, 118 disagree by more than half a cent — but that is
    `cmd/goamort`'s reporting HEURISTIC (main.go:399-427 derives the payment by
    scanning for the first regular row instead of reporting the engine's own
    `res.RegularPayment`), NOT an engine divergence.  The arm's Signal 5
    compares the transported value and reports 0 differ.  Sized here for the
    first time; `attribute_seven.py:285` already says "echo is a HEURISTIC —
    not scored".
  * It compares POSITIONALLY.  A row inserted or dropped in the middle shifts
    every later row and is reported as a long run of differences, not as an
    insertion.  ROWCOUNT mismatches are bucketed separately for that reason.
  * It says NOTHING about totals.  A screen can be IDENTICAL here and still be a
    HARD case; a screen can be DIFF here with every total agreeing — which is
    exactly §109's signature and exactly why this instrument was needed.
  * It excludes every screen DOS REFUSES (`ENGINE ERROR`).  A refusal is an
    answer but it is not a table, so refusal PARITY — does the port refuse the
    same screens? — is a question this sweep does not ask and must not be read
    as having answered.

THE §109 SIGNATURE
------------------
A screen is bucketed SEC109_SHAPE when the ONLY differing row is the LAST one
and, on that row, the oracle reports `bal` == 0 while the port reports `bal` far
from 0.  That is DOS's `PrintAndReset` fold (`AMORTOP.pas:1004`):

    if (DateComp(date,very_last)=0) then begin
       payamt:=payamt+principal;
       cumamt:=cumamt+principal;
       principal:=0;
       end;

applied to a row the port's five hand-gated `atVeryLast` branches never reach.
The bucket is a SHAPE, not an attribution: it says the divergence has §109's
form, not that §109 caused it.  Anything else that folds a terminal residual
lands here too and must be read, not counted.

POSITIVE CONTROL (R51/R77: prove the instrument can see the thing)
------------------------------------------------------------------
`--control` runs the pinned §109 screen and asserts the sweep buckets it
SEC109_SHAPE.  A sweep whose control does not fire has measured nothing, and the
control's verdict is part of the EXIT STATUS in every mode -- an earlier cut
computed `ok`, printed it, and then returned 0 anyway whenever --armdir was
given, which made the banner decorative in the only mode that produces a sweep.

THE CONTROL MUST BE RUN AGAINST A SUBJECT KNOWN TO CARRY THE DEFECT.  It asserts
the shape FIRES, so pointing it at a FIXED binary makes it cry "blind" on a
correct port.  `--control-bin` names the binary it uses and defaults to the
pristine one; `--expect-control` can invert it when the subject is a fixed tree.

BUILDING ITS OWN SUBJECT (R98)
------------------------------
`--build` compiles goamort from THE TREE THIS SCRIPT LIVES IN before sweeping,
and the report NAMES the binary and its md5 either way.  r62's pin file fell
back to a pre-built /tmp/goamort and passed on a tree carrying a different fix.
"""

import argparse
import concurrent.futures as cf
import hashlib
import json
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", ".."))

# Tokens that change WHAT IS PRINTED rather than what is computed.  They must be
# stripped before `rows` is appended or the two drivers print different things.
DISPLAY_TOKENS = {"adjdump", "bdump", "pdump", "dumpraw", "rows", "horizon",
                  "payoff", "apr", "eval"}

# The largest difference attributable to DOS rounding its printed row to cents
# while the port prints the raw quantity.  A hair above half a cent so the tie
# itself is inside the band rather than on its edge.
TIE_MAX = 0.0051

ROW_RE = re.compile(
    r"^row\s+(\S+)\s+int\s+(-?[\d.]+)\s+prin\s+(-?[\d.]+)\s+bal\s+(-?[\d.]+)\s*$")

# The pinned §109 screen, verbatim, so --control needs no corpus.  Provenance:
# seed 50104 case 352 of the standing arm (FZ5CASEDUMP).
# ⚠️ "The TWO screens" is retracted: r62 named two, this list holds ONE.  The
# second (§96's c3 row 89) was never transcribed here, and an audit found the
# docstring still claiming two after the --control prose had been corrected —
# a retraction that reached one sentence and not its neighbour (R86).
CONTROL_SCREENS = [
    ("50104/352",
     "141010.87 0.1012840000 10 1 b365 exact usa loandmy=9.10.2024 "
     "firstdmy=9.3.2026 mor=53 b65=10763.38 b101=37846.14 pre=89:16:4:357.10 "
     "adj=17:0.0663280000: adj=29:0.1455360000:17923.20 targ=3167.69 "
     "pts=0.018160 payhard=23430.59 non lastdmy=9.3.2035"),
]


def md5(path):
    try:
        with open(path, "rb") as fh:
            return hashlib.md5(fh.read()).hexdigest()
    except OSError:
        return "MISSING"


def parse_rows(text):
    """Return (payment, [(int, prin, bal)], fancy) or None if there is no table.

    `fancy` is read off the oracle's OWN row labels.  amort_oracle prints a
    date-leading label ("row 12/") on the FANCY screen format and a bare payment
    number ("row 7") on the ORDINARY one.  That distinction decides whether
    GOAMORT_ALLROWS belongs on this screen -- see run_screen.
    """
    rows = []
    payment = None
    saw_end = False
    fancy = False
    for line in text.splitlines():
        line = line.strip()
        if line.startswith("payment "):
            try:
                payment = float(line.split()[1])
            except (IndexError, ValueError):
                pass
            continue
        if line == "end":
            saw_end = True
            continue
        m = ROW_RE.match(line)
        if m:
            if "/" in m.group(1):
                fancy = True
            rows.append(tuple(float(g) for g in m.groups()[1:]))
    if not saw_end:
        return None
    return payment, rows, fancy


def run_screen(args_str, oracle, goamort, timeout):
    toks = [t for t in args_str.split() if t not in DISPLAY_TOKENS]
    argv = toks + ["rows"]
    # INSTRUMENT DEFECT #15 HAS TWO SIDES AND ROUND 62 ONLY DOCUMENTED ONE.
    #
    # DOS's `rows` mode excludes PayNum 0/-1 settlement rows -- but it can only
    # do that on the ORDINARY format, where the line begins with the payment
    # number.  On the FANCY (date-leading) format IsDetailLine cannot apply the
    # exclusion and DOS emits the settlement row.  So:
    #
    #     oracle FANCY    -> port must emit them too  -> GOAMORT_ALLROWS=1
    #     oracle ORDINARY -> port must NOT emit them  -> GOAMORT_ALLROWS unset
    #
    # Setting it unconditionally puts the port ONE ROW AHEAD on every ordinary
    # prepaid/in-advance screen.  Six of the first 120 screens of the standing
    # arm read as `delta +1` ROWCOUNT divergences for exactly this reason before
    # the format was read off the oracle's own labels.  The oracle runs FIRST
    # and its output decides the port's environment.
    base_env = dict(os.environ)
    base_env.pop("GOAMORT_ALLROWS", None)
    out = {}
    fancy = False
    for name, binary in (("oracle", oracle), ("port", goamort)):
        env = dict(base_env)
        if name == "port" and fancy:
            env["GOAMORT_ALLROWS"] = "1"
        try:
            p = subprocess.run([binary] + argv, capture_output=True, text=True,
                               timeout=timeout, env=env)
        except subprocess.TimeoutExpired:
            return {"bucket": "TIMEOUT", "side": name}
        # CAUTION 14: a "refusal" can be a hang.  A timeout is bucketed above and
        # is NOT counted as a refusal.
        parsed = parse_rows(p.stdout)
        if parsed is not None:
            out[name] = parsed[:2]
            if name == "oracle":
                fancy = parsed[2]
        if parsed is None:
            err = (p.stderr or "")
            # NOTE #24: goamort implements neither `norate` nor `noamt`; they
            # live in amort_oracle and dos_fuzzer5_test.go only.  Round 33 lost
            # 32% of its corpus to this SILENTLY.  It is a named, structural
            # exclusion of the DRIVER, not a divergence and not a refusal.
            if "unimplemented token" in err:
                toks = re.findall(r"unimplemented token\(s\): (.*)", err)
                return {"bucket": "UNIMPL_NOTE24", "side": name,
                        "tokens": toks[0].strip() if toks else "?"}
            # The PORT declines a screen by printing `ERR <message>` on STDOUT
            # and exiting 0.  DOS produced a table and the port did not: that is
            # a real divergence class (R67 — a fix can silently turn an answer
            # into a refusal), NOT a harness failure, and it must never be filed
            # under "no table / harness error".  It is still EXCLUDED from the
            # row comparison, because there are no rows to compare.
            if name == "port" and p.stdout.lstrip().startswith("ERR "):
                return {"bucket": "PORT_REFUSED", "side": name,
                        "msg": p.stdout.strip().splitlines()[0][4:][:120]}
            if name == "oracle" and "ENGINE ERROR" in err:
                # DOS DECLINED THE SCREEN.  A refusal is an answer, but it is
                # not a TABLE, so there is nothing to compare row for row.  It
                # is bucketed on its own so the exclusion can never be read as
                # agreement (CAUTION 12) -- and so that a future round can ask
                # the refusal-parity question this sweep does not ask.
                first = err.strip().splitlines()[0] if err.strip() else "?"
                return {"bucket": "ORACLE_REFUSED", "side": name, "msg": first[:120]}
            return {"bucket": "NO_TABLE", "side": name, "stderr": err[:200]}
    return {"bucket": "OK", "oracle": out["oracle"], "port": out["port"],
            "fancy": fancy}


def classify(res, tol):
    """Bucket one screen.

    TWO thresholds, and the gap between them is the whole point.

    DOS's row values are already rounded to cents by Round2; the port prints the
    unrounded quantity at four decimals.  A row therefore differs by EXACTLY a
    half cent whenever the port's value sits on a `.xx5` boundary — DOS printed
    `5523.97`, the port `5523.975`.  A tolerance set AT 0.005 lands on that tie
    and resolves it by floating-point luck: the first cut of this sweep called
    11 of 29 screens divergent and every one of them was this.

    So: `tol` (default 0.006) is what "differs" means, and any row whose largest
    component difference falls in (0, TIE_MAX] is additionally counted as a TIE.
    Ties are NOT discarded — a screen whose only differences are ties buckets as
    TIE_ONLY and is reported separately.  Whether the port SHOULD round at the
    print boundary the way DOS does is an open question (round 63 item 8); this
    instrument's job is to keep the two populations apart, not to answer it.
    """
    o_pay, o_rows = res["oracle"]
    p_pay, p_rows = res["port"]
    if len(o_rows) != len(p_rows):
        return "ROWCOUNT", {"oracle_rows": len(o_rows), "port_rows": len(p_rows),
                            "delta": len(p_rows) - len(o_rows)}
    diffs = []
    ties = 0
    for i, (o, p) in enumerate(zip(o_rows, p_rows)):
        worst = max(abs(a - b) for a, b in zip(o, p))
        if worst > tol:
            diffs.append((i, o, p))
        elif worst > 0 and worst <= TIE_MAX:
            ties += 1
    if not diffs:
        if ties:
            return "TIE_ONLY", {"rows": len(o_rows), "tie_rows": ties}
        return "IDENTICAL", {"rows": len(o_rows)}
    detail = {"rows": len(o_rows), "ndiff": len(diffs), "tie_rows": ties,
              "first": diffs[0], "last": diffs[-1]}
    # §109's shape: exactly the LAST row, oracle balance folded to zero, port's
    # not.  Read the docstring before quoting this bucket as an attribution.
    if len(diffs) == 1 and diffs[0][0] == len(o_rows) - 1:
        o_bal, p_bal = diffs[0][1][2], diffs[0][2][2]
        if abs(o_bal) <= tol and abs(p_bal) > tol:
            return "SEC109_SHAPE", detail
        return "LASTROW_OTHER", detail
    return "DIFF", detail


def load_cases(armdir):
    """Read FZ5CASE lines out of an arm run.  Returns [(label, args)]."""
    cases = []
    seen = set()
    for fn in sorted(os.listdir(armdir)):
        if not fn.startswith("seed_") or not fn.endswith(".log"):
            continue
        seed = fn[len("seed_"):-len(".log")]
        with open(os.path.join(armdir, fn), errors="replace") as fh:
            for line in fh:
                if not line.startswith("FZ5CASE "):
                    continue
                # `FZ5CASE <idx> amort_oracle <args...>` — FOUR fields, and the
                # driver name is parts[2], NOT the head of parts[3].  Getting
                # this wrong drew ZERO screens and the sweep reported a clean
                # run over an empty corpus (R96, and R64: a run that produced no
                # output is not a zero).  Hence the fail-closed guard in main().
                parts = line.split(None, 3)
                if len(parts) < 4 or parts[2] != "amort_oracle":
                    continue
                idx, args = parts[1], parts[3].strip()
                label = "%s/%s" % (seed, idx)
                # An identical screen drawn in two seeds is TWO screens here;
                # dedup is by LABEL only, so a re-read of the same log cannot
                # double-count (round 33's instrument counted duplicates twice).
                if label in seen:
                    continue
                seen.add(label)
                cases.append((label, args))
    return cases


def load_eras(armdir):
    """Join the arm's own `FZ5VERDICT ... era=` on to each screen label.

    THE PROJECT'S HEADLINE RATE IS THE IN-SCOPE ONE.  `era=1` is horizon > 2099,
    which the client placed OUT OF SCOPE on 2026-08-03, and the fuzzer emits the
    field precisely so an instrument cannot pool them by accident
    (dos_fuzzer5_test.go:2946-2948, CAUTION 1).  The first cut of this sweep
    pooled them and published `1 in 61` over a mixed population.
    """
    eras = {}
    for fn in sorted(os.listdir(armdir)):
        if not fn.startswith("seed_") or not fn.endswith(".log"):
            continue
        seed = fn[len("seed_"):-len(".log")]
        with open(os.path.join(armdir, fn), errors="replace") as fh:
            for line in fh:
                if not line.startswith("FZ5VERDICT "):
                    continue
                f = line.split()
                rec = dict(kv.split("=", 1) for kv in f[2:] if "=" in kv)
                if "era" in rec:
                    eras["%s/%s" % (seed, f[1])] = rec["era"]
    return eras


def load_ledger(armdir):
    """The GENERATOR's own ledger, so `drawn` cannot quietly carry a shortfall.

    `FZ5CASE` lines are emitted per generated case EXCEPT for the ones the
    fuzzer skips before it gets there.  The arm's summary reports `generated`
    and `skipped-plain`; this sweep's "drawn" is `generated - skipped-plain`,
    and saying so is R5's ledger rule applied to an instrument that only
    consumes the arm rather than running it.
    """
    gen = skipped = 0
    pat_g = re.compile(r"generated (\d+)")
    pat_s = re.compile(r"skipped-plain (\d+)")
    for fn in sorted(os.listdir(armdir)):
        if not fn.startswith("seed_") or not fn.endswith(".log"):
            continue
        with open(os.path.join(armdir, fn), errors="replace") as fh:
            for line in fh:
                m = pat_g.search(line)
                if m:
                    gen += int(m.group(1))
                m = pat_s.search(line)
                if m:
                    skipped += int(m.group(1))
    return gen, skipped


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--armdir", help="dir of seed_NNNNN.log from run_arm.sh with FZ5CASEDUMP=1")
    ap.add_argument("--oracle", default="/tmp/oraclebuild/amort_oracle")
    ap.add_argument("--goamort", default="/tmp/goamort")
    ap.add_argument("--build", action="store_true",
                    help="R98: build goamort from THIS tree before sweeping")
    ap.add_argument("--tol", type=float, default=0.006,
                    help="what DIFFERS means; must be strictly ABOVE the "
                         "half-cent print tie or it resolves ties by luck")
    ap.add_argument("--jobs", type=int, default=max(1, (os.cpu_count() or 2) - 1))
    ap.add_argument("--timeout", type=int, default=120)
    ap.add_argument("--limit", type=int, default=0, help="0 = every screen")
    ap.add_argument("--out", default="", help="write per-screen JSON here")
    ap.add_argument("--control", action="store_true",
                    help="run the pinned §109 screen and assert the shape fires")
    ap.add_argument("--control-bin", default="",
                    help="binary the control runs against. MUST be one known to "
                         "CARRY the defect; defaults to --goamort, which is wrong "
                         "whenever --goamort is a FIXED tree")
    ap.add_argument("--expect-control", default="SEC109_SHAPE",
                    help="bucket the control screen must land in")
    args = ap.parse_args()

    if args.build:
        args.goamort = "/tmp/goamort_r63_sweep"
        subprocess.run(["go", "build", "-o", args.goamort, "./cmd/goamort"],
                       cwd=REPO, check=True)

    # R98, SHARPENED.  An md5 identifies a BINARY; it does not identify a
    # SUBJECT when the binary is env-gated.  Round 63's own gated sweep was
    # reproducible only by knowing GOAMORT_SEC109=1, which nothing recorded --
    # re-running it exactly as its JSON documented it reproduced the CONTROL
    # numbers under the treatment label.  Every GOAMORT_* variable in the
    # environment is therefore printed AND written to --out.
    gate_env = {k: v for k, v in os.environ.items() if k.startswith("GOAMORT_")}
    print("r63_row_sweep")
    print("  oracle   : %s  md5 %s" % (args.oracle, md5(args.oracle)))
    print("  port     : %s  md5 %s" % (args.goamort, md5(args.goamort)))
    print("  port env : %s" % (gate_env if gate_env else "<none set>"))
    print("  tolerance: %g  (numeric, both screen formats)" % args.tol)
    print("  GOAMORT_ALLROWS: set PER SCREEN, only when the oracle's own row")
    print("                   labels are FANCY (instrument defect #15 has two sides)")

    control_ok = True
    if args.control:
        cbin = args.control_bin or args.goamort
        print("\n-- POSITIVE CONTROL --")
        print("  binary: %s  md5 %s  (expecting %s)"
              % (cbin, md5(cbin), args.expect_control))
        ok = True
        for label, argstr in CONTROL_SCREENS:
            res = run_screen(argstr, args.oracle, cbin, args.timeout)
            if res["bucket"] != "OK":
                print("  %s: %s -- CONTROL COULD NOT RUN" % (label, res["bucket"]))
                ok = False
                continue
            bucket, detail = classify(res, args.tol)
            print("  %s -> %s  %s" % (label, bucket, detail))
            if bucket != args.expect_control:
                ok = False
        control_ok = ok
        print("  CONTROL %s" % ("FIRED" if ok else "*** DID NOT FIRE — THE SWEEP IS BLIND ***"))
        if not args.armdir:
            return 0 if ok else 2

    if not args.armdir:
        ap.error("--armdir is required unless --control is used alone")

    cases = load_cases(args.armdir)
    eras = load_eras(args.armdir)
    gen_total, gen_skipped = load_ledger(args.armdir)
    # FAIL CLOSED.  An empty corpus is an INSTRUMENT failure and must never be
    # printed as a sweep with nothing wrong in it.  This guard exists because
    # the first cut of this script drew 0 of 3,939 screens and said so in the
    # same shape as a clean result.
    if not cases:
        print("\n*** NO SCREENS DRAWN from %s — the corpus is empty or the "
              "FZ5CASE parse is wrong. THIS IS NOT A ZERO. ***" % args.armdir)
        return 3
    if args.limit:
        cases = cases[:args.limit]
    print("\n  generator: %s" % args.armdir)
    print("  GENERATOR LEDGER (R5): generated %d, skipped-plain %d, "
          "FZ5CASE lines %d" % (gen_total, gen_skipped, len(cases)))
    if gen_total and gen_total - gen_skipped != len(cases):
        print("  *** LEDGER DOES NOT BALANCE: %d - %d != %d ***"
              % (gen_total, gen_skipped, len(cases)))
    print("  screens drawn (= FZ5CASE lines): %d" % len(cases))
    print("  era known for: %d of them" % sum(1 for l in cases if l[0] in eras))

    results = {}
    buckets = {}
    with cf.ThreadPoolExecutor(max_workers=args.jobs) as ex:
        futs = {ex.submit(run_screen, a, args.oracle, args.goamort, args.timeout): lab
                for lab, a in cases}
        done = 0
        for fut in cf.as_completed(futs):
            lab = futs[fut]
            try:
                res = fut.result()
            except Exception as exc:                       # noqa: BLE001
                res = {"bucket": "HARNESS_ERROR", "err": repr(exc)}
            if res["bucket"] == "OK":
                bucket, detail = classify(res, args.tol)
            else:
                bucket, detail = res["bucket"], {k: v for k, v in res.items() if k != "bucket"}
            results[lab] = {"bucket": bucket, "detail": detail}
            buckets[bucket] = buckets.get(bucket, 0) + 1
            done += 1
            if done % 250 == 0:
                print("    ... %d/%d" % (done, len(cases)), flush=True)

    compared = sum(v for k, v in buckets.items()
                   if k in ("IDENTICAL", "TIE_ONLY", "DIFF", "ROWCOUNT",
                            "SEC109_SHAPE", "LASTROW_OTHER"))
    excluded = len(cases) - compared

    print("\n-- BUCKETS (unit: SCREEN) --")
    for k in sorted(buckets):
        print("  %-16s %5d" % (k, buckets[k]))
    print("\n  screens drawn        : %d" % len(cases))
    print("  screens COMPARED     : %d" % compared)
    print("  screens EXCLUDED     : %d" % excluded)
    print("    of which note #24 (goamort lacks norate/noamt): %d"
          % buckets.get("UNIMPL_NOTE24", 0))
    print("    of which DOS refused the screen (ENGINE ERROR) : %d"
          % buckets.get("ORACLE_REFUSED", 0))
    print("    of which the PORT refused but DOS did not      : %d   <-- R67 class"
          % buckets.get("PORT_REFUSED", 0))
    print("    of which no table / timeout / harness error     : %d"
          % (excluded - buckets.get("UNIMPL_NOTE24", 0)
             - buckets.get("ORACLE_REFUSED", 0)
             - buckets.get("PORT_REFUSED", 0)))
    print("  screens IDENTICAL    : %d" % buckets.get("IDENTICAL", 0))
    print("  screens TIE_ONLY     : %d   (worst row diff <= %g)"
          % (buckets.get("TIE_ONLY", 0), TIE_MAX))
    print("    ^^ NOT a defect population AND NOT a clean one. It is the two")
    print("       instruments' JOINT RESOLUTION LIMIT: the port prints 4 decimals")
    print("       and the oracle 2, so neither output can say which side of the")
    print("       half cent the raw value is on. An earlier cut of this line said")
    print("       'not a defect' -- that was a claim this instrument cannot make.")
    if compared:
        rowdiff = compared - buckets.get("IDENTICAL", 0) - buckets.get("TIE_ONLY", 0)
        print("  screens with ANY row divergence: %d  (1 in %.0f of compared)"
              % (rowdiff, (compared / rowdiff) if rowdiff else float("inf")))
        print("  of which §109 SHAPE  : %d" % buckets.get("SEC109_SHAPE", 0))

    # ---- THE IN-SCOPE RESTATEMENT.  R84 + CAUTION 1. ----
    # Two different quantities get called "the rate" and only one is the
    # project's headline:
    #   * COMPARED   -- every screen with two row tables. Pools era 0 and era 1.
    #   * IN SCOPE   -- era 0 only (horizon <= 2099). THE HEADLINE POPULATION.
    # And neither is the denominator for a claim about a DRAIN-path defect: a
    # screen with no drain row landing on very_last cannot exhibit it, so a rate
    # over "compared" measures how often the GENERATOR draws the situation, not
    # how often the port is wrong in it (R84 -- reach is not power).
    if eras:
        inscope = [k for k, v in results.items()
                   if eras.get(k) == "0" and v["bucket"] in
                   ("IDENTICAL", "TIE_ONLY", "DIFF", "ROWCOUNT",
                    "SEC109_SHAPE", "LASTROW_OTHER")]
        ib = {}
        for k in inscope:
            ib[results[k]["bucket"]] = ib.get(results[k]["bucket"], 0) + 1
        n_in = len(inscope)
        good_in = ib.get("IDENTICAL", 0) + ib.get("TIE_ONLY", 0)
        div_in = n_in - good_in
        print("\n-- IN SCOPE ONLY (era=0, horizon <= 2099) — THE HEADLINE POPULATION --")
        print("  compared, in scope   : %d   (of %d compared; %d are era=1, OUT of scope)"
              % (n_in, compared, compared - n_in))
        for k in sorted(ib):
            print("    %-16s %5d" % (k, ib[k]))
        print("  row divergence, in scope: %d  (1 in %s of in-scope compared)"
              % (div_in, ("%.0f" % (n_in / div_in)) if div_in else "-"))
        print("  §109 SHAPE, in scope    : %d" % ib.get("SEC109_SHAPE", 0))

    if args.out:
        with open(args.out, "w") as fh:
            json.dump({"oracle_md5": md5(args.oracle), "port_md5": md5(args.goamort),
                       "port_env": gate_env,
                       # R99, and the gap the audit found in R99's own artefact:
                       # gate_env is what this process INHERITED. GOAMORT_ALLROWS
                       # is set PER SCREEN by run_screen and never appears there,
                       # so it is recorded separately rather than implied.
                       "port_env_set_per_screen": {
                           "GOAMORT_ALLROWS": "1 on fancy-format screens only"}, "tol": args.tol, "tie_max": TIE_MAX,
                       "armdir": args.armdir, "generated": gen_total,
                       "skipped_plain": gen_skipped,
                       "screens": len(cases), "compared": compared,
                       "control_ok": control_ok, "eras": eras,
                       "buckets": buckets, "results": results}, fh, indent=1)
        print("\n  wrote %s" % args.out)
    # The control's verdict gates the EXIT STATUS in EVERY mode, not only when
    # --armdir is absent.  It used to be decorative in the only mode that
    # produces a sweep.
    return 0 if control_ok else 2


if __name__ == "__main__":
    sys.exit(main())
