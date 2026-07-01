# DOS-Fidelity frontier — found, then closed

This documents a corner of the amortization engine that once diverged from the
real DOS engine, how the exhaustive `Amortize` dispatch sweep found it, and the
two fixes that closed it. It is now a strict zero-divergence regression guard,
not an open gap.

## What the sweep validates (zero divergence vs the real DOS engine)

Driven through the FULL `Amortize` field-presence dispatch (the path the API/UI
use), blank payment solved, compared to the DOS oracle (`legacy/oracle/`) —
`internal/finance/amortization/dos_amortize_dispatch_sweep_test.go`:

- **Blank payment + a known balloon** — 250 cases, 0 divergences.
- **Blank payment + an odd first period, plain 360 / prepaid-OFF** — 250 cases, 0.
- **Cross-product: basis {360, 365} × prepaid {off, on} × balloon {none, present}
  × balloon-includes-regular {off, on}, natural first period** — 658 cases, 0
  (tolerance 1e-3; 365-day basis accrues a few cents of per-period rounding).
- **Odd first period × {prepaid | balloon | 365}** — `TestDOSOddFirstFancyFrontier`,
  ~490 cases, **0 divergences** (was 20% before the fixes). Now a strict guard:
  any divergence > 1e-3 fails CI.

On top of the pre-existing ~6,400-case oracle sweeps for the backward solvers and
forward schedules.

## The frontier that was found

An *odd* first period (a short or long first-payment gap — e.g. an annual loan
whose first payment is 1 or 18 months out) combined with prepaid interest, a
balloon, or the 365-day basis. ~20% of those combinations diverged, up to ~13–18%
of the payment on extreme inputs. Two independent causes:

1. **Payment solve (prepaid / 365 odd-first).** The closed-form payment estimate
   assumes a full first period. For an odd first period the regular payment must
   be adjusted so the loan amortizes over the stated term. DOS does this by
   iterating the estimate (`EstimateAndRefinePayment`, Amortize.pas:416); the Go
   port previously augmented only the prepaid-OFF case and left prepaid / 365
   odd-first on the un-refined estimate.

2. **Off-cycle balloon placement.** DOS applies a balloon on its exact date —
   accruing partial interest up to it, then the next regular period accrues only
   from the balloon date (`RepayFancyLoan`, AMORTOP.pas:608-613). The Go port
   folded a balloon that fell between two regular dates into the *next* regular
   payment. An odd first period shifts every payment date off the balloon's
   monthly grid, so the balloon lands between dates — which is when this mattered.

## The fixes (both DOS-faithful)

1. **`oddFirstPeriod` + schedule-oracle refinement** (`engine.go`). When the first
   payment is not exactly one period out, the blank-payment solve refines the
   closed-form estimate with the bisection that drives the *unforced* terminal
   balance of the real (prorated) forward schedule to zero — DOS's `Iterate`,
   expressed against the actual schedule. A snap guard keeps an already-exact
   estimate untouched (no sub-cent bisection noise) and only adopts a materially
   different refined payment.

2. **Off-cycle balloon draining** (`generateFancySchedule`). A balloon dated
   strictly before the next regular payment is now emitted as its own dated row
   at its actual date, reusing the existing off-cycle prepayment draining: partial
   interest to the balloon date, then the next regular period accrues from there.

## Verification

`amort_oracle` gained `loandmy=` / `firstdmy=` date overrides so the differential
rig can drive odd-DAYS first periods (loan day-of-month ≠ first day-of-month),
which the month-only `first=` could not express. That surfaced a genuine
DOS-vs-Windows discrepancy (AM Example 1): for a 19-day short first period DOS
augments the payment to $731.98 while the Windows help shows the plain $733.76.
Per CLAUDE.md the DOS engine is the authority, so the port matches $731.98. See
`docs/discrepancies.md` §7.

Regression coverage: `TestDOSOddFirstFancyFrontier` (strict, oracle),
`TestAPIAmortOffCycleBalloonMatchesDOS` (DOS-pinned, no oracle needed),
`TestVerifyWebAM_EX1_Simple` (asserts the DOS values).

## Two findings from the exhaustive settings cube (2026-06-17)

