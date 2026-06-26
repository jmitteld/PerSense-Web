# Post-mortem: Why "high confidence" coexisted with persistent engine bugs

*Per%Sense — Delphi/Pascal → Go amortization & mortgage port. Written 2026-06-25,
after the faithful DOS amortization port became the default engine and ~38,000
oracle-differential cases (amortization + mortgage) ran to zero divergence.*

## The paradox we are explaining

For long stretches of this project we had every reason to believe the Go port was
faithful: we had the original DOS Pascal source to read, a headless build of the
*real* DOS engine to diff against, randomized fuzzers reporting **zero**
divergences across thousands of cases, and near-100% line coverage. And yet, again
and again, a bug that had survived all of that surfaced the moment we widened the
lens — a payment-mode we hadn't generated, a balloon a day off the payment grid, a
total that was right while a row was wrong, an advisory that fired in one engine
and not the other. Some of these persisted, undetected, until the final cutover.

This document is about *why* that happened. The short version: **every artifact we
used to build confidence measured something narrower than "the engine is correct,"
and the gap between the proxy and the truth is exactly where the bugs lived.** None
of the proxies were wrong; each was just answering a smaller question than the one
we thought we were asking.

---

## Cause 1 — A fuzzer validates the cases it *generates*, not the input space

This was the single most recurrent cause. A randomized differential fuzzer is only
as good as its generator, and ours kept encoding unstated assumptions that quietly
carved whole regions out of the input space:

- **`plus_regular = true` everywhere.** Every fuzzer added balloons and
  prepayments in *additive* mode. REPLACE mode (a balloon/prepayment that
  *replaces* the regular payment) was never generated — so the port's REPLACE-mode
  handling went unvalidated and was wrong, only surfacing at the cutover.
- **Balloons placed *on* payment dates.** The generators dropped balloons at month
  boundaries that coincided with payments. An **off-cycle** balloon (a date between
  payments) was never sampled; the port applies it at the next payment instead of
  its own date, and we found this only because a hand-written test existed.
- **Payments that amortize.** The given-payment fuzzers solved the natural payment
  and then perturbed it slightly. A genuinely **interest-only** payment (exactly
  equal to the period's interest) has measure zero under that scheme, so the
  port's very-last fold behavior on a non-amortizing loan was never exercised.
- **Whole-cent balloon amounts.** Because the generators used round numbers, the
  **hard-payment cent-rounding** of a *fractional* balloon (the Dav Holle
  provision) was a no-op in every case the fuzzer drew — so the missing rounding
  was invisible.
- **"Normal" magnitudes.** Term lengths, rates and amounts were drawn from
  realistic ranges, so the **degenerate** corners — `NPeriods > 10000`, every month
  skipped, a 5-day prepayment window — were never visited.

The lesson is uncomfortable but precise: **"0 divergences at N=1000" means "0
divergences among the 1000 inputs this generator can produce,"** not "0 bugs." We
watched this play out as a moving frontier: each time we broadened the generator
(monthly → all frequencies; clean-first → odd-first; solved → given payment;
forward → backward solves), a *new* class of divergence appeared in code that had
been reading "green" for the previous generator. The bugs were always there; the
generator simply hadn't pointed at them yet.

A corollary that bit us specifically: **edge cases are measure-zero under
continuous random sampling.** Interest-only payments, off-cycle dates, exact
threshold values — a uniform random fuzzer will essentially never hit them. These
are precisely the inputs a *human* writes a targeted test for, which is why the
final tranche of cutover failures came not from the fuzzer but from the existing
hand-authored edge tests running through the new engine.

## Cause 2 — The reference was sometimes itself wrong

Early and often, "validation" meant **Go-vs-Go**: the new port compared against the
existing production engine, or against a test whose expected value had been
captured from that engine. This feels rigorous and is fast, but it silently
assumes the production engine is correct — and it was not, on several paths:

- Production's **in-advance** total omitted the upfront settlement interest
  (`amount·(f-1)`) in *every* in-advance loan — wrong by hundreds to thousands of
  dollars, but self-consistent and matching its own tests for years.
- Production's **AO2 target-balloon** solve diverged from DOS (we confirmed the
  port, not production, matched the oracle).
- The **A-W11 advisory** existed *because* production drops a balloon when it
  solves the payment — a behavior DOS does not have. A test asserting A-W11 was
  really asserting a production bug.

