# Per%Sense — where the Go rewrite stands against the original DOS program

**Status as of 31 July 2026.** Written for a non-technical reader. Every number
below comes from running the *real* original program and the new one side by
side on the same inputs and comparing the answers.

---

## The short version

We are testing the new Per%Sense against the original by running both programs on
hundreds of thousands of randomly generated loan and investment scenarios and
comparing every number they produce.

| Calculator | How much we've tested it | How often the two disagree |
|---|---|---|
| **Present Value** | 29,891 worksheets — over 5 million individual lines compared | **Never** |
| **Mortgage** | 20,754 scenarios, plus 136,270 interest-rate calculations | **Never** |
| **Amortization (loan schedules)** — ordinary loans | ~25,000 scenarios compared | About **1 in 3,600** |
| **Amortization** — unusual loans (very long terms, exotic payment schedules) | 2,300+ scenarios compared | About **1 in 290** |

**Present Value and Mortgage are finished** in the meaningful sense: we have not
been able to find a single input on which they disagree with the original, at any
sample size we have run.

**Amortization is close but not finished.** On ordinary loans the two programs
agree about 99.97% of the time. On deliberately unusual loans the agreement is
lower, and that is where the remaining work is.

---

## Why there are two different numbers for Amortization

This is the single most important thing to understand about the figures above,
and it is easy to misread.

The "1 in 3,600" and the "1 in 290" are **not** a before-and-after. They are the
same software measured by two different test generators. The first generator only
ever produced loans of 25 years or less. When we removed that restriction, we
started testing a whole category of loan we had never tested — and found problems
there.

So the honest reading is: *the software did not get worse; our testing got
better.* We had been quoting a pass rate for a region we had never looked at.

This has happened four times in the past week. Each time we asked "what kind of
loan can our test generator never produce?", we found a real defect on the first
look. That question has become part of the standard procedure.

---

## What we fixed this round

Every defect we find gets traced back to the specific line of the original
program's source code that produces the behaviour, so we are matching the
original's *logic*, not just curve-fitting to its output.

This round's finding is a good illustration of how subtle the remaining issues
are.

Per%Sense has a setting called **"Exact"**. Turning it on tells the program to
count actual calendar days rather than use a simplified 30-day month. The
original program has a quirk: when Exact is switched on but the day-count basis
is left at the default, the setting is **ignored when calculating the payment
amount but honoured when producing the payment schedule**. The two halves of the
program disagree with each other — and they have since the 1990s.

Our rewrite had reasonably treated the setting as ignored in both places. That
was correct for monthly, quarterly, semi-annual and annual loans — for those, the
original's two methods happen to produce identical answers, so the difference is
invisible. It is only visible on **semi-monthly loans** (two payments a month),
and only when the payment falls near the end of the month, where February's short
length makes the two methods diverge.

On one real test case this was a **$1,511 difference** on a $40,606 loan — and it
also caused the loan to be shown as paid off eight payments later than it should
have been.

Fixing it cut the disagreement rate on this category of loan roughly in half
(from about 13% to about 6% on our harshest test generator).

The fix has one deliberate limitation, which is worth stating plainly: we
restricted it to the payment frequencies where it actually matters, rather than
applying it everywhere the original's code technically applies it. Applying it
everywhere caused a *different* problem on loans running past the year 2100,
because of a known and separately-tracked calendar issue. The narrower fix is
provably equivalent for every other payment frequency.

---

## What is still open

**Ranked by how much they affect a normal user.**

1. **Unusual loan schedules — the current focus.** Roughly 4–13% of deliberately
   exotic scenarios still disagree. These involve combinations most users will
   never enter: 300-payment annual loans, prepayment schedules on a different
   frequency from the loan itself, terms running centuries into the future. Real
   defects live here, which is why we keep working it, but the exposure to normal
   use is low.

2. **Very long schedules.** The original program stores a date's year in a way
   that can only hold values up to 2155, and silently wraps around past that.
   We reproduced that behaviour exactly last round. A residual of about 15%
   remains in that region.

3. **Three calculators not yet tested at all:** the actuarial module (deferred by
   decision — it needs a different testing rig), the payoff calculator, and
   sub-monthly loans carrying advanced options. "Not tested" is not the same as
   "known broken", but it is also not the same as "working". These are honestly
   outside every percentage on this page.

4. **A measurement flaw of our own.** About 4% of test cases are discarded because
   our test rig — not the software — occasionally fails to get an answer from the
   original program. This biases every number here slightly and is on the list to
   fix.

---

## How much confidence the numbers deserve

A few caveats we would rather state than have discovered:

- **The percentages are one significant figure, not three.** Read "about 1 in
  3,600", not "99.972%". Our samples are large enough to say "roughly one in
  several thousand" and to say what *kind* of thing is going wrong. They are not
  large enough to defend a second decimal place.

- **A percentage is a property of the test, not only of the software.** The same
  build has measured 99.977%, 99.31%, 99.9% and 99.97% this month under four
  different test generators, with no code change between some of them. The
  variation is almost entirely about what each generator could reach. This is why
  we have stopped leading with a single number.

- **Agreeing to the penny is not the same as agreeing.** Twice this year a real
  defect hid for months because both programs printed the same rounded figure
  while computing different underlying values. We now compare the raw internal
  numbers bit-for-bit, not the printed ones. That comparison is in place for all
  the forward calculations; the "solve backwards for the rate/amount" paths do not
  have it yet, and that is the most likely place for an undetected problem.

- **Nothing here is a claim about the original being correct.** We are matching
  the original program's behaviour, including its bugs and quirks, on purpose.
  Several things we have "fixed" this month were us removing a *correction* the
  rewrite had made to the original's arithmetic.

---

## The one-line summary

> The Present Value and Mortgage calculators match the original exactly on every
> input we have been able to test. Loan amortization matches on about 99.97% of
> ordinary scenarios; the remaining disagreements are concentrated in unusual
> inputs, are being found and fixed at a steady rate of roughly one root cause per
> working session, and each one is traced to a specific line of the original
> program before it is changed.

---

### Measurement provenance

Fresh measurements on the 31 July round-13 build: the long-horizon differential
(two independent random seeds, 240 comparable scenarios) and the semi-monthly fix
above. Present Value, Mortgage and the ordinary-amortization rate are carried
forward from the 31 July round-10 assessment — those surfaces have had no
divergence in any sweep since, and the full gated regression suite is green on
the current build. Detail, method and known caveats:
`claude/convergence_assessment_2026-07-31c.md`.
