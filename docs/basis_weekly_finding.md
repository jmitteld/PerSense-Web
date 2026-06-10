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

**Original finding (now fixed).** Row counts matched but interest diverged
~0.2% per period (max balance relErr 8.3e-3 biweekly, 1.8e-2 weekly over ~2
years). **Cause:** DOS accrues weekly/biweekly interest on the **actual day
count** between payment dates using the calendar-year denominator (366 in the
leap year 2024), while Go's simple schedule used a **constant per-period growth
factor** `f` derived from `yrdays = 365.25`.

**FIXED 2026-06-10.** `generateSimpleSchedule` now accrues weekly/biweekly
(perYr 26/52) interest as **simple interest on the actual day count**:
`interest = balance * rate * YearsDif(thisDate, prevDate)` on the 365 basis —
the convention the DOS displayed table uses. The change is gated strictly on
perYr 26/52, so the monthly/quarterly/annual 360-day paths (where the constant
factor and the day-based formula are arithmetically identical) are byte-for-byte
unchanged. After the fix, `TestDOSWeeklyBiweeklySweep` matches DOS to ~1.6e-5
across 300 cases (0 value divergences) and is hard-asserted.
