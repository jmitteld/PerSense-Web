#!/usr/bin/env python3
"""Generate the expanded actuarial third-party reference (testdata/sult_reference.json).

Everything is computed FIRST-PRINCIPLES from the SOA Standard Ultimate Life Table
survival (Makeham's Law, A=0.00022 B=2.7e-6 c=1.124), at three interest rates, for
single-life AND two-life (joint / last-survivor / only-1 / only-2) quantities. The
i=5% single-life subset is cross-checked against the `actuarialmath` SULT to anchor
this generator to an established third party; the rest extends the same first-
principles computation the Go engine is then differentially checked against.

Run:  pip install actuarialmath ipython
      python3 scripts/gen_actuarial_reference.py
"""
import json, math

A, B, c = 0.00022, 2.7e-6, 1.124
lnc = math.log(c)
OMEGA = 130


def Sf(t):  # Makeham survival from age 0
    return math.exp(-(A * t + (B / lnc) * (c ** t - 1)))


lx = [100000.0 * Sf(a) for a in range(0, OMEGA + 1)]


def surv(a):  # S(age) normalised, engine convention
    if a <= 0:
        return 1.0
    if a >= OMEGA:
        return 0.0
    return lx[a] / lx[0]


def kpx(x, k):  # k-year survival from age x (integer ages -> exact lx ratio)
    sx = surv(x)
    return 0.0 if sx <= 0 else surv(x + k) / sx


# ---- single life ----------------------------------------------------------
def wl_annuity(x, v):
    return sum(v ** k * kpx(x, k) for k in range(0, OMEGA - x + 1))


def temp_annuity(x, n, v):
    return sum(v ** k * kpx(x, k) for k in range(0, n))


def deferred_annuity(x, n, v):
    return sum(v ** k * kpx(x, k) for k in range(n, OMEGA - x + 1))


def wl_insurance(x, v):
    return sum(v ** (k + 1) * (kpx(x, k) - kpx(x, k + 1)) for k in range(0, OMEGA - x))


def term_insurance(x, n, v):
    return sum(v ** (k + 1) * (kpx(x, k) - kpx(x, k + 1)) for k in range(0, n))


def pure_endow(x, n, v):
    return v ** n * kpx(x, n)


def endow_insurance(x, n, v):
    return term_insurance(x, n, v) + pure_endow(x, n, v)


def wl_annuity_var(x, v):
    d = 1 - v
    a1 = wl_insurance(x, v)
    a2 = sum((v * v) ** (k + 1) * (kpx(x, k) - kpx(x, k + 1)) for k in range(0, OMEGA - x))
    return (a2 - a1 * a1) / (d * d)


# ---- two life (independent lives) -----------------------------------------
def joint_kp(x, y, k):
    return kpx(x, k) * kpx(y, k)


def last_kp(x, y, k):
    return 1 - (1 - kpx(x, k)) * (1 - kpx(y, k))


def two_annuity(x, y, v, kpfn):
    K = max(OMEGA - x, OMEGA - y)
    return sum(v ** k * kpfn(x, y, k) for k in range(0, K + 1))


def two_insurance(x, y, v, kpfn):
    K = max(OMEGA - x, OMEGA - y)
    return sum(v ** (k + 1) * (kpfn(x, y, k) - kpfn(x, y, k + 1)) for k in range(0, K))


def only1_annuity(x, y, v):  # x alive, y dead
    K = max(OMEGA - x, OMEGA - y)
    return sum(v ** k * kpx(x, k) * (1 - kpx(y, k)) for k in range(0, K + 1))


def only2_annuity(x, y, v):  # x dead, y alive
    K = max(OMEGA - x, OMEGA - y)
    return sum(v ** k * (1 - kpx(x, k)) * kpx(y, k) for k in range(0, K + 1))


# ---- POD (DOS convention: mid-year death, continuous discount) -------------
def pod_dos(x, i):
    delta = math.log(1 + i)
    return sum((kpx(x, k) - kpx(x, k + 1)) * math.exp(-delta * (k + 0.5))
               for k in range(0, OMEGA - x) if kpx(x, k) - kpx(x, k + 1) > 0)


RATES = [0.03, 0.05, 0.07]
SINGLE_AGES = [20, 30, 40, 50, 60, 70, 80]
NS = [5, 10, 20]
PAIRS = [[20, 25], [40, 45], [60, 65], [50, 70], [30, 65], [70, 75]]


