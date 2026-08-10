# START_HERE — STATE SNAPSHOT at `cee4943` (round 44, 2026-08-10)

**⚠️ THIS IS A SNAPSHOT, NOT THE LIVE DOCUMENT.** The live `START_HERE.md` is the
claude.ai project doc `claude/START_HERE.md`. Narrowed to a STATE snapshot in
round 41 (deliberately — the full copy went five rounds stale and a stale full
copy is worse than a pointer). **If the project is unavailable, this records what
a round needs to not do damage.**

## Commit state
- **HEAD `cee4943`** (round 44), parent `2b03814` (r43 §83), grandparent
  `d4ef1fd` (r42 §82), then `c64e30e`, `bb5e045`, `0776960`.
- **Working tree CLEAN** apart from untracked `_to_delete/`.
- ⚠️ **VERIFY THIS EVERY ROUND, FIRST.** It has been recorded wrong twice in four
  rounds (R45).

## Bootstrap
- **Tarball set `r44*` in `_to_delete/`, built AT `cee4943`, post-0p,
  foreground scratch-build verified (`go build ./...` exit 0):**
  - `r44src.tar.gz` `c3b397b7f1213d35047a35d8713cf97b`
  - `r44dos.tar.gz` `099986e1791a50ee80fc20438485f048`
  - `r44fix.tar.gz` `666bd710e7f9e1d5fbc41b3ddf4d605b`
  **md5 the tarball you extract against this line — note #56.**
- Recreate the symlink path before anything else:
  `mkdir -p /sessions/funny-tender-pascal/mnt && ln -sfn /root/pw /sessions/funny-tender-pascal/mnt/PerSense-Web`
- **Counts at HEAD:** `git ls-files '*.go'` = **433** (the gate; `find` gives 447);
  `git ls-files` = **1710**; build-cache artefacts tracked = **0** (item 0p, r44);
  42 symlinks; 34 `.pas` in `legacy/src/dos_source`.
- **RUN FIX 5's MANIFEST DIFF EVERY ROUND.** An md5 proves a tarball INTACT, not
  CURRENT. Round 44 pre-bootstrap: 2 differing, 1 `.go` absent (note #58, caught
  live). Post-rebuild: **0 differing, 0 `.go` absent, 811 absent** (the standing
  shape — `legacy/src_documented` and `legacy/src`, which no tarball carries).
- **🚨 NOTE #58: before pushing any file the tarball set predates, STAGE THE
  DRIVE'S COPY and build your change on top of it**, then `grep -c '## §NN'` for
  every section that should be there. Round 43 was one push from deleting §82.
- `pip install actuarialmath ipython scipy --break-system-packages`. Do NOT
  `apt install fpc`.
- **Oracle build flags: `-dV_3 -dSCROLLS -dPVLX`. `-dACTU` IS ABSENT AND
  UNBUILDABLE** — the `ACTUARY` unit source is missing (§82). Name the flags
  beside every published zero (R47).

## Gates at this commit (round 44)
Suite **12 ok / 0 fail** (`PERSENSE_FUZZ` unset — CAUTION 10); `check_skips`
**32/32**; both backward bit harnesses at `PERSENSE_BITS_N=1500` pass; mortgage
differential re-run against the rebuilt oracle (ok, 15.2 s); round-trip md5 8/8.
**NOT RUN:** `paired_regression` and the PV/mortgage arms — no amortization or PV
engine file changed.

## Headline numbers (do not quote without the live doc's cautions)
- **Amortization, in scope, seven questions: 30 in 2,086 = 1 in 70.** Bar is
  1 in 400 — **missed 5.8×.** Four-question column: 23 in 2,086 = 1 in 91.
- **Plain, in scope: 0 in 108,778** — ⚠️ a **round-32** number that has never
  compared an APR and excludes every known hazard class by its predicate.
- **PV forward: 0 in 29,917 worksheets** — ⚠️ carried across an r42 PV engine
  change, and **the life-contingency / Payment-on-Death surface has NO ORACLE,
  PERMANENTLY** (decision 3a.15(B), Nate, 2026-08-09).
- **Mortgage forward: 0 in 30,000 cases** — 360-only, **and that is IMMATERIAL**
  (§87, r44: `yrdays` is inert at n=12 in DOS and in the port alike).
- **Convergence score 2, tenth round running.** Dimension 3 rose 6 → 7 in r44.

## Open, user-visible, and blocking
- **NF-1 and NF-2** — the adjustment-echo blank cell and the snapped-date join.
  **OPEN, fifth round at the top of the plan.** Dimension 5 cannot move until
  they close, and as of r44 **nothing blocks them**: Signal 7 is a validated gate
  on both arms (§84) and is shown to see NF-1 (§86).
- **The residual APR class (20 in 1,856) — UNATTRIBUTED.** Round 44 attributed it
  and **withdrew the attribution in-round** (§85): all seven measured screens are
  also totals-divergent. **Next experiment: exhibit an APR divergence on a
  TOTALS-GREEN screen.**
- **§78 is PERMANENTLY UNADJUDICABLE.** §60 open twenty-two rounds. §72/§73/§74
  open. Seed 50152 hangs, unattributed.

## Section index in `docs/discrepancies.md`
§71 CLOSED · §72 RE-KEYED OPEN · §73 OPEN · §74 OPEN · §75/§76/§77/§79 FIXED r42 ·
**§78 PERMANENTLY UNADJUDICABLE r44** · §80/§81 FILED · §82 the `-dACTU` blocker ·
§83 Signal 6's control · **§84 Signal 7's control + R51** ·
**§85 the withdrawn APR attribution** · **§86 NF-1 vs Signal 7, reconciled** ·
**§87 the mortgage day-count invariance**.

## The three rules a fresh round most needs
- **R32 — run an adversarial audit before publishing any number. ELEVEN FOR
  ELEVEN**, and in round 44 it withdrew the round's own headline.
- **R49 — a control is only a control if its population can express the defect.**
- **🚨 R51 (new, r44) — a mutant that is INERT on a HEALTHY population is a
  statement about the MUTANT, not the instrument. Escalate the mutant before
  doubting the instrument.**
