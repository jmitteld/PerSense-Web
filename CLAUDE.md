# CLAUDE.md — Delphi Pascal → Go Web Port

## Project Overview
Porting a DOS-based Delphi Pascal financial services application (Per%Sense)
to Go, with a web interface. Single binary that serves both the REST API
and the static HTML frontend.

## Goals
- Faithfully reproduce all financial logic from the original Delphi/Pascal source
- Build a web-accessible Go application (REST API + single-page HTML)
- Maintain correctness of all financial calculations (rounding, precision, edge cases)

## Tech Stack
- **Backend:** Go (Golang), `net/http` standard library
- **Frontend:** Single-page HTML with Tailwind CSS (CDN) and vanilla JavaScript
- **Database:** None — all calculations are stateless REST API calls
- **Deployment:** Single binary with embedded static files via `go:embed`

## Code Style & Conventions
- Standard Go project layout (`cmd/`, `internal/`, `pkg/`)
- `decimal` package (shopspring/decimal) is available for monetary values where
  decimal-exact rounding matters; internal financial loops use `float64` to match
  Pascal `real` numerical behavior, with conversion at the API boundary
- All financial logic lives in `internal/finance/` with unit tests
- Exported functions must have GoDoc comments
- Error handling: always return errors explicitly, no panics in business logic

## Porting Rules
- Preserve original Pascal business logic exactly — do not refactor meaning
- Document any generated functions; include intention and expectations
- Flag ambiguous Pascal constructs with `// TODO: verify logic` comments
- Pascal `Currency` type → `decimal.Decimal` at the API boundary; `float64`
  inside iterative solvers
- Pascal integer division behavior must be matched explicitly
- Date handling: use `time.Time` wrapped in `types.DateRec`; watch for
  Pascal's TDateTime epoch differences

## Financial Services Constraints
- All monetary arithmetic must be deterministic and auditable
- Rounding: DOS `Round2` uses **round-half-down** (truncation at .5), not
  banker's rounding — `interest.Round2()` replicates this. See
  `docs/discrepancies.md` §3.
- Keep an audit trail for any stateful financial operations
- Floating point is used inside iterative solvers (Newton, fixed-point) for
  numerical parity with Pascal `real`; convert to decimal at the boundary

## Testing
- Every ported function needs a corresponding `_test.go` file
- Use table-driven tests with known Pascal output as expected values
- Run `go test ./...` before every commit
- **Every bug fix MUST ship with a regression test that fails before the fix and
  passes after.** Confirm both directions (temporarily revert the fix, see the test
  fail, then restore). The test should assert the corrected behavior — not just that
  code runs — and reference the root cause (e.g. the Pascal line or the off-by-N) in a
  comment. This applies to engine fixes (Go `_test.go`) and to frontend fixes (add a
  case to the JS-harness sweeps in `internal/api/frontend_*_test.go`).
  Example: `internal/finance/presentvalue/asof_firstguess_test.go` guards the As-of
  first-guess off-by-100 fix.

## Validation Provenance (READ BEFORE CHANGING ENGINE BEHAVIOR)

The DOS oracle (`legacy/oracle`, run headless) is the ONLY authority for financial
behavior. Two rules exist because ignoring them shipped real regressions:

- **`go test ./...` reporting "ok" is NOT validation by itself.** The DOS differential
  sweeps `t.Skip()` when the oracle binary is absent (`/tmp/oraclebuild/amort_oracle`),
  and a skipped sweep still prints "ok". Before calling any amortization / first-period /
  prepaid / USA / basis change validated, build the oracle and run the GATED suite:
  `make ci` (builds the oracle, sets `PERSENSE_REQUIRE_ORACLE=1`, and FAILS — never
  skips — if the oracle is missing or not runnable; `TestOracleGate` enforces this). A
  present-but-macOS binary counts as absent on Linux — the gate execs a smoke case.

- **Internal-consistency tests must never drive a behavior change.** Round-trip / fuzz
  tests (e.g. `TestExactBackwardRoundTripFuzz`) check that Go agrees with *itself*
  (forward inverts backward); they are regression nets, not truth. When one fails, the
  question is *"which leg matches the oracle?"* — resolve it against DOS, never by making
  the two Go legs agree. A change to a shared engine primitive (first-period prorate,
  Iterate terminal selection, usap routing) is justified only by an oracle-differential or
  an oracle-sourced golden, and must run the FULL oracle suite (blast radius), not just the
  test that motivated it.

