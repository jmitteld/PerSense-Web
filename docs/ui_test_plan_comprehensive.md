# Per%Sense — Comprehensive UI Test Plan

**Status:** living document · **Owner:** QA pass 2026-06-24 · builds on
`fresh_ui_test_report.md` / `fresh_ui_test_matrix.md`.

## Purpose

Go beyond the first fresh pass (which verified the headline worked-examples) to an
**exhaustive cross of functionality × features**, with two emphases the client cares about:

1. **Reuse-on-top** — performing new input on top of old input on the same sheet/row without
   clearing, and proving (a) the new result is correct and (b) nothing stale from the prior
   calculation contaminates it (leftover rows, advanced options, solved/green fields, a stale
   dispatch path).
2. **Intermediate-row sanity** — not just the totals, but every row of every generated
   schedule must satisfy a set of invariants and agree with the DOS engine.

## Method

- **Driven live in the browser** against the running `persense` build (localhost:8080) — the
  test exercises the real rendered UI: typing, clicking, dropdowns, reading the rendered
  results, errors, advisories, and schedule tables.
- **Oracles for correctness:** in-product Help worked-examples; the rebuilt **DOS source
  engine** (`amort_oracle`/`pv_oracle`/`mtg_oracle`); and a **schedule-invariant checker**
  (below) run over the schedule the UI actually rendered (extracted from the page).
- **Severity:** S1 wrong number · S2 misleading/confusing · S3 polish.

---

## Part A — Dimension catalog (the axes of the cross)

### Global Settings (affect calculations)
| Setting | Values |
|---|---|
| Basis | 360 · 365 · 365/360 |
| Prepaid interest | YES · NO |
| In-advance (annuity-due) | YES · NO |
| Exact method | YES · NO |
| Balloon/prepayment includes regular pmt | YES · NO |
| COLA escalation month | Anniversary · Continuous · specific month |
| Auto-calculate | on · off |

### Mortgage screen
- **Dispatch paths:** forward (solve Monthly); reverse (solve Price); %Down↔Cash↔Financed
  trio (fill one, solve others); balloon-amount solve; harden-and-reuse a computed value.
- **Modifiers:** Points→APR; Tax+Ins; Balloon Yrs/Amt.
- **Tools:** Compare APR (selected-row anchor); What-If (1-D and 2-D); Calculate Row /
  Calculate All / Clear Row / Clear All; Export CSV.

### Amortization screen
- **Core dispatch:** solve Payment / Amount / Rate / #Periods / Last-Date (field-presence).
- **First-payment date:** explicit · blank-default · **odd (short/long) first period**.
- **Frequencies (Pmts/Yr):** 12 · 52 (weekly) · 26 (biweekly) · 4 (quarterly) · 1 (annual)
  — note auto basis-switch to 365 for weekly/biweekly.
- **Advanced Options:** Prepayments (known amount · solve-amount · solve-stop-date);
  Balloons (known · target/unknown); Adjustments/ARM (rate-only · payment-only · both ·
  neither=re-amortize); Moratorium; Target (principal minimum); Skip-months.
- **Tools:** Payoff/Balance lookup; View per-payment vs annual summary; Export CSV.
- **Settings cross:** basis × prepaid × in-advance × exact (the engine-cube axes, now UI-side).

### Present Value screen
- **Cash flows:** lump sums (N rows) · periodic series (N rows) · COLA on periodics.
- **Rate Type:** True · Loan · Yield.
- **Backward solves:** IRR (rate) · as-of date · payment amount (PV-4) · through-date (PV-5) ·
  COLA start-date (PV-6) · lump amount · lump date.
- **Life Contingency:** tables (SSA male/female/custom) · 1 or 2 lives (joint-survivor) · DOB ·
  reference date · POD (known · solve-unknown) · per-row Life (None/Living/Dead/1/2/E/B).
- **Variable Rate Schedule** (piecewise rates; backward solving disabled here).
- **Transparency:** "Included in total: N lump · M periodic"; active-summary of leftovers.

### Cross-cutting
Welcome journeys (load example→edit) · Import .psn → edit · screen-to-screen navigation &
state retention · dark mode · auto-calc on/off.

---

## Part B — Reuse-on-top (new-input-on-old-input) scenario catalog

General assertion for every scenario: **the second result equals an independent fresh
calculation of the same inputs, and no value/row/option from the first calc leaks in.**

