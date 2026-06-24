# Post-mortem — the 365-basis "exact" interest miss

**Date:** 2026-06-19
**Reporter:** client (DOS-domain expert), via amortization screen, 365-day basis
**Severity:** High (visibly wrong schedule for a documented DOS feature)
**Status:** Root cause known and previously documented; fix not yet implemented
**Related docs:** `dos_known_frontier.md`, `discrepancies.md` §8, `dispatch_gaps.md`,
`path_to_99.md`, `basis365_finding.md`

---

## 1. Summary

A client selected the **365-day basis** on the Amortization screen
($100,000, 12%, 360 monthly payments) expecting **true daily interest** — actual
days / 365 per period, iterated, "without the fiction that all months have 30
days." The port instead produced the standard 30/360 result: a closed-form
payment of $1,028.61 and a flat $1,000.00 interest on every clean month. To a
domain expert this is obviously wrong for a true-daily loan.

The underlying capability — DOS's **"Exact"** computational setting — is
**unimplemented end-to-end** in the port. The API hardcodes `Exact: false`
(`internal/api/handlers.go:805` settings block; PV at `:1317`), the amortization
engine never reads `settings.Exact` at all (only `presentvalue/calc.go:132`
does), and the UI toggle is hidden (`cmd/persense/static/index.html:1213`).

The notable part: **this was not an unknown bug.** The exhaustive option-cube
sweep had already surfaced it, the root cause was correctly written down in
`dos_known_frontier.md`, and the response was to quarantine the failing cell and
hide the UI toggle rather than fix the engine. The client then reached the same
broken math through a *different* door (the 365 basis alone) than the one that
had been closed (the Exact toggle).

## 2. What DOS actually does

True daily interest in the original DOS program requires **two** settings on
together — the help text is explicit (`ComputationalSettingsDlgUnit.pas:32`):

> "if 'No' then a standard approximation will be used … If 'Yes' results will be
> 'exact.' **365 DAY MUST ALSO BE SELECTED.** Note: Exact results are non-standard."

The engine keys per-period accrual off exactly that pair (`AMORTOP.pas:625`):

```pascal
if ((df.c.basis=x360) or (not df.c.exact)) and DaysCloseEnough(...) then
    timedif := (date.y - prevdate.y) + (date.m - prevdate.m)/12   { 30/360 fiction }
else
    timedif := YearsDif(date, prevdate);                          { true actual-days }
```

And any non-360 basis is always routed through the iterated schedule engine
(`Amortize.pas:1493`, `… or (not (df.c.basis=x360))`) — "no formula applies,"
exactly as the client said. So:

- **360 basis:** `exact` is a no-op (the first clause is already true).
- **365 + exact OFF:** still the 30/360 monthly approximation (what the port produces).
- **365 + exact ON:** true daily — the client's expectation.

## 3. Why the oracle missed it (~99 confidence notwithstanding)

The DOS oracle (`legacy/oracle/`) and the differential sweeps were genuinely
strong. The miss was not a lack of oracle; it was four compounding gaps in
*what* was compared and *which axes were crossed*.

**3.1 The dedicated "exact" test validated a no-op.**
`perRowFlagSweep(t, "exact", …)` (`dos_oracle_sweep_test.go:591`) sets
`Exact=true` on the Go side and passes `"exact"` to the oracle — but runs on the
**360-day basis on both sides** (`goRowsFlags` hardcodes `Basis360` and never
adds the `b365` flag). On a 360 basis `exact` is inert in DOS, so the test
compared 30/360 against 30/360 and passed trivially. The one combination that
matters — `exact` **and** `b365` together, at the row level — was never written.

