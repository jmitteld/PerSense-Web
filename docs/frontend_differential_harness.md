# Frontend ↔ Engine Differential Harness

Stands up the automated equivalent of the manual "try a combination and eyeball
the screen" loop, for the frontend layer where most recent bugs lived. See
`docs/convergence_feasibility.md` for the why.

## The pattern

```
random inputs → real /api handler (DOS-oracle-validated) → shipped JS → assert reconcile
```

1. Generate randomized input vectors in Go (a *sweep*, not a fixed list).
2. Run each through the actual HTTP handler — its numbers are already validated
   against the real DOS engine by the oracle sweeps, so it is the source of truth.
3. Render / echo the result with the **shipped** JS pulled straight out of
   `cmd/persense/static/index.html` (no copy), run under Node with a tiny DOM stub.
4. Assert the displayed values reconcile with the engine across every case.

This catches the translation-layer bug classes that the engine oracle can't see:
render drift, wrong echoes, format/round-trip mismatches.

## What exists now

In `internal/api/frontend_diff_sweep_test.go` (skip cleanly if Node is absent):

- `TestFrontendAmzScheduleRenderSweepDOS` — sweeps 60 random amortization loans
  (incl. balloons) through the handler, renders each with the real
  `renderAmzSchedule`, and asserts every row's Balance equals the engine's
  post-payment balance and Principal equals the balance delta. The balance-column
  drift, timezone-bucketing, and CSV bugs found by hand this session would all have
  failed here.
- `TestFrontendAmzRequestMappingSweep` — sweeps 50 random typed field sets through
  the shipped `getAmzInput` and asserts the request body maps correctly (rate ÷ 100,
  money parsed from $/commas, dates → ISO, points ÷ 100, payment present only when
  entered). Guards the request-translation layer (the `.psn`-scramble class).
- `TestFrontendAmzHeadlinePaymentSweep` — runs the **real** `calcAmortization`
  (with `apiPost` stubbed to the engine response and a recording DOM) over plain /
  moratorium / principal-minimum loans, and asserts the displayed top-line Payment
  is the steady amortizing payment — not the interest-only first row (moratorium) or
  the high first row (principal-minimum). These were the client's recent reports.
- `TestFrontendPVRequestMappingSweep` — sweeps random Present Value inputs through
  the shipped `getPVInput` and asserts the request body maps correctly, especially
  the **True / Loan / Yield → continuous True-rate conversion** (verified against an
  independent reimplementation), plus the as-of date and lump-sum mapping.
- `TestFrontendMtgRequestMappingSweep` — sweeps random mortgage-grid field sets
  through the shipped `getMtgRowData` and asserts rate / pctDown / points ÷ 100,
  money/int parsing, and that only cells marked as user input are sent.
- `TestFrontendAmzRecalcIdempotentSweep` — runs the real `calcAmortization`, then
  asserts `getAmzInput` builds the SAME request before and after (computed/green
  cells read as blank, user cells survive) — including the amount-solve case. Guards
  against a solved value being fed back as a hard input on recalc.
- `TestFrontendClearPVStateSweep` — fills everything `clearPV` touches with junk
  (lump/periodic rows, contingency dropdowns, actuarial section incl. POD, the
  variable-rate schedule) across several row counts and asserts a full reset
  (perYr→12, COLA→0, rest blank, dropdowns→None). Guards the leftover-POD/COLA/perYr
  class that silently corrupted totals on worksheet reuse.
- `TestFrontendClearAmzStateSweep` — the same completeness check for
  `clearAmortization` (basic fields, payoff tool, advanced-option rows,
  moratorium/target/skip): asserts perYr→12, points→0, computed cells de-greened,
  summary hidden, schedule cleared, advanced options blank. Guards the
  leftover-advanced-options class (a stray target/moratorium/skip altering the next
  loan).
- `TestFrontendPVValueEchoSweep` — runs the real `calcPV` against engine responses
  over random forward PV worksheets and asserts the displayed Value cells reconcile
  with the engine: `sumValue` → `pv-total`, each `lumpSums[i].value` / `periodics[i].value`
  → its row's Value cell. Closes the PV display loop (the PV analog of the
  amortization render/headline sweeps).

All three screens' request-translation layers are swept; both amortization display
paths (schedule render + headline payment) are swept; amortization recalc is proven
idempotent; the PV clear-state reset is verified complete.

