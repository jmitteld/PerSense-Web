# Amortization dispatch — differential vs the real DOS engine (2026-06-10)

## What was built

The amortization **which-field-to-solve** dispatch — the decision of which of the
four solvable top-row fields {Amount, Rate, Payment, #Periods} to solve for,
given a valid date/frequency context, vs build a forward schedule vs refuse — is
now validated two ways:

1. **Oracle-independent decision table** —
   `internal/finance/amortization/dispatch_differential_test.go:
   TestAmortDispatchCanonical`. With a valid context (Pmts/Yr, Loan Date, 1st Pmt
   Date present; Last Pmt Date blank) and a self-consistent tuple (10000 @ 12%
   nominal, payment 888.4879, n=12 monthly), it enumerates the 16-cell
   {A,R,P,N}-presence matrix and asserts the canonical rule: the screen is
   **solvable iff at most one of the four is blank** (exactly one unknown), and
   refused otherwise. The rule comes from the amortization spec, not the port's
   own code. Runs in ordinary CI.

2. **Dispatch-by-consequence differential vs the real DOS engine** —
   `TestDOSAmortDispatchSweep` + the new `eval` mode in `legacy/oracle/amort_oracle.pas`.
   The same 16 patterns are fed to the genuine DOS `MakeTable` dispatch
   (compiled headlessly) and compared by consequence: SOLVABLE vs REFUSED, and
   the solved regular payment.

The Go side mirrors the production dispatch faithfully: the API handler solves
Amount and/or Rate up front when blank (`handlers.go:1050-1103`, via
`FirstPass`-on-a-copy + the `CanCompute*` guards), then `Amortize` itself solves
the payment or the term when those are blank.

## Result

**16 cells, 0 divergences.** 5 both-solve (all-present forward + each of the four
single-unknown solves), 11 both-refuse (every ≥2-unknown pattern), and the solved
payment matches DOS to <0.01 in all five solvable cells. The Go amortization
dispatch reproduces the DOS `MakeTable` solve-selection exactly across the
matrix, including the refusal of every over-/under-determined screen.

## A methodology note worth recording

The first run showed two "divergences" (Go solved two 2-unknown screens DOS
refused). The cause was the **test harness, not the engine**: it left the
consistent tuple's *value* behind a blank *status* (e.g. `NPeriods=12` with
`NStatus=empty`), so the Go engine used a "ghost" input the user never supplied,
while the oracle zeroed blank fields. Zeroing blank values on the Go side (as the
real API does — an omitted pointer is value 0) collapsed both to a clean refuse,
matching DOS. Lesson, same as the PV pass: differential inputs must be identical
down to the implicit conventions, and a blank field must be blank in *both* value
and status. The corrected harness confirms the engine itself correctly ignores
values behind an empty status.

## Scope and what remains

This pass covers the core which-field-to-solve decision over {Amount, Rate,
Payment, #Periods} with a fixed valid date/frequency context. The orthogonal
**date/term-derivation** dispatch (deriving #Periods from First+Last dates, or a
default First date from Loan Date + Pmts/Yr — DOS `FirstPass` A-FP-defFirst /
A-FP-last / A-FP-n) is already pinned by `firstpass_test.go`. A natural next
extension is a second differential axis varying {#Periods, First Date, Last Date}
presence against the DOS `FirstPass` derivation, and the fancy-option backward
dispatch (prepayment-amount/duration, balloon, adjustment solves), which are
already value-validated by the existing per-row sweeps.

## Durability

The decision table runs in plain CI; the differential skips cleanly when the
oracle binary is absent and runs in the `dos-fidelity` CI job otherwise. A future
change to the amortization dispatch that diverges from DOS fails the build.
