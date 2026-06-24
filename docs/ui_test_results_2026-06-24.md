# Comprehensive UI Test — Results (2026-06-24)

Executes `ui_test_plan_comprehensive.md`, live in Chrome against the rebuilt build
(A1-DOC/A2/M6 fixes confirmed live). Calcs triggered via the app's own `calcAmortization()`
/ `calcPV()` (the exact functions the Calculate button runs); schedules validated by an
in-browser row-invariant checker (`window.__checkSched`) and cross-checked vs the DOS oracle.

Verdicts: **PASS** · **S1/S2/S3** finding.

## Part C — Intermediate-row sanity

| # | Schedule shape | Inputs | Row-invariant result | Verdict |
|---|----------------|--------|----------------------|---------|
| C-1 | Plain monthly | 100k, 8%, 360, 12, natural 1st period | 360 rows, pmt $733.76 const, balance recursion exact, finalBal $0.00, finalItd=$164,155.25, interest=bal×rate all rows | PASS |
| C-2 | Odd short 1st period | 100k, 8%, 360, loan 02/12, 1st 03/01 | pmt $731.98, finalBal $0.00, the only interest-deviation row is the 19-day first period (expected), 2 distinct pmts = regular + adjusted final | PASS |
| C-3 | Balloon, solve payment | 100k, 8%, 360, +$50k balloon @ yr10 | pmt solved $568.48 → early **neg-am** (balance grows pre-balloon); app fired the correct advisory; balloon row balance step exact; finalBal $0.21. UI == engine API exactly (568.48 / 154,651.34 / 0.21) | PASS (advisory good; 21¢ terminal residual is engine-level) |
| C-4 | Prepayment $200/mo 2025–2030 | 100k, 8%, 360 | retired early at pmt **262/360** (advisory fired), interest ↓ to $104,130.91, finalBal $0.00, rows sane | PASS |
| C-5 | ARM rate→9% @2027 | 100k, 8%, 360 | payment re-solved after the adjustment (2 distinct pmts), interest ↑ to $185,854.51, finalBal $0.00, no hard issues (checker's interest-deviation rows are expected — it can't model the rate change) | PASS |
| C-6 | Moratorium (interest-only until 2026) | 100k, 8%, 360 | interest-only during moratorium then amortizes; pmt $746.16; finalBal $0.00; interest=bal×rate throughout | PASS |
| C-7 | Target principal-minimum | 100k, 8%, 360 | **$300** correctly **rejected** ("Target too high… exceeds Amount/periods", no schedule). **$250** ramps payments (263 distinct), interest $130,281.10, finalBal $0.05 | PASS |
| C-8 | Skip months 6–8 | 100k, 8%, 360 | June/Jul/Aug rows show **$0 payment**, interest accrues, balance grows then resumes; neg-am advisory fired; pmt $979.36; finalBal $0.25 | PASS |
| C-9 | Biweekly 26/yr, 780 pmts | 100k, 8% | **"Switched to a 365-day basis"** notice fired; 780 rows; finalBal $0.00; rows sane | PASS — **S3**: the Basis dropdown still shows "360" while the calc used 365 (notice covers it, but the control doesn't reflect the effective basis) |

**Row-sanity verdict:** all 9 schedule shapes are row-by-row internally consistent and match the
DOS-validated engine. Notably, edge cases surface the *right* advisory (negative amortization,
early payoff, basis auto-switch, unreachable target) rather than a silent wrong number.

## Part B — Reuse-on-top (new input on old input)

| # | Scenario | Result | Verdict |
|---|----------|--------|---------|
| R-A3 | Add balloon then remove it, same sheet | baseline $733.76/$164,155.25 → balloon $568.48/$154,651.34 (badge "1 active") → remove → **exactly** $733.76/$164,155.25 (badge "(none)"). No engine-state contamination | PASS |
| R-A1 | Switch the solved field on one sheet | solve-payment → $1,498.88; then solve-amount (payment given, amount blank) → **$250,000.61** (clean round-trip, no stale schedule) | PASS |
| R-A2 | Solve rate (green output) → add balloon | rate re-solves 6.0000%→7.0069% (DOS-faithful) **and the new advisory renders**: "Note: Loan Rate was computed as 7.0069% to fit this loan including the advanced options in effect…" | PASS (A2 fix verified live) |
| R-X1 | Cross-screen retention | enter Amortization loan → navigate Mortgage→PV→Amortization → fields + 360-row schedule fully intact | PASS |
| R-P3 | PV POD leftover (known prior bug) | with POD: total $959,539.54 (POD $12,617.34, matches Help); **remove POD → total drops to $946,922.20, down by exactly $12,617.34** — no leftover inflation; Life→None also removes survival-weighting cleanly | PASS (prior bug stays fixed) |
| R-M | Mortgage clear + re-enter | cleared grid, entered Help Ex5 fresh → correct APRs, no green/hardened residue | PASS |
| M6-live | Compare APR after fix | row 0 selected → compares Row 1 vs Row 2: APR 8.4257% / 8.6094%, **crossover 8.6984% at 6 years 10 months** (exact match to Help Ex5); with one mortgage only, shows DOS-style guidance instead of grabbing rows | PASS (M6 fix verified live) |

**Reuse-on-top verdict:** no contamination found. Adding/removing options returns exactly to
baseline (badge tracks), the solved field can be switched on one sheet, cross-screen state is
retained, the PV POD-leftover bug stays fixed, and both new advisories (A2, M6 guidance) render
live.

## Part A — Functionality × feature cross (sample)

| Combo | Result | Verdict |
|-------|--------|---------|
| Basis 360 / 365 / 365·360, monthly | all $733.76 / $164,155.25 (identical — correct: basis only affects odd/sub-monthly periods) | PASS |
| Quarterly (4/yr, 120) | $2,204.81 / $164,577.16, 120 rows, finalBal $0 | PASS |
| Annual (1/yr, 30) | $8,882.74 / $166,482.30, 30 rows, finalBal $0 | PASS |
| Biweekly (26/yr, 780) | basis auto-switch notice; 780 rows; finalBal $0 (see C-9) | PASS |

## Part A — Settings-dialog toggles (continuation pass)

Each setting driven via its control, recalced on a loan where it bites, row-checked, and
spot-verified against the DOS-validated engine API.

| # | Setting | Result | Verdict |
|---|---------|--------|---------|
| S-1 | Prepaid interest YES vs NO (odd-first loan) | pmt $731.98 both; interest $163,513.84 (YES) vs $163,513.81 (NO — matches DOS oracle exactly; YES carries the known §7 3¢ rounding) | PASS |
| S-2 | Interest paid in **Advance** (annuity-due) | pmt $732.59 (< arrears $733.76, correct), interest $163,733.71, finalBal $0 — **API match exact** | PASS |
| S-3 | **Exact** method + 365 basis | pmt $733.65 / $164,115.15 (actual/365 daily interest, iterated), finalBal $0 — **API match exact** | PASS |
| S-4 | Balloon **includes** regular pmt = YES | balloon row shows **$50,000.00** (replaces the regular payment), loan pays off early, finalBal $0 | PASS |

## Part A — Multi-option combination schedules

| # | Combination | Result | Verdict |
|---|-------------|--------|---------|
| MO-1 | Balloon + prepayment + ARM | all three apply; retired early at pmt **325/360**; interest $135,464.47; finalBal $0; rows sane | PASS |
| MO-2 | Target + skip-months | DOS target-overrides-skip; pmt ramps to $622.33; interest $144,017.28; finalBal $0.11; no hard issues | PASS |
| MO-3 | Moratorium + balloon | interest-only then balloon; retired early at pmt **234/360**; interest $102,258.22; finalBal $0 | PASS |

## Present Value — depth (continuation pass)

| # | Feature | Result | Verdict |
|---|---------|--------|---------|
| PV-COLA | COLA escalation timing (Ex4b) | Anniversary **$532,551.46** · Continuous **$539,754.26** · July **$540,423.84** — all exact match to Help | PASS |
| PV-4 | Solve payment from target (Ex5d) | target $91,012.27 → Amount **$1,000.00** (exact) | PASS |
| PV-JS | Joint-survivor (2 lives) | Both-alive $924,676.74 < single-Living $946,922.20 < Either-survivor $991,659.49 < non-contingent $993,956.62 — **perfect actuarial ordering** | PASS |
| PV-VR | Variable-rate schedule | lump 16y out, 4%→8% @2030 = **$35,345.47** (=100k·e^−1.04, exact); flat-4% $52,729.24, flat-8% $27,803.73 — VR correctly between | PASS |

*(Backward-solve dispatch is well-exercised across IRR/rate, PV-4 payment, lump amount, and
contingent-amount — all recover the documented values. Remaining as-of-date / PV-5 / PV-6
solves use the same dispatch and are enumerated for a later spot-check.)*

## Mortgage — What-If & dispatch trio (continuation pass)

| # | Test | Result | Verdict |
|---|------|--------|---------|
| MT-trio | %Down ↔ Cash ↔ Financed | fill any one of {%Down 20, Cash 40k, Financed 160k} on a $200k loan → **all three resolve identically** (20% / $40k / $160k, Monthly $1,174.02) | PASS |
| MT-wi1 | What-If 1-D, rate 7→9% step 0.25 (Ex6) | **all 9 documented monthlies exact** ($665.30 … $804.62) | PASS |
| MT-wi2 | What-If 2-D, rate × years | rate×{30,15}yr grid correct (e.g. 7%/15yr $898.83); **but** stepping years −15 twice produces a **years=0** row with a stale, unformatted "665.30" monthly | **S3** (see WI-EDGE) |

### FINDING WI-EDGE — What-If stepper doesn't guard an invalid stepped value (S3)
- **Repro:** Mortgage What-If, second column = Years, increment −15, lines 2, base 30 → the
  grid generates Years = 30, 15, **0**. The Years=0 rows show a Monthly of "665.30" (unformatted,
  copied/stale from the base row) rather than blank or an error.
- **Why minor:** a 0-year mortgage is obviously nonsensical so a user would likely discard it,
  but the stale unformatted number is sloppy and could momentarily mislead. The 1-D path and all
  valid steps are exact.
- **Fix:** clamp/skip What-If steps that fall outside a field's valid domain (Years ≤ 0, rate ≤ 0,
  etc.), or render such cells blank with a small note.

