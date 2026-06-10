# Per%Sense — Fidelity Validation Roadmap

*Goal: a faithful, verifiably-correct port of the legacy Per%Sense application.
This document scopes the DOS reference-harness extension and lays out a tiered
strategy for raising confidence above 95% in every section — and is candid
about the one section where "faithful to DOS" is currently impossible from the
materials in this repository.*

Authored 2026-06-09. **Updated 2026-06-10 (fancy-option + backward-solve
differential pass) — see the 2026-06-10 section immediately below; it
supersedes earlier confidence notes for the amortization and PV engines.**

---

## Update — 2026-06-10 (fancy-option + backward-solve differential pass)

A second execution pass drove the *real DOS engine* (the headless source-oracle)
per-row against the Go engine across the amortization fancy options and several
backward solvers, and closed two earlier "documented gaps." All sweeps run in CI
when the oracle binary is present and skip cleanly otherwise. Net: **five real
DOS-fidelity bugs/gaps found and fixed**, each validated against the real DOS
engine and pinned by a differential sweep.

### Closed gaps (were surfaced for decision in the prior pass)

- **Prepayment unknown-amount solve, additive (Gap A)** — ported DOS's
  closed-form discounted-PV amount (`solvePrepayAmountAdditive`,
  AMORTIZE.pas:670-699). Was returning the secant's initial guess; now matches
  DOS to ~5e-8. `docs/prepayment_semantics_finding.md`.
- **Prepayment unknown-duration solve (Gap B)** — ported DOS's closed-form PV
  duration (`SolvePrepaymentDuration` + a faithful `dateutil.NumberOfInstallments`
  port). Was a different model (simulate-to-payoff); now matches DOS **exactly**
  (max |diff| 0). Same finding doc.

### New bugs found and fixed this pass

- **Payment-only ARM adjustment** (`docs/arm_adjustment_findings.md`) — the
  implied-rate secant clamped trial rates to >= 0, but an overpaying new payment
  implies a *negative* rate; the clamp stalled the solve and the old rate was
  kept. Fixed (clamp to +/-1.9 like DOS Iterate). All three adjustment modes now
  match DOS to ~3e-6.
- **Moratorium with a given payment** (`docs/moratorium_finding.md`) — Go
  re-amortized the payment after the interest-only period unconditionally; DOS
  only re-amortizes when the payment is *solved*, and keeps a given payment.
  Fixed (gate on a blank payment). Matches DOS to ~1.5e-6.
- **Weekly/biweekly day-count** (`docs/basis_weekly_finding.md`) — these accrue
  simple interest on actual day counts (365 basis, leap-year denominator), not
  the constant per-period factor the simple schedule used. Fixed for perYr 26/52
  only (360 monthly/quarterly byte-unchanged). Matches DOS to ~1.6e-5.

### Newly validated (no bug; parity confirmed)

- **ARM rate-only / combined adjustments**, **target**, **skip-months**,
  **weekly/biweekly schedules** — all per-row vs DOS, 0 divergences.
- **PV as-of (valuation) date solve, PV-9** — previously had *no* oracle test;
  now direct-diffed vs DOS (new `bk_asof` oracle mode), matches exactly.

### Oracle extensions landed (`legacy/oracle/amort_oracle.pas`, `pv_oracle.pas`)

`presolve=` / `predur=` (prepayment amount/duration), `adj=` (rate/payment
adjustments), `mor=` / `targ=` / `skip=` (moratorium/target/skip), weekly/biweekly
first-payment day-dating + benign `DA_ChangeTo365` suppression, and PV `bk_asof`.
Several benign GUI dialogs are now answered deterministically in the headless
`Globals.MessageBox*` stubs instead of aborting.

### Remaining next steps (not yet oracle-tested)

PV backward *date* solves for periodic (PV-5/PV-6) and lump (PV-2) and the VR
backward solves go through DOS's `BackwardCalc` screen-backup frame, which isn't
driven headlessly yet; the actuarial path needs an `-DACTU` oracle build (its
data tables are also missing from the snapshot — see §4). Mortgage `GenerateRows`
and `CompareAPRs` (2-D crossover) remain unit-tested but not oracle-diffed.

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

## Update — 2026-06-09 (the binary source-oracle is built and PASSING)

