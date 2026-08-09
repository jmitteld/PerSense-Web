# START_HERE snapshot — ROUND 41, commit `9deb35c` (2026-08-09)

## ⚠️ READ THIS HEADER FIRST — THIS IS A STATE SNAPSHOT, NOT A FULL COPY

**The single live document is `claude/START_HERE.md` in the claude.ai project.**
This file is the repo-side snapshot required by the snapshot rule (Nate,
2026-08-01: *"a snapshot lands in `docs/history/` at COMMIT TIME"*).

**It carries the STATE sections — where things stand, the standing decisions, the
work plan, the history ledger — and it deliberately does NOT duplicate the live
document's §1 (container bootstrap), §4 (standing rules R1-R46), §5 (the trap
list) or §6 (instrument inventory).** Those run to tens of kilobytes, change every
round, and a stale partial copy of them is actively dangerous: someone would
bootstrap from an outdated FIX 4 or act on a retired trap.

**🚨 A NOTE FOR NATE — THE SNAPSHOT RULE MAY NEED AMENDING.** This snapshot has
been flagged "NOT REFRESHED" for **five consecutive rounds** (since `804ba0c`).
Five rounds of an unmet obligation is evidence about the obligation, not about the
rounds: a full byte-for-byte duplicate of a 40 KB living document, re-emitted by
hand at every commit, is not a process that holds. **Three options, your call:**
(a) keep the rule and accept it will keep slipping; (b) formally narrow it to what
this file now is — a state snapshot plus a pointer; (c) drop the repo snapshot and
treat the project copy as the sole record. **This file implements (b) on the
assumption it is the least-bad default; say the word and it changes.**

---

## Where the live documents are

| what | path |
|---|---|
| **THE LIVE START_HERE** | project: `claude/START_HERE.md` |
| Round-41 record | project: `claude/round41_r41_measured_engine_transported_and_0commit_was_already_done_2026-08-09.md` |
| Convergence assessment (live) | project: `claude/convergence_assessment_2026-08-09_round41.md` |
| The restatement of published numbers | project: `claude/restatement_of_published_numbers_2026-08-08.md` |
| The round-40 record audit | project: `claude/ROUND40_AUDIT_of_the_round39_record_2026-08-08.md` |
| Client note — **DRAFT, unreviewed, now stale** | project: `claude/convergence_note_client_2026-08-08_DRAFT.md` |
| Discrepancies §71-§74, §35A, NF-6 | repo: `docs/discrepancies.md` (committed) |

---

## Commit state at this snapshot

**HEAD `9deb35c`**, parent `e2b4891`. **Working tree clean.**

Round 41 committed nine files: `internal/finance/amortization/{types.go,
engine.go, dos_fuzzer5_test.go, dosport_entry.go, zzr41_engine_transport_test.go
(new), zzr41_questionset_test.go (new)}`, `legacy/oracle/amort_oracle.pas`,
`scripts/rule7_mordmy_corpus.sh` (new), `docs/discrepancies.md`.

**🚨 THE PRECEDING TWO ROUNDS' HEADLINE ALARM WAS FALSE.** START_HERE led rounds 40
and 41 with *"NOTHING IS COMMITTED, HEAD IS `f78c244`"*. Nate had committed round
39's nineteen files himself on 2026-08-07 (`74bc8b2`, `87f8af5`, `e2b4891`) from
his own Mac while that session's `device_bash` was dead; round 40 had no bridge
and could not see it. **→ R45: an alarm that outlives its condition is a
correction that never got made.** Both of START_HERE's explicitly-flagged
inferences about that commit turned out CORRECT: the unnamed 19th file was
`legacy/oracle/amort_oracle.pas`, and the `.go` count is 426 (428 after round 41's
two new test files).

Bootstrap tarballs on the drive: `_to_delete/r41{src,dos,fix}.tar.gz`, built from
`9deb35c`, scratch-build verified.
`r41src` md5 `2d272c9228cd3807e08b7a1d3b299400`; `r41dos`
`099986e1791a50ee80fc20438485f048`; `r41fix` `666bd710e7f9e1d5fbc41b3ddf4d605b`.

---

## THE ROUND-41 MEASUREMENT — the question-set split (R41)

Seeds 50100-50109, `PERSENSE_FUZZ_N=400`, HEAD `9deb35c`, unfiltered modes,
`horizon` scope key, no engine filter. 4,000 generated / 2,211 compared.
**Same cases, same tree, same run — only the question set differs.**

