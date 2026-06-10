# Prepayment ("Additional Periodic Payments") — semantics study + finding

> **STATUS: PRIMARY ISSUE (replace-vs-add) FIXED 2026-06-09 (approved).** The Go
> fancy engine now honors `PlusRegular` for prepayments exactly as for balloons:
> coincident extras are summed and either added (PlusRegular ON) or **replace**
> the regular payment (OFF, the default — a payment schedule), matching DOS. The
> worked example now matches (interest 27,646.39 / 60 rows), and a new per-row
> differential sweep (`TestDOSPrepaymentPerRowSweep`, 250 replace + 250 add
> loans) matches the real DOS engine with **0 divergences in both modes**. Two
> existing tests that asserted the old additive default were updated to set
> `PlusRegular = true` (their clear intent — additive "extra" payments).
> **Off-cycle prepayment rows — FIXED 2026-06-10 (validated).** A prepayment
> whose date falls strictly between regular payment dates is now emitted as its
> own dated row with its own partial-period interest accrual, matching DOS
> `FindNextExtra` (`balloonpos < 0`). Validated by `TestDOSOffCyclePrepaymentSweep`
> (300 randomized cases, more-frequent prepay series than the regular schedule):
> 0 row-count fails, 0 value fails, max balance relErr 1.1e-4 against the real
> DOS engine.
>
> **Backward solvers (AO9 amount / AO10 duration) — VALIDATED 2026-06-10, with two
> documented gaps.** A `presolve=`/`predur=` path was added to the DOS source-oracle
> so the *real* `EstimateAndRefinePeriodicPrepayment` and `DeterminePrepaymentDuration`
> can be driven headless and differenced against Go (`TestDOSPrepaymentAmountSolveSweep`,
> `TestDOSPrepaymentDurationSolveSweep`). Result:
>
> - **Amount solve under the REPLACE default — matches DOS exactly** (250/250
>   cases, max relErr 1e-8). This is the case that matters for the default
>   semantics and confirms the primary fix end-to-end through the backward path.
> - **Amount solve under ADD (plus_regular ON) — documented gap.** With additive
>   prepayments the final scheduled payment settles any residual, so "final
>   balance == 0" (Go's objective) holds for a *range* of amounts; DOS instead
>   solves the unique value at which the discounted payment stream equals the
>   principal. Go's secant returns its initial guess (≈ pay/2). 17/249 cases
>   diverged, up to ~15%. See "Two documented backward-solver gaps" below.
> - **Duration solve (AO10) — documented gap (larger, systematic).** DOS uses a
>   closed-form present-value duration that credits the regular payment over the
>   *full nominal term*; Go simulates the schedule to payoff and counts. Different
>   quantities, diverging in 227/229 cases. See below.
>
> Per the standing rule, the two gaps are **surfaced for a decision, not silently
> changed.** The replace-default path — the one the primary fix established as the
> DOS default — is fully validated.

*Result of the focused study of the DOS additional-periodic-payment feature
(2026-06-09). It found a real, significant divergence between the Go engine and
DOS in how these payments are applied.*

## How DOS actually works

The "Additional Periodic Payments" feature is driven by `FindNextExtra` and
`Paymenttype.ComputeNext` (`AMORTOP.pas:490-664`). The regular payment `d` is
**unchanged** by the additional payments; each period the engine finds the next
"extra" (prepayment and/or balloon) and combines it with the regular payment
according to where its date falls:

1. **Extra on a regular payment date** (`balloonpos = 0`,
   `AMORTOP.pas:614-621`):
   - if `plus_regular` is **ON**: `payamt := d + extra` (added on top);
   - if `plus_regular` is **OFF (the default)**: `payamt := extra` — the extra
     **replaces** the regular payment.
2. **Extra between regular payment dates** (`balloonpos < 0`,
   `:608-613`): the extra is emitted as **its own dated row** with just the
   extra amount, and interest accrues for the partial period.

So by default (`plus_regular` OFF) an additional periodic payment is a *payment
schedule* that **replaces** the regular payment for those dates — exactly what
the help describes: *"use the Additional Periodic Payments line … to enter the
payment schedule."* Turning the "Balloon includes regular payment" setting ON
makes them additive instead.

## What the Go engine does

