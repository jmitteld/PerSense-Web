# Per%Sense Web — Usability Review

**Scope:** the web frontend served by the Go binary —
`cmd/persense/static/index.html` (the single-page app), plus
`help.html` and `quickstart.html`.
**Method:** expert heuristic evaluation against Jakob Nielsen's 10
usability heuristics, with each finding rated on Nielsen's 0–4
severity scale. A cross-cutting accessibility pass is included
because it materially affects ease of use.
**Date:** 2026-05-24.

---

## Overall score: 68 / 100 — "Good"

Per%Sense Web is a competent, carefully built application. It is
genuinely usable today: the fill-in-the-blank model works, the
documentation is excellent, and recent error-message work has paid
off. It loses points not on broken functionality but on **polish and
modern affordances** — there is no loading feedback, no save/restore,
the date and number fields are bare text boxes, several interaction
models are inconsistent between screens, and accessibility is
partial. None of the issues are catastrophic; most are minor-to-major
and individually cheap to fix.

| Band | Range | This app |
|---|---|---|
| Excellent | 85–100 | |
| Good | 65–84 | **68** |
| Fair | 45–64 | |
| Poor | 0–44 | |

### Severity scale (Nielsen)

`0` not a usability problem · `1` cosmetic · `2` minor · `3` major ·
`4` catastrophe.

---

## Per-heuristic scores

| # | Heuristic | Score | One-line verdict |
|---|---|---|---|
| 1 | Visibility of system status | 6/10 | Great input/output cell colouring; no progress or "settings active" feedback. |
| 2 | Match with the real world | 8/10 | Strong finance vocabulary; a little jargon ("harden") and one obsolete control. |
| 3 | User control & freedom | 6/10 | Clear buttons and dismissible modals; no undo, uneven destructive-action confirms. |
| 4 | Consistency & standards | 6/10 | Consistent toolbar and cells; primary-action verb, row model and "OK" semantics drift between screens. |
| 5 | Error prevention | 6/10 | Thorough pre-submit validation; bare text inputs for dates and numbers invite typos. |
| 6 | Recognition rather than recall | 6/10 | Excellent tooltips; hidden shortcuts and silently-active settings rely on memory. |
| 7 | Flexibility & efficiency | 7/10 | Shortcuts, sorting, What-If, examples, dark mode; no save/restore of a scenario. |
| 8 | Aesthetic & minimalist design | 7/10 | Clean and compact; retro button bevels, a dead column and an obsolete row add noise. |
| 9 | Error recognition & recovery | 7/10 | Best-in-class messages; errors not tied to the offending cell, network failure is silent. |
| 10 | Help & documentation | 9/10 | Genuinely excellent — layered help, per-field tooltips, worked examples. |
| — | Accessibility (cross-cutting) | 6/10 | Good modal/tooltip a11y; missing live regions, label associations, contrast gaps. |

Overall is the mean of the ten heuristics (68), cross-checked against
the accessibility sub-score (which would pull it no higher).

---

## What the app does well

These are real strengths and should be preserved through any redesign.

- **The fill-in-the-blank model is taught, not assumed.** The welcome
  screen, the per-screen help-hint line, and a white-vs-green colour
  convention all reinforce "type what you know, leave the rest
  blank." Computed cells are not colour-only — they also get an
  accent left border, italic text and bold weight, which is good
  redundant coding.
- **Tooltips are everywhere and done properly.** Almost every field
  has a `?` help affordance. The implementation is careful: a hover
  bubble for mouse users, a click/Enter modal for touch and keyboard
  users, the modal has `role="dialog"`, `aria-modal`, focus is moved
  in and restored on close, and Escape dismisses.
- **Documentation is layered and thorough.** A structured `help.html`
  with worked examples for every screen, a `quickstart.html`, and
  contextual Help buttons that deep-link to the right section.
- **Error messages are specific and actionable.** Recent work shows:
  messages name the field or row and suggest a fix. `explainMtgError`
  even adds scenario-specific guidance for the common
  over-determined mortgage case.
- **Stale results are invalidated.** Editing an amortization input
  wipes dependent computed cells and hides the now-stale schedule
  summary, preventing a classic "the number on screen no longer
  matches the inputs" trap.
- **Thoughtful touches:** dark mode persisted across sessions,
  three-state column sorting on the mortgage grid, a `t`-for-today
  date shortcut, "Load Examples" onboarding, and a destructive
  "Clear All" that confirms first.

