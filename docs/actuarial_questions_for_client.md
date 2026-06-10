> **⚠️ SUPERSEDED — do not send this version.** This list was framed as if we
> would reconstruct the actuarial math from conventions. That was the wrong
> approach: the goal is to *translate the existing tested code*. After the
> client confirmed the table names are in the source (and we verified it —
> `PEDATA.pas:66`, `actuarialfilename:('MALE','FEMALE')`), the correct, much
> shorter ask is in **`docs/actuarial_files_to_request.md`**: send the two
> missing table files and the source that defines `LifeProb`/`PODValue` + the
> table reader. Kept below only for history.

---

# Questions for the Client — Actuarial / Life-Contingency Logic

*Purpose: we have fully mapped the actuarial **dispatch** logic you described
(the per-screen "hit Enter, search the screen, decide what to compute" if-then
code) — it is in the repository and we've cited every location. What we do **not**
have is the small set of numeric routines those branches call, plus the
mortality table they read. These questions target exactly that gap, so we can
reproduce the original numbers rather than guess.*

*Context so the questions make sense: the screen-dispatch code calls three
things we can't find a definition for anywhere — `LifeProb(date, contingency)`
(a survival probability), `PODValue(asof, rate)` (present value of a payment-on-
death), and `XPODValue` (the variable-rate version). No apology needed on the
spaghetti — we've traced it fine. We just need the math and the numbers behind
those three calls.*

---

## A. The mortality table (highest priority — this is the main blocker)

1. **Which table?** What mortality/life table did the actuarial feature use —
   e.g. a named published table (1980 CSO, 1958 CSO, a Social Security period
   table, an annuity table like the 1983 GAM), or your own data?
2. **Was it data or a formula?** Was the table stored as an array of numbers in
   the code (a list of values per age), read from a data file at runtime, or
   *computed* from a formula (e.g. Gompertz/Makeham, "force of mortality =
   A + B·c^age")? If a formula, what were the constants?
3. **If you can find the numbers at all** — even a printout, spreadsheet, or the
   table's published name and year — that alone would let us load the correct
   data. The published tables are reproducible if we know which one.
4. **Sex/blend:** Did it use separate male/female tables, a unisex/blended
   table, or ignore sex?
5. **Radix and column:** Did it work from `lx` (survivors out of some starting
   number like 100,000) or from `qx` (probability of dying within the year)?

## B. How a single survival probability was computed (`LifeProb`)

6. **Age basis:** Did it use age **last birthday**, age **nearest birthday**, or
   exact fractional age when looking a person up in the table?
7. **Between integer ages:** For a date that falls partway through a year of
   age, how was the probability interpolated — straight-line between table
   values (uniform distribution of deaths), a constant-force assumption, or no
   interpolation (round to a whole age)?
8. **Reference date:** Survival is measured *from when to when*? Our reading is
   "from the valuation date (the program's 'today') to the cash-flow date." Is
   that right, and is the valuation date the system clock date, or a date the
   user enters?
9. **Past-dated flows:** For a payment dated before the valuation date, what did
   `LifeProb` return — 1.0 (certain, already survived), or something else? (We
   see a special case in the code for the `DEAD`/`ONLY_1`/`ONLY_2` types before
   "now"; we want to confirm the intent.)

## C. The two-life contingency types (`N L D 1 2 E B`)

10. Please confirm the meaning of each contingency code, which we read as:
    **N** = not contingent (certain), **L** = first life living, **D** = first
    life dead, **1** = only life 1 living (life 2 dead), **2** = only life 2
    living, **E** = either living, **B** = both living. Are those correct?
11. **Joint mechanics:** For "either"/"both"/"only-one" cases, were the two
    lives treated as **independent** (so e.g. P(both alive) = p1 × p2), or was
    there a joint-life adjustment?
12. **Inputs per life:** The code carries two dates of birth (`dob[1]`,
    `dob[2]`). Was a second date of birth required for the single-life types
    (L/D), or only for the two-life types (1/2/E/B)?

## D. The payment-on-death value (`PODValue` / `XPODValue`)

13. **What it represents:** Our reading is "the present value, discounted to the
    valuation date, of a fixed death benefit (`pod`) paid at the moment the
    relevant life dies." Correct?
14. **Death timing:** Was the death benefit assumed paid at the **end of the
    year of death**, **end of the month of death**, **immediately on death**
    (continuous), or some other convention?
15. **Integration:** Was the POD value computed as a sum over discrete periods
    (monthly? yearly?) of (benefit × discount × probability-of-dying-in-that-
    period), or via a closed-form/continuous integral?
16. **Whose death:** For a two-life contingency, which life's death triggers the
    benefit, and how was that combined across the two lives?

## E. Discounting & conventions (consistency check)

17. Did the actuarial calculations use the **same** continuous discounting as
    the rest of Present Value (we use `exp(-rate × years)` with a 30/360 day
    count), or did the actuarial path use a different rate or day-count
    convention?
18. Was the COLA (cost-of-living escalation) on a payment applied **before** the
    survival weighting, after, or not combined with actuarial at all?

## F. A few known answers would settle everything (validation)

19. **Worked examples:** The manual references "Example 23, page 152" for a
    *life* annuity. Do you have that example's inputs **and** its printed result
    — or any other actuarial calculation where you know the original program's
    output number? Even two or three input→output pairs would let us verify our
    reconstruction to the cent, the same way we've validated every other engine
    against known outputs.
20. **Any older copy:** Is there an older backup, floppy, or build of the source
    from *before* the actuarial feature was disabled — anything where the
    `LifeProb`/`PODValue` bodies might still be intact? (In this copy they're
    referenced but the definitions aren't present.)

---

### What we'll do with the answers

- **Section A (the table)** is the one true blocker. With the table — even just
  its published name — plus a single worked example from Section F, we can
  reproduce the original actuarial numbers and validate our port to the same
  bit-level confidence we've achieved for amortization, present value, mortgage,
  and variable-rate.
- **Sections B–E** let us match the original's *conventions* exactly (age basis,
  interpolation, death timing). We have reasonable defaults in place; your
  answers turn "reasonable" into "faithful to the original."
- If the only thing you can recover is **Section F** (a few known outputs), that
  alone is enough to validate — we can infer the table and conventions by
  fitting to known results.

If it's easier to answer by voice or to just send whatever fragments you can
find (notes, a spreadsheet, a screenshot of an old run), that works too — we can
take it from there.
