# START_HERE — STATE SNAPSHOT at `bb5e045` (round 42, 2026-08-09)

> ⚠️ **THIS IS A SNAPSHOT, NOT THE LIVE DOCUMENT.** The live `START_HERE.md`
> lives in the claude.ai project (`claude/START_HERE.md`) and carries the full
> work plan, the standing rules, the trap list and the bootstrap recipe. This
> file exists only so a reader with the repo and no project access can see
> **where things stood at this commit**. Refreshed at commit time only (§9).

## Commit state

**HEAD `bb5e045`** — "round 42: the PV screen's advanced options stack the same
way …", parent `0776960`. Working tree clean apart from the untracked
`_to_delete/` scratch directory.

Round 42 committed **eight** files:
`internal/finance/presentvalue/calc.go`,
`internal/finance/presentvalue/advisories.go`,
`cmd/persense/static/index.html`,
`internal/api/frontend_diff_sweep_test.go`,
`cmd/persense/frontend_render_test.go`,
`docs/discrepancies.md`,
`internal/api/zzr42_pv_stacking_test.go` (new),
`cmd/persense/frontend_pv_pod_echo_test.go` (new).

## Where the numbers stand

| surface | figure | round |
|---|---|---|
| **Amortization, STACKED, in scope, SEVEN questions** | **30 in 2,086 = 1 in 70** — bar (1 in 400) missed **5.8×** | r41, carried unchanged |
| — same cases, FOUR questions (the comparability anchor) | 23 in 2,086 = 1 in 91 | r41 |
| Plain, in scope | 0 arithmetic in 108,778 → ≥99.99725% one-sided | r32 |
| PV forward | 29,917 worksheets, 5,095,860 lines, 0 divergences | r29 |
| **PV, re-measured r42 (pristine vs changed, both ways)** | 1,157 table worksheets / 390,564 lines / 1,234 VR worksheets / 5,316 row PVs, **0 divergences, byte-identical** | r42 |
| Mortgage forward | 30,000 cases, 135,853 APR verdicts, 0 | r29 |
| Both backward bit harnesses | 1,500 compared, 1,499 bit-exact, no sign bias | r30, re-run r37/r41/r42 |

🚨 **READ THIS BEFORE QUOTING THE PV ZERO.** `legacy/oracle/build_linux.sh:114`
builds `pv_oracle` with `-dV_3 -dSCROLLS -dPVLX` and **not** `-dACTU`; every
actuarial path in `PRESVALU.pas` is inside `{$ifdef ACTU}`, and
`dos_pv_fuzzer5_test.go` contains no *actuarial*, *pod*, *contingency* or *Act*
token. **The PV screen's life-contingency and Payment-on-Death surface has no
DOS oracle and no differential, and never has had one.** Round 42 found three
defects there (§75, §77, §78). **R47.**

## Gates at this commit

- Full suite `PERSENSE_REQUIRE_ORACLE=1 go test ./... -count=1`: **12 ok, 0 fail.**
  ⚠️ `PERSENSE_FUZZ` **unset**, so the randomized amortization differentials did
  **not** run (R37 — a green gate must name what it did not run).
- `check_skips.sh`: **32 skipping / 32 allowlisted**, no new skips.
- Backward bit harnesses at `PERSENSE_BITS_N=1500`: exit 0, both.
- `paired_regression` **not run** — no amortization-engine file changed this
  round. The PV counterpart arm was run on both trees instead.
- Bootstrap FIX 5 manifest diff: **1 differing hash** (this file, which the r41
  tarball predates), **811 absent, 0 `.go` absent**.

## Open, at this commit

**HIGH, user-visible, untouched for two rounds:** NF-1 (the piecewise adjustment
echo — 35 findings / 845 rows, 100% piecewise) and NF-2 (the snapped-date join).
**Also open:** NF-1b, NF-3/4/5, §60 (twenty-one rounds), §72/§73, §74 (the
oracle is nondeterministic on a malformed `payoff=`), §78, §80, §81, the
2091-2099 band (~1 in 12), seed 50152's hang, `dosport`'s three divergences,
`fancybisect.go:195,253`, and the residual APR class (20 in 1,856 —
⚠️ **uncontrolled probe, do not quote it as a rate**).

**Five decisions still open for Nate:** 3a.11 (the scope key — blocks the frozen
corpus and the router programme), 3a.12 (§73 / `types.DateRec`), 3a.13 (the
mortgage APR day-count), 3a.9 phase 2, 3a.14 (the `set-balloonIncl` default).

## Convergence

**Score 2, eighth round running** (five dimensions, scored as the minimum).
Round 42 moved **dimension 4 from 7 to 6** — the first fall since round 36, and
a knowledge gain rather than a regression: the PV oracle's missing `ACTU` build
is a coverage gap one level below the generators. Live assessment:
`claude/convergence_assessment_2026-08-09_round42.md`.
