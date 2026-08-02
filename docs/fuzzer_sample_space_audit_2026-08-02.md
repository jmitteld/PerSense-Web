# Fuzzer sample-space audit — what the instruments can and cannot reach (2026-08-02, round 16b)

**Charge (Nate):** *"audit that we are truly fuzzing across the entire sample
space instead of mistakenly concluding that a limited sample space is our
entirety (and thereby inflating the convergence score)."*

**Verdict up front: we are not fuzzing the entire space, and we never were.**
The generators sample a well-chosen but bounded region, several of the bounds
were silent, and one instrument (the long-horizon sweep) also samples *outside*
the representable space, which inflates the divergence side instead. Every
quoted convergence number is a statement about the sampled region only. This
document makes the region explicit, marks each bound as deliberate/documented or
SILENT, and pins the envelope with tests so it can no longer drift without a
diff (`zzsamplespace_test.go`; the fz5* constants in `dos_fuzzer5_test.go`).

Precedent for why this matters, twice measured:

- `years := 8 + rng.Intn(18)` — no schedule over 25 years was ever generated;
  removing the bound moved the measured rate from **1 in 3,600 to 1 in 290**
  with no code change (§52).
- `ppy ∈ {12,24,26,52}` — a prepayment series slower than monthly had
  probability zero; the unreachable region measured **30% divergent** once
  reachable (round 9).

---

## 1. `dos_fuzzer5_test.go` — the envelope, axis by axis

Weights: each advanced-option block is present with p=0.85 (`present()`);
mode/flag axes are fair coins; the term is stratified 60/20/10/10 over four
bands. All scalar bounds below are now named constants asserted by
`TestSampleSpaceManifest`.

