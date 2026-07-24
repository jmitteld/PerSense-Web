# Design: Present Value Payment Table

**Feature:** add the DOS Present Value **table** (Ctrl-T / `MakePVLTable`) to the web
Present Value screen, presented with the same UI patterns as the web Amortization
schedule table.

**Authority chain** (per project rules — DOS for financial output, Windows for UI):

| Concern | Source |
|---|---|
| Table rows, values, totals | `legacy/src/dos_source/pvltable.pas` — `MakePVLTable`, driven from `PRESVALU.pas` `MakeTable` (Ctrl-T, code `make_table`) |
| Options UI (detail level, summary period, months) | `legacy/src/dos_source/TableOptionsDlgUnit.pas` (the Windows Table Options dialog) |
| End-user description + worked sample output | `legacy/src/win_source/Help/PV_Tables.html` ("Generating Present Value Tables", Example 17 output) |
| Web presentation patterns | Amortization schedule block in `cmd/persense/static/index.html` (settings caption, View row, sticky-header table, totals bar, Export CSV) |

---

## 1. What the DOS table is

"The table contains a line for each future payment, the amount of that payment,
and its contribution to the present value" (PV_Tables.html) — the forensic-expert
exhibit format. Mechanics from `pvltable.pas`:

- **One chronological stream.** Lump-sum rows are date-sorted; each periodic row
  is expanded payment-by-payment with a per-row `nextdate[i]` walker
  (`SortPayments`, `SelectNextPayment`). The streams are merged by date.
- **Same-date payments are combined** into one line (amounts summed) — **except
  in life-contingency mode**, where same-date payments stay separate because
  they may carry different contingencies (`fold_in_life` early-exit).
- **COLA applied per payment**: continuous mode `amt·exp(cola·YearsDif(t,from))`;
  stepped mode via a `colamult` multiplier stepped when the payment date crosses
  `coladate` (anniversary, or the settings COLA month), `coladate.y++` per step.
- **Value per payment**: `v = q·exp(−rate·YearsDif(t, asof))`; in variable-rate
  mode `v = ValueOfOnePayment(q,t)` (the VR discount integral). In life mode:
  `ifpd = v` ("Value if paid"), `v = ifpd·LifeProb(t, contingency)`.
- **Forever streams** (`todate = latest`) are cut at **50 years** from the
  from-date for the table.
- **Subtotals** (`TimeIsRipe`): a subtotal line is emitted when the months
  crossed since the previous payment intersect `cumset` (a set of calendar
  months). `cum` controls the level: `' '` detail only, `'A'..'Z'` detail +
  subtotals, lowercase = summary lines only. Summary-only lines are dated with
  the last payment date of the period.
- **POD row** (life mode with a Payment-on-Death): an "On death" line with
  `q = pod`, `value = PODValue` (XPODValue under VR), probability column `POD`.
- **Grand Totals**: sum of payments, sum of values (and in life mode the
  if-paid sum with aggregate probability `Σv/Σifpd`).

### Columns

Standard (DOS header, `hdr`):

    Date | Payment | Value | Cum Value

Life mode (`life_hdr1/2` — note: **no Cum Value column**):

    Date | Payment | Value if paid | Cntg | Actuarial Probability | Value

Formatting: money 2dp with thousands separators (`ftoa2`, honoring the commas
setting); probability 4dp (`ftoa4`); contingency letter (`actchar`: L/D/1/2/E/B)
shown beside the probability.

### Documented screen-vs-table discrepancy (NOT a bug)

PV_Tables.html documents that the table's Cum Value can differ slightly from the
screen's Present Value when payments are monthly/quarterly on the **365** basis:
the screen uses the equal-spacing closed form, the table sums actual per-payment
values, and "the answer at the bottom of the table is slightly more accurate than
the answer on the screen." The web port must reproduce this faithfully (i.e. the
table does per-payment `YearsDif`, not the closed form) and NOT "fix" the screen
to match. Presentation addition (port-only): when the difference exceeds a cent,
show a muted info note under the table quoting the help's explanation.

