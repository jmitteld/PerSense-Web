# docs/history — commit-time snapshots of the continuity docs

**These files are SNAPSHOTS, not the live documents. Do not edit them here.**

The Per%Sense port keeps its continuity and history docs in the claude.ai
project ("Persense"), not in this repo:

| document | live home |
|---|---|
| `START_HERE.md` — live state, next action, standing rules, traps | project: `claude/START_HERE.md` |
| `workflow_sync_to_ssk.md` — sync workflow + container bootstrap | project: `claude/workflow_sync_to_ssk.md` |
| `round*.md` — one write-up per round | project: `claude/round*.md` |

That is where every working session reads from and writes to, and where the
authoritative version always is.

## Why snapshots exist here

Decided by Nate, 2026-08-01. The project copy is not on the SSK drive and not in
git, which means the state of the port could not be read from disk, did not
appear in `git log`, and had no versioned history alongside the code it
describes. Mirroring *continuously* would create two live copies that drift and
leave "which one is right?" ambiguous mid-session.

The compromise: **the project copy stays the single live document; a snapshot
lands here at COMMIT TIME only.**

## The procedure (for the agent)

When a round's work is synced and Nate is ready to commit:

1. Overwrite **`docs/history/START_HERE.md`** with the current project copy.
   Overwriting rather than dating it is deliberate — `git log -p
   docs/history/START_HERE.md` then reads as the evolution of the port's state,
   one diff per round, which is the whole point.
2. Add the round's write-up as **`docs/history/roundNN_<slug>_<date>.md>`**.
   These are append-only; a round doc is history the moment it is written.
3. Sync both to the drive with the usual md5 verification.
4. Note the snapshot in the round doc's file table so the two stay in step.

**Do not snapshot mid-round.** A snapshot taken before the round's gates are
green records a state that never existed, and the next session cannot tell the
difference. If a round is abandoned or its conclusion retracted (see round 13's
retraction), the snapshot should reflect the corrected state, not the first one.

## What is NOT mirrored here

`workflow_sync_to_ssk.md` is agent-operational — device-bridge cautions,
staging quirks, bootstrap recipes. It has no bearing on the code and is not
snapshotted. If that changes, add it above.

---

## ⚠️ SNAPSHOT DEBT — noted by round 30 (2026-08-04)

`START_HERE.md` in this directory was last refreshed **2026-08-02** and is
therefore **eleven rounds stale** (rounds 19-30 are all missing). The snapshot
rule's stated purpose — "`git log -p docs/history/START_HERE.md` reads as one
diff per round" — is already broken, and a single catch-up commit would not
restore it.

**The live document is the claude.ai project doc `claude/START_HERE.md` and it is
current.** Nothing has been lost; only this convenience copy has drifted.

Round 30 chose to record the debt rather than paper over it with one large jump.
Whoever refreshes it next should say in the commit message which rounds the diff
collapses.