The Phase-1 settings cube (`TestDOSAmortSettingsCube`,
`docs/exhaustive_option_sweep_plan.md`) enumerates basis{360,365} × prepaid ×
in-advance × exact × pmts/yr against the DOS engine. It surfaced two real gaps.

### Fixed: in-advance blank-payment solve

The closed-form payment estimate is the ordinary in-arrears annuity, but DOS's
shortcut excludes in-advance and iterates the annuity-due
(Amortize.pas:402-416). The blank-payment solve produced the ordinary payment for
**in-advance (annuity-due)** loans — 256 of 512 cube cells diverged, up to 9%. The
`needRefine` change in `engine.go` (refine the estimate against the real schedule,
which already models in-advance timing) closed it on the 360 basis and in every
pair of flags. Now 0 divergence outside the corner below.

### RESOLVED (2026-06-19): the `Exact` interest setting is now implemented

Previously the **"Exact method" setting was not functional end-to-end** (the API
hardcoded `Exact: false`, the engine ignored `settings.Exact`, and the UI toggle
was hidden), so selecting the 365 basis gave the standard 30/360 approximation —
the bug a client reported. It is now implemented (option a): the request carries
`exact`, the engine accrues actual-day interest with an iterated payment solve
(`exactDaily` in `engine.go`; DOS AMORTOP.pas:625 `YearsDif` branch + non-360
routed through `RepayFancyLoan`, Amortize.pas:1493), and the UI toggle is live.
On the 360 basis Exact stays a no-op, matching DOS.

Validated row-for-row against the real DOS oracle by the new exhaustive suite
`TestDOSGroundZeroRowCube` (every UI row column, all bases × methods × prepaid):
the 365 exact schedule matches to the cent. See
`docs/exact_groundzero_findings.md` and `docs/postmortem_365_exact_interest.md`.

**CLOSED (Revision 12, 2026-06-22): exact × in-advance** (annuity-due). True
daily accrual is now implemented for the in-advance branch via a dedicated
schedule path (`generateExactInAdvanceSchedule`) reproducing DOS's distinct SHAPE
— a row-0 settlement-interest row at the loan date, a one-period base-date shift,
and `n-1` actual-day amortizing rows — with the payment solved by the in-advance
branch of `dosIteratePayment`/`repayExactTerminal`. `TestDOSGroundZeroRowCube`
now classifies exact×in-advance (non-360 basis) as CLEAN (rows + payment to the
cent), and `TestDOSExactInAdvanceSettlement` validates the settlement row and
totals via `dumpraw`. The `frExactInadv` envelope guard remains as a 0-valued
backstop. Remaining in-advance frontiers: the NON-exact (whole-month) annuity-due
schedule and 360-basis in-advance (exact inert) — see below — plus the
non-360 closed-form payment day-count gap (`docs/exact_groundzero_findings.md`
§4–5).

### Fixed: skip-month blank-payment solve

The Phase-2 fancy×settings cube (`TestDOSAmortFancySettingsCube`) crosses a
moratorium and skip-months with the settings cube. It surfaced that the
**skip-month blank-payment solve** was ~18% high vs DOS: the skip branch used
`refineFancyPayment`, which bisects on `FinalPrinc` — but `generateFancySchedule`
*forces* the final payment, pinning `FinalPrinc` to ~0, so the bisection signal was
degenerate. Routing skip through `solveFancyPayment` (which reconstructs the
*unforced* residual via `fancyOverUnder`) fixed it. Skip and moratorium are now 0
divergence across the whole settings cube — except in-advance (below).

### Two in-advance corners — distinct causes (corrected after diagnosis)

