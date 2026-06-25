# Scope: port DOS's global `Iterate` for fancy amortization solves

**Status:** proposed. **Author:** engine fidelity work, 2026-06-24. **Driver:** the randomized
option-cube fuzzer (`TestDOSOptionCubeFuzz`) shows the residual DOS divergences are all
*stacked* advanced-option combinations (`mor+target+ARM`, `multi-ARM+skip`, `balloon+mor+target`,
`mor+ARM-after-window`). Point fixes have closed every realistic single- and two-option case; the
stacks need the original solver's architecture, not more heuristics.

## 1. Why point fixes plateau

The Go engine solves a fancy loan **piecewise**:

1. A dispatch (`engine.go`, the `loan.PayAmtStatus < InOutDefault` block) picks ONE strategy to solve
   the base payment — `solveFancyPayment`, `refineFancyPayment`, the strip-adjustments branch, the
   exact-daily `dosIteratePayment`, or a closed form.
2. `generateFancySchedule` then walks the schedule and **re-amortizes at each event** with a *local*
   rule: `estimatePayment` / `solveMoratoriumPayment` at the moratorium boundary, `annuityPayment` +
   `refineAdjustmentPayment` at each ARM, the skip/target/balloon handlers per period.

Each local rule is correct in isolation, and we have made them DOS-faithful one interaction at a time
(ARM+balloon, ARM+skip, balloon+multi-ARM, moratorium+balloon, moratorium+ARM-during). But the rules
**don't compose**: when two re-amortization events overlap, the quantity each local rule drives to zero
is not the *same* quantity — the whole schedule's terminal balance — so their corrections fight. The
fuzzer's residual 45/228 at N=240 is precisely this non-composition.

## 2. What DOS actually does (the target architecture)

DOS solves every fancy unknown with **one** Newton refinement over **one** schedule walk. Source map
(legacy/src/dos_source):

- **`Iterate(p, usap, loandate, firstdate, var x, entire_or_no)`** — `AMORTOP.pas:1415`. A
  finite-difference Newton method on a single scalar `x` (the payment, a balloon, the rate, or the
  loan amount — `x` is passed by reference and may alias `h^.amount`). Each step runs the whole
  schedule and reads the terminal principal `p`; it adjusts `x` by `delta*p/(final-p)` until
  `|p| < halfpenny` (or 20 iters / divergence guard). This ONE routine backs every solve:
  `EstimateAndRefinePayment` (`Amortize.pas:416`), `…LoanAmount` (`:460`), `…Rate` (`:477`),
  `…AdjPayment`/`…AdjRate` (`:324`/`:347`), and APR-with-points (`:516`).
- **`RepayFancyLoan(...)`** — `AMORTOP.pas:1101`. The schedule walk. `ComputeNext` advances one period
  (interest split, US-rule `usap`, balloons, prepayment series, skip months, target minimum,
  moratorium interest-only). At each adjustment date it calls **`Re_Amortize`** (`:1215`). The final
  scheduled payment absorbs residual principal via the `WhenToStop` fold (`:1208-1212`).
- **`Re_Amortize(p)`** — `AMORTOP.pas:1499`. At an ARM: set the new rate; compute the segment payment
  as the analytic annuity over `[adj date → last date]` (`:1547`,`:1565-1569`) minus **discounted
  future balloons** (`:1561-1563`); then refine with `Iterate(..., til_adj)` **only when balloons or
  prepayments are present** (`:1571-1587`) — otherwise the analytic value stands and the schedule's
  final payment balloons.
- **Moratorium** has no boundary recompute — it is just interest-only inside `ComputeNext`
  (`AMORTOP.pas:641-648`); the governing payment is whatever the base solve or a prior `Re_Amortize`
  set, computed over the full term, so the loan under-amortizes and the final payment balloons.
- **`entire` vs `til_adj`** — the walk can run to the end or stop at the next adjustment, so a solve
  can target either the whole-loan terminal or a single segment. The base-payment solve uses
  `til_adj` (`Amortize.pas:416`).

