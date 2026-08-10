# uidiff — the DOS-anchored UI differential

**Item 24, open since round 39, built and proven in round 45b, committed in round 46.**

```
go build -o /tmp/persense ./cmd/persense && /tmp/persense -port 8080 &   # REBUILD: the server is go:embed
legacy/oracle/build_linux.sh                                             # -> /tmp/oraclebuild/amort_oracle
node testplan/harness/uidiff/run.js                                      # ~4 min for 258 screens on 2 cores
```

Exit codes are distinct on purpose: **0** no divergence, **1** divergences,
**2 HARNESS FAULT SUSPECTED** (identity controls failed, or the tell fired at
scale — the count is *not* a defect rate), **3** the harness itself threw.

Environment: `PERSENSE_URL` (default `http://localhost:8080/`),
`PERSENSE_ORACLE` (default `/tmp/oraclebuild/amort_oracle`), `UIDIFF_VERBOSE`.

## What it is, and what every other instrument here is not

| instrument | compares | against |
|---|---|---|
| `dos_fuzzer5` and the engine sweeps | the Go **engine** | the DOS engine |
| `internal/api/frontend_diff_sweep_test.go:447` | the **display** | **the engine's own response** — self-referential, by design and documented at `docs/frontend_differential_harness.md:15` |
| `testplan/harness/run_amz.js` | `amzScheduleData.result` — **engine space** | `amort_oracle`, over **29 hand-written cases**, one invocation, no adjustment-rate kicker |
| **this** | **what the user sees** — painted totals, painted schedule cells, the painted APR input, the painted grid | `amort_oracle`, over a **generated** stacked population |

Four of the amortization screen's last five defects lived in the gap on the
last row. `run_amz.js` is real prior art for the *mechanism* — reuse its
`fillAndCalc` discipline — but it is not this comparison.

## The three tiers

- **plain** — identity controls. All ten were clean in 45b *while the grid
  mapping was wrong*, because a plain screen carries no grid rows and is
  **structurally unable** to express a grid-mapping error. R49, one level down.
- **single** — 🚨 **the tier this round added.** Exactly one option away from
  plain. It can express a grid-mapping error *and* attribute it without
  ablation. On the first full run it named four wrong mappings by name.
- **stacked** — the adversarial population, median ~7 options.

## The three traps, as assertions (`selftest.js`, runs before anything is scored)

1. **`apr` and `bdump`/`adjdump` are separate OUTPUT MODES.** `apr` `Halt(0)`s at
   `legacy/oracle/amort_oracle.pas:1305`, so `apr adjdump` returns
   `apr 0.000000 status 0` — a real-looking zero — and no adjustment rows.
   `tokens.js` refuses to *build* a mixed line, `oracle.js` refuses to *run*
   one and asserts the shape of what came back, and the selftest **calls the
   binary directly to demonstrate the hazard is live in this build.**
2. **The 365/360 kicker, both directions.** In: the loan rate **and every
   adjustment rate** must be kicked (`handlers.go:1949`, call sites `:995` and
   `:1184`); kicking only the loan rate produced 63 of 63 diverging in 45b.
   Out: the page paints adjustment rates in *typed* space (`handlers.go:1449`
   `amzUnkickerRate` — the NF-1c fix) while `adjdump` reports *internal* space,
   so `compare.js` re-kicks before comparing. The selftest proves the kicker is
   **load-bearing on its probe case** rather than merely present (R51).
3. **MM/DD/YYYY for the UI, D.M.Y for the oracle, ISO never.** Every date typed
   into the page is first passed through the page's own `dateValidity()`, and
   every date the page paints back is passed through it again on the way out
   (R53). The live half asserts that the page still rejects ISO — if that ever
   changes, §89 needs re-deriving before this harness is trusted.

Plus **the tell**: payment, total-interest and total-paid diverging on exactly
the same set is the signature of two sides computing *different screens*.
`compare.js:theTell` says so out loud and, above 30% of the scored population,
downgrades the whole run to a harness fault.

Plus **the live-page double-submit check** on every passing case — calculate,
then calculate again with nothing changed. That is the round-45b process rule,
executable. It is what would have caught §89.

## Tolerances

Declared in one place (`compare.js:TOLERANCES`), each with its reason, and
**printed with every run** — so no rate this harness produces can be quoted
without them. CAUTION 1 records five unpinned floors in the engine fuzzer and
round 45 caught one of them deciding an APR answer; this instrument does not
add a sixth.

## What the generator cannot produce — say it, don't let it be inferred

`perYr ∈ {24, 26, 52}` (day-based periods; an off-grid date made 2 of 45b's 3
findings unadjudicable) · day-of-month > 28 · off-grid grid dates · horizons
beyond 2099 · backward/solved screens. And the **payment check is not scored**
on screens carrying a target or a moratorium: those have no level regular
payment, so DOS's `payment` line and any painted payment are different
quantities.

## The mapping this file owns

`tokens.js` is the single owner of UI-case → oracle argv. Before it, the date
tokens `bdate=`/`adjdmy=`/`predmy=`/`mordmy=` were **constructed nowhere** in Go
or JS — they existed only as hand-written literals and as Pascal parsers at
`amort_oracle.pas:194/269/383/1004`. Note `cmd/goamort` **rejects all four**.

Two mapping facts worth carrying:

- **`pts=` is emitted even at zero.** `amort_oracle.pas:899-906` sets
  `pointsstatus := inp`, and that is what makes DOS run the APR solver at all.
  Omit it and every plain screen returns the blank-APR shape while the page
  paints a correct APR — the whole APR axis silently compares nothing.
- **`balloonIncl` → `plusreg` is INVERTED.** `handlers.go:972` is
  `PlusRegular: !req.BalloonIncludesRegular`. So UI `'no'` → `plusreg`.
  `run_amz.js:41` has it the other way round, and that is reachable —
  `AMZ-C-57` and `AMZ-C-63` both set `balloonIncl:'yes'`. The consequence of
  getting it right: the web's shipped default is ADD while DOS's own default is
  REPLACE (`PEDATA.pas:68`) — decision 3a.14 / item 23, ratified **HOLD**. This
  harness compares the web as shipped against the configuration the web asks
  for, so that decision is not re-reported as a defect on every extras screen.
