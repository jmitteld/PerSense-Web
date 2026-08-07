#!/usr/bin/env python3
"""sec72_horizon_arm.py — IS THE ERA KEY A HORIZON KEY?  (round 36)

WHAT THIS MEASURES, AND WHY IT IS NOT sec71_ceiling_arm.py
----------------------------------------------------------
Round 35 opened §72 by reporting the faithful port at **3 divergences in 255
IN-SCOPE compared cases** on the ceiling family, against **0 in 1,707** on
`dos_fuzzer5` — and drew R31 from the contrast ("a zero is a statement about the
generator").  The three cases were annotated `last 2082`, `last 2044`,
`last 2065`, i.e. the era split was keyed on **`goamort bdump`'s `lastdate`**.

`lastdate` is the last REGULAR payment date.  It is NOT the walk's horizon.  A
prepayment series can carry the schedule decades past it, and on all three of
§72's cases it does:

    case            bdump lastdate      PORT horizon (fz5MaxYear)
    -89,085.58      2044                2100
    -1,109.52       2065                2116
    +4.68           2082                2109

    (⚠️ an earlier version of this table quoted 4/29/2104 for the first case.
    That is DOS's last row; the PORT retires that screen at 3/1/2100, and the
    difference is itself a finding — see §73.  Round-36 audit.)

`dos_fuzzer5_test.go`'s own `fz5MaxYear` — the definition every published
in-scope figure in this project is built on — takes the latest year in the
ENGINE'S answers: the schedule's last row, every balloon, and the resolved last
date.  By that definition all three are OUT of scope, and round 22 introduced
that definition for exactly this failure ("a screen carrying balloons at period
222 ... i.e. the year 2137" had been labelled in scope by a weaker key).

⚠️ AND `fz5MaxYear` IS ITSELF BIASED, THE OTHER WAY.  It includes the loan's
NOMINAL last regular payment date, which a prepayment-retired schedule never
reaches: a loan both engines finish in 2030 can carry a nominal LastDate in 2101
and be labelled out of scope for a date no row ever holds.  The ratified client
boundary is about the dates the schedule REACHES.  So this arm prints THREE
splits — lastdate, horizon and reached — and the reader is expected to read the
third when asking what a client sees.  (Round-36 adversarial audit.)

So this arm re-splits the ceiling family on ALL THREE keys and cross-tabulates
them against agreement.  It imports `sec71_ceiling_arm.build_screen` rather than
copying it, so it draws from the SAME GENERATOR by construction.

⚠️ IT IS NOT ROUND 35's SAMPLE.  `build_screen`'s DATE draw changed inside round
35 (the impossible-date fix), so seed 71 now yields the same money fields with
different dates, and round 35's "3 in 255" does not reproduce from this file on
either key.  This arm measures the same GENERATOR, not the same SAMPLE.  Do not
diff its cells against round 35's.  (Round-36 audit.)

THE HORIZON COMES FROM THE PORT, NOT FROM THIS SCRIPT (R2).
`cmd/goamort`'s new `horizon` token prints `horizon <year> lastdate <year>` from
the engine's own resolved answers.  A harness-computed date manufactures a
frontier — that rule has returned six defects, and this script is not going to
be the seventh.

WHAT A CLEAN RESULT LOOKS LIKE
------------------------------
If §72's divergences are all beyond the client's 2099 boundary, the cell
[in scope by horizon] x [diverged] is EMPTY and the arm says so.  That is a
RETRACTION of §72's headline, not a pass: the divergences are real, they are
simply out of the population the project has promised to match.

⚠️ AND THE ARM PRINTS THE THING THAT MAKES THAT RETRACTION INCOMPLETE.
A whole-case era label cannot express "the answer a 2024 user sees is wrong
because of arithmetic in 2104".  On case B the FIRST divergent row is row 0,
dated 9/29/2024 — the payment cell itself, because the payment is SOLVED over
the whole horizon.  So for every screen this arm calls out-of-scope-and-
divergent, it also reports the DATE OF THE FIRST DIVERGENT ROW, and counts how
many of those dates are themselves in scope.  That count is the exposure the
era rule hides.

Usage:
    sec72_horizon_arm.py --go /tmp/goamort --oracle /tmp/oraclebuild/amort_oracle \\
        [--n 500] [--seed 71] [--timeout 30]

--seed 71 --n 500 is round 35's generator and seed; see the caveat above about
the sample.  Run with and WITHOUT --engine dosport: the excluded piecewise
screens are not neutral.
"""

