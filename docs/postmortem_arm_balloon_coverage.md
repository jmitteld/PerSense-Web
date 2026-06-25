# Post-mortem: how a real DOS divergence survived near-100% unit-test coverage

**Bug.** Solving the payment for an amortization loan that has BOTH a rate adjustment (ARM)
AND a balloon produced a schedule that did not retire — leaving a final balance of
~$1,322 and total interest off by **$12k–$25k** versus the authoritative DOS engine. Two
related combinations (skip-months that include the first payment; a second balloon after the
adjustment) diverged the same way. (`docs/amort_option_combo_divergences.md`.)

**Status.** ARM+balloon is fixed and DOS-matched (regression test
`internal/api/amort_arm_balloon_test.go`). The skip-first-payment case and a small
two-balloon tail remain (see end).

**The uncomfortable part.** The amortization engine carried **near-100% line coverage**. Every
line of the adjustment (AO5/AO6/AO7) re-amortization and every line of the balloon code was
executed by the test suite. The bug still shipped. This post-mortem is about *why coverage said
"done" while the behavior was wrong*, and what we changed so the next one is caught.

---

## Why coverage didn't catch it

### 1. Coverage measures *lines executed*, not *combinations of inputs*
The adjustment code was covered by adjustment-only tests. The balloon code was covered by
balloon-only tests. Both ran green; both reported high coverage. But the **defect lived in the
interaction** — the adjustment's re-amortization needs to account for a balloon, and the base
payment solve needs to be balloon-aware *while* an adjustment is present. No single line is
"the bug"; the bug is in a path that only exists when **adjustment ∧ balloon** are both set.
Line coverage is blind to this: executing line A in test 1 and line B in test 2 counts both as
covered, but never exercises A-then-B-with-shared-state. The real coverage metric that mattered
— **the cross-product of advanced options** — was a few percent, not 100%.

### 2. The tests that existed were *self-referential*, not *oracle-checked*
Most engine tests assert internal consistency: round-trips (solve, then back-solve, recover the
input) and invariant checks (does the schedule run, does it look monotonic). Those pass even
when the whole schedule is **consistently wrong**: a payment that under-amortizes round-trips
back to itself, and an invariant like "balance decreases" holds for a schedule that ends at
$1,322 instead of $0. The only test that would have failed is a **differential** one —
"does the number equal what the real DOS engine produces?" — and for this *combination* there
was none. The DOS oracle existed (`legacy/oracle/amort_oracle`) and was wired into "cubes," but
the cubes swept **single** options (basis × prepaid × in-advance × exact; the mortgage dispatch
cube), not **option × option**.

### 3. A test actually *codified the bug*
`TestAO7AdjustmentReamortizesAtCurrentRate` asserted that a date-only adjustment on a
balloon loan makes the payment "drop below the baseline." That was the *symptom* of the bug
(the base payment was balloon-blind, so re-amortizing dropped it). The test was written against
observed Go behavior, not against DOS — so it locked the wrong behavior in and turned green.
Green tests that encode current behavior give false confidence; this one had to be **corrected**
as part of the fix (it now asserts the DOS value, ~$869.57, verified against the oracle).

### 4. The gap was *known but invisible to CI*
The code comment at the AO5 site literally said *"Exact retirement on the most pathological
cases still requires the full DOS Iterate routine — task #103."* The limitation was understood
and written down — but a TODO in a comment is not a failing test. Nothing in CI was red, so the
known gap stayed a backlog note rather than a tracked defect with a reproduction.

### 5. The bug hid in a *plausible* output
The wrong schedule didn't crash or print a wild number. It ran the full 360 rows, every row
looked reasonable, and it ended at $1,322 — a number a human skimming the table would not flag.
"Looks plausible" is exactly the failure mode unit tests are supposed to cover and coverage
metrics are supposed to reassure us about. Both missed it.

---

## What actually found it

A **differential, combinatorial** check: the new exhaustive Amortization case set
(`internal/api/testdata/amort_ui_cases.json`, 109 cases) deliberately includes option
*combinations*, and the runner checks each schedule's **terminal balance retires to ~0** — a
property self-referential tests don't assert. Three combinations failed that property; running
them through the DOS oracle confirmed real divergence. The bug was found in minutes once the
test *asked the right question of the right combinations against the right oracle*.

---

## Process changes (so the next one is caught)

1. **Combinatorial coverage, not just line coverage.** Track the cross-product of advanced
   options as its own coverage target. The exhaustive case suite is the start; extend it so
   every option × option pair has at least one case. Line coverage stays useful but is no longer
   treated as sufficient.
2. **Differential by default for engine math.** A financial result is only "tested" when it is
   compared to the DOS oracle (or a documented hand calculation), not when it merely round-trips
   or satisfies an invariant. New engine behavior ships with an oracle golden.
3. **Assert the property, not the current output.** Prefer invariants the *correct* answer must
   satisfy (the loan retires to $0; total paid = principal + interest) over "equals what the code
   produced today." When a numeric golden is used, it comes from DOS, with the oracle command in
   the test comment.
4. **A TODO in a comment must have a failing/skipped test.** "task #103" should have been a
   `t.Skip("known gap: ...")` regression case, visible in CI output, not a buried comment. Known
   gaps get a test that documents and tracks them.
5. **Bug fixes ship with a fail-before/pass-after regression test** (now a rule in `CLAUDE.md`).
   `amort_arm_balloon_test.go` was verified to fail on the pre-fix engine and pass after.

The one-line lesson: **coverage tells you which lines ran, never whether the answer was right.**
For a faithful port, correctness lives in agreement with the original across the *combinations*
users actually exercise — and only a differential test over that combination space can see it.

---

## Remaining (tracked, with reproductions)

Both follow-ups from the first fix are now also fixed:

- **Skip-months including the first payment** (`skip=1-3,7`): FIXED — the payment solve now lands
  on DOS's $1,105.34 and the loan retires in 359 rows (was ~$1,104 owed over 360). Root cause was
  the same family of bug: `fancyOverUnder` applied the forced-final-payment correction to a
  trailing **zero-payment skip row**. Regression test `amort_skip_firstpay_test.go`.
- **Two balloons spanning an ARM**: FIXED to within ~$2.60 (was $192 → $50 → $2.60) — the
  re-amortization refinement now reconstructs the terminal residual against the **post-adjustment**
  payment, not the base payment.

After these, the exhaustive suite logs **no** residual tail above $1 except the $2.60 three-option
case, which is in DOS-vs-port rounding-tail territory (≈0.003% of principal). All fixes ship with
fail-before/pass-after regression tests.
