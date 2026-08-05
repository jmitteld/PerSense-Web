#!/usr/bin/env python3
"""sec71_ceiling_arm.py — the JULIAN-CEILING FAMILY differential (round 35).

WHY THIS EXISTS, AND WHY paired_regression.sh CANNOT REPLACE IT
--------------------------------------------------------------
§71's defect lives on a shape `dos_fuzzer5` does not draw unaided — a prepayment
series on the 26/52 (Julian) arm of AddPeriod that is still live when the walk
crosses DOS's day-70000 ceiling, 26 August 2091.  The amortization sample-space
audit already listed that region as SILENT ("a prepayment series starting past
the entered term", docs/fuzzer_sample_space_audit_2026-08-02.md), and §71 is the
NINTH defect that audit's question has returned (§72 makes it ten).

⚠️ THIS FILE'S FIRST CUT WAS WRONG IN THREE WAYS AND ITS NUMBERS ARE SUPERSEDED.
It drew 94 impossible dates in 500 screens; its verdict scored PASS against a
completely unfixed binary; and its "termination canary" could not fire.  All
three are fixed below and each carries the comment that explains it.  Published
figures from it before the round-35 audit (FIXED 33 / STILL 122) must not be
quoted.

So the standing arms measure the fix's COLLATERAL (nothing moves elsewhere) and
this arm measures its EFFECT (the family is answered, and answered correctly).
Neither substitutes for the other.  Rule 16: a fix that changes nothing has not
been confirmed.

WHAT IT DOES
------------
For each generated screen it runs three binaries on the SAME tokens:

    PRE    the port before the fix          (--pre)
    POST   the port after  the fix          (--post)
    DOS    the compiled oracle              (--oracle)

and classifies the pair (PRE==DOS, POST==DOS) into

    FIXED    PRE wrong/absent, POST right   -> the intended effect
    STILL    both wrong                     -> not closed
    NEW      PRE right, POST wrong          -> A REGRESSION.  Blocks the fix.
    SAME_OK  both right                     -> the control population
    BOTH_ND  DOS itself declines            -> not scorable either way

⚠️ THE ORACLE NEEDS PERSENSE_ORACLE_SOFT_EMESSAGE=1 ON THIS FAMILY, and this
script sets it.  Without it the driver escalates DOS's non-fatal line-25
EMessage to a Halt and every screen here reads as a refusal — which is exactly
why round 34 could not adjudicate the answer.  DOS's EMessage has no `exit` and
no `errorflag := true` at either shipped implementation (VIDEODAT.pas:86-100,
dos_source/Globals.pas:98-104), so the gate changes what the DRIVER reports, not
what the ENGINE computes.

⚠️ THE TERMINATION SIGNAL IS `NET_FIRED_POST`, NOT `HANG` (note #29, corrected
at the round-35 audit).  Round 34's 2,000,000-iteration bound is still in the
tree, so a reintroduced frozen-date runaway does NOT hang — it returns
`ERR ... payment solve did not converge` in ~7 s, inside any sane --timeout.
Keying termination on the timeout alone would therefore be a tautology.  A
timeout IS still bucketed as HANG and still fails the arm (the bound could
itself be removed); but the signal that §71 is back is the NET FIRING.

⚠️ AND THE VERDICT ASSERTS THE EFFECT.  FIXED=0 is a FAILURE unless
--allow-zero-fixed is passed: an arm that only checks "nothing got worse" scores
PASS on a tree with the fix reverted, which is what the first cut of this file
did.

Usage:
    sec71_ceiling_arm.py --pre /tmp/goamort --post /tmp/goamort_fix \
        --oracle /tmp/oraclebuild/amort_oracle [--n 400] [--seed 71] \
        [--timeout 30]
"""

import argparse
import os
import random
import subprocess
import sys
from collections import Counter

# The Julian arm.  AddPeriod routes 26 and 52 through Julian/MDY
# (INTSUTIL.pas:1211-1216); every other peryr is field arithmetic and cannot
# reach the ceiling.  Both are drawn so the arm carries its own negative control.
JULIAN_PERYR = (26, 52)
FIELD_PERYR = (1, 2, 3, 4, 6, 12, 24)


