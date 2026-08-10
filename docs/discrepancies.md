# Per%Sense — Known Discrepancies

This document tracks known differences between the DOS source code behavior,
the Windows help documentation, standard financial textbook formulas, and
the Go port. Understanding these is important for correctness validation
and for anyone comparing Per%Sense output to other financial calculators.

---

## 1. Summation Formula: Continuous vs. Discrete Compounding

**Status:** Resolved — the API boundary converts user input to the
true rate, so port output matches the textbook discrete formula.

### Description

The `Summation()` function in `internal/finance/mortgage/mortgage.go`
(ported from `Mortgage.pas`) uses a continuous-compounding formula
based on the natural exponential:

```
f     = exxp(-r / 12)        = e^(-r/12)
last  = exxp(-r * t)         = e^(-r*t)
sum   = f * (1 - last) / (1 - f)
```

The textbook mortgage formula uses discrete (monthly) compounding:

```
sum = (1 - (1 + r/12)^(-12*t)) / (r/12)
```

The two formulas are mathematically equivalent once the rate is
expressed in the right frame: the continuous formula expects a
continuously-compounded "true" rate, while the discrete formula
expects the nominal monthly-compounded loan rate. They are linked by
`trueRate = 12·ln(1 + loanRate/12)`.

### What the port does

The Mortgage API handler converts at the boundary:

- `LoanRateToTrueRate(loanRate)` is applied to every user-supplied
  rate before populating `MtgLine.Rate` (`internal/api/handlers.go`
  in `HandleMortgageCalc` and `HandleMortgageCompare` / `HandleMortgageWhatIf`).
- `TrueRateToLoanRate(trueRate)` is applied on the way out so the
  response carries a user-facing loan rate.

`Summation()` then receives a true rate, the continuous formula
applies correctly, and the resulting monthly payment matches what the
textbook discrete formula produces for the user's loan rate.

This boundary conversion is the F1 fix documented in
`docs/help_examples_test_report.md`. Before that fix, the API handler
copied `req.Rate` straight through, which treated the user's loan
rate as a true rate and produced payments roughly 0.27% too high.

### Caveat

The conversion runs in `HandleMortgageCalc` and the two adjacent
mortgage handlers. Callers that construct `MtgLine` directly (refdata
cross-checks, intermediate solver iterations) must pass a true rate;
the helpers are documented in the comments on `LoanRateToTrueRate` /
`TrueRateToLoanRate` in `mortgage.go`.

---

## 2. Help Documentation Examples vs. Running Program Output

**Status:** Resolved — port output matches the help-doc values after
the F1 fix in §1.

### Description

Earlier revisions of this document recorded a discrepancy between the
help-doc example values and what the running port produced (~$2.67 on
Mortgage Help Example 1, ~$515 on Mortgage Help Example 2). The root
cause was the missing loan-rate → true-rate conversion described in
§1; the port treated the user's loan rate as a true rate, biasing
every mortgage forward computation by ~0.27%.

After the F1 fix, the help-example test suite confirms parity. The
relevant assertions live in:

- `internal/finance/mortgage/help_examples_test.go` — the in-engine
  expectations (e.g. `wantMonthly = 1538.30` for MS_EX1, `wantPrice =
  241749.12` for MS_EX2).
- `internal/api/verify_web_help_examples_test.go` — the HTTP-level
  round-trips with the help-doc inputs.
- `docs/help_examples_test_report.md` — the 36-case audit (36 / 36
  passing as of 2026-05-13).

The web-rendered help at `cmd/persense/static/help.html` continues to
show the help-doc values verbatim; they are now the actual program
output as well. The legacy Windows help in
`legacy/src/win_source/Help/` remains the READ-ONLY reference.

### Historical numbers (pre-F1, kept for archaeology)

| Setup | Help doc | Pre-F1 port output | Difference |
|---|---|---|---|
| MS_EX1: $200k, 20yr, 8%, 2pts, 20% down, $200 tax | $1,538.30/mo | ~$1,540.97/mo | $2.67 |
| MS_EX2: $56k cash, $1,650/mo, 1.5pts, 8.5%, 30yr, $200 tax | Price $241,749.12 | Price $241,233.69 | $515.43 |

Both rows now match the help-doc values to within the 0.10-cent test
tolerance.

---

## 3. Rounding: Round-Half-Down vs. Banker's Rounding

**Status:** Go port matches DOS behavior.

### Description

The DOS `Round2()` function uses **round-half-down** (truncation at the
half): when the value is exactly at the midpoint (e.g., 1.235), it
rounds toward zero (to 1.23), not to the nearest even number.

From `refdata.json`:
```
Round2(1.235) = 1.23   (round-half-down)
Round2(1.236) = 1.24
Round2(0.005) = 0.00
Round2(0.006) = 0.01
```

Standard banker's rounding (round-half-to-even) would give:
```
Round2(1.235) = 1.24   (round to even)
Round2(0.005) = 0.00   (same — rounds to even)
```

### Impact

The difference only manifests when a value falls exactly on the half-cent
boundary, which is rare in practice. Over a 360-payment amortization
schedule, cumulative rounding differences are absorbed in the final
payment adjustment.

### Go port decision

The Go port's `interest.Round2()` replicates the DOS round-half-down
behavior exactly. The `CLAUDE.md` notes say "use banker's rounding
unless original code differs" — the original code does differ, and we
follow it.

---

## 4. Present Value: True Rate vs. Loan Rate on the PV Screen

**Status:** Resolved — UI converts to TrueRate before posting.

### Description

The Present Value screen exposes a "Rate Type" selector (Loan Rate,
True Rate, Yield). In the DOS program these three are interconvertible
representations of the same discount rate, mediated by
`InterpretedRate()` (PRESVALU.pas line 535, `YieldRateTranslation`).

### Resolution in the Go port

The conversion is performed **client-side** in
`cmd/persense/static/index.html` (`pvRateToTrue` / `pvTrueToType`,
called from `onPVRateTypeChange` and the PV input builder). The API's
`PVRequest.rate` field is always the continuously-compounded TrueRate,
so the engine never has to figure out which form the caller intended.

`InterpretedRate()` itself is ported in
`internal/finance/interest/rates.go` and is exercised by
`internal/finance/interest/rates_test.go` (round-trip with
`ReportedRate`) and `internal/api/pv_rate_interpretation_test.go`.

The `PVRequest.rateSchedule` entries for variable-rate mode also use
TrueRate exclusively, for the same reason — the comment in
`internal/api/handlers.go` on `PVRateLineReq.TrueRate` documents this
explicitly.

---

## 5. Date Arithmetic: 360-Day vs. 365-Day Edge Cases

**Status:** Go port matches DOS behavior — verified via refdata.json.

### Description

The DOS `YearsDif()` function computes the fractional year difference
between two dates. For 360-day basis, all months are treated as 30 days
with specific rules for end-of-month dates. The Go port replicates
these rules exactly.

Verified cases from `refdata.json`:

| From | To | 360-day | 365-day |
|------|----|---------|---------|
| 2024-01-01 | 2025-01-01 | 1.000000 | 1.002053 |
| 2024-01-01 | 2024-07-01 | 0.500000 | 0.498289 |
| 2024-01-15 | 2024-03-01 | 0.127778 | 0.125941 |
| 2000-01-01 | 2030-06-15 | 30.455556 | 30.453114 |

No discrepancies found.

---

## 6. exxp() and lnn() — Taylor Series Threshold

**Status:** Go port matches DOS behavior.

### Description

The DOS `exxp()` and `lnn()` functions use Taylor series approximations
for values very close to 0 (for exxp) or 1 (for lnn). The comment in
the DOS source says this compensates for a Turbo Pascal compiler bug
where `ln(1+x)` lost precision for small `x`.

The Go port replicates these thresholds (`|x| < 1e-4` for exxp,
`|x-1| < 1e-4` for lnn) even though Go's `math.Exp` and `math.Log`
do not have the same precision issue. This ensures identical output
for edge cases near zero rates.

All `exxp` and `lnn` test cases from `refdata.json` pass with full
precision matching.

## 7. Odd-first-period payment: DOS augments, Windows help does not

**Status:** Go port matches DOS (the financial authority), in BOTH prepaid modes.

### Description

When a loan has an *odd first period* — the first payment is not exactly one
compounding period after the loan date (a short or long first gap) — the regular
payment must be adjusted so the loan still amortizes over the stated number of
payments. The authoritative DOS engine refines the payment for this
(`EstimateAndRefinePayment` iterates the estimate, Amortize.pas:416). The Windows
help screens show the *un-adjusted* plain payment for the same inputs.

Example (AM Example 1): $100,000 @ 8%, 360 monthly payments, 30/360 basis, loan
dated 2024-02-12 with the first payment 2024-03-01 (a 19-day short first period):

| Quantity               | Windows help | DOS engine (authority) | Go port |
|------------------------|--------------|------------------------|---------|
| Regular payment        | $733.76      | **$731.98**            | $731.98 |
| Total interest         | $161,499.77  | **$163,513.81**        | $163,513.81 |
| First-payment interest | $422.22      | $422.22                | $422.22 |

Verified directly against the real DOS engine — and, importantly, DOS gives the
SAME $731.98 whether or not "1st interest prepaid" is set: the prepaid flag moves
the odd-period interest to a settlement stub in the schedule, it does NOT change
the solved payment:

```
legacy/oracle/amort_oracle 100000 0.08 360 12 loandmy=12.2.2024 firstdmy=1.3.2024          → 731.9828
legacy/oracle/amort_oracle 100000 0.08 360 12 loandmy=12.2.2024 firstdmy=1.3.2024 prepaid  → 731.9828
```

A *natural* first period gives the plain $733.76 in both DOS and the port — the
adjustment only applies to odd first periods.

> **Correction (2026-07-03).** A revision briefly claimed DOS-prepaid produced the
> plain $733.76 (from a Go-only actual-day closed-form prorate that had not yet been
> checked against the oracle) and shipped that as the default. Running the newly
> rebuilt Linux oracle proved DOS augments to $731.98 in BOTH modes; the change was
> reverted. The odd-first payment prorate is `firstPeriodProrate` (whole calendar
> month = a whole period even on 365; only a genuine odd-DAY stub uses the actual-day
> count), used by both the schedule and the closed-form RepayLoan solve.

### Go port decision

Per CLAUDE.md, the DOS version is the authority for financial logic, so the port
augments the payment to match DOS ($731.98). Pinned by
`TestAmortPrepaidFirstPeriodBothModes` (both prepaid modes → 731.98, to the cent),
`TestVerifyWebAM_EX1_Simple`, and UI cases AMZ-045 / AMZ-046.

## 8. Amortization "Exact method" setting — IMPLEMENTED (2026-06-19)

**Status:** RESOLVED for the in-arrears case. Implemented end-to-end and
validated row-for-row against the real DOS oracle. See
`docs/exact_groundzero_findings.md` and `docs/postmortem_365_exact_interest.md`.

### History

The "Exact method" toggle (`set-exact` in `index.html`) was previously inert:
the request never carried the flag, `HandleAmortizationCalc` hardcoded
`Exact: false`, and the engine never read `settings.Exact`, so selecting
"Exact: YES" silently did nothing. A client testing the 365 basis (expecting
true daily interest) reported the resulting payment as wrong.

### Resolution

Exact interest now accrues on the actual day count of each period
(actual/365.25) with an iterated payment solve, matching DOS (AMORTOP.pas:625
`YearsDif` branch; non-360 routed through `RepayFancyLoan`, Amortize.pas:1493).
The request carries `exact`, it is threaded into `Settings.Exact`, the engine
honours it (`exactDaily`), and the UI toggle is live. On the 360 basis Exact
remains a no-op, matching DOS.

Validated by `TestDOSGroundZeroRowCube`: the 365 exact schedule matches DOS to
the cent (rows ≤ 2.6¢, payment ≤ $0.30). One bounded corner remains —
**exact × in-advance** (annuity-due) — where true daily accrual is not yet
implemented; it is tracked with an envelope guard. See
`docs/exact_groundzero_findings.md` §4.

## 9. APR discounted the truncated schedule, not the full-term value walk — FIXED (2026-07-09)

**Status:** RESOLVED. The engine's APR now matches the DOS oracle to <½ bp
across interest-timing × basis × exact × points. Guarded by
`TestAPRVsOracleSweep` (`dos_apr_oracle_test.go`).

### Symptom

A client comparing a high-rate loan ($500,000 @ 33%, $15,000 payment, 360/12,
365/360 basis, interest paid in **advance**) saw the web APR read **33.0254%**
where the legacy app showed a higher figure. The balance-as-of matched the DOS
oracle to the cent, but the APR did not: for identical inputs the DOS source
oracle computes **33.0660%**. The gap was small for 365-family bases but large
elsewhere — on a **360 basis in advance the engine returned 37.01% vs the
oracle's 33.10%, a ~3.9-point error.**

### Root cause

APR was the one headline output never validated against the DOS oracle (the
oracle had no APR query until this fix added one — `pts=`/`apr` in
`amort_oracle.pas`). `ComputeAPRWithPoints` discounted `result.Schedule`, the
**display** schedule, which is *truncated at early payoff* — this loan retires
around payment 97 of 360. DOS computes the APR present value differently
(`EstimateAndRefineAPRwithPoints` → `RepayFancyLoan` in `value_calc` mode,
Amortize.pas:553-556): it discounts every regular payment across the **full
stated term**, letting the balance over-amortize negative past payoff, then
tacks on the terminal (negative) balance as a balloon (AMORTOP.pas:1224-1225).
Discounting the truncated tail instead under-counts it. Arrears was nearly
unaffected because its truncated tail almost coincides with the full-term one;
in-advance diverged materially. The plain 30/360 non-exact case takes a
different DOS path (`CalculateValueForPlainLoan`, Amortize.pas:493 — a
closed-form annuity of N equal payments over the full term with no balloon),
which the truncated schedule also failed to reproduce.

### Resolution

`aprValueCashflows` (backward.go) rebuilds the exact stream DOS discounts,
mirroring the Amortize.pas:553 dispatch: the fancy/exact/non-360 cases use the
**unforced full-term walk** (`generateFancyScheduleMode` /
`generateExactInAdvanceScheduleMode` with `unforced=true` — no final-row fold,
no early-payoff stop) plus a terminal-balance balloon row; the plain 30/360
non-exact case uses the N-payment full-term annuity. `ComputeAPRWithPoints` then
discounts that stream. The display schedule, payoff balance, and all other
outputs are unchanged — only the APR present-value input was corrected.

Validated by `TestAPRVsOracleSweep`: engine APR matches the DOS oracle within
5e-5 across {arrears, advance} × {360, 365, 365/360} × {exact on/off} ×
{0, 3 points}. The oracle gained a machine-readable `apr` query (with `pts=` to
enter points so the solver runs) so this output is now differentially guarded
like every other.

## 10. Rate solve clamped to positive — under-funded (negative-rate) loans failed — FIXED (2026-07-09)

**Status:** RESOLVED. `SolveRate` now solves negative rates and converges,
matching the DOS oracle. Guarded by `TestRateSolveVsOracle_Negative`
(`dos_rate_oracle_test.go`).

### Symptom

A loan whose payments total **less than** the principal has a genuinely negative
implied rate. Client case: $100,000 borrowed, 120 monthly payments of $750 =
$90,000 repaid → rate ≈ −2%. Leaving Loan Rate blank to solve it, the web
returned **0.0100%** with "Loan Rate solve did not converge — the value shown is
the closest the iterative refinement reached," and every downstream figure
(balance-as-of, totals, APR) was computed off that wrong 0.01%. DOS solves the
negative root cleanly (teal ≈ −2.2%).

### Root cause

Two positive-only clamps in `SolveRate` (backward.go) that DOS's `Iterate`
(AMORTOP.pas:1415-1493) does not have:

1. The closed-form Newton pinned the rate on every negative step
   (`if rate < 0 { rate = small }`), so it could never leave the ~0 neighborhood.
2. The schedule refinement discarded a correct negative root
   (`if ok && refined > 0`), falling through to the non-converged warning.

DOS's `Iterate` runs the Newton into negative territory and only bails when
`|rate| > 2` (±200%).

### Resolution

Replaced the positive clamp with DOS's divergence bound (break when `|rate| > 2`)
and accept a converged refinement anywhere in `(−2, 2)` including negatives. The
positive-rate first guess (`payamt·perYr/amount`, floored at 0.02) is unchanged,
matching DOS. Validated by `TestRateSolveVsOracle_Negative`: the engine's solved
rate matches the DOS oracle within 5e-5 across {arrears, advance} × {360, 365,
365/360} × {exact on/off} for the negative-rate loan. The oracle gained a
`solverate` query (blank the rate, solve from amount + payment + term) so this is
now differentially guarded. Positive-rate solves are unaffected (full suite green).

## 11. Plain-loan rate solve ignores prepaid interest — KNOWN, pre-existing

**Status:** OPEN (separate from §10; discovered during the §10 work).

The rate solve for a **plain** loan (arrears, and either a 360 basis or the
non-exact method — i.e. the cases that refine against the closed-form `RepayLoan`
terminal rather than the actual schedule) does not account for the
first-period interest a **prepaid** loan collects at settlement. The engine
returns the same rate whether "1st interest prepaid at settlement" is YES or NO,
whereas DOS shifts it slightly. This affects **positive** rates too, so it is not
a consequence of the §10 negative-rate fix — the same scenario just surfaced it.

Magnitude on the §10 loan (arrears, 360, prepaid): engine −2.0336% vs DOS
−2.0532% (≈0.02 points). Exact/non-360 loans route through the schedule-based
terminal, which already handles prepaid correctly and matches DOS; in-advance is
unaffected (prepaid does not change its solved rate). The fix would route the
plain prepaid rate solve through a prepaid-aware terminal; deferred as a distinct
change. `TestRateSolveVsOracle_Negative` skips the arrears+prepaid+plain combos
with a pointer here.

## 12. Balloon-amount solve: positive clamp + truncated schedule — FIXED (2026-07-10)

**Status:** RESOLVED. The target-balloon solve now matches the DOS oracle to the
cent for under- and over-funded loans. Guarded by `TestBalloonSolveVsOracle`
(`dos_balloonsolve_oracle_test.go`); the `solveballoon` oracle query was added to
make it testable.

### Symptom

A *terminating target balloon* (balloon date given, amount blank) on an
**over-funded** loan solved to **0**, when DOS solves a large **negative** amount
(the regular payment over-pays, so the balloon refunds the overpayment to land the
terminal balance on zero). Example: $100k @ 10%, 120 × $2000, balloon at month 120
— DOS −136,985.82, old engine 0.00.

### Root cause (two stacked bugs, same classes as §9/§10)

1. **Truncated schedule.** `SolveBalloonAmount`'s residual used
   `Amortize().FinalPrinc` — the *display* schedule, which retires early on an
   over-funded loan, so a balloon at/after retirement had no effect and the secant
   returned immediately at ~0. DOS evaluates the **full-term** walk
   (`EstimateAndRefineBalloon` → `RepayFancyLoan`), letting the balance
   over-amortize negative. (Same truncated-vs-full-term class as the APR bug, §9.)
2. **Positive clamp.** `if a2 < 0 { a2 = 0 }` pinned the secant at zero, so even
   with the full-term residual it could not reach the negative root. DOS's `Iterate`
   has no such clamp. (Same sign-clamp class as the rate bug, §10.) The same clamp
   was removed from `SolvePrepaymentAmount`.

### Resolution

`SolveBalloonAmount`'s `eval` now discounts the **unforced full-term terminal**
(`generateFancyScheduleMode(..., unforced=true).FinalPrinc`) and the ≥0 clamp is
gone. Validated vs the oracle across under/over-funded × {360, 365, 365/360}. A
side effect: the A-W5 "negative target balloon" advisory — previously dead code
because the clamp made a negative solve impossible — now fires correctly; two
coverage tests that asserted the old clamped-to-zero behavior were corrected.

## 13. Balance→date inverse diverges from DOS — logic FIXED, wiring pending

**Status:** Engine logic ported and verified for arrears (2026-07-10);
API/frontend wiring and the in-advance walk remain.

**Update (07-10):** `ComputeDateFromBalanceDOS` (engine.go) now implements DOS's
`ComputeDateFromBalance` rule — stop at the first payment whose `principal +
payamt` (arrears) or `principal + payamt - interest` (in-advance) drops below the
target, return that payment's date + the corrected balance. Verified to the DAY
against the oracle's `datefrombalance` query across the balance range for arrears
(`TestDateFromBalanceVsOracle_Arrears`). Two pieces remain: (1) wire the API +
frontend to call this instead of the client-side row-snap (the shipped path is
still the JS scan); (2) the in-advance base-date shift isn't fully reproducible
from the display schedule, so in-advance can be off by a period — a fully faithful
in-advance result needs the balance_calc walk. The naive `DateForBalance` row-snap
is retained but marked non-faithful.

**Original finding (confirmed and quantified during the §12 audit).**

The user-facing "enter a target balance → find the date it is reached" runs in
**client-side JS** (a scan for the first row whose balance ≤ target); the Go port
`DateForBalance` (engine.go) is a matching row-scan but is **dead code** (no
callers). DOS `ComputeDateFromBalance` (Amortize.pas:1153) instead walks the loan
and returns `nextpayment.date` with a mode-dependent corrected amount. Measured
divergence ($100k @ 10%, 120 × $1500, target $50,000): engine/scan returns
**12/01/2028 for both modes**, DOS returns **1/1/2029 (arrears)** and **2/1/2029
(in-advance)** — off by one to two months, and the scan does not distinguish
in-advance from arrears. The `datefrombalance` oracle query now exists to guard a
fix. **Action:** port `ComputeDateFromBalance` to a DOS-faithful server path, wire
the API/frontend to it, and add a differential test.

## 14. ARM / option loans returned APR = 0 — FIXED (2026-07-10)

**Status:** RESOLVED. Fancy loans (ARM adjustments, prepayments, stacked options)
now compute an APR matching the DOS oracle. Guarded by `TestFancyAPRVsOracle`.

### Symptom

Any loan routed to the structural DOS port (`AmortizeDOS` — ARM rate/payment
adjustments, prepayment series, and other stacked-option loans) returned
**APR = 0**, where DOS computes a real APR (e.g. an ARM to 13% at mid-term with 3
points → DOS 0.1135, engine 0.0000).

### Root cause

`AmortizeDOS` is invoked and its result **returned early** (engine.go:231), before
the piecewise engine's A9 APR block (engine.go:707). So the whole option domain
skipped APR entirely. Discovered by extending the APR differential guard to fancy
loans (`TestFancyAPRVsOracle`) — the coverage gap flagged in the audit.

### Resolution

Extracted the A9 APR computation into `applyAPR` (engine.go) and called it from
both the piecewise engine and the `AmortizeDOS` delegation (using the modal solved
payment from the schedule). Fancy-loan APR now uses the same full-term value walk
(§9) and matches the DOS oracle to <½ bp for balloon, ARM (rate up/down), and
prepayment loans with points.

### Also this session (§13 update)

The Balance→date inverse (§13) is now wired **server-side**: the amortization API
accepts `targetBalance` and returns `payoffDateSolved` via `ComputeDateFromBalanceDOS`
(`TestAmortTargetBalanceInverse`, arrears exact vs the oracle). Remaining: the
frontend still runs its client-side row-snap (change `updatePayoffDate` in
index.html to POST `targetBalance` and read `payoffDateSolved`), and the in-advance
base-date-shift walk.

## 15. Date primitives: DOS raw un-normalized dates vs Go `time.Time`

**Status:** Partly resolved (P1, P5 FIXED); one bounded corner documented (P2).

Source: the 2026-07-09 DOS-vs-Go logic-divergence audit (Fable model), which
compared `INTSUTIL.pas` / `VIDEODAT.pas` against `internal/dateutil` and verified
each finding directly against the real DOS engine via `amort_oracle intutil`
(`addn`, `noi`, `yearsdif`). The root theme: DOS stores dates as a raw
`{d,m,y}` record and tolerates transiently-invalid values (e.g. "Feb 30",
"Feb 29 2100"), whereas the Go port wraps `time.Time`, which normalizes any
invalid date forward at construction. Three consequences were found.

### 15a. `AddNPeriods` lost DOS's month-clamp on the year-jumped date — FIXED (P1)

When a monthly-family date is advanced by whole years, DOS keeps `firstdate.d`
on the year-jumped date even if it overflows the target month (Feb 29 in a
non-leap year survives as a raw "Feb 29"), then either `CheckForDaysTooLarge`
clamps it (whole-year case) or `AddPeriod` re-derives the day from the origin
(partial case) — INTSUTIL.pas:1398-1405. Go's `time.Date` instead normalized the
overflow FORWARD (Feb 29 → Mar 1), which corrupted the base *month* and shifted
every subsequent period by a full month.

Verified vs the real DOS engine:

```
amort_oracle intutil addn 2024 2 29 12 12  → 2025-02-28   (Go was 2025-03-01)
amort_oracle intutil addn 2024 2 29 12 13  → 2025-03-29   (Go was 2025-04-29 — a MONTH off)
amort_oracle intutil addn 2024 2 29 12 1   → 2024-03-29   (already matched)
```

**Fix:** `AddNPeriods` (`internal/dateutil/dateutil.go`) clamps the day to the
target month's length *before* constructing the year-jumped date, so the month
is preserved. Guarded by `TestDOSAddNPeriodsSweep` (6,600-case differential vs
the oracle, 0 mismatches) and `TestDOSAddNPeriodsFeb29Clamp`
(`internal/dateutil/dos_addnoi_oracle_test.go`). Callers that derive a LastDate
(`amortization/firstpass.go`, `engine.go`, `backward.go`;
`presentvalue/backward.go`) now match DOS for Feb-29-dated first payments.

### 15b. `NumberOfInstallments` missing the "forever → maxint" short-circuit — FIXED (P5)

DOS returns `maxint` WITHOUT snapping the to-date when the terminal is in the
sentinel/latest year (2149) — INTSUTIL.pas:1026-1028. The port computed a finite
count instead (~1482 for a monthly series) AND snapped the to-date off the
sentinel, which both truncated a converging perpetuity and defeated the
forever-detection in `PeriodicSummation` (which keys on `toDate == latest`).

Verified: `amort_oracle intutil noi 2026 1 1 2149 6 15 12 on_or_before` →
`n 2147483647 last 2149 6 15` (count = maxint, date untouched).

**Fix:** `NumberOfInstallments` returns `math.MaxInt32` with `l` unchanged when
`l.Year() == 2149`. Downstream PV valuation is closed-form (`SumFormula`), so the
sentinel count yields the convergent infinite-series limit rather than iterating;
every consumer loop is otherwise bounded by a date walk to ≤2149. Guarded by
`TestDOSNumberOfInstallmentsForeverGuard`. (Sub-note: in the degenerate
`|rate − cola| < 1e-10` branch the value scales with the maxint constant, which
differs between 16-bit DOS's 32767 and the FPC oracle's 2147483647; that exact-
equality forever corner is not well-defined across DOS builds and is immaterial
for any meaningfully-converging series.)

### 15c. `NumberOfInstallments` snapped "Feb 30" terminal — FIXED 2026-07-25 (P2)

When the snapped last payment date would carry the origin's day into a shorter
month (day 29/30/31 landing in February) with the origin NOT on a month-end, DOS
keeps the raw overflow — e.g. **Feb 30** — and its 30/360 `YearsDif` treats that
as a clean whole month (`YearsDif(Feb 30, Jan 30) = 1/12` exactly). Go's
`time.Time` cannot represent Feb 30 and normalizes it to **Mar 2**, adding ~2/360
of a year of interest on that single anchor.

```
amort_oracle intutil noi 2025 1 30 2025 2 5 12 on_or_after → n 2 last 2025 2 30
amort_oracle intutil yearsdif 2025 2 30 2025 1 30          → 0.083333333333  (= 1/12)
Go NumberOfInstallments(...)                                → n=2  last=2025-03-02
Go YearsDif(2025-03-02, 2025-01-30, 360)                    → 0.088888888889
```

The installment **count** is unaffected and matches DOS. The date feeds a
valuation only in PV's `asOf > fromDate` (retrospective) branch of
`PRESVALU.pas` `Summation` — the `since_from := false` arm, which is the sole
place that routine passes `todate` to `YearsDif`, and it does so **twice**:

```pascal
since := YearsDif(asof, todate);                                   { discount exponent }
if (cola<>0) then sum := sum * exxp(YearsDif(todate,fromdate)*cola); { COLA range }
```

**Originally deferred**, on the theory that a ≤2-day 30/360 effect on one anchor
was not worth an engine-wide representation change. **PV fuzzer5 seed 20404
(2026-07-25) refuted the "negligible" half of that reasoning:**

```
pv_oracle table 0.0357760000 360 both 4,10 cnt asof=11.6.2026 \
  lump=3.4.2024:71322.19 lump=5.6.2035:97554.79 lump=31.10.2042:174021.12 \
  per=8.2.2027:8.9.2054:1:124983.44:0.0000000000 \
  per=30.8.2024:30.4.2048:2:163986.02:0.0215740000 \
  per=29.9.2024:1.12.2149:2:45837.43:0.0042010000
DOS 12667674.552427   port 12667389.295758   Δ -285.256669
```

Every table line agreed; only the closed-form screen total was wrong. Ablation
pinned it to the single row `per=30.8.2024:30.4.2048:2:163986.02:0.021574`. For
`f = 30.8.2024`, `l = 30.4.2048`, `peryr = 2`, `on_or_before`: `flast = false`,
`mdiff = (4-8) mod 6 = 2`, so `l.m := 2` and then `l.d := f.d = 30` — the phantom
**30/2/2037**. Both `YearsDif` reads above were then short by exactly `2/360`,
and `exp(-0.035776·(2/360)) · exp(ln(1.021574)·(2/360)) = 0.99991985` against the
observed ratio `0.99991982`.

**Fix.** Rather than change the engine's date representation, the raw fields are
threaded only to the arithmetic that can see them:

- `dateutil.NumberOfInstallmentsRaw(f, l, peryr, z) (n, y, m, d)` carries the
  real implementation and returns DOS's three raw fields. `NumberOfInstallments`
  is now a thin wrapper that normalizes, so its ~20 comparison-only callers are
  untouched.
- `dateutil.YearsDifRawA(z, ay, am, ad, …)` is the mirror of the existing
  `YearsDifRawZ` (added for the stepped-COLA case in §15's sibling note),
  implemented as `-YearsDifRawZ(a, z, …)` by DOS's own antisymmetry
  (`if DateComp(a,z)>0 then YearsDif := -YearsDif(a,z)`).
- `presentvalue.PeriodicSummationRawTo` takes the To date twice — normalized for
  `DateComp`/`Equal`/the exact-mode walk, raw for the two `YearsDif` calls in the
  `since_from = false` branch. `PeriodicSummation` delegates to it with the
  normalized date's own fields, so unrelated callers are unaffected.
- `PeriodicPayment.toRawM/toRawD` (with `rawTo`/`setRawTo`) preserve the phantom
  from the snap site to the summation site. The year is never phantom: the snap
  only overwrites month and day, and a day overflow can occur only in a month
  shorter than 31 days, so it never carries into January.

`pp.ToDate` deliberately stays **normalized**. The phantom differs from its
normalization only by naming a day past the month end, and every payment on the
grid is anchored to the from-date's day, so no payment date can fall strictly
between the two — `DateComp`, the table walk, `LastDayFn` and
`SummationForSteppedCola`'s comparisons are all observationally identical. The
table lines already matched DOS exactly and must not regress.

Verified: the seed-20404 worksheet now agrees to `Δ0.000000`, as do the nine
to-date variants swept across the divergent and previously-agreeing corners.
Characterized by `TestNumberOfInstallmentsFeb30`.

A related representation limit — DOS's synthetic **Feb 29 2100** (its calendar
treats 2100 as leap) — is unrepresentable in `time.Time` for the same reason; it
only bites day-29 schedules crossing Feb 2100 and remains bounded and unfixed.

## 16. Mortgage `Financed > Price`: DOS refuses, port now refuses too — FIXED

**Status:** Resolved — the Go port matches DOS's refusal.

Audit finding F1 (2026-07-09). When Price is given and the entered Amount
Borrowed exceeds it, DOS FirstPass calls `RecordError` (Mortgage.pas:179-184),
which sets `errorflag` (INTSUTIL.pas:1065); `CalculateRows` then skips `Calc`
(Mortgage.pas:1128-1134), so the row produces **no computed cells** — only the
message "Amount borrowed cannot exceed price."

An earlier revision (and `dispatch_gaps.md` V6-6) claimed DOS "flags but still
computes a negative % Down", and the port emitted a warning plus computed
negative outputs. That rested on a misreading of `RecordError` as not setting
`errorflag`. Verified against the real DOS engine:

```
mtg_oracle mfin 100000 120000 30 0.07  → ERR Amount borrowed cannot exceed price.   (financed 120k > price 100k)
mtg_oracle mfin 100000  80000 30 0.07  → monthly 533.34 …                            (financed 80k < price — solves)
```

**Fix:** `mortgage.Calc` (`internal/finance/mortgage/mortgage.go`) returns the
error "Amount borrowed cannot exceed price." and no outputs. Pinned by
`TestCalcFinancedExceedsPrice` and `TestCalc_WarningsAndComputeBranches`
(both updated from the old warn-and-compute expectation).

## 17. Mortgage APR comparison rejected valid amount-borrowed + payment rows — FIXED

**Status:** Resolved — the compare handler now accepts these rows.

Audit finding F2 (2026-07-09). DOS's comparison path (MortgageScreenUnit.pas:
780-791) runs `Calc` on each row for side effects but gates the comparison ONLY
on `EnoughDataForAPR` — financed, monthly, rate, years (Mortgage.pas:571-575). A
row given as {Amt Borrowed, Monthly, Rate, Years} with no Price funding therefore
compares fine, even though `Calc` "refuses" to solve its price (the "Fill in
Percent Down or Cash Required" message, which does NOT set `errorflag`). The
classic "compare two loan offers by amount + payment" is exactly this shape.

The Go handler `HandleMortgageCompare` called `mortgage.Calc` first and returned
HTTP 400 on its refusal, so it rejected comparisons DOS performs. `CompareAPRs`
itself already gated correctly on `EnoughDataForAPR`; only the handler was wrong.

Verified against the real DOS engine (new `mtg_oracle aprfin` mode drives the
genuine `ReportAPR` on a financed+monthly row):

```
mtg_oracle aprfin 160000 1100 30 0.07   → apr 0.0732840000   (no price — DOS reports, doesn't refuse)
mtg_oracle aprfin 160000 1080 30 0.0675 → apr 0.0714400000
```

**Fix:** the handler now uses the computed line when `Calc` succeeds (for
price-funded rows it fills derived Financed/Monthly) and falls back to the raw
input line on a `Calc` refusal, letting `CompareAPRs`' `EnoughDataForAPR` gate
decide. Pinned by `TestMortgageCompareAcceptsFinancedMonthlyRows` (APRs anchored
to the oracle values) and `TestMortgageCompareStillRejectsUnderspecified` (a row
missing Monthly is still a clean 400).

## 18. Amortization logic audit round 2 (2026-07-11) — schedule walk + backward solves

Source: the second amortization sweep of the DOS-vs-Go logic audit (two module
audits: forward schedule engine; backward solvers). Every item below was
CONFIRMED against the real DOS oracle **and re-verified against the current
tree** (the 2026-07-10 fixes — negative-rate solve, balloon-amount solve, ARM
APR — resolved two earlier candidates before this round; those are NOT listed).
Statuses: OPEN = real divergence awaiting a fix decision; DOC = behavioral
difference recorded as deliberate/bounded.

### A1 — FIXED (2026-07-11): `periodYearFraction` misses DOS's `DaysCloseEnough` month-end rule (V6-7 severity was wrong)

DOS `ComputeNext` (AMORTOP.pas:625-632) takes a whole-period `timedif` whenever
`DaysCloseEnough` (INTSUTIL.pas:716-727) holds — which includes **end-of-month
clamped pairs** (Jan 31→Feb 29→Mar 31) and the semimonthly 15th/month-end pair,
and applies on BOTH bases when the method is non-exact. Go's shortcut
(`periodYearFraction`, engine.go, also used by the DOS-port walk) requires exact
day-of-month equality and excludes peryr=24 entirely, so month-end fancy loans
fall to actual-day `YearsDif` around February:

```
amort_oracle 100000 0.12 24 12 loandmy=31.12.2023 firstdmy=31.1.2024 targ=0.01 rows
  DOS: payment 4707.3472 | 2/29/24 int 962.93 | 3/31/24 int 925.48
  Go : payment 4706.5447 | 2/29/24 int 930.84 | 3/31/24 int 956.02   (~$32/row)
Semimonthly (15th/month-end): DOS 2/29/24 int 393.79 (=1/24yr) vs Go 367.54 (14/360).
```

Trigger: any per-period-walk loan (any advanced option, USA/exact/non-360
routing, in-advance) with payment day 29/30/31, or any semimonthly loan — very
common (month-end closings). Every existing cube/fuzzer builds day-1 first
dates, which is why this survived. `dateutil.DaysCloseEnough` is already ported
— the schedule engines just don't call it. Supersedes V6-7's
"presentation-grade" classification (`docs/dispatch_gaps.md` updated).

### A2 — FIXED (2026-07-11): extras dated after the last regular payment — piecewise engine emits phantom regular payments

DOS: once the walk passes `lastdate`, the next extra is forced off-cycle and the
walk jumps straight to it (AMORTOP.pas:602-613) — a trailing balloon gets one
row with all intervening interest. Go's piecewise `generateFancySchedule` keeps
paying the full regular payment until `veryLast`, and every materially-trailing
balloon reaches the piecewise engine (the DOS-port walk's on-grid cap rejects
it):

```
amort_oracle 100000 0.08 24 12 b30=50000   (balloon 6 months past payment 24)
  DOS: payment 2668.85, interest 14052.47, last rows 1/1/26 then 7/1/26 (balloon only)
  Go : payment 2245.62, interest 15122.86, five phantom regular rows 2/1/26-6/1/26
```

Payment off $423; totals off $1,070; the blank-payment solve is polluted by the
phantom terminal. Distinct from the Rev-10 off-cycle-balloon fix (which handles
balloons BETWEEN payments) and the NN-trailing prepayment fix.

### A3 — FIXED (2026-07-11): coincident-extra rows under target / moratorium drop DOS's `payamt − d` term (piecewise engine, REPLACE mode)

DOS ComputeNext (AMORTOP.pas:639-645), when an extra COINCIDES with a regular
payment and the moratorium/target floor fires, keeps the extra component:
`payamt := payamt − d + targ^.target + interest` (resp. `− d + interest` under
moratorium). Go's piecewise engine applies a flat floor `pmt = target + interest`,
losing `payamt − d`. The DOS-port walk has the correct formula; REPLACE-mode
(balloon-includes-regular = YES, the DOS default), off-grid extras, in-advance,
and exact all route to the piecewise engine.

```
amort_oracle 100000 0.08 60 12 pay=1500 targ=800 b12=500
  DOS row 1/1/25: pay 403.48 (= 500−1500+800+603.48) int 603.48 bal 90721.58
  Go  row 1/1/25: pay 1403.48                        int 603.48 bal 89721.58
```

### A4 — FIXED (2026-07-11): R78 × in-advance — DOS gives R78 precedence; Go disables R78 AND skips the annuity-due payment refine

DOS MakeTable checks `r78` before `in_advance` (Amortize.pas:1505-1543): both
flags → R78 sum-of-digits split seeded from the **in-advance-solved** payment.
Go gates R78 off when InAdvance (`engine.go` `r78 := settings.R78 &&
!settings.InAdvance…`) and `needPaymentRefine` returns false whenever R78 is
set, so the payment is the plain in-arrears annuity:

```
amort_oracle 100000 0.10 24 12 r78 inadv → DOS payment 4579.8857, row1 int 793.38, total 10750.59
Go                                       →     payment 4614.4926, row1 int 833.33, total 10665.15
```

Controls: each flag alone matches to the cent. Both flags are one click apart in
the UI. The GroundZero cube treats {ordinary, in-advance, R78, USA} as mutually
exclusive methods, so the cross was never swept.

### A5 — FIXED (2026-07-11): Amount/Rate solves were blind to skip-months, moratorium, and target (up to 75% off)

DOS `EstimateAndRefineRate` always Iterates, and the amount shortcut requires
`prepaid` — both against the FANCY terminal whenever any option is set
(skip/mor/target all set fancy). Go's `hasFancyOptions` (fancybisect.go) checks
only prepayments/adjustments/balloons, so `needScheduleRefine` skips refinement
for skip/mor/target-only loans and returns the option-blind closed form:

| case | DOS | Go (current tree) |
|---|---|---|
| amount, skip 6-8 (`noamt pay=888.4879 skip=6-8`, 24mo@12%) | 14134.97 | 18874.49 |
| rate, skip 6-8 (100k, 360mo, pay 733.7646) | 0.05213 | 0.08000 |
| amount, moratorium 12mo (24mo@12%, pay 500) | 6066.87 | 10621.69 |

(The blank-PAYMENT solves for these options are oracle-clean — only amount/rate.)
Likely one-line fix shape: include Moratorium/Target/SkipMonths in
`hasFancyOptions`; needs the full oracle sweep for blast radius.

### A6 — FIXED (2026-07-11, extends §11): prepaid Amount/Rate solves ignore the settlement-stub first-period shift on ODD first gaps (~2%)

§11 records the natural-first-period prepaid rate shift (~0.02 pts). The audit
found the odd-first-gap case is ~100× larger and hits the AMOUNT solve too: DOS
pins `prorate := 1` for prepaid (Amortize.pas:1277-1287) while Go's plain-loan
terminals (`dosIterateAmount`/`dosIterateRate` → `RepayLoan`) apply the odd-first
prorate from raw dates:

```
noamt pay=888.4879 prepaid first=3 → DOS 10000.00 | Go 9805.83
norate pay=888.4879 prepaid first=3 (10000) → DOS 0.120000 | Go 0.091551
```

Non-prepaid controls match exactly. Re-verified on the current tree (the §10/§12
fixes do not cover this).

### A7 — FIXED (2026-07-11): over-determined N + LastDate — DOS lets N win; Go's fancy walk truncates at the stale LastDate

DOS FirstPass unconditionally overwrites `lastdate` from first+N
(Amortize.pas:218-226). Go's FirstPass keeps a user-supplied LastDate
(`firstpass.go` gates on `LastStatus < InOutDefault`), and the fancy walk bounds
at it: `10000 @12% ×24, pay 488, balloon@6mo, lastDate=2025-01-01` → DOS 24 rows
/ interest 854.89 (lastdmy ignored); Go 12 rows / 780.66. Simple (non-fancy)
loans agree — fancy only.

### A8 — FIXED (2026-07-11): prepaid term solve off-by-one on odd first gaps

DOS `solveNPeriods` uses the global `prorate` (= 1 for prepaid,
Amortize.pas:1277-1282); Go's `solveNPeriodsFromPayment` always derives prorate
from raw dates. `10000 @12%, pay 888.4879, prepaid, first 3mo out` → DOS n=12
(last 2025-03-01); Go n=13 (2025-04-01). Non-prepaid control matches (n=13).

### A9 — FIXED (2026-07-11): FirstPass derives N by rounding YearsDif instead of DOS `NumberOfInstallments` snapping

`firstpass.go` A-FP-n: `n = round(YearsDif360(first,last)×perYr)+1` rounds UP
across a payment boundary that DOS's month-end-aware snap rounds DOWN:
first 1/15/2024, last 3/12/2025 → DOS `intutil noi … on_or_before` = **14**
(snapped 2/15/2025); Go n=15. The faithful `dateutil.NumberOfInstallments`
exists and is used elsewhere — FirstPass just doesn't call it.

### A10 — FIXED (2026-07-11): missing `|rate| > 2` iteration brake

DOS aborts a rate solve at `abs(rate) > 2` with "did not converge"
(AMORTOP.pas:1485); Go converges and returns e.g. 2.78 (277%) with
`converged=false` for a payment 3× the principal. Go's answer is arguably more
informative; record as a deliberate divergence OR add the brake for parity —
needs a product call. Low stakes (absurd inputs only).

### A11 — DOC: DOS fancy term solve dies past its Y2K horizon (2019); Go solves correctly

When N and LastDate are both blank on a fancy loan, DOS's open-ended walk stops
at year `100 + centurydiv−1` = **2019** (AMORTOP.pas:1143-1147), so any
modern-dated loan errors "Payment amount is too small to compute number of
periods" — while the same loan dated 1994 solves (n=16 for the skip case,
n=28 for the moratorium case). Go solves modern dates and matches DOS's
pre-2019 answers exactly (n=16 / n=28) — i.e. it matches DOS's *intent*,
not its Y2K artifact. Deliberate non-reproduction; the CLAUDE.md "flag
Y2K-style date assumptions" rule applies.

### A12 — CONFIRMED and FIXED (2026-07-11): discount points missing from the settlement row and total interest

DOS MakeTable's loan-date settlement line fires for points too (not just
prepaid): `interest := PrepaidInterest + points×amount` (Amortize.pas:1476-1491)
and flows into total interest. Go uses Points only for the APR. Not yet
oracle-verifiable (no points-in-schedule token; adding one touches `legacy/`).
Park until an oracle extension is sanctioned.

### Verified clean in this round (oracle-matched)

ComputeNext structure (skip zeroing, balloonpos, plus_regular merge, mor/target
else-if precedence, usap clamp); skip-month string parsing incl. wrap-around
ranges; no-target sentinel; R78 non-360/exact routing; trailing balloon in the
DOS-port walk; weekly/biweekly 360→365 coercion; semimonthly non-Feb rows;
simple-loop prepaid/in-advance/hard-payment/early-payoff/R78 parity;
PrintAndReset very_last fold; adjustment firing between payment dates; plain
amount/rate/term solves; non-prepaid odd-first amount/rate; balloon amount
solve; balloon rate solve (fixed 2026-07-10); negative plain rate solve (fixed
2026-07-10); term solves for in-advance and skip/mor (vs DOS-1994 semantics);
hard-payment × rate-solve interplay; over-determined N+LastDate on simple
loans; the 16-pattern field-presence eval cross-product.


### §18 resolution round (2026-07-11) — all fixable items closed

Every finding above except A11 is now FIXED and oracle-guarded; the full
`PERSENSE_REQUIRE_ORACLE=1` suite passes (amortization cubes, the ~3,300-case
in-advance fancy fuzzer, and the new per-finding regressions). Summary of the
fixes and their tests (each test comment cites the oracle command + output per
the provenance rules):

- **A1** `periodYearFraction`/`firstPeriodProrate` now gate on the ported
  `dateutil.DaysCloseEnough` (with DOS's semimonthly ±half-month adjustment),
  and the off-cycle drain row uses the same fraction — `TestA1MonthEndDaysCloseEnough`,
  `TestA1SemimonthlyFebRow`.
- **A2** the fancy walk stops emitting regular payments past the last regular
  date and drains trailing extras at their own dates; the solver's unforced
  terminal no longer caps an off-cycle extra at the balance (which made the
  residual flat-zero and the payment solve land $1.20 off) —
  `TestA2TrailingBalloonNoPhantomRows`.
- **A3** moratorium/target floors moved AFTER the extras merge and keep DOS's
  `payamt − d` term on coincident rows — `TestA3CoincidentExtraTargetFloor`.
- **A4** R78 wins over in-advance (rendering no longer disabled; the payment
  refine solves the annuity-due payment; the shared in-advance settlement row
  is kept) — `TestA4R78InAdvance`.
- **A5** `hasFancyOptions` now includes skip-months / moratorium / target, so
  Amount/Rate solves refine against the option-aware terminal — `TestA5`.
- **A6/A8** `prepaidNaturalStartShift` (the payment solve's stub rule) is now
  shared by the amount/rate terminals and the term solve — `TestA6A8PrepaidOddFirstSolves`
  (including the short-first no-shift control at 10063.10).
- **A7** FirstPass overwrites a user LastDate from first+N (N wins), matching
  Amortize.pas:220-226 — `TestA7NWinsOverLastDate`; the old
  `TestValidateFirstAfterLast` expectation (error on the over-determined row)
  was itself a divergence and now asserts the DOS compute-normally behavior.
- **A9** FirstPass derives N via `dateutil.NumberOfInstallments` (snapping the
  last date, VAR-param semantics) — `TestA9NDerivationSnaps`. A "forever"
  (year-2149) last date is refused with a clear message rather than DOS's
  maxint-row attempt.
- **A10** the rate solve refuses beyond ±200% with DOS's "did not converge"
  wording — `TestA10RateBrake`.
- **A12** was CONFIRMED with the HEAD oracle's `pts=` token (`10000 0.12 12 12
  pts=0.02 dumpraw` → L0 int 200.00, totals 861.85): `applyPointsSettlement`
  adds `points×amount` to the settlement line (combining with a prepaid stub
  into ONE row, DOS Amortize.pas:1476-1491) in both engines — `TestA12PointsSettlement`.
- **A11** remains a documented deliberate divergence (DOS's 2019 Y2K term-solve
  horizon is not reproduced; Go matches DOS's pre-2019 answers).

The oracle harness gained `noamt` / `norate` / `noterm` / `non` / `lastdmy=`
tokens (emitting `solvedamount` / `solvedrate` / `solvedterm` after the totals)
so these solves stay differentially guardable.

## 19. Adjudication of the pre-existing `TestAPIAmortBalloonAmountEchoed` failure (2026-07-11) — three findings, all FIXED

The test had been failing at HEAD ("self-amortizing target balloon = 120266.39,
want ~0"). Driving the real DOS engine on the same input (new non-halting
`dateballoon=` oracle token) showed BOTH the test's expectation AND the
engine's behavior were divergences, and pulling the thread surfaced a third.

### 19a — Unknown balloon/prepayment amounts require a KNOWN payment (dispatch)

DOS's `SufficientDataOnScreen` (Amortize.pas:889-895) admits a screen with an
unknown balloon (`unkballoon > 0`) or unknown prepayment (`unkpre > 0`) only
when `payamtstatus >= defp` — otherwise the table is refused ("not enough
data"). Verified:

```
amort_oracle 100000 0.06 120 12 dateballoon=37     → no schedule, payment unsolved, balloon status empty
amort_oracle 100000 0.06 120 12 presolve=6:12:12   → prepay 0.0000 (unsolved)
amort_oracle 100000 0.06 120 12 payhard=800 presolve=6:12:12 → prepay 3265.5268
```

The port previously let the unknown balloon/prepayment claim the solve with the
payment left blank (effectively $0 regular payments), producing a nonsensical
120266.39 balloon on a plain self-amortizing loan; the old test expectation
(solved ~0) was equally non-DOS. **Fixed:** the Amortize dispatch now refuses
with a "not enough data" error when an unknown balloon or prepayment is present
and the payment is blank. UI case AMZ-073 (which encoded the old Go behavior)
now expects the refusal. Guards: `TestA13UnknownBalloonNeedsPayment`,
`TestAPIAmortBalloonAmountEchoed` (rewritten).

### 19b — Iterative solve walks are UNROUNDED (DOS Iterate's hard_payment=false)

DOS's `Iterate` begins with `hard_payment := false; {temporarily, for
iteration}` (AMORTOP.pas:1433): the per-row Round2 ("Dav Holle provision")
applies to the DISPLAY schedule only, never to a solve's residual walk. The
port's balloon-amount eval ran the unforced walk with rounding on, which
quantized the terminal (~2¢ on the case below) and shifted the solved root:

```
amort_oracle 100000 0.06 120 12 payhard=1110.21 solveballoon=37 → balloon 1109.6700
Go (rounded eval, before): 1109.6863 | Go (unrounded eval, after): 1109.6705
```

**The nuance:** `EstimateAndRefineBalloon` (Amortize.pas:628-663) has two
paths. A balloon ON `very_last` (terminating) uses a CLOSED FORM computed from
the schedule at balloon=0 — outside Iterate, so THAT walk keeps rounding (the
§12 over-funded golden −300757.72 comes from the rounded walk and still
stands). Only the mid-term path Iterates unrounded. The port now blanks the
hard-payment status in the solve eval only for balloons dated before the last
regular payment. Guards: `TestA13SolveWalksUnrounded` (balloon + prepayment
solves pinned to the oracle to ≤0.01), `TestSolveBalloonAmountOverFundedNegative`
(terminating path unchanged).

### 19c — A solved mid-term balloon ON a payment date covers the payment it replaces

Not a code change, but recorded because the old test expectation (~0) shows the
intuition trap: in REPLACE mode (DOS `plus_regular=false`, the oracle default)
a date-only balloon landing on a regular payment date must cover the regular
payment it displaces — DOS solves 1109.67 on a self-amortizing loan, not ~0.
Note the API's own default is the ADDITIVE mode (`plusRegular =
!balloonIncludesRegular`, and the request field defaults false) — opposite of
the oracle's REPLACE default; API callers must set `balloonIncludesRegular:
true` to reproduce oracle runs. Whether the API default should flip to match
the DOS setting default is a UI/product question, flagged here.

## 20. Amortization audit pass 1 (2026-07-11) — 6 confirmed divergences, ALL FIXED (same day)

First pass of the iterative "audit until 3 consecutive clean passes" campaign,
focused on corners the earlier sweeps did not cover: amortization APR, payoff /
balance-from-date, US-Rule internals, ARM fine structure, and error parity.
~160 oracle-vs-Go comparisons. All items below are CONFIRMED against the real
DOS engine and NOT yet fixed. (Clean-pass counter: 0.)

### 20a — FIXED: in-advance APR misses DOS's forced `prepaid := true` (~0.22–0.29 pts low)

DOS FirstPass forces the `prepaid` global ON for in-advance loans
(Amortize.pas:206-209), so the APR target subtracts the annuity-due settlement
interest: `target := amount·(1−points) − PrepaidInterest` (Amortize.pas:547,
AMORTOP.pas:181-182). Go's `PrepaidInterest` returns 0 unless `settings.Prepaid`
(engine.go:57-59, used by applyAPR).

```
amort_oracle 100000 0.10 120 12 payhard=1500 inadv pts=0.03 apr          → DOS 0.141184 | Go 0.138956
amort_oracle 100000 0.10 120 12 payhard=1500 inadv b60=20000 pts=0.03 apr → DOS 0.110437 | Go 0.107513
```

Root cause proven single: substituting netProceeds = amount·(1−pts) −
amount·rate·YearsDif(first,loan) reproduces DOS to 6 decimals in both cases.
Trigger: any in-advance loan with points. Fix shape: apply the DOS forced-prepaid
rule inside the APR path (and see 20e — same root in payoff).

### 20b — FIXED: REPLACE-mode balloon + under-funded hard payment — terminal fold missing, loan left unretired

DOS folds the whole residual into the final scheduled payment (display very-last
fold, AMORTOP.pas:~1004) and retires to $0; Go's fold branches (engine.go:
2228-2272) cover plainFancy/ARM/plain-in-advance but not balloon loans, so the
schedule ends with the residual as a final balance and totals short by it.

```
amort_oracle 100000 0.08 240 12 payhard=600 usa b60=20000 → DOS paid 235543.41, retires (final row L239: 72743.41 → bal 0.00)
Go: rows match to the cent, but paid 163400.00, final balance 72143.41
Non-USA control: DOS paid 238513.72 | Go leaves bal 75113.72
ADD-mode control (plusreg): both 237129.54 — Go folds correctly there.
```

Trigger: hard payment below interest + balloon, balloon-includes-regular = YES
(the DOS default). Sharpens the documented A10/R4-8 "TackOnFinalBalloon —
advisory only" item (dispatch_gaps.md:1204): REPLACE mode neither folds nor
flags, and the divergence is in totals/balance, not just presentation.

### 20c — FIXED: payoff on plain USA-rule neg-am loans — DOS's payoff walk applies `usap`, its display doesn't

DOS ComputeBalanceFromDate always runs RepayFancyLoan (USA: interest on p−usap;
Amortize.pas:1107-1132), while DOS's DISPLAY schedule for a non-fancy USA loan
takes the simple loop where usap is inert (verified: DOS `usa` and non-usa
display dumps are byte-identical). Go's PayoffBalance reads the display schedule
(payoff.go:127-137), so it matches DOS's screen but not DOS's payoff number.

```
100000 0.08 360 12 payhard=600 usa payoff=1.7.2026  → DOS 102612.9862 | Go 102805.9340
                          payoff=15.1.2029          → DOS 104323.7562 | Go 105224.8108
                          payoff=1.1.2054           → DOS 124760.7602 | Go 199957.0690
```

Fancy USA neg-am payoff matches exactly (both walks apply usap). Trigger: USA +
payment below interest + payoff, no advanced options. DOS is internally
inconsistent here (payoff ≠ its own displayed balance) — needs a fidelity
decision: follow DOS's payoff walk, or document as deliberate.

### 20d — FIXED: payoff `RateInForce` — DOS accrues the stub at the NEXT/only adjustment's rate, never the base rate

DOS RateInForce (Amortize.pas:617-626) scans forward and returns
`adj[i]^.loanrate` for the first adjustment dated AFTER the payoff date — the
base rate is unreachable once any adjustment exists; between two ARMs it uses
the SECOND one's future rate; for a payment-only AO6 row it uses the implied
rate DOS solved. Go's payoffRateInForce (payoff.go:191-209) returns the most
recent adjustment on-or-before, else the base rate.

```
100000 0.08 360 12 adj=48:0.09: payoff=1.2.2024 → DOS 100750.0000 | Go 100666.6667
two ARMs, payoff between them                    → DOS 88585.3257 (11%!) | Go 88439.0246 (9%)
```

Trigger: any ARM loan + payoff on a date not after the last adjustment. NOTE:
DOS's "next adjustment's rate for a stub before it" is financially odd — the
fix decision (reproduce vs document as deliberate) needs a product call.

### 20e — FIXED: in-advance payoff BEFORE the first payment misses the PrepaidInterest rebate (same root as 20a)

DOS subtracts PrepaidInterest for prepaid loans (Amortize.pas:1097-1100), and
in-advance forces prepaid ON. Go gates on s.Prepaid (payoff.go:73-78).

```
100000 0.10 120 12 inadv payoff=15.1.2024 → DOS 99555.5556 | Go 100388.8889 (Δ = the 833.33 settlement interest)
```

Distinct from the OPEN in-advance payoff-walk frontier — this branch never
reaches the walk and is precisely fixable.

### 20f — FIXED: prepayment series starting ON the loan date — DOS refuses, Go computes

DOS: `DateComp(loandate, startdate) >= 0 → "Your dates are out of order."`
(Amortize.pas:1231-1237, equality included). Go validate.go:78-88 rejects only
strictly-before. `100000 0.08 120 12 pre=0:12:12:500` → DOS ERR | Go computes
(48928.84 interest). Extends V6-9 (which covered only strictly-before).

### 20g — DELIBERATE (documented): DOS moratorium payment-solve fails at an isolated input

`100000 0.10 120 12 mor=12` → DOS emits no table (Newton failure at exactly
rate 0.10; 0.0999/0.105/0.09/360-period variants all solve). Go solves 1399.8922
and retires to $0. Same family as A11 (DOS numeric fragility not reproduced);
record as deliberate after a product call.

### Pass-1 areas verified CLEAN (oracle-matched)

Fancy/settings APR across {mor, skip, target, USA, R78, prepaid, 365, 365/360,
exact, balloon+mor, ARM+skip, prepaid+balloon, USA+balloon, prepay series} ×
{hard, solved} payments; payoff × 9 option families at 8 date positions;
USA × fancy totals incl. the usap<0 clamp; ARM fine structure (between-dates,
before-first, AO5/AO6, adjacent ARMs); balance→date × {balloon, ARM}; error
parity (balloon-before-mor, mor-before-first, inadv+ARM, amount-solve+target,
adj-on/after-last, same-date adjs, balloon-before-first, target-too-high, n=1).


### §20 resolution round (2026-07-11, same day) — all items closed

Per the product decision "aim for DOS consistency", 20a-20f are FIXED and
oracle-guarded (`dos_audit_pass1_test.go`; full gated suite green):

- **20a/20e** `PrepaidInterest` and the early-payoff rebate now honor DOS's
  forced `prepaid := true` for in-advance loans (`Test20aInAdvanceAPRForcedPrepaid`,
  `Test20eInAdvanceEarlyPayoffRebate` — APRs and payoff to the oracle).
- **20b** the fancy walk folds the residual into the final scheduled payment of
  a balloon-bearing loan (all balloons consumed, no prepay series) —
  `Test20bUnderfundedBalloonFold`, both USA and plain to the cent. This also
  adjudicated `TestAPIAmortBalloonIncludesRegular_Override`, which had pinned a
  NON-DOS expectation (final payment = the raw balloon, residual left unretired):
  the oracle shows DOS ADJUSTS a non-retiring terminating balloon and notifies
  ("Please note that the amount of your terminating balloon has been ajusted."),
  i.e. final payment = balance + interest. Test corrected.
- **20c** plain USA-rule payoffs run DOS's usap-aware payoff walk (which
  intentionally disagrees with DOS's own displayed balance column) —
  `Test20cUSAPlainPayoffUsapWalk`, three dates to the cent.
- **20d** `payoffRateInForce` is now DOS's forward scan, including the AO6
  implied-rate recompute via solveAdjRate — `Test20dRateInForceForwardScan`.
  (The pass-1 report's two-ARM figure of 88585.3257 did not reproduce; the
  fresh oracle value 96120.6613 is the pinned golden — token-date arithmetic
  in the original probe was the likely culprit. Always re-derive `adj=`/`b=`
  dates: month N lands at loan-month + N.)
- **20f** a prepayment series starting ON the loan date is refused
  (`Test20fPrepayOnLoanDateRefused`).
- **20g** DELIBERATE non-reproduction: probing showed the DOS failure is a
  measure-zero FP coincidence — exactly (rate=0.10 AND n=120 AND mor=12);
  rate ±1e-6, n ±1, mor ±1 all solve in DOS. There is no rule to be consistent
  with; reproducing it would make Go fail on a solvable loan (A11 precedent).
  `Test20gMorSolveIsolatedDOSFailure` pins Go solving it.

Pass-1 clean-pass counter remains 0 (findings were made this pass); the
campaign continues with pass 2.

## 21. Amortization audit pass 2 (2026-07-11) — 9 confirmed divergences ALL FIXED, plus one deliberate error-parity group

Source: the second pass of the "iterate until 3 consecutive clean passes"
campaign (focus rotated to non-monthly frequencies, hard-payment Round2 rows,
first-period stubs × options, degenerate inputs, and a fresh-seed fuzz,
seed 20260711; ~900 oracle-vs-Go case comparisons + ~800 row-level checks).
Every finding was CONFIRMED against the live DOS oracle and fixed the same
day; every golden below was re-derived from the oracle when the regression
file (`dos_audit_pass2_test.go`) was written. Clean-pass counter reset to 0.

### F1 — FIXED: plain USA/exact non-360 loans never folded a large residual

DOS's very-last fold has no "residual < one payment" restriction; a hard
payment below accruing interest still retires the loan in the final row. Go's
`plainFancy` fold required `residual < payment`, so a neg-am loan just stopped.

    amort_oracle 100000 0.15 36 12 payhard=900 b365 usa   → paid 145000.00 (Go was 32400, bal 112600 left)
    amort_oracle 100000 0.15 36 12 payhard=900 b365 exact → paid 148179.40

Fix: fold gate is now `p + int − pmt > 0` only (engine.go).
`TestPass2F1NegAmResidualFold`.

### F2 — FIXED: semimonthly (24/yr) non-360 rows kept the constant 1/24 accrual

DOS routes every non-360 loan through `RepayFancyLoan`, whose `ComputeNext`
accrual is the `DaysCloseEnough`-gated timedif (AMORTOP.pas:625-632 +
INTSUTIL.pas:716-727): actual days on the 1st/16th grid, whole ±half-month
periods on the 15th/month-end grid. Go's simple schedule used the constant
`p*(f-1)` for peryr=24.

    amort_oracle 10000 0.06 24 24 firstdmy=16.1.2024 b365 pay=429.7945 rows
      → row2 int 25.17 (16 actual days; Go was 23.99), interest 314.56 (Go 315.07)
    amort_oracle 10000 0.06 24 24 loandmy=15.1.2024 firstdmy=30.1.2024 b365 pay=429.7945
      → row1 int 25.00 whole half-month (Go prorated 24.59), interest 315.50
    amort_oracle 10000 0.06 24 24 firstdmy=16.1.2024 b365_360 pay=429.7945 → interest 320.01 (Go 315.49)

Fix: the simple-schedule actual-day branch now also engages for
`PerYr == 24 && basis != 360` and uses `periodYearFraction` (the
DaysCloseEnough port). `TestPass2F2SemimonthlyNon360Accrual`.

### F3 — FIXED: plain amount/rate solves on non-360 used the whole-period prorate

DOS's solve-side terminal uses the actual-day global
`prorate := YearsDif(first_repay, repay_from) * peryr` (Amortize.pas:1284-1287)
on the ACTIVE basis; Go's `RepayLoan` used `firstPeriodProrate` (= 1 on clean
calendar boundaries on all bases) — right for the schedule, wrong for the
non-prepaid solve terminal on 365 / 365-360.

    amort_oracle 0 0.11 36 12 b365 noamt pay=3929.2311    → 120000.0012 (Go was 120017.87)
    amort_oracle 120000 0 36 12 b365 norate pay=3929.2311 → 0.1100000067 (Go 0.110103)
    annual n=6 on 365/360: amount off $198, rate off 5.7bp before the fix

Fix: `RepayLoan` now computes DOS's actual-day prorate (falling back to
`firstPeriodProrate` when degenerate). `TestPass2F3PlainSolvesNon360Prorate`.

### F4 — FIXED: exact × moratorium payment solve returned the non-exact payment

DOS refines the moratorium payment via Iterate over the exact-accrual
`RepayFancyLoan` terminal whenever exact is set (Amortize.pas:416 +
AMORTOP.pas:625); Go's segment solve declined to engage for exact.

    amort_oracle 100000 0.10 24 12 b365 exact mor=6 → 5712.4662 (Go was the exact-OFF 5712.6693; fuzz cases up to $60)

Fix: `solveSegmentPayment` engages for `exactDaily`.
`TestPass2F4ExactMoratoriumSolve`.

### F5 — FIXED: exact × in-advance × 360 basis is a DISPLAY split, not a solve split

DOS's Iterate gate is `fancy or (exact and basis<>x360)` (AMORTOP.pas:1438) —
the PAYMENT on 360 stays the plain annuity-due — while the display gate
`fancy or (exact and not R78)` (Amortize.pas:1493) still routes the SCHEDULE
through the shifted exact in-advance shape (settlement row + one-period base
shift). Go treated exact as fully inert at 360 (wrong schedule); a first fix
attempt that forced fancy moved the SOLVE too (payment 3269.81 — also wrong).

    amort_oracle 104844 0.0593783730 36 12 inadv exact
      → payment 3172.2326 (plain annuity-due), first amortizing row 2024-03-01, interest 10421.61 (Go was 9875.16)
    amort_oracle 52287 0.0748745490 12 12 inadv exact prepaid → interest 2452.41

Fix: a display-only dispatch arm routes `exact && inadv` (any basis) through
`generateExactInAdvanceSchedule`; the solve path is untouched.
`TestPass2F5ExactInAdvance360DisplaySplit`.

### F6 — FIXED: prepaid solves missed DOS's `prorate := 1` pin at day-count frequencies

Prepaid unconditionally pins `prorate := 1` (Amortize.pas:1277-1282). A natural
weekly/biweekly/semimonthly period is NOT 1 under raw YearsDif (7·52 = 364 ≠
365.25), so the prepaid payment/amount/rate solves shifted.

    amort_oracle 250000 0.145 26 26 b365 prepaid → 10353.4897 (Go was the non-prepaid 10353.1770)
    amount solve → 250000.0004 (Go 250007.55); rate → 0.1450000029 (Go 0.14506011)
    amort_oracle 10000 0.06 24 24 firstdmy=16.1.2024 b365 prepaid → 429.8121 (Go 429.7945)

Fix: `RepayLoan` pins prorate to 1 whenever the prepaid settlement stub is in
force (loan ≥ one period before first). `TestPass2F6PrepaidProratePinDayCount`.

### F7 — FIXED: R78 hard payment rounds the recurrence ACCUMULATOR

`Round2` is a var-procedure in DOS — with a hard payment the NEXT period
subtracts from the ROUNDED interest (Amortize.pas:1524-1528), so drift never
accumulates. Go rounded only the emitted copy.

    amort_oracle 10000 0.1237 24 12 payhard=471.73 r78 rows
      → row2 101.31, row3 96.90, row4 92.49 (Go was 101.32 / 96.91 / 92.51, drift growing)

Fix: `r78int` itself is rounded each period under a hard payment (and the seed
is rounded). This also adjudicated the pre-existing `TestCrossCheckRule78`
fixture: the refdata harness computes the SOFT-payment (unrounded) chain —
verified by oracle (`payhard=470.73` → 56.17/4.21 vs `pay=470.73` →
56.23/4.33 = harness) — so that test now feeds the payment soft.
`TestPass2F7R78HardAccumulatorRounding`.

### F8 — FIXED: in-advance × moratorium × weekly/biweekly payment solve

Two layers: (a) the day-count frequencies on a non-360 basis invalidate the
analytic annuity seed (rows accrue 14/366 in a leap year, the constant f uses
14/365.25) — DOS drives the actual-day walk; (b) the mid-loan segment must be
solved WITHOUT re-applying the in-advance settlement/shift — `ComputeNext`
accrues ordinary opening-balance interest even when in_advance is set
(AMORTOP.pas:636); the shift is a loan-START shape already behind the boundary.
(A first fix that engaged the segment solve but carried `InAdvance` into the
sub-loan made it WORSE: 2423.3853.)

    amort_oracle 100000 0.10 52 26 b365 inadv mor=3 → payment 2375.0973, interest 11549.56
      (Go seed was 2375.3444; rows at DOS's payment already matched row-for-row)

Fix: `solveSegmentPayment` engages for day-count × non-360 and clears
`InAdvance` in the sub-loan settings. `TestPass2F8InAdvanceMoratoriumDayCount`.

### F9 — FIXED: zero-amount balloon in REPLACE mode is a skipped installment

DOS keys balloon presence on the row STATUS, not on amount ≠ 0: an explicit
$0 balloon REPLACES the regular payment on its date (a skipped installment),
raising the solved payment. Go filtered `Amount != 0` in several presence
checks and treated the loan as plain.

    amort_oracle 10000 0.12 24 12 b12=0 → payment 491.2571, interest 1298.91 (Go was the plain 470.7347 / 1357.33)

Fix: presence-by-status in `hasKnownBalloon`, `hasFancyOptions`, and the
dosport entry balloon scan. ADD mode (+0) is naturally a no-op.
`TestPass2F9ZeroAmountBalloonReplaces`.

### F10 — error-parity group: one FIXED, four documented DELIBERATE

**F10a — FIXED:** `skip=1-12` (every month skipped) with a blank payment: DOS
refuses ("did not converge"); Go silently emitted an all-zero schedule with the
balance growing. Fix: `allMonthsSkipped` refusal in the skip branch.
`TestPass2F10aAllMonthsSkippedRefused`.

**F10b-e — DELIBERATE (DOS-fragility precedent, per §18-A11 / §20g):** these
are numeric fragilities of DOS's solver, not rules; reproducing them would
make Go fail on solvable loans:

- **F10b** rate exactly 0 with a blank payment: DOS emits nothing (division
  blowup in the seed); rate 1e-10 works in both engines. Go solves the
  zero-interest annuity (833.33 on 10000/12) — kept.
- **F10c** n=2 annual/semiannual × in-advance × {target, skip, mor, balloon}:
  DOS dies with "Internal error - last payment not found" (its shifted n−1 walk
  leaves a single row the very-last scan misses); Go computes — kept.
- **F10d** a prepayment series extending past the term at annual frequency:
  DOS "did not converge"; Go solves — kept.
- **F10e** TWO balloons on the same date: DOS INFINITE-LOOPS (hangs both on
  solve and payhard — unrecoverable in the real program). Go computes sanely.
  This must NEVER be reproduced; a hang is not a behavior to port.

(mor-past-term: both engines refuse; DOS's message is targeted, Go's generic —
presentation only.)

### Verified clean in pass 2

Frequencies 1/2/4/12 × {balloon, skip, mor, target, prepaid, prepay-series} ×
3 bases (totals); weekly/biweekly 360→365 auto-switch; weekly/biweekly ×
options incl. 365/360; hard-payment Round2 rows across bases × 9 option
groups (row-exact); odd-day/odd-month/zero-length/3-years-out stubs ×
{targ, skip, mor, usa, prepaid} (69/70 — the 1 is the documented USA
odd-first envelope); term/balloon solves at all frequencies;
payment=interest exactly; negative balloon; amount 0.01; n=2/n=1 parity;
targ>payment parity; fresh-seed fuzz row subsample: 0 fails.

All 9 fixes validated by the full `PERSENSE_REQUIRE_ORACLE=1 go test
./internal/...` suite (green 2026-07-11). Clean-pass counter: 0 — the
campaign continues with pass 3.

## 22. Amortization audit pass 3 (2026-07-12) — 13 confirmed divergence families, ALL FIXED

Source: the third pass of the campaign, run as two halves — forward paths
(~1,030 comparisons: points/APR, payoff pairs, month-end grids, USA pairings,
neg/near-zero rates, extreme terms) and backward solves (~605 comparisons +
~400 row checks: term/balloon solves, date evals, amount/rate × fancy, odd
firsts, fresh-seed fuzz 20260712). Full finding data with oracle commands:
project doc `claude/amort_logic_audit_pass3_2026-07-12.md`. Clean-pass
counter: 0.

### FIXED same day (regressions in `dos_audit_pass3_test.go`)

- **AF1 — payoff AO6 implied rate off by one payment.** A payment dated ON an
  adjustment date is made BEFORE the adjustment applies. `payoffRateInForce`'s
  made/bal scan now uses `<= 0`. Verified:
  `amort_oracle 100000 0.09 120 12 payhard=1300 adj=24::1100 payoff=1.2.2024`
  → 100449.9644 (was 100451.4622); two-AO6 case 65537.8966 exact.
  `TestPass3AF1PayoffAdjOnPaymentDate`.
- **AF2 — 360-basis first-period prorate on clamped/Feb month-end pairs.**
  DOS's simple-route first period is the RAW 30/360 YearsDif
  (Amortize.pas:1286+1516) — `firstPeriodProrate`'s whole-month shortcut now
  applies only off the 360 basis (they agree everywhere except clamped/Feb
  pairs). Also implemented DOS's prepaid-CLEARING rule (Amortize.pas:1252-1259):
  a loan taken strictly after the natural period start clears prepaid outright
  (new block at the top of `Amortize`). Verified: 31.1→29.2 row1 402.78
  (was 416.67), totals 3166.53; Feb28→Mar28 row1 388.89; prepaid variant
  3797.6090 (was 3798.6555). `TestPass3AF2Clamped360FirstPeriod`.
- **AF6 — prepay-series residual terminal fold.** The display very-last fold
  now covers prepayment-series loans (new branch in the fancy walk, with a
  veryLast guard for NN-derived trailing rows). This exposed that
  `SolvePrepaymentAmount` (REPLACE mode) secanted on the FORCED display
  balance — it now evaluates the UNFORCED `fancyTerminal`, which is what DOS's
  Iterate drives (AMORTOP.pas:1433-1437). Verified:
  `50000 0.08 60 12 payhard=900 pre=6:12:12:200` → final row folds 20722.07,
  paid 65560.22; `200000 0.09 480 12 payhard=1600 pre=12:120:12:100` → paid
  4,258,344.89 (was short $3.67M). `TestPass3AF6PrepaySeriesResidualFold`.

### Fixed in the second session (2026-07-12), regressions in `dos_audit_pass3_test.go`

- **AF3 — semimonthly first date on day 31.** DOS's AddPeriod(24) round trip
  re-grids the schedule to the 1st/16th (31.1 → 1.2; INTSUTIL.pas:1208-1237)
  while the SOLVE keeps the original date's prorate. Diagnosis complete: a
  global first-date re-grid reproduced every ROW to the cent
  (`50000 0.10 26 24 loandmy=15.1.2024 firstdmy=31.1.2024 b365` → rows 2/1,
  2/16, …, row1 232.24) but moved the solved payment to 2034.09 — DOS's
  2033.5386 comes from the ORIGINAL 31.1 prorate (16 days: closed form gives
  2033.5578; the re-grid's 17 days gives 2034.11). The fix needs a walk/solve
  split: re-grid the schedule dates only, keep `h^.firstdate` for the solve
  prorate. (The attempted global re-grid was reverted.)
- **AF4 — USA leaks into the blank-payment solve on non-360.** DOS's plain-loan
  Iterate never sees usap; solve payment is identical with/without `usa`
  (`100000 0.09 120 12 usa inadv b365` → 1260.9130, Go 1273.3378). Also the
  fuzz "USA × day-count" cluster (19 cases, $0.006–$3.60) — same root; the
  older "deliberate routing" note is superseded and the envelope should close.
- **AF5 — in-advance overfunded hard payment: no early-retirement truncation**
  (`100000 0.09 120 12 payhard=1300 inadv` → DOS 115 rows bal 0; Go 120 rows,
  bal −6157.26).
- **P3-F1 — term-solve reported n/last-date**: usa/exact non-360 must use DOS's
  closed-form log branch (AMORTOP.pas:1383-1397), not the fancy walk. NOTE:
  DOS itself can report n=25 while rendering 24 rows — match the REPORTED
  field.
- **P3-F2 — term-solve prepaid non-360** misses the `prorate := 1` pin in the
  closed form (extends pass-2 F6 to `solveNPeriodsFromPayment`).
- **P3-F3 — missing refusal gates**: unknown balloon / unknown prepayment /
  term solves must refuse when a coexisting adjustment is not fully specified
  (SufficientDataOnScreen, Amortize.pas:885-894), and even a fully-specified
  ARM aborts DOS's term solve (AMORTOP.pas:1345-1346). Go currently "solves"
  degenerate values (e.g. balloon = 50000.0000 — the half-amount seed).
- **P3-F4 — exact × non-360 × ARM amount/rate solves badly broken** (amount
  26824.60 vs DOS 100000.00; rate −0.98 vs 0.08). Suspected: the walk
  re-solves the post-adjustment payment per trial ⇒ flat residual; DOS fixes
  adjustment payments before Iterate.
- **P3-F5 — non-exact non-360 × ARM rate solve** drifts 2.5–5.1e-5 (tolerance
  5e-6); 360 is clean.
- **P3-F6 — prepaid × day-count freqs × stub first**: the loan→first span is
  charged AGAIN in row 1 (and capitalized) on top of the settlement row —
  totals ~2× DOS (`50000 0.11 52 26 b365 prepaid firstdmy=1.1.2027` → Go
  totInt 42260.55 vs DOS 22075.72). The prepaid repay_from shift is missing
  from the day-count accrual branch.
- **P3-F7 — 1-day first stub × weekly/biweekly × 365/360**: payment solve off
  ~21¢ (totals identical — the fold absorbs it).

### Error-parity notes (pass 3)

In-advance payoff exactly ON a payment date: DOS reads a stale `nextpayment`
global and prints garbage (1217525.0000) — a DOS fragility; do NOT reproduce.
One flaky oracle run (`solvedrate 0.0`) and one harness artifact were discarded
after re-runs.

### Pass-3 resolution round (same campaign, second session)

- **AF3** — the walk/solve split: `generateSimpleSchedule` re-grids its
  STARTING date via the AddPeriod(24) round trip (31.1 → 1.2;
  INTSUTIL.pas:1208-1237) while the solve keeps the original date's prorate
  (payment 2033.5386 exact; the earlier global re-grid moved it to 2034.09 and
  was reverted). Fancy semimonthly day-31 was already clean via the DOS-port
  walk. `TestPass3AF3SemimonthlyDay31Regrid`.
- **AF4** — `usaFancyDisplay` now carries the split the comment always
  described: USA × (exact | non-360) routes only the DISPLAY to the usap-aware
  fancy walk; the solve stays on RepayLoan. Solved payments now equal the
  plain values on every probe (1260.9130 inadv, 709.6785 biweekly, the
  21142.6074 fuzz cell). This CLOSES the old "USA odd-first envelope" note in
  `exact_backward_roundtrip_test.go`. `TestPass3AF4USASolveIsPlain`.
- **AF5** — the simple in-advance walk gets the same early-retirement fold as
  arrears (115 rows, final 342.74/int 0, paid 149292.74).
  `TestPass3AF5InAdvanceOverfundedTruncation`.
- **P3-F1/F2** — the A6 term-solve dispatch keys on the CALLER's
  Advanced-Options toggle (captured as `uiFancy` before the exactDaily
  forcing), so usa/exact non-360 take DOS's closed form; the closed form pins
  prorate := 1 under prepaid. All six oracle cells exact (incl. the DOS
  report-25-render-24 case). `TestPass3F1F2TermSolves`.
- **P3-F3** — refusal gates: unknown balloon / unknown prepayment refuse when
  any adjustment row is incomplete (`adjRowsNotFullySpecified`, per
  SufficientDataOnScreen); a term solve with ANY adjustment errors (DOS's
  AMORTOP.pas:1345 ABORT). Fully-specified control still solves 20696.52.
  `TestPass3F3RefusalGates`.
- **P3-F4/F5** — THE STRUCTURAL FIND OF THE PASS: DOS's Re_Amortize gate
  `((next_adj <= adjnum) or entire)` (AMORTOP.pas:1215) means Iterate's walk
  (entire=til_adj=FALSE, adjnum=0) NEVER re-amortizes at an adjustment — the
  payment/amount/rate solves ignore ARMs entirely (oracle: solved values
  identical with and without the adjustment; the exact ARM payment = the plain
  exact payment 1213.0959). `fancyTerminal` now strips adjustments. The
  balloon-amount and unknown-prepayment solves are the OPPOSITE — their walks
  re-amortize (balloon 20696.52 with ARM vs 21099.52 without; prepay
  2198.1283 vs 2260.1872) — and keep their adjustments. The display's nested
  segment Iterate now also fires for exact×non-360 (AMORTOP.pas:1571) and
  stores the solved payment on the row (amtok, :1579-81). Amount solve
  restored from 26824.60 → 99999.9973 exact; rate from −0.98 → 0.0799999938;
  8-digit agreement on the b365_360 cross-check. `TestPass3F4F5ARMSolves`.
- **P3-F6** — prepaid day-count stub: row 1 anchors on the SHIFTED start
  (repay_from = first − 1 period) in the actual-day accrual branches; the
  settlement span is no longer double-charged (totInt 22075.72 exact vs the
  old 42260.55). `TestPass3F6PrepaidDayCountStub`.
- **P3-F7** — pinned (1070.2449 / 1015.1715 exact); resolved by the pass-3
  prorate work. `TestPass3F7OneDayStub365360`.

Full `PERSENSE_REQUIRE_ORACLE=1 go test ./internal/...` green after all 13
fixes. Clean-pass counter: 0 — pass 4 (including the user-requested
INVENTED-LOGIC sweep: Go logic with no DOS counterpart) is next.

## 23. Amortization audit pass 4 (2026-07-13) — invented-logic + fresh differential sweep

Source: pass 4, run in two parts per the user's request — an INVENTED-LOGIC
sweep (Go logic with no DOS counterpart, which introduces divergences) plus a
fresh-seed differential fuzz. ~1,000 oracle-vs-Go comparisons. Two confirmed
divergence classes FIXED; one bounded numerical frontier documented.

### P4-1 — FIXED: R78 payment solve skipped the odd-first refine (invented special-case)

`needPaymentRefine` carried an INVENTED `if s.R78 { return s.InAdvance }`
branch — its own comment admitted the arrears R78 odd-first case was "untested
against the oracle." DOS's `EstimateAndRefinePayment` (Amortize.pas:377-430) is
entirely R78-AGNOSTIC: it never inspects the R78 flag (R78 only changes the
interest SPLIT of the resulting schedule), so an R78 loan's payment is
identical to the plain loan's. The special-case skipped the odd-first refine
for arrears R78, leaving the un-refined estimate. Verified vs the real DOS
engine:

    amort_oracle 10000 0.1213 36 24 r78 → payment 302.9827 (= the plain
    semimonthly payment; the un-refined estimate was 304.5140. First payment
    ON the loan date — a zero-length first period.)

Fix: `needPaymentRefine` now returns `s.InAdvance || oddFirstPeriod(...)` with
no R78 branch. `TestPass4R78PaymentIsPlain`. (The old A4 in-advance R78 case is
preserved — in-advance still refines.)

### P4-2 — FIXED: R78 did not suppress the exact display (missing `not R78` gate)

DOS's schedule DISPLAY gate is `fancy or (exact and not R78) or non-360`
(Amortize.pas:1493): R78 SUPPRESSES exact. The port's exact × in-advance
display arm (added in pass-2 F5) quoted that gate in its comment but the CODE
omitted the `!R78` check, so an R78 × exact × in-advance loan on the 360 basis
wrongly rendered the exact settlement-shifted shape instead of the plain
in-advance R78 schedule. Verified vs the real DOS engine:

    amort_oracle 100000 0.1173 48 24 r78 exact inadv → interest 11944.77
    (= the r78+inadv value = n·d − principal; the exact arm added a spurious
    settlement, rendering 12477.59)

Fix: the arm is now `settings.Exact && settings.InAdvance && !settings.R78`.
`TestPass4R78SuppressesExactDisplay`. (Each PAIR — r78+exact, r78+inadv,
exact+inadv — was already correct; only the triple broke.)

### P4-F — FIXED: prepaid × moratorium × day-count frequency (DOS early-exit)

At weekly/biweekly/semimonthly frequencies, a PREPAID moratorium loan's solved
payment differs from the non-prepaid moratorium payment by ~$0.13–0.19 (and
~$1–3 total interest), even though the two schedules are BYTE-IDENTICAL
(verified via `dumpraw`) apart from that payment. Monthly/quarterly are clean
(closed-form and Iterate coincide there).

    amort_oracle 100000 0.10 104 52 prepaid mor=3 → payment 1186.6343
    amort_oracle 100000 0.10 104 52 mor=3         → payment 1186.5113 (Go gives
      this for BOTH — it does not distinguish the prepaid case)

Mechanism traced to DOS's Iterate (AMORTOP.pas:1415): the acceptance clause
stops the Newton once the terminal balance is within `halfpenny` (0.005) OR
`acc_limit·amount` (2e-8·100000 = $0.002), so the *accepted* payment depends on
the SEED trajectory. DOS's prepaid and non-prepaid seeds differ slightly and
land at different roots within that ~$0.002 tolerance band — a DOS numerical
fragility, not a clean rule (the schedule structure, `paidthru`, and `prorate`
are all identical between the two). Reproducing DOS's exact accepted root would
require matching its internal Newton trajectory bug-for-bug; per the
faithful-port rule (no logic beyond what DOS specifies) this is recorded as a
bounded frontier rather than approximated. Magnitude ≤ $0.19 payment / ≤ ~$3
interest, confined to prepaid + moratorium + {weekly, biweekly, semimonthly}.

DEEPER INVESTIGATION (2026-07-13, after the in-advance payoff fix): the
mechanism was pursued through the full DOS source — EstimateAndRefinePayment's
early-exit (Amortize.pas:402-407), the moratorium `nrepay`/`repay_from`
(Amortize.pas:1261-1302), RepayFancyLoan's `paidthru` (AMORTOP.pas:1151-1156),
ComputeNext's `timedif`/DaysCloseEnough (AMORTOP.pas:625-632, INTSUTIL.pas:
716-727), and the Iterate terminal (AMORTOP.pas:1415). Findings: DOS does NOT
early-exit for a moratorium loan (the closed form at the amortizing `nrepay`
matches neither DOS value at monthly OR weekly); `paidthru`, `prorate`
(the moratorium block pins it to 1 for both), and `nrepay` are all IDENTICAL
between the prepaid and non-prepaid cases; and Go's `fancyTerminal` (a faithful
port of RepayFancyLoan's unforced terminal) returns byte-identical residuals
for prepaid vs non-prepaid at every trial payment. Since both DOS's Iterate and
Go's port drive the SAME terminal-balance criterion, and every structural input
to that criterion is identical between the two cases, the ≤$0.19 shift is a
genuine artifact of DOS's Iterate internals at day-count frequencies that the
faithful port does not — and arguably should not — reproduce. Matching it would
require an empirical offset with no DOS rule behind it (forbidden by the
faithful-port constraint), so it remains a documented bounded frontier. This is
categorically unlike the in-advance payoff (P4-F2), which had a clear
structural root — DOS's distinct balance_calc walk — and was fixed by porting
that walk.

FIXED 2026-07-13: on returning to it after the payoff fix, the mechanism WAS
found — the earlier hand-calc used the 360 basis, but weekly/biweekly force the
365 basis (Amortize.pas:300-305). On the 365 basis the closed-form annuity over
the amortizing count `nrepay` equals DOS's prepaid value EXACTLY (1186.6343 for
the reference case). DOS's EstimateAndRefinePayment EARLY-EXITS to that closed
form for a prepaid loan (Amortize.pas:402-407) — moratorium is not excluded —
while the non-prepaid case Iterates, and at a day-count frequency the two
differ. The port's `estimateAndRefinePayment` (dosport_entry.go) previously
seeded over the full NPeriods and ALWAYS Iterated; it now (a) computes nrepay =
`NumberOfInstallments(first_repay, lastdate, on_or_before)` for a moratorium
(Amortize.pas:1302, first_repay snapped on_or_after), seeds over it, and (b)
takes the early-exit (returns the closed form without Iterate) when prepaid and
no balloon/prepayment/target/skip. Verified to the cent across 500 prepaid/mor
fuzz cases; mor-alone, monthly prepaid+mor, and plain prepaid all unchanged.
`TestPass4PrepaidMoratoriumEarlyExit`. So NOT a bounded frontier after all — a
genuine DOS-faithful fix.

### P4-F2 — FIXED: in-advance payoff-walk balance selection

The pre-existing "OPEN in-advance payoff-walk frontier" (referenced under §20e)
was quantified in pass 4: EVERY in-advance payoff (mid-period or on a payment
date) is systematically LOW by ~0.2–0.7% of the balance (~$90–1,560 across the
fuzz sample). Arrears payoffs are exact (verified: `100000 0.0632 48 12
payhard=684.67 payoff=15.6.2025` → DOS = Go = 97436.6405).

    amort_oracle 100000 0.0632 48 12 payhard=684.67 payoff=15.6.2025 inadv
      → DOS 97096.1096 | Go 96909.2958 (Δ $186.81)

Root: DOS computes the payoff from a dedicated `balance_calc` RepayFancyLoan
walk stopped at `very_last = asOf` (Amortize.pas:1108-1127), reading
`payment.principal` and `nextpayment.date` from that IN-ADVANCE base-date-
shifted walk. Go instead selects the balance and next-payment date from the
DISPLAY schedule rows (payoff.go). The display balances match DOS's to the
cent (e.g. 6/1/25 → 97182.27 on both sides), and Go's formula `balance·(1 −
rif·YearsDif(nextPmt, asOf))` computes 96909.30 correctly FROM those rows — so
the divergence is that DOS's shifted walk resolves a DIFFERENT (balance,
nextPmt) pair than the display rows carry. Resolving this DOS-faithfully
requires running the in-advance base-date-shifted balance walk in the payoff
path rather than reading display rows. Arrears, R78, and USA payoffs are
unaffected.

FIXED 2026-07-13: `inAdvancePayoffBalance` (payoff.go) reconstructs DOS's
balance_calc `RepayFancyLoan` walk directly — a faithful port of
`Paymenttype.ComputeNext` (AMORTOP.pas:596-664) with the in-advance base-date
init (base_date := firstdate, AMORTOP.pas:1159-1177): it steps period-by-period
accruing plain opening-balance interest (whole-period `timedif` via
DaysCloseEnough, or YearsDif; the peryr=24 half-month adjustment; hard-payment
Round2; moratorium/target floors), stops when `nextpayment.date >= asOf`, and
applies DOS's rebate `payment.principal·(1 − rif·YearsDif(nextpayment.date,
asOf))`. Verified to the cent across 400 fuzz payoffs (all dates × flags) and
the on-payment-date variants; arrears unchanged. `TestPass4InAdvancePayoffWalk`.

## 24. Pass-4 confirmation fuzz (2026-07-13) — newly-surfaced in-advance × fancy-combo divergences

After fixing both pass-4 frontiers (in-advance payoff, prepaid×moratorium), a
broad fresh-seed confirmation fuzz (~600 cases, seed 1010101, all option
combos × payoff/APR tails) surfaced a NEW cluster — all confined to IN-ADVANCE
combined with a second fancy option. These are pre-existing (untouched by the
pass-4 fixes) and OPEN for the next round:

### P4-N0 — FIXED: R78 × prepaid × moratorium × day-count (piecewise early-exit)

R78 loans are excluded from AmortizeDOS (dosPortCanHandle), so they take the
PIECEWISE engine, where the moratorium payment is set in the mid-schedule
recompute — which was Iterating the day-count segment (5563.2471) instead of
DOS's early-exit closed form. The recompute now honours the same prepaid
early-exit (Amortize.pas:402-407): `amort_oracle 250000 0.0711 60 26 r78
prepaid mor=6` → 5563.4990 (= the no-R78 value; R78 does not change the
payment). `TestPass4PrepaidMoratoriumEarlyExit` (R78 case).

### P4-N1 — FIXED: in-advance × skip × moratorium (moratorium boundary on a skipped month)

`amort_oracle 100000 0.10 36 12 inadv skip=4-6 mor=3` → DOS interest 18151.23 |
Go 16695.51 (payment matches at 4659.3825). The `skip+mor` pair WITHOUT
in-advance is exact (both 18151.23); each of `inadv+skip`, `inadv+mor`,
`inadv+targ`, `inadv+skip+targ` is exact. Only the TRIPLE breaks: under the
in-advance base-date shift, Go's moratorium ends after row 1 (row 2 already
amortizes) while DOS keeps the interest-only rows through the shifted
first_repay (Jan-Mar), then negative-amortizes the skipped Apr-Jun. Root is
the interaction of the in-advance one-period base-date shift with the
moratorium-boundary detection when skip months are also present
(`moratoriumActive` keys on the shifted first date — Revision 13).

FIXED 2026-07-13: the in-advance shift lands the FirstRepay boundary (4/1) on a
skipped month (April). The piecewise moratorium recompute set `pmt = d` at the
boundary, overriding the skip. DOS's ComputeNext zeroes payamt for a skipped
month first and the past-moratorium arm leaves that zero (AMORTOP.pas:596,
648-653), so the recompute now re-applies the skip. `100000 0.10 36 12 inadv
skip=4-6 mor=3` → 18151.23 exact (was 16695.51). `TestPass4InAdvanceSkipMoratorium`.
NOTE the 4-option combo inadv+target+skip+mor is still OPEN (P4-N1b): DOS's
value equals the non-in-advance value (in-advance does not change the mor
payment), but Go's recompute `remaining--` in-advance adjustment × skip × target
gives a different payment (`50000 0.164 24 12 inadv targ=0.01 skip=4-6 mor=3` →
DOS 3500.3264 | Go 3706.5934). Each of inadv+targ+mor, inadv+targ+skip,
inadv+skip+mor is now clean; only the quad breaks.

### P4-N2 — FIXED: fancy in-advance payoff (skip months)

The pass-4 in-advance payoff walk (`inAdvancePayoffBalance`) modelled moratorium
and target but NOT skip months, so a fancy in-advance loan with skip fell back to
the display-row rebate — ~10% off (`250000 0.1616 48 24 skip=4-6 b365 inadv
payhard=2188.33 payoff=15.6.2025` → DOS 257200.5866 | Go was 230541.0151).

FIXED 2026-07-13: the walk now zeroes payamt for a skipped month FIRST (ComputeNext
AMORTOP.pas:599 — `if (date.m in skipmonthset) then payamt:=0 else payamt := d`),
before the moratorium/target arm (the moratorium overwrites the skip-zero with
interest-only; the target floor overrides skip via `0−interest < target`). The
caller gates the walk on NO balloon/prepayment/adjustment (dated extras the closed
walk still does not model — those keep the display-row fallback). `250000 0.1616 48
24 skip=4-6 b365 inadv payhard=2188.33 payoff=15.6.2025` → 257200.5866 exact.
Confirmed clean across a 252-case in-advance-payoff fuzz sweep (skip / mor / target
× basis × pmts/yr × 3 payoff dates) and the full gated oracle suite.
`TestPass4FancyInAdvancePayoffSkip`. Remaining unmodelled: balloon / prepayment /
adjustment on an in-advance payoff (dated extras) — rare, still on the display-row
fallback.

### P4-N3 — CLASSIFIED (deliberate divergence): DOS skip-loan zero-output singularity

`amort_oracle 250000 0.0983 104 52 skip=4-6 payhard=614.38 payoff=15.6.2025` →
DOS payoff 0.0000 | Go 253892.23. ROOT CAUSE (traced 2026-07-13): the payoff of 0
is a downstream symptom of a DOS **engine defect** — for a FANCY skip-month loan,
DOS's `RepayFancyLoan`/`MakeTable` silently produces ZERO output rows (process
exits 0, no error fired, `dumpraw` → `lines 0`) at an isolated, resonant set of
loan rates. The oracle reports `interest -1.00` (its sentinel for "no
`Total payments:` line") and the payoff query returns w^.amount = 0 because the
schedule was never built.

Evidence it is a DOS numerical artifact, not financial logic:
- It fires on skip loans at ~58 discrete rates in [0.05, 0.20] (~0.4% of the rate
  space), e.g. 0.0515, 0.0543, 0.0571, …, 0.0983, …, 0.1995 — a periodic
  resonance, NOT a "loan doesn't retire" condition.
- The failing point is razor-thin: `0.0982` and `0.0984` build full 26-line
  schedules; only `0.0983` collapses to `lines 0`.
- It is independent of amount (50k/100k/150k/250k all collapse at 0.0983) and of
  whether the payment is SOLVED (blank → `payment 0.0000 lines 0`) or GIVEN
  (`payhard=3000` → `payment 3000.0000 lines 0`), so it is a MakeTable/RepayFancyLoan
  walk failure, not a payment-solve non-convergence.
- It requires skip: `0.0983` with NO skip builds fine (5279.80); `0.0983 mor=3`
  (no skip) builds fine (5663.80).

The Go port produces the correct, complete schedule at every one of these rates
(the loans are perfectly valid — the neighbouring rates retire cleanly). Per the
project's established policy of preferring the financially-correct result over
reproducing a DOS engine bug (cf. the balloon-on-first in-advance deliberate
divergence, §7 / dos_known_frontier), the port does NOT reproduce DOS's
zero-output singularity. `TestPass4SkipRateSingularityIsClean` guards that Go
builds a valid retiring schedule at 0.0983 (and neighbours) where DOS emits
nothing.

### P4-N4 — CLASSIFIED (pathological): annual in-advance, ≥95-year term

`amort_oracle 10000 0.1715 120 1 b365 inadv` → DOS int 205562.63 | Go 197263.94
(payment matches at 1715.00). ROOT CAUSE (traced 2026-07-13): at very long terms
the annuity payment converges to the interest-only amount r·P = 1715.00 and
becomes numerically INDISTINGUISHABLE from it at cent precision. In that regime
DOS's Iterate settles on exactly 1715.00 (pure interest-only) and folds the entire
residual principal into a final balloon row — the balance stays at ~10000 for the
whole term and interest accrues 1715/yr (~120·1715). Go instead solves a payment a
hair above r·P that amortizes SMOOTHLY over 119 rows (balance glides to 0), so its
front-loaded interest totals less.

The boundary is sharp and pathological: every term ≤ 90 years is CLEAN
(10/30/40/60/80/90-yr in-advance annual all match to the cent), and the split only
appears at ≥ 95-year terms where the annuity payment and r·P collide at two
decimals. No real financial product is a 95–120-year annual loan at 17.15%.
Classified as pathological (a degenerate payment≈r·P boundary), NOT a general
in-advance-annual gap. Forcing DOS's interest-only-plus-final-fold shape here would
risk the clean realistic-term cases, so the port keeps its smoothly-amortizing
schedule. `TestPass4LongTermInAdvanceAnnualBoundary` guards that ≤90-yr terms stay
clean.

All four are IN-ADVANCE-related; the non-in-advance option cube remains clean
across the confirmation fuzz. Clean-round counter: 0 (this fuzz found
divergences).

### P4-N5 — VERIFIED CLEAN (2026-07-13): R78 × skip × moratorium — total-interest aggregation

`amort_oracle 50000 0.198 24 12 skip=4-6 r78 mor=3` → DOS total interest
13024.81 | Go now 13024.81 (was 11477.17). Re-checked against the current build:
payment 3835.9256, interest 13024.81, paid 63024.81 — all match to the cent. The
R78 total-aggregation-over-skip-deferred-rows path now equals its own row sum.
Retained here as a regression note; no open work.

### P4-N1b — FIXED: in-advance × target × skip × moratorium (quad) — segment solve dropped target

`amort_oracle 50000 0.164 24 12 inadv targ=0.01 skip=4-6 mor=3` → DOS payment
3500.3264 | Go was 3706.5934. DOS's value equals the NON-in-advance quad
(in-advance only prepends an interest-only settlement row and does not change the
amortizing payment). The root was NOT an off-by-one in the recompute count: it
was that the in-advance moratorium SEGMENT SOLVE (`solveSegmentPayment`)
deliberately omitted the target. With a target present, DOS's Iterate walks the
full fancy schedule and the target floor (AMORTOP.pas:643 — target overrides
skip) converts the skipped months from negative-amortizing rows (pay 0, balance
grows) into tiny-principal rows (pay interest + the minimum reduction), which
LOWERS the retiring payment. The target-omitting segment solved the skip rows as
pure negative-am and returned the no-target inadv+skip+mor value (3706.5934).

FIXED 2026-07-13: `solveSegmentPayment` now threads the target into the sub-loan
WHEN skip is also present (the skip-free moratorium+target still returns at the
plain annuity via the early gate — target never binds the base solve there, so
the warned mor=74+targ=61 case is untouched). `50000 0.164 24 12 inadv targ=0.01
skip=4-6 mor=3` → 3500.3264 exact (was 3706.5934). Confirmed clean across a
270-case inadv×skip×target×mor fuzz sweep and the full gated oracle suite.
`TestPass4InAdvanceTargetSkipMoratoriumQuad`.

### P4-N6 — FIXED (2026-07-13): payment-only adjustment whose implied-rate solve does not converge

`amort_oracle 100000 0.08 36 12 adj=12::0.09` (change the payment to $0.09 at
month 12) → DOS emits `ERR Computation of payment amount or interest rate did not
converge.` | Go WAS building a ballooned schedule (payment 3133.64, interest
18868.66). Now Go errors, matching DOS's control flow.

ROOT CAUSE (correcting the earlier "solver-robustness" classification — the real
mechanism is more specific): a payment-only adjustment (`adj=…::AMOUNT` with the
rate blank) is NOT "apply this payment". DOS's dispatch (Amortize.pas:1408-1419)
routes it to `EstimateAndRefineAdjRate` — it SOLVES the implied RATE at which the
new payment amortizes the balance over the remaining term. When that Iterate can't
drive the terminal balance within half a penny it returns false and DOS does
`if (not EstimateAndRefineAdjRate(i)) then exit` (Amortize.pas:1417) →
`did not converge` (AMORTOP.pas:1489). A $0.09 payment has no amortizing rate in
DOS's |rate|<2 band, so DOS refuses.

The port ALREADY had this implied-rate solve (`solveAdjRate`, backward.go:1317,
with DOS's own within-half-a-penny acceptance test) and matches DOS to the cent on
every payment that converges. The bug was purely a MISSING error branch at
`engine.go` (the AO6 block): when `solveAdjRate` returned `ok=false`, the port
dropped the failure on the floor — no `else` — and silently continued the walk at
the UN-adjusted rate, ballooning the residual into a schedule DOS never makes.

FIX: propagate the solve failure exactly as DOS's dispatch does — set an error and
abort the table (the `if !ok { result.Err = …; return result }` branch). This is
DOS's own control flow, not a special case or a hard-coded boundary. Because Go's
secant is at least as strong as DOS's 20-iteration Newton, `{Go solve fails}` ⊆
`{DOS solve fails}`, so the change can only move toward DOS — it never makes Go
refuse a loan DOS would schedule. Verified: full gated oracle suite green; a
payment-only-adjustment fuzz shows every case is now either both-error or a matching
schedule. `TestPass4PaymentOnlyAdjustmentNonConvergence`.

RESIDUAL (documented, not force-matched): in a narrow band (on the $100k/36mo/8%
loan, payment-changes in ~[$200,$450]) Go's secant DOES converge to a valid but
extreme NEGATIVE implied rate, where DOS's weaker 20-iteration Newton gives up and
errors. DOS itself accepts a negative-rate schedule one step away (at $500), so its
$450-vs-$500 cutoff is an iteration-budget artifact, not a principled bound —
reproducing it would mean deliberately weakening Go's solver to give up on problems
it can actually solve. Per the standing policy (don't reproduce a DOS solver
artifact that degrades correct output), the port keeps the valid solution there.

### P4-N7 — FIXED (payment, 2026-07-14 pass 5): payment-only adjustment × day-count initial-payment solve

`amort_oracle 100000 0.06 72 24 adj=24::2083` → DOS payment 1515.5786 / interest
22172.35 | Go WAS 1519.3676 / 22358.01, **now 1515.5786 / 22172.35 (to the cent)**.

ROOT CAUSE (corrected — the earlier hypothesis had it backwards): the divergence
was NOT that Go's solve missed day-count interest, but that Go's odd-first
initial-payment REFINE was gated OUT whenever any adjustment was present. The
non-exact odd-first/in-advance refine arm (engine.go, the `dosIteratePayment`
call) required `len(input.Adjustments) == 0`, so a payment-only (implied-rate)
adjustment skipped the refine and left the *un-refined closed-form seed*
(1519.3676) — while the base loan with no adjustment correctly refined to
1515.5786. DOS's `EstimateAndRefinePayment` refines EVERY odd-first/in-advance
loan, and its Iterate walk STRIPS adjustments (`Re_Amortize` gate,
AMORTOP.pas:1215) — so the refined payment is the plain refined payment. The
exact-daily refine arm already admitted adjustments for exactly this reason; the
fix drops the `Adjustments == 0` gate on the non-exact arm to match. `fancyTerminal`
already strips adjustments, so `dosIteratePayment` solves the correct plain
refined payment. A day-count first period (first payment ON the loan date, a
zero-length first period) is always "odd", so the refine now fires.

Verified vs the real DOS engine (`TestPass5PaymentOnlyAdjustmentDayCount`):
semimonthly payment AND interest to the cent (`adj=24::2083` and `adj=12::1800`);
the rate-change control unchanged; biweekly PAYMENT now DOS-faithful (1401.8410,
was 1398.6254). Full gated amort suite green.

RESIDUAL — now FIXED (2026-07-14 pass 6): biweekly/weekly (365-basis, actual-day)
payment-only adjustments HAD a small INTEREST residual (~$19 / 0.08% on the
N7-biweekly case) in the POST-adjustment implied-rate SEGMENT — the payment was
exact but the segment over-amortized (a $2019 final payment instead of $2000).

Root cause: DOS's EstimateAndRefineAdjRate (Amortize.pas:347-368) solves the
implied rate by calling RepayFancyLoan — the ACTUAL-DAY schedule walk — and
Iterating its terminal balance to zero. The port's `solveAdjRate` used a
UNIFORM-period recurrence (`balanceAfterN`, constant GrowthPerPeriod). On the 360
basis uniform == actual (semimonthly matched); on the 365 basis a biweekly period
is exactly 14 days vs the uniform 365.25/26 = 14.05, so the implied rate drifted.

Fix: `solveSegmentRate` (fancybisect.go) — the AO6 analog of `solveSegmentPayment`
— solves the implied rate over the REAL segment schedule (the sub-loan
[adj → last] with the new payment, driven through `dosIterate`, the faithful port
of DOS's Iterate, over the actual-day fancy terminal), exactly as
EstimateAndRefineAdjRate does. Engaged only on exact / day-count-non-360 (where
uniform != actual); the 360 basis keeps the identical, cheaper uniform solve.
Verified vs the real DOS engine to the cent across semimonthly/biweekly/weekly and
the rate-change controls (`TestPass6PaymentOnlyAdjustmentSegmentInterest`):
`100000 0.06 78 26 adj=24::2000` → interest 24895.73 (was 24914.75). Full gated
amort suite green.

Historical note (original framing): Unrelated to the P4-N6 fix (these cases
solve successfully, `ok=true`). Small (~0.25% of payment), narrow (day-count ×
AO6). Next candidate to investigate.

---

## Amortization convergence status (2026-07-13, pass-4 close-out)

After the pass-4 fixes (P4-N1b target-in-segment-solve, P4-N2 skip-in-payoff-walk)
and the P4-N5 verify, the amortization engine matches the DOS oracle **to the cent
on every financially-valid loan** across the option cube — basis × prepaid ×
in-advance × exact × R78 × moratorium × skip × target × balloon × adjustment ×
prepayment × pmts/yr, plus payoffs and backward solves. Three fresh confirmation
fuzz rounds (schedule payment+interest and payoffs) surfaced no wrong-number
divergence on a valid loan.

The ONLY remaining DOS-vs-port differences are a single family of **solver-artifact
rejections on pathological inputs**, all documented above and none affecting a
realistic loan:
- **P4-N3** — DOS's `RepayFancyLoan` silently emits zero rows for skip loans at
  ~58 isolated resonant rates (~0.4% of [0.05,0.20]); Go builds the correct schedule.
- **P4-N4** — ≥95-year annual in-advance where the payment ≈ r·P at cent precision;
  DOS keeps interest-only + a final-fold balloon, Go amortizes smoothly. All ≤90-yr
  terms clean.
- **P4-N6** — payment-change adjustments to a near-zero payment; DOS's Newton
  rejects as non-convergent, Go balloons. Realistic payment changes clean.

In each, DOS produces an error / no output (not a competing number), and the port
produces the financially-correct result — consistent with the pre-existing
deliberate-divergence policy (cf. balloon-on-first in-advance, §7).

## 25. Present Value "Exact method for periodic payments" setting ignored — FIXED (2026-07-12)

**Status:** RESOLVED. The PV screen now honours the "Exact method for periodic
payments" Computational Setting; the periodic annuity matches the DOS oracle to
the cent. PV twin of the Amortization "Exact method" gap (§8, fixed 2026-06-19).

### Symptom

A client comparing a PV with **Exact = YES, basis 365** saw the periodic value
read **$198,993.50** where the legacy app showed **$198,980.58** ($12.92 high).
Reproduced (real DOS engine): as-of 04/25/2006, Loan 5.0000% (true 4.9896%),
lump $5,000,000 on 10/01/2027, periodic $3,500/mo 05/15/2030→12/15/2060, COLA 0,
basis 365, Exact YES. DOS periodic 198,980.58; the lump matched all along.

### Root cause

`HandlePVCalc` hardcoded `Settings.Exact = false` and `getPVInput` never forwarded
the shared `set-exact` toggle, so selecting "Exact = YES" silently did nothing on
the PV screen — the periodic annuity used the closed-form nominal-period formula
instead of DOS's period-by-period actual-day summation. The exact path already
existed in `PeriodicSummation` and reproduces DOS to the cent; it was just never
reachable from the API.

### Resolution

`PVRequest` gained an `exact` field; `HandlePVCalc` threads it into
`PVSettings.Exact`; `getPVInput` forwards `body.exact` from `set-exact`. Verified
end-to-end: periodic $198,980.58, total $1,914,872.15 (= DOS). Guard:
`internal/api/pv_exact_periodic_test.go`.

## 26. PV multi-row backward solve picked the wrong unknown — FIXED (2026-07-12)

**Status:** RESOLVED. Value-bearing PV rows are classified DOS-faithfully, so a
multi-row backward screen solves the genuine unknown and reconciles to the target
Present Value. From the 2026-07-12 PV audit (finding B1).

### Symptom

With a target Sum Value and two upper-block rows — one carrying a Value (e.g. a
lump Date+Value, Amount blank) and one genuine single-field unknown (a Date-only
lump) — the port solved the value-bearing row and left the real unknown at 0, so
the total did not reach the target. Reproduced: target 100, Row A {Date, Value
40}, Row B {Date}: port returned Row A 42.05 and Row B amount 0 (total 40),
instead of solving Row B to Value 60 (total 100).

### Root cause

DOS `ComputeLumpsumLineValues` (PRESVALU.pas:174-178) and the active
`ComputePeriodicLineValues` classifier ("NEW 3/31/92", :485-489) mark a
Date+Value lump / From+To+Value periodic as `fully_specified`: the missing Amount
is DERIVED and the Value is a KNOWN contributor. The port labeled these
`LineContainsUnknown` (numerically identical for a single row), which on a
multi-row screen let a value-bearing "known" row steal backward dispatch from the
genuine single-field unknown.

### Resolution

`FirstPass` now classifies Date+Value and From+To+Value as `fully_specified`;
`forwardOnly` and `computeKnownRowSum` value them via `valueFullySpecifiedLump` /
`valueFullySpecifiedPeriodic` (deriving/echoing the Amount, "essentially zero"
divide guard preserved). Guard: `internal/api/pv_multirow_backward_test.go`. Also
updated two `dispatch_matrix_test.go` rows and
`TestFirstPassPeriodicZeroValueIsValidZeroRow` to the DOS-faithful taxonomy.
Amount+Value (solve Date) and From/To+Amount+Value shapes remain row-level solves.

## 27. Present Value 365/360 basis missing the DOS "kicker" — FIXED (2026-07-12)

**Status:** RESOLVED. The 365/360 basis now applies DOS's 365/360 rate "kicker",
matching the DOS oracle to the cent on both lump-sum and periodic PV. Found by a
client comparison after the Exact-method fix (§25).

### Symptom

On the **365/360** basis a Present Value read ~1.5% high vs the legacy app, on
BOTH the lump and the periodic. Reproduced (real DOS engine, PV screen, basis
365/360, Exact=YES): as-of 04/25/2006, Loan 5.0000% (true rate 4.9896%), lump
$5,000,000 on 10/01/2027, periodic $3,500/mo 05/15/2030→12/15/2060. DOS: lump
1,664,120, periodic 189,127.30, total 1,853,247.7. The port returned lump
1,689,336.53, periodic 193,898.27, total 1,883,234.79. The 365 basis matched to
the cent all along.

### Root cause

For the `x365_360` basis DOS discounts with an **internal rate scaled in YIELD
space by `kicker = 365/360`** (PEDATA.pas:141), while `YearsDif` stays Julian/360
(the active `SetYrDays` maps x365_360 → yrdays 360; the alternative 365 mapping is
commented out). The cell layer converts internal→displayed by dividing the yield
by the kicker (INTSUTIL.pas `PercentValueFromCell`, tratecol/x365_360), so the
stored discount rate is `RateFromYield(YieldFromRate(displayed, n)·kicker, n)`
with `YieldFromRate(r,n)=n·(exp(r/n)−1)`, `RateFromYield(y,n)=n·ln(1+y/n)`,
`n = RealPerYr(peryr)`. The Go port applied neither the kicker nor any 365/360
scaling — it discounted at the raw displayed true rate over Julian/360 — so every
365/360 discount was too small by the kicker factor.

### Resolution

`pvKickerRate` (handlers.go) applies the yield-space transform to the discount
rate at the handler boundary for the 365/360 basis (forward rate and each
variable-rate schedule rate); `pvUnkickerRate` undoes it when echoing a solved
rate so the UI round-trips the displayed value. COLA is not kicker-scaled (DOS
`COLAcol` has no kicker), so only the discount rate is transformed. Verified to
the cent: 365/360 lump 1,664,120.45 (DOS prints whole dollars → 1,664,120),
periodic 189,127.30, total 1,853,247.75; the 365 basis is unchanged. Guard:
`internal/api/pv_basis365360_test.go` (fails at the pre-fix 1,689,336.53 /
193,898.27 without it). The default payments-per-year for the kicker's
`RealPerYr(n)` is 12 (the PV screen default); non-12 PV defaults would need that
value threaded through if ever configurable.

### Validation note

Reproduced and fixed in-sandbox to the cent; full presentvalue + api + cmd Go
suites green. The gated DOS oracle PV sweep (`make ci`) should reconfirm across
day-of-month / offset / COLA on the 365/360 basis.

## 28. 365/360 "kicker" is a UI cell-layer transform (app vs oracle) — Amortization FIXED (2026-07-12)

**Status:** Go RESOLVED for Amortization (app-fidelity). **Action required:** the
oracle harnesses must be updated to re-validate (see below).

### The finding (unifies §25/§26/§27 and the amortization payment)

On the **365/360** basis DOS scales the rate by `kicker = 365/360` (PEDATA.pas:141)
before computing. This is a **UI cell-layer** transform (`PercentValueFromCell`,
lratecol/tratecol/aratecol arms, INTSUTIL.pas:1564-1650): the DOS **app** applies
it when a rate is typed into a cell, so the stored/internal rate the calc uses is
`displayed × kicker`. The **headless oracles bypass it** — they assign the rate
directly (`amort_oracle.pas:50 loanrate := pRate`; `pv_oracle.pas c[1]^.r.rate :=
pRate`). So on x365_360 the DOS **app** and the DOS **oracle disagree**: the app
is ~1–1.5% "more discounted" (higher amortization payment, lower PV). The port
followed the oracle, so every 365/360 result was off vs the app.

### Symptom (amortization)

$1,000,000, loan 01/15/2001, 8.0000%, 1st pmt 05/01/2001, 360×12, basis 365/360,
Exact=YES. DOS app payment **7,498.56**; port returned **7,419.50**. (The 360 and
365 bases were always correct.)

### Resolution (Go — app-fidelity, per the 2026-07-12 decision)

`amzKickerRate` (handlers.go) scales the nominal loan rate by a plain ×365/360 on
x365_360 at the handler boundary (`input.Loan.LoanRate` then holds DOS's internal
rate); `amzUnkickerRate` undoes it on the echoed/solved rate. Amortization payment
now 7,498.56 (= app); 360 basis unchanged. Guard:
`internal/api/amort_basis365360_test.go`. This mirrors the PV kicker (§27,
`pvKickerRate`, yield-space because the PV rate is continuous, not nominal).

### Revalidation (2026-07-13) — NO oracle-harness change needed

The DOS oracle harness represents the headless calc ENGINE (it assigns the rate
directly, bypassing the cell layer), and the Go kickers live in the API HANDLER,
not the engine. So the architecture mirrors DOS exactly: cell layer (kicker) →
calc engine. Consequences, all verified by building/running the oracle in-sandbox
(`legacy/oracle/build_linux.sh`):

- **Engine ≡ unkicked oracle**, including x365_360. Both gated sweeps
  (`internal/finance/amortization` amort sweep; `internal/finance/presentvalue`
  `dos_pv_gen`) pass with `PERSENSE_REQUIRE_ORACLE=1` — the kickers touch only the
  handler, so the engine is unchanged and still matches the oracle to the cent.
- **Handler ≡ DOS app (kicked)**, validated two ways: the handler regression
  tests pin the DOS-app screenshot values (amort 7498.56; PV lump 1,664,120,
  periodic 189,127.30), and a spot check where `amort_oracle`'s x365_360 arm was
  *temporarily* kicked reproduced the Go handler to the cent (7497.6677 ==
  7497.67 for the shared loan-2024-01-01 case).

Therefore the oracle sources stay **unkicked** (engine-faithful) and are NOT
changed — kicking them would break the engine sweeps (the engine is unkicked by
design). The earlier "update the oracle harness" note was mistaken: it conflated
the app (cell layer + engine) with the oracle (engine only). If a future
belt-and-suspenders oracle-differential test for the *handler* kicker is wanted,
add an opt-in `appkick` flag to the oracle drivers (applying the kicker) used only
by a new handler-level differential test — leaving the default unkicked so the
engine sweeps stay valid.

## 29. Stepped-COLA anniversary clamped by AddYears in life/VR paths — FIXED (2026-07-13)

**Status:** RESOLVED. The life-contingent and variable-rate stepped-COLA paths now
advance the COLA anniversary with a plain year-field increment, matching DOS and
the fixed-rate path. Audit finding D1.

### Root cause

DOS advances the COLA anniversary with `inc(coladate.y)` — a plain year-field
increment (PRESVALU.pas:289,302). The Go **fixed-rate** stepped path
(`periodicSumAnnualCOLA`) already did this (its `incYear` closure) and is
oracle-validated. But the **life** path (`periodicWithActuarial`) and the
**variable-rate** path (`vrPeriodicValue`) advanced it with `dateutil.AddYears`,
which CLAMPS month-ends (Feb-29 → Feb-28). On a leap-day / month-end `fromDate`
that lands the COLA step one payment early, so those paths diverged from the
fixed-rate path (and DOS). `firstCOLAStepDate` (shared by both) had the same
AddYears clamp in its ANN arm.

### Resolution

Added `nextColaAnniversary` (a plain year++ = `NewDateRec(y+1, m, d)`, identical to
the fixed-rate `incYear`); `firstCOLAStepDate`, the life-path increment, and the
VR-path increment all route through it. The fixed-rate `incYear` closure now
aliases it too (behavior-preserving — the gated `dos_pv_gen` PV sweep stays
green). Guard: `internal/finance/presentvalue/cola_anniversary_d1_test.go`
(`firstCOLAStepDate(2024-02-29, ANN)` = 2025-03-01, not the AddYears-clamped
2025-02-28).

### Note — separate VR/life-vs-fixed leap-anchor difference (out of scope)

While reproducing D1, a *separate* smaller divergence surfaced: the VR/life paths
use a period-by-period loop over ACTUAL payment dates, while the fixed-rate path
uses DOS's three-part decomposition with NOMINAL (1/peryr) spacing
(SummationForSteppedCola; cf. `pv_periodic_divergence_frontier.md` §3). On a
leap-day `fromDate` these two methods differ even with `cola=0` (~0.015% on a
360-basis test), amplified under COLA. This is a pre-existing method difference,
NOT D1, and is not addressed here. Validating the VR path against DOS's own VR
(`pvlfancy`) mode for leap-anchored stepped COLA is a separate follow-up (the
pv_oracle does support a variable-rate mode).

## 30. PV rate solver capped the second pass at 30 iterations — FIXED (2026-07-13)

**Status:** RESOLVED. The PV-8 rate solver now converges on high rates, matching the
DOS oracle. Audit finding B2, confirmed vs the built oracle (`bk_rate` mode).

### Symptom / root cause

DOS's rate solve (`FrontwardCalc`, PRESVALU.pas:694-747) is a damped Newton
(±0.04/step) from a 0.1 seed that restarts once from 0. The restart
(`goto START_AGAIN_FROM_0`, :707) does NOT reset the byte `count`, so the second
pass runs a full byte cycle (~256 iterations) before `count` wraps back to 30. The
port used an outer `for attempt` loop that reset `count` to 0 each pass, capping the
second pass at 30 steps — not enough to walk a ±0.04-clamped Newton up to a high
rate. Confirmed vs the oracle: DOS solves 150% / 200% / 300%; the old port returned
"did not converge" for every true rate above ~120%.

### Resolution

`solveRate` (backward.go) rewritten as a single loop with a `uint8 count` that is
NOT reset on the restart, reproducing DOS's byte-wrap; the second pass gets ~256
iterations. Verified vs oracle (`bk_rate`): 100%/150%/200%/300%/400% all match.
Guard: `internal/finance/presentvalue/rate_solve_highrate_b2_test.go`. Gated PV
sweep stays green (moderate-rate solves converge on the first pass, unaffected).

## 31. PV perpetual-stream guard used raw-yield instead of continuous COLA — FIXED (2026-07-13)

**Status:** RESOLVED. Audit finding A1.

### Symptom / root cause

`PeriodicSummation`'s infinite-series guard (for a perpetual stream, toDate ==
latest) rejected the stream when the RAW yield `cola >= rate`. But the series
converges to a FINITE PV whenever the rate exceeds the CONTINUOUS COLA, ln(1+cola)
— which is what DOS compares (its stored COLA is already continuous,
PRESVALU.pas:379). So in the band rate in (ln(1+cola), cola] the port errored where
DOS returns a finite value (e.g. cola=6% yield, rate=6%: ln(1.06)=5.827% < 6%, so it
converges).

### Resolution

The guard now compares `math.Log1p(cola)` against the rate (calc.go). Guard:
`internal/finance/presentvalue/infinite_guard_a1_test.go`; the existing
`TestPeriodicSummationInfinite` was corrected (it had encoded the raw-yield
threshold). Reach is low — a perpetual (toDate=latest) stream mostly appears in
backward-solve internals — but the guard is now DOS-faithful.

## Remaining audit candidates — classification (2026-07-13)

Confirmed vs the built oracle and classified rather than "fixed", consistent with
the deliberate-divergence policy (§7, §24) — these are cases where DOS refuses or is
coarser and the port's answer is correct/better:

- **B3 (lump-date solve beyond ~2099):** DOS aborts (`if wdate.y>199 then count:=30`,
  PRESVALU.pas:915) because its 2-digit year can't represent the date; the port
  returns the correct far-future date (oracle refuses at sv→2116/2162/2208, port
  returns them). DELIBERATE divergence — the port's answer is financially correct.
- **B4/B5 (PV-6 fromDate solve):** the port refines fromDate to the day where DOS
  leaves it on the whole-period grid, and its COLA second-approx seed differs; both
  hit the target Value and the reported fromDate agrees within <1.5 days (the gated
  `dos_pv_backward_boundary` sweep passes). The day-precise date is the deliberate
  enhancement the audit flagged. ACCEPTABLE.
- **D2 (VR POD 365-day date synthesis):** BLOCKED — the DOS actuarial unit
  `ACTUARY.pas` is absent from `legacy/`, so there is no oracle authority to diff
  the POD/life math against.
- **D1 follow-up (VR/life leap-day COLA anniversary):** RESOLVED — see §32 below.
  The pv_oracle `pvlfancy` VR mode (`vrp_gen`) was built and used to reproduce and
  confirm the fix to the cent.


## 32. Leap-day COLA anniversary normalized to Mar-01 in per-payment paths — FIXED (2026-07-13)

**Symptom.** A periodic PV stream anchored exactly on **Feb-29** with a stepped COLA
read ~0.047% low vs the DOS engine. Reproduced to the cent via the pv_oracle `vrp_gen`
(`pvlfancy` VR) mode: as-of 01/01/2024, $1,000/mo from 02/29/2024, 60 pmts, 12/yr, flat
5%, COLA 3% (ANN):

| basis   | DOS Sum Value | port (pre-fix) | diff |
|---------|---------------|----------------|------|
| x360    | 56,655.888694 | 56,629.2646    | −26.62 |
| x365    | 56,660.904252 | 56,634.2807    | −26.62 |
| x365/360| 56,553.090569 | 56,526.5477    | −26.54 |

cola=0 and every non-leap anchor matched **exactly** on all three bases, isolating the
gap to COLA stepping on a leap-day anchor.

**Root cause.** The per-payment stepped-COLA loops — variable-rate `vrPeriodicValue`,
life `periodicWithActuarial`, and the exact fixed-rate branch of `periodicSumAnnualCOLA`
— advanced the COLA anniversary with `nextColaAnniversary`, whose `types.NewDateRec`
**normalizes an invalid Feb-29 to Mar-01**. DOS instead holds the anniversary as a raw,
unnormalized `daterec {29,2,y}`: `dateok()` only checks `1<=month<=13`
(INTSUTIL.pas:584), and `DateComp` overlays `(d,m,y)` as a longint comparing y→m→d
(INTSUTIL.pas:828,66). So DOS's Feb-29 anniversary sorts **between Feb-28 and Mar-01**,
and in a leap year lands exactly on the Feb-29 payment (`UpdateAmountWithCola` /
`FancySummation`, PVLXSCRN.pas). The normalized Mar-01 anniversary stepped one payment
late every leap year, under-COLA'ing that Feb-29 payment (verified by per-payment trace:
on 02/29/2028 DOS carried cola 1.12550881, the port 1.09272700).

**Fix.** New `colaAnniversary{year,month,day}` value type with `reached(t)` (DOS's raw
y→m→d longint order) and `next()` (whole-year increment = `inc(coladate.y)`), plus
`firstColaAnniversary`. All three per-payment loops route through it. For **every
non-Feb-29 anchor** the raw and normalized comparisons are identical (the anchor's own
month/day is valid in every year), so this changes results only for the leap-day corner
and regresses nothing — non-leap Mar-15 still matches to the cent across all three bases,
and every gated PV sweep (`PERSENSE_REQUIRE_ORACLE=1`) stays green. Post-fix the table
above matches DOS exactly on all three bases.

The fixed-rate **non-exact closed-form** path is deliberately untouched: it sums whole
years via `SumFormula(nfullyears)` and only reads coladate at a boundary that resolves
identically for a Feb-29 ANN anchor (fromDate < the first anniversary either way), so it
is immune. This supersedes §29 for the per-payment paths — §29 moved the clamp from
Feb-28 to Mar-01, which was still one payment late; §32 lands it on DOS's true Feb-29.

**Not a bug (recorded to prevent a regression):** a diagnostic harness that manually set
`YrDays=365` briefly showed a ~$5 x365-only gap. The real handler is correct —
`interest.NewCalcContext` uses **365.25** for the `Basis365` PVL context, matching DOS's
`iPVL` branch `(Julian(z)-Julian(a))·yrinv` with a 365.25-day year (INTSUTIL.pas:787+).
With 365.25 the port matches DOS exactly on x365. Do not "correct" that constant to 365.

**Guard.** `internal/finance/presentvalue/cola_leapanchor_d1followup_test.go` (raw-order
semantics + the three-basis oracle values + fixed-rate-exact == VR-single-rate agreement).
**Tooling.** `legacy/oracle/pv_oracle.pas` gains the `vrp_gen` mode (`SetupVRPeriodicGen`)
driving the genuine DOS `FancySummation`/`ValueOfOnePayment` VR path.

## 33. Rate solve on Exact + 365/360 — DOS APP stale-session-state bug, port is correct (2026-07-22, CONFIRMED)

**Status:** ADJUDICATED — the port (web + engine) is correct; the shipped DOS app
has a rate-solve display bug in this corner. No code change; guarded by
`TestRateSolveExact365360Prepaid`.

**Client report:** solving for the rate is "off by 2%." Screenshots: Amount
100,000; Payment 750; 360 periods; 12/yr; loan 01/01/2025, 1st pmt 02/01/2025;
Basis **365/360**; **Exact** YES; 1st-interest-prepaid YES; arrears; Rate blank.
The web shows **Rate 8.0050% / APR 8.1158%**; the DOS app shows **Rate 6.2160% /
APR 6.3021%** — a ~1.79-point gap.

**Adjudication (round-trip against the headless oracle):**

| Rate fed forward (engine space) | Oracle payment |
|---|---|
| 0.081162 (web engine rate = 8.0050% × 365/360) | **749.9988** ✓ |
| 0.063021 (DOS-app 6.2160% × 365/360) | 624.9954 ✗ |
| 0.062160 (DOS-app 6.2160% raw) | 619.29 ✗ |

The DOS **headless engine** (`amort_oracle … solverate`, same Pascal source)
solves engine-space **0.0811622**, agreeing with the Go engine to <1e-5. Feeding
that rate forward reproduces the $750 payment to the cent. The web un-kicks it for
display (×360/365 ≈ 8.0050%), and re-entering 8.0050% re-kicks and reproduces $750
— the display round-trips.

The DOS **app's** 6.2160% does NOT round-trip: kicked to 6.3021% it amortizes to a
**$625** payment, not $750. So the app's rate-solve cell layer is self-inconsistent
on Exact + 365/360 — it solved the wrong engine rate (≈6.30% instead of 8.12%) and
then displayed it un-kicked. This is the inverse of §28: there the port matches the
app for the FORWARD kicker (the app is right); here the app disagrees with its own
engine and the port deliberately does not reproduce the bug.

**Guard:** `internal/finance/amortization/dos_rate_exact365360_test.go`
(`TestRateSolveExact365360Prepaid`) pins the engine-space solve to the oracle,
asserts the ~8.116% band (a value near 6.30% would mean the port had reproduced the
app bug), and checks the forward round-trip retires at $750.

### Why the DOS app produces 6.2160% — mechanism (revised 2026-07-22, second pass)

**Retraction first:** the initial write-up blamed an inconsistent 365/360 kicker
composition in the app's cell layer. Quantitatively that cannot be right: the
kicker is ×365/360 ≈ 1.0139, so any single mis-application moves a rate by
~1.4%, and no small composition of kicker/yield conversions maps 8.116 → 6.302
(the gap is ~22% relative). The cell layer's display transform is in fact fine
(`aratecol: ReportedRate(rp)/kicker`, INTSUTIL.pas:1650 — plain nominal
division, matching the web's `amzUnkickerRate`).

**The actual fingerprint.** The app's stored engine-space rate (6.2160 × 365/360
= 6.30234%) is EXACTLY the correct solve for this loan at a payment of
**$625.00 = $750 × 5/6** (equivalently amount $120,000 = $100,000 × 6/5):

    amort_oracle 100000 0 360 12 b365_360 exact prepaid payhard=625 norate
      loandmy=1.1.2025 firstdmy=1.2.2025 → solvedrate 0.0630217
      (display: ×360/365 = 6.2158 ≈ the observed 6.2160 to display precision)
    amort_oracle 120000 … payhard=750 norate → solvedrate 0.0630217 (identical)
    forward at 0.0630234 → payment 625.01

So the DOS app's Newton CONVERGED CORRECTLY — on the wrong money. Its solve
consumed a payment of 625.00 (or an amount of 120,000) while the screen showed
the typed 750.00 / 100,000.

**Ruled out:** a settings-interpretation difference. Sweeping `solverate
payhard=750` over EVERY combination of {360, 365, 365/360} × {exact on/off} ×
{prepaid on/off} × {arrears/advance} × {plain/USA/R78} produces solved rates
only in **8.11%–8.24%** — no combination reaches 6.30%. Also ruled out:
convergence failure returning a garbage iterate (the answer is a clean, exact
solve for 625.00, not noise).

**Where the corrupted money could come from.** In this repo's (mid-port) source
the payment global is refreshed immediately before the solve (`d := h^.payamt`,
Amortize.pas:1329 — directly above the `EstimateAndRefineRate` dispatch at
:1338), so THIS tree cannot produce the bug — which is consistent with the
headless oracle (built from this tree) solving correctly. The client's shipped
DOS binary is a different, older build; two candidate faults fit the fingerprint:

1. **Stale solve state** — the shipped binary's solve path reads a payment (or
   amount) global left over from an earlier computation in the session; 625.00
   happens to be what was there. (The web port's own "reuse without clearing"
   audit, docs/reuse_no_clear_findings.md, documents how easily leftover state
   feeds Amortization recalcs.)
2. **A structural ×5/6 mis-scale** — e.g. a 10-vs-12 payments/yr mixup
   (750 × 10/12 = 625 exactly) somewhere in that binary's solve wiring. No 5/6
   constant is visible in this source tree.

**CONFIRMED — hypothesis 1 (stale session state), 2026-07-22.** The client
cold-restarted the DOS app and re-entered the identical case; it then solved
**Loan Rate 8.0050% / APR 8.1158%** — matching the web to display precision. So
the 6.2160% was NOT reproducible from a clean state: the shipped binary's
rate-solve consumed a payment (or amount) left over from an earlier computation
in the same session (the 5/6 fingerprint = a stale $625 / $120,000), and a fresh
process clears it. This is a DOS-app **session-state carryover bug**, not a
structural mis-scale and not a settings difference — the ×5/6 constant never
existed in the code; it was just whatever stale money was in the global.

The discriminating experiment that settled it: cold restart → 8.0050 confirms
stale state; the alternative test (fresh entry of payment 900 solving to 8.0050
instead of the correct 10.0189%, which would have implicated a structural ×5/6)
was not needed once the cold restart already reproduced the correct value.

Practical note for the client: on the DOS app, **start a fresh worksheet (or
restart) before a rate solve** if the session has computed other loans — the app
does not fully clear the prior payment/amount before solving. The web port has
no such carryover (each request rebuilds the worksheet from the posted fields),
and its "reuse without clearing" behavior is separately audited in
docs/reuse_no_clear_findings.md.

The adjudication stands: the port is correct (its solve round-trips to the
entered payment; the stale DOS-app solve does not), and
`TestRateSolveExact365360Prepaid` guards it.

## 34. Solved target-balloon not re-read as blank → stale, non-updating recalc — FIXED (2026-07-23)

**Status:** FIXED (frontend). Client-found via UI testing: solve a target balloon
(date-only Balloon → engine solves the Amount), then change any other input (e.g.
add a Skip Month) and Calculate — **nothing updates**; the balloon keeps its
first solved value.

**Root cause (frontend, not engine).** `getAmzInput` collected the Balloon
Amount cell with a raw `.value` read, unlike the top-line cells which treat a
green (`cell-output`) solved cell as BLANK (`amzVal`). So after a target-balloon
solve, the green solved amount was re-sent as a **fixed** balloon
(`balloons:[{date, amount: <prior solved value>}]`). The worksheet was then
over-determined by the balloon's own prior output, so the engine echoed it back
unchanged — the skip (or any edit) had no effect. Confirmed by capturing the POST
body: `"amount":-132614.64` where it should have been absent.

Also latent: the global input handler cleared `cell-output` on edit only for PV
cells, not the Amortization advanced grids, so typing over a solved advanced-grid
value did not turn it back into a real input.

**Fix (cmd/persense/static/index.html):**
1. `getAmzInput` Balloon collection: a green Amount cell re-reads as BLANK, so a
   date-only target balloon re-solves on every Calculate (mirrors `amzVal`).
2. Input handler: editing a green cell inside `#amz-balloon-body /
   #amz-prepay-body / #amz-adj-body` drops `cell-output`, so a typed-over value
   becomes a real fixed input.

**Verified (Playwright, harness/regress_balloon_skip.js):** on the client's exact
case (10k / 8% / 360 / 12, 365/360, Exact, prepaid, balloon-includes-regular=YES,
payment 733.76, balloon @ 02/01/2030): solve → −132,614.64; then add Skip 7-8 and
recalc → **−107,865.54, matching the DOS app to the cent** (was stuck at the
no-skip value before the fix). Type-over case (type 5000 over the solved balloon)
correctly yields a fixed 5000, not green. Full `go test ./cmd/persense ./internal/api`
green.

NOTE: this is a display/dispatch bug distinct from §28/§33; the underlying engine
was always correct (a fresh solve with skip already produced −107,865.54, which
matches the DOS app — the headless oracle differs by ~$694 on this 365/360 solve,
the usual app-vs-oracle split).

## 35. AO9 solved prepayment amount never surfaced to the UI — FIXED (2026-07-23)

**Status:** FIXED (API handler + frontend). Found while hunting the same
advanced-options stale/echo class as §34. An "unknown prepayment" (AO9 — a
prepay series with a **# Pmts count but a blank Amount**, plus a given payment)
is solved by the engine (`dosport_entry.go:621`,
`res.SolvedPrepay = e.pres[e.unkPre].payment`), and the resulting schedule is
correct and retires. But the value was **dropped at the API layer**: the
`/api/amortization/calc` handler never mapped `AmortResult.SolvedPrepay` into its
response, so `solvedPrepay` was absent from the JSON and the UI left the
prepay Amount cell blank. The user saw a correct schedule but no computed
per-payment prepay — the same "solved value invisible" gap the balloon echo
(§34 sibling) already closed for balloons.

**Fix:**
1. `internal/api/handlers.go` — added `SolvedPrepay *float64 \`json:"solvedPrepay,omitempty"\``
   to `AmortizationResponse` (pointer so a solved 0 is distinguishable from
   "not solved"), and mapped it after the balloon echo:
   `if result.SolvedPrepay > 0 { v := interest.Round2(result.SolvedPrepay); resp.SolvedPrepay = &v }`.
2. `cmd/persense/static/index.html` (`calcAmortization`) — mirror of the balloon
   echo: when `data.solvedPrepay != null`, write it green (`cell-output`) into the
   single prepay row whose Amount is blank or a prior solved output.

The §34 companion fixes already make this safe against re-feeding: `getAmzInput`'s
prepay collector reads a green Amount (and stopDate) as BLANK (`rawG`), and the
global input handler drops `cell-output` when a prepay cell is edited — so a
recalc **re-solves** rather than treating the green output as a fixed input.

**Verified (Playwright, real UI vs engine):** 67000 / 11.37% / 24 mo / payment
2269.90, prepay start 08/01/2024, 6/yr, 4 payments, blank amount → the Amount cell
shows **$5,074.92** green, schedule retires (last principal 0), and
`solvedPrepay` = 5074.92 in the response. Idempotent: a second Calculate with the
green cell untouched re-solves to the same 5074.92 (no drift). Solve-then-edit:
changing # Pmts 4 → 6 re-solves to **$3,446.67** (more prepays ⇒ smaller each),
loan still retires — no stale carryover. Companion check: target-balloon
date-only solve 01/01/2030 → $41,661.70, edit date → 01/01/2031 re-solves to
$45,119.60, idempotent. `go test ./internal/api ./cmd/persense
./internal/finance/amortization` green.

**Not surfaced (noted, not a correctness/stale bug):** an AO6 adjustment
(payment change with a blank Rate ⇒ engine solves the *implied* rate) still has no
result field to echo the solved rate back into the adjustment grid's Rate cell —
the schedule is correct, but the solved rate is not displayed. Adjustment cells
never render green, so there is no input-leak/stale hazard; this is a pure
display nicety, deferred.

## 36. Prepayment # Pmts not displayed for a Stop-Date-bounded series — FIXED (2026-07-23)

**Status:** FIXED (API + frontend). Client-found via UI comparison. When an
Additional Periodic Payment series is entered with a **Start Date + Pmts/Yr +
Stop Date but a blank # Pmts**, the DOS app derives and DISPLAYS the payment
count in the grid (e.g. 107). V3 computed the same series internally (the
schedule was correct) but left the # Pmts cell blank.

**Fix:**
1. `internal/api/handlers.go` — `countPrepayDates(start, stop, perYr)` counts the
   payment dates from Start, stepping via `dateutil.AddPeriod`, that fall on or
   before the Stop Date — the same walk the engine's prepayment loop uses to
   bound the series. Exposed as `prepayResolvedNN []int` (aligned to the request's
   prepayment order; 0 = not derived).
2. `cmd/persense/static/index.html` — `calcAmortization` paints the derived count
   green into the blank # Pmts cell, row-aligned. The prepay collector now reads a
   green # Pmts as BLANK (`rawG`), the symmetric twin of the existing derived
   Stop-Date handling, so the count re-derives from the Stop Date on recalc
   instead of being re-sent as a hard count (stale-proof).

**Verified (Playwright, real UI):** 100k / 8% / 360 / 12, payment 733.76, skip
7-8, prepay Start 01/01/2026, Stop 06/15/2030, 24/yr, $500 → # Pmts cell shows
**107** (green), matching the DOS app. Idempotent on recalc; editing the Stop
Date to 06/15/2028 re-derives to **59**. Confirmed the count equals the engine's
own AddPeriod stepping (the 107th step lands 06/16/2030, just past the stop).
`go test ./internal/api ./cmd/persense` green; 60-case advanced-options fuzz on
the new build: 34/34 oracle matches, 0 stale.

## 37. Amortization APR investigation — RESOLVED: web APR is DOS-exact (2026-07-24)

**RESOLUTION:** No bug. The web APR matches the DOS `amort_oracle` to the digit
once the SAME payment configuration is compared. The apparent ~2 bp gap was a
flawed differential: the web ran a HARD payment (733.76, which retires the loan
early and leaves a value_calc phantom-region residual) while the oracle, given no
`payhard`, SOLVED a lower payment (703.60, retiring exactly at term) — two
different loans. Proven by instrumenting the DOS engine's own value_calc
discounting stream (a temporary `aprdump` flag in AMORTOP.pas RepayFancyLoan,
since reverted): with `payhard=733.76` the oracle produces the IDENTICAL residual
(-44,943.66 @ 1/1/2054) and IDENTICAL APR (0.082433) as the web; with the payment
solved, both give 0.082214. Matched in both configurations:

| config (100k/8%/360/12, 2pts, monthly prepay 200x24) | web | oracle |
|---|---|---|
| hard payment 733.76 | 0.082433 | 0.082433 |
| solved payment (703.60) | 0.082214 | 0.082214 |

The client's screenshot 8.1155 vs 8.1166 is therefore a config/build artifact
(hard-vs-solved payment, or the screenshot build predating this repo), NOT a code
defect — the current engine is APR-faithful. No code change.

---

### Original investigation (superseded by the resolution above)

**Observed:** client reports the Amortization **APR %** differs slightly between
V3 and the DOS app on a 0-point loan — web 8.1155% vs DOS 8.1166% (their
screenshots), on 100k / 8% / 360 with the prepay series + skip active.

**Mechanism (confirmed):** with 0 points the APR is NOT the nominal 8%. DOS
(`EstimateAndRefineAPRwithPoints`, Amortize.pas:516) solves the rate at which the
PV of the ACTUAL payment stream — including the extra $500 prepayments — equals
`amount·(1−points) − PrepaidInterest`. Here `PrepaidInterest = 0` (byte-identical
DOS↔port: firstDate is exactly one period after loanDate, so YearsDif steps to 0)
and points = 0, so target = 100,000; the extra prepayment outflows raise the
solved rate above 8%. So a >8% APR with 0 points is expected, and a sub-basis-
point web/DOS difference is the iterative secant solver converging to a slightly
different point (DOS: 20-pass secant; the port mirrors it) — a benign numerical
difference, not a logic error.

**Root cause localized (oracle differential):** the web APR engine is EXACT
against the DOS `amort_oracle` when there are NO prepayments, and diverges only
when a prepayment series is present:

| case (100k/8%/360/12) | web APR | oracle APR | Δ |
|---|---|---|---|
| 2 points, no prepay | 0.082140 | 0.082140 | **0.000000** (exact) |
| 3 points, no prepay | 0.083238 | 0.083238 | **0.000000** (exact) |
| 2 points + monthly prepay 200×24 | 0.082433 | 0.082214 | **+0.000219** (~2 bp) |

So the APR *solver* is faithful; the ~2 bp gap is isolated to how the
**prepayment cash flows enter the APR value stream**. The SCHEDULE is correct —
the web's schedule interest matches the oracle to the cent (131,141.44; the web's
displayed total is +points, the DOS-faithful settlement line), and the loan
retires cleanly (FinalPrinc ≈ 0). The divergence lives in `aprValueCashflows`
(backward.go:640) → `generateFancyScheduleMode(value_calc=true)` + the appended
terminating balloon at `FinalPrinc`/lastDate, vs DOS's `RepayFancyLoan` value_calc
discounting. This 2 bp effect is the "slight difference"; on the 0-point case
points no longer dominate so the same modeling gap surfaces larger.

**Caveat — build divergence on the exact number:** running the client's EXACT
0-point inputs against the build in THIS workspace yields APR ≈ 8.02% (8.0192
with payment 733.76), not their 8.1155%. The screenshot build and this repo are
not bit-identical on the 0-point display, so the exact 8.1155-vs-8.1166 figure
can't be reproduced here — but the oracle differential above pins the real,
reproducible defect regardless of build. FIX DIRECTION: align the value_calc
prepayment/terminating-balloon discounting in `aprValueCashflows` with DOS
`RepayFancyLoan` value_calc so the with-prepayment APR matches the oracle to the
same precision the no-prepayment case already does.

## 38. PV row-target backward solves: $0.00 screen total + recalc drift — FIXED (2026-07-24)

**Status:** FIXED (engine + frontend). Found by the full three-section
UI-vs-oracle differential run (harness/run_pv.js), not user-reported.

**38a — engine: date solves left the screen total at $0.00.** A backward solve
whose target is typed in a ROW's Value cell (screen Present Value blank) — the
PV-2 lump-date and PV-5/PV-6 periodic-date solves — solved the date correctly
(oracle-exact) but echoed `SumValue = PresVal.SumValue = 0`. The UI faithfully
painted a **$0.00** Present Value over a screen whose rows sum to the target,
and the P-W7 "payments net to about zero" advisory fired on top. DOS never
shows this state: Enter always follows BackwardCalc with FrontwardCalc
(PRESVALU.pas:1253 `for i := nlines downto stopat do FrontwardCalc(i)`), which
re-sums the rows into `sumvalue`. Fix (`presentvalue/backward.go` BackwardCalc):
when the solve succeeded and no screen-level Sum Value was supplied, complete
the DOS flow by summing the row values the solvers already wrote. Guarded by
`TestBackwardRowTargetSumValue` (all three date solves + a screen-target
control). The amount solves were unaffected (they route through the §26
value-pinned forward path, which sums normally).

**38b — frontend: display-precision re-send drifted recalcs.** The PV response
handler re-sends solved values on the next calc (deliberate, DOS-like — DOS
treats solved outp cells as known), but re-parsed them from the DISPLAY text:
a solved amount 974.495922 re-sent as $974.50 turned a 33,000.00 total into
**33,000.14** on an untouched recalc; a solved rate re-sent at 4-dp display
precision turned 44,000.00 into **43,999.95**. DOS keeps the solved value as a
full real in the cell, so its recalc reproduces the target exactly. Fix
(index.html): `writeOut` now stashes the engine's full-precision number on the
cell (`dataset.pvRaw`), `getPVInput` prefers the stash while the cell is still
green (editing drops the green, so typed text always wins), and the hidden rate
canonical stores full precision (it has no display constraint; the three
visible rate boxes keep their 4-dp display). Verified: PV harness idempotency
strict-equality passes on every amount/rate solve.

**Date-solve recalc note (not a bug):** after a date solve, an untouched second
Calculate legitimately lands NEAR (not on) the target — the solved date is
day/period-granular, and the forward pass from the snapped date differs from
the target by up to ~one period's discounting. DOS does the same on re-Enter.
The harness asserts the solved DATE is stable across the recalc instead.

## 39. Three-section UI-vs-oracle differential run — CLEAN (2026-07-24)

Full manual-test-plan sweep through the REAL browser UI vs the DOS oracles:

| Section | Harness | Result |
|---|---|---|
| Amortization | run_amz.js (29 cases) | 21/21 core pass; 8 flagged = known ARM one-month split (§28-class, web matches the app/manual) + 4 harness balloon-encoding limitations |
| Present Value | run_pv.js (26 cases) | **26/26 pass** after §38 — forward (lump/periodic/mixed/negative/365/long-n), COLA (ann/cnt/month-specific), variable-rate lump+periodic, and ALL SEVEN backward solves (lump amt/date, periodic amt/to/from, rate, as-of) match pv_oracle to the cent/day, idempotent |
| Mortgage | run_mtg.js (12 cases) | **12/12 pass** — monthly/price solves, all three funding branches (pct/cash/financed), points+tax, known balloon, balloon-amount solve, APR with/without points vs the real ReportAPR, idempotent (the mtgStatus input/output machinery is stale-proof by construction) |

mtg_oracle built for the first time this session (TARGET=mtg_oracle
build_linux.sh). Harness infrastructure notes: (1) the app restores the
worksheet from localStorage on load and silently re-sums leftover rows — the
exact behavior the manual plan warns testers about; harnesses clear storage via
addInitScript before every reload; (2) cdn.tailwindcss.com stalls 'load' ~13s
in the sandbox — harnesses block external requests.

## 40. Solver × advanced-options audit — 3 fixes, 2 frontier items (2026-07-24)

A dedicated differential sweep (`dos_solver_options_audit_test.go`, oracle-gated)
crossing EVERY backward solver — amount, rate, term, balloon-amount, AO9
prepay-amount, AO10 prepay-duration — with the advanced options the earlier
fuzzers never crossed: adjustments and target against the amount/rate/term
solves, options against the term/balloon/AO9/AO10 solves, and stacked option
pairs. ~800 randomized cases per run; classification mirrors fuzzer3-fancy.
End state: **0 hard failures, 6 residual divergences in 2 documented clusters.**

### 40a — FIXED: amount solve is SKIP-BLIND under DOS's prepaid shortcut

DOS `EstimateAndRefineLoanAmount`'s fast-path gate (Amortize.pas:458) —
`((basis=x360) or (not exact)) and prepaid and (nballoons=0) and (npre=0) and
(not in_advance)` — returns the closed form WITHOUT Iterate. Unlike the PAYMENT
solve's gate (:402), it does NOT exclude skip-months, so with prepaid set (the
UI default) a skip-months amount solve returns the skip-BLIND closed form and
the schedule folds the shortfall into the final payment. Oracle-verified:
`noamt pay=888.4879 prepaid skip=6-8` → DOS 18874.49 (= the no-skip value);
without prepaid → 14134.97 (skip-aware, = the port). Moratorium is unaffected
(the closed form handles it via nrepay: prepaid mor+skip = prepaid mor alone,
15305.10). Fix: `SolveLoanAmount` strips SkipMonths under exactly the DOS gate.
Guarded by `TestAmountSolvePrepaidSkipBlind` (3 oracle-pinned cases).
Adjustment-blindness needed no fix: DOS Iterates `til_adj` and the port's
fancyTerminal already strips adjustments to match (both solves land on the
no-adjustment root; oracle-confirmed identical across adj shapes).

### 40b — FIXED: term solve returned a phantom 959-period term where DOS refuses

`solveFancyTermFromPayment` caps the walk at 80 years (960 monthly periods) and
refused on `n >= cap` — but an IN-ADVANCE schedule emits one fewer row than the
scheduled term, so a never-retiring walk landed at 959 and slipped past the
guard, returning a phantom term where DOS says "Payment amount is too small to
compute number of periods." Fix: also refuse when the horizon walk leaves
`FinalPrinc > $1` (the semantic test, immune to row-count off-by-ones).

### 40c — FIXED: AO10 duration solve ignored the moratorium (repay_from anchor)

DOS `DeterminePrepaymentDuration` (Amortize.pas:709-774) is a closed form whose
every YearsDif anchors on `repay_from` (Amortize.pas:1260-1288): moratorium →
first_repay snapped to the payment grid then one period back; else prepaid →
firstdate one period back; else the loan date. The port hardcoded the loan
date, so a duration solve under a moratorium diverged wildly (audit case:
Go 55 vs DOS 27 extra payments, mor=44). Fix: `SolvePrepaymentDuration`
implements the DOS repay_from selection; the audit case now returns 27 exactly.
(The amount/AO9 closed forms don't need it — their estimates are refined
against the real schedule; AO10's closed form IS the answer.)

### 40d — FRONTIER (documented, not fixed): REPLACE-mode balloon+prepay same-date collision

When a balloon-amount solve runs in REPLACE mode (which `solveballoon=` forces
in DOS — and note the oracle sets `plus_regular := false` AFTER flag parsing,
a harness trap) and a prepayment series covers the balloon's own date, DOS
applies the balloon ALONE on that date (forward rows confirm: no prepay
anywhere on the balloon row), while the port's piecewise walk sums balloon +
prepay as the replacement payment. Signature: the solved balloon differs by
EXACTLY one prepay amount. Repro:
`amort_oracle 479945.47 0.027815 3 1 b365 prepaid pre=12:1:1:1989.91
payhard=195018.48 solveballoon=12` → DOS 118948.29, port 116958.38 (Δ =
1989.91). Correct fix needs DOS FindNextExtra/ComputeNext (AMORTOP.pas:490-621)
one-extra-per-date semantics reproduced in the walk's coincident-extras block
(engine.go ~2618-2688) plus a blast-radius sweep — deferred with the audit
test logging it. UI reach: balloonIncl=YES + a prepay series overlapping a
solved balloon date.

### 40e — ADJUDICATED: predur on negative-amortization input

`predur` (AO10) on a loan whose payment is below interest (never amortizes):
DOS emits `duration 0` (a numerical artifact of its log-solve — no error, a
zero-payment series), the port returns a count that also cannot retire the
loan. Both degenerate; no parity action. The API-level input advisories
("payment below interest due") already steer users off this input.

### Audit-harness traps recorded for future differential work
- `solveballoon=` forces plus_regular=false post-parse (40d) — mirror it.
- `rate`/`term`/`balloon` oracle outputs carry a status byte; status≠1 is a
  refusal, NOT a solved zero.
- The engine-internal solvers accept adj+in-advance; the API (like DOS)
  categorically refuses — audit at the right layer or mirror the guard.

## 41. Client APR discrepancy (fancy neg-am case) — web matches the DOS ENGINE; the app is the outlier (2026-07-24)

**Reported:** client screenshots — same rich loan in both apps — web APR **9.7310%**
vs DOS-app APR **9.8304%** (~0.10 pt). Loan: 100k, 365/360 + Exact, payment
733.76 (below interest ⇒ negative amortization, retires via implied terminal
balloon), rate SOLVED to 9.0041%, plus an OFF-CYCLE balloon (10/19/2027, between
payment dates), a REPLACE-mode semi-monthly prepayment series (107 × $500), and a
set-both rate/payment adjustment (10% / $800 @ 1/1/2028). Client noted the gap
appeared only once the adjustment was added.

**Finding: the web is APR-faithful to the genuine DOS engine; the DOS APP's
9.8304 diverges from its own engine.** Verified across ~25 differential probes vs
`amort_oracle` (the headless build of the same DOS Pascal units the app uses):

- Every sub-combination matches to the digit: adjustment alone; adjustment+prepay;
  adjustment+on-cycle balloon; 360 / 365/360 / 365/360-Exact; fixed rate; and the
  FULL case with the off-cycle balloon — engine **0.097301** vs oracle **0.097301**.
- The solved RATE matches exactly (internal 0.091267; displayed ≈ 9.0017%, →
  9.0041% at the client's 2025 dates — a pure loan-date shift, reproduced).
- On plain 360 basis the full stack (incl. off-cycle balloon) matches
  (0.100218); on 365/360-Exact at a fixed rate every option combo matches. The
  APR value-calc, the off-cycle balloon drain, the Re_Amortize on the adjustment,
  and the rate solve are all engine-faithful.

So the DOS app produces a number its own computational engine does not, and only
when the adjustment is present. This is an **app-vs-engine split**, the same class
as §33 (the DOS app showed a stale solved rate until a cold restart, which the
client confirmed). Recommended client check: cold-restart the DOS app, re-enter
the loan, and re-read APR — it should land at ~9.7310%. NO web code change: guard
`TestFancyAPRFullComboVsOracle` pins the engine=oracle match for this exact combo.

**Tooling added this pass:** `amort_oracle` gained a `bdate=D.M.Y:AMT` token for
OFF-CYCLE balloons (the `b<months>=` form only lands on payment dates) — the gap
that made this case un-expressible against the oracle until now. Also relied on
`pts=0` (explicit zero points) to make the oracle run its APR solve, mirroring
DOS MakeTable's `pointsstatus > defp` gate (Amortize.pas:1420) — the app computes
APR at 0 points because the field is explicitly 0, not defaulted.

## 42. Derived prepayment Stop Date stale/wrong for semi-monthly (24/yr) — FIXED (2026-07-24)

**Status:** FIXED (frontend). Client-found via a web-vs-DOS side-by-side. A
prepayment row displayed **# Pmts 107 @ Pmts/Yr 24 with Stop Date 11/01/2034** —
internally inconsistent: 107 semi-monthly payments from 01/01/2026 end mid-2030,
and 11/01/2034 is the 107-months-at-12/yr answer. The engine's SCHEDULE was
correct (the 107th extra lands 2030-06-01); only the displayed Stop Date was
wrong, which misled the reader into thinking the series ran to 2034.

**Root cause.** `addPeriodsForFreq` (the frontend helper `fillDerivedPrepayStops`
uses to show the Stop for a # Pmts-bounded series) handled 26/52 specially and
otherwise required `12 % perYr === 0` — so **24 (semi-monthly) fell through and
returned null**. Sequence: a Stop derived while Pmts/Yr was 12 (→ 11/01/2034,
green); flipping Pmts/Yr to 24 could not recompute (null), and the stale green
value was left on screen. (The reverse path — # Pmts derived from a Stop, §36 —
was fine: it uses the backend `countPrepayDates`/`dateutil.AddPeriod`, which
handles 24/yr.)

**Fix (`cmd/persense/static/index.html`):**
1. `addPeriodsForFreq` now has a 24/yr case that mirrors `dateutil.AddPeriod`'s
   semi-monthly stepping exactly (±15 days with day-of-month snapping and
   month-end clamp), so the derived Stop matches where the engine places the last
   extra payment. Verified inverse-consistent: engine's 107th semi-monthly extra
   = 2030-06-01 = the derived Stop.
2. `fillDerivedPrepayStops` now CLEARS a stale green Stop when the count is gone
   or the frequency can't be stepped, instead of leaving a wrong date showing.

**Verified** (harness/regress_prepay_stop_semimonthly.js): addPeriodsForFreq for
12/24/4 per year; and end-to-end #Pmts=107 with Pmts/Yr flipped 12→24 re-derives
the Stop from 11/01/2034 to 06/01/2030. `go test ./cmd/persense ./internal/api`
green.

**Note on the client's APR/rate gap in these screenshots:** the two apps' inputs
actually differed — the prepay was Pmts/Yr **12** in DOS vs **24** in the web, and
the balloon 10/**29** vs 10/**19** — so the different solved rates (6.2189 vs
9.0041) and APRs (9.4276 vs 9.7310) are expected for different loans, not an
engine divergence. The stale Stop Date (this bug) is the likely reason the inputs
drifted apart. With the fix the row stays self-consistent; entering identical
inputs in both apps should align them (per §41, the web matches the DOS engine).

---

## §43 — Rate/payment adjustment #2: APR is a DOS-*app* split; but the rate-ONLY ARM re-solve had a real prepay-clip bug (2026-07-24)

**Client report (screenshot #2).** A rich negative-amortizing loan showed web APR
**9.1677%** vs the DOS *app* APR **9.230%**. Inputs: $100k, pay 733.76 (rate
SOLVED to 8.5878%), 360/12, 365/360 + Exact + Prepaid, REPLACE mode; a semi-monthly
REPLACE-mode prepay series starting OFF-CYCLE on the 15th (1/15/2026, NN=97, 24/yr,
$100) while payments are on the 1st; and a set-both rate/payment adjustment at
1/1/2030 (→ rate 10%, payment $1000). "It was fine until the rate/payment adjustment."

**Finding A — the reported APR is a DOS-app split, now PROVEN against the genuine
engine (not merely asserted, cf. §41).** A faithful headless reproduction through
the real DOS units (amort_oracle) solves the SAME rate the web solves
(internal `0.0870707447`, displayed 8.5878%) AND computes the SAME APR
(`0.091677` = 9.1677%) — to the digit. The loan is heavily over-amortized (the APR
value walk ends on a ~−252k terminal balloon per DOS's RepayFancyLoan value_calc
tack-on, AMORTOP.pas:1224), and the port's `aprValueCashflows` discounts that
full-term stream identically. Baseline check: WITHOUT the adjustment both engines
give 8.7127%; adding the set-both adjustment moves BOTH to 9.1677%. So the web is
APR-faithful to the DOS COMPUTATIONAL ENGINE; the DOS APP's 9.230 is an
app-vs-engine split (the app diverges from its own engine only when the adjustment
is present — the §33/§41 DOS-app stale-state class). The web needs no change for the
reported number. Guarded by `TestFancyAPRAdjustmentOffCyclePrepayVsOracle`
(set_both / no_adjustment subcases).

**Finding B — a REAL port bug surfaced while probing (rate-ONLY variant).** Driving
the same loan with a rate-ONLY adjustment (10%, blank payment ⇒ DOS re-solves the
regular payment) exposed a genuine divergence: DOS re-solves the post-adjustment
payment to **814.52** and the loan amortizes (total interest 198,079); the port
re-solved **618.83**, far too low, so the loan never retired (total paid 505,608 vs
298,079). APR came out 9.5386% vs DOS 9.4060% (~13 bp).

**Root cause.** The piecewise engine's rate-only / AO7 re-amortization (engine.go
~3045) refines the segment payment with `solveSegmentPayment`
(`fancybisect.go`), which builds a sub-loan over [adjustment → last] and solves its
payment. It filtered BALLOONS to those after the boundary (`futureBalloons`) but
passed the prepayment series WHOLE (`input.Prepayments`) — original StartDate
1/15/2026, full NN=97. The sub-loan, whose loan date IS the adjustment, then
re-applied the ENTIRE 97-extra series from scratch, massively over-prepaying the
segment and solving far too low a regular payment. DOS's Re_Amortize instead uses
the prepay array in its PARTIALLY-CONSUMED state (`old_pre`, AMORTOP.pas:1552-1557):
only the extras still ahead of the adjustment bound the segment (here just the one
at 1/15/2030). `solveSegmentRate` (the AO6 payment-only analog) had the identical
latent bug.

**Fix (`internal/finance/amortization/fancybisect.go`).** New
`clipPrepaymentsForSegment(pps, boundary)` restricts each series to the extras
strictly after the boundary, rebuilt as an NN-bounded series starting on its first
surviving extra (mirrors old_pre's advanced nextDate + shrunk nn). Both
`solveSegmentPayment` and `solveSegmentRate` now feed the CLIPPED series into the
sub-loan (and the gate that decides whether to engage the oracle solve). With the
fix the rate-only re-solve gives 814.52 and APR 9.4060% = the oracle to the digit
(a ~$2 terminal-fold residual on total interest remains — the known task #103
ARM-fold gap, 1e-5 relative, not this bug).

**Also noted (not fixed — latent, no repro yet):** the port's reAmortize Iterate
gate (dosport_walk.go ~356) drops DOS's third clause `(exact and basis≠x360)`
(AMORTOP.pas:1571) — harmless here (prepays present force the Iterate anyway), but a
rate-only ARM with no balloons/prepays on the exact non-360 path would skip the
refine. Flagged for a focused follow-up.

**Verified.** `TestFancyAPRAdjustmentOffCyclePrepayVsOracle` (all 3 subcases: APR
AND total-interest vs oracle). Full `go test ./...` with `PERSENSE_REQUIRE_ORACLE=1`
green (amortization suite 86s, api 11s) — the shared re-amortization change caused
no oracle-differential regressions. New oracle tokens for the faithful repro:
`predmy=D.M.Y:NN:PERYR:AMOUNT` (off-cycle prepay start) and
`adjdmy=D.M.Y:RATE:AMOUNT` (absolute-date adjustment), plus loandmy=/firstdmy= to
keep all dates consistent at loan-year 2025.

---

## §44 — Rate/payment adjustment #3: the adjustment's NEW RATE was missing the 365/360 kicker; and DOS's `TackOnFinalBalloon` display row the port lacks (2026-07-24)

**Client report (screenshot pair #3).** "There's still a divergence here. DOS version
adds the balloon payments. The APR doesn't match. What is going on?" Same top-line
inputs on both sides: $100,000, loan 01/01/2025, rate 8.0050%, 1st pmt 02/01/2025,
360 periods, last pmt 01/01/2055, 12/yr, payment $750.00, points 0; one rate/payment
adjustment 01/01/2030 → new rate 10, new amount $1,000.00. DOS status line
`Settings: 365/360 Act Arr InclReg PrePd 1975 12perYr`.

- Web: APR **8.9703%**, Balloon Payments grid **empty**.
- DOS: APR **9.0493**, Balloon Payments shows a computed row **1/ 1/55, −159,554.53**.

Two independent causes, one real bug and one missing display feature.

### Finding A — REAL BUG. The API never applied the 365/360 kicker to an adjustment's new rate.

`INTSUTIL.pas` `PercentValueFromCell` (1648-1652) is the authority on which columns
carry the kicker, and it puts the adjustment new-rate column in the **same arm** as
the top-line loan rate column, exempting only the APR column:

```pascal
  aratecol,adjratecol,aaprcol : begin
                                if (df.c.basis=x365_360) and (col<>aaprcol) then
                                   PercentValueFromCell:=ReportedRate(rp^)/kicker
                                else PercentValueFromCell:=ReportedRate(rp^);
                                end;
```

`ReportedRate` (1499-1504) is the identity except for Canadian/daily peryr. So on
the `x365_360` basis a **displayed** adjustment rate of 10% is stored **internally**
as 10 × 365/360 = **10.13889%** — exactly the §28 treatment of the top-line rate.

`internal/api/handlers.go` applied `amzKickerRate` to `req.Rate` (handlers.go:931,
added in §28) but passed `req.Adjustments[i].Rate` straight through, so every ARM
row on the 365/360 basis ran ~1.4% light from the adjustment date onward.

**Decisive differential** (headless oracle, real DOS units; all on
`b365_360 exact prepaid`, loan 1/1/2025, first 1/2/2025, 100000, 360, 12/yr,
`payhard=750.00`, `pts=0`, adjustment 1/1/2030 amount 1000 — varying ONLY the
adjustment rate that reaches the engine):

| top rate (internal) | adj rate (internal) | oracle APR | terminating balloon | interest / paid |
|---|---|---|---|---|
| 0.0811618 (kicked) | 0.10 — **raw, the pre-fix web** | **0.089703** | −168,670.72 | 139,331.92 / 239,331.92 |
| 0.0811618 (kicked) | 0.1013889 — **kicked, DOS truth** | **0.090493** | **−159,554.53** | 143,608.51 / 243,608.51 |

`0.089703` is exactly the web's reported 8.9703%; `0.090493` and `−159,554.53` are
exactly the DOS app's 9.0493 and −159,554.53. Both halves of the report are
explained digit-for-digit, and the match also pins down the settings the screenshot
was taken under (365/360 + Exact + Prepaid + Arrears, `plus_regular` **off**).

**The engine was not at fault.** Driven directly, the Go engine matched the oracle
across all 24 basis × exact × prepaid × plus_regular combinations. The defect was
purely in the request → `LoanInput` conversion at the handler boundary.

**Fix (`internal/api/handlers.go` ~1110).** `row.LoanRate = amzKickerRate(*a.Rate, basis)`.
`req.Rate` and `req.Adjustments[i].Rate` are the only rate-bearing request fields, so
the audit is closed on the in-bound direction. On the out-bound direction there is
nothing to un-kick today: `AmortizationResponse` carries `Balloons`, `SolvedPrepay`
and `PrepayResolvedNN` but has **no adjustment echo field**, so a *solved*
adjustment rate (DOS `EstimateAndRefineAdjRate`, the AO6 shape) never reaches the
UI. If such an echo is ever added it MUST go through `amzUnkickerRate` — noted here
so the pairing is not lost.

**Verified.** New `TestAmortAdjustmentRate365360KickerMatchesDOS`
(`internal/api/amort_adj_rate_365360_kicker_test.go`) drives the client's exact
request through `HandleAmortizationCalc` and asserts APR `0.090493`, with a second
arm asserting the 360 basis is untouched (0.088597) so an over-broad kicker is
caught. Full `go test ./...` green, and green again with
`PERSENSE_REQUIRE_ORACLE=1` across `./internal/...` — no oracle-differential
regressions.

### Finding B — MISSING DISPLAY FEATURE. DOS's `TackOnFinalBalloon` row is never surfaced.

The DOS balloon grid row the client sees is not a computational input; it is a
**report artifact** DOS deliberately excludes from its own arithmetic.
`Amortize.pas` `TackOnFinalBalloon` (1040-1088) fires whenever the computation is
over-specified — its caller gate (1386-1394) requires `fancy`, amount and loan rate
known, and either no adjustments with a known payment or `adj_fully_specified`
(our case sets BOTH a new rate and a new amount, so it qualifies), plus
`unkballoon = 0` and `nballoons < maxlines` (the `-dSCROLLS` arm). It then:

1. appends a balloon row at `very_last` with `datestatus := outp`,
2. runs `EstimateAndRefineBalloon` to compute its amount, and
3. **`dec(nballoons)`** — with the source comment *"This says, don't really use this
   last balloon in generating a table. Table should truncate when balance goes
   negative..."*

So the row stays visible on the balloon block (`nlines`) with
`datestatus`/`amountstatus = outp`, while being excluded from the table walk **and**
from the APR. (The APR's own terminal residual is a separate mechanism —
`AMORTOP.pas:1223-1226`, `{$ifndef BOFA}`, where `RepayFancyLoan` in `value_calc`
mode discounts the leftover `NextPayment.principal` into `aprvalue`.)

The port has no equivalent: `AmortResult.Balloons` came back empty in all 24 sweep
combinations. Surfacing it is a UI + echo change (`ResolvedBalloon`/`BalloonEcho`
plus the `index.html` grid) and **must keep the row out of both the table and the
APR** to stay faithful. Left unimplemented pending a decision — flagged, not fixed.

**Tooling added.** `legacy/oracle/amort_oracle.pas` gained a `bdump` query token
that prints the balloon GRID as the DOS screen would show it after `MakeTable`
(`nballoons`, `nlines[AMZBalloonBlock]`, and every row with a non-empty
date/amount status, plus `lastdate`/`nperiods`). Without it the `dec(nballoons)`
row is invisible to every other oracle query, which is why the balloon half of the
report first looked unreproducible.

---

## §45 — Moratorium + principal minimum: the two screenshots were different loans, but the C-A-9 target guard fired where DOS never checks at all (2026-07-24)

**Client report.** Two screenshots, web vs DOS, with *"this divergence appeared
when I entered a moratorium and principal minimum."* Web: APR **7.0967%**, payoff
as of 06/01/2035 **$128,470.81**, Total Interest **$158,833.34**, Total Paid
**$258,833.34**. DOS: APR **7.2832**, payoff check 6/1/35 **115,719.71**, and a
`TackOnFinalBalloon` row 1/1/55 **−15,913.07** alongside the 6/15/31 33,333.00.

### Adjudication: both sides are faithful — the inputs differ

The two screens are not the same loan. The web shot carries **Pmt Amount
$1,200.00**; the DOS shot carries **Payment 750.00**. Everything else that
differs is immaterial (adjustment #2 is dated 06/05/2035 on the web and 6/ 1/35
in DOS; isolating that alone moves the APR by less than 1e-6).

Headless-oracle runs on the exact advanced-option stack settle it. Note the rate
argument: the oracle bypasses the app's cell layer, so the DISPLAYED 8.0050% must
be pre-multiplied by the 365/360 kicker (§28) — 8.0050 × 365/360 = **8.11618%** —
and both adjustment rates likewise (§44). Feeding the raw 8.0050 instead yields
0.070544 / 0.072377, which matches neither screen; this is worth recording
because it is an easy way to mis-diagnose a non-divergence as a divergence.

```
amort_oracle 100000 0.08116180555555555 360 12 \
  b365_360 exact prepaid loandmy=1.1.2025 firstdmy=1.2.2025 pts=0 \
  payhard=<PAY> predmy=1.3.2028:125:24:150 predmy=1.6.2040:209:52:75 \
  bdate=15.6.2031:33333.00 skip=1,3,5 mor=65 targ=300 \
  adjdmy=1.1.2030:0.10138888888888889:1000 \
  adjdmy=5.6.2035:0.030416666666666665:750
```

| | oracle APR | payoff 6/1/2035 | tack-on balloon 1/1/2055 | interest / paid |
|---|---|---|---|---|
| `payhard=1200` (**web shot**) | **0.070967** | **128,470.8134** | +4,662.15 | 158,833.34 / 258,833.34 |
| `payhard=750` (**DOS shot**) | **0.072832** | **115,719.7128** | **−15,913.07** | 141,902.46 / 241,902.46 |

Every figure matches its own screenshot to the digit, on both sides. The port
reproduces the 1200 column exactly (`TestAmortFullAdvancedMoratoriumTargetMatchesDOS`).
As a bonus this also proves the §44 kicker fix is live in the client's build: a
pre-§44 binary would have printed 7.0398% where the screenshot shows 7.0967%.

The moratorium and the principal minimum are genuinely load-bearing here — they
are not vacuously matching:

| | oracle APR | interest | paid |
|---|---|---|---|
| neither | 0.076886 | 106,619.33 | 206,619.33 |
| target only | 0.076466 | 113,600.61 | 213,600.61 |
| moratorium only | 0.066486 | 176,526.75 | 276,526.75 |
| both | 0.070967 | 158,833.34 | 258,833.34 |

### The real bug this investigation turned up: C-A-9 fires where DOS never checks

Walking that cube through the API is what exposed it. The **`target only`** arm —
principal minimum set, no moratorium — was **rejected outright** by
`internal/finance/amortization/validate.go` with

> The principal-reduction Target is too high to be reachable — it exceeds Amount
> Borrowed divided by the number of repaying periods.

because 100,000 / 360 = 277.78 < the $300 minimum, while the DOS engine computes
the same input happily (`apr 0.076466`). Confirmed independently on a plain loan
with no advanced options at all: `200000 0.08 360 12 payhard=1600 targ=600` →
`apr 0.079996`, where amount/NPeriods = 555.56 < 600.

**Root cause.** `Amortize.pas:1299-1317` — the guard is not unconditional, and it
is not anchored on `NPeriods`:

```pascal
if (h^.lastok) and (DateComp(mor^.first_repay, h^.firstdate) <> 0) then
  begin
    save_last := h^.lastdate;
    nrepay := NumberOfInstallments(mor^.first_repay, h^.lastdate,
                                   h^.peryr, on_or_before);
    h^.lastdate := save_last;
    if (nrepay <= 0) then
      MessageBox('Principal repayment must begin before the last payment date.');
    if (h^.amount / nrepay < targ^.target) then
      MessageBox('Your principal reduction target is too high.');
  end
else
  nrepay := h^.nperiods;      { <-- no validation whatsoever on this path }
```

and `mor^.first_repay` arriving at that test has already been normalized by
`Amortize.pas:1260-1288`: when a moratorium **is** set it is SNAPPED onto the
payment grid `on_or_after` (the var-result date of `NumberOfInstallments`); when
none is set it is **DEFAULTED** to `balloon[1]^.date` if a balloon precedes
`firstdate`, else to `h^.firstdate` itself.

So on a plain loan `DateComp(first_repay, firstdate) = 0`, DOS skips the whole
arm, takes `nrepay := h^.nperiods`, and **never validates the target at all.**
The check exists only to protect the moratorium path's `amount / nrepay` divide.

Three separate divergences in one block, all now fixed:

1. **The guard ran unconditionally.** Now gated on a moratorium (or a
   before-firstDate balloon) actually shifting `first_repay` off `firstDate`,
   plus `lastok` — DOS's exact condition.
2. **The denominator was wrong.** The port used
   `NPeriods − round(YearsDif(FirstRepay, FirstDate, Basis360) × PerYr)`; DOS uses
   `NumberOfInstallments(first_repay, lastdate, peryr, on_or_before)`, anchored on
   **lastdate**. These agree on clean monthly cases but are not the same function.
   `dateutil.NumberOfInstallments` (dateutil.go:734) is now used directly.
3. **DOS's sibling `nrepay <= 0` arm had no Go equivalent** — *"Principal
   repayment must begin before the last payment date."* It is independent of the
   Target (it guards the divide) and is now restored as C-A-8.

**Blast radius.** Two existing tests encoded the old, non-DOS behaviour and were
rewritten against the DOS source and the oracle: `TestValidateTargetTooHigh`
(firstpass_test.go) now carries a moratorium, since without one the rejection it
asserts is simply not something DOS does; and `TestMoratoriumTargetDenominator`
now asserts the plain-loan case is **accepted**, plus both moratorium arms
(reachable at 600 vs amount/nrepay = 666.67, unreachable at 800).

**A test-harness bug the fix uncovered.** With the guard no longer rejecting them,
`TestDOSFancyCombinationSweep`'s `adjust+target` cases began running for the first
time, and one surfaced a ~$0.09 balance gap on the second-to-last row of a
$474,085 / 80-period loan (DOS 392.71 vs Go 392.62) — with every interest figure
matching to the cent, which is the signature of a rate epsilon rather than a logic
divergence. It was: the `adjustRate` builder formatted the new rate to six decimals
for the oracle token while handing Go the unrounded float, so the two engines
amortized slightly different loans and the gap compounded over the
post-adjustment tail. Feeding both sides the same round-tripped value closes it to
under two cents, and also tightened `balloon+adjust` (7.93e-6 → 6.42e-6 max
relative) and `adjust+skip`. The `target` builder alongside it already did this.

**Not changed (noted for the record).** DOS's `if (nballoons > 0) and
(mor^.first_repaystatus >= defp) and (balloon[1]^.date < mor^.first_repay)` guard
(Amortize.pas:1239) rejects a balloon before first-repay **only when a moratorium
is set**, and the `first_repay := balloon[1]^.date` default at 1272-1273 exists
precisely because DOS otherwise tolerates a balloon dated before `firstdate`. The
port's C-A-4 rejects that unconditionally, which makes the balloon arm of the new
code unreachable. It is transcribed anyway for structural fidelity. Whether C-A-4
itself should be relaxed is a separate question and is **not** part of this fix.

**Guards added.** `internal/api/amort_target_no_moratorium_test.go` pins the whole
2×2 cube against the oracle values above (and asserts the four APRs stay distinct,
so a future change that silently drops either option cannot pass);
`TestMoratoriumRepayAfterLastDate` pins the restored C-A-8 message. Full suite
green with and without `PERSENSE_REQUIRE_ORACLE=1`.

---

## §46 — DOS's terminating balloon (`TackOnFinalBalloon`) is now surfaced end-to-end, and the dateless-option very-last fold that hid behind it (2026-07-24)

**Client request.** Following §45 — *"yes lets surface it to make it more dos
faithful."* §45 established that the client's DOS screenshot and web screenshot
were different loans, but it also left one real presentation divergence standing:
DOS's Balloon Payments grid showed a second row, `1/1/55  −15,913.07`, that the
web grid did not show at all. That row is DOS's `TackOnFinalBalloon`, and this
section makes the port paint it.

### What DOS actually does

`TackOnFinalBalloon` (`Amortize.pas:1040-1088`) runs from `Amortize` at
`:1386-1394`, immediately before `MakeTable`, and only when **all** of:

- the Advanced-Options toggle (`fancy`) is on — DOS's `fancy` here is the UI
  toggle alone, not any internal routing flag;
- the payment is a genuine user value (`PayAmtStatus >= defp` — DEFAULT or
  INPUT; a *solved* payment (`outp`) is excluded by construction, because a
  solved payment amortizes exactly and has no residual);
- the schedule is over-specified, i.e. it does not land on zero.

It resolves the terminal date from `DetermineVeryLast` (`AMORTOP.pas:1293-1304`
— the latest of the last regular payment, every balloon date, and every
prepayment stop date), computes the balance owing on that date, optionally
subtracts `VeryLastRegularAmount` (`AMORTOP.pas:1306-1320`) when `plus_regular`
is set, and writes the result into the balloon array as an **output** cell
(`dstatus`/`astatus` = `outp`).

Then comes the part that matters:

```pascal
  dec(nballoons);
```

The row is **de-activated**. `BalloonValues2Grid`
(`AmortizationScreenUnit.pas:1691-1713`) walks the *raw* array and ignores
`nballoons`, so the row still paints into the grid. `MakeTable` and the APR both
stop at `nballoons` and never see it. DOS therefore **displays the figure and
does not pay it**.

Two arms leave the row live instead (`merge_w_existing`, and the sub-`minpmt`
arm): there DOS keeps it inside `nballoons`, so it *does* take part in the table
and the APR, and on the merge arm DOS additionally warns
`"Please note that the amount of your terminating balloon has been ajusted."`
(spelling DOS's).

### What the port now does

`internal/finance/amortization/tackon.go` ports the procedure verbatim, with
`determineVeryLast` / `veryLastRegularAmount` extracted so the schedule walk and
the tack-on resolve the terminal date from the *identical* rule. Both engines
call it at DOS's ordering — immediately before the table is generated:

- **`AmortizeDOS` path** (`engine.go:305-345`): live rows are spliced into the
  balloon set the walk sees; a de-activated row is appended to `res.Balloons`
  only, after the schedule is built.
- **piecewise path** (`engine.go:1065-1185`): same shape. The gate keys on
  `uiFancy` — the caller's Advanced-Options toggle captured *before* the port's
  internal `Fancy` forcings (exact-daily / US-Rule), because those forcings can
  make an ordinary loan look fancy and DOS would never tack a balloon onto one.

The API echoes it as `BalloonEcho.tackedOn` (`handlers.go:366-374, 1358`), and
the front end paints it as a read-only output row.

### The front end: a separate `<tbody>`, deliberately

The display row lives in `#amz-balloon-tack-body`, a **second `<tbody>` inside
the same table**, opened after `#amz-balloon-body` closes. It renders as the next
grid row visually while being invisible to every selector keyed on the input
body — there are twelve of them: the request collector, row counting, `Add Row`,
undo snapshots, `resizeDynamicBody`, import, Clear All, the `input` clear sweeps
and the event delegation.

That isolation is not tidiness, it is correctness. The request collector
deliberately reads a green (`cell-output`) Amount cell as **blank**. A tack row
sitting inside `#amz-balloon-body` would therefore be re-submitted on the next
recalculation as a user **date-only "target balloon"** — and the engine would
solve a balloon at the terminal date, silently changing the answer every time the
user pressed Calculate. Rendering plain `div.grid-cell.cell-output` cells rather
than `<input>`s closes the same hole from the other side.

The row carries `data-amz-balloon-tack-row` (never `data-amz-balloon-row` /
`data-amz-balloon-field`) and a tooltip that states in words what DOS conveys by
convention: the figure is computed, not entered, and is excluded from the payment
schedule and the APR. It is cleared on error, on Clear All, on import, and is not
restored by `restoreState` — it is a computed result, not saved state.

**Known placement nuance (accepted).** DOS writes the row at array index
`nballoons+1`, so it sits flush beneath the last filled balloon. The web grid
keeps fixed blank input rows, so the row renders *below* those blanks instead.
Moving it flush would require putting it back inside `#amz-balloon-body` and
patching `addClonedRow`, `snapshotForUndo`/`resizeDynamicBody` and
`populateAdvRows` — i.e. re-opening the round-trip hole above. Not done.

### The trap: the tack amount equals the final payment, and that is correct

On the client's web-screenshot loan the tacked-on row is **4,662.15** — and the
final *scheduled* row also pays **4,662.15**. That is not a double count. Both
numbers are the balance owing **on** the terminal date (principal plus that
period's interest): the display fold retires it in the last row, and
`TackOnFinalBalloon` reports the same figure as the de-activated grid row. DOS
shows it twice and counts it once.

The consequence for testing is important: **row-value matching is not a valid
leak detector.** The totals and the APR are. An oracle `rows` tail confirms it:

```
row 12/ int 13.65 prin  736.35 bal 4649.97
row  1/ int 12.18 prin 4649.97 bal 0.0000     ← 12.18 + 4649.97 = 4662.15
```

### The bug this surfaced: the dateless-option very-last fold

Making the row visible exposed a genuine arithmetic hole in the piecewise engine.
DOS's display very-last fold (`PrintAndReset`, `AMORTOP.pas:~1004`) retires
**any** residual into the last row. The port implemented that rule in four
branches — plain, ARM, balloon and prepayment — and every one of them keys on a
**dated** option being present. `plainFancy` excludes the case by construction
(it requires *no* advanced option).

So a loan whose only advanced options are **dateless** — skip months, a
moratorium, a principal minimum, in any combination, with no balloon, prepayment
or adjustment — fell through every branch. Routed to the piecewise engine (which
happens under the exact method; a non-exact loan goes to `AmortizeDOS`, which
folds correctly), its table ended with the residual still outstanding and its
totals were short by **exactly that residual**.

Verified against the real DOS engine:

```
amort_oracle 100000 0.08 360 12 b365_360 exact prepaid pts=0 \
  loandmy=1.1.2025 firstdmy=1.2.2025 payhard=800.00 skip=1,3,5
  → interest 333436.00  paid 433436.00
    Go before: paid 216000.00, final row pay 0.00, bal 217436.00 left standing

… same loan skip=2,3,5 → paid 440966.64   (Go 216000.00)
amort_oracle … 120 12 … payhard=700.00 targ=50.00
  → interest  78531.31  paid 178531.31    (Go  85327.72)
```

Note the first case: DOS folds even when the terminal month is **itself
skipped** (skip=1,3,5 ends on a January). The skip zeroes the *regular* payment;
it does not suppress the terminating payoff. Interest is unchanged either way —
it has already accrued on those balances — only the final payment and the closing
balance move. Fixed at `engine.go:2908-2939` with the residual-guarded fold the
other four branches already carry.

### Adjudication of the §45 screenshots, now that both columns are pinned

Both the client's photographed loans, on the densest advanced-option stack we
have (two prepayment series, a mid-term balloon, two rate/payment adjustments, a
moratorium, a principal minimum and skip months, 365/360 + Exact,
`balloonIncludesRegular:true`):

| | web shot (Pmt 1,200.00) | DOS shot (Payment 750.00) |
|---|---|---|
| tacked-on row @ 1/1/2055 | **4,662.15** | **−15,913.07** |
| APR | 7.0967% | 7.2832% |
| payoff 06/01/2035 | 128,470.8134 | 115,719.7128 |
| total interest | 158,833.34 | 141,902.46 |
| total paid | 258,833.34 | 241,902.46 |
| final scheduled row | 4,662.15 (int 12.18) | 548.20 (int 1.39) |

Every figure is the oracle's. The **−15,913.07** is the exact number on the
client's DOS photograph; the web screen now shows it too. The pay=750 residual is
negative — the schedule *overpays*, so DOS's terminating row is money the loan has
run past rather than money still owed, and its table retires early (599 rows)
rather than on the terminal date.

### Guards added

- `internal/finance/amortization/dos_tackon_test.go` —
  `TestTackOnFinalBalloonMatchesDOSGrid` (the `bdump` grid, row for row),
  `TestTackOnFinalBalloonExcludedFromTableAndAPR` (the leak detector: totals
  333,436.00 / 433,436.00 *with* the row surfaced), `TestTackOnFinalBalloonGateClosed`
  (a solved payment and a non-fancy loan get no row).
- `internal/api/amort_tackon_screenshot_test.go` — both screenshot columns
  end-to-end through the handler: two echoes, `Balloons[1].tackedOn`, the tack
  amount, no schedule row past the terminal date, the DOS final-row payment and
  zero closing balance, and the tack-**free** APR / payoff / totals.
- `cmd/persense/frontend_tack_balloon_test.go` — the display half:
  `TestTackBalloonRowIsolatedFromInputGrid` pins the structural isolation (the
  separate tbody, the collector's attribute selector, no input elements, no
  input-grid data attributes, and that the renderer is actually called);
  `TestTackBalloonRenderJS` runs the **shipped** renderer under Node over five
  scenarios (negative residual, positive residual, ordinary balloons only,
  after clear, undefined argument).
- Browser-verified end-to-end: the client's exact web-screenshot loan renders
  APR 7.0967% and a grid row `01/01/2055 | $4,662.15` in italic green.

Full suite green (`PERSENSE_REQUIRE_ORACLE=1 go test ./...`), `go vet` clean.

**Still open (unchanged by this work).** Moratorium + exact interest diverges by
~308.60 on the `mor_exact` case (Go interest 96,924.39 vs DOS 97,232.99). Not
caused by §46 and not fixed here.

---

## §47 — DOS's Julian day ceiling (70000): a perpetual weekly/biweekly PV stream TRUNCATES, a real To date past it REFUSES (2026-07-31)

### The rule

`VIDEODAT.pas:373`, the first line of DOS's `MDY`:

```pascal
if (daynumber<0) or (daynumber>70000) then begin x.m:=errorbyte; exit; end;
```

Day 70000 is **26 August 2091**. Every DOS date step that goes through Julian
arithmetic — and that is exactly the weekly/biweekly arms of `AddPeriod`
(`INTSUTIL.pas:1213`), `AddNPeriods`, and `NumberOfInstallments`'
`ChoosePaymentDate` (`:960`) — dies there. The monthly/quarterly/semi-monthly
arms step the **month field** instead and are completely unaffected.

The failure is a **poisoned record, not an abort**: `exit` leaves `x.y` and `x.d`
alone and sets only `x.m := errorbyte` (−99). The record stays readable, fails
`dateok`, and `DateComp` orders it **after every real date**
(`INTSUTIL.pas:829-830, 836-845`, "Blank or unknown dates are later than
everything").

### The port had it backwards

`dateutil.MDY` carried this comment and this constant:

```go
// Original Pascal limit was 70000, but with base 1900 (y up to 249 = year 2149),
// Julian values can reach ~91000. We use 100000 for safety.
if daynumber < 0 || daynumber > 100000 {
```

The reasoning inverts the fact. DOS's own perpetual sentinel `1/12/2149` is
Julian **91282** — *outside* the range its own `MDY` accepts. That is not an
oversight to paper over; it is the mechanism by which a DOS weekly/biweekly
perpetuity stops. Raising the ceiling did not fix a limitation, it deleted a
behaviour.

### Two consequences, and DOS is asymmetric between them

| Case | DOS | Why |
|---|---|---|
| To = the 1/12/2149 forever sentinel | **truncates**, keeps the sum | `NumberOfInstallments` short-circuits on the sentinel year *before* `ChoosePaymentDate` (`INTSUTIL.pas:1026`), so nothing refuses; the per-payment walk instead runs until `AddPeriod`'s `MDY` poisons the cursor, and the loop's own `DateComp` then reports it past the terminal and exits |
| To = a real date past the ceiling | **refuses** | `ChoosePaymentDate`'s 26/52 arm calls `MDY` directly and hands the poisoned daterec back through the VAR parameter; the next `Julian()` fires `EMessage('Bad date passed to Julian function: m=',-99)` and the screen dies |

Measured against the real oracle (`pv_oracle table`, rate 0.10, basis 360,
as-of 1/1/2028, from 1/6/2035, 1000.00/payment):

| Case | DOS | port BEFORE | port AFTER |
|---|---|---|---|
| forever biweekly, COLA 3% | 170805.731733 | 174057.625977 (+1.9%) | **170805.731733** |
| forever weekly, COLA 3%, 365/360, COLA-month 11, as-of 1/1/2045, from 1/1/2026 | 4806557.229011 | 5936375.874446 (+23.5%) | **4806557.229011** |
| forever biweekly, COLA 3%, 365/360, COLA-month 11 | 168651.578079 | 171610.000808 (+1.8%) | **168651.578079** |
| finite biweekly to 1/12/2090 (control) | 122248.284724 | 122248.284724 | 122248.284724 |
| forever **monthly**, COLA 3% (confinement) | 80278.265609 | 80278.265609 | 80278.265609 |
| to = 1/12/2091 | refuses | 122290.413102 | **refuses** |

### The fix

- `dateutil.MDY` restores the 70000 ceiling and returns `ErrJulianCeiling`
  (wrapping the offending day number). The zero `DateRec` it returns is the
  port's analogue of the poisoned record: `IsUnknown`, so `DateOK` is false and
  `DateComp` orders it last, exactly as DOS does.
- `NumberOfInstallmentsRawE` / `NumberOfInstallmentsE` are new variants that
  **return** that error; the existing `NumberOfInstallmentsRaw` /
  `NumberOfInstallments` still swallow it, which is right for the ~30 callers
  that only need a count and a date to *compare*.
- `presentvalue.FirstPass` (`backward.go`, the port of `PRESVALU.pas:605-608`)
  refuses on `ErrJulianCeiling`. `valueFullySpecifiedPeriodic` carries the same
  arm for paths that do not run `FirstPass`.
- Stop-and-keep `break`s at the four per-payment walks in `calc.go` and the
  table walk in `table.go`: on `ErrJulianCeiling` the loop ENDS and the
  accumulated sum stands. Aborting there would refuse perpetual rows that DOS
  values.

**The refusal boundary is the SNAPPED grid date, not the entered date.**
`NumberOfInstallments` snaps the To date backward onto the payment grid before
`MDY` sees it. For a 1/6/2035 biweekly anchor the last representable payment is
24 Aug 2091, so every To date up to **6 Sep 2091** snaps back onto it and
computes, and **7 Sep 2091** is the first that refuses. Both engines agree day
by day across Aug–Sep 2091. A guard keyed on the entered date would refuse
eleven days early — and *cannot work anyway*, because by the time any PV guard
runs the To date has already been replaced by the unknown date (four such guard
placements failed across 2026-07-30 rounds 2-4, all reading `Julian = -88`
instead of 70097).

### Why this survived so long: `refdata.pas` is not a second oracle

`legacy/reference-output/refdata.json` asserts that `MDY` round-trips Julian
73050 → 1/1/2100 and 90948 → 1/1/2149. It does not, in DOS. The generator
`legacy/testharness/refdata.pas` **hand-transcribes** `MDY` at line 82 and drops
the range guard — it does not call the DOS routine. Three port tests
(`TestJulianMDYFullRange`, `TestJulianLeapYears`, `TestCrossCheckJulian`) were
built on that transcription and actively required the port to do something the
original refuses; they are now aligned to the compiled DOS engine, with the
harness's omission recorded in place. `legacy/` is read-only, so `refdata.pas`
itself is unchanged — but it should not be treated as authority for date-range
behaviour.

### Guards

- `internal/finance/presentvalue/zzjulian_ceiling_test.go` — the five oracle-pinned
  values above (each re-verified against `pv_oracle` in-test when the binary is
  present), the truncate/refuse asymmetry, the snapped-grid boundary pair
  (6 Sep computes / 7 Sep refuses), the table-walk stream isolation, and the
  `dateutil` propagation contract.
- `internal/dateutil/extreme_test.go` — `TestJulianMDYCeiling` pins the constant
  in day numbers so it cannot drift silently.
- `dos_pv_fuzzer5_test.go` no longer forces a forever row down to
  `perYr <= 4`; that exclusion was what kept the sweep off this defect.

### Verification

Differential sweeps against the real `pv_oracle` over the affected region —
basis × COLA mode (ann/cnt/month) × peryr {12,26,52} × forever/finite/boundary
To dates × COLA × rate × as-of-before/after-from, single- and multi-row:
**4464 cases, 0 divergences** beyond two last-digit float64 ULP differences
(1e-6 on values of 939938 and 2299394, i.e. ~4e-13 relative, both on `peryr=12`
cases that never touch `MDY`).

---

## §48 — The COLA yield→continuous conversion used `log1p`, not DOS's `lnn(1+y)`; and FPC's `:0:6` is not a value (2026-07-31)

Two findings from the round-6 bit-level PV differential. The first is a real
port divergence; the second is a harness artifact that had twice been logged as
"unexplained noise" and is recorded here so it is not chased a third time.

### 1. The conversion (real divergence — FIXED)

**DOS stores `periodic.cola` already in CONTINUOUS form.** `PRESVALU.pas:281` is
a bare `exp_cola := exxp(cola)` — no conversion, so whatever reaches the engine
must already be a continuous rate. The PV screen re-renders that cell through
`PercentValueFromCell`'s `COLAcol` arm:

```pascal
      COLAcol:
        begin
          PercentValueFromCell := YieldFromRate(rp^, 1);      {INTSUTIL.pas:1601-1606}
```

and `YieldFromRate(rr,1) = 1*(exxp(rr/1)-1)` (`INTSUTIL.pas:1263-1268`). The
unique inverse of that display — i.e. what entry must do — is

```pascal
function RateFromYield(yy :real; n:byte):real;                {INTSUTIL.pas:1270-1275}
         begin nn:=RealPerYr(n); RateFromYield:=nn*lnn(1+yy/nn); end;
```

with **n = 1** (`RealPerYr(1) = 1`, `INTSUTIL.pas:1255-1261`) — i.e.
`cola := lnn(1 + yield)`, *not* `peryr*lnn(1 + yield/peryr)`.

The port converts at point of use rather than at cell entry, which is fine in
itself (the DOS TUI keystroke unit, `INPUT.pas`/`PEPANE.pas`, is not in the
surviving checkout — the n=1 conclusion is derived from the round-trip partner
above, not read off an input statement). What was wrong is the *arithmetic*: six
sites used `math.Log1p(yield)`. **`log1p(y)` is not `lnn(1+y)`** — log1p never
forms the intermediate `1+y`, so it does not incur that rounding — and the two
land on different doubles.

Sites changed, all now routed through the new
`internal/finance/presentvalue/colaconv.go: colaContinuous`, which calls
`interest.RateFromYield(cola, 1, yrDays)`:

`calc.go` (the perpetual-stream guard, the continuous-COLA closed form,
`periodicSumAnnualCOLA`, and the per-payment continuous arm), `variablerate.go`,
`table.go`. A COLA of exactly 0 short-circuits to 0 — DOS never converts an
absent COLA (`PRESVALU.pas:610-611`).

**Magnitude.** Sub-cent: the largest observed effect is ~85 ULP (about 2.4e-9
relative). It is invisible in every existing tolerance and moved **no** golden
value in the suite. It matters for the same reason `crmath.go` did (see
`claude/exp_log_correct_rounding_root_cause_2026-07-28.md`): this project's
fidelity claim is bit-level, and a systematic last-bits offset on a third of all
COLA inputs is a standing seed for basin divergence anywhere a solver later
iterates on these values.

**How it was localized.** A differential that compares the screen total as a raw
`float64` rather than as the oracle's 6dp text:

| sweep | cells | bit-divergences BEFORE | AFTER |
|---|---|---|---|
| basis × peryr × rate × cola × colamonth | 2880 | 1048 | **0** |
| + varied from/to/as-of, 29-Feb anchor, +lump row | 2160 | 1015 | **0** |

Divergences were **identically zero on every `cola == 0` cell** in both sweeps,
which is what pointed at the conversion rather than at the PV engine.

### 2. FPC's `:0:6` output is not the value (harness artifact — NOT a divergence)

While chasing the above, one case looked like a 1e-6 divergence:

```
pv_oracle table 0.125 365360 detail none ann asof=1.1.2028 \
          per=1.6.2030:1.6.2085:12:1000.00:0.02
  DOS prints 83347.209124      Go prints 83347.209123
```

Both engines hold **bit pattern `40F459335891E214`** — the exact same double,
83347.2091234999825…. FPC's `Str`/`Write` for a real converts to ~16 significant
digits and *then* rounds to the requested decimals, so it double-rounds: a value
whose expansion continues `…4999x` just past the cut is first pulled up to
`…5000` and then rounded half-up. Go's `strconv` is correctly rounded and is not.

Measured over 200,000 random doubles in the 1e2–1e6 band, FPC's `:0:6` disagrees
with Go's `%.6f` on **15 (0.0075%)**, every one of that shape.

Consequences to carry forward:

- **A 1-in-the-last-printed-place disagreement against an oracle's `:0:6` output
  is not evidence of an engine divergence.** The two ULP cases recorded at the
  end of §47 were both of this class; that paragraph should be read with this
  correction. Neither was a `peryr=12` arithmetic difference.
- The existing fuzzers are unaffected — their tolerances (`max(0.01, 1e-7·|x|)`
  for totals, `0.0051 + 1e-9·|x|` for table lines) absorb it — but they also
  cannot see past it, which is why the conversion defect above survived every
  sweep to date. **A formatted-decimal differential cannot certify bit
  fidelity.** Where that claim is wanted, compare raw float64s: build the oracle
  with its `Writeln(…:0:6)` extended to also emit `hexstr(qword,16)` of the same
  variable.

### Guards

`internal/finance/presentvalue/zzcola_yieldconv_test.go`:

- `TestCOLAContinuousIsDOSRateFromYield` — pins the conversion to
  `RateFromYield(y, 1, yrDays)` bit-for-bit, pins `cola == 0 → 0`, and carries a
  witness asserting the result is still observably *different* from
  `math.Log1p`, so the guard cannot silently stop testing anything.
- `TestPVCOLAScreenTotalMatchesDOSBits` — eight oracle-sourced goldens pinned as
  raw float64 bit patterns (provenance: the `pv_oracle table …` command and the
  precision-extended `hexstr` output are quoted in the test). Five of the eight
  fail with the fix reverted, by 1 to 85 ULP.
- `TestFPCSixDecimalRenderingIsNotAValueDifference` — records finding 2 with the
  real instance, so the next differential run recognizes the class immediately.

### Verification

Full gated suite `PERSENSE_REQUIRE_ORACLE=1 go test ./... -count=1` **EXIT=0**,
12/12 packages, **zero golden churn**. Post-fix differential sweeps vs the real
oracles: PV fuzzer5 seeds 20611/20612/20613 at N=4000 (5,995 table worksheets,
2.00M lines diffed, 5,972 variable-rate worksheets, 0 divergences); mortgage
fuzzer5 seeds 20614/20615 at N=3000 (0 divergences, 0 ulp-noise); the full
`PERSENSE_FUZZ=1` amortization opt-in fuzz suite green. Both directions
confirmed per CLAUDE.md.

---

## §49 — Bit-level differential: the oracles now emit raw float64s, and the 365/360 kicker was bypassing the DOS primitives (2026-07-31)

§48 established that a differential built on the oracle's printed decimals
cannot certify bit fidelity, and that a real defect had been hiding under that
resolution for months. This section makes the bit-level comparison a standing
capability rather than a throwaway script, and records what it found when
pointed at the mortgage and amortization engines.

### 1. Raw-bits emission in the three oracle drivers

`legacy/oracle/OracleBits.pas` (new) lets a driver emit the raw 64-bit pattern
of any value it prints. Wired into `pv_oracle` (screen total, per-row values,
solved rate), `mtg_oracle` (monthly / price / cash / financed, apr, howmuch,
crossover) and `amort_oracle` (payment, solved amount / rate, payoff).

**Emission is OFF unless `PERSENSE_ORACLE_RAWBITS` is set, and with it unset
every driver's stdout is byte-identical to before.** That is not
belt-and-braces; it is required. Roughly sixty Go exec sites parse these
binaries and none of them share a parser: several use exact field counts, five
mortgage parsers walk the whole flattened output in stride-2 key/value pairs,
three `dateutil`/`interest` parsers require the ENTIRE stdout to be one number,
and the Playwright harness `testplan/harness/run_mtg.js` takes the LAST line of
stdout. An unconditional extra token or line would break some of them.
Verified: 22 invocations spanning every mode of all three oracles (including
`intutil`), pre-change vs post-change binaries, **0 byte differences**.

When enabled the format is `RAWBITS name=<16 hex>|name=<16 hex>` — exactly two
whitespace tokens so the stride-2 pair walkers keep their parity, `=`/`|`
separators so no token can collide with a scanned key name, no leading `ERR`,
never emitted in `intutil` mode.

Three new `intutil` sub-functions expose the conversion primitives directly:
`rfybits YIELD N`, `yfrbits RATE N`, `kickbits RATE N SCALE`. New names, so no
existing `intutil` caller is affected.

**`amort_oracle` deliberately does NOT emit bits for `interest` / `paid`.**
Those are not engine doubles — the driver re-parses them out of the already
formatted 2-decimal totals line (`NumAfter(totalsLine, …)`), so their low bits
carry no engine information and comparing them would manufacture divergences.
They stay covered as decimals by the existing sweeps.

### 2. What the bit differential found

| engine | surface | cells | result |
|---|---|---|---|
| Present value | screen total | 1920 | **0 divergences** (868 with §48 reverted, worst 966 ULP) |
| Mortgage | monthly, price, cash, financed | 300 × 4 | **0 divergences** |
| Amortization | solved payment | 400 | **0 divergences** |
| `interest` | RateFromYield / YieldFromRate | 252 | **0 divergences** |

The mortgage and amortization engines needed no change. A source audit for the
§48 defect class — a caller bypassing `interest.Exxp`/`Lnn`/`Power`/`Sqrrt`/
`RateFromYield`/`YieldFromRate` — found **zero** live `math.Exp`/`Log`/`Log1p`/
`Pow` calls in either engine; every transcendental already routes through the
DOS primitive, and every `n` argument matches the DOS line it ports
(`Amortize.pas:387,448,674,730`, `AMORTOP.pas:1265-1266,1562-1565`,
`Mortgage.pas:140,382,515,532`). The two bit tests are the empirical
confirmation of that audit.

### 3. The one real find: the 365/360 kicker at the API boundary (FIXED)

`internal/api/handlers.go: pvKickerRate` / `pvUnkickerRate` implement DOS's
`PercentValueFromCell` vratecol `x365_360` arm, INTSUTIL.pas:1611-1614:

```pascal
      vratecol:
        begin
          if (df.c.basis=x365_360) then
            begin
              n:=df.c.peryr; {YieldFromRate is smart about n=canadian, etc.}
              if (df.c.peryr and CANADIANorDAILY =0) and (g[i]^.peryr > 0) then
                n := g[i]^.peryr;
              PercentValueFromCell:= RateFromYield(YieldFromRate(rp^,n)/kicker,n)
```

Their own doc comment named `RateFromYield(YieldFromRate(...))` — and then spelled
it out by hand as `n*(math.Exp(r/n)-1)` and `n*math.Log(1+y*k/n)`. Same defect
class as §48, with three consequences rather than one:

- `math.Exp` is not `interest.Exxp`. Beyond rounding (`math.Exp` misrounds
  13.67% of arguments where FPC's is effectively correctly rounded — see
  `interest/crmath.go`), `Exxp` carries DOS's `|x| < 1e-4` Taylor branch and its
  ±70 guards. **The bare call was taking a different branch, not just a
  different last bit.**
- `math.Log(1+y/n)` is not `interest.Lnn(1+y/n)` — the milder cousin of §48's
  bug: the `1+` intermediate *is* formed here, so only the log is wrong.
- n-argument: see the scoped gap below.

Both helpers now call the shared functions. Measured against
`amort_oracle intutil kickbits`: **100 comparisons, 0 divergences after; up to
706 ULP before**, and the worst cases were at small rates (0.001 at peryr 12 →
x = 8.3e-5) — i.e. precisely inside the Taylor branch the bare `math.Exp`
skipped, confirming this was a missing branch rather than noise.

On error the helper returns NaN, which is exactly what the hand-inlined
`math.Log` produced for the same inputs. The only reachable failure is a rate
negative enough to drive `1 + y·scale/n` to zero or below, where DOS's `lnn`
refuses. Preserving NaN keeps this a pure arithmetic-fidelity change; making the
boundary refuse the way DOS does is a separate decision.

### 4. Scoped gaps left open, with citations

- **Per-row `n` in the kicker.** DOS overrides `n := df.c.peryr` with the rate
  row's own `g[i]^.peryr` when the settings frequency carries neither the
  canadian nor the daily bit (INTSUTIL.pas:1612-1613). The port always passes
  the settings frequency because `presentvalue.RateLine` has no `PerYr` field at
  all — the override is *structurally unreachable*, not merely unimplemented.
  Closing it means adding that field through the API DTO and the engine type.
- **Amortization APR frequency.** `Amortize.pas:597-601` picks
  `py := df.c.peryr` when the settings frequency has the canadian/daily bit and
  `h^.peryr` otherwise; `backward.go:921` always takes the `h^.peryr` leg.
  Unreachable through the REST surface, where `handlers.go:891,903` populate both
  from the same `req.PerYr`. It diverges only for direct library callers.
- **`ReportedRate` on the amortization rate echo.** `INTSUTIL.pas:1652-1656`
  applies `ReportedRate` before the kicker division; `amzUnkickerRate` does the
  division but not `ReportedRate`. `interest.ReportedRate` is the identity unless
  the settings frequency has the canadian/daily bit, so this has the same
  reachability caveat.

All three are one-conditional fixes whose only effect is on canadian/daily
compounding or on per-row variable-rate frequencies, neither of which the
current front door can produce. They are listed here so the next audit does not
have to re-derive them.

### 5. The harness rule this produced

**Quantize every oracle argument to the exact double the oracle will parse.**
Arguments cross the process boundary as text; if Go keeps a full-precision float
while the oracle parses a rounded rendering, the two engines start from
different inputs and the differential measures quantization, not fidelity.
Measured: an unquantized rate at 6dp made **400 of 400** amortization payments
compare unequal, worst ~4.8e10 ULP (~1e-5 relative); quantizing made all 400
bit-identical with no engine change. This is also why the older decimal sweeps
need tolerances near 1e-4 — they are absorbing this, not engine error.

### Guards

- `internal/finance/presentvalue/zzbits_fidelity_test.go` — 1920 cells; the
  grid keeps `cola == 0` cells deliberately, because divergences being
  identically zero there is what localized §48.
- `internal/finance/mortgage/zzbits_fidelity_test.go` — 300 cases × 4 outputs.
- `internal/finance/amortization/zzbits_fidelity_test.go` — 400 solved payments.
- `internal/finance/interest/zzconv_bits_test.go` — 252 conversions, with values
  straddling the `|x| < 1e-4` Taylor branch.
- `internal/api/zzkicker_bits_test.go` — 100 kicker conversions plus the
  identity checks on the 360/365 bases and a round-trip guard.

### Note on `legacy/`

CLAUDE.md said "do not modify any files under `legacy/`". That rule exists for
`legacy/src/**`, which is the untouchable original (and whose `*.pas` additionally
hold CP437 bytes that transfer can corrupt). `legacy/oracle/**` is
project-authored harness code — the headless drivers, the FPC stubs and
`build_linux.sh` were all written for this port. This work modifies the drivers
only; `legacy/src/**` is unchanged. CLAUDE.md has been amended to state that
distinction explicitly.

---

## §50 — DOS's adjustment pre-pass leaks a `var` snap into `h^.lastdate`; the structural port had no pre-pass at all (2026-07-31)

The first root cause found under the new source-audit rule
(`docs/testing_policy.md` §7b), and a clean example of why that rule exists: the
mechanism is **invisible from any input/output pair**, because both engines run
the same formula on the same balance at the same rate.

### Symptom

```
amort_oracle 100000 0.08 144 12 loandmy=30.6.2023 firstdmy=30.8.2023 \
             adj=67:0.04: adj=77:0.11:
  DOS total interest 60857.90   |   Go 60504.78     (Δ 353.12)
```

Row diff (both rounded to cents — DOS's `rows` mode re-parses its own 2-decimal
table, so comparing raw 4dp output is meaningless):

```
  row 67  date 2/28/29
    DOS  int 220.35  prin 732.81  bal 65372.21
    GO   int 220.35  prin 743.50  bal 65361.52
```

**Interest identical to the cent, principal off by 10.69** — same rate, same
opening balance, different PAYMENT: DOS 953.16, Go 963.85.

### Root cause

DOS runs a pre-pass before the display walk, `Amortize.pas:1408-1419`:

```pascal
    for i := 1 to nadj do
      if (adj[i]^.loanratestatus >= defp) and (adj[i]^.amountstatus < defp) then
        begin
          if (not EstimateAndRefineAdjPayment(i)) then
            exit;
          d := h^.payamt;
        end
```

Every payment it solves is thrown away: `EstimateAndRefineAdjPayment`
(`Amortize.pas:324-345`) restores the rate and the balloon state on the way out,
and `Re_Amortize` sets `amountstatus := outp` **without** setting `amtok`
(`AMORTOP.pas:1591-1592`), so the display walk recomputes every adjustment amount
from scratch regardless.

It is not a no-op because of the one thing it does **not** restore —
`h^.lastdate`. `Re_Amortize`'s payment arm is `AMORTOP.pas:1547`:

```pascal
        n := NumberOfInstallments(adj[next_adj]^.date, h^.lastdate, h^.peryr, on_or_after);
```

`l` is a **VAR parameter**, and `NumberOfInstallments` snaps it onto the payment
grid in place (`INTSUTIL.pas:936-941`). There is no save/restore guard here —
contrast `Amortize.pas:1301-1304`, which brackets its own call with
`save_last`/restore, and `saved_balloon_state` (`AMORTOP.pas:132-146`), which
saves `next_balloon, npre, next_adj, d, pre[]` but not `lastdate`.

For a month-end adjustment date the snap fires twice over:
`if (flast) then l.d := daysinm(l)` (`INTSUTIL.pas:1018`) pushes the day out to
the terminal month's length, and then `ddiff > 0` advances the month
(`INTSUTIL.pas:1003`).

So on this case the pre-pass moves `h^.lastdate` 30.7.2035 → **31.7.2035** (via
adjustment 2, which lands 30 Nov, a month end). The display walk then re-solves
adjustment **1** against that snapped date and counts `n = 80`, `pred(n) = 79`.
A cold walk counts 79 / 78. The port, having no pre-pass, was the cold walk.

### Two conditions, both necessary

- A **later** adjustment must fall on a month end, so `LastDayFn`
  (`INTSUTIL.pas:921-925`) is true and `daysinm` is reached.
- The **terminal month must be longer than the grid day**, or `daysinm` does not
  move it.

Together these explain why only day-of-month **30** shows the defect: only 30 can
be a month end (Apr/Jun/Sep/Nov) while still sitting below a 31-day terminal
month. Days 1, 15, 28 and 29 all agree. Verified against the oracle:

```
adj=67 + adj=69  -> 30.3.2029 (Mar 31, not a month end)  row 67 prin 743.50   agrees
adj=67 + adj=70  -> 30.4.2029 (Apr 30, MONTH END)        row 67 prin 732.81   diverges
adj=67 + adj=76  -> 30.10.2029 (Oct 31)                  row 67 prin 743.50   agrees
adj=67 + adj=77  -> 30.11.2029 (Nov 30, MONTH END)       row 67 prin 732.81   diverges
term 141 -> lastdate 30.4.2035 (Apr 30 == grid day)      no divergence
term 144 -> lastdate 30.7.2035 (Jul 31 > grid day)       diverges
```

### The fix

`internal/finance/amortization/dosport_adjprepass.go` (new) ports
`Amortize.pas:1408-1419` as `runAdjustmentPrePassDOS`, called from
`dosport_entry.go` immediately before the display walk.

It reproduces the side effect and **nothing else**: rate, balloon and prepayment
state are saved and restored exactly as `EstimateAndRefineAdjPayment` does, and
`e.d` is reset to the base payment (DOS's `d := h^.payamt`). Only
`e.loan.LastDate` carries forward.

**Caching the pre-pass AMOUNT would reproduce the old wrong answer**, because DOS
deliberately does not set `amtok` — the display walk must recompute. That is the
trap in this fix and the reason the comment at the call site is as long as it is.

### The piecewise half — CLOSED 2026-07-31 (round 9)

Round 8 fixed only the structural port and recorded the piecewise engine's two
copies of the same defect as a known gap. Both are now closed.

**1. The pre-pass gate conflated two distinct DOS steps.** `engine.go` ran
`runAdjustmentPrePass` only `if !input.inAdjPrePass && hardPayment`. DOS's SOLVE
LOOP is unconditional —

```pascal
for i := 1 to nadj do
  if (adj[i]^.loanratestatus >= defp) and (adj[i]^.amountstatus < defp) then
    begin
      if (not EstimateAndRefineAdjPayment(i)) then exit;
      d := h^.payamt;
    end                                          {Amortize.pas:1408-1412}
```

— and only the Dav Holle Round2 sweep that follows it is hard-payment gated:

```pascal
if (hard_payment) and (fancy) then begin
   for i:=1 to nadj do Round2(adj[i]^.amount);
   for i:=1 to nballoons do Round2(balloon[i]^.amount);
   end;                                          {Amortize.pas:1430-1435}
```

So the `hardPayment` test belongs on the ROUNDING, not on whether the pre-pass
runs. It has been moved inside `runAdjustmentPrePass`, which now takes
`hardPayment` and writes back `src.Amount` raw when the payment is solved and
`Round2(src.Amount)` when it is hard. With the pre-pass skipped, a solved-payment
screen began its display walk with a pristine `h^.lastdate` where DOS had already
snapped it.

**2. The snap was recorded to a display cell and never read back.** The seedN
block wrote `result.reAmortLastDate` and then re-read the pristine
`loan.LastDate` on the next adjustment, so with two adjustments the second one
counted against an unsnapped date. In DOS both counts go through the same
mutated global.

**THE TRAP, and why the fix is a dedicated variable.** Writing the snap into
`loan.LastDate` was tried on 2026-07-30 and moved an otherwise-exact schedule by
**6838.28** (`amort_oracle 252424.20 0.1170790000 44 2 prepaid usa
loandmy=31.12.2025 firstdmy=30.6.2026 mor=120 b126=53307.55 …` → DOS
`int=495329.73`, port `int=502168.01`, first divergence row 385/552). The
piecewise walk reads `loan.LastDate` at sites where DOS reads `very_last`; the
two engines' readers are **not** 1:1, and only the structural port has its
horizon (`e.veryLast`) pinned before the walk — which is why round 8's fix could
safely mutate `e.loan.LastDate` and this one cannot. The fix therefore threads
the snapped date through a new `adjLastDate` local read by the
`AMORTOP.pas:1547` counterpart and nothing else. That trap case is now exact
(495329.73 both sides). See
`claude/lost_session_recovery_and_reamortize_correction_2026-07-30.md` for the
asymmetric site model.

Both halves are load-bearing and were verified independently: with only the gate
removed the primary case still reported 60283.55, and with only the threading
applied it still reported 60283.55. The `payhard=` variant — where the pre-pass
already ran — fails on the threading alone.

Repro, now exact:

```
amort_oracle 100000 0.08 144 12 loandmy=30.6.2023 firstdmy=30.8.2023 \
             adj=67:0.04: adj=77:0.11: pre=20:24:12:150
  -> payment 1078.3207 interest 60633.01 paid 160633.01   (port was 60283.55)
```

Guard: `internal/finance/amortization/zzadj_prepass_piecewise_test.go`, eight
oracle-sourced cases including the `payhard=` isolation case and five controls.

### Guards

`internal/finance/amortization/zzadj_prepass_lastdate_test.go` — seven
oracle-sourced cases: the divergent case, both month-end controls
(Oct = agrees, Apr = diverges), both single-adjustment controls, the day-1
control, and the short-term control that pins the second necessary condition.
With the pre-pass disabled, exactly the two month-end cases fail (−353.12 and
−361.00) and every control stays green — which is what proves the fix is
conditional rather than a blanket shift.

### Verification

Full gated suite `PERSENSE_REQUIRE_ORACLE=1 go test ./... -count=1` **EXIT=0**,
12/12. **Paired regression** (`testplan/harness/paired_regression.sh`, seeds
42000-42079, N=400, 32,000 generated): **NEW = 0**. Randomized `goamort` vs
`amort_oracle` differential over 296 multi-option screens: unchanged at 282
agreeing — the 14 residual disagreements all carry balloons or prepayments and
route to the piecewise engine, i.e. the gap named above.

**Round 9 (the piecewise half).** Full gated suite EXIT=0, 12/12; `gofmt -l .`
empty tree-wide; `go vet` clean. **Paired regression** seeds 43000-43039, N=400
(16,000 generated): **NEW = 0**. A targeted 216-cell sweep over the mechanism's
own axes (day-of-month × month × term × adjustment offset, all with a
prepayment, `goamort` vs `amort_oracle`): **FIXED 21, NEW 0**, pre-fix failures
42 → post-fix 21. Every one of the 21 residuals was then traced to a HARNESS
defect, not the engine — see §51.

---

## §51 — `cmd/goamort` silently ROLLED impossible dates where the oracle stores them verbatim (2026-07-31)

**Class: harness defect, not an engine divergence.** The fourth in this family
(see `claude/START_HERE.md` §5); it manufactured 21 fake divergences in a single
round-9 sweep before being caught.

### What happened

The oracle's `ParseDMY` (`legacy/oracle/amort_oracle.pas:147-160`) writes the
three fields of a `D.M.Y` token straight into a Pascal `daterec`:

```pascal
dr.d := StrToIntDef(ds, 1);
dr.m := StrToIntDef(ms, 1);
dr.y := StrToIntDef(ys, 1924) - 1900;
```

No validation, no clamp, no roll. `loandmy=31.6.2023` leaves DOS holding an
impossible **31 June 2023**, and the DOS date primitives resolve it their own
way.

`cmd/goamort`'s `parseDMY` built the same token with `types.NewDateRec`, which
is `time.Time`-backed. `time.Date(2023, June, 31, …)` **normalises** to
**1 July 2023**. The two engines then ran different screens while the runner
believed it was comparing one.

### How it was caught, and the proof

A 216-cell sweep over day-of-month {1, 15, 28, 29, 30, 31} reported 21
"divergences" — every one at day 31 in a 30-day month, none anywhere else. That
shape indicts the harness (a divergence confined to exactly the inputs the
harness cannot represent is not an engine property). Confirmed directly:

```
oracle  loandmy=31.6.2023 firstdmy=31.8.2023 -> interest 60638.01
goamort loandmy=31.6.2023 firstdmy=31.8.2023 -> interest 59673.04
oracle  loandmy=1.7.2023  firstdmy=31.8.2023 -> interest 59673.04   <-- the rolled screen
goamort loandmy=1.7.2023  firstdmy=31.8.2023 -> interest 59673.04
```

goamort was answering the **rolled** screen correctly. There is no engine defect
in these 21 cases.

### The fix, and what is deliberately NOT fixed

`parseDMY` now REFUSES an unrepresentable date with an explanatory message on
stderr instead of silently substituting a different one. Refusal is the right
answer rather than a clamp: a clamp would be a third distinct behaviour (DOS
keeps 31 June, `time.Time` rolls to 1 July, a clamp would give 30 June) and
would go on producing quiet mismatches.

**The underlying representation gap is real, pre-existing and out of scope.**
`types.DateRec` wraps `time.Time` and structurally cannot hold an impossible
date, so DOS's behaviour on one is unreachable from the port at any level, not
just from the harness. Changing that is a port-wide representation change. It is
recorded here so the next sweep does not re-derive it: **the port cannot be
differentially tested on impossible calendar dates, and goamort now says so out
loud instead of pretending otherwise.**

---

## §52 — the display very-last fold fired at the last REGULAR payment instead of at `very_last` (2026-07-31)

**Status: FIXED (round 10).** Found by a generator written specifically to draw
what `TestDOSFuzzer5AllAdvancedOptions` cannot. Pre-existing — byte-identical
before and after round 9's §50 fix.

### Reduced case

```
amort_oracle 100000 0.08 180 12 loandmy=1.1.2024 firstdmy=1.2.2024 \
             adj=44:0.0706: pre=21:45:2:351.90
  DOS 68710.43 in 200 rows  |  Go 67086.46 in 180 rows
```

The payment was identical (1038.8268) and every row matched to the cent up to
row 180. DOS then kept walking, emitting prepayment-only rows of 351.90 every
six months until the series stopped; the port folded the outstanding 5662.07
into row 180 and ended the table there.

### Root cause

DOS's fold is keyed on `very_last` and on nothing else — PrintAndReset,
**AMORTOP.pas:1004**:

```pascal
if (DateComp(date,very_last)=0) then begin
 {Adjust last payment to cover entire remaining principal.}
    payamt:=payamt+principal;
    cumamt:=cumamt+principal;
    principal:=0;
    end;
```

and `very_last` is the LATEST of the last payment date, the last balloon date
and every prepayment stop date — DetermineVeryLast, **AMORTOP.pas:1293-1304**:

```pascal
if (nballoons > 0) and (DateComp(balloon[nballoons]^.date, h^.lastdate) > 0) then
  very_last := balloon[nballoons]^.date
else
  very_last := h^.lastdate;
for i := 1 to npre do
  if (DateComp(pre[i]^.stopdate, very_last) > 0) then
    very_last := pre[i]^.stopdate;
```

In the port, `terminalRow` is `payNum >= loan.NPeriods || atVeryLast`
(`engine.go`), so it goes true at the last REGULAR payment even when `veryLast`
is later. `generateFancyScheduleMode` has four fold branches; **three carried an
`atVeryLast` guard and the ARM branch did not** — and the ARM branch is tested
FIRST. On a loan carrying both an adjustment and a trailing prepayment series it
won the race, and the guard on the prepayment branch below it never got a
chance. The comment on that lower branch already stated the requirement
verbatim: *"The veryLast guard keeps NN-derived TRAILING prepay rows (which run
past the last regular payment) ahead of the fold."*

The fix is that guard, on the ARM branch. It is inert whenever nothing trails
the last regular payment (`very_last == h^.lastdate`), which is every case the
ARM branch was written for.

### The boundary, and why it looked like a three-way option interaction

Divergence switched on exactly when the series' stop date passed the last
payment date, and the delta grew with the overshoot:

```
ppy=2  NN=27  stop month 177 <= 180   agrees
ppy=2  NN=28  stop month 183 >  180   DIVERGED  (-6.11)
ppy=2  NN=45  stop month 285 >  180   DIVERGED  (-1623.97)
ppy=4  NN=54  stop month 180 <= 180   agrees
ppy=4  NN=55  stop month 183 >  180   DIVERGED  (-6.11)
ppy=6  NN=80  stop month 179 <= 180   agrees
ppy=6  NN=82  stop month 183 >  180   DIVERGED  (-6.11)
```

Removing any one of three conditions made the port agree — no adjustment (the
prepayment branch's own guard then applies), a stop date inside the term
(nothing trails), or a prepayment frequency at or above the payment frequency
(both engines refuse the overshoot outright, so nothing is observable). That is
what made a single missing guard read as an exotic interaction. The adjustment's
position is irrelevant: `adj=12:` (before the window opens) diverged by the same
1623.98.

### Why no sweep had ever seen it

`dos_fuzzer5_test.go` excludes the region twice over, by construction:

```go
ppy := []int{12, 24, 26, 52}[rng.Intn(4)]          // never 1, 2, 4 or 6
// Keep the series inside the term so CheckPrepayments' derived stop date
// does not run off the horizon.
remMonths := ((n-1)*mPer + firstMonths) - m
maxNN := (remMonths * ppy) / 12                    // NN can never overshoot
```

Both restrictions were deliberate and reasonable when written. Their combined
effect is that the divergent region had **probability zero** under the
generator, so every convergence number quoted before round 10 was conditioned on
its absence. Measured with a generator that does draw it: **36 of 120 randomized
screens divergent (30%) at `ppy != perYr` before the fix, 2 of 120 (1.7%)
after** — 0 of 120 at `ppy == perYr` in both cases.

**That is the durable lesson, and it outlives this defect: a divergence rate is
a property of the sampler as much as of the engine.** Before quoting one, read
the generator's bounds and name what it excludes.

### A dead end worth recording

The first audit pointer was wrong and cost a detour. `AMORTOP.pas:1355`

```pascal
while (DateComp(h^.lastdate, calc_pre[i].stopdate) < 0) do
  AddPeriod(calc_pre[i].stopdate, pre[i]^.peryr, startdate.d, subtract);
```

does fire on exactly this condition and does mutate `h^.lastdate`, which made it
look like another §50-style VAR leak. But it lives inside
`DetermineLastPaymentDate`, which DOS calls **only when `not h^.lastok`** —
i.e. only on a term solve. The reduced case supplies the term, so that routine
never runs. Ruled out; recorded per `docs/testing_policy.md` §7b.

### Guard

`internal/finance/amortization/zzprepay_overshoot_fold_test.go` — eight
oracle-sourced cases: the primary case, the minimal one-period overshoot at two
frequencies, three controls that each remove one necessary condition, and an ARM
negative-amortization case proving the new guard stayed inert on the branch's
original purpose. Removing the guard fails exactly the four overshoot cases and
leaves every control green.

### Verification

`gofmt -l .` empty tree-wide, `go vet` clean, full gated suite
`PERSENSE_REQUIRE_ORACLE=1 go test ./...` **EXIT=0** 12/12 with all three oracles
built. Paired regression **NEW = 0** on two seed ranges (43200-43239 general;
43300-43339 aimed at `noterm,non` — FIXED 0 in both, as expected, since the
fuzzer cannot generate this class). Randomized 281-screen sweep against the
round-9 build: **FIXED 22, NEW 0**, failures 48 → 26.

### Still open in this region

Two residuals survive in the 281-screen sweep, both distinct from this
mechanism and both reduced no further yet:

```
364306.79 0.104973 132 12 loandmy=30.5.2023 firstdmy=30.8.2023 \
  adj=122:0.0818: adj=12:0.1291: adj=112:0.0523: pre=32:31:2:185.20
  DOS 300901.45 | Go 300663.96   (was 300095.42 before this fix — improved, not closed)

185232.59 0.020861 360 4 loandmy=29.9.2023 firstdmy=29.11.2023 \
  adj=305:0.0303: pre=10:44:12:523.02 targ=401.73
  DOS 240705.40 | Go 240921.92   (unchanged by this fix; note ppy > perYr and a target)
```

---

## §53 — the ARM segment solve and the walk's regular-vs-extra test read the PRISTINE last-payment date where DOS reads the mutated global (2026-07-31)

**Status: FIXED (round 11).** This is the third and last reader of the §50 VAR
snap. Round 8 ported the mutation for the structural engine; round 9 gave the
piecewise engine a dedicated `adjLastDate` carrier and wired it to one reader;
this closes the other two.

### Reduced case

```
amort_oracle 364306.79 0.104973 132 12 loandmy=30.5.2023 firstdmy=30.8.2023 \
             adj=112:0.0523: adj=122:0.0818: pre=32:31:2:185.20
  before: Go 247075.78 in 145 rows
  after:  Go 247296.53 in 146 rows = DOS, exactly
```

Both engines agreed through the first adjustment (re-solved payment 5072.27 on
both). They parted on the second: DOS's Iterate returns **4720.33**,
`solveSegmentPayment` returned **5170.29**, and DOS emitted one regular payment
the port did not.

### The snap fires twice and COMPOUNDS

This is what made the case hard to see. `NumberOfInstallments`' `var l` snap
(INTSUTIL.pas:985-1019) has two arms, and consecutive adjustments can take one
each:

```
adj 1: f = 30.9.2032 — September has 30 days, so flast = TRUE
       l = 30.7.2034 — July has 31, so llast = FALSE; ddiff = 0 -> no month step
       `if (flast) then l.d := daysinm(l)`      {INTSUTIL.pas:1018}
       -> lastdate becomes 31.7.2034

adj 2: f = 30.7.2033 — flast = FALSE
       l = 31.7.2034 — llast is TRUE now; ddiff = +1 > 0, not(flast and llast)
       `l.m := l.m + monthsbtwn`                {INTSUTIL.pas:1003}
       then the else-arm restores `l.d := f.d` = 30
       -> lastdate becomes 30.8.2034
```

A whole **month** past the real last payment date. The first snap only moved the
day; the second used that moved day as its input and moved the month.

### The two readers

**1. `ComputeNext`'s regular-vs-extra test — AMORTOP.pas:602-613:**

```pascal
if (xsource > 0) then
  begin
    balloonpos := DateComp(nextextra.date, date);
    if (DateComp(date, h^.lastdate) > 0) then
      balloonpos := -1;
```

With `h^.lastdate` = 30.8.2034, the 30.8.2034 grid date is still a regular
payment; with the pristine 30.7.2034 it is forced off-cycle onto the next
prepayment. The port's counterpart (`engine.go`, the "past the last REGULAR
payment date" jump block) cited this exact DOS line in its own comment and read
`loan.LastDate`.

**2. The segment solve's period count.** DOS has no count here: its
`Re_Amortize` calls `Iterate(p, usap, Payment.date, t, d, til_adj)`
(AMORTOP.pas:1577), which walks `RepayFancyLoan`, whose `ComputeNext` applies
the same date test row by row. The number of regular payments in the segment is
therefore *implied by the snapped date*. The port passed
`remaining = loan.NPeriods - payNumNow`. It now passes the count derived from
`adjLastDate` — which is `seedN`, already computed one line above for the
annuity seed.

### The cap, and the regression it prevents

DOS bounds the segment walk **twice**: by the snapped date (ComputeNext) *and*
by `stopdate`, which for the segment Iterate's `adjnum = 0` call is `very_last`
(AMORTOP.pas:1140-1142). `very_last` is computed by `DetermineVeryLast` at
Amortize.pas:1320 — **before** the adjustment pre-pass at :1408 — so it never
sees the snap.

When the snap pushes `h^.lastdate` past `very_last`, the extra period is
unreachable: the walk ends first. Passing the uncapped count cost a real
regression, caught by the randomized `goamort` sweep before it landed:

```
amort_oracle 90498.48 0.108453 84 12 loandmy=28.2.2023 firstdmy=28.5.2023 \
             adj=18:0.1434: adj=24:0.1013: adj=37:0.0380: b17=3533.04
  DOS 31961.76  |  uncapped seedN gave 32059.15
```

There the 28.2.2025 adjustment (**February 28 IS a month end**) snapped
28.4.2030 → 30.4.2030, and the 28.3.2026 adjustment snapped that to 28.5.2030 —
one month past `very_last` = 28.4.2030, because that screen has no trailing
option to extend it. DOS solves 49 periods; the snapped count says 50.

### Verification

Three components, each verified independently by reverting it alone:

| reverted | effect on the reduced case |
|---|---|
| the `ComputeNext` reader | 250470.28 (+3173.75) |
| the segment count | 246519.77 (−776.76) |
| the `very_last` cap | cap case 32059.15 (+97.39); §53 cases stay green |

Controls stay green in every direction. `gofmt -l .` empty tree-wide, `go vet`
clean, full gated suite `PERSENSE_REQUIRE_ORACLE=1 go test ./...` **EXIT=0**
12/12. Paired regression **NEW = 0** on two seed ranges (44000-44039 general;
44200-44239 aimed at `noterm,non`). Randomized 281-screen sweep against the
round-10 build: **FIXED 1, NEW 0**.

### Guard

`internal/finance/amortization/zzadj_segment_horizon_test.go` — eight
oracle-sourced cases: the two-adjustment and three-adjustment screens, the cap
case, and five controls. The day-15 control is the sharpest of them: its value,
**247075.78, is exactly what the port produced for the day-30 case before this
fix**, which is the cleanest available demonstration that the port had been
walking the un-snapped schedule.

---

## §54 — DEFERRED BY DECISION: DOS's leap rule has no century correction, and `types.DateRec` cannot hold 29 Feb 2100 (2026-07-31)

**Status: documented and deferred (Nate's call, 2026-07-31). Guarded by
`internal/dateutil/zzleap_century_test.go` so the limit is stated, not
rediscovered.**

### The two calendars

DOS, `VIDEODAT.pas:340-347`:

```pascal
function DaysInM(f :daterec):byte;
         begin with f do begin
         if (m=2) then begin
            if (y mod 4 = 0) then daysinm:=29 else daysinm:=28;
            end
```

No century correction. DOS believes **29 February 2100 exists**. The port's
`daysInMonthPascal` reproduces that faithfully and returns 29 — but
`types.DateRec` wraps a `time.Time`, and `time.Date(2100, February, 29, …)`
normalizes to **1 March 2100**. The arithmetic layer is faithful; the storage
layer cannot hold what it computes.

Note DOS is internally inconsistent about this: `DecideAboutFeb29` guards with
`(wy mod 4 = 0) and (wy>0)` — excluding 1900 — while `DaysInM` has no such
guard, and `INTSUTIL.pas:1683` carries a comment about Lotus "not recognizing
that 1900 wasn't a leap year".

### Measured boundary

Bisected against the real DOS engine on a quarterly day-29 grid:

```
n=305  last payment 11/2099   DOS 177231.14  Go 177231.14   agrees
n=306  last payment  2/2100   DOS 177735.47  Go 177735.54   DIVERGES
```

Day-of-month 15 and 28 agree at every term (they never touch the 29th); 29 and
30 diverge from n=306 on. 2100 is the only century non-leap year inside DOS's
forward date range, so it is the only place the calendars can part company.

Direct probe:

```
dateutil.DaysInM(Feb 2100) = 29        (DOS's answer, correctly ported)
types.NewDateRec(2100, Feb, 29)        -> 2100-03-01
AddPeriodFields(29.11.2099, perYr=4, anchorDay=29) -> 2100-03-01
```

### Why it is deferred rather than fixed

Closing it means giving `DateRec` raw y/m/d fields like the Pascal record — a
port-wide refactor touching every date site and every test. It would also close
**§51** (the port cannot hold an impossible date such as 31 June either — the
same root cause), so if that refactor is ever undertaken the two should be done
together. The affected region is schedules that reach February 2100 on a
day-29/30 grid.

**Related and unclosed:** §51 (impossible calendar dates) and this section are
the two known consequences of the `time.Time` backing. Neither is a logic defect
in the ported arithmetic.

### QUANTIFIED, round 17 (2026-08-02) — §54 *is* the long-horizon residual

Rounds 13-16c priced this section off a "stratum C" divergence rate — schedules
whose last payment lands 2092-2155, past DOS's 70000-day Julian ceiling and
inside the `daterec` year byte. The rate moved from 17.6% (n=13) to 14.0%
(n=343, round 16c) and was the strongest measured argument for the refactor.
**It was never a stratum-C rate.** `long_horizon_sweep.py` drew its term off a
nine-point lattice, and round 17 measured what that lattice reaches: stratum C
was populated by the single `90` point, i.e. last years **2109-2120 — 12 of the
stratum's 64 years.** Every screen it could draw ended after 2100, so the
Julian-ceiling mechanism and the leap-2100 mechanism were **perfectly
confounded**: no screen existed that was past the ceiling and did not cross
1 March 2100.

`--years-mode wide` adds the lattice points that break the confound. Seeds
913+77, n=6000 each, representable inputs only, DOS refusals excluded:

| sub-range | what it isolates | honest den | diverged | rate |
|---|---|---|---|---|
| **C1 2092-2099** | **past the Julian ceiling, does NOT cross 1 Mar 2100** | 605 | 9 | **1.5%** |
| C2 2100-2120 | crosses 2100 — the only band the old lattice could reach | 1238 | 159 | **12.8%** |
| C3 2121-2155 | crossed 2100 long ago, approaching the year byte | 2386 | 305 | **12.8%** |
| C (all) | | 4229 | 473 | 11.2% |

**A step function at one calendar date.** C1 sits at 1.5%, indistinguishable
from stratum B's ~1.0% — so **DOS's Julian ceiling costs the port essentially
nothing.** Crossing 1 March 2100 takes the rate to 12.8%, and it is then FLAT to
the year byte: C3 equals C2 to the tenth of a point, so proximity to 2155 (§55's
boundary) contributes nothing either. The entire stratum-C concentration carried
for four rounds is this section, and only this section.

Source attribution, not inference. DOS's `mod 4` rule appears independently in
its calendar table (`VIDEODAT.pas:334, 343`) **and** in the day-count basis
(`INTSUTIL.pas:812`, `if (a.y mod 4 = 0) and (a.y>0) then yrdaz:=366`). The
port's `dateutil.isLeapYearPascal` (`dateutil.go:105`) reproduces it exactly.
`types.DateRec` does not: it wraps `time.Time` (`types/records.go:17,22`), which
is proleptic Gregorian. The two calendars the port carries first disagree in
**2100**, agree again 2104-2196, and disagree next in **2200** — which is past
the year byte and therefore in stratum D.

**What this does and does not license.** It prices the refactor: the cost of the
two date layers is ~11 points of divergence on every schedule that reaches 2100
and ~0 before it. It does not re-open the decision — that is Nate's
(`START_HERE` §7). Caveats that travel with the number: forward payment solves
only (`long_horizon_sweep.py` emits no backward modes — round 16c), a thin
option surface (basis / adj / prepay), and a `wide` lattice whose rates must
never be compared against a `default`-lattice rate, since the two describe
different populations.

---

## §55 — DOS stores a date's YEAR in a BYTE, so every long horizon wraps mod 256; the port's year was unbounded (2026-07-31)

**Status: FIXED (round 12).** The amortization counterpart of §47.

### The rule

```pascal
daterec = record  d,m : shortint;  y : byte;  end;     { Globals.pas:46-48 }
```

The year is ONE BYTE, and the DOS sources are compiled with range checking off,
so **every assignment to that field truncates mod 256**:

```pascal
lastdate.y := firstdate.y + nyears;                    { INTSUTIL.pas:1402 }
...  inc(y); m := m - 12;                              { INTSUTIL.pas:1233 }
...  dec(y); m := m + 12;                              { INTSUTIL.pas:1244 }
```

`y` is years-since-1900, so the representable range is **1900–2155** and
anything past it rolls back to 1900. This is not a display artifact: the wrapped
value is what the ENGINE computes with. `NumberOfInstallments` counts
`12*(l.y-f.y)` off the truncated byte (INTSUTIL.pas:1037), so the payment solve,
the row count and the totals all follow the wrapped horizon.

Verified against the real DOS engine — the boundary is exact:

```
amort_oracle intutil addn 2023 7 29 1 299   -> last 2066 7 29   (123+299 = 422 -> 166)
amort_oracle intutil addn 2023 7 29 1 133   -> last 1900 7 29   (123+133 = 256 -> 0)
amort_oracle intutil addn 2023 7 29 1 132   -> last 2155 7 29   (123+132 = 255, inert)
amort_oracle intutil addn 2150 6 15 12 67   -> last 1900 1 15   (AddPeriod inc(y))
amort_oracle intutil addn 1901 6 15 12 -24  -> last 2155 6 15   (AddPeriod dec(y))
```

### What the port did

`internal/dateutil` stores a `time.Time`, whose year is unbounded, and computed
`lastPY := py + nyears` with no truncation. A loan whose nominal terminal was
2322 therefore got a 2322 horizon in the port and a **2066** horizon in DOS.

The worst screen measured in `claude/convergence_assessment_2026-07-31c.md` §3 —
which **DOS answers cleanly**, it is not a refusal:

```
amort_oracle 391495.35 0.029252 300 1 loandmy=29.5.2023 firstdmy=29.7.2023 \
             adj=188:0.0808: adj=90:0.0227: pre=28:25:12:74.69

DOS  payment 16693.3528  interest   623,111.96   —  67 rows, retires 7/29/2066
Go   payment 11747.0979  interest 8,964,450.80   — 323 rows, runs to 2323
```

DOS's 44 annual payments are `NumberOfInstallments(7/29/2023, 7/29/2066, 1)`;
the port solved over the entered 300.

### The fix

`wrapPascalYear(py) = ((py % 256) + 256) % 256`, applied at each site where DOS
assigns to the year field. The sites are independent and were verified
independently (reverting either produces a distinct failure signature):

| site | DOS | port |
|---|---|---|
| **A** the year JUMP | `lastdate.y := firstdate.y + nyears` (INTSUTIL.pas:1402) | `AddNPeriods` |
| **B** `inc(y)` / `dec(y)` | INTSUTIL.pas:1224/1233/1244/1248 | `AddPeriodFields`, both the 24 arm and the 1/2/3/4/6/12 arm |
| **C** the walk's horizon | `until … DateComp(WhenToStop^.date, stopdate) >= 0` (AMORTOP.pas:1221) | `generateSimpleSchedule`'s `walkPeriods` clamp — see consequence 2 below |

Because the result is always in `[0, 255]` the calendar year is always in
`[1900, 2155]`, so — unlike §51 and §54 — this rule needs **no** raw y/m/d
fields on `types.DateRec`. The 26/52 arm is untouched: it goes through
`Julian`/`MDY`, where DOS's 70000-day ceiling (VIDEODAT.pas:373, restored for PV
in §47) already refuses.

### Measured effect

`testplan/harness/long_horizon_sweep.py`, 200 random long-horizon screens,
stratified by the NOMINAL (un-wrapped) year of the last payment:

| stratum | screens | DOS refused | before | after |
|---|---|---|---|---|
| A ≤ 2048 | 36 | 1 | 0/35 | 0/35 |
| B 2049–2091 | 48 | 0 | 1/48 | 1/48 |
| C 2092–2155 (past the Julian ceiling) | 20 | 5 | 1/15 | 1/15 |
| **D > 2155 (past the year byte)** | 96 | 82 | **14/14 (100%)** | **2/14 (14%)** |
| total | 200 | 88 | 16/112 (14.3%) | **4/112 (3.6%)** |

Confirmed on an independent seed (913, 200 screens): stratum D **13/13 (100%) →
2/13 (15%)**, total 23/128 (18.0%) → 12/128 (9.4%). The seed-913 residual is
mostly in strata A and B and is UNCHANGED between the two builds — pre-existing
divergences this harsher generator finds and fuzzer5 does not, unrelated to §55.

Paired against the round-11 build: **FIXED 43, NEW 0** (seed 77); **FIXED 30,
NEW 0** (seed 913). `paired_regression.sh`
over both standard fuzzer5 windows: **NEW 0** (fuzzer5 cannot draw a schedule
longer than 25 years, so it neither gains nor loses here — which is exactly why
this defect survived).

### Two consequences worth knowing

1. **The engine's 10000-payment guard is now nearly unreachable through the
   term.** With the horizon bounded to 2155 a monthly schedule cannot exceed
   ~3060 rows. `generateSimpleSchedule`'s `NPeriods > MaxSchedulePeriods` refusal
   still fires on the ENTERED term, and DOS does not refuse there
   (`amort_oracle 200000 0.06 10001 12 … payhard=0.01` answers) — a narrow
   over-refusal that is now the residual, not the headline. See below.
2. **There is a SECOND reader of this same rule, and it is also fixed.**
   `generateSimpleSchedule` walked `for i := 0; i < loan.NPeriods; i++` with no
   horizon bound at all, so once the year wrapped it cycled the calendar
   repeatedly instead of stopping. DOS's one table loop ends on
   `DateComp(WhenToStop^.date, stopdate) >= 0` (AMORTOP.pas:1221) with
   `stopdate = very_last`, which for an option-free screen is `h^.lastdate`:

   ```
   amort_oracle 421052.18 0.047119 7200 24 loandmy=15.4.2029 firstdmy=15.5.2029
     DOS  1056 rows, interest   875,474.53, final row 4/30/2073 clears 421875.06
     Go   7200 rows, interest 5,542,481.87, having wrapped the calendar 28 times
   ```

   (The fancy/piecewise walk already had the test — `if loan.LastOK &&
   DateComp(currentDate, adjLastDate) > 0` — which is why case A above needed
   only reader 1.)

   **The transcription matters, and the obvious one is wrong.** Writing DOS's
   `until` literally, as a per-row `DateComp(currentDate, lastDate) >= 0`, was
   tried first and it regressed a screen whose horizon does not wrap at all:

   ```
   amort_oracle 114948.20 0.025189 1080 12 loandmy=29.4.2029 firstdmy=29.5.2029 exact
     DOS   interest 175844.60, 1080 rows
     row-by-row form: 175844.03 — folded one row early
   ```

   The cause is §54: the port's WALK dates drift a month at February 2100 on any
   day-29/30 grid, so a row-by-row date test reaches `lastDate` before the
   period counter does. Counting the bound off the two ENDPOINTS instead —
   `NumberOfInstallments(FirstDate, LastDate, PerYr, ON_OR_BEFORE)`, clamped
   only when it is SMALLER than the term — uses dateutil's DOS-faithful
   arithmetic and cannot be moved by a mid-walk drift. It was the single NEW
   divergence in the seed-77 sweep and is now a standing test case
   (`E/inert-when-the-horizon-does-not-wrap`).

   **This is the general lesson, not a detail of this section: a literal
   transcription of a DOS loop condition is only safe where the port's dates are
   identical to DOS's, and §54 guarantees they are not everywhere.**

### Tests

- `internal/dateutil/zzyearbyte_test.go` — site-by-site goldens with their oracle
  command lines, plus a live differential that sweeps both byte boundaries at
  every monthly-family frequency (49 probes, 0 mismatched).
- `internal/finance/amortization/zzyear_byte_horizon_test.go` — the engine-level
  case above, the one-short-of-the-wrap boundary (must stay inert), and the
  mirror direction: a wrap that lands BEHIND the first payment must be refused,
  as DOS refuses it.
- `testplan/harness/long_horizon_sweep.py` — the standing instrument for the
  region `dos_fuzzer5_test.go` cannot generate.

Two pre-existing tests changed with this section, both because their premise was
an input DOS cannot represent: `TestSolvePeriodicAmountZeroFactor` (dates moved
from 2300 to 2150 — still ~5e-16, still reaches the guard) and
`TestAmortizeMaxIterSafety` (renamed; a 12000-period monthly term now wraps to
the year 2000 and is refused on date order, which is what DOS does).

---

## §56 — `exact` at the 360 basis is a no-op for the payment SOLVE but not for the schedule DISPLAY; the port collapsed the two (2026-07-31)

**Status: FIXED (round 13).** Found by the stratum-A arm of
`long_horizon_sweep.py`, seed 913 — the region §55's instrument was built for,
but at the *near* end of it, in the same date range `dos_fuzzer5_test.go`
already samples.

### The rule

`df.c.exact` has five computational readers. **Four are gated on the basis; one
is not.**

```pascal
AMORTOP.pas:625    if ((df.c.basis=x360) or (not df.c.exact)) and DaysCloseEnough(...)
AMORTOP.pas:1438   if (fancy) or ((df.c.exact) and (df.c.basis<>x360))   { Iterate seed }
AMORTOP.pas:1464   if (fancy) or ((df.c.exact) and (df.c.basis<>x360))   { Iterate loop }
AMORTOP.pas:1571   if (user_nballoons>0) or (npre>0) or ((df.c.exact) and (df.c.basis<>x360))
Amortize.pas:458   if ((df.c.basis=x360) or (not df.c.exact)) and (prepaid) and ...

Amortize.pas:1493  if (fancy) or ((df.c.exact) and (not df.c.R78))
                              or (not (df.c.basis=x360)) then    { <-- NO BASIS GUARD }
                     RepayFancyLoan(...)
                   else
                     { the inline nominal table loop, Amortize.pas:1500-1553 }
```

So at the 360 basis with `exact` on, DOS **solves** the payment with the nominal
`RepayLoan` recursion (`p := p*f - d`, `f-1 = loanrate/RealPerYr(peryr)`) and
then **renders** the schedule with the date-walking `RepayFancyLoan`. The APR
dispatch (Amortize.pas:553/572) is ungated the same way.

The port's `exactDaily()` helper — `Exact && Basis != Basis360` — collapsed all
five readers into one predicate. That is right for the four gated sites and
wrong for the display, so display stayed on `generateSimpleSchedule`.

### Why it hid

On a grid whose period is a whole number of months the two DOS engines are the
same computation, **algebraically, not coincidentally**:

- `AddPeriod`'s `else` branch pins `d := orig_day` every period
  (INTSUTIL.pas:1240), so consecutive dates share a day-of-month (or both clamp
  to month-end and `LastDayFn` fires);
- so `DaysCloseEnough` (INTSUTIL.pas:716-727) always holds;
- so `ComputeNext` takes the nominal branch (AMORTOP.pas:627) and
  `timedif = Δm/12 = (12/peryr)/12 = 1/peryr`;
- which is exactly `RepayLoan`'s `f-1 = loanrate/RealPerYr(peryr)`.

peryr 26 and 52 never reach this gate at a real 360 basis at all —
`coerceSubMonthlyBasis` (Amortize.pas:297-303) rewrites their basis to 365
upstream and `exactDaily` then fires on its own.

**That leaves peryr=24 as the only observable frequency**, and only on anchors
that are not the 15th. The semi-monthly `AddPeriod` branch (INTSUTIL.pas:1217-1238)
walks `d±15` instead of pinning the anchor, so `DaysCloseEnough` fails and
`timedif` comes from `YearsDif`'s 30/360 rules — which are 15/360 in the ordinary
case but **not** across February:

```
INTSUTIL.pas:798   if (a.d=31) and (z.d<31)   then til := til + 1/360
INTSUTIL.pas:800   else if (a.m=2) and (a.d>27) then til := til - (30-a.d)/360
```

A 15th anchor is protected by `LastDayFn`'s explicit `(peryr=24) and (d=15)`
special case (INTSUTIL.pas:923) plus `ComputeNext`'s half-month snap
(AMORTOP.pas:628-629), and does not leak.

### The repro

```
amort_oracle 40606.39 0.094051 600 24 loandmy=29.6.2021 firstdmy=29.7.2021 exact dumpraw
```

The 29th anchor walks a 29th/14th grid. Every row is 15/360 except the two that
touch February:

```
L13|14  2/14/22  158.87        <- 15/360, agrees
L14|15  2/28/22  148.21        <- 14/360 (the 29th clamped by CheckForDaysTooLarge)
L15|16  3/14/22  148.11        <- 14/360 (INTSUTIL.pas:800's Feb correction)
L16|17  3/29/22  158.57        <- 15/360, agrees again
```

```
DOS  payment 176.6524  interest 63873.37  paid 104479.76   (595 dumpraw lines)
Go   payment 176.6524  interest 65385.02  paid 105991.41   (602)  = its own exact-OFF answer
```

The payment agrees — it is solved by `RepayLoan` on both sides, exactly as the
basis-gated Iterate says it should be. Only the rows move, by $1,511.65 over 600
periods, and the loan retires eight rows early.

Second, independent draw (seed 913, different amount/rate/term/anchor month):

```
amort_oracle 274179.66 0.036833 360 24 loandmy=29.10.2022 firstdmy=29.11.2022 exact
DOS interest 82839.82 | Go 83432.91   (delta 593.09)
```

### The fix

`internal/finance/amortization/engine.go`, the DISPLAY dispatch — a third arm
beside the existing in-advance one:

```go
} else if settings.Exact && !settings.R78 && !wholeMonthGrid(loan.PerYr) {
        result = generateFancySchedule(dispInput, d, &settings, truerate, f)
}
```

plus a new `wholeMonthGrid(perYr)` helper (`perYr <= 12 && 12%perYr == 0`).

**On the `wholeMonthGrid` term — it is a deliberate narrowing, and it was forced.**
The literal gate (`exact and not R78`, no frequency test, as DOS writes it) was
implemented first and **regressed `zzyear_byte_horizon_test.go` case E**: a
1080-period monthly `exact` loan running to 2119 rendered 176010.29 against DOS's
175844.60, one row short. That is START_HERE §5's first trap in its purest form.
DOS cannot tell which of its two engines rendered a whole-month grid — they are
identical there — but the port can, because **§54 is deferred** and its walk dates
drift a month at February 2100 while DOS's plain mod-4 leap rule does not. Routing
a grid DOS proves inert through the port's date walk imports a §54 artifact and
nothing else.

So the narrowing declines the redirect exactly where DOS's own two engines
provably agree, and it loses no DOS behaviour. It is scoped to the identity, not
to the symptom: case D of the regression test is peryr=24 on the protected 15th
anchor, where `DaysCloseEnough` does hold, and it still goes through the walk and
still matches. **If §54 is ever closed, this term can be dropped and the gate made
literal.**

This is NOT the 2026-07-25 seed-20110 finding (`dosport_entry.go:451`). That one
was about routing the whole `exact × 360` COMPUTATION to the piecewise engine and
remains correct; this is display-only, and case C of the regression test pins the
monthly answer the seed-20110 finding was protecting.

### Measured

```
long_horizon_sweep seed 913 stratum A   11/103 (10.7%) -> 6/103 (5.8%)   FIXED 5, NEW 0
long_horizon_sweep seed 913 all strata  12/128 (9.4%)  -> 10/128 (7.8%)  FIXED 2, NEW 0
long_horizon_sweep seed 77  all strata   4/112 (3.6%)  -> 4/112          FIXED 0, NEW 0
peryr 1/2/4/6/12 x four anchor days     bit-identical through the new branch
PERSENSE_REQUIRE_ORACLE=1 go test ./... EXIT=0, 12/12
```

### Tests

`internal/finance/amortization/zzexact360_display_test.go` — six cases, and the
gate's **three independent terms each have their own revert signature**:

| revert | what fails |
|---|---|
| delete the whole arm | A and B render their exact-OFF answers; C-F stay green |
| drop `!settings.R78` | E alone flips, to 63873.37 / 104479.76 |
| drop `!wholeMonthGrid(...)` | F alone fails, 176010.29 vs 175844.60, one row short |

Cases C (monthly), D (peryr=24 on the 15th anchor), E (R78 suppression) and F
(long monthly past Feb 2100) are the inertness proofs.

### Harness

`testplan/harness/long_horizon_sweep.py` gained `--stratum LETTERS`, which skips
the engine calls for screens outside the chosen strata while drawing from the
same unfiltered stream — so a given `--seed` yields the same screens with or
without the filter. Stratum D refuses ~85% of its draws, so an A-only harvest
that would have cost 600 oracle runs costs ~105.

### §56 addendum — the backward-solve guard, and a CORRECTED finding

`paired_regression.sh 44000-44039` returned **FIXED 0, STILL 56, NEW 1** on the
first §56 build. The regression:

```
amort_oracle 291207.99 0.1209560000 2688 24 exact prepaid loandmy=29.5.2024 \
  firstdmy=29.7.2024 pts=0.009110 payhard=1962.94 norate bdump
```

a semi-monthly `exact` **rate solve**. The §56 arm was firing inside the backward
solvers' trial evaluations, so the solve bisected on a residual DOS never
computes. `!input.inBackwardSolve` was added to the gate — the same rule
`dosPortCanHandle` already applies (dosport_entry.go:428), and the DOS-faithful
one: Iterate's terminal (AMORTOP.pas:1438/:1464) IS basis-guarded, and
Amortize.pas:1493 runs afterwards, on the solved answer.

**The signal did not clear.** On the re-run the same case still reports, and its
nature is now understood: it is `fz5NonConverge` + non-retiring — a `t.Logf`
ADVISORY, not a `t.Errorf`. DOS refuses this screen outright ("did not
converge"), so there is no oracle answer to be faithful to; what changed is that
the port's schedule, drawn from the solved rate, no longer retires the loan.
`paired_regression.sh` greps every `amort_oracle` line, advisories included
(standing rule 9), so it scores as NEW.

### RETRACTION — there is NO semi-monthly backward-rate-solve defect

**The first version of this addendum claimed the semi-monthly backward RATE
solve was "out by up to 76%". That was wrong, and it was a harness error of
exactly the family this project has now made five times.**

`cmd/goamort` does **not implement `norate`**, and its token loop has no
`default` arm — it **silently ignores unknown tokens**. So the "port" column in
that comparison was a FORWARD run at the entered rate 0.094051, while DOS had
solved a rate of 0.1101 or 0.1427 and amortized that. Two different
computations, compared as if they were one. Proof:

```
$ goamort 40606.39 0.094051 600 24 ... payhard=250.00 THIS_IS_GARBAGE
payment 250.0000 interest 24519.91 paid 65126.30      <- identical without the token
```

**Measured properly, by calling `SolveRate` in-process, the port matches DOS
exactly** — solved rate to ten decimals, totals to the cent, on all six screens.
The rate solve was never the problem.

### What the corrected measurement actually shows: §56 is stronger than claimed

The same six screens are three MORE independent confirmations of §56, on a path
the original repro never touched — a backward solve:

```
amort_oracle 40606.39 0.094051 600 24 loandmy=<a>.6.2021 firstdmy=<a>.7.2021 \
  exact payhard=<pay> norate

 a  pay      DOS solvedrate   DOS int / paid          port BEFORE §56        after
29  176.65   0.0940493440      63871.65 / 104478.04    65383.41 / 105989.80  exact
29  200.00   0.1101240219      76987.20 / 117593.59    79394.02 / 120000.41  exact
29  250.00   0.1427016805     103676.47 / 144282.86   109393.48 / 149999.87  exact
15  176.65   0.0940493440      65383.41 / 105989.80    (matches, before and after)
15  200.00   0.1101240219      79394.02 / 120000.41    (matches, before and after)
15  250.00   0.1427016805     109393.48 / 149999.87    (matches, before and after)
```

Read the "before" column against the 15-anchor rows: **pre-§56 the port rendered
the 15th anchor's schedule for a 29th-anchor loan**, running the full 600 rows
where DOS retires at 592 / 588 / 578. §56 closes all three to the cent, and
leaves the protected 15th anchor untouched — the inertness half, on the backward
path.

That is why the `!input.inBackwardSolve` term is right AND the display arm is
right: the solver's trials stay nominal (as DOS's basis-guarded Iterate does),
the final table walks dates (as Amortize.pas:1493 does).

Regression coverage: `TestDOSExact360SolvedRateRendersWithTheDateWalk` in
`zzexact360_display_test.go`. Verified both directions — on the pre-§56 tree all
three 29-anchor rows fail by 1511.76 / 2406.82 / 5717.01 and eight-to-twenty-two
rows, while all three 15-anchor rows stay green.

**The remaining NEW=1 advisory is therefore the only open item from this gate,
and it is on a screen DOS refuses.** §56 is kept.

---

## §57 — the port gates DOS's rate `Iterate` behind a Go-only closed-form pre-solve; when the pre-solve exhausts its budget the port reports non-convergence where DOS converges (2026-08-01)

**Status: FIXED (round 15).** Found by the round-14 round-trip differential
(`zzroundtrip_test.go`), adjudicated by its new horizon strata
(`TestRoundTripRateHorizonStrata`). Round 14 recorded the symptom and explicitly
declined to call it a defect because §54 and §55 were confounded with it; the
stratification separated them.

### The rule

DOS's rate solve has **no pre-solve**. `EstimateAndRefineRate`
(Amortize.pas:467-491) seeds and calls `Iterate` unconditionally:

```pascal
loanrate := payamt * peryr / amount;      {first guess - better high than low.}
if (loanrate<0.02) then loanrate := 0.02; {Iterate won't work if you start with zero.}
if Iterate(amount, usap, loandate, firstdate, loanrate, til_adj) then
  begin EstimateAndRefineRate := true; loanratestatus := outp;
        f := GrowthPerPeriod; ComputeTrueRate; end
else
  begin EstimateAndRefineRate := false; loanratestatus := empty; end;
```

The port added a closed-form Newton loop in front of that (`backward.go`,
`const maxIter = 30`, a secant on the `RepayLoan` terminal residual) and made
`dosIterateRate` — the DOS `Iterate` — reachable **only from inside that loop's
`math.Abs(step) < teeny` convergence branch.** So when the port's own pre-solve
failed to settle within 30 iterations, `SolveRate` fell out of the loop and
returned the **unrefined closed-form estimate with `converged=false`**, having
never attempted the DOS `Iterate` at all.

`SolveLoanAmount` does **not** have this shape and is unaffected: its estimate is
a genuine closed form with no iteration, and it calls `dosIterateAmount`
unconditionally whenever `needScheduleRefine` says to (backward.go:332).

### Where it is live

Loans deep into **perpetuity territory** — where the payment is within a hair of
`A·i` and the principal barely amortizes. The marker is the perpetuity depth
`(1+i)^-n`; every observed failure sits below `1e-5`. There `RepayLoan`'s
terminal balance is astronomically stiff in the rate, the secant's reused
`delta` collapses into cancellation noise, and `|step|` never reaches `teeny`.

Measured on the round-trip gate's horizon strata (25 cases each, seed 1501):

| stratum | span | ends | port-worse than DOS, pre-fix | post-fix |
|---|---|---|---|---|
| A | 2-40 yr | ≤2064 | 0 / 25 | 0 / 25 |
| B | 50-74 yr | ≤2098 | **3 / 25** | 0 / 25 |
| C | 80-125 yr | 2104+ | **6 / 22** | 0 / 22 |
| D | 140-320 yr | 2164+ | **1 / 3** | 0 / 3 |

**Stratum B is the adjudication.** Its failures end in 2095, 2096 and 2098 —
short of both §54's Feb 2100 century-leap divergence and §55's 2155 year byte —
so this is **not** a date-layer effect, and round 14's long-horizon signal is
**not** evidence for the deferred date refactor. The matched-n control (identical
period counts over a short and a long calendar span: `n=100` at `perYr=12` ends
2032 and is clean, at `perYr=1` ends 2124 and was 7/25 port-worse) puts the
effect on the calendar span, not on the iteration count.

Three representative screens, DOS vs the port before and after:

```
amort_oracle 77668.37 0.1687130000 864 12 payhard=1091.98 norate
  DOS solvedrate 0.1687132662 | port pre-fix 0.1687354022 (converged=false) | post-fix 0.1687132662
amort_oracle 229095.37 0.1853950000 426 6 payhard=7078.87 norate
  DOS solvedrate 0.1853949316 | port pre-fix 0.1855860626 (converged=false) | post-fix 0.1853949316
amort_oracle 328360.37 0.1886200000 888 12 payhard=5161.28 norate
  DOS solvedrate 0.1886198999 | port pre-fix 0.1890050073 (converged=false) | post-fix 0.1886198999
```

The user-visible effect is twofold: a rate wrong in the 4th-5th decimal place,
**and** a spurious "did not converge" warning on a screen DOS answers cleanly.

### The fix

After the Newton loop exhausts `maxIter` — and **after** the existing ±200%
refusal, which DOS also applies (AMORTOP.pas:1485) — run `dosIterateRate` from
the DOS seed, exactly as DOS does, and accept the result under the same guards
the converged branch uses. Those guards moved into `acceptRefinedRate` so the
two paths cannot drift apart.

Regression: `internal/finance/amortization/zzratepresolve_test.go`, four tests,
one per independent component. Reverting the fallback alone fails exactly the
three §57 cases and leaves the other three tests green.

### Independent confirmation from the fuzzer corpus

§57 was found and fixed through the round-trip gate, which is a different
instrument from `dos_fuzzer5_test.go`. It also shows up in the fuzzer corpus,
which is the stronger evidence because nothing about that corpus was chosen with
§57 in mind.

`paired_regression 44000-44039` (all modes, `FUZZ_N=400`) run pre-§57 vs post-§57
reported **FIXED 1, STILL 56, NEW 0**, and the same range run pre-§56 vs post-§57
reported **FIXED 1, STILL 55, NEW 1**. The FIXED case, recovered from the run's
work files, is:

```
amort_oracle 226197.89 0.1117370000 85 1 b365 prepaid r78 usa
  loandmy=6.8.2023 firstdmy=6.11.2023 mor=279
  pre=507:380:52:90.02 pre=543:5:1:1833.16
  adj=759:0.0237490000:33908.00 adj=795:0.0911720000:28289.15
  targ=2504.22 payhard=30049.30 norate bdump
```

**`norate` — a backward RATE solve — at `perYr=1` over `n=85`, i.e. an 85-year
calendar span.** That is §57's signature exactly, arrived at from the opposite
direction and on a screen carrying a full set of advanced options rather than the
plain loans the round-trip gate draws.

The `NEW 1` in the pre-§56 comparison is **round 13's known advisory, unchanged**:

```
amort_oracle 291207.99 0.1209560000 2688 24 exact prepaid
  loandmy=29.5.2024 firstdmy=29.7.2024 pts=0.009110 payhard=1962.94 norate bdump
```

semi-monthly, `exact`, off the 15th anchor — §56's region, not §57's. **§57 does
NOT close it**, which was the hypothesis this run was started to test. It remains
the one open item from §56's gate.

### Still open, and it is the same shape

The port's ±200% refusal is triggered by **the port's pre-solve** wandering out
of range; DOS's is triggered by **DOS's own `Iterate`**. That is the same "a
Go-only pre-solve is gating a DOS decision" defect as §57, in the refusal
direction. No screen was found where the two disagree — `TestSec57RefusalStillFires`
passes with the refusal and the fallback swapped — so it is recorded here rather
than fixed.

It was also probed directly (round 15, 600 screens, seed 7701) with the payment
drawn **independently** of any forward solve, at 0.05x-6x of `amount/n`, which is
the draw that actually reaches a divergent rate solve:

```
drawn 600 | port refused 18 | DOS also refused all 18 | mismatches 0
```

The port and DOS refuse the same screens over that draw. Per standing rule 8 this
bounds only what the draw can reach: it holds the rate cell at 0.10, uses a
2024-01-01 loan date with whole-month frequencies, and carries no advanced
options, so the fancy `Iterate` path is untested by it. Anyone touching
`SolveRate` should still look for a case where the port's secant diverges past ±2
while DOS's `Iterate` converges; that case would be a live over-refusal.

---

## §58 — the fuzzer amortized at a rate the product REFUSES: `dos_fuzzer5_test.go` discarded the backward solvers' `converged` flag (2026-08-02)

**This is a HARNESS defect, not an engine defect, and it closes round 13's
paired-regression `NEW=1` — open three rounds and twice proposed for acceptance
as a benign advisory. It was neither benign nor an engine regression. The engine
was faithful the whole time.**

### The two failure signals, and the one the harness ignored

`SolveRate` and `SolveLoanAmount` report failure in two different ways:

| signal | meaning | DOS's equivalent |
|---|---|---|
| non-nil `err` | a DOS-faithful screen refusal (condemnation, ±200%, growth-factor guard) | `errorflag` raised by a guarded primitive |
| `converged == false`, **nil err** | `Iterate` exhausted its 20 passes with `bestp` over BOTH `halfpenny` and `acc_limit*init` | `MessageBox(...)`; `Iterate := false` (AMORTOP.pas:1489-1492) |

**DOS treats them identically.** `Iterate := false` sets `errorflag`, and
`MakeTable`'s `if (errorflag) then exit` draws **no table**.

**The shipped port also treats them identically** — `handlers.go:1260`:

```go
if !amountConverged || !rateConverged {
    writeJSON(w, http.StatusOK, AmortizationResponse{
        Error: "Computation of payment amount or interest rate did not converge."})
    return
}
result := amortization.Amortize(input)   // only converged solves reach here
```

**`dos_fuzzer5_test.go` did not.** Its backward-mode arms read

```go
case fz5ModeRate:
    v, _, err := SolveRate(in)          // <-- the converged bool, discarded
    if goSolveErr = err; err == nil { ... in.Loan.LoanRate = v ... }
```

so it amortized at a rate the product refuses to display, and then scored the
resulting table as "Go produced a schedule". This is the **same** defect the
comment immediately below those arms already describes for the `err` case ("a
failed backward solve ends the screen — no table is drawn… attributing to the
port a table no user can ever see", 2026-07-29, seed 21001). Only the `err` half
had been closed.

### Round 13's `NEW=1`, adjudicated

```
amort_oracle 291207.99 0.1209560000 2688 24 exact prepaid
  loandmy=29.5.2024 firstdmy=29.7.2024 pts=0.009110 payhard=1962.94 norate bdump
```

A near-perpetuity rate solve: `payamt*peryr/amount = 0.1617763304` is the
interest-only rate, and the true root approaches it **from below** as `n` grows,
so the terminal balance becomes astronomically stiff in the rate.

Sweeping `n` on this screen against the real oracle, holding everything else
fixed (2026-08-02):

| n | years | DOS `solvedrate` | port |
|---|---|---|---|
| 1920 | 80 | 0.1617759257 | identical |
| 1944 | 81 | 0.1617759860 | identical |
| 1968 | 82 | 0.1617760373 | identical |
| 1992 | 83 | 0.1617760809 | identical |
| 2016 | 84 | 0.1617761181 | identical |
| 2040 | 85 | 0.1617761497 | identical |
| 2064 | 86 | 0.1617761766 | identical |
| 2088 | 87 | 0.1617761995 | identical |
| 2112 | 88 | 0.1617762190 | identical |
| 2136 | 89 | 0.1617762356 | identical |
| **2160** | **90** | **did not converge** | `converged=false` ✅ |
| 2400 | 100 | did not converge | `converged=false` ✅ |
| 2688 | 112 | did not converge | `converged=false` ✅ |

**The port agrees with DOS to ten decimal places at every `n` DOS answers, and
correctly reports non-convergence at every `n` DOS refuses.** The step between
consecutive answers is ~2e-8; the port's `dosIterateRate` trace at n=2160 ends

```
FITRend bestp=0.0063180707 bestx=0.1617762497 count=20
```

against `acc_limit*init = 2e-8 × 291207.99 = 0.0058241598` — over tolerance,
hence not converged, exactly as DOS reports. The solver is right, its verdict is
right, and its verdict was being thrown away one layer up.

### What the discarded flag was protecting

When `acceptRefinedRate` rejects the refinement, `SolveRate` returns the
**unrefined closed-form estimate** alongside `converged=false`. At n=2688 that
estimate is `0.1627399644` — **9.6e-4 ABOVE the interest-only rate**, so it is not
a root at all: the loan negatively amortizes for 112 years. Amortizing at it, as
the harness did, yields

```
rows=2688   terminal balance $21,419,818,667.05   int=$21,424,810,455.64
```

on a $291K loan. That table is what the advisory was reporting, and no user can
ever see it.

### Why it presented as a §56 regression

Round 13's paired regression flagged this as `NEW` at §56 because **§56 changed
the rendering, not the solve.** Measured on identical input, pre-§56 and post-§56
return the *same* rate (`0.1627399644`, bit-identical); pre-§56 rendered it on the
nominal walk, which ends at `n+1` rows with a $0.00 balance and therefore
*passed* `retires()`, while §56's DOS-faithful exact display walk renders the
runaway. **§56 did not introduce the defect; it made a latent one observable.**
§56's rendering is independently confirmed correct here — on the four screens in
the table above where DOS answers, the port's rendered totals match DOS to the
cent (ΔInt = ΔPaid = 0.00).

### The fix

Honour the flag in the harness, exactly as `handlers.go` does — a non-converged
backward solve ends the screen and no table is drawn. Applied to both backward
arms (`fz5ModeRate` and `fz5ModeAmount`).

### Regression

`zzsec58_nonconverge_test.go`, four independent components:

- **A** — rate fidelity at all 17 values of `n` where DOS answers, to 10 dp
- **B** — `converged=false` (or an err) at every `n` DOS refuses
- **C** — the refusal boundary is monotone in `n`
- **D** — the consequence: rendering the unconverged estimate gives a >$1e9
  terminal balance, so a future change that promotes the estimate to `converged`
  fails loudly

Reverting `dosIterateCore`'s verdict to unconditional `true` fails A/B/D with
exactly the three refusal screens named and the $21.4bn table pinned; component A
is unaffected, which is the informative signature.

### Standing consequence

This is the **seventh** defect in the harness-date/harness-logic family and the
first where the harness re-implemented a *decision* rather than a *value*. The
rule it produces: **the harness must drive the same entry point the product
drives.** `handlers.go` does solve → check the flag → amortize; the fuzzer
reassembled that sequence from parts and dropped the middle step. See
`docs/harness_policy.md`.

---

## §59 — CLOSED (round 19). The terminating balloon on a schedule reaching past 26 Aug 2091: the port stopped its probe walk at the last regular payment date because a port-only date increment hit DOS's Julian ceiling (2026-08-02 opened, 2026-08-03 closed)

**Status: FIXED, round 19.** Repro reproduces DOS to the cent on a three-rung
dose-response ladder; two rungs below the ceiling are unchanged controls. Gate:
`internal/finance/amortization/zzsec59_ceiling_test.go`.

> **Two earlier root causes were published and both were wrong. They are kept
> below, marked, because each was wrong in an instructive way and because
> standing rule 11 says a claim does not stop aging just because a round
> shipped it.**

### The defect, in one paragraph

`generateFancyScheduleMode`'s "past the last regular payment date" block (the A2
block) ends the regular payment grid and re-enters the off-cycle drain loop so
that every trailing balloon and prepayment is emitted at its own date. It did
that by setting `currentDate = AddDays(lastPending, 1)`. `AddDays` is
`Julian()+n` followed by `MDY()`, and `MDY` enforces DOS's own range guard —

```pascal
if (daynumber < 0) or (daynumber > 70000) then begin x.m := errorbyte; exit; end;
                                                     { VIDEODAT.pas:373 }
```

**Day 70000 is 26 August 2091.** When the last pending extra fell past that date
the `+1` returned an error, the block dropped through to its `break`, and the
walk stopped at the last regular payment date with every trailing balloon
undrained. `TackOnFinalBalloon` then read its terminating balloon off a schedule
that had never reached `very_last`.

That `break` carried the comment **"(coverage: excluded — defensive: jump not
representable)"**. It was not defensive and it was not unreachable; it fired on
every schedule running past 2091.

### Why DOS does not have it

DOS never forms this date. The "+1 day" is a **port-only construct** for
re-entering the drain loop — DOS's `ComputeNext` (AMORTOP.pas:602-613) tests
each extra's own date against `h^.lastdate` and never round-trips a date through
`MDY` here. The port applied a *faithful* DOS guard at a site DOS does not have.

That is the transferable lesson, and it is the opposite of the usual one in this
document: the bug was not an infidelity, it was fidelity imported into a
structure DOS does not share. §47 added this ceiling precisely because the port
had it wrong in the other direction (`refdata.pas` dropped it, hiding a PV
defect worth up to 23.5%). The ceiling is right. Its reach was not.

### The fix

Replace the date-arithmetic round-trip with a flag. The A2 block sets
`currentDate = lastPending` and `drainAll = true`; `drainAll` widens the two
drain predicates from "strictly before `currentDate`" to "on or before
`currentDate`".

**It is exactly equivalent below the ceiling**, which is what makes it safe:
with `jump = lastPending+1`, the drain admitted every extra dated `<=
lastPending`, and the block was entered only when `jump > currentDate`, i.e.
`lastPending >= currentDate`. The new form states both directly.
`TestSec59DrainAllEquivalence` pins that equivalence as an internal-consistency
test (rule 10: it never drives a behaviour change; it exists so a future edit to
either predicate has to confront the claim).

### The dose-response ladder — the measurement that closed it

One screen, varying only which balloons are present, so `very_last` moves across
the ceiling and nothing else changes. All three rungs are the MERGE path; DOS
solves the term to 22 periods ending 10/12/2046 on all three.

| rung (`very_last`) | vs 26 Aug 2091 | PRE | POST | DOS |
|---|---|---|---|---|
| 10/12/2126 | **past** | 6974.06 | **-321878.17** | -321878.17 |
| 10/12/2059 | before | -28709.82 | -28709.82 | -28709.82 |
| 10/12/2046 | before | 6912.93 | 6912.93 | 6912.93 |

Measured with `cmd/goamort` built from both trees, 2026-08-03. **The two
controls are the load-bearing half**: they are what shows the change is confined
to the region past the ceiling, and per standing rule 3, a component whose
revert does not change the outcome must be said so explicitly.

This also explains §59's measured signature, which had been recorded as a
curiosity: all 27 flips sat **69 to 121 years past origination** on loans dated
2024-2026 — i.e. 2093 and later. The 69-year floor is not a property of the
generator. **It is the Julian ceiling**, and nobody recognised it for two rounds
because the number was being read as a term length rather than as a date.

### Provenance

```
amort_oracle 49726.63 0.1048540000 249 1 b365_360 exact prepaid inadv plusreg r78 usa \
  loandmy=12.10.2024 firstdmy=12.10.2025 b264=13027.49 b420=10892.59 b1224=5915.90 \
  pre=1536:376:6:61.13 targ=135.78 payhard=5994.80 noterm bdump
  DOS: balloonrow 3 date 10/12/2126 dstatus 3 amount -321878.1700 astatus 1
  Go : balloonrow 3 date 10/12/2126     amount -321878.1700 solved true tacked true
```

`cmd/goamort`'s `bdump` now prints the resolved balloon grid (round 19). Before
that, reading the port's terminating-balloon cell from a CLI required patching a
trace into the engine — which is how rounds 18b and 18c each read it. A
differential this project depends on should not need a temporary source edit to
observe.

### Severity, restated now that it is closed

The bound recorded in rounds 18/18b/18c was correct and is worth keeping: on the
repro, `payment 5994.8000 interest 82429.88 paid 132156.51` was **byte-identical
to DOS's before the fix and after it**. The schedule, both totals and the APR
never diverged; the defect was confined to one displayed grid cell. It is
nonetheless 25% of the HARD residual by count, which is the honest way to hold
both facts at once: **display fidelity, and a quarter of the remaining signal.**

---

### RETRACTED root cause #1 (round 18): "all 27 are DOS's de-activated rows"

Round 18 reported the 27 sign flips as sitting on rows DOS de-activates with
`dec(nballoons)` — display-only by construction. It inferred this from
`nballoons == user-balloon-count`, read as "inc then dec".

**Wrong.** `nballoons == nuser` is equally consistent with the MERGE path, which
never increments at all. Re-probing all 27 against `dstatus` gave **27 of 27
`dstatus=inp` — the merge path**, 0 of 27 the de-activated append path.

The disambiguating field was in the oracle's own output the whole time. What
prevented anyone reading it: the fuzzer's own failure message printed the
hardcoded string `"dstatus/astatus outp"` while reading neither field. Round 18
then read its own log back as evidence. See **harness policy R13**, and note the
severity argument was *also* backwards as a result — the merge path's row is
live, not de-activated.

### RETRACTED root cause #2 (round 18c): "the piecewise walk's horizon is `loan.LastDate`"

Round 18c corrected #1 and root-caused the defect as the probe running through
the piecewise walk, "whose horizon is `loan.LastDate` — so it stops at the
solved last PAYMENT date". It scoped the fix as **separating the regular-payment
grid horizon from the walk's terminal date**: a structural change to the walk
every fancy schedule uses, explicitly deferred as too large to land in the tail
of a session.

**Also wrong, and expensively so — it named a session's worth of structural work
that was not needed.** The A2 block had already computed the correct terminal
date (`lastPending = 10/12/2126` on the repro, confirmed by trace). It simply
could not express "one day past it". No horizon needed separating, and
`loan.LastDate` is not read for this at all.

What made #2 persuasive was that its *symptom* description was exactly right —
the walk does stop at the solved last payment date — and a true symptom with a
plausible mechanism reads like a root cause. The step that was skipped is the
cheap one: **print the failing branch.** One trace line on the A2 block gave the
answer in a single run.

Round 18c's separate finding that the naive `LastDate := veryLast` fix measures
wrong (-232,046,575 against DOS's -321,878, from 80 phantom regular rows) stands
and is unrelated — that fix is wrong for its own reasons, and it being wrong is
not evidence for #2.

### The one measured enrichment (round 18, retained)

Against the arm's own generator base rates (rule 9), `inadv` at 25 of 27 (93%)
against a 50.0% base rate — **x1.85, z=+4.4**. `plusreg` at x1.43 (z=+2.2) is a
plausible second and is mechanically implicated by the source but was never
established. Both are consistent with the ceiling mechanism rather than
competing with it: these flags select the screens whose `very_last` is driven by
a far-future user balloon.

## §60 — payoff-as-of: the port and DOS disagree on an `exact` monthly loan, and DOS returns a bare 0 on a screen it plainly has a balance for (2026-08-02, round 18b)

**Status: OPEN, found on the first run of a new instrument, NOT yet adjudicated.**

Payoff had 70 oracle comparisons, all on one loan (`100000/8%/360/12`).
`TestPayoffRandomizedSweep` (round 18b) varies amount, rate, term, both dates and
the payoff date against the same `payoff=` oracle query. First run, 600 generated:

```
ledger: 600 = compared 428 + oracle-refused 0 + go-refused 0
            + in-advance-frontier 171 + dos-zero-port-nonzero 1  | UNACCOUNTED 0
```

### (a) A real value divergence — 1 in 428

```
amort_oracle 21587.00 0.126800 84 12 loandmy=28.2.2016 firstdmy=28.3.2016 \
  exact payoff=28.1.2020
  Go  12241.0759
  DOS 12265.7378        diff -24.6619
```

$24.66 on a $21,587 loan, four years into an 84-month term. The one divergence in
the compared population carries `exact` (base rate 48.8%), which is a sample of
one and is **not** an enrichment claim.

### (b) DOS returns `payoff 0.0000` on a screen with a balance

```
amort_oracle 415792.00 0.089484 36 12 loandmy=28.7.2023 firstdmy=28.8.2023 \
  exact payoff=28.12.2023
  Go  377686.7789
  DOS      0.0000
```

Five months into a three-year loan. The port's figure is the plausible one.

**A bare 0 is AMBIGUOUS and that is why this is bucketed rather than scored.** It
is also DOS's genuine answer for a payoff date past full repayment
(`100000 0.08 36 12 … payoff=1.1.2030` → `payoff 0.0000`). Two probes narrow it
without settling it:

- the SAME screen with `b365` added gives DOS **377772.7849** — a real value;
- the same screen with `n=120` instead of `36` gives DOS **410111.0180**.

So DOS answers this loan shape at other bases and other terms, and returns 0 at
the default 360 basis with a 36-month term. That is more consistent with a bail
than with a computed zero, but it has not been traced to a line and is therefore
recorded, not concluded.

### Scope, and what the sweep deliberately does NOT cover

**Monthly only.** The first payment date is drawn one month after the loan date,
which is exactly one period only at `perYr=12`. The first run allowed all eight
frequencies and measured 27 divergences in 433 with sub-monthly enriched **×2.56**
(25 of 27, base rate 36%) — but an off-grid harness-computed first period would
produce exactly that signature too, and harness-computed dates are the family
that has produced eight defects (R2). **That population is unadjudicable as built
and is not evidence about payoff.** Widening the axis requires the first date to
come from the engine's own derivation rather than the harness.

Also excluded: the 171 in-advance non-R78 cases, which sit on the documented
settlement-shift approximation in `PayoffBalance` and are bucketed, not scored.

The sweep is opt-in (`PERSENSE_FUZZ=1`) like the other differential fuzzers, so a
real finding fails loudly when someone hunts without reddening the default gate.

---

## §61 — NOT A DIVERGENCE, BOUNDED: the solved LOAN RATE comes from different algorithms on the two sides, and the difference is DOS's stopping rule (2026-08-03, round 20)

**Status: CHARACTERISED AND BOUNDED. No fix proposed. Recorded so that the next
person to see a leaning bit-level rate difference does not open it as a defect.**

On a plain loan the two engines' `norate` answers are produced by different
procedures, and this is faithful rather than accidental:

- **DOS** has no closed form for the rate. `EstimateAndRefineRate`
  (Amortize.pas:467-491) seeds `loanrate := payamt*peryr/amount`, floors it at
  0.02 under the comment *"first guess - better high than low"*, and calls
  `Iterate` **unconditionally**. `Iterate` stops the moment its residual is
  inside `max(halfpenny 0.005, acc_limit 2e-8 x init)` (AMORTOP.pas:1422-1423,
  1485-1490) and then returns **`bestx` — the NEXT extrapolated point**, not the
  point that achieved the best residual.
- **The port** settles its own Newton on the `RepayLoan` residual and uses
  `dosIterateRate` for the convergence VERDICT (backward.go:550, the §57/§58
  gate), so on the plain arm it returns its own root.

So DOS's answer is an early stop approached from a deliberately high seed. The
consequence is a small, systematically signed difference — measured over 1,500
plain cases: 1,435 bit-identical, 65 differing, **60 of the 65 with the port
below DOS (two-tailed p=4.9e-13), worst case 83 ULP**; over round 20's three
horizon strata (4,447 cases) the worst is 227 ULP, and 63 of the 64 non-exact
cases sit in the **short**-term control band, not the long ones.

**Why it is bounded rather than open.** Repriced through the same closed form,
the two rates produce payments differing by at most **9.1e-09 of DOS's own
Iterate acceptance band** — eight orders of magnitude inside the tolerance DOS
itself converges on. Of the 65 non-exact pairs, 60 reprice to the SAME payment
double; of the five that do not, the port is nearer the handed payment three
times and DOS twice. There is no signal there and no user-visible consequence at
any display precision.

**Why it is not §48's shape.** §48 was a systematic conversion defect. Here
`solvedamount` runs the SAME `dosIterateCore` over the SAME draws and is
**bit-identical on 4,500 of 4,500** cases, because `EstimateAndRefineLoanAmount`
computes a closed form first (Amortize.pas:457) and both engines return it. The
difference is confined to the rate target, whose terminal evaluates a
`ComputeTrueRate`/`GrowthPerPeriod` chain per pass that the amount target never
touches.

**What would reopen this.** The standing gate is now the acceptance-band ratio
and the "is the port farther from the root than DOS" reversal test, both in
`zzbits_backward_test.go` (R14). A band ratio that grows toward 1, or a
significant reversal, is a real finding; a larger ULP count on its own is not.

---

## §62 — CLOSED (round 21). The last payment on a CLAMPED FEBRUARY: the plain schedule dropped its final row, because the table was bounded by a routine DOS never uses for that job (2026-08-03)

**Status: CLOSED. Fix in `internal/finance/amortization/engine.go`
(`installmentsOnPaymentGrid`); regression `zzsec62_feb_grid_test.go`; the
coverage hole it came out of is now covered by
`zzplain_differential_test.go`.**

### The defect

The plain schedule generator bounded its table by

```go
n, _ := dateutil.NumberOfInstallments(loan.FirstDate, loan.LastDate,
                                      loan.PerYr, types.OnOrBefore)
```

`NumberOfInstallments` is a correct port of DOS's `noi` (INTSUTIL.pas:936) and it
answers a **date-difference** question: how many whole periods fit between two
dates. **DOS does not use it to bound a table.** DOS walks its own payment grid
and stops on `DateComp(WhenToStop^.date, stopdate) >= 0` (AMORTOP.pas:1221).

The two answers come apart exactly when the last payment date was **CLAMPED** —
a clamped date is ON the grid but is short of a whole period from the first date:

```
loandmy=29.2.2024 firstdmy=29.3.2024, monthly, n=12
  last date, both engines           28 Feb 2025      (day 29 clamped)
  amort_oracle intutil noi 2024 3 29 2025 2 28 12 on_or_before
                                    -> n 11 last 2025 1 29
  DOS's table                       12 rows
  the port                          11 rows
```

The twelfth period's interest was never charged and its principal was folded into
row 11. Smallest repro, four rows:

```
amort_oracle 250000.00 0.072500 4 2 loandmy=29.2.2024 firstdmy=29.8.2024
  DOS   4 rows, interest 23006.41, paid 273006.41
  port  3 rows, interest 20618.84, paid 270618.84      (-2387.57)
```

**A ROUTINE THAT IS FAITHFUL AT AN UNFAITHFUL SITE IS A DEFECT.** §59's lesson,
one round later, in a different routine. The population is ordinary and entirely
in scope: any loan anchored on the 29th, 30th or 31st whose last payment lands in
a February.

### The fix, and the §54 result inside it

`installmentsOnPaymentGrid` counts steps along the schedule's own payment grid,
on RAW (year, month) fields, clamping the day with DOS's own `DaysInM`. Two
properties matter:

- it keeps the clamp's ORIGINAL job — §55's year-byte wrap, where a 300-year term
  truncates its horizon mod 256 and the period count overruns the date. A wrapped
  last date is in the past, so the grid walk stops at once, exactly as before.
- **it does not re-open §54.** The first version of the fix stepped the grid with
  `dateutil.AddPeriod`, which returns a `types.DateRec`; a DateRec cannot hold
  29 Feb 2100, normalises to 1 March, and the next step then reads March as its
  starting month and skips one. That version fixed the in-scope defect and cost
  406 out-of-scope divergences. Counting on raw fields costs nothing, because
  **a counter only ever compares the day — it never stores it.**

Measured on the round-21 probe: 4,560 screens, last payment 2097-2101, days
15/28/29/30, all four frequencies.

| | in-scope (<=2099) | out-of-scope (>2099) |
|---|---|---|
| before the fix | **66 divergences / 2,736** | 22 / 1,824 |
| AddPeriod grid | 0 | **406** |
| raw-field grid (landed) | **0** | **0** |

**This re-prices §54.** The two-date-layer cost is real only where the port must
STORE a date DOS can form and Go cannot. Where it only needs to COUNT or COMPARE,
DOS's calendar is available through `dateutil` at field level and the §51/§54
refactor is not required to get the right answer. §54 stays deferred; its
measured surface is smaller than the round-17 quantification implies, because
part of it was never a storage problem at all.

### Why no instrument had seen it — and the bigger finding

`dos_fuzzer5_test.go` **abandons a case that draws no advanced option**
(`skippedPlain`, :1557) before the oracle is ever spawned, and its own comment
says so, adding that plain-loan fidelity is covered by `zzmetafuzz_test.go` and
the unit suite. **That was a coverage claim and it was not backed.**
zzmetafuzz's forward corpus is five hand-written screens on days 1, 8, 15 and 29,
and no committed case put a day-29 anchor's last payment on a February.

So the port's plain path — the simplest thing the product does — **had no
randomized differential at all**, and every headline amortization figure this
project has published came from a generator that excludes it. §62 lived in that
gap for the life of the port. The standing residual is unchanged by this fix
(42 HARD in 26,857, 1 in 639) precisely because the instrument that produces it
cannot see plain loans.

`zzplain_differential_test.go` (round 21) closes the hole. Post-fix, three seeds,
18,000 generated:

```
15,639 compared, UNACCOUNTED 0
  in-scope <=2099   13,736 compared   0 divergences
  out-of-scope      1,903 compared    1 case (3 signals), horizon 2149
```

and against the unfixed tree the same instrument reports **30 in-scope signals
over 11 cases in 4,560 in-scope screens on its first seed alone — about 1 in
415.** That is the incidence §62 had on the plain path while the published rate
described a different population.

---

## §63 — NOT A COMPUTATION DEFECT, IN SCOPE: DOS leaves its de-activated terminating balloon standing on the last grid row and the port folds it in (2026-08-03, round 22)

**Status: CHARACTERISED. Display fidelity, in scope, awaiting a product
decision. Detected and CLASSIFIED by `zzplain_differential_test.go`
(`SIG=ADVISORY:plain_terminating_balloon_final_row`) so it can never again be
counted as three ordinary row-value divergences.**

This is §59's mechanism, one round later, on the surface §59 could not reach.

Where the regular payment does not quite amortize the loan over its stated term,
DOS computes a terminating balloon for the shortfall and then **de-activates it**
— `dec(nballoons)`, Amortize.pas:1040-1088, which is why the oracle duly reports
`nballoons 0` on these screens. DOS's last GRID row therefore shows the REGULAR
payment's own interest/principal split and leaves the balloon balance standing.
The port folds the same balloon into that row's payment and shows a zero balance,
and emits the warning *"the final payment includes an implied terminating
balloon."*

The measured repro, from the round-22 standing range (seed 21035):

```
amort_oracle 51673.00 0.095436 121 24 loandmy=31.7.2024 firstdmy=31.8.2024

  DOS   row 120  int 4.28  prin 536.69  bal 538.82        (120 rows, balance left)
  port  row 120  amt 1079.78  int 4.2767  balance 0.0000  (120 rows, retired)
```

**Everything computed agrees, to the cent, on both sides:**

```
payment    540.9623  =  540.9623
interest    13781.29 =   13781.29
paid        65454.29 =   65454.29
nperiods         121 =        121
last date  8/31/2029 =  8/31/2029
row count        120 =        120
```

The divergence is confined to what the final row DISPLAYS: DOS shows the split
of the regular payment and a $538.82 balance carried; the port shows a $1,079.78
payment and nothing carried. **The two engines disagree about presentation, not
about money** — the same amount is collected on the same day either way.

**Why it is recorded rather than fixed.** §59's rule governs: establish whether a
divergent cell is USED before assigning severity. It is used — the user reads the
last row of the table — so this is not nothing. But it is not a computation
defect, and which rendering is *wanted* is a product question, not a fidelity
question that the oracle can answer. Two positions are defensible and Nate/the
client should pick one:

- **Match DOS exactly**: the port shows the regular split and leaves the balance
  standing, matching the original's grid line for line.
- **Keep the port's rendering**: it is arguably the more useful of the two — it
  tells the user what they actually pay on the last day — and the port already
  emits an explicit warning naming the implied balloon.

**Severity and frequency.** IN SCOPE (horizon ≤2099). Measured on the round-22
standing ranges; the count is reported per arm by
`testplan/harness/analyze_plain_arm.py` under *"terminating-balloon final rows"*.
It is EXCLUDED from the per-row value counters, because counting one
presentation difference as `rowbal` + `rowprin` + possibly `rowint` would inflate
a per-row divergence rate with a class that has no arithmetic content — the
round-18 tack-tolerance mistake in a new place.

**What would reopen this as a real defect.** Any case where the terminating
balloon changes a TOTAL, the payment, the horizon or the row count. The
instrument's signature requires all four of those to agree before it classifies a
case here, so a balloon case that moves any of them falls through to the ordinary
counters and fails.

---

## §64 — NOT A DIVERGENCE, BOUNDED: on a plain loan past DOS's year-byte ceiling both engines refuse and the two refusal SENTENCES differ (2026-08-03, round 22)

**Status: CHARACTERISED AND BOUNDED. 0 in-scope occurrences over the round-22
standing ranges. Recorded so the next person to measure the plain refusal bucket
does not open it as a defect.**

Round 22 was the first round to run the port at all on the ~14% of plain-loan
draws the ORACLE refuses (see R16). The refusal SET turns out to pair exactly —
every screen DOS refuses, the port refuses, with zero cases answered by either
side alone. The refusal MESSAGE does not always pair:

| DOS says | the port says | count (arm A, 48,000 draws) |
|---|---|---|
| "Computation of payment amount or interest rate did not converge." | the same sentence | 2,625 |
| "There must be at least two regular payments." | "1st Pmt Date is after Last Pmt Date. Make sure 1st Pmt Date comes first, or clear one of the two dates and let Per%Sense derive it." | 3,728 |

**The mechanism.** Every case in the second row has an arithmetic horizon past
**2155** — DOS's `daterec` year is a byte based at 1900 (§55), so a series long
enough to run past it wraps, and DOS's last date lands BEFORE its first. DOS
reports that state as "fewer than two regular payments"; the port, which
reproduces the same wrap faithfully (§55, closed round 12), reports it as a date
ordering problem. Both engines are describing the same wrapped calendar in
different words.

**Why it is bounded.** Measured over the round-22 standing ranges, the differing
class is **entirely** past 2155 and **0 cases fall inside the client's ≤2099
comparison boundary**. The instrument does not assume this — it counts
`refuseMsgDifferInScope` and logs
`SIG=ADVISORY:plain_refusal_message_differs_in_scope` for any case that is in
scope, so if a future draw finds one it is loud rather than absent.

**What would reopen this.** A single in-scope occurrence. That would mean the two
engines disagree about *why* a reachable screen is invalid, which is a different
and more serious thing than disagreeing about an unrepresentable one.

---

## §65 — RESOLVED AS A HARNESS DEFECT, AND IT REBASED THE PROJECT'S HEADLINE RATE: DOS's "internal error" is an ADVISORY DIALOG, and the oracle DRIVER was discarding the table the engine had already built (2026-08-04, round 32)

> **⚠️ READ THIS BANNER BEFORE THE SECTION BELOW IT.** Everything from the old
> heading down to the §66 heading was written in rounds 22-31 on a premise that
> is FALSE: that DOS refuses these screens. **It does not.** The advisory
> subclass's account is at the END OF THIS DOCUMENT (after §69), under
> **"ROUND 32 — THE ADVISORY SUBCLASS WAS OURS. CLOSED AS A HARNESS DEFECT."**
> — it is appended there rather than spliced in so the superseded reasoning stays
> readable in the order it was written. The `noterm` subclass's
> round-31 root cause (the `MonthSetFromString` over-read) is unaffected and
> still correct.

### (superseded heading) OPEN, NEEDS ADJUDICATION: DOS returns its own INTERNAL ERROR on in-scope screens and the port answers them (2026-08-03, round 22)

**Status: OPEN. Newly visible — the bucket it lives in had never asked the port
anything (R16). Counted and reported by `dos_fuzzer5_test.go`
(`SIG=ADVISORY:go_solved_dos_internal_error_in_scope`); deliberately NOT scored
HARD pending Nate's decision, because the correct behaviour is a product
question and not a fidelity question.**

### What was measured

Round 22 gave `dos_fuzzer5_test.go`'s `date-horizon` bucket the asymmetry check
that `refused` and `non-converged` have had since round 16. On seed 50100 at
`FUZZ_N=400`:

```
date-horizon bucket, 34 cases
  port produced a schedule in                    7
  of those, IN SCOPE (port horizon <=2099)       5
  of those, DOS's own INTERNAL ERROR             5
  in-scope, non-internal-error remainder         0
```

All five in-scope cases return the same DOS message, and it is not a calendar
message:

```
ENGINE ERROR: Internal error - last payment not found.  Please contact Ones & Zeros.
```

Reproducing command for one of them (the rest are in the round write-up):

```
amort_oracle 176785.81 0.0309170000 50 2 b365 prepaid plusreg r78 usa \
  loandmy=28.12.2025 firstdmy=28.6.2026 mor=138 b180=40876.05 b204=34715.92 \
  b222=6505.97 pre=48:114:12:67.02 pre=192:2:4:160.67 adj=84::7146.68 \
  adj=162::7253.06 adj=234:0.0864350000: targ=115.44 pts=0.013454 \
  payhard=5106.05 non lastdmy=28.12.2050 bdump
```

### The second finding, which is about the instrument

**The `date-horizon` bucket holds two different claims and its name only makes
one of them.** Membership is decided by DOS's message containing `julian`,
`bad date` **or** `last payment not found`. The first two are DOS hitting a
REPRESENTATION limit — its Julian routine, its 1900-based year byte (§55) — and
a port with a wider calendar answering there is expected, understood and out of
scope. The third is DOS reporting that *it failed*, in a sentence that ends
"Please contact Ones & Zeros". Those are not the same statement, and they have
shared a bucket since the bucket existed.

This is R15's shape a third time in two rounds: a label that is a coverage claim,
asserted and never audited.

### Why it is not scored HARD today

Matching DOS's *bugs* is the project's stated policy, and matching DOS's
*internal errors* is arguably a different thing: an internal error is the
original telling the user it could not do the job, not the original judging the
screen invalid. Three positions are defensible:

1. **The port should refuse too.** Maximum fidelity — the user sees the same
   behaviour, including the failure.
2. **The port should answer, as it does now.** The port is strictly better here,
   and no user is served by reproducing an internal error.
3. **The port should answer and SAY SO** — produce the schedule with an advisory
   noting the original could not compute this screen.

Making it HARD before that decision would turn the standing gate red on a class
nobody has adjudicated, which is the round-17 mistake (naming a frontier from an
unread numerator). The in-scope NON-internal-error remainder — DOS refusing a
reachable screen for a stated calendar reason while the port answers — DOES fail
hard, and is currently **0**.

### Verified in both directions

The HARD branch was proved reachable by disabling its sub-classifier in a
throwaway build: the same five real in-scope cases then fired
`SIG=HARD:go_solved_dos_date_horizon` and the seed failed. The branch works; it
is the classification that holds it back, not an unreachable condition.

### A SECOND subclass, found by the same detector, and this one DOES fail hard

The `date-horizon` bucket is reached two ways. The first is DOS's error TEXT
(above). The second is a valid-looking dump whose horizon cells are garbage —
`d.payment != 0 && d.interest == -1 && d.paid == -1` with `nPeriods == 0` or a
last date beginning `-`. Over the standing range 50100-50139 the new check found
**two** such cases where the port answered with an IN-SCOPE schedule, and they
fire `SIG=HARD:go_solved_dos_date_horizon` as designed:

```
amort_oracle 156681.73 0.1004060000 216 12 exact inadv loandmy=13.10.2025   firstdmy=13.12.2025 mor=21 pre=67:12:6:209.54 targ=320.47 skip=1,7   payhard=1638.01 noterm bdump
    DOS: lastdate -88/0/1900 nperiods 0        <-- month -88, day 0, year 1900

amort_oracle 256824.30 0.0803510000 252 12 b365_360 exact prepaid inadv usa   loandmy=2.10.2024 firstdmy=2.12.2024 mor=84 b89=47129.32 b179=57668.03   b194=49297.23 pre=86:289:26:117.75 pre=15:2:2:2401.01 targ=52.59 skip=6   payhard=2259.36 noterm bdump
```

Both are `noterm` — DOS's BACKWARD TERM SOLVE — and both are ordinary in-scope
horizons: the first is 216 monthly payments from 2025, ending 2043. **DOS's term
solve returns a nonsense date on an 18-year monthly loan** (heavily stacked:
moratorium, skips, a target and a hard payment), and the port returns an answer.

This is not the calendar ceiling and it is not out of scope. It is the same class
as `go_solved_dos_refused` on a surface where nobody had looked, and unlike the
internal-error subclass it is scored HARD, because DOS's failure here has no
representational excuse.

**Note for the record:** round 20 instrumented post-2100 backward solves and
found them clean over 17,031 paired solves. That instrument
(`zzbits_backward_long_test.go`) pairs solves DOS COMPLETES. This subclass is
solves DOS does NOT complete, on IN-SCOPE horizons, which that instrument by
construction cannot see. The two results do not conflict; they are about
different populations, and the second one had no instrument until now.

### ROUND 31 — the `noterm` subclass ROOT-CAUSED. It is not a term solve at all.

**It has nothing to do with the term solve, the horizon, the moratorium or the
stacking. It is `skip=`.**

Ablating the first repro one option at a time leaves exactly one culprit:

```
… mor=21 pre=67:12:6:209.54 targ=320.47 skip=1,7 payhard=1638.01 noterm
                                        -> interest -1.00  solvedterm 0  last 1900--88-0
… mor=21 pre=67:12:6:209.54 targ=320.47          payhard=1638.01 noterm
                                        -> interest 240974.48  solvedterm 255  last 2047-2-13
```

and dropping `noterm` does **not** help — the same screen with the term SUPPLIED
also returns `lines 0`. So DOS never reached a solver; it refused the screen.

**The refusal moves when the input does not.** Same month set, different
spelling, on the same screen:

```
skip=1,7   -> lines 0            skip=01,07 -> lines 218
skip=6     -> lines 0            skip=06    -> lines 218   (the second repro)
```

#### The mechanism

`MonthSetFromString` (Amortize.pas:149-181) **reads one byte past the end of its
argument**:

```pascal
      ws := s[i];
      inc(i);
      if (s[i] in digitset) then ws := ws + s[i]
      else if (s[i] = '-') then dec(i);
      n := round(value(ws));
      if (n >= 1) and (n <= 12) then ... else exit;    { exit returns FALSE }
```

After consuming the LAST digit it evaluates `s[i]` at `length(s)+1`. Round 31
added `scripts/build_trace_oracle.sh -mode msf`, which prints what the parser
actually reads; on `skip=1,7` it prints

```
MSF enter len=3 s=[1,7]
MSF tok i=2 len=3 ord=44 ws=[1] n=1
MSF tok i=4 len=3 ord=53 ws=[75] n=75      <-- i=4 > len=3; ord 53 is '5'
MSF bad n=75 -> FALSE
```

Month `7` and a stray `'5'` were scored together as month **75**, out of range,
parse FALSE. `FirstPass` (Amortize.pas:253-255) then calls `RecordError`, which
sets `errorflag` and — under `scripting` — returns **with no MessageBox**;
`MakeTable` exits at `if (errorflag) then exit` having emitted nothing:
`lines 0`, totals `-1`, `nperiods` 0, and `h^.lastdate.m` still holding the
`unkbyte` sentinel FirstPass wrote at :244, which prints as **-88** because
`daterec.m` is signed. **`-88/0/1900` was never a computed date. It is DOS's
"unknown date" marker, untouched.**

`s` is a **by-value `str15` parameter**, so the byte lives in the callee's own
stack frame, not in `skp^.skipmonths`. This was established by a **negative
control that failed**: zeroing the caller's `skipmonths` tail changed nothing,
and so did filling it with `'9'`. R22's lesson in a new costume — a by-value
parameter is a different variable, and here it is a differently-*sized* one.

A string whose last number has TWO digits cannot over-read: the second digit
lands exactly on `length(s)` and the lookahead never fires. That is the whole
difference between `1,7` and `01,07`.

#### Why this is not a divergence, and what was done

The verdict is not a function of the screen, so **the oracle cannot be an
authority on it**, and the port cannot be asked to reproduce it — there is
nothing deterministic to reproduce. This is round 30's lesson (R13 applies to the
ORACLE) reaching a second instrument: the oracle was printing a refusal derived
from a byte it never read from its arguments.

`amort_oracle`'s `skip=` handler now runs the token through **`PadSkipMonths`**,
which two-digits every month (`1,7` → `01,07`, `5-7` → `05-07`). The month set is
identical; only the spelling changes; no DOS source is touched; the PORT still
receives whatever string the fuzzer generated. Anything outside the grammar
`[0-9,-]` is passed through verbatim.

#### The evidence, both directions

`testplan/harness/audit_skip_overread.py` compares the PRE and POST oracles and
goamort over 40 base screens × 15 skip strings = **600 cases**:

| bucket | count |
|---|---|
| **FIXED** — PRE refused, POST answered | **10** |
| **SAME** — identical verdict and identical output | **590** |
| **CHANGED** — both answered, different output | **0** |
| **BROKE** — PRE answered, POST refused | **0** |
| **goamort MATCH / MISS** on every answered case | **485 / 0** |

and on the two named repros the port's answer is the oracle's answer to the cent:

```
repro 1  goamort  interest 255167.47 paid 411849.20
         oracle   interest 255167.47 paid 411849.20   (skip=01,07)
repro 2  goamort  interest 268392.54 paid 525216.84
         oracle   interest 268392.54 paid 525216.84   (skip=06)
```

**The port was right on both of §65's HARD cases.**

#### Blast radius on the standing measurement

`dos_fuzzer5_test.go:1510` draws its skip string from
`{"6", "1,7", "5-7", "2,8,11", "11-12", "1,3,5"}`. **Four of the six end in a
single digit** and are therefore subject to the over-read; only `2,8,11` and
`11-12` are structurally safe. Every `skip=` case measured before round 31
carried an oracle verdict that could flip on a stack byte — which is why this
class looked random and small at the same time.

The over-read has a second, quieter mode that the schedule comparison WOULD have
scored: when the stray byte forms an in-range month (visible `"1"` plus a stray
`'2'` parses as month **12**), DOS silently amortises a DIFFERENT month set and
answers. The 600-case audit found **0 CHANGED**, so no such case is present in
this sample — but it is the reason the correction is worth more than the two
signals it closed.

### What is needed to close it

**One thing now, not two.**

1. **A decision from Nate on the internal-error subclass** — which of the three
   positions above the port should take — and then either a fix plus a
   regression test, or a written statement in `CLAUDE.md` that answering through
   DOS's internal errors is intended, with this section as its evidence.
2. ~~A root-cause investigation of the `noterm` garbage-date subclass~~ —
   **DONE, round 31. Not a port defect; the oracle is corrected and gated by
   `audit_skip_overread.py --selfcheck`.**

### Where the reproducing commands come from

The per-case repro lines are behind `PERSENSE_FUZZ_FLAKEDUMP=1` for the
internal-error subclass, because an ungated per-case line changes default harness
output and `paired_regression.sh` — which keys on `grep -oE "amort_oracle .*"`
across the whole run — reports every one of them as a regression (measured this
round: NEW 195, then NEW 177, against an engine proved byte-identical). The HARD
subclass stays ungated: a hard failure that only appears under an env var is R12
wearing a different hat.

---

## §66 — CLOSED (round 28). The adjustment rate/payment solved by `Re_Amortize` differed. BOTH arms are now root-caused and fixed: AO6 (blank RATE) in round 25, AO7 (blank AMOUNT) in round 28 (2026-08-04, rounds 24-25-28)

**Status, round 28: BOTH arms are CLOSED. All five §66 cases (c1, c3, c4, c5,
c7) are free of material divergence; c3, c4 and c5 retain only §63's
terminating final row.**

*(Round 25 status, superseded: the RATE arm closed, c3 still open.)*

### ROUND 28 — the AO7 arm, and it was the SAME defect one branch over

**One line of Pascal, two port call sites, one of them fixed.** Round 25 closed
AO6 by clamping the segment sub-loan's regular-payment bound to the caller's
live `h^.lastdate`, because DOS's sub-walk keeps testing every candidate regular
date against that global:

```pascal
    balloonpos := 1;
    if (xsource > 0) then
      begin
        balloonpos := DateComp(nextextra.date, date);
        if (DateComp(date, h^.lastdate) > 0) then
          balloonpos := -1;      { AMORTOP.pas:606 — the regular row is DROPPED
                                   and the pending extra is taken instead }
```

`solveSegmentRate` got that clamp. `solveSegmentPayment` — the AO7 twin, reached
from `Iterate(p, usap, Payment.date, t, d, til_adj)` at AMORTOP.pas:1577 — did
not, and it had the identical latent hole.

**The caller was already right; the callee threw the answer away.** engine.go
computes `segN` from `NumberOfInstallmentsRaw(adjDate, adjLastDate, …)` — the
snap-aware count that is the whole of §53 — and passes it in. When the phantom
snap has moved the sub-walk's first row off the boundary date,
`solveSegmentPayment` calls `segmentPeriods()` and OVERWRITES it. That recount
walks `while dt < loan.LastDate`, i.e. it is a CEILING: it counts up to the first
grid date at or past the bound. Correct for an ON-GRID bound; one whole period
too many for a bound the `INTSUTIL.pas:1018` month-end snap has pushed OFF-grid,
because the true last regular date is then strictly below it and the ceiling
steps past.

**c3, measured against the real DOS engine** (`scripts/build_trace_oracle.sh
-mode cn`, whose CN lines are DOS's per-row `ComputeNext` record, diffed against
the port's `DPTRACETERMROWS=1` TERMROW lines):

```
amort_oracle 393752.15 0.0477520000 26 2 prepaid usa loandmy=29.4.2023 \
  firstdmy=29.2.2024 mor=70 b94=82687.19 b106=93767.40 b118=59796.70 \
  pre=10:89:6:507.99 adj=34:0.0762230000: adj=112:0.0437960000:15897.68 \
  targ=3910.14 pts=0.030192 payhard=26754.42 non lastdmy=29.8.2036
```

| | value |
|---|---|
| screen `lastdmy` | 2036-08-29 |
| `h^.lastdate` after the snap (day 29 → 31) | 2036-08-31 |
| caller's `segN` | **21 — correct** |
| `segmentPeriods()` recount | **22** |
| sub-loan bound the port then derived | 2037-02-28 |
| DOS's Iterate sub-walk | **76 rows**, 2026-04-29 → 2038-10-29 |
| the port's terminal walk | **76 rows**, same range |

**The row COUNT is identical on both sides, which is why counting rows alone
finds nothing** — the walk horizon is `very_last` = 2038-10-29, set by the
prepayment series, and it was never the divergent bound. Rows 0-64 are
byte-identical. Row 65 is the tell:

| row | DOS | port (pre-fix) |
|---|---|---|
| 64 | 2036-12-29 pay 507.99 | identical |
| **65** | **2037-02-28 `bpos=-1` pay 507.99** (extra only) | **2037-02-28 pay −18162.04** (a 22nd REGULAR payment) |
| 66-75 | pay 507.99 | pay 507.99, balance offset by 18670.03 |

That single row moved the terminal at the SHARED seed, and the two secants —
which agree step for step — converged on honestly different roots: DOS
**−21236.435395** against the port's **−20178.789827**. On the displayed schedule
that is row 17, DOS 26746.59 against the port's 25688.94.

**The fix** derives the sub-loan's `LastDate` explicitly and clamps it to the
caller's live `h^.lastdate`, mirroring `solveSegmentRate` line for line. *Clamp,
never extend* — a derived date SHORTER than `h^.lastdate` is the port's own
reconstruction of a walk DOS bounds by `very_last`, and lengthening it would
re-introduce the extra row from the other side; §53 is the case where the snap
legitimately pushes the global PAST the last scheduled payment and DOS does emit
a regular row there, so the pristine `loan.LastDate` is the wrong bound.

**Post-fix c3 is 90/90 rows with no date divergence and the first >2c at the LAST
row — it has fallen back to c4/c5's §63 class.** Verified BOTH DIRECTIONS with
distinct binary md5s (pre `7df1f1ce`, post `fbccdb19`);
`attribute_seven.py --assert` FAILED on the pre-fix binary (`c3: first >2c
divergence 17, expected 90`) and PASSED 7/7 on the post-fix one.

*(Round 27's caller-side hypothesis — that `engine.go` was missing
AMORTOP.pas:1575's snap — was tested, moved zero bits, and was reverted; the
callee already applied it. That negative result is standing rule R20, and it is
what redirected this hunt from the sub-walk's START to its BOUND.)*

### ROUND 25 — the root cause, and it was never the solver

Round 24 filed this as a basin-selection problem: "two solvers that both
converge land on different roots." **That reading is retracted.** Traced side by
side on c4 — DOS through an instrumented `-mode itr` oracle build
(`scripts/build_trace_oracle.sh`, ITR lines), the port through `DPTRACE=1`
(FITR lines) — the two secants are *identical in every respect except the number
they are handed at the seed*:

```
DOS   ITR0  seedx=0.0671720000  p= 2237.0538681843   -> bestx=0.0646819059
port  FITR0 seedx=0.0671720000  p=-25540.5339966426  -> bestx=0.0930233351
```

Same seed, same divergence brake, same acceptance test, same `bestx` timing,
same iterate count pattern. The RESIDUAL FUNCTION differed, so the roots
differed honestly. **A solver that agrees step for step and still returns a
different answer is not a solver defect — look at what it is being asked.**

Localised to a row with a per-row dump of both terminal walks (DOS's `-mode cn`
CN lines against the port's new `DPTRACESEGROWS=1` SEGROW lines). Rows 0-69 of
the sub-walk are byte-identical. At row 70:

```
row 69  DOS 2037-03-29 pay    323.93   PORT 2037-03-29 pay    323.93   identical
row 70  DOS 2037-04-29 pay    323.93   PORT 2037-03-31 pay  22916.18   <-- extra
```

**The port emitted an ELEVENTH regular payment where DOS emits ten.** The extra
row falls on 2037-03-31, past the screen's own `lastdmy=28.2.2037`.

**The DOS rule is one line, in `Paymenttype.ComputeNext` (AMORTOP.pas:602-613):**

```pascal
balloonpos := 1;
if (xsource > 0) then
  begin
    balloonpos := DateComp(nextextra.date, date);
    if (DateComp(date, h^.lastdate) > 0) then
      balloonpos := -1;          {the regular row is DROPPED and the pending
                                  extra is taken at its own date instead}
```

Once a candidate regular date passes `h^.lastdate`, the regular stream is dead
and only extras are emitted. `Re_Amortize`'s RATE branch calls
`Iterate(p, usap, payment.date, nextpayment.date, h^.loanrate, til_adj)`
(AMORTOP.pas:1523), which re-enters `RepayFancyLoan` **on the same globals** —
and nothing on that path assigns `h^.lastdate`. So DOS's sub-walk is still
bounded by the parent screen's last payment date.

The port's `solveSegmentRate` built its sub-loan a *fresh* `LastDate` from
`remaining` — a port-only reconstruction with no DOS analogue at this site — and
on c4 it landed one semi-annual period late. The port's own faithful
implementation of :606 (engine.go, the `adjLastDate` reader) was then being fed
the wrong bound. **The bug was not in the ported rule; it was in the value
handed to it.**

**The fix** (`internal/finance/amortization/fancybisect.go`): clamp the derived
sub-loan `LastDate` to the caller's LIVE `h^.lastdate`, threaded in as a
parameter. That bound is **`adjLastDate`, NOT `loan.LastDate`** — §53's whole
point is that §50's VAR-parameter snap (AMORTOP.pas:1547) can push the global a
month PAST the pristine last payment, and DOS then legitimately emits a regular
row there. Clamping to the pristine date would have silently reverted §53. The
clamp only ever shortens; lengthening would re-introduce the extra row from the
other side.

After the fix the port's secant on c4 reproduces DOS's to every printed digit,
seed residual included.

### Measured effect (per-row differential, the seven in-scope HARD cases)

| case | arm | first >2c divergence BEFORE | AFTER | row counts before → after |
|---|---|---|---|---|
| c1 | AO6 | 138 | **none** | 211/213 → **211/211** |
| c4 | AO6 | 64 | **163 — the LAST row, §63 only** | 163/163 |
| c5 | AO6 | 228 | **366 — the LAST row, §63 only** | 366/366 |
| c7 | AO6 | 198 | **none** | 361/370 → **361/361** |
| c3 | AO7 | 17 | **90 (last row) — CLOSED round 28**, §63 residue only | 90/90 |
| c2 | — | none | none | — |
| c6 | — | 2 | 2 — §67, unrelated | — |

The two row-count gaps closing is the confirmation: a schedule run at a
different rate retires on a different date, and both now retire on DOS's.

### What WAS still open after round 25 — the AO7 (blank AMOUNT) arm, c3 (CLOSED round 28, see above)

The amount branch does NOT reduce to the same cause. Its `Iterate` call is
preceded by a horizon computation that mutates its own argument
(AMORTOP.pas:1573-1575):

```
RA amt pre   t=2026-04-29  fd=2024-02-29  ld=2026-02-28  seed=27582.331985
RA amt post  t=2026-08-31  n=6                    <-- t SNAPPED by the VAR param
RA amt solved d=-21236.435395
```

`NumberOfInstallments(h^.firstdate, t, h^.peryr, on_or_after)` takes `t` as a
VAR parameter and rounds the lookahead date 2026-04-29 **forward** onto the
`h^.firstdate` grid, landing on 2026-08-31 (month-end carry, INTSUTIL.pas:1018).
That snapped `t` is what `Iterate` then solves to. Start there; the trace above
is reproducible with `scripts/build_trace_oracle.sh -mode ra`.

### The regression gate

`python3 testplan/harness/attribute_seven.py --assert` pins the first material
divergence index for all seven cases. **Verified both directions, 2026-08-04:**
against the pre-fix tree it fails with exactly `c1 138`, `c4 64`, `c5 228`,
`c7 198` and both row-count gaps; against the fix it passes 7/7.

---

### The round 24 filing, retained for the record

**Status as of round 24: open, root cause LOCALISED to one routine and proven by
ablation, not yet explained line by line.**

### What it is

On a screen carrying an `adj=` adjustment whose rate or payment is left BLANK,
DOS's `Re_Amortize` (AMORTOP.pas:1499-1613) solves the missing cell by calling
`Iterate(..., til_adj)`. The port solves a **different** value. Every row from
the adjustment date onward then diverges, in the same direction, and the gap
persists to the end of the table.

Two branches, both ending in the same `Iterate`:

| branch | screen shape | what DOS solves | AMORTOP.pas |
|---|---|---|---|
| **AO6** | `adj=<n>::<amount>` — amount given, rate blank | the implied RATE | :1521-1535 |
| **AO7** | `adj=<n>:<rate>:` — rate given, amount blank | the new PAYMENT `d` | :1547-1590 |

### The evidence — ablation, one token at a time

Dropping the single blank-celled `adj=` token from each case and re-running the
per-row differential (`testplan/harness/attribute_seven.py`):

| case | branch | DOS's solved cell | first >2c divergence, as-is | with the `adj=` token dropped |
|---|---|---|---|---|
| c1 | AO6 | rate **0.1465031874** @ 11/19/2039 | row 138 | **NONE** (row counts 213/213, were 211/213) |
| c4 | AO6 | rate **0.0646819059** @ 2/29/2032 | row 64 | last row only — **§63**, a known class |
| c5 | AO6 | rate **−0.1207597014** @ 6/30/2036 | row 228 | last row only — **§63** |
| c7 | AO6 | rate **−0.2107495046** @ 10/26/2034 | row 198 | **NONE** (row counts 406/406, were 361/370) |
| c3 | AO7 | payment **−21236.44** @ 2/28/2026 | row 17 | last row only — **§63** |

The adjustment date in every case is the row IMMEDIATELY BEFORE the first
divergent row, computed by DOS's own `monthsAfter` anchoring on the loan date —
so the divergence begins on the first row that runs under the newly solved cell,
with no gap.

**The two row-count differences are the same defect, not separate ones.** c1
(DOS 211 / port 213) and c7 (DOS 361 / port 370) both go to equal counts when the
adjustment is removed. A schedule run at a different rate retires on a different
date.

### The direction is uniform

In all four AO6 cases the port's solved rate is **algebraically higher** than
DOS's — including the two where DOS's own answer is a large NEGATIVE rate
(−12.1%, −21.1%) and the port lands closer to zero. Implied from the rows:
c4 DOS 0.06468 vs port ≈0.09302; c1 DOS 0.14650 vs port ≈0.15049.

That DOS accepts −21% at all is the point of contact with **§61**: DOS's
`Iterate` accepts on `bestp < 0.005` **OR** `bestp <= 2e-8 × loan amount` and
returns `bestx` (AMORTOP.pas:1489). On a screen where the objective is flat or
multi-rooted near a negative rate, two solvers that both "converge" land on
different roots and both are internally consistent. §61 established this for the
loan-rate cell and bounded it as NOT a divergence; §66 is the same stopping rule
reached through a different caller, and here it is NOT bounded — it moves every
subsequent row of a displayed schedule.

### Why this matters more than its case count

Of the seven in-scope HARD cases on the standing ranges, **five are §66** and a
sixth (c2) is already assigned to §61's backward-solve neighbourhood. Six of
seven therefore reduce to DOS's `Iterate` acceptance and return semantics. The
seventh (c6) is unrelated — a semi-monthly date-grid divergence, see below.

For the exit criterion's mechanism clause this is the difference between "seven
unattributed signals" and "one mechanism plus two singletons."

### What is NOT yet established

- **Which** part of the port's `Iterate` differs — seed, bracket, step,
  acceptance test, or the returned `bestx` — has not been read line by line. Rule
  5 applies: fuzzing has located it, only reading will explain it.
- Whether DOS's answer is *right* in any external sense. On the negative-rate
  screens it may well not be; the port's may be the better number. **That is not
  the question this project answers** — the question is whether the port
  reproduces DOS, and it does not.
- Whether §66 also fires outside the standing ranges, and at what rate. No base
  rate over compared cases exists yet for `adj=` screens with a blank cell.

### The next action *(round 24's, now DONE — and its instrument note was wrong)*

Instrument both `Iterate` implementations on c4 and compare seed, every iterate,
the acceptance test that fires, and the returned value. **CORRECTION, round 25:
`DPTRACE=1` does NOT give DOS's secant — it is the PORT's env var, and the port
emits `FITR` lines. DOS's side needs an instrumented oracle,
`bash scripts/build_trace_oracle.sh -mode itr`, which emits `ITR` lines to
stderr from `/tmp/oracletrace/amort_oracle_trace`.** Done in round 25; see the
top of this section for the answer.

### Repro

```
amort_oracle 284917.49 0.0671720000 28 2 b365_360 exact prepaid plusreg r78 \
  loandmy=31.7.2023 firstdmy=31.8.2023 mor=73 pre=55:144:12:323.93 \
  adj=103::22916.18 pts=0.005528 payhard=20219.51 non lastdmy=28.2.2037 adjdump
```

→ `adjrow 1 date 2/29/2032 rate 0.0646819059 ratestatus 1 amount 22916.180000`.
The port, implied from rows 64-65 of the same screen, is running ≈0.09302.

`python3 testplan/harness/attribute_seven.py c4` prints the aligned per-row
differential. Read `GOAMORT_ALLROWS` and instrument defect #15 in
`docs/harness_policy.md` before comparing any two row sets by index.

---

## §67 — OPEN, IN SCOPE: on a SEMI-MONTHLY screen whose first payment date is the 31st, DOS moves the first payment to the 1st of the next month and the port does not (2026-08-04, round 24)

The seventh in-scope HARD case, and the only one of the seven that is not §66 or
§61.

```
amort_oracle 294350.23 0.1390570000 312 24 b365_360 plusreg r78 \
  loandmy=31.8.2025 firstdmy=31.10.2025 targ=503.52 pts=0.034335 \
  payhard=2477.43 non lastdmy=30.9.2051
```

24 payments/year. The typed first payment date is 31 October 2025. DOS's grid
runs **11/1/25, 11/16/25, 12/1/25, 12/16/25, …** — it moves the typed date
forward to the 1st. The port emits **10/31/25** and then joins DOS's grid at
11/16/25.

| row | DOS | port |
|---|---|---|
| 1 (settlement) | 8/31/25 pay 10106.52 int 10106.52 | identical |
| **2** | **11/1/25** pay 7552.83 int 7049.31 | **10/31/25** pay 7325.43 int 6821.91 |
| 3 | 11/16/25 int 1702.56 | 11/16/25 int **1816.06** |
| 4 | 12/1/25 int 1698.07 | 12/1/25 int 1698.73 |

Principal on row 2 is identical (503.52 — the target floor), so the two engines
agree on what is being paid and disagree on WHEN the first payment falls and
therefore on the interest accrued to it. The row count is identical (206), the
dates re-converge from row 3 onward, and a small residue persists in the balance
column for the rest of the table.

**Where to read.** `Paymenttype.ComputeNext` (AMORTOP.pas:596-597) starts from
`base_date` and `AddPeriod(date, h^.peryr, h^.firstdate.d, add)`; the `peryr = 24`
branch of `AddPeriod` and of the `DaysCloseEnough` timedif adjustment
(AMORTOP.pas:626-629, `round((2*(date.d - prevdate.d))/30)/(2*12)`) are the two
sites that treat semi-monthly specially. The port's `dateutil.AddPeriod` and
`periodYearFraction` are the counterparts.

### ROUND 26 — ROOT-CAUSED, AND THE CLASS IS BOUNDED TO A SINGLE CELL

**The class is `peryr = 24` × first-payment day 31, and nothing else.** Matched
sweep, everything but `firstdmy` held constant (oracle command above, `dumpraw`
row `L1`):

| first payment | peryr 24 | peryr 26 | peryr 12 |
|---|---|---|---|
| 15 Oct | 10/15 ✓ | — | — |
| 28 Oct | 10/28 ✓ | — | — |
| 29 Oct | 10/29 ✓ | 10/29 ✓ | — |
| 30 Oct | 10/30 ✓ | 10/30 ✓ | 10/30 ✓ |
| **31 Oct** | **11/1 ← SHIFTS** | 10/31 ✓ | 10/31 ✓ |

Days 15/28/29/30 are honoured verbatim at every frequency, and day 31 is honoured
at `peryr` 12 and 26. **Only the semi-monthly × 31 cell moves.** Round 24's open
question — "day-31 only, or every day past the 28th?" — is answered: **day 31
only.**

**THE MECHANISM — a back-then-forward round trip that is not an involution.**
`RepayFancyLoan` does not use `firstdate` as given. It steps it BACK one period
and then FORWARD one period:

```pascal
    t := firstdate;                                     { AMORTOP.pas:1148 }
    AddPeriod(t, h^.peryr, firstdate.d, subtract);      { AMORTOP.pas:1150 }
    ...
        AddPeriod(t, h^.peryr, firstdate.d, add);       { AMORTOP.pas:1165 }
```

For almost every date that round-trips to where it started. **In `AddPeriod`'s
`peryr = 24` branch (INTSUTIL.pas:1216-1237) it does not**, because that branch
is the only one of the three carrying a `d >= 31` normalization:

- `subtract` from 31 Oct: `abs(31-31) < 4` → `d := 31`; `d-15` = 16 → **16 Oct**.
- `add` from 16 Oct: `abs(16-31) = 15`, no snap; `d+15` = 31; **`if (d>=31)` fires**
  → `inc(m)`, `d := 31-30 = 1` → **1 Nov**.

Day 30 survives because its half-period lands on `15+15 = 30`, short of the
`>= 31` test; days 29 and below land shorter still. **31 is the only first-payment
day whose backward half-step lands on 16, and 16 is the only day whose forward
half-step reaches the boundary.**

The other two branches are exactly invertible, which is why they show no shift:
`peryr` 26/52 step `Julian ± (364 div peryr)` days (a true inverse), and
`peryr` 1/2/3/4/6/12 assign `d := orig_day` outright. **The three branches of
`AddPeriod` predict the observed sweep cell for cell.**

Verified arithmetically against all five swept days — the four that do NOT move
as well as the one that does (round 26; standing rule 9's "adjudicate a sample",
applied to the controls and not just the positive):

```
 day | back-then-forward | DOS observed
  15 | 10/15             | 10/15   MATCH
  28 | 10/28             | 10/28   MATCH
  29 | 10/29             | 10/29   MATCH
  30 | 10/30             | 10/30   MATCH
  31 | 11/1              | 11/1    MATCH
```

**Why the port diverges.** `internal/dateutil.AddPeriodFields`' `case 24` is a
faithful port of the DOS branch, snap and `>= 31` rule included — **the callee is
not the defect**. The port's fancy walk seeds its schedule from the typed
`firstdate` directly and never performs the back-then-forward round trip, so it
never meets the non-involution. This is the standing pattern from §66 and
`CLAUDE.md`'s "read the callers, not just the callee": *a routine faithful to the
original, reached by a caller that is not.*

**THE FIX IS NOT `if day == 31`.** DOS does not special-case 31; it round-trips
every first date through `AddPeriod`, and 31 is merely where that happens to be
lossy. Porting the ROUND TRIP (AMORTOP.pas:1148-1150 → 1165) reproduces DOS on
all five swept days by construction; clamping day 31 reproduces it on the one
input the sweep happened to draw. Per `CLAUDE.md`'s "replicate the DOS logic; do
NOT patch around a divergence", the round trip is the port.

### STATUS: FIXED (round 26b, 2026-08-04)

`engine.go`'s `generateFancyScheduleMode` seeded row 1 from the typed
`loan.FirstDate`. It now round-trips that date through `AddPeriod` — back one
period, then forward one — exactly as AMORTOP.pas:1148-1150 → 1165 does. **Ported
as the round trip, not as `if day == 31`.**

Measured on the sweep, port row-1 date vs DOS:

| day | DOS | port BEFORE | port AFTER |
|---|---|---|---|
| 15 | 10/15/25 | 10/15/25 | 10/15/25 |
| 28 | 10/28/25 | 10/28/25 | 10/28/25 |
| 29 | 10/29/25 | 10/29/25 | 10/29/25 |
| 30 | 10/30/25 | 10/30/25 | 10/30/25 |
| **31** | **11/1/25** | **10/31/25** | **11/1/25** |

The four control days are unchanged, which is the assertion that matters most —
a fix introducing a shift where DOS has none was the likelier failure mode.

**Gates (all green):**

```
attribute_seven.py c6        206/206 rows, no DATE divergence, no >2c divergence
attribute_seven.py --assert  c6 EXPECT 2 -> None
  BOTH DIRECTIONS, IN FACT:
    pre-fix binary  (md5 cf73b69a) GATE FAILED — c6: first >2c 2, expected None
    post-fix binary (md5 92c846e7) c6 clean
PERSENSE_REQUIRE_ORACLE=1 go test ./internal/finance/amortization/   ok (96.0s)
paired_regression 50100-50139, FUZZ_N=400   FIXED 0 · STILL 22 · NEW 0
```

The two binaries carry different md5s — round 25's vacuous-PASS trap (a "pre-fix"
binary accidentally built from the post-fix tree) was checked for explicitly.

---

## §68 — CLOSED (round 30). On a USA-rule term probe crossing a rate adjustment, the port carried Re_Amortize's ACCUMULATOR forward; DOS's probe cannot see it (2026-08-04, rounds 29-30)

**Found:** fuzzer5 arm 50100, **seed 50134**, in scope (≤2099). Round 29 opened it;
round 30 fixed and gated it. It was **the last in-scope HARD case** on the stacked
surface.

**Repro:**

```
amort_oracle 77496.89 0.0473990000 19 1 b365 prepaid r78 usa \
  loandmy=10.9.2025 firstdmy=10.11.2025 mor=2 b14=17529.77 b62=13636.25 \
  pre=122:15:4:309.49 adj=26:0.1299630000:5682.71 adj=86:0.1238060000:4812.50 \
  adj=134:0.0505390000:8827.81 targ=267.22 pts=0.007932 payhard=7582.46 noterm
```

| | DOS | port BEFORE | port AFTER |
|---|---|---|---|
| `payment` / `interest` / `paid` | 7582.4600 / 80913.13 / 158410.02 | identical | identical |
| every rendered row (39 incl. settlement) | — | identical | identical |
| terminating balloon AMOUNT | 8566.1700 | 8566.1700 | 8566.1700 |
| **solved term** | **n=24** | n=25 | **n=24** |
| **terminating balloon DATE** | **11/10/2048** | 11/10/2049 | **11/10/2048** |

**The inverse of §66.** §66 was a summary scalar exactly right while a hundred rows
were wrong. §68 is a hundred rows exactly right while the summary scalar is wrong —
and unlike §63 it is worth a whole HARD case, because `solved_term_differs` trips
the whole-case classifier.

### The mechanism, read from DOS source (rule 5), confirmed by DOS's own trace

`scripts/build_trace_oracle.sh -mode cn` on the repro:

```
CN d=137-2-10 p=54319.94 usap=11517.51 int=1324.80 pay=309.49 pout=55335.25 uout=12532.82   <- superseded
CN d=137-2-10 p=54319.94 usap=11517.51 int= 540.80 pay=309.49 pout=54551.25 uout=11748.82   <- Re_Amortize redo
CN d=137-5-10 p=54551.25 usap=12532.82 int= 530.89 …                                        <- walk RESUMES
```

**The walk resumes on the redo's BALANCE and on the SUPERSEDED row's ACCUMULATOR.**
The reason is scope, and it is visible in four declarations:

```pascal
AMORTOP.pas:73    var f, f_1, p, usap, d, … : real;      { INTERFACE-level globals }
AMORTOP.pas:1323  function DetermineLastPaymentDate (p, usap: real): boolean;   { BY VALUE }
AMORTOP.pas:1413  function Iterate (p, usap: real; …): boolean;                 { BY VALUE }
AMORTOP.pas:1499  procedure Re_Amortize (var p: real);                          { p ONLY }
```

`Re_Amortize` is a **sibling** of the two probe routines, not nested in them. Its
`p` is a var param and reaches the walk; its `usap` advance (`:1610`) binds to the
**global**, which a walk reading its own by-value shadow never sees. The port has
ONE `reAmortize` and wrote both back — correct for the render path, wrong for the
probe. Go compounds this by emulating DOS's `Output=nil` term probe with a DISPLAY
walk (`engine.go`'s `probeARMRow` note), so there is only one version of the row to
carry forward. Δ9.91 of interest at 2037-05-10 compounds to ≈317 by period 24 and
flips the retirement test.

### The fix, and the two things that scoped it

`generateFancyScheduleMode`, gated on `input.termHorizonWalk` and on the crossing
row itself: the accumulator advances on the **superseded** row while the balance and
the reported row keep the redo's. Two negative controls set the boundaries:

1. **NOT `unforced` (R21/R22).** Only two of RepayFancyLoan's eleven call sites
   shadow the accumulator. `EstimateAndRefineBalloon` (`Amortize.pas:637`) is
   parameterless and passes the real globals, so it DOES see the advance. A first
   draft gated on `termHorizonWalk || unforced` fixed the term and simultaneously
   moved the terminating balloon AMOUNT off DOS, 8566.17 → **8249.26**.
2. **The superseded row differs in its PAYMENT as well as its RATE.** Seed 50134's
   crossing lands on a PREPAYMENT row, whose payment is the prepayment amount in
   both computations — so a rate-only correction fits it exactly and still regressed
   `373945.61 … adj=121:0.0820360000:16924.33 …` (n=57 → 56), where the crossing is
   a REGULAR row and DOS's trace shows `pay 22559.89` superseded against
   `pay 16924.33` in the redo. **The paired regression found that case, not the
   author.**

### Gates

```
paired_regression 50100-50139 FUZZ_N=400   FIXED 1 · STILL 21 · NEW 0
  (the rate-only draft: FIXED 1 · STILL 21 · NEW 1 — BLOCKED)
PRE /tmp/pre.test  md5 089f53a0…   POST /tmp/post.test md5 ef6971e4…   (distinct)
zzsec68_termusap_test.go   3 tests, seen to FAIL on the pre-fix tree in fact
PERSENSE_REQUIRE_ORACLE=1 go test ./... -v   12/12 ok, exit 0
check_skips.sh (SUITE_LOG reuse)             32 skipping / 32 allowlisted
arm 50100 re-run, 40 seeds, FUZZ_N=400       IN SCOPE compared 8,686  HARD 1 -> 0
```

**R22 — AN ALIASING CLAIM IS SCOPED TO A CALL SITE, NOT TO A ROUTINE** is this
defect's standing rule. Before porting a global's write-back, read the PARAMETER
LIST of every routine on the call path and ask which name the global resolves to
*there*.

---

## §69 — NOT A DIVERGENCE, BOUNDED, NEWLY ISOLATED: DOS's Julian ceiling is reached by the walk's ONE-PERIOD LOOKAHEAD, so a schedule whose last row is under the ceiling can still be unreachable (2026-08-04, round 31)

**Status: OPEN for a CLASSIFIER decision only. Not a computation defect. It is the
LAST in-scope HARD signal anywhere in the project after §65's `noterm` subclass
closed, and it is a representation limit, not a failure.**

### How it surfaced

Round 31 root-caused §65's `noterm` HARD subclass to a skip-months buffer
over-read in the ORACLE (see §65). Re-running the three standing arms paired
took `SIG=HARD:go_solved_dos_date_horizon` from **8 to 1**. This is the one that
survived — and it is a different mechanism entirely.

**Repro** (fuzzer5 arm 44000, seed 44021):

```
amort_oracle 143013.54 0.1178680000 1742 26 exact prepaid plusreg usa \
  loandmy=23.9.2024 firstdmy=23.11.2024 pts=0.033775
    DOS:  ERR Bad date passed to Julian function: m=-99
    port: lastdate 8/18/2091  nperiods 1742  payment 646.4129
```

No skip string is involved; the message is `bad date`, not `last payment not
found`, so it enters the `date-horizon` bucket by a different door.

### The mechanism

`MDY` (VIDEODAT.pas:373) refuses to convert a day number outside its range:

```pascal
if (daynumber<0) or (daynumber>70000) then begin x.m:=errorbyte; exit; end;
```

`errorbyte = -99` (PETYPES.PAS:146, VIDEODAT.pas:23). Any later `Julian` call on
that poisoned date hits VIDEODAT.pas:359-362 and reports
`Bad date passed to Julian function: m=-99`.

**Day number 70,000 is exactly 26 August 2091.** DOS's Julian is
`(1461*y - 1) div 4 + daysbefore[m] + d` with `y` based at 1900; for y=191
(=2091) that leading term is 69,762, leaving 238 days, and 2091 is not a leap
year, so 238 = 212 (`daysbefore[Aug]`) + 26.

**The screen's own horizon is under that ceiling. The walk's LOOKAHEAD is not.**
Bisecting the term on the repro:

| periods | port's last row | DOS |
|---|---|---|
| 1740 | 21 Jul 2091 | answers |
| 1741 | **4 Aug 2091** | **answers** |
| 1742 | **18 Aug 2091** | **`m=-99`** |

At `peryr=26` one period is 14 days. 4 Aug + 14 = 18 Aug, still under 26 Aug —
fine. 18 Aug + 14 = **1 Sep 2091, past the ceiling** — and DOS poisons the date
while computing the row AFTER the last one. So the last SCHEDULED payment being
representable is not sufficient; the NEXT one must be too.

### Why this is not scored as a port defect

§65's own text already states the disposition for this class: DOS hitting a
REPRESENTATION limit — its Julian routine, its 1900-based year byte (§55) — is
"expected, understood and out of scope", and a port with a wider calendar
answering there is correct. This case is exactly that; it is only scored HARD
because the sub-classifier excuses the *internal-error* text and not the
*bad-date* text, and because the era test uses the client's 2099 boundary.

**The boundary numbers do not line up, and that is the whole of the finding:**

- the project's in-scope rule is **≤ 2099** (client decision, 2026-08-03);
- DOS's Julian ceiling is **26 August 2091**;
- **the ~8¼ years between them are in scope by the project's rule and
  structurally unreachable for DOS** — and one further period of head-room
  below that, because of the lookahead.

Nothing in the project had ever stated DOS's ceiling as a DATE. §47 closed the
Julian ceiling as a computation; §55 recorded the 1900-based year byte (max
2155). The 70,000-day limit binds ~64 years earlier than the year byte does, and
it is the one that actually bites.

### What is needed to close it

**A decision, not a fix.** Two defensible options:

1. **Treat a `bad date`/`m=-99` refusal as out of scope**, the same way the
   `julian`/year-byte refusals already are — i.e. give the date-horizon
   sub-classifier a second excused text, and say so in the coverage statement.
   Under this reading the in-scope HARD count on the stacked surface is **0**.
2. **Move the in-scope boundary for this bucket to DOS's real ceiling**
   (26 Aug 2091, minus one period of lookahead), and state the 2091-2099 band as
   a named, measured gap rather than folding it into "in scope".

Option 2 is the more honest instrument and the more expensive one: it changes a
denominator that has been quoted for nine rounds. **Round 31 deliberately did
NOT change the classifier** — lowering a gate on the strength of one's own
same-session reasoning is how §59 acquired two wrong root causes. The signal is
left standing and named.

⚠️ **Do not fold this into the headline rate — it is a different population
(CAUTION 1).** `date-horizon` cases are BUCKETED, not COMPARED: the ledger reads
`generated = compared + refused + non-converged + date-horizon + …`, so this
signal sits **outside** the era-split denominator entirely, exactly as §65's
in-scope answered-refusals do. The correct pair of statements after round 31 is:

- **era-split, stacked, in scope ≤2099: 0 HARD in 25,842 COMPARED cases**
  (three standing arms, re-run post-fix this round);
- **and, separately, the `date-horizon` bucket contributes exactly ONE in-scope
  HARD signal — this one — which is a representation limit, not an arithmetic
  divergence.**


---

### ROUND 32 — THE ADVISORY SUBCLASS WAS OURS. CLOSED AS A HARNESS DEFECT.

**DOS answers these screens. The oracle driver was throwing the answer away.**

`RepayFancyLoan`, AMORTOP.pas:1226-1233:

```pascal
if ((not h^.lastok) and (WhenToStop^.principal = 0)) then
  begin if (not entire) then h^.lastdate := WhenToStop^.date; end
else if (DateComp(WhenToStop^.date, very_last) > 0) and (not balance_Calc) then
  MessageBox('Internal error - last payment not found.  Please contact Ones & Zeros.',
             DA_InternalError );
h^.loanrate := saverate;
ComputeTrueRate;
DisposeOfOld_Pre;
```

**A BARE STATEMENT.** No `exit`. No `errorflag := true`. Control falls straight
through the epilogue and the procedure returns normally. And real DOS's
`MessageBox` (`dos_source/Globals.pas:107-116`) is `MessageDialog.ShowMessage`;
`MessageDialogUnit.pas:63` is a Delphi `TForm` that sets a caption and shows an
OK button. **It latches nothing.** In the real product the user dismisses a
dialog and **the schedule is drawn.**

The refusal was the ORACLE DRIVER:

```pascal
{ amort_oracle.pas:1101-1109 }
MakeTable(Output, false);          { the table IS BUILT, into Output }
if OracleErrorFired then
begin Writeln('ERR ', OracleFirstError); Halt(0); end;   { and DISCARDED }
```

This is the **fourth** bare-statement `MessageBox` corrected in
`legacy/oracle/Globals.pas` for exactly this reason — after `DA_ChangeTo365`,
`DA_APRNoConverge` and `DA_TerminatingBalloonChanged`. That file's own
`OracleFirstError` comment already called these dialogs **"advisory"** by name.
The fix is one line: `if HelpCode = $02010017 then exit;`

#### What it cost — booked paired, four arms, 160 seeds, one line of difference

| | PRE (committed oracle) | POST (corrected) |
|---|---|---|
| in-scope COMPARED (≤2099) | 34,412 | **35,000**  (+588) |
| **in-scope HARD** | **0** | **475** |
| in-scope rate | ≥99.99130% (0 events) | **1 in 74 = 98.643%** |
| §65's in-scope advisory bucket | 690 | **0** |

**475 of the 588 newly visible cases are HARD — 80.8%.** Every published stacked
figure from rounds 22-31 was measured over a population the oracle had truncated.

**Attribution is ONE class:** `SIG=HARD:divergent_class` 13 → 502 across the four
arms; every other SIG is flat (`balloon_value_differs` 18→19,
`go_solved_dos_refused` 9→9, `solved_rate_differs` 2→2, `solved_amount_differs`
2→2, `go_solved_dos_date_horizon` 1→1).

#### Controls

- **NEGATIVE (R19):** over 145 generated screens DOS already answered, PRE and
  POST oracle stdout is **byte-identical, 145 of 145**.
- **POSITIVE (R24):** 10 of the 95 harvested repros still refuse once the
  advisory is swallowed, with a genuine `did not converge`.
- **IDENTITY:** the shipped binary's md5 is byte-identical to the validated probe.
- **INDEPENDENT INSTRUMENT:** on the 95 repros, DOS's table MATCHES the port on
  14, DIVERGES on 54, still refuses on 10, 17 unmeasured (note #24) — 54/68 = 79%
  against the arms' 80.8%.

#### The open part — the remaining defect is ARITHMETIC, and it has a signature

> **⚠️ CORRECTED THE SAME DAY, AND THE COMMIT MESSAGE OF `6fc6927` STILL CARRIES
> THE UNCORRECTED VERSION.** The first version of this list led with
> *"`dInt == dPaid` to the cent in 54 of 54 → the total principal repaid is
> IDENTICAL and the whole difference is INTEREST"*. **That is not evidence.**
> **62 of the 68 scored repros retire to zero on BOTH sides** (50 of the 54
> divergent ones), and two schedules that both
> retire the same loan differ in `paid` by exactly what they differ in
> `interest` — it is an accounting identity. **It is also not fully understood:**
> the other 4 leave a RESIDUAL, whose difference should break the identity and
> measurably does not. Treat that as an open loose thread in the instrument, not
> a finding, and do not quote the identity in any argument until it is explained.

What survives as evidence:

- the port's **total interest is LOWER in 53 of 54**;
- row counts are **EQUAL in 49 of 54**;
- on a worked example the first rows agree exactly and the schedules separate
  **mid-schedule, after an adjustment**.

**AND, FROM THE SAME CHECK, A SEPARATE FINDING THAT RETIRES THE LAST OF ROUND
31's NUMBERS.** Over all 95 repros — 68 scored, 27 unscored and counted as such
(R12) — **62 retire on BOTH sides**. Six do not, and **five of the six have a DOS
residual too**:

```
                    DOS            port         delta
                   0.00        5,732.34      5,732.34   <-- PORT ONLY. A real defect.
               1,648.42        3,732.75      2,084.33
             173,539.45      174,711.69      1,172.24
             322,478.08      323,361.06        882.98
              26,589.35       27,299.84        710.49
               4,517.71        4,988.32        470.61
```

**Exactly ONE case in 68 is the port leaving a balance where DOS retires.** Round
31's *"9% of those screens are the PORT shipping a schedule AND TOTALS for a loan
that never pays off, residuals to \$323,361"* is **~1.5%, and the \$323,361 case
is not one of them — DOS leaves \$322,478 on that same screen.** That was the
specific number standing decision 3a.4 rested on, and it is gone twice over: the
screens are not refused, and the residual is almost never the port's alone.

**⚠️ ONE UNCONFIRMED COINCIDENCE, WORTH ONE PROBE AND NO MORE.** On the
322,478 / 323,361 case the delta is **882.98** and the screen carries
**`targ=882.98`** — exact to the cent, on one case out of six, noticed by someone
looking for a pattern. **Probably chance.** Recorded because a suppressed
observation is worse than a flagged one; flagged because this section's previous
version was itself a pattern that turned out to be an accounting identity.

Instrument: `testplan/harness/audit_sec65_final_balance.py`.

Suspect a `Re_Amortize` / rate-reconstruction site — the §66/§67 family — but
hold it loosely; it is a prior, not a measurement. **Start with
`scripts/build_trace_oracle.sh -mode cn` (~40 s) and localise the FIRST divergent
row on three or four repros before forming a theory.** The trigger condition is
*the walk overshot `very_last`*, so the rows PAST `very_last` are where to look.

**⚠️ IGNORE THE `dosport_walk.go:156` LEAD THIS SECTION ORIGINALLY CARRIED**
(the `DateComp(e.payment.date, e.veryLast) == 0` fold "cannot fire when the walk
steps past rather than onto it"). It was derived from the retracted principal
reading: that fold governs whether the final row ABSORBS the residual, and the
residual evidence above says both engines behave the same way there.

#### What round 31 got wrong, and why

Round 31 measured this subclass with `audit_sec65_advisory.py` and reported
"91% of these screens the port handles fine, 9% it ships a residual". That script
asks **does the PORT's own schedule terminate** — it had no DOS answer to compare
against, *because the oracle was hiding it*. With one, the port agrees on 19%.
**"Does our answer look sane" is not "does our answer match."** Standing decision
3a.4 (the port refuses) rested on the 9% figure and was **WITHDRAWN by Nate on
2026-08-04.**

#### Rule filed

**R26 — A REFUSAL IS A CONTROL-FLOW CLAIM, AND IT BELONGS TO WHOEVER WROTE THE
`exit`.** Read the call site of every message the harness treats as fatal and
check the flag it actually SETS, not the words in the string. Four of four help
codes examined in this project have been non-fatal in the original; the rest
should be audited.

#### Gate

`internal/finance/amortization/zzsec65_oracle_advisory_test.go` — advisory
yields a table, a real refusal still refuses, an answered screen is
byte-unchanged. Seen to FAIL against the PRE binary and PASS against the POST
binary, with the negative control passing under both.

---

## §70 — THE STACKED DIVERGENCE IS AN ENGINE-COVERAGE RESULT, NOT AN ARITHMETIC ONE: every measured divergence lives in the PIECEWISE FALLBACK, and the faithful DOS port has zero over the ~3.5% of the population it is allowed to answer (2026-08-05, round 33)

### The state this replaces

Round 32 rebased the in-scope stacked rate from "0 HARD in 34,412" to **475
HARD in 35,000 — 1 in 74**, and attributed 100% of it to one SIGNAL class,
`HARD:divergent_class`. Round 32's own signature for that class was: row counts
equal in 49 of 54, the port's total interest LOWER in 53 of 54, the schedules
separating mid-schedule **at an adjustment**. That reading pointed at
`Re_Amortize` and at the §66/§67 family ("a routine faithful to the original,
reached by a caller that is not"), and round 33's plan named that as the round's
engine work.

**`divergent_class` is a class of SYMPTOM.** It is a whole-case verdict on the
TOTALS. It says the two schedules ended up in different places and says nothing
about where they parted, and nothing at all about WHICH ENGINE produced the
port's schedule.

### What was measured

`testplan/harness/localise_divergent_row.py` (new) aligns DOS's `dumpraw` rows
against the port's `rows` **by ordinal with the date as a cross-check** — not by
index (defect #15: DOS interleaves announcement and summary lines into the same
numbered stream) — and reports the FIRST cell that disagrees by more than half a
cent. Over a 20-seed × 400-case FLAKEDUMP harvest (72 repros, 56 comparable
after note #24), the first divergent cell was the **interest** in 49 of 56, with
every earlier row — payment, principal and balance — exact. Three more first
diverge by exactly one cent on the balance, and four by row COUNT.

On every case read by hand, the divergent interest row sat immediately after a
re-amortization DOS announces in its own default output:

```
L37| 2/16/34 3627.58 418.06 3209.52 14573.56 26966.91
L38|--->On  2/16/34, re-computed at 21.0091%:  Payment fixed at 3416.33
L39| 5/16/34 35.91 765.44 -729.53 15303.09 27732.35   <- DOS
     5/16/34 35.91 622.18 -586.27 15159.83            <- port
```

which is round 32's signature confirmed at row resolution. So the next question
was what rate each engine left the adjustment with — and the port had no way to
say. `DPTRACERA=1` was added to print exactly the line DOS prints unprompted.

**It printed nothing.** Not on that case, and not on any of the other 55.

### The finding

`DPTRACEENGINE=1` (new) names which of the two engines answered a screen, from
`dosPortRoute`'s own return value. **All 56 comparable repros route to the
PIECEWISE engine. Not one reaches the faithful DOS port.** The port's
`reAmortize` was never the suspect; it was never called.

100% is not a finding without its base rate (rule 9), and most of the fuzzer's
stacked population is piecewise as well. `testplan/harness/engine_attribution_arm.py`
(new) produces the contingency table over 20 seeds × 400 cases — the corpus from
`FZ5CASEDUMP=1` as the denominator, the divergences from
`PERSENSE_FUZZ_FLAKEDUMP=1` on the SAME seeds as the numerator, matched back by
argument string so the two are provably the same population:

| engine / first rejecting clause | cases | diverged | rate |
|---|---|---|---|
| `piecewise:in_advance_or_r78_or_daily` | 3401 | 34 | 1 in 100 |
| `piecewise:replace_mode_with_extras` | 206 | 13 | **1 in 16** |
| `piecewise:balloon_plus_ao6_or_ao7_adjustment` | 82 | 6 | **1 in 14** |
| `piecewise:exact_non360` | 398 | 4 | 1 in 100 |
| `piecewise:adjustment_carries_amount_ao6` | 70 | 1 | 1 in 70 |
| **`dosport`** | **166** | **0** | **0** |
| `piecewise:disabled_or_not_fancy_or_backward` | 189 | 0 | 0 |
| `piecewise:degenerate_term_or_peryr` | 208 | 0 | 0 |
| **TOTAL** | **4720** | **58** | **1 in 81** |

Three things fall out of it:

1. **The faithful DOS port answers 166 of 4,720 routed cases — 3.5% — with
   ZERO divergences.** The engine the project has spent eight rounds hardening
   is reached by one case in twenty-nine.
2. **The whole measured rate is the fallback's.** Every divergence is in a
   population `dosPortCanHandle` handed to the piecewise engine.
3. **Two clauses are ENRICHED, not merely large.** `replace_mode_with_extras`
   (1 in 16) and `balloon_plus_ao6_or_ao7_adjustment` (1 in 14) run ~6× the
   pooled 1 in 81, on small denominators. The biggest absolute contributor,
   `in_advance_or_r78_or_daily` (34 of 58), is at the pooled rate.

### ⚠️ THE REASON IS THE FIRST REJECTING CLAUSE, NOT THE ONLY ONE

`dosPortRoute` short-circuits. A case attributed to `in_advance_or_r78_or_daily`
may satisfy three later clauses too. Every per-clause rate above is therefore an
**upper bound on that clause's exclusive contribution**, and no clause's row can
be read as "removing this clause removes these divergences".

### ⚠️ NOTE #24 COSTS A THIRD OF THE POPULATION HERE, AND IT IS DECLARED

goamort implements neither `norate` nor `noamt`, so those screens cannot be
routed by this method. **2,554 of 7,863 corpus cases (32%) and 17 of 75
divergences are EXCLUDED FROM BOTH COLUMNS.** An exclusion applied only to the
denominator would have deflated every rate on the table. A further 589 cases
emitted no `GENGINE` line at all and are counted as UNROUTABLE, not as either
engine.

### The remedy is NOT "route everything to the faithful port"

Measured, not assumed. A PROBE build with the five named clauses neutralised
(`/tmp/proberepo`, md5-distinct binary) was run over the same 20 seeds and made
things **far worse**: 455 `HARD:divergent_class` in the first 8 seeds against 38
for the shipped build over the same 8. `in_advance`, `r78` and `daily` are
genuinely not ported — that clause is load-bearing and the probe is its positive
control.

A NARROW probe leaving that clause alone and neutralising only the four whose
comments describe a **validation-scope or cosmetic** exclusion
(`exact_non360`, `replace_mode_with_extras`,
`balloon_plus_ao6_or_ao7_adjustment`, `adjustment_carries_amount_ao6`) reduced
the count on every seed it completed and increased it on none.

**⚠️ THE FIRST FIGURE WRITTEN FOR THIS WAS WRONG AND IS CORRECTED HERE.** The
round-33 commit message says *"22 vs 38 over 9 seeds"*. That tally counted a
NINTH seed whose probe run had not finished — its numerator was the partial
log's 0 and its denominator the baseline's 5, so an unfinished seed scored as a
perfect result. **A RUN IN PROGRESS READS EXACTLY LIKE A RUN THAT FOUND
NOTHING**, the same shape as the standing trap "a killed binary reports nothing,
and nothing looks like success". The honest figure over the seeds that actually
COMPLETED:

| seeds 50100-50107 (8 complete) | HARD:divergent_class |
|---|---|
| shipped build | 33 |
| narrow probe | 22 |
| reduction | **33%** |

with no seed worse. Per-case, with those clauses neutralised, 22 of the 56
repros AGREE outright and 24 more first diverge by a half-cent print tie.

**That is a candidate, not a fix.** It does not reach zero, it was not gated by
the paired regression, and the four clauses were neutralised together so no
single one is attributed. It did not land in round 33.

### Why the clauses exist, and why that is the interesting part

Two of the four read, in their own comments, as scope statements rather than
fidelity ones:

- `replace_mode_with_extras`: *"REPLACE mode is unvalidated through the port —
  every fuzzer used plus_regular=true."* The fuzzer now generates REPLACE mode.
  **The sample space was widened past the router's validation and nothing
  re-asked the routing question.**
- `adjustment_carries_amount_ao6`: *"carries the A-W12 negative-implied-rate
  Note that the port does not emit"* — a missing WARNING CELL, sending the
  arithmetic to the other engine.

### Instruments and guard landed

- `testplan/harness/localise_divergent_row.py` — first divergent ROW, aligned by
  date, note #24 declared and printed.
- `testplan/harness/engine_attribution_arm.py` — the contingency table above.
- `DPTRACERA=1` — the port's re-amortize entry/exit, the counterpart to the
  announcement DOS already prints.
- `DPTRACEENGINE=1` — which engine answered, and the clause that decided it.
- `dosPortRoute` — `dosPortCanHandle` refactored to return its REASON, with the
  predicate defined as a wrapper over it, so instrument and decision are one
  code path (R13).
- `internal/finance/amortization/zzsec70_engine_route_test.go` — the predicate
  and the reason cannot drift, and every reason in the table above is reachable.
  **Seen to FAIL on the narrow-probe tree** (3 of 6 reasons unreachable there).

### Status

**OPEN, and it is now §3b item 1's actual subject.** The stacked gap is a
COVERAGE result: the validated engine is reached by 3.5% of the population. The
route out is to widen `dosPortCanHandle` one clause at a time, each with its own
oracle validation on that axis and its own paired-regression gate — not to
root-cause an arithmetic defect in `Re_Amortize`, which round 33 disproved as
the site.

---

### §70, ADDENDUM (2026-08-05, round 33) — READING THE DOS SIDE CHANGES THE SHAPE OF THE REMEDY

The body above establishes WHERE the divergence lives. Nate then asked the
question that should have been asked first: **does the ORIGINAL have two
amortization engines, or is that only the port?**

#### DOS has two repay routines, and the split is a different one

`AMORTOP.pas` defines `RepayLoan` (:1269) and `RepayFancyLoan` (:1101).
`RepayLoan` is ~20 lines — no dates, no options, just `p := p * f - d` compounded
`nperiods` times. `RepayFancyLoan` is the ~140-line dated walk carrying balloons,
prepayments, adjustments, moratorium, the USA rule and skip months. The dispatch
is identical at both `Iterate` call sites (AMORTOP.pas:1438, :1464):

```pascal
if (fancy) or ((df.c.exact) and (df.c.basis<>x360)) then
  RepayFancyLoan(...)
else
  RepayLoan(p);
```

So DOS's split is **plain vs fancy**, decided by whether the screen carries any
advanced option at all. **Every stacked screen — the entire population §70
measures — goes through ONE routine.** There is no second fancy engine in the
original. The port's piecewise engine has no counterpart in DOS: it is an
independently written implementation of the FINANCE, predating the structural
port of the CODE.

#### ⚠️ WHICH MEANS "THE PIECEWISE ENGINE IS THE BAD ENGINE" IS WRONG

`dosPortCanHandle` opens with `!in.Fancy → false`, so **every PLAIN loan is
answered by the piecewise engine** — and the plain surface is the project's
strongest result, 0 arithmetic divergences in 108,778, re-measured at round 32
and identical to round 22. The engine carrying all 58 stacked divergences is
exact on the plain population.

The accurate statement is narrower and more useful:

> **The piecewise engine is exact where DOS itself takes the simple path, and
> drifts where DOS takes the fancy walk.** The goal is not to replace it; it is
> that *the fancy walk should be answered by the port of the fancy walk*.

#### 🚨 AND THE BIGGEST ROUTING CLAUSE IS MOSTLY NOT PORTING WORK

`in_advance_or_r78_or_daily` is 3,401 cases and 34 of 58 divergences, and reads
like three unported features. Reading the DOS sources for each flag:

**`R78` IS INERT IN DOS FOR A FANCY LOAN.** All six reads of `df.c.R78` in the
DOS units are either header/legend TEXT gated on `not fancy`, or dominated by a
`fancy` disjunct:

```
AMORTOP.pas:748    if (not fancy) and (df.c.R78) then OutputLine(R78Header1);
AMORTOP.pas:782    else if (not fancy) and (df.c.R78) then ws:=R78Header1
Amortize.pas:1107  if (not df.c.R78) or (fancy) then begin
Amortize.pas:1157  if (fancy) or (not df.c.R78) or (not (df.c.basis=x360)) then begin
Amortize.pas:1493  if (fancy) or ((df.c.exact) and (not df.c.R78)) or (not (df.c.basis=x360))
INTSUTIL.pas:401   if (fancy) then ... else if (df.c.R78) then write('R78');   {status line}
```

**This is the SAME SHAPE, and partly the same LINES, as the argument the port
already made for `exact`** — see `dosPortCanHandle`'s own comment, which cites
Amortize.pas:1493 among others and concludes *"routing exact × 360 to the
piecewise engine was not a fidelity choice, it was a fidelity LOSS"*, worth
\$982.27 on the repro recorded there. Nobody re-ran that argument for `R78`.

**`in_advance` and `daily` ARE live in the fancy walk** and are genuine porting
work: `AMORTOP.pas` reads `df.c.in_advance` 14 times, two of them explicitly
`(fancy) and (df.c.in_advance)` (:1041, :1049), and `daily` is a `peryr` MODE
read at :187 and :633 with `ComputeTrueRate` called from `RepayFancyLoan`'s
epilogue.

#### The bucket, split by flag

Same 20-seed corpus, note-#24 exclusions applied to both columns:

| flag combination | cases | diverged | rate |
|---|---|---|---|
| **`r78` only** | **1345** | **30** | **1 in 45** |
| `inadv` only | 1326 | 4 | 1 in 332 |
| `inadv` + `r78` | 1324 | 0 | 0 |
| neither | 1314 | 24 | 1 in 55 |
| TOTAL | 5309 | 58 | 1 in 81 |

**`r78` without in-advance is the worst slice measured — 1 in 45, and 52% of all
divergences.** And R78 is inert in the original.

#### ⚠️ TWO CAVEATS, AND THE FIRST ONE KILLS THE OBVIOUS READING OF ROWS 2 AND 3

**THE `inadv` ROWS ARE A STRUCTURALLY SIMPLER POPULATION AND ARE NOT COMPARABLE.**
`Amortize.pas:1294` rejects any screen with `(df.c.in_advance) and (nadj > 0)` —
ANY adjustment row — so the generator emits **no adjustments at all under
`inadv`** (`dos_fuzzer5_test.go`:1441-1449). Rows 2 and 3 above therefore
describe screens carrying strictly fewer stacked options than row 1, and their
low rates are NOT evidence that in-advance is fine. What they do show is that
in-advance-with-adjustments barely exists in DOS, so porting in-advance into the
faithful walk buys little on this surface.

**`daily` CONTRIBUTES ZERO CASES.** `perYrs` is `{1,2,3,4,6,12,24,26,52}` — the
fuzzer never draws daily. A third of the clause's NAME is measured by nothing.
(`docs/fuzzer_sample_space_audit_2026-08-02.md` already lists Daily compounding
as a silent axis; this is where that silence shows up.)

#### R28 again, and from the fuzzer's own comment

`dos_fuzzer5_test.go`:1117 — *"R78 and the USA Rule were pinned false when this
fuzzer was written, so the two interest-ALLOCATION modes were the only advanced
options never stacked."* R78 is a RECENT widening of the sample space, and the
routing clause that excludes it predates the widening. That is the third
instance this round of a scope note outrun by the generator, after
`replace_mode_with_extras` and the sample-space audit's own silent rows.

#### 🚨 THE OBVIOUS REMEDY WAS TESTED AND IT MOVED NOTHING — AND THAT IS THE REAL FINDING

The reading above said: split `R78` out of the clause and the largest slice of
divergences should reach the faithful port with no porting work. A probe port
with `R78` removed from that clause **and nothing else** (md5-distinct binary,
`/tmp/r78probe`) was run over seeds 50100-50107:

| seeds 50100-50107 | HARD:divergent_class |
|---|---|
| shipped build | 33 |
| `R78`-only probe | **33** |

**Identical, seed for seed, on all eight.** R20: a fix that changes nothing has
not been confirmed.

The reason is the caveat this section already carried and then quietly forgot
when it proposed the remedy. `dosPortRoute` SHORT-CIRCUITS, so its per-clause
table reports the FIRST rejecting clause. Routing the 56 repros through the probe
shows where they land once `R78` no longer catches them:

| with `R78` removed, the repros now reject at | count |
|---|---|
| `replace_mode_with_extras` | 26 |
| `balloon_plus_ao6_or_ao7_adjustment` | 14 |
| `exact_non360` | 11 |
| `in_advance_or_r78_or_daily` (still — genuinely in-advance) | 3 |
| `adjustment_carries_amount_ao6` | 2 |
| **reaching the faithful port** | **0** |

**Every divergent case is excluded by TWO OR MORE clauses.** That is why the
narrow probe (four clauses at once) cut the count 33% while a single-clause probe
cuts it 0%.

#### ⚠️ WHICH INVALIDATES THE ONE-CLAUSE-AT-A-TIME PLAN

The round-33 plan in START_HERE §3b said to widen the clauses one at a time, in
enrichment order, each with its own gate. **That plan cannot work and would have
measured zero four times in a row before anyone understood why.** The clauses are
not independent and their case counts are not additive: a screen carrying stacked
options typically trips several at once, and only a JOINT widening moves it.

The corrected shape for round 34:

1. **Stop reading the per-clause table as a work queue.** It ranks clauses by the
   population they FIRST catch, which is not the population they exclude. Build
   the ALL-CLAUSES-THAT-MATCH profile per case — `dosPortRoute` needs to return a
   SET, not the first hit — before any more remedy planning.
2. **Widen JOINTLY, and let the gate say which combination is safe.** The narrow
   four-clause probe is the only configuration measured to help (33 → 22, no seed
   worse); that is the candidate to validate, not any single clause from it.
3. **`R78`'s inertness under `fancy` still stands as a SOURCE reading** — all six
   `df.c.R78` reads are header/legend text gated on `not fancy` or dominated by a
   `fancy` disjunct — and it is still worth landing as a routing correction on
   fidelity grounds. **But it is worth ZERO against the rate on its own**, and
   this addendum is the record of that being measured rather than assumed.


---

### §70, SAME-DAY REVIEW (2026-08-05, round 33 close) — four findings, two landed now, two deferred to round 34

A fresh-eyes review of the round's changes, with two new measurements. The core
finding survives; the instruments and docs carried four defects.

**LANDED NOW:**

1. **`zzsec70_engine_route_test.go` did not cover what the docs said it covers.**
   The claim "every reason in the table is reachable" was written while the test
   pinned only 6 of the table's 8 reasons — and one of the two missing was
   `balloon_plus_ao6_or_ao7_adjustment`, the table's MOST ENRICHED clause
   (1 in 14). Both rows are now pinned (AO7 date-only adjustment + on-grid known
   balloon; `LastOK=false`) and the test passes. The claim is true as of this
   commit and was false before it.

2. **The 589 UNROUTABLE cases are diagnosed.** Not a token hole: goamort exits 0
   with NO `GENGINE` line at all — `Amortize` is never reached, an early-error
   path. Sampling 220 routed-population cases found 24 such (predicting ~579 vs
   589 measured — consistent), all extreme-term/long-horizon screens (n=648 to
   4000, dates past the representation ceiling). They were correctly excluded
   from BOTH columns, so the table stands; the "mystery" label does not.

3. **R18 compliance for the narrow probe, recorded here:** per-seed pairs over
   50100-50107 are 5 improved / 0 worse / 3 ties → one-sided sign test
   **p = 1/32 ≈ 0.03**. The 33% reduction is significant, and now says so.

4. **The r78-only probe's null result, completed:** the missing half of that
   measurement was how many cases RE-ROUTE to the faithful port when `R78` is
   removed from the clause (see the sampled figure appended below). Together with
   33 = 33 this closes the question of whether the fidelity fix silently moved
   population between engines.

**DEFERRED TO ROUND 34 (do these before trusting any new attribution run):**

5. **`GENGINE` emits MULTIPLE lines per invocation** — verified: one screen
   printed two. `engine_attribution_arm.py` takes the FIRST, but a screen whose
   first `Amortize` call is a pre-solve rather than the table build would be
   attributed to the wrong call — and `disabled_or_not_fancy_or_backward` (189
   cases) contains the `inBackwardSolve` guard, which is exactly where this
   bites. Fix: assert all GENGINE lines agree, or take the LAST; then re-run and
   confirm the table does not move. **Until then the table's row boundaries carry
   this caveat.**

6. **Set-membership tallying double-counts an identical case drawn in two
   seeds.** Negligible at this scale; fix when the script is touched for #5.

**The re-route figure (finding 4's measurement):** over a 300-case random sample
of the routed population, the r78-only probe routes **19 vs the shipped build's
10** cases to the faithful port — **9 re-routed, ~160 of 5,309 extrapolated
(~3%)**. So the fidelity fix is NOT a no-op on routing: it roughly doubles the
faithful port's share, moves only non-divergent cases (the HARD count is 33 = 33,
seed-identical), and none of the re-routed cases diverged under the port. That is
weak-but-free evidence the faithful port handles r78-marked screens correctly —
consistent with R78 being inert in DOS — while confirming it is worth zero
against the rate, because the divergent cases are all caught by other clauses.

---

## §71 — THE FAITHFUL PORT'S FANCY WALK DOES NOT TERMINATE ON A SCREEN DOS HANDLES, BECAUSE DOS'S DATE ARITHMETIC POISONS WHERE THE PORT'S FAILS (2026-08-05, round 34)

**STATUS: CLOSED 2026-08-05 (round 35) — ADJUDICATED, FIXED, GATED AND MEASURED.**
The port returns DOS's answer to the cent on the repro screen, in 8 ms. Guarded
by `internal/finance/amortization/zzsec71_walk_terminates_test.go` — five tests:
termination, the ANSWER, the monthly negative control, two positive controls,
and the cursor-freeze boundary. Round 34's iteration bound is KEPT as a net and
is now unreachable on any measured screen.

**Round 35, in order — detail in `### ROUND 35` at the end of this section:**

1. **The ORACLE DRIVER was the obstacle, for the third time** (§65 in r31 and
   again in r32). `legacy/oracle/Globals.pas` escalated DOS's non-fatal
   `EMessage` to `noteError → Halt`. It is now gated behind
   **`PERSENSE_ORACLE_SOFT_EMESSAGE=1`** — DEFAULT UNCHANGED AND VERIFIED
   BYTE-IDENTICAL — and with the gate DOS answers the repro screen in 15 ms:
   `payment 743.6690 interest 126970.33 paid 226970.33`.
2. **All three call sites restored**, each confirmed by its own negative
   control: sites 2 and 3 are independently necessary, and site 1 is necessary
   in a ONE-PERIOD-WIDE band the source predicts and the sweep confirms.
3. **Measured on the family the fuzzer cannot draw**
   (`testplan/harness/sec71_ceiling_arm.py`, 500 screens):
   **FIXED 47 · NEW 0 · POST hangs 0** — every one of the 47 on the Julian
   (peryr 26/52) arm, the field arm inert.
4. Gates: suite GREEN `-count=1`, 12 packages, 0 cached; `check_skips` 32/32;
   `paired_regression` 44000-44039 **NEW=0**; termination gated SEPARATELY, per
   note #29.
5. **⚠️ AND AN INDEPENDENT AUDIT MOVED EVERY ONE OF THOSE NUMBERS.** It found a
   real defect in the fix that all of the above had passed over, and three
   defects in the instrument that produced them. **§35A below is the account,
   and it is the most useful part of this section.**

⚠️ **AND IT OPENED §72.** Adjudicating this family compared 500 screens nobody
had compared before, and the FAITHFUL PORT diverges on **3 of the 255 it answers
IN SCOPE**. None of the three is §71, and none needs the gate.

### The screen

```
goamort 100000 0.08 900 12 plusreg loandmy=17.10.2025 firstdmy=17.11.2025 \
  pre=12:5000:52:10
```

A 900-period monthly loan with a 5,000-payment **weekly** prepayment series
starting at period 12. No advanced option, no adjustment, no balloon. Its router
exclusion set is **empty** — `DPTRACEENGINE=1` prints `GENGINE dosport` — and
`AmortizeDOS` never returns. The `peryr=12` twin of the same screen returns in
milliseconds.

Found indirectly: round 34 was gating an unrelated R78 clause split when fuzzer5
seed 44016 case 12 burned 29 minutes of CPU on one case. That screen carries
`r78` and its co-exclusion set is the singleton `in_advance_or_r78_or_daily`, so
the split exposed it — but the *shape* is not gated by any clause and the defect
is live without the split.

### The mechanism — and it is the fifth instance of one pattern

**A ROUTINE FAITHFUL TO THE ORIGINAL, REACHED BY A CALLER THAT IS NOT** (§59,
§66, §67, §68, and now this).

DOS does not treat a Julian overflow as a failure. It **poisons the record and
keeps going**:

| step | DOS | file:line |
|---|---|---|
| overflow | `if (ndays > 70000) then begin x.m := errorbyte; exit; end` — the record stays readable | VIDEODAT.pas:373 (`MDY`) |
| ordering | "blank or unknown dates are later than everything" — a poisoned record sorts after every real date | INTSUTIL.pas:829-830 (`DateComp`) |
| consequence | a poisoned prepayment `nextdate` gives `balloonpos := +1` forever, so the extra is never consumed again and the walk proceeds on regular payments | AMORTOP.pas:605 |
| horizon clamp | `if (not dateok(stopdate)) then begin stopdate := firstdate; stopdate.y := 100+pred(df.c.centurydiv); end;` **{Keep going as long as possible}** | AMORTOP.pas:1143-1147 |

Measured against the compiled DOS engine: `amort_oracle intutil addn 2096 12 17
52 245` → `last 1900 -99 0`. The record is poisoned, not rejected.

The port's `dateutil.MDY` returns the analogous unusable `DateRec` **together
with an error**, and its own comment (dateutil.go:219-245) instructs callers to
"reproduce that stop-and-keep … and NOT convert it into a hard error." Two
DOS-port call sites do exactly the opposite:

- `internal/finance/amortization/dosport.go` — `checkOffBalloon` (CheckOffBalloon,
  AMORTOP.pas:559) advances `pp.nextdate` only `if err == nil`, so on overflow the
  date stays at its OLD value instead of becoming poison. It is then neither
  poisoned (never sorts last) nor advanced (never passes the stop date), so the
  series never retires.
- `internal/finance/amortization/dosport_entry.go` — `buildDosEng`
  (CheckPrepayments, AMORTOP.pas:416-419) sets `dp.stopdate/stopOK` only
  `if err == nil`, so the series gets NO stop date and `e.veryLast` is never
  poisoned. DOS sets `stopdatestatus := outp` on that arm regardless of overflow.
- `internal/finance/amortization/dosport_walk.go` has **no counterpart at all** to
  AMORTOP.pas:1143-1147's horizon clamp.

With the date frozen, `computeNext`'s `balloonpos = -1` arm sets
`pt.date = nextextra.date` and does not advance the base date (mirroring
AMORTOP.pas:623), so **every walk state is invariant**. Instrumented: 400,000
iterations with `date=2096-12-17`, balance unchanged, `rows=0`.

### Why nothing caught it

- The walk's own `if len(rows) > 5000 { break }` safety bound is **dead code on
  the walk that runs away**: `rows` is appended only inside `if collect`, and the
  runaway is in `Iterate`'s TRIAL walk (`collect=false`). A payment-GIVEN screen
  returns; the payment-SOLVE screen on the same input does not.
- The full suite was GREEN throughout.
- `paired_regression.sh` returned `NEW=0` and **structurally cannot** catch this
  class: a hung seed is killed by `timeout 900`, emits no failure lines, and
  therefore reads as **FIXED**. *A killed binary reports nothing, and nothing
  looks like success* — the standing trap, living inside the gate that exists to
  prevent regressions.
- A 159-seed paired arm comparison said 470 vs 470, tied on every seed.
- **What caught it was `engine_coexclusion_arm.py` refusing to count the one seed
  whose log carried no `ledger:` line.** That unfinished-seed guard exists because
  round 33 scored a partial log as a perfect result.

### Blast radius

A **fancy** screen with a prepayment series at `peryr ∈ {26, 52}` still live when
the walk crosses DOS's Julian ceiling (day 70000 = **26 August 2091**, §69), on
any walk driven by `Iterate` (solve payment, solve rate, solve amount,
`reAmortize`). `peryr ∈ {1,2,3,4,6,12,24}` is immune — those `AddPeriod` arms are
field arithmetic and never call `MDY`.

`balloon_off_grid` already excludes balloons dated past the term, so a balloon
cannot practically drive the horizon there; a **long term** is the remaining
driver, and `nperiods_gt_max` at 10,000 is far too loose (900 monthly periods
already reaches 2100). fuzzer5 does not currently generate the shape unaided,
which is why the suite is green; `goamort` reaches it in one line.

**Measured: the round's recommended remedy is NOT blocked by this.** A probe with
`adjustment_carries_amount_ao6` and `balloon_plus_ao6_or_ao7_adjustment` both
disabled, carrying a wall detector and a 200,000-iteration canary, over 22 seeds ×
400 cases: **0 wall hits, 0 runaways.** The wall is a prepayment-`peryr` ×
horizon defect and is orthogonal to adjustment shape.

### What round 34 landed

**Only a net.** `dosport_walk.go` gained an iteration bound (2,000,000, far above
`MaxSchedulePeriods` × any legitimate steps-per-period) that sets the walk's
existing `e.abort`/`e.errorflag`, so the caller reports a failure instead of
looping. The runaway screen now returns `ERR payment solve did not converge` in
4.7 s. Three controls byte-identical before and after. The regression test was
seen to FAIL (45 s deadline, no return) on a probe tree with the bound removed.

### The fix, not applied

1. `dosport.go` `checkOffBalloon` — adopt the poisoned date:
   `nd, _ := dateutil.AddPeriod(...); pp.nextdate = nd`.
2. `dosport_entry.go` `buildDosEng` — same, and set `stopOK = true` unconditionally.
3. `dosport_walk.go` — port AMORTOP.pas:1143-1147's clamp:
   `if !dateutil.DateOK(e.stopdate) { e.stopdate = firstdate with y = 100+pred(centurydiv) }`
   (`centurydiv = 50`, PEDATA.pas:67,697 → 2049). Read it from settings if configurable.
4. Consider also AMORTOP.pas:1197-1198 (`if (overflowflag) then exit` INSIDE the
   walk; the port only tests it one level up in `iterate`) and :1232-1233's
   "last payment not found" advisory, which the port does not emit.

A prototype of 1-3 was verified in both directions at the round-34 audit: the
runaway terminates, and two oracle-exact controls do not move —
`pre=100:52:52:1.36` → `payment 772.7063 interest 190704.57 paid 238836.96` and
`pre=854:246:12:1.36` → `payment 773.2638 interest 191140.68 paid 239273.07`,
both equal to the oracle. fuzzer5 ledgers on seeds 44016 and 913 are identical
with and without it.

### ⚠️ What is NOT known

**What DOS PRINTS on these screens has not been adjudicated.** `EMessage` is
commented out in the shipped `VIDEODAT.pas:87-100` and does not set `errorflag`;
the oracle harness escalates it to `noteError → Halt`
(`legacy/oracle/Globals.pas:108-111`, `amort_oracle.pas:1105-1109`). A real DOS
session would flash the line-25 message and continue to the 2049-clamped
schedule. So the TERMINATION claim is adjudicable and the ANSWER claim is not —
which is precisely why round 34 landed the net and not the fix. The harness
already classes the oracle's `Bad date passed to Julian function` as
**date-horizon = indeterminate**, not a refusal the port must mirror
(`dos_fuzzer5_test.go:73, :556`), so the port answering here would not be a
`go_solved_dos_refused`.

### R30

**A ROUTER CLAUSE CAN BE LOAD-BEARING FOR A REASON UNRELATED TO ITS NAME, AND
THAT REASON CAN BE A DEFECT ELSEWHERE.** R28 asked whether each clause is FIDELITY
(DOS genuinely differs) or SCOPE (the port is merely unvalidated).
`in_advance_or_r78_or_daily`'s R78 term is NEITHER: as fidelity it is
demonstrably wrong (two screens adjudicated against the oracle, one of them a
live `go_solved_dos_refused`), as scope it is obsolete, and it is currently
coupled to an open engine defect. **Classify a clause by MEASURING WHAT HAPPENS
WHEN IT IS REMOVED — and measure TERMINATION, not just the answer.**

### ROUND 35 — HOW §71 WAS CLOSED

#### (a) The driver, and why the default did not move

The obstacle was never the engine. `Julian` (VIDEODAT.pas:359-364) is

```pascal
if (m>13) or (m<1) then begin
   EMessage('Bad date passed to Julian function: m=',m);
   daynumber:=-88;
   end
```

— **no `exit`, no `errorflag := true`.** Control falls straight through and the
caller carries on. Both shipped implementations of `EMessage` agree that it is a
NOTIFICATION and not a refusal:

| build | body | file:line |
|---|---|---|
| DOS | paint row 25, `ReadKey`, restore row 25, return | VIDEODAT.pas:86-100 (commented out in the shipped tree; the call resolves to Globals') |
| Win32 | `MessageDlg(Output, mtError, [mbOK], 0)` — modal, one OK button, result not inspected | `dos_source/Globals.pas:98-104` |

The oracle's `Globals.pas` routed it to `noteError`, which latches
`OracleErrorFired`, which makes `amort_oracle.pas:1105-1109` print `ERR` and
`Halt(0)`. **R26's fifth site, and §3b item 11 predicted it.**

It is fixed **behind `PERSENSE_ORACLE_SOFT_EMESSAGE=1`, not by default.** The
default is load-bearing for §47/§69's Julian-ceiling work and for
`presentvalue/zzjulian_ceiling_test.go`, and lifting it by default converts an
INDETERMINATE population (`dos_fuzzer5_test.go:72-73` — "Date-horizon breakdowns
are indeterminate, not refusals") into a COMPARED one on several surfaces at
once. **That is a measurement change, not a fix, and it needs its own round.**
Verified: with the gate unset, the post-fix oracle's stdout AND stderr are
byte-identical to the pre-fix build on the repro screen
(`amort_oracle` md5 `b1301ec33f9a16b0b2eeea468c15667e` → `cbe3aa4c45e146d39e30d4afd3278e60`).

The one message this screen fires is `m=-88` — `unkbyte`, the UNKNOWN-date
sentinel read through a signed field, **not** MDY's `errorbyte` (-99). So DOS's
single notice here is `Julian` on a blank date, not on a poisoned one.

#### (b) The three sites, each with its own negative control

Reverting each site individually in a probe tree (note #28) and running the §71
tests:

| site | file | reverted alone | verdict |
|---|---|---|---|
| 1 | `dosport.go` `checkOffBalloon` adopts the poisoned `nextdate` | tests **PASS** | inert *on the repro screen* — see below |
| 2 | `dosport_entry.go` `buildDosEng` adopts the poisoned `stopdate` and sets `stopOK` unconditionally | **FAIL** — 501792.40, want 126970.33 | independently necessary |
| 3 | `dosport_walk.go` the AMORTOP.pas:1143-1147 horizon clamp | **FAIL** — 501792.34, want 126970.33 | independently necessary |
| all three | — | **FAIL** — round 34's net fires, `payment solve did not converge` after 6.9 s | the negative direction, in fact |

**Site 1 looked inert and nearly did not land.** A 150-screen ceiling-arm
differential of "sites 2+3 only" against the full fix returned FIXED=0, NEW=0.
By rule 16 that is an unconfirmed change.

**The source says exactly where it bites, and the sweep confirms it.** Sites 2+3
terminate the repro by clamping at **2049**, which is EARLIER than the
**26-Aug-2091** ceiling, so the cursor is normally retired long before it can
freeze — and the clamp only engages when `stopdate` is INVALID. The exposed band
is therefore: a series whose own stop date is still VALID (hence at or just under
the ceiling, because `AddNPeriods`' 26/52 arm went through `MDY`) while the
schedule's `very_last` is valid and LATER. There the cursor advances to within
one week of day 70000, `AddPeriod` overflows, and with the `err == nil` guard in
place it FREEZES at a date still ≤ its own stopdate: never retired, re-emitted
forever, walk invariant.

Sweeping the series length on the repro screen (`pre=12:<nn>:52:10`):

| nn | sites 2+3 only | full fix | DOS |
|---|---|---|---|
| 3384 | 501782.70 | 501782.70 | 501782.70 |
| **3385** | **`ERR did not converge`** | **501792.40** | **501792.40** |
| 3386 | 126970.33 | 126970.33 | 126970.33 (gate) |

**ONE value of nn wide.** Below it nothing overflows; above it the stop date is
itself poisoned and site 2 hands the walk a wall. At exactly the boundary site 1
is the only thing between the shipped product and a hang — and the answer it
produces is DOS's, to the cent. `TestSec71CursorFreezeBoundary` pins it, and it
was seen to FAIL on the site-1 probe tree.

#### (c) The measurement — a new instrument for a family the fuzzer cannot draw

`testplan/harness/sec71_ceiling_arm.py`. `dos_fuzzer5` does not generate this
shape unaided, so `paired_regression.sh` measures the fix's COLLATERAL and this
arm measures its EFFECT; neither substitutes for the other. It runs PRE, POST and
the oracle on identical tokens, scores FIXED / STILL / NEW / SAME_OK / BOTH_ND,
and — note #29 — **buckets a timeout as HANG rather than letting a killed run
read as a pass.**

500 screens, seed 71, **post-audit instrument and post-audit engine**:

| bucket | all | julian arm (26/52) | field arm |
|---|---|---|---|
| **FIXED** | **47** | **47** | **0** |
| **NEW** | **0** | **0** | **0** |
| **HANG (POST)** | **0** | 0 | 0 |
| SAME_OK | 387 | 53 | 334 |
| STILL | 63 | 15 | 48 |
| BOTH_ND | 3 | 0 | 3 |

**Every fix is on the Julian arm and the field arm is inert** — the effect lands
exactly where the mechanism says it must, and nowhere else.

⚠️ **THE PRE-AUDIT VERSION OF THIS TABLE READ `FIXED 33 / STILL 122 / SAME_OK 341`
AND IS SUPERSEDED IN FULL.** It was measured with a generator that drew 94 of its
500 screens with impossible dates (31 April), which `cmd/goamort` refuses with an
empty stdout and the oracle answers — so all 94 scored as unfixed divergences.
The Julian arm was 90 comparable screens, not 115. **Do not quote 33 or 122.**

#### (d) §35A — WHAT THE INDEPENDENT AUDIT FOUND, AND WHY IT IS THE ROUND'S MOST IMPORTANT RESULT

Two independent adversarial reviewers were run against the round's changes after
every gate was green. **Between them they moved every published figure in this
section and found a real engine defect.** Recorded in full because the pattern
matters more than the round.

**ONE DEFECT IN THE FIX ITSELF, which every gate passed over.**

`dosport_walk.go` built the fallback wall from `firstdate.Time.Day()` — the
CLAMPED day. DOS's clamp is a straight record copy, so the wall inherits
firstdate's day field VERBATIM, phantom and all, and `e.subFirstDay` is the port's
carrier for exactly that raw day twenty lines below in the same function.
`backward.go:1657-1662` had documented the hazard and was not consulted.

Incidence **18 of 122** randomized poisoned-prepay + rate-adjustment screens.
Repro, verified both ways:

```
goamort 150000 0.06 900 12 plusreg loandmy=30.12.2025 firstdmy=30.1.2026 \
  pre=3:6000:52:5 adj=13:0.09:
  ORACLE          200580.20
  clamped-day     200576.83     <-- what the round would have shipped
  raw-day (fixed) 200580.20
```

What passed over it: the full suite `-count=1`, `check_skips` 32/32,
`paired_regression` NEW=0, all five §71 tests, AND the 500-screen ceiling arm.
**Six gates, none of which drew a sub-walk with a phantom anchor.**

**THREE DEFECTS IN THE NEW INSTRUMENT, all found before its numbers were used
for anything except this document.**

| # | defect | effect |
|---|---|---|
| 1 | generator drew `day ∈ {1,15,17,28,29,31}` against a free month, producing 94 impossible dates in 500 | `goamort` exits 2 with empty stdout, oracle answers → all 94 scored STILL. **19% of the sample measured the harness** |
| 2 | the verdict keyed FAIL on NEW and hangs only | run against a COMPLETELY UNFIXED binary it printed **PASS** — an instrument built to confirm an effect (rule 16) could not see the effect vanish |
| 3 | "POST hangs 0" was a tautology | round 34's iteration bound guarantees a return in ~7 s, so `--timeout` can never fire. The note-#29 canary was decorative |

And when defect 3 was "fixed" by keying on the port's `did not converge` string,
the instrument immediately produced a **FALSE ALARM** — three screens flagged
"§71 is back" that were in fact PRE, POST and DOS all declining identically.
The port emits that string both for round 34's net AND for its ordinary Newton
giving up, which is DOS's own
`Computation of payment amount or interest rate did not converge.` **The string
cannot separate them; only "did DOS answer?" can.** Fixed by moving the check
below the oracle's verdict. *A harness suspect before the engine (rule 12), in
the direction that manufactures a beautiful finding.*

**TWO FALSE CLAIMS IN THE ROUND'S OWN TEST FILE**, both removed: that the pre-fix
binary "does not return at all" (round 34's bound had already made it return in
~6.5 s), and two goamort md5s offered as provenance (Go embeds build paths, so
they reproduce for nobody).

**AND TWO SCOPE LIMITS ON THE "§71 CLOSED" CLAIM, which stand:**

1. **§71 is closed for the DOS-port slice only.** The `adjustment_carries_amount_ao6`
   router clause sends every amount-carrying-adjustment screen to the PIECEWISE
   engine, which has no counterpart to AMORTOP.pas:1143-1147. Measured: 40/40 and
   100/100 divergent against the oracle on poisoned-stop AO6 and rate+amount
   screens, byte-identical before and after this round. The three changed files
   are not on that path. **This is §3b item 2's widening, and it now has a second
   reason to happen.**
2. **On some adjustment screens the fix converts a hang into a wrong answer that
   no longer announces itself.** DOS aborts its walk when Re_Amortize's inner
   Iterate fails; the port converges and emits a full schedule. Example: oracle
   4 rows / interest 5,649.91, port 1,372 rows / interest 544,560.00, with no
   error flag. **Termination was bought, fidelity was not.** Filed as an open
   item on §72.

**The reason this section exists.** Every gate this project owns was green on a
tree with a real arithmetic defect in it, and the instrument that was supposed to
prove the fix worked was itself wrong in three ways — one of which made it report
success on an unfixed binary. **An adversarial audit AFTER the gates are green is
not a formality; this round it was the only thing that worked.**

---

---

## §73 — `types.DateRec` CANNOT REPRESENT 29 FEBRUARY 2100, WHICH DOS'S CALENDAR SAYS EXISTS (2026-08-05, round 36)

**STATUS: OPEN, ROOT-CAUSED, NOT FIXED.** Found by the round-36 adversarial
audit while attacking that round's own claim about §72's boundary.

### The defect

DOS's leap rule is `(y mod 4 = 0)` with no century correction
(`VIDEODAT.pas:341-346`, `DaysInM`). The port ports that rule **faithfully** —
`internal/dateutil/dateutil.go:114-125`, `daysInMonthPascal`, returns 29 for
February of any year divisible by 4. But `types.DateRec` is `time.Time`-backed,
and `time.Date` is **proleptic Gregorian**: it cannot hold 29 February 2100.

```
DOS daysInMonthPascal(Feb, py=200) = 29 | types.NewDateRec(2100,Feb,29) -> 2100-03-01
DOS daysInMonthPascal(Feb, py=300) = 29 | types.NewDateRec(2200,Feb,29) -> 2200-03-01
DOS daysInMonthPascal(Feb, py=100) = 29 | types.NewDateRec(2000,Feb,29) -> 2000-02-29   (control)
DOS daysInMonthPascal(Feb, py=500) = 29 | types.NewDateRec(2400,Feb,29) -> 2400-02-29   (control)
```

`CheckForDaysTooLarge` never fires, because 29 is not larger than DOS's own 29.
**The roll is silent.** After it, a monthly series anchored on day 29 has its
day-of-month permanently shifted to the 1st and every subsequent row diverges.

Row-level proof, from the round-36 ceiling-family population:

```
DOS  L1131| 1/29/00 ...   L1132| 2/29/00 255.15 187.60 67.55 ...   L1133| 3/29/00 ...
PORT row 1132 1/29/0 ...  row 1133 3/1/0  255.17 199.2573 55.9130 ...
```

### It is a REPRESENTATION defect, not a leap-rule port defect

This distinction matters because the obvious fix is wrong. The port's leap
arithmetic already agrees with DOS. What disagrees is the CONTAINER. Any fix has
to give `types.DateRec` a representation for dates DOS's calendar admits and
Gregorian does not — which is exactly the change START_HERE §7 lists as **"not
on the list by decision: giving `types.DateRec` raw y/m/d fields."** That
decision was taken without this defect on the table and should be re-taken with
it.

### It is the mechanism under §62

§62 has said "THE PORT CARRIES TWO CALENDARS. They disagree at 2100" since round
21. This is *why*. The trigger is precise: **a payment landing in February of a
year divisible by 100 but not by 400, with day-of-month >= 29.**

Ablations (round-36 audit), on
`403901.74 0.0926 240 12 plusreg loandmy=<D>.8.2024 firstdmy=<D>.9.2024 pre=854:NN:6:3683.25 exact`:

```
day=15 NN=52  horizon 2104  delta      +0.00     <- crosses 2100, runs to 2104, EXACT
day=15 NN=246 horizon 2136  delta      +0.00     <- runs to 2136, EXACT
day=29 NN=52  horizon 2100  delta -89085.58
day sweep at NN=52 (series lands in Feb): 27 -> +0.00  28 -> +0.00  29 -> -89085.58  30 -> -89085.59
phase-shifted so the series never lands in February (Sep start / Jul start): +0.00 / +0.00
basis ablation at day=29: 30/360 +0.07, exact +0.07, b365 +0.07, usa +0.10  -> `exact` is NOT the cause
```

**A series that starts past the term, crosses 2100 and runs to 2136 agrees to
the cent — provided the day is 15.** Crossing 2100 is not sufficient; landing on
29 February 2100 is.

### ⚠️ AND IT CORRECTS ROUND 36's OWN FIRST READING

Round 36 initially reported the boundary as "the prepayment series' STOP DATE
crossing 1 January 2100" on the strength of an nn bisection (nn=26, stop
29.12.2099, exact; nn=27, stop 29.2.2100, diverges). That bisection was run at
day-of-month 29 throughout, so it varied the date and held the trigger fixed.
The day sweep above is the ablation it was missing. **A mechanism found on one
case is scoped by that case's accidents (R23), and the accident here was the
day.**

### Scope

Out of scope for the client comparison boundary (2099) *as a whole-case label* —
but see §72: the divergence is observable on rows dated decades earlier, because
the payment is solved over the whole horizon. The 2200 case is out of every
scope this project has. The 2100 case is one calendar month past the boundary.

---

## §72 — THE FAITHFUL PORT'S IN-SCOPE ZERO IS A PROPERTY OF THE FUZZER'S SAMPLE SPACE, NOT OF THE PORT (2026-08-05, round 35)

**STATUS: OPEN. Three in-scope divergences located, none mechanised.** No net,
no fix; this section exists so the claim in START_HERE §2 is not carried forward
unqualified.

### The claim it qualifies

Round 34 measured the faithful port (`dosport`) at **0 divergences in 1,707
in-scope compared cases** and published `≥99.825% one-sided`. That measurement is
sound — and it is a measurement over **`dos_fuzzer5`'s sample space**.

Round 35 adjudicated a family that sample space cannot draw (§71's, above) and
compared 500 screens nobody had compared before. Splitting by era on the port's
own resolved `lastdate` (R2) and keeping only screens `DPTRACEENGINE=1` shows
were answered by `dosport`:

| population | diverged | compared | rate |
|---|---|---|---|
| **ceiling family, IN SCOPE ≤2099** | **3** | **255** | **1 in 85** |
| ceiling family, OUT OF SCOPE | 18 | 116 | 1 in 6 |
| *(r34, fuzzer5, in scope)* | *0* | *1,707* | *none* |
| *(r34, fuzzer5, out of scope)* | *3* | *91* | *1 in 30* |

⚠️ **DIFFERENT POPULATIONS — DO NOT POOL THEM** (CAUTION 1, rule 9). The r34 rows
are `dos_fuzzer5` draws; the r35 rows are `sec71_ceiling_arm.py` draws, which
oversample long terms and long prepayment series ON PURPOSE. The r35 rate is not
an estimate of anything the client sees; it is evidence that **the zero was a
statement about the generator.**

### The three

None needs `PERSENSE_ORACLE_SOFT_EMESSAGE` — the DEFAULT oracle answers all
three, so they were adjudicable at any point in the last twenty rounds and simply
were never drawn.

```
333366.23 0.0575 700 12 plusreg loandmy=15.3.2024 firstdmy=15.4.2024 \
  pre=240:20000:24:3717.61
  last 2082   port 4971260.70 / 5304626.93   DOS 4971256.02 / 5304622.25   (+4.68)

403901.74 0.0926 240 12 plusreg loandmy=29.8.2024 firstdmy=29.9.2024 \
  pre=854:52:6:3683.25 exact
  last 2044   port  578917.36 /  982819.10   DOS  668002.94 / 1071904.68   (-89,085.58)

483080.02 0.0839 480 12 plusreg loandmy=29.5.2025 firstdmy=29.6.2025 \
  pre=854:246:12:1137.73 usa b365
  last 2065   port 1469521.35 / 1952601.37   DOS 1470630.87 / 1953710.89   (-1,109.52)
```

**⚠️ NOT §71, and the pairing is the point.** Their prepayment series are at
peryr **24, 6 and 12** — the FIELD arms of `AddPeriod`, which never call
`Julian`/`MDY` and cannot reach the ceiling. §71's mechanism cannot produce them.
What they share is the other axis: **a prepayment series starting far past the
entered term** (`pre=854:...` on a 240- and a 480-period loan) or running far past
it (20,000 semi-monthly payments). Two of the three are large — 89,085 and 1,109,
not rounding — and the third (4.68 on 5.3M) may be a tolerance artefact and should
be adjudicated before it is counted.

### Why this matters more than three cases

`docs/fuzzer_sample_space_audit_2026-08-02.md` listed **"a prepayment series
starting past the entered term"** as a SILENT axis. §71 lived there. These three
live there. **Rule 8's question — what can the generator NOT produce? — is now 10
for 10.**

### What is owed

1. **Adjudicate the +4.68 case** before it is counted (a tie is not a divergence,
   and a half-cent tie is not either).
2. **Mechanise the other two.** `DPTRACEENGINE=1` first (R27), then
   `localise_divergent_row.py`.
3. **Widen `dos_fuzzer5`'s prepayment axes** — start period past the term, and
   `nn` beyond the schedule — so this family lands in the STANDING denominator
   instead of a bespoke arm. That is §3b item 4's method applied to the axis that
   has now paid twice.
4. **Re-state the faithful port's bound with its population named.** "0 in 1,707"
   is true of `dos_fuzzer5` in scope and must never again be quoted as "the
   faithful port has no in-scope divergences."

### Also filed on §72 (found by the round-35 audit, not chased)

1. **`checkOffBalloon`'s `!stopOK` fallback is not DOS.** DOS's bare `stopdate` at
   AMORTOP.pas:560 binds to `pre[i]^.stopdate` through `with pre[i]^ do` (:558) —
   RepayFancyLoan's local of the same name is in a SIBLING procedure and not in
   scope. DOS therefore retires a series against its OWN stop date always, and
   for an unbounded series that field is `unkbyte` (:434), which DateComp never
   reports as later — so DOS never retires one there. The port falls back to the
   walk horizon. Left exactly as it was (round 35 briefly changed it on a wrong
   reading and reverted), and unreachable in practice now that buildDosEng sets
   `stopOK` whenever `nn > 0`.
2. **A latent PV hazard at dosport_entry.go's discount site.** It guards
   `!DateOK(stop)` but falls back to `e.veryLast`, which round 35 can now also
   poison; `YearsDif(zeroDateRec, repayFrom)` measures **-2024.79 years** and the
   annuity seed reaches ~1e70. An ablation falling back to `loan.LastDate`
   produced identical output on 91/91 DOS-port cases — Newton recovers every
   time — so it is a hazard, not a defect. **Fix it before it is a defect.**
3. **`e.stopWallOn` is not saved/restored around nested walks**, exactly as
   `e.stopdate` is not. An ablation saving both produced zero output changes
   across 213 targeted cases including 3-adjustment screens. Hygiene, not a
   defect — but DOS's `stopdate` IS a genuine local and the port should say so.
4. **Termination bought without fidelity on adjustment screens** — §35A's second
   scope limit, above. This is the sharpest of the four.

### R31

**A ZERO IS A STATEMENT ABOUT THE GENERATOR UNTIL A SECOND, INDEPENDENT GENERATOR
REPRODUCES IT.** Round 34 already knew a zero can be era-conditional (the faithful
port is 0 in scope and 3 in 91 out of scope) and split the eras. It did not ask
the prior question: whether the SAMPLE SPACE the zero was measured over can reach
the shapes that break it. It could not — the very family round 34 had just found a
non-termination in was one the generator does not draw. **Before quoting a zero,
name the generator, and widen it once in the direction of the most recent defect.**


---

### ⚠️ ROUND 36 — §72 IS RE-KEYED, NOT RETRACTED, AND THE CORRECTED RATE IS WORSE

**Round 35's era split was keyed on `goamort bdump`'s `lastdate`** — the last
REGULAR payment date. The three cases above are annotated `last 2082`,
`last 2044`, `last 2065` for that reason. `lastdate` is not the walk's horizon:
the port's own `fz5MaxYear` horizons for those three are **2109 / 2100 / 2116**,
so all three are OUT of scope under the definition every published in-scope
figure in this project is built on. `cmd/goamort` grew a `horizon` token this
round so an arm can ask the port for its own keys (R2), and
`zzhorizon_key_test.go` pins the token to `fz5MaxYear`.

**But `fz5MaxYear` is biased the other way, and that is the round's real
finding.** It takes `max(last schedule row, balloons, resolved LastDate)`. The
last term is the loan's NOMINAL last regular payment date, which a
prepayment-retired schedule **never reaches**:

```
233825.48 0.0567 900 12 plusreg loandmy=29.11.2026 firstdmy=29.12.2026 pre=1:246:24:4199.15 usa
  horizon 2101   reached 2030   lastdate 2101
  BOTH engines: 97 rows, last row 12/29/2030, balance 0.00
  port interest 28,459.75   DOS 28,450.87   delta $8.88
```

A four-year loan, divergent, excluded from the in-scope population because of a
date **71 years after the last row either engine prints**. The ratified boundary
(`claude/decisions_2026-08-03b_client_2099_boundary.md`) is about the dates the
schedule REACHES.

**The ceiling family, re-measured on all three keys** (`sec72_horizon_arm.py`,
`sec71_ceiling_arm.build_screen`, seed 71, n=500):

| key | engine filter | IN SCOPE diverged / compared | rate |
|---|---|---|---|
| lastdate (round 35's) | dosport | 4 / 302 | 1 in 76 |
| **horizon** (fz5MaxYear) | dosport | **0 / 263** | none |
| **reached** (what the walk produces) | dosport | **5 / 292** | **1 in 58** |
| lastdate | ALL engines | 7 / 327 | 1 in 47 |
| horizon | ALL engines | 3 / 288 | 1 in 96 |
| **reached** | **ALL engines (the product)** | **10 / 319** | **🚨 1 in 32** |

⚠️ **THE `dosport` FILTER IS NOT NEUTRAL.** 38-39 of the 500 screens route to the
PIECEWISE fallback, which has no horizon clamp at all (§35A scope limit 1). The
in-scope divergences it carries are large — one is a factor of 2.7. A rate quoted
over `dosport` alone is a rate over ~96% of the router, not over the product.

**So: round 35 was right that this family has in-scope divergences and wrong
about which ones and how many. The corrected product-level in-scope rate on the
ceiling family is 1 in 32, worse than round 35's 1 in 85 — and it is not the
three cases §72 names.**

### What round 36 settled, and what it did not

**Settled:**

1. The mechanism. It is **§73** — `types.DateRec` cannot hold 29 February 2100.
   Not "a prepayment series starting past the term": `pre=241` and `pre=300`,
   both past the term, agree exactly, and a series running to 2136 agrees exactly
   at day-of-month 15.
2. §72's three cases are out of scope on every horizon-flavoured key. The
   +4.68 case is **adjudicated NOT A DIVERGENCE**: row 0 is identical in both
   engines (payment -749.41), and the first difference is one cent in the
   PRINCIPAL column where the port is arithmetically exact
   (-749.41 - 1608.62 = -2358.03) and DOS renders -2358.04. That is the standing
   "DOS's row figures are its RENDERED CENTS" trap. **The count of located
   divergences is TWO, not three.**
3. `dos_fuzzer5`'s prepayment axis is widened (a series starting past the term is
   now drawn 1 time in 8), so this family lands in the standing denominator.

**Not settled:**

4. **The standing table has not been re-measured on the widened generator.**
   475 in 34,967, the contingency table, the co-exclusion profile and
   `dosport 0 in 1,707` are all **pre-widening figures over a strictly narrower
   generator** and must not be quoted as current. Head-to-head at 25 seeds x
   N=800 the in-scope HARD rate moved 1.458% -> 1.069% and the divergent case
   sets are DISJOINT — the widening buys out-of-scope coverage, and the
   improvement must not be read as the port getting better.
5. **Whether `reached` should replace `horizon` as the standing key.** It should
   — the decision says so — but changing it re-measures every published in-scope
   figure at once, which is a measurement change owed its own round.
6. **An in-scope `dosport` REFUSAL against a DOS answer** (seed 90210,
   `301887.36 0.1057 900 12 plusreg loandmy=30.10.2024 firstdmy=30.11.2024
   pre=600:1000:3:819.78 usa b365`; DOS's whole schedule ends 2051). That is
   §71's own class, in scope, on the faithful engine.
7. §73 itself.

### R31, after this

Round 35 drew R31 from the contrast `0 in 1,707` vs `3 in 255`. Re-keyed, that
contrast is `0 in 1,707` vs `0 in 263` on the same filter — **the specific
numeric argument for R31 is withdrawn.** R31 still stands on §71 (a
non-termination on a family `dos_fuzzer5` provably could not draw) and on the
out-of-scope stratum gap (`dos_fuzzer5` 1 in 30 vs ceiling family 1 in 4). The
rule survives; its headline evidence does not.

⚠️ And **0 in 263 licenses only >=98.86% one-sided** (2.995732/263). The exit
criterion's 1-in-400 bar needs N >= 1,199 at zero events. On §72's own late-start
axis the in-scope denominator is **58 screens**. No "the faithful port has no
in-scope divergences" claim is supportable from this round.


---

## §74 — the oracle driver returns a DIFFERENT ANSWER TO THE SAME INPUT when a date token is malformed (round 41, OPEN)

**Severity: this is in the AUTHORITY BINARY.** Every published number in this
project is a comparison against `amort_oracle`. A path on which that binary is
nondeterministic is a path on which "the port diverged" and "the oracle diverged"
are indistinguishable.

**The mechanism.** `ParseDMY` (`legacy/oracle/amort_oracle.pas:147-160`) `exit`s
at `:151` and `:154` — the "fewer than two dots" arms — **without ever writing to
its `dr` out-parameter.** Its callers do not initialise the record first. At
`:1120` the `payoff=` handler does `New(w); w^.amount := 0; ParseDMY(..., w^.date);`
and then `w^.datestatus := inp`, so a freshly-allocated, *uninitialised* heap
record is marked as a user-supplied input date. `w^.amount` is initialised;
`w^.date` is not.

**Reproduction** (round-41 audit, 40 identical invocations of the same argv):

```
for i in $(seq 1 40); do amort_oracle 250000 0.0725 360 12 payoff=24; done | sort | uniq -c
     37 payoff 0.0000
      1 payoff 5169.1378
      1 payoff 91462.8442
      1 ERR It makes no sense to ask for the loan balance before the loan date.
```

`payoff=1.2` behaves identically. A well-formed `payoff=1.1.2026` is stable
(25/25 → `payoff 88410.8100`).

**Scope, stated honestly.**
- **NOT a round-41 regression** — it is present in the pre-round binary too.
- **No published number is known to be affected**: nothing in the tree emits a
  malformed `payoff=`, and the well-formed path is deterministic (verified over a
  220-case × 3-repeat sweep and a 1,150-case cross-product).
- **But it invalidates a METHOD, not just a path.** "Ran an A/B corpus once
  against each binary, 0 differing" cannot distinguish *unchanged* from
  *nondeterministic and it happened to agree*. Every rule-7 verification this
  project has ever done was single-shot. `scripts/rule7_mordmy_corpus.sh`
  (round 41) therefore repeats each line `REPEATS` times, default 3.
- **Same shape, one notch quieter:** `:1187-1191` (`datefrombalance=`) discards
  `Val`'s error code `ec`. Empirically deterministic (`datefrombalance=abc` →
  `date 1/1/2034 status 1`, 12/12) but it is the identical silent-default pattern.

**Not fixed this round, deliberately.** The fix is to make `ParseDMY` write a
sentinel on every exit path and to make its callers refuse — which is a rule-7
decision about several tokens' accept behaviour, not a typo fix, and it wants its
own round with a corpus. Filed here so it is not rediscovered.

**Standing trap added:** *AN UNINITIALISED OUT-PARAMETER MAKES THE AUTHORITY
NONDETERMINISTIC, AND A SINGLE-SHOT A/B CORPUS CANNOT SEE IT.*

---

## NF-6 — CLOSED (round 41)

The `mordmy=` token's year bound was `yv <= 2200` while `daterec.y` is a BYTE
based at 1900, so 2156-2200 passed the check and wrapped (2190 → 1934). Fixed at
`amort_oracle.pas`: the bound is now 2155 **and an out-of-range or unparseable
value is a loud `ERR … Halt(0)` refusal** rather than a silently-ignored token —
narrowing the bound alone would only have traded a wrong-century answer for a
silently-moratorium-free one (measured: `mordmy=15.2.2190` returned the plain
loan's `payment 1321.5074` under the narrowed bound alone).

**Rule 7:** default stdout unchanged on every well-formed command line — 378-line
corpus (`scripts/rule7_mordmy_corpus.sh`, checked in so the claim is
reproducible), 0 differing, 0 nondeterministic at REPEATS=3; independently
confirmed by the round-41 audit over a 1,150-case cross-product.

⚠️ **The first version of the in-source rule-7 claim was WRONG and is corrected in
place (R43).** It said the refusal "cannot fire on any input the old binary
ACCEPTED". The audit found two counterexamples where the old binary answered
CORRECTLY and the new one refuses the whole line — a duplicate `mordmy=` where one
token is junk, and `mor=6 mordmy=0.0.0`. Nothing in the tree emits either.

⚠️ **The exposure is wider than NF-6 said.** `mordmy=` is the ONLY date token in
the driver that range-checks its year at all; `bdate=`, `adjdmy=`/`adj=`, `pre=`'s
start date and the loan/first dates (`:159, :208, :289, :404`) all do `- 1900`
with no bound. **No live measurement is corrupted** — `dos_fuzzer5` draws loan
years 2023-2025 only, and where it constructs far-future dates it *deliberately
mirrors the wrap* (`dos_fuzzer5_test.go:3144-3153` applies `py = ((py%256)+256)%256`
so the emitted token and the Go-side record carry the same wrapped value). Filed,
not fixed, for the same rule-7 reason as §74.

---

## §75 — THE PRESENT VALUE SCREEN SOLVES THE PAYMENT ON DEATH, PUTS IT ON THE WIRE, AND NOBODY READS IT (2026-08-09, round 42) — FIXED

**This is R39 on the PV screen**, found by asking the amortization screen's
round-39 questions of the PV screen's stacked advanced options.

Leaving the Payment on Death cell blank while a target Sum Value is typed asks
the engine to solve for the death benefit. DOS does this in
`ComputeUnknownPOD` and then **paints the answer onto the screen**:

```pascal
{ PRESVALU.pas:1268-1269 }
if (podunk) then ComputeUnknownPOD;
PlacePODOnScreen;
```

`PlacePODOnScreen` is UNCONDITIONAL — it follows the solve every time.

The port computes it correctly (`presentvalue.solveUnknownPOD`), and the API
has carried it since the field was added: `PVResponse.POD`,
`json:"pod,omitempty"` (`internal/api/handlers.go:534`, populated at `:2005`).
**No consumer read it.** The only POD-ish reader in `index.html` was
`data.podValue` (the POD's present value, a different number). `#actu-pod` was
read four times and cleared once and never written. So the one number the
calculation existed to produce stayed invisible and the cell stayed blank.

**MEASURED.** Rows-only PV 33,418.5613; target +10,000; the engine solves a
face amount of **52,870.3135** whose present value is exactly the 10,000.00
gap. That number was reaching the browser and being discarded.

**FIXED** in `cmd/persense/static/index.html`: `getPVInput` records whether the
request went out without a `pod` (`pvPodBlank`), and `calcPV` paints
`data.pod` into `#actu-pod` with the green cell-output style and a
full-precision `dataset.pvRaw` stash, exactly as the row grids do.

**Guarded:** `internal/api/zzr42_pv_stacking_test.go`
`TestR42_SolvedPODIsOnTheWireAndIsRight` (the wire contract and the
arithmetic, including a non-vacuity assertion that the face exceeds its own
present value) and `cmd/persense/frontend_pv_pod_echo_test.go`
`TestR42PVPaintPairing` (the consumer). **Both seen to fail** against mutants.

⚠️ **Two follow-ons the round-42 audit found in this fix, both fixed:**
1. `#actu-pod` was NOT in the global input handler's `cell-output` drop list
   (`index.html` ~7264): the row grids are covered by a
   `closest('#pv-ls-body, #pv-per-body')` match and `actu-pod` is a sibling of
   `actu-now`, matched by neither. Reading the `pvRaw` stash while the cell is
   green would therefore have made a POD the user typed over a painted one
   **shown but never transmitted, forever**. The cell is now on the list and
   the stash is deleted with the green.
2. The paint is floored at half a cent — see §78.

---

## §76 — A VARIABLE-RATE SCHEDULE SILENCED EVERY ADVISORY THE SAME WORKSHEET RAISES AT A FIXED RATE (2026-08-09, round 42) — FIXED

`Calculate`'s variable-rate branch (`presentvalue/calc.go` ~786) `return`ed
from each of its four arms, which stepped over `appendResultAdvisories` at the
bottom of the function. **Stacking a rate schedule onto a worksheet therefore
removed the advisory channel entirely.** `advisories.go`'s own doc comment
recorded the bypass as though it were a design note: *"The variable-rate and
POD early-return paths in Calculate bypass this pass."*

**MEASURED, with a control (R19/R24).** One worksheet, two lump rows, the
second with a blank amount and a target equal to the first row's value alone,
so the solved amount comes out ~0 and P-W4 must fire:

| arm | warnings |
|---|---|
| fixed rate | `P-W4 the solved amount is essentially zero …` |
| **+ rate schedule, nothing else changed** | **none** |

**FIXED**: the arms assign into a local and the pass runs once on the way out.
No arithmetic moved (asserted). The dispatch is unchanged — verified
independently by the round-42 audit, including that `vrUnknownAmount` and
`vrUnknownDate` take a `*PVInput` but only READ it, so nothing new leaks into
`forwardVariableRate` or into the advisory pass's view of the input.

**Cannot double-append**: `internal/finance/presentvalue/variablerate.go`
contains no call to `Calculate`. The test asserts the warning count is 1 and
kills a deliberate double-append mutant.

**The comment's other half was never true.** `solveUnknownPOD` returns the
result of a nested `Calculate`, which has already been through the pass. The
comment is corrected in place (R43).

⚠️ **P-W6 (over-specified row) is still NOT emitted under a rate schedule**,
because `FirstPass` is deliberately skipped in VR mode. See §80.

---

## §77 — ON THE PV SCREEN, A TYPED ZERO PAYMENT ON DEATH WAS TRANSMITTED AS "ABSENT", AND ABSENT MEANS "SOLVE FOR IT" (2026-08-09, round 42) — FIXED

`getActuarialConfig` gated the wire field on the VALUE, not on presence:

```js
if (pod != null && !isNaN(pod) && pod > 0) cfg.pod = pod;   // before
```

Omitting `pod` is not a neutral act. `internal/api/handlers.go:2087-2091`
reads an absent `pod` as `PODUnknown = true`, and
`presentvalue/calc.go` then dispatches to `solveUnknownPOD` — DOS's
`ComputeUnknownPOD`. So typing `0` into the Payment on Death cell —
"there is no death benefit" — was transmitted as a request to **solve for
one**. This is the AO9-7 shape (`> 0` on a value that can legitimately be
zero) with a semantic flip attached: here 0 and absent are different questions,
not a value and its default.

**FIXED**: `if (pod != null && !isNaN(pod)) cfg.pod = pod;` — only a genuinely
blank cell asks for the solve.

⚠️ **The audit found the widening incomplete and it is now closed:**
`updatePVActiveSummary`'s `podActive` was also `> 0`, so after this fix a
leftover **negative** POD would have reduced the total with nothing on screen
to say so — exactly the hole that line exists to close, in the other
direction. The predicate is now "does this cell move the total" (`!== 0`).

⚠️ **Still open, filed not fixed:** a worksheet with NO life contingency and a
POD of 0 never reaches `getActuarialConfig` at all, because the gate that
decides whether to send the actuarial block is still `hasPOD = podRaw > 0`
(`index.html` ~4960). On that worksheet §77 is unreachable. It is also
harmless — with no contingency and no POD there is nothing for the block to
carry — but the two predicates disagree and that is how the next defect gets
in.

**Guarded:** `cmd/persense/frontend_pv_pod_echo_test.go`
`TestR42PVActuarialConfigZeroPOD` runs the SHIPPED `getActuarialConfig` under
Node over blank / whitespace / `0` / `$0.00` / `$20,000.00` / `-5000` / a green
solved cell, and `internal/api/zzr42_pv_stacking_test.go`
`TestR42_ExplicitZeroPODIsNotASolveRequest` pins the server contract it relies
on.

---

## §78 — THE PORT'S POD SOLVE PRE-EMPTS EVERY OTHER BACKWARD SOLVE; DOS MAKES IT THE LAST RESORT, BEHIND A CONFIRMATION (2026-08-09, round 42) — 🚨 PERMANENTLY UNADJUDICABLE (Nate, decision 3a.15(B), 2026-08-09; recorded here round 44, item 0-DISC)

> 🚨 **STATUS CHANGED 2026-08-10, ROUND 44 — READ THIS BEFORE THE SECTION BELOW.**
> This section was FILED and MEASURED INERT. It is now **PERMANENTLY
> UNADJUDICABLE**, and that is a stronger statement than "open."
>
> Adjudicating it needs a DOS oracle for the PV screen's life-contingency and
> Payment-on-Death paths. **No such oracle can be built**: the `ACTUARY` unit
> source is absent from the materials we hold, `uses ACTUARY` is commented out
> in the source itself, and an `-dACTU` build fails with 11 errors in two
> families (**§82**). **Nate decided decision 3a.15 on 2026-08-09 — option (B):
> the gap is DISCLOSED PERMANENTLY and no DOSBox black-box sweep is funded.**
>
> **Therefore: whether this pre-emption's measured inertness (Δrate = 0 exactly,
> four ages) is DESIGN or LUCK cannot now be established by any instrument this
> project owns, and will not be.** It is not pending. It is not awaiting a
> round. **Do not carry it as an open work item, and do not let a future round
> re-scope it as one** — R47: an absent authority must be published as ABSENT,
> not as PENDING.
>
> ⚠️ **DO NOT OVERSTATE IT EITHER.** This is a claim about AUTHORITY, not about
> correctness. The actuarial mathematics is checked against an independent
> implementation (663 checks, ~12 significant figures) and against five values
> read from the original program under emulation (~1e-8). What is missing is
> specifically *"the port matches PER%SENSE's mathematics"*, which for a
> bug-for-bug port is the claim that matters.
>
> ⭐ **THE ONE THING THAT WOULD REOPEN IT, filed as a standing ask and not a work
> item: `ACTUARY.pas` from the client.** (B) closed the funding question, not the
> source question. ⚠️ Necessary but possibly not sufficient — seven of §82's
> eleven errors are the print/screen layer the ACTU table path drags in, which
> needs its own answer.

**DOS.** `podunk` is set FALSE at the top of `Enter` (`PRESVALU.pas:1156`) and
becomes true in exactly two places, both behind an explicit message box the
user can Escape out of:

- `:1177-1179`, inside the `if (frontward)` branch — i.e. **nothing on the
  screen is unknown** — *"All blanks are filled in. Proceed only to compute an
  unknown POD amount."*
- `:1207`, when there is neither a frontward nor a backward calc to do AND
  there are no payment rows at all — *"No payments to value. Proceed only to
  evaluate a POD amount (or press &lt;Esc&gt;)."*

`BackwardCalc` runs on its own path (`:1255`) with `podunk` false. **In DOS the
POD solve never pre-empts another backward solve.**

**THE PORT.** `calc.go` fires it on `(Actuarial.PODUnknown && SumValue given)`,
**before `FirstPass`**, so it pre-empts — and the client produces `PODUnknown`
whenever the POD cell is blank, which is the default state of every
life-contingent worksheet.

**MEASURED: currently INERT.** `solveUnknownPOD`'s first step re-enters
`Calculate` with POD=0 **and the same blank field and the same target still in
place**, so the nested backward solve drives its own SumValue onto the target
and the residual it divides by is solver noise. Measured over four ages
(dob 1934/1954/1974/1994), rate blank, target 150,000:

| | solved rate, POD blank | solved rate, POD=0 | Δ | solved POD |
|---|---|---|---|---|
| all four | — | — | **0 exactly** | ±6e-11 |

**FILED, NOT FIXED (R20: a fix that changes nothing has not been confirmed).**
`TestR42_PODSolveIsInertWhenAnotherFieldIsBlank` pins the inertness with a
non-vacuity check that a rate really is being solved, so the day the dispatch
order or a solver tolerance moves this off zero it is caught here rather than
silently changing a shipped answer.

⚠️ **Two second-order facts worth recording.**
1. `solveUnknownPOD` computes its unit probe at `input.PresVal.R.Rate` — the
   **pre-solve** rate — not the rate the nested `Calculate` just solved. On the
   pre-emptive path that is the blank rate. It does not currently matter
   because the numerator is noise; it would matter the moment the numerator
   is not.
2. Because the pre-emptive path returns a POD of ~1.5e-11 rather than an exact
   zero, **§75's paint had to be floored at half a cent.** Without the floor,
   every life-contingent backward solve would have stamped a green `$0.00`
   into the Payment on Death cell — and a painted cell is a hard input on the
   next submit.

---

## §79 — THE PV RESPONSE WAS PAINTED BY REQUEST INDEX INTO A GRID THAT IS NOT COMPACTED (2026-08-09, round 42) — FIXED

`getPVInput` `continue`s past an empty grid slot, so `body.lumpSums` and
`body.periodics` are **compacted**; the DOM rows are not. The response paint
loops used the request index as the selector:

```js
document.querySelector(`input[data-ls="${i}"][data-f="date"]`)   // before
```

With grid row 1 blank and row 2 filled, request index 0 IS grid row 2, and row
2's solved Date / Amount / Value were written into the **blank row above it**.
Two consequences, the second worse than the first: the answer appears in the
wrong row, and — because a painted cell reads as a filled cell on the next
submit — the blank row becomes a **phantom payment that is then summed into
the total**.

This is the NF-4 family. `deletePVRow` renumbers `data-ls`/`data-per` precisely
so a hole cannot open (its own comment: *"a hole would silently desynchronize
the API response from the rows on screen"*), but BLANKING a row's cells — the
reuse-without-clearing path this project has already been bitten by — opens
exactly that hole and nothing renumbers.

**FIXED**: `pvLumpBlanks` / `pvPerBlanks` now record the grid row each request
row was read from (`dom: i`), and both paint loops select on it.

**Guarded** by `TestR42PVPaintPairing`, scoped to the paint loops — the READ
loops legitimately use `i`, because there `i` IS the grid row. The first cut of
that guard searched the whole file and fired on the read loops; a guard that
cannot tell the two loops apart would have to be silenced, and a silenced gate
is how operators learn to ignore gates (round 41).

---

## §80 — NEITHER PV ARM REPRODUCES DOS's SCREEN-TOTAL OVER-DETERMINATION WARNING (2026-08-09, round 42) — FILED

DOS warns when the screen's own Present Value line is already determined by the
data above it, on BOTH the plain and the fancy (variable-rate) path:

```pascal
{ PRESVALU.pas:1160-1167 }
if (frontward) then
   if (pvlfancy) then begin
      if (d^.xasofstatus=inp) and (d^.xvaluestatus=inp) then
        MessageBoxWithCancel('Warning: value entered in line '+strb(i,0)+
                             ' below is already determined by data above.', ...)
   end
   else for i:=1 to nlines[PVLPresValBlock] do
     if (c[i]^.status>=fully_specified) then MessageBoxWithCancel(... same ...)
```

**MEASURED — the port is silent on both arms.** Complete rows, a typed screen
total of 99,999 that disagrees with the rows:

| arm | sumValue returned | warning |
|---|---|---|
| fixed 6% | 34,883.8163 | none |
| VR 6/8 | 33,516.0023 | none |

The typed total is discarded without a word. The port's P-W6 is a ROW-level
over-specification advisory (amount and value both given); DOS's is about the
PresVal LINE. They are different predicates and the port has only one of them.

**FILED, not fixed:** it touches the fixed-rate arm, which is a published
surface, so it is an R36 change to be made deliberately with a before/after,
not a drive-by.

⚠️ Related and also filed: `PVRateLineReq.TrueRate` has no required-value
check (`handlers.go` ~1838 requires the Date and not the rate), so a schedule
row posted with the rate omitted is silently a **0% stratum** — measured,
33,516.00 → 42,607.19 on the same worksheet. **Not reachable from the shipped
UI**: `readPVRateSchedule` (`index.html:5046`) skips a row whose rate is blank
or unparseable. It is an API-layer hole for any other client.

---

## §81 — THE PV DISPLAY LAYER'S REMAINING ZERO-SUPPRESSION AND STALE-OUTPUT GAPS (2026-08-09, round 42) — FILED

Found by the round-42 PV audit, verified in source, deliberately NOT fixed this
round. Each is the amortization screen's own defect family, on the PV screen.

1. **A PV error leaves the previous result fully painted.** `calcPV`'s error
   path sets the message and `return false`s without clearing the total, the
   green per-row Value cells, `#pv-pod-result` or the table. The amortization
   screen has `clearAmzScheduleOutput()` for exactly this — its comment reads
   *"wipes a displayed schedule so a stale result can't be mistaken for the
   answer to a failed calculation"* — and there is no PV counterpart. **NF-3/
   NF-5 family, user-visible.** Not fixed because the desired behaviour is
   Nate's call and DOS's own behaviour on a refused PV calc has not been read.
2. **`podValue > 0`** suppresses a legitimately zero POD present value. Left
   alone on purpose: unlike §77, absent and zero mean the same thing on a
   display line, and flipping it changes a shipped display on every
   non-actuarial worksheet.
3. **`prob > 0`** drops the `(p=…)` annotation when a survival probability is
   exactly 0, so a `$0.00` row value is shown with no explanation.
4. **`omitempty` on four computed outputs** — `podValue`, `pod`, `rate`,
   `prob`. An exactly-zero solved IRR is dropped from the JSON and is then
   indistinguishable from "not computed"; the solved-rate cell would never
   fill. Reachability not established.
5. **`#pv-active-summary` cannot name most of what is in force.** It reports
   row counts, COLA, life contingency, POD and the rate schedule. It cannot
   report the mortality TABLE selection or a pasted custom CSV, the Reference
   Date "Now" override, the rate-type selection, WHICH rows are contingent, the
   fact that the screen is in a backward-solve mode, or the table options —
   and there is **no count badge**, where the amortization screen has
   `amz-adv-badge` + `amzAdvActiveCount()`. `expandPVSectionsWithEntries()`
   runs only on load and on entering the screen, so a section collapsed
   mid-session with a leftover POD or rate schedule inside stays collapsed and
   unlabelled. **This is the stacking-visibility gap Nate asked about, stated
   exactly.**
6. **`clearPV()` misses the PV table options** (`#pv-table-view`,
   `#pv-table-period`, `#pv-table-month`), so the next table renders under
   leftover options; and it leaves `#pv-asOfDate` empty where the shipped
   default is a date, so "Clear All" lands the screen in a different dispatch
   from a fresh load.
7. **`HandlePVTable` has ZERO tests anywhere in the repo**, and the
   unwired-route canary's mirror of the production mux omits
   `/api/presentvalue/table`. Every field of the table response is untested,
   including the `lifeMode` gate that suppresses `ifPaid`/`probability`/
   `contingency` wholesale.
8. **`PeriodicPayment.NInstallments` is computed and ABSENT from the wire**;
   the snapped To-date DOS echoes is carried in `toRawM`/`toRawD`, which are
   **unexported and therefore structurally untransportable** — no consumer can
   ever learn it. NF-2's shape on the PV screen.
9. **`periodics[].installments[]` is transported and read by nobody.** A dead
   wire field is an untested field.

⚠️ **And the coverage statement, which is the point of the whole section:
before round 42 there was NO test anywhere in the repo that posted a PV request
with a variable-rate schedule AND an actuarial config AND a per-row COLA AND a
backward-solve target together.** The four surfaces this project has always
reported as "PV: 0 divergences" were each measured with the advanced options
UNSTACKED. **CAUTION 8 applies to the PV zero with full force.**

---

## §82 — THE `-dACTU` SPIKE: THE PV ACTUARIAL ORACLE CANNOT BE BUILT, AND ROUND 42's STATED REASON WAS WRONG (2026-08-09, round 42 addendum)

**Decision 3a.15 asked: can we build a `pv_oracle` with `-dACTU`? MEASURED
ANSWER: NO, and not for the reason round 42 gave.**

### The spike

Probe tree, one line changed (`build_linux.sh:114`, `-dACTU` appended),
`TARGET=pv_oracle ./build_linux.sh`. **Exit 1. Eleven errors, all in
`pvltable.pas`, in two distinct families:**

| family | identifiers | what it is |
|---|---|---|
| **the missing `ACTUARY` unit** | `LifeProb`, `PODValue`, `XPODValue`, `Ondeath` | the actuarial core — **18 call sites across `presvalu.pas`, `pvltable.pas` and `pvlxscrn.pas`** |
| **the print/screen layer** | `CheckRoomOnPage`, `NewPage`, `textattr`, `GetColor`, `ob`, `DetailLineToLotus`, `OutputLine` | the ACTU table path pulls in the reporting layer the oracle driver deliberately does not link |

`uses ACTUARY` is **commented out in the source itself** — `presvalu.pas:12` and
`pvltable.pas:6`, in both the read-only `legacy/src/dos_source` originals and the
oracle's staged units: `//{$ifdef ACTU} ,ACTUARY {$endif}`. Whoever prepared this
snapshot commented it out because the unit was already absent.

### 🚨 THE CORRECTION (R43, and rule 11 applied to round 42's own finding)

**Round 42 stated the PV actuarial gap as a BUILD-FLAG CHOICE** — *"`pv_oracle` is
compiled without `-dACTU`; the build script says so on purpose"* — which implies
the flag could simply be added. **It cannot. The flag is a SYMPTOM, not a
decision: the `ACTUARY` unit source is MISSING from the snapshot.**

**And the project already knew.** `docs/actuarial_oracle_blocked.md` records
exactly this, with the same four undefined identifiers and the same build
evidence, and calls it *"a missing-source problem, not a porting defect."*
**Round 42 re-discovered a documented blocker and presented it as new. Standing
rule 11 says do not carry a claim forward without verifying it; the converse
obligation — CHECK WHETHER THE TREE ALREADY ANSWERS THE QUESTION BEFORE CALLING
AN ANSWER NEW — is the same discipline and round 42 did not meet it.**

### 🚨 WHAT SURVIVES THE CORRECTION, AND IT IS MOST OF IT

**R47 stands. Only its stated cause was wrong.**

1. **The PV screen's life-contingency and Payment-on-Death surface has no DOS
   differential.** Confirmed at HEAD by the spike, not by a carried claim.
2. **`dos_pv_fuzzer5_test.go` generates nothing actuarial** — no *actuarial*,
   *pod*, *contingency* or *Act* token. **This half is genuinely new.** The
   blocked-doc records the missing ORACLE; nobody had connected it to the
   GENERATOR, and the two together are what makes the gap invisible: there is
   neither an authority to compare against nor a case that would need one.
3. **§75, §77 and §78 are real, were found in that hole, and no differential
   could have found them** — three of round 42's four defects.
4. **There are TWO blockers, not one.** Even if the `ACTUARY` unit appeared, the
   `{$ifdef ACTU}` path in `pvltable.pas` also drags in the print/screen layer.
   The blocked-doc lists this only as "plus screen/print routines"; the spike
   shows it is **seven of the eleven errors** and would need its own answer.
5. **The errors stop at `pvltable.pas`.** `PRESVALU.pas`'s own `{$ifdef ACTU}`
   blocks — including the `podunk` gate at `:1156`, `:1177-1179`, `:1207` that
   §78 diverges from — were never reached, because the table unit fails first.
   **Do not read that as "PRESVALU compiles under ACTU": it calls `PodValue` /
   `XPODValue` at `:689`, `:712`, `:790`, `:842-849` and would fail too.**

### The remedy is not a compile flag

`docs/actuarial_dosbox_oracle_plan.md` is the live path: the original
`PerSense.exe` and the recovered `MALE.ACT` / `FEMALE.ACT` tables exist, and a
**black-box DOSBox differential** has already produced **one cent-accurate golden
case** — DOS Sum Value 104,258.31 vs the Go engine's 104,258.3065 (relErr 3.4e-8),
guarded by `TestDOSActuarialGolden` (`docs/dos_actuarial_golden.md`).
**⚠️ Its own obstacle list is honest: no emulator in the sandbox, an interactive
TUI with no CLI entry point, screen-scraping or a print-to-file path for capture,
and "a few hundred cases is realistic; tens of thousands is not."**

**So the standing position is: the actuarial PV surface will not get a
link-against-units oracle from these materials. Either the client supplies the
`ACTUARY` unit source, or the surface is covered by a bounded black-box sweep and
the gap is written into every PV claim, permanently.** → **decision 3a.15,
REWRITTEN.**

### ⚠️ A PROBE-TREE HAZARD FOUND BY THE SPIKE

`build_linux.sh` writes to the **shared** `/tmp/oraclebuild`, so a build launched
from a PROBE TREE targets the same directory as the real one. The spike's build
failed, so it only clobbered `build.log` — **a build that SUCCEEDED would have
overwritten the working oracle binary with the probe's, silently, and every
subsequent measurement in the session would have been made against it.**
**Set `OUT` (or copy the binaries aside) before building an oracle from a probe
tree.** This is R44's cousin: a probe tree is only isolated if its outputs are.

---

## §83 — SIGNAL 6's NEGATIVE CONTROL WAS VACUOUS, NOT INERT: THE POPULATION MEASURED, THE CONTROL RUN, AND THE GATE EARNED (2026-08-09, round 43)

**Item 0m(i), carried at the top of the work plan for THREE rounds and
re-specified identically each time. The experiment was never the problem.**

### The claim that was wrong

Round 39e reverted the modal-payment reconstruction, Signal 6 (the APR probe)
produced the identical 4 divergences at seed 50100, and 39e recorded — honestly —
that *"the fuzzer-level negative control for the APR probe was INERT."* The
project read that as a statement about the probe's SENSITIVITY, and START_HERE
told rounds 41, 42 and 43: *"build a probe tree that re-introduces the
modal-payment APR defect and assert Signal 6 goes RED. ~20 lines."*

**`ROUND40_AUDIT_of_the_round39_record` §3.1 had already written the correct
requirement**, and it never reached START_HERE:

> *"Signal 6 is a FINDER, not a GATE, until the control is re-run on a seed whose
> population contains a pay-solve ∩ points screen on the `AmortizeDOS` arm where
> modal ≠ payment."*

### The measurement — the funnel, now printed every run

A counter added to `dos_fuzzer5_test.go` at the Signal 6 site, computed on the
SAMPLE and before the APR comparison, so it describes the population rather than
the outcome. Twenty seeds, `PERSENSE_FUZZ_N=400`, 8,000 generated screens:

| seed | pts>0 | pay-solve | **dosport** | **modal ≠ RegularPayment** |
|---|---|---|---|---|
| **50100** (39e's) | 205 | 48 | **1** | **0 — VACUOUS** |
| 50105 · 50110 · 50118 | ~185 | ~40 | **0** | 0 |
| **50107** | 184 | 45 | 5 | **1** |
| **50109** | 186 | 43 | 4 | **1** |
| **20 seeds pooled** | **3,752** | **791** | **37** | **2** |

**The bottleneck is the ENGINE clause: only 37 of 791 pay-solve ∩ points screens
(4.7%) route to `dosport` at all**, and almost none of those has a modal that
differs from the transported regular payment. **The control's population is
roughly 1 in 4,000 generated screens, and seed 50100 contains none of it.**

**No mutant could have moved 39e's result.** The experiment was VACUOUS, not
negative — and re-running it, as the plan instructed three times, could never
have produced anything else.

### The two discriminating cases, and why they discriminate

```
seed 50107  modal=462.10   RegularPayment=2196.2951  delta=-1734.1951
  amort_oracle 111299.94 0.0725630000 72 4 b365 prepaid plusreg usa
    loandmy=28.9.2023 firstdmy=28.10.2023 mor=85 b109=19452.82 b124=18502.29
    pre=325:207:6:462.10 targ=547.99 pts=0.012291

seed 50109  modal=1903.16  RegularPayment=8296.4029  delta=-6393.2429
  amort_oracle 370288.11 0.0656300000 92 4 exact plusreg usa
    loandmy=5.10.2025 firstdmy=5.1.2026 mor=63
    pre=495:160:4:1903.16 targ=552.23 pts=0.014356
```

**Both carry a `pre=` prepayment whose rows OUT-COUNT the regular rows, so the
modal statistic returns the PREPAYMENT amount.** That is the shape the round-39
defect fed to the APR pass, and the deltas are $1,734 and $6,393 — not a rounding
tail.

### The controlled experiment

Probe tree, one mutation: the modal reconstruction re-introduced into
`applyAPR`'s payment argument at `engine.go:756`.

| seed | discriminating population | pristine | **mutant** | Δ |
|---|---|---|---|---|
| **50100** | **0** | 4 differ | 4 differ | **0 — 39e's inertness reproduced EXACTLY** |
| **50107** | 1 | 2 differ | **3 differ** | **+1 — RED** |
| **50109** | 1 | 1 differ | **3 differ** | **+2 — RED** |

**Signal 6 goes red exactly where the population can express the defect, and
nowhere else.** Seed 50100's pristine run also reproduces 39e's published figures
to the case (204 compared, 4 differ), so the instrument is the same instrument.

⚠️ **Seed 50109's mutant adds TWO divergences against a discriminating count of
one.** The funnel is restricted to the `dosport` arm per ROUND40 §3.1, while the
mutation applies wherever `applyAPR` is called — so **the funnel is a LOWER BOUND
on the true discriminating population.** Do not read it as exact.

### What this licenses, and what it does not

- ✅ **Signal 6 is a GATE for the modal-payment regression.** The R20 requirement
  is met: the instrument has been seen to fail when the defect is present and to
  pass when it is not. **The standing ban on quoting its rate is lifted for that
  claim.**
- 🚨 **It is NOT an attribution of the residual class.** The r41 figure — 20 APR
  divergences located in 1,856 comparisons — remains **UNATTRIBUTED (R27)**, and
  the mutant reproduces none of those 20. A gate that detects one known
  regression says nothing about the mechanism behind twenty unknown ones.
- ⚠️ **The population is a property of the GENERATOR (R31).** 2 in 8,000 is thin,
  and any generator change can move it. The funnel prints every run precisely so
  the next round can check rather than assume.

### → R49 — A CONTROL IS ONLY A CONTROL IF ITS POPULATION CAN EXPRESS THE DEFECT

R20 says a fix that changes nothing has not been confirmed. R38 says a guard
written this round is as suspect as one written ten rounds ago. **R49 is the one
underneath both: before concluding an instrument is insensitive, COUNT THE CASES
IN THE SAMPLE THAT COULD HAVE SHOWN THE DIFFERENCE. If that count is zero the
experiment was vacuous, not negative, and re-running the mutant will not fix it.
Assert the count inside the test, as the positive control.**

Measured cost of not having this rule: **three rounds of a top-three work item,
re-specified identically each time, against a documented correction the state
file never carried.**

**Guarded by** `internal/finance/amortization/zzr43_signal6_control_test.go`
(8 mutants written, 8 killed; two earlier forms of the seed-record guard were
found VACUOUS by mutation testing — see below and R50).

### ⚠️ A NEW VACUITY SHAPE FOUND WHILE GUARDING THIS — R50

The first form of the seed-record guard read **its own source file** and asserted
that the seeds appeared in it. Mutation testing killed it instantly: renaming
`50107` throughout the file renames it in **both the needle and the haystack**,
so the assertion is true for every possible value. **A self-reading guard whose
expected value lives in the file it reads is unconditionally true.** It is
round 42's *"a guard can match its own declaration"* one level up, and the fix is
the same: **assert ACROSS files.** The guard now reads this section.

---

## §84 — SIGNAL 7 IS A GATE ON BOTH ARMS, AND AN INERT MUTANT ON A HEALTHY POPULATION PRODUCED A NEW RULE (2026-08-10, round 44)

**Item 0m(i)-B.** Signal 7 (the adjustment-cell echo probe) was added in the
same 39e commit as Signal 6 and, like Signal 6, shipped with no negative
control. Round 43 closed 0m(i) for Signal 6 and wrote **R49**. This is the same
work for Signal 7. It came out differently, and the difference is the finding.

### 1. THE DISCRIMINATING POPULATION, MEASURED FIRST (R49 — do not invert this)

The control 0-NF wants is *"re-drop `adj.Amount`/`AmountStatus` on the piecewise
path, assert Signal 7 goes RED."* The population that can express that is **not**
"piecewise adjustment rows." NF-1 means some piecewise rows are **already red**
on exactly that arm, and a row already booking `adj_amount_missing` cannot be
turned red by a mutant that stops Go marking it solved. The discriminating
population is the rows that are **currently GREEN**:

> piecewise row ∧ the date matched (else the switch is never reached)
> ∧ DOS displays a computed value (status 1) ∧ Go currently carries it (solved).

Instrumented as a funnel in `dos_fuzzer5_test.go`, computed from sample
properties only and placed **before** the comparison switches. Printed every run.

**Seeds 50100 / 50107 / 50109, `PERSENSE_FUZZ_N=400`, 1,200 generated screens,
HEAD `2b03814`, oracle built `-dV_3 -dSCROLLS -dPVLX` (R47 — `-dACTU` is
unbuildable and irrelevant here; this is the amortization oracle):**

| seed | piecewise rows | DOS displays amt | Go carries (discriminating) | already red |
|---|---|---|---|---|
| 50100 | 91 | 30 | **30** | 0 |
| 50107 | 79 | 24 | **23** | 1 |
| 50109 | 86 | 28 | **28** | 0 |

| seed | DOS displays rate | Go carries (discriminating) | already red |
|---|---|---|---|
| 50100 | 24 | **24** | 0 |
| 50107 | 25 | **25** | 0 |
| 50109 | 21 | **21** | 0 |

**Unlike Signal 6's, this population is healthy.** Signal 6's was 0/1/1 in 8,000
screens; Signal 7's is 30/23/28 in 1,200.

### 2. THE CONTROLLED EXPERIMENTS

**MUTANT A2** — `amtHas` forced `false` at the piecewise echo call
(`engine.go:1931`), schedule untouched:

| seed | `adj_amount_missing` pristine → mutant | delta | funnel predicted |
|---|---|---|---|
| 50100 | **0 -> 30** | 30 | 30 |
| 50107 | **1 -> 24** | 23 | 23 |
| 50109 | **0 -> 28** | 28 | 28 |

**MUTANT B** — `rateHas` forced `false` at the same call, **echo only**:

| seed | `adj_rate_missing` pristine → mutant | funnel predicted | `apr_differs` pristine → mutant |
|---|---|---|---|
| 50100 | **0 -> 24** | 24 | 4 → **4** |
| 50107 | 0 -> 25 | 25 | 2 → **2** |
| 50109 | 0 -> 21 | 21 | 1 → **1** |

**THE DELTA EQUALS THE FUNNEL'S THIRD NUMBER ON EVERY SEED, ON BOTH ARMS.** That
is a stronger positive control than Signal 6 got: the population count does not
merely license the control, it **predicts the mutant's finding count exactly**.

**✅ SIGNAL 7 IS A GATE FOR THE ADJUSTMENT-CELL ECHO, ON BOTH THE AMOUNT AND THE
RATE ARM.** The exit criterion's instrument-control clause (R49) is met for it.
🚨 **Its 35-in-858 residual is a separate claim and is NOT thereby attributed —
see §85, which shows most of it is not NF-1 at all.**

### 3. 🚨 MUTANT A WAS INERT ON THAT HEALTHY POPULATION → **R51**

The **first** mutant re-introduced the historical defect named in
`engine.go:1931`'s own comment: key the has-a-value test on `a.AmtOK` alone
rather than `a.AmtOK || a.AmountStatus == types.InOutOutput`. The comment's
stated reason is that *"`AmtOK` alone is too narrow ... keying on amtok loses
exactly the row DOS paints."*

**MUTANT A produced byte-identical findings on all three seeds — INERT — while
this funnel read 30, 23 and 28.**

Round 43's R49 would read that as *"the population is fine, so the instrument is
insensitive."* **It is neither.** Mutant A2 turned the same population red
30/24/28. Mutant A mutated a **disjunct that is dead on this generator's
population**: `a.AmtOK` is already true on every one of those rows.

> **R51 — A MUTANT THAT IS INERT ON A HEALTHY POPULATION IS A STATEMENT ABOUT
> THE MUTANT, NOT THE INSTRUMENT.**
> R49 asks *can the population express the defect?* R51 asks *does the mutant
> reach that population?* A funnel keyed to the SIGNAL's predicate is silent
> about the second question — this one was right about Signal 7 and said nothing
> about mutant A. **Escalate the mutant before doubting the instrument.**
> It is the exact inverse of the failure round 43 spent three rounds on.

### 4. ⚠️ FILED, NOT FIXED — A DISJUNCT THAT IS DEAD ON THE MEASURED SAMPLE

`engine.go:1931`'s `|| a.AmountStatus == types.InOutOutput` produced **no
observable effect on any of 256 measured piecewise adjustment rows**, though its
comment says it is load-bearing. Either the generator cannot produce the
rate-only-adjustment shape the comment describes (`amtok` FALSE ∧ `amtstatus`
outp), or the claim is wrong.

> **⚠️ SCOPED BY THE ROUND'S OWN AUDIT — DO NOT READ THIS AS "THE DISJUNCT IS
> DEAD ON 256 ROWS."** Mutant A is observable ONLY through Signal 7's amount
> switch, which has arms for `amtStatus` 0 and 1 and **none for 2 or 3**; rows
> whose date differs `continue` before the switch entirely. On seed 50100 only
> **30 of 91** piecewise rows have `amtStatus == 1`. The evidence therefore
> licenses *"not load-bearing on any row OBSERVABLE TO SIGNAL 7's AMOUNT
> SWITCH"* and says nothing about the residue. **The stronger form is the same
> overreach R51 was written to punish, one paragraph after writing R51.**
> **TO SETTLE IT:** instrument `engine.go:1931` with a counter of rows where
> `!a.AmtOK && a.AmountStatus == types.InOutOutput` — the disjunct's actual
> load-bearing condition — and print an `amtStatus` histogram beside it.

**R31: that is a statement about the GENERATOR until someone widens it. DO NOT
DELETE THE DISJUNCT** — removing a guard that is dead on the sample you happened
to measure is how a latent defect ships, and this project has retired that exact
reasoning before (CAUTION 8, the `dosport` zero that died at r38).

**Guarded by** `internal/finance/amortization/zzr44_signal7_control_test.go`.

---

## §85 — 🚨 ATTRIBUTION WITHDRAWN IN-ROUND: THE APR/ADJUSTMENT-RATE CO-OCCURRENCE IS CONFOUNDED BY WHOLE-SCREEN TOTALS DIVERGENCE (2026-08-10, round 44)

> **🚨 READ THIS FIRST. THIS SECTION'S ORIGINAL CLAIM WAS WRONG AND IS WITHDRAWN
> BY THE ROUND THAT MADE IT, BEFORE PUBLICATION.** Round 44 wrote §85 as *"the
> residual APR class is attributed to the piecewise adjustment-rate solve."*
> The round's own adversarial audit (R32) attacked it, the attack was MEASURED,
> and **the attribution does not survive.** What follows is the corrected
> record. This is the r32 same-day-retraction pattern working as intended —
> **R32 is now ELEVEN FOR ELEVEN.**

**Item 5.** The residual APR class — **20 divergences in 1,856 comparisons**
(r41, pooled, `horizon` key, no engine filter, tol 2e-6) — remains **OPEN and
UNATTRIBUTED (R27)**, exactly as round 43 left it. This section does not change
that. It records a measurement, a false attribution, and why the attribution
failed, because the failure is more useful than the measurement.

### 1. WHAT WAS MEASURED (this part stands)

Seeds 50100/50107/50109/50101, `PERSENSE_FUZZ_N=400`, screens keyed by their
full oracle command line:

| seed | `apr_differs` | `adj_rate_differs` | ∩ | **APR without adj_rate** |
|---|---|---|---|---|
| 50100 | 4 | 4 | 4 | **0** |
| 50107 | 2 | 3 | 2 | **0** |
| 50109 | 1 | 1 | 1 | **0** |
| 50101 | 0 | 0 | 0 | **0** |
| **total** | **7** | **8** | **7** | **0** |

And **mutant B** (§84 — drop the rate from the ECHO only, schedule untouched)
moves the echo finding wholesale while leaving `apr_differs` at **4 / 2 / 1,
unchanged**. **That much is solid: the echo is DOWNSTREAM and is not the cause.**

### 2. 🚨 WHY THE ATTRIBUTION FAILS — THE THIRD SET NOBODY LOOKED AT

The audit asked whether a third signal covers the same screens. It does:

| seed | `apr_differs` screens | also `divergent_class` (totals) | **APR on a TOTALS-GREEN screen** |
|---|---|---|---|
| 50100 | 4 | 4 | **0** |
| 50107 | 2 | 2 | **0** |
| 50109 | 1 | 1 | **0** |
| 50101 | 0 | 0 | **0** |
| **total** | **7** | **7** | **🚨 0** |

**EVERY ONE OF THE SEVEN APR-DIVERGENT SCREENS IS ALSO DIVERGENT ON ITS
SCHEDULE TOTALS** — worst `dInt` on seed 50100 alone: **$200.08, $56,839.77,
$16,901.37, $9,330.19**.

> **AN APR COMPUTED OVER A SCHEDULE WHOSE INTEREST TOTAL IS WRONG BY $56,839 IS
> EXPLAINED BY THE SCHEDULE.** Nothing in this evidence isolates the adjustment
> RATE as the mechanism rather than as a fellow symptom. The co-occurrence with
> `adj_rate_differs` is real and 7-for-7 — and so is the co-occurrence with
> `divergent_class`, and the second one is sufficient. **R27: an attribution to
> a signal class is not an attribution to a site, and a co-occurrence with two
> classes at once is not an attribution to either.**

### 3. 🚨 AND THE BASE RATE WAS COMPUTED OVER THE WRONG DENOMINATOR

The withdrawn text quoted *"8 of ~737 compared screens = 1.1%, so p ≈ 2e-14."*
**737 is the APR-COMPARED denominator.** `adj_rate_differs` is *structurally
impossible* outside the stratum "piecewise adjustment row carrying a
DOS-solved rate" — **24 / 25 / 21 / 23 such rows per seed**, not 737 screens.
Conditioned on the stratum where both events can occur at all, the enrichment
is roughly **8×, not 10¹⁰×**, and the honest p is in the 1e-4…1e-7 region.
**Rule 9's base rate must be taken over the stratum where BOTH events are
possible.** The 2e-14 is retired; do not quote it.

### 4. WHAT REPLACES THE CLAIM

**The seven APR divergences in this sample are on screens that are wrong in
several ways at once, and cannot be used to attribute anything.**

🚨 **AND A SHARPER QUESTION THE ROUND CANNOT ANSWER: are these seven even
MEMBERS of the r41 residual class?** If the r41 "20 in 1,856" was understood as
an APR-specific class *distinct from* the known totals classes, then a sample in
which every APR divergence sits on a totals-divergent screen is a sample of
something else. **Nobody has checked how many of the r41 twenty were
totals-green. Until someone does, "20 in 1,856" should not be described as an
independent class.**

**THE DECISIVE NEXT EXPERIMENT, and item 5's whole remaining job: EXHIBIT AN
APR DIVERGENCE ON A TOTALS-GREEN SCREEN.** Until one exists there is nothing to
attribute. If none exists across a wide sample, the residual APR class dissolves
into the totals classes and the exit criterion's unattributed-signal count
changes — which is a **bigger** result than the withdrawn one.

### 5. THE RATE DELTAS, RECORDED FOR WHOEVER RUNS THAT EXPERIMENT

DOS | Go on seed 50100's four screens: `-1.1581913618 | -1.2779210335`;
`-0.0873932096 | -0.1081156521`; `0.2366099727 | 0.2267640748`;
`0.1235120932 | 0.1190252367`. **1–10% relative, Go below DOS in all four**
(a one-sided run of four, p = 0.0625 — weak on its own, say so). Magnitudes
rule out a tolerance artefact; the shape is the §66 family, a solver in a
different basin. **This is a lead, NOT a finding.**

---

## §86 — THE NF-1 / SIGNAL 7 RECONCILIATION: SIGNAL 7 IS NOT BLIND, THE FIGURES ARE OVER DIFFERENT POPULATIONS, AND "PARTLY NF-1, UNSPLIT" IS NOW SPLIT (2026-08-10, round 44)

**Item 0-NF, first step.** START_HERE required this before any NF-1 fix: *"Take
ROUND39D's minimal repro and check whether Signal 7 books it as a finding AT
ALL."* Done. Three separate answers, and the third changes the plan.

### 1. ✅ SIGNAL 7 IS NOT BLIND TO NF-1 — THE REPRO STILL FIRES

ROUND39D's minimal repro, run against both engines at HEAD `2b03814`
(oracle `-dV_3 -dSCROLLS -dPVLX`; Go via a probe instrument added to
`cmd/goamort` that dumps the R39 echo — **R44: a probe instrument, not a fix**):

```
amort_oracle 105319.00 0.0648162 120 12 payhard=947.00 b365 prepaid \
             adj=17:0.0422379: adj=67::1303.00 adjdump

DOS  adjrow 1  6/1/2025  rate .0422379 ratestatus 3  amount 1142.997616 amtstatus 1  amtok FALSE
GO   goadjrow 1 6/1/2025 rate .0422379 ratesolved false amount   0.000000 amountsolved false
```

`dr.amtStatus == 1 && !ga.AmountSolved` is **exactly** Signal 7's
`adj_amount_missing` arm. **Signal 7 would book this screen.** The totals are
byte-exact on both sides (`payment 947.0000 interest 36988.85 paid 142307.85`),
reproducing 39D's "totals EXACT" — this is a pure display-transport defect,
still open, still user-visible.

> **THE HYPOTHESIS THAT SIGNAL 7 CANNOT SEE NF-1 IS REFUTED.** START_HERE
> offered it as the first branch of item 0-NF; it is closed, measured.

### 2. THE GAP IS POPULATION, NOT INSTRUMENT — AND IT IS ~27×, NOT 8×

Measured on the fz5 generator (§84's funnel, seeds 50100/50107/50109,
`PERSENSE_FUZZ_N=400`): `adj_amount_missing` fires on **1 of 82 DOS-displays
piecewise rows = 1.2%**, and on **1 of 256 piecewise adjustment rows = 0.4%**.

NF-1 is published at **"30-38% of adjustment rows"** (ROUND39D, and the
restatement §1g already records it as **UNSCOPED — no N, no denominator,
"schedule-clean" undefined, seeds unnamed**). Against the arm NF-1 is *defined
by*, the discrepancy is **~27×, not the 8× START_HERE carried** — the 8× was
computed against Signal 7's ALL-TOKEN finding count, which §3 below shows is
mostly a different defect.

**The fz5 figure does not refute 39D's; they are over different generators.** (Round 44 re-derived only the fz5 side. 39D's 30-38% is NOT re-measured here and remains UNSCOPED — do not read this section as ratifying it.) 39D swept a
hand-built adjustment population; fz5 samples a wide advanced-option space in
which the NF-1 shape is rare. **R31: a rate is a statement about its generator.**
**39D's figure stays UNSCOPED and must not be quoted as a rate on the fz5
population — and 1.2% must not be quoted as a refutation of it.**

### 3. 🚨 "PARTLY NF-1, UNSPLIT" IS NOW SPLIT — AND IT IS MOSTLY NOT NF-1

Signal 7's findings by token, seeds 50100/50107/50109 at `PERSENSE_FUZZ_N=400`
(1,200 generated screens, 256 piecewise adjustment rows, 12 findings):

| token | count | share |
|---|---|---|
| **`adj_rate_differs`** | **10** | **83%** |
| `adj_amount_missing` (**NF-1's arm**) | **1** | **8%** |
| `adj_amount_differs` | 1 | 8% |
| `adj_echo_count` · `adj_echo_date` · `adj_amount_invented` · `adj_rate_missing` · `adj_rate_invented` | **0** | — |

**Signal 7's headline number is dominated by a SOLVED-ADJUSTMENT-RATE class, and
§85 shows that class is the residual APR class.** The restatement's
*"partly NF-1, UNSPLIT"* is hereby split: **~8% NF-1, ~83% the §85 rate class.**

### 4. 🚨 THEREFORE 0-NF's PRE-DECLARED HEADLINE MOVE IS WRONG — CORRECTED BEFORE ANY FIX

START_HERE pre-declares, for item 0-NF: *"If NF-1's fix removes [Signal 7's]
findings, the in-scope Q7 rate moves from 1 in 70 toward ~1 in 87 for WHOLLY
DISPLAY-LAYER REASONS."* Both halves are now wrong, and in opposite directions:

1. **NF-1's fix removes ~8% of Signal 7's findings, not all of them.** The
   predicted rate move is far smaller than ~1 in 87.
2. **The remaining ~83% are NOT display-layer.** They are the §85 rate class —
   an ENGINE defect. Removing *those* would be convergence, not cosmetics, and
   must not be discounted as a display-transport correction.

**Pre-declared here, before the fix, per R36/R41.** The Q4 column remains the
control and must not move.

**⚠️ SCOPE (R31/CAUTION 9):** three seeds, 1,200 generated screens, `horizon`
key, `PERSENSE_FUZZ_N=400`, no engine filter beyond the piecewise stratification
the funnel applies. **This is NOT a re-derivation of the r41 pooled "35 in 858"
and must not be quoted as one.**

---

## §87 — ITEM 0-13b: THE MORTGAGE ORACLE HAS A BASIS TOKEN NOW, AND THE ANSWER RETIRES THE CONCERN (2026-08-10, round 44)

**Nate's decision 3a.13, 2026-08-09: MEASURE FIRST, DO NOT CHANGE THE
ARITHMETIC.** This is the measurement. **It is a negative result, and it
retires the premise of items 0-13b and 13.**

### 1. THE TOKEN (rule 7)

`legacy/oracle/mtg_oracle.pas` pinned `df.c.basis := x360` in `AllocMtg` with no
way to ask it anything else. Round 44 added a **NEW TOKEN** — `b360`, `b365`,
`b365_360`, scanned over all params, semantics copied verbatim from
`amort_oracle.pas:938/941` so the two drivers cannot drift. **The DEFAULT IS
UNTOUCHED**: with no token present basis stays `x360` and the driver's smoke
test is byte-identical (`monthly 1066.683053`).

**§74's requirement discharged — A NEW TOKEN IS A NEW SURFACE, repeat-sampled:**
10 runs per token on two independent cases, **1 distinct answer each**. No
nondeterminism of the `ParseDMY` kind.

> **⚠️ NOTED BY THE ROUND'S AUDIT (§82's hazard, in a mild form): rebuilding
> `mtg_oracle` REPLACED the binary that produced the published mortgage zero,
> and `build.log` is 0 bytes, so that build's defines are recorded nowhere.**
> The "default untouched" claim therefore could not be checked against the old
> binary. **It was checked empirically instead: the full mortgage differential
> was re-run against the REBUILT oracle and passes (`ok … 15.2s`), so the
> published surface reproduces.** Record the build flags next time — this is
> R47's disclosure requirement applied to our own driver.

### 2. WHAT IT MEASURED

```
mtg_oracle apr 250000 0.20 30 0.0725 0.015 0 0 [token]
  <none> 0.0742490000   b360 0.0742490000   b365 0.0742490000   b365_360 0.0742490000
```

**IDENTICAL ON EVERY BASIS.** R51 says an inert change is a statement about the
change until you show it reaches, so this was read from the Pascal (rule 5)
rather than concluded from the null:

- `SetYrDays` (**INTSUTIL.pas:333**, the active `{3/94}` variant) sets
  `yrdays := 365.25` **only** for `x365`; everything else — **including
  `x365_360`** — gets 360.
- **`Mortgage.pas` never reads `yrdays` at all** (zero occurrences in the unit).

**DOS's mortgage screen is day-count invariant BY CONSTRUCTION.**

### 3. 🚨 AND THE PORT IS TOO — THE ROUND'S OWN CONFIDENT FINDING, REFUTED BY ITS OWN POSITIVE CONTROL

`internal/api/handlers.go:78-83` (`mtgAPRYrDays`) returns **365.25 for every
basis except the literal `"360"` — including `""`, the shipped default** — and
passes it to `FullTermAPR` at `:668` and `:767`. Round 44 wrote the obvious
conclusion: *the default production path computes every mortgage APR on a
different day-count than DOS.* **That claim is FALSE, and the first form of
`zzr44_mtg_basis_test.go` failed on its own R24 vacuity guard, which is how it
was caught:**

```
interest.YieldFromRate(rr, n, yrdays) -> nn := RealPerYr(n, yrdays)
RealPerYr consults yrdays for `daily`, 52 and 26 ONLY; every other n returns n.
   rates.go:42-54  ==  intsutil.pas:1255-1261, VERBATIM
```

**The mortgage screen compounds MONTHLY. `n` is 12 at every APR call site. So
`yrdays` is structurally inert in the mortgage APR — in DOS and in the port
equally, by the same line of the same function.** Measured: 360 / 365 / 365.25
all give `0.0742486057`.

### 4. THE VERDICT

**F7 / restatement §1e are CORRECT that the mortgage zero (30,000 cases /
135,853 APR verdicts) is a 360-ONLY statement — and that restriction is
IMMATERIAL, because the excluded axis cannot move the answer.** `mtgAPRYrDays`
is dead code on the mortgage APR path, not a defect.

- **Item 0-13b: CLOSED.** The token is shipped and the measurement is done.
- **Item 13 (the mortgage APR arithmetic): its premise is RETIRED.** There is
  nothing to change; Nate's "do not change the arithmetic" needs no revisiting.
- **The mortgage zero does NOT need the R47 day-count caveat** it has been
  carrying. It needs the caveats it always needed (no date axis, 360-only
  *and immaterially so*).

**⚠️ WHAT IS NOT RETIRED.** This covers the **APR only**. It says nothing about
any other mortgage cell, and the whole argument **lapses if a compounding-
frequency selector is ever added to the mortgage screen** — the guard's positive
control asserts `yrdays` IS load-bearing at daily compounding precisely so that
this cannot go stale invisibly. **And it is not a substitute for item 0-MTG**,
the read-back audit of the mortgage display layer, which remains unstarted.

**Guarded by** `internal/finance/mortgage/zzr44_mtg_basis_test.go`
(`TestR44MortgageAPRIsDayCountInvariantLikeDOS` with an in-guard positive
control, and `TestR44MortgageFuzzerHardCodes360` pinning why the differential
never explored the axis: 5 of 5 call sites pass a literal 360).

---

## §88 — NF-1 ROOT-CAUSED AND FIXED: DOS's UNCONDITIONAL STORE WAS PORTED INSIDE ITS OWN GATE — PLUS NF-1c (NEW), NF-2 RE-DESCRIBED, AND ITEM 5 ANSWERED (2026-08-10, round 45)

**NF-1 had been open and untouched since round 39D — five rounds, four of them
with it at the top of the plan.** Every round invented a prerequisite. Round 45
budgeted it as a fix, and it took one afternoon of reading Pascal.

### 1. NF-1 — THE ROOT CAUSE IS A GATE BOUNDARY, NOT ARITHMETIC

`AMORTOP.pas:1571-1592`, inside `Re_Amortize`'s `else { not amtok }` arm
("compute new payment amount"):

```pascal
if (user_nballoons > 0) or (npre > 0) or ((df.c.exact) and (df.c.basis<>x360)) then
  begin
    ...
    if Iterate(p, usap, Payment.date, t, d, til_adj) then
      begin
        adj[next_adj]^.amount       := d;      { GATED store }
        adj[next_adj]^.amountstatus := outp;
        adj[next_adj]^.amtok        := true;   { the LATCH, gated }
      end
    else begin abort := true; errorflag := true; end;
    nballoons := save_nballoons;
  end;

adj[next_adj]^.amount       := d;              { UNCONDITIONAL store }
adj[next_adj]^.amountstatus := outp;           { OUTSIDE the gate }
```

**DOS stores the re-amortized payment on EVERY crossing.** Only the `amtok`
latch — DOS's "a later walk may reuse this", and the branch selector at
`AMORTOP.pas:1515` — lives behind the balloons-or-prepay-or-exact gate.

`dosport_walk.go:867-870` ports that faithfully, which is exactly why 39D
measured the dosport route **clean over 558 screens**. `engine.go` put the
**whole** store inside the gate. So on any PIECEWISE screen with no balloon, no
prepayment and not exact-non-360, the amount DOS paints was computed, used to
build the schedule, and then **discarded**: the echo's "has a value" test
(`a.AmtOK || a.AmountStatus == types.InOutOutput`) saw neither flag and the cell
came back BLANK.

**That is Nate's blank cell, and NF-1's "one engine" framing was exactly right —
it just had a one-line cause.**

**THE FIX.** Add DOS's unconditional store at the end of the `!hasAmt` arm,
`adj.Amount = d; adj.AmountStatus = types.InOutOutput`, **deliberately without
`adj.AmtOK = true`** — because DOS does not set it there either.

**⚠️ IN-ADVANCE IS UNREACHABLE FOR A RATE ADJUSTMENT, AND 39D's DESCRIPTION
OVERSTATED THE POPULATION:**

```
amort_oracle 100000 0.07 360 12 adj=84:0.08: inadv adjdump
  → ERR Sorry - you can't change rates when interest is computed in advance.
```

The reachable piecewise ∩ rate-adjustment population is **R78, daily, and the
AO6-carrying screens** — not "AO6 / in-advance / R78 / daily".

### 2. THE MEASUREMENT, AND THE PRE-DECLARATION IT WAS WRITTEN AGAINST

Pre-declared **before measuring** (R36/R41): *this is a display-transport
correction; the FOUR-question column is the control and must not move.*

Seeds 50100-50109, `PERSENSE_FUZZ_N=400`, `horizon` key, no engine filter,
pre-fix vs post-fix binaries over the identical generator:

| | pre | post |
|---|---|---|
| compared pooled | 2,211 | 2,211 |
| **Q4 HARD in-scope (THE CONTROL)** | **23** | **23 — HELD** |
| **Q7 HARD in-scope** | **30** | **25** |
| `adj_amount_missing` findings | 5 | **0** |
| every other signal class | — | **byte-identical** |

`paired_regression.sh` over the same seeds: **FIXED 5, STILL BROKEN 62, NEW 0.**

**In-scope Q7: 30 in 2,086 = 1 in 70 → 25 in 2,086 = 1 in 83.**
Bar miss **5.75× → 4.79×**.

**🚨 HOW TO READ THAT, AND HOW NOT TO.** The five removed cases were HARD *only*
because the amount echo was missing; their totals were already correct. This is
**a real user-visible defect closed**, and it is **NOT engine convergence** — no
arithmetic moved, and the Q4 control proves it. The bar is still missed by 4.8×.

**⚠️ AND THE ROUND-43 PRE-DECLARATION WAS WRONG IN BOTH DIRECTIONS, AS §86
PREDICTED.** It said the fix would move Q7 "toward ~1 in 87 for wholly
display-layer reasons". The move is smaller (1 in 83, not 1 in 87) because
NF-1's arm is only part of Signal 7; and the residue is `adj_rate_differs`,
which is an ENGINE matter and remains open.

### 3. 🚨 NF-1c — A NEW USER-VISIBLE DEFECT, FOUND BY THE MANDATED PRIOR-ART SWEEP

**`docs/discrepancies.md:2885-2887`, written 2026-08-07, left an explicit
forward warning:**

> *"On the out-bound direction there is nothing to un-kick today …
> `AmortizationResponse` … has **no adjustment echo field** … If such an echo is
> ever added it **MUST go through `amzUnkickerRate`** — noted here so the
> pairing is not lost."*

**Round 39 added exactly that echo two days later and did not wire the un-kick.**
The INBOUND leg kicks (`row.LoanRate = amzKickerRate(*a.Rate, basis)`); the
outbound echoed `a.Rate` **raw**, while the LOAN rate on the same response is
correctly un-kicked. The handler's own comment says *"the echo divides it back"*
— true of the loan rate, false of this one.

DOS, `INTSUTIL.pas:1649-1651` — `adjratecol` sits in the SAME arm as `aratecol`,
and only the APR column is exempt:

```pascal
aratecol,adjratecol,aaprcol :
  if (df.c.basis=x365_360) and (col<>aaprcol) then
     PercentValueFromCell := ReportedRate(rp^)/kicker
  else PercentValueFromCell := ReportedRate(rp^);
```

**Measured.** User types 8% on the 365/360 basis (internal
0.081111111111111106); AO6 adjustment, payment 900 typed, rate blank:

```
amort_oracle 100000 0.081111111111111106 360 12 adj=84::900.00 b365_360 adjdump
  → adjrow 1 rate 0.1064151870 ratestatus 1
```

DOS's screen shows `0.1064151870/(365/360) = 0.1049574447`. **The port painted
0.1064151870 — 0.146 percentage points high, 1.3889% relative, in a cell the
user reads as an interest rate.**

**⚠️ ONLY THE SOLVED (AO6) ARM IS VISIBLE.** A caller-typed adjustment rate
echoes `rateSolved:false` and the UI leaves it alone. That is why this survived
five rounds.

### 4. 🚨 NF-2's PUBLISHED DESCRIPTION WAS WRONG — RETRACTED (rule 11)

START_HERE and 39D both record NF-2 as *"DOS snaps the adjustment onto the
payment grid and echoes the SNAPPED date; the DOM row keeps the typed date."*
**The first clause is not a defect: THE PORT ALSO SNAPS, and always did** — the
port of `Amortize.pas:258-271` runs inside `Amortize` ahead of `ValidateInputs`
and ahead of the dosport delegation, mutating `input.Adjustments` in place, so
**both** engines' echoes already carried the snapped date.

**The break was entirely at the CONSUMER**, `index.html`:

```js
if (rowISO !== a.date) return;
```

`rowISO` is parsed from the DOM row's own date input, which still holds what the
user typed. For an off-grid date that test failed for **every** row, the forEach
returned for all of them, and nothing was painted — however correct the echo
was. 39D measured 16 of 400.

**So NF-2 is R42 in its purest form: verified at the producer, broken at the
consumer.** Fix: the response carries `requestedDate` when (and only when) a snap
moved the row; the client matches on `requestedDate || date` and writes the
SNAPPED date back into the cell as a computed output — which is what DOS does
(`datestatus := defp`, *"Let user know we've adjusted rate change date"*).

### 5. ⭐ ITEM 5 ANSWERED — AND BOTH PRIOR ANSWERS WERE WRONG

The question: **how many of the r41 twenty APR divergences sit on a
TOTALS-GREEN screen?** Round 44 measured 0 of 7 across four seeds and concluded
the class might dissolve into the totals classes.

Over the full r41 population (seeds 50100-50109, N=400):

| measure | answer |
|---|---|
| APR divergences | **20** |
| totals-green **by the instrument's own signal set** | **2** |
| totals-**IDENTICAL** by ground truth (oracle vs port) | **1** |

**🚨 AND THE FIRST ATTEMPT AT THIS NUMBER WAS AN ARTEFACT — recorded because it
is the round's best near-miss.** Joining APR cases to totals cases on the
reproducing oracle command line returned **20 of 20 totals-green**, which would
have been a spectacular result. It was false: the `apr_differs` line ends
`… adjdump bdump apr` and the `divergent_class` line for the SAME case ends
`… adjdump bdump`. **The exact-string join found zero overlap by construction.**
Stripping the output-only tokens {`apr`,`adjdump`,`bdump`} gives 2, not 20.

**The one genuinely totals-identical case:**

```
amort_oracle 39942.04 0.0881870000 1320 24 b365 prepaid inadv plusreg r78 usa \
  loandmy=30.9.2023 firstdmy=30.11.2023 targ=30.28 pts=0.018003 \
  payhard=179.29 non lastdmy=30.10.2133
  ORACLE: payment 179.2900 interest 44809.93 paid 84751.97   apr 0.086663
  GO    : payment 179.2900 interest 44809.93 paid 84751.97   apr 0.086656
  GENGINE piecewise reason=in_advance_or_r78_or_daily
```

**Totals byte-identical; the APR differs by 7e-6.** Two properties matter:

1. **It carries NO ADJUSTMENTS AT ALL** (no `adj=` token). §85's withdrawn
   attribution of the APR class to the piecewise adjustment-rate solve **could
   not have covered this case**, which is independent support for the
   withdrawal.
2. **Its horizon is 2133 — OUT OF SCOPE.** So the one clean isolate the project
   owns is outside the population every published in-scope rate is measured on.

The other totals-green-by-signal case (seed 50104, in scope) has a **$4.05**
totals delta sitting under the fuzzer's `max($1.00, 5e-4·|DOS|)` = **$27.97**
floor — CAUTION 1's unpinned tolerance floors (item 0e) biting directly.

**VERDICT: the residual APR class does NOT dissolve.** 19 of 20 remain
confounded with a totals divergence at some tolerance, so **R27 stands and the
class stays UNATTRIBUTED.** But the decisive case §85 asked for now exists, is
named, and is adjustment-free.

### 6. WHAT THE ADVERSARIAL AUDIT TOOK OFF US — R32 IS 12 FOR 12

Run with the complete change inventory (R46), **before** the document edits.

- **🚨 IT REFUTED THIS SECTION'S OWN REASONING.** The commit comment claimed
  *"every OTHER reader of an adjustment's AmountStatus thresholds at
  `>= InOutDefault`"*. **False** — `engine.go:1945` and `dosport_entry.go:1237`
  compare `== InOutOutput`, and the write flips exactly those. That IS the
  intended effect, so the CONCLUSION (no arithmetic moves) survives, but the
  stated argument was wrong and is corrected in the source.
- **It caught that the item-5 join method is sound only by accident here**: the
  fuzzer prints one `SIG=HARD:divergent_class` line **per class**, not per case.
  All 24 classes in this run read `1/x`, and the per-seed
  `divergent option classes: N of M` totals sum to 24, so the set is complete —
  **but the method must not be reused without re-checking that field.**
- **It confirmed C2 against the Pascal**, including that DOS still reaches the
  unconditional store when the gated `Iterate` FAILS (`abort` is a flag, not a
  control transfer; the only label is inside `Iterate`).

### 7. 🚨 FILED, NOT FIXED — AND ONE STANDING CLAIM IS NOW DOUBTFUL

**`amzUnkickerRate` omits DOS's `ReportedRate` wrapper.** `INTSUTIL.pas:1646-1650`
applies `ReportedRate` on **both** branches; `amzUnkickerRate` divides on
365/360 and is the bare identity elsewhere. `ReportedRate` is the identity only
when the settings frequency lacks the canadian/daily bit
(`INTSUTIL.pas:1499-1504`).

**`docs/discrepancies.md:3637-3640` calls this "unreachable through the REST
surface". THAT CLAIM IS NOT SUPPORTED.** `handlers.go` sets
`Settings.PerYr = byte(req.PerYr)` with no bound, and a request with
`perYr: 76` (= 64 daily | 12) or `perYr: 140` (= 128 canadian | 12) is
**accepted and returns an adjustment echo**. We did not exhibit a *solved* rate
on such a screen, so the gap is **REACHABILITY-UNVERIFIED, not demonstrated
live** — but "unreachable" must not be carried forward as settled.

**An `AmtOK` mutant on the new store SURVIVES and is measurably INERT** (800
cases, seeds 50100/50104, byte-identical SIG counts). The clause is kept on
**faithfulness to DOS (rule 5), not on a demonstrated hazard** — the same shape
as §84's dead disjunct, and recorded as an UNGUARDED mutant rather than left
looking pinned (R38/R51).

**In the gated block, `ok && refined == 0` leaves `d` at the annuity seed** where
DOS's by-reference `Iterate(…, d, …)` would have stored whatever it produced.
Pre-existing, untouched by this round, filed here.

### 8. GUARDS

| file | what it pins |
|---|---|
| `internal/finance/amortization/zzr45_nf1_piecewise_echo_test.go` | NF-1 on the PIECEWISE engine, with in-guard positive controls asserting the route IS piecewise and that DOS's gate is CLOSED; 39D's minimal repro (AO5 + AO6 on one screen); and a blank-blank adjustment that still gets the re-amortized payment |
| `internal/api/zzr45_adjustment_echo_wire_test.go` | **A1-A5, the first API-layer tests the R39 wire fields have ever had**, plus NF-1c with an in-guard positive control that the 360 basis stays IDENTITY, plus NF-2 on BOTH engines |
| `cmd/persense/frontend_r45_adjustment_paint_test.go` | NF-2 at the CONSUMER — the shipped `index.html` block run in node against a fake DOM, with an on-grid negative control |

**Mutation pass: 17 mutants, 16 killed, 1 survivor (the `AmtOK` mutant above,
measured inert and recorded).** All new guards seen to FAIL on `/tmp/pristine`.

---

## §89 — 🚨 THE ROUND-45 NF-2 FIX SHIPPED A CELL THE APP'S OWN VALIDATOR REJECTS, AND THE GUARD THAT WAS SUPPOSED TO CATCH IT COMMITTED THE RULE IT ENFORCES (2026-08-10, round 45b)

**Found by driving the LIVE page in Chrome against `amort_oracle` — the first
DOS-anchored UI differential the project has run (item 24).** It is not an engine
defect and no engine differential could have seen it.

### 1. THE DEFECT

§88's NF-2 fix writes the snapped adjustment date back into the date cell:

```js
dEl.value = a.date;          // the RAW WIRE VALUE — ISO, e.g. "2034-11-30"
```

That input is validated by `dateValidity` (`index.html`), whose regex is
`^(\d{1,2})([\/-])(\d{1,2})\2(\d{4})$` — **MM/DD/YYYY or MM-DD-YYYY only. ISO is
REJECTED.** So the cell ends up holding a value **the application itself
refuses**.

**Measured on the live page, reproduced by hand:**

1. Type `12/13/2034` into an adjustment date, Calculate → **succeeds**; the cell
   is rewritten to `2034-11-30` and marked `cell-output`.
2. Press Calculate again, **changing nothing** → **BLOCKED**:
   *"Fix the highlighted date before calculating: Incomplete date — use
   MM/DD/YYYY or MM-DD-YYYY with a 4-digit year."* The cell is flagged
   `date-invalid`.

**Scale, measured over the live run: 21 of 76 adjustment-carrying screens = 28%
become unsubmittable on the user's SECOND Calculate.**

This is 39D's *"a solved value painted into a grid becomes a hard input on the
next submit"* trap, and its "unsubmittable screen" outcome, **reintroduced one
layer up by the round-45 fix itself.**

**⚠️ WHY NOTHING DOWNSTREAM CAUGHT IT: `parseDate` ACCEPTS ISO while
`dateValidity` REJECTS it.** The two date paths in `index.html` disagree, so the
value round-trips through the request builder perfectly and only dies at
validation.

**THE FIX (one line):** `dEl.value = fmtDateDisplay(a.date);` — the app's own
ISO→MM/DD/YYYY formatter — plus clearing any stale `date-invalid`. Verified on
the live page: cell reads `11/30/2034`, no invalid flag, calc succeeds, and the
answer is unchanged (`Total Interest: $83,642.37`).

### 2. 🚨 THE GUARD FAILED IN EXACTLY THE WAY IT EXISTED TO PREVENT

`cmd/persense/frontend_r45_adjustment_paint_test.go` was written in round 45 to
enforce **R42 — verify at the CONSUMER, not the producer.** It passed throughout,
because it asserted the painted cell **equalled `a.date`** against a **fake DOM
with no validator**. It checked the value it expected instead of asking whether
the application would accept it.

> **A GUARD THAT COMPARES A PAINTED VALUE TO A LITERAL HAS VERIFIED THE PRODUCER
> AGAIN. R42 committed by the guard written to enforce R42.**

**→ R53 — A PAINTED CELL IS AN INPUT TO THE NEXT SUBMIT. A TEST THAT PAINTS A
VALUE MUST FEED THAT VALUE BACK THROUGH THE REAL VALIDATOR, NOT COMPARE IT TO A
LITERAL.** The test now extracts the shipped `dateValidity` and `fmtDateDisplay`
and asserts `dateValidity(painted).valid` on every case, painted or not.
**Both arms seen to fail on a probe tree carrying the round-45 write-back** — the
source needle AND, independently, the behavioural assertion, which reports the
user's exact error text.

### 3. THE INSTRUMENT, AND THE THREE HARNESS BUGS IT ATE FIRST

**Population:** 210 screens — **200 STACKED** (median 7 advanced options,
generated to be UI-expressible) plus **10 PLAIN IDENTITY CONTROLS**. Every grid
row expressed with DATE-based oracle tokens (`bdate=`/`adjdmy=`/`predmy=`/
`mordmy=`) so no installment-number translation can manufacture a divergence.
Oracle flags `-dV_3 -dSCROLLS -dPVLX` (no `-dACTU`; R47).

**🚨 THREE HARNESS BUGS WERE FOUND AND KILLED BEFORE ANY NUMBER WAS BELIEVED:**

1. **`apr` and `bdump` are each a SEPARATE OUTPUT MODE** of `amort_oracle` and
   suppress the payment/adjrow lines. `apr adjdump` returns
   `apr 0.000000 status 0` — **which a naive parser reads as a real zero APR.**
   Two invocations per case are required.
2. **THE 365/360 KICKER — a fake 1-in-3 divergence rate.** The API kicks the
   typed rate by 365/360 to reach DOS's internal rate; the harness handed the
   oracle that same number *as* the internal rate. **63 of 63 `365/360` cases
   diverged — 100%, which is a mapping bug, not a defect.** After kicking the
   loan rate and the adjustment rates: **0 of 63.** *(The tell both times was the
   SHAPE: payment, total-interest and total-paid diverged on exactly the same
   set — the signature of two sides computing different screens.)*
3. **ISO dates are rejected by the UI's own validator**, which blocked the first
   browser pass entirely — the same inconsistency that turned out to BE §89.

### 4. WHAT THE DIFFERENTIAL FOUND ON THE ENGINE

Identity controls **0 of 10**. Stacked **200**: **3 cases**, of which **two are
NOT ADJUDICABLE** (`perYr=26` with adjustment dates the generator placed
off-grid — they are snapped, and both sides cannot be shown to snap identically).

**One is real and clean — every date on the annual grid:**

```
amort_oracle 217463.13 0.1066252194 15 1 loandmy=1.10.2025 firstdmy=1.10.2026 \
  b365_360 usa predmy=1.10.2029:7:4:1835.20 \
  adjdmy=1.10.2034:0.0686234472: adjdmy=1.10.2030::3723.03
```

| cell | DOS | port (PAINTED in the browser) | gap |
|---|---|---|---|
| adj 10/01/2030 — solved RATE | −20.795% | **−23.52%** | ~3.0 pp |
| adj 10/01/2034 — solved AMOUNT | 12,417.72 | **10,462.66** | **$1,955** |
| Total interest | 20,528.59 | **8,798.24** | **$11,730** |

**Ablation isolates it to USA rule ∧ prepayment ∧ adjustment, and doubles as the
mapping control: every option AGREES in isolation and in every PAIR; only the
triple diverges.** The browser reproduced the API-layer figure exactly, so the
display layer transports it faithfully — the port simply fits a different
negative rate on the AO6 solve. **OPEN, UNATTRIBUTED (R27).**

**⚠️ AND A NEW SCOPE FACT: IN-ADVANCE REFUSES *ANY* ADJUSTMENT**, not merely a
rate one — the amount-only AO6 path solves a rate, so DOS rejects it. §88
established this for rate adjustments only.

### 5. GATES

Full suite with `PERSENSE_FUZZ` unset, `check_skips`, and the new guard seen to
fail on a probe tree. **NOT RUN: the randomized fuzz arms** — no engine file
changed; `index.html` and one test are the whole diff.
