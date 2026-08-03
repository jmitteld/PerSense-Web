# Why Per%Sense's original engine believes 29 February 2100 exists

*Prepared 3 August 2026*

## The short version

Leap years follow a rule with two halves. Most people know the first half: **a
year is a leap year if it divides by four.** Fewer people need the second half:
**century years are skipped, unless they divide by 400.** So 1996 and 2024 are
leap years, 2000 was a leap year — and 1900, 2100 and 2200 are not.

The original DOS engine implements the first half only. It asks "does this year
divide by four?", and for the year 2100 the answer is yes. So it puts a 29th day
in February 2100, and any schedule that runs through that date counts one extra
day of interest.

That is the whole of the cause. What makes it interesting is everything around
it.

## The author knew about the rule

This was not ignorance of how leap years work. Elsewhere in the same codebase
there is a comment complaining that a rival product got exactly this wrong:

> *"Lotus doesn't recognize that 1900 wasn't a leap year, so its Julian numbers
> after Feb 29, 1900 are off by 1."*

So the century rule was understood well enough to catch a competitor failing it.
What happened instead is a shortcut: rather than implement the general rule, the
engine stores each year as a small number counting up from 1900 — 1900 is 0,
2024 is 124 — and carries a single hand-written exception saying "year 0 is not
a leap year."

That one exception is a patch for 1900 and 1900 alone.

## So it gets 1900 right and 2100 wrong

The consequence is a system that is correct at one century boundary and wrong at
the next, from the same piece of code. We measured this against the original
engine directly, by asking it to charge one day of interest either side of the
end of February:

| Loan dated 28 Feb, first payment 1 March | Interest charged | Days the engine counted |
|---|---|---|
| 1900 — *not* a leap year | 450.37 | 1 &nbsp;✅ correct |
| 2000 — a leap year | 476.59 | 2 &nbsp;✅ correct |
| 2096 — a leap year | 476.59 | 2 &nbsp;✅ correct |
| 2099 — not a leap year | 450.37 | 1 &nbsp;✅ correct |
| **2100 — not a leap year** | **476.59** | **2 &nbsp;❌ one day too many** |

Every ordinary year is handled correctly. Only the century boundary is missed,
and only the one the hand-written exception does not cover.

## Why nobody noticed for thirty years

Because between 1901 and 2099 the simple rule and the real rule agree on **every
single year**. There is no year in that window where "divides by four" gives the
wrong answer.

The first date the two rules disagree on is 29 February 2100. For software
written for a business running mortgage and lease schedules in the 1990s, that
date sat beyond the end of any schedule anyone would ever enter. The shortcut was
invisible for as long as the product was used, which is the ordinary way this
kind of thing survives — not carelessness, but a boundary that was safely over
the horizon and has since drifted into range as terms lengthen and origination
dates advance.

## One more quirk worth knowing

The same design has a hard stop. Because a year is stored as a single small
number counting from 1900, the largest year the original engine can represent at
all is **2155**. Dates beyond that do not produce a wrong answer so much as a
different date entirely.

This matters for the leap-year question in a specific way: **2100 is the only
century year the original engine can get wrong.** The next one, 2200, is past
the point where it can represent dates at all — so it never gets the chance.

---

### Appendix — the source, for your technical team

The rule lives in one routine, `DecideAboutFeb29` (`VIDEODAT.pas`, lines
333-338). `wy` is the year stored as an offset from 1900:

```pascal
procedure DecideAboutFeb29(wy :byte);
          begin
          if (wy mod 4 = 0) and (wy>0) then begin
               daysin[2]:=29; leapyear:=true;  daysbefore:=@leapdaysbefore end
          else begin
               daysin[2]:=28; leapyear:=false; daysbefore:=@notleapdaysbefore; end;
          end;
```

`wy mod 4 = 0` is the divisible-by-four half of the rule. There is no
century test and no 400-year test. The `and (wy>0)` clause is the hand-written
exception: it excludes `wy = 0`, which is the year 1900.

For 2100, `wy` is 200; 200 divides by four, so February is given 29 days. The
`byte` type is what caps the representable range at `wy = 255`, i.e. the year
2155, and therefore puts 2200 out of reach.

The year base is set in `Globals.pas:253` (`RetVal.y := YearOf(Hold) - 1900`)
and read back at `Globals.pas:264` (`IntToStr(TheDate.y + 1900)`). The Lotus
comment quoted above is at `INTSUTIL.pas:1683-1684`.

The measurements in the table were taken by running the original engine and
reading the interest it charged; they are reproducible with:

```
amort_oracle 100000 0.10 2 12 b365 exact loandmy=28.2.<year> firstdmy=1.3.<year>
```
