# Amortization — DOS-Fidelity Review (client feedback triage)

**Date:** 2026-06-13
**Scope:** Client feedback on the Amortization screen, evaluated against the DOS
source (the financial-logic authority), the Windows help docs, the Go engine
(`internal/finance/amortization/`), and the web frontend
(`cmd/persense/static/index.html`). Engine claims were confirmed live against the
running server.

## The one root cause behind half the list

Five of the complaints (balloon, principal-minimum eventual payment, "retired
early", "does not amortize", prepaid-off) are the **same bug wearing different
hats**:

> When the Payment is left blank, the port estimates a **plain closed-form
> payment that ignores the fancy features** (balloon, target/principal-minimum,
> odd first period), then bolts those features on afterward. DOS instead solves a
> payment that already accounts for them, so the loan amortizes over the stated
> term.

In the code: `SolvePayment` / `solveFancyPayment` exist (`backward.go:70-146`) but
are **dead code** — the engine calls the plain `estimatePayment()`
(`engine.go:256-263`) and only refines via bisection when *skip-months* are
present (`engine.go:384-395`). Balloons, targets, and odd-first-period are never
fed back into the payment solve. Fixing this one thing resolves items 3, 8, 9, 10,
and 12b. Confirmed live (100k, 6%, 120 mo, loan 6/13, 1st pmt 8/1):

| Scenario | Port result | Should be (DOS) |
|---|---|---|
| + $20k balloon @ pmt 60, blank pmt | payment stays 1110.21 → **retires early at 98/120** | payment re-solved so it amortizes to 120 with the balloon |
| Principal-min $800, blank pmt | early pmts 1300→…, eventual = **1110.21 (no-target base)** → **retires at 114/120** | eventual constant solved *lower* so it still amortizes to 120; that constant shown on top line |
| Prepaid OFF, odd first period | payment stays 1110.21; odd interest dumped into pmt 1 | payment **augmented** (higher, constant) to absorb odd interest over the term |

---

## A correction on the F10 item

The client said "In the DOS version F10 calculates." The DOS/Windows help actually
shows **Enter computes** the payment/rate and **Ctrl-T** prints the table; **F10
pages through** the table output once it's up (`AM_GridBasicLoanInfo.html:27` —
"Press <Enter> for computation… or Ctrl-T to generate a table"; `AM_EX1.html:18` —
"Press F10 to begin. The table appears one screenful at a time").

So the real, defensible fix for "still not calculating on Enter" is: **make Enter
trigger the calculation on the Amortization screen** (it currently just advances to
the next field — a deliberate web-port change). That matches DOS. We can also bind
F10 to calculate if the client prefers the muscle memory. This is a UX decision —
see "Decisions needed."

---

## Item-by-item

Severity: **ENGINE** = Go financial logic; **FE** = frontend only; **DEC** =
needs a client decision.

### 1. Enter doesn't calculate (FE) — ✅ DONE
- **Decision (client):** Enter moves to the next field AND recalculates when
  Auto-calculate is on; F10 is a general Calculate everywhere.
