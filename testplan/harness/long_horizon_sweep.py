#!/usr/bin/env python3
"""long_horizon_sweep.py — differential sweep over LONG-HORIZON amortization
screens, the region `dos_fuzzer5_test.go` cannot generate.

WHY THIS EXISTS
fuzzer5 draws `years := 8 + rng.Intn(18)`, so it has never produced a schedule
longer than 25 years (see claude/convergence_assessment_2026-07-31c.md §3). The
2026-07-31 assessment measured that unsampled region at 1.5% divergent inside
DOS's date range and 5.5% past its Julian ceiling, with individual screens off
by 14x — and none of it inside any convergence number the project had quoted.
This script is the standing instrument for that region.

WHAT IT DOES
Generates random screens stratified by the NOMINAL (un-wrapped) year of the last
scheduled payment, runs each through the real DOS engine (amort_oracle) and
through one or more Go builds (cmd/goamort), and reports the divergence rate per
stratum per build. With two Go binaries it also set-diffs them, so a fix can be
scored FIXED / STILL / NEW exactly like paired_regression.sh does for fuzzer5.

    A  <= 2048          what fuzzer5 can almost reach
    B  2049 - 2091      long, still inside DOS's 70000-day Julian ceiling
    C  2092 - 2155      past the Julian ceiling, inside the daterec YEAR byte
    D  > 2155           past the year byte: DOS wraps mod 256 (see §55)

Screens DOS refuses are counted separately and excluded from the rate: both
engines refusing is agreement, not a divergence, and is asserted as such.

USAGE
    testplan/harness/long_horizon_sweep.py --n 200 --seed 12 \
        --bin post=/tmp/goamort --bin pre=/tmp/goamort_pre

Requires /tmp/oraclebuild/amort_oracle (legacy/oracle/build_linux.sh).
"""

import argparse
import random
import subprocess
import sys
from collections import defaultdict

ORACLE = "/tmp/oraclebuild/amort_oracle"
PERYRS = [1, 2, 4, 6, 12, 24, 26, 52]
# Whole-month option tokens cannot be placed on a grid whose period is not a
# whole number of months, exactly as dos_fuzzer5_test.go documents for 24/26/52.
SUBMONTHLY = {24, 26, 52}


def months_per(peryr):
    return 12 // peryr if peryr in (1, 2, 3, 4, 6, 12) else 0


def add_months(y, m, d, k):
    t = (m - 1) + k
    y2 = y + t // 12
    m2 = t % 12 + 1
    dim = [31, 29 if y2 % 4 == 0 else 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][m2 - 1]
    return y2, m2, min(d, dim)


