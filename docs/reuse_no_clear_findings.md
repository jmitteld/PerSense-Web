# Per%Sense Web — "Reuse the same sheet/row without clearing" findings

**Date:** 2026-06-04
**Scope:** Running the worked help examples through the web UI while reusing the same worksheet (and, on Mortgage, the same grid row) and never clearing between examples — evaluating both calculation correctness and usability.
**Method:** Live UI testing against `go run ./cmd/persense` (localhost:8080), driven through the browser. Calculation correctness is independently backed by the `internal/finance/*/help_examples_test.go` suites, which pass under `go test ./...` (Go 1.26.4).

---

## Executive summary

1. **Correctness is solid.** Every help example run in the UI matched its documented value, and all are covered by passing Go tests. One apparent Present Value discrepancy turned out to be a rate-convention choice, not a bug (see §4).

2. **The "reuse without clearing" workflow is only safe on Mortgage.** The three worksheets handle leftover data very differently:
   - **Mortgage fails *safe*** — leftover inputs trigger a clear over-determination error; you cannot silently get a wrong number.
   - **Amortization and Present Value fail *unsafe*** — leftover advanced options, COLA, or extra rows are silently absorbed into the next calculation with no error, producing confidently wrong results.

3. **Recommendation:** Treat **Clear Row / Clear All between examples** as the supported workflow, and add a visible indicator when modifiers (advanced options, COLA, extra rows) are active — mirroring the existing "computational settings changed" badge.

---

## 1. Mortgage — reuse one grid row, Calculate All

All single-row examples were entered into **row 1**, one after another, **without clearing**, each computed with **Calculate All**.

| Example | Solves for | UI result | Help doc | Match |
|---|---|---|---|---|
| M01 | Monthly | $1,538.30 (Cash $43,200, Amt Borrowed $160,000) | 1,538.30 | ✓ |
| M02 | Price | $241,749 (Amt Borrowed $188,577.78) | 241,749.12 | ✓ |
| M03 | Monthly | $1,982.84 (Cash $61,600, Amt Borrowed $224,000) | 1,982.84 | ✓ |
| M04 | Balloon Amt | $98,372.47 (APR 8.5321%) | ≈98,372 | ✓ |
| M05 | Monthly | $1,593.67 (APR 8.5323%) | 1,593.67 | ✓ |
| M08 | Balloon Amt | $184,912.27 | ≈184,912 | ✓ |
| M06 | APR comparison | A 8.4257%, B 8.6094%, crossover 8.6984% @ 6 yr 10 mo | same | ✓ |

(M06 inherently needs two rows + the Compare APR button, so the single-row-reuse constraint does not apply to it; it was run separately and is correct.)

### Usability finding — fails safe

Because the grid keeps **hard-typed** values as inputs but auto-treats **computed (green)** cells as blank, reusing a row only works when consecutive examples solve for the *same* unknown. When the next example solves for a *different* unknown, the previous example's hard inputs remain and over-determine the row.

Observed twice, identically:
- **M01 → M02**: M01's Price and % Down lingered; M02 added Cash + Monthly → error: *"Row 1: Price and Monthly Total are both filled in, so there is nothing left to solve. Leave one of them blank…"*
- **M02 → M03**: M02's Cash and Monthly lingered → same error.

To proceed, the conflicting cells had to be **manually deleted** each time. This is friction, but it is **safe**: the app blocks rather than computing a wrong number, and the error names the exact conflict. Transitions that *add* a degree of freedom (e.g. M03 → M04, where an 8-year balloon absorbs the extra value) do not error.

---

## 2. Amortization — reuse the single form

Verified correct in the UI:

| Example | UI result | Help doc | Match |
|---|---|---|---|
| A01 (forward) | Payment $2,648.76; settlement-stub row 0 = $861.11; pmt #1 interest $2,583.33 / principal $65.43 / balance $249,934.57 | same | ✓ |
| A04 (target $1,000) | Payment $2,062.50; pmt #1 interest $1,062.50 + principal $1,000 → balance $149,000; "retired early at payment 117" | same | ✓ |

### Usability finding — fails UNSAFE

Advanced Options (Target, Moratorium, Skip Months, balloons, rate adjustments) live in a panel and **persist across examples** on the reused form. Two failure modes:

