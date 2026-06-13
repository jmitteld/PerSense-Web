# Front-End QA Report — Per%Sense Web

**Date:** 2026-06-13
**Scope:** All four screens (Welcome, Mortgage, Amortization, Present Value) plus the
Settings modal, the single-page `cmd/persense/static/index.html`.
**Method:** Static read of the full file + a deep static audit of the ~4,000 lines of
JavaScript, then live verification in Chrome against the running server
(`localhost:8080`), exercising calculations through both the real UI buttons and the
backend API, and comparing rendered values against the engine's authoritative output.

The Go backend is well covered by tests; this pass targeted the **JavaScript
translation layer** — how inputs are collected, sent, and how results are rendered.

---

## Summary

The calculation paths are sound. Spot-checks matched expected figures exactly:
a 30-yr $100k loan at 6% computes a $599.55 payment; a $200k house at 20% down,
30 yr, 6% returns $160,000 financed, $959.28 P&I, 6.0000% APR, $40,000 cash. No
UI freezes or broken handlers were found, and every `onclick`/`onchange` resolves.

Seven issues were found in the rendering/clearing/import layer and **all seven were
fixed and verified**. None of them affected the engine's numbers — they affected what
the user *sees* or what gets *left behind* between calculations.

> **Action required:** the server serves the static files via `go:embed`, so these
> source edits go live only after a rebuild/restart (`go run ./cmd/persense` or rebuild
> the binary). All fixes were verified by injecting the corrected logic into the running
> page and by a clean `node --check` of the edited file.

---

## Bugs found and fixed

### 1. Amortization Balance column drifted from the engine (per-payment view)
**Severity: BUG** · `renderAmzSchedule`, per-payment branch.
The Balance column was re-derived client-side with a running subtraction
(`balance = lastBalance - (payment - interest)`) instead of using the engine's
authoritative post-payment balance (returned in the schedule's `principal` field).
This accumulates rounding drift (measured up to ~$0.28 over a schedule with a
prepayment series) and gets balloons / rate adjustments wrong. Notably, the
**payoff lookup tool and the quarterly/yearly views on the same screen already used
the engine balance** — so the per-payment table could disagree with the payoff box
just above it.
**Fix:** Balance now reads the engine's balance directly; the Principal column is
derived as the balance delta, so all three columns reconcile exactly.
**Verified:** displayed balance == engine balance for every row (max diff $0.00),
final balance $0.00.

### 2. CSV export had the same wrong-balance walk
**Severity: BUG** · `exportAmzCSV`.
Identical running-subtraction logic, so an exported schedule with any advanced option
disagreed with the on-screen payoff tool.
**Fix:** export now uses the engine balance and balance-delta principal, matching the
on-screen schedule.

### 3. Quarterly/Yearly summaries bucketed by the wrong period in non-UTC timezones
**Severity: BUG** · `renderAmzSchedule`, aggregated branch.
Dates were parsed with `new Date("YYYY-MM-DD")` (UTC midnight) but read back with
`getMonth()`/`getFullYear()` (local time). For any user west of UTC this shifts
period-boundary payments into the previous bucket. **Confirmed live in
America/New_York:** a Jan 1 2024 payment was labelled "Q4 2023" / year "2023".
**Fix:** dates are now parsed from the ISO string parts directly (no `Date` object),
so no timezone shift.
**Verified:** Jan 1 2024 → "Q1 2024" / "2024"; Apr 1 → "Q2 2024".

### 4. "Clear All" on Present Value broke reused periodic rows
**Severity: BUG (usability)** · `clearPV`.
Clear blanked every periodic field including Pmts/Yr and COLA, but a freshly added
row defaults those to 12 and 0. After a Clear, reusing an existing periodic row
immediately failed the "fill in Pmts/Yr" validation with no obvious cause.
**Fix:** Clear now restores Pmts/Yr=12 and COLA=0 (matching new-row defaults).
**Verified:** after the new clear, Pmts/Yr="12", COLA="0".

### 5. Legacy `.psn` import scrambled prepayment columns
**Severity: BUG** · `importPSN` → prepayment row mapping.
The prepay row's column order is startDate, # Pmts, Stop Date, Per/Yr, Amount, but the
importer wrote startDate, **Stop Date, Per/Yr, # Pmts**, Amount — misplacing three
columns. An imported prepayment got the wrong dates and frequency.
**Fix:** mapping now matches the actual column order.
**Verified:** each value lands in its correct field.

### 6. "Clear Row" (Mortgage) left the red error outline behind
**Severity: MINOR (usability)** · `clearMortgageRow`.
Clearing a row that had a calculation error wiped the cells and the message but left
the red row marker, implying a problem on an empty row.
**Fix:** Clear Row now also clears field-error marks.
**Verified live:** the red mark persisted before the fix; logic confirmed.

### 7. "Load Examples" silently combined examples with leftover state
**Severity: USABILITY** · `loadExamples`.
It reset the mortgage grid but left any prior Amortization advanced options
(prepay/balloon/adjust/moratorium/target/skip + stale schedule) and Present Value
state (POD, actuarial tables, variable-rate schedule, contingency dropdowns, extra
rows) in place — so the example numbers were quietly mixed with leftover modifiers.
**Fix:** Load Examples now clears Amortization advanced options and the prior schedule,
and calls `clearPV()` (no confirmation prompt) before populating.

---

### 8. Auto-calculate could overwrite freshly-typed values with a stale result
**Severity: BUG (concurrency)** · `calcMortgageRow` / `calcAmortization` / `calcPV`,
auto-calc path.
With Auto-calculate ON, each field exit fires a silent recalculation. An
underdetermined row (e.g. price + years + rate, no down payment) is answered by
the engine with an all-zeros echo. If the user kept typing while that request was
in flight, the response was applied unconditionally — and `updateMtgRowUI` writes a
returned `0` into a cell now marked as user input (the zero-skip guard only
protected non-input cells). **Deterministically reproduced:** a stale all-zeros
response turned a freshly-typed Years "30" into "0" and Rate "6" into "0.0000".
On a deployed server (100–300 ms latency) this race is easy to hit while tabbing
between fields.
**Fix:** added a monotonic `calcGeneration` counter bumped on every keystroke; each
auto-calc captures it before its request and discards the result if it changed
before the response arrived (a fresh recalc for the new input is already queued).
The guard is scoped to silent auto-calc, so the explicit Calculate button is
unaffected.
**Verified:** the shipped `calcMortgageRow` run under Node drops the result when an
edit lands mid-flight (no write, returns false) and applies it normally otherwise.

---

## Auto-calculate behavior (tested with and without)

- **OFF (default):** fully inert. Filling an entire valid row produces no output,
  no errors, and no field changes until Calculate is pressed (then it computes
  correctly). Confirmed live.
- **ON, incomplete input:** silent — no premature/partial results, no error text,
  no red marks. The worksheet is left untouched until there is enough data.
- **ON, complete input:** computes the correct result on field exit; changing an
  input clears the stale outputs and recomputes.
- **ON, stale result on screen but current input insufficient:** shows the quiet
  "showing a previous result" hint rather than a wrong number.
- **ON, race while typing:** fixed by item 8 above — a superseded in-flight result
  is dropped instead of clobbering newer input.

---

## Regression tests added

`cmd/persense/frontend_render_test.go` — in the same style as
`frontend_dob_year_test.go` (extracts the shipped JS and runs it under Node; skips
when Node is absent). Run with `go test ./cmd/persense/`:

- `TestAmzScheduleRenderJS` — Balance column equals the engine balance and Principal
  is the balance delta; quarterly/yearly buckets use the date's own period (run
  under `TZ=America/New_York` to catch the timezone regression).
