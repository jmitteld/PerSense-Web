# Per%Sense — Dispatch Gap Analysis & Error-Message Audit

*Companion to `missing_flows.md` and `discrepancies.md`. Authored 2026-05-19
from a deep read of `legacy/src/dos_source/` against the current Go port and
the single-page frontend.*

---

## Executive summary

The client's two complaints are correct, but for different reasons than the
old `missing_flows.md` suggested.

**Field-presence dispatch.** The hallmark DOS behavior — "leave any field
blank and the program solves for it" — is patchily implemented in three
distinct ways across the three worksheets:

| Worksheet | DOS dispatch arms | Reachable end-to-end | Math exists in Go but unwired | Genuinely missing |
|---|---:|---:|---:|---:|
| Mortgage      | 8 main + 2 adjacent flows | 8 | 2 adjacent (CompareAPRs, GenerateRows) | 0 |
| Amortization  | 10 basic + 14 advanced    | 5 | 3 (SolveRate, SolveLoanAmount, SolvePayment) | 12 |
| Present Value | 12 row + screen          | 9 | 2 (PV‑8, PV‑9 in UI)                  | 3 |
| **Total**     | **46**                   | **22** | **7**                           | **15** |

Of the 24 missing-or-partial paths, **7 are pure wiring** (the engine code
already exists and is unit-tested; only the handler/UI plumbing is absent),
**12 require porting a discrete DOS procedure**, and **5 are UX work**
(per-cell error reporting, warnings channel, rate-type selector).

**Error messages.** I cataloged 62 user-facing error/validation strings.
**45% are vague** (identify a problem but not which field), **28% are
ambiguous** (user cannot infer what to change), and **7 distinct silent-failure
paths** produce wrong or empty output with no error at all — the most
dangerous user experience. The single most dangerous: submitting an
amortization request with `rate: 0` runs a 0%-rate schedule silently, when
the user almost certainly meant "solve for rate."

**Top three priorities** (high impact × low effort):

1. **Eliminate the two amortization silent-wrong-answer paths.** Switch
   `AmortizationRequest.Amount` and `.Rate` to `*float64`, dispatch missing
   fields to the already-implemented `SolveLoanAmount` and `SolveRate`. Effort: **S**.
2. **Wire the present-value rate solver to the UI** by deleting the stale
   "IRR not supported" guard at `cmd/persense/static/index.html:2381–2384`
   (the backend `solveRate` at `presentvalue/backward.go:848` has worked
   for over a release). Effort: **S**.
3. **Introduce a `FieldError` struct** so the engines can return
   `{code, field, message}` triplets instead of unstructured strings. This
   single change unlocks correct per-cell highlighting and lets us replace
   the brittle `explainMtgError` regex-matcher at `index.html:1537`. Effort: **M**.

The rest of this document enumerates every permutation per screen, proposes
specific fixes with effort estimates (S = same day, M = 2–4 days, L = a week+),
and lists every error string with a precise rewording.

---

## How to read this document

- **Status legend.** ✅ ported and reachable; ⚠ partial (notes what's
  missing); ❌ missing entirely; ▪ genuinely under-determined (DOS also
  cannot solve).
- **Citations.** All `file:line` references are verbatim against the repo
  at the time of writing. DOS Pascal lives under `legacy/src/dos_source/`
  and is the authority for financial logic per CLAUDE.md.
- **Sample inputs.** JSON bodies are ready to POST against the running
  server. The repo's `docs/QUICKSTART.md` covers boot and curl basics.
- **Effort estimates.** S = same day (an hour or two). M = 2–4 days
  including tests and DOS-regression validation. L = a week-plus,
  typically because the DOS procedure does multi-pass iteration with
  edge cases (cola=rate, sign mismatch, non-convergence fallbacks).

---

## 1. Mortgage screen

### 1.1 State summary

`mortgage.Calc` (`internal/finance/mortgage/mortgage.go:142`) is a faithful
port of DOS `Mortgage.pas:192–310`. Of the 8 dispatch arms DOS distinguishes
on this screen, **all 8 are ported** and reachable through
`POST /api/mortgage/calc`. The real gaps are at the boundaries:

- `CompareAPRs` (`mortgage.go:475`) is fully implemented including the
  Newton-iterated crossover-time computation, but **no HTTP route exposes
  it**. The frontend's `compareMtgAPR` (`index.html:1644`) does a local
  heuristic on stored APR values and never computes the crossover.
- `GenerateRows` (`rowgen.go:52`) is implemented, but the frontend's
  What-If feature (`index.html:1699`) loops `/api/mortgage/calc` per row
  in JavaScript instead of calling the Go function.
- Per-cell error reporting from DOS (`RecordError(row, col)`,
  `Mortgage.pas:182, 219, 234`) is collapsed into a single error string,
  so the UI must regex-match the message to identify the offending field.
- Two silent-output failure modes (Section 4.1, S-6 and S-7) where the
  engine returns "no error" with no computed cells.

### 1.2 Permutation matrix (Mortgage)

Columns: **Price · Points · %Down · Cash · Financed · Years · Rate · Tax ·
Monthly · BalloonYrs · BalloonAmt**. "I" = input (filled); "—" = blank.

| # | Inputs | Solves | DOS site | Go site | Status |
|---|---|---|---|---|---|
| M1 | Price + %Down (+Years+Rate+Tax) | Cash, Financed, Monthly | `Mortgage.pas:198–244, 285–289` | `mortgage.go:284–312, 238–251` | ✅ |
| M2 | Price + Cash (+Years+Rate)      | %Down, Financed, Monthly | `Mortgage.pas:214–228, 285–289` | `mortgage.go:294–301, 238–251` | ✅ |
| M3 | Price + Financed (+Years+Rate)  | %Down, Cash, Monthly     | `Mortgage.pas:229–243, 285–289` | `mortgage.go:302–310, 238–251` | ✅ |
| M4 | Monthly + %Down (+Years+Rate)   | Price, Cash, Financed    | `Mortgage.pas:291–307`          | `mortgage.go:252–275`          | ✅ |
| M5 | Monthly + Cash (+Years+Rate)    | Price, %Down, Financed   | `Mortgage.pas:295–296`          | `mortgage.go:263–264`          | ✅ |
| M6 | Monthly + Financed only         | **Error** "fill in % Down or Cash" | `Mortgage.pas:297–300`     | `mortgage.go:265–268`          | ✅ |
| M7 | Price + %Down + Years + Rate + Monthly + BalloonYrs | BalloonAmt | `Mortgage.pas:247–257, 271–283` | `mortgage.go:316–329, 226–237` | ✅ |
| M8 | All solvable columns filled     | **Error** "leave some blank"       | `Mortgage.pas:278–282`     | `mortgage.go:234–236`          | ✅ |
| M9 | Reverse-price with balloon known | Price                              | `Mortgage.pas:291–307`     | `mortgage.go:252–275`          | ✅ |
| M10 | Solve for **Rate** from Price, Monthly, funding, Years | — | n/a — DOS guard `Mortgage.pas:266` | n/a | ▪ DOS does not solve rate on this screen; use Amortization |
| M11 | Solve for **Years**             | —                                  | n/a — same guard          | n/a | ▪ Same |
| M12 | Solve for **Points** or **Tax**  | —                                  | n/a — never the unknown   | n/a | ▪ Same |
| M13 | APR for one row                 | (auto when row qualifies)         | `Mortgage.pas:538–555`    | `mortgage.go:442–444`; auto in `handlers.go:327–335` | ✅ |
| M14 | APR comparison between two rows + crossover time | crossover years | `Mortgage.pas:613–711, 391–535` | `mortgage.go:475–534, 540–651` | ⚠ unreachable — no `/api/mortgage/compare` route |
| M15 | What-If row generation (1-D)    | varied rows                       | `Mortgage.pas:852–1047` (compiled out, authority only) + `MortgageRowGenerationDlgUnit.pas` | `rowgen.go:52–97` | ⚠ unreachable — no `/api/mortgage/whatif` route; frontend reimplements client-side |
| M16 | What-If 2-D (vary two columns)  | grid                              | DOS allows up to 3 dimensions (`Mortgage.pas:906–912`) | Frontend loops; `GenerateRows` is 1-D only | ⚠ partial |