---

## Findings by heuristic

### 1. Visibility of system status — 6/10

| ID | Finding | Severity |
|---|---|---|
| V-1 | **No loading indicator during API calls.** `apiPost` is an async `fetch`; Calculate / Generate Schedule give no spinner, no button-disable, no "working…" text. On a slow request the app looks frozen and the user may click again. | 3 |
| V-2 | **Active computational settings are invisible.** Settings live in a modal and are read silently at calc time. A user who set "USA Rule" or "Rule of 78s" three minutes ago gets different numbers with nothing on the main screen saying a non-default setting is in force. | 2 |
| V-3 | **Worksheet persistence across navigation is undiscoverable.** Switching screens preserves all inputs (by design), but nothing tells the user that — they may assume it reset, or be surprised it didn't. | 1 |
| V-4 | Derive-only amortization succeeds quietly — the only signal is a few fields filling in. A short "Term derived: 360 periods" confirmation would help. | 1 |

### 2. Match between system and the real world — 8/10

| ID | Finding | Severity |
|---|---|---|
| M-1 | **"Harden" is jargon.** "Double-click a computed cell to harden it as input" — *harden* is not a term users bring with them. "Lock as input" or "Keep this value" would land better. | 1 |
| M-2 | The obsolete "Year to divide century" setting is still shown (disabled, greyed). It made sense in 1995; today it is dead UI the user must mentally skip. | 1 |
| M-3 | A few labels are very terse ("Mo Tax+Ins"; `CAN`/`DAY` options in the per-year dropdown). Tooltips cover them, but the label alone is cryptic. | 1 |

### 3. User control and freedom — 6/10

| ID | Finding | Severity |
|---|---|---|
| C-1 | **No undo anywhere.** Combined with C-2 this means a misclick can destroy work irreversibly. | 2 |
| C-2 | **Destructive actions confirm inconsistently.** "Clear All" (mortgage) asks for confirmation; "Clear Row" (mortgage) and "Clear" (amortization — which wipes the *entire* screen including Advanced Options) do not. The amortization "Clear" is the most destructive of the three and the only one with no guard. | 2 |
| C-3 | A What-If run can expand the grid to dozens of rows; there is no "clear just the generated block" — the only retreat is "Clear All". | 1 |

### 4. Consistency and standards — 6/10

| ID | Finding | Severity |
|---|---|---|
| K-1 | **The primary action is named three ways.** Mortgage: "Calculate Row" / "Calculate All". Amortization: "Generate Schedule". Present Value: "Calculate". One concept, three verbs. | 1 |
| K-2 | **Three different row-count models.** Mortgage is a 12-row auto-growing grid; PV grids have an explicit "+ Add Row"; the Amortization Advanced-Options tables are a fixed three rows with no way to add a fourth. The user re-learns "how do I get another row?" on every screen. | 2 |
| K-3 | **Settings modal "OK" implies a commit that does not happen.** Settings are read live at calc time; there is no "Cancel that reverts". "OK" should be "Close", or the modal should genuinely buffer-and-commit. | 1 |
| K-4 | Minor label drift: the PV periodic column header says "Through" while tooltips and code say "To Date". | 1 |

### 5. Error prevention — 6/10

| ID | Finding | Severity |
|---|---|---|
| E-1 | **Date fields are plain text inputs.** No date picker, no input mask. The placeholder and a `t` shortcut help, but `MM/DD/YYYY` typed by hand is error-prone, and `parseDate` silently rejects (returns null) on a wide range of near-misses. | 2 |
| E-2 | **Numeric fields are `type="text"` with no `inputmode`.** On a phone the user gets the full alphabetic keyboard for a money field. | 2 |
| E-3 | Over-determination (e.g. Price *and* Monthly Total both filled) is only caught **after** the server round-trip. A live hint as the user fills the conflicting cell would prevent the error entirely. | 1 |
| E-4 | What-If "Increment" and "# of lines" accept any text; a runaway value is only caught by a post-submit cap. | 1 |

### 6. Recognition rather than recall — 6/10

