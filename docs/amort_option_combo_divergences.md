# Amortization option-combination divergences vs the DOS engine

Found by the exhaustive UI case suite (`internal/api/amort_ui_cases_test.go`), then run
through the real DOS source engine (`legacy/oracle/amort_oracle`) for ground truth.
**Correction:** an earlier note guessed these residual tails were "likely DOS-faithful." They
are **not** — two specific combinations genuinely differ from DOS. A third case turned out to be
an oracle-setup artifact (now understood). All amounts: $100,000, 8%, 360 months, monthly, 360-day.

## Method note — balloon "plus regular" semantics (resolved)

The Go default `balloonIncludesRegular=false` means the balloon is **ADDED** to that period's
regular payment. The DOS internal flag is `plus_regular`, and `plus_regular=TRUE` = ADD. The DOS
UI setting *"Balloon includes regular pmt = NO"* (the default) maps to `plus_regular=TRUE` (the
balloon is *plus* the regular), i.e. **ADD** — matching Go. The oracle's CLI defaults to
`plus_regular=false` (REPLACE), so DOS comparisons must pass the `plusreg` flag to match Go's
default. (Verified below; the oracle's inline comments contradict each other — line 162 vs 569 —
the **runtime behavior** is authoritative: default=REPLACE, `plusreg`=ADD.)

## Control — single balloon matches DOS (no divergence)

`amort_oracle 100000 0.08 360 12 b120=50000 plusreg`  vs Go default (balloon $50k @ month 120):

| | payment | total interest | final balance |
|---|---|---|---|
| DOS (ADD) | 568.4755 | 154,651.18 | 0.00 |
| Go (ADD, default) | 568.4800 | 154,651.34 | 0.21 |