An earlier draft of this note guessed both corners shared a `fancyOverUnder`
root cause. A direct diagnosis (feed DOS's payment into the Go forward schedule,
probe `fancyOverUnder`'s sign) showed that is **wrong** — they are unrelated:

**1. `365 × in-advance × exact`** (`TestDOSAmortSettingsCube`, ~9–13%) — this is
just the unimplemented **Exact** setting (§ above): the engine ignores
`settings.Exact`, so it correctly solves the *non-exact* in-advance payment while
DOS applies exact interest. `fancyOverUnder` is fine here. **Now unreachable from
the product** — the Exact UI toggle is hidden (`docs/discrepancies.md` §8), so a
user cannot set exact; the cube only reaches it by driving the oracle directly.
Bounded by `TestDOSAmortSettingsCube` (0.30).

**2. `in-advance × fancy` (skip / moratorium / balloon)** — **CLOSED (2026-07-01).**
Once a loan is fancy, DOS routes it through `RepayFancyLoan`, which for an
in-advance loan (AMORTOP.pas:1159-1187):

- emits a **settlement-interest row** at the loan date (interest =
  `amount·rate·<one period>`, principal 0, balance unchanged — the annuity-due
  "time-0" interest), then
- **shifts the base date one period forward** so the first amortizing payment
  lands at `FirstDate + 1 period`, and
- accrues **ordinary** opening-balance interest on the shifted walk (verified: a
  fancy in-advance loan's amortizing-row interest is `balance·rate·<period>`, NOT
  the annuity-due `(p-d)·(f-1)/(2-f)` the plain `generateSimpleSchedule` uses —
  ComputeNext uses plain in-arrears interest even when `in_advance` is set,
  AMORTOP.pas:636).

So for a fancy loan, in-advance changes the schedule via the payment-timing
**structure**, not the interest formula.

**The fix** (`generateFancySchedule`, the `inAdvanceFancy` block): emit DOS's
settlement row, shift the base one period (`currentDate = FirstDate+1period`,
`prevDate = FirstDate`), and REMOVE the former post-payment interest recompute —
the loop's ordinary opening-balance interest is already the correct DOS value.
The settlement stub uses actual-day `YearsDif` (DOS `PrepaidInterest`), so it is
right on the 365 basis too. The moratorium boundary recompute was corrected for
the shift (the loan amortizes over `n-1` rows, so `remaining--`), and
`moratoriumActive` is keyed on the shifted first amortizing date so a moratorium
that no longer produces interest-only rows does not fire a spurious recompute.

The two earlier ruled-out attempts (annuity-due factor `(f-1)/(2-f)`; dropping the
recompute to pure ordinary interest without the structure) failed because they
tweaked the FORMULA, not the STRUCTURE. This fix supplies the structure.

**Validation.** `TestDOSAmortFancySettingsCube` in-advance cells are now **strict
0 divergence** (was a 0.10 envelope). `TestDOSInAdvanceFancyFuzz` drives ~3,300
randomized in-advance fancy loans (skip / moratorium incl. deep-biting / balloon /
prepayment × basis{360,365,365/360} × prepaid × pmts/yr) against the real DOS
oracle and asserts, for skip / moratorium / balloon-non-coincident, both the
solved payment AND every schedule row (interest, principal, balance) match to the
cent — 0 strict divergences.

### The two in-advance-fancy sub-frontiers the fuzzer found — both now resolved (2026-07-01)

- **Prepayment trailing row from an NN-derived stop date — CLOSED.** When a
  periodic prepayment series is specified by count (`NN`) with no explicit stop
  date and the derived last extra falls past the loan's last payment date, DOS's
  `DetermineVeryLast` (AMORTOP.pas:1293-1304) extends the schedule to that derived
  stop date; the loan's residual is retired by those trailing extras. Go's
  `veryLast` previously extended only for an EXPLICIT StopDate, so an NN-only
  series was cut one row short, leaving the balance unretired. Fixed in
  `generateFancySchedule`'s `veryLast` block by deriving the stop date =
  `StartDate + (NN-1)` prepayment periods (mirrors DOS for arrears and in-advance).
  This also surfaced that the prepayment blank-payment SOLVE was never refined (the
  dispatch's every branch excluded `hasPrepay`, leaving the option-blind closed-form
  seed): added a `hasPrepay` branch that refines via the schedule-oracle bisection,
  and fixed `fancyOverUnder` so a trailing prepayment row (PayAmt below the regular
  payment, since a prepayment REPLACES it when PlusRegular is off) is not mistaken
  for a forced final regular payment. Now strict 0 divergence (payment + every row)
  in `TestDOSInAdvanceFancyFuzz`.

- **Balloon ON or before the first payment date — RESOLVED (balloon-before is an
  error; balloon-on-first in-advance is a deliberate divergence).**
  - *Balloon strictly BEFORE the first payment date*: DOS rejects it in BOTH
    arrears and in-advance ("Balloon cannot precede first regular payment"). Go
    already returns the equivalent error, so the two agree.
  - *Balloon ON the first payment date, ARREARS*: already matched DOS to ~1e-7
    (the balloon coincides with / replaces the first regular payment).
  - *Balloon ON the first payment date, IN-ADVANCE*: a **deliberate divergence** —
    see the section below. DOS's in-advance init consumes the balloon via the dead
    `firstd` path (AMORTOP.pas:1166-1178): the solved payment is inflated as if the
    balloon existed, but the balloon is never applied to principal nor collected in
    the totals (`paid` excludes it). Go instead applies the balloon, collects it,
    and retires the loan with the correct (lower) interest. We keep the correct
    answer and do not reproduce the DOS bug. `TestDOSInAdvanceFancyFuzz` asserts
    Go's schedule retires to ~0 on these (74/3,300 cases) rather than matching DOS.

## Findings from the R78/USA cube (2026-06-17)

`TestDOSAmortR78USACube` crosses Rule-of-78 and the US Rule with basis {360,365}
× pmts/yr, comparing every schedule row to DOS.

### Fixed: Rule of 78 must be a no-op on the 365 basis

DOS routes any non-360 basis through `RepayFancyLoan` (Amortize.pas:1493), the
standard per-period walk, which does **not** apply the sum-of-digits split — so on
the 365 basis R78 is silently inert and the borrower gets ordinary amortization
interest. Go applied the R78 split regardless of basis. Fixed by gating R78 to
`settings.Basis == types.Basis360` (`engine.go`), verified: DOS R78+b365 ≡ DOS
plain b365 (identical rows), now matched.

### Fixed: first-period prorate on the 365 basis

Precisely root-caused (diagnostic: compare Go vs DOS row-by-row). The per-period
growth `f-1` is correct and basis-independent, and rows 2…n already used `p*(f-1)`
exactly. The only divergence was **row 1**: the prorate was
`YearsDif(firstDate, loanDate, Basis365) * perYr`, so a calendar-natural first
period — 182 days on a 366-day leap year — prorated to 182/366 × 2 = 0.9945 instead
of 1.0 (interest 1243.17 instead of 1250.00). DOS treats a clean-boundary first
period as a **whole** period (prorate = 1) using month arithmetic, reserving
actual-day counting for genuine sub-period day stubs (loan day-of-month ≠ payment
day). Because a year's day-fractions sum to 1 the payment/totals already agreed —
only the per-row split (and the small drift it carried) was off, only on 365.

**Fix:** `firstPeriodProrate` (`engine.go`) returns the exact month-based fraction
`monthsGap / (12/perYr)` when the dates land on clean period boundaries (matching
day-of-month, month-dividing frequency), and falls back to the basis-specific
actual-day `YearsDif` only for genuine odd-day stubs. It is shared by the schedule,
the iterative solver (`RepayLoan`), the payment augmentation, and `oddFirstPeriod`,
so the solver's model matches the schedule. On the 360 basis it is a no-op (30/360
already makes a whole month exactly 1/12 year); odd-day stubs (already DOS-faithful)
are untouched. Verified: `TestDOSAmortR78USACube` now strict 0 divergence on **both**
bases; `TestDOS365BasisMonthlyFirstPeriod` updated to assert the match; the
odd-first/odd-days sweeps (`TestDOSOddFirstFancyFrontier`,
`TestDOSOddDaysFirstPeriodSweep`) still pass.

