#!/usr/bin/env python3
"""audit_sec65_final_balance.py — the check that retracted round 32's own signature.

WHY THIS EXISTS. Round 32 measured, over the 54 §65 divergences, that
`dInt == dPaid` to the cent in 54 of 54, and wrote it into three documents and a
commit message as *"the total principal repaid is identical, so the whole
difference is interest"* — a single-mechanism signature.

It is an ACCOUNTING IDENTITY. Two schedules that both retire the same loan differ
in `paid` by exactly what they differ in `interest`. This script asks the
question that should have been asked first — **do both schedules retire?** —
and the answer was 50 of 54 on both sides.

  ⚠️ AND IT IS STILL NOT FULLY UNDERSTOOD. The other 4 leave a RESIDUAL, and the
  residuals DIFFER (322,478 vs 323,361). That difference should break the
  identity and measurably does not. Until someone explains that, the identity is
  a loose thread in the instrument, not a finding.

IT ALSO RETIRED ROUND 31's LAST NUMBER. On those same 4 non-retiring cases DOS
leaves essentially the same residual as the port, so round 31's "9% of these
screens are the port shipping a schedule for a loan that never pays off,
residuals to $323,361" describes DOS's behaviour too, to within ~1%. That figure
was the evidence standing decision 3a.4 rested on.

THE GENERAL LESSON, filed as a trap in START_HERE §5: an n-of-n agreement looks
identical to a mechanism signature. **Before quoting one as evidence, ask what
n-of-n would look like if nothing were true.**

Input: the JSON emitted alongside audit_sec65_messagebox_probe.py's run, or any
file of `amort_oracle …` command lines (one per line).

Usage:
    python3 testplan/harness/audit_sec65_final_balance.py repros.txt \
        [--oracle /tmp/oraclebuild/amort_oracle] [--go /tmp/goamort]
"""
import argparse
import os
import subprocess
import sys

HALF_CENT = 0.005


def run(binary, args, env=None, timeout=180):
    e = dict(os.environ)
    if env:
        e.update(env)
    try:
        return subprocess.run([binary] + args, capture_output=True, text=True,
                              timeout=timeout, env=e).stdout
    except subprocess.TimeoutExpired:
        return ""


def last_balance(out):
    rows = [l for l in out.splitlines() if l.startswith("row ")]
    if not rows:
        return None
    t = rows[-1].split()
    return float(t[t.index("bal") + 1]) if "bal" in t else None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repros")
    ap.add_argument("--oracle", default="/tmp/oraclebuild/amort_oracle")
    ap.add_argument("--go", default="/tmp/goamort")
    a = ap.parse_args()

    lines = [l.strip() for l in open(a.repros) if l.strip().startswith("amort_oracle")]
    dos_zero = go_zero = both = 0
    unscored = 0
    residual = []
    for line in lines:
        args = line.split()[1:]
        db = last_balance(run(a.oracle, args + ["rows"]))
        gb = last_balance(run(a.go, args + ["rows"], {"GOAMORT_ALLROWS": "1"}))
        if db is None or gb is None:
            unscored += 1          # R12: a skip is not a pass
            continue
        d0, g0 = abs(db) < HALF_CENT, abs(gb) < HALF_CENT
        dos_zero += d0
        go_zero += g0
        both += (d0 and g0)
        if not (d0 and g0):
            residual.append((db, gb, line))

    n = len(lines) - unscored
    print(f"{len(lines)} repros, {n} scored, {unscored} UNSCORED (not agreement — R12)\n")
    print(f"  DOS final balance zero          : {dos_zero}")
    print(f"  port final balance zero         : {go_zero}")
    print(f"  BOTH retire                     : {both}")
    print(f"      -> for these, dInt == dPaid is an ACCOUNTING IDENTITY and carries")
    print(f"         no mechanism information whatsoever.")
    print(f"  neither retires / one does not   : {len(residual)}")
    for db, gb, line in residual:
        print(f"      DOS {db:>14,.2f}  port {gb:>14,.2f}   (delta {gb-db:,.2f})")
        print(f"        {line}")
    if residual:
        print("\n  -> where BOTH leave a residual and the residuals are close, DOS is")
        print("     doing the same thing the port is. A residual is not by itself a")
        print("     port defect; it is only a defect if DOS's differs.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
