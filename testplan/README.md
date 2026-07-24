# PerSense-Web test plan & UI-vs-oracle harnesses

Manual side-by-side test plans (web vs the legacy DOS Per%Sense) and the
automated browser harnesses that drive the REAL web UI in headless Chromium
and compare every result against the genuine DOS engines built headless.

## Contents

| Path | What it is |
|---|---|
| `amz.md` / `pv.md` / `mtg.md` | Manual test cases (100 per section) for human side-by-side runs |
| `PerSense_Manual_Test_Plan.pdf` | The three sections compiled to one PDF (`build_pdf.py` rebuilds it) |
| `amz_ui_vs_oracle_report.md` | Amortization automated run report (2026-07-23) |
| `ui_vs_oracle_full_report.md` | Full three-section run report (2026-07-24) — 67 cases, findings & fixes |
| `harness/run_amz.js` + `amz_cases.json` | Amortization UI-vs-oracle harness (29 cases) |
| `harness/run_pv.js` | Present Value harness (26 cases inline: forward, COLA, VR, all 7 backward solves) |
| `harness/run_mtg.js` | Mortgage harness (12 cases inline: funding branches, balloon, APR) |
| `harness/fuzz_amz_stale.js` | Randomized amz advanced-options fuzzer: oracle correctness + path-independence (stale detection) |
| `harness/regress_balloon_skip.js` | Regression probe for the §34 balloon-solve+skip stale bug |
| `harness/*_results.json` | Machine-readable results of the reported runs |
| `pv_table_design.md` | Design doc for the PV payment table (implemented) |
| `investments_design.md` / `investments_mockup.html` / `Investment_Probe_Pack.pdf` | Investments screen design (deferred — client says use PV for now) |

## Running the harnesses

Prereqs (the harnesses drive real binaries end to end):

1. **Build & start the web server** (any port; harnesses default to `:8099`):
   `go build -o /tmp/persense-srv ./cmd/persense && /tmp/persense-srv -port 8099`
2. **Build the DOS oracles** (FPC; the scripts fetch a local compiler, no root):
   `legacy/oracle/build_linux.sh` (amort), `TARGET=pv_oracle …`, `TARGET=mtg_oracle …`
   → binaries land in `/tmp/oraclebuild/` (the harnesses' default path).
3. **Node + Playwright** with Chromium available (`npm i -g playwright` or a
   preinstalled browser; the scripts use `require('playwright')`).

Then, from `harness/`:

```
node run_amz.js            # 29 amortization cases
node run_pv.js             # 26 present-value cases
node run_mtg.js            # 12 mortgage cases
node fuzz_amz_stale.js 150 424242   # N cases, seed
```

## Hard-won harness rules (read before writing a new one)

- **The app restores the worksheet from localStorage on load** and silently
  re-sums leftover rows — reset with `page.addInitScript(() =>
  localStorage.clear())`; clearing from the old page loses a race with the
  app's ~400ms debounced save. (Users now get a "restored" notice, but
  harnesses must still start pristine.)
- **Block external requests** (`page.route`): cdn.tailwindcss.com stalls the
  `load` event ~13s in sandboxed environments.
- `clearAmortization()`'s `confirm()` auto-dismisses under automation and
  ABORTS the clear — never rely on it; register a dialog handler and reset
  fields explicitly (or reload).
- **Oracle flag traps:** `solveballoon=` forces `plus_regular := false` after
  flag parsing; `rate`/`term`/`balloon` outputs carry a status byte where
  status≠1 means refusal (not a solved zero); the UI sends only non-default
  settings (prepaid=YES is the default and must be mirrored to the oracle).
- Date solves are day/period-granular: assert the solved DATE is stable
  across a recalc, not the total (DOS behaves the same on re-Enter).
- Engine-vs-app splits (365/360 kicker §28, ARM one-month offset) are
  documented in `docs/discrepancies.md` — check there before calling a
  divergence a bug, and match the SHIPPED APP, not the headless oracle.