### Fixed: fancy-schedule per-row accrual on the 365 basis

`TestDOSAmortFancy365RowCube` extended the per-row basis check to FANCY loans
(balloon → `generateFancySchedule`). It found the `firstPeriodProrate` fix had
reached only `generateSimpleSchedule`: `generateFancySchedule` accrued each
period's interest with `loan.LoanRate * YearsDif(currentDate, prevDate, Basis)`,
so on the 365 basis every row oscillated against DOS's per-period-rate accrual
(31- vs 28-day months over/under-charging).

**Fix:** `periodYearFraction` (`engine.go`) — the fancy-loop analog of
`firstPeriodProrate` — returns the month-based fraction `months/12` for clean
whole/odd-month boundaries (basis-independent, matching `p*(f-1)`) and the
basis-specific actual-day `YearsDif` only for genuine partial spans (off-cycle
balloon/prepayment remainders, day stubs). It is used by both the non-Daily
regular accrual and the in-advance recompute; Daily compounding keeps the true day
count. Verified: `TestDOSAmortFancy365RowCube` is now strict 0 divergence on
**both** bases (was 256 rows / max 132). The off-cycle prepayment and balloon
sweeps still pass (partial-period spans correctly keep actual-day counting).

---

## Deliberate divergences (DOS bugs we do NOT reproduce)

