# START HERE — Per%Sense port: continuity, work plan, and standing rules

**Last updated: 2026-08-10, ROUND 46.** Read this file and nothing else to start.

**⭐⭐⭐ (a) ITEM 24 IS COMMITTED. `testplan/harness/uidiff/`** — a generated,
DOS-anchored differential that compares **what the user sees** (painted totals,
painted schedule cells, the painted APR input, the painted grid) against
`amort_oracle`. Three tiers, the three round-45b traps as **executable
assertions with positive controls**, **6 of 6 mutants killed**, tolerances
declared and printed on every run. **It drives the CONTAINER'S OWN CHROMIUM via
playwright, so it needs no human's browser attached.**
**🚨🚨 (b) THE ROUND'S HEADLINE ATTRIBUTION WAS WITHDRAWN BEFORE PUBLICATION BY
ITS OWN ADVERSARIAL AUDIT.** It attributed a new refusal-parity class to the
amount-only adjustment at **9 of 9 / 0 of 117**. The audit found `gen.js` never
recomputed `basePayment` after the `peryr` option changed the period, so on
annual screens the generated adjustment set the new installment to a **monthly**
payment — up to **10.9× too small** — **manufacturing the non-convergence it then
attributed.** After the fix: **3 of 83 vs 1 of 117. ATTRIBUTION WITHDRAWN.**
R32's auditor is **THIRTEEN FOR THIRTEEN**.
**⭐ (c) A REAL NEW CLASS SURVIVES: REFUSAL PARITY. 4 in 139 SCORED stacked
screens.** DOS's `Iterate` declines to converge; the page paints a full schedule
anyway — **three of the four with no warning of any kind.** OPEN, UNATTRIBUTED.
**⭐ (d) TWO REACHABLE DEFECTS IN COMMITTED PRIOR ART.** `run_amz.js:41` inverts
`balloonIncl → plusreg` against `handlers.go:972` (measured **$1,600.09** apart on
AMZ-C-57); and **`pts=` must be emitted EVEN AT ZERO** or DOS never runs the APR
solver and the whole APR axis silently compares nothing.
**⭐ (e) ITEM 0-MTG IS DONE** — 14 findings, 3 HIGH, **filed not fixed**. It found
**NO NF-1c analogue** (all seven kicker crossings correctly paired) and **no §79
paint-index instance**. Two expected findings that did not materialise.
**🚨 (f) ITEM 0j MEASURED AT LAST — and the round's own claim about it REFUTED
too.** Its line numbers have been wrong since round 37. Correct: **`fancybisect.go:248`
and `:306`**, not 195/253.
**🚨 (g) THE HARNESS WAS THE SUSPECT SEVEN TIMES BEFORE THE ENGINE WAS ONCE, plus
four more the audit caught. Rule 12, seventeenth round.**
**🚨 (h) CONVERGENCE SCORE IS STILL 2. Nothing this round moved it.**
**⚠️ (i) NOT REACHED: item 3 (`reached`) FOUR rounds · item 4 (0-USAGE) THREE
rounds · the APR class itself.**
**✅ (j) THE DECISION QUEUE IS EMPTY. NOTHING IS WAITING ON NATE.**

> **Nate:** to continue in a fresh session, say *"Read `claude/START_HERE.md` and
> continue."* **⚠️ AND STAY AT THE MACHINE FOR THE FIRST MINUTE — the folder-grant
> dialog needs a click and it times out.**
>
> **⚠️ AND REBUILD/RESTART THE SERVER BEFORE ANY UI TESTING.** `cmd/persense/main.go:23`
> is `//go:embed static`, so **the running binary serves an embedded SNAPSHOT of
> `index.html`**. Round 46 verified it by fetching `/` and md5-comparing to the
> file on disk — do that, it is one command and it is exact.

---

## 🚨 0. THE COMMIT-STATE RULE. READ THIS FIRST.

**This file NO LONGER ASSERTS A HEAD HASH.** R45 fired on it in rounds 42, 43 and
44 for a structural reason: **the `docs/history/START_HERE.md` snapshot commit
necessarily POSTDATES the file that describes it.**

| what | value |
|---|---|
| **LAST CODE COMMIT** | **`789a7d1`** — *"round 46: commit the DOS-anchored UI differential as a standing instrument (item 24) — and its own adversarial audit refuted the round's headline attribution"* |
| previous code commits | `8ef6fce` (r45b §89) · `bfc90f2` (r45 §88) · `cee4943` (r44) · `2b03814` (r43) |
| **HEAD** | **MEASURE IT.** `git rev-parse --short HEAD`. It is the last code commit **or one doc-only snapshot commit on top of it. BOTH ARE CORRECT.** |
| **the tarball set** | **`r46*`**, rebuilt at `789a7d1` — see FIX 4 |

