#!/usr/bin/env python3
"""Generate an exhaustive set of Amortization UI test cases.

Each case is the exact JSON body the Amortization screen POSTs to
/api/amortization/calc (see getAmzInput in cmd/persense/static/index.html and
AmortizationRequest in internal/api/handlers.go), plus metadata. The Go test
internal/api/amort_ui_cases_test.go loads these, drives them through the real
handler, and checks the per-row schedule invariants.

Run:  python3 scripts/gen_amort_ui_cases.py
Out:  internal/api/testdata/amort_ui_cases.json
"""
import json, os

cases = []
def add(category, desc, req, expectError=False, expectPayment=None):
    c = {"id": f"AMZ-{len(cases)+1:03d}", "category": category, "desc": desc,
         "expectError": expectError, "request": req}
    if expectPayment is not None:
        c["expectPayment"] = expectPayment
    cases.append(c)

def base(**ov):
    r = {"amount": 100000.0, "loanDate": "2024-01-01", "rate": 0.06,
         "nPeriods": 360, "perYr": 12, "basis": "360"}
    r.update(ov)
    # omit Nones so the field is truly absent (the "blank/solve" signal)
    return {k: v for k, v in r.items() if v is not None}

# 1) Plain forward (solve payment): amount x rate x term x perYr x basis sample
amounts = [50000.0, 100000.0, 250000.0, 500000.0, 1000000.0, 12345.67]
rates   = [0.0, 0.025, 0.05, 0.08, 0.12, 0.18, 0.30]
terms   = [(360,12),(180,12),(120,12),(60,12),(24,12),(12,12),(120,4),(60,4),(30,1),(780,26),(1560,52)]
i = 0
for amt in amounts:
    for rt in rates:
        n, py = terms[i % len(terms)]
        b = ["360","365","365/360"][i % 3]
        add("plain-forward", f"solve payment amt={amt} rate={rt} n={n} perYr={py} basis={b}",
            base(amount=amt, rate=rt, nPeriods=n, perYr=py, basis=b))
        i += 1
# a couple of canonical golden-payment checks
add("plain-golden", "100k 8% 360 12 natural-first -> 733.76",
    base(amount=100000.0, rate=0.08, firstDate="2024-02-01"), expectPayment=733.76)
add("plain-golden", "250k 6% 360 12 -> 1498.88",
    base(amount=250000.0, rate=0.06, firstDate="2024-02-01"), expectPayment=1498.88)

# 2) First-period variations + firstIntPrepaid
add("first-period", "odd SHORT first period (loan 02/12, first 03/01)",
    base(loanDate="2024-02-12", firstDate="2024-03-01", rate=0.08), expectPayment=731.98)
add("first-period", "odd short first, firstIntPrepaid=false (no stub)",
    {**base(loanDate="2024-02-12", firstDate="2024-03-01", rate=0.08), "firstIntPrepaid": False})
add("first-period", "odd LONG first period (loan 01/01, first 04/01)",
    base(loanDate="2024-01-01", firstDate="2024-04-01", rate=0.08))
add("first-period", "blank first date -> engine default one period after loan",
    base(loanDate="2024-02-12"))
add("first-period", "natural first + firstIntPrepaid=false",
    {**base(firstDate="2024-02-01", rate=0.07), "firstIntPrepaid": False})

# 3) Backward solves
add("solve-amount", "omit amount, payment given -> solve amount",
    {**base(amount=None, firstDate="2024-02-01"), "payment": 1498.88, "rate": 0.06}, expectPayment=None)
add("solve-amount", "omit amount, 8% 360 payment 733.76",
    {**base(amount=None, firstDate="2024-02-01", rate=0.08), "payment": 733.76})
add("solve-rate", "omit rate, amount+payment given -> solve rate",
    {**base(rate=None, amount=250000.0, firstDate="2024-02-01"), "payment": 1498.88})
add("solve-rate", "omit rate, 100k payment 733.76 -> ~8%",
    {**base(rate=None, amount=100000.0, firstDate="2024-02-01"), "payment": 733.76})
add("solve-periods", "omit nPeriods (and lastDate), payment given -> solve term",
    {**base(nPeriods=None, amount=200000.0, rate=0.06, firstDate="2024-02-01"), "payment": 1199.10})
add("derive-term", "first+last dates given, omit nPeriods -> derive term then payment",
    base(nPeriods=None, lastDate="2054-01-01", firstDate="2024-02-01", rate=0.06))
