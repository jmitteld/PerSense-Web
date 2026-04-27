# Per%Sense — Product Requirements

*Adapted from original Per%Sense Windows Help documentation for the Go web port.*

---

## 1. Product Overview

Per%Sense is a financial analysis tool with three worksheets, each designed as a flexible "fill-in-the-blank" calculator. Users enter the values they know and leave unknowns blank — the system determines what can be computed, selects the appropriate algorithms, and fills in the answers automatically.

**Core principle:** Input numbers are *hard data* (entered by the user); computed answers are *soft data* (displayed distinctly, e.g. with a green background). Soft data is erased and recomputed whenever the user makes a change.

### 1.1 What Does Per%Sense Do?

Every economic decision involves the trade-off of costs and benefits over time. Per%Sense reduces complex cash flows at different times to common terms so they can be compared directly.

**Applications include:**

- Analyzing annuities and other payment structures
- Valuing legal claims for damages
- Financial planning
- Investment analysis and internal rate of return (IRR)
- Comparison of business options — which is the best deal?
- Deciding whether to lease or purchase
- Mortgage analysis and comparison
- Structured loan design with advanced features:
  - Multiple balloon payments
  - Interest-only periods (moratorium)
  - Fixed payment to principal (target reduction)
  - Skipped payments
  - Combinations of monthly, weekly, and quarterly payments on the same loan

---

## 2. Worksheets

Per%Sense has three worksheets. The user navigates to each from a welcome screen.

### 2.1 Mortgage Worksheet

**Purpose:** Choose among different loan options and financing scenarios.

Use cases:
- How much house can you afford on your budget?
- Find the loan with the lowest APR
- Decide between a larger down payment and a larger monthly payment
- Evaluate whether paying more points to obtain a lower base rate is worthwhile
- Evaluate the effect of a balloon payment on monthly payments
- Compare multiple loan scenarios side-by-side

#### 2.1.1 Mortgage Grid

Each row models a single, self-contained calculation. The grid has the following columns:

| Column | Description | Type |
|--------|-------------|------|
| **Price** | Total purchase price | Dollar |
| **Points** | One-time bank charge at settlement. 1.0 points = 1% of amount borrowed paid to the bank | Percentage (4 decimal) |
| **% Down** | Percentage of purchase price paid by buyer at settlement. Mutually exclusive group with Cash Required and Amount Financed — fill in only one of the three | Percentage (4 decimal) |
| **Cash Required** | Sum of down payment + value of points charged. Generally the bulk of money needed at closing | Dollar |
| **Amount Financed** | Loan amount = purchase price minus down payment | Dollar |
| **Years** | Duration of the mortgage (e.g. 15, 20, 30 years) | Number (2 decimal) |
| **Loan Rate** | Quoted interest rate from the bank. Equals APR only when Points = 0 | Percentage (4 decimal) |
| **Monthly Tax+Ins** | Annual real estate taxes + homeowner's insurance divided by 12 | Dollar |
| **Monthly Total** | Total monthly mortgage payment including taxes and insurance | Dollar |
| **Balloon Yrs** | Number of years from settlement until balloon payment (optional) | Integer |
| **Balloon Amount** | Lump sum balloon payment amount (optional) | Dollar |

**Calculation rules:**

- Fill in **Price** OR **Monthly Total** — whichever is blank gets computed
- Fill in exactly one of: **% Down**, **Cash Required**, or **Amount Financed**
- All other left-side cells (Points, Years, Loan Rate) must be filled by the user
- **Balloon** area is optional. If both Yrs and Amount are filled, monthly payment adjusts accordingly. If only Yrs is filled, Amount is computed (requires both Price and Monthly Total to be specified)
- The Mortgage screen always uses a 360-day calendar and does not respond to computational settings

#### 2.1.2 APR Display

APR (Annual Percentage Rate) is computed automatically when sufficient data is present. APR is the standard measure of true borrowing cost defined by the Federal Truth In Lending Act.

- When Points = 0, APR equals the Loan Rate
- When Points > 0, APR rises above the Loan Rate
- The quoted APR is the *full-term APR* — assumes the loan is held for its full scheduled duration
- Early payoff increases the effective APR because points are amortized over a shorter period

#### 2.1.3 APR Comparison

Users can compare two mortgage rows. Per%Sense computes APR as a function of time for both loans:

- If the APR curves cross at some point, the system reports: "If mortgage is held for more than N years, M months, then Mortgage A is better"
- If the curves do not cross, one mortgage is unambiguously better

**Requirement:** Implement an APR comparison feature that accepts two mortgage rows and reports the crossover point or declares one better.

#### 2.1.4 "What-If" Tables (Row Generation)

Generate multiple mortgage rows automatically by varying one or more columns:

- User selects up to 3 columns to vary
- Specifies an increment for each column
- Specifies the number of lines to generate
- System creates all combinatorial variations and computes each row

**Example:** Vary Loan Rate from 7% to 9% in 0.25% increments = 9 rows.

**Example (double what-if):** Vary both Years and Rate simultaneously = (N+1) × (M+1) rows covering all combinations.

#### 2.1.5 Canadian Mortgages

Canadian mortgages differ in rate quoting: although payments are monthly, interest is compounded semi-annually. When Canadian mode is selected in computational settings, the rate column label should change to reflect this.

---

### 2.2 Amortization Worksheet

**Purpose:** Flexible loan analysis — create amortization schedules and compute payment amounts for complex structured loans.

An amortization table is a list of loan payments where each payment is divided into interest and principal parts, with running totals of interest paid to date and remaining balance.

#### 2.2.1 Basic Loan Information Grid

| Column | Description | Type |
|--------|-------------|------|
| **Amount Borrowed** | The principal amount of the loan | Dollar |
| **Loan Date** | Date the loan is originated / settlement date | Date |
| **Rate %** | The quoted annual interest rate | Percentage (4 decimal) |
| **1st Payment Date** | Date of the first regular payment | Date |
| **# of Periods** | Total number of payments | Integer |
| **Last Payment Date** | Computed: date of the final payment | Date (output) |
| **Pmts/Year** | Payment frequency: 1, 2, 4, 6, 12, 24, 26, 52 | Integer |
| **Payment Amount** | Dollar amount of each regular payment. Leave blank to compute | Dollar |
| **Points** | Points charged at settlement (for APR calculation) | Percentage (4 decimal) |
| **APR %** | Computed Annual Percentage Rate including points | Percentage (output) |

**Calculation rules:**

- Fill in Amount Borrowed, Loan Date, Rate, 1st Payment Date, # Periods, and Pmts/Year
- Leave Payment Amount blank to have it computed, OR enter it and leave another field blank
- Points are used only for APR computation; they do not affect the schedule
- Last Payment Date and APR are always computed outputs

#### 2.2.2 Payoff / Balance Section

Enter a date to compute the remaining balance of the loan as of that date. This is useful for determining payoff amounts at arbitrary points during the loan.

#### 2.2.3 Advanced Options (Extended / "Fancy" Mode)

When advanced options are enabled, six additional grids become available:

##### Prepayment Grid
Additional periodic payments beyond the regular payment.

| Column | Description |
|--------|-------------|
| Start Date | When extra payments begin |
| # Payments | How many extra payments |
| Stop Date | When extra payments end |
| Per Year | Frequency of extra payments |
| Amount | Dollar amount of each extra payment |

##### Balloon Payments Grid
One-time lump sum payments applied to principal.

| Column | Description |
|--------|-------------|
| Date | When the balloon is due |
| Amount | Dollar amount of the balloon payment |

The computational setting "Stated balloon includes regular payment" controls whether a $5,000 balloon on a date when regular payment is $1,000 means $5,000 total or $6,000 total.

##### Rate/Payment Adjustments Grid
For adjustable-rate mortgages (ARMs) and payment changes.

| Column | Description |
|--------|-------------|
| Date | When the adjustment takes effect |
| Rate | New interest rate from this date forward |
| Amount | New payment amount from this date forward |

##### Moratorium Grid
Interest-only period — payments cover only interest, with no principal reduction.

| Column | Description |
|--------|-------------|
| First Repayment Date | Date when principal repayment resumes |

##### Target Principal Reduction Grid
Force a minimum principal reduction each period. Payment amount adjusts upward if needed to meet the target.

| Column | Description |
|--------|-------------|
| Target Amount | Minimum principal reduction per payment |

##### Skip Months Grid
Specify months when no payment is made.

| Column | Description |
|--------|-------------|
| Months | String specifying month numbers, e.g. "6-8,12" means skip June, July, August, December |

