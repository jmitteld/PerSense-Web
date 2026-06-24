# Fresh UI→Engine Test Pass — Working Findings Log

Tester: Claude (expert-tester mode). Date: 2026-06-24.
Method: live UI driven in Chrome against local `persense` server (localhost:8080),
cross-checked against the authoritative help-doc expected values and, where the
CLI semantics match, the rebuilt DOS source-oracle (amort/pv/mtg).

Severity key: **S1** broken/wrong number a client would catch · **S2** misleading
UX / wrong-looking behavior · **S3** polish / cosmetic · **PASS** verified correct.

---

## Mortgage screen

| # | Case | Expected (help) | Live UI | Verdict |
|---|------|-----------------|---------|---------|
| M1 | Ex1 forward: 200k,20%dn,20y,8%,2pt,T&I 200 | Cash 43,200 · Fin 160,000 · Monthly 1,538.30 · APR ~8.27 | Cash 43,200.00 · Fin 160,000.00 · Monthly 1,538.30 · APR 8.2730% | PASS |
| M2 | Ex2 reverse solve (Price blank, Monthly 1650) | Price 241,749.12 · %Dn 21.9944 · Fin 188,577.78 | Price 241,749.12 · %Dn 21.9944 · Fin 188,577.78 · APR 8.6648% | PASS |
| M3 | Ex3 balloon solve (Bal Yrs 8, Bal Amt blank) | Cash 61,600 · Fin 224,000 · Balloon 98,372.47 | Cash 61,600.00 · Fin 224,000.00 · Balloon 98,372.47 · APR 8.5321% | PASS |
| M4 | Ex4 harden computed Monthly + 15y balloon | Monthly 1,777.79 (hardened) · Balloon 184,912.27 | Harden→white input OK · Balloon 184,912.27 | PASS |

| M5 | Over-determined row (Price + Monthly, no balloon) | Clear actionable error | Clear red error naming both conflicting fields + fix steps | PASS (S3: base msg + appended hint partly duplicate wording) |
| M6 | Compare APR auto-picks first 2 rows w/ APR (a 20y vs a 30y loan) | should compare comparable loans / warn | Compared without warning; summary grammar "1 years, 1 months" | **S2** (guard) + **S3** (grammar) |

### FINDING M6 — Compare APR: silently compares non-comparable rows; pluralization bug
- **What the user sees:** `compareMtgAPR` selects the **first two rows that have any
  computed APR** (index.html ~2569-2581) and compares them with no check that their loan
  terms match — even though the help states "Years must be the same on both rows." With a
  20-year row and a 30-year row it produced: "APRs cross at 9.9598% for duration
  **1 years, 1 months**. If held longer than 1 years, 1 months, Mortgage A is better."
- **Verified facts (raw `/api/mortgage/compare`):** `crossoverYears = +1.0833` (positive —
  the UI's "~1.08 years" is correct; there is NO sign bug, ruled out by direct API check).
  The substantive issues are: (a) no comparability/same-term guard and no indication of
  *which* two rows were auto-selected, so a user can get a confident recommendation from an
  apples-to-oranges pair; (b) **grammar**: the summary always renders "%d years, %d months"
  with no singular form — `mortgage.go:553-560` — so "1 years, 1 months" instead of
  "1 year, 1 month".
- **Client risk:** medium. Repro: have ≥2 calculated rows (any terms) → click Compare APR.
- **Suggested fix:** warn when the two selected rows differ in Years (or let the user pick
  the pair), and singularize the duration string.

---

## Amortization screen

| # | Case | Expected (DOS authority) | Live UI | Verdict |
|---|------|--------------------------|---------|---------|
| A1 | Ex1: 100k,8%,360,12, loan 02/12/24, 1st 03/01/24 (odd short 1st period) | Pmt **731.98** · TotInt **163,513.81** · Pmt#1 int 422.22 | Pmt 731.98 · TotInt 163,513.84 · Pmt#1 int 422.22 · LastPmt solved 02/01/2054 | ENGINE PASS — but help mismatch (see A1-DOC) |

### FINDING A1-DOC — In-app Help Example 1 shows pre-fix Windows numbers (S2)
- **What the user sees:** Help → Amortization → Example 1 states "Payment = **$733.76**"
  and "Total interest … is **$161,499.77**". The app actually computes **$731.98** and
  **$163,513.84** for those exact inputs.
- **Root cause:** documented in `docs/discrepancies.md §7`. The odd 19-day first period
  makes DOS *augment* the payment; the Windows help shows the un-adjusted plain payment.
  The Go engine correctly follows DOS (validated: oracle gives 731.98 / 163,513.81), but
  **`help.html` Example 1 was never updated** — it still teaches the old Windows values and
  even reasons about "$666.67 a standard month."
- **Why it matters for the meeting:** the engine is RIGHT. The risk is purely optics — a
  client who opens in-app Help, follows Example 1, and gets $731.98 instead of the
  documented $733.76 will think the app is wrong. This is the most likely "gotcha."
- **Fix:** update help.html AM Example 1/1b to the DOS values (731.98 / 163,513.84), or add
  a one-line note that an odd first period adjusts the payment. (Also: DOS 163,513.81 vs Go
  163,513.84 = known 3¢ rounding, §7 — harmless, but tidy if updating.)

| A1d–f | Field-presence solves: payment / amount / rate | 1,498.88 / 250,000.61 / 6.0000% | 1,498.88 / **250,000.61** / **6.0000%** (all green outputs) | PASS |
| A2 | Balloon added while **rate is in solve mode** (rate left blank) | balloon should reduce interest / flag change | Engine **silently re-solved rate 6.0000%→7.0069%**, total interest +$50k (289,597→**339,597**), no warning | **S2** |
| A3 | Balloon on a fully-specified loan (rate given, payment blank) | balloon reduces interest, payment re-solved down | Pmt re-solved 1,498.88→1,334.11, interest **280,280** (↓), balance drops $50k at 2034-01-01, badge "1 active" | PASS |

### FINDING A2 — Adding an Advanced Option silently re-solves a blank/solved core field (S2)
- **Repro (exactly what happened live):** Solve a loan's rate (leave Rate blank → it
  computes 6.0000% as a green output, the Ex1f flow). Then open Advanced Options and add a
  $50,000 balloon. Click Calculate.