Match to the cent (Go's $0.21 terminal tail is rounding). **A balloon alone is DOS-faithful**, as
is an ARM alone and a skip set that excludes the first payment (verified separately).

## Divergence 1 — ARM rate-adjustment **combined with** a balloon  (S2) — **FIXED**

> **FIXED 2026-06-24.** Root cause: (a) the payment-solve dispatch had no branch for
> "balloon + adjustment," so it kept the balloon-blind plain seed (733.76) as the initial
> payment; (b) the AO5 re-amortization used only DOS's analytic *seed* and skipped the
> `Iterate` refinement DOS runs when balloons are present (AMORTOP.pas:1577 — the deferred
> "task #103"). Fix in `engine.go` (solve the initial payment balloon-aware with the adjustment
> stripped) + `fancybisect.go` (`refineAdjustmentPayment` Iterate-refines the re-amortized
> payment). Result: initial payment now **601.53 / 634.82** (matches DOS exactly), final balance
> **$0.00 / ~$50** (was $1,322 / $192), interest within **~$26–50** of DOS (was off $12k–$25k).
> Regression test: `internal/api/amort_arm_balloon_test.go` (fail-before/pass-after verified).
> (A follow-up refinement fix — reconstructing the terminal residual against the post-adjustment
> payment — later brought the two-balloon-spanning-the-ARM tail from ~$50 down to ~$2.60.)

Rate-only ARM adjustment + a balloon, solving the payment:

| case | engine | total interest | final balance |
|---|---|---|---|
| balloon $40k @m120 + ARM→9% @m48 | DOS+plusreg | **177,034.22** | **0.00** |
| | Go | **165,057.90** | **1,322.05** |
| two balloons $15k @m72/@m192 + ARM→7% @m132 | DOS+plusreg | **147,390.16** | **0.00** |
| | Go | **122,821.54** | **192.41** |

Total interest differs by **$12k–$25k** — far beyond rounding — and Go leaves a non-zero final
balance where DOS amortizes to exactly $0.

**Mechanism.** The two engines solve a different problem for "solve payment + mid-stream rate
change + a future balloon." Row trajectory for balloon+ARM:

- **DOS** solves a low single base payment (~$603) — negative-amortizing from the start — and at
  the ARM date raises it (~$669); the schedule, taken as a whole, retires the loan to $0 at the
  final row.
- **Go** keeps the plain 8% payment ($733.76) until the ARM, then re-solves to ~$604 — which is
  *below* the new interest, so the balance grows (negative amortization) after the ARM; the later
  balloon knocks it down but the loan never fully retires, leaving ~$1,322.

So Go's rate-only-ARM re-amortization (documented as "re-solve the payment over the remaining
term") does not reconcile with a balloon that occurs later: it re-solves a payment that
under-amortizes, and reports materially less total interest than DOS.

## Divergence 2 — skip set that includes the first payment month  (S2/S3) — **FIXED**

> **FIXED 2026-06-24.** Root cause in `fancybisect.go fancyOverUnder`: when the skip set
> includes the final period, the schedule's last row is a skipped month (payment $0), and the
> forced-final-payment correction `last.PayAmt - regular` subtracted a full payment that never
> happened — so the payment solve landed ~$1 low and left ~$1,104 owed over 360 rows. Fix: apply
> the correction to the last ACTUAL payment row, skipping trailing zero-payment skip rows. Result:
> regular payment **1105.34** (matches DOS to the cent), final balance **$0.87** (was $1,104),
> **359 rows** (matches DOS, was 360), interest within **$0.66**. A skip set that excludes the
> first payment (e.g. `6-8`) is unchanged. Regression test:
> `internal/api/amort_skip_firstpay_test.go` (fail-before/pass-after verified).

`skip = 1-3,7` (Jan–Mar + Jul). The first scheduled payment (Feb) is itself a skipped month.

| engine | payment (regular) | total interest | rows | final balance |
|---|---|---|---|---|
| DOS | 1,105.34 | 165,281.17 | 359 | 0.00 |
| Go | (row 1 = $0, Feb skipped) | 166,117.24 | 360 | 1,103.84 |

DOS produces a 359-row schedule that amortizes to $0; Go produces 360 rows and leaves ~$1,104,
with ~$836 more total interest. A skip set that **excludes** the first payment month (e.g. `6-8`)
matches DOS to within rounding — so the divergence is specific to skipping the first payment.

## Divergence 3 — moratorium **combined with** a balloon  (S2) — **FIXED**

> **FIXED 2026-06-24.** Found by the standing tracker (below); same bug family as Divergence 1.
> Root cause: when a moratorium is present, the post-moratorium payment re-amortization
> (`engine.go` moratorium branch, `d = estimatePayment(...)`) was **balloon-blind**, so the loan
> over-paid and retired early. The DOS source settles it decisively: DOS does **not** re-amortize
> at the boundary — it solves the loan as a **single** payment that drives the whole schedule (the
> moratorium interest-only periods included) to a zero terminal, so that payment is inherently
> balloon-aware (interest-only handling at `AMORTOP.pas:641-648`; the single solved payment at
> `:1216/:1499`). Fix: new `fancybisect.go solveMoratoriumPayment` builds a sub-loan for the
> remaining term (the post-moratorium balance over the remaining periods, carrying the
> not-yet-applied balloons/prepayments) and solves its payment with the balloon-aware
> `solveFancyPayment`; the moratorium recompute in `engine.go` refines the balloon-blind annuity
> seed with it. Regression test: `internal/api/amort_moratorium_balloon_test.go`
> (fail-before/pass-after verified — reverting drops interest to $102,258 and the loan retires at
> payment 234).

| case | engine | total interest | final balance |
|---|---|---|---|
| moratorium (interest-only to m24) + balloon $30k @m120 | DOS | **157,192.84** | 0 (retires at term) |
| | Go (before) | 102,258.22 | 0 (retires early, ~pmt 234/360) |
| | Go (after fix) | **157,192.87** | 0.05 (retires at term) |

Was off by **~$55k**; now agrees to **$0.03** of total interest and retires at the full 360-period
term. Tracked as a tight guard in `TestDOSOptionComboDivergenceTracker` (no longer KNOWN-OPEN).

## Standing tracker

`internal/finance/amortization/dos_option_combo_tracker_test.go` (`TestDOSOptionComboDivergenceTracker`)
is a standing differential check over the **option-combination cross** — the gap the post-mortem
identified. For each combination it runs the product `Amortize` path and the DOS oracle, **logs the
total-interest and final-balance divergence** (visible with `-v`), and **fails** if a fixed/control
combo drifts (regression guard) while **tracking** known-open combos without breaking CI. Runs only
where the oracle binary is present. Add a row whenever a new option pair becomes reachable.

Current state (`go test -run TestDOSOptionComboDivergenceTracker -v`):

| combo | Δ total interest vs DOS | status |
|---|---|---|
| single_balloon · single_ARM | $0.00–0.16 | guard |
| balloon + ARM | −$3.45 | guard (fixed) |
| two_balloons + ARM | +$1.48 | guard (fixed) |
| skip excludes / includes first pmt | +$0.19 / +$0.66 | guard (fixed) |
| moratorium + balloon | +$0.03 | guard (fixed) |

All tracked combinations now agree with the DOS engine to within a few dollars (DOS-vs-port
rounding tail). No KNOWN-OPEN combos remain.

## Randomized option-cube fuzzer (NEW — found a whole frontier)

`internal/finance/amortization/dos_option_cube_fuzz_test.go` (`TestDOSOptionCubeFuzz`) generates
**random loans crossed with random subsets of options** (balloons, ARMs, moratorium, target, skip)
and diffs every case against the real DOS engine, comparing total interest **and** whether the loan
retires to ~0. It reaches the 3-, 4-, and 5-way combinations no hand table covers. Opt-in (it fails
while the frontier below is open): `PERSENSE_FUZZ=1 go test -run TestDOSOptionCubeFuzz -v`
(needs the oracle binary). Deterministic seed; every divergence prints the exact `amort_oracle`
repro command and is bucketed by an option-signature so the classes are visible.

**Divergence map (seed 20260624, N=240).** All confirmed real against the oracle (DOS retires to $0
cleanly in every case). Buckets are `diverged/ran`; `2+` means two or more of that option.

CLEAN (0 divergences) — individual options and most balloon/ARM/moratorium/target pairs:
`(plain)`, `balloon`, `balloon2+`, `ARM`, `ARM2+`, `ARM+target`, `ARM2++mor`, `ARM2++target`,
`balloon+ARM`, `balloon+ARM+mor`, `balloon+ARM+target`, `balloon+mor`, `balloon+skip`,
`balloon+target`, `balloon2++ARM`, `balloon2++ARM+mor`, `balloon2++mor`, `mor`, `skip`, `target`.

BROKEN — the open frontier, grouped by the underlying mechanism:

| mechanism | example signatures (diverged/ran) | symptom | status |
|---|---|---|---|
| **A. `skip` inside a re-amortization** | `ARM+skip` 5/5, `ARM2++skip`, `mor+skip` 5/5 | re-amortization skip-blind; large leftover balance or interest off 5–6 figures | **ARM side FIXED**; `mor+skip` improved (no leftover; ~4% interest tail in the skip-during-moratorium sub-case, folded into class B) |
| **B. `moratorium` + (`target` or `ARM`)** | `mor+target` 1/1, `ARM+mor` 3/10, `balloon+ARM+mor+target` 4/4 | the moratorium re-amortization accounts for balloons/prepayments/skip/target but **not a later ARM** | **`mor+target` FIXED**; **`mor+ARM` open** (exotic) |

**Class B status.** `mor+skip` (sub-loan carries it), **`mor+ARM` (ARM during the moratorium)** and
**`mor+target` — all FIXED 2026-06-24**. The `mor+target` root cause turned out to be a **precedence
inversion**, not the sub-loan: Go applied the per-period TARGET unconditionally, so during the
interest-only window it paid down ~TargetValue per period and lowered the balance before amortization
began. DOS gives the moratorium precedence — in `ComputeNext` the interest-only branch is an else-if
BEFORE the target branch (AMORTOP.pas:641-648), so a target never forces principal during the
moratorium. Fix: gate the target block on `!inMoratorium` (`engine.go`). For `mor=74 targ=61` on
$261k/240 the balance now holds at 261,000 through m73, payment 2297.73, interest 216,716.17 — identical
to the same loan with no target (the $61 target never binds), matching DOS to the cent. This bug
COMPOUNDED across every `*mor+target*` stack, so the fix dropped the fuzzer divergence count ~7-9 per
200-case seed (38→29, 38→32, 34→27). Regression: `mortarget_test.go`. (Also dropped `target` from the
moratorium sub-loan trigger — DOS uses the plain annuity for a target — which is consistent though it
was not itself the cause.) Tracing the oracle (`adj=19:0.1189: mor=77` on $388k/300) showed DOS
applies the new rate during the interest-only window (correct), then keeps the ARM's `Re_Amortize`
payment computed over `[ARM date → last date]` (AMORTOP.pas:1547) WITHOUT recomputing at the
moratorium boundary — so the payment is sized for more periods than actually amortize, the loan
under-amortizes, and DOS balloons the FINAL scheduled payment ($180k). Go was re-solving at the
boundary over the shorter window (retiring smoothly, ~$129k less interest). Fix
(`engine.go armDuringMoratorium`): suppress the boundary recompute when an ARM date falls inside the
moratorium; the AO5 payment + final-fold then reproduce DOS. Verified to the cent (interest
992,776.00, post-moratorium payment 4101.29, ballooned final ~182,059). Regression:
`amort_moratorium_arm_test.go`.

**Still open (deferred to the global-Iterate refactor, `docs/global_iterate_refactor.md`):** the
DEEPLY-stacked moratorium combos — `mor+target+ARM`, `mor+ARM-AFTER-the-window`, `balloon+mor+target`,
and `multi-ARM+skip` — where two or more re-amortization events compound. These need DOS's global
`Iterate`, not another point fix.
| **C. balloon + 2+ ARMs** | `balloon+ARM2+` 8/12, `balloon+ARM2++target` 5/7 | `refineAdjustmentPayment` refines a **single** adjustment; with two ARMs + a balloon the second re-amortization drifts (interest off hundreds–thousands) | **FIXED** (small ~0.1–0.2% tail on 4-way 2-balloon×2-ARM) |
| **D. edge: huge balloon adjacent to ARM** | one case in `balloon+ARM` 1/11 | a balloon ≈half the principal next to the ARM date; retires but interest off ~$5k | open |

**Class C — FIXED 2026-06-24.** `refineAdjustmentPayment` (the schedule-oracle solve that retires a
SINGLE ARM+balloon tail to zero — Divergence 1) is ill-posed with multiple ARMs: refining the first
adjustment's payment while a later adjustment is still a pending rate-only reset makes the terminal
depend on that second re-amortization, so the bisection lands off DOS's value (seg payments drifted
to 3301.73 / 3302.14 instead of DOS's 3380.59 / 2937.74; interest off $12k–$42k). DOS uses the plain
balloon-discounting annuity at EACH reset (Re_Amortize: `netBal = balance − discounted future
balloons`). Fix: gate `refineAdjustmentPayment` to `len(Adjustments)==1`; multi-ARM loans use the
per-reset plain annuity, matching DOS's segment payments (verified on the $329k/240 case: seg
2483.94 / 3380.59 / 2937.74, interest within ~$46 of DOS). A small ~0.1–0.2% interest tail remains
only on the 4-way 2-balloon×2-ARM corner (DOS-vs-port balloon-discounting rounding). Regression:
`internal/finance/amortization/amort_multiarm_balloon_test.go`.

