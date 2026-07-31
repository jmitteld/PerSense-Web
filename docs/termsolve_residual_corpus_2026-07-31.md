# Term-solve residual: minimal repros, 2026-07-31

Companion to `docs/discrepancies.md` §49 and the round-7 write-up. This is the
working corpus for the residual amortization divergence, characterised **by
mechanism**, not by fuzzer option signature (see
`claude/defect_population_estimate_2026-07-30.md` for why signature counts are
meaningless here).

## Where the residual actually is

Measured 2026-07-31 against the freshly built Linux oracle at HEAD:

| sweep | compared | divergences | rate |
|---|---|---|---|
| unbiased, all 9 payment frequencies (4 seeds × N=1500) | 4,160 | 4 | 1 in 1,040 |
| frontier-biased `PERSENSE_FUZZ_MODES=noterm,non` (10 seeds × N=1500) | 11,149 | 2 | 1 in 5,575 |

**All 6 are in `non` / `noterm` — the term-solve modes.** Non-term-solve modes:
0 in roughly 2,773 compared cases. This confirms rather than overturns the
2026-07-30 finding that term solve carries ~86% of divergences.

A caution recorded because it produced a wrong claim on 2026-07-31: an earlier
2-seed biased run returned 0 in 2,222 and was read as the frontier having
closed. It had not. 0-in-2,222 bounds the rate at roughly 1 in 740 at 95%
confidence — it is not evidence of absence, and a 10-seed run found hits.

Note the biased and unbiased populations do NOT overlap: the mode filter draws
an extra RNG value per case (`dos_fuzzer5_test.go:1079`), which shifts the whole
subsequent option stream. Both are valid samples of term-solve mode; they are
different cases.

## The instrument: `cmd/goamort` now speaks the full solve-mode token set

Bisecting these required driving the Go engine from the shell with the same
tokens as `amort_oracle`. `goamort` was missing exactly the tokens the failing
cases use, so every investigation needed a bespoke Go test — the same
bottleneck the PV audit hit before `cmd/pvprobe` gained `colamonth=`.

Added: `noterm`, `non`, `lastdmy=D.M.Y`, `bdump`. With those, a bisect is a
shell loop over option subsets instead of a scratch test per hypothesis.

**Validation — this matters before trusting any bisect below.** `goamort`
reproduces `amort_oracle` EXACTLY on control screens spanning the same option
blocks as the failing cases:

```
SAME  100000 0.08 120 12 adj=24:0.06:
SAME  100000 0.08 120 12 loandmy=1.1.2024 firstdmy=1.2.2024 adj=24:0.06:
SAME  250000 0.075 360 12 loandmy=1.1.2024 firstdmy=1.2.2024 adj=60:0.05:
SAME  100000 0.08 120 12 loandmy=1.1.2024 firstdmy=1.2.2024 adj=24:0.06:1200
SAME  100000 0.08 120 12 loandmy=1.1.2024 firstdmy=1.2.2024 adj=24:0.06: payhard=1200
SAME  100000 0.08 120 12 loandmy=1.1.2024 firstdmy=1.2.2024 pre=12:24:12:200
SAME  10000 0.12 12 12
```

So a DIFFER below is the engine, not the harness.

## MINIMAL REPRO — adjustment on a mid-year loan date (OPEN, not root-caused)

The tightest form found. Four tokens beyond the base screen:

```
amort_oracle 100000 0.08 120 12 loandmy=30.6.2023 firstdmy=30.8.2023 adj=24:0.06: payhard=1200
  DOS interest 38319.95   |   Go interest 39313.84     (Δ 993.89, 2.6%)
```

Reduction, all with `goamort` validated against the oracle as above:

```
loandmy=30.6.2023 firstdmy=30.7.2023 adj=24:0.06: payhard=1200   DOS 37508.86  GO 38469.33   DIFFER
loandmy=30.6.2023 firstdmy=30.8.2023 adj=24:0.06: payhard=1200   DOS 38319.95  GO 39313.84   DIFFER
loandmy=30.6.2023 firstdmy=30.9.2023 adj=24:0.06: payhard=1200   DOS 39121.31  GO 40148.48   DIFFER
loandmy=30.6.2023 firstdmy=30.8.2023                payhard=1200   DOS 47898.78  GO 47898.78   SAME
loandmy=30.6.2023 firstdmy=30.8.2023 adj=24:0.06:                  DOS 38142.67  GO 39084.66   DIFFER
loandmy=30.6.2023 firstdmy=30.8.2023 adj=24:0.06: pay=1200         DOS 38319.94  GO 39313.84   DIFFER
loandmy=1.6.2023  firstdmy=1.8.2023  adj=24:0.06: payhard=1200   DOS 38319.95  GO 39475.10   DIFFER
loandmy=15.6.2023 firstdmy=15.8.2023 adj=24:0.06: payhard=1200   DOS 38319.95  GO 39313.84   DIFFER
loandmy=1.1.2024  firstdmy=1.2.2024  adj=24:0.06: payhard=1200   DOS  --------  GO  -------   SAME
```