import argparse
import os
import re
import subprocess
import sys
import random
from collections import Counter

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from sec71_ceiling_arm import build_screen, run, agrees  # noqa: E402

# The client boundary (decisions_2026-08-03b_client_2099_boundary.md).
IN_SCOPE_MAX_YEAR = 2099


def port_engine(binary, toks, timeout):
    """Which ENGINE answered (R27, note #27).

    ⚠️ NOT OPTIONAL BEFORE QUOTING A FAITHFUL-PORT RATE. This family is drawn on
    long terms and long prepayment series, and a good share of it routes to the
    PIECEWISE fallback, which is a different engine with a different defect
    profile and no horizon clamp at all. Round 35's §72 figure was filtered to
    `dosport`; a rate over "whatever answered" is a different population wearing
    the same name. Returns the engine name or None.
    """
    e = dict(os.environ)
    e["DPTRACEENGINE"] = "1"
    try:
        p = subprocess.run([binary] + toks, capture_output=True, text=True,
                           timeout=timeout, env=e)
    except subprocess.TimeoutExpired:
        return None
    for line in (p.stderr or "").splitlines():
        w = line.split()
        if len(w) >= 2 and w[0] == "GENGINE":
            return w[1]
    return None


def port_keys(binary, toks, timeout):
    """Ask the PORT for its own horizon and lastdate years.

    Returns (horizon, reached, lastdate), or (None, None, None) when the port
    prints no horizon line.  A refusal is never folded into a year: an absent key
    must not be silently bucketed as in-scope, which is the direction that
    flatters the headline.

    ⚠️ AND AN ABSENT KEY IS NOT NEUTRAL EITHER. main.go returns before the
    horizon block when the engine errors, so a port REFUSAL lands here too — and
    a screen where DOS answers and the port refuses is §71's own class, the most
    damning divergence there is. The round-36 audit found the first cut of this
    arm dropping those without ever asking the oracle. main() now asks.
    """
    try:
        p = subprocess.run([binary] + toks + ["horizon"], capture_output=True,
                           text=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        return None, None, None
    for line in (p.stdout or "").splitlines():
        w = line.split()
        if len(w) == 6 and w[0] == "horizon" and w[2] == "reached" and w[4] == "lastdate":
            try:
                return int(w[1]), int(w[3]), int(w[5])
            except ValueError:
                return None, None, None
    return None, None, None


# ⚠️ THE HALF-CENT THRESHOLD FINDS RENDERING, NOT DIVERGENCE.
#
# Round 36's first cut of this arm reported "53 of 54 out-of-scope divergent
# screens have their first divergent row IN SCOPE" at a half-cent threshold, and
# the very first case checked by hand was
#   DOS  L0 12/30/25  -369.24     PORT row 1 12/30/25  -369.25
# — a ONE CENT difference in DOS's RENDERED cents, on a screen whose totals
# differ by 23.05. That is the standing trap "THE ORACLE'S `row` FIGURES ARE
# DOS'S RENDERED CENTS" plus "A HALF-CENT TIE IS NOT A DIVERGENCE", and it would
# have turned a rounding floor into a headline about client exposure.
#
# So the exposure is reported at THREE thresholds and the reader is shown the
# whole ladder. A claim that survives only at 0.005 is a claim about rendering.
ROW_THRESHOLDS = (0.005, 0.05, 1.00)

# DOS renders "L10| 4/ 7/26 2126.80 478.28 1648.52 242128.96 8477.47".
DOS_ROW_RE = re.compile(r"^\s*(\d+)/\s*(\d+)/\s*(\d+)\s+(.*)$")


def first_divergent_row_dates(oracle, go, toks, timeout):
    """Date of the first row where DOS and the port disagree, PER THRESHOLD.

    Aligned positionally over PAYMENT ROWS ONLY, the same walk
    localise_divergent_row.py makes (defect #15: DOS interleaves announcement
    and separator lines into the same numbered stream).  Returns a (m, d, y)
    triple with a 4-digit year, or None.

    The port's rows carry a 2-DIGIT year (GOAMORT_ROWDATES mirrors DOS's
    rendering), so the century is reconstructed from the port's own horizon:
    years run forward from the loan date, so a 2-digit year that would fall
    before the loan year belongs to the next century.  This is arithmetic on
    the ENGINE'S output, not a second date derivation.
    """
    env = dict(os.environ)
    env["GOAMORT_ALLROWS"] = "1"
    env["GOAMORT_ROWDATES"] = "1"
    try:
        gp = subprocess.run([go] + toks + ["rows"], capture_output=True,
                            text=True, timeout=timeout, env=env)
        # ⚠️ THE ORACLE NEEDS THE SOFT-EMESSAGE GATE HERE TOO (note #32). The
        # first cut of this arm passed it to the SCORING run and not to this one,
        # so on ceiling-family screens `dumpraw` emitted three lines of refusal,
        # drows came back empty, and the screen vanished from the exposure count
        # with no bucket at all. Found by the round-36 audit; 2 of 60 screens,
        # and the direction was to flatter the retraction.
        oe = dict(os.environ)
        oe["PERSENSE_ORACLE_SOFT_EMESSAGE"] = "1"
        op = subprocess.run([oracle] + toks + ["dumpraw"], capture_output=True,
                            text=True, timeout=timeout, env=oe)
    except subprocess.TimeoutExpired:
        return None
    grows = []
    for line in (gp.stdout or "").splitlines():
        w = line.split()
        # ⚠️ COLUMN INDICES, AND ROUND 36 GOT THEM WRONG ONCE.
        # GOAMORT_ROWDATES emits
        #   row N M/D/YY pay P int I prin R bal B
        # so pay is w[4] and PRINCIPAL is w[8] -- w[6] is INTEREST. The first cut
        # of this arm read w[6] as principal and compared the port's interest
        # against DOS's principal, which made EVERY screen diverge at row 0 and
        # produced a flat "53 of 54" at all three thresholds. An instrument's
        # error can damn the port as easily as flatter it.
        if len(w) >= 11 and w[0] == "row":
            try:
                m, d, y = (int(x) for x in w[2].split("/"))
                grows.append((m, d, y, float(w[4]), float(w[8])))
            except ValueError:
                continue
    # ⚠️ DOS SPACE-PADS SINGLE-DIGIT MONTHS AND DAYS: "L10| 4/ 7/26 2126.80 ...".
    # `body.split()` then yields w[0] == "4/", which still contains "/" and so
    # passes a naive guard, and int("") raises — so the row is DISCARDED. The
    # port's rows are never padded, so every later index is shifted and the two
    # sequences stop describing the same payment. The first cut of this arm did
    # exactly that and dropped 652 of 56,573 DOS rows, up to 21% on a single
    # screen, concentrated on the SUB-MONTHLY (peryr 26/52) prepayment grids this
    # arm exists to probe. It reported one screen's first material divergence as
    # 4/21/2026 when it is 7/18/2056. This is the standing trap "a space-padded
    # field breaks a \d+ parser and the failure looks like a finding", and it had
    # already been written up once, in round 33. Found by the round-36 audit.
    drows = []
    for line in (op.stdout or "").splitlines():
        if not line.startswith("L") or "|" not in line:
            continue
        mm = DOS_ROW_RE.match(line.split("|", 1)[1])
        if not mm:
            continue
        try:
            m, d, y = int(mm.group(1)), int(mm.group(2)), int(mm.group(3))
            w = mm.group(4).split()
            if len(w) < 4:
                continue
            drows.append((m, d, y, float(w[0]), float(w[2])))
        except ValueError:
            continue
    # TWO ROW SETS ARE NOT COMPARABLE BY INDEX unless they are the same length.
    # A silent length difference is how the padded-date defect stayed invisible.
    if len(grows) != len(drows):
        return {"__lenmismatch__": (len(grows), len(drows))}
    # ⚠️ ROUND-37 AUDIT, F1. The first cut of this walk stored DOS's RAW 2-digit
    # row year as century()'s prev_year, whose contract requires the previous
    # row's EXPANDED 4-digit year. With a floor like 26 the anti-wrap loop could
    # never fire, the expansion degenerated to exactly the naive `base + yy`
    # version century()'s own docstring warns WRAPS, and a first-divergent row
    # genuinely dated 2100+ was counted as in-scope exposure — visibly: the
    # exposure list printed a row dated 2/29/2000, which is DOS's fake 29
    # February 2100 (§73's own trigger date) mis-expanded by a century. The
    # published "48 of 53" carried 16 such artifacts; the real figure is 32/53
    # (dosport) / 33/59 (product). Round 36 listed this hazard as found and
    # fixed; the fix never worked and nothing executed century() in a test
    # (R20: a fix that changes nothing has not been confirmed; R35: the guard
    # must be an assertion). The years are now expanded ONCE, by expand_years(),
    # which _selftest_century() executes on every invocation of this arm.
    loan_year = int([t for t in toks
                     if t.startswith("loandmy=")][0].split(".")[-1])
    dyears = expand_years([o[2] for o in drows], loan_year)
    out = {}
    for thr in ROW_THRESHOLDS:
        for i in range(len(drows)):
            g, o = grows[i], drows[i]
            if abs(g[3] - o[3]) > thr or abs(g[4] - o[4]) > thr:
                out[thr] = (o[0], o[1], dyears[i])
                break
    # A length difference IS a divergence, but it has no single row date, so it
    # is simply absent from `out` rather than folded into one.
    return out


def century(yy, loan_year, prev_year=None):
    """Expand DOS's 2-digit year, carrying the century FORWARD monotonically.

    ⚠️ THE NAIVE VERSION WRAPS. `base + yy, +100 if below the loan year` maps a
    DOS row dated ".24" on a 2024 loan to 2024 — but this population contains
    horizons up to 2154, i.e. spans over 100 years, and 110 of 497 screens have a
    horizon >= loan_year + 100. Such a row would be relabelled a century early and
    counted as IN-SCOPE exposure: the direction that DAMNS the port. Found as a
    hazard by the round-36 audit before it fired.

    `prev_year` is the previous row's already-expanded 4-digit year; a schedule's
    dates are non-decreasing, so the century is whichever candidate is >= it.
    """
    base = (loan_year // 100) * 100
    y = base + yy
    floor = loan_year if prev_year is None else prev_year
    while y < floor:
        y += 100
    return y


def expand_years(yys, loan_year):
    """Expand a whole schedule's 2-digit years, monotonically, in ONE place.

    The carry passed to century() is the previous row's EXPANDED year — never
    the raw 2-digit one. Round-37 audit, F1: a raw carry makes century()'s
    anti-wrap floor unreachable and the expansion silently degenerates to the
    naive wrapping version. Every consumer of row years goes through here so
    the carry cannot be re-typed wrong at a call site.
    """
    out, prev4 = [], None
    for yy in yys:
        y4 = century(yy, loan_year, prev4)
        # R35 (round-38 reviewer): monotonicity is an ASSUMPTION about DOS
        # schedules — assert it, so an out-of-order row poisons the run loudly
        # instead of silently pushing every later row out of scope (+100).
        assert prev4 is None or 0 <= y4 - prev4 < 90, \
            "non-monotone or jumping year expansion: %r after %r" % (y4, prev4)
        out.append(y4)
        prev4 = y4
    return out


def _selftest_century():
    """R35 — the trap is an ASSERTION, executed on every invocation of the arm.

    A synthetic schedule spanning >100 years, on a 2024 loan:
    .26 .99 .00 .26 .54 must expand to 2026 2099 2100 2126 2154. The broken
    raw-2-digit carry expands it to 2026 2099 2100 2126 2154 ONLY if the carry
    is the expanded year; with the raw carry the floor never fires and .00
    lands on 2000 and the second .26 on 2026 — the exact artifact that put 16
    century-wrapped rows into the published exposure count (round-37 audit F1).
    """
    assert century(26, 2024) == 2026
    assert century(0, 2024) == 2100, "bare wrap below the loan year must carry"
    assert century(26, 2024, 2100) == 2126, \
        "the case the raw carry broke: prev=2100 must lift .26 to 2126"
    got = expand_years([26, 99, 0, 26, 54], 2024)
    assert got == [2026, 2099, 2100, 2126, 2154], got
    # §73's own trigger date must expand to 2100, not 2000:
    assert expand_years([26, 0], 2024) == [2026, 2100]
    # Round-38 reviewer: a ONE-ROW-LAGGED carry passes every assertion above
    # (no row there strictly needs the immediately-previous carry). This pair
    # does need it: .99 then .26 must be 2099 -> 2126, and a lagged carry
    # yields 2026.
    assert expand_years([99, 26], 2024) == [2099, 2126]


def main():
    _selftest_century()
    ap = argparse.ArgumentParser()
    ap.add_argument("--go", required=True)
    ap.add_argument("--oracle", required=True)
    ap.add_argument("--n", type=int, default=500)
    ap.add_argument("--seed", type=int, default=71)
    ap.add_argument("--timeout", type=int, default=30)
    ap.add_argument("--rowdates", action="store_true",
                    help="locate the first divergent row on every divergent "
                         "screen (slower; the exposure count needs it)")
    ap.add_argument("--engine", default="",
                    help="keep only screens this engine answered, e.g. dosport "
                         "(R27). Round 35's §72 figure was dosport-only; a rate "
                         "over 'whatever answered' is a different population.")
    a = ap.parse_args()

    # The oracle needs the soft-EMessage gate on this family (note #32) or every
    # ceiling-crossing screen reads as a refusal.
    oenv = {"PERSENSE_ORACLE_SOFT_EMESSAGE": "1"}

    rng = random.Random(a.seed)
    cells = Counter()          # (scope_by_horizon, scope_by_lastdate, diverged)
    rcells = Counter()         # (scope_by_REACHED, diverged)
    port_refusals = []
    other_engine_rows = []
    buckets = Counter()
    engines = Counter()
    disagreeing_keys = 0
    exposed = {t: [] for t in ROW_THRESHOLDS}  # per threshold
    oos_divergent = 0
    inscope_divergent = []     # IN SCOPE BY HORIZON and divergent — the real ones

    for i in range(a.n):
        toks, _ppy = build_screen(rng)
        hz, rc, ld = port_keys(a.go, toks, a.timeout)
        eng = port_engine(a.go, toks, a.timeout)
        engines[eng or "unknown"] += 1
        if hz is None:
            # ⚠️ ASK THE ORACLE BEFORE DROPPING. A port refusal against a DOS
            # ANSWER is §71's own class and the most damning divergence there is;
            # the first cut of this arm `continue`d here without ever running the
            # oracle. Found by the round-36 audit, which then produced exactly
            # such a screen on seed 90210.
            d0 = run(a.oracle, toks, a.timeout, env=oenv)
            if d0[0] == "ok":
                buckets["PORT_REFUSED_DOS_ANSWERED"] += 1
                port_refusals.append((toks, eng))
            else:
                buckets["both_declined"] += 1
            continue
        if a.engine and eng != a.engine:
            buckets["other_engine"] += 1
            other_engine_rows.append((toks, hz, rc, ld, eng))
            continue
        g = run(a.go, toks, a.timeout)
        d = run(a.oracle, toks, a.timeout, env=oenv)
        if g[0] != "ok" or d[0] != "ok":
            buckets["not_scorable:%s/%s" % (g[0], d[0])] += 1
            continue
        # ⚠️ THE ORACLE'S REFUSAL SENTINEL IS NOT AN ANSWER. amort_oracle prints
        # `payment 0.0000 interest -1.00 paid -1.00` when it declines the totals;
        # run()/agrees() happily score that as a divergence against any real
        # figure. One of the round-36 audit's 54 "divergences" was this.
        if d[1][0] == -1.00 and d[1][1] == -1.00:
            buckets["oracle_nototals_sentinel"] += 1
            continue
        div = not agrees(g, d)
        sh = "in" if hz <= IN_SCOPE_MAX_YEAR else "out"
        sl = "in" if ld <= IN_SCOPE_MAX_YEAR else "out"
        if sh != sl:
            disagreeing_keys += 1
        cells[(sh, sl, div)] += 1
        rcells[("in" if rc <= IN_SCOPE_MAX_YEAR else "out", div)] += 1
        buckets["compared"] += 1
        if div and sh == "in":
            inscope_divergent.append((toks, hz, ld, eng, g[1], d[1]))
        if div and sh == "out":
            oos_divergent += 1
            if a.rowdates:
                rows = first_divergent_row_dates(a.oracle, a.go, toks, a.timeout)
                if rows is None or "__lenmismatch__" in rows:
                    buckets["rowdates_unusable"] += 1
                else:
                    # r[2] is ALREADY the expanded 4-digit year — expansion
                    # happens once, inside first_divergent_row_dates, via
                    # expand_years() (round-37 audit F1). Re-expanding here was
                    # where the broken raw carry lived.
                    for thr, r in rows.items():
                        y4 = r[2]
                        if y4 <= IN_SCOPE_MAX_YEAR:
                            exposed[thr].append((toks, r[0], r[1], y4))

    print("=== sec72_horizon_arm — the era key vs the horizon key ===")
    print("population: sec71_ceiling_arm.build_screen, seed=%d, n=%d" % (a.seed, a.n))
    print()
    for k in sorted(buckets):
        print("  %-28s %d" % (k, buckets[k]))
    print()
    print("  engines that answered (R27):")
    for k in sorted(engines):
        print("    %-24s %d" % (k, engines[k]))
    print("  engine filter: %s" % (a.engine or "(none — DO NOT quote this as a "
                                              "faithful-port rate)"))
    print()
    print("  screens where the TWO KEYS DISAGREE about scope: %d" % disagreeing_keys)
    print()
    print("  %-22s %-10s %-10s %s" % ("scope(horizon)", "scope(last)", "diverged", "cases"))
    for (sh, sl, div) in sorted(cells):
        print("  %-22s %-10s %-10s %d" % (sh, sl, div, cells[(sh, sl, div)]))
    print()

    def rate(sh):
        tot = sum(v for (h, _l, _d), v in cells.items() if h == sh)
        bad = sum(v for (h, _l, d), v in cells.items() if h == sh and d)
        return bad, tot

    for label, key in (("BY HORIZON (fz5MaxYear — the project's own definition)", "h"),
                       ("BY LASTDATE (what round 35's §72 used)", "l")):
        if key == "h":
            bi, ti = rate("in")
            bo, to = rate("out")
        else:
            ti = sum(v for (_h, l, _d), v in cells.items() if l == "in")
            bi = sum(v for (_h, l, d), v in cells.items() if l == "in" and d)
            to = sum(v for (_h, l, _d), v in cells.items() if l == "out")
            bo = sum(v for (_h, l, d), v in cells.items() if l == "out" and d)
        print("  %s" % label)
        print("     IN  SCOPE: %d diverged / %d compared%s" %
              (bi, ti, "" if not ti else "  (1 in %.0f)" % (ti / bi) if bi else "  (none)"))
        print("     OUT SCOPE: %d diverged / %d compared%s" %
              (bo, to, "" if not to else "  (1 in %.0f)" % (to / bo) if bo else "  (none)"))
        print()

    print("  BY REACHED (max over rows and balloons — what the walk PRODUCES,")
    print("              and what decisions_2026-08-03b actually says)")
    ri = sum(v for (k, _d), v in rcells.items() if k == "in")
    rib = sum(v for (k, d), v in rcells.items() if k == "in" and d)
    ro = sum(v for (k, _d), v in rcells.items() if k == "out")
    rob = sum(v for (k, d), v in rcells.items() if k == "out" and d)
    print("     IN  SCOPE: %d diverged / %d compared" % (rib, ri))
    print("     OUT SCOPE: %d diverged / %d compared" % (rob, ro))
    print("  ⚠️ `horizon` includes the loan's NOMINAL last regular payment date,")
    print("  which an early-retiring schedule NEVER REACHES. A four-year loan can")
    print("  carry a nominal LastDate 71 years after its final row and be labelled")
    print("  out of scope for a date no engine ever prints. Read this block, not")
    print("  the one above, when asking what the CLIENT sees.")
    print()
    if port_refusals:
        print("  🚨 PORT REFUSED WHERE DOS ANSWERED (§71's class): %d" % len(port_refusals))
        for toks, eng in port_refusals[:5]:
            print("      engine=%s  %s" % (eng, " ".join(toks)))
        print()
    if a.engine and other_engine_rows:
        print("  ⚠️ %d screens were EXCLUDED by --engine %s. They are not neutral:" %
              (len(other_engine_rows), a.engine))
        print("     the piecewise fallback is a different engine with no horizon")
        print("     clamp, and a rate quoted over `dosport` alone is a rate over")
        print("     ~96%% of the router, not over the product. Re-run without")
        print("     --engine for the product-level figure.")
        print()
    print("  === IN SCOPE BY HORIZON *AND* DIVERGENT — the ones that count ===")
    if not inscope_divergent:
        print("     none")
    for toks, hz, ld, eng, gv, dv in inscope_divergent:
        print("     horizon=%d lastdate=%d engine=%s" % (hz, ld, eng))
        print("       port %.2f / %.2f   DOS %.2f / %.2f   (%+.2f)" %
              (gv[0], gv[1], dv[0], dv[1], gv[0] - dv[0]))
        print("       %s" % " ".join(toks))
    print()

    if a.rowdates:
        print("  === THE EXPOSURE THE ERA RULE HIDES ===")
        print("  out-of-scope divergent screens: %d" % oos_divergent)
        print("  ... of which the first divergent row is itself IN SCOPE,")
        print("      as a function of what counts as a divergent ROW:")
        for thr in ROW_THRESHOLDS:
            print("        > %-6s : %d" % (("%.3f" % thr), len(exposed[thr])))
        print()
        print("  ⚠️ READ THE LADDER, NOT THE TOP ROW. DOS renders row figures in")
        print("  CENTS, so a 0.005 threshold reports the RENDERING FLOOR: round")
        print("  36's first cut published 53/54 at 0.005 and the first case")
        print("  checked by hand was a one-cent difference. Only the rows that")
        print("  survive the 1.00 threshold are exposure a client would notice.")
        print()
        for toks, m, d, y in exposed[ROW_THRESHOLDS[-1]][:10]:
            print("      %s/%s/%s  %s" % (m, d, y, " ".join(toks)))
        if len(exposed[ROW_THRESHOLDS[-1]]) > 10:
            print("      ... and %d more" % (len(exposed[ROW_THRESHOLDS[-1]]) - 10))
        print()
        print("  A whole-case era label cannot express a defect whose CAUSE is")
        print("  out of scope and whose EFFECT is in scope — but the size of that")
        print("  exposure is the 1.00 row above, not the 0.005 one.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
