#!/usr/bin/env python3
"""analyze_plain_arm.py — pool one PLAIN-LOAN differential arm. Round 22.

Companion to run_plain_arm.sh, in the same relationship analyze_arm.py has to
run_arm.sh: the runner keeps whole logs, this reads the LEDGER lines out of them
and adds up populations that are actually comparable.

THREE UNITS RULES, EACH OF WHICH THIS PROJECT HAS BROKEN AT LEAST ONCE:

  1. CASES and SIGNAL INSTANCES are different quantities and must never appear
     in the same rate. A case can carry several signals.
  2. ROWS are a third population, an order of magnitude larger than cases, and a
     per-row rate is not comparable to a per-case rate in any direction.
  3. The DENOMINATOR for a case rate is ACTUALLY COMPARED, never GENERATED.
     Refused, no-totals and unparsed cases never reached a comparison; counting
     them in a denominator is how a real rate gets diluted into a reassuring one.

It also prints the ERA SPLIT separately (client decision 2026-08-03: generation
still reaches DOS's whole range, the REPORT splits at 2099) and the refusal
bucket's contents, because a bucket nobody prints is a bucket nobody audits —
which is the whole reason round 22 exists.
"""
import re
import sys
import glob
import os
from collections import Counter

LEDGER = re.compile(
    r"ledger: generated (\d+) = compared (\d+) \+ oracle-refused (\d+) \+ "
    r"port-refused (\d+) \+ no-totals (\d+) \+ unparsed (\d+) \| UNACCOUNTED (-?\d+)")
SIGS = re.compile(
    r"signals: row-count (\d+), total-interest (\d+), total-paid (\d+), PAYMENT (\d+), "
    r"LAST-DATE (\d+), NPERIODS (\d+) \(of (\d+) compared\) \| "
    r"DOS trailing all-zero rows excluded (\d+)")
ROWSIG = re.compile(
    r"signals PER ROW: interest (\d+), principal-paid (\d+), balance (\d+) "
    r"\(of (\d+) rows compared")
REFUSE = re.compile(
    r"refusals: oracle refused (\d+) = twopay (\d+) \+ nonconverge (\d+) \+ other (\d+) \| "
    r"PAIRED \(port also refused\) (\d+) \| go-solved-dos-refused (\d+) \(HARD\) \| "
    r"dos-nonconverge port retires (\d+) / port spurious (\d+)")
ERA = re.compile(
    r"era split: in-scope<=2099 compared (\d+) signals (\d+) \| "
    r"out-of-scope>2099 compared (\d+) signals (\d+) \| of the out-of-scope, (\d+)")
TIES = re.compile(
    r"PER-ROW numerator adjudicated: half-cent PRINT TIES — interest (\d+), "
    r"principal-paid (\d+), balance (\d+)")
BALLOON = re.compile(
    r"terminating-balloon final rows [^:]*: (\d+) cases, of which IN SCOPE (\d+)")
COV = re.compile(
    r"coverage: anchor day>=29 (\d+) \| February last date (\d+) \(of which CLAMPED (\d+)\) \| "
    r"odd first period (\d+) \| DOS last years (\d+)-(\d+)")
MSGDIFF = re.compile(r"refusal messages: same class (\d+) \| differing (.*?) \| "
                     r"of the differing, IN SCOPE \(horizon <=2099\) (\d+)")


