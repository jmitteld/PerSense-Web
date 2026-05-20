# Per%Sense — Unit Test Plan

*Companion to `docs/dispatch_gaps.md`. Authored 2026-05-19 from a coverage
audit of every `*_test.go` file under `internal/` against the dispatch gaps
already cataloged.*

---

## Executive summary

Three audits converge on the same conclusion: **the test suite is shaped
around the happy path and provides almost no coverage for the silent-failure
modes that erode user trust.** Specifically:

- **Both critical Amortization silent-wrong-answer paths (S-1 rate=0, S-2
  amount=0) have zero canary tests at the API layer.** The engine-level
  `SolveRate` and `SolveLoanAmount` are well unit-tested at `backward.go:121`
  and `backward.go:194`, but no test posts `{rate: 0, ...}` or `{amount: 0, ...}`
  to `/api/amortization/calc` and asserts the response. The regression
  cannot fire from where it would actually catch users.
- **Of the 10 DOS-regression keys in `refdata.json`, only one covers a
  mortgage dispatch arm (M1 "Price + %Down → Monthly"),** and the four
  rows in that key are all variations of the same arm. Cross-checks at
  `crosscheck_backward_test.go` re-invert these same forward rows
  rather than loading independent DOS backward output — so the "DOS
  regression" guarantee is much weaker than the file name suggests.
- **Several Newton/iteration tail-arms are dead-letter-tested.** No
  current test reliably enters the PV `solveRate` second-pass restart
  (`backward.go:866`), the amortization `SolveRate` non-convergence
  return path (`backward.go:237`), the PV `solveAsOf` non-convergence
  arm (`backward.go:959`), or the PV-6 PUNT path
  (`backward.go:718`). These are exactly the edge cases most likely to
  regress silently during a refactor.

**Recommended test addition: ~95 new tests in three waves.**

| Wave | Purpose | Count | Effort |
|---|---|---:|---|
| 1. Canary tests | Document existing bugs; fail today, pass after fix | 18 | 1 day |
| 2. Paired-with-fix tests | Land with each dispatch_gaps.md item | 52 | spread across phases |
| 3. Coverage-gap tests | Cover untested branches in already-working code | 25 | 2 days |
| **Total** | | **~95** | |

Wave 1 alone is the highest-ROI batch: each test is a one-page demo
of "here is a real user input that produces wrong output; here is the
test that will go red until you fix it." They land in a single sitting
and give the client a tangible artifact for what the gap analysis
described.

---

## How to read this document