| ID | Finding | Severity |
|---|---|---|
| R-1 | **Keyboard shortcuts are undocumented in the UI.** `t` = today, `h` = harden, and Enter-advances-focus exist only in `help.html`. Users will never discover them in normal use. | 2 |
| R-2 | **Enter does not submit.** Enter moves to the next field (a deliberate choice). It is non-standard for a web form and nothing on screen signals it. | 1 |
| R-3 | Choosing *which* fields to leave blank for each backward-solve is recall-heavy. The help-hint covers the basics, but the full set of solvable combinations lives in tooltips and help. A compact "what can I leave blank?" cue near the action buttons would help. | 1 |
| R-4 | See V-2 — a changed setting must be *remembered*; nothing on the worksheet reflects it. | 2 |

### 7. Flexibility and efficiency of use — 7/10

| ID | Finding | Severity |
|---|---|---|
| F-1 | **No save / restore of a scenario.** A page reload loses everything except the theme. For a tool users return to (a loan they are negotiating, a settlement model), the absence of "save this worksheet" is a real efficiency gap. | 2 |
| F-2 | **Export is uneven.** Amortization exports CSV; Mortgage and Present Value have no export or print path at all. No print stylesheet. | 2 |
| F-3 | "Load Examples" loads one fixed set across all three screens; users cannot pick an example relevant to their current task. | 1 |

### 8. Aesthetic and minimalist design — 7/10

| ID | Finding | Severity |
|---|---|---|
| A-1 | **Mixed visual eras.** Windows-95 `outset`/`inset` button bevels sit beside an otherwise modern flat Tailwind theme. The retro nod is intentional but reads as unfinished. | 1 |
| A-2 | The Amortization screen shows an **"APR %" column that is not implemented** (its own tooltip says so). A visible-but-dead control is noise — hide it until it works. | 2 |
| A-3 | The obsolete "Year to divide century" settings row (see M-2) is visual clutter. | 1 |
| A-4 | The mortgage grid is dense — 12 columns × 12 rows of 12px cells — and overflows horizontally below ~1100px wide. | 1 |

### 9. Help users recognize, diagnose and recover from errors — 7/10

| ID | Finding | Severity |
|---|---|---|
| D-1 | **A failed `fetch` is silent.** `apiPost` has no `try/catch`; if the server is unreachable the promise rejects, the calc function throws, and the user sees *nothing happen* — no error, no spinner stopping. Indistinguishable from "the button is broken." | 3 |
| D-2 | **Errors are not tied to the offending cell.** A mortgage error reads "Row 3: …" but row 3 is not visually flagged, and the message renders in a thin strip below a 12-row grid — potentially off-screen. The backend already has a `FieldError` type; the UI does not use it for inline highlighting. | 2 |
| D-3 | **Errors are not announced to assistive tech.** The error `<div>`s are not live regions, so a screen-reader user who presses Calculate hears nothing. | 2 |

### 10. Help and documentation — 9/10

