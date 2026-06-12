# Actuarial — extended third-party validation (2026-06-10)

The DOS actuarial core (`ACTUARY` unit + mortality table) is absent from this
repository, so the life-contingency engine cannot be differentially tested
against the original the way the other engines are (see
`docs/actuarial_oracle_blocked.md`). The strongest available alternative is
validation against **independent actuarial standards** — and that validation is
now comprehensive across the engine's whole surface, not just the single-life
whole-life subset it started with.

## What is now validated (and how it is independent)

Reference values are produced by `scripts/gen_actuarial_reference.py`, a
**first-principles** computation from each mortality law's survival function,
**anchored to the open-source `actuarialmath` library** at i=5% (the generator
must agree with `actuarialmath` to <1e-6 or it aborts). The Go engine is then
checked against that reference. Two mortality laws are used to show the engine is
**table-independent**, not accidentally tuned to one curve:

- the SOA **Standard Ultimate Life Table** (Makeham's Law, A=0.00022, B=2.7e-6,
  c=1.124); and
- a separate **Gompertz** law (B=0.0003, c=1.07), independently anchored to
  `actuarialmath`'s `Gompertz`.

On **both** tables (`internal/finance/actuarial/sult_extended_test.go`,
`TestActuarialExtendedThirdParty`):

| Area | What's checked | Result |
|---|---|---|
| LifeProb — all 6 contingency types | Living, Dead, Only1Living, Only2Living, EitherLiving, BothLiving across age pairs and durations | 252 points/table, 0 fails, ~5e-11 |
| Single-life annuities | whole-life, temporary, deferred — at i = 3/5/7 % | 0 fails, ~5e-7 |
| Single-life insurance | whole-life, term, endowment, pure endowment — 3 rates | 0 fails |
| Whole-life annuity **variance** (2nd moment) | `(²Aₓ − Aₓ²)/d²` | 0 fails |
| Payment-on-Death | DOS mid-year/continuous convention, multiple ages × 3 rates | 0 fails |
| **Two-life** annuities & insurance | joint-life **and** last-survivor, plus only-1 / only-2 contingent annuities — built through the engine's own LifeProb, 3 rates | 0 fails, ~5e-9 |
| Exact identities | `ä_x̄ȳ = äₓ + ä_y − ä_xy` and `A_x̄ȳ = Aₓ + A_y − A_xy`, from engine values | machine precision (~2e-14) |

The two-life identities are independent of *any* combination formula — they would
catch a swapped Only-1/Only-2 or a wrong last-survivor complement even if the
reference made the same mistake.

### A concrete, third-party-verified two-life example

`TestTwoLifeJointAndSurvivorExample`
(`internal/finance/actuarial/two_life_example_test.go`) pins one hand-checkable
case: two independent SULT lives aged **65 and 60 at i=5%**. The joint-life and
last-survivor annuities-due are carried as literal constants
**independently verified with `actuarialmath`** (`SULT().p_x` composed under
independence — the library has no built-in multiple-life module):

| Quantity | Value (actuarialmath) | Engine |
|---|---|---|
| joint-life ä₆₅:₆₀ | 12.373812 | matches to <1e-5 |
| last-survivor ä₆₅:₆₀‾ | 16.080052 | matches to <1e-5 |
| single ä₆₅ / ä₆₀ | 13.549790 / 14.904074 | matches |

The generator now also asserts this two-life anchor at regeneration time
(`actuarialmath_twolife_anchor_maxerr`, max |err| ≈ 5e-9), so the committed
joint/last-survivor SULT reference can never silently drift from
`actuarialmath`.

## The production PV path is validated too

Beyond the standalone life-contingency math, the **actual present-value path**
users hit — where survival is folded into each period's discount factor by
`periodicWithActuarial` / `lumpRowPV` inside `Calculate` — is validated against
independent sums in `internal/finance/presentvalue/actuarial_canonical_test.go`:
single-life life annuity, Payment-on-Death, and now a **two-life contingent
annuity** (`TestCanonicalTwoLifeAnnuityPV`) driven end-to-end through the real
engine for joint-life, last-survivor, only-1 and only-2, each matched to an
independent two-life annuity sum to <1e-6.

## What this establishes — and the residual cap

**Established:** the engine's life-contingency mathematics is correct — survival
projection, all six contingency combinations, single- and two-life annuities and
insurances, term/endowment/deferred variants, second moments, and Payment-on-
Death — reproducing an authoritative third-party computation on two different
mortality laws and across three interest rates, both standalone and as wired into
the present-value engine. Combined with the discounting engine already being
bit-identical to the real DOS engine, the composed actuarial present values rest
on two independently-validated halves.

**Residual cap:** this validates *correctness against standards*, not
bit-fidelity to the *original product's* actuarial outputs, which still requires
the original 1988-era mortality table and the DOS `LifeProb`/`PODValue` code (a
different table yields different, equally valid numbers). The only correctness
items still resting on round-trip rather than third-party checks are the
fractional-age interpolation convention and the variable-rate POD solve (the
latter blocked by the same missing actuarial source). These keep actuarial just
below the bit-identical tier the DOS-backed engines reach.

## Reproducing

```
pip install actuarialmath ipython
python3 scripts/gen_actuarial_reference.py   # regenerates testdata/sult_reference.json
go test ./internal/finance/actuarial/ ./internal/finance/presentvalue/
```
The committed `sult_reference.json` means CI needs no Python.
