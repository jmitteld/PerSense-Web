# ARM adjustment per-row validation vs DOS — findings (2026-06-10)

A per-row differential sweep of ARM-style rate/payment adjustments against the
real DOS engine (`TestDOSAdjustmentPerRowSweep`, 200 randomized cases per mode,
driven through the new `adj=MONTHS:RATE:AMOUNT` oracle token) covers the three
DOS dispatch arms (Amortize.pas:1408-1419):

| Adjustment | DOS behavior | Result |
|---|---|---|
| **rate-only** (`adj=M:0.08:`) | `EstimateAndRefineAdjPayment` (AO5): re-amortize the payment at the new rate | matches, 0 divergences (3.5e-6) |
| **payment-only** (`adj=M::1500`) | `EstimateAndRefineAdjRate` (AO6): solve the *implied rate* at which the new payment amortizes the remaining balance | **bug found + fixed** (below) |
| **combined** (`adj=M:0.08:1500`) | use both as given | matches, 0 divergences (5e-5) |

## Bug found and fixed — payment-only implied-rate solve clamped at zero

**Symptom.** For a payment-only adjustment, when the new payment is too *low* to
amortize the remaining balance at a positive rate (i.e. below the level that
would retire the loan over the remaining term even at 0% interest), DOS
re-computes a **negative** implied rate (it prints e.g. `re-computed at -12.3%`)
and the post-adjustment interest goes negative. Go diverged sharply (positive
interest), as if it had kept the old rate.

> **Direction note (corrected 2026-06-11).** An earlier draft of this finding
> described the trigger as an *over*payment. That is backwards: a payment *above*
> the zero-interest payoff level implies a *positive* rate and converges fine; it
> is an *under*payment — too small to amortize the balance over the remaining
> term — that forces the negative implied rate. Verified empirically (e.g. a
> ~$186k balance over ~300 periods needs ~$620/mo at 0%; a $400/mo new payment
> yields ~−$507 first-period interest). The fix and the differential result below
> are unchanged; only the description of *which* payments trigger it is corrected.

**Cause.** `solveAdjRate` (the AO6 secant in `backward.go`) clamped each trial
rate to `>= 0` (`if r2 < 0 { r2 = 0 }`). DOS's `Iterate` allows a negative rate
and only bounds `|rate| < 2` (AMORTOP.pas:1485). With the zero clamp the secant
stalled on any such (under-paying) payment, returned "not converged", and the
engine left the rate unchanged — so the schedule continued at the old rate
instead of the (negative) implied one.

**Fix.** Clamp the trial rate to `[-1.9, 1.9]` instead of `[0, ∞)`, matching DOS.
After the fix the payment-only sweep matches DOS to ~3e-6 across 199 cases,
including the negative-implied-rate cases. Moderate and high positive implied
rates (e.g. DOS's `re-computed at 30.88%`) already converged and were unaffected.

This is a genuine DOS-fidelity correction to the ported engine, surfaced here per
the standing rule. It only changes results for payment-only ARM adjustments whose
new payment implies a rate outside the previously-clamped range — most visibly
the under-paying (negative-rate) case.

## UI advisory (added 2026-06-11)

Because the negative-rate case produces negative interest rows that surprise
users, the engine now emits a non-blocking **Note (A-W12)** when an AO6 solve
returns a rate below zero: *"A payment-only adjustment set a new payment too low
to amortize the loan at a positive rate, so Per%Sense fit a negative interest
rate for the periods after it…"*. It changes no number — it only explains the
DOS-faithful result. Covered by `TestAO6NegativeImpliedRateAdvisory` (and a
positive-rate control) in `arm_negrate_advisory_test.go`.

## Note on test precision

The `combined` mode initially showed a sub-cent tail drift on long loans. That
was a test artifact: the oracle parsed the rate/amount from a decimal string
while Go used the full-precision float. Rounding both sides to the same decimal
string (`roundTok` in the sweep) removed it; it was not an engine difference.
