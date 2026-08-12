#!/usr/bin/env python3
"""r55_mtg_domain_sweep.py — the R75 endpoint sweep for mortgage FINDING 5.

WHY THIS EXISTS
---------------
Finding 5 ports MortgageScreenUnit.pas:1202-1250's five per-cell domain
validators, which the port never had. A domain validator TURNS ANSWERS INTO
REFUSALS BY DESIGN, and R75 (round 54) says that delta must be SWEPT ACROSS
EVERY ENDPOINT THAT SHARES THE DECODER, ON BOTH TREES, and published WITH ITS
ELIGIBLE COUNT (R69) — because r54 shipped two such regressions that its tests
did not catch and only an endpoint sweep found.

Three endpoints share the mortgage input decoding: /api/mortgage/calc (via
MortgageRequest) and /api/mortgage/compare and /api/mortgage/whatif (via
mortgageLineInput). All three are swept.

WHAT IT IS NOT
--------------
🚨 NOT A PRODUCT RATE. The generator deliberately over-samples out-of-domain
values so the exposed cell is populated at all; the ANSWER->REFUSAL rate below
is a property of THIS generator, not of the screen. Quote the buckets, never
the ratio, and always beside `eligible`.

BUCKETS (every request lands in exactly one, R61/R64):
  both_answer      pristine 200 and fixed 200
  both_refuse      pristine non-200 and fixed non-200
  NEWLY_REFUSED    pristine 200, fixed non-200   <- the R67/R75 delta
  NEWLY_ANSWERED   pristine non-200, fixed 200   <- must be 0
  timeout / transport / unparsed                 <- NEVER folded into a refusal

Usage:
  r55_mtg_domain_sweep.py --pristine 8081 --fixed 8082 --n 400 --seed 55055
  r55_mtg_domain_sweep.py --selftest       # positive control, see below
"""
import argparse, hashlib, json, os, random, sys, urllib.error, urllib.request

TIMEOUT = 15.0


def md5(path):
    try:
        h = hashlib.md5()
        with open(path, "rb") as fh:
            for chunk in iter(lambda: fh.read(1 << 20), b""):
                h.update(chunk)
        return h.hexdigest()
    except OSError as e:
        return f"<unreadable: {e}>"


