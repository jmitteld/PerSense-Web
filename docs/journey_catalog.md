# Expanded Journey Catalog (question-framed)

A fuller brainstorm of goals a user might legitimately bring to Per%Sense,
phrased the way they'd think of them — as questions. The emphasis is on
**capabilities people wouldn't guess are there**, since the home page can only
hint at a fraction of what each worksheet does.

Legend:

- **● on home page** — already one of the ten home-page journeys.
- **★ hidden gem** — genuinely supported, but low-discoverability; a strong
  candidate to surface.
- **○ supported** — works today, more discoverable / niche.

Every item below maps to a real, shipping capability (verified against the
engine and the help examples). Things the engine does *not* support — Canadian
semi-annual compounding, daily (DAY) compounding, and simple/non-compounding
interest — are deliberately excluded.

---

## Mortgage

1. **What's my monthly payment?** — ● price + terms → payment.
2. **How much house can I afford?** — ● work back from a monthly budget.
3. **How much cash do I need at closing?** — ● down payment + points.
4. **Low points or a low base rate — which is better, and when?** — ★ Compare
   APR finds the crossover holding period where the cheaper loan flips
   (`Compare APR`). Most people don't know the answer depends on how long they
   stay.
5. **How does my payment change across a range of rates?** — ★ What-If table
   generates a row per rate automatically, no re-typing.
6. **How do rate *and* term together change my payment?** — ★ Double What-If
   builds the full grid of combinations.
7. **Can a balloon at the end lower my monthly payment?** — ○ balloon support.
8. **What balloon keeps my payment at the 30-year level on a 15-year loan?** —
   ★ solve the balloon amount (leave it blank).
9. **What loan amount does the payment I can afford support?** — ○ solve price
   / amount borrowed from the monthly figure.
10. **What's my true APR once points are folded in?** — ○ APR computed
    automatically whenever points are present.

## Amortization

1. **What does my full payment schedule look like?** — ● month-by-month table.
2. **When will my balance reach a payoff target?** — ● bidirectional payoff
   lookup (type a balance, get the date — or vice-versa).
3. **How much do extra payments save?** — ● interest saved + time shaved.
4. **What interest rate am I actually paying?** — ★ leave Rate blank and supply
   amount + payment + term; the engine back-solves the rate.
5. **How much can I borrow for a payment I can afford?** — ★ leave Amount blank
   and back-solve from payment + rate + term.
6. **How long until the loan is paid off?** — ★ leave # Periods blank and derive
   the term from a known payment.
7. **How much faster do biweekly payments retire the loan?** — ★ switch the
   frequency and compare; surprisingly large effect.
8. **My loan is an ARM — what's my payment after the rate resets?** — ★
   Rate/Payment Adjustments model the reset (and can solve the new payment, or
   the implied rate, for you).
9. **Interest-only for the first year, then amortize (construction loan)?** — ★
   Moratorium defers principal until a date you choose.
10. **Skip payments over the slow season every year?** — ★ Skip Months (e.g.
    `6-8` for a summer-closed business).
11. **What happens if my payment doesn't even cover the interest?** — ★ negative
    amortization is modeled (balance grows, tracked per period).
12. **Guarantee I pay down at least $X of principal each period?** — ★ Target
    principal reduction bumps the payment up when needed.
13. **Export the whole schedule to Excel?** — ○ CSV export.

## Present Value

1. **What's a future payment worth today?** — ● discount a lump sum.
2. **What's my rate of return (IRR)?** — ● leave Rate blank, supply the price.
3. **What's a pension worth (with a COLA)?** — ● annuity + cost-of-living
   adjustment.
4. **What will today's money be worth later (inflation / future value)?** — ●
   put the as-of date in the future.
5. **What's a structured legal settlement worth today?** — ○ value the payment
   stream as of the settlement date.