**Class A (ARM side) — FIXED 2026-06-24.** Two coupled bugs: (1) the dispatch checked `skipActive`
before the adjustment-strip branch, so an ARM+skip loan solved its BASE payment with the adjustment
present (ill-posed) — the strip branch now runs first for any adjustment + downstream option
(balloon/prepay/skip/target), recovering DOS's base payment; (2) DOS keeps the **skip-blind** annuity
after the reset (it Iterates only for balloons) and dumps the residual into the FINAL scheduled
payment, so Go now folds the final residual into the last row for ARM loans (interest unchanged — it
already accrued — only the last payment/balance move, retiring the loan to $0). Verified to the cent
vs the oracle (`adj=70:0.0938: skip=6` on $88k/360: base 505.69, reset 692.09, final balloon
~$63,497.88, interest 191,266.58). Regression: `internal/finance/amortization/amort_arm_skip_test.go`.

Single options and the pairs we explicitly fixed (ARM+balloon, moratorium+balloon, skip-first-pmt,
**ARM+skip**) are DOS-faithful. The fuzzer shows the **re-amortization machinery is option-aware only
for the cases hard-wired so far** — the general N-way combination needs the re-amortization at every
mid-schedule point (moratorium boundary, each ARM) to account for **all** downstream options, the way
DOS's global `Iterate` does. Classes B–C are the next targeted fixes in the same family.