| population | compared | FOUR-question HARD | SEVEN-question HARD |
|---|---|---|---|
| **in scope ≤2099** | **2,086** | **23 — 1 in 91** | **30 — 1 in 70** |
| out of scope >2099 | 125 | 2 — 1 in 62 | 3 — 1 in 42 |
| pooled | 2,211 | 25 — 1 in 88 | 33 — 1 in 67 |

**The three signals added in 39e put +32% on the HARD numerator with no code
change. The 1-in-400 bar goes from missed by 4.4× to missed by 5.8×.**
The four-question column reproduces round 38's 1-in-85 (measured over 33,753
cases) on a 16× smaller sample — that agreement is what makes the seven-question
column credible rather than a small-sample artefact.

`dos_fuzzer5` now prints **both columns every run**, guarded by
`zzr41_questionset_test.go` (a source-layout guard, seen to fail).

### The per-signal baseline (item 0l — DONE)

| signal | denominator | events | rate |
|---|---|---|---|
| Signal 5 — **SOLVED** | 449 checked | 2 | **1 in 224** |
| Signal 5 — TYPED | 1,759 checked | 0 | *input vs itself — near-vacuous* |
| Signal 6 — APR | 1,856 compared | 20 | **1 in 93** |
| Signal 7 — adjustment rows | 858 rows / 461 screens | 35 | **1 in 25** |
| engine `dosport` | 106 compared | 0 | 0 in 106 |
| engine `piecewise` | 2,102 compared | 33 | 1 in 64 |
| Signal 7 `piecewise` | 845 rows | 35 | 1 in 24 |
| Signal 7 `dosport` | **13 rows** | 0 | ⚠️ denominator 13 |

**Three reading rules, all load-bearing:**
1. **Only 20.3% of Signal 5's checks are the SOLVED stratum.** 39e's "230 checked,
   0 differ ← the transport HOLDS" rested on 50 load-bearing cases, not 230.
2. **Signal 5's 2 events are NOT a surviving R39 transport defect** — both cases'
   totals also diverge. Do not write the tempting sentence.
3. **Signal 6 is STILL UNCONTROLLED** (its negative control was INERT and item
   0m(i) did not land). **1 in 93 is a finder's output, not a gate's — R20.**

---

## Gates at this commit (R37 — naming what did NOT run)