---

## 2. Web UX design

Placement: a new **"Payment Table"** block on the Present Value screen, below the
grids/actuarial/VR sections and above the error area — the same position the
schedule occupies on the Amortization screen. Hidden until a successful
Calculate; DOS requires a computed screen before Ctrl-T (`InsufficientData…`
gate), so the web builds the table from the last successful calc (including
solved/echoed values after a backward solve) and clears it whenever inputs
change (same staleness rule as the amort schedule).

Controls row (maps 1:1 to the Windows Table Options dialog):

- **View**: `Detail only` (default — DOS `cum := ' '`) · `Detail + summary` ·
  `Summary only`.
- **Summary period** (enabled unless Detail only): `Annual` (default) ·
  `Semi-Annual` · `Quarterly` · `Monthly` — Monthly offered only when some row
  has Per/Yr > 12, exactly like the dialog (`if m_PerYr > 12`).
- **Starting month**: 1–12 (default 1), from which the semi-annual/quarterly
  month sets are derived the way the dialog builds `cumset`; Annual uses the
  single month; Monthly uses all twelve.
- **Export CSV** button — DOS already defines the comma-separated line formats
  (`bCommaSeperated` branches); reuse them verbatim: detail
  `Date,Payment,Value,CumValue`, life-mode columns accordingly, `Grand Totals,…`
  final line.
- The **settings caption** (same component as `amz-schedule-settings`) records
  the computational settings the table was generated under.

Column count switches automatically to life mode when any included row carries a
Life contingency or a POD is set (`fold_in_life` equivalent) — not a user toggle,
same as DOS. Totals render as a Grand Totals footer row (sticky, like the amort
totals bar). Table body scrolls at max-height 450px with sticky headers
(`.schedule-table` styles as-is).

Screen output only — the DOS printer/file/Lotus destinations are covered by the
CSV export; the Windows dialog's "Output" combo is not reproduced.

---

## 3. API design

New endpoint mirroring DOS's separate Ctrl-T command:

    POST /api/presentvalue/table

Request = the existing PV calc request (same JSON shape — the frontend re-posts
the current screen) plus:

```json
{
  "...": "existing PVRequest fields",
  "detail": "detail" | "both" | "summary",     // DOS cum: ' ' / upper / lower
  "summaryPeriod": "annual" | "semiannual" | "quarterly" | "monthly",
  "summaryMonth": 1
}
```

Response:

```json
{
  "lifeMode": false,
  "rows": [
    {"kind": "payment", "date": "1993-02-15", "payment": 610.00,
     "value": 696.77, "cumValue": 696.77,
     "valueIfPaid": null, "contingency": null, "probability": null},
    {"kind": "subtotal", "date": "1993-12-01",
     "payment": 26000.00, "value": 25100.12, "cumValue": 120963.01},
    {"kind": "pod", "payment": 100000.00, "valueIfPaid": 41230.55,
     "probability": 1.0, "value": 41230.55}
  ],
  "grandTotals": {"payment": 1144023.68, "value": 648362.68,
                  "valueIfPaid": null, "probability": null},
  "screenValue": 648362.68,
  "tableVsScreenNote": false,
  "settings": { "...": "echo, for the caption" }
}
```

Errors reuse DOS wording: `"Insufficient data to make a table."` when the screen
cannot compute (PRESVALU.pas `InsufficientDataMessage(tablestr)` /
`DP_InsufficientDataForTable`).

---

## 4. Engine work

New file `internal/finance/presentvalue/table.go` — a direct port of
`MakePVLTable` and its helpers, reusing the package's existing primitives
(`YearsDif`, `Exxp`, VR `integrateRateForward`/per-payment value, `LifeProb`,
`PODValue`, the COLA anniversary walker):

