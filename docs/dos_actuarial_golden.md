# DOS-vs-Go actuarial PV golden comparison (first direct fidelity test)

First direct comparison of the Go actuarial present-value engine against the
**real** DOS Per%Sense program, for a present-value-with-life-contingency case.
The DOS number was captured by driving the original `PerSense.exe` under DOSBox
(headless / Xvfb) on the Present Value screen and reading the Sum Value off the
rendered text screen.

## The case (entered live in DOS)

| Field | Value |
|---|---|
| Periodic payment | $2,000.00 / month |
| From / Through | 01/01/2024 → 01/01/2029 (12/yr) |
| COLA | blank (0) |
| Contingency | **L** (Living) on the periodic row |
| Present Value: As of | 01/01/2024 |
| Present Value: True Rate % | 5.0000 (continuously-compounded) |
| Actuarial (Ctrl-A): Date Of Birth | 01/01/1959 (age 65) |
| Actuarial table | MALE (the recovered 1988 HHS MALE.ACT) |
| Actuarial "Today" | 01/01/2024 |
| Payable on death | 0 |
| Settings (status bar) | COLA:Ann  basis 360  centurydiv 1950  12/yr |

## Result

| Quantity | DOS | Go | abs diff | rel diff |
|---|---|---|---|---|
| Non-contingent PV (baseline) | 107,954.58 | 107,954.58 | -0.0015 | - |
| **Living-contingent Sum Value** | **104,258.31** | **104,258.3065** | **-0.0035** | -3.36e-08 |

**Verdict: PASS** — agree to the cent. The Go engine (LifeProb + survival-weighted
PV summation, using `actuarial.Persense1988Male()`) reproduces the real DOS
actuarial Sum Value.

Evidence screenshots: `outputs/dos_golden/DOS_living_contingent_result_104258.31.png`,
`DOS_noncontingent_107954.58.png`, `DOS_actuarial_window_inputs.png`.

## Reproduce

Go side (opt-in test):
```
PERSENSE_GOLDEN=1 go test ./internal/finance/presentvalue/ -run TestDOSActuarialGolden -v
```
(`internal/finance/presentvalue/dos_actuarial_golden_test.go`; env-gated so it
never runs in the normal suite.)

DOS side (DOSBox rig, sandbox): boot `PerSense.exe` with the program dir mounted
as **both C: and E:** (SETTINGS.% hard-codes the support path as E:\), reach the
Present Value screen (Alt-P), enter the periodic row, drop into the bottom block
(Home then ~8×Down) for As-of + True Rate, press **Ctrl-A** to enter the
actuarial marking mode, press **L** then Esc, fill DOB (2-digit year: 1/1/59),
confirm table MALE and "Today", then **F10** to compute. The Sum Value renders in
the Present Value "Value" cell.

Note: there is no PEX/CLI path for a life contingency — `act` is per-row state set
only through the Ctrl-A actuarial window, so the comparison must drive the TUI.
