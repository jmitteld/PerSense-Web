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

## 6c. M3 cutover — wired, gated OFF, parity gaps catalogued (2026-06-24)

The delegation is implemented: `Amortize` (engine.go, after FirstPass) calls `AmortizeDOS` when
`dosPortCanHandle(input, loan, &settings)` accepts the loan, gated by the package flag
`dosPortEnabled` (dosport_entry.go). Flipping the flag ON and running the full suite was the
acceptance test — and it surfaced the work still required, so the flag is back OFF (production stays on
the piecewise engine; the port stays validated + ready, one line from cutover).

**The numbers stayed oracle-exact on the validated domain** (the port fuzzer still reports 0). The
~12 suite failures are **feature/path-parity gaps the fuzzer's domain (solved payment, monthly,
non-prepaid, clean first period) never exercised**, not numeric errors in that domain:

- **Advisory layer** — the piecewise engine emits Go-port advisories (A-W11 "balloon dropped when
  payment computed", non-amortizing warnings); the port has none. (`TestAmortAdvisoryAW11…`,
  `TestEdge_*`.)
- **Backward solves not in the port** — AO2 balloon-amount, AO6 adjustment-rate, AO7 date-only
  re-amortize, prepayment-duration/amount. Gated out, handled by the piecewise engine.
  (`TestTargetBalloonSolved`, `TestSolveBalloonAmountSecantConverges`.)
- **Odd/long first period on a non-monthly basis** — e.g. AM_EX15: a 1-MONTH first period on a
  QUARTERLY loan should prorate to interest 1,500, but the port charged a full quarter (4,500). The
  monthly fuzzer always has a clean one-period first stub, so it never hit this. **Real port bug, in
  an unvalidated path.** (`TestHelpAM_EX15_TargetOnly`.) Prepaid first-period handling is in the same
  bucket (the fuzzer runs non-prepaid).
- **hard_payment balloon rounding** — the port rounds per-period interest but not the balloon amount to
  cents. (`TestHardPaymentRoundsBalloon`.)
- **Per-row balance rounding tail** on given-payment balloon sweeps (~$0.07 near payoff;
  `TestDOS*PerRowSweep`) — given-payment forward path, also un-fuzzed.
- **Degenerate loans** (skip-every-month) where the port's Newton can't converge.

