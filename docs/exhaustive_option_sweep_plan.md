# Exhaustive option-sweep plan — Mortgage & Amortization

Goal: move the Mortgage and Amortization engines from "common paths + randomly
sampled corners validated against the DOS engine" to "**every reachable discrete
option combination** validated against the DOS engine across a value grid." This
is the strongest claim achievable against the recompile oracle; it pushes those
sections toward the oracle ceiling (~97–98). The residual gap to 99 is the
recompile-vs-real-binary assumption and test-power quantification — see
`docs/path_to_99.md`.

Everything here is differential against `legacy/oracle/` (the headless real-DOS
engine), at the cent / per-row level, asserting **zero** divergence.

---

## 1. The amortization option lattice

### 1a. Discrete dimensions (user-reachable)

| Dimension | Values | Oracle support today |
|---|---|---|
| Basis | 360, 365, **365/360 hybrid** | 360 ✓, 365 ✓, hybrid ✗ (needs flag) |
| Compounding | periodic, **daily** | periodic ✓, daily ✗ (needs flag) |
| Prepaid interest | off, on | ✓ (`prepaid`) |
| In-advance (annuity-due) | off, on | ✓ (`inadv`) |
| Exact interest | off, on | ✓ (`exact`) |
| Rule of 78 | off, on | ✓ (`r78`) |
| USA rule | off, on | ✓ (`usa`) |
| Balloon-includes-regular | off, on | ✓ (`plusreg`) |
| Pmts/yr | 1, 2, 4, 12 / 26, 52 | ✓ (weekly/biweekly are day-based) |
| First period | natural, odd-months, **odd-days** | ✓ (`first=`, `loandmy=`/`firstdmy=`) |
| Unknown field (dispatch) | payment, amount, rate, nPeriods, lastDate | payment ✓ direct; amount/rate ✓ via round-trip |
| Advanced option | none, balloon, prepay, adjustment, moratorium, target, skip | ✓ each (`b…=`, `presolve`, `adj`, `mor=`, `targ=`, `skip=`) |

### 1b. Validity constraints (mutual exclusions, per DOS)

Not a naive 2ⁿ cube — these structure the lattice:
- **Rule of 78** is a precomputed-interest method: combine only with plain /
  balloon loans, never with ARM adjustments or variable structures.
- **In-advance** changes interest timing; legal with everything but interacts
  with the odd-first-period proration (already a validated frontier).
- **PlusRegular** only matters when an extra (balloon/prepay) coincides with a
  payment date — sweep it only alongside balloon/prepay.
- **Daily** compounding is its own discount path; sweep as a separate axis.
- **USA rule** caps negative amortization — pair with low-payment cases to
  actually exercise it.

### 1c. Proposed sweep structure (avoids Cartesian explosion)

1. **Core settings cube** (free-composing): basis{360,365} × prepaid × inadv ×
   exact × perYr{1,2,4,12} = 64 settings combos, each over:
   - 5 dispatch directions (payment / amount / rate / nPeriods / lastDate), and
   - a 20-tuple continuous grid (amount, rate, term).
   → 64 × 5 × 20 ≈ **6,400 cases**.
2. **R78 axis**: r78 × {plain, balloon} × perYr{12} × grid. ≈ 800 cases.
3. **USA-rule axis**: usa × low-payment + adjustment end-states × grid. ≈ 800.
4. **Advanced-option × settings cube**: each of {balloon, prepay, adjustment,
   moratorium, target, skip} crossed with the core cube's basis×prepaid×perYr
   subset (16) × grid(10). ≈ 6 × 16 × 10 = **960 cases per option** → ~5,800.
5. **First-period axis**: {natural, odd-months, odd-days} × core-cube subset ×
   grid — extends the already-closed odd-first frontier across all settings.

Total Phase-1+2 ≈ **20k–30k oracle comparisons**. At a few ms each (batched),
this is a multi-minute dedicated CI job.

---

## 2. The mortgage option lattice (fully sweepable today)

| Dimension | Values |
|---|---|
| Unknown funding field | pct-down, cash, financed |
| Solve direction | monthly-from-price, price-from-monthly |
| Balloon | none, given (when+amount), solve-amount |
| Points | 0, nonzero (drives APR) |
| Tax | 0, nonzero |

→ 3 × 2 × 3 × 2 × 2 = **72 dispatch shapes**, each over a ~50-tuple grid of
(price, pct, years, rate, points) = **~3,600 cases**. `mtg_oracle` already
supports `monthly` / `price` modes + points + balloon, so this needs **no oracle
change** — it can land first.

Open item to confirm: `mtg_oracle` balloon-**amount-solve** mode (MS Example 3).
If absent, add it (small) or validate via the round-trip technique.

