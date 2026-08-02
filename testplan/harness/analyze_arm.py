#!/usr/bin/env python3
"""analyze_arm.py — adjudicate a fuzzer5 arm's signals against a real denominator.

Reads the FULL per-seed logs (run_arm.sh keeps them) so every rate is computed
over ACTUALLY COMPARED, per standing rule 9 and R5. Also extracts the
balloon_value_differs cases with both engines' values so the class can be split
into "rounding tail on a huge balance" and "real disagreement" instead of being
counted as one undifferentiated 27.
"""
import re, sys, os, glob

BAL = re.compile(
    r"SIG=HARD:balloon_value_differs (amort_oracle [^\n]*)\n"
    r"\s*DOS: (\S+) (-?[\d.]+) \(row (\d+),[^\n]*nballoons=(\d+) nlines=(\d+)\)\n"
    r"\s*Go : (\S+) (-?[\d.]+)")
LEDGER = re.compile(r"ledger: generated (\d+) = .*?\| UNACCOUNTED (\d+)")
COMPARED = re.compile(r"ACTUALLY COMPARED (\d+)")
SIG = re.compile(r"SIG=(HARD|ADVISORY):([a-z_0-9]+)")


def analyze(d):
    gen = comp = unacc = 0
    seeds = ledgers = 0
    sigs = {}
    bals = []
    cmds = set()
    for f in sorted(glob.glob(os.path.join(d, "seed_*.log"))):
        txt = open(f, errors="replace").read()
        seeds += 1
        m = LEDGER.search(txt)
        if m:
            ledgers += 1
            gen += int(m.group(1)); unacc += int(m.group(2))
        c = COMPARED.search(txt)
        if c:
            comp += int(c.group(1))
        for sev, cls in SIG.findall(txt):
            sigs[(sev, cls)] = sigs.get((sev, cls), 0) + 1
        for mm in BAL.finditer(txt):
            cmd, ddate, damt, row, nb, nl, gdate, gamt = mm.groups()
            amount = float(cmd.split()[1])
            damt, gamt = float(damt), float(gamt)
            bals.append(dict(seed=os.path.basename(f), cmd=cmd, amount=amount,
                             ddate=ddate, gdate=gdate, dos=damt, go=gamt,
                             row=int(row), nb=int(nb), nl=int(nl)))
        cmds.update(re.findall(r"SIG=(?:HARD|ADVISORY):[a-z_0-9]+ (amort_oracle .*)", txt))
    return dict(dir=d, seeds=seeds, ledgers=ledgers, gen=gen, comp=comp,
                unacc=unacc, sigs=sigs, bals=bals, cmds=cmds)


def report(r):
    print(f"\n===== {r['dir']} =====")
    print(f"seeds {r['seeds']}, reached a ledger {r['ledgers']}, "
          f"generated {r['gen']}, ACTUALLY COMPARED {r['comp']}, UNACCOUNTED {r['unacc']}")
    hard = sum(v for (s, _), v in r['sigs'].items() if s == 'HARD')
    adv = sum(v for (s, _), v in r['sigs'].items() if s == 'ADVISORY')
    uniq = len(r['cmds'])
    if r['comp']:
        print(f"HARD {hard} = 1 in {r['comp']/hard:.0f}" if hard else "HARD 0")
        print(f"ADVISORY {adv}   unique reproducing commands {uniq} "
              f"= 1 in {r['comp']/uniq:.0f}" if uniq else "")
    for (s, c), v in sorted(r['sigs'].items(), key=lambda x: -x[1]):
        print(f"   {s:9s} {c:35s} {v}")


def balloons(r):
    b = r['bals']
    if not b:
        return
    print(f"\n--- balloon_value_differs, {len(b)} cases in {r['dir']} ---")
    print(f"{'|DOS|':>18} {'absdiff':>16} {'rel':>10} {'tackTol':>10} "
          f"{'valTol(5e-4)':>13} {'datediff':>8}")
    survives = []
    for x in b:
        diff = abs(x['dos'] - x['go'])
        rel = diff / abs(x['dos']) if x['dos'] else float('inf')
        tacktol = max(0.05, 1e-5 * abs(x['amount']))
        valtol = max(0.05, 5e-4 * abs(x['dos']))
        dd = x['ddate'] != x['gdate']
        if diff > valtol or dd:
            survives.append(x)
        print(f"{abs(x['dos']):18.2f} {diff:16.2f} {rel:10.2e} {tacktol:10.2f} "
              f"{valtol:13.2f} {'DATE' if dd else '':>8}")
    print(f"\nof {len(b)} cases, {len(b)-len(survives)} are inside a "
          f"VALUE-scaled 5e-4 tolerance (the same slope the totals use);")
    print(f"{len(survives)} survive it and are real disagreements:")
    for x in survives:
        rel = abs(x['dos'] - x['go']) / abs(x['dos']) if x['dos'] else 0
        print(f"   rel={rel:.3%} DOS {x['ddate']} {x['dos']:.2f} | "
              f"Go {x['gdate']} {x['go']:.2f}")
        print(f"      {x['cmd'][:200]}")
    return survives


if __name__ == "__main__":
    for d in sys.argv[1:]:
        r = analyze(d)
        report(r)
        balloons(r)
