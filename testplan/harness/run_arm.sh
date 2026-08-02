#!/usr/bin/env bash
# run_arm.sh — run one fuzzer5 arm (a mode filter over a seed range) and keep
# the FULL per-seed output, not just the signal lines. The ledger, COVERAGE and
# `block coverage:` lines are what make a rate adjudicable (R5/R7/rule 9), and
# paired_regression.sh's grep -oE throws them away.
#
#   ARMDIR=/tmp/r18/non MODES=non FUZZ_N=400 ./run_arm.sh <binary> <lo> <hi>
set -uo pipefail
BIN=${1:?test binary}; LO=${2:?first seed}; HI=${3:?last seed}
N=${FUZZ_N:-400}
DIR=${ARMDIR:?ARMDIR required}
MODES=${MODES:-}
JOBS=${JOBS:-$(( $(nproc 2>/dev/null || echo 2) - 1 ))}; [ "$JOBS" -lt 1 ] && JOBS=1
mkdir -p "$DIR"

run_one() {  # $1=seed
  PERSENSE_FUZZ=1 PERSENSE_REQUIRE_ORACLE=1 \
  PERSENSE_FUZZ_MODES="$MODES" \
  PERSENSE_FUZZ_SEED="$1" PERSENSE_FUZZ_N="$N" \
    timeout 900 "$BIN" -test.run TestDOSFuzzer5AllAdvancedOptions -test.v \
    > "$DIR/seed_$1.log" 2>&1
}
# EXPORT EVERYTHING THE SUBSHELL READS. Round 17 lost a run to an unexported N
# and silently measured the default 300 instead of 400.
export -f run_one; export DIR N MODES BIN

seq "$LO" "$HI" | xargs -P "$JOBS" -I{} bash -c 'run_one {}'
echo "ARM DONE modes='${MODES}' seeds ${LO}-${HI} N=${N} -> $DIR"
