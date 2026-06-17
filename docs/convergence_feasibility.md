# DOS-Fidelity Convergence: Feasibility Analysis

**Date:** 2026-06-13
**Question:** Many settings/combinations require manual testing; bugs keep surfacing
only when we try them. Is reaching DOS convergence feasible, and how?

## Bottom line

Yes — convergence is feasible, and the hard part is already done. The financial
**engine** is provably converged with the real DOS code: ~6,400 randomized
differential cases run against a headless build of the original Pascal units, plus
93 reference cases, all green in CI with zero divergences. That is the part that is
normally impossible to "hit convergence" on, and it's effectively there.

The bugs you're hitting today are almost all in the **layers the DOS oracle doesn't
reach** — the web frontend (display, echo, clear, round-trip, keyboard, defaults)
and a few engine paths that exist but weren't *wired* or weren't *swept*. They
don't surface until tried because nothing automatically compares them against DOS.

So the feasibility question isn't "can the math converge" (it has). It's "can we
stop manually whack-a-moling the translation layer." That is solvable by pointing
the differential approach you already have at (a) more of the engine's input space
and (b) the frontend — turning the manual matrix into an automated sweep.

## Why bugs aren't surfacing until tried

The amortization input space alone is enormous. Conservatively:

- Computational settings that affect amortization: perYr (~10) × basis (3) ×
  prepaid (2) × in-advance (2) × balloon-includes-regular (2) × exact (2) ×
  rule-of-78 (2) × interest rule (2) ≈ **~1,900 setting combinations**.
- × field-presence dispatch (which of amount / rate / payment / term is blank): ~5.
- × advanced options (prepayment, balloon, adjustment, moratorium, target, skip —
  each absent / present / solve-for-blank): easily ×100+.

That's ~10^5–10^6 meaningful amortization combinations, before Present Value (rate
types, COLA, variable-rate, contingencies, POD, seven backward paths) and Mortgage.
Manual testing covers dozens. No human process converges a space that size — which
is exactly why "it works until you try a new combination."

The project already understood this for the **engine**: instead of hand-listing
cases, it sweeps thousands of randomized inputs against the DOS oracle. The gap is
that the sweep stops at the engine boundary.

## What today's bugs actually were

Nearly all of today's findings were in the untested layers, not the math:

| Bug (today) | Layer | Caught by existing DOS sweeps? |
|---|---|---|
| Schedule Balance column drift; CSV balances | Frontend render | No (UI not swept) |
| Quarterly/Yearly timezone bucketing | Frontend render | No |
| clearPV blanked Pmts/Yr; clearMtg error mark | Frontend clear | No |
| `.psn` import column scramble | Frontend import | No |
| Money not reformatted to `$x,xxx.xx` | Frontend format | No |
| Payoff date snapped 08/15→08/01; balance not green | Frontend binding | No |
| Principal-minimum / moratorium headline payment | Frontend display | No |
| Prepay Stop Date / solved balloon not echoed | Frontend echo | No |
| Auto-calc stale-response clobber | Frontend concurrency | No |
| Fancy-aware payment solve (balloon/target blank pmt) | **Engine, unswept corner** | No — sweeps supplied a payment |
| First-payment default rule (first-of-2nd-month) | **Engine, unswept** | No — sweeps supplied firstDate |
| Prepaid-OFF augmented payment (odd first period) | **Engine, unswept corner** | No — sweeps used prepaid default |
| Negative-am advisory false positive | Engine advisory | No (advisory text not swept) |

Two takeaways:

1. **~80% were frontend/translation** — a layer with essentially no automated
   DOS or engine comparison. That's the primary leak.
2. **The engine bugs sat in unswept corners** of the cross-product: "blank payment
   **and** a balloon," "blank firstDate," "prepaid OFF **and** an odd first
   period." The sweeps are excellent but their *dimensions* didn't cross
   field-presence × settings × advanced options. The math was right wherever it was
   swept; it was wrong only where the sweep didn't look.

