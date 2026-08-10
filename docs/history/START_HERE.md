# START_HERE — STATE SNAPSHOT at the round-45b code commit `8ef6fce` (2026-08-10)

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

- **LAST CODE COMMIT: `8ef6fce`** — *"round 45b: the NF-2 fix shipped a cell the
  app's own validator rejects — and the guard that should have caught it verified
  the PRODUCER (§89)"*. Parent `ec631e0` (r45 snapshot), then `bfc90f2` (r45 §88),
  `efe69f3` (r44 snapshot), `cee4943` (r44), `2b03814` (r43 §83).
- **HEAD: MEASURE IT.** `git rev-parse --short HEAD`. It is `8ef6fce` **or one
  doc-only snapshot commit on top of it. BOTH ARE CORRECT.**
- **Working tree CLEAN** apart from untracked `_to_delete/`.
- **⚠️ MEASURE EVERY COUNT BELOW. DO NOT QUOTE THEM.** Round 44's `.go` gate was
  recorded as 430 and was 433.

## Bootstrap

- **Tarball set `r45b*` in `_to_delete/`, built at the LAST CODE COMMIT,
  foreground scratch-build verified (`go build ./...` exit 0, `go vet` 0):**
  - `r45bsrc.tar.gz` `0795c64437d7e208ddffabd45267b629`
  - `r45bdos.tar.gz` `099986e1791a50ee80fc20438485f048`
  - `r45bfix.tar.gz` `666bd710e7f9e1d5fbc41b3ddf4d605b`

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
  documented invariant, not drift. **ANY OTHER differing file, above all a `.go`
  or `index.html`, IS REAL DRIFT: stage the drive's copy before touching it
  (note #58).** Rounds 45 and 45b both ran it and got exactly that.
- **⚠️ EXCLUDE `_to_delete/` FROM THE FIX-4 MEMBER LIST** or the new set carries
  the previous set inside itself.

## What round 45 changed (code), and what 45b changed on top of it

**ROUND 45 — the engine and wire half:**

- **NF-1 FIXED** — `internal/finance/amortization/engine.go`. `AMORTOP.pas:1591-1592`
  stores the re-amortized adjustment payment on EVERY crossing; only the `amtok`
  LATCH sits behind the gate at `:1571`. The port had the WHOLE store inside the
  gate. **Open since round 39D. Guard: `zzr45_nf1_piecewise_echo_test.go`.**
- **NF-1c FIXED** — `internal/api/handlers.go`. The solved adjustment rate echoed
  in INTERNAL space; `amzUnkickerRate` now applies (`INTSUTIL.pas:1649-1651`).
- **NF-2 FIXED, AND ITS DESCRIPTION RETRACTED** — the port ALWAYS snapped and
  echoed the snapped date. The break was one strict `!==` in
  `cmd/persense/static/index.html`. The wire now carries `requestedDate`.
- **A1-A5** — `internal/api/zzr45_adjustment_echo_wire_test.go`.
- **`docs/discrepancies.md` §88.**

**🚨 ROUND 45b — AND IT IS A CORRECTION OF THE ABOVE:**

- **§89 — THE ROUND-45 NF-2 FIX SHIPPED A USER-VISIBLE REGRESSION.** It wrote the
  snapped date back as `a.date`, the **RAW WIRE VALUE, which is ISO**. That
  field's validator `dateValidity` admits **MM/DD/YYYY only and REJECTS ISO**, so
  the cell held a value **the application itself refuses**: the first Calculate
  succeeded and the **SECOND — with the user changing nothing — was BLOCKED**.
  **21 of 76 adjustment-carrying screens = 28% unsubmittable.**
  **THE FIX:** `dEl.value = fmtDateDisplay(a.date);` plus clearing `date-invalid`.
- **🚨 THE GUARD FAILED IN THE WAY IT EXISTED TO PREVENT.**
  `frontend_r45_adjustment_paint_test.go` was written to enforce **R42 (verify at
  the CONSUMER)** and passed throughout, because it compared the painted cell to a
  **literal** against a **fake DOM with no validator**. **→ R53.** It now extracts
  the shipped `dateValidity` / `fmtDateDisplay` and asserts
  `dateValidity(painted).valid` on every case.
- **⭐ FOUND BY THE FIRST DOS-ANCHORED UI DIFFERENTIAL THE PROJECT HAS EVER RUN**
  — 200 stacked screens + 10 identity controls, driven through the **LIVE page in
  Chrome** against `amort_oracle`. **🚨 THE INSTRUMENT WAS SCRATCH IN `/tmp` AND
  IS GONE. Rebuilding and committing it is round 46's item 1.**
- **⭐ A NEW ENGINE DIVERGENCE, OPEN AND UNATTRIBUTED:** DOS total interest
  **20,528.59** vs port **8,798.24**; ablates to **USA ∧ prepayment ∧ adjustment**
  (every option agrees alone and in every PAIR).
- **⭐ NEW SCOPE FACT: IN-ADVANCE REFUSES *ANY* ADJUSTMENT**, not merely a rate
  one — so NF-1's "in-advance" clause was never a reachable population.

## The numbers that matter

- **In-scope SEVEN-question HARD: 25 in 2,086 = 1 in 83** (was 30 = 1 in 70).
  **The FOUR-question CONTROL HELD at 23 = 1 in 91.**
  **🚨 This is a DISPLAY-TRANSPORT correction, NOT engine convergence.**
  Bar (1 in 400) missed **4.79×**. *(r45; NOT re-measured in 45b.)*
- **UI differential, stacked, DOS-anchored: 3 in 200 SCREENS** — 1 adjudicable,
  2 off-grid and not adjudicable. Identity controls 0 in 10. **⚠️ UNIT IS SCREENS.**
- **Item 5: of the 20 APR divergences, 2 are totals-green by the instrument's
  signals and only 1 is totals-IDENTICAL by ground truth.** That isolate carries
  **NO ADJUSTMENTS** and is **OUT OF SCOPE** (horizon 2133). **The class does NOT
  dissolve; R27 stands.**
- **🚨 Convergence score 2, TWELFTH round. 45b did not move it** — it closed a
  regression the project created and added an open defect. **Dimension 5 does not
  rise for fixing your own regression.**

## Gates round 45b ran, and what it did NOT run (R37)

**RAN:** full suite **12 ok / 0 fail** with `PERSENSE_FUZZ` unset, **after the
last edit**; `check_skips.sh` **32/32**; the new guard **seen to fail both ways**
on a probe tree; scratch build exit 0 / vet 0; round-trip md5 **6 of 6**; FIX 4
rebuilt at the code commit with the §89 fix verified present inside the tarball.

**DID NOT RUN:** every randomized fuzz arm, `paired_regression`, the PV and
mortgage arms, the backward bit harnesses, and the gated (`PERSENSE_FUZZ=1`)
suite — **which is RED BY DESIGN**. No engine file changed; the diff is
`index.html` plus one test.

**ORACLE BUILD FLAGS, beside every zero (R47):**
`-Mdelphi -Sg -CPPACKRECORD=1 -dV_3 -dSCROLLS -dPVLX`. **`-dACTU` IS ABSENT AND
UNBUILDABLE** — the `ACTUARY` unit source is missing (§82). Say **ABSENT**, never
"pending".

## Standing hazards a fresh round must not re-learn

- **🚨 `cmd/persense/main.go:23` IS `//go:embed static`** — the running binary
  serves an embedded SNAPSHOT of `index.html`. **A fix on disk is NOT live until
  the binary is rebuilt. Grep the SERVED html for a marker of your fix before
  trusting any UI result.**
- **🚨 ANY `index.html` CHANGE GETS A LIVE-PAGE DOUBLE-SUBMIT CHECK BEFORE
  COMMIT** — calculate, then calculate again with nothing changed. That one check
  would have caught §89. It costs about a minute.
- **🚨 `parseDate` ACCEPTS ISO; `dateValidity` REJECTS IT.** Two date paths in one
  file disagree. **Feed the UI MM/DD/YYYY.**
- **🚨 `clearAmortization()` CALLS `confirm()`** — a modal freezes the whole
  browser-automation channel. Write your own reset, and **clear
  `#amz-total-paid` / `#amz-total-int` in it** or a blocked calc scores the
  restored session's totals.
- **🚨 `apr` AND `bdump` ARE EACH A SEPARATE ORACLE OUTPUT MODE** and suppress the
  payment/adjrow lines. **`apr adjdump` returns `apr 0.000000 status 0`** — a
  real-looking zero. Two invocations per case.
- **🚨 THE 365/360 KICKER PRODUCED A FAKE 100% DIVERGENCE RATE.** Kick the loan
  rate **and the adjustment rates**. **A 100% rate is a MAPPING BUG, not a
  defect** — and the tell is payment, total-interest and total-paid diverging on
  EXACTLY the same set.
- **`device_bash` CANNOT DELETE**; `git rm` fails, `git rm --cached` and `mv`
  work. **Rename `.git/index.lock` and `.git/HEAD.lock` IN PLACE before and after
  every git write; read `git log`, not the unlink warnings. A stale lock from
  YOUR OWN previous commit blocks the next one.**
- **`cd X && setsid nohup … & disown ; cmd` runs `cmd` in the ORIGINAL cwd** —
  the `&` backgrounds the whole `cd &&` list (note #55, fired three times in
  r45/45b).
- **The device bridge and Chrome are INDEPENDENT channels.** In 45b the bridge
  died mid-session and came back after ~10 min of periodic `RefreshMcpTools`
  while Chrome stayed up. **Check both.**
- **`tar x` on the SSK mount cannot overwrite** — new files only.
- **A tarball set that predates HEAD can silently REVERT a commit** (note #58).
  `grep -c '## §NN'` before every push.
- **A join across two log lines can return a spectacular, wholly false answer** —
  **normalise the key, and check whether the emitter prints one line per CASE or
  per CLASS (R52).**
- **A mutation needle that does not match reads exactly like a surviving
  mutant.** Count the needle first and report INVALID.
- **`PERSENSE_FUZZ_N`, not `FUZZ_N`, is what the test reads** (note #52).