#### 2.2.4 Schedule Output

The amortization schedule table includes:

| Column | Description |
|--------|-------------|
| # | Payment number |
| Date | Payment date |
| Payment | Total payment amount |
| Interest | Interest portion |
| Principal | Principal portion |
| Balance | Remaining principal balance |
| Interest to Date | Cumulative interest paid |

**Output options:**
- Display on screen (scrollable table)
- Export as comma-separated file (CSV)
- Export as tab-separated file
- Summary periods: detail only, summary only, or detail + summary
- Summary by year, quarter, or custom period

#### 2.2.5 APR Table

From the amortization screen, generate an APR report showing:
- Annual Percentage Rate
- Finance Charge (total interest + points over life of loan)
- Amount Financed (net proceeds to borrower)
- Total of Payments
- Payment details (number, amount, when due)

This mirrors the Federal Truth In Lending disclosure format.

#### 2.2.6 Pennies and Rounding

Amortization schedules must be accurate to the penny. Per%Sense uses banker's rounding and adjusts the final payment to absorb any accumulated rounding discrepancy. The last payment in a schedule may differ slightly from the regular payment amount.

---

### 2.3 Present Value Worksheet

**Purpose:** Flexible tool for working with the time value of money. Applications include financial planning, structured legal settlements, valuation of legal claims, annuities, pensions, and internal rates of return (IRR).

**Core concept:** The present value of a future dollar is the amount you would need to invest today for it to grow to that dollar by the future date. Conversely, the future value of a past dollar is what it would have grown to by now with compound interest.

As with all Per%Sense screens, flexibility comes from choosing which cells to fill in and which to leave blank:
- **Present value:** List payments, fill in date and rate, read the Value
- **IRR:** List payments, fill in the Value, leave Rate blank to be computed
- **Future value:** Set the As-of date in the future

#### 2.3.1 Single Payment (Lump Sum) Grid

| Column | Description | Type |
|--------|-------------|------|
| **Date** | Date of the single payment | Date |
| **Amount** | Dollar amount of the payment | Dollar |
| **Value** | Computed present/future value at the as-of date | Dollar (output) |

Multiple lump sum rows can be entered. The system computes the value of each independently.

#### 2.3.2 Periodic Payment Grid

| Column | Description | Type |
|--------|-------------|------|
| **From Date** | Date of first payment in the series | Date |
| **Through Date** | Date of last payment in the series | Date |
| **Periods/Year** | Payment frequency (1, 2, 4, 6, 12, 24, 26, 52) | Integer |
| **Amount** | Dollar amount of each payment | Dollar |
| **COLA %** | Annual cost-of-living adjustment (optional) | Percentage |
| **Value** | Computed present/future value of the entire series | Dollar (output) |

Multiple periodic rows can be entered. The system values each series independently.

**COLA behavior:**
- COLA column is optional; blank = 0%
- COLAs are interpreted as yields (not compounding rates), following convention
- The COLA month setting controls when the adjustment is applied: on the anniversary of the first payment, in a specific month, or continuously

#### 2.3.3 Present Value Summary

| Field | Description |
|-------|-------------|
| **As-of Date** | Reference date for all present/future value calculations |
| **Rate** | Interest/discount rate |
| **Present Value** | Sum of all individual lump sum and periodic values |

#### 2.3.4 Rate Representations

Interest rates can be expressed in three equivalent formats:

| Format | Description |
|--------|-------------|
| **True Rate** | Continuous compounding rate. Use for present value and savings computations |
| **Loan Rate** | Discrete compounding rate (depends on payments per year). Use for loan computations |
| **Yield** | Actual interest earned on one dollar during one year after all compounding |

These are equivalent — like pounds vs. kilograms. Example: True Rate 8.0% = Monthly Loan Rate 8.0267% = Yield 8.3287%. Entering any one computes the other two.

**Bond yields and Canadian rates** are loan rates for semi-annual compounding.

**Daily rates** are loan rates with day-by-day compounding, numerically very close to true rates.

#### 2.3.5 Variable Rate Mode

The regular Present Value screen uses a single interest rate. Variable Rate mode allows different rates for different time periods.

**Rate Schedule Grid:**

