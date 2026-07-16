# Amortization audit pass — differential HARNESS bugs (2026-07-16)

_Follow-up pass ("we keep finding errors"). Method: per-row + multi-adjustment
differential vs the rebuilt Linux DOS `amort_oracle`, gated (`PERSENSE_REQUIRE_ORACLE=1`)._

## Headline: the recurring "errors" were in the differential HARNESSES, not the engine.
Three separate "divergences" resolved to Go and the oracle being fed **different
inputs**. With identical inputs the engine matches DOS to the cent across the board.

### 1. "adjustment + option combination gap" — HARNESS BUG (was a large false gap)
`docs/amort_adjustment_combination_finding.md` documented a big post-adjustment
divergence (balance relErr up to ~82×) for balloon+adjust / adjust+skip / adjust+target,
logged-but-not-asserted in `TestDOSFancyCombinationSweep`. Root cause: the `adjustRate`
helper emitted the oracle token **`adj=M:R:0`**, whose trailing `0` sets the adjustment
**payment to $0** (interest-only) in DOS, while the Go mutation applied a **rate-only**
change. Go and DOS amortized different loans. Proof: DOS `adj=15:0.09:` → interest
$20,411.69; DOS `adj=15:0.09:0` → $35,285.80. **Fix:** emit `adj=M:R:` (rate-only,
blank amount). All six combos now match to the cent (relErr ~1e-5) and are **asserted**.
The engine's adjustment+option composition is DOS-faithful. Doc corrected.

### 2. blast-radius "finding-2" (segment-rate interest residual) — HARNESS ARTIFACT
The prior pass reported a sub-1e-4 interest residual on long day-count-non-360
adjustment loans. Cause: the sweep fed Go a 6-decimal payment-adjustment amount while
the oracle got 2-decimal (`FormatFloat 'f',2`) — a 0.45¢ input difference that compounds.
**Fix:** quantize payment-adjustment amounts to cents. Re-run (N=5000): payFails 0 AND
**intFails 0** (was 16). No real engine residual.

### 3. day-count Basis360 divergence — pre-existing, handler-masked (unchanged from prior pass)
DOS silently switches biweekly/weekly 360→365 before amortizing; the Go **API handler**
does this coercion (`handlers.go:854`), the raw engine does not. Not user-reachable; not
a fix issue. (Optional hardening: coerce in `Amortize` itself.)

## Positive confirmations (engine is clean)
- **Per-row** differential over 1–3 mixed rate/payment adjustments × bases × freqs:
  0 structural row failures across ~2,300 cases; worst non-terminal per-row Δ sub-cent
  (only the known final-row sub-dollar terminal residual appears).
- **Blast-radius totals** (N=5000): 0 payment, 0 interest divergences.
- Pass5/6 fix remains clean (prior pass).

## Code changes (this pass)
- `internal/finance/amortization/dos_fancy_combination_test.go`: `adjustRate` token
  `:0` → `:` (rate-only); `cleanCombo` now includes balloon+adjust / adjust+skip /
  adjust+target (all asserted). Full gated package green (~101s).
- `docs/amort_adjustment_combination_finding.md`: RESOLVED banner (harness bug).

## Meta-lesson
Differential harnesses must feed Go and the oracle **byte-identical** inputs. Every false
"gap" this session came from a precision/encoding mismatch in the harness (rate 10dp,
payment 2-vs-6 dp, `:0` amount token). Audit the harness before believing an engine gap.
