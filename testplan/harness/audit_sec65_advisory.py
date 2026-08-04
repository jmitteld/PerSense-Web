#!/usr/bin/env python3
"""audit_sec65_advisory.py — the §65 DETECTABILITY investigation.

THE QUESTION
------------
On 519 in-scope screens DOS reports

    Internal error - last payment not found.  Please contact Ones & Zeros.

and the port produces a schedule.  Standing decision 3a.1 is "answer, but say
so", and it is blocked on one thing: **can the port know DOS would have
failed?**

WHAT DOS'S CONDITION ACTUALLY IS
--------------------------------
`RepayFancyLoan` (AMORTOP.pas:1226-1233), after its walk:

    if ((not h^.lastok) and (WhenToStop^.principal = 0)) then
      begin if (not entire) then h^.lastdate := WhenToStop^.date; end
    else if (DateComp(WhenToStop^.date, very_last) > 0) and (not balance_Calc) then
      MessageBox('Internal error - last payment not found. ...', DA_InternalError);

In words: **the walk ran past the screen's own last payment date with principal
still outstanding.**  The loan did not amortise to zero.  That is not a random
internal fault — it is a deterministic, input-determined state, and both
quantities (`WhenToStop^.principal`, `very_last`) have port-side counterparts.

WHAT THIS SCRIPT MEASURES
-------------------------
For each repro, run the PORT and read the balance on its final row:

  TERMINATES  final balance is zero  -> the port produced a schedule that pays
              off.  DOS refused a screen that is fine; the original is at fault
              and the port is genuinely better.
  RESIDUAL    final balance is NOT zero -> the port emitted a schedule and
              totals for a loan that never pays off.  DOS's refusal is CORRECT
              and protective, and the port is silently shipping a schedule that
              leaves money outstanding.

The split between those two buckets is the whole of the §65 decision: "refuse
everywhere" throws the first bucket away, "answer everywhere" ships the second.

Harvest repros with (they are behind FLAKEDUMP by design — an ungated per-case
line breaks paired_regression.sh's grep):

    PERSENSE_FUZZ=1 PERSENSE_REQUIRE_ORACLE=1 PERSENSE_FUZZ_FLAKEDUMP=1 \
    PERSENSE_FUZZ_SEED=<s> PERSENSE_FUZZ_N=400 <amort test binary> \
      -test.run TestDOSFuzzer5AllAdvancedOptions -test.v \
      | grep 'SIG=ADVISORY:go_solved_dos_internal_error_in_scope' \
      | sed -E 's/.*(amort_oracle .*)/\1/' | sed 's/ bdump$//' > repros.txt

Usage:
    python3 testplan/harness/audit_sec65_advisory.py repros.txt \
        --go /tmp/goamort [--oracle /tmp/oraclebuild/amort_oracle] [-v]
"""
import argparse
import subprocess
import sys

HALF_CENT = 0.005


def run(binary, args, env=None, timeout=120):
    import os
    e = dict(os.environ)
    if env:
        e.update(env)
    try:
        p = subprocess.run([binary] + args, capture_output=True, text=True,
                           timeout=timeout, env=e)
    except subprocess.TimeoutExpired:
        return None
    return p.stdout or ""


def final_balance(go, args):
    """The balance on the port's LAST schedule row.

    GOAMORT_ALLROWS=1 so settlement rows are included — a schedule that pays off
    only on a tacked-on final row must not be scored as a residual."""
    out = run(go, args + ["rows"], env={"GOAMORT_ALLROWS": "1",
                                        "GOAMORT_ROWDATES": "1"})
    if out is None:
        return None, "TIMEOUT"
    rows = [l for l in out.splitlines() if l.startswith("row ")]
    if not rows:
        return None, "NOROWS"
    last = rows[-1]
    # `row <n> <date> pay <p> int <i> prin <pr> bal <b>`
    toks = last.split()
    if "bal" not in toks:
        return None, "NOBAL"
    try:
        return float(toks[toks.index("bal") + 1]), last
    except (ValueError, IndexError):
        return None, "PARSE"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repros", help="file of `amort_oracle …` command lines")
    ap.add_argument("--go", default="/tmp/goamort")
    ap.add_argument("--oracle", default=None,
                    help="if given, re-confirm each case still internal-errors")
    ap.add_argument("-v", "--verbose", action="store_true")
    a = ap.parse_args()

    cases = [l.strip() for l in open(a.repros) if l.strip()]
    terminates, residual, unknown = [], [], []
    not_confirmed = 0

    for line in cases:
        args = line.split()
        assert args[0] == "amort_oracle", args[0]
        args = args[1:]

        if a.oracle:
            o = run(a.oracle, args)
            if o is None or "last payment not found" not in (o or ""):
                # The oracle prints the message on stderr; check there too.
                import os
                p = subprocess.run([a.oracle] + args, capture_output=True,
                                   text=True, timeout=120)
                if "last payment not found" not in (p.stdout + p.stderr):
                    not_confirmed += 1
                    continue

        bal, detail = final_balance(a.go, args)
        if bal is None:
            unknown.append((line, detail))
        elif abs(bal) < HALF_CENT:
            terminates.append((line, detail))
        else:
            residual.append((line, bal, detail))

    n = len(cases)
    print(f"§65 advisory subclass — {n} in-scope repros\n")
    print(f"  TERMINATES  port's schedule reaches zero   : {len(terminates):4d}"
          f"   ({100.0*len(terminates)/max(n,1):.1f}%)")
    print(f"              -> DOS refused a screen that is FINE. Port is better.")
    print(f"  RESIDUAL    port's schedule does NOT pay off: {len(residual):4d}"
          f"   ({100.0*len(residual)/max(n,1):.1f}%)")
    print(f"              -> DOS's refusal is CORRECT. Port ships a schedule")
    print(f"                 leaving a balance outstanding, with totals.")
    if unknown:
        from collections import Counter
        reasons = Counter(d if d in ("TIMEOUT", "NOROWS", "NOBAL", "PARSE")
                          else "OTHER" for _, d in unknown)
        print(f"  UNSCORED                                   : {len(unknown):4d}"
              f"   ({', '.join(f'{k}={v}' for k, v in reasons.most_common())})")
        print(f"              -> ⚠️ NOT agreement. Measured 2026-08-04: every")
        print(f"                 NOROWS case is a `norate` screen, and goamort")
        print(f"                 does not implement norate/noamt (they live in")
        print(f"                 amort_oracle and dos_fuzzer5_test.go only).")
        print(f"                 These are UNMEASURED, and the percentages above")
        print(f"                 are over ALL repros — divide by the measured")
        print(f"                 count if you want the conditional rate.")
    if a.oracle:
        print(f"  (not re-confirmed as internal-error         : {not_confirmed})")

    if residual:
        rs = sorted(r[1] for r in residual)
        print(f"\n  residual balances: min {rs[0]:,.2f}  median "
              f"{rs[len(rs)//2]:,.2f}  max {rs[-1]:,.2f}")

    if a.verbose:
        for line, bal, detail in residual[:20]:
            print(f"\nRESIDUAL {bal:,.2f}\n  {line}\n  {detail}")
        for line, detail in terminates[:5]:
            print(f"\nTERMINATES\n  {line}\n  {detail}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
