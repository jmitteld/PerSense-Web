# Full UI-vs-Oracle differential run — all three sections — 2026-07-24

**What this is.** The manual test plan's scenarios driven through the **real web
UI in a headless browser** (Playwright → the actual DOM → JS → API → engine, the
same path a user hits) and compared against the **genuine DOS engines** built
headless (`amort_oracle`, `pv_oracle`, `mtg_oracle` — the third built for the
first time this session). This exercises the layers the engine unit tests
can't: request building, rate-convention conversion, settings mapping, row
collection, solved-cell echo, and recalc/stale behavior.

## Results

| Section | Cases | Result |
|---|---|---|
| **Amortization** (re-run on current build) | 29 | 21/21 core **pass**, 8 flagged (known adjudications, no regressions) |
| **Present Value** (new) | 26 | **26/26 pass** (after fixing 2 defects found by this run — see below) |
| **Mortgage** (new) | 12 | **12/12 pass**, no defects |

Every case also includes an untouched-recalc idempotency check — the stale-state
class that produced this week's §34/§35 findings.

### Present Value coverage (26)
Forward: lump (after/on/before as-of), monthly/quarterly/semi-annual/annual
frequencies, mixed 4-row worksheet, negative amounts, 365 basis (lump +
periodic), 30-year precision, zero rate, periodic straddling the as-of.
COLA: anniversary, continuous, month-specific (January).
Variable-rate: multi-step schedule on a lump and on a periodic.
Backward (all seven solver paths): lump Amount, lump Date, periodic Amount,
periodic Through-date, periodic From-date, Rate (IRR), As-of Date — each solved
value matches the DOS oracle to the cent / exact day.

### Mortgage coverage (12)
Monthly solve (basic, points+tax, known balloon, zero-down), price solve (%
Down funding, tax netted, cash+points affordability), funding derivations
(cash→pct, financed→pct), balloon-Amount solve, full-term APR with and without
points (vs the real DOS ReportAPR). The mortgage grid's input/output status
machinery proved stale-proof by construction — strict idempotency on all 12.

## Defects found and fixed by this run (discrepancies §38)

1. **PV date solves showed a $0.00 total (engine).** Solving a lump Date or a
   periodic Through/From date from a row-level target left the screen Present
   Value at $0.00 — and fired a bogus "payments net to about zero" advisory —
   even though the rows sum to the target. DOS always runs FrontwardCalc after
   BackwardCalc (PRESVALU.pas:1253), re-summing the screen; the port skipped
   that completion for row-level targets. Fixed in `BackwardCalc`; regression
   test `TestBackwardRowTargetSumValue`.

2. **PV recalc drift from display-precision re-send (frontend).** A solved
   amount echoed as "$974.50" was re-parsed from the display text on the next
   calc, drifting the total (33,000.00 → 33,000.14); a solved rate at 4-dp
   display precision drifted 44,000.00 → 43,999.95. DOS keeps solved cells as
   full reals. The cells now stash the engine's full-precision value
   (`dataset.pvRaw`) and the collector prefers it while the cell is green;
   the hidden rate canonical stores full precision.

## Notes
- Solved-date recalcs legitimately land near (not on) the target — the solved
  date is day/period-granular; DOS behaves identically on re-Enter. The
  harness asserts date stability instead of total equality there.
- The app's localStorage worksheet-restore silently re-sums leftover rows on
  reload — exactly the behavior the manual plan's "Clear All before every
  case" instruction guards testers against. The harnesses clear storage
  before each case.
- Harness files: `harness/run_amz.js`, `harness/run_pv.js`, `harness/run_mtg.js`
  (+ results JSONs). Server on :8099; oracles in /tmp/oraclebuild.

## Bottom line
Across 67 UI-driven differential cases, the only divergences found were the two
PV display/echo defects above — both fixed and re-verified — plus the
previously-adjudicated ARM one-month split (where the web matches the shipped
DOS app and the printed manual, and the headless oracle is the outlier). The
financial numbers produced through the real UI match the genuine DOS engines
to the cent in every comparable case.