A genuine strength. Layered help (welcome text → per-screen hint →
per-field tooltip → full `help.html` with worked examples →
`quickstart.html`), contextual deep-linked Help buttons. The only
gripes are small: no in-app guided tour for first-timers (the "Quick
Tour" is a help section, not an interactive walkthrough), and help
opens in a new tab with no in-context return path.

### Accessibility (cross-cutting) — 6/10

| ID | Finding | Severity |
|---|---|---|
| Y-1 | Error regions lack `role="alert"` / `aria-live` — see D-3. | 2 |
| Y-2 | Top-of-screen single inputs (`amz-amount`, `pv-rate`, etc.) are labelled by a `<th>` in a header row, not an associated `<label for>`. Screen readers may not announce the field name with the input. | 2 |
| Y-3 | The selected mortgage row is indicated by a hard-coded inline `#FFFF00` background — colour-only (no border/icon cue) and not theme-aware (a jarring pure yellow in dark mode). | 2 |
| Y-4 | The `?` tooltip trigger is 14×14px — below the ~24px minimum recommended touch-target size. | 1 |
| Y-5 | The italic 11px help-hint text (`--text-hint`, a mid green on a pale green page) is around 3:1 contrast — below WCAG AA (4.5:1) for small text. | 2 |
| Y-6 | Desktop-first layout: the mortgage grid is hard to use on a phone (horizontal scrolling, tiny tap targets). | 2 |

---

## Issue register, sorted by severity

| Severity | IDs |
|---|---|
| **3 — Major** | V-1 (no loading feedback), D-1 (silent network failure) |
| **2 — Minor** | V-2, C-1, C-2, K-2, E-1, E-2, R-1, R-4, F-1, F-2, A-2, D-2, D-3, Y-1, Y-2, Y-3, Y-5, Y-6 |
| **1 — Cosmetic** | V-3, V-4, M-1, M-2, M-3, C-3, K-1, K-3, K-4, E-3, E-4, R-2, R-3, F-3, A-1, A-3, A-4, Y-4 |

No severity-4 (catastrophic) issues were found.

---

## Recommendations, prioritised

> **Implementation status (2026-05-25).** All quick wins (1–8), all
> medium-term items (9–13), and all longer-term items (14–18) below have
> been implemented. The full build and the `go test ./...` suite pass
> after the changes. The only deferred item is a moderated usability
> test with real participants (see the methodology note).

### Quick wins — low risk, high impact (do first)

1. **Handle network failure** (D-1). Wrap `apiPost` so a rejected
   fetch returns `{error: "Could not reach the calculator service…"}`;
   every caller already checks `data.error`.
2. **Make errors announce themselves** (D-3, Y-1). Add
   `role="alert"` to the three error `<div>`s.
3. **Add a loading state** (V-1). Disable the action button and show
   "Calculating…" for the duration of the request.
4. **Surface the keyboard shortcuts** (R-1). Add a one-line
   "Shortcuts: T = today's date · Enter = next field" note to the
   per-screen help-hint.
5. **Confirm the amortization "Clear"** (C-2). It wipes the whole
   screen — give it the same confirmation "Clear All" already has.
6. **Set `inputmode`** (E-2). `inputmode="decimal"` on numeric cells
   and date inputs gives mobile users the right keyboard.
7. **Raise help-hint contrast and tooltip tap-target** (Y-5, Y-4).
   Pure CSS — darken `--text-hint`, enlarge the `?` hit area.
8. **Theme the selected-row highlight** (Y-3). Replace the inline
   `#FFFF00` with a class so dark mode is not jarring.

### Medium-term — worth a small project

9. **[Done] Unify the primary action and row models** (K-1, K-2). The
   amortization toolbar's "Generate Schedule" is now "Calculate" to
   match the other two screens, and all three Advanced Options tables
   plus the PV grids carry a consistent "+ Add Row" button.
10. **[Done] Inline field-level errors** (D-2). A failed calculation
    now outlines the offending cell(s) in red (`.cell-error`) and marks
    the mortgage row's number cell, so the user finds the problem
    without re-reading the message. The mark clears as the field is
    edited.
11. **[Done] Save / restore a worksheet** (F-1). The whole worksheet
    (all three screens, Advanced Options rows, and computational
    settings) is debounce-autosaved to `localStorage` and restored on
    the next visit; a final flush runs on `beforeunload`.
12. **[Done] Real date inputs** (E-1). Date fields are masked: typing
    digits inserts the `MM/DD/YYYY` slashes automatically, with the
    caret kept in place for mid-string edits.
13. **[Done] Show active non-default settings** (V-2, R-4). Both
    Settings buttons carry a count badge whenever a computational
    setting differs from its default, with a tooltip explaining the
    impact.

### Longer-term — polish

14. **[Done, revised]** The Amortization Points and APR columns (A-2)
    were briefly removed as unimplemented, then restored and fully
    wired: the engine's APR-with-points solve (DOS
    `EstimateAndRefineAPRwithPoints`) was already ported and tested, so
    the columns now compute a live APR — verified against help example
    AM_EX2 (12.7499%). The obsolete century-split setting row (M-2/A-3)
    is retained at the project owner's request — it stays in the
    Settings modal, disabled, with a tooltip explaining it has no
    effect.
15. **[Done]** Settled on one visual language — the Win95 bevels are
    gone in favour of a flat, modern panel/button treatment (A-1).
16. **[Done]** Added CSV export for Mortgage and Present Value, plus a
    print stylesheet that drops the chrome and keeps the worksheet
    (F-2).
17. **[Done]** Added a responsive `@media (max-width:640px)` pass for
    narrow windows and phones (A-4, Y-6).
18. **[Done]** Added an interactive five-step first-run tour, shown
    once and suppressed thereafter via `localStorage` (heuristic 10).

---

## Methodology note

This is an expert inspection, not usability testing with real users.
Heuristic evaluation reliably surfaces design problems but cannot
measure task success rates or time-on-task. The natural next step,
if budget allows, is a small moderated test (5–8 participants) of the
three core tasks — compute a mortgage payment, build an amortization
schedule with a balloon, value an annuity — to confirm which of the
issues above actually block users and in what order of pain.
