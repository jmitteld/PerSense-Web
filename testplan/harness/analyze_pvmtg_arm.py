#!/usr/bin/env python3
"""analyze_pvmtg_arm.py — pool the PV / MORTGAGE fuzzer5 counters over an arm.

The counterpart of `analyze_arm.py` / `era_split_arm.py` for the two surfaces
that had no seed-looper and no analyzer at all until round 29. See
`run_pvmtg_arm.sh` for why this exists.

R12 / R13 — an instrument may print only what it has READ, and a skip is not a
pass. A seed whose log carries NO counter line is COUNTED AND NAMED, never
silently dropped: a killed binary reports nothing, and nothing looks like
success (round 19). The per-seed `.rc` files `run_pvmtg_arm.sh` writes are read
too, so a non-zero exit is its own reported bucket rather than an absence.

Rule 9 — the pooled figures are labelled with their UNIT (worksheets vs table
LINES vs eval CASES vs APR VERDICTS) because mixing them is exactly the mistake
that rule exists to prevent.

Usage:
    python3 testplan/harness/analyze_pvmtg_arm.py pv  /tmp/r29/pv
    python3 testplan/harness/analyze_pvmtg_arm.py mtg /tmp/r29/mtg
"""
import glob
import os
import re
import sys

PV = re.compile(
    r"pv fuzzer5 \(all advanced options, [\d.]+% omit\): (\d+) table worksheets "
    r"\((\d+) lines diffed\), (\d+) variable-rate worksheets \((\d+) row PVs diffed\), "
    r"(\d+) both-refused, (\d+) oracle flakes, (\d+) divergences")

MTG_EVAL = re.compile(
    r"eval checked=(\d+) both-refused=(\d+) divergences=(\d+) oracle-faults=(\d+)")
MTG_CONV = re.compile(
    r"APR convergence verdict: checked=(\d+) divergences=(\d+) "
    r"both-nonconverged=(\d+) ulp-noise=(\d+)")
MTG_CASES = re.compile(r"mtg fuzzer5 seed=(\d+) cases=(\d+) ")


def rc_of(path):
    p = path[:-4] + ".rc"
    if not os.path.exists(p):
        return None
    try:
        return int(open(p).read().strip())
    except ValueError:
        return None


def main():
    if len(sys.argv) < 3:
        print(__doc__)
        return 2
    surface, d = sys.argv[1], sys.argv[2]
    logs = sorted(glob.glob(os.path.join(d, "seed_*.log")))
    if not logs:
        print(f"no seed logs under {d}")
        return 2

    missing, badrc = [], []
    tot = {}

    def add(k, v):
        tot[k] = tot.get(k, 0) + v

    for f in logs:
        txt = open(f, errors="replace").read()
        rc = rc_of(f)
        if rc not in (0, None):
            badrc.append((os.path.basename(f), rc))
        if surface == "pv":
            m = PV.search(txt)
            if not m:
                missing.append(os.path.basename(f))
                continue
            g = [int(x) for x in m.groups()]
            for k, v in zip(("worksheets_table", "lines", "worksheets_vr",
                             "row_pvs", "both_refused", "flakes", "divergences"), g):
                add(k, v)
        else:
            me, mc, mk = MTG_EVAL.search(txt), MTG_CONV.search(txt), MTG_CASES.search(txt)
            if not (me and mc and mk):
                missing.append(os.path.basename(f))
                continue
            add("cases", int(mk.group(2)))
            for k, v in zip(("eval_checked", "eval_both_refused", "eval_divergences",
                             "eval_oracle_faults"), [int(x) for x in me.groups()]):
                add(k, v)
            for k, v in zip(("apr_verdicts", "apr_divergences", "apr_both_nonconv",
                             "apr_ulp_noise"), [int(x) for x in mc.groups()]):
                add(k, v)

    print(f"\n===== {surface.upper()}  {d} =====")
    print(f"  seeds {len(logs)}   emitted counters {len(logs) - len(missing)}")
    if surface == "pv":
        print(f"  WORKSHEETS  table {tot.get('worksheets_table',0):,} + "
              f"variable-rate {tot.get('worksheets_vr',0):,} = "
              f"{tot.get('worksheets_table',0)+tot.get('worksheets_vr',0):,}")
        print(f"  TABLE LINES diffed   {tot.get('lines',0):,}")
        print(f"  VR ROW PVs  diffed   {tot.get('row_pvs',0):,}")
        print(f"  both-refused {tot.get('both_refused',0):,}   "
              f"oracle flakes {tot.get('flakes',0):,}")
        print(f"  DIVERGENCES          {tot.get('divergences',0):,}")
    else:
        print(f"  CASES generated      {tot.get('cases',0):,}")
        print(f"  EVAL CASES checked   {tot.get('eval_checked',0):,}   "
              f"both-refused {tot.get('eval_both_refused',0):,}   "
              f"oracle-faults {tot.get('eval_oracle_faults',0):,}")
        print(f"  EVAL DIVERGENCES     {tot.get('eval_divergences',0):,}")
        print(f"  APR VERDICTS checked {tot.get('apr_verdicts',0):,}   "
              f"both-nonconverged {tot.get('apr_both_nonconv',0):,}   "
              f"ulp-noise {tot.get('apr_ulp_noise',0):,}")
        print(f"  APR DIVERGENCES      {tot.get('apr_divergences',0):,}")

    print("\nUNITS (rule 9): worksheets/cases, table LINES, row PVs and APR VERDICTS "
          "are four different populations. Never pool two of them into one rate.")
    if badrc:
        print(f"  ⚠️ NON-ZERO EXIT STATUS: {badrc}")
    if missing:
        print(f"  ⚠️ PARTIAL — {len(missing)} seed(s) emitted no counter line: {missing}")
        print("  VERDICT: PARTIAL — do not quote these totals without saying so.")
    else:
        print("  VERDICT: COMPLETE — every seed contributed its counters.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
