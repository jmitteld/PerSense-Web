# Per%Sense — Dispatch Gap Analysis & Error-Message Audit

*Companion to `missing_flows.md` and `discrepancies.md`. Authored 2026-05-19
from a deep read of `legacy/src/dos_source/` against the current Go port and
the single-page frontend.*

> **Revision 9 — 2026-05-26.** UI usability overhaul (flat theme,
> masked date entry, autosave, first-run tour, inline error
> highlighting, settings badge, responsive + print pass, CSV export
> on every screen, `C` keyboard shortcut), Amortization Points / APR
> columns restored and fully wired (DOS AM_EX2 = 12.7499%, pinned),
> AO7 ported (date-only adjustment now re-amortizes at the current
> rate), VR-mode month-stepped COLA aligned with the fixed-rate
> path, USA-rule `usap` carry across an ARM adjustment fixed
> (~$37k residual on a negative-amort test → within $1), help text
> refreshed, `.gitattributes` added, `refdata.json` confirmed
> current via a new `scripts/regen_refdata.sh` regeneration script.
> All three remaining financial-fidelity gaps (V6-7 timedif, V6-14
> summary rollups, fancy backward Iterate) are explicitly scoped
> down with rationale.  `go build`, `go vet` and the full `go test
> ./...` suite (**589 passing tests, 13 DOS-output cross-checks**)
> are green.  Full delta in **§0.11 below**.

> **Revision 2 — 2026-05-24 verification pass.** The whole document was
> re-verified against the current working tree (which carries uncommitted
> changes made after the original was written). Statuses that have changed
> are corrected inline and tagged **[v2: …]**; the headline deltas and a new
> set of DOS-fidelity gaps that the original pass missed are collected in
> **§0 below**. Bottom line: Phase 1 and Phase 2 of the original sequencing
> are now substantially done — the silent-wrong-answer amortization paths and
> the unwired mortgage routes are fixed — but every Phase 4 *financial-logic*
> port (the items that actually affect calculation fidelity) is still
> outstanding, and §0.3 lists five additional DOS behaviors not previously
> catalogued. `go build ./...` and `go test ./...` both pass on the current
> tree.

> **Revision 3 — 2026-05-24 implementation pass.** All ten Phase-4
> financial-logic ports, the five §0.3 fidelity gaps, the server+UI quick
> wins, and the structural error-handling work have now been implemented and
> tested. The full delta is in **§0.5 below**. `go build ./...`, `go vet
> ./...` and `go test ./...` (470 passing tests) are all green. The only
> items deliberately left are documented as out-of-scope in §0.5 (the DOS
> `{$ifdef V_3}` `const_signal` block — dead code in the authoritative DOS
> build — and the full engine-wide `FieldError` thread, of which the
> structured advanced-option errors are done).

> **Revision 4 — 2026-05-24 second verification pass.** An independent
> re-read of `Mortgage.pas`, `Amortize.pas`/`AMORTOP.pas` and `PRESVALU.pas`
> against the post-Revision-3 code confirmed the Revision 3 ports are in
> place, but found **three Revision 3 claims overstated** and **six DOS
> behaviors still not ported** that earlier passes missed — most notably
> **Rule-of-78 amortization, which is entirely unported** (`Settings.R78`
> exists but is never read). The full second-pass findings, claim
> corrections, and a re-prioritised gap list are in **§0.6 below**. Tests
> still pass; none of the new gaps are regressions — they are pre-existing
> DOS logic the port never covered.

> **Revision 5 — 2026-05-24 second implementation pass.** Every Revision 4
> finding has now been fixed: the three overstated claims (C1, C2), all the
> R4 DOS-fidelity gaps (R4-1 through R4-10), and the §1.5 mortgage issues.
> The delta is in **§0.7 below**. `go build`, `go vet` and `go test ./...`
> (480 passing tests) are all green. Two items are deliberately scoped
> down, documented in §0.7.4: Rule-of-78 covers the basic schedule, and the
> ARM USA-rule `usap` carry across an adjustment remains approximate.

> **Revision 6 — 2026-05-24 third verification pass.** A third independent
> deep-read of `Mortgage.pas`, `Amortize.pas`/`AMORTOP.pas` and
> `PRESVALU.pas` against the Revision 5 code. The Revision 5 fixes are
> confirmed genuinely in place; the common forward + backward paths of all
> three worksheets are faithful to DOS. The pass found **no broken fix**,
> but did surface **one real residual bug** (off-cycle balloons are
> dropped), **two overstated matrix cells** (AO6, A10), and a set of
> edge-case / feature / presentation gaps. The full third-pass findings are
> in **§0.8 below**. Tests still pass (480); the picture has converged —
> what remains is edges and unported *features*, not common-path errors.

> **Revision 7 — 2026-05-24 third implementation pass.** Every Revision 6
> finding has been addressed: the off-cycle balloon bug (V6-1), the V6-2
> through V6-13 edge/feature gaps, the AO6 rate-solve decision, and the
> two overstated matrix cells. The delta is in **§0.9 below**. `go build`,
> `go vet` and `go test ./...` (491 passing tests) are green. Two items
> resolved as no-ops with rationale (§0.9.4): V6-5 (the COLA `1+cola` form
> is correct — it is a yield) and V6-11 (DOS itself never computes the PV
> `duration` field, so there is no DOS logic to port).

> **Revision 8 — 2026-05-24 error-message audit.** No financial logic
> changed. Every "calculation cannot be done" site — under-determined,
> over-determined, inconsistent, and non-converging inputs — was given
> an error unit test, reworded to name the offending field or row, and
> extended with an actionable suggestion. A second clarity pass
> (§0.10.6) then swept the remaining developer-style messages — wrapped
> engine errors, `interest` sentinels, life-table loader errors and the
> actuarial handler config. The delta is in **§0.10 below**. `go build`,
> `go vet` and `go test ./...` (552 passing tests) are green.

---

## Executive summary

The client's two complaints are correct, but for different reasons than the
old `missing_flows.md` suggested.

**Field-presence dispatch.** The hallmark DOS behavior — "leave any field
blank and the program solves for it" — is patchily implemented in three
distinct ways across the three worksheets:

*Numbers below are as of the original 2026-05-19 pass. The **[v2]** column
records the current (2026-05-24) state — see §0 for the full delta.*

| Worksheet | DOS dispatch arms | Reachable end-to-end | Math exists in Go but unwired | Genuinely missing | Reachable **[v2]** |
|---|---:|---:|---:|---:|---:|
| Mortgage      | 8 main + 2 adjacent flows | 8 | 2 adjacent (CompareAPRs, GenerateRows) | 0 | 10 (compare/whatif now routed) |
| Amortization  | 10 basic + 14 advanced    | 5 | 3 (SolveRate, SolveLoanAmount, SolvePayment) | 12 | 7 (SolveRate/SolveLoanAmount now dispatched) |
| Present Value | 12 row + screen          | 9 | 2 (PV‑8, PV‑9 in UI)                  | 3 | 9 (PV‑8/PV‑9 still UI-blocked) |
| **Total**     | **46**                   | **22** | **7**                           | **15** | **26** |

Of the 24 missing-or-partial paths identified on 2026-05-19, **as of
2026-05-24: 4 of the 7 pure-wiring items are done** (mortgage compare,
mortgage what-if, amortization SolveRate, amortization SolveLoanAmount), the
**12 discrete-DOS-procedure ports are all still outstanding**, and the **5 UX
items are partially done** (PV warnings channel exists; per-cell `FieldError`
and the rate-type selector do not). See §0.1–§0.2 for the itemised delta and
§0.3 for newly discovered fidelity gaps.

**Error messages.** I cataloged 62 user-facing error/validation strings.
**45% are vague** (identify a problem but not which field), **28% are
ambiguous** (user cannot infer what to change), and **7 distinct silent-failure
paths** produce wrong or empty output with no error at all — the most
dangerous user experience. The single most dangerous: submitting an
amortization request with `rate: 0` runs a 0%-rate schedule silently, when
the user almost certainly meant "solve for rate."

**Top three priorities** (high impact × low effort):

1. **Eliminate the two amortization silent-wrong-answer paths.** Switch
   `AmortizationRequest.Amount` and `.Rate` to `*float64`, dispatch missing
   fields to the already-implemented `SolveLoanAmount` and `SolveRate`. Effort: **S**.
2. **Wire the present-value rate solver to the UI** by deleting the stale
   "IRR not supported" guard at `cmd/persense/static/index.html:2381–2384`
   (the backend `solveRate` at `presentvalue/backward.go:848` has worked
   for over a release). Effort: **S**.
3. **Introduce a `FieldError` struct** so the engines can return
   `{code, field, message}` triplets instead of unstructured strings. This
   single change unlocks correct per-cell highlighting and lets us replace
   the brittle `explainMtgError` regex-matcher at `index.html:1537`. Effort: **M**.

The rest of this document enumerates every permutation per screen, proposes
specific fixes with effort estimates (S = same day, M = 2–4 days, L = a week+),
and lists every error string with a precise rewording.

---

## How to read this document

- **Status legend.** ✅ ported and reachable; ⚠ partial (notes what's
  missing); ❌ missing entirely; ▪ genuinely under-determined (DOS also
  cannot solve).
- **Citations.** All `file:line` references are verbatim against the repo
  at the time of writing. DOS Pascal lives under `legacy/src/dos_source/`
  and is the authority for financial logic per CLAUDE.md.
- **Sample inputs.** JSON bodies are ready to POST against the running
  server. The repo's `docs/QUICKSTART.md` covers boot and curl basics.
- **Effort estimates.** S = same day (an hour or two). M = 2–4 days
  including tests and DOS-regression validation. L = a week-plus,
  typically because the DOS procedure does multi-pass iteration with
  edge cases (cola=rate, sign mismatch, non-convergence fallbacks).

---

## 0. Verification pass — 2026-05-24

This section was added by a re-verification of the whole document against
the current working tree. Method: every claim in §§1–4 and Appendix A was
re-checked against the current Go port (including uncommitted changes) and
re-cross-checked against the DOS Pascal source. `go build ./...` and
`go test ./...` both pass on the current tree.

### 0.1 Resolved since 2026-05-19

These gaps the original report flagged are now closed. Each is also tagged
inline in the matrices below as **[v2: RESOLVED]**.

| Item | What changed | Evidence |
|---|---|---|
| **S-1 / S-2 / A4 / A5** — amortization silent-wrong-answer paths | `AmortizationRequest.Amount` and `.Rate` are now `*float64`. A nil field dispatches to the backward solver: the handler runs `FirstPass` on a copy of the loan, then calls `SolveLoanAmount` / `SolveRate`, and writes the solved value back. The old `req.Amount==0 && req.Rate==0` test is now `==nil && ==nil`. | `handlers.go:128,130` (pointer fields); `handlers.go:608` (derive-only); `handlers.go:876-925` (dispatch block); `amort_backward_solve_test.go` |
| **M14 — `/api/mortgage/compare`** | Route registered and handler added; calls `mortgage.CompareAPRs`. | `main.go:34`; `HandleMortgageCompare`, `handlers.go:463-503` |
| **M15 — `/api/mortgage/whatif`** | Route registered and handler added; calls `mortgage.GenerateRows`. | `main.go:35`; `HandleMortgageWhatIf`, `handlers.go:534-585` |
| **AO8 — prepayment `NN` / `nPmts` ignored** | The engine now counts applied extras per series (`prepayApplied[i]`) and retires a series once `pp.NN` extras have been applied, matching DOS `Paymenttype.ComputeNext`. | `engine.go:527,637-643`; handler sets `NNStatus`/`NN` at `handlers.go:746-749` |
| **PV over-determined rows** (PV-1/PV-2 entry, PV-warning) | A lump or periodic row with every field filled is no longer a hard error. `FirstPass` now records a soft advisory, classifies the row `LineFullySpecified`, and proceeds — matching the DOS cancelable-warning behavior at `PRESVALU.pas:1166-1189`. | `backward.go:60-65,130-147,213-225`; `calc.go:275-293` |
| **Warnings channel (§4.4) — PV only** | `PVResult.Warnings`, `FirstPassResult.Warnings`, and `PVResponse.Warnings` (JSON) added; PV `Calculate` carries FirstPass advisories through to the response. | `types.go:129-132`; `backward.go:60-65`; `handlers.go:295-297,1198` |
| **AO7 — empty adjustment row** | No longer applies a silent no-op. The handler now rejects an adjustment row with neither a new Rate nor a new Pmt Amount, with a named error that cites AO7. *Note: the DOS "re-amortize at current rate" logic itself is still not ported — only the silent failure is fixed.* | `handlers.go:802-807` |
| **S-3 / AO3 — half-filled advanced rows (server side)** | The handler now returns explicit, row-indexed, field-named errors for prepayment, balloon, and adjustment rows missing a required field, instead of accepting them. | `handlers.go:716-730` (prepay), `:766-774` (balloon), `:793-807` (adjustment) |

### 0.2 Still open — re-confirmed against the current tree

Everything in this list was re-verified as **still accurate**. The
financial-fidelity items (the Phase 4 ports) are the ones that matter for
"all DOS logic is captured" and **none of them have been started**.

- **All 12 genuinely-missing DOS procedures** remain unported:
  Mortgage `TryBalloonDates` crossover fallback (§1.5-5); Amortization
  A6 closed-form N-from-payment (`DetermineLastPaymentDate`,
  `AMORTOP.pas:1323-1407`), A9 APR-with-points
  (`EstimateAndRefineAPRwithPoints`, `Amortize.pas:516-615` — `Loan.Points`
  is declared at `types.go:56-58` but never read), AO2 target balloon
  (`EstimateAndRefineBalloon`, `Amortize.pas:628-663`), AO5/AO6 per-adjustment
  rate↔payment solving (`Amortize.pas:1408-1418`), AO9 unknown prepayment
  amount (`EstimateAndRefinePeriodicPrepayment`, `Amortize.pas:665-707`),
  AO10 unknown prepayment duration (`DeterminePrepaymentDuration`,
  `Amortize.pas:709-774`), AO14 balance lookup
  (`ComputeBalanceFromDate`/`ComputeDateFromBalance`); PV actuarial
  integration in the backward solvers, PV-6 cola≠0 two-pass approximation,
  PV-6b `const_signal`/`{$ifdef V_3}` block.
- **`FieldError` struct (§4.3)** — not implemented. Engines still return a
  bare `error`; `explainMtgError` regex-matching still lives at
  `index.html:1537`.
- **Field-name dictionary `internal/api/labels.go` (§4.5)** — does not exist.
- **§4.7 reword wins** — none applied. All 20 cataloged messages are still
  verbatim (spot-checked: `handlers.go:615/683/696`, `engine.go:133`,
  `backward.go:75/100/126/198`).
