# START HERE — Per%Sense port (repo snapshot)

> **⚠️ THIS SNAPSHOT IS PARTIAL AS OF ROUND 33 AND SAYS SO RATHER THAN PRETENDING
> OTHERWISE.** §9's snapshot rule puts a copy here at commit time; round 33
> refreshed **the §1 bootstrap block (new md5s, sizes and counts) and this
> header** but did NOT re-copy the whole body, which is still ROUND 32's text.
>
> **THE LIVE DOCUMENT IS THE PROJECT COPY: `claude/START_HERE.md` in the claude.ai
> project "Persense". READ THAT ONE.** Everything below §1's bootstrap block
> predates round 33 and its §2, §3a, §3b, §4, §5, §6, §8 and §9 are superseded.
>
> **Round 33's state in one line:** the 475 in-scope stacked HARD cases are an
> ENGINE-COVERAGE result — `dosPortCanHandle` routes 96.5% of the stacked
> population to the piecewise fallback and every measured divergence is there;
> the faithful port answers 166 of 4,720 routed cases with ZERO divergences.
> Full account: `docs/discrepancies.md` §70, and
> `claude/round33_engine_coverage_the_475_are_the_piecewise_fallback_2026-08-05.md`.
>
> **⚠️ ONE PART OF THIS FILE IS AUTHORITATIVE AND THE PROJECT COPY IS NOT: the §1
> bootstrap block below carries the round-33 tarballs' REAL md5s and sizes.**
> The project copy's §1 names the r33 tarballs correctly but repeats round 32's
> BYTE SIZES for `r33dos`/`r33fix` and gives no md5s. **Take the tarball md5s and
> sizes from HERE, not from there** — and fix the project copy the next time it is
> rewritten.
>
> **Refreshing the rest of this file is a round-34 chore.**

---


**Last updated: 2026-08-04, ROUND 32 COMPLETE. §3b IS AN ORDERED ROUND-33 WORK
PLAN — start there.**
**(a) 🚨🚨 THE HEADLINE NUMBER MOVED, AND IT MOVED THE WRONG WAY. §65's ADVISORY
SUBCLASS WAS AN ORACLE-DRIVER ARTIFACT.** DOS's *"Internal error - last payment
not found"* is a **BARE `MessageBox` STATEMENT** (AMORTOP.pas:1233) — no `exit`,
no `errorflag` — and real DOS's `MessageBox` is a Delphi `TForm` that latches
NOTHING. **`MakeTable` had already filled `Output`; `amort_oracle.pas:1104`
threw it away.** The DOS engine answers those screens.
**(b) 🚨 BOOKED, PAIRED, FOUR ARMS, ONE LINE OF ORACLE DIFFERENCE: in-scope
COMPARED 34,412 → 35,000, in-scope HARD **0 → 475**. The honest in-scope stacked
rate is **1 in 74**, not zero in 34,412. **THE EXIT CRITERION (1 in 400) IS
MISSED BY 5.4×.** 475 of the 588 newly visible cases are HARD — 81%.**
**(c) ✅ DECISION 3a.4 IS WITHDRAWN (Nate, 2026-08-04). The port must ANSWER;
DOS answers. The defect is the ARITHMETIC, and it is ONE CLASS: the port's total
interest is LOWER in 53 of 54, row counts equal in 49 of 54, and the schedules
separate MID-SCHEDULE AT AN ADJUSTMENT. **THAT MECHANISM IS ROUND 33's ENGINE
WORK — §3b item 1.****
**⚠️ (c2) A CORRECTION, MADE THE SAME DAY: the `dInt == dPaid` 54/54 figure first
written up as the signature IS NOT EVIDENCE — 62 of the 68 scored repros retire on
BOTH sides, and for those the identity is arithmetic. **AND WHERE A SCHEDULE DOES
NOT RETIRE, DOS MOSTLY DOES NOT EITHER: of the 6, five have a DOS residual too and
only ONE is the port alone.** Round 31's *"9% the port ships a loan that never
pays off, to \$323,361"* is ~1 in 68. See §3b item 1.**
**(d) ✅ THE PLAIN SURFACE RE-MEASURED AND IT REPRODUCES ROUND 22 EXACTLY —
108,778 in-scope, 13 signals, **every one confirmed `tie=true`**. Rule 11's
probation on the plain figures is DISCHARGED.**
**(e) ✅ CAUTION 6 RESOLVED — the published ≥99.9966% is the exact **two-sided**
95% Poisson limit (3.688879/N); §2's rule is the **one-sided** one. Both correct.
**BUT THE TWO HEADLINE FIGURES WERE NEVER ON THE SAME FOOTING — see §2.****
**(f) ⚠️ R26 — A REFUSAL IS A CONTROL-FLOW CLAIM, AND IT BELONGS TO WHOEVER
WROTE THE `exit` (§4.22).**
**(g) EVERYTHING IS COMMITTED. Suite GREEN with `-count=1` (12 packages, 0 FAIL,
0 cached); `check_skips` 32/32.**

Read this first, in full. It is short by design; everything else is a pointer.

> **Nate:** to continue in a fresh session, say *"Read `claude/START_HERE.md` and
> continue."* If the project's custom instructions carry §0's line, say
> *"continue"*.
>
> **This file lives in the claude.ai project, NOT on the SSK drive.** A snapshot
> goes to `docs/history/START_HERE.md` at commit time (§9). **Round 32 refreshed
> it — it had been 12 rounds stale.**

> **Agent:** §1 gets you a container. §2 is where things stand. **§3a carries the
> standing decisions and §3b is the round-33 plan, ORDERED — do it in order.**
> §4 and §5 are non-negotiable. **Update §2, §3, §7 and §8 before the session
> ends.**

---

## 0. How to make this automatic (one-time setup, Nate)

Paste into the Project's custom-instructions box (claude.ai → Persense →
Instructions):

```
This project is a DOS/Delphi Pascal → Go port (Per%Sense).

At the start of EVERY session, before doing anything else, read the project doc
claude/START_HERE.md and follow it. It carries the current state, the next
action, the standing rules, and the known traps. Do not begin work from an older
round write-up — those are history; START_HERE is the live state.

Update START_HERE before the session ends.
```

---

## 1. Getting a working container (~5 minutes)

### ⚠️⚠️ THE DRIVE IS AT `/Volumes/SSK/persense/PerSense-Web`

### ⚠️⚠️ FIX 2 — **CALL THE GRANT TOOL FIRST. IT WORKS.**

`device_request_folder_access` on the SSK path returned
`{"granted":["/Volumes/SSK/persense/PerSense-Web"]}` immediately in rounds 29
through **32**. No "Add folder" click needed. Only ask Nate if it fails.

### ⚠️⚠️ FIX 2b — `device_bash` — **ALIVE IN ROUNDS 30, 31 AND 32**

Dead in rounds 27 and 28 (`Workspace unavailable`), working since. **Try it — it
is worth a lot when it answers:** it runs `git log`/`git status`/`git commit` on
the drive and **md5s the synced files ON THE DRIVE**, which beats note #18's
metadata check outright. Round 32 verified all three synced files that way.

**⚠️ `device_bash` SEES THE DRIVE AT ITS SESSION MOUNT, NOT AT `/Volumes/...`.**
`cd /Volumes/SSK/...` fails with *No such file or directory*. Find the real path
with `ls /sessions/*/mnt/` first — it is
`/sessions/<session-id>/mnt/PerSense-Web`. (`device_stage_files` and
`device_commit_files` DO take the `/Volumes/...` path. The two tool families
disagree; this is not a bug to chase.)

**⚠️ `device_bash` CANNOT DELETE. And git on this mount CANNOT UNLINK ITS OWN
LOCK FILES** — `.git/index.lock` **and `.git/HEAD.lock`** (notes #20/#21).
**The recipe: `mv .git/*.lock _to_delete/` IMMEDIATELY BEFORE every git command
that writes the index or a ref, and again after.**

**⚠️ ROUND 34: A CURRENT THREE-TARBALL SET IS ON THE DRIVE.**

```
_to_delete/r33src.tar.gz   421e45f1ef68296b7e0a15e4d306ea02   2,856,199 B
_to_delete/r33dos.tar.gz   099986e1791a50ee80fc20438485f048     195,311 B
_to_delete/r33fix.tar.gz   808e446d5561eb8d8707c701a12fc498     210,457 B
```

Verified by extraction at build time: **418 `.go`, `pkg/` present, 42/42
resolvable symlinks, 34 `.pas` in `dos_source`, both fixture trees present**, and
the member list diffs against r32's as *"last round's set plus exactly what round
33 added"* — `zzsec70_engine_route_test.go`, `engine_attribution_arm.py` and
`localise_divergent_row.py`. (r32's tarball carried a `./` root member and r33's
does not; extraction is unaffected.) **Check the counts yourself anyway** (§1
recipe step 5).

**Running the round ON NATE'S COMPUTER** (desktop app → **"Run this task" picker,
top right**) removes the grant step and the tar/stage bootstrap at once.

