# Finding: single-payment loan (n=1) — Go accepts, DOS rejects

> **STATUS: FIXED 2026-06-09 (approved, option 1).** The Go amortization engine
> now rejects a single-payment loan with "There must be at least two regular
> payments," matching the DOS table generator (`engine.go`, the firstDate ==
> lastDate guard after FirstPass). `TestAmortizeShortTerm` updated to expect the
> rejection and to confirm n=2 (the minimum) still works; full suite green.

*Surfaced 2026-06-09 by the boundary-case differential testing
(`TestDOSBoundaryCases` exploration). A minor input-validation divergence, not a
calculation error.*

## What differs

For a loan with a single regular payment (`n = 1`):

- **DOS** refuses to compute it: `FirstPass` raises *"There must be at least two
  regular payments."* and produces no schedule.
- **Go** accepts it and produces a valid one-row schedule.

Every loan with `n >= 2` (the smallest DOS allows) is validated and matches —
see `TestDOSBoundaryCases` (n=2 cases included).

## Assessment

This is an input-validation difference, not a numeric one: where DOS draws a
hard floor at two payments, the Go engine is more permissive. The single-payment
schedule Go produces is internally consistent (a lump payoff), so no wrong
*number* is generated — Go simply allows a case the original disallows.

## Options

1. **Match DOS (faithful):** add the same `n >= 2` guard to the Go engine's
   first-pass validation so a single-payment loan is rejected with an equivalent
   message. One-line change; it makes a previously-accepted input an error, so
   it is a (small) behavior change for any API caller relying on n=1.
2. **Leave as-is and document:** keep Go's more-permissive behavior and note the
   divergence. The numbers for n>=2 are unaffected either way.

Per the client's guidance that ported behavior changes be validated to the
original's standard, this is surfaced for a decision rather than changed
silently. Recommended: option 1 (match DOS), since "faithful to the original"
is the project's goal and the single-payment case is degenerate.