What the reduction establishes:

- **The adjustment is necessary.** Remove it and the two engines agree exactly.
- **A hard payment is NOT necessary** — `adj` alone diverges, and `pay=` behaves
  like `payhard=`.
- **An odd first period is NOT necessary** — a normal one-month first period
  (`30.6.2023` → `30.7.2023`) diverges too.
- **The loan DATE is the discriminator.** Loan `1.1.2024` agrees; loan
  `1.6.2023`, `15.6.2023` and `30.6.2023` all diverge, at all three day-of-month
  values. So it is not a day-of-month clamp.

**Not yet root-caused.** The next step is a row-level diff (`rows` mode on both
CLIs) to find the first divergent row, then the DOS source crawl. Candidate area
is the adjustment prepass / `Re_Amortize` last-date and payment re-derivation —
see `claude/reamortize_lastdate_var_snap_2026-07-29.md` and
`claude/lost_session_recovery_and_reamortize_correction_2026-07-30.md`, which
established that DOS mutates a global there and that the structural and piecewise
engines read it at different sites. That is a hypothesis, not a finding.

Ruled OUT as the discriminator by direct test: the `adj=` date anchoring. Both
CLIs compute `tot := (loandate.m - 1) + months` and take `(tot mod 12) + 1` /
`loandate.y + (tot div 12)` with the loan day (`amort_oracle.pas:316-320` vs
`goamort` `monthsAfter`). For every adjustment offset in these repros the day is
valid in the target month, so DOS's `CheckForDaysTooLarge` and Go's
`types.NewDateRec` normalisation cannot differ. (They WOULD differ on a day-30
loan date with an offset landing in February — DOS clamps to 28/29, Go rolls to
1 March. That is a separate latent issue worth its own test, not this one.)

## The last-date shape, same case family

On the original fuzzer case the divergence also moves the derived last date,
which is a second observable of the same option:

```
amort_oracle 369950.48 0.1244090000 144 12 exact plusreg r78 usa \
  loandmy=30.6.2023 firstdmy=30.8.2023 payhard=5536.72 non lastdmy=30.7.2035 bdump
  base                                    DOS lastdate 7/30/2035   GO 7/30/2035   SAME
  + skip=2,8,11                           DOS lastdate 7/30/2035   GO 7/30/2035   SAME
  + adj=67:0.0383670000:                  DOS lastdate 7/30/2035   GO 7/30/2035   (totals differ)
  + adj=77:0.1077290000:                  DOS lastdate 7/31/2035   GO 7/30/2035   DIFFER
  + adj=67:... adj=77:...                 DOS lastdate 8/31/2035   GO 7/30/2035   DIFFER
```

`nperiods` is 144 in every variant — only the derived LAST DATE moves, and only
when adjustments are present. `skip=` and `targ=` are inert here.

## Remaining cases in the corpus, not yet reduced

```
seed 41003  balloon3+mor+prepay2+targ|first<|noterm   dInt 128.34
  amort_oracle 482790.87 0.0589160000 24 2 exact inadv plusreg r78 usa \
    loandmy=21.4.2023 firstdmy=21.8.2023 mor=52 b76=38069.16 b94=33877.37 \
    b100=46566.04 pre=70:136:52:42.84 pre=46:175:24:285.95 targ=778.20 \
    payhard=34744.83 noterm bdump

seed 41004  pts+targ|non                              dInt 202.38
  amort_oracle 240341.90 0.1306500000 408 24 b365_360 exact r78 usa \
    loandmy=31.12.2023 firstdmy=31.1.2024 targ=121.70 pts=0.017992 \
    payhard=1894.19 non lastdmy=31.12.2057 bdump
  (terminal balloon differs in VALUE at the same date: DOS -8709211.33, Go -8701573.96)

seed 41017  adj1+balloon2+prepay1+pts+targ|first<|noterm   dInt 81.17
seed 41018  pts+targ|first>|non                             dInt 58.24
```

Two of these carry no adjustment at all, so there is **more than one mechanism**
in the residual. Do not assume the repro above explains all six.

## Method note

The option signature is not a mechanism. In this corpus `targ` appears in 5 of 6
cases — which looks like a strong signal until you check the base rate: the
fuzzer draws `targ` on ~84% of cases, so 5 of 6 is exactly expected. Same for
`pts` (~70% base, 3 of 6 — below base). Every apparent signature correlation
here vanishes against its base rate, which is precisely the failure mode the
defect-population estimate warned about. Reduce with a bisect; do not infer from
signatures.
