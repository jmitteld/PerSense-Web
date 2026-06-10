# Actuarial / Life-Contingency Source — Investigation Report

*Investigation date: 2026-06-09. Scope: locate the DOS source for the
life-contingency "Actuarial window" feature (annuities "for life", payment-on-
death), which the client reports did ship but is "broken up across the modules."*

> **Correction (2026-06-09, after client feedback).** The client confirmed the
> source contains the default table names, and **he is right** — `PEDATA.pas:66`
> sets `actuarialfilename:('MALE','FEMALE')` (the 1988 HHS male/female tables),
> backed by the `actuarialfilename: array[1..2] of str8` field at
> `PETYPES.pas:434`, and `PETYPES.dcu` binary-matches the field, confirming it
> was a real compiled feature. This does **not** change the core finding but it
> sharpens it: the issue is not "the code was never written" — it is that the
> **specific files are missing from the source snapshot we received.** The goal
> is to *translate* the existing, tested code, not to rewrite it from logic; the
> only reason a reconstruction exists today is that those files weren't in the
> snapshot. See `docs/actuarial_files_to_request.md` for the short, concrete
> list of files to obtain.

---

## Executive summary

The life-contingency **Actuarial window** was a real, shipped feature — the
user manual documents it ("to calculate an annuity that continues *for life*,
use the Actuarial window — see Example 23, page 152"). The client is correct
that actuarial logic is spread across several modules rather than living in one
clean unit.

The investigation found a precise split:

- **The actuarial *integration* layer IS present and recoverable** — scattered
  across `PRESVALU.pas`, `PVLXSCRN.pas`, and `pvltable.pas` as **36
  `{$ifdef ACTU}` blocks**, plus the complete type system and runtime data
  structures in `PETYPES.PAS` / `PEDATA.pas`. This is the "broken up across the
  modules" code the client is describing, and it is enough to recover the exact
  *interface contract* the feature uses.

- **The actuarial *computational core* is NOT present in this repository** — the
  three functions that the scattered code calls (`LifeProb`, `PODValue`,
  `XPODValue`) and the underlying mortality/life table have **no definition
  anywhere**: not in any `.pas`, not in any compiled `.dcu`, and not even as
  strings inside the shipped `Persense.exe`. They live in a unit named
  `ACTUARY` that is referenced but **commented out** of every `uses` clause.

So "the DOS version has all the logic we need" is *partly* true: the repo has
everything about *how* the actuarial feature plugs into present value (the hard-
to-guess integration semantics), but not the *what* (the survival-probability
formula and the mortality table). Recovering full DOS fidelity requires
obtaining the `ACTUARY` unit (or its compiled `.dcu`, or the manual's worked
Example 23) from the client. Everything else needed to wire it in is already
reconstructed and validated.

---

## What was searched (and the evidence)

| Search | Result |
|---|---|
| `function LifeProb` / `PODValue` / `XPODValue` definitions, all `.pas` (dos + win) | **None found** — every occurrence is a *call*, never a definition |
| Files named `ACTUARY.*`, `actu.*`, `*life*`, `*mortality*` anywhere under `legacy/` | **None exist** |
| `ACTUARY` in `uses` clauses | Present but **commented out**: `//{$ifdef ACTU} ,ACTUARY {$endif}` in `PRESVALU.pas:12` and `pvltable.pas:6` (both dos and win) |
| `strings Persense.exe` and every `.dcu` for `LifeProb` / `PODValue` / `ACTUARY` / `Mortality` | **No hits** — the actuarial core was never linked into the shipped binary |
| Build config `win_source/Persense.cfg` | Defines `V_3;SCROLLS;PVLX` — **not `ACTU`** |
| `{$define ACTU}` anywhere | **Never defined** — so all 36 `{$ifdef ACTU}` blocks are dead in this build |
| Mortality table data (`lx`/`qx` arrays) | **None** — `INTSUTIL`'s `lx`/`interpol` hits are generic `Linear`/`QuadraticInterpolation`, unrelated to mortality |
| Help docs (`win_source/Help/`) for an Actuarial-window page | **None** — no `AC_*`/actuarial help page shipped; "Actuarial" in the settings help refers to the *negative-amortization* "Actuarial Rule", a different feature |

The two independent confirmations — no source definition *and* no symbol in the
compiled `.exe`/`.dcu` — are conclusive: the `ACTUARY` computational unit is not
anywhere in the materials currently in this repository.

---

## What IS present: the scattered integration layer (recoverable)

This is the code the client means by "broken up across the modules." It defines
exactly how the actuarial core is consumed, which is the part that would be
hardest to guess correctly. Map of every location:

### Type system & data — `PETYPES.PAS`, `PEDATA.pas`

- **Contingency type codes** (`PETYPES.PAS:169-171`):
  `NOT_CONTINGENT=0, LIVING=1, DEAD=2, ONLY_1_LIVING=3, ONLY_2_LIVING=4,
  EITHER_LIVING=5, BOTH_LIVING=6` (with aliases `ONLY_1/ONLY_2/EITHER/BOTH`).
- **Per-row contingency field**: `act0` on lump-sum rows, `actn` on periodic
  rows (`PETYPES.PAS:578,596,650,664`).
- **Letter mapping** for the screen: `actchar = ('N','L','D','1','2','E','B')`
  (`PEDATA.pas:144`).
- **Runtime actuarial state** (`PEDATA.pas:288-290`): `actu_now` (valuation
  date), `termdate`, `dob[1..2]` (dates of birth for up to two lives),
  `pod` (death-benefit amount entered by the user), `podval` (its present
  value), `actset` (the set of valid contingency letters).
- **Initialization**: `InitActuarial` (`PEDATA.pas:1097`) — zeroes `pod`,
  `podval`, the two `dob` entries and `termdate`, sets `actu_now := now`, and
  (under ACTU) builds `actset` from `actchar`. Notably this runs *even when
  ACTU is off* ("need to do this even if not ACTU", `peDataInit`).

### Present-value integration — `PRESVALU.pas` (25 `{$ifdef ACTU}` blocks)

Block line numbers: 12, 25, 44, 211, 219, 233, 290, 295, 389, 395, 552, 675,
688, 711, 789, 840, 847, 873, 893, 1087, 1155, 1175, 1186, 1196, 1266. The
substantive ones:

- **Lump-sum value weighting** (`:211`, `:219`): `val0 := val0 * LifeProb(date,
  act0)` on the forward path; on the inverse path `amt0 := amt0 / LifeProb(...)`
  (with a "beyond life span" guard when the probability underflows).
- **Periodic summation weighting** (`:295`, `:395`): each period's discounted
  payment is multiplied by `LifeProb(t, actn)`; there is a special case
  (`:399`) that for `actn ∈ {DEAD, ONLY_1, ONLY_2}` before `actu_now` keeps the
  running term from truncating early (annotated "JJM 2/28/93").
- **Grand-total POD term** (`:688`): `sumvalue := sumvalue + PODValue(asof,
  r.rate)`.
- **Backward calc POD subtraction** (`:840-849`): before solving, the target
  value has the POD value removed first — `val := val - XPODValue` (variable-
  rate) or `val - PODValue(asof, rate)` (fixed).
- **Actuarial-window entry** (`PrepareForLife`, `:1087`) and the
  `ActuarialCalc` dispatch in `Enter` (`:1196`, `:1266`) — the UI flow that
  pushes the life-contingency overlay and runs the calc.

### Variable-rate (fancy) integration — `PVLXSCRN.pas` (4 blocks: 247, 254, 324, 499)

- Same `LifeProb` weighting on fancy lump values (`:247-255`) and fancy periodic
  summation (`:324`).
- Grand total adds `podval` (`:426`, just outside an ACTU guard) and the fancy
  POD value comes from `XPODValue`.

### Table output — `pvltable.pas` (7 blocks: 6, 393, 476, 515, 531, 560, 628)

- Per-payment detail multiplies the discounted value by `LifeProb(t,
  contingency)` and tags the row with the contingency letter (`:515-519`).
- `PrintPOD` (`:560`) emits the on-death line: `podval := XPODValue` (fancy) or
  `PODValue(c[1]^.asof, c[1]^.r.rate)`, and folds it into the table total.

---

## The interface contract the missing core must satisfy

Even without the `ACTUARY` source, the call sites pin down the exact signatures
and semantics the core must implement:

```
function LifeProb(date: daterec; contingency: byte): real;
    { Probability that the contingency condition holds at `date`, given the
      lives' dates of birth (dob[1], dob[2]) and the valuation date actu_now.
      contingency ∈ {NOT_CONTINGENT, LIVING, DEAD, ONLY_1, ONLY_2, EITHER, BOTH}.
      NOT_CONTINGENT -> 1.0. Used as a multiplicative weight on each cash flow. }

function PODValue(asof: daterec; rate: real): real;
    { Present value at `asof`, discounted at `rate`, of the death benefit `pod`
      paid at the (uncertain) moment of death — i.e. ∫ pod·v(t)·f_death(t) dt
      over the relevant life(s). }

function XPODValue: real;
    { As PODValue but discounted through the variable-rate schedule (cc[]) to
      d^.xasof, for the PVLfancy path. }
```

Inputs available to these: `dob[1..2]`, `actu_now`, `termdate`, `pod`, the
discount `rate`/schedule, and the contingency code. This is precisely the
surface the Go port reconstructed (see below), so wiring a recovered `ACTUARY`
core back in would be mechanical.

---

## What the Go port already has (and what it can't verify)

The Go reconstruction in `internal/finance/actuarial/` implements the full
machinery to this contract:

- `contingency.go`: `LifeProb(date, contingency)` with all seven contingency
  types, `survivalProb1/2`, `RequiresSecondLife`, `ActuarialConfig` (carries the
  two DOBs + valuation date), `PODValue` and `PODValueFunc` (variable-rate POD).
- `table.go`: a `LifeTable` (age-indexed `qx`/`lx`, radix 100,000) with
  **linear interpolation** between integer ages (uniform-distribution-of-deaths
  assumption), conditional survival `lx(futureAge)/lx(currentAge)`, life
  expectancy, and CSV/JSON table loaders.

This matches the recoverable DOS contract well: the contingency-type set, the
multiplicative-weight usage, the POD-value subtraction-before-backward-solve,
and the two-life structure all line up with the scattered call sites.

**What cannot be verified against the original from this repository:**

1. **The mortality table values.** The Go table is data-driven (loaded from
   CSV/JSON); the DOS table values are unknown. Different tables (e.g. 1980 CSO
   vs 2001 CSO vs an insurer's own) give materially different probabilities.
2. **The exact `LifeProb` interpolation/age convention.** Go assumes uniform
   distribution of deaths with linear `lx` interpolation; DOS may have used a
   different intra-year rule or an age-nearest-birthday vs age-last-birthday
   convention.
3. **The exact `PODValue` integration.** Go uses a discrete monthly death-
   benefit summation; the DOS `PODValue` quadrature is unknown.

These three unknowns are why actuarial confidence is capped (~86) on a
*"faithful to the original"* basis, even though the engine is correct against
standard actuarial mathematics.

---

## Why the earlier "source missing" conclusion still stands — with a refinement

The earlier roadmap statement ("the actuarial source was never shipped") needs a
one-word refinement in light of the client's information: the *feature* shipped,
but the *computational unit* (`ACTUARY`) is **not in the materials in this
repository**. The integration code that surrounds it did ship and is here. The
distinction matters because it tells us exactly what to ask the client for — a
single unit, not a rewrite.

---

## Recommendations — what to ask the client for, in priority order

1. **The `ACTUARY` unit itself** — `ACTUARY.PAS` (DOS) or `ACTUARY.dcu`
   (Windows). It is named explicitly in the commented `uses` clauses. A `.dcu`
   alone would let us (a) extract the embedded mortality table and (b) compile a
   headless oracle for it exactly as we did for amortization / PV / mortgage /
   variable-rate, lifting actuarial to the same bit-identical footing as every
   other engine.
2. **The mortality table**, if the unit can't be found — even just the table's
   *name/year* (e.g. "1980 CSO, age-nearest-birthday") would let us load the
   correct data and re-validate; the published table values are reproducible.
3. **The manual's Example 23 (page 152)** and any other worked actuarial
   examples with input/output numbers. Even a handful of known DOS outputs would
   serve as a validation oracle for the Go reconstruction (the same role
   `legacy/reference-output/refdata.json` plays for the other engines).
4. **Any build with `ACTU` defined**, or a screenshot/printout of an actuarial
   table from the original program — a secondary cross-check source.

With item 1, actuarial joins the other five engines at bit-identical
confidence. With item 2 or 3 alone, it can reach high-90s correctness against
known outputs. Without any of them, ~86 (correct-by-construction against
canonical actuarial math, table-source unverified) is the ceiling.

---

## Appendix — exact citations

- Commented-out unit reference: `legacy/src/dos_source/PRESVALU.pas:12`,
  `legacy/src/dos_source/pvltable.pas:6` (identical in `win_source`).
- Build config without ACTU: `legacy/src/win_source/Persense.cfg` (`-DV_3;SCROLLS;PVLX`).
- Contingency types: `legacy/src/dos_source/PETYPES.PAS:169-171`.
- Runtime actuarial state: `legacy/src/dos_source/PEDATA.pas:288-290`,
  `InitActuarial` at `:1097-1110`.
- `{$ifdef ACTU}` integration blocks — `PRESVALU.pas` (25): lines 12, 25, 44,
  211, 219, 233, 290, 295, 389, 395, 552, 675, 688, 711, 789, 840, 847, 873,
  893, 1087, 1155, 1175, 1186, 1196, 1266; `PVLXSCRN.pas` (4): 247, 254, 324,
  499; `pvltable.pas` (7): 6, 393, 476, 515, 531, 560, 628.
- Manual reference to the Actuarial window: `win_source/Help/PV_EX1.html`,
  `PV_EX3.html` ("use the Actuarial window. (See Example 23, page 152.)").
- Go reconstruction: `internal/finance/actuarial/contingency.go`,
  `internal/finance/actuarial/table.go`.