| DOS | Go |
|---|---|
| `SortPayments` (sort lumps, compact periodics, init COLA walkers, 50-yr forever cutoff) | `sortTableStreams` |
| `SelectNextPayment` (min-date merge + same-date combine, COLA stepping) | `nextTablePayment` |
| `TimeIsRipe` + `cumset` | `subtotalDue(monthSet)` |
| `PrintNextPayment` / `PrintSummary` / `PrintPOD` / `GrandTotals` | row emitters appending typed `TableRow`s |

Fidelity traps to carry over explicitly:

1. Same-date combining is **skipped** in life mode (different contingencies).
2. Stepped-COLA walker in the table uses the **table's own** `colamult/coladate`
   walk (`InitializeColaData` + step-on-or-after semantics) — validate it lands
   on the same payments as the engine's `colaAnniversary` (incl. the §32
   raw-Feb-29 rule); any divergence adjudicates to the DOS **table** code for
   table output.
3. Value uses per-payment discounting even where the screen used the closed
   form — the documented discrepancy in §1.
4. Life mode: probability displayed is `v/ifpd` (not LifeProb re-evaluated),
   4dp; grand-total probability is `Σv/Σifpd`.
5. Summary-only lines are dated with the **last payment date in the period**
   (`oldt`), not the period boundary.
6. Forever periodic rows cut at from-date + 50 years (table only — the screen
   value still refuses/handles per the perpetual rules).

No changes to `calc.go`/`backward.go`/solvers — the table is read-only over the
computed screen. Blast radius: zero on existing outputs (new endpoint + new
file), so the full-suite rule is satisfied by the new differential tests plus
one green run of the PV package.

---

## 5. Validation plan

1. **Oracle extension**: add a `table` mode to `legacy/oracle/pv_oracle.pas`
   that calls `MakePVLTable` into the TStringList and dumps lines (the harness
   already links `pvltable` — see its uses clause — and has precedent for added
   modes, e.g. `vrp_gen`). Flags for `cum`/`cumset` to drive the three views.
2. **Differential tests** (`dos_pv_table_test.go`, oracle-gated like the rest):
   line-for-line vs the oracle across: multi-lump + multi-periodic merge;
   same-date combine (and non-combine under life); stepped vs continuous vs
   month-specific COLA; Feb-29 anchor; VR schedule; life single/two-life + POD;
   annual/quarterly/monthly subtotals in all three views; 50-year forever
   cutoff; 365-basis monthly (screen-vs-table discrepancy sign and size).
3. **Help golden**: PV_Tables.html Example 17 prints real output (weekly 610.00
   stream + lumps; first line `2/15/93 610.00 696.77 696.77`, grand totals
   `1,144,023.68  648,362.68`) — a `help_examples`-style pinned test.
4. **Frontend echo test** in `cmd/persense` (frontend_render-style): table
   renders, view switches, CSV matches the DOS comma format.

## 6. Implementation plan (post-approval)

1. `table.go` + unit tests (port, ~1 session, the bulk of the work)
2. `pv_oracle.pas` `table` mode + differential suite (oracle rebuilt in-sandbox)
3. `handlers.go`: `/api/presentvalue/table` (+ `verify_web_help_examples` entry)
4. `index.html`: Payment Table block (markup + `renderPVTable()` + CSV export +
   staleness wiring into the existing recalc/autocalc paths)
5. Help page section (link the existing PV help to the new table, mirroring
   PV_Tables.html) + manual-test-plan addendum (≈8 cases: PV-101…PV-108)

Open questions for sign-off:
- Default view `Detail only` (DOS default) — or `Detail + summary / Annual`
  (what forensic users usually print)?
- Show the port-only "table vs screen" info note (recommended), or omit any
  presentation additions?
- CSV columns: DOS's exact comma format (recommended for continuity with old
  workflows) vs. adding a header row (DOS's CSV has none).