def post(port, path, body):
    """Return (status, parsed_or_None, bucket_override_or_None)."""
    data = json.dumps(body).encode()
    req = urllib.request.Request(
        f"http://localhost:{port}{path}", data=data,
        headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as r:
            raw = r.read()
            status = r.status
    except urllib.error.HTTPError as e:          # 4xx/5xx still carry a body
        raw, status = e.read(), e.code
    except TimeoutError:
        return None, None, "timeout"
    except urllib.error.URLError as e:
        if "timed out" in str(e).lower():
            return None, None, "timeout"
        return None, None, "transport"
    except Exception:
        return None, None, "transport"
    try:
        return status, json.loads(raw.decode()), None
    except Exception:
        return status, None, "unparsed"


def answered(status, parsed):
    """An ANSWER is a 200 with no error field. R61: a refusal is not a hang,
    and an error carried inside a 200 is still a refusal."""
    if status != 200 or parsed is None:
        return False
    return not parsed.get("error")


def gen_row(rng, force_bad):
    """One mortgage row on the WIRE (fractions for rate/pctDown/points).

    force_bad drives a value out of the DOS domain so the exposed cell is
    populated; without it the in-domain draws dominate and the sweep would
    measure nothing (R49: a control can be inert because the sample cannot
    express the difference)."""
    row = {
        "price": round(rng.uniform(50_000, 900_000), 2),
        "pctDown": round(rng.uniform(0.0, 0.60), 4),
        "years": rng.choice([5, 10, 15, 20, 25, 30, 40]),
        "rate": round(rng.uniform(0.0, 0.22), 6),
    }
    if rng.random() < 0.5:
        row["points"] = round(rng.uniform(0.0, 0.06), 5)
    if rng.random() < 0.3:
        row["tax"] = round(rng.uniform(0, 600), 2)
    if rng.random() < 0.25:
        row["balloonYears"] = rng.choice([3, 5, 7, 10, 15])
    if force_bad:
        which = rng.choice(["points", "pctDown", "years", "rate", "balloonYears"])
        row[which] = {
            "points": rng.choice([0.10, 0.25, 1.5, -0.01]),
            "pctDown": rng.choice([1.0, 1.5, 5.0, -0.10]),
            "years": rng.choice([100, 250, 500, -5]),
            "rate": rng.choice([1.0, 6.0, 500.0, -1.0, -3.0]),
            "balloonYears": rng.choice([99, 120, 400, -2]),
        }[which]
    return row


ENDPOINTS = ("calc", "compare", "whatif")


def build_request(kind, row, rng):
    if kind == "calc":
        return "/api/mortgage/calc", dict(row)
    if kind == "compare":
        other = dict(row)
        other["rate"] = round(min(max(row.get("rate", 0.06) + 0.01, 0.0), 0.2), 6)
        return "/api/mortgage/compare", {"a": dict(row), "b": other}
    return "/api/mortgage/whatif", {
        "base": dict(row),
        "vary": rng.choice(["rate", "years", "points", "pctDown", "price"]),
        "increment": 0.005, "count": 5}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pristine", default="8081")
    ap.add_argument("--fixed", default="8082")
    ap.add_argument("--n", type=int, default=400)
    ap.add_argument("--seed", type=int, default=55055)
    ap.add_argument("--bad-frac", type=float, default=0.5)
    ap.add_argument("--selftest", action="store_true",
                    help="POSITIVE CONTROL: post only out-of-domain rows. "
                         "NEWLY_REFUSED must be > 0 on every endpoint, or the "
                         "sweep cannot see the thing it is measuring.")
    ap.add_argument("--binaries", nargs="*", default=[])
    a = ap.parse_args()

    rng = random.Random(a.seed)
    bad_frac = 1.0 if a.selftest else a.bad_frac

    print("argv:", " ".join(sys.argv))
    print(f"seed={a.seed} n={a.n} bad_frac={bad_frac} "
          f"pristine=:{a.pristine} fixed=:{a.fixed}")
    for b in a.binaries:
        print(f"  binary {b} md5={md5(b)}")

    # FAIL CLOSED: both servers must be alive and DISTINGUISHABLE before any
    # verdict is computed. Two ports serving the same binary would print a
    # clean green NEWLY_REFUSED=0 that means nothing.
    probe = {"price": 200000, "pctDown": 0.2, "years": 500, "rate": 0.06}
    ps, pp, pb = post(a.pristine, "/api/mortgage/calc", probe)
    fs, fp, fb = post(a.fixed, "/api/mortgage/calc", probe)
    if pb or fb:
        print(f"FATAL: liveness probe failed (pristine={pb}, fixed={fb})")
        return 2
    if not (answered(ps, pp) and not answered(fs, fp)):
        print("FATAL: the two ports are not the two trees — the pristine port "
              f"must ANSWER years:500 (got {ps}) and the fixed port must "
              f"REFUSE it (got {fs}). Refusing to score.")
        return 2
    print("liveness+distinguishability probe OK "
          "(pristine answers years:500; fixed refuses it)\n")

    buckets = {ep: {k: 0 for k in ("both_answer", "both_refuse",
                                   "NEWLY_REFUSED", "NEWLY_ANSWERED",
                                   "timeout", "transport", "unparsed")}
               for ep in ENDPOINTS}
    examples = {ep: [] for ep in ENDPOINTS}

    for i in range(a.n):
        row = gen_row(rng, rng.random() < bad_frac)
        for ep in ENDPOINTS:
            path, body = build_request(ep, row, rng)
            ps, pp, pb = post(a.pristine, path, body)
            fs, fp, fb = post(a.fixed, path, body)
            if pb or fb:
                buckets[ep][pb or fb] += 1
                continue
            pa, fa = answered(ps, pp), answered(fs, fp)
            if pa and fa:
                buckets[ep]["both_answer"] += 1
            elif not pa and not fa:
                buckets[ep]["both_refuse"] += 1
            elif pa and not fa:
                buckets[ep]["NEWLY_REFUSED"] += 1
                if len(examples[ep]) < 5:
                    examples[ep].append(
                        (body, (fp or {}).get("error", "")[:80]))
            else:
                buckets[ep]["NEWLY_ANSWERED"] += 1
                if len(examples[ep]) < 5:
                    examples[ep].append((body, "NEWLY ANSWERED"))

    rc = 0
    print(f"{'endpoint':10s} {'eligible':>9s} {'both_ans':>9s} {'both_ref':>9s} "
          f"{'NEW_REF':>8s} {'NEW_ANS':>8s} {'t/o':>4s} {'txp':>4s} {'unp':>4s}")
    for ep in ENDPOINTS:
        b = buckets[ep]
        eligible = b["both_answer"] + b["both_refuse"] + \
            b["NEWLY_REFUSED"] + b["NEWLY_ANSWERED"]
        print(f"{ep:10s} {eligible:9d} {b['both_answer']:9d} "
              f"{b['both_refuse']:9d} {b['NEWLY_REFUSED']:8d} "
              f"{b['NEWLY_ANSWERED']:8d} {b['timeout']:4d} "
              f"{b['transport']:4d} {b['unparsed']:4d}")
        if b["NEWLY_ANSWERED"]:
            rc = 1
        if b["timeout"] or b["transport"] or b["unparsed"]:
            rc = max(rc, 1)

    print("\n🚨 ELIGIBLE is the denominator for every figure above (R69). "
          "NEWLY_REFUSED is the R67/R75 delta this fix introduces BY DESIGN; "
          "it is NOT a defect count and NOT a product rate (this generator "
          "over-samples out-of-domain values on purpose).")

    for ep in ENDPOINTS:
        if examples[ep]:
            print(f"\n--- {ep}: sample NEWLY_REFUSED ---")
            for body, err in examples[ep][:3]:
                print(f"    {json.dumps(body)[:150]}\n      -> {err}")

    if a.selftest:
        bad = [ep for ep in ENDPOINTS if buckets[ep]["NEWLY_REFUSED"] == 0]
        if bad:
            print(f"\nSELFTEST FAIL: NEWLY_REFUSED == 0 on {bad} — the sweep "
                  "cannot see the change it exists to measure (R49).")
            return 3
        print("\nSELFTEST PASS: every endpoint shows NEWLY_REFUSED > 0, so a "
              "zero on the main run would be a real zero.")
    print(f"\nexit {rc}")
    return rc


if __name__ == "__main__":
    sys.exit(main())