There are two places where the Go engines intentionally disagree with the DOS
oracle because the DOS behavior is a confirmed bug producing financially-
nonsensical output. The second — **a balloon dated ON the first payment date under
in-advance** (DOS inflates the solved payment but never applies or collects the
balloon; Go applies it and retires correctly) — is described in the in-advance ×
fancy sub-frontier section above and guarded by `TestDOSInAdvanceFancyFuzz`. The
first is detailed here.

### date-only (AO7) / payment-only (AO6) adjustment + a later balloon (2026-06-25)

This is the single place where the Go engines **intentionally** disagree with the
DOS oracle, because the DOS behavior is a confirmed **bug** that produces
financially-nonsensical output. Decision (with the product owner): keep the
correct result in both Go engines; do NOT reproduce the DOS bug.

### Symptom

A **date-only** adjustment (AO7 — a date with neither a new rate nor a new
payment) OR a **payment-only** adjustment (AO6), combined with a **balloon dated
AFTER the adjustment**, makes DOS retire the loan EARLY with roughly half the
interest. Canonical case — `100000 @ 6%, 24 mo, adj@month6 (date-only), balloon@month12 = 20000`:

- DOS oracle: interest **3172.08**, loan "paid off" at the first payment after
  the adjustment, with a bogus final-row date of `1/1/2026` (month 24) and the
  message `re-computed at 0.0000%: Payment fixed at 0.00`.
- Both Go engines (the piecewise default AND the faithful port): interest
  **6331.47**, loan amortizes correctly to term — the financially correct answer,
  and identical to the same loan with no adjustment.

An **explicit-rate** adjustment at the same rate (`adj=6:0.06:`) does NOT trigger
it (continues to term, interest 6331.47) — so it is specifically the blank-rate
(date-only / payment-only) adjustment that is buggy.

### Root cause (instrumented confirmation)

The DOS engine units were instrumented with debug output (a staged override copy;
the read-only `legacy/src/dos_source` was left untouched). Findings:

- `Re_Amortize` is **byte-identical** for the buggy date-only case and the normal
  explicit-rate case: `n=19, adjp=61772.93, denom=0.085864, d=3597.14` — the
  re-amortized payment is unchanged in both. **So this is NOT a financial-logic
  divergence.**
- The split happens AFTER, in DOS's build-path print/re-amortize recursion
  (`DecideWhetherToPrintALine` → `PrintAndReset`, AMORTOP.pas:953-1077): for a
  date-only adjustment the post-adjustment row's *date* corrupts to `very_last`
  (month 24), which trips the very-last payoff fold (AMORTOP.pas:1004) and retires
  the loan. It is a **display/print-path state-corruption bug**, not a calculation.

### Why we don't reproduce it

Reproducing it bug-for-bug would mean porting DOS's recursive print/date-handling
corruption into the validated build-path walk — high-risk (it could destabilize
the zero-divergence state the port otherwise holds), and it would replicate output
that is financially meaningless (a loan "paid off" 17 months early on a bogus
date). The Go result is correct and self-consistent.

### How it's handled in code

- `dosPortCanHandle` routes AO6/AO7 + balloon to the piecewise engine (both engines
  agree on the correct answer, so this is behavior-preserving). See
  `internal/finance/amortization/dosport_entry.go`.
- Guarded by `TestAO7BalloonDOSBugCharacterization` (pins both Go engines at the
  correct 6331.47 and asserts they agree) and `TestAO7BalloonOracleIsBug` (opt-in;
  confirms the oracle still exhibits the 3172.08 bug, so the rationale stays honest
  if the oracle is rebuilt). See `dosport_ao7balloon_edge_test.go`.
