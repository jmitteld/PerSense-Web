# Harness policy — why the fidelity harness keeps producing wrong findings, and the rules that stop it

**Created 2026-08-02 (round 16), after §58 became the seventh defect in the same
family.** Companion to `docs/testing_policy.md`, which covers how we test and how
we count confidence. This document covers the *instrument*.

---

## 1. The pattern

Eleven harness defects, over five weeks, every one of them the harness computing —
or judging — something the product already gets right:

| # | defect | where the product got it right | write-up |
|---|---|---|---|
| 1-4 | four `cmd/goamort` date bugs | the engine's date layer | §51 and earlier |
| 5 | the oracle driver's option-date blocks assign the year into a Pascal `byte`, so a widened term handed DOS a wrapped year and Go an unwrapped one | — (a faithfulness bug in project-authored Pascal) | §55 |
| 6 | `firstPeriodDate` computes `m := 1 + 12/perYr` in INTEGER division, collapsing every sub-monthly frequency to month 1 | the engine | round 14 §4a |
| **7** | **the fuzzer discarded the backward solvers' `converged` flag and amortized at a rate the product refuses to display** | **`handlers.go:1260`** | **§58** |
| **8** | **`cmd/goamort`'s `parseDMY` printed "Refusing" on an impossible date and then fell through to the DEFAULT loan date** — every call site is `if d, ok := …; ok {}`, so goamort amortized 1.1.2024 while DOS amortized the typed 30 February: fake divergences >$500/payment in the long-horizon sweep | the refusal message itself described the right behavior; the code didn't do it | round 16b, `docs/fuzzer_sample_space_audit_2026-08-02.md` §3 |
| **9** | **`runDump` spawned the oracle with a bare `exec.Command().Output()` — no timeout.** The DOS engine does not terminate on some screens (it writes `Bad date passed to Julian function: m=-99` to stdout forever), so ONE such screen hung the test binary until the outer wrapper's `timeout` killed it, **discarding that seed's entire 400 cases: no ledger, no COVERAGE line, no signals, and nothing in the output saying so** | there is no product counterpart — the product never execs the oracle. A pure instrument defect, and the first one whose failure mode is *silent loss of a whole measurement unit* rather than a wrong value | round 17 |
| **10** | **the terminating-balloon tolerance was scaled to the LOAN AMOUNT** (`max(0.05, 1e-5*amount)`) while the value it guards is the balance at the schedule's terminating date — which on a screen that does not amortize is not bounded by the loan at all. Measured over the `non` arm, `\|tack\|/\|amount\|` has a **median of 59.9 and a maximum of 1,572,380**, so a $2-4 absolute tolerance demanded agreement to 1.4e-11 relative. **78% of that arm's balloon signals agreed to better than 1e-2 relative — most to 1e-7 — and every one was reported `SIG=HARD`.** It inflated the largest class in the standing residual and manufactured round 17's `non`-mode "frontier" | every other comparison in the same test scales to the value being compared: `intTol`/`paidTol` use `5e-4*\|dos.interest\|`, the backward-solve check `2e-6*\|dos.solvedAmt\|`. The balloon was the only one keyed to a different quantity | round 18 |
| **11** | **the `-1/-1 with a valid date` no-totals sentinel returned IMMEDIATELY, on the premise that it is deterministic.** It is not. Re-probing 18 dumped cases: **12 reproduce the sentinel, 4 return REAL totals, 2 do not parse**; re-running those 4 at concurrency 6 on 2 cores gives `TOTALS 23 / SENTINEL 1` on three of them. So a screen DOS answers perfectly well was being PERMANENTLY excluded from comparison ~4% of the time on large schedules, with nothing saying a comparable case had been dropped | the sibling arm was right: the DATE-HORIZON sentinel carries a structural marker (`nperiods 0`, or a wrapped `-88/0/1900` date) that a resource failure cannot manufacture, and it really is deterministic. The no-totals arm carries no marker at all | round 18b |

Plus the near-misses that cost a round each without earning a section number:

- `cmd/goamort` **silently ignores tokens it does not implement** (no `default`
  arm, and it implements neither `norate` nor `noamt`). Round 13 reported a
  fictitious **76% backward-solve defect** this way, and it reached
  `START_HERE`'s NEXT ACTION before being caught.
- `cmd/goamort`'s **payment echo is a heuristic**, not a value — it reports the
  first `PayNum>=1` row with a nonzero `PayAmt`, so on a prepayment series it can
  report the prepayment. Two "divergences" in round 13's stratum A were this.
- `long_horizon_sweep.py` draws `ld = rng.choice([1,15,28,29,30])` against a
  random month, so it generates **30 February** routinely; the port refuses those
  (§51) and the sweep scored each as a divergence. Three of eleven stratum-A hits.
- **Unquantized oracle arguments** made **400 of 400** amortization payments
  compare unequal, worst ~4.8e10 ULP — indistinguishable from a catastrophic
  engine defect.
- `refdata.json` was treated as a second oracle for months. It hand-transcribes
  the DOS routines instead of calling them, and its `MDY` silently drops DOS's
  `daynumber > 70000` guard — the likely origin of the port's wrong Julian
  ceiling, which hid a PV defect worth up to 23.5% (§47).

**The harness is a second, worse implementation of the product.** That is the
single root cause, and every rule below follows from it.

§58 is the purest instance and the reason this document exists. `handlers.go`
does *solve → check the flag → amortize*. The fuzzer reassembled that sequence
from parts and dropped the middle step. It could not have happened if the fuzzer
had called what production calls.

---

