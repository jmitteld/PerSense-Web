# oraclediff — differential testing against an external authority

`oraclediff` generates thousands of random amortization worksheets, runs the
Go engine on each, and compares the result against a pluggable **oracle** —
an independent reference implementation. Any disagreement is reported and
**shrunk** to a minimal reproducer.

This is the tool the fidelity roadmap (`docs/fidelity_validation_roadmap.md`
§3) calls for: a way to validate the port against an *independent authority*
rather than a hand-written Pascal reimplementation, which can only ever be as
correct as its own transcription (the "shared transcription error" ceiling).

## Modes

```
oraclediff -n 5000 -oracle=self                 # Go vs Go — must report 0 (plumbing check)
oraclediff -n 5000 -oracle=mutant               # Go vs a one-period-short variant — proves detection+shrink
oraclediff -n 5000 -oracle=cmd -cmd "<command>" # Go vs an EXTERNAL authority
```

`self` and `mutant` run anywhere and are pinned in CI (`oraclediff_test.go`).
`cmd` is how the real authority is wired in.

## External-oracle contract (`-oracle=cmd`)

For each worksheet the command is run once. It receives **one Worksheet JSON
object on stdin** and must print **one Result JSON object on stdout**.

Worksheet:

```json
{ "amount": 200000, "rate": 0.06, "nPeriods": 120, "perYr": 12,
  "payment": 2220.41, "balloonPeriod": 60, "balloonAmount": 40000,
  "plusRegular": false }
```

- `balloonPeriod` (1-based, monthly only; 0 = no balloon) places a balloon on
  that payment date; `plusRegular` mirrors the DOS "stated balloon includes
  regular pmt" setting (false ⇒ the balloon *replaces* the regular payment
  that period, true ⇒ it is *added*).

Result:

```json
{ "totalInterest": 0, "finalPrinc": 0, "payment": 0,
  "midInterest": 0, "midBalance": 0, "nRows": 0 }
```

- `payment` is the row-1 regular payment; `midInterest`/`midBalance` are the
  interest and remaining balance at row `nRows/2`. Tolerances: totals/balance
  2¢, payment/interest 1¢, `nRows` exact.

## Wiring the legacy authority (`Persense.exe`)

`legacy/src/win_source/Persense.exe` is a **Win32 GUI** program with no batch
mode, so it cannot be driven from stdin directly. Two practical adapters, on a
Windows or Wine/CrossOver host:

1. **Headless Free Pascal driver (recommended).** Compile a small console
   program that `uses` the *actual* legacy computational units
   (`Amortize`/`AMORTOP`/`INTSUTIL`/`PETYPES`/`PEDATA`) — not a reimplementation
   — stubbing only the UI/screen units they reference. Read the Worksheet from
   stdin, drive the real `Amortize`/`RepayLoan` procedures, print the Result.
   This links the genuine ported logic and is the strongest oracle short of the
   GUI itself. (Feasibility depends on how cleanly the computational units
   separate from global screen state; this is the open engineering task.)

2. **GUI automation.** Drive `Persense.exe` under Wine with a UI-automation
   wrapper (enter the worksheet, read the schedule), exposing the same
   stdin/stdout contract. Slower and more fragile, but needs no source build.

Either wrapper, once it honors the contract above, plugs straight in:

```
oraclediff -n 20000 -oracle=cmd -cmd "wine persense_oracle.exe"
```

A non-zero exit and a list of shrunk mismatches then tells you exactly where —
and on what minimal input — the Go engine and the original product disagree.
