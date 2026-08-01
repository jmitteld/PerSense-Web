# Per%Sense — Testing & Verification Policy

**Status:** Authoritative. Applies to all future tests and verification work on
the Go port.
**Owner:** porting team
**Companion docs:** `postmortem_365_exact_interest.md` (the incident that
motivated this), `dos_known_frontier.md`, `path_to_99.md`, `test_plan.md`,
`fidelity_validation_roadmap.md`.

This document defines how we test and how we *count confidence*, so that a
documented bug can never again sit green inside the suite. It is written against
the failure modes that produced the 365-basis "exact" miss, and generalizes them.

---

## 0. The one-line version

> A passing test proves only what it **compares**, on the axes it actually
> **crosses**, for the entry path it actually **drives**. Confidence is the
> coverage of *failing output quantities across crossed axis-combinations* — not
> the count of inputs touched.

Everything below makes that operational.

---

## 1. Compare the observable the user sees ("output-quantity coverage")

The unit of confidence is **(output quantity × axis-combination × entry path)**,
not "a cell was exercised."

1.1 For every financial path, the regression must compare **every materially
distinct output**, not a convenient summary:

- Amortization: the **per-row interest split AND the running balance AND the
  terminal balance** (does the loan retire to zero?) — not only the regular
  payment. The 365-exact bug was invisible to a payment-only check.
- Mortgage / PV: the solved field **and** any derived schedule/total the UI
  renders.

1.2 If a quantity is shown in the UI or exported (CSV, schedule rows, totals),
it is in scope for comparison. "We compared the payment" is not "we verified the
schedule."

1.3 A `// known small effect` claim about an output must be backed by a test that
actually measures that output on the axis where the effect is largest — not the
axis where it is a no-op (see §2).

## 2. Cross axes that interact — never test an interacting factor in isolation

2.1 When a setting is documented to matter **only in combination**, the test
must exercise the **combination**. Canonical example: DOS's Exact setting is a
no-op on the 360 basis ("365 DAY MUST ALSO BE SELECTED"). Testing `exact` on the
360 basis validates nothing. Required combinations include, at minimum:

- `exact ∈ {off,on}` **×** `basis ∈ {360, 365, 365/360}`
- `in-advance`, `R78`, `USA-rule`, `prepaid`, `daily compounding` each **×**
  `basis`, and pairwise with each other where DOS branches on the pair.
- odd-first-period **×** {prepaid, balloon, 365, exact}.

2.2 Maintain an explicit **interaction matrix** (in `test_plan.md`) listing which
axis pairs/triples DOS branches on. Every branch condition in the Pascal
(`if (… basis …) and (… exact …)` etc.) is an interaction that must appear as a
crossed pair in the suite. Grep the legacy source for compound boolean guards
and back-fill any missing crossing.

2.3 A coverage map that asserts "cell exercised" is only valid if the cell's
assertion compares an output that **can change** in that cell. A cell whose
comparison is structurally insensitive to the axis under test counts as **not
covered** — annotate it as such, don't let it inflate the number.

## 3. Drive the real entry path; verify "unreachable" against the whole surface

3.1 Differential tests must run through the **same dispatch the API/UI use**
(the field-presence path, `Amortize` / `Calculate` / `mortgage.Calc`), not a
private helper that bypasses validation, defaulting, or proration.

3.2 Any claim that a bad path is "unreachable from the product" must be checked
against **every** input that reaches it, not the one entrance under discussion.
The 365-exact path was declared unreachable because the *Exact toggle* was
hidden — but the *365 basis dropdown* still led straight into the un-exact
approximation. Enumerate entry paths before declaring a path dead:

- every UI control and its default,
- every API field and its omitted-field behavior,
- every settings combination a `.psn` import can carry.

3.3 Frontend/UI behavior that affects numbers (auto-calculate, stale-response
guards, clear/import) gets its own differential coverage — see
`frontend_differential_harness.md`.

## 4. The DOS oracle is the authority — and it must be exercised, not assumed

4.1 The real DOS engine (`legacy/oracle/`) is the source of truth for financial
logic (per `CLAUDE.md`). Round-trip / internal-consistency tests are necessary
but **not sufficient** — they can hide systematic forward/backward bias. Every
new financial function ships with **at least one DOS-oracle regression case**.

