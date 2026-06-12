# Amortization — first-period interest on the actual/365 basis (monthly) (2026-06-10)

## What was found

Extending the amortization per-row differential to an ordinary **monthly** loan on
the **actual/365 day-count basis** surfaced a first-period convention difference:

```
100000 @ 6%, 12 monthly payments, actual/365 basis, payment 8607.34
row 1: interest  Go = 508.20   DOS = 500.00
row 2: interest  Go = 459.50   DOS = 459.46   (both = balance × rate/12)
row 3+: ...                                    (both = balance × rate/12)
```

- **DOS** charges the first regular period the **nominal** `rate / Pmts-per-Yr`
  (here 0.06/12 × 100000 = **500.00**), the same monthly factor it uses for every
  subsequent period — even though the basis is actual/365.
- **Go** charges the first period by **actual days** over the year
  (Jan 1 → Feb 1 = 31 days; 100000 × 0.06 × 31/365.25 = **508.20**), then uses the
  nominal `rate/12` for periods 2..n like DOS.

The divergence is therefore **confined to the first period**. After row 1 both
engines accrue identically (`balance × rate/12`); the only residual is the ~8.20
first-period difference carried forward in the balance (well within the
proportional balance tolerance, ~0.008% of principal).

## Why it only shows on the 365 basis

On the **30/360** basis the two conventions coincide: a regular monthly first
period is 30/360 = 1/12, which equals `rate/12` exactly. That is why every
30/360 per-row sweep (the common case) is bit-faithful, and why this only appears
once the basis is actual/365 (where 31/365.25 ≠ 1/12). It is also why the
weekly/biweekly 365 sweeps are clean — those periods are inherently day-based and
have no nominal `rate/Pmts-per-Yr` equivalent.

Go computes the first period from the loan-date→first-payment date gap
(`YearsDif`, which is day-based on the 365 basis); DOS uses the nominal per-period
factor for a *regular* (non-odd-stub) first period regardless of basis.

## Status and recommendation

- **Scope:** actual/365 basis with **monthly** (or other nominal-period) payments
  is an uncommon configuration — actual/365 is normally used for daily / odd-
  period accrual, where Go and DOS already agree. The common 30/360 monthly case
  is unaffected and bit-faithful.
- **Magnitude:** first-period only, ~0.008% of principal; both schedules retire
  the loan correctly.
- **Surfaced, not silently changed:** matching DOS would mean making the *regular*
  first period use the nominal `rate/Pmts-per-Yr` even on actual/365, a change to
  the validated first-period interest path (which is bit-faithful on 30/360 and
  for the odd-stub and weekly/biweekly cases). Per the project standard this is
  documented for a decision rather than changed speculatively.
- The characterization test `TestDOS365BasisMonthlyFirstPeriod` pins this exactly:
  it asserts DOS's first period equals the nominal factor, Go's equals the
  actual-day proration, and that periods 2..n are otherwise identical — so the gap
  cannot widen unnoticed.

**Client decision:** keep the Go actual-day first-period accrual on actual/365
(arguably the more correct interpretation of an actual-day basis), or align it to
DOS's nominal `rate/Pmts-per-Yr`. No common (30/360) schedule changes either way.
