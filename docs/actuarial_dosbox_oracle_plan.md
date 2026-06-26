# Plan: black-box DOS actuarial oracle via DOSBox

## Why this exists

The actuarial **engine** cannot be oracle-tested the way amortization, mortgage,
and present-value are, because the `ACTUARY` unit *source* is missing — so a
link-against-units harness like `legacy/oracle/pv_oracle.pas` cannot be compiled
(see `docs/actuarial_oracle_blocked.md`). What we *do* now have is the original
runnable program (`PerSense.exe`) plus its data files (the recovered `MALE.ACT` /
`FEMALE.ACT` tables, `.PEX` example scripts, `HELP.%`, `EXAMPLES.%`). That makes a
**black-box** oracle possible: run the real program under emulation, drive the
actuarial screens, and diff its printed Sum Value / POD against the Go engine.

This is the inversion noted in `docs/postmortem_porting_confidence.md` (Cause 3):
interrogating the running engine beats reading source — and here we have no source
to read, so black-box is the only route to true bit-fidelity on `LifeProb` /
`PODValue` rounding and the table interpolation.

## What `PerSense.exe` is (probed 2026-06-25)

- Real-mode **MS-DOS MZ** executable, Borland Turbo Pascal (`Portions Copyright
  (c) 1983,92 Borland`).
- Uses **Turbo Pascal overlays** — strings `No EMS available - using program file
  for overlay` and `Overlay initialization failed` confirm the TP overlay manager.
- Interactive **text-mode TUI**: the `.PEX` files (e.g. `PRESVALU.PEX`) are ASCII
  screen scripts (`COLOR`, `FRAME`, `WINDOW`, `CURSOR_TO`, `CENTER`, `WRITE`) —
  the program is menu/form driven, not a batch CLI.
- Reads its mortality tables from `MALE.ACT` / `FEMALE.ACT` in the working dir.

Implication: there is **no command-line entry point** that takes inputs and prints
a number. Any oracle must drive the interactive UI and scrape the result off the
text screen.

## Obstacles

1. **No emulator in the sandbox.** `dosbox`, `dosbox-x`, `dosbox-staging`,
   `qemu-system-i386`, `dosemu` are all absent. `apt-get` exists but sandbox
   network is allowlisted; installing DOSBox may require enabling the package
   source. First feasibility gate.
2. **Interactive driving.** Inputs must be delivered as scripted keystrokes
   (DOSBox `autoexec` + a key-injection method, or `dosbox-x` which supports a
   `--keyboard` macro / `IPX`-free scripted input). Each actuarial case = a
   navigation path (open PV screen → set table + DOB + now → set a contingency on
   a row → enter amounts → read Sum Value / POD).
3. **Output capture.** Read the computed value from the VGA text buffer.
   `dosbox-x` can dump the screen (`screenshot`/`INT2F` logging is overkill); the
   practical route is its text-screen capture or piping the program's "print to
   file" path (PerSense has a printer path — `NOPRNCHK.EXE` disables the printer
   check — so "print report to file" may yield a parseable text artifact, which
   is far more robust than screen-scraping).
4. **Determinism / speed.** TUI automation is slow (cycles, key timing). A few
   hundred cases is realistic; tens of thousands (as in the amort fuzzer) is not.
   Scope the sweep to a representative grid, not a brute-force cube.

## Recommended approach (in order)

1. **Feasibility spike.** Get `dosbox-x` installed (preferred over stock DOSBox
   for scripting/headless), mount the `OnesAndZerosSoftware` dir, launch
   `PerSense.exe`, and confirm it boots to the menu under `SDL_VIDEODRIVER=dummy`
   (headless). Deliverable: a screenshot/text-dump proving it runs.
2. **Find the most parseable output.** Prefer the program's **print-to-file**
   report path over screen-scraping (run with `NOPRNCHK.EXE` first to defeat the
   printer check). Confirm a single actuarial calc can be written to a file we can
   read from the host.
3. **One golden case end-to-end.** Script the keystrokes for a single known PV +
   Living-contingency case (e.g. the help example: male, DOB 01/01/1959, now
   01/01/2024, $2,000/mo) and capture its Sum Value. Compare against the Go engine
   *using the recovered 1988 table* (select it explicitly — SSA-2021 is the UI
   default), which removes the table basis as a variable.
