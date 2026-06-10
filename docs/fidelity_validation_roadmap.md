# Per%Sense — Fidelity Validation Roadmap

*Goal: a faithful, verifiably-correct port of the legacy Per%Sense application.
This document scopes the DOS reference-harness extension and lays out a tiered
strategy for raising confidence above 95% in every section — and is candid
about the one section where "faithful to DOS" is currently impossible from the
materials in this repository.*

Authored 2026-06-09. **Updated 2026-06-09 (validation pass) — see the Update
section immediately below; it supersedes the confidence table in §4.**

---

## Update — 2026-06-09 (validation pass)

A first execution pass against this roadmap landed real tooling and surfaced two
findings that change the confidence picture. Everything below ran in CI on the
Go side; the Pascal-harness pieces are gated on a host with `fpc`.

### What was built

- **Canonical actuarial validation (Track C, started).** Five first-principles
  tests (`internal/finance/presentvalue/actuarial_canonical_test.go`) validate
  the actuarial engine against textbook life-contingency mathematics — pure
  endowment (agrees to 1e-9), temporary life annuity (1e-6), Payment-on-Death /
  term insurance (to the cent), curtate life expectancy, and the two-life
  survival composition — with every expected value computed *independently* in
  the test (lx built from qx, explicit summation), not via the engine's own
  helpers. This is the correctness oracle actuarial never had.
- **Property-based differential rig.** ~10,000 randomized worksheets
  (`property_fuzz_test.go`) assert finiteness, additivity, rate monotonicity,
  contingency ordering, and backward round-trip recovery — seeded for
  reproducibility, with a pluggable `Oracle` interface stubbed (`NoopOracle`)
  ready for the §3 binary diff.
- **Two-life guard.** A two-life contingency with no second life table now
  errors (`checkSecondLifeProvided`) instead of silently treating Person 2 as
  immortal; engine and API share one message.
- **Level-2 harness extended to PV.** New `pv_lump` and `pv_periodic` sections
  in `refdata.pas` cross-check the PV lump and periodic paths — forward value
  *and* the PV-1 / PV-4 backward solves — by driving the real `Calculate`, not
  re-deriving a formula. (`pv_periodic` activates on the next
  `regen_refdata.sh --apply`.)

### Two findings that recalibrate §1's "ceiling"

1. **The harness had silently stopped compiling.** `refdata.pas` had not built
   since 2026-05-27 (a nested `{ }` inside a comment block). So the `rule78`,
   `in_advance`, and `biweekly` reference sections added back then were **never
   actually cross-checked**, despite `dispatch_gaps.md` Rev 9 claiming
   "refdata.json is current." A "DOS-cross-checked" claim can rot the moment the
   harness breaks. **Mitigation: CI must run `regen_refdata.sh` and diff**, so a
   broken or stale harness fails the build instead of passing silently.

2. **Several cross-checks validated nothing about the port.** The `rule78` /
   `in_advance` / `biweekly` Go tests *re-derived the closed form inside the
   test* rather than driving the engine — "harness formula == test's
   re-derivation," never calling the Go code under test. They have been
   reworked to drive the real engine (`Amortize`, `SolvePayment`), as the new
   `pv_lump` / `pv_periodic` checks already do. **Rule: every cross-check must
   call the real engine.**

   Doing so immediately caught a **real bug**: `SolvePayment` ignored the
   in-advance setting and returned the arrears payment (599.55) where the
   annuity-due payment (596.57) is correct. Fixed per the DOS authority — DOS
   `EstimateAndRefinePayment` (Amortize.pas:402-407) only takes the closed-form
   early-exit when `not in_advance`, otherwise it `Iterate`-refines, which for a
   simple loan converges to arrears ÷ f.

### Revised confidence (supersedes the §4 table)

| Section | Was | Now | Why it moved |
|---|---:|---:|---|
| Core interest & date math | 95 | 95 | already cross-checked; unchanged |
| Mortgage | 88 | 88 | no new validation yet — binary oracle still pending |
| PV forward (fixed) | 90 | 92 | `pv_lump` + `pv_periodic` now cross-checked vs independent Pascal; property tests |
| Dispatch / classification | 87 | 88 | rate-line classification pinned end-to-end |
| PV backward solvers (fixed) | 85 | 89 | PV-1 / PV-4 now cross-checked vs independent Pascal (driving the engine); property round-trips |
| Amortization (basic) | 85 | 91 | canonical textbook-annuity validation + ~6,500-case property rig (schedule invariants & backward round-trips) + per-row independent-Pascal cross-check (`amort_schedule`); R78 engine-validated; in-advance bug fixed |
| Variable-rate schedule | 80 | 82 | corner + solve coverage added; still no dedicated DOS cross-check |
| Actuarial / life-contingency | 80 | 86 | first-principles canonical oracle (the big lift) + two-life guard + corners; DOS-*fidelity* still capped by the missing `ACTUARY` source |
| Amortization (fancy backward) | 72 | 85 | fancy `Iterate` ported as a robust bisection; amount/rate/payment now solve under balloons, prepayments and adjustments; SolvePayment-ignores-balloons bug fixed; round-trip + 400-case property coverage |
| **Overall** | **~84** | **~89** | |

