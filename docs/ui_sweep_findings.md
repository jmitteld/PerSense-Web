# Full-system UI sweep vs the DOS oracle (2026-07-01)

A few-hundred-case-per-section differential sweep was run against the real DOS
oracle, checking **intermediary** (per-row / per-line) values in addition to
final answers. The sweep drives the same engine functions the API handlers call.

## Present Value — CLEAN

The existing PV oracle sweeps cover the UI space and were re-run:

- lump 400, periodic 600, variable-rate 500, month-specific COLA 1200
- all seven backward-solve paths (amount / date / rate / as-of / to-date /
  from-date) — hundreds each
- **per-row (intermediary)**: multi-row 500 cases + 869 individual line PVs;
  VR multi-row 869 line PVs

Total ~7,000+ cases. **0 divergences**, max relErr ~1e-9 (bit-exact).

## Mortgage — CLEAN

- dispatch sweep 128 cells, dispatch cube 540 cells, solve-monthly 500,
  solve-price 300, APR 400, down-payment dispatch 500, compare 486
- **intermediary (What-If generated rows)**: VaryRate 750, multi-field 1200,
  VaryMonthly 750

Total ~4,000+ cases. **0 divergences**, max relErr ~1e-9.

## Amortization — intermediary CLEAN; three pre-existing FINAL-answer gaps

`TestUIAmortSweepVsDOS` (report-only) drove 316 randomized cases across the
Advanced-Options × settings space.

**Intermediary (per-row accrual, DOS payment fed to both engines): essentially
clean.** Given a payment, the Go schedule reproduces the DOS schedule row for
row; the only per-row diffs seen were 1–5 cents of 365-basis rounding on very
long loans (80+ periods), at the tolerance boundary.

**Final-answer (solved payment / total interest) divergences — all pre-existing,
none introduced by the in-advance × fancy work:**

### A. Plain in-advance × non-360 basis  (largest bucket, ~41/316; normal inputs)

The simple (non-fancy) in-advance schedule is **basis-blind**: it produces
identical total interest on the 360, 365, and 365/360 bases, while DOS accrues
actual-day interest on the non-360 bases (DOS routes every non-360 loan through
`RepayFancyLoan`). Example — 90 285 @ 12.48 %, 36 × quarterly, in-advance:

| basis | DOS total interest | Go total interest |
|-------|--------------------|-------------------|
| 360   | 61 545.10          | 61 545.08  ✓      |
| 365   | 65 837.05          | 61 545.08  ✗ (−6.5 %) |
| 365/360 | 65 883.74        | 61 545.08  ✗ (−6.6 %) |

The solved payment is even identical across bases in DOS (4139.2558) — only the
accrual/reported interest differs. On the 360 basis Go matches to the cent.

Root cause: `generateSimpleSchedule`'s in-advance path does not honor the basis
day-count. DOS-faithful fix would route non-360 in-advance loans through the
(basis-aware) fancy engine, or make the simple in-advance accrual basis-aware —
mirroring the earlier `firstPeriodProrate` / `periodYearFraction` basis fixes
that were applied to the arrears/fancy paths but not the simple in-advance path.
Reachable from the UI (basis and interest-in-advance are independent toggles).

### B. Prepayment blank-payment SOLVE precision  (~45/316; unusual inputs)

When the regular payment is left blank and a fixed periodic prepayment series is
supplied, Go's solved regular payment differs from DOS by ~1–14 % of total
interest. The forward SCHEDULE given a fixed payment matches DOS to the cent
(validated separately); this is purely payment-solve precision. Many of these
cases are pathological — a small prepayment that *replaces* a large regular
payment (PlusRegular off) drives heavy negative amortization, where total
interest is extremely sensitive to the solved payment. This solve was materially
improved on 2026-07-01 (previously it returned the option-blind seed and the
loan did not retire); tightening it to the cent is the remaining work.

### C. Balloon SOLVE on short / annual terms  (~6/316; edge inputs)

A few balloon payment-solves on short annual loans (e.g. a balloon on the final
payment of a 3-year annual loan) diverge. Edge cases at the boundary of the
solve's domain.

## Status (updated 2026-07-01)

**A — FIXED.** The source crawl showed DOS renders an in-advance loan with two
engines: the PAYMENT is solved by `Iterate→RepayLoan` (the annuity-due recursion,
basis-INDEPENDENT — no first-period proration, AMORTOP.pas:1437/1276), and the
SCHEDULE is rendered by `RepayFancyLoan` for any non-360 basis (actual-day accrual,
AMORTOP.pas:1493) with `WhenToStop` folding the residual into the final row. The
port now reproduces that split (`engine.go`: proration gated on `!InAdvance`; the
non-360 in-advance schedule display routed to the fancy engine; the payment solved
on the simple basis-360 model; the final-row fold). Validated to the cent — payment,
every intermediary row, folded final row, and total interest — on 360/365/365-360.
Unit tests: `dos_inadvance_basis_test.go` (a basis-independence invariant + a
row-by-row oracle check). UI-sweep total-interest fails dropped 98 → 57.

**B (prepayment-replace solve) and C (short/annual balloon solve) — OPEN, and the
correct fix is architectural.** DOS has NO bisection; its only refinement primitive
is the Newton/secant `Iterate` (AMORTOP.pas:1416). Go added a bisection
(`solveFancyPayment`/`fancyBisect`) as a non-DOS convenience because the forced
display schedule makes the unforced Newton terminal hard to observe. The right fix
— and the one that closes B/C — is to remove the bisection and drive DOS's single
`Iterate` over the *unforced* terminal (as DOS does: `RepayFancyLoan` with
`Output=nil`).

A first implementation of that unforced terminal (`generateFancyScheduleMode` with
an `unforced` flag that skipped the final-row fold and ran to `veryLast`) was built
and reverted: it did NOT reproduce DOS's `RepayFancyLoan`-in-`Iterate` semantics
(which still fold when `principal < minpmt` via `WhenToStop`, with specific stop
conditions) — the terminal was far from zero at DOS's solved payment on the
prepayment-replace case, so the Newton diverged. Replicating that terminal exactly
for every option combination without destabilizing the ~15k passing oracle cases is
a substantial, focused piece of work; it is the tracked next step. Note the diverging
cases are unusual inputs (a blank regular payment beside a prepayment series that
*replaces* it, causing negative amortization; a balloon on the final payment of a
short annual loan), and the per-row engine given a fixed payment is already exact.