- **Golden values carry provenance.** Every pinned expected number cites the oracle command
  and its output in a comment (e.g. `amort_oracle 100000 0.08 360 12 … → 731.98`). A golden
  whose source is "the Go engine produces X" or a chain of reasoning is circular and is not
  allowed — that is exactly how a wrong "DOS = 733.76" value once got shipped.

- **Replicate the DOS logic; do NOT patch around a divergence.** When a differential test
  surfaces a divergence, resolve it by CRAWLING the DOS Pascal source (`Amortize.pas`,
  `AMORTOP.pas`, `PRESVALU.pas`, `Mortgage.pas`, `INTSUTIL.pas`) for the exact code path
  that produces DOS's result and reproducing that mechanism faithfully — the branch it
  takes, the formula it evaluates, the seed it iterates from, the condition it bails on.
  A heuristic guard that clamps a value, skips a case, hard-codes a threshold, or reverse-
  engineers a boundary purely to make the fuzzed inputs match is a PATCH, not a port: it
  drifts from DOS on the inputs the fuzzer didn't happen to draw, and the next pass finds
  the drift. Always answer *WHY* DOS does what it does (cite the source line) before
  changing the port. Example: the 2026-07-16 term-solve over-count was fixed by matching
  DOS's zero-first-period `prorate` in the closed form (AMORTOP.pas:1388), NOT by clamping
  the result to the schedule length; the fancy negative-payment case by adopting whatever
  `Iterate` converges to (Amortize.pas:416 / AMORTOP.pas:1489), NOT by special-casing a
  dominating balloon. If the DOS behavior is itself an artifact (a date-horizon overflow,
  an uninitialized-variable bug), document it WITH the source line and decide deliberately
  whether to mirror it — that adjudication is part of the crawl, not a shortcut around it.

## What to Ask Me
- If Pascal source behavior is unclear, ask before assuming
- If a DOS UI pattern (menus, forms) has no obvious web equivalent, propose options
- Highlight any place where the original may have had Y2K-style date assumptions

## Legacy Source
- Original Delphi/Pascal source is in `legacy/src/` — treat as READ-ONLY reference
- Do not modify any files under `legacy/`
- There are two versions of the original software:
  - `legacy/src/dos_source` contains the original DOS application
  - `legacy/src/win_source` contains the ported Windows version
- **Treat the DOS version as the authority for financial logic.**
- Treat the Windows version as the authority for how the UI should look.
- There may be logic in the DOS version that hasn't been ported to the Windows
  version. We want to include this.
- When porting a module, cite the original filename in a comment, e.g.:
  `// Ported from legacy/src/dos_source/CalcInterest.pas`
- Known good outputs for regression testing are in `legacy/reference-output/`
- Help documentation (HTML) is in `legacy/src/win_source/Help/` — READ-ONLY reference

