# PV periodic divergence frontier — FOUND & RESOLVED (2026-07-06)

## Summary
Hardening the PV oracle sweep (stop pinning day-of-month, basis, from-offset, and
COLA magnitude) surfaced three pre-existing periodic-PV divergences from DOS that
the old day=1 / fromdate=asof / basis-360 sweep had masked. **All three are now
fixed and validated to the cent against the DOS oracle across every axis.**

## How it was found
New oracle modes `periodic_gen` / `lump_gen` in `legacy/oracle/pv_oracle.pas` drive
the real DOS engine with full control over day-of-month (payments and as-of), basis
(`x365` / `x360` / `x365_360`), signed from-offset, and COLA. Differential sweep:
`TestDOSPVOracleGenSweep` in `internal/finance/presentvalue/dos_pv_gen_test.go`.

## Root causes and fixes
1. **COLA=0 stream starting before the as-of date** (the original report): the
   accumulate branch of `PeriodicSummation` reused `SumFormula(lnf,n)` (the
   since_from anchoring) instead of `SumFormula(-lnf,n)` (PRESVALU.pas:438-447).
   Under-valued by ~2.7x. Fixed in `calc.go`.

2. **Wrong installment count off day-of-month=1 / basis 360**: `estimateInstallments`
   approximated the count as `int(YearsDif_360 * peryr) + 1`. DOS counts with
   `NumberOfInstallments(from, to, peryr, ON_OR_BEFORE)` (PRESVALU.pas:607), a
   calendar walk that respects the day-of-month and is basis-independent. A wrong
   count changed the geometric-sum length → up to ~50% divergence on non-360 bases
   and at non-first days. Fixed `estimateInstallments` to call `NumberOfInstallments`.

3. **Stepped-COLA on non-360 bases**: Go summed stepped COLA with an exact
   per-payment day-count loop; DOS `SummationForSteppedCola` (PRESVALU.pas:269-363)
   uses a **three-period** method with **nominal** (1/peryr) spacing in the middle
   whole-years block and advances anniversaries by a plain year-field increment
   (`inc(coladate.y)`), not `AddYears`. On x360 nominal == actual so the loop agreed;
   on x365 / x365_360 it diverged and COLA compounding amplified it to ~2%. Fixed by
   porting the three-period method into `periodicSumAnnualCOLA`.

4. **Month-end (end-of-month convention) todate**: DOS `NumberOfInstallments`
   takes todate as a VAR parameter and snaps it onto the exact terminal payment
   date; `ComputePeriodicLineValues` keeps that adjusted todate for the valuation
   (PRESVALU.pas:606-608). Go discarded it. When fromdate is a month-end (e.g.
   Feb 28) the end-of-month convention moves later payments to each month's last
   day, so an input todate of Nov-28 becomes Nov-30 and `since = YearsDif(asof,
   todate)` shifts a couple days. Rare (month-end anchor days only) and small
   (<0.1%); surfaced only at 2500-case volume. Fixed by keeping the snapped todate
   in the forward loop (calc.go).

## Current state (all strictly asserted)
- Lumps: 0 divergence on every basis / day / offset.
- Periodic `periodic` (at as-of), `periodic_off` (pre-as-of): 0 divergence.
- `periodic_gen` (day-of-month × basis × offset-sign × COLA≤/≥rate × all modes):
  x360, x365, x365_360 all 0 divergence, max rel err ~1e-9.
- 2500-case sweeps per section (lump, periodic@as-of, periodic pre-as-of,
  periodic_gen, lump_gen): all 0 divergence.