## Feasibility by layer

**Engine — already converged, just widen the lens.** A headless build of the actual
DOS Pascal (`legacy/oracle/`) is the gold standard: it can't share a transcription
error with the Go port because it *is* the original code. Extending convergence here
is cheap and bounded — add dimensions to the existing randomized sweeps so they
cross field-presence with settings and advanced options. This converts "engine
corner bugs" from manual discoveries into CI failures. Effort: low; the rig exists.

**Frontend — the real work, but tractable.** There is no DOS/engine comparison of
the UI today (only a handful of isolated JS unit tests). The leverage move: feed the
**same randomized input vectors** through the frontend's own request-builders and
echo-renderers and assert they round-trip against the (DOS-validated) engine. Most
of today's bugs would have been caught by that. Two tiers:

- **Tier 1 (high ROI, low cost):** Node-extraction differential tests — pull the
  shipped JS (`getAmzInput`/`getPVInput`/`getMtgRowData`, the response echoes, the
  render/clear functions) and run random vectors through *frontend → request →
  engine → echo → frontend*, asserting the displayed values equal the engine's.
  This is the same technique already used in `cmd/persense/frontend_render_test.go`,
  just scaled to a sweep. Catches the format/echo/clear/round-trip class — the bulk
  of today's bugs.
- **Tier 2 (interaction flows):** headless-browser automation (Playwright/Chrome)
  for the things only a real DOM exercises — clicks, tab/Enter/F10, focus-driven
  auto-calc, import/export, field-error highlighting. Lower volume, higher cost;
  reserve for flows Tier 1 can't reach.

**Wire what's already built.** A few DOS behaviors are coded in the engine but not
reachable from the UI (in-advance, USA rule, hard-payment penny rounding). Wiring
them is small and immediately makes them sweepable.

**Unported solvers.** `docs/dispatch_gaps.md` lists ~12 specialized DOS procedures
not yet ported (APR-with-points, target-balloon iteration, per-adjustment solving,
etc.). These are genuinely bounded and mostly edge-of-usage; they should be closed
by priority, each with its own oracle sweep so it stays converged.

## A path to convergence

1. **Make convergence measurable.** Maintain a scorecard of the cross-product
   (screen × setting × field-presence × advanced option), each cell marked
   not-built / built-unswept / swept-green. "Convergence" then has a number, and
   "bugs we haven't tried yet" become visible as unswept cells rather than
   surprises.
2. **Widen the engine sweeps** to cross field-presence × settings × advanced
   options (the corners that produced today's engine bugs). Cheapest, highest-value
   step; the oracle already exists.
3. **Build the Tier-1 frontend differential sweep** (random vectors through the
   real JS, asserted against the engine). This closes the layer that produced ~80%
   of today's bugs.
4. **Wire the engine-ready settings** (in-advance, USA rule, hard-payment) so they
   are reachable and swept.
5. **Close the unported solvers by priority**, each landing with its own sweep.
6. **Tier-2 browser automation** for interaction-only flows.

Steps 2–4 are days, not weeks, and remove most of the manual burden. Step 5 is the
long tail (bounded, edge-case). Step 6 is ongoing polish.

## Honest limits

- The oracle validates *numbers and end-states*, not pixels — visual layout, focus
  behavior, and a few interaction flows still need Tier-2 or manual review.
- The headless oracle has a known ~9% process-spawn flake (Pascal heap), already
  handled by retry; high-volume sweeps must keep that retry.
- Convergence is a moving target only where DOS behavior is itself ambiguous (e.g.
  which "balloon" semantics the user wants); those need a product decision, not just
  a test.

## Verdict

Convergence is feasible and, for the engine, substantially achieved. The recurring
bugs are a *coverage* problem in the translation layer, not a *correctness ceiling*
in the math. The same differential-testing investment that already tamed the engine,
extended to cross-product engine sweeps and a frontend differential harness, turns
the manual matrix into an automated one — which is what "hitting convergence"
practically means here.
