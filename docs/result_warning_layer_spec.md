# Result-Sanity Warning Layer — Predicate Spec

> **Implementation status (shipped).** The layer is built and green across the
> suite. Delivery mechanism: advisories ride the existing `Warnings []string`
> channel as a structured token (`types.FormatAdvisory` →
> `@@ADV|tier|code|fields@@ msg`); the frontend `renderAdvisoryHTML` parses it
> into amber (advisory) / grey (note) and the mortgage path now renders
> warnings for the first time. Per-engine passes live in
> `internal/finance/*/advisories.go`; handler-side rate/amount checks in
> `internal/api/handlers.go`.
>
> Built: **M-W1, M-W2, M-W3, M-W5, M-W6, M-W7** (mortgage); **A-W1, A-W3,
> A-W4, A-W5, A-W6, A-W7, A-W11** plus pre-existing **A-W2** (non-convergence)
> and **A-W9** (TackOnFinalBalloon); **P-W4, P-W7** plus pre-existing **P-W6**
> (over-specified). Fires/doesn't-fire tests in each package's
> `advisories_test.go`. (A-W11 — balloon dropped when the payment is computed —
> was added after the initial pass, prompted by Amortization Example 5.)
>
> Deliberately not built, with reason: **M-W4** (interest-only monthly) — a
> solved Monthly is always amortizing and a sub-amortizing input is the
> legitimate balloon case, so it would false-positive; the interest-only
> advisory lives on amortization as A-W6. **M-W8 / P-W1 / P-W2** — already
> surface as the APR non-converged advisory and the rate-solver's hard
> errors. **P-W3** (rate > 100%) — kept as a harmless backstop but the PV rate
> solver errors out before reaching such rates, so it effectively never fires.
> **A-W8, A-W10, P-W5, L-W1, L-W2** — deferred (prepay-never-retires is already
> a hard error; final-payment drift and date-far-out are low-value/noisy; the
> actuarial survival-probability hooks need deeper plumbing). Cell-outline
> highlighting is deferred — advisories name the field in the message; the
> token already carries the cell ids for a later pass.

**Status:** draft for review. **Scope:** a post-calc advisory pass that flags
*valid-but-nonsensical* solver output (the "−0.72 balloon" class), distinct
from the existing pre-calc validation that rejects un-solvable input.

This does **not** change any financial number. It inspects the solved result
and, when a value is degenerate or almost-certainly-unintended, attaches a
non-blocking advisory to the offending cell.

---

## 1. Where it fits

The engines already carry the machinery this needs:

- **Warnings channel** — `PVResult.Warnings`, `AmortizationResponse.warnings`,
  `MortgageResponse.warnings`. These advisories ride the same channel.
- **Cell targeting** — `FieldError{Code, Message, Fields, RowIdx, Block}` in
  `errorDetail` already lets a message point at specific cells; the frontend's
  inline highlighter consumes it.
- **Auto-calc hint surface** — `setAutoCalcHint()` already renders a quiet,
  non-blocking line; advisories reuse it.

The new work is a small **post-solve inspection pass** in each engine: after a
successful solve (`Err == nil`), look at the solved/output fields plus the
dispatch path that produced them, and append advisories. No new solver, no
change to the numbers.

## 2. Severity tiers

| Tier | Meaning | Blocking? | Surfaces in auto-calc? |
|---|---|---|---|
| 🔴 **Error** (exists) | Input can't be solved; calc does not run. | Yes | No (auto-calc is silent on no-result) |
| 🟡 **Advisory** (new) | Calc ran; the value is valid but almost certainly not what you meant. | No | **Yes** — the result rendered, so this is exactly when the user needs the explanation |
| 🔵 **Note** (new) | A legitimate but noteworthy situation (intentional neg-am, final-payment drift). | No | Yes, subtle/dismissible |

Key interaction with the auto-calc feature: advisories and notes fire on a
**successful** result (`ok == true`), so unlike hard errors they are *not*
suppressed in silent auto-calc mode. That is deliberate — the −0.72 appears
during auto-calc, so its explanation must too.

## 3. General rules

1. Run only when the solve succeeded; never block; never alter the value.
2. One advisory per offending cell; dedupe by `Code`.
3. Attach to the **solved** cell (the one the user left blank), not the inputs,
   unless the message explicitly asks the user to change a specific input.
4. Thresholds are relative and self-scaling where possible (see §7), so they
   hold across loan sizes.
5. Wording follows the house style: name the field, say what's odd, say what to
   do. End advisories with the corrective action.

---

## 4. Mortgage screen (`mortgage.Calc`)

