# Diagnosis + RESOLUTION: `internal/finance/amortization` pass5/pass6

_2026-07-16. Found while validating an unrelated frontend change; now FIXED._

## Status: RESOLVED
Both fixes implemented in `engine.go` and validated against the freshly-built
Linux DOS oracle. `dos_audit_pass5` and `dos_audit_pass6` pass; the full
amortization package passes with `PERSENSE_REQUIRE_ORACLE=1` (all DOS
differential sweeps forced on, ~92s), and the API amort UI cube passes.
`engine.go` was written back to the working tree.

## Verdict
The failing tests were **correct** (oracle-verified). The Go engine diverged
from DOS for a payment-only (implied-rate) adjustment at a day-count frequency
(semimonthly/biweekly/weekly). The pass5/pass6 fixes described in the test
comments had never been applied to the engine, even though the tests and the
`solveSegmentRate` helper were committed (`42e2ee7`). The suite had been red on
this path since then. Pre-existing; unrelated to the date-field work.

## Evidence (authoritative — the DOS oracle)
Rebuilt `amort_oracle` from `legacy/src/dos_source` with Free Pascal 3.2.2 on
Linux. All golden commands matched the test goldens exactly:

| command | oracle → payment / interest |
|---|---|
| `100000 0.06 72 24 adj=24::2083` | 1515.5786 / 22172.35 |
| `100000 0.06 72 24 adj=12::1800` | 1515.5786 / 22489.47 |
| `100000 0.06 72 24 adj=24:0.09:` | 1515.5786 / 9640.77 |
| `100000 0.06 78 26 adj=24::2000` | 1401.8410 / 24895.73 |
| `100000 0.06 78 26 adj=18::1600` | 1401.8410 / 17071.80 |
| `100000 0.06 156 52 adj=24::900` | 700.5532 / 19657.54 |

Pre-fix engine produced 1519.3676 (semimonthly payment) and 24920.26 (biweekly
interest) — both wrong.

## Fixes applied (engine.go, +45 lines)
1. **Payment** — dropped the `len(input.Adjustments) == 0 &&` clause on the
   "universal non-shortcut refinement" arm. A day-count first period (first
   payment ON the loan date → zero-length first period) is always "odd"; DOS's
   `EstimateAndRefinePayment` refines every odd-first/in-advance loan and its
   `Iterate` walk strips adjustments (Re_Amortize, AMORTOP.pas:1215), so
   `dosIteratePayment` (which also strips them) returns the correct base payment.
2. **Segment interest (non-360)** — in the AO6 rate dispatch, after the uniform
   `solveAdjRate`, refine the implied rate with `solveSegmentRate` (fancybisect.go)
   over the REAL actual-day segment schedule when
   `exactDaily(settings) || (dayCount && Basis != 360)`, `dayCount := PerYr ∈ {24,26,52}`
   — mirroring DOS's `EstimateAndRefineAdjRate` (Amortize.pas:347-368). On 360
   uniform == actual, so the cheaper uniform solve is kept there.

## Both-directions verification
- Committed engine: pass5/pass6 FAIL (payment 1519.3676; interest 24920.26).
- Part 1 only: payments correct, 360 interest correct, biweekly/weekly interest
  still off (24914.75) — proves part 2 is load-bearing.
- Parts 1+2: pass5/pass6 PASS; full amortization package green vs oracle; API
  amort cube green. `gofmt`/`go vet` clean.

## Follow-up
- The `engine.go` change was still **uncommitted** as of the 2026-07-16 blast-radius
  re-audit — commit it. See `amort_reaudit_pass5_6_blastradius_2026-07-16.md`, which
  validated the fix's blast radius as clean (0 payment divergences across ~2,900
  adjustment cases) and flagged a sub-1e-4 interest residual on very long
  day-count-non-360 adjustment loans (segment-rate convergence floor).