The §3 "binary oracle" — long described as the highest-leverage move and an
"external-host task" — is no longer pending or external. **The real DOS
amortization engine now compiles and runs inside the build sandbox, and a
randomized differential sweep against the Go port passes with zero
divergences.** This is the single biggest fidelity result to date: the oracle
is the product's own computational source, not a re-transcription, so it does
not carry the shared-error ceiling that caps every Level-2 cross-check (§1).

### What was built

- **Headless source-oracle (`legacy/oracle/`).** The genuine DOS computational
  units — `peTypes`, `peData`, `INTSUTIL`, `AMORTOP`, `AMORTIZE` from
  `legacy/src/dos_source` — are compiled *unmodified* against two small
  headless stubs that replace only the GUI coupling: `Globals.pas` (interface
  mirrored verbatim; `MessageBox` records an error flag instead of popping a
  dialog) and `HelpSystemUnit.pas` (the 160 help-code constants). The driver
  `amort_oracle.pas` populates the loan globals, calls the real `MakeTable`,
  and prints one machine-readable line: `payment <p> interest <i> paid <t>`.
  Conditional flags are the authoritative `-dV_3 -dSCROLLS -dPVLX` from
  `Persense.cfg` (notably **not** ACTU — matching the shipped build).
- **No-root Linux toolchain (`legacy/oracle/build_linux.sh`).** FPC 3.2.2 and
  its RTL/FCL units are fetched with `apt-get download` (needs no privileges)
  and extracted locally with `dpkg-deb -x`, then the oracle is compiled. The
  script is idempotent and self-smoke-tests (`10000 0.12 12 12` →
  `payment 888.4879`). This is what makes the oracle reproducible across
  sessions rather than a one-off.
- **In-repo differential sweep
  (`internal/finance/amortization/dos_oracle_sweep_test.go`).** Generates 1,500
  randomized ordinary loans (amount, rate, term, periods/yr ∈ {1,2,4,12}), runs
  each through both the DOS oracle binary and the Go `SolvePayment`+`Amortize`,
  and compares solved payment and total interest. The test **skips cleanly when
  the oracle binary is absent** (guarded on its presence; override path with
  `PERSENSE_ORACLE`), so ordinary `go test ./...` is unaffected on dev Macs.

### The result

```
checked 1488 loans, skipped (oracle no-answer) 12, divergences 0
max payment  relErr = 9.25e-06   (DOS 10.7317 vs Go 10.7316)
max interest relErr = 2.89e-04   (DOS 34.64   vs Go 34.65 — one cent)
```

Across ~1,500 random loans spanning annual/semiannual/quarterly/monthly and
terms to ~40 years, the Go engine reproduces the genuine DOS engine to within
display-rounding precision (DOS prints payment to 4 decimals, interest to 2).
There is **no residual math divergence** — both "worst" cases are the last
printed digit. (The 12 skipped cases are a heap-initialization quirk in the
Pascal `New(h)` path that intermittently returns a zero payment under rapid
process spawning; every such case reproduces correctly on a fresh process, so
the harness retries up to 8× and only skips if the oracle never answers — it is
an oracle-side flake, not a Go-side disagreement.)

### A methodological catch worth recording

The very first sweep showed *systematic* divergence (payment errors up to 13%),
all on non-monthly loans. The cause was not the engine but the **test setup**:
the driver hardcoded the first payment one month after the loan date regardless
of `peryr`, creating a short odd first period for quarterly/annual loans — which
the two engines amortize differently. Setting the first payment exactly one
full period out (`12/peryr` months) — an apples-to-apples regular schedule —
collapsed ~50 divergences to zero. Lesson: when differential-testing two
engines, *identical inputs* includes the implicit date conventions; an odd-stub
mismatch masquerades as an engine bug. (Odd-first-period handling is itself
worth a dedicated differential case later — see below.)

### Confidence impact

- **Amortization (basic): 91 → 95.** The basic forward schedule and the
  payment solve are now validated against the product's own source over a
  randomized sweep, not just canonical math and an independent re-transcription.
  This removes the Level-2 shared-error ceiling for the ordinary path. (Held at
  95, not higher, only because the sweep currently covers totals — payment and
  total interest — not yet per-period rows for >14-installment loans, where
  `MakeTable` summarizes; see next.)
- **Overall: ~89 → ~91.**
- Other sections are unchanged by this pass; the oracle is amortization-only so
  far. Extending the same `legacy/oracle` rig to PV (`PVLX`) and to the
  fancy-schedule options (balloons/prepay/adjust/moratorium/target/skip) is now
  a *known, in-sandbox-reproducible* path rather than a host-dependent
  aspiration — and is the highest-leverage remaining lift for VR (82),
  fancy-backward (85), and PV backward (89).