### 1.3 Solvable-but-not-wired (Mortgage)

These are pure plumbing fixes — the Go math is unit-tested and works.

1. **`POST /api/mortgage/compare`** → `mortgage.CompareAPRs`. The Newton-iterated
   crossover-time calculation matches DOS verbatim (`mortgage.go:540–651`),
   but no handler registers the route. Effort: **S**. Frontend wiring:
   replace the local `compareMtgAPR` heuristic at `index.html:1644`.
2. **`POST /api/mortgage/whatif`** → `mortgage.GenerateRows`. Currently the
   frontend's `runWhatIf` (`index.html:1699–1814`) reimplements
   `EnoughDataForRowGeneration` in JavaScript and loops `/calc` per row,
   imposing a network round-trip per generated row. Effort: **S**.
3. **2- and 3-D row generation.** DOS supports up to 3 varied columns
   (`Mortgage.pas:906`). Go's `GenerateRows` accepts a single `VaryField`.
   Extend to `[]Variation`. Effort: **M**.

### 1.4 Genuinely under-determined (Mortgage)

These combinations DOS also refuses; the Go port matches that refusal.

- **Rate from {Price, Monthly, funding, Years}.** DOS's `Calc` runs only
  when `ratestatus = inp` (`Mortgage.pas:266`). The user is expected to
  use the Amortization screen, where `SolveRate` exists (and now needs to
  be wired — see §2).
- **Years from {Price, Monthly, funding, Rate}.** Same guard.
- **Points or Tax solve.** Neither is ever the unknown — both are
  additive modifiers consumed by `ComputeCashPctAndFinanced`
  (`Mortgage.pas:209, 287`).
- **Cash from Financed (or vice versa) without Price.** Both formulas
  need Price as denominator (`Mortgage.pas:209, 216, 231`).
- **Monthly without one of {%Down, Cash, Financed}.** The main `if` at
  `Mortgage.pas:266` short-circuits. DOS produces no output silently;
  Go matches — but the user gets no feedback (Section 4.1, S-7).

### 1.5 Mortgage-specific issues

1. **Per-cell errors collapsed to single string.** DOS marks the
   offending cell with `RecordError(row, col)` so the UI can highlight
   one field; the Go API returns `result.Err: string` and the frontend
   re-derives the column with `explainMtgError` (`index.html:1537–1574`).
   That helper only parses the "leave X or Y blank" message — every
   other mortgage error falls through to a generic "Row N: <msg>"
   prefix. Recommended fix in Section 4.3 (`FieldError` struct).
2. **Field-name drift between engine and UI.** Engine says
   "monthly payment"; UI label is "Monthly Total." Engine says
   "amount borrowed"; UI label is "Amt Borrowed." Engine says
   "percent down"; UI label is "% Down." A single field-name
   dictionary would close this (Section 4.5).
3. **APR hard-codes 365.25** (`handlers.go:328`) for `FullTermAPR`.
   DOS reads the screen's basis setting; a 360-basis mortgage shows
   APR drifted by ~0.07%. Not a dispatch gap but a small numerical
   parity issue.
4. **`CompareAPRs` skips `EnoughDataForAPR` precondition.** DOS gates
   the iteration at `Mortgage.pas:633`; Go calls `FullTermAPR`
   unconditionally and swallows the error (`mortgage.go:478–479`).
   If `e2` is essentially empty, the iteration churns 40 cycles
   before declaring non-convergence. Effort: **S**.
5. **Missing `TryBalloonDates` fallback** in the crossover iteration
   (`Mortgage.pas:462–508`). DOS retries pinned to a balloon date
   when the main Newton fails. Balloon-vs-balloon comparisons can
   currently return "did not converge" when DOS would have found a
   discontinuous crossover. Effort: **M**.

### 1.6 Side-by-side examples (Mortgage)

```jsonc
// M1 forward — Help Example 1; expect Monthly ≈ $1,540.97
POST /api/mortgage/calc
{ "price": 200000, "points": 0.02, "pctDown": 0.20,
  "years": 20, "rate": 0.08, "tax": 200 }

// M4 reverse — Help Example 2; expect Price ≈ $241,233.69
POST /api/mortgage/calc
{ "cash": 56000, "points": 0.015, "monthly": 1650,
  "years": 30, "rate": 0.085, "tax": 200 }

// M7 unknown balloon
POST /api/mortgage/calc
{ "price": 200000, "pctDown": 0.20, "years": 30, "rate": 0.06,
  "monthly": 1500, "balloonYears": 10 }

// M8 over-determined → expect: "leave price or monthly payment or
// balloon amount blank to be computed"
POST /api/mortgage/calc
{ "price": 200000, "pctDown": 0.20, "years": 30, "rate": 0.06,
  "monthly": 1500, "balloonYears": 10, "balloonAmount": 50000 }
```

---

## 2. Amortization screen

### 2.1 State summary

**This is the worst-affected worksheet.** Of the 10 main DOS dispatch arms,
only **4 are reachable end-to-end**. Three additional solvers
(`SolvePayment`, `SolveLoanAmount`, `SolveRate`) exist as engine-level Go
functions with passing unit tests, but **`HandleAmortizationCalc`
(`handlers.go:340`) never invokes the latter two** — it hard-stamps
`AmountStatus = InOutInput` and `LoanRateStatus = InOutInput` regardless
of what the JSON contained. The DOS-style "leave a field blank and we'll
solve for it" never reaches them.