| ID | Trigger (after solve) | Attaches to | Tier | Message |
|---|---|---|---|---|
| **M-W1** | Solved **Balloon Amt** `≈ 0` (`\|balloon\| < one monthly P&I payment`) | Balloon Amt | 🟡 | "Balloon Amt is essentially zero — the Monthly Total you supplied already pays the loan off by the balloon date, so no balloon is needed. To size a real balloon, enter a Monthly Total *below* the full payment." |
| **M-W2** | Solved **Balloon Amt** `< −(one payment)` | Balloon Amt | 🟡 | "Balloon Amt came out negative — your Monthly Total more than pays the loan off before year {balloonYrs}. Lower the Monthly Total, or remove the balloon." |
| **M-W3** | Solved **Balloon Amt** `≥ Amt Borrowed` | Balloon Amt | 🔵 | "Balloon Amt is larger than the amount borrowed — the Monthly Total doesn't cover interest, so the balance grows until the balloon (negative amortization). Intended only if that's the structure you want." |
| **M-W4** | Solved **Monthly Total** `≤` interest-only (`AmtBorrowed × periodic rate`) | Monthly Total | 🔵 | "This Monthly Total only covers interest (or less) — no principal is repaid over the term. Fine for an interest-only structure; otherwise raise the payment or shorten the term." |
| **M-W5** | Solved **% Down** `< 0` (Amt Borrowed > Price) | % Down | 🟡 | "Amount Borrowed exceeds Price, so % Down is negative. Check Price, Cash Required, or Amt Borrowed — exactly one of the three should be your input." |
| **M-W6** | Solved **Price** `≤ 0` or **Cash Required** `< 0` | the solved cell | 🟡 | "{Field} solved to a non-positive value, which isn't a real loan. Re-check the inputs feeding it." |
| **M-W7** | **Balloon Yrs** `≥ Years` | Balloon Yrs | 🟡 | "Balloon Yrs is on or after the loan's final year, so the balloon never takes effect. Set Balloon Yrs earlier than {Years}." |
| **M-W8** | APR iteration hit its cap (non-converged) | APR | 🟡 | *(extend existing non-converged-APR advisory)* "APR didn't converge for this row — the reported figure is approximate." |

## 5. Amortization screen (`engine` + backward solvers)

| ID | Trigger (after solve) | Attaches to | Tier | Message |
|---|---|---|---|---|
| **A-W1** | Solved **Rate** `≤ 0` (payment `< AmtBorrowed / #Periods`) | Rate | 🟡 | "The payment is below principal ÷ number of payments, so the implied Rate is zero or negative. Check the Pmt Amount or the term." |
| **A-W2** | Solved **Rate** hit iteration cap | Rate | 🟡 | "Rate didn't converge — no single rate makes this payment amortize the loan. Check Amount, Pmt Amount, and the term." |
| **A-W3** | Solved **Loan Amount** `≤ 0` | Amt Borrowed | 🟡 | "Amount Borrowed solved to a non-positive value — the payment can't support a positive loan at this rate and term." |
| **A-W4** | **Target balloon** solves `≈ 0` (`< one payment`) | balloon row Amount | 🟡 | "The target balloon is essentially zero — the regular payment already retires the loan by this date, so no balloon is needed." |
| **A-W5** | **Target balloon** solves `< 0` | balloon row Amount | 🟡 | "The target balloon is negative — the regular payment over-pays before this date. Lower the payment or move the balloon date later." |
| **A-W6** | User **Pmt Amount** `<` first-period interest (forward neg-am) | Pmt Amount | 🔵 | "This payment is below the first period's interest, so the balance grows (negative amortization). Intended only if that's the structure you want." |
| **A-W7** | **Unknown prepayment amount** solves `≈ 0` | prepayment row Amount | 🟡 | "The extra payment needed is essentially zero — the loan already retires on schedule without it." |
| **A-W8** | **Prepayment duration** never retires the loan within the schedule | prepayment row | 🟡 | "Even running to the end of the schedule, this extra payment never retires the loan. Increase the amount." |
| **A-W9** | Schedule ends with a residual balance (over-specified) | Pmt Amount | 🔵 | *(extend existing R4-8 advisory)* "The payment is too small to clear the loan over the term — a terminating balloon of ~${residual} is implied at the end." |
| **A-W10** | Final payment differs from the regular payment by `> one payment` | last row | 🔵 | "The final payment ({final}) differs noticeably from the regular payment — rounding and any advanced options are absorbed in the last installment." |
| **A-W11** | A user-entered Balloon is set but the Payment is computed, so the balloon is dropped (its amount appears in no payment row) | Pmt Amount + balloon | 🟡 | "A balloon is set but the Payment Amount is being computed, so Per%Sense solved the payment without the balloon and the balloon was ignored. Enter a Payment Amount (for an interest-only loan, principal × rate ÷ payments per year) so the balloon settles the remaining principal." |