When the port (correctly) disagreed with these, a Go-vs-Go check reported a
"regression." Confidence built on a flawed reference is confidence in the flaw.
**Only port-vs-*oracle* (the real DOS engine) could break the tie**, and we only
reached full trust once every section was diffed against the oracle rather than
against ourselves.

There is a sharper version of this for the AO7/AO6+balloon case: **both** Go
engines diverged from DOS *identically*. Comparing them to each other showed
perfect agreement — a strong but entirely false signal of correctness. A shared
blind spot between two implementations written under the same assumptions is
invisible to any test that compares them to one another.

## Cause 3 — Having the source is not the same as understanding the runtime

We had the authoritative DOS Pascal the whole time. It did not save us, for
reasons that are worth being honest about:

- **Static reading produced confident, wrong hypotheses.** For the AO7+balloon
  early-payoff, the schedule dump *looked* like DOS was re-amortizing to a
  degenerate payment, and a careful read of `Re_Amortize` supported a plausible
  story (wrong term, no balloon discount). Instrumenting the actual engine proved
  `Re_Amortize` was **byte-identical** to the normal case — the corruption lived in
  the build-path *print* recursion (`DecideWhetherToPrintALine`/`PrintAndReset`),
  a place no one would think to look from the financial logic alone. The source was
  available; the *relevant* source was not where intuition pointed.
- **The DOS engine carries tangled global mutable state.** Bugs like the
  `next_balloon` counter being clobbered by a *nested* `Iterate` call only exist
  because of Pascal module-level globals and re-entrancy. You cannot see those by
  reading any single procedure; they emerge from the interaction.
- **The source contains dead and divergent code.** A `{$ifdef V_3}` block that is
  never defined; a `ComputeNextForAdvancedInterestPayment` that is commented out; a
  Windows source tree (`win_source`) that differs from the authoritative DOS tree
  (`dos_source`) the oracle actually builds. Reading the wrong copy, or reasoning
  about dead code, is a real and easy mistake — we spent effort on the win_source
  `Re_Amortize` before realizing the oracle builds `dos_source`.
- **Behavior depends on day-count epochs and rounding conventions** (`Round2`
  round-half-down, the `daterec.y = year-1900` epoch) that are invisible until a
  specific input crosses them. A Y2K-style date bug hid behind exactly this and
  only appeared for as-of dates past 2028.

The throughline: **the DOS source told us what *should* happen in the abstract,
but the bugs lived in runtime interactions, build-vs-solve path differences, dead
code, and state, none of which a reading exercise reliably surfaces.** The tool
that actually cracked the hardest case was *instrumenting the real engine* and
reading the values it produced — treating DOS as an oracle to interrogate, not a
document to study.

## Cause 4 — Proxies for correctness (coverage, totals) hide the structure

Two proxies repeatedly gave false comfort:

- **Line coverage.** An earlier post-mortem in this repo already recorded that
  near-100% coverage missed the ARM+balloon bug, because coverage measures *lines
  executed*, not *option-combinations exercised* or *agreement with the oracle*. A
  line can be covered by an input that doesn't stress it.
- **Totals matching.** Comparing total interest is cheap and robust, and it hid at
  least two real bugs: the prepaid×ARM stub used the post-ARM mutated rate, yet the
  *total* coincidentally matched for plain prepaid — only a **row-by-row** diff
  exposed the trajectory error. Conversely, totals can be the *only* thing that's
  wrong (in-advance's missing settlement row left every per-row value correct while
  the total was short). A scalar agreement is a necessary condition for correctness
  and a poor sufficient one; you need both the total and the trajectory.

## Cause 5 — The test harness bounds what is even testable

The differential rig itself shaped the blind spots:

- The oracle's `rows` mode emits only "detail" rows and **filters the settlement
  row**, which is exactly why the in-advance settlement bug looked absent at first.
  It only became visible through the `dumpraw` mode that prints *every* line. If
  your inspection tool hides a row, you cannot diff against it.
- Several inputs could not be expressed until we *extended the harness* — adding
  `payhard=`, `presolve=`, `predur=` tokens so the oracle could even be asked the
  question. Any path the harness couldn't express was, by construction, untested —
  and "we haven't tested it" is indistinguishable from "it passes" if you aren't
  tracking which paths the harness can reach.