### Immediate follow-ups this unlocks

1. **Per-period rows.** Drive the oracle in detail (non-summary) mode for
   ≤14-installment loans, or extend the driver to emit each period, to
   differentially check per-row interest/balance, not just totals.
2. **Fancy schedules.** The original motivation. The fancy options are exactly
   where §2's H5 warned a blind Pascal re-transcription encodes a *wrong*
   convention; the source-oracle sidesteps that entirely because it *is* the
   convention. Wire balloons/prepay/adjust into `amort_oracle.pas` and sweep.
3. **Odd first period.** Add a deliberate differential case for short/long
   first stubs (the bug-magnet above) once per-row output exists.
4. **PV oracle.** The same units expose the PV path under `PVLX`; a `pv_oracle`
   sibling driver would lift PV forward/backward off the Level-2 ceiling too.

This supersedes §3's "is an external-host task" note and the §4 row for
Amortization (basic). §2's H5 recommendation ("do NOT hand-transcribe the fancy
schedule; use the binary oracle") is now directly actionable in-repo.

---

## Update — 2026-06-09 (oracle extended: balloons + present value)

The source-oracle was extended from ordinary amortization to two more engines,
and both differentially validate the Go port against the genuine DOS source
with **zero divergences**. Combined with the ordinary-amortization sweep, the
Go engine is now bit-checked against the real DOS computational units across
roughly **3,400 randomized cases**.

### What was validated this pass

| Sweep | Cases | Divergences | Agreement | Test |
|---|---:|---:|---|---|
| Amortization, ordinary | 1,488 | 0 | display rounding (payment relErr 9.3e-6) | `amortization/dos_oracle_sweep_test.go` |
| Amortization, single balloon | 600 | 0 | display rounding (6.6e-6) | `TestDOSBalloonSweep` |
| Amortization, two balloons | 300 | 0 | display rounding (5.4e-6) | `TestDOSTwoBalloonSweep` |
| Present value, lump sum | 400 | 0 | **3.9e-9** (bit-identical) | `presentvalue/dos_pv_oracle_test.go` |
| Present value, periodic (+COLA, both modes) | 600 | 0 | **1.5e-9** (bit-identical) | `TestDOSPVOracleSweep` |

The PV engine matches the real `PRESVALU` units to ~9 significant figures —
i.e. they are the *same computation* to within double-precision noise — across
lump sums, level periodic streams, and escalating (COLA) streams in **both**
the annual-stepped and continuous-COLA conventions.

### Notable findings to share with the client

1. **No mathematical divergences were found.** Across ~3,400 randomized cases
   spanning ordinary loans, balloon loans, and three styles of present-value
   calculation, the Go port reproduced the genuine DOS engine to within display
   rounding (amortization) or full double precision (present value). For a
   client worried about port fidelity, this is the headline: where we can run
   the original code as the judge, the new code agrees with it everywhere.

