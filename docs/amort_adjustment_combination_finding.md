# Amortization — rate adjustment co-occurring with another advanced option (2026-06-10)

## What was found

The path-to-99 amortization **combination** differential (`TestDOSFancyCombinationSweep`,
driving several advanced options at once per-row vs the real DOS engine) surfaced
a divergence that the single-option sweeps never could: when a **rate adjustment
(ARM)** co-occurs with **another option** — a balloon, skip-months, or target —
the Go schedule diverges from DOS *after the adjustment fires*. Combinations that
do **not** include an adjustment are bit-faithful.

Concretely:

| Combination | Per-row result vs DOS |
|---|---|
| balloon + skip-months | 0 divergences ✓ |
| balloon + moratorium | 0 divergences ✓ |
| moratorium + balloon + skip (triple) | 0 divergences ✓ |
| **balloon + adjustment** | diverges after the adjustment row |
| **adjustment + skip** | diverges after the adjustment row |
| **adjustment + target** | diverges (incl. some row-count differences) |

This is **not** a regression in the single-option paths: a rate adjustment *on
its own* is per-row bit-faithful (`TestDOSAdjustmentPerRowSweep`, 0 fails), as is
each of balloon / skip / moratorium / target on its own. The gap is purely in the
**interaction** — how the post-adjustment payment re-amortization composes with a
co-occurring option.

## Concrete reproducer

100000 @ 6%, 60 monthly payments, given payment 1933.28, with a rate adjustment
to 9% at month 15 and a 10000 balloon at month 30. Rows match exactly through the
adjustment month (15); at month 16 onward the balance diverges:

```
row15: bal Go=77731.77  DOS=77731.77   (adjustment month — still equal)
row16: bal Go=76509.59  DOS=78314.76   (interest equal at 582.99; balance splits)
row30: bal Go=50209.77  DOS=76950.96
```

The interest at row 16 is identical (both apply the new 9% to the same balance),
so the rate change itself is applied consistently. What differs is the **principal
applied after the adjustment** — i.e. the re-amortized regular payment when a
balloon (or skip/target) is also present. DOS's post-adjustment balance stays far
higher, indicating its re-amortization composes with the co-occurring option
differently than the Go port's.

## Root cause (investigated 2026-06-10) — DOS's behaviour here is *degenerate*

Driving the real DOS engine in detail mode for the reproducer above shows what
DOS actually does after the adjustment, and it is **not a clean amortization**:

```
row 4:  int 396.34  prin  1536.94  bal 77731.78   (last row before the adjustment)
row 5:  int 582.99  prin  -582.99  bal 78314.77   (rate now 9% — payment ≈ 0; balance GROWS)
row 6:  int 587.36  prin  -587.36  bal 78902.13
 ...    (negative amortization for the entire rest of the term)
row 59: int 711.44  prin  -711.44  bal 95569.80
row 60: int 716.77  prin 95569.80  bal     0.00   (a single TERMINAL payment retires everything)
```

After the rate-only adjustment, DOS's re-amortized regular payment collapses to
**~0** (each row's principal is the negative of the interest, so the balance
negative-amortizes upward), and the whole balance is cleared by one giant
**terminal payment** at the end. This is the classic signature of DOS's `Iterate`
routine (AMORTOP.pas:1415, the finite-difference Newton it calls from
`Re_Amortize` when a balloon/prepayment co-occurs) **failing to converge** and
leaving a degenerate near-zero payment plus a terminal balloon.

The Go port, by contrast, produces a **clean, steadily-amortizing** schedule for
the same inputs (a positive re-amortized payment, balance declining to zero at
term). So in user-meaningful terms the Go result is arguably the *better*
behaviour; the DOS result is a non-convergence artifact of an uncommon
ARM-plus-balloon combination.

This reframes the gap: it is **not** "Go is wrong, DOS is right." It is an
ambiguous edge case where *bit-fidelity to DOS would mean replicating DOS's
`Iterate` non-convergence* (the near-zero payment + terminal balloon), on a
validated forward code path, for an advanced and uncommon input combination.

## Status and recommendation

- **Validated (new):** co-occurring advanced options *without* an adjustment —
  including a moratorium+balloon+skip triple — are now per-row bit-faithful vs
  DOS. This is genuine new coverage beyond the prior one-option-at-a-time sweeps.
- **Open gap:** the ARM-re-amortization-with-co-occurring-option path. It is
  documented here and pinned by `TestDOSFancyAdjustmentCombinationGap`, which runs
  the divergent combinations and records the divergence counts (it does **not**
  fail the build — the gap is surfaced, not hidden).
- **Why NOT auto-"fixed":** the root-cause investigation above shows DOS's result
  is itself **degenerate** (an `Iterate` non-convergence → near-zero payment +
  terminal balloon), while the Go port produces a clean amortization. "Matching
  DOS" would therefore mean *deliberately reproducing DOS's non-convergence
  behaviour* on a validated forward code path (the per-period adjustment handler
  in `engine.go`, which is currently bit-faithful for every adjustment case that
  does NOT co-occur with a balloon/skip/target — `TestDOSAdjustmentPerRowSweep`,
  0 fails). Changing that path risks regressing the validated single-option
  behaviour to chase a degenerate edge case. Per the project's standard, this is
  surfaced for an explicit decision rather than changed speculatively.

- **Note on the `Iterate` port:** the backward solvers (`SolveLoanAmount`,
  `SolveRate`) do **not** need an `Iterate` port — they already use a robust
  bisection-on-over/under-amortization-sign (`fancybisect.go`) and pass their
  round-trip + 400-case property sweeps. The only place DOS's `Iterate` matters is
  this forward post-adjustment re-amortization, and (per above) reproducing it
  means reproducing a non-convergence artifact.

**Client decision required.** Three options, in order of recommendation:
1. **Keep the Go behaviour** (clean amortization) and document this as an
   intentional improvement over a DOS non-convergence edge case. Lowest risk; the
   Go result is the more useful one for a user. *Recommended.*
2. Treat it as out of scope (ARM + balloon/skip/target on one loan is rare) and
   leave it documented as a known, surfaced difference.
3. Only if exact bit-fidelity to the original is contractually required: port
   DOS's `Iterate` *including* its non-convergence fallback into the forward
   adjustment handler, gated so it cannot affect the validated single-option
   paths, then extend `TestDOSFancyCombinationSweep` to assert the combinations.
   Highest effort and risk, to replicate a degenerate result.

Until a decision is made, this holds amortization (fancy options) at 96 (just
below the bit-identical tier). It does **not** affect any single-option or
non-adjustment-combination schedule, all of which are bit-faithful.
