This project is a Golang port for Persense that is meant to be web-enabled.

The goal is to faithfully reproduce the financial logic of the original DOS
Per%Sense application (the authority for all calculations) behind a REST API and
a single-page web UI. See `CLAUDE.md` for the architecture and porting rules.

## Known differences from the DOS original

The port is validated to zero divergence against the real DOS engine across an
extensive differential test suite (see `docs/dos_known_frontier.md`). There are a
small number of places where the port **intentionally** differs from DOS because
the DOS behavior is a confirmed bug that produces a financially-nonsensical
result. In each case the port keeps the financially-correct answer and does not
reproduce the bug.

### 1. Balloon dated ON the first payment date, interest-in-advance

When a loan is set to compute interest **in advance** (annuity-due) and a
**balloon** is dated exactly on the **first payment date**, DOS's schedule
generator (`RepayFancyLoan`, AMORTOP.pas) consumes the balloon through a dead
initialization path (`firstd`, which is assigned but never read). The result is
that DOS **inflates the solved regular payment as if the balloon existed, but
never applies the balloon to principal and never collects it** — the balloon
amount is absent from the schedule and from the reported total paid. A borrower
would be charged a higher payment for a lump sum that never reduces their balance.

**The port's behavior:** it applies the balloon on that date, collects it, and
retires the loan correctly — producing a slightly lower total interest than DOS.
This is guarded by `TestDOSInAdvanceFancyFuzz`, which asserts the port's schedule
retires to zero on these cases rather than matching DOS.

Related, and NOT a divergence: a balloon dated strictly **before** the first
payment date is rejected as an error by both DOS and the port; a balloon on the
first payment date with interest **in arrears** matches DOS exactly.

### 2. Date-only (AO7) / payment-only (AO6) adjustment followed by a later balloon

A date-only or payment-only rate adjustment combined with a balloon dated after it
triggers a display/print-path state-corruption bug in DOS that retires the loan
early on a bogus date with roughly half the interest. The port amortizes the loan
correctly to term. Details and the instrumented root-cause analysis are in
`docs/dos_known_frontier.md`.

For the complete, continually-updated record of DOS-vs-port differences — including
those that have been reconciled — see `docs/discrepancies.md` and
`docs/dos_known_frontier.md`.
