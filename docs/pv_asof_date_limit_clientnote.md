# Per%Sense — Present-Value "As-of Date" solve failure: a port off-by-100, now fixed

**Correction note:** an earlier draft of this memo described this as a DOS-faithful limitation.
That was wrong. On closer reading of the date encoding it is a **porting defect** — an
off-by-100-years error in one line of the new code. The DOS original does **not** have this
problem. It has been fixed; details below so you can describe it accurately to the client.

---

## Summary for the client

When the Present-Value screen solves for the **As-of Date** (As-of left blank, target Present
Value supplied), the new version aborted with **"time period too long"** whenever the answer fell
on or after ~**2029**. The cause was a one-line mistake in the port — its iterative date search
started 100 years too early (at 1900 instead of 2000). The original DOS program starts the same
search at the year 2000 and solves these cases without trouble. The fix makes the port match the
original exactly; no calculated result changes.

## How the As-of solver works (original Pascal)

The solver is an iterative search that starts from a fixed guess and steps to the answer
(`PRESVALU.pas:755-797`):

```pascal
asof.d:=1; asof.m:=1; asof.y:=100;     { First guess }
repeat
  ... sum := total present value of all rows, evaluated at the current asof ...
  diff := lnn(sumvalue/sum) / r.rate;  { years to shift; see below }
  AddYears(asof, diff);
until (count >= 10) or (abs(diff) < 0.002);
```

`AddYears` refuses any step longer than 128 years (`INTSUTIL.pas:894`):

```pascal
if (abs(yrs) > 128) then TimeTooLong   {Arbitrary limit}
```

## Where the 1900 came from — and why it was wrong

The original's first guess is `asof.y := 100`. The `daterec.y` field is **not** a calendar year;
it is a byte holding **(calendar year − 1900)**. This is confirmed three independent ways:

- the port's own type doc: *"the original Pascal daterec stores year as a byte (0-249
  representing 1900-2149)"* (`internal/types/records.go:10`);
- the date string routines: `RetVal.y := YearOf(Hold) - 1900` and display `y + 1900`
  (`Globals.pas:253` / `:264`);
- the DOS engine harness: `asof.y := 124  { 2024-01-01 }` (`legacy/oracle/pv_oracle.pas:67`).

So **`asof.y := 100` is the year 2000**, not 1900. The DOS search therefore starts at **2000**;
its first step to a ~2029 answer is only ~29 years — comfortably inside the 128-year cap — and it
converges immediately.

The port mis-translated this. It used `NewDateRec(1900, 1, 1)` for the first guess
(`internal/finance/presentvalue/backward.go:1291`), with a comment that conflated the byte value
100 with the year 1900. Starting at 1900 makes the first step ~(answer − 1900) years — ~129 years
for a 2029 answer — which trips the (correctly-ported) 128-year guard in `AddYears` and aborts
before the search can move. In short: the guard is DOS-faithful; the **starting point was not**.

## The fix

One line, `backward.go:1291`:

```
- asof := types.NewDateRec(1900, 1, 1)   // 100 years too early
+ asof := types.NewDateRec(2000, 1, 1)   // = DOS asof.y:=100
```

Verified after the fix (rebuilt engine):

| Input (rate 5%, lump $10,000) | Target PV | Before | After |
|---|---|---|---|
| lump 2034 | $7,788.01 | "time period too long" | As-of **2029-01-01** |
| lump 2026 | $9,048.37 | As-of 2024-01-01 | As-of 2024-01-01 (unchanged) |
| lump 2060 | $3,678.79 | "time period too long" | As-of **2040-01-01** |

No converged result changes (the 2024 case is identical); the failures simply stop. `go test
./...` passes. The 128-year cap remains exactly as in DOS, so the absolute boundary now sits near
**2128** — matching the original, instead of 2028.

## What was affected / not affected

Only the backward solve for the **As-of Date** was affected. All forward Present-Value
calculations, the rate/IRR solve, the payment and through-date solves, and every
Mortgage/Amortization calculation were correct throughout.

*(Related code sharing the same 1900-based date type — for completeness, not defects:
`PRESVALU.pas:240` single-payment date solve; `AMORTOP.pas:1146` `stopdate.y := 100 + ...` with
the two-digit-year "century divider" setting.)*
