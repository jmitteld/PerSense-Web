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

### 15c. `NumberOfInstallments` snapped "Feb 30" terminal — KNOWN, representation-limited (P2)

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

The installment **count** is unaffected and matches DOS. The date only feeds a
valuation in PV's `asOf > fromDate` (retrospective) branch, where `since =
YearsDif(asOf, toDate)` uses the snapped terminal — so the impact is ≤2 days of
30/360 discounting on the anchor of a retrospectively-valued periodic series whose
from-date day is 29/30/31 and whose terminal lands in February, on the 360 basis.

**Why not fixed:** a faithful fix requires either storing raw `{d,m,y}` records
in place of `time.Time` (an engine-wide representation change with ~11k validated
oracle cases at risk) or a bespoke `YearsDif` special-case; both are out of
proportion to a ≤2-day effect in this narrow corner. **Decision:** leave the port
on the normalized `Mar 2` and document the bound. Characterized by
`TestNumberOfInstallmentsFeb30KnownDivergence`, which pins the current Go behavior
and records the DOS value so any future change (or a real fix) is noticed. A
related representation limit — DOS's synthetic **Feb 29 2100** (its calendar
treats 2100 as leap) — is unrepresentable in `time.Time` for the same reason; it
only bites day-29 schedules crossing Feb 2100 and is noted here as bounded.

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

## 22. Amortization audit pass 3 (2026-07-12) — 13 confirmed divergence families: 3 FIXED, 10 OPEN (fixes in progress)

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

### OPEN — confirmed, diagnosed, fix pending (next session)

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
