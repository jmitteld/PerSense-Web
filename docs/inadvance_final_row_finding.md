# Finding: in-advance (annuity-due) final-payment interest differs from DOS

> **STATUS: FIXED 2026-06-09 (approved).** The Go in-advance branch in
> `engine.go` now charges the final-period interest per the DOS formula. The
> per-row oracle confirms the full in-advance schedule (including the final row)
> matches DOS across 300 randomized loans, the worked example matches exactly
> (DOS 387.98 = Go 387.98, `TestDOSInAdvanceFinalRowFix`), and the existing
> in-advance payment-solve and totals tests still pass. The original finding is
> retained below for the record.

*Surfaced 2026-06-09 by the new per-row differential oracle. This was a real,
reproducible discrepancy between the Go engine and the original DOS engine on
the **last** payment of an in-advance (annuity-due) amortization. Rows 1..n-1
already matched the DOS engine exactly.*

## What differs

Worked example: $100,000 at 12%/yr, 4 annual payments, **in advance**. Both
engines solve the same level payment (29,395.93) and produce identical rows for
periods 1-3. On the final (4th) payment:

| | interest | principal this period | balance after | final payment |
|---|---:|---:|---:|---:|
| **DOS** | 387.98 | 32,241.15 | 0.00 | 32,629.13 |
| **Go**  | 0.00   | 32,241.15 | 0.00 | 32,241.15 |

The loan is fully retired in both (balance 0), but DOS charges 387.98 of
interest on the final in-advance payment and Go charges none. The total amount
paid therefore differs by 387.98 on this loan.

## Why — the exact code

**DOS** (`legacy/src/dos_source/AMORTIZE.pas:1533-1538`), per period:

```pascal
if (df.c.in_advance) then
  begin
    if (p < d) then
      interest := 0            {needed for odd last payment}
    else
      interest := (p - d) * f_1 / (2 - f);
  end
```

DOS uses the level payment `d` every period and charges
`(p - d)·(f-1)/(2-f)` interest, zeroing it **only** when the balance `p` is
smaller than the level payment `d`. On the final row here `p = 32,241.15 > d =
29,395.93`, so DOS charges interest = `(32241.15 - 29395.93)·0.12/0.88 ≈ 387.98`.

**Go** (`internal/finance/amortization/engine.go:641-648`):

```go
} else if settings.InAdvance {
    if i == loan.NPeriods-1 {
        pmt = p              // force final payment = outstanding balance
    }
    intThisPd = inAdvanceFF * (p - pmt)   // = 0 on the last period
    ...
}
```

Go special-cases the final period to force `pmt = p`, which makes `(p - pmt) =
0` and the interest zero — a different convention from DOS.

## Proposed fix (faithful to DOS)

Remove the final-period special case and apply the DOS rule on every period:
interest = `inAdvanceFF · (p - d)` using the level payment `d`, zeroed only when
`p < d`; the final payment then clears the balance **plus** that interest. This
is a direct translation of `AMORTIZE.pas:1533-1538`.

**Why it is not applied unilaterally:** it changes the in-advance schedule total
and the final payment amount. Per the client's (correct) guidance that ported
behavior changes should be validated to the same standard as the original, this
is surfaced for a decision rather than silently changed. Once approved, the fix
is small and the per-row oracle will validate it against the real DOS engine
across the full randomized sweep (the in-advance body already matches; this
closes the final row), and the in-advance payment-solve and totals will be
re-checked to confirm they still match.

## Current test state

- `TestDOSFancyFlagSweep` validates in-advance rows 1..n-1 (0 divergences over
  300 loans) and Rule-of-78 in full (0 divergences).
- `TestDOSInAdvanceFinalRowFinding` pins the discrepancy (asserts DOS charges a
  non-zero final-row interest) so that if the engine is changed, the test
  notices and must be updated deliberately.