## Cause 6 — Numerical correctness is not integration correctness

The port reached genuine zero numerical divergence and we (correctly) trusted that.
But flipping it to the default surfaced ~24 failures that were **not numerical at
all**: the API and several tests read *solved* values back out of the mutated
input record; the advisory layer keys off `AmountStatus == Output` on that same
record; and the production backward-solvers call `Amortize` *internally* with trial
values, so making the port the default silently changed what those inner trials
returned. None of these are about the schedule numbers — they're about the
**contract between the engine and its callers**. A differential fuzzer that only
checks "do the totals/rows match the oracle" cannot see a contract violation,
because the contract isn't expressed in the numbers.

## Cause 7 — Confidence compounds, and "zero" is provisional

Reviewing the work log, several conclusions were stated as "fixed" and later
overturned by *more* testing: a moratorium+target fix was an overclaim that a wider
seed sweep undid; the option cube read "0" before a broadened merged fuzzer found
prepaid×ARM×odd-first. Each "0 divergences" was true and also provisional —
true for that generator, that seed, that N. The danger is that a string of green
runs *feels* like accumulating proof when it is really repeated sampling from the
same distribution. Confidence compounded faster than coverage of the input space
did.

## Cause 8 — Tolerances cut both ways

Differential tests need a tolerance for floating-point noise. Set it too loose and
a real divergence hides inside it; set it too tight and benign accumulation (the
60-year annual in-advance loan, ~9 cents/period over 60 periods) reads as a
failure. We hit both edges. A fixed scalar tolerance is a blunt instrument against
a phenomenon (rounding accumulation) that scales with term length and rate.

---

## What actually caught the bugs (the antidotes)

It is worth naming what *worked*, because the fixes follow from it:

1. **Diffing against the real engine, not ourselves.** Every durable bug fell to
   port-vs-*oracle*, not port-vs-production. The independent reference is the whole
   game.
2. **Instrumenting the oracle when reading failed.** The hardest case (AO7+balloon)
   only yielded when we compiled an instrumented copy of the DOS engine and printed
   its internal `Re_Amortize` values — turning the oracle from a document into an
   interrogable witness.
3. **Row-by-row over totals.** Trajectory diffs found what scalar diffs hid.
4. **Deliberately broadening the generator,** and treating each new green only as
   "green for *this* generator." The frontier moved because we kept moving it on
   purpose (frequency, first-period, prepaid, given-payment, backward solves,
   REPLACE mode, off-cycle, degenerate).
5. **Keeping the hand-written edge tests.** The fuzzer never sampled interest-only
   or off-cycle balloons; the human-authored tests did, and running them through
   the new engine at cutover is what surfaced the last tranche.

## Lessons to institutionalize

- **Track the input space, not just the pass rate.** A green fuzzer should be
  accompanied by an explicit, written statement of *what it does not generate*
  (modes, date alignments, degenerate corners). "0 divergences" is only meaningful
  next to "over this domain."
- **The oracle is the reference; the other Go engine is not.** Any check that is
  Go-vs-Go is a consistency check, not a correctness check. Reserve "correct" for
  agreement with the real DOS engine.
- **Two implementations agreeing proves nothing about a shared assumption.** When
  the port and the piecewise engine agree but both could be wrong, only the oracle
  arbitrates.
- **Prefer interrogating the engine over reading its source** for runtime behavior;
  use the source to form hypotheses and the instrumented oracle to confirm them.
- **Diff trajectories, not just totals**, at least on a sampled subset.
- **Test the engine↔caller contract separately** from the numerics: input
  mutation, advisory state, and re-entrant routing are their own surface.
- **Make tolerances scale** with the thing that accumulates (term length), or pin
  exact values where the basis is exact (360-day whole-month) and only widen for
  the genuinely accumulating paths.
- **Treat "fixed" as a hypothesis until a *wider* test confirms it.** The cheapest
  insurance against an overclaim is to broaden the fuzzer before declaring victory,
  not after.

The deepest point is the simplest: **we were never measuring "is the engine
correct." We were measuring "does this proxy agree on the cases we drew."** Every
bug that persisted lived in the space between those two sentences — a mode the
generator didn't draw, a row a total didn't see, a runtime interaction the source
didn't reveal, a contract the numbers didn't encode. Closing that space is not a
matter of running *more* of the same test; it is a matter of making the proxy
larger than the truth you are trying to certify, on every axis the truth can vary.

