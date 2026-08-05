#!/usr/bin/env python3
"""Localise the FIRST divergent row between DOS (amort_oracle dumpraw) and the
port (goamort rows), for the §3b-item-1 root cause of round 32's 475.

WHY A DEDICATED SCRIPT (round 33).  The signal `HARD:divergent_class` is a
whole-case TOTALS verdict: it says the two schedules ended up in different
places, and says nothing about WHERE they parted.  Round 32's attribution work
read totals and final balances; this reads ROWS, aligned on DATE, and prints the
first index at which any cell disagrees by more than half a cent.

ALIGNMENT IS BY DATE, NOT BY INDEX (defect #15).  DOS interleaves announcement
lines ("--->On 1/10/30, re-computed at ...") and summary/separator lines into the
same numbered stream; the port emits only payment rows.  Comparing L[i] to
row[i] manufactures an off-by-one at the first announcement.  Dates repeat
(prepayment sub-rows inside one payment number), so the alignment is a
POSITIONAL walk over the two PAYMENT-ROW-ONLY sequences, with the date carried
alongside as a cross-check: if the two sequences ever disagree on the date at
the same ordinal, that is itself reported and is a stronger finding than a cell
difference.

ROW COUNTS DIFFER ON SOME CASES (5 of 54 in round 32), so a length mismatch is
NOT an error -- it is reported, and the comparison runs to the shorter length.

USAGE
    python3 testplan/harness/localise_divergent_row.py repros.txt \
        [--oracle /tmp/oraclebuild/amort_oracle] [--go /tmp/goamort] \
        [--limit N] [-v]

NOTE #24 APPLIES.  goamort implements neither `norate` nor `noamt`; this script
SKIPS those repros loudly and prints the excluded count, rather than comparing a
screen the two drivers do not agree on the meaning of.  A skip is not a pass
(R12), so the skipped list is printed in full.
"""
import argparse
import os
import re
import subprocess
import sys

HALF_CENT = 0.005

# DOS: "L12| 1/10/35 22281.41 ..." -- BUT the day is SPACE-PADDED to width 2, so
# the real shape is "L0|12/ 6/25 5016.14 ...".  A `\d+/\d+/\d+` date pattern
# matches NEITHER that nor "L3| 4/ 6/27", and the whole case then reports zero
# DOS rows.  That cost 12 of 56 repros on this script's first run and looked
# exactly like an oracle refusal.  (Defect #14 is the same hazard in the `rows`
# subcommand, where the pad is TRUNCATED instead of printed.)
DOS_ROW = re.compile(
    r"^L\d+\|\s*(\d+\s*/\s*\d+\s*/\s*\d+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s*$"
)
# port: "row 10 2/10/35 pay 257.35 int 967.1700 prin -709.8200 bal 84792.5500"
GO_ROW = re.compile(
    r"^row\s+\d+\s+(\d+\s*/\s*\d+\s*/\s*\d+)\s+pay\s+(-?[\d.]+)\s+int\s+(-?[\d.]+)\s+prin\s+(-?[\d.]+)\s+bal\s+(-?[\d.]+)\s*$"
)

CELLS = ("pay", "int", "prin", "bal")


def norm_date(s):
    """Canonicalise M/D/YY: strip the width-2 space padding both sides use."""
    return "/".join(p.strip().lstrip("0") or "0" for p in s.split("/"))


def run(binary, args, env=None, timeout=180):
    e = dict(os.environ)
    if env:
        e.update(env)
    try:
        p = subprocess.run([binary] + args, capture_output=True, text=True,
                           timeout=timeout, env=e)
    except subprocess.TimeoutExpired:
        return None, "TIMEOUT"
    return p.stdout, None


def dos_rows(oracle, args):
    out, err = run(oracle, args + ["dumpraw"])
    if err:
        return None, err
    rows = []
    for line in out.splitlines():
        m = DOS_ROW.match(line.strip())
        if m:
            rows.append((norm_date(m.group(1)),
                         [float(m.group(i)) for i in (2, 3, 4, 5)]))
    if not rows:
        return None, "NO_DOS_ROWS"
    return rows, None


