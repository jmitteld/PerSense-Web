# UI ↔ Engine Confidence Assessment

Confidence that the full user-flow — UI input → API request → engine → response →
UI display — is faithful to the original DOS program, rated per section 0–100.

Each rating weights three things: (a) engine correctness against the real DOS
oracle (`legacy/oracle/`), (b) automated frontend-translation coverage (the
differential/render sweeps in `internal/api/frontend_diff_sweep_test.go` and
`cmd/persense/frontend_render_test.go`), and (c) known residual gaps documented
in `docs/dispatch_gaps.md`, `docs/missing_flows.md`, and `docs/discrepancies.md`.
These are calibrated estimates from test coverage, not guarantees.

## Scores

Updated 2026-06-17 after (1) the exhaustive option-cube work, (2) the path-to-95
frontend sweeps, and (3) the hardened-field-invariance fix. Prior scores in parens
where they changed. The Mortgage and Amortization moves reflect a shift from
*sampled* DOS validation to *exhaustive* enumeration of the discrete option lattice
— which both found and fixed real bugs the sampled sweeps had missed (see "Why
these moved" below).

| Section | Confidence | Basis |
|---|---|---|
| Welcome / navigation | 97 | Static; no computation |
| Mortgage | 97 (was 95) | Exhaustive dispatch cube (`TestDOSMortgageDispatchCube`, 540 cells, every dispatch shape × grid, 0 divergence); output-echo sweep (565 cells, every computed cell greened); hardened-field invariance guard; What-If 1-D + 2-D frontend sweeps |
| Amortization — core (forward + blank-payment solve) | 97 (was 95) | Exhaustive settings cube (basis × prepaid × in-advance × exact × pmts/yr) + per-row 360/365 cubes; real in-advance, skip, R78-on-365, 365-first-period bugs found & fixed |
| Amortization — Advanced Options | 96 (was 95) | R78/USA per-row cube, fancy×settings cube, fancy-365 per-row cube all 0 divergence; one bounded niche corner (in-advance × fancy, ~2-3%) |
| Present Value — backward solves | 95 | 7 paths DOS-validated; solved-cell echo sweep (rate/as-of/amount round-trip) |
| Present Value — forward | 95 | DOS oracle (multi-row, VR, COLA) + value-echo + contingency probability-suffix/green-output sweep |
| Request mapping & round-trip (all screens) | 95 | Request-mapping + recalc-idempotency sweeps on all 3 screens; advanced rows + VR-schedule + actuarial config mapping swept |
| Schedule render & display | 95 | Amz render (incl. balloons, off-cycle prepay, adjustments) + PV value-echo + mortgage output-echo all swept |
| Clear-state / auto-calc / keyboard / money format | 95 | Clear-state swept on all 3 screens; money reformat-on-blur sweep; F10/Enter wired; green output-cell state guarded both directions; hardened-field invariance guarded |
| Import / export (.psn) | 93 | 31 real Help worksheets parsed across all option blocks (`TestLoadHelpWorksheetCorpus`); import-only (no writer to round-trip) |
| PV — contingency / actuarial (POD, life tables) | 90* | LIVE actuarialmath oracle (survival/annuity/insurance/eₓ ~5e-13, POD ~5e-9) + library-independent SOA published-SULT anchor, both in CI; *DOS-specific table/rounding unverifiable — original source absent (client action) |

## Why Mortgage and Amortization moved (and why only to 97)

The earlier 95s rested on *randomly sampled* DOS sweeps. The exhaustive option
cubes replaced sampling with deterministic enumeration of every reachable discrete
option combination — and immediately surfaced **real engine bugs that sampling had
missed**: the in-advance blank-payment solve (up to 9% off, 256 cube cells), the
skip-month solve (~18%), Rule-of-78 wrongly applied on the 365 basis, and the 365
first-period prorate (plain *and* fancy schedules). All are now fixed and the cubes
are 0-divergence regression guards. So the +2 is *better-earned* than the prior 95:
the discrete space is now proven, not sampled, and the engine is actually more
correct.

It stops at ~97, not higher, for two honest reasons: (1) the **recompile-oracle
ceiling** — the oracle is a headless recompile of the DOS units, not the shipped
binary, so the last ~2 points need the client's real worksheet corpus
(`docs/path_to_99.md`); and (2) two **bounded, documented, reachable-but-niche
corners** remain — the in-advance × fancy payment-timing structure (~2-3%,
`docs/dos_known_frontier.md` #38) and the unimplemented "Exact method" setting (now
a hidden, unreachable toggle). Advanced Options sits one point below core because it
still carries the in-advance × fancy corner.

## Per-section assessment

**Mortgage (97).** Engine validated over the entire dispatch lattice — down-payment
group (price↔pct↔cash↔financed), price↔monthly, balloon and points — by the 540-cell
dispatch cube at 0 divergence, plus APR, two-mortgage compare, row generation and
the MS help examples. The frontend output path is swept too: `updateMtgRowUI` is
driven by the real handler and every computed cell is asserted to parse back to the
engine value *and* to turn green, while user cells are asserted to stay un-green.
Hardened cells (output→input via **H**) are now invariant under recalculation
(structural; see §H below). Residual ~3 pts is the recompile-oracle ceiling.

**Amortization core (97).** The most exhaustively validated engine. The blank-payment
dispatch and per-row schedule are checked against DOS over the *entire* discrete
option lattice — basis {360,365} × prepaid × in-advance × exact × pmts/yr × the
field-presence directions — not a random sample, at zero divergence. Getting here
fixed four real bugs the sampled sweeps had missed (in-advance solve, skip solve,
R78-on-365, 365 first-period prorate). Residual ~3 pts: the recompile-oracle ceiling
(oracle ≠ shipped binary) and float-vs-decimal cent rounding at the boundary.

**Amortization Advanced Options (96).** Every option (balloons incl. off-cycle,
prepayments, ARM adjustments, moratorium, target, skip) has a DOS oracle sweep, and
the R78/USA per-row cube, fancy×settings cube and fancy-365 per-row cube are all 0
divergence. Fancy *backward* solves (loan-amount / rate with advanced options active)
now refine the closed-form estimate against the real DOS-validated forward schedule
and are validated by `TestDOSFancyBackwardAmountRateRoundTrip` (0 fails). One bounded
niche corner holds it one point below core: in-advance × fancy payment timing (~2-3%,
frontier #38).

**PV forward (95) / backward (95).** Forward PV incl. variable-rate and COLA is
DOS-validated (lump 400 + periodic 600 + VR 500, all 0 divergence) and displayed
Value cells are echo-checked, including the contingency probability-suffix and
green-output state. Backward solving has all 7 dispatch paths validated against DOS,
and the frontend echo of *solved* values (a solved rate/date/amount landing in the
right green cell) is now swept by `TestFrontendPVSolvedEchoSweep`.

**Request mapping (95).** All three screens have randomized sweeps proving the
shipped JS builds correct request bodies (rate ÷ 100, money parsing, dates → ISO,
only user-entered cells sent). Recalc-idempotency is proven on all three: a recalc
rebuilds the same request because green (computed) cells read back as blank, so a
solved value is never fed back as a hard input.

**Schedule render & display (95).** The amortization render sweep reconciles the
displayed schedule against the engine row-by-row, including balloons, off-cycle dated
prepayment rows and rate adjustments, with coverage asserted. PV value-echo and
mortgage output-echo cover the non-tabular outputs.

**Clear / auto-calc / keyboard / formatting (95).** Where this session's reported
bugs surfaced (stale auto-calc, leftover POD/COLA on reuse, payoff-date binding) —
each now has a dedicated guard. Clear-state is swept on all three screens (every
output de-greens), money reformat-on-blur is swept, F10/Enter are wired, and the
hardened-field invariant is guarded. UI-state interactions are combinatorially
large; only known failure modes are pinned, hence 95 not higher.

**Import/export .psn (93).** Tests exist (incl. prepay column order) and all 31
genuine Help worksheets parse across every option block, but a legacy binary format
with many optional blocks is import-only here (no writer ⇒ no encode→decode
round-trip) and no out-of-corpus real files exist to widen further.

**Contingency / actuarial (90, capped externally).** Unlike every other module this
one has **no DOS oracle — and cannot have one from this snapshot**: the `ACTUARY`
unit that defines `LifeProb`/`PODValue` and the MALE/FEMALE mortality tables were
never in the source tree, and `-dACTU` won't compile
(`docs/actuarial_oracle_blocked.md`). In its place the engine is validated against an
**authoritative independent third party** — the SOA Standard Ultimate Life Table and
the `actuarialmath` library — across two mortality laws (Makeham + Gompertz), all six
contingency types, survival, life expectancy, annuities, insurance, and POD, agreeing
to ~5e-13 (POD ~5e-9), with a library-independent anchor to the SOA's *published*
values that runs offline in CI. That is stronger than a tolerance-based oracle match
for the *math*. The residual cap below 95 is narrow but real and **not closeable by
us**: bit-fidelity to the specific DOS implementation's table interpolation and
internal rounding can't be confirmed without the original `ACTUARY` source + tables.
**Client action** (supply those files) is the only path to 95 here; see
`docs/actuarial_files_to_request.md`.

## Watch-items closed this session

1. ✅ **Mortgage output-echo sweep** (90 → 95 → folded into 97) —
   `TestFrontendMtgOutputEchoSweep` drives the real handler → shipped
   `updateMtgRowUI`, asserts every cell parses back to the engine value with correct
   scaling and turns green. 565 cells / 60 rows.
2. ✅ **PV solved-value echo sweep** (86 → 95) — `TestFrontendPVSolvedEchoSweep`
   round-trips blank-Rate / blank-As-of / blank-Amount backward solves and asserts
   the solved value echoes into the right green cell. 30 worksheets.
3. ✅ **Advanced Options** (85 → 96) — `TestFrontendAmzAdvancedRowMappingSweep`
   (40 worksheets, prepay/balloon/adjust + mor/target/skip mapping) and
   `TestDOSFancyBackwardAmountRateRoundTrip` (fancy backward amount/rate validated
   against the DOS engine: 0 fails, max ~5e-6 amount / ~2e-5 rate). The
   "best-effort" caveat is retired.
4. ✅ **`.psn` import corpus** (80 → 93) — `TestLoadHelpWorksheetCorpus` parses all
   31 genuine Help worksheets across every option block. Capped below 95 only
   because the format is import-only and no out-of-corpus real files exist.
5. ✅ **Actuarial live third-party oracle** (→ 90, capped externally) — built around
   `actuarialmath` + the SOA SULT (`scripts/actuarial_oracle.py`,
   `internal/finance/actuarial/thirdparty_oracle_test.go`): survival/annuity/insurance/eₓ
   match to ~5e-13, POD to ~5e-9, with an offline library-independent SOA anchor. The
   live oracle even caught a POD discounting-convention subtlety (continuous force
   δ=ln(1+i) vs effective i), now documented in the test.
6. ✅ **Clear-state / keyboard / money-format** (90 → 95) — mortgage clear-state sweep
   (`TestFrontendClearMortgageStateSweep`, which also caught & fixed a leftover-green
   APR cell), money reformat-on-blur sweep across all screens
   (`TestFrontendMoneyReformatSweep`), F10/Enter wiring.
7. ✅ **PV forward** (93 → 95) — contingency probability-suffix display + green-output
   state on contingent rows (`TestFrontendPVContingencyEchoSweep`).
8. ✅ **Request-mapping recalc idempotency** — sweeps added for PV and Mortgage
   (`TestFrontendPVRecalcIdempotentSweep`, `TestFrontendMtgRecalcIdempotentSweep`), so
   all three screens prove a recalc rebuilds the same request; plus a VR-schedule +
   actuarial-config mapping sweep (`TestFrontendPVVRActuarialMappingSweep`).
9. ✅ **Schedule render** — the amortization render sweep now includes off-cycle
   prepayment rows and rate adjustments (not just balloons), with coverage asserted.

### G. Green output-cell state (user-flagged watch-item)

Now asserted in **both directions on all three screens**: computed outputs must turn
green and user inputs must not. Mortgage output-echo checks every field; PV
solved-echo + PV contingency sweeps check the PV cells; **amortization** payment /
amount / rate / term / dates / APR greening is positively asserted inside the shipped
`calcAmortization` (`TestFrontendAmzRecalcIdempotentSweep` — previously this only
checked idempotency; it now also asserts that an engine-filled blank goes green and a
user-supplied field does not). Every clear de-greens (PV/Amz/Mortgage clear-state
sweeps). A computed cell that fails to turn green, or a stale green marker after
clear/edit, now fails CI.

### W. Mortgage What-If table (was an untested frontend flow)

The What-If table was the one major mortgage flow the frontend sweeps had
skipped. It has two shipped JS paths: 1-D (vary one column → `/api/mortgage/whatif`
→ engine `GenerateRows` → `placeWhatIfRow`) and 2-D (a *client-side* reimplementation
that steps both fields and calls `/api/mortgage/calc` per cell, bypassing the
engine's `GenerateGrid`). Two exhaustive differential sweeps now pin it:

- `TestFrontendWhatIf1DSweep` — cube of vary field {rate, points, pctDown, years,
  price} × increment × line count × base row. Drives the shipped `runWhatIf`,
  feeds the real engine's `/whatif` response, and asserts (a) the request the JS
  built (vary name, the **÷100 increment scaling** for rate/points/pctDown, count
  = lines+1, base = `getMtgRowData`), and (b) every placed row's resulting `/calc`
  request matches the engine row's inputs (placement format round-trip, statuses,
  and row count — no drop/dup). 20 cases, 110 generated rows.
- `TestFrontendWhatIf2DSweep` — cube of field pairs × increments × counts × base.
  Drives the 2-D client-side stepping, captures the per-cell `/calc` requests, and
  asserts each grid cell equals the engine-faithful stepping `base[col] +
  step·increment` (same scaling), exhaustively. 20 cases, 210 grid cells.

Both pass at zero divergence — the flow was correct, it just had no guard.

The engine side is now DOS-grounded across **all six** vary fields, not just rate:
`TestDOSMtgGenerateRowsSweep` (VaryRate, 750 rows), `TestDOSMtgGenerateRowsMultiFieldSweep`
(Years/Points/%Down/Price, 1200 rows), and `TestDOSMtgGenerateRowsVaryMonthlySweep`
(VaryMonthly → price solve, 750 rows) — every generated row reproduces the real DOS
forward solve, 0 divergence, max relErr ~1e-9. The earlier "only VaryRate is
DOS-grounded" gap is closed; What-If is now exhaustively validated end-to-end
(frontend request/placement/stepping **and** engine per-row solve).

### S. Computational settings — exhaustive-sweep coverage

Per-setting audit of whether each general Computational Setting is swept
end-to-end vs the DOS oracle:

| Setting | Values | Swept vs DOS | Notes |
|---|---|---|---|
| Default payments/year | 1,2,4,6,12 / 26 / 52 | ✓ | settings cube + `TestDOSWeeklyBiweeklySweep` (26,52) |
| Default payments/year | **CAN / DAY** | n/a | Were **inert UI** (the global `set-perYr` dropdown is never read into any request; the API has no CAN/DAY→engine mapping, `Settings.Daily` is never set). **Now hidden** from the dropdown, mirroring the Exact setting; re-add when wired |
| COLA escalation month | anniversary | ✓ | `TestDOSPVOracleSweep` (ANN) |
| COLA escalation month | continuous | ✓ | `TestDOSPVOracleSweep` varies `cnt` (CNT) |
| COLA escalation month | **Jan–Dec** | ✓ (new) | `TestDOSPVColaMonthSweep` — all 12 months, 1200 cases, 0 divergence (added a `colamonth=N` mode to the PV oracle) |
| Interest on interest | Actuarial / USA | ✓ | `TestDOSAmortR78USACube` |
| Basis days/year | 360 / 365 | ✓ | settings + per-row cubes |
| Basis days/year | **365/360** | ✓ (new) | `TestDOSAmort365o360BasisSweep` — clean per-row (300) + odd-DAYS first period (400, the actual-day differentiator), 0 divergence (added `b365_360` oracle flag) |
| 1st interest prepaid | YES / NO | ✓ | `prepaid` dimension of settings cube |
| Interest paid in | Arrears / Advance | ✓* | `inadv` dimension; bounded in-advance×fancy corner (#38) |
| Balloon incl. regular pmt | NO / YES | ✓ | dispatch sweep toggles `PlusRegular` |
| Exact method | (hidden) | n/a | Intentionally unimplemented/hidden (§8) |
| Rule of 78s | NO / YES | ✓ | `TestDOSAmortR78USACube` (no-op on 365, documented) |
| Auto-calculate | On / Off | ✓ (UI) | stale-guards + harden↔auto-calc sweep |

All computationally-active settings are now exhaustively DOS-swept. The only
open item is **CAN/DAY**, which is not a test gap but a *wiring* gap: those two
dropdown options don't reach the engine at all, so there is nothing to sweep
until they are either wired up or hidden.

### H. Hardened-field invariance (client requirement)

Requirement: *"hardened fields should never change on recalculation."* Hardening
(press **H** / double-click) turns a green computed cell into a fixed input that
drives the next solve. The mortgage update path previously rewrote *every* echoed
field, so a hardened cell's survival relied on the engine echoing it back
bit-identically. That is now **structural**: a hardened cell carries a `data-hardened`
marker (set on harden, cleared when the user types over it or clears the row) and
`updateMtgRowUI` skips it entirely, so recalculation can never overwrite a frozen
value — even if a future engine change re-normalized the echo. Guarded by
`TestFrontendMtgHardenInvariantSweep` (50 worksheets: forward-solve Monthly → harden
→ blank Price → recalc; asserts the hardened Monthly drives the solve, is
byte-identical before/after, does not re-green, and the genuinely re-solved Price
greens). Freshly-typed (non-hardened) inputs are still echo-reformatted, so money
formatting is unchanged.

## Convergence status (2026-06-17, post-harden re-run)

Full exhaustive pass re-run after the hardening change: **12/12 packages pass, 0
failures**, every option cube at 0 divergence, identical to the pre-change baseline —
confirming the frontend fix did not perturb the engine. DOS differential (amort/PV/
mortgage/interest/dateutil), frontend Node sweeps (`internal/api`), and the actuarial
live oracle all green. The only non-zero residuals are the two documented bounded
corners (365 × in-advance × exact; in-advance × fancy ~2.9%), neither of which moved.

Every section is now ≥93; all the engineering-closable ones are at 95–97. The only
sub-95 sections are externally capped: actuarial 90 (needs client `ACTUARY` source +
tables) and `.psn` import 93 (import-only format, nothing to round-trip).
