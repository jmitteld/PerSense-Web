#!/usr/bin/env bash
# run_pvmtg_arm.sh — loop the PV and MORTGAGE fuzzer5 differentials over a seed
# range at a stated FUZZ_N, keeping the FULL per-seed log.
#
# WHY THIS EXISTS (round 29). The two carried-forward headline rows —
#   "Present value: 29,891 worksheets, 5,034,725 table lines, 0 divergences"
#   "Mortgage:      20,754 eval cases, 136,270 APR verdicts, 0 divergences"
# — were measured ONCE, in the round-10 assessment of 2026-07-31, and have been
# carried forward unverified through rounds 16, 18, 21, 22, 26, 27 and 28.
# Standing rule 11 says do not carry a claim forward without verifying it.
#
# There was no script to re-measure them. `run_arm.sh`, `run_plain_arm.sh` and
# `paired_regression.sh` are ALL amortization-only. This is the missing
# counterpart, and it exists in the repo rather than /tmp so the next round has
# it (standing rule: a script written to /tmp is lost with the container).
#
# ⚠️ THE SEEDS OF THE ORIGINAL RUN WERE NEVER RECORDED. Only `docs/discrepancies.md`
# §48 names any PV/mortgage seed set at all (20611/20612/20613 at N=4000;
# 20614/20615 at N=3000). A re-measurement therefore CANNOT reproduce 29,891 and
# 20,754 exactly — it can only re-establish the claim at a NAMED seed set and N.
# Say which. That is rule 9: never quote a rate without naming the population.
#
# ⚠️ `PERSENSE_FUZZ_N` and `PERSENSE_FUZZ_SEED` are read by EVERY fuzzer in the
# tree, and `PERSENSE_FUZZ_SEED` re-seeds every fuzzer a `-run` matches. Both are
# therefore scoped here to one package AND one anchored `-run`. Never widen it.
#
#   SURFACE=pv  ARMDIR=/tmp/r29/pv  FUZZ_N=1500 ./run_pvmtg_arm.sh 20611 20630
#   SURFACE=mtg ARMDIR=/tmp/r29/mtg FUZZ_N=1500 ./run_pvmtg_arm.sh 20614 20633
set -uo pipefail
LO=${1:?first seed}; HI=${2:?last seed}
N=${FUZZ_N:-1500}
DIR=${ARMDIR:?ARMDIR required}
SURFACE=${SURFACE:?SURFACE must be pv or mtg}
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

case "$SURFACE" in
  pv)  PKG=./internal/finance/presentvalue/; RUN='^TestDOSPVFuzzer5AllAdvancedOptions$' ;;
  mtg) PKG=./internal/finance/mortgage/;     RUN='^TestDOSMtgFuzzer5$' ;;
  *)   echo "SURFACE must be pv or mtg" >&2; exit 2 ;;
esac

# JOBS defaults to nproc-1, as everywhere else in this harness. Do NOT override:
# rounds 13-29 all got 2 cores and the oracle spawns are the bottleneck.
JOBS=${JOBS:-$(( $(nproc 2>/dev/null || echo 2) - 1 ))}; [ "$JOBS" -lt 1 ] && JOBS=1
mkdir -p "$DIR"

run_one() {  # $1=seed
  ( cd "$ROOT" && \
    PERSENSE_REQUIRE_ORACLE=1 PERSENSE_FUZZ_SEED="$1" PERSENSE_FUZZ_N="$N" \
      timeout 3600 go test "$PKG" -run "$RUN" -count=1 -v -timeout 55m \
      > "$DIR/seed_$1.log" 2>&1 )
  # R8 / round 22: a killed binary reports nothing, and nothing looks like
  # success. Record the exit status as its own artefact, always.
  echo "$?" > "$DIR/seed_$1.rc"
}
export -f run_one; export DIR N ROOT PKG RUN

seq "$LO" "$HI" | xargs -P "$JOBS" -I{} bash -c 'run_one {}'
echo "PVMTG ARM DONE surface=${SURFACE} seeds ${LO}-${HI} N=${N} -> $DIR"
