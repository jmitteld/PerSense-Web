# Amortization UI test cases (`amort_ui_cases.json`)

109+ Amortization test cases for exhaustive testing. Each entry is the **exact JSON
body the Amortization screen POSTs** to `/api/amortization/calc` (same shape as
`getAmzInput` in `cmd/persense/static/index.html` → `AmortizationRequest` in
`internal/api/handlers.go`), so the cases double as UI inputs and API payloads.

## Run

```
go test ./internal/api/ -run TestAmortUICasesExhaustive          # pass/fail
go test ./internal/api/ -run TestAmortUICasesExhaustive -v       # + per-case logs / residual tails
```

The runner (`amort_ui_cases_test.go`) drives each case through the real handler and
checks the per-row schedule invariants (Part C of `docs/ui_test_plan_comprehensive.md`):
balance recursion, no negative balance, intToDate = running interest sum & monotonic,
final intToDate = totalInterest, and amortization to ~0 (gross-failure guard). Cases
flagged `expectError` must surface an error; a few carry an `expectPayment` golden.

## Regenerate / extend

Edit and re-run the generator, then re-run the test:

```
python3 scripts/gen_amort_ui_cases.py
```

Add cases by appending `add(category, desc, request, expectError=, expectPayment=)`
calls. Because validation is invariant-based (not golden-value based), new cases need
no hand-computed expected outputs — just a valid request.

## Coverage (categories)

plain-forward (amount × rate × term × perYr × basis), plain-golden, first-period
(odd short/long, firstIntPrepaid), solve-amount / solve-rate / solve-periods /
derive-term, settings (in-advance, exact+365, rule78, USA-rule, points/APR),
balloon / balloon-target, prepay / prepay-solve, arm (rate / payment / both /
re-amortize / multiple), moratorium, target, skip, combo (balloon+prepay, balloon+ARM,
moratorium+balloon, target+skip, …), and edge/error (rate 0, very long, very high rate,
weekly/biweekly auto-365, n=1, last-before-first, unreachable target).

## Known residual tails (surfaced by the suite, `-v`) — CONFIRMED divergences

Three advanced-option cases leave a non-zero final balance: `balloon + ARM`,
`two balloons + ARM`, and `skip 1-3,7`. These were chased to the real DOS engine and are
**genuine divergences from DOS** (DOS amortizes each to $0; the port leaves a residual and
computes different total interest). Single options (balloon alone, ARM alone, skip excluding
the first payment month) match DOS. Full analysis + DOS-oracle goldens:
`docs/amort_option_combo_divergences.md`. They pass the gross-amortization guard so the suite
stays green; treat them as a known **S2** engine follow-up, not noise.