| axis | drawn | CANNOT produce | status |
|---|---|---|---|
| term | 8–300 **whole years** × perYr, stratified; n capped 4000 | **terms < 8 years**; **any non-whole-year term** (n=100, n=361 …) except via the cap | **SILENT ×2** — now pinned (`TestSampleSpaceWholeYearTermLattice`) |
| amount | $25k–<$500k | small loans (<$25k, where half-penny effects are proportionally largest); >$500k | **SILENT** |
| rate | 3%–<14% | 0%, <3%, ≥14% — the project's own bit harness uses a **29%** screen, so DOS's domain is provably wider | **SILENT** |
| payment (`payhard`) | fair × 0.85–1.35 | payments far from fair — and therefore, in the backward modes, **solved cells far from their entered values** (see §2) | documented forward; consequence silent |
| loan date | 2023–2025, all months, day 1–31 clamped | origins outside 3 years; origin-side century boundaries | visible; consequence undocumented |
| first period | one period, or an odd stub in (0, 2 periods] | first payment > 2 periods out (moratorium covers the interior separately) | documented |
| frequencies | {1,2,3,4,6,12,24,26,52} uniform | **`Daily` compounding — never set** (dedicated unit tests only) | **SILENT** |
| balloons | ≤3, each 2–<30% of principal, **sum ≤60%**, months in **first 80%** of term | a balloon in the **last 20%** (TackOnFinalBalloon vs a LATE typed balloon unsampled); dominating balloons | **SILENT** |
| moratorium | first 50% of term | longer | visible |
| prepayments | ≤2 series, start in first 70%, 3–25% of payment flow, nn ≤ 400, stop-overshoot 1-in-6 | **a series STARTING past the entered term** — which is stratum A's one known real remainder (backlog #8): the generator cannot reproduce its own open case | **SILENT, and it matters** |
| adjustments | ≤3, rate 2–<15%, payment fair × 0.75–1.45, first 80% of term; suppressed under inadv (DOS refuses, verified) | **negative or zero adjustment rates** (a DOS-refusal behavior documented at seed 20384 — reached by an older draw, unreachable now); adj in the last 20% | **SILENT** |
| target | fair × 2–25% | out-of-band targets | visible |
| skip | 6 fixed patterns, perYr=12 only | any other pattern or frequency | visible |
| points | 0–<4%; never with `noamt` (documented false-divergence trap) | ≥4% | silent bound, documented exclusion |
| modes | 6 uniform (or `PERSENSE_FUZZ_MODES`) | off-grid `lastdmy` for `non` (documented as a future axis) | documented |
| other screens | — | `payoff=` (backlog #15), `solveballoon=`/`dateballoon=` oracle blocks | known |

**Sub-monthly (24/26/52) × month-anchored options** remains a documented
structural exclusion, blocked on absolute-date tokens (backlog #13). Those
frequencies are sampled only on the basis × mode × points × target × stub axes.

## 2. Backward-mode reach — the subtle half of the payment band

Because `payhard` is drawn as fair(entered) × [0.85, 1.35), a backward solve can
only ever be asked to recover a cell **near its entered value**:

- `norate` on a plain 30y/12/9% screen reaches solved rates ≈ **7.3%–12.7%**
  and nothing outside (`TestSampleSpaceBackwardSolvedCellBand`, closed form).
- `noamt` similarly recovers amounts within a band of the typed one.

Round 15 hit this wall from the other side: its ±200% over-refusal probe had to
draw the payment **independently** (0.05x–6x of amount/n) before a divergent
rate solve was reachable at all. The near-perpetuity family (§58) sits at the
extreme *edge* of the reachable band — `payhard ≈ 1.34 × fair` — i.e. the
fuzzer found §58's screen with roughly the last 2% of its payment draw. A
slightly narrower band and §58's region would have been invisible.

**Recommendation R-A:** add a low-probability wide-payment arm to the backward
modes (e.g. 1-in-8 cases draw pay ∈ fair × [0.3, 3]), gated by a paired
regression per R9.

## 3. `long_horizon_sweep.py` — narrower than its reputation, and leaky

The sweep is quoted as the long-horizon authority, and its strata rates fed the
§54-refactor debate. Its actual space:

| axis | drawn | consequence |
|---|---|---|
| **solve modes** | **forward payment-solve ONLY** — no `payhard`, no `noterm`/`non`/`noamt`/`norate` | **structurally blind to every backward-solver defect.** It could never have gated §57, despite START_HERE §3 naming it "§57's missing randomized gate". That obligation is discharged by the fuzzer5 paired runs (44000-44039 all-modes, FIXED 1 / NEW 0 across §57), which DO sample backward modes at long horizons. |
| options | basis/exact, `adj=`, `pre=` only | no r78/usa/inadv/prepaid/plusreg/points/mor/targ/skip/balloons at ANY long horizon — the B/C/D rates cover a thin option surface |
| day of month | `{1,15,28,29,30}` vs a random month | **emits 30 February ~1 in 25 screens** — DOS stores it verbatim (§51), the port refuses BY DECISION, and every such screen scored as a divergence |
| term | 9 fixed year values {15…420} | interior terms unsampled (coarse, acceptable for strata) |
| origin | 2020–2030 | fine for purpose |

### Round 16b: two sweep-side defects fixed, one goamort defect found

1. **Harness defect #8** (`cmd/goamort`): `parseDMY` printed "Refusing" to
   stderr on an impossible date and then **fell through to the DEFAULT loan
   date (1.1.2024)** — every call site is `if d, ok := …; ok { }`. goamort was
   amortizing a *different screen* than DOS was handed, producing fake
   divergences of >$500/payment. Now `os.Exit(2)` — an impossible date ends the
   run exactly like an unknown token (R3). Valid-input stdout byte-identical
   (13/13 corpus); paired sweep pre/post: **FIXED 0, STILL 10, NEW 0** (the fix
   converts silent wrong-screen output into refusal; no representable screen
   changes).
2. **Sweep scoring**: screens with unrepresentable input dates are now
   classified at generation time and bucketed out of an added HONEST rate
   (they remain in the legacy TOTAL for comparability); port refusals are also
   counted separately per stratum.

### The re-measured long-horizon rates (n=2000 per seed, seeds 913 & 77)

Representable inputs only ("honest"), per stratum, combined:

| stratum | old (n=25/17/13!) | seed 913 | seed 77 | **combined** |
|---|---|---|---|---|
| A ≤2048 | 5.8% | 6/311 (1.9%) | 2/281 (0.7%) | **8/592 = 1.4%** |
| B 2049–2091 | 4.5% | 3/561 (0.5%) | 8/554 (1.4%) | **11/1115 = 1.0%** |
| C 2092–2155 | **17.6%** | 22/171 (12.9%) | 26/172 (15.1%) | **48/343 = 14.0%** |
| D >2155 | **15.4%** | 6/80 (7.5%) | 9/101 (8.9%) | **15/181 = 8.3%** |

Headline honest totals: **3.29%** (913) and **4.06%** (77) — against the 7.81%
the same seed-913 draw reports under the old scoring. **Roughly a third to a
half of the previously quoted long-horizon divergence was the instrument**: 30
February screens plus goamort's wrong-screen fallback.

**What survives is real and concentrated: stratum C — past DOS's 70000-day
Julian ceiling, inside the year byte — at ~14% on n=343.** That number, on a
sample 20× the old one, is the input the §54-refactor pricing needs (with the
option-surface caveat above: forward solves, thin options).

### Round 17 addendum — the term lattice, and why "stratum C" was the wrong label

The row above marks the term axis "coarse, acceptable for strata". **It was not
acceptable, and the audit's own thesis is why.** The sweep draws its term from
nine fixed year values, and nobody had measured which last-years that reaches.
Over 20,000 draws at seed 913:

| stratum | nominal range | **actually sampled** | coverage |
|---|---|---|---|
| A | ≤ 2048 | 2034–2048 | 2026-2033 never sampled |
| B | 2049–2091 | 2049–2090 | 2091 never sampled |
| **C** | **2092–2155** | **2109–2120** | **12 of 64 years — 19%** |
| D | > 2155 | 2159–2450 | 2156-2158 never sampled |

Stratum C is reachable **only through the single `90` lattice point.** So:

1. Every "stratum C = 14%" figure is a rate at ~90-year terms wearing a
   64-year label. This is the audit's own thesis — *an instrument credited with
   a region it cannot reach* — one level below where the audit found it.
2. Worse for the purpose it was being used for: **every stratum-C screen the
   default lattice can draw ends after 2100**, so all of them cross the date
   where DOS's `mod 4` leap rule and the port's Gregorian one part company. The
   Julian-ceiling mechanism and the leap-2100 mechanism were **perfectly
   confounded** — the §54 bucketing START_HERE §3 asked for was not unfinished,
   it was *unidentifiable* with this instrument.

`--years-mode wide` (round 17) adds the points that reach 2092–2108 and
2121–2155. Default is unchanged and byte-identical, so every previously quoted
rate stays comparable; the mode and lattice are printed in the run header.
**Never compare a `wide` rate against a `default` one — they describe different
populations.** With the confound broken, stratum C separates cleanly:

| sub-range | isolates | honest den | rate |
|---|---|---|---|
| C1 2092–2099 | past the ceiling, does NOT cross 1 Mar 2100 | 605 | **1.5%** |
| C2 2100–2120 | crosses 2100 (all the old lattice could see) | 1238 | **12.8%** |
| C3 2121–2155 | crossed it long ago, nears the year byte | 2386 | **12.8%** |

A step function at one calendar date, flat thereafter: **the Julian ceiling
costs the port essentially nothing (C1 ≈ stratum B), and the whole stratum-C
concentration is §54.** See `docs/discrepancies.md` §54 for the source
attribution.

**The transferable lesson: a generator's DRAW LIST is part of its envelope, and
a stratification label is a claim about coverage that has to be measured like
any other.** `zzsamplespace_test.go` pins fuzzer5's bounds; the sweep's lattice
had no such pin, which is why this survived the audit that was looking for
exactly it.

## 4. What the quoted convergence numbers actually cover (scope statements)

- **fuzzer5 widened, ~1 in 290** → stacked-option screens, whole-year terms
  8–300y, $25–500k, 3–14%, payments 0.85–1.35× fair, origins 2023–2025,
  options per the table in §1. **Not**: plain loans (never compared — R5),
  short terms, off-lattice terms, small/large amounts, extreme rates or
  payments, Daily, late balloons, past-term prepay starts, negative adj rates.
- **long-horizon strata** → forward payment solves with basis/adj/pre only,
  under the honest scoring above.
- **PV / mortgage zeros** → separate instruments, not affected by this audit.

## 5. Recommendations, in value order

1. **R-A** (§2): wide-payment arm for backward modes.
2. **Short and off-lattice terms**: extend the term draw below 8 years and off
   the whole-year lattice (e.g. n drawn directly 1-in-4). The manifest tests
   force this to be a reviewed change.
3. **Rate/amount tails**: widen to DOS's accepted domain (rate at least to the
   29% the bit harness already uses; amounts down to ~$500).
4. **Negative/zero adjustment rates** (a documented DOS refusal behavior with
   no current coverage) and **late balloons** (last 20%).
5. **Sweep**: teach `gen()` `payhard` + backward modes so the long-horizon
   region gets backward coverage at all; keep 30-Feb draws but score them in
   their own bucket (done).
6. **Daily mode axis** — smallest, possibly a separate cube rather than fuzzer5.

None of these blocks the current numbers; they bound what the numbers may be
said to cover. The manifest tests exist so that when any bound moves, the audit
and the quoted scope move with it, in the same commit.

---

## 6. Round 18 addendum — the ADJUDICATOR is part of the sample space, and the mode labels were wrong

Round 17's addendum established that a generator's DRAW LIST is part of its
envelope. Round 18 adds the step after it.

### 6.1 The `non` mode is not what four rounds of documents said it was

Every write-up since round 16, and START_HERE's next-action list, described the
`non` backward mode as *"both amount and term blank"*. The generator says
otherwise (`dos_fuzzer5_test.go`, const block):

```go
fz5ModeTerm  // + `noterm`: BOTH n and the last date blank
fz5ModeN     // + `non` + `lastdmy=`: n blank, last date TYPED
```

`non` blanks the period count and **types an explicit last payment date**; the
amount is never involved. It is the ONLY mode that feeds a date into the oracle
command line, which makes it the only mode whose results are downstream of the
harness's own date arithmetic (R2 — `addMonthsFrom`, pinned by
`zzharnessdates_test.go`, so pinned but not absent).

