# Per%Sense — Missing Field-Driven Formula Flows

> **STATUS as of 2026-05-27 — historical document.** This audit was
> written when only the forward paths of PV and Amortization were
> wired. Since then every gap below has been closed end-to-end —
> engine, API, *and* frontend. The R10 pass (2026-05-27) widened
> the PV `FirstPass` dispatcher so a row that supplies its own
> target Value with one core field blank fires the matching solver
> (PV-1 / PV-2 / PV-4 / PV-5 / PV-6) without needing a screen-level
> Sum Value; the `getPVInput` and `getAmzInput` JS validators were
> updated to stop rejecting the half-filled rows those solvers
> need; and the per-row Value cell on the PV screen is now editable
> as a target. Target balloon (`AmortBalloonReq.Amount` nil),
> unknown prepayment amount/duration, and AO7 "re-amortize at
> current rate" are likewise reachable from the UI now. Six new
> API-layer tests pin the contract: `TestPV{1,2,4,5,6}RowLevel*`
> and `TestAmortTargetBalloonViaAPI` in
> `internal/api/pv_row_backward_test.go`. Current state of the
> port is tracked in:
>
> - `CLAUDE.md` — "What's Ported" table and "Outstanding Items"
> - `docs/dispatch_gaps.md` — the living gap doc (latest Revision 9,
>   2026-05-26)
> - `docs/missing_flows_pass2.md` — the second-pass audit, most of
>   whose items are also now closed
>
> Each section below has been annotated with a **Status** line citing
> the file/function that closed the gap. The body is preserved
> verbatim as a historical record of the original analysis.

Comparison of conditional formula dispatch in `legacy/src/dos_source/`
(authoritative for financial logic per CLAUDE.md) vs. the current Go port
under `internal/finance/` and `internal/api/`.

The DOS application's distinguishing behavior is **field-presence dispatch**:
the user fills in some fields and leaves others blank, and the program
selects a formula path to solve for whatever's missing. Each row carries a
status (`empty`, `contains_unknown`, `fully_specified`, `over_determined`,
…) and `FirstPass` classifies the screen state into a `frontward` or
`backward` solve mode.

The mortgage screen ports most of this. The **present value** and
**amortization** screens do not — only the forward path is currently wired.

---

## 1. Present Value — `BackwardCalc` is entirely missing

> **Status: RESOLVED.** `FirstPass`, `BackwardCalc`, and the per-path
> solvers (`solveLumpAmount`, `solveLumpDate`, `solvePeriodicAmount`,
> `solvePeriodicDate`, `solveRate`, `solveAsOf`) all live in
> `internal/finance/presentvalue/backward.go`. The dispatcher
> `Calculate(input)` routes to forward or backward based on
> `FirstPass` flags. `HandlePVCalc` in `internal/api/handlers.go`
> uses pointer fields so absent values map to `StatusEmpty`. PV-1,
> PV-2, PV-4, PV-5, PV-6, PV-8, and PV-9 are all ported. The PV-3
> and PV-7 error arms are ported in `FirstPass`. The only PV
> dispatch arm still flagged is the `V_3` ifdef block in
> `solvePeriodicDate`, which is dead code in the authoritative DOS
> build (CLAUDE.md "Outstanding Items"; `dispatch_gaps.md` §0.5.5).
>
> The body below is preserved as the original analysis.

**DOS authority:** `legacy/src/dos_source/PRESVALU.pas`,
`procedure BackwardCalc` (lines 828–1085) and the rate/as-of solver
embedded in `procedure FrontwardCalc` (lines 668–818).

**Go port:** `internal/finance/presentvalue/calc.go` — only the forward
path is implemented. `types.go:10` advertises "Backward calculation
(value → unknown rate, date, or amount)" but no such function exists.
`Calculate()` (line 182) requires rate AND as-of, and skips any row
where date or amount is missing (lines 201–203, 229–232).

The API handler `internal/api/handlers.go:321` (`HandlePVCalc`) stamps
every field with `types.InOutInput`, leaving no path for the caller to
indicate that a field should be solved for.

### Specific solve paths missing from the port

