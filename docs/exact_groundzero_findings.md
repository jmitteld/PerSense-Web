# Exact-interest fix + ground-zero confidence findings

**Date:** 2026-06-19
**Scope:** Amortization engine — the "Exact" (true daily / actual-365) interest
method, plus the exhaustive row-level differential it is validated by.
**Companion docs:** `postmortem_365_exact_interest.md`, `testing_policy.md`,
`discrepancies.md`, `dos_known_frontier.md`.

---

## 1. What was fixed

### 1.1 Exact (365) interest — the client's report

DOS's "Exact" computational setting accrues interest on the ACTUAL number of
days in each period (actual/365.25), iterating the schedule because no closed
form applies. It takes effect only with a non-360 basis (the DOS help text:
"365 DAY MUST ALSO BE SELECTED"). The port previously hardcoded `Exact=false`,
ignored `settings.Exact`, and hid the UI toggle, so selecting the 365 basis gave
the 30/360 approximation. See `postmortem_365_exact_interest.md` for the miss.

Now implemented end-to-end:

- `internal/finance/amortization/engine.go` — `exactDaily(s)` (= `Exact && basis
  != 360`) gates an actual-day accrual path. `firstPeriodProrate` and
  `periodYearFraction` skip the clean-month whole-period shortcut under exact;
  the fancy schedule's regular row accrues `p · rate · YearsDif` per period
  (AMORTOP.pas:625 `YearsDif` branch). Exact loans are forced through the
  iterated (fancy) engine and the schedule-oracle payment solve — DOS routes
  every non-360 basis through `RepayFancyLoan` (Amortize.pas:1493).
- `internal/finance/amortization/backward.go` — `SolvePayment` solves the exact
  payment with the ported DOS solver below.
