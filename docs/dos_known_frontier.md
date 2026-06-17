# DOS-Fidelity frontier — found, then closed

This documents a corner of the amortization engine that once diverged from the
real DOS engine, how the exhaustive `Amortize` dispatch sweep found it, and the
two fixes that closed it. It is now a strict zero-divergence regression guard,
not an open gap.

## What the sweep validates (zero divergence vs the real DOS engine)

Driven through the FULL `Amortize` field-presence dispatch (the path the API/UI
use), blank payment solved, compared to the DOS oracle (`legacy/oracle/`) —
`internal/finance/amortization/dos_amortize_dispatch_sweep_test.go`:

- **Blank payment + a known balloon** — 250 cases, 0 divergences.
- **Blank payment + an odd first period, plain 360 / prepaid-OFF** — 250 cases, 0.
- **Cross-product: basis {360, 365} × prepaid {off, on} × balloon {none, present}
  × balloon-includes-regular {off, on}, natural first period** — 658 cases, 0
  (tolerance 1e-3; 365-day basis accrues a few cents of per-period rounding).
- **Odd first period × {prepaid | balloon | 365}** — `TestDOSOddFirstFancyFrontier`,
  ~490 cases, **0 divergences** (was 20% before the fixes). Now a strict guard:
  any divergence > 1e-3 fails CI.

On top of the pre-existing ~6,400-case oracle sweeps for the backward solvers and
forward schedules.

## The frontier that was found

An *odd* first period (a short or long first-payment gap — e.g. an annual loan
whose first payment is 1 or 18 months out) combined with prepaid interest, a
balloon, or the 365-day basis. ~20% of those combinations diverged, up to ~13–18%
of the payment on extreme inputs. Two independent causes:

1. **Payment solve (prepaid / 365 odd-first).** The closed-form payment estimate
   assumes a full first period. For an odd first period the regular payment must
   be adjusted so the loan amortizes over the stated term. DOS does this by
   iterating the estimate (`EstimateAndRefinePayment`, Amortize.pas:416); the Go
   port previously augmented only the prepaid-OFF case and left prepaid / 365
   odd-first on the un-refined estimate.

2. **Off-cycle balloon placement.** DOS applies a balloon on its exact date —
   accruing partial interest up to it, then the next regular period accrues only
   from the balloon date (`RepayFancyLoan`, AMORTOP.pas:608-613). The Go port
   folded a balloon that fell between two regular dates into the *next* regular
   payment. An odd first period shifts every payment date off the balloon's
   monthly grid, so the balloon lands between dates — which is when this mattered.

## The fixes (both DOS-faithful)

1. **`oddFirstPeriod` + schedule-oracle refinement** (`engine.go`). When the first
   payment is not exactly one period out, the blank-payment solve refines the
   closed-form estimate with the bisection that drives the *unforced* terminal
   balance of the real (prorated) forward schedule to zero — DOS's `Iterate`,
   expressed against the actual schedule. A snap guard keeps an already-exact
   estimate untouched (no sub-cent bisection noise) and only adopts a materially
   different refined payment.

2. **Off-cycle balloon draining** (`generateFancySchedule`). A balloon dated
   strictly before the next regular payment is now emitted as its own dated row
   at its actual date, reusing the existing off-cycle prepayment draining: partial
   interest to the balloon date, then the next regular period accrues from there.

## Verification

`amort_oracle` gained `loandmy=` / `firstdmy=` date overrides so the differential
rig can drive odd-DAYS first periods (loan day-of-month ≠ first day-of-month),
which the month-only `first=` could not express. That surfaced a genuine
DOS-vs-Windows discrepancy (AM Example 1): for a 19-day short first period DOS
augments the payment to $731.98 while the Windows help shows the plain $733.76.
Per CLAUDE.md the DOS engine is the authority, so the port matches $731.98. See
`docs/discrepancies.md` §7.

Regression coverage: `TestDOSOddFirstFancyFrontier` (strict, oracle),
`TestAPIAmortOffCycleBalloonMatchesDOS` (DOS-pinned, no oracle needed),
`TestVerifyWebAM_EX1_Simple` (asserts the DOS values).
