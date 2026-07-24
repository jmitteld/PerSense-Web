# Investment screen — Plan B probe pack (run in the DOS app)

**Why:** the Investment screen's compute unit is absent from every source copy in
this repo (verified 2026-07-22: no `INVEST*.pas` in dos_source / win_source /
src_documented / oracle units; no unit registers `EnterProc[iINV]`; the Delphi
.dpr compiles no investment unit). The port therefore reconstructs the engine
from the running DOS app. The manual scans already pin the core semantics —
chapter 7 p.91: the screen **equates the value of Funds IN (deposits) with Funds
OUT (withdrawals)**, solving for any single blank dollar amount or the interest
rate (IRR), optionally in inflation-adjusted constant dollars; and ch.1 p.3:
results are **identical to the Present Value screen's** — so the engine is the
already-ported PV machinery with a two-sided balance equation. These probes pin
down the remaining conventions (rate convention, inflation compounding, basis
handling, dispatch/refusals).

**BIGGEST-VALUE ITEM — before any probes:** scan/photograph **manual pages
92–105** (the rest of chapter 7: Financial Planning examples p.93, Investment
Analysis examples p.95, Lease or Purchase p.100). Worked examples with printed
numbers are golden tests for free and may make several probes unnecessary.

**How to run:** DOSBox, Alt-I to the Investment screen. **Restart the app (or
File→New) before every probe** — we have confirmed the DOS app can carry stale
session state into solves (discrepancies.md §33). Default Computational
Settings (360 basis) unless a probe says otherwise. Dates below are M/D/YYYY;
enter years per the app's century setting. For every probe, photograph the full
screen after pressing Enter — the picture is the record; no transcription
needed.

## D — discovery probes (behavior, not numbers)

- **D1** Empty screen, press Enter. Then only a rate (6), Enter. Record every
  message. (What does it consider under-determined?)
- **D2** One deposit row: Date 1/1/2020, Amount 10000 — cursor into the
  `Dollars` cell: what does it display, and which key cycles it (space?)?
  Record the exact captions of its states (we expect Current/Constant forms).
- **D3** Press Alt-F1 (Examples) on this screen. Photograph the example LIST,
  then run each example and photograph its result screen. (These are built-in
  worked cases — more free goldens.)
- **D4** Enter the Q1 worksheet but leave TWO amounts blank → message? Then no
  blanks at all (all cells filled consistently, then inconsistently — e.g.
  withdrawal 99,999) → does it refuse, recompute something, or accept?
- **D5** Photograph the Settings line at the bottom of this screen after
  changing Basis to 365 and back (which Computational Settings does this screen
  honor?). Also: does Ctrl-T produce a table on this screen? Photograph it.

## Q — quantitative probes (each discriminates hypotheses)

Rate 6, Inflation blank, Constant date blank, 360 basis unless noted.
H1 = rate cell holds a TRUE (continuously-compounded) rate; H2 = a LOAN rate
(monthly compounding). The recorded value tells us which (or neither).

| # | Worksheet | Blank (solve) | H1 predicts | H2 predicts |
|---|---|---|---|---|
| Q1 | Deposit lump 10,000 @ 1/1/2020 | Withdrawal lump amount @ 1/1/2030 | 18,221.19 | 18,193.97 |
| Q2 | Withdrawal lump 20,000 @ 1/1/2030 | Deposit lump amount @ 1/1/2020 | 10,976.23 | 10,992.65 |
| Q3 | Deposit 10,000 @ 1/1/2020; Withdrawal 20,000 @ 1/1/2030 | Interest rate (IRR) | 6.9315 | 6.9515 |
| Q4 | Periodic deposit 100/mo From 1/1/2020 Through 12/1/2029 (PrYr 12) | Withdrawal lump @ 1/1/2030 | 16,483.52 | 16,469.87 |
| Q5 | Deposit lump 200,000 @ 1/1/2030; Periodic withdrawal From 2/1/2030 Through 1/1/2050 PrYr 12, amount blank | Periodic withdrawal amount | 1,434.60 | 1,432.86 |
| Q6 | As Q5 but withdrawal amount 1,000 with `Dollars` = **Constant**, Inflation 3, Constant date 1/1/2030, Through 1/1/2040; deposit lump @ 1/1/2030 blank | Deposit amount | 103,324.57 (yield-conv. infl) / 103,543.17 (raw-cont. infl) | — |
| Q7 | Q6 with Constant date moved to 1/1/2035 (else identical) | Deposit amount | records whether the anchor shifts amounts (predict: ×exp(−infl·5) scaling ≈ 89,036 under yield-conv.) | — |
| Q8 | Q1 with Basis 365 in Settings | Withdrawal amount | 18,220.07 (365.25 day-count on clean dates ≈ H1) | — |
| Q9 | Q1 but withdrawal date 1/1/2015 (BEFORE the deposit) | Withdrawal amount | 7,408.18 (H1 discount back 5y) | 7,413.72 |
| Q10 | Q1 exactly, but run WITHOUT restarting after Q9 | Withdrawal amount | should equal Q1 — if not, the stale-state bug affects this screen too |

Also record for Q1: the exact decimals displayed (2? 4?), and whether the
solved cell renders highlighted/soft (computed) like other screens.

## What happens next

Manual pages + probe photos come back → the Go engine is implemented over the
existing PV primitives, fit to reproduce every probe and manual example to the
cent, each becoming a pinned golden test (`internal/finance/investment/`).
Conventions the probes leave ambiguous ship as documented assumptions in
docs/discrepancies.md until contradicted.
