# `docs/history/` — WHY THERE IS NO `START_HERE.md` SNAPSHOT HERE ANY MORE

**Decided and executed in round 49 (2026-08-11). Superseded the snapshot rule of
2026-08-01.**

## The live document

**`claude/START_HERE.md`, in the claude.ai project, is the SINGLE LIVE
DOCUMENT.** There is no repo copy, and there is deliberately no repo copy.

## Why the snapshot was retired

`docs/history/START_HERE.md` was a hand-maintained mirror of the live document,
refreshed at commit time. It was retired because every property that was
supposed to make it useful failed in measurement:

1. **It went stale and nobody noticed.** Its last refresh was round 46
   (`50d8bf1`, snapshotting the code commit `789a7d1`). Rounds 47 and 48 made no
   snapshot commit. **No round ever read it.** The first round that did — round
   49 — found it wrong.

2. **🚨 IT SILENTLY DIVERGED INSIDE THE BOOTSTRAP TARBALL SET, AND WAS A LIVE
   REVERT HAZARD.** From round 46 onward the container's copy was never
   refreshed after the drive was committed, so every tarball rebuild
   (r46, r47, r48) re-packed the **round-45** narrow snapshot (6,800 bytes)
   while HEAD carried the **round-46** one (61,669 bytes). Any round that had
   extracted the set and pushed that path back would have **silently reverted
   commit `50d8bf1`** — note #58's hazard, live, for three rounds.

3. **🚨 IT TRAINED THE PROJECT TO ACCEPT REAL DRIFT.** `START_HERE` §0 asserted
   *"FIX 5's manifest diff is expected to show EXACTLY ONE DIFFERING FILE,
   `docs/history/START_HERE.md`"*, explaining it as a snapshot commit that
   postdates the tarball. That explanation was false — it cannot account for a
   whole-generation replacement of the file — and rounds r42 and r45 through r48
   all recorded "1 differing (the expected snapshot)" and moved on. **The
   invariant that was supposed to detect drift was the thing concealing it.**

4. **It is the anti-pattern the project had just deleted.** Round 48's own lead
   finding was three false statements in `START_HERE`, and the same round
   removed a hand-retyped tolerance mirror from `zztacktolerance_test.go`
   because a copy with no link to its source is not a check. A 61 KB hand-copy
   of the project's most-read document, refreshed by hand, is that same shape.

## What replaced it

**`START_HERE` §0's invariant is now: FIX 5's manifest diff MUST SHOW ZERO
DIFFERING FILES.** That is an invariant the tree can actually satisfy, and any
violation of it is unambiguously drift rather than a documented exception a
future round is trained to wave through.

## If the project is unavailable

The round records under `claude/` are the durable account, and each one is
self-contained: `claude/round49_…`, `claude/round48_…`, and so on back through
the history ledger in §8 of the live document. `docs/discrepancies.md` in this
repo carries the full technical account of every numbered finding (§35A,
§71-§90) and is committed, versioned and linked to the code it describes.
That is the repo's system of record — not a mirror of a document that lives
somewhere else.

## What remains in this directory

`round14_roundtrip_inverse_differential_2026-08-01.md` and
`round15_term_axis_and_sec57_ratepresolve_2026-08-01.md` — two early round
write-ups that were committed here before the project became the home for round
records. **They are historical artefacts and are correct as of their own dates.**
They are left in place deliberately: unlike the retired `START_HERE.md` snapshot
they are not mirrors of a living document, so they cannot go stale relative to
anything. Every round from 16 onward lives only in the project, under `claude/`.

## The superseded procedure

The commit-time snapshot procedure decided on 2026-08-01 applied to
`START_HERE.md`. **It is withdrawn.** Nothing in this repo should be a
hand-maintained copy of a document that lives elsewhere; the reasons are above,
and the full account is `docs/discrepancies.md` §90 §9 and the round-49 record.
