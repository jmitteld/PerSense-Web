#!/usr/bin/env python3
"""Phase 2 localisation for the seven in-scope HARD cases — round 24.

Rebuilt from claude/phase2_attribution_the_seven_2026-08-04.md §7 because
/tmp/r22/attribute.sh was never synced.

THE POINT OF THIS REWRITE: read row DATES from `dumpraw`, not from `rows`.
`amort_oracle`'s rows mode takes the date as GetTok(line,1) — the first
whitespace token — and DOS pads a single-digit day, so `12/ 9/30` comes out as
`12/` (instrument defect #14, docs/harness_policy.md). The row VALUES in rows
mode are taken from the END of the line and are fine.

Self-check built in (R13 / rule 12: the harness is a suspect before the engine):
this script parses dumpraw itself, replicating amort_oracle's IsDetailLine
(amort_oracle.pas:550-566), and then asserts its own int/prin/bal sequence is
byte-identical to what rows mode emits for the same screen. If the two disagree
the line filter is wrong and no localisation from this script may be believed.
"""

import os
import re
import subprocess
import sys

ORACLE = "/tmp/oraclebuild/amort_oracle"
GOAMORT = "/tmp/goamort"

CASES = {
    "c1": "143088.37 0.0740180000 120 6 exact prepaid plusreg r78 "
          "loandmy=19.8.2025 firstdmy=19.11.2025 mor=107 b149=25770.24 b183=30832.49 "
          "b191=29250.29 pre=143:99:12:184.48 pre=137:80:24:114.55 adj=171::2194.26",
    "c2": "77496.89 0.0473990000 19 1 b365 prepaid r78 usa "
          "loandmy=10.9.2025 firstdmy=10.11.2025 mor=2 b14=17529.77 b62=13636.25 "
          "pre=122:15:4:309.49 adj=26:0.1299630000:5682.71 adj=86:0.1238060000:4812.50 "
          "adj=134:0.0505390000:8827.81 targ=267.22 pts=0.007932 payhard=7582.46 noterm",
    "c3": "393752.15 0.0477520000 26 2 prepaid usa loandmy=29.4.2023 "
          "firstdmy=29.2.2024 mor=70 b94=82687.19 b106=93767.40 b118=59796.70 "
          "pre=10:89:6:507.99 adj=34:0.0762230000: adj=112:0.0437960000:15897.68 "
          "targ=3910.14 pts=0.030192 payhard=26754.42 non lastdmy=29.8.2036",
    "c4": "284917.49 0.0671720000 28 2 b365_360 exact prepaid plusreg r78 "
          "loandmy=31.7.2023 firstdmy=31.8.2023 mor=73 pre=55:144:12:323.93 "
          "adj=103::22916.18 pts=0.005528 payhard=20219.51 non lastdmy=28.2.2037",
    "c5": "478549.13 0.0885940000 114 6 r78 loandmy=28.1.2023 "
          "firstdmy=28.2.2023 mor=99 b121=26805.93 b143=98767.29 b157=86876.72 "
          "pre=41:38:2:1794.63 pre=93:251:26:126.42 adj=161::12287.25 targ=1449.49 "
          "pts=0.012427 payhard=11111.10 non lastdmy=28.12.2041",
    "c6": "294350.23 0.1390570000 312 24 b365_360 plusreg r78 "
          "loandmy=31.8.2025 firstdmy=31.10.2025 targ=503.52 pts=0.034335 "
          "payhard=2477.43 non lastdmy=30.9.2051",
    "c7": "470769.21 0.0664250000 51 3 usa loandmy=26.10.2023 "
          "firstdmy=26.6.2024 mor=64 pre=144:1:4:886.30 pre=56:357:26:76.07 "
          "adj=44:0.1348970000:20671.34 adj=100:0.0890860000: adj=132::12286.30 "
          "targ=3154.45",
}


def toks(s):
    return s.split()


def is_float(t):
    try:
        float(t)
        return True
    except ValueError:
        return False


def is_pos_int(t):
    return t.isdigit() and int(t) > 0


def is_detail_line(s):
    """Replicates amort_oracle.pas:550-566 IsDetailLine."""
    tk = toks(s)
    if len(tk) < 6:
        return False
    if "Total" in s:
        return False
    stripped = s.lstrip(" ")
    if stripped.startswith("-"):
        return False
    if not is_float(tk[-1]):
        return False
    t1 = tk[0]
    return is_pos_int(t1) or ("/" in t1)


def run(cmd_args, env=None):
    e = dict(os.environ)
    if env:
        e.update(env)
    p = subprocess.run(cmd_args, capture_output=True, text=True, timeout=300, env=e)
    return p.stdout, p.stderr, p.returncode