add("solve-amount", "omit amount, blank first date (defaults) ",
    {**base(amount=None, loanDate="2024-01-01"), "payment": 600.0, "rate": 0.05})

# 4) Settings toggles
add("settings", "in-advance (annuity-due)", {**base(firstDate="2024-02-01", rate=0.08), "inAdvance": True})
add("settings", "exact + 365 basis", {**base(firstDate="2024-02-01", rate=0.08, basis="365"), "exact": True})
add("settings", "exact + 365/360 basis", {**base(firstDate="2024-02-01", rate=0.08, basis="365/360"), "exact": True})
add("settings", "rule-of-78", {**base(firstDate="2024-02-01", rate=0.08), "rule78": True})
add("settings", "USA rule (simple-interest)", {**base(firstDate="2024-02-01", rate=0.08), "usaRule": True})
add("settings", "points 2.0 -> APR computed", {**base(firstDate="2024-02-01", rate=0.08), "points": 0.02})
add("settings", "points 0 -> APR == yield baseline", {**base(firstDate="2024-02-01", rate=0.08), "points": 0.0})
add("settings", "in-advance + 365 basis", {**base(firstDate="2024-02-01", rate=0.08, basis="365"), "inAdvance": True})

# 5) Single advanced option
def adv(**kw):
    r = base(firstDate="2024-02-01", rate=0.08)
    r.update(kw); return r
add("balloon", "known $50k balloon at yr10, solve payment",
    adv(balloons=[{"date":"2034-01-01","amount":50000.0}]))
add("balloon", "known $100k balloon at yr15",
    adv(balloons=[{"date":"2039-01-01","amount":100000.0}]))
add("balloon", "balloon includes regular pmt (replaces), payment given",
    {**adv(balloons=[{"date":"2034-01-01","amount":50000.0}]), "payment":733.76, "balloonIncludesRegular":True})
add("balloon", "two balloons",
    adv(balloons=[{"date":"2030-01-01","amount":20000.0},{"date":"2040-01-01","amount":20000.0}]))
add("balloon-target", "balloon date only (omit amount) -> solve target balloon, payment given",
    {**adv(balloons=[{"date":"2034-01-01"}]), "payment":900.0})
add("prepay", "extra $200/mo 2025-2030",
    adv(prepayments=[{"startDate":"2025-01-01","stopDate":"2030-01-01","perYr":12,"amount":200.0}]))
add("prepay", "extra $500/mo for 24 pmts (NPmts-bounded)",
    adv(prepayments=[{"startDate":"2025-01-01","nPmts":24,"perYr":12,"amount":500.0}]))
add("prepay", "annual extra $5000 for 10 yrs",
    adv(prepayments=[{"startDate":"2025-01-01","nPmts":10,"perYr":1,"amount":5000.0}]))
add("prepay-solve", "omit prepay amount (NPmts-bounded) -> solve prepayment",
    adv(prepayments=[{"startDate":"2025-01-01","nPmts":60,"perYr":12}]))
add("arm", "ARM rate-only -> 9% at 2027 (re-solve payment)",
    adv(adjustments=[{"date":"2027-01-01","rate":0.09}]))
add("arm", "ARM rate down to 5% at 2030",
    adv(adjustments=[{"date":"2030-01-01","rate":0.05}]))
add("arm", "ARM payment-only -> $1000 at 2028",
    adv(adjustments=[{"date":"2028-01-01","amount":1000.0}]))
add("arm", "ARM rate+payment at 2029",
    adv(adjustments=[{"date":"2029-01-01","rate":0.07,"amount":900.0}]))
add("arm", "ARM re-amortize (date only, no rate/amount) at 2030",
    adv(adjustments=[{"date":"2030-01-01"}]))
add("arm", "two adjustments (2027 up, 2035 down)",
    adv(adjustments=[{"date":"2027-01-01","rate":0.10},{"date":"2035-01-01","rate":0.06}]))
add("moratorium", "interest-only until 2026",
    adv(moratorium="2026-01-01"))
add("moratorium", "interest-only until 2027",
    adv(moratorium="2027-01-01"))
add("target", "principal-minimum $250/period",
    adv(targetAmt=250.0))
add("target", "principal-minimum $150/period",
    adv(targetAmt=150.0))
add("target", "principal-minimum too high ($300 on 100k/360) -> ERROR",
    adv(targetAmt=300.0), expectError=True)
