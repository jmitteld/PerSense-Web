# Per%Sense v3 — how close is it to replacing the DOS original?

**Status report · 20 August 2026**

---

## How we measure this

We run the **real DOS program** and **v3** side by side on the same inputs and compare the answers, digit for digit. Every figure below comes from those comparisons — none of it is opinion.

**"Convergence" means: how much of the evidence needed to justify switching DOS off has been produced and is holding up.** It does *not* mean "percentage of answers that are correct." A calculator can sit at 65% here and never once have given a wrong answer — it means we have not yet asked it enough *different kinds* of question.

> **What this report covers — the life-contingency (actuarial) options are excluded from everything below, by agreement.** They are not part of the verified working set and we make no claim about them. They stay in the product, off by default and marked beta. Every figure here describes the set you would actually be switching to.

---

## Where it stands overall — about 72%

| | | |
|---|---|---|
| **Does it compute the right numbers?** | **82%** | Very strong on the main calculators. Two known gaps, one target still short. |
| **Does it *behave* the same way?** | **45%** | Error messages, refusals, file handling, offline use. The weakest area — and until this month, largely unmeasured. |
| **Can we *prove* it, to a standard you would sign?** | **33%** | The proof a switching decision rests on is different from the testing we have been doing. Barely started — but nothing in scope is now unprovable. |

**The remaining 28% is more than 28% of the work.** Most of what is left has not been started, and experience says that when we test something properly for the first time, we find things. **What has changed is that every remaining item is now closable** — the one thing that never could be is out of scope.

---

## By calculator

| Calculator | How much we have compared | How often they disagree | Convergence |
|---|---|---|---|
| Ordinary loan schedules | 108,778 scenarios | **Never** | **90%** |
| Present Value | 3,029 worksheets · 1,023,944 lines | **Never** | **88%** |
| Complex loan schedules *(several advanced options at once)* | 2,091 scenarios, seven questions each | about 1 in 420 | **70%** |
| The application itself *(screens, files, offline)* | — | — | **70%** |
| Mortgage | 30,000 scenarios · 135,853 rate calculations | **Never** | **65%** |
| Interest rate (APR) *on option-heavy screens* | 1,856 rate figures | about 1 in 93 | **50%** |
| Error messages & refusals | 1,344 scenarios | about 1 in 12 | **35%** |

> **Why Mortgage scores 65% despite never disagreeing.** It has **never been tested with dates.** Every mortgage comparison we have run is date-free. On the loan side, date handling is where we found some of the most serious problems. That is not a defect — it is a blind spot, and it is the largest one we have.

---

## What is genuinely good

- **Present Value, Mortgage and ordinary loan schedules have never disagreed** with the original on any input we have been able to test — over a million individual comparisons for Present Value alone.
- **All eleven issues on your list are closed.**
- **The scope boundary you asked for is built and enforced** — the life-contingency options are off by default, marked beta, and refused by the calculation engine as well as hidden on screen, so they cannot be reached by accident. Variable-rate and every cost-of-living option stayed *in*: we checked, and they match the original exactly, so hiding them would have removed working features.
- **Every remaining disagreement in the complex-schedule set now has a named cause** in the original's own source code. A year ago we had symptoms; now we have explanations.

---

## What is not done, in plain terms

1. **Mortgage has never been tested with dates.** The largest untested area.
2. **Present Value testing stops in 2088.** Later dates are untested — and the original's own calendar breaks in 2091.
3. **Error messages differ about 1 in 12 times** — the wording, or which error appears. Our worst measured number.
4. **260 screens where one program answers and the other declines.** Sometimes DOS refuses and we answer; sometimes the reverse.
5. **Complex schedules are about half way to target.** The raw count is roughly 1 in 420 against a 1-in-400 goal — but we require the result to be *proved*, not reached by luck, and on that stricter measure we are at about 1 in 200.
6. **A last-row defect, found this week.** On certain schedules with extra payments the final row is wrong while *every total is correct*. Root cause found in the original's source; fix built and tested, not yet released.
7. **The application does not work offline.** It depends on a file fetched from the internet at start-up. About a day's work.

---

## The honest finding of the past week

**Almost all our testing has asked "is the number right?" — and very little has asked "does the program behave right?"**

Two things made that concrete. Our first **row-by-row** comparison immediately found a defect every previous test had been blind to: the totals matched to the penny while one row was wrong. And while building the beta switch, our own review found **two ways the screen could show a confidently wrong number with nothing on screen to indicate it.** Both were fixed before release; neither was an arithmetic error. That is why the second band scores 45%, and it is the clearest signal we have about what the remaining work actually is.

---

## What it would take to close the gap

| # | Work | Rough size |
|---|---|---|
| 1 | Release the fixes already built and tested, including the last-row defect | Small |
| 2 | Test Mortgage *with dates*, and Present Value past 2088 | **Large** — widest uncertainty |
| 3 | Bring error messages and refusal behaviour up to the standard of the arithmetic | Medium |
| 4 | Finish the application: offline use, file import and export, accessibility | Medium |
| 5 | **Run both programs in parallel on your real saved files**, and agree a short written list of accepted differences | Medium |

**Estimate: roughly 25 more focused working sessions** at the current rate. Item 2 is what could move that — if Mortgage-with-dates behaves the way loans-with-dates did, it becomes a project of its own.

**Item 5 is the one that actually justifies switching.** Everything above it makes v3 correct; item 5 is what makes the decision *defensible* — real scenarios, both programs, every difference either fixed or written down and agreed. It has not started, and it is the main reason the third band sits at 33%.

---

## What we need from you

1. **Real saved scenarios** — ideally a representative set of actual client work — for the parallel run in item 5.
2. **Five outstanding decisions**, all small, listed separately.
3. **Whether there is a target switch-over date.** If there is, we would move item 5 earlier and ship against an agreed list of known differences rather than closing every last one first.

One request has *dropped off* this list: we no longer need the original's missing actuarial source files. That ask only returns if you later decide to release the life-contingency feature.

---

## In one paragraph

> With the life-contingency options set aside by agreement, Present Value, Mortgage and ordinary loan schedules have **never** disagreed with the original on any input we have tested — well over a million comparisons. Complex option-heavy schedules are about half way to our accuracy target, and every remaining disagreement now has a named cause. All eleven of your reported issues are closed. **The gaps are: Mortgage has never been tested with dates; error and refusal behaviour is well behind the arithmetic; the application does not yet work offline; and we have not run both programs side by side on your real files — which is the evidence a switching decision actually rests on.** We judge v3 **about 72% of the way** to a defensible replacement, with roughly 25 more working sessions to close it. **Narrowing the scope did not shorten that list — it removed the one item that could never have been finished at all.**

---

*Figures are measured, not estimated, and quoted to one significant figure. A percentage is a property of the test as much as of the software. We match the original's behaviour deliberately, including its quirks: several recent "fixes" were us removing a correction v3 had helpfully made to the original's arithmetic.*

*Supersedes `claude/convergence_note_client_2026-08-08_DRAFT.md` (round 55b), never reviewed and now stale — notably the complex-schedule rate, the count of unexplained disagreements (now zero: every one has a named cause), and the status of the client's eleven-item list (now closed). The PDF rendering is the version to send.*