## Requirements & Discrepancies
- Product requirements: `docs/requirements.md`
- Known DOS-vs-port differences: `docs/discrepancies.md`
- Known DOS-vs-port porting gaps (and what's been filled): `docs/missing_flows.md`

---

## Project Status (current)

### Module Layout
```
internal/
  api/            HTTP handlers for /api/{mortgage,amortization,presentvalue}/calc
  dateutil/       Julian, AddYears, AddPeriod, NumberOfInstallments, YearsDif
  fileio/         Legacy file format I/O
  finance/
    actuarial/    Life-table calculations, contingency types, POD value
    amortization/ Loan amortization schedules + backward solvers
    interest/     Exxp, Lnn, Power, RateFromYield, YieldFromRate, Round2
    mortgage/     Mortgage Calc, APR comparison, row generation
    presentvalue/ PV calculation: forward + BackwardCalc dispatcher
  types/          DateRec, status enums, BasisType, line-status codes
cmd/persense/     Main entry point + embedded static HTML
docs/             requirements, discrepancies, missing_flows, this file
legacy/           DOS + Windows Pascal source (READ-ONLY) and refdata.json
```

### Field-Presence Dispatch Pattern

The DOS application's distinguishing feature is **field-presence dispatch**:
the user fills in some fields, leaves others blank, and the program selects a
formula path to solve for whatever's missing. Each row carries a status:
`empty`, `contains_unknown`, `fully_specified`, `over_determined`.

In the Go port this is implemented as:
1. **API layer** uses pointer types (`*float64`, `*string`) so omitted JSON
   fields become `StatusEmpty`. See `MortgageRequest`, `PVRequest`,
   `AmortizationRequest` in `internal/api/handlers.go`.
2. **`FirstPass`** (in `presentvalue/backward.go`) walks the input, classifies
   each row, and sets `Frontward` or `Backward` flags.
3. **`Calculate(input)`** is the public entry point. It runs `FirstPass`, then
   dispatches to `forwardOnly` (forward calc) or `BackwardCalc` (solve for
   missing field).
4. **`BackwardCalc`** routes by `BackwardKind` to one of the per-path solvers:
   `solveLumpAmount`, `solveLumpDate`, `solvePeriodicAmount`,
   `solvePeriodicDate`, `solveRate`, `solveAsOf`.

The `mortgage.Calc` function follows a similar pattern via direct
`*Status == InOutInput` checks (no separate dispatcher).

### What's Ported

| Module | Forward | Backward / Field-presence dispatch |
|---|---|---|
| Mortgage `Calc` | ✓ | ✓ — Pct↔Cash↔Financed; Price↔Monthly; Balloon |
| Mortgage APR comparison | ✓ — `CompareAPRs` | n/a |
| Mortgage row generation | ✓ — `GenerateRows`, `EnoughDataForRowGeneration` | n/a |
| Present Value | ✓ — `forwardOnly` | ✓ — `BackwardCalc` (7 paths: PV-1, PV-2, PV-4, PV-5, PV-6, PV-8, PV-9; see `docs/missing_flows.md`) |
| Amortization fancy schedule | ✓ — balloons, adjustments, prepayments, moratorium, target, skip-months | ✓ — `SolveLoanAmount`, `SolveRate` |
| Actuarial (life contingency) | ✓ — POD, LifeProb | n/a |

### Outstanding Items

The Phase-4 financial-logic ports and the Revision-4 fidelity gaps
are done; Revision 9 (2026-05-26) closed the AO7, VR-COLA, and
USA-rule + ARM gaps that the original Outstanding Items list called
out.  Revision 10 (2026-06-17) closed the **odd-first-period ×
{prepaid | balloon | 365} frontier** that the exhaustive `Amortize`
dispatch sweep isolated: the blank-payment solve now refines the
odd-first estimate against the real schedule (`oddFirstPeriod` +
`solveFancyPayment`), and off-cycle balloons are applied at their
exact date (balloon draining in `generateFancySchedule`) instead of
folded into the next payment.  Validated to zero divergence vs the
real DOS engine (`TestDOSOddFirstFancyFrontier`, now a strict guard);
see `docs/dos_known_frontier.md`.  This also surfaced a DOS-vs-Windows
discrepancy in odd-first payments — DOS augments, the Windows help
does not; the port follows DOS (`docs/discrepancies.md` §7).
Revision 11 (2026-06-17) began the exhaustive option-cube sweeps
(`docs/exhaustive_option_sweep_plan.md`): the mortgage dispatch cube
(`TestDOSMortgageDispatchCube`, 540 cells, 0 divergence) and the
amortization settings cube (`TestDOSAmortSettingsCube`, basis ×
prepaid × in-advance × exact × pmts/yr). The amort cube surfaced a
real blank-payment-solve gap for **in-advance** loans (Go solved the
ordinary annuity; DOS iterates the annuity-due — Amortize.pas:402-416);
the `needRefine` change in `engine.go` closed it on the 360 basis and
every flag pair.  Revision 12 (2026-06-22) closed the
**exact × in-advance** frontier — the last narrow triple — by adding a
dedicated `generateExactInAdvanceSchedule` path that reproduces DOS's
distinct annuity-due SHAPE (a row-0 settlement-interest row at the loan
date, a one-period base-date shift, and `n-1` actual-day amortizing
rows; AMORTOP.pas:1159-1177 + ComputeNext) and solving its payment with
the in-advance branch of `dosIteratePayment`/`repayExactTerminal`.
Validated to zero divergence vs the rebuilt Linux DOS oracle:
`TestDOSGroundZeroRowCube` now classifies exact×in-advance (non-360
basis) as CLEAN (rows + payment to the cent), and a focused
`TestDOSExactInAdvanceSettlement` checks the settlement row and totals
via `dumpraw`.  The non-exact (whole-month) annuity-due schedule and
360-basis in-advance (where the exact method is inert) remain the
separate, bounded `envInadvPay` frontier.
Revision 13 (2026-07-01) closed the **in-advance × fancy** corner
(the former `docs/dos_known_frontier.md` #38 bounded ~2-3% envelope):
`generateFancySchedule` now reproduces DOS `RepayFancyLoan`'s in-advance
SHAPE for the general (non-exact) fancy path — a settlement-interest row
at the loan date, a one-period base-date shift, and ORDINARY
opening-balance interest on the shifted walk (AMORTOP.pas:1159-1187 +
ComputeNext:636) — replacing the old post-payment interest-recompute
approximation (the `inAdvanceFancy` block in `engine.go`). The moratorium
boundary recompute accounts for the shift (`n-1` amortizing rows), and
`moratoriumActive` keys on the shifted first date.  `TestDOSAmortFancySettingsCube`
in-advance cells are now strict 0 divergence, and a new ~3,300-case
differential fuzz sweep `TestDOSInAdvanceFancyFuzz` (skip / moratorium /
balloon / prepayment × basis × prepaid × pmts/yr vs the DOS oracle)
asserts payment + every schedule row to the cent for skip / moratorium /
balloon / prepayment.  The two sub-frontiers it first surfaced are now
BOTH resolved: the **prepayment NN-derived trailing row** is CLOSED
(`veryLast` derives the stop date = StartDate + (NN-1) periods per DOS
`DetermineVeryLast`; this also added the previously-missing prepayment
blank-payment-solve refinement and a `fancyOverUnder` fix scoped to
prepay loans), and **balloon on/before the first payment date** is
handled correctly (balloon-before-first errors in both engines;
balloon-on-first in-advance is a DELIBERATE divergence — DOS's dead
`firstd` init inflates the payment but never applies/collects the
balloon, so the port keeps the financially-correct result and does not
reproduce the DOS bug).  All tracked in `docs/dos_known_frontier.md`.
What remains, all explicitly scoped-down in `docs/dispatch_gaps.md`
§0.11.5 with rationale:

- **PV `V_3` ifdef block** (`const_signal`) is intentionally NOT
  ported: `V_3` is never `{$define}`d in the DOS source, so that
  block is dead code in the authoritative DOS build
  (`docs/dispatch_gaps.md` §0.5.5).
- **Engine-wide `FieldError` threading** — the structured error
  type and the advanced-option row errors are in place; the
  frontend's inline-error highlighting works via regex-based field
  detection on the message string.  Threading `FieldError` through
  every deep-engine `fmt.Errorf` and retiring `explainMtgError` is
  a structural refactor that does not change wording.
- **`SolveLoanAmount` / `SolveRate` for fancy loans** — these refine
  the closed-form estimate with `solveFancyAmount` / `solveFancyRate`
  (the schedule-oracle bisection in `fancybisect.go`, which drives the
  real DOS-validated forward schedule to a zero terminal balance — the
  same criterion as DOS's `Iterate`).  Now validated against the real
  DOS engine by `TestDOSFancyBackwardAmountRateRoundTrip` (round-trips
  a DOS-solved payment back to the original amount/rate: 0 divergences,
  max relErr ~5e-6 amount / ~2e-5 rate).  The two `// TODO: verify
  logic` markers in `backward.go` predate that validation.
- **V6-7 sub-day `timedif` shortcut** and **V6-14 yearly/quarterly
  summary aggregation** — presentation-grade only; both leave the
  per-payment numbers untouched.
- **Extending `legacy/testharness/refdata.pas`** — the checked-in
  `refdata.json` is current (see `scripts/regen_refdata.sh`), but
  the harness doesn't yet cover Rule-of-78, in-advance fancy,
  biweekly basis, month-specific COLA under VR, or the AO7 / V6-2
  USA-rule-with-ARM end-states.  Adding one representative case per
  area would tighten DOS-output coverage; doing so requires
  touching `legacy/`, hence the deferral.

### Advanced Options (Amortization)

The Amortization screen's Advanced Options panel supports:
- **Prepayments** — extra periodic payments between two dates at a given freq
- **Balloons** — one-time lump payments at specific dates
- **Adjustments** — rate and/or payment changes on specific dates (ARMs)
- **Moratorium** — interest-only deferment until a given date
- **Target** — minimum principal reduction per payment
- **Skip Months** — months when payments are suppressed (string like "6-8,12")

When any advanced option is supplied via the API, the request automatically
runs in **fancy mode** (`input.Fancy = true`). The fancy schedule engine in
`internal/finance/amortization/engine.go: generateFancySchedule` walks
period-by-period and consults each option per period. Order of operations
within a period mirrors DOS `Paymenttype.ComputeNext` in AMORTOP.pas:574–664.

**DOS-faithful behavioral note**: when both `targetAmt` and `skipMonths` are
set on an amortization, target's minimum-principal-reduction overrides
skip-month zeroing (matches DOS at AMORTOP.pas:643). This is documented in the
test `TestAPIAmortAdvancedTargetOverridesSkipMonth`.

### Testing Patterns

Two complementary styles are used:

1. **Round-trip tests** validate internal consistency. Forward-calculate a
   known input, then run the backward solver against the result and verify
   the original inputs are recovered. Examples:
   `presentvalue/backward_test.go: TestRoundTripLumpAmount`,
   `TestRoundTripPeriodicToDate`, `TestRoundTripRate`, etc.

2. **DOS-regression tests** validate against known Pascal output. The
   reference data lives at `legacy/reference-output/refdata.json`,
   generated by running `legacy/testharness/refdata.pas` under Free Pascal.
   The cross-check tests in `internal/finance/crosscheck_test.go` and
   `internal/finance/crosscheck_backward_test.go` load this JSON and assert
   the Go port produces matching values within a documented tolerance.

   **When adding a new financial function, add at least one DOS-regression
   case** — round-trip tests alone can hide systematic forward/backward bias.

3. **Boundary tests** cover threshold values explicitly:
   - Rate at the `Teeny=1e-10` cutoff (not just "near zero")
   - `cola = rate` exactly (special-case branch in PV-5)
   - Newton non-convergence (max iterations hit)
   - Empty inputs, single-row inputs
   - Very long terms (50+ years), very high rates (50%+)

   See `presentvalue/backward_boundary_test.go` for the canonical examples.

### API Surface

Three endpoints, all `POST` with JSON bodies:
- `/api/mortgage/calc` — single-row mortgage Calc
- `/api/amortization/calc` — schedule generation; supports Advanced Options
- `/api/presentvalue/calc` — PV with optional backward solve

All three handlers use **pointer fields** for optional inputs. Omitting a
field means "blank" — for backward calc, omit the field you want solved for
and supply `sumValue` (PV) or the relevant target.

See `docs/QUICKSTART.md` for sample request bodies.

### Syncing changes back to Nate's machine (STANDING WORKFLOW)

When working in a cloud/Cowork session, the git repository lives on Nate's
Mac at `/Volumes/SSK/persense/PerSense-Web` (connected via the device
bridge); the cloud workspace copy has NO `.git`. **Every time a change-set
lands (fix verified, tests green), sync the modified/new files back to that
folder without being asked** — work that only exists in the cloud workspace
is invisible to git and dies with the session. Procedure: stage the local
counterparts first (`device_stage_files`) and content-diff to confirm the
device copies contain nothing the cloud lacks, then `SendUserFile` +
`device_commit_files` with the staged `mtimeMs` as `expectedMtimeMs`.
Never sync `legacy/src/dos_source/*.pas` unless actually changed — the DOS
sources contain CP437 box-drawing bytes that transfer re-encoding can
corrupt. Committing to git stays manual (Nate reviews and commits).