def nominal_last_year(y, m, d, peryr, n):
    """The year the last payment WOULD fall in with unbounded arithmetic. This is
    the stratification key, so it must NOT be computed the way either engine
    computes it — it is deliberately naive."""
    if peryr in (26, 52):
        return y + int((n - 1) * (365 // peryr) / 365.25)
    if peryr == 24:
        return add_months(y, m, d, (n - 1) // 2)[0]
    return add_months(y, m, d, (n - 1) * months_per(peryr))[0]


def stratum(year):
    if year <= 2048:
        return "A"
    if year <= 2091:
        return "B"
    if year <= 2155:
        return "C"
    return "D"


def gen(rng):
    peryr = rng.choice(PERYRS)
    # Draw the TERM IN YEARS wide enough to reach every stratum, then convert.
    years = rng.choice([15, 25, 40, 60, 90, 140, 200, 300, 420])
    n = max(4, years * peryr)
    if n > 20000:
        n = 20000
    amount = round(25000 + rng.random() * 475000, 2)
    rate = round(0.02 + rng.random() * 0.10, 6)
    ly = rng.randint(2020, 2030)
    lm = rng.randint(1, 12)
    ld = rng.choice([1, 15, 28, 29, 30])
    fy, fm, fd = add_months(ly, lm, ld, max(1, months_per(peryr)))
    toks = [
        "loandmy=%d.%d.%d" % (ld, lm, ly),
        "firstdmy=%d.%d.%d" % (fd, fm, fy),
    ]
    if rng.random() < 0.4:
        toks.append(rng.choice(["b365", "b365_360", ""]) or "exact")
    if peryr not in SUBMONTHLY:
        remm = (n - 1) * months_per(peryr)
        if remm > 24 and rng.random() < 0.55:
            for _ in range(rng.randint(1, 2)):
                at = rng.randint(6, max(7, min(remm - 1, 600)))
                toks.append("adj=%d:%s:" % (at, round(0.01 + rng.random() * 0.12, 4)))
        if remm > 24 and rng.random() < 0.45:
            start = rng.randint(2, max(3, min(remm - 1, 400)))
            nn = rng.randint(2, 60)
            ppy = rng.choice([1, 2, 4, 6, 12, 24, 26, 52])
            amt = round(20 + rng.random() * 800, 2)
            toks.append("pre=%d:%d:%d:%s" % (start, nn, ppy, amt))
    return amount, rate, n, peryr, toks, (fy, fm, fd)


def run(binary, argv, timeout=90):
    try:
        p = subprocess.run([binary] + argv, capture_output=True, text=True,
                           timeout=timeout)
    except subprocess.TimeoutExpired:
        return "TIMEOUT"
    out = (p.stdout or "").strip().splitlines()
    return out[0].strip() if out else "EMPTY"


def key(line):
    """Reduce a driver's first stdout line to the comparable triple, or an ERR
    marker. Values are compared as text at the drivers' own precision — this is
    a REGION rate, not a bit-fidelity instrument (see the FPC double-rounding
    note in claude/workflow_sync_to_ssk.md)."""
    if line.startswith("ERR") or line.startswith("ENGINE ERROR") or line in ("TIMEOUT", "EMPTY"):
        return ("ERR",)
    f = line.split()
    try:
        return (f[f.index("payment") + 1], f[f.index("interest") + 1], f[f.index("paid") + 1])
    except (ValueError, IndexError):
        return ("ERR",)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--n", type=int, default=200)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--bin", action="append", default=[],
                    help="NAME=PATH of a cmd/goamort build; repeatable")
    ap.add_argument("--show", type=int, default=6, help="example repros per build")
    ap.add_argument("--stratum", default="", metavar="LETTERS",
                    help="only RUN screens whose stratum is in this set, e.g. "
                         "'A' or 'AB'. Draws are still taken from the same "
                         "unfiltered stream, so a given --seed yields the same "
                         "screens with or without the filter; the filter only "
                         "skips the engine calls. Default: run everything.")
    args = ap.parse_args()
    want = set(args.stratum.upper())

    bins = []
    for spec in args.bin:
        name, _, path = spec.partition("=")
        bins.append((name, path))
    if not bins:
        bins = [("go", "/tmp/goamort")]

    rng = random.Random(args.seed)
    tot = defaultdict(int)
    dosrefused = defaultdict(int)
    diverged = defaultdict(lambda: defaultdict(int))
    repros = defaultdict(list)
    seen = {name: set() for name, _ in bins}

    for _ in range(args.n):
        amount, rate, n, peryr, toks, _f = gen(rng)
        argv = ["%.2f" % amount, "%.6f" % rate, str(n), str(peryr)] + toks
        st = stratum(nominal_last_year(2024, 1, 1, peryr, n) if False else
                     nominal_last_year(int(toks[1].split("=")[1].split(".")[2]),
                                       int(toks[1].split("=")[1].split(".")[1]),
                                       int(toks[1].split("=")[1].split(".")[0]),
                                       peryr, n))
        if want and st not in want:
            continue
        dos = key(run(ORACLE, argv))
        tot[st] += 1
        if dos == ("ERR",):
            dosrefused[st] += 1
        for name, path in bins:
            got = key(run(path, argv))
            if got != dos:
                cmd = "amort_oracle " + " ".join(argv)
                seen[name].add(cmd)
                if dos != ("ERR",):
                    diverged[name][st] += 1
                    if len(repros[name]) < args.show:
                        repros[name].append((st, cmd, dos, got))

    print("stratum  screens  DOS-refused  " + "  ".join("%-10s" % n for n, _ in bins))
    for st in ("A", "B", "C", "D"):
        if not tot[st]:
            continue
        comp = tot[st] - dosrefused[st]
        cells = []
        for name, _ in bins:
            d = diverged[name][st]
            cells.append("%-10s" % ("%d/%d (%.1f%%)" % (d, comp, 100.0 * d / comp) if comp else "-"))
        print("   %s     %4d       %4d      %s" % (st, tot[st], dosrefused[st], "  ".join(cells)))
    comp_all = sum(tot.values()) - sum(dosrefused.values())
    for name, _ in bins:
        d = sum(diverged[name].values())
        print("TOTAL %-6s %d/%d (%.2f%%)" % (name, d, comp_all,
                                             100.0 * d / comp_all if comp_all else 0))

    if len(bins) == 2:
        a, b = bins[0][0], bins[1][0]
        fixed = seen[b] - seen[a]
        new = seen[a] - seen[b]
        print("\nPAIRED (%s is the candidate, %s the baseline):" % (a, b))
        print("  FIXED %d   STILL %d   NEW %d" % (len(fixed), len(seen[a] & seen[b]), len(new)))
        for c in sorted(new)[:10]:
            print("  NEW: " + c)

    for name, _ in bins:
        if repros[name]:
            print("\nexample divergences (%s):" % name)
            for st, cmd, dos, got in repros[name]:
                print("  [%s] %s\n        DOS %s\n        Go  %s" % (st, cmd, dos, got))
    return 0


if __name__ == "__main__":
    sys.exit(main())
