# Per%Sense — Fresh UI-to-Engine Test Report

**Prepared for:** client-meeting readiness review
**Date:** 2026-06-24
**Method:** Black-box, user-perspective testing. Every screen was driven live in a real
browser against the running `persense` binary (localhost:8080) — typing into the actual
grids, clicking the actual buttons, and reading the rendered results and error messages
exactly as a user (or the client) would. Each headline number was cross-checked against
(a) the app's own in-product Help worked-examples and (b) the **real DOS source engine**,
rebuilt from `legacy/src/dos_source` into command-line oracles (`amort_oracle`, `pv_oracle`,
`mtg_oracle`) so figures could be confirmed to the cent against the financial authority.

**Severity key:** **S1** wrong number a client would catch · **S2** misleading/​confusing
behavior a client would question · **S3** polish/cosmetic.

---

## Bottom line

The financial engine is in good shape. Across Mortgage, Amortization, Present Value and
the Actuarial (life-contingency) module, **every valid worked example reproduced to the
cent** against both the Help documentation and the DOS engine. The signature
"fill-in-the-blank" backward solves (reverse-solve price, solve loan amount, solve rate,
IRR, contingent-amount) all returned the documented answers. **No S1 defects were found** —
there is no case in this pass where the app prints a wrong number for a sensibly-entered
problem.

The risks that remain are about **presentation and stale-state**, not arithmetic — and
two of them are exactly the kind of thing a sharp client will poke at. They are listed
below in priority order, each with a concrete reproduction.

| ID | Area | Severity | One-line |
|----|------|----------|----------|
| A1-DOC | Amortization Help | **S2** | In-app Help Example 1 still shows the old Windows numbers; the app (correctly) computes the DOS numbers, so the example "doesn't match the program." |
| A2 | Amortization Advanced Options | **S2** | Adding a balloon while a core field is in solve-mode silently re-solves it (rate 6%→7.0069%) and total interest jumps — looks irrational. |
| M6 | Mortgage Compare APR | **S2 / S3** | Silently compares two non-comparable loans (different terms) with no warning; duration text reads "1 years, 1 months." |
| C1 | Clear All dialog | **S3** | The native confirm dialog hard-blocks the page (a UX/automation nuisance, harmless to a human). |

---

## Findings (priority order)

### A1-DOC — In-app Help Example 1 contradicts the program  ·  S2  ·  *most likely "gotcha"*

> **RESOLVED 2026-06-24** — `help.html` Amortization Example 1 updated to the DOS values
> ($731.98 / $163,513.84) with a short note on the odd-first-period adjustment; Example 1b
> corrected to show its natural-period $733.76 and explain the contrast. Verified in a fresh
> build. **Action for you:** rebuild the `persense` binary so the embedded help ships the fix.

**What the client will see.** Open **Help → Amortization → Example 1**. It says the payment
on a $100,000, 8%, 30-year loan (loan 02/12/2024, first payment 03/01/2024) is **$733.76**
and total interest **$161,499.77**. Enter those exact inputs in the program and you get
**$731.98** and **$163,513.84**.

**Why it happens — and why the engine is actually right.** The 19-day short first period
makes the authoritative DOS engine *augment* the regular payment so the loan still
amortizes; the old Windows help screens showed the un-adjusted payment. The Go port
correctly follows DOS — confirmed directly against the rebuilt DOS oracle:
`amort_oracle 100000 0.08 360 12 loandmy=12.2.2024 firstdmy=1.3.2024 → payment 731.98,
interest 163,513.81`. This is documented in `docs/discrepancies.md §7`; the engine and its
tests were updated, **but `help.html` Example 1 was never updated** and still teaches the
$733.76 figure (and even reasons about "$666.67 a standard month").

**Impact.** High optics risk. A client who follows the program's own example and gets a
different answer will read it as a bug — even though the program is the correct one.

**Fix.** Update `help.html` AM Example 1 (and 1b) to the DOS figures, or add a one-line note
that an odd first period adjusts the payment. (Tidy the known 3¢ DOS-vs-Go interest rounding,
§7, while there.)

---

### A2 — Adding an Advanced Option silently re-solves a core field  ·  S2

> **RESOLVED 2026-06-24 (DOS-faithful).** Confirmed against the DOS source that this is the
> *correct* DOS dispatch: when the payment is given and the rate is blank, DOS solves the rate
> (Amortize.pas:1338), including any balloon — so the 7.0069% / +$50k result matches DOS and
> the engine was left untouched. The surprise is a web-only artifact (a leftover green "rate"
> output gets re-solved). Fix is additive and changes no numbers: the Amortization screen now
> shows a "Note:" advisory whenever Rate or Amount is solved while an Advanced Option is in
> effect — e.g. "Loan Rate was computed as 7.0069% to fit this loan including the advanced
> options in effect…". Frontend-only (`index.html`); verified the advisory renders and engine
> output is unchanged. **Action for you:** rebuild the binary to ship it.

**Reproduction (verbatim from the live session).** On Amortization, solve a loan's rate
(enter Amount 250,000, Payment 1,498.88, leave **Rate blank** → it computes 6.0000% as a
green output — the standard Help "solve rate" flow). Now open **Advanced Options**, add a
**$50,000 balloon** dated 01/01/2034, and click Calculate.

**What the user sees.** Total interest jumps from $289,596.93 to **$339,596.94** — i.e. *up*
by the balloon amount — and the Rate cell silently changes **6.0000% → 7.0069%**. To a user
this reads as "I added a $50,000 payment and my interest went *up* $50,000 and my rate
became 7%."

