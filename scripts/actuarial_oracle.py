#!/usr/bin/env python3
"""Live third-party actuarial oracle for the Per%Sense life-contingency engine.

The original DOS actuarial core (the `ACTUARY` unit + mortality tables) is absent
from this snapshot (docs/actuarial_oracle_blocked.md), so the engine cannot be
differentially tested against the original the way every other engine is. This
script stands in an INDEPENDENT, authoritative substitute oracle built around the
open-source `actuarialmath` library and the SOA Standard Ultimate Life Table
(SULT, Makeham A=0.00022 B=2.7e-6 c=1.124, i=5%) — the table whose values
`actuarialmath` reproduces to the SOA's published booklet.

Every quantity is derived from actuarialmath's own survival p_x, so the oracle's
numbers come from the third-party library, not from a re-implementation here.

Usage (batch): read one query per stdin line, print `value <x>` per line.
  Queries:
    surv X K          k-year survival kp_x  (= p_x(X, t=K))
    lifeexp X         curtate life expectancy e_x = sum_{k>=1} kp_x
    annuity X         whole-life annuity-due ä_x = sum_{k>=0} v^k kp_x
    tempannuity X N   temporary annuity-due ä_{x:n}
    insurance X       whole-life insurance A_x
    endow X N         pure endowment nE_x = v^N * Np_x
    pod X             Payment-on-Death, DOS convention (mid-year death,
                      continuous discount delta=ln(1+i))

  Run:  pip install actuarialmath ipython
        printf 'annuity 20\\ninsurance 40\\n' | python3 scripts/actuarial_oracle.py
"""
import sys, math
from actuarialmath import SULT

s = SULT()                       # SOA SULT, i = 0.05
I = 0.05
V = 1.0 / (1.0 + I)
DELTA = math.log(1.0 + I)
OMEGA = 120                      # p_x returns ~0 beyond the table's max age


def kpx(x, k):
    """k-year survival from actuarialmath's own p_x (the third-party source)."""
    if k <= 0:
        return 1.0
    try:
        return float(s.p_x(int(x), t=int(k)))
    except Exception:
        return 0.0


def annuity(x):
    return sum(V ** k * kpx(x, k) for k in range(0, OMEGA - x + 1))


def temp_annuity(x, n):
    return sum(V ** k * kpx(x, k) for k in range(0, n))


def insurance(x):
    return sum(V ** (k + 1) * (kpx(x, k) - kpx(x, k + 1)) for k in range(0, OMEGA - x))


def endow(x, n):
    return V ** n * kpx(x, n)


def lifeexp(x):
    return sum(kpx(x, k) for k in range(1, OMEGA - x + 1))


def pod(x):
    return sum((kpx(x, k) - kpx(x, k + 1)) * math.exp(-DELTA * (k + 0.5))
               for k in range(0, OMEGA - x) if kpx(x, k) - kpx(x, k + 1) > 0)


def answer(parts):
    cmd = parts[0]
    a = [int(p) for p in parts[1:]]
    if cmd == "surv":
        return kpx(a[0], a[1])
    if cmd == "lifeexp":
        return lifeexp(a[0])
    if cmd == "annuity":
        return annuity(a[0])
    if cmd == "tempannuity":
        return temp_annuity(a[0], a[1])
    if cmd == "insurance":
        return insurance(a[0])
    if cmd == "endow":
        return endow(a[0], a[1])
    if cmd == "pod":
        return pod(a[0])
    raise ValueError("unknown query: " + cmd)


def main():
    out = []
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            out.append("value %.12f" % answer(line.split()))
        except Exception as e:  # emit a parseable error so the Go side can skip
            out.append("err " + str(e).replace("\n", " "))
    sys.stdout.write("\n".join(out) + "\n")


if __name__ == "__main__":
    main()