def main(d):
    logs = sorted(glob.glob(os.path.join(d, "seed_*.log")))
    if not logs:
        print(f"no seed logs in {d}")
        return 1
    t = Counter()
    no_ledger, minyear, maxyear = [], 9999, 0
    worst = []
    for p in logs:
        s = open(p, errors="replace").read()
        m = LEDGER.search(s)
        if not m:
            # A SEED WITH NO LEDGER IS NOT A CLEAN SEED. Go buffers t.Logf, so a
            # killed binary prints nothing and a missing ledger is indis-
            # tinguishable from success unless it is named. R8, round 19.
            no_ledger.append(os.path.basename(p))
            continue
        g, c, orf, prf, nt, up, un = map(int, m.groups())
        t["generated"] += g; t["compared"] += c; t["oracle_refused"] += orf
        t["port_refused"] += prf; t["no_totals"] += nt; t["unparsed"] += up
        t["unaccounted"] += un
        t["seeds"] += 1
        if (x := SIGS.search(s)):
            rc, ti, tp, pay, ld, npr, _, tz = map(int, x.groups())
            t["sig_rowcount"] += rc; t["sig_totint"] += ti; t["sig_totpaid"] += tp
            t["sig_payment"] += pay; t["sig_lastdate"] += ld; t["sig_nperiods"] += npr
            t["dos_trailing_zero_rows"] += tz
        if (x := ROWSIG.search(s)):
            ri, rp, rb, rows = map(int, x.groups())
            t["row_interest"] += ri; t["row_prinpaid"] += rp; t["row_balance"] += rb
            t["rows_compared"] += rows
        if (x := REFUSE.search(s)):
            tot, tw, nc, ot, pr, hard, ret, spur = map(int, x.groups())
            t["ref_twopay"] += tw; t["ref_nonconv"] += nc; t["ref_other"] += ot
            t["ref_paired"] += pr; t["ref_go_solved_dos_refused"] += hard
            t["ref_nonconv_go_retires"] += ret; t["ref_nonconv_go_spurious"] += spur
        if (x := ERA.search(s)):
            ic, isig, oc, osig, wrap = map(int, x.groups())
            t["in_scope"] += ic; t["in_scope_sig"] += isig
            t["out_scope"] += oc; t["out_scope_sig"] += osig
            t["era_wrap_rescued"] += wrap
        if (x := TIES.search(s)):
            ti, tp, tb = map(int, x.groups())
            t["tie_interest"] += ti; t["tie_prinpaid"] += tp; t["tie_balance"] += tb
        if (x := BALLOON.search(s)):
            t["balloon_cases"] += int(x.group(1))
            t["balloon_in_scope"] += int(x.group(2))
        if (x := COV.search(s)):
            ah, fl, fc, of_, y0, y1 = map(int, x.groups())
            t["anchor_high"] += ah; t["feb_last"] += fl; t["feb_clamped"] += fc
            t["odd_first"] += of_
            minyear = min(minyear, y0); maxyear = max(maxyear, y1)
        if (x := MSGDIFF.search(s)):
            t["ref_msg_same"] += int(x.group(1))
            t["ref_msg_differ_in_scope"] += int(x.group(3))
        for ln in s.splitlines():
            if re.search(r"^\s+(rowint|rowbal|rowprin|lastdate|nperiods|rows|int|paid|pay):", ln.strip()) \
               or re.search(r"^\s+(rowint|rowbal|rowprin|lastdate|nperiods|pay):", ln):
                if len(worst) < 25:
                    worst.append(ln.strip())

    def rate(num, den):
        return f"1 in {den // num:,}" if num else "0 events"

    print(f"PLAIN ARM {d}")
    print(f"  seeds with a ledger        {t['seeds']} of {len(logs)}")
    if no_ledger:
        print(f"  !! SEEDS WITH NO LEDGER    {len(no_ledger)}  {no_ledger}")
        print("     A killed binary emits nothing, and nothing looks like success.")
    print(f"  generated                  {t['generated']:,}")
    print(f"  ACTUALLY COMPARED          {t['compared']:,}   <-- the only case denominator")
    print(f"  oracle refused             {t['oracle_refused']:,}  "
          f"(twopay {t['ref_twopay']:,} + nonconverge {t['ref_nonconv']:,} + other {t['ref_other']:,})")
    print(f"  port refused               {t['port_refused']:,}")
    print(f"  no-totals / unparsed       {t['no_totals']:,} / {t['unparsed']:,}")
    print(f"  UNACCOUNTED                {t['unaccounted']:,}")
    print()
    print("  --- case-level signals (SIGNAL INSTANCES, not cases) ---")
    for k, lbl in [("sig_rowcount", "row count"), ("sig_totint", "total interest"),
                   ("sig_totpaid", "total paid"), ("sig_payment", "PAYMENT"),
                   ("sig_lastdate", "LAST DATE"), ("sig_nperiods", "NPERIODS")]:
        print(f"    {lbl:<16} {t[k]:,}")
    print(f"    (DOS trailing all-zero rows excluded, not scored: {t['dos_trailing_zero_rows']:,})")
    print()
    print("  --- ROW-level signals (a THIRD population — do not mix with the above) ---")
    print(f"    rows compared    {t['rows_compared']:,}")
    print(f"    interest         {t['row_interest']:,}   {rate(t['row_interest'], t['rows_compared'])}")
    print(f"    principal paid   {t['row_prinpaid']:,}   {rate(t['row_prinpaid'], t['rows_compared'])}")
    print(f"    balance          {t['row_balance']:,}   {rate(t['row_balance'], t['rows_compared'])}")
    print(f"    of those, HALF-CENT PRINT TIES: interest {t['tie_interest']:,}, "
          f"principal-paid {t['tie_prinpaid']:,}, balance {t['tie_balance']:,}")
    print(f"    terminating-balloon final rows (§63, excluded from the above): "
          f"{t['balloon_cases']:,} cases, IN SCOPE {t['balloon_in_scope']:,}")
    print()
    print("  --- the refusal bucket, adjudicated ---")
    print(f"    PAIRED (both refused)              {t['ref_paired']:,}")
    print(f"    go-solved-dos-refused (HARD)       {t['ref_go_solved_dos_refused']:,}")
    print(f"    dos-nonconverge, port retires      {t['ref_nonconv_go_retires']:,}")
    print(f"    dos-nonconverge, port spurious     {t['ref_nonconv_go_spurious']:,}")
    print(f"    refusal message same class         {t['ref_msg_same']:,}")
    print(f"    differing message, IN SCOPE        {t['ref_msg_differ_in_scope']:,}")
    print()
    print("  --- era split (client boundary 2099) ---")
    print(f"    in scope   compared {t['in_scope']:,}  signals {t['in_scope_sig']:,}   "
          f"{rate(t['in_scope_sig'], t['in_scope'])}")
    print(f"    out scope  compared {t['out_scope']:,}  signals {t['out_scope_sig']:,}   "
          f"{rate(t['out_scope_sig'], t['out_scope'])}")
    print(f"    of the out-of-scope, {t['era_wrap_rescued']:,} are visible ONLY to the "
          f"arithmetic bound (§55 year-byte wrap); round 21 filed those as IN SCOPE")
    print()
    print("  --- draw coverage (a stratification label is a coverage claim) ---")
    print(f"    anchor day >= 29           {t['anchor_high']:,}")
    print(f"    February last date         {t['feb_last']:,}  (CLAMPED {t['feb_clamped']:,})")
    print(f"    odd first period           {t['odd_first']:,}")
    print(f"    DOS last years             {minyear}-{maxyear}")
    if worst:
        print("\n  --- sample signals ---")
        for w in worst:
            print(f"    {w}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "."))
