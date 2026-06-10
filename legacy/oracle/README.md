# legacy/oracle — headless DOS source-oracle

This directory compiles the **real** DOS computational units (from
`legacy/src/dos_source`) headlessly and drives them from the command line, so
the Go port can be differentially tested against the genuine engine — not a
re-transcription of it.

This is the Level-3 ("binary oracle") validation tier described in
`docs/fidelity_validation_roadmap.md` §3. Because the oracle *is* the product's
own source, it escapes the shared-transcription-error ceiling that caps every
hand-written Pascal cross-check (`refdata.pas`).

## How it works

The real units (`peTypes`, `peData`, `INTSUTIL`, `AMORTOP`, `AMORTIZE`) are
compiled **unmodified**. Only the GUI coupling is replaced, by two stubs here:

- `Globals.pas` — interface mirrored verbatim from the real unit; the
  implementation drops Dialogs/Controls. `MessageBox` sets `OracleErrorFired`
  and records `OracleLastError` instead of popping a dialog, so a headless
  driver can detect an engine error.
- `HelpSystemUnit.pas` — just the help-code constants the units reference.

Drivers:
- `amort_oracle.pas` — drives the real amortization engine. Prints
  `payment <p> interest <i> paid <t>` (or `ERR <message>`). Args:
  `AMOUNT RATE NPER PERYR [bMONTHS=AMT ...]` (rate as a fraction, 0.12 = 12%).
  The payment is left blank so the engine solves it. Trailing `bMONTHS=AMT`
  tokens add balloons and switch the engine into fancy mode.
- `pv_oracle.pas` — drives the real present-value engine (PRESVALU). Prints
  `pv <value> ...`. Modes: `lump AMOUNT RATE ASOF_MONTHS`,
  `periodic AMTN RATE PERYR NPERIODS [COLA] [ann|cnt]`, and
  `vr AMOUNT PAY_MONTHS NRATES year0 rate0 year1 rate1 ...` (variable-rate /
  PVLfancy: discount through a multi-step rate schedule). COLA is stored
  continuous (`ln(1+yield)`) to match what the DOS GUI feeds the engine.
- `mtg_oracle.pas` — drives the real mortgage engine (`Mortgage.CalculateRows`).
  Modes: `monthly PRICE PCT YEARS TRUERATE [POINTS] [BWHEN BHOWMUCH]` and
  `price PCT YEARS TRUERATE MONTHLY [POINTS]`. RATE is the *true* (continuous)
  rate — fed directly to bypass the APR→true-rate conversion as a confound.
- `link_probe.pas` / `pv_link_probe.pas` — milestone-1 link checks (resolve
  every symbol, no compute).

Conditional flags are the authoritative `-dV_3 -dSCROLLS -dPVLX` from
`legacy/src/win_source/Persense.cfg` — the shipped full-product paths,
**not** ACTU (life-contingency was never compiled in the shipped build).

## Building

**Linux, no root (the agent sandbox)** — fetches FPC via `apt-get download`
and extracts it locally, then compiles. Set `TARGET` to pick a driver:

```bash
legacy/oracle/build_linux.sh                    # -> /tmp/oraclebuild/amort_oracle
TARGET=pv_oracle  legacy/oracle/build_linux.sh  # -> /tmp/oraclebuild/pv_oracle
TARGET=mtg_oracle legacy/oracle/build_linux.sh  # -> /tmp/oraclebuild/mtg_oracle
# (each self-smoke-tests on build; building one target leaves the others intact)
```

**macOS / any host with FPC on PATH:**

```bash
TARGET=amort_oracle legacy/oracle/build.sh
# -> legacy/oracle/build/amort_oracle
```

## Running

```bash
amort_oracle 10000 0.12 12 12
# payment 888.4879 interest 661.85 paid 10661.85
```

## The differential sweep

`internal/finance/amortization/dos_oracle_sweep_test.go` generates randomized
ordinary loans, runs each through both this oracle and the Go
`SolvePayment`+`Amortize`, and asserts agreement to display-rounding. It
**skips automatically** when the oracle binary is absent, so ordinary
`go test ./...` is unaffected. To enable it:

```bash
legacy/oracle/build_linux.sh
go test ./internal/finance/amortization/ -run TestDOSDifferentialSweep -v
# checked 1488, skipped 12, divergences 0
```

Override the binary path with `PERSENSE_ORACLE=/path/to/amort_oracle`.

## Continuous integration

`.github/workflows/ci.yml` runs three jobs on every push/PR:

- **go** — `go build` / `vet` / `test ./...`. The differential sweeps skip here
  (no oracle binary present), so this job stays fast and dependency-free.
- **dos-fidelity** — installs Free Pascal, runs `scripts/build_oracles.sh` to
  compile all three oracles, then runs the `TestDOS*` sweeps against them with
  `PERSENSE_{,PV_,MTG_}ORACLE` pointing at the built binaries. A Go-vs-DOS
  divergence fails the build.
- **refdata-harness** — re-runs `scripts/regen_refdata.sh` so the Level-2
  reference data can't silently drift.

Locally, `scripts/build_oracles.sh` builds and smoke-tests all three at once.

### Known oracle-side flake

The Pascal `New(h)` heap path intermittently returns a zero payment under rapid
process spawning (~9% of spawns); every such case reproduces correctly on a
fresh process. The sweep retries up to 8× and skips only if the oracle never
answers — it is an oracle-side flake, never a Go-side disagreement.

## Status / coverage

- **Amortization (ordinary):** ✅ validated — 1,488 random loans, 0 divergences
  (`amortization/dos_oracle_sweep_test.go: TestDOSDifferentialSweep`).
- **Amortization (balloons):** ✅ validated — single-balloon (600) and
  two-balloon (300) payment solves, 0 divergences, agreement to display
  rounding. Confirms the balloon replace-vs-add (`PlusRegular=false`) convention
  matches DOS.
- **Present value (forward):** ✅ validated — lump (400) and periodic (600)
  streams incl. annual-stepped **and** continuous COLA, 0 divergences,
  agreement to ~9 significant figures (`presentvalue/dos_pv_oracle_test.go:
  TestDOSPVOracleSweep`). Pinned the COLA yield→continuous (`ln(1+yield)`)
  storage convention.
- **Variable-rate / PVLfancy:** ✅ validated — 500 random multi-step rate
  schedules, 0 divergences, ~9 sig figs (`presentvalue/dos_pv_oracle_test.go:
  TestDOSVROracleSweep`). Single-rate VR reproduces a plain lump PV exactly.
- **Mortgage:** ✅ validated — solve-monthly (500, incl. points + balloons) and
  solve-price (300), 0 divergences, ~9 sig figs
  (`mortgage/dos_mtg_oracle_test.go: TestDOSMtgOracleSweep`).
- **Next extensions:** PV backward solves (blank a field, diff vs DOS
  `BackwardCalc`); amortization per-period rows and prepayments/adjustments (the
  `pre[]`/`adj[]` arrays in `amort_oracle.pas`); wiring all sweeps into CI.