That is not a cosmetic correction. It reverses the mechanism story: round 17
read the `non` enrichment as evidence about *backward solving with two unknowns*,
when the distinguishing feature is *a typed terminal date*. **A mode label is a
claim about what a stratum contains, and it must be read out of the generator,
not out of the previous round's prose** — the same failure as round 17's
"stratum C", one layer up.

### 6.2 An adjudication rule bounds coverage exactly as a draw list does

Harness defect #10 (`docs/harness_policy.md`): the terminating-balloon tolerance
was keyed to the loan amount while the value compared was a balance reaching
1.57e6 times the loan. The generator's reach was never in question — those
screens were drawn, run, and compared. **What the instrument could not do was
return a correct verdict about them.**

So the audit's question — *what can this instrument not reach?* — has a second
half: *and of what it reaches, what can it not correctly judge?* Defect #10 was
invisible to every check in this document, because every check here is about the
draw. The manifest test (`zzsamplespace_test.go`) pins 31 scalar bounds, the term
reach, the whole-year lattice and the backward reach band. It pins **no
tolerance**. `TestFz5TackToleranceScaling` is the first of that second kind and
should not be the last: `intTol`, `paidTol` and the backward-solve tolerances all
carry scaling premises that no test currently states.

### 6.3 What the correction did to the numbers

