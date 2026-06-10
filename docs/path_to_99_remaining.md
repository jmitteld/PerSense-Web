# Path to 99+ — the five non-actuarial engines

*Assessment of what remains to take amortization, present value, variable-rate,
PV-backward, and mortgage from their current mid-90s to 99+. Written 2026-06-09.*

## What "99" means here, and what's already done

The common path of all five engines is already at the **bit-identical ceiling**:
~6,400 randomized cases run against the real DOS engine in CI with zero
divergences (agreement to ~9 significant figures, or to display rounding for
amortization). So 99 is not about the typical calculation — that's settled.

99 is about closing the **long tail**: the per-row detail (not just totals), the
full matrix of options/flags, the backward-solve variants, and the date/boundary
conditions that the current sweeps don't yet exercise. A schedule can match on
totals while splitting interest/principal wrong on an individual line; an engine
can be perfect on a level loan and wrong under a moratorium. 99 means we've
differentially checked those too, against the original.

Everything below is concrete engineering on infrastructure that already exists
(the three headless oracles + the CI harness) — no new research, no Windows
host, no original binary.

---

## Progress log

- **2026-06-09 — PV multi-row (#88): DONE.** Added a `multi` oracle mode
  (several lump and/or periodic lines, one rate). `TestDOSPVMultiRowSweep` (500
  random mixes of 0-3 lumps and 0-3 periodics) matches the real DOS engine with
  **0 divergences, max relErr 5.4e-9** — the multi-line classification and
  cross-row summation are validated. (The PV as-of/date *backward* solves remain
  a smaller lower-priority item; they run through the `bf` screen machinery, like
  the lump/periodic amount backward solves.)
- **2026-06-09 — n=1 guard (#89): FIXED.** Go now rejects a single-payment loan
  to match DOS (docs/n1_minimum_term_finding.md).
- **2026-06-09 — 365-basis trace → a real payment-solve BUG, now FIXED.** The
  365-basis difference traced to a broader bug: `SolvePayment` used the plain
  annuity formula and ignored the first-period proration, so the solved payment
  was wrong for **any odd first period (any basis)** and for **every 365-basis
  month**. Fixed by scaling the closed-form payment by `ffFirst/f`
  (`ffFirst = 1 + (f-1)*prorate`), a no-op for the common one-period-out case.
  Now matches DOS: 365-monthly 4523.2202, 360 short-stub 13719.9584. Validated
  directly against the DOS solved payment over 400 odd-first + 300 365-basis
  loans, 0 divergences (`TestDOSPaymentSolveOddFirstAndBasis`). Closed the gap
  that hid it (the odd-first sweep fed a shared payment and never checked the
  solve). See `docs/basis365_finding.md`. **Still queued for #89:** biweekly/
  weekly (14-/7-day date arithmetic) and 365+exact (per-month day counts).

- **2026-06-09 — per-row detail (amortization): DONE.** The `amort_oracle` now
  has a `rows` mode (sets the DOS `cum=' '` flag so the table prints every
  payment) and an optional non-hard given-payment (`pay=`). New sweep
  `TestDOSPerRowSweep`: 500 randomized loans, ~9,000 lines, **0 row-count and 0
  row-value divergences** — every period's interest and remaining balance match
  the real DOS engine to the displayed cent. Method note: to isolate the per-row
  split, both engines are fed the *same* non-hard payment (so neither rounds
  per-period), and the rate is passed at full precision (a 6-decimal rate caused
  a spurious ~0.3% drift on a 31-year/14% loan via the documented "small
  difference of large numbers" sensitivity — fixed by passing 10 decimals).
  This closes the biggest blind spot: totals could reconcile while a row's
  interest/principal split was wrong. Amortization per-row is now validated.

- **2026-06-09 — fancy flags (in-advance, R78): DONE, incl. a bug fix.** Added
  oracle flag tokens (`inadv`, `r78`, `usa`, `prepaid`). `TestDOSFancyFlagSweep`:
  **Rule-of-78 and in-advance per-row both match DOS in full** (300 loans each,
  0 divergences). Along the way the per-row oracle **found and we fixed a real
  bug**: the Go engine zeroed the interest on the final in-advance payment,
  whereas DOS charges `(p-d)·f_1/(2-f)` (AMORTIZE.pas:1533-1538). Fixed in
  `engine.go`; the full in-advance schedule (incl. final row) now matches DOS,
  the worked example matches to the cent (`TestDOSInAdvanceFinalRowFix`), and the
  existing in-advance payment-solve/totals tests still pass. See
  `docs/inadvance_final_row_finding.md`.
- **2026-06-09 — prepayment semantics study: DONE → a significant finding.**
  Traced `FindNextExtra`/`ComputeNext` (`AMORTOP.pas:490-664`). The DOS feature
  is a *payment schedule*: by default (`plus_regular` OFF) an additional periodic
  payment **replaces** the regular payment on coincident dates, and an off-cycle
  one is its own dated row. **The Go engine always ADDS prepayments** (ignores
  `PlusRegular`) and folds off-cycle ones into the next period — so the same
  inputs give very different results (worked example: DOS interest 27,646 / 60
  rows vs Go 19,775 / 57 rows). Documented in
  `docs/prepayment_semantics_finding.md` with the exact code refs and a
  recommended two-part fix (honor `PlusRegular` for prepayments; emit off-cycle
  rows).
- **2026-06-09 — prepayment replace-vs-add fix: DONE + validated.** The Go fancy
  engine now honors `PlusRegular` for prepayments (sum coincident extras, then
  add when ON / replace when OFF — the default), matching DOS. Worked example
  matches (27,646.39 / 60 rows); new per-row sweep `TestDOSPrepaymentPerRowSweep`
  (250 replace + 250 add loans) matches DOS with **0 divergences in both modes**.
  Two tests that encoded the old additive default updated to `PlusRegular=true`.
  Still open (secondary): off-cycle prepayment rows, and a validation pass on the
  prepayment backward solvers under the replace default.
- **2026-06-09 — fancy-mode per-row output (enabler 1): DONE.** The fancy
  schedule (`RepayFancyLoan`) prints detail lines in a different format
  (`date payamt int prin bal cumint` — no leading paynum) than the ordinary path
  (`paynum date int prin bal cumint`). Generalised the oracle's detail-line
  detection (`IsDetailLine`: ≥6 tokens, numeric last column, starts with a
  positive paynum *or* a date token; excludes the in-advance/prepaid
  settlement-interest "row 0" and the totals/dashes lines). Validated with
  `TestDOSBalloonPerRowSweep` (300 balloon loans): per-row interest matches DOS
  on every row, balances match on the body (the final payoff row's 1-2 cent
  completion residual is excluded, as for in-advance). The fancy per-row pipeline
  now works, unlocking per-row validation of the remaining array options.
- **Still to do in this item:** prepayment semantics study (enabler 2) and then
  the array-option per-row sweeps — prepayments, rate adjustments (ARMs),
  moratorium, target, skip-months — plus USA-rule under negative amortization and
  the fancy backward solves.
- **2026-06-09 — cross-cutting date/boundary (#89): largely DONE.** Added a
  `first=MONTHS` oracle token (override the first-payment date) and validated:
  (a) **odd first periods** — `TestDOSOddFirstPeriodSweep` (400 loans, short and
  long first stubs, prorated first-period interest) matches DOS per-row, 0
  divergences; (b) **boundary inputs** — `TestDOSBoundaryCases` (zero rate, a
  teeny rate at the engine threshold, very high rates, the minimum term n=2, and
  long 30-/50-year terms) all match. Two findings: zero rate matches (Go
  handles the no-interest case correctly), and **n=1 diverged** — DOS requires
  ≥2 payments while Go accepted a single-payment loan; **now FIXED** (Go rejects
  it with the matching message; `docs/n1_minimum_term_finding.md`). The
  encountered final-payment row-merge (DOS folds a sub-`minpmt` last payment that
  Go keeps separate) is handled in the comparisons. **Remaining for #89 — the basis variants (its own sub-task).** The
  365-day / exact-basis and biweekly (26/52) modes are more involved than the
  rest of #89 and are queued: (a) exact/365 mode uses actual per-month day
  counts (the deliberately "strange" schedules the help docs describe), so the
  oracle and Go must agree on the day-count convention per row; (b) biweekly
  (26/52) auto-switches the engine to 365-day basis via an *informational*
  MessageBox that the headless Globals stub currently records as an error, so the
  driver needs to distinguish informational messages from real errors first.
  Leap years only matter off the default 30/360 basis; the Y2K century-boundary
  date *entry* (`centurydiv`) isn't reachable through the numeric driver, which
  sets the year directly. (30/360 — the default and common basis — is fully
  validated.)

---

- **2026-06-09 — mortgage (#86): DONE.** Added oracle `apr` mode (full-term APR
  via the real `ReportAPR`/`IterateToFindAPR`, captured through the headless
  MessageBox stub) and `mcash`/`mfin` modes (cash- and financed-input down
  payments). New sweeps: **APR with points** (`TestDOSMtgAPRSweep`, 400
  mortgages, 0 divergences, max |err| 5e-7 = the 4-decimal display rounding) and
  the **cash/percent/financed dispatch** (`TestDOSMtgDownPaymentDispatch`, 500
  cases, ~1e-9). With the earlier monthly+price solves, the mortgage engine —
  solve, APR, and full down-payment dispatch — is now differentially validated.
  (`GenerateRows` is `Calc` in a loop, already covered.)
- **2026-06-09 — VR periodic + COLA (#87): DONE.** Added oracle `vrp` mode
  (variable-rate PERIODIC stream via the real `FancySummation`). Single-rate
  `vrp` reproduces a plain periodic PV exactly; `TestDOSVRPeriodicSweep` (500
  random multi-step schedules, with and without COLA) matches DOS with **0
  divergences, max relErr 1.05e-9**. The other parts of #87 are out of oracle
  reach: the VR **POD** backward solve depends on the blocked actuarial code,
  and the VR **date** solve is the documented intentional enhancement over a DOS
  limitation (DOS cannot solve it, so there is nothing to diff against).

## The single highest-value item: per-period rows (amortization, PV, VR)

**Gap.** The amortization oracle compares only **totals and the solved
payment**. The DOS `MakeTable` switches to a summary (no per-line detail) once a
schedule exceeds 14 installments (`AMORTIZE.pas:1470`), so for any normal-length
loan we never diff the per-period interest / principal / running balance. The
same is true for the PV and VR table output. This is the biggest blind spot:
totals reconcile while a per-row split could be off.

**Work.** Drive the oracle for **≤14-installment** loans (where `MakeTable`
emits full per-line detail), parse each detail line, and diff every row's
interest/principal/balance against the Go schedule. Short loans exercise the
same period-by-period engine code as long ones, so validating per-row there
generalizes; the existing totals sweep continues to cover long loans. Pairs with
every option below (run each option's sweep in ≤14-installment mode to get its
per-row detail).

**Effort:** moderate. **Lifts:** amortization, PV forward, VR (per-row detail is
shared output machinery).

---

## Amortization (96 → 99)

Current oracle coverage: ordinary loans (totals + payment) and single/double
balloons (payment solve), 30/360 basis, payments one period out.

Gaps to close:

1. **Per-row detail** — as above.
2. **The other fancy options, none yet oracle-tested:** prepayments, rate
   adjustments (ARMs), moratorium, target (min principal reduction),
   skip-months, Rule-of-78, in-advance (annuity-due), USA-rule. The Go engine
   supports all of these (`types.go`); only balloons are differentially checked.
   The `amort_oracle.pas` option arrays (`pre[]`, `adj[]`, `mor`, `targ`, `skp`)
   are already allocated — the driver just needs tokens to populate them, mirror
   the balloon work, and sweep each option and representative combinations.
3. **Fancy backward solves** — `SolveLoanAmount` / `SolveRate` /
   `SolveBalloonAmount` under fancy options (only the balloon *payment* solve is
   tested today). These are the two `// TODO: verify logic` markers in
   `backward.go`.
4. **Basis / frequency variants:** 365-day + exact mode, biweekly (26/52). Only
   30/360 is swept.

**Effort:** the largest of the five (it has the most option surface), but
entirely mechanical given the balloon work as the template.

---

## Present value, forward (97 → 99)

Current coverage: single lump and single periodic (level + COLA, both modes).

Gaps:

1. **Multi-row worksheets** — several lump and periodic lines mixed in one
   calculation (the real screen allows many rows). Only single-row is swept.
2. **Per-row detail** in the PV table (shares the per-period item above).
3. **Exact-mode** per-payment summation vs the closed form.

**Effort:** small-to-moderate (the driver already builds line arrays; this is
mostly populating more of them).

---

## Variable-rate / PVLfancy (93 → 99)

Current coverage: a single lump discounted through a multi-step rate schedule.

Gaps:

1. **Periodic streams under VR** (only lump is tested) and **VR + COLA**.
2. **VR backward solves** — the ported POD-solve and date-solve paths
   (`solveVariableRatePOD`, `vrUnknownDate`, `solveVariableRateDate`) have no
   oracle check yet.
3. **Multi-row** VR worksheets.
4. Note: the VR **date-solve is a documented intentional enhancement** over a
   DOS limitation — it must stay flagged as an intended divergence, not counted
   as a sweep failure.

**Effort:** moderate (extend the `vr` driver mode to periodic + the solves).

---

## PV backward solvers (96 → 99)

Current coverage: rate solve (direct vs DOS), lump amount and periodic amount
(round-trip through the bit-identical forward).

Gaps:

1. **As-of date solve** and **lump/periodic date solves** (`BackwardLumpDate`,
   `BackwardPeriodicToDate`, the as-of path) — not yet oracle-checked.
2. **Direct (non-round-trip) amount solves.** The lump/periodic amount solves
   are currently validated by round-trip because the DOS engine's amount-solve
   runs through the `bf` screen-backup object, which depends on the GUI column
   layout (`blockdata`/`fcol`/`dataoffset`) we don't drive headlessly. Closing
   this means initializing that screen-layout state so `BackwardCalc` runs
   directly — then the amount solves are diffed against DOS, not just
   round-tripped. (The round-trip is already rigorous; this is a belt-and-
   suspenders upgrade.)
3. **Multi-row** backward (which-field-blank across a worksheet with several
   rows).

**Effort:** moderate; item 2 (screen-layout wiring) is the fiddly part and is
optional given the round-trip already validates those paths.

---

## Mortgage (96 → 99)

Current coverage: solve-monthly (incl. points + balloon) and solve-price.

Gaps:

1. **The remaining dispatch permutations:** solve from cash vs financed vs
   percent-down explicitly, and the **balloon-amount** solve (`SolveBalloonAmount`
   exists in the engine but isn't swept).
2. **APR comparison** (`CompareAPRs`) — compares two mortgages and finds the
   crossover; its own DOS code path (`IterateToFindCrossoverAPRandTime`), not
   exercised at all by the oracle.
3. **Row generation** (`GenerateRows` / `EnoughDataForRowGeneration`) — the
   vary-a-field table feature, also unexercised.
4. Tax field and the monthly-blank-with-balloon interplay.

**Effort:** small-to-moderate; CompareAPRs and GenerateRows need new
`mtg_oracle` modes but follow the proven pattern.

---

## Cross-cutting (applies to all five)

- **Date conventions.** Every sweep uses a fixed loan date with the first
  payment exactly one period out. To reach 99, parametrize over: odd first
  periods (short/long stubs — the documented bug-magnet), varied day-of-month,
  leap years, and the **century boundary / Y2K** handling (DOS `centurydiv`,
  `TDateTime` epoch). These are where date bugs hide.
- **Boundary values.** Explicit cases at rate = 0, n = 1, very high rates and
  very long terms, and the `Round2` half-down rounding behavior at the .5 cusp.
- **Keep intentional divergences flagged.** The VR date-solve enhancement (and
  any other documented deviation) must be asserted as *intended*, so a "99"
  number isn't quietly hiding a real regression — or quietly counting an
  improvement as a failure.

---

## Suggested sequence (highest value first)

1. **Per-row detail in ≤14-installment mode** — unlocks per-line validation for
   amortization, PV, and VR at once. Biggest single confidence gain.
2. **Amortization fancy options** (prepay/adjust/moratorium/target/skip/R78/
   in-advance/USA) + their backward solves — the largest remaining option
   surface; closes the two `// TODO: verify logic` markers.
3. **Mortgage** CompareAPRs + GenerateRows + dispatch permutations — distinct
   code paths currently untested.
4. **VR** periodic + COLA + the POD/date solves.
5. **PV** multi-row + as-of/date backward solves.
6. **Cross-cutting** date/basis/boundary parametrization across all sweeps.
7. Optional: upgrade PV amount backward from round-trip to direct `BackwardCalc`
   diff (screen-layout wiring).

Each step is gated by the same CI harness already in place, so coverage only
ratchets up — once a path is differentially validated it stays validated.

## Honest caveat on "99"

Even with all of the above, the meaning is "exhaustively differentially
validated against the original DOS engine across the feature matrix, per-row,
with edge and date coverage, in CI." It is not a literal probability of zero
bugs. The residual risk after this work is: (a) a feature combination nobody
swept because nobody thought of it, and (b) the handful of documented intentional
divergences, which are deliberate. Both are mitigated by the method — the oracle
is the original program's own code, so any case we *do* run is judged by the real
thing, not by our interpretation of it.