- **Done:** Enter handler now also calls `scheduleAutoCalc(t)` so it recalcs on
  commit (covers the last-field case where focus doesn't move); added an `F10`
  keydown branch that dispatches to the active screen's calc
  (`calcMortgageRow`/`calcAmortization`/`calcPV`), working regardless of focus but
  not behind a modal/tour. Shortcut hints updated on all three screens.
- **Verified live:** F10 calculates with Auto-calc on and off; Enter triggers an
  auto-calc when enabled. Regression guard: `TestKeyboardCalcWiring`.

### 2. Payoff "as of" ignores partial-month interest (FE) — ✅ DONE
- **Done:** `updatePayoffBalance` now reports the balance owed just before the
  as-of date PLUS interest accrued since the last payment. It uses the last payment
  *strictly before* the date (so a payment-date query shows the pre-payment balance,
  per `AM_GridPayoffBalance.html`) and accrues partial-period interest with a
  basis-aware day count (`amzYearFraction`: 30/360, actual/365, actual/360).
- **Verified:** 100k @ 6%, 30/360 — a 15-day accrual adds $250; a full month adds
  $500 (= the pre-payment one-period interest). Implemented client-side off the
  schedule + loan rate/basis (no new endpoint); for ARM loans it uses the initial
  rate for the accrual (acceptable approximation).
- **Follow-up fix (client report): the typed Payoff date snapped to a payment
  date (e.g. 08/15 → 08/01).** Cause: the computed Balance wasn't marked as an
  output, so the inverse "Balance → date" lookup treated it as a hard target and
  rewrote the date to the nearest schedule payment date (always the 1st). Fix: the
  payoff pair now follows the white-input/green-output convention — typing a date
  makes the Balance a green computed output and the inverse never fires; only a
  user-typed Balance drives the date lookup (which is then shown green). Verified:
  typing 08/15/2027 keeps the date and shows Balance $94,996.61 (= 08/01 balance
  $94,555.35 + 14 days interest).

### 3. Adding a balloon should recompute the payment (ENGINE) — ✅ DONE
- **Done:** when the payment is blank and a known balloon is present, the engine
  now solves the regular payment via the schedule-oracle bisection
  (`solveFancyPayment`, previously dead code) so principal + balloon amortize over
  the term. Wired in `engine.go` after the balloon/prepayment dispatch; scoped to
  known-balloon and target cases (adjustments/prepayments keep their own payment
  handling, which the regression caught).
- **Verified:** `TestAPIAmortBalloonBlankPaymentSolves` — no "retired early"/"does
  not amortize" warnings, runs to payment 120, and the solved payment is lower than
  the no-balloon baseline.

### 4. Entered amounts not reformatted to $xx,xxx.xx (FE) — ✅ DONE
- **Done:** amortization money inputs (amount, payment, principal-min, payoff
  balance, and balloon/prepay/adjustment amounts) reformat to `$xx,xxx.xx` on
  blur via `formatMoneyField`. Default amount and computed echoes now use the same
  `$` format for consistency. `parseMoney` strips `$`/commas, so calc is
  unaffected; unparseable text is left alone.
- **Verified:** `100000` → `$100,000.00`, `$1,234.5` → `$1,234.50`, `abc`
  untouched. Guard: `TestAmzDateMoneyHelpersJS`.

### 5. First-payment date default = first of the SECOND following month (FE done; ENGINE optional)
- **Done (FE):** committing the loan date writes the DOS default into the
  first-payment field as an editable soft default (`computeDefaultFirstPayment`,
  matching `Amortize.pas:184-194`): loan day > 1 → first of the second following
  month (6/13 → 8/1); on the 1st → next period (6/1 → 7/1); honors the payment
  frequency for the monthly family. Marked `data-soft` so a later loan-date change
  refreshes it, but a user-typed value is never overwritten.
- **Verified:** 6/13 → 2024-08-01, 6/01 → 2024-07-01, 12/15 → 2025-02-01,
  quarterly 6/13 → 2024-12-01, biweekly → null (left to engine).
- **Done (ENGINE):** `FirstPass` A-FP-defFirst now implements the DOS
  `DefaultFirstPaymentDate` rule (Amortize.pas:184-194) for direct API callers too —
  first of the second following month when the loan day > 1, next period when on the
  1st. Two tests that had encoded the old `loan + 1 period` value were corrected to
  the DOS values (loan 1/15 → 3/01; loan 2/12 → 4/01). Verified: `go test ./...`.

### 6. First-payment date should infer the year from the loan date (FE) — ✅ DONE
- **Done:** the 1st/last payment date fields accept a bare `MM/DD` and infer the
  year from the loan date (`inferAmzDateFromLoan`), rolling forward a year if the
  date would otherwise precede the loan date. Applied on blur (the field expands to
  the full `MM/DD/YYYY`) and again in `getAmzInput` so it also works when the user
  hits F10 without leaving the field. Scoped to these fields and to year-elision
  only — the global 4-digit rule (no 2-digit century guessing) is unchanged.
- **Verified:** loan 6/13/2024 → `8/1` becomes 2024-08-01; `1/1` becomes
  2025-01-01; a full date is passed through untouched; no loan-date anchor → falls
  back to the strict parser. Guard: `TestAmzDateMoneyHelpersJS`.

### 7. Rename "Target Amt" → "Principal minimum" (FE) — ✅ DONE
- **Done:** column relabeled "Principal minimum" with an updated tooltip
  ("Minimum amount by which the principal must decrease each payment…"). API key
  `targetAmt` unchanged. (Client chose "Principal minimum" over the original help's
  "Targeted Principal Reduction".)

### 8. Top-line payment under a principal-minimum = eventual, not first (ENGINE + FE) — ✅ DONE
- **Done (ENGINE):** the eventual constant is now solved (item 9). **Done (FE):**
  under a principal minimum the top-line payment shows the *modal* regular payment
  (the steady constant the schedule settles to), not the high first payment.
  Non-target loans are unchanged (all regular payments are equal).

### 9. "Retired early — paid off at payment 114 of 120" (ENGINE) — ✅ DONE
- **Done:** the principal-minimum eventual payment is now solved by bisection so
  the schedule amortizes over exactly the stated term.
- **Verified:** `TestAPIAmortPrincipalMinimumBlankPaymentSolves` — runs to payment
  120, no "retired early" advisory, and payments ramp down (first > eventual).

### 10. "The regular payment does not amortize the loan over the stated term" (ENGINE) — ✅ DONE (root cause)
- **Done:** with the blank payment now solved for balloon/target loans, the
  oversized-final-payment advisory no longer fires for these ordinary cases. It is
  retained only for genuinely over-specified input (an explicit user payment that
  can't amortize), matching DOS's TackOnFinalBalloon behavior.

### 11. Remove "Year to divide century" from Settings (FE) — ✅ DONE
- **Done:** the disabled, no-op setting row was deleted; no JS references remain.

### 12. Prepaid interest (ENGINE) — ✅ DONE
- **12a — settlement stub now shows.** Root cause was item 5: the wrong default
  produced no odd first period, so prepaid-ON had nothing to prepay. With the
  corrected default, an omitted first date (e.g. loan 2/12 → 4/01) yields an odd
  first period and the engine emits the PayNum-0 stub carrying the prepaid odd-days
  interest. **Verified:** `TestAPIAmortPrepaidOnShowsSettlementStub` (stub is
  interest-only, interest > 0).
- **12b — prepaid OFF augments the payment.** Done: when the payment is blank,
  prepaid is OFF, and the first period is odd, the engine augments the estimate by
  the DOS `ffFirst/f` factor (Amortize.pas:1513-1522) so the payment stays constant
  and the loan amortizes. **Verified:** `TestAPIAmortPrepaidOffAugmentsPayment`
  (no "does not amortize" warning, final balance ~0, augmented payment > prepaid-ON).

---

## Status

**Done (frontend, verified live + regression tests):** 1 (Enter recalc + F10),
4 (money blur-format), 5 FE (first-payment soft default), 6 (year inference),
7 (Principal minimum), 11 (removed Year-to-century). All in `index.html`; rebuild
to ship. Tests: `TestKeyboardCalcWiring`, `TestAmzDateMoneyHelpersJS`.

**Done (engine, with `go test` validation):** fancy-aware blank-payment solver
wired (`engine.go` → `solveFancyPayment`) for known-balloon and principal-minimum
loans → fixes 3, 8 (engine), 9, 10. Adjustments/prepayments deliberately excluded
(they manage their own payment / shorten the term — caught by the AM_EX6
regression). Tests: `internal/api/amort_blank_payment_solve_test.go`. Full
`go test ./...` green.

**Done (this pass, all `go test ./...`-validated):**

- **Engine first-payment default** (5) → DOS first-of-second-month in `FirstPass`.
- **Prepaid-OFF augmented payment** (12b) → `ffFirst/f` augmentation in `engine.go`.
- **Prepaid-ON settlement stub** (12a) → now appears via the corrected default.
- **Payoff as-of accrued interest** (2) → `updatePayoffBalance` + `amzYearFraction`.

Tests: `internal/api/amort_blank_payment_solve_test.go` (balloon, principal-minimum,
prepaid-OFF augmentation, prepaid-ON stub), `firstpass_test.go` and
`verify_web_help_examples_test.go` updated to DOS values.

**Follow-up fixes from client testing:**
- **Payoff date snapped to a payment date** (08/15 → 08/01): the computed Balance
  wasn't marked as an output, so the inverse lookup rewrote the typed date. Fixed —
  the payoff pair now follows white-input/green-output; see item 2.
- **Negative-amortization Note misfired** on a long odd first period (prepaid OFF):
  the single first period's interest exceeds the constant payment and bumps the
  balance once before it amortizes. `A-W6` now flags only *sustained* growth (a
  rise after the first regular period), so the one-period bump no longer trips it
  while genuine neg-am still surfaces. Tests: `TestAPIAmortNoNegAmOnOddFirstPeriodBump`,
  `TestAPIAmortNegAmStillFlagged`.

- **Moratorium/skip headline payment**: the top-line Payment showed the first
  scheduled payment, which for a moratorium is the interest-only amount (e.g. $750)
  rather than the regular amortizing payment (e.g. $1,465.02). The "show the steady
  (modal) payment" rule — added for principal-minimum — now also covers moratorium
  and skip-months. Uniform schedules are unaffected.
- **Prepayment Stop Date from # Pmts**: the engine already honors `# Pmts` (it stops
  the series after that many extras), but the Stop Date stayed blank. It's now
  computed and shown as a green output: Start Date + (# Pmts − 1) periods at the
  row's frequency (e.g. 9 yearly from 01/01/2027 → 01/01/2035). Test:
  `TestAmzPrepayStopDateJS`.
- **Date-only balloon ("calculate a balloon")**: confirmed DOS-correct. DOS
  `EstimateAndRefineBalloon` (Amortize.pas:633-660) returns the payoff balance only
  when the balloon date equals the last payment date; for an earlier date it solves
  the amount that completes repayment over the term, which is ~0 for a
  self-amortizing loan (hence the "balloon essentially zero" advisory). A meaningful
  balloon needs a non-self-amortizing setup (a lower entered payment, or the balloon
  on the last payment date). The *solved* balloon amount is now echoed into its
  cell (green): engine `AmortResult.Balloons` → API `balloons[]` (with a `solved`
  flag) → frontend fills the matching row's Amount cell, so the calculation is
  visible even when it solves to 0. Test: `TestAPIAmortBalloonAmountEchoed`.

**All 12 client feedback items are now addressed.** Remaining nice-to-haves: the
payoff balance→date inverse lookup doesn't yet add accrual (only date→balance does);
ARM-loan payoff accrual uses the initial rate.

## Decisions — resolved

- **Enter / F10** → Enter moves to next field and recalcs when Auto-calc is on;
  F10 is a general Calculate. ✅
- **Year elision** on amortization date fields, inferred from the loan date. ✅
- **Field name** → "Principal minimum". ✅
