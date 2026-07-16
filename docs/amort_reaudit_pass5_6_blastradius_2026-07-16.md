# Amortization re-audit — pass5/6 fix blast radius (2026-07-16)

_Triggered by "another session found + fixed an issue; do another auditing pass."
The fix under audit: the 2026-07-16 pass5/pass6 change to `engine.go` (payment-only
implied-rate adjustments at day-count frequencies). See
`amort_pass5_6_diagnosis_2026-07-16.md`._

## Setup / provenance
- Audited the **current** on-disk amortization tree (the fix on `engine.go` was
  **uncommitted** on the working tree at audit time — verified via checksum that the
  audit ran against exactly what's on disk, incl. `dosport_entry.go` and the pass5–8
  tests). Dependency packages (`dateutil`, `types`, `finance/interest`) matched exactly.
- Rebuilt the Linux DOS `amort_oracle` (Free Pascal 3.2.2); the fix goldens reproduce
  exactly (`100000 0.06 72 24 adj=24::2083` → 1515.5786 / 22172.35, etc.).
- Baseline: full **oracle-gated** amortization package passes (`PERSENSE_REQUIRE_ORACLE=1`, 92s).

## What the fix changed (both SHARED dispatch arms)
1. Dropped `len(input.Adjustments)==0` on the odd-first/in-advance payment-refine arm
   → the refine (`dosIteratePayment`) now fires for adjustment loans.
2. Added `solveSegmentRate` after the uniform `solveAdjRate` for payment-only
   adjustments on `exactDaily || (dayCount && Basis != 360)`.

## Method
Focused differential sweep over the corner the dropped gate newly enables:
**adjustment** (payment-only / rate-only / both) × {ordinary, in-advance, exact} ×
{360, 365, 365/360} × pmts-yr {12, 24, 26, 52} × odd-first, comparing the **solved
regular payment AND total interest** to the oracle. Reused the vetted
`gzSettings`/`basisFlag` mappings. Plus a targeted odd-first-monthly-stub × adjustment check.

## Findings

### 1. Fix blast radius is CLEAN — 0 payment divergences
Across ~2,912 solvable adjustment cases (in-advance, exact, all bases, all frequencies)
**and** targeted odd-first-monthly-stub cases (payment/rate/both), the solved regular
payment matches DOS **to the cent** — 0 divergences, including corners the pass5/6
goldens never exercised (in-advance × adj, exact × adj, long odd-first stubs).
**No regression from the fix.**

### 2. Minor bounded interest residual (segment-rate convergence floor)
16 tiny total-interest residuals, **max Δ ≈ $2.64 on ~$70k = 3.79e-5 relative**, all on
**long weekly/biweekly non-360** adjustment loans (n ≈ 500–624). This is the
`solveSegmentRate` bisection's convergence limit; it scales slowly with segment length
(pass6 goldens at n≈78–156 are exact). Not structural, not user-impacting.
*Optional*: tighten `solveSegmentRate` convergence to close it.

### 3. Surfaced (NOT a fix issue): raw engine relies on the handler's day-count basis coercion
The sweep initially showed ~16% divergence — all on pmts-yr 26/52 with **Basis360**.
Root cause: DOS silently switches weekly/biweekly loans from a 360 basis to 365 before
amortizing (`Amortize.pas:297-303`; plain biweekly DOS 360 == 365 == 2372.7004). The Go
**API handler** performs this coercion (`internal/api/handlers.go:854`, with a user
warning), so **real users are unaffected** and it is already noted in
`docs/discrepancies.md`. The raw `Amortize` engine does **not** self-coerce — it assumes
callers (the handler) pre-coerce. Mirroring the coercion in the sweep took payments
136→**0** divergences. Pre-existing; independent of the pass5/6 fix.
*Optional hardening*: move the 360→365 day-count coercion into `Amortize` itself so no
direct engine caller (tests, future callers) can hit the wrong path.

## Verdict
The pass5/6 fix is **correct and its blast radius is clean** — no regressions, DOS-
faithful payments across the full adjustment space. Two low-priority follow-ups: (a) a
sub-1e-4 interest residual on very long day-count-non-360 adjustment loans, (b) consider
engine-level day-count basis coercion.

## Action items
- **Commit the `engine.go` pass5/6 fix** (working tree keeps getting reset).
- Optional: promote the blast-radius differential sweep as a committed regression guard
  (interest tol ~5e-5 relative to accommodate finding-2).
- Optional: finding-2 (tighten segment-rate solve) and finding-3 (engine-level coercion).