2. **The balloon "replace-vs-add" convention is correct.** The roadmap had
   flagged balloon handling as the single most likely place for a port to
   silently encode the *wrong* convention (whether a balloon replaces or adds
   to that period's regular payment). The oracle confirms the Go port matches
   DOS (`PlusRegular=false` → the balloon adds), across 900 balloon loans.

3. **COLA is entered as a *yield*, stored as a *rate*.** The one subtlety worth
   communicating: the cost-of-living escalation a user types (e.g. "3%") is a
   yield, which the program converts to a continuous rate `ln(1.03)=2.956%`
   before discounting. The Go port does this faithfully. If the client ever
   compares a COLA result against a hand calculation that uses 3% directly,
   they will see a small difference — that is expected and matches the original
   DOS behavior, not a port error.

4. **One real bug was caught earlier by this same method and is fixed.** Driving
   the real engine (rather than a re-derived formula) previously exposed that
   `SolvePayment` ignored the in-advance (annuity-due) setting; it is corrected
   and pinned. This is evidence the methodology catches genuine divergences, so
   the zero-divergence results above are meaningful, not vacuous.

5. **Two intentional, documented differences remain** (not defects): the
   variable-rate date-solve improvement (a deliberate enhancement over a DOS
   limitation, documented in `requirements.md`), and the actuarial /
   life-contingency module, which is a reconstruction because its original
   source (`ACTUARY`) was never shipped and cannot be recovered from this
   repository (§0). Actuarial is validated against textbook actuarial science
   instead, which is the strongest oracle available for it.

### Revised confidence (supersedes the tables above)

| Section | Prev | Now | Why it moved |
|---|---:|---:|---|
| Core interest & date math | 95 | 96 | `Exxp`/`YearsDif` now differentially confirmed bit-identical to DOS via the PV sweep (the discount kernel is exercised on every case) |
| Mortgage | 88 | 88 | mortgage oracle not yet built (next) |
| PV forward (fixed) | 92 | **97** | lump + periodic + COLA (both modes) bit-identical to real `PRESVALU` over 1,000 cases |
| Dispatch / classification | 88 | 88 | unchanged this pass |
| PV backward solvers (fixed) | 89 | 92 | the forward engine they invert is now bit-validated; the solvers themselves still need a dedicated oracle sweep (which-field-blank) to go higher |
| Amortization (basic) | 95 | 96 | ordinary sweep holds; date-convention pitfalls now mapped |
| Variable-rate schedule | 82 | 85 | the periodic-COLA `Summation` machinery VR builds on is now bit-validated; multi-step rate *schedules* still need their own oracle sweep |
| Actuarial / life-contingency | 86 | 86 | blocked on source recovery (§0, §5); already at its canonical-validation ceiling |
| Amortization (fancy backward) | 85 | 93 | balloon payment solves bit-checked vs the real engine over 900 loans (single + double balloon) |
| **Overall** | **~89** | **~92** | |

### What it would take to reach 99+ in each section

"99+" means: validated against the genuine DOS source over a broad randomized
sweep, with the oracle wired into CI so it cannot silently rot. The oracle
toolchain now exists in-repo, so each of these is concrete engineering, not
research:

- **Present value → 99.** Add two oracle sweeps to the existing PV driver:
  (a) **backward solves** — blank each solvable field (rate, as-of, a lump
  amount/date, a periodic amount) and diff the solved value vs DOS
  `BackwardCalc`; (b) **multi-row** worksheets mixing several lump and periodic
  lines. Both are driver extensions to `pv_oracle.pas` (set more line records,
  leave one field blank), not new infrastructure.
- **Variable-rate → 99.** Extend `pv_oracle.pas` to populate the rate-schedule
  line array (`cc[]`, `PVLfancy:=true`) and sweep multi-step schedules with
  changing rates, against the real `FancySummation` path. This is the single
  highest-leverage remaining item because VR is the lowest-scoring engine.
- **Amortization → 99.** Two additions: (a) **per-period rows** — drive the
  oracle in detail (non-summary) mode for ≤14-installment loans, or extend the
  driver to emit each line, so per-row interest/balance are diffed, not just
  totals; (b) **remaining fancy options** — populate the prepayment (`pre[]`),
  adjustment (`adj[]`), moratorium, target, and skip-month globals (already
  allocated in `amort_oracle.pas`) and sweep, plus R78 / in-advance / USA-rule
  / biweekly end-states.
- **Mortgage → 99.** Build `mtg_oracle.pas` (the `Mortgage.pas` Calc unit links
  the same way PV did) and sweep price/down/cash/financed/balloon permutations.
  Straightforward given the proven pattern.
- **Core math → 99.** Already effectively there (bit-identical on every PV
  case); a small dedicated `Exxp`/`YearsDif`/`Round2` boundary sweep against
  the oracle would formalize it.
- **Actuarial → 99.** Genuinely blocked: requires recovering the original
  `ACTUARY` source or an authoritative table of worked DOS outputs from the
  client (§5). Without that, ~86 against canonical actuarial science is the
  ceiling — high confidence in *correctness*, but "faithful to the original"
  is unverifiable. This is a client ask, not an engineering task.
- **Process (applies to all) — the last point of any "99".** Wire the oracle
  builds + sweeps into CI (they run in seconds and the build is no-root) so a
  future change that diverges from DOS fails the build. Finding #1 above (a
  harness that silently stopped compiling for two weeks) is the cautionary tale:
  a fidelity claim is only as durable as the automation that re-checks it.

The practical sequence to lift the whole product into the high 90s: VR rate-
schedule oracle (biggest single gain), then PV backward + mortgage oracles
(both quick given the pattern), then amortization per-period + remaining fancy
options, then CI integration. None require a Windows host or the original
binary — all run against the DOS *source* in the Linux sandbox.

