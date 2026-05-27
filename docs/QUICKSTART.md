# Per%Sense Quick Start

A practical onboarding guide for the Per%Sense Go web port. Read
`CLAUDE.md` first for project context and conventions.

---

## Prerequisites

- Go 1.22 or newer (`go version`)
- Internet access on first build (pulls `github.com/shopspring/decimal`)
- A modern browser for the web UI

No database, no external services, no build step for the frontend. The
HTML/CSS/JS is embedded in the Go binary via `go:embed`.

---

## Build and Run

From the project root:

```bash
# Quick run (no compile artifact)
go run ./cmd/persense

# Compiled binary
go build -o persense ./cmd/persense
./persense                  # default port 8080
./persense -port 9090       # custom port
```

Open `http://localhost:8080` in a browser.

---

## Web UI Tour

The home screen offers four calculators:

1. **Welcome** — landing page with screen picker
2. **Mortgage** — comparison mortgage calculator (multi-row)
3. **Amortization** — full payment schedule with Advanced Options
4. **Present Value** — discounted-cash-flow calculator with optional
   backward solve

Each screen has a `Calculate` (or `Generate Schedule`) button. Outputs
appear inline. The UI does not persist state between page loads.

### Mortgage screen

Fill in any combination of `Price`, `% Down` (or `Cash` or `Financed`),
`Rate`, `Years`, `Tax`, optional `Points` and `Balloon`. The Calc engine
dispatches on field presence and can solve for any of:

- `Price` / `Monthly` / `Balloon Amount` — the headline missing fields
- `% Down` ↔ `Cash` ↔ `Financed` — any of the three derived from the
  other two and `Price`

APR is computed when there's enough data. Two adjacent flows are also
wired: **Compare APRs** (`/api/mortgage/compare`) finds the crossover
term between two mortgages with different rates/points, and **What-If**
(`/api/mortgage/whatif`) generates a series of rows varying one field
(rate, years, points, …) across a range.

### Amortization screen

Required inputs: `Loan Amount`, `Loan Date`, `Rate %`, `Pmts/Yr`, and
either `# Periods` or `Last Pmt Date`. Optional: `1st Pmt Date`
(defaults to `Loan Date + 1 period` if omitted), `Payment` (leave blank
or zero to have it computed exactly via the annuity formula).

Field-presence dispatch on the top row recognizes these combinations:
- Loan Date + Pmts/Yr (no 1st Pmt Date) → 1st Pmt Date defaulted to one period later
- 1st Pmt Date + # Periods → Last Pmt Date computed
- 1st Pmt Date + Last Pmt Date → # Periods computed
- Amount + Rate + # Periods (Payment blank) → Payment computed
- Amount + Payment + # Periods (Rate blank) → Rate solved
- Rate + Payment + # Periods + 1st Pmt Date (Amount blank) → Amount solved

The **Advanced Options** panel adds:
- **Prepayments** — extra periodic payments
- **Balloons** — one-time lump payments
- **Rate/Payment Adjustments** — ARM-style rate changes
- **Moratorium** — interest-only period
- **Target** — minimum principal reduction
- **Skip Months** — months without payments (e.g. `6-8,12`)

Filling any Advanced field automatically switches the engine into fancy
mode. See `docs/missing_flows.md`, `docs/missing_flows_pass2.md`, and
`internal/finance/amortization/` for the per-period order of
operations.

**Validation:** the engine refuses inconsistent input combinations
(balloon before first payment, duplicate adjustment dates, target >
amount/n, moratorium before first payment, etc.) before generating a
schedule. See `internal/finance/amortization/validate.go` for the full
list.

### Present Value screen

Two grids:
- **Single Payments (Lump Sums)** — `date` + `amount`, plus optional
  `value` (pin this row's expected PV) and `act` (life-contingency code:
  `N`,`L`,`D`,`1`,`2`,`E`,`B`)
- **Periodic Payments** — `fromDate` + `toDate` + `perYr` + `amount`,
  plus optional `cola`, `value`, and `act`

Plus the top-level controls: `As-of Date`, `Rate Type`, `Rate %`,
`Present Value` (read-only output).

The Rate Type selector (Loan Rate / True Rate / Yield) is converted
client-side; the API only accepts continuously-compounded `trueRate`.

**Backward solve** — leave one field blank and provide a target
`Present Value`:
- Blank `Rate` → solves for the IRR
- Blank `As-of Date` → solves for the date that produces the target PV
- Blank lump-sum amount → solves for the missing payment
- Blank lump-sum date → solves for when the payment occurs
- Blank periodic amount, fromDate, or toDate → solves for that field

Life contingency and variable-rate schedules are also exposed as
collapsible sections.

---

## API

Five POST endpoints plus a health check, all accepting and returning
JSON:

| Endpoint | Purpose |
|---|---|
| `POST /api/mortgage/calc` | Mortgage row Calc with optional APR |
| `POST /api/mortgage/compare` | APR crossover between two mortgages |
| `POST /api/mortgage/whatif` | Row generation varying one field |
| `POST /api/amortization/calc` | Schedule generation, supports Advanced Options |
| `POST /api/presentvalue/calc` | Forward PV or backward solve |
| `GET  /api/health` | `{"status":"ok"}` |

Optional fields in any request are JSON pointers: omit them entirely (do
not send `null`) to indicate "blank".

### Mortgage — forward example

```bash
curl -s -X POST http://localhost:8080/api/mortgage/calc \
  -H 'Content-Type: application/json' \
  -d '{
    "price":   200000,
    "pctDown": 0.20,
    "years":   30,
    "rate":    0.06,
    "points":  0
  }' | jq
```

Response includes `monthly`, `cash`, `financed`, and `apr` if there's
enough data.

### Amortization — solve for the payment

Omit `firstDate` (defaults to `loanDate + 1 period`) and `payment`
(triggers `SolvePayment`):

```bash
curl -s -X POST http://localhost:8080/api/amortization/calc \
  -H 'Content-Type: application/json' \
  -d '{
    "amount":   250000,
    "loanDate": "2024-01-01",
    "rate":     0.06,
    "perYr":    12,
    "nPeriods": 360
  }' | jq
```

Returns a schedule with the computed payment (~$1,498.88) and first row
dated 2024-02-01.

### Amortization — Advanced Options example

```bash
curl -s -X POST http://localhost:8080/api/amortization/calc \
  -H 'Content-Type: application/json' \
  -d '{
    "amount":    200000,
    "loanDate":  "2024-01-01",
    "firstDate": "2024-02-01",
    "rate":      0.06,
    "perYr":     12,
    "nPeriods":  360,
    "payment":   1199.10,

    "prepayments": [
      {"startDate":"2024-02-01","stopDate":"2029-02-01","perYr":12,"amount":100}
    ],
    "balloons":    [{"date":"2030-01-01","amount":50000}],
    "adjustments": [{"date":"2027-01-01","rate":0.05}],
    "moratorium":  "2025-01-01",
    "targetAmt":   500,
    "skipMonths":  "12"
  }' | jq
```

Returns `schedule[]` (one row per payment), `totalPaid`, `totalInterest`.

### Amortization — Computational Settings

The amortization request also accepts these settings fields (each maps
to a DOS Computational Setting; all default to the DOS default):

| Field | Type | Effect |
|---|---|---|
| `basis` | `"360"` / `"365"` / `"365/360"` | Day-count convention for fractional periods |
| `inAdvance` | bool | Interest charged at start of period (annuity-due) instead of end |
| `usaRule` | bool | US Rule — unpaid interest tracked separately, not compounded |
| `rule78` | bool | Rule-of-78 (sum-of-the-digits) interest allocation for basic schedules |
| `balloonIncludesRegular` | bool | When a balloon lands on a payment date, treat the balloon amount as the total for that period instead of adding it to the regular payment |
| `points` | float | Discount points as a fraction of the loan (e.g. `0.02` = 2 points). When supplied, the engine returns an APR. |

### Present Value — forward example

```bash
curl -s -X POST http://localhost:8080/api/presentvalue/calc \
  -H 'Content-Type: application/json' \
  -d '{
    "asOfDate": "2024-01-01",
    "rate":     0.06,
    "lumpSums": [
      {"date":"2025-01-01","amount":10000},
      {"date":"2026-01-01","amount":10000}
    ],
    "periodics": [
      {"fromDate":"2024-02-01","toDate":"2034-01-01","perYr":12,"amount":500}
    ]
  }' | jq
```

### Present Value — backward solve example

Solve for the missing rate:

```bash
curl -s -X POST http://localhost:8080/api/presentvalue/calc \
  -H 'Content-Type: application/json' \
  -d '{
    "asOfDate": "2024-01-01",
    "sumValue": 18334.71,
    "lumpSums": [{"date":"2026-01-01","amount":20000}]
  }' | jq
```

Solve for a missing lump-sum amount:

```bash
curl -s -X POST http://localhost:8080/api/presentvalue/calc \
  -H 'Content-Type: application/json' \
  -d '{
    "asOfDate": "2024-01-01",
    "rate":     0.06,
    "sumValue": 9433.96,
    "lumpSums": [{"date":"2025-01-01"}]
  }' | jq
```

The response shape is the same as forward calc; the previously-blank
field will be populated.

### Present Value — variable-rate schedule

Supply a `rateSchedule` to switch the engine into variable-rate mode
(forward-only; backward solving is not supported here, matching DOS
`PV_VariableRate`):

