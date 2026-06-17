# Path to 99% confidence

Where the UI↔engine confidence stands and what it would take to move it from ~95
to 99. See `docs/ui_engine_confidence.md` for the current per-section scores.

## The honest summary

A system-wide 99 is **not reachable from the current snapshot**, and the reasons
are mostly external rather than effort. Two structural ceilings cap it, and
closing the rest means graduating from "sampled differential testing against
oracles" to a stricter class of validation. 95 is a strong, defensible place to
hold for common and realistic usage.

## The two hard ceilings (not closable by us)

### 1. Missing actuarial source

The contingency / life-table section is capped at ~90 until the client supplies
the original `ACTUARY` unit source and the 1988 HHS mortality tables
(`docs/actuarial_oracle_blocked.md`). We have validated the *mathematics* to
~5e-13 against the SOA Standard Ultimate Life Table and the `actuarialmath`
library, but bit-fidelity to the original DOS implementation cannot be checked
against source that is not present.

### 2. Oracle = recompile, not the shipped binary

Our DOS oracle (`legacy/oracle/`) is a **headless recompile** of the DOS Pascal
units with GUI stubs. It is the real engine, which is why 95 is honest — but at
99 you can no longer assume "our recompile ≡ the executable the client actually
shipped." Compiler version, conditional defines (`V_3`, `ACTU`, `BOFA`), and
floating-point/rounding modes can differ between source and the released `.exe`.

Erasing that assumption requires the **original program's actual outputs**:
- the shipped `Per%Sense.exe` driven headlessly under DOSBox, or — better,
  because it reflects real use —
- a corpus of genuine client worksheets (`.MTG` / `.AMZ` / `.PVL`) paired with
  the results the original program produced for them.

This single artifact would do more for engine confidence than any amount of
additional synthetic testing, because it tests against what the original program
*actually did*, not against our reconstruction of it.

## What 95→99 takes on the parts we control

1. **Exhaustive option coverage** (in progress — see
   `docs/exhaustive_option_sweep_plan.md`). Graduate from randomized sampling to
   exhaustive sweeps over the discrete option lattice of the mortgage and
   amortization screens, every combination compared to the DOS engine across a
   value grid. This pushes those two sections to the recompile-oracle ceiling
   (~97–98).
2. **Mutation testing.** We know the tests pass; we don't yet have a number for
   what fraction of injected defects they would catch. Mutation testing
   (e.g. `go-mutesting`) quantifies and hardens test power — essential evidence
   for a 99 claim.
3. **Property-based testing.** Add invariants with shrinking (monotonicity of PV
   in rate, schedule balance monotonically non-increasing absent draws, solver
   round-trip identity) so failures auto-minimize to the smallest reproducer.
4. **Real-browser E2E.** The frontend is tested by extracting the shipped JS and
   running it under Node with DOM stubs — strong, but not the real browser and it
   stubs neighboring functions. 99 wants Playwright/headless-Chrome tests driving
   the actual page, plus the full event lifecycle, accessibility, and
   cross-browser behavior.
5. **Close the documented edge cross-products.** The scoped-down items in
   `docs/dispatch_gaps.md` §0.11.5 — settings cross-products (R78 × in-advance ×
   USA-rule × ARM), weekly/biweekly day-count edges, date-epoch / Y2K boundaries,
   solver convergence in the corners. Individually small; collectively the
   difference between "validated on common paths" and "proven safe across the
   reachable domain."
6. **Independent audit.** 99-level assurance conventionally implies someone who
   did not write the code re-deriving the validation. Self-certification tops out
   below 99 almost by definition.

## Priority order

1. Request from the client: a corpus of real worksheets (with known outputs) and
   the actuarial source + tables. *(Unlocks both structural ceilings.)*
2. Exhaustive option sweeps for mortgage + amortization. *(In our control now;
   see the companion plan.)*
3. Mutation testing + property-based tests to quantify and harden test power.
4. Real-browser E2E for the frontend.
5. Close the documented settings/edge cross-products.
6. Independent audit.

Items 1 are client asks; 2–5 are weeks of work each with steeply diminishing
returns; 6 is external. Recommendation: pursue 99 only on the modules where the
business actually demands it, and lead with the worksheet-corpus request.
