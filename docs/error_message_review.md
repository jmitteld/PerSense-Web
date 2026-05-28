# Per%Sense Web — Error Message Review

**Scope:** the user-facing error and warning messages produced by the
Go port — across `internal/finance/{amortization,mortgage,presentvalue,
actuarial,interest}`, the `internal/api` request handlers, and the
structured `FieldError` layer — compared against the original DOS
message strings in `legacy/src/dos_source`.
**Method:** extracted all 90 DOS `MessageBox`/`ShowMessage`/`MessageDlg`
strings and the ~80 user-facing Go `fmt.Errorf` / `newFieldError` /
`Warnings` strings, then evaluated each against a clarity rubric and
checked for coverage gaps in both directions.
**Date:** 2026-05-28.

---

## Verdict: the bar is met, and comfortably exceeded

The port did not merely translate the DOS messages — it rewrote them to
a markedly higher standard. The DOS originals are terse and frequently
cryptic; the Go versions consistently follow a strong three-part
pattern: **name the field → say why → give the fix (often with an
example)**. The API layer additionally introduces a structured
`FieldError` (code + block + row + field list) so the frontend can
highlight the offending cell — a capability DOS never had.

### Rubric

A good message in this domain should:

1. **Name the field(s)** the user must act on, using the on-screen label.
2. **Explain why** the calculation cannot proceed (not just "error").
3. **Give a concrete remedy**, ideally with an example value.
4. **Distinguish severity** — a hard error (blocks the result) vs. a
   soft warning (result still computed, but check something).
5. **Be readable at a glance** — front-load the action; keep the
   sentence count low.

The Go messages score well on 1–4 almost everywhere. The main
opportunity is item 5 (a handful run long) and minor stylistic
consistency.

### Representative before / after

| DOS | Go port |
|---|---|
| `Insufficient data on screen - no can be calculated.` | `there is not enough inputs to solve this present value. Fill in the Rate and the As-of Date, and complete at least one payment row (a single payment needs a Date and an Amount; a periodic payment needs From Date, To Date, Pmts-Yr and an Amount)` |
| `Too many unknowns.` | `there is more than one missing field on the screen, so Per%Sense cannot tell which one to solve for. Leave exactly one cell blank — the field you want computed — and fill in all the others` |
| `Your dates are out of order.` | `1st Pmt Date is after Last Pmt Date. Make sure 1st Pmt Date comes first, or clear one of the two dates and let Per%Sense derive it.` |
| `Rate too small to determine duration of extra payments.` | `Pmt Amount is too small to pay off the loan — it does not even cover the interest, so the loan would never amortize. Raise the Pmt Amount, or enter # Periods directly.` |
| `Cannot compute Amount Borrowed when using Targ Principal Reduction.` | `Amount Borrowed cannot be solved while a principal reduction Target is set — the Target needs a known loan amount to work from. Clear the Target, or enter Amount Borrowed directly.` |

---

## Improvement opportunities

### 1. A few messages are too long to scan under pressure

Several present-value backward-solve errors run three to four sentences
(e.g. `presentvalue/backward.go:682`, the opposite-signs explanation;
`backward.go:715`, the date-stall message). The *content* is excellent,
but the cognitive load is high at the moment of failure. Recommended
pattern: keep the lead sentence (what happened) and the closing remedy
(what to do); move the middle "why discounting cannot flip a sign"
reasoning into expandable help text the UI can reveal on demand.

### 2. Stylistic inconsistency in capitalization / voice

Amortization and mortgage messages are sentence-cased and lead with the
on-screen field name ("Pmt Amount is too small…"). Present-value and
interest messages are lowercase sentence fragments ("the present value
cannot…", "the calculation overflowed…"). Both are clear, but the
capitalized, field-named style reads as more finished and should be the
house standard. This is cosmetic and low-risk to normalize.

---

## Coverage gaps (DOS messages with no Go equivalent)

Three were identified. Two were genuine and have now been wired; one
turned out to already exist.

### Wired — high-rate sanity warning ✓

DOS warned `You entered an unusually high Loan Rate.` when a Loan Rate
above 20% nominal was typed (almost always a units slip) — see
`MortgageScreenUnit.pas:222`, threshold `0.19835162342` (the
true-rate form of 20% nominal). The Go port had **no equivalent
anywhere**.

Added as a soft `Warning` (not a hard error, matching DOS, which let
the computation proceed):

- `mortgage.Calc` — `internal/finance/mortgage/mortgage.go`, threshold
  `unusuallyHighTrueRate = 0.19835162342` (true rate, ported verbatim).
- `amortization.Amortize` — `internal/finance/amortization/engine.go`,
  threshold `unusuallyHighRate = 0.20` (nominal). DOS had no such
  warning on the amortization screen; extended here because a typo'd
  rate is equally damaging there. Appended *after* schedule generation
  so it survives the result reassignment by the schedule generators.

Both fire only on a **user-entered** rate (status `InOutInput`), never
on a rate the engine solved. Wording upgraded from the terse DOS string
to the house style, echoing the entered rate:
`"Loan Rate of about 25.00% is unusually high — double-check it was
entered in percent (for example 6 for 6%, not 0.06 or 600)."`
Tests: `mortgage/highrate_warning_test.go`,
`amortization/highrate_warning_test.go`.

### Wired — actuarial "beyond life span" error ✓

DOS errored `date of lump sum payment N is beyond life span.` when
solving a life-contingent payment's amount whose date is so far out that
survival probability is ~0 (the value gets divided by that probability)
— see `PRESVALU.pas:873-883`. The Go port (`presentvalue/backward.go`,
`solveLumpAmount`) was **silently skipping the division** when the
probability was at or below `Teeny`, returning a wrong, un-adjusted
amount.

Now errors with an explanation:
`"single payment line N is a life-contingency payment dated beyond the
life table's horizon — the survival probability is effectively zero, so
the Amount cannot be solved (the value would be divided by a probability
of zero). Use an earlier Date, or check the Date of Birth and life-table
settings."`
Test: `presentvalue/lifespan_error_test.go` (beyond-horizon error +
in-horizon control).

### Already present — weekly/biweekly basis notice ✓

DOS showed `Changing to 365 day basis for weekly/biweekly payments.`
(`Amortize.pas:297-303`) and switched the day-count basis. This is
already ported in `internal/api/handlers.go:717-725` (coerce 360→365 for
`PerYr` 26/52) with the notice at `handlers.go:1075`
(`"Switched to a 365-day basis for weekly/biweekly payments."`). No work
needed.

---

## Already-ported DOS messages worth noting

These DOS advisories were checked and confirmed present in the Go port:

- Terminating-balloon adjustment notice (`amortization/engine.go`).
- Over-determination warning "value already determined by data above"
  (`presentvalue/backward.go`).
- "There must be at least two regular payments" minimum.
- All convergence-failure messages (rate / APR / "as of" / date solves).

---

## Summary

The error-messaging work is a strength of the port, not a weakness. The
recommended follow-ups are incremental polish rather than fixes:
shorten the longest present-value messages and standardize the
present-value/interest messages onto the capitalized, field-named
style. The two real coverage gaps (high-rate warning, beyond-life-span
error) are now closed and tested.
