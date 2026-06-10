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

With packing fixed, the **periodic** backward solves now run headlessly and are
**direct-diffed vs DOS** (`TestDOSPVPeriodicBackwardDirectSweep`):

- **Periodic amount (PV-4):** 300 cases, 0 divergences, max relErr 5.8e-10
  (was round-trip only).
- **Periodic to-date (PV-5):** 300 cases, 0 divergences, 0 days
  (was Go-unit-test only — now a direct DOS diff).

Still open: the **lump-block** BackwardCalc path (PV-1 amount, PV-2 date) still
faults inside `Enter`'s `ComputeLumpsumLineValues` even with packing fixed — a
residual distinct from the periodic block (which works) that we could not
localize without a runtime debugger. PV-1 stays validated by round-trip; PV-2
(lump date) is the one PV backward path still only Go-unit-tested.