## 2. The rules

### R1. The harness drives the product's entry point, not a reassembly of it.

Where the product has a pipeline, the harness calls that pipeline. It does not
re-derive the sequence, and it does not re-decide anything the product decides.

*Status: **LANDED, round 16** (`internal/finance/amortization/screen.go`).*
`SolveBlankCells` / `SolveBlankCellsPrepared` / `RunScreen` now own the
solve → gate → amortize sequence. `handlers.go` calls `SolveBlankCells`;
`dos_fuzzer5_test.go` calls `SolveBlankCellsPrepared` — the `...Prepared` variant
because the generator constructs a fully-specified screen itself and must not
inherit the handler's `FirstPass` derivation, which would change the draw. **The
gate is shared; the preparation is not.**

Verified behaviour-identical: the fuzzer's signal set over seeds 44000-44004 at
`FUZZ_N=150` is byte-identical before and after the rewiring, and the full gated
suite (including `internal/api`, uncached) is green. §58's defect class is now
structurally impossible — there is one convergence gate and both callers reach it.

Still to do: `cmd/goamort` should call `RunScreen` too. It currently implements
neither `norate` nor `noamt`, which R3 now refuses outright rather than ignoring.

### R2. The harness never computes a date.

Six of the seven. Every date the harness needs comes from the oracle's own echo
(`GOAMORT_ROWDATES=1`, `solvedterm <n> last <y>-<m>-<d>`, `dumpraw`) or from a
single shared helper that is itself differentially tested against
`amort_oracle intutil addn`.

Corollary, and it has paid twice: **check what the oracle already prints before
writing a parser.** Round 14 recorded that the round trip's term axis "needs the
`bdump` parser"; round 15 found `solvedterm` already carrying both cells and the
whole job was a five-line field scan.

*Status: **PARTIALLY LANDED, round 16** (`zzharnessdates_test.go`).* The escape
clause — *or from a single shared helper that is itself differentially tested* —
is now satisfied for `dos_fuzzer5_test.go`'s option-date placement.

`fz5AddMonths` was a **closure inside the test function**: one hand-rolled
implementation of DOS's month step (year byte and month-end clamp included) that
nothing could reach and therefore nothing could test. It is now a package-level
helper pinned against DOS's own `AddNPeriods` via `amort_oracle intutil addn`
across 8 start dates × 6 whole-month frequencies × 11 period counts (0 → 3600
periods, i.e. past the §55 year byte). Extraction verified behaviour-identical:
the fuzzer's signal set over seeds 44000 / 44003 / 50100 is byte-identical
before and after.

**It found a disagreement on its first run — 1 of 528 — and it is §54.**
900 months from 29 Feb 2024 wraps (year byte) to 1900; DOS's leap rule has no
century correction, so DOS holds **1900-02-29**, and `types.DateRec` is backed by
`time.Time`, which rolls that to 1 March. So the harness hands DOS one option
date and the Go engine another whenever an option lands on 29 February in a
DOS-leap / Gregorian-non-leap year. **That is not a new defect — it is §54,
deferred by decision — but it is the first measurement of §54's reach inside the
harness itself, and it feeds START_HERE §3's "quantify §54".** The test buckets
those separately, fails only on genuine mismatches, and reports the rate.

A second test pins the *boundary*: at 24/26/52 a period is not a whole number of
months, `12/perYr` is 0, and a month step collapses every option onto the loan
date. That is precisely round 14's `firstPeriodDate` bug, now an executable
assertion rather than a comment.

**Still open under R2:** `cmd/goamort`'s own `monthsAfter` / `364/perYr` first-date
rule, and `long_horizon_sweep.py`'s date construction, are unpinned.

*Round 16b addendum:* defect **#8** (the table in §1) was exactly this family —
and the SWEEP now classifies unrepresentable input dates at generation time,
reporting an honest rate that excludes them. Combined effect on the quoted
long-horizon numbers: seed-913 total went from 7.81% quoted to **3.29% honest**;
the strata table in `docs/fuzzer_sample_space_audit_2026-08-02.md` §3 is the
current reference.

*Round 16b, second addendum — the SAMPLE-SPACE rule now has teeth:* every scalar
bound in fuzzer5's generator is a named `fz5*` constant, `zzsamplespace_test.go`
asserts the manifest (plus the term-draw reach, the whole-year-lattice
limitation, and the backward-mode solved-cell band), and the ledger prints the
GENERATED envelope beside the COMPARED one. The audit doc is the narrative;
the tests are the enforcement.

### R3. Unknown input is a hard error, everywhere.

Any token, mode, or option a driver does not implement must fail loudly.
*Status: **LANDED, round 16.*** `cmd/goamort` now refuses unknown tokens via
`unknownTokens()` (its four token loops have no single `default` arm to hang this
on, so it is a recognizer pass over the whole token list). Refusal goes to
**stderr with exit 2**, so DEFAULT STDOUT for every valid invocation is
byte-identical to the pre-change build — verified over a 13-command corpus
covering every implemented token, 13/13 identical. `norate` and `noamt` are now
rejected with a message naming the trap, instead of being silently ignored.

### R4. The harness is tested.

It currently has no tests at all. The instrument that decides whether the engine
is right is the only component in the repo nothing checks.

The cheap version: a **meta-fuzzer** that feeds screens *already adjudicated as
agreeing* through the full harness and asserts it reports agreement. Every
false-positive class above — unquantized arguments, wrong dates, ignored tokens —
manifests as "the harness invents a divergence on a screen we have already
proved matches", and would be caught by construction.

