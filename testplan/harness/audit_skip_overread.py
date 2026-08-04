#!/usr/bin/env python3
"""audit_skip_overread.py — the round-31 gate for the skip-months over-read.

WHAT IT GUARDS
--------------
`MonthSetFromString` (Amortize.pas:149-181) evaluates `s[length(s)+1]`, one byte
past the end of its by-value `str15` argument.  When that stack byte happens to
be a digit it is scored into the month number, the parse fails, `FirstPass`
calls `RecordError`, and `MakeTable` exits silently — no rows, no totals, no
message.  That is §65's "garbage horizon cells" HARD subclass.

`amort_oracle`'s `skip=` handler now runs the string through `PadSkipMonths`,
which two-digits every month so the lookahead lands inside the string.  This
script is the evidence that the correction is exactly that and nothing more:

  FIXED     PRE refused / POST answered        — the defect, gone
  SAME      both answered, identical output    — the negative control
  CHANGED   both answered, different output    — MUST BE 0 (R20/R21)
  BROKE     PRE answered / POST refused        — MUST BE 0
  MATCH/MISS whether goamort agrees with POST on every answered case

Usage:
  python3 testplan/harness/audit_skip_overread.py \
      --pre /tmp/preoracle/amort_oracle \
      --post /tmp/oraclebuild/amort_oracle \
      --go /tmp/goamort [--seeds 60]

Exits non-zero if CHANGED, BROKE or MISS is non-zero.
"""
import argparse
import random
import subprocess
import sys

# The six strings dos_fuzzer5_test.go actually draws (:1510), plus the shapes
# that bracket the defect: single digit, two digit, range, list, and a list whose
# LAST number is two digits (which can never over-read).
SKIPS = ["6", "1,7", "5-7", "2,8,11", "11-12", "1,3,5",
         "1", "12", "1,2", "6-8", "1-3", "2,4,6,8", "1,6,12", "3,9", "7,1"]


def run(binary, args, timeout=90):
    try:
        p = subprocess.run([binary] + args, capture_output=True, text=True,
                           timeout=timeout)
    except subprocess.TimeoutExpired:
        return "TIMEOUT"
    return (p.stdout or "").strip()


def refused(out):
    """DOS emitted nothing usable: the silent-exit signature or an ERR line."""
    return (out == "" or out.startswith("ERR") or "interest -1.00 paid -1.00" in out)


def base_screens(n, rng):
    """Monthly screens only — `skip=` is month-of-year and the fuzzer draws it
    only at perYr == 12 (dos_fuzzer5_test.go:43)."""
    out = []
    for _ in range(n):
        amt = round(rng.uniform(20_000, 400_000), 2)
        rate = round(rng.uniform(0.03, 0.14), 10)
        nper = rng.choice([60, 120, 180, 216, 240, 252, 300, 360])
        y = rng.choice([2023, 2024, 2025, 2026])
        m = rng.randint(1, 12)
        d = rng.choice([1, 2, 13, 15, 28])
        fm, fy = (m + 2, y) if m + 2 <= 12 else (m + 2 - 12, y + 1)
        args = [f"{amt:.2f}", f"{rate:.10f}", str(nper), "12",
                f"loandmy={d}.{m}.{y}", f"firstdmy={d}.{fm}.{fy}"]
        for tok, p in (("exact", .5), ("inadv", .35), ("prepaid", .3),
                       ("usa", .25), ("r78", .2), ("b365", .2), ("plusreg", .3)):
            if rng.random() < p:
                args.append(tok)
        if rng.random() < .5:
            args.append(f"mor={rng.choice([12, 21, 24, 36, 84])}")
        if rng.random() < .5:
            args.append(f"targ={round(rng.uniform(30, 400), 2)}")
        # A hard payment keeps the screen solvable without a term; `noterm` is
        # what §65's two repros use, so draw it half the time.
        args.append(f"payhard={round(amt * rng.uniform(0.006, 0.014), 2)}")
        if rng.random() < .5:
            args.append("noterm")
        out.append(args)
    return out


