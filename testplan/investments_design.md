# Design: Investments screen port (DOS iINV = screen 5)

**Goal:** port the DOS Per%Sense **Investments** page to the web app, as the
fourth worksheet tab (Mortgage · Amortization · Present Value · **Investments**).

**Status of this document:** design for client review. One dependency gates the
implementation approach (§2) — everything else (data model, UI, API, validation
framework) is settled here from repository evidence.

---

## 1. What the DOS Investments screen is (repository evidence)

The repo's legacy tree contains the screen's complete **data layer** but not its
compute unit. From `PETYPES.PAS` / `PEDATA.pas` (all cited lines in the repo):

**Screen roster** (PETYPES.PAS:452): `iinv=5` exists only in the V_3 build
(`nscr=6`), alongside a sixth screen `iarb=6` (arbitrage/rebate — out of scope
here, see §8).

**Blocks** (PETYPES.PAS:195-199, PEDATA.pas:480-484, 672-683): four scrolling
payment grids plus a rates block, laid out top-to-bottom:

| Block | Columns (Fcol…Lcol) | Meaning |
|---|---|---|
| Deposits — single | `ddate, damount, ddollar0` | one-time deposits: Date, Amount, dollar-type |
| Deposits — periodic | `dfrom, dto, dtimes, dpamount, ddollarn` | recurring deposits: From, Through, Times/Yr, Amount, dollar-type |
| Withdrawals — single | `wdate, wamount, wdollar0` | one-time withdrawals (mirror of deposits) |
| Withdrawals — periodic | `wfrom, wto, wtimes, wpamount, wdollarn` | recurring withdrawals |
| Rates (bottom) | `irate, inflation, constdate` | Interest Rate %, Inflation %, Constant-$ Date |

**Records** (PETYPES.PAS:642-670) — the decisive semantic evidence:

```pascal
ilumpsum  = record date; amt0; constant: boolean; act0 ... end;
iperiodic = record fromdate; todate; peryr; amtn; constant: boolean; ninstallments; actn ... end;
irate     = record irr, iir: raterec; constdate: daterec ... end;
```

Two rates — `irr` (investment return) and `iir` (inflation) — plus a
**constant-dollar reference date**, and a per-row `constant` boolean marking
whether that row's amount is entered in constant (inflation-adjusted, as of
Constant-$ Date) or current dollars. The `ddollar0/ddollarn` columns are
string-formatted (PEDATA.pas:586-591) — i.e. a per-row **Current $ / Constant $**
toggle cell, not a number.

