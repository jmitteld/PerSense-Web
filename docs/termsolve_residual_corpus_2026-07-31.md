# Amortization residual: reduction, and a harness bug that faked a finding

Working corpus for the residual amortization divergence. Supersedes the first
version of this file from earlier the same day, which reported a "minimal repro"
that was **an artifact of the audit tool, not the engine**. That reversal is
recorded in full below because the failure mode is the recurring one in this
project and the correction is more useful than the retraction.

## Where the residual is

Measured 2026-07-31 against the freshly built Linux oracle at HEAD:

| sweep | compared | divergences | rate |
|---|---|---|---|
| unbiased, all 9 payment frequencies (4 seeds × N=1500) | 4,160 | 4 | 1 in 1,040 |
| frontier-biased `PERSENSE_FUZZ_MODES=noterm,non` (10 seeds × N=1500) | 11,149 | 2 | 1 in 5,575 |

**All 6 are in `non` / `noterm`.** Non-term-solve modes: 0 in roughly 2,773
compared cases. This confirms the 2026-07-30 finding (term solve carries ~86% of
divergences) rather than overturning it.

An earlier 2-seed biased run returned 0 in 2,222 and was briefly read as the
frontier having closed. It had not — that bounds the rate at about 1 in 740 at
95% confidence and no better.

Note the biased and unbiased populations do not overlap: the mode filter draws an
extra RNG value per case (`dos_fuzzer5_test.go:1079`), shifting the whole
subsequent option stream. That is also the most likely reason their rates differ
5×, and it means neither is a reliable rate estimate on its own.

## THE HARNESS BUG — read this before trusting any bisect

`cmd/goamort` anchored `b<N>=`, `pre=` and `adj=` on the **default** loan date,
applying `loandmy=`/`firstdmy=` only afterwards. Its own comment asserted the
oracle did the same. It does not. `amort_oracle.pas:759-779` applies the override
**between** `SetupLoan` and `SetupBalloons`/`SetupPrepayments`/`SetupAdjustments`,
and carries an explicit warning:

> "This MUST run between SetupLoan and SetupBalloons/SetupPrepayments/
> SetupAdjustments. Those three anchor every option date on h^.loandate … so if
> the loan date is overridden AFTER them, the balloons/prepayments/adjustments
> stay pinned to SetupLoan's default 1.1.2024 while the loan itself moves. …
> 2026-07-25: this was the state of the driver when fuzzer5 grew a loan-date
> axis, and it turned 85 of 95 compared cases divergent in a single run — all of
> them the same harness artifact, none a port bug."

The identical defect was live in `goamort`, and it produced the identical false
result: a confident four-token "minimal repro" of an engine divergence that was
purely the tool placing the adjustment on the wrong date. It was caught only
because the measured offset was *too regular* — a constant row shift per loan
date, independent of the adjustment offset, which no real accrual bug produces.

Fixed by a pre-pass that applies `loandmy=`/`firstdmy=` before any option token.
After the fix the rate-switch row matches DOS exactly across every loan date and
adjustment offset tested (20 combinations, delta 0 everywhere).

**Rule this cost us twice now: a differential tool must be validated on the axis
being investigated, not just on default screens.** The original validation used 7
control screens, all on the default loan date — which is precisely the case the
bug could not affect.

### Third harness bug — option dates: DOS CLAMPS, `types.NewDateRec` ROLLS

`goamort`'s `monthsAfter` built every option date (`b<N>=`, `pre=`, `adj=`) with
`types.NewDateRec`, which is `time.Time`-backed and NORMALISES an impossible day
forward — 30 Feb becomes 2 March. The oracle finishes the same three lines with
`CheckForDaysTooLarge` (`amort_oracle.pas:176, :258, :314`), which pins the day
DOWN to the month end — 28 Feb.

So a day-30 loan date with an offset landing in February put the option on a
DIFFERENT DATE in each engine. It presented convincingly as an engine defect:
`pre=20:24:12:150` on a 30 June loan diverged by 883 in total interest, day 28
and day 1 were clean, and the trigger looked like "month-end grid" — the same
shape as the real §50 mechanism. It was the harness.

Fixed by clamping in `monthsAfter`. Effect on the randomized differential:

| | agree | rate |
|---|---|---|
| before the clamp fix | 282 / 296 | 95.3% |
| after | **291 / 296** | **98.3%** |

and **all five remaining disagreements carry an `adj=`**, i.e. they all point at
the one named open mechanism below rather than at anything new.

That is three harness bugs in this tool in two days (token order, then the
clamp). Each one produced a confident, plausible, wrong finding. The pattern to
internalise: **any date the harness computes must be computed the way the oracle
computes it, arithmetic and clamping included** — reproducing the formula is not
enough if the normalisation differs.