add("skip", "skip months 6-8 each year",
    adv(skipMonths="6-8"))
add("skip", "skip month 12 each year",
    adv(skipMonths="12"))
add("skip", "skip 1-3,7 each year",
    adv(skipMonths="1-3,7"))

# 6) Combinations
add("combo", "balloon + prepayment",
    adv(balloons=[{"date":"2034-01-01","amount":50000.0}],
        prepayments=[{"startDate":"2025-01-01","stopDate":"2030-01-01","perYr":12,"amount":100.0}]))
add("combo", "balloon + ARM",
    adv(balloons=[{"date":"2034-01-01","amount":40000.0}],
        adjustments=[{"date":"2028-01-01","rate":0.09}]))
add("combo", "moratorium + balloon",
    adv(moratorium="2026-01-01", balloons=[{"date":"2034-01-01","amount":30000.0}]))
add("combo", "target + skip (DOS: target overrides skip)",
    adv(targetAmt=200.0, skipMonths="7"))
add("combo", "prepay + skip",
    adv(prepayments=[{"startDate":"2025-01-01","nPmts":36,"perYr":12,"amount":150.0}], skipMonths="8"))
add("combo", "balloon + ARM + prepay",
    adv(balloons=[{"date":"2034-01-01","amount":50000.0}],
        adjustments=[{"date":"2028-01-01","rate":0.09}],
        prepayments=[{"startDate":"2025-01-01","stopDate":"2030-01-01","perYr":12,"amount":100.0}]))
add("combo", "two balloons + ARM",
    adv(balloons=[{"date":"2030-01-01","amount":15000.0},{"date":"2040-01-01","amount":15000.0}],
        adjustments=[{"date":"2035-01-01","rate":0.07}]))
add("combo", "balloon + in-advance",
    {**adv(balloons=[{"date":"2034-01-01","amount":50000.0}]), "inAdvance": True})
add("combo", "moratorium + ARM",
    adv(moratorium="2026-01-01", adjustments=[{"date":"2030-01-01","rate":0.09}]))
add("combo", "prepay + balloon-target (solve balloon w/ extra payments)",
    {**adv(balloons=[{"date":"2034-01-01"}],
           prepayments=[{"startDate":"2025-01-01","nPmts":24,"perYr":12,"amount":100.0}]), "payment":800.0})

# 7) Edge / boundary / error
add("edge", "rate 0% (interest-free)", base(rate=0.0, firstDate="2024-02-01"))
add("edge-error", "single period (n=1) -> engine requires >=2 installments",
    base(nPeriods=1, firstDate="2024-02-01", rate=0.08), expectError=True)
add("edge", "very long 50yr monthly (600)", base(nPeriods=600, firstDate="2024-02-01", rate=0.06))
add("edge", "very high rate 50%", base(rate=0.50, firstDate="2024-02-01"))
add("edge", "tiny amount 100.00", base(amount=100.0, firstDate="2024-02-01", rate=0.08))
add("edge", "weekly 52/yr 1560 (auto-365)", base(nPeriods=1560, perYr=52, rate=0.08))
add("edge", "biweekly 26/yr 780 (auto-365)", base(nPeriods=780, perYr=26, rate=0.08))
add("edge", "quarterly 4/yr 120", base(nPeriods=120, perYr=4, firstDate="2024-04-01", rate=0.08))
add("edge", "annual 1/yr 30", base(nPeriods=30, perYr=1, firstDate="2025-01-01", rate=0.08))
add("edge-error", "last payment before first -> ERROR",
    base(nPeriods=None, firstDate="2024-06-01", lastDate="2024-01-01", rate=0.06), expectError=True)
add("edge", "skip very wide 2-11 (only 1 paying month/yr)", adv(skipMonths="2-11"))
add("edge", "balloon larger than balance early (replace)",
    {**adv(balloons=[{"date":"2026-01-01","amount":120000.0}]), "balloonIncludesRegular":True})

os.makedirs("internal/api/testdata", exist_ok=True)
out = "internal/api/testdata/amort_ui_cases.json"
json.dump(cases, open(out,"w"), indent=1)
# category histogram
from collections import Counter
hist = Counter(c["category"] for c in cases)
print(f"wrote {len(cases)} cases to {out}")
for k,v in sorted(hist.items()): print(f"  {k:16s} {v}")
