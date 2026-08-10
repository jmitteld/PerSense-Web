# START_HERE — STATE SNAPSHOT at the round-45 code commit `bfc90f2` (2026-08-10)

**⚠️ THIS IS A SNAPSHOT, NOT THE LIVE DOCUMENT.** The live `START_HERE.md` is the
claude.ai project doc `claude/START_HERE.md`. Narrowed to a STATE snapshot in
round 41 (deliberately — the full copy went five rounds stale and a stale full
copy is worse than a pointer). **If the project is unavailable, this records what
a round needs to not do damage.**

## 🚨 Commit state — THE RULE CHANGED IN ROUND 45. READ THIS FIRST.

**R45 fired on the live document's HEAD hash in rounds 42, 43 AND 44. It will not
fire again, because neither document now asserts a HEAD hash as current state.**

The cause was structural: **this snapshot commit necessarily POSTDATES the file
that describes it.** Any hash written here is stale the moment it lands.

- **LAST CODE COMMIT: `bfc90f2`** — *"round 45: NF-1 root-caused and FIXED — DOS's
  unconditional store was ported inside its own gate (§88)"*. Parent `efe69f3`
  (r44 snapshot), then `cee4943` (r44), `2b03814` (r43 §83), `d4ef1fd` (r42 §82).
- **HEAD: MEASURE IT.** `git rev-parse --short HEAD`. It is `bfc90f2` **or one
  doc-only snapshot commit on top of it. BOTH ARE CORRECT.**
- **Working tree CLEAN** apart from untracked `_to_delete/`.
- **⚠️ MEASURE EVERY COUNT BELOW. DO NOT QUOTE THEM.** Round 44's `.go` gate was
  recorded as 430 and was 433.

## Bootstrap

- **Tarball set `r45*` in `_to_delete/`, built at the LAST CODE COMMIT,
  foreground scratch-build verified (`go build ./...` exit 0, `go vet` 0):**
  - `r45src.tar.gz` `574ddc7f8d6d33afbcbab70fb433ecda`
  - `r45dos.tar.gz` `099986e1791a50ee80fc20438485f048`
  - `r45fix.tar.gz` `666bd710e7f9e1d5fbc41b3ddf4d605b`

  **md5 the tarball you extract against this line — note #56.**
- Recreate the symlink path before anything else:
  `mkdir -p /sessions/funny-tender-pascal/mnt && ln -sfn /root/pw /sessions/funny-tender-pascal/mnt/PerSense-Web`
- **Counts at the code commit:** `git ls-files '*.go'` = **436** (the gate; `find`
  gives more — untracked scratch); `git ls-files` = **1713**; build-cache
  artefacts tracked = **0**; 42 symlinks; 34 `.pas` in `legacy/src/dos_source`.
- **RUN FIX 5's MANIFEST DIFF EVERY ROUND.** An md5 proves a tarball INTACT, not
  CURRENT.
  **🚨 EXPECT EXACTLY ONE DIFFERING FILE — THIS FILE — AND NOTHING ELSE.** The set
  is built at the CODE commit; the snapshot commit lands after it. That is the
  documented invariant, not drift. **ANY OTHER differing file, above all a `.go`,
  IS REAL DRIFT: stage the drive's copy before touching it (note #58).**
  Round 45 ran it and got exactly that.
- **⚠️ EXCLUDE `_to_delete/` FROM THE FIX-4 MEMBER LIST** or the new set carries
  the previous set inside itself (new r45).

## What round 45 changed

- **NF-1 FIXED** — `internal/finance/amortization/engine.go`. `AMORTOP.pas:1591-1592`
  stores the re-amortized adjustment payment on EVERY crossing; only the `amtok`
  LATCH sits behind the gate at `:1571`. The port had the WHOLE store inside the
  gate, so on any piecewise screen with no balloon, no prepayment and not
  exact-non-360 the amount DOS paints was computed, used to build the schedule,
  and discarded. **Open since round 39D. Guard:
  `zzr45_nf1_piecewise_echo_test.go`.**