### Current validation state of `cmd/goamort`

Randomized differential vs `amort_oracle`, 296 comparable screens with loan-date
overrides, odd first periods, and 0-3 option blocks: **291 agree, 5 differ
(98.3%)**, after both the token-order and the date-clamp fixes. All five
remaining carry an `adj=` and are believed to be the piecewise pre-pass gap
(§50's "known related gap"), which has a clean repro:

```
amort_oracle 100000 0.08 144 12 loandmy=30.6.2023 firstdmy=30.8.2023 \
             adj=67:0.04: adj=77:0.11: pre=20:24:12:150
  DOS 60633.01  |  Go 60283.55     (Δ 349.46, vs Δ 353.12 for the structural case)
```

The prepayment forces the piecewise engine (the structural port declines
prepayments), and the piecewise engine still lacks the lastdate snap that §50
ported into the structural one.

Screens with impossible dates (`31.11`, `31.2`) were excluded: DOS clamps them
and Go's `types.NewDateRec` rolls them over, so they diverge for reasons the real
UI cannot produce. That difference is real but is a separate latent issue.

## The reduced case that survives the fix

```
amort_oracle 100000 0.08 144 12 loandmy=30.6.2023 firstdmy=30.8.2023 \
             adj=67:0.04: adj=77:0.11:
  DOS interest 60857.90  |  Go interest 60504.78
```

Established by bisect, with the fixed tool:

- **Two rate adjustments are required.** Either one alone agrees exactly.
- **Day 30 is the trigger.** The same screen at days 1, 15, 28 and 29 agrees;
  only day 30 diverges. (Day 31 in June is not a real date and both engines
  normalise it to something they agree on.)
- The adjustment offset matters: with the second adjustment fixed at 77,
  offsets 66/67/68 diverge while 60 and 72 agree.
- Nothing else is needed — no `payhard`, no odd first period, no `exact`,
  no `targ`, no `skip`, plain 360 basis.

### First divergent row

Row-diff at cent precision (DOS's `rows` mode re-parses its own 2-decimal table,
so both sides must be rounded to cents before comparing — comparing raw 4dp
output shows all 144 rows "differing" and means nothing):

```
  row 67  date 2/28/29
    DOS  int 220.35  prin 732.81  bal 65372.21
    GO   int 220.35  prin 743.50  bal 65361.52
  78 of 144 rows differ, all at or after row 67
```

**Interest is identical to the cent; the principal differs by 10.69** — so the
two engines are applying the same rate to the same balance but a *different
payment*: DOS 953.16, Go 963.85.

Row 67 is where `adj=67` takes effect, and its date is **2/28/29 — a February
row clamped from the day-30 grid**. So the divergence is in the **payment
re-solve at a rate adjustment whose effective row falls on a month-end-clamped
date**, which is why day 30 is the discriminator and days 1-29 are clean.

**Not root-caused.** The candidate area is the adjustment prepass /
`Re_Amortize` remaining-term derivation — see
`claude/reamortize_lastdate_var_snap_2026-07-29.md` and
`claude/lost_session_recovery_and_reamortize_correction_2026-07-30.md`, which
established that DOS mutates a global there and that the structural and piecewise
engines read it at different sites. Hypothesis, not finding. The next step is to
instrument both re-solves and print the remaining term and balance each uses.

## Original fuzzer cases, still to be reduced

```
seed 41002  adj2+prepay1+skip|first>|non              dInt 1348.34   (source of the above)
seed 41003  adj2+balloon1+mor+prepay1+targ|noterm     dInt  182.24
seed 41003  balloon3+mor+prepay2+targ|first<|noterm   dInt  128.34
seed 41004  pts+targ|non                              dInt  202.38
seed 41017  adj1+balloon2+prepay1+pts+targ|first<|noterm  dInt 81.17
seed 41018  pts+targ|first>|non                       dInt  58.24
```

Full command lines are in the fuzzer output; three of the six carry no
adjustment, so **there is more than one mechanism** and the reduction above does
not explain them.

## Method notes

**The option signature is not a mechanism.** In this corpus `targ` appears in 5
of 6 cases, which looks decisive until you check that the fuzzer draws `targ` on
~84% of cases anyway. `pts` is 3 of 6 against a ~70% base rate — below chance.
Every apparent signature correlation vanished against its base rate. Reduce with
a bisect; never infer from signatures.

**A too-regular pattern indicts the harness, not the engine.** The false finding
showed a constant row offset per loan date, independent of the adjustment offset.
Real accrual divergences vary with the inputs; a constant integer shift is a
pointer being computed off the wrong anchor.