- **Test ID format.** `T-<phase>-<n>` for paired-with-fix tests
  (e.g. `T-P1-3` = Phase 1 test #3). `C-<n>` for canary tests
  (Wave 1). `G-<n>` for coverage-gap tests (Wave 3).
- **Layer.** *Engine* = `internal/finance/.../<x>_test.go`.
  *Handler* = `internal/api/<x>_test.go`. *Cross-check* =
  `internal/finance/crosscheck_*_test.go` against `refdata.json`.
- **DOS reference.** Where a test depends on running the
  Free-Pascal harness to produce a `refdata.json` entry, the test
  spec includes the harness invocation. Without DOS-generated
  reference values, the test can only validate internal
  consistency (round-trip), not DOS parity.
- **Effort hints.** Each test ranges roughly 30 minutes (a
  table-driven case added to an existing function) to 4 hours
  (a new test file that needs harness data + boundary thinking).
  Section 4 estimates roll up to whole-day numbers.

---

## 1. Wave 1 — Canary tests (the 18 to write first)

These tests **fail today**. Each documents a real bug or ambiguous
behavior from the dispatch-gaps analysis. When the corresponding fix
lands in Phase 1 or 2, the canary flips to green and becomes a regression
test. **Write all 18 in a single PR before any fix work begins.** That
PR is the cleanest possible client-facing demo of "here's exactly what's
broken, in code."

### Silent-failure canaries (the dangerous seven)

| ID | Bug | File:line for new test | Sample input | Expected (after fix) | Expected fail mode (today) |
|---|---|---|---|---|---|
| **C-1** | Amortization with `rate: 0` silently runs 0%-rate schedule (dispatch_gaps S-1) | `internal/api/amort_solve_rate_test.go` (new file) | `{amount: 200000, payment: 1199.10, nPeriods: 360, perYr: 12, loanDate: "2025-01-01"}` | Response has `apr ≈ 0.06`, schedule amortizes to ~0 | Asserts `result.LoanRate > 0.001` — today gets 0 |
| **C-2** | Amortization with `amount: 0` returns confusing error or silent zero schedule (dispatch_gaps S-2) | `internal/api/amort_solve_amount_test.go` (new file) | `{rate: 0.06, payment: 1199.10, nPeriods: 360, perYr: 12, loanDate: "2025-01-01"}` | Response has `amount ≈ 200000` | Asserts `result.Amount > 1000` — today gets the "insufficient loan data: need amount and payments per year" error |
| **C-3** | Frontend silently drops half-filled prepayment row (S-3) | `internal/api/amort_advanced_partial_test.go` (new file) | Post `{prepayments: [{startDate: "...", amount: null, perYr: 12}]}` | Returns explicit error "Prepayment row 1: Start Date + Amount + Pmts/Yr are all required" | Today: drops the row, returns schedule without prepayment, no error |
| **C-4** | Frontend silently drops half-filled balloon row (S-3) | same file | `{balloons: [{date: null, amount: 50000}]}` | Returns "Balloon row 1: Date is required" | Today: row dropped silently |
| **C-5** | Frontend silently drops half-filled adjustment row (S-3) | same file | `{adjustments: [{date: "...", rate: null, amount: null}]}` | Returns "Adjustment row 1: supply at least one of Rate or Amount" | Today: row dropped silently |
| **C-6** | PV with half-filled lump-sum row submitted (S-4) | `internal/api/pv_partial_rows_test.go` (new file) | `{lumpSums: [{date: "2025-01-01", amount: null}]}` after a valid row | Returns explicit row-2-incomplete error | Today: row 2 silently dropped before submit |
| **C-7** | PV submit produces "Enter at least one row" when user did enter rows (S-5 cascade) | same file | Two half-filled rows + one good row | Returns row-1/row-2 errors, processes row 3 | Today: all dropped, generic "Enter ... at least one" message |

### Wiring-stale canaries

| ID | Bug | File for new test | Assertion | Today's fail |
|---|---|---|---|---|
| **C-8** | `mortgage.CompareAPRs` exists but no HTTP route (dispatch_gaps §1.2 M14) | `internal/api/mortgage_compare_test.go` (new) | `POST /api/mortgage/compare` returns 200 with `crossoverYears` field | Today: 404 (route undefined) |
| **C-9** | `mortgage.GenerateRows` exists but no HTTP route (M15) | `internal/api/mortgage_whatif_test.go` (new) | `POST /api/mortgage/whatif` returns N row dicts | Today: 404 |
| **C-10** | Frontend blocks PV-8 with stale message (PV-14) — engine-level proof that backend supports it | `internal/api/pv_backward_test.go` (extend) | `POST /api/presentvalue/calc {asOfDate, sumValue:9000, lumpSums:[{date,amount:10000}], rate omitted}` returns `rate ≈ 0.111` | Today already passes (engine does support); this test exists to bind a contract so the frontend guard can be removed safely |
| **C-11** | Prepayment `nPmts` accepted by API but ignored by engine (AO8) | `internal/finance/amortization/prepayment_npmts_test.go` (new) | `Prepayment{StartDate, PerYr:12, Amt:200, NN:24, StopDateStatus: empty}` produces 24 extra payments | Today: runs forever / runs to natural stop |

### Ambiguous-error canaries

These bind the *current* wording so when §4.7 rewords land, you see exactly
which assertions need updating. Pair each with a comment marking it as
"reword-pending."

| ID | Path | File | Today's text being asserted | Reword target (will break this assertion) |
|---|---|---|---|---|
| **C-12** | Mortgage M2 overdetermined error | `mortgage_test.go` (extend `TestCalcValidationErrors`) | `err.Error() == "leave price or monthly payment or balloon amount blank to be computed"` | Phase 3 reword to row-naming form |
| **C-13** | Mortgage rate-too-small `summation too small` (mortgage.go:246) | `mortgage_test.go` (new test, `TestCalcRateNearZero`) | Asserts on `"summation too small"` substring | Phase 3 reword |
| **C-14** | Mortgage cash-too-close (mortgage.go:297) | `mortgage_test.go` (new, `TestCalcCashTooCloseToPrice`) | `Price=100000, Cash=99800, Points=0` → "cash too close to price" | Phase 3 reword |
| **C-15** | Mortgage financed-too-close (mortgage.go:305) | `mortgage_test.go` (new, `TestCalcFinancedTooCloseToPrice`) | `Price=100000, Financed=99800` → "financed amount too close to price" | Phase 3 reword |
| **C-16** | Amortization missing first-payment-date message | `amort_firstpass_test.go` (extend) | Asserts `"need first payment date"` | Phase 3 reword to `1st Pmt Date is required...` |
| **C-17** | PV "too many unknowns" actually reachable | `internal/finance/presentvalue/backward_test.go` (replace existing TestTooManyUnknowns) | New input: lump row {date, amount, value} all filled + extra row {amount blank} + sumValue. Should trigger `frontward AND backward` per `PRESVALU.pas:1242` | Today's `TestTooManyUnknowns` admits in a comment it cannot trigger the path |
| **C-18** | PV "insufficient data on screen" actually reachable | same file | Empty `lumpSums` and `periodics` with sumValue set. Should hit `calc.go:285` | No test reaches this today |

### Wave 1 deliverable estimate

Implementing all 18 takes about **6–8 hours** with Claude Code. The PR
should compile but have 18 red tests; CI configured to expect them
(skip-list with TODO references, or a separate `gaps_test.go` build tag).

---

## 2. Wave 2 — Tests paired with each gap fix

Organized by the same phase numbering as `dispatch_gaps.md §5`. Each fix
should land with the listed tests in the same PR — no merge without
a green path through them.

### Phase 1 — Eliminate silent wrong answers (10 tests)

| ID | Fix it pairs with | Test layer | What to assert |
|---|---|---|---|
| T-P1-1 | `AmortizationRequest.Rate` becomes `*float64`; SolveRate dispatched | Handler | Replaces C-1: rate-omitted input produces correct `LoanRate` and schedule |
| T-P1-2 | Same | Handler | `rate: 0.0` (explicitly typed zero) still produces zero-rate schedule — confirm the pointer-nil-vs-zero distinction works |
| T-P1-3 | Same | Engine | DOS-regression: pick 3 rows from a future `solve_rate` refdata key, assert Go output matches to 4 decimals |
| T-P1-4 | Same | Engine | Boundary: rate-from-payment for high rate (>30%) — `EstimateAndRefineRate` damping behavior |
| T-P1-5 | Same | Engine | Boundary: rate-from-payment for very long term (50 years) — convergence within 30 iterations |
| T-P1-6 | `AmortizationRequest.Amount` becomes `*float64`; SolveLoanAmount dispatched | Handler | Replaces C-2: amount-omitted produces correct `Amount` |
| T-P1-7 | Same | Handler | Both Amount and Rate omitted → still derive-only mode (regression guard for the existing `req.Amount == 0 && req.Rate == 0` branch) |
| T-P1-8 | Same | Engine | DOS-regression: 3 rows from `solve_loan_amount` refdata key |
| T-P1-9 | `Prepayment.NPmts` honored | Engine | Replaces C-11: `NN: 24, StopDateStatus: empty` → exactly 24 extra payments applied |
| T-P1-10 | Same | Engine | Edge: `NN: 24` AND `StopDate` set — DOS behavior: whichever expires first wins (verify against DOS source comment) |

### Phase 2 — Wire existing math to API (10 tests)

| ID | Fix | Layer | Assertion |
|---|---|---|---|
| T-P2-1 | `POST /api/mortgage/compare` | Handler | Replaces C-8; round-trip: post {A: knownRow1, B: knownRow2}, assert `crossoverYears` ≈ DOS-computed value from Mortgage.pas:613-711 |
| T-P2-2 | Same | Handler | "Always better" case — non-crossing APRs return `aIsBetter: true` (no crossover) |
| T-P2-3 | Same | Handler | `EnoughDataForAPR` precondition (dispatch_gaps §1.5.4) — incomplete `B` row returns explicit "insufficient data in mortgage B" error |
| T-P2-4 | `POST /api/mortgage/whatif` | Handler | Replaces C-9; varying Rate from 7% to 9% by 0.25 produces 9 rows |
| T-P2-5 | Same | Handler | `EnoughDataForRowGeneration` precondition — source row missing Price/Monthly/Financed returns explicit error |
| T-P2-6 | PV rate-type selector wired client-side | Frontend integration (Playwright/Cypress) or unit | Posting with `rateType: "loan"` produces same `sumValue` as `rateType: "true"` with the converted rate |
| T-P2-7 | PV frontend PV-8 guard removed | Frontend unit | Submitting blank rate with valid lump-sum row triggers POST, no longer blocked client-side |
| T-P2-8 | Mortgage `calcMortgageRow` pre-validation | Frontend unit | Row with no funding column shows named-field error message instead of silently producing empty output |
| T-P2-9 | Same | Frontend unit | Row with missing Years shows "Years is required" |
| T-P2-10 | Same | Frontend unit | Over-determined row shows "Leave one of Price or Monthly Total blank" with both fields named |

### Phase 3 — Structural error handling (8 tests)

| ID | Fix | Layer | Assertion |
|---|---|---|---|
| T-P3-1 | `FieldError` struct introduced | Handler | Existing M2 over-determined test: response now contains `{code: "MTG_OVERDETERMINED", fields: ["Price", "Monthly Total"], rowIdx: 1}` |
| T-P3-2 | Same | Handler | All three handlers emit `FieldError` instead of raw string; backwards-compat shim test (the `error` string is still populated for old clients) |
| T-P3-3 | `Warnings []FieldError` channel | Handler | PV "value already determined by data above" produces `warnings:[...]` and the calc still succeeds |
| T-P3-4 | Same | Handler | Amortization "365-day basis was assumed because PerYr=52" warning surfaces |
| T-P3-5 | Field-name dictionary | Handler | Spot-check: 5 messages from old engine code now use UI labels ("Monthly Total" not "monthly", etc.) |
| T-P3-6 | Top-20 message rewordings | Engine | Update each of the ~10 fragile assertions identified in audit §F (e.g. `advanced_test.go:199` `"true rate"` → `"Rate is out of range"`) |
| T-P3-7 | `explainMtgError` replaced by code dispatch | Frontend | Asserts the regex-matcher is gone; instead a `switch (data.code)` covers all current mortgage error codes |
| T-P3-8 | Date-format consistency | Handler | All date parse errors emit `MM/DD/YYYY` (or `ISO YYYY-MM-DD`), not just one |

### Phase 4 — Port missing DOS procedures (20 tests)

| ID | Fix | Layer | Assertion |
|---|---|---|---|
| T-P4-1 | Closed-form N from payment (A6) | Engine | `{amount, rate, payment, perYr, loanDate}` → correct `nPeriods` |
| T-P4-2 | Same | Engine | Boundary: `payment too small` rejection (DOS `AMORTOP.pas:1384`) |
| T-P4-3 | Same | Engine | DOS-regression: 3 rows from new `solve_n_from_payment` refdata key |
| T-P4-4 | Per-adjustment rate→pmt solving (AO5) | Engine | `{adjustments: [{date, rate: 0.075}]}` produces new payment that amortizes remaining balance |
| T-P4-5 | Per-adjustment pmt→rate solving (AO6) | Engine | `{adjustments: [{date, amount: 1800}]}` produces new rate that makes that payment amortize |
| T-P4-6 | Re-amortize at current rate (AO7) | Engine | `{adjustments: [{date}]}` recomputes payment, rate unchanged |
| T-P4-7 | All three adjustment cases together | Engine | Two ARM adjustments at different dates produce expected schedule |
| T-P4-8 | APR with points (A9) | Engine | `Loan.Points > 0` → `result.APR` populated; matches `mortgage.FullTermAPR` for equivalent inputs |
| T-P4-9 | Same | Engine | DOS-regression: refdata `amort_apr_with_points` |
| T-P4-10 | Auto-append residual balloon (A10) | Engine | Insufficient-payment schedule has a final-row balloon labeled as such |
| T-P4-11 | PV actuarial integration in PV-1 | Engine | `solveLumpAmount` with `Act: Living` divides by `LifeProb` per DOS `PRESVALU.pas:873-883` |
| T-P4-12 | PV-2 `no_time_with_life` reject | Engine | `solveLumpDate` with `Actuarial != nil && Act != NotContingent` returns DOS-matching error |
| T-P4-13 | PV `computeKnownRowSum` actuarial weighting | Engine | Multi-row PV with one contingent row produces correct residual for the backward solver |
| T-P4-14 | PV-6 cola≠0 two-pass approximation | Engine | `toDate, amount, value, cola=0.03` produces `fromDate` to ±1 day of DOS-computed value |
| T-P4-15 | Balance-lookup engine path (AO14) | Engine | New function `BalanceOn(date)` walks the schedule with balloons/adjustments and returns exact balance |
| T-P4-16 | Same | Handler | `POST /api/amortization/balance` returns 200 with correct balance |
| T-P4-17 | `TryBalloonDates` crossover fallback (Mortgage) | Engine | Balloon-vs-balloon comparison where main Newton diverges returns discontinuous crossover at balloon date |
| T-P4-18 | Target balloon (AO2) | Engine | `{balloons: [{date, amount: null}]}` solves for the amount that hits zero balance at that date |
| T-P4-19 | Unknown prepayment amount (AO9) | Engine | `{prepayments: [{startDate, stopDate, perYr, amount: null}]}` solves for amount |
| T-P4-20 | Unknown prepayment duration (AO10) | Engine | `{prepayments: [{startDate, perYr, amount, stopDate: null}]}` solves for stopDate; respects `plus_regular` precondition |

### Phase 5 — UX polish (4 tests)

| ID | Fix | Layer | Assertion |
|---|---|---|---|
| T-P5-1 | 2-D What-If | Engine | Varying Rate × Years produces N×M rows |
| T-P5-2 | 3-D What-If | Engine | Three-dimension variation; matches DOS `Mortgage.pas:906` |
| T-P5-3 | PV "value already determined" warning | Handler | Over-specifying produces calc result + warning, not error |
| T-P5-4 | APR uses screen basis (not hard-coded 365.25) | Handler | 360-basis mortgage's APR differs from 365.25 APR by expected amount |

---

## 3. Wave 3 — Coverage gaps in existing code (25 tests)

These cover code paths that **already work** but have no test. They are
the "silent regression risk" tests — change the surrounding code, and
nothing tells you it broke.

### Untested engine branches (per coverage audit §D)

| ID | Branch | File:line | Test sketch |
|---|---|---|---|
| G-1 | Mortgage M5 (Cash arm of Monthly→Price) | `mortgage.go:263` | Variant of `TestCalcComputePrice` with Cash + Monthly inputs |
| G-2 | Mortgage M6 silent-no-output guard | `mortgage.go:266` | Monthly + Financed only → error "fill in % Down or Cash Required" |
| G-3 | Mortgage `must specify years to balloon payment` | `mortgage.go:191` | `{balloonAmount: 50000}` alone returns the error |
| G-4 | Mortgage `summation too small` | `mortgage.go:246` | Rate ≈ 1e-12 with non-zero status → error (bind for C-13) |
| G-5 | Mortgage `price too small` | `mortgage.go:286` | `Price=1e-12, Pct=0.20` → error |
| G-6 | Mortgage cash-too-close-to-price 0.995 arm | `mortgage.go:297` | (binds for C-14) |
| G-7 | Mortgage financed-too-close 0.995 arm | `mortgage.go:305` | (binds for C-15) |
| G-8 | Mortgage `Financed > Price` validator | `mortgage.go:199` | `{Price: 100000, Financed: 120000}` → error |
| G-9 | `CompareAPRs` no-precondition (one mortgage empty) | `mortgage.go:478-479` | Confirm explicit error, not 40-cycle non-convergence |
| G-10 | Amortization `SolveRate` non-convergence return | `backward.go:237` | Construct input that hits 30 iterations without converging; assert `converged=false` |
| G-11 | Amortization `SolveRate` `delta < teeny` fallback | `backward.go:223-225` | Newton step underflow → uses `small` fallback |
| G-12 | Amortization simple `payNum > 10000` safety | `engine.go:531-533` | Construct a runaway-rate scenario in the simple (non-fancy) path |
| G-13 | Amortization adjustment `LoanRateStatus < InOutDefault && AmtOK` | `engine.go:691-693` | Amount-only adjustment (pairs with T-P4-5) |
| G-14 | Amortization moratorium `moratoriumActive == false` path | `engine.go:565-580` | FirstRepay ≤ FirstDate; moratorium effectively absent |
| G-15 | Amortization Case B prepaid (loan starts inside first period) | `engine.go:511-516` | Currently only asserted negatively |
| G-16 | Amortization balloon `cmp < 0` arm | `engine.go:585-587` | Balloon date between two pay periods |
| G-17 | PV `solveRate` second-pass restart from 0 | `backward.go:866-868` | Input where first pass fails but second pass succeeds — assert via instrumentation or by adding a counter |
| G-18 | PV `solveRate` `count==2 && diff==0` rate-not-determined | `backward.go:884-887` | All rows have Value but no Amount |
| G-19 | PV `solveLumpDate` sign-mismatch | `backward.go:514-517` | Amount > 0 with negative Value → error |
| G-20 | PV `solveLumpDate` `dval < teeny` divergence | `backward.go:538-540` | (binds boundary) |
| G-21 | PV `solvePeriodicDate` sign-mismatch | `backward.go:645-649` | Periodic Amount/Value sign disagree |
| G-22 | PV-6 PUNT path (`firstTerm <= 0`) | `backward.go:718-721` | cola > rate |
| G-23 | PV `solveAsOf` sum=0 | `backward.go:935-937` | Construct cash flows that sum to zero |
| G-24 | PV `solveAsOf` sign mismatch | `backward.go:940-943` | sumValue sign vs cash flow sign disagree |
| G-25 | PV `solveAsOf` exceeds maxdate | `backward.go:959-962` | Very small rate, very large sumValue |

### DOS-regression data extensions (run the Free Pascal harness)

The current `refdata.json` only regresses 10 functions, and the mortgage
calc key uses one dispatch arm. **Add the following keys** to
`legacy/testharness/refdata.pas`, regenerate, and write Go cross-checks:

| New refdata key | Purpose | Pairs with |
|---|---|---|
| `mortgage_calc_m2` through `m9` | One row each per Mortgage dispatch arm | Cross-check tests in `crosscheck_test.go` |
| `mortgage_apr` | Numerical APR values for 6 typical inputs | T-P4-9 |
| `compare_aprs` | Crossover year for 4 representative comparisons | T-P2-1 |
| `amort_simple` | Schedule rows for 3 simple loans | New `TestCrossCheckAmortSimple` |
| `amort_fancy` | Schedule rows with prepay/balloon/moratorium | New `TestCrossCheckAmortFancy` |
| `amort_solve_payment` | Closed-form payment for 6 inputs | T-P1-3 placeholder, plus standalone |
| `amort_solve_rate` | Newton-iterated rate for 6 inputs | T-P1-3 |
| `amort_solve_loan_amount` | 6 inputs | T-P1-8 |
| `pv_solve_lump_amount` | PV-1 with 3 rates × 3 dates = 9 cases | T-P4-11 actuarial variant |
| `pv_solve_rate` | PV-8 IRR for 4 cases | C-10 binds |
| `pv_actuarial_lump_amount` | PV-1 with `LifeProb` divide | T-P4-11 |

Effort: regenerating refdata.json after editing `refdata.pas` is
single-command; the Pascal source edits are mechanical. **~4 hours
total** for all 11 new keys.

---

## 4. Test-infrastructure recommendations

### 4.1 Shared Newton-iteration test helper

Today each solver re-implements its own non-convergence test
("construct an input that won't converge in 30 iterations"). Six
solvers do this; the helper at `internal/finance/testutil/newton.go`
should provide:

```go
// AssertConverges asserts that solver(input) returns within
// maxIters iterations and the result satisfies pred. Reports the
// iteration count taken so tuning regressions are visible.
func AssertConverges(t *testing.T, name string, solver func() (float64, int, error),
                    pred func(float64) bool, maxIters int)

// AssertDoesNotConverge constructs the diagnostic message when a
// solver IS supposed to fail (boundary tests like
// TestSolveLumpDateConvergenceClamp).
func AssertDoesNotConverge(t *testing.T, name string, solver func() (float64, int, error))
```

Effort: **S** (~3 hours). Replaces ~15 hand-rolled snippets across
amortization and PV backward tests.

### 4.2 Table-driven test pattern for dispatch arms

Today each dispatch path gets its own `func TestX(t *testing.T)`. With
46 paths cataloged, that's a lot of test functions. Introduce a
table-driven harness per worksheet:

```go
type mortgageDispatchCase struct {
    name     string
    inputs   map[string]any   // field -> value, omit = blank
    wantOut  map[string]any   // field -> expected
    wantErr  string           // empty = expect success
    arm      string           // "M1", "M4", etc. — for grep
}
```

Then a single `TestMortgageDispatchTable` walks the case slice. Adding
a new arm is one table row, not a new function. Effort: **S** (~4
hours including migrating existing tests).

### 4.3 Round-trip property tests (fuzz-adjacent)

Backward solvers are tested with hand-picked inputs today. Round-trip
properties are stronger:

- Forward(Backward(x)) ≈ x for all valid x
- Backward(Forward(y)) ≈ y for all valid y

Add `go test -run Property -count=100` style fuzz harness using `quick`
or `fuzz`. For each solver:

```go
func FuzzSolveLoanAmountRoundTrip(f *testing.F) {
    f.Fuzz(func(t *testing.T, rate, payment, years float64, periodsPerYear int) {
        // domain guards: skip pathological inputs
        if rate < 0.001 || rate > 0.30 || ... { t.Skip() }
        amt, _ := SolveLoanAmount(...)
        pmt, _ := SolvePayment(amount=amt, ...)
        if abs(pmt - payment) > 0.01 { t.Errorf(...) }
    })
}
```

Catches edge cases hand-picked tests miss (very high rates, very long
terms, sign-mismatch combos). Effort: **M** (~1 day to set up the
fuzz harness for all six solvers and the domain guards).

### 4.4 DOS-regression regeneration workflow

The `legacy/testharness/refdata.pas` workflow is undocumented. Add a
top-level `Makefile` target and a `docs/refdata_regen.md` so the
"how do we get new DOS values?" answer is in version control:

```makefile
refdata:
	cd legacy/testharness && fpc refdata.pas && ./refdata > ../reference-output/refdata.json
	go test ./internal/finance/... -run TestCrossCheck
```

Pair with a CI job that runs `make refdata` against the committed
refdata.json and fails if the file diff is non-empty (catches drift
between Pascal source and committed reference data).

### 4.5 API-level integration tests

Today most engine paths are unit-tested but the **handler→engine wiring
is barely tested**. The amortization silent-failure paths (S-1, S-2)
exist precisely because no test posts to `/api/amortization/calc` and
asserts the response. Establish a single `internal/api/integration_test.go`
that hits every handler with at least one happy path + one error
path. Estimated 30 tests; effort **M** (~6 hours).

### 4.6 Frontend test gap

The frontend has zero automated tests today. `calcAmortization`,
`getAmzInput`, `getPVInput`, `explainMtgError`, and the half-filled
row droppers (S-3, S-4) are pure JavaScript with significant logic.
Recommend introducing **Vitest** or **Jest** for these, even before
adopting Playwright/Cypress for full integration. Effort **M** (~8
hours to scaffold and write 20 unit tests covering the validation
functions).

### 4.7 Skip-list pattern for canary tests

For Wave 1 canary tests that should fail today: add a build tag
`//go:build gaps` and a CI matrix that runs them with the tag enabled
in a separate "tracking" job that's allowed-to-fail. When a gap fix
ships, remove the tag from the now-passing test. This gives the
client a constantly-updated "broken paths" dashboard without
red-on-master.

---

## 5. Recommended sequencing

### Sprint 1 (week 1): Wave 1 canaries + infra setup

- Write all 18 canary tests (~8 hours).
- Set up `internal/finance/testutil/newton.go` shared helper (~3 hrs).
- Document refdata regeneration (~2 hrs).
- **Outcome:** 18 red tests in CI, infrastructure for the rest of the
  work.

### Sprint 2 (week 2-3): Phase 1 + Phase 2 fixes with paired tests

- Phase 1 dispatch_gaps items + T-P1-1 through T-P1-10 (~12 hrs).
- Phase 2 dispatch_gaps items + T-P2-1 through T-P2-10 (~15 hrs).
- 14 of 18 canaries flip to green.
- Add coverage-gap tests G-1 through G-9 (mortgage branches, low
  effort, ~4 hrs).

### Sprint 3 (week 4): Phase 3 structural + remaining coverage

- Phase 3 fixes + T-P3-1 through T-P3-8 (~10 hrs).
- All ~14 fragile error-string assertions rewritten.
- Add G-10 through G-25 (the engine-branch tests) (~8 hrs).
- Set up fuzz harness (4.3) (~6 hrs).

### Sprint 4–6 (weeks 5-8): Phase 4 ports + DOS refdata

- One Phase 4 item per ~2-day cycle: implementation + paired tests
  + new refdata key.
- Total ~20 ports, ~20 tests, ~10 new refdata keys.

### Sprint 7 (week 9): Phase 5 polish + integration tests

- Phase 5 fixes (Sec. 5 of dispatch_gaps).
- API integration tests (4.5).
- Frontend Vitest setup (4.6).

**Cumulative test count after all sprints: ~95 new tests** (18 canaries
+ 52 paired + 25 coverage), plus ~30 integration tests and ~20
frontend tests. Final test count: roughly **3× current**, but coverage
of dispatch-arm logic is comprehensive instead of patchy.

---

## Appendix A — Sample test bodies

### A.1 Canary C-1 (Amortization rate=0 silent failure)

```go
// internal/api/amort_solve_rate_test.go
package api

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

// TestAPIAmortSolveRateFromPayment — Canary C-1.
//
// Documents dispatch_gaps.md silent-failure path S-1: an Amortization
// request omitting `rate` (or sending rate=0) silently runs a
// 0%-rate schedule instead of dispatching to SolveRate.
//
// EXPECTED today: this test fails. After Phase 1 fix (Rate becomes
// *float64, dispatched to SolveRate when nil), this test passes.
func TestAPIAmortSolveRateFromPayment(t *testing.T) {
    body := `{
        "amount": 200000,
        "loanDate": "2025-01-01",
        "nPeriods": 360,
        "perYr": 12,
        "payment": 1199.10
    }`
    // Note: "rate" is intentionally omitted. With *float64 it will
    // arrive nil; with float64 it arrives 0.

    req := httptest.NewRequest(http.MethodPost, "/api/amortization/calc",
        bytes.NewBufferString(body))
    rec := httptest.NewRecorder()
    HandleAmortizationCalc(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("got status %d, want 200", rec.Code)
    }
    var resp AmortizationResponse
    if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if resp.Error != "" {
        t.Fatalf("unexpected error: %s", resp.Error)
    }
    // The payment 1199.10 amortizes $200K over 360 months at ~6%.
    // If the handler silently used rate=0, APR would be 0 and the
    // schedule's interest column would be all zeros.
    if resp.LoanRate < 0.055 || resp.LoanRate > 0.065 {
        t.Errorf("solved rate = %v, want ~0.06 (handler likely ran 0%%-rate schedule silently)",
            resp.LoanRate)
    }
}
```

### A.2 Coverage gap G-17 (PV solveRate second-pass restart)

```go
// internal/finance/presentvalue/backward_secondpass_test.go
package presentvalue

import "testing"

// TestSolveRateSecondPassRestart verifies the second-pass restart
// from rate=0 at backward.go:866-868. DOS PRESVALU.pas:743 restarts
// from r.rate:=0 when the first pass fails to converge; without
// this restart, ~30% of multi-row IRR inputs fail in DOS regression.
//
// Construction: a payment stream that is monotone enough to fool
// the first-pass starting estimate but converges from 0.
func TestSolveRateSecondPassRestart(t *testing.T) {
    input := &PVInput{
        AsOfDate:     mustDate("2024-01-01"),
        SumValue:     -500.0,  // tuned so the first-pass guess overshoots
        SumValueStat: types.InOutInput,
        // ... rows that yield a multi-modal residual function
    }
    result := &PVResult{}
    solveRate(input, result)

    if result.Err != nil {
        t.Fatalf("expected convergence on second pass, got: %v", result.Err)
    }
    if result.Rate < -0.20 || result.Rate > 0.20 {
        t.Errorf("rate=%v out of plausible range; second-pass restart may not have run", result.Rate)
    }
    // To bind that the second pass actually ran (and not just the
    // first one luckily working), instrument with a test-only
    // counter: see backward.go testIterations exported under `test`
    // build tag.
}
```

### A.3 Test-infrastructure helper

```go
// internal/finance/testutil/newton.go
package testutil

import "testing"

// AssertConverges runs solver and verifies it returns a value
// matching pred within maxIters iterations. Reports iteration
// count for tuning visibility (a passing test that suddenly takes
// 28 iterations instead of 4 should be visible in CI logs).
func AssertConverges(t *testing.T, name string,
    solver func() (val float64, iters int, err error),
    pred func(float64) bool, maxIters int) {
    t.Helper()
    val, iters, err := solver()
    if err != nil {
        t.Errorf("%s: unexpected error: %v", name, err)
        return
    }
    if iters > maxIters {
        t.Errorf("%s: took %d iterations (max %d)", name, iters, maxIters)
    }
    if !pred(val) {
        t.Errorf("%s: result %v does not satisfy predicate", name, val)
    }
    t.Logf("%s: converged in %d iterations (max %d)", name, iters, maxIters)
}
```

---

## Appendix B — Test name conventions

Adopting these now (Wave 1) saves churn later:

| Layer | Prefix | Example |
|---|---|---|
| Engine unit | `TestX` | `TestSolveRateZeroAmountErrors` |
| Engine round-trip | `TestRoundTripX` | `TestRoundTripLumpAmount` |
| Engine DOS-regression | `TestCrossCheckX` | `TestCrossCheckMortgageCalc` |
| Engine boundary | `TestXBoundary` or `TestXAtThreshold` | `TestSolveLumpDateRateAtTeenyThreshold` |
| Handler integration | `TestAPIX` | `TestAPIAmortSolveRateFromPayment` |
| Help-doc verification | `TestHelpX_EXn_Y` | `TestHelpAM_EX1_ForwardSchedule` |
| Canary (current bug) | `TestCanaryX_<id>` | `TestCanaryC1_AmortRateSilentlyZero` |
| Coverage gap | `TestCoverX_<id>` | `TestCoverG17_PVSolveRateSecondPass` |

The `Canary` and `Cover` prefixes make `go test -run Canary` /
`-run Cover` direct queries possible, and unify the test taxonomy
with the documents in `docs/`.

---

## Appendix C — Tracking matrix (executive view)

Single table the senior engineer / client can use to track sprint
progress. Each row is one test from this plan.

| ID | Type | Status | Pairs with | Sprint |
|---|---|---|---|---|
| C-1 | Canary | TODO | dispatch_gaps S-1 | 1 |
| C-2 | Canary | TODO | S-2 | 1 |
| ... | (18 canaries) | | | |
| T-P1-1 | Paired | TODO | Phase 1 | 2 |
| ... | (52 paired) | | | |
| G-1 | Coverage | TODO | mortgage M5 untested | 2 |
| ... | (25 coverage) | | | |

(Recommended: maintain this matrix in a small spreadsheet or in
`docs/test_progress.md`, updated per PR.)