- **NF-1c FIXED** — `internal/api/handlers.go`. The solved adjustment rate echoed
  in INTERNAL space; `amzUnkickerRate` now applies, matching `INTSUTIL.pas:1649-1651`.
  Was 1.3889% relative high on the 365/360 basis.
- **NF-2 FIXED, AND ITS DESCRIPTION RETRACTED** — the port ALWAYS snapped and
  echoed the snapped date. The break was one strict `!==` in
  `cmd/persense/static/index.html`. The wire now carries `requestedDate` when a
  snap moved the row. Guard: `cmd/persense/frontend_r45_adjustment_paint_test.go`
  (runs the shipped block in node).
- **A1-A5** — `internal/api/zzr45_adjustment_echo_wire_test.go`, the first
  API-layer tests the R39 wire fields have ever had.
- **`docs/discrepancies.md` §88** — all of the above plus item 5's answer.

## The numbers that matter

- **In-scope SEVEN-question HARD: 25 in 2,086 = 1 in 83** (was 30 = 1 in 70).
  **The FOUR-question CONTROL HELD at 23 = 1 in 91.**
  **🚨 This is a DISPLAY-TRANSPORT correction, NOT engine convergence.**
  Bar (1 in 400) missed **4.79×**.
- `paired_regression` seeds 50100-50109 N=400: **FIXED 5, STILL 62, NEW 0.**
- **Item 5: of the 20 APR divergences, 2 are totals-green by the instrument's
  signals and only 1 is totals-IDENTICAL by ground truth.** That isolate carries
  **NO ADJUSTMENTS** and is **OUT OF SCOPE** (horizon 2133). **The class does NOT
  dissolve; R27 stands.**
- **Convergence score 2, eleventh round.** Dimension 5 rises 2 → 4 (NF-1, NF-2
  closed). Dimension 1 is still 2 and the score is the MINIMUM.

## Gates round 45 ran, and what it did NOT run (R37)

**RAN:** full suite **12 ok / 0 fail** with `PERSENSE_FUZZ` unset, **after the
last edit**; `check_skips.sh` **32/32**; both backward bit harnesses at
`PERSENSE_BITS_N=1500`; `paired_regression` 10 seeds; FIX 5; round-trip md5
**12 of 12**; scratch build exit 0 / vet 0; all three oracles rebuilt with their
smoke tests READ.

**DID NOT RUN:** the PV and mortgage ARMS (no PV or mortgage engine file
changed), `sec72_horizon_arm.py`, `era_split_arm.py`, the plain arm, and the
gated (`PERSENSE_FUZZ=1`) suite — **which is RED BY DESIGN.**

**ORACLE BUILD FLAGS, beside every zero (R47):**
`-Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX`. **`-dACTU` IS ABSENT AND
UNBUILDABLE** — the `ACTUARY` unit source is missing (§82). Say **ABSENT**, never
"pending".

## Standing hazards a fresh round must not re-learn

- **`device_bash` CANNOT DELETE**; `git rm` fails, `git rm --cached` and `mv`
  work. **Rename `.git/index.lock` and `.git/HEAD.lock` IN PLACE before and after
  every git write; read `git log`, not the unlink warnings.**
- **`cd X && setsid nohup … & disown ; cmd` runs `cmd` in the ORIGINAL cwd** —
  the `&` backgrounds the whole `cd &&` list (note #55, fired again in r45).
- **`tar x` on the SSK mount cannot overwrite** — new files only.
- **A tarball set that predates HEAD can silently REVERT a commit** (note #58).
  `grep -c '## §NN'` before every push.
- **A join across two log lines can return a spectacular, wholly false answer** —
  round 45's first item-5 result was "20 of 20" because one line ends
  `… adjdump bdump apr` and the other `… adjdump bdump`. **Normalise the key, and
  check whether the emitter prints one line per CASE or per CLASS (R52).**
- **A mutation needle that does not match reads exactly like a surviving
  mutant.** Count the needle first and report INVALID.
- **`PERSENSE_FUZZ_N`, not `FUZZ_N`, is what the test reads** (note #52).
