# Adversarial UI↔Engine Test Findings

The adversarial suite lives in `internal/api/adversarial_test.go`. It drives the
REST API (the boundary the frontend talks to) with inputs that make no sense or
try to break the system, and verifies the system fails **gracefully**: never
panics, never hangs or allocates unbounded memory, never returns a 200 with an
empty/undecodable body (the signature of a NaN/Inf result), and for clearly
invalid input returns a non-empty, human-readable error.

~100 cases across the categories: malformed JSON, wrong types, non-finite and
overflow numbers, contradictory / under-determined fields, bad and out-of-order
dates, two-digit-year rejection, advanced-option abuse (balloon-before-first,
duplicate adjustments, unreachable target, malformed skip-months), resource
exhaustion (billion-row / billion-payment), method-not-allowed, and binary
garbage to the `.psn` importer.

## Run it section by section

Each worksheet/endpoint is its own top-level test, so you can run (and read
failures for) one section at a time:

```bash
go test ./internal/api/ -run TestAdversarial_Mortgage      -v
go test ./internal/api/ -run TestAdversarial_Amortization  -v
go test ./internal/api/ -run TestAdversarial_PresentValue  -v
go test ./internal/api/ -run TestAdversarial_MortgageCompare -v
go test ./internal/api/ -run TestAdversarial_MortgageWhatIf  -v
go test ./internal/api/ -run TestAdversarial_ImportPSN       -v
go test ./internal/api/ -run TestAdversarial_MethodNotAllowed -v
# everything:
go test ./internal/api/ -run TestAdversarial -v
```

Each case logs the status and the error message the system actually returned,
so `-v` doubles as a catalog of how the API responds to bad input.

## Robustness gaps found and fixed

The suite surfaced three genuine gaps, all now fixed:

1. **Unbounded amortization schedule (DoS).** The simple-schedule path and the
   solver's `RepayLoan` loop walked every period with no upper bound, so an
   `nPeriods` like 1,000,000,000 would allocate/iterate unbounded and hang.
   - Fix: `amortization.MaxSchedulePeriods = 10000` (mirrors the fancy engine's
     existing runaway guard) enforced both in the engine
     (`generateSimpleSchedule`) and at the API boundary (`HandleAmortizationCalc`
     rejects an oversized `nPeriods` up front). Legitimate loans stay far under
     it — an 80-year weekly loan is ~4,160 periods.

2. **Unbounded What-If table (DoS).** `GenerateRows`/`GenerateGrid` only checked
   `count > 0`, so `count` (or `count2`) in the billions would OOM.
   - Fix: `mortgage.MaxWhatIfRows = 1000`, enforced on both table axes.

3. **Present Value periodic `Pmts/Yr ≤ 0` silently accepted.** Amortization
   already rejected a non-positive payment frequency; the PV handler did not, so
   `perYr: 0` / negative produced a meaningless zero result instead of an error.
   - Fix: `HandlePVCalc` now rejects a periodic row with `PerYr ≤ 0`, naming the
     row.

## Known gaps documented (not fixed — tracked here)

These do not crash, hang, or emit NaN, so the suite asserts health only and
records the behavior rather than failing:

- **PV under-determined multi-row back-solve.** Two single-payment rows each
  missing a *different* field, with one Sum Value, is under-determined and
  should report "too many unknowns." Instead the dispatcher solves one row and
  leaves the other with a default (year-0001) date. It returns a finite result,
  so it is not a crash — but it is a silent-wrong dispatch. Fixing it correctly
  means extending the FirstPass unknown-count logic and risks the legitimate
  single-blank backward solves, so it is deferred.
  Test: `TestAdversarial_PresentValue/two_blanks_underdetermined_known_gap`.

- **Variable-rate date solving vs. the help text.** The help (`#pv-varrate`,
  "Limitations") states variable-rate mode can back-solve only an *amount*, not
  a date or rate. In practice the implementation returns a finite, plausible
  *date* when one is left blank under a rate schedule. This is a
  documentation-vs-implementation nuance (the result may use a simplified
  discount path); it is not a robustness failure. Either enforce the documented
  restriction or update the help to match the implementation.
  Test: `TestAdversarial_PresentValue/variable_rate_solve_date_returns_finite`.

## Notes on intentional, healthy behavior (not gaps)

- **Mortgage tolerates a blank/partial row.** A blank or `null` body to
  `/api/mortgage/calc` returns a 200 echo with no error — by design, because the
  grid computes per row and a partial row simply isn't ready to compute.
  Amortization and Present Value, which need enough data to produce a result,
  do return an error for blank input.
- **Trailing data after a valid JSON object is ignored** (standard
  `encoding/json` decoder behavior). Truly malformed JSON, `NaN`/`Infinity`
  literals, wrong types, and `1e400` overflow are all rejected with a 400.
- **Unknown JSON fields are ignored** rather than rejected (no
  `DisallowUnknownFields`). Harmless; noted for completeness.

## Possible future hardening (out of scope here)

- Add `http.MaxBytesReader` to cap request-body size (a multi-megabyte
  `lumpSums`/`periodics` array is parseable today; bounded by date ranges, not
  by payload size).
- Add a panic-recovery wrapper around the handlers so an unforeseen panic
  becomes a clean 500 rather than a dropped connection. (The suite confirms no
  current input panics.)
