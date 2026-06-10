# Basis variants per-row validation vs DOS — findings (2026-06-10)

## 365-day basis is monthly-irrelevant

The oracle's `b365` flag produces output identical to the default for **monthly**
loans: the DOS engine effectively keeps monthly amortizations on the 360-day
basis (its per-period interest uses a period growth factor, not day counts, and
the one-period-out first stub prorates to exactly one period on 360). A
365-basis *monthly* loan is therefore not a reachable real-world state, so it is
not swept. The meaningful 365-day path is weekly/biweekly, where DOS
auto-switches to 365 and the day counts genuinely drive the interest.

## Weekly / biweekly — exercised, with a documented day-count gap

`TestDOSWeeklyBiweeklySweep` drives weekly (perYr 52) and biweekly (perYr 26)
schedules through both engines. This required two oracle changes:

- swallow the benign `DA_ChangeTo365` notification ("Changing to 365 day basis
  for weekly/biweekly payments") in the headless `MessageBox` stub instead of
  treating it as a fatal error;
- date the first payment by **days** (`AddPeriod`, 7/14 days) rather than months
  in `SetupLoan` for perYr 26/52.

**Result:** row counts match exactly and the schedules track closely, but a
**small systematic interest divergence** remains:

| | max balance relErr |
|---|---|
| biweekly (≤ ~2 yr) | 8.3e-3 |
| weekly (≤ ~2.4 yr) | 1.8e-2 |

**Cause.** DOS accrues weekly/biweekly interest on the **actual day count**
between payment dates using the calendar-year denominator (366 in the leap year
2024 used by the harness), while Go's simple schedule uses a **constant
per-period growth factor** `f` derived from `yrdays = 365.25`. The two differ by
roughly 0.2% per period, which accumulates into the running balance over a
multi-year weekly schedule.

**Status — surfaced, not yet fixed.** Closing this means accruing weekly/biweekly
interest on actual day counts (the way the Daily path already does) rather than
the constant factor, plus aligning the first-period proration denominator. That
is a change to the central accrual loop in `generateSimpleSchedule`, so it is
presented for a decision rather than applied as part of a testing pass. The gap
only affects the weekly/biweekly frequencies; monthly/quarterly/annual and all
the fancy-option paths are unaffected and remain validated to ~1e-5 or better.

The sweep is wired to **measure** this gap (it reports the divergence magnitude
and asserts only the row count), so it stays green while keeping the gap on the
record and catching any regression in the structural behavior.