*Status: **LANDED, round 16** (`zzmetafuzz_test.go`).* First corpus is the §58
sweep plus the §56 anchors — screens whose DOS answers are pinned with oracle
provenance. It asserts the harness reports agreement on all of them.

### R5. Report `generated / comparable / compared / agreed / diverged`, and fail on a large generated-vs-compared gap.

Cases dropping out silently is how a 5% divergence rate and a 50% one look
identical. Round 14's bad `firstPeriodDate` suppressed **7 of 12** oracle
comparisons in one half of the round-trip gate while the other half reported
five spurious failures — the same bug, both directions, and the suppression was
the invisible one.

*Status: **LANDED, round 16.*** The fuzzer now prints an explicit ledger —
`generated = compared + refused + non-converged + date-horizon + flaked +
Go-refused | UNACCOUNTED n` — and FAILS if the unaccounted share exceeds 5%, or if
anything was generated and nothing was compared. **CLOSED, and it found two real defects in the accounting it was measuring.**

1. *The 2 of 120 (1.7%) reaching no terminal bucket* were cases abandoned at
   `if !anyOpt` — every advanced-option coin came up empty, so the case was
   dropped before the oracle was ever spawned. Intended, but invisible. Now
   counted as `skipped-plain`. **Coverage consequence, which is the more important
   half: this fuzzer can NEVER report a divergence on a plain loan.** Plain-loan
   fidelity is covered by `zzmetafuzz_test.go` and the committed unit suite —
   never quote a fuzzer5 rate as though it covered plain loans.

2. *The ledger then reported `UNACCOUNTED -1` on seed 50117* — a case counted
   TWICE. `goRefusedDosSolved` is incremented AFTER `checked++`, so it is a
   **subset** of "compared", not a sibling bucket. **That means the figure this
   harness has always called "compared" is overstated**: a case where DOS solved
   and the PORT refused is counted in it even though no totals comparison ever
   ran. The ledger now reports `ACTUALLY COMPARED = checked − Go-refused` and
   says explicitly that this is the denominator for any rate.

A ledger that must balance found a flaw in its own first formulation within three
seeds. That is the argument for the rule.

### R6. Severity belongs in the data, not in the reader's head.

`paired_regression.sh` greps every `amort_oracle …` line out of `-test.v`, so a
`NEW` can be a logged advisory or a hard failure and they look identical.
Round 13 spent an hour treating an advisory as a hard regression; standing rule 9
exists only because a human has to remember the difference. Emit a machine-
readable tag (`HARD` / `ADVISORY` / `INFO` + class) on every signal line and let
the paired diff bucket them.

*Status: **LANDED, round 16.*** All 11 signal sites in `dos_fuzzer5_test.go` now
emit `SIG=<HARD|ADVISORY>:<class>` immediately BEFORE the reproducing command, so
`paired_regression.sh`'s `grep -oE "amort_oracle .*"` extracts a byte-identical
key and every historical comparison stays valid, while a future paired diff can
bucket on `SIG=`. Classes: `go_solved_dos_refused`, `dos_solved_go_refused`,
`dos_nonconverge_go_nonretiring`, `balloon_{value_differs,dos_only,go_only}`,
`nperiods_differ`, `solved_{amount,rate,term}_differs`, `divergent_class`.

### R7. The generator's coverage is asserted, not assumed.

Standing rule 8 — *ask what the generator CANNOT produce* — has returned a defect
or a harness bug **seven times out of seven**, but it is applied by hand, by
whoever thinks to ask. Make it mechanical: each run emits a coverage manifest
(ranges of `n`, `perYr`, calendar horizon, option stacks actually **compared**,
not merely generated) and asserts it against an expected envelope.

"fuzzer5 had never drawn a schedule over 25 years" (§52, found 2026-07-31) would
have been a failing assertion rather than a discovery — and removing that bound
moved the measured divergence rate from 1 in 3,600 to 1 in 290.

*Status: **LANDED, round 16.*** `dos_fuzzer5_test.go` now emits, over COMPARED
cases only, `n=<min>..<max>  horizon=<min>..<max> yrs  perYr={...}  modes={...}`,
and fails the run if the longest compared schedule drops under 40 years, if fewer
than 3 payment frequencies are compared, or if fewer than 4 of the 6
backward-solve modes reach the comparison. Asserted only at `covN >= 200` with no
mode filter active, so an aimed run does not trip it. Measured on seed 44000 at
`FUZZ_N=120`: `n=0..2080, horizon=8..79 yrs, 9 frequencies, all 6 modes`.

### R8. Kill the oracle flake.