Verified at `handlers.go:361`: `if req.Amount == 0 && req.Rate == 0` —
this is the only path that branches on missing fields, and it triggers
only when *both* Amount and Rate are zero (derive-only mode). A request
with `{rate: 0, amount: 200000, ...}` runs through `Amortize` with a
hard-stamped `LoanRate = 0.0` and produces a zero-interest schedule with
no error and no warning. This is the highest-severity issue in the
audit.

The advanced options are similarly partial: `target` and `moratorium`
are fully supported, but `unkpre` (unknown prepayment amount), unknown
prepayment duration, target-balloon (solve balloon amount for target
balance), and per-adjustment rate↔payment solving are all missing.
The prepayment `nPmts` field is accepted in the API and silently ignored
by the engine.

### 2.2 Permutation matrix — basic grid

Columns: **Amt · Rate · First · Last · N · Pmt · Pts**. "✓" = filled,
"—" = blank.

| # | Amt | Rate | First | Last | N | Pmt | Pts | Solves | DOS site | Go site | Status |
|---|---|---|---|---|---|---|---|---|---|---|---|
| A1 | ✓ | ✓ | ✓/— | — | ✓ | ✓ | — | Forward schedule | `Amortize.pas:218–239` | `firstpass.go:43–99`, `engine.go:127–227` | ✅ |
| A2 | ✓ | ✓ | ✓ | ✓ | — | ✓ | — | Derive N from dates | `Amortize.pas:227–239` | `firstpass.go:75–92`          | ✅ |
| A3 | ✓ | ✓ | ✓ | — | ✓ | — | — | Payment (closed-form annuity) | `Amortize.pas:377–430` | `backward.go:70–103` + `engine.go:178–184` | ✅ (via engine fallthrough) |
| A4 | — | ✓ | ✓ | ✓/N | — | ✓ | — | **Loan amount** | `Amortize.pas:432–465, 853–858` | `backward.go:121–176` (exists) | ⚠ **unreachable** — handler never calls `SolveLoanAmount` |
| A5 | ✓ | — | ✓ | ✓/N | — | ✓ | — | **Rate** | `Amortize.pas:467–491` | `backward.go:194–238` (exists) | ❌ **unreachable** — handler runs 0% schedule silently |
| A6 | ✓ | ✓ | ✓ | — | — | ✓ | — | N or LastDate from payment | `AMORTOP.pas:1323–1407` | none | ❌ missing |
| A7 | — | — | ✓ | ✓ | ✓ or — | — | — | Derive-only term | `Amortize.pas:218–245` | `handlers.go:607–677` | ✅ |
| A8 | — | — | ✓ | — | ✓ | — | — | Derive-only last date | `Amortize.pas:220–226` | `firstpass.go:61–74` | ✅ |
| A9 | ✓ | ✓ | — | — | ✓ | ✓ | ✓ | Schedule + **APR** | `Amortize.pas:516–615` | none | ❌ missing |
| A10 | ✓ | ✓ | ✓ | — | ✓ | ✓ | — | Auto-append residual balloon | `Amortize.pas:1386–1394` | none | ⚠ residual visible but not flagged |

### 2.3 Permutation matrix — advanced options

| # | Block | Inputs | Solves | DOS site | Go site | Status |
|---|---|---|---|---|---|---|
| AO1 | Balloon | date ✓, amount ✓ | Apply at date | `AMORTOP.pas:574–664` | `engine.go:582–599` | ✅ |
| AO2 | Balloon | date ✓, amount — | **Balloon amount (target balloon)** | `Amortize.pas:628–663` | none | ❌ missing |
| AO3 | Balloon | date —, amount ✓ | DOS error "balloon needs date" | `AMORTOP.pas:293–300` | row silently dropped at `index.html:1981` | ⚠ silent |
| AO4 | Adjustment | date ✓, rate ✓, amount ✓ | ARM: rate AND payment | `AMORTOP.pas:1499–1613` | `engine.go:680–695` | ✅ |
| AO5 | Adjustment | date ✓, rate ✓, amount — | New payment from new rate | `Amortize.pas:1408–1413` | none — silent wrong | ❌ |
| AO6 | Adjustment | date ✓, rate —, amount ✓ | New rate from new payment | `Amortize.pas:1415–1418` | none — silent wrong | ❌ |
| AO7 | Adjustment | date ✓, rate —, amount — | Re-amortize at current rate | `AMORTOP.pas:1499+` | none | ❌ |
| AO8 | Prepayment | start ✓, perYr ✓, amount ✓, stop ✓ or nPmts ✓ | Apply series | `AMORTOP.pas:400–475` | `engine.go:601–632` | ⚠ partial — honors `stopDate` only; `nPmts` silently ignored |
| AO9 | Prepayment | start ✓, perYr ✓, stop ✓ | **Prepayment amount (`unkpre`)** | `Amortize.pas:665–707` | none | ❌ missing |
| AO10 | Prepayment | start ✓, perYr ✓, amount ✓ | **Prepayment duration (stopDate)** | `Amortize.pas:709–774` | none | ❌ missing |
| AO11 | Moratorium | date ✓ | Re-amortize after interest-only | `Amortize.pas:1260–1288` | `engine.go:565–580` | ✅ |
| AO12 | Target | amount ✓ | Min principal reduction | `Amortize.pas:288–292` | `engine.go:635–639` | ✅ |
| AO13 | Target+SkipMonths | both ✓ | Target overrides skip (DOS quirk) | `AMORTOP.pas:643` | `engine.go:549, 635` | ✅ (documented in `CLAUDE.md`) |
| AO14 | Balance lookup | date ✓ | Balance at arbitrary date | `Amortize.pas:1423–1425` | none (frontend computes from schedule, wrong with balloons) | ⚠ |

### 2.4 Solvable-but-not-wired (Amortization)

The highest-impact items in the entire audit are here.

1. **Wire `SolveRate` into `HandleAmortizationCalc`.** Effort: **S**.
   Change `AmortizationRequest.Rate` to `*float64`. In the handler,
   when `req.Rate == nil` AND `req.Amount != nil` AND
   `(req.NPeriods > 0 || req.LastDate != "")` AND `req.Payment != nil`,
   call `amortization.SolveRate(input)` and stamp the result back into
   `loan.LoanRate`. Eliminates the most dangerous silent-wrong-answer
   path (Section 4.1, S-1).
