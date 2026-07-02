# Plan: collapse to DOS's single `Iterate` and remove the (non-DOS) bisection

## PROGRESS (2026-07-01)

DONE — Steps 1, 2, and 4 for the OPTION payment-solves:
- Unforced terminal landed (`generateFancyScheduleMode(..., unforced)`, `fancyTerminal`)
  with the one-sided `minpmt` stop and `LastDate` derivation.
- DOS `adjp` seed (`dosSeedPVFactor`) — balloon/prepayment PV subtraction.
- `dosIteratePayment` dispatches the terminal (fancy vs simple/exact) and has a
  seed-fallback (primary estimate → DOS non-prorated adjp seed), Newton-only.
- The skip / balloon / target / prepayment / adjustment payment branches now call
  `dosIteratePayment` instead of `solveFancyPayment`.
- Result: UI-sweep total-interest 57 → 0 (B/C fixed); full suite green.

RESIDUAL / REMAINING:
- One ultra-long-term (n≥240) odd-first + balloon case at ~2e-3 (non-monotone
  terminal, secant root-switch). Bounded in `TestDOSOddFirstFancyFrontier`. Needs
  Step 3 (exact seed/iteration parity via oracle-trace instrumentation) to close.
- Bisection still used by: the in-advance-SIMPLE solve (line ~402, drives
  `generateSimpleSchedule` — needs a RepayLoan annuity-due terminal, not
  `repayExactTerminal` which is ordinary), the exact-solve fallback, the skip
  `refineFancyPayment` fallbacks, and the loan-AMOUNT / RATE solvers
  (`solveFancyAmount` / `solveFancyRate`). These paths already match DOS (no
  divergences); removing the bisection there is mechanism-purity, Steps 4-6 below.

---


## Why

The DOS original has **no bisection**. Its only refinement primitive is
`Iterate` — a finite-difference Newton/secant refinement (AMORTOP.pas:1415-1495,
`{This is written as a general Newton's Method refinement of the var parameter x}`).
DOS uses that one function for every solve: payment, loan amount, rate, balloon
amount, and each ARM re-amortization, selecting one of two terminal procedures:

```pascal
{ AMORTOP.pas:1437 (inside Iterate) }
if (fancy) or ((df.c.exact) and (df.c.basis<>x360)) then
    RepayFancyLoan(p, usap, loandate, firstdate, nil, false, til_adj, no_value_calc, 0)
else
    RepayLoan(p);
```

The Go port instead grew a **bisection** (`fancybisect.go`:
`solveFancyPayment` / `fancyBisect` / `fancyOverUnder`, plus `solveFancyAmount` /
`solveFancyRate`) as a convenience, because the *forced* display schedule (which
folds the final row via `WhenToStop`) makes the unforced Newton residual hard to
observe directly. `fancybisect.go`'s own header admits it is "a more robust
[replacement] without touching the forward engine."

**Directive:** the port must not contain financial logic the original lacks.
Remove the bisection; drive DOS's single `Iterate` (already ported as
`dosIteratePayment`) over the *unforced* terminal — the way DOS does.

## What is already proven (this session; implemented, validated, then reverted)

The hard blocker was computing the **unforced terminal** faithfully. Two bugs were
found and fixed, after which the terminal became correct:

1. **One-sided `minpmt` stop.** DOS's `RepayFancyLoan`-in-`Iterate` does not run to
   `very_last` unconditionally. It stops the moment the running balance drops below
   `minpmt` — one-sided: `WhenToStop^.principal < minpmt` (AMORTOP.pas:1195), which
   includes an overshoot to negative. The first attempt ran to `very_last` always,
   so a too-large payment kept subtracting and drove the terminal to −434k.
   Fix: in the unforced walk, `if p < minPmt { break }` (not the two-sided
   `p < minPmt && p > -minPmt`), plus the `veryLast` bound.

2. **`LastDate` not derived.** The terminal helper called the schedule generator
   directly (not via `Amortize`), so `FirstPass` never derived `LastDate` from
   `NPeriods` and `veryLast` was garbage. Fix: derive
   `LastDate = FirstDate + (NPeriods-1) periods` when `!LastOK`.

With both fixes the unforced terminal is **monotone and crosses zero at DOS's exact
payment**. Canonical check (200000 @ 8%, 120 mo monthly, prepay 500×24 replacing the
regular payment, `plus_regular` OFF):

| trial payment x | fancyTerminal(x) |
|---|---|
| 2500 | +77 799.63 |
| 3000 | +9 135.78 |
| **3066.5254 (DOS answer)** | **0.0008** |
| 3100 | −1 487.08 |