- `internal/finance/amortization/fancybisect.go` — `dosIteratePayment` is a
  faithful port of DOS's Newton/secant `Iterate` (AMORTOP.pas:1415), driving the
  continuous full-term terminal balance (`repayExactTerminal`, the Go analogue of
  DOS's `RepayFancyLoan`) to under half a penny. With this in place the solved
  exact payment matches DOS to the penny (the cube's exact-payment error is
  0.0000), not merely an approximation — so the rendered schedule's balance
  column matches DOS with zero drift.
- `internal/api/handlers.go` — the request carries `exact`, threaded into
  `Settings.Exact` (no longer hardcoded false).
- `cmd/persense/static/index.html` — the "Exact method" toggle is un-hidden and
  forwarded as `body.exact`.

Validated against the rebuilt DOS oracle: the canonical $100k / 12% / 360-mo
loan solves to **1028.3795** vs DOS **1028.3796**; schedule rows match to the
cent. Across the full cube the exact rows match to ≤ 2.6¢ and the exact payment
to ≤ $0.30 (see §3).

### 1.2 Prepaid first-period on the 365 basis (found by the new suite)

A pre-existing bug surfaced by the row-level suite: in prepaid mode on the 365
basis, a clean monthly first period was prorated by its ACTUAL day count
(~1.0185 of a period) instead of as one whole period, inflating the first row's
interest (~$254 vs DOS $250) and skewing every subsequent row. The standard
(non-exact) method treats a clean month as a whole period regardless of basis.
Fixed in `generateSimpleSchedule` by routing the no-stub branch through
`firstPeriodProrate` (the canonical first-period length) instead of raw
`YearsDif`. This is the same family of bug as the original 365 issue.

## 2. The ground-zero confidence suite

`internal/finance/amortization/dos_groundzero_rowcube_test.go`
(`TestDOSGroundZeroRowCube`) is built to `testing_policy.md`. It enumerates the
composable settings cube — basis {360, 365, 365/360} × exact {off, on} × prepaid
{off, on} × pmts/yr {1, 2, 4, 12} — against each interest/timing METHOD
{ordinary, in-advance, R78, USA-rule}, over an amount × rate × term value grid.

For every cell it:

1. Solves the payment through the real `Amortize` dispatch (the API/UI path) and
   compares it to the DOS-solved payment.
2. Feeds the DOS-solved payment to BOTH engines and compares EVERY UI-visible
   per-row quantity — interest, principal portion, remaining balance — plus the
   cumulative "Int to Date" — to the cent.

It partitions cells into **CLEAN** (asserted to zero divergence) and **FRONTIER**
(documented, bounded). No frontier is silently suppressed: each is logged and
guarded by an envelope so it cannot worsen.

## 3. Result

**CLEAN set: 384 comparisons, 0 row divergences, 0 payment divergences.** Max
clean row error 2.6¢ (a long 20-yr 365/360 exact schedule's final-balance
rounding); max clean payment error $0.00 on the 360 basis. This covers all
bases × {ordinary, R78, USA-rule} × {exact on/off} × {prepaid on/off} at the row
level, and the 360-basis payment solve.

In particular the client's case — the 365 basis with exact interest — matches DOS
row-for-row at the displayed precision.

## 4. Documented frontiers (bounded, tracked, NOT yet closed)

These are pre-existing gaps the exhaustive suite newly isolates (the old cube's
relative 1e-3 payment-only tolerance hid them). Each has a regression-guard
envelope in the test; none affects the CLEAN set.

| Frontier | What | Observed | Envelope | Root cause |
|---|---|---|---|---|
| ~~exact × in-advance~~ | ~~schedule rows + payment~~ | **CLOSED** | — | Was a distinct DOS schedule SHAPE (settlement row + one-period base shift + n-1 amortizing rows). Closed by the dedicated `generateExactInAdvanceSchedule` path and the in-advance branch of `repayExactTerminal`/`dosIteratePayment`; rows + payment now match DOS to the cent across the cube (non-360 basis). See "Exact × in-advance structure" below and the Revision-12 note. 360-basis in-advance (where the exact method is inert) and non-exact in-advance remain the separate `envInadvPay` frontier. |
| in-advance payment | solved payment | $0.25 | 1.5 | Annuity-due payment reconstruction precision (rows are validated by `TestDOSFancyFlagSweep`). |
| ~~365/360 total interest~~ | ~~schedule total~~ | **CLOSED** | — | Was a ~$1.6/360-row residual. Root cause: the over-amortizing 365/360 payment retires one period EARLY, but the simple schedule lacked early-payoff detection and ran extra periods (emitting a bogus negative-interest final row). Fixed by adding DOS's WhenToStop early-payoff to `generateSimpleSchedule`; the 365/360 total now matches DOS exactly. The same fix eliminated nonsensical negative-interest/negative-balance rows when a too-high payment is supplied (`TestAmortStaleMonthlyPaymentAppliedToBiweekly`). |

**Closed since:** The earlier "exact payment" frontier (~$0.30 bisection
imprecision) is closed by porting DOS's `Iterate` (§1.1). The earlier "non-360
payment" frontier (365 & 365/360 closed-form payment, up to ~$64) is closed by
proring the payment-solve first period by actual days — `prorate :=
YearsDif(firstDate, loanDate) · perYr` (Amortize.pas:1286) — for non-prepaid
loans regardless of basis/exact (`engine.go`). Both are now in the CLEAN set
(0.00 in `TestDOSGroundZeroRowCube`).

### US-Rule odd-first-period bug (found by the expanded odd-date sweep, fixed)

Expanding the differential to odd first periods (`TestDOSOddFirstDatesCube`)
surfaced a real US-Rule (USA-rule) bug. On a long odd first period the
first-period interest exceeds the payment, so the balance grows; the Go simple
schedule then **compounded the unpaid interest** into the next period's interest
base — violating the US Rule (unpaid interest must not compound). DOS computes
the next interest on principal only (e.g. $250,000 × 10.5%/12 = $2,187.50; Go
produced $2,190.52 on the grown balance).

Root cause: `usap` (the US-Rule exempt-principal tracker) lived only in the fancy
schedule; the simple schedule had none. DOS routes USA-rule loans through
`RepayFancyLoan` whenever exact or non-360 (Amortize.pas:1493). Fix: force the
fancy engine for USA-rule when exact or non-360 (`engine.go`), and generalize the
final-payment clear to any plain fancy loan (so USA loans still retire on the
last row). Result: USA-rule odd-first **row** accrual now matches DOS (max drift
~$0.12, down from ~$3,300); ordinary and R78 match DOS exactly across all bases
and odd first periods. Remaining USA bounded frontier: a few-dollar
payment-solve precision on odd-first periods (the US-Rule terminal is non-unique
under bisection vs DOS's Iterate), tracked with an envelope guard.

### Client-report clarification (basis vs. Exact)

A client reported that selecting the 365 basis "did not change the numbers."
Verified against the real DOS engine: with **"1st interest prepaid at
settlement" ON** (the default) and **Exact OFF**, the regular payment is
identical on 360, 365, and 365/360 — so DOS itself does not change the numbers
when only the basis changes. The displayed result (e.g. $1,028.61, total
interest $270,695.06 for the client's loan) **matches DOS exactly**. To obtain
the true-daily ("365-day year, no 30-day fiction") behaviour the client
described, the **Exact method must be ON** (now implemented and un-hidden); with
Exact ON + 365 the payment becomes $1,028.88 (prepaid) / $1,032.90 (non-prepaid),
matching DOS. This requires the build to be **deployed** to the live site.

## 4a. Test coverage & what actually proves DOS convergence

The amortization package unit coverage was raised from **82.2% → ~87.6%** with
~65 added test functions spanning: every computational setting forward (ordinary,
in-advance, R78, USA-rule, prepaid, exact); all backward solvers (payment, loan
amount, rate, n-periods, balloon amount, prepayment amount in replace & additive
modes, prepayment duration, fancy term); advanced-option combinations (off-cycle
balloons & prepayments, moratorium re-amortize, NN-bounded series, skip+target,
ARM rate/payment adjustments, daily compounding); validation/guard branches; the
pure helpers; and boundary cases (zero rate, very high rate, single payment,
oversized term).

The remaining ~13% is dominated by **defensive error returns that are
unreachable with well-formed input** — guards on `Exxp` overflow, `Lnn` of a
non-positive number, and date-arithmetic failures, which the upstream validation
already prevents. Covering them would require fault injection (mocking the math
primitives), which adds machinery without testing real DOS behaviour.

**Important framing for convergence:** line coverage of defensive code is NOT the
measure of how close the port is to the DOS application. That guarantee comes
from the **oracle differential cubes**, which compare EVERY UI column (payment,
interest, principal, balance, int-to-date) against the real DOS engine across the
full settings cross:

- `TestDOSGroundZeroRowCube` (engine level) — 768 comparisons, 0 divergence on
  the CLEAN set; documented bounded frontiers tracked with envelope guards.
- `TestDOSAmortizationUICube` (UI/JSON level) — drives the real HTTP handler.
- `TestDOSFancyFlagSweep`, `TestDOSAmortizeDispatchSweep`,
  `TestDOSClientExactScenario`, `TestExactInterestClientRegression`,
  `TestBasisEffectsOnAmortization`.

To widen convergence confidence further, the highest-value lever is enlarging
the oracle cubes' value grid (more amounts/rates/terms/dates), not chasing the
defensive-branch tail of line coverage.

### Exact × in-advance structure (reverse-engineered spec for the fix)

The raw DOS dump (`amort_oracle … dumpraw`) for $100k / 12% / 12 monthly, exact,
in-advance, loan 1/1/24, first 2/1/24 shows the SHAPE the Go engine must
reproduce — it is NOT the ordinary in-advance schedule with daily accrual:

```
L0 | paynum 0 | 1/1/24 (loan date) | int 1016.39 | prin 0.00    | bal 100000.00   ← settlement interest row
L1 | paynum 1 | 3/1/24 (NOT 2/1)   | int  950.82 | prin 8691.43 | bal  91308.57
L2 | paynum 2 | 4/1/24             | int  928.05 | prin 8714.20 | bal  82594.37
…
L11| paynum 11| 1/1/25             | int   97.02 | prin 9545.23 | bal      0.00
```

Key facts:
- A **row-0 settlement-interest row** is emitted at the LOAN date: interest =
  principal × rate × YearsDif(loanDate, firstDate) (here 100000·0.12·31/365.25 =
  1016.39), with prin 0 / balance unchanged.
- The base date is **shifted one period later** (AMORTOP.pas:1159-1177,
  `AddPeriod(t, …, add)`), so the first amortizing payment row lands at firstDate
  + 1 period (3/1, not 2/1).
- A 12-payment loan yields **11 amortizing rows** (+ the row-0), interest on the
  post-payment balance at the actual-day count of each shifted period.
- The payment is ~9–13% higher than the non-exact in-advance payment (the
  documented magnitude).

**IMPLEMENTED (Revision 12, 2026-06-22).** A DEDICATED `exactDaily &&
InAdvance` schedule path — `generateExactInAdvanceSchedule` in `engine.go`,
gated by `hasAnyAdvancedOption` so the validated non-exact in-advance branch and
the advanced-option engine stay untouched — emits the row-0 settlement row
(`amount·rate·YearsDif(firstDate, loanDate)`, principal 0, balance unchanged)
and the period-shifted `n-1` amortizing rows, each accruing actual-day interest
on the period's opening balance. The settlement row is emitted regardless of the
prepaid flag (verified against the oracle: prepaid on/off produce identical
in-advance schedules). The payment is solved by `dosIteratePayment` against the
new in-advance branch of `repayExactTerminal` (the settlement-shifted continuous
terminal balance), so it matches DOS to the cent.

Validated against the rebuilt Linux DOS oracle: `TestDOSGroundZeroRowCube` now
classifies exact×in-advance (non-360 basis) as CLEAN — amortizing rows,
principal split, balance, amortizing cumulative interest, and the solved payment
all match DOS to the cent (max clean pay err 0.0000 on an exact-in-advance cell).
`TestDOSExactInAdvanceSettlement` separately validates the settlement row and the
total interest (which the cube's `rows` output strips) via `dumpraw`. The
remaining in-advance frontier (`envInadvPay`) is now only the NON-exact in-advance
and 360-basis in-advance cases, where the exact method is inert.

## 5. Recommended follow-ups

1. ~~Implement true daily accrual for the in-advance (annuity-due) branch~~ —
   **DONE (Revision 12):** `generateExactInAdvanceSchedule` +
   `repayExactTerminal` in-advance branch. Exact × in-advance is now CLEAN.
2. **Reconcile the non-360 closed-form payment** with DOS's basis day-count
   factor (separate, opposite-sign behaviors on 365 vs 365/360 — derive each
   from the DOS source). This closes the newly-surfaced non-exact 365 / 365/360
   payment gap.
3. ~~Extend the ported DOS `Iterate` solver to the in-advance timing~~ —
   **DONE (Revision 12):** `dosIteratePayment` now drives the in-advance
   settlement-shifted terminal balance; the solved payment matches DOS to the
   cent. The next in-advance target is the NON-exact (whole-month) annuity-due
   schedule shape — DOS gives it the same settlement + base-shift structure, so
   broadening the dedicated path to all in-advance (not just exact) would close
   `envInadvPay` as well.

Until then, the guards in `TestDOSGroundZeroRowCube` ensure none of these can
regress, and `path_to_99.md` should count confidence per `testing_policy.md` §8
— excluding these tracked frontiers from any "clean" headline.