def dos_dumpraw(args):
    """Returns (payment, [ (date_or_None, pay, interest, prin, bal) ]) from dumpraw."""
    out, err, rc = run([ORACLE] + toks(args) + ["dumpraw"])
    rows = []
    payment = None
    for line in out.splitlines():
        if line.startswith("payment "):
            payment = float(line.split()[1])
            continue
        if not line.startswith("L") or "|" not in line:
            continue
        body = line.split("|", 1)[1]
        if not is_detail_line(body):
            continue
        tk = toks(body)
        interest, prin, bal = float(tk[-4]), float(tk[-3]), float(tk[-2])
        pay = float(tk[-5]) if is_float(tk[-5]) else None
        # Date: everything before the numeric run at the end. DOS's fancy format
        # leads with the date; the padded-day case splits it into two tokens.
        # Ordinary format leads with a paynum instead and carries no date.
        if "/" in tk[0]:
            # join leading tokens until the date has two slashes
            date = tk[0]
            i = 1
            while date.count("/") < 2 and i < len(tk):
                date += tk[i]
                i += 1
            date = date.replace(" ", "")
        else:
            date = None
        rows.append((date, interest, prin, bal, pay))
    return payment, rows, err, rc


def dos_rows(args):
    """Returns (payment, [(int,prin,bal)]) from rows mode — values only."""
    out, err, rc = run([ORACLE] + toks(args) + ["rows"])
    rows = []
    payment = None
    for line in out.splitlines():
        if line.startswith("payment "):
            payment = float(line.split()[1])
        elif line.startswith("row "):
            tk = line.split()
            # row <datetok> int X prin Y bal Z
            d = {}
            for k in ("int", "prin", "bal"):
                d[k] = float(tk[tk.index(k) + 1])
            rows.append((d["int"], d["prin"], d["bal"]))
    return payment, rows, err, rc


PORT_ROW = re.compile(
    r"^row (\d+) (\d+)/(\d+)/(\d+) pay ([-\d.]+) int ([-\d.]+) prin ([-\d.]+) bal ([-\d.]+)$")


def port_rows(args, allrows):
    env = {"GOAMORT_ROWDATES": "1"}
    if allrows:
        env["GOAMORT_ALLROWS"] = "1"
    out, err, rc = run([GOAMORT] + toks(args) + ["rows"], env=env)
    rows = []
    payment = None
    for line in out.splitlines():
        if line.startswith("payment "):
            payment = float(line.split()[1])
            continue
        m = PORT_ROW.match(line)
        if m:
            n, mo, da, yy, pay, i, p, b = m.groups()
            rows.append(("%s/%s/%s" % (mo, da, yy), float(i), float(p), float(b), float(pay)))
    return payment, rows, err, rc


def norm_date(d):
    """m/d/yy with no padding, year mod 100 — the comparable shape."""
    if d is None:
        return None
    parts = d.split("/")
    if len(parts) != 3:
        return d
    m, dd, y = (p.strip() for p in parts)
    try:
        return "%d/%d/%02d" % (int(m), int(dd), int(y) % 100)
    except ValueError:
        return d


# ---------------------------------------------------------------------------
# THE REGRESSION GATE (round 25).
#
# `--assert` turns this differential into a pass/fail gate over the seven
# in-scope HARD cases. EXPECT[case] is the 1-based index of the first row whose
# int/prin/bal/pay gap exceeds TWO CENTS — above the rendering-tie floor, which
# a per-row comparison against DOS's printed cents can never see past
# (amort_oracle.pas:1186-1191) — or None for "no material divergence anywhere".
#
# Every non-None entry is a KNOWN, NAMED class, and the index is the assertion:
# if a divergence moves EARLIER the gate fails, and if one appears where None is
# recorded the gate fails.
#
#   c3  17   §66's AO7 arm — Re_Amortize's blank-AMOUNT branch, still OPEN. DOS
#            solves payment -21236.435395 after NumberOfInstallments snaps its
#            lookahead 2026-04-29 forward to 2026-08-31 through the VAR
#            parameter (AMORTOP.pas:1547). See docs/discrepancies.md §66.
#   c4  163  the LAST row of 163 — §63's terminating-balloon final row, DECIDED
#            2026-08-04 (match DOS) but not yet implemented, so still a gap.
#   c5  366  the LAST row of 366 — §63 again.
#   c6  2    §67, the semi-monthly first-payment-on-the-31st date grid.
#
# c1, c2 and c7 carry None: §66's AO6 fix (round 25) removed the last material
# divergence from all three. Before that fix they read 138, None and 198, and
# c1/c7 also differed in ROW COUNT (211/213 and 361/370) — a schedule run at a
# different rate retires on a different date.
#
# VERIFIED BOTH DIRECTIONS, IN FACT (standing rule 3), 2026-08-04 — see the
# round 25 write-up for the recorded output of both runs.
EXPECT = {
    "c1": None,
    "c2": None,
    "c3": 17,
    "c4": 163,
    "c5": 366,
    # c6 was 2 until round 26b FIXED §67: the fancy walk seeded row 1 from the
    # typed FirstDate instead of round-tripping it through AddPeriod the way
    # RepayFancyLoan does (AMORTOP.pas:1148-1150 -> 1165). That round trip is the
    # identity everywhere except AddPeriod's peryr=24 branch, the only one with a
    # `d >= 31` rule (INTSUTIL.pas:1216-1237). Post-fix c6 is 206/206 rows with
    # no date divergence and no >2c divergence. Verified BOTH directions.
    "c6": None,
    "c7": None,
}


