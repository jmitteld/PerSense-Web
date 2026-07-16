# Amortization payment-solve non-convergence fix + audit-convergence testing policy — 2026-07-16

## Part 1 — The fix (fuzzer3 finding)

### Symptom
A blank-payment solve on an ill-conditioned high-rate/long-term loan returned a
degenerate schedule where the DOS oracle cleanly refuses. Canonical case
(`amort_oracle 3893589.94 0.2757590000 153 4 … b365 solvepayment`):

- **DOS:** `ERR Computation of payment amount or interest rate did not converge.` (no schedule)
- **Go (before):** payment `268,338.49` (≈ interest-only) → schedule with **negative
  amortization** and a **$37,188,140 terminating balloon** on a $3.9M loan.

### Root cause
DOS solves a blank payment with `Iterate` over `RepayLoan` (AMORTOP.pas:1437) and
takes the closed-form shortcut **only** for a `(not exact) and prepaid and (not
in_advance)` loan (Amortize.pas:402-408). So an ordinary in-arrears loan **still
iterates**. On this loan the schedule's terminal balance is astronomically steep
in the payment (a knife-edge root, slope ≈ −$107,000 of terminal per $1 of
payment), so `Iterate` exhausts its 20-step budget above tolerance and DOS blocks
(AMORTOP.pas:1489).

The port's `needPaymentRefine` returned **false** for the arrears natural-first
case, so Go **skipped `Iterate` entirely**, used the closed-form annuity payment,
and papered over the non-amortization with a terminating balloon.

Not a precision problem: the oracle is built `-Mdelphi`, where FPC `real` = 64-bit
`double` — the same precision as Go's float64. An initial hypothesis (emulate
Turbo Pascal 6-byte `real`) was prototyped and **discarded** once this was
confirmed.

### Fix (engine.go, `Amortize`, blank-payment non-fancy path)
Run Go's **own** double-precision `dosIterateSimplePayment` (the port of DOS's
`Iterate` over `RepayLoan`) for the blank-payment solve whenever DOS iterates
(`exact OR not-prepaid OR in-advance`, bounded to the non-exact-daily RepayLoan
terminal), and **honor its convergence verdict** — on `ok == false`, return
`Err = "Computation of payment amount or interest rate did not converge."` with
no schedule, exactly like DOS.

Two calibration subtleties that made it exact:
1. **Seed from DOS's raw annuity estimate** `estimatePayment(&loan, f)`, NOT Go's
   odd-first-augmented `d`. The augmentation nudges `d` onto the knife-edge root,
   which would trip `Iterate`'s "seed already converged" fast path and wrongly
   accept a loan DOS refuses. Seeding from the raw annuity reproduces DOS's secant
   *path*, hence its verdict.
2. Adopt the refined value only where the closed form is inexact
   (`needPaymentRefine`), preserving all existing payment values; the
   natural-first arrears case keeps its closed-form payment and only gains the
   convergence check.

### Validation (oracle-differential)
- Boundary matches the oracle to the decision **and** to the cent on the solving side:
  `500000 0.28 120 4 → 34997.9108` (solves), `500000 0.28 200 4 → refuse`,
  `100000 0.25 40 4 → 6854.4709` (solves), case#1 → refuse.
- **fuzzer3, 3000 cases:** Go-solves-DOS-refuses = **0** (target class fixed),
  DOS-solves-Go-refuses = **0** (no over-blocking — perfectly calibrated).
- Full amortization package suite (all oracle-gated cubes + fuzzers) green.
- New regression test `TestAmortPaymentSolveNonConvergeMatchesDOS` — confirmed
  fail-before / pass-after, with oracle provenance in comments.
