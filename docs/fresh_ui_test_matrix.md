# Per%Sense — Fresh UI-to-Engine Test Matrix

Companion to `fresh_ui_test_report.md`. Each case was executed **live in the browser**
against `localhost:8080`. "Oracle" = rebuilt DOS source engine; "Help" = in-product
worked-example. Verdicts: **PASS** verified to the cent · **S2/S3** see report.

Date: 2026-06-24.

## Coverage summary

| Area | Cases run | PASS | Findings |
|------|-----------|------|----------|
| Mortgage | 6 | 5 | M6 (S2/S3) |
| Amortization | 6 | 5 | A1-DOC (S2), A2 (S2) |
| Present Value | 4 | 4 | — |
| Actuarial (life contingency) | 4 | 4 | — |
| Cross-cutting UX | 4 | 3 | C1 (S3) |
| **Total** | **24** | **21** | **3×S2, 2×S3** |

No S1 (wrong-number) defects.

**Resolutions (2026-06-24):** A1-DOC, A2, and M6 are all fixed — help/frontend only, engine
untouched (stays DOS-validated). A2 and M6 were confirmed against the DOS source to already be
DOS-faithful in the engine (DOS solves the rate with a balloon; DOS compares different terms and
prints the same plural "X years, Y months"); the fixes add a clarifying advisory (A2) and anchor
Compare-APR to the selected row like DOS (M6). See `fresh_ui_test_report.md` for details.
Rebuild the binary to ship (html/js are `go:embed`'d).

## Mortgage

| ID | Scenario | Inputs (blank = solved) | Expected | Observed | Verdict |
|----|----------|-------------------------|----------|----------|---------|
| M1 | Forward payment (Help Ex1) | 200k, 20% dn, 20y, 8%, 2pt, T&I 200 | Cash 43,200 · Fin 160,000 · Monthly 1,538.30 | identical · APR 8.2730% | PASS |
| M2 | Reverse-solve price (Ex2) | pts 1.5, cash 56k, 30y, 8.5%, T&I 200, monthly 1,650 | Price 241,749.12 · 21.9944% dn · Fin 188,577.78 | identical · APR 8.6648% | PASS |
| M3 | Balloon-amount solve (Ex3) | 280k, 2.5pt, 20% dn, 30y, 8.25%, T&I 300, monthly 1,600, bal yr 8 | Cash 61,600 · Fin 224,000 · Balloon 98,372.47 | identical · APR 8.5321% | PASS |
| M4 | Harden + 15y balloon (Ex4) | 240k, 0% dn, 30y, 8.1% → harden monthly, yrs 15, bal yr 15 | Monthly 1,777.79 · Balloon 184,912.27 | identical | PASS |
| M5 | Over-determined row error | Price + Monthly both filled, no balloon | clear actionable error | clear red error, names conflicts + fix | PASS (S3: msg slightly duplicated) |
| M6 | Compare APR, mixed grid | auto-picks 20y row vs 30y row | comparable / warn | compares silently; "1 years, 1 months" | **S2 / S3** |

## Amortization

| ID | Scenario | Inputs | Expected (DOS) | Observed | Verdict |
|----|----------|--------|----------------|----------|---------|
| A1 | Odd short first period (Ex1) | 100k, 8%, 360, 12/yr, loan 02/12/24, 1st 03/01/24 | Pmt **731.98** · Int 163,513.81 · pmt#1 int 422.22 | 731.98 · 163,513.84 · 422.22 | PASS (engine) — see **A1-DOC** for help mismatch |
| A1d | Solve payment (Ex1d) | 250k, 6%, 360, 12/yr | 1,498.88 | 1,498.88 | PASS |
| A1e | Solve amount (Ex1e) | rate 6%, pmt 1,498.88, 360, 12/yr | 250,000.61 | 250,000.61 | PASS |
| A1f | Solve rate (Ex1f) | 250k, pmt 1,498.88, 360, 12/yr | 6.0000% | 6.0000% | PASS |
| A2 | Advanced balloon while rate in solve-mode | rate blank, pmt 1,498.88, +50k balloon | balloon ↓ interest / flag | rate silently 6→7.0069%, interest +50k, no warning | **S2** |
| A3 | Balloon on fully-specified loan | 250k, 6%, payment blank, +50k balloon | interest ↓, early payoff | pmt→1,334.11, int→280,279.64, balance −50k at date, badge "1 active" | PASS |

## Present Value

| ID | Scenario | Inputs | Expected | Observed | Verdict |
|----|----------|--------|----------|----------|---------|
| P1 | Lump PV, True Rate (Ex1) | as-of 01/01/24, 8%, 10k @ 01/01/25 | 9,231.16 | 9,231.16 (oracle 9231.163464) | PASS |
| P2 | Same, Loan Rate type | rate-type = Loan | 9,233.61 | 9,233.61 | PASS |
| P3 | IRR backward solve (Ex5b) | rate blank, target PV 9,231.16 | rate 8.0000% | 8.0000% | PASS |
| P4 | "Included in total" transparency | mixed rows | shown | shown + per-row Value | PASS |

## Actuarial / Life Contingency (inside PV)

65M, DOB 01/01/1959, $2,000/mo 01/01/2024→01/01/2059, 5%, SSA 2021 Male, ref 01/01/2024.

| ID | Life setting | Expected (Help) | Observed | Verdict |
|----|--------------|-----------------|----------|---------|
| AC1 | Living | ≈ 253,135 | 253,135.24 | PASS |
| AC2 | None (non-contingent) | ≈ 397,763 | 397,762.85 | PASS |
| AC3 | Dead | ≈ 144,628 | 144,627.62 | PASS |
| AC4 | Complementarity Living+Dead=None | exact | 397,762.86 ≈ 397,762.85 (1¢) | PASS |

## Cross-cutting UX

| ID | Check | Observed | Verdict |
|----|-------|----------|---------|
| C1 | Clear All confirmation | native confirm() hard-blocks page | S3 (nuisance) |
| C2 | Leftover-state guards | Adv-Options "N active" badge, PV "Included in total" line, Clear-All hint | PASS (but see A2) |
| C3 | Auto-calc stale guard | "Showing a previous result … press Calculate" instead of stale number | PASS |
| C4 | Computed vs input cells / harden | green italic+border vs white; double-click harden works | PASS |

## Not yet exercised (candidates for a deeper pass)

PV variable-rate schedule (Ex5f); PV COLA escalation-timing modes (Ex4b — exact values
documented: 532,551.46 / 539,754.26 / 540,423.84); joint-and-survivor (two lives); `.psn`
import round-trip; weekly/biweekly basis auto-switch (Ex3/Ex4); What-If tables (Mortgage
Ex6/Ex7); skip-months × target interaction; moratorium.
