# User Journeys & Help-Example Coverage

A brainstorm of the concrete goals a new user arrives with, mapped against the
worked examples that already exist in `help.html`. The intent is to decide which
journeys to surface on the **home page** or in the **Tour**, and where a new
help example would close a gap.

**Coverage key**

- ✓ — a full, worked example exists (inputs + answer)
- ◐ — partially covered: a concept/section explains it, or it appears only in a
  summary table, but there is no step-by-step worked example
- ✗ — no example and no dedicated explanation

The home page currently advertises each worksheet by capability
("Compare loan options…", "…balloons, rate changes, prepayments", "…IRR,
legal settlements, pensions…"). The Tour explains *mechanics* (blank-to-solve,
green cells, date entry, tooltips) but never names a goal the user might have.
Listing a handful of concrete journeys — each a one-click jump into a
pre-loaded example — would turn "what is this for?" into "oh, that's my problem."

---

## 1. Buying a home (Mortgage worksheet)

| # | User goal ("I want to…") | Example | Status |
|---|---|---|---|
| 1 | Find my monthly payment for a given price, rate, term, down payment | Mortgage Ex 1; Amort Ex 1d | ✓ |
| 2 | Work backward from a budget — how much house can I afford? | Mortgage Ex 2 | ✓ |
| 3 | Compare two loans (low points vs. low rate) and find the break-even holding period | Mortgage Ex 5 + Compare APR | ✓ |
| 4 | See the true APR once points are included | Amort Ex 2; Mortgage APR column | ✓ |
| 5 | See how the payment moves as the rate (and/or term) varies | Mortgage Ex 6, Ex 7 (What-If) | ✓ |
| 6 | Lower my monthly payment with a balloon at the end | Mortgage Ex 3, Ex 4 | ✓ |
| 7 | Know how much cash I need at closing (down payment + points) | Cash Required column; no worked example | ◐ |
| 8 | Price a Canadian mortgage (semi-annual compounding) | §Canadian Mortgages (prose only) | ◐ |

## 2. Managing or analyzing a loan (Amortization worksheet)

| # | User goal | Example | Status |
|---|---|---|---|
| 9 | Print a full month-by-month amortization schedule | Amort Ex 1 | ✓ |
| 10 | Find the rate I'm actually paying (amount + payment + term known) | Ex 1f / Ex 18 | ✓ |
| 11 | Find how much a quoted payment lets me borrow | Ex 1e | ✓ |
| 12 | Find how long the loan runs from a known payment | Ex 1g / Ex 9 | ✓ |
| 13 | Look up my remaining balance / payoff as of a date | Payoff lookup example | ✓ |
| 14 | Find the date my balance drops to $X (refi / drop PMI) | Payoff lookup (bidirectional) | ✓ |
| 15 | See how biweekly or "13th" payments accelerate payoff | Ex 4, Ex 16; Advanced D | ✓ |
| 16 | Size the extra monthly payment needed to retire on time | Advanced Solver C | ✓ |
| 17 | Model an ARM rate reset (new rate, solve new payment — or the reverse) | Advanced B & F, Ex 6 | ✓ |
| 18 | Defer principal at the start (construction / seasonal — moratorium) | Ex 13; Ex 5 (interest-only + balloon) | ✓ |
| 19 | Skip seasonal months with no payment | Ex 17 | ✓ |
| 20 | Handle negative amortization (payment < interest) | Ex 10, Ex 11 | ✓ |
| 21 | Guarantee a minimum principal paydown each period (target) | Ex 14, Ex 15 | ✓ |
| 22 | Solve the balloon amount needed to hit a target term | Advanced Solver A; Ex 8 | ✓ |
| 23 | Build a weekly-payment loan | Ex 3 | ✓ |
| 24 | Export the schedule to Excel | CSV export (documented + Tour) | ✓ |
| 25 | See the Rule-of-78s interest/principal split | §Rule of 78s (setting prose only) | ◐ |

## 3. Saving, investing, time value of money (Present Value worksheet)

| # | User goal | Example | Status |
|---|---|---|---|
| 26 | Present value of a single future payment | PV Ex 1 | ✓ |
| 27 | Present/future value of a stream of regular payments (annuity) | Ex 2, Ex 3 | ✓ |
| 28 | Value a pension with a cost-of-living adjustment | Ex 4, Ex 4b | ✓ |
| 29 | Find my IRR / rate of return on an investment | Ex 5b; Ex 10 (bond YTM) | ✓ |
| 30 | Find what level payment a lump sum can fund (decumulation) | Ex 5d | ✓ |
| 31 | Solve the start date / as-of date of a cash flow | Ex 5c, Ex 5e | ✓ |
| 32 | Value cash flows under rates that change over time (IRS/legal interest) | Ex 5f, Ex 11, Ex 12, VR example | ✓ |
| 33 | Adjust today's dollars for inflation (future value) | §Concept (prose); no dated worked example | ◐ |
| 34 | Compare two investment options head-to-head | Ex 16 (summary only) | ◐ |
| 35 | Lease vs. purchase analysis | Ex 17 (summary only) | ◐ |
| 36 | Simple (non-compounding) interest for legal damages | Ex 13, Ex 21 — documented as **not implemented** | ◐ |

## 4. Actuarial / legal valuation (Present Value → Life Contingency)