`generateFancySchedule` (`engine.go`, the prepayment loop ~`:949-984`) applies
every prepayment as:

```go
pmt += pp.Payment   // always added on top
```

It **never replaces** — `PlusRegular` is consulted for balloons (`:929-934`) but
**not** for prepayments. And an off-cycle prepayment is **folded into the next
period's payment** rather than emitted as its own dated row (the code comments
note this: *"it lands a few days later than the DOS row"*).

## The divergence, measured

100,000 at 8%, 60 monthly payments, regular payment 2,027.6394 given, plus a
monthly additional payment of 500 in months 12-23 (coincident with regular
dates), `plus_regular` OFF (the default):

| | total interest | rows |
|---|---:|---:|
| **DOS** (replace) | 27,646.39 | 60 |
| **Go** (always add) | 19,775.36 | 57 |

DOS *replaces* the 2,027.64 payment with 500 for a year → negative amortization,
more interest, full term. Go *adds* 500 on top → faster payoff, less interest,
fewer rows. These are fundamentally different results from the same inputs.

## Two distinct issues

1. **Replace-vs-add (primary).** Go ignores `PlusRegular` for prepayments and is
   always additive; DOS's default replaces. This flips the feature's meaning.
2. **Off-cycle rows (secondary).** A prepayment whose date falls between regular
   payment dates is a separate dated row in DOS; Go folds it into the next
   period. Same total principal, but a different schedule shape and per-row
   split, and a slightly different interest accrual timing.

## Recommendation

Fix to match DOS (faithful translation), in two parts:

- **Honor `PlusRegular` for prepayments** exactly as for balloons: on a
  coincident regular date, replace the regular payment when `PlusRegular` is
  off, add when on. This is the high-impact change and mirrors existing balloon
  code, so it is small and localized.
- **Emit off-cycle prepayments as their own rows** (as DOS does) for full per-row
  fidelity. This is the larger of the two and can follow.

Both are validatable with the now-working fancy per-row oracle
(`TestDOSBalloonPerRowSweep` is the template; a prepayment per-row sweep would
confirm the corrected behavior against DOS across randomized cases).

**Why surfaced, not auto-applied:** this changes prepayment totals and schedules
substantially. Per the client's guidance that ported-behavior changes be
validated to the original's standard, it is presented for a decision. With
approval, the replace-vs-add fix + a prepayment per-row sweep is a contained next
step; the off-cycle-row fidelity can follow as a second pass.

---

## Two backward-solver gaps — both CLOSED (2026-06-10, approved)

> **STATUS: BOTH GAPS FIXED and validated against the real DOS engine.**
> Gap A (additive amount) and Gap B (duration) below were originally surfaced
> for a decision; with approval, DOS's closed-form formulas were ported into
> `SolvePrepaymentAmount` (additive branch) and `SolvePrepaymentDuration`.
> Results now match the real DOS engine: the additive amount solve matches to
> ~5e-8 (`TestDOSPrepaymentAmountSolveSweep`, 0 divergences) and the duration
> solve matches **exactly** (`TestDOSPrepaymentDurationSolveSweep`, max |diff|=0).
> The replace-default amount solve continues to match to ~1e-8. The original
> analysis is retained below for the record.

After the forward fixes (replace-vs-add, off-cycle rows) were validated, the two
prepayment *backward* solvers were differenced against the real DOS engine via a
new oracle path (`presolve=` and `predur=` tokens in
`legacy/oracle/amort_oracle.pas`, exercising the actual
`EstimateAndRefinePeriodicPrepayment` and `DeterminePrepaymentDuration`). The
replace-default amount solve matched DOS to 1e-8; two cases diverged and were
documented here for a decision (since closed).

### Gap A — unknown prepayment AMOUNT, additive (plus_regular ON)

**Symptom.** Go's `SolvePrepaymentAmount` returns a different value than DOS;
across the additive sweep, 17/249 cases diverged by up to ~15%, and Go's value
is frequently exactly the secant's initial guess `PayAmt/2`.