So the terminal is right. The remaining work is seed + secant parity + migration.

## Design target

Two functions only, mirroring DOS:

- **`dosIterate`** — the single Newton (already `dosIteratePayment` in
  `fancybisect.go`; generalize the name/target so it also serves amount and rate,
  as DOS's `Iterate` does via its `var x` parameter).
- **`dosTerminal(input, x)`** — returns the *unforced* terminal balance, selecting:
  - `simpleTerminal` (= DOS `RepayLoan`, AMORTOP.pas:1269) for plain non-fancy,
    non-exact loans — the annuity-due (`ff=(f-1)/(2-f)`, in-advance) or
    arrears-with-`prorate` recursion. **Basis-independent.**
  - `fancyTerminal` (= DOS `RepayFancyLoan` with `Output=nil`) for fancy loans OR
    exact-non-360 — the option-aware walk in `generateFancyScheduleMode(..., unforced=true)`.

`repayExactTerminal` (today's exact terminal) is the exact-accrual case of
`fancyTerminal`; unify or keep as the exact branch.

## Remaining steps (each with a hard validation gate)

### Step 1 — Land the unforced terminal (proven; re-apply cleanly)
- Add `unforced bool` to the fancy walk (`generateFancyScheduleMode`), keeping
  `generateFancySchedule` as the `unforced=false` wrapper (display path byte-for-byte
  unchanged). Gate: the final-row fold chain, the two-sided balance break (→ one-sided
  `p < minPmt` when unforced), the `veryLast` break (unconditional when unforced), and
  the off-cycle `offCyclePaidOff` early return (skip when unforced).
- `fancyTerminal(input, x, ...)`: set `PayAmt=x`, derive `LastDate` if `!LastOK`,
  return `generateFancyScheduleMode(...unforced=true).FinalPrinc`.
- **Gate:** unit test asserting `fancyTerminal` is monotone-through-zero at the DOS
  payment for a battery of loans (the table above). No engine wiring yet.

### Step 2 — Seed parity with DOS `EstimateAndRefinePayment` (THE crux)
DOS seeds `Iterate` from a closed form that subtracts the present value of every
balloon and prepayment (Amortize.pas:384-401):
```
adjp := amount
for balloons:      adjp -= amount_i * exxp(-rate * YearsDif(balloon_i.date, repay_from))
for prepayments:   FirstLastAndFF → adjp -= payment_i * (first - last*ff)/(1-ff)   (else *nn)
d := adjp * (f-1) / denom        {denom = 1 - exxp(-nrepay*lnn(f))}
```
`rate = RateFromYield(loanrate, peryr)` (true rate); `repay_from = loandate`
(prepaid off); `ff = exxp(-rate/peryr)`; `first/last` use the prepay start/stop
dates (stop derived from `NN` = start + (NN-1) periods when blank).

Why this matters: the prepayment-**replace** terminal is ill-conditioned (multiple
near-zeros — e.g. a spurious one at the no-prepay annuity 2426.55 alongside the true
root 3066.53). DOS lands on the correct root **because it starts from this exact
seed and its secant + `bestx` walk from there**. A different seed → the secant
settles on a different near-zero. So the seed must match DOS to the cent.

- **Validation tooling:** DOS's raw seed is not printed by the current oracle. Add a
  `seed`-dump mode to `legacy/oracle/amort_oracle.pas` (instrument a *staged copy* of
  the DOS units — same technique as the AO7 bug post-mortem; do NOT edit read-only
  `legacy/src`) that prints `adjp` and `d` from `EstimateAndRefinePayment` before
  `Iterate`. Then a differential test asserts Go's `adjp`/seed == DOS's across a sweep.
- **Gate:** Go seed matches DOS seed to ≤1e-6 relative on a 1000-case balloon +
  prepayment (additive AND replace) sweep, all bases, all pmts/yr.

### Step 3 — Secant parity with DOS `Iterate`
`dosIteratePayment` already ports the loop (finite-difference secant, divergence
brake `abs(newdelta/delta) > 1 → count += 5`, `bestx` captured *after* the `x`
update, converge at `bestp < halfpenny`, 20-iteration cap). Verify it reproduces
DOS **step for step** given the same seed and terminal — because on an
ill-conditioned terminal the *path* determines which near-zero is returned.

- **Validation tooling:** extend the oracle `seed`-dump to also print each `Iterate`
  step's `x` and `p` (guarded debug copy). Differential test: feed Go's `dosIterate`
  the DOS seed and assert the per-step `(x, p)` trace matches DOS, and the final
  `bestx` matches DOS's solved payment to the cent.
- **Gate:** trace parity on the same 1000-case sweep; final payment 0 divergence.

### Step 4 — Migrate all payment-solve callers off the bisection
Replace every `solveFancyPayment(...)` payment caller (≈9 sites: `engine.go`
dispatch branches for skip / balloon / target / in-advance / odd-first / prepayment /
adjustment, `backward.go` `SolvePaymentClosedForm`, `fancybisect.go`
`solveSegmentPayment`) with `dosIterate` over the correct terminal. Each caller sits
in a specific dispatch context — port them one at a time, re-running the full oracle
suite after each, not all at once.

- **Gate after each caller:** `TestFuzzAmortizePaymentVsDOS` (all 6 basis/mode
  variants), `TestDOSInAdvanceFancyFuzz`, `TestDOSAmortFancySettingsCube`,
  `TestDOSGroundZeroRowCube`, and the full package — all green, and
  `TestUIAmortSweepVsDOS` total-interest fails strictly decreasing toward 0.

### Step 5 — Amount and rate solves (same principle)
DOS solves loan amount (`EstimateAndRefineLoanAmount`) and rate
(`EstimateAndRefineRate`) with the **same** `Iterate` (different `var x` target).
The Go `SolveLoanAmount` / `SolveRate` use `solveFancyAmount` / `solveFancyRate`
(bisection). Migrate these to `dosIterate` targeting amount/rate. NOTE: the closed
form (`SolvePaymentClosedForm`'s `adjp`) is shared with the amount round-trip — this
is exactly what regressed `TestFancyBackwardPrepaymentRoundTrip` when changed in
isolation, so amount/rate must move together with the seed change, not after it.

- **Gate:** `TestDOSFancyBackwardAmountRateRoundTrip`, the fancy backward
  round-trip/property tests, all green.

### Step 6 — Delete the bisection
Once no caller remains, remove `fancyBisect`, `solveFancyPayment`, `fancyOverUnder`,
`solveFancyAmount`, `solveFancyRate`, and `fancyBisectTol`. Confirm nothing imports
them. This is the point at which the port contains only DOS's `Iterate`.

- **Gate:** full `go test ./...` green; `TestUIAmortSweepVsDOS` at **0** total-interest
  divergences; grep confirms no bisection symbols remain.

## Known hazards / gotchas

- **Ill-conditioned replace-mode terminal** — multiple near-zeros. Only exact
  seed + exact secant path (Steps 2-3) lands on DOS's root reliably. This is why the
  work must be trace-validated against an instrumented oracle, not just end-value
  compared.
- **Shared closed form** feeds payment AND amount/rate — change them together (Step 5
  note).
- **Exact vs non-exact accrual** in the terminal: exact non-360 needs actual-day
  (`repayExactTerminal`); non-exact fancy uses the month-based `periodYearFraction`
  in `generateFancyScheduleMode`. The terminal dispatcher must pick correctly.
- **In-advance stays split** (per Fix A, docs/ui_sweep_findings.md #A): the payment is
  the basis-independent simple (`RepayLoan`) solve; only the non-360 *display* uses the
  fancy engine. `dosTerminal`'s simple branch must be the annuity-due `RepayLoan`
  recursion (no first-period proration), not `repayExactTerminal` (which is ordinary).
- **Do not touch `legacy/src`** — instrument staged copies for the oracle seed/trace
  dumps.

## Effort estimate

Steps 1, 3 are largely done/ported. Step 2 (seed parity + oracle instrumentation) and
Step 4 (careful per-caller migration with full revalidation) are the bulk. Realistic:
a focused multi-session pass with the oracle trace harness as the backbone. The bar is
zero divergences across the ~15k existing oracle cases **and** the UI sweep.

## Pointers

- DOS: `legacy/src/dos_source/AMORTOP.pas` — `Iterate` (1415), `RepayLoan` (1269),
  `RepayFancyLoan` (1101), `WhenToStop` fold (1195). `Amortize.pas` —
  `EstimateAndRefinePayment` (377), `FirstLastAndFF` (370),
  `EstimateAndRefineLoanAmount`, `EstimateAndRefineRate`.
- Go: `fancybisect.go` (bisection to remove + `dosIteratePayment`, `repayExactTerminal`),
  `engine.go` (`generateFancySchedule` dispatch + the fancy walk), `backward.go`
  (`SolvePaymentClosedForm`, `SolveLoanAmount`, `SolveRate`).
- Context: `docs/ui_sweep_findings.md` (buckets A/B/C, A closed), this session's
  validated terminal fixes.