- Also in fuzzer3: DOS date-horizon overflow ("Bad date passed to Julian
  function") on an enormous *solved* term is now classified **indeterminate**, not
  a refusal Go must mirror — it's a DOS date-range artifact, not a computational
  refusal.

## Part 2 — Does iterative auditing converge? (testing policy)

**Client's worry:** each audit pass that finds bugs might introduce new ones found
on later passes, never converging.

**Answer:** the process converges *if and only if* three conditions hold — and
this project already has all three. The worry also conflates two different things.

### The conflation to resolve first
- **Finding pre-existing bugs on later passes** (because a new fuzzer probes a new
  region of the input space) is *coverage increasing*, not the code getting worse.
  Cumulative bugs-found rises while bug *density* falls. This converges as the
  input space is covered.
- **Introducing regressions with a fix** is the dangerous kind. It is prevented by
  the ratchet + blast-radius below. The metric that separates them: is a new
  finding in an **already-audited** region (regression — alarm) or a **newly
  probed** one (fine)?

### The three conditions that guarantee convergence
1. **A fixed external oracle as the sole ground truth.** Every fix is adjudicated
   against the DOS oracle, never against the port's own prior output or another Go
   component. Without a non-moving reference, "correct" is a moving target and the
   process *cannot* converge — you get A-matches-B-matches-A tail-chasing.
   (Enforced by CLAUDE.md: "Internal-consistency tests must never drive a behavior
   change"; goldens must cite oracle provenance.)
2. **A monotonically growing regression suite (a ratchet).** Every fix ships with
   an oracle-sourced test, fail-before/pass-after confirmed, that stays green
   forever in CI. This makes progress *monotone*: the set of known-correct
   behaviors only grows; a later fix cannot silently reintroduce an earlier bug
   because that test is still running. This is what turns whack-a-mole into a
   ratchet.
3. **Blast-radius validation on every engine change.** Financial primitives are
   shared, so a fix can perturb far-away cases. Running the **full** oracle
   differential (`make ci`), not just the motivating test, catches a regression
   *within the same pass* rather than two passes later.

With (fixed ground truth) + (monotone ratchet) + (blast-radius), each pass
strictly increases oracle-verified coverage and never loses ground: a contraction,
not an infinite regress. Findings-per-pass trend to zero.

### Recommended policy (mostly already in place)
- DOS oracle is the **only** source of truth; no internal-consistency test ever
  drives a change.
- Every fix: oracle-sourced regression test (fail-before/pass-after) + full
  `make ci` blast-radius run.
- Keep **independent** differential fuzzers with different sampling (fuzzer2 =
  well-conditioned interior; fuzzer3 = adversarial edges). Convergence for a bug
  class = all of them going quiet.
- **Add a regression counter to CI:** any test flipping green→red is a hard stop,
  root-caused before proceeding. This is the single most important early-warning
  signal for the client's exact worry.
- Prefer **root-cause** fixes over per-case special-casing (special-cases grow the
  edge-interaction surface and slow convergence). The payment fix is a good
  example: it aligned Go's `Iterate` gate with DOS's and reused Go's own solver
  rather than special-casing the one input — and reproduced DOS to the cent.
- Classify DOS **refusals** carefully: genuine non-convergence must be mirrored;
  DOS implementation artifacts (date-horizon overflow) must **not** be — mirroring
  an artifact would *add* a bug. (Both handled correctly this pass.)

### How to measure convergence (so the client can see it)
- Findings per 1000 fuzz cases per pass → should trend down.
- Regressions (green→red flips) → should stay ~0. **Nonzero here is the real alarm**,
  not "we found another pre-existing edge case."
- Harness-bug vs engine-bug ratio → this project's meta-lesson is that most
  "findings" have been *test-harness* artifacts (precision, tokens), meaning the
  engine is more DOS-faithful than the raw finding count suggests.
- Declare a module converged when N independent fuzzers × M cases produce zero
  engine findings and zero regressions across K consecutive passes.

### Honest caveat
Convergence is to "matches the DOS oracle," not "bug-free in the absolute." If DOS
itself has a bug, the port reproduces it by design (DOS is the spec). And residual
risk always remains in regions no fuzzer samples. But with the ratchet + fixed
oracle, the process is sound and terminating.
