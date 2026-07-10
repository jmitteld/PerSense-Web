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