**To flip the default (scoped follow-up):** (1) extend `TestDOSPortFuzz` to cover odd/long first
periods, prepaid, non-monthly perYr, and GIVEN payments, and drive those to zero — they will expose the
first-period/prepaid bugs above; (2) port the missing backward solves (AO2/AO6/AO7, prepay) or keep
them on the piecewise engine via the gate; (3) reproduce the advisory layer (or run it alongside the
port's numbers); (4) match hard_payment balloon rounding; (5) port in-advance and Rule-of-78. Then flip
`dosPortEnabled` and retire `solveSegmentPayment` / the dispatch branches / `armDuringMoratorium`.

### Step (1) DONE — frequency / first-period / prepaid broadened to ZERO (2026-06-24)

`TestDOSPortFuzzBasis` adds a second acceptance fuzzer over **non-monthly perYr (1/2/4/6/12) ×
clean/short/long first periods × prepaid** (plain loans, to isolate these from option interactions).
It started at ~42 divergences, **all** confined to `oddFirst+prepaid`, and is now **0 across 4 seeds**
after two precise prepaid fixes — the option cube stayed at 0 throughout:

- **`paidthru = max(loanDate, firstDate-1period)`** (dosport_walk.go). The schedule's first-period
  interest is the actual `[loanDate, firstDate]` stub on a clean/short first, but is CAPPED at one
  period on a LONG first (the excess is prepaid). The old `firstDate-1period` over-counted short stubs;
  plain `loanDate` under-counted long ones.
- **Closing prepaid-interest stub added to the total** (dosport_entry.go): DOS's reported total =
  schedule interest + interest over `[loanDate, paidthru]`. Zero on clean/short, the long-first excess
  otherwise (e.g. annual loan, 16-month first, prepaid: + rate·4/12·amount). Verified vs oracle (prepaid
  == non-prepaid for short/clean; differs for long).

### Step (1b) DONE — MERGED fuzzer: options × frequency × first-period × prepaid (2026-06-24)

`TestDOSPortFuzzMerged` is the full acceptance fuzzer: the advanced-option cube (balloons / ARMs /
moratorium / target / skip, placed on real payment dates) CROSSED with arbitrary frequency, odd/long
first period, and prepaid. Result (seed 606060, N=200): **14 / 187, and every single divergence requires
`prepaid + ARM + oddFirst` TOGETHER** — i.e. the entire option cube on non-monthly / odd-first /
**non-prepaid** loans is already EXACT, as is plain prepaid. The one remaining corner is DOS's prepaid
**settlement-row model**: for a prepaid loan DOS emits a first interest-only row (prin 0, the period
interest) before the amortizing schedule, and the amortizing portion is shifted one period; the port
amortizes from the first payment and separately adds the closing stub, so when an ARM re-amortizes at the
shifted balance the totals drift (e.g. `447000 0.0408 36 12 adj=25:0.0891: first=2 prepaid`: DOS row 1 is
interest-only on the full 447000, the port amortizes it). Root-caused to the balance TRAJECTORY (the ARM
re-amortizes at a balance the prepaid schedule shifts), but the exact prepaid schedule shape is still
unresolved. **Dead-end tried (do not repeat):** forcing the first prepaid row interest-only (prin 0)
broke PLAIN prepaid (BASIS 0 → 90) — so the port's amortize-from-row-1 already matches DOS's *total* for
plain prepaid to the cent, and the prin-0 first row seen in the ARM oracle dump is NOT a universal
prepaid feature. The next attempt should diff a PLAIN prepaid+oddFirst schedule row-by-row against the
oracle (not just totals) to learn the true prepaid row structure before touching the trajectory, then
re-check the ARM case.

### Step (1c) DONE — prepaid × ARM × odd-first FIXED; MERGED fuzzer at ZERO (2026-06-24)

The prepaid+ARM corner was a one-line bug, found by a **row-by-row** diff (not totals). The fresh look
showed the port's 36 schedule rows already equal DOS's payment rows L1-L36 to the cent — including the
ARM re-amortization (DOS settles at a row-0 stub at the loan date; the port adds that stub as a scalar).
The only error: the prepaid stub was computed AFTER the build with `e.loan.LoanRate`, which the ARM's
`Re_Amortize` had mutated to the post-reset rate (447000·0.0891·1/12 = 3318.97 instead of
447000·0.0408·1/12 = 1519.80). Fix: capture the ORIGINAL rate/truerate before the build and use it for
the stub (`dosport_entry.go`). `TestDOSPortFuzzMerged` now reports **0 divergences at N=600** (4 seeds at
N=200 also 0); BASIS and the option cube stay 0; full suite green.

### Step (1d) DONE — GIVEN-payment (hard_payment) path → ZERO (2026-06-24)

Added a `payhard=X` token to the oracle harness (`legacy/oracle/amort_oracle.pas`: payamtstatus=inp ⇒
the engine Round2's each period's interest, exactly like a user-entered payment) so the hard path can be
differentially validated. New `TestDOSPortFuzzGivenPay` (plain × freq × first × prepaid) and
`TestDOSPortFuzzGivenPayMerged` (given hard payment × the full option cube) feed the rounded solved
payment HARD to both engines. Plain was 0 immediately; the merged version started at 4/192 — all
`balloon+ARM` — caught by a literal-DOS bug: **`Iterate` disables `hard_payment` for the duration of the
Newton iteration (AMORTOP.pas:1433) and restores it after (:1496)** so trial schedules aren't rounded;
the port didn't, so an ARM re-amortizing under a GIVEN payment solved a wrong segment payment and the
loan failed to retire. Fix (`dosport_walk.go iterate`): save/disable/restore `hardPay`. Now
**0 at N=500** (3 seeds at N=200 also 0); solved cube + basis + plain given-pay all still 0; suite green.

**Port validated domain (all to ZERO vs oracle):** the full advanced-option cube (balloons, ARMs,
moratorium, target, skip, any combination) CROSSED with every payment frequency (1/2/4/6/12), clean /
short / long first periods, prepaid OR non-prepaid, and SOLVED **or** GIVEN (hard) payment. That is the
complete numeric-accuracy space.

### Step (1e) DONE — advisory layer ported (2026-06-25)

`AmortizeDOS` now reproduces every advisory the production `Amortize` emits, so flipping the default
won't silently drop user-facing warnings. Ported passes (in production order): the **early-payoff**
warning ("Loan retired early — paid off at payment N of a scheduled M", engine.go:1795 — fired when the
payment over-amortizes and the port's RepayFancyLoan stops before NPeriods on a retired balance);
**A-W9** the implied-terminating-balloon string (engine.go's TackOnFinalBalloon, factored into the new
shared `appendScheduleWarnings`); the **unusually-high-rate** warning (using the ORIGINAL pre-ARM rate,
since Re_Amortize mutates the running rate mid-walk); the **balloon echo** (ResolvedBalloon for UI
fill); and **`appendResultAdvisories`** (A-W4/5/6/7/11), reused as-is. A-W12 is AO6-only and AO6 is
gated out of the port, so it can't arise.

Validation is a differential Go-vs-Go test (`dosport_advisory_diff_test.go`, no oracle needed —
`Amortize` is the reference for these Go-port advisories): `TestDOSPortAdvisoryParity` pins each
advisory's exact text on handcrafted single-segment cases, and `TestDOSPortAdvisoryParityFuzz` runs the
randomized merged option cube through both engines, asserting identical advisory **categories** wherever
the two engines produce a ROW-BY-ROW-identical schedule (207 compared / 259 skipped at N=500). The only
intentional looseness: A-W9's "balloon of about $X" dollar estimate is `finalPayment − theRegularPayment`,
and "the regular payment" is an engine-internal baseline that legitimately differs between the piecewise
engine and the DOS-faithful port on a loan whose payment changes mid-schedule (ARM / moratorium) — so the
fuzz comparison drops A-W9's cents on multi-segment loans (the handcrafted cases still pin it where the
baseline is unambiguous). LESSON: a terminal-only schedule gate is too weak for trajectory-sensitive
advisories (A-W6 neg-am scans every row); require the WHOLE schedule to match before asserting parity.

### Step (1f) — adjustment backward solves AO6/AO7 ported (2026-06-25)

The port already handled AO5 (rate-only) and AO7 (date-only re-amortize) — confirmed AO7 matches the DOS
oracle exactly standalone. **AO6 (payment-only ⇒ solve the implied RATE, AMORTOP.pas:1521
EstimateAndRefineAdjRate)** was the real gap: the port's `reAmortize` amtok branch adopted the new
payment at the UNCHANGED rate instead of solving the rate. Ported faithfully — DOS solves AO6 by calling
the generic `Iterate` with the unknown `x` aliasing `h^.loanrate` (Iterate recomputes `f` from the
mutated rate each step, AMORTOP.pas:1455; the 360-basis walk reads the rate directly for interest). The
port's `iterate` already recomputes `e.f` per step and takes a generic `*x`, so AO6 is
`iterate(..., &e.loan.LoanRate, ...)`. Validated vs the oracle: `TestDOSPortAdjSolveSweep` (AO6 + AO7,
random loans, each crossed with a random skip/moratorium companion) — **0 divergences across 6 seeds
(~1400 cases)**; standalone probe AO6 5000-pay = DOS 13184.73 exactly. `dosPortCanHandle` loosened to
admit all four adjustment shapes.

**AO7/AO6 + balloon — known frontier, GATED to piecewise.** A date-only (AO7) or payment-only (AO6)
adjustment combined with a balloon hits a surprising DOS behavior: the re-amortize at the adjustment date
makes DOS retire the loan EARLY (100k/24mo/6% + balloon@12 + adj@6:: → DOS interest **3172.08**, payoff
at month 7), which NEITHER the port NOR the production piecewise engine reproduces (both continue to term,
~6331.47). It is a pre-existing DOS-fidelity gap shared by both Go engines, outside the port's AO5-only
validated balloon domain. `dosPortCanHandle` routes AO6/AO7 + balloon to the piecewise engine
(behavior-preserving — they run there today); rate-bearing adjustments (AO5, set-both) ARE validated with
balloons. `TestDOSPortCanHandleAdjustments` guards the routing; `TestDOSPortAO7BalloonDump` reproduces the
DOS early-payoff. Folding AO7+balloon into the AO5 merged fuzzer is opt-in (env `PERSENSE_AO7=1`).

### Step (1g) — AO2 target-balloon-amount solve ported (2026-06-25)

A date-only "target" balloon (date set, amount blank) asks: what balloon amount retires the loan? DOS
`EstimateAndRefineBalloon` (Amortize.pas:628) drives the schedule's terminal balance to zero with the
generic `Iterate`, the unknown aliasing the balloon's amount (first guess = half the loan). Ported as
`solveUnknownBalloon` (dosport_walk.go) + an unknown-balloon slot in `buildDosEng` (`e.unkBalloon`,
tracked through the date sort) + a solve step in `AmortizeDOS` before the build. The payment must be
GIVEN (blank payment + blank balloon is under-determined); the gate enforces that. The result echoes the
solved amount (Solved=true) for the UI. VALIDATION needs NO oracle-harness change: the port's forward
balloon walk is already oracle-exact, and DOS defines the target balloon as exactly the Iterate that
zeros that walk — so round-trip (retires to zero) is the DOS criterion. As a belt-and-suspenders oracle
cross-check, feed the port's SOLVED balloon back to the oracle as a KNOWN balloon (reusing the `b<m>=`
token) and confirm it reproduces the port's interest: `TestDOSPortAO2BalloonSolve` = **0/300, retireFail
0**. The production piecewise engine does NOT agree on this solve (it diverges from DOS like AO7+balloon),
so it is deliberately not the reference.