### Mortgage
- R-M1 forward → change one input → recalc (auto + manual).
- R-M2 forward (solve Monthly) → blank a different field to switch the solved field (reverse).
- R-M3 harden a computed Monthly → change Years/Balloon Yrs → re-solve balloon (Help Ex4).
- R-M4 over-determine a row → read error → fix by blanking one field → correct result.
- R-M5 What-If expands grid to many rows → Clear All → grid resets to initial rows.
- R-M6 Compare APR with selected row = A, then select a different baseline → compare updates.
- R-M7 Clear Row on a solved row → re-enter different loan → no green/hardened residue.

### Amortization
- R-A1 solve Payment → on same sheet blank Payment-given fields to solve Amount → correct,
  no stale schedule.
- R-A2 solved-rate sheet → add a balloon (the A2 case) → advisory shows; numbers DOS-correct.
- R-A3 add balloon/prepay/ARM/moratorium/target/skip → recalc → then **remove** it → recalc →
  result returns to the plain loan (no lingering option; "N active" badge clears).
- R-A4 change a Setting (basis/prepaid/in-advance/exact) and recalc same inputs → schedule
  changes appropriately and fully (no half-updated rows).
- R-A5 load a journey/example → edit one field → recalc.
- R-A6 Payoff lookup at date D → change inputs → re-run lookup → reflects new inputs.
- R-A7 switch frequency (12→52) on an existing loan → basis auto-switch notice; schedule rebuilt.
- R-A8 fancy (advanced) → clear all options → confirm `input.Fancy` drops and plain path runs.

### Present Value
- R-P1 lump → add periodic → add actuarial → recalc; "Included in total" count tracks.
- R-P2 toggle a row's Life None→Living→Dead→None repeatedly → total returns to baseline.
- R-P3 **POD leftover** (known prior issue): set POD, compute, then remove the contingency →
  POD must not keep inflating the total.
- R-P4 IRR solve → change a payment amount → re-solve → new rate; old solved rate not reused.
- R-P5 Variable-Rate schedule → switch back to single Rate → VR no longer applied.
- R-P6 reuse one sheet across consecutive Help examples (the documented reuse flow) → each
  example's number is correct; stale rows flagged in the active-summary.
- R-P7 solve as-of date → then solve rate on same sheet (switch the blank) → correct.

### Cross-screen
- R-X1 enter data on Mortgage → switch to Amortization → back → Mortgage state intact.
- R-X2 import .psn into a screen that already has data → routes correctly / replaces cleanly.

---

## Part C — Intermediate-row sanity invariants

Applied to **every** generated schedule (extracted from the rendered UI table / `amzScheduleData`):

1. **Row count** = #periods (+ at most one settlement stub row, payNum 0).
2. **Dates** step by exactly one period (per Pmts/Yr) from the first-payment date; monotonic;
   no gaps or duplicates; last row date = last-payment date.
3. **Opening balance** (first regular row) = loan amount (after any row-0 stub).
4. **Balance** is non-increasing for a plain loan; drops by `principal` each row; steps down at
   balloon/prepayment rows; flat (interest-only) during moratorium; **never negative**; final
   balance ≈ 0 (±$0.02) unless an intentional balloon/target leaves a stated remainder.
5. **Interest** `≈ balance_prev × rate / pmtsPerYr`, adjusted for basis (30/360 vs actual/365)
   and the odd first period (prorated by days).
6. **Principal** `= payment − interest` on amortizing rows.
7. **Payment** constant for a plain loan; changes only at ARM adjustments, target ramp,
   moratorium boundary, or skip months (0 / interest-only per setting).
8. **IntToDate** (cumulative interest) is monotonic increasing; final = totalInterest.
9. **Σ principal** over rows reconciles to loan amount (with draws/balloons).
10. **totalPaid = Σ payment = totalInterest + principal repaid.**
11. **Skip months**: the named calendar months show $0 (or interest-only) and the balance
    behaves consistently; target overrides skip where both set (DOS AMORTOP.pas:643).
12. **Balloon rows**: payment = regular + balloon (default) or replaces (setting YES); balance
    drops by the balloon amount.
13. **ARM rows**: interest after an adjustment date uses the new rate; payment re-solved for a
    rate-only adjustment.
14. **DOS cross-check**: totals and ≥3 spot rows match `amort_oracle` within tolerance.

A small checker script (`scripts/schedule_sanity.py`, scratch) ingests the rendered schedule
JSON and asserts 1–14, reporting the first violating row.

---

## Part D — Execution priority (this pass)

1. **Reuse-on-top** (Part B) — all screens. *(client-requested first)*
2. **Intermediate-row sanity** (Part C) — representative + every advanced-option schedule.
3. **Feature cross** (Part A) — settings × advanced-option combinations, as far as fits.

Results recorded in `ui_test_results_2026-06-24.md` (pass/fail, findings, coverage achieved).
Anything not reached this pass remains enumerated here for continuation.
