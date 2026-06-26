# DOS-vs-Go actuarial PV golden comparison

Direct comparison of the Go actuarial present-value engine against the **real**
DOS Per%Sense program, for present-value-with-life-contingency cases. Each DOS
number was captured by driving the original `PerSense.exe` under DOSBox
(headless / Xvfb) on the Present Value screen, marking the periodic row's
contingency in the Ctrl-A actuarial window, pressing F10, and reading the Sum
Value off the rendered text screen.

## The grid (Revision 2 — 4 paths)

Settings for every case: as-of 01/01/2024, True Rate 5.0000 (continuous), basis
360, COLA:Ann, 12/yr. Periodic payment $2,000/mo from 01/01/2024 through
01/01/2029. Actuarial "Today" 01/01/2024. Person-1 DOB 01/01/1959 (MALE table);
two-life adds Person-2 DOB 01/01/1962 (FEMALE table).

| # | Path | Row mark | DOS Sum Value | Go SumValue | abs diff | verdict |
|---|---|---|---|---|---|---|
| 1 | single-life **Living** (s1) | L | 104,258.31 | 104,258.3065 | −0.0035 | PASS (to the cent) |
| 2 | single-life **Dead** (1−s1) | D | 3,696.27 | 3,696.2720 | +0.0020 | PASS (to the cent) |
| 3 | two-life **Both-Living** (s1·s2) | B | 102,761.15 | 102,761.1540 | +0.0040 | PASS (to the cent) |
| 4 | single-life **Living + POD 100k** | L | 147,792.24 | 147,792.2465 | +0.0065 | PASS (POD agrees to ~1¢; mid-year convention confirmed) |

**Identity confirmed:** DOS Living (104,258.31) + DOS Dead (3,696.27) =
107,954.58 = the non-contingent baseline, exactly. Holds in the Go engine too
(104,258.3065 + 3,696.2720 = 107,954.5785).

### Case 4 — Payment on Death (POD): convention confirmed, residual is rounding

DOS itemizes the POD as a single-payment row **"On death 100,000.00 → Value
43,533.93"** and folds it into the Sum (104,258.31 Living + 43,533.93 POD =
147,792.24). The Go `PODValueFunc` term is **43,533.94** (unrounded
**43,533.9402**) — a ~1¢ gap (relErr 2.3e-7).

This was initially flagged as a possible death-timing/discount **convention**
difference. A direct test rules that out: recomputing the POD sum under each
within-year death-timing convention (same 1988 male table, age 65, 5%) gives —

| within-year death timing | POD term | vs DOS 43,533.93 |
|---|---|---|
| start-of-year (k+0) | 44,636.01 | +1,102.08 |
| **mid-year (k+0.5) — Go's** | **43,533.94** | **+0.01** |
| end-of-year (k+1) | 42,459.08 | −1,074.85 |
| exact UDD continuous-force integral | 43,538.48 | +4.55 |

Only the mid-year point-mass convention lands within a cent; every alternative is
off by **dollars to a thousand dollars**. So DOS uses the **same mid-year
convention as Go**, and the ~1¢ residual is ordinary rounding/accumulation noise,
not a logic difference. The opt-in test asserts case 4 to a 2-cent tolerance to
absorb that rounding; the convention itself is confirmed faithful.

Evidence: `outputs/dos_golden/DOS_dead_contingent_3696.27.png`,
`DOS_bothliving_contingent_102761.15.png`,
`DOS_pod_living_147792.24_pod43533.93.png` (+ the `*_inputs.png` actuarial-window
captures), and the original `DOS_living_contingent_result_104258.31.png` /
`DOS_noncontingent_107954.58.png`.

---

## The original case (entered live in DOS)

First direct comparison was a single present-value-with-life-contingency case.

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

### Marking a non-default contingency (Dead / Both-Living) — the exact TUI dance

Ctrl-A opens the **actuarial DATA window** (DOB / table / Today / POD). Marking a
row's contingency is a **separate marking mode** reached by pressing a
contingency letter (`L`/`D`/`N`/`B`/`E`/`1`/`2`) *inside* the data window — that
drops you onto the PV screen with the status bar "L, D, N, E, B, 1, or 2 for each
row · Esc·Done". There you arrow **Right** onto the periodic row and press the
letter to set its mark (it shows as a red letter after the From date, e.g.
`1/ 1/24D`). Esc returns to the data window.

The footgun: **leaving the data window into marking mode discards an uncommitted
DOB**, so DOB must be (re)entered *last*, right before F10. The working order for
a non-default mark:

1. Ctrl-A (data window opens).
2. Press the contingency letter (e.g. `d`) → enter marking mode.
3. `Right` → onto the periodic row → press the letter again to mark it.
4. `Esc` → back to the data window (now blank).
5. `Tab` → into Date Of Birth → type 6 digits `010159` (mask fills slashes).
   For two-life: `Tab Tab` to Spouse DOB, type `010162`. For POD: 5×`Tab` from
   DOB to the "Payable on death" field, type `100000`.
6. `F10` → compute. Sum renders in the Present Value "Value" cell.

The headless rig (`/tmp/run_dos.sh` + `/tmp/persense.conf`, DOSBox 0.74 ARM64
under Xvfb, `import -window root` for screenshots) drives the whole boot→keys→shot
cycle in a single bash call, since the sandbox SIGKILLs lingering high-CPU jobs.