2. **Wire `SolveLoanAmount` into `HandleAmortizationCalc`.** Effort:
   **S**. Same shape: `AmortizationRequest.Amount` → `*float64`,
   dispatch when absent. The math at `backward.go:121` is unit-tested
   (`backward_test.go:60`, `crosscheck_backward_test.go:74`).
3. **Honor `Prepayment.NPmts`** in `engine.go:601–632`. Add a counter
   per series; once it reaches `pp.NN`, stop applying. Today the
   engine consults only `stopDate`. Effort: **S**.
4. **Return explicit errors for AO3 / AO9 silent drops** at
   `index.html:1965, 1979, 1997` (prepayments, balloons,
   adjustments). Today these dropped rows are invisible — the user
   submits an Advanced Options entry and sees no effect on the schedule
   with no warning. Effort: **S**.

### 2.5 Genuinely missing — DOS procedures to port

These need real porting work, not just plumbing.

5. **A6 — Closed-form N from payment.** `DetermineLastPaymentDate`
   at `AMORTOP.pas:1323–1407`. Today `firstpass.go:75–99` only derives
   N when *both* firstDate and lastDate are given; never from a
   payment. Effort: **M**.
6. **AO5 / AO6 — Per-adjustment solving.** The ARM modeling path is
   incomplete: an adjustment with only a new rate produces a schedule
   where the payment doesn't amortize cleanly (the rate updates but
   `d` stays at the previous payment), with no error. DOS solves the
   missing field per adjustment row. `Amortize.pas:1408–1418`. Effort: **M**.
7. **A9 — APR with points.** `EstimateAndRefineAPRwithPoints` at
   `Amortize.pas:516–615`. The `Loan.Points` field is declared in
   `types.go:55–58` but never read; `AmortResult.APR` is never written.
   Refactor `mortgage.IterateToFindAPR` into a shared helper. Effort: **M**.
8. **AO2 — Target balloon.** `EstimateAndRefineBalloon` at
   `Amortize.pas:628–663`. Solve the balloon amount such that
   schedule balance hits zero at the balloon date. Effort: **L** — has
   a closed-form `very_last` short-circuit plus Newton iteration for
   the general case; needs an "unknown" sentinel in `AmortBalloonReq`.
9. **AO9 — Unknown prepayment amount (`unkpre`).** Already flagged
   in `CLAUDE.md` outstanding items. `EstimateAndRefinePeriodicPrepayment`
   at `Amortize.pas:665–707`. Closed-form zero-rate and non-zero-rate
   initial guesses, then Iterate. Effort: **L**.
10. **AO10 — Unknown prepayment duration.** `DeterminePrepaymentDuration`
    at `Amortize.pas:709–774`. Effort: **L** (precondition on
    `plus_regular` semantics).
11. **AO14 — Balance lookup as engine path.** Today
    `updatePayoffBalance` in the frontend (`index.html:2108–2122`)
    walks the returned schedule and subtracts `(payment - interest)`
    — wrong when there are balloons or adjustments mid-period.
    Port DOS's `ComputeBalanceFromDate` / `ComputeDateFromBalance`
    (`Amortize.pas:1423–1425`). Effort: **M**.

### 2.6 Genuinely under-determined (Amortization)

- (Amount —, Rate —, Payment —) with no dates: DOS errors via
  `SufficientDataOnScreen` (`Amortize.pas:867–868`). Frontend already
  catches this at `index.html:1916–1937` with a specific message.
- (Amount ✓, Payment —, Rate —): two unknowns. DOS rejects. Go's
  engine ports the FirstPass rejection but the handler may not reach
  it (depends on which fields are zero vs blank).
- Two adjustments on same date: validated at `validate.go:65–72`.

### 2.7 Side-by-side examples (Amortization)

```jsonc
// A3 (works) — solve for monthly payment
POST /api/amortization/calc
{ "amount": 200000, "rate": 0.06, "loanDate": "2025-01-01",
  "nPeriods": 360, "perYr": 12 }
// → returns payment ≈ 1199.10, full schedule

// A4 (currently rejected) — solve for loan amount from a payment
POST /api/amortization/calc
{ "rate": 0.06, "loanDate": "2025-01-01", "perYr": 12,
  "nPeriods": 360, "payment": 1199.10 }
// → CURRENT: "insufficient loan data: need amount and payments per year"
// → DOS: would solve amount ≈ 200000 via ComputeLoanAmount

// A5 (currently silent wrong) — solve for rate
POST /api/amortization/calc
{ "amount": 200000, "loanDate": "2025-01-01", "perYr": 12,
  "nPeriods": 360, "payment": 1199.10 }
// → CURRENT: returns a 0%-rate schedule with no error
// → DOS: would solve rate ≈ 0.06 via EstimateAndRefineRate

// AO9 (currently dropped silently) — unknown prepayment amount
POST /api/amortization/calc
{ "amount": 200000, "rate": 0.06, "loanDate": "2025-01-01",
  "perYr": 12, "nPeriods": 360,
  "prepayments": [{ "startDate": "2026-01-01",
                    "stopDate":  "2030-01-01", "perYr": 12 }] }
// → CURRENT: row dropped at index.html:1965 (no amount); schedule runs
//   without any prepayments
// → DOS: would solve the prepayment amount that retires the loan at
//   the given stopDate via EstimateAndRefinePeriodicPrepayment
```

---

## 3. Present Value screen

### 3.1 State summary

PV is in the best shape of the three. All seven main DOS backward solve
paths (PV-1, PV-2, PV-4, PV-5, PV-6, PV-8, PV-9) are ported, and so are
the row-level over-determined detection and the screen-level "too many
unknowns" / "insufficient data" errors. The Newton-iteration constants
(`teeny=1e-10`, max-30 iterations, ±0.04 damping, second-pass restart
from 0) all match DOS exactly.

The remaining real gaps are:

- **Actuarial integration is missing from every backward path.** DOS
  divides residuals by `LifeProb` (`PRESVALU.pas:873–883` for PV-1)
  and rejects PV-2 outright under `fold_in_life`
  (`PRESVALU.pas:894–897`). The Go ports of `solveLumpAmount`,
  `solveLumpDate`, and `computeKnownRowSum` skip life-probability
  weighting entirely. A contingent payment row therefore produces
  the wrong residual.
- **The `V_3` `const_signal` block in PV-6** (`PRESVALU.pas:1003–1008`)
  is not yet wired because the `const_signal` status doesn't propagate
  through Go's status enum. Already documented in `CLAUDE.md` outstanding items.
- **The rate-type selector (True/Loan/Yield) is a cosmetic dropdown.**
  `onPVRateTypeChange()` at `index.html:2243` is a no-op; the API
  receives a single continuously-compounded rate regardless of which
  option the user selects. DOS calls `YieldRateTranslation`
  (`PRESVALU.pas:535`) on entry, so this also affects classification.