| # | User goal | Example | Status |
|---|---|---|---|
| 37 | Value a lifetime (life-contingent) pension | Actuarial Ex: Lifetime Pension | ✓ |
| 38 | Wrongful-death lost-support valuation with a death benefit (POD) | Actuarial Ex: Wrongful Death + POD | ✓ |
| 39 | Solve the face amount of a contingent insurance lump sum | Actuarial Ex: Contingent Lump Sum | ✓ |
| 40 | Joint-life / survivor valuation (Only 1 / Only 2 / Either / Both) | Two-life options described; no worked example | ◐ |
| 41 | Set up a custom life table | §Setting Up Life Tables (prose only) | ◐ |

## 5. Cross-cutting workflow goals

| # | User goal | Example | Status |
|---|---|---|---|
| 42 | Reverse any calculation — leave any field blank to solve it | Fill-in-the-blank concept; nearly every example | ✓ |
| 43 | Reuse a computed value as a fixed input (harden) | §Hardening; Mortgage Ex 4 | ✓ |
| 44 | Open a legacy Per%Sense `.psn` workspace file | Import button on home; **not mentioned in help** | ✗ |

---

## Gaps worth closing (ranked)

These are the journeys with no full worked example. Roughly ordered by how
likely a *new* user is to want them:

1. **`.psn` import (#44)** — the only home-page action with zero help coverage.
   A short "Opening a legacy file" subsection (and a Tour mention) would help
   returning DOS/Windows users, who are exactly the migration audience.
2. **Cash-to-close (#7)** — first-time buyers think in "how much do I need up
   front?" terms. A 3-line example solving Cash Required would be cheap to add.
3. **Compare two investments / Lease vs. purchase (#34, #35)** — these exist
   only as one-line entries in the Examples 6–22 summary. Promoting them to
   full worked examples would serve the "investor/finance" persona the home
   page already advertises.
4. **Joint-life / survivor actuarial (#40)** — the UI supports four two-life
   contingencies, but no example shows them. A single survivor-annuity example
   would demonstrate a headline capability.
5. **Future value / inflation (#33)** — the concept text mentions putting the
   inflation rate in the Rate field with a future as-of date, but there is no
   dated worked example. One short example would make a common goal concrete.
6. **Canadian mortgage (#8)** and **Rule of 78s (#25)** — explained as
   settings but never worked end-to-end. Lower priority (narrower audiences).
7. **Custom life table setup (#41)** — prose only; a worked walkthrough would
   help the actuarial/legal power user.

## Suggested home-page / Tour journey list

A compact, high-recognition set that maps to existing examples (so each can be
a one-click "load this example" link), one or two per worksheet:

- **Mortgage:** "What's my monthly payment?" · "How much house can I afford?" ·
  "Low points or low rate?"
- **Amortization:** "Show my full schedule" · "What's my payoff balance?" ·
  "How much do extra payments save?"
- **Present Value:** "What's this worth today?" · "What's my rate of return
  (IRR)?" · "Value a pension or settlement"
- **Actuarial (advanced):** "Value a lifetime pension" · "Wrongful-death support"

All of the above already have full worked examples, so they can ship as
home-page entry points immediately; the "Gaps" list above is the backlog for
new example content.

---

## Update — gap backlog closed (engine-verified)

The gap examples below were authored as **fully worked examples** with figures
captured from the actual engine (the Go binary, via the REST API), not by hand.
Each appears in `help.html`; several are also wired as one-click home-page
journeys (`loadJourney` in `index.html`).

| Gap (orig #) | Resolution | Verified result | Journey |
|---|---|---|---|
| Cash-to-close (#7) | Mortgage **Example 8** | Cash $72,800; Monthly $2,169.79; APR 6.5969% | "Cash at closing" |
| Rule of 78s (#25) | Amortization **Solver Example H** | Same $398.57 payment; owe $8,507.80 vs $8,467.01 after 12 mths | — |
| Extra-payment savings (#16/Adv-D family) | Amortization **Solver Example G** | Saves $79,800.51 interest; 108 pmts (9 yrs) sooner | "Extra payments" |
| Future value / inflation (#33) | PV **Example 5g** | $50,000 → $91,105.94 over 20 yrs at 3% | "Future value" |
| Compare two investments (#34) | PV **Example 5h** | IRR 6.39% (A) vs 4.06% (B) | covered by "Rate of return (IRR)" |
| Lease vs. purchase (#35) | PV **Example 5i** | Lease PV $13,344.20 vs Buy net $15,089.38 | — |
| Joint-life survivor (#40) | Actuarial **Joint-and-Survivor** example | Either $466,438.47; Both $328,148.78; certain $512,647.05 | — |

**Canadian mortgage (#8) — removed, not added.** Investigation showed the
`CAN` (and `DAY`) options are deliberately hidden because they were never wired
to this port's engine (`index.html` notes `Settings.Daily/Canadian are never
set`). Per the principle "if the functionality doesn't exist, it shouldn't be
mentioned," the Canadian Mortgages help section, the `CAN`/`DAY` settings
descriptions, and the "Canadian rates" aside in the rate-types section were
**removed** rather than documented. If Canadian compounding is wired into the
engine later, restore a worked example then.

**Still open (deferred):** Custom life-table setup (#41) remains prose-only;
Simple (non-compounding) interest (#36) stays documented as a known limitation
because the engine does not implement it.