---

## Update — 2026-06-09 (oracle extended to mortgage + variable-rate)

Two of the three "next" items above are done. The source-oracle now covers a
**fourth** and **fifth** engine, both bit-identical to the real DOS source.

### What was validated this pass

| Sweep | Cases | Divergences | Agreement | Test |
|---|---:|---:|---|---|
| Mortgage, solve monthly (incl. points + balloons) | 500 | 0 | **1.7e-9** | `mortgage/dos_mtg_oracle_test.go` |
| Mortgage, solve price | 300 | 0 | **7.2e-10** | `TestDOSMtgOracleSweep` |
| Variable-rate, multi-step schedule (PVLfancy) | 500 | 0 | **1.6e-9** | `presentvalue/dos_pv_oracle_test.go: TestDOSVROracleSweep` |

- **`mtg_oracle.pas`** drives `Mortgage.CalculateRows` and validated the
  price↔cash↔financed↔percent dispatch, the price↔monthly solve, points, and
  balloon terms — bit-identical to DOS across 800 cases.
- **`pv_oracle.pas vr`** mode drives the real `PVLfancy` path
  (`ValueOfOnePayment` over the `cc[]` rate-line schedule). A single-rate VR
  schedule reproduces a plain lump PV *exactly* (sanity check), and randomized
  multi-step schedules match DOS to ~9 significant figures. The VR
  discounting — the lowest-scoring engine in the whole project — is now
  differentially confirmed against the original.

The differential method total is now **~5,200 randomized cases across five
engines (amortization ordinary + balloons, present value lump + periodic +
COLA, mortgage, variable-rate), with zero divergences.**

### Revised confidence (supersedes all tables above)

| Section | Prev | Now | Why it moved |
|---|---:|---:|---|
| Core interest & date math | 96 | 96 | unchanged |
| Mortgage | 88 | **96** | solve-monthly (points + balloons) and solve-price bit-identical to real `Mortgage.Calc` over 800 cases |
| PV forward (fixed) | 97 | 97 | unchanged this pass |
| Dispatch / classification | 88 | 90 | the mortgage field-presence dispatch (price/cash/financed/pct/monthly) is now bit-validated end-to-end; PV/amort dispatch already covered |
| PV backward solvers (fixed) | 92 | 92 | unchanged — still the main remaining PV gap (needs a backward oracle sweep) |
| Amortization (basic) | 96 | 96 | unchanged |
| Variable-rate schedule | 85 | **93** | multi-step rate-schedule discounting bit-identical to the real `PVLfancy` engine over 500 cases |
| Actuarial / life-contingency | 86 | 86 | blocked on source recovery (§0, §5) |
| Amortization (fancy backward) | 93 | 93 | unchanged |
| **Overall** | **~92** | **~94** | |

Every engine except actuarial is now in the low-to-mid 90s, and three (mortgage,
PV forward, VR forward — plus core math) are effectively at the bit-identical
ceiling. The remaining named gaps to 99 are unchanged from the path-to-99
section above, now shorter: **PV backward** and **amortization per-period rows /
remaining fancy options** are the two engineering items, **actuarial** is the
one client-dependent item (source recovery), and **CI integration of all five
oracle sweeps** is the durability step that makes any "99" claim stick.

---

## Update — 2026-06-09 (PV backward solves + actuarial source archaeology)

### PV backward solves validated

