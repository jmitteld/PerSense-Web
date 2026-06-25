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
- **M2 — unify the base solve into `solveUnknownByIterate`**; fold the moratorium boundary into the
  same segment machinery and retire the `armDuringMoratorium` special-case. Target: the remaining
  `*mor*` and `skip×balloon×*` stacks. ~2 days.
- **M3 — fuzzer to zero at N≥1000**, delete remaining heuristics, flip the default, update docs. ~1 day.

Definition of done: `PERSENSE_FUZZ_N=1000 PERSENSE_FUZZ=1 go test -run TestDOSOptionCubeFuzz` reports
**0 divergences**, full suite green, and the three deep regression tests still pin their goldens.

## 7. Why it's worth it

Today the engine is DOS-faithful on every single option and every realistic two-option pair — which
covers essentially all real client worksheets. The refactor buys **provable** fidelity across the
entire option cube (the fuzzer becomes a green CI guard instead of an opt-in meter), and it *removes*
code: the five dispatch strategies and three re-amortization heuristics collapse into one solver that
is a line-for-line analogue of the authority. That is the right end state for a faithful port.