def build_block():
    """Compute single + two-life + lifeprob-grid for the CURRENT global lx."""
    single, twolife = {}, {}
    for i in RATES:
        v = 1 / (1 + i)
        rk = f"{i:.2f}"
        single[rk] = {
            "wl_annuity": {str(x): round(wl_annuity(x, v), 8) for x in SINGLE_AGES},
            "wl_insurance": {str(x): round(wl_insurance(x, v), 8) for x in SINGLE_AGES},
            "wl_annuity_var": {str(x): round(wl_annuity_var(x, v), 6) for x in SINGLE_AGES},
            "temp_annuity": {f"{x},{n}": round(temp_annuity(x, n, v), 8) for x in SINGLE_AGES for n in NS},
            "deferred_annuity": {f"{x},{n}": round(deferred_annuity(x, n, v), 8) for x in SINGLE_AGES for n in NS},
            "term_insurance": {f"{x},{n}": round(term_insurance(x, n, v), 8) for x in SINGLE_AGES for n in NS},
            "pure_endow": {f"{x},{n}": round(pure_endow(x, n, v), 8) for x in SINGLE_AGES for n in NS},
            "endow_insurance": {f"{x},{n}": round(endow_insurance(x, n, v), 8) for x in SINGLE_AGES for n in NS},
            "pod_dos": {str(x): round(pod_dos(x, i), 8) for x in SINGLE_AGES},
        }
        twolife[rk] = {}
        for x, y in PAIRS:
            twolife[rk][f"{x},{y}"] = {
                "joint_annuity": round(two_annuity(x, y, v, joint_kp), 8),
                "last_annuity": round(two_annuity(x, y, v, last_kp), 8),
                "joint_insurance": round(two_insurance(x, y, v, joint_kp), 8),
                "last_insurance": round(two_insurance(x, y, v, last_kp), 8),
                "only1_annuity": round(only1_annuity(x, y, v), 8),
                "only2_annuity": round(only2_annuity(x, y, v), 8),
            }
    grid = {}
    for x, y in PAIRS:
        for k in [0, 1, 5, 10, 20, 30, 40]:
            grid[f"{x},{y},{k}"] = {
                "living1": round(kpx(x, k), 10), "dead1": round(1 - kpx(x, k), 10),
                "only1": round(kpx(x, k) * (1 - kpx(y, k)), 10),
                "only2": round((1 - kpx(x, k)) * kpx(y, k), 10),
                "either": round(1 - (1 - kpx(x, k)) * (1 - kpx(y, k)), 10),
                "both": round(kpx(x, k) * kpx(y, k), 10),
            }
    return single, twolife, grid

ref = {
    "params": {"A": A, "B": B, "c": c, "i": 0.05, "omega": OMEGA, "rates": RATES,
               "single_ages": SINGLE_AGES, "ns": NS, "pairs": PAIRS},
    "lx": [round(x, 8) for x in lx],
}

# legacy keys (consumed by the existing TestSULTvsActuarialMath, i=5%)
ref["kpx"] = {f"{x},{k}": round(kpx(x, k), 10)
              for x in range(20, 91, 5) for k in [1, 2, 5, 10, 15, 20, 30, 40] if x + k <= 118}
ref["ex"] = {str(x): round(sum(kpx(x, k) for k in range(1, OMEGA - x + 1)), 8) for x in range(20, 91, 5)}
ref["annuity_due"] = {str(x): round(wl_annuity(x, 1 / 1.05), 8) for x in range(20, 91, 5)}
ref["insurance"] = {str(x): round(wl_insurance(x, 1 / 1.05), 8) for x in range(20, 91, 5)}
ref["pod_dos"] = {str(x): round(pod_dos(x, 0.05), 8) for x in range(20, 91, 5)}

# expanded per-rate single + two life, for the SULT (Makeham) table
single, twolife, grid = build_block()
ref["single"] = single
ref["twolife"] = twolife
ref["lifeprob_grid"] = grid

# Second, INDEPENDENT table: a Gompertz law (mu = Bg*cg^t, A=0). Validating the
# engine on a different mortality law shows its machinery is table-independent,
# not accidentally tuned to the SULT's Makeham law. Computed by the SAME trusted
# first-principles code (with lx swapped); anchored to actuarialmath's Gompertz
# at i=5% below.
Bg, cg = 0.0003, 1.07
lncg = math.log(cg)
gompertz_lx = [100000.0 * math.exp(-(Bg / lncg) * (cg ** a - 1)) for a in range(0, OMEGA + 1)]
lx = gompertz_lx  # swap the global the quantity functions read
g_single, g_twolife, g_grid = build_block()
ref["gompertz"] = {
    "params": {"B": Bg, "c": cg},
    "lx": [round(x, 8) for x in gompertz_lx],
    "single": g_single, "twolife": g_twolife, "lifeprob_grid": g_grid,
}
lx = [100000.0 * Sf(a) for a in range(0, OMEGA + 1)]  # restore SULT global

