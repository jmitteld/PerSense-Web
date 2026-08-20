#!/usr/bin/env python3
"""r64_additive_check.py — did round 64's server change move ANY number?

Round 64 added one response field (`prepayResolvedStop`) and refactored
`countPrepayDates` into `walkPrepayDates`. The claim is that the change is
purely ADDITIVE: for every request, every field of the response other than the
new one is byte-identical to what a build of the tree WITHOUT the change
produces.

R36 says a fix must be seen to move a number at the consumer. This is that rule
run backwards: a display-only change must be seen to move NOTHING, and the only
honest way to show it is to run both binaries over the same corpus and diff.

    # build both, serve both, then:
    python3 testplan/harness/r64_additive_check.py \\
        --base http://127.0.0.1:8841 --new http://127.0.0.1:8842

Exit 0 = additive. Non-zero = the change moved something, and the field is named.

⚠️ THE CORPUS IS ENUMERATED BELOW AND IT IS SMALL BY DESIGN — it is a targeted
differential over the paths `countPrepayDates` is reachable from, not a
convergence measurement. It says nothing about the port's agreement with DOS.
⚠️ IT DELIBERATELY INCLUDES SCREENS WITH NO PREPAYMENTS AT ALL, so a change that
leaked outside the guarded branch would show up.
⚠️ AND IT FAILS CLOSED: an empty corpus, a transport error, or a run in which the
new field never appeared once is an ERROR, not a pass (R64, R69).
"""

import argparse
import json
import sys
import urllib.error
import urllib.request

NEW_FIELDS = {"prepayResolvedStop"}


def post(url, path, body):
    req = urllib.request.Request(
        url.rstrip("/") + path,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read().decode())


def loan(**kw):
    b = {
        "perYr": 12, "basis": "360", "amount": 100000.0, "rate": 0.06,
        "loanDate": "2026-01-01", "firstDate": "2026-02-01",
        "nPeriods": 360, "points": 0,
    }
    b.update(kw)
    return b


def prepay(start, per_yr, amount, stop=None, n=None):
    row = {"startDate": start, "perYr": per_yr, "amount": amount}
    if stop:
        row["stopDate"] = stop
    if n:
        row["nPmts"] = n
    return row


CORPUS = []

# 1. no advanced options at all — the branch is never entered.
CORPUS.append(("/api/amortization/calc", loan()))
CORPUS.append(("/api/amortization/calc", loan(nPeriods=0, lastDate="2056-03-23")))
CORPUS.append(("/api/amortization/calc", loan(perYr=26, nPeriods=200)))

# 2. prepayments with a COUNT (the arm the new field never reports on).
for n in (12, 60, 362):
    CORPUS.append(("/api/amortization/calc",
                   loan(prepayments=[prepay("2026-02-01", 12, 300.0, n=n)])))

# 3. prepayments with a STOP DATE and no count — ON the grid (no echo) ...
for stop in ("2056-03-01", "2030-07-01", "2027-02-01"):
    CORPUS.append(("/api/amortization/calc",
                   loan(prepayments=[prepay("2026-02-01", 12, 300.0, stop=stop)])))

# 4. ... and OFF the grid (the echo fires).
for stop in ("2056-03-23", "2030-07-30", "2027-02-14", "2029-11-29"):
    CORPUS.append(("/api/amortization/calc",
                   loan(prepayments=[prepay("2026-02-01", 12, 300.0, stop=stop)])))
for py in (1, 2, 4, 6, 24, 26, 52):
    CORPUS.append(("/api/amortization/calc",
                   loan(prepayments=[prepay("2026-02-15", py, 250.0, stop="2040-06-03")])))

# 5. two rows, including an off-grid one second (the response is indexed by
#    request order, so a mis-indexed echo would show here).
CORPUS.append(("/api/amortization/calc", loan(prepayments=[
    prepay("2026-02-01", 12, 100.0, n=24),
    prepay("2027-02-01", 12, 300.0, stop="2035-06-17"),
])))

# 6. prepayments alongside balloons and adjustments, so the echo is measured on a
#    response that carries the other echoes too.
CORPUS.append(("/api/amortization/calc", loan(
    prepayments=[prepay("2026-02-01", 12, 300.0, stop="2035-06-17")],
    balloons=[{"date": "2036-02-01"}],
)))
CORPUS.append(("/api/amortization/calc", loan(
    prepayments=[prepay("2026-02-01", 12, 300.0, stop="2035-06-17")],
    adjustments=[{"date": "2030-03-15", "rate": 0.07}],
)))

# 7. a couple of PV screens: nothing in the PV path changed, and showing that is
#    part of the claim (R75 — a shared helper is a change to every caller).
CORPUS.append(("/api/presentvalue/calc", {
    "asOfDate": "2026-08-20", "rate": 0.0598503,
    "periodics": [{"fromDate": "2026-01-10", "toDate": "2056-03-23",
                   "perYr": 12, "amount": 200.0, "cola": 0, "act": "N"}],
}))
CORPUS.append(("/api/presentvalue/calc", {
    "asOfDate": "2026-08-20", "rate": 0.0598503,
    "lumpSums": [{"date": "2027-08-20", "amount": 12000.0, "act": "N"}],
}))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", required=True, help="URL of a server built WITHOUT round 64's change")
    ap.add_argument("--new", required=True, help="URL of a server built WITH it")
    args = ap.parse_args()

    if not CORPUS:
        print("EMPTY CORPUS — refusing to report a pass (R64)")
        return 2

    diffs = []
    echoed = 0
    for i, (path, body) in enumerate(CORPUS):
        try:
            a = post(args.base, path, body)
            b = post(args.new, path, body)
        except (urllib.error.URLError, OSError) as e:
            print("TRANSPORT FAILURE on case %d (%s): %s" % (i, path, e))
            return 2
        if any(k in b for k in NEW_FIELDS):
            echoed += 1
        keys = (set(a) | set(b)) - NEW_FIELDS
        for k in sorted(keys):
            av = json.dumps(a.get(k), sort_keys=True)
            bv = json.dumps(b.get(k), sort_keys=True)
            if av != bv:
                diffs.append((i, path, k, av[:160], bv[:160]))
        # the new field must not appear on the OLD server at all
        for k in NEW_FIELDS:
            if k in a:
                diffs.append((i, path, k + " (present on the BASE server)",
                              json.dumps(a[k]), ""))

    print("cases: %d   responses carrying the new field: %d" % (len(CORPUS), echoed))
    if echoed == 0:
        print("THE NEW FIELD NEVER FIRED — this run has no power over it (R69). "
              "Either the corpus is wrong or the change is not deployed.")
        return 2
    if diffs:
        print("NOT ADDITIVE — %d field difference(s):" % len(diffs))
        for d in diffs[:25]:
            print("  case %d %s  %s\n    base: %s\n    new : %s" % d)
        return 1
    print("ADDITIVE: every response field other than %s is byte-identical." %
          ", ".join(sorted(NEW_FIELDS)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