### ⚠️⚠️ FIX 1 — THE TARBALL MUST INCLUDE `legacy/src/dos_source`…

`legacy/oracle/units` is a **SYMLINK FARM**. Tar stores the links; `ls` lists 42
plausible filenames, every one dangling.

### ⚠️⚠️ …AND THE LINKS ARE ABSOLUTE TO A PREVIOUS SESSION'S MOUNT PATH

```
legacy/oracle/units/about.pas -> /sessions/funny-tender-pascal/mnt/PerSense-Web/legacy/src/dos_source/about.pas
```

**Recreate the path — verbatim in rounds 27 through 32:**

```
mkdir -p /sessions/funny-tender-pascal/mnt
ln -sfn /root/pw /sessions/funny-tender-pascal/mnt/PerSense-Web
```

### ⚠️⚠️ FIX 3 — THE FIXTURES TARBALL IS MANDATORY FOR ANY SUITE CLAIM

Without `legacy/reference-output/` and `legacy/src/win_source/` the full suite
**FAILS** (20 `TestCrossCheck*`, `TestImportPSN_*`,
`TestLoadHelpWorksheetCorpus`) — every one of them the container, not the port.
**THE BOOTSTRAP IS THREE TARBALLS: src + dos_source + fixtures.**

### ⚠️⚠️ FIX 4 — **REBUILD A TARBALL WITH ITS DIRECTORY ENTRIES**

A file-only member list loses the empty `pkg/` directory. Use
`tar --no-recursion -T <list>` with DIRECTORY entries included, then extract to a
scratch dir and re-run all the checks before shipping. **And diff the member list
against the previous tarball's.**

### The recipe

1. **Call `device_request_folder_access` on the drive path.** Ask Nate only if
   it fails.
2. Extract the three tarballs into `/root/pw`; recreate the absolute symlink
   path; md5-verify.
3. **`pip install actuarialmath ipython scipy --break-system-packages`.**
   **Do NOT `apt install fpc`** — `build_linux.sh` stages its own into
   `/tmp/fpcroot`.
4. Oracles: `legacy/oracle/build_linux.sh`, then `TARGET=pv_oracle`,
   `TARGET=mtg_oracle`. **Each prints its own smoke test — read it**
   (`payment 888.4879` / `pv 9231.163464` / `monthly 1066.683053`).
   **`go build -o /tmp/goamort ./cmd/goamort` too.** **Build all three every
   round**, or `check_skips.sh` FAILs on 34 PV/mortgage skips that are your
   container, not the port. Binaries land in **`/tmp/oraclebuild/`**.
