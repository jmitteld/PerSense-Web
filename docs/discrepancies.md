# Per%Sense — Known Discrepancies

This document tracks known differences between the DOS source code behavior,
the Windows help documentation, standard financial textbook formulas, and
the Go port. Understanding these is important for correctness validation
and for anyone comparing Per%Sense output to other financial calculators.

---

## 1. Summation Formula: Continuous vs. Discrete Compounding

**Status:** By design — Go port matches DOS source code.

### Description

The DOS `Summation()` function in `Mortgage.pas` uses a continuous
compounding formula based on the natural exponential:

```
f     = exxp(-r / 12)        = e^(-r/12)
last  = exxp(-r * t)         = e^(-r*t)
sum   = f * (1 - last) / (1 - f)
```

Standard financial textbook mortgage math uses discrete (monthly)
compounding:

```
sum = (1 - (1 + r/12)^(-12*t)) / (r/12)
```

These produce slightly different results. The difference grows with
higher rates and longer terms.

### Impact

| Rate | Term | DOS Summation | Standard Summation | Difference |
|------|------|---------------|-------------------|------------|
| 6%   | 30yr | 166.5232      | 166.7916          | 0.2684     |
| 8%   | 30yr | 129.6928      | 130.0536          | 0.3609     |
| 8.5% | 30yr | 125.4356      | 125.7887          | 0.3531     |
| 5%   | 15yr | 126.3684      | 126.4850          | 0.1166     |
| 12%  | 30yr | 96.7821       | 97.2183           | 0.4362     |

For a $100,000 loan at 8.5% over 30 years:
- DOS/Go monthly payment: **$771.05**
- Standard formula: **$768.91**
- Difference: ~$2.14/month

### Why the DOS code does this

The `Summation()` function comment says `r is the true rate`, and the
formula treats the user's entered loan rate as if it were a continuously
compounded (true) rate. The DOS program stores the user's percentage
input directly in the `rate` field without converting between loan rate
and true rate representations.

This is a design choice in the original software, not a bug. The
Per%Sense help documentation on rates explains that true rates and loan
rates are different representations of the same quantity, but the
Mortgage screen does not perform the conversion — it passes the
user-entered value directly to a formula that assumes continuous
compounding.

### Go port decision

The Go port faithfully replicates the DOS source code formula. This
ensures identical output for all inputs. Users comparing Per%Sense to
other mortgage calculators (which use standard discrete compounding)
should expect small differences in the range of $1–$3/month on typical
loans.

---

## 2. Help Documentation Examples vs. Running Program Output

**Status:** Documentation artifact — help text may not match program output.

### Description

Several help documentation examples show expected values that were
apparently computed using standard financial formulas rather than
captured from the actual running DOS program. This means the example
outputs in the help docs do not always match what the program produces.

### Known instances

#### Help Example 2 (Mortgage — reverse price computation)

- **Setup:** $56,000 cash, $1,650/month, 1.5 points, 8.5%, 30 years, $200 tax
- **Help doc says:** Price = $241,749.12
- **DOS code produces:** Price = $241,233.69
- **Difference:** $515.43
- **Cause:** The help doc used the standard discrete summation (130.0536)
  while the DOS code uses continuous summation (129.6928). The monthly
  payment value ($1,650) times the different summation factors yields
  different financed amounts, which propagate to different prices.

#### Help Example 1 (Mortgage — monthly payment)

- **Setup:** $200,000, 20 years, 8%, 2 points, 20% down, $200 tax
- **Help doc says:** Monthly total = $1,538.30
- **DOS code produces:** Monthly total ≈ $1,540.97
- **Difference:** $2.67
- **Cause:** Same summation formula difference. For 8% over 20 years,
  the continuous vs. discrete summation gives slightly different payment
  amounts.

### Recommendation

When using help documentation examples as test cases, compare against
the DOS formula output (which the Go port matches), not the help doc
text values. The `refdata.json` file in `legacy/reference-output/`
contains values generated from the actual DOS formulas and should be
treated as the authoritative reference.

---

## 3. Rounding: Round-Half-Down vs. Banker's Rounding

**Status:** Go port matches DOS behavior.

### Description

The DOS `Round2()` function uses **round-half-down** (truncation at the
half): when the value is exactly at the midpoint (e.g., 1.235), it
rounds toward zero (to 1.23), not to the nearest even number.

From `refdata.json`:
```
Round2(1.235) = 1.23   (round-half-down)
Round2(1.236) = 1.24
Round2(0.005) = 0.00
Round2(0.006) = 0.01
```

Standard banker's rounding (round-half-to-even) would give:
```
Round2(1.235) = 1.24   (round to even)
Round2(0.005) = 0.00   (same — rounds to even)
```

### Impact

The difference only manifests when a value falls exactly on the half-cent
boundary, which is rare in practice. Over a 360-payment amortization
schedule, cumulative rounding differences are absorbed in the final
payment adjustment.

### Go port decision

The Go port's `interest.Round2()` replicates the DOS round-half-down
behavior exactly. The `CLAUDE.md` notes say "use banker's rounding
unless original code differs" — the original code does differ, and we
follow it.

---

## 4. Present Value: True Rate vs. Loan Rate on the PV Screen

**Status:** Potential behavioral difference — needs verification.

### Description

The Present Value screen accepts a rate input with a "Rate Type"
selector (True Rate, Loan Rate, Yield). The DOS program's rate handling
on the PV screen involves `InterpretedRate()` which converts between
rate representations based on the computational settings.

The Go API currently accepts a single `rate` field without a type
indicator. The rate is passed directly to the PV calculation engine
without conversion. This may produce different results than the DOS
program when the user intends a loan rate but the engine treats it as a
true rate (or vice versa).

### Recommendation

When the PV rate type selector is wired to the API, the backend should
implement the same `InterpretedRate()` conversion logic:
```
InterpretedRate(input) = YieldFromRate(RateFromYield(input, default_peryr), screen_peryr)
```

For the common case of 12 payments/year on both settings, this is an
identity function and produces no difference.

---

## 5. Date Arithmetic: 360-Day vs. 365-Day Edge Cases

**Status:** Go port matches DOS behavior — verified via refdata.json.

### Description

The DOS `YearsDif()` function computes the fractional year difference
between two dates. For 360-day basis, all months are treated as 30 days
with specific rules for end-of-month dates. The Go port replicates
these rules exactly.

Verified cases from `refdata.json`:

| From | To | 360-day | 365-day |
|------|----|---------|---------|
| 2024-01-01 | 2025-01-01 | 1.000000 | 1.002053 |
| 2024-01-01 | 2024-07-01 | 0.500000 | 0.498289 |
| 2024-01-15 | 2024-03-01 | 0.127778 | 0.125941 |
| 2000-01-01 | 2030-06-15 | 30.455556 | 30.453114 |

No discrepancies found.

---

## 6. exxp() and lnn() — Taylor Series Threshold

**Status:** Go port matches DOS behavior.

### Description

The DOS `exxp()` and `lnn()` functions use Taylor series approximations
for values very close to 0 (for exxp) or 1 (for lnn). The comment in
the DOS source says this compensates for a Turbo Pascal compiler bug
where `ln(1+x)` lost precision for small `x`.

The Go port replicates these thresholds (`|x| < 1e-4` for exxp,
`|x-1| < 1e-4` for lnn) even though Go's `math.Exp` and `math.Log`
do not have the same precision issue. This ensures identical output
for edge cases near zero rates.

All `exxp` and `lnn` test cases from `refdata.json` pass with full
precision matching.