| Column | Description |
|--------|-------------|
| **Effective Date** | Date when this rate takes effect |
| **True Rate %** | True rate for this period |
| **Loan Rate %** | Loan rate for this period |
| **Yield %** | Yield for this period |

**Key rules:**
- The first entry is the rate in effect at the beginning of computation
- IRR calculations cannot be done in variable rate mode (since IRR is by definition a single rate)
- Rate table entries are input only — rates cannot be computed as unknowns in variable rate mode

**Use cases:**
- IRS tax interest computation (rates change by quarter)
- Legal damages valued at statutory rates that vary over time
- Simple interest mode for jurisdictions requiring it

#### 2.3.6 Simple vs. Compound Interest

Variable rate mode includes a simple/compound interest toggle:
- **Compound interest** (default): interest earns interest
- **Simple interest**: interest does not compound — required by some state laws for legal damages
- If a computation involves simple interest on past losses and compound on future losses, these must be computed separately

#### 2.3.7 IRR (Internal Rate of Return)

IRR is computed on the Present Value screen by filling in the Value and leaving the Rate blank. Per%Sense solves for the rate that makes the present value of the listed payments equal to the specified value.

IRR is equivalent to bond yield, APR, and time-weighted average return — all computed using the same mechanism.

#### 2.3.8 Tables

From the Present Value screen, generate a detailed table listing each individual payment with its computed value. This is useful for:
- Legal exhibits showing the present value of each payment
- Financial planning projections
- Audit trails

---

## 3. Computational Settings

These settings affect how calculations are performed across worksheets. They should be configurable via a settings panel.

### 3.1 Year to Divide Century

For two-digit year entry (MM/DD/YY format), years below this threshold are interpreted as 20YY; at or above, as 19YY. Default: 50 (covers 1950-2049).

### 3.2 Default Payments Per Year

Resolves ambiguity when a rate appears without an explicit payments-per-year on the same row. Options: 1, 2, 3, 4, 6, 12 (default), 24, 26, 52, CAN (Canadian semi-annual), DAY (daily compounding).

### 3.3 COLA Escalation Month

Controls when cost-of-living adjustments are applied:
- **Anniversary** — on the anniversary of the first payment date
- **Specific month** (1-12)
- **Continuous** — applied proportionally with each payment

### 3.4 Treatment of Interest on Interest

Applies only to negative amortization scenarios:
- **Actuarial Rule** (default): shortfall (accrued interest - payment) added to outstanding principal balance
- **USA Rule**: no interest on interest. Separate principal and interest balances are maintained. Payments applied first against interest balance, then principal

### 3.5 Basis Days Per Year

| Setting | Description |
|---------|-------------|
| **360** (default) | All months treated as 30 days. Standard for loans and bonds |
| **365** | Actual calendar days divided by 365 or 366. Standard for savings accounts, weekly/biweekly loans |
| **365/360** | Actual calendar days divided by 360. Non-standard hybrid that slightly overstates interest. Included for compatibility with certain banks |

