# Round 14 — the round-trip (inverse-consistency) differential, and what it found on its first run

> **SNAPSHOT of the project doc `claude/round14_roundtrip_inverse_differential_2026-08-01.md`.**
> Round write-ups are append-only history; see `docs/history/README.md`.

**2026-08-01, session J. Short session by design — Nate had a hard stop at 22:45 ET.**
Continues round 13.

---

## 0. What this round was, and what it was NOT

Round 14 opened on START_HERE §3's next action (re-run the two un-run
paired-regression ranges, then quantify §54). **It became a different round
mid-session**, twice, both times at Nate's direction:

1. Nate reframed the priority: rather than keep spending rounds on individual
   stratum-A defects, **put a number on what the deferred date-layer refactor
   (§51 + §54) is actually worth**, since two rounds running have paid a tax to
   route around it. That work is still queued.
2. Then his client asked for a specific instrument — *"each time you check a
   calculation, harden the answer, erase the inputs one-at-a-time, and see that
   you can calculate back to the number you originally had put in"* — and Nate
   chose to build that first, as a **standing gate**.

**Neither of the two paired-regression ranges was run.** One was started and
deliberately killed; see §5.

---

## 1. The client's request, measured against what already existed

The honest answer to *"are we already doing this?"* is **partly, and not where it
counts.**

| what exists | what it asserts |
|---|---|
| `dos_fuzzer5_test.go` `payhard=` + `noamt`/`norate`/`noterm`/`non` | The MECHANISM is exactly the client's description. But at :1494-1513 it compares the port's solved-back value against **DOS's** solved-back value — never against the number originally entered. |
| `TestDOSFancyBackwardAmountRateRoundTrip` (`dos_amortize_dispatch_sweep_test.go:378`) | A real round trip, but on the FANCY path only, from a DOS-solved payment. |
| `TestExactBackwardRoundTripFuzz` (`exact_backward_roundtrip_test.go:174`) | Go-vs-Go self-consistency on the exact path. |

So the project had round-trip coverage in two corners and a mode-machinery that
looked like round-tripping but wasn't. What it did **not** have was the plain
case — an ordinary loan, forward-solved, hardened, each input erased in turn —
as a standing gate. That is the most common real user screen.

**Why fuzzer5 cannot be read as a round trip**, and this is structural rather
than an oversight: it hardens a deliberately perturbed payment,

```go
pay := cents(fair * (0.85 + rng.Float64()*0.5))     // 0.85x - 1.35x fair
```

so the screen it solves back from is not the screen it started from. The
perturbation is there on purpose — it exercises both signs of residual in
`TackOnFinalBalloon` (underpay → negative amortization into a balloon; overpay →
early retirement). Round-tripping it would be meaningless. The new file hardens
the **solved** payment instead, which is what makes the inverse well posed.

## 2. The criterion (Nate's, and it is the right one)

A round trip through a cent-quantized payment cannot be exact, and Nate had
already measured this:

> *"it does lead to rounding differences in the back-solved amount. But this is
> fine just as long as the differential is within the bounds of what DOS solves
> for. Viewed this way, we can process it via an oracle approach."*

So the gate is **not** `|recovered − original| ≈ 0`. It is

```
portRecoveryError <= dosRecoveryError + quantum
```

with **DOS's own inverse error as the yardstick** and `quantum` a per-case
DERIVED floor, not a tuned tolerance: the payment is held to the cent, amount is
locally linear in payment (`dA/dP ≈ A/P`), so half a cent of payment is worth
`A·0.005/P` of principal. Below that, no solver of any quality recovers the
input, and a test demanding better would be reporting arithmetic.

This framing also means a port that round-trips **better** than DOS is flagged as
interesting rather than celebrated — it is differently wrong, not more faithful.

## 3. What it can see that nothing else can — and what it is blind to

**Sees:** the port half of a round trip needs **no oracle**. It is a
self-consistency property, so it runs on screens DOS refuses outright — precisely
the blind spot that left §56's paired-regression `NEW=1` unadjudicable in round
13.

**Blind to §54, and this must be stated loudly:** the port uses its own date
layer in both directions, so it recovers its own input perfectly while still
disagreeing with DOS about February 2100. Only the forward differential sees
that. **The two instruments are complementary because they are blind to different
things; neither subsumes the other**, and the round-trip gate going green must
never be read as convergence.

`CLAUDE.md`'s standing rule applies unchanged and is quoted in the file:
**internal-consistency tests must never drive a behavior change.** When one
fails, the question is *which leg matches the oracle* — resolved against DOS,
never by making the two Go legs agree.

---

## 4. Two findings, and the first one is a harness bug caught before it became a lie

### 4a. `firstPeriodDate`'s integer division — the SIXTH bug in that family

The instrument's very first run reported **five self-inverse failures of up to
1257× the quantum**. Every one was at `perYr` ∈ {24, 26, 52}.

`firstPeriodDate` (`dos_oracle_sweep_test.go:26`) computes `m := 1 + 12/perYr` in
**integer** division, so every sub-monthly frequency collapses to month 1 and
places the first payment **on the loan date**. It also explains why the
DOS-differential half silently adjudicated only 5 of 12 cases — the rest were
skipped on a forward disagreement caused by the same bad date.