## Refactor progress (M1 + mor+target precedence fix)

Since the original sweep, two changes cut the frontier roughly in half (N=200 ×3 seeds):

- **`mor+target` precedence fix** (a per-period inversion — the target was paying principal during the
  interest-only moratorium; see Class B): 38/38/34 → 29/32/27.
- **Refactor M1 — `til_adj` segment solve** (`solveSegmentPayment` replaces the entire-schedule
  `refineAdjustmentPayment`, composing for 2+ ARMs; see `docs/global_iterate_refactor.md`):
  29/32/27 → **19/28/22**.

What remains is now exclusively 4-to-6-way stacks: `skip` co-occurring with `balloon` + (ARM / mor /
target), and `balloon+ARM2++mor`. These are M2 territory (unify the base + moratorium solve).

## Honest frontier at higher N (N=240)

A larger sweep (seed 20260624, N=240) reaches harder cross-products than N=130 and shows the
remaining work clearly: **47 divergences / 228 ran, across 21 signatures**, now essentially all
THREE-to-FIVE-way cross-products of the re-amortization options. The fixed classes hold for their
specific signatures (single `ARM+skip` 0, `balloon+ARM2+` without skip 0, `mor+balloon`/`mor+target`/
`mor+skip` mostly 0), but their COMBINATIONS still diverge:

- `balloon+ARM2++skip` 7/7, `balloon2++ARM2+` 2/6 — multi-ARM crossed with skip or a second balloon
  (the per-reset plain annuity + final-fold doesn't fully retire these multi-segment shapes).
- `*mor+target*` / `*mor+ARM*` families (`balloon2++ARM+mor+target` 5/5, `ARM+mor` 3/10, etc.) — the
  moratorium+ARM corner (DOS balloons the final; see Class B).

**Takeaway / recommendation.** Single options and the common two-way pairs a client is realistically
likely to exercise (ARM+balloon, ARM+skip, balloon+skip, mortgage-style balloon, moratorium+balloon)
are now DOS-faithful to the cent. The residual frontier is exotic 3–5-way option stacks. Driving it to
literal zero is no longer a series of point fixes — Go's piecewise re-amortization (solve the base,
re-amortize at each event) diverges from DOS's GLOBAL `Iterate` (one Newton solve over the whole
schedule, with analytic Re_Amortize seeds) precisely on these stacked combinations. The durable fix is
to mirror DOS's global Iterate for fancy loans; that is an architectural change, scoped separately. The
fuzzer (`PERSENSE_FUZZ=1`) is the standing meter for that work.

## Status / recommendation

- These are real discrepancies from the DOS authority, in **option *combinations*** the prior
  validation cubes did not cover (ARM × balloon; skip-includes-first-payment). Individual options
  are DOS-faithful.
- Severity **S2**: wrong total interest / non-amortizing schedule for those combos; a client
  exercising an ARM with a balloon would see it.
- **Fix target (engine):** the fancy re-amortization on a rate-only adjustment
  (`internal/finance/amortization/engine.go`, the adjustment/`oddFirstPeriod`/`solveFancyPayment`
  path) needs to account for later balloons so the schedule retires to zero like DOS; and the
  skip path needs to handle a first-payment-month skip. Per CLAUDE.md, any fix ships with a
  regression test — the exact cases above, with the DOS-oracle goldens, are ready to assert.
- The exhaustive suite already flags these (logged as residual tails); see
  `internal/api/testdata/amort_ui_cases.json` cases `combo` / `skip`.