- **PV-8 and PV-9 are unreachable from the UI.** The backend
  `solveRate` (`backward.go:848`) and `solveAsOf` (`backward.go:916`)
  work end-to-end, but the frontend refuses to submit a blank rate
  (`index.html:2381–2384`, with a stale message: "IRR computation
  (blank rate) is not yet supported in the API. Please enter a rate")
  and `getPVInput` requires `asOfDate` (`index.html:2253`).

### 3.2 Permutation matrix (Present Value)

| # | Inputs | Solves | DOS site | Go site | Status |
|---|---|---|---|---|---|
| PV-1 | lump date, value, rate, asOf | lump **amount** | `PRESVALU.pas:866–891` | `backward.go:448` | ⚠ LifeProb divide missing |
| PV-2 | lump amount, value, rate, asOf | lump **date** | `PRESVALU.pas:892–931` | `backward.go:492` | ⚠ no `fold_in_life` reject |
| PV-3 | lump value only (no date, no amount) | **error** | `PRESVALU.pas:932–935` | `backward.go:286–290` | ✅ |
| PV-4 | periodic dates+value, rate, asOf | periodic **amount** | `PRESVALU.pas:943–956` | `backward.go:569` | ✅ |
| PV-5 | periodic fromDate, amount, value | periodic **toDate** | `PRESVALU.pas:965–999` | `backward.go:662–691` | ✅ (incl. cola=rate AddNPeriods branch) |
| PV-6 | periodic toDate, amount, value, cola=0 | **fromDate** | `PRESVALU.pas:1009–1027` | `backward.go:693–736` | ⚠ cola=0 path ported; cola≠0 two-pass approximation TODO |
| PV-6b | periodic, cola=const_signal | (special-case path) | `PRESVALU.pas:1003–1008` (`{$ifdef V_3}`) | none | ❌ missing |
| PV-7 | periodic with one date + value only | **error** | `PRESVALU.pas:1080–1083` | `backward.go:316–319` | ✅ |
| PV-8 | rows fully specified; blank rate; sumValue | discount **rate** | `PRESVALU.pas:693–754` | `backward.go:848` | ⚠ unreachable from UI |
| PV-9 | rows fully specified; rate; blank asOf; sumValue | **asOf date** | `PRESVALU.pas:755–818` | `backward.go:916` | ⚠ unreachable from UI |
| PV-10 | rows fully specified; rate; asOf | **sumValue** (forward) | `PRESVALU.pas:669–692` | `calc.go:296 forwardOnly` | ✅ |
| PV-screen | "too many unknowns" | screen-level error | `PRESVALU.pas:1242` | `calc.go:278–280` | ✅ |
| PV-warning | row determined by data above | DOS shows cancelable warning | `PRESVALU.pas:1166–1189` | none | ❌ missing (needs Warnings channel) |

### 3.3 Solvable-but-not-wired (Present Value)

1. **Delete the stale PV-8 guard** at `index.html:2381–2384`. The
   backend supports `solveRate`. The message "IRR computation (blank
   rate) is not yet supported in the API" actively misleads the user.
   Effort: **S**.
2. **Relax the `getPVInput` `asOfDate` requirement** at
   `index.html:2253` so PV-9 is reachable. Effort: **S**.
3. **Wire the rate-type selector** in `onPVRateTypeChange` — convert
   client-side to continuous TrueRate before posting (reuse the
   algebra at `index.html:2346–2358`). Closes
   `discrepancies.md §4`. Effort: **S**.

### 3.4 Genuinely missing — DOS procedures to port

4. **Actuarial integration in backward paths.** The most numerically
   significant gap. Today `solveLumpAmount`, `solveLumpDate`, and
   `computeKnownRowSum` ignore `input.Actuarial`. Plumb life-
   probability weighting through each path; add the `no_time_with_life`
   rejection at the entry of `solveLumpDate`. Effort: **M**.
5. **PV-6 cola≠0 two-pass approximation** (`PRESVALU.pas:1030–1035`).
   TODO already flagged at `backward.go:625–628`. Effort: **M**.
6. **`const_signal` PV-6 special case.** Requires extending the
   status enum to carry a "constant cola" flag through the row
   classifier. Effort: **M**.
7. **Warnings channel for "value already determined" PV warning.**
   The DOS UX is a cancelable dialog; the Go port currently can only
   hard-error or accept silently. Add `PVResponse.Warnings []string`.
   Effort: **M**.

### 3.5 Side-by-side examples (Present Value)

```jsonc
// PV-1 — solve for lump amount given target value
POST /api/presentvalue/calc
{ "rate": 0.05, "asOfDate": "2024-01-01", "sumValue": 9523.81,
  "lumpSums": [{ "date": "2025-01-01" }] }
// → returns amount ≈ 10000

// PV-8 (currently unreachable from UI) — solve for IRR
POST /api/presentvalue/calc
{ "asOfDate": "2024-01-01", "sumValue": 9000,
  "lumpSums": [{ "date": "2025-01-01", "amount": 10000 }] }
// → CURRENT (via API): solves rate ≈ 0.111
// → CURRENT (via UI): refused with "IRR computation (blank rate) is
//   not yet supported in the API"

// PV with actuarial (currently wrong)
POST /api/presentvalue/calc
{ "rate": 0.05, "asOfDate": "2024-01-01", "sumValue": 9523.81,
  "lumpSums": [{ "date": "2025-01-01", "act": "L" }],
  "actuarial": { /* life table */ } }
// → CURRENT: returns wrong amount (no LifeProb divide)
// → DOS: amount ≈ 10000 / LifeProb(2025-01-01, "Living")
```

---

## 4. Error-message audit (cross-cutting)

### 4.1 Silent-failure paths (highest priority)

These are the cases where the user submits an input that produces wrong
or empty output with **no error message**. They are the most dangerous
items in the audit because the user has no way to know something went
wrong.