The single invariant: **the unknown is whatever makes the ONE forward walk land on zero terminal
principal**, with `Re_Amortize` and the moratorium computed *inline* during that walk. Because all
options live in the same walk, every combination composes by construction.

## 3. What we already have (reusable)

Good news: the hardest pieces exist and are DOS-validated.

- **The forward walk** — `generateFancySchedule` is already a faithful `RepayFancyLoan`/`ComputeNext`
  (balloons, prepay, ARM rate changes, skip, target, moratorium interest-only, the `WhenToStop`
  final-fold we added for ARM loans). It is exercised to the cent across the existing cubes.
- **A Newton/secant `Iterate`** — `dosIteratePayment` (exact-daily path) already ports the
  finite-difference refinement; `interest.Iterate`-style helpers exist. `fancyOverUnder` already
  reconstructs the *unforced* terminal residual (the signal Newton needs) past the engine's forced
  final payment.
- **Analytic seeds** — `annuityPayment` + the discounted-balloon `netBal` (engine.go AO5 site) is
  exactly DOS's `Re_Amortize` analytic seed.
- **The oracle + fuzzer** — `legacy/oracle/amort_oracle` and `TestDOSOptionCubeFuzz` give a
  per-combination pass/fail signal; the refactor's definition of done is "fuzzer → 0 at N≥1000."

## 4. Proposed change

Replace the piecewise solve with a single segment-aware Newton, mirroring DOS:

1. **`reAmortizeInline`** — fold the AO5/AO6/AO7 logic into `generateFancySchedule` so that, during the
   walk, hitting an adjustment recomputes the segment payment exactly as `Re_Amortize` does: analytic
   annuity over `[adj→last]` − discounted future balloons, refined by an inner `Iterate(til_adj)` only
   when balloons/prepayments remain. (Most of this is already at the AO5 site; the change is to make it
   a single inline routine with the `til_adj` inner solve instead of the special-cased
   `refineAdjustmentPayment`.)
2. **`solveUnknownByIterate(field)`** — one entry point that drives payment / amount / rate to a zero
   terminal by Newton over `generateFancySchedule` (the existing `fancyOverUnder` residual). Retire the
   dispatch's per-strategy branches; they collapse into "seed analytically, then `Iterate`."
3. **Moratorium** — keep the interest-only walk; DROP the boundary recompute entirely (the
   `armDuringMoratorium` special-case and `solveMoratoriumPayment` both disappear — the single base
   solve + inline `Re_Amortize` produce the right governing payment, including the ballooned final).