5. Verify: **416** `.go` files (417 with round 32's new test), 42/42 resolvable
   symlinks, 34 `.pas` in `dos_source`, `pkg/` present, fixtures present.
6. **Extract a PRISTINE copy** to `/tmp/pretree` and `/tmp/pristine`. **Extract
   the DOS tarball there too** if you will build a PRE oracle: `build_linux.sh`
   needs `legacy/src/dos_source/VIDEODAT.pas` and fails obscurely without it.

**⚠️ TO BUILD A *PRE* ORACLE (round 32's method, cheaper than a pristine tree):**
`cp -a legacy /tmp/proberepo/legacy`, edit the ONE line in
`/tmp/proberepo/legacy/oracle/Globals.pas`, then
`OUT=/tmp/probebuild STAGE=/tmp/probestage /tmp/proberepo/legacy/oracle/build_linux.sh`.
**md5 the two binaries to prove they differ**, and point the harness at either
with **`PERSENSE_ORACLE=<path>`** — `run_arm.sh` honours it, which is how round
32's paired arms were run.

**To leave the next round a bootstrap:** build the three tarballs in the
container, `SendUserFile` each to get a `file_uuid`, then `device_commit_files`
them to `_to_delete/`. Rounds 28-32 did exactly this.

**⚠️ `~` IS `/root` FOR `Bash`, BUT `Write` RESOLVES `/home/claude`.**
**⚠️ `cd` DOES NOT PERSIST ACROSS LINES.** *(Round 32 lost two script runs to
this — `python3 testplan/harness/...` from the wrong cwd.)*
**⚠️ `pkill -f <pattern>` KILLS THE TOOL'S OWN SHELL.** Kill by PID.
**⚠️ THE CONTAINER CLOCK DRIFTS.** Check rather than assume.
**CHECK `nproc`.** Rounds 13-32 got **2**. Do not override `JOBS`. **8 GB RAM.**
**⚠️ A PASCAL `{ }` COMMENT DOES NOT NEST — and pasting DOS source into one
closes it early** (`end; {RepayFancyLoan}`). Use `(* *)` inside, or strip.

**GATE vs MEASUREMENT.** `paired_regression.sh` is the FIXED/STILL/NEW gate.
`era_split_arm.py` measures the IN-SCOPE CASE rate — **the headline
denominator**, and it pools SEVERAL ARM DIRS; `analyze_arm.py` measures SIGNAL
INSTANCES; `run_plain_arm.sh` + `analyze_plain_arm.py` measure the plain surface
(**one dir per invocation — sum the three by hand**); `run_pvmtg_arm.sh` +
`analyze_pvmtg_arm.py` measure PV and mortgage. **And run `check_skips.sh`.**

**⚠️ `go test` CACHES. A SECOND SUITE RUN AFTER AN ORACLE-ONLY CHANGE IS
100% CACHED AND CERTIFIES NOTHING** (note #22). **Use `-count=1`** and check with
`grep -c '(cached)'`. Round 32 measured **0 cached**.

**⚠️ AFTER EVERY ARM RUN, CHECK FOR SEEDS WITH NO `ledger:` LINE.**

```
for d in <armdir>/*/; do echo "$(grep -l 'ledger:' $d/seed_*.log|wc -l)/$(ls $d/seed_*.log|wc -l)"; done
```

### TIMING (2 cores) — corrected round 32

| what | wall clock |
|---|---|
| bootstrap from reused tarballs + 3 oracles + goamort | **~5 min** |
| **full `PERSENSE_REQUIRE_ORACLE=1 go test ./... -count=1 -v`** | **~3 min uncontended (r32), ~6 min contended** |
| `check_skips` REUSING that log via `SUITE_LOG=` | **seconds** |
| **FOUR fuzzer5 arms, 160 seeds, `FUZZ_N=400`** | **~9 min even with a plain run alongside (r32)** |
| **THREE PLAIN arms at `PLAIN_N=1200`** | **~19 min measured r32 — NOT 40** |
| **harvesting FLAKEDUMP repros, 20 seeds** | **~4 min** |
| `audit_sec65_messagebox_probe.py`, 95 repros + 148 controls | ~8 min |
| `paired_regression` 40 seeds, BOTH passes | ~20-25 min. Seed 40028 burns its full 900 s timeout on EACH pass |
| PV or mortgage arm, 20 seeds, `FUZZ_N=1500` | ~13 min each |
| PV / mortgage backward BIT harness at `PERSENSE_BITS_N=1500` | **~1 s. Run them wide** |
| `era_split_arm.py` / `analyze_pvmtg_arm.py` | seconds |
| building one oracle (canonical or probe) | ~40 s |

**Plan for breadth. THE CONTAINER CAN RESTART MID-SESSION. Sync early.**
**START THE LONG RUNS FIRST and do analysis while they run.** *(Round 32 ran the
plain arms, the fuzzer5 arms and a repro harvest concurrently on 2 cores and lost
nothing.)*

---

## 2. Current state

Round 32 write-up:
**`claude/round32_sec65_was_the_oracle_driver_rate_rebased_2026-08-04.md`**.
Live convergence assessment: **`claude/convergence_assessment_2026-08-04_round31.md`
— ⚠️ ITS HEADLINE IS SUPERSEDED BY ROUND 32 (§1 of the round-32 doc); its
methodology section still stands.**
Decisions: **§3a of this file**, plus
`claude/decisions_2026-08-04_sec69_boundary_and_sec65_fallback.md`
(**⚠️ its §65 half is WITHDRAWN IN FULL — see 3a.4 and
`claude/WITHDRAWN_NOTICE_sec65_refuse_decision_2026-08-04.md`**).

**⚠️ Before quoting ANY number, read the SIX cautions below.**

| surface | measured | round | verdict |
|---|---|---|---|
| **🚨 STACKED OPTIONS — IN SCOPE ≤2099, COMPARED CASES** | **475 HARD in 35,000** | **r32 (4 arms)** | **1 in 74 = 98.643%. THE EXIT CRITERION (1 in 400) IS MISSED BY 5.4×** |
| — *the same four arms, same seeds, PRE the oracle fix* | *0 HARD in 34,412* | **r32** | ***≥99.99130% — THE FIGURE THAT WAS PUBLISHED. IT MEASURED A TRUNCATED POPULATION*** |
| — the newly visible cases | **475 HARD of 588** | **r32** | **81%** |
| — attribution | **`HARD:divergent_class` 13 → 502; every other SIG flat** | **r32** | **ONE CLASS** |
| — out of scope >2099, CASES | 48 HARD in 1,459 | r32 | 1 in 30 |
| **§65's in-scope advisory bucket** | **690 → 0** | **r32** | **CLOSED — those cases are COMPARED now** |
| — the remaining in-scope `date-horizon` answered case | **1** | r32 | **§69, a representation limit** |
| **PLAIN LOANS — 3 standing ranges, RE-MEASURED** | **108,778 in-scope, 13 signals** | **r32** | **IDENTICAL TO r22 — 0 arithmetic divergences** |
| — the 13 | **ALL 13 carry `tie=true`, individually read** | **r32** | **half-cent print ties** |
| — 95% bound | 0 events in 108,778 | r32 | **two-sided ≥99.99661% · one-sided ≥99.99725% — CAUTION 6** |
| — out of scope >2099 | 1,001 signals in 16,141 cases | r22 | 1 in 16 |
| **PRESENT VALUE — forward**, seeds 20611-20630 @ `FUZZ_N=1500` | 29,917 worksheets, 5,095,860 table lines | r29 | **0 divergences** |
| — backward RATE, BIT LEVEL | 1,500 compared, 1,499 bit-exact, max 1 ULP | r30 | **0 divergences, no sign bias** |
| **MORTGAGE — forward**, seeds 20614-20633 @ `FUZZ_N=1500` | 30,000 eval cases, 135,853 APR verdicts | r29 | **0 divergences** |
| — backward BALLOON AMOUNT, BIT LEVEL | 1,500 compared, 1,499 bit-exact, max 1 ULP | r30 | **0 divergences, no sign bias** |
| — mortgage APR at bit level | **not yet reached — NOT BLOCKED, §3b item 5** | r31 | |
| **Backward solves, LONG HORIZON** | 17,031 paired solves | r20 | 0 term diffs |
| **§54 long horizon** | A 1.43% · B 0.71% · C1 1.5% · C2/C3 12.8% · D 5.5% | carried | **entirely >2100 — OUT OF SCOPE by the client rule** |
| **Actuarial** | 663 vs `actuarialmath` + 5 DOS goldens | carried | **0 divergences** |
| **Payoff** | 70 golden + 428 randomized | carried | **1 divergence (§60) — 13 rounds open** |

**⚠️ CAUTION 1 — UNITS, MANY POPULATIONS.** CASE rate ≠ SIGNAL INSTANCES ≠ ROWS ≠
PV WORKSHEETS ≠ TABLE LINES ≠ APR VERDICTS ≠ BIT COMPARISONS. **Never mix two.**

**⚠️ CAUTION 2 — POPULATION.** `dos_fuzzer5` SKIPS every plain loan; the plain
differential covers ONLY plain loans; PV and mortgage are two further independent
surfaces. Name which.

**⚠️ CAUTION 3 — DISCHARGED (r32).** All four arms now sit on one oracle version,
both PRE and POST, same 160 seeds.

**⚠️ CAUTION 4 — THESE ARE BOUNDS, NOT POINT ESTIMATES**, and **THE PROJECT HAS
BEEN QUOTING TWO DIFFERENT CONVENTIONS.** The plain figure ≥99.9966% is the
**exact two-sided 95% Poisson** limit, `chi2(0.975,2)/2 / N = 3.688879/N`. §2's
older rule `1 − 0.05^(1/N)` is the **one-sided** limit, `2.99573/N`. On the SAME
34,412 the stacked bound is **99.99130% one-sided but 99.98928% two-sided —
BELOW 99.99%.** **Pick one convention and state it.**

**⚠️ CAUTION 5 — PV AND MORTGAGE FORWARD FIGURES ARE ROUND 29's, backward are
ROUND 30's.** Quote each with its seed set / `PERSENSE_BITS_N`. The retired
figures 29,891 / 5,034,725 / 20,754 / 136,270 must never reappear.

**⚠️ CAUTION 6 — RESOLVED (r32), see CAUTION 4.** The published plain bound was
never wrong and its denominator was never in doubt; it is the two-sided limit.

**⚠️ CAUTION 7 — NEW, AND IT IS THE BIGGEST ONE. EVERY PUBLISHED STACKED FIGURE
BEFORE ROUND 32 MEASURED A TRUNCATED POPULATION.** ~690 in-scope screens per 160
seeds were excluded because the ORACLE DRIVER discarded a table the DOS engine
had built. **Any stacked rate quoted from rounds 22-31 — 99.9729%, 99.9922%,
≥99.99128%, ≥99.99130% — is a rate over cases the oracle chose to show us.**
The correction is in the tree; the numbers in those documents are not.

### The statistical position

- **Plain, in scope: 0 arithmetic divergences in 108,778 → ≥99.9966% (two-sided).
  RE-MEASURED at round 32 and identical to round 22 to the case.** This claim is
  the strongest thing the project owns and it is now reproduced, not carried.
- **🚨 Stacked, in scope: 475 HARD in 35,000 = 1 in 74.** The exit criterion is
  1 in 400. **The bar is NOT met on the stacked surface, and the previous belief
  that it was met with enormous margin was an artifact of the instrument.**
- **The whole of the gap is ONE signal class** (`divergent_class`, i.e. totals):
  same row count in 49 of 54, the port's total interest LOWER in 53 of 54,
  separating mid-schedule at an adjustment. **That is a root-cause target, not a
  scatter — but see §3b item 1 for what is NOT evidence.**
- **PV and mortgage: forward zero (r29); backward bit-verified zero on the two
  solves the oracle can witness (r30) — over an envelope nobody has audited.**

### Harness policy scoreboard

| rule | status |
|---|---|
| R1-R9 | **LANDED** (R2 partial) |
| R10 tolerances scaled to the value guarded | **LANDED + all five audited** |
| R11 solvers verified by bit distribution and SIGN BALANCE | **LANDED**, re-scoped by R14 |
| R12 a skip is not a pass | **LANDED** |
| R13 an instrument may print only what it has READ | **LANDED** — caught the ORACLE in r30, r31 AND r32 |
| R14 materiality in the SOLVER'S acceptance units | **LANDED (r20)** |
| R15 a "covered elsewhere" note is a coverage claim | **LANDED (r21)** |
| R16 a terminal `continue` must say what it ASKED | **LANDED (r22)** |
| R17 diff the RESIDUAL before the SOLVER | **LANDED (r25)** |
| R18 an improvement on the SAME sample carries its significance | **LANDED (r26)** |
| R19 a mechanism is confirmed by its NEGATIVE CONTROLS | **LANDED (r26b/c)** |
| R20 a fix that changes NOTHING has not been confirmed | **LANDED (r27)** |
| R21 a correction must be SCOPED to the reconstruction that lost the value | **LANDED (r28)** |
| R22 an ALIASING claim is scoped to a CALL SITE, not to a routine | **LANDED (r29)** |
| R23 a MECHANISM found on ONE case is scoped by that case's ACCIDENTS | **LANDED (r30)** |
| R24 a POSITIVE control is as obligatory as a NEGATIVE one | **LANDED (r31)** |
| R25 a population nothing compares is an unaudited LIABILITY | **LANDED (r31) — REMEDY CORRECTED BY r32: make it comparable, do not make the port refuse** |
| **R26 a REFUSAL is a CONTROL-FLOW claim, and it belongs to whoever wrote the `exit`** | **LANDED (r32) — §4.22** |
| Note #17 — `eraCompared` vs `ACTUALLY COMPARED`, off by 24 | **FILED**, 0.09%, not chased |
| Note #18 — verify a sync by the commit result and re-stage METADATA, never by the staged copy's CONTENT | **FILED (r28)** — superseded whenever `device_bash` answers |
| Note #19 — `check_skips.sh` accepts `SUITE_LOG=` | **FILED (r29)** |
| Note #20/#21 — git on the SSK mount cannot unlink `index.lock` OR `HEAD.lock`. Clear BOTH, BEFORE and AFTER. | **FILED (r30/r31)** |
| Note #22 — `go test` CACHES; a re-run after an ORACLE-ONLY change is 100% cached. `-count=1`. | **FILED (r31)** |
| Note #23 — `device_bash` sees the drive at `/sessions/<id>/mnt/…`, the stage/commit tools want `/Volumes/…` | **FILED (r31)** |
| Note #24 — `goamort` does NOT implement `norate`/`noamt`. Any goamort-driven audit silently drops those screens. | **FILED (r31)** — **cost 17 of 95 again in r32** |
| **Note #25 — `PERSENSE_ORACLE=<path>` re-points the WHOLE harness, `run_arm.sh` included. This is how a PAIRED ORACLE measurement is run.** | **FILED (r32)** |
| **Note #26 — `analyze_plain_arm.py` takes ONE directory. Pooling three arms means summing by hand.** | **FILED (r32)** |

### Drive and commit state

**HEAD is `04de8b3`** (round 32 landed as `6fc6927` + the snapshot refresh `04de8b3`). **THE WORKING TREE IS CLEAN** (only `_to_delete/` untracked).
Verified by `device_bash` after the commit. The round's five changed/new files
were each md5-verified ON THE DRIVE.

⚠️ **Verify the commit state with `git log`/`git status`, do not read it out of
this file** — it has been stale at the start of four of the last four rounds.

---

## 3a. STANDING DECISIONS (Nate)

1. ~~**§65, DOS's internal-error subclass: ANSWER, BUT SAY SO.**~~ **SUPERSEDED.**
2. **§63, the terminating-balloon final row: MATCH DOS.** **⚠️ Worth ZERO against
   the rate** — it does not trip the whole-case HARD classifier.
3. **The headline claim is stated PER IN-SCOPE CASE, SPLIT BY SURFACE.** Plain
   ≥99.9966%; **stacked is now 1 in 74 and no bound should be quoted for it until
   the mechanism in §3b item 1 is fixed.** Do not pool them. **And state the
   interval convention (CAUTION 4).**
4. ~~**§65's advisory subclass: THE PORT REFUSES.**~~
   **🚨 WITHDRAWN 2026-08-04 (Nate), on round 32's reading.** Its premise was
   that DOS declines to answer. **DOS answers** — AMORTOP.pas:1233 is a bare
   `MessageBox` and `MakeTable` had already filled `Output`. **The port was right
   to answer and is wrong in its ARITHMETIC.** Round 31's *"91% of those screens
   the port is fine"* asked *does the port's own schedule terminate*, having no
   DOS answer to compare against; with one, the port diverges on **81%**.
5. **§69: move the `date-horizon` bucket's in-scope boundary to DOS's real
   ceiling (26 August 2091) and publish the 2091-2099 band as a named, measured
   gap.** UNCHANGED and still unimplemented — §3b item 3.
6. **The mortgage APR bit surface needs NO rule-7 call** — the trace-oracle
   mechanism reaches it. Verified 2026-08-04.

---

## 3b. THE ROUND-33 WORK PLAN — DO IT IN THIS ORDER

### 1. 🚨 ROOT-CAUSE THE 475. This is the round's engine work, and it is the whole ballgame.
*The stacked surface's entire measured gap is one class with one signature.*

**What is known** (round 32, `audit_sec65_messagebox_probe.py` + the arms):

- **⚠️ RETRACTED SAME DAY, BEFORE IT DROVE ANYTHING: `dInt == dPaid` to the cent
  in 54 of 54 is NOT evidence of a mechanism.** It was first written up as *"the
  total principal repaid is identical, the whole difference is interest"*. It is
  not: **50 of the 54 retire to zero on BOTH sides**, and two schedules that both
  retire the same loan differ in `paid` by exactly what they differ in
  `interest`. The identity is arithmetic. **⚠️ AND IT IS NOT FULLY UNDERSTOOD —
  the other 4 leave a RESIDUAL, whose difference should break the identity and
  measurably does not. That is an open loose thread in the instrument, not a
  finding.**
- **the port's total interest is LOWER in 53 of 54** — this one is real and is
  the divergence.
- row counts are **EQUAL in 49 of 54** (DOS one row longer in the other 5).
- on a worked example the first rows agree EXACTLY and the schedules separate
  **mid-schedule, after an adjustment**.
- **🚨 AND WHERE A SCHEDULE DOES NOT RETIRE, DOS MOSTLY DOES NOT EITHER.**
  Over all 95 repros (68 scored, 27 unscored — R12): **62 retire on BOTH sides**,
  6 do not. Of those 6, **FIVE have a DOS residual too** and only **ONE is the
  port leaving a balance where DOS retires**:

```
                    DOS            port         delta
                   0.00        5,732.34      5,732.34   <-- PORT ONLY. A real defect.
               1,648.42        3,732.75      2,084.33
             173,539.45      174,711.69      1,172.24
             322,478.08      323,361.06        882.98
              26,589.35       27,299.84        710.49
               4,517.71        4,988.32        470.61
```

  **Round 31's *"9% of these screens are the port shipping a schedule and totals
  for a loan that never pays off, residuals to \$323,361"* is therefore ~1 case
  in 68, not 9%** — the rest is DOS's own behaviour, reproduced to within a few
  hundred dollars. That figure was the specific evidence decision 3a.4 rested on.
  It is gone twice over: the screens are not refused, and the residual is almost
  never the port's alone.
- **⚠️ ONE UNCONFIRMED COINCIDENCE, WORTH EXACTLY ONE PROBE AND NO MORE:** on the
  322,478 / 323,361 case the delta is **882.98** and the screen carries
  **`targ=882.98`**. Exact to the cent, on one case out of six, found by someone
  looking for a pattern. **It is probably chance. Check it, do not build on it.**
  (Round 32 spent an hour of this round on a pattern that turned out to be an
  accounting identity; this is the same shape of temptation.)

**Suspect a `Re_Amortize` / rate-reconstruction site — the §66/§67 family, "a
routine faithful to the original, reached by a caller that is not", the dominant
class in this port.** But hold that loosely; it is a prior, not a measurement.

**START WITH THE TRACE, NOT WITH A HYPOTHESIS.**
`scripts/build_trace_oracle.sh -mode cn` builds in ~40 s and gives DOS's per-row
`ComputeNext` (years +1900). **Localise the FIRST divergent row exactly on three
or four repros before forming any theory.** The trigger condition is DOS's own —
`DateComp(WhenToStop^.date, very_last) > 0`, i.e. the walk overshot `very_last` —
so the rows PAST `very_last` are where to look first.

**⚠️ IGNORE THE `dosport_walk.go:156` LEAD THIS PLAN ORIGINALLY CARRIED.** It was
derived from the retracted principal reading above: that fold governs whether the
final row ABSORBS the residual, and the residual evidence says both engines
behave the same way there.

**Rule 5 first: read the source. Rule 4: gate it and BOOK it in the same
session** — a paired four-arm re-run is ~9 minutes and the PRE column for it is
already recorded above.

**Repros:** harvest with `PERSENSE_FUZZ_FLAKEDUMP=1`; note the SIG name has
changed — those cases are now ordinary `SIG=HARD:divergent_class` lines, no
longer `ADVISORY:go_solved_dos_internal_error_in_scope`.

**⚠️ CLOSE NOTE #24's HOLE FIRST or it costs another 17 of 95:** teach `goamort`
`norate`/`noamt`, or measure through `dos_fuzzer5_test.go` which implements both.

---

### 2. RE-STATE EVERY PUBLISHED NUMBER, ONCE, UNDER ONE CONVENTION
*An hour, and nothing client-facing should move until it is done.*

CAUTION 7 says every stacked figure from rounds 22-31 measured a truncated
population. CAUTION 4 says the two surfaces' bounds use different Poisson
conventions. **Both are now known and neither is recorded anywhere a client would
read.** Produce one table: surface, population, denominator, events, convention,
bound — and mark the superseded figures as superseded **in the documents that
carry them**, not only here.

---

### 3. MEASURE THE 2091-2099 BAND, then apply decision 3a.5
*Half an hour. Round 32 did NOT do this.*

It needs a **Go-side** change: the era split is keyed on the port's own resolved
dates (R2), and `fz5MaxYear` returns a **YEAR**, so a 26-August-2091 boundary
needs a max-**DATE** companion. Keep the existing line byte-identical (compute
the sub-band only within `caseEra == 0`) so `era_split_arm.py` still parses and
the two eras' counts are provably unchanged.

**⚠️ A PREDICTION TO FALSIFY.** If a COMPARED case is one DOS answered, DOS never
hit the ceiling on it, so the compared population should **already** be bounded at
~26 Aug 2091 and the middle band should be near **empty**. **If it is not, that
reasoning is wrong and the finding is more interesting than the decision.**

---

### 4. ⭐ PV/MORTGAGE SAMPLE-SPACE AUDIT
*Half a round. Untouched by rounds 31 and 32.*

`docs/fuzzer_sample_space_audit_2026-08-02.md` is amortization-only. The question
it asks — **"what can the generator NOT draw?"** — has returned a defect **8
times out of 8**. Nobody has asked it of PV or mortgage, whose zeros span ~5.1 M
table lines and ~136 k APR verdicts over an uncharacterised envelope.

1. Read both generators. 2. Build the **axis → drawn → CANNOT produce → status**
table. 3. Land `zzsamplespace_test.go` in **both** packages. 4. **Then widen ONE
silent axis and measure.** Do not stop at the table.

---

### 5. MORTGAGE APR AT BIT LEVEL — unblocked, no rule-7 call
Add **`-mode aprbits`** to `build_trace_oracle.sh`, staging a `Mortgage.pas` that
emits the raw APR double to stderr from inside `ReportAPR` (it is computed into a
LOCAL by `IterateToFindAPR` and thrown away after `ftoa4(100*apr, 7)` — there is
no record field). Extend `mortgage/zzbits_backward_test.go` to consume it.
**Caveat to state whenever the figure is quoted:** the claim rests on a **trace
build**, not the frozen oracle — same engine, same stdout, one extra stderr line.

---

### 6. CLOSE OR FORMALLY PARK §60
*Thirteen rounds carried.* Root-cause it or write the decision down the way §54
and §64 were parked — **but stop carrying it silently.**

---

### 7. THE PV BACKWARD AMOUNT SOLVES
`pv_oracle.pas:950-956`: the lump/periodic AMOUNT solves go through
`BackwardCalc`'s backup frame and are not drivable headlessly; `bk_asof` is
drivable but emits no bits. **Make them drivable or document them as out of
reach**, and say which.

---

### 8. ⭐ AUDIT THE OTHER `MessageBox` HELP CODES — R26's obvious next question
*An hour, and it is 4-for-4 so far.*

Four help codes have been read at their call sites and **all four turned out to
be non-fatal in the original**. `HelpSystemUnit.pas` defines many more.
**Grep every `MessageBox(` call site in the DOS units, classify each as
`exit`/`errorflag` or BARE, and check what `legacy/oracle/Globals.pas` does with
it.** Any other bare one is another truncated population.

---

### Standing items, below the plan

9. **§63's engine fix** (decision 3a.2). Decided, real, worth zero against the rate.
10. **A Go-side regression test for the seven** (needs goamort's parser factored
    out of `main()`).
11. ~~the `docs/history/START_HERE.md` snapshot is stale~~ — **refreshed r32.**
12. Grow the plain denominator further if a tighter bound is wanted.
13. Note #17; a per-option base rate over COMPARED cases; era-label audit of
    `long_horizon_sweep.py` and `zzpayoff_sweep_test.go`.
14. **The amortization sample space's own SILENT rows** — loans <$25k, rates <3%
    and ≥14%, non-whole-year terms, `Daily` compounding, balloons in the last
    20%, **a prepayment series starting past the entered term**, and the `skip`
    axis's 6 fixed patterns at perYr=12 only.
15. Carried: R16 on `refused`/`non-converged`; the `err == nil` date-advance
    class; **killed-binary bucket in `run_arm.sh` — seed 40028 burns a full 900 s
    timeout**; the ~15% marginal band; §54's other sites; the ±200%
    over-refusal; the two `refdata.json` FIXME skips; the whole-tree manifest.

### The exit criterion — RATIFIED 2026-08-03, STATUS AT ROUND 32

> **PV and mortgage at zero, RE-MEASURED at exit rather than carried forward, and
> bit-verified on their backward solves as well as forward.**
> — **FORWARD: DONE (r29).** **BACKWARD: DONE for the PV rate solve and the
> mortgage balloon-amount solve (r30); the mortgage APR is reachable (item 5) and
> the PV amount solves are not (item 7). Say which.**
> — **⚠️ THE CRITERION SAYS NOTHING ABOUT THE PV/MORTGAGE SAMPLE SPACE, WHICH HAS
> NEVER BEEN AUDITED (item 4).**
>
> **Amortization: HARD rate below 1 in 400 on the standing ranges**, no more than
> 10% of HARD signals unattributed.
> — **🚨 NOT MET. 475 in 35,000 = 1 in 74, missing the bar by 5.4×.** The
> mechanism clause is in better shape than the rate: **100% of the 475 sit in a
> single signal class with a single measured signature** (§3b item 1).
> — **PLAIN: MET and RE-MEASURED (r32), 0 arithmetic in 108,778.**
>
> Any exit claim travels with the sample-space audit's scope statement AND the
> seven cautions in §2.

---

## 4. Standing rules (non-negotiable)

1. **Sync per fix, not per session.** **Verify a sync by md5 ON THE DRIVE with
   `device_bash` when it answers; otherwise by the commit result and the
   re-stage/list METADATA, never by the staged copy's CONTENT (note #18).**
2. **A green suite is not validation.** Build **all three** oracles, run the GATED
   suite, **and run `check_skips.sh`** (note #19). **A round that did not run the
   suite must SAY so.** **If the ORACLE changed, the certifying run must be
   `-count=1` (note #22).**
3. **Every fix ships a regression test, verified BOTH directions, IN FACT.**
   **Build the "before" binary separately and md5 both.** **An ORACLE fix ships a
   test that reads the ORACLE BINARY's own output** — no Go assertion can see it
   otherwise (r32).
4. **Engine changes run the paired regression AND randomized coverage. NEW must
   be 0. A change that cannot be gated inside the session does not land.**
   **AND A LANDED FIX IS NOT A MEASURED ONE — book its effect in the same
   session, or say loudly that you did not.**
5. **Audit the source before fixing.** Fuzzing locates; only reading explains.
   **Round 32 is the strongest case yet: ten minutes of reading AMORTOP.pas:1233
   and MessageDialogUnit.pas overturned nine rounds of measurement.**
6. **Goldens carry provenance.**
7. **`legacy/src/**` is untouchable** — extract into a container, never sync back.
   **The oracle's OWN sources (`legacy/oracle/*.pas`), `build_linux.sh`'s staged
   copies and `build_trace_oracle.sh`'s staged copies are the sanctioned places
   to correct an ARGUMENT ENCODING, read an internal, or — r32 — CORRECT A
   MISCLASSIFIED DIALOG.** **A staged change to a DOS unit's INTERFACE is a
   policy call — ask Nate.**
8. **Ask what the generator CANNOT produce, what the instrument cannot JUDGE,
   what it SKIPS — and what it buckets and never asks about.** **8 for 8.**
9. **Never quote a co-occurrence without its base rate, never name a frontier
   without ADJUDICATING a sample, never mix signal instances with cases with
   ROWS, and never quote a rate without naming the POPULATION.**
10. **Internal-consistency tests never drive a behavior change.**
11. **Do not carry a claim forward without verifying it.** Retired: the commit
    state (**four rounds running**), "stratum C = 14%", "`non` is the frontier",
    §59's two wrong root causes, "the oracle's `rows` line carries the row's
    date", "§66 is two solvers landing in different basins", "`DPTRACE=1` prints
    DOS's secant", "the port's fancy walk starts at the typed FirstDate", "c3 is
    the piecewise engine missing Re_Amortize's VAR snap", "the last in-scope HARD
    case is §61", "the grant tool cannot reach an external volume", the
    PV/mortgage figures 29,891 / 5,034,725 / 20,754 / 136,270, "~300,000 in-scope
    cases are needed", "§65's `noterm` cases are DOS's term solve failing",
    "`-88/0/1900` is a garbage date", "one arm takes 9 minutes", "§65's 519
    screens are one kind of thing", and NEW in r32: **"DOS REFUSES §65's advisory
    screens"**, **"the port answers 519 screens nothing checks"** (something does
    now, and it fails 81% of them), **"the in-scope stacked HARD count is
    zero"**, **"the plain bound does not reproduce"** (it does — CAUTION 4), and
    **"three plain arms take 40 minutes"** (19), and **"`dInt == dPaid` in 54 of 54 is
    a mechanism signature"** (it is an arithmetic identity for the 50 of 54 that
    retire — RETRACTED THE SAME DAY IT WAS WRITTEN), and **"the port ships a
    schedule for a loan that never pays off, to \$323,361"** (**DOS leaves the same
    residual**).
    **⚠️ DISCHARGED: the plain surface's figures — re-measured r32, identical.**
12. **The harness is a suspect before the engine is.** **r30, r31 AND r32 all
    ended with the harness at fault.** Round 32's variant: **the harness can be at
    fault in the direction that FLATTERS the port.**
13. **A SESSION THAT CANNOT GET A CONTAINER MUST SAY SO EARLY.**
14. **AN IMPROVEMENT MEASURED ON THE SAME SAMPLE CARRIES ITS SIGNIFICANCE.** R18.
15. **A MECHANISM IS CONFIRMED BY ITS NEGATIVE CONTROLS.** R19.
16. **A FIX THAT CHANGES NOTHING HAS NOT BEEN CONFIRMED.** R20.
17. **A CORRECTION MUST BE SCOPED TO THE RECONSTRUCTION THAT LOST THE VALUE.** R21.
18. **AN ALIASING CLAIM IS SCOPED TO A CALL SITE, NOT TO A ROUTINE.** R22.
19. **A MECHANISM FOUND ON ONE CASE IS SCOPED BY THAT CASE'S ACCIDENTS.** R23.
20. **A POSITIVE CONTROL IS AS OBLIGATORY AS A NEGATIVE ONE.** R24.
21. **A POPULATION NOTHING COMPARES IS NOT A CREDIT, IT IS AN UNAUDITED
    LIABILITY.** R25. **⚠️ AND ITS REMEDY IS TO MAKE IT COMPARABLE, NOT TO MAKE
    THE PORT REFUSE (corrected r32). Ask WHY nothing compares it first.**
22. **⚠️ R26 — A REFUSAL IS A CONTROL-FLOW CLAIM, AND IT BELONGS TO WHOEVER WROTE
    THE `exit`.** **NEW, round 32.** When the authority "refuses", establish
    whether the ORIGINAL declined to answer or whether OUR DRIVER threw its answer
    away. `MakeTable` had already filled `Output`; nine rounds of measurement
    rested on a `Halt(0)` three lines later. **Read the call site of every message
    the harness treats as fatal and check the flag it actually SETS — not the
    words in the string.** **Four of four help codes examined so far have been
    non-fatal in the original. Audit the rest (§3b item 8).**

---

## 5. Known traps — each produced a confident, wrong finding

- **⚠️ AN ORACLE THAT "REFUSES" MAY HAVE BUILT THE ANSWER AND DISCARDED IT.** R26.
  §65. **The single most expensive error in the project.**
- **⚠️ A MESSAGE'S WORDING IS NOT ITS CONTROL FLOW.** *"Internal error … Please
  contact Ones & Zeros"* reads fatal and is a bare statement; *"Please note …"*
  reads benign and is also a bare statement. **Read the `exit`, not the string.**
  Round 31 filed the opposite lesson ("an internal error message can be a
  substantive finding") from the same line and stopped one step too early.
- **⚠️ AN INSTRUMENT'S ERROR CAN FLATTER THE PORT.** Every prior harness defect
  cost the port credit; this one bought it 690 free cases per 160 seeds.
- **⚠️ A CONTROL CORPUS DRAWN FROM THE WHOLE POPULATION CONTAINS THE SUBJECT.**
  Round 32's first negative control scored the probe's own domain as failures.
  **Partition on the property the change keys on, and PRINT the excluded count.**
- **⚠️ A POPULATION OUTSIDE EVERY DENOMINATOR IS UNVERIFIED BY CONSTRUCTION.** R25.
- **⚠️ "DOES THE PORT'S OWN ANSWER LOOK SANE" IS NOT "DOES IT MATCH".** Round 31
  measured the first and reported it as the second. 91% "fine" → 19% agreeing.
  **And its 9% counterpart — "the port ships a loan that never pays off, to
  \$323,361" — was DOS's behaviour too, within ~1%.**
- **⚠️ AN ACCOUNTING IDENTITY LOOKS EXACTLY LIKE A MECHANISM SIGNATURE.**
  `dInt == dPaid` in 54 of 54 reads as *"only the interest differs"* and is simply
  what two schedules that both retire the same loan must do. **Round 32 wrote it
  into three documents and a commit message before checking the final balances.**
  **Before quoting an n-of-n agreement as evidence, ask what n-of-n would look
  like if NOTHING were true.**
- **⚠️ TWO BOUNDS CAN USE TWO CONVENTIONS.** CAUTION 4. One-sided `2.99573/N`
  vs two-sided `3.688879/N` — a 0.002 percentage-point gap that straddles the
  target bar.
- **⚠️ A ROUTINE CAN READ ONE BYTE PAST ITS ARGUMENT.** §65's other subclass.
- **⚠️ A BY-VALUE SHORTSTRING PARAMETER CARRIES ONLY `length+1` BYTES.**
- **⚠️ A DECLARED SENTINEL READ THROUGH A SIGNED FIELD LOOKS LIKE GARBAGE.**
  `unkbyte = -88`; `errorbyte = -99`.
- **⚠️ A PASCAL `{ }` COMMENT DOES NOT NEST.** Pasting `end; {RepayFancyLoan}`
  into a comment closes it early. Round 32 lost a build to this.
- **⚠️ A FIX THAT CHANGES NOTHING AND A MECHANISM THAT IS WRONG LOOK THE SAME.** R24.
- **⚠️ `go test` CACHES ACROSS AN ORACLE-ONLY CHANGE.** Note #22.
- **⚠️ `goamort` DOES NOT IMPLEMENT `norate`/`noamt`.** Note #24 — 17 of 95 in
  both r31 and r32.
- **⚠️ DOS'S REPRESENTATION CEILING IS A DATE: 26 AUGUST 2091.** §69.
- **⚠️ A CASE THAT FITS PERFECTLY MAY BE FITTING BY ACCIDENT.** R23.
- **A HUNDRED ROWS EXACTLY RIGHT WHILE THE SUMMARY SCALAR IS WRONG.** §68.
- **⚠️ AN ORACLE'S RAWBITS KEY CAN BE PARSED BACK OUT OF PRINTED TEXT.** r30.
- **A ROUTINE FAITHFUL TO THE ORIGINAL, REACHED BY A CALLER THAT IS NOT.** §59,
  §66, §67, §68 — the dominant class in this port, **and the prime suspect for
  the 475.**
- **A PORT-ONLY RECONSTRUCTION OF A DOS GLOBAL IS A DEFECT SITE** (§66) — **and
  correcting TWO reconstructions of the same fact DOUBLE-CORRECTS.** R21.
- **THE PORT EMULATES DOS's `Output=nil` TERM PROBE WITH A DISPLAY WALK.**
- **COUNTING ROWS CANNOT SEE A ROW THAT CHANGED KIND.**
- **AN INSTRUMENT NAMED IN A PLAN MAY NOT WIRE THE BRANCH YOU NEED. GREP BEFORE
  BUILDING.** **A PLAN'S NAMED FIX MAY NOT EXIST.**
- **THERE ARE TWO ENGINES.** `dosPortCanHandle` routes **in-advance, R78, daily**,
  exact×non-360 and REPLACE-mode extras to the PIECEWISE engine. **Check which
  engine your case uses BEFORE you edit.**
- **A SILENT `RecordError` IS A REFUSAL WITH NO MESSAGE.** **`lines 0` with no
  message is a refusal.**
- **`pkill -f` KILLS THE TOOL'S OWN SHELL.** Kill by PID.
- **GIT ON THE SSK MOUNT CANNOT UNLINK `index.lock` OR `HEAD.lock`.** #20/#21.
- **`device_bash` AND `device_stage_files` DISAGREE ABOUT THE DRIVE'S PATH.** #23.
- **`AddPeriod` IS NOT INVERTIBLE AT `peryr=24`.** §67.
- **A DANGLING SYMLINK LISTS EXACTLY LIKE A FILE — AND THE LINKS ARE ABSOLUTE TO
  A PREVIOUS SESSION'S MOUNT PATH.** §1 FIX 1.
- **A TARBALL BUILT FROM A FILE-ONLY MEMBER LIST LOSES ITS EMPTY DIRECTORIES.**
- **A MISSING FIXTURE LOOKS EXACTLY LIKE A PORT DEFECT.** §1 FIX 3.
- **A PRISTINE TREE WITHOUT `dos_source` CANNOT BUILD A PRE ORACLE.**
- **A RE-STAGE RETURNS NEW METADATA WITH OLD CONTENT.** Note #18.
- **A DEVICE-TOOL FAILURE MODE CAN GO AWAY.** **THE CONTAINER CLOCK DRIFTS.**
- **A TIMING FIGURE CAN BE 3.5× WRONG AND SHRINK THE ROUND'S AMBITION.**
- **AN IMPROVEMENT ON THE SAME SAMPLE IS NOT AUTOMATICALLY SIGNIFICANT.** R18.
- **A MECHANISM THAT FITS ONLY ITS POSITIVES IS NOT CONFIRMED.** R19.
- **A LANDED FIX IS NOT A MEASURED ONE** — and **an UNGATED fix is not a landed one.**
- **AN EMITTED RAWBITS KEY THAT NOTHING READS IS NOT COVERAGE.**
- **TWO SOLVERS THAT AGREE STEP FOR STEP CAN STILL RETURN DIFFERENT ANSWERS** (R17).
- **A TRACE FLAG CAN BELONG TO THE OTHER ENGINE.** `DPTRACE=1` is the PORT's.
- **`paired_regression.sh` deletes its work dir** — `KEEPWORK`, or `run_arm.sh`.
- **TWO ROW SETS ARE NOT COMPARABLE BY INDEX.** Defect #15. `GOAMORT_ALLROWS=1`.
- **`amort_oracle … rows` TRUNCATES A ROW DATE WITH A SINGLE-DIGIT DAY.** Defect
  #14. **Read `dumpraw` for dates.**
- **AN `r78` TOKEN DOES NOT MEAN DOS RAN ITS r78 BRANCH.** §68.
- **A TERMINAL BUCKET THAT ENDS IN `continue` IS AN UNMADE ASSERTION.** R16.
- **A STRATIFICATION LABEL IS A COVERAGE CLAIM — AND SO IS AN ERA LABEL, AND SO
  IS A BUCKET NAME.**
- **TWO COUNTERS OVER "THE SAME" POPULATION CAN DISAGREE.** Note #17.
- **A "COVERED ELSEWHERE" NOTE IS A COVERAGE CLAIM.** R15.
- **A VERDICT THAT MOVES WITH THE SAMPLE SIZE IS NOT A MEASUREMENT.** R14.
- **A PROBE THAT CANNOT REACH THE CASE REPORTS CLEAN.**
- **A TIE IS NOT A WIN**, and **A HALF-CENT TIE IS NOT A DIVERGENCE.**
- **THE ORACLE'S `row` FIGURES ARE DOS'S RENDERED CENTS.**
- **THE ORACLE'S `prin` IS THE PRINCIPAL PAID; THE PORT'S `Principal` IS THE
  BALANCE REMAINING.**
- **DOS's `CN` TRACE LINES CARRY YEARS OFFSET BY 1900.**
- **`cmd/goamort`'s PAYMENT ECHO IS A HEURISTIC.** Never score it.
- **RULE 7: NEVER CHANGE DEFAULT HARNESS OUTPUT.**
- **A KILLED BINARY REPORTS NOTHING, AND NOTHING LOOKS LIKE SUCCESS.**
- **A REGRESSION TEST MUST BE SEEN TO FAIL.**
- **GO BUFFERS `t.Logf` UNTIL THE TEST RETURNS.** Use stderr.
- **A SKIP IS NOT A PASS.** R12. **A TOLERANCE IS PART OF THE INSTRUMENT.** #10.
- **AN INSTRUMENT MAY PRINT ONLY WHAT IT HAS READ.** R13.
- **AN AMBIGUOUS OR ABSENT ORACLE RESPONSE MUST NOT BE SCORED EITHER WAY.** §60.
- **DOS PRINTS A TRAILING ALL-ZERO ROW ON SOME SCREENS.**
- **A DISPLAYED ROW CAN DIVERGE WHILE EVERY COMPUTED VALUE AGREES.** §59, §63.
- **BIT-EQUALITY IS THE WRONG ASSERTION FOR A SOLVER; SIGN BALANCE IS RIGHT.**
- **TWO SOLVERS THAT BOTH "CONVERGE" CAN LAND ON DIFFERENT ROOTS.** §61.
- **A HARNESS-COMPUTED DATE WILL MANUFACTURE A FRONTIER.** R2.
- **THE PORT CARRIES TWO CALENDARS.** They disagree at 2100 (§62).
- **DOS's `daterec` year is a BYTE based at 1900** — max 2155, and it WRAPS (§55)
  — **but `MDY`'s 70,000-day ceiling bites first (§69).**
- **DOS's 365 basis is 365.25 DAYS.**
- **Quantize every oracle argument.** **FPC's `:0:6` double-rounds.**
- **DOS's `Iterate` accepts `bestp < 0.005` OR `bestp <= 2e-8 × loan amount`.**
- **A Go-only pre-solve must never GATE a DOS decision.** §57.
- **`find -newer` cannot tell you what a session changed.** md5 manifest.
- **`~` is `/root` for `Bash` but `/home/claude` for `Write`.**
- **`mkdir -p` a log dir BEFORE the `nohup` that redirects into it.**
- **A SCRIPT WRITTEN TO `/tmp` IS LOST WITH THE CONTAINER.** Analysis scripts go
  in the repo.
- **`refdata.json` is NOT an oracle.**

---

## 6. Instrument inventory

| tool | what it does |
|---|---|
| `legacy/oracle/*_oracle` → **`/tmp/oraclebuild/`** | The three real DOS engines. The only authority. **Build all three, every round.** **⚠️ AND IT IS A SUSPECT — r30, r31, r32.** |
| **`PERSENSE_ORACLE=<path>`** | **Re-points the WHOLE harness, `run_arm.sh` included. How a PAIRED ORACLE measurement is run (note #25).** |
| **`testplan/harness/audit_sec65_messagebox_probe.py`** | **NEW r32. Runs each repro through TWO oracle binaries and the port: MATCH / DIVERGE / STILL-REFUSES, with a `--negative` corpus gate that partitions out the probe's own domain.** |
| **`internal/finance/amortization/zzsec65_oracle_advisory_test.go`** | **NEW r32. The ORACLE's regression gate: advisory yields a table, a real refusal still refuses (positive control), an answered screen is byte-unchanged (negative control).** |
| `scripts/build_trace_oracle.sh` | An INSTRUMENTED COPY of a DOS unit writing a trace to STDERR while stdout stays byte-identical. Modes `itr`/`cn`/`ra`/`apr`/`aprv`/`ovf`/`msf`; **`-mode aprbits` is §3b item 5.** |
| `PERSENSE_ORACLE_RAWBITS=1` | Raw float64 bits. **PV `rate` and mortgage `howmuch` CONSUMED (r30). `apr`/`apr1`/`apr2`/`cross` are TEXT-DERIVED.** |
| `amort_oracle … dumpraw` | DOS's whole rendered line. The ONLY reliable source of a row's DATE. **The cheapest probe in the project.** |
| `amort_oracle … adjdump` | Each `adj[i]`'s date, solved rate/amount + statuses. **goamort does NOT implement it.** |
| `cmd/goamort … bdump` · `GOAMORT_ROWDATES=1` · `GOAMORT_ALLROWS=1` · `rows` | Balloon grid + `lastdate`/`nperiods` · rows with dates · settlement rows too. **⚠️ goamort does NOT implement `norate`/`noamt` (note #24).** |
| `DPTRACE=1` · `DPTRACETERMROWS=1` · `DPTRACESEGROWS=1` · `DPTRACESEG=1` | The PORT's secant · unforced Newton terminal rows · segment-RATE rows · segment-payment bracket sweep (R17). |
| `testplan/harness/audit_sec65_advisory.py` | r31. **⚠️ ITS QUESTION WAS THE WRONG ONE** — it asks whether the PORT's schedule terminates, not whether it MATCHES DOS. Superseded by the probe above; keep it for the residual measurement only. |
| `testplan/harness/audit_skip_overread.py` | r31. §65's `noterm` subclass: PRE vs POST oracle vs goamort. `--selfcheck` is the STANDING form. |
| `testplan/harness/attribute_seven.py` | The Phase 2 per-row differential AND, with `--assert`, the regression gate for all seven. Needs `/tmp/goamort`. |
| **`testplan/harness/era_split_arm.py`** | **The IN-SCOPE COMPARED-CASE rate — the headline denominator. Pools SEVERAL arm dirs. §3b item 3 adds a 2091-2099 bucket.** |
| `testplan/harness/run_pvmtg_arm.sh` / `analyze_pvmtg_arm.py` | The PV and MORTGAGE seed-loopers and pooling analyzer. |
| `FZ5CASEDUMP=1` · `PERSENSE_FUZZ_FLAKEDUMP=1` | Every generated case to stderr · per-case repros. **FZ5CASEDUMP is how r32's control corpus was built.** |
| `scripts/actuarial_oracle.py` | Third-party actuarial oracle. Needs `scripts/`. |
| `internal/finance/amortization/screen.go` | R1: the ONE place a blank cell is solved and judged. |
| `zzbits_fidelity_test.go` ×3 | **FORWARD** bit differentials. |
| `amortization/zzbits_backward_test.go` / `_long_` | Backward bit differentials for `norate` / `noamt`. |
| `presentvalue/` and `mortgage/zzbits_backward_test.go` | The PV RATE and mortgage BALLOON-AMOUNT solves at bit level. `PERSENSE_BITS_N=`. |
| `amortization/zzsec68_termusap_test.go` | §68's gate: the defect plus THREE negative controls. |
| `zzadj_segment_horizon_test.go` · `zzsec62_feb_grid_test.go` · `zzsec59_ceiling_test.go` | §53 / §62 / §59 standing guards. |
| `zzplain_differential_test.go` + `run_plain_arm.sh` / `analyze_plain_arm.py` | The plain surface (`PERSENSE_PLAINDIFF=1`); ranges 21000-21039 / 21200-21239 / 33000-33039 at `PLAIN_N=1200`. **~19 min for all three. ⚠️ the analyzer takes ONE dir (note #26).** |
| **`testplan/harness/run_arm.sh` / `analyze_arm.py`** | **fuzzer5 standing ranges 50100-50139 / 44000-44039 / 44200-44239 + growth 50140-50179 at `FUZZ_N=400`. `analyze_arm.py` = SIGNAL INSTANCES; pair with `era_split_arm.py` for CASES.** |
| `dos_pv_fuzzer5_test.go` / `dos_mtg_fuzzer5_test.go` | The PV and mortgage headline instruments. **⚠️ THEIR SAMPLE SPACE HAS NEVER BEEN AUDITED — §3b item 4.** |
| `zzprepay_exhaust_test.go` · `zzroundtrip_test.go` | Prepayment termination · inverse gate (blind to §54). |
| `zzsamplespace_test.go` · `zztacktolerance_test.go` | Envelope manifest · R10 both directions. **Amortization only.** |
| `zzpayoff_sweep_test.go` · `long_horizon_sweep.py` | Randomized payoff differential (monthly only) · long-horizon forward differential. |
| `testplan/harness/paired_regression.sh` | FIXED/STILL/NEW gate. **~20-25 min for 40 seeds — start it FIRST.** |
| `testplan/harness/check_skips.sh` / `skip_allowlist.txt` | R12. **32/32 at rounds 27, 29, 30, 31 AND 32. Accepts `SUITE_LOG=`.** |

---

## 7. Backlog

**The top of the backlog IS §3b, in §3b's order.** Items 9-15 of §3b are the tail.
Nate's standing decisions: **actuarial is secondary**; **§54 is
document-and-defer**; **the harness policy is to be implemented**; **the exit
criterion is RATIFIED at 1 in 400 as a MILESTONE toward 99.99%** (**and it is now
NOT MET on the stacked surface**); **comparison beyond 2099 is NOT REQUIRED
(client, 2026-08-03)**; and **§3a as amended by decisions 3a.4 (WITHDRAWN r32),
3a.5 and 3a.6**.

Not otherwise scheduled: sweep timeout vs refusal key (R8b); the R2 tail; envelope
widening; round-trip v3; sub-monthly with options; the actuarial DOS oracle
(deferred).

**Not on the list by decision:** giving `types.DateRec` raw y/m/d fields.

---

## 8. History ledger

| round / date | what it established |
|---|---|
| 2026-07-28 → Round 15 | `crmath.go`; §47 Julian ceiling; §48 COLA; §49 bit differential; §50 pre-pass; §52, §53, §55, §56 closed; round-trip STANDING GATE; §57 closed. |
| Round 16/16b/16c · 08-02 | §58 closed; `harness_policy.md`; R1-R8; defect #8; the sample-space audit. |
| Round 17 · 08-02 | Defect #9. §54 QUANTIFIED. |
| Round 18/18b/18c · 08-02 | Defects #10, #11; five tolerances audited; backward-solver bit harness — R10/R11; §59 opened; payoff differential — §60; R12. |
| Round 19 · 08-03 | **§59 CLOSED**; **R13**. |
| Rounds 20+21 · 08-03 | Post-2100 backward solves CLEAN. **R14**; **§61**; **§62 CLOSED**; §54 RE-PRICED; **R15**; first randomized plain differential. |
| Round 22 · 08-03 | **Ledger buckets AUDITED → R16.** **§65 opened.** **ERA LABEL found WRONG.** **§63**, **§64**. |
| Round 23 · 08-04 | The route to 99.99% COMPUTED. Three standing decisions. Phase 2 STARTED; defect #14. |
| Round 24 · 08-04 | **PHASE 2 ANSWERED — 7 of 7 attributed.** **§66 opened**, **§67 opened.** Defect #15. |
| Round 25 · 08-04 | **§66's AO6 arm ROOT-CAUSED AND FIXED.** **R17.** |
| Round 26 · 08-04 | **ERA SPLIT MEASURED**; **R18**; note #17. **§67 ROOT-CAUSED and FIXED**; **R19**. |
| Round 27 · 08-04 | §67's RATE BOOKED. **c3's CALLER CLEARED → R20.** FULL SUITE GREEN. |
| Round 28 · 08-04 | **§66 CLOSED — BOTH ARMS.** → **R21**. ⚠️ THE SUITE DID NOT RUN. Note #18. |
| Round 29 · 08-04 | SUITE GREEN + `check_skips` 32/32. **§61 DISPROVEN; §68 OPENED and ROOT-CAUSED → R22.** PV and mortgage RE-MEASURED, 0 each. |
| Round 30 · 08-04 | **§68 FIXED, GATED AND BOOKED.** A fourth arm took the denominator to 34,353 with zero events. **PV/MORTGAGE BACKWARD BIT HARNESSES LANDED.** **R23.** Note #20. |
| Round 31 · 08-04 | **§65's `noterm` subclass ROOT-CAUSED — IT WAS THE ORACLE** (`MonthSetFromString` reads one byte past its by-value `str15`). `PadSkipMonths` lands. Three arms re-run paired, 8 → 1. **§69 OPENED** (day 70,000 = 26 Aug 2091). **R24, R25.** §65's advisory subclass measured **with the wrong question** → decision 3a.4. Notes #21-#24. |
| **Round 32 · 08-04** | **(a) 🚨 §65's ADVISORY SUBCLASS WAS AN ORACLE-DRIVER ARTIFACT.** AMORTOP.pas:1233 is a BARE `MessageBox`; `MakeTable` had filled `Output`; `amort_oracle.pas:1104` discarded it. The FOURTH bare-statement help code corrected in `legacy/oracle/Globals.pas`. **(b) 🚨 BOOKED PAIRED, FOUR ARMS, 160 SEEDS: in-scope COMPARED 34,412 → 35,000, in-scope HARD **0 → 475** = 1 in 74. THE EXIT CRITERION IS MISSED BY 5.4×.** **(c) ATTRIBUTION IS ONE CLASS** — `HARD:divergent_class` 13 → 502, every other SIG flat — **with ONE SIGNATURE**: rows equal 49/54, port's interest lower 53/54, separating mid-schedule at an adjustment. **(c2) ⚠️ SAME-DAY RETRACTION — the `dInt==dPaid` 54/54 figure is NOT evidence (50 of 54 retire on both sides, so the identity is arithmetic), AND on the 4 that do not retire DOS leaves essentially the SAME RESIDUAL as the port, which kills round 31's \$323,361 "the port ships a loan that never pays off" outright.** **(d) DECISION 3a.4 WITHDRAWN (Nate).** **(e) §65's in-scope bucket 690 → 0; the remaining in-scope answered case is §69.** **(f) CONTROLS: negative 145/145 byte-identical, positive 10 real refusals survive, and the shipped binary is md5-identical to the validated probe.** **(g) PLAIN RE-MEASURED — 108,778 / 13 signals / all `tie=true`, IDENTICAL to r22; rule 11's probation DISCHARGED.** **(h) CAUTION 6 RESOLVED — two-sided vs one-sided Poisson; CAUTION 4 rewritten, CAUTION 7 opened.** **(i) `zzsec65_oracle_advisory_test.go` seen to FAIL both directions; `audit_sec65_messagebox_probe.py` lands.** **(j) R26.** **(k) Suite GREEN `-count=1`, 0 cached; `check_skips` 32/32. `docs/history/START_HERE.md` snapshot refreshed.** |

---

## 9. Where the canonical documents live

| what | live home | snapshot in repo? |
|---|---|---|
| **This file** | project: `claude/START_HERE.md` | **yes** — `docs/history/START_HERE.md` — **REFRESHED r32** |
| **Round 32 — the §65 reversal and the rebased rate** | **`claude/round32_sec65_was_the_oracle_driver_rate_rebased_2026-08-04.md`** | no |
| **Convergence assessment** | `claude/convergence_assessment_2026-08-04_round31.md` — **⚠️ HEADLINE SUPERSEDED by round 32; methodology stands** | no |
| — round 22's | `claude/convergence_assessment_2026-08-03_round22.md` — SUPERSEDED | no |
| **Standing decisions** | **§3a of this file** | no |
| **⚠️ THE WITHDRAWAL NOTICE — read it before either doc below** | **`claude/WITHDRAWN_NOTICE_sec65_refuse_decision_2026-08-04.md`** | no |
| — earlier | `claude/decisions_2026-08-04_sec69_boundary_and_sec65_fallback.md` (**§2 WITHDRAWN IN FULL; §1 and §3 STAND; its "the bound stands" box is WRONG**); `claude/decisions_2026-08-03_exit_criterion_and_sec54_sequencing.md`; `claude/decisions_2026-08-03b_client_2099_boundary.md` | no |
| §65's r31 options analysis | `claude/sec65_advisory_options_analysis_2026-08-04.md` — **⚠️ SUPERSEDED: its question was the wrong one** | no |
| Round write-ups | project: `claude/round*.md` | round 14 onward |
| The route to 99.99% | `claude/plan_to_9999_2026-08-03.md` — **its 300,000 figure is RETIRED; its Poisson convention is the ANSWER to CAUTION 6** | no |
| §65 / §69 full accounts | `docs/discrepancies.md` §65, §69 | **yes, committed** |
| Client-facing | `claude/convergence_note_client_2026-08-02.md` — **⚠️ CARRIES A SUPERSEDED STACKED FIGURE (CAUTION 7)**; `docs/why_dos_believes_feb_29_2100.md` | the latter is in the repo |
| Porting rules · testing · harness · audit · discrepancies | repo: `CLAUDE.md`, `docs/testing_policy.md`, `docs/harness_policy.md`, `docs/fuzzer_sample_space_audit_2026-08-02.md`, `docs/discrepancies.md` | — |

### The snapshot rule — DECIDED by Nate, 2026-08-01

**The project copy stays the single live document; a snapshot lands in
`docs/history/` at COMMIT TIME only.** Do not snapshot mid-round.
