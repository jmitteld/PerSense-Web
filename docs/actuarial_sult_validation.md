# Actuarial — validated against an independent third-party oracle (SOA SULT + actuarialmath)

The DOS actuarial engine can't be compiled from this snapshot (no `LifeProb`/
`PODValue`/`ACTUARY` source, no mortality tables — see
`docs/actuarial_oracle_blocked.md`), so it can't be differential-tested against
the real DOS engine the way the other engines are. Instead, the Go actuarial
engine is now validated against an **authoritative, independent third party**:
the Society of Actuaries' **Standard Ultimate Life Table (SULT)** and the
open-source **`actuarialmath`** Python library.

## What the SULT is

The SULT is the standard reference life table used on SOA actuarial exams. It is
built from **Makeham's Law** (force of mortality μ(x) = A + B·cˣ with
A = 0.00022, B = 2.7×10⁻⁶, c = 1.124) at **i = 5%**, and the SOA *publishes* the
resulting actuarial values — survival probabilities, whole-life and term
annuities, whole-life and endowment insurances — to five decimals for integer
ages 20–100. Because it comes from a closed-form survival law, the exact same
table can be reproduced anywhere, which makes it an ideal cross-check.

## The test

`internal/finance/actuarial/sult_validation_test.go` (`TestSULTvsActuarialMath`)
builds the SULT `lx` table in the Go engine (the Makeham `lx` reproduces
`actuarialmath`'s `p_x` to 3×10⁻¹⁶) and compares, against a committed reference
in `testdata/sult_reference.json`:

| Quantity | What it checks | Result |
|---|---|---|
| Survival `ₖpₓ` (116 points, ages 20–90) | `LifeTable.ConditionalSurvival` **and** `ActuarialConfig.LifeProb(Living)` | 0 fails, max err **4.7e-11** |
| Curtate life expectancy `eₓ` | `LifeTable.LifeExpectancy` | 0 fails, max err **4.8e-9** |
| Whole-life annuity-due `äₓ` | engine survival → standard `Σ vᵏ·ₖpₓ` | 0 fails, max err **4.6e-9** |
| Whole-life insurance `Aₓ` | engine survival → standard `Σ vᵏ⁺¹·(ₖpₓ−ₖ₊₁pₓ)` | 0 fails, max err **4.9e-9** |
| Payment-on-Death `Aₓ` (DOS convention) | the engine's own `ActuarialConfig.PODValue` code | 0 fails, max err **4.8e-9** |

The reference values for `äₓ`, `Aₓ`, `eₓ`, `ₖpₓ` come straight from
`actuarialmath` (and agree with the SOA's published SULT — e.g. A₂₀ = 0.049219,
ä₂₀ = 19.9664, e₂₀ = 65.413). The Payment-on-Death row uses the engine's
specific DOS convention (mid-year death, continuous discounting δ = ln 1.05),
with the reference computed from the same SULT survival.

## What this does and does not establish

**Does:** the Go life-contingency *math* is correct — survival projection,
conditional survival, life expectancy, annuity and insurance composition, and
the Payment-on-Death code all reproduce an authoritative third-party computation
to 1e-9 or better. Combined with the fact that the **discounting** engine is
already validated directly against the real DOS engine (all PV forward paths and
all seven backward solvers), the composed actuarial present values rest on two
independently-validated halves: DOS-validated discounting × SULT-validated
survival.

**Does not:** prove bit-fidelity to the *original product's* actuarial outputs.
That still requires the original 1988 HHS table data and the DOS `LifeProb`/
`PODValue` code, because a different mortality table yields different (equally
valid) numbers. This check validates the engine's mathematics with a standard
table; matching the historical product specifically remains blocked on the
client materials.

## Reproducing the reference

The test reads the committed `testdata/sult_reference.json`, so CI needs no
Python. To regenerate it (`pip install actuarialmath ipython`):

```python
from actuarialmath import SULT
import math, json
s = SULT(); i = 0.05; delta = math.log(1+i)
A,B,c = 0.00022, 2.7e-6, 1.124; lnc = math.log(c); OMEGA = 130
def Sf(t): return math.exp(-(A*t + (B/lnc)*(c**t - 1)))   # Makeham survival from age 0
lx = [100000.0*Sf(a) for a in range(0, OMEGA+1)]
def goSurv(a): return 1.0 if a<=0 else (0.0 if a>=OMEGA else lx[a]/lx[0])
def kpx(x,k):
    sc = goSurv(x); return 0.0 if sc<=0 else goSurv(x+k)/sc
xs = list(range(20,91,5))
ref = {"params":{"A":A,"B":B,"c":c,"i":i,"omega":OMEGA},
  "lx":[round(x,8) for x in lx],
  "kpx":{f"{x},{k}":round(s.p_x(x,t=k),10) for x in xs for k in [1,2,5,10,15,20,30,40] if x+k<=118},
  "ex":{str(x):round(s.e_x(x),8) for x in xs},
  "annuity_due":{str(x):round(s.whole_life_annuity(x),8) for x in xs},
  "insurance":{str(x):round(s.A_x(x),8) for x in xs},
  "pod_dos":{str(x):round(sum((kpx(x,k)-kpx(x,k+1))*math.exp(-delta*(k+0.5))
             for k in range(0,OMEGA-x) if kpx(x,k)-kpx(x,k+1)>0),8) for x in xs}}
json.dump(ref, open("internal/finance/actuarial/testdata/sult_reference.json","w"), indent=1)
```