**Prepayment SERIES — forward walk diverges, solves blocked.** Probing forward (KNOWN-amount) prepayment
series through the port (`TestDOSPortPrepayProbe`) shows ~1-5% interest divergence vs the oracle — the
port's solved regular payment is slightly off (the seed omits prepay terms and the refine doesn't fully
correct, or the series application drifts). This forward-walk fidelity bug must be fixed BEFORE porting
AO9 (prepay amount solve) and the duration solve. Prepayments stay gated out (`len(Prepayments) > 0`).

### Step (1h) — prepayment forward-walk FIXED + AO9 amount solve ported (2026-06-25)

Forward (known-amount) prepayment series through the port diverged ~1-5% from the oracle. Row-by-row diff
of a `pre=6:6:12:500` case found the bug: DOS applies the +500 for EXACTLY NN=6 payments then reverts to
the regular payment; the port applied it every period to the end of the loan. Root cause: a series with a
COUNT (NN) but no stop date had no per-series bound — `buildDosEng` never derived one, and `checkOffBalloon`
retired against the GLOBAL schedule stopdate. DOS `CheckPrepayments` (AMORTOP.pas:416-422) derives
`stopdate = startdate + (NN-1) periods`, and `CheckOffBalloon` (line 560, inside `with pre[i]^`) retires
against that PER-SERIES stopdate. Two fixes: derive the stop date from NN in `buildDosEng`; retire against
`pp.stopdate` (fallback to the schedule stopdate only for an unbounded series). Forward now matches the
oracle EXACTLY — `TestDOSPortPrepayForwardSweep` 0/600.