def main():
    cliargs = [a for a in sys.argv[1:] if a != "--assert"]
    asserting = "--assert" in sys.argv[1:]
    which = cliargs or sorted(CASES)
    failures = []
    for name in which:
        args = CASES[name]
        print("=" * 78)
        print(name)
        dpay, draws, derr, drc = dos_dumpraw(args)
        rpay, rrows, rerr, rrc = dos_rows(args)
        # DOS's FANCY (date-leading) format cannot exclude the settlement row —
        # IsDetailLine has no payment number to test — so the port must emit its
        # own settlement row to keep the two row sets index-aligned. Instrument
        # defect #15 (round 24). On the ORDINARY format DOS does exclude it and
        # the port must too.
        fancy = any(d is not None for (d, *_r) in draws)
        ppay, prows, perr, prc = port_rows(args, allrows=fancy)

        # --- self-check: my dumpraw line filter must reproduce rows mode exactly
        mine = [(i, p, b) for (_d, i, p, b, _pay) in draws]
        if mine != rrows:
            print("  !! SELF-CHECK FAILED: dumpraw parse (%d rows) != rows mode (%d rows)"
                  % (len(mine), len(rrows)))
            for k in range(min(len(mine), len(rrows))):
                if mine[k] != rrows[k]:
                    print("     first divergence at index %d: dumpraw %s  rows %s"
                          % (k, mine[k], rrows[k]))
                    break
            print("  localisation from this case is NOT trustworthy; skipping")
            # R12: a skip is not a pass. Under --assert a failed self-check is a
            # FAILURE, not a quiet continue — the instrument is the suspect and
            # an unusable instrument cannot clear a gate.
            failures.append("%s: SELF-CHECK FAILED (instrument unusable)" % name)
            continue
        print("  self-check OK: dumpraw parse == rows mode, %d rows" % len(mine))

        ndates = sum(1 for (d, *_r) in draws if d is not None)
        print("  DOS rows %d (with dates %d)   PORT rows %d" % (len(draws), ndates, len(prows)))
        print("  payment: DOS %s  PORT %s   (echo is a HEURISTIC — not scored)" % (dpay, ppay))
        if drc or prc:
            print("  rc: oracle %d port %d" % (drc, prc))
        if perr.strip():
            print("  port stderr: %s" % perr.strip().splitlines()[0])

        n = min(len(draws), len(prows))
        first_date = None
        first_val = None      # anything over half a cent
        first_mat = None      # over 2 cents: cannot be a rendering tie
        for k in range(n):
            dd, di, dp, db, dpay_r = draws[k]
            pd, pi, pp, pb, ppay_r = prows[k]
            if first_date is None and dd is not None and norm_date(dd) != norm_date(pd):
                first_date = k
            gap = max(abs(di - pi), abs(dp - pp), abs(db - pb))
            if dpay_r is not None:
                gap = max(gap, abs(dpay_r - ppay_r))
            if first_val is None and gap > 0.005:
                first_val = k
            if first_mat is None and gap > 0.02:
                first_mat = k
        print("  first DATE divergence     : %s" % ("none" if first_date is None else first_date + 1))
        print("  first >half-cent          : %s" % ("none" if first_val is None else first_val + 1))
        print("  first >2c (not a print tie): %s" % ("none" if first_mat is None else first_mat + 1))

        if asserting and name in EXPECT:
            got = None if first_mat is None else first_mat + 1
            want = EXPECT[name]
            if got != want:
                failures.append("%s: first >2c divergence %s, expected %s"
                                % (name, got, want))
            if len(draws) != len(prows):
                failures.append("%s: ROW COUNT DOS %d != PORT %d"
                                % (name, len(draws), len(prows)))

        k0 = first_mat if first_mat is not None else (
            first_val if first_val is not None else first_date)
        if k0 is not None:
            lo = max(0, k0 - 3)
            hi = min(n, k0 + 4)
            print("  rows %d..%d"
                  % (lo + 1, hi))
            for k in range(lo, hi):
                dd, di, dp, db, dpay_r = draws[k]
                pd, pi, pp, pb, ppay_r = prows[k]
                mark = "<<<" if k == k0 else "   "
                print("   %s %4d  DOS %-9s pay %11.2f int %11.2f prin %12.2f bal %13.2f" %
                      (mark, k + 1, norm_date(dd), dpay_r if dpay_r is not None else float("nan"), di, dp, db))
                print("        %4s PORT %-9s pay %11.2f int %11.4f prin %12.4f bal %13.4f" %
                      ("", norm_date(pd), ppay_r, pi, pp, pb))
        if len(draws) != len(prows):
            print("  ROW COUNT DIFFERS: DOS %d PORT %d" % (len(draws), len(prows)))

    if asserting:
        print("=" * 78)
        if failures:
            print("GATE FAILED — %d of %d case(s):" % (len(failures), len(which)))
            for f in failures:
                print("  " + f)
            return 1
        print("GATE PASSED — %d/%d cases match their recorded expectation."
              % (len(which), len(which)))
    return 0


if __name__ == "__main__":
    sys.exit(main() or 0)
