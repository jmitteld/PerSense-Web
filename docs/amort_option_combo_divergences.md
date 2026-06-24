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
> A small two-balloon-spanning-the-ARM tail (~$50) and the skip case below remain.

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

## Divergence 2 — skip set that includes the first payment month  (S2/S3)

`skip = 1-3,7` (Jan–Mar + Jul). The first scheduled payment (Feb) is itself a skipped month.

| engine | payment (regular) | total interest | rows | final balance |
|---|---|---|---|---|
| DOS | 1,105.34 | 165,281.17 | 359 | 0.00 |
| Go | (row 1 = $0, Feb skipped) | 166,117.24 | 360 | 1,103.84 |

DOS produces a 359-row schedule that amortizes to $0; Go produces 360 rows and leaves ~$1,104,
with ~$836 more total interest. A skip set that **excludes** the first payment month (e.g. `6-8`)
matches DOS to within rounding — so the divergence is specific to skipping the first payment.

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
