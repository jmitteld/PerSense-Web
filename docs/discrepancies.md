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

**Status:** Go port matches DOS (the financial authority).

### Description

When a loan has an *odd first period* — the first payment is not exactly one
compounding period after the loan date (a short or long first gap) — the regular
payment must be adjusted so the loan still amortizes over the stated number of
payments. The authoritative DOS engine refines the payment for this (DOS
`EstimateAndRefinePayment` iterates the estimate, Amortize.pas:416). The Windows
help screens show the *un-adjusted* plain payment for the same inputs.

Example (AM Example 1): $100,000 @ 8%, 360 monthly payments, 30/360 basis, loan
dated 2024-02-12 with the first payment 2024-03-01 (a 19-day short first period):

| Quantity            | Windows help | DOS engine (authority) | Go port |
|---------------------|--------------|------------------------|---------|
| Regular payment     | $733.76      | **$731.98**            | $731.98 |
| Total interest      | $161,499.77  | **$163,513.81**        | $163,513.84 |
| First-payment interest | $422.22   | $422.22                | $422.22 |

Verified directly against the real DOS engine:

```
legacy/oracle/amort_oracle 100000 0.08 360 12 loandmy=12.2.2024 firstdmy=1.3.2024
  → payment 731.9828  interest 163513.81
```

A *natural* first period (e.g. loan 3/1, first payment 4/1) gives the plain
$733.76 in both DOS and the port — the adjustment only applies to odd first
periods.

### Go port decision

Per CLAUDE.md, the DOS version is the authority for financial logic, so the port
augments the payment to match DOS ($731.98). This is implemented as a
schedule-oracle refinement of the closed-form estimate (`oddFirstPeriod` +
`solveFancyPayment` in `internal/finance/amortization/engine.go`) and validated
against the DOS oracle by the `TestDOSAmortizeDispatch*` sweeps. The
`TestVerifyWebAM_EX1_Simple` test asserts the DOS values, not the help values.

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