- **What the user sees:** Total Interest jumps from $289,596.93 to **$339,596.94** (i.e.
  *up* by the balloon amount) and the Rate field silently changes **6.0000% → 7.0069%**.
  To a user this reads as "I added a $50k payment and my interest went UP $50k and my rate
  became 7%."
- **Why:** field-presence dispatch still treats Rate as the unknown, so it solves the rate
  that makes the fixed $1,498.88 payment amortize over 360 periods *with* the extra balloon —
  mathematically self-consistent (verified via `/api/amortization/calc`: solved rate
  0.0700686, 360 rows) but deeply counterintuitive. **No warning** that the rate was
  re-solved or that the balloon increased interest.
- **Contrast (A3):** with the rate *specified* (or the payment left blank), the same balloon
  behaves intuitively — interest drops, loan pays off early. So balloons are NOT broken; the
  problem is the silent re-solve of a stale/blank field when an option is added.
- **Client risk:** high (looks like a wrong/irrational result). **Fix:** when an Advanced
  Option is present and a core field is in solve mode, surface a notice ("Rate re-solved to
  7.0069% to fit the balloon"), and/or add a result advisory when a balloon/prepayment
  *increases* total interest. The red "1 active" badge helps but doesn't flag the re-solve.

---

## Present Value screen

| # | Case | Expected (help) | Live UI | Verdict |
|---|------|-----------------|---------|---------|
| P1 | Ex1 lump 10k @ 8% **True Rate**, 1yr | PV 9,231.16 (=10k·e^−0.08) | PV **9,231.16** (oracle 9231.163464) | PASS |
| P2 | Same, Rate Type = **Loan Rate** | PV 9,233.61 | PV **9,233.61** | PASS |
| P3 | IRR backward: blank Rate, target PV 9,231.16 | rate → 8.0000% | Rate solved **8.0000%** | PASS |
| P4 | "Included in total: N lump · M periodic" transparency line | present | shown + per-row Value column | PASS |

---

## Actuarial / Life Contingency (lives inside PV)

| # | Case (65M DOB 01/01/1959, $2k/mo→01/01/2059, 5%, SSA Male) | Expected (help) | Live UI | Verdict |
|---|------|-----------------|---------|---------|
| AC1 | Life = **Living** | ≈ 253,135 | **253,135.24** | PASS |
| AC2 | Life = **None** (non-contingent) | ≈ 397,763 | **397,762.85** | PASS |
| AC3 | Life = **Dead** | ≈ 144,628 | **144,627.62** | PASS |
| AC4 | Complementarity: Living + Dead = None | exact | 253,135.24 + 144,627.62 = 397,762.86 ≈ 397,762.85 (1¢) | PASS |

(The actuarial DOS oracle is blocked per repo docs, so verification is vs the documented
help values plus the internal Living+Dead=None identity, which holds to the penny.)

---

## Cross-cutting UI / UX observations

- **C1 (S3, testing note):** The *Clear All* native `confirm()` dialog blocks the page so
  hard that an automated session can't dismiss it (had to reload). Harmless for a human
  clicking OK, but worth knowing. Consider a non-blocking in-page confirm.
- **C2 (good):** Leftover-state guards are largely in place — the Amortization Advanced-Options
  "N active" red badge, the PV "Included in total: N lump · M periodic" line, and the
  "use Clear All before an unrelated example" hint all reduce the silent-carryover risk that
  finding A2 exploits. A2 remains because the badge doesn't flag a *re-solved* core field.
- **C3 (good):** Auto-calc stale-result guard works — editing inputs shows "Showing a previous
  result — the inputs changed but couldn't be auto-calculated. Press Calculate," rather than a
  stale number masquerading as current.
- **C4 (good):** Computed vs input cells are clearly distinguished (green italic + accent
  border vs white), and the harden interaction (double-click) behaves as documented.

## Note on oracle rate convention
`mtg_oracle monthly` takes a true/yield rate and converts internally; the GUI
"Loan Rate" is the nominal note rate. Do NOT compare the helper's raw output to
GUI output without matching the convention (avoided a false S1 here).