This is the sixth bug in the family START_HERE §5 names (*"any date the harness
computes must be computed the way the ORACLE computes it"*) and it has exactly
the shape of round 13's retracted 76% finding. **It was caught before it was
written down as an engine defect.**

v1 therefore draws only frequencies where `12/perYr` is exact. Sub-monthly round
trips need explicit `loandmy=`/`firstdmy=` tokens on both sides — real date work,
in §51/§54 territory — and are the first thing to add.

### 4b. The backward RATE solver degrades at long horizon — a real, new signal

With the term drawn in periods (so `perYr=1` meant terms out to **337 years**),
the rate axis reported **3 of 49 port-worse**, and the gap is not marginal:

```
amort_oracle 178060.37 0.1010640000 130 1 payhard=17995.56 norate
  original 0.1010640000 | DOS back 0.1010640043 (err 4.30e-09)
                        | Go  back 0.1012910812 (err 2.27e-04)     ~53,000x worse

amort_oracle 72632.37 0.0489710000 316 1 payhard=3556.88 norate
  original 0.0489710000 | DOS back 0.0489709894 (err 1.06e-08)
                        | Go  back 0.0497411427 (err 7.70e-04)     ~73,000x worse

amort_oracle 74215.37 0.1420440000 337 4 payhard=2635.48 norate
  original 0.1420440000 | DOS back 0.1420438600 (err 1.40e-07)
                        | Go  back 0.1420505399 (err 6.54e-06)        ~47x worse
```

**DOS converges to ~1e-9 on screens where the port stops at ~1e-4.**

Re-drawing the term in YEARS (2–40, comfortably inside the year byte) and
re-running at 80 cases:

```
drawn 80 | amount compared 80, port-worse 0 | rate compared 80, port-worse 0
```

**So the effect is the HORIZON, not the solver in general.** That is a sharper
result than either "SolveRate is fine" or "SolveRate is broken", and it is the
first measurement of a surface backlog item 5 says has no coverage at all.

It is NOT yet adjudicated as a port defect. The confound is that 130–337 years at
`perYr=1` is §54/§55 territory, so "the solver under-converges" and "the port's
dates are not DOS's dates out there" are not yet separated. **Do not write this
up as a defect until they are.** Note though that DOS recovered to 1e-9 on those
same screens, so whatever it is, it is the port's.

Adjudicability also improved sharply once the date bug and the horizon confound
were removed: **49/60 → 80/80 comparable.**

---

## 5. The paired regression that was started and killed

`paired_regression 44200-44239` under `PERSENSE_FUZZ_MODES=noterm,non` was
started against a reconstructed pre-§56 tree, then **deliberately killed**.

1. **`JOBS=2` on a 2-core box starved the session.** The script's default is
   `nproc-1`; overriding it reproduced round 13's exact failure mode — a `sed` on
   a test file timed out at two minutes. **Do not set `JOBS` on a small box.**
2. Once the hard stop was known, killing it was correct: `paired_regression.sh`
   **writes nothing until it finishes**, so a run landing at the buzzer is worth
   exactly zero while consuming half the machine. Round 15 should start it
   FIRST, at `JOBS=1`, and work while it runs.

**A useful by-product survived: the pre-§56 tree reconstruction is validated.**
Round 13 shipped §56 without a pre-fix tree on the drive, so re-measuring it
needed one. Deleting the arm at `engine.go:1051-1143` reproduces round 13's
documented deltas **exactly** — 1511.65 and 593.09 on the forward cases, and
1511.76 / 2406.82 / 5717.01 on the three backward-solve cases. Round 13's numbers
independently replicate, and the recipe is one `sed`.

---

## 6. Gates

```
gofmt -l .                                          empty, tree-wide
go vet ./internal/finance/amortization/             clean
TestRoundTripAgainstDOSRecoveryError (80 cases)     PASS — amount 80/80 clean, rate 80/80 clean
TestRoundTripPortSelfConsistency (80 cases)         PASS — 80 compared, 0 failed
whole-tree manifest                                 0 differing, 0 cloud-only
md5 on the drive                                    0235af7f6271568e79c9569cb4ae3f97, verified
```

**NOT run, and not claimed:** the full `PERSENSE_REQUIRE_ORACLE=1 go test ./...`
suite, and both paired-regression ranges. The new file is additive and test-only
(one new file, no engine change), so the blast radius is nil — but that is an
argument, not a measurement, and round 15 should run the full gate early.

## 7. Files

| file | change |
|---|---|
| `internal/finance/amortization/zzroundtrip_test.go` | **NEW** — the round-trip differential. `TestRoundTripAgainstDOSRecoveryError` (oracle-adjudicated, Nate's criterion) and `TestRoundTripPortSelfConsistency` (oracle-free, runs where DOS refuses). |
| `docs/history/README.md` | **NEW** — the commit-time snapshot arrangement (Nate's call, 2026-08-01). |
| `docs/history/START_HERE.md` | **NEW** — first commit-time snapshot of the live state doc. |
| `docs/history/round14_roundtrip_inverse_differential_2026-08-01.md` | **NEW** — this file. |

Synced to the SSK drive and md5-verified, 0 rejected.

## 8. Next, in order

1. **`paired_regression 44200-44239` (noterm,non) and `50100-50139` (widened).**
   Still un-run after two rounds. `JOBS=1`. Start them first.
2. **Adjudicate 4b** — solver defect or §54/§55 date artifact? Stratify the
   round-trip draw by horizon and see whether the failure tracks the year-byte
   boundary. Feeds item 3.
3. **Quantify §54** — Nate's original round-14 priority, still queued.
4. **Sub-monthly round trips** — needs explicit `loandmy=`/`firstdmy=` (§4a).
5. **The TERM axis** (`noterm`/`non`) — DOS leaves its answer in
   `h^.nperiods`/`h^.lastdate` rather than a `solved*` echo, so it needs the
   bdump parser. The term is an integer so there is no quantum, but the count
   alone is insufficient: a schedule can land on the right count with the wrong
   final date, so both cells must be compared. Completes the client's "one at a
   time" across all three cells.
6. **`cmd/goamort`: a `default` arm that refuses unknown tokens**, and the
   payment-echo heuristic. Both outstanding from round 13; the token one is a
   one-line change that would have prevented round 13's retraction outright.