def _dim(year, month):
    """Days in a Gregorian month — the legality test the generator owes."""
    if month == 2:
        leap = (year % 4 == 0 and year % 100 != 0) or year % 400 == 0
        return 29 if leap else 28
    return 30 if month in (4, 6, 9, 11) else 31


def build_screen(rng):
    """Draw one screen on the §71 shape.

    The axes are the ones the sample-space audit named as silent, plus the two
    that decide whether the ceiling is reached at all: the prepayment series'
    peryr (Julian arm vs field arm) and its LENGTH in days.
    """
    amount = round(rng.uniform(25_000, 500_000), 2)
    rate = round(rng.uniform(0.03, 0.14), 4)
    # A long term is what walks the schedule into the 2090s.  900 monthly
    # periods off a 2025 first payment already reaches 2100.
    nper = rng.choice([240, 480, 700, 900, 1200])
    peryr = 12
    # ⚠️ THE DAY MUST BE LEGAL IN BOTH THE LOAN MONTH AND THE FIRST-PAYMENT
    # MONTH. Round 35's first cut drew day from {1,15,17,28,29,31} and month
    # freely, so 94 of 500 screens carried an impossible date (31.4.2025).
    # cmd/goamort refuses those with os.Exit(2) and an EMPTY stdout while the
    # oracle stores the daterec verbatim and answers, so every one landed in
    # STILL and 18.8% of the sample measured the harness instead of the port.
    # AN INSTRUMENT'S ERROR CAN FLATTER THE PORT — here it did the opposite,
    # inflating STILL. Caught by the round-35 audit, not by the arm.
    month = rng.randint(1, 12)
    year = rng.choice([2024, 2025, 2026])
    nmonth = month % 12 + 1
    nyear = year + (1 if month == 12 else 0)
    day = rng.choice([1, 15, 17, 28, 29, 31])
    day = min(day, _dim(year, month), _dim(nyear, nmonth))
    # Prepayment series: start period, count, frequency, amount.
    start = rng.choice([1, 12, 60, 240, 600, 854])
    ppy = rng.choice(JULIAN_PERYR + FIELD_PERYR)
    # nn spans "retires early" through "runs centuries past the ceiling".
    nn = rng.choice([10, 52, 246, 1000, 5000, 20000])
    pamt = round(rng.uniform(1.0, 5000.0), 2)

    toks = [
        f"{amount}", f"{rate}", f"{nper}", f"{peryr}",
        "plusreg",
        f"loandmy={day}.{month}.{year}",
        f"firstdmy={day}.{nmonth}.{nyear}",
        f"pre={start}:{nn}:{ppy}:{pamt}",
    ]
    if rng.random() < 0.3:
        toks.append("exact")
    if rng.random() < 0.3:
        toks.append("usa")
    if rng.random() < 0.25:
        toks.append("b365")
    return toks, ppy


def run(binary, toks, timeout, env=None):
    """Run one binary on one screen.  Returns (kind, payload).

    kind is 'ok' (payload = the normalised answer triple), 'err' (payload = the
    message), or 'hang' (payload = None).  A hang is never folded into 'err':
    note #29 exists because a killed process reads as success.
    """
    e = dict(os.environ)
    if env:
        e.update(env)
    try:
        p = subprocess.run([binary] + toks, capture_output=True, text=True,
                           timeout=timeout, env=e)
    except subprocess.TimeoutExpired:
        return "hang", None
    out = (p.stdout or "").strip()
    if not out:
        # cmd/goamort refuses a malformed screen with os.Exit(2) and writes to
        # STDERR, so an empty stdout is a HARNESS refusal, not an engine answer.
        # Bucketed separately so it can never be miscounted as a divergence.
        return "badscreen", (p.stderr or "").strip()[:120]
    last = out.splitlines()[-1].strip()
    if last.startswith("ERR"):
        # ⚠️ ROUND 34's ITERATION BOUND IS STILL IN THE TREE, and it converts a
        # frozen-date runaway into exactly this string after ~5-7 s. So a
        # `--timeout` HANG is NOT the signature of a reintroduced §71; the net
        # firing is. Distinguish them, or "POST hangs 0" is a tautology — the
        # bound guarantees it. (Round-35 audit finding.)
        if "did not converge" in last:
            return "net", last
        return "err", last
    # Both binaries print `payment X interest Y paid Z` (goamort uses the same
    # trailing form as the oracle).  Compare interest and paid only: the payment
    # echo is a heuristic in cmd/goamort and is never scored (standing trap).
    parts = last.split()
    vals = {}
    for i, w in enumerate(parts):
        if w in ("interest", "paid") and i + 1 < len(parts):
            try:
                vals[w] = float(parts[i + 1])
            except ValueError:
                return "err", last
    if "interest" not in vals or "paid" not in vals:
        return "err", last
    return "ok", (round(vals["interest"], 2), round(vals["paid"], 2))