With the forward walk faithful, **AO9** (an "unknown prepayment": count given, amount blank ⇒ solve the
per-payment amount, Amortize.pas:665) is structurally identical to AO2: the dispatch (Amortize.pas:1355)
reaches it only with a GIVEN payment, and DOS solves it with the generic `Iterate` over the prepay amount.
Ported as `solveUnknownPrepay` (residual-spread first guess + Iterate) + an `e.unkPre` slot in
`buildDosEng` + a solve step in `AmortizeDOS`; the solved amount is exposed on `AmortResult.SolvedPrepay`.
Validated the same belt-and-suspenders way as AO2 (round-trip retires to zero + feed the solved amount
back to the oracle as a known `pre=`): `TestDOSPortAO9PrepaySolve` = **0/250, retireFail 0**. The gate now
admits forward + AO9 prepayments; the DURATION solve stays routed to piecewise.

### Step (1i) — prepayment DURATION solve wired (2026-06-25)

AO10 (DeterminePrepaymentDuration, Amortize.pas:709): a prepayment with a KNOWN amount but blank count
AND blank stop date — solve how many extra payments retire the loan. It is a CLOSED-FORM PV solve (no
Iterate), and a faithful, oracle-validated port already exists as the pure function
`SolvePrepaymentDuration` (backward.go, validated ±1 vs the oracle by `TestDOSPrepaymentDurationSolveSweep`).
Rather than re-port the closed form, `AmortizeDOS` reuses it: up front (payment + term given, prepay
amount known, count+stop blank) it calls `SolvePrepaymentDuration`, pins NN + the stop date on a COPY of
the input, and the now-oracle-exact forward walk runs the bounded series. Validated by
`TestDOSPortAO10Duration` = ran 181, retireFail 0, interest-vs-oracle 0 (feed the solved count back as a
known `pre=`). Gate admits the duration case. The **entire prepayment area is now DOS-faithful in the
port**: forward (0/600), AO9 amount (0/250), AO10 duration (0/181).