## 6. Present Value screen (`Calculate` + backward solvers)

| ID | Trigger (after solve) | Attaches to | Tier | Message |
|---|---|---|---|---|
| **P-W1** | **IRR / solveRate**: all cash flows share the sign of the target Value | Rate | 🟡 | "Every payment has the same sign as your target value, so there's no positive rate of return to find. An IRR needs at least one inflow and one outflow." |
| **P-W2** | Solved **Rate** hit iteration cap | Rate | 🟡 | "No rate makes the present value equal your target — check the sign and size of the payments and the target value." |
| **P-W3** | Solved **Rate** implausibly large (`> 100%/yr`) | Rate | 🔵 | "The solved rate is very high ({rate}%). That can be correct for a deep discount, but double-check the dates and amounts." |
| **P-W4** | Solved **Amount** (PV-1/PV-4) `≈ 0` | the row's Amount | 🟡 | "The solved amount is essentially zero — the other rows already account for the target value. Check whether this row is needed." |
| **P-W5** | **Date solve** (as-of / from / through) did not converge, or lands `> 100 yr` from the as-of date | the solved date | 🟡 | "Couldn't pin a sensible date that discounts to your value — check the rate and the target value." |
| **P-W6** | **Over-specified** lump or periodic row | the row | 🔵 | *(exists)* "{Row} is over-specified — the supplied Value is redundant and was recomputed." |
| **P-W7** | **Sum Value** solved `≈ 0` while non-zero payments exist | Present Value | 🔵 | "The payments net to about zero at this rate — inflows and outflows cancel. Verify the signs are what you intend." |

## 7. Actuarial (life-contingent rows)

| ID | Trigger (after solve) | Attaches to | Tier | Message |
|---|---|---|---|---|
| **L-W1** | Survival probability at the payment date `≈ 0` (date far beyond the life table) and the row's amount/value was solved | the solved cell | 🟡 | "Survival probability at this date is near zero, so the contingent solve divides by ~0 and the result is unreliable. Check the payment date or the date of birth." |
| **L-W2** | Solved **contingent amount** `>` ~100× its non-contingent counterpart | the row's Amount | 🔵 | "The contingent face amount is very large because survival to this date is unlikely — sanity-check the date of birth and reference date." |

---

## 8. Thresholds (tunable constants)

| Name | Proposed value | Used by |
|---|---|---|
| `nearZeroBalloon` | `\|x\| < max($10, one regular P&I payment)` | M-W1, A-W4 |
| `negBalloon` | `x < −(one regular payment)` | M-W2, A-W5 |
| `nearZeroAmount` | `\|x\| < max($1, 0.0001 × screen scale)` | P-W4, A-W7 |
| `interestOnlyEps` | within `$1` of the interest-only payment | M-W4, A-W6 |
| `rateImplausible` | solved annual rate `> 1.0` (100%) | P-W3 |
| `dateFarOut` | solved date `> 100 yr` from the reference date | P-W5 |
| `finalPmtDrift` | `\|final − regular\| > one regular payment` | A-W10 |

Self-scaling thresholds (a multiple of one payment, a fraction of principal)
are preferred over flat dollar cutoffs so the same rule holds for a \$20k loan
and a \$2M loan. The flat floors ($1/$10) just keep penny-noise from tripping a
warning.

## 9. Surfacing (frontend)

- **Advisory (🟡):** yellow left-border note rendered through the existing
  auto-calc hint line, plus a subtle outline on the target cell (reuse the
  inline-highlight path, amber instead of red). Non-modal, clears on next solve.
- **Note (🔵):** same channel, lower-emphasis (grey), and collapsible.
- Each carries the `Code` so the frontend can map it to the right cell without
  the brittle message-regex matching used for hard errors today.

## 10. Suggested sequencing

1. **Mortgage M-W1 / M-W2 / M-W5** first — smallest surface, and they cover the
   exact confusion that motivated this (blank balloon → degenerate balloon).
2. Amortization **A-W1–A-W5** (the backward-solve degeneracies).
3. PV **P-W1 / P-W2 / P-W5** (the IRR / date no-solution cases).
4. Notes (🔵) and actuarial (L-W*) last — lower frequency, more tuning.

Each predicate ships with a unit test that drives the solver into the arm and
asserts the advisory fires (mirroring the §0.10 error-message audit pattern),
and at least one that asserts it does **not** fire on a healthy result
(false-positive guard).
