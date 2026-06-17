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

Updated after the confidence-raising pass (2026-06-17). Prior scores in parens.

| Section | Confidence | Basis |
|---|---|---|
| Welcome / navigation | 97 | Static; no computation |
| Mortgage | 95 (was 90) | DOS oracle + new output-echo sweep (`TestFrontendMtgOutputEchoSweep`, 565 cells) |
| Amortization — core (forward + blank-payment solve) | 95 | Exhaustive DOS dispatch + frontend render/headline/recalc sweeps |
| Amortization — Advanced Options | 95 (was 85) | Each option DOS-swept; fancy backward solves now DOS-validated (round-trip); advanced-row mapping swept |
| Present Value — backward solves | 95 (was 86) | 7 paths DOS-validated; new solved-cell echo sweep (rate/as-of/amount round-trip) |
| Request mapping & round-trip (all screens) | 94 (was 93) | Request-mapping sweeps on all 3 + advanced rows; amz recalc idempotent |
| Schedule render & display | 94 (was 88) | Amz render + PV value-echo + mortgage output-echo all swept |
| Present Value — forward | 95 (was 92) | DOS oracle (multi-row, VR, COLA) + value-echo + contingency probability-suffix/green-output sweep |
| Import / export (.psn) | 93 (was 80) | 31 real Help worksheets parsed across all option blocks (`TestLoadHelpWorksheetCorpus`); import-only (no writer to round-trip) |
| Clear-state / auto-calc / keyboard / money format | 95 (was 90) | Clear-state swept on all 3 screens; money reformat-on-blur sweep; F10/Enter wired; green output-cell state guarded both directions |
| PV — contingency / actuarial (POD, life tables) | 90* (was 72) | LIVE actuarialmath oracle (survival/annuity/insurance/eₓ ~5e-13, POD ~5e-9) + library-independent SOA published-SULT anchor, both in CI; *DOS-specific table/rounding unverifiable — original source absent (client action) |

## Per-section assessment

**Amortization core (95).** The most validated path. The blank-payment dispatch —
every basis/prepaid/balloon/odd-first/odd-days combination — matches DOS at zero
divergence; the frontend render sweep checks every row's balance/principal against
the engine; headline-payment and recalc-idempotency sweeps close the display loop.
Residual 5 pts: float-vs-decimal cent rounding at the boundary, and the inherent
limit that "matches our oracle" is bounded by the oracle's own coverage.

**Request mapping (93).** All three screens have randomized sweeps proving the
shipped JS builds correct request bodies (rate ÷ 100, money parsing, dates → ISO,
only user-entered cells sent); amortization recalc is proven not to feed solved
values back as hard inputs.

**PV forward (92) / backward (86).** Forward PV incl. variable-rate and COLA is
DOS-validated and displayed Value cells are echo-checked. Backward solving has all
7 dispatch paths validated against DOS, but the frontend echo of *solved* values
(a solved rate/date/amount landing in the right green cell) isn't swept like the
forward values — the gap between 92 and 86.

**Mortgage (90).** Engine strong: down-payment dispatch, APR, two-mortgage
compare, row generation, balloon/points all DOS-swept; MS help examples pass.
Deduction: only request-mapping is swept on the frontend — no automated check that
computed Cash/Financed/Monthly/Balloon land in the right cells.

**Advanced Options (85).** Every option (balloons incl. off-cycle, prepayments,
ARM adjustments, moratorium, target, skip) has a DOS oracle sweep. Two caveats
hold it below core: fancy *backward* solves (loan-amount / rate with advanced
options active) use closed-form + balloons rather than DOS's `Iterate`
(best-effort, flagged in code); advanced-option *row* request mapping isn't
differentially swept.

**Clear / auto-calc / keyboard / formatting (90).** Where this session's reported
bugs surfaced (stale auto-calc, leftover POD/COLA on reuse, payoff-date binding).
Each now has a dedicated guard. UI-state interactions are combinatorially large;
only known failure modes are pinned.

**Import/export .psn (80).** Tests exist (incl. prepay column order), but a legacy
binary format with many optional blocks is hard to exhaust without a corpus of
real files.