---

## Applying these lessons forward: Present Value & Actuarial

The amortization and mortgage engines are now diffed exhaustively against the real
DOS oracle. The remaining two engines sit in very different positions, and the
lessons above apply unevenly between them.

### Present Value — already near the gold standard; tighten it

PV has a *native* DOS oracle (`legacy/oracle/pv_oracle.pas`), and all seven
backward solves (PV-1, PV-2, PV-4, PV-5, PV-6, PV-8, PV-9) are diffed directly
against it, plus two fuzzers (`property_fuzz_test.go`, `zzfuzz_oracle_test.go`).
So for PV the work is *tightening* an existing differential rig, not building one.
Concrete checklist, each item tied to a cause above:

- **Cause 1 — audit what the generators hold fixed.** `genScenario` pins the as-of
  date and basis, and places a contingency on only ~50% of rows. The `cola = rate`
  exact branch in PV-5 is measure-zero and will never be sampled randomly (only
  the boundary tests reach it). Over-determined rows and *mixed-contingency
  multi-row* worksheets look under-generated. Broaden the generators and treat each
  prior "0 divergences" as 0-for-that-generator, not a clean bill.
- **Cause 4 — diff per-row PV contributions, not just `SumValue`.** A COLA or
  contingency error can preserve the total while shifting the per-row trajectory
  (the R78 trap). Confirm the sweeps compare row-by-row, not the final sum only.
  *(Done 2026-06-25: the oracle now emits per-row values and the multi-row +
  VR-multi sweeps diff each row — 2,406 rows, 0 divergences, max relErr ~1e-8.
  No masked per-row error existed.)*
- **Cause 5 — map the oracle's subcommand tokens against the real API/dispatch
  surface.** Anything the `pv_oracle.pas` tokens can't express (mixed-contingency
  multi-row; the documented dead `V_3` block) is untested by construction.
- **Cause 6 — add integration tests at the PV↔actuarial seam and FirstPass
  dispatch.** This already bit once: the leftover Payment-on-Death that inflated a
  PV total was a state/contract bug a numeric sweep cannot see.
- **Cause 8 — scale tolerances with horizon.** Long-dated (50-year) streams
  accumulate float error the same way the 60-year in-advance loan did; a fixed
  scalar tolerance is the wrong instrument.

### Actuarial — a structural Cause-2 ceiling, to be stated honestly

Per `docs/actuarial_oracle_blocked.md`, the DOS `ACTUARY` unit source *and* the
1988-HHS MALE/FEMALE table data are missing, so a DOS oracle **cannot be built**.
The engine is validated against first-principles life-contingency math and the
`actuarialmath` library instead. This is Cause 2 in its sharpest form: those
references are textbook-*correct*, but the port's mandate is DOS-*fidelity*, and
the DOS computation itself is gone. "Agrees to 1e-8 vs actuarialmath" therefore
certifies the mathematics, **not** bit-fidelity to DOS's specific table
interpolation and `LifeProb`/`PODValue` rounding. The key discipline is to not let
that tight agreement *feel* like DOS-parity — it structurally cannot be one.
Within that ceiling, two lessons still apply against the reference we do have:

- **Coverage ≠ correctness (the ARM+balloon lesson).** The two-life contingency
  routing — `BothLiving` / `EitherLiving` / `Only1Living` / `Only2Living` — is a
  combinatorial surface currently pinned by essentially one worked example. It is
  the actuarial analog of the option-combination cube that near-100% line coverage
  missed. It wants a contingency-type × age-pair × fractional-age sweep against
  the first-principles reference, not a grid of round ages.
- **Cause 4 — decompose the POD scalar.** `PODValue` returns a single number;
  diff its per-period survival-weighted cashflows, not just the cents-level total.

Finally, an inversion of Cause 3: its punchline was that *interrogating the running
engine* beat reading the source. Actuarial has no source — but if the client can
supply even the **compiled DOS binary plus the table data** (not the source),
black-box differential testing becomes possible, exactly the technique that cracked
the hardest amortization bug. That is a more reachable ask than recovering source,
and it is the single thing that would lift actuarial out of the Cause-2 ceiling.