| # | Trigger | Current behavior | Fix |
|---|---|---|---|
| **S-1** | `POST /api/amortization/calc` with `rate: 0` (or `rate` omitted, since `float64` zero-value is 0) and other fields filled | Engine runs a **0%-rate schedule** silently. No error. | Switch `AmortizationRequest.Rate` to `*float64`; dispatch to `SolveRate` when nil. |
| **S-2** | `POST /api/amortization/calc` with `amount: 0` and other fields filled | Engine returns `"insufficient loan data: need amount and payments per year"` — confusing because PerYr *is* supplied. Or runs through with garbage if other status flags align. | Switch `.Amount` to `*float64`; dispatch to `SolveLoanAmount` when nil. |
| **S-3** | Amortization advanced row with a missing required field (e.g. prepayment row with amount but no startDate) | Frontend at `index.html:1965, 1979, 1997` returns from the loop callback — row silently dropped from the request. | Return explicit "row N: missing X" error before submitting. |
| **S-4** | PV row half-filled (e.g. date but no amount on a lump-sum row) | Frontend at `index.html:2264, 2283` filters the row out before posting. | Same — show "row N: missing X" instead. |
| **S-5** | PV submit with all rows silently dropped per S-4 | Frontend then says "Enter ... at least one lump sum or periodic payment" — but the user *did* enter rows. | Cascade from S-4 fix. |
| **S-6** | Mortgage What-If generates rows where the source row had `hasFunding=false`, etc. | The API returns the request unchanged (no error, no computed cells) and the generated table looks half-empty. | Detect "no field populated as output" in `updateMtgRowUI` and warn. The frontend already pre-validates *for* What-If; extend the check to plain Calculate. |
| **S-7** | Mortgage Calculate with the row's main guard failing (e.g. no funding column) | `mortgage.Calc` returns success with no computed fields. Handler returns the input echoed back with zeros. The UI cannot distinguish "engine had nothing to do" from "engine cleared output." | Detect "no output column transitioned to InOutOutput" post-Calc; return "insufficient inputs" error. |

### 4.2 Error-message catalog summary

Full per-message catalog with proposed rewordings is in Appendix A.
Headline counts across all three worksheets:

- **62 distinct user-facing error/validation strings.**
- **17 (27%) clear** — identify the field and the fix.
- **28 (45%) vague** — identify a problem but not which field.
- **17 (28%) ambiguous** — user cannot infer what to change.

**Distribution by worksheet:** Mortgage 13, Amortization 27, PV 22.

The two most common patterns:

1. **Date-parsing errors that don't name which date field failed.**
   `handlers.go:368` says "invalid loanDate format" (good); but
   `handlers.go:459` says just "invalid prepayment startDate" with
   no row index. Trivial fix.
2. **Engine messages using internal Pascal-style names.**
   "summation factor too small," `"rate" computation did not converge`,
   etc. These leak solver internals and don't suggest a fix.

### 4.3 Structural recommendation — `FieldError` struct

Today engines return `error` (single string). Handlers wrap or
concatenate. The frontend either prefixes "Row N:" or regex-matches the
text (`explainMtgError`). Replace with:

```go
type FieldError struct {
    Code    string   // stable identifier (e.g. "MTG_OVERDETERMINED")
    Message string   // human-readable, field-named, UI-label format
    Fields  []string // affected fields, in UI-label form
    RowIdx  int      // 1-based; 0 = screen-level
    Block   string   // "mortgage", "lumpsum", "periodic", "prepayment", ...
}
```

This single change unlocks:

- Stable error codes for the frontend to dispatch on (vs. regex on
  message text).
- Per-cell highlighting in the UI.
- Localization-ready messages.
- A trivial replacement for `explainMtgError`'s regex.

Effort: **M** (touches all three handlers, all engine error returns,
and the frontend display layer).

### 4.4 Structural recommendation — `Warnings []FieldError` channel

Some DOS messages are **non-blocking warnings** the user can dismiss
("value already determined by data above — continue anyway?"). Today
the Go port has only one channel (`result.Err`), so these become
hard errors or are dropped silently. Add a parallel `Warnings []FieldError`
field on every response.

Use it for:

- PV "row determined by data above" (`PRESVALU.pas:1166–1189`).
- "Row dropped because field X was blank" (S-3, S-4).
- Solver clamps and assumptions ("Rate clamped to 0.02 for solver start").
- Format coercions ("Switched to 365-day basis for weekly payments,"
  `Amortize.pas:300`, currently silent).
- "Terminating balloon was adjusted to clear residual"
  (`Amortize.pas:1073`, currently silent).

Effort: **M**.

### 4.5 Structural recommendation — field-name dictionary

Engine code uses internal names ("monthly", "amount", "peryr"); the
UI uses human labels ("Monthly Total", "Amt Borrowed", "Pmts/Yr"). The
disconnect propagates into every error message. Centralize in
`internal/api/labels.go`:

| Engine term | UI label |
|---|---|
| amount, loan amount | Amount Borrowed |
| monthly | Monthly Total |
| peryr / payments per year | Pmts/Yr |
| nperiods, npayments | # Periods |
| firstdate, first payment date | 1st Pmt Date |
| lastdate | Last Pmt Date |
| loandate | Loan Date |
| payamt, payment | Pmt Amount |
| asof | As-of Date |
| sumvalue | Sum Value |
| fromdate / todate | From Date / To Date |
| pct down, percent down | % Down |
| howmuch, balloon amount | Balloon Amt |
| when, balloon years | Balloon Yrs |

Engines should look up labels at error-construction time, not embed
internal names. Effort: **S** for the dictionary; **M** to thread it
through.

### 4.6 Date-format consistency

Engine messages say "use YYYY-MM-DD." Frontend accepts and displays
"MM/DD/YYYY." The frontend pre-parses date strings before submitting,
so the engine messages are only seen by users hitting the API
directly — but those users also read the frontend help, which uses
MM/DD/YYYY. Standardize on "MM/DD/YYYY (or ISO YYYY-MM-DD)" in all
error messages. Effort: **S**.

### 4.7 Quick reword wins (top 20 ambiguous messages)

These are the highest-traffic ambiguous messages. Each is a one-line
edit.