**3.2 The cube that did cross 365 × exact compared only the payment number.**
`TestDOSAmortSettingsCube` enumerates all 64 settings cells (incl. `b365 ×
exact`) and a coverage map asserts each cell "was exercised" — the basis of the
99 feeling. But it compares only the *regular payment* (`modalReg` vs
`runOraclePayment`). For an ordinary (non-in-advance) loan, `exact` does not move
the payment — DOS derives it from the same closed-form annuity factor. The
exact-interest error lives in the **per-period interest split and the
un-retired terminal balance**, which the cube never inspected. "Cell exercised"
was conflated with "schedule verified."

**3.3 The one cell where exact moved the payment was quarantined, not fixed.**
The in-advance corner (`b365 × inadv × exact`) diverged 9–30%. The cube's own
comment correctly diagnoses it ("the 'Exact method' setting is unimplemented
end-to-end — the API hardcodes Exact=false and the engine never reads
settings.Exact") and then `continue`s past the cell so it cannot fail, with only
a coarse `cornerMax > 0.30` envelope guard. The root cause was known and
documented in `dos_known_frontier.md`, and the chosen response was suppression
plus a product deferral.

**3.4 The blast radius was mis-scoped, and the safeguard guarded the wrong door.**
The team measured exact's impact *on the payment* and concluded it was small
(a few $/10,000 on clean dates) except for the in-advance corner — so they judged
it minor, hid the Exact toggle, and reasoned the path was "now unreachable from
the product." But the impact on the *schedule* of an ordinary 365 loan is large
and obvious (every clean month flat at $1,000 instead of actual-days interest),
and the client reached it without ever touching the Exact toggle — simply by
selecting the 365 basis and expecting daily interest. The mitigation protected
the entrance that had been examined, not the one the user used.

## 4. Root cause classification

- **Primary (product/engineering):** a real DOS feature (Exact interest) is
  unimplemented and was shelved as a "known bounded corner" while a reachable,
  high-visibility path (365 basis) still leads into the un-exact approximation.
- **Verification:** the test suite crossed the `exact` axis only where it is a
  no-op (360 basis), and where it crossed the meaningful axis (365) it compared a
  quantity (payment) that is insensitive to the bug. The failing cell that *was*
  sensitive was suppressed rather than escalated.
- **Confidence metric:** "99" counted settings cells touched and payment
  divergences, not output-quantity coverage. Green suite, documented bug inside.

## 5. Corrective actions

**Immediate (engineering)**

1. Implement DOS's exact-interest per-period path in the amortization engine:
   route non-360 bases through the iterated schedule with actual-day `YearsDif`
   accrual when `Exact` is set; solve the blank payment by schedule iteration
   (oracle bisection) rather than the closed form. Mirror in PV.
2. Thread an `exact` field from the API request into `Settings.Exact`; stop
   hardcoding `false`.
3. Un-hide the Exact toggle in `index.html` once the path is real.

**Verification (this is the durable fix)**

4. Add a **row-level** 365 × exact regression test (interest split *and* terminal
   balance, not just the payment) against the DOS oracle. Remove the cube's
   `b365 × inadv × exact` quarantine and assert zero divergence.
5. Add a cross-axis coverage assertion so any "exercised" claim is tied to the
   output quantity that can fail (see the testing policy, `docs/testing_policy.md`).

**Process**

6. Adopt the testing & verification policy (`docs/testing_policy.md`) so the
   class of error — vacuous axis coverage, payment-only comparison, and
   suppressed-instead-of-escalated divergences — is structurally prevented.

## 6. Lessons (generalizable)

- A passing test proves only what it *compares* on the axes it actually
  *crosses*. Coverage of inputs is not coverage of the output that fails.
- When a setting is documented to matter only in combination (here: exact ⇒
  "365 must also be selected"), the test must exercise the **combination**, not
  the factor in isolation.
- Compare the **observable the user sees**. The client reads the schedule;
  a payment-only oracle is blind to schedule-shaped bugs.
- A divergence the suite finds must be **escalated or fixed**, never silently
  carved out to keep the suite green. A quarantine is a deferred incident.
- "Unreachable from the product" is a claim about *every* entry path, not the
  one you were looking at. Verify it against the full input surface.