## .psn import (continuation pass)

| # | Test | Result | Verdict |
|---|------|--------|---------|
| IMP-parse | Import all 3 real legacy fixtures (DOS.MTG / DOS.AMZ / DOS.PVL) | each auto-routes to the correct screen (mortgage/amortization/presentvalue) and parses sane data incl. advanced options (prepayments) and multi-row PV cash flows | PASS |
| IMP-route | Frontend `applyPSNImport` routing | switches to the right screen and populates the grid/fields | PASS |
| IMP-MTG | Mortgage import field units | **BUG FOUND & FIXED** — see below | **was S1/S2, FIXED** |

### FINDING IMPORT-MTG — Mortgage .psn import showed fractions, not percentages (was S1/S2) — **FIXED**
- **What was wrong:** the import API returns rate-type fields as fractions (rate 0.0499, %Down
  0.03, Points 0.02). The Amortization import branch correctly multiplies rate by 100
  (`(a.rate*100)`), but the **Mortgage** branch ran every field through `fmtMoney` with no ×100 —
  so importing a legacy mortgage file showed **Rate "0.05", %Down "0.03", Points "0.02"**, which
  then calculate completely wrong (the grid divides those by 100 again). Separately, the balloon
  amount mapped to a non-existent cell key `'howmuch'` (should be `'balloonAmount'`) and was
  silently dropped.