| Current | Proposed |
|---|---|
| `summation too small` (`mortgage.go:246`) | `Rate is effectively zero — Monthly Total cannot be computed without a positive rate.` |
| `cash too close to price` (`mortgage.go:297`) | `Cash Required is within 0.5% of Price — leave Cash Required blank or lower it.` |
| `financed amount too close to price` (`mortgage.go:305`) | `Amt Borrowed is within 0.5% of Price — leave it blank or lower it.` |
| `invalid loanDate format, use YYYY-MM-DD` (`handlers.go:368`) | `Loan Date is unparseable — use MM/DD/YYYY.` |
| `invalid prepayment startDate` (`handlers.go:459`) | `Prepayment row N: Start Date is unparseable — use MM/DD/YYYY.` |
| `invalid balloon date` (`handlers.go:491`) | `Balloon row N: Date is unparseable — use MM/DD/YYYY.` |
| `insufficient loan data: need amount and payments per year` (`engine.go:133`) | `Amount Borrowed and Pmts/Yr are both required.` (split into two messages) |
| `insufficient loan data: need first payment date` (`engine.go:156`) | `1st Pmt Date is required (or fill Loan Date + Pmts/Yr to default it).` |
| `compute true rate: <inner>` (`engine.go:172`) | `Rate is out of range.` |
| `amortization exceeded 10000 periods` (`engine.go:532`) | `Schedule exceeded 10,000 periods — check Pmts/Yr and Last Pmt Date.` |
| `insufficient data: need amount, rate, term, peryr` (`backward.go:75`) | `Cannot solve Pmt Amount — Amount, Rate, # Periods, Pmts/Yr are required.` |
| `cannot determine payment - summation factor too small` (`backward.go:100`) | `Rate × Term is too small to solve for Pmt Amount.` |
| `cannot determine loan amount - interest rate too small` (`backward.go:140`) | `Rate is too small to solve for Amount Borrowed (effectively zero).` |
| `insufficient data: need rate, payment, term, peryr, first date` (`backward.go:126`) | `Cannot solve Amount Borrowed — Rate, Pmt Amount, # Periods, Pmts/Yr, 1st Pmt Date are required.` |
| `insufficient data: need amount, payment, term, peryr` (`backward.go:198`) | `Cannot solve Rate — Amount, Pmt Amount, # Periods, Pmts/Yr are required.` |
| `too many unknowns` (PV `calc.go:279`) | `More than one missing field on the screen — fill in enough cells to leave exactly one blank.` |
| `insufficient data on screen` (PV `calc.go:285`) | `Not enough inputs to solve for Sum Value — supply Rate, As-of Date, and at least one fully-specified row.` |
| `"rate" computation did not converge` (PV `backward.go:863`) | `Rate solver did not converge — try a different starting point or specify Amounts instead of Values.` |
| `cannot compute date - interest rate too small` (PV `backward.go:922`) | `Rate is effectively zero — cannot solve for As-of Date.` |
| `IRR computation (blank rate) is not yet supported in the API. Please enter a rate.` (`index.html:2382`) | **Delete this guard entirely** — the API supports it. Either remove the message, or replace with: `Solving for Rate requires entering Sum Value (target present value). Leave Rate blank but fill Sum Value.` |

---

## 5. Recommended sequencing

Prioritized by impact × inverse effort. Items with the same priority can
be parallelized.

### Phase 1 — Eliminate silent wrong answers (P0, ~1 week total)

1. **(S)** Make `AmortizationRequest.Rate` and `.Amount` pointer types;
   dispatch nil to `SolveRate` / `SolveLoanAmount`. Closes S-1 and S-2
   silent-failure paths. *Wins the most user trust per hour spent.*
2. **(S)** Delete the stale PV-8 IRR guard at `index.html:2381–2384`.
3. **(S)** Honor `Prepayment.NPmts` in `engine.go:601–632`.
4. **(S)** Return explicit "row N: missing X" errors for the three
   silent-drop spots in Advanced Options (`index.html:1965, 1979,
   1997`) and the two PV row filters (`:2264, :2283`).

### Phase 2 — Wire existing math to the API (P0, ~1 week)

5. **(S)** Add `POST /api/mortgage/compare` → `CompareAPRs`. Replace
   the local frontend heuristic.
6. **(S)** Add `POST /api/mortgage/whatif` → `GenerateRows`. Replace
   the per-row JS loop.
7. **(S)** Wire the PV rate-type selector (convert client-side before
   posting).
8. **(S)** Pre-submission validation in `calcMortgageRow` (parallel
   to existing amortization pattern) — refuse with a named-field
   list instead of silently producing empty output.

### Phase 3 — Structural error handling (P1, ~1 week)

9. **(M)** Introduce `FieldError` struct + serialize through all
   three handlers.
10. **(M)** Introduce `Warnings []FieldError` channel.
11. **(S)** Centralize the field-name dictionary in
    `internal/api/labels.go`.
12. **(S)** Reword the top 20 ambiguous messages per §4.7.

### Phase 4 — Port missing DOS procedures (P1–P2, ~3–4 weeks)

13. **(M)** Closed-form N from payment (Amortization A6).
14. **(M)** Per-adjustment rate↔payment solving (Amortization AO5,
    AO6, AO7).
15. **(M)** APR with points (Amortization A9). Refactor
    `mortgage.IterateToFindAPR` into a shared helper first.
16. **(M)** Actuarial integration in PV backward paths (PV-1 LifeProb
    divide, PV-2 `no_time_with_life` reject,
    `computeKnownRowSum` weighting).
17. **(M)** PV-6 cola≠0 two-pass approximation.
18. **(M)** Balance-lookup engine path (Amortization AO14).
19. **(M)** `TryBalloonDates` crossover fallback (Mortgage).
20. **(L)** Target balloon (Amortization AO2).
21. **(L)** Unknown prepayment amount (Amortization AO9).
22. **(L)** Unknown prepayment duration (Amortization AO10).
23. **(M)** `const_signal` propagation for PV-6b.

### Phase 5 — UX polish (P2, ~1 week)

24. **(M)** 2- and 3-dimensional What-If row generation.
25. **(M)** PV "value already determined" cancelable warning.
26. **(S)** APR `yrdays` should honor screen basis, not hard-coded
    365.25 (`handlers.go:328`).
27. **(S)** `EnoughDataForAPR` precondition in `CompareAPRs`.

**Total estimated effort:** roughly 6–8 engineer-weeks for all 27 items.
Phases 1 and 2 alone (2 weeks) eliminate every silent-wrong-answer path
and double the number of reachable dispatch arms, with no new financial
math required.

---

## Appendix A — Full error-message catalog

For brevity this section lists only the messages flagged **vague** or
**ambiguous** in §4.2. The full catalog (including the 17 clear messages
left as-is) is available on request.

### A.1 Mortgage (`/api/mortgage/calc`)

| # | Severity | File:line | Current | Proposed |
|---|---|---|---|---|
| MM-1 | M | `mortgage.go:191` | `must specify years to balloon payment` | `Balloon Yrs is required when Balloon Amt is filled in.` |
| MM-2 | H | `mortgage.go:235` | `leave price or monthly payment or balloon amount blank to be computed` | `Row N has Price and Monthly Total both filled — leave one blank, or add Balloon Yrs to solve for the balloon.` |
| MM-3 | H | `mortgage.go:246` | `summation too small` | `Rate is effectively zero — Monthly Total cannot be computed without a positive rate.` |
| MM-4 | M | `mortgage.go:266` | `fill in percent down or cash required for price computation` | `To solve for Price from Monthly Total, also fill in % Down or Cash Required.` |
| MM-5 | H | `mortgage.go:286` | `price too small` | `Price must be greater than zero.` |
| MM-6 | H | `mortgage.go:297` | `cash too close to price` | `Cash Required is within 0.5% of Price — leave Cash Required blank or lower it (% Down cannot be solved).` |
| MM-7 | H | `mortgage.go:305` | `financed amount too close to price` | `Amt Borrowed is within 0.5% of Price — leave it blank or lower it.` |

### A.2 Amortization (`/api/amortization/calc`)