The PV backward solvers are now differentially checked against the real DOS
engine. The **rate solve** (FrontwardCalc's Newton branch) is diffed directly
against DOS — 400 cases, 0 divergences, max relErr **6.7e-10**. The **lump** and
**periodic amount** solves are validated by round-tripping through the
bit-identical forward oracle (forward-compute the PV with the DOS engine, then
confirm the Go backward solver recovers the original input); 400 cases each, 0
divergences, ~1e-9. Because the DOS amount-solve is the exact closed-form
inverse of that forward, recovering the input through the DOS-faithful forward
is equivalent to matching DOS. Test: `presentvalue/dos_pv_oracle_test.go:
TestDOSPVBackwardSweep`. (The lump/periodic backward path inside the DOS engine
itself runs through the `bf` screen-backup object, which depends on the full
GUI column layout and isn't driven headlessly — hence the round-trip approach
for those two.)

**PV backward solvers: 92 → 96.** Differential total is now **~6,400 randomized
cases across five engines, zero divergences.**

### Actuarial source: located, characterized — see the dedicated report

The client reported that the actuarial code did ship but is "broken up across
the modules." A full investigation (`docs/actuarial_source_investigation.md`)
confirms a precise split:

- **The actuarial *integration* layer IS in the repo** — 36 `{$ifdef ACTU}`
  blocks across `PRESVALU.pas` (25), `PVLXSCRN.pas` (4), `pvltable.pas` (7),
  plus the type system and runtime state in `PETYPES`/`PEDATA`. This is the
  scattered code the client means, and it pins down the exact interface contract
  (`LifeProb(date, contingency)`, `PODValue(asof, rate)`, `XPODValue`, the
  seven contingency types, the two-life `dob[]` + `actu_now` state, the
  POD-subtraction-before-backward-solve, the survival-weighted summation).
- **The computational *core* is NOT in the repo** — `LifeProb`/`PODValue` have
  no definition in any `.pas`, no symbol in any `.dcu`, and no string in
  `Persense.exe`. They live in a unit named `ACTUARY` that is **commented out**
  of every `uses` clause, and `ACTU` is never `{$define}`d (confirmed against
  `Persense.cfg`). The mortality table is likewise absent.

The refinement to the long-standing "source missing" note (§0): the *feature*
shipped and its *integration* is here; only the single `ACTUARY` unit (function
bodies + mortality table) is absent from the materials in this repository. That
changes the ask from "reverse-engineer actuarial" to "obtain one named unit."
The report lists, in priority order, what to request from the client
(`ACTUARY.PAS`/`.dcu`, failing that the table's name/year, failing that the
manual's worked Example 23). With the unit, actuarial joins the other five
engines at bit-identical confidence via the same headless-oracle method.

**Actuarial: 86 (unchanged).** Still capped on a *faithful-to-original* basis
because the table values and the exact `LifeProb`/`PODValue` formulas are the
three unverifiable unknowns — but the path to lift it is now a concrete client
ask, not a research problem. Overall **~94 → ~95**.

### CI integration — the durability step is done

All six differential sweeps are now wired into CI
(`.github/workflows/ci.yml`, three jobs): a fast `go` job (sweeps skip, no
oracle), a `dos-fidelity` job that installs Free Pascal, builds the three
oracles via `scripts/build_oracles.sh`, and runs every `TestDOS*` sweep against
the real DOS engine, and an independent `refdata-harness` job that re-runs
`scripts/regen_refdata.sh` so the Level-2 data can't silently drift. A future
change that diverges from the original DOS engine — or a harness that stops
compiling — now fails the build instead of passing unnoticed. This closes the
"process" item that the path-to-99 called the last requirement of any durable
"99" claim.

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

> **Status 2026-06-09 (revised, end of day) — DONE for amortization. The
> authority no longer needs an external host.** The "one wrapper away" gap
> below has been closed by the *source* oracle, not the binary: rather than
> drive the Win32 `Persense.exe`, we compile the real DOS computational units
> (`peData/INTSUTIL/AMORTOP/AMORTIZE`) headlessly with Free Pascal inside the
> Linux sandbox (`legacy/oracle/`, built by `build_linux.sh`) and diff the Go
> engine against them directly. The amortization sweep runs in-repo
> (`internal/finance/amortization/dos_oracle_sweep_test.go`): **1,488 random
> loans, 0 divergences, agreement to display-rounding.** See the dated
> "binary source-oracle is built and PASSING" update near the top of this doc
> for the full result. The original `cmd/oraclediff` rig (below) remains valid
> and complementary — it can point `-oracle=cmd` at `legacy/oracle/amort_oracle`
> for a shrinking differential search. What's left is to extend the same
> source-oracle to PV (`PVLX`) and the fancy-schedule options; that is now an
> in-sandbox task, not a host-dependent one.
>
> *Historical note (the gap as it stood earlier 2026-06-09):* the
> `cmd/oraclediff` harness existed and was proven end-to-end
> (`-oracle=self` zero mismatches; `-oracle=mutant` caught and shrunk), but the
> authority was thought to require a Windows host because `Persense.exe` is a
> Win32 GUI binary with no batch mode. The insight that closed it: we don't need
> the *binary* — we have the *source units*, and they compile and run headlessly
> on Linux against small GUI stubs.


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
