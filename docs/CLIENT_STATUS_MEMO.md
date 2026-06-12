# Per%Sense Go Port — Fidelity Validation Status

**Date:** 2026-06-10
**Subject:** How thoroughly the Go port has been verified against the original DOS program, current confidence by module, and the two items that need your input.

---

## The short version

The Go port now reproduces the original DOS Per%Sense engine **exactly** —
to the last displayed digit, and usually to full double-precision — everywhere we
can run the original code as the judge. Every calculation module that has DOS
source behind it has been checked by feeding thousands of randomized inputs to
*both* the genuine DOS engine and the new Go engine and confirming they agree.

The one module we **cannot** verify against the original *specifically* is the
actuarial / life-contingency feature, because its original source code was never
included in the materials we have. Its **math is now very thoroughly verified**
against independent actuarial standards (details below), but matching the
original product's exact mortality-table numbers still needs the materials in the
"What we need from you" section.

The product is now at roughly **97% confidence** overall.

---

## How we verify (so the numbers mean something)

Earlier checks compared the Go code against a *re-typed* copy of the DOS formulas.
That catches many errors but shares one blind spot: if the original formula was
misread, both copies can be wrong in the same way and the check still passes.

To remove that blind spot we built a **source-oracle**: the *actual* DOS
calculation code, compiled and run unchanged in a headless harness. We then run
randomized inputs through both the real DOS engine and the Go engine and require
them to match. Because the judge is the product's own code, agreement is real
fidelity, not a shared assumption. These comparisons run automatically in the
build, so a future change that drifts from the original fails the build.

Over the course of this work the Go engine has been compared to the genuine DOS
engine across **many thousands of randomized cases** spanning loan amortization
(including balloons, ARMs, prepayments, moratoria, skip-months, and weekly/
biweekly schedules), mortgage calculations and APR comparisons, present-value and
variable-rate discounting, and all of their "solve for the blank field" reverse
calculations — with **zero unexplained divergences**.

---

## Confidence by module

| Module | Confidence | Basis |
|---|---:|---|
| Core interest & date math | 96 | Bit-identical to DOS on every discounting case |
| Mortgage | 97 | Payment/price solves, APR comparison, what-if table, and full field-presence dispatch all match DOS exactly |
| Present value (forward) | 97 | Lump, periodic, and cost-of-living streams bit-identical to DOS |
| Present value (reverse solves) | 98 | All seven "solve the blank field" paths match DOS exactly |
| Field-presence dispatch | 96 | The "which field do I solve?" decision matches DOS across all three screens (present value, amortization, mortgage) |
| Amortization (standard) | 96 | Schedule and payment solve match DOS across randomized loans |
| Amortization (advanced options) | 95 | Prepayments, ARMs, moratoria, targets, skip-months, biweekly match DOS row-by-row; 5 real fidelity bugs found and fixed |
| Amortization (advanced reverse solves) | 96 | Loan-amount and duration solves ported to the DOS closed forms and validated |
| Variable-rate schedules | 97 | Forward, reverse amount solve, and multi-row worksheets all match DOS exactly |
| **Actuarial / life-contingency** | **95** | **Math comprehensively validated against independent standards (two mortality tables, single- & two-life, three rates) — but original-table fidelity still needs the materials below** |

The five fidelity bugs found and fixed this cycle were genuine differences from
the original (an ARM payment-only rate case, a moratorium payment, weekly/
biweekly day-count accrual, and two reverse-solve formulas). They are now
corrected to match DOS and locked in by tests, which is also evidence the method
catches real divergences rather than rubber-stamping the code.

---

## What we need from you

### 1. The actuarial source (the one real blocker)

The life-contingency feature (life tables, payment-on-death, two-life
contingencies) was built but its computational core lived in a separate unit
named **ACTUARY** that is **not present** in the source we received, and the
feature was switched **off** in the shipped DOS build. We have therefore
*reconstructed* it from the surrounding code and validated it **comprehensively
against independent actuarial standards** — the Society of Actuaries' Standard
Ultimate Life Table and the open-source `actuarialmath` library, plus a second
independent mortality law to confirm the engine isn't tuned to one curve. The
checks span single-life and **two-life (joint and last-survivor)** annuities and
insurances, payment-on-death, and the survival math, across three interest rates,
all agreeing to nine or more decimal places, including end-to-end through the
real present-value engine. So we are highly confident the **math is correct** —
but we still cannot prove it matches *the original product specifically*, because
a different mortality table yields different (equally valid) numbers.

Any **one** of the following would let us validate actuarial to the same standard
as everything else:

- **`ACTUARY.PAS`** (or whatever the life-contingency unit was named) from a build
  with the feature enabled — the real authority; ideal.
- **An executable of Per%Sense with life-contingency turned on** — even without
  source, we can use it as the comparison oracle.
- **The mortality table** it used (name/year, e.g. the 1988 HHS table) plus the
  manual's worked examples — enough to pin the reconstruction.

Until one of these arrives, actuarial stays at "correct by independent standards,
not verified against the original."

### 2. One product decision (not a bug)

While exhaustively testing the present-value screen's field-presence logic, we
found one place where the Go port is **more permissive** than the original: it
will accept a single row's own *Value* column as the target to solve from,
whereas the original requires the target to go in the screen's *Sum Value* line
and otherwise refuses. The computed numbers are identical; only *which input
layouts are accepted* differs. We left it unchanged and are flagging it for your
call:

- **Keep it** as a convenience enhancement (and we document it as an intentional
  improvement), or
- **Tighten it** to match the original's stricter rule.

Either way no calculation result changes. We just need to know which behavior you
want to be the standard.

---

## Bottom line

Every part of the product with an original to check against now matches that
original to display precision or better, verified automatically and continuously.
The remaining work to reach uniform high-90s confidence is **not engineering** —
it is recovering the actuarial materials (item 1) and a one-line product decision
(item 2). With the actuarial source in hand, that module would join the rest at
the same bit-for-bit fidelity within a normal development cycle.

*Supporting detail: `docs/fidelity_validation_roadmap.md` and the per-area
findings documents it references.*