**So the screen models:** a savings/investment account with money flowing in
(deposits) and out (withdrawals) over time, growing at the investment rate,
with inflation handled explicitly — every flow individually expressible in real
or nominal dollars. The canonical questions it answers: *"will my savings
support these withdrawals?"*, *"what return do I need?" (IRR)*, *"what is the
account worth — in today's dollars — on date X?"*. This matches the product's
marketing ("Financial planning. Investment analysis and internal rate of return
(IRR)" — help IN_WhatDoesItDo).

**File format:** worksheet extension `.INV`, percent-script `%I`
(PEDATA.pas:1221, 1250) — same block-serialization the loader already parses
for `.AMZ/.MTG/.PVL`.

`act0/actn` fields exist for ACTU builds (life contingencies on flows); like the
PV table, the v3 product build compiles without ACTU, so these are out of the
authoritative scope.

## 2. The gating dependency: the compute unit is missing

The repo has **no Investments computational unit**:

- No source file registers `EnterProc[iINV]` / `ScreenProc[iINV]` (Amortize.pas
  registers iAMZ, PRESVALU/Mortgage theirs; nothing registers 5).
- No `INVEST*.pas` / investment screen unit exists in `legacy/src/dos_source/`,
  and no `InvestmentScreenUnit.pas` exists for the Windows/Delphi v3 port — the
  Windows version evidently never carried this screen. There are no Windows
  help pages for it either.
- Therefore the ONLY authorities are the **running DOS binary** and the
  **original DOS source distribution** (wherever the client's full source
  lives — the repo's dos_source is the subset needed for the four ported
  screens plus shared units).

This is the same situation the actuarial engine was in (ACTUARY.pas absent),
and it forks the plan:

**Plan A — source-first (STRONGLY recommended).** Locate the missing unit(s) in
the original DOS source. What to look for: any `.PAS` whose body contains
`EnterProc[iinv]` or that `uses` the `ilumpsum/iperiodic/irate` types —
likely names `INVEST.PAS`, `INVSCRN.PAS`, or similar; roughly 1-2 files. Drop
them into `legacy/src/dos_source/`, and the standard machinery applies: build an
`inv_oracle` (same pattern as `pv_oracle`/`amort_oracle`, headless FPC build),
port the unit to `internal/finance/investment/`, and differentially validate
line-for-line. This is the only path to the project's normal fidelity bar
("validated against the DOS oracle to the cent").

**Plan B — black-box reconstruction (fallback).** If the source is gone:
reconstruct the math from the running DOS app. The formulas are very likely
compositions of primitives the port already owns — the accumulation side of the
PV engine (`Exxp(rate·YearsDif)` growth, `SumFormula` for level streams, the
same basis/day-count machinery) plus an inflation deflator
(`exp(−iir·YearsDif(t, constdate))` for constant-dollar conversion). The
process: I generate a structured probe matrix (~40 worksheets spanning
lump/periodic × deposit/withdrawal × constant/current × bases × solve
directions), the client runs them in DOSBox and captures the screens, and the Go
engine is fit to reproduce every probe to the cent, then guarded by those probes
as golden tests. **Risks Plan B cannot close:** which fields are solvable
(field-presence dispatch), refusal wording, iteration/convergence quirks at
extreme inputs, and rounding-order subtleties — the classes of behavior that
took oracle access to get right on the other three screens. These would ship as
documented approximations until source turns up.

Decision requested from the client: hunt for the source first (days of
calendar time, saves weeks of reconstruction risk), or accept Plan B's caveats.

## 3. Web UI design (applies under either plan)

A new **Investments** tab, mirroring the established worksheet patterns:

- **Deposits** panel: Single grid (Date | Amount | $-type) and Periodic grid
  (From | Through | Times/Yr | Amount | $-type), both with Add Row, like the PV
  grids. $-type is a two-state cell: `Current $` / `Constant $` (DOS `constant`
  boolean; default Current).
- **Withdrawals** panel: identical mirror below it.
- **Rates block**: Interest Rate % (the tri-convention True/Loan/Yield entry
  set, converted client-side exactly as PV does — `irr` is a `raterec`),
  Inflation % (single yield, like COLA), Constant-$ Date.
- **Results**: the computed value(s) the DOS screen shows — final/als-of account
  value in current dollars and in constant dollars (exact result-row layout to
  be pinned from the DOS screen under either plan).
- Blank-to-solve field-presence dispatch (rate blank → IRR; an amount blank →
  required deposit/withdrawal; exact solvable set confirmed per §2).
- Standard chrome: settings strip + caption (basis participates via the shared
  Computational Settings), **active-modifiers summary line** (as just added to
  Amortization), Clear All / Undo / Export CSV, autocalc integration, T-for-
  today in date fields.
- `.INV` worksheet import via the existing legacy-file loader (extension plumb-
  through in `internal/fileio`), alongside `.AMZ/.MTG/.PVL`.
- Later phase: a payment-table analog (chronological deposit/withdrawal ledger
  with running balance — the natural sibling of the PV Payment Table, and
  likely what Ctrl-T produced on this screen if the DOS table unit supports
  iINV; confirm from source).

## 4. Engine & API sketch

- `internal/finance/investment/` — `calc.go` (forward valuation),
  `backward.go` (solvers), `types.go` (InvLumpSum/InvPeriodic/InvRates mirroring
  the Pascal records). Reuses `interest`, `dateutil`, and the PV package's
  stream-walking idioms.
- `POST /api/investment/calc` — request mirrors the screen:

```json
{
  "deposits":    {"lumpSums": [{"date": "...", "amount": 0, "constant": false}],
                  "periodics": [{"fromDate": "...", "toDate": "...", "perYr": 12,
                                  "amount": 0, "constant": false}]},
  "withdrawals": { "...": "same shape" },
  "rate": 0.07, "inflation": 0.03, "constDate": "...",
  "basis": "360"
}
```

  Response: computed values + solved-field echoes + warnings, per the other
  endpoints. Blank `rate` (with a target) → IRR solve, etc.

## 5. Validation plan

- **Plan A:** `inv_oracle.pas` in `legacy/oracle/` (modes: forward eval, each
  solver, a `table` dump if the screen has one) + `dos_inv_*_test.go`
  differential suites, gated by `PERSENSE_REQUIRE_ORACLE` like the rest; fuzzer2
  rotate-the-unknown harness once solvers land.
- **Plan B:** the probe-matrix goldens as the primary guard + property tests
  (round-trips: IRR solve → forward reproduces target; constant/current
  conversions invert; deposits-only equals PV accumulation identities), plus a
  manual-test-plan section (INV-001…) for client side-by-side runs.
- Either way: frontend echo tests in `cmd/persense`, and the screen added to
  the 300-case manual plan as a fourth section once behavior is pinned.

## 6. Phased implementation (post-approval)

1. **Source hunt / probe matrix** (client-assisted — the §2 decision).
2. Engine package + oracle (A) or probe-fit engine + goldens (B).
3. API endpoint + validation wiring.
4. Frontend tab (grids, rates block, results, chrome) + frontend tests.
5. `.INV` loader extension; help page section; manual-test-plan addendum.

Rough effort: phases 2-3 are the bulk; comparable to the PV table build if the
source is found, roughly double under Plan B due to probe iteration.

## 7. Open questions for the client

1. Can they locate the original DOS source for the Investments screen (the
   full source tree the shipped binary was built from)? This is the §2 fork.
2. Which solves do they actually use on this screen (forward value only? IRR?
   required-deposit)? Prioritizes solver order under either plan.
3. A few representative real worksheets (`.INV` files or screenshots) to seed
   the probe matrix / golden tests.
4. Do they also use the sixth screen (`iarb`/RBT — the arbitrage-rebate
   worksheet with methods CONTINUOUS/PERDC-CONT/DAILY/PERIODIC/PMT-TO-PMT/
   SKIP-PMT/US-RULE/P-BEFORE-I)? It shares this "V_3-only, compute unit not in
   repo" situation and would follow the same plan if wanted.

## 8. Out of scope

The `iarb` rebate screen (§7.4), ACTU life-contingent flows on investment rows
(not in the v3 product build), and printer/Lotus output destinations (CSV covers
them, per the PV-table precedent).

---

## Addendum — verification sweep result & Plan B refinement (2026-07-22)

**§2 verdict: CONFIRMED missing.** Exhaustive sweep of every legacy copy in the
repo — `legacy/src/dos_source`, `legacy/src/win_source`, both `src_documented`
mirrors, `legacy/oracle/units`, `oracle-src.tgz`, all `.dpr` project files, and
a repo-wide search for any consumer of `ilumpsum/iperiodic/irate` or an
`EnterProc[iINV]` registrant: the Investment compute unit exists in **none** of
them (the investment types have no consumers outside the shared data-layer
trio PETYPES/PEDATA/INTSUTIL). The Delphi project compiles 16 units — no
investment screen. Plan B applies.

**But the sweep found better authority than expected: the printed DOS manual**
(`manual/*.pdf`, scanned). The Table of Contents shows **Chapter 7 — "Financial
Planning and IRR's: The Investment screen" (pp. 91–105)**, and the scans include
the chapter's opening page 91, which states the engine semantics outright:

> "The paradigm of this screen is a balance between inputs (entered in the top
> half of the screen) and outputs (entered in the bottom half). PerSense
> automatically **equates the values of the two**, taking time and interest
> into account. A single interest rate, entered at bottom left, is assumed to
> prevail for all time." … "Leave a Deposit amount blank to ask 'How much
> should I be saving?'; leave a Withdrawal amount blank to ask 'How much will I
> have available?'" … IRR read from the Interest rate column.

