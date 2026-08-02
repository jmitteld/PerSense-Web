# Harness policy — why the fidelity harness keeps producing wrong findings, and the rules that stop it

**Created 2026-08-02 (round 16), after §58 became the seventh defect in the same
family.** Companion to `docs/testing_policy.md`, which covers how we test and how
we count confidence. This document covers the *instrument*.

---

## 1. The pattern

Seven harness defects, over five weeks, every one of them the harness computing
something the product already computes correctly:

| # | defect | where the product got it right | write-up |
|---|---|---|---|
| 1-4 | four `cmd/goamort` date bugs | the engine's date layer | §51 and earlier |
| 5 | the oracle driver's option-date blocks assign the year into a Pascal `byte`, so a widened term handed DOS a wrapped year and Go an unwrapped one | — (a faithfulness bug in project-authored Pascal) | §55 |
| 6 | `firstPeriodDate` computes `m := 1 + 12/perYr` in INTEGER division, collapsing every sub-monthly frequency to month 1 | the engine | round 14 §4a |
| **7** | **the fuzzer discarded the backward solvers' `converged` flag and amortized at a rate the product refuses to display** | **`handlers.go:1260`** | **§58** |

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
anything was generated and nothing was compared. **First run surfaced a real
shortfall: 2 of 120 cases (1.7%) reach no terminal bucket.** Under the threshold,
so it does not fail the run, but it was invisible before and is now on the record
as an open item.

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
several current numbers rest on samples of 13-25. Majority voting in every oracle
reader (backlog #11).

### R9. A harness change is gated like an engine change.

Run the paired regression with the **new harness in both trees** so the diff
isolates what changed. A harness that refuses more cases moves signals into the
`DOS solved, Go refused` advisory, which is itself a reported line — so "stricter"
is not automatically "safer".

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