def go_rows(go, args):
    out, err = run(go, args + ["rows"],
                   env={"GOAMORT_ALLROWS": "1", "GOAMORT_ROWDATES": "1"})
    if err:
        return None, err
    rows = []
    for line in out.splitlines():
        m = GO_ROW.match(line.strip())
        if m:
            rows.append((norm_date(m.group(1)),
                         [float(m.group(i)) for i in (2, 3, 4, 5)]))
    if not rows:
        return None, "NO_GO_ROWS"
    return rows, None


def first_divergence(dr, gr):
    """Return (ordinal, kind, detail) of the first disagreement, or None."""
    n = min(len(dr), len(gr))
    for i in range(n):
        ddate, dcells = dr[i]
        gdate, gcells = gr[i]
        if ddate != gdate:
            return (i, "DATE", f"dos={ddate} port={gdate}")
        for k, name in enumerate(CELLS):
            if abs(dcells[k] - gcells[k]) > HALF_CENT:
                return (i, name,
                        f"date={ddate} dos={dcells[k]:.2f} port={gcells[k]:.2f} "
                        f"delta={gcells[k] - dcells[k]:+.2f}")
    if len(dr) != len(gr):
        return (n, "LENGTH", f"dos={len(dr)} port={len(gr)}")
    return None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repros")
    ap.add_argument("--oracle", default="/tmp/oraclebuild/amort_oracle")
    ap.add_argument("--go", default="/tmp/goamort")
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("-v", action="store_true")
    a = ap.parse_args()

    lines = [l.strip() for l in open(a.repros) if l.strip()]
    cases, skipped = [], []
    for l in lines:
        toks = l.split()
        if toks and toks[0].endswith("amort_oracle"):
            toks = toks[1:]
        if "norate" in toks or "noamt" in toks:
            skipped.append(toks)
            continue
        cases.append(toks)
    if a.limit:
        cases = cases[:a.limit]

    print(f"repros={len(lines)} comparable={len(cases)} "
          f"SKIPPED_note24_norate_noamt={len(skipped)}")
    for s in skipped:
        print("  SKIP(note#24) amort_oracle " + " ".join(s))
    print()

    hist = {}
    errs = 0
    for idx, toks in enumerate(cases):
        dr, e1 = dos_rows(a.oracle, toks)
        gr, e2 = go_rows(a.go, toks)
        if e1 or e2:
            errs += 1
            print(f"[{idx}] ERROR dos={e1} port={e2}  amort_oracle {' '.join(toks)}")
            continue
        div = first_divergence(dr, gr)
        if div is None:
            print(f"[{idx}] AGREE rows={len(dr)}")
            hist["AGREE"] = hist.get("AGREE", 0) + 1
            continue
        i, kind, detail = div
        hist[kind] = hist.get(kind, 0) + 1
        frac = i / max(1, len(dr))
        print(f"[{idx}] FIRSTDIV row={i}/{len(dr)} ({frac:.0%}) kind={kind} {detail}")
        if a.v:
            print(f"      amort_oracle {' '.join(toks)}")
            lo, hi = max(0, i - 2), min(len(dr), i + 3)
            for j in range(lo, hi):
                dd, dc = dr[j]
                gd, gc = (gr[j] if j < len(gr) else ("--", [0, 0, 0, 0]))
                mark = "<<<" if j == i else "   "
                print(f"      {mark} [{j}] DOS  {dd} " +
                      " ".join(f"{x:12.2f}" for x in dc))
                print(f"      {mark} [{j}] PORT {gd} " +
                      " ".join(f"{x:12.2f}" for x in gc))

    print()
    print("=== first-divergence kind histogram ===")
    for k in sorted(hist, key=lambda x: -hist[x]):
        print(f"  {k:8s} {hist[k]}")
    print(f"  ERROR    {errs}")


if __name__ == "__main__":
    main()