---

## 3. Oracle extensions required (Pascal, `legacy/oracle/`)

To reach *full* exhaustiveness, extend `amort_oracle.pas`:
1. **`b365_360` flag** → `df.c.basis := x365_360` + `SetYrDays`.
2. **`daily` flag** → `df.c.daily := true` (daily compounding discount path).
3. **Batch mode** (performance): read N cases from stdin, emit N result lines, so
   a 20k-case sweep is one subprocess instead of 20k. Mirrors the actuarial
   oracle's batch protocol (`scripts/actuarial_oracle.py`).
4. (Optional) direct `solveamt` / `solverate` emit modes — though amount/rate are
   already coverable via round-trip-through-payment
   (`TestDOSFancyBackwardAmountRateRoundTrip`), so this is a convenience, not a
   blocker.

`mtg_oracle.pas`: confirm/add the balloon-amount-solve mode.

---

## 4. Test structure & conventions

- New files: `internal/finance/amortization/dos_amort_option_cube_test.go`,
  `internal/finance/mortgage/dos_mortgage_dispatch_cube_test.go`.
- Compare the **modal regular payment** (amortization) / each output field
  (mortgage) and per-row balances to the oracle; tolerance 1e-3 relErr (the
  established convention; real divergences are ≥1e-2).
- Amount/rate dispatch via round-trip-through-DOS (oracle solves payment → Go
  solver recovers amount/rate → assert recovery), reused from the existing fancy
  backward test.
- **Coverage assertion**: tally cases per dimension class; fail if any class has
  0 cases (so a generator bug that silently skips a combo is caught).
- Skips cleanly when the oracle binary is absent (matches every existing DOS
  sweep), so the fast PR job is unaffected.

## 5. CI

Add a dedicated **`option-cube`** job (heavier; nightly or on-demand, not every
PR) that builds the oracles and runs the cube sweeps with `-run TestDOS.*Cube`.
Keep the existing fast `dos-fidelity` job for per-PR signal.

---

## Progress log

**Phase 1 — done (2026-06-17).** `TestDOSMortgageDispatchCube` (540 cells, 0
divergence) and `TestDOSAmortSettingsCube` (basis × prepaid × in-advance × exact ×
pmts/yr). The amort cube found and fixed a real **in-advance blank-payment-solve
bug** (256 cells, up to 9%) and isolated the unimplemented **Exact** setting (now
a hidden toggle — `docs/discrepancies.md` §8).

**Phase 2 — in progress.**
- `x365_360` hybrid basis: **no work needed** — `SetYrDays` maps it to 365 days
  for amortization (`lastrun=iamz`), identical to `b365`, already covered.
- `TestDOSAmortFancySettingsCube`: moratorium + skip-months crossed with the
  settings cube. Found and fixed a real **skip-month blank-payment-solve bug**
  (~18%, `refineFancyPayment` → `solveFancyPayment`). Now 0 divergence across the
  settings cube except the in-advance interaction (bounded).
- Two bounded in-advance corners (inadv+exact, inadv+skip) traced to one root
  cause in `fancyOverUnder`; consolidated follow-up (`docs/dos_known_frontier.md`).
- Still to do: `daily` compounding (bit-encoded in `peryr`, needs oracle work);
  R78 and USA-rule axes (schedule/row comparison, need settings threaded through
  the row helpers); advanced-option × settings for balloon/prepay/adjustment.

## 6. Phasing & effort

- **Phase 1 — no oracle change (~1–2 days):** mortgage dispatch cube (fully
  supported) + amortization core settings cube over {360,365} × {prepaid, inadv,
  exact} × perYr × 5 dispatch directions, via round-trip for amount/rate. Lands
  immediately and covers the bulk of real usage.
- **Phase 2 — oracle extensions (~3–5 days):** add `b365_360`, `daily`, and batch
  mode to `amort_oracle`; add the R78 / USA-rule / advanced-option × settings
  crosses; confirm mortgage balloon-solve.
- **Phase 3 — wiring (~1 day):** batch-mode performance pass + the `option-cube`
  CI job + a coverage report logged per run.

## 7. What it buys, and the residual

Buys: every reachable discrete option combination on the two most-used screens
validated against the real DOS engine across a value grid — the difference
between "we tested the common settings" and "we tested all of them." Expected to
lift Mortgage and Amortization toward ~97–98 (the recompile-oracle ceiling).

Residual to 99 (unchanged by this work, see `docs/path_to_99.md`): the
recompile-vs-shipped-binary assumption (needs real DOS outputs / a worksheet
corpus) and mutation testing to quantify test power.