**Contingency / actuarial (87, capped externally).** Re-assessed upward after
reviewing the existing validation. Unlike every other module this one has **no DOS
oracle — and cannot have one from this snapshot**: the `ACTUARY` unit that defines
`LifeProb`/`PODValue` and the MALE/FEMALE mortality tables were never in the source
tree, and `-dACTU` won't compile (`docs/actuarial_oracle_blocked.md`). In its place
the engine is validated against an **authoritative independent third party** — the
SOA Standard Ultimate Life Table and the `actuarialmath` library — across two
mortality laws (Makeham + Gompertz), all six contingency types, survival, life
expectancy, annuities, insurance, and POD, all agreeing to ~5e-11
(`actuarial/sult_validation_test.go`, `sult_extended_test.go`). That is stronger
than a tolerance-based oracle match for the *math*. The residual cap below 95 is
narrow but real and **not closeable by us**: bit-fidelity to the specific DOS
implementation's table interpolation and internal rounding can't be confirmed
without the original `ACTUARY` source + tables. **Client action** (supply those
files) is the only path to 95 here; see `docs/actuarial_files_to_request.md`.

## Path to 95 — status

Done this pass (all green, in CI):

1. ✅ **Mortgage output-echo sweep** (90 → 95) — `TestFrontendMtgOutputEchoSweep`
   drives the real handler → shipped `updateMtgRowUI`, asserts every cell parses
   back to the engine value with correct scaling. 565 cells / 60 rows.
2. ✅ **PV solved-value echo sweep** (86 → 95) — `TestFrontendPVSolvedEchoSweep`
   round-trips blank-Rate / blank-As-of / blank-Amount backward solves and asserts
   the solved value echoes into the right green cell. 30 worksheets.
3. ✅ **Advanced Options** (85 → 95) — `TestFrontendAmzAdvancedRowMappingSweep`
   (40 worksheets, prepay/balloon/adjust + mor/target/skip mapping) and
   `TestDOSFancyBackwardAmountRateRoundTrip` (fancy backward amount/rate validated
   against the DOS engine: 0 fails, max ~5e-6 amount / ~2e-5 rate). The
   "best-effort" caveat is retired.
4. ✅ **`.psn` import corpus** (80 → 93) — `TestLoadHelpWorksheetCorpus` parses all
   31 genuine Help worksheets across every option block. Capped below 95 only
   because the format is import-only (no writer ⇒ no encode→decode round-trip) and
   no out-of-corpus real files exist to widen further.

Remaining, by reason:

5. **Actuarial — now 90, capped externally.** A LIVE third-party oracle was built
   around the `actuarialmath` library + the SOA Standard Ultimate Life Table
   (`scripts/actuarial_oracle.py`, `internal/finance/actuarial/thirdparty_oracle_test.go`,
   CI job `actuarial-oracle`): the engine's survival, annuity, insurance, and life
   expectancy match to ~5e-13 and POD to ~5e-9, with a library-independent anchor
   to the SOA's *published* SULT values that runs offline in CI. (The live oracle
   even caught a POD discounting-convention subtlety — continuous force δ=ln(1+i)
   vs effective i — now documented in the test.) The remaining cap to 95 is
   narrow: the original DOS app used 1988 HHS mortality tables and its own
   `LifeProb`/`POD` rounding, neither of which is in the snapshot, so its *exact*
   numeric outputs can't be reproduced. Closing that needs the client to supply
   the original `ACTUARY` source + tables (`docs/actuarial_files_to_request.md`).
6. ✅ **Clear-state / keyboard / money-format** (90 → 95) — mortgage clear-state
   sweep (`TestFrontendClearMortgageStateSweep`, which also caught & fixed a
   leftover-green APR cell), money reformat-on-blur sweep across all screens
   (`TestFrontendMoneyReformatSweep`), F10/Enter wiring already covered.
7. ✅ **PV forward** (93 → 95) — contingency probability-suffix display +
   green-output state on contingent rows (`TestFrontendPVContingencyEchoSweep`).
8. ✅ **Green output-cell state** (user-flagged watch-item) — now asserted in both
   directions: computed outputs must turn green and user inputs must not
   (mortgage output-echo + PV solved-echo + PV contingency sweeps), and every
   clear de-greens (PV/Amz/Mortgage clear-state sweeps). A computed cell that
   fails to turn green, or a stale green marker after clear/edit, now fails CI.

Remaining cross-cutting at 94 (request-mapping, render): already broadly swept;
residual is inherent (oracle ceilings, combinatorial UI-state). Pushable on request.
