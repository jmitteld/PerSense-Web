# Actuarial — differential testing against DOS is impossible from this snapshot

> **Update (2026-06-25): the table-data half of this blocker is resolved.** The
> original distribution files `MALE.ACT` / `FEMALE.ACT` (1988 HHS qx tables) were
> recovered from a copy of the original software and are now embedded as the
> DOS-faithful basis: `internal/finance/actuarial/persense1988.go`
> (`Persense1988Male` / `Persense1988Female`), served to the frontend via
> `PERSENSE_1988_*_QX` in `cmd/persense/static/lifetables.js` and selected by
> default. See `TestPersense1988*` for the source-spot-value and Go↔JS drift
> guards. What remains blocked is the actuarial **engine** oracle: the `ACTUARY`
> unit *source* is still absent, so a link-against-units oracle (like
> `pv_oracle.pas`) cannot be built. A runnable DOS **binary** (`PerSense.exe`)
> plus these table files now exists, which makes a *black-box* DOSBox
> differential harness feasible — see `docs/actuarial_dosbox_oracle_plan.md`.
> The sections below describe the original blocker as it stood.

**Conclusion (definitive, with build evidence):** the DOS actuarial engine
*cannot be compiled* from the repository as it stands, so the actuarial /
life-contingency paths cannot be differentially tested against the real DOS
engine the way every other engine now is. This is a **missing-source problem**,
not a porting defect.

## Evidence

1. **No `ACTUARY` unit.** There is no `actuary*.pas` / `actu*.pas` file anywhere
   under `legacy/`. The `uses` clauses reference it only in a commented-out form
   (`//{$ifdef ACTU} ,ACTUARY`).

2. **The actuarial core functions are undefined.** `LifeProb`, `PODValue`,
   `XPODValue`, and `Ondeath` are **called** (PRESVALU.pas:212, 221, 297, 397,
   677, 712, 790, 842; pvltable.pas) but **defined nowhere** in the snapshot.

3. **An `-DACTU` build fails to compile.** Building any PV oracle with `-dACTU`
   added (which activates the `{$ifdef ACTU}` code paths) aborts with:

   ```
   pvltable.pas: Error: Identifier not found "LifeProb"
   pvltable.pas: Error: Identifier not found "XPODValue"
   pvltable.pas: Error: Identifier not found "PODValue"
   pvltable.pas: Error: Identifier not found "Ondeath"
   ... (plus screen/print routines the ACTU table path pulls in)
   ```

4. **The life tables are absent.** The default table names are present
   (`PEDATA.pas:66 actuarialfilename:('MALE','FEMALE')`), but there are no
   `MALE`/`FEMALE` table data files (nor any `*.act` / 1988-HHS-format data) in
   the repository.

So even if the functions existed, the engine would have no data to read.

## What this means for confidence

The actuarial engine is the one section where "faithful to the DOS computation"
**cannot be machine-verified** from these materials — there is no DOS
computation present to compare against. The Go actuarial engine is instead
validated against **first-principles life-contingency mathematics**
(`internal/finance/presentvalue/actuarial_canonical_test.go`), with every
expected value computed independently in the test (lx built from qx, explicit
summation), not via the engine's own helpers:

- pure endowment — agrees to 1e-9
- temporary life annuity — 1e-6
- Payment-on-Death / term insurance — to the cent
- curtate life expectancy, two-life survival composition — verified

That is the strongest available oracle in the absence of the DOS core, and it
confirms the Go actuarial math is internally correct. It does **not** confirm
bit-fidelity to the specific (missing) DOS implementation — e.g. its exact
interpolation of the 1988 HHS tables, or rounding inside `LifeProb`/`PODValue`.

## To unblock (client action required)

To enable DOS-differential testing of the actuarial paths, the client would need
to supply, from an original Per%Sense build tree:

- the `ACTUARY` unit source (or whichever unit defines `LifeProb`, `PODValue`,
  `XPODValue`, `Ondeath`), and
- the `MALE` / `FEMALE` (1988 HHS) actuarial table data files.

With those, an `-DACTU` oracle could be built and the actuarial forward/backward
paths swept against the real DOS engine exactly as the amortization, mortgage,
and present-value engines now are. See also `docs/actuarial_files_to_request.md`
and `docs/actuarial_source_investigation.md`.
