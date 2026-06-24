# Actuarial contingency-shape cube — finding

*Added 2026-06-23. Extends the "shapes" sweep philosophy (originally from the
amortization frontier work, `docs/exhaustive_option_sweep_plan.md`) to the
life-contingency / Present-Value paths.*

## Why this exists

The amortization engine was hardened by sweeping its full option space
exhaustively and diffing every cell against the real DOS oracle
(`TestDOSGroundZeroRowCube`, `docs/dos_known_frontier.md`). The actuarial paths
**cannot** be swept against the DOS engine: the `ACTUARY` unit and the
`MALE`/`FEMALE` mortality tables are missing from this source snapshot
(`docs/actuarial_oracle_blocked.md`). So a DOS-fidelity cube is impossible here.

This cube is the faithful analog within that constraint. It sweeps the full
actuarial **shape** space through the production PV dispatch and diffs each
forward cell against an **independent** oracle, then round-trips each backward
cell.

Tests: `internal/finance/presentvalue/actuarial_shape_cube_test.go` (fixed-rate
forward + backward) and `actuarial_shape_cube_vr_test.go` (variable-rate forward,
solve-as-of, standalone-POD).

## The cube

| Dimension | Values |
|---|---|
| Payment kind | lump, periodic |
| Contingency | NotContingent, Living, Dead, Only1Living, Only2Living, EitherLiving, BothLiving |
| Lives | one / two (implied by contingency) |
| Direction | forward, solve-amount, solve-rate, solve-as-of, solve-POD |
| Rate | fixed (0.04, 0.06) and variable (3-segment piecewise schedule) |

## The injected "special oracle profile"

Every other PV actuarial test uses a mock quadratic `qx` curve. This cube
instead **injects the SOA Standard Ultimate Life Table (SULT)** — generated
from Makeham's law (A=0.00022, B=2.7e-6, c=1.124), the table the SOA publishes.
Two benefits:

1. It validates the engine on a *second, independent* mortality profile, so the
   port can't silently depend on a quirk of the mock curve.
2. The Makeham closed form lets the expected PV be computed **in-process**, with
   no external dependency — so the cube always runs in CI (like
   `TestActuarialSOAPublishedAnchor`), unlike the live `actuarialmath` oracle
   (`scripts/actuarial_oracle.py`) which skips when python is absent.

The oracle is genuinely independent: expected forward PV is an explicit
summation over payment dates reading survival straight out of the SULT `lx`
column (`kp_x = lx[ageAtPayment]/lx[ageNow]`) and combining the two lives by the
documented contingency algebra. It never calls the engine's `LifeProb` /
discount / summation helpers. Dates are pinned to Jan-1 so the 30/360 year
fraction is integral and ages index `lx` without interpolation — matching the
engine exactly.

## Result

Fixed-rate (`actuarial_shape_cube_test.go`):

- **Forward differential: 70 cells, max relative error 1.88e-14** — bit-level
  agreement with the independent SULT oracle across every contingency × payment
  kind × rate.
- **Backward round-trips: 14 amount solves + 7 rate solves, all recovered** to
  < 0.5 (amount) / 1e-4 (rate).

Variable-rate + extra directions (`actuarial_shape_cube_vr_test.go`):

- **VR forward differential: 35 cells, max relative error 1.86e-14** — the
  contingent forward under a 3-segment piecewise-rate schedule matches an
  independent `amount × exp(-∫rate) × contingencyWeight` oracle, where the rate
  integral is re-derived over whole-year segments without the engine.
- **Solve-as-of: 7/7 contingencies** recover the as-of date to < 0.05 yr.
- **Standalone-POD solve: 7/7** recover the Payment-on-Death amount to < 1.0
  with a survival-weighted companion row of each shape on the screen.

Degenerate cells (mid-stream contingency weight < 0.02) are skipped throughout,
the same guard the existing per-contingency tests use.

Zero divergence. No engine change was required — this is a coverage/confidence
addition, not a bug fix.

## Scope / honest ceiling

This raises **shape coverage** of the actuarial PV dispatch to parity with the
amortization and mortgage cubes, but it does **not** raise the
*faithful-to-DOS* confidence ceiling (~86, `docs/actuarial_source_investigation.md`):
the oracle is standard actuarial mathematics on the SULT, not the original
(missing) DOS `ACTUARY` computation. The three unverifiable unknowns remain the
DOS mortality-table values, its intra-year interpolation convention, and its
exact `PODValue` quadrature. Closing those still requires the client's
`ACTUARY` unit / `MALE`+`FEMALE` tables (`docs/actuarial_files_to_request.md`).

Now in the cube (added 2026-06-23): variable-rate (PVLX) contingent forward
values against an independent piecewise-rate oracle, the `solve-as-of`
direction, and standalone-POD-amount solves across every contingency.

Still bounded / not swept (low value, scattered coverage exists): the
`solve-date` directions (lump-date / periodic-from/to-date) under contingency;
VR contingent *backward* amount across all contingencies (covered for Living /
BothLiving in `actuarial_vr_pod_audit_test.go`); and COLA-stepped contingent
streams under VR (the COLA math is exercised separately in
`actuarial_cola_test.go`).
