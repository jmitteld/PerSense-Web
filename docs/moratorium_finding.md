# Moratorium per-row validation vs DOS — bug found and fixed (2026-06-10)

A per-row differential sweep of the three remaining fancy options against the
real DOS engine (`TestDOSFancyOptionsSweep`, via new `mor=`/`targ=`/`skip=`
oracle tokens) found:

| Option | Result |
|---|---|
| **Target** (min principal reduction) | matches DOS, 0 divergences |
| **Skip-months** | matches DOS, 0 divergences |
| **Moratorium** (interest-only deferment) | **bug found + fixed** (below) |

## Bug — moratorium re-amortized a user-given payment

**Symptom.** With a moratorium and a **given** regular payment, Go's schedule
diverged from DOS immediately after the interest-only period: Go paid the loan
down faster (its post-moratorium payment was higher), so balances drifted by up
to ~14x relative error.

**Cause.** At the first repayment date the Go fancy engine *unconditionally*
re-solved the regular payment over the remaining periods. DOS only does this when
the payment is being **solved** (left blank). When the user **gives** the
payment, DOS uses it as-is through the moratorium — the interest-only periods
simply defer principal, and any residual rolls to the end. (Verified against the
DOS engine: with the 29-period payment supplied, DOS pays just the interest for
the deferred months, then resumes the *same* given payment.)

**Fix.** Gate the post-moratorium recompute on the payment being blank
(`loan.PayAmtStatus < InOutDefault`). When the payment is given, it is kept.

After the fix the moratorium sweep matches DOS to ~1.5e-6 across 198 cases, and
the blank-payment moratorium tests (AM_EX13 and friends) — which exercise the
solve path the recompute is for — continue to pass unchanged.

This is the second DOS-fidelity correction surfaced in the fancy-option testing
campaign (the first was the payment-only ARM implied-rate clamp; see
`docs/arm_adjustment_findings.md`). Both only change results for inputs that hit
the specific mishandled path, and both are now pinned by differential sweeps
against the real DOS engine.