- `TestAmzCSVExportJS` — exported Balance column uses the engine balance.
- `TestClearPVDefaultsJS` — clearPV restores Pmts/Yr=12 and COLA=0, blanks the rest.
- `TestPSNPrepayColumnOrder` — the markup column order and the import mapping agree.
- `TestAutoCalcStaleGuardJS` / `TestAutoCalcStaleGuardAmzJS` /
  `TestAutoCalcStaleGuardPVJS` — for all three worksheets, a silent auto-calc drops a
  result superseded by a mid-flight edit, and applies it when inputs are unchanged.

---

## Minor / consistency change

- **Amortization APR formatting** now appends `%` to match the Mortgage screen's APR
  cell (was the bare number). Display-only; the field is read-only and never parsed back.

---

## Notable behavior that is correct (not a bug)

- **Mortgage ignores the Settings "Basis"** for its APR — by design (the Mortgage
  screen is always 360-day). The Settings tooltip already says the computational
  settings apply to Amortization and Present Value. Worth a one-line note in the
  Mortgage Basis tooltip if users expect otherwise.
- **Date-of-birth two-digit-year expansion** pivots on the current year (birth dates
  are never in the future) — intended, and the only field allowed a 2-digit year.
- The **leftover-state hazards** previously noted for Amortization and Present Value
  (advanced options / COLA / POD being summed silently) remain mitigated by the
  on-screen badges and the "included in total" summary line; Clear All is the reliable
  reset, and items 4 and 7 above remove two remaining sharp edges.

---

## Files changed

- `cmd/persense/static/index.html` — items 1–8, the APR formatting tweak, and the
  Mortgage Basis tooltip note. Rebuild/restart the server to pick up the changes.
- `cmd/persense/frontend_render_test.go` — new regression tests (see above).