```bash
curl -s -X POST http://localhost:8080/api/presentvalue/calc \
  -H 'Content-Type: application/json' \
  -d '{
    "asOfDate": "2024-01-01",
    "rateSchedule": [
      {"date":"2024-01-01","trueRate":0.05},
      {"date":"2027-01-01","trueRate":0.06},
      {"date":"2030-01-01","trueRate":0.07}
    ],
    "periodics": [
      {"fromDate":"2024-02-01","toDate":"2034-01-01","perYr":12,"amount":500}
    ]
  }' | jq
```

The top-level `rate` field is ignored when a schedule is supplied. The
first entry's `date` is treated as "from the beginning" (matches the
DOS UI where the first row's date cell was non-editable).

### Present Value — life-contingency (actuarial)

Add an `actuarial` block to attach a life table and DOB; rows with an
`act` code (`L`, `D`, `1`, `2`, `E`, `B`) will be weighted by survival
probability. Omitting `pod` (Payment On Death) asks the backward solver
to compute it from a target `sumValue`.

```bash
curl -s -X POST http://localhost:8080/api/presentvalue/calc \
  -H 'Content-Type: application/json' \
  -d '{
    "asOfDate": "2024-01-01",
    "rate":     0.05,
    "actuarial": {
      "table1":  [[60,0.01],[65,0.015],[70,0.025],[75,0.04],[80,0.07]],
      "dob1":    "1959-06-01",
      "asOfNow": "2024-01-01"
    },
    "periodics": [
      {"fromDate":"2024-02-01","toDate":"2044-01-01","perYr":12,"amount":1000,"act":"L"}
    ]
  }' | jq
```

See `internal/api/handlers.go` for the request/response Go types and
`internal/api/pv_backward_test.go`, `pv_variablerate_test.go`, and
`internal/finance/presentvalue/actuarial_backward_test.go` for working
examples.

---

## Testing

```bash
# Whole suite (all packages)
go test ./...

# Single package, verbose
go test ./internal/finance/presentvalue/ -v

# One test
go test ./internal/finance/presentvalue/ -run TestRoundTripRate -v

# DOS reference-data cross-checks only
go test ./internal/finance/ -run TestCrossCheck -v
```

The DOS reference values used by the cross-checks live at
`legacy/reference-output/refdata.json`. They were generated by
`legacy/testharness/refdata.pas` under Free Pascal. When adding new
financial functions, run the harness and append new entries; do not
hand-edit the JSON.

### Test layout

The full suite is in the 600+ range; the files below are the
canonical landmarks for each package. Run `ls internal/.../` for the
exhaustive list (many `canary_*`, `error_messages_*`, and
`help_examples_*` files cover regression and frontend parity).

```
internal/finance/
  crosscheck_test.go              DOS forward regression cases
  crosscheck_backward_test.go     DOS regression for backward solvers

internal/finance/presentvalue/
  calc_test.go                    Forward PV
  backward_test.go                BackwardCalc round-trip + FirstPass classification
  backward_boundary_test.go       Threshold/edge cases (cola=rate, near-teeny rate, etc.)
  variablerate_test.go            PV variable-rate (forward + restricted backward)
  actuarial_backward_test.go      Unknown-POD solve and contingency-weighted backward
  extreme_test.go                 Stress tests

internal/finance/amortization/
  amortization_test.go            Forward schedule
  backward_test.go                SolveLoanAmount, SolveRate
  solvepayment_test.go            SolvePayment closed-form
  advanced_test.go                Each Advanced Option in isolation
  unknown_prepay_test.go          Unknown-prepayment solver
  target_balloon_test.go          Target balance + balloon iteration
  extreme_test.go                 Edge cases

internal/finance/mortgage/
  mortgage_test.go                Calc, APR, comparison
  rowgen_test.go                  Row generation (what-if)

internal/api/
  handlers_test.go                Forward API
  pv_backward_test.go             Backward PV via HTTP
  pv_variablerate_test.go         Variable-rate PV via HTTP
  pv_rate_interpretation_test.go  Rate-type round-trips
  amort_advanced_test.go          Advanced Options via HTTP
  mortgage_compare_whatif_test.go /compare and /whatif endpoints
  extreme_test.go                 Stress
```

---

## Project Layout (for new contributors)