**→ FIX 5's MANIFEST DIFF IS EXPECTED TO SHOW EXACTLY ONE DIFFERING FILE,
`docs/history/START_HERE.md`, AND NOTHING ELSE.** **🚨 ANY OTHER differing file —
above all a `.go` or `index.html` — IS REAL DRIFT: stage the drive's copy before
touching it (note #58).**

**MEASURE EVERY COUNT IN THIS FILE. DO NOT QUOTE THEM.**

---

## 1. Getting a working container (~15 minutes)

### 🚨 FIX 0 — CHECK THE BRIDGE FIRST. FIVE FAILURE MODES.

| symptom | what still works | round |
|---|---|---|
| everything works | — | 29-38, 41, 42, 44, 45, **46** |
| `device_bash` returns "Workspace unavailable" | grant / list / stage / commit | 39 |
| **`remote-devices` exposes ONE tool** | **nothing** | 40, 45b (mid-session, ~10 min, RECOVERED) |
| grant REFUSES the path | everything except the drive | 41 |
| grant dialog **TIMES OUT** | everything except the drive | 43, 44 |

**THE TEST, first tool call:** `mcp__remote-devices__get_device_info`.

- **"not connected to the bridge" →** `RefreshMcpTools` on `remote-devices`, retry
  over ~10 minutes, then **STOP**. **⚠️ ASKING NATE TO CONNECT THE FOLDER DOES NOT
  FIX THIS.** ✅ r45b: it dropped mid-session and came back on its own. Deliver
  finished files with `SendUserFile` while you wait.
- **🚨 GRANT REFUSED →** list a home directory first. Note #49.
- **🚨 GRANT TIMED OUT → NATE IS NOT AT THE MACHINE (#57).** Message him, retry
  **once**, then stop.

**⚠️ A BRIDGE-DEAD SESSION IS NOT A WASTED SESSION** — the record audit needs no
tree and has twice corrected the top items of its own work plan.

### ⚠️ THE DRIVE IS AT `/Volumes/SSK/persense/PerSense-Web`

### 🌐 THE BROWSER — AND ROUND 46 CHANGED THE ANSWER

**🚨 DO NOT DEPEND ON NATE'S CHROME. USE THE CONTAINER'S OWN CHROMIUM.**
Playwright is preinstalled (`PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers`); **never
run `playwright install`.** Round 46 started with `list_connected_browsers` → `[]`
and built the whole instrument headlessly anyway. That is the right design: a
committed harness must not need a human's browser attached.

`mcp__claude-in-chrome__*` remains available and is **separate from the device
bridge** — in 45b the bridge died while Chrome stayed up. Check both. Carried
browser facts, all still true:

- **🚨 NEVER CALL `clearAmortization()` — IT CALLS `confirm()`**, and a modal
  dialog freezes the whole automation channel. Write your own reset.
- **🚨 CLEAR `#amz-total-paid` / `#amz-total-int` textContent AND EMPTY THE
  SCHEDULE TBODY IN YOUR RESET.** A blocked calc otherwise leaves the previous
  session's totals on screen to be scored.
- **⚠️ THE PAGE RESTORES THE LAST SESSION'S WORKSHEET.**
- **⚠️ FEED DATES AS MM/DD/YYYY.** `parseDate` accepts ISO but `dateValidity`
  REJECTS it — that inconsistency IS §89. **Verified still true in r46.**
- **⚠️ `parseRate` RETURNS PERCENT SPACE.** `parseRate('7.0000%') === 7`, not
  0.07. So does the `amz-points` field. **Both cost round 46 a measurement pass.**
- Set `.value` directly; `getAmzInput` reads the DOM fresh at calc time, so no
  events are needed and no auto-recalc storm is triggered.

### ✅ FIX 2b — `device_bash`

**⚠️ Sees the drive at its SESSION MOUNT** — `ls -d /sessions/*/mnt/*`.
**⚠️ CANNOT DELETE.** `rm` and `git rm` fail; **`git rm --cached` and `mv` work.**
**🚨 Rename `index.lock` AND `HEAD.lock` IN PLACE before and after every git write.**
The unlink warnings are noise — **read `git log`, not the warnings.**

**🚨 NOTE #54** — `setsid nohup … & disown` from `device_bash` dies anyway.
**🚨 NOTE #55 FIRED AGAIN IN r46, AND IN THE CONTAINER'S OWN `Bash` THIS TIME.**
`cd X && nohup … &` leaves the *next* command in the ORIGINAL cwd — the container
tool prints "Shell cwd was reset". **Put the `cd` inside the backgrounded
`bash -c`, and use ABSOLUTE PATHS after any `&`.**

### ✅ FIX 2d — ROUND-TRIP md5. **r46: 8 of 8** (3 tarballs in, plus the r46
harness tarball and the rebuilt src tarball, both directions).
**⚠️ PROVES DRIVE == CONTAINER, NOT THAT EITHER == HEAD.** Only FIX 5 does that.

### 🚨 FIX 2c — `tar x` ON THE MOUNT CANNOT OVERWRITE. New files only.

### 🚨 FIX 5 — RUN THE MANIFEST DIFF EVERY ROUND. Note #30.

1. **drive, ONE call:** `git ls-files -z | xargs -0 md5sum > _to_delete/rNN/dm.txt 2> dm.err`
   ⚠️ `xargs` exits **123** (42 dangling symlinks) — check `wc -l dm.err` == 42.
2. **container:** `cd /root/pw && find . -type f | sed 's|^\./||' | sort | while IFS= read -r f; do md5sum "$f"; done > /tmp/container_manifest.txt`
3. `SendUserFile` it, then `device_commit_files` it to the drive (**#31: push, don't pull**)
4. drive: join on PATH; print differing rows, `-v1`, and `-v2` (must be 0)

**🚨 PARSE WITH `substr($0,1,32)` / `substr($0,35)`** — seven tracked PDFs have
SPACES in their names.
**🚨 CHECK COMPLETENESS FIRST:** `wc -l dm.txt` + `wc -l dm.err` == `git ls-files | wc -l`.
A truncated manifest joins to `0 differing`.

**✅ ROUND 46: 1 differing (the expected snapshot), 0 `.go` absent, 811 absent, 0
untracked-on-container.** Rounds 36-38, 41: 0 differing. Round 42: 1/811. Round
45: 1. Rounds 39, 40, 43: could not run.
Three non-engine files are absent by design: `code_docs/_build/render.py`,
`legacy/testharness/test_dos_reference.sh`, `testplan/build_pdf.py`.
**The 811 are overwhelmingly `legacy/src_documented` (380) and `legacy/src` (322)
— the original distribution. That is the standing shape, not drift.**

### ⚠️ FIX 1 — `legacy/oracle/units` IS A SYMLINK FARM
```
mkdir -p /sessions/funny-tender-pascal/mnt
ln -sfn /root/pw /sessions/funny-tender-pascal/mnt/PerSense-Web
```
Verbatim in rounds 27 through 46.

### ⚠️ FIX 3 — THE BOOTSTRAP IS THREE TARBALLS
⚠️ Fixture roots are `legacy/reference-output/` and `legacy/src/win_source/`.

### 🚨 FIX 4 — HOW TO REBUILD A TARBALL SET

**⚠️ DO NOT build the member list from the previous tarball's list plus "the new
files."** Build it from the **CONTAINER'S OWN VERIFIED FILE SET**:

```
tar tzf <dos>.tar.gz | sed 's|^\./||;/^$/d' | sort -u > /tmp/dos_m.txt
tar tzf <fix>.tar.gz | sed 's|^\./||;/^$/d' | sort -u > /tmp/fix_m.txt
cd /root/pw
{ find . -type f; find . -type l; } | sed 's|^\./||' | sort -u > /tmp/all_now.txt
comm -23 /tmp/all_now.txt <(cat /tmp/dos_m.txt /tmp/fix_m.txt | sort -u) \
  | grep -vE '^(_to_delete/|\.git/)' > /tmp/src_m.txt
tar czf /tmp/rNNsrc.tar.gz --no-recursion -T /tmp/src_m.txt
```
**⚠️ INCLUDE `find . -type l`** (42 symlinks) **AND EXCLUDE `_to_delete/`**.
**THEN extract to a scratch dir, `go build ./...`, READ THE EXIT CODE, FOREGROUND.**

**🚨 NOTE #58 — A SET THAT PREDATES HEAD CAN SILENTLY REVERT A COMMIT.**
**→ BEFORE PUSHING ANY FILE THE SET PREDATES, STAGE THE DRIVE'S COPY AND BUILD ON
TOP OF IT**, then `grep -c '^## §'` for every section immediately before the
push. ✅ r45, r45b and r46 all did this. **r46 also REBUILT THE SET rather than
leaving one that predates its own commit** — do the same.

**⚠️ TARBALL SET ON THE DRIVE — `r46*`, built at the LAST CODE COMMIT `789a7d1`,
foreground scratch-build verified (build exit 0, `go vet` 0, 436 `.go`, 34 `.pas`
in dos_source, 42 symlinks, both fixture roots, all 10 `uidiff` files present):**
`_to_delete/r46src.tar.gz` **`3e0bfac671080984186dea4b2d454e43`**,
`_to_delete/r46dos.tar.gz` **`099986e1791a50ee80fc20438485f048`**,
`_to_delete/r46fix.tar.gz` **`666bd710e7f9e1d5fbc41b3ddf4d605b`**.
*(dos and fix are byte-identical to the r36/r41/r42/r44/r45/r45b set; only src moves.)*
Also on the drive: `_to_delete/r46/r46uidiff.tar.gz` **`f7a8993b9a8ea90ff329e9c6a305371f`**
(the harness alone, if you only need that).
**🚨 MD5 THE TARBALL BEFORE EXTRACTING AND COMPARE TO THIS LINE — note #56: this
file once recorded an md5 that was simply wrong.**

**✅ COUNTS AT THE LAST CODE COMMIT — MEASURE, DO NOT QUOTE (§0).**
```
git ls-files '*.go' | wc -l        -> 436   <- THE GATE. Tracked only.
git ls-files | wc -l               -> 1723
git ls-files | grep -cE '\.go\.[0-9]+$' -> 0
```
History: r41 426 · r43 430 · r44 433 · r45 436 · r45b 436 · **r46 436 (no Go file
changed; +10 tracked files, all `testplan/harness/uidiff/`).**
**USE `git ls-files`, NOT `find`.** ⚠️ `git ls-files '*.pas'` is **193** across all
trees; START_HERE's "34" means `legacy/src/dos_source` only. Both are correct;
say which.

### The recipe

1. **FIX 0 — test the bridge**, then `device_request_folder_access`.
   **AND `list_connected_browsers` SEPARATELY** — they are independent.
2. **MEASURE HEAD (§0).** md5 the three `r46*` tarballs against FIX 4's line,
   then extract into `/root/pw`; recreate the symlink path.
3. **RUN FIX 5.** Expect **exactly one differing file** (§0).
4. **`pip install actuarialmath ipython scipy --break-system-packages`.**
   **Do NOT `apt install fpc`.** ⚠️ Without this the two actuarial third-party
   tests fail **identically on a pristine tree** — environmental, not a regression.
5. Oracles: `legacy/oracle/build_linux.sh`, then `TARGET=pv_oracle`,
   `TARGET=mtg_oracle`. **Each prints a smoke test — READ IT**
   (`payment 888.4879` / `pv 9231.163464` / `monthly 1066.683053`).
   `go build -o /tmp/goamort ./cmd/goamort`;
   `go test -c -o /tmp/amorttest ./internal/finance/amortization`.
   **🚨 FLAGS ARE `-dV_3 -dSCROLLS -dPVLX`, NOT `-dACTU` — AND `-dACTU` IS
   UNBUILDABLE (§2 authority note). SAY "ABSENT", NEVER "PENDING".**
   **🚨 A PROBE-TREE ORACLE BUILD SILENTLY REPLACES THE REAL BINARY (§82) —
   REDIRECT `OUT`.** ⚠️ `build.log` is 0 bytes on success; that is normal.
6. Verify: 42/42 symlinks, 34 `.pas` in dos_source, both fixture roots,
   `.go` == **436**.
7. **Extract a PRISTINE copy to `/tmp/pristine` BEFORE any edit** and
   `cp /tmp/oraclebuild/amort_oracle /tmp/pre_amort_oracle` (#25).
8. **THE SERVER, BEFORE ANY UI WORK:** `go build -o /tmp/persense ./cmd/persense`,
   start it, then **`curl -s http://localhost:8080/ | md5sum` and compare to
   `md5sum cmd/persense/static/index.html`. They must be IDENTICAL.** That is
   exact and takes one second; a marker grep is weaker.

**⚠️ PROBE PORT:** `cp -a /root/pw /tmp/proberepo`, edit the COPY.
**🚨 A PROBE TREE VALIDATES THE TEST, NEVER THE FIX (R44).**
**🚨 R51: AN INERT MUTANT ON A HEALTHY POPULATION IS A STATEMENT ABOUT THE MUTANT.**
**🚨 COUNT YOUR NEEDLE FIRST.** ⚠️ And note the direction: when a *missing* needle
reads as SURVIVED (as in r46's JS mutation harness) a full kill sweep is
self-verifying; when it reads as KILLED it is not.

**⚠️ `~` IS `/root` FOR `Bash`, BUT `Write` RESOLVES `/home/claude`.**
**⚠️ `cd` DOES NOT PERSIST ACROSS LINES. 🚨 NOTE #55 — see FIX 2b.**
**⚠️ `pkill -f`, `| xargs kill` AND THE `while read` FORM KILL THE TOOL'S OWN SHELL (#45).**
**⚠️ THE TOOL'S 2-MINUTE CEILING IS REAL. Background anything over ~90 s** — and
`nohup … &` from a `Bash` call **dies when that call returns**; use
`setsid nohup … < /dev/null &`.
**🚨 `PERSENSE_FUZZ_N`, NOT `FUZZ_N`, IS WHAT THE TEST READS (#52).**
**⚠️ `TestDOSFuzzer5AllAdvancedOptions` IS OPT-IN: `PERSENSE_FUZZ=1`** — and with
it the suite is **RED BY DESIGN** (#40, #46).
**⚠️ `go test` CACHES. Use `-count=1`. ⚠️ CHECK `nproc`** — rounds 13-46 got **2**.

### TIMING (2 cores)

| what | wall clock |
|---|---|
| bootstrap + manifest diff + 3 oracles + binaries | ~15 min |
| **full `PERSENSE_REQUIRE_ORACLE=1 go test ./... -count=1`** | **~5 min** |
| `check_skips.sh` | **~4 min — it runs the gated suite** |
| `run_arm.sh` 10 seeds N=400 JOBS=2, ONE tree | ~4 min |
| `run_pvmtg_arm.sh SURFACE=pv`, 4 seeds | **~10 s — absurdly cheap** |
| one mutation cycle | ~5 s |
| **an adversarial audit (subagent)** | **~15-25 min — paid in r35-r46** |
| **`uidiff` full run, 258 screens (2 oracle invocations each + double-submit)** | **~2 min** |

**Plan for breadth. THE CONTAINER CAN RESTART MID-SESSION. Sync early.**

---

## 2. Current state

Round 46: **`claude/round46_the_ui_differential_committed_and_the_round_audited_out_its_own_headline_2026-08-10.md`**
and **`claude/round46_mortgage_screen_readback_audit_0mtg_2026-08-10.md`**.
Round 45b: `claude/round45b_the_ui_differential_and_the_fix_that_broke_the_screen_2026-08-10.md`.
Live convergence assessment: **`claude/convergence_assessment_2026-08-10_round46.md`**.
Still-live: `claude/restatement_of_published_numbers_2026-08-08.md` (**⭐ read
before quoting**), `claude/ROUND40_AUDIT_…`,
**`claude/convergence_note_client_2026-08-08_DRAFT.md` (still DRAFT, still
unreviewed, still does not mention §89)**.
Full accounts: **`docs/discrepancies.md` §71 (CLOSED), §35A, §72-§74 (OPEN),
NF-6 (CLOSED), §75-§77/§79 (FIXED r42), §78 (PERMANENTLY UNADJUDICABLE),
§80/§81 (FILED), §82-§89.**

**⚠️ Before quoting ANY number, read the TWELVE cautions below.**

### 🚨 THE AUTHORITY NOTE — READ BEFORE QUOTING ANY PV FIGURE

`build_linux.sh:114` builds every driver with
`-Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX`. **`ACTU` IS NOT DEFINED,
AND IT CANNOT BE** (§82): the `ACTUARY` unit source is MISSING, `uses ACTUARY` is
commented out, and an `-dACTU` build fails with 11 errors in two families.
**🚨 THE ERRORS STOP AT `pvltable.pas`, AND THAT MUST NOT BE READ AS "`PRESVALU`
COMPILES UNDER ACTU"** — it calls `PodValue`/`XPODValue` at `:689`, `:712`,
`:790`, `:842-849`. **✅ 3a.15(B): DISCLOSED PERMANENTLY, NOT CLOSED.**

### ⭐ THE STANDING AMORTIZATION BASELINE (r45; NOT re-measured in 45b or r46)

Seeds 50100-50109, `PERSENSE_FUZZ_N=400`, `horizon` key, no engine filter.

| population | compared | **Q4 HARD** | **Q7 HARD (pre)** | **Q7 HARD (post-NF-1)** |
|---|---|---|---|---|
| **in scope ≤2099** | **2,086** | **23 — 1 in 91** | **30 — 1 in 70** | **✅ 25 — 1 in 83** |
| out of scope >2099 | 125 | 2 | 3 | 3 |
| pooled | 2,211 | 25 | 33 | 28 |

**🚨 HOW TO READ IT.** `adj_amount_missing` 5 → 0; every other class byte-identical.
**The Q4 column is the CONTROL and HELD at 23** — a **display-transport
correction, not engine convergence.** Bar miss 5.75× → 4.79×. **STILL MISSED.**

### ⭐ THE PER-SIGNAL BASELINE (pooled ≈2,208, `horizon`, no engine filter)

| signal | denominator | events | control |
|---|---|---|---|
| **Signal 5 — SOLVED** | 449 | 2 (1 in 224) | ⚠️ **TYPED stratum UNCONTROLLED — item 0r** |
| **Signal 6 — APR** | 1,856 | **20 (1 in 93)** | ✅ GATED r43 (§83) |
| **Signal 7 — adjustment rows** | 858 rows | **30 findings** | ✅ GATED r44 (§84) |
| engine `dosport` | 106 | 0 | ⚠️ **licenses nothing** |
| engine `piecewise` | 2,102 | 33 (1 in 64) | — |

**✅ QUOTE SIGNAL 6 IN BOTH HALVES:** *"20 in 1,856 = 1 in 93 (pooled, `horizon`,
no engine filter, tol 2e-6). The probe is a demonstrated GATE (§83). THE 20
THEMSELVES REMAIN UNATTRIBUTED (R27)."*

### THE CONTINGENCY TABLE — ROUND 38, NOT RE-MEASURED
| clause | cases | diverged | rate |
|---|---|---|---|
| `piecewise:in_advance_or_r78_or_daily` | 26,269 | 231 | 1 in 114 |
| `piecewise:exact_non360` | 2,779 | 71 | 1 in 39 |
| **`piecewise:replace_mode_with_extras`** | 1,388 | 62 | **🚨 1 in 22** |
| `piecewise:balloon_plus_ao6_or_ao7_adjustment` | 426 | 45 | **1 in 9** |
| `piecewise:adjustment_carries_amount_ao6` | 596 | 18 | 1 in 33 |
| `piecewise:disabled_or_not_fancy_or_backward` | 1,796 | 5 | 1 in 359 |
| **`dosport`** | 1,707 | **🚨 3** | **1 in 569** |
| **TOTAL** | **35,456** | **436** | **1 in 81** |

**⚠️ R29 — NOT ADDITIVE, NOT A WORK QUEUE. ⚠️ DENOMINATOR IS `eraCompared` (#17).
🚨 IT PREDATES THE NF-1 FIX. RE-MEASURE BEFORE QUOTING.**

### THE `EMessage` DELTA — 3a.9 phase 1, MEASURED r38. DEFAULT NOT FLIPPED.
| surface | → COMPARED | DIVERGENT | rate |
|---|---|---|---|
| amortization, in scope | **+1,344** | **+115** | **~1 in 12** |
| **PV / MORTGAGE forward** | **0** | 0 | unchanged |

**⚠️ A ZERO ON PV/MORTGAGE IS A FINDING, NOT A CLEAN BILL.**

### THE THREE SCOPE KEYS — one function, `amortization.HorizonKeys()`

| key | what it is | who used it |
|---|---|---|
| `lastdate` | last REGULAR payment date | r35's §72 — **WRONG** |
| `horizon` | `max(last row, balloons, LastDate)` | the standing table |
| **`reached`** | `max(last row, balloons)` | **the ratified decision → item 3** |

**Ceiling family (seed 71, n=500), IN SCOPE:** `horizon` 3/288 all engines;
**`reached` 10/319 = 1 in 32.**

### 🚨 THE DEFECT LEDGER

| # | defect | state |
|---|---|---|
| **AO9-1..7 · R39-1..5** | prepay-solve family · modal-payment reconstruction | **FIXED (r39/39B/39C/39D)** |
| **✅ NF-6 · NF-1 · NF-1c · NF-2 · NF-2b** | year bound · the adjustment echo on one engine · the internal-space rate echo · off-grid date matching · **the r45 fix that shipped raw ISO into a field its own validator rejects** | **ALL CLOSED (r41/r45/r45b)** |
| **🚨 NF-1b** | 2 of 113 `dosport` rows echo ~2× DOS's amount on a final-position adjustment | **OPEN** |
| NF-3 / NF-4 / NF-5 | green-map restore · echo pairing by sort accident · refused responses carrying a payload | **OPEN — MED / LOW / LOW** |
| **🚨 APR-R** | residual APR class. **20 in 1,856 = 1 in 93** | **OPEN, UNATTRIBUTED (R27).** r45's only clean isolate has horizon **2133 — OUT OF SCOPE** |
| **🚨 USA∧PREPAY∧ADJ** | §89 §4. DOS total interest **20,528.59** vs port **8,798.24**; the port fits **−23.52%** where DOS gives **−20.795%**. Every option agrees alone and in every PAIR; only the triple diverges | **OPEN, UNATTRIBUTED. 🚨 UNGUARDED — no test, fixture or golden anywhere in `internal/` pins it.** ⚠️ **The round-46 brief said −20.51%; that figure is in NO repo file. `discrepancies.md:9153` says −20.795%.** |
| **🚨 REFUSAL PARITY — NEW r46** | DOS's `Iterate` declines to converge (`AMORTOP.pas:1489`) and the page paints a full schedule anyway. **4 in 139 SCORED stacked screens; three of the four show NO warning at all.** Cases `stacked-72/96/120/130`, seed 46046 | **OPEN, UNATTRIBUTED.** ⚠️ **The AO6 attribution was MADE AND WITHDRAWN IN-ROUND** — the generator manufactured it (see (b) above) |
| **🚨 item 0j — NEW MEASUREMENT r46** | `fancybisect.go:248` uses `math.Abs(accInit)` where `AMORTOP.pas:1489`'s `init` is **SIGNED**; `:306` uses `<` where Pascal's negation is `<=` | **OPEN. 🚨 LINE NUMBERS CORRECTED — R37's `195,253` ARE WRONG.** Measured: `accInit < 0` occurs **14 times in 600 fuzz cases** via `solveSegmentRate` (`:1895`, `:1912`, whose own header says it solves against a negative balance) — **REACHABLE, NOT YET DEMONSTRATED TO CHANGE AN OUTCOME.** Needs `\|init\| > 250,000`; largest seen 44,459 |
| **🚨 §74** | `ParseDMY` exits without writing its out-parameter → a malformed `payoff=` makes the ORACLE NONDETERMINISTIC | **OPEN (r41)** |
| **🚨 §78 · §80 / §81** | POD solve pre-empts every other backward solve (measured INERT) · PV screen-total warning, silent 0% rate stratum, nine further PV display gaps | **UNADJUDICABLE / FILED** |
| **🚨 FILED r45** | `amzUnkickerRate` omits DOS's `ReportedRate` wrapper (`INTSUTIL.pas:1646-1650` applies it on BOTH branches); the "unreachable through the REST surface" claim IS NOT SUPPORTED — `perYr:76`/`140` are accepted | **OPEN. 🚨 AND `uidiff` IS BLIND TO IT TWICE OVER — see §6** |
| **🚨 FILED r46 — THE MORTGAGE SCREEN** | 14 findings, 3 HIGH: the wire carries values but **not statuses**, so every computed **zero** paints blank · `apr,omitempty` drops an exact-zero APR and the consumer blanks the cell · **`/api/mortgage/compare` cannot carry Tax or the balloon and drops them silently — measured 215 basis points**, and `tryBalloonDates` is structurally unreachable from the shipped button | **FILED, NOT FIXED.** Separate document |

### The surface table

| surface | measured | unit | round | verdict |
|---|---|---|---|---|
| **🚨 STACKED, in scope, SEVEN questions** | **25 in 2,086** | cases | r45 | **1 in 83. Bar missed 4.79×** |
| — same cases, FOUR questions (CONTROL) | 23 in 2,086 | cases | r41 · r45 | 1 in 91 |
| — **faithful port (`dosport`)** | 3 in 1,707 · 0 in 106 | cases | r38/r41 | **1 in 569 — the zero was the generator** |
| — **EMessage delta, in scope** | ~115 in 1,344 | cases | r38 | **🚨 ~1 in 12 — worst measured** |
| — **product, ceiling family, `reached`** | 10 in 319 | screens | r36-r38 | **🚨 1 in 32** |
| **APR (Signal 6)** | **20 in 1,856** | APR verdicts | r41 · r45 | **1 in 93. GATED; the 20 UNATTRIBUTED** |
| **adjustment cells (Signal 7)** | **30 in 858 rows** | findings over ROWS | r45 | GATED r44 |
| **🚨 UI DIFFERENTIAL — DISPLAY SPACE, DOS-anchored, GENERATED** | **4 in 139 SCORED** *(plain 0/10, single 0/48, stacked 4 divergent + 135 agreed + **61 both-refuse**)* | **SCREENS** | **r46** | **the instrument is COMMITTED. 🚨 DENOMINATOR IS 139, NOT 200** |
| **PLAIN — 3 standing ranges** | 108,778 in scope, 0 arithmetic | cases | **r32 — ⚠️ FOURTEEN ROUNDS** | ≥99.99725% one-sided — **CAUTION 8b** |
| **PV — forward** | 29,917 worksheets / 5,095,860 lines | worksheets · lines | r29 | 0 — **⚠️ CARRIED ACROSS AN r42 PV ENGINE CHANGE · calendar stops 2088 · 🚨 NO ACTUARIAL AXIS, PERMANENTLY** |
| — backward RATE, BIT LEVEL | 1,500 / 1,499 bit-exact | bits | r30 … r45 | 0, no sign bias (p=1) |
| **MORTGAGE — forward** | 30,000 / 135,853 APR verdicts | cases · verdicts | r29 | 0 — **NO DATE AXIS. ✅ 360-ONLY *AND IMMATERIAL* (§87)** |
| — backward BALLOON, BIT LEVEL | 1,500 / 1,499 bit-exact | bits | r30 … r45 | 0 (p=1) |
| **Actuarial** | 663 + `TestDOSActuarialGolden` | checks | carried | 0 — 🚨 **NOT a DOS differential, and never will be** |
| **Payoff** | 70 golden + 428 randomized | cases | carried | **1 divergence (§60) — TWENTY-FOUR rounds** |

**⚠️ CAUTION 1 — UNITS.** CASE ≠ SIGNAL INSTANCES ≠ ROWS ≠ FINDINGS ≠ WORKSHEETS
≠ LINES ≠ APR VERDICTS ≠ BIT COMPARISONS ≠ **SCREENS**.
**⚠️ FIVE TOLERANCE FLOORS, NONE PINNED** — item 0e. **🚨 r45 SAW IT DECIDE AN
ANSWER: an APR case read "totals-green" while its totals differed by $4.05, under
a $27.97 floor.** ✅ **`uidiff` does NOT add a sixth: its tolerances are declared
in one place with provenance and PRINTED ON EVERY RUN.**

**⚠️ CAUTION 2 — POPULATION.** `dos_fuzzer5` SKIPS every plain loan; the plain
differential covers ONLY plain loans; PV and mortgage are two further surfaces;
`sec71_ceiling_arm` a fifth; **the UI differential a sixth. Name which.**

**⚠️ CAUTION 4 — BOUNDS.** One-sided `2.995732/N`; two-sided `3.688879/N`.
**NO HARNESS SCRIPT COMPUTES ANY BOUND.**

**⚠️ CAUTION 5 —** retired figures 29,891 / 5,034,725 / 20,754 / 136,270 must
never reappear as current.

**⚠️ CAUTION 7 —** every stacked figure before r32 measured a truncated population.

**⚠️ CAUTION 8 — A ZERO IS A STATEMENT ABOUT ITS GENERATOR.** `dosport` went 0 → 3
with no code change. **0 in 106 and 0 in 13 are evidence of nothing.**

**⚠️ CAUTION 8b — A ZERO CAN BE UNFALSIFIABLE BY ITS PREDICATE.** The plain
surface excludes every >2099 falsifier **by definition**, emits **no settings
token**, is a **round-32** number, and **has never compared an APR**.

**⚠️ CAUTION 9 —** a rate is a statement about its scope key, count, engine filter
and question set. **39C's per-option APR rates have NO denominators.**

**⚠️ CAUTION 10 — A GREEN GATE IS A STATEMENT ABOUT WHICH TESTS RAN.**
**ROUND 46's GATES: full suite `PERSENSE_REQUIRE_ORACLE=1` 12 ok / 0 fail, exit 0,
run AFTER the last edit, `PERSENSE_FUZZ` UNSET; `check_skips` 32/32; 6 of 6
mutants killed on a probe copy; round-trip md5 8 of 8; FIX 5 exactly 1 differing;
three oracle smoke tests read; served html md5-identical to disk.
NOT RUN: every randomized fuzz arm, `paired_regression`, the PV/mortgage arms,
both bit harnesses — no `.go`, no `index.html` and no legacy file was touched.**

**🚨 CAUTION 11 — A SURFACE CAN BE FULLY GENERATED AND WHOLLY UNREAD.** R40.

**🚨 CAUTION 12 — A SURFACE CAN HAVE NO AUTHORITY AT ALL.** `pv_oracle` carries no
`-dACTU` and **cannot**. Where a flag is UNBUILDABLE **say so**.

### The statistical position

- **Plain, in scope: 0 in 108,778 → ≥99.99725% one-sided** — bounded by CAUTION
  8b, and a **round-32** number.
- **🚨 Stacked, in scope, SEVEN questions: 1 in 83 against a 1-in-400 bar.**
- **🚨 The faithful port is no longer at zero on any adequately-sized generator.**
- **🚨 The EMessage population diverges at ~1 in 12.**
- **⭐ NEW: the DISPLAY surface, DOS-anchored and generated, is 4 in 139 scored
  screens — and all four are one class, REFUSAL PARITY.**

**🚨 THE MOST DANGEROUS SENTENCE THE PROJECT OWNS.** *"Plain: 0 in 108,778 →
≥99.99725%"* printed beside *"stacked is 1 in 83"* reads as a surface split of one
product. **It is not.** It is a **round-32** measurement; its population has
**every advanced option off**; its in-scope predicate excludes the best-known
hazard **by construction**; it is a **different question set** and **has never
compared an APR**. **Never print them as a pair without this paragraph.**

### Harness policy scoreboard

R1-R53 as recorded in §4. **NEW THIS ROUND:**

| rule | status |
|---|---|
| **🚨 R54 — A PASS THAT COMPARED NOTHING IS NOT A PASS; NAME THE DENOMINATOR** | **NEW (r46).** 62 of 188 stacked "passes" were `both-refuse` rows where DOS declined and so did the page — **no number was compared** — and they were being laundered into the headline. `na` was dead code because the comparator always adds at least one check. **Report `agreed / divergent / compared-nothing` separately, and quote the rate over `agreed + divergent`.** |
| **🚨 R55 — A GUARD THAT SKIPS THE EMPTY CASE CANNOT FIND THE BUG WHOSE SYMPTOM IS EMPTINESS** | **NEW (r46).** `frontend_diff_sweep_test.go:1980-1982` is `if cell == "" { continue }`, and a blank cell **is** the mortgage screen's finding 1. R49's sibling, at the level of the **assertion** rather than the **sample**. |
| **🚨 R56 — APPLY A RULE IN EVERY MODULE THAT NEEDS IT, NOT JUST THE ONE WHERE YOU LEARNED IT** | **NEW (r46), fired THREE TIMES in one round.** "An advisory is not a refusal" was fixed in the comparator and not the double-submit gate (44 false results), then in the gate and not its verdict. "Join on the date, not the index" (R52) was fixed for adjustment cells and reintroduced **forty lines away** in the sibling module. |

---

## 3a. STANDING DECISIONS (Nate)

1. ~~§65's internal-error subclass~~ **SUPERSEDED.**
2. **§63, the terminating-balloon final row: MATCH DOS.** ⚠️ Worth ZERO against the rate.
3. **The headline is PER IN-SCOPE CASE, SPLIT BY SURFACE.** **State the
   convention, GENERATOR, SCOPE KEY, ENGINE FILTER, GATE STATE, QUESTION SET and
   ORACLE BUILD FLAGS. 🚨 AND NEVER PRINT PLAIN BESIDE STACKED WITHOUT §2's "most
   dangerous sentence" paragraph.**
4. ~~§65's advisory subclass~~ **WITHDRAWN.**
5. **§69: move the in-scope boundary to DOS's real ceiling (26 Aug 2091) and
   publish the 2091-2099 band as a named gap.** Item 10.
6. **The mortgage APR bit surface needs NO rule-7 call.**
7. **⚠️ OPEN FOR NATE — THE ROUTER IS A PRODUCT DECISION.** Item 8.
8. ~~§71 interim state~~ **RESOLVED.**
9. **✅ RESOLVED.** Lift `EMessage` in two phases. **PHASE 1 DONE (r38); PHASE 2 IS
   UNEXECUTED WORK — item 6.**
10. **✅ RESOLVED — FROZEN REGRESSION CORPUS (item 7).** ⚠️ **A REGRESSION GATE,
    NEVER A CONVERGENCE CLAIM.**

### ✅ THE DECISION QUEUE IS EMPTY. NOTHING IS WAITING ON NATE.

| # | THE ANSWER (Nate, 2026-08-09) | status |
|---|---|---|
| **3a.11** | **✅ ADOPT `reached`.** | **NOT EXECUTED — item 3, FOUR ROUNDS.** 🚨 PRICE: 3/288 → 10/319 = 1 in 32, and **every in-scope rate AND COUNT restates.** Same tree measured correctly — **not a regression (R14/R36).** |
| **3a.12** | **⏸️ MEASURE §73's IN-SCOPE REACH FIRST.** | **NOT DONE — item 4.** |
| **3a.13** | **⏸️ DO 13b FIRST.** | **✅ DONE r44 — PREMISE RETIRED (§87).** |
| **3a.14** | **✅ HOLD — DO NOT FLIP.** DOS ships REPLACE (`PEDATA.pas:68`); the web ships ADD. Item 23. | **⭐ MEASURED AT THE DISPLAY LAYER FOR THE FIRST TIME IN r46**, via `handlers.go:972` `PlusRegular: !req.BalloonIncludesRegular`. Unchanged. |
| **3a.15** | **✅ DISCLOSE THE PV ACTUARIAL GAP PERMANENTLY.** | **✅ COMPLETE — 9 of 9 (r45).** |

**⚠️ NONE OF THESE MOVED THE SCORE.** It is 2 because **dimension 1** is 2 and the
score is the MINIMUM. **The convergence work is the APR class and item 8.**

**⭐ ONE STANDING ASK, COSTING NOTHING: REQUEST `ACTUARY.pas` FROM THE CLIENT.**

---

## 3b. THE ROUND-47 WORK PLAN

**🚨 MANDATORY STEP 0 FOR EVERY ITEM:** *search `docs/` and the project docs for
prior art, and CITE it or state that nothing exists.* **✅ IT HAS PAID THREE
ROUNDS RUNNING** — r45 found NF-1c in a warning our own team wrote three days
before the defect shipped; **r46 found that the work plan's own claim ("the first
DOS-anchored UI differential the project has ever run") was FALSE — `run_amz.js`
and three siblings already existed. A FILENAME SEARCH HID THEM; GREP THE CONTENT.**

### 1. ⭐⭐⭐ **THE APR CLASS — THE ONLY LEVER THAT MOVES THE SCORE**
20 of the 25 remaining in-scope divergences. **It has now been the top unexecuted
item for three rounds; do it before building anything.**
- **⚠️ FIX ITEM 0e FIRST.** The count is decided by an unpinned floor.
- **Widen the search for an IN-SCOPE totals-identical APR case** — §88 §5's open
  measurement. That is a measurement, not a judgement call.
- **🚨 ATTRIBUTE §89 §4's `USA ∧ PREPAY ∧ ADJ` divergence.** Large ($11,730),
  reproducible, ablation already done, **and completely unguarded.** Quote DOS at
  **−20.795%** (`discrepancies.md:9153`), never −20.51%.
- **Read `engine.go:4574` against `AMORTOP.pas:1520-1535`** with `DPTRACEENGINE=1`
  on r45's clean isolate. **🚨 Its horizon is 2133 — OUT OF SCOPE. Say so every time.**

### 2. ⭐⭐⭐ **EXECUTE `reached` — DECIDED 2026-08-09, FOUR ROUNDS UNEXECUTED**
One line in `HorizonKeys()` plus a re-run. **Run BOTH keys in one session (R36),
publish under `reached`, footnote `horizon`. 🚨 PRE-DECLARE: 3/288 → 10/319 = 1 in
32, and EVERY in-scope rate AND COUNT restates — the same tree measured
correctly, NOT a regression. ✅ UNBLOCKS ITEMS 7 AND 8; item 8 is the largest
single measured lever (upper bound 63 of 436).**

### 3. ⭐⭐ **SETTLE ITEM 0j — IT NOW HAS A NAMED EXPERIMENT**
`fancybisect.go:248` (`math.Abs(accInit)`) and `:306` (`<` vs `<=`).
**Reachable — 14 negative `accInit` in 600 fuzz cases via `solveSegmentRate`.**
To flip a verdict you need `bestp ∈ (0.005, 2e-8·|init|]`, i.e. **`|init| >
250,000`**; the largest negative balance seen was 44,459. **Do the targeted search
over large negative segment balances.** If it flips, it is a candidate mechanism
for refusal parity, for §85 §5's "solver in a different basin", and for §89 §4.
**If it does not, close 0j and say so** — it has been carried since round 37.

### 4. ⭐⭐ **ATTRIBUTE REFUSAL PARITY (new r46)**
4 in 139. `stacked-72/96/120/130` at seed 46046; the repro lines are in
`testplan/harness/uidiff/uidiff_run_r46.log`. **🚨 THE AO6 ATTRIBUTION WAS MADE AND
WITHDRAWN IN-ROUND — do not re-make it without a generator you have checked.**
`stacked-130` carries **no adjustment at all**, so it is not solely an AO6 story.
**Three of the four paint a schedule with NO warning: that is the user-visible part.**

### 5. ⭐⭐ **0-USAGE — STILL UNSTARTED, THREE ROUNDS.** *Time-boxed: HALF A DAY, HARD STOP.*
Usage-weighted divergence rate; option probabilities set to a **NAMED, STATED
prior**. **If nobody can supply real frequencies, SAY SO — do not invent one
silently and do not call it "real-world".** **🚨 PRE-DECLARE that this moves the
number UP and is a POPULATION CHANGE, NOT AN IMPROVEMENT. PUBLISH BOTH FIGURES
SIDE BY SIDE WITH THE POPULATION ON EACH. ⚠️ IT IS WORTH ZERO AGAINST THE EXIT
CRITERION — score it as a client-communication instrument.**

### 6. ⭐ **THE THREE HIGH MORTGAGE FINDINGS (r46, filed not fixed)**
Statuses on the wire · `apr,omitempty` · **Compare APR dropping Tax and the
balloon (215 bp, measured)**. ⚠️ **And fix `frontend_diff_sweep_test.go:1980-1982`'s
`if cell == "" { continue }` first (R55), or the guard cannot see the fix land.**

### 0-REST. THE CARRIED INSTRUMENT WORK
**0c.** Harden `check_skips.sh` — key on the FULL `Parent/subtest` path; fix
`skip_allowlist.txt:19` ("30 entries" — it carries 32).
**0d.** Pin note #17; assert `sum(strata) == compared`. ⚠️ Both r41 splits total
2,208 against 2,211.
**0e. RECONCILE THE FIVE TOLERANCE FLOORS. 🚨 PROMOTED — it decides the APR count.**
**0f.** `dosport_entry.go:516` and `zzsec70_engine_route_test.go:21` say "eleven";
`dosPortRouteSet` emits **fourteen**.
**0h. ATTRIBUTE THE SEED-50152 HANG.**
**0n.** Stratify the per-engine table by MODE — `engHard[dosport]` can book
piecewise divergences, **flattering the port.**
**0o.** §74 — make `ParseDMY` write a sentinel on every exit path.
**0q.** Settle §84's dead disjunct. ⚠️ **DO NOT DELETE IT (R31).**
**0r. SIGNAL 5's TYPED STRATUM IS THE LAST UNCONTROLLED SIGNAL.** Dimension 3
does not reach 8 while it stands.
**0s.** `amzUnkickerRate` and `ReportedRate` — **and note `uidiff` is BLIND to it
twice over (§6), so do not read a clean UI run as evidence here.**
**0t.** **M11's mutant is still unwritten.**
**0v. NEW r46 — FIX `run_amz.js:41`** and the two `amz_cases.json` oracle arrays it
masks. **⚠️ Their titles say "REPLACE mode" and they are being compared in ADD mode.**
**0w. NEW r46 — TRIAGE THE AUDITOR'S OUT-OF-SCOPE OBSERVATION:** the opt-in fuzzer
at `PERSENSE_FUZZ_N=600` on a fresh seed gave **6 divergent classes** (worst
`dInt = 275,465.12` on `balloon1+prepay2+pts+skip|non`) and
`TestPayoffRandomizedSweep` reported **1 in-divergence**. **Never triaged.**

### 7-24, and the standing tail
Live: 6 (**EMessage phase 2 + the client note — it still does not mention §89**),
10 (**the 2091-2099 band — half an hour**), 11, 12, 14 (**close or park §60 —
TWENTY-FOUR rounds**), 21, 23.
**⭐ RE-RUN THE PLAIN ARM** — a **round-32** number, the oldest in §2, carried
across six rounds of engine change, **never compared an APR**, and it is the
designated client headline.
**⭐ RE-DERIVE THE PV FORWARD POPULATION AT HEAD — the arm costs ~10 s and has now
been skipped THREE rounds. ⚠️ Dimension 4 cannot reach 7 while this and the plain
zero are both carried.**

### The exit criterion — RATIFIED 2026-08-03, STATUS AT r46

> **PV and mortgage at zero, RE-MEASURED at exit, bit-verified backward as well as
> forward.** — 🚨 **THE SAMPLE-SPACE CLAUSE HAS A PERMANENT HOLE:** PV's calendar
> stops 2088, mortgage has no dates, and the PV life-contingency and
> Payment-on-Death paths have **NO ORACLE AND CANNOT HAVE ONE.**
> **⚠️ THE PV FORWARD FIGURE IS CARRIED ACROSS AN r42 PV ENGINE CHANGE.**
>
> **Amortization: HARD rate below 1 in 400**, ≤10% of HARD signals unattributed.
> — **🚨 NOT MET. 1 in 83 — missed by 4.79×.**
> — **🚨 THE 10% CLAUSE STILL CANNOT BE EVALUATED** — the HARD SIGNAL INSTANCE
> total is published nowhere.
> — **PLAIN: MET (r32)** — bounded by CAUTION 8b, **fourteen** rounds old.
> — **✅ GENERATOR-COVERAGE writable**, and must name the PV 2088 ceiling, the
> mortgage no-date-axis result, the plain envelope, that widening the generator
> destroyed the faithful port's zero, that DOS REFUSES ANY ADJUSTMENT under
> in-advance interest (r45b), **and that `uidiff`'s generator excludes
> `perYr ∈ {24,26,52}`, off-grid dates, day > 28 and every backward solve (r46).**
> — **🚨 SCOPE-KEY CLAUSE.** No exit claim quotes an in-scope rate **or COUNT**
> without its key **and engine filter**.
> — **🚨 GATE-DISCLOSURE CLAUSE** — no audit finding counts as closed until its fix
> has been **seen to move a number (R36), AT THE CONSUMER (R42)**.
> — **🚨 A PRODUCT-SURFACE CLAUSE.** Five display defects on the amortization
> screen (r39), four on the PV screen (r42). ✅ The amortization set is closed.
> **🚨 THE MORTGAGE SET IS NOW OPEN WITH THREE HIGH ITEMS (r46).**
> — **🚨 AN AUTHORITY CLAUSE.** §74's nondeterminism; R47's absent oracle.
> — **🚨 INSTRUMENT-CONTROL CLAUSE (R49 + R51).** ✅ Signals 6 and 7 meet it.
> **🚨 Signal 5's TYPED stratum does NOT — item 0r.**
> — **🚨 R53: A DISPLAY-LAYER CLAIM IS NOT CLOSED UNTIL THE PAINTED VALUE HAS BEEN
> THROUGH THE VALIDATOR THAT WILL SEE IT NEXT.**
> — **🚨 NEW (R54): NO RATE MAY BE QUOTED OVER A DENOMINATOR THAT INCLUDES ROWS
> WHERE NOTHING WAS COMPARED.**
>
> Any exit claim travels with the sample-space scope statements AND the TWELVE
> cautions in §2.

---

## 4. Standing rules (non-negotiable)

1. **Sync per fix.** Verify by ROUND-TRIP md5 (FIX 2d) and the BOOTSTRAP with FIX 5.
2. **A green suite is not validation.** Build all three oracles, run the GATED
   suite, run `check_skips.sh`. **⚠️ RUN IT AFTER THE LAST EDIT.**
3. **Every fix ships a regression test, verified BOTH directions, IN FACT** — and
   **ON EVERY ENGINE THAT CAN ANSWER THE SCREEN (M11). ⚠️ MUTATION-TEST IT.**
4. **Engine changes run the paired regression AND randomized coverage. NEW must be 0.**
5. **Audit the source before fixing.** Fuzzing locates; only reading explains.
6. **Goldens carry provenance; a rule-7 claim carries a CHECKED-IN CORPUS.**
7. **`legacy/src/**` is untouchable. Never disturb a driver's DEFAULT STDOUT.**
8. **Ask what the generator CANNOT produce, what the instrument cannot JUDGE, what
   it SKIPS, WHICH OUTPUTS IT NEVER READS BACK (R40), and WHAT THE ORACLE WAS NOT
   COMPILED WITH AND COULD NOT BE (R47).** **21 for 21.**
9. **Never quote a co-occurrence without its base rate, a rate without its
   POPULATION, a ZERO without its GENERATOR (R31) or ORACLE BUILD (R47), an
   IN-SCOPE RATE without its SCOPE KEY (R34), a COUNT without its ENGINE FILTER, a
   GATE RESULT without its opt-ins (R37), or ANY RATE WITHOUT ITS QUESTION SET (R41).**
   **🚨 A BASE RATE MUST BE TAKEN OVER THE STRATUM WHERE BOTH EVENTS CAN OCCUR.**
   **🚨 NORMALISE THE JOIN KEY AND CHECK THE EMITTER'S CARDINALITY (R52).**
   **🚨 AND THE DENOMINATOR MUST EXCLUDE ROWS WHERE NOTHING WAS COMPARED (R54).**
10. **Internal-consistency tests never drive a behavior change.**
11. **Do not carry a claim forward without verifying it.** Retired: the commit
    state; the PV/mortgage figures 29,891 / 5,034,725 / 20,754 / 136,270; "the
    suite is green"; "the PV actuarial gap is a BUILD-FLAG CHOICE"; "the `.go`
    gate is 430"; "SIGNAL 7 IS BLIND TO NF-1"; "the residual APR class is
    attributed to the piecewise adjustment-rate solve"; "NF-2 is closed";
    "the round-45 paint guard verifies the consumer";
    **and NEW in r46: "the project has never run a DOS-anchored UI differential"
    (`run_amz.js` and three siblings already existed), "refusal parity is
    attributable to the amount-only adjustment" (the generator manufactured it),
    "item 0j's `math.Abs` limb is inert at every reachable call site" (it is
    reached 14 times in 600 cases), and "0j is at `fancybisect.go:195,253"`.**
    **⚠️ APPLIES TO THIS FILE'S OWN ALARMS (R45), TO THE WORK PLAN'S OWN FRAMING,
    TO AUDIT FINDINGS, TO THE ROUND'S OWN HEADLINE, AND TO YOUR OWN GUARDS.**
    **🚨 AND ITS CONVERSE — CHECK WHETHER THE TREE ALREADY ANSWERS THE QUESTION
    BEFORE CALLING AN ANSWER NEW. GREP CONTENT, NOT FILENAMES.**
12. **The harness is a suspect before the engine is.** **r30 through r46 ALL ended
    with the harness, the instrument, the generator, the document, the ORACLE, the
    DISPLAY LAYER, or OUR OWN FIX at fault.** **🚨 r46: SEVEN of its own harness
    bugs before the engine was implicated once, plus FOUR MORE the audit caught.**
13. **A SESSION THAT CANNOT GET A CONTAINER MUST SAY SO EARLY** — FIX 0.
14. **AN IMPROVEMENT ON THE SAME SAMPLE CARRIES ITS SIGNIFICANCE (R18); ONE AFTER
    A GENERATOR OR INSTRUMENT CHANGE IS NOT AN IMPROVEMENT AT ALL (R36/R41).**
    **⚠️ r46 lived this twice: fixing `basePayment` changed the population, so its
    before/after divergence counts are NOT comparable and were not compared.**
15-51. **R19-R53 as recorded in round 45b's edition**, unchanged, plus:
52. **🚨 R54 — A PASS THAT COMPARED NOTHING IS NOT A PASS.** Report
    `agreed / divergent / compared-nothing` separately; quote the rate over
    `agreed + divergent`. `na` that can never fire is not a category, it is a leak.
53. **🚨 R55 — A GUARD THAT SKIPS THE EMPTY CASE CANNOT FIND THE BUG WHOSE SYMPTOM
    IS EMPTINESS.** R49's sibling, at the assertion rather than the sample.
54. **🚨 R56 — APPLY A RULE IN EVERY MODULE THAT NEEDS IT.** Fired three times in
    r46. When you fix a class of error, **grep for the class**, not the line.

---

## 5. Known traps — each produced a confident, wrong finding

**NEW IN ROUND 46:**

- **🚨🚨 YOUR GENERATOR CAN MANUFACTURE THE DEFECT YOU THEN ATTRIBUTE.** A stale
  `basePayment` made annual screens carry a monthly installment, DOS declined to
  converge, and the round nearly published `9 of 9 / 0 of 117`.
- **🚨🚨 A "PASS" CAN MEAN "NOTHING WAS COMPARED."** 62 of 188. → R54.
- **🚨 A WORK PLAN'S OWN FRAMING IS A CLAIM, AND IT CAN BE FALSE.** "The first
  DOS-anchored UI differential the project has ever run" — four already existed.
  **A filename search hid them.**
- **🚨 A GUARD NEEDLE CAN MISS THE ONLY REACHABLE FORM OF A TOKEN.**
  `payoff` in a `Set` never matches `payoff=1.1.2030`.
- **🚨 A FIELD THAT LOOKS LIKE A FRACTION CAN BE PERCENT SPACE.** `amz-points` and
  `parseRate` both are. Two separate 100× errors in one round.
- **🚨 DOS SIGNALS "NO TABLE" WITH A `-1` SENTINEL AND NO ERROR LINE.**
  `payment 0.0000 interest -1.00 paid -1.00`, `nlines 0`.
- **🚨 `pts=` MUST BE EMITTED EVEN AT ZERO** or DOS never runs the APR solver.
- **🚨 A SETTING CAN MAP TO ITS ORACLE FLAG *INVERTED*.** `handlers.go:972` is
  `PlusRegular: !req.BalloonIncludesRegular`.
- **🚨 AN OUTBOUND COMPARISON CAN BE THE IDENTITY ON THE PORT'S OWN TRANSFORM.**
  Painted `= internal/kicker`; comparing `painted × kicker` to DOS's internal rate
  can never fail on the transform. **It masked the FILED `ReportedRate` item.**
- **🚨 A "RETIRES TO ZERO" CHECK CAN BE UNABLE TO FAIL DOWNWARD** — `index.html:4102`
  clamps the painted balance with `Math.max(0, ...)`.
- **🚨 DOS'S `payment` LINE IS NOT THE FIRST PAINTED PAYMENT AND NOT THE MODAL ONE.**
  It is the **first segment's regular payment**, and three separate things break
  each naive choice: an anomalous first row (in-advance / points / moratorium /
  target), a post-adjustment modal, and a prepay series that dominates a short
  schedule.

**CARRIED — the load-bearing set:**

- **🚨🚨 EVERY AUTOMATED GATE THIS PROJECT OWNS CAN BE GREEN ON A TREE WITH A REAL
  ARITHMETIC DEFECT IN IT.** R32.
- **🚨🚨 THE PROJECT CAN ALREADY KNOW THE THING YOU ARE ABOUT TO "DISCOVER."**
- **🚨🚨 A NEGATIVE CONTROL CAN BE INERT BECAUSE THE SAMPLE CANNOT EXPRESS THE
  DIFFERENCE (R49) — AND A MUTANT BECAUSE IT NEVER REACHES (R51).**
- **🚨🚨 A MEASURED ZERO CAN DIE ON FIRST CONTACT WITH A WIDER GENERATOR.** R31.
- **🚨🚨 AN AUDIT FINDING'S FIX CAN SHIP BROKEN AND THE WRITE-UP WILL SAY IT LANDED.**
- **🚨🚨 AN AUDITOR'S MOST CONFIDENT FINDING CAN BE REFUTED BY MEASUREMENT — AND SO
  CAN YOUR OWN.** r42 twice; r44 twice; r45 once; **r46 twice, both the round's.**
- **🚨🚨 YOUR FIX CAN CREATE A WORSE DEFECT THAN THE ONE IT CLOSED, AND YOUR OWN
  GUARD CAN PASS THROUGHOUT.** §89.
- **🚨🚨 TWO FUNCTIONS IN ONE FILE CAN DISAGREE ABOUT THE SAME FORMAT.**
  `parseDate` ACCEPTS ISO; `dateValidity` REJECTS it.
- **🚨🚨 A 100% DIVERGENCE RATE IS A MAPPING BUG, NOT A DEFECT.** And the tell is
  the SHAPE: payment, total-interest and total-paid diverging on EXACTLY the same set.
- **🚨🚨 AN ORACLE TOKEN CAN CHANGE THE WHOLE OUTPUT MODE.** `apr adjdump` returns
  `apr 0.000000 status 0`.
- **🚨 A `go:embed` SERVER SERVES A SNAPSHOT.** md5 the served page against disk.
- **🚨 THE PAGE RESTORES THE LAST SESSION'S WORKSHEET**, and a blocked calc leaves
  the PREVIOUS totals on screen.
- **🚨 `clearAmortization()` CALLS `confirm()`** — a modal freezes the channel.
- **🚨 A CONTROL CAN BE STRUCTURALLY UNABLE TO EXPRESS THE BUG YOU HAVE.** R49.
  **✅ ADDRESSED for the UI surface by `uidiff`'s one-option-at-a-time tier.**
- **🚨🚨 A GENERATOR CAN BE PERFECT AND THE INSTRUMENT STILL BLIND.** R40.
- **🚨🚨 TWO INSTRUMENTS CAN DISAGREE BY 27× AND BOTH BE RIGHT OVER THEIR OWN
  GENERATORS.** §86.
- **🚨 A SELF-READING GUARD IS UNCONDITIONALLY TRUE (R50). ASSERT ACROSS FILES.**
- **🚨 A SUITE CAN BE GREEN BECAUSE THE TESTS THAT REPORT THE OPEN DEFECTS ARE
  OPT-IN AND WERE NOT OPTED IN.** R37.
- **🚨 A UI TEST CAN BE SELF-REFERENTIAL** — `frontend_diff_sweep_test.go:447`,
  **and it is so BY DESIGN AND BY DOCUMENTATION** (`frontend_differential_harness.md:15`).
- **🚨 A SOLVED VALUE PAINTED INTO A GRID BECOMES A HARD INPUT ON THE NEXT SUBMIT.** R53.
- **🚨 FIVE TOLERANCE FLOORS, NONE PINNED** — r45 caught one deciding an answer.
- **🚨 `types.DateRec` CANNOT REPRESENT 29 FEBRUARY 2100.** §73.
- **🚨 A GENERATOR'S CALENDAR CAN STOP BEFORE EVERY KNOWN HAZARD.** PV: 2088.
- **🚨 A ZERO ON A SMALL DENOMINATOR IS NOT A CLEAN BILL.**
- **🚨 A PROBE-TREE ORACLE BUILD SILENTLY REPLACES THE REAL BINARY.** §82.
- **🚨 `omitempty` MAKES AN EXACT ZERO INDISTINGUISHABLE FROM "NOT COMPUTED".** §81.
  **✅ FOUND AGAIN ON THE MORTGAGE SCREEN IN r46, AND DEMONSTRABLY REACHABLE.**
- **⚠️ THERE ARE TWO ENGINES, AND THE ONE YOU ARE READING IS PROBABLY NOT THE ONE
  THAT ANSWERED.** ✅ `AmortResult.EngineUsed` — **ASSERT IT INSIDE THE TEST.**
- **⚠️ A PASCAL `with` CHANGES WHAT A BARE IDENTIFIER MEANS**, `{ }` does not nest,
  and **a `{$ifdef}` can compile out the very behaviour you are porting.**
- **⚠️ A TARBALL'S md5 MATCHING A DOCUMENT PROVES IT IS INTACT, NOT CURRENT.**
- **⚠️ `tar x` ON THE SSK MOUNT SILENTLY LANDS ONLY THE NEW FILES.**
- **⚠️ THE ORACLE'S `prin` IS PRINCIPAL PAID; THE PORT'S `Principal` IS BALANCE
  REMAINING. ⚠️ `cmd/goamort`'s PAYMENT ECHO IS A HEURISTIC** — and **`goamort`
  REJECTS `bdate=`/`adjdmy=`/`predmy=`/`mordmy=` entirely.**
- **A ROUTINE FAITHFUL TO THE ORIGINAL, REACHED BY A CALLER THAT IS NOT.**
- **A REGRESSION TEST MUST BE SEEN TO FAIL.** ✅ r41, r42, r44, r45, r45b, **r46**.
- **A SKIP IS NOT A PASS (R12). `refdata.json` is NOT an oracle.**

---

## 6. Instrument inventory

| tool | what it does |
|---|---|
| `legacy/oracle/*_oracle` → `/tmp/oraclebuild/` | The three real DOS engines. The only authority. **🚨 A SUSPECT NINE TIMES OVER** — §74's nondeterminism, the shared-`OUT` overwrite (§82), no ACTUARIAL paths and none possible (R47), `apr`/`bdump` being separate OUTPUT MODES, **and the `-1` no-table sentinel with no error line (r46)** |
| **`TestDOSFuzzer5AllAdvancedOptions` — SEVEN SIGNALS** | ✅ S6 and S7 CONTROLLED; 🚨 **S5's TYPED stratum is the last uncontrolled one — item 0r.** ⚠️ Its `DIVERGENT CLASS` line prints one command per CLASS, not per case — R52 |
| **⭐ `testplan/harness/uidiff/` — THE DOS-ANCHORED UI DIFFERENTIAL (r46)** | **The only instrument that compares DISPLAY SPACE to DOS over a GENERATED population.** Three tiers (plain identity · **one-option-at-a-time** · stacked), the three traps as executable assertions with positive controls, tolerances declared and printed, a distinct exit code for "harness fault suspected", and the live-page double-submit rule. **🚨 KNOWN BLIND SPOTS, RECORDED NOT FIXED: (1) its outbound adjustment-rate check is SELF-REFERENTIAL on the 365/360 basis (28 of 78 checks) and MASKS the FILED `ReportedRate` item — twice, since `perYr ∈ {1,2,4,6,12}` cannot carry the canadian/daily bit; (2) `retires-to-zero` cannot fail downward because the painted balance is clamped at 0** |
| `testplan/harness/run_amz.js` (+ `run_pv`, `run_mtg`, `fuzz_amz_stale`) | **PRE-EXISTING browser+oracle differentials — the work plan did not know they were there.** ⚠️ **Engine space** (`amzScheduleData.result`), 29 hand-written cases, ONE invocation, no adjustment-rate kicker, **and `:41` INVERTS `balloonIncl → plusreg` — item 0v** |
| `internal/api/frontend_diff_sweep_test.go` | 20 node harnesses over the shipped JS. **🚨 SELF-REFERENTIAL AT `:447` BY DESIGN.** ⚠️ `:1980-1982`'s `if cell == "" { continue }` is R55 |
| `cmd/persense/frontend_r45_adjustment_paint_test.go` | NF-2 + NF-2b at the CONSUMER, asserting `dateValidity(painted).valid` (R53). **The template for any display fix.** |
| `internal/finance/amortization/horizonkeys.go` | **THE ONE implementation of the three scope keys. ← item 2 edits this.** |
| `internal/finance/amortization/oracle_gate_test.go` | `gateOracle` / `PERSENSE_REQUIRE_ORACLE` — the single fail-closed decision point. **Reuse it.** |
| `run_arm.sh` / `analyze_arm.py` | **⚠️ NO KILLED BUCKET (0h); prints SIGNAL INSTANCES over CASES, and that total is published NOWHERE** — which is why the exit criterion's 10% clause cannot be evaluated |
| `paired_regression.sh` | **⚠️ BLIND TO A HANG, TO AO9, TO THE APR, AND TO PV AND MORTGAGE ENTIRELY.** |
| `check_skips.sh` | R12. **⚠️ SUBSET, not equality — item 0c.** r41-r46: 32/32 |
| **AN ADVERSARIAL SUBAGENT AUDIT** | **STANDING INSTRUMENT (R32), THIRTEEN FOR THIRTEEN.** ⚠️ Give it the COMPLETE inventory (R46) and **MEASURE its findings.** **🚨 IN r46 IT REFUTED THE ROUND'S HEADLINE. THE RECORD ARM NEEDS NO TREE.** |
| **A PROBE / MUTATION TREE** | Free, ~5 s a cycle. **🚨 R44, R49, R51 apply — AND COUNT YOUR NEEDLE FIRST.** |

---

## 7. Backlog

**The top of the backlog IS §3b, in §3b's order — led by (1) the APR class,
(2) `reached`, (3) item 0j, (4) refusal parity, (5) 0-USAGE.**

**✅ CLOSED IN r46: item 24 (the UI differential is committed), item 0-MTG (filed),
item 0u (moot — that population died with 45b's container and r46 generated a
fresh 258), and item 0j is finally MEASURED though not settled.**
**⚠️ NOT REACHED: item 3 (FOUR rounds), 0-USAGE (THREE rounds), the APR class,
0c/0d/0e/0f/0h/0n/0o/0q/0r/0s/0t — and the PV forward figure and the plain zero
are carried a FOURTH round.**

Nate's standing decisions: **actuarial is secondary** (a permanent, fully
disclosed coverage hole); **§54 is document-and-defer**; **the exit criterion is
RATIFIED at 1 in 400 as a MILESTONE toward 99.99%** (**NOT MET — 1 in 83**);
**comparison beyond 2099 is NOT REQUIRED (client, 2026-08-03)** — ⚠️ and R33 shows
that boundary does not protect in-scope ROWS.

---

## 8. History ledger

| round / date | what it established |
|---|---|
| 2026-07-28 → Round 21 | `crmath.go`; §47-§58 closed; `harness_policy.md`; R1-R15; §59, §62 CLOSED. |
| Round 22-31 · 08-03/04 | §65-§68; the ERA LABEL found WRONG; PV/MORTGAGE BACKWARD BIT HARNESSES; **§65's `noterm` was THE ORACLE**; R16-R25. |
| Round 32-34 · 08-04/05 | **§65's advisory subclass was an ORACLE-DRIVER ARTEFACT, SAME-DAY RETRACTION.** The contingency table; **§71 — `AmortizeDOS` DOES NOT TERMINATE**; **THE ROUTER RETURNS A SET.** R26-R30. |
| Round 35-37 · 08-05/06 | **§71 CLOSED — the oracle driver was the obstacle for the THIRD time. AN INDEPENDENT AUDIT FOUND A REAL ARITHMETIC DEFECT IN THE FIX → R32.** "IN SCOPE" IS THREE PREDICATES. **An audit fix shipped BROKEN and its number was published through it → R36.** |
| Round 38 · 08-06/07 | **THE TWO BLOCKING MEASUREMENTS DONE, BOTH WORSE.** In scope **1 in 85**. **🚨 THE FAITHFUL PORT'S ZERO DIED: 0 → 3.** The `EMessage` delta: **~1 in 12**. |
| Round 39 (a-e) · 08-07/08 | **NATE'S FOUR UI REPORTS ROOT-CAUSED — AND THE GATES HAD NEVER LOOKED WHERE THE DEFECTS WERE.** Five product defects, ONE cause. The round's own review refuted it TWICE → R40. |
| Round 40 · 08-08 | **A RECORD AUDIT WITH NO TREE AT ALL.** **R41**, reclassifying every stacked figure as a LOWER BOUND. |
| Round 41 · 08-09 | **⭐ THE QUESTION-SET SPLIT MEASURED: 1 in 91 (Q4) vs 1 in 70 (Q7) on the SAME cases.** §74 OPENED. Score **2**. |
| Round 42 · 08-09 | **THE PV SCREEN'S ADVANCED OPTIONS STACK THE SAME WAY, AND THE SURFACE HAS NO ORACLE.** §75-§79. **TWO OF THE AUDIT'S OWN HIGH FINDINGS REFUTED BY MEASUREMENT.** Score **2**. |
| Round 43 · 08-09 | **A RECORD ROUND THAT CORRECTED THE TOP TWO ITEMS OF ITS OWN PLAN.** Signal 6's control was **VACUOUS, not inert** → **R49**, **R50**. **NATE CLEARED THE DECISION QUEUE.** Score **2**. |
| Round 44 · 08-10 | **THE SECOND UNCONTROLLED SIGNAL BECAME A GATE — AND THE ROUND'S TWO BEST FINDINGS WERE KILLED BY ITS OWN CONTROLS.** §84; **R51**; **§85 — an attribution MADE AND WITHDRAWN IN-ROUND**; §87. Score **2**, dimension 3 → 7. |
| Round 45 · 08-10 | **NF-1 FELL TO TWENTY LINES OF PASCAL AFTER FIVE ROUNDS OF FUZZING — AND THE ROUND'S BEST NUMBER WAS AN ARTEFACT IT CAUGHT ITSELF.** NF-1 and NF-2 FIXED; **NF-1c NEW, found by STEP 0**; Q7 in-scope **30 → 25** with the Q4 control HELD — a DISPLAY fix; **R52**. Score **2**, dimension 5 → 4. |
| Round 45b · 08-10 | **THE FIRST DOS-ANCHORED UI DIFFERENTIAL RUN — AND IT CAUGHT THE PREVIOUS ROUND'S FIX BREAKING THE SCREEN.** §89: the r45 NF-2 fix wrote the snapped date back as RAW ISO into a field whose validator REJECTS ISO — **21 of 76 adjustment screens unsubmittable.** **THE GUARD FAILED IN THE WAY IT EXISTED TO PREVENT → R53** + the double-submit process rule. A new engine divergence (USA ∧ prepay ∧ adj). **🚨 THE INSTRUMENT WAS NOT COMMITTED.** Score **2**. |
| **Round 46 · 08-10** | **THE UI DIFFERENTIAL IS COMMITTED — AND THE ROUND AUDITED ITSELF OUT OF ITS OWN HEADLINE.** **⭐ (a) `testplan/harness/uidiff/`: three tiers including a NEW one-option-at-a-time tier that closes R49 one level down, the three r45b traps as executable assertions with positive controls, 6 of 6 mutants killed, tolerances declared and printed, running on the CONTAINER'S OWN CHROMIUM.** **🚨 (b) THE HEADLINE ATTRIBUTION WAS WITHDRAWN BEFORE PUBLICATION BY THE ROUND'S OWN ADVERSARIAL AUDIT:** `gen.js` never recomputed `basePayment` after the `peryr` option, so annual screens carried a MONTHLY installment (10.9× too small) and the harness manufactured the non-convergence it attributed — **9 of 9 / 0 of 117 became 3 of 83 / 1 of 117. R32's auditor is THIRTEEN FOR THIRTEEN.** **⭐ (c) A REAL NEW CLASS SURVIVES: REFUSAL PARITY, 4 in 139 SCORED screens, three of the four painting NO warning.** **⭐ (d) TWO REACHABLE DEFECTS IN COMMITTED PRIOR ART** — `run_amz.js:41` inverts `balloonIncl → plusreg` ($1,600.09 on AMZ-C-57), and `pts=` must be emitted even at zero. **⭐ (e) 0-MTG DONE: 14 findings, 3 HIGH, filed — and NO NF-1c analogue and NO §79 instance, two expected findings that did not materialise.** **🚨 (f) ITEM 0j MEASURED AND ITS LINE NUMBERS CORRECTED after nine rounds; the round's own "inert" claim REFUTED — reachable 14 times in 600 cases, outcome-neutral so far.** **🚨 (g) SEVEN of its own harness bugs killed before the engine was implicated once, plus four more from the audit → R54, R55, R56.** Score **2 — unchanged; and the round says plainly that the APR class and `reached` are what would move it.** |

---

## 9. Where the canonical documents live

| what | live home | snapshot in repo? |
|---|---|---|
| **This file** | project: `claude/START_HERE.md` | `docs/history/START_HERE.md` — refreshed at commit time; **see §0** |
| **⭐ ROUND 46 — the record** | **`claude/round46_the_ui_differential_committed_and_the_round_audited_out_its_own_headline_2026-08-10.md`** | no |
| **⭐ ROUND 46 — the mortgage audit (item 0-MTG)** | **`claude/round46_mortgage_screen_readback_audit_0mtg_2026-08-10.md`** | no |
| **ROUND 45b / 45 — the records** | `claude/round45b_…`, `claude/round45_…` | no |
| **⭐ CONVERGENCE ASSESSMENT (live)** | **`claude/convergence_assessment_2026-08-10_round46.md`** | no |
| **⭐ THE RESTATEMENT** | `claude/restatement_of_published_numbers_2026-08-08.md` | no — **⭐ READ BEFORE QUOTING** |
| **⭐ THE ROUND-40 RECORD AUDIT** | `claude/ROUND40_AUDIT_of_the_round39_record_2026-08-08.md` | no — **§3.1 is the source of R49** |
| **⚠️ CLIENT NOTE — STILL DRAFT, STILL UNREVIEWED** | `claude/convergence_note_client_2026-08-08_DRAFT.md` | no — **item 6. 🚨 Does not yet mention §89.** |
| **§71-§89, §35A, NF-6** | `docs/discrepancies.md` | **yes, committed at `8ef6fce`** |
| **The UI differential** | **`testplan/harness/uidiff/` — committed at `789a7d1`, with its own README** | **yes** |

### The snapshot rule — DECIDED by Nate, 2026-08-01

**The project copy stays the single live document; a snapshot lands in
`docs/history/` at COMMIT TIME only.** Do not snapshot mid-round.
**⚠️ AND PER §0: that snapshot is necessarily a SEPARATE, LATER commit than the
file it contains. Do not treat it as drift, and do not record a HEAD hash here.**