6. **What's a bond's yield to maturity?** — ★ IRR on the bond's cash flows.
7. **What lump sum today funds $X a month for N years (retirement
   drawdown)?** — ★ leave the payment amount blank and solve it from a starting
   balance.
8. **Value cash flows when the interest rate changes over time** — ★ the
   Variable Rate Schedule handles IRS underpayment interest, statutory /
   prejudgment interest, and piecewise yield curves. Few users know this exists.
9. **As of what date is this payment worth $X?** — ★ leave the as-of date blank
   and solve for it.
10. **Lease vs. buy — which costs less in today's dollars?** — ○ value both as
    present-value costs and compare.
11. **Compare two investments by their IRR.** — ○ solve each rate and compare.
12. **Mix one-time lump sums and recurring payments in a single valuation.** —
    ○ both grids feed one total.

## Life Contingency (Actuarial)

This whole module is a hidden gem — it was dropped from the Windows version and
restored here, so almost no one expects it.

1. **What's a lifetime pension worth (payments stop at death)?** — ★ "Living"
   contingency weights each payment by survival probability.
2. **Value a joint-and-survivor annuity (pays while *either* spouse lives).** —
   ★ "Either" — the common pension survivor benefit.
3. **Value a joint-life annuity (stops at the *first* death).** — ★ "Both".
4. **Wrongful death: value lost support over a projected lifespan.** — ★ Living
   contingency + a Payment on Death.
5. **Add a death benefit (payment on death) and value it.** — ★ POD value is
   integrated over the mortality distribution.
6. **Solve the face amount of a contingent insurance payout.** — ★ back-solve a
   contingent payment's amount from a target present value.
7. **Value a survivor-only pension (pays the spouse only after the worker
   dies).** — ★ "Only 1" / "Only 2".
8. **Use my own mortality table.** — ★ paste a custom `age,qx` table (or pick
   the built-in SSA 2021 Male/Female).
9. **Combine changing interest rates *and* life contingency in one
   valuation.** — ★ variable-rate schedule + actuarial weighting together.

## Workflow / cross-cutting

1. **Solve for whatever I leave blank — reverse any calculation.** — ★ the core
   fill-in-the-blank idea, but worth stating as a goal.
2. **Reuse a computed answer as a fixed input for the next step.** — ★ harden a
   green cell (double-click / `H`).
3. **Open my old DOS or Windows `.psn` file.** — ○ Import on the welcome screen.
4. **Generate a whole table of scenarios automatically.** — ○ What-If.
5. **Print a clean copy without the toolbar.** — ○ browser Print drops the
   chrome automatically.

---

## Recommended additions to the home page — DONE

The shortlisted hidden gems below are now wired as one-click `loadJourney`
entries on the welcome screen (15 journeys total, each phrased as a question).
Engine-verified inputs:

- **Mortgage:** "Low points or a low rate — which wins, and when?" — loads two
  loans (8.1%/3pts vs 8.5%/1pt); Compare APR crosses at **6 yrs 10 mths**.
- **Amortization:** "What interest rate am I actually paying?" — back-solves to
  **6.00%** from amount + payment + term. · "Can I skip payments over a slow
  season?" — skip Jun–Aug (`6-8`), opens the Advanced Options panel.
- **Present Value:** "What if the interest rate changes over time?" — the
  variable-rate schedule (5/7/10% by year), valuing a future $100k at
  **$80,251.88**.
- **Life Contingency** (new home-page group): "What's a lifetime pension
  worth?" — $2,000/mo for life, 65-yr-old male, SSA 2021 table → **$253,135**.

The two multi-grid journeys (variable-rate, lifetime pension) auto-load their
grids *and* open the relevant disclosure panel so the populated schedule /
life-table is visible; the user then presses Calculate.

Remaining ★ gems not yet on the home page (good future candidates): biweekly
acceleration, ARM reset, interest-only/moratorium, negative amortization,
target principal, bond YTM, retirement drawdown (solve the payment), solve the
as-of date, and the joint-and-survivor annuity.