| # | Inputs given (status `inp`) | Field to solve | DOS reference |
|---|---|---|---|
| PV-1 | Lump sum row: `date`, `value`; rate; as-of | lump-sum `amount` | PRESVALU.pas:866–891 — `amt0 := val0 * exxp(rate * YearsDif(date, asof))`, with actuarial divide by `LifeProb` |
| PV-2 | Lump sum row: `amount`, `value`; rate; as-of | lump-sum `date` | PRESVALU.pas:892–931 — Newton-Raphson on date; errors out under `fold_in_life` |
| PV-3 | Lump sum row: only `value` (no date, no amount) | error | PRESVALU.pas:932–935 — "Specify either date or amount in Single Payments, line N" |
| PV-4 | Periodic row: `fromdate`, `todate`, `value`; rate; as-of | periodic `amount` | PRESVALU.pas:943–956 — `amtn := valn / Summation(1, j)` |
| PV-5 | Periodic row: `fromdate`, `amount`, `value` | periodic `todate` | PRESVALU.pas:965–999 — solve via `lnn(last*f/first)/(rate-cola)`, then ±1 period iteration; special case when `rate ≈ cola` uses `AddNPeriods` |
| PV-6 | Periodic row: `todate`, `amount`, `value` | periodic `fromdate` | PRESVALU.pas:1000–1070 — two-pass approximation; second pass only fires when `cola ≠ 0` |
| PV-7 | Periodic row: only `value` and one date (no amount, no other date) | error | PRESVALU.pas:1080–1083 — "Specify either other date or amount in Periodic Payments, line N" |
| PV-8 | All rows fully specified; `r.status < defp` (rate blank); `sumvalue` known | discount `rate` | PRESVALU.pas:693–754 — Newton iteration on rate, with second-pass restart from 0; "Rate is not determined - specify amounts instead of values" failure path |
| PV-9 | All rows fully specified; rate known; `asofstatus < defp` (as-of blank); `sumvalue` known | `asof` date | PRESVALU.pas:755–818 — iterative date solve, "Cannot compute date - interest rate too small" failure when `\|rate\| < teeny` |
| PV-10 | Rows fully specified; `sumvaluestatus < defp` | `sumvalue` (the trivial "compute total" branch — actually present, but wired *only* on the API path that always supplies all inputs, so never exercised as the missing-field branch DOS uses) | PRESVALU.pas:669–692 |

### Supporting machinery also missing

- **`FirstPass` row classification** — DOS sets each row's status from
  `empty` through `fully_specified`/`over_determined` and computes the
  `frontward`/`backward` flags (PRESVALU.pas:544–654). Without this,
  the port has no way to detect "exactly one unknown" vs. "too many
  unknowns" vs. "insufficient data".
- **"Too many unknowns" error** — PRESVALU.pas:1242 — fires when
  `frontward AND backward` are both true.
- **Over-determination warning** — PRESVALU.pas:1166–1189 — warns
  when a row's value is filled but is already determined by data
  above.
- **Insufficient-data error with helpful messages** — DOS gives
  per-row guidance (which field to fill); the port returns a single
  generic "need rate and as-of date" message (calc.go:188).
- **YieldRateTranslation** — PRESVALU.pas:535 — translates between
  rate representations (yield ↔ true rate ↔ loan rate) before
  classifying status. Tied into the PV rate-type selector
  discrepancy already noted in `docs/discrepancies.md` §4.

### API-layer fix needed alongside

`HandlePVCalc` needs to stop hard-coding `types.InOutInput` on every
field. Each request field should be a pointer (matching the mortgage
handler at `handlers.go:163–207`) so absence ⇒ `StatusEmpty`. Then
`Calculate()` (or a new dispatcher) can route to the correct solve
path.

---

## 2. Mortgage — APR comparison and row generation not ported

> **Status: RESOLVED.** Both flows are ported:
>
> - **APR comparison** — `CompareAPRs` at
>   `internal/finance/mortgage/mortgage.go:484`, exposed via
>   `POST /api/mortgage/compare` (`HandleMortgageCompare` in
>   `internal/api/handlers.go`).
> - **Row generation (What-If)** — `GenerateRows` and
>   `EnoughDataForRowGeneration` at
>   `internal/finance/mortgage/rowgen.go:38,52`, exposed via
>   `POST /api/mortgage/whatif` (`HandleMortgageWhatIf`).
>
> The body below is preserved as the original analysis.

The core `Calc` dispatch (Mortgage.pas:192–310) is faithfully ported in
`internal/finance/mortgage/mortgage.go:142–248` (`Calc`) and exposed via
`HandleMortgageCalc` with proper pointer-based optional fields. Two
adjacent flows are missing:

### 2a. Mortgage row generation

**DOS:** `EnoughDataForRowGeneration` at Mortgage.pas:33, 839–841 —
true when `(price OR monthly OR howmuch).status = outp`. The
`MortgageRowGenerationDlgUnit.pas` lets the user vary one input
(rate, years, points, etc.) across a series of rows.

**Go port:** No equivalent. Grep for `RowGeneration|EnoughDataForRowGeneration`
in `persense-port/` returns no matches.

This isn't strictly a "different formula based on field presence" —
it's a UI flow that *uses* field-presence dispatch — but the
underlying logic needs the same fields-known check.

### 2b. APR comparison between two mortgages

**DOS:** `ReportComparisonOfAPRs(row1, row2, ...)` at
Mortgage.pas:31, 613+. Computes the crossover term where two
mortgages' total cost crosses, given different APRs/points/balloons.

**Go port:** `crosscheck_test.go` exists at `internal/finance/`
suggesting some crossover logic is intended, but there's no
`Compare`/`Crossover` exported function in `mortgage.go`. The
APRComparisonDLGUnit dialog has no API counterpart in `handlers.go`.

---

## 3. Amortization — solving for loan amount or unknown prepayment