def selfcheck(oracle, go, seeds, seed):
    """The STANDING gate: needs only the current oracle, so it survives into
    rounds where no PRE binary exists.

    Asserts that no fuzzer-drawable skip string makes the oracle exit silently.
    If `PadSkipMonths` is removed or weakened, the four single-digit-terminated
    strings the fuzzer draws start refusing again and this fails.  Also asserts
    that every spelling of the SAME month set gives the SAME answer, which is
    the property the over-read destroys."""
    rng = random.Random(seed)
    screens = base_screens(seeds, rng)
    # (unpadded, padded) — must agree, and neither may refuse silently.
    PAIRS = [("6", "06"), ("1,7", "01,07"), ("5-7", "05-07"),
             ("1,3,5", "01,03,05"), ("2,8,11", "02,08,11"), ("11-12", "11-12")]
    silent = disagree = ok = gmiss = 0
    for scr in screens:
        for u, p in PAIRS:
            ou = run(oracle, scr + [f"skip={u}"])
            op = run(oracle, scr + [f"skip={p}"])
            if "interest -1.00 paid -1.00" in ou or "interest -1.00 paid -1.00" in op:
                silent += 1
                print(f"SILENT REFUSAL skip={u}: {' '.join(scr)}")
                continue
            if ou != op:
                disagree += 1
                print(f"SPELLING DISAGREEMENT skip={u} vs {p}\n  {' '.join(scr)}"
                      f"\n  A: {ou}\n  B: {op}")
                continue
            ok += 1
            if go and not refused(ou):
                g = run(go, scr + [f"skip={u}"])
                if g and not g.startswith("ERR") and g != "TIMEOUT":
                    if " ".join(ou.split()[:6]) != " ".join(g.split()[:6]):
                        gmiss += 1
                        print(f"GOAMORT MISS skip={u}\n  {' '.join(scr)}"
                              f"\n  oracle:  {ou}\n  goamort: {g}")
    print(f"selfcheck: {len(screens)} screens x {len(PAIRS)} pairs   "
          f"OK {ok}   SILENT {silent}   DISAGREE {disagree}"
          + (f"   GOAMORT MISS {gmiss}" if go else ""))
    bad = silent + disagree + gmiss
    print("RESULT: " + ("OK" if bad == 0 else f"FAIL ({bad})"))
    return 1 if bad else 0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--selfcheck", action="store_true",
                    help="standing gate: only needs --post (the current oracle)")
    ap.add_argument("--pre", required=False)
    ap.add_argument("--post", required=True)
    ap.add_argument("--go", default=None)
    ap.add_argument("--seeds", type=int, default=60)
    ap.add_argument("--seed", type=int, default=31031)
    ap.add_argument("-v", "--verbose", action="store_true")
    a = ap.parse_args()

    if a.selfcheck:
        return selfcheck(a.post, a.go, a.seeds, a.seed)
    if not a.pre:
        ap.error("--pre is required unless --selfcheck is given")

    rng = random.Random(a.seed)
    screens = base_screens(a.seeds, rng)

    fixed = same = changed = broke = 0
    gmatch = gmiss = gskip = 0
    examples = []

    for scr in screens:
        for s in SKIPS:
            args = scr + [f"skip={s}"]
            pre = run(a.pre, args)
            post = run(a.post, args)
            pr, po = refused(pre), refused(post)
            if pr and not po:
                fixed += 1
            elif not pr and po:
                broke += 1
                examples.append(("BROKE", s, args, pre, post))
            elif pr and po:
                same += 1        # both refuse (e.g. a genuine ERR) — no change
            elif pre == post:
                same += 1
            else:
                changed += 1
                examples.append(("CHANGED", s, args, pre, post))

            if a.go and not po:
                g = run(a.go, args)
                if g == "TIMEOUT" or g.startswith("ERR") or g == "":
                    gskip += 1
                else:
                    # Compare the payment/interest/paid triple the oracle prints;
                    # goamort does not echo the solved term.
                    pt = " ".join(post.split()[:6])
                    gt = " ".join(g.split()[:6])
                    if pt == gt:
                        gmatch += 1
                    else:
                        gmiss += 1
                        examples.append(("MISS", s, args, pt, gt))

    print(f"screens {len(screens)}  skip strings {len(SKIPS)}  "
          f"cases {len(screens) * len(SKIPS)}")
    print(f"  FIXED   (PRE refused -> POST answered) : {fixed}")
    print(f"  SAME    (identical verdict + output)   : {same}")
    print(f"  CHANGED (both answered, DIFFERENT)     : {changed}   <- must be 0")
    print(f"  BROKE   (PRE answered -> POST refused) : {broke}   <- must be 0")
    if a.go:
        print(f"  goamort MATCH {gmatch}   MISS {gmiss}   <- MISS must be 0"
              f"   (not compared: {gskip})")
    for kind, s, args, x, y in examples[:10 if not a.verbose else len(examples)]:
        print(f"\n{kind} skip={s}\n  {' '.join(args)}\n  A: {x}\n  B: {y}")

    bad = changed + broke + (gmiss if a.go else 0)
    print("\nRESULT: " + ("OK" if bad == 0 else f"FAIL ({bad} bad cases)"))
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