4.2 When you add an oracle flag (e.g. `exact`, `b365`, `inadv`), add a test that
**combines it with the axes where it bites** (§2.1), and assert on the
**schedule**, not just the scalar the harness prints first.

4.3 Tolerances are explicit and justified in a comment (e.g. "365 basis accrues a
few cents of per-period rounding; tol 1e-3"). A tolerance wide enough to hide a
real divergence is a bug in the test.

4.4 Oracle flakiness (the heap-sensitive 0-payment retry) is handled by retry +
`ok=false`, never by skipping the assertion. A skipped case is reported in the
test log and counted as **not covered**.

## 5. A divergence the suite finds is escalated or fixed — never silently suppressed

5.1 When a test discovers a DOS divergence, the **default is to fix it**. If it
cannot be fixed now, it must be:

- written up in a `docs/*_finding.md` (or `dos_known_frontier.md`) with root
  cause, measured magnitude on the worst axis, and a product decision, **and**
- tracked as an open item in `path_to_99_remaining.md` / `dispatch_gaps.md`, **and**
- guarded by a **tight** regression that asserts the magnitude does not grow —
  not a coarse envelope that also passes for a much larger break.

5.2 **No `continue`-past-a-failing-cell** to keep a suite green without all three
of the above. A quarantine without an open-item link is a hidden incident.
Quarantines are reviewed every milestone with the intent to close them.

5.3 "Documented" is not "resolved." A bug written down in a `docs/` file is still
an open bug until the engine matches DOS or the client signs off on the
difference (`CLIENT_DECISIONS_MEMO.md`).

## 6. Boundary, adversarial, and combinatorial obligations

6.1 Keep the existing boundary discipline (`backward_boundary_test.go` style):
rate at the `Teeny=1e-10` cutoff, `cola = rate` exactly, Newton
non-convergence, empty/single-row inputs, very long terms, very high rates.

6.2 Exhaustive option-cube sweeps (`exhaustive_option_sweep_plan.md`) are
deterministic and assert a coverage map — but per §2.3 the coverage map is only
credited when the cell's assertion is sensitive to the cell's axes. Cube cells
that compare an insensitive output are coverage **debt**, listed explicitly.

6.3 Prefer deterministic enumeration over random sampling for the
settings/option space; use random sampling to widen the value grid (amounts,
rates, terms, dates) on top of it.

## 7. Definition of Done for any financial change

A financial change (new function, ported branch, bug fix) is **not done** until:

- [ ] It is driven through the **real dispatch** path (§3.1).
- [ ] A **DOS-oracle** regression compares **every UI-visible output** for it
      (§1, §4) — for amortization that includes per-row interest, running
      balance, and terminal-balance retirement.
- [ ] Every **interacting axis** is crossed where DOS branches on the pair
      (§2), with at least one case on the axis where the effect is largest.
- [ ] Tolerances are explicit and justified (§4.3).
- [ ] No divergence is suppressed; any deferred gap has a finding doc + open-item
      link + tight magnitude guard (§5).
- [ ] `go test ./...` is green and the new test **fails** if the change is
      reverted (verify the test bites — a test that passes against the old code
      proves nothing).

## 7b. Every divergence gets a SOURCE AUDIT before a fix is attempted

**Added 2026-07-31.** Reducing a case and then reaching for another sweep is the
default failure mode of this project. More sampling of the same class tells you
nothing new: fuzzing establishes that a divergence EXISTS and hands you inputs;
it cannot tell you WHY.

Once a divergence is reduced to a minimal repro, the **required** next step is a
line-by-line audit of the DOS path against its Go counterpart. This is a
deliverable, not a habit — it produces a written side-by-side.

**Scope of the audit — all four, in this order:**

1. **The routine that computes the divergent quantity**, on both sides, quoted
   verbatim with `file:line`.
2. **Everything it calls.** The primitives are usually faithful; check anyway,
   and say so explicitly when you rule one out.
3. **Everything that RUNS BEFORE it and can mutate the state it reads.** This is
   the step that gets skipped and the one that most often holds the answer. A
   perfectly faithful routine still diverges if DOS reached it with different
   global state.
4. **VAR parameters and mutated globals on the path.** DOS passes records by
   reference and mutates them in place — `NumberOfInstallments(var f, l, ...)`
   snaps `l` onto the payment grid and the caller keeps the snapped value;
   `h^.lastdate`, `h^.loanrate`, `h^.payamt` are globals a walk can leave
   changed. The port has one copy per call frame where DOS has one global, so a
   DOS side effect that survives across calls has **no port equivalent unless
   somebody deliberately ported it**. Grep the Pascal on the path for `var `
   parameters before concluding the port is faithful.

**Why this is a policy and not advice.** The 2026-07-31 adjustment re-solve
divergence (DOS payment 953.16 vs Go 963.85 on a two-adjustment screen) is
invisible from any input/output pair. Both engines ran the same re-solve formula
on the same balance at the same rate; they differed because DOS's pre-pass
(`Amortize.pas:1408-1419`) had already run `Re_Amortize` for the *later*
adjustment, and that call left `h^.lastdate` month-end-snapped through an
unguarded `var l` (`AMORTOP.pas:1547` → `INTSUTIL.pas:1018`). The port had no
pre-pass, so it counted one fewer remaining period. No quantity of additional
fuzzing would have surfaced that; reading the callers did, in one pass.

**Audit findings are recorded even when they do not lead to a fix.** A ruled-out
hypothesis with its citation is worth as much as a confirmed one — it stops the
next session re-deriving it. State plainly what was examined, what was ruled
out, and what remains unresolved.

**Corollary — a too-regular signal indicts the harness, not the engine.** If the
divergence is a constant integer offset independent of the inputs that should
move it, audit the differential tool first. Real accrual divergences vary with
the inputs. This has now cost the project twice: the oracle's own
`loandmy=` ordering bug (2026-07-25, "85 of 95 compared cases divergent … all of
them the same harness artifact") and the identical bug in `cmd/goamort`
(2026-07-31), which produced a confident but entirely false minimal repro.
Validate a differential tool **on the axis under investigation**, not only on
default screens — the original `goamort` validation used seven control screens,
all on the default loan date, which is precisely the case its bug could not
affect.

---

## 8. How we report confidence

8.1 A confidence figure (e.g. "path to 99") must state **what it counts**:
the fraction of *(output quantity × crossed axis-combination × entry path)*
that matches the DOS oracle within stated tolerance — **excluding** cells whose
comparison is insensitive to their axis, and **excluding** suppressed cells.

8.2 Each milestone update to `path_to_99.md` lists: cells credited, cells that
are coverage debt (insensitive comparison), open findings, and active
quarantines. The headline number never silently absorbs any of the latter three.

8.3 Before raising a confidence number, run the **"would this catch the last
bug?"** check: pick the most recent shipped/found defect and confirm the current
suite would now fail on it. The 365-exact miss is the standing regression for
this check until its row-level test lands.

---

## Appendix A — Failure-mode checklist (from the 365-exact incident)

Use this as a pre-merge smell test:

1. **Vacuous axis** — am I toggling a factor on an axis where it's a no-op?
   (exact-on-360). → Cross it with the axis where it bites.
2. **Insensitive comparison** — does my assertion compare a quantity that can't
   change under the axis I claim to test? (payment vs an interest-split bug). →
   Compare the affected output.
3. **Bypassed dispatch** — am I calling a helper instead of the real API path? →
   Drive the dispatch.
4. **Suppressed divergence** — is there a `continue` / wide envelope hiding a
   known break? → Escalate, document, tighten.
5. **Unverified "unreachable"** — did I check *every* entry path, not just the
   one I closed? → Enumerate UI + API + import.
6. **Non-biting test** — does my test still pass if I revert the fix? → Make it
   bite.
7. **Fix attempted without a source audit** — did I read the DOS path AND its
   callers, or did I infer the mechanism from inputs and outputs? → §7b.
8. **Unaudited state mutation** — is there a `var` parameter or a DOS global on
   this path whose side effect the port does not reproduce? → §7b step 4.
9. **Suspiciously regular divergence** — is the delta a constant offset that
   does not move with the inputs? → Suspect the harness, not the engine; validate
   the differential tool on the axis under investigation.