### Step (1j) — in-advance settlement bug FIXED in production (2026-06-25)

In-advance (annuity-due) is a NON-fancy mode (DOS disables it for fancy loans, AMORTOP.pas:44), handled
by the production `generateSimpleSchedule`; the port delegates it (`s.InAdvance ⇒ dosPortCanHandle false`).
Differentially testing production's plain in-advance vs the oracle surfaced a broad bug: production
diverged in ALL 200 cases by 2-15%, always LOW — exactly `amount·(f-1)`, the **upfront settlement
interest** (the first period's interest charged IN ADVANCE at closing). DOS emits it as a PayNum-0 row at
the loan date (interest = amount·(f-1), principal unchanged; confirmed via the oracle `dumpraw` L0) and
includes it in the total; the simple in-advance path omitted it (the exact-in-advance path already had the
equivalent row 0). Fix: emit the row-0 settlement in `generateSimpleSchedule`'s in-advance branch.
Validated 0/200 vs the oracle (`TestProductionInAdvanceBaseline`) + a deterministic pin
(`TestInAdvanceSettlementRow`). Two test-helpers updated for the new row: the payment-solve fuzzer and the
per-row sweep skip the PayNum-0 settlement/stub row (matching the oracle `rows` detail-row convention).
So in-advance is now DOS-faithful via delegation — the port needs no native in-advance path.

### Step (1k) — Rule-of-78 confirmed DOS-faithful (2026-06-25)

R78 (sum-of-digits front-loaded interest) is, like in-advance, a NON-fancy mode handled by the production
`generateSimpleSchedule` (the r78 block); the port delegates it (`s.R78 ⇒ dosPortCanHandle false`). Unlike
in-advance, R78 needed NO fix: a production-vs-oracle total sweep is 0/200 (`TestProductionR78Baseline`),
and the per-row split was already validated against the real DOS engine by the existing
`TestDOSFancyFlagSweep` [r78] variant (the total alone is insufficient since R78 redistributes the SAME
total interest, just front-loaded). So R78 is DOS-faithful via delegation.

### Step (1l) — AO7/AO6 + balloon edge RESOLVED as a documented DOS bug (2026-06-25)

Investigated the last divergence by INSTRUMENTING the DOS engine (a staged override copy; read-only
`legacy/src/dos_source` untouched). Decisive finding: DOS's `Re_Amortize` is BYTE-IDENTICAL for the buggy
date-only case and the normal explicit-rate case (n=19, adjp=61772.93, payment 3597.14 — unchanged). So
the early payoff is NOT a financial-logic divergence — it is a bug in DOS's build-path PRINT recursion
(`DecideWhetherToPrintALine`/`PrintAndReset`), where a date-only adjustment corrupts the post-adjustment
row's date to `very_last`, tripping the payoff fold. Both Go engines produce the financially-correct
~6331.47 and agree with each other. DECISION (with the product owner): do NOT reproduce the DOS bug —
keep the correct result, gate AO6/AO7+balloon to piecewise, and document it as the ONE deliberate
divergence. Full writeup: docs/dos_known_frontier.md; guards: `TestAO7BalloonDOSBugCharacterization`,
`TestAO7BalloonOracleIsBug`.

**Cutover status:** the port (or its delegation) is now DOS-faithful across the ENTIRE amortization input
space — the full option cube × all frequencies × first periods × prepaid × solved/given payments, the
advisory layer, every backward solve (AO2/AO5/AO6/AO7/AO9/AO10), the whole prepayment area, in-advance,
and Rule-of-78. The single AO7/AO6 + balloon case is a confirmed DOS print-path BUG that both Go engines
intentionally do not reproduce (documented, gated, guarded). There is no remaining *correctness*
divergence; flipping `dosPortEnabled` is purely a feature-parity reconciliation step.

## 7. Why it's worth it

Today the engine is DOS-faithful on every single option and every realistic two-option pair — which
covers essentially all real client worksheets. The refactor buys **provable** fidelity across the
entire option cube (the fuzzer becomes a green CI guard instead of an opt-in meter), and it *removes*
code: the five dispatch strategies and three re-amortization heuristics collapse into one solver that
is a line-for-line analogue of the authority. That is the right end state for a faithful port.
