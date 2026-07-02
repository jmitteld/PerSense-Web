# Plan: collapse to DOS's single `Iterate` and remove the (non-DOS) bisection

## DONE (2026-07-02) — bisection fully removed; all backward solves use DOS's one Newton

The collapse is complete. Every amortization backward solve now runs DOS's single
finite-difference secant `Iterate` (AMORTOP.pas:1415), ported as `dosIterate(seed,
accInit, terminal)` plus one terminal per unknown:

- **payment** — `dosIteratePayment` (fancy / exact-non360, over `fancyTerminal`) and
  `dosIterateSimplePayment` (plain in-advance / odd-first, over `RepayLoan` — the
  annuity-due recursion, basis-independent, prepaid gate below).
- **loan amount** — `dosIterateAmount` (`var x = h^.amount`, AMORTOP.pas:460; the
  trial amount is the schedule's starting balance).
- **rate** — `dosIterateRate` (`var x = loanrate`, :477; recomputes
  `GrowthPerPeriod`/true-rate each step).
- **moratorium / ARM segment** — `solveSegmentPayment` now calls `dosIteratePayment`
  on the bounded sub-loan (DOS's `Iterate(..., til_adj)`).

Deleted (zero production callers): `solveFancyAmount`, `solveFancyRate`,
`solveFancyPayment`, `fancyBisect`, `fancyOverUnder`, `fancyBisectTol`, and the dead
`refineFancyPayment`. DOS has **no bisection anywhere**; the removed sign-bisection
was a Go invention (per user directive: don't introduce logic absent from the
original).

**One subtlety found & fixed during the migration** (regression test
`TestDOSPrepaidShortFirstVsStub`): the `RepayLoan` terminal reads DOS's global
`prorate`, which is 1 for a prepaid loan ONLY when it is taken MORE than one period
before the first payment (stub collected at closing, Amortize.pas:1276-1283);
otherwise the first period is the actual SHORT `loanDate→firstDate` span. An
unconditional loan-date shift (forcing prorate 1) solved short-first prepaid
payments ~9% high. `dosIterateSimplePayment` now gates the shift on
`loanDate < naturalStart`, matching `generateSimpleSchedule` (the row-validated
simple schedule).

**Re-validation (all green, 0 divergences):** payment-solve fuzz ~14k cases
(ordinary/365/365-360/in-advance/prepaid/exact) 0 fails; `TestDOSInAdvanceFancyFuzz`
0 payment fails; fancy backward amount/rate round-trip vs DOS 0 fails
(relErr ≤1.5e-5); `TestUIAmortSweepVsDOS` final total-interest 0 fails; full repo
`go test ./...` green. Commits on top of the acc_limit acceptance fix.

## PROGRESS (2026-07-01)

### (a) The odd-first-balloon residual is CLOSED (strict 1e-3). Root cause found and fixed.

The DOS payment-solve for the OPTION cases (skip / balloon / prepayment / target /
adjustment) now runs DOS's Newton `dosIteratePayment` over the unforced terminal,
seeded by DOS's adjusted-principal closed form (`dosSeedPVFactor`). Results:
`TestUIAmortSweepVsDOS` total-interest 57 → 0; `TestDOSOddFirstFancyFrontier`
strict 0-divergence (max relErr ~6.6e-4); full amortization suite green.

**The breakthrough — the unforced terminal must run the FULL term (no early stop,
no fold), exactly as DOS's `Iterate` does.** Confirmed by instrumenting DOS's
per-period `ComputeNext` (see recipe below) and dumping the walk at an *overpaying*
trial payment (case 454603 / 0.0875 / n=14 / py=1 / first=3 / b24, DOS Iterate
step x=53401.16):

```
CN 4/135  int=6287.80  pay=53401.16  p= 24747.18   (period 13: still positive)
CN 4/136  int=2165.38  pay=53401.16  p=-26488.60   (crosses negative)
CN 4/137  int=-2317.75 pay=53401.16  p=-82207.52   (period 14: FULL payment applied
                                                     again; DOS does NOT stop here)
DBGITER x=53401.16 p=-82207.52   (this -82207 is the terminal DOS's secant uses)
```

DOS's `Iterate`→`RepayFancyLoan(Output=nil)` applies the full regular payment
through `very_last` even when the balance overshoots deeply negative — no early
`minpmt` stop, no final-row fold (folding/early-stop happen only on the *display*
path, `Output≠nil`). The earlier Go `fancyTerminal` had a one-sided
`if p < minPmt break` that stopped at 4/136 (p=−26488), which grew a SPURIOUS
SECOND zero (e.g. 52228 alongside the true 50185); the secant then root-switched to
it (the last ~2e-3 divergence). **Fix:** in `generateFancyScheduleMode(unforced)`,
remove the early stop — run to `very_last` with the full payment. The terminal
becomes MONOTONE (single zero) and the secant lands on DOS's root. Locked in by
`TestFancyTerminalMonotone` (asserts a single sign change across the payment band
on the exact loans that previously root-switched).

**Seed parity — VERIFIED via the instrumented oracle:** DOS's
`EstimateAndRefinePayment` seed for the 288-case = **1825.5110822932**; Go's
`dosSeedPVFactor`-based seed matches to the digit. With the monotone terminal the
proration is no longer needed on the seed for fancy loans (DOS's seed has none —
the proration lives in `RepayLoan`, applied during iteration), so it is gated on
`!hasAnyAdvancedOption`; fancy loans now solve from DOS's true adjp seed and the
frontier stays strict-clean.

### Oracle instrumentation recipe (reusable; legacy is READ-ONLY)

Build a debug oracle that prints DOS's seed + Iterate steps + per-period walk to
STDERR (keeps stdout parsing clean):
1. Run `legacy/oracle/build_linux.sh` once (stages symlinks in `/tmp/oraclestage`,
   builds FPC into `/tmp/fpcroot`).
2. In `/tmp/oraclestage`, **remove the symlink then write a real copy** (writing
   through the symlink corrupts read-only `legacy/src` — recover with
   `git checkout -- legacy/...`). Note line endings: **Amortize.pas is CRLF,
   AMORTOP.pas is LF** — match them in byte-anchored inserts.
   - Amortize.pas: after `d := adjp * (f - 1) / denom;` →
     `Writeln(StdErr,'DBGSEED ',d:0:10);`
   - AMORTOP.pas `Iterate`: after `x := savex;` →
     `Writeln(StdErr,'DBGITER x=',x:0:6,' p=',p:0:6);`
   - AMORTOP.pas `ComputeNext`: after `p := p + interest - payamt;` →
     `Writeln(StdErr,'CN ',date.m,'/',date.y,' int=',interest:0:2,' pay=',payamt:0:2,' p=',p:0:2);`
3. Recompile to a separate binary with the build_linux.sh FPC command:
   `ppca64 -Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX -Fu<UROOT>/rtl … -Fu/tmp/oraclestage -FU<unitout> -o/tmp/oraclebuild/amort_oracle_dbg /tmp/oraclestage/amort_oracle.pas`
4. `amort_oracle_dbg AMT RATE N PERYR flags…` (blank payment) → stderr has the full
   solve trace; add `pay=X` to dump a single fixed-payment walk.

### Exact-long-term divergence — fully explained (NOT a financial-logic error)

The earlier "the Newton diverges on long exact terms so the bisection is a needed
net" was investigated to the bottom. Two distinct findings:

**1. PRIMARY cause — the wrong terminal — is FIXED.** `dosIteratePayment` was using
`repayExactTerminal` (a separate simplified recursion) for plain exact loans. That
recursion does NOT match DOS's actual-day accrual: on a long high-rate exact loan
its overpay-region terminal was ~250x less steep, its zero offset, so the secant
diverged. Proof: the DISPLAY engine `generateFancyScheduleMode` reproduces DOS's
per-period interest to the cent (traced, e.g. 17880.67 / 18164.47 / 18164.34 /
18461.97 — identical to DOS's ComputeNext). Fix: route exact loans' Newton through
`fancyTerminal` (the display engine, unforced), like DOS's Iterate uses
RepayFancyLoan. This closed `TestDOSExactLongTermPayment` (worst relErr 4.6e-7),
i.e. the general long-term exact case now solves by the Newton, no fallback.

**2. RESIDUAL — one ultra-extreme case (573 periods @ 29%) — is a convergence-
ACCEPTANCE difference, NOT wrong math.** Evidence that the schedule/interest logic
is identical:
- Go's `fancyTerminal` matches DOS's terminal to ~9 significant figures at the
  probe point x=52401.93: Go −3,012,592,748.2 vs DOS −3,012,592,734.4 (agree to
  ~4.6e-9 relative). The ~14 difference at the 3-billion scale is float64 rounding
  accumulated over 573 compounding periods at 29% (the overpay balance reaches
  billions). NOTE: on aarch64, FPC's `real` IS 64-bit `double` — the same as Go's
  float64 — so this is not a float-width advantage; it is order-of-accumulation
  rounding on identical arithmetic.
- **Why DOS "converges" and Go doesn't is a MISSING ACCEPTANCE CLAUSE in the port,
  precisely located:** DOS's `Iterate` (AMORTOP.pas:1487-1493) reports success
  unless `(bestp > halfpenny) AND (bestp > acc_limit*init)`, with
  `acc_limit = 2E-8` and `init` = the loan amount. So DOS accepts a residual within
  a RELATIVE tolerance of `2e-8 × amount` — for a $2.16M loan that is ~$0.043, far
  looser than the absolute half-penny (0.005). Go's `dosIteratePayment` returns
  `bestp < halfpenny` ONLY — it omits the `|| bestp <= acc_limit*init` clause. So on
  this billion-scale terminal Go rejects a residual DOS accepts, and falls back to
  the sign-based bisection — which returns the SAME answer to the cent
  (Amortize = 52321.9122 = DOS 52321.9122).

**Bottom line for the client:** there is no outstanding financial-logic error here.
The port's exact schedule reproduces DOS's interest to the cent, and the solved
payment matches DOS to the cent.

**FIXED (2026-07-01):** the acceptance clause was added — `dosIteratePayment` now
returns `bestp < halfpenny || bestp <= accLimit*|amount|` (accLimit=2e-8), mirroring
AMORTOP.pas:1489. With it the Newton converges on the 573-period/29% exact loan
(and the general exact case already converged via the fancyTerminal fix), so the
exact-solve **bisection fallback was REMOVED** — the exact payment solve is now
purely DOS's Newton, no bisection. The skip `refineFancyPayment` fallbacks were also
removed (dead once the Newton is robust). Full suite green.

### (b) Bisection removed from the option solves; the REST is a robustness net, not removable as-is.

Removed the bisection from the option payment branches (they use `dosIteratePayment`).
Findings per remaining caller:

- **Exact-solve fallback — REMOVED (DONE).** The wrong-terminal fix (route exact
  through `fancyTerminal`) plus the acceptance-clause fix (`bestp <= accLimit*amount`)
  made the Newton converge on all exact cases including the 573-period/29% one, so
  the bisection fallback was deleted. Exact payment solve is now pure Newton.
- **Skip `refineFancyPayment` fallbacks — REMOVED (DONE).** Dead once the Newton is
  robust; deleted. (`refineFancyPayment` itself now has no production callers, only
  `coverage_refine_test.go` — safe to delete with its test as cleanup.)
- **In-advance-SIMPLE solve** (`engine.go` ~402): drives `generateSimpleSchedule`
  (basis-360, annuity-due). To move to the Newton it needs a `RepayLoan`
  annuity-due terminal (`repayExactTerminal`'s in-advance branch is *ordinary*, not
  annuity-due) — a small new terminal to write. Currently matches DOS to the cent.
- **Loan AMOUNT / RATE solvers** (`solveFancyAmount` / `solveFancyRate`): DOS uses
  the same `Iterate` with amount/rate as the `var x`. Needs amount/rate terminals;
  these paths already match DOS (no divergences).
- **Skip `refineFancyPayment` fallbacks** and the **moratorium segment solve**
  (`solveSegmentPayment`): only fire when the Newton fails; safe to leave as nets.

Net: the bisection is gone where it caused divergences; what remains is either a
needed robustness net (exact long-term) or a mechanism-purity migration on
already-correct paths (in-advance-simple, amount/rate) that needs a dedicated
terminal each.

---


## Why

The DOS original has **no bisection**. Its only refinement primitive is
`Iterate` — a finite-difference Newton/secant refinement (AMORTOP.pas:1415-1495,
`{This is written as a general Newton's Method refinement of the var parameter x}`).
DOS uses that one function for every solve: payment, loan amount, rate, balloon
amount, and each ARM re-amortization, selecting one of two terminal procedures:

```pascal
{ AMORTOP.pas:1437 (inside Iterate) }
if (fancy) or ((df.c.exact) and (df.c.basis<>x360)) then
    RepayFancyLoan(p, usap, loandate, firstdate, nil, false, til_adj, no_value_calc, 0)
else
    RepayLoan(p);
```

The Go port instead grew a **bisection** (`fancybisect.go`:
`solveFancyPayment` / `fancyBisect` / `fancyOverUnder`, plus `solveFancyAmount` /
`solveFancyRate`) as a convenience, because the *forced* display schedule (which
folds the final row via `WhenToStop`) makes the unforced Newton residual hard to
observe directly. `fancybisect.go`'s own header admits it is "a more robust
[replacement] without touching the forward engine."

**Directive:** the port must not contain financial logic the original lacks.
Remove the bisection; drive DOS's single `Iterate` (already ported as
`dosIteratePayment`) over the *unforced* terminal — the way DOS does.

## What is already proven (this session; implemented, validated, then reverted)

The hard blocker was computing the **unforced terminal** faithfully. Two bugs were
found and fixed, after which the terminal became correct:

1. **One-sided `minpmt` stop.** DOS's `RepayFancyLoan`-in-`Iterate` does not run to
   `very_last` unconditionally. It stops the moment the running balance drops below
   `minpmt` — one-sided: `WhenToStop^.principal < minpmt` (AMORTOP.pas:1195), which
   includes an overshoot to negative. The first attempt ran to `very_last` always,
   so a too-large payment kept subtracting and drove the terminal to −434k.
   Fix: in the unforced walk, `if p < minPmt { break }` (not the two-sided
   `p < minPmt && p > -minPmt`), plus the `veryLast` bound.

2. **`LastDate` not derived.** The terminal helper called the schedule generator
   directly (not via `Amortize`), so `FirstPass` never derived `LastDate` from
   `NPeriods` and `veryLast` was garbage. Fix: derive
   `LastDate = FirstDate + (NPeriods-1) periods` when `!LastOK`.

With both fixes the unforced terminal is **monotone and crosses zero at DOS's exact
payment**. Canonical check (200000 @ 8%, 120 mo monthly, prepay 500×24 replacing the
regular payment, `plus_regular` OFF):

| trial payment x | fancyTerminal(x) |
|---|---|
| 2500 | +77 799.63 |
| 3000 | +9 135.78 |
| **3066.5254 (DOS answer)** | **0.0008** |
| 3100 | −1 487.08 |

So the terminal is right. The remaining work is seed + secant parity + migration.

## Design target

Two functions only, mirroring DOS:

- **`dosIterate`** — the single Newton (already `dosIteratePayment` in
  `fancybisect.go`; generalize the name/target so it also serves amount and rate,
  as DOS's `Iterate` does via its `var x` parameter).
- **`dosTerminal(input, x)`** — returns the *unforced* terminal balance, selecting:
  - `simpleTerminal` (= DOS `RepayLoan`, AMORTOP.pas:1269) for plain non-fancy,
    non-exact loans — the annuity-due (`ff=(f-1)/(2-f)`, in-advance) or
    arrears-with-`prorate` recursion. **Basis-independent.**
  - `fancyTerminal` (= DOS `RepayFancyLoan` with `Output=nil`) for fancy loans OR
    exact-non-360 — the option-aware walk in `generateFancyScheduleMode(..., unforced=true)`.

`repayExactTerminal` (today's exact terminal) is the exact-accrual case of
`fancyTerminal`; unify or keep as the exact branch.

## Remaining steps (each with a hard validation gate)

### Step 1 — Land the unforced terminal (proven; re-apply cleanly)
- Add `unforced bool` to the fancy walk (`generateFancyScheduleMode`), keeping
  `generateFancySchedule` as the `unforced=false` wrapper (display path byte-for-byte
  unchanged). Gate: the final-row fold chain, the two-sided balance break (→ one-sided
  `p < minPmt` when unforced), the `veryLast` break (unconditional when unforced), and
  the off-cycle `offCyclePaidOff` early return (skip when unforced).
- `fancyTerminal(input, x, ...)`: set `PayAmt=x`, derive `LastDate` if `!LastOK`,
  return `generateFancyScheduleMode(...unforced=true).FinalPrinc`.
- **Gate:** unit test asserting `fancyTerminal` is monotone-through-zero at the DOS
  payment for a battery of loans (the table above). No engine wiring yet.

### Step 2 — Seed parity with DOS `EstimateAndRefinePayment` (THE crux)
DOS seeds `Iterate` from a closed form that subtracts the present value of every
balloon and prepayment (Amortize.pas:384-401):
```
adjp := amount
for balloons:      adjp -= amount_i * exxp(-rate * YearsDif(balloon_i.date, repay_from))
for prepayments:   FirstLastAndFF → adjp -= payment_i * (first - last*ff)/(1-ff)   (else *nn)
d := adjp * (f-1) / denom        {denom = 1 - exxp(-nrepay*lnn(f))}
```
`rate = RateFromYield(loanrate, peryr)` (true rate); `repay_from = loandate`
(prepaid off); `ff = exxp(-rate/peryr)`; `first/last` use the prepay start/stop
dates (stop derived from `NN` = start + (NN-1) periods when blank).

Why this matters: the prepayment-**replace** terminal is ill-conditioned (multiple
near-zeros — e.g. a spurious one at the no-prepay annuity 2426.55 alongside the true
root 3066.53). DOS lands on the correct root **because it starts from this exact
seed and its secant + `bestx` walk from there**. A different seed → the secant
settles on a different near-zero. So the seed must match DOS to the cent.

- **Validation tooling:** DOS's raw seed is not printed by the current oracle. Add a
  `seed`-dump mode to `legacy/oracle/amort_oracle.pas` (instrument a *staged copy* of
  the DOS units — same technique as the AO7 bug post-mortem; do NOT edit read-only
  `legacy/src`) that prints `adjp` and `d` from `EstimateAndRefinePayment` before
  `Iterate`. Then a differential test asserts Go's `adjp`/seed == DOS's across a sweep.
- **Gate:** Go seed matches DOS seed to ≤1e-6 relative on a 1000-case balloon +
  prepayment (additive AND replace) sweep, all bases, all pmts/yr.

### Step 3 — Secant parity with DOS `Iterate`
`dosIteratePayment` already ports the loop (finite-difference secant, divergence
brake `abs(newdelta/delta) > 1 → count += 5`, `bestx` captured *after* the `x`
update, converge at `bestp < halfpenny`, 20-iteration cap). Verify it reproduces
DOS **step for step** given the same seed and terminal — because on an
ill-conditioned terminal the *path* determines which near-zero is returned.

- **Validation tooling:** extend the oracle `seed`-dump to also print each `Iterate`
  step's `x` and `p` (guarded debug copy). Differential test: feed Go's `dosIterate`
  the DOS seed and assert the per-step `(x, p)` trace matches DOS, and the final
  `bestx` matches DOS's solved payment to the cent.
- **Gate:** trace parity on the same 1000-case sweep; final payment 0 divergence.

### Step 4 — Migrate all payment-solve callers off the bisection
Replace every `solveFancyPayment(...)` payment caller (≈9 sites: `engine.go`
dispatch branches for skip / balloon / target / in-advance / odd-first / prepayment /
adjustment, `backward.go` `SolvePaymentClosedForm`, `fancybisect.go`
`solveSegmentPayment`) with `dosIterate` over the correct terminal. Each caller sits
in a specific dispatch context — port them one at a time, re-running the full oracle
suite after each, not all at once.

- **Gate after each caller:** `TestFuzzAmortizePaymentVsDOS` (all 6 basis/mode
  variants), `TestDOSInAdvanceFancyFuzz`, `TestDOSAmortFancySettingsCube`,
  `TestDOSGroundZeroRowCube`, and the full package — all green, and
  `TestUIAmortSweepVsDOS` total-interest fails strictly decreasing toward 0.

### Step 5 — Amount and rate solves (same principle)
DOS solves loan amount (`EstimateAndRefineLoanAmount`) and rate
(`EstimateAndRefineRate`) with the **same** `Iterate` (different `var x` target).
The Go `SolveLoanAmount` / `SolveRate` use `solveFancyAmount` / `solveFancyRate`
(bisection). Migrate these to `dosIterate` targeting amount/rate. NOTE: the closed
form (`SolvePaymentClosedForm`'s `adjp`) is shared with the amount round-trip — this
is exactly what regressed `TestFancyBackwardPrepaymentRoundTrip` when changed in
isolation, so amount/rate must move together with the seed change, not after it.

- **Gate:** `TestDOSFancyBackwardAmountRateRoundTrip`, the fancy backward
  round-trip/property tests, all green.

### Step 6 — Delete the bisection
Once no caller remains, remove `fancyBisect`, `solveFancyPayment`, `fancyOverUnder`,
`solveFancyAmount`, `solveFancyRate`, and `fancyBisectTol`. Confirm nothing imports
them. This is the point at which the port contains only DOS's `Iterate`.

- **Gate:** full `go test ./...` green; `TestUIAmortSweepVsDOS` at **0** total-interest
  divergences; grep confirms no bisection symbols remain.

## Known hazards / gotchas

- **Ill-conditioned replace-mode terminal** — multiple near-zeros. Only exact
  seed + exact secant path (Steps 2-3) lands on DOS's root reliably. This is why the
  work must be trace-validated against an instrumented oracle, not just end-value
  compared.
- **Shared closed form** feeds payment AND amount/rate — change them together (Step 5
  note).
- **Exact vs non-exact accrual** in the terminal: exact non-360 needs actual-day
  (`repayExactTerminal`); non-exact fancy uses the month-based `periodYearFraction`
  in `generateFancyScheduleMode`. The terminal dispatcher must pick correctly.
- **In-advance stays split** (per Fix A, docs/ui_sweep_findings.md #A): the payment is
  the basis-independent simple (`RepayLoan`) solve; only the non-360 *display* uses the
  fancy engine. `dosTerminal`'s simple branch must be the annuity-due `RepayLoan`
  recursion (no first-period proration), not `repayExactTerminal` (which is ordinary).
- **Do not touch `legacy/src`** — instrument staged copies for the oracle seed/trace
  dumps.

## Effort estimate

Steps 1, 3 are largely done/ported. Step 2 (seed parity + oracle instrumentation) and
Step 4 (careful per-caller migration with full revalidation) are the bulk. Realistic:
a focused multi-session pass with the oracle trace harness as the backbone. The bar is
zero divergences across the ~15k existing oracle cases **and** the UI sweep.

## Pointers

- DOS: `legacy/src/dos_source/AMORTOP.pas` — `Iterate` (1415), `RepayLoan` (1269),
  `RepayFancyLoan` (1101), `WhenToStop` fold (1195). `Amortize.pas` —
  `EstimateAndRefinePayment` (377), `FirstLastAndFF` (370),
  `EstimateAndRefineLoanAmount`, `EstimateAndRefineRate`.
- Go: `fancybisect.go` (bisection to remove + `dosIteratePayment`, `repayExactTerminal`),
  `engine.go` (`generateFancySchedule` dispatch + the fancy walk), `backward.go`
  (`SolvePaymentClosedForm`, `SolveLoanAmount`, `SolveRate`).
- Context: `docs/ui_sweep_findings.md` (buckets A/B/C, A closed), this session's
  validated terminal fixes.