```
persense-port/
├── CLAUDE.md                  ← read first; project conventions and ported-status
├── go.mod
├── scripts/
│   └── regen_refdata.sh       ← regenerate legacy/reference-output/refdata.json
├── cmd/persense/
│   ├── main.go                ← HTTP server, /api/* routes, embeds static
│   └── static/
│       ├── index.html         ← single-page UI (Tailwind CDN + vanilla JS)
│       └── help.html
├── internal/
│   ├── api/                   ← HTTP handlers, FieldError
│   ├── dateutil/              ← Date arithmetic ported from INTSUTIL.pas
│   ├── fileio/                ← Legacy file I/O
│   ├── finance/               ← All financial logic
│   │   ├── actuarial/         ← life tables, POD, contingency weighting
│   │   ├── amortization/      ← engine.go, backward.go, firstpass.go, types.go, validate.go
│   │   ├── interest/          ← Exxp, Lnn, Round2, RateFromYield, InterpretedRate
│   │   ├── mortgage/          ← mortgage.go, rowgen.go
│   │   └── presentvalue/      ← calc.go, backward.go, variablerate.go, types.go
│   └── types/                 ← DateRec, status enums, constants
├── docs/
│   ├── QUICKSTART.md          ← (this file)
│   ├── requirements.md        ← Worksheet specs (from Windows help docs)
│   ├── discrepancies.md       ← Known DOS↔port behavioral differences
│   ├── missing_flows.md       ← Historical field-presence dispatch audit
│   ├── missing_flows_pass2.md ← Second-pass audit (most items now closed)
│   ├── dispatch_gaps.md       ← Living gap doc (current Revision 9, 2026-05-26)
│   ├── help_examples_test_report.md ← Help-doc parity audit
│   ├── test_plan.md           ← Test strategy notes
│   └── usability_review.md    ← UI usability review
├── legacy/
│   ├── src/dos_source/        ← Original DOS Pascal — READ-ONLY, financial authority
│   ├── src/win_source/        ← Windows Pascal port — UI/help authority
│   ├── testharness/refdata.pas ← Free Pascal harness that produces refdata.json
│   └── reference-output/refdata.json ← DOS-known-good test values
└── pkg/                       ← (currently empty; reserved for future public packages)
```

---

## Common Tasks

### Add a new financial function

1. Read the original Pascal in `legacy/src/dos_source/`
2. Implement in the appropriate `internal/finance/<area>/` package
3. Add a `// Ported from legacy/src/dos_source/<File>.pas: <function>` comment
4. Write a `_test.go` with at least:
   - one round-trip test (if the function has an inverse)
   - one DOS-regression test driven from `refdata.json`
   - threshold/boundary cases per the patterns in `backward_boundary_test.go`
5. If new reference values are needed, add them to
   `legacy/testharness/refdata.pas`, regenerate the JSON via
   `scripts/regen_refdata.sh`, and update the cross-check tables in
   `internal/finance/crosscheck_test.go` /
   `crosscheck_backward_test.go`

### Wire a new field through the API

1. Add the request type's pointer field in `internal/api/handlers.go`
   (the request structs are declared near the top — `MortgageRequest`,
   `AmortizationRequest`, `PVRequest`, etc.)
2. Translate `nil → StatusEmpty`, present → `InOutInput` in the handler
3. Match the mortgage handler's pattern in `HandleMortgageCalc`
   (`internal/api/handlers.go`) — the block that flips each
   `*Status` field to `InOutInput` when the corresponding pointer is
   non-nil
4. Add an API integration test in `internal/api/*_test.go`
5. If the field gates a new code path, also add a UI input in
   `cmd/persense/static/index.html` and update the JS input-builders
   (search for the `getAmzInput` / `getMortgageInput` / `getPVInput`
   equivalents) that POST to the API

### Debug a calculation

1. Find the DOS reference value: search `refdata.json` for the input
   shape, or run the original program if available
2. Confirm forward calc matches in `crosscheck_test.go`
3. If forward is correct but backward is wrong, run the round-trip test
   in the appropriate `backward_test.go`
4. Cross-reference the DOS source — exact line numbers are in the
   ported function's GoDoc comments

### Read the DOS source directly

```bash
# Find a Pascal procedure
grep -n '^procedure ProcName' legacy/src/dos_source/*.pas
grep -n '^function FuncName'  legacy/src/dos_source/*.pas

# Find DOS-only conditional formula dispatch
grep -nE 'if .+(status\s*=\s*inp|status\s*<\s*defp)' legacy/src/dos_source/Mortgage.pas
```

The most-cited Pascal files (with line ranges noted in port comments):
- `Mortgage.pas` — mortgage Calc, APR comparison, row generation
- `PRESVALU.pas` — present value forward + backward calc
- `Amortize.pas` + `AMORTOP.pas` — amortization Calc + RepayFancyLoan engine
- `INTSUTIL.pas` — `NumberOfInstallments`, `AddPeriod`, basic math

---

## Useful Resources

- `docs/discrepancies.md` — when the port and DOS diverge intentionally
- `docs/dispatch_gaps.md` — the living gap doc (current state of port
  fidelity, by revision)
- `docs/missing_flows.md` and `docs/missing_flows_pass2.md` — historical
  field-presence dispatch audits; most items are now closed (cited
  from `dispatch_gaps.md`)
- `docs/requirements.md` — worksheet specs and example values
- `legacy/src/win_source/Help/*.html` — original user-facing documentation