**Why.** Field-presence dispatch still treats Rate as the unknown, so it solves the rate
that makes the fixed payment amortize *with* the extra balloon. It is mathematically
self-consistent (reproduced via the API: solved rate 0.070069, 360 rows) but deeply
counter-intuitive, and there is **no notice** that the rate was re-solved.

**Not a balloon bug.** With the rate *specified* (or the payment left blank) the same balloon
behaves intuitively — interest drops and the loan pays off early (verified: payment re-solves
to $1,334.11, interest falls to $280,279.64, balance drops $50k at the balloon date). The
problem is strictly the silent re-solve of a stale/blank field when an option is added.

**Fix.** When an Advanced Option is present and a core field is in solve-mode, surface a
notice ("Rate re-solved to 7.0069% to fit the balloon"), and/or add a result advisory when a
balloon/prepayment *increases* total interest. The existing red "N active" badge flags that
an option is in effect but not that a core field was re-solved.

---

### M6 — Compare APR compares non-comparable rows, with a grammar slip  ·  S2 / S3

> **RESOLVED 2026-06-24 (DOS-faithful).** The DOS source clarifies two of these are *not* bugs:
> (1) DOS deliberately compares loans of different terms — its crossover just has to fall within
> both terms (Mortgage.pas:534) — so no same-term block was added; (2) DOS itself prints the
> plural "X years, Y months" (Mortgage.pas:684-690), so "1 years, 1 months" is DOS-faithful and
> was kept (a code comment now marks it intentional). The real non-DOS behavior was auto-picking
> the first two rows: DOS designates the pair from the cursor (selected row = mortgage #1).
> Fixed in `index.html` — Compare APR now anchors mortgage #1 to the **selected row** and takes
> the next other APR row as #2, and when fewer than two mortgages have an APR it shows DOS-style
> guidance instead of guessing. Frontend-only; engine unchanged. **Action for you:** rebuild.

**What the user sees.** With two or more calculated rows in the Mortgage grid, *Compare APR*
silently picks the **first two rows that have a computed APR** and compares them, with no
check that their loan terms match — even though the Help says "Years must be the same on both
rows." Comparing a 20-year row against a 30-year row produced: *"APRs cross at 9.9598% for
duration 1 years, 1 months. If held longer than 1 years, 1 months, Mortgage A is better."*

**Verified.** Against the raw `/api/mortgage/compare`, the crossover time is **+1.0833 years**
(positive — the UI's "~1.08 years" is correct; there is no sign error). The real issues are
(a) no same-term guard and no indication of *which* two rows were auto-selected, so a user can
get a confident recommendation from an apples-to-oranges pair, and (b) the duration string is
always pluralized — "1 years, 1 months" instead of "1 year, 1 month"
(`internal/finance/mortgage/mortgage.go:553-560`).

**Fix.** Warn (or let the user choose the pair) when the two rows differ in Years; singularize
the duration text.

---

### C1 — "Clear All" confirm dialog hard-blocks the page  ·  S3 (testing nuisance)

The native `confirm("Clear all rows?")` dialog froze the page renderer hard enough that an
automated session could not dismiss it (a reload was required). Harmless for a human clicking
**OK**, but a non-blocking in-page confirmation would be more robust.

---

## What was verified correct (the reassuring story for the meeting)

Every item below was entered live and matched the Help and/or the DOS oracle to the cent.

**Mortgage.** Forward payment (Ex1 → $1,538.30); reverse-solve price from a target monthly
(Ex2 → $241,749.12, 21.9944% down); balloon-amount solve (Ex3 → $98,372.47); "harden a
computed value and reuse it" with a 15-year balloon (Ex4 → $184,912.27); over-determined-row
error message is clear and actionable.

**Amortization.** Odd-first-period payment is DOS-faithful (Ex1 → $731.98, matches the DOS
oracle); solve-payment (Ex1d → $1,498.88), solve-amount (Ex1e → $250,000.61), solve-rate
(Ex1f → 6.0000%); a balloon on a fully-specified loan correctly reduces interest and pays the
loan off early.

**Present Value.** Lump-sum PV under both rate conventions (Ex1 → $9,231.16 True Rate /
$9,233.61 Loan Rate — and the Rate-Type distinction the Help warns about behaves exactly as
documented); IRR backward-solve recovers the rate (→ 8.0000%).

**Actuarial / life contingency.** Lifetime-pension valuation: Living $253,135.24, Dead
$144,627.62, non-contingent $397,762.85 — all matching the Help, and Living + Dead = the
non-contingent value to the penny (a clean internal-consistency check).

**General UX.** Computed vs. input cells are clearly distinguished; the auto-calc
stale-result guard prevents a stale number from masquerading as current; the leftover-state
guards (Advanced-Options "N active" badge, PV "Included in total" line) are mostly in place.

---

## Suggested meeting talking points

1. **Lead with the engine.** Every valid example reproduces to the cent against the real DOS
   engine, including the backward solves and the actuarial module. There are no wrong-number
   defects in this pass.
2. **Get ahead of A1-DOC.** If the client opens Help → Amortization Example 1, the numbers
   won't match the program. Be ready to explain it's a *documentation* lag (the program is the
   DOS-correct one) — or, better, fix `help.html` before the meeting so the question never
   comes up.
3. **A2 is the one to watch live.** If the client experiments with Advanced Options on top of
   a solved field, the silent rate re-solve looks alarming. A short advisory line would defuse
   it.
