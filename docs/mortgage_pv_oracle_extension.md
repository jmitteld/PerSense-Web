# Mortgage & PV oracle extension — coverage and one blocker (2026-06-10)

A follow-on differential pass extended the source-oracle harness to two areas
that were previously only unit-tested (mortgage) or round-tripped (PV).

## Mortgage — two new direct DOS differentials, both clean

### CompareAPRs / crossover (`TestDOSMtgCompareSweep`)

The two-mortgage APR comparison — including the 2-D crossover Newton
(`iterateToFindCrossoverAPRandTime`) — is now driven directly against the real
DOS `ReportComparisonOfAPRs` via a new oracle `compare` mode. That routine's
screen code is commented out in the DOS source, so it runs headlessly; the
crossover APR comes from its `FinalResult` string ("APR's cross at X") and the
crossover time from the PEDATA global `apr_crossover`.

**Result:** 486 randomized two-mortgage cases — 0 always-better/crossover
classification mismatches, full-term APRs match to ~5e-7, crossover APR to
~5e-7, crossover time to ~3e-11 yr. Previously this was validated only against
the single MS_EX5 help example. **No bug — exact parity.**

### GenerateRows / What-If table (`TestDOSMtgGenerateRowsSweep`)

`GenerateRows` is ported from the DOS *dialog* unit (no core computational
counterpart): it steps a field — for VaryRate, in yield/loan-rate space
(`bumpField`, mirroring DOS `CopyAndIncrement`) — and re-solves each row with
`Calc`. Validated end-to-end: every generated row's (true rate, monthly) pair is
fed to the real DOS monthly solver and must match.

**Result:** 750 generated rows (150 bases × 5), 0 divergences, max relErr 1.2e-9.
The rows a user sees in the What-If table are individually DOS-faithful. **No
bug — exact parity.**

## PV — as-of solve direct-diffed; BackwardCalc direct-driving is blocked

The PV **as-of (valuation) date solve (PV-9)** is now direct-diffed vs DOS
(`bk_asof` oracle mode, `TestDOSPVAsOfSolveSweep`) — it uses the FrontwardCalc
Newton branch, runs headlessly, and matches DOS exactly across 160 varied cases.

The remaining PV/VR backward solves that go through DOS `BackwardCalc` — lump
and periodic **amount** (PV-1/PV-4) and **date** (PV-2/PV-5/PV-6) — could not be
driven headlessly: a direct call faults with an access violation inside
`bf.FixPointers` (PVLUTIL.pas:68), the screen-column/record-layout machinery,
even though `peDataInit` populates `blockdata`/`fcol`/`dataoffset` at unit load
and `scripting` is set true. This is the "full screen-column layout" dependency
the original harness deliberately avoided. Status:

- **Amount solves (PV-1/PV-4):** unique solves, validated by round-tripping
  through the bit-identical forward oracle (the solved value, fed to the DOS
  forward, recovers the target) — adequate.
- **Date solves (PV-2/PV-5/PV-6):** validated by Go round-trip / internal
  consistency unit tests, **not** yet directly diffed vs DOS. Closing this needs
  the backup-frame/screen-table layout reproduced or stubbed past the
  `FixPointers` fault — a deeper Pascal-internals task, deferred.

## Net

No new bugs in this pass — the mortgage comparison and What-If table both confirm
exact DOS parity. The single open item is direct-diffing the PV `BackwardCalc`
date solves, blocked on the screen-layout fault and tracked here.

---

## Update — PV BackwardCalc unblocked for periodic solves (record-packing fix)

The `bf.FixPointers` access violation above was **root-caused and fixed.** The
DOS engine reads records by raw byte offset (`bf.FixPointers`, `dataoffset[]`,
disk I/O) assuming Turbo Pascal's **1-byte record packing**, but FPC's `-Mdelphi`
default aligns `real` fields to 8 bytes — so `dataoffset[3]` pointed to byte 13
while the actual `val0status` sat at byte 16 (a probe confirmed: packed
`sizeof(lumpsum)`=27 vs aligned 40). Name-based field access (every
computational path) was unaffected, which is why forward/solve results were
always correct.

**Fix:** build all three oracles with `-CPPACKRECORD=1` — the global
command-line equivalent of `{$PACKRECORDS 1}` (no legacy-source edit, and *more*
faithful to the original TP layout). All pre-existing sweeps still pass
byte-identically, confirming the change touches only the offset machinery.

Packing alone unblocked the periodic solves but the **lump-block** path still
faulted nondeterministically — which led to the actual root cause below.

### Second root cause — AdvancePointer truncated 64-bit pointers

`AdvancePointer` (VIDEODAT.pas:151) — the helper every offset-based access uses —
declares `var px: longint absolute p`, overlaying a **32-bit longint** on the
**64-bit pointer** and computing `resultx := px + x`. On a 64-bit build this
**discards the high 32 bits** of the pointer and leaves them as whatever stack
garbage `theresult` held, returning a corrupt pointer. `bf.FixPointers` then
dereferenced it and faulted. It was *nondeterministic* — the periodic block
"worked" only when the result's stale high bits happened to match the heap
region; the lump block faulted when they didn't. (A line-numbered backtrace
plus a probe that confirmed the in-memory status bytes were correct pointed
straight at `AdvancePointer` rather than the data.)

**Fix:** the build stages a *patched copy* of `videodat.pas` (the staging step
already lets `legacy/oracle/*` override same-named units) that widens the pointer
overlays from `longint` to **`ptrint`** (FPC's pointer-width signed int). The
read-only legacy source is untouched. With it, the entire `BackwardCalc` /
`FixPointers` machinery is stable.

### Result — all seven PV backward solvers now direct-diffed vs DOS, 0 divergences

| Solve | Test | Result |
|---|---|---|
| Rate (PV-8) | `TestDOSPVBackwardSweep` | direct, exact |
| As-of date (PV-9) | `TestDOSPVAsOfSolveSweep` | 160 cases, 0 div |
| Lump amount (PV-1) | `TestDOSPVLumpAmountDirectSweep` | 400 cases, 0 div, 1.6e-9 |
| Lump date (PV-2) | `TestDOSPVLumpDateSolveSweep` | 400 cases, 0 div, 0 days |
| Periodic amount (PV-4) | `TestDOSPVPeriodicBackwardDirectSweep` | 300 cases, 0 div, 5.8e-10 |
| Periodic to-date (PV-5) | same | 300 cases, 0 div, 0 days |
| Periodic from-date (PV-6) | same | 300 cases, 0 div, 0 days |

Every PV backward solver is now validated directly against the real DOS engine.
(The two fixes — `-CPPACKRECORD=1` and the `ptrint` AdvancePointer patch — are
both harness-build changes; neither touches the read-only legacy source, and all
pre-existing amortization/mortgage/PV sweeps still pass byte-identically.)