**CI:** the `go` job now sets up Node and runs `go test ./...`, so both sweeps run on
every push/PR — a frontend translation regression fails the build, no oracle binary
needed (the sweeps use the in-process API handler, whose numbers are oracle-validated
by the separate `dos-fidelity` job).

Companion isolated-function tests (single-case, not swept) live in
`cmd/persense/frontend_render_test.go` and pin specific fixes
(`TestAmzScheduleRenderJS`, `TestAutoCalcStaleGuard*`, `TestAmzDateMoneyHelpersJS`,
`TestAmzPrepayStopDateJS`, `TestKeyboardCalcWiring`, `TestPSNPrepayColumnOrder`,
`TestAPIAmortBalloonAmountEchoed`).

## How to extend (next sweeps, by ROI)

Each is the same pattern; add a `*_sweep_test.go` that swings a new dimension:

1. ~~**Request mapping** (amortization)~~ — done (`TestFrontendAmzRequestMappingSweep`).
   Still to extend: `getPVInput`/`getMtgRowData`, and the amortization advanced-option
   rows (prepay/balloon/adjust placement).
2. ~~**Echo / headline payment**~~ — done (`TestFrontendAmzHeadlinePaymentSweep`).
   Still to extend: the date echoes (firstDate/lastDate/nPeriods) and the
   amount/rate echoes under backward solves.
3. **PV value-cell echo** — request mapping done (`TestFrontendPVRequestMappingSweep`).
   Still to do: run the real `calcPV` against engine responses and assert each
   `lumpSums[i].value` / `periodics[i].value` lands in the right Value cell and
   `sumValue` in `pv-total` (incl. contingency probability suffixes, COLA,
   variable rate, solved-rate/date echoes).
4. ~~**Mortgage request mapping**~~ — done (`TestFrontendMtgRequestMappingSweep`).
5. ~~**Clear-state**~~ — done: `clearPV` (`TestFrontendClearPVStateSweep`),
   `clearAmortization` (`TestFrontendClearAmzStateSweep`), and recalc idempotency
   (`TestFrontendAmzRecalcIdempotentSweep`).
6. ~~**PV value-cell echo**~~ — done (`TestFrontendPVValueEchoSweep`). Still to
   extend: contingency probability suffixes and backward-solve echoes (solved
   rate/date/amount cells).
7. **Engine cross-product widening** — add field-presence × settings × advanced-option
   dimensions to the existing DOS oracle sweeps (where this session's engine-corner
   bugs hid).

## How to widen the ENGINE sweeps too

Today's engine bugs sat in unswept corners of the cross-product: the existing
oracle sweeps validate the backward SOLVERS directly (e.g. `goSolveBalloon` calls
`SolvePayment`), but the path the API/UI use is `Amortize()` with a blank payment,
which dispatches the solve internally — and that path wasn't swept.

**Done** (`internal/finance/amortization/dos_amortize_dispatch_sweep_test.go`,
runs in the `dos-fidelity` CI job, matches `-run TestDOS`):

- `TestDOSAmortizeDispatchSweep` — blank payment + (A) a known balloon, (B) an odd
  first period; 250 cases each, 0 divergences.
- `TestDOSAmortizeDispatchCrossProduct` — the cross-product basis {360,365} ×
  prepaid {off,on} × balloon {none,present} × balloon-includes-regular {off,on},
  natural first period; 658 cases, 0 divergences.
- `TestDOSOddFirstFancyFrontier` — the corner that once diverged (odd first
  period × {prepaid | balloon | 365}); now a STRICT zero-divergence guard after
  the two engine fixes that closed it. See `docs/dos_known_frontier.md`.

Net: the `Amortize` blank-payment dispatch is exhaustively swept against the real
DOS engine and is fully green — the previously-documented edge-of-usage frontier
(odd-first × prepaid/balloon/365) has been closed by refining the odd-first
payment solve and applying off-cycle balloons at their exact date.

**Still to add:** blank Amount / Rate dispatch with advanced options, and
target/skip/moratorium with a blank payment, against the oracle. Requires the
oracle built (`legacy/oracle/build_linux.sh`, `PERSENSE_ORACLE=...`).

## Running

```
go test ./internal/api/ -run TestFrontendAmzScheduleRenderSweepDOS -v   # needs node
go test ./...                                                           # everything
```

Node is required for the JS sweeps; they skip cleanly without it (as in the
dependency-light CI `go` job). Wire the full sweep into the `dos-fidelity` CI job
so divergences fail the build.
