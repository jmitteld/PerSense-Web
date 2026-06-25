# Per-period precedence audit: DOS `ComputeNext` vs Go `generateFancySchedule`

**Why:** the `mor+target` bug was a *precedence inversion* (Go applied a target during the interest-only
moratorium because it ran the option blocks independently instead of in DOS's ordered else-if chain).
That bug class is directly visible in the source, so this audit diffs DOS's authoritative per-period
branch ORDER against Go's, to find any other inversions before the global-Iterate refactor.

**Authority:** `Paymenttype.ComputeNext`, `legacy/src/dos_source/AMORTOP.pas:574-664`.
**Go:** `internal/finance/amortization/engine.go: generateFancySchedule`, the per-period block (~1525-1690).

## DOS canonical order (one period)

1. Advance `date` to the next regular payment (`AddPeriod`).
2. **SKIP** (`:599`): `if date.m in skipmonthset then payamt:=0 else payamt:=d`.
3. **EXTRA (balloon/prepay)** via `FindNextExtra` → `balloonpos` (`:600-622`):
   - `-1` balloon only: `payamt := nextextra.amount`.
   - `0` coincident: `plus_regular ? payamt += nextextra.amount : payamt := nextextra.amount`.
   - `1` regular only: unchanged.
4. `base_date := date` if `balloonpos >= 0` (`:623-624`).
5. **INTEREST** (`:625-637`): `interest := loanrate·timedif·(p-usap)` (daily: `(e^(truerate·td)-1)·(p-usap)`),
   `Round2` if `hard_payment`. (Depends only on `p,usap,timedif` — independent of `payamt`.)
6. **MORATORIUM / TARGET** — `case balloonpos` (`:639-653`), an **else-if** with moratorium FIRST:
   - `0`: `if mor: payamt := payamt-d+interest` ; `elif payamt-interest<target: payamt := payamt-d+target+interest`.
   - `1`: `if mor: payamt := interest` ; `elif payamt-interest<target: payamt := target+interest`.
   - `-1`: nothing.
7. `prevdate := date; p := p + interest - payamt;` US-rule `usap` update; `principal := p`.

The two structural invariants: **skip → extra → (moratorium ⊳ target)**, and a balloon coincident with
the moratorium is STILL paid (`balloonpos=0` mor branch keeps `payamt-d`, i.e. removes only the regular
`d`, leaving `balloon + interest`).

## Go order (one period)

1. **INTEREST** `intThisPd` computed first (`:1525-1542`) — value identical to DOS step 5 (same `p,usap`).
2. **SKIP** (`:1545-1550`): `pmt := d; if skip month: pmt := 0`.
3. **MORATORIUM** (`:1563-1593`): `if in window: pmt := intThisPd`; else boundary recompute.
4. **EXTRA (balloon + prepay)** (`:1607-1674`): accumulate coincident extras, then
   `if anyCoincident { plus_regular ? pmt += extra : pmt = extra }`; `pmt += offCycleExtra`.
5. **TARGET** (`:1686-1690`, now gated `!inMoratorium`): `if pmt-intThisPd < target: pmt = target+intThisPd`.
6. payoff / final-fold; `p := p + intThisPd - pmt`.

## Findings

| # | Area | DOS | Go | Verdict |
|---|---|---|---|---|
| 1 | skip→extra→target ordering | skip 0s payamt, target later forces `target+interest` | same | ✓ match (incl. **target overrides skip**, guarded by `TestHelpAM_TargetOverridesSkipInteraction`) |
| 2 | **moratorium ⊳ target** | else-if, moratorium first | `!inMoratorium` gate (FIXED) | ✓ match (`mortarget_test.go`) |
| 3 | moratorium vs extra ORDER | extra **before** moratorium | moratorium **before** extra | ✓ **equivalent for `plus_regular=true`** (the real default): both yield `interest + balloon`. See note A. |
| 4 | balloon coincident w/ moratorium still paid | yes (`payamt-d+interest` keeps balloon) | yes (`pmt = intThisPd + coincidentExtra`) | ✓ match (plus_regular=true) |
| 5 | target formula w/ coincident balloon | `payamt-d+target+interest` (keeps balloon) | `pmt = target+intThisPd` (drops balloon) | ⚠ latent — see note B (does not trigger in practice) |
| 6 | `plus_regular=false` + balloon + moratorium | `balloon-d+interest` | `pmt = balloon` (no interest) | ⚠ latent — see note C (non-default flag) |

**Note A (order difference is benign at the default).** Go runs moratorium before the extra; DOS runs the
extra before moratorium. For `plus_regular=true` (Anthropic/DOS real-world default — UI "includes regular
= NO" maps to `plus_regular=TRUE=ADD`) the algebra coincides: Go `intThisPd + balloon` == DOS
`(d+balloon)-d+interest`. Confirmed cent-exact on `mor+balloon` (tracker `moratorium_plus_balloon`).

**Note B (target + coincident balloon, non-triggering).** If a target *and* a balloon land on the same
date AND the target binds, DOS keeps the balloon (`payamt-d+target+interest`) while Go would replace `pmt`
with `target+intThisPd` (dropping the balloon). In practice the guard `pmt-intThisPd < target` is false
whenever a balloon coincides (balloon ≫ target), so neither engine fires the target there. Not observed in
any fuzzer case. Flagged for the refactor, which removes the ambiguity by porting the `case balloonpos`
formula verbatim.

**Note C (`plus_regular=false` + balloon inside the moratorium).** DOS's `case` comment assumes
`payamt = d+balloon`, but for `plus_regular=false` it set `payamt = balloon`, so its `payamt-d` term yields
`balloon-d+interest` — a DOS quirk. Go yields `balloon` (no interest, no `-d`). This only differs when the
non-default REPLACE flag is combined with a balloon dated inside an interest-only window — an exotic 3-way
corner not exercised by the fuzzer (which runs `plus_regular=true`). Flagged for the refactor.

## Conclusion

The per-period precedence is **DOS-faithful for every default-flag, realistically reachable case**. The
only residual discrepancies (notes B, C) are non-default (`plus_regular=false`) or non-triggering
(target∧balloon coincidence) corners, and both are subsumed by the global-Iterate refactor's plan to port
the `case balloonpos` block verbatim. Critically, **the audit found no second active inversion** — so the
remaining fuzzer frontier (multi-ARM+skip, balloon+multi-ARM tails, deep `mor` stacks) is NOT a precedence
problem; it is the multi-event *re-amortization composition* problem the refactor targets.