| gate | result |
|---|---|
| full suite, `PERSENSE_REQUIRE_ORACLE=1`, **`PERSENSE_FUZZ` UNSET** | **12 packages, 0 FAIL** |
| **what therefore did NOT run** | the randomized differentials — `TestDOSFuzzer5AllAdvancedOptions` and `TestPayoffRandomizedSweep`. **Under `PERSENSE_FUZZ=1` the suite is RED by design.** |
| `check_skips.sh` | 32/32; round 41's new tests add **no new skips** |
| `paired_regression.sh`, seeds 50100-50109, N=400 | **FIXED 0 · STILL 67 · NEW 0** |
| PV backward rate, bit level, N=1500 | 1,500 compared, 1,499 bit-exact, 1 ULP, sign p=1 (reproduces r30/r37) |
| mortgage backward balloon amount, bit level, N=1500 | 1,500 compared, 1,499 bit-exact, 1 ULP, sign p=1 |
| PV screen-total bit fidelity | 1,920 checked, 0 divergences |
| actuarial | passed (round 39's environmental failure did not recur) |
| **FIX 5 manifest diff** | **0 differing hashes, 811 absent, 0 `.go` absent** — bootstrap intact AND current |

---

## Open defects at this snapshot

| # | defect | state |
|---|---|---|
| **NF-1** | the piecewise adjustment echo drops the New Amount on 30-38% of rows — **Nate's blank cell is still live on that route** | **OPEN — HIGH.** r41 measured it: 35 findings, 845 rows, **100% piecewise** |
| **NF-1b** | 2 of 113 `dosport` rows echo ~2× DOS's amount | **OPEN.** ⚠️ invisible to Signal 7 at current scale (13 dosport rows in 858) |
| **NF-2** | DOS snaps an off-grid adjustment date and echoes the snapped one; the DOM row keeps the typed date → nothing paints (16 of 400) | **OPEN — HIGH** |
| NF-3 / NF-4 / NF-5 | smaller display-layer items | OPEN |
| **✅ NF-6** | the `mordmy=` year bound wrapped above 2155 | **CLOSED r41** — bound 2155 **and a loud `ERR`+`Halt`**, because narrowing alone was measured to trade a wrong-century answer for a silently moratorium-free one |
| **APR-R** | residual APR divergence class — **20 in 1,856** | **OPEN, UNATTRIBUTED.** Candidates `engine.go:4574`, `backward.go:829` |
| **§74** | `ParseDMY` exits without writing its out-parameter → a malformed `payoff=` makes **the oracle NONDETERMINISTIC** (40 identical invocations, four answers, exit 0) | **NEW r41, OPEN.** Pre-existing; no published number known affected; **invalidates single-shot rule-7 checking** |
| §73 | `types.DateRec` cannot hold 29 Feb 2100 | OPEN — decision 3a.12 |
| §60 | 1 payoff divergence in 428 | OPEN — **twenty rounds** |
| seed 50152 | reproducible hang | UNATTRIBUTED — item 0h |
| `dosport`'s 3 | in-scope divergences on the faithful engine (r38) | OPEN — the most diagnostic cases the project has |

---

## Convergence score at this snapshot

**2 — seventh round running.** Five dimensions, scored as the minimum:
rate-meets-bar **2** (1 in 70 vs a 1-in-400 bar), numerator-adjudicated **5**,
instrument-trustworthy **6** (up 1), sample-space-coverage **7**,
no-known-open-engine-defect **2**.

**Round 41's contribution: zero on the number — no arithmetic changed, and
`paired_regression` returned FIXED 0 / NEW 0 as expected — and +1 on the
instrument.** Round 40 predicted in writing that the first seven-question rate
"will very likely be worse, and that will not be a regression". It is worse by
1.30×, from the instrument and not from the port. That is the second time in four
rounds the project has predicted a negative result in advance and confirmed it.

---

## The round-42 plan, in order

1. **NF-1 and NF-2** — the only open items a user sees, and Signal 7 has already
   done the measurement work (35 findings, 100% piecewise).
2. **0m(i) — re-control Signal 6** (~20 lines) **before quoting 1 in 93.**
3. **0h — attribute the seed-50152 hang**, and first resolve 39D's `NEW=0` over a
   range that contains it.
4. The carried instrument work: 0c, 0d, 0e (five unpinned tolerance floors), 0f,
   0i, 0j, 0g, **0n** (stratify the per-engine table by mode), **0o** (§74).
5. Mechanise: the residual APR class, `dosport`'s 3, the +115 EMessage-delta band.
6. **Decide 3a.11 (the scope key)** — sixth round blocked; gates items 7 and 8.

**Five decisions remain open for Nate: 3a.11 (scope key), 3a.12 (§73), 3a.13
(mortgage APR day-count), 3a.9 phase 2, 3a.14 (the balloon default — and its price
is now in the code at `dosport_entry.go:701-710`: 1 in 22 on the clause it moves
traffic onto).**

---

## New standing rules and notes from round 41

- **R45 — AN ALARM THAT OUTLIVES ITS CONDITION IS A CORRECTION THAT NEVER GOT
  MADE.** Give every carried blocker an expiry; the first session that CAN verify
  it must, before planning around it.
- **R46 — AN AUDIT IS ONLY AS GOOD AS THE INVENTORY IT IS GIVEN.** A partial
  inventory produces authoritative-sounding errors in both directions.
- **Note #49** — a refused folder grant may mean an unmounted volume; list a home
  directory to tell the two apart.
- **Note #50** — a blocked-state claim decays like any other.
- **Note #51** — `mv .git/*.lock _to_delete/` fails on this mount; an **in-place
  rename** works.
- **Note #52** — `FUZZ_N` is the arm scripts' variable; the test reads
  `PERSENSE_FUZZ_N` and silently defaults to **300**.
- **Note #53** — plain `nohup … &` is killed when the tool shell exits, and the
  truncated output reads as a clean result. `setsid … & disown`, plus a sentinel.

**Rule 6 gained a clause:** a rule-7 claim carries a **checked-in corpus**
(`scripts/rule7_mordmy_corpus.sh`). A verification that cannot be re-run is a
claim about its author. **Rule 7 gained one too:** rule-7 verification must be
**repeat-sampled**, because of §74.
