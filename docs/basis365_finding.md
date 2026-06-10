# Finding: 365-day basis amortization differs from DOS (~1e-4)

> **STATUS: FIXED 2026-06-09.** Root cause found and corrected — and it was
> broader than the 365 basis. `SolvePayment` used the plain annuity formula and
> **ignored the first-period proration entirely**, so the solved payment was
> wrong for *any* odd first period (on any basis), and for every 365-basis month
> (whose actual day count is never exactly one even period). The fix scales the
> closed-form payment by `ffFirst/f` where `ffFirst = 1 + (f-1)*prorate` and
> `prorate = YearsDif(first, loan) * perYr` — which is exactly 1.0 for the common
> firstDate = loanDate + one-period case, so all previously-validated 30/360
> sweeps are unchanged. Now matches DOS: 365-basis monthly 4523.2202 = 4523.2202;
> 360 short-stub 13719.9584 = 13719.9584. Validated directly against the DOS
> solved payment over 400 odd-first + 300 365-basis loans, 0 divergences
> (`TestDOSPaymentSolveOddFirstAndBasis`). **Note:** the earlier odd-first-period
> sweep missed this because it fed a *shared* payment to both engines and only
> compared schedules — it never checked the solved payment. That gap is now
> closed by the new direct payment-solve sweep.

*Surfaced 2026-06-09 while extending the differential oracle to non-default
bases. The default 30/360 basis matches DOS bit-for-bit; the actual/365 basis
did not, which led to the broader payment-solve fix above.*

## What differs

A plain monthly loan computed on the **365-day basis** (no exact mode):
$100,000 at 8%, 24 monthly payments.

| | solved payment |
|---|---:|
| **DOS** (`b365`) | 4523.2202 |
| **Go** (`Basis365`, `YrDays=365.25`) | 4522.7291 |

Difference ≈ 0.49 (relative ≈ 1.1e-4). The same loan on the 30/360 basis matches
to ~1e-9.

## Likely cause (to confirm)

DOS `SetYrDays` uses **365.25** days/year for the `x365` basis
(`INTSUTIL.pas`). On that basis the day count between monthly payment dates is
the *actual* number of days, so a Jan→Feb first period is 31/365.25 of a year —
slightly more than one "even" period — whereas 30/360 makes every month exactly
1/12. That perturbs the **first-period proration** (`prorate = YearsDif(first,
loan) * perYr`) and the **rate→true-rate conversion** (`RateFromYield` is fed
`yrdays`), both of which feed the payment solve.

The ~1e-4 gap suggests Go and DOS apply one of these 365-basis conventions
slightly differently — candidates: the exact `yrdays` value (365 vs 365.25), how
`RealPerYr`/`RateFromYield` use `yrdays` on the 365 basis, or the actual-day
count between dates. This requires a focused trace of one loan through both
engines' 365-basis path (the 30/360 path is already confirmed identical).

## Related basis-variant work (queued together)

- **Biweekly / weekly (26 / 52 per year)** auto-use the 365 basis. They also
  need **14-day / 7-day date arithmetic**: the oracle (and the Go comparison)
  currently derive the first-payment date in whole months (`12/perYr`), which is
  0 months for biweekly. The schedule dates must advance by 14/7 days. Go has the
  biweekly factor (`GrowthPerPeriod`: `1 + 14*yrinv*rate`), so the engine
  supports it; the differential harness needs day-based dates to drive it.
- **Exact mode (365 + Exact=YES)** produces the deliberately "strange" schedules
  the help docs describe: interest varies month-to-month with the actual number
  of days. Validating it per-row requires the oracle and Go to agree on the
  per-month day count.

## Status / recommendation

The oracle now accepts `b365` and `exact` tokens (harmless infrastructure). The
365-basis difference above is **not yet explained or fixed**. Because 30/360 is
the default and by far the most common basis (and is fully validated), this is
lower urgency than the engines' default behavior — but it is a real fidelity gap
on the 365 basis and should be traced and reconciled (or the difference
characterized as an intentional/acceptable convention) in a focused pass.