A measured 4-9% flake rate poisons every rate quoted from a small sample, and
several current numbers rest on samples of 13-25. The standing plan was majority
voting in every oracle reader (backlog #11).

*Status: **LANDED, round 16 — and the diagnosis was wrong. There was almost no
flake to kill.***

`runDump` already retries **eight times on a fresh process**, so a genuinely
random per-attempt flake of 4-9% would reach the caller essentially never
(`0.09^8 ≈ 4e-9`). Yet the bucket was measuring **10 of 120 (8.3%)**. That
arithmetic does not describe noise, and it was sitting in plain sight for weeks
behind the word "flake".

Re-running all ten by hand: **every one succeeds 5/5 with identical output.**
Nine of the ten return

```
payment <positive>  interest -1.00  paid -1.00
lastdate -88/0/1900  nperiods 0
```

— DOS solved the payment, then its own date arithmetic overflowed (`-88` is the
`shortint` day, 1900 the wrapped year: §55's territory) and it declined to total
the schedule. The tenth carries the same `-1` totals with a **valid**
`lastdate 5/28/2033 nperiods 120`.

Neither is noise and neither improves on retry. The sentinel
`d.paid <= 0 || d.interest == -1 || d.payment == 0` treated all of it as heap
corruption. Now split:

| shape | classification |
|---|---|
| payment > 0, interest = paid = −1, `nperiods 0` or wrapped `lastdate` | **`fz5DateHorizon`** — returned immediately |
| payment > 0, interest = paid = −1, horizon cells VALID | **`fz5NoTotals`** — new class, returned immediately |
| no payment, or a malformed total without the −1 pair | true heap flake — still retried |

**Measured effect.** On seed 44000 the flake bucket went **10 → 0** with
`compared` unchanged (these were never comparable). Across six seeds at
`FUZZ_N=150` the residual true-flake rate is **0-1 per 150 (~0.5%)**, against a
documented 4-9%. The amortization package's gated run also dropped from **~225 s
to ~89 s**, because ~8% of cases had been burning seven wasted oracle spawns each.

**Two consequences worth carrying forward:**

- **Backlog #11 as written is obsolete.** Majority voting is the right tool for a
  *value* flake (two runs, two different numbers). What was actually there was a
  *classification* bug. Any remaining ~0.5% may still be a true flake and is worth
  a second look, but it is not the thing that was poisoning the samples.
- **`fz5NoTotals` is uninvestigated.** DOS answers the payment and refuses the
  totals on a screen whose horizon is intact. Nobody has looked at why, because
  until now it was called noise. Dump it with `PERSENSE_FUZZ_FLAKEDUMP=1`.

Both new dumps are gated behind that env var, not printed by default: `cmd`
contains an `amort_oracle …` string and `paired_regression.sh` greps exactly
that, so emitting it by default would register every one as a NEW divergence.
Rule 7 — never change a harness's default output.

### R8b. Every external call the harness makes is bounded, and the bound is a counted bucket.

*Added round 17, after harness defect #9.*

R8 was about a bucket that swallowed cases and mislabelled them. This is the same
failure one level up: a bucket that swallowed an entire **seed**.

`runDump` execed the oracle with a bare `exec.Command(oracleBin, args...).Output()`.
The DOS engine does not terminate on every screen it is handed — this one is
deterministic and reproduces in isolation:

```
amort_oracle 236979.58 0.1082040000 940 4 b365 exact prepaid inadv plusreg r78 usa \
  loandmy=12.12.2023 firstdmy=12.1.2024 b1072=23197.49 pre=1297:283:26:92.30 \
  targ=1502.57 payhard=6758.34 noterm bdump
```

a 235-year quarterly `noterm` solve whose prepayment series starts at period
1297, past the term. DOS enters a loop writing
`ENGINE ERROR: Bad date passed to Julian function: m=-99` to stdout forever.
`.Output()` waits for EOF, so the test binary waits forever too, buffering the
runaway output in memory with no bound, until `paired_regression.sh`'s
`timeout 900` (or round 17's `timeout 1800`) killed the process.

**A wrapper `timeout` is not a bound — it is a way to lose the measurement.**
Go's testing framework buffers `t.Logf` until the test returns, so a killed
binary emits nothing at all: no ledger, no COVERAGE line, no signals, no
indication that 400 cases went missing. On the pre-fix binary, seed 50102 of the
standing `50100-50139` range did exactly this.

Why it matters more than the lost wall clock: **the screens that hang are not a
random subset.** They are long-horizon backward solves with stacked options —
precisely the frontier the fuzzer exists to sample. Dropping their seeds biases
the surviving population toward benign screens, so every rate computed from a run
that lost seeds is computed on a population that run does not describe. There is
no way to tell from the output that it happened.

*Status: **LANDED, round 17.*** `runDump` now uses `exec.CommandContext` with
`fz5OracleBudget` (20 s — three orders of magnitude above the 99th-percentile
legitimate call, so a slow-but-finite screen is never misclassified), returns
`fz5OracleTimeout` **immediately** rather than burning the other seven retries
(non-termination is deterministic; retrying multiplies the wait by eight — R8's
arithmetic mistake in a new place), and the loop buckets it as
`dos-nontermination` in the R5 ledger. The count is printed **even at zero**, so
a clean run and a lossy one are no longer identical on paper, and a cross-check
fails the test if a timeout recorded in `runDump` does not reach its bucket.

**Measured effect.** Seed 50102 went from *hangs until killed at 1800 s,
contributing nothing* to **completing in 22 s** with `UNACCOUNTED 0` and
`dos-nontermination 1`. Signal sets on seeds that never hit the budget are
byte-identical pre- and post-fix (R9).

The general rule: **every exec, socket read or subprocess wait in the harness
carries an explicit deadline, and exceeding it lands in a named terminal bucket
that is reported whether or not it fired.** An unbounded wait is a silent
attrition channel, and silent attrition is the one failure this instrument cannot
detect in itself.

`long_horizon_sweep.py` already bounds its runs (`timeout=90`), but folds a
timeout into the `("ERR",)` key, which means DOS non-termination is currently
counted as DOS *refusing* the screen. That is conservative for the rate — the
screen leaves the denominator — but it is the wrong name for it, and it should be
split next time the sweep is touched.

### R9. A harness change is gated like an engine change.

Run the paired regression with the **new harness in both trees** so the diff
isolates what changed. A harness that refuses more cases moves signals into the
`DOS solved, Go refused` advisory, which is itself a reported line — so "stricter"
is not automatically "safer".

### R10. A tolerance is scaled to the value it guards, and the scaling premise is written down where it can be checked.

Round 18, defect #10. The rule has three parts and the third is the one that was
missing.

**Scale to the compared value.** An absolute tolerance, or one keyed to a
*different* quantity than the one being compared, is only correct while the two
quantities stay in a fixed ratio. `intTol`/`paidTol` scale to `|dos.interest|`
and `|dos.paid|`; the backward-solve check scales to `|dos.solvedAmt|`. The
terminating balloon was keyed to the LOAN AMOUNT — fine while the balance is
bounded by the loan, catastrophic on a screen that negatively amortizes for 166
years, where the ratio reached 1.57 million.

**Never demand tighter agreement than the same walk's other outputs get.** The
tack is the terminal point of the accumulation that produces the totals. Holding
it to 1e-11 while the totals it comes from are held to 5e-4 cannot be right under
any error model, and the mismatch alone was enough to condemn the constant
without knowing anything about the engine. There is a standing test for exactly
this: `TestFz5TackToleranceIsNoTighterThanTotals`.

**State the premise in the comment, because a premise is falsifiable and a
constant is not.** The old line did say the right thing — *"the tack amount is a
balance, so scale the tolerance to the loan"* — and that sentence is precisely
what let round 18 find the defect in one reading, because a balance that is
1.57e6 times the loan is a visible contradiction of it. A bare
`max(0.05, 1e-5*amount)` with no comment would have survived indefinitely.
**Write the premise down; someone will eventually check it against the data.**

Corollary, and the reason this rule sits at the end of a list of nine about
*computing* things: **an adjudication rule is part of the instrument.** Nine
defects were the harness computing a wrong value. The tenth computed nothing at
all — it read two correct values and returned the wrong verdict. Diffing the
harness's arithmetic would never have found it. **Audit the comparators, not just
the calculations.**

A tolerance change cannot be R9-gated in the usual way: by construction it
changes the signal set, so "byte-identical" is not available. Gate it instead by
**enumerating both populations** — every signal it silences and every signal it
keeps — and asserting in a test that each silenced row is silenced for the stated
reason. `TestFz5TackToleranceScaling` pins ten measured rows in both directions,
including one on the boundary, and fails if the fix stops silencing anything, if
it stops keeping anything, or if a silenced row's relative disagreement exceeds
the slope the fix claims to apply.

### R10 addendum — what the audit of the OTHER tolerances actually found (round 18b)

Defect #10 raised the obvious question: are the rest mis-scaled too? All five
tolerances in `dos_fuzzer5_test.go` were instrumented and measured over the three
standing ranges (120 seeds, 48,000 generated). **None has defect #10's shape.**

The detector took two attempts and the failed one is the instructive part.

**First attempt — separation gap. It did not work.** The idea was that a
mis-scaled tolerance would show its passing and failing populations running into
each other. Validated by restoring the old tack tolerance and re-running: the
DEFECTIVE constant scored gaps of **30,184x and 815,435,001x** — wider than the
fixed one's. Defect #10 is bimodal in ratio space too, because |delta|/tol was
tracking the BALANCE SIZE rather than the agreement quality. A metric that scores
the known defect as healthy is worse than no metric.

**Second attempt — the SPREAD of `tol/|value|`, which works.** A tolerance keyed
to the value it guards demands the same number of significant figures from every
case, so that ratio is flat. The old tack constant demanded ~1e-4 relative of a
small balance and 2.1e-11 of a large one. Measured live on the same seeds:

```
old tack tolerance:  SPREAD 3.2e+07,  2.8e+06
new tack tolerance:  SPREAD 1.4e+00,  1.2e+00
```

Seven orders of magnitude of discrimination, and it needs no knowledge of the
engine, the units, or which constant is "right". Pinned both directions in
`TestFz5ToleranceScalingIsConsistent`, which fails if the metric ever stops
flagging the known-defective constant.

**The audit's finding.** No other tolerance is mis-scaled — but the pooled
headroom is much tighter than any single seed suggests, and that IS a finding:

| tolerance | judged | fail | max passing | min failing | gap | passing within a decade |
|---|---|---|---|---|---|---|
| `balloon:tack` | 4,156 | 51 | 0.595 | 1.02 | **2x** | 6 |
| `totals:interest` | 7,186 | 38 | 0.516 | 1.26 | **2x** | 7 |
| `totals:paid` | 6,542 | 35 | 0.510 | 1.39 | **3x** | 6 |
| `solve:rate` | 382 | 11 | 0.384 | 3.13 | 8x | 1 |
| `solve:amount` | 15 | 2 | 2.7e-6 | 143 | 5e7x | 0 |

Roughly **nineteen cases across the three ranges sit within a factor of two of a
tolerance boundary.** Their HARD/not-HARD classification is decided by the
constant, not by the data — about 15% of the reported residual. This is not a
scaling error and no different constant fixes it: it is the genuine continuum
between a rounding tail and a small real divergence, and a decimal comparison
cannot resolve it at any threshold. **Quote the residual with that band
attached**, and note that the only instrument that can settle those cases is a
bit-level one (R11).

### R11. A solver is verified by its BIT DISTRIBUTION, and the statistic is the SIGN BALANCE.

Round 18b, and the answer to R10's leftover. Where a tolerance cannot resolve the
boundary, stop choosing constants and compare raw bits.

But bit-EQUALITY is the wrong assertion for a solver: two secant iterations from
different seeds with different stop criteria disagree in the last bits routinely,
and a test that failed on that would be noise everyone learns to skip. The
assertion that distinguishes arithmetic from a defect is **the sign balance**.
Independent rounding splits evenly; a systematic conversion or ordering
difference leans. §48 — a last-bits offset on a third of all COLA inputs that
survived every decimal sweep for months — was exactly that shape, and leaning is
what gave it away, not any single large error.

`zzbits_backward_test.go` implements this for `norate` and `noamt`, closing the
backlog's oldest item. First run, 300 cases each:

```
solvedamount (noamt):  300 compared, 300 BIT-IDENTICAL
solvedrate  (norate):  300 compared, 288 bit-identical, 12 differ
                       ALL 12 lean the same way (Go below DOS), p=4.9e-4
                       worst 4 ULP (~1e-16 relative)
```

Two rules fall out of building it:

**Use an exact binomial tail, not a normal approximation with an n>=30 gate.**
The first version had that gate and would have reported nothing: the interesting
biases are small-n by construction, because a solver that is nearly always
bit-exact produces few non-exact cases and those are precisely the ones worth
reading.

**Split the severity (R6).** A significant lean is a true statement about the
arithmetic at any magnitude, so it is always reported — but failing the suite
over a 4-ULP offset, twelve orders of magnitude below anything a user or any
other instrument can observe, is crying wolf, and a red suite everyone ignores is
worse than no test. It fails only when the bias is significant AND materially
sized (>16 ULP); below that it emits
`SIG=ADVISORY:backward_solve_sign_bias` and travels in the record.

And the R1 trap this test had to avoid: the Go side calls `SolveBlankCells`, the
product's shared entry point, **not** `SolveRate`/`SolveLoanAmount`. §58 was
caused by a harness reassembling that exact sequence and dropping the convergence
gate; a new backward-solve harness doing the same would have been defect #7 over
again. Non-convergence is counted as a product verdict, never bypassed.

### R12. A skip is not a pass.

Round 18c. Landed as `testplan/harness/check_skips.sh` plus a documented
`skip_allowlist.txt`, and as a `PERSENSE_REQUIRE_ORACLE` gate on the actuarial
differentials. Recorded here because it was implemented before it was written
down.

A test that skips still prints `ok`. Three actuarial differentials skipped in
every cloud container this project has ever run, for weeks, because the
bootstrap tarball omitted `scripts/` — while the backlog simultaneously
described that surface as *untested*. Both statements were wrong in the same
direction and neither was checkable from a suite that was green.

The rule has two halves, and the second is the one that keeps it honest:

- Run the gated suite through `check_skips.sh`, which fails if anything outside
  the allowlist skips.
- **A test that skips because a DEPENDENCY IS MISSING never belongs on the
  allowlist.** That case gets a REQUIRE gate so it FAILS. Only deliberate
  opt-ins — fuzzers, operator probes, unreachable-condition canaries — are
  allowlist material, and each entry carries its reason.

### R13. An instrument may print only what it has actually read.

Round 19, and the smallest defect in this document with the largest consequence
so far.

`dos_fuzzer5_test.go`'s terminating-balloon message contained the literal string
`"dstatus/astatus outp"`. It read neither field. `outp` is the signature of
TackOnFinalBalloon's **APPEND** arm, and all 27 of §59's cases are the **MERGE**
arm — so the message asserted the opposite of the truth on the entire population
it existed to describe. Round 18 then read its own log back as evidence and
published a root cause and a severity argument built on it; the error survived a
round and was caught only by re-probing the oracle by hand. The parser had both
fields the whole time, and the sibling error branch twelve lines below already
printed them.

The failure mode is specific to constants standing where measurements belong: in
a log, `dstatus/astatus outp` is **indistinguishable** from a reading. Nothing
about it invites checking. A field the harness genuinely cannot obtain should be
printed as unavailable, or not printed — the one thing it must never be is
guessed and formatted to look measured.

This is the same family as defects #10 and #11 (a tolerance premise that was
never checked; a determinism premise that was never measured), and the family
now has three members and a name: **the instrument stating something it has not
established.** R8's counted terminal buckets, R5's ledger and R9's base rates are
all the constructive form of the same rule — print the number, including when it
is zero, and never a word that stands in for one.

Practical form: when adding a field to a harness message, either wire it to the
parser in the same edit or leave it out. A pass over the quoted strings in a
harness diff is a two-minute audit and worth doing whenever a round touches one.

### R14. A solver differential's MATERIALITY threshold belongs in the solver's own acceptance units — a bare ULP count makes the verdict a function of the sample size.

Round 20, and it is R10 landing on the one instrument R10's own tolerance audit
did not reach: a ULP count did not look like a tolerance, so nobody scaled it.

R11 above ends with "it fails only when the bias is significant AND materially
sized (>16 ULP)". **16 what, relative to what?** Nothing. And a significance test
gets more significant with more data while a max-of-N grows with N, so both
halves of that conjunction move the same way as the sample grows. The
consequence, measured:

```
zzbits_backward_test.go, IDENTICAL population, only the case count changed
  300 cases:   12 of 12 non-exact lean below, p=4.9e-4,  worst   4 ULP -> ADVISORY
 1500 cases:   60 of 65 non-exact lean below, p=4.9e-13, worst  83 ULP -> FAIL
```

Nothing about either engine changed between those two lines. A standing gate that
flips verdict on N is not measuring the product; it is measuring how long you
looked. Round 20 found this by making `cases` settable — itself the lesson worth
keeping: **a harness constant that bounds how much you sample is part of the
instrument, and should be adjustable so somebody can ask what happens at 5x.**

**The correct unit for a solver is the criterion the solver itself stops on.**
DOS's `Iterate` accepts the moment its residual is inside
`max(halfpenny 0.005, acc_limit 2e-8 x init)` (AMORTOP.pas:1422-1423, 1485-1490)
and then returns `bestx`, the NEXT extrapolated point rather than the one that
achieved the best residual. So the ULP distance between the two engines' answers
is bounded by nothing except how fast the secant happened to be converging when
it tripped that test. Asserting a ULP bound on it asserts a property neither
engine promises.

`zzbits_backward_test.go` now reprices the loan at BOTH solved rates through the
same closed form and measures the payment gap **as a fraction of DOS's own
band**. A ratio of 1.0 means the port's answer differs from DOS's by exactly as
much as DOS was willing to leave on the table; at or above that it fails. Round
20 measures the worst ratio at **9.1e-09 over 4,447 cases** — eight orders of
magnitude of headroom, and it does not move with N.

A second, sharper assertion comes free from the same repricing: **the port must
not be systematically FARTHER from the root than DOS's early stop.** Verified by
injection, that arm catches a rate perturbation ~45x smaller than the band arm
does (3e-9 relative vs 2e-7).

**And why the `norate` lean is not §48's shape — from the DOS source, not from
inference.** `EstimateAndRefineRate` (Amortize.pas:475) seeds the secant with
`payamt*peryr/amount` floored at 0.02, under DOS's own comment *"first guess -
better high than low"* — a deliberately high seed — then Iterates to the early
stop above. The port's plain path settles its own Newton. The lean is the
stopping rule.

**The asymmetry is the evidence, and it is also where to look next.**
`solvedamount` runs the SAME `dosIterateCore` on the SAME draws and is
bit-identical on every case measured (1,500 in the standing test, 4,500 across
round 20's horizon strata), because `EstimateAndRefineLoanAmount` computes a
closed form first (Amortize.pas:457) and both engines return that value. What is
left over the rate target and only over it is the per-pass
`ComputeTrueRate`/`GrowthPerPeriod` chain the amount target never evaluates. If
the band ratio ever grows, that chain is the first place to read.

### R15. A "covered elsewhere" note is a coverage claim, and it has to be checked like one.

Round 21, and it is the most expensive rule in this document to have learned
late.

`dos_fuzzer5_test.go` skips any case that draws no advanced option, and says so
honestly at the skip site — then adds where the gap is covered instead:

> *"this fuzzer can never report a divergence on a PLAIN loan … Plain-loan
> fidelity is covered by `zzmetafuzz_test.go`'s forward corpus and by the
> committed unit suite."*

The first sentence is a measurement. **The second is a claim about a different
file, and nobody ever opened that file to check it.** `zzmetafuzz`'s forward
corpus is five hand-written screens on the days 1, 8, 15 and 29, and none of them
puts a day-29 anchor's LAST payment on a February. The plain path — the simplest
thing the product does and the shape most real screens have — had no randomized
differential at all.

§62 sat in that gap for the life of the port: a dropped final payment row on any
loan anchored on the 29th/30th/31st whose last payment lands in a February,
worth $2,387 of uncharged interest on a four-row repro. Every headline
amortization figure this project has published came from the generator that
excludes it, and the standing residual did not move when the defect was fixed —
because the instrument producing it cannot see the population the defect lives
in.

This is R13's family — the instrument stating something it has not established —
one level up: not a value printed without being read, but a **coverage claim
asserted without being audited**. Two practical forms:

- **A skip that names its compensating coverage must name a FILE AND A DRAW, and
  somebody has to go and read that draw.** "Covered by the unit suite" is not a
  coverage statement; "covered by <file>, which draws days 1-31 including the
  clamped Februaries" is.
- **Every terminal bucket in a ledger deserves the question "what would a defect
  that only lives in THIS bucket look like, and what would find it?"** R5 made
  the buckets visible and counted. It did not make anyone ask what was in them.
  `skipped-plain` ran at ~3 per 400 cases for rounds; it was printed every single
  time.

The constructive form is `zzplain_differential_test.go`: a randomized
differential for the skipped population, with the draw's own properties ASSERTED
(R7) — it fails if it stops producing 29th-31st anchors or clamped-February last
dates, so a clean rate can never again describe a population that has quietly
lost the interesting case.

---

## 3. How this connects to the convergence number

Items R4-R7 matter more for the headline fidelity figure than any single engine
fix, because **a metric cannot converge past the noise floor of the instrument
that measures it.**

§58 is the demonstration: a case that sat in the residual for three rounds, was
twice proposed for formal acceptance as a known departure, and was never a defect
at all. The amortization residual is small enough now — 8 signals in ~2,300
compared cases on the widened generator — that a handful of harness artifacts is
a material fraction of it, and the long-horizon strata rates (4.5% / 17.6% /
15.4%) rest on samples of 25 / 17 / 13, where one artifact moves the number by
several points.

Before quoting a rate, ask what fraction of the signals behind it have been
individually adjudicated. Through round 16 the answer for the amortization
residual is: most, but not all — and every one adjudicated so far has resolved
either to a named engine defect or to this document.

---

### R16. A terminal bucket that ends in `continue` must say what it ASKED before continuing — an unreached branch is an unmade assertion.

Round 22. R15 said every terminal bucket deserves the question *"what would a
defect that lives only in THIS bucket look like, and what would find it?"*. Round
22 asked it of two instruments and got the same answer from both, which is what
makes this a rule rather than an anecdote: **the question had already been
answered for some buckets and silently skipped for others, and nothing in the
code distinguished the two.**

`dos_fuzzer5_test.go` has four buckets that end in `continue`:

| bucket | asked the port anything? | share of a 400-case run |
|---|---|---|
| `refused` | **yes** — `go_solved_dos_refused`, HARD | ~113 |
| `non-converged` | **yes** — retires vs spurious, adjudicated | ~28 |
| `date-horizon` | **no** | ~34 |
| `no-totals` | **no** | ~4 |

`zzplain_differential_test.go`, one round old and written *by* R15, was worse: it
counted every oracle refusal into a single integer and `continue`d **before
building the port's input at all**, so the port was never run on 14% of the
population. The single most dangerous thing a port can do on a rejected screen —
answer it — went unasked on the plain surface for the whole life of the
instrument.

Neither omission is visible by reading. A `continue` is a complete-looking
statement; nothing marks the difference between *"there is nothing to ask here"*
and *"nobody asked"*. That is the whole failure mode: R5 made the buckets
COUNTED, R15 made them QUESTIONED, and this rule makes the answer RESIDENT IN THE
CODE where the next reader can check it.

**The practical form.** Every terminal bucket carries either an assertion or a
one-line comment saying, in the code, why there is nothing to assert — and the
counter prints at every level including zero (R13/R8). "Nothing to compare here"
is a claim; write it down and it can be argued with.

**Three corollaries, each measured this round:**

- **A refusal is a PAIRING, not a count.** Both engines rejecting the same
  screen is fidelity and should be counted as such; the port answering where the
  oracle refuses is a defect; and the two engines rejecting the same screen with
  DIFFERENT SENTENCES is a real user-visible difference on a cell no differential
  had ever compared (§64). Three outcomes, three counters — not one integer.

- **A comparison whose negative branch cannot distinguish "same" from "both
  unlike a third thing" is not a comparison.** The first version of the refusal
  message check asked `dosSaysNonConverge == portSaysNonConverge`. That scores
  *neither engine says non-converged* as agreement, and it reported **173 of 173
  refusals as same-message when 98 of them carried two different sentences.**
  Classify BOTH sides into the same label set and compare the labels. Caught
  inside the container; it is the shape that has escaped before.

- **A capped sample list must be ERA-AWARE.** The plain differential printed its
  first twelve signals in draw order. Out-of-scope signals outnumber in-scope
  ones roughly 40:1, so on a failing seed the reproducing commands for the
  IN-SCOPE divergences — the only ones that fail the test — could be crowded out
  of the failure message entirely. An instrument whose failure message may omit
  every repro for the failure is not usable. In-scope samples now have their own
  list and print first.

**And the rule that keeps recurring underneath all of it:** the harness is a
second implementation of the product, so it drifts on NAMES. Round 22's per-row
comparison nearly reported **173,246 divergences in its first run** because the
oracle's `prin` column is the principal PAID THIS PERIOD while the port's
`PaymentRecord.Principal` is the balance REMAINING AFTER IT. The oracle's `bal`
is the corresponding field. Two like-named quantities, opposite meanings, and
every row in every schedule "divergent". Read the Pascal, not the identifier.

---

### Instrument defect #14 — `amort_oracle … rows` truncates any row DATE with a single-digit day, and `dumpraw` has had the truth all along (round 23, Phase 2)

Found while trying to localise the seven in-scope HARD cases by row.

`amort_oracle.pas:1190` emits the row's date as `GetTok(Output[i], 1)` — the
FIRST whitespace-delimited token of DOS's rendered screen line. DOS pads a
single-digit day, so the line reads `12/ 9/30 126.42 1987.76 …` and token 1 is
just **`12/`**. Everything after the month is silently dropped.

It is not rare. Measured on the round-22 in-scope HARD cases:

| case | DOS rows | rows with a truncated date |
|---|---|---|
| c1 | 211 | 40 |
| c5 | 366 | **73** |
| c6 | 206 | **103** (half the table) |
| c7 | 361 | 91 |

**The VALUES are fine.** The same emit site takes interest, principal and
balance from `ti-3, ti-2, ti-1` — counted from the END — under a comment that
says in as many words *"Taking them from the end is robust to however the date
tokenizes."* The author knew the date tokenizes unpredictably, made the numbers
robust against it, and left the date field as token 1 anyway. Everything derived
from those values is unaffected; only date comparisons are.

**No oracle change is needed, and none should be made.** `dumpraw` already emits
the whole raw line (`L<i>|<line>`), so the true date has been available the entire
time:

```
L50|12/ 9/30 126.42 1987.76 -1861.34 578806.29 357657.04
```

That matters because `legacy/oracle/**` is carved out of the untouchable rule
only with conditions, ~60 Go exec sites parse these binaries, and none share a
parser. **An instrument gap that can be closed on the reading side should never
be closed by changing the thing being read.**

**The rule.** *A per-row DATE comparison must read `dumpraw`, never `rows`.* And
the general form, which is the reason this is filed as a defect rather than a
footnote: **a field that is extracted POSITIONALLY from a rendered human-facing
line is only as reliable as that line's column widths.** The values escaped
because someone counted from the end; the date did not because someone counted
from the front. When adding any new field to an oracle dump, count from whichever
end is anchored, and say in the comment which one that is and why.

**What this invalidated.** Four of the seven Phase-2 attributions had reported a
"row date divergence" that was entirely this defect. They were caught before
leaving the container by the rule that the harness is a suspect before the engine
is — the divergences appeared as `DOS 12/ PORT 12/9/30`, which is not what a real
date divergence looks like.
