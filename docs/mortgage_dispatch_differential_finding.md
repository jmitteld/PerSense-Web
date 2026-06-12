# Mortgage dispatch — differential vs the real DOS engine (2026-06-10)

## What was built

The mortgage **field-presence dispatch** — `Mortgage.Calc`'s decision of which
field to solve from which are blank: the funding triangle (Price ↔ Pct / Cash /
Financed), Price ↔ Monthly, balloon, vs refuse an over-/under-determined screen —
is now validated two ways. The solved *values* were already bit-validated (the
mortgage engine is at 97); this pins the dispatch *decision* over the full
presence matrix.

1. **Oracle-independent decision table** —
   `internal/finance/mortgage/dispatch_differential_test.go:
   TestMtgDispatchCanonical`. Asserts the canonical rules: solve Monthly from
   Price+down+Years+Rate; solve via Cash as well as Pct; solve Price from
   Monthly+Pct; over-determined when Price AND Monthly are both given with no
   balloon; refuse a Price-from-Monthly solve when only Financed (not Pct/Cash)
   is given; fill the funding triangle (cash/financed) but solve no payment when
   Years/Rate are absent; hard errors on Years≤0 and a balloon amount without a
   balloon year. Runs in plain CI.

2. **Dispatch-by-consequence differential vs the real DOS engine** —
   `TestDOSMtgDispatchSweep` + a new `eval` mode in `legacy/oracle/mtg_oracle.pas`.
   The full 7-field presence matrix {Price, Pct, Cash, Financed, Monthly, Years,
   Rate} (128 patterns) is fed to the genuine DOS `Mortgage.Calc` (via
   `CalculateRows`, which runs the real `FirstPass`+`Calc`) and compared by
   consequence: refusal vs solve, and for each of {monthly, price, cash,
   financed} whether it became a computed OUTPUT and its value.

The consistent tuple is 200000, 20% down (cash 40000, financed 160000), 30yr, 7%
true rate, monthly 1066.683053, Points 0, no balloon — so any well-posed single
unknown is solvable and recovers the others.

## Result

**128 cells, 0 divergences.** 120 both-solve (including the *partial* cells where
only the funding triangle fills in — cash/financed computed but no payment
solved — which both engines handle identically), and 8 both-refuse (the
over-determined Price+Monthly screens and the Price-from-Financed-only cases). On
every solved cell Go and DOS agree on exactly which fields become outputs and on
their values to <0.01.

This completes the dispatch differential across all three calculation screens —
present value, amortization, and mortgage — each now validated against the real
DOS field-presence dispatch, not just the numeric solvers it feeds.

## Durability

The decision table runs in plain CI; the differential skips cleanly when the
oracle binary is absent and runs in the `dos-fidelity` CI job otherwise. A future
change to the mortgage dispatch that diverges from DOS fails the build.

## Scope / not covered

Points and balloon presence are held fixed (Points 0, no balloon) in the swept
matrix; the balloon-solve and points paths are exercised by the canonical table
and the existing value sweeps (`dos_mtg_oracle_test.go`), not by the full 128-cell
differential. Adding balloon-presence and a points axis would be a
straightforward extension of the same `eval` mode.