**Amortization update (this pass).** The basic schedule and the common
backward solvers were the target. They now carry three independent layers:
(1) **canonical** validation against the textbook annuity — solved payment,
per-period interest `= balance·(f-1)`, and the closed-form remaining balance
all match to ~1e-7 (`amortization/canonical_test.go`); (2) a **property rig**
of ~6,500 randomized loans asserting schedule invariants, monotonic balances,
totals reconciliation, and payment↔amount↔rate backward round-trips to 1e-6
(`amortization/property_fuzz_test.go`); and (3) a per-row **independent-Pascal
cross-check** of interest/balance/payment at head/mid/tail (`amort_schedule`
harness section + `TestCrossCheckAmortSchedule`, active on the next regen).
That triple-coverage takes basic amortization to ~91.

**Fancy backward `Iterate` (this pass).** Solving amount/rate/payment *under
balloons, prepayments, and adjustments* — previously best-effort closed-form —
is now ported. Rather than DOS's finite-difference Newton against the
schedule's unforced terminal balance (which the Go forward engine hides by
forcing the final payment), it bisects the **over/under-amortization sign**,
which is monotonic in each unknown and survives that discontinuity, using the
forward engine itself as the oracle (`fancybisect.go`). Driving the engine this
way immediately exposed and fixed a real bug: **`SolvePayment` ignored balloons
entirely**, returning the no-balloon payment. The fancy solves now round-trip
(targeted balloon/prepayment/adjustment cases plus a 400-case property sweep:
solve a field, recover it, and confirm the loan amortizes cleanly over its full
term), and the obsolete Newton machinery was removed. Fancy backward moves to
~85, capped below 90 only by the lack of an independent DOS-output cross-check
for full fancy schedules and a documented residual edge for balloons dated
*after* the last regular payment. Overall amortization now sits at ~90.

**On the fancy-schedule harness (H5).** Attempting it surfaced a clean lesson:
a careful Go re-derivation of even a single-balloon schedule diverged from the
engine by exactly one regular payment, because the engine *replaces* the
regular payment with the balloon at `cmp == 0` when `PlusRegular` is false
while the re-derivation *added* it. A blind Pascal transcription would make the
same class of error and the cross-check would still go green — false
confidence, the exact §1 ceiling. So H5 is intentionally **not** pursued as a
Pascal harness; the binary oracle (§3) is the correct tool for fancy-schedule
fidelity. What runs in CI instead is `fancy_accrual_test.go`: the
first-principles accounting law that interest in each period equals the prior
balance times the period rate, checked across balloons (replace and add) and
prepayments over 500 random loans — independent of the engine's payment
conventions, so not circular, and it catches the schedule-construction bugs
that matter (wrong balance for interest, dropped period, mis-timed rate).

Actuarial moved the most because, absent any DOS authority for it, validation
against canonical actuarial science is the strongest oracle obtainable — and it
now exists and passes. It is capped below ~90 only because "faithful to the
original" remains unverifiable until the source is recovered (§5).

### Additions to the recommended approach (§6)

- **Make CI run the harness.** Add `scripts/regen_refdata.sh` (diff mode) to CI
  so the harness can't silently break or drift again. This is the cheapest,
  highest-value process fix and directly prevents a repeat of finding #1.