**Cause.** Go's solver drives the schedule's *final balance to zero*. Under
additive semantics the last scheduled payment settles whatever balance remains,
so final-balance-zero is satisfied by a whole *range* of prepayment amounts — the
objective is non-unique, and the secant accepts the first amount it tries that
already retires the loan (its initial guess). DOS's
`EstimateAndRefinePeriodicPrepayment` instead solves the discounted equation
`principal = PV(regular stream) + PV(prepayment stream)` — the unique amount at
which the loan amortizes *smoothly* with no settlement balloon on the final row.

**Scope.** Only the additive (plus_regular ON) unknown-*amount* solve. The
default (replace) unknown-amount solve is unique and matches DOS exactly.

**Closed.** `SolvePrepaymentAmount` now branches on `PlusRegular`: the replace
default keeps the unique final-balance secant; the additive case calls the new
`solvePrepayAmountAdditive`, a direct port of DOS's closed-form discounted-PV
amount (AMORTIZE.pas:670-699, including the tiny-rate branch). Validated to ~5e-8
against the real DOS engine.

### Gap B — unknown prepayment DURATION (AO10)

**Symptom.** Go's `SolvePrepaymentDuration` and DOS's `DeterminePrepaymentDuration`
diverge systematically (227/229 cases). Worked example — 100000 @ 8%, 360
monthly, regular payment 600 (below the 733.76 amortizing payment), +500/mo
additive: **DOS = 42, Go = 141.**

**Cause — different models, both self-consistent:**

- **DOS** is a *closed-form present-value duration* (AMORTIZE.pas:730-768). It
  credits the regular payment with its PV over the **full nominal term**
  (`firstdate..lastdate`): `600 × annuity(360) = 81,768`. It subtracts that (and
  any other extras) from the principal and solves the prepayment count whose PV
  covers the remainder: `500 × annuity(nn) = 18,232 ⇒ nn = 42`. It then sets
  `h^.nperiods := nn`.
- **Go** runs the schedule forward with the series effectively unbounded and
  counts prepayments until the **balance reaches zero** (both the regular and
  prepayment streams stop at payoff). Total 1,100/mo retires 100000 @ 8% in
  ~141 months ⇒ 141.

These compute genuinely different quantities, so the divergence is not a
tolerance issue.

**Note on authority.** Per `CLAUDE.md`, the DOS version is the authority for
financial logic, so DOS=42 is the reference a faithful port should reproduce —
even though the forward schedule a user would *see* (Go's 141) is the physically
intuitive payoff count. The right resolution and even whether DOS's PV-duration
is the desired user-facing behavior is a product decision, hence surfaced.

**Closed.** `SolvePrepaymentDuration` was rewritten as a direct port of DOS's
closed-form PV duration (AMORTIZE.pas:730-768), backed by a faithful port of
`NumberOfInstallments` into `internal/dateutil`. It credits the regular payment
over the full nominal term, subtracts balloons and other prepayments at their
discounted values, solves the last prepayment date, and counts installments in
`before` mode — matching DOS **exactly** (max |diff|=0 across the sweep). The
negative-duration guard ("principal more than covered by the fixed payments")
is reproduced. DOS also sets the loan term to the solved count, but Go leaves the
nominal term intact after the solve (the displayed schedule is unaffected for the
common bounded-series case); the solved count and stop date are what callers read.

Two dispatch points were adjusted so the closed-form solve fires only where DOS
does: (1) the unkpre duration branch sits behind the loan-term solve in DOS, so
Go's AO10 fires only when the loan term was known on input, and the internal
term-solve (`solveFancyTermFromPayment`) bounds unbounded prepayments on its
clone so they run to payoff rather than re-triggering the duration solve.

### What's wired now

- Oracle: `presolve=START:NN:PERYR` (solve amount) and
  `predur=START:PERYR:AMOUNT` (solve duration) in
  `legacy/oracle/amort_oracle.pas`; two benign GUI prompts on these paths
  (`DA_SetBalloonIncPmt`, `DA_SetBalloonIncludesToNo`) are answered
  deterministically in the headless `MessageBoxWithCancel` stub
  (`legacy/oracle/Globals.pas`) instead of aborting.
- Tests: `TestDOSPrepaymentAmountSolveSweep` (asserts replace strictly; logs the
  additive gap) and `TestDOSPrepaymentDurationSolveSweep` (logs the duration
  gap). Both stay green; the gaps are recorded in the test logs and here.
