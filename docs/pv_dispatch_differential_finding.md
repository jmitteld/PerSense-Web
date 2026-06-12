# PV dispatch — differential vs the real DOS classifier (2026-06-10)

## What was built

The PV field-presence **dispatch** (the classifier that reads which fields are
blank and routes to forward / a specific backward solve / refusal) is now
validated two ways, both new this pass:

1. **Oracle-independent decision table** —
   `internal/finance/presentvalue/dispatch_matrix_test.go`. Enumerates the
   single-row field-presence matrix (lump `D/A/V`, periodic `F/T/P/A/V/C`,
   present-value line `R/O/S`) and asserts the canonical PV dispatch rules
   (forward, PV-1/2/4/5/6 row solves, PV-8 rate, PV-9 as-of, over-specified,
   insufficient). The rules come from the PV spec, not from the port's own code,
   so the assertions are not circular. Runs in ordinary CI.

2. **Dispatch-by-consequence differential vs the real DOS engine** —
   `internal/finance/presentvalue/dispatch_oracle_test.go` +
   `legacy/oracle/pv_oracle.pas` `eval` mode. The same field-presence pattern is
   fed to both the Go engine and the genuine DOS `Enter` dispatch (compiled
   headlessly), comparing the observable consequence: **SOLVABLE vs REFUSED**,
   and the forward value where both solve. Comparing the *consequence* (not an
   internal status byte) is deliberate — it is robust to representational
   differences (DOS treats a lump "date + value" row as forward-computing the
   amount; the Go port labels the same case a PV-1 backward solve — same answer,
   different label, both solvable).

   Scope: the rate+as-of-present region with no screen Sum Value (`cspec="RO"`).
   There the discount context is well-defined and DOS produces a value only via
   the forward path, so its dispatch readback is stable and an invalid screen
   surfaces cleanly as ERR / INSUF / a hard fault (read as REFUSED). The
   rate-solve, as-of-solve and screen-Sum-driven backward cells are already
   direct-diffed by the existing `dos_pv_oracle_test.go` sweeps.

## Result

38 single-row patterns, **0 unexpected divergences**: 6 both-solve (forward
values bit-match), 29 both-refuse, and **3 documented behavioural differences**,
all sharing one root cause below. Forward PVs agree to <1e-4.

## The one behavioural difference found (decision for the client)

In the no-screen-Sum region, **the Go port is more permissive than DOS about a
row's own Value column**:

- **Go** accepts a single row's `Value` column as a backward-solve *target* — it
  will solve the amount or a missing date from the row's own Value
  (`FirstPass` `rowLevelTarget`, `backward.go:406-424`).
- **DOS** solves only from the **screen Sum Value** line. With no Sum Value
  present it refuses these: a lump `Amount+Value` (date blank) returns *"time
  period too long"* (it tries to solve the date against a zero screen total), and
  an **over-specified** row (`Date+Amount+Value`, or periodic
  `From+To+Amount+Value`) returns *"you may specify only 2 of 3 columns"*.

The three differing cells are exactly: lump `A+V`, lump `D+A+V`, periodic
`F+T+P+A+V` — every one carries a `Value` column, which is the test's allow-list
marker. The differential asserts that **any** Go-solves / DOS-refuses case
without a Value column, or any DOS-solves / Go-refuses case, is a failure; none
occurred.

**Is this a bug or an enhancement?** Undecided — surfaced rather than silently
changed, per the project rule. Two readings:

- *Enhancement:* for a single row, the row's Value **is** the present value, so
  letting the user type it in the row and solve the date/amount is a reasonable
  convenience the Go port adds. The numeric result is correct (it round-trips
  through the validated forward path).
- *Fidelity gap:* the original requires the target in the Sum Value line and
  refuses the row-Value form; a user porting a saved worksheet could see Go solve
  where DOS would have shown an error.

Note the Go classifier comments cite `PRESVALU.pas:865/939` believing DOS
supports row-level Value targets; the differential shows the shipped DOS engine
does **not** solve them without a screen Sum Value. The over-specified case is
already noted in the Go code (`backward.go`) as a deliberate soft-warning where
DOS hard-refuses.

**Recommendation:** confirm with the client whether the row-Value-target
convenience should stay (document as an intentional enhancement in
`requirements.md`) or be tightened to match DOS's "target goes in Sum Value"
rule. No numbers change either way — only which screens are accepted vs refused.

## Durability

The decision-table test runs in plain CI; the differential skips cleanly when
the oracle binary is absent and runs in the `dos-fidelity` CI job otherwise. A
future change to the PV dispatch that diverges from DOS (beyond the allow-listed
Value-column cases) fails the build.
