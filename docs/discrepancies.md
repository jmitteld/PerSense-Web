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
