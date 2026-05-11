# Per%Sense — Missing Field-Driven Flows, Second Audit Pass

This is the result of a second walk through `legacy/src/dos_source/` to find
field-presence dispatch arms not yet captured in `docs/missing_flows.md`.

**Update (post-fix):** Items marked **[DONE]** below were addressed in a
follow-up commit. See `internal/finance/amortization/firstpass.go`,
`validate.go`, the new `SolvePayment` in `backward.go`, the per-row error
arms in `presentvalue/backward.go: FirstPass`, and the API handler fix in
`internal/api/handlers.go`. Tests for each fix live in
`firstpass_test.go`, `solvepayment_test.go`,
`firstpass_errors_test.go`, and `amort_firstpass_test.go`.

The first pass (see `docs/missing_flows.md`) is now partly out of date —
most of what it called out has since been ported. Specifically: PV
`BackwardCalc` (PV-1, 2, 4, 5, 6, 8, 9), mortgage `CompareAPRs`, mortgage
`GenerateRows`, amortization `SolveLoanAmount` and `SolveRate` all now
exist (see `CLAUDE.md`'s "What's Ported" table).

The gaps below are organized by:
- **A. Confirmed still-open (already known)** — items still flagged in
  `CLAUDE.md` or `missing_flows.md`. Listed for completeness.
- **B. New gaps — solvers/dispatch arms** — DOS field-presence paths that
  produce a numeric result but have no Go equivalent.
- **C. New gaps — input-validation / warning arms** — DOS errors and
  warnings tied to specific field combinations. Easy to miss because they
  don't produce numeric output, but they're part of the dispatch
  contract.
- **D. Behavioral knobs declared but unused** — fields/flags wired through
  the Go types but never read.

Each entry cites the DOS source location and the Go file that would need
to host the port.

---

## A. Confirmed still-open (already documented)

| ID | What | DOS reference | Doc reference |
|---|---|---|---|
| A-1 | Amortization unknown-prepayment (`unkpre`) | AMORTOP.pas:400-475 (`CheckPrepayments`), AMORTOP.pas:665-707 (`EstimateAndRefinePeriodicPrepayment`) | CLAUDE.md "Outstanding Items"; `missing_flows.md` §3b |
| A-2 | Amortization target-balloon | Amortize.pas around line 1370 | CLAUDE.md; `missing_flows.md` §3c |
| A-3 | PV `V_3` ifdef block in `solvePeriodicDate` (`const_signal` propagation) | PRESVALU.pas:1003-1008 | CLAUDE.md; `presentvalue/backward.go:572` TODO |
| A-4 | Prepayment `NPmts` accepted by API but ignored by engine | engine.go:346-370 only consults `StopDate` | CLAUDE.md |
| A-5 | PV rate-type selector → `InterpretedRate` translation | PRESVALU.pas:535 (`YieldRateTranslation`) | `discrepancies.md` §4 |

---

## B. New gaps — solvers / dispatch arms

These are field-presence dispatch arms in DOS that produce a numeric
output but have no equivalent in the Go port.

### B-1. Amortization: solve for **last payment date** / **number of payments** [PARTIAL: classifier done; solver still open]

DOS arm **A-EN-3** (`Amortize.pas` → `DetermineLastPaymentDate`, called from
`Enter` around line 1323 when `payamtstatus >= defp` AND `lastok = false`).

The pre-dispatch classifier `FirstPass` (Amortize.pas:196-321) has three
related arms that all run on field-presence:

- **A-FP-defFirst**: `firststatus < defp` AND `loandatestatus > defp` AND
  `peryrstatus >= defp` → default `firstDate ← loanDate + 1 period`
- **A-FP-last**: `firststatus >= defp` AND `nstatus >= defp` →
  `lastDate ← firstDate + (n-1) periods`, set `laststatus := outp`
- **A-FP-n**: `firststatus >= defp` AND `laststatus >= defp` →
  `n ← NumberOfInstallments(firstDate, lastDate)`

**Go port today:** **[DONE]** for the three derivation arms (A-FP-defFirst,
A-FP-last, A-FP-n) — see `internal/finance/amortization/firstpass.go`.
The API handler's `LastOK: true` hardcode is also fixed; `LastOK` is now
set by `FirstPass` when a valid last-payment date is known. Still open:
the iterative `SolveLastDate` for the case where amount + rate + payment
are given but term is unknown.

Suggested home: `internal/finance/amortization/engine.go` (`FirstPass` /
classifier), with a new solver `SolveLastDate` next to `SolveLoanAmount`.
Fix the handler at the same time.

### B-2. Amortization: solve for **payment amount** [DONE]

DOS arm **A-EN-7** (`Amortize.pas` → `EstimateAndRefinePayment`, lines
377-430). Fast path: when no advanced features are on, closed-form
`d := amount * (f-1) / (1 - 1/f^n)`. Iterative path: `Iterate` for fancy
loans.

**Go port today:** Added as `SolvePayment` in
`internal/finance/amortization/backward.go`. Closed-form annuity formula
mirroring DOS `EstimateAndRefinePayment`'s fast path. Round-trip with
`SolveLoanAmount` is covered in `solvepayment_test.go`. Fancy-mode
iterative refinement (when prepayments/balloons/adjustments are present)
is still TODO — the closed-form result is a good initial estimate but
won't honor advanced features exactly.

### B-3. Amortization: adjustment-row solving (ARM with unknown new rate or payment)

DOS arms **A-EN-Adj-pmt** / **A-EN-Adj-rate** (`Amortize.pas` per-adj loop,
~lines 1380-1410) and **A-RA-no-amt** (`AMORTOP.pas: Re_Amortize`,
1499-1613).

Pattern: an adjustment row carries an effective date plus either a new
rate, a new payment, or both. When one is filled in, DOS solves for the
other so the loan still amortizes over the original term.

| DOS condition on `adj[i]` | What DOS solves |
|---|---|
| `loanratestatus >= defp` AND `amountstatus < defp` | new payment after this date (Iterate via `RepayFancyLoan`) |
| `loanratestatus < defp` AND `amountstatus >= defp` | new rate after this date (Iterate) |
| Neither filled | recompute payment from remaining principal / remaining periods |

**Go port today:** `internal/api/handlers.go` accepts `AmortAdjustmentReq`
with both `Rate` and `Amount` pointers. The engine
(`generateFancySchedule` engine.go:417-435) applies whatever's present at
the adjustment date — it never solves for the missing one. An adjustment
with only a rate change quietly leaves the original payment in force,
which will produce a wrong terminal balance for ARMs.

Suggested home: `engine.go` per-adjustment dispatch + a new
`SolveAdjustment` in `backward.go`.

### B-4. Amortization: APR with points

DOS arm **A-EN-APR** (`Amortize.pas` → `EstimateAndRefineAPRwithPoints`,
lines 516-615). Secant iteration with 20-count cap and `tiny`/`teeny`
acceptance thresholds. BOFA-conditional basis flip to x360.

**Go port today:** `AmortResult.APR` and `APRConverged` fields are
declared (types.go:168-169), and `Loan.Points` / `Loan.PointsStatus` /
`Loan.APR` / `Loan.APRStatus` are declared (types.go:55-58), but **no Go
code reads or sets any of them**. The mortgage package has APR via
`mortgage.FullTermAPR` / `IterateToFindAPR` but no shared/amortization
version. So if you load an amortization with points, the APR field comes
back at zero.

Suggested home: `internal/finance/amortization/apr.go`, or refactor
`mortgage`'s Newton-style APR solver into a shared helper in
`internal/finance/`.

### B-5. Amortization: TackOnFinalBalloon

DOS arm **A-EN-TackOn** (`Amortize.pas` near the bottom of `Enter`). When
amount, rate, and payment are all set and there's no unknown balloon, DOS
auto-appends a terminal balloon row with the residual balance at
`lastDate`.

**Go port today:** No equivalent. A loan that doesn't amortize cleanly
just shows a residual balance on the final row of the schedule but
doesn't surface it as a balloon. Probably not critical, but it's the
mechanism by which the DOS UI surfaces "you'd owe $X at the end."

Suggested home: post-pass in `generateFancySchedule`.

### B-6. Amortization: balance-lookup row (balance ↔ date)

DOS arms **A-EN-Bal-from-Date** and **A-EN-Date-from-Bal**
(`AMORTOP.pas`, `ComputeBalanceFromDate` and `ComputeDateFromBalance`).
The Amortization screen has a separate "Balance Lookup" block where the
user can ask "what's my balance on date X?" or "when will my balance hit
amount Y?"

**Go port today:** There's no `AMZBalanceBlock` equivalent in the API or
the engine. The schedule's payment rows expose `RemainingBalance` per
period, so a caller could approximate this client-side by interpolating,
but the DOS exact form (which handles prepayments, balloons, and rate
adjustments in the intervening period) isn't exposed.

Suggested home: new `BalanceLookup` API endpoint and engine method, or
extend `AmortRequest` with optional `BalanceLookupRows`.

### B-7. PV: PV-3 / PV-7 explicit error arms

DOS rejects contains_unknown lump-sum rows where only `value` is set with
the message "Specify either date or amount in Single Payments, line N"
(PRESVALU.pas:932-935) and the periodic equivalent at PRESVALU.pas:
1080-1083.

**Go port today:** *Has* both of these error arms (`backward.go:237-240`
and `:266-269`). Confirming they're not new gaps; the first audit pass
flagged them as missing but they have since been added.

### B-8. PV-5 / PV-6 second-pass refinement when cola ≠ 0

DOS lines PRESVALU.pas:1060-1070 do a second-pass approximation when
solving for `fromdate` and `cola ≠ 0`. Go performs only the first pass
plus the `±1 period` refinement.

This is already noted as a TODO in `backward.go:575` but **not flagged in
the project-level docs**.

### B-9. Mortgage row-generation "EnoughDataForAPR" predicate

Cosmetic: DOS gates the APR display on `EnoughDataForAPR` (Mortgage.pas:
571-575). Go has the equivalent `mortgage.EnoughDataForAPR`. Not a gap —
listed only because the first audit pass implied it was missing.

---

## C. New gaps — input-validation / warning arms

These are DOS dispatch arms that produce error or warning messages tied to
specific field combinations. They're part of the dispatch contract even
though they don't produce numeric output. Several are easy to miss
because they're scattered in the `FirstPass` / `Enter` procedures.

### Amortization

| ID | DOS site | Trigger condition | DOS behavior | Go status |
|---|---|---|---|---|
| C-A-1 | `AMORTOP.pas: SortAdj` ~line 308 | Two adjustment rows on same date | Error "Two rate adjustments on the same day" | **[DONE]** `validate.go: ValidateInputs` |
| C-A-2 | `AMORTOP.pas: SortAdj` | First adjustment date ≤ loanDate | Error "Rate change cannot precede loan" | **[DONE]** |
| C-A-3 | `AMORTOP.pas: SortAdj` | Last adjustment date ≥ lastDate | Error "Can't change rate after last payment" | **[DONE]** |
| C-A-4 | `AMORTOP.pas: SortBalloons` ~line 305 | First balloon date < firstDate | Error "Balloon cannot precede first regular payment" | **[DONE]** |
| C-A-5 | `Amortize.pas: Enter` ~line 1380 | `DateComp(firstDate, lastDate) >= 0` | Error "There must be at least two regular payments" | **[DONE]** — Go variant rejects `>` (not `>=`) to support degenerate one-payment loans |
| C-A-6 | `Amortize.pas: Enter` | Moratorium first-repay < firstDate | Error "principal repayment cannot precede first pay" | **[DONE]** (DOS audit had the comparison direction inverted; the actual semantic is moratorium-before-firstDate is invalid) |
| C-A-7 | `Amortize.pas: Enter` | Balloon before moratorium first-repay | Error "No balloon can precede first repayment" | **[DONE]** |
| C-A-8 | `Amortize.pas: Enter` | `df.c.in_advance` AND `nadj > 0` | Error "can't change rates when interest is paid in advance" | not applicable (in_advance unimplemented; see D-3) |
| C-A-9 | `Amortize.pas: Enter` | `amount/nrepay < target` | Error "principal reduction target too high" | **[DONE]** |
| C-A-10 | `Amortize.pas: Enter` | `fancy && targetstatus >= defp` AND solving for loan amount (A-EN-6) | Error "Cannot do Borrowed with Target Reduction" | **[DONE]** in `SolveLoanAmount` |
| C-A-11 | `Amortize.pas: DeterminePrepaymentDuration` ~line 730 | `df.c.plus_regular = true` AND solving for unknown prepayment duration | Prompts user to set "balloon includes regular pmt" to NO; cancellable | not applicable (unkpre unimplemented) |

### Present Value

| ID | DOS site | Trigger condition | DOS behavior | Go status |
|---|---|---|---|---|
| C-P-1 | `PRESVALU.pas:1166-1189` (within `Enter`) | A row's value is filled (status ≥ `defp`) but is already determined by data above | Warning "Value already determined - continue anyway?" with cancel option | not checked — Go has screen-level "too many unknowns" but no per-row "already determined" warning |
| C-P-2 | `ComputeLumpsumLineValues` (PRESVALU.pas:169-194) | lump-sum row is `contains_unknown` AND `amt0 = 0` (with `amt0status >= defp`) | RecordError on `amountcol` ("amount is zero, can't divide") | **[DONE]** in `presentvalue/backward.go: FirstPass` |
| C-P-3 | same | same with `val0 = 0` | RecordError on `valuecol` | **[DONE]** |
| C-P-4 | `ComputePeriodicLineValues` (PRESVALU.pas:466-533) | periodic row is `over_determined` (fromdate + todate + amount + value all present) | Error on row | **[DONE]** — added explicit over-determined error for both lump-sum and periodic rows |
| C-P-5 | `Enter` dispatcher around PRESVALU.pas:1206-1218 | `fold_in_life` requested AND no payment lines | POD-only dispatch — computes POD value alone | not applicable until actuarial UI exists |
| C-P-6 | PRESVALU.pas:1126-1134 `code = make_table` | NOT (frontward OR backward) | InsufficientDataMessage with field hints | Go returns "insufficient data on screen" (generic) — no per-row guidance |

### Mortgage

Mortgage validation looks complete vs DOS. All six FirstPass error
arms (`M-FP-1` through `M-FP-6`) and all six Calc error arms have
equivalents in `mortgage.go:142-248`. No new gaps here.

---

## D. Behavioral knobs declared but unused

Fields and flags that are wired into the Go type system but never read
by any solver. These represent "stubs" of DOS behavior that will silently
no-op until implemented.

| ID | Symbol | Declared | Purpose in DOS | Status |
|---|---|---|---|---|
| D-1 | `Settings.R78` | `amortization/types.go:131` | Rule of 78 amortization (front-load interest) | declared, never read in engine.go or backward.go |
| D-2 | `Settings.USARule` | `types.go` | USA Rule (unpaid interest accrual cap) | partially honored at engine.go:381 — verify completeness |
| D-3 | `Settings.InAdvance` | `types.go` | Interest paid at start of period | declared, not read; blocks C-A-8 |
| D-4 | `Settings.Exact` | `types.go` | Exact-day basis (vs 30/360) for interest accrual | declared, not consulted in fast-path short-circuits (engine.go: `EstimateAndRefinePayment` and `LoanAmount`) |
| D-5 | `Loan.Points`, `Loan.PointsStatus`, `Loan.APR`, `Loan.APRStatus` | `amortization/types.go:55-58` | Amortization-screen APR with points | declared, never read; B-4 |
| D-6 | `AmortResult.APR`, `APRConverged` | `amortization/types.go:168-169` | Same — output side | declared, never written |
| D-7 | `Prepayment.NN` (`AmortPrepaymentReq.NPmts`) | API + types | DOS prepaymentrec.nn — number of prepayments | accepted by API, ignored by engine (`generateFancySchedule` consults only `StopDate`); CLAUDE.md flags this |
| D-8 | `PresValLine.Duration`, `DurationStatus` | `presentvalue/types.go:80-81` | DOS computes effective duration of a PV stream | declared, never written or read |
| D-9 | `hard_payment` discipline (`payamtstatus = inp` triggers `Round2(interest)` per period) | `AMORTOP.pas: ComputeNext` CN-Round arm | DOS rounds interest to whole cents per period when user typed the payment | not modeled — Go computes interest with float64 throughout |
| D-10 | `Loan.LastOK` | API hard-codes `true` in handlers.go:338 | DOS toggles based on whether `lastDate` was supplied | **[DONE]** — handler no longer hardcodes; `FirstPass` sets `LastOK` based on derived/supplied lastDate |

---

## Recommended sequencing

Roughly in order of (impact × ease):

1. **Fix the `LastOK: true` hard-code in `handlers.go:338`** (D-10). It's
   a one-line fix and currently fights with the engine.
2. **Add the amortization input validations** (C-A-1 through C-A-9). All
   are simple `if` checks at the entry to `Amortize` or inside
   `SortBalloons` / `SortAdjustments`. They prevent silent miscalculation.
3. **Default-first-payment-date + last-date↔n derivation** (B-1's
   FirstPass arms `A-FP-defFirst`, `A-FP-last`, `A-FP-n`). Closed-form,
   no iteration; mirrors mortgage's `Calc` pattern.
4. **Amortization `SolvePayment`** (B-2). Most-requested missing-field
   case in practice.
5. **`SolveLastDate` / determine number of payments** (B-1 solver part).
   Closed-form for the non-fancy case, Newton for fancy.
6. **Adjustment-row solving** (B-3). Required for ARM modeling.
7. **PV per-row error arms** (C-P-1 through C-P-4) — surface clearer
   messages to the UI.
8. **Amortization APR-with-points** (B-4). Refactor the mortgage APR
   solver into a shared helper first.
9. **TackOnFinalBalloon** (B-5) — UI polish.
10. **Balance-lookup row** (B-6). Bigger lift; new API endpoint.

Once these are done, the only remaining items will be the previously-known
A-1 through A-5 list plus the V_3 ifdef variants in `PVLXSCRN.pas` which
the project has explicitly deferred.

---

## Methodology

Both passes were generated by reading every `.pas` file under
`legacy/src/dos_source/` and grepping for the field-presence dispatch
idioms (`status = inp`, `status >= defp`, `status < defp`, `*ok`, the
`fully_specified` / `contains_unknown` / `over_determined` line codes
defined in PETYPES.PAS:112-143 and 365-376).

Status sentinels for cross-reference:
- DOS `badp=-1`, `empty=0`, `outp=1` (= calculated/fromcalc), `defp=2`,
  `inp=3` — Go: `StatusEmpty=0`, `InOutOutput=1`, `InOutDefault=2`,
  `InOutInput=3`
- DOS line-status: `blank_line=0`, `missing_4/3/2`, `contains_unknown=253`,
  `fully_specified=254`, `over_determined=255` — Go mirrors at
  `internal/types/constants.go:178-189`
