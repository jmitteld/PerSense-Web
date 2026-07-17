# fuzzer2 + fuzzer3 pass across all three sections; PV date-solve fix — 2026-07-16

## What was run

Differential fuzzing (Go engine vs the real DOS oracle) across all three
sections, both the "consistent-rotate" style (fuzzer2) and the new adversarial
edge style (fuzzer3, which hard-fails on **Go solving a case DOS refuses**).

| Section | Harness | Cases | Result |
|---|---|---|---|
| Amortization | fuzzer2 (`TestDOSFuzzer2`) | 2,000 chains | 0 divergences |
| Amortization | fuzzer3 (`TestDOSFuzzer3`) | 6,000 | 0 Go-solves-DOS-refuses, 0 over-block (16 `nper` term-solve boundary value-diverges, logged) |
| Mortgage | value fuzzer (`TestFuzzMortgageVsDOS`) | 4,000 | 0 divergences |
| Mortgage | **fuzzer3 (`TestDOSMtgFuzzer3`, NEW)** | 6,000 | 0 Go-solves-DOS-refuses, 0 over-block, 0 value-diverge |
| Present value | value fuzzers (forward + backward amount/rate) | 4,000+ | 0 divergences |
| Present value | **fuzzer3 (`TestDOSPVFuzzer3`, NEW)** | 6,000 | found 29 Go-solves-DOS-refuses (date solves) → FIXED → now 0/0 |

Two new adversarial fuzzers were added, modelled on the amortization
`dos_fuzzer3_test.go`: `internal/finance/mortgage/dos_mtg_fuzzer3_test.go` and
`internal/finance/presentvalue/dos_pv_fuzzer3_test.go`. Both draw every input
independently and widely (mortgage: ≥100% down, points past the down payment,
balloons up to 2.5× price; PV: an independently-drawn target sum value so a large
fraction of draws are unreachable), and hard-fail only on the one-directional
"Go must not SOLVE what DOS REFUSES" class.

## Mortgage — clean

5,404 both-solved, 596 both-refused, **zero** Go-solves-DOS-refuses and zero
value divergences across 6,000 adversarial cases. The mortgage engine agrees with
DOS on both solvability and value everywhere it was pushed.

## Present value — one real finding, fixed

The PV fuzzer3 surfaced a genuine divergence isolated to the two backward **date**
solvers — lump-date (PV-2, `solveLumpDate`) and as-of (PV-9, `solveAsOf`). For an
unreachable target (a present value above the undiscounted amount ⇒ a date before
the as-of; or far below ⇒ a date centuries out), Go returned an out-of-range date
(e.g. 1596, 2351) where DOS refuses with `"Date"/"As of" computation did not
converge`. The rate, lump-amount, and periodic-amount solves were clean.

### Root cause
DOS bounds the date search to its representable window. In the lump-date Newton
(PRESVALU.pas:915) `if (wdate.y>199) then count:=30` forces non-convergence when
the iterate leaves the window — the daterec year is stored as `year-1900` in a
word, so `y>199` catches year>2099 and, via word-wrap of a negative, year<1900,
i.e. the date must stay within **[1900, 2099]**. The as-of solve (PRESVALU.pas:806)
refuses on `count=10` (budget exhausted) or `asof>maxdate`.

The port had neither: `solveLumpDate`'s Newton converged (|diff|<0.003) to
out-of-range dates before its `count>30` guard could fire, and `solveAsOf` fell
out of its 10-step loop and **accepted an un-converged** as-of.

### Fix (`internal/finance/presentvalue/backward.go`)
- `solveLumpDate`: after each `AddYears`, if the iterate's year leaves [1900, 2099],
  report non-convergence (matching DOS's `y>199` abort, which wins over the
  |diff|<0.003 convergence test).
- `solveAsOf`: track a `converged` flag; after the loop, refuse if the budget was
  exhausted without convergence OR the solved year is outside [1900, 2099].

### Validation
- PV fuzzer3 (6,000): Go-solves-DOS-refuses **0**, DOS-solves-Go-refuses **0** (no
  over-blocking), value-diverge 0.
- Full PV package suite green (existing date sweeps unaffected — legitimate solves
  land near the as-of, well inside the window).
- One Go-internal coverage test (`TestSolveLumpDateLargeStepCapPositive`) had
  codified the NON-DOS behavior (asserting a year-1793 solved date). Verified
  against the oracle (`bk_lump_date 1e8 1000 0.05 → ERR … did not converge`) and
  rewritten as `TestSolveLumpDateFarPastRefusesLikeDOS` to assert the refusal.
  Added `TestSolveAsOfUnreachableRefusesLikeDOS` (oracle-sourced).
- Full `go test ./...` clean except the pre-existing missing-fixture tests
  (win_source DOS.MTG/AMZ/PVL, Help corpus — unrelated, absent in this checkout).

## Meta note (audit convergence)
This pass is a clean example of the convergence policy in
`amort_payment_nonconverge_and_audit_convergence_policy_2026-07-16.md`: two new
independent adversarial fuzzers were added (coverage up), each hard finding was
adjudicated against the DOS oracle (not internal consistency), the one real finding
was root-caused and fixed with an oracle-sourced regression test, and a Go-internal
test that had pinned non-DOS behavior was corrected against the oracle rather than
allowed to block the fix. Zero regressions (green→red) in the process.