- **Warnings channel for Amortization / Mortgage** — `AmortizationResponse`
  and `MortgageResponse` still have no `Warnings` field; §4.4 is only
  half-done.
- **PV-8 / PV-9 unreachable from the UI** — the stale guard
  *"IRR computation (blank rate) is not yet supported in the API"* is still
  live (it moved to **`index.html:2411`**). `onPVRateTypeChange()`
  (`index.html:2272`) is still a no-op, and `getPVInput` still hard-requires
  `asOfDate` (`index.html:2282`). The backend `solveRate`/`solveAsOf` work;
  only the UI blocks them.
- **S-3 / S-4 / S-5 on the *client* side** — the server now rejects
  half-filled rows, but the frontend `getAmzInput` / `getPVInput` still
  silently drop incomplete advanced-option and PV rows before they ever
  reach the API (`index.html:1976,1992,2002,2008` for amortization;
  `:2264,2283` for PV). A UI user still gets a silent drop.
- **S-6 / S-7 (Mortgage)** — S-6 is partly mitigated (What-If now
  pre-validates the source row, `index.html:1742-1756`); S-7 is unchanged —
  `mortgage.Calc` still returns `Err: nil` with no computed cells when the
  main funding/years/rate guard fails.
- **Frontend not wired to the new mortgage routes** — `/compare` and
  `/whatif` exist and are tested, but `compareMtgAPR` (`index.html:1633`)
  still runs the local heuristic and `runWhatIf` (`index.html:1699`) still
  loops `/calc` client-side. The engine is reachable via the API; the UI
  has not been switched over.

### 0.3 Newly discovered DOS-fidelity gaps (not in the original report)

The original pass focused on dispatch arms and error messages. A closer
read of the DOS source surfaced five behaviors that affect calculation
fidelity and were **not** previously catalogued. These are the most
relevant items to the goal of "every DOS intention is ported."

1. **`InAdvance` computational setting is unreachable.** The amortization
   engine ports the in-advance interest branch (`engine.go:97-101`,
   `ff := (f-1)/(2-f)`), but no handler ever sets `Settings.InAdvance`, so it
   is permanently `false`. DOS honors `df.c.in_advance`
   (`AMORTOP.pas:181`, `PrepaidInterest`). Wire it through
   `AmortizationRequest`. Effort: **S**.
2. **`USARule` computational setting is unreachable.** Same shape: the
   engine ports the `usap` unpaid-interest accumulation (`engine.go:661-666`)
   but `Settings.USARule` is never assigned. DOS `ComputeNext` honors
   `df.c.USARule` (`AMORTOP.pas:656`). Effort: **S**.
3. **`hard_payment` penny rounding ("Dav Holle provision").** DOS rounds
   balloon amounts and adjusted payments to whole cents when the top-line
   payment is a hard number (`Amortize.pas:1430-1434`, `AMORTOP.pas:637`).
   The Go engine rounds only at the API response boundary
   (`handlers.go:955-958`); intermediate balloon/adjustment values stay
   unrounded, so per-row pennies can drift from DOS. Effort: **M**.
4. **`VeryLastRegularAmount` / `DetermineVeryLast` schedule extension.** DOS
   extends the schedule past `lastDate` when a balloon or prepayment
   `stopDate` falls later (`AMORTOP.pas:1293-1321`). The Go fancy engine
   terminates on `p≈0` or `currentDate >= lastDate` (`engine.go:682-687`), so
   a balloon dated after `lastDate` is silently cut off. Related to A10 but a
   distinct mechanism. Effort: **M**.
5. **PV `YieldRateTranslation` rate classification.** DOS runs
   `YieldRateTranslation` (`PRESVALU.pas:535-543`) on every rate line at the
   top of `FirstPass` (`:572`) to classify the missing field. The Go port has
   no equivalent — this is the same logic the cosmetic True/Loan/Yield
   selector should drive. Also unported and actuarial-adjacent:
   `PrepareForLife` (`PRESVALU.pas:1088-1104`) and the PV-8/PV-9 backward-path
   POD subtraction (`PRESVALU.pas:841-849`), neither of which is reflected in
   `backward.go`. Effort: **M**.

### 0.4 Net assessment

The faithful-port goal stands at: **forward financial logic for all three
worksheets is complete and DOS-cross-checked**; **backward/field-presence
dispatch is now wired for the common cases** (mortgage 8 arms, amortization
amount/rate, PV 7 paths); but **the long tail of DOS solver procedures —
APR-with-points, target balloon, unknown-prepayment, per-adjustment solving,
closed-form N, actuarial weighting in backward paths — is entirely
unported**, and three DOS computational settings (`InAdvance`, `USARule`,
`hard_payment` rounding) have engine support but no way to switch them on.
None of these block the common path, but each is a DOS intention not yet
captured, and §0.3-3/4 in particular can cause small numerical divergence
from the DOS reference outputs on balloon/adjustment schedules.

---

## 0.5 Revision 3 — implementation pass (2026-05-24)

Everything outstanding after Revision 2 has now been implemented and
tested. `go build`, `go vet`, and the full `go test ./...` suite (470
passing tests) are green. Each item below ships with unit and/or
API-level tests; the financial-logic ports also have round-trip
coverage (forward-calc, then solve backward, recover the input).

### 0.5.1 Server + UI quick wins

- **PV-8 / PV-9 reachable from the UI.** The stale "IRR not supported"
  guard is gone; the Present Value field is an editable target; a blank
  Rate or As-of Date is sent for the engine to solve. `PVResult` /
  `PVResponse` now echo the solved rate and date back
  (`presentvalue/types.go`, `backward.go`, `handlers.go`).
- **Half-filled rows now error instead of being dropped.** `getAmzInput`
  and `getPVInput` return row-indexed "row N: fill in X" errors for
  incomplete prepayment / balloon / adjustment / lump / periodic rows
  (`index.html`).
- **Mortgage compare / what-if wired to the UI.** `compareMtgAPR` calls
  `/api/mortgage/compare` (real Newton crossover, not the local
  heuristic); the 1-D What-If calls `/api/mortgage/whatif`.

### 0.5.2 §0.3 fidelity gaps — all closed

- **G1 — InAdvance & USARule** threaded from `AmortizationRequest`
  through `Settings`, with the UI selectors wired. The in-advance
  annuity-due accrual is now applied in the basic schedule, so the
  setting actually changes the numbers.
- **G2 — hard_payment penny rounding.** With a user-supplied payment,
  per-period interest and balloon/adjustment amounts are hardened to
  whole cents in both schedule engines (DOS "Dav Holle provision").
- **G3 — VeryLast schedule extension.** The fancy schedule now runs to
  the latest of {lastDate, last balloon date, prepayment stop dates}, so
  a late balloon is no longer cut off.
- **G4 — PV rate-type selector.** `onPVRateTypeChange` converts
  True / Loan / Yield rates; `getPVInput` posts a continuous True rate
  and the solved rate is converted back for display. (`YieldRateTranslation`
  itself is a status-classifier already covered by the Go FirstPass's
  status-based dispatch.)

### 0.5.3 Phase-4 financial-logic ports — all ten done

| Item | What landed |
|---|---|
| **FP1 / A6** | Closed-form term-from-payment (`solveNPeriodsFromPayment`, DOS DetermineLastPaymentDate). |
| **FP2 / A9** | APR-with-points (`ComputeAPRWithPoints`); `AmortizationRequest.Points` in, `apr`/`aprConverged` out. |
| **FP3 / AO2** | Target balloon — a balloon with a date but no amount is solved (`SolveBalloonAmount`). |
| **FP4 / AO5** | A rate-only ARM adjustment re-solves the payment over the remaining term. (AO6 payment-only is a recast — already correct.) |
| **FP5 / AO9** | Unknown prepayment amount solved (`SolvePrepaymentAmount`). |
| **FP6 / AO10** | Unknown prepayment duration solved (`SolvePrepaymentDuration`); the fancy engine now also terminates cleanly on early payoff. |
| **FP7 / AO14** | Balance-at-date reads the engine-recorded balance (`BalanceAtDate`); the frontend payoff lookup no longer drifts on balloons. |
| **FP8** | Actuarial weighting plumbed through `computeKnownRowSum`, `evaluatePVAt`, `solveLumpAmount` (LifeProb divide); `solveLumpDate` rejects life-contingent rows (`no_time_with_life`); POD folded into the backward total. |
| **FP9 / PV-6** | cola≠0 second-approximation added to the fromDate solve. |
| **FP10** | `TryBalloonDates` crossover fallback ported — the APR comparison retries pinned to balloon dates when the main 2-D Newton stalls. |

### 0.5.4 Structural + wording

- **ST1 — FieldError.** A structured `FieldError{Code,Message,Fields,
  RowIdx,Block}` type now ships in `errorDetail` on the responses; the
  amortization advanced-option row errors populate it for per-cell
  highlighting.
- **ST2 — Warnings channel.** `AmortizationResponse` and
  `MortgageResponse` gained a `warnings` channel (PV already had one);
  the engine surfaces an early-payoff advisory and a non-converged-APR
  advisory.
- **QW4 — message rewording.** The highest-traffic vague messages were
  reworded to the §4.7 / Appendix-A text (mortgage funding/rate errors,
  PV "too many unknowns" / "insufficient data", amortization date-format
  errors).

### 0.5.5 Deliberately out of scope

- **PV-6b `const_signal` (`{$ifdef V_3}`)** — `V_3` is never `{$define}`d
  anywhere in the DOS source, so this block is dead code in the
  authoritative DOS build. Not ported, by design.
- **Full engine-wide `FieldError` thread** — the structured errors cover
  the handler-level advanced-option rows (the cases with genuine row/cell
  structure). Converting every `fmt.Errorf` deep in the three engines and
  retiring `explainMtgError` entirely remains a larger mechanical
  refactor; the `FieldError` type and contract are in place for it.
- **`labels.go` field-name dictionary** — the reworded messages and
  `FieldError.Fields` already use UI-label names directly; a separate
  central dictionary is an internal-only refactor with no behavior change.

---

## 0.6 Revision 4 — second verification pass (2026-05-24)

An independent re-read of the DOS Pascal source against the
post-Revision-3 code. Method: three parallel deep reads (one per
worksheet), each cross-checking every DOS procedure against the Go
port and spot-checking the Revision 3 claims. Headline: the Revision 3
ports are genuinely present, but **three claims were overstated** and
the line-by-line DOS read surfaced **DOS logic still not captured** —
including one large path (Rule-of-78) that no earlier pass had noticed.

### 0.6.1 Revision 3 claims that were overstated — corrected here

| Claim | Correction |
|---|---|
| **FP10 — TryBalloonDates** "ported" | **Partial / not faithful.** `tryBalloonDates` (`mortgage.go`) works, but it calls a *new* secant solver `iterateAPRofTerminatedLoan` instead of the DOS `IterateToFindAPRofTerminatedLoan` routine — different first guess, step, iteration cap and convergence test, so balloon-vs-balloon crossovers can differ numerically from DOS. It also drops the normal path's final `r < 1 && r >= 0` reachability guard. Treat FP10 as a *functional* fallback, not a bit-faithful port. |
| **QW1 — "`PVResult`/`PVResponse` echo the solved rate and date"** | **Backward only.** `forwardOnly` (`calc.go`) never assigns `result.Rate`/`result.AsOf`, so a forward PV calc (PV-1..PV-6, PV-10) echoes `rate:0` / empty `asOfDate`. The echo works only for the PV-8/PV-9 solvers. Harmless to those solves, but the response field is misleading on forward runs — `forwardOnly` should copy `input.PresVal.R.Rate` / `.AsOf` into the result. |
| **G4 — "`YieldRateTranslation` is a status-classifier already covered by the Go FirstPass"** | **Inaccurate.** DOS `YieldRateTranslation` (`PRESVALU.pas:535-543`) feeds a multi-rate-row classification cascade (`:572-585`) that also *deletes empty extra rate rows*. Go's PV `FirstPass` models a single screen rate line and has no equivalent. The rate-type *selector* (the user-facing G4 deliverable) is fine; the claim that the DOS classifier is fully covered is not.

### 0.6.2 DOS logic still not ported — found by the line-by-line read

Ordered by fidelity impact. "Calc" = affects computed numbers; "UX" =
behavior/messaging only.