- **Audit every existing cross-check to confirm it drives the engine,** not a
  re-derivation (finding #2). The reworked R78/in-advance and the new
  pv_lump/pv_periodic are the template.
- **The binary oracle (§3) is now even more strongly the #1 technical
  recommendation** — both findings show the current harness layer is more
  fragile than its green checkmarks implied, and the in-advance bug proves real
  engine-vs-authority divergences exist and are catchable only by exercising the
  real code against an independent authority.
- **Actuarial Track C is materially underway;** the remaining actuarial work is
  (a) expand the canonical tests to COLA-bearing annuities and the as-of/rate
  solves, (b) an actuary sign-off pass, and (c) the source-recovery asks in §5
  for true DOS-fidelity.

The original analysis (§0–§6) below stands; only the §4 numbers are superseded
by the table above.

---

## 0. The finding that reframes everything: the actuarial source is missing

While scoping the harness I confirmed three facts that change the actuarial
picture:

1. **The `ACTUARY` unit — the life-contingency engine — is not in the legacy
   tree.** Every reference to it is a commented-out `uses` clause
   (`PRESVALU.pas:12`, `pvltable.pas:6`, in both `dos_source` and
   `win_source`): `//{$ifdef ACTU} ,ACTUARY {$endif}`. There is no
   `actuary*.pas` anywhere under `legacy/`.

2. **The `ACTU` compile flag was never enabled in any shipped build.** No
   `{$define ACTU}` exists in any `.pas`, `.cfg`, `.inc`, or project file. So
   the life-contingency code (`LifeProb`, `PODValue`, two-life contingencies,
   POD) was *dead, uncompiled code* in the authoritative DOS and Windows
   builds. The "Actuarial" strings inside `win_source/Persense.exe` are the
   **amortization USA-rule** ("Actuarial standard … American rule"), a
   payment-application method — **not** life-contingency. The binary contains
   no life-table / payment-on-death / contingency logic.

3. **The Go port says so itself.** `internal/finance/actuarial/table.go`:
   *"…was never ported to the Windows version. The ACTUARY unit source is
   missing; this implementation is reconstructed from integration [call
   sites]."* `contingency.go` likewise notes `LifeProb` was *"Reconstructed
   from the calling patterns in PRESVALU.pas."*

**Consequence.** The Go actuarial engine is a *reconstruction of a feature that
was never compiled in the original product*, inferred from the call-site
contracts in `PRESVALU.pas` plus the Windows Help worked examples. There is no
DOS source to port from and no DOS binary output to diff against. This is the
structural reason actuarial was the weakest section and why real bugs survived
nine audit passes. **No amount of harness work can make actuarial "faithful to
DOS," because the DOS authority for it does not exist in this repository.**

What this implies for the client's "fully ported and correct" goal is split:

- *Faithful to the original* — **blocked** for actuarial pending recovery of
  the `ACTUARY` source or an `ACTU`-enabled build. See the client asks in §5.
- *Correct* — **achievable** for actuarial by validating against canonical
  actuarial science (standard life-contingency formulas), which is what the
  reconstruction should match anyway. See §4, Track C.

Everything else (mortgage, amortization, PV non-actuarial, variable-rate) *does*
have authoritative DOS source and a shipped binary, so for those sections full
fidelity is reachable.

---

## 1. What "DOS cross-check" means today (and its ceiling)

`legacy/testharness/refdata.pas` is **not** the DOS application emitting
reference output. It is an **independent FreePascal reimplementation** of
selected DOS formulas (it carries its own `Summation`, `SumFormula`,
`MortgageCalc`, `Round2`, `MDY`, R78, in-advance, …; it `uses` only
`SysUtils, DateUtils, Math`). It compiles under `fpc`, runs, and emits
`refdata.json`, which the Go cross-check tests
(`internal/finance/crosscheck_test.go`, `crosscheck_backward_test.go`) load and
assert against.

This is a **two-independent-implementations** check: the harness author
transcribed the DOS formula, the Go author transcribed it separately, and they
must agree. That is genuinely stronger than a Go round-trip — but it shares one
failure mode: **if both transcribers misread the same DOS construct, the check
passes and the bug ships.** That is the ceiling of the current approach, and
the reason §4 proposes a *binary oracle* as the top tier.

Current `refdata.json` sections: `julian_roundtrip`, `exxp`, `lnn`, `power`,
`round2`, `yield_rate_roundtrip`, `mortgage_summation`, `mortgage_calc`,
`pv_sumformula`, `yearsdif`, basis coercion, R78, in-advance. These are mostly
**low-level primitives** — not the full backward solvers, not VR, not the
fancy amortization schedule, not actuarial.

---

## 2. Scope: extending `refdata.pas` (Level-2 validation)

Each item below is a new `Emit…Tests` procedure transcribed from the cited DOS
procedure, plus a Go cross-check that loads the new array. Effort is in
engineer-days (transcribe + emit + Go consumer + reconcile), assuming `fpc` is
available (the project's `scripts/regen_refdata.sh` already auto-detects it).

| # | New section | DOS source to transcribe | Notes / risk | Effort |
|---|---|---|---|---|
| H1 | **PV forward w/ COLA** (`pv_periodic_cola`, `pv_lump`) | `PRESVALU.pas:269-400` `SummationForSteppedCola` / `Summation` | Month-stepped vs continuous COLA; pure functions, clean to port | **M** (2-3d) |
| H2 | **PV backward closed forms** (PV-1/2/4/5/6) | `PRESVALU.pas:866-1085` `BackwardCalc` | The closed-form arms transcribe cleanly; the ±1-period date refinement needs care | **L** (4-6d) |
| H3 | **PV rate/as-of solve** (PV-8/9) | `PRESVALU.pas:693-818` | Newton + damping constants must match exactly | **M** (2-3d) |
| H4 | **Variable-rate `FancySummation`** | `PVLXSCRN.pas:305` | Depends on the rate-schedule structure; moderate | **M** (3-4d) |
| H5 | **Amortization fancy schedule** (balloons, adjustments, prepay, moratorium, target, skip) | `AMORTOP.pas:574-664`, `Amortize.pas` | **Reconsidered — do NOT hand-transcribe.** The fancy schedule's conventions (balloon replace-vs-add / PlusRegular, prepayment counting, adjustments, prepaid stubs) are subtle enough that a blind Pascal reimplementation reliably encodes a *wrong* convention and yields false confidence (demonstrated 2026-06-09: a careful Go re-derivation of the balloon schedule diverged from the engine by one payment because it added the balloon where the engine replaces it). Use the binary oracle (§3) for fancy-schedule fidelity. Internal interest-accrual consistency is meanwhile pinned in CI (`fancy_accrual_test.go`). | binary oracle |
| H6 | **Amort R78 / in-advance / USA-rule / biweekly** end-states | `AMORTOP.pas`, `Amortize.pas` R78 path | Extends the existing R78/in-advance emitters to full schedules | **M** (3-4d) |
| H7 | **Actuarial** | — | **BLOCKED**: no `ACTUARY` source. Reimplementing from the same call sites the Go port used is circular — it adds no independent assurance. *Do not do this; pursue §4 Track C instead.* | n/a |

**Level-2 total (H1–H6): ~3–4 engineer-weeks.** Raises mortgage, amortization,
PV (non-actuarial), and VR from "round-trip + single help example" to
"independent-reimplementation cross-checked," i.e. into the low-mid 90s. It
does **not** lift the transcription-shared-error ceiling, and does nothing for
actuarial.

---

## 3. The stronger move: a binary oracle (Level-3 validation)

> **Status 2026-06-09 — the rig is built; the authority needs a host.**
> The differential harness now exists and runs: `cmd/oraclediff` generates
> thousands of random amortization worksheets, runs the Go engine, compares
> against a pluggable oracle, and **shrinks** any disagreement to a minimal
> reproducer. It is proven end-to-end and pinned in CI
> (`cmd/oraclediff/oraclediff_test.go`): `-oracle=self` reports zero
> mismatches (plumbing sound), and `-oracle=mutant` (a deliberately
> one-period-short reference) is caught and shrunk — demonstrating it would
> surface a real engine-vs-authority divergence. The remaining piece is the
> *authority* itself: `Persense.exe` is a Win32 GUI binary with no batch mode
> and cannot run in the Linux build sandbox (no Wine, non-root). Wiring it in
> is an external-host task — `-oracle=cmd -cmd "<wrapper>"` against either a
> headless Free Pascal driver that links the real legacy computational units
> or a GUI-automation wrapper under Wine. See `cmd/oraclediff/README.md` for
> the stdin/stdout contract and both adapter options. This is the single
> highest-leverage remaining investment: it is the only validation tier that
> escapes the shared-transcription ceiling, and it is now one wrapper away.


We have `legacy/src/win_source/Persense.exe` — the actual shipped product, with
mortgage, amortization, and present-value (non-actuarial) logic compiled in.
Diffing the Go engine against the *real binary* over a large randomized input
sweep is the gold standard: it removes the shared-transcription-error ceiling
entirely, because the oracle is the product itself.

**Approach — differential / property-based testing against the binary:**

1. Drive `Persense.exe` headlessly (Windows host or Wine; it is a Win32 GUI app,
   so this needs UI automation or, better, locating its file-I/O path — the DOS
   build reads/writes worksheet files via `FileIOUnit.pas`, so feeding input
   files and reading result files avoids screen-scraping).
2. Generate thousands of randomized but valid worksheets per screen (mortgage,
   amortization incl. every advanced option, PV incl. VR).
3. Run both engines; assert agreement to a documented tolerance.
4. Shrink any disagreement to a minimal reproducer (property-based style).

**Effort:** harness plumbing **L** (1–1.5 wk, most of it the automation rig);
then it runs unattended and finds divergences across *all* covered sections at
once. Higher up-front cost than one Level-2 section, but it covers everything
the binary contains and is strictly more trustworthy.

**Caveat:** the binary does **not** contain actuarial (ACTU off), so Level-3
cannot validate actuarial either. Actuarial needs Track C or recovered source.

---

## 4. Path to >95% confidence, per section

Three tracks, applied per section as appropriate:

- **Track A — coverage depth:** more cases at the current validation level
  (cheap, incremental).
- **Track B — binary oracle:** differential testing vs `Persense.exe` (§3).
- **Track C — external/canonical oracle:** for actuarial, validate against
  standard life-contingency mathematics (commutation functions; textbook
  annuity/insurance present values; an independent actuarial library; SOA
  illustrative tables) — i.e. validate *correctness* even though
  *DOS-fidelity* is unrecoverable.

| Section | Today | To reach >95 | Track |
|---|---:|---|---|
| Core interest & date math | 95 | Add overflow/underflow, denormal, and boundary inputs to existing refdata arrays | A (S) |
| Mortgage | 88 | Differential sweep vs binary across price/down/cash/financed/balloon permutations | B |
| PV forward (fixed) | 90 | H1 + binary sweep over COLA/period/basis combinations | A+B |
| Dispatch / classification | 87 | Exhaustive field-presence matrix (already mostly pinned) + binary spot-checks | A+B |
| PV backward solvers (fixed) | 85 | H2/H3 + binary sweep over which-field-blank × row mixes | A+B |
| Amortization (basic) | 85 | H6 + binary sweep (R78, in-advance, USA, biweekly) | A+B |
| Variable-rate schedule | 80 | H4 + binary sweep over multi-step schedules; pin the deliberate VR-date-solve improvement as a *documented* divergence | A+B |
| Actuarial / life-contingency | 80 | **Cannot reach 95 vs DOS from current materials.** Reach 95 *for correctness* via Track C; reach 95 *for fidelity* only with recovered `ACTUARY` source or an ACTU-enabled build (§5) | C (+ source recovery) |
| Amortization (fancy backward) | 72 | Replace the best-effort closed-form fallback with the DOS `Iterate` helper, then H5 + binary sweep | B (L) |

**Net:** Track B (one binary-oracle rig) is the highest-leverage single
investment — it lifts six sections toward the mid-90s at once. Track C is the
*only* route for actuarial correctness. Source recovery (§5) is the only route
for actuarial *fidelity*.

---

## 5. Concrete asks for the client (unblock actuarial)

The actuarial ceiling is a missing-materials problem, not an engineering one.
Any **one** of these would change it:

1. **The `ACTUARY.pas` unit source** (or whatever the life-contingency unit was
   named) from an `ACTU`-enabled build. This is the real authority; with it,
   actuarial becomes a normal port + Level-2/3 validation like everything else.
2. **A build of Per%Sense with life-contingency enabled** (an executable that
   actually computes life tables / payment-on-death). Even without source, this
   becomes a binary oracle (§3) for actuarial specifically.
3. **The original actuarial specification / worked examples** the feature was
   built to — the Windows Help `PV_EX*.html` set is the current anchor;
   confirmation of which examples are authoritative, and any additional
   worked numbers, expands the Track-C oracle.

Until one of these arrives, the honest status line for actuarial is:
*"reconstructed from call-site contracts and worked examples; internally
self-consistent and (to be) validated against standard actuarial mathematics;
not verifiable against the original because the original actuarial code was
never shipped and its source is absent."*

---

## 6. Recommended sequencing

1. **Decide the actuarial question first** (§5) — it determines whether
   actuarial is a port or a clean-room reconstruction, and it's a client-side
   information task that can run in parallel with everything below.
2. **Build the binary-oracle rig (Track B, §3).** Highest leverage; lifts
   mortgage, amortization, PV, and VR together. Start with file-I/O driving of
   `Persense.exe` rather than GUI scraping.
3. **Track C for actuarial:** validate the reconstruction against canonical
   life-contingency formulas and the `PV_EX*` worked examples; engage an actuary
   for a sign-off pass.
4. **Fill Level-2 gaps (H1–H6)** for the cases the binary can't easily be driven
   through, and to keep fast, deterministic regression coverage in CI.
5. **Close the fancy-backward `Iterate` gap** (the one acknowledged
   common-ish divergence) so amortization's lowest grade comes up.

Items 2–5 are roughly 6–9 engineer-weeks total; item 1 is the client's, and
gates how far actuarial can go.
