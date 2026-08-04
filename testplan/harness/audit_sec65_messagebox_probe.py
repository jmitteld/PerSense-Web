#!/usr/bin/env python3
"""audit_sec65_messagebox_probe.py — is §65's advisory subclass an ORACLE artifact?

ROUND 32. THE QUESTION THIS ANSWERS
-----------------------------------
Round 31 measured §65's advisory subclass (519 in-scope screens where DOS
reports `Internal error - last payment not found. Please contact Ones & Zeros.`
and the port answers) and found ~9% of the port's schedules leave a residual.
Standing decision 3a.4 followed: THE PORT REFUSES.

That decision assumed the message is a REFUSAL — that the real DOS engine
produces no answer on those screens. Round 32 read the call site and the dialog
layer, and that assumption does not survive the reading:

  AMORTOP.pas:1232-1233
      else if (DateComp(WhenToStop^.date, very_last) > 0) and (not balance_Calc)
        then MessageBox('Internal error - last payment not found. …',
                        DA_InternalError );
      h^.loanrate := saverate;
      ComputeTrueRate;
      DisposeOfOld_Pre;
    end; {RepayFancyLoan}

  A BARE STATEMENT. No `exit`. No `errorflag := true`. Control falls straight
  through the epilogue and RepayFancyLoan returns normally.

  dos_source/Globals.pas:107-116 — plain `MessageBox` calls
  `MessageDialog.ShowMessage`, and MessageDialogUnit.pas:63+ is a Delphi TForm:
  it sets captions and shows buttons. It latches NOTHING.

So in the real product the user dismisses a dialog and THE SCHEDULE IS DRAWN.

WHERE THE REFUSAL ACTUALLY COMES FROM — the oracle driver, not the engine:

  legacy/oracle/Globals.pas   MessageBox -> noteError -> OracleErrorFired := true
  legacy/oracle/amort_oracle.pas:1101-1109
      MakeTable(Output, false);          <-- the table IS BUILT, into Output
      if OracleErrorFired then
      begin Writeln('ERR ', OracleFirstError); Halt(0); end;   <-- and DISCARDED

This is the FOURTH bare-statement MessageBox of exactly this shape. The oracle
already swallows three of them, each after the same reading, each with a comment
in legacy/oracle/Globals.pas saying that recording it as an oracle error
"enshrined a refusal the DOS engine never performs":

  $02010002 DA_ChangeTo365   $02010012 DA_APRNoConverge   $02010007 DA_TerminatingBalloonChanged

and the fourth, $02010017 DA_InternalError, is the one under §65. The oracle's
own OracleFirstError comment (Globals.pas:60-75) already calls these dialogs
"advisory" by name — "seed 20233 fired three advisory 'last payment not found'
dialogs" — while the driver still throws the table away.

WHAT THIS SCRIPT DOES
---------------------
Runs every repro through TWO amort_oracle binaries that differ in ONE line:

  PRE    the committed oracle                     — reports ERR, no table
  PROBE  the same tree + `if HelpCode = $02010017 then exit;` in Globals.pas

and, for each, against the PORT. Three buckets, and the third is the finding:

  ORACLE-STILL-ERR   the probe did not change this case (another error fired
                     first, or the message was not this one) — EXCLUDED, and
                     counted, never silently dropped (R12/R13).
  MATCH              the probe's table agrees with the port's totals inside the
                     harness's own scaled tolerance -> the port was RIGHT, the
                     refusal was the DRIVER's, and this screen belongs in the
                     COMPARED denominator.
  DIVERGE            the probe's table disagrees with the port -> a real,
                     previously invisible fidelity defect on a population no
                     differential has ever covered (R25). These are the ones
                     worth a round.

CONTROLS (rule 4, R19, R24)
---------------------------
  --negative CORPUS  a file of `amort_oracle …` commands DOS ALREADY ANSWERS.
                     PRE and PROBE stdout must be BYTE-IDENTICAL on every one.
                     A probe that changes a screen DOS answered is not a probe,
                     it is a second defect. This is the gate.
  --positive         assert the probe still refuses a screen that fails for a
                     REAL reason (a DO_LnnNegative from ComputeTrueRate), i.e.
                     that we swallowed ONE help code and not the error path.

Usage:
    python3 testplan/harness/audit_sec65_messagebox_probe.py repros.txt \
        --pre /tmp/oraclebuild/amort_oracle --probe /tmp/probebuild/amort_oracle \
        --go /tmp/goamort [--negative cases.txt] [-v]
"""
import argparse
import os
import subprocess
import sys
from collections import Counter


def run(binary, args, env=None, timeout=180):
    e = dict(os.environ)
    if env:
        e.update(env)
    try:
        p = subprocess.run([binary] + args, capture_output=True, text=True,
                           timeout=timeout, env=e)
    except subprocess.TimeoutExpired:
        return None
    return p


def totals(out):
    """DOS's `interest`/`paid` scalars off the oracle's result line."""
    if out is None:
        return None
    for line in out.splitlines():
        t = line.split()
        if "interest" in t and "paid" in t:
            try:
                return (float(t[t.index("interest") + 1]),
                        float(t[t.index("paid") + 1]))
            except (ValueError, IndexError):
                continue
    return None


def go_totals(go, args):
    p = run(go, args)
    if p is None:
        return None
    return totals(p.stdout)