def agrees(a, b, tol=0.02):
    if a[0] != "ok" or b[0] != "ok":
        return False
    return (abs(a[1][0] - b[1][0]) <= tol) and (abs(a[1][1] - b[1][1]) <= tol)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pre", required=True)
    ap.add_argument("--post", required=True)
    ap.add_argument("--oracle", required=True)
    ap.add_argument("--n", type=int, default=400)
    ap.add_argument("--seed", type=int, default=71)
    ap.add_argument("--timeout", type=int, default=30)
    ap.add_argument("--verbose", action="store_true")
    ap.add_argument("--allow-zero-fixed", action="store_true",
                    help="do not FAIL when FIXED=0 (use only when a "
                         "zero is the expected result)")
    args = ap.parse_args()

    rng = random.Random(args.seed)
    soft = {"PERSENSE_ORACLE_SOFT_EMESSAGE": "1"}

    tally = Counter()
    by_arm = {"julian": Counter(), "field": Counter()}
    news, fixes, hangs, badscreens, nets = [], [], [], [], []

    for i in range(args.n):
        toks, ppy = build_screen(rng)
        arm = "julian" if ppy in JULIAN_PERYR else "field"

        dos = run(args.oracle, toks, args.timeout, soft)
        pre = run(args.pre, toks, args.timeout)
        post = run(args.post, toks, args.timeout)

        if pre[0] == "hang" or post[0] == "hang" or dos[0] == "hang":
            who = [n for n, r in (("PRE", pre), ("POST", post), ("DOS", dos))
                   if r[0] == "hang"]
            tally["HANG_" + "+".join(who)] += 1
            by_arm[arm]["HANG"] += 1
            hangs.append((who, toks))
            continue

        # A screen cmd/goamort refuses as malformed is measurable by neither
        # column.  Counted and reported, never scored.
        if pre[0] == "badscreen" or post[0] == "badscreen":
            tally["BADSCREEN"] += 1
            by_arm[arm]["BADSCREEN"] += 1
            badscreens.append(toks)
            continue

        # ⚠️ ORDER MATTERS, AND THE FIRST CUT GOT IT WRONG. The port emits
        # `ERR amortization: payment solve did not converge` for TWO different
        # reasons: round 34's iteration bound aborting a frozen-date runaway,
        # and its Newton simply giving up — which is DOS's own
        # `Computation of payment amount or interest rate did not converge.`
        # The string cannot tell them apart. What CAN is whether DOS answered.
        # Checking the net BEFORE the oracle's verdict flagged three screens on
        # which PRE, POST and DOS all decline identically, i.e. perfect
        # agreement, as "§71 is back". A HARNESS SUSPECT BEFORE THE ENGINE
        # (rule 12) — in the direction that manufactures a beautiful finding.
        if dos[0] != "ok":
            tally["BOTH_ND"] += 1
            by_arm[arm]["BOTH_ND"] += 1
            continue

        # DOS answered and POST did not converge. That is the signature worth
        # flagging: either round 34's net firing on a reintroduced runaway, or a
        # solver gap. It is ALSO scored below as STILL or NEW — this is a
        # spotlight, not a separate bucket, so nothing is double-counted.
        if post[0] == "net":
            tally["POST_NOCONV_DOS_ANSWERED"] += 1
            by_arm[arm]["POST_NOCONV_DOS_ANSWERED"] += 1
            nets.append((toks, pre[0]))

        pok, sok = agrees(pre, dos), agrees(post, dos)
        if pok and sok:
            bucket = "SAME_OK"
        elif not pok and sok:
            bucket = "FIXED"
            fixes.append((toks, pre, post, dos))
        elif pok and not sok:
            bucket = "NEW"
            news.append((toks, pre, post, dos))
        else:
            bucket = "STILL"
        tally[bucket] += 1
        by_arm[arm][bucket] += 1

    print("=== §71 JULIAN-CEILING ARM ===")
    print(f"screens: {args.n}  seed: {args.seed}  timeout: {args.timeout}s")
    print(f"pre:    {args.pre}")
    print(f"post:   {args.post}")
    print(f"oracle: {args.oracle}  (PERSENSE_ORACLE_SOFT_EMESSAGE=1)")
    print()
    for k in sorted(tally):
        print(f"  {k:24s} {tally[k]:6d}")
    print()
    print("  by prepayment arm (julian = peryr 26/52, the only arm that can overflow):")
    for arm in ("julian", "field"):
        row = "  ".join(f"{k}={v}" for k, v in sorted(by_arm[arm].items()))
        print(f"    {arm:8s} {row}")
    print()

    if hangs:
        print(f"!!! {len(hangs)} HANG(S) — note #29: a killed run is never a pass")
        for who, toks in hangs[:10]:
            print("    " + "+".join(who) + ": " + " ".join(toks))
        print()
    if news:
        print(f"!!! {len(news)} NEW REGRESSION(S) — the fix moved a correct answer")
        for toks, pre, post, dos in news[:20]:
            print("    " + " ".join(toks))
            print(f"      pre={pre[1]} post={post[1]} dos={dos[1]}")
        print()
    if args.verbose and fixes:
        print(f"FIXED sample ({len(fixes)} total):")
        for toks, pre, post, dos in fixes[:20]:
            print("    " + " ".join(toks))
            print(f"      pre={pre[0]}:{pre[1]} post={post[1]} dos={dos[1]}")

    newNets = [t for t, prekind in nets if prekind == "ok"]
    if nets:
        print(f"note: {len(nets)} screen(s) where DOS ANSWERED and POST did not "
              f"converge ({len(newNets)} of them a REGRESSION — PRE answered)")
        for toks, prekind in nets[:10]:
            print(f"    [pre={prekind}] " + " ".join(toks))
        print()
    if badscreens:
        print(f"note: {len(badscreens)} screen(s) refused by cmd/goamort as "
              f"malformed and NOT scored (see BADSCREEN)")
        print()

    # ⚠️ THE VERDICT ASSERTS THE EFFECT, NOT ONLY THE ABSENCE OF HARM. Round
    # 35's first cut keyed FAIL on NEW and POST hangs alone, so a tree with the
    # fix entirely REVERTED scored PASS: a lost fix shows up as STILL, and STILL
    # did not reach the exit code.  An instrument written to confirm an effect
    # (rule 16) must fail when the effect is gone.  Demonstrated at the round-35
    # audit: --post = the unfixed binary printed "VERDICT: PASS".
    postHangs = sum(1 for w, _ in hangs if "POST" in w)
    reasons = []
    if news:
        reasons.append(f"NEW={len(news)}")
    if postHangs:
        reasons.append(f"POST hangs={postHangs}")
    if newNets:
        reasons.append(f"POST stopped converging where PRE and DOS both "
                       f"answered={len(newNets)}")
    if tally["FIXED"] == 0 and not args.allow_zero_fixed:
        reasons.append("FIXED=0 — this arm exists to measure an EFFECT; zero "
                       "means the fix is absent from --post, or the generator "
                       "no longer reaches the family (pass --allow-zero-fixed "
                       "if a zero is genuinely expected)")
    print(f"VERDICT: {'FAIL' if reasons else 'PASS'}"
          + (" (" + "; ".join(reasons) + ")" if reasons
             else f" (FIXED={tally['FIXED']}, NEW=0, POST hangs=0)"))
    return 1 if reasons else 0


if __name__ == "__main__":
    sys.exit(main())