> **Status: largely RESOLVED.** The three sub-items below are ported
> to varying degrees:
>
> - **3a — `ComputeLoanAmount`** — `SolveLoanAmount` at
>   `internal/finance/amortization/backward.go:128`. Plus
>   `SolvePayment` (line 70) and `SolveRate` (line 208) for the
>   sibling unknowns. Dispatched from `HandleAmortizationCalc` when
>   `Amount` or `Rate` is omitted.
> - **3b — Unknown prepayment row (`unkpre`)** — handled in
>   `engine.go` around line 284 ("unknown prepayment — solve the
>   per-payment amount"); tested in
>   `amortization/unknown_prepay_test.go`. See
>   `dispatch_gaps.md` for residual fancy-loan iteration edges
>   (the `// TODO: verify logic` markers in `backward.go`).
> - **3c — Target-balloon iteration** — `EstimateAndRefineBalloon`
>   equivalent is wired via the `AmortBalloonReq.Amount` pointer
>   (nil ⇒ solve); see `target_balloon_test.go`. The minimum
>   principal-reduction "target" feature is in `engine.go` and
>   tested in `target_balloon_test.go` /
>   `moratorium_target_test.go`.
>
> The body below is preserved as the original analysis.

**DOS authority:** `legacy/src/dos_source/Amortize.pas`.

**Go port:** `internal/finance/amortization/engine.go`.

`Amortize()` at engine.go:127 hard-requires `loan.AmountStatus >=
InOutDefault` and `loan.PerYrStatus >= InOutDefault` (lines 132–135).
DOS handles two additional cases:

### 3a. `ComputeLoanAmount` — solve for loan amount

**DOS:** Amortize.pas:870 — `if not (((amountstatus >= defp) or
ComputeLoanAmount) and (adjok))` shows that when amount is unknown,
DOS calls `ComputeLoanAmount` to back into the principal from
payment + rate + term + (optional target balance/balloon). The body
of `ComputeLoanAmount` runs the schedule with a guess and iterates
until the terminal balance matches the target.

**Go port:** No `ComputeLoanAmount` — passing a request without
`Amount` returns "insufficient loan data".

### 3b. Unknown prepayment row (`unkpre`)

**DOS:** Amortize.pas:679–697 — when one of the prepayment rows
is left blank, the schedule iterates and solves for that prepayment
amount such that the loan terminates correctly:

```
for i:=1 to npre do if (i<>unkpre) then with pre[i]^ do ...
with pre[unkpre]^ do ...
FirstLastAndFF(rate, first, last, ff, unkpre);
```

**Go port:** The `Amortize` engine accepts a list of `Adjustments`
but not a list of prepayments with one marked unknown. No equivalent
solver.

### 3c. Target-balloon iteration

**DOS:** `targ^.targetstatus` checks at Amortize.pas:288–292,
1370 — when a target balance is specified for a future date, DOS
solves backward to choose the payment amount or the rate that hits
that target.

**Go port:** No target/balloon-target solver visible in
`engine.go`.

---

## Recommended sequencing for the port

> **Historical.** Items 1–7 below were the planned porting order in
> the original audit. Items 1–5 (PV restructure, FirstPass, easy
> backward arms, rate solver, date solvers) and items 6–7 (amortization
> backward, mortgage compare/what-if) are all complete; see the
> annotations on §1, §2, §3 above. The plan is preserved so future
> readers can map the sequencing decisions to the resulting commits.

These are listed in roughly increasing implementation cost.

1. **Restructure the PV API surface** (`HandlePVCalc`) to use pointer
   fields so absence ⇒ `StatusEmpty`. This is a prerequisite for
   anything below.

2. **Port `FirstPass`** to the PV package: classify each row's
   status, set `frontward`/`backward` flags, return "too many
   unknowns" / "insufficient data" with the same per-line messages
   the DOS code emits.

3. **Port the easy `BackwardCalc` arms** (PV-1, PV-4 — closed-form
   solves for amount given dates+value). These don't need iteration.

4. **Port the rate solver** (PV-8). Use the same Newton method with
   second-pass restart from 0 that DOS uses; the iteration constants
   (`<teeny`, `count=30`) and damping (`±0.04`) need to match
   exactly per the project's "preserve original Pascal business
   logic" rule.

5. **Port the date solvers** (PV-2, PV-5, PV-6, PV-9). Watch for the
   `cola ≈ rate` and `cola ≠ 0` special-case branches, and the
   `±1 period` refinement loop after the closed-form approximation.

6. **Amortization `ComputeLoanAmount`** and **unknown prepayment**
   solvers. These need the same iteration scaffolding as the PV
   rate solver — likely a shared Newton helper in `internal/finance/`
   would be cleaner than per-module copies.

7. **Mortgage row generation** and **APR comparison** dialogs — UI-
   level work; the underlying `Calc` already handles each individual
   row.

Each of these should be cited with the originating Pascal file and
function (per CLAUDE.md's "cite the original filename in a comment"
rule), and validated against `legacy/reference-output/` known-good
outputs where they exist.