and chapter 1 (p. 3) adds: *"whether to work in the Investment screen or the
Present Value screen is usually a matter only of convenience and intuitive
clarity; **both will yield identical results**."*

**Plan B is therefore not free-form reconstruction.** The engine equation is
`Value(deposits) = Value(withdrawals)` over the ALREADY-PORTED, oracle-validated
PV primitives (LumpSumValue / PeriodicSummation / YearsDif / Exxp), solving the
one blank cell (any dollar amount, or the rate = IRR — note the balance
equation makes the valuation reference date cancel out). What remains to pin
down empirically: the rate cell's convention (the cell layer suggests
true-rate, like the PV tratecol), the inflation/constant-dollar compounding
convention and its Constant-date anchoring, basis participation, and the
dispatch/refusal behavior. The screen shot on p. 91 fixes the exact UI: Funds
IN / Funds OUT halves, each with Single (Date|Amount|Dollars) and Periodic
(From|Through|PrYr|Amount|Dollars) grids, and a bottom row of Interest rate % ·
Inflation rate % · Constant dollars date.

**Client asks (in priority order):**
1. Scan manual pages **92–105** — the worked examples are golden tests for free.
2. Run the probe pack (`docs/inv_probe_pack.md` / Investment_Probe_Pack.pdf):
   5 discovery probes (screenshots of behavior, the built-in Alt-F1 examples,
   Ctrl-T) + 10 quantitative probes, each with predictions that discriminate
   the remaining convention hypotheses. Fresh restart before each probe (the
   §33 stale-state bug).