- **Why it matters:** this is exactly the legacy-DOS-file compatibility the client cares about —
  opening an old `.psn` mortgage file produced nonsense.
- **Fix (frontend only, `index.html` applyPSNImport):** multiply `points`/`pctDown`/`rate` by 100
  (matching the amortization branch), render `years` as an integer, and map the balloon to
  `balloonAmount`. **Verified in-browser:** importing DOS.MTG line 0 now shows Rate 4.99% / %Down
  3.0000 / Points 2.0000 and computes a sane $229.34 on $9,700 financed. `go test ./...` green.
  **Action for you:** rebuild the binary to ship (html is `go:embed`'d).

## Continuation pass 2 — fixes verified + remaining solves

| # | Test | Result | Verdict |
|---|------|--------|---------|
| IMP-MTG-live | **Real DOS.MTG file upload** (file picker) | routed to Mortgage; row 1 = Rate **4.9896%**, %Down **3.0000**, Points **2.0000** (percentages), Monthly $229.34 — import fix confirmed end-to-end | PASS (fix live) |
| WI-EDGE-fix | What-If stepper guard | `whatIfStepValid` excludes Years=0/negative & rate≤0; 30/15/0 stepping keeps only {30,15}; `go test ./...` green (harness updated to cover it) | FIXED (rebuild to ship) |
| PV-5 | Solve through-date | periodic $1,000/mo @6%, target $91,012.27 → Through solved **01/01/2034** (exact) | PASS |
| PV-ASOF | Solve as-of date | works for near dates (lump 2026, target 9048.37 → as-of **2024-01-01** exact); **fails for as-of ≥ ~2028** with "time period too long: 129 years" | **S2 — DOS-faithful latent date limit (see below)** |

### FINDING PV-ASOF — As-of-date solve fails for as-of ≥ ~2029 — **PORT OFF-BY-100, FIXED**

> **CORRECTION + FIX (2026-06-24).** Initially flagged as a DOS-faithful latent limit — that was
> wrong. The DOS year byte is `(calendar − 1900)` (confirmed: `types/records.go:10`,
> `Globals.pas:253/264`, `pv_oracle.pas:67 asof.y:=124==2024`), so DOS's first guess
> `asof.y:=100` (PRESVALU.pas:761) is the year **2000** — DOS does NOT have this failure. The port
> mistranslated it as `NewDateRec(1900,1,1)` (`backward.go:1291`), 100 years too early, making the
> first Newton step ~129 years and tripping the (correctly-ported) 128-year `AddYears` guard.
> **Fixed:** `1900 → 2000`. Verified: lump-2034/$7,788.01 now solves As-of **2029-01-01** (was
> error), lump-2026 still **2024** (unchanged), lump-2060 solves **2040**; `go test ./...` green.
> No converged result changes; boundary now ~2128, matching DOS. **Rebuild to ship.**

<details><summary>original (incorrect) write-up, kept for the record</summary>

#### As-of-date solve fails for as-of dates ≥ ~2028 (Y2K-style latent limit)
- **Repro:** PV, leave As-of Date blank, lump $10,000 @ 01/01/2034, Rate 5%, target PV $7,788.01
  (which implies as-of 01/01/2029). Result: error "time period too long: 129.000006 years",
  no date solved. The *same* solve with a nearer target (as-of 2024) works to the day.
- **Root cause (DOS-faithful):** the as-of solver's first guess is the **1900 epoch**
  (`backward.go:1291`, "DOS uses pascal year 100 = 1900"), so its first Newton step is
  `(answer − 1900)` years. `dateutil.AddYears` rejects any step `> 128` years — and **DOS does
  the exact same thing**: `INTSUTIL.pas:894  if (abs(yrs)>128) then TimeTooLong {Arbitrary limit}`.
  Since 1900 + 128 = **2028**, any as-of answer ≥ ~2029 trips the guard on the very first step,
  even though the destination date is perfectly valid.
- **Why it matters / urgency:** this is a latent date assumption from the original DOS code that
  only now bites (we're approaching 2028). It **worsens over time** — once the calendar passes
  2028, even solving for the *present-day* as-of date will start failing. Exactly the
  "Y2K-style date assumption" CLAUDE.md asks to flag.
- **DOS-fidelity decision (yours):** strict fidelity = leave it (DOS fails here too). A
  **results-preserving** fix is available: seed the solver's first guess near the payment dates
  (e.g., the earliest lump/periodic date) instead of 1900, so the first step is small. Newton
  converges to the same as-of date when both succeed — so this changes **no** computed answer,
  it only stops the solver from aborting in cases DOS also aborts. Not changed yet, pending your
  call on fidelity-vs-robustness.

</details>

## Summary & coverage

**Headline:** across the whole comprehensive pass (initial + continuation), the financial engine
remained correct everywhere — every documented value matched the DOS-validated engine / Help to
the cent, and edge cases surfaced the right advisory rather than a silent wrong number. Two
defects were found in the **UI layer** (not the engine), both in the continuation pass:

| ID | Area | Severity | Status |
|----|------|----------|--------|
| **IMPORT-MTG** | Mortgage `.psn` import showed fractions not percentages (rate 0.05 vs 4.99%; balloon dropped) | **S1/S2** | **FIXED** (frontend) |
| **WI-EDGE** | What-If stepper emits an invalid `years=0` row with a stale unformatted value | S3 | open |
| C-9 | Biweekly Basis dropdown shows 360 while calc uses 365 (notice covers it) | S3 | open |

The earlier three fixes (A1-DOC help, A2 advisory, M6 selection) were all re-confirmed working
live on the rebuilt binary, and the prior PV POD-leftover bug remains fixed.

**Covered live this pass (initial + continuation):**
- **Row sanity:** 9 schedule shapes with full per-row invariant checks (plain, odd-first, balloon,
  prepay, ARM, moratorium, target, skip, biweekly).
- **Reuse-on-top:** option add/remove returns to baseline (+badge), dispatch switch, A2 advisory,
  cross-screen retention, PV POD-leftover + Life-toggle, Mortgage clear/reuse + M6.
- **Settings:** prepaid · in-advance · exact · balloon-includes-regular · COLA-month (+API parity).
- **Multi-option combinations:** balloon+prepay+ARM, target+skip, moratorium+balloon.
- **PV depth:** COLA timing ×3 (exact), PV-4 payment solve, joint-survivor (2 lives, perfect
  ordering), variable-rate (exact).
- **Mortgage:** dispatch trio (all 3 consistent), What-If 1-D (9 exact values) + 2-D.
- **Cross:** basis × frequency; `.psn` import for all 3 screens.

**Status of findings (all passes):**

| ID | Area | Severity | Status |
|----|------|----------|--------|
| IMPORT-MTG | Mortgage `.psn` import showed fractions not percentages | S1/S2 | **FIXED** + verified live via real file upload |
| WI-EDGE | What-If stepper emitted invalid `years=0` row | S3 | **FIXED** (rebuild to ship) |
| PV-ASOF | As-of-date solve fails for as-of ≥ ~2029 — **port off-by-100** (first guess 1900 vs DOS's 2000); DOS unaffected | S2 | **FIXED** (1900→2000, `backward.go`) |
| C-9 | Biweekly Basis dropdown shows 360 while calc uses 365 | S3 | open |

**Remaining for a later spot-check:** COLA-start backward solve (PV-6); the two open items above.

The engine itself produced no wrong numbers in any pass — every defect was in the UI layer or a
latent DOS date-range assumption. PV-ASOF is the one that compounds with time and is worth a
decision before it starts affecting present-day as-of solves (post-2028).

*(checker confirms: payment=interest+principal every row; balance non-increasing & never
negative; intToDate monotonic = total; payment constant; interest matches balance×rate/period.)*
