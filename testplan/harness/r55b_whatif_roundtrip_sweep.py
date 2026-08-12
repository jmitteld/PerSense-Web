#!/usr/bin/env python3
"""r55b_whatif_roundtrip_sweep.py — the sweep r55 needed and did not have.

WHY THIS EXISTS
---------------
Round 55 closed mortgage finding 5 and swept three ENDPOINTS clean:
`r55_mtg_domain_sweep.py` reported NEWLY_REFUSED = 0 over 250 in-domain rows on
/calc, /compare and /whatif. It still shipped §97, an ANSWER->REFUSAL regression
on the What-If screen — because THE SHIPPED CLIENT DOES NOT STOP AT /whatif.
`runWhatIf` posts the base to /whatif, places the returned rows, and then reposts
EVERY GENERATED ROW through /api/mortgage/calc. A validator on /calc therefore
refuses generated values that /whatif itself happily produced, and
`calcGeneratedRows` blanks the row.

An endpoint sweep cannot see that. This sweeps THE CLIENT'S ROUND TRIP.

WHAT IT COMPARES
----------------
Three trees, because two is not enough to tell a fix from a coincidence:

  --pristine  pre-r55: no domain validators anywhere
  --regressed r55 as shipped: validators on EVERY /calc, generated or not
  --fixed     r55b: validators on TYPED rows only (MortgageRequest.Generated)

  regressed vs pristine -> MUST show NEWLY_REFUSED > 0.  This is the POSITIVE
                           CONTROL. If it is zero the sweep is not exercising
                           the defect and its other arm proves nothing (R49).
  fixed     vs pristine -> MUST be 0. DOS's CopyRowWithIncrement never enters
                           MortgageGridVerifyCellString, so a generated row must
                           behave exactly as it did before finding 5 landed.

🚨 NOT A PRODUCT RATE. The generator picks bases near a domain edge on purpose
so the round trip can leave the domain at all. Quote the buckets beside
`eligible` (R69), never the ratio.

Usage:
  r55b_whatif_roundtrip_sweep.py --pristine 8081 --regressed 8083 --fixed 8082 \
      --n 150 --seed 55155
"""
import argparse, json, random, sys, urllib.error, urllib.request

TIMEOUT = 15.0
# The five columns runWhatIf can vary, with the field's wire units.
VARY = ["rate", "years", "points", "pctDown", "price"]


def post(port, path, body):
    data = json.dumps(body).encode()
    req = urllib.request.Request(
        f"http://localhost:{port}{path}", data=data,
        headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as r:
            return r.status, json.loads(r.read().decode()), None
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read().decode()), None
        except Exception:
            return e.code, None, "unparsed"
    except TimeoutError:
        return None, None, "timeout"
    except urllib.error.URLError as e:
        return None, None, "timeout" if "timed out" in str(e).lower() else "transport"
    except Exception:
        return None, None, "transport"


def answered(status, parsed):
    """R61: a 200 CARRYING an error field is a refusal, not an answer."""
    return status == 200 and parsed is not None and not parsed.get("error")


def what_if_step_valid(key, v):
    """VERBATIM port of whatIfStepValid in cmd/persense/static/index.html.

    🚨 IT MUST STAY VERBATIM. Rows this drops are never placed and never
    reposted, so counting them would inflate the denominator with requests the
    client does not make (R54)."""
    if v is None:
        return False
    if key == "years":
        return v >= 1
    if key in ("rate", "price", "cash", "financed", "monthly"):
        return v > 0
    if key in ("points", "pctDown"):
        return v >= 0
    return True


def gen_base(rng):
    """A base row near a domain edge, so stepping can leave the domain.

    ⚠️ DELIBERATELY ENRICHED. DOS's own increment cannot be negative
    (KeyPressForIncrement strips the minus sign), so the reachable way out of a
    domain is stepping UP off a high base — which is what this models."""
    return {
        "price": round(rng.uniform(80_000, 700_000), 2),
        "pctDown": round(rng.uniform(0.55, 0.95), 4),
        "years": rng.choice([70, 80, 90, 95, 30]),
        "rate": round(rng.uniform(0.60, 0.95), 6),
        "points": round(rng.uniform(0.05, 0.09), 5),
    }


def round_trip(port, base, vary, inc, count):
    """Replicate runWhatIf: /whatif, filter, then repost each row to /calc.

    Returns (list_of_answered_flags, bucket_or_None). The generated:true marker
    is sent unconditionally — the pristine and regressed trees have no such
    field and json.Decode ignores it, so the three trees see the same bytes."""
    st, resp, b = post(port, "/api/mortgage/whatif",
                       {"base": base, "vary": vary,
                        "increment": inc, "count": count})
    if b:
        return None, b
    if not answered(st, resp):
        return [], None          # the table itself was refused: no round trip
    rows = (resp.get("rows") or [])[1:]
    out = []
    for r in rows:
        if not what_if_step_valid(vary, r.get(vary)):
            continue
        body = {k: r[k] for k in
                ("price", "points", "pctDown", "years", "rate")
                if r.get(k) is not None}
        body["generated"] = True
        st2, resp2, b2 = post(port, "/api/mortgage/calc", body)
        if b2:
            return None, b2
        out.append((answered(st2, resp2), (resp2 or {}).get("error", "")))
    return out, None