| quantity | round 17 | round 18 |
|---|---|---|
| standing residual, `50100-50139` | 53 signals | **43** (paired: FIXED 10, STILL 43, NEW 0) |
| HARD-carrying cases | 39 = 1 in 232 | **29 = 1 in 312** |
| the "only defensible frontier" | `non`, x2.40, z=+4.5 | **withdrawn** — real-disagreement rate is 1 in 737 in `non` against **1 in 265 in `noterm`** |

The `non` enrichment was an artifact of defect #10 interacting with the mode:
`non` types a far-future terminal date, which produces the runaway balances, which
blow a loan-scaled tolerance. Round 17 measured the enrichment correctly and
attributed it to the engine.

### 6.4 The rule this produces

**Before naming a frontier, adjudicate a sample of its signals individually.**
Round 17 satisfied rule 9 — it measured the base rates and computed a z-score —
and still reached a wrong conclusion, because rule 9 governs the *denominator*
and this failure was in the *numerator*. A signal count is only evidence if the
signals are real, and that is a separate check from whether the base rate is
right. Twenty minutes of case-by-case reading over 97 signals overturned a
finding that a correct significance test had endorsed.

### 6.5 Still unmeasured after round 18

- `fz5NoTotals`: 18 unique cases dumped over seeds 50100-50119. Re-probing each
  command directly against the oracle, **12 reproduce DOS's `-1.00/-1.00`
  no-totals sentinel, 4 return REAL totals, 2 do not parse.** The four are a
  misclassification — the printed reproducing command produces totals
  deterministically (5 of 5 runs) — so the bucket is not purely a DOS refusal.
  Candidate harness defect #11; not diagnosed.
- The sweep's 90 s timeout still folds into its `("ERR",)` refusal key (R8b).
- Everything in §5 above.