- **Incompatible leftover → error, but misleading.** After A04 (Target = 1,000), reusing the form for A01's $250K/12.4%/360 loan produced: *"The principal-reduction Target is too high to be reachable…"* — correct, but the **stale A04 schedule remained visible below the error**, which reads like a result.

- **Compatible leftover → SILENT corruption.** Lowering the leftover Target to **200** (reachable for the $250K/360 loan) recomputed with **no error** but the wrong answer:
  - Payment **$2,783.33** (correct A01 is $2,648.76)
  - pmt #1 principal forced to **$200.00** (correct is $65.43)
  - "loan retired early at payment 317 of a scheduled 360"

A user who set a target for one example and then reused the form would get a silently wrong schedule, the only hint being a green "retired early" note. **This is the highest-priority finding.**

---

## 3. Present Value — reuse the same row

Verified correct in the UI:

| Example | UI result | Help doc | Match |
|---|---|---|---|
| P01 (annuity, True Rate 7%) | $129,531.87 | 129,531.87 | ✓ |
| P02 (same annuity + COLA 3%) | $162,651.50 | 162,651.50 | ✓ |

### Usability finding — fails UNSAFE

Present Value **sums every row present and applies every modifier present**, so leftovers are absorbed silently:

- **Leftover COLA.** With identical periodic inputs (dates, amount, rate), the result reads **$162,651.50** with a leftover COLA vs **$129,531.87** without it — a 25% swing, no error. A user reusing the row for a no-COLA example and forgetting to zero the COLA gets the inflated figure.
- **Leftover extra rows.** Adding a stray **$50,000** lump sum silently raised the total to **$179,531.87** (= $129,531.87 + $50,000), no error. Any leftover lump/periodic row from a prior example is silently included.

---

## 4. Resolved: the P01 "discrepancy" is a rate convention, not a bug

An earlier UI run of P01 produced **$129,734.90** instead of $129,531.87. Cause: the **Rate Type** dropdown was set to **"Loan Rate"** (nominal 7%). The help examples and Go tests specify the rate as the internal **true rate** (`RateEntry{Status: StatusFromRate, Rate: 0.07}`). Setting **Rate Type = "True Rate", 7%** reproduces **$129,531.87** exactly. A true 7% is a slightly higher effective rate than a nominal 7%, which is why the Loan-Rate run came out marginally higher. The engine is correct; this is a user-facing convention choice worth being aware of when reproducing the help docs.

---

## 5. Interaction with the new Auto-calculate feature

A worksheet-level Auto-calculate preference was added this session (recalculate when inputs are sufficient and valid; silent otherwise). It is relevant here: with Auto-calculate **on**, the Mortgage over-determination error is **silently suppressed** until the user presses Calculate. In the reuse-without-clearing workflow this means an Auto-calculate user could type a new example over an old one and be left looking at the **previous** example's stale (green) numbers, with no signal that the new one did not compute. Worth a small hint ("inputs conflict — press Calculate") when Auto-calculate declines to run.

---

## 6. Recommendations (prioritized) — all addressed in this revision

1. **Amortization silent corruption (high).** Make leftover Advanced Options visible/auditable — e.g. a badge or highlight when any advanced option is active, mirroring the existing "computational settings changed" badge — and/or clear the stale schedule when a calculation errors so it cannot be mistaken for a result. — **Done (§7.1).**
2. **Present Value leftovers (high).** Surface "N lump sums, M periodics, COLA active" near the result so silently-summed rows/modifiers are obvious; consider a per-row "active" indicator. — **Done (§7.2).**
3. **Auto-calculate silent decline (medium).** When Auto-calculate cannot solve (e.g. over-determined row), show a faint, non-blocking hint rather than leaving stale output on screen. — **Done (§7.3).**
4. **Documentation (low).** Note in the help/quick-start that the worked examples assume the **True Rate** convention on the PV screen, and that switching between examples is best done with **Clear Row / Clear All**. — **Done (§7.4).**

---

## 7. Implemented changes (2026-06-04 revision)

All four recommendations were implemented in `cmd/persense/static/index.html`. JavaScript passes a syntax check and `go build ./...` is clean; the changes are frontend-only.