def compare(name, base_port, other_port, cases, rng_seed):
    rng = random.Random(rng_seed)
    buckets = dict(both_answer=0, both_refuse=0, NEWLY_REFUSED=0,
                   NEWLY_ANSWERED=0, timeout=0, transport=0, unparsed=0,
                   shape_mismatch=0)
    examples = []
    for base, vary, inc, count in cases:
        a, ba = round_trip(base_port, base, vary, inc, count)
        c, bc = round_trip(other_port, base, vary, inc, count)
        if ba or bc:
            buckets[ba or bc] += 1
            continue
        if len(a) != len(c):
            # Different row COUNTS mean the two trees disagreed before /calc.
            # That is a real difference but not an answer->refusal one; it gets
            # its own bucket rather than being silently zipped away.
            buckets["shape_mismatch"] += 1
            continue
        for (aa, _), (cc, cerr) in zip(a, c):
            if aa and cc:
                buckets["both_answer"] += 1
            elif not aa and not cc:
                buckets["both_refuse"] += 1
            elif aa and not cc:
                buckets["NEWLY_REFUSED"] += 1
                if len(examples) < 3:
                    examples.append((base, vary, inc, cerr[:70]))
            else:
                buckets["NEWLY_ANSWERED"] += 1
    eligible = (buckets["both_answer"] + buckets["both_refuse"] +
                buckets["NEWLY_REFUSED"] + buckets["NEWLY_ANSWERED"])
    print(f"\n=== {name} ===")
    print(f"  eligible {eligible}  both_answer {buckets['both_answer']}  "
          f"both_refuse {buckets['both_refuse']}  "
          f"NEWLY_REFUSED {buckets['NEWLY_REFUSED']}  "
          f"NEWLY_ANSWERED {buckets['NEWLY_ANSWERED']}")
    print(f"  t/o {buckets['timeout']}  transport {buckets['transport']}  "
          f"unparsed {buckets['unparsed']}  "
          f"shape_mismatch {buckets['shape_mismatch']}")
    for b, v, i, e in examples:
        print(f"    vary={v} inc={i} base={json.dumps(b)[:90]}\n      -> {e}")
    return buckets, eligible


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pristine", default="8081")
    ap.add_argument("--regressed", default="8083")
    ap.add_argument("--fixed", default="8082")
    ap.add_argument("--n", type=int, default=150)
    ap.add_argument("--seed", type=int, default=55155)
    a = ap.parse_args()

    print("argv:", " ".join(sys.argv))
    rng = random.Random(a.seed)
    cases = []
    for _ in range(a.n):
        vary = rng.choice(VARY)
        inc = {"rate": 0.05, "years": 8, "points": 0.01,
               "pctDown": 0.05, "price": 25_000.0}[vary]
        cases.append((gen_base(rng), vary, inc, rng.choice([4, 5, 6])))

    # FAIL CLOSED: the three ports must be three DIFFERENT trees.
    probe = {"price": 200000, "pctDown": 0.2, "years": 130, "rate": 0.06}
    pr = answered(*post(a.pristine, "/api/mortgage/calc", probe)[:2])
    rg = answered(*post(a.regressed, "/api/mortgage/calc", probe)[:2])
    gen = dict(probe, generated=True)
    fx_typed = answered(*post(a.fixed, "/api/mortgage/calc", probe)[:2])
    fx_gen = answered(*post(a.fixed, "/api/mortgage/calc", gen)[:2])
    print(f"probe years=130: pristine_answers={pr} regressed_answers={rg} "
          f"fixed_typed_answers={fx_typed} fixed_generated_answers={fx_gen}")
    if not (pr and not rg and not fx_typed and fx_gen):
        print("FATAL: the three ports are not the three trees. Expected "
              "pristine ANSWER, regressed REFUSE, fixed REFUSE-when-typed and "
              "ANSWER-when-generated. Refusing to score.")
        return 2

    ctl, ctl_elig = compare("POSITIVE CONTROL — regressed vs pristine "
                            "(MUST be > 0)", a.pristine, a.regressed,
                            cases, a.seed)
    fix, fix_elig = compare("THE FIX — r55b vs pristine (MUST be 0)",
                            a.pristine, a.fixed, cases, a.seed)

    print("\n🚨 ELIGIBLE is the denominator (R69). These buckets are a property "
          "of a generator that deliberately starts near a domain edge; they "
          "are NOT a product rate.")

    rc = 0
    if ctl["NEWLY_REFUSED"] == 0:
        print("\nFAIL: the positive control found NO regression, so this sweep "
              "is not exercising §97 and its other arm proves nothing (R49).")
        rc = 3
    else:
        print(f"\nPOSITIVE CONTROL OK: {ctl['NEWLY_REFUSED']} newly-refused "
              f"generated rows in {ctl_elig} eligible on the regressed tree.")
    if fix["NEWLY_REFUSED"] or fix["NEWLY_ANSWERED"]:
        print(f"FAIL: the fixed tree still differs from pristine on the "
              f"generated round trip "
              f"(NEWLY_REFUSED {fix['NEWLY_REFUSED']}, "
              f"NEWLY_ANSWERED {fix['NEWLY_ANSWERED']} in {fix_elig}).")
        rc = max(rc, 1)
    else:
        print(f"FIX OK: 0 differences in {fix_elig} eligible generated rows — "
              f"a generated row behaves exactly as it did before finding 5, "
              f"which is what DOS does.")
    for k in ("timeout", "transport", "unparsed"):
        if ctl[k] or fix[k]:
            print(f"FAIL: {k} bucket non-empty — a run that produced no output "
                  f"is not a zero (R64).")
            rc = max(rc, 1)
    print(f"\nexit {rc}")
    return rc


if __name__ == "__main__":
    sys.exit(main())