# ---- anchor: cross-check the i=5% single-life subset vs actuarialmath ------
try:
    from actuarialmath import SULT
    s = SULT()
    worst = 0.0
    for x in SINGLE_AGES:
        worst = max(worst, abs(single["0.05"]["wl_annuity"][str(x)] - s.whole_life_annuity(x)))
        worst = max(worst, abs(single["0.05"]["wl_insurance"][str(x)] - s.A_x(x)))
        for n in NS:
            worst = max(worst, abs(single["0.05"]["temp_annuity"][f"{x},{n}"] - s.temporary_annuity(x, t=n)))
            worst = max(worst, abs(single["0.05"]["term_insurance"][f"{x},{n}"] - s.term_insurance(x, t=n)))
            worst = max(worst, abs(single["0.05"]["pure_endow"][f"{x},{n}"] - s.E_x(x, t=n)))
            worst = max(worst, abs(single["0.05"]["endow_insurance"][f"{x},{n}"] - s.endowment_insurance(x, t=n)))
            worst = max(worst, abs(single["0.05"]["deferred_annuity"][f"{x},{n}"] - s.deferred_annuity(x, u=n)))
    ref["params"]["actuarialmath_anchor_maxerr"] = worst
    print(f"actuarialmath anchor (i=5% SULT single-life): max |err| = {worst:.3e}")
    assert worst < 1e-6, "generator disagrees with actuarialmath beyond 1e-6"

    # Two-life anchor: cross-check the i=5% SULT joint-life and last-survivor
    # annuities against actuarialmath's own SULT survival composed under the
    # independence assumption (a_xy = Σ vᵏ·ₖpₓ·ₖp_y ; last = inclusion-exclusion).
    # actuarialmath has no built-in multiple-life module, so the independent
    # third party here is its single-life p_x, combined by the textbook formula
    # — the same composition the Go engine's LifeProb performs.
    sv = 1 / 1.05
    s_kpx = lambda a, k: s.p_x(int(a), t=int(k))
    tl_worst = 0.0
    for x, y in PAIRS:
        am_joint = sum(sv ** k * s_kpx(x, k) * s_kpx(y, k) for k in range(0, OMEGA + 1))
        am_last = sum(sv ** k * (1 - (1 - s_kpx(x, k)) * (1 - s_kpx(y, k))) for k in range(0, OMEGA + 1))
        tl_worst = max(tl_worst, abs(twolife["0.05"][f"{x},{y}"]["joint_annuity"] - am_joint))
        tl_worst = max(tl_worst, abs(twolife["0.05"][f"{x},{y}"]["last_annuity"] - am_last))
    ref["params"]["actuarialmath_twolife_anchor_maxerr"] = tl_worst
    print(f"actuarialmath anchor (i=5% SULT two-life joint/last): max |err| = {tl_worst:.3e}")
    assert tl_worst < 1e-6, "two-life generator disagrees with actuarialmath beyond 1e-6"

    # anchor the Gompertz block against actuarialmath's Gompertz law
    from actuarialmath import Gompertz
    g = Gompertz(B=Bg, c=cg).set_interest(i=0.05)
    gworst = 0.0
    for x in SINGLE_AGES:
        gworst = max(gworst, abs(g_single["0.05"]["wl_annuity"][str(x)] - g.whole_life_annuity(x)))
        gworst = max(gworst, abs(g_single["0.05"]["wl_insurance"][str(x)] - g.A_x(x)))
        for n in NS:
            gworst = max(gworst, abs(g_single["0.05"]["temp_annuity"][f"{x},{n}"] - g.temporary_annuity(x, t=n)))
            gworst = max(gworst, abs(g_single["0.05"]["term_insurance"][f"{x},{n}"] - g.term_insurance(x, t=n)))
    ref["gompertz"]["params"]["actuarialmath_anchor_maxerr"] = gworst
    print(f"actuarialmath anchor (i=5% Gompertz single-life): max |err| = {gworst:.3e}")
    assert gworst < 1e-6, "Gompertz generator disagrees with actuarialmath beyond 1e-6"
except ImportError:
    print("WARNING: actuarialmath not installed; skipping the i=5% anchor cross-check")

json.dump(ref, open("internal/finance/actuarial/testdata/sult_reference.json", "w"), indent=1)
print("wrote internal/finance/actuarial/testdata/sult_reference.json")