### 7.1 Amortization — active-options badge + stale-schedule clearing (rec 1)
- A red **"N active"** badge now appears on the **Advanced Options** disclosure whenever any advanced option is in effect (moratorium, target, skip-months, or any filled prepayment / balloon / adjustment row). It updates live as fields change, on calculate, on clear, and on load. Implemented via `amzAdvActiveCount()` / `updateAmzAdvBadge()`.
- When a manual amortization calculation **errors**, the previously displayed schedule is now **wiped** (`clearAmzScheduleOutput()`), so a stale table from a prior example can no longer sit beneath an error message looking like a result. (Auto-calc runs intentionally leave the prior result in place; see §7.3.)

### 7.2 Present Value — "included in total" summary (rec 2)
- A line under the rate controls now reads e.g. **"Included in total: 1 lump sum · 1 periodic · COLA active · life contingency active"**, recomputed on every edit and after each calculation (`updatePVActiveSummary()`). Because PV sums every row and applies every modifier present, this makes a leftover row or a stray COLA immediately visible instead of silently inflating the total.

### 7.3 Auto-calculate — non-blocking stale hint (rec 3)
- When Auto-calculate is on and a silent run cannot produce a result **while a previous result is still on screen**, a quiet italic hint now appears: *"Showing a previous result — the inputs changed but couldn't be auto-calculated. Press Calculate to update or see why."* (`hasStaleOutput()` / `setAutoCalcHint()`). It is suppressed on a blank/just-started worksheet (nothing stale to flag) and cleared on any successful or manual calculation. This closes the "stale green numbers with no signal" gap noted in §5.

### 7.4 In-app documentation (rec 4)
- The **Present Value** instruction strip now notes that the help-doc figures use the **True Rate** convention (pick it in *Rate Type* to reproduce them) and that every listed row/modifier is summed, so **Clear All** before an unrelated example.
- The **Amortization** instruction strip now notes that Advanced Options stay in effect until cleared (with the badge as the cue) and to **Clear All** before an unrelated loan.

**Verification status:** JS syntax check and `go build ./...` pass, and all four changes were confirmed live in the browser after a server restart:
- **Rec 1** — the **"1 active"** badge appeared for a leftover Target; raising the Target to an unreachable value produced the error **and cleared the previously-shown schedule** (no stale table left beneath the error).
- **Rec 2** — the result line read **"Included in total: 1 lump sum · 1 periodic"**, correctly exposing a leftover $50,000 lump that was inflating the total to $179,531.87.
- **Rec 3** — with Auto-calculate on, blanking the rate (an unsolvable state) left the stale $179,531.87 on screen and showed the hint *"Showing a previous result — the inputs changed but couldn't be auto-calculated. Press Calculate to update or see why."*; restoring a valid rate recomputed and cleared the hint.
- **Rec 4** — the new True-Rate / Clear-All notes render on the Present Value and Amortization instruction strips.

---

## Appendix — example inputs used

**Mortgage** (row 1, Calculate All): M01 = 200000 / 2pts / 20% / 20yr / 8% / $200 tax. M02 = solve Price from 1.5pts / Cash 56000 / 30yr / 8.5% / $200 tax / Monthly 1650. M03 = 280000 / 2.5pts / 20% / 30yr / 8.25% / $300 tax. M04 = M03 + Monthly 1600 + Balloon 8yr. M05 = M03 + Balloon 8yr + Balloon Amt 100000. M08 = 240000 / 0pts / 0% down / 15yr / 8.1% / $0 tax / Monthly 1777.79 / Balloon 15yr. M06 = Row1 10000/3pts/0down/30yr/8.1%; Row2 10000/1pt/0down/30yr/8.5%; Compare APR.

**Amortization** (single form): A01 = 250000 / loan 6/21/1994 / 12.4% / 1st pmt 8/1/1994 / 360 / 12_per_yr. A04 = 150000 / loan 2/1/1995 / 8.5% / 1st pmt 3/1/1995 / 120 / Target 1000.

**Present Value** (reuse row): P01 = as-of 2/18/1994, True Rate 7%, periodic $1000/mo from 2/18/1994 to 1/18/2014. P02 = P01 + COLA 3%.