4. **Keep** the closed forms as Newton *seeds* (fast, and they match DOS's analytic seeds), and keep
   the `WhenToStop` final-fold.

Net: one solver, one walk, no per-combination heuristics.

## 5. Risk & mitigation

- **Regression on currently-exact cases.** The cubes + per-feature tests + the three new
  regression tests (ARM+skip, multi-ARM+balloon, moratorium+ARM) are the guard. Run
  `go test ./...` plus the fuzzer at every step; the fuzzer must be **monotone non-increasing** in
  divergences. *Mitigation:* land behind a build flag / parallel function and diff the two engines on
  the fuzzer before switching the default.
- **Newton non-convergence** on pathological inputs (very long exact terms, near-zero rates) — DOS
  itself caps at 20 iters and shows "did not converge." Port that guard and surface the same advisory.
- **Performance** — Newton × full-schedule walk is O(iters × N). DOS does this fine; N≤600 here.
  Cache the analytic seed so iters stay ≈3–5.
- **`til_adj` segment semantics** are the subtle part (a solve can target a segment, not the whole
  loan). This is where a naïve "drive whole-loan terminal to zero" differs from DOS; the port must
  honor the segment target for intermediate `Re_Amortize` solves.

## 6. Estimate & sequencing

- **M1 — `til_adj` SEGMENT solve at each adjustment — DONE 2026-06-24.** `refineAdjustmentPayment`
  (which bisected the adjustment payment against the ENTIRE schedule terminal — ill-posed once a
  second ARM re-amortizes downstream, hence its single-ARM gate) was REPLACED by `solveSegmentPayment`,
  a sub-loan over `[adj → last]` at the current rate that ignores later adjustments — exactly DOS's
  `Iterate(..., til_adj)` with `adjnum=0` (AMORTOP.pas:1571/1215). The same helper now backs both the
  moratorium boundary and each ARM. Trigger stays balloon/prepay (DOS Iterates only for those; skip/
  target keep the plain annuity + ballooned final). Net: removed `refineAdjustmentPayment` (~60 lines)
  and the single-ARM gate. Fuzzer (N=200 ×3 seeds): **29→19, 32→28, 27→22**; all key regression tests
  + full suite green. Remaining divergences are now 4-to-6-way stacks dominated by `skip`
  co-occurring with `balloon`+(ARM/mor/target), plus `balloon+ARM2++mor`.
- **M2 — unify the base solve into `solveUnknownByIterate`** over the ACTUAL schedule; fold the
  moratorium boundary into it and retire the `armDuringMoratorium` special-case. Target: the remaining
  `*mor*` and `skip×balloon×*` stacks. ~2 days. **Investigation 2026-06-24 (below) confirms this is the
  irreducible architectural step — the residuals cannot be patched at the sub-loan level.**
- **M3 — fuzzer to zero at N≥1000**, delete remaining heuristics, flip the default, update docs. ~1 day.

### M2 investigation — why the sub-loan approach plateaus (the finding that scopes M2)

Tracing the post-M1 residuals (fuzzer 19/28/22 at N=200×3) pinned the mechanism precisely. The
segment solve (`solveSegmentPayment`, M1) builds a SUB-LOAN over `[adj → last]` and solves it to
retire. That sub-loan is self-consistent, but it is **not identical to the corresponding tail of the
ACTUAL schedule**, so its solved payment can be wrong for the real schedule. Worked example
(`balloon2++ARM`: $193k/240, 11.66%, b@m32 + b@m108, ARM→7.91%@m107 — verified vs the oracle):

| | base pmt (pre-ARM) | post-ARM pmt | result |
|---|---|---|---|
| DOS | 1772.04 | ~1492 | retires at full term (240), int 249,546.50 |
| Go (M1) | 1772.04 ✓ | **1829** | retires EARLY (row 206), int 230,636.96 |

The base payment matches DOS to the cent; the **segment** payment is wrong. The analytic seed is
~1492 (≈DOS), but `solveFancyPayment` returns **1829**: with a balloon in the FIRST period of the
segment (m108, one period after the ARM), `fancyOverUnder`'s terminal sign is non-monotonic across the
`[0.5·seed, 1.5·seed]` bracket, so the wide bisection escapes to a spurious root. DOS avoids this
because its `Iterate` is a **Newton seeded from the analytic annuity** and stays at the near root.

A natural guard — "accept the refinement only if the sub-loan retires at full term" — was implemented
and **had zero effect** (0/600 cases changed): the spurious 1829 retires the *sub-loan* at full term
yet over-amortizes the *real* schedule. The sub-loan and the real tail diverge (first-period interest
/ balloon-vs-segment-start alignment), so **no sub-loan-level check can close this**. The only fix is
to make the solve run over the REAL schedule: one Newton on the base payment, with each `Re_Amortize`
computed inline as the schedule walks (no sub-loans), driving the actual unforced terminal to zero —
i.e. a faithful port of DOS's `Iterate` + `RepayFancyLoan`, exactly the M2/M3 plan. Incremental
sub-loan patching is exhausted; M2 must be the structural change, landed behind the build-flag /
parallel-engine mitigation in §5 and validated by the fuzzer staying monotone non-increasing to zero.

Definition of done: `PERSENSE_FUZZ_N=1000 PERSENSE_FUZZ=1 go test -run TestDOSOptionCubeFuzz` reports
**0 divergences**, full suite green, and the three deep regression tests still pin their goldens.

## 6b. Build status — the faithful port exists (2026-06-24)

The structural port is implemented as a parallel, gated engine in four files:
`dosport.go` (records + `ComputeNext`/`FindNextExtra`/`CheckOffBalloon`), `dosport_walk.go`
(`RepayFancyLoan`/`Iterate`/`Re_Amortize` + `saved_balloon_state`), `dosport_entry.go`
(`buildDosEng` + the `EstimateAndRefinePayment` seed + the `AmortizeDOS` entry), and
`dosport_fuzz_test.go` (`TestDOSPortFuzz`, the acceptance gate diffing `AmortizeDOS` vs the oracle).
The production `Amortize` is untouched and remains the default.

**It already solves the cases the piecewise engine cannot.** `AmortizeDOS` matches the oracle TO THE
CENT on plain ($888.4879 / $661.85), single balloon ($568.4755 / $154,651.18), multi-ARM+balloon
($426,265.36 — the M2 worked example), and **`balloon2++ARM` ($249,546.50)** — the exact case the M2
investigation proved unfixable with sub-loans. That validates the whole approach.

**Two literal-fidelity details the port had to reproduce** (each found by oracle diff, not guesswork):

1. *No interest-only floor without a target.* `ComputeNext`'s unguarded `payamt<interest → interest`
   branch (AMORTOP.pas:643/649) would floor every low payment, but the oracle NEGATIVE-amortizes a
   low-payment balloon loan (balance grows, prin<0). So the effective no-target `targ.target` is −∞,
   not the literal 0 from `ZeroTarget`. (`dosport_entry.go`.)
2. *The last payment absorbs the whole balance.* `PrintAndReset` (AMORTOP.pas:1004-1009) folds the
   ENTIRE remaining principal into the payment landing on `very_last` — regardless of size, in the
   BUILD path only — which is why an ARM/skip schedule's interest matched but the balance would not
   retire without it. (`dosport_walk.go`.)
   A subtle Pascal-globals trap also had to be handled: the nested `Iterate` walk's per-period
   `SaveDataForReAmortize` clobbers the OUTER `Re_Amortize`'s `old_next_balloon`, so the saved state
   must restore the `old*` fields too — without it a SECOND ARM misses the balloon.

**Fuzzer status: ZERO DIVERGENCES — definition-of-done MET (2026-06-24).** `TestDOSPortFuzz` reports
**0 divergences at N=1000** (seed 20260624, 948 ran) and 0 across 8 independent seeds (~1,800 cases).
The path there: 39/38/37 → 22/23/28 (the −∞ target + the very_last fold) → **0/0/0** after the final
bug — `reAmortize` was passing a COPY of the payment to its inner `Iterate` instead of `&e.d`, so the
Newton never moved the payment the walk used and diverged → aborted at the first ARM on balloon×skip
stacks. DOS passes the global `d` by reference (AMORTOP.pas:1577); aliasing `e.d` fixed it and the whole
`balloon × ARM × skip` class collapsed to exact. The faithful port now reproduces the DOS engine across
the entire random advanced-option cube. Goldens pinned in `dosport_golden_test.go`.

**Remaining to make it the default (M3):** route the production `Amortize` fancy path through
`AmortizeDOS` behind a settings/flag, run the full existing regression + API suites against it, reconcile
any intentional wording/shape differences, then flip the default and retire the piecewise heuristics
(`solveSegmentPayment`, the dispatch branches, `armDuringMoratorium`). In-advance and Rule-of-78 still
fall back to the piecewise engine (the fuzzer does not generate them); port them before a full cutover.

## 7. Why it's worth it

Today the engine is DOS-faithful on every single option and every realistic two-option pair — which
covers essentially all real client worksheets. The refactor buys **provable** fidelity across the
entire option cube (the fuzzer becomes a green CI guard instead of an opt-in meter), and it *removes*
code: the five dispatch strategies and three re-amortization heuristics collapse into one solver that
is a line-for-line analogue of the authority. That is the right end state for a faithful port.