**Notes:**
- The Mortgage screen always uses 360-day, ignoring this setting
- "365-day loans" conventionally use 365-day only for odd first/last periods, with all regular months as 1/12 year
- True 365-day computation (where each month's interest varies by actual days) requires setting both Basis = 365 and Exact Method = YES

### 3.6 First Interest Prepaid at Settlement

When a loan closes mid-month with payments due on the 1st:
- **YES** (default): Interest for the partial month is prepaid at settlement from loan proceeds. First regular payment covers the next full month
- **NO**: The regular payment amount is adjusted slightly to spread the partial month's interest over the loan life

Only applies when time between loan date and first payment exceeds one period.

### 3.7 Interest Paid in Advance or Arrears

- **Arrears** (default): Each payment includes interest for the period just ended
- **Advance**: Each payment includes interest for the period to come

### 3.8 Stated Balloon Includes Regular Payment

If a $5,000 balloon is specified on a date when the regular payment is $1,000:
- **YES**: Total payment on that date is $5,000 (balloon replaces regular)
- **NO** (default): Total payment is $6,000 (balloon is in addition to regular)

This also affects: to model skipped payments, enter a balloon of $0 with this set to YES.

### 3.9 Exact Method

- **NO** (default): Uses computational shortcuts assuming equally-spaced payments. Standard practice for all loan calculations
- **YES**: Computes every individual payment separately, accounting for varying month lengths. More precise but slower, and produces non-standard results

The difference between exact and standard is generally a few dollars per $10,000 or a few hundredths of a basis point.

In tables generated from the Present Value screen, individual payments are always listed exactly — so the table total may differ slightly from the screen total unless Exact Mode is on.

### 3.10 Rule of 78s

An accounting method that front-loads interest allocation, favoring the lender on early repayment. Payment amounts are not affected — only the interest/principal split.

- Only available with basic (non-advanced) amortization
- Automatically disabled when advanced options are active (no standard exists for Rule of 78 with balloon/rate changes)

---

## 4. Examples

These examples serve as both documentation and test cases for the system.

### 4.1 Mortgage Examples

#### Example 1: Computing Monthly Payments
**Given:** Purchase price $200,000. 20-year mortgage at 8% with 2 points. 20% down. Monthly taxes/insurance $200.
**Expected:** Monthly total = $1,538.30

#### Example 2: How Much House Can You Afford?
**Given:** $56,000 cash available. Budget $1,650/month including $200 tax+ins. Bank: 1.5 points, 8.5%, 30 years.
**Expected:** Price = $241,749.12, Amount Borrowed = $188,577.83

#### Example 3: Balloon Payment Amount
**Given:** House $280,000. 20% down, 30 years at 8.25% with 2.5 points. Monthly budget $1,600 including $300 tax+ins. 8-year balloon.
**Expected:** Balloon amount = $98,372 (or with $100,000 balloon, monthly = $1,593.67)

#### Example 4: 30-Year Payments with 15-Year Balloon
**Given:** $240,000 at 8.1%, 30-year amortization with 15-year balloon.
**Step 1:** Compute 30-year monthly payment = $1,777.79.
**Step 2:** Change to 15 years with 15-year balloon.
**Expected:** Balloon amount = $184,912

#### Example 5: APR Comparison (Low Points vs. Low Rate)
**Given:** Mortgage A: 8.1% with 3 points. Mortgage B: 8.5% with 1 point. Both 30 years.
**Expected:** APR A = 8.4257%, APR B = 8.6094%. APRs cross at 6 years 10 months. Mortgage A is better if held longer.

#### Example 6: What-If Table
**Given:** $100,000, 0 points, 0 down, 30 years, 7% rate.
**Action:** Generate table varying rate from 7% to 9% in 0.25% increments (8 lines).
**Expected:** Monthly payments from $665.30 (7%) to $804.62 (9%).

#### Example 7: Double What-If
**Given:** Same as Example 6.
**Action:** Vary both Years (30 down to 15 in -5 increments) and Rate (7% to 8.5% in 0.25% increments).
**Expected:** 28 rows covering all (4 years × 7 rates) combinations.

### 4.2 Amortization Examples

#### Example 1: Simple Amortization Table
**Given:** $100,000 at 8%, 30 years, monthly payments starting 03/01/1998, loan date 02/12/1998.
**Settings:** 360-day, prepaid interest YES.
**Expected:** Payment = $733.76. First period is short (02/12 to 03/01 = 17 days). Prepaid interest = $377.78. Final payment slightly different due to rounding.

#### Example 2: Payment Computation
**Given:** Same as Example 1, but leave payment blank.
**Expected:** System computes payment = $733.76.

#### Example 3: Weekly Payments
**Given:** $100,000 at 8%, first payment 02/19/1998, 1560 payments (30 years × 52), weekly.
**Settings:** 365-day basis.
**Expected:** Weekly payment = $169.36.

#### Example 4: Canadian Mortgage
**Given:** $100,000 at 8% Canadian rate, 25 years, monthly payments.
**Expected:** Monthly payment = $763.21 (vs. $771.82 for US 8%).

#### Example 5: Interest-Only Loan with Balloon
**Given:** $100,000 at 8%, monthly payments = interest only ($666.67), 5-year balloon terminates loan.
**Expected:** Each payment is pure interest, balloon = $100,000.

#### Example 6: Adjustable Rate Loan
**Given:** $100,000 at 8%, monthly payments, rate adjusts to 9% after 5 years, then 10% after 10 years.
**Expected:** Payment changes at each adjustment date.

#### Example 7: Extra Annual Payment
**Given:** Standard 30-year mortgage with one extra monthly payment each December.
**Expected:** Loan pays off years early.

#### Example 8: Moratorium (Interest-Only Start)
**Given:** $100,000 at 8%, interest-only for first 2 years, then regular amortization begins.
**Expected:** First 24 payments are interest only ($666.67), then higher payments to amortize remaining over 28 years.

### 4.3 Present Value Examples

#### Example 1: Lump Sum Present Value
**Given:** Promise of $10,000 on 1/1/1999. As-of 1/1/1994. Rate 8%.
**Expected:** Present value ≈ $6,756

#### Example 2: Monthly Annuity
**Given:** $1,000/month for 10 years starting 1/1/1995. As-of 1/1/1995. Rate 6%.
**Expected:** Present value ≈ $90,073

#### Example 3: Future Value
**Given:** Same annuity, but as-of date is 10 years in the future.
**Expected:** Future value > sum of payments due to interest accumulation.

#### Example 4: IRR Computation
**Given:** Investment of $50,000 today returns $1,000/month for 5 years.
**Action:** Enter -$50,000 as lump sum, $1,000 periodic. Leave rate blank.
**Expected:** System computes the IRR.

#### Example 5: Annuity with COLA
**Given:** $2,000/month pension with 3% annual COLA. As-of date at retirement. Rate 5%.
**Expected:** Present value higher than without COLA.

---

## 5. Data Types and Formatting

### 5.1 Cell Types

| Type | Display Format | Examples |
|------|---------------|----------|
| Dollar | xx,xxx.xx | 200,000.00 |
| Percentage (4 decimal) | xx.xxxx | 8.2500 |
| Percentage (2 decimal) | xx.xx | 20.00 |
| Integer | xx,xxx | 360 |
| Date | MM/DD/YYYY | 01/15/2024 |
| Computed output | Same as above but with green (#C0DCC0) background | — |

### 5.2 Cell Status

Each cell has a status:
- **Empty** — no value entered
- **Input (hard data)** — user-entered value, white background
- **Output (soft data)** — computed by the system, green background
- Computed values are cleared and recomputed when any input in the same row/context changes

### 5.3 Date Handling

- Accept both MM/DD/YYYY and YYYY-MM-DD
- Two-digit years interpreted using the century divider setting
- Internal calculations use a calendar-aware date system handling leap years correctly

---

## 6. Export and Output

### 6.1 Table Output Options

For amortization schedules and present value tables:
- **Detail lines**: Show every individual payment
- **Summary only**: Show totals by period (yearly, quarterly, etc.)
- **Detail + Summary**: Show both
- **Export format**: Display on screen, CSV file, or tab-separated file

### 6.2 Clipboard

The system should support copy/paste of grid data with configurable delimiters (tab, newline, or custom).

---

## 7. Non-Requirements (Excluded from Port)

The following features from the Windows version are **not** being ported:

- **Software registration / licensing** — the web app will not require registration keys
- **Hard drive serial number checks** — not applicable to web
- **Windows Registry storage** — replaced by web-native storage (localStorage or server-side)
- **MDI (Multiple Document Interface)** — replaced by tab/screen navigation
- **Windows HTML Help (HHCtrl)** — replaced by integrated web help
- **File save/load of .MTG/.AMZ/.PVL binary files** — future consideration for import/export

---

## 8. Technical Constraints

### 8.1 Financial Arithmetic

- All monetary arithmetic must be deterministic and auditable
- Use banker's rounding (round half to even) unless original code differs — the original uses round-half-down for `Round2()`
- No floating-point in the financial calculation path at API boundaries; internal calculations preserve Pascal `real` behavior for algorithm fidelity
- Rounding discrepancies are absorbed in the final payment of amortization schedules

### 8.2 Calculation Precision

- Interest rate conversions between true rate, loan rate, and yield must be exact to at least 4 decimal places
- APR calculations use iterative convergence (Newton-Raphson) and must converge to within 0.0001%
- Present value summations using the formula method and exact method may differ by a few dollars per $10,000 — this is expected and documented

### 8.3 Edge Cases

- Zero interest rate loans must be handled (straight division of principal)
- Very short loans (1-2 periods) must work correctly
- Balloon payments that exceed the remaining balance should be flagged
- Negative amortization (payment < interest) must be handled per USA Rule or Actuarial settings
- Loans where first payment date is more than one period after loan date (odd first period)