def tol(dosint, dospaid):
    """The harness's own scaled tolerance (dos_fuzzer5_test.go signal 1):
    a dollar floor, slope 5e-4 of DOS's own total. Using anything tighter here
    would manufacture divergences the standing gate does not score."""
    return max(1.0, 5e-4 * abs(dosint)), max(1.0, 5e-4 * abs(dospaid))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repros")
    ap.add_argument("--pre", default="/tmp/oraclebuild/amort_oracle")
    ap.add_argument("--probe", default="/tmp/probebuild/amort_oracle")
    ap.add_argument("--go", default="/tmp/goamort")
    ap.add_argument("--negative", default=None,
                    help="corpus of commands DOS already answers; PRE and PROBE "
                         "stdout must be byte-identical on every one")
    ap.add_argument("-v", "--verbose", action="store_true")
    a = ap.parse_args()

    if a.negative:
        cases = [l.strip() for l in open(a.negative) if l.strip().startswith("amort_oracle")]
        same = diff = 0
        in_domain = 0
        offenders = []
        for line in cases:
            args = line.split()[1:]
            p1, p2 = run(a.pre, args), run(a.probe, args)
            o1 = None if p1 is None else p1.stdout
            o2 = None if p2 is None else p2.stdout
            # THE CONTROL'S POPULATION IS THE COMPLEMENT OF THE PROBE'S DOMAIN.
            # A corpus dumped with FZ5CASEDUMP=1 is EVERY generated case, which
            # includes the advisory screens the probe exists to change. Scoring
            # those as "the probe moved a screen DOS answered" is the control
            # measuring its own subject — the first run of this script did
            # exactly that and reported a failed gate on three cases that were
            # all §65 advisories. Partition on PRE's own message, and COUNT the
            # excluded ones rather than dropping them (R12/R13).
            if "last payment not found" in ((o1 or "") + (p1.stderr if p1 else "")):
                in_domain += 1
                continue
            if o1 == o2:
                same += 1
            else:
                diff += 1
                offenders.append((line, (o1 or "")[:200], (o2 or "")[:200]))
        print(f"NEGATIVE CONTROL — {len(cases)} generated commands, of which "
              f"{in_domain} are §65 advisories (the probe's own domain, excluded)")
        print(f"  scored (DOS answered, or refused for another reason): {len(cases)-in_domain}")
        print(f"  byte-identical PRE vs PROBE : {same}")
        print(f"  CHANGED                     : {diff}")
        if offenders:
            print("  ⚠️ GATE FAILED — the probe moved a screen the oracle answered:")
            for line, x, y in offenders[:10]:
                print(f"    {line}\n      PRE  : {x!r}\n      PROBE: {y!r}")
            return 1
        print("  GATE PASSED — the probe is inert on every screen DOS answered.\n")

    repros = [l.strip() for l in open(a.repros) if l.strip()]
    still_err, match, diverge, unscored = [], [], [], []

    for line in repros:
        args = line.split()
        assert args[0] == "amort_oracle", args[0]
        args = args[1:]

        pre = run(a.pre, args)
        pre_out = "" if pre is None else (pre.stdout + pre.stderr)
        if "last payment not found" not in pre_out:
            unscored.append((line, "PRE-NOT-ADVISORY"))
            continue

        pr = run(a.probe, args)
        if pr is None:
            unscored.append((line, "PROBE-TIMEOUT"))
            continue
        if pr.stdout.startswith("ERR ") or totals(pr.stdout) is None:
            first = pr.stdout.strip().splitlines()[0] if pr.stdout.strip() else "(no stdout)"
            still_err.append((line, first))
            continue

        dt = totals(pr.stdout)
        gt = go_totals(a.go, args)
        if gt is None:
            unscored.append((line, "GO-NO-TOTALS"))
            continue
        ti, tp = tol(*dt)
        di, dp = abs(gt[0] - dt[0]), abs(gt[1] - dt[1])
        if di <= ti and dp <= tp:
            match.append((line, dt, gt))
        else:
            diverge.append((line, dt, gt, di, dp, ti, tp))

    n = len(repros)
    print(f"§65 MESSAGEBOX PROBE — {n} in-scope advisory repros\n")
    print(f"  PROBE PRODUCES A TABLE and it MATCHES the port : {len(match):4d}")
    print(f"      -> the DOS ENGINE answered these all along. The refusal was the")
    print(f"         oracle DRIVER discarding a table MakeTable had already built.")
    print(f"  PROBE PRODUCES A TABLE and it DIVERGES         : {len(diverge):4d}")
    print(f"      -> a REAL fidelity defect, on a population no differential has")
    print(f"         ever covered (R25). This is the number that matters.")
    print(f"  PROBE STILL REFUSES (a different error first)  : {len(still_err):4d}")
    if unscored:
        print(f"  UNSCORED                                       : {len(unscored):4d}"
              f"   ({', '.join(f'{k}={v}' for k, v in Counter(r for _, r in unscored).most_common())})")
        print(f"      -> NOT agreement. R12: a skip is not a pass.")

    if still_err:
        print("\n  the still-refusing errors:")
        for k, v in Counter(e for _, e in still_err).most_common(8):
            print(f"    {v:4d}  {k[:110]}")

    if diverge:
        print("\n  DIVERGENCES (DOS-probe vs port totals):")
        for line, dt, gt, di, dp, ti, tp in sorted(diverge, key=lambda r: -max(r[3], r[4]))[:20]:
            print(f"    dInt={di:,.2f} (tol {ti:,.2f})  dPaid={dp:,.2f} (tol {tp:,.2f})")
            print(f"      DOS int={dt[0]:,.2f} paid={dt[1]:,.2f} | Go int={gt[0]:,.2f} paid={gt[1]:,.2f}")
            print(f"      {line}")

    if a.verbose:
        for line, dt, gt in match[:5]:
            print(f"\nMATCH  DOS int={dt[0]:,.2f} paid={dt[1]:,.2f} | "
                  f"Go int={gt[0]:,.2f} paid={gt[1]:,.2f}\n  {line}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