4. **Parameterize into a small harness.** Wrap keystroke-script generation +
   output parsing in a Go test (opt-in, like `PERSENSE_FUZZ`/oracle-gated tests),
   producing a per-case Sum Value / POD diff. Start with a hand-picked grid:
   single-life Living/Dead across a few ages; two-life Both/Either; a POD case.
5. **Decompose, don't trust the scalar** (Cause 4). Where the UI exposes per-row
   or per-year detail (or the printed report does), diff those, not just the final
   Sum Value — a contingency-weighting error can preserve the total.

## What this would buy

Closing the actuarial engine's Cause-2 ceiling: today the engine is validated
against textbook-correct references (`actuarialmath`, first-principles), which
proves the math but **not** DOS-fidelity of the specific rounding/interpolation.
A black-box DOS oracle — even at a few hundred representative cases — is the only
thing that converts "mathematically correct" into "matches the original program",
the same bar every other engine now meets.

## Feasibility spike result (2026-06-25): GO — all four pieces proven

A spike ran the full pipeline end to end and extracted a real PV result
(`Total present value: 11,676.24`) from the running DOS program, root-free:

- **Emulator:** native ARM64 **DOSBox 0.74** runs without root. The sandbox has
  no `sudo` but `apt-get download` works; extract the `.deb`s (`dosbox`,
  `libsdl1.2debian`, `libsdl-sound1.2`, `libsdl-net1.2`, `libmikmod3`, plus
  `xdotool`/`libxdo3`) and run via `LD_LIBRARY_PATH`. Headless display via
  **Xvfb**; screen capture via ImageMagick `import`.
- **Boot (the key gotcha):** PerSense is a network-license build that demands the
  dir holding `HELP.%`/`EXAMPLES.%`. `SETTINGS.%` hard-codes the server path as
  `E:\`, so the support dir **must be mounted as drive E:** — mount the folder as
  **both C: and E:** and it boots straight to the main menu. Conf:
  ```
  [autoexec]
  mount c /tmp/persense_run
  mount e /tmp/persense_run
  c:
  PerSense.exe
  ```
- **Input = the `.PEX` scripting language** (plain ASCII) drives the real engine
  directly: `PUT_NUMBER value,col,row` → `CALC PVL|INV` → `GET_NUMBER result,col,row`
  → `WRITE result`. The column map matches the Go PV model (`sumvaluecol=14`,
  `tratecol=11`, `pamountcol=7`, `asofcol=10`, …). Stock scripts are the working
  template; a fully-unattended *custom* PEX needs its exact header/terminator
  syntax nailed (the one remaining fiddly bit).
- **Output = print-to-file**, not screen scrape: `strings` shows
  `Path for word processor output:` / `Path for LOTUS spreadsheet output:`. Drive
  a PEX that computes, export a report file, parse it.
- **Golden cases are built in:** `EXAMPLES.%` contains worked living-contingency
  examples ("Reverse mortgage, payable for life") with known DOS answers — ideal
  first comparisons (select the 1988 table on the Go side to match the basis).

**Recommended harness shape:** per case, emit a self-contained `.PEX` that `PUT`s
inputs, `CALC`s, and exports a report file; boot DOSBox headless with the C:+E:
mount; parse the file; diff against the Go engine. This same oracle can replace
`NoopOracle` in `internal/finance/presentvalue/property_fuzz_test.go` (the
`Oracle` interface is already stubbed for exactly this), turning the PV property
fuzzer into a live differential test too.

## Status

Feasibility: **proven (GO)**. Tables-first work (embedding the recovered 1988
tables, keeping SSA-2021 as default, the validation + Go↔JS drift-guard tests) is
complete. Remaining follow-on, in order: (1) nail the unattended `.PEX` syntax;
(2) validate one stock `EXAMPLES.%` living-contingency case against the Go engine
on the 1988 table; (3) wrap into an opt-in Go harness and run a representative
grid; (4) extend the oracle to emit **per-row** PV so sweeps can diff trajectories,
not just `SumValue` (today's `dos_pv_oracle_test.go` sweeps compare the total
only — a confirmed Cause-4 gap).
