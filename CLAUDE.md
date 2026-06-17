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