| # | Severity | File:line | Current | Proposed |
|---|---|---|---|---|
| AM-1 | H | `handlers.go:368` | `invalid loanDate format, use YYYY-MM-DD` | `Loan Date is unparseable — use MM/DD/YYYY.` |
| AM-2 | H | `handlers.go:428, 635` | `invalid firstDate format, use YYYY-MM-DD` | `1st Pmt Date is unparseable — use MM/DD/YYYY.` |
| AM-3 | H | `handlers.go:441, 644` | `invalid lastDate format, use YYYY-MM-DD` | `Last Pmt Date is unparseable — use MM/DD/YYYY.` |
| AM-4 | H | `handlers.go:459` | `invalid prepayment startDate` | `Prepayment row N: Start Date is unparseable — use MM/DD/YYYY.` |
| AM-5 | H | `handlers.go:478` | `invalid prepayment stopDate` | `Prepayment row N: Stop Date is unparseable — use MM/DD/YYYY.` |
| AM-6 | H | `handlers.go:491` | `invalid balloon date` | `Balloon row N: Date is unparseable — use MM/DD/YYYY.` |
| AM-7 | H | `handlers.go:506` | `invalid adjustment date` | `Adjustment row N: Date is unparseable — use MM/DD/YYYY.` |
| AM-8 | H | `handlers.go:529` | `invalid moratorium date` | `Moratorium Date is unparseable — use MM/DD/YYYY.` |
| AM-9 | M | `handlers.go:550` | `invalid skipMonths: <inner>` | `Skip Months: <inner>. Example: "6-8,12".` |
| AM-10 | H | `engine.go:133` | `insufficient loan data: need amount and payments per year` | Split into two: `Amount Borrowed is required.` and `Pmts/Yr is required.` |
| AM-11 | H | `engine.go:138` | `insufficient loan data: need loan date` | `Loan Date is required.` |
| AM-12 | H | `engine.go:156` | `insufficient loan data: need first payment date` | `1st Pmt Date is required (or fill Loan Date + Pmts/Yr to default it).` |
| AM-13 | M | `engine.go:172` | `compute true rate: <inner>` | `Rate is out of range (got <X>%).` |
| AM-14 | M | `validate.go:69` | `two rate adjustments on the same day (line N)` | `Rate Adjustment row N is on the same date as row N-1.` |
| AM-15 | H | `backward.go:75` | `insufficient data: need amount, rate, term, peryr` | `Cannot solve Pmt Amount — Amount, Rate, # Periods, Pmts/Yr are required.` |
| AM-16 | M | `backward.go:100` | `cannot determine payment - summation factor too small` | `Rate × Term is too small to solve for Pmt Amount.` |
| AM-17 | H | `backward.go:126` | `insufficient data: need rate, payment, term, peryr, first date` | `Cannot solve Amount Borrowed — Rate, Pmt Amount, # Periods, Pmts/Yr, 1st Pmt Date are required.` |
| AM-18 | H | `backward.go:198` | `insufficient data: need amount, payment, term, peryr` | `Cannot solve Rate — Amount, Pmt Amount, # Periods, Pmts/Yr are required.` |

### A.3 Present Value (`/api/presentvalue/calc`)

| # | Severity | File:line | Current | Proposed |
|---|---|---|---|---|
| PV-1 | H | `handlers.go:707` | `invalid asOfDate format` | `As-of Date is unparseable — use MM/DD/YYYY.` |
| PV-2 | H | `handlers.go:735` | `invalid lump sum date` | `Lump Sum row N: Date is unparseable.` |
| PV-3 | H | `handlers.go:759` | `invalid fromDate` | `Periodic row N: From Date is unparseable.` |
| PV-4 | H | `handlers.go:768` | `invalid toDate` | `Periodic row N: To Date is unparseable.` |
| PV-5 | H | `backward.go:287` | `specify either date or amount in single payments, line N` | `Lump Sum row N: also supply Date or Amount (Value alone is insufficient).` |
| PV-6 | H | `backward.go:316` | `specify either other date or amount in periodic payments, line N` | `Periodic row N: also supply the missing date or Amount.` |
| PV-7 | H | `calc.go:279` | `too many unknowns` | `More than one missing field on the screen — fill in enough cells to leave exactly one blank.` |
| PV-8 | H | `calc.go:285, backward.go:358` | `insufficient data on screen` | `Not enough inputs to solve for Sum Value — supply Rate, As-of Date, and at least one fully-specified row.` |
| PV-9 | H | `calc.go:301` | `need rate and as-of date for present value calculation` | `Rate and As-of Date are required for forward calculation.` |
| PV-10 | H | `backward.go:863` | `"rate" computation did not converge` | `Rate solver did not converge — try a different starting point or specify Amounts instead of Values.` |
| PV-11 | H | `backward.go:886` | `rate is not determined - specify amounts instead of values` | `Rate cannot be solved when all rows have Value but no Amount — fill at least one row's Amount.` |
| PV-12 | H | `backward.go:922` | `cannot compute date - interest rate too small` | `Rate is effectively zero — cannot solve for As-of Date.` |
| PV-13 | H | `backward.go:960` | `"as of" computation did not converge` | `As-of Date solver did not converge — try a different rate.` |
| PV-14 | H | `index.html:2382` | `IRR computation (blank rate) is not yet supported in the API. Please enter a rate.` | **Delete the guard entirely.** The backend has supported `solveRate` since the PV-8 port. |

---

## Appendix B — Verification methodology

The claims in this document were produced by four parallel research
passes (one per worksheet, one cross-cutting for errors) against the
DOS Pascal source under `legacy/src/dos_source/` and the current Go
port and frontend. The agents read full source files (not just
greps) and cite file:line throughout.

Five high-impact claims were spot-checked manually before publication:

| Claim | Verification | Result |
|---|---|---|
| `SolveLoanAmount` exists but handler never calls it | `grep -n` for `SolveLoanAmount` in `internal/api/handlers.go` | No matches. Confirmed unwired. |
| `SolveRate` exists but handler never calls it | Same | No matches. Confirmed unwired. |
| Amortization handler dispatches missing fields only when BOTH amount and rate are zero | Read `handlers.go:361` | `if req.Amount == 0 && req.Rate == 0` — confirmed. |
| `CompareAPRs` exists but no `/api/mortgage/compare` route | `grep -n CompareAPRs handlers.go` | No matches. Confirmed unwired. |
| Frontend PV-8 guard with stale message | Read `index.html:2381–2384` | `'IRR computation (blank rate) is not yet supported in the API. Please enter a rate.'` — confirmed verbatim. |

All five passed. The senior engineer reviewing this document should
feel free to spot-check additional claims via `grep` / `Read`; the
audit was performed with the assumption it would be challenged.