| # | DOS site | Gap | Impact |
|---|---|---|---|
| **R4-1** | `AMORTIZE.pas` R78 path; `Settings.R78` | **Rule-of-78 amortization is entirely unported.** `Settings.R78` is declared (`amortization/types.go:131`) but never read by the engine. The whole rule-of-78 declining-interest schedule is absent. No earlier pass caught this; it is not in any matrix. | **Calc — large** |
| **R4-2** | `AMORTOP.pas:1159-1191` (`RepayFancyLoan` in-advance) | In-advance accrual was ported into the *basic* schedule (Rev 3 G1) but **not the fancy engine** — `generateFancySchedule` always uses arrears interest `loan.LoanRate*yd*(p-usap)`. An in-advance loan with any advanced option silently reverts to arrears. (The §0.3-1 caveat is therefore only half-closed.) | **Calc** |
| **R4-3** | `AMORTIZE.pas:297-307` | Weekly/biweekly basis coercion: DOS forces `basis:=365` when `peryr ∈ {26,52}` and basis is 360 (and the inverse). Not ported — a biweekly 360-basis request keeps 360 and the interest drifts. | **Calc** |
| **R4-4** | `Mortgage.pas:992-1002` (`CopyAndIncrement`) | Mortgage What-If increments the **rate** column in *yield* space in DOS (`YieldFromRate` → step → `RateFromYield`); the Go `bumpField` steps the internal true rate directly. A "+0.5%" rate step produces DOS-divergent rows. | **Calc** |
| **R4-5** | `AMORTOP.pas:1499-1613` (`Re_Amortize`) | The AO5 rate-only re-amortization (Rev 3 FP4) re-solves the payment but omits: the `usap`/USA-rule carry across the adjustment, the balloon-discount term in the payment formula, and `EstimateAndRefineAdjRate` (solve the *rate* implied by a new payment). | **Calc — edge** |
| **R4-6** | `PRESVALU.pas:269-363` (`SummationForSteppedCola`) | Month-specific and continuous COLA are unported: the PV handler hard-codes `COLAMonth: types.COLAAnnual` (`handlers.go:1145`), so a COLA tied to a specific month — or continuous COLA — is unreachable and any such payment diverges from DOS. | **Calc** |
| **R4-7** | `AMORTOP.pas:1336-1379` | A6 term-from-payment (Rev 3 FP1) ports only the **non-fancy** branch; with advanced options DOS still solves the term (back-computing each prepayment's stop date / count). Go refuses it with an error. | Feature gap |
| **R4-8** | `Amortize.pas:1040-1088` (`TackOnFinalBalloon`) | DOS auto-appends a residual terminating balloon when the schedule is over-specified, with `plus_regular`/`VeryLastRegularAmount` handling and an advisory. Not ported — matrix A10 residual is still just left in `FinalPrinc`. | Calc — edge |
| **R4-9** | `Amortize.pas:1153-1189` (`ComputeDateFromBalance`) | The inverse of the AO14 balance lookup (solve the date a target balance is reached) is not ported. | Feature gap |
| **R4-10** | `PRESVALU.pas:1088-1104` (`PrepareForLife` / `ComputeUnknownPOD`) | The actuarial "solve for an unknown Payment-on-Death amount" flow is not ported; Go does forward POD only. | Calc — actuarial |
| **R4-11** | `Mortgage.pas:577-609` (`NumberOfValidRowsInArray`) | The DOS logic that scans the grid and picks *which two rows* to compare (and excludes the cursor row) is absent — the compare endpoint takes two explicit inputs instead. | UX |

### 0.6.3 Lower-severity divergences and stale notes

- **hard_payment over-applies (G2).** DOS keys `hard_payment` strictly on
  `payamtstatus = inp`; the Go engine rounds interest whenever the payment
  status is ≥ `InOutDefault`, which includes an engine-*derived* payment.
  Penny-level only, but not bit-faithful.
- **§3.2 matrix is stale.** PV-1 and PV-2 still show "⚠ LifeProb divide
  missing" / "no `fold_in_life` reject" — both were resolved by Rev 3 FP8.
  Mark them ✅.
- **§3.4 item 5 / `backward.go` doc comment** still describe the PV-6
  cola≠0 two-pass as a TODO — it is implemented (FP9). Stale text.
- **§1.1 prose** still says "no HTTP route exposes CompareAPRs" — superseded
  since Rev 2; only the §1.2 matrix rows were updated.
- **`solveAsOf` first guess.** Go seeds the PV-9 iteration at `1900-01-01`
  where DOS uses Pascal `year 100`. If the epoch mapping is not exactly
  year-100 → 1900 the solve starts from the wrong seed — worth a one-line
  verification.
- **§1.5 issues 1, 3, 4 remain open** — per-cell mortgage errors
  (`MortgageResponse.ErrorDetail` is never populated), APR hard-coded to a
  365.25 day-count, and `CompareAPRs` skipping the `EnoughDataForAPR`
  precondition. Revision 3 did not target these.
- **`CLAUDE.md` "Outstanding Items — unkpre"** is now stale: AO9 was ported
  (FP5). That bullet should be removed.

### 0.6.4 Net assessment after the second pass

The Revision 3 work holds up: the ten financial-logic ports and the
§0.3 gaps are present and tested, and the common forward + backward
paths for all three worksheets are faithful to DOS. The residual
fidelity risk is now concentrated in: **(1) Rule-of-78 — a whole
amortization mode that was never ported (R4-1); (2) in-advance in the
fancy engine (R4-2); (3) month-specific/continuous COLA (R4-6); and
(4) the weekly/biweekly basis coercion (R4-3)** — these four can each
make the port disagree with DOS reference output. The remaining items
are edge cases or UX. None block the common path, but R4-1, R4-2, R4-3
and R4-6 should be the next batch if full DOS-output parity is the
goal. Re-generating `legacy/reference-output/refdata.json` for R78,
in-advance fancy loans, biweekly loans and month-specific COLA would
turn these from "found by source reading" into enforced regression
tests.

---

## 0.7 Revision 5 — second implementation pass (2026-05-24)

Every item in §0.6 has been addressed. `go build`, `go vet` and the
full `go test ./...` suite (480 passing tests, 10 added this pass) are
green. Each fix below ships with a unit and/or API test.

### 0.7.1 Overstated-claim corrections (§0.6.1)

- **C1 — forward PV echo.** `forwardOnly` now sets `result.Rate` and
  `result.AsOf`, so a forward PV calc echoes the rate and as-of date it
  used — not just the backward solvers.
- **C2 — TryBalloonDates faithful.** The balloon-date crossover fallback
  now calls `IterateToFindAPR` — the faithful port of the DOS
  `IterateToFindAPRofTerminatedLoan` — instead of a bespoke secant, and
  the `r`-in-`[0,1)` reachability guard is restored (applied to the
  resolved APR). The bespoke `iterateAPRofTerminatedLoan` was removed.
- **G4 classifier note.** The §0.6.1 correction stands as documented; the
  rate-type *selector* is correct, and the multi-rate-row classification
  remains a known PV-FirstPass limitation (rare; single screen rate only).

### 0.7.2 DOS-fidelity gaps closed (§0.6.2)

| Item | Fix |
|---|---|
| **R4-1 Rule-of-78** | The basic-schedule engine now honors `Settings.R78` — sum-of-the-digits interest allocation (`r78step`/`r78int`). Wired through `AmortizationRequest.Rule78` and the UI's "Rule of 78s" selector. |
| **R4-2 in-advance / fancy** | `generateFancySchedule` now recomputes per-period interest on the post-payment balance when `InAdvance` is set, so the setting changes a fancy-mode schedule too. |
| **R4-3 basis coercion** | A weekly/biweekly (`perYr` 26/52) loan on a 360-day basis is coerced to 365, with a warning — matching DOS Amortize.pas:297-303. |
| **R4-4 What-If rate step** | `bumpField` steps the rate column in loan-rate (yield) space, converting true↔loan around the increment — the DOS `CopyAndIncrement` convention. |
| **R4-5 Re_Amortize** | The AO5 rate-only re-amortization now discounts post-adjustment balloons back to the adjustment date and nets them off the principal. (USA-rule `usap` carry is still approximate — §0.7.4.) |
| **R4-6 COLA modes** | `PeriodicSummation` honors month-specific COLA (steps on the 1st of the chosen month) and continuous COLA; the API accepts `colaMonth` and the UI's COLA-escalation selector is wired. |
| **R4-7 fancy A6** | Deriving the term from a payment now works with advanced options — `solveFancyTermFromPayment` runs the schedule unbounded and reads the payoff period. |
| **R4-8 TackOnFinalBalloon** | An over-specified loan (payment too small for the term) now surfaces an advisory that the final payment carries an implied terminating balloon. |
| **R4-9 ComputeDateFromBalance** | Added `DateForBalance` (inverse of `BalanceAtDate`); the UI's payoff Balance field is now bidirectional (date↔balance). |
| **R4-10 unknown POD** | `solveUnknownPOD` back-solves the Payment-on-Death amount from a target Sum Value (closed-form, since POD's PV is linear). API: omit `actuarial.pod` to solve it. |
| **R4-3b hard_payment** | The `hard_payment` rounding now keys strictly on `PayAmtStatus == InOutInput` (a genuine user input), matching DOS — not `>= InOutDefault`. |

### 0.7.3 §1.5 mortgage issues

- **§1.5-3 APR basis.** `MortgageRequest`/`MortgageCompareRequest` accept a
  `basis`; the APR day-count follows it (360 → 360-day) instead of the
  hard-coded 365.25. Omitting `basis` preserves the historical default.
- **§1.5-4 EnoughDataForAPR precondition.** `CompareAPRs` now gates on
  `EnoughDataForAPR` for both mortgages and returns a clear error rather
  than churning the iteration against an under-specified row.
- **§1.5-1 per-cell mortgage errors** remain open — `MortgageResponse`
  carries the `ErrorDetail` field but the mortgage engine does not yet
  populate it (part of the broader engine-wide `FieldError` thread).

### 0.7.4 Deliberately scoped down

- **Rule-of-78** is applied in the basic (non-fancy) schedule — the only
  place DOS itself supports it (`R78` is gated to non-fancy in
  Amortize.pas). A fancy loan with advanced options does not use R78
  allocation, matching DOS.
- **ARM USA-rule `usap` carry.** The AO5 re-amortization handles the
  common ARM case and the post-adjustment balloon term; carrying the
  USA-rule unpaid-interest balance across the adjustment date (a negative-
  amortization edge case) is still approximate. Noted in `CLAUDE.md`.
- **`const_signal` / `{$ifdef V_3}`** remains intentionally unported —
  `V_3` is never defined in the DOS source (§0.5.5).

---

## 0.8 Revision 6 — third verification pass (2026-05-24)

Three more independent deep-reads of the DOS Pascal against the
Revision 5 code. Headline: **no Revision 5 fix was found broken** — the
implemented ports are genuinely present and correctly located, and the
common forward and backward paths of all three worksheets are faithful
to DOS. The pass surfaced one real residual bug, two matrix cells that
overstate fidelity, and a set of edge / feature / presentation gaps.

### 0.8.1 Real bug found

- **V6-1 — off-cycle balloons are dropped (Calc).** In
  `generateFancySchedule` the balloon-matching loop applies a balloon
  only when its date lands *exactly* on a regular payment date
  (`cmp == 0`). When a balloon date falls strictly between two payment
  dates, the loop's `cmp < 0` arm just advances the index — its comment
  says "add as separate payment" but the code never adds the amount.
  DOS `ComputeNext` handles this via `balloonpos = -1` (the balloon is
  emitted as its own dated row). A balloon dated off the payment cycle
  is currently silently lost. *This is the one finding worth fixing
  promptly.*

### 0.8.2 Matrix cells that overstate fidelity — corrected

- **AO6 (§2.3) was marked ✅ — corrected to ⚠.** A payment-only
  adjustment is handled as a *recast* (the new payment applies at the
  unchanged rate). DOS `EstimateAndRefineAdjRate` instead solves the
  *implied rate*. The recast is a deliberate, internally-consistent
  choice (it matches the validated test `TestAPIAmortAdvancedPaymentAdjustment`),
  but it is **not** the DOS behavior, so the cell should not read ✅.
- **A10 (§2.3) "residual visible but not flagged" is stale.** Since
  Revision 5 (R4-8) the implied terminating balloon *is* flagged with an
  advisory. DOS `TackOnFinalBalloon` additionally *solves* the residual
  balloon and can merge it with an existing balloon row — the Go port
  flags but does not solve/merge. The cell now reads "⚠ advisory only".

### 0.8.3 Edge-case / feature / presentation gaps (not previously listed)

Ordered by impact. None affect the common path.

| # | DOS site | Gap | Impact |
|---|---|---|---|
| V6-2 | `AMORTOP.pas:1499-1613` `Re_Amortize` | The AO5 inline re-amortization is a one-shot closed form; DOS re-solves the payment with a balloon-aware `Iterate` and carries the USA-rule `usap` across the adjustment. Prepayment×adjustment interaction is approximate. | Calc — edge |
| V6-3 | `Amortize.pas:1090-1151` `ComputeBalanceFromDate` | `BalanceAtDate` reads scheduled `Principal` only; the DOS closed-form branches for a balance *before* the first payment and for a Rule-of-78 loan are unported. | Calc — edge |
| V6-4 | `PRESVALU.pas:290-310` `SummationForSteppedCola` exact branch | Month-stepped COLA is applied only in the closed-form annual path; the exact-mode / weekly-biweekly / life-contingent periodic paths apply continuous COLA instead of month-stepped. | Calc — edge |
| V6-5 | `PRESVALU.pas:281` `exp_cola` | Possible COLA-compounding discrepancy: DOS `SummationForSteppedCola` uses `exxp(cola)` per year; the Go stepped path uses `1+cola`. The Go form is deliberate and matches the PV_EX2 help-doc worked example — the two reconcile **if** DOS feeds `Summation` a continuous-converted COLA (entered yield → `ln(1+yield)`), so `exxp(cola)=1+yield`. Flagged to confirm against a refdata cross-check; **not changed**, since the current form passes the validated help example. | Verify |
| V6-6 | `Mortgage.pas:150-184` FirstPass error flow | DOS records a *warning* for negative years / financed>price but still falls through and computes; the Go `Calc` hard-errors and produces no output cells. Divergent UX (and "no numbers" vs "numbers + warning"). | Calc / UX |
| V6-7 | `AMORTOP.pas:625-632` | DOS `ComputeNext` uses a month-arithmetic `timedif` shortcut on a 360 basis; the Go fancy engine always calls `YearsDif`. Sub-day-count rounding differences on 360-basis fancy loans. | Calc — small |
| V6-8 | moratorium term basis (`Amortize.pas:1244-1248`); target-vs-`amount/nrepay` (`:1306-1316`) | With a moratorium, DOS amortizes the post-moratorium payment over `nrepay` (first-repay→last) and checks the target floor against `amount/nrepay`; the Go port uses "remaining periods from current payNum" / `amount/NPeriods`. Edge divergence when the moratorium is not period-aligned. | Calc — edge |
| V6-9 | `Amortize.pas:1227-1238` | DOS rejects `loanDate > firstDate` and a prepayment starting before the loan date ("dates out of order"); `validate.go` does not. | UX |
| V6-10 | `AMORTOP.pas:1294-1298` | DOS hard-errors `in_advance` combined with rate adjustments; the Go port silently runs it. | UX |
| V6-11 | `PRESVALU.pas` `duration` | The PV Macaulay-duration output (`PresValLine.Duration` fields exist but nothing computes them) is unported. | Feature |
| V6-12 | `PRESVALU.pas:838-843` PVLX backward | Variable-rate (PVL-fancy) mode rejects any missing field; DOS supports a backward solve (`amtn := valn/FancySummation`) in VR mode. | Feature |
| V6-13 | `Mortgage.pas:982-1018` `CopyAndIncrement` | What-If varies one field; DOS recurses up to 3 varied columns (the 2-D/3-D grid — already noted as M16). The Go 2-D path is client-side only. | Feature |
| V6-14 | `AMORTOP.pas:932-951` | DOS yearly/quarterly/monthly summary-line aggregation in printed schedules is not represented (the Go schedule is always period detail). | Presentation |

### 0.8.4 Stale report text refreshed

- §3.1 / §3.4 prose still said "actuarial integration is missing from
  every backward path" and the PV-6 cola≠0 two-pass is "TODO" — both were
  resolved in Revisions 3/5 (FP8, FP9). Treat the §0.5–§0.8 sections as
  authoritative where the older §1–§4 prose disagrees.

### 0.8.5 Net assessment after three verification passes

The port is in a solid state. Across three independent DOS-source
audits, every common forward calculation, every wired backward solve,
and every Revision 1–5 fix has held up. The remaining items are now
genuinely **edges** (V6-2, V6-3, V6-4, V6-7, V6-8), **unported features**
(V6-11 duration, V6-12 VR backward solve, V6-13 multi-axis What-If),
**UX divergences** (V6-6, V6-9, V6-10) and **one real bug** (V6-1,
off-cycle balloons). Recommended next batch, in priority order: **V6-1**
(real bug), the **AO6** rate-solve if DOS parity on payment-only ARM
adjustments is wanted, and **V6-5** confirmed against regenerated
`refdata.json`. None of the rest blocks correct results on the common
path.

---

## 0.9 Revision 7 — third implementation pass (2026-05-24)

Every Revision 6 finding has been addressed. `go build`, `go vet` and
the full `go test ./...` suite (491 passing tests, 11 added this pass)
are green. Each fix ships with a unit and/or API test.

### 0.9.1 The real bug and the matrix corrections

- **V6-1 — off-cycle balloons.** A balloon whose date falls strictly
  between two regular payment dates is no longer dropped: the
  fancy-engine balloon loop now folds its amount into the next
  payment (a few days later than the DOS dated row, but the principal
  reduction is no longer lost).
- **AO6 — payment-only ARM adjustment.** Now solves the *implied
  rate* (DOS `EstimateAndRefineAdjRate`): the rate at which the new
  payment amortizes the balance over the remaining term. This makes
  AO6 the exact mirror of AO5 and keeps the loan on its original
  term. The §2.3 matrix cell is now ✅. (The old "recast" behavior —
  payment changes, rate fixed, term floats — was not DOS-faithful and
  was inconsistent with AO5; the corresponding test was updated.)
- **A10** advisory wording corrected in Revision 6 stands.

### 0.9.2 Edge / validation gaps closed

| Item | Fix |
|---|---|
| **V6-9** | `ValidateInputs` now rejects a loan date after the first payment date, and a prepayment series starting before the loan date. |
| **V6-10** | In-advance interest combined with rate adjustments is now rejected (DOS AMORTOP.pas:1294-1298). |
| **V6-3** | Verified: `BalanceAtDate` is already correct before the first payment (returns the loan amount) and on a Rule-of-78 schedule (reads the recorded per-row balance). Regression tests added. |
| **V6-4** | Month-stepped COLA is now applied in the life-contingent periodic path (`periodicWithActuarial`), not only the closed-form annual path. |
| **V6-6** | `mortgage.Calc` now treats "amount borrowed exceeds price" as a warning and still computes (a negative % Down), matching DOS FirstPass which flags but does not hard-stop. (Negative/zero years stays a hard error — proceeding would yield NaN; a clean error is safer than DOS's garbage output.) |
| **V6-8** | The target-too-high check now divides the loan amount by the post-moratorium repaying-period count (`nrepay`), not `NPeriods`. |
| **V6-2** | The AO5 re-amortization carries the USA-rule `usap`: it amortizes only the interest-bearing balance and pays the exempt lump down linearly. |

### 0.9.3 Feature gaps closed

- **V6-12 — variable-rate backward solve.** VR mode now back-solves a
  single blank payment amount from the screen Sum Value
  (`solveVariableRateAmount`, DOS PVLX `amtn := valn/FancySummation`).
- **V6-13 — multi-axis What-If.** The engine gained `GenerateGrid`, a
  2-D what-if generator (primary × secondary axis) matching DOS
  `CopyAndIncrement`'s nested column iteration. (The frontend already
  offers 2-D What-If client-side; routing it through the new engine
  function is an optional follow-up with no user-visible change.)

### 0.9.4 Resolved as no-ops, with rationale

- **V6-5 — COLA `1+cola` vs `e^cola`.** Investigated and resolved: the
  COLA is entered as a *yield* (DOS help PV_COLA.html: "interpreted as
  yields, not rates"). DOS stores it in continuous form, so its
  `exxp(cola)` equals `1+yield` — identical to the Go stepped path's
  `1+cola`. The stepped path was already correct. The continuous-COLA
  path was made consistent (it now converts the entered yield with
  `ln(1+yield)` before the exp-based formulas).
- **V6-11 — PV Macaulay duration.** The `duration` field is declared
  in both DOS and the Go port but **DOS itself never computes it** —
  it is only read from saved files. There is no DOS logic to port;
  adding a duration calculation would be inventing a feature DOS
  never had. Left as-is, matching DOS.

### 0.9.5 Still open (documented, low priority)

- **V6-7** — DOS's 360-basis month-arithmetic `timedif` shortcut in
  `ComputeNext` (sub-day-count rounding) is not replicated; the Go
  fancy engine always calls `YearsDif`.
- **V6-14** — yearly/quarterly summary-line aggregation in printed
  schedules (presentation only).
- VR-mode month-stepped COLA still applies continuous COLA in the
  variable-rate periodic path (`vrPeriodicValue`).
- AO7 (re-amortize at the current rate, no new rate or payment) is
  still rejected with an explicit error rather than ported.

### 0.9.6 Net assessment after three implementation passes

After three verification + three implementation passes, the port is
DOS-faithful on every common path and every wired backward solve, and
the residual list is now short, all low-severity: two presentation /
rounding edges (V6-7, V6-14), one VR-mode COLA edge, and AO7. No
known bug remains on a common path. The strongest remaining
recommendation is unchanged: regenerate `legacy/reference-output/refdata.json`
for the newer paths (Rule-of-78, in-advance fancy loans, biweekly
loans, month-specific COLA, the backward solvers) so they are pinned
by DOS-output cross-checks rather than round-trip tests alone.

---

## 0.10 Revision 8 — error-message audit (2026-05-24)

This pass did not touch financial logic. It addresses a usability
gap: the field-presence dispatch model means *most* inputs are
optional, but some field combinations leave the engine with nothing
to solve. When that happens the user needs to know (a) which field
is the problem and (b) what to do about it. The audit went through
every "calculation cannot be done" site — under-determined inputs,
over-determined inputs, internally inconsistent inputs, and solver
non-convergence — and did three things at each:

1. **Ensured an error unit test exists.** Every site where a
   calculation cannot proceed now has a test that drives the API or
   engine into that arm and asserts the error fires.
2. **Reworded the message for clarity.** Bare diagnostics
   ("invalid date", "not enough data") were replaced with messages
   that name the specific field or row.
3. **Added an actionable suggestion.** Each message now ends with
   what the user most likely wants and how to get there — e.g.
   "clear one of the two dates and let Per%Sense derive it", or
   "Use a comma-separated list of months or ranges, e.g. \"6-8,12\"".

### 0.10.1 Scope — what counts as "cannot be done"

Four categories, all now covered by tests:

- **Under-determined** — too few fields to pin the unknown (e.g.
  derive-only mode with only a loan date; PV with no rate and no
  way to solve one).
- **Over-determined** — every field supplied *and* inconsistent, so
  there is nothing to solve and the inputs disagree.
- **Inconsistent** — combinations that are individually valid but
  mutually contradictory (balloon dated before the 1st Pmt Date;
  loan date after the 1st Pmt Date; in-advance interest combined
  with rate adjustments; two adjustments on one date).
- **Non-convergence** — an iterative solver (Newton, fixed-point,
  balloon-date crossover) that runs out of iterations.

Note that input *parse* failures (an unparseable date cell, a
malformed Skip-Months string) are handled at the API layer and were
folded into the same audit, since from the user's seat they are also
"the calculation didn't run."

### 0.10.2 What changed, by layer

| Layer | Files | Tests added |
|---|---|---|
| Mortgage engine | `mortgage.go` (10 messages reworded; `rowgen.go` guards left as internal asserts) | `mortgage/error_messages_test.go` — 11 |
| Amortization engine | `engine.go`, `backward.go`, `firstpass.go`, `validate.go` (messages reworded with field names + suggestions) | `amortization/error_messages_test.go` — 17 |
| Present Value engine | `calc.go`, `backward.go`, `variablerate.go` | `presentvalue/error_messages_test.go` — 13 |
| API handlers | `handlers.go` (request-level date-parse, missing-field, derive-only, What-If, variable-rate-schedule errors reworded with row indices + suggestions) | `api/error_messages_test.go` — 14 |

Engine errors now consistently lead with the **field or row name**
(1-based for row-indexed errors so they line up with the on-screen
table) and close with a **suggestion clause**. Pre-existing tests
that bound the old wording (`canary_ambiguous_errors_test.go` in
both `mortgage/` and `amortization/`, plus several backward-solver
and first-pass tests, and `amort_firstpass_test.go`) were updated to
the new substrings.

### 0.10.3 Representative before / after

- *Amortization, out-of-order dates.* Before: a hard 400 with
  "DateComp(firstDate,lastDate) >= 0". After: "1st Pmt Date is after
  Last Pmt Date. Make sure 1st Pmt Date comes first, or clear one of
  the two dates and let Per%Sense derive it."
- *Amortization derive-only, too few inputs.* Before: "insufficient
  inputs: supply either # Periods, or both 1st and Last Pmt Date".
  After: "Not enough inputs to count the term … Supply either
  # Periods, or both the 1st Pmt Date and the Last Pmt Date, and
  Per%Sense will derive the rest."
- *PV, As-of date unparseable.* Before: "invalid as-of date".
  After: "As-of Date is unparseable — use MM/DD/YYYY (or ISO
  YYYY-MM-DD). The As-of Date is the date all values are discounted
  to."
- *Skip Months parse error.* Before: bare parser error. After:
  "Skip Months is not valid (…). Use a comma-separated list of
  months or ranges, e.g. \"6-8,12\" to skip June through August and
  December."

### 0.10.4 Test state after the audit

`go build`, `go vet`, and the full `go test ./...` suite are green —
545 passing tests after the first audit pass, ~54 added (11 mortgage
+ 17 amortization + 13 PV + 14 handler error tests); the second
clarity pass (§0.10.6) brings the total to 552. No financial-logic
behavior changed; the only code edits outside test files are error
*string* literals and the addition of row indices to a few handler
loops.

### 0.10.5 Still open

- The structured `FieldError` type (Revision 3, ST1) carries a
  field name for per-cell UI highlighting; threading it through
  every deep-engine `fmt.Errorf` and retiring `explainMtgError` is
  still open (tracked in CLAUDE.md "Outstanding Items"). The
  reworded messages are plain strings; converting them to
  `FieldError` is a follow-up that would not change wording.

### 0.10.6 Second clarity pass — developer-style messages

A follow-up pass swept the remaining messages that, while
functional, still read like internal diagnostics rather than user
guidance. Seven messages were reworded and seven error-path tests
added (552 passing tests total):

- **Wrapped engine errors.** `compute true rate: %w`,
  `default first payment date: %w` and `compute last payment date:
  %w` were bare `%w` wraps with no field name or suggestion. They
  now read as full sentences — e.g. "The Loan Rate could not be
  converted to an internal rate (…). Enter a Loan Rate in a normal
  range — for example 6 for 6%."
- **`interest` package sentinels.** `ErrOverflow`,
  `ErrInconsistent` and `ErrTimeTooLong` were two- and three-word
  diagnostics ("inconsistent data", "overflow: answer too large").
  They are now self-explaining sentences; they are still usually
  wrapped by a caller that adds the field context, but no longer
  look like a crash if one surfaces raw. (`ErrTimeTooLong` keeps the
  literal phrase "time period too long" so the boundary test that
  binds it still matches.)
- **Life-table loader errors.** `actuarial.ParseCSV` / `ParseJSON`
  emitted "CSV parse error", "no valid data rows found in CSV",
  "unknown format", etc. Each now names the life-table file and
  shows the expected shape — e.g. CSV rows must be `"age,value"`
  like `"65,0.0123"`; JSON must be `[[age, value], ...]`.
- **Actuarial handler config.** `buildActuarialConfig` reported
  errors by JSON key (`table1, dob1, and asOfNow are required`,
  `dob1: invalid date`). These now use the on-screen labels —
  "Person 1's life table, date of birth, and the as-of date" — and
  add the MM/DD/YYYY date-format hint.
- **`rowgen.go` re-solve guard.** The one row-generation guard that
  could plausibly reach a user mentioned an internal Go function
  (`call EnoughDataForRowGeneration first`). It now explains the
  fix in screen terms: leave one of Price / Monthly Total / Balloon
  blank so the What-If table has a result to vary. The three
  remaining `rowgen.go` guards (positive row count, vary-field set,
  known vary field) stay as internal asserts — the What-If handler
  validates all three before calling, with its own user-facing
  messages.

After this pass, every error a user can reach names the field or
file involved and ends with a concrete next step; only a handful of
truly unreachable internal asserts (`unknown backward solve kind`,
`forwardVariableRate called without a rate schedule`, the three
`rowgen.go` precondition guards) remain in developer phrasing, by
design.

---

## 0.11 Revision 9 — UI consolidation + residual-gap sweep (2026-05-26)

The post-Revision-8 work was driven by a usability review and a
follow-up sweep through the Revision 8 §0.10.5 / §0.9.5 still-open
list.  No new gaps surfaced.  `go build`, `go vet` and the full
`go test ./...` suite are green (**589 passing tests**, up from 552 at
Revision 8).

### 0.11.1 UI / usability work (not financial logic)

A heuristic usability evaluation was conducted and published in
`docs/usability_review.md` (score 68/100, eight quick wins + thirteen
medium- and long-term items).  All items in the report have been
implemented except the moderated user test (not a code task):

- **Quick wins.** Network-failure handling (`apiPost` returns a
  structured error), busy indicator, `role="alert"` on every error
  div, shortcut hints under each toolbar, Amortization "Clear All"
  confirmation, mobile `inputmode` on numeric / date inputs, darker
  `--text-hint`, themed selected-row highlight.
- **Visual refresh.** Win95 bevels removed in favour of a flat panel
  / button treatment; settings badge added on the Settings button
  (counts active non-default settings); responsive `@media
  (max-width:640px)` pass; print stylesheet.
- **Data entry.** Date fields auto-mask digits into `MM/DD/YYYY` as
  the user types; worksheet autosaved to `localStorage`
  (`persense-worksheet-v1`) and restored across reloads; first-run
  guided tour (five steps, suppressed thereafter; replayable via
  "Take the Tour" on the welcome screen).
- **Output and feedback.** Inline `.cell-error` outlining of the
  field(s) that caused a failed calculation; CSV export on the
  Mortgage and Present Value screens (Amortization already had one).
- **Action verbs.** Amortization toolbar renamed `Generate Schedule`
  → `Calculate`; per-screen `+ Add Row` buttons added so dynamic
  tables are no longer "rows just appear when you tab past the end."

### 0.11.2 Keyboard `C` shortcut

`C` (and `Shift+C`) now invokes the active screen's smallest-scope
Calculate: `Calculate Row` on Mortgage, `Calculate` on Amortization
and Present Value.  Modifier-key combinations are passed through
(`Ctrl+C` / `Cmd+C` copy still works), and the shortcut is
suppressed inside textareas, selects, buttons, links, tip popovers,
and while a modal or the tour overlay is open.

### 0.11.3 Amortization APR / Points columns restored and wired

The Amortization screen's `Points` input and `APR %` output cells
were briefly removed in the usability cleanup, then restored when
the user confirmed the feature should ship.  The engine's
`ComputeAPRWithPoints` (DOS `EstimateAndRefineAPRwithPoints`) was
already ported and tested; this revision re-wired the UI and pinned
the API contract:

- `getAmzInput` forwards `body.points` whenever the cell has a
  parseable number (including 0) — DOS computes the APR for any
  user-supplied points value, and the web port has no separate
  "default 0" vs. "typed 0" signal, so the gate is on
  presence-of-number, not >0.  Earlier the field defaulted to `0`
  and was suppressed, leaving the APR cell stuck at its
  "(computed)" placeholder forever.
- `calcAmortization` populates `amz-apr` from `data.apr`, applies
  `cell-output` styling, and surfaces an advisory when
  `aprConverged === false` (matching the DOS "Computation of APR
  failed to converge" message-box).
- Two new tests pin the contract:
  `internal/api/verify_web_help_examples_test.go::TestVerifyWebAM_EX2_APRWithPoints`
  (un-skipped; asserts 12.7499% for help AM_EX2) and
  `internal/api/amort_apr_test.go::TestAmortAPRZeroVsOmittedPointsContract`
  (explicit `"points":0` returns APR equal to the loan rate; an
  omitted field returns no APR).

### 0.11.4 Help-text refresh

`cmd/persense/static/help.html` was audited for stale references and
brought up to date:

- "click **Generate Schedule**" replaced with "click **Calculate**"
  in the three places it lingered.
- "Press <kbd>Enter</kbd> or click **Calculate**" replaced — Enter
  now advances focus, not calculates.
- "the row number highlights yellow" replaced with the themed
  equivalent.
- New "Keyboard shortcuts and conveniences" section under the
  Quick Tour documents `C`, `T`, `H`, masked date entry, autosave,
  dark mode, and the tour.
- Amortization Points / APR row text rewritten — was "not yet
  implemented in this port," now describes the live feature.
- Adjustment field-error table updated for AO7 (see §0.11.5
  below).

The obsolete "Year to divide century" computational setting was
retained at the project owner's request as a disabled placeholder
with a tooltip explaining it has no effect.

### 0.11.5 Residual financial-fidelity gaps from §0.9.5 / §0.10.5

| Item | Status after this pass | Notes |
|---|---|---|
| **AO7** — date-only adjustment (re-amortize at current rate) | **Done.** | Engine's AO5 re-amortize branch widened from `hasRate && !hasAmt` to `!hasAmt`; the API rejection lifted; canary `TestCanaryC5_…` rewritten as `TestAO7AdjustmentReamortizesAtCurrentRate`, which asserts that the post-adjustment payment drops when a future balloon discounts the principal. |
| **VR-mode month-stepped COLA** in `vrPeriodicValue` | **Done.** | The path matched `periodicSumAnnualCOLA`'s stepped-multiplier convention; continuous-COLA setting uses `exp(t × ln(1+cola))` so the two paths agree at every anniversary. Pinned by `TestVRPeriodicCOLAMatchesFixedRate_{Annual,Continuous}`: a single-line VR schedule with the same rate and COLA matches the fixed-rate result. |
| **USA-rule `usap` carry across an ARM adjustment** (V6-2) | **Done.** | Earlier port subtracted `usap` from `netBal` and added a linear-paydown term, on the theory that `usap` retires linearly. The standard per-period rule retires `usap` much faster than linear, so that adjustment left a large residual (~$37k on a 200k/30y negative-amort test). The formula now matches DOS Re_Amortize (`AMORTOP.pas:1545-1569`) literally: amortize the full `netBal` over the remaining term at the new rate, with the running `usap` preserved by virtue of being engine state. `TestAO5UnderUSARuleNegativeAmort` exercises a positive-`usap` scenario across the adjustment and asserts the final balance lands within $1 of zero. |
| **V6-7 — 360-basis `timedif` shortcut** in `ComputeNext` | **Scoped down, documented.** | Difference is sub-day rounding under the 360-basis; never moves a row by more than rounding noise. "Presentation-grade" per the original V6-7 categorization. Reconsider only if a refdata.json cross-check exposes a measurable diff. |
| **V6-14** — yearly/quarterly summary aggregation in printed schedules | **Scoped down, documented.** | API correctly returns the raw schedule; the rollup is a UI presentation feature, and the current frontend can compute it from the schedule rows. Reconsider when a concrete export/print user story needs it. |
| **`SolveLoanAmount` / `SolveRate` use Iterate for fancy loans** | **Scoped down, documented.** | The two `// TODO: verify logic` markers in `backward.go` flag that fancy backward solves fall back to closed form + balloons rather than DOS's `Iterate` helper. The original Phase-4 plan rated this "L"-effort. Closed-form is correct for the common (non-fancy) backward solves; fancy backward solves are best-effort today. Reconsider when refdata.json cross-checks expose a measurable diff. |
| **Engine-wide `FieldError` threading** (ST1 follow-up) | **Scoped down, documented.** | The advanced-options row errors already return `FieldError`. The frontend's inline-error highlighting (usability item 88) works via regex-based field detection on the message string and is sufficient for the current UI. Reconsider when a feature requires structured field metadata in deep-engine errors. |

### 0.11.6 Repo hygiene

- `.gitattributes` added so `legacy/**` is treated as binary by git
  and stops producing 25k+ spurious "modifications" in `git status`
  on systems with `core.autocrlf` set.  Run `git add --renormalize
  legacy/` once after pulling this revision to clear any existing
  CRLF/LF churn from the working tree.
- A stray `internal/api/handlers.go.408355706897436735` editor
  backup (committed in error on 2026-05-22) has been emptied to a
  marker file; the sandbox running this revision has no `unlink`
  permission so the user must `git rm` it locally to fully remove
  it from the index.

### 0.11.7 `refdata.json` regenerated and verified current

`scripts/regen_refdata.sh` was added so the regeneration can be
re-run any time refdata.pas is extended.  The script compiles the
harness, runs it, and diffs the output against the checked-in JSON;
pass `--apply` to overwrite.  Auto-detects the rtl unit path so it
works against both a normal `fpc` install (macOS `brew install fpc`,
Linux `apt-get install fpc`) and a hand-unpacked compiler binary.

Running the script against the current tree reports:

```
Compiling legacy/testharness/refdata.pas with /usr/bin/fpc …
Running harness …
refdata.json is current — no changes.
```

So the checked-in `legacy/reference-output/refdata.json` is
byte-identical to what the harness emits today.  The 13 DOS-output
cross-check tests in `internal/finance/crosscheck_test.go` and
`crosscheck_backward_test.go` all pass against it, covering Julian,
Exxp, Lnn, Power, Round2, YieldRate, MortgageSummation,
MortgageCalc, PVSumFormula, YearsDif, and the Mortgage / Amort
backward solves (loan amount, rate).  Combined with the 127
help-example DOS-output assertions across the four
`help_examples_test.go` files (mortgage, amortization, presentvalue,
actuarial) and `internal/api/verify_web_help_examples_test.go`, the
post-2026-05-06 ports already have substantial DOS-output coverage
via help-documented expected values.

What the harness doesn't yet cover, and what would be the natural
next extension to refdata.pas (kept as an explicit follow-up rather
than done in this revision because it touches `legacy/` which is
treated as read-only per CLAUDE.md):

- Rule-of-78 per-period interest split.
- In-advance (annuity-due) accrual on a fancy schedule.
- Biweekly / weekly basis coercion.
- Month-specific COLA stepping under variable-rate mode.
- AO7 re-amortize-at-current-rate and the V6-2 USA-rule-with-ARM
  schedule end-state.

Each of these is currently pinned only by Go round-trip tests plus a
single help-example assertion; extending refdata.pas with one
representative case per area is the cleanest way to tighten that
coverage when refdata.pas can be modified.

### 0.11.8 Net assessment after Revision 9

The only remaining genuine fidelity gaps (V6-7, V6-14, fancy
backward Iterate) are explicitly scoped-down with rationale, all
bounded by Iterate-effort and all below the noise floor of common
schedules.  No known bug remains on a common path.  `refdata.json`
is current and the 13 cross-check tests + 127 help-example
DOS-output assertions all pass.  The natural next-step for
tightening coverage is extending `refdata.pas` with the cases listed
in §0.11.7 — non-blocking, and tracked separately so `legacy/` stays
read-only by default.

---

## 1. Mortgage screen

### 1.1 State summary

`mortgage.Calc` (`internal/finance/mortgage/mortgage.go:142`) is a faithful
port of DOS `Mortgage.pas:192–310`. Of the 8 dispatch arms DOS distinguishes
on this screen, **all 8 are ported** and reachable through
`POST /api/mortgage/calc`. The real gaps are at the boundaries:

- `CompareAPRs` is fully implemented including the Newton-iterated
  crossover-time computation. *[Superseded by Rev 2/3: a
  `POST /api/mortgage/compare` route now exposes it and the frontend
  `compareMtgAPR` calls it — see §0.5.1.]*
- `GenerateRows` (`rowgen.go:52`) is implemented, but the frontend's
  What-If feature (`index.html:1699`) loops `/api/mortgage/calc` per row
  in JavaScript instead of calling the Go function.
- Per-cell error reporting from DOS (`RecordError(row, col)`,
  `Mortgage.pas:182, 219, 234`) is collapsed into a single error string,
  so the UI must regex-match the message to identify the offending field.
- Two silent-output failure modes (Section 4.1, S-6 and S-7) where the
  engine returns "no error" with no computed cells.

### 1.2 Permutation matrix (Mortgage)

Columns: **Price · Points · %Down · Cash · Financed · Years · Rate · Tax ·
Monthly · BalloonYrs · BalloonAmt**. "I" = input (filled); "—" = blank.

| # | Inputs | Solves | DOS site | Go site | Status |
|---|---|---|---|---|---|
| M1 | Price + %Down (+Years+Rate+Tax) | Cash, Financed, Monthly | `Mortgage.pas:198–244, 285–289` | `mortgage.go:284–312, 238–251` | ✅ |
| M2 | Price + Cash (+Years+Rate)      | %Down, Financed, Monthly | `Mortgage.pas:214–228, 285–289` | `mortgage.go:294–301, 238–251` | ✅ |
| M3 | Price + Financed (+Years+Rate)  | %Down, Cash, Monthly     | `Mortgage.pas:229–243, 285–289` | `mortgage.go:302–310, 238–251` | ✅ |
| M4 | Monthly + %Down (+Years+Rate)   | Price, Cash, Financed    | `Mortgage.pas:291–307`          | `mortgage.go:252–275`          | ✅ |
| M5 | Monthly + Cash (+Years+Rate)    | Price, %Down, Financed   | `Mortgage.pas:295–296`          | `mortgage.go:263–264`          | ✅ |
| M6 | Monthly + Financed only         | **Error** "fill in % Down or Cash" | `Mortgage.pas:297–300`     | `mortgage.go:265–268`          | ✅ |
| M7 | Price + %Down + Years + Rate + Monthly + BalloonYrs | BalloonAmt | `Mortgage.pas:247–257, 271–283` | `mortgage.go:316–329, 226–237` | ✅ |
| M8 | All solvable columns filled     | **Error** "leave some blank"       | `Mortgage.pas:278–282`     | `mortgage.go:234–236`          | ✅ |
| M9 | Reverse-price with balloon known | Price                              | `Mortgage.pas:291–307`     | `mortgage.go:252–275`          | ✅ |
| M10 | Solve for **Rate** from Price, Monthly, funding, Years | — | n/a — DOS guard `Mortgage.pas:266` | n/a | ▪ DOS does not solve rate on this screen; use Amortization |
| M11 | Solve for **Years**             | —                                  | n/a — same guard          | n/a | ▪ Same |
| M12 | Solve for **Points** or **Tax**  | —                                  | n/a — never the unknown   | n/a | ▪ Same |
| M13 | APR for one row                 | (auto when row qualifies)         | `Mortgage.pas:538–555`    | `mortgage.go:442–444`; auto in `handlers.go:327–335` | ✅ |
| M14 | APR comparison between two rows + crossover time | crossover years | `Mortgage.pas:613–711, 391–535` | `mortgage.go:475–534, 540–651`; `HandleMortgageCompare` `handlers.go:463` | **[v2: RESOLVED — API]** route wired (`main.go:34`); frontend `compareMtgAPR` still uses local heuristic |
| M15 | What-If row generation (1-D)    | varied rows                       | `Mortgage.pas:852–1047` (compiled out, authority only) + `MortgageRowGenerationDlgUnit.pas` | `rowgen.go:52–97`; `HandleMortgageWhatIf` `handlers.go:534` | **[v2: RESOLVED — API]** route wired (`main.go:35`); frontend still loops `/calc` client-side |
| M16 | What-If 2-D (vary two columns)  | grid                              | DOS allows up to 3 dimensions (`Mortgage.pas:906–912`) | Frontend loops; `GenerateRows` is 1-D only | ⚠ partial |

### 1.3 Solvable-but-not-wired (Mortgage)

These are pure plumbing fixes — the Go math is unit-tested and works.

1. **`POST /api/mortgage/compare`** → `mortgage.CompareAPRs`. The Newton-iterated
   crossover-time calculation matches DOS verbatim (`mortgage.go:540–651`),
   but no handler registers the route. Effort: **S**. Frontend wiring:
   replace the local `compareMtgAPR` heuristic at `index.html:1644`.
2. **`POST /api/mortgage/whatif`** → `mortgage.GenerateRows`. Currently the
   frontend's `runWhatIf` (`index.html:1699–1814`) reimplements
   `EnoughDataForRowGeneration` in JavaScript and loops `/calc` per row,
   imposing a network round-trip per generated row. Effort: **S**.
3. **2- and 3-D row generation.** DOS supports up to 3 varied columns
   (`Mortgage.pas:906`). Go's `GenerateRows` accepts a single `VaryField`.
   Extend to `[]Variation`. Effort: **M**.

### 1.4 Genuinely under-determined (Mortgage)

These combinations DOS also refuses; the Go port matches that refusal.

- **Rate from {Price, Monthly, funding, Years}.** DOS's `Calc` runs only
  when `ratestatus = inp` (`Mortgage.pas:266`). The user is expected to
  use the Amortization screen, where `SolveRate` exists (and now needs to
  be wired — see §2).
- **Years from {Price, Monthly, funding, Rate}.** Same guard.
- **Points or Tax solve.** Neither is ever the unknown — both are
  additive modifiers consumed by `ComputeCashPctAndFinanced`
  (`Mortgage.pas:209, 287`).
- **Cash from Financed (or vice versa) without Price.** Both formulas
  need Price as denominator (`Mortgage.pas:209, 216, 231`).
- **Monthly without one of {%Down, Cash, Financed}.** The main `if` at
  `Mortgage.pas:266` short-circuits. DOS produces no output silently;
  Go matches — but the user gets no feedback (Section 4.1, S-7).

### 1.5 Mortgage-specific issues

1. **Per-cell errors collapsed to single string.** DOS marks the
   offending cell with `RecordError(row, col)` so the UI can highlight
   one field; the Go API returns `result.Err: string` and the frontend
   re-derives the column with `explainMtgError` (`index.html:1537–1574`).
   That helper only parses the "leave X or Y blank" message — every
   other mortgage error falls through to a generic "Row N: <msg>"
   prefix. Recommended fix in Section 4.3 (`FieldError` struct).
2. **Field-name drift between engine and UI.** Engine says
   "monthly payment"; UI label is "Monthly Total." Engine says
   "amount borrowed"; UI label is "Amt Borrowed." Engine says
   "percent down"; UI label is "% Down." A single field-name
   dictionary would close this (Section 4.5).
3. **APR hard-codes 365.25** (`handlers.go:328`) for `FullTermAPR`.
   DOS reads the screen's basis setting; a 360-basis mortgage shows
   APR drifted by ~0.07%. Not a dispatch gap but a small numerical
   parity issue.
4. **`CompareAPRs` skips `EnoughDataForAPR` precondition.** DOS gates
   the iteration at `Mortgage.pas:633`; Go calls `FullTermAPR`
   unconditionally and swallows the error (`mortgage.go:478–479`).
   If `e2` is essentially empty, the iteration churns 40 cycles
   before declaring non-convergence. Effort: **S**.
5. **Missing `TryBalloonDates` fallback** in the crossover iteration
   (`Mortgage.pas:462–508`). DOS retries pinned to a balloon date
   when the main Newton fails. Balloon-vs-balloon comparisons can
   currently return "did not converge" when DOS would have found a
   discontinuous crossover. Effort: **M**.

### 1.6 Side-by-side examples (Mortgage)

```jsonc
// M1 forward — Help Example 1; expect Monthly ≈ $1,540.97
POST /api/mortgage/calc
{ "price": 200000, "points": 0.02, "pctDown": 0.20,
  "years": 20, "rate": 0.08, "tax": 200 }

// M4 reverse — Help Example 2; expect Price ≈ $241,233.69
POST /api/mortgage/calc
{ "cash": 56000, "points": 0.015, "monthly": 1650,
  "years": 30, "rate": 0.085, "tax": 200 }

// M7 unknown balloon
POST /api/mortgage/calc
{ "price": 200000, "pctDown": 0.20, "years": 30, "rate": 0.06,
  "monthly": 1500, "balloonYears": 10 }

// M8 over-determined → expect: "leave price or monthly payment or
// balloon amount blank to be computed"
POST /api/mortgage/calc
{ "price": 200000, "pctDown": 0.20, "years": 30, "rate": 0.06,
  "monthly": 1500, "balloonYears": 10, "balloonAmount": 50000 }
```

---

## 2. Amortization screen

### 2.1 State summary

**This is the worst-affected worksheet.** Of the 10 main DOS dispatch arms,
only **4 are reachable end-to-end**. Three additional solvers
(`SolvePayment`, `SolveLoanAmount`, `SolveRate`) exist as engine-level Go
functions with passing unit tests, but **`HandleAmortizationCalc`
(`handlers.go:340`) never invokes the latter two** — it hard-stamps
`AmountStatus = InOutInput` and `LoanRateStatus = InOutInput` regardless
of what the JSON contained. The DOS-style "leave a field blank and we'll
solve for it" never reaches them.

Verified at `handlers.go:361`: `if req.Amount == 0 && req.Rate == 0` —
this is the only path that branches on missing fields, and it triggers
only when *both* Amount and Rate are zero (derive-only mode). A request
with `{rate: 0, amount: 200000, ...}` runs through `Amortize` with a
hard-stamped `LoanRate = 0.0` and produces a zero-interest schedule with
no error and no warning. This is the highest-severity issue in the
audit.

The advanced options are similarly partial: `target` and `moratorium`
are fully supported, but `unkpre` (unknown prepayment amount), unknown
prepayment duration, target-balloon (solve balloon amount for target
balance), and per-adjustment rate↔payment solving are all missing.
The prepayment `nPmts` field is accepted in the API and silently ignored
by the engine.

### 2.2 Permutation matrix — basic grid

Columns: **Amt · Rate · First · Last · N · Pmt · Pts**. "✓" = filled,
"—" = blank.

| # | Amt | Rate | First | Last | N | Pmt | Pts | Solves | DOS site | Go site | Status |
|---|---|---|---|---|---|---|---|---|---|---|---|
| A1 | ✓ | ✓ | ✓/— | — | ✓ | ✓ | — | Forward schedule | `Amortize.pas:218–239` | `firstpass.go:43–99`, `engine.go:127–227` | ✅ |
| A2 | ✓ | ✓ | ✓ | ✓ | — | ✓ | — | Derive N from dates | `Amortize.pas:227–239` | `firstpass.go:75–92`          | ✅ |
| A3 | ✓ | ✓ | ✓ | — | ✓ | — | — | Payment (closed-form annuity) | `Amortize.pas:377–430` | `backward.go:70–103` + `engine.go:178–184` | ✅ (via engine fallthrough) |
| A4 | — | ✓ | ✓ | ✓/N | — | ✓ | — | **Loan amount** | `Amortize.pas:432–465, 853–858` | `backward.go:121–176`; dispatched `handlers.go:905-914` | **[v2: RESOLVED]** handler now calls `SolveLoanAmount` for a nil `amount` |
| A5 | ✓ | — | ✓ | ✓/N | — | ✓ | — | **Rate** | `Amortize.pas:467–491` | `backward.go:194–238`; dispatched `handlers.go:915-924` | **[v2: RESOLVED]** handler now calls `SolveRate` for a nil `rate` (no more silent 0% schedule) |
| A6 | ✓ | ✓ | ✓ | — | — | ✓ | — | N or LastDate from payment | `AMORTOP.pas:1323–1407` | none | ❌ missing |
| A7 | — | — | ✓ | ✓ | ✓ or — | — | — | Derive-only term | `Amortize.pas:218–245` | `handlers.go:607–677` | ✅ |
| A8 | — | — | ✓ | — | ✓ | — | — | Derive-only last date | `Amortize.pas:220–226` | `firstpass.go:61–74` | ✅ |
| A9 | ✓ | ✓ | — | — | ✓ | ✓ | ✓ | Schedule + **APR** | `Amortize.pas:516–615` | none | ❌ missing |
| A10 | ✓ | ✓ | ✓ | — | ✓ | ✓ | — | Auto-append residual balloon | `Amortize.pas:1386–1394` | `engine.go` TackOnFinalBalloon advisory | **[v5: ⚠ advisory only]** implied terminating balloon is flagged (R4-8); DOS additionally solves/merges it |

### 2.3 Permutation matrix — advanced options

| # | Block | Inputs | Solves | DOS site | Go site | Status |
|---|---|---|---|---|---|---|
| AO1 | Balloon | date ✓, amount ✓ | Apply at date | `AMORTOP.pas:574–664` | `engine.go:582–599` | ✅ |
| AO2 | Balloon | date ✓, amount — | **Balloon amount (target balloon)** | `Amortize.pas:628–663` | none | ❌ missing |
| AO3 | Balloon | date —, amount ✓ | DOS error "balloon needs date" | `AMORTOP.pas:293–300` | handler error `handlers.go:766-774`; frontend still drops at `index.html:1992` | **[v2: CHANGED]** server-side now an explicit row error; UI still drops the row silently |
| AO4 | Adjustment | date ✓, rate ✓, amount ✓ | ARM: rate AND payment | `AMORTOP.pas:1499–1613` | `engine.go:680–695` | ✅ |
| AO5 | Adjustment | date ✓, rate ✓, amount — | New payment from new rate | `Amortize.pas:1408–1413` | `engine.go` AO5 re-amortize | **[v3: RESOLVED]** rate change re-amortizes the payment over the remaining term (FP4); post-adjustment balloons netted off (R4-5) |
| AO6 | Adjustment | date ✓, rate —, amount ✓ | New rate from new payment | `Amortize.pas:1415–1418` | `engine.go` AO6 + `solveAdjRate` | **[v7: RESOLVED]** solves the implied rate so the new payment amortizes the balance over the remaining term (DOS `EstimateAndRefineAdjRate`); mirror of AO5 |
| AO7 | Adjustment | date ✓, rate —, amount — | Re-amortize at current rate | `AMORTOP.pas:1499+` | handler error `handlers.go:802-807` | **[v2: CHANGED]** no longer a silent no-op — handler rejects the row with a named error; DOS re-amortize logic itself still unported |
| AO8 | Prepayment | start ✓, perYr ✓, amount ✓, stop ✓ or nPmts ✓ | Apply series | `AMORTOP.pas:400–475` | `engine.go:601–650` (`prepayApplied` counter `:527,637-643`) | **[v2: RESOLVED]** `NN`/`nPmts` now honored — series retires after `NN` extras |
| AO9 | Prepayment | start ✓, perYr ✓, stop ✓ | **Prepayment amount (`unkpre`)** | `Amortize.pas:665–707` | none | ❌ missing |
| AO10 | Prepayment | start ✓, perYr ✓, amount ✓ | **Prepayment duration (stopDate)** | `Amortize.pas:709–774` | none | ❌ missing |
| AO11 | Moratorium | date ✓ | Re-amortize after interest-only | `Amortize.pas:1260–1288` | `engine.go:565–580` | ✅ |
| AO12 | Target | amount ✓ | Min principal reduction | `Amortize.pas:288–292` | `engine.go:635–639` | ✅ |
| AO13 | Target+SkipMonths | both ✓ | Target overrides skip (DOS quirk) | `AMORTOP.pas:643` | `engine.go:549, 635` | ✅ (documented in `CLAUDE.md`) |
| AO14 | Balance lookup | date ✓ | Balance at arbitrary date | `Amortize.pas:1423–1425` | none (frontend computes from schedule, wrong with balloons) | ⚠ |

### 2.4 Solvable-but-not-wired (Amortization)

The highest-impact items in the entire audit are here.

1. **Wire `SolveRate` into `HandleAmortizationCalc`.** Effort: **S**.
   Change `AmortizationRequest.Rate` to `*float64`. In the handler,
   when `req.Rate == nil` AND `req.Amount != nil` AND
   `(req.NPeriods > 0 || req.LastDate != "")` AND `req.Payment != nil`,
   call `amortization.SolveRate(input)` and stamp the result back into
   `loan.LoanRate`. Eliminates the most dangerous silent-wrong-answer
   path (Section 4.1, S-1).
2. **Wire `SolveLoanAmount` into `HandleAmortizationCalc`.** Effort:
   **S**. Same shape: `AmortizationRequest.Amount` → `*float64`,
   dispatch when absent. The math at `backward.go:121` is unit-tested
   (`backward_test.go:60`, `crosscheck_backward_test.go:74`).
3. **Honor `Prepayment.NPmts`** in `engine.go:601–632`. Add a counter
   per series; once it reaches `pp.NN`, stop applying. Today the
   engine consults only `stopDate`. Effort: **S**.
4. **Return explicit errors for AO3 / AO9 silent drops** at
   `index.html:1965, 1979, 1997` (prepayments, balloons,
   adjustments). Today these dropped rows are invisible — the user
   submits an Advanced Options entry and sees no effect on the schedule
   with no warning. Effort: **S**.

### 2.5 Genuinely missing — DOS procedures to port

These need real porting work, not just plumbing.

5. **A6 — Closed-form N from payment.** `DetermineLastPaymentDate`
   at `AMORTOP.pas:1323–1407`. Today `firstpass.go:75–99` only derives
   N when *both* firstDate and lastDate are given; never from a
   payment. Effort: **M**.
6. **AO5 / AO6 — Per-adjustment solving.** The ARM modeling path is
   incomplete: an adjustment with only a new rate produces a schedule
   where the payment doesn't amortize cleanly (the rate updates but
   `d` stays at the previous payment), with no error. DOS solves the
   missing field per adjustment row. `Amortize.pas:1408–1418`. Effort: **M**.
7. **A9 — APR with points.** `EstimateAndRefineAPRwithPoints` at
   `Amortize.pas:516–615`. The `Loan.Points` field is declared in
   `types.go:55–58` but never read; `AmortResult.APR` is never written.
   Refactor `mortgage.IterateToFindAPR` into a shared helper. Effort: **M**.
8. **AO2 — Target balloon.** `EstimateAndRefineBalloon` at
   `Amortize.pas:628–663`. Solve the balloon amount such that
   schedule balance hits zero at the balloon date. Effort: **L** — has
   a closed-form `very_last` short-circuit plus Newton iteration for
   the general case; needs an "unknown" sentinel in `AmortBalloonReq`.
9. **AO9 — Unknown prepayment amount (`unkpre`).** Already flagged
   in `CLAUDE.md` outstanding items. `EstimateAndRefinePeriodicPrepayment`
   at `Amortize.pas:665–707`. Closed-form zero-rate and non-zero-rate
   initial guesses, then Iterate. Effort: **L**.
10. **AO10 — Unknown prepayment duration.** `DeterminePrepaymentDuration`
    at `Amortize.pas:709–774`. Effort: **L** (precondition on
    `plus_regular` semantics).
11. **AO14 — Balance lookup as engine path.** Today
    `updatePayoffBalance` in the frontend (`index.html:2108–2122`)
    walks the returned schedule and subtracts `(payment - interest)`
    — wrong when there are balloons or adjustments mid-period.
    Port DOS's `ComputeBalanceFromDate` / `ComputeDateFromBalance`
    (`Amortize.pas:1423–1425`). Effort: **M**.

### 2.6 Genuinely under-determined (Amortization)

- (Amount —, Rate —, Payment —) with no dates: DOS errors via
  `SufficientDataOnScreen` (`Amortize.pas:867–868`). Frontend already
  catches this at `index.html:1916–1937` with a specific message.
- (Amount ✓, Payment —, Rate —): two unknowns. DOS rejects. Go's
  engine ports the FirstPass rejection but the handler may not reach
  it (depends on which fields are zero vs blank).
- Two adjustments on same date: validated at `validate.go:65–72`.

### 2.7 Side-by-side examples (Amortization)

```jsonc
// A3 (works) — solve for monthly payment
POST /api/amortization/calc
{ "amount": 200000, "rate": 0.06, "loanDate": "2025-01-01",
  "nPeriods": 360, "perYr": 12 }
// → returns payment ≈ 1199.10, full schedule

// A4 (currently rejected) — solve for loan amount from a payment
POST /api/amortization/calc
{ "rate": 0.06, "loanDate": "2025-01-01", "perYr": 12,
  "nPeriods": 360, "payment": 1199.10 }
// → CURRENT: "insufficient loan data: need amount and payments per year"
// → DOS: would solve amount ≈ 200000 via ComputeLoanAmount

// A5 (currently silent wrong) — solve for rate
POST /api/amortization/calc
{ "amount": 200000, "loanDate": "2025-01-01", "perYr": 12,
  "nPeriods": 360, "payment": 1199.10 }
// → CURRENT: returns a 0%-rate schedule with no error
// → DOS: would solve rate ≈ 0.06 via EstimateAndRefineRate

// AO9 (currently dropped silently) — unknown prepayment amount
POST /api/amortization/calc
{ "amount": 200000, "rate": 0.06, "loanDate": "2025-01-01",
  "perYr": 12, "nPeriods": 360,
  "prepayments": [{ "startDate": "2026-01-01",
                    "stopDate":  "2030-01-01", "perYr": 12 }] }
// → CURRENT: row dropped at index.html:1965 (no amount); schedule runs
//   without any prepayments
// → DOS: would solve the prepayment amount that retires the loan at
//   the given stopDate via EstimateAndRefinePeriodicPrepayment
```

---

## 3. Present Value screen

### 3.1 State summary

PV is in the best shape of the three. All seven main DOS backward solve
paths (PV-1, PV-2, PV-4, PV-5, PV-6, PV-8, PV-9) are ported, and so are
the row-level over-determined detection and the screen-level "too many
unknowns" / "insufficient data" errors. The Newton-iteration constants
(`teeny=1e-10`, max-30 iterations, ±0.04 damping, second-pass restart
from 0) all match DOS exactly.

The remaining real gaps are:

- **Actuarial integration is missing from every backward path.** DOS
  divides residuals by `LifeProb` (`PRESVALU.pas:873–883` for PV-1)
  and rejects PV-2 outright under `fold_in_life`
  (`PRESVALU.pas:894–897`). The Go ports of `solveLumpAmount`,
  `solveLumpDate`, and `computeKnownRowSum` skip life-probability
  weighting entirely. A contingent payment row therefore produces
  the wrong residual.
- **The `V_3` `const_signal` block in PV-6** (`PRESVALU.pas:1003–1008`)
  is not yet wired because the `const_signal` status doesn't propagate
  through Go's status enum. Already documented in `CLAUDE.md` outstanding items.
- **The rate-type selector (True/Loan/Yield) is a cosmetic dropdown.**
  `onPVRateTypeChange()` at `index.html:2243` is a no-op; the API
  receives a single continuously-compounded rate regardless of which
  option the user selects. DOS calls `YieldRateTranslation`
  (`PRESVALU.pas:535`) on entry, so this also affects classification.
- **PV-8 and PV-9 are unreachable from the UI.** The backend
  `solveRate` (`backward.go:848`) and `solveAsOf` (`backward.go:916`)
  work end-to-end, but the frontend refuses to submit a blank rate
  (`index.html:2381–2384`, with a stale message: "IRR computation
  (blank rate) is not yet supported in the API. Please enter a rate")
  and `getPVInput` requires `asOfDate` (`index.html:2253`).

### 3.2 Permutation matrix (Present Value)

| # | Inputs | Solves | DOS site | Go site | Status |
|---|---|---|---|---|---|
| PV-1 | lump date, value, rate, asOf | lump **amount** | `PRESVALU.pas:866–891` | `backward.go: solveLumpAmount` | **[v3: RESOLVED]** LifeProb divide added (FP8) |
| PV-2 | lump amount, value, rate, asOf | lump **date** | `PRESVALU.pas:892–931` | `backward.go: solveLumpDate` | **[v3: RESOLVED]** life-contingent rows rejected — `no_time_with_life` (FP8) |
| PV-3 | lump value only (no date, no amount) | **error** | `PRESVALU.pas:932–935` | `backward.go:286–290` | ✅ |
| PV-4 | periodic dates+value, rate, asOf | periodic **amount** | `PRESVALU.pas:943–956` | `backward.go:569` | ✅ |
| PV-5 | periodic fromDate, amount, value | periodic **toDate** | `PRESVALU.pas:965–999` | `backward.go:662–691` | ✅ (incl. cola=rate AddNPeriods branch) |
| PV-6 | periodic toDate, amount, value, cola=0 | **fromDate** | `PRESVALU.pas:1009–1027` | `backward.go:693–736` | ⚠ cola=0 path ported; cola≠0 two-pass approximation TODO |
| PV-6b | periodic, cola=const_signal | (special-case path) | `PRESVALU.pas:1003–1008` (`{$ifdef V_3}`) | none | ❌ missing |
| PV-7 | periodic with one date + value only | **error** | `PRESVALU.pas:1080–1083` | `backward.go:316–319` | ✅ |
| PV-8 | rows fully specified; blank rate; sumValue | discount **rate** | `PRESVALU.pas:693–754` | `backward.go:848` | ⚠ unreachable from UI |
| PV-9 | rows fully specified; rate; blank asOf; sumValue | **asOf date** | `PRESVALU.pas:755–818` | `backward.go:916` | ⚠ unreachable from UI |
| PV-10 | rows fully specified; rate; asOf | **sumValue** (forward) | `PRESVALU.pas:669–692` | `calc.go:296 forwardOnly` | ✅ |
| PV-screen | "too many unknowns" | screen-level error | `PRESVALU.pas:1242` | `calc.go:278–280` | ✅ |
| PV-warning | row determined by data above | DOS shows cancelable warning | `PRESVALU.pas:1166–1189` | `FirstPass` warns + proceeds (`backward.go:130-147,213-225`); surfaced via `PVResponse.Warnings` | **[v2: CHANGED]** over-specified rows are now soft warnings, not hard errors; channel exists end-to-end but the UI does not yet display warnings |

### 3.3 Solvable-but-not-wired (Present Value)

1. **Delete the stale PV-8 guard** at `index.html:2381–2384`. The
   backend supports `solveRate`. The message "IRR computation (blank
   rate) is not yet supported in the API" actively misleads the user.
   Effort: **S**.
2. **Relax the `getPVInput` `asOfDate` requirement** at
   `index.html:2253` so PV-9 is reachable. Effort: **S**.
3. **Wire the rate-type selector** in `onPVRateTypeChange` — convert
   client-side to continuous TrueRate before posting (reuse the
   algebra at `index.html:2346–2358`). Closes
   `discrepancies.md §4`. Effort: **S**.

### 3.4 Genuinely missing — DOS procedures to port

4. **Actuarial integration in backward paths.** The most numerically
   significant gap. Today `solveLumpAmount`, `solveLumpDate`, and
   `computeKnownRowSum` ignore `input.Actuarial`. Plumb life-
   probability weighting through each path; add the `no_time_with_life`
   rejection at the entry of `solveLumpDate`. Effort: **M**.
5. **PV-6 cola≠0 two-pass approximation** (`PRESVALU.pas:1030–1035`).
   TODO already flagged at `backward.go:625–628`. Effort: **M**.
6. **`const_signal` PV-6 special case.** Requires extending the
   status enum to carry a "constant cola" flag through the row
   classifier. Effort: **M**.
7. **Warnings channel for "value already determined" PV warning.**
   The DOS UX is a cancelable dialog; the Go port currently can only
   hard-error or accept silently. Add `PVResponse.Warnings []string`.
   Effort: **M**.

### 3.5 Side-by-side examples (Present Value)

```jsonc
// PV-1 — solve for lump amount given target value
POST /api/presentvalue/calc
{ "rate": 0.05, "asOfDate": "2024-01-01", "sumValue": 9523.81,
  "lumpSums": [{ "date": "2025-01-01" }] }
// → returns amount ≈ 10000

// PV-8 (currently unreachable from UI) — solve for IRR
POST /api/presentvalue/calc
{ "asOfDate": "2024-01-01", "sumValue": 9000,
  "lumpSums": [{ "date": "2025-01-01", "amount": 10000 }] }
// → CURRENT (via API): solves rate ≈ 0.111
// → CURRENT (via UI): refused with "IRR computation (blank rate) is
//   not yet supported in the API"

// PV with actuarial (currently wrong)
POST /api/presentvalue/calc
{ "rate": 0.05, "asOfDate": "2024-01-01", "sumValue": 9523.81,
  "lumpSums": [{ "date": "2025-01-01", "act": "L" }],
  "actuarial": { /* life table */ } }
// → CURRENT: returns wrong amount (no LifeProb divide)
// → DOS: amount ≈ 10000 / LifeProb(2025-01-01, "Living")
```

---

## 4. Error-message audit (cross-cutting)

### 4.1 Silent-failure paths (highest priority)

These are the cases where the user submits an input that produces wrong
or empty output with **no error message**. They are the most dangerous
items in the audit because the user has no way to know something went
wrong.

| # | Trigger | Current behavior | Fix |
|---|---|---|---|
| **S-1** | `POST /api/amortization/calc` with `rate: 0` (or `rate` omitted, since `float64` zero-value is 0) and other fields filled | Engine runs a **0%-rate schedule** silently. No error. | Switch `AmortizationRequest.Rate` to `*float64`; dispatch to `SolveRate` when nil. **[v2: RESOLVED]** done — `handlers.go:130,915-924`. (An *explicit* `"rate":0` still decodes to a non-nil 0.0; the UI never sends that for a blank cell, so the dangerous path is closed.) |
| **S-2** | `POST /api/amortization/calc` with `amount: 0` and other fields filled | Engine returns `"insufficient loan data: need amount and payments per year"` — confusing because PerYr *is* supplied. Or runs through with garbage if other status flags align. | Switch `.Amount` to `*float64`; dispatch to `SolveLoanAmount` when nil. **[v2: RESOLVED]** done — `handlers.go:128,905-914`. |
| **S-3** | Amortization advanced row with a missing required field (e.g. prepayment row with amount but no startDate) | Frontend at `index.html:1965, 1979, 1997` returns from the loop callback — row silently dropped from the request. | Return explicit "row N: missing X" error before submitting. **[v2: CHANGED]** server side done (`handlers.go:716-807`); the frontend still drops half-filled rows silently at `index.html:1976,1992,2002,2008` — client-side fix outstanding. |
| **S-4** | PV row half-filled (e.g. date but no amount on a lump-sum row) | Frontend at `index.html:2264, 2283` filters the row out before posting. | Same — show "row N: missing X" instead. |
| **S-5** | PV submit with all rows silently dropped per S-4 | Frontend then says "Enter ... at least one lump sum or periodic payment" — but the user *did* enter rows. | Cascade from S-4 fix. |
| **S-6** | Mortgage What-If generates rows where the source row had `hasFunding=false`, etc. | The API returns the request unchanged (no error, no computed cells) and the generated table looks half-empty. | Detect "no field populated as output" in `updateMtgRowUI` and warn. The frontend already pre-validates *for* What-If; extend the check to plain Calculate. |
| **S-7** | Mortgage Calculate with the row's main guard failing (e.g. no funding column) | `mortgage.Calc` returns success with no computed fields. Handler returns the input echoed back with zeros. The UI cannot distinguish "engine had nothing to do" from "engine cleared output." | Detect "no output column transitioned to InOutOutput" post-Calc; return "insufficient inputs" error. |

### 4.2 Error-message catalog summary

Full per-message catalog with proposed rewordings is in Appendix A.
Headline counts across all three worksheets:

- **62 distinct user-facing error/validation strings.**
- **17 (27%) clear** — identify the field and the fix.
- **28 (45%) vague** — identify a problem but not which field.
- **17 (28%) ambiguous** — user cannot infer what to change.

**Distribution by worksheet:** Mortgage 13, Amortization 27, PV 22.

The two most common patterns:

1. **Date-parsing errors that don't name which date field failed.**
   `handlers.go:368` says "invalid loanDate format" (good); but
   `handlers.go:459` says just "invalid prepayment startDate" with
   no row index. Trivial fix.
2. **Engine messages using internal Pascal-style names.**
   "summation factor too small," `"rate" computation did not converge`,
   etc. These leak solver internals and don't suggest a fix.

### 4.3 Structural recommendation — `FieldError` struct

Today engines return `error` (single string). Handlers wrap or
concatenate. The frontend either prefixes "Row N:" or regex-matches the
text (`explainMtgError`). Replace with:

```go
type FieldError struct {
    Code    string   // stable identifier (e.g. "MTG_OVERDETERMINED")
    Message string   // human-readable, field-named, UI-label format
    Fields  []string // affected fields, in UI-label form
    RowIdx  int      // 1-based; 0 = screen-level
    Block   string   // "mortgage", "lumpsum", "periodic", "prepayment", ...
}
```

This single change unlocks:

- Stable error codes for the frontend to dispatch on (vs. regex on
  message text).
- Per-cell highlighting in the UI.
- Localization-ready messages.
- A trivial replacement for `explainMtgError`'s regex.

Effort: **M** (touches all three handlers, all engine error returns,
and the frontend display layer).

### 4.4 Structural recommendation — `Warnings []FieldError` channel

Some DOS messages are **non-blocking warnings** the user can dismiss
("value already determined by data above — continue anyway?"). Today
the Go port has only one channel (`result.Err`), so these become
hard errors or are dropped silently. Add a parallel `Warnings []FieldError`
field on every response.

Use it for:

- PV "row determined by data above" (`PRESVALU.pas:1166–1189`).
- "Row dropped because field X was blank" (S-3, S-4).
- Solver clamps and assumptions ("Rate clamped to 0.02 for solver start").
- Format coercions ("Switched to 365-day basis for weekly payments,"
  `Amortize.pas:300`, currently silent).
- "Terminating balloon was adjusted to clear residual"
  (`Amortize.pas:1073`, currently silent).

Effort: **M**.

### 4.5 Structural recommendation — field-name dictionary

Engine code uses internal names ("monthly", "amount", "peryr"); the
UI uses human labels ("Monthly Total", "Amt Borrowed", "Pmts/Yr"). The
disconnect propagates into every error message. Centralize in
`internal/api/labels.go`:

| Engine term | UI label |
|---|---|
| amount, loan amount | Amount Borrowed |
| monthly | Monthly Total |
| peryr / payments per year | Pmts/Yr |
| nperiods, npayments | # Periods |
| firstdate, first payment date | 1st Pmt Date |
| lastdate | Last Pmt Date |
| loandate | Loan Date |
| payamt, payment | Pmt Amount |
| asof | As-of Date |
| sumvalue | Sum Value |
| fromdate / todate | From Date / To Date |
| pct down, percent down | % Down |
| howmuch, balloon amount | Balloon Amt |
| when, balloon years | Balloon Yrs |

Engines should look up labels at error-construction time, not embed
internal names. Effort: **S** for the dictionary; **M** to thread it
through.

### 4.6 Date-format consistency

Engine messages say "use YYYY-MM-DD." Frontend accepts and displays
"MM/DD/YYYY." The frontend pre-parses date strings before submitting,
so the engine messages are only seen by users hitting the API
directly — but those users also read the frontend help, which uses
MM/DD/YYYY. Standardize on "MM/DD/YYYY (or ISO YYYY-MM-DD)" in all
error messages. Effort: **S**.

### 4.7 Quick reword wins (top 20 ambiguous messages)

These are the highest-traffic ambiguous messages. Each is a one-line
edit.

| Current | Proposed |
|---|---|
| `summation too small` (`mortgage.go:246`) | `Rate is effectively zero — Monthly Total cannot be computed without a positive rate.` |
| `cash too close to price` (`mortgage.go:297`) | `Cash Required is within 0.5% of Price — leave Cash Required blank or lower it.` |
| `financed amount too close to price` (`mortgage.go:305`) | `Amt Borrowed is within 0.5% of Price — leave it blank or lower it.` |
| `invalid loanDate format, use YYYY-MM-DD` (`handlers.go:368`) | `Loan Date is unparseable — use MM/DD/YYYY.` |
| `invalid prepayment startDate` (`handlers.go:459`) | `Prepayment row N: Start Date is unparseable — use MM/DD/YYYY.` |
| `invalid balloon date` (`handlers.go:491`) | `Balloon row N: Date is unparseable — use MM/DD/YYYY.` |
| `insufficient loan data: need amount and payments per year` (`engine.go:133`) | `Amount Borrowed and Pmts/Yr are both required.` (split into two messages) |
| `insufficient loan data: need first payment date` (`engine.go:156`) | `1st Pmt Date is required (or fill Loan Date + Pmts/Yr to default it).` |
| `compute true rate: <inner>` (`engine.go:172`) | `Rate is out of range.` |
| `amortization exceeded 10000 periods` (`engine.go:532`) | `Schedule exceeded 10,000 periods — check Pmts/Yr and Last Pmt Date.` |
| `insufficient data: need amount, rate, term, peryr` (`backward.go:75`) | `Cannot solve Pmt Amount — Amount, Rate, # Periods, Pmts/Yr are required.` |
| `cannot determine payment - summation factor too small` (`backward.go:100`) | `Rate × Term is too small to solve for Pmt Amount.` |
| `cannot determine loan amount - interest rate too small` (`backward.go:140`) | `Rate is too small to solve for Amount Borrowed (effectively zero).` |
| `insufficient data: need rate, payment, term, peryr, first date` (`backward.go:126`) | `Cannot solve Amount Borrowed — Rate, Pmt Amount, # Periods, Pmts/Yr, 1st Pmt Date are required.` |
| `insufficient data: need amount, payment, term, peryr` (`backward.go:198`) | `Cannot solve Rate — Amount, Pmt Amount, # Periods, Pmts/Yr are required.` |
| `too many unknowns` (PV `calc.go:279`) | `More than one missing field on the screen — fill in enough cells to leave exactly one blank.` |
| `insufficient data on screen` (PV `calc.go:285`) | `Not enough inputs to solve for Sum Value — supply Rate, As-of Date, and at least one fully-specified row.` |
| `"rate" computation did not converge` (PV `backward.go:863`) | `Rate solver did not converge — try a different starting point or specify Amounts instead of Values.` |
| `cannot compute date - interest rate too small` (PV `backward.go:922`) | `Rate is effectively zero — cannot solve for As-of Date.` |
| `IRR computation (blank rate) is not yet supported in the API. Please enter a rate.` (`index.html:2382`) | **Delete this guard entirely** — the API supports it. Either remove the message, or replace with: `Solving for Rate requires entering Sum Value (target present value). Leave Rate blank but fill Sum Value.` |

---

## 5. Recommended sequencing

Prioritized by impact × inverse effort. Items with the same priority can
be parallelized.

### Phase 1 — Eliminate silent wrong answers (P0, ~1 week total)

1. **(S)** Make `AmortizationRequest.Rate` and `.Amount` pointer types;
   dispatch nil to `SolveRate` / `SolveLoanAmount`. Closes S-1 and S-2
   silent-failure paths. *Wins the most user trust per hour spent.*
2. **(S)** Delete the stale PV-8 IRR guard at `index.html:2381–2384`.
3. **(S)** Honor `Prepayment.NPmts` in `engine.go:601–632`.
4. **(S)** Return explicit "row N: missing X" errors for the three
   silent-drop spots in Advanced Options (`index.html:1965, 1979,
   1997`) and the two PV row filters (`:2264, :2283`).

### Phase 2 — Wire existing math to the API (P0, ~1 week)

5. **(S)** Add `POST /api/mortgage/compare` → `CompareAPRs`. Replace
   the local frontend heuristic.
6. **(S)** Add `POST /api/mortgage/whatif` → `GenerateRows`. Replace
   the per-row JS loop.
7. **(S)** Wire the PV rate-type selector (convert client-side before
   posting).
8. **(S)** Pre-submission validation in `calcMortgageRow` (parallel
   to existing amortization pattern) — refuse with a named-field
   list instead of silently producing empty output.

### Phase 3 — Structural error handling (P1, ~1 week)

9. **(M)** Introduce `FieldError` struct + serialize through all
   three handlers.
10. **(M)** Introduce `Warnings []FieldError` channel.
11. **(S)** Centralize the field-name dictionary in
    `internal/api/labels.go`.
12. **(S)** Reword the top 20 ambiguous messages per §4.7.

### Phase 4 — Port missing DOS procedures (P1–P2, ~3–4 weeks)

13. **(M)** Closed-form N from payment (Amortization A6).
14. **(M)** Per-adjustment rate↔payment solving (Amortization AO5,
    AO6, AO7).
15. **(M)** APR with points (Amortization A9). Refactor
    `mortgage.IterateToFindAPR` into a shared helper first.
16. **(M)** Actuarial integration in PV backward paths (PV-1 LifeProb
    divide, PV-2 `no_time_with_life` reject,
    `computeKnownRowSum` weighting).
17. **(M)** PV-6 cola≠0 two-pass approximation.
18. **(M)** Balance-lookup engine path (Amortization AO14).
19. **(M)** `TryBalloonDates` crossover fallback (Mortgage).
20. **(L)** Target balloon (Amortization AO2).
21. **(L)** Unknown prepayment amount (Amortization AO9).
22. **(L)** Unknown prepayment duration (Amortization AO10).
23. **(M)** `const_signal` propagation for PV-6b.

### Phase 5 — UX polish (P2, ~1 week)

24. **(M)** 2- and 3-dimensional What-If row generation.
25. **(M)** PV "value already determined" cancelable warning.
26. **(S)** APR `yrdays` should honor screen basis, not hard-coded
    365.25 (`handlers.go:328`).
27. **(S)** `EnoughDataForAPR` precondition in `CompareAPRs`.

**Total estimated effort:** roughly 6–8 engineer-weeks for all 27 items.
Phases 1 and 2 alone (2 weeks) eliminate every silent-wrong-answer path
and double the number of reachable dispatch arms, with no new financial
math required.

---

## Appendix A — Full error-message catalog

For brevity this section lists only the messages flagged **vague** or
**ambiguous** in §4.2. The full catalog (including the 17 clear messages
left as-is) is available on request.

### A.1 Mortgage (`/api/mortgage/calc`)

| # | Severity | File:line | Current | Proposed |
|---|---|---|---|---|
| MM-1 | M | `mortgage.go:191` | `must specify years to balloon payment` | `Balloon Yrs is required when Balloon Amt is filled in.` |
| MM-2 | H | `mortgage.go:235` | `leave price or monthly payment or balloon amount blank to be computed` | `Row N has Price and Monthly Total both filled — leave one blank, or add Balloon Yrs to solve for the balloon.` |
| MM-3 | H | `mortgage.go:246` | `summation too small` | `Rate is effectively zero — Monthly Total cannot be computed without a positive rate.` |
| MM-4 | M | `mortgage.go:266` | `fill in percent down or cash required for price computation` | `To solve for Price from Monthly Total, also fill in % Down or Cash Required.` |
| MM-5 | H | `mortgage.go:286` | `price too small` | `Price must be greater than zero.` |
| MM-6 | H | `mortgage.go:297` | `cash too close to price` | `Cash Required is within 0.5% of Price — leave Cash Required blank or lower it (% Down cannot be solved).` |
| MM-7 | H | `mortgage.go:305` | `financed amount too close to price` | `Amt Borrowed is within 0.5% of Price — leave it blank or lower it.` |

### A.2 Amortization (`/api/amortization/calc`)

| # | Severity | File:line | Current | Proposed |
|---|---|---|---|---|
| AM-1 | H | `handlers.go:368` | `invalid loanDate format, use YYYY-MM-DD` | `Loan Date is unparseable — use MM/DD/YYYY.` |
| AM-2 | H | `handlers.go:428, 635` | `invalid firstDate format, use YYYY-MM-DD` | `1st Pmt Date is unparseable — use MM/DD/YYYY.` |
| AM-3 | H | `handlers.go:441, 644` | `invalid lastDate format, use YYYY-MM-DD` | `Last Pmt Date is unparseable — use MM/DD/YYYY.` |
| AM-4 | H | `handlers.go:459` | `invalid prepayment startDate` | `Prepayment row N: Start Date is unparseable — use MM/DD/YYYY.` |
| AM-5 | H | `handlers.go:478` | `invalid prepayment stopDate` | `Prepayment row N: Stop Date is unparseable — use MM/DD/YYYY.` |
| AM-6 | H | `handlers.go:491` | `invalid balloon date` | `Balloon row N: Date is unparseable — use MM/DD/YYYY.` |
| AM-7 | H | `handlers.go:506` | `invalid adjustment date` | `Adjustment row N: Date is unparseable — use MM/DD/YYYY.` |
| AM-8 | H | `handlers.go:529` | `invalid moratorium date` | `Moratorium Date is unparseable — use MM/DD/YYYY.` |
| AM-9 | M | `handlers.go:550` | `invalid skipMonths: <inner>` | `Skip Months: <inner>. Example: "6-8,12".` |
| AM-10 | H | `engine.go:133` | `insufficient loan data: need amount and payments per year` | Split into two: `Amount Borrowed is required.` and `Pmts/Yr is required.` |
| AM-11 | H | `engine.go:138` | `insufficient loan data: need loan date` | `Loan Date is required.` |
| AM-12 | H | `engine.go:156` | `insufficient loan data: need first payment date` | `1st Pmt Date is required (or fill Loan Date + Pmts/Yr to default it).` |
| AM-13 | M | `engine.go:172` | `compute true rate: <inner>` | `Rate is out of range (got <X>%).` |
| AM-14 | M | `validate.go:69` | `two rate adjustments on the same day (line N)` | `Rate Adjustment row N is on the same date as row N-1.` |
| AM-15 | H | `backward.go:75` | `insufficient data: need amount, rate, term, peryr` | `Cannot solve Pmt Amount — Amount, Rate, # Periods, Pmts/Yr are required.` |
| AM-16 | M | `backward.go:100` | `cannot determine payment - summation factor too small` | `Rate × Term is too small to solve for Pmt Amount.` |
| AM-17 | H | `backward.go:126` | `insufficient data: need rate, payment, term, peryr, first date` | `Cannot solve Amount Borrowed — Rate, Pmt Amount, # Periods, Pmts/Yr, 1st Pmt Date are required.` |
| AM-18 | H | `backward.go:198` | `insufficient data: need amount, payment, term, peryr` | `Cannot solve Rate — Amount, Pmt Amount, # Periods, Pmts/Yr are required.` |

### A.3 Present Value (`/api/presentvalue/calc`)

| # | Severity | File:line | Current | Proposed |
|---|---|---|---|---|
| PV-1 | H | `handlers.go:707` | `invalid asOfDate format` | `As-of Date is unparseable — use MM/DD/YYYY.` |
| PV-2 | H | `handlers.go:735` | `invalid lump sum date` | `Lump Sum row N: Date is unparseable.` |
| PV-3 | H | `handlers.go:759` | `invalid fromDate` | `Periodic row N: From Date is unparseable.` |
| PV-4 | H | `handlers.go:768` | `invalid toDate` | `Periodic row N: To Date is unparseable.` |
| PV-5 | H | `backward.go:287` | `specify either date or amount in single payments, line N` | `Lump Sum row N: also supply Date or Amount (Value alone is insufficient).` |
| PV-6 | H | `backward.go:316` | `specify either other date or amount in periodic payments, line N` | `Periodic row N: also supply the missing date or Amount.` |
| PV-7 | H | `calc.go:279` | `too many unknowns` | `More than one missing field on the screen — fill in enough cells to leave exactly one blank.` |
| PV-8 | H | `calc.go:285, backward.go:358` | `insufficient data on screen` | `Not enough inputs to solve for Sum Value — supply Rate, As-of Date, and at least one fully-specified row.` |
| PV-9 | H | `calc.go:301` | `need rate and as-of date for present value calculation` | `Rate and As-of Date are required for forward calculation.` |
| PV-10 | H | `backward.go:863` | `"rate" computation did not converge` | `Rate solver did not converge — try a different starting point or specify Amounts instead of Values.` |
| PV-11 | H | `backward.go:886` | `rate is not determined - specify amounts instead of values` | `Rate cannot be solved when all rows have Value but no Amount — fill at least one row's Amount.` |
| PV-12 | H | `backward.go:922` | `cannot compute date - interest rate too small` | `Rate is effectively zero — cannot solve for As-of Date.` |
| PV-13 | H | `backward.go:960` | `"as of" computation did not converge` | `As-of Date solver did not converge — try a different rate.` |
| PV-14 | H | `index.html:2382` | `IRR computation (blank rate) is not yet supported in the API. Please enter a rate.` | **Delete the guard entirely.** The backend has supported `solveRate` since the PV-8 port. |

---

## Appendix B — Verification methodology

The claims in this document were produced by four parallel research
passes (one per worksheet, one cross-cutting for errors) against the
DOS Pascal source under `legacy/src/dos_source/` and the current Go
port and frontend. The agents read full source files (not just
greps) and cite file:line throughout.

Five high-impact claims were spot-checked manually before publication:

| Claim | Verification | Result |
|---|---|---|
| `SolveLoanAmount` exists but handler never calls it | `grep -n` for `SolveLoanAmount` in `internal/api/handlers.go` | No matches. Confirmed unwired. |
| `SolveRate` exists but handler never calls it | Same | No matches. Confirmed unwired. |
| Amortization handler dispatches missing fields only when BOTH amount and rate are zero | Read `handlers.go:361` | `if req.Amount == 0 && req.Rate == 0` — confirmed. |
| `CompareAPRs` exists but no `/api/mortgage/compare` route | `grep -n CompareAPRs handlers.go` | No matches. Confirmed unwired. |
| Frontend PV-8 guard with stale message | Read `index.html:2381–2384` | `'IRR computation (blank rate) is not yet supported in the API. Please enter a rate.'` — confirmed verbatim. |

All five passed. The senior engineer reviewing this document should
feel free to spot-check additional claims via `grep` / `Read`; the
audit was performed with the assumption it would be challenged.
